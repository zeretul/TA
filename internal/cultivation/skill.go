package cultivation

import "math"

// This file implements the technique definitions and casting rules of
// 《自制-技能与施展》V0.2 §3–§5.
//
// The design is explicit that a character's learned techniques are fixed,
// versioned facts: the same technique keeps the same power, cost, burden and
// target count across battles, restarts and branches, and the narrative may not
// re-invent numbers for dramatic convenience. The definitions here are
// therefore data, and the rules below are the only place a cost is computed.

// SkillDef is one registered technique. Its fields mirror the minimum
// definition the design requires.
//
// The JSON tags exist because a battle persists the technique a combatant is
// mid-way through performing. That has to be the whole definition rather than
// only its ID: a resumed battle completes the technique at the numbers the
// character actually began casting at, and re-resolving it from the registry
// would silently apply a later balance revision to a cast already paid for.
type SkillDef struct {
	// RequiredManualID is a held complete source, not mastery or a free unlock.
	RequiredManualID string `json:"required_manual_id,omitempty"`
	// ApplicableStage gates both study and casting; empty preserves legacy skills.
	ApplicableStage string `json:"applicable_stage,omitempty"`
	DefinitionDetails
	// ID is the stable identity of the definition.
	ID string `json:"id"`
	// Version identifies the definition revision. A balance change creates a
	// new version and formally replaces the character's known version; it does
	// not silently rewrite the old one.
	Version int `json:"version"`
	// NameKey is a localisation key, so the interface supplies the wording
	// rather than the rules layer.
	NameKey string `json:"name_key,omitempty"`
	// Name is the wording for a definition whose text is its own. A generated
	// technique has no localisation key — its name came from the story — so the
	// interface falls back to this. A published definition leaves it empty and
	// keeps using NameKey.
	Name string `json:"name,omitempty"`
	// Power is the damage multiplier.
	Power float64 `json:"power,omitempty"`
	// ManaFraction is the fraction of the caster's bare stage mana maximum the
	// technique costs. It is a fixed coefficient, so the absolute cost follows
	// the caster's legal stage: the same technique costs more at a higher
	// layer without the definition changing.
	ManaFraction float64 `json:"mana_fraction,omitempty"`
	// Burden is the meridian burden this technique adds when performed.
	Burden int `json:"burden,omitempty"`
	// Targets is the maximum number of reachable enemies affected.
	Targets int `json:"targets,omitempty"`
	// HitOffset is the additive hit-chance correction, e.g. −0.05.
	HitOffset float64 `json:"hit_offset,omitempty"`
	// ExecutionActions is 1 for a technique completed this exchange, 2 for one
	// that prepares first and completes on the next.
	ExecutionActions int `json:"execution_actions,omitempty"`
	// IgnoreDefense is the fraction of the target's aggregated defense this
	// technique legally bypasses, capped at 20%.
	IgnoreDefense float64 `json:"ignore_defense,omitempty"`
	// RequiresFocus marks a technique that needs its implement actually in
	// hand. An implement's own operating cost is then used rather than the
	// ordinary-attack name making a high-grade treasure free to invoke.
	RequiresFocus bool `json:"requires_focus,omitempty"`
	// Element is the damage element, used for sourced advantage. It never
	// grants an advantage on its own.
	Element string `json:"element,omitempty"`
	// ShieldBaseHealthFraction is the shield size as a fraction of the caster's
	// bare stage health maximum, for a shielding technique.
	ShieldBaseHealthFraction float64 `json:"shield_base_health_fraction,omitempty"`
	// DurationExchanges is how many exchanges a lasting effect survives,
	// counting the exchange it was cast in.
	DurationExchanges int `json:"duration_exchanges,omitempty"`
	// ResidualBurdenFloor is the burden below which recovery cannot go while a
	// residual injury is present.
	ResidualBurdenFloor int `json:"residual_burden_floor,omitempty"`
	// LeavesResidual marks a technique that leaves a meridian residual injury.
	LeavesResidual bool `json:"leaves_residual,omitempty"`
	// SafeRecoveryMinutes is the continuous safe regulation this technique's
	// residual requires before it clears.
	SafeRecoveryMinutes float64 `json:"safe_recovery_minutes,omitempty"`
	// RecoveryMinHealth and RecoveryMinMana are the real body conditions that
	// must hold before that regulation may begin.
	RecoveryMinHealth float64 `json:"recovery_min_health,omitempty"`
	RecoveryMinMana   float64 `json:"recovery_min_mana,omitempty"`
	// VariantBase names the alternative form this definition publishes, if any.
	// It lives on the definition rather than being derived from the ID so a
	// generated technique inherits the form of the template it was built from:
	// keying the forms off the base technique's ID would silently strip a
	// generated fire technique of the burning form its template publishes.
	VariantBase CombatVariant `json:"variant_base,omitempty"`
}

