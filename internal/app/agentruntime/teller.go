package agentruntime

import (
	"ta/config"
	"ta/internal/agents/prompts"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/interactive/teller"
	"ta/internal/style"
)

func WritingTellerForConfig(cfg *config.Config) prompts.IDEStoryTeller {
	if cfg == nil || cfg.DataDir() == "" {
		return prompts.IDEStoryTeller{}
	}
	tellerID := cfg.IDEStoryTellerID
	if tellerID == "" {
		tellerID = style.DefaultID
	}
	return WritingTeller(interactiveapp.LoadWritingTeller(cfg.DataDir(), tellerID), nil)
}

func WritingTeller(definition teller.Definition, styleRules []prompts.StyleRule) prompts.IDEStoryTeller {
	if definition.ID == "" {
		return prompts.IDEStoryTeller{}
	}
	return prompts.IDEStoryTeller{
		ID: definition.ID, Name: definition.Name, Description: definition.Description,
		Prompt: definition.PromptForTargets("system", "turn_context"), StyleRules: styleRules,
	}
}
