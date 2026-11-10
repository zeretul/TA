> 2026-11-05设计整理：当前0.6.18设计见[人物关系与通信](交付资料/设计稿/人物关系与通信.md)。下文保留原批次设计/报告，其旧版本、候选状态、计划和验证范围不代表当前交付。

# 0.6.17 AI 提案、正式关系与后续规则消费

## 本轮范围

2026-11-03，从已核验的 0.6.17 云端源码继续，读过 START_HERE、AGENTS、PROJECT、progress、交接以及 LORE/ECHO/CRAFT 已有结果。只补一条已存在的规则闭环，不另造万能判定引擎，不以旧 Git 或旧包覆盖当前树。

路径是：模型的 character_facts.relationships / 人物私聊 relationships 提案 → character.Project 规则判断 → CharacterRecord 与 Story journal → 当前人物关系 → 人物窗口、下一次对话上下文和既有突破护法资格。

## 已实现

- 新正式关系必须由主角和该人物分别说出两条不同话语，回答必须在提出之后。不能把同一句话引用两次，不能把同一人的两句话当作双方接受。
- 主故事场景的关系话语须用 addressed_to 明确指向另一方。对第三者或对全场说的话不能给主角建立或解除关系。私聊本来就有确定的双方，仍可省略目标。
- 双方任一方可先提出，非人类的明确动作仍可作为本人回应。单方断交、公开为敌不强求另一方接受。高好感仍只是原有门槛，不自动生成关系。
- 主回合预检和最终提交共用判定。非法模块返回可修正诊断，修好该模块可以继续提交，不要求把已接受模块重新生成。引用按输入位置而非临时生成ID核验，避免预检占位ID相同导致正常关系全部被拒。
- 三处实际模型提示和关系schema同步说明：合理请求、好感、沉默、拒绝或附条件回答不等于接受，不能替玩家补写同意。没有新增关系类型或另一份事实库。
- 修复同一回合多次关系变化的去重：先断交后和好、或先和好后断交，都以本回合已经成立的新事实继续判断，不因最终种类恰好等于回合前而丢掉最后变化。
- 修复既有突破消费者对整个关系历史的错误读取：已断交/敌对后不再沿用旧亲密关系给予护法加成；旧分支仍保留当时关系。关系历史保留，最新关系才是当前资格。
- 同一人物由source_key重绑到已存在Actor时，话语对象addressed_to同步重绑，避免称谓变化被误判为对第三者发言；不按同名合并不同人物。
- 旧journal按既有正式记录回放，不用新提案校验重判历史；没有迁移或改写旧存档。主角好感的直接StateUpdates已有禁止规则，人物/势力散文没有被提升为正式关系、技能或组织权力。

## 实际验证

Linux、Go 1.26.6，隔离测试数据。本轮模型调用只使用本地 httptest 协议夹具，不是自然模型体验验收。

