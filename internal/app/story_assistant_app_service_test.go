package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ta/internal/interactive"
)

// These tests hold the in-story assistant to the boundaries
// 自制-剧情助手与选项分析 states, at the layer that assembles what the model is
// allowed to see.
//
// The assistant's model call is not exercised here — that would need a live
// endpoint and would only prove the prompt reached it. What can be proven
// offline is the part that decides what is even available to leak: the context
// it builds, the turn it binds the question to, and the fact that asking writes
// nothing.

const assistantTestPrivateField = combatTestSkillsField

// assistantTestContext builds a story with a settled cast and a frozen schema,
// then returns the story context the assistant reads.
func assistantTestContext(t *testing.T) (*App, string, interactive.StoryContext) {
	t.Helper()
	app, storyID := combatTestApp(t, "strike")
	store := app.interactive
	storyContext, err := store.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return app, storyID, storyContext
}

// TestTheAssistantSeesNoPrivateDossier is the information boundary.
//
// The rival carries learned techniques, and §3.2 makes those private. The
// assistant is handed the same projection the player's snapshot gets, so the
// technique cannot appear in its context at all — not because the prompt says
// not to mention it, but because it was never assembled.
func TestTheAssistantSeesNoPrivateDossier(t *testing.T) {
	app, _, storyContext := assistantTestContext(t)
	known := storyAssistantKnownContext(app.interactive, storyContext)
	if known == "" {
		t.Fatal("the assistant must receive the situation it is asked about")
	}

	rivalState := storyContext.Snapshot.State["actors"].(map[string]any)["rival"].(map[string]any)
	skills, _ := rivalState["state"].(map[string]any)[assistantTestPrivateField]
	if skills == nil {
		t.Fatalf("the fixture must give the rival a private field to leak: %#v", rivalState)
	}
	leaked, _ := skills.(map[string]any)["strike"].(map[string]any)
	if leaked == nil {
		t.Fatalf("the fixture must record the rival's learned technique: %#v", skills)
	}

	// Neither the field name nor its content may reach the assistant's context.
	for _, secret := range []string{assistantTestPrivateField, "掌握程度"} {
		if strings.Contains(known, secret) {
			t.Fatalf("§3.2: the assistant's context must not carry %q:\n%s", secret, known)
		}
	}

	// The protagonist's own numbers are the player's own, so they stay: this is
	// about withholding someone else's dossier, not about blinding the advisor.
	protagonist := storyContext.Snapshot.State["actors"].(map[string]any)[interactive.DefaultActorID].(map[string]any)
	if protagonist["state"].(map[string]any)[combatTestRealmField] == nil {
		t.Fatal("the fixture must give the protagonist a realm")
	}
	if !strings.Contains(known, combatTestRealmField) {
		t.Fatalf("the protagonist's own situation belongs in the context:\n%s", known)
	}
}

// TestTheAssistantContextMatchesThePlayerProjection keeps the two reads from
// drifting. If the snapshot withholds a field, the assistant must not see it;
// the cheapest way to keep that true is to build from the same projection, and
// this test fails if someone builds the context from the raw state instead.
func TestTheAssistantContextMatchesThePlayerProjection(t *testing.T) {
	app, _, storyContext := assistantTestContext(t)
	raw := storyContext.Snapshot.State
	projected := playerKnownState(raw, storyContext.Snapshot.ActorStateSchema)

	rivalRaw := raw["actors"].(map[string]any)["rival"].(map[string]any)["state"].(map[string]any)
	rivalProjected := projected["actors"].(map[string]any)["rival"].(map[string]any)["state"].(map[string]any)
	if _, present := rivalRaw[assistantTestPrivateField]; !present {
		t.Fatal("the fixture must record the rival's private field in the raw state")
	}
	if _, present := rivalProjected[assistantTestPrivateField]; present {
		t.Fatal("the projection must withhold the rival's private field")
	}

	// The assistant's rendered context is what the projection allows, so the
	// withheld field has no route into it.
	known := storyAssistantKnownContext(app.interactive, storyContext)
	if strings.Contains(known, assistantTestPrivateField) {
		t.Fatalf("the assistant's context must follow the projection:\n%s", known)
	}
}

