package cultivation

import (
	"strings"
)

// This file implements the first-version catalogue of
// 《自制-装备与物品品级》V0.1 §9: a small representative set of a main
// implement, protective gear, a restoring pill, an attack-boosting pill and a
// single-type formation, enough to exercise the whole loop the design asks for —
// obtain, equip or use, take effect, be consumed or expire, recover after a
// restart, and stay isolated per branch.
//
// The design is explicit that this is not a full economy: §9 says to validate
// with 低品常见、中品稀有、顶品独特 representatives first and add entries
// afterwards, and §11 says the full drop, refining and auction systems are out
// of scope. The catalogue is therefore deliberately short and the entries are
// chosen to make each published budget boundary reachable.
//
// The one structural rule this file exists to keep is §7's:
//
//	物品定义与当前物品实例、掌握能力区分；同一定义可有多份真实实例，
//	不能把定义存在当作库存存在。
//
// A definition is a registered entry with fixed numbers. An instance is one
// real copy a character actually holds, with its own remaining duration and
// its own consumption history. Holding a definition means nothing; the
// catalogue is not an inventory.

// ItemKind is what a catalogue entry is.
type ItemKind string

const (
	// KindImplement is the one active main implement of §4.1.
	KindImplement ItemKind = "implement"
	// KindArmor is the one active piece of protective gear of §4.1.
	KindArmor ItemKind = "armor"
	// KindPill is a consumable dose of §4.2.
	KindPill ItemKind = "pill"
	// KindTalisman is a single-use ward, activated through the item action.
	KindTalisman ItemKind = "talisman"
	// KindFormation is a deployable formation focus of §4.3.
	KindFormation ItemKind = "formation"
	// The three 法宝 kinds are 《自制-装备栏与法宝御使》§1's 攻击法宝／防御法宝／
	// 辅助法宝. They are separate kinds rather than one 法宝 kind because the
	// design gives each its own single slot, and because a slot is chosen from the
	// definition's kind — one kind would leave the equip path unable to tell which
	// of the three a copy belongs in.
	//
	// They are deliberately not a fourth and fifth 主法器: §1 says 武器与攻击法宝不是
	// 重复槽位, so a character may carry one of each, but 同一实物只能占一个位置 —
	// which the item module enforces by name rather than by kind.
	KindAttackTreasure  ItemKind = "attack_treasure"
	KindDefenseTreasure ItemKind = "defense_treasure"
	KindSupportTreasure ItemKind = "support_treasure"
	// KindMaterial is a measured quantity of a craft input: an herb, an ore, a
	// talisman blank. 《自制-修仙技艺》§2 requires a craft's real materials to be
	// consumed by the batch, and §5 requires the item catalogue to be reused
	// rather than a second inventory table invented for it, so a material is a
	// catalogue entry like any other — it has a stable identity, a published
	// price and a count, and it is spent by name.
	//
	// It is deliberately not equippable and not consumable as a dose. A herb is
	// not a pill: 丹药 is what a character takes in a fight, and §10's
	// 背包法器不全加攻 is the same separation read from the other side.
	KindMaterial ItemKind = "material"
)

