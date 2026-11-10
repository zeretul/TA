"""Build a local-only NPC portrait inventory and human-review bundle."""

from __future__ import annotations

import argparse
import csv
import json
import shutil
import subprocess
import sys
from datetime import datetime
from pathlib import Path

import default_package
from records import build_initial_mapping, build_people, export_people_csv, source_fingerprints, validate_review_mapping
from review_page import build_review_page
from scan import IMAGE_TYPES, VIDEO_TYPES, safe_output_path, safe_resolve, scan_candidate_images, scan_media_roots, sha256_file, write_if_changed

PROJECT_ROOT = Path(__file__).resolve().parents[2]
WORK_ROOT = PROJECT_ROOT / "work" / "luna6-portrait-assets"
CANDIDATE_ROOT = PROJECT_ROOT / "assets" / "npc-portraits-candidate"
MEDIA_ROOTS = [
    ("mother", Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\头像母图")),
    ("h3", Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\H3生成")),
]

# Captured in one read of each package file before this offline tool was built.
# Do not refresh this snapshot by reading the assigned Claude worktree.
PACKAGE_SNAPSHOT = {
    "capture_date": "2026-10-16",
    "capture_time": "not_recorded",
    "capture_method": "one-time static declaration read; no runtime or Story import test",
    "main": {
        "path": "internal/storypack/builtin.go",
        "sha256": "fae0583aae0c210b9a9c963d4ca5f3438b279906e6fd4f610649d6d5655a4660",
        "bytes": 9495,
        "declarations": ["protagonist:沈青:fanren-jie", "protagonist:陆知白:wumian-zhentan", "actor:袁明远", "actor:沈曼青"],
    },
    "assigned_claude_worktree": {
        "path": ".claude/worktrees/game-only-cont/internal/storypack/builtin.go",
        "sha256": "7e8aa5cea4e727700c2c18a0262b39543affb46908e79c9f6a4c4fcd7a23b1e4",
        "bytes": 9419,
        "declarations": ["protagonist:沈青:fanren-jie", "protagonist:陆知白:wumian-zhentan", "actor:袁明远", "actor:沈曼青"],
    },
    "code_declaration_count": 8,
    "runtime_import_status": "unknown_not_verified",
}


def _json_bytes(value: object) -> bytes:
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode("utf-8")


def _verify_directory_chain(path: Path) -> Path:
    absolute = Path(path.anchor)
    for part in path.parts[1:]:
        absolute = absolute / part
        if absolute.exists() and absolute.is_symlink():
            raise ValueError(f"approved directory chain contains a symbolic link: {path.name}")
        junction_check = getattr(absolute, "is_junction", None)
        if absolute.exists() and junction_check is not None and junction_check():
            raise ValueError(f"approved directory chain contains a junction: {path.name}")
    resolved = path.resolve(strict=True)
    if resolved != path:
        raise ValueError(f"approved directory resolves to a different real path: {path.name}")
    if not resolved.is_dir():
        raise ValueError(f"approved path is not a directory: {path.name}")
    return resolved


def _run_build() -> dict:
    for _, root in MEDIA_ROOTS:
        _verify_directory_chain(root)
    WORK_ROOT.mkdir(parents=True, exist_ok=True)
    _verify_directory_chain(WORK_ROOT)
    candidate_root = CANDIDATE_ROOT
    if candidate_root.exists():
        _verify_directory_chain(candidate_root)
    tools_available = shutil.which("ffmpeg") is not None and shutil.which("ffprobe") is not None
    media, media_diagnostics, duplicate_hashes = scan_media_roots(MEDIA_ROOTS, WORK_ROOT, tools_available)
    generated, generated_diagnostics = scan_candidate_images(candidate_root, WORK_ROOT)
    people, conflicts = build_people(PROJECT_ROOT, media, generated, PACKAGE_SNAPSHOT)
    checked_at = datetime.now().astimezone().isoformat(timespec="seconds")
    for person in people:
        person["evidence_checked_at"] = checked_at
        person["evidence"] = {
            "profile": {"path": person["author_profile_path"], "sha256": person["author_profile_hash"], "status": person["author_profile_status"]},
            "audit_status": person["asset_audit_status"],
            "exact_media": [{"source": a["source"], "relative_path": a["relative_path"], "sha256": a["sha256"], "identity_basis": a["identity_evidence"]} for a in person["old_media"]],
            "filename_candidates": [{"source": a["source"], "relative_path": a["relative_path"], "sha256": a["sha256"]} for a in person["weak_name_candidates"]],
            "reference_diagnostics": person["reference_diagnostics"],
            "out_of_scope_candidates": person["out_of_scope_candidates"],
            "legacy_audit_candidates": person["legacy_audit_candidates"],
            "scope_note": "Only the approved media roots and formats were scanned; absence means no verified match within that scope."
        }
    all_diagnostics = media_diagnostics + generated_diagnostics
    for person in people:
        all_diagnostics.extend(person["reference_diagnostics"])
    old_bound_keys = {(asset["source"], asset["relative_path"]) for person in people for asset in person["old_media"]}
    unassigned = [asset for asset in media if (asset["source"], asset["relative_path"]) not in old_bound_keys]
    payload = {
        "schema_version": 1,
        "generated_at": datetime.now().astimezone().isoformat(timespec="seconds"),
        "scope": "offline_candidate_only_no_production_binding",
        "source_fingerprints": source_fingerprints(PROJECT_ROOT),
        "package_snapshot": PACKAGE_SNAPSHOT,
        "people": people,
        "media": media,
        "generated_media": generated,
        "unassigned_media": unassigned,
        "diagnostics": all_diagnostics,
        "duplicate_hashes": duplicate_hashes,
        "identity_conflicts": conflicts,
        "initial_mapping": build_initial_mapping(people),
        "runtime_import_status": "unknown_not_verified",
    }
    _write_json("package-snapshot.json", PACKAGE_SNAPSHOT)
    _write_json("inventory.json", payload)
    export_people_csv(safe_output_path(WORK_ROOT, "coverage.csv"), people)
    page_payload = dict(payload)
    # The review page resolves thumbnails relative to its own location.
    for asset in media + generated + unassigned:
        for size, thumb in asset.get("thumbnails", {}).items():
            asset["thumbnails"][size] = thumb
    write_if_changed(safe_output_path(WORK_ROOT, "review.html"), build_review_page(payload))
    _write_json("review-export.json", payload["initial_mapping"])
    _write_json("diagnostics.json", {"diagnostics": all_diagnostics, "identity_conflicts": conflicts, "video_poster_tool_available": tools_available})
    return {
        "people": len(people), "catalog": sum(p["static_catalog_declared"] for p in people),
        "media": len(media), "generated": len(generated), "diagnostics": len(all_diagnostics),
        "identity_conflicts": len(conflicts), "unmatched": sum(p["unmatched"] for p in people),
        "no_verified_scoped_daily_portrait": sum(p["asset_status"] == "no_verified_scoped_daily_portrait" for p in people), "runtime_import_status": "unknown_not_verified",
    }


def _write_json(relative: str, value: object) -> None:
    write_if_changed(safe_output_path(WORK_ROOT, relative), _json_bytes(value))


def _run_import(relative: str) -> dict:
    input_path = safe_resolve(WORK_ROOT, relative)
    if input_path.suffix.lower() != ".json":
        raise ValueError("review input must be a JSON file inside the work directory")
    try:
        value = json.loads(input_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"review input is unreadable or invalid JSON: {error.__class__.__name__}") from error
    inventory_path = safe_resolve(WORK_ROOT, "inventory.json")
    inventory = json.loads(inventory_path.read_text(encoding="utf-8"))
    asset_hashes = {
        f"legacy/{asset['source']}/{asset['relative_path']}": asset["sha256"]
        for asset in inventory.get("media", [])
    }
    asset_hashes.update({asset["workspace_relative_path"]: asset["sha256"] for asset in inventory.get("generated_media", [])})
    mapping = inventory.get("initial_mapping", {})
    asset_owners = {}
    for entry in mapping.get("entries", []):
        for media in entry.get("media", []):
            asset_owners.setdefault((media["asset_path"], media["source_sha256"]), set()).add(entry["stable_id"])
    stable_id_names = {person["stable_id"]: person["name"] for person in inventory.get("people", [])}
    validated = validate_review_mapping(value, PROJECT_ROOT, asset_hashes, asset_owners, stable_id_names)
    output = safe_output_path(WORK_ROOT, "review-import.json")
    write_if_changed(output, _json_bytes(validated))
    return {"imported_entries": len(validated["entries"]), "output": "review-import.json", "human_approved_entries": sum(media.get("human_approved", False) for entry in validated["entries"] for media in entry.get("media", []))}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Offline portrait inventory and manual review bundle")
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("build", help="scan fixed approved sources and build review outputs")
    defaults = subparsers.add_parser("export-defaults", help="export the user-authorized portable portrait defaults")
    defaults.add_argument("choices", nargs="?", default="default-choices.json", help="choices file relative to work/luna6-portrait-assets")
    subparsers.add_parser("verify-defaults", help="verify the portable default portrait package")
    importer = subparsers.add_parser("import-review", help="validate and copy a review JSON from the work directory")
    importer.add_argument("relative_path", help="path relative to work/luna6-portrait-assets")
    arguments = parser.parse_args(argv)
    try:
        if arguments.command == "build":
            result = _run_build()
        elif arguments.command == "import-review":
            result = _run_import(arguments.relative_path)
        elif arguments.command == "export-defaults":
            result = default_package.export_defaults(PROJECT_ROOT, arguments.choices)
        else:
            result = default_package.verify_defaults(PROJECT_ROOT)
    except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError) as error:
        print(json.dumps({"error": error.__class__.__name__, "message": str(error)}, ensure_ascii=False), file=sys.stderr)
        return 2
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
