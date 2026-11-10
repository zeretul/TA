package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ta/config"
	"ta/internal/interactive"
)

func TestParseEnemyDecisionAcceptsOnlyThisActorsCandidate(t *testing.T) {
	candidates := []interactive.EnemyDecisionCandidate{{ID: "guard"}, {ID: "skill:own"}}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"legal", `{"candidate_id":"skill:own"}`, "skill:own"},
		{"fenced", "```json\n{\"candidate_id\":\"guard\"}\n```", "guard"},
		{"whitespace", `{"candidate_id":" guard "}`, "guard"},
		{"other actor skill", `{"candidate_id":"skill:someone-else"}`, ""},
		{"arbitrary action", `{"candidate_id":"guard","action":{"damage":99999}}`, ""},
		{"reasoning", `{"candidate_id":"guard","reasoning":"private analysis"}`, ""},
		{"duplicate", `{"candidate_id":"guard","candidate_id":"skill:own"}`, ""},
		{"trailing object", `{"candidate_id":"guard"}{"candidate_id":"skill:own"}`, ""},
		{"trailing prose", `{"candidate_id":"guard"} I chose guard.`, ""},
		{"number", `{"candidate_id":17}`, ""},
		{"null", `{"candidate_id":null}`, ""},
		{"array", `[{"candidate_id":"guard"}]`, ""},
		{"empty", `{}`, ""},
		{"oversized", strings.Repeat(" ", enemyDecisionMaxOutputBytes) + `{"candidate_id":"guard"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnemyDecision(tc.input, candidates)
			if got != tc.want || (err == nil) != (tc.want != "") {
				t.Fatalf("selection = %q, err = %v; want %q", got, err, tc.want)
			}
		})
	}
}

type enemyDecisionTestRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Tools []any `json:"tools"`
}

func enemyDecisionTestConfig(t *testing.T, workspace string, answer func(enemyDecisionTestRequest) string) *config.Config {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request enemyDecisionTestRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode model request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "enemy-decision-fixture", "object": "chat.completion",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": answer(request), "reasoning_content": "private model reasoning must never be a committed action",
			}}},
		})
	}))
	t.Cleanup(server.Close)
	return &config.Config{
		NovaDir: workspace, Workspace: workspace,
		OpenAIBaseURL: server.URL + "/v1", OpenAIModel: "enemy-fixture-model", OpenAIAPIKey: "fixture-only",
		ModelEndpoints: []config.ModelEndpointSettings{{ID: "enemy-fixture", Provider: "openai-compatible", Protocol: "openai-chat-completions", BaseURL: server.URL + "/v1", APIKey: "fixture-only"}},
		ModelProfiles:  []config.ModelProfileSettings{{ID: "enemy-fixture", EndpointID: "enemy-fixture", Model: "enemy-fixture-model"}},
		AgentModels:    config.AgentModelSettings{InteractiveStory: config.AgentModelOverride{ProfileID: "enemy-fixture", ThinkingLevel: "low"}},
	}
}

func enemyDecisionTestActor(id string) interactive.EnemyDecisionActor {
	return interactive.EnemyDecisionActor{
		ActorID: id, Name: id, Personality: []string{"谨慎"}, Motives: []string{"保护自己"}, Bounds: []string{"不滥杀"}, Attitude: "戒备",
		KnownSituation: []string{"对方已在上一轮受伤"},
		Candidates:     []interactive.EnemyDecisionCandidate{{ID: "own-guard-" + id, Description: "防备来袭"}, {ID: "own-attack-" + id, Description: "使用本人已学招式"}},
	}
}

func TestEnemyDecisionCallsConfiguredModelWithIndependentKnowledge(t *testing.T) {
	application, storyID := combatTestApp(t)
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		calls.Add(1)
		if request.Model != "enemy-fixture-model" || len(request.Messages) != 2 || len(request.Tools) != 0 {
			t.Errorf("unexpected model contract: %+v", request)
			return `{}`
		}
		if !strings.Contains(request.Messages[0].Content, "Output only the JSON object") || strings.Contains(request.Messages[0].Content, "Output only the story prose") {
			t.Error("decision prompt conflicts with structured output")
		}
		var input struct {
			Round int                            `json:"round"`
			Enemy interactive.EnemyDecisionActor `json:"enemy"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil {
			t.Errorf("decode decision context: %v", err)
			return `{}`
		}
		if input.Round != 3 || len(input.Enemy.Candidates) != 2 || len(input.Enemy.Motives) != 1 {
			t.Errorf("decision context lost its source facts: %+v", input)
		}
		other := "enemy-A"
		if input.Enemy.ActorID == other {
			other = "enemy-B"
		}
		if strings.Contains(request.Messages[1].Content, other) {
			t.Errorf("%s learned %s's private menu", input.Enemy.ActorID, other)
		}
		return `{"candidate_id":"` + input.Enemy.Candidates[1].ID + `"}`
	})
	snapshot := interactive.EnemyDecisionSnapshot{Head: "not-model-context", Round: 3, Enemies: []interactive.EnemyDecisionActor{enemyDecisionTestActor("enemy-A"), enemyDecisionTestActor("enemy-B")}}
	choices, status, err := application.decideEnemyActions(context.Background(), application.interactive, storyID, "main", snapshot)
	if err != nil || status != "ai" || choices["enemy-A"] != "own-attack-enemy-A" || choices["enemy-B"] != "own-attack-enemy-B" || calls.Load() != 2 {
		t.Fatalf("model decisions = %+v, status = %s, calls = %d, err = %v", choices, status, calls.Load(), err)
	}
}

