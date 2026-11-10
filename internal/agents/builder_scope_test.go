package agents

import (
	"context"
	"testing"

	"ta/config"
	producttools "ta/internal/agents/tools"
	"ta/internal/agents/prompts"
)

func scopedHost(scope string) AgentHostCapabilities {
	return AgentHostCapabilities{
		Interactive: true,
		ToolScope: &producttools.IDEContext{
			LoreScope:       &scope,
			ConfigResources: []string{"narrative_style", "rule_system"},
		},
	}
}

func TestNarrowSettingsRemovesEveryCapabilityOutsideThePurpose(t *testing.T) {
	host := scopedHost("world-a")
	settings := host.resolveToolSettings(&config.Config{}, config.AgentKindIDE)
	if settings == nil {
		t.Fatal("scoped host produced no settings")
	}
	for _, capability := range ScopedAgentToolCapabilities {
		if !settings.Allows(capability) {
			t.Errorf("scoped settings dropped %q", capability)
		}
	}
	for _, capability := range []string{
		config.AgentToolWorkspaceWrite, config.AgentToolShell,
		config.AgentToolWebSearch, config.AgentToolWebFetch, config.AgentToolBrowser,
		config.AgentToolDelegation, config.AgentToolScript,
		config.AgentToolTrajectory,
	} {
		if settings.Allows(capability) {
			t.Errorf("scoped settings kept %q", capability)
		}
	}
}

func TestNarrowSettingsLeavesAnUnscopedHostAlone(t *testing.T) {
	host := AgentHostCapabilities{Interactive: true}
	if settings := host.resolveToolSettings(&config.Config{}, config.AgentKindIDE); settings != nil {
		t.Fatalf("unscoped host narrowed settings to %v", settings)
	}
	if settings := host.narrowSettings(ExternalConversationToolSettings(config.AgentKindIDE)); settings != nil {
		t.Fatalf("unscoped host narrowed external settings to %v", settings)
	}
}

func TestScopedExternalAssemblyRegistersNoHostOrDelegationTools(t *testing.T) {
	cfg := &config.Config{Workspace: t.TempDir(), DenovaDir: t.TempDir()}
	assembly, err := BuildExternalConversationAssembly(
		context.Background(), cfg, nil, prompts.IDEStoryTeller{}, config.AgentKindIDE, scopedHost("world-a"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range []string{
		config.AgentToolWorkspaceWrite, config.AgentToolShell,
		config.AgentToolWebSearch, config.AgentToolWebFetch, config.AgentToolBrowser,
		config.AgentToolDelegation, config.AgentToolScript,
	} {
		if assembly.ToolSettings.Allows(capability) {
			t.Errorf("external assembly kept capability %q", capability)
		}
	}
	names := make(map[string]struct{}, len(assembly.Tools))
	for _, definition := range assembly.Tools {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = struct{}{}
	}
	// Attachment reading and retrieval stay: the purpose keeps filesystem_read
	// so the assistant can still open what the user attached. Everything that
	// writes the host, reaches the network or delegates is gone.
	for _, forbidden := range []string{
		"write", "edit", "shell", "bash", "pwsh",
		"web_search", "web_fetch", "browser", "send", "await", "list_agents", "task", "task_wait",
	} {
		if _, ok := names[forbidden]; ok {
			t.Errorf("scoped external assembly registered %q", forbidden)
		}
	}
	for _, required := range []string{"read", "glob", "grep", "read_lore_items", "write_lore_items", "config_read", "config_apply", "generate_image"} {
		if _, ok := names[required]; !ok {
			t.Errorf("scoped external assembly is missing %q", required)
		}
	}
}

func TestUnscopedExternalAssemblyKeepsTheEditorSurface(t *testing.T) {
	cfg := &config.Config{Workspace: t.TempDir(), DenovaDir: t.TempDir()}
	assembly, err := BuildExternalConversationAssembly(
		context.Background(), cfg, nil, prompts.IDEStoryTeller{}, config.AgentKindIDE,
		AgentHostCapabilities{Interactive: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]struct{}, len(assembly.Tools))
	for _, definition := range assembly.Tools {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = struct{}{}
	}
	for _, required := range []string{"read", "write", "config_read"} {
		if _, ok := names[required]; !ok {
			t.Errorf("unscoped external assembly lost %q", required)
		}
	}
}
