"""Read-only, allowlisted legacy media inventory and per-person asset mapping."""

from __future__ import annotations

from collections import defaultdict
from pathlib import Path, PurePosixPath, PureWindowsPath


MEDIA_TYPES = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".webp": "image/webp",
    ".gif": "image/gif",
    ".mp4": "video/mp4",
}


def _is_link_or_junction(path: Path) -> bool:
    if path.is_symlink():
        return True
    is_junction = getattr(path, "is_junction", None)
    if callable(is_junction) and is_junction():
        return True
    try:
        attributes = path.lstat().st_file_attributes
    except (OSError, AttributeError):
        return False
    return bool(attributes & 0x400)


def _safe_relative(value: str):
    if not isinstance(value, str) or not value or "\\" in value or ":" in value:
        return None
    posix, windows = PurePosixPath(value), PureWindowsPath(value)
    if posix.is_absolute() or windows.is_absolute() or windows.drive or ".." in posix.parts:
        return None
    if posix.suffix.lower() not in MEDIA_TYPES:
        return None
    return posix


def _no_link_path(root: Path, target: Path, *, file_required: bool) -> bool:
    try:
        relative = target.relative_to(root)
    except ValueError:
        return False
    if not relative.parts or any(part in {".", ".."} for part in relative.parts):
        return False
    cursor = root
    if _is_link_or_junction(cursor) or not cursor.is_dir():
        return False
    for index, part in enumerate(relative.parts):
        cursor = cursor / part
        if _is_link_or_junction(cursor):
            return False
        last = index == len(relative.parts) - 1
        if last and file_required:
            return cursor.is_file()
        if not cursor.is_dir():
            return False
    return not file_required and cursor.is_dir()


def inventory_media_roots(roots, digest_function):
    """Hash allowlisted media only; skipped entries never receive content reads or hashes."""
    inventory, skipped, errors, hashes = [], [], [], defaultdict(list)

    def visit(kind, root, folder):
        try:
            children = sorted(folder.iterdir(), key=lambda child: child.name.casefold())
        except OSError as error:
            errors.append({"source": kind, "error": str(error)})
            return
        for path in children:
            relative = path.relative_to(root).as_posix()
            extension = path.suffix.lower()
            if _is_link_or_junction(path):
                if extension in MEDIA_TYPES:
                    errors.append({"source": kind, "relative_path": relative, "error": "media_link_rejected"})
                else:
                    skipped.append({"source": kind, "relative_path": relative, "skip_reason": "link_not_opened"})
                continue
            if extension not in MEDIA_TYPES:
                if path.is_dir():
                    visit(kind, root, path)
                else:
                    skipped.append({"source": kind, "relative_path": relative, "skip_reason": "extension_not_allowlisted_content_not_opened"})
                continue
            if path.is_dir():
                visit(kind, root, path)
                continue
            if _safe_relative(relative) is None or not _no_link_path(root, path, file_required=True):
                errors.append({"source": kind, "relative_path": relative, "error": "media_path_rejected"})
                continue
            try:
                file_hash = digest_function(path)
                byte_count = path.stat().st_size
            except OSError as error:
                errors.append({"source": kind, "relative_path": relative, "error": str(error)})
                continue
            parts = PurePosixPath(relative).parts
            item = {
                "source": kind,
                "relative_path": relative,
                "sha256": file_hash,
                "bytes": byte_count,
                "extension": extension,
                "media_type": MEDIA_TYPES[extension],
            "source_directory_group": f"{kind}/{parts[0] if len(parts) > 1 else 'root'}",
            }
            inventory.append(item)
            hashes[file_hash].append(f"{kind}:{relative}")

    for kind, root in roots.items():
        if _is_link_or_junction(root) or not root.is_dir():
            errors.append({"source": kind, "relative_path": ".", "error": "source_root_missing_or_linked"})
            continue
        visit(kind, root, root)
    duplicates = [
        {"sha256": file_hash, "files": sorted(paths)}
        for file_hash, paths in sorted(hashes.items())
        if len(paths) > 1
    ]
    return inventory, skipped, errors, duplicates


