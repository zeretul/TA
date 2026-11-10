package interactive

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"ta/internal/cultivation"
)

// This file turns the actor's recorded equipment and lasting effects into the
// attribute modifiers the settlement aggregates.
//
// It exists because §3.4 of 《自制-修仙数值规则》 requires every effective source
// to be summed in ONE pass. Before it, the settlement read only the stage key and
// passed no modifiers at all, so an equipped implement, a worn piece of armour, a
// deployed formation and a boosting pill were all invisible to the numbers the
// player reads — the rules existed and were tested, but nothing during play ever
// fed them.
//
// The reader derives the numbers rather than trusting them. A recorded implement
// carries a grade, a rarity and how much of the rarity budget went into the main
// attribute, and the bonus is computed from the published tables. That is what
// makes "AI不能因戏剧需要把普通药物写成十倍攻击" structural: there is no
// recorded bonus for a narrative to inflate, only a grade to look up.

// xiuxianEquipmentSlots lists the field IDs that hold one active item each.
//
// They are read in a fixed order so a replay produces the same modifier list,
// and therefore the same aggregated integers, every time.
//
// The three 法宝 slots are appended after the three older slots rather than
// interleaved with them. The order is what a replay depends on, so appending is
// what keeps a branch recorded before the treasure slots existed producing the
// same list it always did.
var xiuxianEquipmentSlots = []string{
	xiuxianFieldImplement,
	xiuxianFieldArmor,
	xiuxianFieldFormation,
	xiuxianFieldAttackTreasure,
	xiuxianFieldDefenseTreasure,
	xiuxianFieldSupportTreasure,
}

// xiuxianSlotOrder is the order the item module writes its slots in.
//
// It is the same list as xiuxianEquipmentSlots, named separately because the two
// answer different questions — one is what the settlement reads, the other is
// what a turn writes — and a reader that assumes they must stay in step is
// easier to keep correct than a literal repeated in a third place.
var xiuxianSlotOrder = xiuxianEquipmentSlots

// storyItemCatalog is the catalogue a story's items are priced from.
//
// It is built from the rules' own first-version table, so the set of definitions
// and their numbers are fixed by the rule version rather than assembled per
// turn. It is the item-side counterpart of storySkillRegistry.
// With a state, include only definitions registered on that Story branch.
func storyItemCatalog(cultivationRules *cultivation.Rules, states ...map[string]any) *cultivation.ItemCatalog {
	if len(states) == 0 {
		return cultivation.NewItemCatalogWithFirstVersion(cultivationRules)
	}
	return storyItemCatalogFromState(cultivationRules,
		states[0])
}

// xiuxianEquipmentModifiers collects every real, sourced modifier the actor's
// recorded equipment and lasting effects contribute.
//
// It returns nil for an actor with nothing equipped, which is the ordinary case
// and the reason the settlement's output is unchanged for a character who owns
// nothing.
func xiuxianEquipmentModifiers(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.AttributeModifier {
	var modifiers []cultivation.AttributeModifier
	// A slot is priced only if the character can really operate what is in it.
	// The equip path already refuses a copy above the character's stage, but
	// this reader is the one that pays the slot, and a slot the item module
	// never wrote — one an authored state op filled, or one a fork inherited
	// — would otherwise be paid at full value by a character who could not
	// wield it. §4.1 is explicit that 适用阶段不是点击颜色就能越过.
	stage, stageKnown := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	usable := func(entry map[string]any) bool {
		if !stageKnown {
			// A stage the rules cannot read cannot be compared, and assuming it
			// reaches a floor the definition declared would grant a number the
			// rules never confirmed. The recorded copy still exists; it just
			// contributes nothing until the stage is readable.
			return false
		}
		definition, ok := xiuxianItemDefinition(cultivationRules,
			entry, state)
		if !ok {
			// A copy with no registered definition has no published floor to
			// check. It is priced by grade and rarity like any other, which is
			// the behaviour these records had before definitions existed.
			return true
		}
		return definition.UsableBy(stage) == nil
	}
	if entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImplement); ok && usable(entry) {
		if bonus, ok := xiuxianItemMainBonus(entry); ok {
			modifiers = append(modifiers, cultivation.AttributeModifier{
				ID:        "implement:" + xiuxianItemName(entry),
				Kind:      cultivation.SourceMainImplement,
				Attribute: cultivation.AttrAttack,
				Value:     bonus,
			})
		}
	}
	if entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldArmor); ok && usable(entry) {
		if bonus, ok := xiuxianItemMainBonus(entry); ok {
			modifiers = append(modifiers, cultivation.AttributeModifier{
				ID:        "armor:" + xiuxianItemName(entry),
				Kind:      cultivation.SourceArmor,
				Attribute: cultivation.AttrDefense,
				Value:     bonus,
			})
		}
	}
	if entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldFormation); ok {
		modifiers = append(modifiers, xiuxianFormationModifiers(cultivationRules,
			entry, state)...)
	}
	for _, slot := range xiuxianTreasureSlots {
		if entry, ok := xiuxianEquipmentEntry(state, actorID, slot); ok && usable(entry) {
			modifiers = append(modifiers, xiuxianTreasureModifiers(cultivationRules,
				slot, entry, state)...)
		}
	}
	modifiers = append(modifiers, xiuxianInnateTalentModifiers(cultivationRules,
		state, actorID)...)
	modifiers = append(modifiers, xiuxianLastingEffectModifiers(cultivationRules,
		state, actorID)...)
	return modifiers
}

