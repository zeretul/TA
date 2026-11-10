package cultivation

import (
	"math"
	"testing"
)

// The tests here cover the exchange loop that drives the combat rules of
// 《自制-合理战斗_研究与初步方案》V0.5 §6 and 《自制-技能与施展》V0.3 §4–§5.
//
// Before it existed, every function it calls was implemented and unit-tested and
// nothing during play ever reached them: a battle round had no way to spend mana,
// add burden, absorb a blow or attach a lasting effect. These tests assert the
// published numbers a played exchange produces.

// fixedDraws returns a DrawSource answering the same value for every purpose, so
// a test can pin the randomness the engine would otherwise take from the battle's
// identity.
func fixedDraws(value float64) DrawSource {
	return func(purpose, actorID, targetID string) float64 { return value }
}

// drawByPurpose returns a DrawSource answering per purpose, which is how a test
// forces a hit while pinning damage variance.
func drawByPurpose(values map[string]float64) DrawSource {
	return func(purpose, actorID, targetID string) float64 {
		if value, ok := values[purpose]; ok {
			return value
		}
		return 0.5
	}
}

// combatantAt builds a live combatant at a refining layer, with full pools.
func combatantAt(t *testing.T, id string, side CombatSide, layer int) Combatant {
	t.Helper()
	cultivation := Qi(layer)
	base := BaseAttributes(cultivation)
	return Combatant{
		ID:          id,
		Side:        side,
		Cultivation: cultivation,
		Attributes:  base,
	}
}

// twoWayBattle is the published baseline of §14.1: two bare same-tier
// cultivators, 200 health and 20 defense each.
func twoWayBattle(t *testing.T) CombatSession {
	t.Helper()
	return CombatSession{
		ID: "battle1",
		Combatants: []Combatant{
			combatantAt(t, "hero", SidePlayer, 6),
			combatantAt(t, "foe", SideEnemy, 6),
		},
		Round: 1,
	}
}

// withBladeInHand gives a combatant the focus 冲脉秘剑 requires.
//
// §9 makes the requirement a fact about real equipment rather than a claim in
// the declaration, so a fixture that wants to perform the secret blade has to
// say the blade is really held. A fixture that leaves it out is exercising the
// refusal.
func withBladeInHand(session CombatSession, ids ...string) CombatSession {
	for _, id := range ids {
		if combatant := session.CombatantByID(id); combatant != nil {
			combatant.HasImplement = true
		}
	}
	return session
}

// §14.1 fixes the baseline: a six-layer bare cultivator has 200 health, 160 mana,
// 40 attack and 20 defense, and a fire bolt lands for 32 centre damage.
func TestTheBaselineExchangeMatchesThePublishedCentre(t *testing.T) {
	session := twoWayBattle(t)
	skill := mustSkill("fire_bolt")
	// A draw of 0.5 is the middle of the variance band, so the damage is the
	// published centre of 32 before any rounding drift.
	draws := drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5})

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: skill, TargetID: "foe"},
	}, draws)

	step := findStep(t, outcome, "hero")
	if !step.Cast.Allowed {
		t.Fatalf("a six-layer cultivator with full mana must cast a fire bolt: %+v", step.Cast)
	}
	if step.Cast.ManaCost != 8 {
		t.Errorf("fire bolt mana = %d, want the published 8", step.Cast.ManaCost)
	}
	if step.Cast.BurdenCost != 0 {
		t.Errorf("fire bolt burden = %d, want 0 for a routine cast", step.Cast.BurdenCost)
	}
	if !step.Resolution.Hit {
		t.Fatal("the attack was forced to hit")
	}
	// 40 × 1.20 × 40/(40+20) = 32, and the variance draw of 0.5 is exactly 1.00.
	if step.Resolution.Damage != 32 {
		t.Errorf("damage = %d, want the published centre 32", step.Resolution.Damage)
	}
	foe := next.CombatantByID("foe")
	if foe.Attributes.Health != 168 {
		t.Errorf("target health = %d, want 168 (200 − 32)", foe.Attributes.Health)
	}
	hero := next.CombatantByID("hero")
	if hero.Attributes.Mana != 152 {
		t.Errorf("caster mana = %d, want 152 (160 − 8)", hero.Attributes.Mana)
	}
	if hero.Burden.Current != 0 {
		t.Errorf("burden = %d, want 0", hero.Burden.Current)
	}
}

