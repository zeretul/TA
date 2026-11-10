package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"ta/internal/history"
)

func TestHistoryWorldDirectDecreeSemanticRepairAndIdempotency(t *testing.T) {
	var decreeCalls, simulationCalls, reportCalls atomic.Int32
	var orderID string
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		prompt := req.Messages[0].Content
		switch {
		case strings.Contains(prompt, "全局季度推演者"):
			call := simulationCalls.Add(1)
			amount := int64(50)
			if call == 1 {
				amount = 101
			}
			proposal := history.WorldProposal{Transfers: []history.Transfer{{From: "treasury", Resource: "silver", Amount: amount, OrderID: orderID, Reason: "Authorized relief"}}, Results: []history.OrderResult{{OrderID: orderID, Status: "active", Progress: 10, ConditionMet: true, Reason: "Relief continues"}}}
			b, _ := json.Marshal(proposal)
			return string(b), 200
		case strings.Contains(prompt, "季报执笔者"):
			reportCalls.Add(1)
			if strings.Contains(req.Messages[1].Content, "disposition-wei-zhongxian") {
				t.Error("report author received undisclosed personal truth")
			}
			return `{"narrative":"Relief cost 50 taels; the standing policy continues."}`, 200
		default:
			decreeCalls.Add(1)
			return `{"orders":[{"action":"create","kind":"policy","text":"Continue relief within a total 100 tael cap","minister_id":"bi-ziyan","targets":["shaanxi"],"limits":{"silver":100}}]}`, 200
		}
	})
	ctx := context.Background()
	state, err := service.Create(ctx, projectID, history.CreateRequest{Name: "Full world", CommandID: "create-full", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	request := HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "direct", ExpectedRevision: state.Revision}, Text: "Provide continuing relief, capped at 100 taels"}
	issued, err := service.DirectEdict(ctx, projectID, state.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	orderID = issued.State.World.Orders[0].ID
	repeated, err := service.DirectEdict(ctx, projectID, state.ID, request)
	if err != nil || !repeated.Duplicate || decreeCalls.Load() != 1 {
		t.Fatal("direct retry repeated model or decree")
	}
	advance := HistoryCommandRequest{CommandID: "quarter", ExpectedRevision: issued.State.Revision}
	settled, err := service.Advance(ctx, projectID, state.ID, advance)
	if err != nil {
		t.Fatal(err)
	}
	if simulationCalls.Load() != 2 || reportCalls.Load() != 1 || settled.State.Turn != 1 || settled.State.World.Accounts[0].Balance != 999950 {
		t.Fatal("semantic correction did not settle exactly once")
	}
	data, _ := json.Marshal(settled)
	if strings.Contains(string(data), "disposition-wei-zhongxian") {
		t.Fatal("receipt leaked hidden truth")
	}
	_, err = service.Advance(ctx, projectID, state.ID, advance)
	if err != nil || simulationCalls.Load() != 2 || reportCalls.Load() != 1 {
		t.Fatal("quarter retry repeated expensive work")
	}
}

func TestHistoryWorldReportFailureAndUnknownReportDoNotMutate(t *testing.T) {
	var calls atomic.Int32
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls.Add(1)
		if strings.Contains(req.Messages[0].Content, "全局季度推演者") {
			return `{"transfers":[],"changes":[],"new_entities":[],"results":[],"events":[]}`, 200
		}
		return `{"wrong":"invalid report schema"}`, 200
	})
	ctx := context.Background()
	state, err := service.Create(ctx, projectID, history.CreateRequest{Name: "Report failure", CommandID: "create-failure", ScenarioID: "song-1127"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Advance(ctx, projectID, state.ID, HistoryCommandRequest{CommandID: "failed-quarter", ExpectedRevision: state.Revision})
	if err == nil {
		t.Fatal("invalid report accepted")
	}
	current, err := service.Read(ctx, projectID, state.ID)
	if err != nil || current.Revision != state.Revision || current.Turn != 0 || len(current.Reports) != 0 {
		t.Fatal("failed report partially committed quarter")
	}
	before := calls.Load()
	_, err = service.Chat(ctx, projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "missing-report", ExpectedRevision: state.Revision}, MinisterIDs: []string{"li_gang"}, Message: "Discuss this", SourceReportID: "invented"})
	if err == nil || calls.Load() != before {
		t.Fatal("missing report reached the model")
	}
}