// IsPaid reports whether this technique consumes mana. A zero-mana technique is
// a real ordinary attack, not a free spell.
func (s SkillDef) IsPaid() bool { return s.ManaFraction > 0 }

// IsPrepared reports whether the technique takes more than one exchange.
func (s SkillDef) IsPrepared() bool { return s.ExecutionActions > 1 }

// CanCast reports whether the character may perform this technique at all.
//
// A mortal is refused every technique, including a zero-cost one: the design
// requires that zero mana not become free casting merely because a cost formula
// rounds to zero.
//
// The ordinary attack is not a technique, so the gate does not reach it.
// OrdinaryAttackSkillID's contract is that a mortal, who may cast nothing else,
// must still be able to fight; refusing the plain swing left an uninitiated
// protagonist with no legal action at all. It costs no mana, so exempting it
// cannot become the free casting the gate exists to stop.
func (s SkillDef) CanCast(c Cultivation) bool {
	if IsOrdinaryAttack(s) {
		return baseHealth(c) > 0
	}
	return CanUseTechniques(c) && (s.ApplicableStage == "" || stageAtLeast(c, s.ApplicableStage))
}

// ManaCost returns the whole-number mana this technique costs the given caster.
//
// The cost is the fixed coefficient times the caster's bare stage mana maximum,
// rounded up. It deliberately does not depend on current mana, so the displayed
// cost is stable, and it deliberately does not grow with a raised maximum from
// equipment: the design says extra capacity must not raise a technique's cost.
func (s SkillDef) ManaCost(c Cultivation) int {
	if s.ManaFraction <= 0 {
		return 0
	}
	bare := BaseAttributes(c).ManaMax
	return int(ceilNonNegative(s.ManaFraction * float64(bare)))
}

// PaidManaFloor is the minimum mana any paid technique may cost after
// reductions: 2% of the caster's bare stage mana maximum, rounded up. It stops
// stacked cost reductions from making a real technique free.
func PaidManaFloor(c Cultivation) int {
	bare := BaseAttributes(c).ManaMax
	if bare <= 0 {
		return 0
	}
	return int(ceilNonNegative(0.02 * float64(bare)))
}

// ReducedManaCost applies a total cost reduction to a technique's cost.
//
// The reduction itself is capped at 20%, and the result may not fall below the
// paid floor, so a real technique always costs something real.
func (s SkillDef) ReducedManaCost(c Cultivation, reduction float64) int {
	base := s.ManaCost(c)
	if base == 0 {
		return 0
	}
	reduction = clampFloat(reduction, 0, 0.20)
	reduced := int(ceilNonNegative(float64(base) * (1 - reduction)))
	if floor := PaidManaFloor(c); reduced < floor {
		return floor
	}
	return reduced
}

// BurdenState is the meridian burden of one combatant.
type BurdenState struct {
	// Current is the accumulated burden, 0–100.
	Current int `json:"current"`
	// Residual is the floor recovery may not cross, set while a meridian
	// residual injury is present.
	Residual int `json:"residual,omitempty"`
}

