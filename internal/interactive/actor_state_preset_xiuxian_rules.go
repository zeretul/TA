package interactive

import (
	"strings"

	"ta/internal/cultivation"
)

// This file declares the state fields the cultivation rules settle.
//
// The fields are declared here rather than in the preset spec because their
// shape is not a genre preference: it is dictated by the rules. The stage field
// offers exactly the keys cultivation.StageKeys() can read back, and the six
// attribute fields are the six names cultivation.Attributes carries. Keeping
// the two definitions in one place is what stops the schema and the rules from
// drifting into disagreeing about which stages or which attributes exist.

// xiuxianStageField is the canonical cultivation stage.
//
// It is an enum over the rules' own stage keys, not free text. The design
// requires that the model not be able to reword a character's cultivation, and
// a free text field is precisely a licence to reword it. An enum means the
// stored value is either a stage the rules recognise or a rejected write.
func xiuxianStageField(path, name string) ActorStateField {
	field := presetStateField(path, name, "enum",
		"角色在境界阶段表中的位置；取值即规则可结算的境界键，由修炼与突破结算写入，不由叙述改写。",
		"修为", "inline")
	field.Options = cultivation.StageKeys()
	field.Default = cultivation.StageKey(cultivation.Mortal())
	field.ValueFormat = ActorStateValueFormatStage
	field.UpdateInstruction = "只由修炼推进、突破结算、跌境或封印等规则结果更新；取值必须是给定选项之一，不自行改写境界称谓，也不为未实现的境界取值。"
	return field
}

// xiuxianAttributeField is one of the six settled attributes.
//
// The value is written by the settlement, not by the narrative: the design
// requires the panel and the battle view to show the real integers the rules
// produce. The maximum is left open because a stage's maximum rises by orders
// of magnitude across the table, and a closed bound would silently clip a
// late-stage character.
func xiuxianAttributeField(path, name, description string) ActorStateField {
	field := presetStateField(path, name, "number", description+" 由规则结算，叙述不得直接改写。", "战斗与状态", "stat")
	field.Default = float64(0)
	field.Min = presetFloatPointer(0)
	field.UpdateInstruction = "由修炼、突破、受伤、治疗、装备、丹药、阵法与其余有效来源的结算写入；不按临场描写手动增减，也不把加成重复计入。"
	return field
}

// xiuxianPoolField is one of the two current pools, bounded by a sibling
// maximum field.
//
// The two pools are the only settled numbers whose bound is another settled
// number rather than a constant, so they are the only fields that declare
// MaxField. Declaring it is what lets the page show 生命 200 / 200 and draw a
// real meter: without it the field has no maximum at all and the reader sees a
// bare number that cannot be read as a proportion of anything.
func xiuxianPoolField(path, name, description, maxField string) ActorStateField {
	field := xiuxianAttributeField(path, name, description)
	field.MaxField = maxField
	// The pools deliberately have no default. The six derived attributes can
	// start at zero because the settlement overwrites every one of them from the
	// stage key, but zero is the one value a pool must never be given for free:
	// it is the value that means "already down", and the settlement reads an
	// unwritten pool as full for exactly that reason. Materializing 0 here would
	// make every freshly created story open with its protagonist at 0 health and
	// 0 mana, and the guard that would have filled them could no longer tell an
	// untouched pool from an emptied one.
	field.Default = nil
	return field
}

// xiuxianRealmProgressField is the banked progress toward the current stage.
//
// The bound is open ended rather than a 0–100 percentage because the rules
// accumulate a real quantity: 100×L² for refining layer L, which is not a
// percentage of anything. Storing a percentage would make a long advance and
// the same advance split into pieces disagree, which the design forbids.
func xiuxianRealmProgressField() ActorStateField {
	field := presetStateField("state.realm_progress", "境界进度", "number",
		"当前阶段已积累的修炼进度，单位为规则进度值，门槛随阶段提高；不是百分比，也不等同于胜负或绝对战力。",
		"修为", "stat")
	field.Default = float64(0)
	field.Min = presetFloatPointer(0)
	field.UpdateInstruction = "只有修炼、机缘、损伤或突破结算明确影响进度时更新；小阶段转换与突破后按新阶段门槛重置或结转。"
	return field
}

// xiuxianBurdenField is the meridian burden and its residual floor.
//
// Both are bounded 0–100 because the rules bound them: burden accumulates to a
// capacity of 100 and the residual floor is a burden value. Declaring the real
// bound is what lets the interface draw a truthful meter instead of guessing a
// scale.
func xiuxianBurdenField(path, name, description string) ActorStateField {
	field := presetStateField(path, name, "number", description, "战斗与状态", "stat")
	field.Default = float64(0)
	field.Min = presetFloatPointer(0)
	field.Max = presetFloatPointer(100)
	field.UpdateInstruction = "由施展术法的负担结算、按轮次与真实时间消解、以及余伤的形成与痊愈更新；不在叙述中直接增减。"
	return field
}

