"""Join the authorized author, package-catalog, and scoped media evidence."""

from __future__ import annotations

import csv
import hashlib
import json
from pathlib import Path
from typing import Any

from scan import safe_resolve


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def _safe_profile(project_root: Path, relative: str) -> tuple[str, str | None]:
    try:
        path = safe_resolve(project_root / "设计稿", relative)
        content = path.read_bytes()
        return "verified", sha256_bytes(content)
    except (OSError, ValueError):
        return "missing_or_unsafe", None


def _asset_key(source: str, relative_path: str) -> tuple[str, str]:
    return source, relative_path.replace("\\", "/")


def build_people(project_root: Path, scanned_media: list[dict], generated_media: list[dict], package_snapshot: dict) -> tuple[list[dict], list[dict]]:
    author_path = project_root / "设计稿" / "资源清单" / "凡人人界_101人作者目录_20261015.json"
    catalog_path = project_root / "work" / "luna6-world-content" / "static-world-catalog.json"
    audit_path = project_root / "work" / "luna6-world-content" / "portrait-asset-audit.json"
    index_path = project_root / "设计稿" / "人物画像" / "凡人人界" / "画像索引.json"
    author = _read_json(author_path)
    catalog = _read_json(catalog_path)
    audit = _read_json(audit_path)
    portrait_index = _read_json(index_path)

    catalog_map: dict[str, dict] = {}
    for entry in catalog.get("characters", []):
        key = entry.get("source_key")
        if key in catalog_map and catalog_map[key].get("name") != entry.get("name"):
            raise ValueError(f"static catalog ID conflict for {key}")
        catalog_map[key] = entry
    audit_map = {entry.get("source_key"): entry for entry in audit.get("denova_author_bindings", [])}
    portrait_map = {entry.get("source_key"): entry for entry in portrait_index.get("profiles", [])}
    media_map = {_asset_key(asset["source"], asset["relative_path"]): asset for asset in scanned_media}
    generated_by_person: dict[str, list[dict]] = {}
    for asset in generated_media:
        relative = asset["relative_path"].replace("\\", "/")
        if "/" not in relative:
            continue
        stable_id = relative.split("/", 1)[0]
        generated_by_person.setdefault(stable_id, []).append(asset)

    conflicts: list[dict] = []
    seen_author_ids: dict[str, str] = {}
    people: list[dict] = []
    for profile in author.get("profiles", []):
        stable_id = profile.get("source_key", "")
        display_name = profile.get("name", "")
        previous_name = seen_author_ids.get(stable_id)
        if previous_name is not None and previous_name != display_name:
            conflicts.append({"stable_id": stable_id, "names": [previous_name, display_name], "code": "author_source_id_name_conflict"})
        seen_author_ids[stable_id] = display_name
        catalog_entry = catalog_map.get(stable_id)
        portrait_entry = portrait_map.get(stable_id)
        if catalog_entry and catalog_entry.get("name") != display_name:
            conflicts.append({"stable_id": stable_id, "names": [display_name, catalog_entry.get("name")], "code": "author_catalog_name_conflict"})
        profile_rel = profile.get("path", "")
        profile_status, profile_hash = _safe_profile(project_root, profile_rel)
        catalog_hash = catalog_entry.get("profile_sha256") if catalog_entry else None
        profile_matches_catalog = bool(profile_hash and catalog_hash and profile_hash == catalog_hash)
        audit_entry = audit_map.get(stable_id, {})
        binding = audit_entry.get("candidate_pool", {})
        binding_status = audit_entry.get("binding_status", "no_audit_record")
        media_refs: list[dict] = []
        reference_errors: list[dict] = []
        for ref in binding.get("verified_media_refs", []):
            source = ref.get("source")
            relative = ref.get("relative_path", "").replace("\\", "/")
            if source not in {"mother", "h3"} or not relative:
                continue
            found = media_map.get(_asset_key(source, relative))
            if found is None:
                reference_errors.append({"code": "audited_media_missing_or_out_of_scope", "source": source, "relative_path": relative})
                continue
            if found["sha256"] != ref.get("sha256"):
                reference_errors.append({"code": "audited_media_hash_changed", "source": source, "relative_path": relative})
                continue
            media_refs.append({**found, "identity_evidence": "old_asset_audit_exact_legacy_id_candidate", "scene_status": "unreviewed"})
        weak_name_hits: list[dict] = []
        if binding_status == "no_explicit_asset_candidate":
            terms = [term for term in (display_name, stable_id) if len(term) >= 2]
            for asset in scanned_media:
                if any(term.casefold() in asset["relative_path"].casefold() for term in terms):
                    weak_name_hits.append({**asset, "identity_evidence": "filename_only_unverified", "scene_status": "unreviewed"})
        direct_images = [asset for asset in media_refs if asset["media_type"].startswith("image/")]
        direct_videos = [asset for asset in media_refs if asset["media_type"].startswith("video/")]
        out_of_scope_paths = []
        out_of_scope_candidates = []
        legacy_audit_candidates = []
        for candidate in audit_entry.get("candidates", []):
            legacy_audit_candidates.append({
                "legacy_name": candidate.get("legacy_name", ""),
                "legacy_npc_id": candidate.get("legacy_npc_id", ""),
                "legacy_status": candidate.get("legacy_status", ""),
                "legacy_tier": candidate.get("legacy_tier", ""),
                "name_equal": candidate.get("name_equal"),
                "identity_visual_qualification": candidate.get("identity_visual_qualification", "not_performed"),
                "shared_template": candidate.get("shared_template", False),
            })
            for item in candidate.get("asset_candidates", []):
                scope_status = item.get("scope_status", "")
                if scope_status == "outside_read_scope_not_opened":
                    out_of_scope_paths.append(item.get("relative_path", ""))
                    out_of_scope_candidates.append({"source": item.get("source", ""), "relative_path": item.get("relative_path", ""), "scope_status": scope_status})
        conflict = "conflict" in binding_status or any(item.get("stable_id") == stable_id and item.get("code") in {"author_source_id_name_conflict", "author_catalog_name_conflict"} for item in conflicts)
        if conflict:
            legacy_names = [item.get("legacy_name", "") for candidate in audit_entry.get("candidates", []) for item in [candidate] if item.get("legacy_name")]
            conflicts.append({
                "stable_id": stable_id,
                "formal_name": display_name,
                "legacy_names": legacy_names,
                "code": "audit_exact_source_key_name_conflict_manual_review",
                "automatic_binding_allowed": False,
            })
        if conflict:
            asset_status = "identity_conflict_manual_review"
        elif direct_images:
            asset_status = "dedicated_old_image_candidate_visual_review_required"
        elif direct_videos:
            asset_status = "old_video_candidate_without_still"
        elif out_of_scope_paths:
            asset_status = "old_asset_metadata_outside_read_scope_unverified"
        elif weak_name_hits:
            asset_status = "filename_candidate_identity_unverified"
        elif reference_errors:
            asset_status = "old_audit_reference_unavailable_or_changed"
        else:
            asset_status = "no_verified_scoped_daily_portrait"
        generated_assets = generated_by_person.get(stable_id, [])
        people.append({
            "stable_id": stable_id,
            "name": display_name,
            "country": profile.get("country", ""),
            "role": profile.get("role", ""),
            "level": profile.get("level", ""),
            "initial_stage": profile.get("initial_stage", ""),
            "author_profile_path": profile_rel,
            "author_profile_hash": profile_hash,
            "author_profile_status": profile_status,
            "catalog_profile_hash": catalog_hash,
            "profile_hash_matches_catalog": profile_matches_catalog,
            "author_directory_member": True,
            "static_catalog_declared": catalog_entry is not None,
            "static_catalog_runtime_flag": catalog_entry.get("runtime_imported") if catalog_entry else None,
            "runtime_import_status": "unknown_not_verified",
            "portrait_index_status": portrait_entry.get("status") if portrait_entry else "not_indexed",
            "identity_evidence": audit_entry.get("identity_evidence", "no_explicit_id_mapping"),
            "asset_audit_status": binding_status,
            "asset_status": asset_status,
            "identity_conflict": conflict,
            "out_of_scope_candidate_count": len([path for path in out_of_scope_paths if path]),
            "out_of_scope_candidates": out_of_scope_candidates,
            "legacy_audit_candidates": legacy_audit_candidates,
            "old_image_count": len(direct_images),
            "old_video_count": len(direct_videos),
            "video_only": bool(direct_videos and not direct_images),
            "unmatched": binding_status == "no_explicit_asset_candidate",
            "old_media": media_refs,
            "weak_name_candidates": weak_name_hits,
            "generated_candidates": generated_assets,
            "reference_diagnostics": reference_errors,
            "candidate_pool_id": binding.get("pool_id", ""),
            "scene_review_status": binding.get("scene_review_status", "unreviewed"),
        })

    return people, conflicts


