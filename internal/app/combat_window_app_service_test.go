package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the battle window's own contract: what the read reports,
// what the write accepts, and which of the two answers a retry gets.
//
// The store layer beneath this already has its own suite for the rules and the
// journal record. What it cannot reach is everything this layer decides — that
// an empty branch and the current branch are the same command, that a battle
// belongs to the branch it was fought on, that a value the rules never settled
// is omitted rather than sent as a zero, and that a settled battle is reported
// rather than advanced. Those are asserted here and not restated.

const (
	combatTestRealmField         = "当前境界"
	combatTestHealthField        = "生命"
	combatTestManaField          = "灵力"
	combatTestBurdenField        = "经脉负担"
	combatTestBurdenFloorField   = "余伤负担下限"
	combatTestRealmProgressField = "境界进度"
	combatTestSkillsField        = "已学功法"
	combatTestCarriedField       = "重要物品"
	// combatTestImplementField is the real equipment slot 冲脉秘剑 needs in hand.
	combatTestImplementField = "主法器"
	// The three T22 also names as withheld: the purse, how the character spends
	// their time, and the safe window an outing offers.
	combatTestCurrencyField       = "灵石"
	combatTestArrangementField    = "修行安排"
	combatTestTrainingWindowField = "修炼间隙"
)

// combatTestSystem builds the real xiuxian preset with a second tracked
// template, so both sides have numbers the rules own.
//
// The module comes from the library rather than from an unexported preset
// constructor, because this package is the one that would ship the module. The
// rival template is a copy of the protagonist's, which is what makes it a
// combatant the cultivation rules track.
func combatTestSystem(t *testing.T, rivalSkills ...string) interactive.StoryDirectorActorStateSystem {
	t.Helper()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	system := module.ActorState
	if len(system.Templates) == 0 {
		t.Fatal("the xiuxian module must declare templates")
	}
	rival := system.Templates[0]
	rival.ID = "rival"
	rival.Name = "对手"
	system.Templates = append(system.Templates, rival)

	stage := cultivation.StageKey(cultivation.Qi(6))
	system.InitialActors = append(system.InitialActors, interactive.ActorStateInitialActor{
		ID: "rival", Name: "对手", TemplateID: "rival",
		State: map[string]any{
			combatTestRealmField:  stage,
			combatTestSkillsField: learnedSkillRecords(rivalSkills),
			// Written so the projection test's absences are real absences rather
			// than fields the rival never had. The current burden stays below the
			// residual floor: a rival carrying more burden than its own injury
			// permits could cast nothing that adds any, so the tests that let the
			// enemy choose for itself would only ever see the plain attack.
			combatTestBurdenField:         float64(10),
			combatTestBurdenFloorField:    float64(20),
			combatTestRealmProgressField:  float64(1200),
			combatTestImplementField:      map[string]any{"名称": "青锋剑", "定义": "item_blade", "品级": "中品", "稀有度": "常见"},
			combatTestCurrencyField:       float64(480),
			combatTestArrangementField:    "专心闭关",
			combatTestTrainingWindowField: "后山石室，无人打扰，可调息三日。",
		},
	})
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestRealmField] = stage
		system.InitialActors[index].State = state
	}
	return system
}

func learnedSkillRecords(skillIDs []string) map[string]any {
	records := make(map[string]any, len(skillIDs))
	for _, skillID := range skillIDs {
		records[skillID] = map[string]any{"定义": skillID, "版本": 1, "掌握程度": "入门"}
	}
	return records
}

// combatTestApp builds a story whose cast is already settled, so a battle has
// real pools to read rather than the zero a fresh story starts at.
func combatTestApp(t *testing.T, rivalSkills ...string) (*App, string) {
	t.Helper()
	return combatTestAppWithSystem(t, combatTestSystem(t, rivalSkills...))
}

func combatTestAppWithSystem(t *testing.T, system interactive.StoryDirectorActorStateSystem) (*App, string) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "战斗窗口", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = turn })
	return &App{workspace: root, interactive: store}, story.ID
}

func combatTestSpecs() []interactive.CombatantSpec {
	return []interactive.CombatantSpec{
		{ActorID: interactive.DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
		{ActorID: "rival", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
	}
}

func combatTestStrike(t *testing.T) cultivation.SkillDef {
	t.Helper()
	skill, ok := cultivation.SkillByID("strike")
	if !ok {
		t.Fatal("the published table must contain the ordinary attack")
	}
	return skill
}

func combatWindowSide(t *testing.T, window CombatWindow, actorID string) CombatWindowSide {
	t.Helper()
	for _, side := range window.Sides {
		if side.ActorID == actorID {
			return side
		}
	}
	t.Fatalf("the window must report %q: %#v", actorID, window.Sides)
	return CombatWindowSide{}
}

// TestCombatWindowWithoutAStoreRefusesRatherThanInventingABattle covers the
// boundary the window is most tempted to cross: with no workspace there is no
// journal to read, and a window that answered anyway would show a fight that
// does not exist.
func TestCombatWindowWithoutAStoreRefusesRatherThanInventingABattle(t *testing.T) {
	var missing *App
	if _, err := missing.CombatWindow(CombatWindowRequest{StoryID: "story"}); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a nil app must refuse: %v", err)
	}
	if _, err := (&App{}).CombatWindow(CombatWindowRequest{StoryID: "story"}); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("an app with no store must refuse: %v", err)
	}
	if _, err := (&App{}).PlayCombatRound(CombatRoundRequest{StoryID: "story"}); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a round with no store must refuse: %v", err)
	}
}

func TestCombatWindowRequiresAStory(t *testing.T) {
	application, _ := combatTestApp(t)
	if _, err := application.CombatWindow(CombatWindowRequest{}); err == nil {
		t.Fatal("the window must require a story")
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{}); err == nil {
		t.Fatal("a round must require a story")
	}
}