// xiuxianImplementField is the one active main implement.
//
// It is a single object rather than a list because §4.1 allows exactly one
// implement to participate in the main attack and one piece of armour in the
// main defence. A list would invite the reader to expect every entry to
// contribute, which is precisely the 背包法器不全加攻 mistake the design names:
// carried items are recorded under 重要物品 and contribute nothing here.
//
// The sub-values are the published vocabulary of §4.1 — grade, rarity and how
// much of the rarity budget was spent on the main attribute — so the settlement
// can price the item against the same tables the catalogue validates against.
// The name is recorded for the player to read; it carries no rules weight.
func xiuxianImplementField() ActorStateField {
	field := presetStateField("equipment.implement", "主法器", "object",
		"当前参与主要攻击的一件法器或法宝；未装备、不能操控、借出或损坏时留空。写成对象，键为“名称”“定义”“品级”“稀有度”：“定义”取已登记物品标识，“品级”写下品／中品／上品／顶品／极品之一，“稀有度”写常见／少见／稀有／珍稀／独特之一——两项分开放，不要合并成“中品·少见”这样的单键，合并后规则读不到，这件法器就不参与结算。",
		"行囊", "")
	field.Default = map[string]any{}
	// The create exemption comes first on purpose. It used to sit at the end of a
	// long sentence that opened with 不要在 state_changes 中改写, and the observed
	// result was an opponent who learned a technique that needs its blade in hand
	// but never had 主法器 written: the model read the opening prohibition, put the
	// blade only under 重要物品, and the battle then refused the technique it had
	// just been given. Ordering the exemption first is what makes the two records
	// symmetric — 已学功法 already tells the model to write a first-appearance
	// technique into create.
	field.UpdateInstruction = "新建 Actor 首次出场时手里就已经拿着的那一件，直接在 create 的初始状态里写入本字段，连同重要物品一起写：那不是本回合取得的，没有可结算的取得动作。会真正在交手中施展、且需要手持法器的功法，缺了本字段就会被战斗拒绝，所以首次出场就必须写全。此后对它的任何变动走物品模块：取得时用 items.acquire 把该物品记入重要物品，装备时用 items.equip 指定副本名称，卸下时用 items.unequip 指定“主法器”。规则会校验定义是否登记、角色是否真的持有该副本、是否达到适用阶段并满足操控要求，并把品级、稀有度、主要属性预算与操控成本按定义写入。适用阶段不是点击颜色就能越过，境界不足的宝物装备会被拒绝。本字段不随叙述改写。"
	return field
}

// xiuxianArmorField is the one active piece of protective gear. It has the same
// shape as the implement because §4.1 prices both from one budget table.
func xiuxianArmorField() ActorStateField {
	field := presetStateField("equipment.armor", "护具", "object",
		"当前参与主要防御的一件护具；未装备、不能操控、借出或损坏时留空。本字段由物品规则写入，叙述只读不写。",
		"行囊", "")
	field.Default = map[string]any{}
	field.UpdateInstruction = "由物品模块写入，不要在 state_changes 中改写：装备用 items.equip 指定副本名称，卸下用 items.unequip 指定“护具”。规则校验定义、持有副本与适用阶段后，按定义写入品级、稀有度、主要属性预算与操控成本。护具只增加防御，不把同一个百分比同时加到六属性。"
	return field
}

// xiuxianFormationField is the one active formation.
//
// §4.3 requires a formation to record its real area, focus, activation and
// upkeep, and to stop working when it leaves the area, loses upkeep or has its
// focus destroyed. The field therefore records whether it is currently active
// rather than only what it is: an inactive formation contributes nothing, and
// making the reader state that is what stops a deployed-then-abandoned
// formation from quietly keeping its bonus.
//
// The slot is rules-owned like the implement and the armour. Deploying a
// formation is not a sentence: it costs 10% of the operator's stage base mana to
// raise and 2% per exchange to keep, and a story that could write the slot
// directly could grant the bonus without either payment.
func xiuxianFormationField() ActorStateField {
	field := presetStateField("equipment.formation", "布阵", "object",
		"当前由本人维持、仍在作用区域内的阵法；离开阵区、失去维持能力或阵盘被破坏后留空。本字段由物品规则写入，叙述只读不写。",
		"行囊", "")
	field.Default = map[string]any{}
	field.UpdateInstruction = "由物品模块写入，不要在 state_changes 中改写：取得阵盘时用 items.acquire 把阵盘记入重要物品，布阵时用 items.equip 指定阵盘名称，撤阵时用 items.unequip 指定“布阵”。规则按阵盘定义写入品级、稀有度、增益、启动耗灵与每轮维持，并从灵力中扣除启动消耗；灵力不足时布阵会被拒绝。每轮维持失败、离开阵区或阵盘被破坏后，用 items.unequip 撤去此阵。"
	return field
}