// TestAQuestionAboutALeftTurnIsRefused is the stale-response binding.
//
// The design requires the request to be bound to story, branch and turn. A
// question asked about turn N, answered after the branch moved to N+1, describes
// a scene the player has left; showing it as current advice would be worse than
// showing nothing.
func TestAQuestionAboutALeftTurnIsRefused(t *testing.T) {
	app, storyID, storyContext := assistantTestContext(t)
	current := storyContext.Snapshot.CurrentTurn
	if current == nil {
		t.Fatal("the fixture must have a committed turn")
	}

	_, err := app.AskStoryAssistant(t.Context(), StoryAssistantRequest{
		StoryID: storyID, BranchID: "main", TurnID: "turn-that-is-not-current",
		Action: "analyze",
	})
	if !errors.Is(err, ErrStoryAssistantStaleTurn) {
		t.Fatalf("a question about a left turn must be refused: %v", err)
	}

	// The same request bound to the current turn is accepted far enough to reach
	// the model; it fails on the model call instead, which is what proves the
	// binding check is the only thing that refused the first one.
	_, err = app.AskStoryAssistant(t.Context(), StoryAssistantRequest{
		StoryID: storyID, BranchID: "main", TurnID: current.ID,
		Action: "analyze",
	})
	if errors.Is(err, ErrStoryAssistantStaleTurn) {
		t.Fatalf("the current turn must not be treated as stale: %v", err)
	}
}

// TestTheAssistantRejectsAnUnknownAction keeps a typo from being answered with
// something plausible but unrequested.
func TestTheAssistantRejectsAnUnknownAction(t *testing.T) {
	app, storyID, _ := assistantTestContext(t)
	for _, action := range []string{"", "predict", "write", "ANALYZE!"} {
		_, err := app.AskStoryAssistant(t.Context(), StoryAssistantRequest{
			StoryID: storyID, BranchID: "main", Action: action,
		})
		if !errors.Is(err, ErrStoryAssistantUnknownAction) {
			t.Fatalf("action %q must be refused: %v", action, err)
		}
	}
}

// TestAskingTheAssistantWritesNothing is the read-only requirement, checked on
// the journal rather than on the answer.
//
// Analysis and inspiration cost a model call and nothing else: no turn, no
// branch head, no state, no plan, no time. The test compares the whole
// projection before and after, so anything the call might have committed shows
// up as a difference.
func TestAskingTheAssistantWritesNothing(t *testing.T) {
	app, storyID, before := assistantTestContext(t)
	journalBefore := assistantTestJournalSize(t, app, storyID)

	// This call fails at the model, which is exactly the point: the refusal must
	// not have written anything either.
	_, _ = app.AskStoryAssistant(t.Context(), StoryAssistantRequest{
		StoryID: storyID, BranchID: "main", Action: "inspire",
		PlayerInput: "我想找条稳妥的路出城",
	})

	after, err := app.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if journalAfter := assistantTestJournalSize(t, app, storyID); journalAfter != journalBefore {
		t.Fatalf("asking the assistant must not write a journal record: %d → %d", journalBefore, journalAfter)
	}
	if after.Snapshot.ContextRevision != before.Snapshot.ContextRevision {
		t.Fatalf("asking the assistant must not change the context revision: %d → %d",
			before.Snapshot.ContextRevision, after.Snapshot.ContextRevision)
	}
	if !reflect.DeepEqual(after.Snapshot.State, before.Snapshot.State) {
		t.Fatal("asking the assistant must not change story state")
	}
	if after.Snapshot.CurrentTurn == nil || before.Snapshot.CurrentTurn == nil ||
		after.Snapshot.CurrentTurn.ID != before.Snapshot.CurrentTurn.ID {
		t.Fatal("asking the assistant must not add a turn")
	}
}

// assistantTestJournalSize counts the records the story journal physically
// holds. It reads the file rather than the projection so a write that does not
// yet show up in the projection still counts.
func assistantTestJournalSize(t *testing.T, app *App, storyID string) int {
	t.Helper()
	path := filepath.Join(app.workspace, "interactive", "story", "story-"+storyID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the story journal %s: %v", path, err)
	}
	return strings.Count(strings.TrimSpace(string(data)), "\n") + 1
}