// ItemDef is one registered catalogue entry.
//
// Exactly one of the payload fields is set, according to Kind, so a definition
// cannot carry numbers its kind does not use — a pill with an attack ceiling
// would be a second, unchecked budget.
type ItemDef struct {
	DefinitionDetails
	// ID is the stable identity, and NameKey the localisation key, so the
	// interface supplies the wording rather than this layer.
	ID      string     `json:"id,omitempty"`
	NameKey string     `json:"name_key,omitempty"`
	Kind    ItemKind   `json:"kind,omitempty"`
	Grade   ItemGrade  `json:"grade,omitempty"`
	Rarity  ItemRarity `json:"rarity,omitempty"`
	// RaritySpentOnMainStat is how much of the rarity's extra budget this
	// definition spends on its main attribute rather than on a special effect.
	// §4.1 makes this an explicit spend: the rarity budget is headroom the
	// definition may use, never a bonus it receives for being rare.
	RaritySpentOnMainStat float64 `json:"rarity_spent_on_main_stat,omitempty"`
	// Rated effects spend a separate part of the SAME rarity allowance.
	RaritySpentOnSpecialEffect float64 `json:"rarity_spent_on_special_effect,omitempty"`
	// EffectStage anchors a paid effect's capacity, never the user's current tier.
	EffectStage string `json:"effect_stage,omitempty"`
	// ApplicableStage is the lowest stage that may use the entry at all, as a
	// canonical stage key. §4.1 requires 适用阶段 to be written on a formal entry
	// and checked at use, and that it cannot be bypassed by naming a colour: a
	// 顶品 treasure is not merely a stronger 下品 one, it is one a low-stage
	// character may not be able to operate.
	//
	// An empty value means the entry states no floor, which is the honest
	// reading for a definition that never declared one. It is not a licence to
	// skip the check on an entry that did declare a stage.
	ApplicableStage string `json:"applicable_stage,omitempty"`
	// BasePrice is the published first-version retail reference for one copy,
	// in whole 下品灵石.
	//
	// §13.1 of 《自制-灵石经济与简单交易》 gives a category range per item class,
	// and §14 requires the base price to be a fixed property of the content
	// version: the same goods in the same market must reuse the promised quote
	// rather than re-roll it. Keeping the number on the definition is what makes
	// that structural — there is nowhere else for a turn to invent a price from.
	//
	// Zero means the entry publishes no price. A definition without a price
	// cannot be bought or sold: §13.2 says a rare resource is priced by its
	// registered use, and inventing a number at the counter is exactly what the
	// design forbids.
	BasePrice int64 `json:"base_price,omitempty"`
	// Implement is set for KindImplement.
	Implement *ImplementDef `json:"implement,omitempty"`
	// Armor is set for KindArmor.
	Armor *ArmorDef `json:"armor,omitempty"`
	// Treasure is set for the three 法宝 kinds.
	Treasure *TreasureDef `json:"treasure,omitempty"`
	// Pill is set for KindPill.
	Pill *PillDef `json:"pill,omitempty"`
	// Talisman is set for KindTalisman.
	Talisman *TalismanDef `json:"talisman,omitempty"`
	// Formation is set for KindFormation.
	Formation *FormationDef `json:"formation,omitempty"`
	// Material is set for KindMaterial.
	Material *MaterialDef `json:"material,omitempty"`
}

// MaterialDef is the rules payload of a craft input.
//
// It carries no attribute bonus at all. A material's contribution is what a
// registered craft method consumes it for, and 《自制-修仙技艺》§2 is explicit
// that holding a material is not holding a result: 不得先叙述出神器再临时注册效果.
// Keeping the payload empty is what makes "this herb cannot be worn as a weapon"
// structural rather than a rule someone has to remember.
type MaterialDef struct {
	// UnitKey is the localisation key for one unit's unit noun ("份", "块", "株"),
	// so a count reads in the story's language rather than as a bare number.
	UnitKey string
}

// ImplementDef is the rules payload of a main implement.
type ImplementDef struct {
	// AttackBonus is the total attack bonus, main-stat grade plus the rarity
	// budget actually spent on it.
	AttackBonus float64 `json:"attack_bonus,omitempty"`
	// DefensePierce is fixed absolute defense removal, shared under the 20% cap.
	DefensePierce int `json:"defense_pierce,omitempty"`
	// SpecialEffect names an authored capability. Legacy descriptive effects
	// consume the whole rarity allowance; rated numeric effects explicitly
	// divide that unchanged allowance between main-stat and special-effect
	// spends, so no portion can be counted twice.
	SpecialEffect string `json:"special_effect,omitempty"`
	// OperatingManaFraction is the real cost of driving the implement, as a
	// fraction of the wielder's bare stage mana maximum. It is what stops a
	// high-grade treasure from being free to invoke.
	OperatingManaFraction float64 `json:"operating_mana_fraction,omitempty"`
}

// ArmorDef is the rules payload of protective gear.
type ArmorDef struct {
	// DefenseBonus is the total defence bonus.
	DefenseBonus    float64 `json:"defense_bonus,omitempty"`
	StrainReduction float64 `json:"strain_reduction,omitempty"`
	SpecialEffect   string  `json:"special_effect,omitempty"`
	// OperatingManaFraction is the cost of wearing it in a real fight.
	OperatingManaFraction float64 `json:"operating_mana_fraction,omitempty"`
}

