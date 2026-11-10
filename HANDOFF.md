# TA 0.7.0 开发交接

面向接手的开发方。先读本文件，再读 `AGENTS.md`（工程不变量）、`PROJECT.md`（产品与架构）、`progress.md`（按日期排列的交付流水）。

本文件取代 `docs/` 下未入库的待办文档；那些文档被 `.gitignore` 的 `docs/*` 规则排除，克隆后不可见。

## 当前状态

版本 0.7.0，双 Go module（根 module + `agent/`）。前端 React 19 + TypeScript + Vite，pnpm。

源码包含 Windows 本地定制，不能当作标准基线回盖。

## 已完成

以下 26 项来自用户 2026-11-09 的试玩反馈。原文档 `docs/试玩反馈待办.md` 的复选框全部未勾选，与实际代码不符；此处按核对结果列出证据。

### 世界包与创作

| 待办 | 状态 | 证据 |
| --- | --- | --- |
| 冒险目录移除三国入口 | 已完成 | `internal/storypack/storypack.go:262` 在 `ListLocale` 中跳过 `HistoricalSanguoPackID`；`internal/api/handlers/handler_story_pack.go` 四处拒绝该 ID 的写操作 |
| 哈利与其他新增世界撤下测试/候选/裁判字样 | 部分完成 | `audience = gm` 已隔离；玩家可见 `name` 字段仍有「裁判」字样，见「未完成」 |
| 所有世界按公开资料统计，保留幕后条目 | 已完成 | `internal/book/lore/audience.go` 的 `NormalizeAudience` / `IsPublic` |
| 去掉共用的「行走其间」和玩法宣传句 | 已完成 | 全仓 `internal/` 与 `web/src/` 零命中 |
| 九套配置统一为三普通、两爽文、一自定义 | 已完成 | `internal/storypack/assets/*.json` 中 `opening_tier` 均为 `ordinary:3, strong:2` |
| 创作页分公共区与剧本包区 | 已完成 | `web/src/components/Home/WorldCreationDialog.tsx` 与 `web/src/i18n/locales/*/worldCreation.ts` |
| 新建角色去掉「悟性」选项与默认标签 | 已完成 | 产品代码仅余旧存档兼容正则 `foundation-display.ts:19` 的 `LEGACY_INSIGHT` |
| 世界局势记录概况与主要事件 | 已完成 | `web/src/features/interactive/components/director-console/OverviewView.tsx` |
| 世界局势与旧概况合为一张卡 | 已完成 | 同上，`OverviewView` 合并渲染 |

### 主角、行囊与战斗

| 待办 | 状态 | 证据 |
| --- | --- | --- |
| 功法/技能/物品详情只在主角标签下 | 已完成 | `web/src/features/interactive/components/character-window/CharacterWindowPanel.tsx` 按角色分栏 |
| 清除插画来源标签和光效免责声明 | 已完成 | `web/src` 零命中 |
| 技能介绍去掉掌握记录、条件与公式表 | 已完成 | `web/src`、`internal/agents` 零命中 |
| 凡人轻招不积经脉负担 | 已完成 | `internal/cultivation/` 规则与测试 |
| 建角基本身份取已选出身，手填文字先润色 | 已完成 | `web/src/components/Home/CharacterCreationDialog.tsx` |
| 行囊与背包合用一份物品清单 | 已完成 | `web/src/features/interactive/components/story-state/StoryStateLedger.tsx` |
| 物品分类标签，列表只显示名称与品级徽标 | 已完成 | 同上，`cultivationPossessionFields` 与 `isMethodSpecialisationField` |
| 技能与研习合为一份清单 | 已完成 | `web/src/features/interactive/components/story-state/player-panel.test.tsx:327` |

### 人物档案与游玩界面