// §14.1: a six-layer fire-bolt reserve of 160 mana at 8 per cast is 20 casts, and
// the light cast adds no burden.
func TestAFireBoltReserveIsSustainableForTwentyCasts(t *testing.T) {
	session := twoWayBattle(t)
	skill := mustSkill("fire_bolt")
	// A target that never dies, so the test measures the caster's own budget
	// rather than the fight ending.
	session.Combatants[1].Attributes.Health = 1 << 20
	session.Combatants[1].Attributes.HealthMax = 1 << 20

	casts := 0
	for casts < 25 {
		if session.Ended {
			break
		}
		next, outcome := ResolveExchange(session, map[string]CombatAction{
			"hero": {Kind: ActionTechnique, Skill: skill, TargetID: "foe"},
			// The foe does nothing that costs anything, so the caster's mana is
			// the only pool under test.
			"foe": {Kind: ActionRest},
		}, fixedDraws(0.5))
		session = next
		step := findStep(t, outcome, "hero")
		if !step.Cast.Allowed {
			break
		}
		casts++
	}
	if casts != 20 {
		t.Errorf("casts before the reserve ran out = %d, want the published 20", casts)
	}
}

// §4.2: a heavy technique can be blocked by burden rather than by mana, which is
// the property the design names specifically.
func TestContinuousHeavyCastingIsStoppedByBurdenNotMana(t *testing.T) {
	session := twoWayBattle(t)
	skill := mustSkill("heavy_thunder")
	session.Combatants[1].Attributes.Health = 1 << 20
	session.Combatants[1].Attributes.HealthMax = 1 << 20

	casts := 0
	for casts < 10 {
		if session.Ended {
			break
		}
		next, outcome := ResolveExchange(session, map[string]CombatAction{
			"hero": {Kind: ActionTechnique, Skill: skill, TargetID: "foe"},
			"foe":  {Kind: ActionRest},
		}, fixedDraws(0.5))
		session = next
		step := findStep(t, outcome, "hero")
		if !step.Cast.Allowed {
			// The third cast needs 72 + 42 burden, with mana still in reserve.
			if step.ReasonKey != ReasonBurdenCapacity {
				t.Fatalf("refusal reason = %q, want the burden capacity reason", step.ReasonKey)
			}
			hero := session.CombatantByID("hero")
			if hero.Attributes.Mana <= 0 {
				t.Fatal("mana should not be the binding constraint here")
			}
			break
		}
		casts++
	}
	if casts != 2 {
		t.Errorf("casts before burden stopped it = %d, want 2", casts)
	}
}

// §5: a shield absorbs the final damage before health is touched, and its term
// counts the exchange it was cast in.
func TestAShieldAbsorbsBeforeHealth(t *testing.T) {
	session := twoWayBattle(t)
	shieldSkill := mustSkill("shield")

	// The caster raises a shield on itself: 20% of 200 = 40.
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: shieldSkill, TargetID: "hero"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0.5))
	hero := next.CombatantByID("hero")
	if hero.Shield.Amount != 40 {
		t.Fatalf("shield = %d, want 40 (20%% of 200)", hero.Shield.Amount)
	}
	// 2轮护盾包括施展当轮，到下一轮结束解除: the casting round is the first of
	// the two, so one exchange remains after it.
	if hero.Shield.RemainingExchanges != 1 {
		t.Fatalf("shield term = %d after the casting exchange, want 1 remaining of the published 2",
			hero.Shield.RemainingExchanges)
	}
	step := findStep(t, outcome, "hero")
	if step.Cast.BurdenCost != 18 {
		t.Errorf("shield burden = %d, want the published 18", step.Cast.BurdenCost)
	}

	// The foe now strikes for 32, which the 40-point shield swallows whole.
	after, outcome := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("fire_bolt"), TargetID: "hero"},
	}, drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5}))
	step = findStep(t, outcome, "foe")
	if step.Resolution.ShieldAbsorbed != 32 || step.Resolution.HealthLost != 0 {
		t.Fatalf("absorption = %+v, want all 32 absorbed and no wound", step.Resolution)
	}
	hero = after.CombatantByID("hero")
	if hero.Attributes.Health != 200 {
		t.Errorf("health = %d, want the untouched 200; a shield absorbed the blow", hero.Attributes.Health)
	}
}

// §5: a shield expires at the end of its published term, not by being used.
func TestAShieldExpiresAtTheEndOfItsTerm(t *testing.T) {
	session := twoWayBattle(t)
	next, _ := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("shield"), TargetID: "hero"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0.5))
	if next.CombatantByID("hero").Shield.Amount == 0 {
		t.Fatal("the shield must be standing after the exchange it was cast in")
	}
	// A two-exchange shield includes the casting exchange and expires at the end
	// of the next one.
	after, _ := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0.5))
	if after.CombatantByID("hero").Shield.Amount != 0 {
		t.Errorf("shield = %d after its term, want it expired",
			after.CombatantByID("hero").Shield.Amount)
	}
}