// TestAdoptingADirectionWithNoPlanIsRefused covers the honest refusal: a branch
// whose planning is off has nowhere to record a direction, and inventing a plan
// to hold it would be a second source of truth.
func TestAdoptingADirectionWithNoPlanIsRefused(t *testing.T) {
	app, storyID := combatTestApp(t)
	_, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", Direction: "先去城南的药铺打听车马行的消息",
	})
	if !errors.Is(err, ErrStoryAssistantNoPlan) {
		t.Fatalf("a branch without a plan must refuse an adopted direction: %v", err)
	}
}

// TestAdoptingADirectionRefusesAStaleTurn keeps an adopted wish from landing on
// a situation that is over.
func TestAdoptingADirectionRefusesAStaleTurn(t *testing.T) {
	app, storyID := assistantTestAppWithPlan(t)
	_, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", TurnID: "turn-that-is-not-current",
		Direction: "先去城南的药铺打听车马行的消息",
	})
	if !errors.Is(err, ErrStoryAssistantStaleTurn) {
		t.Fatalf("a direction adopted at a left turn must be refused: %v", err)
	}
}

// TestAdoptingADirectionSurvivesAReload is the design's acceptance check: adopt
// a direction, reopen the store, and read it back on the same branch.
func TestAdoptingADirectionSurvivesAReload(t *testing.T) {
	app, storyID := assistantTestAppWithPlan(t)
	const direction = "先去城南的药铺打听车马行的消息"

	// The revision the interface shows the player, and the one a retry after a
	// lost response would still be carrying.
	before, err := app.interactive.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.BranchPlan == nil {
		t.Fatal("the fixture must start with a plan")
	}
	shownRevision := before.BranchPlan.Revision

	adopted, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", Direction: direction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !adopted.Applied || adopted.Scope != "branch" {
		t.Fatalf("the direction must be recorded on the branch: %#v", adopted)
	}
	if !strings.Contains(adopted.BranchPlan.Markdown, direction) {
		t.Fatalf("the result must report the plan that holds the direction: %q", adopted.BranchPlan.Markdown)
	}

	// Reopen from disk: the direction is part of the journal's plan record, not
	// something the running process remembers.
	reopened := interactive.NewStore(app.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	snapshot, err := reopened.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BranchPlan == nil || !strings.Contains(snapshot.BranchPlan.Markdown, direction) {
		t.Fatalf("the adopted direction must be readable after a reload: %#v", snapshot.BranchPlan)
	}

	// A second adoption of the same wording is a read, so a retried request
	// cannot write the line twice.
	again, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", Direction: direction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Applied {
		t.Fatal("re-adopting the same direction must not write again")
	}
	if strings.Count(again.BranchPlan.Markdown, direction) != 1 {
		t.Fatalf("the direction must appear once: %q", again.BranchPlan.Markdown)
	}

	// The retry a lost response produces carries the revision the player was
	// shown, which the first attempt has already replaced. It must still read
	// back as "already recorded" rather than as a conflict: the adoption
	// succeeded, and the caller only needs to know that.
	retried, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", Direction: direction,
		BaseRevision: shownRevision,
	})
	if err != nil {
		t.Fatalf("a lost-response retry must not be refused: %v", err)
	}
	if retried.Applied {
		t.Fatal("a lost-response retry must not write again")
	}
	if strings.Count(retried.BranchPlan.Markdown, direction) != 1 {
		t.Fatalf("the direction must still appear once: %q", retried.BranchPlan.Markdown)
	}
}