// TestCombatWindowBeforeABattleIsChosenShowsTheSceneAndTheChoice is the entry
// point's contract: with no session selected the window reports where the story
// stands and which battles are live, and nothing about a battle's sides.
func TestCombatWindowBeforeABattleIsChosenShowsTheSceneAndTheChoice(t *testing.T) {
	application, storyID := combatTestApp(t)

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.BranchID != "main" {
		t.Fatalf("an unset branch must resolve to the current one, got %q", window.BranchID)
	}
	if window.Scene.NodeID == "" {
		t.Fatal("the window must report the scene it belongs to")
	}
	if len(window.Sessions) != 0 {
		t.Fatalf("no battle has been opened yet: %#v", window.Sessions)
	}
	if window.SessionID != "" || len(window.Sides) != 0 {
		t.Fatalf("nothing may be reported about an unchosen battle: %#v", window)
	}
}

// TestCombatWindowRefusesABattleThatIsNotLive keeps the window from answering
// with an invented fight. The error is the sentinel the handler maps to a 404,
// because the request was well formed and the battle simply is not there.
func TestCombatWindowRefusesABattleThatIsNotLive(t *testing.T) {
	application, storyID := combatTestApp(t)

	_, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, SessionID: "cbt-nobody"})
	if !errors.Is(err, ErrCombatUnknown) {
		t.Fatalf("an unknown battle must be ErrCombatUnknown, got %v", err)
	}
}

// TestCombatRoundHashTreatsAnEmptyBranchAsTheCurrentBranch is the retry-safety
// boundary at the branch level.
//
// The window may be opened with no branch named and retried with the branch it
// was told about. If those two produced different fingerprints the retry would
// be refused as a conflict, so the fingerprint is resolved against the branch
// the store will actually write to.
func TestCombatRoundHashTreatsAnEmptyBranchAsTheCurrentBranch(t *testing.T) {
	application, storyID := combatTestApp(t)
	request := CombatRoundRequest{StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs()}

	implicit := combatRoundCommandHash(request, "main")
	explicit := combatRoundCommandHash(CombatRoundRequest{
		StoryID: storyID, BranchID: "main", CommandID: "cmd-open", Combatants: combatTestSpecs(),
	}, "main")
	if implicit != explicit {
		t.Fatalf("an unset branch and the current branch must be the same command: %q vs %q", implicit, explicit)
	}
	explicitSpare := combatRoundCommandHash(CombatRoundRequest{StoryID: storyID, PlayerIntent: interactive.PlayerCombatIntentSpare}, "main")
	emptyIntent := combatRoundCommandHash(CombatRoundRequest{StoryID: storyID}, "main")
	if explicitSpare != emptyIntent {
		t.Fatalf("an omitted intent and explicit spare must share a retry identity: %q vs %q", emptyIntent, explicitSpare)
	}
	takeLife := combatRoundCommandHash(CombatRoundRequest{StoryID: storyID, PlayerIntent: interactive.PlayerCombatIntentTakeLife}, "main")
	if takeLife == explicitSpare {
		t.Fatal("the player's take-life choice must be part of the command identity")
	}
	if !strings.HasPrefix(implicit, "sha256:") {
		t.Fatalf("the fingerprint must be a prefixed digest, got %q", implicit)
	}
	_ = application
}

// TestCombatRoundHashIsStableAcrossMapOrder is what makes a retry safe at all:
// a fingerprint that changed between two identical rounds would refuse a
// legitimate retry.
func TestCombatRoundHashIsStableAcrossMapOrder(t *testing.T) {
	strike := combatTestStrike(t)
	build := func() string {
		actions := map[string]cultivation.CombatAction{
			"protagonist": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "rival"},
			"rival":       {Kind: cultivation.ActionRest},
			"third":       {Kind: cultivation.ActionEscape},
		}
		return combatRoundCommandHash(CombatRoundRequest{StoryID: "s", SessionID: "cbt-1", Actions: actions}, "main")
	}
	first := build()
	for attempt := 0; attempt < 20; attempt++ {
		if got := build(); got != first {
			t.Fatalf("the fingerprint changed between identical rounds: %q vs %q", got, first)
		}
	}
}

// TestPlayCombatRoundRequiresItsCombatants covers the one thing a new battle
// cannot derive: which characters are actually in the fight.
func TestPlayCombatRoundRequiresItsCombatants(t *testing.T) {
	application, storyID := combatTestApp(t)

	_, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "cmd-open"})
	if err == nil {
		t.Fatal("opening a battle with no participants must be refused")
	}
	sessions, err := application.interactive.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("a refused opening must leave no battle behind: %#v", sessions)
	}
}

