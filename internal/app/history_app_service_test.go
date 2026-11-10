package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ta/config"
	"ta/internal/history"
	projectdomain "ta/internal/project"
)

type historyModelRequest struct {
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func historyServiceFixture(t *testing.T, answer func(historyModelRequest) (string, int)) (*HistoryAppService, string, history.State) {
	t.Helper()
	return historyServiceFixtureWithFinishReason(t, answer, "stop")
}

func historyServiceFixtureWithFinishReason(t *testing.T, answer func(historyModelRequest) (string, int), finishReason string) (*HistoryAppService, string, history.State) {
	t.Helper()
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request historyModelRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
			http.Error(w, "invalid", 400)
			return
		}
		content, status := answer(request)
		if status != 0 && status != 200 {
			http.Error(w, content, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "history-fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finishReason, "message": map[string]any{"role": "assistant", "content": content, "reasoning_content": "not a saved reply"}}}})
	}))
	t.Cleanup(server.Close)
	settings := config.Settings{
		ModelEndpoints: []config.ModelEndpointSettings{{ID: "history-fixture", Provider: "openai-compatible", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "test-only"}},
		ModelProfiles:  []config.ModelProfileSettings{{ID: "history-fixture", EndpointID: "history-fixture", Model: "history-model"}},
		AgentModels:    config.AgentModelSettings{InteractiveStory: config.AgentModelOverride{ProfileID: "history-fixture", ThinkingLevel: "off"}},
	}
	if err := config.WriteSettingsFile(config.UserConfigPath(root), settings); err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: &config.Config{NovaDir: root, Workspace: "untouched-adventure", OpenAIModel: "wrong-foreground-model"}, projectRegistry: projectdomain.NewRegistry(root)}
	service := application.HistoryService()
	t.Cleanup(service.close)
	bootstrap, err := service.Bootstrap(context.Background(), "en-US")
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Create(context.Background(), bootstrap.ProjectID, history.CreateRequest{Name: "Test reign", CommandID: "create-1"})
	if err != nil {
		t.Fatal(err)
	}
	return service, bootstrap.ProjectID, state
}

func TestHistoryBootstrapKeepsProjectAndForegroundIndependent(t *testing.T) {
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) { return `{}`, 200 })
	again, err := service.Bootstrap(context.Background(), "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if again.ProjectID != projectID || len(again.Campaigns) != 1 || again.Campaigns[0].ID != state.ID {
		t.Fatalf("bootstrap changed identity: %+v", again)
	}
	if service.app.cfg.Workspace != "untouched-adventure" {
		t.Fatal("history changed foreground adventure")
	}
	record, err := service.app.projectRegistry.Get(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Location.Kind != projectdomain.LocationManaged || record.Location.Path != "games/history" {
		t.Fatalf("nonportable history project: %+v", record.Location)
	}
	otherPath := filepath.Join(service.app.cfg.DataDir(), "other")
	if err := os.Mkdir(otherPath, 0700); err != nil {
		t.Fatal(err)
	}
	other, err := service.app.projectRegistry.Add(otherPath, projectdomain.TypeGeneral, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Read(context.Background(), other.ID, state.ID); !errors.Is(err, ErrHistoryProject) {
		t.Fatalf("history accepted unrelated project: %v", err)
	}
}

func TestHistoryChatRetryReadsReceiptBeforeModelAndRejectsChangedInput(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls.Add(1)
		if req.Model != "history-model" || len(req.Messages) != 2 {
			t.Errorf("model escaped dedicated settings: %+v", req)
		}
		return `{"reply":"臣会记住这个承诺。"}`, 200
	})
	req := HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "chat-1", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: "请记住我们对赈灾的承诺。"}
	result, err := service.Chat(context.Background(), projectID, state.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.Chat(context.Background(), projectID, state.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !duplicate.Duplicate || len(duplicate.State.Chats) != 1 || duplicate.State.Revision != result.State.Revision {
		t.Fatalf("retry repeated work: calls=%d duplicate=%+v", calls.Load(), duplicate)
	}
	req.Message = "another request"
	if _, err := service.Chat(context.Background(), projectID, state.ID, req); !errors.Is(err, history.ErrCommandConflict) {
		t.Fatalf("reused command accepted: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("conflict called model")
	}
}

func TestHistoryCouncilNeverInjectsPrivateOrOtherAudienceMemory(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls.Add(1)
		if strings.Contains(req.Messages[1].Content, "private-canary") || strings.Contains(req.Messages[1].Content, "ab-council-canary") {
			t.Error("council leaked a private audience")
		}
		return `{"reply":"臣仅据已知灾情陈奏。"}`, 200
	})
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range []history.ChatTurn{
		{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "private-canary", Replies: []history.Reply{{MinisterID: "bi-ziyan", Text: "secret"}}},
		{Mode: "council", MinisterIDs: []string{"bi-ziyan", "fan-jingwen"}, UserMessage: "ab-council-canary", Replies: []history.Reply{{MinisterID: "bi-ziyan", Text: "secret"}, {MinisterID: "fan-jingwen", Text: "understood"}}},
	} {
		id := "seed-" + chat.Mode
		receipt, err := store.Apply(context.Background(), state.ID, history.Command{ID: id, ExpectedRevision: state.Revision, Kind: "chat", Chat: &chat})
		if err != nil {
			t.Fatal(err)
		}
		state = receipt.State
	}
	receipt, err := service.Chat(context.Background(), projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "new-council", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan", "sun-chengzong"}, Message: "讨论本季灾情"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(receipt.State.Chats) != 3 || len(receipt.State.Chats[2].Replies) != 2 {
		t.Fatal("council did not commit one shared turn")
	}
}

