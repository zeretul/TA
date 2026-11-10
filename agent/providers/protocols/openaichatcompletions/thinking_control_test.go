package openaichatcompletions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "ta/agent"
	"ta/agent/providers"
)

// The WB gateway is reached through the "openai-compatible" provider preset,
// whose only chat endpoint is declared with an empty Compatibility:
//
//	{ID: providers.ProviderOpenAICompatible, Endpoints: {
//	    ProtocolOpenAIChatCompletions: {ProtocolOptions: openAIChat}}}
//
// That matters for the thinking control, because an empty Compatibility leaves
// SupportsReasoningEffort nil, and resolveCompatibility defaults it to true. So
// the preset carries no thinking *toggle*, but it does carry a reasoning
// *effort* — which means the control is expressed on the wire as
// `reasoning_effort`, not as `thinking` or `enable_thinking`.
//
// This is the thing a reader cannot learn from "the response had no reasoning
// field": the request-side field is `reasoning_effort`, and "off" maps to the
// literal "none" rather than to an omitted field. A test that only asserted the
// absence of a response field would pass even if nothing were sent at all.
func TestOpenAICompatiblePresetCarriesThinkingControlAsReasoningEffort(t *testing.T) {
	compatibility, err := resolveCompatibility(providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if compatibility.ThinkingToggle != ThinkingToggleNone {
		t.Fatalf("the openai-compatible preset declared a thinking toggle %q; "+
			"the wire field asserted below would then not be the whole control",
			compatibility.ThinkingToggle)
	}
	if compatibility.SupportsReasoningEffort == nil || !*compatibility.SupportsReasoningEffort {
		t.Fatal("SupportsReasoningEffort is not defaulted true, so no effort is emitted")
	}
	// The control the character window relies on.
	if effort, ok := compatibility.mappedEffort(providers.ThinkingLevelOff); !ok || effort != "none" {
		t.Fatalf("off maps to %q, %t; want \"none\", true", effort, ok)
	}
	// And the control the main narrative relies on.
	if effort, ok := compatibility.mappedEffort(providers.ThinkingLevelHigh); !ok || effort != "high" {
		t.Fatalf("high maps to %q, %t; want \"high\", true", effort, ok)
	}
	// The narrative's own default is pinned by the branch runtime config, which
	// the acceptance journal records as thinking_level=high. It must survive to
	// the wire rather than being dropped as "default".
	if fields := compatibility.thinkingFields(providers.ThinkingLevelHigh); fields != nil {
		t.Fatalf("the preset emitted an unexpected thinking field: %#v", fields)
	}
}

// TestCharacterWindowAndNarrativeSendDifferentThinkingLevels drives the two
// levels the product actually uses through a real request and reads the bodies
// back. The narrative turn runs at high and the character window runs with the
// control off, so the two must not produce the same request.
func TestCharacterWindowAndNarrativeSendDifferentThinkingLevels(t *testing.T) {
	bodies := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		bodies <- body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"wb-test","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	send := func(level providers.ThinkingLevel) map[string]any {
		model, err := newTestModel(context.Background(), &providers.ModelConfig{
			Provider: providers.ProviderOpenAICompatible, APIKey: "placeholder",
			Model: "deepseek-v4.1-flash", BaseURL: server.URL + "/v1",
			HTTPClient: server.Client(), ThinkingLevel: level,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := model.Generate(context.Background(), []*agent.Message{
			{Role: agent.System, Content: "system"},
			{Role: agent.User, Content: "hello"},
		}); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(<-bodies, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	narrative := send(providers.ThinkingLevelHigh)
	window := send(providers.ThinkingLevelOff)

	narrativeEffort, _ := narrative["reasoning_effort"].(string)
	windowEffort, _ := window["reasoning_effort"].(string)
	if narrativeEffort != "high" {
		t.Fatalf("the narrative turn sent reasoning_effort=%q, want \"high\": %#v", narrativeEffort, narrative)
	}
	if windowEffort != "none" {
		t.Fatalf("the character window sent reasoning_effort=%q, want \"none\": %#v", windowEffort, window)
	}
	if narrativeEffort == windowEffort {
		t.Fatal("the two product levels produced the same request, so the control is inert")
	}

	// Neither arm may smuggle the control through a second field: a gateway that
	// honoured both would let the two disagree.
	for name, body := range map[string]map[string]any{"narrative": narrative, "window": window} {
		if _, exists := body["enable_thinking"]; exists {
			t.Fatalf("%s request also set enable_thinking: %#v", name, body)
		}
		if _, exists := body["thinking"]; exists {
			t.Fatalf("%s request also set thinking: %#v", name, body)
		}
	}
}

// TestThinkingLevelDefaultSendsNoControl pins the other end: an unset level must
// leave the provider to its own default rather than being silently rewritten to
// "none". Without this, "off" and "unset" would be indistinguishable on the
// wire, and the character window's explicit choice would mean nothing.
func TestThinkingLevelDefaultSendsNoControl(t *testing.T) {
	compatibility, err := resolveCompatibility(providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if effort, ok := compatibility.mappedEffort(providers.ThinkingLevelDefault); ok {
		t.Fatalf("an unset level emitted reasoning_effort=%q", effort)
	}
	if effort, ok := compatibility.mappedEffort(""); ok {
		t.Fatalf("an empty level emitted reasoning_effort=%q", effort)
	}
}
