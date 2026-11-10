#!/usr/bin/env python3
"""Build reproducible, non-installed world-content indexes and audit source files."""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from pathlib import Path, PurePosixPath
from pathlib import PureWindowsPath
import sys
from collections import defaultdict
from asset_audit import build_portrait_audit


ROOT = Path(".")
OUTPUT = ROOT / "work/luna6-world-content"
ASSETS = ROOT / "设计稿/资源清单"
EVENTS = ROOT / "设计稿/交付/LUNA6_世界内容与资源_20261015/六条公共事件作者卡.md"
SOURCE_FILES = {
    "profiles": "设计稿/资源清单/凡人人界_101人作者目录_20261015.json",
    "discovery": "设计稿/资源清单/凡人人界_人物发现索引草案_20261015.json",
    "relations": "设计稿/资源清单/凡人人界_世界检索关联草案_20261015.json",
    "handoff": "设计稿/资源清单/凡人人界_首版衔接清单_20261015.json",
    "event_library": "设计稿/资源清单/凡人人界_修仙事件素材库_20261015.json",
    "abilities": "设计稿/资源清单/凡人人界_功法技能条目草案_20261015.json",
    "portrait_bindings": "设计稿/资源清单/头像资产候选绑定_20261015.json",
    "portrait_media": "设计稿/资源清单/头像视频元数据与去重_20261015.json",
    "legacy_portraits": "设计稿/资源清单/旧库NPC头像匹配清单_20261015.csv",
    "denova_portraits": "设计稿/资源清单/DENOVA静态人物匹配清单_20261015.csv",
    "baseline": "设计稿/首版基线/凡人人界_作者基线1.0_20261015/清单.json",
}
EVENT_SPECS = [
    ("C01", "血色试炼", "canon", "fanren-public-canon", "conditional_candidate", ["exploration", "competition", "rescue"]),
    ("C02", "燕家相关事变", "canon", "fanren-public-canon", "parallel_author_scenario", ["family", "investigation", "negotiation"]),
    ("C03", "魔道入侵相关公共事件", "canon", "fanren-public-canon", "conditional_candidate", ["border", "intelligence", "evacuation"]),
    ("O01", "太岳灵潮", "original", "fanren-public-original", "parallel_author_scenario", ["exploration", "environment"]),
    ("O02", "越元交流会", "original", "fanren-public-original", "parallel_author_scenario", ["social", "learning", "exchange"]),
    ("O03", "边境失踪案", "original", "fanren-public-original", "parallel_author_scenario", ["investigation", "rescue", "border"]),
]
MEDIA_TYPES = {".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif", ".mp4": "video/mp4"}
KNOWN_DOMAINS = {"core", "items", "learning", "cultivation", "combat", "exploration", "assessment", "beast_domain", "formation_domain", "craft_domain", "healing_domain", "soul_domain", "later_domain"}


def read_json(path: Path):
    with path.open("r", encoding="utf-8") as stream:
        return json.load(stream)


def digest(path: Path) -> str:
    sha = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            sha.update(block)
    return sha.hexdigest()


def rel(path: Path) -> str:
    value = path.relative_to(ROOT).as_posix()
    if "\\" in value or value.startswith("/") or PurePosixPath(value).is_absolute():
        raise ValueError(f"Non-portable repository path: {value}")
    return value


def dump(path: Path, value) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n")


def load_sources(root: Path = ROOT):
    values, errors = {}, []
    for key, filename in SOURCE_FILES.items():
        path = root / filename
        try:
            if path.suffix == ".json":
                values[key] = (path, read_json(path))
            else:
                with path.open("r", encoding="utf-8-sig", newline="") as stream:
                    values[key] = (path, list(csv.DictReader(stream)))
        except (OSError, UnicodeError, json.JSONDecodeError) as error:
            errors.append({"source": filename, "error": str(error)})
    return values, errors


def _document(sources, key, default, issues):
    item = sources.get(key)
    if not item:
        return default
    value = item[1]
    if not isinstance(value, type(default)):
        issues.append({"severity": "error", "code": "invalid_source_type", "source": key, "expected": type(default).__name__})
        return default
    return value


