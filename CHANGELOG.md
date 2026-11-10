## [Unreleased]

### Added
- 新增独立历史经营入口，以及斗破、哈利波特成年/学生、诡秘、三国、紫川六个世界配置；真实模型验收状态见本地交付报告。
- Added the historical simulation destination and six authored configurations for five new worlds; see the local acceptance report for live-model verification status.
- 四个冒险世界新增具有正式身份、等级与资源的强势出身；旧故事继续使用已保存初态。
- Added established, well-resourced origins to all four adventure worlds; existing stories retain their saved state.

### Fixed
- 修复自定义世界核心规则遗漏、开局状态无法适配及裁剪后世界包重载失败；校验缺失参数和失效的知情限制引用。
- Fixed missing core rules, opening-state adaptation and reload failures in custom worlds; reject missing gameplay data and broken knowledge restrictions.
- 回合判定可在兼容选项中选择并显示世界推荐，切换会同步实际判定且保留角色状态。
- Check systems can be selected from compatible options with a world recommendation; changing them updates active checks while preserving character state.
- 修复同名地点导致跨世界路线串用；四个世界按本局冻结的包数据读取既有玩法，新增与自定义世界无需登记程序版本白名单。
- Fixed cross-world travel through matching place names. Worlds now use their own frozen gameplay data without a program-maintained world revision whitelist.
- 修复 Windows 目录联接入口无法打开工作区、同一目录被登记为重复项目的问题。
- Fixed workspace access and duplicate project registration through Windows directory junctions.

# 0.7.0

- AI生成NPC可凭正式保存的成年年龄参与资格检查；原生目录年龄、在场与异性条件继续生效。
- 通过既有人物记录补充NPC间亲属、师承、朋友及伴侣关系，并保持正式主伴侣边界。
- Generated NPCs may use their persisted adult age for eligibility; authored ages and existing presence/gender checks remain authoritative.
- Existing character records now carry consistent family, mentorship and partnership rules.

---

# 0.6.18

- 个人制作支持暂停、保存与续作，开工投入材料，完成后取得成品；重开或重试不会重复扣料、重复发放。
- 补齐普通行路、黄枫谷出入口及太岳灵潮的英文展示，修正小时、时辰和短时长的显示换算。
- Personal crafting now retains unfinished work across saves and resumes without charging inputs twice or issuing products early.
- Added English labels for ordinary routes, Huangfeng Valley entrances and the Taiyue Spirit Tide, and corrected elapsed-time display units.

- 新增碎冠诸境，整合四世界玩法、210 张专属配图、无损 WebP 和“古今星海”品牌。
- 见闻录聚合读取同一次提交，战斗回执记录敌方决策来源。
- Added the Shattered Crown world, 210 new illustrations and lossless WebP art.
- Read the journal from one committed snapshot and retain enemy decision provenance.

---

# Changelog

