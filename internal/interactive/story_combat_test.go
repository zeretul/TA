package interactive

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ta/internal/cultivation"
)

// These tests exercise the story-side half of the battle: the side record on the
// canonical journal, its paired settlement, and the two properties §七.2 and
// §七.8 require of it — that a retry re-uses the exchange it already settled, and
// that a settled outcome stands.

// combatTestSystem is the real xiuxian preset with a second tracked template, so
// an exchange has two participants whose numbers the rules own.
//
// The starting realm is seeded through the initial actor rather than through a
// turn's state updates. 当前境界 is a settlement-owned field, and
// IsXiuxianSettledField refuses a model write to it for an actor that already
// exists; a character's first materialisation is the one place the rules allow
// the stage to arrive as a story fact.
func combatTestSystem() StoryDirectorActorStateSystem {
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianLearnedSkillsField())
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: fields},
		{ID: "rival", Name: "对手", Fields: append([]ActorStateField{}, fields...)},
		// The story-context actor owns the record of techniques this story
		// generated, so a battle fought by a character who invented one can read it.
		{ID: ActorStateStoryContextTemplateID, Name: "故事状态", Fields: []ActorStateField{xiuxianGeneratedSkillsField()}},
	}}
	system.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: stage}},
		{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{xiuxianFieldRealm: stage}},
		{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
	}
	return system
}

// combatTestStory builds a story whose cast is already settled, so a battle has
// real numbers to read rather than the zero a fresh story starts at.
func combatTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// combatTestSpecs declares the two sides with a real route out of the fight, so
// the retreat templates have something to evaluate against.
func combatTestSpecs() []CombatantSpec {
	return []CombatantSpec{
		{ActorID: DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
		{ActorID: "rival", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
	}
}

func combatTestState(t *testing.T, store *Store, storyID string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.State
}

func combatTestSystemFromStore(t *testing.T, store *Store, storyID string) StoryDirectorActorStateSystem {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil {
		t.Fatal("the story must have a frozen actor-state schema")
	}
	return actorStateSystemFromSnapshot(snapshot.ActorStateSchema, StoryDirectorActorStateSystem{})
}

func combatTestStrike(t *testing.T) cultivation.SkillDef {
	t.Helper()
	definition, ok := storySkillRegistry(cultivation.ReferenceRules()).Current("strike")
	if !ok {
		t.Fatal("the rules' published table must contain the basic strike")
	}
	return definition
}

func combatTestProjection(t *testing.T, store *Store, storyID string) *storyBranchProjection {
	t.Helper()
	return combatTestProjectionForBranch(t, store, storyID, "main")
}

// combatTestExchange is the one exchange every lifecycle test commits: the
// protagonist strikes the rival with the basic technique.
func combatTestExchange(t *testing.T, store *Store, storyID, commandID, hash string) cultivation.CombatSession {
	t.Helper()
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-1", commandID, combatTestSpecs(), nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: session, Outcome: outcome,
		CommandID: commandID, CommandHash: hash,
	}); err != nil {
		t.Fatal(err)
	}
	return session
}

// TestNewXiuxianStoryOpensAtTheStageTablesRealPools is the regression test for
// the pool defaults.
//
// A pool field must never carry a default of zero. Zero is the one value that
// means "already down", so materializing it at story creation makes the
// settlement's own guard — an unwritten pool starts full — unable to tell an
// untouched pool from an emptied one, and every new story opens with its
// protagonist at 0 health and 0 mana. This asserts the opposite: a freshly
// created character who has never been wounded is at the stage table's real
// maximum, and is so before the first turn is written.
func TestNewXiuxianStoryOpensAtTheStageTablesRealPools(t *testing.T) {
	system := combatTestSystem()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "初始生命", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}

	// The creation-time settlement is what fills these, so the pre-opening
	// snapshot the player sees already carries the real numbers. Leaving them
	// unwritten until the first turn was the earlier shape, and it showed a
	// qi-refining character at 0/0 while the prose said they were a cultivator.
	created := combatTestState(t, store, story.ID)
	for fieldID, want := range map[string]float64{
		xiuxianFieldHealth:    200,
		xiuxianFieldMana:      160,
		xiuxianFieldHealthMax: 200,
		xiuxianFieldManaMax:   160,
	} {
		got, ok := actorStateNumber(actorStateFieldValue(created, DefaultActorID, fieldID))
		if !ok {
			t.Fatalf("a fresh story must already carry %s, got nothing", fieldID)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", fieldID, got, want)
		}
	}

	// The first turn settles the cast. Layer 6 is fixed by the rules at 200
	// health and 160 mana; anything else means the character was materialized at
	// zero and then clamped to zero.
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角睁眼。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	settled := combatTestState(t, store, story.ID)
	for fieldID, want := range map[string]float64{
		xiuxianFieldHealth:    200,
		xiuxianFieldMana:      160,
		xiuxianFieldHealthMax: 200,
		xiuxianFieldManaMax:   160,
	} {
		got, ok := actorStateNumber(actorStateFieldValue(settled, DefaultActorID, fieldID))
		if !ok {
			t.Fatalf("the settlement must write %s, got nothing", fieldID)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", fieldID, got, want)
		}
	}
}

