package app

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover §三 and §四 of 战斗建议复核与NPC信息边界: the battle window is
// a player-facing projection, not the battle's own record.
//
// The rules keep the full truth so a fight can be settled and recovered, and the
// journal keeps every committed exchange. What the player is shown is narrower:
// their own character's real numbers, and of everyone else only what the fight
// has actually put in front of them — the techniques an opponent really
// performed, a dose they were seen to take, the lasting effects in plain sight,
// and a coarse visible condition. Nothing here may be the opponent's stored
// realm, pools, burden, learned set or carried stock, and the omission happens
// on the server: the browser never receives the value it is meant not to show.

// combatTestAppWithRivalCarrying seeds the rival's carried stock the same way
// the protagonist's is seeded, so a dose the opponent takes is a real one.
func combatTestAppWithRivalCarrying(t *testing.T, carried map[string]any, rivalSkills ...string) (*App, string) {
	t.Helper()
	system := combatTestSystem(t, rivalSkills...)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != "rival" {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestCarriedField] = carried
		// A stranger with no spiritual power could not perform the art this test
		// needs them to perform, so the pool is real here rather than absent.
		if _, ok := state[combatTestManaField]; !ok {
			state[combatTestManaField] = 200
		}
		// The rival starts wounded. §8 refuses a healing dose at a full pool, and
		// whether the opening strike lands is a real roll the rules own, so
		// leaving health at its unwritten full value would make the dose tests
		// depend on that roll rather than on the projection they are about. It
		// stays above §14.2's 30% retreat trigger, because these tests are about
		// what the window shows of an opponent that keeps fighting.
		if _, ok := state[combatTestHealthField]; !ok {
			state[combatTestHealthField] = float64(150)
		}
		system.InitialActors[index].State = state
	}
	return combatTestAppWithSystem(t, system)
}

// TestCombatWindowDoesNotShowAStrangersNumbers is the first acceptance item: an
// opponent the player has never learned anything about shows no realm, no pools,
// no attributes, no learned set and no carried stock — and the omission is in
// the payload rather than in the rendering.
func TestCombatWindowDoesNotShowAStrangersNumbers(t *testing.T) {
	application, storyID := combatTestAppWithRivalCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, "fire_bolt")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatWindowSide(t, window, "rival")
	if rival.Exact {
		t.Fatalf("a stranger's exact figures are not the player's to know: %#v", rival)
	}
	if rival.Realm != "" || rival.Health != nil || rival.HealthMax != nil ||
		rival.Mana != nil || rival.ManaMax != nil || rival.Attack != nil ||
		rival.Defense != nil || rival.Agility != nil || rival.Perception != nil ||
		rival.Burden != nil || rival.BurdenFloor != nil {
		t.Fatalf("the opponent's stored figures must not reach the player: %#v", rival)
	}
	if len(rival.Skills) != 0 {
		t.Fatalf("a stranger's learned set must not be listed: %#v", rival.Skills)
	}
	if len(rival.Items) != 0 {
		t.Fatalf("a stranger's carried stock must not be listed: %#v", rival.Items)
	}
	if rival.Retreat != "" {
		t.Fatalf("an opponent's retreat template is a rule, not a sight: %q", rival.Retreat)
	}
	if rival.Observed == nil || rival.Observed.Condition == "" {
		t.Fatalf("an opponent is still shown as a visible condition: %#v", rival.Observed)
	}
	// Unknown is not empty: the payload must not carry a zero the interface could
	// read as a settled value.
	if rival.Health != nil && *rival.Health == 0 {
		t.Fatal("an unknown pool must be absent, not zero")
	}
}

// TestCombatWindowShowsThePlayersOwnNumbersExactly is the other half of the
// boundary. The player's own character is not projected at all.
func TestCombatWindowShowsThePlayersOwnNumbersExactly(t *testing.T) {
	application, storyID := combatTestApp(t)
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if !player.Exact {
		t.Fatal("the player's own figures must be reported exactly")
	}
	if player.Observed != nil {
		t.Fatalf("the player's own side is not an observation: %#v", player.Observed)
	}
	if player.Health == nil || player.HealthMax == nil || player.Mana == nil || player.ManaMax == nil {
		t.Fatalf("the player's own pools must be reported: %#v", player)
	}
	if window.PlayerActorID != interactive.DefaultActorID {
		t.Fatalf("the window must name the actor whose actions the player composes, got %q", window.PlayerActorID)
	}
}

