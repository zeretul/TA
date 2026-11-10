package cultivation

import "math"

// This file implements the item rules of 《自制-装备与物品品级》V0.1 §4: pills and
// formations, plus the two restricted combat variants of
// 《自制-技能与施展》V0.2 §5.
//
// The recurring rule in both documents is that a tier's value is a ceiling the
// definition may spend, never an automatic bonus. A rare pill is not a stronger
// pill, and a rare formation is not a stronger formation; rarity changes how the
// item is obtained and what special use it may carry.

// PillPurpose is what one dose of a pill does. A single dose has one primary
// use; it does not grant every listed effect at once.
type PillPurpose string

const (
	// PillRestoreHealth restores health.
	PillRestoreHealth PillPurpose = "restore_health"
	// PillRestoreMana restores mana.
	PillRestoreMana PillPurpose = "restore_mana"
	// PillBoostAttack grants a temporary attack bonus.
	PillBoostAttack PillPurpose = "boost_attack"
	// PillSupportCultivation is an auxiliary cultivation resource: §6.2's
	// 普通辅助/强辅助. It is consumed outside a battle, by the training interval
	// that really spends it, and its effect ends when the dose's own published
	// duration does.
	PillSupportCultivation PillPurpose = "support_cultivation"
)

// pillPoolFraction is the published fraction of the registered stage's bare
// pool a restoring pill recovers, by grade.
var pillPoolFraction = map[ItemGrade]float64{
	GradeLower:  0.10,
	GradeMiddle: 0.15,
	GradeUpper:  0.20,
	GradeTop:    0.30,
}

// pillAttackBonus is the published attack bonus of an attack-boosting pill, by
// grade.
var pillAttackBonus = map[ItemGrade]float64{
	GradeLower:  0.05,
	GradeMiddle: 0.08,
	GradeUpper:  0.12,
	GradeTop:    0.15,
}

// PillDef is a registered pill.
type PillDef struct {
	ID      string      `json:"id,omitempty"`
	NameKey string      `json:"name_key,omitempty"`
	Grade   ItemGrade   `json:"grade,omitempty"`
	Rarity  ItemRarity  `json:"rarity,omitempty"`
	Purpose PillPurpose `json:"purpose,omitempty"`
	// RegisteredStage is the stage the dose's absolute effect was registered
	// against. A restoring pill's amount is fixed to this stage, so the same
	// dose gives the same recovery to a stronger user rather than scaling up.
	RegisteredStage Cultivation `json:"registered_stage,omitempty"`
	// DurationExchanges is the duration of a boosting effect, counting the
	// exchange it was taken in.
	DurationExchanges int `json:"duration_exchanges,omitempty"`
	// RemovesResidual marks a dose that can clear a meridian residual injury.
	// The first version provides no such pill in combat, and the design is
	// explicit that an ordinary restoring pill never clears one.
	RemovesResidual bool `json:"removes_residual,omitempty"`
	// SupportStrength is the auxiliary strength a cultivation-support dose
	// provides while its published duration runs. §6.2 prices 普通辅助 at 1.20
	// and 强辅助 at 1.40, so the value is one of those two enums and never a
	// free number the story chose.
	SupportStrength SupportResource `json:"support_strength,omitempty"`
	// SupportDays is how many real days one dose supports. §6.2 line 232
	// requires the quantity and effective duration to live on the item data, so
	// the rate really drops back when the stock runs out rather than a single
	// pill granting a permanent multiplier.
	SupportDays float64 `json:"support_days,omitempty"`
}

// RestoreAmount returns the absolute health or mana one dose restores.
//
// The amount is bound to the pill's registered stage, not to the user's, and it
// is clamped to the user's own maximum. This is what stops a low-grade pill
// from silently becoming a high-grade one for a stronger character.
func (p PillDef) RestoreAmount(user Attributes) int {
	if p.Purpose != PillRestoreHealth && p.Purpose != PillRestoreMana {
		return 0
	}
	fraction, ok := pillPoolFraction[p.Grade]
	if !ok {
		return 0
	}
	bare := BaseAttributes(p.RegisteredStage)
	var registeredMax, userMax int
	if p.Purpose == PillRestoreHealth {
		registeredMax, userMax = bare.HealthMax, user.HealthMax
	} else {
		registeredMax, userMax = bare.ManaMax, user.ManaMax
	}
	amount := int(roundHalfUp(fraction * float64(registeredMax)))
	if amount > userMax {
		amount = userMax
	}
	return amount
}

