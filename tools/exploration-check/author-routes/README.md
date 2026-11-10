> 2026-11-05文档整理：这里保留历史作者/分析工具用法，示例中的根目录“设计稿/”已是旧路径。原资料现位于[设计沿革](../../../资料归档/设计沿革/0.6.18整理前/)；当前规则与内容入口见[现行设计](../../../docs/交付资料/设计稿/README.md)。不要直接运行旧生成器覆盖当前四世界作者JSON；本轮未修改或执行工具代码。

# Author route candidate converter

Run from the repository root with the repository Go toolchain:

```powershell
go run .\tools\exploration-check\author-routes
```

The command reads `work/luna6-world-content/static-world-catalog.json` and the manifested author handoff source `设计稿/资源清单/凡人人界_首版衔接清单_20261015.json`. It writes `candidate-routes.json`, `report.json`, and `diagnostics.json` under `work/luna6-exploration/q1/`. Candidates are review artifacts marked `not_installable`; this tool does not install or modify game data.

The converter uses stable `places` IDs for both authored places and network nodes, keeps source versions, route IDs, direction, elapsed minutes, and author restrictions, and never infers missing connections. It validates each converted route through `internal/exploration`. Invalid rows are reported while independently valid candidates remain in the output; any diagnostic makes the report invalid and the process exits with code 2. A catalog override is accepted only from the Q1 output directory for local negative-input checks.

## Portable test inputs

The unit tests use explicitly synthetic, self-contained fixtures and temporary output directories. Historical author drafts/catalogs are not included in this source distribution. CLI defaults still refer to those historical inputs and report a missing-input diagnostic unless the actual sources are supplied; the tests do not claim to validate those excluded author documents.