// xiuxianTreasureField builds one of the three 法宝 slots.
//
// The three are the same shape because 《自制-装备栏与法宝御使》§1 gives them the
// same rules: one copy each, contributing only what its registered definition
// says, with a real operating cost and no automatic activation. What differs is
// which kind the slot accepts, and that is decided by the definition rather than
// by the field's wording — the equip path maps a definition's kind onto its slot
// and refuses a mismatch.
//
// The field is rules-owned for the same reason the implement is. §2 says 装备不等于
// 自动发动全部能力 and §3 says 描述、背景故事和稀有度不能再次送一份数值, so a story
// that could write this slot directly could put a treasure in a character's hands
// without the copy being obtained, without the stage being checked and without
// the operating cost ever being paid.
func xiuxianTreasureField(
	path string,
	name string,
	purpose string,
	unequipWord string,
) ActorStateField {
	field := presetStateField(path, name, "object",
		purpose+"；未装备、不能操控、借出或损坏时留空。本字段由物品规则写入，叙述只读不写。",
		"行囊", "")
	field.Default = map[string]any{}
	field.UpdateInstruction = "由物品模块写入，不要在 state_changes 中改写：取得时用 items.acquire 把该宝物记入重要物品，装备时用 items.equip 指定副本名称，卸下时用 items.unequip 指定“" + unequipWord + "”。规则校验定义、持有副本与适用阶段后，按定义写入品级、稀有度、主要属性预算、增益属性与操控成本；境界不足时装备会被拒绝。三件法宝各自一个位置，互不顶替；同一件实物不能同时占武器与法宝位。装备不等于自动发动，主动效果仍要按定义支付行动与灵力。"
	return field
}

// xiuxianAttackTreasureField is the 攻击法宝 slot.
//
// §1 gives it 主动攻击、破防或其他已支持的攻击用途. It is not a second weapon:
// §1 says 武器与攻击法宝不是重复槽位 but also 同一实物只能占一个位置、执行一次动作,
// so a flying sword may be defined as either and a character may carry one of
// each, but one real blade never fills both.
func xiuxianAttackTreasureField() ActorStateField {
	return xiuxianTreasureField("equipment.attack_treasure", xiuxianFieldAttackTreasure,
		"当前御使的一件攻击法宝，用于主动攻击、破防等已登记的攻击用途",
		xiuxianFieldAttackTreasure)
}

// xiuxianDefenseTreasureField is the 防御法宝 slot: 护盾、格挡等已支持的防御用途.
func xiuxianDefenseTreasureField() ActorStateField {
	return xiuxianTreasureField("equipment.defense_treasure", xiuxianFieldDefenseTreasure,
		"当前御使的一件防御法宝，用于护盾、格挡等已登记的防御用途",
		xiuxianFieldDefenseTreasure)
}

// xiuxianSupportTreasureField is the 辅助法宝 slot.
//
// §1 names 探查、隐匿、束缚、脱身 as the uses and says explicitly 不因列了用途就自动
// 获得效果: the listed uses are what the category may cover, not a bundle the slot
// grants. The definition decides, which is why the field records the copy rather
// than a set of capabilities.
func xiuxianSupportTreasureField() ActorStateField {
	return xiuxianTreasureField("equipment.support_treasure", xiuxianFieldSupportTreasure,
		"当前御使的一件辅助法宝，用于探查、隐匿、束缚、脱身等已登记的用途",
		xiuxianFieldSupportTreasure)
}

// xiuxianLearnedSkillsField is the character's learned techniques.
//
// §6 of 《自制-技能与施展》 keeps three facts apart: a definition exists, a
// definition is registered, and a character has learned it. This field holds the
// third and only the third. Its entries name a registered definition and the
// version the character actually learned, so an upgrade that formally replaces a
// known version is visible in the record rather than inferred from the registry's
// current definition.
//
// The version is recorded per character rather than read live from the registry,
// because a character who learned version 1 keeps version 1's numbers until
// version 2 is formally granted to them. Reading the registry's current
// definition at cast time would silently rewrite what the character knows.
//
// The numbers are not recorded. Power, cost and burden live in the registered
// definition, and the play-time reader prices a cast from there, so a narrative
// cannot inflate a technique by editing this list.
func xiuxianLearnedSkillsField() ActorStateField {
	field := objectStateFieldWithInstruction("abilities.learned", "已学功法",
		"记录本人已登记、修习中与已学会的术法、神通、秘术及已学会制作方法。",
		"修为",
		"键为功法名称，子值使用“定义”“版本”“掌握程度”，另有“描述”“来历”两项记事文字。技能经 skills 登记并按实际minutes累计学习，普通/良好/高阶标准1/3/7日再按资质调整；0分钟仅登记，标记“修习中”不能施展，足时才“已学会”。首次学习须有完整资料或有效传授的source。制作方法按实际掌握授业，不套技能天数。定义为已登记功法定义的稳定标识（如 strike／fire_bolt／condensed_blade／heavy_thunder／flame_wave／shield／meridian_secret_blade；制作方法如 craft.talisman.ward，仅授予制作资格，不是战斗招式），版本为该角色实际学会的版本号。“描述”写这门功法施展起来是什么样子、什么手感，一两句，可带一点机锋；“来历”写从谁手里、从哪本册子或哪处遗迹得来。两项都在首次学会时由学习结算写入，之后固定不变，后续叙述不得改写；也不要在这里写威力、消耗或任何数值，那些一律由已登记定义决定。登记、修习进度和掌握程度均由学习结算写入；仅正文提到不授予能力，不能在state_changes里手写学习结果。定义不存在或版本未登记时不写，改由学习结算写入。新建 Actor 首次出场时就已经会、且定义已登记的功法，直接在 create 的初始状态里写入本字段，不必再走学习结算——这个人的过去不是本回合学到的。本故事自行造出的功法由学习结算按已批准模板登记，标识形如 gen.<模板>.<名称>，不要手写。")
	return field
}