// xiuxianInnateTalentModifiers reads the one 天赋特性 a character was born with.
//
// The field carries the talent's name, and its size comes from the rules' table
// — never from the field and never from the pack. That is what makes the bonus
// impossible to duplicate: the state holds a name, so a retried commit, a
// re-read or a fork that copies the state still yields exactly one modifier, at
// exactly the published size. A name the rules do not know contributes nothing
// rather than a guessed number, which is the same rule the equipment readers
// follow for an unreadable grade.
func xiuxianInnateTalentModifiers(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.AttributeModifier {
	raw, ok := actorStateFieldValue(state, actorID, xiuxianFieldTalent).(string)
	if !ok {
		return nil
	}
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	talent, ok := cultivation.InnateTalentByName(text, cultivationRules)
	if !ok {
		// A save written with the stable id instead of the name still resolves:
		// both are the rules' own spellings of the same talent.
		talent, ok = cultivation.InnateTalentByID(text, cultivationRules)
	}
	if !ok {
		return nil
	}
	return []cultivation.AttributeModifier{cultivation.InnateTalentModifier(talent)}
}

// xiuxianTreasureSlots is the three 法宝 slots, in the fixed order the settlement
// reads them.
//
// It is a slice of its own rather than a tail of xiuxianEquipmentSlots because
// the treasure reader has to know that all three are treasures, while the slot
// list also carries the implement, the armour and the formation.
var xiuxianTreasureSlots = []string{
	xiuxianFieldAttackTreasure,
	xiuxianFieldDefenseTreasure,
	xiuxianFieldSupportTreasure,
}

// xiuxianTreasureModifiers turns one equipped 法宝 into its modifier.
//
// 《自制-装备栏与法宝御使》§3 keeps the old framework: a treasure's bonus is an
// ordinary sourced positive modifier, summed with the implement, the armour, the
// formation, the techniques and the pills under the same 60%／40% ceiling. §1's
// warning that 新增3个法宝位置是增加可选择的战术工具，不自动给每件再加一份攻击／防御 is
// what the reader enforces structurally — a treasure contributes only the one
// attribute its definition names, at the size its definition published, and an
// entry that names none contributes nothing.
//
// §2's 装备不等于自动发动全部能力 is the other half. This reader reports what the
// equipped copy grants while it is equipped; an active capability the definition
// also publishes still has to be invoked through the action it is registered
// with, and is not paid for here.
func xiuxianTreasureModifiers(cultivationRules *cultivation.Rules, slot string, entry map[string]any, states ...map[string]any) []cultivation.AttributeModifier {
	// A copy obtained before the catalogue held a definition for it has no
	// published bonus to read. It contributes nothing rather than a number
	// derived from its recorded wording, which is the same rule the implement
	// and the armour follow.
	definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, states...)
	if !ok {
		return nil
	}
	if definition.Treasure == nil {
		return nil
	}
	attribute := definition.Treasure.BonusAttribute
	if attribute == "" || definition.Treasure.Bonus <= 0 {
		return nil
	}
	return []cultivation.AttributeModifier{{
		ID:        "treasure:" + slot + ":" + xiuxianItemName(entry),
		Kind:      cultivation.SourceTreasure,
		Attribute: attribute,
		Value:     definition.Treasure.Bonus,
	}}
}

// xiuxianTreasureOperatingCost is the real mana one equipped 法宝 costs to drive
// for an exchange.
//
// It mirrors xiuxianImplementOperatingCost: the fraction is read from the
// definition, never from the recorded copy, and it is computed against the bare
// stage mana so a boost cannot inflate the price. A treasure with no published
// fraction costs nothing, which is the honest reading for one the rules never
// gave an operating cost.
func xiuxianTreasureOperatingCost(cultivationRules *cultivation.Rules, state map[string]any, actorID, slot string) int {
	entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
	if !ok {
		return 0
	}
	definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, state)
	if !ok || definition.Treasure == nil {
		return 0
	}
	stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	if !ok {
		return 0
	}
	return definition.OperatingManaCost(stage)
}

