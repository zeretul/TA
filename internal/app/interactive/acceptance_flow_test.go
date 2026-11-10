package interactiveapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ta/config"
	"ta/internal/agents/canonicalstore"
	agentchat "ta/internal/agents/chat"
	agentexecution "ta/internal/agents/execution"
	agentinteractive "ta/internal/agents/interactive"
	agentrun "ta/internal/agents/run"
	agenttoolruntime "ta/internal/agents/toolruntime"
	producttools "ta/internal/agents/tools"
	"ta/internal/interactive"
	"ta/internal/project"

	agent "ta/agent"
	agentpermission "ta/agent/permission"
)

// This file is the acceptance run the batch asks for: one ordinary natural
// sentence takes an errand the world really offered, the work already committed
// is credited back to it, and the promised payment is handed over exactly once —
// all as committed results, not as a schema, a receipt or a unit assertion.
//
// The runtime is the product one over a canonical journal, the same shape the
// native path uses, so what is proven here is the path a player actually runs.
// Every byte of data is synthetic and lives in t.TempDir(); no shared save, no
// fixed port and no credential is touched.

const (
	acceptanceOrigin   = "越国坊市外围散修人家"
	acceptanceTaskID   = "quest.herb_handover"
	acceptanceCurrency = "灵石"
)

