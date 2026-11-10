package prompts

import (
	"strings"

	"ta/config"
	agentcontext "ta/internal/agents/context"
)

func LoreClassificationSystemInstruction() string {
	return strings.Join([]string{
		"Classify Text Adventure lore items.",
		"Output only a JSON object: {\"items\":[{\"id\":\"input id\",\"type\":\"character|world|location|faction|rule|item|other\",\"confidence\":\"high|medium|low\",\"reason\":\"brief rationale\"}]}.",
		"Names take precedence over bodies. Names clearly denoting character details or profiles classify as character; apply the same principle to explicit location, faction, rule, and item names.",
		"Use world only for worldbuilding, history, culture, or era background spanning locations. When classification is uncertain, use other with low confidence instead of guessing.",
		"Return each input id at most once, never invent an id absent from the input, and do not output Markdown.",
	}, "\n")
}

func ContextCompactionSystemInstruction() string {
	base := staticPromptAsset(contextCompactionWorkflowAsset)
	return base + "\n\nUse the following single stable Markdown checkpoint schema for every source kind. Sections that do not apply may be empty, but do not rename them or create an alternative format:\n" +
		agentcontext.CompactionCheckpointSchema()
}

// ComposeContextCompactionInstruction admits the internal checkpoint protocol
// against the source Agent's context budgets. It deliberately excludes
// user-configurable Agent prompts because compaction is a side fork of the
// source request, not an independently configurable Agent identity.
func ComposeContextCompactionInstruction(cfg *config.Config, sourceAgentKind string) (SystemPromptComposition, error) {
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	return composeSystemPrompt(cfg, sourceAgentKind, "context_compaction", workspace, []SystemPromptFragment{{
		ID:       "context_checkpoint_protocol",
		Source:   "internal runtime",
		Title:    "Context checkpoint maintenance protocol",
		Purpose:  "compile a bounded incremental checkpoint for the source Agent request",
		Content:  ContextCompactionSystemInstruction(),
		Prefix:   "# Internal Context Checkpoint Protocol\n\n",
		Required: true,
		Overflow: SystemPromptOverflowReject,
	}})
}
