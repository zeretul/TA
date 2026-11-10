package interactive

import (
	"errors"
	"maps"
	"sort"
	"strconv"
	"strings"

	"ta/internal/cultivation"
)

// This file closes §4.1's equipment loop during play: an item is obtained, then
// equipped, then its effect is really paid, then it is taken off and returns to
// the pack.
//
// Before it, 主法器 and 护具 were ordinary narrative fields. A story could write
// {名称: "斩仙剑", 品级: "顶品"} into the slot and the settlement would price it,
// with nothing checking that the character had ever obtained the sword, that the
// definition existed, or that a 炼气 cultivator could operate a 顶品 treasure at
// all. §4.1 says the applicable stage, the 驾驭 requirement and the real driving
// cost must be checked at use and that 适用阶段不是点击颜色就能越过, so the slot
// cannot be a field the prose writes freely.
//
// The three facts §7 separates are kept apart here too:
//
//   - A definition exists in the catalogue. That is not stock.
//   - A copy exists under 重要物品, with its own quantity. That is stock.
//   - A copy is equipped in a slot. That is the one whose numbers are paid.
//
// Equipping moves a copy; it never mints one. A request naming a copy the
// character does not hold is refused rather than created, which is what makes
// "the story wrote that he owns a treasure" different from "he can wield it".

// StateOpSourceItemChange marks an Actor State operation the item rules produced
// while acquiring, equipping or unequipping.
//
// It is its own source kind so the reroll path drops and re-derives these writes
// without touching anything the narrative recorded, exactly as it does for skill
// and method learning.
const StateOpSourceItemChange = "item_change"

// TurnItemsInput is the model-facing item module of one story turn.
//
// It carries no grade, no rarity and no bonus. Those belong to the registered
// definition, which is what makes "AI不能因戏剧需要把普通法器写成顶品" structural:
// there is nothing here for the prose to put numbers into.
type TurnItemsInput struct {
	// ActorID is whose pack and slots change. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"The character whose items change. Omit for the protagonist."`
	// Acquire records copies the character really obtains this turn.
	Acquire []TurnItemAcquire `json:"acquire,omitempty" jsonschema_description:"Registered items the character actually obtains this turn: bought, found, awarded, refined. Omit when nothing was obtained; a thing merely seen or described does not count."`
	// Equip moves a held copy into its slot.
	Equip []TurnItemEquip `json:"equip,omitempty" jsonschema_description:"Copies the character takes into hand this turn, named by the key they are recorded under in 重要物品. The rules check that the copy is really held, that its definition is registered, and that the character has reached its applicable stage."`
	// Unequip clears a slot and returns the copy to the pack.
	Unequip []TurnItemUnequip `json:"unequip,omitempty" jsonschema_description:"Slots the character empties this turn: 主法器, 护具, 布阵, 攻击法宝, 防御法宝 or 辅助法宝. The equipped copy returns to 重要物品 and stops contributing to the attributes. Dismantling a formation ends it; it does not refund its activation."`
	// Use spends one dose of a carried pill outside a battle.
	Use []TurnItemUse `json:"use,omitempty" jsonschema_description:"Pills the character really swallows this turn, named by the key they are recorded under in 重要物品. Only a 回血 or 回灵 pill has anything to settle outside a battle; an attack pill lasts three exchanges and is taken inside the fight window, and a cultivation-support pill is spent by a training interval. A dose is spent only when the pool really rises, so taking one on a full pool is refused rather than charged."`
}

// TurnItemUse is one carried pill swallowed outside a battle.
//
// §4.2 requires the current stock, the consumption of one dose and the effect to
// be really saved. A battle settles its own dose through the fight window, but a
// character who is out of the fight and hurt — the common case after one — had no
// way to spend a pill at all: 生命 only rises through a rest interval, and the
// item module could acquire and equip but never consume. A turn whose prose said
// 吞下回春丹 therefore left the wound and the dose where they were.
type TurnItemUse struct {
	// InstanceID is the copy's key under 重要物品, which is its recorded name.
	InstanceID string `json:"instance_id" jsonschema_description:"The key the dose is recorded under in 重要物品 — the pill's own name. The copy must really be held; taking one never creates one."`
}

// TurnItemAcquire is one registered item being obtained.
type TurnItemAcquire struct {
	TemplateID string `json:"template_id,omitempty" jsonschema_description:"新创作物品可选现有物品定义作为效果模板，并填写 name、source、description；与 def_id 二选一。名称背景自由创作，品级、效果、价格和使用条件沿用模板。不能复制独特物品或材料。"`
	// DefID names a registered catalogue definition.
	DefID string `json:"def_id,omitempty" jsonschema_description:"The stable identifier of a registered item definition, such as implement.lower.common, implement.middle.rare, implement.top.unique, implement.upper.rare.effect, armor.middle.rare, treasure.attack.middle, treasure.defense.middle, treasure.support.upper.effect, pill.restore.health.lower, pill.boost.attack.middle or formation.attack.middle."`
	// Name is the item's own wording, and becomes its key in the pack. Two copies
	// of one definition under different names are two real copies, which is what
	// §7 requires.
	Name string `json:"name,omitempty" jsonschema_description:"The item's name in the story's own words. It becomes the key the copy is recorded under, so a second copy with the same name is a second dose of the same stack. Omit to use the definition identifier."`
	// Quantity is how many copies are obtained. An implement or a piece of armour
	// is one object, and the rules record one regardless of what is asked.
	Quantity int `json:"quantity,omitempty" jsonschema_description:"How many copies are obtained. Omit for one. An implement, a piece of armour, a formation focus or a treasure is always one real object."`
	// Source records where it came from, in prose. It is journal material and is
	// never priced.
	Source string `json:"source,omitempty" jsonschema_description:"Where the item came from in a few words: a merchant, a corpse, a reward, a furnace. Recorded for the journal; it does not change any number."`
	// Description is what the thing looks, feels or smells like in the story's
	// own words. It is journal material: the effect, the cost and every number
	// stay on the registered definition, so a vivid sentence cannot buy an
	// unregistered bonus. Written once, when the copy is first obtained.
	Description string `json:"description,omitempty" jsonschema_description:"One or two sentences describing what this copy looks or feels like in the story's own words, with a little wit where it fits. Do not restate its effect, cost or numbers — those come from the registered definition. Omit for an ordinary, unremarkable copy."`
	// From names the tracked character the copy is taken from, when it already
	// belongs to somebody in this story — a gift, a trade between two people, a
	// weapon taken from a defeated opponent. Omit when the copy comes from
	// outside the tracked cast: a merchant, a corpse, a furnace, a reward.
	//
	// It exists because a copy is one real object with one holder, and the
	// module had no way to say so. Taking a sword into hand removes it from
	// 重要物品, so acquiring the same name again found nothing there and wrote a
	// second one; and a sword a tracked character was holding could be acquired
	// by the player with nothing taking it off the first belt. Two live runs
	// produced exactly those: one sword in a slot and in the pack, one sword on
	// two belts.
	From string `json:"from,omitempty" jsonschema_description:"The Actor id of the character this copy is taken from, when it already belongs to someone in the story — a gift, a private sale, a weapon taken from a defeated opponent. The rules remove it from that character in the same submission. Omit when the copy comes from outside the tracked cast: a merchant, a corpse, a furnace, an award."`
}

// TurnItemEquip is one copy being taken into hand.
type TurnItemEquip struct {
	// InstanceID is the copy's key under 重要物品, which is its recorded name.
	InstanceID string `json:"instance_id" jsonschema_description:"The key the copy is recorded under in 重要物品 — the item's own name. The copy must really be held; equipping never creates one."`
}

