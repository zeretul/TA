# 碎冠诸境：第二位独立 ULTRA 玩法消费者审查

审查窗口：2026-11-03 21:27 UTC 起。本文首次形成于 21:50 UTC，最终结稿 21:51 UTC。

## 当前结论

本轮找到并推动闭环了一处明确的玩法事实断点：三种调查结局把原件从玩家扣掉，却没有把它写入实际收件人的库存。不是只有文案欠佳，也不是用条目数量代替深度。正式 Store 的两个独立反例在修前均得到“玩家 0 / 接收人 0”，修后分支、冷恢复和重复命令检查通过。

其余重点复核没有发现第二个同等强度的当前阻断：正常开局有挣钱和跨路线学习入口，匿名敌人并非只有技能名而没有实际武器，既有组织与独行、调查、回响、灯塔探索有实际消费者。不过，64 个技能“各有某种正式消费者”不能解释成 64 种同等鲜明的战术，或 64 条自然游玩路线已经全部验证。闭纹禁咒的文案曾有具体过度承诺，现已完成不改变数值的最小修正及作者图复审；若干配方的经济价值、失败分支的可触发程度和精神防护的用途也应如实限定。

## 方法与证据边界

- 已读 sandbox 的 AGENTS、PROJECT、progress、README、deliverable/README 和前位审查 AUDIT_ULTRA_2020.md，未把已修的地方事务、参与人、灯塔工资、共绘地图、开局地点、导入器问题再次冒充新发现。
- 本审查不写 sandbox，不运行 Go/Node，不控制浏览器、不调用自然模型。独立工作为源码/作者数据静态复核、只读 Python 检查、最小反例与补丁候选。作者在共享测试窗口实际执行候选；本文引用的是已经读取的原始 jsonl 终态，不把候选当成执行结果。
- 首次所有权修复证据：evidence/handovers/before.jsonl、before.exit、after.jsonl、after.exit。后续首批边界：boundaries.jsonl、boundaries.exit。
- 既有全量/定向回归、浏览器阻断、原生 Windows 和自然 AI 游玩，分别保留原有证据边界。本报告不把某个 focused pass 写成全仓/实机全通过。

## H1：成功交付的实物没有实际接收人（已复现并修复）

### 精确输入与观察

1. sg-crypt-inquiry.stage3.c1：铁匠夹具通过真实 stage1/stage2，取得 sg-oath-key，已有且在场的 sg-p12/乔文接收归还。修前 main 分支玩家钥匙 0、keeper 钥匙 0。留钥匙的另分支应为玩家 1、keeper 0。
2. sg-watergate-inquiry.stage3.c1/c2：通过真实调查和取匣，实际旅行到外城或黑蜡下街，把 sg-drowned-ledger 分别交给 sg-p31/菲奥或卖给 sg-p34/缪特。修前交付分支玩家 0、fio 0。原测试只断言玩家扣除，没有检查目的库存。

来源：internal/quest/assets/sui-guan.json 当前约 4116、4485、4538 行；internal/interactive/fantasy_signature_test.go 原有消费断言；story_quest_choices.go 与 adventure_item_transfer.go。

### 根因

ItemCosts 只有玩家负数增量。adventureQuestOwnedRewardOps 处理已拥有独特物的正向奖励，即 NPC → 玩家，不会把负数自动解释为玩家 → NPC。所有 required_character_source_keys 也不能被当作收货人：在场的证人、参与者与真正受领者不同；绳、铁料等耗材更不应默认归对方。

### 最小修法及边界

review/handovers.patch 引入可选 Choice.ItemHandovers：[{character_source_key, items: {item_id: count}}]。

- 作者明确指定接收来源；各项必须是 ItemCosts 的正子集，跨接收人合计不超过原扣除量，同一选择不能再通过 Rewards 正向重发该物。
- 按实际扣除的原件 ID 核对，拒绝把改制品作为模板耗材后凭空给 NPC 一个未经改制的原版。
- 实际唯一在场接收人、非主角、健康、非战斗/占用，并有合法 canonical AdventureActor；无既有状态时才可从冻结 NPC 初始化，不能用初始库存覆盖损坏的已有状态。
- 沿用 owner clone、ValidateAdventureActor、actorOpsForAdventureOwner，和玩家扣除处于同一 plan.Ops。先应用奖励视图，再合并收件，防同一 NPC 先送出另一物后被旧副本“补回”。
- 带次数/冷却的物件、部署资产/机器/船不在本小路径内；拒绝而非清空状态后伪转移。
- 不用 adventureUniqueItemOwner 的冻结初始 provenance reservation 判断“当前已持有”，否则会误拦合法归还。
- 空字段不改变旧三包行为。

