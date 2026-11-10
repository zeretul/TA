# 碎冠诸境：ULTRA 双轮独立静态审查

审查时间：2026-11-03 20:05–20:29 UTC；运行证据只读复核更新至 20:40 UTC。数据/导入一致性快照：20:25:09 UTC。

## 结论

**仍为候选，不能给出发行通过。** 本轮发现的两个 P1 是正式 Conversation 遗漏新世界地方事务私有上下文，以及导入回滚在一次恢复失败后停止、留下部分已改文件却报告 NOT APPLIED。前者已最小修复，第八轮真实 Conversation 冷读/证言/分支测试已通过；后者已修复，作者运行的隔离故障注入及本审查提供的独立候选均有成功证据。

另有五个 P2、一个 P3 和一个初轮测试夹具缺陷，均已反馈并作最小修复；实际运行又揭示两处测试夹具问题，修复和历史失败一并保留。本审查复读了修后代码/数据，随后只读检查作者第六/八轮真实日志。Conversation 与七条确认建角入口均在第八轮窄重跑实际通过。第六/八轮去重为 92 顶层、661 含子例；没有把静态检查本身升级为运行通过，也没有称为完整回归。未发现需要另造世界状态库、职业系统或战斗引擎才能补救的问题。

截至快照，导入包的 37 个现有文件均匹配原三世界源码的精确 LF 归一化 base，273 个新增文件没有目标冲突，全部 overlay 哈希正确且与 sandbox 同路径内容一致。140 条路线及当前 181 个图条目独立合入，未发现覆盖旧世界 ID 的当前反例。图片快照是 60 NPC + 121/214 定义，共 241 个运行变体，**不是完整图覆盖，也不是浏览器显示验证**。

## 方法与证据边界

- 按要求读取 sandbox 的 AGENTS.md、PROJECT.md、progress.md，deliverable/README.md，既有 PLAYABILITY_AUDIT.md / REPAIR_DISPOSITION.md，CURRENT_STATUS.json。
- 轮一从 builtin/Library.Get → 建角 choices → ResolveIdentity → withConfirmedIdentity → withPackTemplate → Store 冻结，跟到 Conversation 的世界行动上下文、事务选择结算及冷读/分支。
- 轮二从对抗输入检查 RequiredTool、RequiredCharacterSourceKeys、地方事务支持范围、英文显示冻结、源树精确 base、路线/图增量、生成图保护、写失败/回滚及旧包兼容。
- 完整阅读 82 个事务选择的即时成功文案、参与人、奖励和费用。原 20 个无参与人选择中，确认 q04 的共绘图需要实际参与人；修后为 19 个独立选择、63 个有参与人门槛的选择。没有把延后且另有本人确认门槛的回响当成即时同意。
- 复查 64 技能的类型/资源/来源结构、20 配方的工具与材料、7 开局和 19 实名对手的绑定路径。目录计数、静态消费者对应关系不等同全流程自然游玩。
- **本审查没有运行 Go、Node 或浏览器，没有调用真实模型，没有寻找密钥，没有改生产源码、作者 JSON、运行图片或原三包主线。** Python 仅用于只读解析、哈希核对及写独立审查/测试候选。下文真实 Python 导入测试是作者运行的证据，本审查已读取，不冒充自己执行。

## 可行动发现及修复处置

### U1 — P1：Store 已支持新世界，正式 Conversation 仍用三个 pack ID 放行

- 位置：`internal/app/interactive/story_world_context.go:75–77`；`internal/interactive/story_local_affair.go:603–656` 的私有 narrator accessor。
- 修前输入：绑定 sui-guan-realms、冻结实际地点；正常提交 encounter，记录一个本人 witness exchange，关闭再冷读，再构建 `Conversation.storyWorldActionsContext`。
- 反例：Store 可以保存并恢复 cause/evidence，但 Conversation 的三 ID 条件不调用私有 accessor。公开 LocalAffairView 故意隐藏未验证的 cause/evidence，也没有 witness exchange ID，故正式模型回合拿不到后续核验所需的稳定事实。单测只调 Store accessor 会漏掉这一层。
- 最小修复：导出 `LocalAffairsSupported(meta)`，遍历真实登记的 journey 地点并复用结算的 `localAffairPlaceInPack`，保留旧三包原门槛；Conversation 调同一能力，不增第二套白名单。
- 当前：修复已静态确认。`fantasy_audit_regressions_test.go:99` 覆盖 unbound、仅名字、伪造地点、真实冻结地点与旧三包。
- 新回归：本审查写 `review/fantasy_conversation_local_affair_test.go.candidate`，作者已复制到 `internal/app/interactive/fantasy_conversation_local_affair_test.go`。它检查真实 pack、正式 encounter/证言、公开投影不泄密、冷读保留 cause/evidence/main witness ID、证言前分支不串该 ID。第六轮因夹具选项数遗漏未到目标断言，第七轮执行器中断无终态；补明确 ChoiceCount 后，第八轮已实际通过全部目标断言。失败/中断历史保留，见文末。