1. 先写复现：七种错误关系提案全部被旧代码错误接受，测试真实失败，日志 `/tmp/ta-ai-rule-repro.log`。
2. 先写后续规则整链复现：正式结伴后断交、冷重开仍得到35%突破概率，和断交前相同，测试真实失败，日志 `/tmp/ta-ai-rule-guardian-repro.log`。修正后正常去除既有5个百分点护法收益。
3. 新character测试覆盖错误引用、同说话者、逆序、错误目标、含糊公开回应；合法私聊/场景双向及NPC先提出；预检重复占位ID；单方正确断交和拒绝错误对象。
4. 新interactive整链覆盖主回合预检拒绝、最终提交拒绝且无人物事实半提交、修正模块成功、正式关系、收益消费、断交、冷重开、旧分支隔离。
5. 新app协议测试覆盖模型JSON → SendCharacterMessage → 正式事实 → CharacterWindow → 下一次实际模型system上下文；单方双引用拒绝无半提交；友谊成立及随后断交均正确显示并被下一次对话消费。两项0.16s和0.43s通过，日志 `/tmp/ta-relationship-app-evidence.log`。
6. 09:46五包定向通过：character 0.002s、interactive 2.915s、app 0.676s、prompts 0.008s、tools 0.246s。包含原灵兽遇见到重开全链，灵兽动作回应未被禁用。日志 `/tmp/ta-ai-rule-targeted.log`。
7. 09:46启动五个影响包完整test，09:55全部退出0：character 0.013s、interactive 166.113s、app 310.943s、prompts 0.058s、tools 0.767s；对应vet也退出0。此全量先于后述两项小修，最终源码另有定向和vet，不能混称。日志 `/tmp/ta-ai-rule-full.log`、`/tmp/ta-ai-rule-vet.log`。
8. 复查再发现同一回合关系变化只和回合前比较的旧逻辑问题。新增“断交后和好”“和好后断交”均先真实失败，日志 `/tmp/ta-ai-rule-same-turn-repro.log`；随后修复，并于09:49启动最终定向、character全量与五包vet复验。此修正晚于第7项全量开始，最终证据分别记，不把较早编译结果伪称最后代码全量。
9. 新严格方向校验暴露同人称谓重绑缺addressed_to的问题。先新增“宋师兄→source_key既有宋观澜→正式断交”回归，真实失败见 `/tmp/ta-ai-rule-target-rebind-repro.log`；随后将话语对象和所属人物一同重绑。09:51源码静止，再跑包含该例的五包最终定向与vet。09:52确认全部退出0：character 0.004s、interactive 2.598s、app 0.565s、prompts 0.006s、tools 0.282s；五包vet无诊断。日志 `/tmp/ta-ai-rule-final-targeted.log`、`/tmp/ta-ai-rule-final-vet.log`。

## 明确边界

- 程序检查话语引用、双方身份、方向、顺序、门槛与正式持久化。自然语言是否真的表达接受仍是模型判断，不宣称能机械识别所有否定、讽刺、条件或伪造语义。模型实际稳定性、长线关系塑造未做自然试玩。
- 突破护法仍沿用已有“当前合格关系 + 同场”的有限规则；本轮只修历史关系失效，没有新增每次护法的独立意愿、能力或在场状态判定体系。不能把这项修复说成已解决所有自动服从问题。
- 第一轮未新增NPC之间的正式关系引擎或通用势力授权引擎。下节按追加要求补了主角势力身份P0；NPC之间的成员身份和通用组织权力仍不在该有限切片内。经营切片由并行工作单独实现。
- 没有修改UI布局、作者世界JSON、版本号、正式安装、玩家数据或发布。未验证自然外部模型、浏览器视觉和Windows原生运行；双module全量及完整打包由总集成单独报告。

## 修改文件

- `internal/character/project.go`、`relationship_evidence.go`、`relationship_evidence_test.go`
- `internal/interactive/turn_character_facts.go`、`cultivation_advance.go`、`relationship_rule_flow_test.go`
- `internal/interactive/turn_spirit_beast_flow_test.go`：原灵兽双向场景夹具补明确对象，动作与原验证链保留。
- `internal/agents/prompts/interactive.go`、`character_chat.go`；`internal/agents/tools/interactive_story.go`：实际提示与schema局部指导同步。
- `internal/app/character_relationship_evidence_test.go`


## 追加 N06：主角势力身份 P0（2026-11-03）

范围审计明确要求补正式／临时／待定／已离开、有权接纳与真实消费。先核查《自制-家族与宗门经营》，与经营、世界内容和见闻录工作协调后，沿已有 character_facts、CharacterRecord、CharacterState、Quest/Journey 实现，不另立身份数据库或组织管理平台。

### 实现与事实来源

