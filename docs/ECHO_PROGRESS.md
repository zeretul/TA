# 0.6.17 延迟回响开发记录

## 基线与范围

2026-11-03：从已验收的 0.6.17 云端输入包继续，未恢复旧提交、未修改正式目录或玩家数据。首次阅读 START_HERE、AGENTS、PROJECT、progress 及 cloud-handoff-0617 文档。执行环境为 Linux，Go 1.26.6；版本仍为 0.6.17。

已有回响主要是 StoryClock 到时后置位并读出作者文字。本批落地有界切片：既有选择/真实完成结算 → 合理游戏时间 → 本人实际在场核实 → 来源知识与有向态度 → 后续条件消费。成熟不等于消息传遍世界，回响不额外发钱物，不创建现实计时器或第二事实库。

## 已实现

- `internal/quest/echo.go` 与既有 Choice/Instance：保存来源结果、成熟时刻、是否要求本人回应、已核实人物与时刻。普通完成任务可拥有 completion_echo，而不是为苏浅伪造 AdventureRules。
- `internal/interactive/story_quest_echo.go`：沿既有 Story journal 发成熟记录；同一时间区间内回响按正式到期时刻稳定排序。`follow_up` 要求真实历史、到时、同一来源人物、实际在场且可交谈。拒绝离场、死亡/失能、忙碌、战斗与重复核实。不复活、召回或替换人物。
- 本人核实的 QuestRecord 和 CharacterRecord 同一事务落盘；仅记录本人已核实并明确说出的有限回应及针对主角的态度。来源凭据不会把主角的隐秘做法自动当作已坦白，知识不向旁人扩散。主回合记录带同一个 TurnID。人物最新回合前快照与既有状态/时钟快照一起恢复，修复再生成后旧回应残留；可重建 projection 版本升至25，journal schema不变。dry-run也读取同一父人物快照；不授予技能、身份、组织权限或免费资源。
- 三题材作者内容由独立内容工作修改。本实现接入武侠滩涂三种处理（1/3/2日）、星海泊位三种处理（6/12/24小时）、凡人苏浅实际交接后3日回访。新个人回应须获知后才开放对应后续，不自动接任务。
- 苏浅后续复查任务使用 `fresh_facts_only`：接受前已经付过报酬的坊市旧行程不能立即完成新约定；接受后真实再次往返才能满足新目标。
- 非任务承诺/行动复用 Character Matter，增加最早回访时刻，不复用 Deadline。约定必须引用当事人本回合实际话语；到时仍需新回应才可完成，提前放弃仍可。没有到期自动完成或自动制造任务。
- `quest.VisibleChoices` 对中文、英文、未接任务目录统一过滤：未做方法的谜底、未发生结果、未核实回应和未选分支结局不会通过原始 JSON 提前泄露。只读投影不修改作者定义或 journal。
- 作者回应支持独立英文侧车；人物窗口按请求语言投影已知回应，内部事实不重写。
- 保存的旧 ChoiceResult 保留原到期时刻。缺少新 encounter 标记的旧记录继续保留原公开语义，已经记录的后续任务不因新增核实门槛被重新锁住。

## 本轮验证记录

实际命令均在隔离测试目录运行，Go 缓存单独配置；没有使用正式玩家数据。