### U2 — P2：独立观察可凭空收到八银铢

- 位置：`internal/quest/assets/sui-guan.json` 的 `sg-q24.c2`，当前约 2736 行；消费路径 `story_quest_choices.go:37–41,195–211`。
- 修前输入：合格角色独自在 sg-l38，有 sg-lore、足够精力，正常 accept sg-q24 后 choose c2。选择没有 required_character_source_keys，却即时奖励 8 银铢并称当地修护金付款。成功回响里的 sg-p38 门槛不会追溯限制这次现金入账。
- 最小修复：保留独立观察；删掉八银铢及两语付款承诺，不为了付款强制召来 NPC。
- 当前：修后仅 +12 经验，仍消耗 5 精力和 1800 秒，文案明确没有约定酬劳。修前选择保存在 `review/sg-q24-c2-before-ultra.json`。
- 回归：`TestFantasyIndependentBeaconObservationHasNoRemotePayment`，正式 Store，无守塔人。**作者第六轮实际通过，本审查已核对原始 pass 记录。**

### U3 — P2：独自测绘仍由不存在的参加者赠送共绘路图

- 位置：同文件 `sg-q04.c1:374–397`；物件 `sg-shared-map` 的介绍明确有不同人的笔迹。
- 修前输入：在 sg-l04，已学 survey、持有测绘工具，现场没有任何参与人；选择成功仍写“参加测绘的人让你保留一张共绘路图”，并发放该物件。
- 根因：把观测方法归为独立行动，却没有审查即时产物的交付主体。
- 最小修复：给确实共同测绘/交图的选择加 sg-p04 实际在场门槛。若坚持独自执行，必须把产物和两语文案同时改成自己的实测图，不能只改标签。
- 当前：已补 sg-p04。`TestFantasySharedMapNeedsActualSurveyParticipant` 检查缺人在场拒绝且资源不变，再正式会面后发图。**作者第六轮实际通过，本审查已核对原始 pass 记录。**

### U4 — P2：七出身数值生效，但 auto 起点没有带到确认后的开局事实

- 位置：`storypack/identity.go:220–285,510–527,729–750`；`app/story_pack_creation.go:389–478`；`app/story_pack_template.go:58–60`。
- 修前输入：选择 sg-start-scout、affiliation none、location auto，不另写 location_text/situation。
- 路径：ResolveIdentity 正确选中模板和装备技能；但唯一 auto location 返回字面“随所选出身”。composePremise 只用 origin 描述；withConfirmedIdentity 把该 premise 写进 Origin/当前处境，withPackTemplate 因 Origin 非空不补模板中明确的“沉钟林口”。修前 scout 描述只有“沿林缘”；carer/runner 等也丢失精确起点。
- 影响：不是证明不能进游戏，而是作者宣称“随所选出身”没有把那一处准确地点交给建角/首幕；可能造成场景和路线入口偏离。
- 最小修复已采用：七个 origin 的中英描述明确当前作者起点，不另建 auto 推断系统。正式首幕仍负责把实际场景写入 Store。
- 当前：`sui-guan-realms.json:8155–8197` 已含七个精确地点；`TestFantasyEveryOriginStartsThroughProductDefaults` 已改走真实 withConfirmedIdentity → defaults → withPackTemplate，并检查 Origin 和冻结当前处境。**第八轮七个出身子例及顶层全部实际通过。**
- 专项结论：`effect_notes.origins=narrative` 本身没有造成七模板不生效。UI 目前只消费 root/aptitude/weapon/trait 的 effect_notes，未读 origins/affiliations/locations。Origin.template_id 确实选择真实 state_basis_template，所有技能/钱物仍服务端解析。这里不应因字段名就宣称“纯叙事无数值”或新增职业锁。

### U5 — P2：路线与图片增量没有纳入校验和

