package cultivation

import (
	"math"
	"testing"
)

// TestPublishedDamageRanges reproduces the four damage ranges the design states
// in §4.1. They are the sharpest available check on the formula, because each
// range is the span produced by the damage-variance band alone.
func TestPublishedDamageRanges(t *testing.T) {
	strike, _ := SkillByID("strike")
	fireBolt, _ := SkillByID("fire_bolt")
	condensedBlade, _ := SkillByID("condensed_blade")

	cases := []struct {
		name  string
		stage Cultivation
		skill SkillDef
		low   int
		high  int
	}{
		{"qi 1 ordinary attack", Qi(1), strike, 11, 15},
		{"qi 6 ordinary attack", Qi(6), strike, 23, 31},
		{"qi 6 fire bolt", Qi(6), fireBolt, 27, 37},
		{"qi 6 condensed blade", Qi(6), condensedBlade, 36, 49},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attrs := BaseAttributes(tc.stage)
			low := damageWithRoll(attrs, attrs, tc.stage.Realm, tc.stage.Realm, tc.skill, 0)
			high := damageWithRoll(attrs, attrs, tc.stage.Realm, tc.stage.Realm, tc.skill, 0.999999)
			if low != tc.low || high != tc.high {
				t.Errorf("damage range %d–%d, want %d–%d", low, high, tc.low, tc.high)
			}
		})
	}
}

// damageWithRoll resolves one attack that is guaranteed to land, at the given
// damage-variance draw, and returns the damage dealt.
func damageWithRoll(attacker, target Attributes, attackerRealm, targetRealm Realm, skill SkillDef, varianceDraw float64) int {
	resolution := ResolveAttack(attacker, target, attackerRealm, targetRealm, skill, NeutralDamageContext(), Shield{}, CombatRolls{
		Hit:    0, // always below the 15% floor's minimum, so the attack lands
		Damage: varianceDraw,
	})
	return resolution.Damage
}

// TestAttackNeedsOneHundredPercentToGuaranteeNothing checks the hit band: a draw
// at or above the computed chance misses, and the clamp keeps a very agile
// attacker from becoming certain.
func TestAttackNeedsOneHundredPercentToGuaranteeNothing(t *testing.T) {
	strike, _ := SkillByID("strike")
	attacker := BaseAttributes(Qi(6))
	// Massively faster attacker: the raw term would exceed 97%, so the clamp
	// must hold it there.
	attacker.Agility = 10000
	target := BaseAttributes(Qi(6))
	chance := HitChance(attacker, target, strike.HitOffset)
	if math.Abs(chance-0.97) > 1e-9 {
		t.Errorf("hit chance = %v, want the 0.97 ceiling", chance)
	}
	// A draw of 0.97 exactly must miss, because the comparison is >=.
	resolution := ResolveAttack(attacker, target, Qi(6).Realm, Qi(6).Realm, strike, NeutralDamageContext(), Shield{}, CombatRolls{Hit: 0.97})
	if resolution.Hit {
		t.Error("a draw equal to the hit chance landed")
	}
}

// TestHitChanceBoundsAreReachableOnlyAsDocumented pins down which clamps can
// actually bind.
//
// The agility term is bounded to ±1 and carries a 0.25 coefficient, so agility
// alone moves the chance only within 0.55–1.05. That means the 0.97 ceiling is
// reachable from agility alone but the 0.15 floor is not: reaching the floor
// requires a technique with a large negative hit offset. Recording that here
// stops a future edit from "fixing" a floor that was never meant to bind on
// agility.
func TestHitChanceBoundsAreReachableOnlyAsDocumented(t *testing.T) {
	slow := BaseAttributes(Qi(1))
	fast := BaseAttributes(Qi(1))
	fast.Agility = 10000

	// The worst agility disadvantage alone leaves the chance well above the
	// floor.
	agilityOnly := HitChance(slow, fast, 0)
	if agilityOnly <= 0.15 {
		t.Errorf("agility alone reached %v; the floor should not bind on agility", agilityOnly)
	}
	if math.Abs(agilityOnly-0.5505) > 0.001 {
		t.Errorf("worst agility-only chance = %v, want about 0.55", agilityOnly)
	}

	// The ceiling does bind on agility alone.
	if chance := HitChance(fast, slow, 0); math.Abs(chance-0.97) > 1e-9 {
		t.Errorf("best agility-only chance = %v, want the 0.97 ceiling", chance)
	}

	// The floor binds once a technique carries a large enough negative offset.
	if chance := HitChance(slow, fast, -0.80); math.Abs(chance-0.15) > 1e-9 {
		t.Errorf("chance with a large negative offset = %v, want the 0.15 floor", chance)
	}
}

// TestCrossRealmDamageScale checks the published 0.35 and 0.10 factors and that
// a small-stage difference gets no extra factor.
func TestCrossRealmDamageScale(t *testing.T) {
	cases := []struct {
		attacker, target Realm
		want             float64
	}{
		{RealmQi, RealmQi, 1.00},
		{RealmFoundation, RealmQi, 1.00}, // higher attacker: no bonus factor
		{RealmQi, RealmFoundation, 0.35},
		{RealmQi, RealmCore, 0.10},
		{RealmQi, RealmSpirit, 0.10},
		{RealmMortal, RealmQi, 0.35},
	}
	for _, tc := range cases {
		if got := CrossRealmDamageScale(tc.attacker, tc.target); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("scale %s→%s = %v, want %v", tc.attacker, tc.target, got, tc.want)
		}
	}
}