// TreasureDef is the rules payload of one 法宝.
//
// It reuses the same published budget tables as an implement and a piece of
// armour rather than opening a fourth one: 《自制-装备栏与法宝御使》§3 keeps the
// old framework and says 新增3个法宝位置是增加可选择的战术工具，不自动给每件再加一份
// 攻击／防御，也不扩大旧上限. The numbers a treasure contributes therefore come from
// the same grade-plus-rarity arithmetic, and the aggregation's 60%／40% cap sees
// them as ordinary sources.
//
// A treasure is not merely a second implement. §1 says 装备不等于自动发动全部能力
// and §2 says the treasure's own registered capability decides what it can do, so
// the payload records which attribute it strengthens and what it costs to drive.
// An entry with no bonus and no cost is a treasure that is owned but grants
// nothing, which is the honest reading of a 法宝 the author never gave a
// capability.
type TreasureDef struct {
	// BonusAttribute names the one attribute this treasure strengthens, and
	// Bonus its size. §1's three slots are distinguished by use rather than by
	// attribute — a 攻击法宝 need not add attack — so the attribute is recorded
	// rather than inferred from the kind.
	//
	// An empty attribute means the treasure contributes no numeric bonus. That is
	// the case the design asks for: 法宝的独特价值优先体现为有消耗、有条件的具体用途.
	BonusAttribute AttributeName `json:"bonus_attribute,omitempty"`
	Bonus          float64       `json:"bonus,omitempty"`
	// SpecialEffect names the non-numeric effect the rarity budget bought, when
	// the budget was not spent on the bonus.
	SpecialEffect string `json:"special_effect,omitempty"`
	// OperatingManaFraction is the real cost of driving it for one exchange, as a
	// fraction of the wielder's bare stage mana maximum.
	OperatingManaFraction float64 `json:"operating_mana_fraction,omitempty"`
}

// Validate refuses a catalogue entry that exceeds its published budget.
//
// The check runs before registration for the same reason a technique's does:
// §4.1's ceilings are a property of the definition, and a definition that is
// merely capped at aggregation time would still be an out-of-budget entry the
// player could obtain.
func (d ItemDef) Validate() error {
	if err := d.DefinitionDetails.Validate(); err != nil {
		return err
	}
	if d.QualityGrade != "" && !d.QualityGrade.Valid() {
		return ErrItemOverBudget
	}
	if strings.TrimSpace(d.ID) == "" {
		return ErrItemMissingID
	}
	// §4.1 requires the applicable stage to be recorded on a formal entry. A
	// stage the rules cannot read is refused here rather than at use, because a
	// requirement nobody can evaluate would either block the item forever or be
	// silently ignored.
	if strings.TrimSpace(d.ApplicableStage) != "" {
		if _, ok := ParseStageKey(d.ApplicableStage); !ok {
			return ErrItemUnknownStage
		}
	}
	// The grade lives on the entry and again on its payload, because PillDef and
	// FormationDef are usable on their own. Two copies can disagree, and a
	// disagreement would silently price the item against one grade while the
	// catalogue displays another, so it is refused rather than reconciled.
	if payloadGrade, set := d.payloadGrade(); set && payloadGrade != d.Grade {
		return ErrItemGradeMismatch
	}
	// A price is a whole, positive number of 下品灵石. §14 forbids fractional
	// coin, and §13.1's cheapest category is 1, so zero or negative is a
	// definition that published no price rather than a free item.
	if d.BasePrice < 0 {
		return ErrItemInvalidPrice
	}
	// The payload is validated against the entry's grade, so an author who
	// writes the grade only once — on the entry — gets a definition that is
	// checked and usable, rather than one that fails because its payload looks
	// ungraded. A direct Validate call therefore behaves exactly like Register.
	d = d.withGrade()
	if err := d.validateRatedEquipmentEffects(); err != nil {
		return err
	}
	switch d.Kind {
	case KindImplement:
		if d.Implement == nil {
			return ErrItemPayloadMismatch
		}
		ceiling, ok := MainImplementBonus(d.Grade, d.Rarity, d.RaritySpentOnMainStat)
		if !ok {
			return ErrUnsupportedGrade
		}
		if d.Implement.AttackBonus < 0 {
			return ErrItemNegativeBonus
		}
		// A definition that spends its rarity budget on an effect must not also
		// carry that budget as attack. This is checked before the ceiling so the
		// specific mistake is reported instead of the generic one.
		if d.Implement.SpecialEffect != "" && d.Implement.DefensePierce == 0 && d.Implement.AttackBonus > mainStatOnlyBonus(d.Grade) {
			return ErrItemBudgetCountedTwice
		}
		if d.Implement.AttackBonus > ceiling {
			return ErrItemOverBudget
		}
	case KindArmor:
		if d.Armor == nil {
			return ErrItemPayloadMismatch
		}
		ceiling, ok := ArmorBonus(d.Grade, d.Rarity, d.RaritySpentOnMainStat)
		if !ok {
			return ErrUnsupportedGrade
		}
		if d.Armor.DefenseBonus < 0 {
			return ErrItemNegativeBonus
		}
		if d.Armor.SpecialEffect != "" && d.Armor.StrainReduction == 0 && d.Armor.DefenseBonus > mainStatOnlyBonus(d.Grade) {
			return ErrItemBudgetCountedTwice
		}
		if d.Armor.DefenseBonus > ceiling {
			return ErrItemOverBudget
		}
	case KindPill:
		if d.Pill == nil {
			return ErrItemPayloadMismatch
		}
		if _, ok := pillPoolFraction[d.Grade]; !ok {
			return ErrUnsupportedGrade
		}
		if d.Pill.Purpose == "" {
			return ErrItemPillMissingPurpose
		}
		// A cultivation-support dose is priced from its own published strength
		// and duration. §6.2 line 232 keeps those on the item data, so a
		// definition that names the purpose without them would be a dose with
		// no rate and no end, which is exactly the permanent multiplier the
		// rule forbids.
		if d.Pill.Purpose == PillSupportCultivation {
			if _, ok := d.Pill.SupportStrengthOf(); !ok {
				return ErrItemPillSupportIncomplete
			}
			if d.Pill.SupportDays <= 0 {
				return ErrItemPillSupportIncomplete
			}
		}
	case KindTalisman:
		if d.Talisman == nil {
			return ErrItemPayloadMismatch
		}
		if err := d.Talisman.Validate(); err != nil {
			return err
		}
	case KindFormation:
		if d.Formation == nil {
			return ErrItemPayloadMismatch
		}
		if err := d.Formation.Validate(); err != nil {
			return err
		}
	case KindMaterial:
		if d.Material == nil {
			return ErrItemPayloadMismatch
		}
	case KindAttackTreasure, KindDefenseTreasure, KindSupportTreasure:
		if d.Treasure == nil {
			return ErrItemPayloadMismatch
		}
		// A treasure is priced from the same budget arithmetic as an implement and
		// a piece of armour, so an entry that exceeds the ceiling is refused here
		// for the same reason: §3 says the new slots do not enlarge the old cap.
		//
		// The bonus is checked only when the entry claims one. A treasure with no
		// bonus is a legitimate entry — §1 says a 法宝 that has no registered
		// capability grants nothing — and demanding a number would force every
		// author to invent one.
		if d.Treasure.Bonus < 0 {
			return ErrItemNegativeBonus
		}
		if d.Treasure.BonusAttribute != "" {
			if _, ok := AttributeByName(d.Treasure.BonusAttribute); !ok {
				return ErrItemUnknownAttribute
			}
			ceiling, ok := MainImplementBonus(d.Grade, d.Rarity, d.RaritySpentOnMainStat)
			if !ok {
				return ErrUnsupportedGrade
			}
			if d.Treasure.SpecialEffect != "" && d.Treasure.Bonus > mainStatOnlyBonus(d.Grade) {
				return ErrItemBudgetCountedTwice
			}
			if d.Treasure.Bonus > ceiling {
				return ErrItemOverBudget
			}
		} else if d.Treasure.Bonus > 0 {
			// A bonus with no attribute would be a number the aggregation can never
			// place. Refusing it is what stops a definition from carrying an
			// unattached amount.
			return ErrItemUnknownAttribute
		}
	default:
		return ErrItemUnknownKind
	}
	return nil
}