- 修前位置：`deliverable/apply_candidate.py` 原第 28/38 行直接 read 两份 delta；import-manifest 仅记路径。
- 复现：只把路线 delta 某条合法耗时改成另一个合法值，或改图 delta 的 source SHA，所有 overlay 的 after_sha256 均不变。修前预检仍接受未经审查的增量。
- 最小修复：manifest 增加两份 delta SHA；读 delta 前使用 secure 并验证哈希。
- 当前：`apply_candidate.py:20–24` 已实现；20:25 独立重算均匹配。作者的 `evidence/import_safety.json` 记录篡改两种 delta 均整次拒绝且目标无写入。**有真实 Python 修后证据；不属于 Go 或浏览器验收。**

### U6 — P2：备份路径绕过了树内符号链接检查

- 修前位置：原 `backup=T/'.ta-dev'/...; backup.mkdir(...)`。
- 复现前提：目标源码根合法，但 `.ta-dev` 是指向树外的符号链接。overlay 目标校验都通过，备份阶段仍向树外写入。
- 当前实际主线 `.ta-dev` 是目录，不存在已观察到的树外写入。
- 最小修复：与其它目标一致，先 `secure(T, '.ta-dev/before-...')` 再 mkdir；修后位置第 55 行。
- 当前：作者隔离 symlink 负例成功，目标不变且树外无写，见 import_safety.json。**有真实 Python 修后证据。**

### U7 — P1：一次持续恢复失败会阻断其它文件回滚，并谎报未应用

- 修前位置：写循环先 append written 再 write；except 内逐个 copyfile 没有独立异常处理，原约 63–70 行。
- 复现：第一文件写成功；第二文件部分写入后报错，且该文件恢复持续报错。倒序恢复从第二文件开始即再抛异常，第一文件完全未尝试恢复，外层只输出 NOT APPLIED。
- 影响：一个可预期的失败路径把正式源树留成混合版本，且给操作者错误结论。不是“所有磁盘失败都必须可自动救回”的要求，而是不能让一个恢复失败阻止其它可恢复文件，也不能掩盖部分导入。
- 最小修复：每个恢复独立 try；内容原样时跳过；持续失败仍继续其它恢复；最后用单独 ImportRollbackError、exit 2、PARTIAL IMPORT 和备份路径暴露未恢复目标。
- 当前：`apply_candidate.py:63–79` 已实现。作者运行完整实际 overlay 故障夹具，并运行本审查的 `review/import_rollback_failure_test.py.candidate`；`evidence/import_rollback_independent.txt` 确认第一文件恢复、第二个故意无法恢复的文件被准确报告，恢复清单保留。**有真实 Python 修后证据。**
- 边界：不是 crash-atomic 安装器；SIGKILL、磁盘损坏及人工删备份不在这些结果覆盖内。部分回滚失败仍需按备份人工处理，不能继续声称零修改。

### U8 — P3：岸边测绘选择声称已经随船

- 位置：`sg-q30.c1`，当前约 3353 行。
- 修前只检查灯塔地点，成功描述却超出该行动有证据支持的位置。未要求新造载船系统。
- 当前两语改成灯塔岸边实际可见灯区，没有到达的海面保留空白。属于叙述边界修正，不是证明航海系统坏了。

### T1 — 测试夹具缺陷，不是生产漏洞

- `fantasy_catalog_consumers_test.go:136` 的新 synthetic opponent 修前无 Location；`adventure_catalog_validation.go:308` 拒绝空地点，44 个测试会在技能消费前失败。
- 作者只给测试对手补“渡鸦桥镇”，没有放宽生产校验器。第六轮 44 方法实际通过，依据是原始 pass 记录，不是静态修复本身。

## 第二轮未发现新的具体反例的边界

