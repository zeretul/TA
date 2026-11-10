package tools

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	booklore "ta/internal/book/lore"
	"ta/internal/interactive"

	agent "ta/agent"
	agenttools "ta/agent/tools"

	"ta/config"
	"ta/internal/agents/configresource"
	novaskills "ta/internal/agents/skills"
	"ta/internal/webaccess"
	workspacechange "ta/internal/workspace/change"
)

// Catalog is the only construction boundary for Text Adventure's concrete tools. It
// selects executable definitions from resolved capabilities; implementations
// remain in their cohesive modules and host differences remain in Adapters.
type Catalog struct {
	cfg                *config.Config
	workspaceMetadata  WorkspaceMetadataProvider
	runtimeExecutables RuntimeExecutables
}

// RuntimeExecutables are host-owned dependencies discovered by the
// application rather than guessed by reusable Agent modules.
type RuntimeExecutables struct {
	Ripgrep      string
	Bash         string
	Pwsh         string
	ShellRuntime func() (ShellRuntime, error)
}

// ShellRuntime is resolved lazily because most Agent configurations do not
// enable an arbitrary shell. Environment capture may execute a user's login
// shell, so it must remain behind the exact capability boundary.
type ShellRuntime struct {
	Bash        string
	Pwsh        string
	Environment []string
}

func NewCatalog(cfg *config.Config, workspaceMetadata WorkspaceMetadataProvider, runtimeExecutables RuntimeExecutables) *Catalog {
	return &Catalog{cfg: cfg, workspaceMetadata: workspaceMetadata, runtimeExecutables: runtimeExecutables}
}

type Factory func(config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error)

func catalogToolResultMaxBytes(cfg *config.Config) int {
	if cfg == nil || cfg.AgentToolResultLimitKB <= 0 {
		return defaultToolResultMaxBytes
	}
	return cfg.AgentToolResultLimitKB * 1024
}

func (catalog *Catalog) Lore(forceReadOnly bool) Factory {
	return loreToolsFactory(catalog.cfg, forceReadOnly)
}

func (catalog *Catalog) IDE() Factory { return ideToolsFactory(catalog.cfg) }

func (catalog *Catalog) Image() Factory { return imageToolsFactory(catalog.cfg) }

func (catalog *Catalog) InteractiveStory(toolContext InteractiveContext) Factory {
	return interactiveStoryToolsFactory(catalog.cfg, toolContext)
}

func (catalog *Catalog) Configuration() Factory { return configurationToolsFactory(catalog.cfg) }

// ScopedConfiguration limits the configuration registry to these resource
// names without adding any other product tool. It narrows a General Agent that
// was opened for one purpose; an empty list keeps every resource.
//
// A scoped surface never regains the shared libraries' write operations, even
// when the caller supplied no target: "scoped" is what makes those libraries
// read-only templates, and the target only decides which story and world are
// writable. A missing target therefore narrows further, never less.
func (catalog *Catalog) ScopedConfiguration(scope IDEContext) Factory {
	return scopedConfigurationToolsFactory(catalog.cfg, scope.ConfigResources, scopedTarget(scope))
}

// ScopedIDE binds the Writing Agent's product tools to one editable world and
// one allowed set of configuration resources. It is the Game Settings
// assistant's construction: the same implementations, narrowed by purpose.
func (catalog *Catalog) ScopedIDE(scope IDEContext) Factory {
	scope.SettingsTarget = scopedTarget(scope)
	return scopedIDEToolsFactory(catalog.cfg, scope)
}

// scopedTarget is the target every scoped surface carries. An absent target
// becomes an empty one rather than nil, because nil is the editor's in-place
// preset editing: a purpose-scoped conversation with nothing bound has no
// writable configuration at all, which is the refusal to fall back to the
// shared write scope.
func scopedTarget(scope IDEContext) *configresource.ScopedTarget {
	if scope.SettingsTarget != nil {
		return scope.SettingsTarget
	}
	return &configresource.ScopedTarget{}
}

func (catalog *Catalog) Workspace(settings config.ResolvedAgentToolSettings, readAdapters ...ReadAdapterBinding) ([]agent.ToolDefinition, error) {
	workspace := ""
	var cfg *config.Config
	var metadata WorkspaceMetadataProvider
	var executables RuntimeExecutables
	if catalog != nil {
		cfg = catalog.cfg
		metadata = catalog.workspaceMetadata
		executables = catalog.runtimeExecutables
	}
	if cfg != nil {
		workspace = cfg.Workspace
	}
	return workspaceToolsFactory(
		workspace,
		func() string {
			if cfg == nil {
				return ""
			}
			return cfg.ProjectStoreDir
		}(),
		metadata,
		executables,
		catalogToolResultMaxBytes(cfg),
		readAdapters...,
	)(settings)
}