// payloadGrade reports the grade carried by the kind's payload, and whether the
// payload sets one at all.
func (d ItemDef) payloadGrade() (ItemGrade, bool) {
	switch d.Kind {
	case KindPill:
		if d.Pill != nil && d.Pill.Grade != "" {
			return d.Pill.Grade, true
		}
	case KindFormation:
		if d.Formation != nil && d.Formation.Grade != "" {
			return d.Formation.Grade, true
		}
	}
	return "", false
}

// withGrade returns a copy whose payload carries the entry's grade.
//
// Registration stamps the grade rather than asking an author to repeat it, so
// the stored definition has exactly one authoritative grade and a payload that
// cannot disagree with it. An author who does write both is still checked, by
// Validate, against a mismatch.
func (d ItemDef) withGrade() ItemDef {
	switch d.Kind {
	case KindPill:
		if d.Pill != nil && d.Pill.Grade == "" {
			pill := *d.Pill
			pill.Grade = d.Grade
			d.Pill = &pill
		}
	case KindFormation:
		if d.Formation != nil && d.Formation.Grade == "" {
			formation := *d.Formation
			formation.Grade = d.Grade
			d.Formation = &formation
		}
	}
	return d
}

// mainStatOnlyBonus is the grade's main-attribute bonus with no rarity spend.
// Legacy descriptive effects cannot exceed it; rated effects additionally
// validate their explicit, non-overlapping portions of the rarity budget.
func mainStatOnlyBonus(grade ItemGrade) float64 {
	return gradeMainStatBonus[grade]
}