- character_facts.memberships 只提出主角本人已知的变动。每个稳定 faction_id 保留历史和最新 pending / formal / temporary / left；同名不同ID不合并，left/pending不会让旧formal复活。
- formal/temporary必须有本轮真实玩家原话source_text、实际说出的定向申请、随后不同索引的本人scene回应；回应目标必须是protagonist。当前接纳权限来自本局冻结作者catalog的 membership_admission，并且人名、source_key、faction_id一致。模型角色称呼、好感、普通合作、所属势力文本不能生成权限。
- 接纳人须在回合前已经登记，实际在场，并非已归档、已死亡、行路/恢复/占用或在战斗中。首次见面先正常登记，不在同一提案中自行补出一个有权收人的NPC。同source_key换称谓会绑定既有人物及话语目标，不造第二个接纳人。
- pending仅记录主角确实已提出、尚未获准的申请；普通所属联系人可以听到请求，但没有正式权限就不能接纳。left可由玩家真实原话主动退出/撤回，或由有权本人明确拒绝待定申请；**接纳权不被推断成开除已在籍成员的权力**。暂未实现有独立来源的组织除名流程。
- 作者catalog、FrozenPerson、存档PackCatalogPerson和两条新建局路径完整保留 FactionName/MembershipAdmission。只允许formal/temporary并要求稳定ID/名称。旧冻结目录无该字段就无接纳权，不按Role猜测，不热替换老存档作者权力。
- 世界内容工作确认并标记三位：钟灵道→yue.huangfeng；葛长篙→lanchuan-o17；韩萧→xinghai-o11。只标记作者确有接纳职责的人，不给整个阵营的NPC泛授收人权。
- 建角明确确认的所属由服务端解析后写成source_kind=identity的正式CharacterRecord；内部InitialFactionMembership字段不从客户端JSON反序列化。未确认开局、无所属和单纯身份文本不会自动得权。
- CharacterWindow.faction_memberships是主角已知最新状态；见闻录直接消费同一字段，不扫描隐藏作者目录。下一轮模型上下文同步读取当前身份；接纳范围联系人只进入narrator上下文，不进入玩家WorldState JSON。
- 重生成的预检人物状态改用TurnWorldView的真实parent；被替换回合的接纳不能成为自己的前置资格。世界上下文历史重建由世界内容工作统一接线，保留同一时间点的任务、旅程和人物事实。
- 新上下文严格按字节预算保留完整行/条目，不能在membership段填满后继续无条件追加正文造成越界；预算为1字节等极端值也不截坏UTF-8或ID。

### 真实消费者与小内容

- 新增三条成员小事务：quest.huangfeng_member_market_visit、quest.canglang_member_ferry_visit、quest.blackstar_member_office_visit。只有formal可offer/accept，temporary/pending/left不行。旧任务没有被突然加资格门槛。
- 每条只要求一项真实的fresh journey_arrival，不用对白证明“已送货/已巡视”；均无奖励，不凭新文字派钱/物/好感。中英文定义齐全。离开后不能领新委托，但已有事务和已赚取的结算不会被没收。
- 澜川与星海使用已有真实连接。凡人原本只有坊市内外30分钟路，缺山门连接，新增独立版本1.0的两对稀疏适配路：黄枫谷山门内↔对外入口30分钟，对外入口↔公共坊市2880分钟（含正常休息）。**这是本次游戏适配数据，不是声称来自小说的精确距离**；旧route_036的ID、版本、时长未改。
- 新凡人连接仅对冻结catalog>=30且确有黄枫谷作者实体的局开放，不改变旧版本/其他世界。进入山门须现存formal；离开和公共道路不要求成员身份。已经按资格出发的未完成行程可以走完，不能用历史完成的路票换新入门权。内门入口资格不等于公库、指挥、私宅或其他NPC服从。

### 本轮验证状态

- 规则反例覆盖无权NPC自称掌门、旧目录、未知/冒名/不在场/死亡/忙碌/战斗、非法冻结权限、错误势力/频道/目标/方向/次序、伪造或未说出的玩家引用。合法pending、正式/临时、本人撤回和拒绝待定申请有正例。
- 主流程覆盖预检拒绝→修正→正式提交、无半提交、精确来源记录、资格影响新事务、主动离开、冷重开、旧分支、重生成移除被替换接纳以及parent上下文不见未来结果。
- 真实Store链从掌门处接纳、接下新成员事务、不能靠对白完成，到退出、实际山门出口30分钟与坊市2880分钟、fresh抵达自动完成、允许结算。路径干跑同样读取冻结catalog，不会预检拒绝而提交可走。
- app测试覆盖真实withConfirmedIdentity→CreateStory→CharacterWindow，以及未确认/无所属的身份文本不能授正式资格；修改返回窗口不改变journal。context测试验证其他world动作保留、联系人不泄露、严格预算。
- 10:25最终8包定向通过：character 0.011s、quest 0.954s、interactive 7.528s、app 1.896s、app/interactive 0.035s，storypack/prompts/tools编译通过。interactive合计含旧Journey/WorldState/灵兽整链，不是单项测试超过7秒。日志：工作区tmp-build/ta-membership-targeted.log。
- 10:26启动8影响包全量test，随后vet；实际结果见下方末次复查，不计整体成功。此前一次遇/tmp容量不足的链接失败不是代码通过，未计成功；切换已有工作区TMPDIR继续，没有删除其他工作进程的缓存。

