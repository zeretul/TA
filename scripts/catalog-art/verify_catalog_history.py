#!/usr/bin/env python3
"""Independently verify every prior registered URL against a generated catalog."""
import argparse
import hashlib
import io
import json
from pathlib import Path
from PIL import Image
from convert_native_webp_catalog import Catalog, valid_hash


def descriptors(catalog):
    result = {}
    aliases = dict(catalog.manifest.get("aliases", {}))
    for variant, definition in catalog.variants():
        key = variant["sha256"]
        result[key] = variant
        if variant.get("legacy_sha256"):
            alias = variant["legacy_sha256"]
            if alias in aliases and aliases[alias] != key:
                raise ValueError("Conflicting alias")
            aliases[alias] = key
    return result, aliases


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--generated-assets", type=Path, required=True)
    p.add_argument("--history-assets", type=Path, action="append", default=[])
    p.add_argument("--history-archive", type=Path, action="append", default=[])
    args = p.parse_args()
    generated = Catalog(args.generated_assets)
    images, aliases = descriptors(generated)
    histories = [Catalog(path) for path in args.history_assets] + [Catalog(path, True) for path in args.history_archive]
    if not histories:
        raise ValueError("At least one authoritative history source is required")
    for alias, target in aliases.items():
        if not valid_hash(alias) or target not in images or alias in images:
            raise ValueError("Invalid alias target or shadowed hash")
    checked = {}
    for history in histories:
        old_images, old_aliases = descriptors(history)
        urls = {f"{h}.{v.get('format') or 'png'}": h for h, v in old_images.items()}
        urls.update({h + ".png": h for h, v in old_images.items() if v.get("format") == "webp"})
        urls.update({alias + ".png": target for alias, target in old_aliases.items()})
        for filename, old_hash in urls.items():
            identity, extension = filename.split(".")
            canonical = identity if identity in images else aliases.get(identity) if extension == "png" else None
            if canonical not in images:
                raise ValueError(f"Historical URL is missing: {filename}")
            old_variant, new_variant = old_images[old_hash], images[canonical]
            old, new = history.read(old_variant), generated.read(new_variant)
            if hashlib.sha256(old).hexdigest() != old_hash or hashlib.sha256(new).hexdigest() != canonical:
                raise ValueError(f"Content hash mismatch for {filename}")
            if (old_variant.get("format") or "png") == "webp" and extension == "webp" and new != old:
                raise ValueError(f"Previously published native bytes changed: {filename}")
            with Image.open(io.BytesIO(old)) as a, Image.open(io.BytesIO(new)) as b:
                if a.size != b.size or a.convert("RGBA").tobytes() != b.convert("RGBA").tobytes():
                    raise ValueError(f"Historical image pixels changed: {filename}")
            checked[filename] = canonical + "." + (new_variant.get("format") or "png")
    print(json.dumps({"historical_urls_verified": len(checked), "all_content_hashes_match": True, "all_original_rgba_pixels_match": True, "prior_native_bytes_unchanged": True, "urls": checked}, indent=2))


if __name__ == "__main__":
    main()