// §5: the burning variant lowers the direct hit and keeps dealing 15% of the
// committed damage for two more exchanges.
func TestTheBurningVariantKeepsDealingDamage(t *testing.T) {
	session := twoWayBattle(t)
	draws := drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5})

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("fire_bolt"), Variant: VariantBurn, TargetID: "foe"},
	}, draws)
	step := findStep(t, outcome, "hero")
	if !step.BurnApplied {
		t.Fatal("a burning hit must attach the lasting effect")
	}
	// Power 1.00 rather than 1.20: 40 × 1.00 × 40/60 = 26.67, rounded to 27.
	if step.Resolution.Damage != 27 {
		t.Errorf("burning direct damage = %d, want 27 (power 1.00)", step.Resolution.Damage)
	}
	foe := next.CombatantByID("foe")
	healthAfterHit := foe.Attributes.Health
	if foe.Burn.RemainingTicks != 2 {
		t.Fatalf("burn ticks = %d, want the published 2", foe.Burn.RemainingTicks)
	}
	// 15% of the 27 committed is 4.05, rounded to 4.
	if foe.Burn.PerTick != 4 {
		t.Errorf("burn per tick = %d, want 4 (15%% of 27)", foe.Burn.PerTick)
	}
	// The effect does not tick in the exchange it landed in: §5 puts the two
	// residual damages in 后续两个回合, the two rounds after the hit.
	if foe.Attributes.Health != healthAfterHit {
		t.Errorf("health = %d in the exchange the burn landed, want the untouched %d",
			foe.Attributes.Health, healthAfterHit)
	}

	// Two more exchanges each tick the burn, and the effect then ends.
	after, _ := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionRest},
	}, draws)
	if got := after.CombatantByID("foe").Attributes.Health; got != healthAfterHit-4 {
		t.Errorf("health after one burn tick = %d, want %d", got, healthAfterHit-4)
	}
	last, _ := ResolveExchange(after, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionRest},
	}, draws)
	foe = last.CombatantByID("foe")
	if foe.Attributes.Health != healthAfterHit-8 {
		t.Errorf("health after two burn ticks = %d, want %d", foe.Attributes.Health, healthAfterHit-8)
	}
	if foe.Burn.RemainingTicks != 0 {
		t.Errorf("burn ticks = %d after its term, want it expired", foe.Burn.RemainingTicks)
	}
}

// §5: the slowing variant lowers the direct hit and removes 15% of the target's
// agility until its next primary action completes.
func TestTheSlowingVariantReducesAgilityForOneAction(t *testing.T) {
	session := twoWayBattle(t)
	draws := drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5})

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("heavy_thunder"), Variant: VariantSlow, TargetID: "foe"},
	}, draws)
	step := findStep(t, outcome, "hero")
	if !step.SlowApplied {
		t.Fatal("a slowing hit must attach the effect")
	}
	// Power 1.45 rather than 1.85: 40 × 1.45 × 40/60 = 38.67, rounded to 39.
	if step.Resolution.Damage != 39 {
		t.Errorf("slowing direct damage = %d, want 39 (power 1.45)", step.Resolution.Damage)
	}
	// The effect lasts until the target's next primary action, which is the
	// following exchange rather than the one it landed in.
	if next.CombatantByID("foe").Slow.RemainingTargetActions != 1 {
		t.Fatalf("slow = %+v after the exchange it landed in, want one action remaining",
			next.CombatantByID("foe").Slow)
	}

	// §5 says the effect reduces the target's agility by 15%, not merely that it
	// is recorded. A six-layer cultivator has 20 agility, so 20 × 0.85 = 17, and
	// the hit-chance term it feeds must move with it: 0.25 × (20−17)/(20+17).
	slowed := next.CombatantByID("foe")
	if slowed.Attributes.Agility != 17 {
		t.Fatalf("agility = %d while slowed, want 17 (20 × 0.85)",
			slowed.Attributes.Agility)
	}
	hero := next.CombatantByID("hero")
	if got, want := HitChance(hero.Attributes, slowed.Attributes, 0), 0.80+0.25*3.0/37.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("hit chance against the slowed target = %v, want %v", got, want)
	}

	after, _ := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionRest},
	}, draws)
	if after.CombatantByID("foe").Slow.RemainingTargetActions != 0 {
		t.Errorf("slow = %+v after the target acted, want it expired",
			after.CombatantByID("foe").Slow)
	}
	// Expiry has to give the agility back. The effect is carried through the
	// same aggregation the dose term uses, so losing it re-derives the numbers
	// rather than subtracting the penalty back out.
	if got := after.CombatantByID("foe").Attributes.Agility; got != 20 {
		t.Errorf("agility = %d after the slow expired, want the base 20", got)
	}
}

