# 敌人行动 AI 接入进度

2026-11-03，基于当前 0.6.17 开发源码，未安装或修改正式游玩目录。

## 已实现的应用层

- 战斗结算之前调用现有模型服务 `modelio.NewChatModel(...).Generate(...)`，沿当前故事的模型配置与 thinking level；这不是将固定规则排序改名为 AI。
- 每个敌人分别得到一次独立请求。输入只有领域层提供的本人性格、动机、底线、态度、本人资源与可用候选、已观测战况；不拼接完整 Story、正文、玩家当前尚未执行的动作或其他人的私密菜单。
- 模型只返回 `candidate_id`。应用层拒绝未知候选、额外动作/数值、重复键、尾随正文与多对象；允许修复一个包围 JSON 的代码围栏。没有读取模型 reasoning 字段来替代最终选择。
- 输入菜单上限 256 KiB，超限明确降级，不裁掉一部分技能后声称完整。仍执行现有 provider 输入容量校验。
- 凡人战斗和武侠/星际独立战斗窗口均先检查命令回执，再决策与提交。凡人公开 App 入口不再允许客户端显式操纵敌方行动。冒险 API 已把请求 context 传至模型调用。
- 模型不可用、非法回复或上下文过大时保留原有规则安全降级；日志分类为 `fallback_unavailable`、`fallback_invalid`、`fallback_context_limit`，部分敌人成功则为 `ai_partial_fallback`。正常为 `ai`。用户取消返回取消错误，不按模型失败继续结算。
- 同一 App 中的相同 Story/命令串行进入准备、模型和提交，等待支持取消，完成后清理；等待结束再次查正式回执，防并发重复点击多付模型费。
- 决策准备返回已解析的真实分支，模型配置、调用与提交均钉住该分支；空分支别名不能在模型等待期间因切换存档分支而改写另一分支。诊断 trace 的消息副本清除 ReasoningContent，正常候选输出不受影响。
- 共用提示词协议增加 `enemy_decision` JSON 模式，避免主故事的纯正文协议与决策 JSON 相互矛盾。

## 已接入的规则边界

领域层提供合法候选、独立知情快照、锁内候选复验、expected head/round 防过期及正式回执。该部分由同批领域修改负责；应用层不会自行定义 SkillDef、扣资源、决定数值或写状态。冒险首切片支持普通攻击与现有真实结算的已学招式，不虚构尚未实现的 NPC 逃跑、投降或额外库存动作。凡人多目标招式提供单目标及按每个主目标确定性展开到目标上限的组合，不声称穷举所有目标排列。

## 幂等范围

已提交命令重试读取正式回执，同一进程内并发重复命令只允许一个模型决策段。模型返回后、正式提交前若进程崩溃，重启后可能再次请求模型；当前没有持久化未提交的模型决策草稿，因此不承诺跨崩溃窗口的模型调用恰好一次。正式数值仍由原子回执防止重复结算。不同 App 进程之间仍由 Store 防止重复提交；进程内的调用门闩不承诺跨进程模型费用去重。

## 验证状态

已新增测试源码：严格输出解析、真实 HTTP 模型替身、并发重复命令仅调用一次与等待取消/清理、逐敌知情隔离、部分非法选择降级、缺模型与取消区分、容量上限、reasoning 不作动作；凡人 App 选择生效/重复命令不再调用/模型调用中状态改变拒绝/取消不扣回合；武侠与星际 App 选择低伤试探而非固定最高伤害并真实扣费/重试/旧头拒绝。

Linux Go 串行验证已通过，日志位于 [enemy-ai-validation](enemy-ai-validation/)：