func (catalog *Catalog) WebAccess(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
	searchEnabled := settings.Allows(config.AgentToolWebSearch)
	fetchEnabled := settings.Allows(config.AgentToolWebFetch)
	if !searchEnabled && !fetchEnabled {
		return nil, nil
	}
	clientConfig := resolveWebAccessClientConfig(catalog.cfg)
	probe, err := webaccess.New(clientConfig)
	if err != nil {
		return nil, err
	}
	if err := probe.Close(context.Background()); err != nil {
		return nil, fmt.Errorf("close web access validation client: %w", err)
	}
	client, err := newInvocationWebAccessClient(func() (managedWebAccessClient, error) {
		return webaccess.New(clientConfig)
	})
	if err != nil {
		return nil, err
	}
	definitions := make([]agent.ToolDefinition, 0, 2)
	if searchEnabled {
		definition, err := newWebSearchTool(client, config.AgentToolWebSearch)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if fetchEnabled {
		definition, err := newWebFetchTool(client, config.AgentToolWebFetch)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// Browser registers the isolated named-tab tool only when both policy and an
// installed local browser runtime are available. Unavailable hosts expose no
// dead model endpoint.
func (catalog *Catalog) Browser(ctx context.Context, settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
	if !settings.Allows(config.AgentToolBrowser) {
		return nil, nil
	}
	definition, available, err := newRuntimeBrowserTool(ctx)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, nil
	}
	var cfg *config.Config
	if catalog != nil {
		cfg = catalog.cfg
	}
	definition.Descriptor.MaxResultBytes = catalogToolResultMaxBytes(cfg)
	if err := definition.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate browser definition: %w", err)
	}
	return []agent.ToolDefinition{definition}, nil
}

func (catalog *Catalog) Skill(ctx context.Context, backend *novaskills.Backend, maxBytes int) (agent.ToolDefinition, error) {
	return newSkillTool(ctx, backend, maxBytes)
}

func (catalog *Catalog) SkillReference(backend *novaskills.Backend) (ReadAdapterBinding, error) {
	adapter, err := newSkillReferenceReadAdapter(backend)
	if err != nil {
		return ReadAdapterBinding{}, err
	}
	return newReadAdapterBinding(config.AgentToolSkills, adapter)
}

func loreToolsFactory(cfg *config.Config, forceReadOnly bool) Factory {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		readEnabled := settings.Allows(config.AgentToolLoreRead)
		writeEnabled := settings.Allows(config.AgentToolLoreWrite)
		if cfg == nil || (!readEnabled && !writeEnabled) {
			return nil, nil
		}
		definitions, err := newLoreTools(cfg.Workspace, !forceReadOnly && writeEnabled)
		if err != nil {
			return nil, err
		}
		return enabledDefinitions(settings, definitions), nil
	}
}

func ideToolsFactory(cfg *config.Config) Factory {
	return scopedIDEToolsFactory(cfg, IDEContext{})
}

// scopedIDEToolsFactory is the Writing Agent tool set with an optional world
// binding and configuration-resource subset. The unscoped call keeps the
// editor behaviour that sees every world and every configuration resource.
func scopedIDEToolsFactory(cfg *config.Config, scope IDEContext) Factory {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		if cfg == nil {
			return nil, nil
		}
		var definitions []agent.ToolDefinition
		if settings.Allows(config.AgentToolLoreRead) || settings.Allows(config.AgentToolLoreWrite) {
			lore, err := newLoreTools(cfg.Workspace, settings.Allows(config.AgentToolLoreWrite), loreToolsOptions{Scope: scope.LoreScope})
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, enabledDefinitions(settings, lore)...)
		}
		if settings.Allows(config.AgentToolImageGeneration) {
			images, err := newIllustrationTools(cfg)
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, enabledDefinitions(settings, images)...)
		}
		configured, err := scopedConfigurationToolsFactory(cfg, scope.ConfigResources, scope.SettingsTarget)(settings)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, configured...)
		return definitions, nil
	}
}

func imageToolsFactory(cfg *config.Config) Factory {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		if cfg == nil || !settings.Allows(config.AgentToolImageGeneration) {
			return nil, nil
		}
		definitions, err := newIllustrationTools(cfg)
		if err != nil {
			return nil, err
		}
		return enabledDefinitions(settings, definitions), nil
	}
}