// TestShieldAbsorbsBeforeHealth checks the ordering the design requires: the
// shield takes the final damage first and only the remainder reaches health.
func TestShieldAbsorbsBeforeHealth(t *testing.T) {
	strike, _ := SkillByID("strike")
	attacker := BaseAttributes(Qi(6))
	target := BaseAttributes(Qi(6))
	shield := Shield{Amount: 20, RemainingExchanges: 2, SourceID: "test"}
	resolution := ResolveAttack(attacker, target, Qi(6).Realm, Qi(6).Realm, strike, NeutralDamageContext(), shield, CombatRolls{Hit: 0, Damage: 0.5})
	if !resolution.Hit {
		t.Fatal("the attack did not land")
	}
	if resolution.ShieldAbsorbed+resolution.HealthLost != resolution.Damage {
		t.Errorf("absorbed %d + lost %d != damage %d", resolution.ShieldAbsorbed, resolution.HealthLost, resolution.Damage)
	}
	if resolution.Damage > 20 && resolution.ShieldAbsorbed != 20 {
		t.Errorf("a 20-point shield absorbed %d of %d", resolution.ShieldAbsorbed, resolution.Damage)
	}
}

// TestSkillManaCostsMatchThePublishedTable checks the six-layer costs of §4.2.
func TestSkillManaCostsMatchThePublishedTable(t *testing.T) {
	cases := []struct {
		id   string
		want int
	}{
		{"strike", 0},
		{"fire_bolt", 8},
		{"condensed_blade", 20},
		{"heavy_thunder", 36},
		{"flame_wave", 36},
		{"shield", 16},
		{"meridian_secret_blade", 64},
	}
	six := Qi(6)
	for _, tc := range cases {
		skill, ok := SkillByID(tc.id)
		if !ok {
			t.Fatalf("%s is not registered", tc.id)
		}
		if got := skill.ManaCost(six); got != tc.want {
			t.Errorf("%s costs %d at qi 6, want %d", tc.id, got, tc.want)
		}
	}
}

// TestManaCostFollowsTheCasterStage checks that a fixed coefficient produces a
// different absolute cost at a different legal stage, which is the property
// that stops a definition from carrying a hard-coded six-layer cost.
func TestManaCostFollowsTheCasterStage(t *testing.T) {
	fireBolt, _ := SkillByID("fire_bolt")
	// qi 1 mana maximum is 80, so 5% rounds up to 4.
	if got := fireBolt.ManaCost(Qi(1)); got != 4 {
		t.Errorf("fire bolt at qi 1 costs %d, want 4", got)
	}
	// qi 13 mana maximum is 272, so 5% rounds up to 14.
	if got := fireBolt.ManaCost(Qi(13)); got != 14 {
		t.Errorf("fire bolt at qi 13 costs %d, want 14", got)
	}
}

// TestHeavyTechniqueBlockedByBurdenNotMana is the design's own acceptance case:
// a heavy technique must be refused for lack of burden headroom even when the
// caster has ample mana.
func TestHeavyTechniqueBlockedByBurdenNotMana(t *testing.T) {
	heavyThunder, _ := SkillByID("heavy_thunder")
	stage := Qi(6)
	attrs := BaseAttributes(stage)
	attrs.Mana = attrs.ManaMax // plenty of mana

	// §4.2 states that a third heavy thunder exceeds capacity: 42 + 42 = 84,
	// then recovery brings it to 78, and a third would reach 120.
	burden := BurdenState{Current: 78}
	check := CanPerform(heavyThunder, stage, attrs, burden, 0, true)
	if check.Allowed {
		t.Fatal("a third heavy thunder was allowed at 78 burden")
	}
	if check.ReasonKey != ReasonBurdenCapacity {
		t.Errorf("refusal reason %q, want %q", check.ReasonKey, ReasonBurdenCapacity)
	}
}

// TestAMortalCanStillSwing reproduces the defect a page check found: a mortal
// was refused the ordinary attack with reason.mortal_no_techniques, so a
// protagonist who had not yet begun cultivating could not take a single round.
//
// OrdinaryAttackSkillID's own contract says it is "the zero-cost attack every
// combatant always has", and explicitly that "a mortal, who may cast nothing
// else — must still be able to fight". The realm gate belongs to learned
// techniques, whose zero-mana cost formula must not become free casting; it
// must not reach the plain attack, which is not a technique at all.
func TestAMortalCanStillSwing(t *testing.T) {
	mortal := Mortal()
	attrs := BaseAttributes(mortal)
	check := CanPerform(mustSkill("strike"), mortal, attrs, BurdenState{}, 0, false)
	if !check.Allowed {
		t.Fatalf("a mortal's ordinary attack was refused with %q", check.ReasonKey)
	}
	// The gate still holds where it belongs: a mortal casts no learned technique,
	// even a zero-mana one.
	fireBolt, _ := SkillByID("fire_bolt")
	check = CanPerform(fireBolt, mortal, attrs, BurdenState{}, 0, false)
	if check.Allowed || check.ReasonKey != ReasonMortalNoTechniques {
		t.Errorf("a mortal's fire bolt = allowed %v reason %q, want the realm refusal", check.Allowed, check.ReasonKey)
	}
}

// TestTheOrdinaryAttackSurvivesAnOverCapacityBody covers the way out of a
// wound. §4.1 has every surviving cultivator shed burden at the end of a round,
// ordinary attacks included, and it prices a residual injury above what a badly
// wounded body can carry. If the plain attack were refused for the burden that
// injury already put on the character, there would be no legal action left and
// the fight could not be finished.
func TestTheOrdinaryAttackSurvivesAnOverCapacityBody(t *testing.T) {
	stage := Qi(6)
	attrs := BaseAttributes(stage)
	// Under 30% health the capacity is 50, and a secret technique's residual
	// injury holds the burden at 60.
	attrs.Health = 40
	burden := BurdenState{Current: 60, Residual: 60}
	check := CanPerform(mustSkill("strike"), stage, attrs, burden, 0, false)
	if !check.Allowed {
		t.Fatalf("the ordinary attack was refused with %q", check.ReasonKey)
	}
	if check.BurdenCost != 0 {
		t.Errorf("the ordinary attack costs %d burden, want 0", check.BurdenCost)
	}
	// A technique that does add burden is still refused, which is the rule the
	// exemption must not weaken.
	check = CanPerform(mustSkill("heavy_thunder"), stage, attrs, BurdenState{Current: 60}, 0, false)
	if check.Allowed || check.ReasonKey != ReasonBurdenCapacity {
		t.Errorf("heavy thunder at 60/50 = allowed %v reason %q, want the burden refusal", check.Allowed, check.ReasonKey)
	}
}