作者字段准确值：钥匙归还接收 sg-p12、账册交付接收 sg-p31、账册出售接收 sg-p34，数量均 1。不是按西幻 ID 在底座硬编码。

### 失败语义闭环

最初候选按旧成本语义在失败时扣除交付物。作者指出这会让新通用字段在失败分支再次销毁原件，本审查同意。review/handovers_failure_preserve.patch 只在已验证全额成本/实际原件后，对失败结果将 handedCounts 加回最终 item delta。普通耗材、费用、气力和时间照付；成功才转交，失败不写 NPC。现三条作者难度 2、最低资格能力 2，本来不会失败，但新字段不应依赖这点才守恒。

### 已读取的真实运行结果

- 修前 21:38:58 UTC：TestFantasyReturnedKeyHasActualRecipientOnBothBranches 失败，日志明确 branch=main player=0 keeper=0。
- 修前 21:39:00 UTC：TestFantasyLedgerEndingsTransferToTheirDifferentActualRecipients 失败，日志明确 receiver=fio player=0 recipient=0。修前 exit 1。
- 修后 21:40:21/24 UTC：两条 Store 测试通过，包括两种钥匙归属、不同账册收件人、分支隔离、重复命令与冷读。schema 13 子例及 Conversation 冷读亦通过，after.exit 0。
- 首批 boundaries.exit 0：8 顶层 / 37 含子例，包括上述两条、11 种接收人/物件拒绝、生成替代品/跨接收人实物总额、失败保留原件但消耗铁/钱/精力/时间、同 NPC 赠奖和收物合并、provenance 合法返还、原件不能重复交付、schema 和 Conversation 冷读。
- 此后追加“失败只保留同物件的已声明子集 / 空字段仍按旧成本全耗”2 子例，执行情况见最终补充。

## D1：64 技能矩阵的正确解释

evidence/skill_consumer_matrix.json 已写明 Static authored coverage, not proof each route was earned or used naturally。其 64/64 存在性结论成立，但不能扩大：

- 44 项战斗/防护/恢复/机动、20 项社交/制作技能。20 项分别落在真实选择、配方或工作资格中。64 习本均有具体市场，不靠凭空传授。
- choice 的引用可能是资格/成功能力，不一定是执行一次战斗招式。例 sg-q08.c1 的 sg-shield 是演练资格，不能因此宣称在任务系统额外结算了战斗盾效果。
- TestFantasyEveryCombatMethodUsesRealResources 对 44 项用的是配足资源和等级的隔离对手，断言主要是消耗/工具。其对三个 pilot 技能用 Choice=skill，会得到付费防御姿态，不能证明 mobility 已改变撤离/追击。
- 真正机动消费者存在于 adventure_disengagement.go，且战斗操作支持 escape/pursue + SkillID。为此单列 review/fantasy_movement_effects_test.go.candidate，在未改作者规则、现成铁壳与暗渠收货人对手上，最低合法等级、出身装备、实物习本实际 learn 后检查效果。是确定服务器敌意的规则测试，不是自然 AI 游玩。
- 预期实际进度：quietstep 逃离 15→51，smoketurn 15→75，unseenroad 36→100；追赶现成双火候烟步的敌人时，其撤离进度分别 100→69、100→45、84→0。测试同时检查真实精力/烟粉和无赠送攻击/胜利奖励。运行情况见最终补充。

## D2：匿名敌人不是空载技能（当前静态确认，不伪报 41 场 AI 实战）

当前 adventure_settlement.go 约 415–421 行已把匿名对手 WeaponID 放进 EnemyInventory 与 EnemyEquipment；实名对手则绑定其实际人物的库存和装备，不继承匿名兜底。

对当前匿名具技对手的 41 条 skill_id 组合，逐项复核了作者等级、skill rank、RequiredTool、武器 ID/类、实际库存，以及招式和武器共用材料的总需求。未发现初始即不可调用的组合。例 sg-op23/暗渠收货人实际持短刀、烟粉 8 和毒膏 8，刀/毒/烟步各火候 2；并非只有列出的技能名。movement candidate 直接使用它的原配烟步。

注意：部分实名人物有跨武器知识，当前只装备一种武器；不能说其所有已知技法此刻都能不换装备使用。这是知识与当前负载的区别，不是作者漏装备的证据。

## D3：辨识度与文案，不以扩底座凑深度

### 闭纹禁咒与薄霜缠足