// BurdenCapacity returns the burden a character can carry, from their real
// health state.
//
// The published thresholds are 100 while healthy, 75 below 60% health and 50
// below 30%. A real seal or injury is a separate, explicitly defined effect; it
// must not be improvised by the narrative as a lower capacity here.
func BurdenCapacity(health, healthMax int) int {
	if healthMax <= 0 {
		return 50
	}
	fraction := float64(health) / float64(healthMax)
	switch {
	case fraction < 0.30:
		return 50
	case fraction < 0.60:
		return 75
	}
	return 100
}

// CastCheck is the outcome of asking whether a technique may be performed.
type CastCheck struct {
	// Allowed reports whether every precondition holds.
	Allowed bool `json:"allowed"`
	// ReasonKey names the single blocking reason, as a localisation key, so the
	// interface can explain the real cause rather than greying a card out
	// silently.
	ReasonKey string `json:"reason_key,omitempty"`
	// ManaCost and BurdenCost are the amounts that would be spent.
	ManaCost   int `json:"mana_cost,omitempty"`
	BurdenCost int `json:"burden_cost,omitempty"`
}

// OrdinaryAttackSkillID is the zero-cost attack every combatant always has.
//
// It is deliberately not a learned ability. The design lists it separately from
// 已学技能 because it requires no study, and a character whose learned set is
// empty — or a mortal, who may cast nothing else — must still be able to fight.
const OrdinaryAttackSkillID = "strike"

// IsOrdinaryAttack reports whether a technique is the always-available plain
// attack rather than something the character had to learn.
func IsOrdinaryAttack(skill SkillDef) bool { return skill.ID == OrdinaryAttackSkillID }

// Reason keys for a refused cast. They are stable identifiers, not prose.
const (
	ReasonSkillStageTooLow    = "reason.skill_stage_too_low"
	ReasonMortalNoTechniques  = "reason.mortal_no_techniques"
	ReasonUnknownSkill        = "reason.unknown_skill"
	ReasonInsufficientMana    = "reason.insufficient_mana"
	ReasonBurdenCapacity      = "reason.burden_capacity"
	ReasonMeridianUnrecovered = "reason.meridian_unrecovered"
	ReasonImplementRequired   = "reason.implement_required"
	// ReasonTooManyTargets refuses a declaration that names more combatants than
	// the technique reaches. §9 settles reach from the actual choice and the cap,
	// so a declaration past the cap is not a technique that reaches further — it
	// is one the rules cannot carry out, and refusing it costs nothing.
	ReasonTooManyTargets = "reason.too_many_targets"
)

// CanPerform checks every precondition for performing a technique.
//
// The burden rule is the one the design calls out specifically: a heavy
// technique may be unperformable even with full mana, because the current
// burden plus this technique's burden would exceed what the body can carry.
// Being unable to perform one technique does not disable legal actions that add
// no burden, which is why this function refuses only the technique asked about.
func CanPerform(skill SkillDef, c Cultivation, attrs Attributes, burden BurdenState, manaReduction float64, hasRequiredImplement bool) CastCheck {
	return CanPerformWithStrain(skill, c, attrs, burden, manaReduction, 0, hasRequiredImplement)
}

