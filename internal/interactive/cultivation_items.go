package interactive

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"ta/internal/cultivation"
)

// This file connects the character's real carried stock to the battle rules.
//
// §7 of 《自制-装备与物品品级》 separates a definition from a real copy: the
// catalogue holds definitions with fixed numbers, and a character holds copies
// with their own remaining quantity. Before this file, a pill could be registered
// and priced and the settlement could even pay for a lasting effect written by
// hand — but nothing ever read what the character actually carries, so a dose
// could never be spent in a fight and no consumption could be recorded.
//
// The reader prices every dose from the catalogue. §4.2 fixes 增攻药 at
// 5%／8%／12%／15% by grade, so the definition is the input and the percentage is
// the answer: a story that wrote 数值: 40 beside a registered 中品 pill is paid
// the grade's 8%, which is what makes "AI不能因戏剧需要把普通药物写成十倍攻击"
// structural rather than an instruction the model is asked to respect.

// xiuxianItemGradeWording and xiuxianItemRarityWording are the published §4
// wording for a definition's grade and rarity. The catalogue keys them in
// English and the story records them in Chinese, so the guidance has to render
// the same words xiuxianItemGrade and xiuxianItemRarity read back.
var xiuxianItemGradeWording = map[cultivation.ItemGrade]string{
	cultivation.GradeLower:  "下品",
	cultivation.GradeMiddle: "中品",
	cultivation.GradeUpper:  "上品",
	cultivation.GradeTop:    "顶品",
}

var xiuxianItemRarityWording = map[cultivation.ItemRarity]string{
	cultivation.RarityCommon:   "常见",
	cultivation.RarityUncommon: "少见",
	cultivation.RarityRare:     "稀有",
	cultivation.RarityPrecious: "珍稀",
	cultivation.RarityUnique:   "独特",
}

// xiuxianItemDefinitionGuidance names the definitions a carried entry may point
// at, so a story can record one the rules will really price.
//
// §7 requires the application layer to assemble the known items for the model to
// choose from. Without this list the model has no way to learn a definition's ID,
// and the only thing it could write under 定义 is an identifier the catalogue does
// not hold — which xiuxianCarriedDoses skips, so no dose could ever be carried
// into a real fight no matter how well the battle side worked. The list is
// generated from the catalogue rather than written out by hand, so adding an
// entry cannot leave the prompt naming a set that no longer exists.
//
// It says plainly that only a pill becomes a dose. A story that recorded a
// backpack of implements here expecting them to add attack would otherwise be
// reading its own instruction against §10's 背包法器不全加攻.
func xiuxianItemDefinitionGuidance(cultivationRules *cultivation.Rules) string {
	definitions := storyItemCatalog(cultivationRules).Definitions()
	parts := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		parts = append(parts, fmt.Sprintf("%s（%s·%s·%s）",
			definition.ID,
			xiuxianItemGradeWording[definition.Grade],
			xiuxianItemRarityWording[definition.Rarity],
			xiuxianItemEffectWording(definition)))
	}
	return "已登记物品只能经 items 模块入行囊：取得时用 items.acquire 写该定义的稳定标识与名称，规则据此写入“定义”“数量”“适用阶段”，不要在同一回合的 state_changes 里再手写一遍同一件物品，否则会被记成两份。" +
		"取得时可在同一处另写“描述”与“来源”：描述写这件东西看上去、摸上去、闻起来是什么样，一两句，可带一点机锋；来源写从谁手里、从什么地方得来。两项只是记事，不进任何数值；不要借描述复述效果、消耗或数字。同一件物品只写一次，之后装备、卸下、再取出都沿用最初那两句，不会随叙述改写。" +
		"一件实物只有一个持有者。若这件东西本来就在本故事某个已登记角色手里——馈赠、私下转让、从败者身上取下——取得时把 from 写成他的 Actor id，规则会在同一次提交里从他名下移出、记到你名下；不写 from 而此物已在别人手里会被拒绝。已经装备在身上的同一件东西不能再取得一次，要放回行囊请先 unequip。另取一把同名但确实是另一把的同类武器不受限制。" +
		"实际制作的产物须由craft结算，不能再以items.acquire重复取得；先用skills登记craft_design可推导更高品级或不同受支持属性的正式产物及其真实材料和工时。新创作且从外界实际取得的装备或消耗品可在 items.acquire 用 template_id 选下列同类效果模板，填写 name、source、description，规则会登记本局新定义再记录持有。品级、效果、价格与使用条件沿用模板，名称和背景可自由创作；独特物品与材料不能复制。生成档案按ID取用，安全上限16384项及4MiB，满时保留旧记录并拒绝新登记；旧内容可用search_generated_definitions按名称、ID或分页读取。普通叙事物件仍可按正文记述。模板或既有定义取以下之一：" +
		strings.Join(parts, "；") +
		"。丹药与符箓会成为战斗中真正可消耗的一份；记在此处的法器、护具与阵盘不计入属性，也不因稀有度自动获得加成。"
}

