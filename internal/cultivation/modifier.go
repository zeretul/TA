package cultivation

import "math"

// This file implements the unified attribute budget of
// 《自制-修仙数值规则》V0.3 §3.4: every source of an attribute change — technique
// specialisation, equipment, pills, formations, ally assistance, event rewards —
// enters one aggregation, so no combination can multiply past the published cap
// by arriving under several names.
//
// The design states the rule as:
//
//	actual = base × max(0, 1 + min(sum of positive, positive cap)
//	                            − min(sum of negative, negative cap))
//
// applied per attribute, then rounded once. Negative modifiers do not release
// positive headroom: the two are capped separately and only then combined.

// SourceKind names where a modifier comes from. It is used to enforce the rule
// that the same reason cannot be counted twice, and that a temporary effect of
// one kind replaces rather than stacks with another of the same group.
type SourceKind string

const (
	// SourceTechniqueSpecialisation is the 均衡/强攻/防护/轻身/神识 template
	// of §3.1.
	SourceTechniqueSpecialisation SourceKind = "technique_specialisation"
	// SourceTechniqueQuality is the 普通/良好/高阶 attack correction of §3.1.
	SourceTechniqueQuality SourceKind = "technique_quality"
	// SourceMainImplement is the one active main implement of §3.2.
	SourceMainImplement SourceKind = "main_implement"
	// SourceArmor is the one active piece of protective gear of §3.2.
	SourceArmor SourceKind = "armor"
	// SourceTreasure is one of the three equipped 法宝 of
	// 《自制-装备栏与法宝御使》§1. It is a source of its own rather than reusing
	// SourceMainImplement because the two are different slots with different
	// operating costs, and a breakdown that named one as the other would tell the
	// player their weapon grants what their treasure grants.
	//
	// It is still an ordinary source to the aggregation: §3 keeps the old caps, so
	// a treasure's bonus is summed with every other positive modifier under the
	// same 60%／40% ceiling.
	SourceTreasure SourceKind = "treasure"
	// SourceMedicine is a temporary attack-boosting pill of §3.5.
	SourceMedicine SourceKind = "medicine"
	// SourceFormation is a formation's single main bonus of §3.5.
	SourceFormation SourceKind = "formation"
	// SourceAllyAssistance is a real, present ally's help.
	SourceAllyAssistance SourceKind = "ally_assistance"
	// SourceEventReward is a story-granted bonus with a real source.
	SourceEventReward SourceKind = "event_reward"
	// SourceInnateTalent is a 天赋特性 the character was created with. It is a
	// source of its own because it is neither an obtained copy nor a story
	// reward: it cannot be lost by putting something down, and it is written once
	// at creation rather than granted again by a turn.
	SourceInnateTalent SourceKind = "innate_talent"
	// SourceEnvironment is a sourced battlefield condition. It is applied as a
	// combat multiplier, not as an attribute modifier, so it must never appear
	// here — see DamageContext.
	SourceEnvironment SourceKind = "environment"
)

// AttributeModifier is one real, sourced change to one attribute.
//
// Value is a signed fraction: +0.05 is a 5% increase, −0.20 a 20% decrease.
type AttributeModifier struct {
	// ID identifies the specific effect, so a retried commit can be recognised
	// and the same effect is not applied twice.
	ID string
	// Kind is the source category.
	Kind SourceKind
	// Attribute names the affected attribute. Empty means the modifier applies
	// to every attribute in its own group, which only the specialisation
	// template uses.
	Attribute AttributeName
	// Value is the signed fraction.
	Value float64
	// Group is the stacking group. Two temporary effects in the same non-empty
	// group do not add: the strongest effective one applies. An empty group
	// means the modifier is intrinsic to its source and always applies.
	Group string
	// Temporary marks an effect with a real remaining duration. Temporary
	// effects in the same group replace each other; a permanent modifier and a
	// temporary one in the same group both apply, because they are different
	// reasons.
	Temporary bool
}

// AttributeName identifies one of the six displayed attributes.
type AttributeName string

const (
	AttrHealthMax  AttributeName = "health_max"
	AttrManaMax    AttributeName = "mana_max"
	AttrAttack     AttributeName = "attack"
	AttrDefense    AttributeName = "defense"
	AttrAgility    AttributeName = "agility"
	AttrPerception AttributeName = "perception"
)