def _author_asset_refs(row):
    refs = row.get("candidate_asset_paths", [])
    if not isinstance(refs, list):
        return []
    result = []
    for value in refs:
        if not isinstance(value, str) or ":" not in value:
            result.append({"source_ref": value, "scope_status": "invalid_reference"})
            continue
        source, relative = value.split(":", 1)
        safe = _safe_relative(relative)
        result.append({
            "source": source,
            "relative_path": relative,
            "scope_status": "authorized_root_candidate" if source in {"mother", "h3"} and safe is not None else ("outside_read_scope_not_opened" if source not in {"mother", "h3"} else "invalid_reference"),
        })
    return result


def build_portrait_audit(
    *,
    perform_scan,
    sources,
    roots,
    digest_function,
    project_relative_path,
):
    binding_document = sources.get("portrait_bindings", (None, {}))[1]
    binding_rows = binding_document.get("bindings", []) if isinstance(binding_document, dict) else []
    if not isinstance(binding_rows, list) or any(not isinstance(row, dict) for row in binding_rows):
        binding_rows = []
    media_document = sources.get("portrait_media", (None, {}))[1]
    media_rows = media_document.get("videos", []) if isinstance(media_document, dict) else []
    if not isinstance(media_rows, list) or any(not isinstance(row, dict) for row in media_rows):
        media_rows = []
    profile_document = sources.get("profiles", (None, {}))[1]
    formal_profiles = profile_document.get("profiles", []) if isinstance(profile_document, dict) else []
    if not isinstance(formal_profiles, list) or any(not isinstance(row, dict) for row in formal_profiles):
        formal_profiles = []
    builtin_rows = sources.get("denova_portraits", (None, []))[1]
    if not isinstance(builtin_rows, list) or any(not isinstance(row, dict) for row in builtin_rows):
        builtin_rows = []

    root_presence = {
        kind: root.is_dir() and not _is_link_or_junction(root)
        for kind, root in roots.items()
    }
    if perform_scan:
        inventory, skipped, scan_errors, duplicates = inventory_media_roots(roots, digest_function)
    else:
        inventory, skipped, scan_errors, duplicates = [], [], [], []
    by_media_path = {(row["source"], row["relative_path"]): row for row in inventory}
    video_metadata = {}
    declared_video_details = []
    for row in media_rows:
        kind = row.get("source")
        relative = _safe_relative(row.get("relative_path"))
        detail = {key: value for key, value in row.items() if key != "relative_path"}
        detail["relative_path"] = relative.as_posix() if relative is not None else None
        if kind in roots and relative is not None:
            detail["read_scope_status"] = "allowlisted_root_candidate_actual_file_check_recorded_separately"
        elif kind not in roots and relative is not None:
            detail["read_scope_status"] = "outside_read_scope_metadata_only_not_opened"
        else:
            detail["read_scope_status"] = "invalid_reference_not_opened"
        declared_video_details.append(detail)
        if kind in roots and relative is not None:
            video_metadata[(kind, relative.as_posix())] = row

    declared = set()
    for row in binding_rows:
        for ref in _author_asset_refs(row):
            if ref["scope_status"] == "authorized_root_candidate":
                declared.add((ref["source"], ref["relative_path"]))
    checked, missing, unsafe = [], [], []
    for kind, value in sorted(declared | set(video_metadata)):
        relative = _safe_relative(value)
        if kind not in roots or relative is None:
            unsafe.append({"source": kind, "relative_path": value, "reason": "path_or_media_type_rejected_before_open"})
            continue
        root = roots[kind]
        target = root.joinpath(*relative.parts)
        if not _no_link_path(root, target, file_required=True):
            missing.append({"source": kind, "relative_path": relative.as_posix(), "reason": "missing_or_linked_candidate"})
            continue
        media_item = by_media_path.get((kind, relative.as_posix()))
        checked.append({
            "source": kind,
            "relative_path": relative.as_posix(),
            "exists": media_item is not None,
            "actual_sha256": media_item.get("sha256") if media_item else None,
            "actual_bytes": media_item.get("bytes") if media_item else None,
            "media_type": media_item.get("media_type") if media_item else MEDIA_TYPES[relative.suffix.lower()],
        })
        if perform_scan and media_item is None:
            missing.append({"source": kind, "relative_path": relative.as_posix(), "reason": "candidate_not_in_scanned_inventory"})
    for row in binding_rows:
        for ref in _author_asset_refs(row):
            if ref["scope_status"] == "invalid_reference":
                unsafe.append({"source": ref.get("source"), "relative_path": ref.get("relative_path"), "source_ref": ref.get("source_ref"), "reason": "path_or_media_type_rejected_before_open"})

    binding_index = defaultdict(list)
    for row in binding_rows:
        if row.get("legacy_npc_id"):
            binding_index[row["legacy_npc_id"]].append(row)
    shared_pools = [
        {
            "portrait_key": row.get("portrait_key"),
            "legacy_npc_id": row.get("legacy_npc_id"),
            "legacy_tier": row.get("legacy_tier"),
            "asset_candidates": _author_asset_refs(row),
            "qualification": "shared occupation pool only; age, gender, appearance, setting, and visual review remain required",
        }
        for row in binding_rows
        if row.get("shared_template")
    ]
    formal_bindings, unbound = [], []
    for person in formal_profiles:
        source_key, name = person.get("source_key"), person.get("name")
        exact = binding_index.get(source_key, [])
        name_only = [row for row in binding_rows if name and row.get("name") == name]
        candidates = []
        for legacy in exact:
            refs = _author_asset_refs(legacy)
            verified, stills, videos = [], [], []
            for ref in refs:
                if ref.get("scope_status") != "authorized_root_candidate":
                    continue
                key = (ref["source"], ref["relative_path"])
                asset = by_media_path.get(key)
                if asset is None:
                    continue
                item = {
                    **ref,
                    "sha256": asset["sha256"],
                    "bytes": asset["bytes"],
                    "media_type": asset["media_type"],
                    "visual_review": "not_performed",
                    "scene_review_status": "unreviewed",
                    "auto_selection_allowed": False,
                }
                verified.append(item)
                (videos if asset["media_type"].startswith("video/") else stills).append(item)
            video_relations = []
            for video in videos:
                metadata = video_metadata.get((video["source"], video["relative_path"]))
                poster_candidates = [
                    {"source": still["source"], "relative_path": still["relative_path"], "sha256": still["sha256"], "visual_review": "not_performed"}
                    for still in stills
                ]
                video_relations.append({
                    **video,
                    "container_metadata": metadata,
                    "poster_candidates": poster_candidates,
                    "poster_status": "same_legacy_id_stills_are_candidates_visual_review_required" if poster_candidates else "poster_missing_manual_selection_required",
                })
            candidates.append({
                "legacy_npc_id": legacy.get("legacy_npc_id"),
                "candidate_pool_id": f"legacy-npc:{legacy.get('legacy_npc_id')}",
                "legacy_name": legacy.get("name"),
                "legacy_tier": legacy.get("legacy_tier"),
                "shared_template": legacy.get("shared_template", False),
                "legacy_status": legacy.get("status"),
                "name_equal": legacy.get("name") == name,
                "asset_candidates": refs,
                "verified_media_candidates": verified,
                "scene_review_status": "unreviewed",
                "auto_selection_allowed": False,
                "videos_and_posters": video_relations,
                "identity_visual_qualification": "not_performed",
            })
        if len(exact) > 1:
            status = "ambiguous_duplicate_legacy_source_key"
        elif exact and exact[0].get("name") != name:
            status = "exact_source_key_name_conflict_manual_review"
        elif exact:
            status = "exact_source_key_candidate_visual_review_required"
        elif name_only:
            status = "name_only_ambiguous_candidates_not_bound"
            candidates = [{
                "legacy_npc_id": row.get("legacy_npc_id"),
                "legacy_name": row.get("name"),
                "asset_candidates": _author_asset_refs(row),
                "binding_status": "name_only_candidate_not_bound",
            } for row in name_only]
        else:
            status = "no_explicit_asset_candidate"
        binding = {
            "source_key": source_key,
            "formal_name": name,
            "country": person.get("country"),
            "role": person.get("role"),
            "binding_status": status,
            "identity_evidence": "exact_source_key" if exact else ("display_name_only" if name_only else "none"),
            "candidate_pool": {
                "pool_id": f"formal-person:{source_key}",
                "source_key": source_key,
                "legacy_id_evidence": [row.get("legacy_npc_id") for row in exact],
                "candidate_pool_ids": [f"legacy-npc:{row.get('legacy_npc_id')}" for row in exact],
                "verified_media_refs": [
                    {"legacy_npc_id": candidate.get("legacy_npc_id"), "source": media.get("source"), "relative_path": media.get("relative_path"), "sha256": media.get("sha256")}
                    for candidate in candidates for media in candidate.get("verified_media_candidates", [])
                ],
                "scene_review_status": "unreviewed" if exact or name_only else "no_explicit_asset_candidate",
                "auto_selection_allowed": False,
                "runtime_import_allowed": False,
            },
            "candidates": candidates,
            "runtime_imported": False,
        }
        formal_bindings.append(binding)
        if status != "exact_source_key_candidate_visual_review_required":
            unbound.append(binding)

    formal_keys = {row.get("source_key") for row in formal_profiles}
    builtin_objects = []
    for row in builtin_rows:
        old_ids = [value.strip() for value in row.get("matched_old_ids", "").replace("|", ",").split(",") if value.strip()]
        legacy_candidates = sorted({old_id for old_id in old_ids if old_id in binding_index})
        formal_candidates = sorted({old_id for old_id in old_ids if old_id in formal_keys})
        builtin_objects.append({
            "checkout": row.get("checkout"),
            "pack_id": row.get("pack_id"),
            "runtime_id": row.get("id"),
            "name": row.get("name"),
            "matched_old_ids": old_ids,
            "formal_source_key_candidates": formal_candidates,
            "legacy_npc_candidates": legacy_candidates,
            "mapping_status": "explicit_id_candidate_manual_review" if old_ids else "unmatched_no_explicit_source_id",
            "runtime_imported": False,
        })
    return {
        "status": "read_only_asset_audit",
        "objects": [
            {"object_class": "legacy_npc_asset_sources", "source_key_field": "legacy_npc_id", "records": len(binding_rows), "runtime_imported": False},
            {"object_class": "denova_builtin_static_objects", "identity_fields": ["checkout", "pack_id", "id"], "records": len(builtin_objects), "runtime_imported": False},
            {"object_class": "denova_formal_author_profiles", "source_key_field": "source_key", "records": len(formal_profiles), "binding_rows": len(formal_bindings), "runtime_imported": False},
        ],
        "legacy_npc_objects": binding_rows,
        "denova_builtin_objects": builtin_objects,
        "denova_author_bindings": formal_bindings,
        "denova_unbound_profiles": unbound,
        "shared_occupation_pools": shared_pools,
        "declared_video_records": len(media_rows),
        "declared_video_metadata": declared_video_details,
        "declared_media_inventory": media_document.get("source_counts", {}) if isinstance(media_document, dict) else {},
        "checked_source_files": checked,
        "missing_source_files": missing,
        "unsafe_references": unsafe,
        "source_roots_present": root_presence,
        "media_inventory": inventory,
        "non_media_files": skipped,
        "scan_errors": scan_errors,
        "duplicate_content_hashes": duplicates,
        "full_scan_performed": perform_scan,
        "scope": {"mother": "legacy_project/头像母图", "h3": "legacy_project/H3生成"},
        "presentation_policy": {
            "main_story_avatars": "never_auto_display",
            "interactive_window": "user-selected person; stable still; one focused video plays once then returns to still",
            "video": {"poster": "required when formally bound; current associations are candidates only", "default_audio": "muted", "autoplay": "active visible window only", "scene_change": "user or confirmed state driven"},
            "gallery": {"auto_rotation_default": "off", "still_interval_seconds_when_user_enabled": 12, "video_policy_when_user_enabled": "play once, wait 3 seconds, then advance"},
            "manual_selection": "not implemented or visually verified",
            "runtime_verified": False,
        },
        "runtime_display_verified": False,
    }
