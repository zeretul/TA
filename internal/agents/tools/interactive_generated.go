package tools

import (
	"context"
	"encoding/json"

	agent "ta/agent"
	"ta/internal/interactive"
)

func newInteractiveGeneratedDefinitionTool(ctx InteractiveContext) (agent.ToolDefinition, error) {
	tool, err := agent.InferTool("search_generated_definitions", "Read this branch's registered generated skills, equipment, techniques or recipes by exact ID, name substring, category, or bounded cursor pages. Use this when an older definition is absent from the small runtime working set. Results include validated real mechanics and costs, but registration does not grant mastery or inventory. This tool is read-only.", func(_ context.Context, input interactive.GeneratedDefinitionSearchRequest) (string, error) {
		input.MaxBytes = ctx.MaxResultBytes
		result, err := ctx.Store.SearchGeneratedDefinitions(ctx.StoryID, ctx.BranchID, input)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(result)
		return string(data), err
	})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	descriptor := boundedReadDescriptor(ToolSourceHistory, "", agent.ToolResultRecoveryRerun)
	if ctx.MaxResultBytes > 0 {
		descriptor.MaxResultBytes = ctx.MaxResultBytes
	}
	return defineTool(tool, descriptor)
}
