"""Focused tests for safe paths, media inventory, cache, and review validation."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from PIL import Image

import records
import scan
import default_package


class PortraitAssetTests(unittest.TestCase):
    def _default_fixture(self, root: Path, count: int = 101) -> tuple[dict, dict]:
        people = []
        choices = []
        profile = root / "设计稿" / "profiles" / "person.md"
        profile.parent.mkdir(parents=True, exist_ok=True)
        profile.write_text("formal profile", encoding="utf-8")
        profile_hash = scan.sha256_file(profile)
        for index in range(count):
            stable_id = f"person-{index:03}"
            candidate = root / "assets" / "npc-portraits-candidate" / stable_id / "portrait.png"
            candidate.parent.mkdir(parents=True, exist_ok=True)
            Image.new("RGB", (24, 24), (index % 255, 20, 30)).save(candidate)
            digest = scan.sha256_file(candidate)
            metadata = candidate.parent / "metadata.json"
            metadata.write_text(json.dumps({"asset_sha256": digest, "source_profile": {"sha256": profile_hash}, "generation_prompt": f"Prompt {index}", "reference_images": [], "fact_vs_visual_supplement": {"visual_supplements": "Palette"}}), encoding="utf-8")
            people.append({"stable_id": stable_id, "name": f"Person {index}", "author_directory_member": True,
                           "author_profile_path": "profiles/person.md", "author_profile_hash": profile_hash,
                           "catalog_profile_hash": profile_hash, "generated_candidates": [{"workspace_relative_path": f"assets/npc-portraits-candidate/{stable_id}/portrait.png", "sha256": digest}], "old_media": []})
            choices.append({"stable_id": stable_id, "display_name": f"Person {index}", "source": {"kind": "candidate", "project_relative_path": f"assets/npc-portraits-candidate/{stable_id}/portrait.png", "metadata_project_relative_path": f"assets/npc-portraits-candidate/{stable_id}/metadata.json", "sha256": digest}, "selection_authority": "user_authorized_default_selection", "selection_basis": "User authorized default selection", "user_human_review": "not_reviewed", "model_visual_inspection": "Face and clothing checked.", "game_install_status": "not_installed_not_verified"})
        author = root / "author.json"
        author.write_text("{}", encoding="utf-8")
        work = root / "work" / "luna6-portrait-assets"
        work.mkdir(parents=True, exist_ok=True)
        inventory_path = work / "inventory.json"
        inventory_path.write_text(json.dumps({"people": people}), encoding="utf-8")
        choices_path = work / "choices.json"
        choices_path.write_text(json.dumps({"schema_version": 1, "mapping_version": default_package.CHOICES_VERSION, "choices": choices}), encoding="utf-8")
        with patch.object(default_package, "INVENTORY_PATH", inventory_path.relative_to(root).as_posix()), \
                patch.object(default_package, "AUTHOR_DIRECTORY_PATH", author.relative_to(root).as_posix()):
            return json.loads(inventory_path.read_text(encoding="utf-8")), json.loads(choices_path.read_text(encoding="utf-8"))

    def test_default_choices_require_exact_unique_complete_coverage(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary)
            inventory, choices = self._default_fixture(root)
            choices["choices"].pop()
            with self.assertRaisesRegex(ValueError, "cover each author"):
                default_package._validate_choices(inventory, choices)
            inventory, choices = self._default_fixture(root)
            choices["choices"][-1] = dict(choices["choices"][0])
            with self.assertRaisesRegex(ValueError, "duplicate stable ID"):
                default_package._validate_choices(inventory, choices)

    def test_default_package_export_and_verify_keep_review_and_runtime_status_separate(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary)
            self._default_fixture(root)
            metadata = root / "assets" / "npc-portraits-candidate" / "person-000" / "metadata.json"
            metadata_value = json.loads(metadata.read_text(encoding="utf-8"))
            metadata_value["source_profile"]["sha256"] = metadata_value["source_profile"]["sha256"].upper()
            metadata.write_text(json.dumps(metadata_value), encoding="utf-8")
            with patch.object(default_package, "INVENTORY_PATH", "work/luna6-portrait-assets/inventory.json"), \
                    patch.object(default_package, "AUTHOR_DIRECTORY_PATH", "author.json"):
                result = default_package.export_defaults(root, "choices.json")
                self.assertEqual(result["entries"], 101)
                verified = default_package.verify_defaults(root)
                self.assertEqual(verified["entries"], 101)
                manifest_path = root / default_package.DEFAULT_ROOT / "manifest.json"
                manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
                self.assertEqual(manifest["entries"][0]["selection_authority"], "user_authorized_default_selection")
                self.assertEqual(manifest["entries"][0]["user_human_review"], "not_reviewed")
                self.assertEqual(manifest["entries"][0]["game_install_status"], "not_installed_not_verified")
                image_path = root / manifest["entries"][0]["portrait"]["path"]
                image_path.write_bytes(b"changed")
                with self.assertRaisesRegex(ValueError, "hash mismatch"):
                    default_package.verify_defaults(root)

    def test_default_package_rejects_traversal_and_wrong_source_hash(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary)
            inventory, choices = self._default_fixture(root)
            choices["choices"][0]["source"]["project_relative_path"] = "assets/npc-portraits-candidate/../outside.png"
            with self.assertRaisesRegex(ValueError, "normalized project-relative path"):
                default_package._read_source(root, inventory["people"][0], choices["choices"][0])

    def test_audited_shared_template_requires_exact_person_and_explicit_selection(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary) / "project"
            external = Path(temporary) / "legacy"
            image_path = external / "frontend" / "assets" / "portraits" / "job_shop.png"
            image_path.parent.mkdir(parents=True)
            Image.new("RGB", (24, 24), (40, 60, 80)).save(image_path)
            audit_path = root / "audit.json"
            audit_path.parent.mkdir(parents=True)
            audit_path.write_text(json.dumps({"legacy_npc_objects": [{
                "legacy_npc_id": "person-000", "portrait_key": "job_shop", "name": "Person 0",
                "primary_image": "frontend/assets/portraits/job_shop.png", "shared_template": True,
            }]}), encoding="utf-8")
            choice = {"source": {
                "kind": "audit_shared_template", "stable_id": "person-000",
                "sha256": scan.sha256_file(image_path),
            }, "shared_template": True}
            with patch.object(default_package, "PORTRAIT_AUDIT_PATH", "audit.json"), \
                    patch.object(default_package, "_external_project_root", return_value=external):
                source, provenance, _ = default_package._read_source(root, {"stable_id": "person-000", "name": "Person 0"}, choice)
            self.assertEqual(source, image_path)
            self.assertEqual(provenance["kind"], "legacy_shared_template")

    def test_safe_resolve_rejects_traversal_and_links(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary)
            (root / "inside").mkdir()
            with self.assertRaises(ValueError):
                scan.safe_resolve(root, "../outside")
            (root / "inside" / "file.txt").write_text("ok", encoding="utf-8")
            with patch("scan.is_linklike", return_value=True):
                with self.assertRaises(ValueError):
                    scan.safe_resolve(root, "inside/file.txt")

    def test_bad_image_duplicate_hash_and_video_diagnostic_are_separate(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary) / "source"
            output = Path(temporary) / "output"
            root.mkdir()
            Image.new("RGB", (12, 8), (120, 20, 30)).save(root / "ok.png")
            (root / "duplicate.png").write_bytes((root / "ok.png").read_bytes())
            (root / "broken.jpg").write_bytes(b"not an image")
            (root / "clip.mp4").write_bytes(b"video bytes")
            assets, diagnostics, duplicates = scan.scan_media_roots([("mother", root)], output, False)
            codes = {item["code"] for item in diagnostics}
            self.assertIn("duplicate_content_hash", codes)
            self.assertIn("image_decode_failed", codes)
            self.assertIn("video_poster_tool_unavailable", codes)
            clip = next(item for item in assets if item["media_type"] == "video/mp4")
            self.assertEqual(clip["poster_status"], "unavailable_no_local_video_tool")
            self.assertTrue(duplicates)
            self.assertIsNone(next(item for item in assets if item["relative_path"] == "broken.jpg").get("thumbnails", {}).get("192"))

    def test_thumbnail_cache_hits_then_source_hash_change_invalidates(self) -> None:
        with tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parents[2]) as temporary:
            root = Path(temporary) / "source"
            output = Path(temporary) / "output"
            root.mkdir()
            source = root / "person.png"
            Image.new("RGB", (16, 10), (10, 20, 30)).save(source)
            first, _, _ = scan.scan_media_roots([("mother", root)], output, False)
            second, _, _ = scan.scan_media_roots([("mother", root)], output, False)
            self.assertFalse(first[0]["cache_hit"])
            self.assertTrue(second[0]["cache_hit"])
            Image.new("RGB", (16, 10), (30, 20, 10)).save(source)
            third, _, _ = scan.scan_media_roots([("mother", root)], output, False)
            self.assertFalse(third[0]["cache_hit"])

    def test_review_mapping_defaults_pending_and_round_trips_explicit_approval(self) -> None:
        people = [{
            "stable_id": "sample-person", "name": "样例", "old_media": [],
            "generated_candidates": [{"workspace_relative_path": "assets/npc-portraits-candidate/sample-person/a.png", "sha256": "a" * 64, "media_type": "image/png"}],
        }]
        mapping = records.build_initial_mapping(people)
        media = mapping["entries"][0]["media"][0]
        self.assertEqual(media["review_status"], "pending")
        self.assertFalse(media["human_approved"])
        media["review_status"] = "approved"
        asset_path = media["asset_path"]
        validated = records.validate_review_mapping(mapping, Path.cwd(), {asset_path: "a" * 64})
        self.assertTrue(validated["entries"][0]["media"][0]["human_approved"])
        self.assertEqual(validated["entries"][0]["media"][0]["review_status"], "approved")

    def test_review_mapping_rejects_cross_person_asset_and_unknown_identity(self) -> None:
        mapping = {
            "schema_version": 1,
            "mapping_version": "portrait-review-v1",
            "entries": [{
                "stable_id": "person-a", "display_name": "甲", "media": [{
                    "asset_path": "assets/a.png", "source_sha256": "a" * 64,
                    "candidate_scene": "daily_base_candidate", "review_status": "approved",
                }],
            }],
        }
        asset_hashes = {"assets/a.png": "a" * 64}
        with self.assertRaisesRegex(ValueError, "not associated"):
            records.validate_review_mapping(
                mapping, Path.cwd(), asset_hashes,
                {("assets/a.png", "a" * 64): {"person-b"}},
                {"person-a": "甲", "person-b": "乙"},
            )
        with self.assertRaisesRegex(ValueError, "known stable ID"):
            records.validate_review_mapping(
                mapping, Path.cwd(), asset_hashes,
                {("assets/a.png", "a" * 64): {"person-a"}},
                {"person-a": "乙"},
            )

    def test_review_mapping_rejects_versions_conflicts_and_duplicate_binding(self) -> None:
        base = {"schema_version": 2, "mapping_version": "portrait-review-v1", "entries": []}
        with self.assertRaises(ValueError):
            records.validate_review_mapping(base, Path.cwd())
        duplicate = {"schema_version": 1, "mapping_version": "portrait-review-v1", "entries": [
            {"stable_id": "same", "display_name": "甲", "media": []},
            {"stable_id": "same", "display_name": "乙", "media": []},
        ]}
        with self.assertRaises(ValueError):
            records.validate_review_mapping(duplicate, Path.cwd())
        binding = {"schema_version": 1, "mapping_version": "portrait-review-v1", "entries": [
            {"stable_id": "same", "display_name": "甲", "media": [
                {"asset_path": "assets/a.png", "source_sha256": "b" * 64, "review_status": "pending"},
                {"asset_path": "assets/a.png", "source_sha256": "b" * 64, "review_status": "pending"},
            ]},
        ]}
        with self.assertRaises(ValueError):
            records.validate_review_mapping(binding, Path.cwd())
        with self.assertRaises(ValueError):
            records.validate_review_mapping(binding, Path.cwd(), {})


if __name__ == "__main__":
    unittest.main()