Text Adventure 仅在此记录用户可感知的重大功能、重要不兼容或数据变更、安全更新，以及影响核心流程的关键修复。内部重构、测试调整、文案修改和细节级 UI 优化不再逐项列出，完整记录可查看 [Git history](https://github.com/alfredxw/denova/commits/master)。

Text Adventure records only major user-visible features, important compatibility or data changes, security updates, and fixes affecting core workflows. Internal refactors, test changes, copy edits, and minor UI polish are omitted; see the [Git history](https://github.com/alfredxw/denova/commits/master) for full details.

`Unreleased` 以最近一个本地交付版本（当前为 0.7.0）为比较基线，只描述升级用户最终可感知的净变化；内部接口、实现重构和 v0.5.2 后从未发布的中间格式不计入。

`Unreleased` compares against the latest local delivery (currently 0.7.0) and describes only the final user-visible delta. Internal APIs, implementation refactors, and intermediate formats never released after v0.5.2 are excluded.

格式参考 [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)。

## [Unreleased]

## [v0.6.18-dev.20261104] - 2026-11-04

### Brief / 简要说明

- 独立四世界开发候选，包含碎冠诸境玩法修正，以及真实模型敌方决策回执和已知情资料展示修正。
- 新增210张专属配图，catalog资源改为逐像素一致的无损WebP并保留旧图片地址；采用已选“古今星海”品牌。
- Isolated four-world development candidate with Shattered Crown gameplay corrections, enemy-decision receipt handling and known-information library fixes.
- Adds 210 dedicated illustrations, pixel-identical lossless WebP catalog assets with previous image URL compatibility, and the selected Ancient-to-Starsea branding.
- 仍有17项缺图；不代表自然模型长线、Windows原生或完整平衡验收。Seventeen entries remain unillustrated; natural-model long runs, native Windows execution and complete balance are not certified.


## [0.6.18-dev.20261103] - 2026-11-03

### Brief / 简要说明

- 在原三世界继续增加正式成长和玩法消费者：凡人高阶与有限灵界，武侠/星海八阶段人物、装备、实际起手五系/兵器内功及有限跨尺度交战。
- 关系与势力身份、共同制作、延迟回响与地方事务复用 Story 正式事实；配图按世界和版本精确绑定。仍是独立开发候选，长期内容、平衡、自然模型和 Windows 原生运行不在已完成范围。
- 修复取消外部运行时提问或权限请求时，中断和工具回执偶发丢失的竞态；仍等待服务端结束确认，不把取消当成工具已完成。
- Continues the original three worlds with formal higher-stage growth, a limited spirit-realm slice, eight-stage adventure characters and equipment, real starting disciplines, and bounded cross-scale encounters.
- Fixes cancellation racing with external-runtime tool and rejection replies, while retaining the provider terminal acknowledgement requirement.
- Relationships, faction membership, joint crafting, delayed echoes and local affairs consume committed Story facts. Artwork is bound to exact world and content versions. This remains a development candidate, not a claim of complete long-term balance or native Windows/natural-model validation.


## [0.6.17] - 2026-11-03

### Brief / 简要说明

- 游戏顶部提供新开一局、切换存档和删除存档；存档列表覆盖所有可用世界，删除前明确确认角色和故事。
- Added visible New game, Load game and Delete save actions, with saves from all available worlds and explicit deletion confirmation.

### Fixed / 修复

- 删除当前或最后一个存档后，界面不再保留已删除的选择。
- Clear stale selection after deleting the current or final save.

## [0.6.16] - 2026-11-03

### Added / 新增

- 新增三题材同场协作：亲查线索后可与真实在场且愿意的人物共同研判，双方支付真实成本，方案只服务本次探索；可独行、拒绝或只分享部分结果。
- Added optional local cooperation in all three worlds: share a discovered clue, agree on one concrete task, pay actual participant costs, and use the plan once in that exploration. Solo alternatives and selective reporting remain available.

- 三世界新增可选择方法的局部探索：真实工具与技能改变路线，保全或拆取留下不同回访结果，发现侧路后可重复利用捷径。
- Added optional exploration places in all three worlds, with real preparation costs, persistent salvage/preservation choices and discoverable shortcuts.

- 新增澜川群侠传与星海争锋世界包，提供题材专属的成长、装备、战斗与任务规则，选择的后果随故事时间保存与推进。
- 制符接入学习、材料消耗、制作与战斗使用；世界作者可保存制作方法，并在新故事中冻结使用。
- 人物与事务关注随故事分支保存，并参与上下文选取；可展开查看已提交检定的公开依据。
- Added the Lanchuan and Starsea worlds with genre-specific progression, equipment, combat, tasks, and consequences that persist with story time.
- Talisman crafting now connects learning, materials, production, and combat use. World authors can save crafting methods for new stories.
- Character and task follows persist with story branches and inform context selection. Committed checks expose a read-only view of their public evidence.

### Fixed / 修复

- 普通劳动按冻结来源结算工资、精力与正式时间；改制装备继续进入学习、交易、制作、战斗、任务和无线电消费者。机械耗尽后可正式败退，非致命武侠切磋可认输。
- 六个题材出身显示各自真实初态；修复切换世界的迟到错误与长内容/窄屏建角布局。
- Ordinary work now settles authored wages, effort and time. Adapted items retain their original consumers; exhausted vehicles can resolve defeat and non-lethal wuxia duels allow surrender.
- Genre origins preview their actual initial state; creation handles late errors and long content on narrow screens.

- 游戏设置编辑目标独立于正在游玩的故事，并隔离切换目标时迟到的保存响应。
- Game settings keep their editing target separate from the story being played and isolate delayed save responses after target changes.

本工作树为 `0.6.16` 开发候选，验收记录随批次交付；没有替换正式游玩安装。

This checkout is the `0.6.16` development candidate. Verification is recorded with its delivery; it does not replace the installed game.

## [v0.6.12] - 2026-11-02

### Brief / 简要说明

#### 中文

- 聊天合并为一个入口：会话列表带真实预览与稳定排序，私聊与群聊共用同一套窗口；故事窗口可以浮在整个界面上，也可停靠在正文所在的那一列。
- 私聊里商定的行动先暂存，与主选择一起发送，真正进入下一回合，不再说完就散。
- 灵感按钮给出的是现有选项里没有的方向，而不是把已有选项换个说法。
- 技能与功法合并成一个「技能」列表；施招受限时说明真实原因，伤势就写作伤势；状态面板读作几处真实差异，不再罗列改写过程。
- 新回合结束后落点是正文开头，不是回合末尾的页脚；对敌窗口只显示玩家已经知道的信息。
- 开局不再从预设库里挑人：要么由模型生成，要么玩家自定义。年龄可直接输入，出身与所属联动，非法组合由后端拒绝。
- 内置世界固定在一个明确的时间点，人物与内容随附英文显示文本，产品名称统一为 TA。
- 生图修正了发给图像模型的配置字段，会把故事里已经露过面的人物一并交给模型；出图失败时说明是哪一步没成，而不是静默返回空。
- 叙事设置去掉每局单独补充，中文文风收敛为六种；游戏助手的能力按用途收窄，不会写到别的世界去。
- 资料库分「世界包原生」与「本局新建」两页；新增条目默认归当前世界并按需加载；规则不再出现在资料库里。
- 本局新建的人物、地点、物品有独立的设置与图片入口，可增删改，重开后仍在。
- 修复中文标签保存后被补上不可见空字符的问题。
- 创建故事时选择了互不兼容的规则系统与状态系统会被直接拒绝并说明原因，不再被悄悄改写成另一套组合。
- 英文页面里人物对白也跟随页面语言，不再出现叙述是英文、引号内却全是中文。
- 聊天里的资源链接会直接落到持有该资源的设置分节，不再停在页面顶部。
- 修复在资料库页面直接刷新后「本局新建」页签变灰、必须绕回游戏才能恢复的问题。
- 删除旧预设管理相关界面，只保留游戏实际读取的预设数据与入口。

#### English

- Chat is one entry point: the conversation list shows a real preview and keeps a stable order, and private and group conversations share the same window. Story windows can float over the whole application or dock in the column holding the prose.
- An action agreed in a private conversation is held and sent with the main choice, so it actually reaches the next turn.
- The ideas button offers directions the current choices do not already contain.
- Techniques and cultivation methods are one 技能 list; a blocked technique says why, a wound is called a wound, and the state panel reads as a few real differences instead of a list of writes.
- A finished turn lands on the start of the prose, not on its footer. An opponent's window shows only what the player already knows.
- A story starts from the model or the player, never from a preset library. Age is typed in, origin and affiliation stay linked, and impossible combinations are refused by the backend.
- The built-in world is pinned to one moment and ships with English display text; the product is named TA.
- Image runs send the configuration under the field the image model actually accepts, include the faces the story has already shown, and say which step failed instead of returning nothing.
- Per-story narrative supplements are gone and the Chinese styles settle at six. A scoped assistant's writes stay inside its own story and world.
- The lore library splits into built-in world content and entities created in this story; new entries default to the current world and load on demand; rules no longer appear there.
- People, places and things created in this story have their own settings and picture entry, survive reopening, and can be added, changed or removed.
- Fixed Chinese labels being padded with invisible characters on save.
- Choosing an incompatible rule system and state system is now refused with an explanation instead of being quietly rewritten into another pair.
- On an English page, spoken dialogue follows the page language instead of staying in Chinese inside the quotes.
- Resource links in chat land on the settings section that holds that resource instead of stopping at the top of the page.
- Fixed the 本局新建 page staying greyed out after a direct reload of the lore library, which used to need a detour through the game.
- The old preset management screens are removed; the preset data and entry points the game actually reads are kept.

## [v0.6.6] - 2026-10-29

### 中文

- 修复原信已送达后无法保存后续书面回信的问题；人物窗口显示原信正文、实际收件状态及关联回信。
- AI首次创建的灵兽可登记自己的身份和已支持的吐纳功法，按自身状态成长；交战对手缺少数值角色时会要求补齐，避免故事已经开打而战斗窗口无人可战。
- 无等级成长的通用世界不再混入等级字段；世界包属性与关闭战斗的设定在加载、分支和重开后保持一致。
- 回合正式接受后出现额外工具调用时，仅恢复已接受的正文；无法恢复时保留错误，不展示未经确认的新正文。

### English

- Written replies can be saved after the original letter has already been received. The character window shows the original text, receipt and linked reply.
- Newly generated spirit beasts can register their own identity and supported breathing method. Declared opponents must have rules-backed characters before a fight is accepted.
- General worlds without level growth no longer inherit a level field. World attributes and disabled combat remain consistent across loading, branches and reopening.
- Unexpected tool calls after turn acceptance recover only the accepted narrative; missing retained prose remains an error.

## [v0.6.4] - 2026-10-28

### Brief / 简要说明

#### 中文

- 自创世界可在开始生成前确认人物状态、数值尺度、成长、战斗风险和开场，并绑定实际支持的通用行动或修仙规则。未接入的题材玩法会明确显示为不支持，通用人物不再默认套用侦探身份。
- 支持按世界设定寄出玩家来信。信件随正式故事回合保存，送达时间由故事时间推进决定；来信与关联回信可在人物窗口查看，并在重开后恢复。

#### English

- Custom worlds now confirm character state, numeric scale, growth, combat risk and opening before generation, then bind to a supported general-action or cultivation ruleset. Unsupported genre mechanics are identified, and general protagonists are no longer defaulted to detective roles.
- Players can send letters according to a world's communication rules. Letters are saved with committed story turns, delivery follows story time, and the sent letter and linked reply remain available in the character window after reopening.

## [v0.6.3] - 2026-10-27

### Brief / 简要说明

#### 中文

- 灵兽现在会按正式规则成长。以前世界书里写着「灵兽」、也有登记的吐纳功法，但故事一旦开局，它就不再被当成灵兽：登记信息在开局冻结时漏掉了物种和功法两项，之后每回合问「这只是不是登记过的灵兽」都答否，于是永远不涨修为。开局时把这两项一起带上即可，已开的故事不受影响。
- 取人性命这件事，现在只认玩家或对手自己当回合说出口的那一句，并且要指名道姓对着谁。之前只核对了「这句话存在」和「谁说的」，没核对「冲着谁」，于是玩家问一句「为什么有人要杀我」也会被记成玩家要杀人，对手说「我要杀那个旁观者，你先走」也会被记成对手要杀玩家。现在说话的对象也必须对上，对不上就不记，有歧义一律按不致命处理。
- 死亡仍然只由战斗规则的真实掉血决定。开战那一刻把双方取命意图冻结下来，之后照常打，血量归零才死；打输、逃走、认输都不算。

#### English

- Spirit beasts now grow by the real rules. The world book already said 灵兽 and named a registered breathing method, but once a story started the beast was no longer treated as one: freezing the catalog at story creation dropped its species classification and its method, so every later turn answered "not a registered spirit beast" and it never gained progress. Both fields now travel with the freeze; stories already in progress are untouched.
- A lethal intent is now accepted only from the exact public line the player or the opponent said that turn, and only when that line names who it is aimed at. The check used to verify that the line existed and who spoke it, but not who it was aimed at, so a player asking "why would someone want to kill me" was recorded as the player declaring a killing, and an opponent saying "I will kill that bystander, you leave" was recorded as that opponent threatening the player. The direction must now match; when it does not, nothing is recorded, and anything ambiguous is treated as non-lethal.
- Death is still decided only by the combat rules' actual damage. Both sides' intents are frozen when the fight opens, the fight plays out normally, and only a target whose health reaches zero dies; losing, fleeing and surrendering do not count.

## [v0.6.2] - 2026-10-27

### Brief / 简要说明

#### 中文

- 修复持续生灵回访时未能稳定沿用同一人物身份的问题。登记后，同一主体可在后续故事、相同命令重试和冷重开后承接已记录身份与经历。

#### English

- Fixed recurring companions losing their stable character identity. Once registered, the same character identity and recorded experiences persist across story revisits, identical-command retries and cold restarts.
## [v0.6.1] - 2026-10-27

### 中文

- 任务只展示已经获知的事情，个人委托不再因为人物被收录就提前出现；公开求药消息有明确来源，接取、期限与分支恢复继续走同一份故事记录。
- 外置图片和视频统一按世界、人物、场景与活动筛选。普通人物页不显示私密活动素材，动态人物的视频也能在自己的故事里稳定绑定。
- 修复视频播完后再播仍被静图遮挡、失败后无法重试的问题；普通故事回合不再自动换片或重播。运行素材清单剔除了错误关联到未成年人物的活动片段。

### English

- Tasks show only known opportunities. A personal commission is not revealed merely because its giver is in the world catalog; discovery, acceptance, deadlines and branch recovery use the same story records.
- External pictures and clips share world, character, scene and activity filtering. Ordinary views exclude private material, and generated characters can keep clips within their own story.
- Fixed replay being covered by a resting image and retry after playback failure. Ordinary turns no longer reshuffle or restart clips. Private-activity bindings to an under-18 character were removed from the runtime media set.

## [v0.6.0] - 2026-10-26

### Brief / 简要说明

#### 中文

- 头像、立绘、场景图和视频改从 `game/media` 读取，程序里只留 101 张默认头像做缺图兜底。以后添加或替换素材只要放文件、改清单，不用改代码、不用重新打包。
- 人物窗口里多了「选择图片」和「恢复默认」：选一张本地图片（PNG / JPEG / WebP），程序把文件复制进 `game/media`、自动登记归属并立即显示，不需要 Go、Node 或重编译 EXE。
- 归属按身份绑定，不按名字猜：世界包里的人物用稳定 `source_key`，故事里现场生成、没有 `source_key` 的人物用「故事作用域 + actor」，重开仍是同一人，也不会串到别的故事或别的世界包。
- 清单只存相对路径：整个游玩文件夹搬到另一个路径、换一台电脑仍能显示；图片坏了或格式不支持时回退默认图，故事照常继续。
- 视频正式接入：当时实际清单为129段，覆盖17名NPC，按内容哈希去重。旧交付稿的133段为误计；v0.6.1再排除5条错误绑定后为124段。默认静音、单次播放，播放本身不推进世界时间、不改变好感、不扣资源。
- 没有合适素材时仍显示原头像和文字，不会出现空播放器。程序更新只替换程序和 `web`、`skills`、`tools`、`licenses`，不会动 `game/media` 和你的 `.ta-game`。

#### English

- Avatars, portraits, scene images and videos are now read from `game/media`; the program keeps only the 101 built-in default avatars as a fallback. Adding or replacing material is a file plus a manifest entry — no code change, no rebuild.
- The character window gained "choose a picture" and "restore default": pick a local PNG / JPEG / WebP, the program copies it into `game/media`, records who it belongs to, and shows it at once. No Go, Node, or recompiled EXE required.
- Material is bound by identity, never by display name: world-pack characters use their stable `source_key`, while characters generated during play use the story scope plus the actor, so they stay the same person across a reopen and never leak into another story or another world pack.
- The manifest stores relative paths only, so the whole play folder still shows its pictures after being moved or copied to another machine. A broken or unsupported file falls back to the default picture and the story keeps going.
- Videos were wired in with 129 actual clips across 17 NPCs, de-duplicated by content hash. The earlier count of 133 was incorrect; v0.6.1 excludes five misbound clips, leaving 124. Playback is muted and single-play by default and does not advance world time, change affinity, or spend resources.
- With no suitable material the original avatar and the text are still shown — never an empty player. Program updates replace only the program and `web`, `skills`, `tools`, `licenses`; `game/media` and your `.ta-game` are left alone.

## [v0.5.9] - 2026-10-26

### Brief / 简要说明

#### 中文

- 同一个人对主角的好感，在状态详情和人物窗口里显示成同一个数了。以前这两处各读一份记录，实测每个角色都对不上，其中一人一处写 12、另一处写 -3，方向都是反的。
- 好感现在只有一个来源：故事结算会写下这次互动为什么改变了关系，数值由后端算。正文里直接改好感不再被接受，会提示改走互动记录。
- 已存档的故事重开后，两处读数同样一致。

#### English

- How one character feels about the protagonist now reads as the same number in the state details and in the character window. Those two surfaces used to read separate records, and in the acceptance run every character disagreed — one of them read 12 in one place and -3 in the other, opposite in sign.
- Affinity now has a single source: the settlement records why an interaction changed the relationship and the backend computes the value. Rewriting the number directly in prose is no longer accepted and points at the interaction record instead.
- Existing saves read consistently in both places after a reopen.

## [v0.5.8] - 2026-10-26

### Brief / 简要说明

#### 中文

- 身上佩着一把下品青锋剑时，再买一柄同名中品剑会被当场拒绝，灵石一分不扣；以前会先收钱，卸下时两把都变回下品。
- 两件同名但不同品级的东西同时存在时，各自保留自己的品级与来历，不再互相覆盖。
- 局域网访问自动生成的用户名改用 `ta-` 前缀，自己填过的用户名不受影响。
- 分享包里不再有原作者微信收款码：外置页面和纯程序内嵌页面都不再返回那张图。
- 本版按本地更新器要求的 `TA/` 根目录与 `TA-v0.5.8-windows-x64.zip` 文件名打包，可直接用程序内的「从本地选择新版包」升级。

#### English

- Buying a second sword that shares a name with a worn one of another grade is now refused before any payment; previously the money was taken and unequipping collapsed both blades to the lesser grade.
- Two items that share a name but not a grade now keep their own grade and origin instead of overwriting each other.
- The auto-generated LAN access username now uses the `ta-` prefix; a username you entered yourself is left alone.
- The share package no longer carries the upstream author's WeChat donation code: neither the external page nor the pure embedded build returns that image.
- This release is packaged with the `TA/` root and the `TA-v0.5.8-windows-x64.zip` filename the built-in local updater expects, so it can be installed from the app's "select a local package" flow.

## [v0.5.7] - 2026-10-26

### Brief / 简要说明

#### 中文

- 装备了几件同款法器再卸下一件，剩下的都还在包里，不会再只剩一件。
- 手里已经有一件同名东西时再拿到同名但不同款的那件，规则会直接拒绝，而不是把先有的那件悄悄顶掉。
- 新建世界页在没有选中工作区时先给空状态，不再先发一次请求再报错；切换工作区时迟到的旧响应会被丢弃。
- 没配模型时的提示改成游戏的说法，不再写「能写作的模型」。
- 白术、韩立、无名剑客的公开简介改成经历与表现，不再是规则说明或改编备注；旧世界打开时会换成新版。

#### English

- Unequipping one of several identical items leaves the rest in the bag instead of collapsing to one.
- When a different item shares a name already held, the rules refuse it rather than silently replacing the first one.
- The new-world page shows an empty state instead of firing a request before a workspace is selected, and a late response from a previous workspace is dropped.
- The hint shown when no model is configured now speaks about the game rather than about a writing model.
- Three character descriptions now read as experience and bearing rather than rule notes or adaptation remarks; existing worlds pick up the revision.

## [v0.5.6] - 2026-10-25

### Brief / 简要说明

#### 中文

- 新装的数据目录改叫 `.ta-game`，随包工具在 `.ta-toolchain`；已经装过的旧数据目录照样能打开，不会另起一份空存档。
- 独立写作工作台、小说导入、章节卷状态与书籍导出这些不属于本游戏的入口，从界面到后端一起删掉了。世界快照照旧能读。
- 同一场景里发生的事，当时在场的人都记得，不在场的人不会因为被写进名单就跟着知道。私密内容的读者按实际在场判定。
- 装备槽只在角色真的用得动那件东西时才算数；买了几件同款再装备一件，剩下的还在包里，不再整叠消失。
- 亲密空间的成员由参与者自己答复决定，群聊界面不能替他们改名单；同意的话记在他们当时所在的房间里。
- 人物年龄随本分支过去的整年推进，成人判定和出场资料读的是同一个年龄。

#### English

- A fresh install keeps its data under `.ta-game` and its tooling under `.ta-toolchain`; an existing older data directory still opens, so upgrading does not start an empty save.
- The standalone writing workspace, novel import, chapter/volume status and book export are gone from the interface through the backend. The world snapshot still reads.
- What happens in a scene is remembered by everyone who was there, and nobody is handed it just by being named in a list. Who may read a private record is decided by who was actually present.
- An equipment slot counts only when the character can really operate what is in it; equipping one of several identical items leaves the rest in the pack instead of consuming the whole stack.
- Who is in an intimacy space is decided by the participants' own answers, and the group surface cannot edit that roster; an agreement is recorded in the room it was given in.
- A character's age advances by the whole years this branch has spent, and the adult check reads the same age the cast block states.

### Changed / 变更

- 新安装的数据根改为 `.ta-game`，随包工具链在 `.ta-toolchain`；已有的 `.denova` 与 `.nova` 数据目录继续沿用，不会另起一份空存档。
- A fresh installation now keeps its data under `.ta-game` and its toolchain under `.ta-toolchain`. Existing `.denova` and `.nova` data directories are still used, so upgrading does not start an empty save.

### Removed / 移除

- 独立写作工作台、小说导入、章节卷状态与书籍导出等不属于本游戏的入口已从界面、接口与后端一并移除。世界快照读取接口保留。
- The standalone writing workspace, novel import, chapter/volume status and book export are gone from the interface, the API and the backend. The world snapshot endpoint remains.

### Fixed / 修复

- 同一场景里发生的事，当时在场的人都能记住，不在场的人不会因为被写进名单就得知；私密内容的读者按实际在场判定。
- What happens in a scene is remembered by everyone who was there, and nobody is handed it merely by being named in a list; who may read a private record is decided by who was actually present.
- 装备槽只在角色确实用得动该物品时才计入数值；阶段读不出来或没有登记定义时不再凭空给值。买下多件同种物品再装备一件，其余几件留在包里，不再整叠消失。
- An equipment slot contributes its numbers only when the character can really operate what is in it; an unreadable stage or an unregistered item no longer grants a value on assumption. Equipping one of several identical items now leaves the rest in the pack instead of consuming the whole stack.
- 亲密空间的成员名单只由参与者自己的答复决定，群聊界面不能替他们增删；同意的话记在他们所在的房间里，同室的人记得这一刻。
- Who is in an intimacy space is decided only by the participants' own answers, and the group surface can no longer edit that roster. An agreement is recorded in the room it was given in, so the people there remember it.
- 人物年龄按本分支已经过去的整年推进，成人判定与出场资料读到的是同一个年龄。
- A character's age advances by the whole years this branch has actually spent, and the adult check reads the same age the cast block states.

## [v0.5.5] - 2026-10-24

### Fixed / 修复

- 修正角色详情在缺少说明时显示内部定义编号的问题；内置修仙世界现以“凡人修仙”列出。
- Character details no longer expose rule IDs when descriptive fields are absent. The built-in cultivation world is now named 凡人修仙.

## [v0.5.4] - 2026-10-24

### Brief / 简要说明

#### 中文

- 玩家读到的文字改成正常游戏描述：资质写成「悟性一般，偏好拳脚」，技能与物品详情显示名称、来历与实际效果，不再出现定义 ID、schema 版本或原始 JSON。
- 资料总览、任务、秘境与战斗提示里的工程腔长段落改成简短提示；人物世界书逐条区分人物设定与作者指令，NPC 按各自的经历和分寸说话。
- 状态详情里的生命、灵力、伤害、灵石、数量、消耗、期限与修炼进度全部保留，不做取整。

#### English

- Text the player reads now reads as a game: aptitude as 悟性一般，偏好拳脚, and skill and item details show name, origin and actual effect instead of definition IDs, schema versions or raw JSON.
- Engineering-tone paragraphs in the lore overview, quests, realms and combat hints are now short prompts; character world-book entries separate character setting from author instruction, and NPCs speak from their own history and restraint.
- Life, spirit, damage, stones, quantities, costs, deadlines and cultivation progress are all still shown in the state detail, with no rounding.

### Fixed / 修复

- 资质行是创建时写下的记录，修炼规则每回合从中读档位。旧档里的措辞像一条规则说明而不是人物描述，现在只在显示时改写，存档值不动，档位与修炼速度不变。
- The aptitude line is written at creation and the cultivation rules read the tier word out of it every turn. Characters made earlier carry phrasing that read like a note about the rules; it is now rewritten for display only, leaving the stored value, the tier and the cultivation rate untouched.

- 世界书条目的「setting」「history」类型统一归到「世界观」书架，不再以原始类型名出现在资料总览里。
- World-book entries typed setting or history are filed under the 世界观 shelf rather than showing their raw type name in the lore overview.

- 思考强度使用已有中文标签，故事线使用可读名称，缺失的技能、物品与原因改用自然中文兜底，内部值仍写入日志。
- Thinking level uses its Chinese label, story lines use readable names, and missing skills, items and reasons fall back to natural Chinese while the internal value still goes to the log.

## [v0.5.3] - 2026-10-24

### Brief / 简要说明

#### 中文

- 人物聊天草稿在关闭窗口后不再丢失，换人、换分支时仍然清空。
- 势力记录会写明它写于哪个回合、当时是什么时刻；过了很久再读到时，会明确告诉你这条记录已经不是现在的样子。

#### English

- A character-chat draft is no longer lost when the window is closed; switching person or branch still clears it.
- Faction records now state which turn wrote them and what moment that was; reading one long afterwards says plainly that it is no longer the present.

### Fixed / 修复

- 人物窗口关闭时整块卸载，草稿原本只存在面板自己的状态里，回来就是空的。现在草稿与「当前打开的是谁」一样由宿主持有，面板只读写宿主的值；换人或换分支仍然清空，因为草稿属于某一个人、某一个分支。
- Closing the character window unmounts it, and the draft used to live only in the panel's own state, so it came back empty. The draft is now held by the host the same way the open person is, and the panel reads and writes that value; switching person or branch still clears it, because a draft belongs to one person on one branch.

- 一条记录不会被每回合重述。十年前写下、此后再未改动的「当前行动」，对不知情的模型仍是现在时。现在写下它的回合与当时的分支时刻会记在记录上，读取时一并给出，并说明这条记录写的是那个时候的样子。
- A record is not restated every turn. A 当前行动 written a decade ago and never touched since still reads as present tense to a model that was never told otherwise. The turn that wrote it and the branch instant it stood at are now recorded with it and reported when it is read, along with a plain statement that the record describes that moment rather than now.

- 这句话只对确实早于当下的记录说。当回合刚写下的记录不再自称「不一定是现在的样子」，因为每回合都出现的告诫会被当成套话跳过。
- That sentence is said only about a record that really predates the present. A record written this turn no longer calls itself out as possibly stale, because a warning that appears every turn is one the narrator learns to skip.

## [v0.5.2] - 2026-10-24

### Brief / 简要说明

#### 中文

- 修复工作区读取，打开已有书不再报 `Project response scope mismatch`。
- 修复修炼与 NPC 成长的小数进度丢失：成长记录里写了、人物快照里却没有。
- 修复人物聊天窄屏布局，375×860 下发送按钮不再被长事项和历史挤出屏幕。
- 修复中英本地化检查的三条 Skill 键放错命名空间。

#### English

- Fix workspace loading so opening an existing book no longer reports `Project response scope mismatch`.
- Fix lost fractional progress in cultivation and NPC growth, where the growth record was written but the character snapshot did not reflect it.
- Fix the character chat layout on narrow screens, where long matters and history pushed the send button off screen at 375×860.
- Fix three Skill keys that the Chinese and English localization checks reported under the wrong namespace.

### Fixed / 修复

- 工作区读取不再请求已删除的 `/book/tree` 与 `/book/summary`。两条读路径共用仍注册的 `/book` 完整快照取得 tree 与 summary，项目身份校验保持不变，另一种项目的快照仍被拒绝。
- Workspace loading no longer requests the removed `/book/tree` and `/book/summary`. Both reads take tree and summary from the still-registered `/book` snapshot, the project identity check is unchanged, and a snapshot from another project is still rejected.

- 修炼与 NPC 成长按实际数值判断是否变化，不再把 0.77 到 0.867 这样的小增量当成没有变化筛掉。同一天整段结算与拆成 24 个一小时得到同一结果，成长记录与人物快照一致；重复区间不重算，同一条命令重试仍是重复。
- Cultivation and NPC growth now compare real values instead of rounded ones, so a small gain such as 0.77 to 0.867 is no longer discarded as unchanged. Settling a whole day at once and settling it as 24 separate hours now agree, the growth record matches the character snapshot, repeated intervals are still not recomputed, and retrying the same command is still a duplicate.

- 从已结算的回合开出分支时，同门的小数进度跟着过去，不再只剩成长水位而没有进度。
- A branch opened from a settled Turn now carries the same fractional progress its companions had, instead of inheriting the growth watermark with a progress of zero.

- 人物聊天把消息、事项、历史与商议放进同一个剩余高度的滚动区，输入与发送栏留在外面，窄屏下始终可见。草稿、聊天记录与重试能力不变。
- Character chat puts messages, matters, history, and negotiation into one scroll area that takes the remaining height, leaving the input and send bar outside it so they stay visible on narrow screens. Drafts, chat history, and retry behaviour are unchanged.

- 本地化检查报告的三条 Skill 键从 `skills` 命名空间移到 `api` 命名空间，键名与文案不变，中英两种语言同步。
- Move the three Skill keys reported by the localization check from the `skills` namespace to the `api` namespace, keeping the same keys and wording in both Chinese and English.

## [v0.5.1] - 2026-09-28

### Brief / 简要说明

#### 中文

- 减少长会话在历史压缩、恢复和复用外部运行时会话时的开销。
- 修复 Native Agent 工具审批与暂停恢复，以及外部运行时取消排队写入后无法继续会话的问题。
- Skills 支持中文等多语言名称。

#### English

- Reduce long-conversation overhead during compaction, recovery, and external runtime session reuse.
- Fix Native Agent tool approvals and paused-task recovery, and unblock external runtime conversations after cancelling queued writes.
- Support multilingual Skill names, including Chinese.

### Added / 新增

- Skills 支持中文等多语言名称，可通过名称显式调用。
- Skills support multilingual names, including Chinese, with explicit invocation by name.

### Fixed / 修复

- Native Agent 在写作、通用对话与游戏中压缩历史后，只保留活动上下文，并在版本一致时直接恢复，减少长会话的历史读取与检查点开销；已有会话和旧检查点仍可读取、恢复。
- After compaction, Native Agent keeps an active context window for Writing, General, and Game and reuses aligned checkpoints, reducing long-session history reads and checkpoint work. Existing conversations and older checkpoints remain readable and recoverable.

- 修复 Native Agent 在本轮引用文件或项目指令修改后，工具审批失败或暂停任务无法继续的问题。新任务恢复时保留本轮已接受的上下文，下一轮读取最新内容。
- Fix Native Agent tool approvals and paused-task continuation after referenced files or project instructions change. New tasks retain accepted context on resume and read current content on the next turn.

- 写作与通用对话复用外部运行时会话时，跳过不必要的完整历史加载，减少长会话的准备开销。
- Skip unnecessary full-history loading when Writing and General conversations reuse an external runtime session, reducing preparation overhead for long conversations.

- 修复写作与通用对话使用外部运行时时，取消排队中的写入后无法继续会话或切换运行时的问题。
- Fix external-runtime Writing and General conversations remaining blocked after cancelling a queued write, preventing continuation or runtime switching.

## [v0.5.0] - 2026-09-22

### Brief / 简要说明

#### 中文

- 写作、通用对话与游戏支持 Native、Codex 和 Claude Code，可使用本机登录或兼容的 API 模型，并在回合间切换。
- 支持任务暂停与重启后恢复、读取本地图片，改善长任务与多图会话的压缩和断流重试。
- 新增游戏语音朗读、可搜索和更新的 Skills 技能库，以及安装包上传更新。
- 升级会自动备份受影响的会话记录；v0.4.5 无法直接读取新版记录，降级需恢复升级前备份并另行保留新增内容。

#### English

- Writing, General chat, and Game support Native, Codex, and Claude Code, with local sign-in or compatible API models and runtime switching between turns.
- Pause tasks and resume after restart, inspect local images, and use more reliable compaction and stream retries for long tasks and image-rich conversations.
- Added Game read-aloud, a searchable Skills library with updates, and updates from uploaded release archives.
- Affected conversation journals are backed up before upgrading. v0.4.5 cannot read the new records; downgrading requires restoring pre-upgrade backups and separately preserving newer content.

### Added / 新增

- 写作、通用对话、游戏及对应自定义 Agent 可在回合间切换 Native、Codex 和 Claude Code，使用本机登录或 Denova 中兼容的 API 模型。外部引擎保留持续会话和自身压缩；写作与通用对话支持 Goal，游戏不支持 Goal。
- Writing, General chat, Game, and their custom Agents can switch between Native, Codex, and Claude Code between turns using local sign-in or compatible API models in Denova. External engines retain their sessions and native compaction; Goals are supported in Writing and General chat, but not Game.

- 写作与游戏支持暂停整个 Agent 任务并在重启后继续，保留已接收输入、子任务和游戏已接纳草稿；可核实结果不明的中断操作或取消任务。
- Pause an entire Writing or Game Agent task and continue after restart, preserving accepted input, child tasks, and accepted Game drafts; verify uncertain interrupted operations or cancel the task.

- 游戏支持 OpenAI 兼容的语音朗读，可配置声线、自动朗读、引号与动作过滤，并控制暂停、语速和音量。
- Game supports OpenAI-compatible read-aloud with voice selection, automatic reading, quoted-text and action filters, pause, playback speed, and volume controls.

- Skills 新增可搜索的卡片式技能库和启用开关，可使用 `~/.agents/skills` 通用技能；远程安装支持默认关闭的每日自动更新，并保护本地修改、保留备份。
- Skills has a searchable card library with availability controls, optional shared Skills from `~/.agents/skills`, and opt-in daily updates for remote installs with local-edit protection and backups.

- Agent 可通过 `read` 查看本地图片与生成图；写作和游戏保留读取时的图片副本，重启后仍可分析。
- Agents can inspect local and generated images with `read`; Writing and Game retain captured image copies for analysis after restart.

- 支持在设置中上传 GitHub Release 安装包，离线校验后重启安装。
- Upload a GitHub Release archive in Settings, validate it offline, and restart to install.

### Fixed / 修复

- 改善写作与游戏的长任务、多图会话及重复压缩，避免误报容量不足和摘要来源不匹配；保留当前要求、近期工具结果与图片输入，超大工具输出可回读完整文件，重启后摘要仍可用。
- Improve compaction for long tasks, image-rich conversations, and repeated Game summaries, avoiding false capacity errors and source mismatches. Preserve current instructions, recent tool results, and image inputs; oversized tool output remains readable from files, and checkpoints survive restart.

- 修复生成中途断流无法重试的问题，重试等待可取消，且不会执行未接纳响应中的工具。
- Retry interrupted model streams with cancellable backoff and prevent tool execution from unaccepted responses.

- 修复设置部分保存失败后状态不同步、重试仍提交已撤回草稿的问题；保留未保存修改并展示逐文件结果。
- Fix stale settings state and retries submitting withdrawn drafts after partial saves; preserve unsaved edits and show per-file outcomes.

### Changed / 变更

- v0.4.5 无法读取新版任务恢复、压缩、外部运行时记录及 Agent Profile 配置。首次升级写入前为对应 journal 保留 `.pre-resilience-v1.bak`、`.pre-incremental-compaction-v2.bak` 或 `.pre-external-runtime-v1.bak`；旧游戏摘要会从原始历史重建。切回 Native 不会降级格式，降级需退出应用、恢复最早的升级前备份，并另行保留后续内容及配置。
- v0.4.5 cannot read the new task recovery, compaction, external runtime records, or Agent Profile settings. Before the first upgraded write, affected journals receive `.pre-resilience-v1.bak`, `.pre-incremental-compaction-v2.bak`, or `.pre-external-runtime-v1.bak`; old Game checkpoints are rebuilt from original history. Switching back to Native does not downgrade the format. To downgrade, stop the app, restore the earliest pre-upgrade backup, and separately preserve newer content and settings.

## [v0.4.5] - 2026-09-09

### Brief / 简要说明

#### 中文

- 修复写作与游戏的手动压缩和长故事反复压缩，摘要在继续回合与重启后保持有效。
- 修复版本对比显示旧快照中的运行时文件，以及 Windows 开发环境的请求连接延迟。
- 更新默认创作指令与内置叙事风格，迁移前备份旧默认内容，保留用户自定义内容。

#### English

- Fixed manual compaction in Writing and Game and repeated compaction in long stories; summaries remain valid across subsequent turns and restarts.
- Fixed version comparisons exposing runtime files from older snapshots and request connection delays in Windows development.
- Updated default creative instructions and built-in narrative styles, backing up retired defaults and preserving user customizations.

### Changed / 调整

- 清理默认创作指令中的破限提示，移除“直白情色”内置叙事风格；升级时备份并清理未修改的旧默认内容，保留用户自定义内容。
- Remove unrestricted-content instructions from the default creator template and retire the Direct Erotica preset; back up unchanged released defaults during migration and preserve user customizations.

### Fixed / 修复

- 修复写作和游戏在查看上下文分析或重启后无法手动压缩的问题，以及游戏继续回合后摘要失效、长故事反复压缩的问题。
- Fix manual context compaction after context analysis or restart in writing and games, and preserve game checkpoints across subsequent turns to prevent repeated compaction in long stories.

- 版本对比沿用当前文件排除规则，不再显示旧快照中的会话锁等运行时文件；改善 Windows 开发环境的 API 连接复用与本机双栈连接。
- Apply current file exclusions to historical version comparisons so runtime files such as session locks stay hidden; improve API connection reuse and local dual-stack connections in Windows development.

## [v0.4.4] - 2026-09-08

### Brief / 简要说明

#### 中文

- 首次开启局域网访问时自动生成随机用户名和密码，支持查看、复制与重新生成密码。
- 修复局域网地址误用代理虚拟网卡、开发环境端口不正确的问题。
- 修复长写作会话无法继续、子 Agent 结果读取失败，以及 Windows 版本管理报错。
- 修复同一轮对话中后续工具审批失败或卡片消失的问题，可连续审批并继续执行。
- 更新编辑器和 HTML 清理依赖，修复已知安全问题。

#### English

- Automatically generate a random username and password when enabling LAN access for the first time, with password viewing, copying, and regeneration.
- Fixed LAN links selecting proxy tunnel addresses or the wrong port in development.
- Fixed long writing conversations failing to continue, completed SubAgent result reads, and version management errors on Windows.
- Fixed subsequent tool approvals failing or disappearing within a run, allowing consecutive approvals and continued execution.
- Updated editor and HTML sanitization dependencies to address known security issues.

### Changed / 调整

- 首次开启局域网访问时自动补齐缺失的用户名和密码并一并保存，已有凭据继续保留；新密码可在当前设置页面查看、复制或重新生成。
- Enabling LAN access now generates and saves missing credentials together while preserving existing credentials; newly entered or generated passwords can be viewed, copied, or regenerated in the current settings page.

### Fixed / 修复

- 局域网链接优先使用有效的内网地址，跳过代理使用的测试网段，并在本机开发代理下沿用实际网页端口；地址提示与配对二维码保持一致。
- LAN links now prefer valid private network addresses, exclude proxy benchmarking networks, and preserve the actual web port behind a local development proxy; displayed addresses and pairing codes stay consistent.

- 修复 Windows 上活动会话的锁文件导致版本状态读取及快照创建失败的问题。
- Fixed active session lock files blocking version status and snapshot creation on Windows.

- 修复长写作会话因上下文索引错位而无法继续的问题，已有会话可在重新加载后正常续写。
- Fixed context index mismatches blocking long writing conversations; existing sessions can continue after reloading.

- 修复同一轮对话后续工具审批提交失败，以及待审批卡片被历史刷新覆盖的问题。
- Fixed subsequent tool approvals failing within a run and history refreshes hiding pending approval cards.

- 修复多个观察者同时读取子 Agent 完成结果时，会话句柄关闭导致的偶发失败。
- Fixed intermittent completed SubAgent result reads failing when another observer closes the shared session handle.

### Security / 安全

- 更新 Tiptap 与 DOMPurify，修复编辑器属性处理及 HTML 清理中的已知安全问题。
- Updated Tiptap and DOMPurify to fix known security issues in editor attribute handling and HTML sanitization.

## [v0.4.3] - 2026-09-07

### Brief / 简要说明

#### 中文

- 改善手机网页与 PWA 的写作、游戏、导航和输入体验。
- 修复局域网登录与刷新后的认证，支持一次性二维码登录。
- 修复 Agent 设置保存覆盖无关配置，并将异常对话历史的影响限制在对应会话内。

#### English

- Improved writing, gameplay, navigation, and input on mobile web and PWA.
- Fixed LAN sign-in and authentication after refresh, with one-use QR sign-in.
- Prevented Agent settings saves from overwriting unrelated profiles and isolated invalid conversation history to the affected session.


### Changed / 调整

- 优化手机网页与 PWA 的导航、写作和游戏交互：全屏工作面板、独立正文与 Agent 视图、剧情历史与分支列表，以及适应软键盘的输入布局；保留桌面布局。
- Improved mobile web and PWA navigation, writing, and gameplay with full-screen panels, separate editor and Agent views, readable story history and branches, and keyboard-aware input while preserving desktop layouts.

### Fixed / 修复

- 异常对话历史仅阻止对应会话或故事加载，避免影响其他会话和工作台使用。
- Invalid conversation history now blocks only the affected session or story, keeping other conversations and the workbench usable.

- 修复切换模型或保存 Agent 设置时覆盖无关配置文件的问题；按文件检查冲突并保留可恢复的变更记录。
- Fixed model selection and Agent settings saves overwriting unrelated profiles; each changed file now checks for conflicts and retains recoverable history.

- 修复局域网登录页面无法显示及刷新后反复认证的问题；浏览器可保持登录 30 天，并支持本机生成短时一次性登录二维码和链接。
- Fixed LAN sign-in pages failing to load and repeated authentication after refresh; browsers stay signed in for 30 days and can connect through short-lived, one-use QR codes and links created on the host.

## [v0.4.2] - 2026-09-06

### Brief / 简要说明

#### 中文

- 改善本地推理服务的内置工具调用兼容性，保留完整参数说明和执行校验。
- 记住手动选择的模型与思考强度，供后续同类对话和新故事使用。

#### English

- Improved built-in tool compatibility with local inference services while retaining full parameter guidance and execution validation.
- Remembered manually selected models and thinking levels for subsequent conversations of the same kind and new stories.

### Changed / 调整

- 手动调整模型或思考强度后，后续同类对话和新故事会沿用该选择，减少重复配置。
- Subsequent conversations of the same kind and new stories reuse manually selected models and thinking levels, reducing repeated setup.

### Fixed / 修复

- 简化内置工具的参数 Schema，改善本地推理服务的工具调用兼容性，同时保留参数校验和状态提交约束。
- Simplified built-in tool parameter schemas for local inference compatibility while preserving argument validation and state submission guarantees.

## [v0.4.1] - 2026-09-06

### Brief / 简要说明

#### 中文

- 修复工作台变更审阅页面崩溃、刷新后仍反复报错的问题。
- 明确 Agent 资料库写入规则：按人物、地点等独立实体或主题分条维护，更新时保留已有有效设定。

#### English

- Fixed workbench change-review crashes that could recur after refreshing.
- Clarified Agent lore-writing guidance to maintain separate entities or topics and preserve valid existing canon during updates.

### Fixed / 修复

- 修复正式构建中打开或恢复工作台变更审阅标签时出现 React 渲染异常、刷新后仍无法恢复的问题。
- Fixed a React rendering crash when opening or restoring workbench change-review tabs in production builds, including crashes that persisted after refreshing.

## [v0.4.0] - 2026-09-05

### Brief / 简要说明

#### 中文

v0.4.0 的核心是重建创作工作台与 Agent 执行底座：围绕 Project 统一创作资源，让持续对话、任务执行与恢复使用一致的会话机制。

- **统一 Project 工作台**：新增通用 Agent，书籍与本地目录都可作为项目；写作、游戏与共通工具采用并列导航，共享文件、资料库、终端和版本历史，支持多会话并行。
- **重建 Agent 运行与恢复**：统一保存对话、工具结果与任务状态，支持运行中追加指令、中断后继续及刷新或重启后恢复；新增自定义主 Agent、Goal 和脚本编排，统一管理 Skills、协作与权限。
- **重组游戏创作流程**：从组合预设改为故事自身配置，规划模板独立复用；单页完成开局，在故事控制台调整规划、事件与状态，并可从任意已保存回复创建分支。
- **统一模型接入**：语言与图像模型采用可复用的“连接 + 模型”配置，扩展 Responses、Anthropic、Gemini Image、Seedream 与 ComfyUI 等接入。
- **项目数据可整体迁移**：受管项目使用稳定身份和相对路径，退出应用后可将完整 `.denova` 目录跨 Windows、WSL、Linux 与 macOS 搬迁，保留会话、版本和附件。

**升级注意**：v0.3.3 数据按支持范围保留或先备份再迁移；旧全局 Automation 须在项目内重新创建，旧 Config Manager 历史不再展示，部分模型与上下文选项需重新设置。旧游戏预设中的自定义规划 Markdown 保留在备份中，不自动转换。

#### English

v0.4.0 rebuilds the creation workbench and Agent runtime around Projects, giving ongoing conversations, task execution, and recovery a consistent session model.

- **Unified Project workbench**: General Agents, Books, and local directories share project tools and parallel conversations; Writing, Game, and shared tools use peer navigation.
- **Rebuilt Agent runtime**: Conversations, tool results, and task state persist together, with follow-ups, resumable interruptions, restart recovery, custom main Agents, Goals, and scripting.
- **Restructured Game creation**: Stories own their settings; reusable Planning Templates, single-page setup, the Story Console, and branching from any saved reply reshape the workflow.
- **Unified model setup**: Reusable connections and model profiles expand language and image-provider support.
- **Portable project data**: Managed Projects retain history, versions, and attachments when moving the complete `.denova` directory across supported systems after shutdown.

**Upgrade note**: Supported v0.3.3 data is retained or migrated after backup. Recreate global Automations within Projects; old Config Manager history is no longer displayed, and some model/context options need reselection. Custom planning Markdown from old Game Presets remains in backups and is not converted automatically.

### Major changes / 重大变更

#### 中文

- **以 Project 统一创作工作台与资源。** 从写作、游戏模式切换改为并列一级导航；新增通用 Agent，书籍和任意本地目录可运行多个通用或写作会话。Files、资料库、阅读器、终端与版本历史在工作台内共享，配置对话与 Automation 也归入 Project 会话。双栏布局可持久保存，文件按类型使用文档、源码或图片视图，并共享自动保存、冲突保护与版本恢复。
- **以持久会话重建 Agent 执行。** 写作、通用对话、图像、自动化与游戏复用统一运行机制；每个逻辑会话以一份日志保存对话、工具结果、Goal、待办和上下文压缩等恢复状态，减少刷新、重连和重启后的丢失或重复。运行中可追加 Follow Up，中断后可继续；会话独立保存模型与权限。新增自定义主 Agent、Goal、JavaScript `script` 编排和 Project `AGENTS.md`，既有 Skills 与 SubAgent 协作纳入统一配置和执行机制，支持 Ask、Write、Full access 权限模式。主 Agent 默认自行处理任务，仅在用户或 Skill 明确要求时委派。Agent 配置本身也作为受管 Project 管理，可通过会话、Files 与版本历史维护。
- **将游戏规划与故事配置分开。** v0.3.3 由组合预设提供的叙事、事件、检定、状态与图像选择，现在直接保存在故事上；规划独立为五种内置方式及可自定义的规划模板。创建故事采用单页开局，故事控制台集中管理规划、事件与状态，并支持从任意已保存 AI 回复创建和管理分支。创作者可以分别调整故事的运行配置与后续剧情规划。
- **将模型服务连接与模型选项分开。** 同一连接可复用到多个模型，支持语言模型发现、批量添加和连接测试；协议覆盖 OpenAI Chat Completions、Responses、Anthropic Messages 及自定义兼容端点。图像接入扩展为 OpenAI Images、xAI/Grok、火山方舟 Seedream、Google Gemini Image 与 ComfyUI Workflow，多图任务保留部分成功结果。思考强度与单模型输出上限统一配置，改善长会话、工具调用和不同服务商之间的兼容性。
- **让项目数据脱离本机绝对路径。** 受管 Project 使用稳定 Project ID，数据目录内引用改为规范相对路径；退出 Denova 后，完整 `.denova` 目录可跨 Windows、WSL、Linux 与 macOS 移动或复制，会话、游戏、版本、附件、工具产物和自动化随项目保留。v0.3.3 数据按支持范围先备份再迁移；外部 Project 的内容不包含在数据目录内，不承诺随目录跨系统迁移。

#### English

- **One Project workbench for creation and resources.** Peer top-level navigation replaces Writing/Game mode switching. General Agents are added, and Books or arbitrary local directories can run multiple General or Writing conversations. Files, Lore, Reader, terminals, and version history share the workbench; configuration conversations and Automations also belong to Project sessions. Persistent split layouts and file-appropriate document, source, or image views share autosave, conflict protection, and version restoration.
- **An Agent runtime built around durable sessions.** Writing, General chat, Image, Automation, and Game share execution mechanisms. Each logical session keeps conversations, tool results, Goals, todos, and context-compaction state in one journal, reducing loss or duplication after refresh, reconnect, or restart. Runs accept follow-ups and support resumable interruptions; sessions retain their own model and permissions. Custom main Agents, Goals, JavaScript scripting, and Project instructions extend the existing Skills and SubAgent capabilities under shared configuration and execution, with Ask, Write, and Full access modes. Delegation remains opt-in through user or Skill instructions. Agent configuration is itself managed as a Project with conversations, Files, and version history.
- **Game planning separated from story settings.** Narrative, event, check, state, and image selections previously supplied by composition presets now belong directly to each story. Planning becomes independently reusable through five built-in approaches and custom Planning Templates. Single-page setup, a consolidated Story Console, and branching from any saved AI reply let creators adjust story settings and future plot plans separately.
- **Model connections separated from model options.** Multiple models reuse one connection, with language-model discovery, batch addition, and connection tests. Protocols include OpenAI Chat Completions, Responses, Anthropic Messages, and custom-compatible endpoints. Image support covers OpenAI Images, xAI/Grok, Volcengine Ark Seedream, Google Gemini Image, and ComfyUI Workflow, preserving partial success in multi-image jobs. Unified thinking settings and per-model output caps improve long-session, tool-call, and provider compatibility.
- **Project data independent of host paths.** Managed Projects use stable IDs and normalized relative references. After Denova exits, the complete `.denova` directory can move or be copied across Windows, WSL, Linux, and macOS with sessions, Game state, versions, attachments, tool artifacts, and Automations intact. Supported v0.3.3 data migrates after backup. External Project contents are outside the data directory and are not covered by this cross-system portability guarantee.

### Incompatible data changes / 用户数据不兼容变更

- v0.3.3 的游戏预设不再作为运行时组合配置。故事首次打开时会先备份原故事日志和引用的旧预设，再保留当时实际生效的叙事、事件、检定、状态、图像及规则展示配置，并切换到默认规划模板；旧预设中的自定义规划 Markdown 不自动转换，原内容保留在迁移备份中。
- v0.3.3 Game Presets no longer act as runtime composition settings. When a story is first opened, Denova backs up its journal and referenced legacy preset, preserves the effective narrative, event, check, state, image, and rule-visibility selections on the story, and selects the default Planning Template. Custom planning Markdown is not converted automatically and remains available in the migration backup.
- v0.3.3 的独立 Config Manager 会话、专属设置和用户级全局 Automation 任务文件会保留，但不再展示、触发或参与运行；后续配置对话使用对应 Project 的统一会话列表，需要继续使用的 Automation 须在对应 Project 下重新创建。
- Standalone v0.3.3 Config Manager sessions, dedicated settings, and user-level global Automation files are retained but no longer displayed, triggered, or used at runtime. Future configuration conversations use the Project's shared conversation list, and Automations that remain needed must be recreated in that Project.
- v0.3.3 的旧思考、输出上限、工具结果保留、低层 Cleanup 与 `labs.continual_learning` 设置不再生效；升级后需重新选择当前选项。有效用户级 Agent 设置会在备份后迁入 `.denova/agents` Profile，Project 覆盖继续保留。
- Legacy v0.3.3 thinking, output-limit, tool-result retention, low-level Cleanup, and `labs.continual_learning` settings no longer take effect and must be reselected where applicable. Active user-level Agent settings are backed up before migration into `.denova/agents` Profiles, while Project overrides remain intact.

### Major fixes / 重要修复

- 修复 Agent 流式输出和子 Agent 写作时界面明显卡顿的问题。
- Fixed severe UI stalls during Agent streaming and delegated writing.
- 修复子 Agent 成功写入文件后被错误标记为失败、导致主 Agent 重复写入的问题。
- Fixed delegated Agents reporting failure after successful file writes, which could cause the parent Agent to repeat the writes.

- Agent 首轮上下文现在直接包含当前可用的 Skill 目录，按名称加载只需一次工具调用；显式 `/<skill-name>` 仍会直接预载，减少等待和无效上下文消耗。
- The first Agent context now includes the available Skill catalog, so loading by name takes one tool call; explicit `/<skill-name>` invocations remain preloaded, reducing latency and unnecessary context usage.

- 修复首次切换 v0.3.3 书籍时工作区路径与 Project ID 短暂不同步而报错的问题；切换现在会一次发布完整 Project 身份，无需刷新恢复。
- Fixed first-switch errors for v0.3.3 Books caused by the workspace path and Project ID updating separately; switches now publish one complete Project identity without requiring a refresh.
- 修复数据目录绝对路径参与 Project 与会话身份，以及部分 v0.3.3 游戏故事含旧上下文事件的问题；搬迁或升级后不再出现历史消失、重复 Project 或故事无法打开，迁移会先备份原始数据再原子更新。
- Fixed absolute data-directory paths participating in Project and session identity, along with obsolete context events in some v0.3.3 Game stories. Relocation or upgrade no longer causes missing history, duplicate Projects, or stories that fail to open, and migrations back up source data before atomic updates.
- 修复跨轮、工具调用、刷新、重连、中断、继续或重启后推理上下文、消息、工具结果和已提交内容丢失或重复的问题；主动中断现在进入可恢复暂停，Token 校准不会再下调保守上下文估算。
- Fixed reasoning context, messages, tool results, or committed content being lost or duplicated across turns, tool calls, refresh, reconnect, interruption, continuation, or restart. User interruption now creates a resumable pause, and token calibration no longer lowers conservative context estimates.
- 修复写作与资料编辑中的自动保存、外部修改合并、评论消费和版本恢复问题，避免草稿被覆盖或已提交反馈重复出现。
- Fixed autosave, external-edit merging, comment consumption, and version restoration in Writing and Lore, preventing draft overwrites and submitted feedback from reappearing.
- 修复游戏检定重复应用故事修正、重生成重试丢失原回合、生成中调校丢弃正文，以及空回合崩溃、历史跳转和刷新后重复回放等问题；规则模板修正由后端解析，大成功与大失败仅由自然 20/1 触发。
- Fixed Game checks applying story tuning twice, regeneration retries losing their target, tuning during generation discarding prose, empty-turn crashes, incorrect history navigation, and settled content replaying after refresh. Rule-template modifiers are resolved by the backend, and critical success or failure requires a natural 20 or 1.
- 修复内置终端退出、重连、主题和 PTY 生命周期问题，交互式 CLI 退出后可可靠返回原工作目录的 Shell。
- Fixed embedded-terminal exit, reconnect, theme, and PTY lifecycle issues so interactive tools reliably return to the shell in the original working directory.
- 修复 DeepSeek、MiniMax、Claude 等模型及游戏回合输出上限过低，以及 DeepSeek 思考模式在工具结果投影或启动 SubAgent 后未回传 `reasoning_content` 而失败的问题。
- Fixed incorrectly low output limits for DeepSeek, MiniMax, Claude, and Game turns, plus DeepSeek thinking-mode failures when `reasoning_content` was not replayed after tool-result projection or SubAgent startup.
- 模型回复达到输出或上下文限制、被内容过滤或被服务商标记未完成时，会保留已生成内容、按真实原因提示并阻止执行可能残缺的工具参数；模型最大输出 Token 现在同时参与上下文预算，压缩摘要使用独立的有界输出。
- Model responses that reach output or context limits, are content-filtered, or are marked incomplete now retain generated content, report the actual reason, and block potentially partial tool arguments. Model output caps now also participate in context budgeting, while checkpoint summaries use their own bounded output.
- 应用内更新在 Release 缺少 `checksums.txt` 时会拒绝安装，不再跳过完整性校验。
- In-app updates now refuse releases without `checksums.txt` instead of skipping integrity verification.
- 升级 Go、`go-git` 与 `x/image` 安全基线，修复标准库、Git 路径/符号链接和 WebP 解码相关漏洞。
- Upgraded Go, `go-git`, and `x/image` security baselines to address standard-library, Git path/symlink, and WebP decoding vulnerabilities.

## [v0.3.3] - 2026-07-25

- 修复写作编辑器与对话框切换时输入法被重置的问题。
- Fixed IME resets when switching between the Writing editor and chat.
- 每个 Markdown 文件使用独立撤销历史，跨标签页撤销不再串改内容。
- Each Markdown file now has an isolated undo history, preventing cross-tab edits during undo.

## [v0.3.2] - 2026-07-23

- Agent 对话按 Run 聚合为可折叠的执行过程；工作台支持跨 Project、多会话真正并行运行。
- Agent conversations are grouped into collapsible run histories, while Workspace supports true concurrent conversations across Projects.
- Skills 新增分类与能力声明，工具参数支持安全规范化，网页搜索支持可替换 Provider 与抓取回退。
- Skills gained categories and capability declarations, tool arguments gained safe normalization, and web access gained replaceable search providers and fetch fallbacks.
- 编辑器自动保存和 Git 自动版本改为修改后延迟执行，并用 revision/CAS 与三方合并保护并发编辑。
- Editor autosave and Git versions now run after edit idle periods, with revision/CAS checks and three-way merging for concurrent changes.
- 游戏历史、模型上下文和 UI 展示拆分为独立投影，并强化最新回合编辑、重生成与恢复的一致性。
- Game history, model context, and UI rendering now use separate projections with safer latest-turn editing, regeneration, and recovery.

## [v0.3.1] - 2026-07-23

- Agent Trace 支持复制 Run ID 和导出完整 JSONL；前端独立启动可显式指定后端端口。
- Agent Trace can copy Run IDs and export complete JSONL traces; standalone frontend startup can target an explicit backend port.
- 修复端口冲突、Windows 设置自动保存和游戏正文候选重复等关键问题。
- Fixed important port-conflict, Windows settings-autosave, and duplicate Game narrative issues.

## [v0.3.0] - 2026-07-22

- 写作模式新增持久化 Change Review、正文评论、跨重启 Undo/Redo、正则替换和带备份的全局替换。
- Writing added durable Change Review, document comments, restart-safe undo/redo, regex replacement, and backed-up workspace-wide replacement.
- 游戏模式新增导演运行策略、故事级状态结构、Actor 归档/恢复、状态布局、回复修正和全屏导演台。
- Game added Director policies, story-scoped state schemas, Actor archive/restore, state layouts, response correction, and a full-screen Director Desk.
- 自动保存、三方合并、工作区变更账本、崩溃恢复与活动任务重连统一为更可靠的数据保护链路。
- Unified autosave, three-way merging, workspace change journals, crash recovery, and active-task reconnection into a safer data pipeline.
- 新增上下文检查、完整 Trace、更平滑的流式展示和 Unicode 规范化安全升级。
- Added context inspection, complete traces, smoother streaming, and a Unicode normalization security update.

## [v0.2.0] - 2026-07-15

- 写作工作台与 AI 互动叙事统一到长期创作架构，明确 Story Director、Actor State、事件、TRPG、资料与历史边界。
- Unified the Writing workbench and interactive narrative around clear Story Director, Actor State, event, TRPG, Lore, and history boundaries.
- Agent/Subagent 新增可配置 Skills、上下文压缩、工具结果策略、运行计划、自动化和本地 Trace。
- Agent and Subagent workflows gained configurable Skills, context compaction, tool-result policies, plans, automation, and local traces.
- 游戏当前状态改为摘要优先的自适应布局，并支持安全恢复无效的旧预设覆盖。
- Game Current State moved to an adaptive summary-first layout with safe recovery for invalid legacy preset overrides.
- 资料库新增按需加载、批量读取、revision 保护和 Tavern 角色卡/世界书导入。
- Lore gained on-demand loading, batch reads, revision protection, and Tavern character-card/world-book imports.

## [v0.1.18] - 2026-07-01

- 完成 Denova 品牌与分发命名切换，Release 包只提供 `denova` / `denova.exe`。
- Completed the Denova branding and distribution rename; release packages now use only `denova` / `denova.exe`.
- 新增新用户引导、消息中心、PWA/移动端主屏和前端内嵌自托管能力。
- Added onboarding, a message center, PWA/mobile home-screen support, and embedded-web self-hosting.
- 完善移动端写作与游戏的输入、弹窗、文件操作、故事记忆与分支导航。
- Expanded mobile Writing and Game input, dialogs, file actions, story memory, and branch navigation.
- 改进书籍封面、互动图像、Plan Mode 和资源保存冲突保护。
- Improved book covers, interactive images, Plan Mode, and resource-save conflict protection.

## [v0.1.17] - 2026-06-27

- 新增章节插图与互动图像生成工作流，并完善模型设置和故事开局交互。
- Added chapter-illustration and interactive-image workflows, with improved model settings and story-opening controls.

## [v0.1.16] - 2026-06-27

- 新增图像生成配置和默认模型别名支持。
- Added image-generation settings and default model aliases.

## [v0.1.15] - 2026-06-27

- 完善 Writing Agent 配置、运行控制和应用内更新体验。
- Improved Writing Agent configuration, run controls, and in-app updates.

## [v0.1.14] - 2026-06-26

- 新增写作 Skill 预设、配置工具和 Subagent 会话详情，并强化 Agent 启动与更新稳定性。
- Added Writing Skill presets, configuration tools, and Subagent session details, while improving Agent startup and updater reliability.

## [v0.1.13] - 2026-06-24

- 改进 Agent 上下文缓存、配置资源管理和移动端适配，并隔离配置管理会话。
- Improved Agent context caching, configuration resources, mobile layouts, and Config Manager session isolation.

## [v0.1.12] - 2026-06-20

- 重构互动故事记忆与状态 Schema，并补充上下文控制和失败重试。
- Reworked interactive-story memory and state schemas with stronger context controls and retry behavior.

## [v0.1.11] - 2026-06-18

- 新增故事开局、资料启用状态、Tavern 卡导入与记忆召回。
- Added story openings, Lore enablement, Tavern-card import, and memory recall.
- 新增自动化触发、消息收件箱、GitHub Release 更新器和深浅主题。
- Added automation triggers, an inbox, a GitHub Release updater, and light/dark themes.

## [v0.1.10] - 2026-06-12

- 改进工作区删除与恢复安全性。
- Improved workspace deletion and recovery safety.

## [v0.1.9] - 2026-06-12

- 修复空工作区启动、Skills 与互动叙事回归，完成核心架构稳定性清理。
- Fixed empty-workspace startup, Skills, and interactive-story regressions, with core architecture stabilization.

## [v0.1.8] - 2026-06-11

- 新增网页搜索和自定义 Skills。
- Added web search and custom Skills.

## [v0.1.7] - 2026-06-10

- 新增现有小说导入、自动化、工作区版本历史、中英文本地化和全局字号设置。
- Added existing-novel import, automation, workspace version history, Chinese/English localization, and global font sizing.

## [v0.1.6] - 2026-06-05

- 新增角色状态跟踪，增强资料管理，并将版本能力收敛为本地快照。
- Added character-state tracking, improved Lore management, and consolidated versioning around local snapshots.

## [v0.1.5] - 2026-06-02

- 资料库迁移为结构化数据并支持渐进加载与批量操作；资料、导演和写作工作区升级为完整 IDE 页面。
- Migrated Lore to structured storage with progressive loading and batch operations, and promoted Lore, Director, and Writing surfaces to full IDE pages.
- 互动故事新增回合版本切换、行动候选和按 Agent 配置的模型 Profile。
- Interactive stories gained turn versioning, action choices, and per-Agent model Profiles.

## [v0.1.4] - 2026-05-29

- 建立可玩的互动故事工作台，支持流式回合、状态、分支路线、行动候选和可中断生成。
- Introduced the playable interactive-story workbench with streaming turns, state, branch routes, action choices, and interruptible generation.
- 资料库升级为结构化 Lore Item，并新增资料 Agent、导演配置和 Tavern v2 角色卡导入。
- Upgraded Lore to structured items and added a Lore Agent, Director configuration, and Tavern v2 character-card import.
- 写作工作台新增作品统计，并统一写作与游戏的紧凑导航、字体和布局设置。
- Added manuscript statistics and unified compact navigation, typography, and layout controls across Writing and Game.

## [v0.1.3] - 2026-05-24

- 工作区新增多 Tab、分层设置、无工作区引导和 `brainstorm.md` 创作流程。
- Added multi-tab workspaces, layered settings, empty-workspace onboarding, and the `brainstorm.md` creation flow.
- Agent 新增异常中断恢复、上下文边界和场景化风格规则。
- Added Agent interruption recovery, context boundaries, and scene-specific style rules.

## [v0.1.2] - 2026-05-18

- 新增多会话管理、`/clear` 上下文分界、可调工作区布局和章节版本 Diff/回滚界面。
- Added multi-session management, `/clear` context boundaries, resizable workspace layouts, and chapter version diff/rollback views.

## [v0.1.1] - 2026-05-17

- 发布基于 React、TipTap 与 Go/Hertz 的首个 Web 小说 IDE，包含文件树、Markdown 编辑、自动保存和三栏工作区。
- Released the first web novel IDE based on React, TipTap, and Go/Hertz, with a file tree, Markdown editing, autosave, and a three-pane workspace.
- 创作 Agent 支持 SSE 流式输出、思考与工具时间线、中断、风格引用和 `CREATOR.md` 指令。
- The Writing Agent supports SSE streaming, reasoning/tool timelines, interruption, style references, and `CREATOR.md` instructions.
- 新增本地版本管理、书籍切换和基础会话持久化，并移除旧 TUI。
- Added local version management, book switching, and basic session persistence, and removed the former TUI.