def _rows(document, key, source_name, issues):
    value = document.get(key, []) if isinstance(document, dict) else []
    if not isinstance(value, list) or any(not isinstance(row, dict) for row in value):
        issues.append({"severity": "error", "code": "invalid_collection_type", "source": source_name, "collection": key, "expected": "array_of_objects"})
        return []
    return value


def _index_rows(rows, collection, key_field, issues, *, severity="error"):
    index = defaultdict(list)
    for position, row in enumerate(rows):
        key = row.get(key_field)
        if not isinstance(key, str) or not key:
            issues.append({"severity": "error", "code": "missing_stable_id", "collection": collection, "position": position})
            continue
        index[key].append(row)
    for key, matches in sorted(index.items()):
        if len(matches) > 1:
            issues.append({"severity": severity, "code": "duplicate_stable_id", "collection": collection, "id": key, "count": len(matches)})
    return index


def _safe_profile(root, ref):
    if not isinstance(ref, str) or not ref:
        return None
    portable = PurePosixPath(ref)
    windows = PureWindowsPath(ref)
    if portable.is_absolute() or windows.is_absolute() or windows.drive or ".." in portable.parts or "\\" in ref or ":" in ref:
        return None
    relative = PurePosixPath("设计稿") / portable
    path = root.joinpath(*relative.parts)
    try:
        path.relative_to(root)
    except ValueError:
        return None
    if not is_regular_path_without_links(root, path, allow_file=True):
        return None
    return relative, path