// TestOpeningABattleEstablishesTheEncounterAndWaitsForTheFirstAction is §七.1's
// opening as an assertion: the fight exists and is shown at the numbers both
// sides already have, and nothing is settled until the player has seen that and
// chosen.
//
// The pools and the stock are the player-visible consequence, so they are what
// the test reads. The battle must also be a fact the journal holds rather than a
// window-local state: closing and reopening the window has to find the same
// encounter still waiting on the same first exchange.
func TestOpeningABattleEstablishesTheEncounterAndWaitsForTheFirstAction(t *testing.T) {
	application, storyID := combatTestAppCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, "fire_bolt")

	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.SessionID == "" {
		t.Fatal("opening must establish a battle the window can address")
	}
	if opened.Round != 1 || opened.Ended {
		t.Fatalf("an opened battle waits on the first exchange: round %d ended %t", opened.Round, opened.Ended)
	}
	if len(opened.History) != 0 || opened.LastRound != nil {
		t.Fatalf("opening settles no exchange: %#v", opened.History)
	}
	player := combatWindowSide(t, opened, interactive.DefaultActorID)
	if *player.Health != *player.HealthMax || *player.Mana != *player.ManaMax {
		t.Fatalf("opening must not spend the player's pools: %#v", player)
	}
	if dose := combatTestWindowItem(t, opened, interactive.DefaultActorID, "回春丹"); dose.Quantity != 3 {
		t.Fatalf("opening must not spend stock, quantity = %d", dose.Quantity)
	}
	// The selector is what the player acts with, so the encounter has to arrive
	// with it: a fight that opens with no action to choose is not playable.
	if opened.PlayerActorID != interactive.DefaultActorID {
		t.Fatalf("the opened encounter must name the side the player acts as: %q", opened.PlayerActorID)
	}
	if len(player.Items) == 0 {
		t.Fatalf("the opened encounter must offer the player's own stock: %#v", player)
	}

	// Closing and reopening the window is a read. It must find the same battle
	// still waiting, not a round that was played in the meantime.
	reopened, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: opened.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Round != 1 || len(reopened.History) != 0 || reopened.LastRound != nil {
		t.Fatalf("reopening must wait on the same first exchange: round %d, history %d", reopened.Round, len(reopened.History))
	}
	if after := combatWindowSide(t, reopened, interactive.DefaultActorID); *after.Health != *player.Health || *after.Mana != *player.Mana {
		t.Fatalf("reopening must not settle anything: %#v", after)
	}

	// And the first exchange is the action the player chose, not an automatic one.
	played, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-first",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if played.LastRound == nil || played.LastRound.Outcome.Round != 1 {
		t.Fatalf("the chosen action must be the first round: %#v", played.LastRound)
	}
	if len(played.History) != 1 {
		t.Fatalf("one played round is one committed exchange: %d", len(played.History))
	}
	step, ok := combatTestStep(played.LastRound.Outcome, interactive.DefaultActorID)
	if !ok || step.Kind != cultivation.ActionTechnique || step.SkillID != "strike" {
		t.Fatalf("round 1 must be the declared action: %#v", played.LastRound.Outcome)
	}
	// The enemy decided for itself in the same round: the opening no longer skips
	// its decision.
	if _, ok := combatTestStep(played.LastRound.Outcome, "rival"); !ok {
		t.Fatalf("the enemy must act in the first round from its own state: %#v", played.LastRound.Outcome)
	}
}

// TestPlayingARoundWithoutThePlayersOwnActionIsRefused keeps an omitted action
// from becoming the engine's default plain attack. The rules substitute 普通攻击
// for an undeclared combatant, which would spend the player's pools for a
// decision they never made, so the round is refused before it is resolved.
func TestPlayingARoundWithoutThePlayersOwnActionIsRefused(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	before := combatWindowSide(t, opened, interactive.DefaultActorID)

	for name, actions := range map[string]map[string]cultivation.CombatAction{
		"nothing declared": nil,
		"only the enemy":   {"rival": {Kind: cultivation.ActionRest}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := application.PlayCombatRound(CombatRoundRequest{
				StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-unplayed",
				Actions: actions,
			})
			if !errors.Is(err, interactive.ErrCombatPlayerActionRequired) {
				t.Fatalf("a round with no player action must be refused, got %v", err)
			}
		})
	}

	after, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: opened.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.History) != 0 || after.Round != 1 {
		t.Fatalf("a refused round must leave the encounter where it was: round %d, history %d", after.Round, len(after.History))
	}
	player := combatWindowSide(t, after, interactive.DefaultActorID)
	if *player.Health != *before.Health || *player.Mana != *before.Mana {
		t.Fatalf("a refused round must spend nothing: %#v -> %#v", before, player)
	}
	// The ordinary attack is still a legal choice; what is refused is choosing
	// nothing at all.
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-strike",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	}); err != nil {
		t.Fatalf("the ordinary attack must remain playable: %v", err)
	}
}

// TestAnOpeningRequestThatAlsoDeclaresARoundIsRefused states §七.1's split as a
// refusal rather than an interpretation. Settling an exchange in the request
// that opens the fight would settle it before the player could see the state it
// was fought at, which is the defect the split removes.
func TestAnOpeningRequestThatAlsoDeclaresARoundIsRefused(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")

	_, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if !errors.Is(err, ErrCombatOpeningHasActions) {
		t.Fatalf("an opening that also declares a round must be refused, got %v", err)
	}
	sessions, err := application.interactive.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("a refused opening must leave no battle behind: %#v", sessions)
	}
}