// AllAttributes lists the six attributes in display order.
var AllAttributes = []AttributeName{
	AttrHealthMax, AttrManaMax, AttrAttack, AttrDefense, AttrAgility, AttrPerception,
}

// AttributeByName reports whether a name is one of the six displayed attributes.
//
// It exists so a definition that carries an attribute by name — a treasure's
// bonus attribute — can be refused at registration rather than contributing
// nothing silently at settlement time.
func AttributeByName(name AttributeName) (AttributeName, bool) {
	for _, attribute := range AllAttributes {
		if attribute == name {
			return attribute, true
		}
	}
	return "", false
}

// positiveCap is the published ceiling on the sum of positive modifiers.
//
// Attack, defense, agility and perception share one 60% cap; the two pools have
// their own 40% cap. Keeping them in one function means a caller cannot invent
// a third cap by asking for an attribute that does not exist.
func positiveCap(attribute AttributeName) float64 {
	switch attribute {
	case AttrHealthMax, AttrManaMax:
		return 0.40
	}
	return 0.60
}

// negativeCap is the published ceiling on the sum of negative modifiers, which
// is 50% for every attribute.
func negativeCap(AttributeName) float64 { return 0.50 }

// AttributeBreakdown is the auditable result of one aggregation: what the base
// was, which sources were counted, and which were dropped.
//
// The design requires the interface to be able to explain which extra effects
// did not raise the current ability, so the dropped list is part of the result
// rather than something a caller reconstructs.
type AttributeBreakdown struct {
	Attribute AttributeName
	Base      int
	// PositiveSum and NegativeSum are the capped sums actually applied.
	PositiveSum float64
	NegativeSum float64
	// Applied lists the IDs that contributed.
	Applied []string
	// Dropped lists the IDs that were superseded by a stronger effect in the
	// same temporary group.
	Dropped []string
	// Value is the final whole number.
	Value int
}

// selectEffective reduces the supplied modifiers for one attribute to the set
// that actually applies.
//
// Within a temporary stacking group only the strongest effective modifier
// survives, which is how the design stops a second attack pill or a second
// same-type formation from adding. Different groups, and non-temporary
// modifiers, all remain: they are genuinely different reasons.
func selectEffective(modifiers []AttributeModifier, attribute AttributeName) []AttributeModifier {
	// Track the strongest temporary modifier per group.
	strongest := map[string]AttributeModifier{}
	for _, m := range modifiers {
		if !m.Temporary || m.Group == "" {
			continue
		}
		current, seen := strongest[m.Group]
		if !seen || math.Abs(m.Value) > math.Abs(current.Value) {
			strongest[m.Group] = m
		}
	}

	applied := make([]AttributeModifier, 0, len(modifiers))
	for _, m := range modifiers {
		if !m.appliesTo(attribute) {
			continue
		}
		if m.Temporary && m.Group != "" {
			if winner, ok := strongest[m.Group]; ok && winner.ID != m.ID {
				continue
			}
		}
		applied = append(applied, m)
	}
	return applied
}

// appliesTo reports whether the modifier affects the given attribute. An empty
// Attribute means the modifier carries its own explicit per-attribute values,
// which the caller expands before reaching here.
func (m AttributeModifier) appliesTo(attribute AttributeName) bool {
	return m.Attribute == attribute
}

// AggregateAttribute computes one attribute from its base value and the real
// modifiers in effect.
func AggregateAttribute(attribute AttributeName, base int, modifiers []AttributeModifier, rules ...*Rules) AttributeBreakdown {
	posCap, negCap := positiveCap(attribute), negativeCap(attribute)
	if len(rules) > 0 && rules[0] != nil {
		posCap, negCap = rules[0].PositiveCaps[attribute], rules[0].NegativeCaps[attribute]
	}
	effective := selectEffective(modifiers, attribute)

	positive := 0.0
	negative := 0.0
	applied := make([]string, 0, len(effective))
	for _, m := range effective {
		if m.Value > 0 {
			positive += m.Value
		} else {
			negative += m.Value
		}
		applied = append(applied, m.ID)
	}

	cappedPositive := math.Min(positive, posCap)
	// Negative is summed as a negative number, so the cap is applied to its
	// magnitude and re-signed.
	cappedNegative := -math.Min(-negative, negCap)

	scale := math.Max(0, 1+cappedPositive+cappedNegative)
	value := int(roundHalfUp(float64(base) * scale))

	return AttributeBreakdown{
		Attribute:   attribute,
		Base:        base,
		PositiveSum: cappedPositive,
		NegativeSum: cappedNegative,
		Applied:     applied,
		Value:       value,
	}
}

