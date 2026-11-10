package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	agentrun "ta/internal/agents/run"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/storypack"
)

// A freshly prepared adapt_template story has a frozen declaration, not live
// Actors. This cold-read assertion distinguishes that lifecycle from lost data.
func ruleCreationAssertWaiting(t *testing.T, store *interactive.Store, storyID string, template storypack.ProtagonistTemplate, overlay map[string]any) interactive.StoryContext {
	t.Helper()
	cold := interactive.NewStore(store.Root())
	defer cold.Close()
	view, err := cold.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if view.Meta.StateSchemaPolicy == nil || view.Meta.StateSchemaPolicy.Mode != interactive.StoryStateSchemaModeAdaptTemplate ||
		view.Meta.StateSchemaInitialization == nil || view.Meta.StateSchemaInitialization.Status != interactive.StateSchemaInitializationWaitingOpening {
		t.Fatalf("expected unchanged production waiting_opening contract, got policy=%+v status=%+v", view.Meta.StateSchemaPolicy, view.Meta.StateSchemaInitialization)
	}
	actors, _ := view.Snapshot.State["actors"].(map[string]any)
	if len(actors) != 0 || view.Snapshot.TurnCount != 0 {
		t.Fatal("opening tools published Actors or a turn before the atomic output commit")
	}
	if view.Meta.ActorStateSchema == nil {
		t.Fatal("prepared story has no frozen actor-state declaration")
	}
	var initial map[string]any
	for _, actor := range view.Meta.ActorStateSchema.System.InitialActors {
		if actor.ID == interactive.DefaultActorID {
			initial = actor.State
			break
		}
	}
	if initial == nil {
		t.Fatal("authored protagonist InitialActors declaration was lost before opening")
	}
	for key, want := range template.State {
		if value, ok := overlay[key]; ok {
			want = value
		}
		got, exists := initial[key]
		if !exists {
			t.Fatalf("frozen initial field disappeared before opening: %s", key)
		}
		ruleCreationEqual(t, "frozen initial field "+key, got, want)
	}
	for key, want := range overlay {
		ruleCreationEqual(t, "frozen confirmed field "+key, initial[key], want)
	}
	return view
}

