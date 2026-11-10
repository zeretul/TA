# 0.6.17 见闻录与人物信息续工

## 已实现
- 复用玩家已知 Snapshot 与 CharacterWindow，无新API、无新持久库、无作者目录扫描。
- 玩家和NPC详情显示明确记录的所属势力；势力页列已知NPC及最后获知位置，并可跳转详情。
- 人物页增加关系图和完整列表：精确Actor ID、不按重名合并；箭头保留方向；正式社交关系优先同类旧文本，但亲属/师徒等其他关系可并存；不从好感推定关系。
- 大型关系图分组显示，每组六个相邻人物；完整列表保留全部已知关系。节点支持键盘与详情跳转。
- 当前同场位置标本次所见并展示场景时间；离场位置保留最后记录，不套用其他字段更新时间伪称实时。
- 现有Actor模板补两个有明确知情约束的字段：所属势力、已知人物关系。NPC私密关系字段仍私密。没有迁移或重写旧局。
- 新增19个中英资源键及标准字段标签，继承黑白主题与自适应容器。

## 实际验证（Linux 云端）
- pnpm 11.15.1 离线 frozen-lockfile 安装成功，815包复用、零下载，锁文件未变，SHA256：3c1d79f9f42e8e25600631cbf9d671957684db81535bf0e858ace21e1010025f。
- `pnpm --dir web exec tsc --noEmit` 通过。
- `pnpm --dir web test:unit src/features/interactive/components/lore-overview src/features/interactive/components/story-state/builtin-display.test.tsx --maxWorkers=2`：5文件45项通过。含见闻录37项，覆盖同名ID、未知不泄漏、默认值非事实、方向、正式关系优先与师徒并存、图分页/列表、精确跳转、空数据。
- `pnpm --dir web check:i18n` 通过，当前协作树4772键对齐（包含并行代理的资源改动）。
- `go test ./internal/interactive -run 'TestKnownSocialFactsCommitReopenAndFork|TestSnapshotWithholds|TestTurnCharacter|TestStoryTurnPublishesCharacterFacts' -count=1` 通过。新增集成链以正常StateUpdates提交归属/关系/位置，重开和分支分别保留正确旧/新事实。
- `go test ./internal/app -run 'TestLoreSocialProjection|TestCharacterWindow|TestPlayerProjection' -count=1` 通过。新增投影测试确认公开见闻可见而NPC私密关系仍不出界，原始状态未变。
- `go vet ./internal/app ./internal/interactive` 通过。

## 浏览器验证缺口
`TA_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium pnpm --dir web exec playwright test --config=playwright.lore.config.ts` 的中英×深浅×390/1440八组合全部在Chromium启动前失败，原因是容器socket权限，不是UI断言失败。申请require_escalated后同样受限。

官方云浏览器localhost连接拒绝，已登记终端主机被浏览器客户端策略阻断；未继续尝试其他网络路线。两个临时Vite服务已停止。保留独立playwright.lore.config.ts、tests/lore-social.spec.ts与tests/fixtures/lore-social*作为可复现合成数据检查入口，目前不能声称实际页面验收通过。

## 仍有的边界
旧局若只有身份散文/Tags，不猜归属。缺少明确字段时显示尚未了解；新增标准字段默认用于新模板，旧局须由正常状态/schema提交建立相应公开见闻，未强行改写冻结schema。原私密关系记录不能作为关系图的数据源。关系图只连已经认识的目标，未发现人物不露出。自然AI是否稳定记录这些见闻、长期游玩与Windows原生未验证。完整构建和全仓回归由父任务统一进行，不以本定向结果代替。

## 修改文件
- internal/interactive/actor_state_preset_fields.go
- internal/interactive/lore_social_flow_test.go
- internal/app/lore_social_projection_test.go
- web/src/features/interactive/builtin-display-labels.ts
- web/src/features/interactive/components/lore-overview/model.ts、LoreOverviewPanel.tsx、known-people.ts、known-people.test.ts、RelationshipView.tsx、RelationshipView.test.tsx
- web/src/i18n/locales/zh-CN/loreOverview.ts、en-US/loreOverview.ts
- web/playwright.lore.config.ts、web/tests/lore-social.spec.ts、web/tests/fixtures/lore-social-data.ts、lore-social.tsx、lore-social.html

没有改动CharacterFacts协议、世界包作者JSON、真实存档、正式安装目录或版本号。打包时排除node_modules、test-results及临时Vite缓存。