// AggregateAttributes computes all six attributes and returns them as a ready
// Attributes value with current pools set to the derived maxima.
//
// Current health and mana are set to the maxima here because this function has
// no knowledge of real wounds; a caller holding real current values overwrites
// them and clamps to the new maxima, as the design requires when a bonus is
// lost.
func AggregateAttributes(base Attributes, modifiers []AttributeModifier, rules ...*Rules) Attributes {
	baseValues := map[AttributeName]int{
		AttrHealthMax:  base.HealthMax,
		AttrManaMax:    base.ManaMax,
		AttrAttack:     base.Attack,
		AttrDefense:    base.Defense,
		AttrAgility:    base.Agility,
		AttrPerception: base.Perception,
	}

	// Aggregate once per attribute, so the same modifier list is not walked six
	// times with a different filter each time.
	values := make(map[AttributeName]int, len(AllAttributes))
	for _, attribute := range AllAttributes {
		values[attribute] = AggregateAttribute(attribute, baseValues[attribute], modifiers, rules...).Value
	}

	health := values[AttrHealthMax]
	mana := values[AttrManaMax]
	// Carry the real current values through, clamped to the new maxima: gaining
	// a bonus does not heal, and losing one only trims what exceeds the cap.
	currentHealth := clampInt(base.Health, 0, health)
	currentMana := clampInt(base.Mana, 0, mana)

	return Attributes{
		HealthMax:  health,
		Health:     currentHealth,
		ManaMax:    mana,
		Mana:       currentMana,
		Attack:     values[AttrAttack],
		Defense:    values[AttrDefense],
		Agility:    values[AttrAgility],
		Perception: values[AttrPerception],
	}
}

// AggregateBreakdown returns the per-attribute breakdown for the interface to
// explain which sources are in effect.
func AggregateBreakdown(base Attributes, modifiers []AttributeModifier, rules ...*Rules) []AttributeBreakdown {
	baseValues := map[AttributeName]int{
		AttrHealthMax:  base.HealthMax,
		AttrManaMax:    base.ManaMax,
		AttrAttack:     base.Attack,
		AttrDefense:    base.Defense,
		AttrAgility:    base.Agility,
		AttrPerception: base.Perception,
	}
	breakdowns := make([]AttributeBreakdown, 0, len(AllAttributes))
	for _, attribute := range AllAttributes {
		breakdowns = append(breakdowns, AggregateAttribute(attribute, baseValues[attribute], modifiers, rules...))
	}
	return breakdowns
}

// TechniqueSpecialisation is the §3.1 template: the six-attribute profile of a
// character's cultivation method, before its quality correction.
type TechniqueSpecialisation string

const (
	SpecialisationBalanced TechniqueSpecialisation = "balanced"
	SpecialisationAttack   TechniqueSpecialisation = "attack"
	SpecialisationDefense  TechniqueSpecialisation = "defense"
	SpecialisationLight    TechniqueSpecialisation = "light"
	SpecialisationSpirit   TechniqueSpecialisation = "spirit"
)

// specialisationProfile is the published §3.1 table. Values are signed
// fractions and are applied additively with equipment and quality, then
// multiplied into the base once — the design forbids chaining these
// passives multiplicatively.
var specialisationProfile = map[TechniqueSpecialisation]map[AttributeName]float64{
	SpecialisationBalanced: {},
	SpecialisationAttack: {
		AttrHealthMax: -0.05,
		AttrAttack:    0.10,
		AttrDefense:   -0.20,
	},
	SpecialisationDefense: {
		AttrHealthMax: 0.10,
		AttrAttack:    -0.10,
		AttrDefense:   0.20,
		AttrAgility:   -0.05,
	},
	SpecialisationLight: {
		AttrAttack:  -0.05,
		AttrDefense: -0.10,
		AttrAgility: 0.25,
	},
	SpecialisationSpirit: {
		AttrManaMax:    0.10,
		AttrAttack:     -0.05,
		AttrDefense:    -0.05,
		AttrPerception: 0.30,
	},
}