// Use the same App opening instruction, cycle assembly, conversation tools,
// canonical hidden input and domain output transaction as the Game Agent.
// Only the Agent's schema decisions/scene prose are controlled test inputs.
// No real model is claimed; no selected-template state is copied into ActorOps
// or StateUpdates, and the production adapt_template policy is never changed.
func ruleCreationCompleteOpening(t *testing.T, application *App, storyID string, template storypack.ProtagonistTemplate, overlay map[string]any) {
	t.Helper()
	ctx := context.Background()
	before := ruleCreationAssertWaiting(t, application.interactive, storyID, template, overlay)
	instruction, err := application.InteractiveStoryOpeningInstruction(storyID, "main")
	if err != nil || strings.TrimSpace(instruction) == "" {
		t.Fatalf("product opening instruction: %v", err)
	}
	cycle, err := application.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
		CommandID: "rule-creation-opening-" + storyID, StoryID: storyID, BranchID: "main",
		Message: instruction, Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly,
	})
	if err != nil {
		t.Fatalf("product opening cycle: %v", err)
	}
	conversation := cycle.conversation
	bindInteractiveCycleForTest(conversation)
	inputReceipt, err := conversation.MaterializeAgentCanonicalInput(ctx, instruction, nil, nil)
	if err != nil || !inputReceipt.Event.ContextOnly {
		t.Fatalf("opening instruction must be a canonical model-only input: %+v %v", inputReceipt, err)
	}

	fields := map[string]interactive.ActorStateField{}
	for _, actorTemplate := range before.Meta.ActorStateSchema.System.Templates {
		if actorTemplate.ID == interactive.DefaultActorID {
			for _, field := range actorTemplate.Fields {
				fields[field.Name] = field
			}
		}
	}
	// This one newly declared ledger has no selected instance in the authored
	// starts. Empty means no camp facts, never zero stock or a granted command.
	emptyCampFacts := false
	if field, declared := fields["营务事实"]; declared {
		if before.Meta.PackBinding == nil || before.Meta.PackBinding.PackID != "sanguo-190-ta07-full-gm-candidate" || field.Type != "object" || field.Default != nil {
			t.Fatal("unexpected camp-facts opening declaration; review its semantics before adding a fixture")
		}
		if _, authored := template.State["营务事实"]; authored {
			t.Fatal("camp-facts fixture cannot replace authored initial instances")
		}
		emptyCampFacts = true
	}
	keys := make([]string, 0, len(template.State))
	for key := range template.State {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	reviews := make([]interactive.ActorStateSchemaRequirementReview, 0, len(keys))
	for _, key := range keys {
		field, exists := fields[key]
		if !exists {
			t.Fatalf("author field missing from the production opening schema: %s", key)
		}
		reviews = append(reviews, interactive.ActorStateSchemaRequirementReview{
			Source:      interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"},
			Requirement: "所选身份已确认的“" + key + "”事实须原样保留，不在初始化时赠送成长。",
			ValuePolicy: "schema_only", ExpectedType: field.Type, Decision: "covered",
			TemplateID: interactive.DefaultActorID, FieldID: key,
		})
	}
	batch, err := conversation.SubmitOpeningStateSchemaBatch(ctx, interactive.ActorStateSchemaBatch{
		Summary: "仅确认现有作者字段结构，保留所选身份的完整初始事实。", Finalize: true,
		Items: []interactive.ActorStateSchemaBatchItem{{ItemID: "preserve-authored-opening", Requirements: reviews}},
	})
	if err != nil || !batch.Finalized || len(batch.Rejected) != 0 || len(batch.Blocked) != 0 || batch.InitializationGuide == nil {
		t.Fatalf("production schema-only opening tool failed: result=%+v err=%v", batch, err)
	}
	ruleCreationAssertWaiting(t, application.interactive, storyID, template, overlay)

	// Only missing non-authored scene/appearance fields and the explicitly
	// empty Sanguo camp-facts ledger are supplied.
	// A missing author value is a regression, never a request to re-seed it here.
	updates := []interactivestate.Update{}
	for _, missing := range batch.InitializationGuide.RequiredStateChanges {
		if missing.ActorID == interactive.DefaultActorID {
			if _, authored := template.State[missing.FieldID]; authored {
				t.Fatalf("opening tool lost authored initial value: %s", missing.FieldID)
			}
			if _, confirmed := overlay[missing.FieldID]; confirmed {
				t.Fatalf("opening tool lost confirmed identity value: %s", missing.FieldID)
			}
		}
		value := ruleCreationOpeningFixtureValue(t, missing)
		updates = append(updates, interactivestate.Update{
			Op:   interactivestate.Replace,
			Path: interactivestate.FormatPath([]string{missing.ActorID, missing.FieldID}), Value: value,
		})
	}
	choices := ruleCreationFixtureChoices(before.Meta.ChoiceCount)
	submission := interactive.TurnSubmissionInput{
		StateUpdates: &updates, Choices: &choices,
		World: &interactive.TurnWorldActionInput{Action: interactive.TurnWorldActionNone},
	}
	if before.Meta.PlanningMode == interactive.StoryPlanningModeEnabled && before.Snapshot.BranchPlan == nil {
		submission.PlanUpdate = &interactive.TurnPlanUpdateInput{
			Mode:     interactive.TurnPlanUpdateModeReplaceDocument,
			Markdown: "保持作者初态与自主选择；未接受委托，不授予成长或额外资源。",
		}
	}
	receipt, err := conversation.SubmitTurnResult(ctx, submission)
	if err != nil || !receipt.Ready {
		t.Fatalf("production opening submit_interactive_turn failed: receipt=%+v err=%v", receipt, err)
	}
	narrative := strings.TrimSpace(before.Meta.Opening.CustomText)
	if narrative == "" {
		narrative = "你在开局现场停下脚步，查看随身行装。还没有接受新的委托，也没有获得额外资源。"
	}
	if err := conversation.AppendAssistantWithThinking(narrative, ""); err != nil {
		t.Fatalf("stage opening narrative: %v", err)
	}
	ruleCreationAssertWaiting(t, application.interactive, storyID, template, overlay)
	if err := conversation.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatalf("atomic opening domain commit: %v", err)
	}

	cold := interactive.NewStore(application.interactive.Root())
	defer cold.Close()
	after, err := cold.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	status := after.Meta.StateSchemaInitialization
	if status == nil || status.Status != interactive.StateSchemaInitializationReady || status.Outcome != "unchanged" {
		t.Fatalf("opening did not finalize the unchanged schema: %+v", status)
	}
	if after.Snapshot.TurnCount != 1 || len(after.Snapshot.Turns) != 1 {
		t.Fatalf("opening must commit exactly one turn: %+v", after.Snapshot.Turns)
	}
	turn := after.Snapshot.Turns[0]
	if status.SourceTurnID != turn.ID || !turn.UserContextOnly || turn.StateStatus != "ready" || turn.StateDelta == nil {
		t.Fatalf("opening status/hidden input/bootstrap is not tied to the same committed turn: status=%+v turn=%+v", status, turn)
	}
	if after.Meta.ActorStateSchema == nil || after.Meta.ActorStateSchema.Adaptation == nil || after.Meta.ActorStateSchema.Adaptation.Source != "game_agent" {
		t.Fatal("opening bypassed the production foreground schema audit")
	}
	if emptyCampFacts {
		state := actorStateOf(t, after.Snapshot.State, interactive.DefaultActorID)
		facts, ok := state["营务事实"].(map[string]any)
		if !ok || len(facts) != 0 {
			t.Fatalf("empty camp ledger gained instances, resources, troops or ownership: %#v", state["营务事实"])
		}
		// No new camp/squad/person Actors may be smuggled in by initialization.
		actors, _ := after.Snapshot.State["actors"].(map[string]any)
		expectedActors := map[string]bool{}
		for _, actor := range before.Meta.ActorStateSchema.System.InitialActors {
			expectedActors[actor.ID] = true
		}
		if len(actors) != len(expectedActors) {
			t.Fatalf("empty camp ledger changed the initial Actor roster: got %d want %d", len(actors), len(expectedActors))
		}
		for id := range actors {
			if !expectedActors[id] {
				t.Fatalf("empty camp ledger created an undeclared Actor: %s", id)
			}
		}
		world, err := cold.TurnWorldView(storyID, "main", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(world.LocalAffairs) != 0 || (world.Characters != nil && len(world.Characters.FactionMemberships) != 0) {
			t.Fatal("empty camp ledger created a local affair or granted formal faction authority")
		}
	}
	if _, err := application.InteractiveStoryOpeningInstruction(storyID, "main"); err == nil {
		t.Fatal("product opening endpoint still accepts an already opened story")
	}
}

