package interactiveapp

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ta/config"
	"ta/internal/agents"
	"ta/internal/agents/canonicalstore"
	agentchat "ta/internal/agents/chat"
	agentexecution "ta/internal/agents/execution"
	agentinteractive "ta/internal/agents/interactive"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/runtime/external"
	agenttoolruntime "ta/internal/agents/toolruntime"
	producttools "ta/internal/agents/tools"
	"ta/internal/interactive"
	"ta/internal/project"

	agent "ta/agent"
	agentpermission "ta/agent/permission"
)

// These are the two halves of the N10 recovery contract on the Native path:
// a world action the rules refuse must invalidate the prose that narrated it,
// the model's rewrite must be accepted, and the turn must commit exactly once
// with the corrected prose and the real outcome.
//
// The tool-level unit test proves the submit tool calls the reset. Only a full
// cycle proves the protocol middleware then accepts the rewrite instead of
// deadlocking on the refused candidate, which is the defect this batch fixes.
// The runtime here is the product one over a canonical journal: an ephemeral
// runtime has no journal, and product acceptance refuses to commit through it.

// worldRefusalStory builds a xiuxian story whose committed scene puts the
// protagonist at the market, so walking the market's own outward road is
// refused by the rules while a walk from the outskirts is legal.
func worldRefusalStory(t *testing.T, workspace string) (*interactive.Store, string) {
	t.Helper()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	system := module.ActorState
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State["当前详细地点"] = "公共坊市"
	}
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "世界拒绝", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "到了坊市", Narrative: "主角在坊市口站定，四下看了看。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

func worldRefusalStateArgs(event string) string {
	return `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前事件","value":"` + event + `"}],"choices":["继续行动","观察环境","询问同伴","检查状态","暂时等待"],"world":{"action":"none"}}`
}

// worldRefusalCycle builds one Game cycle exactly as the product does: native
// submit tool, protocol middleware, and the canonical journal runtime.
func worldRefusalCycle(
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
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.test.world-refusal", Version: 1}, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return agentexecution.Cycle{
		Definition: agent.Definition{
			Key: "test.game-world-refusal", Name: "game", Model: model, Tools: toolset, Permission: agentpermission.FullAccess(),
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.world-refusal", Version: 1},
			Execution:     agent.ExecutionPolicy{ModelMaxAttempts: 8},
			Middlewares: []agent.Middleware{agentinteractive.NewTurnProtocolMiddleware(agentinteractive.InteractiveStoryToolContext{
				TurnResultReady: conversation.InteractiveNarrativeReady, LoadNarrativeCandidate: conversation.LoadNarrativeCandidate,
				AcceptNarrativeCandidate: conversation.AcceptNarrativeCandidate,
			})},
		},
		Conversation: conversation,
		Request:      agentchat.ChatRequest{CommandID: "world-refusal-" + message, Message: message},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: cfg.ProjectID, StoryID: storyID, BranchID: "main",
			Workspace: cfg.Workspace, Mode: "interactive",
		},
	}
}