// UsableBy reports whether a character at this stage may operate the entry.
//
// §4.1 requires the applicable stage, the 神识/驾驭 requirement and the real
// driving cost to be checked at use, and says explicitly that 适用阶段不是点击颜色
// 就能越过. This is that check. It is a rule about the definition, not about the
// character's recorded copy: a story that wrote a 顶品 treasure into a 炼气
// character's bag has recorded that they own it, not that they can wield it.
//
// An entry with no applicable stage is usable by anyone, which is the case for a
// definition that never declared a floor. A stage the rules cannot read is
// refused rather than assumed reachable, matching stageAtLeast.
func (d ItemDef) UsableBy(stage Cultivation) error {
	minimum := strings.TrimSpace(d.ApplicableStage)
	if minimum == "" {
		return nil
	}
	if !stageAtLeast(stage, minimum) {
		return ErrItemStageTooLow
	}
	return nil
}

// OperatingManaCost is the real mana cost of driving this item for one exchange.
//
// §4.1 and 数值规则 line 128 require the cost to come from the equipment's own
// definition, so an ordinary attack performed with a treasure in hand is not
// free merely because it used the plain-attack action. The amount is the
// published fraction of the wielder's bare stage mana maximum, rounded up, and
// it is read from the payload rather than from the recorded copy, so a story
// cannot make its own treasure cheaper.
//
// It returns 0 for anything that is not worn equipment and for an entry that
// declares no fraction — a pill, a formation focus, or an implement that costs
// nothing to swing.
func (d ItemDef) OperatingManaCost(stage Cultivation) int {
	fraction := d.OperatingManaFraction()
	if fraction <= 0 {
		return 0
	}
	bare := BaseAttributes(stage).ManaMax
	if bare <= 0 {
		return 0
	}
	return int(ceilNonNegative(fraction * float64(bare)))
}

// OperatingManaFraction is the published fraction of the wielder's bare stage
// mana that driving this item costs per exchange.
func (d ItemDef) OperatingManaFraction() float64 {
	switch d.Kind {
	case KindImplement:
		if d.Implement != nil {
			return d.Implement.OperatingManaFraction
		}
	case KindArmor:
		if d.Armor != nil {
			return d.Armor.OperatingManaFraction
		}
	case KindAttackTreasure, KindDefenseTreasure, KindSupportTreasure:
		if d.Treasure != nil {
			return d.Treasure.OperatingManaFraction
		}
	}
	return 0
}

// Equippable reports whether this entry is worn rather than consumed or
// deployed.
//
// §4.1 gives the character exactly one main implement and one piece of armour,
// and a formation is deployed into a slot of its own, so all three are equipped.
// 《自制-装备栏与法宝御使》§1 gives the three 法宝 kinds a slot each, so they are
// equipped too. A pill is not: it is spent, and treating it as equipment would
// let a dose be worn instead of taken.
func (d ItemDef) Equippable() bool {
	switch d.Kind {
	case KindImplement, KindArmor, KindFormation,
		KindAttackTreasure, KindDefenseTreasure, KindSupportTreasure:
		return true
	}
	return false
}

// ItemCatalog holds the registered entries of one story branch.
type ItemCatalog struct {
	lookup    func(string) (ItemDef, bool)
	lookupIDs func() []string
	defs      map[string]ItemDef
	order     []string
}

// NewItemCatalog returns an empty catalogue.
func NewItemCatalog() *ItemCatalog {
	return &ItemCatalog{defs: map[string]ItemDef{}}
}

// NewItemCatalogWithFirstVersion returns a catalogue holding the first-version
// representative set of §9.
func NewItemCatalogWithFirstVersion(rules ...*Rules) *ItemCatalog {
	catalog := NewItemCatalog()
	if len(rules) > 0 && rules[0] != nil {
		for _, def := range rules[0].Items {
			_ = catalog.Register(def)
		}
		return catalog
	}
	for _, def := range append(append([]ItemDef(nil), firstVersionCatalog...), spiritWorldItems...) {
		_ = catalog.Register(def)
	}
	return catalog
}

// Register validates and adds one definition.
//
// A duplicate ID is refused rather than overwritten: §6 requires that a balance
// change be a version change rather than a silent rewrite of an existing entry,
// and this first version carries no versioning on items, so replacing an entry
// is deliberately not offered.
func (c *ItemCatalog) Register(def ItemDef) error {
	if c.defs == nil {
		return ErrItemCatalogNotInitialised
	}
	if err := def.Validate(); err != nil {
		return err
	}
	// The stored definition carries one authoritative grade: the payload is
	// stamped from the entry so a caller reading PillDef.Grade cannot see a
	// different grade from the one the catalogue validated against.
	def = def.withGrade()
	if c.lookup != nil {
		_, _ = c.Definition(def.ID)
	}
	if _, exists := c.defs[def.ID]; exists {
		return ErrItemDuplicateID
	}
	c.defs[def.ID] = def
	c.order = append(c.order, def.ID)
	return nil
}

