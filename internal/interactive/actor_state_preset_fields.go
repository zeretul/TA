package interactive

import (
	"fmt"
	"ta/internal/cultivation"
)

const actorStateFavorabilityGuidance = "0 为路人基线，表示尚无个人好恶；1–100 表示好感逐渐加深，-1–-100 表示敌意逐渐加深。好感度只描述个人好恶的方向与强度，不自动改变关系类型或关系阶段；朋友、恋爱、亲属、师徒、竞争、敌对等关系使用各自符合故事设定的阶段与行为表现。"

// engagementFieldDescription states the field's scope: this scene's fight, not
// a standing enemy list. The battle window reads it to know who is fighting and
// on which side, so its wording has to keep the two facts apart — a person can
// be hostile in disposition, or be an opponent by template, and neither alone
// puts them in a fight.
const engagementFieldDescription = "记录当前实际交战的生灵，键为已存在 Actor 的精确 ID。敌方值可为简短依据，或 {basis,side:\"enemy\",lethal_intents}。本场自愿助战者用 {basis,side:\"player\",target_id,source_exchange_index}；target_id 必须是本场真实在场敌方，source_exchange_index 引用本回合 character_facts.exchanges 中本人向主角作出接受助战的回应（从1起，speaker=character、channel=scene、addressed_to=protagonist）。回应可为符合灵智的明确动作，不强求人类语言。本人回应须记录 participation_decision={kind:combat,target_id:该敌方ID,decision:accept}，引用本回合对此事最后一条本人决定，refuse或未作答不可参战。已登记者须确实同场；首次真实相遇可在同回合合法登记自己的Actor和profile再回应。目录、好感、关系、主角的命令不能代替本人同意，也不能把远处或归档者凭空召来。此意愿只覆盖本场，不建立常驻队伍。可选 lethal_intents 项仍为 {actor_id,target_id,source_exchange_index}，只声明真实公开杀意，不决定死亡。"

// engagementFieldInstruction tells the model when to write and when to clear
// the field. The empty object is the ordinary value: most scenes have no fight,
// and a cleared field is how a truce, a surrender or a fight that ended stops
// the battle window from reopening it. The entry is written in the same turn
// that settles the attack, so the fight exists before anyone clicks.
const engagementFieldInstruction = "只有实际开始或继续交战才更新。敌意、低好感、敌对关系、敌人模板和持有兵器不等于开战。双方须真实在场，离场、收回、停战、投降或被制服者移出；交战结束清空对象。助战必须先有当事者本场的明确接受，不能把拒绝、模糊回应或普通承诺解释为参战；变更其迎击对象须重新取得回应。保留未变的本场记录时保留原始依据和引用，不伪造新的同意。每名生灵每回合按自身已学技能、资源和伤势行动，不能用长串命令增加出手机会。lethal_intents 仅引用本回合被接受的实际公开场景话语，exchange.addressed_to 必须与 target_id 一致，并明确表达已决定杀死目标或生死交战。不得依据旁人、转述、疑问、私念、假设、含糊措辞、切磋或制服意图推断杀意；玩家话语须是玩家本次输入实际包含的内容。不得重用旧杀意或在结算后补写。伤害、死亡和消耗由正式战斗规则决定。"

func defaultActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                   DefaultActorStateModuleID,
		Name:                 "默认状态系统",
		Description:          "以故事、主角和世界实体三个初始 Actor 集中维护关键状态；角色只预置最通用的等级与生命，其他面板和动态状态由开局 Agent 按故事实际需要主动添加。",
		PanelFields:          defaultActorPanelFields(),
		StateFields:          defaultActorDynamicStateFields(),
		AbilityGuidance:      "类型使用战斗、社交、探索、制造或其他符合故事设定的分类。",
		ItemGuidance:         "类型使用装备、消耗品、资源、任务物、线索或其他符合故事设定的分类。",
		RelationshipGuidance: "关系类型按故事实际使用亲属、朋友、同伴、竞争、敌对、恋爱、主从或师徒等自然语言。",
		QuestGuidance:        "任务类型按故事实际使用主线、支线、个人、委托或世界事件。",
		LocationGuidance:     "地点类型按世界观使用城市、建筑、野外、遗迹、设施或房间等自然语言。",
		FactionGuidance:      "势力类型按世界观使用组织、国家、公司、教会、帮派或其他自然语言。",
	}
}