// §5: the slow lasts until the target's next primary action completes, so when
// the caster acts first the target spends it in the very exchange it landed in.
// That is where the effect has to matter — the target's own check is made at the
// reduced agility, and the slow is gone by the end of the exchange.
//
// The two hit chances are 0.80 (agility 20 against 20) and 0.7797 (agility 17
// against 20). A draw of 0.79 lies between them, so the foe's blow lands if the
// slow never reached the numbers and misses if it did.
func TestASlowLandedBeforeTheTargetActsIsSpentOnThatAction(t *testing.T) {
	session := twoWayBattle(t)
	// The hero acts first, so the foe's own action follows inside this exchange.
	draws := func(purpose, actorID, targetID string) float64 {
		switch purpose {
		case "initiative":
			if actorID == "hero" {
				return 1.0
			}
			return 0.0
		case "hit":
			return 0.79
		case "damage":
			return 0.5
		}
		return 0.5
	}

	// A target that cannot be dropped, so the foe still gets to act.
	session.Combatants[1].Attributes.Health = 1 << 20
	session.Combatants[1].Attributes.HealthMax = 1 << 20

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("heavy_thunder"), Variant: VariantSlow, TargetID: "foe"},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"},
	}, draws)
	heroStep := findStep(t, outcome, "hero")
	if !heroStep.Resolution.Hit {
		t.Fatalf("the hero's blow was forced to land: %+v", heroStep.Resolution)
	}
	if !heroStep.SlowApplied {
		t.Fatal("the slowing hit must attach the effect")
	}
	// The foe's own action follows, and it is made at 17 agility: its hit chance
	// is 0.7797, below the 0.79 draw.
	if foeStep := findStep(t, outcome, "foe"); foeStep.Resolution.Hit {
		t.Errorf("the slowed foe's blow landed at a 0.79 draw; the penalty never reached its check")
	}
	// Its next primary action has completed, so the effect is spent.
	if got := next.CombatantByID("foe").Slow; got.RemainingTargetActions != 0 {
		t.Errorf("slow = %+v after the target acted in the same exchange, want it spent", got)
	}
	if got := next.CombatantByID("foe").Attributes.Agility; got != 20 {
		t.Errorf("agility = %d after the slow was spent, want the base 20", got)
	}
}

// §4.2: a prepared technique completes on the following exchange, and its cost,
// burden and residual injury are all settled when it is started.
func TestAPreparedTechniqueCompletesOnTheFollowingExchange(t *testing.T) {
	session := withBladeInHand(twoWayBattle(t), "hero")
	secret := mustSkill("meridian_secret_blade")

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret, TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5}))
	step := findStep(t, outcome, "hero")
	if !step.Cast.Allowed {
		t.Fatalf("a full-health six-layer cultivator must be able to start the secret blade: %+v", step.Cast)
	}
	if step.ReasonKey != "reason.preparing" {
		t.Fatalf("reason = %q, want the preparing marker", step.ReasonKey)
	}
	hero := next.CombatantByID("hero")
	// 40% of 160 is 64 mana, paid at the start.
	if hero.Attributes.Mana != 96 {
		t.Errorf("mana after starting = %d, want 96 (160 − 64)", hero.Attributes.Mana)
	}
	// 开始准备时扣耗灵并增加85负担，同时留下冲脉余伤. Burden is added, the
	// exchange's own 6 relief applies, and the floor of 60 then holds it there:
	// 85 − 6 = 79, which is above 60, so the floor does not bind yet.
	if hero.Burden.Residual != 60 {
		t.Errorf("residual = %d, want the published floor 60 to be set at the start", hero.Burden.Residual)
	}
	if _, ok := next.Preparations["hero"]; !ok {
		t.Fatal("the technique must be recorded as in preparation")
	}
	if next.CombatantByID("foe").Attributes.Health != 200 {
		t.Fatal("a preparation must not damage anyone in the exchange it starts")
	}

	// The following exchange completes it, ignoring 20% of the target's defense:
	// 40 × 3.20 × 40/(40+16) = 91.43, rounded to 91.
	after, _ := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionContinuePreparation},
		"foe":  {Kind: ActionRest},
	}, drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5}))
	if got := after.CombatantByID("foe").Attributes.Health; got != 200-91 {
		t.Errorf("health after the secret blade = %d, want %d", got, 200-91)
	}
	if _, ok := after.Preparations["hero"]; ok {
		t.Error("the preparation must be cleared once it completes")
	}
	// §4.2's worked example: 下一轮完成后B=73, then 继续战斗最多消解到60.
	if got := after.CombatantByID("hero").Burden.Current; got != 73 {
		t.Errorf("burden after completion = %d, want the published 73 (79 − 6)", got)
	}
	if got := after.CombatantByID("hero").Attributes.Mana; got != 96 {
		t.Errorf("continuing a prepared technique must not recover or repay mana, got %d", got)
	}
}