// TestCombatWindowRecordsOnlyTheTechniquesTheOpponentPerformed is the second
// acceptance item: one used technique adds one seen entry, and it does not open
// the opponent's learned set, its total count, its costs or its strength.
func TestCombatWindowRecordsOnlyTheTechniquesTheOpponentPerformed(t *testing.T) {
	application, storyID := combatTestAppWithRivalCarrying(t, nil,
		"fire_bolt", "meridian_secret_blade")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(combatWindowSide(t, opened, "rival").Observed.Techniques) != 0 {
		t.Fatal("nothing has been performed yet, so nothing has been seen")
	}
	// The rival acts in the second round. One performed art is one observation,
	// and the other art it knows stays unseen.
	after, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-watch",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatWindowSide(t, after, "rival")
	if len(rival.Skills) != 0 {
		t.Fatalf("the learned set must stay closed: %#v", rival.Skills)
	}
	if rival.Observed == nil || len(rival.Observed.Techniques) != 1 {
		t.Fatalf("the one technique the opponent performed must be the one entry: %#v", rival.Observed)
	}
	seen := rival.Observed.Techniques[0]
	if seen.Round != 1 {
		t.Fatalf("an observation must keep the round it was made in, got %d", seen.Round)
	}
	// The protagonist has not learned this art, so the player saw something
	// happen without being able to name it. The identifier is not sent either:
	// a skill_id is the definition's name in another form.
	if seen.Recognized {
		t.Fatalf("a technique the player never learned must not be identified: %#v", seen)
	}
	if seen.SkillID != "" || seen.NameKey != "" {
		t.Fatalf("an unidentified technique must carry no identifier: %#v", seen)
	}
}

// TestCombatWindowIdentifiesATechniqueThePlayerHasLearned is the other side of
// recognition: a real source of identification exists, so the technique is named
// then and only then.
func TestCombatWindowIdentifiesATechniqueThePlayerHasLearned(t *testing.T) {
	// Learning happens outside a battle, so the fixture for a learned art is the
	// learned set the projection reads. With nothing learned, nothing is
	// identified; with the same art learned, the technique is named.
	if seen := combatObservedTechnique("fire_bolt", map[string]cultivation.SkillDef{}); seen.Recognized {
		t.Fatal("an empty learned set cannot identify anything")
	}
	learned, ok := cultivation.SkillByID("fire_bolt")
	if !ok {
		t.Fatal("fire_bolt must be published")
	}
	seen := combatObservedTechnique("fire_bolt", map[string]cultivation.SkillDef{"fire_bolt": learned})
	if !seen.Recognized || seen.SkillID != "fire_bolt" || seen.NameKey == "" {
		t.Fatalf("a learned art must be identified: %#v", seen)
	}
}

// TestCombatWindowRecordsADoseAsAnActionNotAStock is the fourth acceptance item:
// watching an opponent take a pill records that they took one, not the pill, the
// amount or what else is in the bag.
func TestCombatWindowRecordsADoseAsAnActionNotAStock(t *testing.T) {
	application, storyID := combatTestAppWithRivalCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, "fire_bolt")
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		return `{"candidate_id":"item:回春丹"}`
	})
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(combatWindowSide(t, opened, "rival").Items) != 0 {
		t.Fatal("a stranger's bag must stay closed before anything is taken")
	}

	after, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, SessionID: opened.SessionID, CommandID: "cmd-dose",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatWindowSide(t, after, "rival")
	if len(rival.Items) != 0 {
		t.Fatalf("one visible dose does not open the bag: %#v", rival.Items)
	}
	if rival.Observed == nil || rival.Observed.Doses != 1 {
		t.Fatalf("the dose the player watched must be recorded as one: %#v", rival.Observed)
	}
}