## 2026-11-03 审计续补
- V02：已改为势力记录稳定键/faction_id连接，不按标题合并；同名势力保持不同实体。未知旧名称不猜ID，显示身份未核实。主角正式归属读取CharacterWindow.faction_memberships，每个ID正式记录优先，待定和已结束不算当前成员；缺记录时只建该已知ID的最小条目。NPC的formal/temporary/pending/left分组显示。
- V03/V04：新增结构化已知位置字段；位置和关系均展示获知时刻、公开来源类别、获知方式、可信度及对话引用。缺信息标未记录，不给历史位置套当前时间。传闻/推测不能升级为确认；关系图用虚线标未确认或缺可信度。正式关系复用原world_time及expression/answer引用。
- API边界：专门知识过滤读取真实CharacterState已知名单；秘密target不下发；证据只允许玩家确实可见的已提交exchange ID，混合/未知/私有引用整条隐藏，自由证据原文丢弃；知识字段默认值不下发。历史/流式知识写入因无当时知情上下文而保守省略，原journal保持不变。已加真实snapshot/history/stream反例。需要明确：这验证结构和来源，不是对任意模型location/kind文本的语义真假证明。
- 一致性边界：读取同批校验story/branch，并在已知context_revision/turn变化时清空旧视图、丢弃旧响应。不同API尚无共享提交token，Promise.all不是事务；恰好在请求期间发生的未通知并发提交仍可能跨版本。本轮未声称实现原子快照。
- 仅消费安全knowledge.location或本次同场观察；不再回退裸presence/lifecycle位置，因为它可能是主角未获知的后台更新。旧局缺公开知识记录时显示未知。通用Snapshot原有公共位置字段全产品规则没有在本轮改写。
- API知识读取还在CharacterState之后复读Snapshot，核对context_revision和branch head；若期间发生新发现，旧snapshot不给使用新名单。已加入旧快照→提交新认识人物→拒绝旧快照名单的反例。
- 单方疏远/敌对记录在关系图画无方向的“已记录状态”，不假装双方互相敌视；正向正式关系保留双向，显式已知非正式关系保留方向。
- 最新前端8文件55项通过，双语4806键对齐（包括并行资源）；新增DOM元数据/未知状态/同名身份/branch响应/知情来源反例。interactive重开/分支定向及vet通过。锁文件SHA仍与原基线一致。
- 服务端最终 `go test -vet=off ./internal/app -run 'Test(Lore|Player|Interactive.*Projection|EmitInteractiveTurnPersisted)' -count=1`：18项通过，其中本轮新增8项；随后 `go vet ./internal/app` 通过。测试与vet拆开执行是为了避免并行文件编辑干扰，最终两项均已实际通过。
- 服务端app全量在追加前状态通过（316.779s）；其后追加的revision fence、affiliations及API正向反例由上述最终定向覆盖，不把早先全量当最终追加版覆盖。
- 全仓tsc在10:10前多次通过；随后仅未核实成员分组、显示来源边界和搜索关键词调整，最后全仓复查连续被SIGKILL/137（包括直接node限制768MB堆），无类型诊断输出，不能称最后全仓复查通过。已停止重复全仓尝试。最后使用extends原tsconfig、include见闻录目录和builtin-display-labels的临时配置，以 `node --max-old-space-size=512 ./node_modules/typescript/bin/tsc --noEmit -p .tsconfig.lore-verify.json` 定向类型检查成功退出0；临时文件已删除。此结果不替代全仓tsc。
- 浏览器仍沿先前已确认环境阻断，未再次尝试绕路。fixture已更新以覆盖元数据及正式归属，仍不能声称真实浏览器验收通过。


### 本轮追加文件范围
- 新增前端 known-affiliations.ts/test、known-provenance.ts、ProvenanceDetails.tsx/test、AffiliationList.tsx、use-lore-overview.test.tsx。
- 更新现有见闻录model/panel/known-people/relationship view及测试、use-lore-overview、双语loreOverview资源、types.ts的只读DTO、builtin字段标签、合成浏览器fixture。
- 新增服务端 internal/app/lore_knowledge_projection.go/test；更新 interactive_player_projection.go、interactive_app_service.go、interactive_agent_app_service.go 的知识投影调用以及现有lore_social_projection_test。
- 更新 Actor知识模板字段指导与internal/interactive/lore_social_flow_test持久化、重开、分支元数据验证。正式Membership核心事实写入由另一个协作任务负责，本任务只消费CharacterWindow安全投影。