// xiuxianItemEffectWording states one definition's effect in the published terms.
func xiuxianItemEffectWording(definition cultivation.ItemDef) string {
	switch {
	case definition.Talisman != nil:
		return fmt.Sprintf("护身符：消耗一张，护盾按炼气一层基础生命的 %g%% 固定，持续 %d 轮，不叠加", definition.Talisman.ShieldFraction*100, definition.Talisman.DurationExchanges)
	case definition.Pill != nil:
		switch definition.Pill.Purpose {
		case cultivation.PillRestoreHealth:
			return "回复生命"
		case cultivation.PillRestoreMana:
			return "回复灵力"
		case cultivation.PillBoostAttack:
			if bonus, ok := definition.Pill.AttackBonus(); ok {
				return fmt.Sprintf("增攻 %g%%、持续 %d 轮", bonus*100, definition.Pill.DurationExchanges)
			}
		case cultivation.PillSupportCultivation:
			if strength, ok := definition.Pill.SupportStrengthOf(); ok {
				return fmt.Sprintf("%s、每份可支撑 %g 日修炼", xiuxianSupportStrengthWording(strength), definition.Pill.SupportDuration())
			}
		}
		return "丹药"
	case definition.Implement != nil:
		return fmt.Sprintf("攻击 +%g%%", definition.Implement.AttackBonus*100)
	case definition.Armor != nil:
		return fmt.Sprintf("防御 +%g%%", definition.Armor.DefenseBonus*100)
	case definition.Formation != nil:
		if definition.Formation.AttackBonus > 0 {
			return fmt.Sprintf("增攻 %g%%", definition.Formation.AttackBonus*100)
		}
		return fmt.Sprintf("防护 %g%%", definition.Formation.DefenseBonus*100)
	case definition.Material != nil:
		// A material grants nothing, and saying so is the honest entry: §2 of
		// 自制-修仙技艺 makes a material's whole contribution what a registered
		// method consumes it for. An empty parenthetical would read as an effect
		// the list failed to name, and the model would supply one.
		return "炼制材料，本身无效果"
	}
	return ""
}

// xiuxianSupportStrengthWording states one auxiliary strength in §6.2's own words.
func xiuxianSupportStrengthWording(strength cultivation.SupportResource) string {
	switch strength {
	case cultivation.SupportNormal:
		return "普通辅助"
	case cultivation.SupportStrong:
		return "强辅助"
	}
	return ""
}