// TestCombatExchangeIsSettledFromTheActorsOwnNumbers is §七.1's floor: the window
// can only show real attributes if the battle was built from the state that is
// actually stored.
//
// It checks the three things separately rather than reading one number off the
// settled session, because ResolveCombatExchange returns the session *after* the
// exchange resolved: entry numbers, the layer-6 baseline, and the post-round
// result are three different facts and a single equality would conflate them.
func TestCombatExchangeIsSettledFromTheActorsOwnNumbers(t *testing.T) {
	store, storyID := combatTestStory(t)
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)

	// Entry: the session as the cast entered it, built by the same function the
	// production path builds it with. Resolving an exchange with no declared
	// actions would NOT give this — a combatant who declares nothing still acts,
	// and the rules give them an ordinary strike — so the entry numbers are read
	// off the constructed session rather than off a settled one.
	//
	// Layer 6 is fixed by the rules at 200 health, 160 mana and 40 attack, and a
	// battle built from anywhere other than the character's own record would not
	// land on those numbers.
	entry, err := combatSessionFromState(state, system, "cbt-1", combatTestSpecs())
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Combatants) != 2 {
		t.Fatalf("a battle must carry both sides: %#v", entry.Combatants)
	}
	entryProtagonist := combatTestCombatant(t, entry, DefaultActorID)
	entryProtagonistBurden := entryProtagonist.Burden
	for _, combatant := range entry.Combatants {
		attributes := combatant.Attributes
		if attributes.HealthMax != 200 || attributes.ManaMax != 160 {
			t.Fatalf("combatant %s must enter at the stage's real pools: %#v", combatant.ID, attributes)
		}
		if attributes.Health != attributes.HealthMax || attributes.Mana != attributes.ManaMax {
			t.Fatalf("an untouched combatant %s must enter at full: %#v", combatant.ID, attributes)
		}
		if attributes.Attack != 40 {
			t.Fatalf("combatant %s must carry the stage's real attack: %#v", combatant.ID, attributes)
		}
	}

	// Resolved: now the protagonist strikes. The post-round numbers must agree
	// with the steps that actually happened, not with a hoped-for total.
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", combatTestSpecs(), nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Round != 1 || len(outcome.Steps) == 0 {
		t.Fatalf("the first exchange must settle: %#v", outcome)
	}

	// The maxima are a property of the stage, so they survive the exchange
	// unchanged even when the pools themselves move.
	for _, combatant := range session.Combatants {
		if combatant.Attributes.HealthMax != 200 || combatant.Attributes.ManaMax != 160 {
			t.Fatalf("combatant %s must keep the stage's maxima after the round: %#v", combatant.ID, combatant.Attributes)
		}
	}

	// Each pool is the maximum less exactly what the steps took from that actor.
	// Deriving the expectation from the outcome is what keeps this a check on the
	// arithmetic rather than a hard-coded 170 that would bless any single result.
	healthLost := map[string]int{}
	manaSpent := map[string]int{}
	for _, step := range outcome.Steps {
		if step.Resolution.Hit {
			healthLost[step.TargetID] += step.Resolution.HealthLost
		}
		manaSpent[step.ActorID] += step.Cast.ManaCost
	}
	for _, combatant := range session.Combatants {
		wantHealth := 200 - healthLost[combatant.ID]
		if combatant.Attributes.Health != wantHealth {
			t.Errorf("combatant %s health = %d, want %d (200 less the %d the steps took)",
				combatant.ID, combatant.Attributes.Health, wantHealth, healthLost[combatant.ID])
		}
		wantMana := 160 - manaSpent[combatant.ID]
		if combatant.Attributes.Mana != wantMana {
			t.Errorf("combatant %s mana = %d, want %d (160 less the %d spent)",
				combatant.ID, combatant.Attributes.Mana, wantMana, manaSpent[combatant.ID])
		}
	}

	// The basic strike is published at zero mana and zero burden, so the caster's
	// mana is untouched by the cast itself, and the exchange's own quiet
	// regulation relieves burden by the rules' own rate. Both expectations are
	// read off the published rules rather than written as a literal: asserting
	// "burden went down by 6" would demand a relief the rules floor away when the
	// character was already at rest.
	strike := combatTestStrike(t)
	if strike.ManaCost(cultivation.Qi(6)) != 0 || strike.Burden != 0 {
		t.Fatalf("the fixture assumes a free basic strike: mana=%d burden=%d",
			strike.ManaCost(cultivation.Qi(6)), strike.Burden)
	}
	protagonist := combatTestCombatant(t, session, DefaultActorID)
	if protagonist.Attributes.Mana != 160 {
		t.Errorf("a free strike must leave the protagonist's mana at 160, got %d", protagonist.Attributes.Mana)
	}
	if want := cultivation.RecoverBurdenPerExchange(entryProtagonistBurden, true).Current; protagonist.Burden.Current != want {
		t.Errorf("one exchange of quiet regulation must relieve burden from %d to %d, got %d",
			entryProtagonistBurden.Current, want, protagonist.Burden.Current)
	}
	// The rival only took damage, so their pool is the one that must have moved.
	rival := combatTestCombatant(t, session, "rival")
	if rival.Attributes.Health >= 200 {
		t.Errorf("a landed strike must have cost the rival health: %#v", rival.Attributes)
	}
	if healthLost["rival"] == 0 {
		t.Fatal("the fixture must actually land a hit for this test to mean anything")
	}
}