// TestContinuousCastingIsLimitedByRecoveryNotACooldown reproduces the §4.2
// heavy-cast rule: two heavy thunders fit from zero and the third exceeds the
// burden limit, even though the caster still has enough mana.
func TestContinuousCastingIsLimitedByRecoveryNotACooldown(t *testing.T) {
	heavy := mustSkill("heavy_thunder")
	stage := Qi(6)
	attrs := BaseAttributes(stage)
	attrs.Mana = attrs.ManaMax
	burden := BurdenState{}

	casts := 0
	for i := 0; i < 10; i++ {
		check := CanPerform(heavy, stage, attrs, burden, 0, true)
		if !check.Allowed {
			break
		}
		attrs, burden = ApplyCast(check, attrs, burden)
		burden = RecoverBurdenPerExchange(burden, true)
		casts++
	}
	if casts != 2 {
		t.Errorf("performed %d consecutive heavy thunders, want 2", casts)
	}
	if burden.Current != 72 {
		t.Errorf("burden after two casts = %d, want 72", burden.Current)
	}
}

// TestFireBoltIsSustainable checks the contrasting case: net zero burden, so a
// fire bolt can be repeated while mana lasts.
func TestFireBoltIsSustainable(t *testing.T) {
	fireBolt, _ := SkillByID("fire_bolt")
	stage := Qi(6)
	attrs := BaseAttributes(stage)
	burden := BurdenState{}

	for i := 0; i < 20; i++ {
		check := CanPerform(fireBolt, stage, attrs, burden, 0, true)
		if !check.Allowed {
			t.Fatalf("fire bolt refused on cast %d: %s", i+1, check.ReasonKey)
		}
		attrs, burden = ApplyCast(check, attrs, burden)
		burden = RecoverBurdenPerExchange(burden, true)
	}
	// Twenty casts at 8 mana each consume the whole 160.
	if attrs.Mana != 0 {
		t.Errorf("mana after twenty fire bolts = %d, want 0", attrs.Mana)
	}
	if burden.Current != 0 {
		t.Errorf("burden after twenty fire bolts = %d, want 0", burden.Current)
	}
	// The twenty-first is refused for mana, which is the real limit here.
	check := CanPerform(fireBolt, stage, attrs, burden, 0, true)
	if check.Allowed || check.ReasonKey != ReasonInsufficientMana {
		t.Errorf("twenty-first cast: allowed=%v reason=%q, want a mana refusal", check.Allowed, check.ReasonKey)
	}
}

// TestResidualInjurySurvivesRestoringPill reproduces the design's stated
// acceptance case: after the secret technique, taking a mana pill still does not
// allow it to be performed again.
func TestResidualInjurySurvivesRestoringPill(t *testing.T) {
	secretBlade, _ := SkillByID("meridian_secret_blade")
	stage := Qi(13)
	attrs := BaseAttributes(stage)

	// Prepare it: pay the mana, add the burden, take the residual.
	burden := BurdenState{Current: 79, Residual: secretBlade.ResidualBurdenFloor}
	attrs.Mana = 54

	// A restoring pill refills mana — but only by its own fixed amount. The
	// dose is registered against qi 6, so it restores 32 whatever the user's
	// stage. Three doses take the qi 13 character from 54 to 150, still short
	// of the 272 maximum, which is the point: a qi 6 pill is not a full refill
	// for a higher cultivator.
	pill := PillDef{ID: "spirit_pill", Grade: GradeUpper, Purpose: PillRestoreMana, RegisteredStage: Qi(6)}
	attrs, burden = pill.ApplyPill(attrs, burden)
	if attrs.Mana != 86 {
		t.Fatalf("one dose restored to %d, want 86", attrs.Mana)
	}
	// Take enough doses to clear the mana precondition, so the refusal below
	// cannot be explained by low mana.
	for i := 0; i < 10; i++ {
		attrs, burden = pill.ApplyPill(attrs, burden)
	}
	if attrs.Mana < int(0.80*float64(attrs.ManaMax)) {
		t.Fatalf("mana reached only %d of %d; the test cannot isolate the residual", attrs.Mana, attrs.ManaMax)
	}
	// The residual still blocks the technique.
	check := CanPerform(secretBlade, stage, attrs, burden, 0, true)
	if check.Allowed {
		t.Fatal("the secret blade was allowed again after a mana pill")
	}
	if check.ReasonKey != ReasonMeridianUnrecovered {
		t.Errorf("refusal reason %q, want %q", check.ReasonKey, ReasonMeridianUnrecovered)
	}
}

