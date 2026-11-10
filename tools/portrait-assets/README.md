> 2026-11-05文档整理：这里保留历史作者/分析工具用法，示例中的根目录“设计稿/”已是旧路径。原资料现位于[设计沿革](../../资料归档/设计沿革/0.6.18整理前/)；当前规则与内容入口见[现行设计](../../docs/交付资料/设计稿/README.md)。不要直接运行旧生成器覆盖当前四世界作者JSON；本轮未修改或执行工具代码。

# Offline portrait asset review

This tool inventories the two approved legacy media roots and the workspace candidate folder. It builds a local review page, a CSV coverage matrix, content hashes, and derived 128/192 pixel thumbnails. It does not write production portrait bindings or access runtime, Story, account, or model services.

## Run

From the repository root on the configured Windows machine:

```powershell
python tools/portrait-assets/cli.py build
python tools/portrait-assets/cli.py import-review review-export.json
python tools/portrait-assets/cli.py export-defaults default-choices.json
python tools/portrait-assets/cli.py verify-defaults
```

`build` uses fixed input and output roots. It only reads media files with supported image/video extensions from the approved old-media directories and reads the repository's author catalog, static package catalog, portrait audit, and portrait index. It skips links/junctions and reports unreadable media. Thumbnails are cached by source SHA-256, thumbnail parameters, and Pillow version; unchanged files are not rewritten. Duplicate hashes are reported and never merged automatically.

`import-review` accepts a JSON path relative to `work/luna6-portrait-assets`, validates its schema and fields, and writes `review-import.json` in that same work directory. It accepts only workspace-relative asset paths under the `legacy/` or `assets/` roots. `approved` is preserved only when explicitly present in the input. The command does not modify production data.

## Outputs

- `work/luna6-portrait-assets/review.html`: offline bilingual review page with filters, expandable people, media previews, scene labels, review states, JSON import/export, and diagnostics.
- `inventory.json`: source fingerprints, author/catalog joins, scanned media metadata, hashes, thumbnails, and diagnostic records.
- `coverage.csv`: one row per author profile, including catalog coverage, old images/videos, unmatched/conflict status, generation candidates, and runtime status.
- `package-snapshot.json`: fixed hashes/declarations captured once for the main and assigned Claude static package files; it does not probe runtime import.
- `review-export.json`: initial versioned mapping. Every item starts pending and unapproved.
- `diagnostics.json`, `cache.json`, `derived/`: scan diagnostics, thumbnail cache, and local thumbnails.

The runtime import state is always recorded as `unknown_not_verified`. The page and exported mapping are review aids only. A filename, profession folder, or visual resemblance is not identity evidence. Old media candidates remain unreviewed until a person checks both identity and scene suitability. When ffmpeg and ffprobe are unavailable, videos are listed without a fabricated poster.

## Portable default portrait package

`export-defaults` reads `work/luna6-portrait-assets/default-choices.json` and requires exactly one authorized choice for every formal entry in the 101-person author directory. It exports normalized PNG originals and 128/192-pixel thumbnails under `assets/npc-portraits-default/<stable_id>/`, then writes `manifest.json` with profile and asset hashes, source provenance, visual-inspection notes, and separate human-review and game-install states. Supported sources are inventory-listed legacy media, generated candidates with matching metadata, and exact person-specific or explicitly selected shared images from the scoped portrait audit.

The choice authorization permits a default selection without setting `human_approved`; the manifest keeps `user_human_review` as `not_reviewed` and `game_install_status` as `not_installed_not_verified`. Exporting does not alter game runtime bindings. `verify-defaults` checks directory coverage, unique IDs, profile and image hashes, image decoding, expected package paths, and orphan files.

## Checks

```powershell
python -B -m unittest discover -s tools/portrait-assets -p "test_*.py"
```



## Browser preview (loopback only)

For a real browser review, first confirm the chosen local port has no listener. From the project root run `python -B tools/portrait-assets/preview_server.py --port 18765` in the foreground, then open `http://127.0.0.1:18765/review.html`. Stop the server with Ctrl+C when finished. The server binds only to loopback and serves the review page plus exact inventory-listed media and derived thumbnails; it does not expose directory listings, arbitrary project files, writes, or uploads. It supports byte ranges for inventory-listed MP4 files.