// TestCombatRecordCommitsItsSettlementInOneTransaction proves the record and the
// wounds it produced reach the journal together, which is what makes a battle
// either exist with its consequences or not at all.
func TestCombatRecordCommitsItsSettlementInOneTransaction(t *testing.T) {
	store, storyID := combatTestStory(t)
	session := combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	projection := combatTestProjection(t, store, storyID)
	committed, ok := projection.Combats["cbt-1"]
	if !ok {
		t.Fatalf("the battle must be live after the commit: %#v", projection.Combats)
	}
	if committed.Round != session.Round {
		t.Fatalf("the projected battle must be the settled one: %#v", committed)
	}

	// The wounds must have reached the save, not only the record.
	after := combatTestState(t, store, storyID)
	projectedRival := combatTestCombatant(t, committed, "rival")
	storedHealth, ok := actorStateNumber(actorStateFieldValue(after, "rival", xiuxianFieldHealth))
	if !ok {
		t.Fatal("the battle must have written the rival's health")
	}
	if int(storedHealth) != projectedRival.Attributes.Health {
		t.Fatalf("stored health %v must equal the settled %d", storedHealth, projectedRival.Attributes.Health)
	}
	if int(storedHealth) >= 200 {
		t.Fatalf("a landed strike must have cost the rival health, got %v", storedHealth)
	}
	// The settlement re-derives the maxima from the stage the character is still
	// at, so the panel and the battle read one set of numbers.
	if got, ok := actorStateNumber(actorStateFieldValue(after, "rival", xiuxianFieldHealthMax)); !ok || got != 200 {
		t.Fatalf("the battle must leave the derived maximum in place, got %v", actorStateFieldValue(after, "rival", xiuxianFieldHealthMax))
	}
}

