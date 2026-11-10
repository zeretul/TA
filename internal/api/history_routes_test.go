package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	appsvc "ta/internal/app"
	"ta/internal/history"
)

func TestHistoryRoutesBootstrapCreateCheckpointRestoreWithoutAdventure(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, ":0")
	bootstrapResponse := performJSONRequest(t, server, "POST", "/api/history/bootstrap", map[string]any{})
	if bootstrapResponse.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", bootstrapResponse.Code, bootstrapResponse.Body.String())
	}
	var bootstrap appsvc.HistoryBootstrap
	decodeResponse(t, bootstrapResponse.Body.Bytes(), &bootstrap)
	prefix := "/api/projects/" + bootstrap.ProjectID + "/history/campaigns"
	created := performJSONRequest(t, server, "POST", prefix, map[string]any{"name": "Isolated reign", "command_id": "create-history", "scenario_id": "ming-1627-slice"})
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var state history.State
	decodeResponse(t, created.Body.Bytes(), &state)
	prefix += "/" + state.ID
	checkpoint := performJSONRequest(t, server, "POST", prefix+"/checkpoints", map[string]any{"command_id": "save-start", "expected_revision": state.Revision, "label": "Opening"})
	if checkpoint.Code != http.StatusOK {
		t.Fatalf("checkpoint: %d %s", checkpoint.Code, checkpoint.Body.String())
	}
	var saved history.Receipt
	decodeResponse(t, checkpoint.Body.Bytes(), &saved)
	if len(saved.State.Checkpoints) != 1 {
		t.Fatal("checkpoint missing")
	}
	edict := performJSONRequest(t, server, "POST", prefix+"/edict", map[string]any{"command_id": "relief", "expected_revision": saved.State.Revision, "plan": history.EdictPlan{Text: "Relief", Instructions: []history.Instruction{{ID: "one", Kind: "relief", Target: "shaanxi", MinisterID: "bi-ziyan", Budget: 100000, Grain: 20000, Quarters: 2}}}})
	if edict.Code != http.StatusOK {
		t.Fatalf("edict: %d %s", edict.Code, edict.Body.String())
	}
	var ordered history.Receipt
	decodeResponse(t, edict.Body.Bytes(), &ordered)
	if len(ordered.State.Projects) != 1 {
		t.Fatal("project was not committed")
	}
	restored := performJSONRequest(t, server, "POST", prefix+"/restore", map[string]any{"command_id": "restore-opening", "expected_revision": ordered.State.Revision, "checkpoint_id": saved.State.Checkpoints[0].ID})
	if restored.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", restored.Code, restored.Body.String())
	}
	var result history.Receipt
	decodeResponse(t, restored.Body.Bytes(), &result)
	if len(result.State.Projects) != 0 || result.State.Revision <= ordered.State.Revision {
		t.Fatal("restore did not reset state with a monotone journal revision")
	}
	read := performJSONRequest(t, server, "GET", prefix, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("read: %d %s", read.Code, read.Body.String())
	}
	var reopened history.State
	decodeResponse(t, read.Body.Bytes(), &reopened)
	if reopened.Revision != result.State.Revision || reopened.Treasury != state.Treasury {
		t.Fatal("read did not see restored state")
	}
}

func TestHistoryRoutesRejectStaleAndUnknownFieldsInBothLanguages(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, ":0")
	result := performJSONRequest(t, server, "POST", "/api/history/bootstrap", map[string]any{})
	var bootstrap appsvc.HistoryBootstrap
	decodeResponse(t, result.Body.Bytes(), &bootstrap)
	prefix := "/api/projects/" + bootstrap.ProjectID + "/history/campaigns"
	result = performJSONRequest(t, server, "POST", prefix, map[string]any{"name": "Stale", "command_id": "create"})
	var state history.State
	decodeResponse(t, result.Body.Bytes(), &state)
	prefix += "/" + state.ID
	for _, tc := range []struct{ locale, part string }{{"zh-CN", "已经有新变化"}, {"en-US", "has changed"}} {
		body := `{"command_id":"stale-command","expected_revision":0,"label":"save"}`
		response := ut.PerformRequest(server.engine.Engine, "POST", prefix+"/checkpoints", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "X-TA-Locale", Value: tc.locale})
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), tc.part) {
			t.Fatalf("wrong localized conflict: %d %s", response.Code, response.Body.String())
		}
		var problem map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem["code"] != "api.history.conflict" {
			t.Fatalf("error code missing: %+v", problem)
		}
	}
	response := performJSONRequest(t, server, "POST", prefix+"/advance", map[string]any{"command_id": "typo", "expected_revision": 1, "free_money": 999})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown financial field accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestHistoryCatalogKeepsHealthySavesAndCreationAvailableBesideCorruption(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, ":0")
	result := performJSONRequest(t, server, "POST", "/api/history/bootstrap", map[string]any{})
	var bootstrap appsvc.HistoryBootstrap
	decodeResponse(t, result.Body.Bytes(), &bootstrap)
	prefix := "/api/projects/" + bootstrap.ProjectID + "/history/campaigns"
	var states []history.State
	for _, id := range []string{"healthy", "damaged"} {
		result = performJSONRequest(t, server, "POST", prefix, map[string]any{"name": id, "command_id": id})
		if result.Code != http.StatusOK {
			t.Fatalf("create: %d %s", result.Code, result.Body.String())
		}
		var state history.State
		decodeResponse(t, result.Body.Bytes(), &state)
		states = append(states, state)
	}
	operation, err := application.AcquireProjectOperation(context.Background(), bootstrap.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(operation.Layout().StoreRoot, "history", states[1].ID, "journal.jsonl")
	operation.Release()
	damaged := []byte("{deliberately broken test save}\n")
	if err := os.WriteFile(journal, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	result = performJSONRequest(t, server, "POST", "/api/history/bootstrap", map[string]any{})
	if result.Code != http.StatusOK {
		t.Fatalf("one damaged save blocked bootstrap: %d %s", result.Code, result.Body.String())
	}
	decodeResponse(t, result.Body.Bytes(), &bootstrap)
	var healthy, unavailable bool
	for _, summary := range bootstrap.Campaigns {
		if summary.ID == states[0].ID && !summary.Unavailable {
			healthy = true
		}
		if summary.ID == states[1].ID && summary.Unavailable && summary.ErrorCode == "history.campaign_unreadable" {
			unavailable = true
		}
	}
	if !healthy || !unavailable || len(bootstrap.Campaigns) != 2 {
		t.Fatalf("catalog hid a save or its corruption: %+v", bootstrap.Campaigns)
	}
	result = performJSONRequest(t, server, "GET", prefix+"/"+states[0].ID, nil)
	if result.Code != http.StatusOK {
		t.Fatal("healthy save could not open")
	}
	result = performJSONRequest(t, server, "POST", prefix, map[string]any{"name": "New reign", "command_id": "new-after-damage"})
	if result.Code != http.StatusOK {
		t.Fatal("damaged save blocked a new game")
	}
	after, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(damaged) {
		t.Fatal("catalog silently replaced or reset damaged save")
	}
}
