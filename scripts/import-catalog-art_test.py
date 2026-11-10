#!/usr/bin/env python3
"""Exercise the offline artwork importer in an isolated miniature repository."""
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

from PIL import Image


class CatalogArtImportTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='ta-art-import-test-')
        self.root = pathlib.Path(self.temp.name)
        (self.root / 'scripts').mkdir()
        self.script = self.root / 'scripts/import-catalog-art.py'
        shutil.copyfile(pathlib.Path(__file__).with_name('import-catalog-art.py'), self.script)
        self.assets = self.root / 'internal/catalogart/assets'
        self.assets.mkdir(parents=True)
        self.manifest = self.assets / 'manifest.json'
        self.manifest.write_text(json.dumps({'schema': 'catalog-art.v1', 'entries': []}))
        self.original = self.root / 'portrait.png'
        picture = Image.new('RGB', (600, 900), '#246080')
        # A distinct upper region makes the top-biased avatar observable.
        picture.paste('#c03020', (0, 0, 600, 300))
        picture.save(self.original)

    def tearDown(self):
        self.temp.cleanup()

    def run_import(self, *args):
        return subprocess.run([
            sys.executable, str(self.script), '--world', 'test-world', '--kind', 'npc',
            '--id', 'person-1', '--source-sha256', 'a' * 64, '--image', str(self.original),
            '--review-state', 'reviewed', *args,
        ], text=True, capture_output=True)

    def assert_manifest_unchanged(self, before, result):
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.manifest.read_bytes(), before)

    def test_portrait_keeps_full_display_and_hashes_derivatives(self):
        result = self.run_import()
        self.assertEqual(result.returncode, 0, result.stderr)
        entry = json.loads(result.stdout)
        self.assertEqual(entry['original_sha256'], hashlib.sha256(self.original.read_bytes()).hexdigest())
        self.assertEqual(set(entry['variants']), {'avatar', 'display'})
        for role, dimensions in [('avatar', (128, 128)), ('display', (512, 768))]:
            variant = entry['variants'][role]
            path = self.assets / 'images' / (variant['sha256'] + '.png')
            self.assertEqual(hashlib.sha256(path.read_bytes()).hexdigest(), variant['sha256'])
            with Image.open(path) as image:
                image.load()
                self.assertEqual(image.size, dimensions)
        self.assertEqual(json.loads(self.manifest.read_text())['entries'], [entry])

    def test_explicit_avatar_crop_does_not_crop_display(self):
        result = self.run_import('--avatar-crop', '0', '300', '600', '600')
        self.assertEqual(result.returncode, 0, result.stderr)
        entry = json.loads(result.stdout)
        for role in ['avatar', 'display']:
            path = self.assets / 'images' / (entry['variants'][role]['sha256'] + '.png')
            with Image.open(path) as image:
                pixel = image.getpixel((image.width // 2, 10))
            if role == 'avatar':
                self.assertGreater(pixel[2], pixel[0])
            else:
                self.assertGreater(pixel[0], pixel[2])

    def test_duplicate_identity_and_reused_original_never_overwrite(self):
        self.assertEqual(self.run_import().returncode, 0)
        before = self.manifest.read_bytes()
        same = self.run_import()
        self.assert_manifest_unchanged(before, same)
        self.assertIn('identity/version already exists', same.stderr)
        reused = self.run_import('--id', 'person-2')
        self.assert_manifest_unchanged(before, reused)
        self.assertIn('already bound to another identity', reused.stderr)

    def test_bad_identity_hash_and_crop_fail_before_manifest_write(self):
        before = self.manifest.read_bytes()
        for args in [('--id', '../person'), ('--source-sha256', 'not-a-hash'),
                     ('--avatar-crop', '0', '800', '600', '600'),
                     ('--avatar-crop', '0', '0', '50', '50')]:
            with self.subTest(args=args):
                self.assert_manifest_unchanged(before, self.run_import(*args))

    def test_definition_requires_square_original_and_no_avatar_crop(self):
        before = self.manifest.read_bytes()
        self.assert_manifest_unchanged(before, self.run_import('--kind', 'definition'))
        square = self.root / 'definition.png'
        Image.new('RGB', (512, 512), '#0a6030').save(square)
        self.assert_manifest_unchanged(before, self.run_import(
            '--kind', 'definition', '--image', str(square), '--avatar-crop', '0', '0', '512', '512'))
        result = self.run_import('--kind', 'definition', '--image', str(square))
        self.assertEqual(result.returncode, 0, result.stderr)
        variant = json.loads(result.stdout)['variants']['illustration']
        self.assertEqual((variant['width'], variant['height']), (512, 512))

    def test_original_changed_after_review_cannot_be_imported(self):
        reviewed_hash = hashlib.sha256(self.original.read_bytes()).hexdigest()
        before = self.manifest.read_bytes()
        Image.new('RGB', (600, 900), '#a06080').save(self.original)
        result = self.run_import('--expected-original-sha256', reviewed_hash)
        self.assert_manifest_unchanged(before, result)
        self.assertIn('original bytes changed since review', result.stderr)
        self.assertFalse((self.assets / 'images').exists())
        actual_hash = hashlib.sha256(self.original.read_bytes()).hexdigest()
        result = self.run_import('--expected-original-sha256', actual_hash)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)['original_sha256'], actual_hash)


if __name__ == '__main__':
    unittest.main()