### 仍未声称完成

- 临时身份目前只保存已知记录，没有结构化自动期限结算，因此temporary不授新成员事务或山门通行权，不能把“一日暂留”变成永久通行；有期限的实际准入需后续专门规则，不能从散文推断。
- 没有通用宗门/帮派/派系权限引擎、领导晋升、公库、任意NPC收人、NPC间正式隶属、组织强制开除或跨势力排他性判定。NPC所属仍沿见闻录安全的已知观察；主角身份是新增正式事实。
- P0的加入/退出发生在主故事真实场景，不在人物私聊窗口增加第二个收人渠道。首次见面的同回合不授正式身份；后续轮次使用已登记本人。自然语言是否真在接受、是否符合该人物意愿与世界背景仍由叙事模型判断，代码只核验可审查来源、权限、方向和既有规则，不能把这些测试说成完整自然语言真实性证明。
- 未进行外部自然模型试玩、浏览器视觉或Windows原生验证。没有改版本号、正式安装或玩家数据；整仓、双module及发布打包由总集成报告。

新增与局部接线集中在 character/faction_membership*、interactive/turn_faction_membership*、story_membership_context*、faction_membership_consumers*、membership_routes*，既有character记录/创建/主回合/任务/旅程消费者的窄接入，两个app窗口/context测试，以及quest成员资产和catalog冻结测试。源码没有另造一份事实副本。

#### 末次安全复查

- 独立复查明确临时身份无期限却放行会造成越期权限，采用最小安全收紧：temporary只留记录，不用于山门准入；该入口仅formal可用，已按资格开始的行程可结束而不产生新入门权。
- 另外先写“主角借作者key给自己收人”反例，真实失败（tmp-build/ta-membership-self-admit-repro.log）；拒绝主角及系统Actor担任接纳人后，全部新身份定向1.351s通过。此护栏晚于10:26全量起点，不能将先前全量当成最后代码验证。
- 10:26全量中的character/quest/interactive（203.546s）/storypack（85.772s）/prompts/tools通过；app及app/interactive撞到并行LocalAffairsNarratorContext签名中间态而编译失败，整体TEST_EXIT=1，**不计为全量成功**。随后8包vet已退出0。世界源签名完成后于10:31重跑interactive/app/app-interactive完整test：interactive 182.609s通过；app 328.232s因新增成员任务未纳入既有版本数量夹具而失败，app/interactive 298.257s因严格预算裁掉旧省略标记而失败。这两项均已修正，未把三包整体计为通过。


#### 历史与版本收尾（10:55 UTC）