// TestCombatRoundAcceptsATechniqueThisStoryGenerated is §6 reaching the battle:
// a technique the story invented from a template, registered in the story's own
// record, is declared by name alone and settled from the definition the story
// holds.
//
// It also checks the other half: the same declaration is refused for a technique
// the actor never learned, so a generated ID cannot become a way to cast anything.
func TestCombatRoundAcceptsATechniqueThisStoryGenerated(t *testing.T) {
	store, storyID := combatTestStory(t)
	system := combatTestSystemFromStore(t, store, storyID)
	state := combatTestState(t, store, storyID)

	// Generate and learn the technique through the real learning path, so the
	// battle reads the same records a played turn would have written.
	learningOps, learned, ok := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{
		Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: "赤霞弹", Source: "《赤霞诀》完整火弹篇", Minutes: 1440}},
	}, "ev-skill")
	if !ok || len(learned.Learned) != 1 {
		t.Fatalf("the technique must be generated and learned: %#v", learned)
	}
	generated := learned.Learned[0]
	state = appliedXiuxianState(t, state, learningOps)
	system = combatTestSystemFromStore(t, store, storyID)

	// The declaration names the identity and nothing else, which is exactly what
	// the API layer produces for a generated name.
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-gen", "cmd-gen", combatTestSpecs(), nil,
		map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: generated.ID}, TargetID: "rival"},
		})
	if err != nil {
		t.Fatalf("a generated technique the actor learned must be declarable: %v", err)
	}
	if outcome.Round != 1 || len(outcome.Steps) == 0 {
		t.Fatalf("the exchange must settle: %#v", outcome)
	}
	// The settled numbers must be the template's, not the empty reference's.
	var cast bool
	for _, step := range outcome.Steps {
		if step.ActorID == DefaultActorID && step.SkillID == generated.ID {
			cast = true
		}
	}
	if !cast {
		t.Fatalf("the round must record the generated technique as what was cast: %#v", outcome.Steps)
	}
	if player := combatTestCombatant(t, session, DefaultActorID); player.Attributes.Mana >= 160 {
		t.Fatalf("the round must have charged the technique's mana: %#v", player.Attributes)
	}

	// A name the actor never learned is refused rather than silently resolved.
	_, _, err = ResolveCombatExchange(state, system, "cbt-gen-2", "cmd-gen-2", combatTestSpecs(), nil,
		map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: cultivation.GeneratedSkillID("fire_bolt", "没有学过")}, TargetID: "rival"},
		})
	if !errors.Is(err, ErrCombatTechniqueNotLearned) {
		t.Fatalf("an unlearned generated technique must be refused, got %v", err)
	}
}

func combatTestCombatant(t *testing.T, session cultivation.CombatSession, actorID string) cultivation.Combatant {
	t.Helper()
	for _, combatant := range session.Combatants {
		if combatant.ID == actorID {
			return combatant
		}
	}
	t.Fatalf("battle has no combatant %s: %#v", actorID, session.Combatants)
	return cultivation.Combatant{}
}

// TestCombatRecordIsIdempotentOnItsCommand is §七.2: a refresh or a retry must
// re-use the exchange already settled rather than resolving a second one, so a
// player cannot click again to fish for a better round.
func TestCombatRecordIsIdempotentOnItsCommand(t *testing.T) {
	store, storyID := combatTestStory(t)
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)

	session, outcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", combatTestSpecs(), nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: session, Outcome: outcome,
		CommandID: "cmd-1", CommandHash: "hash-1",
	}
	first, err := store.AppendCombatRecord(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.SettlementOps) == 0 {
		t.Fatal("an exchange that dealt damage must settle at least one pool")
	}
	for _, op := range first.SettlementOps {
		if op.SourceKind != StateOpSourceCombatSettlement {
			t.Fatalf("a battle's writes must be attributable to the battle: %#v", op)
		}
	}
	second, err := store.AppendCombatRecord(storyID, request)
	if err != nil {
		t.Fatalf("a retry of the same command must be a read, not a failure: %v", err)
	}
	if second.EventID != first.EventID {
		t.Fatalf("a retry must return the first commit: %q vs %q", second.EventID, first.EventID)
	}
	if second.Session.Round != first.Session.Round {
		t.Fatalf("a retry must not advance the battle: %#v vs %#v", second.Session, first.Session)
	}

	// The journal must hold one record, not two. A second record would be a
	// second fight, which is what the command identity exists to prevent.
	if count := combatTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("a retried command must commit exactly one battle record, got %d", count)
	}

	// A cold store over the same directory must reach the same answer, because
	// the journal is what remembers the command and not the process.
	cold := NewStore(store.Root())
	replayed, found, err := cold.CombatRecordForCommand(storyID, "main", "cmd-1", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || replayed.EventID != first.EventID {
		t.Fatalf("a cold store must find the committed battle: found=%v %#v", found, replayed)
	}
}