// TestResidualRecoveryRequiresRealConditions checks the §4.3 requirements: the
// residual clears only after the full safe interval at sufficient health and
// mana, and not when the interval is split.
func TestResidualRecoveryRequiresRealConditions(t *testing.T) {
	secretBlade, _ := SkillByID("meridian_secret_blade")
	attrs := BaseAttributes(Qi(13))

	// Split into two 15-minute intervals: neither satisfies the 30-minute
	// continuous requirement.
	if CanClearResidual(secretBlade, 15, attrs, false) {
		t.Error("a split interval cleared the residual")
	}
	// Wounded below 60% health.
	hurt := attrs
	hurt.Health = int(0.50 * float64(attrs.HealthMax))
	if CanClearResidual(secretBlade, 30, hurt, false) {
		t.Error("the residual cleared below the health requirement")
	}
	// Low mana.
	drained := attrs
	drained.Mana = int(0.10 * float64(attrs.ManaMax))
	if CanClearResidual(secretBlade, 30, drained, false) {
		t.Error("the residual cleared below the mana requirement")
	}
	// Blocked by a real injury or seal.
	if CanClearResidual(secretBlade, 30, attrs, true) {
		t.Error("the residual cleared while regulation was blocked")
	}
	// The full conditions.
	if !CanClearResidual(secretBlade, 30, attrs, false) {
		t.Error("the residual did not clear under the full conditions")
	}
}

// TestClearResidualZeroesBurdenAndKeepsPools checks that clearing the residual
// relieves ordinary burden but does not refill health or mana.
func TestClearResidualZeroesBurdenAndKeepsPools(t *testing.T) {
	attrs := BaseAttributes(Qi(13))
	attrs.Health = 200
	attrs.Mana = 100
	burden := ClearResidual(BurdenState{Current: 60, Residual: 60})
	if burden.Current != 0 || burden.Residual != 0 {
		t.Errorf("burden after clearing = %+v, want zero", burden)
	}
	if attrs.Health != 200 || attrs.Mana != 100 {
		t.Errorf("pools changed to %d/%d; clearing a residual must not refill them", attrs.Health, attrs.Mana)
	}
}

// TestOrdinaryAttackAlwaysAvailableEvenAtCapacity checks that being unable to
// perform a heavy technique does not disable a legal burden-free action.
func TestOrdinaryAttackAlwaysAvailableEvenAtCapacity(t *testing.T) {
	strike, _ := SkillByID("strike")
	stage := Qi(6)
	attrs := BaseAttributes(stage)
	burden := BurdenState{Current: 100}
	check := CanPerform(strike, stage, attrs, burden, 0, true)
	if !check.Allowed {
		t.Errorf("an ordinary attack was refused at full burden: %s", check.ReasonKey)
	}
}

// TestMortalCannotPerformALearnedTechnique checks that a mortal has no
// technique eligibility, so zero mana is not free casting. The ordinary attack
// is not a technique and is covered separately by TestAMortalCanStillSwing.
func TestMortalCannotPerformALearnedTechnique(t *testing.T) {
	fireBolt, _ := SkillByID("fire_bolt")
	check := CanPerform(fireBolt, Mortal(), BaseAttributes(Mortal()), BurdenState{}, 0, true)
	if check.Allowed {
		t.Fatal("a mortal performed a technique")
	}
	if check.ReasonKey != ReasonMortalNoTechniques {
		t.Errorf("refusal reason %q, want %q", check.ReasonKey, ReasonMortalNoTechniques)
	}
}

// TestManaReductionIsCappedAndFloored checks that cost reductions cannot make a
// paid technique free: the 20% cap and the 2% floor both bind.
func TestManaReductionIsCappedAndFloored(t *testing.T) {
	fireBolt, _ := SkillByID("fire_bolt")
	stage := Qi(6)
	// Full reduction, far beyond the cap: 8 × 0.80 = 6.4, rounding up to 7.
	if got := fireBolt.ReducedManaCost(stage, 0.90); got != 7 {
		t.Errorf("reduced cost = %d, want 7", got)
	}
	// The floor binds for a technique whose base cost is already small.
	cheap := SkillDef{ID: "cheap", ManaFraction: 0.03, Targets: 1, ExecutionActions: 1}
	// 3% of 160 is 4.8, rounding up to 5; a 20% reduction gives 4, but the
	// floor is ceil(2% of 160) = 4, so the result is 4 either way.
	if got := cheap.ReducedManaCost(stage, 0.20); got < PaidManaFloor(stage) {
		t.Errorf("reduced cost %d fell below the paid floor %d", got, PaidManaFloor(stage))
	}
}

// TestBurdenCapacityFollowsHealthState checks the published thresholds.
func TestBurdenCapacityFollowsHealthState(t *testing.T) {
	cases := []struct {
		health, healthMax, want int
	}{
		{200, 200, 100},
		{120, 200, 100}, // exactly 60% still carries 100
		{119, 200, 75},
		{60, 200, 75}, // exactly 30% still carries 75
		{59, 200, 50},
	}
	for _, tc := range cases {
		if got := BurdenCapacity(tc.health, tc.healthMax); got != tc.want {
			t.Errorf("capacity at %d/%d = %d, want %d", tc.health, tc.healthMax, got, tc.want)
		}
	}
}

// TestBurdenRecoveryNeverCrossesAResidualFloor checks that waiting inside a
// battle cannot clear a secret technique's injury.
func TestBurdenRecoveryNeverCrossesAResidualFloor(t *testing.T) {
	burden := BurdenState{Current: 79, Residual: 60}
	for i := 0; i < 20; i++ {
		burden = RecoverBurdenPerExchange(burden, true)
	}
	if burden.Current != 60 {
		t.Errorf("burden settled at %d, want the residual floor 60", burden.Current)
	}
}

// TestBurdenRecoveryUsesRealElapsedTime checks the off-battle rate: six points
// per real minute, accumulated over the interval that actually passed.
//
// The rate is applied to the real duration and truncated to whole points, so a
// fraction of a minute that does not reach a whole point relieves nothing, while
// a long interval relieves proportionally more.
func TestBurdenRecoveryUsesRealElapsedTime(t *testing.T) {
	burden := BurdenState{Current: 100}
	// Half a minute is exactly three points at this rate.
	if got := RecoverBurdenOffBattle(burden, 0.5); got.Current != 97 {
		t.Errorf("half a minute left %d, want 97", got.Current)
	}
	// Five minutes relieves the full 30.
	if got := RecoverBurdenOffBattle(burden, 5); got.Current != 70 {
		t.Errorf("five minutes left %d, want 70", got.Current)
	}
	// No elapsed time relieves nothing, so closing a window cannot be used to
	// clear burden.
	if got := RecoverBurdenOffBattle(burden, 0); got.Current != 100 {
		t.Errorf("no elapsed time left %d, want 100", got.Current)
	}
}

