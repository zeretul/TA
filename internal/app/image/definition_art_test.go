package imageapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	agent "ta/agent"
	"ta/config"
	agenttools "ta/internal/agents/tools"
	"ta/internal/interactive"
	"testing"
)

func TestDefinitionArtConfiguredAvailabilityNeverInventsDefaults(t *testing.T) {
	if DefinitionArtAgentAvailable(config.Config{}) {
		t.Fatal("empty config enabled billing")
	}
	cfg := config.Config{ModelProfiles: []config.ModelProfileSettings{{ID: "art", Model: "fixture-agent", BaseURL: "http://127.0.0.1/fixture", APIKey: "fixture-key"}}, AgentModels: config.AgentModelSettings{Image: config.AgentModelOverride{ProfileID: "art"}}, OpenAIModel: "fixture-agent", OpenAIBaseURL: "http://127.0.0.1/fixture", OpenAIAPIKey: "fixture-key", DefaultImageAPIProfileID: "art-image", ImageAPIProfiles: []config.ImageAPIProfileSettings{{ID: "art-image", Provider: "openai", Protocol: "openai-images", BaseURL: "http://127.0.0.1/fixture", APIKey: "fixture-key", Model: "fixture-image"}}}
	if !DefinitionArtAgentAvailable(cfg) {
		t.Fatal("explicit configured built-in Image Agent unavailable")
	}
	cfg.AgentModels.Image.ProfileID = "stale-model"
	if DefinitionArtAgentAvailable(cfg) {
		t.Fatal("stale model selection silently fell back")
	}
	cfg.AgentModels.Image.ProfileID = "art"
	cfg.AgentTools.Image = config.AgentToolOverride{config.AgentToolImageGeneration: false}
	if DefinitionArtAgentAvailable(cfg) {
		t.Fatal("disabled image permission bypassed")
	}
	cfg.AgentTools.Image = nil
	cfg.DefaultImageAgentID = "missing-custom"
	if DefinitionArtAgentAvailable(cfg) {
		t.Fatal("missing custom agent silently fell back")
	}
	cfg.DefaultImageAgentID = ""
	cfg.ImageAPIProfiles[0].APIKey = ""
	if DefinitionArtAgentAvailable(cfg) {
		t.Fatal("missing image credentials silently filled")
	}
}

// Controlled HTTP fixture exercises the existing concrete image tool, provider
// adapter and real asset writer under the persisted job guard. It is not a
// real external image-model call and never uses user credentials.
func TestDefinitionArtGuardRealImageToolOneCallAndColdReopen(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint("failure=", failure), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/images/generations" {
					t.Errorf("unexpected provider path: %s", r.URL.Path)
				}
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request["n"] != float64(1) {
					t.Errorf("not single image: %+v", request)
				}
				w.Header().Set("Content-Type", "application/json")
				if failure {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"message":"controlled provider outage"}}`))
					return
				}
				png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aS3sAAAAASUVORK5CYII="
				if _, err := base64.StdEncoding.DecodeString(png); err != nil {
					t.Fatal(err)
				}
				_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
			}))
			defer server.Close()
			root := t.TempDir()
			store := interactive.NewStore(root)
			module, moduleErr := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
			if moduleErr != nil {
				t.Fatal(moduleErr)
			}
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Art fixture", ActorState: &module.ActorState})
			if err != nil {
				t.Fatal(err)
			}
			turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "An art appears."})
			if err != nil {
				t.Fatal(err)
			}
			job := interactive.DefinitionArtJob{DefinitionID: "fire_bolt", Version: 1, Category: "skill", Name: "Fire", Style: "realistic"}
			if ok, err := store.TransitionDefinitionArt(story.ID, "main", turn.ID, job, "", "queued", nil); err != nil || !ok {
				t.Fatal(err)
			}
			cfg := config.Config{Workspace: root, DefaultImageAPIProfileID: "fixture", ImageAPIProfiles: []config.ImageAPIProfileSettings{{ID: "fixture", Provider: "openai", Protocol: "openai-images", APIKey: "fixture-not-secret", BaseURL: server.URL, Model: "fixture", DefaultOutputFormat: "png"}}}
			definitions, err := agenttools.NewCatalog(&cfg, nil, agenttools.RuntimeExecutables{}).Image()(config.ResolvedAgentToolSettings{config.AgentToolImageGeneration: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(definitions) != 1 {
				t.Fatalf("unexpected image tools: %d", len(definitions))
			}
			guard := &definitionArtGuard{store: store, storyID: story.ID, branchID: "main", turnID: turn.ID, projectID: "project-fixture", job: job}
			endpoint, err := guard.WrapToolCall(context.Background(), definitions[0].Tool.Run, &agent.ToolContext{Name: "generate_image"})
			if err != nil {
				t.Fatal(err)
			}
			_, callErr := endpoint(context.Background(), `{"purpose":"general","prompt":"a pale flame","n":1}`)
			if failure && callErr == nil || !failure && callErr != nil {
				t.Fatalf("unexpected controlled result: %v", callErr)
			}
			if calls.Load() != 1 {
				t.Fatalf("provider called %d times", calls.Load())
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store = interactive.NewStore(root)
			defer store.Close()
			restored := &definitionArtGuard{store: store, storyID: story.ID, branchID: "main", turnID: turn.ID, projectID: "project-fixture", job: job}
			endpoint, _ = restored.WrapToolCall(context.Background(), definitions[0].Tool.Run, &agent.ToolContext{Name: "generate_image"})
			if _, err := endpoint(context.Background(), `{"purpose":"general","prompt":"retry"}`); err == nil {
				t.Fatal("cold retry was allowed")
			}
			if calls.Load() != 1 {
				t.Fatal("cold retry charged again")
			}
			refs, err := store.DefinitionArtReferences(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			art, exists := refs["fire_bolt:1"]
			if failure && exists || !failure && !exists {
				t.Fatalf("unexpected art reference: %+v", refs)
			}
			if exists {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(art.ImagePath))); err != nil {
					t.Fatalf("real asset missing: %v", err)
				}
			}
		})
	}
}

func TestDefinitionArtGuardRejectsExtraWorkBeforeProvider(t *testing.T) {
	var calls int
	guard := &definitionArtGuard{}
	next := func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
		calls++
		return agent.TextToolResult("unused"), nil
	}
	for _, test := range []struct{ name, args string }{{"write_lore_items", `{}`}, {"generate_image", `{"purpose":"book_cover","n":1}`}, {"generate_image", `{"purpose":"general","n":2}`}} {
		endpoint, _ := guard.WrapToolCall(context.Background(), next, &agent.ToolContext{Name: test.name})
		if _, err := endpoint(context.Background(), test.args); err == nil {
			t.Fatalf("extra operation accepted: %+v", test)
		}
	}
	if calls != 0 {
		t.Fatal("invalid art work reached provider")
	}
}