// §4.2: 继续战斗最多消解到60，所以不能再次承担85负担 — the residual floor holds the
// burden at 60, so the same secret technique cannot be performed again inside the
// battle however long the character waits.
func TestAResidualFloorBlocksRepeatingTheSecretTechnique(t *testing.T) {
	session := withBladeInHand(twoWayBattle(t), "hero")
	secret := mustSkill("meridian_secret_blade")
	draws := drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5})

	next, _ := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret, TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, draws)
	next.Combatants[1].Attributes.Health = 1 << 20
	next.Combatants[1].Attributes.HealthMax = 1 << 20
	next, _ = ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionContinuePreparation},
		"foe":  {Kind: ActionRest},
	}, draws)

	// Several more quiet exchanges: burden falls to the floor and stops there.
	for i := 0; i < 5; i++ {
		next, _ = ResolveExchange(next, map[string]CombatAction{
			"hero": {Kind: ActionRest},
			"foe":  {Kind: ActionRest},
		}, draws)
	}
	hero := next.CombatantByID("hero")
	if hero.Burden.Current != 60 {
		t.Fatalf("burden = %d, want it held at the residual floor 60", hero.Burden.Current)
	}
	// Mana is restored so only the residual floor can be the reason.
	hero.Attributes.Mana = hero.Attributes.ManaMax

	_, outcome := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret, TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, draws)
	step := findStep(t, outcome, "hero")
	if step.Cast.Allowed {
		t.Fatal("the secret blade must be refused while its own residual injury stands")
	}
	if step.Cast.ReasonKey != ReasonMeridianUnrecovered {
		t.Errorf("reason = %q, want the meridian reason", step.Cast.ReasonKey)
	}
}

// §14.2: a survival-templated enemy with no route out offers to yield rather
// than fighting to nothing, once it is at or below 30% health.
func TestASurvivalEnemyOffersToYieldWhenHurt(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = false
	foe.Attributes.Health = 50 // 25% of 200, below the 30% line.

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionSurrender {
		t.Fatalf("action = %q, want an offer to yield", action.Kind)
	}
}

// §14.2: the same enemy with a real route out runs instead.
func TestASurvivalEnemyWithARouteRuns(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	foe.Attributes.Health = 50

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionEscape {
		t.Fatalf("action = %q, want an escape attempt", action.Kind)
	}
}

// §14.2: the evaluation is not a fixed health line that always fires. A healthy
// enemy keeps fighting.
func TestAHealthyEnemyKeepsFighting(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	// 120 of 200 is 60%, above the 30% line.
	foe.Attributes.Health = 120

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionTechnique {
		t.Fatalf("action = %q, want it to keep fighting", action.Kind)
	}
}

// §14.2: a mindless enemy gets no surrender template at all, so it never
// evaluates retreat however badly it is losing.
func TestAMindlessEnemyNeverEvaluatesRetreat(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatNone
	foe.RouteExists = true
	foe.Attributes.Health = 1

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionTechnique {
		t.Fatalf("action = %q, want a mindless enemy to keep attacking", action.Kind)
	}
}

// §14.2: the cautious template evaluates earlier, at 50%, but only while it has
// actually observed a clear disadvantage. Being hurt is not by itself a
// disadvantage — a cautious enemy that is ahead keeps fighting.
func TestACautiousEnemyWaitsForAnObservedDisadvantage(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatCautious
	foe.RouteExists = true
	foe.Attributes.Health = 90 // 45%, below the 50% line.
	// The player is the one worse off, so no disadvantage has been observed.
	session.CombatantByID("hero").Attributes.Health = 80

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionTechnique {
		t.Fatalf("action = %q, want it to keep fighting while it is the one ahead", action.Kind)
	}

	// Now the player is the one far healthier: the disadvantage is real.
	session.CombatantByID("hero").Attributes.Health = 180
	action = DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionEscape {
		t.Fatalf("action = %q, want an escape attempt once clearly behind", action.Kind)
	}
}

// §14.2: the survival template also evaluates when no usable primary technique
// remains and the objective cannot be pursued, not only on a health line.
func TestASurvivalEnemyEvaluatesWhenItsTechniqueIsUnusable(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	// Full health, so only the unusable technique can trigger the evaluation.
	foe.Attributes.Health = 200
	foe.Attributes.Mana = 0

	action := DecideEnemyAction(session, "foe", mustSkill("fire_bolt"))
	if action.Kind != ActionEscape {
		t.Fatalf("action = %q, want an escape attempt once it can no longer cast", action.Kind)
	}
}