func protagonistStateTemplate(cultivationRules *cultivation.Rules, spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.name", "姓名", "主角姓名；尚未确定时保持空白，确认后只在正式改名时更新。", "概况", "inline"),
		textStateField("identity.profile", "基本身份", "合并记录年龄、性别、职业、种族、社会身份等当前故事确实使用的信息。", "概况", "block"),
		listStateFieldWithInstruction("identity.affiliations", "所属势力", "主角已经获知的本人物势力归属及其状态；身份或状态未知时不推断。", "关系", "只记录主角实际获知的归属。列表项优先使用{faction_id,name,status}，faction_id须为势力记录的精确稳定键，name只作展示；status为formal、temporary、pending或left。同名势力不合并；不要删除已知离开而误作仍在籍。旧纯文字项不能推断新的稳定ID。主角正式归属以character_facts.memberships为准，不能靠该列表改变成员资格。不从服饰、标签、出生地、师承、来访地点或作者秘密身份推断；最多8项。"),
		textStateField("identity.appearance_style", "外貌与装扮", "合并记录有辨识度的外貌、衣着、装备外观和当前伪装；忽略无承接价值的临时描写。", "概况", "block"),
		textStateField("identity.character_background", "性格与背景", "记录会持续影响选择的性格倾向与已确认背景，不复述回合经历。", "概况", "block"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields, textStateField("current.situation", "当前处境", "记录当前优势、危险、限制和直接压力；具体地点由故事状态维护。", "战斗与状态", "block"))
	fields = append(fields, spec.ProtagonistFields...)
	fields = append(fields, actorOwnedRecordFields(cultivationRules,
		spec, 12, 16, 12, "")...)
	return actorStatePresetTemplate(
		DefaultActorID,
		"主角状态表",
		"集中维护主角人物设定、检定面板、动态状态、技能、重要物品和有方向的关系；不假设 Lore 已包含完整角色资料。",
		fields,
	)
}