// xiuxianEquippedCopies reads the copies really occupying the five positions of
// 《自制-装备栏与法宝御使》§1, as the engine's own equipment state.
//
// The battle needs this as live state rather than as a modifier list because §2
// lets a spare replace a copy mid-fight: the copy that leaves has to stop paying
// and the one that arrives has to start, in the same exchange the swap was paid
// for. A list folded once at the opening would keep the old copy's bonus for the
// rest of the fight.
//
// The position comes from the recorded field, which §4's migration maps from the
// item's real use — 主法器 is the weapon, 护具 the garment — rather than from the
// item's name. A slot the character's template never declared simply has no copy
// in it.
func xiuxianEquippedCopies(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.EquipmentCopy {
	stage, hasStage := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	var copies []cultivation.EquipmentCopy
	for _, slot := range xiuxianSlotOrder {
		engineSlot, ok := xiuxianEngineEquipmentSlot(slot)
		if !ok {
			continue
		}
		entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
		if !ok {
			continue
		}
		copies = append(copies, xiuxianEquipmentCopy(cultivationRules,
			slot, engineSlot, entry, stage, hasStage, state))
	}
	return copies
}

// xiuxianEngineEquipmentSlot maps a recorded field onto the engine's position.
//
// 布阵 has no position here on purpose: §4 keeps a deployed formation a real
// formation, raised and ended through its own live state, rather than turning it
// into a treasure that can be swapped in from the pack.
func xiuxianEngineEquipmentSlot(fieldID string) (cultivation.EquipmentSlot, bool) {
	switch fieldID {
	case xiuxianFieldImplement:
		return cultivation.EquipmentSlotWeapon, true
	case xiuxianFieldArmor:
		return cultivation.EquipmentSlotGarment, true
	case xiuxianFieldAttackTreasure:
		return cultivation.EquipmentSlotAttackTreasure, true
	case xiuxianFieldDefenseTreasure:
		return cultivation.EquipmentSlotDefenseTreasure, true
	case xiuxianFieldSupportTreasure:
		return cultivation.EquipmentSlotSupportTreasure, true
	}
	return "", false
}

// xiuxianEquipmentCopy prices one recorded slot for the battle.
//
// The modifiers are the ones this same file already derives for the settlement,
// so the battle and the panel cannot disagree about what a copy grants. The
// driving cost is read from each real definition. Weapons pay on attack, while
// powered armor and treasures pay when their registered effect is invoked.
func xiuxianEquipmentCopy(cultivationRules *cultivation.Rules, fieldID string, slot cultivation.EquipmentSlot, entry map[string]any, stage cultivation.Cultivation, hasStage bool, states ...map[string]any) cultivation.EquipmentCopy {
	copy := cultivation.EquipmentCopy{
		InstanceID: xiuxianItemName(entry),
		DefID:      xiuxianEntryText(entry, "定义"),
		Name:       xiuxianItemName(entry),
		Slot:       slot,
	}
	if definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, states...); ok {
		copy.ApplicableStage = definition.ApplicableStage
	}
	switch slot {
	case cultivation.EquipmentSlotWeapon:
		if bonus, ok := xiuxianItemMainBonus(entry); ok {
			copy.Modifiers = []cultivation.AttributeModifier{{
				ID:        "implement:" + copy.Name,
				Kind:      cultivation.SourceMainImplement,
				Attribute: cultivation.AttrAttack,
				Value:     bonus,
			}}
		}
		if definition, ok := xiuxianItemDefinition(cultivationRules,
			entry, states...); ok && definition.Kind == cultivation.KindImplement && hasStage {
			copy.OperatingManaCost = definition.OperatingManaCost(stage)
			copy.DefensePierce = definition.DefensePierceAmount(stage)
		}
	case cultivation.EquipmentSlotGarment:
		if bonus, ok := xiuxianItemMainBonus(entry); ok {
			copy.Modifiers = []cultivation.AttributeModifier{{
				ID:        "armor:" + copy.Name,
				Kind:      cultivation.SourceArmor,
				Attribute: cultivation.AttrDefense,
				Value:     bonus,
			}}
		}
		if definition, ok := xiuxianItemDefinition(cultivationRules,
			entry, states...); ok && definition.Kind == cultivation.KindArmor && hasStage {
			copy.OperatingManaCost = definition.OperatingManaCost(stage)
			copy.StrainReduction = definition.EquipmentStrainReduction(stage)
		}
	default:
		copy.Modifiers = xiuxianTreasureModifiers(cultivationRules,
			fieldID, entry, states...)
		if definition, ok := xiuxianItemDefinition(cultivationRules,
			entry, states...); ok && hasStage {
			copy.OperatingManaCost = definition.OperatingManaCost(stage)
		}
	}
	return copy
}