func TestHistoryQuarterModelFailureDoesNotHalfCommit(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		if calls.Add(1) == 1 {
			return `{"adjustments":[],"events":[]}`, 200
		}
		return `not a usable report`, 200
	})
	_, err := service.Advance(context.Background(), projectID, state.ID, HistoryCommandRequest{CommandID: "quarter-1", ExpectedRevision: state.Revision})
	if !errors.Is(err, ErrHistoryModelOutput) {
		t.Fatalf("expected report refusal: %v", err)
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != state.Revision || after.Turn != state.Turn || after.Treasury != state.Treasury || len(after.Reports) != 0 {
		t.Fatal("failed report committed a half-quarter")
	}
}

func TestHistoryQuarterReportSeesSettledFactsAndCommitsOnce(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		if calls.Add(1) == 1 {
			return `{"adjustments":[],"events":[]}`, 200
		}
		var payload struct {
			Report history.QuarterReport `json:"report"`
			Next   struct {
				Treasury int64 `json:"treasury"`
				Turn     int   `json:"turn"`
			} `json:"next_state"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &payload); err != nil {
			t.Error(err)
		}
		if payload.Next.Turn != 1 || payload.Next.Treasury != payload.Report.ClosingTreasury || payload.Report.Income <= 0 {
			t.Errorf("narrator did not receive settled report: %+v", payload)
		}
		return `{"narrative":"本季收支已核定。"}`, 200
	})
	req := HistoryCommandRequest{CommandID: "quarter-1", ExpectedRevision: state.Revision}
	first, err := service.Advance(context.Background(), projectID, state.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Advance(context.Background(), projectID, state.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.State.Turn != 1 || len(first.State.Reports) != 1 || !second.Duplicate || calls.Load() != 2 {
		t.Fatalf("quarter was repeated: calls=%d %+v", calls.Load(), second)
	}
}

func TestHistoryCancelWaitsForModelThenRejectsCommit(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return `{"reply":"a late reply"}`, 200
	})
	result := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- errors.New("panic in test operation")
			}
		}()
		_, err := service.Chat(context.Background(), projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "cancel-me", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: "wait"})
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("model did not start")
	}
	if !service.Cancel(projectID, state.ID, "cancel-me") {
		t.Error("active command not cancelled")
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("late result was not cancelled: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled model did not finish")
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != state.Revision || len(after.Chats) != 0 {
		t.Fatal("cancelled reply was committed")
	}
}

func TestHistoryDecodeRejectsAmbiguousMutationsAndRepairsFence(t *testing.T) {
	for _, input := range []string{`{"narrative":"one","narrative":"two"}`, `{"narrative":"one"}{}`, `{"narrative":"one","treasury":90000000}`, `null`, `{"narrative":"one"} trailing`} {
		var value struct {
			Narrative string `json:"narrative"`
		}
		if err := decodeHistoryModel(input, &value); !errors.Is(err, ErrHistoryModelOutput) {
			t.Fatalf("ambiguous output accepted: %q %v", input, err)
		}
	}
	var value struct {
		Narrative string `json:"narrative"`
	}
	if err := decodeHistoryModel("```json\n{\"narrative\":\"safe report\"}\n```", &value); err != nil || value.Narrative != "safe report" {
		t.Fatalf("fence was not repaired: %+v %v", value, err)
	}
}

func TestHistoryStaleModelResultCannotOverwriteConcurrentSave(t *testing.T) {
	var service *HistoryAppService
	var projectID string
	var state history.State
	service, projectID, state = historyServiceFixture(t, func(historyModelRequest) (string, int) {
		store, _, err := service.binding(projectID)
		if err != nil {
			t.Error(err)
			return `{}`, 200
		}
		_, err = store.Apply(context.Background(), state.ID, history.Command{ID: "concurrent-save", ExpectedRevision: state.Revision, Kind: "checkpoint", Label: "Concurrent"})
		if err != nil {
			t.Error(err)
		}
		return `{"reply":"prepared against the old revision"}`, 200
	})
	_, err := service.Chat(context.Background(), projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "stale-chat", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: "How are matters?"})
	if !errors.Is(err, history.ErrConflict) {
		t.Fatalf("stale model result accepted: %v", err)
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Chats) != 0 || len(after.Checkpoints) != 1 || after.Revision != state.Revision+1 {
		t.Fatalf("stale result overwrote concurrent state: %+v", after)
	}
}

