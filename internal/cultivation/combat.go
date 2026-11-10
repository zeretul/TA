package cultivation

import "math"

// This file implements the combat formulas of 《自制-修仙数值规则》V0.3 §4 and
// the derived escape and rest actions of §5.
//
// Every random draw is passed in by the caller rather than taken here. The
// design requires that initiative, hit and damage variance be bound to the
// request identity, so a retry, a refresh or a reopened window must re-use the
// same draw instead of re-rolling. Taking the draw as an argument makes that
// property structural rather than a convention a caller could forget.

// SecondsPerExchange is the real time one combat exchange represents. It is the
// unit that converts an off-battle interval into remaining effect durations.
const SecondsPerExchange = 20

// CombatRolls are the reproducible draws for one exchange between one attacker
// and one target. Each is a uniform value in [0,1) from the caller's seeded
// stream.
type CombatRolls struct {
	// Initiative is drawn once per combatant per exchange.
	Initiative float64
	// Hit and Damage are drawn per target, because a multi-target technique
	// resolves each target separately.
	Hit    float64
	Damage float64
}

// HitChance returns the probability that an attack lands, before the draw.
//
// The base rate is 80% plus the technique's own offset, then the agility
// difference contributes at most ±25 points. The result is clamped to the
// published 15%–97% band, so no combination of agility and technique modifiers
// can make an attack certain or impossible.
func HitChance(attacker, target Attributes, hitOffset float64) float64 {
	chance := 0.80 + hitOffset
	chance += 0.25 * agilityTerm(attacker.Agility, target.Agility)
	return clampFloat(chance, 0.15, 0.97)
}

// agilityTerm is the symmetric difference term used by both hit chance and
// escape: (a − b) / (a + b), and 0 when both are zero so an unperceiving or
// unmoving pair does not produce a division by zero.
func agilityTerm(attackerValue, targetValue int) float64 {
	sum := attackerValue + targetValue
	if sum == 0 {
		return 0
	}
	return float64(attackerValue-targetValue) / float64(sum)
}

// Initiative returns this combatant's action-priority value for the exchange.
// The draw is a uniform value in [0,1) mapped onto the published 0.70–1.30
// band, so a faster combatant usually acts first without the order being fixed.
func Initiative(agility int, draw float64) float64 {
	return float64(agility) * (0.70 + 0.60*clampFloat(draw, 0, 1))
}

// DamageContext carries the sourced conditions of one attack. Every field is
// a real, already-determined condition; none of them may be invented by the
// narrative at resolution time.
type DamageContext struct {
	// DefensePierce is the paid weapon's absolute rated penetration. It shares
	// the existing 20% aggregate ceiling with the technique's own ignore.
	DefensePierce int
	// CounterModifier is the elemental/approach advantage actually in effect:
	// 1.15 favourable, 1.00 neutral, 0.85 unfavourable. Only one applies, and
	// the same elemental affinity cannot be counted again as an environment
	// bonus.
	CounterModifier float64
	// EnvironmentModifier is the sourced battlefield condition, bounded to
	// 0.80–1.20. Several conditions on the same battlefield still total within
	// that band.
	EnvironmentModifier float64
}

// NeutralDamageContext is the context of an attack with no sourced advantage
// or disadvantage, which is what the published damage ranges assume.
func NeutralDamageContext() DamageContext {
	return DamageContext{CounterModifier: 1.00, EnvironmentModifier: 1.00}
}

// normalize replaces an unset (zero) modifier with neutral, so a caller that
// fills only one of the two fields does not accidentally annihilate the attack.
func (d DamageContext) normalize() DamageContext {
	if d.CounterModifier <= 0 {
		d.CounterModifier = 1.00
	}
	if d.EnvironmentModifier <= 0 {
		d.EnvironmentModifier = 1.00
	}
	// The rules bound these from both sides; clamp rather than trust the caller.
	d.CounterModifier = clampFloat(d.CounterModifier, 0.85, 1.15)
	d.EnvironmentModifier = clampFloat(d.EnvironmentModifier, 0.80, 1.20)
	return d
}

// CrossRealmDamageScale returns the damage multiplier applied when the attacker
// stands below the target in the major-realm order.
//
// The published table is 0.35 for one major realm below and 0.10 for two or
// more. A small-stage difference within a realm deliberately gets no extra
// factor: it is already expressed by the real attack, defense and health.
func CrossRealmDamageScale(attackerRealm, targetRealm Realm) float64 {
	gap := RealmGap(attackerRealm, targetRealm)
	switch {
	case gap == -1:
		return 0.35
	case gap <= -2:
		return 0.10
	}
	// Equal, higher, or an unrecognised realm: no extra factor.
	return 1.00
}

// DamageVariance maps a draw in [0,1) onto the published 0.85–1.15 band.
func DamageVariance(draw float64) float64 {
	return 0.85 + 0.30*clampFloat(draw, 0, 1)
}

// AttackResolution is the settled outcome of one attack against one target.
type AttackResolution struct {
	// Hit reports whether the attack landed. A miss deals no damage at all and
	// attaches no on-hit effect.
	Hit bool `json:"hit"`
	// Damage is the whole-number damage after every multiplier. It is the
	// amount a shield absorbs before health is touched.
	Damage int `json:"damage,omitempty"`
	// ShieldAbsorbed is how much of Damage an existing shield took, and
	// HealthLost is the remainder. Their sum is Damage.
	ShieldAbsorbed int `json:"shield_absorbed,omitempty"`
	HealthLost     int `json:"health_lost,omitempty"`
}

