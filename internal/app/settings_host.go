package app

import (
	"context"
	"log/slog"
	"strings"

	"ta/config"
	agentrun "ta/internal/agents/run"
	novaskills "ta/internal/agents/skills"
	resourcecatalogapp "ta/internal/app/resourcecatalog"
	appsettings "ta/internal/app/settings"
	"ta/internal/book"
	"ta/internal/concurrency"
	projectdomain "ta/internal/project"
)

type resourceCatalogHost struct{ app *App }

func (host resourceCatalogHost) SkillDirectories(target resourcecatalogapp.SkillTarget) ([]novaskills.Directory, error) {
	if host.app == nil {
		return nil, concurrency.ErrClosed
	}
	host.app.mu.RLock()
	if host.app.cfg == nil {
		host.app.mu.RUnlock()
		return nil, ErrAgentDataDirRequired
	}
	builtinDir := host.app.cfg.SkillsDir
	dataDir := host.app.cfg.DataDir()
	registry := host.app.projectRegistry
	host.app.mu.RUnlock()

	projectID := target.ProjectID()
	if projectID == "" {
		return novaskills.NewDirectories(builtinDir, dataDir, ""), nil
	}
	if registry == nil {
		return nil, projectdomain.ErrNotFound
	}
	_, layout, err := registry.Resolve(projectID, true)
	if err != nil {
		return nil, err
	}
	return novaskills.NewDirectories(builtinDir, dataDir, layout.ContentRoot), nil
}

type settingsHost struct{ app *App }

func (host settingsHost) SettingsRuntime(target appsettings.Target) (appsettings.Runtime, error) {
	if host.app == nil {
		return appsettings.Runtime{}, concurrency.ErrClosed
	}
	host.app.mu.RLock()
	if host.app.cfg == nil {
		host.app.mu.RUnlock()
		return appsettings.Runtime{}, ErrAgentDataDirRequired
	}
	dataDir := host.app.cfg.DataDir()
	runtimeWebPort := host.app.cfg.RuntimeWebPort
	devMode := host.app.cfg.DevMode
	imagePresetToolPrompt := host.app.cfg.ImagePresetToolPrompt
	// MediaDir is resolved from the executable's own location at startup and is
	// never persisted, so a configuration rebuilt from the layers alone has no
	// way to rediscover it. Dropping it makes every imported avatar unreadable
	// for the rest of the process.
	mediaDir := host.app.cfg.MediaDir
	registry := host.app.projectRegistry
	host.app.mu.RUnlock()

	projectID := target.ProjectID()
	workspace := ""
	projectConfigPath := ""
	var state *book.State
	var layout projectdomain.Layout
	if projectID != "" {
		if registry == nil {
			return appsettings.Runtime{}, projectdomain.ErrNotFound
		}
		record, resolved, err := registry.Resolve(projectID, true)
		if err != nil {
			return appsettings.Runtime{}, err
		}
		layout = resolved
		workspace = layout.ContentRoot
		projectConfigPath = layout.ConfigPath()
		if record.Type == projectdomain.TypeBook {
			state = book.NewState(workspace)
		}
	}
	runtimeConfig, _, err := config.LoadWithProject(dataDir, workspace, projectConfigPath)
	if err != nil {
		return appsettings.Runtime{}, err
	}
	runtimeConfig.RuntimeWebPort = runtimeWebPort
	runtimeConfig.DevMode = devMode
	runtimeConfig.ImagePresetToolPrompt = imagePresetToolPrompt
	runtimeConfig.MediaDir = mediaDir
	runtimeConfig.ProjectID = projectID
	runtimeConfig.ProjectStoreDir = layout.StoreRoot
	runtimeConfig.Workspace = workspace
	return appsettings.Runtime{
		Config:            *runtimeConfig,
		ProjectID:         projectID,
		Workspace:         workspace,
		ProjectConfigPath: projectConfigPath,
		BookState:         state,
	}, nil
}