// xiuxianEquipmentSpares reads the equipment sitting in the pack.
//
// §1 is explicit that 多余宝物可以放在行囊…不能因为放在包里就加属性. That is a
// statement about where a bonus is read from, not about what a spare records:
// equipmentModifiers only ever reads the positions, so a spare's numbers sit
// unused until the copy is taken up. Carrying them here is what lets the swap
// change a number in the same exchange it was paid for — a spare priced at
// nothing would make the change a label.
//
// The numbers come from the registered definition rather than from the pack
// record, because the pack record deliberately holds only what the copy is and
// how many there are. Reading them from the definition keeps one source: a copy
// that leaves a position and later comes back is priced the same way both times.
func xiuxianEquipmentSpares(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.EquipmentCopy {
	record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	if !ok {
		return nil
	}
	stage, hasStage := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	catalog := storyItemCatalog(cultivationRules,
		state)
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var spares []cultivation.EquipmentCopy
	for _, key := range keys {
		entry, ok := record[key].(map[string]any)
		if !ok || len(entry) == 0 {
			continue
		}
		definition, ok := catalog.Definition(xiuxianEntryText(entry, "定义"))
		if !ok || !definition.Equippable() {
			continue
		}
		fieldID, err := xiuxianItemSlotForKind(definition.Kind)
		if err != nil {
			continue
		}
		slot, ok := xiuxianEngineEquipmentSlot(fieldID)
		if !ok {
			continue
		}
		// A stack in the pack is still one object for the purposes of a position.
		// An exhausted copy is not something a character can take out and wear.
		if xiuxianCarriedQuantity(entry) <= 0 {
			continue
		}
		name := strings.TrimSpace(key)
		priced := xiuxianItemSlotEntry(definition, name, stage,
			xiuxianEntryText(entry, "描述"), xiuxianEntryText(entry, "来历"))
		spare := xiuxianEquipmentCopy(cultivationRules,
			fieldID, slot, priced, stage, hasStage, state)
		// The pack key is the copy's identity; the priced record's own name is
		// what the panel shows, and they are the same object either way.
		spare.InstanceID = name
		spares = append(spares, spare)
	}
	return spares
}

// xiuxianItemDefinition resolves an equipment entry's registered definition.
//
// A story that wrote 主法器: {名称: "青锋剑", 品级: "下品"} has recorded a priced item
// without naming a catalogue entry, and there is no definition to resolve: the
// entry is priced from its recorded grade, which is what the settlement already
// does. An entry that names 定义 is a copy the rules handed out, and only that
// copy carries an applicable stage and a real operating cost.
//
// The lookup is by definition ID rather than by name, because a story may call
// the same treasure anything.
// xiuxianFieldForEngineSlot is the inverse of xiuxianEngineEquipmentSlot.
//
// The settlement writes a position by the field name the actor's template
// declares, while a battle holds the position by the engine's own name, so one
// direction has to be spelled out somewhere. Keeping both directions here means
// a new position is added in one place and both readers see it.
func xiuxianFieldForEngineSlot(slot cultivation.EquipmentSlot) (string, bool) {
	switch slot {
	case cultivation.EquipmentSlotWeapon:
		return xiuxianFieldImplement, true
	case cultivation.EquipmentSlotGarment:
		return xiuxianFieldArmor, true
	case cultivation.EquipmentSlotAttackTreasure:
		return xiuxianFieldAttackTreasure, true
	case cultivation.EquipmentSlotDefenseTreasure:
		return xiuxianFieldDefenseTreasure, true
	case cultivation.EquipmentSlotSupportTreasure:
		return xiuxianFieldSupportTreasure, true
	}
	return "", false
}

// xiuxianEquipmentSwapSettlementOps derives the writes a mid-fight change of
// equipment produces.
//
// §2 requires the change to outlast the window — 关闭窗口、换视图、暂停或重开不能绕过该
// 动作和消耗 — so the positions the battle ended with are what the branch keeps.
// The session holds them, and this writes the ones that really moved.
//
// It writes the position as an absolute value for the same reason the pools are
// written that way: a retried round states the same arrangement rather than
// moving a second copy. Only a position whose occupant differs from the recorded
// one is written, so a fight in which nobody changed anything produces no
// operation and the journal stays honest about what the round did.
func xiuxianEquipmentSwapSettlementOps(
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
		if actorID == "" || len(combatant.Equipment) == 0 {
			continue
		}
		templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
		if !found {
			continue
		}
		template := actorStateTemplateByID(system, templateID)
		if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
			continue
		}
		stage, hasStage := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), system.CultivationRules)
		if !hasStage {
			continue
		}
		catalog := storyItemCatalog(system.CultivationRules,
			state)

		recorded, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
		nextPack := make(map[string]any, len(recorded)+1)
		for key, value := range recorded {
			nextPack[key] = value
		}
		packChanged := false

		actorOps := make([]ActorStateOp, 0, len(cultivation.SwappableEquipmentSlots)+1)
		for _, engineSlot := range cultivation.SwappableEquipmentSlots {
			fieldID, ok := xiuxianFieldForEngineSlot(engineSlot)
			if !ok || !actorStateTemplateDeclaresField(template, fieldID) {
				continue
			}
			held, heldNow := xiuxianEquipmentEntry(state, actorID, fieldID)
			heldName := ""
			if heldNow {
				heldName = xiuxianItemName(held)
			}
			live, liveNow := combatant.EquipmentInSlot(engineSlot)
			if heldNow == liveNow && heldName == live.Name {
				continue
			}
			// The copy that came out returns to the pack with the wording it was
			// recorded with. §1 keeps an item owned either way — only which position
			// it pays from changes — so dropping it here would destroy it.
			if heldNow && heldName != "" {
				returned := xiuxianItemPackEntryFromSlot(held)
				key := heldName
				if existing, packed := nextPack[heldName].(map[string]any); packed && !xiuxianEntriesShareDefinition(existing, returned) {
					// A copy of another grade under the same name keeps its own key;
					// joining the stack would rewrite its definition to the pack's.
					key = xiuxianDistinctPackKey(nextPack, heldName, returned)
				}
				nextPack[key] = returned
				packChanged = true
			}
			if !liveNow {
				actorOps = append(actorOps, combatSettlementOp(actorID, fieldID, map[string]any{}, eventID))
				continue
			}
			// The copy that went in leaves the pack. Its description and source travel
			// with it, so the panel reads the same sentence whether the object is in
			// the pack or in hand.
			definition, ok := catalog.Definition(live.DefID)
			if !ok {
				continue
			}
			description, source := "", ""
			if entry, ok := nextPack[live.Name].(map[string]any); ok {
				description = xiuxianEntryText(entry, "描述")
				source = xiuxianEntryText(entry, "来历")
			}
			delete(nextPack, live.Name)
			packChanged = true
			actorOps = append(actorOps, combatSettlementOp(actorID, fieldID,
				xiuxianItemSlotEntry(definition, live.Name, stage, description, source), eventID))
		}
		if !packChanged || !actorStateTemplateDeclaresField(template, xiuxianFieldImportantItems) {
			ops = append(ops, actorOps...)
			continue
		}
		actorOps = append(actorOps, ActorStateOp{
			Op:           "set",
			ActorID:      actorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
			Value:        nextPack,
			Reason:       "由战斗结算写入换装结果，不由叙述改写。",
			SourceTurnID: eventID,
			SourceKind:   StateOpSourceCombatSettlement,
			SourceID:     "combat",
		})
		ops = append(ops, actorOps...)
	}
	return normalizeActorStateOps(ops)
}