func TestPlayCombatRoundRetryReturnsTheCommittedRound(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-round-1",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	}

	first, err := application.PlayCombatRound(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.PlayCombatRound(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID == "" || first.SessionID != second.SessionID {
		t.Fatalf("a retry must address the same battle: %q vs %q", first.SessionID, second.SessionID)
	}
	if first.Round != second.Round {
		t.Fatalf("a retry must not advance the battle: round %d vs %d", first.Round, second.Round)
	}
	if len(first.History) != 1 || len(second.History) != 1 {
		t.Fatalf("a retry must not commit a second round: %d vs %d", len(first.History), len(second.History))
	}
	before, after := combatWindowSide(t, first, interactive.DefaultActorID), combatWindowSide(t, second, interactive.DefaultActorID)
	if *before.Health != *after.Health {
		t.Fatalf("a retry must not deal damage again: %d vs %d", *before.Health, *after.Health)
	}
}

// TestPlayCombatRoundRefusesAReusedCommandForADifferentRound is the other half
// of idempotency: the same identity for genuinely different content is refused
// rather than answered with the wrong round.
func TestPlayCombatRoundRefusesAReusedCommandForADifferentRound(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-round",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// The same command identity, now describing a different round.
	_, err = application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-round",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	})
	if !errors.Is(err, interactive.ErrCombatCommandConflict) {
		t.Fatalf("a reused command identity for different content must conflict, got %v", err)
	}
}

// TestPlayCombatRoundDoesNotAdvanceASettledBattle is §七.8 at the window: a
// result stands, so reopening a finished battle reports it instead of offering
// another exchange.
func TestPlayCombatRoundDoesNotAdvanceASettledBattle(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		if strings.Contains(request.Messages[1].Content, `"id":"surrender"`) {
			return `{"candidate_id":"surrender"}`
		}
		return `{"candidate_id":"rest"}`
	})

	// The rival yields and the protagonist accepts, which ends the fight as a
	// truce rather than a kill — the non-lethal path the design requires to be
	// reachable. The encounter is opened first: §七.1 keeps the first exchange the
	// player's own, so the rival's offer is made in a played round.
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true, StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true, StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-offer",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	}); err != nil {
		t.Fatal(err)
	}
	settled, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true, StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-accept",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionAcceptSurrender, TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !settled.Ended {
		t.Fatalf("accepting a surrender must settle the battle: %#v", settled)
	}
	rounds := len(settled.History)

	// A further round against a settled battle must be a read, not a round.
	again, err := application.PlayCombatRound(CombatRoundRequest{
		SkipNarration: true, StoryID: storyID, SessionID: settled.SessionID, CommandID: "cmd-after",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Ended || again.Result != settled.Result {
		t.Fatalf("a settled result must stand: %#v", again)
	}
	if len(again.History) != rounds {
		t.Fatalf("a settled battle must not commit another round: %d vs %d", len(again.History), rounds)
	}
}

// TestCombatWindowOmitsValuesTheRulesNeverSettled is the knowledge boundary the
// window must not blur: an effect that is not in force is absent rather than
// sent as a zero the player would read as fact.
func TestCombatWindowOmitsValuesTheRulesNeverSettled(t *testing.T) {
	application, storyID := combatTestApp(t)
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range window.Sides {
		if side.Shield != nil || side.BurnTicks != nil || side.SlowActions != nil {
			t.Fatalf("an effect not in force must be omitted, got %#v", side)
		}
	}
	// The pools are the stage table's real numbers for the seeded realm, which
	// is what makes the window show the fight's own numbers rather than a zero.
	// They are reported for the player's own side only; an opponent's pools are
	// covered by TestCombatWindowDoesNotShowAStrangersNumbers.
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if player.Health == nil || player.HealthMax == nil || player.Mana == nil || player.ManaMax == nil {
		t.Fatalf("a settled pool must be reported for the player: %#v", player)
	}
	if *player.HealthMax != 200 || *player.ManaMax != 160 {
		t.Fatalf("the seeded realm's pools must be reported: health_max=%d mana_max=%d", *player.HealthMax, *player.ManaMax)
	}
	if player.Realm != cultivation.StageKey(cultivation.Qi(6)) {
		t.Fatalf("the window must report the rules' own stage key, got %q", player.Realm)
	}
}

// TestCombatWindowListsOnlyTheCombatantsOwnTechniques is §七.1 at the read
// side: an opponent's list is what that opponent learned, never the registry's
// whole published table — and it is not the player's to read at all. The
// player's own side still lists exactly what it learned.
func TestCombatWindowListsOnlyTheCombatantsOwnTechniques(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatWindowSide(t, window, "rival")
	if len(rival.Skills) != 0 {
		t.Fatalf("an opponent's learned set must not be listed: %#v", rival.Skills)
	}
	// The protagonist has learned nothing, so its list is empty rather than the
	// table.
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Skills) != 0 {
		t.Fatalf("a combatant who learned nothing must list nothing: %#v", player.Skills)
	}
}

// TestCombatWindowListsThePlayersOwnTechniquesWithTheirRealCosts is the other
// half of the read side: the player's own list is reported with the rules' own
// prices, because those are the numbers the player acts on.
func TestCombatWindowListsThePlayersOwnTechniquesWithTheirRealCosts(t *testing.T) {
	application, storyID := combatTestAppWithPlayerSkills(t, "fire_bolt")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Skills) != 1 || player.Skills[0].ID != "fire_bolt" {
		t.Fatalf("the player must list what they learned: %#v", player.Skills)
	}
	// A cost is priced against the caster's own stage, so it is the rules'
	// number rather than the published table's coefficient.
	if player.Skills[0].ManaCost <= 0 {
		t.Fatalf("a paid technique must report a real cost: %#v", player.Skills[0])
	}
	if player.Skills[0].NameKey == "" {
		t.Fatalf("a technique must carry its localisation key: %#v", player.Skills[0])
	}
}

// TestCombatWindowReportsACastThatCannotBePaid covers the resource boundary as
// the interface sees it: a technique the player's own character cannot afford is
// listed with the reason rather than hidden or silently allowed. An opponent's
// refusals are not reported at all — see
// TestCombatWindowDoesNotShowAStrangersNumbers.
func TestCombatWindowReportsACastThatCannotBePaid(t *testing.T) {
	// The protagonist has learned the prepared secret and holds almost no mana,
	// so the technique is real, learned, and still not payable.
	application, storyID := combatTestAppWithPlayerMana(t, 1, "meridian_secret_blade")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Skills) != 1 {
		t.Fatalf("the player's learned technique must still be listed: %#v", player.Skills)
	}
	skill := player.Skills[0]
	if skill.Castable {
		t.Fatalf("a technique beyond the caster's mana must not be castable: %#v", skill)
	}
	if skill.ReasonKey != cultivation.ReasonInsufficientMana {
		t.Fatalf("the refusal must name the resource it lacks: %q", skill.ReasonKey)
	}
	if skill.ManaCost <= *player.Mana {
		t.Fatalf("the cost must exceed what is held: cost=%d mana=%d", skill.ManaCost, *player.Mana)
	}
}