// These are explicit test-only scene details, not a model-authored simulation
// or inferred canon. Unknown missing fields fail rather than being fabricated.
func ruleCreationOpeningFixtureValue(t *testing.T, field interactive.OpeningStateInitializationField) any {
	t.Helper()
	if field.ActorID == interactive.DefaultActorID && field.TemplateID == interactive.DefaultActorID && field.FieldID == "营务事实" && field.Type == "object" {
		// No instance key, stock balance, headcount, capacity or ownership.
		return map[string]any{}
	}
	if field.Type != "string" {
		t.Fatalf("unhandled non-authored opening fixture field: %+v", field)
	}
	switch field.ActorID + "/" + field.FieldID {
	case "protagonist/外貌与装扮":
		return "朴素的行装。"
	case "story/当前时间":
		return "受控测试的开局时刻"
	case "story/当前详细地点":
		return "受控测试的开局现场"
	case "story/当前事件":
		return "主角停下脚步查看随身行装。"
	case "story/可承接钩子":
		return "主角准备整理行囊。"
	case "story/世界局势":
		return "开局现场尚未发生新的局势变化。"
	default:
		t.Fatalf("unhandled non-authored opening fixture field: %+v", field)
		return nil
	}
}

// Count the declared catalogue, not subtests selected by go test -run. Each
// selected pack separately checks its actual template count against this table.
// A full run still visits all 65; a filtered run is not falsely called 65 runs.
func ruleCreationTemplateCounts(t *testing.T) map[string]int {
	t.Helper()
	expected := map[string]int{
		"doupo-continent-ta07-full-gm-candidate":           16,
		"harry-potter-1991-ta07-full-gm-candidate":         8,
		"harry-potter-1991-student-ta07-full-gm-candidate": 6,
		"lord-of-mysteries-ta07-full-gm-candidate":         11,
		"sanguo-190-ta07-full-gm-candidate":                12,
		"zichuan-xichuan-ta07-full-gm-candidate":           12,
	}
	if len(ruleCreationPackIDs) != 6 || len(expected) != len(ruleCreationPackIDs) {
		t.Fatal("expected exactly six authored configurations")
	}
	seen := map[string]bool{}
	total := 0
	for _, packID := range ruleCreationPackIDs {
		count, exists := expected[packID]
		if !exists || seen[packID] {
			t.Fatalf("missing or duplicate authored configuration: %s", packID)
		}
		seen[packID] = true
		total += count
	}
	if total != 65 {
		t.Fatalf("authored catalogue declares %d templates, want 65", total)
	}
	return expected
}

func ruleCreationFixtureChoices(count int) []string {
	choices := make([]string, count)
	for index := range choices {
		choices[index] = fmt.Sprintf("观察开局现场的第%d处细节", index+1)
	}
	return choices
}

// Keep a verbatim contiguous body substring. Collapsing newlines would let a
// leaked multiline GM body evade strings.Contains in the fake model request.
func ruleCreationGMBodyMarker(t *testing.T, content string) string {
	t.Helper()
	var longest []rune
	for _, line := range strings.Split(content, "\n") {
		runes := []rune(strings.TrimSpace(line))
		if len(runes) > len(longest) {
			longest = runes
		}
	}
	if len(longest) < 40 {
		t.Fatal("GM fixture has no sufficiently specific body line to audit")
	}
	if len(longest) > 100 {
		longest = longest[:100]
	}
	marker := string(longest)
	if !strings.Contains(content, marker) {
		t.Fatal("GM leakage marker is not a verbatim part of imported content")
	}
	return marker
}
