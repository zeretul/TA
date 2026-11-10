package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"ta/config"
	"ta/internal/interactive"
)

// This is full production service/Image Agent orchestration against local
// controlled model/provider HTTP fixtures, NOT a real external image call.
func TestDefinitionArtServiceUsesActualImageAgentAndDeduplicates(t *testing.T) {
	var modelCalls, imageCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch r.URL.Path {
		case "/v1/images/generations":
			imageCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aS3sAAAAASUVORK5CYII="}]}`)
		case "/v1/chat/completions":
			modelCalls.Add(1)
			hasTool := false
			for _, raw := range body["messages"].([]any) {
				if message, ok := raw.(map[string]any); ok && message["role"] == "tool" {
					hasTool = true
				}
			}
			message := map[string]any{"role": "assistant", "content": "插图已完成。"}
			finish := "stop"
			if !hasTool {
				message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "call-art", "type": "function", "function": map[string]any{"name": "generate_image", "arguments": `{"purpose":"general","prompt":"a pale spiritual flame","n":1}`}}}}
				finish = "tool_calls"
			}
			if stream, _ := body["stream"].(bool); stream {
				w.Header().Set("Content-Type", "text/event-stream")
				delta := map[string]any{"role": "assistant", "content": message["content"]}
				if !hasTool {
					call := message["tool_calls"].([]any)[0].(map[string]any)
					call["index"] = 0
					delta["tool_calls"] = []any{call}
				}
				data, _ := json.Marshal(map[string]any{"id": "fixture-art", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				data, _ = json.Marshal(map[string]any{"id": "fixture-art", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			} else {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture-art", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
			}
		default:
			http.Error(w, "unexpected fixture endpoint", 404)
		}
	}))
	defer server.Close()
	data, workspace := t.TempDir(), t.TempDir()
	t.Setenv("TA_DIR", data)
	settings := config.Settings{
		OpenAIModel: "fixture-agent", OpenAIBaseURL: server.URL + "/v1", OpenAIAPIKey: "fixture-only",
		AgentModels:              config.AgentModelSettings{Image: config.AgentModelOverride{ProfileID: "art-agent"}},
		ModelEndpoints:           []config.ModelEndpointSettings{{ID: "fixture", Provider: "openai-compatible", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "fixture-only"}},
		ModelProfiles:            []config.ModelProfileSettings{{ID: "art-agent", EndpointID: "fixture", Model: "fixture-agent"}},
		DefaultImageAPIProfileID: "art-provider",
		ImageAPIEndpoints:        []config.ImageAPIEndpointSettings{{ID: "fixture-image", Provider: "openai", Protocol: "openai-images", BaseURL: server.URL + "/v1", APIKey: "fixture-only"}},
		ImageAPIProfiles:         []config.ImageAPIProfileSettings{{ID: "art-provider", EndpointID: "fixture-image", Model: "fixture-image", DefaultOutputFormat: "png"}},
		AgentApprovalMode:        config.AgentApprovalFullAccess,
	}
	if err := config.WriteSettingsFile(config.UserConfigPath(data), settings); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.LoadWithWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	application, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	module, err := interactive.NewActorStateLibrary(data).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	story, err := application.interactive.CreateStory(interactive.CreateStoryRequest{Title: "Full art service", ActorState: &module.ActorState})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := application.interactive.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "A registered skill is illustrated."})
	if err != nil {
		t.Fatal(err)
	}
	before, err := application.interactive.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	job := interactive.DefinitionArtJob{DefinitionID: "fire_bolt", Version: 1, Category: "skill", Name: "Fire", Description: "Pale spiritual flame", Style: "realistic"}
	if err := application.Images().GenerateDefinitionArt(ctx, workspace, story.ID, "main", turn.ID, job); err != nil {
		t.Fatal(err)
	}
	refs, err := application.interactive.DefinitionArtReferences(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	art, ok := refs["fire_bolt:1"]
	if !ok {
		t.Fatalf("full service published no art; model=%d image=%d", modelCalls.Load(), imageCalls.Load())
	}
	if modelCalls.Load() != 2 || imageCalls.Load() != 1 {
		t.Fatalf("unexpected model/provider counts: %d/%d", modelCalls.Load(), imageCalls.Load())
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(art.ImagePath))); err != nil {
		t.Fatal(err)
	}
	if err := application.Images().GenerateDefinitionArt(ctx, workspace, story.ID, "main", turn.ID, job); err != nil {
		t.Fatal(err)
	}
	if modelCalls.Load() != 2 || imageCalls.Load() != 1 {
		t.Fatal("repeated service request rebilled")
	}
	after, err := application.interactive.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || before.CurrentTurn.ID != after.CurrentTurn.ID {
		t.Fatal("image service changed game facts or head")
	}
}
