package cultivation

import "testing"

func startSecretPreparation(t *testing.T) CombatSession {
	t.Helper()
	session := twoWayBattle(t)
	session = withBladeInHand(session, "hero")
	session, _ = ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("meridian_secret_blade"), TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	return session
}

func TestChoosingAnotherTechniqueAbandonsPreparationInsteadOfDoubleActing(t *testing.T) {
	session := startSecretPreparation(t)
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))

	playerHits := 0
	for _, step := range outcome.Steps {
		if step.ActorID == "hero" && step.Resolution.Hit {
			playerHits++
		}
	}
	if playerHits != 1 {
		t.Fatalf("player landed %d hits in the completion exchange, want only the chosen ordinary attack", playerHits)
	}
	if _, pending := session.Preparations["hero"]; pending {
		t.Fatal("choosing another primary action must abandon the old preparation")
	}
	if session.CombatantByID("hero").Attributes.Mana != 96 {
		t.Fatalf("mana after abandoning should keep the already-paid cost without a refund, got %d", session.CombatantByID("hero").Attributes.Mana)
	}
}

func TestChoosingRestAbandonsPreparationWithoutCompletingIt(t *testing.T) {
	session := startSecretPreparation(t)
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))

	for _, step := range outcome.Steps {
		if step.ActorID == "hero" && step.SkillID == "meridian_secret_blade" && step.Resolution.Hit {
			t.Fatal("a rest action must not secretly complete the prepared secret technique")
		}
	}
	if _, pending := session.Preparations["hero"]; pending {
		t.Fatal("choosing rest must abandon the old preparation")
	}
	if got := session.CombatantByID("hero").Attributes.Mana; got != 104 {
		t.Fatalf("rest recovery after abandoning should be 104, got %d", got)
	}
}

func TestCancelPreparationUsesTheActionWithoutARefundOrFollowUp(t *testing.T) {
	session := startSecretPreparation(t)
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionCancelPreparation},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.Kind != ActionCancelPreparation || step.ReasonKey != "reason.preparation_abandoned" || !step.PreparationAbandoned {
		t.Fatalf("cancel outcome must clearly record abandonment: %+v", step)
	}
	if step.Resolution.Hit || step.ManaRecovered != 0 {
		t.Fatalf("canceling must not add a strike or rest for the player: %+v", step)
	}
	if len(session.Preparations) != 0 || session.CombatantByID("hero").Attributes.Mana != 96 {
		t.Fatalf("cancel must clear preparation without a refund: %+v", session)
	}
	if session.CombatantByID("foe").Attributes.Health != 200 {
		t.Fatal("canceling must not secretly attack the opponent")
	}
}

func TestInvalidActionDoesNotSilentlyContinueOrAbandonPreparation(t *testing.T) {
	session := startSecretPreparation(t)
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: "continue_with_guess"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey != "reason.unknown_action" {
		t.Fatalf("unknown action feedback = %+v", step)
	}
	if _, pending := session.Preparations["hero"]; !pending {
		t.Fatal("an invalid action must neither abandon nor complete the preparation")
	}
	if session.CombatantByID("foe").Attributes.Health != 200 {
		t.Fatal("an invalid action must not fall through to a free secret attack")
	}
}

func TestMissingActionDoesNotSilentlyContinueOrAbandonPreparation(t *testing.T) {
	session := startSecretPreparation(t)
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"foe": {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.ReasonKey != "reason.action_required" || step.SkillID != "meridian_secret_blade" {
		t.Fatalf("a missing choice must ask for an explicit preparation action: %+v", step)
	}
	if _, pending := session.Preparations["hero"]; !pending {
		t.Fatal("a missing action must preserve the paid preparation")
	}
	if session.CombatantByID("foe").Attributes.Health != 200 {
		t.Fatal("a missing action must not complete a free secret attack")
	}
}

func TestRealHealthDamageInterruptsContinuation(t *testing.T) {
	session := startSecretPreparation(t)
	foe := session.CombatantByID("foe")
	foe.Attributes.Attack = 500
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionContinuePreparation},
		"foe":  {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"},
	}, fixedDraws(0))
	for _, step := range outcome.Steps {
		if step.ActorID == "hero" && step.SkillID == "meridian_secret_blade" && step.Resolution.Hit {
			t.Fatal("a hit taken during continuation must interrupt the prepared strike")
		}
	}
	interrupted := false
	for _, step := range outcome.Steps {
		interrupted = interrupted || (step.ActorID == "hero" && step.ReasonKey == "reason.preparation_interrupted")
	}
	if !interrupted || len(session.Preparations) != 0 || session.CombatantByID("hero").Attributes.Mana != 96 {
		t.Fatalf("real damage must interrupt without a refund: session=%+v outcome=%+v", session, outcome)
	}
}