// TestShieldSizeAndCap checks that a shield is built from the caster's bare
// health and capped against the target's own bare health.
func TestShieldSizeAndCap(t *testing.T) {
	shieldSkill, _ := SkillByID("shield")
	// A six-layer caster: 20% of 200 is 40, and the six-layer target's cap is
	// 35% of 200 = 70, so the shield is its full 40.
	six := Qi(6)
	built := NewShield(shieldSkill, six, six, "s1")
	if built.Amount != 40 {
		t.Errorf("shield = %d, want 40", built.Amount)
	}
	if built.RemainingExchanges != 2 {
		t.Errorf("shield lasts %d exchanges, want 2", built.RemainingExchanges)
	}

	// A strong caster shielding a weak target is limited by the target's cap.
	strong := Cultivation{Realm: RealmFoundation, Stage: StageEarly}
	weak := Qi(1)
	limited := NewShield(shieldSkill, strong, weak, "s2")
	if want := ShieldCap(weak); limited.Amount != want {
		t.Errorf("shield on a weak target = %d, want the target cap %d", limited.Amount, want)
	}
}

// TestSameTypeShieldDoesNotAccumulate checks the replacement rule.
func TestSameTypeShieldDoesNotAccumulate(t *testing.T) {
	six := Qi(6)
	existing := Shield{Amount: 15, RemainingExchanges: 1, SourceID: "same"}
	incoming := Shield{Amount: 40, RemainingExchanges: 2, SourceID: "same"}
	stacked := StackShield(existing, incoming, six)
	if stacked.Amount != 40 {
		t.Errorf("a same-type recast produced %d, want the fresh 40 rather than a sum", stacked.Amount)
	}
}

// TestShieldExpiresByRealTime checks that leaving a battle does not refresh a
// shield: the remaining term is computed from the real seconds that passed.
func TestShieldExpiresByRealTime(t *testing.T) {
	shield := Shield{Amount: 40, RemainingExchanges: 2, SourceID: "s"}
	// 40 real seconds is two exchanges, so the shield is gone.
	if got := ExpireShieldsAfter(shield, 40); got.Amount != 0 {
		t.Errorf("after 40 seconds the shield is %+v, want expired", got)
	}
	// 20 seconds leaves one exchange.
	if got := ExpireShieldsAfter(shield, 20); got.RemainingExchanges != 1 {
		t.Errorf("after 20 seconds %d exchanges remain, want 1", got.RemainingExchanges)
	}
}

// TestRestManaRecoveryNeedsAnUndamagedExchange checks §5's rest action.
func TestRestManaRecoveryNeedsAnUndamagedExchange(t *testing.T) {
	if got := RestManaRecovery(160, false); got != 8 {
		t.Errorf("rest recovered %d, want 8", got)
	}
	if got := RestManaRecovery(160, true); got != 0 {
		t.Errorf("rest after taking damage recovered %d, want 0", got)
	}
}

// TestEquipmentGradesAndBudgets checks the §4.1 published values, including the
// rule that a low-grade common item cannot reach the top-grade ceiling.
func TestEquipmentGradesAndBudgets(t *testing.T) {
	// 下品常见 = +5%.
	if bonus, _ := MainImplementBonus(GradeLower, RarityCommon, 0); math.Abs(bonus-0.05) > 1e-9 {
		t.Errorf("lower common = %v, want 0.05", bonus)
	}
	// 中品稀有 spent entirely on attack = 10% + 5% = 15%.
	if bonus, _ := MainImplementBonus(GradeMiddle, RarityRare, 0.05); math.Abs(bonus-0.15) > 1e-9 {
		t.Errorf("middle rare fully spent = %v, want 0.15", bonus)
	}
	// 上品珍稀 = 18% + 8% = 26%.
	if bonus, _ := MainImplementBonus(GradeUpper, RarityPrecious, 0.08); math.Abs(bonus-0.26) > 1e-9 {
		t.Errorf("upper precious fully spent = %v, want 0.26", bonus)
	}
	// 顶品独特 = 25% + 10% = 35%.
	if bonus, _ := MainImplementBonus(GradeTop, RarityUnique, 0.10); math.Abs(bonus-0.35) > 1e-9 {
		t.Errorf("top unique fully spent = %v, want 0.35", bonus)
	}
	// A 下品 common item cannot reach 35% even if a definition asks for it: the
	// rarity budget is zero, so the spend is clamped away.
	if bonus, _ := MainImplementBonus(GradeLower, RarityCommon, 0.30); math.Abs(bonus-0.05) > 1e-9 {
		t.Errorf("lower common asking for more = %v, want the 0.05 ceiling", bonus)
	}
}