def is_link_or_junction(path: Path) -> bool:
    if path.is_symlink():
        return True
    is_junction = getattr(path, "is_junction", None)
    if callable(is_junction) and is_junction():
        return True
    try:
        attributes = path.lstat().st_file_attributes
    except (OSError, AttributeError):
        return False
    return bool(attributes & getattr(__import__("stat"), "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))


def is_regular_path_without_links(root: Path, target: Path, *, allow_file: bool) -> bool:
    try:
        relative = target.relative_to(root)
    except ValueError:
        return False
    if not relative.parts or any(part in {".", ".."} for part in relative.parts):
        return False
    cursor = root
    if is_link_or_junction(cursor) or not cursor.is_dir():
        return False
    for index, part in enumerate(relative.parts):
        cursor = cursor / part
        if is_link_or_junction(cursor):
            return False
        is_last = index == len(relative.parts) - 1
        if is_last and allow_file:
            return cursor.is_file()
        if not cursor.is_dir():
            return False
    return False


def build_catalog(sources, source_errors, root: Path | None = None):
    root = root or ROOT
    issues = [{"severity": "error", "code": "source_read_error", **error} for error in source_errors]
    people = _document(sources, "profiles", {}, issues)
    discovery = _document(sources, "discovery", {}, issues)
    relations = _document(sources, "relations", {}, issues)
    handoff = _document(sources, "handoff", {}, issues)
    eventlib = _document(sources, "event_library", {}, issues)
    abilities = _document(sources, "abilities", {}, issues)
    profile_rows = _rows(people, "profiles", "profiles", issues)
    discovery_rows = _rows(discovery, "characters", "discovery", issues)
    initial_rows = _rows(handoff, "initial_characters", "handoff", issues)
    factions = _rows(relations, "factions", "relations", issues)
    locations = _rows(relations, "locations", "relations", issues)
    places = _rows(handoff, "places", "handoff", issues)
    routes = _rows(handoff, "routes", "handoff", issues)
    categories = _rows(eventlib, "categories", "event_library", issues)
    cards = _rows(eventlib, "cards", "event_library", issues)

    profile_index = _index_rows(profile_rows, "profiles", "source_key", issues)
    discovery_index = _index_rows(discovery_rows, "discovery", "source_key", issues)
    initial_index = _index_rows(initial_rows, "initial_characters", "source_key", issues)
    faction_index = _index_rows(factions, "factions", "id", issues)
    location_index = _index_rows(locations, "locations", "id", issues)
    place_index = _index_rows(places, "places", "id", issues)
    route_index = _index_rows(routes, "routes", "id", issues)
    category_index = _index_rows(categories, "event_categories", "id", issues)
    card_index = _index_rows(cards, "event_cards", "id", issues)
    profile_keys = set(profile_index)
    for collection, index, parent in (("discovery", discovery_index, profile_keys), ("initial_characters", initial_index, profile_keys)):
        missing = sorted(profile_keys - set(index))
        orphaned = sorted(set(index) - profile_keys)
        for key in missing:
            issues.append({"severity": "error", "code": f"missing_{collection}_record", "source_key": key})
        for key in orphaned:
            issues.append({"severity": "error", "code": f"orphan_{collection}_record", "source_key": key})

    name_sources = defaultdict(list)
    characters = []
    valid_location_ids = set(location_index) | set(place_index)
    faction_ids = set(faction_index)
    for key, matches in sorted(profile_index.items()):
        if len(matches) != 1:
            continue
        profile = matches[0]
        discovery_matches = discovery_index.get(key, [])
        initial_matches = initial_index.get(key, [])
        d = discovery_matches[0] if len(discovery_matches) == 1 else {}
        h = initial_matches[0] if len(initial_matches) == 1 else {}
        ref_result = _safe_profile(root, profile.get("path"))
        if ref_result is None:
            issues.append({"severity": "error", "code": "missing_or_unsafe_profile_ref", "source_key": key, "profile": profile.get("path")})
            profile_ref, profile_hash = profile.get("path"), None
        else:
            relative, actual = ref_result
            profile_ref = relative.as_posix()
            try:
                profile_hash = digest(actual)
            except OSError as error:
                profile_hash = None
                issues.append({"severity": "error", "code": "profile_read_error", "source_key": key, "profile": profile_ref, "error": str(error)})
        name = profile.get("name")
        if not isinstance(name, str) or not name:
            issues.append({"severity": "error", "code": "missing_profile_name", "source_key": key})
            name = ""
        else:
            name_sources[name].append(key)
        if not isinstance(profile.get("country"), str) or profile.get("country") not in {"越国", "元武国", "紫金国"}:
            issues.append({"severity": "error", "code": "unexpected_country", "source_key": key, "country": profile.get("country")})
        faction_id = d.get("faction_id")
        if faction_id and (not isinstance(faction_id, str) or faction_id not in faction_ids):
            issues.append({"severity": "error", "code": "unresolved_faction", "source_key": key, "faction_id": faction_id})
        place_id = d.get("initial_place_candidate")
        if place_id and (not isinstance(place_id, str) or place_id not in valid_location_ids):
            issues.append({"severity": "error", "code": "unresolved_initial_place_candidate", "source_key": key, "place_id": place_id})
        if not d:
            issues.append({"severity": "error", "code": "missing_discovery_entry", "source_key": key})
        if not h:
            issues.append({"severity": "error", "code": "missing_initial_character", "source_key": key})
        characters.append({
            "source_key": key, "name": name, "country": profile.get("country"), "faction_id": faction_id,
            "role": profile.get("role"), "initial_place_candidate": place_id,
            "initial_place_label": d.get("initial_place_label"), "profile_ref": profile_ref,
            "profile_sha256": profile_hash, "initial_stage": h.get("initial_stage", profile.get("initial_stage")),
            "disclosure": d.get("disclosure", "author_catalog_only_not_player_or_npc_knowledge"),
            "contact_gate": d.get("contact_gate"), "runtime_imported": False,
        })
    for name, keys in sorted(name_sources.items()):
        if len(keys) > 1:
            issues.append({"severity": "warning", "code": "same_name_kept_as_separate_source_keys", "name": name, "source_keys": sorted(keys)})

    for collection_name, rows in (("factions", factions), ("locations", locations), ("places", places)):
        for row in rows:
            refs = row.get("character_refs", [])
            if not isinstance(refs, list) or any(not isinstance(value, str) for value in refs):
                issues.append({"severity": "error", "code": "invalid_character_refs", "collection": collection_name, "id": row.get("id")})
                continue
            unknown = sorted(set(refs) - profile_keys)
            if unknown:
                issues.append({"severity": "error", "code": "unresolved_character_refs", "collection": collection_name, "id": row.get("id"), "refs": unknown})
    for place in places:
        parent = place.get("parent")
        if parent and (not isinstance(parent, str) or parent not in place_index and parent not in location_index and parent not in faction_index):
            issues.append({"severity": "error", "code": "unresolved_place_parent", "place_id": place.get("id"), "parent": parent})
    for initial in initial_rows:
        place_id = initial.get("initial_place")
        if place_id and (not isinstance(place_id, str) or place_id not in valid_location_ids):
            issues.append({"severity": "error", "code": "unresolved_initial_place", "source_key": initial.get("source_key"), "place_id": place_id})
    route_node_ids = set(place_index) | set(location_index)
    for route in routes:
        for endpoint in ("from", "to"):
            value = route.get(endpoint)
            if not isinstance(value, str) or value not in route_node_ids:
                code = "faction_used_as_route_place" if isinstance(value, str) and value in faction_ids else "unresolved_route_endpoint"
                issues.append({"severity": "error", "code": code, "route_id": route.get("id"), "endpoint": endpoint, "value": value})

    category_package = {}
    for category in categories:
        package_id = category.get("package_id")
        category_id = category.get("id")
        if not isinstance(package_id, str) or not package_id:
            issues.append({"severity": "error", "code": "missing_event_category_package", "category_id": category.get("id")})
        elif isinstance(category_id, str):
            category_package[category_id] = package_id
    for card in cards:
        card_id = card.get("id")
        category_id = card.get("category_id")
        if not isinstance(category_id, str) or category_id not in category_index:
            issues.append({"severity": "error", "code": "unknown_event_category", "event_id": card_id, "category_id": category_id})
        elif card.get("package_id") != category_package.get(category_id):
            issues.append({"severity": "error", "code": "event_package_category_mismatch", "event_id": card_id, "package_id": card.get("package_id"), "category_package_id": category_package.get(category_id)})
        domains = card.get("required_domains")
        if not isinstance(domains, list) or not domains:
            issues.append({"severity": "error", "code": "missing_event_domains", "event_id": card_id})
        else:
            for domain in domains:
                contracts = eventlib.get("domain_contracts")
                if not isinstance(domain, str) or domain not in KNOWN_DOMAINS or not isinstance(contracts, dict) or domain not in contracts:
                    issues.append({"severity": "error", "code": "unknown_event_domain", "event_id": card_id, "domain": domain})
        for field in ("type_name", "description_markdown", "trigger", "fusion", "logic", "payoff", "reward_cost", "guardrail"):
            if not isinstance(card.get(field), str) or not card[field].strip():
                issues.append({"severity": "error", "code": "missing_event_content_field", "event_id": card_id, "field": field})

    ability_methods = _rows(abilities, "methods", "abilities", issues)
    ability_skills = _rows(abilities, "skills", "abilities", issues)
    ability_frames = _rows(abilities, "mechanical_frames", "abilities", issues)
    method_ids = {row.get("id") for row in ability_methods if isinstance(row.get("id"), str)}
    skill_ids = {row.get("id") for row in ability_skills if isinstance(row.get("id"), str)}
    frame_ids = {row.get("id") for row in ability_frames if isinstance(row.get("id"), str)}
    for method in ability_methods:
        skill_refs = method.get("learnable_skill_refs", [])
        if not isinstance(skill_refs, list):
            issues.append({"severity": "error", "code": "invalid_method_skill_refs", "method_id": method.get("id")})
            continue
        for skill_id in skill_refs:
            if skill_id not in skill_ids:
                issues.append({"severity": "error", "code": "unresolved_method_skill_ref", "method_id": method.get("id"), "skill_id": skill_id})
    for skill in ability_skills:
        frame_id = skill.get("mechanics_ref")
        if frame_id and (not isinstance(frame_id, str) or frame_id not in frame_ids):
            issues.append({"severity": "error", "code": "unresolved_skill_mechanics_ref", "skill_id": skill.get("id"), "mechanics_ref": frame_id})
    for collection, rows, allowed in (("methods", ability_methods, method_ids), ("skills", ability_skills, skill_ids)):
        _index_rows(rows, collection, "id", issues)

    source_manifest = []
    for key, (path, document) in sources.items():
        declared_version = None
        if isinstance(document, dict):
            declared_version = next((document[field] for field in ("content_version", "author_version", "draft_contract_version", "author_contract_version", "date", "author_baseline_id") if document.get(field) is not None), None)
        try:
            source_manifest.append({"id": key, "path": rel(path), "declared_version": declared_version, "sha256": digest(path), "format": "json" if path.suffix == ".json" else "csv"})
        except (OSError, ValueError) as error:
            issues.append({"severity": "error", "code": "source_hash_error", "source": key, "error": str(error)})
    counts = {"profiles": len(characters), "factions": len(factions), "world_relation_locations": len(locations), "handoff_places": len(places), "routes": len(routes), "event_cards": len(cards), "first_scope_event_templates": (eventlib.get("counts") or {}).get("first_scope_templates"), "later_reserve_templates": (eventlib.get("counts") or {}).get("later_reserve_templates"), "ability_definitions": {"methods": len(ability_methods), "skills": len(ability_skills), "traits": len(_rows(abilities, "traits", "abilities", issues))}}
    catalog = {
        "kind": "derived_static_world_catalog_not_runtime_state", "content_version": "1.1.0",
        "era": eventlib.get("era"), "countries": relations.get("countries", []), "counts": counts,
        "characters": sorted(characters, key=lambda item: item["source_key"]),
        "factions": sorted(factions, key=lambda item: item.get("id", "")),
        "locations": sorted(locations, key=lambda item: item.get("id", "")),
        "places": sorted(places, key=lambda item: item.get("id", "")),
        "routes": sorted(routes, key=lambda item: item.get("id", "")),
        "event_categories": sorted(categories, key=lambda item: item.get("id", "")),
        "event_cards": sorted(cards, key=lambda item: item.get("id", "")),
        "source_manifest": sorted(source_manifest, key=lambda item: item["id"]),
        "identity_rule": "source_key is stable; display names and absolute paths are not identities",
        "knowledge_rule": "catalog membership, profile existence, and candidate location do not imply player/NPC awareness, encounter, familiarity, or current presence",
        "runtime_imported": False,
    }
    return catalog, issues


def parse_public_author_cards(path: Path = EVENTS):
    text = path.read_text(encoding="utf-8")
    matches = list(__import__("re").finditer(r"(?m)^## (C01|C02|C03|O01|O02|O03)\s+(.+)$", text))
    cards = {}
    for index, match in enumerate(matches):
        end = matches[index + 1].start() if index + 1 < len(matches) else len(text)
        body = text[match.end():end].strip()
        cards[match.group(1)] = {"id": match.group(1), "title": match.group(2).strip(), "body_markdown": body}
    expected = {item[0] for item in EVENT_SPECS}
    if set(cards) != expected:
        raise ValueError(f"Public event author cards mismatch: expected={sorted(expected)}, actual={sorted(cards)}")
    source_hash = digest(path)
    for card in cards.values():
        card["source_ref"] = rel(path) + "#" + card["id"]
        card["source_sha256"] = source_hash
        card["body_sha256"] = hashlib.sha256(card["body_markdown"].encode("utf-8")).hexdigest()
        card["character_count"] = len(card["body_markdown"])
    return cards


def build_event_packages(public_cards):
    packages = []
    for package_id in ("fanren-public-canon", "fanren-public-original"):
        events = []
        for code, _, origin, group, state, tags in EVENT_SPECS:
            if group != package_id:
                continue
            author = public_cards.get(code, {"title": next(title for item_code, title, *_ in EVENT_SPECS if item_code == code), "source_ref": rel(EVENTS) + "#" + code, "body_sha256": None})
            events.append({
                "id": code.lower(), "type_name": author["title"],
                "description_markdown": f"Disabled author projection. Full author card: {author['source_ref']}. Projection is incomplete and must remain disabled until all author fields have a verified consumer.",
                "enabled": False, "category": "conditional_candidate",
                "tags": [origin, state, *tags, f"author-sha256:{author['body_sha256']}"], "intensity": "medium",
            })
        packages.append({"version": 1, "id": package_id, "name": "Yue public-event candidates", "description": "Disabled schema projection only; full author cards are outside this runtime schema and are not installed.", "events": events, "custom": True})
    return packages


def build_author_staging(event_library, public_cards, asset_bindings):
    cards = _rows(event_library, "cards", "event_library", [])
    return {
        "kind": "author_staging_resources_not_installed_not_runtime_state",
        "content_version": "1.1.0",
        "runtime_installed": False,
        "primary_source_ref": rel(ROOT / SOURCE_FILES["event_library"]),
        "primary_source_sha256": digest(ROOT / SOURCE_FILES["event_library"]) if (ROOT / SOURCE_FILES["event_library"]).is_file() else None,
        "primary_author_cards": cards,
        "public_event_author_cards": [public_cards[code] for code, *_ in EVENT_SPECS if code in public_cards],
        "event_package_projection": {
            "status": "disabled_incomplete_schema_projection",
            "lossless": False,
            "projection_ids": [code.lower() for code, *_ in EVENT_SPECS],
            "lost_runtime_contracts": ["trigger and eligibility", "audience and knowledge scopes", "phase transitions", "capability bindings", "reward source and budget", "instance identity and idempotency"],
            "enable_allowed": False,
        },
        "portrait_assets": asset_bindings,
    }


def scan_assets(perform_scan, sources):
    roots = {
        "mother": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\头像母图"),
        "h3": Path(r"E:\游戏\游戏开发\凡人修仙传·人界篇\H3生成"),
    }
    return build_portrait_audit(
        perform_scan=perform_scan,
        sources=sources,
        roots=roots,
        digest_function=digest,
        project_relative_path=lambda path: path.relative_to(ROOT).as_posix(),
    )


def check_baseline(root: Path = ROOT):
    try:
        manifest = read_json(root / SOURCE_FILES["baseline"])
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        return {"manifest_records": 0, "checked": 0, "mismatches": [], "result": "not_run", "error": str(error)}
    if not isinstance(manifest, dict) or not isinstance(manifest.get("files"), list):
        return {"manifest_records": 0, "checked": 0, "mismatches": [{"error": "invalid_baseline_manifest_shape"}], "result": "fail"}
    mismatches = []
    for item in manifest.get("files", []):
        if not isinstance(item, dict) or not isinstance(item.get("snapshot_path"), str) or not isinstance(item.get("sha256"), str):
            mismatches.append({"error": "invalid_baseline_file_record"})
            continue
        snapshot_ref = PurePosixPath(item["snapshot_path"])
        if snapshot_ref.is_absolute() or PureWindowsPath(item["snapshot_path"]).drive or ".." in snapshot_ref.parts or "\\" in item["snapshot_path"]:
            mismatches.append({"path": item["snapshot_path"], "error": "unsafe_snapshot_path"})
            continue
        target = root / "设计稿/首版基线/凡人人界_作者基线1.0_20261015" / Path(*snapshot_ref.parts)
        if not is_regular_path_without_links(root, target, allow_file=True):
            mismatches.append({"path": item["snapshot_path"], "error": "missing_or_linked_snapshot"})
            continue
        actual = digest(target)
        if actual != item["sha256"]:
            mismatches.append({"path": item["snapshot_path"], "error": "sha256_mismatch", "actual": actual, "expected": item["sha256"]})
    return {"manifest_records": len(manifest["files"]), "checked": len(manifest["files"]) - len(mismatches), "mismatches": mismatches, "result": "pass" if not mismatches else "fail"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--skip-legacy-media", action="store_true", help="Do not read files in the two explicitly allowed legacy media roots.")
    args = parser.parse_args()
    OUTPUT.mkdir(parents=True, exist_ok=True)
    sources, source_errors = load_sources()
    catalog, issues = build_catalog(sources, source_errors)
    try:
        public_cards = parse_public_author_cards()
    except (OSError, ValueError) as error:
        public_cards = {}
        issues.append({"severity": "error", "code": "public_author_cards_unavailable", "error": str(error)})
    if EVENTS.is_file():
        try:
            catalog["source_manifest"].append({"id": "public_event_author_cards", "path": rel(EVENTS), "declared_version": "1.1", "sha256": digest(EVENTS), "format": "markdown"})
            catalog["source_manifest"].sort(key=lambda item: item["id"])
        except (OSError, ValueError) as error:
            issues.append({"severity": "error", "code": "public_author_cards_hash_error", "error": str(error)})
    packages = build_event_packages(public_cards)
    asset_audit = scan_assets(not args.skip_legacy_media, sources)
    staging = build_author_staging(sources.get("event_library", (None, {}))[1], public_cards, asset_audit)
    baseline = check_baseline()
    for package in packages:
        for card in package["events"]:
            assert set(card) == {"id", "type_name", "description_markdown", "enabled", "category", "tags", "intensity"}
            assert card["enabled"] is False
    dump(OUTPUT / "static-world-catalog.json", catalog)
    dump(OUTPUT / "disabled-event-package-candidates.json", packages)
    dump(OUTPUT / "portrait-asset-audit.json", asset_audit)
    dump(OUTPUT / "author-staging-resources.json", staging)
    dump(OUTPUT / "baseline-hash-check.json", baseline)
    manifest = {
        "kind": "derived_world_content_resource_manifest_not_runtime_registry",
        "content_version": "1.0.0",
        "runtime_installed": False,
        "source_manifest": catalog.get("source_manifest", []),
        "derived_artifacts": [
            {"path": rel(path), "status": status, "sha256": digest(path), "bytes": path.stat().st_size}
            for path, status in (
                (OUTPUT / "static-world-catalog.json", "derived_candidate"),
                (OUTPUT / "disabled-event-package-candidates.json", "schema_shaped_disabled_candidate"),
                (OUTPUT / "portrait-asset-audit.json", "read_only_inventory"),
                (OUTPUT / "author-staging-resources.json", "complete_author_staging_not_installed"),
            )
        ],
        "counts": catalog.get("counts", {}),
        "event_cards": [{"id": code, "title": (public_cards.get(code) or {}).get("title", title), "origin": origin, "state": state, "runtime_enabled": False, "author_card": rel(EVENTS), "author_card_sha256": (public_cards.get(code) or {}).get("source_sha256")} for code, title, origin, _, state, _ in EVENT_SPECS],
        "identity_policy": "Use source_key and canonical forward-slash relative paths; names and host absolute paths are never persistent identity.",
    }
    dump(ASSETS / "LUNA6_世界内容派生清单_20261015.json", manifest)
    report = {"source_errors": source_errors, "catalog_issues": issues, "baseline": baseline,
              "author_staging": {"formal_event_source_cards": len(staging["primary_author_cards"]), "public_event_cards": len(staging["public_event_author_cards"]), "projection_lossless": staging["event_package_projection"]["lossless"], "projection_enabled": False},
              "asset_summary": {"full_scan_performed": asset_audit["full_scan_performed"], "media_files_scanned": len(asset_audit["media_inventory"]), "candidate_refs_checked": len(asset_audit["checked_source_files"]), "missing_candidate_refs": len(asset_audit["missing_source_files"]), "unsafe_refs": len(asset_audit["unsafe_references"]), "duplicate_hash_groups": len(asset_audit["duplicate_content_hashes"]), "unmatched_denova_author_rows": sum(item["mapping_status"] == "unmatched_no_explicit_source_id" for item in asset_audit["denova_builtin_objects"])},
              "status": "pass" if not source_errors and not any(issue.get("severity") == "error" for issue in issues) and baseline["result"] == "pass" and not asset_audit["scan_errors"] and (args.skip_legacy_media or not asset_audit["missing_source_files"] and not asset_audit["unsafe_references"]) else "issues"}
    dump(OUTPUT / "build-report.json", report)
    print(json.dumps(report, ensure_ascii=False, sort_keys=True))
    return 0 if report["status"] == "pass" else 1


if __name__ == "__main__":
    sys.exit(main())