// xiuxianItemDefinition reads the registered definition an entry was priced from.
func xiuxianItemDefinition(cultivationRules *cultivation.Rules, entry map[string]any, states ...map[string]any) (cultivation.ItemDef, bool) {
	defID := xiuxianEntryText(entry, "定义")
	if defID == "" {
		return cultivation.ItemDef{}, false
	}
	return storyItemCatalog(cultivationRules,
		states...).Definition(defID)
}

// xiuxianImplementOperatingCost is the real mana the implement in hand costs to
// drive for one exchange.
//
// 数值规则 line 128 requires it: 普通攻击的真实法器操控成本依装备定义，不因按普攻按钮
// 免费施展. It is read from the definition rather than from the recorded copy, so a
// story cannot make its own treasure cheaper by writing a smaller number, and it
// is computed against the wielder's bare stage mana rather than their current
// maximum so a boost cannot inflate the price.
//
// It returns 0 for an entry with no registered definition and for one whose
// definition publishes no fraction — the ordinary case, and not a missing number.
func xiuxianImplementOperatingCost(cultivationRules *cultivation.Rules, state map[string]any, actorID string) int {
	entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImplement)
	if !ok {
		return 0
	}
	definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, state)
	if !ok || definition.Kind != cultivation.KindImplement {
		return 0
	}
	stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules)
	if !ok {
		return 0
	}
	return definition.OperatingManaCost(stage)
}

// xiuxianHasMainImplement reports whether a focus is really in this actor's
// hands right now.
//
// It answers presence, not price. A story that recorded 主法器: "青锋剑" has put a
// sword in the character's hand even though it never priced the grade, and
// refusing every focus technique because of an omission in the pricing would be
// the rules inventing a handicap out of a narrative gap. The grade is only
// consulted where a bonus is actually needed — xiuxianItemMainBonus — which is a
// different question.
//
// It exists so a technique that RequiresFocus is gated on a real fact rather
// than on a caller's convenience, which is what §七.4 asks for: an enemy may
// perform only what it can really operate.
func xiuxianHasMainImplement(state map[string]any, actorID string) bool {
	raw := actorStateFieldValue(state, actorID, xiuxianFieldImplement)
	switch typed := raw.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]any:
		return len(typed) > 0
	default:
		return strings.TrimSpace(stringifyEntryValue(raw)) != ""
	}
}