// xiuxianPlanTrainingSupport decides which auxiliary resource a training
// interval really consumes, and how much of the interval it covers.
//
// §6.2 line 232 is the whole rule: an auxiliary resource must actually exist and
// be deducted, and once it runs out the rate changes from that instant rather
// than a single pill granting a permanent multiplier. So this reads the
// character's real carried stock, takes the strongest support the stock can pay
// for, and reports how many days that stock covers. The caller trains those days
// at the supported rate and the rest at the ordinary one, which is exactly the
// "从那个时间点换速率" the rule describes.
//
// Only one strength is used for the covered days. A mixed stock would otherwise
// have to be modelled as several rate segments, and the design has no need for
// that: a stronger dose is what a character would take first, and the remainder
// is ordinary training.
//
// The doses are counted from the interval the caller is about to run, so a
// seclusion longer than the stock covers spends the stock and then continues
// unsupported — it never stops the character for lack of a pill.
func xiuxianPlanTrainingSupport(cultivationRules *cultivation.Rules, state map[string]any, actorID string, days float64) (cultivation.SupportResource, []cultivation.CarriedDose, float64) {
	if days <= 0 {
		return cultivation.SupportNone, nil, 0
	}
	strength := cultivation.SupportNone
	for _, dose := range xiuxianCarriedDoses(cultivationRules,
		state, actorID) {
		if dose.Quantity <= 0 || dose.Pill.SupportDuration() <= 0 {
			continue
		}
		doseStrength, ok := dose.Pill.SupportStrengthOf()
		if !ok {
			continue
		}
		if strength == cultivation.SupportNone || doseStrength == cultivation.SupportStrong {
			strength = doseStrength
		}
	}
	if strength == cultivation.SupportNone {
		return cultivation.SupportNone, nil, 0
	}

	covered := 0.0
	var spent []cultivation.CarriedDose
	for _, dose := range xiuxianCarriedDoses(cultivationRules,
		state, actorID) {
		if covered >= days {
			break
		}
		if dose.Quantity <= 0 {
			continue
		}
		doseStrength, ok := dose.Pill.SupportStrengthOf()
		if !ok || doseStrength != strength {
			continue
		}
		perDose := dose.Pill.SupportDuration()
		if perDose <= 0 {
			continue
		}
		needed := int(math.Ceil((days - covered) / perDose))
		if needed <= 0 {
			break
		}
		if needed > dose.Quantity {
			needed = dose.Quantity
		}
		covered += float64(needed) * perDose
		remaining := dose.Quantity - needed
		dose.Quantity = remaining
		spent = append(spent, dose)
	}
	if covered > days {
		covered = days
	}
	if covered <= 0 {
		return cultivation.SupportNone, nil, 0
	}
	return strength, spent, covered
}

// xiuxianTrainingSupportOps writes the doses one interval really consumed.
//
// It goes through the same spent-record helper the battle settlement uses, so a
// dose spent training and a dose spent fighting leave the same kind of entry: the
// copy's own key with its new count, and a zero rather than a silent deletion
// when it is used up.
func xiuxianTrainingSupportOps(state map[string]any, actorID string, spent []cultivation.CarriedDose, turnID string) []ActorStateOp {
	if len(spent) == 0 {
		return nil
	}
	record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	if !ok {
		return nil
	}
	next, changed := xiuxianSpentCarriedRecord(record, spent)
	if !changed {
		return nil
	}
	return []ActorStateOp{{
		Op:           "set",
		ActorID:      normalizeStatePanelActorID(actorID),
		FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
		Value:        next,
		Reason:       "由修炼时段实际服用的辅助丹药扣除，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceCultivationAdvance,
		SourceID:     "cultivation",
	}}
}