func TestEnemyDecisionInvalidActorDoesNotDiscardOtherValidChoice(t *testing.T) {
	application, storyID := combatTestApp(t)
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		// This is valid only for A, even though B is on the same side.
		return `{"candidate_id":"own-guard-enemy-A"}`
	})
	choices, status, err := application.decideEnemyActions(context.Background(), application.interactive, storyID, "main", interactive.EnemyDecisionSnapshot{Round: 1, Enemies: []interactive.EnemyDecisionActor{enemyDecisionTestActor("enemy-A"), enemyDecisionTestActor("enemy-B")}})
	if err != nil || status != "ai_partial_fallback" || len(choices) != 1 || choices["enemy-A"] != "own-guard-enemy-A" {
		t.Fatalf("partial decisions = %+v, status = %s, err = %v", choices, status, err)
	}
}

func TestEnemyDecisionUnavailableAndCanceledAreDifferentOutcomes(t *testing.T) {
	application, storyID := combatTestApp(t)
	snapshot := interactive.EnemyDecisionSnapshot{Round: 1, Enemies: []interactive.EnemyDecisionActor{enemyDecisionTestActor("enemy")}}
	choices, status, err := application.decideEnemyActions(context.Background(), application.interactive, storyID, "main", snapshot)
	if err != nil || status != "fallback_unavailable" || len(choices) != 0 {
		t.Fatalf("unconfigured decision = %+v, %s, %v", choices, status, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if choices, status, err := application.decideEnemyActions(ctx, application.interactive, storyID, "main", snapshot); !errors.Is(err, context.Canceled) || len(choices) != 0 || status != "" {
		t.Fatalf("canceled decision unexpectedly became fallback: %+v, %s, %v", choices, status, err)
	}
}

func TestEnemyDecisionRejectsOversizedContextBeforeModelCall(t *testing.T) {
	var calls atomic.Int32
	cfg := enemyDecisionTestConfig(t, t.TempDir(), func(enemyDecisionTestRequest) string {
		calls.Add(1)
		return `{"candidate_id":"guard"}`
	})
	actor := enemyDecisionTestActor("enemy")
	actor.KnownSituation = []string{strings.Repeat("x", enemyDecisionMaxInputBytes)}
	_, err := generateEnemyDecision(context.Background(), cfg, "story", "main", 1, actor)
	if !errors.Is(err, errEnemyDecisionContextLimit) || calls.Load() != 0 {
		t.Fatalf("oversized input called model: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestEnemyDecisionDoesNotUseReasoningAsFinalSelection(t *testing.T) {
	cfg := enemyDecisionTestConfig(t, t.TempDir(), func(enemyDecisionTestRequest) string { return "" })
	content, err := generateEnemyDecision(context.Background(), cfg, "story", "main", 1, enemyDecisionTestActor("enemy"))
	if err != nil || content != "" {
		t.Fatalf("reasoning leaked into final selection: %q, %v", content, err)
	}
	if _, err := parseEnemyDecision(content, enemyDecisionTestActor("enemy").Candidates); err == nil {
		t.Fatal("missing final choice was treated as a legal decision")
	}
}

func TestEnemyDecisionProviderInputGuardRunsBeforeCall(t *testing.T) {
	var calls atomic.Int32
	cfg := enemyDecisionTestConfig(t, t.TempDir(), func(enemyDecisionTestRequest) string {
		calls.Add(1)
		return `{"candidate_id":"guard"}`
	})
	oneToken := 1
	cfg.ModelProfiles[0].ContextWindowTokens = &oneToken
	_, err := generateEnemyDecision(context.Background(), cfg, "story", "main", 1, enemyDecisionTestActor("enemy"))
	if err == nil || calls.Load() != 0 {
		t.Fatalf("provider context boundary was bypassed: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestEnemyDecisionCancellationDuringModelCallDoesNotBecomeFallback(t *testing.T) {
	application, storyID := combatTestApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	application.cfg = enemyDecisionTestConfig(t, application.workspace, func(request enemyDecisionTestRequest) string {
		calls.Add(1)
		cancel()
		return `{"candidate_id":"own-guard-enemy"}`
	})
	choices, status, err := application.decideEnemyActions(ctx, application.interactive, storyID, "main", interactive.EnemyDecisionSnapshot{Round: 1, Enemies: []interactive.EnemyDecisionActor{enemyDecisionTestActor("enemy")}})
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 || len(choices) != 0 || status != "" {
		t.Fatalf("in-flight cancellation became a playable fallback: calls = %d, choices = %+v, status = %s, err = %v", calls.Load(), choices, status, err)
	}
}

func TestEnemyDecisionExcludesNarratorSecretsFromEveryModelMessage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*config.Config)
	}{
		{"default narrator", func(cfg *config.Config) {
			cfg.AgentPrompts.Default.SystemPrompt = "NARRATOR_PRIVATE_CANARY knows the protagonist's secret inventory."
		}},
		{"story narrator", func(cfg *config.Config) {
			cfg.AgentPrompts.InteractiveStory.SystemPrompt = "NARRATOR_PRIVATE_CANARY knows the hidden escape route."
			cfg.AgentPrompts.InteractiveStory.FlowPrompt = "NARRATOR_PRIVATE_CANARY owns the future plot."
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			cfg := enemyDecisionTestConfig(t, t.TempDir(), func(request enemyDecisionTestRequest) string {
				calls.Add(1)
				all, err := json.Marshal(request)
				if err != nil {
					t.Errorf("encode captured request: %v", err)
				}
				if strings.Contains(string(all), "NARRATOR_PRIVATE_CANARY") {
					t.Error("author-only narrator instruction reached an enemy's model call")
				}
				if request.Model != "enemy-fixture-model" || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, "own-guard-enemy") {
					t.Errorf("isolation discarded the configured model or legitimate menu: %+v", request)
				}
				return `{"candidate_id":"own-guard-enemy"}`
			})
			tc.configure(cfg)
			before := cfg.AgentPrompts
			content, err := generateEnemyDecision(context.Background(), cfg, "story", "main", 1, enemyDecisionTestActor("enemy"))
			if err != nil || calls.Load() != 1 || content != `{"candidate_id":"own-guard-enemy"}` {
				t.Fatalf("isolated decision failed: calls=%d content=%q err=%v", calls.Load(), content, err)
			}
			if cfg.AgentPrompts != before {
				t.Fatal("isolating one decision changed the narrator's saved configuration")
			}
		})
	}
}