// TestCombatRecordRefusesAReusedCommandForADifferentExchange is the other half of
// §七.2: one command identity may not be spent on two different fights, because
// either answer would misreport what happened.
func TestCombatRecordRefusesAReusedCommandForADifferentExchange(t *testing.T) {
	store, storyID := combatTestStory(t)
	combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", combatTestSpecs(), nil, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", Session: session, Outcome: outcome,
		CommandID: "cmd-1", CommandHash: "hash-2",
	})
	if !errors.Is(err, ErrCombatCommandConflict) {
		t.Fatalf("reusing a command for another exchange must conflict, got %v", err)
	}
	if count := combatTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("a refused command must not commit a second record, got %d", count)
	}
}

// TestCombatRecordSurvivesAJournalRebuild is the single-journal guarantee: a
// battle is a projection of the canonical journal, so closing and reopening the
// store reproduces it rather than losing it.
func TestCombatRecordSurvivesAJournalRebuild(t *testing.T) {
	store, storyID := combatTestStory(t)
	session := combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	rebuilt := NewStore(store.Root())
	projection := combatTestProjection(t, rebuilt, storyID)
	replayed, ok := projection.Combats["cbt-1"]
	if !ok {
		t.Fatalf("a rebuilt journal must reproduce the battle: %#v", projection.Combats)
	}
	if replayed.Round != session.Round || len(replayed.Combatants) != len(session.Combatants) {
		t.Fatalf("the replayed battle must be the committed one:\n got %#v\nwant %#v", replayed, session)
	}
	for _, combatant := range session.Combatants {
		if combatTestCombatant(t, replayed, combatant.ID).Attributes.Health != combatant.Attributes.Health {
			t.Fatalf("combatant %s must replay at the settled health: %#v", combatant.ID, replayed)
		}
	}
}

// TestCombatRecordCarriesThroughARewind keeps the window honest after the story
// is rewound: the pools come back with the state checkpoint, so the fight that
// produced them has to come back too, or the window would show wounds with no
// battle behind them.
func TestCombatRecordCarriesThroughARewind(t *testing.T) {
	store, storyID := combatTestStory(t)
	combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	// A second turn makes the battle something other than the latest event, so
	// the rewind has to carry it explicitly rather than inherit it as "now".
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "主角收剑后退。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := combatTestProjection(t, store, storyID).Combats["cbt-1"]; !ok {
		t.Fatal("a later turn must not drop the battle")
	}

	if err := store.RewindToTurnParent(storyID, RewindTurnRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	after := combatTestProjection(t, store, storyID)
	if _, ok := after.Combats["cbt-1"]; !ok {
		t.Fatalf("a rewind must carry the battle with the state it restored: %#v", after.Combats)
	}
}

// TestCombatRecordCarriesThroughAFork is the same guarantee for a fork: a branch
// cut from a turn inherits the fights that were live at that turn, and the new
// branch must not lose them.
func TestCombatRecordCarriesThroughAFork(t *testing.T) {
	store, storyID := combatTestStory(t)
	combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	branch, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: combatTestLatestTurnID(t, store, storyID),
		Title:         "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}
	forked := combatTestProjectionForBranch(t, store, storyID, branch.ID)
	if _, ok := forked.Combats["cbt-1"]; !ok {
		t.Fatalf("a fork of the battle's turn must inherit the fight: %#v", forked.Combats)
	}
}