| 待办 | 状态 | 证据 |
| --- | --- | --- |
| NPC 概况卡合并身份/外貌/性格/位置/目标 | 已完成 | `web/src/features/interactive/components/character-window/CharacterWindowPanel.tsx` |
| NPC 好感度与关系合为一张关系卡 | 已完成 | 同上，输出形如「好感：46｜关系：朋友」 |
| 浮窗缩放拖柄移到右下角 | 已完成 | `web/src/features/interactive/components/story-stage/story-window.tsx:61` 的 `ResizeEdge` 含 `bottom-right` |
| 本回合变化显示完整文本 | 已完成 | `web/src/features/interactive/components/story-state/ChangesSummary.tsx` |
| 开局后仍可打开游戏设置 | 已完成 | `web/src/features/interactive/components/setting-panel/` |
| 秘境、见闻、资料总览统一弹窗 | 已完成 | `web/src/features/interactive/components/` 弹窗组件 |
| 故事助手灵感切分析不崩溃 | 已完成 | `StoryAssistantPanel.test.tsx:70` |
| 等待短句持续轮播 | 已完成 | `web/src/features/interactive/components/story-stage/IncomingMessageHint.tsx` |

## 未完成

### 1. 紫川包「攻城」措辞自相矛盾

文件 `internal/storypack/assets/zichuan-xichuan-ta07-full-gm-candidate.json`。

- `world[1114]`（id `author-doc-zichuan-ac8993cf48e1d57b-page-005`）写明「木板展示不是攻城能力」
- `world[1206]`（id `author-doc-zichuan-e2813d17175bffa0-page-001`）却写「可侦察、依真实路线进退、攻城、在守军失去战力后依授权接管」

同一包内一处否认攻城、一处允许攻城，会诱导 AI 让玩家攻城或接管城池。用户要求清掉诱导措辞。

修改范围：`world[1206]` 的 `content` 字段，移除「攻城」及「接管」相关表述，保留侦察、进退、补给、军团战力等内容。改后需同步 `.en-US.json` 若该字段有英文对应。

### 2. 玩家可见字段残留「裁判」字样

`audience = gm` 已生效（`internal/book/lore/audience.go`），但这些条目的 `name` 仍含「裁判」，在按名称列出的界面会露出：

| 文件 | 位置 |
| --- | --- |
| `doupo-continent-ta07-full-gm-candidate.json` | `world[1009].name` = 裁判资料取用与事实提交 |
| `harry-potter-1991-student-ta07-full-gm-candidate.json` | `world[1388].name` |
| `harry-potter-1991-ta07-full-gm-candidate.json` | `world[1388].name` |
| `lord-of-mysteries-ta07-full-gm-candidate.json` | `world[2819].name` |
| `zichuan-xichuan-ta07-full-gm-candidate.json` | `world[1228].name` |
| `sanguo-190-ta07-full-gm-candidate.json` | `world[1117].name` |

另有三国包顶层措辞漏到玩家可见字段：

- `name` = 三国：汉末行路（完整裁判资料候选）
- `description` = 完整作者资料已映射到裁判专用GM资料；待受众隔离及真实状态/模型验收，不是最终可玩交付

这些是作者内部措辞。改 `name` 时需同步 `keywords` 与 `source_ref` 引用，否则按名称读取会失效。

## 三世界机制适配（WP-01—07，本轮施工范围）

2026-11-08 登记为待办，2026-11-10 用户确认 **WP-01 到 WP-07 全部要做**，不是只记录。完整分析见 `docs/三世界机制与底座适配分析_20261108.md`。要点：

**范围**：斗破苍穹、哈利波特、诡秘之主。规则与 AI 推理优先，确有结果或持久化缺口才补底座。不给三个世界分别造专用程序。

| 编号 | 内容 | 完成标准 |
| --- | --- | --- |
| WP-01 | 确定各样例采用的原作版本、年代、区域、可玩阶段、角色起点；分开原作规则、游戏化约定、未核实设定。哈利波特自由时间旅行、诡秘无限制高序列因果改写暂列范围外 | 每个样例有明确范围与不支持项；不混用小说、影视、游戏、同人规则 |
| WP-02 | 整理世界书与状态映射：斗破的境界/星级/功法/异火/炼药；哈利波特的已学咒语/魔杖忠诚/身份伪装/效果期限；诡秘的途径/序列/扮演/晋升/污染/封印物 | 每条机制说明前提、例外、代价、失败后果、需保存的事实及取用方式；先复用现有对象/列表与正式提交 |
| WP-03 | 复现条件知情缺口；按本局授权、已获线索、自定义阶段过滤资料；区分裁判知识、主角知识、NPC 所知 | 搜索、按 ID 读取、常驻上下文、人物聊天边界一致；重开/分支不串知情 |
| WP-04 | 用复方汤剂与封印物代价验证持续效果记录：开始、期限、解除、跨期等待、周期代价、多效果并存、失败重试、重开 | 用现有正式时钟与提交，不新增第二时钟；到期不漏、重试不重复、分支不串 |
| WP-05 | 验证斗破境界与诡秘序列能否用通用状态表达；确认现有数值模块可复用到哪里 | 不换名套用炼气/筑基结构；规则＋AI 足够则关闭程序扩展项 |
| WP-06 | 三个世界各做最小验证样例，执行反例清单 | 保留真实模型输入输出、正式状态、重开证据；AI 正文正确但状态错误算失败；三例分别验收 |
| WP-07 | 汇总哪些靠世界书、哪些靠配置、哪些需通用代码；改后回归四世界与自定义通用世界 | 不引入世界名分支，不泄漏专有规则，不破坏可选判定与包绑定 |