// §14.2: the enemy's own choice must be priced the way settlement prices it.
// A technique the enemy can cast but cannot also drive its implement for is not
// one it can really carry out, so it must not be selected — otherwise the
// exchange records a refusal the enemy never intended and its round is wasted.
func TestAnEnemyTechniqueCountsTheRealOperatingCost(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	foe.Skills = []SkillDef{mustSkill("fire_bolt")}
	// fire_bolt costs 8 mana at layer 6. Leave exactly enough for the technique
	// and none for the implement, which declares a real per-exchange cost.
	foe.Attributes.Mana = 8
	foe.OperatingManaCost = 5

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Skill.ID == "fire_bolt" {
		t.Fatal("the enemy chose a technique whose settlement would be refused for the implement cost")
	}
	// The plain attack adds no burden and costs no mana, so it stays available
	// and is the honest fallback rather than a refusal.
	if action.Kind != ActionTechnique || action.Skill.ID != "strike" {
		t.Fatalf("action = %+v, want the affordable plain attack", action)
	}
}

// The same enemy with the implement's cost covered keeps fighting, so the check
// is about the real price and not a blanket refusal.
func TestAnEnemyTechniqueIsAllowedWhenTheOperatingCostIsCovered(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	foe.Skills = []SkillDef{mustSkill("fire_bolt")}
	foe.OperatingManaCost = 5

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionTechnique || action.Skill.ID != "fire_bolt" {
		t.Fatalf("action = %+v, want the affordable technique", action)
	}
}

// The enemy's own circulating-method reduction is part of the real price, so a
// technique that is only affordable after the reduction must still be chosen.
func TestAnEnemyTechniqueUsesItsOwnManaReduction(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Retreat = RetreatSurvival
	foe.RouteExists = true
	foe.Skills = []SkillDef{mustSkill("fire_bolt")}
	// 8 base, 20% reduction floors at ceil(6.4)=7, so 7 mana is exactly enough.
	foe.Attributes.Mana = 7
	foe.ManaReduction = 0.20

	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionTechnique || action.Skill.ID != "fire_bolt" {
		t.Fatalf("action = %+v, want the technique the reduction makes affordable", action)
	}
}

// §14.2: a combatant with no route cannot escape, however the draw falls.
func TestEscapeIsRefusedWithoutARealRoute(t *testing.T) {
	session := twoWayBattle(t)
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionEscape},
	}, fixedDraws(0.0))
	step := findStep(t, outcome, "hero")
	if step.Escaped {
		t.Fatal("a character with no route out must not escape")
	}
	if next.CombatantByID("hero").Fled {
		t.Fatal("the combatant must still be in the fight")
	}
}

// The same draws must produce the same exchange, which is what lets a retry or a
// reopened window show the battle it already showed.
func TestTheSameDrawsProduceTheSameExchange(t *testing.T) {
	session := twoWayBattle(t)
	skill := mustSkill("fire_bolt")
	actions := map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: skill, TargetID: "foe"},
		"foe":  {Kind: ActionTechnique, Skill: skill, TargetID: "hero"},
	}
	draws := drawByPurpose(map[string]float64{"hit": 0.3, "damage": 0.7})

	first, firstOutcome := ResolveExchange(session, actions, draws)
	second, secondOutcome := ResolveExchange(session, actions, draws)

	for _, id := range []string{"hero", "foe"} {
		a := first.CombatantByID(id).Attributes.Health
		b := second.CombatantByID(id).Attributes.Health
		if a != b {
			t.Errorf("%s health = %d then %d; the same draws must settle the same way", id, a, b)
		}
	}
	if len(firstOutcome.Steps) != len(secondOutcome.Steps) {
		t.Fatalf("step counts differ: %d then %d", len(firstOutcome.Steps), len(secondOutcome.Steps))
	}
	// And the original session must be untouched, so an exchange can be held
	// uncommitted or abandoned.
	if session.CombatantByID("hero").Attributes.Health != 200 {
		t.Error("ResolveExchange must not mutate the session it was given")
	}
}

// Resolving must not mutate the caller's session, which is what lets the caller
// replay or discard an exchange.
func TestResolvingLeavesTheGivenSessionUntouched(t *testing.T) {
	session := twoWayBattle(t)
	before := session.Combatants[0].Attributes.Health
	next, _ := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
	}, fixedDraws(0.0))
	if session.Combatants[0].Attributes.Health != before {
		t.Error("the given session was mutated")
	}
	if next.Combatants[1].Attributes.Health == 200 {
		t.Error("the returned session should carry the settled damage")
	}
}

// A battle ends when one side has nobody left standing, and the result says
// which side that was.
func TestABattleEndsWhenOneSideIsDown(t *testing.T) {
	session := twoWayBattle(t)
	session.Combatants[1].Attributes.Health = 1
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
	}, drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5}))

	if !next.Ended || !outcome.Ended {
		t.Fatal("the battle must end when the enemy is down")
	}
	if next.Result != ResultPlayerWon {
		t.Errorf("result = %q, want the player to have won", next.Result)
	}
	// Being downed is not being dead: §7 keeps those apart.
	if !next.CombatantByID("foe").Defeated {
		t.Error("the enemy should be marked defeated")
	}
	if next.CombatantByID("foe").Attributes.Health != 0 {
		t.Errorf("health = %d, want 0", next.CombatantByID("foe").Attributes.Health)
	}
}