// TestCombatRecordRejectsABattleTheRulesCannotAccountFor guards the journal: a
// record with no combatants or an unknown side would put something in front of
// the player that no rule produced.
func TestCombatRecordRejectsABattleTheRulesCannotAccountFor(t *testing.T) {
	store, storyID := combatTestStory(t)
	for name, session := range map[string]cultivation.CombatSession{
		"no id":         {Round: 1, Combatants: []cultivation.Combatant{{ID: "rival", Side: cultivation.SideEnemy}}},
		"no combatants": {ID: "cbt-bad", Round: 1},
		"zero round":    {ID: "cbt-bad", Round: 0, Combatants: []cultivation.Combatant{{ID: "rival", Side: cultivation.SideEnemy}}},
		"bad side":      {ID: "cbt-bad", Round: 1, Combatants: []cultivation.Combatant{{ID: "rival", Side: "neutral"}}},
		"duplicate": {ID: "cbt-bad", Round: 1, Combatants: []cultivation.Combatant{
			{ID: "rival", Side: cultivation.SideEnemy}, {ID: "rival", Side: cultivation.SidePlayer},
		}},
	} {
		if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
			BranchID: "main", Session: session,
		}); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if count := combatTestRecordCount(t, store, storyID); count != 0 {
		t.Fatalf("a refused battle must not reach the journal, got %d records", count)
	}
}

// TestCombatSessionRefusesAnUntrackedParticipant is §七.1's other side: the window
// has to show real attributes for both sides, so a participant the rules do not
// track is refused rather than given a row of the stage table it never had.
func TestCombatSessionRefusesAnUntrackedParticipant(t *testing.T) {
	store, storyID := combatTestStory(t)
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)

	if _, _, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", []CombatantSpec{
		{ActorID: DefaultActorID, Side: cultivation.SidePlayer},
		{ActorID: "absent", Side: cultivation.SideEnemy},
	}, nil, nil); err == nil {
		t.Fatal("a combatant with no resolvable template must be refused")
	}
	if _, _, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", []CombatantSpec{
		{ActorID: DefaultActorID, Side: cultivation.SidePlayer},
		{ActorID: DefaultActorID, Side: cultivation.SideEnemy},
	}, nil, nil); err == nil {
		t.Fatal("the same actor on both sides must be refused")
	}
}

// TestCombatDrawsAreBoundToTheCommand is §七.2 at the level the property actually
// lives at: the draws are a function of the command identity, so the same
// exchange replayed resolves to the same numbers and a different command may
// resolve to different ones.
func TestCombatDrawsAreBoundToTheCommand(t *testing.T) {
	first := combatDrawSource("cmd-1", "cbt-1", 1)
	same := combatDrawSource("cmd-1", "cbt-1", 1)
	other := combatDrawSource("cmd-2", "cbt-1", 1)
	for _, purpose := range []string{"initiative", "hit", "damage"} {
		a := first(purpose, DefaultActorID, "rival")
		if a != same(purpose, DefaultActorID, "rival") {
			t.Fatalf("%s must be reproducible for one command: %v vs %v", purpose, a, same(purpose, DefaultActorID, "rival"))
		}
		if a < 0 || a >= 1 {
			t.Fatalf("%s must be a fraction in [0,1): %v", purpose, a)
		}
		if a == other(purpose, DefaultActorID, "rival") {
			t.Fatalf("%s must depend on the command identity", purpose)
		}
	}
	if first("hit", DefaultActorID, "rival") == first("hit", "rival", DefaultActorID) {
		t.Fatal("a draw must depend on who is acting and who is targeted")
	}
	if first("hit", DefaultActorID, "rival") == combatDrawSource("cmd-1", "cbt-1", 2)("hit", DefaultActorID, "rival") {
		t.Fatal("a draw must depend on the exchange it belongs to")
	}
}

