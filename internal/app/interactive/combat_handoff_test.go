package interactiveapp

import (
	"context"
	"strings"
	"testing"

	agentcontext "ta/internal/agents/context"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the app layer's half of the battle handoff: that the
// Conversation actually asks the journal what the main story has not yet been
// told, and that the answer reaches the model's input.
//
// The derivation itself is the store's and has its own suite. What is proven
// here is the wiring — a Conversation that never asked would leave the narrator
// writing as though the fight had not happened, which is the failure §2 of the
// handoff design names.

// handoffXiuxianSystem is the real xiuxian module with a second tracked
// template, so a battle has two sides whose numbers the rules own.
func handoffXiuxianSystem(t *testing.T) interactive.StoryDirectorActorStateSystem {
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
		State: map[string]any{"当前境界": stage},
	})
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultActorID {
			continue
		}
		state := system.InitialActors[index].State
		if state == nil {
			state = map[string]any{}
		}
		state["当前境界"] = stage
		system.InitialActors[index].State = state
	}
	return system
}

// handoffTestConversation builds a story with a settled cast and a conversation
// over it, so the handoff can be asked for against a real journal.
func handoffTestConversation(t *testing.T) (*interactive.Store, string, *Conversation) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	system := handoffXiuxianSystem(t)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "承接", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), root, story.ID, "main", "继续", 800, nil)
	return store, story.ID, conversation
}

func handoffFightToAResult(t *testing.T, store *interactive.Store, storyID string) string {
	t.Helper()
	opened, err := store.StartBattle(storyID, interactive.BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: []interactive.CombatantSpec{
			{ActorID: interactive.DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
			{ActorID: "rival", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
		},
		CommandID: "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	skill, ok := cultivation.SkillByID("strike")
	if !ok {
		t.Fatal("the published table must contain the ordinary attack")
	}
	ended := opened.Session.Ended
	for round := 2; round <= 40 && !ended; round++ {
		result, err := store.ResolveBattleRound(storyID, interactive.BattleRoundRequest{
			BranchID: "main", SessionID: opened.Session.ID,
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: skill, TargetID: "rival"},
			},
			CommandID: "cmd-" + string(rune('a'+round)), CommandHash: "hash-round",
		})
		if err != nil {
			t.Fatal(err)
		}
		ended = result.Session.Ended
	}
	if !ended {
		t.Fatal("the fixture must be able to finish a battle against an even opponent")
	}
	return opened.Session.ID
}

// TestConversationPreparesTheHandoffForABattleTheStoryHasNotAbsorbed is the
// wiring itself: the conversation must ask, and the answer must be the English
// fragment §7 specifies.
func TestConversationPreparesTheHandoffForABattleTheStoryHasNotAbsorbed(t *testing.T) {
	store, storyID, conversation := handoffTestConversation(t)
	handoffFightToAResult(t, store, storyID)

	fragment, err := conversation.pendingCombatHandoffFragment()
	if err != nil {
		t.Fatal(err)
	}
	if fragment == "" {
		t.Fatal("a battle the story has not absorbed must produce a handoff fragment")
	}
	if len(fragment) > interactive.CombatHandoffFragmentLimit {
		t.Fatalf("the fragment must respect the size limit: %d", len(fragment))
	}
	if !strings.Contains(fragment, "must not be fought again") {
		t.Fatalf("the fragment must forbid restaging the battle: %q", fragment)
	}
}

// TestConversationStopsHandingOverABattleTheStoryAbsorbed is §4's "one result
// decides once" at the app layer: once the player continues the story, the next
// cycle must not be told about the fight a second time.
func TestConversationStopsHandingOverABattleTheStoryAbsorbed(t *testing.T) {
	store, storyID, conversation := handoffTestConversation(t)
	handoffFightToAResult(t, store, storyID)

	if fragment, err := conversation.pendingCombatHandoffFragment(); err != nil {
		t.Fatal(err)
	} else if fragment == "" {
		t.Fatal("the fixture must start with the fight pending")
	}

	if _, _, err := store.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路", Narrative: "沈砚没有再回头看那道山道。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}

	fragment, err := conversation.pendingCombatHandoffFragment()
	if err != nil {
		t.Fatal(err)
	}
	if fragment != "" {
		t.Fatalf("a battle the story has already absorbed must not be handed over again: %q", fragment)
	}
}