// AttackBonus returns the temporary attack bonus of a boosting pill, and
// whether this pill is one.
func (p PillDef) AttackBonus() (float64, bool) {
	if p.Purpose != PillBoostAttack {
		return 0, false
	}
	bonus, ok := pillAttackBonus[p.Grade]
	return bonus, ok
}

// SupportStrengthOf returns the auxiliary strength a cultivation-support dose
// provides, and whether this pill is one.
//
// It is a query on the definition rather than on the caller's intent, which is
// what §6.2 line 232 means by "此表确定辅助强度，具体丹药数量与有效持续时间在物品
// 数据中定义": a story that writes 强辅助 beside a registered 普通 dose is paid
// the registered strength.
func (p PillDef) SupportStrengthOf() (SupportResource, bool) {
	if p.Purpose != PillSupportCultivation {
		return SupportNone, false
	}
	switch p.SupportStrength {
	case SupportNormal, SupportStrong:
		return p.SupportStrength, true
	}
	return SupportNone, false
}

// SupportDuration returns how many real days one cultivation-support dose lasts.
func (p PillDef) SupportDuration() float64 {
	if p.Purpose != PillSupportCultivation || p.SupportDays <= 0 {
		return 0
	}
	return p.SupportDays
}

// ApplyPill applies one dose to a user's pools.
//
// A restoring dose never exceeds the user's maximum, and it never clears a
// meridian residual: the design requires that a mana-restoring pill does not
// undo a secret technique's injury.
func (p PillDef) ApplyPill(user Attributes, burden BurdenState) (Attributes, BurdenState) {
	switch p.Purpose {
	case PillRestoreHealth:
		user.Health = minInt(user.HealthMax, user.Health+p.RestoreAmount(user))
	case PillRestoreMana:
		user.Mana = minInt(user.ManaMax, user.Mana+p.RestoreAmount(user))
	case PillBoostAttack:
		// The boost is an attribute modifier, not a pool change; the caller
		// registers it through the modifier aggregation so it obeys the global
		// cap and the same-type-strongest rule.
	}
	return user, burden
}

// FormationDef is a registered formation.
type FormationDef struct {
	ID      string     `json:"id"`
	NameKey string     `json:"name_key,omitempty"`
	Grade   ItemGrade  `json:"grade,omitempty"`
	Rarity  ItemRarity `json:"rarity,omitempty"`
	// AttackBonus and DefenseBonus are the bonuses this formation provides.
	// A first-version formation provides one main bonus; a definition that sets
	// both is rejected by Validate rather than silently granting both.
	AttackBonus  float64 `json:"attack_bonus,omitempty"`
	DefenseBonus float64 `json:"defense_bonus,omitempty"`
}

// formationAttackBonus is the published ceiling on a single-type attack
// formation's bonus, by grade.
var formationAttackBonus = map[ItemGrade]float64{
	GradeLower:  0.05,
	GradeMiddle: 0.08,
	GradeUpper:  0.12,
	GradeTop:    0.15,
}

// formationDefenseBonus is the published ceiling on a single-type protective
// formation's bonus, by grade.
var formationDefenseBonus = map[ItemGrade]float64{
	GradeLower:  0.05,
	GradeMiddle: 0.10,
	GradeUpper:  0.15,
	GradeTop:    0.20,
}

// FormationAttackBonus returns the published attack bonus of a single-type
// attack formation at the given grade.
//
// It is exported so the play-time reader prices a deployed formation from the
// same table Validate enforces, rather than carrying a second copy of the
// numbers that could drift away from it.
func FormationAttackBonus(grade ItemGrade) (float64, bool) {
	bonus, ok := formationAttackBonus[grade]
	return bonus, ok
}