1. RequiredTool：20 个实际配方工具均存在，且与对应技能工具一致；工具不是其耗材。普通 craft 检查所有权且不消费工具；equipment design 走同一工具门槛；derived craft 保留基础 recipe 字段。未发现当前内容可以用改制绕过工具的反例。旧配方字段为空保持原行为。
2. 参与人：门槛在费用、奖励和时钟提交前；questEchoPartner 检查已建立同源且唯一、实际在场、非归档/死亡/占用/旅行/恢复/战斗。已有恶意 identity 例是投影级测试，代表性 absent/success 是 Store 级；不把它们叫作所有 63 选择全跑。
3. 地方事务：能力和结算复用真实注册地点，非名字前缀准入；旧凡人界灵界扩展门槛仍保留。私有 cause/evidence 与公开投影分离；历史重写 target 仍传入相同私有 accessor。
4. 两条调查：钥匙/账册阶段依赖、实际胜利前置、物件扣除、交付地点和本人条件明确。账册交付不能在水宫直接跨到外城/黑蜡，互斥阶段避免同账双卖。古灯塔有有限料、封蜡与抄录互斥、四小时世界等待、分支/冷读消费者。此轮是静态复核，不替代既有定向测试重跑。
5. 英文：技能/物件/配方及成长基础名优先用本局冻结翻译，gen.* 仍保留玩家命名；canonical 中文没有被显示语言回写。第六轮 214 定义文本完整性测试已实际通过。对手显示名仍有依赖当前安装 pack 匹配的既有边界，不声称全部旧存档所有英文名都已脱离安装版本。
6. 图像：精确 world/kind/id/version 绑定；新包缺图不借用旧主题。attachBuiltinDefinitionArt 不覆盖已明确的非 preset 图。导入路径是内容哈希 PNG 与按 ID 合并 manifest，没有把整个原图/运行图目录覆盖过去。
7. 旧三包：family 放宽只新增 fantasy；surrender 仍拒绝 lethal/非 person；旧 required_tool/required_character_source_keys 为空的定义不多出门槛。作者正在运行更宽回归，本审查尚未取得其完整终态。

## 深度对标：可以说什么，不能说什么

- 当前 8 地区、48 地点、60 NPC、12 组织、7 出身、64 技能、130 物品、20 配方、10 市场、54 遭遇（19 实名）、38 事务/82 选择、140 路线。
- 64 技能具体为 44 战斗/恢复/机动类（attack 15、guard 11、control 6、heal 5、pilot 3、poison 2、antidote 1、energy 1），以及 10 social、10 engineering。两组不能都用“可战斗施放”衡量；非战斗者应由真实事务/制作结算。
- 按既有 18:07 基线，澜川 76 技能/155 物品/98 事务/175 选择，星海 85/167/97/166。各有 36 传授来源任务，本包主要通过 64 习本采购与学习，不应为了数量机械加 36 任务。
- 这个差异既不自动证明失败，也不支持“已完全对齐三包深度”。两个三阶段调查、六个真实时间/本人回响后续及古灯塔提供有状态分歧；更多长期角色弧线、自由输入质量、自然 AI 连续行动和完整美术仍需实际证明。
- 凡人界有独立修仙底座，不能拿 adventure 目录零项或上述数字替代对修炼/法术/法宝深度的比较。

## 运行证据与剩余限制（20:40 UTC 只读更新）

本审查独立读取原始 JSONL/exit，并自行按 `(Package, Test)` 去重，第六/八轮合计 **92 个不同顶层、661 个不同含子例通过**，与 `focused_current_summary.json` 一致。这是终态定向测试的并集，**不是一次完整包回归，不相加重复运行，也不加入没有终态的第七轮**。

### 第六轮：保留失败事实

作者于 20:25 获唯一测试窗启动八包 Go。`go-focused-sixth.exit` 为 **1**：六包通过、两包失败，75 顶层/635 含子例成功，整体未通过。quest 包通过但筛选下没有匹配测试，不能另算成已跑事务全套。

有明确 pass 记录的关键新增面：

- `TestFantasyNarratorCapabilityUsesActualRegisteredPlaces`，以及旧三包 LocalAffair 主回合/冷读/分支。
- `TestFantasyIndependentBeaconObservationHasNoRemotePayment`、`TestFantasySharedMapNeedsActualSurveyParticipant`。
- 44 个实际技能资源夹具、19 个实名绑定/出招夹具、214 条双语定义完整性。
- `TestFantasyEveryImportedImageReachesActualPNGRoute` 实际 handler 的 181 条身份/241 PNG 变体。

两个失败原因均为新增测试夹具，而不是放宽生产规则才能解决：

1. `internal/app` 编译失败：`fantasy_creation_test.go:23` 使用不存在的 `IdentityChoice.GenderID`；实际字段为 `Gender`。该包在第六轮没有运行测试。
2. `TestFantasyConversationRetainsPrivateAffairAfterColdRead` 首个 Append 失败：本审查提供的夹具只给三选项，裸 CreateStoryRequest 却未设 ChoiceCount，默认五。已明确补 ChoiceCount:3。此遗漏已记录，不能把这次失败归为生产缺陷；也不应抹去历史失败。

### 第七轮：执行器中断，永不计入通过或测试失败

