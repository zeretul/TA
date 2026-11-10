package app

import (
	"context"
	"encoding/json"
	"ta/internal/history"
	"testing"
)

func TestHistorySavedAdviceDraftKeepsCanonicalSourceWithoutExecution(t *testing.T) {
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		var input struct {
			Advice *history.SavedAdvice `json:"selected_advice"`
			Source *history.ChatTurn    `json:"source_discussion"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Fatal(err)
		}
		if input.Advice == nil || input.Advice.Text != "Spend at most 80 taels on relief" || input.Source == nil || input.Source.ID != "saved-discussion" {
			t.Fatalf("Source lost: %+v", input)
		}
		return `{"orders":[{"action":"create","kind":"one_shot","text":"Relief up to 80 taels","minister_id":"bi-ziyan","targets":["shaanxi"],"limits":{"silver":80}}]}`, 200
	})
	ctx := context.Background()
	s, err := service.Create(ctx, projectID, history.CreateRequest{Name: "Advice", CommandID: "advice-world", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	store, _, _ := service.binding(projectID)
	r, err := store.Apply(ctx, s.ID, history.Command{ID: "saved-discussion", Kind: "chat", ExpectedRevision: s.Revision, Chat: &history.ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "Propose relief", Replies: []history.Reply{{MinisterID: "bi-ziyan", Text: "Spend at most 80 taels on relief"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	saved, err := service.StateCommand(ctx, projectID, s.ID, "advice", HistoryStateRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "bookmark", ExpectedRevision: s.Revision}, Advice: &history.AdviceChange{Action: "save", ChatID: "saved-discussion", MinisterID: "bi-ziyan"}})
	if err != nil {
		t.Fatal(err)
	}
	s = saved.State
	prepared, err := service.PrepareEdict(ctx, projectID, s.ID, HistoryPrepareRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "draft-bookmark", ExpectedRevision: s.Revision}, Text: "Draft the saved suggestion", SourceAdviceID: s.Advice[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.SourceChatID != "saved-discussion" || prepared.Plan.SourceAdviceID != s.Advice[0].ID {
		t.Fatal("Provenance changed")
	}
	cold, _ := store.Read(ctx, s.ID)
	if cold.Revision != s.Revision || len(cold.World.Orders) != 0 {
		t.Fatal("Preview executed")
	}
	_, err = service.CommitEdict(ctx, projectID, s.ID, HistoryEdictRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "issue-bookmark", ExpectedRevision: s.Revision}, Plan: prepared.Plan})
	if err != nil {
		t.Fatal(err)
	}
}