1. `go test ./internal/quest ./internal/character ./internal/interactive ./internal/app -run 'Test(QuestEcho|QuestCompletionEcho|SuQianCompletionEcho|OrdinaryDelayedMatter|AdventureQuestDelayedChoice|AdventureQuestThirtyTurnStatePressure|AdventureQuestSecondAffairChoices|LocalizedQuest)' -count=1`：09:23 已通过实际命中的 interactive 规则、苏浅完整二次往返、普通事务、三题材压力与分支链。其余三个包该过滤器未命中测试，不据此声明它们通过全量。
2. 前序 `internal/character` 新增成熟约定规则、`internal/quest` 溢出/旧到时规则、`internal/app` 中英已知投影定向测试已通过。
3. 新增主回合 TurnID 复现先失败，确认回响 QuestRecord 没绑定实际回合；补齐 `questReq.TurnID` 后与下面全包一起复验。
4. 09:25 启动五个影响包的完整 `go test ./internal/quest ./internal/character ./internal/interactive ./internal/app ./internal/app/interactive -count=1`，09:37确认退出0，五包全部通过：0.066s / 0.015s / 156.357s / 307.310s / 166.853s。日志 `/tmp/ta-0617-echo-full.log`。
5. 新增“主回合前后分支→把核实回合再生成成聊天→冷重开”先确实失败，发现既有 reducer 缺人物回合前快照。补快照后通过；再加“前一回合→窗口建立本人→核实→dry-run→重新生成核实”通过，旧窗口谈话保留、回应不重复。日志 `/tmp/ta-0617-echo-regenerate.log`、`/tmp/ta-0617-echo-all-new.log`。
6. 上述修复后重跑 `go test ./internal/quest ./internal/character ./internal/interactive -count=1`，09:37确认退出0，三包全部通过（interactive 164.952s）。日志 `/tmp/ta-0617-echo-final-core.log`。
7. 英文任务投影、人物窗口相关 app 定向测试通过（5.219s）。最新“不能将秘密做法视作已坦白”修正后，全部新增回响、苏浅、普通事务、30回合压力、两分支及既有再生成定向测试通过（9.348s）。日志 `/tmp/ta-0617-echo-final-app.log`、`/tmp/ta-0617-echo-final-targeted.log`。
8. `go vet ./internal/quest ./internal/character ./internal/interactive ./internal/app ./internal/app/interactive` 最新退出0，无诊断。并行组织经营中间态曾短暂阻断编译/vet，待对方可编译检查点后已重新通过；没有把中间失败记成成功。

失败后补救的语义已明确并测试：更好准备且真实重新支付成本的尝试取代未核实失败回应，不强制先听一次失败对话。Previous保留原失败事实，已形成的人物事务仍为历史；只有新结果形成新的待访回应，不凭旧成熟时刻提前兑现。

已修正本轮真实失败：旧压力夹具硬编码6小时且未核实；苏浅二次行程夹具使用错误路线ID；测试误把 AppendStateDelta 当独立回合，导致重放旧回合位置；普通事务失败原子性断言混入允许保留的待处理输入。包版本资格失败由内容工作修正兼容列表和前向约束；再生成幽灵知情由人物快照及父投影修正。没有把这些失败隐藏或当作产品已验收。

## 边界与剩余

- 本批没有完成通用幕后行动模拟、任意组织传播、道路中断/势力解散的转化规则。普通 Matter 的到时只开放核实，不模拟无人见证的生产/传信/执行。
- 回响内部按成熟时间顺序提交，但既有 deadline、NPC growth 等领域仍由统一 interval 分段调用；不宣称完成所有跨领域相互依赖的全球事件优先队列。
- 旧选择结果、资源及保存时刻不重算；全局任务 Definition 尚未按旧局完全冻结，旧作者文案变化并非全部隔离。本批不声称所有旧任务未来语义完全冻结。
- 刚建立的新分支直接重写继承的最新回合时，既有 BeforeLatest 缓存未建立属于旧版本系统边界；本批不扩写该流程，无快照时不把缺省误当作“此前无人”。正常建立新回合后的再生成、独立窗口前事实与事前/事后分支已测。
- 当前验证为规则与 journal 集成测试；未做自然 AI 连续试玩、浏览器观感验收或 Windows 原生运行。全仓构建/双模块验收由整合记录分别报告。

## 交接状态

09:38：回响范围实现、定向与影响包复验完成。主要入口：`internal/quest/echo.go`、`internal/interactive/story_quest_echo.go`、`internal/character/matter_maturity.go`；各自独立测试及 `story_matter_maturity_test.go`。主回合、API中英显示、普通任务与人物投影的既有文件按职责做了最小接入。后续全仓集成可能继续有其他并行工作，最终构建以整合验收记录为准。
