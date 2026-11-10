"""Safe offline media inventory and thumbnail derivation."""

from __future__ import annotations

import hashlib
import json
import os
import stat
import tempfile
from pathlib import Path
from typing import Iterable

from PIL import Image, ImageOps, __version__ as PILLOW_VERSION

IMAGE_TYPES = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".webp": "image/webp",
    ".bmp": "image/bmp",
}
VIDEO_TYPES = {".mp4": "video/mp4"}
REPARSE_POINT = 0x400
THUMBNAIL_VERSION = "portrait-thumb-v1"


def is_linklike(path: Path) -> bool:
    """Return true for symlinks and Windows junction/reparse points."""
    try:
        if path.is_symlink():
            return True
        junction_check = getattr(path, "is_junction", None)
        if junction_check is not None and junction_check():
            return True
        attributes = getattr(path.lstat(), "st_file_attributes", 0)
        return bool(attributes & REPARSE_POINT)
    except OSError:
        return True


def safe_resolve(root: Path, relative: str, *, must_exist: bool = True) -> Path:
    root = root.resolve(strict=True)
    candidate_rel = Path(relative)
    if candidate_rel.is_absolute() or any(part in {"..", ""} for part in candidate_rel.parts):
        raise ValueError("path must be relative and remain inside its approved root")
    candidate = root / candidate_rel
    current = root
    for part in candidate_rel.parts:
        current = current / part
        if current.exists() and is_linklike(current):
            raise ValueError("symbolic links and junctions are not accepted")
    resolved = candidate.resolve(strict=must_exist)
    try:
        resolved.relative_to(root)
    except ValueError as error:
        raise ValueError("path resolves outside its approved root") from error
    return resolved


def safe_output_path(root: Path, relative: str) -> Path:
    root.mkdir(parents=True, exist_ok=True)
    if is_linklike(root):
        raise ValueError("output root cannot be a link or junction")
    root = root.resolve(strict=True)
    relative_path = Path(relative)
    if relative_path.is_absolute() or any(part in {"..", ""} for part in relative_path.parts):
        raise ValueError("output path must remain inside the approved output root")
    target = root / relative_path
    target.parent.mkdir(parents=True, exist_ok=True)
    current = root
    for part in relative_path.parts:
        current = current / part
        if current.exists() and is_linklike(current):
            raise ValueError("output path cannot traverse a symbolic link or junction")
    try:
        target.parent.resolve(strict=True).relative_to(root)
    except ValueError as error:
        raise ValueError("output path resolves outside the approved output root") from error
    return target


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def write_if_changed(path: Path, content: bytes) -> bool:
    if path.exists() and not is_linklike(path) and path.read_bytes() == content:
        return False
    if path.exists() and is_linklike(path):
        raise ValueError("refusing to overwrite a link or junction")
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, prefix=".portrait-", delete=False) as stream:
        temporary = Path(stream.name)
        stream.write(content)
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)
    return True


