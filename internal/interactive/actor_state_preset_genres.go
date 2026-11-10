package interactive

func xiuxianActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:          ActorStateXiuxianID,
		Name:        "修仙状态系统",
		Description: "面向境界、灵根与特殊体质、功法法宝、宗门关系和天地异变；境界与六属性按《自制-修仙数值规则》V0.3 由规则结算，不由叙述改写。",
		PanelFields: []ActorStateField{
			xiuxianStageField("panel.realm", "当前境界"),
			// The two pools carry their current value and their maximum as
			// separate fields because the rules store them separately: a wound
			// is a real fact the stage table cannot reconstruct, and the design
			// requires the page to show both the real current value and the real
			// maximum. Folding them into one "12/40" text field would make the
			// current value unreadable to the settlement that has to clamp it.
			xiuxianPoolField("panel.health", "生命", "当前生命值；由受伤与治疗结算写入，结算时按实际生命上限截断。", xiuxianFieldHealthMax),
			xiuxianAttributeField("panel.health_max", "生命上限", "由境界阶段表与有效加成结算的生命上限。"),
			xiuxianPoolField("panel.mana", "灵力", "当前灵力值；由消耗与调息结算写入，结算时按实际灵力上限截断。", xiuxianFieldManaMax),
			xiuxianAttributeField("panel.mana_max", "灵力上限", "由境界阶段表与有效加成结算的灵力上限。"),
			xiuxianAttributeField("panel.attack", "攻击", "由境界阶段表与有效加成结算的攻击值。"),
			xiuxianAttributeField("panel.defense", "防御", "由境界阶段表与有效加成结算的防御值。"),
			xiuxianAttributeField("panel.agility", "身法", "由境界阶段表与有效加成结算的身法值。"),
			xiuxianAttributeField("panel.perception", "神识", "由境界阶段表与有效加成结算的神识值。"),
		},
		StateFields: []ActorStateField{
			xiuxianRealmProgressField(),
			xiuxianBurdenField("state.burden", "经脉负担", "当前经脉负担，0–100；由施展术法累加、按真实时间与轮次消解。"),
			xiuxianBurdenField("state.burden_floor", "余伤负担下限", "经脉余伤未愈时负担不能低于此值；未受伤时为 0。"),
			// The three equipment slots are declared because §4.1 allows exactly
			// one implement, one piece of armour and one formation to contribute.
			// They live in the actor's own state rather than in the world so a
			// branch that swaps equipment keeps its own copy.
			xiuxianImplementField(),
			xiuxianArmorField(),
			xiuxianFormationField(),
			// 《自制-装备栏与法宝御使》§1 gives a character one 攻击法宝, one 防御法宝
			// and one 辅助法宝 beside the weapon and the robe, so all three are
			// declared here. They are separate fields rather than one list because
			// the equip path picks the slot from the definition's kind.
			xiuxianAttackTreasureField(),
			xiuxianDefenseTreasureField(),
			xiuxianSupportTreasureField(),
			// §14 makes the base unit one whole 下品灵石 and forbids fractional
			// coin, so the purse is an integer count rather than mixed
			// denominations. It carries a default of zero because a character who
			// is not stated to have any money has none — that is a real fact, not
			// a missing one. The default must not swallow a stated amount: create
			// is the one turn allowed to write this settled field, so the
			// instruction tells the model to author it there whenever the prose
			// already says how much the person carries.
			openEndedNumberStateField("state.currency", xiuxianFieldCurrency,
				"随身可支配的下品灵石枚数；只计整枚，零头不以灵石计。",
				"行囊", 0, 0, "整数枚下品灵石。",
				"新建此人时，正文或设定已经交代他随身带多少灵石，就在 create 的 initial_state 里按实际枚数一次写足；这是叙述唯一一次给出持有量的机会，默认的 0 表示确实一文没有，不是“待定”。create 之后本项归规则，只由交易与明确的收支结算写入，叙述不再直接改写余额，也不因剧情需要凭空增加。"),
			textStateFieldWithDefaultInstruction("state.cultivation_method", "功法与专精", "记录当前主修功法及其专精方向（均衡／强攻／防护／轻身／神识）与品阶（普通／良好／高阶）；六属性按专精与品阶统一结算，不在此处手写加成数值。", "修为", "inline", "未定", "只在正式转修、功法进阶或设定被明确补充时更新；不因临场描写变化。"),
			listStateFieldWithInstruction("state.effects", "持续状态", "只列仍会影响修行或行动的伤势、封印、增益与异常。", "战斗与状态", "每项使用“名称｜影响｜解除条件或时长”；结束后移除，不记录已经结算的瞬时变化。会参与属性结算的增益（丹药、符箓、加持等）另用对象形式记录，键为“名称”“属性”“数值”“同类”“剩余轮次”：属性取 攻击／防御／身法／神识／生命上限／灵力上限，数值为百分点（增攻药下品 5、中品 8、上品 12、极品 15），同类为叠加分组（如 增攻药），剩余轮次为真实剩余轮数。同类取强不叠加，轮次归零即失效。"),
			listStateFieldWithInstruction("state.cooldowns", "冷却状态", "只列尚未恢复的功法、术法、神通或法宝。", "战斗与状态", "每项写名称与剩余时间、次数或恢复条件；恢复后移除。"),
		},
		ProtagonistFields: []ActorStateField{
			textStateField("cultivation.foundation", "灵根与特殊体质", "记录灵根、血脉、特殊体质、道基等会决定修行路径的根本特征；没有对应设定时保持空白。", "修为", "block"),
			xiuxianTalentField(),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("cultivation.profile", "根基与传承", "记录已知灵根、特殊体质和主要传承；境界、伤势与临时异常由对应分组字段维护，未知部分不估算。", "修为", "block"),
			// The growth settlement prices every tracked character from their own
			// arrangement and their own ground. Both are declared here rather than
			// in the shared character templates because only this genre has a
			// growth rule that reads them.
			xiuxianArrangementField(),
			xiuxianTrainingWindowField(),
		},
		OpponentFields: []ActorStateField{
			// The abilities this opponent really casts belong in 已学功法, not in
			// the free-form record beside it: the battle reads the learned set and
			// nothing else, so a technique that exists only as prose never gets
			// used. This instruction used to point the other way, and the observed
			// result was a boss who fought a whole encounter with plain attacks.
			textStateField("cultivation.threat_profile", "本相与传承特征", "记录已确认的妖身、法相、传承特征和境界压制方式。会真正在交手中施展、且已有登记定义的功法写入“已学功法”（首次出场时就已会的，一并在 create 的初始状态里写入）；规则没有登记定义的独门手段才记入“技能与能力”。", "修为", "block"),
			// A hostile cultivator the story keeps tracking lives through the same
			// intervals as anyone else, so it is priced from its own arrangement
			// too rather than from whatever the protagonist happens to be doing.
			xiuxianArrangementField(),
			xiuxianTrainingWindowField(),
		},
		StoryFields: []ActorStateField{
			textStateField("cultivation_world.state", "修行世界状态", "合并记录天地灵气、天道或飞升规则、宗门秩序及正在影响多地的修行界异变。", "概况", "block"),
			// §6 requires a technique generated inside a story to be a versioned
			// fact written into the same Story JSONL as the character's own
			// records, bound to that world, rules version, story and branch. A
			// definition registered here is one this story invented: it is not
			// shared between two games, and a branch inherits only the definitions
			// registered before the fork.
			xiuxianGeneratedSkillsField(),
			// §12.1 keeps a shop's funds apart from the character's own purse, so
			// the merchants the story has really dealt with are recorded on the
			// story context and not on the character. §12.4 allows each faction's
			// cash position to be prepared once, and this record is where that
			// one-time preparation is committed.
			xiuxianMerchantsField(),
		},
		AbilityGuidance:      "类型可使用功法、术法、神通、秘术、剑诀、身法或天赋；阶段与效果按正文设定表达，不换算成统一数值。",
		ItemGuidance:         "类型可使用法宝、法器、丹药、符箓、阵盘、灵材、灵石或传承物；记录品阶、认主、耐久或剩余次数等实际存在的状态。",
		RelationshipGuidance: "关系按师徒、同门、道侣、盟友、因果、恩怨、主从或敌对等修仙语义表达。",
		QuestGuidance:        "任务类型可使用修炼、突破、宗门、历练、秘境、因果或主线。",
		LocationGuidance:      "地点类型可使用洞府、宗门、坊市、秘境、禁地、城池、洞天或战场。",
		FactionGuidance:       "势力类型可使用宗门、世家、仙朝、魔门、妖族、商会、散修盟或上界势力。",
	}
}

func westernFantasyActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:          ActorStateWesternFantasyID,
		Name:        "西幻状态系统",
		Description: "面向职业与超凡位阶、魔法神术、种族血脉、装备、阵营势力和冒险任务；默认提供职业、等级与 AC/DC，其他属性按作品规则增加。",
		PanelFields: []ActorStateField{
			textStateFieldWithDefaultInstruction("panel.profession", "职业", "记录当前主要职业、兼职或等价成长路径。", "面板", "inline", "未定", "转职、兼职或职业身份正式变化时更新，不把临时伪装写入。"),
			scaledNumberStateField("panel.level", "等级", "当前职业体系采用的有效等级。", "面板", 1, 1, 20, "1–4 初阶；5–10 中阶；11–16 高阶；17–20 传奇。", "只随正式升级、降级或规则结算更新。"),
			scaledNumberStateField("panel.attack_ac", "攻击 AC", "参与攻击相关检定的有效值。", "面板", 10, 0, 30, "0–5 极低；6–9 偏低；10–13 常规；14–17 高；18–30 极高。", "装备、等级、祝福或诅咒改变攻击结算时更新。"),
			scaledNumberStateField("panel.defense_dc", "防御 DC", "参与防御相关检定的有效值。", "面板", 10, 0, 30, "0–5 极低；6–9 偏低；10–13 常规；14–17 高；18–30 极高。", "装备、等级、祝福或诅咒改变防御结算时更新。"),
		},
		StateFields: []ActorStateField{
			textStateFieldWithDefaultInstruction("state.health", "生命", "使用“当前值/上限”表达生命或等价耐久资源。", "状态", "inline", "10/10", "受伤、治疗或上限改变时更新。"),
			textStateFieldWithDefaultInstruction("state.spell_resource", "施法资源", "合并记录作品实际采用的法力、法术位、神术次数或充能；没有时留空。", "状态", "block", "", "施法、恢复或上限变化时更新；只写当前作品真实采用的资源。"),
			listStateFieldWithInstruction("state.effects", "持续效果", "只列仍在生效的祝福、诅咒、专注、伤势与异常。", "状态", "每项写名称、影响及剩余条件或时长；结束后移除。"),
			listStateFieldWithInstruction("state.cooldowns", "冷却状态", "只列尚未恢复的职业能力、法术或物品。", "状态", "每项写名称与剩余回合、时间、次数或恢复条件；恢复后移除。"),
		},
		ProtagonistFields: []ActorStateField{
			textStateField("fantasy.progression", "超凡来源与血脉契约", "记录魔法、神术或其他超凡来源，以及会持续影响能力的血脉与契约；职业等级放入面板。", "概况", "block"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("fantasy.profile", "超凡来源与阵营职责", "记录已知超凡来源、血脉契约和其在阵营中的职责；职业等级与检定项放入面板。", "概况", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("fantasy.threat_profile", "生物特性与抗性", "记录已确认的种族或生物类型、抗性与免疫；阶位与检定项放入面板，具体能力放入技能与能力。", "概况", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("fantasy_world.order", "魔法、信仰与政治秩序", "合并记录当前世界的魔法环境、神祇或教会规则、王国秩序及跨地区冲突。", "概况", "block"),
		},
		AbilityGuidance:      "类型可使用职业能力、法术、神术、战技、专长、血脉能力或仪式；记录法术环阶、次数、冷却、专注等作品实际采用的限制。",
		ItemGuidance:         "类型可使用武器、护甲、饰品、药剂、卷轴、材料、货币或任务物；记录品质、充能、耐久、诅咒或绑定等实际状态。",
		RelationshipGuidance: "关系按家族、同伴、效忠、契约、教会、雇佣、盟友、竞争或敌对等语义表达。",
		QuestGuidance:        "任务类型可使用主线、委托、探索、阵营、誓约、讨伐或个人任务。",
		LocationGuidance:     "地点类型可使用王国、城市、村镇、城堡、地城、遗迹、荒野、位面或神域。",
		FactionGuidance:      "势力类型可使用王国、贵族、教会、公会、学院、商会、部族、军团或邪教。",
	}
}

func apocalypseActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:          ActorStateApocalypseID,
		Name:        "末世状态系统",
		Description: "面向灾变求生、感染异变、稀缺资源、基地与幸存者冲突；默认不创建数值面板，也不为饥饿、口渴、疲劳逐项造字段。",
		StateFields: []ActorStateField{
			textStateFieldWithDefaultInstruction("state.survival_condition", "生存状态", "合并记录当前伤势、疲劳、饥渴、感染或污染中真正会影响行动的部分。", "状态", "block", "稳定", "只在生理或环境压力已经造成行动影响时写入，并注明影响与缓解条件；无影响时使用“稳定”。"),
			listStateFieldWithInstruction("state.effects", "持续效果", "只列仍在生效的增益、异常、药物效果或变异影响。", "状态", "每项写名称、行动影响及解除条件或时长；结束后移除。"),
			listStateFieldWithInstruction("state.cooldowns", "冷却状态", "只列尚未恢复的技能、装备或消耗品能力。", "状态", "每项写名称与剩余时间、次数或恢复条件；恢复后移除。"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("survival.profile", "生存专长与职责", "记录关键生存专长，以及其在队伍或基地中的职责；感染、污染和伤势放入状态。", "概况", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("survival.mutation_profile", "感染与变异特征", "合并记录传播方式、变异形态、感知方式和群体行为；具体攻击能力放入技能与能力。", "概况", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("apocalypse.situation", "灾变与生存局势", "合并记录灾变类型、感染或变异扩散、基础设施、区域污染和跨地点资源压力。", "概况", "block"),
			objectStateFieldWithInstruction("apocalypse.base", "基地状态", "基地存在时记录其当前运行状态；未建立基地时保持空 object。", "题材状态", "object 只写基地名称与地点名称（名称即 ID）、存续与安全状态、人员、关键储备、设施能力、当前威胁和紧急需求。地点名称使用故事语言并与地点记录一致。安全状态使用崩溃、危险、勉强、稳定、安全等文字等级；删除基地时 replace 为空 object。"),
		},
		AbilityGuidance:      "类型可使用生存、医疗、维修、战斗、驾驶、侦察、制造或谈判。",
		ItemGuidance:         "类型可使用武器、弹药、食物、饮水、药品、燃料、工具、防具或任务物；记录数量、耐久、弹药、保质或污染等实际状态。",
		RelationshipGuidance: "关系按团队信任、资源互助、保护、服从、竞争、交易或敌对等末世语义表达。",
		QuestGuidance:        "任务类型可使用生存、搜救、补给、撤离、调查、基地、势力或主线。",
		LocationGuidance:     "地点类型可使用安全屋、基地、城市、郊区、野外、设施、道路或污染区。",
		FactionGuidance:      "势力类型可使用幸存者团队、聚居地、军方、公司、邪教、匪帮或感染群体。",
	}
}

func infiniteFlowActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:          ActorStateInfiniteFlowID,
		Name:        "无限流状态系统",
		Description: "面向副本规则、任务结算、空间资源、规则污染、队伍博弈和异常实体；面板只承接空间明确公布的评级，不自行套用六维。",
		PanelFields: []ActorStateField{
			textStateFieldWithDefaultInstruction("panel.space_rating", "空间评级", "记录轮回空间或副本系统明确公布的等级、阶位或综合评级。", "面板", "inline", "未公布", "只在空间正式公布、结算或修正评级时更新；没有公开评级时保持“未公布”。"),
		},
		StateFields: []ActorStateField{
			textStateFieldWithDefaultInstruction("state.current_resources", "当前资源", "合并记录当前副本实际采用的生命、精神、积分或次数资源；没有的项目不写。", "状态", "block", "", "资源消耗、恢复或上限变化时更新，使用“名称 当前值/上限”逐项表达。"),
			listStateFieldWithInstruction("state.rule_effects", "规则影响", "只列正在作用于当前 Actor 的污染、死亡标记、规则限制、增益与异常。", "状态", "每项写名称、影响及解除条件或时限；已经结算或解除后移除。"),
			listStateFieldWithInstruction("state.cooldowns", "冷却状态", "只列尚未恢复的技能、血统能力或道具。", "状态", "每项写名称与剩余时间、次数或恢复条件；恢复后移除。"),
		},
		ProtagonistFields: []ActorStateField{
			textStateField("infinite_space.profile", "空间身份", "记录权限、队伍身份和已完成副本等长期信息；评级放入面板，积分与规则污染放入状态。", "概况", "block"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("infinite_space.role", "队伍角色与空间身份", "合并记录当前队伍职责和空间权限；已公开的个人任务统一写入当前目标与处境。", "概况", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("infinite_space.rule_profile", "触发、规避与规则影响", "合并记录触发条件、规避方式、规则领域和追击阶段；持续污染放入状态，只写已经确认或有充分线索支持的内容。", "概况", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("infinite_space.status", "轮回空间状态", "记录跨副本长期生效的空间规则、权限体系、结算秩序和当前整体局势。", "概况", "block"),
			objectStateFieldWithInstruction("infinite_space.current_instance", "当前副本", "记录当前副本的阶段、规则、时限和结算条件。", "题材状态", "object 只写副本名称、类型与难度、当前阶段与区域、剩余时间、任务名称（名称即 ID）、已确认规则、违规记录、核心威胁、结算和逃离条件。任务名称使用故事语言并与当前任务记录一致。稳定程度使用稳定、波动、崩坏等文字状态；副本结算后 replace 为空 object。"),
		},
		AbilityGuidance:      "类型可使用主动技能、被动技能、血统、天赋、临时能力或职业能力；记录次数、冷却、积分代价和规则限制等实际存在的约束。",
		ItemGuidance:         "类型可使用道具、消耗品、诅咒物、线索物、兑换物、任务物或装备；记录剩余次数、绑定、污染和诅咒等实际状态。",
		RelationshipGuidance: "关系按合作、竞争、资源债务、救命债、队伍承诺或敌对等副本语义表达。",
		QuestGuidance:        "任务类型可使用副本主线、支线、隐藏、生存、团队、个人或结算。",
		LocationGuidance:     "地点类型可使用空间大厅、副本区域、房间、规则节点、安全区、禁区或出口。",
		FactionGuidance:      "势力类型可使用轮回队伍、空间组织、原住民阵营、规则实体或敌对小队。",
	}
}