当前两者均 control1、同导具、炭料 1、spark 前置；frost 为等级 3/学习 6h/精力 11，seal 为等级 8/12h/16。通用 control 使对方一次行动受控，并有恢复抗连续控制，未读取“正在维持的术式”或提供驱散/沉默结算。

具体误导句是 sg-seal 的“打乱目标正在维持的术式，不能抹去其知识或永久废掉天赋。”英文为“Disrupt an active pattern without erasing knowledge or permanently destroying aptitude.”

最小候选 review/seal_description_only.patch 将其改为：

“用闭纹扰乱对方的一次行动；不能解除已成立的术式，也不能永久废掉天赋。”

“Use a closed sigil to disrupt one action. It does not dispel existing spells or permanently damage aptitude.”

来源 authoring/mechanics.py:16；builder 同源生成 world 的 brief/content、gameplay.skills.description，以及英文 world brief/content、definitions.description。候选不改名称、效果与数值。此修正消除额外魔法承诺，但不会凭空令两个技能成为不同战术。更高价格而等效仍是后续平衡/辨识度弱项，不是本轮必须扩引擎的阻断。

图来源影响：按现有 source={definition,lore} 独立比较，只有 sg-seal 的来源哈希变化，c7a626cabeaff6eb2b8422ff282969fa21ce31f5ecfb9ab8a73c59537050914e → d81ab9f82f4420ddabfbcbc5867361172c71ef44404d2b3203ec14e3531c05b2。sg-manual-seal 自身定义/哈希不变，但其历史生成 prompt 引用了技能定义，需要保留历史并检查画面不表现额外能力。不能整批重写旧生成来源再冒充原始来源。作者已实际看图、复审和重新导入；本审查读取来源、复审记录及校验结果，不声称自己看过原图。

### 誓心定念

sg-resolve 有真实 guard22，另有 mental_guard45。54 个当前遭遇未登记 mental_attack/mental 攻击，64 技能也无 mental_attack；底座把该攻击类限制于 space。因此额外精神防护在现包没有相应威胁可体现。

作者描述只有“记住愿意保护的具体人，稳住受惊的心神”，没有明确承诺一条当前不存在的精神实战路线。不应把全技能消费者矩阵解释成该附加效果已有实际玩法覆盖；也不应为了凑覆盖新增精神敌人或让全部冻结图来源失效。

## D4：正常开局、代价与收益

- 七个作者出身均起始 24 银铢，各有实际已学方法、工具/装备与 parley1。其所在地有可工作市场；冷锤工坊的 forge1 门槛由铁匠原始技能满足。未发现起步无钱、无工作、又被唯一来源锁死的循环。
- 市场工作按作者服务为每小时 10–16 银铢、4–8 精力。所有 market acquisition 物件和 64 习本都有登记卖点。工作、真实买物、学习、制作、练习交锋可串起来；已有 TestFantasyEarnBuyLearnCraftAndBattle 验证其中行路人→学锻造这条解析器路径，不代表七出身全自然长局。
- 兼修不受职业锁，长期成长基础给 blade/spark/trail/firstaid/parley 多种入口，所有出身已有 parley。没有因出身缺某唯一基础而必然卡住的证据。
- 制作不等于普遍赚钱：20 配方中 12 项按当前市场原料买入、成品半价卖出会负毛利。短剑材料 36、成品买价 30、卖价 15、耗时 3h；箭 10 支材料 22、买价 20、卖价 10、耗时 1h。与之相对，行粮材料 9、卖出 21、1h，根甲材料 96、卖出 195、8h。若宣传“制作普遍是致富路径”会过度；作为学艺、自用或改制基础仍真实，不因负毛利就算玩法断点。

## D5：失败、回响、独行与入会

- 82 个选择按当前本地资格最低能力静态比较，25 项可能低于难度，57 项一旦满足门槛即保证成功。尤其高阶技能本身要求高等级，搭配较低难度会让失败文案不可达。这里不是“所有失败是假”，而是不能把 82 个选择或失败文案数量当作 82 个真实风险分岔。
- 六个后续事务依赖真实成功、世界等待及当事人回应；其本质是成功后关系/工作延伸，不是六条失败专属剧情。既有失败后回响与更好准备再试的规则有实际记录，不是任意重掷。
- 12 组织均有在地、有权限的联系人、正式/临时申请消费者。现有组织测试中 12 组织为验证器覆盖，正式 Store 的完整加入→分支→冷读→退出代表路线是烬河自由盟，不能改称 12 条完整实机组织剧情。
- 入会可开启对应工作，未入会有独立生活、市场、调查与多数地方事务，不强制入会。退出后不能领取新会务；已经接受的工作继续兑现先前约定，是当前明确语义，未将其误报成权限漏洞。
- NPC 原始计划 56 个 none、4 个 train。规范能写回实际库存、战斗结果和成长不意味着 60 人已有丰富自治日程。所有权修复提高了三条重要结局的事实完整性；不能据此宣传整个社会经济自动流转。
- 灯塔有限材料、保全/取材、记录互斥、离开/等待/返回和冷恢复已有专门 Store 测试；本轮未发现新的具体断点，也未把前位修掉的工资/岸边文案再次计作成果。