def build_initial_mapping(people: list[dict]) -> dict:
    entries = []
    for person in people:
        media = []
        for asset in person["old_media"]:
            media.append({
                "asset_path": f"legacy/{asset['source']}/{asset['relative_path']}",
                "source_sha256": asset["sha256"],
                "media_type": asset["media_type"],
                "candidate_scene": "unclassified",
                "review_status": "pending",
                "human_approved": False,
            })
        for asset in person["generated_candidates"]:
            media.append({
                "asset_path": asset["workspace_relative_path"],
                "source_sha256": asset["sha256"],
                "media_type": asset["media_type"],
                "candidate_scene": "daily_base_candidate",
                "review_status": "pending",
                "human_approved": False,
            })
        entries.append({"stable_id": person["stable_id"], "display_name": person["name"], "media": media})
    return {"schema_version": 1, "mapping_version": "portrait-review-v1", "entries": entries}


def validate_review_mapping(value: Any, project_root: Path, asset_hashes: dict[str, str] | None = None, asset_owners: dict[tuple[str, str], set[str]] | None = None, stable_id_names: dict[str, str] | None = None) -> dict:
    if not isinstance(value, dict) or value.get("schema_version") != 1 or value.get("mapping_version") != "portrait-review-v1":
        raise ValueError("unsupported review mapping version")
    entries = value.get("entries")
    if not isinstance(entries, list):
        raise ValueError("review mapping entries must be an array")
    seen: dict[str, str] = {}
    seen_assets: set[tuple[str, str, str]] = set()
    for entry in entries:
        if not isinstance(entry, dict) or not isinstance(entry.get("stable_id"), str) or not entry["stable_id"] or not isinstance(entry.get("display_name"), str) or not isinstance(entry.get("media"), list):
            raise ValueError("each review entry requires a stable ID, name, and media list")
        stable_id = entry["stable_id"]
        name = entry.get("display_name", "")
        if stable_id in seen and seen[stable_id] != name:
            raise ValueError(f"stable ID name conflict: {stable_id}")
        if stable_id_names is not None and stable_id_names.get(stable_id) != name:
            raise ValueError("review entry does not match a known stable ID and name")
        seen[stable_id] = name
        for media in entry.get("media", []):
            if not isinstance(media, dict):
                raise ValueError("media mapping must be an object")
            status = media.get("review_status")
            if status not in {"pending", "approved", "rejected"}:
                raise ValueError("review status must be pending, approved, or rejected")
            raw_path = media.get("asset_path", "")
            if "candidate_scene" not in media:
                raise ValueError("candidate scene is required")
            path = Path(raw_path)
            if not isinstance(raw_path, str) or "\\" in raw_path or not raw_path or path.is_absolute() or any(part in {"", ".", ".."} for part in path.parts):
                raise ValueError("asset paths must be workspace-relative")
            if path.parts and path.parts[0] not in {"legacy", "assets"}:
                raise ValueError("asset path must use an approved media root")
            if path.parts[0] == "legacy" and (len(path.parts) < 3 or path.parts[1] not in {"mother", "h3"}):
                raise ValueError("legacy asset path must use an approved source")
            source_hash = media.get("source_sha256", "")
            if len(source_hash) != 64 or any(char not in "0123456789abcdef" for char in source_hash):
                raise ValueError("asset source hash must be lowercase SHA-256")
            if asset_hashes is not None and asset_hashes.get(path.as_posix()) != source_hash:
                raise ValueError("asset path is unknown or its source hash changed")
            if asset_owners is not None and stable_id not in asset_owners.get((path.as_posix(), source_hash), set()):
                raise ValueError("asset is not associated with this stable ID")
            key = (stable_id, path.as_posix(), source_hash)
            if key in seen_assets:
                raise ValueError("duplicate stable ID and asset mapping")
            seen_assets.add(key)
            scene = media.get("candidate_scene")
            if scene not in {"unclassified", "daily_base_candidate", "travel_candidate", "public_scene_candidate"}:
                raise ValueError("candidate scene is invalid")
            media["review_status"] = status
            media["human_approved"] = status == "approved"
    return value