func interactiveStoryToolsFactory(cfg *config.Config, toolContexts ...InteractiveContext) Factory {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		var definitions []agent.ToolDefinition
		// The story's own world scope binds its lore tools, so the narrator can
		// only read and write this world's book plus the user's shared entries.
		var loreScope *string
		var observeLoreRead func([]string)
		if len(toolContexts) > 0 {
			observeLoreRead = toolContexts[0].ObserveLoreRead
			scope := toolContexts[0].LoreScope
			loreScope = &scope
		}
		if cfg != nil && settings.Allows(config.AgentToolLoreRead) {
			lore, err := newLoreTools(cfg.Workspace, false, loreToolsOptions{Scope: loreScope, ReadPolicy: &loreReadPolicy{OnRead: observeLoreRead}, Visibility: func() func(booklore.Item) bool {
				if len(toolContexts) == 0 {
					return nil
				}
				ctx := toolContexts[0]
				if ctx.Store == nil {
					return func(booklore.Item) bool { return false }
				}
				story, err := ctx.Store.StoryContext(ctx.StoryID, ctx.BranchID)
				if err != nil {
					return func(booklore.Item) bool { return false }
				}
				return interactive.StoryLoreVisibility(story.Meta, story.Snapshot.State)
			}})
			if err != nil {
				return nil, err
			}
			definitions = append(definitions, enabledDefinitions(settings, lore)...)
		}
		if len(toolContexts) == 0 {
			return definitions, nil
		}
		toolContext := toolContexts[0]
		if toolContext.MaxResultBytes <= 0 {
			toolContext.MaxResultBytes = catalogToolResultMaxBytes(cfg)
		}
		history, err := newInteractiveHistoryTools(toolContext)
		if err != nil {
			return nil, err
		}
		stateSchema, err := newInteractiveOpeningStateSchemaTools(toolContext)
		if err != nil {
			return nil, err
		}
		protagonist, err := newInteractiveProtagonistTools(toolContext)
		if err != nil {
			return nil, err
		}
		turn, err := newInteractiveTurnTools(toolContext)
		if err != nil {
			return nil, err
		}
		events, err := newInteractiveEventTools(toolContext)
		if err != nil {
			return nil, err
		}
		trade, err := newInteractiveTradeTools(toolContext)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, history...)
		definitions = append(definitions, stateSchema...)
		definitions = append(definitions, protagonist...)
		definitions = append(definitions, events...)
		definitions = append(definitions, trade...)
		definitions = append(definitions, turn...)
		return definitions, nil
	}
}

func configurationToolsFactory(cfg *config.Config) Factory {
	return scopedConfigurationToolsFactory(cfg, nil, nil)
}

func scopedConfigurationToolsFactory(cfg *config.Config, resources []string, target *configresource.ScopedTarget) Factory {
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		if cfg == nil || (!settings.Allows(config.AgentToolConfigRead) && !settings.Allows(config.AgentToolConfigApply)) {
			return nil, nil
		}
		definitions, err := configresource.NewScopedTools(cfg, catalogToolResultMaxBytes(cfg), resources, target)
		if err != nil {
			return nil, err
		}
		return enabledDefinitions(settings, definitions), nil
	}
}