// TurnItemUnequip is one slot being emptied.
type TurnItemUnequip struct {
	// Slot is the field being cleared.
	Slot string `json:"slot" jsonschema_description:"The slot to empty: 主法器, 护具, 布阵, 攻击法宝, 防御法宝 or 辅助法宝. The equipped copy returns to 重要物品."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnItemsInput) Empty() bool {
	return len(input.Acquire) == 0 && len(input.Equip) == 0 && len(input.Unequip) == 0 && len(input.Use) == 0
}

// withoutPaidAcquisitions drops acquisitions the same turn's trade module also
// records.
//
// A purchase is settled by the trade module: it prices the copy from the
// registered definition, deducts the whole total and writes the pack. If the
// item module also acquires the same copy in the same turn, the pack records it
// twice while the purse pays for it once — the character ends up holding goods
// nobody bought. The paid copies therefore belong to trade alone, and only the
// acquisitions that really were free (found, awarded, refined) stay here.
//
// The key is the pack key, which is the copy's name. A purchase always carries
// a name, so a nameless acquisition can only match on its definition. A free
// acquisition of the same name is reduced by the paid amount rather than
// dropped, so one found dose alongside two bought ones still records three.
func withoutPaidAcquisitions(input TurnItemsInput, trade *TurnTradeInput) TurnItemsInput {
	if trade == nil || len(trade.Buys) == 0 || len(input.Acquire) == 0 {
		return input
	}
	paid := make(map[string]int, len(trade.Buys))
	for _, buy := range trade.Buys {
		key := strings.TrimSpace(buy.Name)
		if key == "" {
			key = strings.TrimSpace(buy.DefID)
		}
		quantity := buy.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		paid[key] += quantity
	}
	kept := make([]TurnItemAcquire, 0, len(input.Acquire))
	for _, acquire := range input.Acquire {
		key := strings.TrimSpace(acquire.Name)
		if key == "" {
			key = strings.TrimSpace(acquire.DefID)
		}
		remaining := paid[key]
		if remaining <= 0 {
			kept = append(kept, acquire)
			continue
		}
		quantity := acquire.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		if quantity > remaining {
			acquire.Quantity = quantity - remaining
			kept = append(kept, acquire)
		}
		paid[key] = remaining - quantity
	}
	input.Acquire = kept
	return input
}

// cloneTurnItemsInput copies an item module so an accepted one can be retained in
// a draft without the caller's slices being aliased.
func cloneTurnItemsInput(input *TurnItemsInput) *TurnItemsInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Acquire = append([]TurnItemAcquire(nil), input.Acquire...)
	cloned.Equip = append([]TurnItemEquip(nil), input.Equip...)
	cloned.Unequip = append([]TurnItemUnequip(nil), input.Unequip...)
	cloned.Use = append([]TurnItemUse(nil), input.Use...)
	return &cloned
}

// validateTurnItemsModule rejects a module the rules could not act on.
//
// It checks shape, not outcome. Whether the definition exists, whether the copy
// is held and whether the character has reached the item's stage are the rules'
// answers and are reported in the result, so the player reads a real reason
// rather than a submission error. What must not pass is an empty definition, an
// empty instance key, an unknown slot or a negative quantity.
func validateTurnItemsModule(input *TurnItemsInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	for index, item := range input.Acquire {
		if (strings.TrimSpace(item.DefID) == "") == (strings.TrimSpace(item.TemplateID) == "") || (item.TemplateID != "" && strings.TrimSpace(item.Name) == "") {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleItems, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/items/acquire/"+strconv.Itoa(index)+"/def_id", "a registered item definition identifier",
				"", "物品应填写 def_id，或用 template_id、name 和 source 创作新定义，不能同时指定两种来源。",
			)
		}
		if item.Quantity < 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleItems, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/items/acquire/"+strconv.Itoa(index)+"/quantity", "a positive count, or omitted for one",
				strconv.Itoa(item.Quantity), "An obtained quantity must be positive, or omitted to obtain one.",
			)
		}
	}
	for index, item := range input.Equip {
		if strings.TrimSpace(item.InstanceID) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleItems, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/items/equip/"+strconv.Itoa(index)+"/instance_id", "the key the copy is recorded under",
				"", "Equipping must name the copy the character holds.",
			)
		}
	}
	for index, item := range input.Unequip {
		slot := strings.TrimSpace(item.Slot)
		if !xiuxianIsEquipmentSlot(slot) {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleItems, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/items/unequip/"+strconv.Itoa(index)+"/slot",
				"主法器, 护具, 布阵, 攻击法宝, 防御法宝 or 辅助法宝",
				slot, "Only an implement, a piece of armour, a deployed formation or one of the three equipped treasures is taken off through this module.",
			)
		}
	}
	for index, item := range input.Use {
		if strings.TrimSpace(item.InstanceID) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleItems, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/items/use/"+strconv.Itoa(index)+"/instance_id", "the key the dose is recorded under",
				"", "Taking a pill must name the copy the character holds.",
			)
		}
	}
	return nil
}

// ItemChangeResult reports what one turn's item module did, in player-readable
// terms.
type ItemChangeResult struct {
	// ActorID is the character the change applied to.
	ActorID string
	// Acquired lists the copies actually added to the pack.
	Acquired []ItemAcquisition
	// Equipped lists the copies moved into a slot.
	Equipped []ItemEquipResult
	// Unequipped lists the slots cleared, with the copy that returned to the pack.
	Unequipped []ItemUnequipResult
	// Used lists the doses really swallowed, with what the rules restored.
	Used []ItemUseResult
	// Refused lists changes that were not applied, with the reason.
	Refused []ItemRefusal
}

// ItemUseResult is one dose spent outside a battle.
type ItemUseResult struct {
	DefID string
	Name  string
	// Restored is the pool field the dose filled and the amount it really added,
	// after the maximum clamped it. It is what the prose may then say happened.
	RestoredField  string
	RestoredAmount int
}

// ItemAcquisition is one copy added to the pack.
type ItemAcquisition struct {
	DefID string
	Name  string
	Count int
}

// ItemEquipResult is one copy moved into a slot.
type ItemEquipResult struct {
	DefID string
	Name  string
	Slot  string
}

// ItemUnequipResult is one slot cleared.
type ItemUnequipResult struct {
	Name string
	Slot string
}

// ItemRefusal is one change that was not applied, with the reason the player
// needs to read.
type ItemRefusal struct {
	Action string
	Name   string
	Reason string
}

// xiuxianItemTransfer is one copy moving from one tracked character to another.
//
// It is recorded as a pair of writes, never as one: the donor's slot or pack
// loses the copy and the recipient's pack gains it in the same submission. A
// transfer that only wrote the recipient would leave the sword on both belts,
// which is the failure this exists to prevent.
type xiuxianItemTransfer struct {
	From     string
	To       string
	Name     string
	Quantity int
	// Slot is the field the donor held it in, or empty when it was in their pack.
	Slot string
}

// xiuxianItemHolder is where a copy really is: a character's slot or their pack.
type xiuxianItemHolder struct {
	ActorID string
	// Slot is the equipment field, or empty for the pack.
	Slot string
	// Entry is the recorded value, so the copy's wording travels with it.
	Entry map[string]any
}

// xiuxianItemHeldBy resolves the named donor's actual copy without changing it.
func xiuxianItemHeldBy(state map[string]any, actorID, name string) (xiuxianItemHolder, bool) {
	if slot, ok := xiuxianHeldSlot(state, actorID, name); ok {
		entry, _ := xiuxianEquipmentEntry(state, actorID, slot)
		return xiuxianItemHolder{ActorID: actorID, Slot: slot, Entry: entry}, true
	}
	pack, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	entry, ok := pack[name].(map[string]any)
	return xiuxianItemHolder{ActorID: actorID, Entry: entry}, ok
}

// xiuxianHeldSlot reports which slot a character is holding a named copy in.
//
// Equipment leaves the pack when it is taken into hand, so this is the only
// place a worn copy can be found. Without it, asking for the same sword again
// found nothing in the pack and recorded a second one.
func xiuxianHeldSlot(state map[string]any, actorID, name string) (string, bool) {
	for _, slot := range xiuxianSlotOrder {
		entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
		if !ok {
			continue
		}
		if xiuxianItemName(entry) == name {
			return slot, true
		}
	}
	return "", false
}

// xiuxianHeldDefinition reports the pack key under which a character already
// holds a copy of one registered definition, in their slots or in their pack.
//
// It exists for definitions that are one object in the world: a second copy
// under a different name would be two entries the rules price as two blades.
// The name is returned so a refusal can say which entry the character already
// has, rather than only that something is in the way.
func xiuxianHeldDefinition(state map[string]any, actorID, defID string) (string, bool) {
	if strings.TrimSpace(defID) == "" {
		return "", false
	}
	for _, slot := range xiuxianSlotOrder {
		entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
		if !ok {
			continue
		}
		if xiuxianEntryText(entry, "定义") == defID {
			if name := xiuxianItemName(entry); name != "" {
				return name, true
			}
		}
	}
	pack, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	names := make([]string, 0, len(pack))
	for name := range pack {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, ok := pack[name].(map[string]any)
		if !ok {
			continue
		}
		if xiuxianEntryText(entry, "定义") == defID {
			return name, true
		}
	}
	return "", false
}

// xiuxianPackHeldDefinition is xiuxianHeldDefinition's pack half, applied to the
// pack as this submission has written it.
func xiuxianPackHeldDefinition(pack map[string]any, defID string) (string, bool) {
	names := make([]string, 0, len(pack))
	for name := range pack {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, ok := pack[name].(map[string]any)
		if !ok {
			continue
		}
		if xiuxianEntryText(entry, "定义") == defID {
			return name, true
		}
	}
	return "", false
}

// xiuxianEntriesShareDefinition reports whether two recorded copies are the
// same kind of thing, which is the only case where their counts may be added
// together under one pack key.
//
// The pack keys a copy by its display name, so a name alone is not evidence of
// what an object is: 青锋剑 comes in a 下品 and a 中品 version, and adding their
// counts together would silently turn the better one into the worse one. An
// entry that recorded no definition is only the same thing as another entry
// that also recorded none.
func xiuxianEntriesShareDefinition(a, b map[string]any) bool {
	return xiuxianEntryText(a, "定义") == xiuxianEntryText(b, "定义")
}

// xiuxianWornDefinitionMismatch reports the slot holding a copy under name
// whose recorded definition differs from defID.
//
// It is the slot half of the purchase-time name check. Equipment leaves the
// pack when it is worn, so a pack-only scan cannot see it: buying a second
// 青锋剑 of another grade while the first one is on the character used to pass
// the check, take the money, and then lose one of the two on the way back into
// the pack.
func xiuxianWornDefinitionMismatch(state map[string]any, actorID, name, defID string) (string, bool) {
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	for _, slot := range xiuxianSlotOrder {
		entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
		if !ok {
			continue
		}
		if xiuxianItemName(entry) != name {
			continue
		}
		recorded := xiuxianEntryText(entry, "定义")
		if recorded != "" && recorded != defID {
			return slot, true
		}
	}
	return "", false
}

// xiuxianDistinctPackKey finds a key for a copy whose name is already bound to
// a different definition in the pack.
//
// Overwriting the existing entry destroys one of the two objects; refusing
// leaves the copy with nowhere to go. Qualifying the incoming one with its
// grade keeps both identities, which is what §4.1 asks of a name that is not
// unique in the world.
func xiuxianDistinctPackKey(pack map[string]any, name string, entry map[string]any) string {
	base := name
	if grade := xiuxianEntryText(entry, "品级"); grade != "" {
		base = name + "（" + grade + "）"
	}
	if _, taken := pack[base]; !taken {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "（" + strconv.Itoa(i) + "）"
		if _, taken := pack[candidate]; !taken {
			return candidate
		}
	}
}

// xiuxianFindItemHolder looks for a named copy anywhere else in the tracked
// cast: in another character's slot or pack.
//
// The scan is over Actors rather than over a separate ownership index because
// the item module has no such index and does not need one: a copy is one map
// entry, and the number of tracked characters in a story is small. It returns
// the first holder in a stable order so a replay reports the same one.
func xiuxianFindItemHolder(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	name string,
	excludeActorID string,
) (xiuxianItemHolder, bool) {
	actors, _ := state[actorStateRoot].(map[string]any)
	if len(actors) == 0 {
		return xiuxianItemHolder{}, false
	}
	ids := make([]string, 0, len(actors))
	for actorID := range actors {
		if normalizeStatePanelActorID(actorID) == excludeActorID {
			continue
		}
		ids = append(ids, actorID)
	}
	sort.Strings(ids)
	for _, rawID := range ids {
		actorID := normalizeStatePanelActorID(rawID)
		if actorID == "" {
			continue
		}
		// Only a character whose template really declares these fields can hold
		// one; a template without them has nowhere to have put the copy.
		templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
		if !found {
			continue
		}
		template := actorStateTemplateByID(system, templateID)
		if !actorStateTemplateDeclaresField(template, xiuxianFieldImportantItems) {
			continue
		}
		for _, slot := range xiuxianSlotOrder {
			entry, ok := xiuxianEquipmentEntry(state, actorID, slot)
			if ok && xiuxianItemName(entry) == name {
				return xiuxianItemHolder{ActorID: actorID, Slot: slot, Entry: entry}, true
			}
		}
		pack, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
		if entry, ok := pack[name].(map[string]any); ok {
			return xiuxianItemHolder{ActorID: actorID, Entry: entry}, true
		}
	}
	return xiuxianItemHolder{}, false
}

// xiuxianItemRefusalWords turns a rules refusal into the sentence a player is
// shown.
//
// §8 requires a refused operation to spend nothing and to name its real cause,
// so the wording distinguishes "you do not hold it" from "you cannot operate it".
func xiuxianItemRefusalWords(err error) string {
	switch {
	case errors.Is(err, cultivation.ErrItemDefinitionNotFound):
		return "没有这个已登记物品定义。"
	case errors.Is(err, cultivation.ErrItemStageTooLow):
		return "境界不足，尚不能驾驭此物；适用阶段不能靠改标签越过。"
	case errors.Is(err, cultivation.ErrItemNotWieldable):
		return "只有法器、护具与阵盘可以装备；丹药是服用之物。"
	case errors.Is(err, cultivation.ErrItemUnknownStage):
		return "该物品的适用阶段无法识别，规则不能判断能否驾驭。"
	case errors.Is(err, cultivation.ErrItemExhausted):
		return "这份物品已经用尽。"
	}
	return err.Error()
}

// xiuxianItemSlotForKind maps a definition's kind onto the slot it occupies.
//
// §4.1 gives a character exactly one main implement and one piece of armour,
// §4.3 gives a deployed formation a place of its own, and
// 《自制-装备栏与法宝御使》§1 gives each of the three 法宝 kinds one slot. A pill
// occupies no slot.
//
// The mapping is from the definition rather than from the request because §1
// says a treasure's category is a fact about the entry: an equip request that
// could name its own slot would let a 攻击法宝 be worn in the 辅助法宝 position,
// which is the same "describe it and it works" hole the slot's rules-ownership
// closes.
func xiuxianItemSlotForKind(kind cultivation.ItemKind) (string, error) {
	switch kind {
	case cultivation.KindImplement:
		return xiuxianFieldImplement, nil
	case cultivation.KindArmor:
		return xiuxianFieldArmor, nil
	case cultivation.KindFormation:
		return xiuxianFieldFormation, nil
	case cultivation.KindAttackTreasure:
		return xiuxianFieldAttackTreasure, nil
	case cultivation.KindDefenseTreasure:
		return xiuxianFieldDefenseTreasure, nil
	case cultivation.KindSupportTreasure:
		return xiuxianFieldSupportTreasure, nil
	}
	return "", cultivation.ErrItemNotWieldable
}

// xiuxianIsEquipmentSlot reports whether a name is one of the equipment fields.
//
// The check is against the same list the settlement reads, so a slot the module
// writes and a slot the settlement reads cannot drift apart.
func xiuxianIsEquipmentSlot(name string) bool {
	for _, slot := range xiuxianSlotOrder {
		if slot == name {
			return true
		}
	}
	return false
}

// xiuxianItemSlotEntry builds the recorded slot value from a registered
// definition.
//
// The numbers are written from the definition rather than from the request, so a
// copy cannot be equipped at a grade the catalogue never published. 定义 and
// 适用阶段 travel with the copy because the operating cost and the stage check
// are answered from the definition, not from the recorded wording.
//
// A formation also records its 增益, its real activation and upkeep costs and
// whether it is still running, because §4.3 makes all four facts part of the
// deployed state: the panel has to say what the formation does and what it costs,
// and the reader has to be able to tell a running formation from an abandoned
// one without re-deriving the answer from the catalogue.
//
// The slot is a move, not a new acquisition, so the copy's description and source
// come with it: a sword keeps the wording it was obtained with when it is taken
// into hand, and gets it back unchanged when it is put away. Dropping them here
// would make equipping erase the journal entry the player already read.
func xiuxianItemSlotEntry(
	definition cultivation.ItemDef,
	name string,
	stage cultivation.Cultivation,
	description string,
	source string,
) map[string]any {
	entry := map[string]any{
		"名称":     name,
		"定义":     definition.ID,
		"品级":     xiuxianItemGradeWording[definition.Grade],
		"稀有度":    xiuxianItemRarityWording[definition.Rarity],
		"主要属性预算": definition.RaritySpentOnMainStat * 100,
	}
	if strings.TrimSpace(definition.ApplicableStage) != "" {
		entry["适用阶段"] = definition.ApplicableStage
	}
	if text := strings.TrimSpace(description); text != "" {
		entry["描述"] = text
	}
	if text := strings.TrimSpace(source); text != "" {
		entry["来历"] = text
	}
	if definition.Kind == cultivation.KindFormation && definition.Formation != nil {
		switch {
		case definition.Formation.AttackBonus > 0:
			entry["增益"] = "攻击"
		case definition.Formation.DefenseBonus > 0:
			entry["增益"] = "防御"
		}
		entry["启动耗灵"] = float64(definition.Formation.StartManaCost(stage))
		entry["每轮维持"] = float64(definition.Formation.UpkeepManaCost(stage))
		entry["维持中"] = "是"
	}
	return entry
}

// xiuxianItemPackEntry builds the recorded pack value for one acquired copy.
func xiuxianItemPackEntry(definition cultivation.ItemDef, name string, quantity int) map[string]any {
	return xiuxianItemPackEntryWithFlavor(definition, name, quantity, "", "")
}

// xiuxianItemPackEntryWithFlavor records what the copy looks like and where it
// came from alongside the facts the rules price.
//
// The two prose fields are journal material. They are written once, when the
// copy is first obtained, and never re-derived: a later turn that mentions the
// same sword must not be able to restyle it, and a replayed commit must produce
// the same sentence. The numbers stay on the registered definition, so nothing
// written here can buy an unregistered bonus.
func xiuxianItemPackEntryWithFlavor(
	definition cultivation.ItemDef,
	name string,
	quantity int,
	description string,
	source string,
) map[string]any {
	entry := map[string]any{
		"定义": definition.ID,
		"数量": float64(quantity),
	}
	if strings.TrimSpace(definition.ApplicableStage) != "" {
		entry["适用阶段"] = definition.ApplicableStage
	}
	if text := strings.TrimSpace(description); text != "" {
		entry["描述"] = text
	}
	if text := strings.TrimSpace(source); text != "" {
		entry["来历"] = text
	}
	return entry
}

// xiuxianItemOps applies one turn's item module and returns the writes that
// record it.
//
// Ordering inside the module is acquire, then equip, then unequip, so a copy
// obtained and taken into hand in the same turn is equipable and a slot cleared
// in the same turn does not fight the copy replacing it.
func xiuxianItemOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	input TurnItemsInput,
	turnID string,
) ([]ActorStateOp, ItemChangeResult, bool) {
	result := ItemChangeResult{ActorID: normalizeStatePanelActorID(actorID)}
	if result.ActorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, result, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, result.ActorID)
	if !found {
		return nil, result, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, result, false
	}
	// The template must declare 重要物品: that is where a copy is held, and a
	// module that could not write it would have nowhere to put anything.
	//
	// The equipment slots are different. Each one is written only if the template
	// declares it, and a slot the template never declared is simply not part of
	// this actor's state — writing one would put a value where the interface
	// cannot read it. What must not happen is the old rule, which required every
	// slot to be declared or disabled the whole module: a world pack written
	// before the three 法宝 slots existed declares only the older three, and
	// demanding all six would take away its acquire, equip, unequip and use
	// entirely. So the module reports which slots this template really has, and
	// the loops below skip the rest.
	if !actorStateTemplateDeclaresField(template, xiuxianFieldImportantItems) {
		return nil, result, false
	}
	declared := make(map[string]bool, len(xiuxianSlotOrder))
	for _, slot := range xiuxianSlotOrder {
		declared[slot] = actorStateTemplateDeclaresField(template, slot)
	}
	if !declared[xiuxianFieldImplement] && !declared[xiuxianFieldArmor] && !declared[xiuxianFieldFormation] &&
		!declared[xiuxianFieldAttackTreasure] && !declared[xiuxianFieldDefenseTreasure] && !declared[xiuxianFieldSupportTreasure] {
		// No equipment slot at all: the actor template models a pack but not
		// equipment, and this module is not the one that should be writing it.
		return nil, result, false
	}
	if input.Empty() {
		return nil, result, true
	}

	catalog := storyItemCatalog(system.CultivationRules,
		state)
	registryRecord, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	registryRecord = maps.Clone(registryRecord)
	if registryRecord == nil {
		registryRecord = map[string]any{}
	}
	definitionsChanged := false
	pack, _ := xiuxianEquipmentEntry(state, result.ActorID, xiuxianFieldImportantItems)
	nextPack := make(map[string]any, len(pack)+len(input.Acquire))
	for key, value := range pack {
		nextPack[key] = value
	}
	slots := make(map[string]map[string]any, len(xiuxianSlotOrder))
	slotChanged := map[string]bool{}
	for _, slot := range xiuxianSlotOrder {
		slots[slot] = map[string]any{}
		if !declared[slot] {
			continue
		}
		if entry, ok := xiuxianEquipmentEntry(state, result.ActorID, slot); ok {
			slots[slot] = entry
		}
	}

	stageKey := xiuxianStoredStageKey(state, result.ActorID)
	stage, stageOK := cultivation.ParseStageKey(stageKey, system.CultivationRules)
	// The pool a formation's activation is paid from. It is read once and
	// written back only if a formation was really raised, so an ordinary
	// equip does not restate a number it never touched.
	mana := xiuxianStoredInt(state, result.ActorID, xiuxianFieldMana)
	manaFromFormation := false
	manaFromPill := false
	// The health pool a swallowed pill fills. It is a separate local from the
	// mana above because the two are written by different rules, and a turn that
	// both raised a formation and took a pill must publish both numbers.
	health := xiuxianStoredInt(state, result.ActorID, xiuxianFieldHealth)
	healthChanged := false
	// Copies this turn takes off another tracked character. They are collected
	// while the acquisitions are checked and written at the end, because the
	// holder's own state is a different Actor's fields and needs its own writes.
	transfers := make([]xiuxianItemTransfer, 0, len(input.Acquire))
	// Reservations are local to this submission. A later acquisition must not
	// spend the same donor stock again before the final actor writes are applied.
	reserved := map[string]int{}

	// Acquire. A definition the catalogue does not hold is refused rather than
	// recorded, because an entry claiming to be a registered item and not being
	// one has no published numbers to be priced from.
	for _, request := range input.Acquire {
		defID := strings.TrimSpace(request.DefID)
		var generatedRecord map[string]any
		if request.TemplateID != "" {
			id, err := generatedContentID("item", strings.TrimSpace(request.TemplateID), request.Name)
			generatedRecord = generatedContentRecord("equipment", id, strings.TrimSpace(request.TemplateID), request.Name, request.Source, request.Description)
			_, valid := generatedItemFromRecord(system.CultivationRules,
				generatedRecord)
			if err != nil || !valid || defID != "" || !generatedRegistryCanAdd(registryRecord, generatedRecord) {
				result.Refused = append(result.Refused, ItemRefusal{Action: "acquire", Name: request.Name, Reason: "新物品需要可用的效果模板、名称与实际来历；独特物品、材料或已满的本局目录不能生成。"})
				continue
			}
			if existing, ok := registryRecord[id].(map[string]any); ok {
				generatedRecord = existing
			}
			defID = id
		}
		definition, ok := catalog.Definition(defID)
		if !ok && generatedRecord != nil {
			definition, ok = generatedItemFromRecord(system.CultivationRules,
				generatedRecord)
		}
		if !ok {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "acquire", Name: defID,
				Reason: xiuxianItemRefusalWords(cultivation.ErrItemDefinitionNotFound),
			})
			continue
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = definition.ID
		}
		quantity := request.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		if definition.Equippable() {
			// Equipment is one real object, not a stack. A formation focus and a
			// treasure are single objects in the same sense the implement and the
			// armour are: each occupies one slot, so a count of two would be a
			// stack the slots cannot express.
			quantity = 1
		}
		obtained := quantity
		// A unique definition is one object in the world, not a tier of quality.
		// §4.1 of 自制-装备与物品品级 makes 稀有度 a statement about where a thing
		// can be got, and 独特 is the strongest of those statements: the only
		// honest ways to hold one are to be handed it by whoever really has it or
		// to be awarded it by authored content. A free acquisition would be the
		// model minting a second 顶品独特 blade out of a sentence, which is what
		// the transfer rules exist to prevent.
		if definition.Rarity == cultivation.RarityUnique && normalizeStatePanelActorID(request.From) == "" {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "acquire", Name: name,
				Reason: "这件是世上仅此一件的独特之物，不能凭空取得；它要么本来就在某个已登记角色手里（取得时把 from 写成他的 Actor id），要么由已登记的奖励正式发放。",
			})
			continue
		}
		// The same reasoning from the other side: a character cannot hold two
		// copies of one unique definition under two different names. The pack as
		// this turn has written it is checked first, so two acquisitions in one
		// submission cannot slip past a lookup that only saw the committed state.
		if definition.Rarity == cultivation.RarityUnique {
			if heldName, held := xiuxianPackHeldDefinition(nextPack, definition.ID); held {
				result.Refused = append(result.Refused, ItemRefusal{
					Action: "acquire", Name: name,
					Reason: "你名下已经有这件独特之物（记为「" + heldName + "」），同一件实物不能记成两份。",
				})
				continue
			}
			if heldName, held := xiuxianHeldDefinition(state, result.ActorID, definition.ID); held {
				result.Refused = append(result.Refused, ItemRefusal{
					Action: "acquire", Name: name,
					Reason: "你名下已经有这件独特之物（记为「" + heldName + "」），同一件实物不能记成两份。",
				})
				continue
			}
		}
		// One real object, one holder. These checks are what keep a copy from
		// existing twice, and neither of them is a reminder to the model:
		//
		//   - Asking for a copy already in the character's own hand is not a
		//     second acquisition. Equipping removes the copy from 重要物品, so the
		//     pack lookup below found nothing there and wrote a fresh one — a live
		//     run ended with the same sword in the slot and in the pack, both
		//     priced.
		//   - Asking for a copy another tracked character holds is a transfer, and
		//     a transfer takes it off them in the same submission. Without this the
		//     same blade hung on two belts, which another live run reproduced.
		//
		// Renaming is not a separate hole: a transfer moves the object whatever it
		// is called, so a renamed copy still leaves its holder. What stays allowed
		// is a genuinely different sword of the same kind, which the rules cannot
		// tell from a duplicate and have no business refusing.
		if slot, inHand := xiuxianHeldSlot(state, result.ActorID, name); inHand {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "acquire", Name: name,
				Reason: "这件物品正装备在" + slot + "上，是同一件实物，不能再取得一次；要放回行囊请先用 unequip 卸下。",
			})
			continue
		}
		donor := normalizeStatePanelActorID(request.From)
		if donor == result.ActorID {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "acquire", Name: name,
				Reason: "不能从自己手里取得；这件物品本来就在你名下。",
			})
			continue
		}
		holder, heldElsewhere := xiuxianFindItemHolder(state, system, name, result.ActorID)
		if donor != "" {
			// An explicit donor disambiguates identical names on different actors.
			holder, heldElsewhere = xiuxianItemHeldBy(state, donor, name)
		}
		if donor == "" {
			if heldElsewhere {
				result.Refused = append(result.Refused, ItemRefusal{
					Action: "acquire", Name: name,
					Reason: "这件物品现在由 " + holder.ActorID + " 持有，同一件实物不能同时挂在两个人身上；请把 from 写成本次转移的让与人，规则会在同一次提交里从他名下取走。",
				})
				continue
			}
		} else if !heldElsewhere || holder.ActorID != donor {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "acquire", Name: name,
				Reason: "from 写的 " + donor + " 名下没有这件物品；转移只能搬动已经存在的实物，不会凭空生成。",
			})
			continue
		}
		if donor != "" {
			if xiuxianEntryText(holder.Entry, "定义") != definition.ID {
				result.Refused = append(result.Refused, ItemRefusal{Action: "acquire", Name: name,
					Reason: "The requested definition does not match the donor's recorded item. A transfer cannot change its grade or effects."})
				continue
			}
			available := 1
			if holder.Slot == "" {
				available = xiuxianCarriedQuantity(holder.Entry)
			}
			if quantity > available-reserved[donor+"\x00"+name] {
				result.Refused = append(result.Refused, ItemRefusal{Action: "acquire", Name: name,
					Reason: "The donor does not hold enough unreserved stock. No items were transferred."})
				continue
			}
		}
		existing, held := nextPack[name].(map[string]any)
		if held {
			// A second acquisition of the same name is more of the same stack, but
			// only when it really is the same definition: two different definitions
			// under one name would make the pack key ambiguous.
			if recorded := xiuxianEntryText(existing, "定义"); recorded != "" && recorded != definition.ID {
				result.Refused = append(result.Refused, ItemRefusal{
					Action: "acquire", Name: name,
					Reason: "同名物品已按另一登记定义记入行囊；请换一个名称，以免两份不同物品共用一个键。",
				})
				continue
			}
			// The slot count caps how many may be worn at once, not how many may
			// be kept (《自制-装备栏与法宝御使》§2), so the pack key is a count for
			// equipment too — the trade path already records a second paid sword
			// as a second copy under the same name. Writing this entry with the
			// request's own count instead of the pack's dropped everything already
			// there: a pack holding three 阵盘 came out of a free acquisition
			// holding one.
			if !definition.Equippable() || donor != "" {
				// A transfer really moves the object, so the recipient's existing
				// copies stay and the moved one joins them. Refusing here used to
				// force the player to invent a second name for an identical sword
				// just to receive it.
				quantity += xiuxianCarriedQuantity(existing)
			} else if carried := xiuxianCarriedQuantity(existing); carried > quantity {
				// A free acquisition is either the same copy narrated again or one
				// the story did not pay for; neither may reduce what is held, and
				// neither may mint more. Keep the count the pack already has.
				quantity = carried
			}
		}
		// The description and the source are the copy's own first impression, so
		// an existing copy keeps the wording it was recorded with and only a new
		// acquisition supplies it. Restyling the same sword on a later turn would
		// make the journal change under the player.
		description := request.Description
		source := request.Source
		if held {
			if recorded := xiuxianEntryText(existing, "描述"); recorded != "" {
				description = recorded
			}
			if recorded := xiuxianEntryText(existing, "来历"); recorded != "" {
				source = recorded
			}
		}
		nextPack[name] = xiuxianItemPackEntryWithFlavor(definition, name, quantity, description, source)
		if generatedRecord != nil {
			_ = catalog.Register(definition)
		}
		if generatedRecord != nil && registryRecord[definition.ID] == nil {
			registryRecord[definition.ID] = generatedRecord
			definitionsChanged = true
		}
		if donor != "" {
			// Move the recorded object, including its provenance and wording. The
			// request may name the transfer but cannot rewrite the donor's copy.
			entry := maps.Clone(holder.Entry)
			if holder.Slot != "" {
				entry = xiuxianItemPackEntryFromSlot(holder.Entry)
			}
			entry["数量"] = float64(quantity)
			nextPack[name] = entry
			reserved[donor+"\x00"+name] += obtained
			transfers = append(transfers, xiuxianItemTransfer{
				From: donor, To: result.ActorID, Name: name, Slot: holder.Slot, Quantity: obtained,
			})
		}
		result.Acquired = append(result.Acquired, ItemAcquisition{DefID: definition.ID, Name: name, Count: obtained})
	}

	// Equip. The copy must really be held, and the character must really be able
	// to operate it. Both are checked before anything moves.
	equippedThisTurn := map[string]bool{}
	for _, request := range input.Equip {
		instanceID := strings.TrimSpace(request.InstanceID)
		if equippedThisTurn[instanceID] {
			// A position holds one object, so naming the same copy twice in one
			// submission describes one placement. Letting the second pass through
			// the loop again did not move a second object — it re-wrote the same
			// slot — but it did charge the deployment again, so raising one 阵盘
			// twice in a turn cost two activation payments for one standing
			// formation. This is the same rule the use loop below states for a
			// dose named twice: one copy, one effect.
			continue
		}
		equippedThisTurn[instanceID] = true
		entry, held := nextPack[instanceID].(map[string]any)
		if !held {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: "行囊中没有这件物品；装备只能移动已持有的副本，不会凭空生成。",
			})
			continue
		}
		if xiuxianCarriedQuantity(entry) <= 0 {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: xiuxianItemRefusalWords(cultivation.ErrItemExhausted),
			})
			continue
		}
		defID := xiuxianEntryText(entry, "定义")
		definition, ok := catalog.Definition(defID)
		if !ok {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: xiuxianItemRefusalWords(cultivation.ErrItemDefinitionNotFound),
			})
			continue
		}
		slot, err := xiuxianItemSlotForKind(definition.Kind)
		if err != nil {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID, Reason: xiuxianItemRefusalWords(err),
			})
			continue
		}
		if !declared[slot] {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: "这个角色的模板没有“" + slot + "”这一栏，规则无处安放这件物品；物品仍留在行囊里。",
			})
			continue
		}
		if !stageOK {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: "角色的当前境界无法读取，规则不能判断能否驾驭此物。",
			})
			continue
		}
		if err := definition.UsableBy(stage); err != nil {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID, Reason: xiuxianItemRefusalWords(err),
			})
			continue
		}

		// One real object occupies one position. 《自制-装备栏与法宝御使》§1 says
		// 同一实物只能占一个位置、执行一次动作, so a copy already worn somewhere
		// else is refused here rather than silently hung in two places. The name
		// is the identity the records carry, which is the same key the acquisition
		// path uses to refuse a second copy of what is already in hand; a
		// genuinely different item that happens to share a name cannot be told
		// apart by these records, and the design gives the rules no business
		// guessing.
		if heldSlot, inHand := xiuxianHeldSlot(state, result.ActorID, instanceID); inHand && heldSlot != slot {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "equip", Name: instanceID,
				Reason: "这件实物已经装备在“" + heldSlot + "”上，同一件物品只能占一个位置；要换到“" + slot + "”请先 unequip 再装备。",
			})
			continue
		}

		// Deploying a formation is a real spend before it is a bonus. §4.3 fixes
		// 启动耗操作者阶段基准灵力10%, and a deployment the operator cannot pay for
		// is refused rather than recorded and then quietly paid for by nobody.
		if definition.Kind == cultivation.KindFormation && definition.Formation != nil {
			startCost := definition.Formation.StartManaCost(stage)
			if mana < startCost {
				result.Refused = append(result.Refused, ItemRefusal{
					Action: "equip", Name: instanceID,
					Reason: "灵力不足以启动此阵；阵法启动要支付真实灵力，不是写上就生效。",
				})
				continue
			}
			mana -= startCost
			manaFromFormation = true
		}

		// The slot holds one item. Whatever was there returns to the pack rather
		// than being destroyed: §4.1's one-at-a-time rule is about what contributes
		// to the numbers, not about what the character owns. A copy already in the
		// pack joins that stack instead of replacing it.
		remaining := xiuxianCarriedQuantity(entry) - 1
		if previous, occupied := slots[slot]; occupied && len(previous) > 0 {
			previousName := xiuxianItemName(previous)
			returned := xiuxianItemPackEntryFromSlot(previous)
			if previousName == instanceID && xiuxianEntriesShareDefinition(entry, returned) {
				// The same copy comes out of the slot and one goes back in, so the
				// pack keeps what it held. A shared name is not enough to make that
				// true: a 中品 and a 下品 青锋剑 are keyed alike, and cancelling the
				// two here would delete the copy leaving the slot together with the
				// one arriving, leaving both records reading whichever grade the
				// surviving entry carried. Only a copy of the same definition is the
				// same object; the rest fall through to the distinct-key path below.
				remaining++
			} else if existing, held := nextPack[previousName].(map[string]any); held {
				// Only a copy of the same definition joins the stack; a different
				// one keeps its own key so the grade it was recorded with is not
				// overwritten by whichever entry the name happened to hit.
				if xiuxianEntriesShareDefinition(existing, returned) {
					returned["数量"] = float64(xiuxianCarriedQuantity(existing) + 1)
				} else {
					previousName = xiuxianDistinctPackKey(nextPack, previousName, returned)
				}
			}
			nextPack[previousName] = returned
		}
		// Equipment is one object, so exactly one copy leaves the pack. Deleting
		// the whole entry used to destroy the rest of a stack: buying three 阵盘
		// and equipping one silently consumed the other two the player paid for.
		// The wording the pack entry was recorded with travels with it, so the
		// state panel reads the same whether the sword is in the pack or in hand.
		if remaining > 0 {
			nextPack[instanceID] = xiuxianItemPackEntryWithFlavor(
				definition,
				instanceID,
				remaining,
				xiuxianEntryText(entry, "描述"),
				xiuxianEntryText(entry, "来历"),
			)
		} else {
			delete(nextPack, instanceID)
		}
		slots[slot] = xiuxianItemSlotEntry(
			definition,
			instanceID,
			stage,
			xiuxianEntryText(entry, "描述"),
			xiuxianEntryText(entry, "来历"),
		)
		slotChanged[slot] = true
		result.Equipped = append(result.Equipped, ItemEquipResult{DefID: definition.ID, Name: instanceID, Slot: slot})
	}

	// Unequip. A cleared slot returns its copy to the pack, which is the other
	// half of §4.1's loop: the item is owned either way, only its contribution
	// changes.
	for _, request := range input.Unequip {
		slot := strings.TrimSpace(request.Slot)
		entry, occupied := slots[slot]
		if !occupied || len(entry) == 0 {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "unequip", Name: slot, Reason: "该槽位当前没有装备。",
			})
			continue
		}
		name := xiuxianItemName(entry)
		// The copy coming out of the slot joins whatever the pack already holds
		// under that name. Writing it straight over the pack entry is the other
		// half of the bug the equip loop above was fixed for: buying three 阵盘
		// and equipping one left two in the pack, and putting that one back
		// replaced the two with a single copy. The player paid for three.
		//
		// It only joins when it really is the same thing. A 中品 sword shares its
		// name with a 下品 one but not its grade, and adding the counts together
		// used to leave two entries both reading 下品: the better blade the player
		// paid for became the worse one and could not be recovered. A copy of
		// another definition gets a key of its own instead, so both are kept with
		// the grade each was recorded with.
		returned := xiuxianItemPackEntryFromSlot(entry)
		key := name
		if existing, held := nextPack[name].(map[string]any); held {
			if !xiuxianEntriesShareDefinition(existing, returned) {
				key = xiuxianDistinctPackKey(nextPack, name, returned)
			} else {
				for _, field := range []string{"描述", "来历"} {
					if text := xiuxianEntryText(existing, field); text != "" {
						returned[field] = text
					}
				}
				returned["数量"] = float64(xiuxianCarriedQuantity(existing) + xiuxianCarriedQuantity(returned))
			}
		}
		nextPack[key] = returned
		slots[slot] = map[string]any{}
		slotChanged[slot] = true
		result.Unequipped = append(result.Unequipped, ItemUnequipResult{Name: name, Slot: slot})
	}

	// Use. A dose is swallowed outside the fight window, so it is settled here
	// rather than by a battle round.
	//
	// §4.2 fixes the restored amount by the definition's grade, not by anything
	// the request says, and a battle already prices its own dose the same way. A
	// pill that has nothing to do is refused rather than charged: taking it on a
	// full pool spends a dose and restores nothing, which is the dishonest half of
	// the same rule the battle side states as reason.pool_already_full.
	//
	// The attack pill and the cultivation-support pill are deliberately not
	// settled here. An attack pill lasts three exchanges, which only a fight
	// window can count; a support pill is spent by a training interval, which has
	// its own module. Recording either as a swallowed dose would consume stock the
	// turn never paid for.
	usedDoses := map[string]int{}
	useOrder := make([]string, 0, len(input.Use))
	for _, request := range input.Use {
		instanceID := strings.TrimSpace(request.InstanceID)
		if _, seen := usedDoses[instanceID]; seen {
			// Two requests for one copy are one swallow: a turn that names the same
			// dose twice must not spend two of them.
			continue
		}
		entry, held := nextPack[instanceID].(map[string]any)
		if !held {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "行囊中没有这份丹药；服用只能消耗已持有的副本，不会凭空生成。",
			})
			continue
		}
		if xiuxianCarriedQuantity(entry) <= 0 {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: xiuxianItemRefusalWords(cultivation.ErrItemExhausted),
			})
			continue
		}
		defID := xiuxianEntryText(entry, "定义")
		definition, ok := catalog.Definition(defID)
		if !ok || definition.Kind != cultivation.KindPill || definition.Pill == nil {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "行囊中的这份东西不是已登记的丹药，规则没有可结算的药效。",
			})
			continue
		}
		pill := *definition.Pill
		fieldID := ""
		switch pill.Purpose {
		case cultivation.PillRestoreHealth:
			fieldID = xiuxianFieldHealth
		case cultivation.PillRestoreMana:
			fieldID = xiuxianFieldMana
		default:
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "这份丹药的药效不在战斗外结算：增攻药只在战斗窗口里服用，修炼辅助药由修炼时段消耗。",
			})
			continue
		}
		// The amount is priced from the character's settled attributes, so the
		// grade's fraction is applied to the same maximum the panel shows.
		character, err := xiuxianCharacterFromState(system.CultivationRules,
			state, result.ActorID)
		if err != nil {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "角色的当前境界或属性无法读取，规则不能结算这份药效。",
			})
			continue
		}
		attributes := character.Attributes()
		current, maximum := health, attributes.HealthMax
		if fieldID == xiuxianFieldMana {
			current, maximum = mana, attributes.ManaMax
		}
		if current >= maximum {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "这一项已经满了，吞下也只是白费一份丹药；药效没有可以回升的余地。",
			})
			continue
		}
		amount := pill.RestoreAmount(attributes)
		if amount <= 0 {
			result.Refused = append(result.Refused, ItemRefusal{
				Action: "use", Name: instanceID,
				Reason: "这份丹药没有可结算的回复量。",
			})
			continue
		}
		restored := min(maximum, current+amount) - current
		if fieldID == xiuxianFieldHealth {
			health = current + restored
			healthChanged = true
		} else {
			mana = current + restored
			manaFromPill = true
		}
		usedDoses[instanceID] = 1
		useOrder = append(useOrder, instanceID)
		result.Used = append(result.Used, ItemUseResult{
			DefID: definition.ID, Name: instanceID,
			RestoredField: fieldID, RestoredAmount: restored,
		})
	}
	// The stock is written from the doses really swallowed, and only those: a
	// refused dose leaves its count where it was, which is what §8's "a refused
	// operation spends nothing" means for a pill.
	if len(useOrder) > 0 {
		spent := make([]cultivation.CarriedDose, 0, len(useOrder))
		for _, instanceID := range useOrder {
			entry, _ := nextPack[instanceID].(map[string]any)
			spent = append(spent, cultivation.CarriedDose{
				InstanceID: instanceID,
				Quantity:   xiuxianCarriedQuantity(entry) - usedDoses[instanceID],
			})
		}
		if updated, changed := xiuxianSpentCarriedRecord(nextPack, spent); changed {
			nextPack = updated
		}
	}

	if len(result.Acquired) == 0 && len(result.Equipped) == 0 && len(result.Unequipped) == 0 && len(result.Used) == 0 {
		return nil, result, true
	}

	ops := make([]ActorStateOp, 0, 6)
	if definitionsChanged {
		ops = append(ops, generatedDefinitionOp(registryRecord, turnID, StateOpSourceItemChange))
	}
	ops = append(ops, ActorStateOp{
		Op:           "set",
		ActorID:      result.ActorID,
		FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
		Value:        nextPack,
		Reason:       "由物品取得、装备与卸下结算写入，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceItemChange,
		SourceID:     "items",
	})
	// The slots are written in a fixed order so a replay produces the same
	// operation list, and therefore the same state, every time.
	for _, slot := range xiuxianSlotOrder {
		if !slotChanged[slot] {
			continue
		}
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      result.ActorID,
			FieldID:      normalizeActorStateFieldName(slot),
			Value:        slots[slot],
			Reason:       "由物品装备结算写入，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceItemChange,
			SourceID:     "items",
		})
	}
	ops = append(ops, xiuxianItemTransferOps(state, transfers, turnID)...)
	// The activation is a real spend, so it is written as one. §4.3 fixes the
	// cost at 10% of the operator's stage base mana; recording the deduction here
	// is what makes the deployment cost the player something rather than being a
	// sentence the narrative asserts.
	//
	// The same field is also where a swallowed 回灵 pill writes, so one write
	// covers both and the reason names whichever really happened.
	if manaFromFormation || manaFromPill {
		reason := "由阵法启动耗灵结算写入，不由叙述改写。"
		switch {
		case manaFromFormation && manaFromPill:
			reason = "由阵法启动耗灵与服丹回灵结算写入，不由叙述改写。"
		case manaFromPill:
			reason = "由服丹回灵结算写入，不由叙述改写。"
		}
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      result.ActorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldMana),
			Value:        mana,
			Reason:       reason,
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceItemChange,
			SourceID:     "items",
		})
	}
	// A swallowed pill's restore is the other half of §4.2: the stock went down
	// and the pool has to go up by the same settled amount, or the prose says the
	// wound closed while the panel still shows it open. The settlement that follows
	// re-derives the maximum and clamps the pool to it, so the absolute value
	// written here is the pool as this module leaves it, not a claim about the
	// character's ceiling.
	if healthChanged {
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      result.ActorID,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldHealth),
			Value:        health,
			Reason:       "由服丹回血结算写入，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceItemChange,
			SourceID:     "items",
		})
	}
	return ops, result, true
}

// xiuxianItemTransferOps removes a transferred copy from each donor.
//
// The recipient's side is already written: the acquisition puts the copy in the
// recipient's pack, so only the donor's loss is left. It is written as the
// donor's own fields — their slot cleared, or their pack with the key removed —
// so the same submission takes the object off one belt and puts it on the other.
//
// Donors are written in acceptance order. Reservations made during validation
// bound repeated requests to the actual stock; these writes debit only the
// accepted quantities and preserve the donor's remaining stack.
func xiuxianItemTransferOps(
	state map[string]any,
	transfers []xiuxianItemTransfer,
	turnID string,
) []ActorStateOp {
	if len(transfers) == 0 {
		return nil
	}
	// One pack write per donor, carrying every copy taken from them, so a turn
	// that takes two things from the same character does not write their pack
	// twice from two separate reads.
	donorPacks := map[string]map[string]any{}
	donorSlots := map[string]map[string]map[string]any{}
	donorOrder := make([]string, 0, len(transfers))
	slotOrder := make([]string, 0, len(transfers))
	for _, transfer := range transfers {
		donor := normalizeStatePanelActorID(transfer.From)
		if donor == "" {
			continue
		}
		if _, seen := donorPacks[donor]; !seen {
			pack, _ := xiuxianEquipmentEntry(state, donor, xiuxianFieldImportantItems)
			clone := make(map[string]any, len(pack))
			for key, value := range pack {
				clone[key] = value
			}
			donorPacks[donor] = clone
			donorSlots[donor] = map[string]map[string]any{}
			for _, slot := range xiuxianSlotOrder {
				entry, ok := xiuxianEquipmentEntry(state, donor, slot)
				if !ok {
					entry = map[string]any{}
				}
				donorSlots[donor][slot] = entry
			}
			donorOrder = append(donorOrder, donor)
		}
		if transfer.Slot != "" {
			donorSlots[donor][transfer.Slot] = map[string]any{}
			slotOrder = append(slotOrder, donor+"\x00"+transfer.Slot)
			continue
		}
		entry, _ := donorPacks[donor][transfer.Name].(map[string]any)
		remaining := xiuxianCarriedQuantity(entry) - transfer.Quantity
		if remaining <= 0 {
			delete(donorPacks[donor], transfer.Name)
		} else {
			copy := maps.Clone(entry)
			copy["数量"] = float64(remaining)
			donorPacks[donor][transfer.Name] = copy
		}
	}

	ops := make([]ActorStateOp, 0, len(donorOrder)+len(slotOrder))
	for _, donor := range donorOrder {
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      donor,
			FieldID:      normalizeActorStateFieldName(xiuxianFieldImportantItems),
			Value:        donorPacks[donor],
			Reason:       "物品已转交他人，由物品转移结算从行囊移出，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceItemChange,
			SourceID:     "items",
		})
	}
	for _, key := range slotOrder {
		parts := strings.SplitN(key, "\x00", 2)
		ops = append(ops, ActorStateOp{
			Op:           "set",
			ActorID:      parts[0],
			FieldID:      normalizeActorStateFieldName(parts[1]),
			Value:        donorSlots[parts[0]][parts[1]],
			Reason:       "物品已转交他人，由物品转移结算从槽位移出，不由叙述改写。",
			SourceTurnID: turnID,
			SourceKind:   StateOpSourceItemChange,
			SourceID:     "items",
		})
	}
	return ops
}

// xiuxianItemPackEntryFromSlot turns a recorded slot value back into a pack
// entry.
//
// The definition and the applicable stage travel with it, so re-equipping the
// copy later is checked against the same definition it came from. The slot's
// display fields are not copied: a pack entry is priced from its definition, and
// carrying a second copy of the numbers would be a second, unchecked source of
// truth.
//
// The description and the source do come back. They are not numbers and nothing
// prices them; dropping them would mean taking a sword in hand and putting it
// down erased the sentence the player had already read about it.
func xiuxianItemPackEntryFromSlot(entry map[string]any) map[string]any {
	pack := map[string]any{"数量": float64(1)}
	if defID := xiuxianEntryText(entry, "定义"); defID != "" {
		pack["定义"] = defID
	}
	if stage := xiuxianEntryText(entry, "适用阶段"); stage != "" {
		pack["适用阶段"] = stage
	}
	for _, key := range []string{"描述", "来历"} {
		if text := xiuxianEntryText(entry, key); text != "" {
			pack[key] = text
		}
	}
	return pack
}

// removeItemChangeActorOps drops a previous item change's writes.
//
// It mirrors the settlement's, the advance's and the learning modules' removers
// so the reroll path can discard an equip or an acquisition along with the rule
// outcome it was derived from.
func removeItemChangeActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceItemChange {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}