## 本轮产物

- DEPTH_ULTRA_SECOND.md：本报告。
- handovers.patch：可选交付字段、静态校验、原件原子转交、三处作者字段与叙事条款候选。
- handovers_failure_preserve.patch：失败保留交付子集的增量候选。
- fantasy_choice_handover_test.go.candidate：两条正式 Store 反例，已由作者跑出修前失败/修后通过。
- quest_choice_handover_validation_test.go.candidate：13 个 schema 子例，已通过。
- fantasy_handover_boundaries_test.go.candidate：初批已通过；追加失败子集/旧字段对照见末尾状态。
- fantasy_movement_effects_test.go.candidate：三个机动技能的逃离/追击真效果断言。
- seal_description_only.patch：来源与中英文生成文本的最小消误导候选。

## 最终补充

截至 2026-11-03 21:51 UTC，本审查范围内没有已知未闭合的 P1/P2 玩法缺陷。这里是本轮具体反例的结论，不是对全项目无缺陷或已可发布的保证。

1. 已独立读取 evidence/go-focused-finalart.jsonl 与 .exit：exit 0，43 顶层 / 274 含子例，无 fail。新增失败子集/legacy 对照在 21:47:18 UTC 通过；三个机动技能的全部六个 escape/pursue 子例及顶层在 21:47:19 UTC 通过，包含上文精确进度、真实资源与无附送攻击断言。因此 D1 的机动验证缺口已经补齐，不再只是“有扣费”。
2. sg-seal 来源与中英文生成物已实际采用修正文案，数值保持 control1、cost16、rank8、12h、原导具/耗材/前置不变。art/definition-art-records.jsonl 的 revalidation.utc 为 21:49:24 UTC；作者记录直接重看技能及习本原图通过。generation_source_snapshot/hash 与原始 prompt 保留旧来源；当前 source_hash 为 d81ab9…，原图 SHA 仍为 930488…，未重生或伪造生成历史。sg-manual-seal 原 source_hash 738a6c…未变。
3. evidence/authoring_rebuild.json、rebuild-final.json 的 21:49:43 UTC 终态 exit 0，5 项 storypack/quest/route 逐字节独立重建一致。dependency_art_audit_summary.json 为 passed=true、issues=[]，60 NPC + 207 定义 = 267/274 原图、327 运行变体；七项仍缺，不能声称全部 274 图齐。TestFantasyEveryImportedImageReachesActualPNGRoute 确有原始 pass，仅表示实际 API 处理器/PNG 路由测试，不等于打包后的 HTTP 或浏览器视觉验收。
4. 剩余 D3/D4/D5 是已明确的辨识度、经济取舍和覆盖解释弱项，不是本轮观察到的阻断。尤其不能宣传“64 种同等独特战术”“全部精神防护已对抗实际敌人”“所有制作都盈利”“60 人完整自治经济”。无须为了这些描述扩底座、扩到 98 事务或让已冻结图片无谓失效。
5. 作者正在冻结后做完整根回归；完整回归、发行打包及打包 HTTP 仍待验。浏览器/自然 AI/Windows 原生的原有边界继续保留。本文没有提前将它们标过。

关键结稿锚点（SHA-256）：
- story_quest_choices.go：e049700fcda77a5383f3d854ab261a4693e618568a02f457f793006116374b66
- adventure_item_transfer.go：8cb438408e29ae59985850790fec0eb6ba6235f01463a82cf1d2eebe7cfeede7
- quest/choices.go：01d36054cc83fe6d4132f0c95d94c9b407fc552ac8684ba53d738c75563600ff
- storypack/assets/sui-guan-realms.json：188e41463b1e99fdcaca3302f939021d54cddbccda07f0241850d31539c84893
- storypack/assets/sui-guan-realms.en-US.json：2e4ff40f285e78f86039cacafb565a22be8173a8b640fa42ec69eef1630ae6e5
- quest/assets/sui-guan.json：94f67b174bc8bc6277a5a2d1f194a42816de4760433a3e5cd5400a7200a300f6