- 新功能聚焦：`go test ./internal/app ./internal/agents/prompts ./internal/api/handlers -run 'EnemyDecision|CombatRoundUsesModel|CombatModelDecision|CombatCanceledDecision|AdventureRoundUsesModel' -count=1`。App 3.796s，prompts 0.009s；handlers 编译通过，该正则没有匹配 handler 测试。
- 领域聚焦：`go test ./internal/cultivation ./internal/interactive -run 'Test(EnemyActionOptions|EnemyDecision|AdventureEnemy|AdventurePeriodExpert|AdventurePeriodRecovery)' -count=1`，分别 0.003s、0.258s。
- App 战斗回归：从 `combat*_test.go`、`enemy_decision*_test.go` 和 `adventure_window_app_service_test.go` 提取的 111 个测试入口全部通过，45.842s。完整入口列表见 `app-test-names.txt`，不把该范围冒充整个 App 全量。
- `go vet ./internal/app ./internal/agents/prompts ./internal/api/handlers ./internal/cultivation ./internal/interactive` 退出码 0，无诊断。

本次先记录失败再更正测试前提：新增冒险夹具最初缺少技能学习来源，补齐真实手册目录；旧已结束战斗、吃丹观测和准备跨重启测试原用客户端声明替敌人选行动，现在由 HTTP 模型明确选择对应合法候选；旧护体观测测试原依赖弱护体必然排第一，现由模型选择护体。C02 求饶文本重用改为玩家本人合法发出的求饶。未为保留旧夹具而恢复客户端操纵敌人，也未删除断言。`app-before-fixture-update.log` 保留吃丹旧前提的实际失败，后续 `app-regression.log` 是更正后的通过记录。

HTTP 替身通过只证明配置、协议和实际规则接线，不能冒充自然模型行为质量验收。复制件未含真实模型凭据；自然模型策略质量、多敌长期行为与 Windows 原生试玩仍未验证。规则全包、最终源码锚点及后续同批领域投影小修以总交付验证为准。

## 集中最终验证前的补修

在上述111项回归与vet完成后，新增实际分支钉死和诊断reasoning清除的窄修，并增加 `TestEnemyDecisionPinsAdventureBranchWhileModelCreatesFork`：假模型调用中从同一个head建立并切换fork，要求只原main推进，fork不变。这一新增测试及最新源码由同批最终串行验证执行；上面的111项与应用源码哈希属于补修前的验证快照，不能作为补修后已通过声明。

随后完成叙述指令隔离：决策提示词编排只获得数值上下文预算，不继承默认/故事/自定义叙述者指令；新增整次HTTP messages私密canary测试，仍保持原模型及候选。公开冒险action API现在要求显式branch，避免跨切分支重试重解析空别名；新增missing/empty/whitespace/explicit分支handler回归。上述补修已gofmt，尚未单独运行Go测试，仍交同批最终串行验证。

## 2026-11-03 15:02 UTC independent boundary corrections

The focused final checks passed after five audit findings were corrected:

- Own cultivation-method mana reduction is applied once, including the actual selected cast's payment.
- The resolved branch is pinned across model generation and commit. Adventure HTTP commands require an explicit branch; internal callers retain current-branch alias support for the duration of one call.
- A valid held pill/ward declared in anticipation of damage may turn out unnecessary. That spends the action, keeps the consumable, and records the chosen enemy turn rather than allowing a free model reroll. Missing, exhausted or unqualified items remain initial input errors.
- Enemy model system messages do not inherit narrator custom instructions or workspace lore. A full HTTP-message canary test covers both default and interactive narrator secrets.
- Generated skill references are checked against the actual learned set and resolved to the frozen definition before model-time declaration validation. A generated self-targeted shield now reaches the same real settlement as an ordinary learned skill; unlearned techniques stop before calling the model.

Validation: cultivation 0.012s, interactive 0.607s, prompts 0.011s, handler 0.051s; final App AI regression 5.185s. The first combined run failed only because the new branch test tried to append narrative during an active battle. It was corrected to start a real encounter in a turn checkpoint; both failed and successful logs are retained in enemy-ai-validation. These focused passes do not substitute for the pending independent broader review. Source hashes are in audit-source-sha256.json.

Cancellation is honored before entering the Store commit. Once atomic mutation has begun, cancellation does not promise to undo it. The durable receipt remains the source of truth for retry.

New pursuit/ceasefire and personal-versus-vessel gameplay requested afterward are separate pending implementation, not part of this completed AI-selection checkpoint.
