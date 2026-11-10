package imageapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"
	"ta/config"
	appsettings "ta/internal/app/settings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// DefinitionArtAgentAvailable is conservative: auto-generation needs an
// explicit Image Agent model/selection, a usable image provider and the
// existing permission capability. It never invents configuration or secrets.
func DefinitionArtAgentAvailable(cfg config.Config) bool {
	explicit := strings.TrimSpace(cfg.DefaultImageAgentID) != "" || strings.TrimSpace(cfg.AgentModels.Image.ProfileID) != ""
	if !explicit {
		return false
	}
	if err := config.ApplyCustomAgent(&cfg, config.AgentKindImage, cfg.DefaultImageAgentID); err != nil {
		return false
	}
	selectedModel := strings.TrimSpace(cfg.AgentModels.Image.ProfileID)
	if selectedModel == "" {
		selectedModel = strings.TrimSpace(cfg.AgentModels.Default.ProfileID)
	}
	if selectedModel == "" {
		selectedModel = "default"
	}
	modelExists := selectedModel == "default" && strings.TrimSpace(cfg.OpenAIModel) != ""
	for _, profile := range cfg.ModelProfiles {
		if profile.ID == selectedModel && strings.TrimSpace(profile.Model) != "" {
			modelExists = true
		}
	}
	if !modelExists {
		return false
	}
	selectedImage := strings.TrimSpace(cfg.DefaultImageAPIProfileID)
	if selectedImage == "" {
		return false
	}
	imageExists := false
	for _, profile := range cfg.ImageAPIProfiles {
		if profile.ID == selectedImage && (strings.TrimSpace(profile.Model) != "" || profile.ComfyUI != nil && strings.TrimSpace(profile.ComfyUI.Workflow) != "") {
			imageExists = true
		}
	}
	if !imageExists {
		return false
	}
	if !config.ResolveAgentTools(&cfg, config.AgentKindImage).Allows(config.AgentToolImageGeneration) {
		return false
	}
	if _, err := config.ResolveImageAPIProfile(&cfg, ""); err != nil {
		return false
	}
	model := config.ResolveAgentModel(&cfg, config.AgentKindImage)
	return model.ProfileID == selectedModel && strings.TrimSpace(model.Model) != "" && strings.TrimSpace(model.BaseURL) != ""
}

// GenerateDefinitionArt executes one already registered job through the normal
// Image Agent. Callers detach it using the registered product Task lifecycle.
// A persisted claim precedes admission; failure leaves an empty picture and no
// automatic retry, even when a provider may have charged before disconnecting.
func (service *Service) GenerateDefinitionArt(ctx context.Context, workspace, storyID, branchID, turnID string, job interactive.DefinitionArtJob) error {
	runtime, err := service.AcquireRuntime(ctx, workspace)
	if err != nil {
		return err
	}
	defer runtime.Release()
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(runtime.Config.DataDir(), runtime.Workspace, config.ProjectConfigPath(runtime.Config.ProjectStoreDir)); loadErr == nil {
		appsettings.ApplyLayered(&runtime.Config, layered)
	} else {
		slog.WarnContext(runtime.Context(), "[definition-art] optional settings unavailable", "error", loadErr)
		return nil
	}
	if runtime.Interactive == nil || !DefinitionArtAgentAvailable(runtime.Config) {
		return nil
	}
	claimed, err := runtime.Interactive.TransitionDefinitionArt(storyID, branchID, turnID, job, "", "queued", nil)
	if err != nil || !claimed {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_, _ = runtime.Interactive.TransitionDefinitionArt(storyID, branchID, turnID, job, "queued", "failed", nil)
			_, _ = runtime.Interactive.TransitionDefinitionArt(storyID, branchID, turnID, job, "executing", "failed", nil)
			panic(recovered) // The registered Task owns the recover/log boundary.
		}
	}()
	req := AgentGenerateRequest{CommandID: fmt.Sprintf("definition-art-%x", sha256.Sum256([]byte(storyID+"\x00"+job.Key()))), CustomAgentID: runtime.Config.DefaultImageAgentID, Purpose: "definition_art", StoryID: storyID, BranchID: branchID, TurnID: turnID, ImagePresetID: job.Style, DefinitionArtJob: &job, SourceContext: fmt.Sprintf("为已经登记的%s绘制一张独立插图。名称：%s\n描述：%s", job.Category, job.Name, job.Description), SystemPrompt: "你是游戏物品插图助手。只调用一次 generate_image，purpose 使用 general，n 为 1。只画内容本身，不画界面、文字或品级光效，不修改任何游戏事实。失败后直接结束，不重试。"}
	result, runErr := service.generateWithAgent(runtime, req)
	err = runErr
	if err == nil {
		refs, readErr := runtime.Interactive.DefinitionArtReferences(storyID, branchID)
		if readErr != nil {
			err = readErr
		} else if _, ok := refs[fmt.Sprintf("%s:%d", job.DefinitionID, job.Version)]; !ok {
			err = result.MissingImageError()
		}
	}
	// Success is saved by the guarded tool before its result leaves the process.
	if err != nil {
		_, _ = runtime.Interactive.TransitionDefinitionArt(storyID, branchID, turnID, job, "queued", "failed", nil)
		_, _ = runtime.Interactive.TransitionDefinitionArt(storyID, branchID, turnID, job, "executing", "failed", nil)
		slog.WarnContext(runtime.Context(), "[definition-art] optional image failed; no automatic retry", "definition_id", job.DefinitionID, "error", err)
	}
	return err
}