// FormationDefenseBonus returns the published defence bonus of a single-type
// protective formation at the given grade.
func FormationDefenseBonus(grade ItemGrade) (float64, bool) {
	bonus, ok := formationDefenseBonus[grade]
	return bonus, ok
}

// Validate rejects a formation that exceeds its grade's published ceiling or
// that grants two main bonuses at once.
//
// This is the check the design requires before a generated definition is
// registered: an out-of-budget definition must be refused rather than
// registered and then hidden behind the global cap.
func (f FormationDef) Validate() error {
	attackCeiling, ok := formationAttackBonus[f.Grade]
	if !ok {
		return ErrUnsupportedGrade
	}
	defenseCeiling, ok := formationDefenseBonus[f.Grade]
	if !ok {
		return ErrUnsupportedGrade
	}
	if f.AttackBonus < 0 || f.DefenseBonus < 0 {
		return ErrNegativeFormationBonus
	}
	if f.AttackBonus > 0 && f.DefenseBonus > 0 {
		return ErrFormationTwoBonuses
	}
	if f.AttackBonus > attackCeiling {
		return ErrFormationAttackOverBudget
	}
	if f.DefenseBonus > defenseCeiling {
		return ErrFormationDefenseOverBudget
	}
	return nil
}

// StartManaCost is the mana spent to activate the formation: 10% of the
// operator's bare stage mana maximum, rounded up.
func (f FormationDef) StartManaCost(operator Cultivation) int {
	return int(ceilNonNegative(0.10 * float64(BaseAttributes(operator).ManaMax)))
}

// UpkeepManaCost is the mana spent each exchange to keep the formation running:
// 2% of the operator's bare stage mana maximum, rounded up.
func (f FormationDef) UpkeepManaCost(operator Cultivation) int {
	return int(ceilNonNegative(0.02 * float64(BaseAttributes(operator).ManaMax)))
}

// FormationState is an active formation.
type FormationState struct {
	Def FormationDef `json:"def"`
	// RemainingExchanges counts down as exchanges pass. Zero means the
	// formation has no fixed term and stays up for as long as its operator keeps
	// paying: §4.3 gives a formation an activation cost and a per-round upkeep,
	// not a duration, so a deployed 阵盘 is not something that expires on its
	// own. A positive value is a term the caller imposed, and it still counts
	// down as before.
	RemainingExchanges int `json:"remaining_exchanges,omitempty"`
	// Active is false once upkeep fails, the operator leaves, or the focus is
	// destroyed.
	Active bool `json:"active,omitempty"`
	// OperatorRealm is the realm of the operator, for the realm-tagging of any
	// effect it drives.
	OperatorRealm Realm `json:"operator_realm,omitempty"`
}

// Activate charges the formation's activation cost and reports whether the
// operator could pay it.
//
// §4.3 makes deployment a real spend rather than a label: 启动耗操作者阶段基准灵力10%.
// A formation whose operator cannot pay is not deployed at all — returning the
// pool untouched and false is what keeps an unaffordable deployment from being
// recorded as a success and then quietly contributing its bonus.
//
// The cost is read from the operator's bare stage mana, not from their current
// maximum, so a temporary boost cannot make a formation cheaper to raise.
func (f FormationState) Activate(operator Attributes, operatorStage Cultivation) (FormationState, Attributes, bool) {
	if f.Def.ID == "" {
		return f, operator, false
	}
	cost := f.Def.StartManaCost(operatorStage)
	if operator.Mana < cost {
		return f, operator, false
	}
	operator.Mana -= cost
	f.Active = true
	return f, operator, true
}