func canPerformWithBurden(skill SkillDef, c Cultivation, attrs Attributes, burden BurdenState, manaReduction float64, hasRequiredImplement bool, burdenCost int) CastCheck {
	if !skill.CanCast(c) {
		if CanUseTechniques(c) {
			return CastCheck{ReasonKey: ReasonSkillStageTooLow}
		}
		return CastCheck{ReasonKey: ReasonMortalNoTechniques}
	}
	if skill.ID == "" {
		return CastCheck{ReasonKey: ReasonUnknownSkill}
	}
	if skill.RequiresFocus && !hasRequiredImplement {
		return CastCheck{ReasonKey: ReasonImplementRequired}
	}

	manaCost := skill.ReducedManaCost(c, manaReduction)

	// A residual injury blocks a technique whose burden would push past what
	// the recovering meridians can take, even when raw capacity would allow it.
	if burden.Residual > 0 && burden.Current+burdenCost > burden.Residual && burdenCost > 0 {
		return CastCheck{
			ReasonKey:  ReasonMeridianUnrecovered,
			ManaCost:   manaCost,
			BurdenCost: burdenCost,
		}
	}

	capacity := BurdenCapacity(attrs.Health, attrs.HealthMax)
	// Only a technique that adds burden can exceed the capacity. The plain attack
	// adds none, and §4.1 makes the ordinary-attack round the way a survivor sheds
	// burden: refusing it whenever the current burden already sits above a wounded
	// body's capacity would leave a character carrying a residual injury unable to
	// act at all, and unable to reach the recovery that would free them.
	if burdenCost > 0 && burden.Current+burdenCost > capacity {
		return CastCheck{
			ReasonKey:  ReasonBurdenCapacity,
			ManaCost:   manaCost,
			BurdenCost: burdenCost,
		}
	}
	if attrs.Mana < manaCost {
		return CastCheck{
			ReasonKey:  ReasonInsufficientMana,
			ManaCost:   manaCost,
			BurdenCost: burdenCost,
		}
	}
	return CastCheck{Allowed: true, ManaCost: manaCost, BurdenCost: burdenCost}
}

// ApplyCast applies a successful cast to the caster's pools and burden.
//
// It is separate from the check so a caller cannot spend resources on a cast
// that was refused: a failed check must not consume mana or add burden.
func ApplyCast(check CastCheck, attrs Attributes, burden BurdenState) (Attributes, BurdenState) {
	if !check.Allowed {
		return attrs, burden
	}
	attrs.Mana = maxInt(0, attrs.Mana-check.ManaCost)
	burden.Current += check.BurdenCost
	if burden.Current > 100 {
		burden.Current = 100
	}
	return attrs, burden
}

// RecoverBurdenPerExchange returns the burden relieved at the end of one
// exchange.
//
// Every surviving cultivator sheds 6 points per exchange, including an exchange
// spent on an ordinary attack, with no separate "rest" action required. The
// relief never crosses a residual floor, so a secret technique's injury cannot
// be waited off inside a battle.
func RecoverBurdenPerExchange(burden BurdenState, canRecover bool) BurdenState {
	if !canRecover {
		return burden
	}
	next := burden.Current - 6
	if floor := burden.Residual; next < floor {
		next = floor
	}
	if next < 0 {
		next = 0
	}
	burden.Current = next
	return burden
}

// RecoverBurdenOffBattle returns the burden relieved by a real off-battle
// interval.
//
// The published rate is 6 points per real minute of safe regulation. It is
// deliberately driven by elapsed real time rather than by opening or closing
// the window: the design forbids narrating that minutes passed while the
// back-end does not advance them.
func RecoverBurdenOffBattle(burden BurdenState, minutes float64) BurdenState {
	if minutes <= 0 {
		return burden
	}
	next := burden.Current - int(math.Floor(6*minutes))
	if floor := burden.Residual; next < floor {
		next = floor
	}
	if next < 0 {
		next = 0
	}
	burden.Current = next
	return burden
}

// CanClearResidual reports whether the conditions for clearing a residual
// injury are met.
//
// The design lists them together: the character must be out of real danger for
// the technique's full required interval, at at least 60% health and 20% mana,
// with no real injury or seal blocking regulation. Splitting the same interval
// into several requests must not bypass the requirement, so the caller passes
// the continuous interval it actually observed.
func CanClearResidual(skill SkillDef, continuousSafeMinutes float64, attrs Attributes, blocked bool) bool {
	if !skill.LeavesResidual {
		return false
	}
	if blocked || continuousSafeMinutes < skill.SafeRecoveryMinutes {
		return false
	}
	if attrs.HealthMax <= 0 || float64(attrs.Health) < skill.RecoveryMinHealth*float64(attrs.HealthMax) {
		return false
	}
	if attrs.ManaMax <= 0 || float64(attrs.Mana) < skill.RecoveryMinMana*float64(attrs.ManaMax) {
		return false
	}
	return true
}