func TestContinuationWithMissingOriginalTargetResolvesNoTarget(t *testing.T) {
	session := startSecretPreparation(t)
	session.CombatantByID("foe").Fled = true
	session, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionContinuePreparation},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if step.SkillID != "meridian_secret_blade" || step.ReasonKey != "reason.no_target" || step.Resolution.Hit {
		t.Fatalf("continuation must remain bound to its now-unavailable target: %+v", step)
	}
	if len(session.Preparations) != 0 || session.CombatantByID("hero").Attributes.Mana != 96 {
		t.Fatalf("a no-target completion must clear the paid preparation without refund: %+v", session)
	}
}

// §9: 需要法器的技能从真实装备与控制条件确定资格，不能被默认false覆盖. The combatant's
// own HasImplement decides, and the declaration carries no say in it at all.
func TestTheFocusRequirementComesFromTheCombatantNotTheDeclaration(t *testing.T) {
	secret := mustSkill("meridian_secret_blade")

	// The blade really is in hand: the technique starts.
	armed := withBladeInHand(twoWayBattle(t), "hero")
	next, outcome := ResolveExchange(armed, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret, TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step := findStep(t, outcome, "hero")
	if !step.Cast.Allowed {
		t.Fatalf("a held focus must let the technique start: %+v", step.Cast)
	}
	if _, preparing := next.Preparations["hero"]; !preparing {
		t.Fatal("the held focus must record the preparation")
	}

	// The same declaration for the same combatant without the focus is refused
	// for the focus reason and spends nothing.
	bare := twoWayBattle(t)
	before := *bare.CombatantByID("hero")
	next, outcome = ResolveExchange(bare, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: secret, TargetID: "foe"},
		"foe":  {Kind: ActionRest},
	}, fixedDraws(0))
	step = findStep(t, outcome, "hero")
	if step.Cast.Allowed || step.Cast.ReasonKey != ReasonImplementRequired {
		t.Fatalf("a missing focus must refuse for the focus reason: %+v", step.Cast)
	}
	after := *next.CombatantByID("hero")
	if after.Attributes.Mana != before.Attributes.Mana ||
		after.Burden.Current != before.Burden.Current || after.Burden.Residual != before.Burden.Residual {
		t.Errorf("a focus refusal must spend nothing: mana %d→%d burden %+v→%+v",
			before.Attributes.Mana, after.Attributes.Mana, before.Burden, after.Burden)
	}
	if _, preparing := next.Preparations["hero"]; preparing {
		t.Error("a refused technique must not be recorded as begun")
	}
}

func TestEnemyContinuesItsPreparedTechniqueInsteadOfChoosingAnotherAction(t *testing.T) {
	session := twoWayBattle(t)
	session = withBladeInHand(session, "foe")
	secret := mustSkill("meridian_secret_blade")
	session.Preparations = map[string]Preparation{
		"foe": {Skill: secret, TargetID: "hero", StartedRound: 1},
	}
	session.Round = 2
	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Kind != ActionContinuePreparation {
		t.Fatalf("prepared enemy chose %q, want explicit continuation", action.Kind)
	}
	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionRest},
		"foe":  action,
	}, fixedDraws(0))
	step := findStep(t, outcome, "foe")
	if step.SkillID != secret.ID || !step.Resolution.Hit || len(next.Preparations) != 0 {
		t.Fatalf("enemy continuation must resolve one prepared attack and clear state: step=%+v session=%+v", step, next)
	}
}

func TestPreparationInterruptionUsesSingleLossThreshold(t *testing.T) {
	for _, test := range []struct {
		name            string
		loss            int
		wantInterrupted bool
	}{
		{name: "below threshold", loss: 39},
		{name: "exact threshold", loss: 40, wantInterrupted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := startSecretPreparation(t)
			var outcome ExchangeOutcome
			markPreparationInterruptions(&session, "hero", test.loss, map[string]bool{}, &outcome)
			_, stillPreparing := session.Preparations["hero"]
			if stillPreparing == test.wantInterrupted {
				t.Fatalf("single loss %d interruption state=%t, want interrupted=%t", test.loss, !stillPreparing, test.wantInterrupted)
			}
		})
	}
}

func TestPreparationInterruptionDoesNotAccumulateSmallHits(t *testing.T) {
	session := startSecretPreparation(t)
	interrupted := map[string]bool{}
	var outcome ExchangeOutcome
	markPreparationInterruptions(&session, "hero", 20, interrupted, &outcome)
	markPreparationInterruptions(&session, "hero", 20, interrupted, &outcome)
	if _, stillPreparing := session.Preparations["hero"]; !stillPreparing {
		t.Fatal("two separate 10-percent losses must not combine into an interruption")
	}
}
