package storypack

import (
	"encoding/json"
	"testing"

	"ta/internal/interactive"
)

// This exercises formal state commits, replay and historical branches with a
// deterministic adjudication fixture. It does not claim model rule compliance.
func TestRuleWorldFactCommitReplayAndBranchSurviveReopen(t *testing.T) {
	for _, p := range ruleWorldCandidates(t) {
		t.Run(p.ID, func(t *testing.T) {
			root := t.TempDir()
			lib := NewLibrary(root)
			if _, err := lib.materializeGameplayModules(&p); err != nil {
				t.Fatal(err)
			}
			module, err := interactive.NewActorStateLibrary(root).Get(p.Story.ModuleRefs.ActorStateID)
			if err != nil {
				t.Fatal(err)
			}
			var system interactive.StoryDirectorActorStateSystem
			raw, _ := json.Marshal(module.ActorState)
			if err := json.Unmarshal(raw, &system); err != nil {
				t.Fatal(err)
			}
			origin := p.ProtagonistTemplates[0]
			for i := range system.InitialActors {
				if system.InitialActors[i].ID == interactive.DefaultActorID {
					system.InitialActors[i].State = origin.State
				}
			}
			request, _, err := p.StoryRequestForTemplate(origin.ID)
			if err != nil {
				t.Fatal(err)
			}
			request.ActorState = &system
			request.StateSchemaPolicy = &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate}
			store := interactive.NewStore(root)
			defer store.Close()
			story, err := store.CreateStory(request)
			if err != nil {
				t.Fatal(err)
			}

			// An AI-mode story has no turn until its opening is formally committed.
			// This is a deterministic opening fixture, not a model-generated opening.
			if _, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
				BranchID: "main", User: "开始", Narrative: "你确认了当前处境。",
				TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "检查状态"}},
			}); err != nil {
				t.Fatal(err)
			}
			before, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "Before adjudication"})
			if err != nil {
				t.Fatal(err)
			}
			identity := interactive.DomainCommitIdentity{CommandID: "rule-fact-once", OperationID: "rule-fact-run", Cycle: 1}
			input, err := interactive.NewPlayerInputIntent(identity, "main", "在实际完成训练后记下本次结果。")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.CommitPlayerInput(story.ID, input); err != nil {
				t.Fatal(err)
			}
			// A complete author-shaped fact is supplied, not a per-world growth engine.
			var fact map[string]any
			raw, _ = json.Marshal(origin.State["历练状态"])
			if err = json.Unmarshal(raw, &fact); err != nil {
				t.Fatal(err)
			}
			if fact == nil {
				fact = map[string]any{}
			}
			fact["测试训练记录"] = "已完成且仅记一次"
			turn := interactive.AppendTurnWithStateRequest{BranchID: "main", User: input.Text, Narrative: "训练结束，你记录了实际完成的内容。", AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
				ActorOps:   []interactive.ActorStateOp{{Op: "set", ActorID: interactive.DefaultActorID, FieldID: "历练状态", Value: fact, Reason: "Deterministic fixture for an adjudicated fact"}},
				TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "检查状态"}}}
			first, _, err := store.AppendTurnWithState(story.ID, turn)
			if err != nil {
				t.Fatal(err)
			}
			again, _, err := store.AppendTurnWithState(story.ID, turn)
			if err != nil {
				t.Fatal(err)
			}
			if again.ID != first.ID {
				t.Fatal("replay created another formal turn")
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			cold := interactive.NewStore(root)
			defer cold.Close()
			read := func(branchID string) interactive.Snapshot {
				s, e := cold.Snapshot(story.ID, branchID)
				if e != nil {
					t.Fatal(e)
				}
				return s
			}
			main, old := read("main"), read(branch.ID)
			field := func(s interactive.Snapshot) any {
				return s.State["actors"].(map[string]any)[interactive.DefaultActorID].(map[string]any)["state"].(map[string]any)["历练状态"]
			}
			if interactive.ContentHashOf(field(main)) != interactive.ContentHashOf(fact) {
				t.Fatal("cold read lost committed rule facts")
			}
			if interactive.ContentHashOf(field(old)) != interactive.ContentHashOf(origin.State["历练状态"]) {
				t.Fatal("pre-adjudication branch inherited future facts")
			}
			if main.TurnCount != before.TurnCount+1 {
				t.Fatalf("replay counted %d turns", main.TurnCount-before.TurnCount)
			}
		})
	}
}
