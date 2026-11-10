import hashlib
import json
from pathlib import Path
import subprocess
import shutil
import zipfile
import sys
import tempfile
import unittest

from PIL import Image, PngImagePlugin


SCRIPT = Path(__file__).with_name("convert_native_webp_catalog.py")


class ConversionSafetyTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.source = self.root / "original"
        (self.source / "images").mkdir(parents=True)
        self.target = self.root / "converted"
        self.picture = Image.new("RGBA", (3, 2))
        self.picture.putdata([(19, 23, 31, 0), (0, 0, 255, 1), (90, 123, 211, 77), (255, 0, 1, 255), (11, 19, 71, 128), (60, 70, 80, 200)])
        self.write_source()

    def write_source(self, pnginfo=None):
        path = self.root / "temporary.png"
        self.picture.save(path, pnginfo=pnginfo)
        self.identity = hashlib.sha256(path.read_bytes()).hexdigest()
        (self.source / "images" / (self.identity + ".png")).write_bytes(path.read_bytes())
        self.manifest = {"schema": "catalog-art.v1", "entries": [{"world": "fixture", "kind": "definition", "id": "one", "version": 1, "variants": {"illustration": {"sha256": self.identity, "width": 3, "height": 2}}}]}
        self.save_manifest()

    def save_manifest(self):
        (self.source / "manifest.json").write_text(json.dumps(self.manifest))

    def run_conversion(self, expected_success=False, destination=None, extra=None):
        before = {str(p.relative_to(self.source)): p.read_bytes() for p in self.source.rglob("*") if p.is_file()}
        result = subprocess.run([sys.executable, str(SCRIPT), "--source-assets", str(self.source), "--dest-assets", str(destination or self.target)] + (extra or []), capture_output=True, text=True)
        after = {str(p.relative_to(self.source)): p.read_bytes() for p in self.source.rglob("*") if p.is_file()}
        self.assertEqual(before, after, "Input tree changed")
        self.assertEqual(result.returncode == 0, expected_success, result.stdout + result.stderr)
        return result

    def test_reuses_checkpoint_only_after_pixel_verification(self):
        cache = self.root / "checkpoint"
        self.run_conversion(True, extra=["--verified-cache", str(cache)])
        checkpoint = cache / (self.identity + ".webp")
        self.assertTrue(checkpoint.is_file())
        other = self.root / "second"
        self.run_conversion(True, destination=other, extra=["--verified-cache", str(cache)])
        report = json.loads((self.root / "second-conversion-report.json").read_text())
        self.assertEqual(report["verified_checkpoint_reuses"], 1)
        checkpoint.write_bytes(b"corrupt")
        self.run_conversion(False, destination=self.root / "third", extra=["--verified-cache", str(cache)])
        self.assertFalse((self.root / "third").exists())

    def test_rejects_checkpoint_inside_input(self):
        self.run_conversion(False, extra=["--verified-cache", str(self.source / "checkpoint")])

    def test_preserves_pixels_alpha_public_identity_and_originals(self):
        self.run_conversion(True)
        output = json.loads((self.target / "manifest.json").read_text())
        v = output["entries"][0]["variants"]["illustration"]
        self.assertEqual(v["legacy_sha256"], self.identity)
        self.assertEqual(v["format"], "webp")
        data = (self.target / "images" / (v["sha256"] + ".webp")).read_bytes()
        self.assertEqual(hashlib.sha256(data).hexdigest(), v["sha256"])
        with Image.open(self.target / "images" / (v["sha256"] + ".webp")) as image:
            self.assertEqual(image.convert("RGBA").tobytes(), self.picture.tobytes())
        self.assertFalse(list((self.target / "images").glob("*.png")))

    def test_retains_replaced_historical_png_and_url(self):
        history = self.root / "history"
        shutil.copytree(self.source, history)
        old_id = self.identity
        old_pixels = self.picture.tobytes()
        self.picture = Image.new("RGBA", (3, 2), (1, 2, 3, 255))
        self.write_source()
        self.run_conversion(True, extra=["--history-assets", str(history)])
        output = json.loads((self.target / "manifest.json").read_text())
        self.assertEqual(len(output["retained_images"]), 1)
        retained_id = output["aliases"][old_id]
        with Image.open(self.target / "images" / (retained_id + ".webp")) as image:
            self.assertEqual(image.convert("RGBA").tobytes(), old_pixels)
        report = json.loads((self.root / "converted-conversion-report.json").read_text())
        self.assertEqual(report["historical_urls_verified"], 1)
        self.assertEqual(report["historical_urls"][old_id + ".png"], retained_id)

    def test_preserves_prior_native_bytes_and_cached_redirect_targets(self):
        self.run_conversion(True)
        previous = json.loads((self.target / "manifest.json").read_text())
        old_png = self.identity
        old_native = previous["entries"][0]["variants"]["illustration"]["sha256"]
        old_bytes = (self.target / "images" / (old_native + ".webp")).read_bytes()
        self.picture = Image.new("RGBA", (3, 2), (44, 55, 66, 255))
        self.write_source()
        destination = self.root / "second"
        self.run_conversion(True, destination, ["--history-assets", str(self.target)])
        self.assertEqual((destination / "images" / (old_native + ".webp")).read_bytes(), old_bytes)
        manifest = json.loads((destination / "manifest.json").read_text())
        self.assertEqual(manifest["aliases"][old_png], old_native)
        report = json.loads((self.root / "second-conversion-report.json").read_text())
        self.assertEqual(report["historical_urls_verified"], 3)

    def test_reads_prior_delivery_archive_without_extracting_it(self):
        archive = self.root / "history.zip"
        with zipfile.ZipFile(archive, "w") as z:
            for p in self.source.rglob("*"):
                if p.is_file():
                    z.write(p, "delivery/internal/catalogart/assets/" + p.relative_to(self.source).as_posix())
        self.picture = Image.new("RGBA", (3, 2), (15, 25, 35, 255))
        self.write_source()
        self.run_conversion(True, extra=["--history-archive", str(archive)])
        report = json.loads((self.root / "converted-conversion-report.json").read_text())
        self.assertEqual(report["retained_images"], 1)
        self.assertEqual(report["historical_urls_verified"], 1)

    def test_independent_verifier_rejects_wrong_historical_picture(self):
        history = self.root / "history"
        shutil.copytree(self.source, history)
        old_id = self.identity
        self.picture = Image.new("RGBA", (3, 2), (99, 88, 77, 255))
        self.write_source()
        self.run_conversion(True, extra=["--history-assets", str(history)])
        output = json.loads((self.target / "manifest.json").read_text())
        output["aliases"][old_id] = output["entries"][0]["variants"]["illustration"]["sha256"]
        (self.target / "manifest.json").write_text(json.dumps(output))
        result = subprocess.run([sys.executable, str(SCRIPT.with_name("verify_catalog_history.py")), "--generated-assets", str(self.target), "--history-assets", str(history)], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("pixels changed", result.stderr)

    def test_rejects_incomplete_current_world_manifest(self):
        history = self.root / "history"
        shutil.copytree(self.source, history)
        self.manifest["entries"][0]["world"] = "wrong-subset"
        self.save_manifest()
        result = self.run_conversion(extra=["--history-assets", str(history)])
        self.assertIn("historical author identities", result.stderr)
        self.assertFalse(self.target.exists())

    def test_refuses_existing_output(self):
        self.target.mkdir()
        sentinel = self.target / "sentinel.txt"
        sentinel.write_text("unchanged")
        self.run_conversion()
        self.assertEqual(sentinel.read_text(), "unchanged")

    def test_refuses_output_inside_source(self):
        self.run_conversion(destination=self.source / "output")

    def test_rejects_wrong_original_hash(self):
        (self.source / "images" / (self.identity + ".png")).write_bytes(b"tampered")
        self.run_conversion()
        self.assertFalse(self.target.exists())

    def test_rejects_wrong_dimensions(self):
        self.manifest["entries"][0]["variants"]["illustration"]["width"] = 4
        self.save_manifest()
        self.run_conversion()
        self.assertFalse(self.target.exists())

    def test_rejects_unpreserved_color_metadata(self):
        info = PngImagePlugin.PngInfo()
        info.add(b"sRGB", b"\x00")
        self.write_source(info)
        self.run_conversion()
        self.assertFalse(self.target.exists())

    def test_requires_explicit_bulk_conversion_flag(self):
        entry = self.manifest["entries"][0]
        self.manifest["entries"] = [dict(entry, id=str(i), variants={"illustration": {"sha256": f"{i:064x}", "width": 3, "height": 2}}) for i in range(33)]
        self.save_manifest()
        result = self.run_conversion()
        self.assertIn("More than 32", result.stderr)
        self.assertFalse(self.target.exists())


if __name__ == "__main__":
    unittest.main()