// xiuxianTechniquesField is the rules-owned method record.
//
// It is private for the same reason the learned-skill list is: what a character
// has mastered is learned by finding out, not by reading a panel about someone
// else. The player still sees their own record, which is the point.
//
// The field is declared with an empty default object. A character whose record
// is empty is a character with no structured methods yet, and the settlement
// then falls back to the free-text 功法与专精 field, so a save written before
// this record existed keeps the numbers it had.
func xiuxianTechniquesField() ActorStateField {
	return objectStateFieldWithInstruction("cultivation.techniques", xiuxianFieldTechniques,
		"记录本人已登记、修习中、已掌握与当前运转的功法。功法由规则登记，属性加成由规则结算，不在此处写数值。",
		"修为",
		"键为功法名称，子值使用“定义”“版本”“掌握程度”“运转”，另有“描述”“来历”两项记事文字。定义为已登记功法定义的稳定标识（如 method.basic_breathing／method.warm_origin／method.cold_spring／method.origin_guard／method.flowing_wind／method.clear_mind／method.body_cycle／method.economical_breath），版本为该角色实际学会的版本号。掌握程度为“已掌握”或“修习中”；运转为“运转”或“停止”。“描述”写这门功法运转起来是什么样子、什么手感，一两句，可带一点机锋；“来历”写师承、家传或得自何处。两项在首次修习时由学习结算写入，之后固定不变，后续叙述不得改写；也不要在这里写效果、消耗或任何数值，那些一律由已登记定义决定。新功法可用 techniques.learn 的template_id/name/source登记同效模板，本局生成定义上限512；学习结算保存名称、已修习分钟、修习中/已掌握，登记不等于掌握。普通调整一次10分钟，温寒换运一次安全调息60分钟且结束负担为0；使用既有恢复规则，不免费补满血蓝、不把同一准备重复提交rest，战斗中不能切换。数值、调息状态与时间均由规则记录。")
}