// TestCombatExchangeReplayIsStableAcrossRuns is the property §七.2 exists for,
// stated end to end: resolving the same command twice produces the same battle.
func TestCombatExchangeReplayIsStableAcrossRuns(t *testing.T) {
	store, storyID := combatTestStory(t)
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	actions := map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	}
	first, firstOutcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", combatTestSpecs(), nil, actions)
	if err != nil {
		t.Fatal(err)
	}
	second, secondOutcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-1", combatTestSpecs(), nil, actions)
	if err != nil {
		t.Fatal(err)
	}
	if firstOutcome.Order[0] != secondOutcome.Order[0] || len(firstOutcome.Steps) != len(secondOutcome.Steps) {
		t.Fatalf("a replayed exchange must settle identically:\n%#v\n%#v", firstOutcome, secondOutcome)
	}
	for _, combatant := range first.Combatants {
		if combatTestCombatant(t, second, combatant.ID).Attributes.Health != combatant.Attributes.Health {
			t.Fatalf("combatant %s must replay at the same health: %#v vs %#v", combatant.ID, first, second)
		}
	}
}

// TestSettledBattleIsNotRefought is §七.8: a battle that has reached a result
// stands, and reopening the window shows the result rather than offering another
// round.
func TestSettledBattleIsNotRefought(t *testing.T) {
	store, storyID := combatTestStory(t)
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)

	ended := cultivation.CombatSession{
		ID: "cbt-1", Round: 4, Ended: true, Result: cultivation.ResultPlayerWon,
		Combatants: []cultivation.Combatant{
			{ID: DefaultActorID, Side: cultivation.SidePlayer, Attributes: cultivation.Attributes{Health: 90, HealthMax: 200}},
			{ID: "rival", Side: cultivation.SideEnemy, Defeated: true, Attributes: cultivation.Attributes{Health: 0, HealthMax: 200}},
		},
	}
	session, outcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-2", combatTestSpecs(), &ended, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Ended || outcome.Result != cultivation.ResultPlayerWon {
		t.Fatalf("a settled battle must report its result rather than resolving: %#v", outcome)
	}
	if session.Round != ended.Round {
		t.Fatalf("a settled battle must not advance: %d vs %d", session.Round, ended.Round)
	}
	if len(outcome.Steps) != 0 {
		t.Fatalf("a settled battle must resolve no further exchange: %#v", outcome.Steps)
	}
}

// TestCombatRecordBindsItsCommandToTheBranch keeps a story's two branches from
// answering each other's retries: the command identity is looked up on the branch
// that committed it, so a branch cut before the battle does not report a fight
// that is not on its path.
func TestCombatRecordBindsItsCommandToTheBranch(t *testing.T) {
	store, storyID := combatTestStory(t)
	combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	// The committing branch finds it, and the answer is the exchange it settled.
	found, ok, err := store.CombatRecordForCommand(storyID, "main", "cmd-1", "hash-1")
	if err != nil || !ok {
		t.Fatalf("the committing branch must find its command: found=%v err=%v", ok, err)
	}
	if found.SessionID != "cbt-1" {
		t.Fatalf("the retry must read back the settled exchange, got %q", found.SessionID)
	}

	// A fork cut at the Turn the battle was fought after is positioned *before*
	// the record: the record is that Turn's child, so it is not on the fork's
	// path. The fork must therefore not answer main's retries — that is what
	// keeps one branch's battle from being replayed onto another.
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: combatTestLatestTurnID(t, store, storyID),
		Title:         "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.CombatRecordForCommand(storyID, branch.ID, "cmd-1", "hash-1"); err != nil || ok {
		t.Fatalf("a branch that did not commit the command must not answer it: found=%v err=%v", ok, err)
	}
	// An unknown command is not found on any branch.
	if _, found, err := store.CombatRecordForCommand(storyID, branch.ID, "cmd-never", "hash-1"); err != nil || found {
		t.Fatalf("an uncommitted command must not be found: found=%v err=%v", found, err)
	}
}

func combatTestRecordCount(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeCombatRecord {
			count++
		}
	}
	return count
}

