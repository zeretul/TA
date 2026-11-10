#!/usr/bin/env python3
"""Import one reviewed standalone artwork without touching an author world file.
Requires Pillow. Original files stay in the production archive; shipped derivatives
are immutable PNGs, and the manifest is replaced only after all files exist.
"""
import argparse, hashlib, io, json, pathlib, re
from PIL import Image, ImageOps


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--world', required=True)
    parser.add_argument('--kind', choices=['npc', 'definition'], required=True)
    parser.add_argument('--id', required=True)
    parser.add_argument('--version', type=int, default=1)
    parser.add_argument('--category', default='')
    parser.add_argument('--source-sha256', required=True)
    parser.add_argument('--image', type=pathlib.Path, required=True)
    parser.add_argument('--expected-original-sha256', help='require the exact reviewed original bytes before writing any artifact')
    parser.add_argument('--review-state', choices=['candidate', 'reviewed'], required=True)
    parser.add_argument('--avatar-crop', nargs=4, type=int, metavar=('LEFT', 'TOP', 'WIDTH', 'HEIGHT'), help='reviewed NPC avatar crop in original pixels; display image stays complete')
    args = parser.parse_args()
    for value in (args.world, args.id):
        if not re.fullmatch(r'[a-z0-9_.-]{1,128}', value) or '..' in value:
            parser.error('unsafe world or stable ID')
    if not re.fullmatch('[a-f0-9]{64}', args.source_sha256) or args.version < 1:
        parser.error('source SHA256 or version is invalid')
    if args.expected_original_sha256 and not re.fullmatch('[a-f0-9]{64}', args.expected_original_sha256):
        parser.error('expected original SHA256 is invalid')
    root = pathlib.Path(__file__).resolve().parents[1]
    assets = root / 'internal/catalogart/assets'
    manifest_path = assets / 'manifest.json'
    manifest = json.loads(manifest_path.read_text())
    key = (args.world, args.kind, args.id, args.version)
    # Existing rows may be deliberately superseded, never silently overwritten.
    if any((e['world'], e['kind'], e['id'], e['version']) == key for e in manifest['entries']):
        parser.error('identity/version already exists; review and explicitly revise the manifest before replacing it')
    if args.avatar_crop and args.kind != 'npc':
        parser.error('avatar crops apply only to NPC artwork')
    original = args.image.read_bytes()
    original_sha = hashlib.sha256(original).hexdigest()
    if args.expected_original_sha256 and original_sha != args.expected_original_sha256:
        parser.error('original bytes changed since review; no artwork was imported')
    if any(e.get('original_sha256') == original_sha for e in manifest['entries']):
        parser.error('original artwork is already bound to another identity; per-entry illustrations must be distinct')
    variants = {}
    with Image.open(io.BytesIO(original)) as image:
        image.load()
        if image.width < 512 or image.height < 512 or (args.kind == 'definition' and image.width != image.height):
            parser.error('source must be at least 512px; definition illustrations must be square')
        for role, size in ([('avatar', 128), ('display', 512)] if args.kind == 'npc' else [('illustration', 768)]):
            source = image.convert('RGB')
            if role == 'avatar':
                if args.avatar_crop:
                    left, top, width, height = args.avatar_crop
                    if min(left, top) < 0 or min(width, height) < 128 or left + width > image.width or top + height > image.height:
                        parser.error('avatar crop is outside the source or too small')
                    source = source.crop((left, top, left + width, top + height))
                # Portrait thumbnails keep the head in the upper part of tall art.
                derivative = ImageOps.fit(source, (128, 128), Image.Resampling.LANCZOS, centering=(0.5, 0.0))
            else:
                ratio = min(size / image.width, (768 if role == 'display' else size) / image.height, 1.0)
                dimensions = (round(image.width * ratio), round(image.height * ratio))
                derivative = source.resize(dimensions, Image.Resampling.LANCZOS)
            output = io.BytesIO()
            derivative.save(output, format='PNG', optimize=True)
            data = output.getvalue()
            sha = hashlib.sha256(data).hexdigest()
            path = assets / 'images' / f'{sha}.png'
            path.parent.mkdir(parents=True, exist_ok=True)
            if not path.exists():
                temp = path.with_suffix('.png.tmp'); temp.write_bytes(data); temp.replace(path)
            elif path.read_bytes() != data:
                raise RuntimeError('content-addressed image collision')
            variants[role] = {'sha256': sha, 'width': derivative.width, 'height': derivative.height}
    entry = {'world': args.world, 'kind': args.kind, 'id': args.id, 'version': args.version,
             'category': args.category, 'review_state': args.review_state,
             'source_sha256': args.source_sha256, 'original_sha256': original_sha, 'variants': variants}
    manifest['entries'].append(entry)
    manifest['entries'].sort(key=lambda e: (e['world'], e['kind'], e['id'], e['version']))
    temp = manifest_path.with_suffix('.json.tmp')
    temp.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    temp.replace(manifest_path)
    print(json.dumps(entry, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