// TestCombatWindowReportKeepsTheOpponentsCostsOutOfTheSettledRound covers the
// nested leak §四 names explicitly. The committed round is part of the payload,
// so its steps must be projected too: an opponent's technique identifier, its
// mana and burden cost, its restore amount and the reason a cast failed are all
// still the opponent's own business.
func TestCombatWindowReportKeepsTheOpponentsCostsOutOfTheSettledRound(t *testing.T) {
	application, storyID := combatTestAppWithRivalCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, "fire_bolt")
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		return `{"candidate_id":"item:回春丹"}`
	})
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, SessionID: opened.SessionID, CommandID: "cmd-dose",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	rivalSteps := 0
	for _, round := range after.History {
		for _, step := range round.Outcome.Steps {
			if step.ActorID != "rival" {
				continue
			}
			rivalSteps++
			if step.SkillID != "" {
				t.Fatalf("an opponent's technique or pill identity must not be in the round: %#v", step)
			}
			if step.Cast.ManaCost != 0 || step.Cast.BurdenCost != 0 {
				t.Fatalf("an opponent's cast cost must not be in the round: %#v", step.Cast)
			}
			if step.ManaRecovered != 0 || step.HealthRecovered != 0 {
				t.Fatalf("how much an opponent got back must not be in the round: %#v", step)
			}
		}
	}
	if rivalSteps == 0 {
		t.Fatal("the fixture must contain steps taken by the opponent")
	}
	// The player's own step is untouched, because it is the player's own action.
	playerSteps := 0
	for _, round := range after.History {
		for _, step := range round.Outcome.Steps {
			if step.ActorID != interactive.DefaultActorID {
				continue
			}
			playerSteps++
		}
	}
	if playerSteps == 0 {
		t.Fatal("the player's own step must survive the projection")
	}
}

// TestCombatNarrationIsNotToldTheOpponentsNumbers keeps the leak out of the
// prose as well. The passage is player-facing, so the facts it is written from
// cannot contain an opponent's pools, burden or technique identifiers.
func TestCombatNarrationIsNotToldTheOpponentsNumbers(t *testing.T) {
	session := cultivation.CombatSession{
		ID: "cbt_1", Round: 2,
		Combatants: []cultivation.Combatant{
			{ID: interactive.DefaultActorID, Side: cultivation.SidePlayer,
				Attributes: cultivation.Attributes{Health: 64, HealthMax: 80, Mana: 40, ManaMax: 60},
				Burden:     cultivation.BurdenState{Current: 3}},
			{ID: "rival", Side: cultivation.SideEnemy,
				Attributes: cultivation.Attributes{Health: 37, HealthMax: 120, Mana: 88, ManaMax: 90},
				Burden:     cultivation.BurdenState{Current: 11}},
		},
	}
	facts := combatNarrationFacts(
		cultivation.ExchangeOutcome{Round: 2, Order: []string{interactive.DefaultActorID}, Steps: []cultivation.ExchangeStep{
			{ActorID: "rival", TargetID: interactive.DefaultActorID, Kind: cultivation.ActionTechnique,
				SkillID: "fire_bolt", Cast: cultivation.CastCheck{Allowed: true, ManaCost: 24, BurdenCost: 4},
				Resolution: cultivation.AttackResolution{Hit: true, Damage: 9, HealthLost: 9}},
			{ActorID: "rival", Kind: cultivation.ActionUseItem, SkillID: "pill.restore.health.lower",
				ItemApplied: true, HealthRecovered: 20},
		}},
		session, doseNarrationCast(),
	)
	for _, leaked := range []string{
		"37 of 120", "88 of 90", "meridian burden 11",
		"fire_bolt", "pill.restore.health.lower", "recovered 20",
	} {
		if strings.Contains(facts, leaked) {
			t.Fatalf("the opponent's own figure %q reached the narration facts:\n%s", leaked, facts)
		}
	}
	// The player's own numbers are the player's, so they stay.
	if !strings.Contains(facts, "64 of 80") {
		t.Fatalf("the player's own pools must still be stated:\n%s", facts)
	}
	_ = character.State{}
}