// The settlement and the schema both read these, so a rename cannot update one
// and leave the other pointing at a field that no longer exists.
const (
	xiuxianFieldRealm         = "当前境界"
	xiuxianFieldHealth        = "生命"
	xiuxianFieldHealthMax     = "生命上限"
	xiuxianFieldMana          = "灵力"
	xiuxianFieldManaMax       = "灵力上限"
	xiuxianFieldAttack        = "攻击"
	xiuxianFieldDefense       = "防御"
	xiuxianFieldAgility       = "身法"
	xiuxianFieldPerception    = "神识"
	xiuxianFieldRealmProgress = "境界进度"
	// xiuxianFieldArrangement is how a character is spending their time. It is
	// the one fact the growth settlement reads to pick §4's row, so it is a
	// declared field rather than a phrase the settlement would have to parse.
	xiuxianFieldArrangement = "修行安排"
	// xiuxianFieldTrainingWindow is the safe window an outing really offers. It
	// is the source the rules require before 20% travel time may be claimed.
	xiuxianFieldTrainingWindow = "修炼间隙"
	xiuxianFieldBurden         = "经脉负担"
	xiuxianFieldBurdenFloor    = "余伤负担下限"
	xiuxianFieldImplement      = "主法器"
	xiuxianFieldArmor          = "护具"
	xiuxianFieldFormation      = "布阵"
	// The three 法宝 slots of 《自制-装备栏与法宝御使》§1. They are separate fields
	// rather than one list because §1 gives each kind exactly one slot, and the
	// item module's equip path picks the slot from the definition's kind — one
	// list would leave it unable to say which of the three a copy belongs in.
	//
	// They are new names rather than a rename of the old three. The old 主法器／
	// 护具／布阵 keep their names because a saved branch is read by field name
	// with no legacy fallback, so renaming would make every existing save lose
	// its equipment; and §4 already maps the old slots onto 武器／衣服法袍／阵法,
	// which is what they always carried.
	xiuxianFieldAttackTreasure  = "攻击法宝"
	xiuxianFieldDefenseTreasure = "防御法宝"
	xiuxianFieldSupportTreasure = "辅助法宝"
	xiuxianFieldEffects         = "持续状态"
	// xiuxianFieldTalent holds the stable id of the one 天赋特性 the character was
	// created with, and nothing else. It stores an id rather than a percentage
	// because the size is the rules' (internal/cultivation/innate_talent.go): a
	// number written into state would be a second place the same talent is
	// priced, and a fork or a retry could then leave two copies to add up.
	//
	// It is written once at creation and then rules-owned, so no turn can grant a
	// second talent, raise this one, or hand a character a talent they were not
	// born with.
	xiuxianFieldTalent        = "天赋特性"
	xiuxianFieldLearnedSkills = "已学功法"
	xiuxianFieldTechniques    = "功法"
	// xiuxianFieldSkillRegistry is the story's own registered definitions: the
	// techniques this story generated from approved templates. It lives on the
	// story-context actor because §6 binds a generated definition to the world
	// pack, rule version, story and branch rather than to a character, and because
	// a branch inherits only what was registered before the fork.
	xiuxianFieldSkillRegistry = "技能定义"
	// xiuxianFieldImportantItems is the carried-items record every actor
	// template declares, not only the cultivation one. It contributes nothing to
	// the six attributes — that is the 背包法器不全加攻 rule — but it is where a
	// consumable a character really holds is written, so it is the field a battle
	// reads a dose from and writes the spent dose back to.
	xiuxianFieldImportantItems = "重要物品"
	// xiuxianFieldCurrency is the character's own spendable 下品灵石.
	//
	// §14 makes the currency's base unit one whole 下品灵石 and forbids
	// fractional coin, so the field is an integer count rather than a purse of
	// mixed denominations. It is the character's own cash, not a faction
	// treasury: §12.1 keeps personal property, shop funds and faction treasuries
	// apart, and folding them into one number is exactly the mistake that would
	// let a sect's wealth appear in a disciple's pocket.
	xiuxianFieldCurrency = "灵石"
	// The arrangement wordings are the allocation table's own row names. They are
	// read by the growth settlement and offered by the field's options, so the
	// two cannot disagree about which rows exist.
	xiuxianArrangementUnstated   = "未定"
	xiuxianArrangementOrdinary   = "正常生活兼修行"
	xiuxianArrangementSeclusion  = "专心闭关"
	xiuxianArrangementTravel     = "外出历练"
	xiuxianArrangementOccupied   = "事务繁忙"
	xiuxianArrangementRecovering = "养伤"
	// xiuxianFieldMerchants is the story's record of merchants the player has
	// really dealt with.
	//
	// §12.4 allows the application to prepare a faction's cash position once, the
	// first time it genuinely matters, and forbids back-filling it afterwards to
	// suit the player. Keeping the prepared record here is what makes that
	// one-time promise structural: the numbers are committed state, so a later
	// turn reads them instead of choosing new ones.
	xiuxianFieldMerchants = "商贾"
)

// xiuxianGeneratedSkillsField is the story's record of techniques it generated.
//
// §6 requires a generated definition to be a versioned fact written into the same
// Story JSONL, bound to the world pack, rule version, story and branch, and says
// explicitly that definitions generated during play cannot be shared between two
// games. Keeping the record on the story-context actor is what makes that
// structural: it is part of the story's own state, so a fork carries what existed
// before it and a different story never sees it.
//
// It is declared with an empty default object so a story that generated nothing
// needs no entry, and so a save written before this record existed opens with an
// empty registry rather than a missing field.
func xiuxianGeneratedSkillsField() ActorStateField {
	field := objectStateFieldWithInstruction("cultivation.skill_definitions", xiuxianFieldSkillRegistry,
		"记录本局按已有规则模板登记的功法、装备与制作方法；由学习或物品规则写入。",
		"修为",
		"键为生成定义的稳定标识；子值记录类别、定义、版本、模板、名称、品级、稀有度、来源和描述。数值与实际条件取自对应规则模板，名称和背景由故事创作。生成档案独立于每轮上下文，按稳定ID取用；档案安全上限16384项及4MiB。达到容量时保留已有记录并拒绝新登记，不截断旧记录。")
	return field
}

// xiuxianMerchantsField records the shops and merchants the story has really
// dealt with, together with the cash position each was prepared with.
//
// It belongs to the story context rather than to any character because a shop is
// part of the world, and §12.1 keeps a shop's funds apart from the character's
// own purse. The values are written by the trade settlement, so §12.4's promise
// that a faction's cash position is prepared once and never back-filled is a
// property of the committed state rather than a rule the prose is asked to
// remember.
func xiuxianMerchantsField() ActorStateField {
	return objectStateFieldWithInstruction("economy.merchants", xiuxianFieldMerchants,
		"记录已经实际交易过的商家及其现银与回购率。由交易结算一次性写入，不由叙述改写。",
		"经济",
		"键为商家名称，子值使用“现银”（整枚下品灵石）与“回购率”（千分率，500–700）。现银是这家商铺自己能动用的钱，不是玩家的余额，也不等于它的全部家产。")
}