def _load_cache(output_root: Path) -> dict[str, dict[str, str]]:
    cache_path = output_root / "cache.json"
    if not cache_path.exists():
        return {}
    if is_linklike(cache_path):
        raise ValueError("cache cannot be a link or junction")
    try:
        value = json.loads(cache_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    if not isinstance(value, dict):
        return {}
    return value


def _save_cache(output_root: Path, cache: dict[str, dict[str, str]]) -> None:
    encoded = json.dumps(cache, ensure_ascii=False, sort_keys=True, indent=2).encode("utf-8") + b"\n"
    write_if_changed(safe_output_path(output_root, "cache.json"), encoded)


def _walk_media(root: Path, source: str) -> tuple[list[dict], list[dict]]:
    assets: list[dict] = []
    diagnostics: list[dict] = []
    if not root.exists() or not root.is_dir() or is_linklike(root):
        return assets, [{"code": "media_root_unavailable", "source": source, "path": root.name}]
    real_root = root.resolve(strict=True)
    stack = [real_root]
    while stack:
        directory = stack.pop()
        try:
            entries = sorted(os.scandir(directory), key=lambda entry: entry.name.casefold(), reverse=True)
        except OSError as error:
            diagnostics.append({"code": "directory_read_failed", "source": source, "path": directory.name, "detail": error.__class__.__name__})
            continue
        with_entries = entries
        for entry in with_entries:
            path = Path(entry.path)
            try:
                if entry.is_symlink() or is_linklike(path):
                    diagnostics.append({"code": "linked_path_skipped", "source": source, "path": path.relative_to(real_root).as_posix()})
                    continue
                resolved = path.resolve(strict=True)
                resolved.relative_to(real_root)
                if entry.is_dir(follow_symlinks=False):
                    stack.append(resolved)
                    continue
                if not entry.is_file(follow_symlinks=False):
                    continue
            except (OSError, ValueError):
                diagnostics.append({"code": "unsafe_or_unreadable_path_skipped", "source": source, "path": path.name})
                continue
            suffix = path.suffix.lower()
            media_type = IMAGE_TYPES.get(suffix) or VIDEO_TYPES.get(suffix)
            if media_type is None:
                continue
            relative_path = path.relative_to(real_root).as_posix()
            try:
                digest = sha256_file(resolved)
                size_bytes = resolved.stat().st_size
            except OSError as error:
                diagnostics.append({"code": "media_read_failed", "source": source, "path": relative_path, "detail": error.__class__.__name__})
                continue
            assets.append({
                "source": source,
                "relative_path": relative_path,
                "media_type": media_type,
                "bytes": size_bytes,
                "sha256": digest,
            })
    assets.sort(key=lambda item: (item["source"], item["relative_path"].casefold()))
    return assets, diagnostics


def _derive_thumbnail(path: Path, output_root: Path, asset_hash: str, size: int, cache: dict[str, dict[str, str]]) -> tuple[str | None, bool, dict | None]:
    key = f"{THUMBNAIL_VERSION}:{PILLOW_VERSION}:{asset_hash}:{size}:contain-lanczos"
    relative = f"derived/{asset_hash}-{size}.png"
    target = safe_output_path(output_root, relative)
    cached = cache.get(key)
    if cached and target.exists() and not is_linklike(target):
        try:
            if sha256_file(target) == cached.get("thumbnail_sha256"):
                return relative, True, None
        except OSError:
            pass
    try:
        with Image.open(path) as image:
            image = ImageOps.exif_transpose(image)
            image.load()
            width, height = image.size
            if width < 1 or height < 1:
                raise ValueError("image has empty dimensions")
            if image.mode not in {"RGB", "RGBA"}:
                image = image.convert("RGBA" if "A" in image.getbands() else "RGB")
            image.thumbnail((size, size), Image.Resampling.LANCZOS)
            from io import BytesIO

            buffer = BytesIO()
            image.save(buffer, format="PNG", optimize=False, compress_level=9)
            encoded = buffer.getvalue()
        write_if_changed(target, encoded)
        thumb_hash = sha256_file(target)
        cache[key] = {"thumbnail_path": relative, "thumbnail_sha256": thumb_hash}
        return relative, False, None
    except Exception as error:  # PIL raises several decoder-specific exception types.
        return None, False, {"code": "image_decode_failed", "path": path.name, "detail": error.__class__.__name__}


def scan_media_roots(roots: Iterable[tuple[str, Path]], output_root: Path, video_tool_available: bool) -> tuple[list[dict], list[dict], dict[str, list[str]]]:
    root_map = dict(roots)
    cache = _load_cache(output_root)
    assets: list[dict] = []
    diagnostics: list[dict] = []
    for source, root in root_map.items():
        found, root_diagnostics = _walk_media(root, source)
        diagnostics.extend(root_diagnostics)
        assets.extend(found)
    seen: dict[str, list[str]] = {}
    for asset in assets:
        seen.setdefault(asset["sha256"], []).append(f"{asset['source']}:{asset['relative_path']}")
        if asset["media_type"].startswith("image/"):
            source_root = root_map[asset["source"]]
            source_path = safe_resolve(source_root, asset["relative_path"])
            dimensions = None
            for size in (128, 192):
                thumb_path, cache_hit, decode_error = _derive_thumbnail(source_path, output_root, asset["sha256"], size, cache)
                if decode_error:
                    diagnostics.append({**decode_error, "source": asset["source"], "relative_path": asset["relative_path"]})
                    break
                asset.setdefault("thumbnails", {})[str(size)] = thumb_path
                if size == 128:
                    asset["cache_hit"] = cache_hit
                    try:
                        with Image.open(source_path) as image:
                            dimensions = list(ImageOps.exif_transpose(image).size)
                    except Exception:
                        dimensions = None
            asset["dimensions"] = dimensions
        else:
            asset["poster_status"] = "candidate_unreviewed"
            if not video_tool_available:
                asset["poster_status"] = "unavailable_no_local_video_tool"
    duplicates = {digest: paths for digest, paths in seen.items() if len(paths) > 1}
    for digest, paths in duplicates.items():
        diagnostics.append({"code": "duplicate_content_hash", "sha256": digest, "paths": paths})
    video_count = sum(asset["media_type"].startswith("video/") for asset in assets)
    if video_count and not video_tool_available:
        diagnostics.append({"code": "video_poster_tool_unavailable", "media_count": video_count, "detail": "ffmpeg and ffprobe were not both available"})
    _save_cache(output_root, cache)
    return assets, diagnostics, duplicates


def scan_candidate_images(candidate_root: Path, output_root: Path) -> tuple[list[dict], list[dict]]:
    if not candidate_root.exists():
        return [], []
    assets, diagnostics = _walk_media(candidate_root, "generated_candidate")
    cache = _load_cache(output_root)
    for asset in assets:
        if not asset["media_type"].startswith("image/"):
            continue
        path = safe_resolve(candidate_root, asset["relative_path"])
        dimensions = None
        for size in (128, 192):
            thumb_path, cache_hit, decode_error = _derive_thumbnail(path, output_root, asset["sha256"], size, cache)
            if decode_error:
                diagnostics.append({**decode_error, "source": asset["source"], "relative_path": asset["relative_path"]})
                break
            asset.setdefault("thumbnails", {})[str(size)] = thumb_path
            if size == 128:
                asset["cache_hit"] = cache_hit
                try:
                    with Image.open(path) as image:
                        dimensions = list(ImageOps.exif_transpose(image).size)
                except Exception:
                    dimensions = None
        asset["dimensions"] = dimensions
        asset["workspace_relative_path"] = f"assets/npc-portraits-candidate/{asset['relative_path']}"
    _save_cache(output_root, cache)
    return assets, diagnostics