// Upkeep advances a formation by one exchange, deactivating it if the operator
// cannot pay the upkeep.
//
// The design requires that a formation stops working when maintenance fails,
// when the operator leaves the area, or when its focus is destroyed. Those are
// real conditions reported by the caller; this function handles the payment
// failure, which is the one that is purely arithmetic.
//
// A formation with no imposed term keeps running: only a caller that set a
// RemainingExchanges count can expire it by counting down. Treating the zero
// value as "no exchanges left" would end every formation in the exchange it was
// deployed, which is the opposite of what §4.3 describes.
func (f FormationState) Upkeep(operator Attributes, operatorStage Cultivation) (FormationState, Attributes) {
	if !f.Active {
		return f, operator
	}
	cost := f.Def.UpkeepManaCost(operatorStage)
	if operator.Mana < cost {
		f.Active = false
		return f, operator
	}
	operator.Mana -= cost
	if f.RemainingExchanges > 0 {
		f.RemainingExchanges--
		if f.RemainingExchanges <= 0 {
			f.Active = false
		}
	}
	return f, operator
}

// FormationModifiers turns an active formation into the attribute modifiers it
// contributes.
//
// An inactive formation contributes nothing, which is how leaving the area or
// losing maintenance stops the bonus without the caller having to remember to
// remove it.
func (f FormationState) FormationModifiers() []AttributeModifier {
	if !f.Active {
		return nil
	}
	var modifiers []AttributeModifier
	if f.Def.AttackBonus > 0 {
		modifiers = append(modifiers, AttributeModifier{
			ID:        "formation:" + f.Def.ID + ":attack",
			Kind:      SourceFormation,
			Attribute: AttrAttack,
			Value:     f.Def.AttackBonus,
		})
	}
	if f.Def.DefenseBonus > 0 {
		modifiers = append(modifiers, AttributeModifier{
			ID:        "formation:" + f.Def.ID + ":defense",
			Kind:      SourceFormation,
			Attribute: AttrDefense,
			Value:     f.Def.DefenseBonus,
		})
	}
	return modifiers
}

// DamageOverTime is a lasting damage effect, such as the burning variant.
type DamageOverTime struct {
	// PerTick is the damage each remaining tick deals. It is computed once,
	// from the direct damage that was already settled when the effect landed,
	// so it never re-draws a hit or re-applies a bonus.
	PerTick int `json:"per_tick,omitempty"`
	// RemainingTicks counts down as exchanges end.
	RemainingTicks int `json:"remaining_ticks,omitempty"`
	// SourceID identifies the casting, so a same-type effect is refreshed
	// rather than stacked.
	SourceID string `json:"source_id,omitempty"`
}

// NewBurn builds the burning variant's lasting effect from a settled direct
// hit.
//
// The published variant lowers the direct power from 1.20 to 1.00 and then
// deals 15% of the committed direct damage on each of the next two exchanges.
// The percentage is taken from the damage that already passed through shields,
// and no bonus is applied again.
func NewBurn(committedDirectDamage int, sourceID string) DamageOverTime {
	return DamageOverTime{
		PerTick:        int(roundHalfUp(0.15 * float64(committedDirectDamage))),
		RemainingTicks: 2,
		SourceID:       sourceID,
	}
}

// StackBurn applies a new burning effect to an existing one.
//
// Same-type effects do not accumulate into a third or fourth layer: the
// stronger unsettled effect is kept, and a fresh legal application refreshes
// the duration back to two ticks.
func StackBurn(existing DamageOverTime, incoming DamageOverTime) DamageOverTime {
	if existing.RemainingTicks <= 0 {
		return incoming
	}
	if incoming.PerTick >= existing.PerTick {
		return incoming
	}
	// The existing effect is stronger: keep its damage but refresh its term.
	existing.RemainingTicks = incoming.RemainingTicks
	return existing
}

// TickDamageOverTime advances a lasting effect by one exchange and returns the
// damage it deals and the surviving effect.
//
// A tick deals no damage once the effect has run out, so a caller cannot keep
// collecting from an expired effect.
func TickDamageOverTime(effect DamageOverTime) (int, DamageOverTime) {
	if effect.RemainingTicks <= 0 {
		return 0, DamageOverTime{}
	}
	damage := effect.PerTick
	effect.RemainingTicks--
	if effect.RemainingTicks <= 0 {
		return damage, DamageOverTime{}
	}
	return damage, effect
}

