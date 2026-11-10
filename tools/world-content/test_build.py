import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import asset_audit
import build


class CatalogBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(ignore_cleanup_errors=True)
        self.root = Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def sources(self, *, discovery=True, initial=True, initial_place="loc-a", category="cat-a", domain="core", route_from="loc-a", profile_path="profiles/a.md", keys=("a",)):
        profile_dir = self.root / "设计稿/profiles"
        profile_dir.mkdir(parents=True, exist_ok=True)
        (profile_dir / "a.md").write_text("profile", encoding="utf-8")
        profiles = [{"source_key": key, "name": "same-name", "country": "越国", "path": profile_path} for key in keys]
        files = {
            "profiles": ("profiles.json", {"profiles": profiles}),
            "discovery": ("discovery.json", {"characters": [{"source_key": key, "faction_id": "f1", "initial_place_candidate": "loc-a", "disclosure": "catalog_only"} for key in keys] if discovery else []}),
            "relations": ("relations.json", {"countries": [], "factions": [{"id": "f1", "character_refs": list(keys)}], "locations": [{"id": "loc-a", "character_refs": list(keys)}]}),
            "handoff": ("handoff.json", {"initial_characters": [{"source_key": key, "initial_stage": "stage", "initial_place": initial_place} for key in keys] if initial else [], "places": [], "routes": [{"id": "r1", "from": route_from, "to": "loc-a"}]}),
            "event_library": ("events.json", {"era": "parallel", "counts": {}, "cards": [{"id": "e1", "category_id": category, "package_id": "pkg-a", "required_domains": [domain], "type_name": "test", "description_markdown": "test", "trigger": "test", "fusion": "test", "logic": "test", "payoff": "test", "reward_cost": "test", "guardrail": "test"}], "categories": [{"id": "cat-a", "package_id": "pkg-a"}], "domain_contracts": {"core": "core"}}),
            "abilities": ("abilities.json", {"methods": [], "skills": [], "mechanical_frames": [], "traits": []}),
            "portrait_bindings": ("bindings.json", {}), "portrait_media": ("media.json", {}),
            "legacy_portraits": ("legacy.csv", []), "denova_portraits": ("denova.csv", []), "baseline": ("baseline.json", {}),
        }
        loaded = {}
        for key, (filename, value) in files.items():
            path = self.root / filename
            if path.suffix == ".csv":
                path.write_text("", encoding="utf-8")
            else:
                path.write_text(json.dumps(value), encoding="utf-8")
            loaded[key] = (path, value)
        return loaded

    def catalog(self, sources):
        with patch.object(build, "ROOT", self.root):
            return build.build_catalog(sources, [], root=self.root)

    def test_same_display_names_are_nonblocking_and_source_keys_stay_distinct(self):
        catalog, issues = self.catalog(self.sources(keys=("a", "b")))
        self.assertEqual([item["source_key"] for item in catalog["characters"]], ["a", "b"])
        self.assertIn("same_name_kept_as_separate_source_keys", {item["code"] for item in issues})
        self.assertFalse(any(item["severity"] == "error" for item in issues))

    def test_unsafe_profile_path_is_rejected_without_hashing_external_file(self):
        sources = self.sources(profile_path="../private.md")
        with patch.object(build, "ROOT", self.root), patch.object(build, "digest", wraps=build.digest) as digest_spy:
            catalog, issues = build.build_catalog(sources, [], root=self.root)
        self.assertIsNone(catalog["characters"][0]["profile_sha256"])
        self.assertIn("missing_or_unsafe_profile_ref", {item["code"] for item in issues})
        outside = (self.root / "設計稿/private.md").resolve()
        self.assertFalse(any(call.args and Path(call.args[0]).resolve() == outside for call in digest_spy.call_args_list))

    def test_missing_discovery_and_initial_records_are_reported(self):
        _, issues = self.catalog(self.sources(discovery=False, initial=False))
        codes = {item["code"] for item in issues}
        self.assertIn("missing_discovery_record", codes)
        self.assertIn("missing_initial_characters_record", codes)

    def test_unknown_initial_place_is_reported(self):
        _, issues = self.catalog(self.sources(initial_place="unknown-place"))
        self.assertIn("unresolved_initial_place", {item["code"] for item in issues})

    def test_unknown_event_category_and_domain_are_reported(self):
        _, issues = self.catalog(self.sources(category="missing-category", domain="unknown-domain"))
        codes = {item["code"] for item in issues}
        self.assertIn("unknown_event_category", codes)
        self.assertIn("unknown_event_domain", codes)

    def test_malformed_nested_source_values_are_reported_without_crashing(self):
        sources = self.sources()
        sources["relations"][1]["locations"][0]["character_refs"] = None
        sources["handoff"][1]["routes"][0]["from"] = ["not-a-route-id"]
        sources["event_library"][1]["cards"][0]["category_id"] = ["not-a-category"]
        sources["abilities"][1]["methods"].append({"id": "method", "learnable_skill_refs": None})
        _, issues = self.catalog(sources)
        codes = {item["code"] for item in issues}
        self.assertIn("invalid_character_refs", codes)
        self.assertIn("unresolved_route_endpoint", codes)
        self.assertIn("unknown_event_category", codes)
        self.assertIn("invalid_method_skill_refs", codes)

    def test_faction_cannot_be_used_as_route_place(self):
        _, issues = self.catalog(self.sources(route_from="f1"))
        self.assertIn("faction_used_as_route_place", {item["code"] for item in issues})

    def test_absolute_project_root_is_valid(self):
        _, issues = self.catalog(self.sources())
        self.assertFalse(any(item["code"] == "unsafe_profile_ref" for item in issues))

    def test_missing_sources_return_an_issue_instead_of_crashing(self):
        catalog, issues = build.build_catalog({}, [{"source": "missing", "error": "gone"}], root=self.root)
        self.assertIsInstance(catalog, dict)
        self.assertIn("source_read_error", {item["code"] for item in issues})

    def test_missing_source_files_and_baseline_are_reported_without_crashing(self):
        loaded, errors = build.load_sources(self.root)
        self.assertEqual(loaded, {})
        self.assertTrue(errors)
        self.assertEqual(build.check_baseline(self.root)["result"], "not_run")

    def test_invalid_baseline_shape_is_reported_without_crashing(self):
        path = self.root / build.SOURCE_FILES["baseline"]
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("[]", encoding="utf-8")
        result = build.check_baseline(self.root)
        self.assertEqual(result["result"], "fail")
        self.assertEqual(result["mismatches"][0]["error"], "invalid_baseline_manifest_shape")

    def test_missing_public_author_cards_leave_safe_disabled_projection(self):
        packages = build.build_event_packages({})
        self.assertEqual(sum(len(package["events"]) for package in packages), 6)
        self.assertTrue(all(card["enabled"] is False for package in packages for card in package["events"]))

    def test_event_projection_stays_disabled_and_author_card_is_preserved_separately(self):
        source = self.root / "synthetic-public-cards.md"
        source.write_text("\n\n".join(
            f"## {code} Synthetic {code}\nSynthetic narrative card {code}; test only."
            for code in ("C01", "C02", "C03", "O01", "O02", "O03")
        ), encoding="utf-8")
        with patch.object(build, "ROOT", self.root), patch.object(build, "EVENTS", source):
            cards = build.parse_public_author_cards(source)
            packages = build.build_event_packages(cards)
        self.assertEqual(len(cards), 6)
        self.assertTrue(all(card["source_ref"].startswith("synthetic-public-cards.md#") for card in cards.values()))
        self.assertEqual(sum(len(package["events"]) for package in packages), 6)
        self.assertTrue(all(card["enabled"] is False for package in packages for card in package["events"]))
        self.assertTrue(all(card["body_markdown"] for card in cards.values()))

    def test_missing_public_author_source_reports_the_missing_file(self):
        with self.assertRaises(FileNotFoundError):
            build.parse_public_author_cards(self.root / "missing-cards.md")


