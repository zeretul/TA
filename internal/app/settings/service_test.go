package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/config"
)

type reloadTestHost struct {
	runtime Runtime
	applied config.LayeredSettings
	layer   config.SettingsLayer
}

func (host *reloadTestHost) SettingsRuntime(Target) (Runtime, error) { return host.runtime, nil }

func (host *reloadTestHost) ApplySettings(settings config.LayeredSettings, layer config.SettingsLayer) {
	host.applied = settings
	host.layer = layer
}

func TestSnapshotAnswersModelReadinessFromTheRuntimeConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}

	unset := NewService(&reloadTestHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir, NovaDir: dataDir}}})
	layered, err := unset.Snapshot(Global())
	if err != nil {
		t.Fatal(err)
	}
	if layered.Runtime.StoryModelReady == nil || *layered.Runtime.StoryModelReady {
		t.Fatalf("story model readiness with no model = %#v, want false", layered.Runtime.StoryModelReady)
	}

	// A local provider needs no API key, and its preset supplies the base URL.
	// Reading the persisted layers alone calls that "no model"; the runtime
	// configuration is what a real call resolves against.
	local := NewService(&reloadTestHost{runtime: Runtime{Config: config.Config{
		DenovaDir: dataDir, NovaDir: dataDir, OpenAIModel: "qwen3:8b",
		ModelEndpoints: []config.ModelEndpointSettings{{ID: "ollama", Provider: "ollama"}},
		ModelProfiles:  []config.ModelProfileSettings{{ID: "local", EndpointID: "ollama", Model: "qwen3:8b"}},
		AgentModels:    config.AgentModelSettings{InteractiveStory: config.AgentModelOverride{ProfileID: "local"}},
	}}})
	layered, err = local.Snapshot(Global())
	if err != nil {
		t.Fatal(err)
	}
	if layered.Runtime.StoryModelReady == nil || !*layered.Runtime.StoryModelReady {
		t.Fatalf("story model readiness for a keyless local provider = %#v, want true", layered.Runtime.StoryModelReady)
	}
	if layered.Effective.ModelProfiles != nil {
		t.Fatalf("the persisted layers must stay untouched, got profiles %#v", layered.Effective.ModelProfiles)
	}
}

func TestSnapshotPublishesReadonlyCompactionPromptSources(t *testing.T) {
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	service := NewService(&reloadTestHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir, NovaDir: dataDir}}})

	layered, err := service.Snapshot(Global())
	if err != nil {
		t.Fatal(err)
	}
	if len(layered.BuiltinCompactionSources.IDE.Sources) != 3 ||
		!strings.Contains(layered.BuiltinCompactionSources.IDE.Sources[2].Content, "Workspace/writing requirements") {
		t.Fatalf("Writing Agent compaction sources = %#v", layered.BuiltinCompactionSources.IDE)
	}
	if !strings.Contains(layered.BuiltinCompactionSources.InteractiveStory.Sources[2].Content, "Game-mode requirements") {
		t.Fatalf("Game Agent compaction sources = %#v", layered.BuiltinCompactionSources.InteractiveStory)
	}
}

func TestReloadAppliesOutOfBandAgentProfileMutation(t *testing.T) {
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	profile := []byte("schema_version = 1\nkind = \"general\"\n\n[prompt]\nsystem_prompt = \"Use the updated profile.\"\n")
	if err := os.WriteFile(filepath.Join(config.AgentProfilesRoot(dataDir), "main", "general.toml"), profile, 0o644); err != nil {
		t.Fatal(err)
	}
	host := &reloadTestHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir, NovaDir: dataDir}}}
	service := NewService(host)

	layered, err := service.Reload(Global(), config.SettingsLayerUser)
	if err != nil {
		t.Fatal(err)
	}
	if got := layered.User.AgentPrompts.General.SystemPrompt; got != "Use the updated profile." {
		t.Fatalf("reloaded General Agent prompt = %q", got)
	}
	if got := host.applied.User.AgentPrompts.General.SystemPrompt; got != "Use the updated profile." {
		t.Fatalf("applied General Agent prompt = %q", got)
	}
	if host.layer != config.SettingsLayerUser {
		t.Fatalf("applied layer = %q", host.layer)
	}
}