// Definition returns a registered entry.
func (c *ItemCatalog) Definition(id string) (ItemDef, bool) {
	if c == nil {
		return ItemDef{}, false
	}
	id = strings.TrimSpace(id)
	def, ok := c.defs[id]
	if !ok && c.lookup != nil {
		def, ok = c.lookup(id)
		if !ok || def.ID != id || def.Validate() != nil {
			return ItemDef{}, false
		}
		def = def.withGrade()
		c.defs[id] = def
	}
	return def, ok
}

// Definitions lists every registered entry in registration order.
func (c *ItemCatalog) Definitions() []ItemDef {
	if c == nil {
		return nil
	}
	out := make([]ItemDef, 0, len(c.order))
	for _, id := range lookupCatalogIDs(c.order, c.lookupIDs) {
		if def, ok := c.Definition(id); ok {
			out = append(out, def)
		}
	}
	return out
}

// ItemInstance is one real copy a character holds.
//
// It is a separate fact from its definition: §7 requires that a definition's
// existence never be read as stock. Two characters may hold instances of one
// definition with different remaining durations, and an instance may be consumed
// while the definition stays registered for the next acquisition.
type ItemInstance struct {
	// InstanceID identifies this copy, so a retried command cannot consume the
	// same dose twice.
	InstanceID string
	// DefID names the registered definition this copy was made from.
	DefID string
	// Quantity is how many doses or copies remain.
	Quantity int
	// Equipped marks the copy currently worn or wielded. At most one implement
	// and one piece of armour may be equipped, which the caller enforces
	// against its own inventory; the flag is recorded here because it is a real
	// per-copy fact, not a property of the definition.
	Equipped bool
	// RemainingExchanges is the real remaining duration of an active temporary
	// effect this copy produced, such as a boosting pill's attack bonus.
	RemainingExchanges int
}

// ItemInstanceError reports an operation an instance refused.
type ItemInstanceError struct {
	InstanceID string
	Reason     error
}

func (e *ItemInstanceError) Error() string {
	return "cultivation: item instance " + e.InstanceID + ": " + e.Reason.Error()
}

func (e *ItemInstanceError) Unwrap() error { return e.Reason }

// NewItemInstance makes one copy of a registered definition.
//
// The definition must be registered first, so an instance can never reference
// numbers that were never fixed.
func (c *ItemCatalog) NewItemInstance(instanceID, defID string, quantity int) (ItemInstance, error) {
	def, ok := c.Definition(defID)
	if !ok {
		return ItemInstance{}, &ItemInstanceError{InstanceID: instanceID, Reason: ErrItemDefinitionNotFound}
	}
	if quantity <= 0 {
		return ItemInstance{}, &ItemInstanceError{InstanceID: instanceID, Reason: ErrItemQuantityOutOfRange}
	}
	if def.Kind == KindImplement || def.Kind == KindArmor {
		// Equipment is one real object, not a stack.
		quantity = 1
	}
	return ItemInstance{InstanceID: instanceID, DefID: def.ID, Quantity: quantity}, nil
}

// Consume spends one dose of a consumable instance.
//
// A refused consumption changes nothing, which is what §8 requires: an illegal
// parameter, a missing target or a failed check must not deduct the resource. A
// repeated command therefore cannot silently eat a second dose, because the
// second call finds the same pre-state and the same refusal.
func (i ItemInstance) Consume(catalog *ItemCatalog) (ItemInstance, error) {
	def, ok := catalog.Definition(i.DefID)
	if !ok {
		return i, &ItemInstanceError{InstanceID: i.InstanceID, Reason: ErrItemDefinitionNotFound}
	}
	if def.Kind != KindPill {
		return i, &ItemInstanceError{InstanceID: i.InstanceID, Reason: ErrItemNotConsumable}
	}
	if i.Quantity <= 0 {
		return i, &ItemInstanceError{InstanceID: i.InstanceID, Reason: ErrItemExhausted}
	}
	i.Quantity--
	return i, nil
}