// TestConversationPreparesNoHandoffForAnOrdinaryTurn keeps the common case clean:
// a story that has fought nothing must carry no battle instruction at all.
func TestConversationPreparesNoHandoffForAnOrdinaryTurn(t *testing.T) {
	_, _, conversation := handoffTestConversation(t)
	fragment, err := conversation.pendingCombatHandoffFragment()
	if err != nil {
		t.Fatal(err)
	}
	if fragment != "" {
		t.Fatalf("a story that has fought nothing must produce no fragment: %q", fragment)
	}
}

// handoffAssembledModelText renders every model-visible message the native
// runtime would send for one turn. The helper above proves the Conversation can
// compute the fragment; only this proves the fragment is actually placed.
func handoffAssembledModelText(t *testing.T, conversation *Conversation, userMessage string) string {
	t.Helper()
	assembled, err := conversation.AssembleModelContext(context.Background(), userMessage, agentcontext.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for _, message := range assembled.Messages {
		if message == nil {
			continue
		}
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}

// TestAssembledTurnContextCarriesTheHandoff is the regression test for the
// wiring defect: the handoff was derived correctly and asserted only through
// the helper above, while the assembly the native runtime actually sends never
// placed it. A story that fights a battle and then continues was therefore
// narrated as though the fight had not happened.
//
// The interactive Game defaults to the native runtime, so the native assembly
// is the live path; asserting the helper alone cannot catch a missing
// placement.
func TestAssembledTurnContextCarriesTheHandoff(t *testing.T) {
	store, storyID, conversation := handoffTestConversation(t)
	sessionID := handoffFightToAResult(t, store, storyID)

	assembled := handoffAssembledModelText(t, conversation, "我从这场战斗中脱出身来，接着往下走。")
	if !strings.Contains(assembled, "must not be fought again") {
		t.Fatalf("the assembled turn context must carry the battle handoff; got %q", assembled)
	}
	if !strings.Contains(assembled, sessionID) {
		t.Fatalf("the handoff must name the settled battle %q; got %q", sessionID, assembled)
	}
	if !strings.Contains(assembled, "Do not restage") {
		t.Fatalf("the handoff must forbid restaging in the assembled context; got %q", assembled)
	}
}

// TestAssembledTurnContextDropsTheHandoffOnceAbsorbed is the other half: the
// fragment must not become a permanent resident of every later turn. Once a
// Turn absorbs the fight the assembly must stop carrying it.
func TestAssembledTurnContextDropsTheHandoffOnceAbsorbed(t *testing.T) {
	store, storyID, conversation := handoffTestConversation(t)
	handoffFightToAResult(t, store, storyID)

	if assembled := handoffAssembledModelText(t, conversation, "接着走。"); !strings.Contains(assembled, "must not be fought again") {
		t.Fatalf("the fixture must start with the fight pending; got %q", assembled)
	}

	if _, _, err := store.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路", Narrative: "沈砚没有再回头看那道山道。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembled := handoffAssembledModelText(t, conversation, "沿山道往北走。")
	if strings.Contains(assembled, "must not be fought again") {
		t.Fatalf("an absorbed battle must not be handed over again: %q", assembled)
	}
}

// TestAssembledTurnContextCarriesNoHandoffWhenNothingWasFought keeps the
// ordinary turn free of battle instructions in the assembly itself, not only in
// the helper's answer.
func TestAssembledTurnContextCarriesNoHandoffWhenNothingWasFought(t *testing.T) {
	_, _, conversation := handoffTestConversation(t)
	assembled := handoffAssembledModelText(t, conversation, "我环顾四周。")
	if strings.Contains(assembled, "must not be fought again") {
		t.Fatalf("a story that has fought nothing must carry no battle instruction: %q", assembled)
	}
}