// TestCombatWindowReportsACastBlockedByBurden covers the other resource the
// design singles out: a technique can be unaffordable in burden while mana is
// plentiful, and the window must say so rather than only checking mana.
func TestCombatWindowReportsACastBlockedByBurden(t *testing.T) {
	// Full mana, but the meridians are already carrying a residual injury whose
	// floor this technique would push past.
	application, storyID := combatTestAppWithPlayerBurden(t, "meridian_secret_blade")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	skill := combatWindowSide(t, window, interactive.DefaultActorID).Skills[0]
	if skill.Castable {
		t.Fatalf("a technique past the residual floor must not be castable: %#v", skill)
	}
	if skill.ReasonKey != cultivation.ReasonMeridianUnrecovered {
		t.Fatalf("the refusal must name the residual injury: %q", skill.ReasonKey)
	}
}

// TestCombatWindowReportsACastBlockedByTheMissingFocus covers §9's third
// published reason: 法器不在手中. The card stays listed and says why rather than
// disappearing, and the reason is answered from real equipment — the same
// character with the blade in the slot is castable.
func TestCombatWindowReportsACastBlockedByTheMissingFocus(t *testing.T) {
	application, storyID := combatTestAppWithPlayerState(t, func(state map[string]any) {
		state[combatTestSkillsField] = learnedSkillRecords([]string{"meridian_secret_blade"})
	})
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	side := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(side.Skills) != 1 {
		t.Fatalf("the technique must still be listed so its reason can be read: %#v", side.Skills)
	}
	if side.HasImplement {
		t.Fatal("the fixture must not put a focus in hand")
	}
	if side.Skills[0].Castable || side.Skills[0].ReasonKey != cultivation.ReasonImplementRequired {
		t.Fatalf("a missing focus must be reported as such: %#v", side.Skills[0])
	}
}

// TestCombatWindowListsNoTechniqueTheCharacterNeverLearned covers §T21's
// 招式未掌握: the window is where the player picks what to do, so a technique
// they do not know must not be on it at all. Refusing it after they chose would
// be answering a question that should never have been asked.
//
// §七.1 states the same rule for an opponent. It holds for the player too, and
// for the same reason: the list is what a character can really do, not what the
// game's table contains.
func TestCombatWindowListsNoTechniqueTheCharacterNeverLearned(t *testing.T) {
	// One learned technique and full mana, so nothing else could explain an
	// absence.
	application, storyID := combatTestAppWithPlayerMana(t, 160, "fire_bolt")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	if len(player.Skills) != 1 {
		t.Fatalf("only the one learned technique may be listed: %#v", player.Skills)
	}
	if player.Skills[0].ID != "fire_bolt" {
		t.Fatalf("the listed technique is the learned one: %#v", player.Skills[0])
	}
	for _, skill := range player.Skills {
		if skill.ID == "heavy_thunder" || skill.ID == "condensed_blade" {
			t.Fatalf("a technique never learned must not appear: %#v", skill)
		}
	}
}

// combatTestAppWithPlayerSkills writes the protagonist's learned set down, which
// is the only side the window reports exactly.
func combatTestAppWithPlayerSkills(t *testing.T, skillIDs ...string) (*App, string) {
	t.Helper()
	return combatTestAppWithPlayerState(t, func(state map[string]any) {
		state[combatTestSkillsField] = learnedSkillRecords(skillIDs)
	})
}

// combatTestBlade is a real, priced implement for the slot 冲脉秘剑 needs.
//
// It is written as an item rather than a bare name because the field is declared
// as an object; the requirement itself only asks that something really is in
// hand, which is what makes a refused cast a fact about equipment.
func combatTestBlade() map[string]any {
	return map[string]any{
		"名称": "青锋剑", "品级": "中品", "稀有度": "常见", "主要属性预算": float64(5),
	}
}

// combatTestAppWithPlayerMana is the protagonist's own resource boundary.
func combatTestAppWithPlayerMana(t *testing.T, mana int, skillIDs ...string) (*App, string) {
	t.Helper()
	return combatTestAppWithPlayerState(t, func(state map[string]any) {
		state[combatTestManaField] = mana
		state[combatTestSkillsField] = learnedSkillRecords(skillIDs)
		state[combatTestImplementField] = combatTestBlade()
	})
}

// combatTestAppWithPlayerBurden writes a residual injury onto the protagonist.
func combatTestAppWithPlayerBurden(t *testing.T, skillIDs ...string) (*App, string) {
	t.Helper()
	return combatTestAppWithPlayerState(t, func(state map[string]any) {
		state[combatTestBurdenFloorField] = 10
		state[combatTestSkillsField] = learnedSkillRecords(skillIDs)
		state[combatTestImplementField] = combatTestBlade()
	})
}

func combatTestAppWithPlayerState(t *testing.T, write func(map[string]any)) (*App, string) {
	t.Helper()
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		write(state)
		system.InitialActors[index].State = state
	}
	return combatTestAppWithSystem(t, system)
}

// combatTestAppCarrying is the same fixture with the protagonist's carried
// record written down, so a battle has real stock to read and spend.
func combatTestAppCarrying(t *testing.T, carried map[string]any, rivalSkills ...string) (*App, string) {
	t.Helper()
	system := combatTestSystem(t, rivalSkills...)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestCarriedField] = carried
		system.InitialActors[index].State = state
	}
	return combatTestAppWithSystem(t, system)
}

// combatTestAppCarryingWounded writes the protagonist's health down alongside the
// carried stock.
//
// Whether the rival's opening blow lands is a draw, so a test that waited for a
// wound would pass or fail by luck; a wound the rules must treat has to be a
// settled fact. The seeded pool is well below the Qi-refining layer 6 maximum of
// 200, which is what the restoring pill is priced against.
func combatTestAppCarryingWounded(t *testing.T, carried map[string]any, health int, rivalSkills ...string) (*App, string) {
	t.Helper()
	system := combatTestSystem(t, rivalSkills...)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state[combatTestCarriedField] = carried
		state[combatTestHealthField] = health
		system.InitialActors[index].State = state
	}
	return combatTestAppWithSystem(t, system)
}