// xiuxianSettledFields lists the field IDs the cultivation settlement owns.
//
// A field in this list is written only by the settlement. The design requires
// that the model not be able to reword a character's cultivation, and that
// requirement is only real if the model's own writes to these fields are
// refused rather than merely discouraged.
//
// The current pools are deliberately absent. A wound or an expenditure is a
// real fact the narrative does record, and the settlement's job for those two
// fields is to clamp them to the derived maximum rather than to author them.
// Listing them here would leave the story unable to record that anyone was hurt.
func xiuxianSettledFields() map[string]bool {
	return map[string]bool{
		xiuxianFieldRealm:         true,
		xiuxianFieldHealthMax:     true,
		xiuxianFieldManaMax:       true,
		xiuxianFieldAttack:        true,
		xiuxianFieldDefense:       true,
		xiuxianFieldAgility:       true,
		xiuxianFieldPerception:    true,
		xiuxianFieldRealmProgress: true,
		xiuxianFieldBurden:        true,
		xiuxianFieldBurdenFloor:   true,
		xiuxianFieldLearnedSkills: true,
		xiuxianFieldTechniques:    true,
		xiuxianFieldSkillRegistry: true,
		// §4.1's equipment slots are rules-owned because equipping is a move of a
		// real copy, not a description. A story that could write 主法器 directly
		// could put a 顶品 treasure in a 炼气 character's hand without the copy ever
		// being obtained and without the applicable stage being checked, which is
		// exactly what 适用阶段不是点击颜色就能越过 forbids. The slot is written by
		// the item module's equip and unequip.
		//
		// 布阵 joined them once §4.3's costs became real: deploying a formation is
		// paid for at the moment it is raised (10% of the operator's stage base
		// mana) and every exchange after (2%). A story that could write the slot
		// itself could claim the bonus without either payment. Leaving the area or
		// losing the focus is still recorded — through items.unequip, which is the
		// path that also returns the 阵盘 to the pack.
		xiuxianFieldImplement: true,
		xiuxianFieldArmor:     true,
		xiuxianFieldFormation: true,
		// A 天赋特性 is decided at birth, so it is rules-owned for the same reason
		// the stage is: a story that could write it could hand a character a talent
		// they were not born with, and the settlement would then pay a bonus no
		// creation ever recorded. The create branch writes it once from the
		// confirmed identity; afterwards only the character's own state carries it.
		xiuxianFieldTalent: true,
		// The three 法宝 slots join them for the same reason: §1 gives each one
		// slot and §2 says equipping is not activating, so a slot the story could
		// write itself would be a treasure with neither an acquisition nor an
		// operating cost behind it.
		xiuxianFieldAttackTreasure:  true,
		xiuxianFieldDefenseTreasure: true,
		xiuxianFieldSupportTreasure: true,
		// The purse is rules-owned for the same reason the equipment slots are:
		// money is a fact about what really changed hands, not a description.
		// §4.3 fixes prices against the world content version and forbids them
		// moving with the player's wallet, and §10 requires a purchase to deduct
		// the whole amount or nothing. A story that could write 灵石 directly
		// could pay for anything for free, which is the one thing a trade system
		// exists to prevent. It is written by the trade module's settlement.
		xiuxianFieldCurrency: true,
		// 商贾 is the other purse, one level out. §12.1 keeps a shop's funds apart
		// from the character's own, and §12.4 allows its cash position to be
		// prepared once and forbids back-filling it later to suit the player. The
		// field's own instruction already said 由交易结算一次性写入，不由叙述改写,
		// but nothing enforced it: a whole-object replace of 商贾 compiled, so a
		// turn could raise a shop's 现银 and then buy from money the shop never
		// had. This is F18's 无权限不能调动组织资源 reached through the one
		// organisation purse the game actually models — it is a check on an
		// existing boundary, not a permission platform.
		xiuxianFieldMerchants: true,
	}
}

// xiuxianArrangementField is how a character is spending this stretch of their
// life, in the terms §4's allocation table is written in.
//
// It is a declared field rather than prose read for keywords because §三 forbids
// the settlement guessing: "不根据自由文本关键词任意判'灵脉”闭关'". The wording is
// the table's own row names, so the record and the table cannot drift.
//
// 未定 is a real option and the default. A character whose arrangement the story
// has not established is living their life on the ordinary row, which is the
// conservative default §三 allows; it is not a claim of seclusion.
// xiuxianTalentField is the one 天赋特性 a character was born with.
//
// It holds the rules' stable id and never a percentage, because the size of a
// talent is the rules' table (internal/cultivation/innate_talent.go). Storing a
// number here instead would make this field a second place the same talent is
// priced: a fork inheriting the state, or a retried commit, would then be able
// to add a copy of the bonus the creation never granted.
//
// It is written once by the creation overlay and is a settled field afterwards,
// so a story cannot hand a character a talent they were not born with.
func xiuxianTalentField() ActorStateField {
	return textStateFieldWithInstruction("cultivation.talent", xiuxianFieldTalent,
		"出生时带来的一项天赋特性，只记规则认可的名称；没有就是没有，不写“无”或“待定”。",
		"修为", "inline",
		"只在建角时由确认的身份写入，之后不由叙述改写；剧情不会凭空给出或更换天赋特性。")
}