// TestNativeTurnRewritesAfterARefusedWorldAction is the native path end to end.
//
// Attempt 1 narrates a walk the rules refuse and submits it. The receipt asks
// for a rewrite, the submit tool discards the retained prose, and the middleware
// accepts the second narrative instead of keeping the refused one. The turn
// commits once, with the corrected prose, and the next turn continues from it.
func TestNativeTurnRewritesAfterARefusedWorldAction(t *testing.T) {
	ctx := t.Context()
	workspace, dataDir := t.TempDir(), t.TempDir()
	store, storyID := worldRefusalStory(t, workspace)
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "World refusal")
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

	refusedProse := "主角大步走出坊市，沿着土路回到了散修人家。"
	correctedProse := "主角刚要动身，守路的执事拦住他，说这条路今日不通。他只好退回坊市口。"
	refusedJourney := `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前详细地点","value":"越国坊市外围散修人家"}],"choices":["继续行动","观察环境","询问同伴","检查状态","暂时等待"],"world":{"action":"journey","route_id":"route_036:reverse","arrive":true}}`
	model := &worldRefusalModel{responses: []*agent.Message{
		agent.AssistantMessage(refusedProse, nil),
		draftSubmissionMessage("refused", refusedJourney),
		agent.AssistantMessage(correctedProse, nil),
		draftSubmissionMessage("corrected", worldRefusalStateArgs("主角被拦回坊市口")),
	}}

	var submissions atomic.Int32
	operation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: worldRefusalCycle(t, store, cfg, storyID, "回散修人家去", model, &submissions),
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("the rewritten turn must complete: %+v", outcome)
	}
	if outcome.Content != correctedProse {
		t.Fatalf("the committed prose must be the rewrite, not the refused success: %q", outcome.Content)
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TurnCount != 2 || snapshot.CurrentTurn.Narrative != correctedProse {
		t.Fatalf("one committed turn with the corrected prose: count=%d narrative=%q", snapshot.TurnCount, snapshot.CurrentTurn.Narrative)
	}
	// The refused walk settled nothing: the character still stands where they
	// were and no road record was published.
	world, err := store.WorldStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if world.Location != "公共坊市" {
		t.Fatalf("a refused walk must not move the character: %q", world.Location)
	}
	if len(world.ActiveJourneys) != 0 {
		t.Fatalf("a refused walk must publish no journey: %#v", world.ActiveJourneys)
	}
	if submissions.Load() != 2 {
		t.Fatalf("one refused submission and one corrected submission: %d", submissions.Load())
	}
	// The rewrite attempt was told why: the refusal feedback reached the model,
	// and the discarded success was not re-offered as a retained candidate.
	rewriteInput := model.input(t, 2)
	if !containsMessageContent(rewriteInput, "narrative_rewrite") {
		t.Fatal("the rewrite attempt was not told the world refused the narrated action")
	}
	if containsMessageContent(rewriteInput, "retained_narrative") {
		t.Fatal("the refused success was still offered to the model as the locked candidate")
	}

	// The next turn continues on the same branch from the corrected turn.
	model.setResponses([]*agent.Message{
		agent.AssistantMessage("主角在坊市口站了一会儿，转身回到人群里。", nil),
		draftSubmissionMessage("next", worldRefusalStateArgs("主角回到坊市人群")),
	})
	nextOperation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: worldRefusalCycle(t, store, cfg, storyID, "回到人群里", model, &submissions),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := nextOperation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("the next turn must continue after the rewrite: %+v", outcome)
	}
	after, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.TurnCount != 3 || after.CurrentTurn.Narrative != "主角在坊市口站了一会儿，转身回到人群里。" {
		t.Fatalf("the next turn appended once: count=%d narrative=%q", after.TurnCount, after.CurrentTurn.Narrative)
	}
}

// worldRefusalModel plays a fixed script and records every input, so the test
// can assert the rewrite request really reached the model with the refusal
// feedback and without the refused prose locked as the candidate.
type worldRefusalModel struct {
	mu        sync.Mutex
	responses []*agent.Message
	inputs    [][]*agent.Message
}

func (model *worldRefusalModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	model.inputs = append(model.inputs, cloned)
	if len(model.responses) == 0 {
		return nil, errWorldRefusalScriptExhausted
	}
	response := agent.CloneMessage(model.responses[0])
	model.responses = model.responses[1:]
	return response, nil
}

func (model *worldRefusalModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

func (model *worldRefusalModel) setResponses(responses []*agent.Message) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.responses = responses
}

func (model *worldRefusalModel) input(t *testing.T, index int) []*agent.Message {
	t.Helper()
	model.mu.Lock()
	defer model.mu.Unlock()
	if index >= len(model.inputs) {
		t.Fatalf("the model received %d requests, wanted request %d", len(model.inputs), index)
	}
	return model.inputs[index]
}

var errWorldRefusalScriptExhausted = &worldRefusalScriptError{}

type worldRefusalScriptError struct{}

func (*worldRefusalScriptError) Error() string { return "world refusal model script exhausted" }