// acceptanceStory builds a story bound to the shipped 凡人人界 pack and standing
// at the cave's own entrance, so the errand and the realm are both content this
// story's world really contains.
func acceptanceStory(t *testing.T, workspace, novaDir string) (*interactive.Store, string) {
	t.Helper()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	system := module.ActorState
	system.WorldContent = fanrenFormalPack(t, novaDir).Story.Gameplay.WorldContent
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State["当前详细地点"] = acceptanceOrigin
	}
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:       "药瓶差事",
		ActorState:  &system,
		PackBinding: fanrenFormalPack(t, novaDir).Binding(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "到山脚下看看",
		Narrative: "主角在散修人家门口站了一会儿，听人说山里有座塌了半边的古修洞府。",
		TurnResult: &interactive.TurnResult{Choices: []string{
			"进山看看", "先回屋里收拾", "问问路怎么走", "检查随身的家伙", "在门口等一等",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// acceptanceBackfill takes the herb out of the cave before any task is in play.
// It is the committed fact the acceptance has to credit: the player really did
// the work first, and 不要求补点"领取" says taking the errand afterwards must
// still pay the real result.
func acceptanceBackfill(t *testing.T, store *interactive.Store, storyID string) {
	t.Helper()
	for _, req := range []interactive.RealmRequest{
		{
			BranchID: "main", RealmID: "realm.gu_xiu_dong_fu", Action: interactive.RealmActionEnter,
			CommandID: "acceptance-enter", CommandHash: "h-enter",
		},
		{
			BranchID: "main", RealmID: "realm.gu_xiu_dong_fu", Action: interactive.RealmActionMove,
			ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot",
			CommandID: "acceptance-move", CommandHash: "h-move",
		},
		{
			BranchID: "main", RealmID: "realm.gu_xiu_dong_fu", Action: interactive.RealmActionReward,
			ContentRefID: "realm.herb_ref",
			CommandID:    "acceptance-reward", CommandHash: "h-reward",
		},
	} {
		if _, err := store.ApplyRealm(storyID, req); err != nil {
			t.Fatalf("the pre-acceptance realm fact %s must commit: %v", req.Action, err)
		}
	}
}

// acceptanceTurn builds one Game cycle exactly as the product does: the native
// submit tool, the protocol middleware, and the canonical journal runtime.
func acceptanceTurn(
	t *testing.T,
	store *interactive.Store,
	cfg *config.Config,
	storyID, message string,
	model agent.BaseChatModel,
	submissions *atomic.Int32,
) agentexecution.Cycle {
	t.Helper()
	conversation := NewConversation(store, "", cfg.Workspace, storyID, "main", message, 800, cfg)
	definitions, err := producttools.NewInteractiveTurn(producttools.InteractiveContext{
		Store: store, StoryID: storyID, BranchID: "main", RequestTurnCompletion: agentinteractive.RequestTurnCompletion,
		PrepareTurn:             conversation.PrepareInteractiveTurn,
		ResetNarrativeCandidate: agentinteractive.RequestNarrativeRewrite,
		SubmitTurnResult: func(ctx context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			submissions.Add(1)
			return conversation.SubmitTurnResult(ctx, input)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.test.acceptance", Version: 1}, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return agentexecution.Cycle{
		Definition: agent.Definition{
			Key: "test.game-acceptance", Name: "game", Model: model, Tools: toolset, Permission: agentpermission.FullAccess(),
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.acceptance", Version: 1},
			Execution:     agent.ExecutionPolicy{ModelMaxAttempts: 8},
			Middlewares: []agent.Middleware{agentinteractive.NewTurnProtocolMiddleware(agentinteractive.InteractiveStoryToolContext{
				TurnResultReady: conversation.InteractiveNarrativeReady, LoadNarrativeCandidate: conversation.LoadNarrativeCandidate,
				AcceptNarrativeCandidate: conversation.AcceptNarrativeCandidate,
			})},
		},
		Conversation: conversation,
		Request:      agentchat.ChatRequest{CommandID: "acceptance-" + message, Message: message},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: cfg.ProjectID, StoryID: storyID, BranchID: "main",
			Workspace: cfg.Workspace, Mode: "interactive",
		},
	}
}

// acceptanceWorldArgs builds the turn submission: the required story event, the
// choices, and the one world decision this turn settles.
func acceptanceWorldArgs(event, world string) string {
	return `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前事件","value":"` + event + `"}],` +
		`"choices":["继续当前行动","观察周围变化","询问在场人物","检查自身状态","暂时等待"],"world":` + world + `}`
}

func acceptanceQuestWorld(action string) string {
	return `{"action":"quest","task_id":"` + acceptanceTaskID + `","quest_action":"` + action + `"}`
}

// TestNaturalInputTakesAnErrandAndSettlesItOnce is the batch's acceptance bar:
// one real natural sentence per turn, through the product runtime, ending in a
// committed acceptance that credits work already done and a single payment.
func TestNaturalInputTakesAnErrandAndSettlesItOnce(t *testing.T) {
	ctx := t.Context()
	workspace, dataDir, novaDir := t.TempDir(), t.TempDir(), t.TempDir()
	store, storyID := acceptanceStory(t, workspace, novaDir)
	acceptanceBackfill(t, store, storyID)

	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Acceptance")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: workspace, ProjectID: record.ID}

	journal, err := canonicalstore.New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir,
		agentexecution.WithSessionStore(journal),
		agentexecution.WithProfiles(publicGameNoopProfile(workspace, storyID)),
		agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	before := acceptanceSnapshot(t, store, storyID)
	beforeCurrency := acceptancePurse(t, store, storyID)
	beforeClock := acceptanceWorld(t, store, storyID).Clock.Seconds

	// The player simply says they will take the errand the scene offered. The
	// prose settles nothing; the world module is what records it.
	acceptProse := "主角想起坊市里那桩托付，便应了下来：那只封了口的药瓶，他前几日在洞府药圃里已经取到手了。"
	model := &worldRefusalModel{responses: []*agent.Message{
		agent.AssistantMessage(acceptProse, nil),
		draftSubmissionMessage("accept", acceptanceWorldArgs("主角接下寻药的差事", acceptanceQuestWorld(interactive.QuestActionAccept))),
	}}
	var submissions atomic.Int32
	operation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: acceptanceTurn(t, store, cfg, storyID, "这桩寻药的差事我接下了，药瓶我手里就有", model, &submissions),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != acceptProse {
		t.Fatalf("the acceptance turn must commit its own prose: %+v", outcome)
	}

	afterAccept := acceptanceSnapshot(t, store, storyID)
	if afterAccept.TurnCount != before.TurnCount+1 || afterAccept.CurrentTurn.Narrative != acceptProse {
		t.Fatalf("the acceptance must append exactly one turn: before=%d after=%d narrative=%q",
			before.TurnCount, afterAccept.TurnCount, afterAccept.CurrentTurn.Narrative)
	}
	if submissions.Load() != 1 {
		t.Fatalf("one submission per turn: %d", submissions.Load())
	}

	// The committed task shows the work already done and no payment yet.
	task := acceptanceTask(t, store, storyID)
	if len(task.Objectives) != 1 || !task.Objectives[0].Completed {
		t.Fatalf("acceptance must credit the herb already taken: %#v", task.Objectives)
	}
	if task.Status != "completed" {
		t.Fatalf("crediting the only objective must complete the errand: %q", task.Status)
	}
	if task.Settled {
		t.Fatal("crediting the work must not pay; settle is still the only payment")
	}
	if got := acceptancePurse(t, store, storyID); got != beforeCurrency {
		t.Fatalf("acceptance must move no money: before=%d after=%d", beforeCurrency, got)
	}

	// A task turn settles no travel, so the formal clock is untouched.
	afterAcceptWorld := acceptanceWorld(t, store, storyID)
	if afterAcceptWorld.Clock.Seconds != beforeClock {
		t.Fatalf("a task decision must not spend formal time: before=%d after=%d", beforeClock, afterAcceptWorld.Clock.Seconds)
	}

	// The next turn is the payment. It is the ordinary thing the player says to
	// the person who promised it, not a keyword.
	settleProse := "主角把药瓶递过去，对方验看过后点点头，数出二十枚下品灵石推到他面前。"
	model.setResponses([]*agent.Message{
		agent.AssistantMessage(settleProse, nil),
		draftSubmissionMessage("settle", acceptanceWorldArgs("主角交药收下灵石", acceptanceQuestWorld(interactive.QuestActionSettle))),
	})
	nextOperation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: acceptanceTurn(t, store, cfg, storyID, "把药瓶交给他，说好的灵石请点给我", model, &submissions),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := nextOperation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != settleProse {
		t.Fatalf("the settle turn must commit its own prose: %+v", outcome)
	}

	afterSettle := acceptanceSnapshot(t, store, storyID)
	if afterSettle.TurnCount != afterAccept.TurnCount+1 || afterSettle.CurrentTurn.Narrative != settleProse {
		t.Fatalf("the settle must append exactly one turn: before=%d after=%d narrative=%q",
			afterAccept.TurnCount, afterSettle.TurnCount, afterSettle.CurrentTurn.Narrative)
	}
	if submissions.Load() != 2 {
		t.Fatalf("one submission per turn: %d", submissions.Load())
	}
	settled := acceptanceTask(t, store, storyID)
	if !settled.Settled {
		t.Fatal("the settle turn must record the payment")
	}
	if got, want := acceptancePurse(t, store, storyID), beforeCurrency+20; got != want {
		t.Fatalf("settling must add the agreed payment exactly once: before=%d after=%d want=%d", beforeCurrency, got, want)
	}
	if got := acceptanceWorld(t, store, storyID).Clock.Seconds; got != beforeClock {
		t.Fatalf("settling a task must not spend formal time: %d", got)
	}

	// The journal itself must carry one acceptance and one settlement, not a
	// success written by the prose.
	records := acceptanceQuestRecords(t, store, storyID)
	if len(records) != 2 {
		t.Fatalf("the errand must commit exactly two task records: %#v", records)
	}
	if records[0].Action != interactive.QuestActionAccept || records[0].Status != "completed" || records[0].Settled {
		t.Fatalf("the first record must be the acceptance that credited the work: %#v", records[0])
	}
	if records[1].Action != interactive.QuestActionSettle || !records[1].Settled {
		t.Fatalf("the second record must be the single settlement: %#v", records[1])
	}
}

type acceptanceQuestRecord struct {
	Action  string
	Status  string
	Settled bool
}

// acceptanceQuestRecords reads the committed task records out of the story's own
// journal, which is the artifact a restart replays.
func acceptanceQuestRecords(t *testing.T, store *interactive.Store, storyID string) []acceptanceQuestRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(store.Root(), "interactive", "story", "story-"+storyID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var records []acceptanceQuestRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var transaction struct {
			Records []json.RawMessage `json:"records"`
		}
		if err := json.Unmarshal([]byte(line), &transaction); err != nil {
			t.Fatalf("parse story journal line: %v", err)
		}
		for _, raw := range transaction.Records {
			var record struct {
				Type     string `json:"type"`
				Action   string `json:"action"`
				Instance struct {
					Status         string `json:"status"`
					SettledRewards bool   `json:"settled_rewards"`
				} `json:"instance"`
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatalf("parse story journal record: %v", err)
			}
			if record.Type != interactive.StoryEventTypeQuestRecord {
				continue
			}
			records = append(records, acceptanceQuestRecord{
				Action: record.Action, Status: record.Instance.Status, Settled: record.Instance.SettledRewards,
			})
		}
	}
	return records
}

func acceptanceSnapshot(t *testing.T, store *interactive.Store, storyID string) interactive.Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func acceptanceWorld(t *testing.T, store *interactive.Store, storyID string) interactive.WorldState {
	t.Helper()
	world, err := store.WorldStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return world
}

func acceptanceTask(t *testing.T, store *interactive.Store, storyID string) interactive.QuestStateView {
	t.Helper()
	state, err := store.QuestStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range state.Tasks {
		if task.TaskID == acceptanceTaskID {
			return task
		}
	}
	t.Fatalf("the accepted errand must be in play: %#v", state.Tasks)
	return interactive.QuestStateView{}
}

func acceptancePurse(t *testing.T, store *interactive.Store, storyID string) int64 {
	t.Helper()
	snapshot := acceptanceSnapshot(t, store, storyID)
	actors, _ := snapshot.State["actors"].(map[string]any)
	actor, _ := actors[interactive.DefaultActorID].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	switch value := values[acceptanceCurrency].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			t.Fatalf("the committed purse is not a whole number: %v", err)
		}
		return parsed
	default:
		return 0
	}
}