// xiuxianEquipmentEntry reads one equipment slot as an object.
//
// A slot that holds a string, a number or nothing is treated as empty rather
// than as a malformed item: a story that wrote 主法器: "青锋剑" has recorded a
// name, not a priced item, and pricing it would require inventing a grade.
func xiuxianEquipmentEntry(state map[string]any, actorID, fieldID string) (map[string]any, bool) {
	raw := actorStateFieldValue(state, actorID, fieldID)
	entry, ok := raw.(map[string]any)
	if !ok || len(entry) == 0 {
		return nil, false
	}
	return entry, true
}

// xiuxianItemMainBonus prices an implement or a piece of armour from the
// published §4.1 budget tables.
//
// The grade and rarity are read as either the rules' own keys or the published
// Chinese wording, because the field is written in the story's language while
// the tables are keyed in English. An unreadable grade is reported as absent so
// an unrecognised item contributes nothing rather than a guessed ceiling.
func xiuxianItemMainBonus(entry map[string]any) (float64, bool) {
	grade, ok := xiuxianItemGrade(entry)
	if !ok {
		return 0, false
	}
	rarity, ok := xiuxianItemRarity(entry)
	if !ok {
		return 0, false
	}
	spend := xiuxianItemBudgetSpend(entry)
	bonus, ok := cultivation.MainImplementBonus(grade, rarity, spend)
	if !ok {
		return 0, false
	}
	return bonus, true
}