// SpecialisationModifiers expands a specialisation template into the per
// attribute modifiers the aggregation consumes.
func SpecialisationModifiers(s TechniqueSpecialisation) []AttributeModifier {
	profile, ok := specialisationProfile[s]
	if !ok {
		return nil
	}
	modifiers := make([]AttributeModifier, 0, len(profile))
	for attribute, value := range profile {
		modifiers = append(modifiers, AttributeModifier{
			ID:        "specialisation:" + string(s) + ":" + string(attribute),
			Kind:      SourceTechniqueSpecialisation,
			Attribute: attribute,
			Value:     value,
		})
	}
	return modifiers
}

// techniqueGradeAttackBonus is the published attack correction by technique
// quality (§3.1). The same quality also scales cultivation speed — see
// qualityScale in progress.go — but the two are separate budgets and are not
// multiplied together.
var techniqueGradeAttackBonus = map[TechniqueQuality]float64{
	QualityOrdinary: 0.00,
	QualityGood:     0.05,
	QualityAdvanced: 0.10,
}

// TechniqueQualityModifier returns the attack modifier for a technique grade.
func TechniqueQualityModifier(grade TechniqueQuality) []AttributeModifier {
	bonus, ok := techniqueGradeAttackBonus[grade]
	if !ok || bonus == 0 {
		return nil
	}
	return []AttributeModifier{{
		ID:        "technique_quality:" + string(grade),
		Kind:      SourceTechniqueQuality,
		Attribute: AttrAttack,
		Value:     bonus,
	}}
}

// ItemGrade is the 下品/中品/上品/顶品 quality tier of
// 《自制-装备与物品品级》V0.1.
type ItemGrade string

const (
	GradeLower  ItemGrade = "lower"
	GradeMiddle ItemGrade = "middle"
	GradeUpper  ItemGrade = "upper"
	GradeTop    ItemGrade = "top"
)

// ItemRarity is the acquisition-difficulty tier. Its budget is extra headroom
// that a definition may spend on the main attribute or on an explicit special
// effect — it is never an automatic bonus.
type ItemRarity string

const (
	RarityCommon   ItemRarity = "common"
	RarityUncommon ItemRarity = "uncommon"
	RarityRare     ItemRarity = "rare"
	RarityPrecious ItemRarity = "precious"
	RarityUnique   ItemRarity = "unique"
)

// gradeMainStatBonus is the published main-attribute bonus by grade.
var gradeMainStatBonus = map[ItemGrade]float64{
	GradeLower:  0.05,
	GradeMiddle: 0.10,
	GradeUpper:  0.18,
	GradeTop:    0.25,
}

// rarityExtraBudget is the published additional headroom by rarity.
var rarityExtraBudget = map[ItemRarity]float64{
	RarityCommon:   0.00,
	RarityUncommon: 0.02,
	RarityRare:     0.05,
	RarityPrecious: 0.08,
	RarityUnique:   0.10,
}

// MainImplementBonus returns the attack bonus of one active main implement.
//
// The rarity budget is deliberately NOT added automatically: it is a ceiling
// the item's definition may spend, and a common-tier white item must not reach
// the top-tier 35% merely by being rare. A definition that spends its rarity
// budget on the main attribute passes that spend explicitly.
func MainImplementBonus(grade ItemGrade, rarity ItemRarity, raritySpentOnMainStat float64) (float64, bool) {
	base, ok := gradeMainStatBonus[grade]
	if !ok {
		return 0, false
	}
	budget, ok := rarityExtraBudget[rarity]
	if !ok {
		return 0, false
	}
	spend := clampFloat(raritySpentOnMainStat, 0, budget)
	return base + spend, true
}

// ArmorBonus returns the defense bonus of one active piece of protective gear.
// The same budget rules apply, and the result is a defense modifier only: one
// percentage is not added to all six attributes at once.
func ArmorBonus(grade ItemGrade, rarity ItemRarity, raritySpentOnDefense float64) (float64, bool) {
	return MainImplementBonus(grade, rarity, raritySpentOnDefense)
}

// EquipmentModifier turns one active item into an attribute modifier.
//
// A caller must have already established that the item is equipped, operable,
// not lent out and not damaged; this function only does the arithmetic.
func EquipmentModifier(id string, attribute AttributeName, bonus float64) AttributeModifier {
	return AttributeModifier{
		ID:        id,
		Kind:      SourceMainImplement,
		Attribute: attribute,
		Value:     bonus,
	}
}