// SlowEffect is the delaying variant's agility reduction.
type SlowEffect struct {
	// AgilityPenalty is the fraction of the target's agility removed.
	AgilityPenalty float64 `json:"agility_penalty,omitempty"`
	// RemainingTargetActions counts down as the target completes primary
	// actions. The effect expires after the target's next primary action.
	RemainingTargetActions int    `json:"remaining_target_actions,omitempty"`
	SourceID               string `json:"source_id,omitempty"`
}

// NewSlow builds the delaying variant's effect.
//
// The published variant lowers power from 1.85 to 1.45, keeping the mana and
// burden cost unchanged, and reduces the target's agility by 15% until the
// target's next primary action completes. The design is explicit that this is
// not a hard control: it does not take away the target's action, and the
// already-drawn order of this exchange is not re-drawn.
func NewSlow(sourceID string) SlowEffect {
	return SlowEffect{
		AgilityPenalty:         0.15,
		RemainingTargetActions: 1,
		SourceID:               sourceID,
	}
}

// AgilityModifier returns the agility modifier this effect contributes while it
// is in force.
func (s SlowEffect) AgilityModifier() []AttributeModifier {
	if s.RemainingTargetActions <= 0 {
		return nil
	}
	return []AttributeModifier{{
		ID:        "slow:" + s.SourceID,
		Kind:      SourceEventReward,
		Attribute: AttrAgility,
		Value:     -s.AgilityPenalty,
	}}
}

// TickSlow advances the effect past one of the target's primary actions.
func (s SlowEffect) TickSlow() SlowEffect {
	if s.RemainingTargetActions <= 0 {
		return SlowEffect{}
	}
	s.RemainingTargetActions--
	if s.RemainingTargetActions <= 0 {
		return SlowEffect{}
	}
	return s
}

// BurnVariantPower and SlowVariantPower are the published variant powers. A
// variant replaces its base technique's power; the two variants are not both
// attached to one template.
const (
	BurnVariantPower = 1.00
	SlowVariantPower = 1.45
)

// ValidateSkillDefinition refuses a generated definition that exceeds the
// supported rules, before it can be registered.
//
// The design requires that a generated definition be checked for supported
// effects, budget and numeric range first: registering first and relying on the
// global cap afterwards would let an out-of-budget technique exist and merely
// be capped, which the design explicitly rejects.
func ValidateSkillDefinition(skill SkillDef) error {
	if skill.ApplicableStage != "" {
		if _, ok := ParseStageKey(skill.ApplicableStage); !ok {
			return &UnknownStageKeyError{Key: skill.ApplicableStage}
		}
	}
	if err := skill.DefinitionDetails.Validate(); err != nil {
		return err
	}
	if skill.ID == "" {
		return ErrSkillMissingID
	}
	if skill.Power < 0 {
		return ErrSkillNegativePower
	}
	if skill.ManaFraction < 0 || skill.ManaFraction > 1 {
		return ErrSkillManaFractionOutOfRange
	}
	if skill.Burden < 0 || skill.Burden > 100 {
		return ErrSkillBurdenOutOfRange
	}
	if skill.IgnoreDefense < 0 || skill.IgnoreDefense > 0.20 {
		return ErrSkillIgnoreDefenseOverCap
	}
	if skill.Targets < 1 {
		return ErrSkillNoTargets
	}
	if skill.ExecutionActions < 1 || skill.ExecutionActions > 2 {
		return ErrSkillUnsupportedPreparation
	}
	if skill.LeavesResidual && skill.ResidualBurdenFloor <= 0 {
		return ErrSkillResidualWithoutFloor
	}
	if skill.LeavesResidual && skill.SafeRecoveryMinutes <= 0 {
		return ErrSkillResidualWithoutRecovery
	}
	// A shield may not exceed the 35% single-target cap by construction.
	if skill.ShieldBaseHealthFraction < 0 || skill.ShieldBaseHealthFraction > 0.35 {
		return ErrSkillShieldOverCap
	}
	if math.IsNaN(skill.Power) || math.IsInf(skill.Power, 0) {
		return ErrSkillNonFiniteValue
	}
	return nil
}