// A variant is a published form of a technique the player has learned. §9's
// scope requires the forms §5 fixes to be castable, so the window has to offer
// them: a form the rules settle but the interface never shows is a capability
// the player paid for and cannot reach.
//
// The projection is the place this is decided, because it is the only thing the
// browser sees. The test therefore reads the window's own payload rather than
// asking the engine's table.
func TestTheWindowOffersThePublishedFormsOfALearnedTechnique(t *testing.T) {
	application, storyID := combatTestAppWithPlayerSkills(t, "fire_bolt", "heavy_thunder")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	skills := map[string]CombatWindowSkill{}
	for _, skill := range player.Skills {
		skills[skill.ID] = skill
	}

	fireBolt, ok := skills["fire_bolt"]
	if !ok {
		t.Fatalf("the learned set must list the fire bolt: %#v", player.Skills)
	}
	if len(fireBolt.Variants) != 1 {
		t.Fatalf("the fire bolt must offer exactly its burning form: %#v", fireBolt.Variants)
	}
	burn := fireBolt.Variants[0]
	if burn.ID != string(cultivation.VariantBurn) {
		t.Errorf("variant id = %q, want %q", burn.ID, cultivation.VariantBurn)
	}
	// §5 replaces the base power: the burning form casts at 1.00, not 1.20.
	if burn.Power != cultivation.BurnVariantPower {
		t.Errorf("burning power = %v, want %v", burn.Power, cultivation.BurnVariantPower)
	}
	if burn.NameKey == "" {
		t.Error("a form the window offers must be nameable")
	}

	thunder, ok := skills["heavy_thunder"]
	if !ok {
		t.Fatalf("the learned set must list the thunder technique: %#v", player.Skills)
	}
	if len(thunder.Variants) != 1 || thunder.Variants[0].ID != string(cultivation.VariantSlow) {
		t.Fatalf("the thunder technique must offer exactly its slowing form: %#v", thunder.Variants)
	}
	if thunder.Variants[0].Power != cultivation.SlowVariantPower {
		t.Errorf("slowing power = %v, want %v", thunder.Variants[0].Power, cultivation.SlowVariantPower)
	}

	// A technique §5 gave no form must not acquire one, or the window would offer
	// a cast the engine refuses with reason.variant_unsupported.
	if plain, ok := skills["strike"]; ok && len(plain.Variants) != 0 {
		t.Errorf("the plain attack has no published form: %#v", plain.Variants)
	}
}

// A generated technique keeps the form its template publishes, and the form is
// worded by the interface rather than by a key derived from an identity no
// locale can know.
//
// The key the rules derive for a published form is `skill.<id>.variant.<form>`.
// A generated id is `gen.<template>.<name>`, so that key can never resolve and
// sending it would invite a translation that does not exist. The form itself is a
// published fact, so the window words it generically instead — and the test
// asserts the generated entry carries no key at all, which is the fact the
// interface reads.
func TestAGeneratedTechniqueKeepsItsFormWithoutAnUntranslatableKey(t *testing.T) {
	template, ok := cultivation.SkillTemplateByID("fire_bolt")
	if !ok {
		t.Fatal("fire_bolt must be an approved template")
	}
	generated, err := template.Instantiate("赤霞弹")
	if err != nil {
		t.Fatal(err)
	}
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		switch system.InitialActors[index].ID {
		case interactive.DefaultActorID:
			state := system.InitialActors[index].State
			if state == nil {
				state = map[string]any{}
			}
			state[combatTestSkillsField] = map[string]any{
				generated.ID: map[string]any{
					"定义": generated.ID, "版本": float64(1), "掌握程度": "入门",
				},
			}
			system.InitialActors[index].State = state
		case interactive.DefaultStoryContextActorID:
			// The definition has to be registered in this story, or the battle
			// cannot resolve the identity the learned record names. The record is
			// the same shape the learning settlement writes.
			state := system.InitialActors[index].State
			if state == nil {
				state = map[string]any{}
			}
			state["技能定义"] = map[string]any{
				generated.ID: map[string]any{
					"定义": generated.ID, "版本": float64(1),
					"模板": "fire_bolt", "名称": "赤霞弹",
					"品级": "普通", "稀有度": "常见",
				},
			}
			system.InitialActors[index].State = state
		}
	}
	application, storyID := combatTestAppWithSystem(t, system)

	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	var listed *CombatWindowSkill
	for index := range player.Skills {
		if player.Skills[index].ID == generated.ID {
			listed = &player.Skills[index]
			break
		}
	}
	if listed == nil {
		t.Fatalf("the window must list the generated technique: %#v", player.Skills)
	}
	if listed.Name != "赤霞弹" {
		t.Fatalf("the window must show the story's own name: %q", listed.Name)
	}
	if listed.Power != template.Definition.Power {
		t.Fatalf("power = %v, want the template's %v", listed.Power, template.Definition.Power)
	}
	if len(listed.Variants) != 1 {
		t.Fatalf("the generated technique must keep its template's form: %#v", listed.Variants)
	}
	if listed.Variants[0].ID != string(cultivation.VariantBurn) {
		t.Fatalf("variant = %q, want the template's %q", listed.Variants[0].ID, cultivation.VariantBurn)
	}
	if listed.Variants[0].NameKey != "" {
		t.Fatalf("a generated form has no translatable key, got %q", listed.Variants[0].NameKey)
	}
	if listed.Variants[0].Power != cultivation.BurnVariantPower {
		t.Fatalf("variant power = %v, want %v", listed.Variants[0].Power, cultivation.BurnVariantPower)
	}
}