// combatTestStep finds one actor's step in a settled exchange.
func combatTestStep(outcome cultivation.ExchangeOutcome, actorID string) (cultivation.ExchangeStep, bool) {
	for _, step := range outcome.Steps {
		if step.ActorID == actorID {
			return step, true
		}
	}
	return cultivation.ExchangeStep{}, false
}

// combatTestWindowItem finds one carried copy as the window reports it.
func combatTestWindowItem(t *testing.T, window CombatWindow, actorID, instanceID string) CombatWindowItem {
	t.Helper()
	for _, item := range combatWindowSide(t, window, actorID).Items {
		if item.InstanceID == instanceID {
			return item
		}
	}
	t.Fatalf("%q does not carry %q: %#v", actorID, instanceID, combatWindowSide(t, window, actorID).Items)
	return CombatWindowItem{}
}

// TestCombatWindowReportsTheCarriedStockWithItsRealNumbers is §3 at the window:
// the name, the kind and the effect are shown together, and the effect is the
// catalogue's own figure rather than anything the story wrote.
func TestCombatWindowReportsTheCarriedStockWithItsRealNumbers(t *testing.T) {
	application, storyID := combatTestAppCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
		// §10: an implement carried in the pack contributes nothing. It must not
		// appear as a dose either, because only a pill is something a round spends.
		"备用剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	})
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	items := combatWindowSide(t, window, interactive.DefaultActorID).Items
	if len(items) != 1 {
		t.Fatalf("only the pill is stock a round can spend: %#v", items)
	}
	item := items[0]
	if item.InstanceID != "回春丹" || item.DefID != "pill.restore.health.lower" {
		t.Fatalf("the copy must be named as itself: %#v", item)
	}
	if item.NameKey == "" {
		t.Fatalf("a definition must carry its localisation key: %#v", item)
	}
	if item.Quantity != 2 {
		t.Fatalf("quantity = %d, want the recorded 2", item.Quantity)
	}
	if item.Purpose != string(cultivation.PillRestoreHealth) {
		t.Fatalf("purpose = %q, want a restoring pill", item.Purpose)
	}
	// A 下品 restoring pill is 10% of its registered stage's bare pool. At
	// qi-refining layer 6 the bare health max is 200, so the dose is 20 — the
	// catalogue's number, not one the story could have written.
	if item.RestoreAmount == nil || *item.RestoreAmount != 20 {
		t.Fatalf("restore amount = %v, want the catalogue's 20", item.RestoreAmount)
	}
	// Usability has to agree with the pool the same window reports: a wounded
	// combatant is offered the dose, and a full one is refused with the reason.
	// The opening round may or may not have drawn blood, so the assertion is the
	// consistency rather than one of the two outcomes.
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	wounded := *player.Health < *player.HealthMax
	if item.Usable != wounded {
		t.Fatalf("usability must follow the pool: usable=%v health=%d/%d", item.Usable, *player.Health, *player.HealthMax)
	}
	if !item.Usable && item.ReasonKey != "reason.pool_already_full" {
		t.Fatalf("a refused dose must name the pool: %q", item.ReasonKey)
	}
}

// TestCombatWindowOffersNoDoseTheRulesWouldRefuse keeps §8's "a refused use
// costs nothing" visible before the click rather than after it: a copy that is
// used up is reported with its count and its reason, and is not offered.
func TestCombatWindowOffersNoDoseTheRulesWouldRefuse(t *testing.T) {
	application, storyID := combatTestAppCarrying(t, map[string]any{
		"空瓶丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(0)},
		// A definition the catalogue does not hold has no published numbers, so it
		// is not stock the rules can spend.
		"伪造丹": map[string]any{"定义": "pill.restore.health.made_up", "数量": float64(3)},
	})
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	items := combatWindowSide(t, window, interactive.DefaultActorID).Items
	if len(items) != 1 {
		t.Fatalf("an unregistered definition is not stock: %#v", items)
	}
	if items[0].Usable || items[0].ReasonKey != "reason.item_exhausted" {
		t.Fatalf("a spent copy must be reported as spent: %#v", items[0])
	}
	if items[0].Quantity != 0 {
		t.Fatalf("quantity = %d, want the recorded 0", items[0].Quantity)
	}
}

// TestPlayCombatRoundSpendsADoseAndSavesTheStock is §4.2 end to end through the
// window's own write: 当前库存、消耗一个剂量都真实保存.
func TestPlayCombatRoundSpendsADoseAndSavesTheStock(t *testing.T) {
	// The wound is written down rather than hoped for: the rival's opening blow
	// is a draw, and a dose that is only effective on a hit would make this test
	// pass or fail by luck.
	application, storyID := combatTestAppCarryingWounded(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, 100)
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	wounded := combatWindowSide(t, opened, interactive.DefaultActorID)
	if *wounded.Health >= *wounded.HealthMax {
		t.Fatalf("the fixture must leave a wound for the dose to treat: %#v", wounded)
	}
	if !combatTestWindowItem(t, opened, interactive.DefaultActorID, "回春丹").Usable {
		t.Fatalf("a wounded combatant must be offered the restoring dose: %#v", wounded)
	}

	after, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-dose",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: "回春丹"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The dose took effect, which the settled step reports directly. Reading the
	// net pool instead would confuse the dose with whatever the other side did in
	// the same exchange.
	step, ok := combatTestStep(after.LastRound.Outcome, interactive.DefaultActorID)
	if !ok {
		t.Fatalf("the round must report the protagonist's step: %#v", after.LastRound.Outcome.Steps)
	}
	if !step.ItemApplied || step.HealthRecovered <= 0 {
		t.Fatalf("the dose must have taken effect: %#v", step)
	}
	// And the consumption is saved: the window reads it back from the journal.
	if remaining := combatTestWindowItem(t, after, interactive.DefaultActorID, "回春丹").Quantity; remaining != 2 {
		t.Fatalf("saved stock = %d, want 2 after one dose", remaining)
	}
}