// TestPublishedAttributeExample reproduces §3.4's worked example: base attack
// 100 with technique +10%, implement +20%, pill +15% and formation +15% gives
// 160, and a combination summing to 85% still gives 160.
func TestPublishedAttributeExample(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}

	// Technique specialisation +10% attack comes from the attack template's
	// 强攻 profile; here it is expressed directly as its own source.
	modifiers := []AttributeModifier{
		{ID: "tech", Kind: SourceTechniqueSpecialisation, Attribute: AttrAttack, Value: 0.10},
		{ID: "implement", Kind: SourceMainImplement, Attribute: AttrAttack, Value: 0.20},
		{ID: "pill", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.15, Group: "attack_pill", Temporary: true},
		{ID: "formation", Kind: SourceFormation, Attribute: AttrAttack, Value: 0.15},
	}
	got := AggregateAttributes(base, modifiers)
	if got.Attack != 160 {
		t.Errorf("attack = %d, want 160", got.Attack)
	}

	// Now an over-budget combination: the cap holds it at 60%.
	over := []AttributeModifier{
		{ID: "a", Kind: SourceTechniqueSpecialisation, Attribute: AttrAttack, Value: 0.30},
		{ID: "b", Kind: SourceMainImplement, Attribute: AttrAttack, Value: 0.25},
		{ID: "c", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.15, Group: "attack_pill", Temporary: true},
		{ID: "d", Kind: SourceFormation, Attribute: AttrAttack, Value: 0.15},
	}
	if got := AggregateAttributes(base, over); got.Attack != 160 {
		t.Errorf("attack with 85%% of sources = %d, want the capped 160", got.Attack)
	}
}

// TestSameTypeTemporaryEffectsDoNotStack checks that two attack pills give the
// stronger one rather than their sum.
func TestSameTypeTemporaryEffectsDoNotStack(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	modifiers := []AttributeModifier{
		{ID: "weak_pill", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.05, Group: "attack_pill", Temporary: true},
		{ID: "strong_pill", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.15, Group: "attack_pill", Temporary: true},
	}
	// 15% of 100 is 15, giving 115 — not 120 from summing both.
	if got := AggregateAttributes(base, modifiers); got.Attack != 115 {
		t.Errorf("attack with two pills = %d, want the stronger 115", got.Attack)
	}
}

// TestHealthCapIsLowerThanTheCombatCap checks that the two pools use their own
// 40% ceiling rather than the 60% one.
func TestHealthCapIsLowerThanTheCombatCap(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	modifiers := []AttributeModifier{
		{ID: "big", Kind: SourceEventReward, Attribute: AttrHealthMax, Value: 0.90},
	}
	// 90% is capped at 40%, so the maximum is 1400.
	if got := AggregateAttributes(base, modifiers); got.HealthMax != 1400 {
		t.Errorf("health maximum = %d, want the capped 1400", got.HealthMax)
	}
}