// A multi-target cast is one action with several struck opponents, and each of
// them is a surface too. §四's rule does not change with the number of targets:
// the player may know a companion was wounded, never how much it cost them, so
// the projected round keeps only the hit and the visible lasting effect.
func TestTheProjectionWithholdsAnOpponentsExtraTargetAmounts(t *testing.T) {
	outcome := cultivation.ExchangeOutcome{Steps: []cultivation.ExchangeStep{{
		ActorID: "rival", TargetID: interactive.DefaultActorID, Kind: cultivation.ActionTechnique,
		SkillID: "flame_wave", Cast: cultivation.CastCheck{Allowed: true, ManaCost: 36, BurdenCost: 30},
		Resolution: cultivation.AttackResolution{Hit: true, Damage: 20, HealthLost: 20},
		ExtraTargets: []cultivation.ExtraTargetStrike{{
			TargetID:    "companion",
			Resolution:  cultivation.AttackResolution{Hit: true, Damage: 21, HealthLost: 21},
			BurnApplied: true,
		}},
	}}}
	projected := combatWindowProjectedOutcome(outcome, map[string]bool{})
	step := projected.Steps[0]
	if step.SkillID != "" || step.Cast.ManaCost != 0 {
		t.Fatalf("the opponent's own identity and cost must be withheld: %#v", step)
	}
	if len(step.ExtraTargets) != 1 {
		t.Fatalf("the extra target must survive the projection: %#v", step.ExtraTargets)
	}
	extra := step.ExtraTargets[0]
	if !extra.Resolution.Hit {
		t.Error("whether the blow landed is visible")
	}
	if extra.Resolution.Damage != 0 || extra.Resolution.HealthLost != 0 {
		t.Errorf("the amount an opponent's extra target lost must be withheld: %#v", extra.Resolution)
	}
	if !extra.BurnApplied {
		t.Error("a wound visibly catching fire is something the player saw")
	}
}