func (host settingsHost) ApplySettings(_ config.LayeredSettings, layer config.SettingsLayer) {
	if host.app == nil {
		return
	}
	host.app.mu.RLock()
	projectID := ""
	if host.app.cfg != nil {
		projectID = strings.TrimSpace(host.app.cfg.ProjectID)
	}
	host.app.mu.RUnlock()
	target := appsettings.Global()
	if projectID != "" {
		target = appsettings.Project(projectID)
	}
	runtime, err := host.SettingsRuntime(target)
	if err != nil {
		slog.ErrorContext(context.Background(), "[internal/app/settings_host.go] reload foreground settings after persistence failed",
			"project_id", projectID,
			"error", err,
		)
		return
	}

	host.app.mu.Lock()
	currentProjectID := ""
	if host.app.cfg != nil {
		currentProjectID = strings.TrimSpace(host.app.cfg.ProjectID)
	}
	if currentProjectID != projectID || host.app.cfg == nil {
		host.app.mu.Unlock()
		return
	}
	*host.app.cfg = runtime.Config
	syncRuntimeDiagnostics(host.app.cfg)
	versionService := host.app.versionService
	autoSettings := versionAutoSettingsForConfig(host.app.cfg)
	host.app.mu.Unlock()

	if layer == config.SettingsLayerUser && versionService != nil {
		versionService.ConfigureAutoVersion(autoSettings)
	}
	if layer == config.SettingsLayerUser {
		if err := host.app.ProjectFiles().ScheduleAutoVersion(projectdomain.AgentsProjectID); err != nil {
			slog.ErrorContext(context.Background(), "[internal/app/settings_host.go] schedule Agents Project version after user Agent settings mutation failed",
				"project_id", projectdomain.AgentsProjectID,
				"error", err,
			)
		}
	}
}

func syncRuntimeDiagnostics(cfg *config.Config) {
	if cfg == nil {
		agentrun.SetModelInputLoggingEnabled(false)
		agentrun.SetTraceContentCaptureEnabled(false)
		return
	}
	agentrun.SetModelInputLoggingEnabled(cfg.DevMode && cfg.LLMInputLogEnabled)
	agentrun.SetTraceRuntimeConfig(cfg.TraceCaptureLevel, cfg.TraceExporter, cfg.TraceRetentionRuns)
	agentrun.SetTraceContentCaptureEnabled(cfg.Labs.DeveloperMode)
}

type modelHost struct{ app *App }

func (host modelHost) ModelConfigSnapshot() config.Config {
	if host.app == nil {
		return config.Config{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	if host.app.cfg == nil {
		return config.Config{}
	}
	snapshot := *host.app.cfg
	snapshot.ModelEndpoints = cloneModelEndpoints(host.app.cfg.ModelEndpoints)
	snapshot.ModelProfiles = cloneModelProfiles(host.app.cfg.ModelProfiles)
	return snapshot
}

func cloneModelEndpoints(endpoints []config.ModelEndpointSettings) []config.ModelEndpointSettings {
	if endpoints == nil {
		return nil
	}
	cloned := make([]config.ModelEndpointSettings, len(endpoints))
	for index, endpoint := range endpoints {
		cloned[index] = endpoint
		if endpoint.Headers != nil {
			cloned[index].Headers = make(map[string]string, len(endpoint.Headers))
			for name, value := range endpoint.Headers {
				cloned[index].Headers[name] = value
			}
		}
		if endpoint.ProtocolOptions != nil {
			cloned[index].ProtocolOptions = cloneModelProtocolOptions(endpoint.ProtocolOptions)
		}
	}
	return cloned
}

func cloneModelProfiles(profiles []config.ModelProfileSettings) []config.ModelProfileSettings {
	if profiles == nil {
		return nil
	}
	cloned := make([]config.ModelProfileSettings, len(profiles))
	for index, profile := range profiles {
		cloned[index] = profile
		if profile.Headers != nil {
			cloned[index].Headers = make(map[string]string, len(profile.Headers))
			for name, value := range profile.Headers {
				cloned[index].Headers[name] = value
			}
		}
		if profile.ProtocolOptions != nil {
			cloned[index].ProtocolOptions = cloneModelProtocolOptions(profile.ProtocolOptions)
		}
	}
	return cloned
}

func cloneModelProtocolOptions(options map[string]any) map[string]any {
	cloned := make(map[string]any, len(options))
	for key, value := range options {
		cloned[key] = cloneModelProtocolValue(value)
	}
	return cloned
}

func cloneModelProtocolValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneModelProtocolOptions(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneModelProtocolValue(item)
		}
		return cloned
	default:
		return typed
	}
}