// §4.1: the burden capacity falls with real health, so a badly wounded character
// cannot perform a technique that a healthy one could.
func TestAWoundedCharacterLosesBurdenCapacity(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	// 50% health puts the capacity at 75.
	hero.Attributes.Health = 100
	hero.Burden.Current = 70

	check := CanPerform(mustSkill("heavy_thunder"), hero.Cultivation, hero.Attributes, hero.Burden, 0, false)
	if check.Allowed {
		t.Fatal("70 burden plus 42 must exceed the 75 capacity of a wounded body")
	}
	if check.ReasonKey != ReasonBurdenCapacity {
		t.Errorf("reason = %q, want the burden reason", check.ReasonKey)
	}
}

// A rest recovers 5% of the real mana maximum, which §4 is explicit about — and
// only when the character was not wounded this exchange.
func TestRestingRecoversManaDuringAnExchange(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.Attributes.Mana = 100
	// The foe is out of the fight, so the resting character takes no damage this
	// exchange and the recovery applies.
	session.Combatants[1].Attributes.Health = 0
	session.Combatants[1].Defeated = true

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionRest},
	}, fixedDraws(0.5))
	step := findStep(t, outcome, "hero")
	// 5% of 160 is 8.
	if step.ManaRecovered != 8 {
		t.Errorf("mana recovered = %d, want 8 (5%% of 160)", step.ManaRecovered)
	}
	if next.CombatantByID("hero").Attributes.Mana != 108 {
		t.Errorf("mana = %d, want 108", next.CombatantByID("hero").Attributes.Mana)
	}
}

// §4: the recovery does not apply to a character who lost real health this
// exchange, however the loss arrived.
func TestARestRecoversNothingWhenWounded(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.Attributes.Mana = 100

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("fire_bolt"), TargetID: "hero"},
	}, drawByPurpose(map[string]float64{"hit": 0.0, "damage": 0.5}))
	step := findStep(t, outcome, "hero")
	if step.ManaRecovered != 0 {
		t.Errorf("mana recovered = %d after taking a wound, want 0", step.ManaRecovered)
	}
	if next.CombatantByID("hero").Attributes.Mana != 100 {
		t.Errorf("mana = %d, want the unchanged 100", next.CombatantByID("hero").Attributes.Mana)
	}
}

// A mortal cannot perform a learned technique, which is the rule that keeps
// zero mana from meaning free casting. The ordinary attack is not a technique:
// the mortal may still swing, and TestAMortalCanStillSwing holds that half.
func TestAMortalCannotPerformATechnique(t *testing.T) {
	session := CombatSession{
		ID: "battle1",
		Combatants: []Combatant{{
			ID: "mortal", Side: SidePlayer,
			Cultivation: Mortal(),
			Attributes:  BaseAttributes(Mortal()),
		}, {
			ID: "foe", Side: SideEnemy,
			Cultivation: Qi(6), Attributes: BaseAttributes(Qi(6)),
		}},
		Round: 1,
	}
	_, outcome := ResolveExchange(session, map[string]CombatAction{
		"mortal": {Kind: ActionTechnique, Skill: mustSkill("fire_bolt"), TargetID: "foe"},
	}, fixedDraws(0.5))
	step := findStep(t, outcome, "mortal")
	if step.Cast.Allowed {
		t.Fatal("a mortal must not perform a technique")
	}
	if step.Cast.ReasonKey != ReasonMortalNoTechniques {
		t.Errorf("reason = %q, want the mortal reason", step.Cast.ReasonKey)
	}
}

// A refused cast must spend nothing: no mana, no burden, no damage.
func TestARefusedCastSpendsNothing(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.Attributes.Mana = 0

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("fire_bolt"), TargetID: "foe"},
	}, fixedDraws(0.5))
	step := findStep(t, outcome, "hero")
	if step.Cast.Allowed {
		t.Fatal("a caster with no mana must be refused")
	}
	if step.Cast.ReasonKey != ReasonInsufficientMana {
		t.Errorf("reason = %q, want the mana reason", step.Cast.ReasonKey)
	}
	if next.CombatantByID("hero").Attributes.Mana != 0 {
		t.Error("a refused cast must not spend mana")
	}
	if next.CombatantByID("hero").Burden.Current != 0 {
		t.Error("a refused cast must not add burden")
	}
	if next.CombatantByID("foe").Attributes.Health != 200 {
		t.Error("a refused cast must not damage anyone")
	}
}