func TestHistoryPreparePreservesWholePlanConditionsAndUnsupportedIntent(t *testing.T) {
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		return `{"text":"AI must not replace the original","instructions":[{"id":"relief","kind":"relief","target":"shaanxi","minister_id":"bi-ziyan","budget":100000,"grain":20000,"quarters":2},{"id":"works","kind":"construction","target":"shaanxi","minister_id":"fan-jingwen","budget":50000,"quarters":2,"not_before_turn":1,"depends_on":["relief"]},{"id":"foreign","kind":"unsupported","reason":"外交机制尚未实现"}]}`, 200
	})
	text := "赈济陕西，完成后兴修水利，另与外国谈判。"
	prepared, err := service.PrepareEdict(context.Background(), projectID, state.ID, HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "prepare-1", ExpectedRevision: state.Revision}, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.Text != text || len(prepared.Plan.Instructions) != 3 || len(prepared.Plan.Instructions[1].DependsOn) != 1 || prepared.Plan.Instructions[1].NotBeforeTurn != 1 || prepared.Outcomes[2].Status != "unsupported" {
		t.Fatalf("plan lost intent: %+v", prepared)
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != state.Revision || len(after.Projects) != 0 {
		t.Fatal("preparation executed an unconfirmed edict")
	}
}

func TestHistoryOutputBudgetDoesNotRepeatFormatRepairOrCommit(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixtureWithFinishReason(t, func(historyModelRequest) (string, int) { calls.Add(1); return "", 200 }, "length")
	_, err := service.Advance(context.Background(), projectID, state.ID, HistoryCommandRequest{CommandID: "budget", ExpectedRevision: state.Revision})
	if !errors.Is(err, ErrHistoryModelBudget) || calls.Load() != 1 {
		t.Fatalf("Budget exhaustion retried or obscured: calls=%d error=%v", calls.Load(), err)
	}
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(context.Background(), state.ID)
	if err != nil || read.Revision != state.Revision || read.Turn != state.Turn {
		t.Fatal("Truncated model output changed the campaign")
	}
}