// firstVersionCatalog is the §9 representative set.
//
// Each entry is chosen to reach a published boundary rather than to be
// interesting: 下品常见 shows the white-item 5%, 中品稀有 shows the 15% ceiling,
// and 顶品独特 shows that 35% is reachable only at the very top.
var firstVersionCatalog = append([]ItemDef{
	{
		// 下品常见: the white item. 5% and nothing more, so the design's
		// "白装＋5%且不能变成＋35%" is a property of a real entry.
		ID: "implement.lower.common", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.implement.lower_common.name",
		Kind: KindImplement, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice:       20,
		ApplicableStage: "qi_1",
		Implement:       &ImplementDef{AttackBonus: 0.05},
	},
	{
		// 中品稀有: 10% grade plus the 5% rarity budget, spent on the main
		// attribute, reaching the published 15% ceiling for this pair.
		ID: "implement.middle.rare", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, NameKey: "item.implement.middle_rare.name",
		Kind: KindImplement, Grade: GradeMiddle, Rarity: RarityRare,
		BasePrice:             60,
		ApplicableStage:       "qi_4",
		RaritySpentOnMainStat: 0.05,
		Implement:             &ImplementDef{AttackBonus: 0.15},
	},
	{
		// 顶品独特: 25% grade plus the 10% rarity budget reaches the published
		// 35% upper bound. Only this pair may reach it.
		ID: "implement.top.unique", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, NameKey: "item.implement.top_unique.name",
		Kind: KindImplement, Grade: GradeTop, Rarity: RarityUnique,
		BasePrice:             500,
		ApplicableStage:       "foundation_early",
		RaritySpentOnMainStat: 0.10,
		Implement:             &ImplementDef{AttackBonus: 0.35},
	},
	{
		// A rare implement that spends its budget on an effect instead of on
		// attack: it keeps only its grade's 10%, which is the case that proves
		// the rarity budget is headroom rather than an automatic bonus.
		ID: "implement.upper.rare.effect", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, NameKey: "item.implement.upper_rare_effect.name",
		Kind: KindImplement, Grade: GradeUpper, Rarity: RarityRare,
		BasePrice:             150,
		ApplicableStage:       "qi_7",
		RaritySpentOnMainStat: 0,
		Implement: &ImplementDef{
			AttackBonus:           0.18,
			SpecialEffect:         "item.effect.implement.meridian_throughput",
			OperatingManaFraction: 0.02,
		},
	},
	{
		// 中品稀有 armour: 10% plus 5%, the same budget arithmetic on defence.
		ID: "armor.middle.rare", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, NameKey: "item.armor.middle_rare.name",
		Kind: KindArmor, Grade: GradeMiddle, Rarity: RarityRare,
		BasePrice:             60,
		ApplicableStage:       "qi_4",
		RaritySpentOnMainStat: 0.05,
		Armor:                 &ArmorDef{DefenseBonus: 0.15},
	},
	{
		// A restoring pill whose amount is fixed to the stage it was brewed at,
		// so a stronger character does not get a stronger dose.
		ID: "pill.restore.health.lower", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.pill.restore_health_lower.name",
		Kind: KindPill, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 3,
		Pill: &PillDef{
			Purpose:         PillRestoreHealth,
			RegisteredStage: Qi(6),
		},
	},
	{
		// The attack-boosting pill, with the 3-exchange duration of §4.2.
		ID: "pill.boost.attack.middle", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("green")}, NameKey: "item.pill.boost_attack_middle.name",
		Kind: KindPill, Grade: GradeMiddle, Rarity: RarityCommon,
		BasePrice: 8,
		Pill: &PillDef{
			Purpose:           PillBoostAttack,
			DurationExchanges: 3,
		},
	},
	{
		// The first cultivation-support dose. §6.2 prices 普通辅助 at 1.20 and
		// requires the quantity and the effective duration to live on the item
		// data, so this one supports one rule month of training. A character who
		// names a longer seclusion than the stock covers is trained at the
		// supported rate only for the days the doses really pay for; the rest is
		// ordinary, which is the "耗尽后从那个时间点换速率" the rule states.
		ID: "pill.support.cultivation.lower", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.pill.support_cultivation_lower.name",
		Kind: KindPill, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 5,
		Pill: &PillDef{
			Purpose:         PillSupportCultivation,
			SupportStrength: SupportNormal,
			SupportDays:     float64(DaysPerRuleMonth),
		},
	},
	{
		// A single-type attack formation. It grants one main bonus, which
		// Validate enforces.
		ID: "formation.attack.middle", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("green")}, NameKey: "item.formation.attack_middle.name",
		Kind: KindFormation, Grade: GradeMiddle, Rarity: RarityCommon,
		BasePrice:       50,
		ApplicableStage: "qi_4",
		Formation:       &FormationDef{AttackBonus: 0.08},
	},
	{
		ID: "talisman.ward.lower", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.talisman.ward_lower.name",
		Kind: KindTalisman, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 12, ApplicableStage: "qi_1",
		Talisman: &TalismanDef{RegisteredStage: Qi(1), ShieldFraction: 0.20, DurationExchanges: 3},
	},
	{
		ID: "material.talisman.paper", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.material.talisman_paper.name",
		Kind: KindMaterial, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 2, Material: &MaterialDef{UnitKey: "item.unit.portion"},
	},
	{
		ID: "material.talisman.ink", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.material.talisman_ink.name",
		Kind: KindMaterial, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 3, Material: &MaterialDef{UnitKey: "item.unit.portion"},
	},
	{
		// The first craft input. 《自制-修仙技艺》§2 requires a craft's materials to
		// be real items that are really consumed, and §5 forbids a second
		// inventory table for them, so the herb is a catalogue entry: it has an
		// identity, a published price and a count, and the craft module spends it
		// by name. It grants nothing on its own — that is the point of listing it
		// here rather than letting the prose carry it, since an unpriced herb
		// could be claimed in any quantity.
		ID: "material.herb.green_vine", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.material.herb_green_vine.name",
		Kind: KindMaterial, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 2,
		Material:  &MaterialDef{UnitKey: "item.unit.portion"},
	},
	{
		// The smithing input. §3 of 《自制-修仙技艺》 requires the four arts to share
		// the existing item templates rather than each growing an engine, so 炼器
		// spends a real material and yields a definition the catalogue already
		// prices: the same 凡铁剑 a story can otherwise only be handed. Naming the
		// ore rather than letting prose carry it is the same reason the herb is
		// here — an unpriced input could be claimed in any quantity, and §2 forbids
		// a batch that costs nothing.
		ID: "material.ore.iron", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.material.ore_iron.name",
		Kind: KindMaterial, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 3,
		Material:  &MaterialDef{UnitKey: "item.unit.block"},
	},
	{
		// The array input. A formation focus is 阵盘 in the story's own words, and
		// what it is cut from is the stone this entry names. It is a separate entry
		// from the ore because §2 keeps a craft's materials real: a sword and a
		// formation are not made of interchangeable stock, and one material serving
		// both would make every method's cost the same number.
		ID: "material.stone.ward", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, NameKey: "item.material.stone_ward.name",
		Kind: KindMaterial, Grade: GradeLower, Rarity: RarityCommon,
		BasePrice: 6,
		Material:  &MaterialDef{UnitKey: "item.unit.block"},
	},
	{
		// The 攻击法宝 representative. 《自制-装备栏与法宝御使》§1 gives it a slot
		// of its own beside the weapon, and §3 says the new slots must not enlarge
		// the old ceiling — so its attack bonus is priced from the same table as an
		// implement of the same grade and rarity, and it carries a real operating
		// cost so a treasure is not free to drive.
		ID: "treasure.attack.middle", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, NameKey: "item.treasure.attack_middle.name",
		Kind: KindAttackTreasure, Grade: GradeMiddle, Rarity: RarityRare,
		BasePrice:             80,
		ApplicableStage:       "qi_4",
		RaritySpentOnMainStat: 0.05,
		Treasure: &TreasureDef{
			BonusAttribute:        AttrAttack,
			Bonus:                 0.15,
			OperatingManaFraction: 0.02,
		},
	},
	{
		// The 防御法宝 representative. §1's 防御法宝 covers 护盾、格挡等已支持的防御用途,
		// and §2 says 装备不等于自动发动全部能力: the entry grants a defence bonus
		// that the settlement reads while it is equipped, and nothing more.
		ID: "treasure.defense.middle", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, NameKey: "item.treasure.defense_middle.name",
		Kind: KindDefenseTreasure, Grade: GradeMiddle, Rarity: RarityRare,
		BasePrice:             80,
		ApplicableStage:       "qi_4",
		RaritySpentOnMainStat: 0.05,
		Treasure: &TreasureDef{
			BonusAttribute:        AttrDefense,
			Bonus:                 0.15,
			OperatingManaFraction: 0.02,
		},
	},
	{
		// The support representative currently strengthens perception for escape
		// and pursuit checks. It does not grant supernatural sight: there is no
		// registered executor for that ability, so no such effect is advertised.
		ID: "treasure.support.upper.effect", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, NameKey: "item.treasure.support_upper_effect.name",
		Kind: KindSupportTreasure, Grade: GradeUpper, Rarity: RarityRare,
		BasePrice:             150,
		ApplicableStage:       "qi_7",
		RaritySpentOnMainStat: 0,
		Treasure: &TreasureDef{
			BonusAttribute:        AttrPerception,
			Bonus:                 0.18,
			OperatingManaFraction: 0.02,
		},
	},
}, highRealmItems...)