// xiuxianCarriedDoses reads the consumable copies an actor really holds.
//
// Only pills become doses. An implement or a piece of armour recorded under
// 重要物品 stays there and contributes nothing, which is §10's 背包法器不全加攻
// read at the one place a battle could be tempted to break it.
//
// The record's own key is the instance identity. The state schema makes the key
// the item's stable name — 名称即 ID — so it survives a round, a restart and a
// branch fork without a second index, and two keys naming one 定义 are correctly
// two separate copies.
//
// A definition the catalogue does not hold, or one whose kind is not a pill, is
// skipped rather than trusted: an entry that claims to be a registered pill and
// is not has no published numbers to be priced from, so it is not stock the rules
// can spend.
func xiuxianCarriedDoses(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.CarriedDose {
	record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	if !ok {
		return nil
	}
	catalog := storyItemCatalog(cultivationRules,
		state)
	// The keys are sorted so a replay produces the same dose list, and therefore
	// the same window order and the same settled inventory, every time.
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var doses []cultivation.CarriedDose
	for _, key := range keys {
		entry, ok := record[key].(map[string]any)
		if !ok || len(entry) == 0 {
			continue
		}
		defID := xiuxianEntryText(entry, "定义")
		if defID == "" {
			// Without a registered definition there is no published number to price
			// the dose from, so the entry stays a narrative fact rather than becoming
			// stock the rules can spend.
			continue
		}
		definition, ok := catalog.Definition(defID)
		if !ok {
			continue
		}
		if definition.Kind == cultivation.KindTalisman && definition.Talisman != nil {
			talisman := *definition.Talisman
			doses = append(doses, cultivation.CarriedDose{InstanceID: strings.TrimSpace(key), DefID: definition.ID, NameKey: definition.NameKey, Quantity: xiuxianCarriedQuantity(entry), Talisman: &talisman})
			continue
		}
		if definition.Kind != cultivation.KindPill || definition.Pill == nil {
			continue
		}
		doses = append(doses, cultivation.CarriedDose{
			InstanceID: strings.TrimSpace(key),
			DefID:      definition.ID,
			NameKey:    definition.NameKey,
			Quantity:   xiuxianCarriedQuantity(entry),
			Pill:       *definition.Pill,
		})
	}
	return doses
}

// xiuxianCarriedQuantity reads how many doses a carried entry holds.
//
// An entry with no count is one copy: a story that wrote a pill without a number
// has recorded that the character has it, and reading that as zero would be the
// rules inventing an absence out of an omission. A negative or fractional count
// is floored at zero rather than rounded up, because rounding up would mint a
// dose the record never said existed.
func xiuxianCarriedQuantity(entry map[string]any) int {
	raw, ok := actorStateNumber(entry["数量"])
	if !ok {
		return 1
	}
	if raw <= 0 {
		return 0
	}
	return int(math.Floor(raw))
}

// xiuxianProseCarriedItems keeps the rules' copies and the story's own.
//
// 重要物品 holds two different kinds of entry. A copy with a registered
// definition has published numbers, a price and a count the rules own: it is
// written by items.acquire, by a settled battle and by the trade module, and
// §10 requires a purchase to deduct the whole amount exactly once. An entry the
// catalogue does not know is a narrative fact — the keepsake, the letter, the
// thing with no price — and the prose is the only writer it has.
//
// A whole-object replace therefore cannot be taken literally. A live run showed
// why: a sell turn wrote 回春丹 = 3 into 重要物品 and the trade module then
// deducted one more from that number, so the character ended the turn holding
// two doses after handing over one of four. The prose's registered entries are
// dropped and the committed ones kept, while its own unregistered entries are
// applied as written.
func xiuxianProseCarriedItems(cultivationRules *cultivation.Rules, current, proposed any, states ...map[string]any) any {
	proposedEntries, ok := proposed.(map[string]any)
	if !ok {
		return proposed
	}
	currentEntries, _ := current.(map[string]any)
	catalog := storyItemCatalog(cultivationRules,
		states...)
	merged := make(map[string]any, len(proposedEntries)+len(currentEntries))
	for key, value := range currentEntries {
		entry, _ := value.(map[string]any)
		if _, registered := catalog.Definition(xiuxianEntryText(entry, "定义")); registered {
			merged[key] = value
		}
	}
	for key, value := range proposedEntries {
		entry, _ := value.(map[string]any)
		if _, registered := catalog.Definition(xiuxianEntryText(entry, "定义")); registered {
			continue
		}
		merged[key] = value
	}
	return merged
}

// xiuxianDoseSettlementOps derives the inventory writes a settled battle
// produces.
//
// §4.2 requires the current stock, the consumption of one dose and the cost of
// the primary action to be really saved. The pools are written by
// combatSettlementOps; this is the other half, and it writes only the copies
// whose count actually moved.
//
// It writes the settled quantity as an absolute value rather than a delta, for
// the same reason the pools are written that way: the session holds the stock
// after the exchange, and a delta would accumulate the difference between two
// settle attempts. Because the write is absolute and is derived from the session
// the command identity fixed, a retried round re-states the same count instead of
// spending a second dose — the store never reaches this code twice for one
// command, and if it did the second write would be the same number.
func xiuxianDoseSettlementOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	session cultivation.CombatSession,
	eventID string,
) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	var ops []ActorStateOp
	for _, combatant := range session.Combatants {
		actorID := normalizeStatePanelActorID(combatant.ID)
		if actorID == "" || len(combatant.Carried) == 0 {
			continue
		}
		templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
		if !found {
			continue
		}
		template := actorStateTemplateByID(system, templateID)
		if !xiuxianDoseRecordIsWritable(template, system.CultivationRules) {
			continue
		}
		record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
		if !ok {
			continue
		}
		next, changed := xiuxianSpentCarriedRecord(record, combatant.Carried)
		if !changed {
			continue
		}
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      actorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
			Value:        next,
			Reason:       "由战斗结算写入服用消耗，不由叙述改写。",
			SourceTurnID: eventID,
			SourceKind:   StateOpSourceCombatSettlement,
			SourceID:     "combat",
		})
	}
	return normalizeActorStateOps(ops)
}

