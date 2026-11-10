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

func TestHistoryDiscussionDraftUsesSelectedProposalAndKeepsProvenance(t *testing.T) {
	var drafts atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		if strings.Contains(req.Messages[0].Content, "季度推演者") {
			return `{"adjustments":[],"events":[]}`, 200
		}
		if strings.Contains(req.Messages[0].Content, "季末奏报执笔者") {
			return `{"narrative":"依照已颁诏令办理本季赈济。"}`, 200
		}
		drafts.Add(1)
		var input struct {
			State struct {
				Treasury int64 `json:"treasury"`
			} `json:"current_state"`
			Text   string            `json:"edict"`
			Source *history.ChatTurn `json:"source_discussion"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Error(err)
		}
		if input.Source == nil || input.Source.ID != "selected-discussion" || len(input.Source.Replies) != 1 || !strings.Contains(input.Source.Replies[0].Text, "100000") || input.Source.Turn != 0 {
			t.Errorf("selected minister proposal was lost: %+v", input.Source)
		}
		if input.State.Treasury != 1056250 {
			t.Errorf("draft did not receive current rather than old balance: %d", input.State.Treasury)
		}
		if strings.Contains(req.Messages[1].Content, "unrelated-private-canary") {
			t.Error("draft pulled unrelated private conversation")
		}
		return `{"source_chat_id":"model-invented-source","instructions":[{"id":"relief","kind":"relief","target":"shaanxi","minister_id":"bi-ziyan","budget":100000,"grain":20000,"quarters":2}]}`, 200
	})
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, who, message, reply string }{
		{"unrelated-chat", "wang-chengen", "unrelated-private-canary", "这是私事，不涉及赈灾。"},
		{"selected-discussion", "bi-ziyan", "旧时国库1000000两，陕西赈灾请给个方案。", "臣建议由毕自严负责陕西赈济，拨国库100000两、粮20000石，分2季办理。"},
	} {
		receipt, err := store.Apply(context.Background(), state.ID, history.Command{ID: item.id, ExpectedRevision: state.Revision, Kind: "chat", Chat: &history.ChatTurn{Mode: "private", MinisterIDs: []string{item.who}, UserMessage: item.message, Replies: []history.Reply{{MinisterID: item.who, Text: item.reply}}}})
		if err != nil {
			t.Fatal(err)
		}
		state = receipt.State
	}
	advanced, err := store.Apply(context.Background(), state.ID, history.Command{ID: "pass-quarter-before-draft", ExpectedRevision: state.Revision, Kind: "advance", Quarter: &history.QuarterPlan{}})
	if err != nil {
		t.Fatal(err)
	}
	state = advanced.State
	prepared, err := service.PrepareEdict(context.Background(), projectID, state.ID, HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "draft-selected", ExpectedRevision: state.Revision}, Text: "采纳此次大臣提出的赈济方案，拟成诏令供我审阅。", SourceChatID: "selected-discussion"})
	if err != nil {
		t.Fatal(err)
	}
	if drafts.Load() != 1 || prepared.Plan.SourceChatID != "selected-discussion" || prepared.Plan.Instructions[0].Budget != 100000 {
		t.Fatalf("wrong prepared provenance or proposal: %+v", prepared)
	}
	untouched, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Revision != state.Revision || len(untouched.Edicts) != 0 {
		t.Fatal("draft prematurely issued decree")
	}
	issued, err := service.CommitEdict(context.Background(), projectID, state.ID, HistoryEdictRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "issue-selected", ExpectedRevision: state.Revision}, Plan: prepared.Plan})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := service.Advance(context.Background(), projectID, state.ID, HistoryCommandRequest{CommandID: "settle-selected", ExpectedRevision: issued.State.Revision})
	if err != nil {
		t.Fatal(err)
	}
	cold, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	report := cold.Reports[len(cold.Reports)-1]
	if settled.State.Revision != cold.Revision || cold.Edicts[0].Plan.SourceChatID != "selected-discussion" || cold.Projects[0].EdictID != cold.Edicts[0].ID || report.Outcomes[0].ProjectID != cold.Projects[0].ID {
		t.Fatal("formal decree/project/report lost discussion provenance")
	}
}

func TestHistoryDraftRefusesUnknownAndDiscardedSourceBeforeModel(t *testing.T) {
	var calls atomic.Int32
	service, projectID, state := historyServiceFixture(t, func(historyModelRequest) (string, int) { calls.Add(1); return `{}`, 200 })
	store, _, err := service.binding(projectID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Apply(context.Background(), state.ID, history.Command{ID: "before-discussion", ExpectedRevision: state.Revision, Kind: "checkpoint", Label: "Before discussion"})
	if err != nil {
		t.Fatal(err)
	}
	state = saved.State
	talked, err := store.Apply(context.Background(), state.ID, history.Command{ID: "discarded-discussion", ExpectedRevision: state.Revision, Kind: "chat", Chat: &history.ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "拟个赈济方案", Replies: []history.Reply{{MinisterID: "bi-ziyan", Text: "拨100000两与20000石，分2季办理。"}}}})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Apply(context.Background(), state.ID, history.Command{ID: "rewind-discussion", ExpectedRevision: talked.State.Revision, Kind: "restore", CheckpointID: saved.State.Checkpoints[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	state = restored.State
	for _, source := range []string{"unknown-or-other-campaign", "discarded-discussion"} {
		_, err := service.PrepareEdict(context.Background(), projectID, state.ID, HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "draft-" + source, ExpectedRevision: state.Revision}, Text: "按讨论拟诏", SourceChatID: source})
		if !errors.Is(err, ErrHistoryInput) {
			t.Fatalf("invalid source admitted: %s %v", source, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unknown or discarded source reached model")
	}
	after, err := service.Read(context.Background(), projectID, state.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != state.Revision || len(after.Edicts) != 0 {
		t.Fatal("invalid source mutated the campaign")
	}
}
