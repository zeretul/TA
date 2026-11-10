> 2026-11-05文档整理：这里保留历史作者/分析工具用法，示例中的根目录“设计稿/”已是旧路径。原资料现位于[设计沿革](../../资料归档/设计沿革/0.6.18整理前/)；当前规则与内容入口见[现行设计](../../docs/交付资料/设计稿/README.md)。不要直接运行旧生成器覆盖当前四世界作者JSON；本轮未修改或执行工具代码。

# World content candidate builder

This standard-library Python tool reads the current author source JSON/CSV, checks stable identity and references, and writes reproducible derived candidates under `work/luna6-world-content/`. It does not install content, touch `.denova` data, call a model, start a service, or modify source author files.

From the repository root in Windows PowerShell:

```powershell
python tools/world-content/build.py
python -m unittest discover -s tools/world-content -p 'test_*.py' -v
python tools/world-content/verify_deterministic.py
```

The builder hashes and indexes the two author image maps. By default, it inventories only allowlisted media extensions inside the two permitted legacy roots `E:\游戏\游戏开发\凡人修仙传·人界篇\头像母图` and `E:\游戏\游戏开发\凡人修仙传·人界篇\H3生成`. Non-media files are listed by path and skip reason; their contents are never opened or hashed. Symlinks, junctions, traversal paths, and non-media references are rejected before content reads. To skip reading those roots:

```powershell
python tools/world-content/build.py --skip-legacy-media
```

Outputs are candidate artifacts, not runtime proof:

- `static-world-catalog.json`: stable `source_key` people, profile hashes, candidate locations/routes, faction references, event categories, and source hashes. Full profiles remain referenced by portable relative path; they are not copied into turn-one lore.
- `author-staging-resources.json`: all full event-library source cards plus the full Markdown bodies of the six public author cards. It is a staging resource, not runtime data. It records the disabled projection as lossy and disallows enabling it until its omitted contracts have formal consumers.
- `disabled-event-package-candidates.json`: two schema-shaped v1 EventPackageModule candidates containing six explicitly disabled projections. Disabled means not usable; projection summaries do not replace the complete source cards.
- `portrait-asset-audit.json`: separate 179 legacy NPC asset records, 8 internal static objects, and 101 formal author profiles with candidate pools keyed by stable person ID. `source_directory_group` describes only source folders and is not a scene/person pool. Candidate scenes remain unreviewed and cannot auto-select; name conflicts require manual review. It never checks legacy runtime asset directories outside the permitted roots and does not claim visual verification or runtime import.
- `baseline-hash-check.json`: compares every frozen manifest record to its frozen snapshot.
- `build-report.json`: source parsing, reference issues, and baseline summary.
- `verify_deterministic.py`: runs the builder twice and compares the two generated snapshots by SHA-256.

All JSON is emitted with sorted keys and stable list ordering. A change to any source content changes its SHA-256. The first safe implementation deliberately does not copy profile text into a Story Pack `World` field: current Story Pack loading makes those facts available from turn one, which violates per-character knowledge and discovery gates.