// TestPlayCombatRoundDoesNotSpendASecondDoseOnARetry is the same command
// replayed, which is what a lost response or a repeated click produces.
func TestPlayCombatRoundDoesNotSpendASecondDoseOnARetry(t *testing.T) {
	// Wounded from the fixture, so the first dose really is spent and the retry
	// has something to not spend again.
	application, storyID := combatTestAppCarryingWounded(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, 100)
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	dose := CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-dose",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: "回春丹"},
		},
	}
	if _, err := application.PlayCombatRound(dose); err != nil {
		t.Fatal(err)
	}
	retried, err := application.PlayCombatRound(dose)
	if err != nil {
		t.Fatal(err)
	}
	if remaining := combatTestWindowItem(t, retried, interactive.DefaultActorID, "回春丹").Quantity; remaining != 2 {
		t.Fatalf("saved stock = %d after a retry, want 2 (one dose, spent once)", remaining)
	}
}

// TestCombatWindowRefusesAnUntrackedCombatant covers the failure boundary: a
// participant the rules do not track is refused, and nothing is settled
// partially.
func TestCombatWindowRefusesAnUntrackedCombatant(t *testing.T) {
	application, storyID := combatTestApp(t)

	_, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open",
		Combatants: []interactive.CombatantSpec{
			{ActorID: interactive.DefaultActorID, Side: cultivation.SidePlayer},
			{ActorID: "a-bystander", Side: cultivation.SideEnemy},
		},
	})
	if err == nil {
		t.Fatal("a combatant with no resolvable template must be refused")
	}
	sessions, err := application.interactive.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("a refused battle must not be partially settled: %#v", sessions)
	}
}

// TestCombatWindowOnAForkDoesNotShowTheParentsLaterBattle covers ownership: a
// battle belongs to the branch it was fought on, so a branch forked before it
// must not inherit it.
func TestCombatWindowOnAForkDoesNotShowTheParentsLaterBattle(t *testing.T) {
	application, storyID := combatTestApp(t)
	context, err := application.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if context.Snapshot.CurrentTurn == nil {
		t.Fatal("the fixture must commit a turn to fork from")
	}
	branch, err := application.interactive.CreateBranch(storyID, interactive.CreateBranchRequest{
		ParentEventID: context.Snapshot.CurrentTurn.ID, Title: "分叉",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The battle is opened on the fork, after the fork point.
	forked, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, BranchID: branch.ID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if forked.BranchID != branch.ID || len(forked.Sessions) != 1 {
		t.Fatalf("the battle must belong to the fork: %#v", forked)
	}

	parent, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range parent.Sessions {
		if sessionID == forked.SessionID {
			t.Fatalf("a battle fought after the fork must not appear on the parent: %#v", parent.Sessions)
		}
	}
	if _, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: forked.SessionID,
	}); !errors.Is(err, ErrCombatUnknown) {
		t.Fatalf("the parent branch must not answer for the fork's battle, got %v", err)
	}
}

// TestCombatWindowHistoryPageCutAtTheCursor covers paging as a pure contract,
// because it is the one part of the window with no store behind it to test.
func TestCombatWindowHistoryPageCutAtTheCursor(t *testing.T) {
	rounds := make([]CombatWindowRound, 0, 5)
	for index := 0; index < 5; index++ {
		rounds = append(rounds, CombatWindowRound{EventID: string(rune('a' + index))})
	}

	whole := combatWindowHistoryPage(rounds, "", 40)
	if whole.hasMore || whole.cursor != "" || len(whole.rounds) != 5 {
		t.Fatalf("a history inside the limit must be returned whole: %#v", whole)
	}

	paged := combatWindowHistoryPage(rounds, "", 2)
	if !paged.hasMore || paged.cursor != "d" || len(paged.rounds) != 2 {
		t.Fatalf("a bounded page must report the most recent rounds and a cursor: %#v", paged)
	}
	if paged.rounds[0].EventID != "d" || paged.rounds[1].EventID != "e" {
		t.Fatalf("the page must be the most recent rounds in commit order: %#v", paged.rounds)
	}

	earlier := combatWindowHistoryPage(rounds, paged.cursor, 2)
	if len(earlier.rounds) != 2 || earlier.rounds[0].EventID != "b" || earlier.rounds[1].EventID != "c" {
		t.Fatalf("paging back must cut at the cursor: %#v", earlier.rounds)
	}
	if !earlier.hasMore || earlier.cursor != "b" {
		t.Fatalf("a page with older rounds still behind it must say so: %#v", earlier)
	}

	// The oldest page is the one that runs out of history, and only there does
	// the window report that nothing earlier remains.
	oldest := combatWindowHistoryPage(rounds, earlier.cursor, 2)
	if len(oldest.rounds) != 1 || oldest.rounds[0].EventID != "a" {
		t.Fatalf("the oldest page must be what is left: %#v", oldest.rounds)
	}
	if oldest.hasMore || oldest.cursor != "" {
		t.Fatalf("the oldest page must report no more: %#v", oldest)
	}

	// An unknown cursor cuts at the end rather than dropping the history, so a
	// cursor from a branch that no longer holds it cannot empty the window.
	unknown := combatWindowHistoryPage(rounds, "zz", 40)
	if len(unknown.rounds) != 5 {
		t.Fatalf("an unknown cursor must not drop the history: %#v", unknown)
	}
}