作者报告执行器出现 `executor key changed`，旧会话无法恢复，持久日志当时只到 20:29:21，没有退出码/全部包终态。这是执行状态不确定，不是通过或新的断言失败。作者没有用新执行器的进程列表冒充旧任务终止证明，没有盲目重复运行。随后另行安排的第八轮才提供闭环证据；第七轮本身仍没有完成证明。

### 第八轮：两个应用包及目标边界已闭环

`go-focused-eighth.exit` 为 **0**；原始 JSONL 中 `ta/internal/app` 与 `ta/internal/app/interactive` 均有 package pass。

- `TestFantasyConversationRetainsPrivateAffairAfterColdRead` 明确 pass，覆盖真实 frozen cause/evidence、本人证言 exchange ID、冷恢复及证言前分支不串线。
- `TestFantasyEveryOriginStartsThroughProductDefaults` 的 road、learner、scout、smith、carer、grove、runner 七个子例全部 pass，顶层 pass；这次确实走 withConfirmedIdentity → defaults → withPackTemplate → Store，不是仅验证模板 JSON。
- 该轮应用层的艺术冻结、英文及配方资格定向回归已随两包成功终态完成，具体测试以原始 JSONL 为准。

因此，本轮已发现的正式 Conversation/确认入口运行缺口已关闭；不再把它们列作“尚待跑”。

### 前端定向证据

- `frontend-type-second.exit` = 0，日志无错误。作者说明该轮 Node 内存上限为 1024 MB；本审查验证退出码，不把早先 512 MB OOM 改写成通过。
- `frontend-i18n.exit` = 0，日志明确 `4933 keys across zh-CN, en-US` 对齐。
- `frontend-focused-second.json` 与 `frontend-focused-direct.json` 均 `success=true`、3/3 通过、0 失败；是同一小范围的不同执行证据，不能加成 6 个独立测试。

### 仍未完成的发行边界

- 作者当前串行执行完整根/agent/test/vet/全前端回归。**此处未读取到全部终态，不计为通过**，也不由本审查重复启动。
- 浏览器实测明确未验：IPC 被禁用及会话网络隔离阻断了允许路径。保持此限制，不重试绕过，不将 API 图片路由、类型检查或固定页面夹具冒充浏览器中英/深浅色/宽窄屏验收。
- 真实模型按用户安排留本地，云端未做自然 AI 连续游玩；Windows 原生运行及最终发行构建仍须独立证据。
- 20:25 快照仍缺 93 个定义图；后续补图须重新审核来源、最终导入包与实际路由，不能沿用 181/241 快照结论。
- 44 资源测试是合格隔离夹具，不等于 44 场赚取资格的自然战斗；19 loadout 测试主要是 resolver 绑定/出招，不等于 19 个完整 Store 会面。图片 handler 成功不等于浏览器布局或美术质量通过。

旧 35/166 与本轮 92/661 都是各自范围的证据，不加总冒充完整新版验证。本审查始终没有运行 Go/Node/浏览器，没有修改 sandbox 或干扰作者持有的测试窗。

## 交付与快照

本审查交付：

- `review/AUDIT_ULTRA_2020.md`
- `review/fantasy_conversation_local_affair_test.go.candidate`
- `review/import_rollback_failure_test.py.candidate`

复核的作者证据：`evidence/import_safety.json`、`evidence/import_rollback_independent.txt`、`evidence/authoring_rebuild.json`、`evidence/dependency_art_audit.json`，以及第六/八轮 JSONL 与 exit、`focused_current_summary.json`、前端类型/i18n/定向终态文件。五份中英世界/事务/路线作者产物的独立临时重建逐字节一致，避免把本轮修复留成无法重建的手改。

20:25:09 UTC 哈希锚点：

- 世界中文：`355c3e43d8c3312462cac193347f3003a68916ae45b841ce98146f46dc35c1e1`
- 世界英文：`d069d74007ee8a37d9d5fbba8b132892a32100f5a18057ab0bf7d9146c908101`
- 事务中文：`0358495a651247c8f30da519009affe4bfe59038eb8365c43a618ec442f4f78e`
- import-manifest：`33414c7ce66668b560a16b338d6b4947dcfb4b1246ce971130f155650e39644e`
- apply_candidate.py：`caa61b87fc6110907267321da9bf3f45644a19417ae0aeeb5f80fcec1a8b4b7e`

后续导入更多图片或改代码后，上述快照计数/哈希自然失效；必须检查最终实际交付包，不能拿本快照替代。