// TestTheProjectionKeepsAnOpponentsRemainingAmountsOutOfTheRound covers the
// remaining fields of a committed step, which are surfaces too.
//
// A step is sent whole for the player's own side and must be narrowed for an
// opponent. The player watched an art land and a wound catch fire; they did not
// read what it cost the opponent to drive an implement, why a cast was refused,
// or how much of a lasting effect is left. §四 puts the whole nested payload in
// scope, so the test reads the encoded round rather than the rendering.
func TestTheProjectionKeepsAnOpponentsRemainingAmountsOutOfTheRound(t *testing.T) {
	outcome := cultivation.ExchangeOutcome{Round: 2, Steps: []cultivation.ExchangeStep{
		{
			ActorID: "rival", TargetID: interactive.DefaultActorID, Kind: cultivation.ActionTechnique,
			SkillID: "fire_bolt", Cast: cultivation.CastCheck{Allowed: true, ManaCost: 24, BurdenCost: 4},
			OperatingManaCost: 7,
			Resolution:        cultivation.AttackResolution{Hit: true, Damage: 9, HealthLost: 9},
			BurnApplied:       true, SlowApplied: true,
		},
		{
			ActorID: "rival", Kind: cultivation.ActionTechnique,
			SkillID: "heavy_thunder", Cast: cultivation.CastCheck{Allowed: false, ReasonKey: cultivation.ReasonInsufficientMana},
			ReasonKey: cultivation.ReasonInsufficientMana,
		},
	}}
	projected := combatWindowProjectedOutcome(outcome, map[string]bool{interactive.DefaultActorID: true})
	for _, step := range projected.Steps {
		if step.OperatingManaCost != 0 {
			t.Fatalf("what an opponent's implement cost to drive is not the player's to know: %#v", step)
		}
		if step.ReasonKey != "" {
			t.Fatalf("an opponent's blocking reason names its own pools: %#v", step)
		}
		if step.Cast.ReasonKey != "" || step.Cast.ManaCost != 0 || step.Cast.BurdenCost != 0 {
			t.Fatalf("an opponent's cast check is its own business: %#v", step.Cast)
		}
	}
	// The player still sees the consequence: the blow landed, the player's own
	// loss is exact, and the wound is visibly burning and slowing.
	kept := projected.Steps[0]
	if !kept.Resolution.Hit || !kept.BurnApplied || !kept.SlowApplied {
		t.Fatalf("the visible consequence must survive the projection: %#v", kept)
	}
	if kept.Resolution.Damage != 9 || kept.Resolution.HealthLost != 9 {
		t.Fatalf("the player's own loss is exact and must survive: %#v", kept.Resolution)
	}

	// The player's own step keeps every figure, because they are the player's.
	own := combatWindowProjectedOutcome(cultivation.ExchangeOutcome{Round: 2, Steps: []cultivation.ExchangeStep{{
		ActorID: interactive.DefaultActorID, TargetID: "rival", Kind: cultivation.ActionTechnique,
		SkillID: "fire_bolt", Cast: cultivation.CastCheck{Allowed: true, ManaCost: 24, BurdenCost: 4},
		OperatingManaCost: 7, Resolution: cultivation.AttackResolution{Hit: true, Damage: 9, HealthLost: 9},
	}}}, map[string]bool{interactive.DefaultActorID: true})
	step := own.Steps[0]
	if step.SkillID != "fire_bolt" || step.Cast.ManaCost != 24 || step.OperatingManaCost != 7 {
		t.Fatalf("the player's own step must keep its own figures: %#v", step)
	}
}

// TestTheWindowShowsAnOpponentsLastingEffectsAsPresenceNotAsCounts is the second
// half of the same boundary: the panel may say a shield is turning blows aside,
// but the size of that shield and how many exchanges it has left are the
// opponent's own numbers.
//
// The observation carries the facts, so the omission has to happen where the
// observation is built rather than in the panel.
func TestTheWindowShowsAnOpponentsLastingEffectsAsPresenceNotAsCounts(t *testing.T) {
	session := cultivation.CombatSession{
		ID: "cbt_1", Round: 2,
		Combatants: []cultivation.Combatant{
			{ID: interactive.DefaultActorID, Side: cultivation.SidePlayer},
			{ID: "rival", Side: cultivation.SideEnemy,
				Shield: cultivation.Shield{Amount: 40, RemainingExchanges: 2},
				Burn:   cultivation.DamageOverTime{PerTick: 6, RemainingTicks: 2},
				Slow:   cultivation.SlowEffect{AgilityPenalty: 0.15, RemainingTargetActions: 1}},
		},
	}
	observations := combatWindowObservations(session, nil, nil)
	observed := observations["rival"]
	if observed == nil {
		t.Fatal("an opponent in the fight must be observed")
	}
	if !observed.Shield || !observed.BurnTicks || !observed.SlowActions {
		t.Fatalf("an effect plainly in sight must be reported as present: %#v", observed)
	}
	encoded, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	// The presence is one flag each. The exact size, the remaining exchanges and
	// the agility penalty are the opponent's own figures and must not be in the
	// payload at all — the panel's wording is already presence-only, so a number
	// here would be a figure the server sent and the interface merely declined
	// to print.
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"shield", "burn_ticks", "slow_actions"} {
		value, present := decoded[field]
		if !present {
			t.Fatalf("%s must be present so the panel can say the effect is there", field)
		}
		if _, isNumber := value.(float64); isNumber {
			t.Fatalf("%s must be a presence, not a count the player cannot see: %s", field, encoded)
		}
	}
	for _, leaked := range []string{"40", "per_tick", "agility_penalty", "remaining_exchanges", "remaining_ticks", "remaining_target_actions"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("the opponent's own effect figure %q reached the payload: %s", leaked, encoded)
		}
	}
}