def export_people_csv(path: Path, people: list[dict]) -> None:
    columns = [
        "stable_id", "name", "country", "role", "level", "author_profile_status", "profile_hash_matches_catalog",
        "author_directory_member", "static_catalog_declared", "runtime_import_status", "asset_audit_status", "asset_status",
        "old_image_count", "old_video_count", "video_only", "unmatched", "identity_conflict",
        "weak_name_candidate_count", "out_of_scope_candidate_count", "generated_candidate_count", "evidence_checked_at",
    ]
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=columns, extrasaction="ignore")
        writer.writeheader()
        for person in people:
            row = dict(person)
            row["generated_candidate_count"] = len(person["generated_candidates"])
            row["weak_name_candidate_count"] = len(person["weak_name_candidates"])
            writer.writerow(row)


def source_fingerprints(project_root: Path) -> dict[str, str]:
    paths = {
        "author_directory": project_root / "设计稿" / "资源清单" / "凡人人界_101人作者目录_20261015.json",
        "static_catalog": project_root / "work" / "luna6-world-content" / "static-world-catalog.json",
        "portrait_asset_audit": project_root / "work" / "luna6-world-content" / "portrait-asset-audit.json",
        "portrait_index": project_root / "设计稿" / "人物画像" / "凡人人界" / "画像索引.json",
    }
    return {label: sha256_bytes(path.read_bytes()) for label, path in paths.items()}