// ClearResidual applies a completed safe regulation.
//
// Clearing the residual also relieves ordinary burden to zero, and that relief
// IS the effect of the regulation interval: the same minutes are not counted a
// second time as ordinary per-minute recovery. Health and mana are not refilled.
func ClearResidual(burden BurdenState) BurdenState {
	burden.Residual = 0
	burden.Current = 0
	return burden
}

// Shield is an active damage-absorbing effect.
type Shield struct {
	// Amount is the remaining absorption.
	Amount int `json:"amount"`
	// RemainingExchanges counts down as exchanges pass, including the one the
	// shield was cast in.
	RemainingExchanges int `json:"remaining_exchanges,omitempty"`
	// SourceID identifies the casting, so a same-type shield can be replaced
	// rather than accumulated.
	SourceID string `json:"source_id,omitempty"`
	// CasterBaseHealthFraction is the fraction of the caster's bare stage
	// health maximum the shield was built from.
	CasterBaseHealthFraction float64 `json:"caster_base_health_fraction,omitempty"`
}

// NewShield builds a shield for a casting.
//
// The size is a fraction of the caster's bare stage health maximum, and the
// total of all effective shields on one target is capped at 35% of that
// target's own bare health maximum — a helper's shield is scaled to the
// caster, then limited by the recipient.
func NewShield(skill SkillDef, caster Cultivation, target Cultivation, sourceID string) Shield {
	amount := int(roundHalfUp(skill.ShieldBaseHealthFraction * float64(BaseAttributes(caster).HealthMax)))
	if cap := ShieldCap(target); amount > cap {
		amount = cap
	}
	return Shield{
		Amount:                   amount,
		RemainingExchanges:       skill.DurationExchanges,
		SourceID:                 sourceID,
		CasterBaseHealthFraction: skill.ShieldBaseHealthFraction,
	}
}

// ShieldCap is the most shielding one target may hold at once: 35% of its own
// bare stage health maximum.
func ShieldCap(target Cultivation) int {
	return int(roundHalfUp(0.35 * float64(BaseAttributes(target).HealthMax)))
}

// StackShield adds a newly cast shield to a target's existing shields.
//
// A same-type shield does not accumulate: the stronger remaining one applies,
// and a fresh casting replaces a damaged same-type shield rather than adding to
// it. Different sources may coexist up to the target's cap, with the weakest
// trimmed so the cap holds.
func StackShield(existing Shield, incoming Shield, target Cultivation) Shield {
	if existing.Amount <= 0 || existing.SourceID == incoming.SourceID {
		// Replacing: the new shield takes effect at its own size.
		if incoming.Amount > ShieldCap(target) {
			incoming.Amount = ShieldCap(target)
		}
		return incoming
	}
	// Different source: the stronger of the two same-type effects applies
	// rather than their sum, then the target cap is enforced.
	if incoming.Amount <= existing.Amount {
		return existing
	}
	if cap := ShieldCap(target); incoming.Amount > cap {
		incoming.Amount = cap
	}
	return incoming
}

// TickShield advances a shield by one exchange, expiring it at zero.
func TickShield(s Shield) Shield {
	s.RemainingExchanges--
	if s.RemainingExchanges <= 0 {
		return Shield{}
	}
	return s
}

// ExpireShieldsAfter converts an off-battle interval into remaining shield
// exchanges.
//
// A lasting effect keeps its real remaining duration across a battle boundary;
// it is not refreshed by leaving and re-entering. The design fixes the exchange
// at 20 real seconds and requires the remaining term to be computed from the
// time that actually passed.
func ExpireShieldsAfter(s Shield, seconds float64) Shield {
	if seconds <= 0 || s.RemainingExchanges <= 0 {
		return s
	}
	elapsed := int(math.Floor(seconds / SecondsPerExchange))
	s.RemainingExchanges -= elapsed
	if s.RemainingExchanges <= 0 {
		return Shield{}
	}
	return s
}