// TestGainingABonusDoesNotHeal checks that a raised maximum does not refill the
// current value, and that losing a bonus only trims what exceeds the new cap.
func TestGainingABonusDoesNotHeal(t *testing.T) {
	wounded := Attributes{HealthMax: 1000, Health: 500, ManaMax: 800, Mana: 400, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	gain := []AttributeModifier{{ID: "g", Kind: SourceEventReward, Attribute: AttrHealthMax, Value: 0.40}}
	after := AggregateAttributes(wounded, gain)
	if after.HealthMax != 1400 {
		t.Fatalf("health maximum = %d, want 1400", after.HealthMax)
	}
	if after.Health != 500 {
		t.Errorf("current health became %d; a raised maximum must not heal", after.Health)
	}

	// Losing a bonus trims only what exceeds the new maximum.
	overfull := Attributes{HealthMax: 1400, Health: 1400, ManaMax: 800, Mana: 400, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	trimmed := AggregateAttributes(overfull, nil)
	if trimmed.HealthMax != 1400 || trimmed.Health != 1400 {
		t.Errorf("no modifiers changed %d/%d, want it untouched", trimmed.Health, trimmed.HealthMax)
	}
}

// TestNegativeModifiersDoNotReleasePositiveHeadroom checks the §3.4 rule that
// the two sums are capped separately.
func TestNegativeModifiersDoNotReleasePositiveHeadroom(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	modifiers := []AttributeModifier{
		{ID: "up", Kind: SourceMainImplement, Attribute: AttrAttack, Value: 0.50},
		{ID: "down", Kind: SourceEventReward, Attribute: AttrAttack, Value: -0.20},
	}
	// 50% positive and 20% negative, capped separately: 100 × 1.30 = 130. If
	// the negative had released headroom the result would be 100 × 1.50 = 150.
	if got := AggregateAttributes(base, modifiers); got.Attack != 130 {
		t.Errorf("attack = %d, want 130", got.Attack)
	}
}

// TestSpecialisationProfilesMatchTheTable checks the §3.1 template values.
func TestSpecialisationProfilesMatchTheTable(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}

	attack := AggregateAttributes(base, SpecialisationModifiers(SpecialisationAttack))
	if attack.Attack != 110 || attack.Defense != 80 || attack.HealthMax != 950 {
		t.Errorf("attack profile = atk %d def %d hp %d, want 110/80/950", attack.Attack, attack.Defense, attack.HealthMax)
	}

	spirit := AggregateAttributes(base, SpecialisationModifiers(SpecialisationSpirit))
	if spirit.Perception != 130 || spirit.ManaMax != 880 {
		t.Errorf("spirit profile = perception %d mana %d, want 130/880", spirit.Perception, spirit.ManaMax)
	}

	light := AggregateAttributes(base, SpecialisationModifiers(SpecialisationLight))
	if light.Agility != 125 {
		t.Errorf("light profile agility = %d, want 125", light.Agility)
	}
}

// TestTechniqueQualityAttackBonus checks the §3.1 quality correction.
func TestTechniqueQualityAttackBonus(t *testing.T) {
	base := Attributes{HealthMax: 1000, Health: 1000, ManaMax: 800, Mana: 800, Attack: 100, Defense: 100, Agility: 100, Perception: 100}
	if got := AggregateAttributes(base, TechniqueQualityModifier(QualityAdvanced)); got.Attack != 110 {
		t.Errorf("advanced quality attack = %d, want 110", got.Attack)
	}
	if got := AggregateAttributes(base, TechniqueQualityModifier(QualityOrdinary)); got.Attack != 100 {
		t.Errorf("ordinary quality attack = %d, want 100", got.Attack)
	}
}

// TestUpperGradeImplementExample reproduces §3.2's worked example: an upper
// grade common implement raises a six-layer attack of 40 to 47.
func TestUpperGradeImplementExample(t *testing.T) {
	bonus, _ := MainImplementBonus(GradeUpper, RarityCommon, 0)
	if math.Abs(bonus-0.18) > 1e-9 {
		t.Fatalf("upper common = %v, want 0.18", bonus)
	}
	six := Qi(6)
	base := BaseAttributes(six)
	modifiers := []AttributeModifier{EquipmentModifier("sword", AttrAttack, bonus)}
	got := AggregateAttributes(base, modifiers)
	// 40 × 1.18 = 47.2, displayed as 47.
	if got.Attack != 47 {
		t.Errorf("attack = %d, want 47", got.Attack)
	}
}

// TestPillRestoreIsFixedToTheRegisteredStage checks that a dose does not become
// stronger for a stronger user.
func TestPillRestoreIsFixedToTheRegisteredStage(t *testing.T) {
	// An upper-grade mana pill registered against qi 6: 20% of 160 = 32.
	pill := PillDef{ID: "spirit_pill", Grade: GradeUpper, Purpose: PillRestoreMana, RegisteredStage: Qi(6)}

	six := BaseAttributes(Qi(6))
	six.Mana = 0
	restored, _ := pill.ApplyPill(six, BurdenState{})
	if restored.Mana != 32 {
		t.Errorf("qi 6 restored %d, want 32", restored.Mana)
	}

	// The same dose on a foundation cultivator still restores 32.
	foundation := BaseAttributes(Cultivation{Realm: RealmFoundation, Stage: StageEarly})
	foundation.Mana = 0
	restoredFoundation, _ := pill.ApplyPill(foundation, BurdenState{})
	if restoredFoundation.Mana != 32 {
		t.Errorf("foundation restored %d, want the same 32", restoredFoundation.Mana)
	}
}

// TestPillAttackBonusByGrade checks the published boost values.
func TestPillAttackBonusByGrade(t *testing.T) {
	cases := []struct {
		grade ItemGrade
		want  float64
	}{
		{GradeLower, 0.05},
		{GradeMiddle, 0.08},
		{GradeUpper, 0.12},
		{GradeTop, 0.15},
	}
	for _, tc := range cases {
		pill := PillDef{ID: "boost", Grade: tc.grade, Purpose: PillBoostAttack, RegisteredStage: Qi(6), DurationExchanges: 3}
		bonus, ok := pill.AttackBonus()
		if !ok || math.Abs(bonus-tc.want) > 1e-9 {
			t.Errorf("%s boost = %v, want %v", tc.grade, bonus, tc.want)
		}
	}
}

// TestFormationBonusesMatchTheTableAndValidate checks the published ceilings
// and that an over-budget formation is refused.
func TestFormationBonusesMatchTheTableAndValidate(t *testing.T) {
	// 上品 attack formation at its ceiling is valid.
	upperAttack := FormationDef{ID: "f1", Grade: GradeUpper, AttackBonus: 0.12}
	if err := upperAttack.Validate(); err != nil {
		t.Errorf("an in-budget formation was refused: %v", err)
	}
	// Over the ceiling is refused.
	overBudget := FormationDef{ID: "f2", Grade: GradeLower, AttackBonus: 0.30}
	if err := overBudget.Validate(); err == nil {
		t.Error("an over-budget formation was accepted")
	}
	// Two main bonuses at once is refused.
	twoBonuses := FormationDef{ID: "f3", Grade: GradeTop, AttackBonus: 0.10, DefenseBonus: 0.10}
	if err := twoBonuses.Validate(); err == nil {
		t.Error("a formation granting two main bonuses was accepted")
	}
	// A top-grade defensive formation at 20% is valid.
	topDefense := FormationDef{ID: "f4", Grade: GradeTop, DefenseBonus: 0.20}
	if err := topDefense.Validate(); err != nil {
		t.Errorf("an in-budget defensive formation was refused: %v", err)
	}
}

// TestFormationCostsUseTheOperatorsBareMana checks the 10% activation and 2%
// upkeep costs.
func TestFormationCostsUseTheOperatorsBareMana(t *testing.T) {
	formation := FormationDef{ID: "f", Grade: GradeUpper, AttackBonus: 0.12}
	six := Qi(6)
	// 10% of 160 = 16; 2% of 160 = 3.2, rounding up to 4.
	if got := formation.StartManaCost(six); got != 16 {
		t.Errorf("activation cost = %d, want 16", got)
	}
	if got := formation.UpkeepManaCost(six); got != 4 {
		t.Errorf("upkeep cost = %d, want 4", got)
	}
}

// TestInactiveFormationContributesNothing checks that leaving the area or
// failing upkeep stops the bonus without further bookkeeping.
func TestInactiveFormationContributesNothing(t *testing.T) {
	formation := FormationDef{ID: "f", Grade: GradeUpper, AttackBonus: 0.12}
	state := FormationState{Def: formation, RemainingExchanges: 3, Active: true}
	if len(state.FormationModifiers()) != 1 {
		t.Fatal("an active formation contributed no modifier")
	}
	// Failing upkeep deactivates it.
	operator := BaseAttributes(Qi(6))
	operator.Mana = 0
	state, _ = state.Upkeep(operator, Qi(6))
	if state.Active {
		t.Error("the formation stayed active without upkeep")
	}
	if len(state.FormationModifiers()) != 0 {
		t.Error("an inactive formation still contributed a modifier")
	}
}

// TestBurningVariantMatchesThePublishedRule checks the burning variant's power
// and its two ticks at 15% of the committed damage.
func TestBurningVariantMatchesThePublishedRule(t *testing.T) {
	if math.Abs(BurnVariantPower-1.00) > 1e-9 {
		t.Errorf("burn variant power = %v, want 1.00", BurnVariantPower)
	}
	// A committed 30-damage hit burns for round(4.5) = 5 per tick, twice.
	burn := NewBurn(30, "b1")
	if burn.PerTick != 5 || burn.RemainingTicks != 2 {
		t.Errorf("burn = %+v, want 5 per tick over 2 ticks", burn)
	}
	first, burn := TickDamageOverTime(burn)
	second, burn := TickDamageOverTime(burn)
	if first != 5 || second != 5 {
		t.Errorf("ticks dealt %d then %d, want 5 then 5", first, second)
	}
	if burn.RemainingTicks != 0 {
		t.Errorf("burn still has %d ticks, want it exhausted", burn.RemainingTicks)
	}
	third, _ := TickDamageOverTime(burn)
	if third != 0 {
		t.Errorf("an exhausted burn dealt %d", third)
	}
}

// TestBurningDoesNotStackLayers checks that a second application does not add a
// third or fourth tick of a weaker effect.
func TestBurningDoesNotStackLayers(t *testing.T) {
	strong := NewBurn(30, "b1") // 5 per tick
	weak := NewBurn(10, "b1")   // 2 per tick
	stacked := StackBurn(strong, weak)
	if stacked.PerTick != 5 {
		t.Errorf("per tick = %d, want the stronger 5", stacked.PerTick)
	}
	if stacked.RemainingTicks != 2 {
		t.Errorf("ticks = %d, want a refresh to 2", stacked.RemainingTicks)
	}
}

// TestSlowVariantMatchesThePublishedRule checks the delaying variant's power,
// its 15% agility reduction, and that it expires after one target action.
func TestSlowVariantMatchesThePublishedRule(t *testing.T) {
	if math.Abs(SlowVariantPower-1.45) > 1e-9 {
		t.Errorf("slow variant power = %v, want 1.45", SlowVariantPower)
	}
	slow := NewSlow("s1")
	modifiers := slow.AgilityModifier()
	if len(modifiers) != 1 || math.Abs(modifiers[0].Value+0.15) > 1e-9 {
		t.Errorf("slow modifiers = %+v, want a single -0.15 agility", modifiers)
	}
	// It expires after the target's next primary action, not after a round.
	if expired := slow.TickSlow(); expired.RemainingTargetActions != 0 {
		t.Errorf("after one action %d remain, want the effect expired", expired.RemainingTargetActions)
	}
	if len(SlowEffect{}.AgilityModifier()) != 0 {
		t.Error("an expired slow still contributed a modifier")
	}
}

// TestValidateSkillDefinitionRefusesOutOfBudget checks that a generated
// definition exceeding the rules is refused before registration.
func TestValidateSkillDefinitionRefusesOutOfBudget(t *testing.T) {
	valid := SkillDef{ID: "ok", Power: 1.2, ManaFraction: 0.05, Burden: 6, Targets: 1, ExecutionActions: 1}
	if err := ValidateSkillDefinition(valid); err != nil {
		t.Errorf("a valid definition was refused: %v", err)
	}

	cases := map[string]SkillDef{
		"no id":                {Power: 1.0, Targets: 1, ExecutionActions: 1},
		"defense ignore > 20%": {ID: "x", Power: 1.0, Targets: 1, ExecutionActions: 1, IgnoreDefense: 0.50},
		"burden > 100":         {ID: "x", Power: 1.0, Burden: 150, Targets: 1, ExecutionActions: 1},
		"three-action prepare": {ID: "x", Power: 1.0, Targets: 1, ExecutionActions: 3},
		"shield over 35%":      {ID: "x", Power: 0, Targets: 1, ExecutionActions: 1, ShieldBaseHealthFraction: 0.50},
		"residual no floor":    {ID: "x", Power: 1.0, Targets: 1, ExecutionActions: 1, LeavesResidual: true},
		"negative power":       {ID: "x", Power: -1.0, Targets: 1, ExecutionActions: 1},
	}
	for name, skill := range cases {
		if err := ValidateSkillDefinition(skill); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// TestBuiltInDefinitionsAreAllValid checks that the published template set
// satisfies its own validation, so a future edit cannot leave the table in a
// state the rules would refuse.
func TestBuiltInDefinitionsAreAllValid(t *testing.T) {
	for _, skill := range skillTable {
		if err := ValidateSkillDefinition(skill); err != nil {
			t.Errorf("built-in %s is invalid: %v", skill.ID, err)
		}
	}
}

// TestSkillTableIsACopy checks that a caller cannot mutate the shared
// definitions, which the design requires to stay fixed once registered.
func TestSkillTableIsACopy(t *testing.T) {
	first := SkillTable()
	original := first["fire_bolt"]
	mutated := original
	mutated.Power = 99
	first["fire_bolt"] = mutated

	second := SkillTable()
	if second["fire_bolt"].Power != 1.20 {
		t.Errorf("the shared definition was mutated to power %v", second["fire_bolt"].Power)
	}
}

func TestCrossRealmSuppressionCoversEverySupportedDistance(t *testing.T) {
	attrs := BaseAttributes(Qi(6))
	for _, lower := range RealmOrder {
		for _, higher := range RealmOrder {
			gap := RealmGap(lower, higher)
			if gap > -2 {
				continue
			}
			if scale := CrossRealmDamageScale(lower, higher); scale != 0.10 {
				t.Fatalf("%s -> %s gap%d regained damage scale %g", lower, higher, gap, scale)
			}
			if chance := EscapeChance(attrs, attrs, lower, higher, 0, 0); chance != 0.05 {
				t.Fatalf("%s -> %s gap%d regained escape chance %g", lower, higher, gap, chance)
			}
		}
	}
}