// ResolveAttack settles one attack against one target.
//
// The formula is the published one:
//
//	base = attack × power × attack / (attack + effective defense)
//	damage = round(base × counter × environment × cross-realm × variance)
//
// The realms are passed explicitly rather than read off the attributes, because
// the cross-realm factor is a property of the two combatants' positions on the
// stage table, not of any number derived from them.
//
// The defense passed in must already be the fully aggregated effective defense
// with the technique's own legal defense-ignore applied — see IgnoreDefense.
// The same defensive value is not scaled a second time by the realm gap.
//
// A shield is consumed after the final damage is known, so shield absorption
// never re-enters the multiplier chain.
func ResolveAttack(attacker, target Attributes, attackerRealm, targetRealm Realm, skill SkillDef, context DamageContext, shield Shield, rolls CombatRolls) AttackResolution {
	context = context.normalize()
	chance := HitChance(attacker, target, skill.HitOffset)
	if rolls.Hit >= chance {
		return AttackResolution{Hit: false}
	}

	ignore := skill.IgnoreDefense
	if target.Defense > 0 {
		ignore += float64(maxInt(0, context.DefensePierce)) / float64(target.Defense)
	}
	effectiveDefense := IgnoreDefense(target.Defense, ignore)
	damage := BaseHitDamage(attacker.Attack, skill.Power, effectiveDefense)
	final := damage *
		context.CounterModifier *
		context.EnvironmentModifier *
		CrossRealmDamageScale(attackerRealm, targetRealm) *
		DamageVariance(rolls.Damage)

	// A landing attack deals at least nothing; the design does not force a
	// minimum of one point through a strong defense.
	total := int(roundHalfUp(math.Max(0, final)))

	absorbed := minInt(shield.Amount, total)
	return AttackResolution{
		Hit:            true,
		Damage:         total,
		ShieldAbsorbed: absorbed,
		HealthLost:     total - absorbed,
	}
}

// BaseHitDamage is the pre-modifier damage center value: attack × power ×
// attack / (attack + defense).
//
// Attack is required to be at least 1 and defense at least 0, so the ratio
// stays in (0,1] and a zero attack cannot produce a NaN.
func BaseHitDamage(attack int, power float64, defense int) float64 {
	if attack < 1 {
		attack = 1
	}
	if defense < 0 {
		defense = 0
	}
	return float64(attack) * power * float64(attack) / float64(attack+defense)
}

// IgnoreDefense applies a technique's legal defense-ignore to a fully
// aggregated defense value.
//
// The cap is 20%, and this is applied after any temporary defense debuff has
// already been folded into the aggregated defense. A caller must not apply the
// same armor-breaking reason again as an attack bonus.
func IgnoreDefense(defense int, ignore float64) int {
	if defense <= 0 {
		return 0
	}
	ignore = clampFloat(ignore, 0, 0.20)
	return int(roundHalfUp(float64(defense) * (1 - ignore)))
}

// EscapeChance returns the probability of successfully disengaging.
//
// The base is 50% plus a symmetric agility term and a symmetric perception
// term, then cover and adverse conditions shift it, and the whole is clamped
// to 5%–90%. Standing a major realm below halves it, and two or more realms
// below divides it by ten.
//
// A caller must first establish that a real way out exists and that the
// character is neither incapacitated nor restrained: the floor is not a licence
// to escape from a sealed room.
func EscapeChance(escaper, pursuer Attributes, escaperRealm, pursuerRealm Realm, coverBonus, adversePenalty float64) float64 {
	chance := 0.50
	chance += 0.50 * agilityTerm(escaper.Agility, pursuer.Agility)
	chance += 0.20 * agilityTerm(escaper.Perception, pursuer.Perception)
	chance += coverBonus
	chance -= adversePenalty

	gap := RealmGap(escaperRealm, pursuerRealm)
	switch {
	case gap == -1:
		chance *= 0.5
	case gap <= -2:
		chance *= 0.1
	}
	return clampFloat(chance, 0.05, 0.90)
}

// RestManaRecovery returns the mana restored by spending one primary action on
// quiet regulation during combat.
//
// The design is explicit that this recovers 5% of the real mana maximum only
// if the character took no health damage this exchange, and nothing otherwise.
// A shield absorbing the blow does not make it a wound, but it also does not
// make the exchange damage-free — the condition is about real health loss.
func RestManaRecovery(manaMax int, tookHealthDamage bool) int {
	if tookHealthDamage || manaMax <= 0 {
		return 0
	}
	return int(roundHalfUp(0.05 * float64(manaMax)))
}

// SourcedAttributes derives a character's attributes from a stage and applies
// the supplied real sources. The stage is returned alongside so a caller can
// pass it to the combat functions, which take realms explicitly.
func SourcedAttributes(c Cultivation, modifiers []AttributeModifier) (Cultivation, Attributes) {
	return c, AggregateAttributes(BaseAttributes(c), modifiers, c.rules)
}
