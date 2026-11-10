#!/usr/bin/env python3
"""Build a separate content-addressed lossless WebP catalog with history retention.

Never changes inputs. More than 32 distinct input images requires --all.
Use --history-assets or --history-archive for every prior delivered catalog whose
URLs must survive. Existing native WebP bytes are copied, never re-encoded.
"""
import argparse
import copy
from collections import Counter
import hashlib
import io
import json
from pathlib import Path
import tempfile
import zipfile
from PIL import Image, features, __version__ as pillow_version


def sha(data):
    return hashlib.sha256(data).hexdigest()


def valid_hash(value):
    return isinstance(value, str) and len(value) == 64 and all(c in "0123456789abcdef" for c in value)


class Catalog:
    def __init__(self, path, archive=False):
        self.path = Path(path).resolve(strict=True)
        self.archive = None
        self.source_stat = (self.path.stat().st_size, self.path.stat().st_mtime_ns)
        if archive:
            self.archive = zipfile.ZipFile(self.path)
            names = [n for n in self.archive.namelist() if n.endswith("/internal/catalogart/assets/manifest.json")]
            if len(names) != 1:
                raise ValueError("History archive must contain exactly one catalog manifest")
            self.prefix = names[0].removesuffix("manifest.json")
            self.raw = self.archive.read(names[0])
        else:
            self.raw = (self.path / "manifest.json").read_bytes()
        self.manifest = json.loads(self.raw)
        if self.manifest.get("schema") != "catalog-art.v1":
            raise ValueError("Unsupported manifest schema")

    def variants(self):
        for entry in self.manifest["entries"]:
            for v in entry["variants"].values():
                yield v, entry["kind"] == "definition"
        for retained in self.manifest.get("retained_images", []):
            yield retained, bool(retained.get("definition"))

    def read(self, variant):
        extension = variant.get("format") or "png"
        identity = variant["sha256"]
        if extension not in {"png", "webp"} or not valid_hash(identity):
            raise ValueError("Invalid image format or content hash")
        rel = f"images/{identity}.{extension}"
        return self.archive.read(self.prefix + rel) if self.archive else (self.path / rel).read_bytes()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-assets", type=Path, required=True)
    parser.add_argument("--dest-assets", type=Path, required=True)
    parser.add_argument("--history-assets", type=Path, action="append", default=[])
    parser.add_argument("--history-archive", type=Path, action="append", default=[])
    parser.add_argument("--all", action="store_true", help="Explicitly allow more than 32 unique input images")
    parser.add_argument("--verified-cache", type=Path, help="Optional separate checkpoint directory; cached bytes are fully reverified against each source")
    args = parser.parse_args()
    current = Catalog(args.source_assets)
    histories = [Catalog(p) for p in args.history_assets] + [Catalog(p, True) for p in args.history_archive]
    sources = [current] + histories
    identity = lambda e: (e["world"], e["kind"], e["id"], e["version"])
    current_keys = {identity(e) for e in current.manifest["entries"]}
    if len(current_keys) != len(current.manifest["entries"]):
        raise ValueError("Current manifest contains duplicate author identities")
    for history in histories:
        missing = {identity(e) for e in history.manifest["entries"]} - current_keys
        if missing:
            raise ValueError(f"Current manifest omits {len(missing)} historical author identities; provide the complete frozen merged manifest")
    target = args.dest_assets.resolve(strict=False)
    report_path = target.parent / (target.name + "-conversion-report.json")
    if target.exists() or report_path.exists():
        raise ValueError("Output or report exists; refusing to overwrite it")
    for source in sources:
        if target == source.path or source.path in target.parents or target in source.path.parents:
            raise ValueError("Input and output must be separate trees")
    if not features.check("webp"):
        raise ValueError("This Pillow build lacks WebP support")
    needed = {}
    for source in sources:
        for variant, definition in source.variants():
            if variant.get("storage_format") or variant.get("storage_sha256"):
                raise ValueError("The superseded storage-only prototype is not a supported input")
            identity, extension = variant["sha256"], variant.get("format") or "png"
            if not valid_hash(identity) or extension not in {"png", "webp"}:
                raise ValueError("Invalid source image identity")
            needed.setdefault((identity, extension), (source, variant))
    if len(needed) > 32 and not args.all:
        raise ValueError("More than 32 images; use a bounded fixture or explicitly pass --all")
    target.parent.mkdir(parents=True, exist_ok=True)
    conversions = {}
    records = []
    cache_hits = 0
    cache = args.verified_cache.resolve(strict=False) if args.verified_cache else None
    if cache:
        if cache == target or target in cache.parents or cache in target.parents:
            raise ValueError("Checkpoint cache must be outside generated assets")
        for source in sources:
            if cache == source.path or source.path in cache.parents or cache in source.path.parents:
                raise ValueError("Checkpoint cache must be separate from inputs")
        cache.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".native-webp-", dir=target.parent) as tmp:
        staging = Path(tmp) / "assets"
        (staging / "images").mkdir(parents=True)
        for index, ((identity, extension), (source, variant)) in enumerate(sorted(needed.items()), 1):
            original = source.read(variant)
            if len(original) > 8*1024*1024 or sha(original) != identity:
                raise ValueError(f"Original image hash mismatch: {identity}")
            with Image.open(io.BytesIO(original)) as picture:
                picture.load()
                if picture.mode not in {"RGB", "RGBA"}:
                    raise ValueError(f"Unreviewed image mode: {identity} {picture.mode}")
                if (variant["width"], variant["height"]) != picture.size or not all(1 <= n <= 2048 for n in picture.size):
                    raise ValueError("Manifest and decoded dimensions differ")
                if any(k in picture.info for k in ("icc_profile", "gamma", "srgb", "chromaticity", "exif")):
                    raise ValueError(f"Color metadata needs explicit preservation: {identity}")
                if extension == "png":
                    if picture.format != "PNG" or len(original) < 26 or original[24] != 8:
                        raise ValueError("Only verified 8-bit PNG conversion is supported")
                    checkpoint = cache / (identity + ".webp") if cache else None
                    if checkpoint and checkpoint.is_file():
                        encoded = checkpoint.read_bytes()
                        cache_hits += 1
                    else:
                        output = io.BytesIO()
                        picture.save(output, format="WEBP", lossless=True, method=6, exact=True)
                        encoded = output.getvalue()
                else:
                    encoded = original
                if encoded[:4] != b"RIFF" or encoded[8:16] != b"WEBPVP8L":
                    raise ValueError("Only plain lossless VP8L WebP is accepted")
                with Image.open(io.BytesIO(encoded)) as decoded:
                    if decoded.size != picture.size or decoded.convert("RGBA").tobytes() != picture.convert("RGBA").tobytes():
                        raise ValueError(f"Lossless RGBA verification failed: {identity}")
                if extension == "png" and cache and not checkpoint.exists():
                    pending = checkpoint.with_suffix(".webp.pending")
                    pending.write_bytes(encoded)
                    pending.replace(checkpoint)
                native = {"sha256": sha(encoded), "width": picture.width, "height": picture.height, "format": "webp"}
                if extension == "png":
                    native["legacy_sha256"] = identity
                elif variant.get("legacy_sha256"):
                    native["legacy_sha256"] = variant["legacy_sha256"]
                conversions[(identity, extension)] = native
                (staging / "images" / f"{native['sha256']}.webp").write_bytes(encoded)
                records.append({"original_sha256": identity, "original_format": extension, "storage_sha256": native["sha256"], "original_bytes": len(original), "storage_bytes": len(encoded), "rgba_equal": True, "native_bytes_unchanged": extension == "webp"})
            print(f"{index}/{len(needed)} verified {identity}", flush=True)
        for source in sources:
            for variant, _ in source.variants():
                native = conversions[(variant["sha256"], variant.get("format") or "png")]
                if (variant["width"], variant["height"]) != (native["width"], native["height"]):
                    raise ValueError("A repeated source variant has inconsistent dimensions")
        manifest = copy.deepcopy(current.manifest)
        manifest.pop("aliases", None)
        manifest.pop("retained_images", None)
        active = {}
        aliases = {}
        for entry in manifest["entries"]:
            for role, old in list(entry["variants"].items()):
                native = copy.deepcopy(conversions[(old["sha256"], old.get("format") or "png")])
                entry["variants"][role] = native
                active[native["sha256"]] = active.get(native["sha256"], False) or entry["kind"] == "definition"
                if native.get("legacy_sha256"):
                    aliases[native["legacy_sha256"]] = native["sha256"]
        retained = {}
        for source in sources:
            for old, definition in source.variants():
                native = conversions[(old["sha256"], old.get("format") or "png")]
                identity = native["sha256"]
                if native.get("legacy_sha256"):
                    aliases.setdefault(native["legacy_sha256"], identity)
                if identity not in active or (definition and not active[identity]):
                    row = retained.setdefault(identity, {k: v for k, v in native.items() if k != "legacy_sha256"})
                    row["definition"] = row.get("definition", False) or definition
            for alias, old_target in source.manifest.get("aliases", {}).items():
                matches = [v for (h, fmt), v in conversions.items() if h == old_target]
                if not valid_hash(alias) or len(matches) != 1:
                    raise ValueError("Historical alias has an invalid or missing target")
                aliases.setdefault(alias, matches[0]["sha256"])
        native_ids = set(active) | set(retained)
        for alias, identity in aliases.items():
            if not valid_hash(alias) or identity not in native_ids or alias in native_ids:
                raise ValueError("Alias shadows a content hash or has no target")
        if retained:
            manifest["retained_images"] = [retained[h] for h in sorted(retained)]
        if aliases:
            manifest["aliases"] = dict(sorted(aliases.items()))
        historical_urls = {}
        for source in histories:
            for old, _ in source.variants():
                extension = old.get("format") or "png"
                identity = old["sha256"]
                native = conversions[(identity, extension)]
                historical_urls[f"{identity}.{extension}"] = native["sha256"]
                if extension == "webp":
                    historical_urls[f"{identity}.png"] = native["sha256"]
                if old.get("legacy_sha256"):
                    historical_urls[f"{old['legacy_sha256']}.png"] = aliases[old["legacy_sha256"]]
            for alias in source.manifest.get("aliases", {}):
                historical_urls[f"{alias}.png"] = aliases[alias]
        for file, expected in historical_urls.items():
            identity, extension = file.split(".")
            resolved = identity if identity in native_ids else aliases.get(identity) if extension == "png" else None
            if resolved != expected or not (staging / "images" / f"{resolved}.webp").is_file():
                raise ValueError(f"Historical URL coverage failed: {file}")
        (staging / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        report = {"source_manifest_sha256": sha(current.raw), "history_manifest_sha256": [sha(s.raw) for s in histories], "encoder": {"pillow": pillow_version, "libwebp": features.version("webp"), "lossless": True, "method": 6, "exact": True}, "images": len(needed), "verified_checkpoint_reuses": cache_hits, "current_images": len(active), "current_entries": len(manifest["entries"]), "current_world_entries": dict(Counter(e["world"] for e in manifest["entries"])), "retained_images": len(retained), "original_bytes": sum(r["original_bytes"] for r in records), "storage_bytes": sum(p.stat().st_size for p in (staging / "images").glob("*.webp")), "all_rgba_equal": True, "legacy_aliases": len(aliases), "historical_urls_verified": len(historical_urls), "historical_urls": historical_urls, "records": records}
        for source in sources:
            if source.archive:
                unchanged = (source.path.stat().st_size, source.path.stat().st_mtime_ns) == source.source_stat
            else:
                unchanged = (source.path / "manifest.json").read_bytes() == source.raw
            if not unchanged:
                raise ValueError("An input manifest/archive changed during conversion; freeze resources and retry separately")
        staging.rename(target)
        report_path.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: v for k, v in report.items() if k not in {"records", "historical_urls"}}), flush=True)


if __name__ == "__main__":
    main()