func combatTestLatestTurnID(t *testing.T, store *Store, storyID string) string {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	meta, lines, err := store.readStoryRecentLocked(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turnID := latestLogicalTurnID(meta, lines, "main")
	if turnID == "" {
		t.Fatal("the fixture must have a turn to fork from")
	}
	return turnID
}

func combatTestProjectionForBranch(t *testing.T, store *Store, storyID, branchID string) *storyBranchProjection {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	projection, err := store.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

// TestCombatRecordKeepsTheBranchParentChainWalkable is the regression for the
// self-parenting settlement.
//
// The battle commits two events: the record, then the settlement parented to it.
// If the settlement reuses the record's identity as its own parent, the branch's
// parent chain closes into a cycle, and every later walk of it — history pages,
// model context, checkpoints — never reaches an ancestor. The symptom is a hang,
// not an error, so this asserts the shape directly and then proves the walk
// terminates by actually reading history and the snapshot back.
func TestCombatRecordKeepsTheBranchParentChainWalkable(t *testing.T) {
	store, storyID := combatTestStory(t)
	combatTestExchange(t, store, storyID, "cmd-1", "hash-1")

	store.mu.Lock()
	meta, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	// Every event in the journal must carry an identity, and no event may be its
	// own parent. Both together are what make the chain a walkable path.
	seen := map[string]bool{}
	for _, record := range records {
		id := record.Envelope.ID
		if id == "" {
			t.Fatalf("every event needs an identity: %#v", record.Envelope)
		}
		if seen[id] {
			t.Fatalf("event identity %q is used twice; identities must be unique", id)
		}
		seen[id] = true
		if parent := parentIDFromRaw(record.Raw); parent != "" && parent == id {
			t.Fatalf("event %q is its own parent, which closes the chain into a cycle", id)
		}
	}

	// The chain from the branch head must terminate at an ancestor within a bound
	// proportional to the journal, not spin. Walking with an explicit step budget
	// makes a cycle a failure rather than a hung test binary.
	head := meta.Branches["main"].Head
	byID := map[string]map[string]any{}
	for _, record := range records {
		byID[record.Envelope.ID] = record.Raw
	}
	steps := 0
	for cursor := head; cursor != ""; {
		steps++
		if steps > len(records)+1 {
			t.Fatalf("the parent chain from head %q did not terminate within %d steps; it is cyclic", head, len(records)+1)
		}
		raw, ok := byID[cursor]
		if !ok {
			break
		}
		cursor = parentIDFromRaw(raw)
	}

	// The walk must be real: reading history and the snapshot back is what proves
	// the two consumers that hung before now return.
	page, err := store.ReadHistoryPage(storyID, "main", "", 20)
	if err != nil {
		t.Fatalf("history must be readable after a battle: %v", err)
	}
	if len(page.Turns) == 0 {
		t.Fatal("history must still carry the turns the battle was fought between")
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatalf("the snapshot must be readable after a battle: %v", err)
	}
	if snapshot.BranchID != "main" {
		t.Fatalf("the snapshot must describe the branch that fought: %#v", snapshot.BranchID)
	}
}

// TestHistoryRefusesACyclicParentChain checks the guard itself, on isolated
// synthetic data: a journal whose head is its own ancestor must be reported, not
// walked forever.
//
// It writes the exact corruption the combat commit used to produce — an event
// whose ParentID is its own ID, with the branch head on it — through the same
// locked append path a real commit uses, so the guard is exercised against a
// journal that is genuinely cyclic rather than a hand-built map.
func TestHistoryRefusesACyclicParentChain(t *testing.T) {
	store, storyID := combatTestStory(t)

	store.mu.Lock()
	release, err := store.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	defer func() {
		release()
		store.mu.Unlock()
	}()

	meta, _, err := store.readStoryRecentLocked(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	branch := meta.Branches["main"]
	cycleID := "sd-cycle"
	event := StateDeltaEvent{
		V: schemaVersion, Type: StoryEventTypeStateDelta, ID: cycleID,
		ParentID: cycleID, BranchID: "main", Ts: time.Now().UTC().Format(time.RFC3339Nano),
	}
	branch.Head = cycleID
	meta.Branches["main"] = branch
	if err := store.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		t.Fatal(err)
	}

	_, err = store.readStoryHistoryPageLocked(storyID, "main", "", 20, true)
	if err == nil {
		t.Fatal("a cyclic parent chain must be reported, not walked forever")
	}
	if !strings.Contains(err.Error(), "环") {
		t.Fatalf("the failure must name the cycle, got %v", err)
	}
}