- 审查先补真实复现：上一主回合后由战斗入口提交的仍在进行战斗，在重生成时从旧主回合checkpoint恢复而丢失，导致忙于交手的接纳人被错误放行。两项反例失败见 tmp-build/ta-membership-combat-parent-repro.log。改用真实parent journal路径的Combats，主回合最终提交与TurnWorldView一致。
- 新分支刚建立即重生成继承的接纳回合时，没有before-cache，旧身份会在内存和冷重开后继续存在。反例失败见 tmp-build/ta-membership-fork-parent-cold-repro.log。现从既有祖先来源重建准确人物/状态/计划与各领域parent，在同一次事务先落既有BranchHeadMoved checkpoint再提交替换回合；不是只修缓存。精确重试需维持同一receipt及回退事件，不重复写入。
- 版本选择原来不恢复CurrentCharacters，且克隆被舍弃主回合的CharacterRecord会把旧接纳带到新版本。现人物事实按选中版本祖先恢复，排除被舍弃主回合本身的记录，独立窗口后缀仍保留；投影独立记录的严格SourceEventID指向新envelope，原来源保留在version_projection。没有借此重写原journal或扩大其他世界领域的版本语义。
- 上述历史修正之后、最后版本选择改动之前，既有重生成/回退/分支广泛定向25.994s通过（tmp-build/ta-membership-regeneration-fixed.log）。其后受并行编译内存压力与灵界新增测试类型中间态影响的两次运行不计通过，最后定向结果另记。
- 新建角所属的固定服务端说明不再直接在英文界面回显中文：只对source_kind=identity使用本地化展示键，省去固定“开局”文本；真正场景/玩家说出的statement和world_time照原事实展示，不自动翻译或改journal。见闻录两文件10测试通过（tmp-build/ta-membership-ui.log）；中英文4811键对齐（tmp-build/ta-membership-i18n.log）。三位作者接纳人的英文faction_name由世界内容工作完成，冻结身份/权限保持不变。
- 最后8包定向中character 0.009s、quest 1.262s、app 33.840s、app/interactive 2.039s、storypack 0.929s通过，prompts/tools编译通过；interactive当时遇并行新测试StoryTurn未定义而编译失败。灵界工作已修为TurnEvent后单独重跑，最终结果见下；不能将该次整体算作通过。日志 tmp-build/ta-membership-latest-targeted.log。


#### 最后可复核结果（10:58 UTC）

- 最后interactive广泛定向 **32.988s通过、TEST_EXIT=0**：包括全部FactionMembership/Membership、新分支重生成、回退、分支、版本选择、关系和旧灵兽从遇见到重开整链。A未接纳/B已接纳四次切换，各次均保留独立私聊的原ID、来源、受众、knowledge及matter，冷重开完整CharacterState一致，journal旧字节前缀不变。日志 tmp-build/ta-membership-history-final.log。
- fresh-fork冷重试唯一性测试第一次误用readStoryRecentLocked，冷加载active页不包含audit-only的恢复事件，误报after空；改成读取完整journal后，原有同receipt、同恢复ID、无重复写入断言全部通过。未删弱断言，也不为测试改生产存档逻辑。之前失败保留在 tmp-build/ta-membership-latest-interactive.log。
- 最后8影响包vet退出0，日志 tmp-build/ta-membership-latest-vet.log（成功时为空）。该次之后没有生产Go改动；最后新增私聊后缀测试及完整journal断言已由上面的成功编译/运行覆盖。
- 最新app及app/interactive定向同时覆盖此前全量暴露的radio目录版本数量夹具和上下文完整条目省略标记，均通过。三条新成员任务是新revision的独立内容，不改96条既有星海任务的版本绑定；严格预算保留完整行并预留省略提示。
- 全局Go整仓最终全量未由本切片再次执行；不能将上述分时全量/最终定向汇总写成最后源码整仓全绿。灵界和成长扩展仍由其他工作继续，最终总体集成另验。
- 前端全仓tsc两次未能完成：768MB人工堆上限触发OOM，2048MB重试进程被SIGKILL，不能计为类型检查通过。随后以临时配置仅包含本次身份展示helper、组件和测试（及其真实导入依赖）进行类型检查，1024MB下退出0；临时配置已清理。日志分别为 tmp-build/ta-membership-ui-typecheck.log、ta-membership-ui-typecheck-final.log、ta-membership-ui-typecheck-scoped.log。中英文locale另外由4811键对齐检查覆盖。

### 后续原则

按用户重申，能由AI在现有规则内推演并可靠记录的内容继续交给AI；本切片只实现实际资格消费、来源/权限护栏、单一正式记录及历史持久化底座。没有继续增加人物意愿判定、泛化派系推演或额外万能规则层。后续应优先真实试玩与总集成验证，不能因本切片完成就把未实现的临时期满、晋升/除名/公库/组织命令列为已完成。
