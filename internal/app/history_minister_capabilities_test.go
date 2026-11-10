package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoryMinisterGetsPublicRosterAndCurrentActionsWithoutPrivateProfiles(t *testing.T) {
	const message = "请按当前事实给一个能直接拟旨的陕西救济方案，预算最多100000两；现在只议事，不颁行。"
	service, projectID, state := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		var input struct {
			Message string                       `json:"message"`
			Roster  []map[string]json.RawMessage `json:"available_ministers"`
			Actions []string                     `json:"available_actions"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Error(err)
		}
		if input.Message != message || len(input.Roster) != 4 {
			t.Fatalf("original request or public court lost: %+v", input)
		}
		for _, minister := range input.Roster {
			if len(minister) != 3 || minister["id"] == nil || minister["name"] == nil || minister["office"] == nil {
				t.Errorf("roster leaked private minister data: keys=%v", minister)
			}
		}
		if strings.Contains(req.Messages[1].Content, "忠谨沉默，皇帝亲信") || strings.Contains(req.Messages[1].Content, "稳重持国，车营大家") {
			t.Error("other minister private profile leaked through public roster")
		}
		hasRelief, hasConstruction := false, false
		for _, kind := range input.Actions {
			hasRelief = hasRelief || kind == "relief"
			hasConstruction = hasConstruction || kind == "construction"
			if kind == "procurement" || kind == "appoint" {
				t.Error("advertised unimplemented action")
			}
		}
		if !hasRelief || !hasConstruction {
			t.Fatal("minister did not receive actual executable scope")
		}
		return `{"reply":"臣建议由毕自严主办陕西赈济，总拨银80000两、粮30000石，分2季办理。此刻仅供陛下审议。"}`, 200
	})
	receipt, err := service.Chat(context.Background(), projectID, state.ID, HistoryChatRequest{HistoryCommandRequest: HistoryCommandRequest{CommandID: "actionable-advice", ExpectedRevision: state.Revision}, MinisterIDs: []string{"bi-ziyan"}, Message: message})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State.Chats[0].UserMessage != message || receipt.State.Treasury != state.Treasury || receipt.State.Grain != state.Grain || len(receipt.State.Projects) != 0 {
		t.Fatal("context enrichment rewrote player intent or executed advice")
	}
}
