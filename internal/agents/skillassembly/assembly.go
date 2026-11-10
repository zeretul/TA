// Package skillassembly projects one effective Skill catalog into the model
// prompt, callable tool, and reference-read surfaces used by an Agent.
package skillassembly

import (
	"context"
	"fmt"

	agent "ta/agent"

	"ta/config"
	"ta/internal/agents/prompts"
	novaskills "ta/internal/agents/skills"
	agenttoolruntime "ta/internal/agents/toolruntime"
	producttools "ta/internal/agents/tools"
)

// Assembly contains the three runtime surfaces derived from one effective
// catalog. SystemPrompt remains unchanged when Skills are unavailable.
type Assembly struct {
	SystemPrompt prompts.SystemPromptComposition
	Tools        []agent.ToolDefinition
	ReadAdapters []producttools.ReadAdapterBinding
}

// Build resolves one Agent's effective catalog for prompt assembly and wires
// the callable tool and skill:// reference router to the same filtered
// backend. This keeps Agent-specific visibility and overrides consistent
// across all three surfaces.
func Build(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	enabled bool,
	settings config.ResolvedAgentToolSettings,
	systemPrompt prompts.SystemPromptComposition,
) (Assembly, error) {
	return build(ctx, cfg, agentKind, enabled, settings, systemPrompt, false)
}

// BuildReferenceRoute resolves only the skill:// read route, without the
// catalog prompt or the skill tool. A purpose-scoped assistant keeps exactly
// this half of the Skills surface: every resource it may write advertises a
// reference document, and that document is the only place the write shape is
// written down. Dropping the route leaves the model with a resource it can
// describe but not use, and it goes looking for the file instead.
func BuildReferenceRoute(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	settings config.ResolvedAgentToolSettings,
) (Assembly, error) {
	return build(ctx, cfg, agentKind, true, settings, prompts.SystemPromptComposition{}, true)
}

func build(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	enabled bool,
	settings config.ResolvedAgentToolSettings,
	systemPrompt prompts.SystemPromptComposition,
	referenceRouteOnly bool,
) (Assembly, error) {
	assembly := Assembly{SystemPrompt: systemPrompt}
	if !enabled || !settings.Allows(config.AgentToolSkills) || cfg == nil {
		return assembly, nil
	}
	mode, overrides, _ := config.ResolveActiveAgentSkillPolicy(cfg, agentKind)
	backend := novaskills.NewAgentBackendWithPolicy(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		agentKind,
		overrides,
		mode == config.AgentSkillPolicyExplicit,
	)
	available, err := backend.List(ctx)
	if err != nil {
		return Assembly{}, fmt.Errorf("list available Skills for Agent %s: %w", agentKind, err)
	}
	if len(available) == 0 {
		return assembly, nil
	}

	if !referenceRouteOnly {
		entries := make([]prompts.SkillCatalogEntry, 0, len(available))
		for _, item := range available {
			entries = append(entries, prompts.SkillCatalogEntry{Name: item.Name, Description: item.Description})
		}
		assembly.SystemPrompt, err = prompts.AppendSkillsCatalogPrompt(cfg, systemPrompt, entries)
		if err != nil {
			return Assembly{}, fmt.Errorf("append Skill catalog for Agent %s: %w", agentKind, err)
		}
	}

	catalog := agenttoolruntime.NewCatalog(cfg)
	maxBytes := config.ResolveAgentContext(cfg, agentKind).MaxFragmentBytes
	referenceAdapter, err := catalog.SkillReference(backend)
	if err != nil {
		return Assembly{}, fmt.Errorf("create Skill reference adapter for Agent %s: %w", agentKind, err)
	}
	assembly.ReadAdapters = []producttools.ReadAdapterBinding{referenceAdapter}
	if referenceRouteOnly {
		return assembly, nil
	}
	canonical, err := catalog.Skill(ctx, backend, maxBytes)
	if err != nil {
		return Assembly{}, fmt.Errorf("create canonical Skill descriptor for Agent %s: %w", agentKind, err)
	}
	if canonical.Tool == nil {
		return Assembly{}, fmt.Errorf("create Skill tool for Agent %s: expected one canonical tool", agentKind)
	}
	if err := canonical.Validate(ctx); err != nil {
		return Assembly{}, fmt.Errorf("validate Skill tool for Agent %s: %w", agentKind, err)
	}
	assembly.Tools = []agent.ToolDefinition{canonical}
	return assembly, nil
}