// xiuxianDoseRecordIsWritable reports whether this template can record a spent
// dose.
//
// A template that never declared 重要物品 cannot hold an inventory, and writing
// the field anyway would put a value in a schema that never declared it. The
// battle still spends the dose in the session; it simply has no record to write
// it back to.
func xiuxianDoseRecordIsWritable(template ActorStateTemplate, profiles ...*cultivation.Rules) bool {
	if !actorStateTemplateIsXiuxian(template, profiles...) {
		return false
	}
	return actorStateTemplateDeclaresField(template, xiuxianFieldImportantItems)
}

// xiuxianSpentCarriedRecord returns the carried record with each battle dose's
// settled count written onto the copy it came from, and whether anything moved.
//
// The copy is matched by its key, which is the instance identity
// xiuxianCarriedDoses read. A dose whose key is no longer in the record is left
// alone rather than appended: the battle's stock came from this record, so a
// missing key means the record moved underneath the battle, and inventing an
// entry would be a consumption of something the character no longer holds.
//
// A copy spent down to nothing keeps its entry at zero rather than disappearing.
// §6 requires the consumption to be part of the branch's saved history, and an
// entry that still names the pill with nothing left is a truer record of "this
// was used up" than a silent deletion — the engine refuses a dose from it with
// item_exhausted, which is exactly the fact the zero states.
func xiuxianSpentCarriedRecord(record map[string]any, carried []cultivation.CarriedDose) (map[string]any, bool) {
	next := make(map[string]any, len(record))
	for key, value := range record {
		next[key] = value
	}
	changed := false
	for _, dose := range carried {
		entry, ok := next[dose.InstanceID].(map[string]any)
		if !ok {
			continue
		}
		current, hasCurrent := actorStateNumber(entry["数量"])
		if hasCurrent && int(math.Round(current)) == dose.Quantity {
			continue
		}
		updated := make(map[string]any, len(entry)+1)
		for key, value := range entry {
			updated[key] = value
		}
		// Written as a whole number so the record reads back as a count rather than
		// as a float that happens to have no fraction.
		updated["数量"] = float64(dose.Quantity)
		next[dose.InstanceID] = updated
		changed = true
	}
	if !changed {
		return nil, false
	}
	return next, true
}