// xiuxianItemGrade reads a grade from an equipment entry.
func xiuxianItemGrade(entry map[string]any) (cultivation.ItemGrade, bool) {
	text := xiuxianEntryText(entry, "品级")
	switch text {
	case "下品":
		return cultivation.GradeLower, true
	case "中品":
		return cultivation.GradeMiddle, true
	case "上品":
		return cultivation.GradeUpper, true
	case "顶品", "极品":
		return cultivation.GradeTop, true
	}
	for _, candidate := range []cultivation.ItemGrade{
		cultivation.GradeLower, cultivation.GradeMiddle, cultivation.GradeUpper, cultivation.GradeTop,
	} {
		if text == string(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// xiuxianItemRarity reads a rarity from an equipment entry.
func xiuxianItemRarity(entry map[string]any) (cultivation.ItemRarity, bool) {
	text := xiuxianEntryText(entry, "稀有度")
	switch text {
	case "常见":
		return cultivation.RarityCommon, true
	case "少见":
		return cultivation.RarityUncommon, true
	case "稀有":
		return cultivation.RarityRare, true
	case "珍稀":
		return cultivation.RarityPrecious, true
	case "独特":
		return cultivation.RarityUnique, true
	}
	for _, candidate := range []cultivation.ItemRarity{
		cultivation.RarityCommon, cultivation.RarityUncommon, cultivation.RarityRare,
		cultivation.RarityPrecious, cultivation.RarityUnique,
	} {
		if text == string(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// xiuxianItemBudgetSpend reads how much of the rarity budget went into the main
// attribute, in percentage points.
//
// The published budget is stated in percentage points (0/2/5/8/10), so the field
// records points and this converts to the fraction the tables use. An entry that
// names a special effect without stating a spend has spent nothing on the main
// attribute, which is the case §4.1 requires to be priced at its grade alone.
func xiuxianItemBudgetSpend(entry map[string]any) float64 {
	points, ok := actorStateNumber(entry["主要属性预算"])
	if !ok {
		return 0
	}
	points = math.Max(0, points)
	return points / 100
}

// xiuxianFormationModifiers turns one recorded formation into its modifier.
//
// An inactive formation contributes nothing. §4.3 makes leaving the area, losing
// upkeep and losing the focus real ways for a formation to stop working, and
// reading the recorded state is what stops an abandoned formation from keeping
// its bonus.
//
// A formation deployed through the item module names its definition, and is
// priced from that definition's published bonus rather than from the recorded
// wording: §4.3's table is a property of the entry, so a story cannot write a
// larger 增益 beside it. A formation the narrative wrote by hand has no
// definition and keeps the grade-based path, which is the honest reading for a
// formation the rules never catalogued.
func xiuxianFormationModifiers(cultivationRules *cultivation.Rules, entry map[string]any, states ...map[string]any) []cultivation.AttributeModifier {
	if !xiuxianFormationActive(entry) {
		return nil
	}
	if definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, states...); ok && definition.Kind == cultivation.KindFormation && definition.Formation != nil {
		return xiuxianFormationBonusModifiers(xiuxianItemName(entry),
			definition.Formation.AttackBonus, definition.Formation.DefenseBonus)
	}
	grade, ok := xiuxianItemGrade(entry)
	if !ok {
		return nil
	}
	gain := xiuxianEntryText(entry, "增益")
	switch gain {
	case "攻击", "attack":
		bonus, ok := cultivation.FormationAttackBonus(grade)
		if !ok || bonus <= 0 {
			return nil
		}
		return xiuxianFormationBonusModifiers(xiuxianItemName(entry), bonus, 0)
	case "防御", "defense":
		bonus, ok := cultivation.FormationDefenseBonus(grade)
		if !ok || bonus <= 0 {
			return nil
		}
		return xiuxianFormationBonusModifiers(xiuxianItemName(entry), 0, bonus)
	}
	return nil
}

// xiuxianFormationBonusModifiers builds the one main modifier a formation
// contributes.
//
// §4.3 gives a first-version formation one main bonus and refuses a definition
// that claims two, so at most one of these is ever non-zero.
func xiuxianFormationBonusModifiers(name string, attack, defense float64) []cultivation.AttributeModifier {
	if attack > 0 {
		return []cultivation.AttributeModifier{{
			ID:        "formation:" + name,
			Kind:      cultivation.SourceFormation,
			Attribute: cultivation.AttrAttack,
			Value:     attack,
		}}
	}
	if defense > 0 {
		return []cultivation.AttributeModifier{{
			ID:        "formation:" + name,
			Kind:      cultivation.SourceFormation,
			Attribute: cultivation.AttrDefense,
			Value:     defense,
		}}
	}
	return nil
}

// xiuxianFormationState reads a deployed formation as the rules' live state.
//
// The battle needs the formation as a state rather than as a fixed modifier,
// because §4.3 lets upkeep end it mid-fight: a modifier copied into the
// combatant's list would keep paying after the operator stopped paying for it.
// The definition is resolved from the catalogue so the bonus and both costs are
// the published ones, and a hand-written formation the rules never catalogued
// produces an inactive state rather than an unpriced bonus.
//
// The recorded 维持中 flag is honoured. A formation the story already recorded
// as stopped is not raised again merely by entering a fight.
func xiuxianFormationState(cultivationRules *cultivation.Rules, state map[string]any, actorID string) cultivation.FormationState {
	entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldFormation)
	if !ok || !xiuxianFormationActive(entry) {
		return cultivation.FormationState{}
	}
	definition, ok := xiuxianItemDefinition(cultivationRules,
		entry, state)
	if !ok || definition.Kind != cultivation.KindFormation || definition.Formation == nil {
		return cultivation.FormationState{}
	}
	return cultivation.FormationState{Def: *definition.Formation, Active: true}
}

// xiuxianIntrinsicModifiers drops the sources a battle carries as live state.
//
// The settlement prices a deployed formation from its recorded slot, which is
// the right source outside battle. Inside battle the same bonus arrives through
// the live FormationState instead, so carrying it in both places would count it
// twice — and worse, would keep counting it after upkeep had ended the
// formation.
//
// The equipped copies are dropped for the same reason and one more: §2 lets a
// spare replace a copy mid-fight, so the battle reads them from its own
// positions. A copy left in this list would keep paying the item that was put
// away, and the swap would change nothing but the label.
//
// The character's own build — the technique specialisation, the technique grade
// and any lasting boost — stays: those have no live term to track.
func xiuxianIntrinsicModifiers(modifiers []cultivation.AttributeModifier) []cultivation.AttributeModifier {
	if len(modifiers) == 0 {
		return nil
	}
	kept := make([]cultivation.AttributeModifier, 0, len(modifiers))
	for _, modifier := range modifiers {
		switch modifier.Kind {
		case cultivation.SourceFormation,
			cultivation.SourceMainImplement,
			cultivation.SourceArmor,
			cultivation.SourceTreasure:
			continue
		}
		kept = append(kept, modifier)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// xiuxianFormationActive reports whether a recorded formation is still running.
//
// An entry that omits the flag is treated as active, because the story recorded
// a deployed formation and the field's own instruction is to clear it once it
// stops. An entry that says it is not running is honoured.
func xiuxianFormationActive(entry map[string]any) bool {
	text := xiuxianEntryText(entry, "维持中")
	return text != "否" && text != "false" && text != "no"
}

// xiuxianLastingEffectModifiers reads the temporary numeric effects in force.
//
// The effects are entries in the actor's own lasting-effects list, which is the
// one channel §5 of the item design requires: an effect is settled from the
// effect record, not once from the item description and again from a skill
// record. An entry that is a plain sentence is a narrative status with no
// numbers, and contributes nothing.
//
// The stacking group is read from 同类, so two boosts of the same kind replace
// each other through selectEffective rather than adding up. That is §4.2's
// 同类取强, and it is enforced by the aggregation rather than by this reader
// remembering to deduplicate.
//
// An entry that names a registered pill definition is priced from the catalogue's
// grade table rather than from its recorded 数值. That is what makes §4.2's
// 增攻药 5%／8%／12%／15% a property of the pill rather than of the prose: a story
// that records a 中品 boost cannot write 40 into the entry and have it paid, and
// a 下品 and a 上品 of the same pill differ by the table instead of by whatever
// number the narrative chose. An entry with no 定义 is a blessing, a talisman or
// another effect the rules do not catalogue, and keeps its recorded number —
// which is the honest answer for a source with no registered definition.
func xiuxianLastingEffectModifiers(cultivationRules *cultivation.Rules, state map[string]any, actorID string) []cultivation.AttributeModifier {
	raw := actorStateFieldValue(state, actorID, xiuxianFieldEffects)
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var modifiers []cultivation.AttributeModifier
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		attribute, ok := xiuxianEffectAttribute(xiuxianEntryText(entry, "属性"))
		if !ok {
			continue
		}
		percent, ok := xiuxianEffectBoostPercent(cultivationRules,
			entry, state)
		if !ok || percent == 0 {
			continue
		}
		remaining, hasDuration := actorStateNumber(entry["剩余轮次"])
		if hasDuration && remaining <= 0 {
			// An effect whose term has run out is over. The story clears it, but
			// honouring the countdown here means a stale entry cannot keep paying.
			continue
		}
		modifiers = append(modifiers, cultivation.AttributeModifier{
			ID:        "effect:" + xiuxianItemName(entry),
			Kind:      cultivation.SourceMedicine,
			Attribute: attribute,
			Value:     percent / 100,
			Group:     strings.TrimSpace(xiuxianEntryText(entry, "同类")),
			Temporary: true,
		})
	}
	return modifiers
}

// xiuxianEffectBoostPercent reads the percentage-point size of one lasting
// effect.
//
// A registered pill's size comes from the catalogue's grade table, not from the
// recorded number: §4.2 fixes 增攻药 at 5%／8%／12%／15% by grade, so the grade is
// the input and the percentage is the answer. The definition is looked up by ID
// rather than by name, because a story may call the same pill anything.
//
// An entry with no 定义 at all falls back to the recorded 数值 — that is the
// honest reading for a talisman, a blessing or another effect with no registered
// definition, and it is not a way to launder a catalogue entry, because naming a
// registered pill takes the table path. An entry that names a definition the
// catalogue does not hold, or one that is not an attack-boost pill, contributes
// nothing rather than an unverifiable amount.
func xiuxianEffectBoostPercent(cultivationRules *cultivation.Rules, entry map[string]any, states ...map[string]any) (float64, bool) {
	defID := xiuxianEntryText(entry, "定义")
	if defID == "" {
		return actorStateNumber(entry["数值"])
	}
	definition, ok := storyItemCatalog(cultivationRules,
		states...).Definition(defID)
	if !ok {
		// A definition the catalogue does not hold is not a licence to trust the
		// recorded number: the entry claims to be a registered item and is not,
		// so it contributes nothing rather than an unverifiable amount.
		return 0, false
	}
	if definition.Pill == nil {
		// A registered definition that is not a pill has no attack-boost grade
		// table to read, so it contributes nothing rather than a guessed size.
		return 0, false
	}
	bonus, ok := definition.Pill.AttackBonus()
	if !ok {
		return 0, false
	}
	return bonus * 100, true
}

// xiuxianEffectAttribute maps the recorded attribute wording onto a rules
// attribute. Only the six displayed attributes are recognised; an unreadable
// name contributes nothing rather than being attached to the wrong attribute.
func xiuxianEffectAttribute(text string) (cultivation.AttributeName, bool) {
	switch text {
	case "攻击", "attack":
		return cultivation.AttrAttack, true
	case "防御", "defense":
		return cultivation.AttrDefense, true
	case "身法", "agility":
		return cultivation.AttrAgility, true
	case "神识", "perception":
		return cultivation.AttrPerception, true
	case "生命上限", "health_max":
		return cultivation.AttrHealthMax, true
	case "灵力上限", "mana_max":
		return cultivation.AttrManaMax, true
	}
	return "", false
}

// xiuxianEntryText reads one sub-value of a structured entry as trimmed text.
func xiuxianEntryText(entry map[string]any, key string) string {
	if raw, ok := entry[key]; ok {
		return strings.TrimSpace(normalizeActorStateFieldName(stringifyEntryValue(raw)))
	}
	return ""
}

// xiuxianItemName reads an entry's display name, falling back to a stable
// placeholder so two unnamed items do not collapse into one modifier ID.
func xiuxianItemName(entry map[string]any) string {
	if name := xiuxianEntryText(entry, "名称"); name != "" {
		return name
	}
	return "未命名"
}

// stringifyEntryValue renders a sub-value for comparison.
//
// Only strings and numbers are meaningful in these entries; anything else is
// reported as empty rather than as a Go-formatted structure, because a
// comparison against a printed map would never match a table key anyway.
func stringifyEntryValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		// 'f' with precision -1 is the shortest form that round-trips, so a
		// recorded 5 renders as "5" and a recorded 5.5 as "5.5".
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	}
	return ""
}