**不做的事**（不因做 WP-01—07 而放开）：三个专用战斗/升级/污染引擎、任意规则脚本引擎、自由时间旅行、无实际需求的中途顶层 schema 扩展。

**首轮反例**（随 WP-06 执行）：

- 斗破：获得异火但未炼化；未学焚诀却要求功法自动进化
- 斗破：炼药材料不足；失败后重试/重开
- 哈利波特：复方伪装期间与知情/不知情人物分别交谈，再等待到期
- 哈利波特：禁止幻影移形区域尝试移动
- 诡秘：只有玩家声明「我完成了扮演/仪式」，本局无相应事实
- 诡秘：未授权者搜索、猜 ID、聊天间接追问同一秘密
- 诡秘：携带有持续代价的封印物，跨越期限后重开和分支

## 已知约束

- **两个 Go module**：根目录 + `agent/`（`replace ta/agent => ./agent`）。`agent/` 是独立 Native Agent Runtime，不感知外部 runtime；runtime 选择与适配留在应用层 `internal/agents/runtime`
- **游戏禁止 Goal**：UI、API、执行层均不得开启
- **模型可见文本用中文**：内置提示词、工具说明、schema 自然语言说明统一中文；代码标识、JSON 字段、协议枚举、模型 ID 保持原样
- **上下文缓存**：注入片段必须有明确来源、用途、容量上限；上限需覆盖合理规模并明确超限处理
- **`.denova` 路径**：受管 Project 以稳定 `ProjectID` 为身份；`.denova` 内引用持久化为 `/` 规范相对路径，宿主绝对路径不得写入 Registry/Session/metadata
- **LLM 任务不限总时长**：不写死强制中断上限；确有安全需求时提供可配置限制并保留取消能力

## 构建与测试

```bash
# 前端
pnpm --dir web install
pnpm --dir web test:unit          # vitest
pnpm --dir web test:browser       # playwright browser
pnpm --dir web test:e2e
pnpm --dir web check:i18n         # 中英键对齐

# 后端（两个 module）
go test ./...
go vet ./...
cd agent && go test ./... && go vet ./...

# 完整构建
./scripts/build.sh                # 前端 + 主程序 + updater + ripgrep + skills
```

`scripts/build.sh` 会执行 `pnpm build`、把 `web/dist` 复制到 `internal/webfs/dist`，再以 `-tags embedweb` 编译主程序。`web/dist` 与 `internal/webfs/dist` 必须字节一致。

本仓库未包含构建产物；克隆后需自行 `pnpm install` 并构建。

## 内嵌资源不可裁剪

以下目录被 `go:embed` 在编译期硬引用，缺任何一个文件都会编译失败，不是运行时降级：

- `internal/catalogart/assets/manifest.json` 与 `assets/images/*`（1208 张 WebP）
- `internal/storypack/assets/` 全部 20 个 JSON，其中 11 个由 `assets_rule_candidates.go` 引用
- `internal/portrait/assets/`、`internal/quest/assets/` 等其余内嵌资源

## 运行环境

- Go 1.26.6+（`go.mod` 与 `agent/go.mod` 均要求）
- Node 22（见 `.github/actions/setup-web/action.yml`）
- pnpm 11.15.1（`web/package.json` 的 `packageManager`）

## 未验证

- 真实模型回合复测未执行
- 分享版（`TA-github`）未按 2026-11-10 改动更新