// definitionArtGuard preserves ordinary tool permission checks and adds a
// durable at-most-once fence. Restore after an uncertain provider call refuses
// another charge. Only generic single-image output is accepted; lore/cover/
// turn mutations and extra tools cannot be smuggled through this art request.
type definitionArtGuard struct {
	agent.BaseMiddleware
	store                                *interactive.Store
	storyID, branchID, turnID, projectID string
	job                                  interactive.DefinitionArtJob
}

func (g *definitionArtGuard) WrapToolCall(_ context.Context, next agent.ToolCallEndpoint, tool *agent.ToolContext) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, args string, options ...agent.ToolOption) (agent.ToolResult, error) {
		if tool.Name == "skill" {
			return next(ctx, args, options...)
		}
		if tool.Name != "generate_image" {
			return agent.ToolResult{}, fmt.Errorf("definition art permits only one image tool")
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return agent.ToolResult{}, err
		}
		if purpose, _ := input["purpose"].(string); purpose != "" && purpose != "general" {
			return agent.ToolResult{}, fmt.Errorf("definition art requires general image purpose")
		}
		if n, ok := input["n"].(float64); ok && n != 0 && n != 1 {
			return agent.ToolResult{}, fmt.Errorf("definition art requires one image")
		}
		input["n"] = 1
		encoded, _ := json.Marshal(input)
		claimed, err := g.store.TransitionDefinitionArt(g.storyID, g.branchID, g.turnID, g.job, "queued", "executing", nil)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if !claimed {
			return agent.ToolResult{}, fmt.Errorf("definition art was already attempted; automatic retry refused")
		}
		result, callErr := next(ctx, string(encoded), options...)
		var payload struct {
			Schema string `json:"schema"`
			Images []struct {
				Path string `json:"path"`
			} `json:"images"`
		}
		if callErr == nil && json.Unmarshal([]byte(result.ModelContent), &payload) == nil && payload.Schema == "generated_image.v1" && len(payload.Images) == 1 && cultivation.SafeDefinitionImagePath(payload.Images[0].Path) {
			art := &cultivation.DefinitionArt{Source: "generated", ImagePath: payload.Images[0].Path, ProjectID: g.projectID}
			saved, saveErr := g.store.TransitionDefinitionArt(g.storyID, g.branchID, g.turnID, g.job, "executing", "succeeded", art)
			if saveErr != nil {
				return result, saveErr
			}
			if !saved {
				return result, fmt.Errorf("definition art result could not settle")
			}
		} else {
			_, _ = g.store.TransitionDefinitionArt(g.storyID, g.branchID, g.turnID, g.job, "executing", "failed", nil)
			if callErr == nil {
				callErr = fmt.Errorf("definition art tool returned no safe single image")
			}
		}
		return result, callErr
	}, nil
}