func importantCharacterStateTemplate(cultivationRules *cultivation.Rules, spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.profile", "基本身份", "合并记录年龄、性别、职业、种族、组织与公开身份等有用信息。", "概况", "block"),
		listStateFieldWithInstruction("identity.affiliations", "所属势力", "主角已经获知的本人物势力归属及其状态；身份或状态未知时不推断。", "关系", "只记录主角实际获知的归属。列表项优先使用{faction_id,name,status}，faction_id须为势力记录的精确稳定键，name只作展示；status为formal、temporary、pending或left。同名势力不合并；不要删除已知离开而误作仍在籍。旧纯文字项不能推断新的稳定ID。主角正式归属以character_facts.memberships为准，不能靠该列表改变成员资格。不从服饰、标签、出生地、师承、来访地点或作者秘密身份推断；最多8项。"),
		textStateField("identity.appearance_style", "外貌与装扮", "合并记录辨识特征、常见装束、装备外观和当前伪装。", "概况", "block"),
		textStateField("identity.character_background", "性格与背景", "记录稳定性格、关键来历和会持续影响行为的背景。", "概况", "block"),
		textStateField("current.presence_location", "出场与位置", "用一句话记录在场、离场、失联、死亡等存续状态及当前位置或最后确认位置。", "战斗与状态", "inline"),
		objectStateFieldWithInstruction("knowledge.location", "已知位置", "主角最后实际获知的位置及该条消息的来源、获知时刻和可信度。", "经历见闻", "只在主角真正观察或获得新消息时更新；对象使用location、learned_at、source、nature、confidence及可选evidence_refs。location写这条消息的位置，不写后台实时位置；learned_at是获知时的世界时间，不是当前时刻或文件时间。source仅observation、conversation、public_record；nature仅experienced、told、hearsay、conjecture；confidence仅confirmed或uncertain，传闻和推测必须uncertain。evidence_refs仅引用主角可见的已提交exchange精确ID；没有证据就省略，不复制私人谈话、NPC私有状态或作者资料。信息缺失留空，不推断，不随着NPC后台移动更新。"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields,
		textStateField("current.goal_situation", "当前目标与处境", "记录已经由行为、对话或设定确认的近期目标、压力、优势与限制，不猜测幕后计划。", "战斗与状态", "block"),
		textStateField("protagonist_relation.summary", "与主角关系", "记录关系类型、当前态度、关系阶段、重要承诺、边界和主要矛盾。", "关系", "block"),
		favorabilityStateField("protagonist_relation.favorability", "对主角好感度", "该角色对主角的总体情感倾向，不代表信任、服从或恋爱关系。", "关系"),
		objectStateFieldWithInstruction("knowledge.relationships", "已知人物关系", "只记录主角已经获知的该人物对其他已知人物的有方向关系。", "关系", "键为已登记且主角已知的目标Actor精确ID；值用kind写有方向的关系，learned_at写实际获知的世界时刻。source仅observation、conversation或public_record；nature仅experienced、told、hearsay或conjecture；confidence仅confirmed或uncertain，传闻和推测必须uncertain。evidence_refs可列主角可见的已提交exchange精确ID，无引用就省略。只记亲眼见到或明确获知的关系，不从好感、同场、作者资料、NPC私有关系或私人谈话复制，不附自由证据原文。最多12项；不自动镜像，只有真正获知变化才更新。正式主角关系以character_facts.relationships为准。"),
		listStateField("knowledge.about_protagonist", "对主角的已知信息", "只列会影响该角色判断或行动、且该角色确实已经得知的信息，最多 6 项。", "经历见闻"),
	)
	fields = append(fields, spec.ImportantCharacterFields...)
	withholdXiuxianGrowthSchedule(fields)
	fields = append(fields, actorOwnedRecordFields(cultivationRules,
		spec, 6, 6, 6, "对主角的关系已由专用字段维护，此处只记录该角色对其他 Actor 或势力的关系。")...)
	return actorStatePresetTemplate(
		ActorStateImportantCharacterTemplateID,
		"重要角色状态表",
		"每个反复登场的重要角色使用独立 Actor；重点维护当前状态、对主角好感与已知信息，以及自身技能、物品和其他关系。",
		fields,
	)
}

func opponentStateTemplate(cultivationRules *cultivation.Rules, spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.profile", "类型与外观", "合并记录敌人、怪物、Boss、陷阱或异常实体的类型、身份与辨识特征。", "概况", "block"),
		textStateField("current.lifecycle_location", "存续与位置", "用一句话记录活跃、撤退、被俘、失效、消灭等状态及所在或最后确认位置。", "概况", "inline"),
		textStateFieldWithInstruction("threat.assessment", "威胁态势", "合并记录威胁级别、警戒或追击状态和当前目标。", "威胁", "block", "使用低、中、高、致命四档：低=通常无需专门准备；中=会造成明确损失；高=需要资源、协作或针对性策略；致命=正面对抗很可能导致死亡或不可逆后果。只有能力、数量、环境或目标发生实质变化时调整。"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields,
		textStateField("behavior.pattern", "行动模式", "记录已经观察或可靠确认的攻击倾向、触发方式和行为规律。", "经历见闻", "block"),
		textStateField("weakness.exit_condition", "弱点与退场条件", "合并记录已确认弱点，以及击败、驱散、封印、谈判或摆脱条件。", "经历见闻", "block"),
	)
	fields = append(fields, spec.OpponentFields...)
	withholdXiuxianGrowthSchedule(fields)
	fields = append(fields, actorOwnedRecordFields(cultivationRules,
		spec, 6, 4, 4, "")...)
	return actorStatePresetTemplate(
		ActorStateOpponentTemplateID,
		"敌对对象状态表",
		"每个需要持续追踪的敌人、怪物、Boss 或异常实体使用独立 Actor；相近威胁信息合并维护。",
		fields,
	)
}

func storyContextStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("scene.current_time", "当前时间", "按世界观记录当前日期、时段或阶段；只保留对行动有意义的精度。", "当前场景", "inline"),
		textStateField("scene.location", "当前详细地点", "用一个字段表达当前具体地点及必要的上级范围，不再拆分大区、区域和房间。", "当前场景", "inline"),
		textStateField("scene.current_event", "当前事件", "合并记录正在发生的事件、直接压力和下一步必须面对的问题。", "当前场景", "block"),
		listStateField("scene.present_actors", "在场角色", "使用 Actor 状态中已有的精确 ID；新建 Actor 直接使用故事语言中的角色名称，Actor 名称即 ID。只保留当前可感知或正在互动的角色。", "当前场景"),
		objectStateFieldWithInstruction("scene.engagement", "本次交战", engagementFieldDescription, "当前场景", engagementFieldInstruction),
		textStateFieldWithInstruction("scene.continuation_hook", "可承接钩子", "保留一个下一段可以直接承接的未完成动作、对话、发现或迫近威胁；它不是剧情总结或选项列表。", "当前场景", "block", "只写一个当前最直接的承接点。原钩子已经兑现、失效或被更强的新钩子替代时更新；使用具体对象与动作，不写泛泛的“继续探索”或多个备选项。"),
		textStateField("world.situation", "世界局势", "记眼下的世界概况和正在发生的主要事件。写清哪些地方或势力发生了什么，哪些变化仍在持续。", "世界状态", "block"),
		objectStateFieldWithInstruction("tasks.current", "当前任务", "记录已经成立且仍需推进的任务。", "当前任务", questRecordUpdateInstruction(spec)),
	}
	fields = append(fields, spec.StoryFields...)
	return actorStatePresetTemplate(
		ActorStateStoryContextTemplateID,
		"故事状态表",
		"集中维护当前场景、世界局势和当前任务；不保存纪要、总结、行动选项或幕后计划。",
		fields,
	)
}

func worldEntitiesStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		objectStateFieldWithInstruction("world.locations", "地点记录", "记录仍有探索、通行、资源或剧情价值的重要地点。", "地点", locationRecordUpdateInstruction(spec)),
		objectStateFieldWithInstruction("world.factions", "势力记录", "记录仍在影响主角、任务或世界局势的重要势力。", "势力", factionRecordUpdateInstruction(spec)),
	}
	return actorStatePresetTemplate(
		ActorStateWorldEntitiesTemplateID,
		"世界实体表",
		"地点与势力集中在同一个世界 Actor 中，以两个 object 字段保持不同记录结构。",
		fields,
	)
}

func actorOwnedRecordFields(cultivationRules *cultivation.Rules, spec actorStatePresetSpec, abilityLimit, itemLimit, relationshipLimit int, relationshipScope string) []ActorStateField {
	itemInstruction := itemRecordUpdateInstruction(spec, itemLimit)
	if spec.ID == ActorStateXiuxianID {
		// §7 requires the application layer to assemble the known items for the
		// model to choose from. The cultivation preset is the one with an item
		// catalogue, so it is the one that names the definitions the rules can
		// really price; the other presets have no catalogue and their free-form
		// record is the whole of it.
		itemInstruction += xiuxianItemDefinitionGuidance(cultivationRules)
	}
	fields := []ActorStateField{
		objectStateFieldWithInstruction("abilities.records", "技能与能力", "记录属于当前 Actor、会再次使用或仍有可变状态的能力。", "行囊", abilityRecordUpdateInstruction(spec, abilityLimit)),
		objectStateFieldWithInstruction("assets.important_items", "重要物品", "记录由当前 Actor 持有、会影响后续行动的物品。", "行囊", itemInstruction),
		objectStateFieldWithInstruction("relations.records", "关系", "记录当前 Actor 对目标 Actor 或势力的有方向关系。", "行囊", relationshipRecordUpdateInstruction(spec, relationshipLimit, relationshipScope)),
	}
	// §3.2: what a character can do, what they carry and who they are bound to
	// are learned by finding out, not by opening a panel. The pack declares the
	// three private here, once, so both the state read and the stream withhold
	// them for everyone the player is not playing.
	markActorStateFieldsPrivate(fields...)
	if spec.ID == ActorStateXiuxianID {
		// The cultivation rules own which techniques a character has learned, so
		// the list is a rules field rather than a free-form ability record. It is
		// added here, beside the records it would otherwise be confused with,
		// because that is where a reader looks for what a character can do.
		learned := xiuxianLearnedSkillsField()
		learned.Visibility = ActorStateVisibilityPrivate
		fields = append(fields, learned)
		// The method record is the cultivation rules' counterpart for 功法: what
		// the character has mastered and what is circulating. It is private for
		// the same reason, and it is what lets a second method contribute without
		// the free-text specialisation having to be widened into prose.
		methods := xiuxianTechniquesField()
		methods.Visibility = ActorStateVisibilityPrivate
		fields = append(fields, methods)
	}
	return fields
}