// TestAdoptingADirectionIsBranchScoped is the branch-isolation check: a wish
// made on one branch must not appear on a sibling branch that forked before it.
func TestAdoptingADirectionIsBranchScoped(t *testing.T) {
	app, storyID := assistantTestAppWithPlan(t)
	const direction = "先去城南的药铺打听车马行的消息"

	store := app.interactive
	mainSnapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	forked, err := store.CreateBranch(storyID, interactive.CreateBranchRequest{
		ParentEventID: mainSnapshot.CurrentTurn.ID, Title: "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main", Direction: direction,
	}); err != nil {
		t.Fatal(err)
	}

	forkSnapshot, err := store.Snapshot(storyID, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forkSnapshot.BranchPlan != nil && strings.Contains(forkSnapshot.BranchPlan.Markdown, direction) {
		t.Fatalf("a direction adopted on main must not reach the fork: %q", forkSnapshot.BranchPlan.Markdown)
	}
}

// TestAdoptingADirectionRefusesAStalePlanRevision keeps an older plan from being
// written back over a newer one.
func TestAdoptingADirectionRefusesAStalePlanRevision(t *testing.T) {
	app, storyID := assistantTestAppWithPlan(t)
	store := app.interactive
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Someone else edits the plan first.
	if _, err := store.UpdateBranchPlan(storyID, interactive.UpdateBranchPlanRequest{
		BranchID:     "main",
		Markdown:     snapshot.BranchPlan.Markdown + "\n\n## Cast\n\nA ferryman.",
		BaseRevision: snapshot.BranchPlan.Revision,
	}); err != nil {
		t.Fatal(err)
	}

	_, err = app.AdoptStoryAssistantDirection(t.Context(), StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: "main",
		Direction:    "先去城南的药铺打听车马行的消息",
		BaseRevision: snapshot.BranchPlan.Revision,
	})
	if !errors.Is(err, interactive.ErrBranchPlanRevisionConflict) {
		t.Fatalf("a stale plan revision must be refused: %v", err)
	}
}

// assistantTestAppWithPlan builds a story whose branch already has a plan, which
// is what makes a direction adoptable.
//
// The story is created with planning enabled and a full choice set, because a
// committed turn's result is validated against the story's own settings: a
// plan update only survives normalization when the story is in planning mode,
// and a short choice list is refused outright.
func assistantTestAppWithPlan(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	system := combatTestSystem(t)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "剧情助手", ActorState: &system,
		PlanningMode: interactive.StoryPlanningModeEnabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	const plan = "## Long-term direction\n\nReach the sealed city.\n\n## Near-term beats\n\nFind a safe route."
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "主角在原地打量四周。",
		TurnResult: &interactive.TurnResult{
			Choices:    []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
			PlanUpdate: stringPtr(plan),
		},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

func stringPtr(value string) *string { return &value }

// TestAnAnswerWithBrokenJSONKeepsItsReadableFields covers what a real model did
// on 2026-10-31: it closed a field and then kept writing prose, so the object was
// invalid as a whole ("invalid character '这' after object key:value pair") while
// the analysis and every suggestion in it were complete. The call had already
// succeeded, so the answer has to survive a formatting fault.
func TestAnAnswerWithBrokenJSONKeepsItsReadableFields(t *testing.T) {
	content := `{"read": "他把剑换到右手，先看这条胳膊还听不听话。", "suggestions": [{"label": "换手试剑", "detail": "当着钱守信的面比划一下", "action": "我把剑换到右手，比划一个起手式。"}, {"label": "先问旧事", "action": "我问钱守信：上月荒坡那一趟他知道多少。"}]} 这之后还有一句多余的散文。`

	read, suggestions, err := parseStoryAssistantPayload(content)
	if err != nil {
		t.Fatalf("a complete answer with trailing prose was refused: %v", err)
	}
	if !strings.Contains(read, "换到右手") {
		t.Fatalf("the analysis was lost: %q", read)
	}
	if len(suggestions) != 2 {
		t.Fatalf("want 2 suggestions, got %d: %+v", len(suggestions), suggestions)
	}
	if suggestions[1].Action != "我问钱守信：上月荒坡那一趟他知道多少。" {
		t.Fatalf("second action was mangled: %q", suggestions[1].Action)
	}
}

// A suggestion with no action is still dropped: the panel offers these as things
// the player can send, so one that cannot be sent would be a dead button.
func TestASalvagedAnswerStillDropsAnUnsendableSuggestion(t *testing.T) {
	content := `{"read": "门口那两个人看的是你腰间的袋子。" 他们说完了。`

	read, suggestions, err := parseStoryAssistantPayload(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if read == "" {
		t.Fatal("the analysis was lost")
	}
	if len(suggestions) != 0 {
		t.Fatalf("a suggestion with no action survived: %+v", suggestions)
	}
}

// Nothing readable at all is still a refusal, so the player is told the answer
// could not be read instead of being shown an empty panel.
func TestAnAnswerWithNothingReadableIsRefused(t *testing.T) {
	if _, _, err := parseStoryAssistantPayload(`{"suggestions": [{"label": "x"}`); !errors.Is(err, ErrStoryAssistantUnreadableAnswer) {
		t.Fatalf("want ErrStoryAssistantUnreadableAnswer, got %v", err)
	}
}