// externalWorldRefusalTurn builds the external (provider) path over the same
// story and the same product tools, so the only difference under test is how
// the refusal is recovered: the loop must restore the submission schemas and
// ask for a rewrite instead of publishing the refused success.
func externalWorldRefusalTurn(t *testing.T, store *interactive.Store, storyID string, cfg *config.Config, request agentchat.ChatRequest, adapter external.Adapter) *ExternalTurn {
	t.Helper()
	conversation := NewConversation(store, t.TempDir(), cfg.Workspace, storyID, "main", request.Message, 800, cfg)
	assembly, err := agents.BuildExternalGameAssembly(t.Context(), cfg, nil, prompts.InteractiveStorySystemInstructionInput{}, agents.AgentHostCapabilities{Interactive: true}, agentinteractive.InteractiveStoryToolContext{
		Store: store, StoryID: storyID, BranchID: "main", PrepareTurn: conversation.PrepareInteractiveTurn, SubmitTurnResult: conversation.SubmitTurnResult,
		TurnResultReady: conversation.InteractiveNarrativeReady, LoadNarrativeCandidate: conversation.LoadNarrativeCandidate, AcceptNarrativeCandidate: conversation.AcceptNarrativeCandidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := StartExternalTurn(t.Context(), ExternalTurnConfig{Conversation: conversation, Request: request, Config: *cfg, Assembly: assembly, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

// TestExternalTurnRestoresSubmissionToolsAfterARefusedWorldAction is the
// external path end to end.
//
// Attempt 1 streams prose that walks a road the rules refuse, then submits the
// journey. The receipt asks for a rewrite and the draft prose is discarded. The
// next attempt must receive the rewrite instruction with the submission tool
// still available; it writes the real outcome and submits the complete turn.
// The refused success is never published, and the branch continues.
func TestExternalTurnRestoresSubmissionToolsAfterARefusedWorldAction(t *testing.T) {
	workspace := t.TempDir()
	store, storyID := worldRefusalStory(t, workspace)
	cfg := &config.Config{
		Workspace: workspace, ProjectID: "project-world-refusal-external", ProjectStoreDir: store.Root(), Language: "en-US",
		ActiveAgentRuntime: &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}},
	}

	refusedProse := "主角大步走出坊市，沿着土路回到了散修人家。"
	correctedProse := "主角刚要动身，守路的执事拦住他，说这条路今日不通。他只好退回坊市口。"
	refusedJourney := `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前详细地点","value":"越国坊市外围散修人家"}],"choices":["继续行动","观察环境","询问同伴","检查状态","暂时等待"],"world":{"action":"journey","route_id":"route_036:reverse","arrive":true}}`

	calls := 0
	turn := externalWorldRefusalTurn(t, store, storyID, cfg, agentchat.ChatRequest{CommandID: "external-world-refusal", Message: "回散修人家去"},
		gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
			// Context maintenance is a separate model purpose. It must not consume
			// one of the two scripted gameplay attempts or impersonate the rewrite.
			if input.Mode == external.OperationSummarize {
				return external.Result{Text: "The protagonist remains at the public market."}, nil
			}
			calls++
			switch calls {
			case 1:
				if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": refusedProse}}); err != nil {
					t.Fatal(err)
				}
				result := gameHostCall(t, ctx, host, "refused", "submit_interactive_turn", refusedJourney)
				if !strings.Contains(result.Text, "narrative_rewrite_required") {
					t.Fatalf("the refused submission must ask for a rewrite: %s", result.Text)
				}
				return external.Result{}, ctx.Err()
			case 2:
				if !strings.Contains(input.Text, "refused") {
					t.Fatalf("the rewrite attempt was not told why it must rewrite: %s", input.Text)
				}
				// The turn still has modules to submit, so the provider must keep
				// the submission tool. Dropping it here is the N10 defect.
				if !hasExternalTool(input.Tools, "submit_interactive_turn") {
					t.Fatal("the rewrite attempt lost the submission tool")
				}
				if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": correctedProse}}); err != nil {
					t.Fatal(err)
				}
				gameHostCall(t, ctx, host, "corrected", "submit_interactive_turn", worldRefusalStateArgs("主角被拦回坊市口"))
				return external.Result{}, ctx.Err()
			default:
				t.Fatalf("unexpected external attempt %d", calls)
				return external.Result{}, nil
			}
		}))
	if outcome := turn.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != correctedProse {
		t.Fatalf("the external rewrite must complete with the corrected prose: %+v", outcome)
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TurnCount != 2 || snapshot.CurrentTurn.Narrative != correctedProse {
		t.Fatalf("one committed external turn with the corrected prose: count=%d narrative=%q", snapshot.TurnCount, snapshot.CurrentTurn.Narrative)
	}
	world, err := store.WorldStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if world.Location != "公共坊市" || len(world.ActiveJourneys) != 0 {
		t.Fatalf("a refused external walk must settle nothing: location=%q journeys=%#v", world.Location, world.ActiveJourneys)
	}
	if calls != 2 {
		t.Fatalf("one refused attempt and one rewrite attempt: %d", calls)
	}

	// The next external turn continues from the corrected one.
	next := externalWorldRefusalTurn(t, store, storyID, cfg, agentchat.ChatRequest{CommandID: "external-world-refusal-next", Message: "回到人群里"},
		gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
			if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "主角在坊市口站了一会儿，转身回到人群里。"}}); err != nil {
				t.Fatal(err)
			}
			gameHostCall(t, ctx, host, "next", "submit_interactive_turn", worldRefusalStateArgs("主角回到坊市人群"))
			return external.Result{}, ctx.Err()
		}))
	if outcome := next.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "主角在坊市口站了一会儿，转身回到人群里。" {
		t.Fatalf("the next external turn must continue after the rewrite: %+v", outcome)
	}
	after, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.TurnCount != 3 {
		t.Fatalf("the next external turn appended once: %d", after.TurnCount)
	}
}

func hasExternalTool(tools []external.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
