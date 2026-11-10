"""Export and verify a portable, user-authorized default portrait package."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

from PIL import Image, ImageOps

from scan import is_linklike, safe_output_path, safe_resolve, sha256_file, write_if_changed

SCHEMA_VERSION = 1
CHOICES_VERSION = "portrait-default-choices-v1"
MANIFEST_VERSION = "portrait-defaults-v1"
DEFAULT_ROOT = Path("assets/npc-portraits-default")
WORK_ROOT = Path("work/luna6-portrait-assets")
INVENTORY_PATH = "work/luna6-portrait-assets/inventory.json"
AUTHOR_DIRECTORY_PATH = "设计稿/资源清单/凡人人界_101人作者目录_20261015.json"
PORTRAIT_AUDIT_PATH = "work/luna6-world-content/portrait-asset-audit.json"
LEGACY_PROJECT_ROOT = Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇")
LEGACY_ROOTS = {
    "mother": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\头像母图"),
    "h3": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\H3生成"),
}


def _json_bytes(value: object) -> bytes:
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode("utf-8")


def _external_project_root() -> Path:
    current = Path(LEGACY_PROJECT_ROOT.anchor)
    for part in LEGACY_PROJECT_ROOT.parts[1:]:
        current /= part
        if current.exists() and is_linklike(current):
            raise ValueError("legacy project path contains a symbolic link or junction")
    resolved = LEGACY_PROJECT_ROOT.resolve(strict=True)
    if not resolved.is_dir():
        raise ValueError("legacy project path is not a directory")
    return resolved


def _load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{path.name} must contain a JSON object")
    return value


def _read_source(project_root: Path, person: dict, choice: dict) -> tuple[Path, dict, dict | None]:
    source = choice.get("source")
    if not isinstance(source, dict):
        raise ValueError("each default choice must declare one source")
    kind = source.get("kind")
    if kind == "legacy":
        source_name = source.get("source")
        relative = source.get("relative_path")
        if source_name not in LEGACY_ROOTS or not isinstance(relative, str):
            raise ValueError("legacy source must use an approved root and relative path")
        matching = [asset for asset in person.get("old_media", []) if asset.get("source") == source_name and asset.get("relative_path") == relative and asset.get("media_type", "").startswith("image/")]
        if len(matching) != 1:
            raise ValueError(f"legacy image is not uniquely associated with {person.get('stable_id')}")
        asset = matching[0]
        if asset.get("sha256") != source.get("sha256"):
            raise ValueError("legacy source hash differs from the scoped inventory")
        root = LEGACY_ROOTS[source_name]
        path = safe_resolve(root, relative)
        provenance = {"kind": "legacy", "source": source_name, "relative_path": relative.replace("\\", "/"), "sha256": asset["sha256"]}
        return path, provenance, None
    if kind == "candidate":
        relative = source.get("project_relative_path")
        if not isinstance(relative, str) or not relative.startswith("assets/npc-portraits-candidate/"):
            raise ValueError("generated source must be inside the approved candidate directory")
        if "\\" in relative or any(part in {"", ".", ".."} for part in relative.split("/")):
            raise ValueError("generated source must be a normalized project-relative path without traversal")
        generated = [asset for asset in person.get("generated_candidates", []) if asset.get("workspace_relative_path") == relative]
        if len(generated) != 1 or generated[0].get("sha256") != source.get("sha256"):
            raise ValueError(f"generated image is not uniquely associated with {person.get('stable_id')}")
        path = safe_resolve(project_root, relative)
        metadata_relative = source.get("metadata_project_relative_path")
        metadata = None
        if metadata_relative is not None:
            if not isinstance(metadata_relative, str) or not metadata_relative.startswith("assets/npc-portraits-candidate/") or not metadata_relative.endswith("/metadata.json"):
                raise ValueError("candidate metadata must stay inside the approved candidate directory")
            if "\\" in metadata_relative or any(part in {"", ".", ".."} for part in metadata_relative.split("/")):
                raise ValueError("candidate metadata path must be normalized and cannot traverse")
            metadata_path = safe_resolve(project_root, metadata_relative)
            metadata = _load_json(metadata_path)
            expected_profile = person.get("author_profile_hash")
            recorded_profile = metadata.get("source_profile", {}).get("sha256")
            if not isinstance(recorded_profile, str) or recorded_profile.lower() != expected_profile.lower():
                raise ValueError("candidate metadata profile hash does not match the formal author profile")
            recorded_asset = metadata.get("asset_sha256")
            if not isinstance(recorded_asset, str) or recorded_asset.lower() != source.get("sha256", "").lower():
                raise ValueError("candidate metadata image hash does not match the selected candidate")
        return path, {"kind": "generated_candidate", "project_relative_path": relative, "sha256": source["sha256"]}, metadata
    if kind in {"audit_primary", "audit_shared_template"}:
        stable_id = person.get("stable_id")
        if source.get("stable_id") != stable_id:
            raise ValueError("legacy portrait audit identity does not match the selected person")
        audit = _load_json(safe_resolve(project_root, PORTRAIT_AUDIT_PATH))
        rows = [row for row in audit.get("legacy_npc_objects", []) if row.get("legacy_npc_id") == stable_id]
        if len(rows) != 1 or rows[0].get("name") != person.get("name"):
            raise ValueError("legacy portrait audit does not confirm an exact identity and display name")
        row = rows[0]
        relative = row.get("primary_image")
        if not isinstance(relative, str) or "\\" in relative or any(part in {"", ".", ".."} for part in relative.split("/")):
            raise ValueError("legacy primary image path is invalid")
        if kind == "audit_primary":
            expected = f"frontend/assets/portraits/{stable_id}.png"
            if row.get("shared_template") or row.get("portrait_key") != stable_id or relative != expected:
                raise ValueError("legacy audit does not declare a person-specific primary portrait")
        else:
            if not row.get("shared_template") or choice.get("shared_template") is not True or not relative.startswith("frontend/assets/portraits/job_"):
                raise ValueError("legacy audit does not declare an applicable shared occupation template")
        source_root = _external_project_root()
        path = safe_resolve(source_root, relative)
        digest = source.get("sha256")
        if not isinstance(digest, str) or sha256_file(path) != digest:
            raise ValueError("legacy primary image hash does not match the recorded source hash")
        return path, {"kind": "legacy_primary" if kind == "audit_primary" else "legacy_shared_template", "audit_path": PORTRAIT_AUDIT_PATH, "legacy_project_relative_path": relative, "sha256": digest}, None
    raise ValueError("default source kind must be legacy or candidate")


def _decode_and_copy(source: Path, destination: Path) -> tuple[str, list[int]]:
    with Image.open(source) as image:
        image.verify()
    with Image.open(source) as image:
        if image.width < 1 or image.height < 1:
            raise ValueError("source image has invalid dimensions")
        clean = ImageOps.exif_transpose(image)
        if clean.mode not in {"RGB", "RGBA"}:
            clean = clean.convert("RGBA" if "transparency" in clean.info else "RGB")
        clean.save(destination, format="PNG", optimize=True)
        return sha256_file(destination), [image.width, image.height]


def _write_thumbnails(master: Path, output_root: Path, stable_id: str) -> dict[str, dict[str, Any]]:
    results = {}
    with Image.open(master) as image:
        for size in (128, 192):
            relative = f"{stable_id}/portrait-{size}.png"
            target = safe_output_path(output_root, relative)
            thumb = ImageOps.fit(image.convert("RGB"), (size, size), method=Image.Resampling.LANCZOS, centering=(0.5, 0.42))
            thumb.save(target, format="PNG", optimize=True)
            results[str(size)] = {"path": f"{DEFAULT_ROOT.as_posix()}/{relative}", "sha256": sha256_file(target), "dimensions": [size, size]}
    return results


def _validate_choices(inventory: dict, choices: dict) -> dict[str, dict]:
    if choices.get("schema_version") != SCHEMA_VERSION or choices.get("mapping_version") != CHOICES_VERSION:
        raise ValueError("default choice schema or version is invalid")
    rows = choices.get("choices")
    if not isinstance(rows, list):
        raise ValueError("default choices must be a list")
    people = {person["stable_id"]: person for person in inventory.get("people", []) if person.get("author_directory_member")}
    if len(people) != 101:
        raise ValueError("inventory must contain the complete 101-person author directory")
    by_id = {}
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("every default choice must be an object")
        stable_id = row.get("stable_id")
        if stable_id not in people or stable_id in by_id:
            raise ValueError("default choices contain an unknown or duplicate stable ID")
        person = people[stable_id]
        if row.get("display_name") != person.get("name"):
            raise ValueError(f"display name mismatch for {stable_id}")
        by_id[stable_id] = row
    missing = set(people) - set(by_id)
    if missing or len(rows) != 101:
        raise ValueError(f"default choices must cover each author exactly once; missing={len(missing)}")
    return by_id


def export_defaults(project_root: Path, choices_relative: str) -> dict[str, Any]:
    project_root = project_root.resolve(strict=True)
    work_root = safe_resolve(project_root, WORK_ROOT.as_posix())
    inventory = _load_json(safe_resolve(project_root, INVENTORY_PATH))
    choices = _load_json(safe_resolve(work_root, choices_relative))
    by_id = _validate_choices(inventory, choices)
    output_root = safe_resolve(project_root, DEFAULT_ROOT.as_posix(), must_exist=False)
    output_root.mkdir(parents=True, exist_ok=True)
    entries = []
    people = {person["stable_id"]: person for person in inventory["people"] if person.get("author_directory_member")}
    for stable_id in sorted(people):
        person = people[stable_id]
        choice = by_id[stable_id]
        if choice.get("selection_authority") != "user_authorized_default_selection":
            raise ValueError(f"selection authority is missing for {stable_id}")
        if choice.get("user_human_review") != "not_reviewed" or choice.get("game_install_status") != "not_installed_not_verified":
            raise ValueError(f"review and runtime status must remain distinct for {stable_id}")
        visual_note = choice.get("model_visual_inspection")
        if not isinstance(visual_note, str) or not visual_note.strip():
            raise ValueError(f"a real image inspection note is required for {stable_id}")
        source_path, provenance, metadata = _read_source(project_root, person, choice)
        expected_hash = choice["source"].get("sha256")
        if sha256_file(source_path) != expected_hash:
            raise ValueError(f"source image hash mismatch for {stable_id}")
        author_profile_rel = f"设计稿/{person['author_profile_path']}".replace("\\", "/")
        profile_path = safe_resolve(project_root, author_profile_rel)
        profile_hash = sha256_file(profile_path)
        if profile_hash != person.get("author_profile_hash") or profile_hash != person.get("catalog_profile_hash"):
            raise ValueError(f"author profile hash mismatch for {stable_id}")
        master_relative = f"{stable_id}/portrait.png"
        master = safe_output_path(output_root, master_relative)
        packaged_hash, dimensions = _decode_and_copy(source_path, master)
        thumbnails = _write_thumbnails(master, output_root, stable_id)
        entry = {
            "stable_id": stable_id,
            "display_name": person["name"],
            "author_profile": {"path": author_profile_rel, "sha256": profile_hash},
            "portrait": {"path": f"{DEFAULT_ROOT.as_posix()}/{master_relative}", "sha256": packaged_hash, "dimensions": dimensions, "media_type": "image/png"},
            "thumbnails": thumbnails,
            "selection_authority": "user_authorized_default_selection",
            "selection_basis": choice.get("selection_basis", ""),
            "user_human_review": "not_reviewed",
            "model_visual_inspection": visual_note,
            "game_install_status": "not_installed_not_verified",
            "source": provenance,
        }
        if metadata:
            entry["generation_provenance"] = {
                "metadata_source": choice["source"].get("metadata_project_relative_path"),
                "prompt": metadata.get("generation_prompt"),
                "reference_images": metadata.get("reference_images", []),
                "visual_supplements": metadata.get("fact_vs_visual_supplement", {}).get("visual_supplements", ""),
            }
        entries.append(entry)
    author_directory = safe_resolve(project_root, AUTHOR_DIRECTORY_PATH)
    manifest = {
        "schema_version": SCHEMA_VERSION,
        "manifest_version": MANIFEST_VERSION,
        "scope": "portable_default_portraits_not_installed_in_game",
        "author_directory": {"path": AUTHOR_DIRECTORY_PATH, "sha256": sha256_file(author_directory)},
        "entries": entries,
    }
    write_if_changed(safe_output_path(output_root, "manifest.json"), _json_bytes(manifest))
    return {"entries": len(entries), "generated": sum(item["source"]["kind"] == "generated_candidate" for item in entries), "legacy": sum(item["source"]["kind"] == "legacy" for item in entries), "output": DEFAULT_ROOT.as_posix()}


def verify_defaults(project_root: Path) -> dict[str, Any]:
    project_root = project_root.resolve(strict=True)
    root = safe_resolve(project_root, DEFAULT_ROOT.as_posix())
    manifest = _load_json(safe_resolve(root, "manifest.json"))
    if manifest.get("schema_version") != SCHEMA_VERSION or manifest.get("manifest_version") != MANIFEST_VERSION:
        raise ValueError("default portrait manifest version is invalid")
    entries = manifest.get("entries")
    if not isinstance(entries, list) or len(entries) != 101:
        raise ValueError("default portrait manifest must contain exactly 101 entries")
    ids = [entry.get("stable_id") for entry in entries]
    if len(set(ids)) != 101:
        raise ValueError("default portrait stable IDs must be unique")
    inventory = _load_json(safe_resolve(project_root, INVENTORY_PATH))
    people = {person["stable_id"]: person for person in inventory.get("people", []) if person.get("author_directory_member")}
    if set(ids) != set(people):
        raise ValueError("default portrait IDs do not match the formal author directory")
    checked_files = set()
    for entry in entries:
        stable_id = entry["stable_id"]
        if entry.get("selection_authority") != "user_authorized_default_selection" or entry.get("user_human_review") != "not_reviewed" or entry.get("game_install_status") != "not_installed_not_verified":
            raise ValueError(f"authorization, human review, or install status is invalid for {stable_id}")
        profile = entry.get("author_profile", {})
        if not profile.get("path", "").startswith("设计稿/") or \
                sha256_file(safe_resolve(project_root, profile["path"])) != profile.get("sha256"):
            raise ValueError(f"author profile path or hash mismatch for {stable_id}")
        for asset in [entry.get("portrait", {}), *entry.get("thumbnails", {}).values()]:
            relative = asset.get("path", "")
            prefix = f"{DEFAULT_ROOT.as_posix()}/{stable_id}/"
            if not relative.startswith(prefix):
                raise ValueError(f"portrait path escapes its stable-ID directory for {stable_id}")
            path = safe_resolve(project_root, relative)
            if sha256_file(path) != asset.get("sha256"):
                raise ValueError(f"portrait hash mismatch for {stable_id}: {relative}")
            with Image.open(path) as image:
                image.verify()
            checked_files.add(path.relative_to(root).as_posix())
    actual_files = {path.relative_to(root).as_posix() for path in root.rglob("*") if path.is_file()}
    if actual_files != checked_files | {"manifest.json"}:
        raise ValueError("default portrait package contains missing or orphan files")
    return {"entries": len(entries), "files": len(actual_files), "status": "verified", "output": DEFAULT_ROOT.as_posix()}