class MediaAuditTests(unittest.TestCase):
    def test_person_candidate_pools_isolate_media_by_stable_source_key(self):
        with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as directory:
            root = Path(directory)
            (root / "person-a.png").write_bytes(b"A")
            (root / "person-b.png").write_bytes(b"B")
            sources = {
                "profiles": (None, {"profiles": [
                    {"source_key": "person-a", "name": "Person A"},
                    {"source_key": "person-b", "name": "Person B"},
                ]}),
                "portrait_bindings": (None, {"bindings": [
                    {"legacy_npc_id": "person-a", "name": "Person A", "candidate_asset_paths": ["mother:person-a.png"]},
                    {"legacy_npc_id": "person-b", "name": "Person B", "candidate_asset_paths": ["mother:person-b.png"]},
                ]}),
                "portrait_media": (None, {"videos": []}),
                "denova_portraits": (None, []),
            }
            audit = asset_audit.build_portrait_audit(
                perform_scan=True,
                sources=sources,
                roots={"mother": root},
                digest_function=lambda path: f"hash-{path.stem}",
                project_relative_path=lambda path: path.name,
            )
            by_key = {item["source_key"]: item for item in audit["denova_author_bindings"]}
            pools = {key: item["candidate_pool"] for key, item in by_key.items()}
            refs = {key: [media["relative_path"] for media in pool["verified_media_refs"]] for key, pool in pools.items()}
            self.assertEqual(refs, {"person-a": ["person-a.png"], "person-b": ["person-b.png"]})
            self.assertNotEqual(pools["person-a"]["pool_id"], pools["person-b"]["pool_id"])
            self.assertTrue(all(pool["scene_review_status"] == "unreviewed" for pool in pools.values()))
            self.assertTrue(all(pool["auto_selection_allowed"] is False for pool in pools.values()))
            self.assertTrue(all("scene_pool_ref" not in item for item in audit["media_inventory"]))
            self.assertTrue(all("source_directory_group" in item for item in audit["media_inventory"]))

    def test_non_media_log_is_never_passed_to_digest(self):
        with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as directory:
            root = Path(directory)
            (root / "image.png").write_bytes(b"media")
            (root / "_monitor.log").write_text("private log", encoding="utf-8")
            digest_paths = []

            def digest_spy(path):
                digest_paths.append(path.name)
                self.assertEqual(path.suffix.lower(), ".png")
                return "media-hash"

            inventory, skipped, errors, _ = asset_audit.inventory_media_roots({"mother": root}, digest_spy)
            self.assertEqual([item["relative_path"] for item in inventory], ["image.png"])
            self.assertEqual(digest_paths, ["image.png"])
            self.assertEqual(skipped, [{"source": "mother", "relative_path": "_monitor.log", "skip_reason": "extension_not_allowlisted_content_not_opened"}])
            self.assertEqual(errors, [])

    def test_unsafe_media_reference_is_rejected(self):
        self.assertIsNone(asset_audit._safe_relative("../outside.png"))
        self.assertIsNone(asset_audit._safe_relative("C:/outside.png"))
        self.assertIsNone(asset_audit._safe_relative("video.txt"))

    def test_linked_media_path_is_rejected_before_digest(self):
        with tempfile.TemporaryDirectory(ignore_cleanup_errors=True) as directory:
            root = Path(directory)
            image = root / "image.png"
            image.write_bytes(b"must-not-read")
            with patch.object(asset_audit, "_is_link_or_junction", side_effect=lambda path: path == image):
                inventory, _, errors, _ = asset_audit.inventory_media_roots({"mother": root}, lambda _: self.fail("linked file was read"))
            self.assertEqual(inventory, [])
            self.assertEqual(errors[0]["error"], "media_link_rejected")


if __name__ == "__main__":
    unittest.main()