func xiuxianArrangementField() ActorStateField {
	field := presetStateField("cultivation.arrangement", xiuxianFieldArrangement, "enum",
		"该角色当前时段的修行安排，决定结算时投入修炼的有效比例；只写已经由剧情确立的安排，未确立时保持未定。",
		"修为", "inline")
	field.Options = xiuxianArrangementOptions()
	field.Default = xiuxianArrangementUnstated
	field.UpdateInstruction = "只在剧情明确改变该角色的长期安排时更新：开始或结束闭关、外出、被事务占满、进入养伤。临时的一句话状态不写入；不改写境界、进度或属性。"
	return field
}

// xiuxianTrainingWindowField is the established window an outing really offers.
//
// §4 pays 长期外出 20% only "且确有安全适用机会". The opportunity is a fact about
// this character's outing — a place they can sit unseen for a while — not about
// the protagonist's scene, so it is recorded beside the arrangement. An empty
// window is the honest answer for an outing with none, and the settlement then
// prices the interval at 0% rather than inventing an opportunity.
func xiuxianTrainingWindowField() ActorStateField {
	return textStateFieldWithInstruction("cultivation.training_window", xiuxianFieldTrainingWindow,
		"外出期间确实存在的、可以安全调息修炼的时段与条件；没有这样的机会时留空。",
		"修为", "block",
		"只记录已经确立的安全修炼机会，写清何时、何处、为何不被打扰；与修行安排一并更新，不写期待中的机会。")
}

func xiuxianArrangementOptions() []string {
	return []string{
		xiuxianArrangementUnstated,
		xiuxianArrangementOrdinary,
		xiuxianArrangementSeclusion,
		xiuxianArrangementTravel,
		xiuxianArrangementOccupied,
		xiuxianArrangementRecovering,
	}
}

// ActorStateSystemIsXiuxian reports whether an actor-state system is the
// cultivation rules system.
//
// The system itself carries no module ID — a frozen story snapshot holds the
// templates, not the module that produced them — so identity is established
// structurally, from the one field only this system declares: a stage field
// whose options are exactly the keys the rules can read back. A genre that
// happens to name a field 当前境界 but does not offer the rules' own stage set
// is therefore correctly not treated as this system.
func ActorStateSystemIsXiuxian(system StoryDirectorActorStateSystem) bool {
	system = normalizeActorStateSystem(system)
	for _, template := range system.Templates {
		if actorStateTemplateIsXiuxian(template, system.CultivationRules) {
			return true
		}
	}
	return false
}

// actorStateTemplateIsXiuxian reports whether one template carries the rules'
// stage field.
//
// Identity is the stage field's option set rather than the field's name: a genre
// that happens to name a field 当前境界 but does not offer the rules' own stage
// set is correctly not treated as this system.
func actorStateTemplateIsXiuxian(template ActorStateTemplate, profiles ...*cultivation.Rules) bool {
	if len(profiles) > 0 && profiles[0] != nil {
		keys := profiles[0].StageOrder
		for _, f := range template.Fields {
			if actorStateFieldID(f) != xiuxianFieldRealm || len(f.Options) != len(keys) {
				continue
			}
			same := true
			for i, k := range keys {
				if f.Options[i] != k {
					same = false
					break
				}
			}
			if same {
				return true
			}
		}
		return false
	}
	keys := cultivation.StageKeys()
	// Old stories froze the human-realm table. Recognize that exact version
	// without rewriting its options or granting higher-realm progression.
	legacyCount := 1 + cultivation.QiLayers + 4*3
	for _, field := range template.Fields {
		if actorStateFieldID(field) != xiuxianFieldRealm {
			continue
		}
		if len(field.Options) != len(keys) && len(field.Options) != legacyCount {
			continue
		}
		expected := make(map[string]bool, len(field.Options))
		for _, key := range keys[:len(field.Options)] {
			expected[key] = true
		}
		for _, option := range field.Options {
			delete(expected, strings.TrimSpace(option))
		}
		if len(expected) == 0 {
			return true
		}
	}
	return false
}

// xiuxianTemplateSupportsStage keeps the story's frozen realm boundary binding
// for player and NPC growth even when the engine learns additional realms.
func xiuxianTemplateSupportsStage(template ActorStateTemplate, key string) bool {
	for _, field := range template.Fields {
		if actorStateFieldID(field) != xiuxianFieldRealm {
			continue
		}
		for _, option := range field.Options {
			if option == key {
				return true
			}
		}
	}
	return false
}