// workspaceToolsFactory registers only executable tools. Disabled definitions
// are never built, so a missing shell or mutation dependency cannot leak a
// nil endpoint into the model-visible registry.
func workspaceToolsFactory(workspace, projectStoreRoot string, metadata WorkspaceMetadataProvider, executables RuntimeExecutables, maxResultBytes int, extraReadAdapters ...ReadAdapterBinding) Factory {
	if maxResultBytes <= 0 {
		maxResultBytes = defaultToolResultMaxBytes
	}
	return func(settings config.ResolvedAgentToolSettings) ([]agent.ToolDefinition, error) {
		readEnabled := settings.Allows(config.AgentToolFilesystemRead)
		writeEnabled := settings.Allows(config.AgentToolWorkspaceWrite)
		shellEnabled := settings.Allows(config.AgentToolShell)
		enabledReadAdapters, err := enabledReadAdapterBindings(settings, extraReadAdapters)
		if err != nil {
			return nil, err
		}
		if !readEnabled && !writeEnabled && !shellEnabled && len(enabledReadAdapters) == 0 {
			return nil, nil
		}
		var backend *agenttools.LocalWorkspace
		if readEnabled || writeEnabled || shellEnabled {
			backend, err = agenttools.OpenWorkspaceWithOptions(agenttools.WorkspaceOptions{
				Root: workspace, RipgrepExecutable: executables.Ripgrep,
				Limits: agenttools.WorkspaceLimits{MaxResultBytes: maxResultBytes},
			})
			if err != nil {
				return nil, fmt.Errorf("create workspace backend: %w", err)
			}
		}
		definitions := make([]agent.ToolDefinition, 0, 7)
		readAdapters := make([]agenttools.ReadAdapter, 0, len(enabledReadAdapters)+2)
		// ToolDescriptor has one capability field. Bindings have already removed
		// unauthorized adapters, so keep filesystem_read as the combined endpoint's
		// primary receipt and otherwise use the first enabled URI capability.
		readCapability := ""
		if readEnabled {
			textAdapter, err := agenttools.LocalFileAdapter(backend)
			if err != nil {
				return nil, fmt.Errorf("create local text read adapter: %w", err)
			}
			directoryAdapter, err := agenttools.DirectoryAdapter(backend)
			if err != nil {
				return nil, fmt.Errorf("create directory read adapter: %w", err)
			}
			readAdapters = append(readAdapters, textAdapter, directoryAdapter)
			readCapability = config.AgentToolFilesystemRead
			options := []agenttools.DefinitionOption{
				agenttools.WithCapability(config.AgentToolFilesystemRead),
				agenttools.WithMaxResultBytes(maxResultBytes),
			}
			globDefinition, err := agenttools.Glob(backend, options...)
			if err != nil {
				return nil, fmt.Errorf("create glob tool: %w", err)
			}
			grepDefinition, err := agenttools.Grep(backend, options...)
			if err != nil {
				return nil, fmt.Errorf("create grep tool: %w", err)
			}
			definitions = append(definitions, globDefinition, grepDefinition)
		}
		for _, binding := range enabledReadAdapters {
			readAdapters = append(readAdapters, binding.adapter)
			if readCapability == "" {
				readCapability = binding.capability
			}
		}
		if len(readAdapters) > 0 {
			options := []agenttools.DefinitionOption{
				agenttools.WithCapability(readCapability),
				agenttools.WithMaxResultBytes(maxResultBytes),
			}
			if readCapability == config.AgentToolTrajectory {
				options = append(options, agenttools.WithPresentation(agent.ToolPresentationFile))
			}
			readDefinition, err := agenttools.Read(readAdapters, options...)
			if err != nil {
				return nil, fmt.Errorf("create read tool: %w", err)
			}
			definitions = append([]agent.ToolDefinition{readDefinition}, definitions...)
		}
		if writeEnabled {
			var changes *workspacechange.Service
			if strings.TrimSpace(projectStoreRoot) != "" {
				changes, err = workspacechange.ForWorkspaceAt(backend.Root(), projectStoreRoot)
			} else {
				changes, err = workspacechange.ForWorkspace(backend.Root())
			}
			if err != nil {
				return nil, fmt.Errorf("create workspace change service: %w", err)
			}
			adapter, err := newWorkspaceMutationAdapter(changes, metadata)
			if err != nil {
				return nil, fmt.Errorf("create workspace mutation adapter: %w", err)
			}
			options := []agenttools.DefinitionOption{
				agenttools.WithCapability(config.AgentToolWorkspaceWrite),
				agenttools.WithMaxResultBytes(maxResultBytes),
			}
			writeDefinition, err := agenttools.Write(adapter, options...)
			if err != nil {
				return nil, fmt.Errorf("create write tool: %w", err)
			}
			editDefinition, err := agenttools.Edit(adapter, options...)
			if err != nil {
				return nil, fmt.Errorf("create edit tool: %w", err)
			}
			definitions = append(definitions, writeDefinition, editDefinition)
		}
		if shellEnabled {
			shellRuntime := ShellRuntime{
				Bash: executables.Bash, Pwsh: executables.Pwsh,
			}
			if executables.ShellRuntime != nil {
				shellRuntime, err = executables.ShellRuntime()
				if err != nil {
					return nil, fmt.Errorf("prepare agent shell environment: %w", err)
				}
			}
			shellKind := agenttools.ShellBash
			executable := shellRuntime.Bash
			constructor := agenttools.Bash
			if runtime.GOOS == "windows" {
				shellKind = agenttools.ShellPwsh
				executable = shellRuntime.Pwsh
				constructor = agenttools.Pwsh
			}
			runner, err := newAgentCommandRunner(backend, shellKind, executable, shellRuntime.Environment, projectStoreRoot)
			if err != nil {
				return nil, fmt.Errorf("create %s runner: %w", shellKind, err)
			}
			definition, err := constructor(
				runner,
				agenttools.WithCapability(config.AgentToolShell),
				agenttools.WithMaxResultBytes(maxResultBytes),
			)
			if err != nil {
				return nil, fmt.Errorf("create %s tool: %w", shellKind, err)
			}
			definitions = append(definitions, definition)
		}
		return definitions, nil
	}
}

func enabledDefinitions(settings config.ResolvedAgentToolSettings, definitions []agent.ToolDefinition) []agent.ToolDefinition {
	result := make([]agent.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		capability := strings.TrimSpace(definition.Descriptor.Capability)
		if capability == "" || settings.Allows(capability) {
			result = append(result, definition)
		}
	}
	return result
}