// A refused heavy move must not take the ordinary attack down with it.
//
// §T21's 普攻、防御或合法轻招不被一并封死. Burden is what stops a heavy
// technique; it is not a condition that stops the character acting at all. The
// plain strike costs no mana and no burden, so it has to remain available in
// the same exchange the heavy move was refused in — and it has to be the one
// that spends nothing, or the refusal would have charged for itself.
func TestARefusedHeavyMoveLeavesTheOrdinaryAttackAvailable(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	// Already at the burden limit: any further heavy move is out of reach.
	hero.Burden.Current = BurdenCapacity(hero.Attributes.Health, hero.Attributes.HealthMax)

	heavy := mustSkill("heavy_thunder")
	plain := mustSkill(OrdinaryAttackSkillID)
	if plain.ManaFraction != 0 || plain.Burden != 0 {
		t.Fatalf("the ordinary attack must cost nothing, got mana %v burden %d", plain.ManaFraction, plain.Burden)
	}
	manaBefore := hero.Attributes.Mana
	burdenBefore := hero.Burden.Current

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: heavy, TargetID: "foe"},
	}, fixedDraws(0.5))
	refused := findStep(t, outcome, "hero")
	if refused.Cast.Allowed {
		t.Fatal("the heavy move must be refused at the burden limit")
	}
	if refused.Cast.ReasonKey != ReasonBurdenCapacity {
		t.Errorf("reason = %q, want the burden reason", refused.Cast.ReasonKey)
	}
	// The check still carries the price the move would have cost — that is what
	// lets the interface say how far over the limit it is — but nothing is
	// deducted for a refusal. Only the pools can prove that.
	if next.CombatantByID("hero").Attributes.Mana != manaBefore {
		t.Error("the refused heavy move must not have spent mana")
	}
	// The exchange itself sheds 6 burden for everyone, so the assertion is that
	// nothing was spent beyond that, not that the number stood still.
	if shed := burdenBefore - next.CombatantByID("hero").Burden.Current; shed != 6 {
		t.Errorf("burden fell by %d, want only the per-exchange recovery of 6", shed)
	}

	// The same character, in the same state, swings instead.
	afterRefusal, swung := ResolveExchange(next, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: plain, TargetID: "foe"},
	}, fixedDraws(0.5))
	step := findStep(t, swung, "hero")
	if !step.Cast.Allowed {
		t.Fatalf("the ordinary attack must stay available, refused with %q", step.Cast.ReasonKey)
	}
	if step.Resolution.Damage <= 0 {
		t.Error("the ordinary attack must actually land")
	}
	if afterRefusal.CombatantByID("hero").Attributes.Mana != manaBefore {
		t.Error("the ordinary attack costs no mana")
	}
}

// A variant that does not belong to the technique being cast is refused rather
// than silently changing the power.
func TestAVariantOfAnotherTechniqueIsRefused(t *testing.T) {
	if _, ok := VariantPower(VariantBurn, "heavy_thunder"); ok {
		t.Fatal("burning is a form of the fire bolt, not of the thunder technique")
	}
	if power, ok := VariantPower(VariantBurn, "fire_bolt"); !ok || math.Abs(power-BurnVariantPower) > 1e-9 {
		t.Fatalf("burning power = %v/%v, want %v", power, ok, BurnVariantPower)
	}
	if power, ok := VariantPower(VariantSlow, "heavy_thunder"); !ok || math.Abs(power-SlowVariantPower) > 1e-9 {
		t.Fatalf("slowing power = %v/%v, want %v", power, ok, SlowVariantPower)
	}
}

// TestVariantsForListsOnlyTheFormsOfThatTechnique keeps the published forms
// attached to their own base technique.
//
// §5 fixes exactly two variants and gives each one base. The window offers them
// from this list, so a table that leaked the burning form onto the thunder
// technique would offer a cast the engine refuses with
// reason.variant_unsupported.
func TestVariantsForListsOnlyTheFormsOfThatTechnique(t *testing.T) {
	if got := VariantsFor("fire_bolt"); len(got) != 1 || got[0] != VariantBurn {
		t.Fatalf("fire_bolt variants = %v, want only the burning form", got)
	}
	if got := VariantsFor("heavy_thunder"); len(got) != 1 || got[0] != VariantSlow {
		t.Fatalf("heavy_thunder variants = %v, want only the slowing form", got)
	}
	// A technique with no published form offers none rather than every form.
	for _, id := range []string{"strike", "flame_wave", "shield", ""} {
		if got := VariantsFor(id); len(got) != 0 {
			t.Errorf("%q variants = %v, want none", id, got)
		}
	}
}

// findStep returns the step a given combatant produced.
func findStep(t *testing.T, outcome ExchangeOutcome, actorID string) ExchangeStep {
	t.Helper()
	for _, step := range outcome.Steps {
		if step.ActorID == actorID {
			return step
		}
	}
	t.Fatalf("no step for %q in %+v", actorID, outcome.Steps)
	return ExchangeStep{}
}