func TestHistoryWorldDiscussionDraftRetainsCanonicalProposal(t *testing.T) {
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		var input struct {
			Source *history.ChatTurn `json:"source_discussion"`
			State  struct {
				World history.World `json:"world"`
			} `json:"current_state"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Error(err)
		}
		if input.Source == nil || input.Source.ID != "selected-world-discussion" || !strings.Contains(input.Source.Replies[0].Text, "40") || len(input.State.World.Accounts) == 0 {
			t.Error("canonical proposal or current world missing")
		}
		return `{"orders":[{"action":"create","kind":"project","text":"Inspect the granary with a total ceiling of 40 taels","minister_id":"bi-ziyan","targets":["shaanxi"],"limits":{"silver":40}}]}`, 200
	})
	ctx := context.Background()
	s, err := service.Create(ctx, projectID, history.CreateRequest{Name: "Source linkage", CommandID: "source-linkage", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.Apply(ctx, s.ID, history.Command{ID: "selected-world-discussion", ExpectedRevision: s.Revision, Kind: "chat", Chat: &history.ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "Give me a limited inspection proposal", Replies: []history.Reply{{MinisterID: "bi-ziyan", Text: "Inspect Shaanxi granaries within 40 taels."}}}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareEdict(ctx, projectID, s.ID, HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "draft-world", ExpectedRevision: r.State.Revision}, SourceChatID: "selected-world-discussion", Text: "Draft the proposal for review"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Read(ctx, s.ID)
	if err != nil || len(before.World.Orders) != 0 {
		t.Fatal("preview enacted the proposal")
	}
	issued, err := service.CommitEdict(ctx, projectID, s.ID, HistoryEdictRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "issue-world", ExpectedRevision: prepared.ExpectedRevision}, Plan: prepared.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if issued.State.Edicts[0].Plan.SourceChatID != "selected-world-discussion" || issued.State.World.Orders[0].Limits["silver"] != 40 || issued.State.World.Accounts[0].Balance != 1000000 {
		t.Fatal("decree lost source, authorization or unspent balance")
	}
}

func TestHistoryQuarterRepairsDifferentErrorsUntilTheWholeCandidateIsValid(t *testing.T) {
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		if strings.Contains(req.Messages[0].Content, "季报执笔者") {
			return `{"narrative":"The corrected quarter has been recorded."}`, 200
		}
		calls++
		switch calls {
		case 1:
			return `{"transfers":[{"from":"missing-a","to":"treasury","resource":"silver","amount":1,"reason":"Test"}]}`, 200
		case 2:
			return `{"events":[{"title":"Test","text":"A report","entity_ids":["missing-b"],"audience":["public"],"status":"open"}]}`, 200
		case 3:
			return `{"transfers":[{"from":"treasury","to":"","resource":"silver","amount":-1,"reason":"Test"}]}`, 200
		default:
			return `{"transfers":[],"changes":[],"new_entities":[],"results":[],"events":[]}`, 200
		}
	})
	s, err := service.Create(context.Background(), projectID, history.CreateRequest{Name: "Repair flow", CommandID: "create-repair-flow", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := service.Advance(context.Background(), projectID, s.ID, HistoryCommandRequest{CommandID: "repair-quarter", ExpectedRevision: s.Revision})
	if err != nil || calls != 4 || r.State.Turn != 1 || r.State.World.Accounts[0].Balance != s.World.Accounts[0].Balance {
		t.Fatalf("Correction chain failed: calls=%d error=%v", calls, err)
	}
}

func TestHistoryRepeatedInvalidQuarterStopsWithoutCommitting(t *testing.T) {
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(historyModelRequest) (string, int) {
		calls++
		return `{"transfers":[{"from":"missing-a","to":"treasury","resource":"silver","amount":1,"reason":"Test"}]}`, 200
	})
	s, err := service.Create(context.Background(), projectID, history.CreateRequest{Name: "Repeated invalid", CommandID: "create-repeat", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Advance(context.Background(), projectID, s.ID, HistoryCommandRequest{CommandID: "bad-quarter", ExpectedRevision: s.Revision})
	if !errors.Is(err, ErrHistoryModelOutput) || calls != 2 {
		t.Fatalf("No-progress loop did not stop: %d %v", calls, err)
	}
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	read, err := store.Read(context.Background(), s.ID)
	if err != nil || read.Revision != s.Revision || read.Turn != 0 {
		t.Fatal("Invalid quarter was partly committed")
	}
}

func TestHistoryCorrectsDecreeEntityReferencesBeforeCommitting(t *testing.T) {
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls++
		if calls == 1 {
			return `{"orders":[{"action":"create","kind":"one_shot","text":"Inspect the treasury records without spending","minister_id":"bi-ziyan","targets":["treasury"]}]}`, 200
		}
		if !strings.Contains(req.Messages[1].Content, "validation_outcomes") {
			t.Error("Reference repair received no specific validation feedback")
		}
		return `{"orders":[{"action":"create","kind":"one_shot","text":"Inspect the treasury records without spending","minister_id":"bi-ziyan","targets":["ming"]}]}`, 200
	})
	s, err := service.Create(context.Background(), projectID, history.CreateRequest{Name: "Decree correction", CommandID: "create-decree-correction", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	req := HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "correct-reference", ExpectedRevision: s.Revision}, Text: "Inspect the treasury records without spending"}
	r, err := service.DirectEdict(context.Background(), projectID, s.ID, req)
	if err != nil || calls != 2 || len(r.State.World.Orders) != 1 || r.State.World.Orders[0].Targets[0] != "ming" || r.State.World.Accounts[0].Balance != s.World.Accounts[0].Balance {
		t.Fatalf("Reference correction failed: calls=%d error=%v", calls, err)
	}
	repeat, err := service.DirectEdict(context.Background(), projectID, s.ID, req)
	if err != nil || !repeat.Duplicate || calls != 2 {
		t.Fatalf("Retry repeated model work: %v", err)
	}
}