// xiuxianDerivedFields lists the fields that are a pure function of the stage
// key, with no independent input of their own.
//
// The distinction from xiuxianSettledFields matters at the story's opening. The
// stage key is a story fact the opening turn has to state — a character is at some
// realm when the story begins — but the six attributes are not: the settlement
// derives every one of them from that key. Asking the model to author them would
// be asking it to write the numbers the rules own, which is the thing the design
// forbids, so they are excluded from the opening's required coverage and refused
// to the model on every turn.
func xiuxianDerivedFields() map[string]bool {
	return map[string]bool{
		xiuxianFieldHealthMax:  true,
		xiuxianFieldManaMax:    true,
		xiuxianFieldAttack:     true,
		xiuxianFieldDefense:    true,
		xiuxianFieldAgility:    true,
		xiuxianFieldPerception: true,
	}
}

// IsXiuxianDerivedField reports whether a field is derived from the stage key
// under the given actor-state system.
func IsXiuxianDerivedField(system StoryDirectorActorStateSystem, fieldID string) bool {
	if !ActorStateSystemIsXiuxian(system) {
		return false
	}
	return xiuxianDerivedFields()[normalizeActorStateFieldName(fieldID)]
}

// IsXiuxianSettledField reports whether a field is owned by the cultivation
// settlement under the given actor-state system.
//
// It is false for every other preset, so the settlement's ownership is a
// property of the xiuxian system rather than of the field name alone: a
// different genre is free to use the same wording for its own field.
func IsXiuxianSettledField(system StoryDirectorActorStateSystem, fieldID string) bool {
	if !ActorStateSystemIsXiuxian(system) {
		return false
	}
	return xiuxianSettledFields()[normalizeActorStateFieldName(fieldID)]
}

// xiuxianFieldNameForAttribute maps a rules attribute onto the field that
// stores it.
//
// The mapping exists so the settlement cannot write an attribute to the wrong
// field: there is one place that answers "where does attack go", and an
// attribute with no field is a compile-time absence rather than a silent miss.
func xiuxianFieldNameForAttribute(attribute cultivation.AttributeName) (string, bool) {
	switch attribute {
	case cultivation.AttrHealthMax:
		return xiuxianFieldHealthMax, true
	case cultivation.AttrManaMax:
		return xiuxianFieldManaMax, true
	case cultivation.AttrAttack:
		return xiuxianFieldAttack, true
	case cultivation.AttrDefense:
		return xiuxianFieldDefense, true
	case cultivation.AttrAgility:
		return xiuxianFieldAgility, true
	case cultivation.AttrPerception:
		return xiuxianFieldPerception, true
	}
	return "", false
}

// normalizeCultivationSpecialisationText reads the specialisation out of the
// free text field that records the character's method.
//
// The field is prose because a method has a name and a description, but the
// settlement needs the enumerated template. This reads the template out of the
// prose and reports whether it found one; an unreadable value is reported as
// absent rather than guessed, so a method the rules do not know contributes no
// bonus instead of a wrong one.
func normalizeCultivationSpecialisationText(text string) (cultivation.TechniqueSpecialisation, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	for _, candidate := range []cultivation.TechniqueSpecialisation{
		cultivation.SpecialisationBalanced,
		cultivation.SpecialisationAttack,
		cultivation.SpecialisationDefense,
		cultivation.SpecialisationLight,
		cultivation.SpecialisationSpirit,
	} {
		if strings.Contains(text, string(candidate)) {
			return candidate, true
		}
	}
	// Accept the published Chinese wording as well, because the method field is
	// written in the story's own language.
	byWording := map[string]cultivation.TechniqueSpecialisation{
		"均衡": cultivation.SpecialisationBalanced,
		"强攻": cultivation.SpecialisationAttack,
		"防护": cultivation.SpecialisationDefense,
		"轻身": cultivation.SpecialisationLight,
		"神识": cultivation.SpecialisationSpirit,
	}
	for wording, specialisation := range byWording {
		if strings.Contains(text, wording) {
			return specialisation, true
		}
	}
	return "", false
}

// normalizeCultivationQualityText reads the technique grade out of the same
// method field. Like the specialisation, an unreadable grade contributes no
// bonus rather than a guessed one.
func normalizeCultivationQualityText(text string) (cultivation.TechniqueQuality, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	for _, candidate := range []cultivation.TechniqueQuality{
		cultivation.QualityOrdinary,
		cultivation.QualityGood,
		cultivation.QualityAdvanced,
	} {
		if strings.Contains(text, string(candidate)) {
			return candidate, true
		}
	}
	byWording := map[string]cultivation.TechniqueQuality{
		"普通": cultivation.QualityOrdinary,
		"良好": cultivation.QualityGood,
		"高阶": cultivation.QualityAdvanced,
	}
	for wording, quality := range byWording {
		if strings.Contains(text, wording) {
			return quality, true
		}
	}
	return "", false
}