func TestCombatWindowLimitClamps(t *testing.T) {
	tests := map[int]int{-5: 40, 0: 40, 1: 1, 40: 40, 200: 200, 500: 200}
	for input, want := range tests {
		if got := combatWindowLimit(input); got != want {
			t.Fatalf("combatWindowLimit(%d) = %d, want %d", input, got, want)
		}
	}
}

// TestCombatWindowHistorySurvivesAReopenedWindow is the recovery boundary at
// this layer: the history is read from the journal, so a window opened again
// reports the rounds that were committed rather than starting over.
func TestCombatWindowHistorySurvivesAReopenedWindow(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-round-1",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-round-2",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.History) != 1 || len(advanced.History) != 2 {
		t.Fatalf("two played rounds must be committed: %#v", advanced.History)
	}

	reopened, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, SessionID: opened.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.History) != 2 || reopened.History[0].EventID != advanced.History[0].EventID {
		t.Fatalf("a reopened window must report the same rounds: %#v", reopened.History)
	}
	if reopened.LastRound == nil {
		t.Fatal("a reopened window must report the round just settled")
	}
	if reopened.LastRound.Outcome.Round != advanced.LastRound.Outcome.Round {
		t.Fatalf("the last round must be the same exchange: %#v vs %#v", reopened.LastRound, advanced.LastRound)
	}
}

func TestPreparedTechniqueSurvivesStoryRestartAndContinuesIntoMainStory(t *testing.T) {
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		actor := &system.InitialActors[index]
		switch actor.ID {
		case interactive.DefaultActorID:
			actor.State[combatTestSkillsField] = learnedSkillRecords([]string{"strike", "meridian_secret_blade"})
			actor.State[combatTestImplementField] = combatTestBlade()
		case "rival":
			actor.State[combatTestHealthField] = 1
		}
	}
	application, storyID := combatTestAppWithSystem(t, system)
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		return `{"candidate_id":"rest"}`
	})
	secret, ok := cultivation.SkillByID("meridian_secret_blade")
	if !ok {
		t.Fatal("the published table must contain the secret technique")
	}
	start := CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, CommandID: "preparation-open", Combatants: combatTestSpecs(),
	}
	opened, err := application.PlayCombatRound(start)
	if err != nil {
		t.Fatal(err)
	}
	firstRound := CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, SessionID: opened.SessionID, CommandID: "preparation-start",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: secret, TargetID: "rival"},
		},
	}
	played, err := application.PlayCombatRound(firstRound)
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, played, interactive.DefaultActorID)
	if player.Preparation == nil || player.Preparation.SkillID != "meridian_secret_blade" || *player.Mana != 96 {
		t.Fatalf("paid preparation must be visible in the window: %#v", player)
	}
	startReplay, err := application.PlayCombatRound(firstRound)
	if err != nil {
		t.Fatal(err)
	}
	if len(startReplay.History) != 1 || *combatWindowSide(t, startReplay, interactive.DefaultActorID).Mana != 96 {
		t.Fatalf("replaying the same preparation command must not double-spend: %#v", startReplay)
	}

	if err := application.interactive.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore := interactive.NewStore(application.workspace)
	t.Cleanup(func() { _ = reopenedStore.Close() })
	application.interactive = reopenedStore
	reopened, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, SessionID: opened.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	player = combatWindowSide(t, reopened, interactive.DefaultActorID)
	if player.Preparation == nil || *player.Mana != 96 {
		t.Fatalf("restarting the story store must preserve the paid preparation: %#v", player)
	}

	continueRequest := CombatRoundRequest{
		SkipNarration: true,
		StoryID:       storyID, SessionID: opened.SessionID, CommandID: "preparation-continue",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionContinuePreparation},
		},
	}
	continued, err := application.PlayCombatRound(continueRequest)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := application.PlayCombatRound(continueRequest)
	if err != nil {
		t.Fatal(err)
	}
	if continued.LastRound == nil || retried.LastRound == nil || continued.LastRound.EventID != retried.LastRound.EventID {
		t.Fatalf("a repeated continue command must return the same committed exchange: first=%#v retry=%#v", continued.LastRound, retried.LastRound)
	}
	if *combatWindowSide(t, retried, interactive.DefaultActorID).Mana != 96 || combatWindowSide(t, retried, interactive.DefaultActorID).Preparation != nil {
		t.Fatalf("continuing must not charge twice and must clear the preparation: %#v", combatWindowSide(t, retried, interactive.DefaultActorID))
	}

	for attempt := 1; !retried.Ended && attempt <= 12; attempt++ {
		retried, err = application.PlayCombatRound(CombatRoundRequest{
			SkipNarration: true,
			StoryID:       storyID, SessionID: opened.SessionID, CommandID: fmt.Sprintf("finish-battle-%d", attempt),
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !retried.Ended || retried.Result != string(cultivation.ResultPlayerWon) {
		t.Fatalf("the synthetic battle must finish from its recorded combat result: %#v", retried)
	}
	handoffs, err := reopenedStore.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	fragment := interactive.CombatHandoffFragment(handoffs)
	if len(handoffs) != 1 || !handoffs[0].Settled || handoffs[0].Outcome.Result != cultivation.ResultPlayerWon || !strings.Contains(fragment, "The battle ended: the player's side won.") {
		t.Fatalf("the next main-story turn must read the actual settled battle result: handoffs=%#v fragment=%q", handoffs, fragment)
	}
	if _, _, err := reopenedStore.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路", Narrative: "沈砚从交手中脱身，接着往前走。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	if pending, err := reopenedStore.PendingCombatHandoffs(storyID, "main"); err != nil || len(pending) != 0 {
		t.Fatalf("after the main story absorbs the result, it must not be handed over again: pending=%#v err=%v", pending, err)
	}
}