// markActorStateFieldsPrivate declares pack-private visibility on fields in
// place. It is how a preset states the player-information boundary without the
// projection having to recognise a field by its name.
func markActorStateFieldsPrivate(fields ...ActorStateField) {
	for index := range fields {
		fields[index].Visibility = ActorStateVisibilityPrivate
	}
}

func abilityRecordUpdateInstruction(spec actorStatePresetSpec, limit int) string {
	return fmt.Sprintf("%s子值按需记录类型、掌握或当前状态、效果、代价与限制、来源，最多 %d 条。删除记录时 replace 整个 object。%s", statePanelReadableRecordKeyInstruction("技能或能力"), limit, spec.AbilityGuidance)
}

func itemRecordUpdateInstruction(spec actorStatePresetSpec, limit int) string {
	return fmt.Sprintf("%s当前 Actor 即持有者；子值按需记录类型、数量或状态、作用、限制、来历与描述，最多 %d 条。来历写从谁手里、从什么地方得来；描述写它看上去、摸上去是什么样，一两句，可带一点机锋。两项只作记事，不进任何数值，首次写入后沿用不改。物品转移时同轮从原持有者移除并写入新持有者；删除记录时 replace 整个 object。%s", statePanelReadableRecordKeyInstruction("物品"), limit, spec.ItemGuidance)
}

func relationshipRecordUpdateInstruction(spec actorStatePresetSpec, limit int, scope string) string {
	instruction := fmt.Sprintf("键使用目标 Actor 或势力的精确 ID；新建目标直接使用故事语言中的名称，目标 Actor 或势力的名称即 ID，不得翻译成英文、转写拼音或生成 slug。只表示当前 Actor 对目标的单向关系，不自动镜像；每项只写关系类型、阶段、好感度、当前态度、边界或承诺、主要矛盾，最多 %d 条。好感度规则：%s 普通同场或闲聊不自动增加；删除记录时 replace 整个 object。%s", limit, actorStateFavorabilityGuidance, spec.RelationshipGuidance)
	if scope != "" {
		instruction += " " + scope
	}
	return instruction
}

func questRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelReadableRecordKeyInstruction("任务") + "子值按需记录类型、状态、目标与进度、时限、明确的奖惩和关联对象名称/ID，最多 8 条；关联对象引用已有状态面板名称/ID。任务结算且结果已进入正文后删除；删除记录时 replace 整个 object。" + spec.QuestGuidance
}

func locationRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelReadableRecordKeyInstruction("地点") + "子值按需记录类型、所属范围、当前状态、风险、关键特征与通路，最多 16 条。风险使用低、中、高、致命等文字等级，不保存画布坐标；删除记录时 replace 整个 object。" + spec.LocationGuidance
}

func factionRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelReadableRecordKeyInstruction("势力") + "子值按需记录类型、核心特征、存续状态、对主角立场、主要范围和当前行动，最多 8 条。" +
		"存续状态、主要范围与当前行动只在本次确有依据时才改写：写清这个变化是怎么来的——谁说的、谁办的、在什么地方见到的，或主角亲眼见到的结果；" +
		"只听到有人这么说、只被转告一句旧消息、只是自己的推测，都不足以改写，宁可先写成传闻或怀疑，也不要当成这个势力现在的样子。" +
		"没有依据时保留原有记录不动。旧记录可能写于很久以前，见 Actor 上标出的写下时间；超出这个时间的说法不能当成眼下的事实。" +
		"删除记录时 replace 整个 object。" + spec.FactionGuidance
}

func statePanelReadableRecordKeyInstruction(recordKind string) string {
	return fmt.Sprintf("map key 直接使用故事语言中稳定、可读的%s名称，名称即 ID；不得翻译成英文、转写拼音或生成 slug；子值不要求重复名称字段。", recordKind)
}

func actorPanelAndStateFields(spec actorStatePresetSpec) []ActorStateField {
	fields := make([]ActorStateField, 0, len(spec.PanelFields)+len(spec.StateFields))
	fields = append(fields, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	// §3.2: an opponent's realm and settled numbers are not laid out on the
	// panel. The two presets that settle those numbers say so here; the default
	// preset's level is left visible because it has no rule behind it and the
	// story already states it in prose.
	if spec.ID == ActorStateXiuxianID {
		for index := range fields {
			switch fields[index].Name {
			case xiuxianFieldRealm, xiuxianFieldHealth, xiuxianFieldHealthMax,
				xiuxianFieldMana, xiuxianFieldManaMax, xiuxianFieldAttack,
				xiuxianFieldDefense, xiuxianFieldAgility, xiuxianFieldPerception,
				// The rest of §3.2's withheld set. The burden pair is what the
				// panel summarises as 施招负担, so it carries the same weight as
				// the pools above it: a reader who could see the enemy's real
				// burden would know exactly how many heavy moves they have left.
				// Progress and the main implement are the same kind of fact —
				// 修为进度 says how close a breakthrough is, and 主法器 names
				// the item whose grade the settlement prices the attack from.
				xiuxianFieldBurden, xiuxianFieldBurdenFloor,
				xiuxianFieldRealmProgress, xiuxianFieldImplement,
				// The purse is the same kind of fact as the pools. T22 names
				// 钱袋 as withheld and says that knowing an asking price is not
				// knowing the wallet, so a reader must not be able to count an
				// opponent's coin by opening a panel. The protagonist keeps
				// theirs: the projection skips the actor the player is playing,
				// which is how every field above already works.
				xiuxianFieldCurrency,
				// 功法与专精 is the prose half of the method sheet and its
				// default is the literal "未定". T22 withholds 已学功法 and says
				// a default is not an observed fact, so leaving this open both
				// printed a value nobody had told the player and named a method
				// the structured record had already been withheld for. It is
				// marked on the shared list rather than per template for the
				// same reason as every field above: the protagonist's sheet
				// keeps it, because the projection skips the actor being played.
				xiuxianFieldMethod:
				fields[index].Visibility = ActorStateVisibilityPrivate
			}
		}
	}
	return fields
}

// withholdXiuxianGrowthSchedule marks the two fields that say what a character
// is doing with their time.
//
// They are declared in the genre's per-template lists rather than in
// PanelFields/StateFields, so actorPanelAndStateFields never sees them. T22 names
// 修行安排 as withheld: it is the row the growth settlement reads, so a reader
// who could see an opponent's would know how fast that opponent is advancing and
// in what direction — the same disclosure 修为进度 already avoids. 修炼间隙 is
// the companion fact (the safe window an outing really offers), and reading it
// would tell the player where an enemy can be found alone.
//
// It applies only to the two tracked-character templates. The protagonist is the
// one actor the player owns, and the projection already skips it, so marking the
// field on the shared preset list is safe for them and correct for everyone else.
func withholdXiuxianGrowthSchedule(fields []ActorStateField) {
	for index := range fields {
		switch fields[index].Name {
		case xiuxianFieldArrangement, xiuxianFieldTrainingWindow:
			fields[index].Visibility = ActorStateVisibilityPrivate
		}
	}
}

func defaultActorPanelFields() []ActorStateField {
	return []ActorStateField{
		openEndedNumberStateField("panel.level", "等级", "角色当前规则等级。", "战斗与状态", 1, 1, "1–4 初阶；5–10 中阶；11–16 高阶；17 及以上传奇。", "等级只随明确的升级、降级或规则结算改变。"),
	}
}

func defaultActorDynamicStateFields() []ActorStateField {
	return []ActorStateField{
		textStateFieldWithDefaultInstruction("state.health", "生命", "使用“当前值/上限”表达可恢复的生命资源；例如 8/12。", "战斗与状态", "inline", "10/10", "受伤、治疗或上限改变时更新；没有生命数值规则时改用作品中的等价状态。"),
	}
}

func textStateField(path, name, description, group, display string) ActorStateField {
	return presetStateField(path, name, "string", description, group, display)
}

func textStateFieldWithInstruction(path, name, description, group, display, instruction string) ActorStateField {
	field := textStateField(path, name, description, group, display)
	field.UpdateInstruction = instruction
	return field
}

func textStateFieldWithDefaultInstruction(path, name, description, group, display, defaultValue, instruction string) ActorStateField {
	field := textStateFieldWithInstruction(path, name, description, group, display, instruction)
	field.Default = defaultValue
	return field
}

func listStateField(path, name, description, group string) ActorStateField {
	field := presetStateField(path, name, "list", description, group, "list")
	field.Default = []any{}
	return field
}

func listStateFieldWithInstruction(path, name, description, group, instruction string) ActorStateField {
	field := listStateField(path, name, description, group)
	field.UpdateInstruction = instruction
	return field
}

func objectStateFieldWithInstruction(path, name, description, group, instruction string) ActorStateField {
	field := presetStateField(path, name, "object", description, group, "")
	field.Default = map[string]any{}
	field.UpdateInstruction = instruction
	return field
}

func favorabilityStateField(path, name, description, group string) ActorStateField {
	field := presetStateField(path, name, "number", description+" "+actorStateFavorabilityGuidance, group, "stat")
	field.Default = float64(0)
	field.Min = presetFloatPointer(-100)
	field.Max = presetFloatPointer(100)
	// The number is the settlement's, not the narrator's. It arrives through
	// character_facts.affinities, which the projector turns into a settled
	// AffinityChange with a band and a basis; this field only carries the result
	// so the story ledger and the character window read one value. The write
	// instruction therefore points at that channel instead of inviting a second
	// number the rules never computed.
	field.UpdateInstruction = "该数值由好感结算写入，不要在 state_changes 中直接改写；会改变双方关系的有效互动或事件通过 character_facts.affinities 提交，并说明所依据的那一件事实。关系类型与关系阶段只随相应剧情事实更新，不根据好感度自动推导。"
	return field
}

func openEndedNumberStateField(path, name, description, group string, defaultValue, minValue float64, scale, instruction string) ActorStateField {
	field := presetStateField(path, name, "number", description+" 数值区间："+scale, group, "stat")
	field.Default = defaultValue
	field.Min = presetFloatPointer(minValue)
	field.UpdateInstruction = instruction
	return field
}

func scaledNumberStateField(path, name, description, group string, defaultValue, minValue, maxValue float64, scale, instruction string) ActorStateField {
	field := presetStateField(path, name, "number", description+" 数值区间："+scale, group, "stat")
	field.Default = defaultValue
	field.Min = presetFloatPointer(minValue)
	field.Max = presetFloatPointer(maxValue)
	field.UpdateInstruction = instruction
	return field
}

func presetStateField(path, name, fieldType, description, group, display string) ActorStateField {
	return ActorStateField{
		Path:        path,
		Name:        name,
		Type:        fieldType,
		Description: description,
		Group:       group,
		Display:     display,
	}
}

func presetFloatPointer(value float64) *float64 {
	return &value
}
