> 2026-11-05文档整理：这里保留历史作者/分析工具用法，示例中的根目录“设计稿/”已是旧路径。原资料现位于[设计沿革](../../资料归档/设计沿革/0.6.18整理前/)；当前规则与内容入口见[现行设计](../../docs/交付资料/设计稿/README.md)。不要直接运行旧生成器覆盖当前四世界作者JSON；本轮未修改或执行工具代码。

# Economy balance analyzer

This standalone command reads the versioned economy candidate JSON and runs the storage-free calculations in `internal/economy`. It writes a deterministic JSON report, Markdown report, and diagnostics JSON. It does not edit the candidate or connect to game state.

Run from the repository root with the repository's Windows Go toolchain:

```powershell
$env:GOCACHE = (Resolve-Path 'work/luna6-economy/gocache').Path
$env:GOTMPDIR = (Resolve-Path 'work/luna6-economy/gotmp').Path
$env:GOTOOLCHAIN = 'local'
go run ./tools/economy-balance `
  --input '设计稿/资源清单/凡人人界_经济尺度候选_20261015.json' `
  --output 'work/luna6-economy'
```

Use `--input` to select another candidate and `--output` to choose an output directory. Source paths in the candidate and the analyzer's supporting references must resolve inside the repository. Missing or unsafe source references are recorded as diagnostics. Any diagnostic is written to the output files and causes a nonzero exit status.

The report records SHA-256 hashes for the candidate and the design references used. Its `kind` explicitly identifies the result as independent candidate analysis, not runtime validation. Quotes are not trades; budget checks do not authorize or deduct spending; reward idempotency, transaction atomicity, game saves, and combat are outside this command's scope.

## Portable test inputs

The unit tests use explicitly synthetic, self-contained fixtures and temporary output directories. Historical author drafts/catalogs are not included in this source distribution. CLI defaults still refer to those historical inputs and report a missing-input diagnostic unless the actual sources are supplied; the tests do not claim to validate those excluded author documents.