// skillTable is the published demonstration template set of §4.2 and the
// numeric rules §5.
//
// The design is explicit that these names are self-authored demonstrations, not
// verified canon abilities, and that a world pack may generate different names,
// elements and presentations from these templates. A world pack must not
// therefore treat this table as the set of abilities everyone has learned:
// registering a definition and having learned it are different facts.
var skillTable = append([]SkillDef{
	{
		ID: "strike", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, Version: 1, NameKey: "skill.strike.name",
		Power: 1.00, ManaFraction: 0, Burden: 0, Targets: 1,
		HitOffset: 0, ExecutionActions: 1,
	},
	{
		ID: "fire_bolt", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("white")}, Version: 1, NameKey: "skill.fire_bolt.name",
		Power: 1.20, ManaFraction: 0.05, Burden: 6, Targets: 1,
		HitOffset: 0, ExecutionActions: 1, Element: "fire",
		VariantBase: VariantBurn,
	},
	{
		ID: "condensed_blade", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, NameKey: "skill.condensed_blade.name",
		Power: 1.60, ManaFraction: 0.12, Burden: 22, Targets: 1,
		HitOffset: 0, ExecutionActions: 1,
	},
	{
		ID: "heavy_thunder", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, Version: 1, NameKey: "skill.heavy_thunder.name",
		Power: 1.85, ManaFraction: 0.22, Burden: 42, Targets: 1,
		HitOffset: 0, ExecutionActions: 1, Element: "thunder",
		VariantBase: VariantSlow,
	},
	{
		ID: "flame_wave", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("blue")}, Version: 1, NameKey: "skill.flame_wave.name",
		Power: 0.75, ManaFraction: 0.22, Burden: 30, Targets: 3,
		HitOffset: 0, ExecutionActions: 1, Element: "fire",
	},
	{
		ID: "shield", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("green")}, Version: 1, NameKey: "skill.shield.name",
		Power: 0, ManaFraction: 0.10, Burden: 18, Targets: 1,
		HitOffset: 0, ExecutionActions: 1,
		ShieldBaseHealthFraction: 0.20, DurationExchanges: 2,
	},
	{
		// 冲脉秘剑 is the runtime binding of the catalogue's skill.meridian_sword.
		// Its body requirement is held_usable_blade_with_real_equipment_provenance,
		// so the blade has to be in hand rather than merely described.
		ID: "meridian_secret_blade", DefinitionDetails: DefinitionDetails{QualityGrade: QualityGrade("purple")}, Version: 1, NameKey: "skill.meridian_secret_blade.name",
		Power: 3.20, ManaFraction: 0.40, Burden: 85, Targets: 1,
		HitOffset: -0.05, ExecutionActions: 2,
		RequiresFocus:  true,
		IgnoreDefense:  0.20,
		LeavesResidual: true, ResidualBurdenFloor: 60,
		SafeRecoveryMinutes: 30, RecoveryMinHealth: 0.60, RecoveryMinMana: 0.20,
	},
}, highRealmSkills...)

// SkillTable returns a copy of the built-in demonstration technique set, keyed
// by ID.
//
// A copy is returned so a caller cannot mutate the shared definitions: the
// design requires a technique's numbers to stay fixed once registered.
func SkillTable() map[string]SkillDef {
	table := make(map[string]SkillDef, len(skillTable))
	for _, skill := range skillTable {
		table[skill.ID] = skill
	}
	return table
}

// SkillByID looks up one built-in definition.
func SkillByID(id string) (SkillDef, bool) {
	for _, skill := range skillTable {
		if skill.ID == id {
			return skill, true
		}
	}
	return SkillDef{}, false
}
