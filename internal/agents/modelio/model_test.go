package modelio

import (
	"strings"
	"testing"

	"ta/agent/providers"

	"ta/config"
)

func TestChatModelConfigFromResolvedKeepsNeutralThinkingLevel(t *testing.T) {
	mapping := &providers.SessionKeyMapping{Location: providers.SessionKeyLocationHeader, Name: "X-Session-Id"}
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{
		BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai/",
		Model:             "gemini-3.5-flash",
		ThinkingLevel:     "xhigh",
		SessionKeyMapping: mapping,
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.ThinkingLevel != providers.ThinkingLevelXHigh {
		t.Fatalf("thinking level = %q, want xhigh", modelCfg.ThinkingLevel)
	}
	if modelCfg.SessionKeyMapping != mapping {
		t.Fatalf("session key mapping = %#v", modelCfg.SessionKeyMapping)
	}
}

func TestConfigFromResolvedKeepsProfileMaxTokens(t *testing.T) {
	maxTokens := 16384
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{MaxTokens: &maxTokens})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.MaxOutputTokens == nil || *modelCfg.MaxOutputTokens != maxTokens {
		t.Fatalf("max output tokens = %#v, want %d", modelCfg.MaxOutputTokens, maxTokens)
	}
}

func TestConfigFromResolvedRejectsUnencodableProtocolOptions(t *testing.T) {
	_, err := ConfigFromResolved(config.ResolvedModelSettings{
		ProtocolOptions: map[string]any{"invalid": func() {}},
	})
	if err == nil || !strings.Contains(err.Error(), "convert model protocol options") {
		t.Fatalf("error = %v", err)
	}
}

func TestChatModelConfigFromResolvedKeepsOffThinkingLevel(t *testing.T) {
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{
		BaseURL:       "https://api.deepseek.com/v1",
		Model:         "deepseek-v4-pro",
		ThinkingLevel: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.ThinkingLevel != providers.ThinkingLevelOff {
		t.Fatalf("thinking level = %q, want off", modelCfg.ThinkingLevel)
	}
}

func TestCanWriteFollowsTheRouteARealCallResolves(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	if err := CanWrite(&config.Config{}); err == nil {
		t.Fatal("an empty configuration must not report a usable model")
	}

	// A local provider supplies its own base URL and needs no API key, so the
	// settings-layer reading that demands a key is wrong about this machine.
	local := &config.Config{
		ModelEndpoints: []config.ModelEndpointSettings{{ID: "ollama", Provider: "ollama"}},
		ModelProfiles:  []config.ModelProfileSettings{{ID: "local", EndpointID: "ollama", Model: "qwen3:8b"}},
		AgentModels:    config.AgentModelSettings{InteractiveStory: config.AgentModelOverride{ProfileID: "local"}},
	}
	if err := CanWrite(local); err != nil {
		t.Fatalf("a keyless local provider must count as usable: %v", err)
	}

	// Environment overrides are the highest-precedence route and never reach the
	// persisted settings layers.
	t.Setenv("OPENAI_API_KEY", "n54-key")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:19739/v1")
	t.Setenv("OPENAI_MODEL", "n54-model")
	fromEnv := &config.Config{}
	config.ApplyModelEnvironment(fromEnv)
	if err := CanWrite(fromEnv); err != nil {
		t.Fatalf("a model configured through the environment must count as usable: %v", err)
	}
}

func TestLoggableModelBaseURLRemovesCredentialsAndQuery(t *testing.T) {
	got := loggableModelBaseURL("https://user:password@api.example.test/v1?api_key=secret#fragment")
	if got != "https://api.example.test/v1" {
		t.Fatalf("loggable base URL = %q", got)
	}
	if got := loggableModelBaseURL("not a URL?api_key=secret"); got != "<invalid>" {
		t.Fatalf("invalid loggable base URL = %q", got)
	}
}
