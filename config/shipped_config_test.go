package config

import (
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// A portable development package contains model examples, not a personal
// gateway or credentials. Preserve valid profile wiring while requiring the
// first-run setup instead of pretending a cleared key is a working model.
func TestShippedStartupConfigRequiresPersonalModelConfiguration(t *testing.T) {
	path := filepath.Join("..", "config.example.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shipped config.example.toml: %v", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse shipped config.example.toml: %v", err)
	}

	settings := settingsFromConfig(&cfg)
	// Never ship a personal key, including on an unused endpoint or image profile.
	endpointByID := make(map[string]ModelEndpointSettings, len(settings.ModelEndpoints))
	for _, endpoint := range settings.ModelEndpoints {
		if endpoint.APIKey != "" {
			t.Fatal("shipped model endpoint must not contain credentials")
		}
		endpointByID[modelEndpointID(endpoint)] = endpoint
	}
	for _, profile := range settings.ImageAPIProfiles {
		if profile.APIKey != "" {
			t.Fatal("shipped image profile must not contain credentials")
		}
	}
	profileByID := make(map[string]ModelProfileSettings, len(settings.ModelProfiles))
	for _, profile := range settings.ModelProfiles {
		profileByID[modelProfileID(profile)] = profile
	}

	defaultProfileID := normalizeModelProfileID(cfg.AgentModels.Default.ProfileID)
	if defaultProfileID == "" {
		t.Fatal("shipped config.example.toml selects no default model profile")
	}
	profile, ok := profileByID[defaultProfileID]
	if !ok {
		t.Fatalf("default profile %q is not declared", defaultProfileID)
	}
	if profile.Model == "" {
		t.Fatalf("default profile %q declares no model", defaultProfileID)
	}
	endpoint, ok := endpointByID[profile.EndpointID]
	if !ok {
		t.Fatalf("default profile %q references undeclared endpoint %q", defaultProfileID, profile.EndpointID)
	}
	if endpoint.APIKey != "" {
		t.Fatal("fresh portable package must remain unconfigured until the user supplies a model")
	}
	if endpoint.BaseURL != "" {
		t.Fatal("portable source package must not retain a personal gateway endpoint")
	}
	if endpoint.Provider == "" || endpoint.Protocol == "" {
		t.Fatal("default endpoint must declare an explicit provider and protocol")
	}

	// The narrative Agent must enable thinking on a gateway that supports it,
	// while ordinary character chat stays non-thinking by leaving it unset.
	if got := resolvedThinkingLevel(cfg.AgentModels.InteractiveStory.ThinkingLevel); got == "" {
		t.Fatal("interactive story agent has no thinking level")
	}
	if cfg.AgentModels.Default.ThinkingLevel != "" {
		t.Fatalf("default agent thinking level = %q, want unset for non-thinking chat", cfg.AgentModels.Default.ThinkingLevel)
	}
}
