package interactiveapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ta/internal/agents/prompts"
	"ta/internal/interactive"
	"ta/internal/interactive/teller"
	"ta/internal/style"
)

func (c *Conversation) teller(tellerID string) teller.Definition {
	return LoadGameTeller(c.novaDir, tellerID)
}

// StoryStyleFragments is the expression configuration one generation cycle
// actually applies, already resolved to lean fragments.
//
// It exists so the three purposes are assembled once and read from one place:
// the main narrative request takes MainSystem, MainTurn, Dialogue and Combat;
// a character window takes Dialogue alone. Nothing here carries world facts or
// character knowledge — those are injected by the caller that owns them — so
// the same fragment can be applied to a purpose without dragging a second copy
// of the story's rules into the request.
type StoryStyleFragments struct {
	// MainSystem and MainTurn are the base narrative voice and its per-turn
	// movement.
	MainSystem string
	MainTurn   string
	// Dialogue is the fragment applied to character speech, whether that speech
	// happens inside the main narrative or in a standalone character window.
	Dialogue string
	// Combat overrides the base voice while an already-settled exchange is being
	// described. It never decides the exchange itself.
	Combat string
	// MainStyleID, DialogueStyleID and CombatStyleID are the resolved style ids.
	// They are recorded so a generation can say which style produced it, and so
	// the dialogue and combat purposes can be seen to inherit the main narrative
	// rather than silently falling back.
	MainStyleID     string
	DialogueStyleID string
	CombatStyleID   string
}

// ResolveStoryStyleFragments resolves a story's three purposes into fragments.
//
// A purpose that names no style of its own follows the main narrative, so the
// dialogue and combat fragments are the main style's own fragments in that case.
// The four pre-migration styles carry no dialogue or combat fragment, so a story
// left on one of them yields empty dialogue and combat text; the caller applies
// nothing rather than a wrong voice, which is the same outcome as inheriting.
func ResolveStoryStyleFragments(novaDir string, meta interactive.StoryMeta) StoryStyleFragments {
	purposes := meta.StylePurposes
	mainID := purposes.ResolvedStyleID(interactive.PurposeMain, meta.StoryTellerID)
	dialogueID := purposes.ResolvedStyleID(interactive.PurposeDialogue, meta.StoryTellerID)
	combatID := purposes.ResolvedStyleID(interactive.PurposeCombat, meta.StoryTellerID)

	main := loadInteractiveTeller(novaDir, mainID)
	fragments := StoryStyleFragments{
		MainSystem:      main.PromptForTargets(teller.TargetSystem),
		MainTurn:        main.PromptForTargets(teller.TargetTurnContext),
		MainStyleID:     mainID,
		DialogueStyleID: dialogueID,
		CombatStyleID:   combatID,
	}
	// Load each purpose's own style only when it differs from the main one. An
	// inheriting purpose is the common case, and re-reading the same preset would
	// only produce the same fragments.
	if dialogueID == mainID {
		fragments.Dialogue = main.PromptForTargets(teller.TargetDialogue)
	} else {
		fragments.Dialogue = loadInteractiveTeller(novaDir, dialogueID).PromptForTargets(teller.TargetDialogue)
	}
	if combatID == mainID {
		fragments.Combat = main.PromptForTargets(teller.TargetCombat)
	} else {
		fragments.Combat = loadInteractiveTeller(novaDir, combatID).PromptForTargets(teller.TargetCombat)
	}
	return fragments
}

func (c *Conversation) styleFragments(meta interactive.StoryMeta) StoryStyleFragments {
	return ResolveStoryStyleFragments(c.novaDir, meta)
}

func (c *Conversation) StoryRuntimeForMeta(meta interactive.StoryMeta) interactive.StoryDirector {
	return LoadStoryRuntimeForMeta(c.novaDir, meta)
}

// LoadStoryRuntimeForMeta assembles runtime resources from the story's own
// module selections and its independently selected planning outline.
//
// When the story carries a frozen module graph — which every story created
// through the app service does — that graph is what it resolves from, so editing
// a preset file changes the next game rather than one already in progress. A
// story without a frozen graph (one created before freezing existed, or by a
// caller that resolves its own modules) falls back to the live catalog, which is
// the behaviour it was created under.
func LoadStoryRuntimeForMeta(novaDir string, meta interactive.StoryMeta) interactive.StoryDirector {
	runtime := interactive.DefaultStoryDirector()
	template := LoadGamePlanningTemplateForMeta(novaDir, meta)
	runtime.ID = template.ID
	runtime.Name = template.Name
	runtime.Description = template.Description
	runtime.Strategy.PromptMarkdown = interactive.RenderGamePlanningTemplateMarkdown(template)
	runtime.Strategy.RuleStateConsumptionMode = meta.CheckSettings.RuleStateConsumptionMode
	runtime.Strategy.RuleVisibilityMode = meta.CheckSettings.RuleVisibilityMode
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if meta.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	}
	runtime.ModuleRefs = refs
	if meta.FrozenModules != nil {
		runtime.ResolvedSnapshot = *meta.FrozenModules
		return interactive.ResolveFrozenStoryDirectorModules(novaDir, runtime)
	}
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	return interactive.ResolveStoryDirectorModules(novaDir, runtime)
}

func LoadGamePlanningTemplateForMeta(novaDir string, meta interactive.StoryMeta) interactive.GamePlanningTemplate {
	return loadGamePlanningTemplate(novaDir, meta.PlanningTemplateID)
}

func storyDirectorForSnapshot(preset interactive.StoryDirector, snapshot *interactive.ActorStateSchemaSnapshot) interactive.StoryDirector {
	if snapshot == nil || len(snapshot.System.Templates) == 0 {
		return preset
	}
	preset.ActorState = snapshot.System
	if !preset.ModuleRefs.RuleSystemDisabled && len(snapshot.TRPGSystem.RuleTemplates) > 0 {
		preset.TRPGSystem = snapshot.TRPGSystem
	}
	return preset
}

func LoadWritingTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID)
}

func LoadGameTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID)
}

func loadInteractiveTeller(novaDir, tellerID string) teller.Definition {
	if novaDir == "" {
		return teller.Definition{}
	}
	library := teller.NewLibrary(novaDir)
	selected, err := library.Get(tellerID)
	if err == nil {
		return selected
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load narrative style failed id=%s err=%v", tellerID, err))
	fallback, fallbackErr := library.Get(style.DefaultID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load default narrative style failed id=%s err=%v", style.DefaultID, fallbackErr))
		return teller.Definition{}
	}
	return fallback
}

func loadGamePlanningTemplate(novaDir, templateID string) interactive.GamePlanningTemplate {
	if novaDir == "" {
		return interactive.DefaultGamePlanningTemplate()
	}
	item, err := interactive.NewGamePlanningTemplateLibrary(novaDir).Get(templateID)
	if err == nil {
		return item
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load planning template failed id=%s err=%v", templateID, err))
	fallback, fallbackErr := interactive.NewGamePlanningTemplateLibrary(novaDir).Get(interactive.DefaultGamePlanningTemplateID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load fallback planning template failed err=%v", fallbackErr))
		return interactive.DefaultGamePlanningTemplate()
	}
	return fallback
}

// StoryTellerSystemInput builds the main narrative's system instruction.
//
// The teller supplies the display identity; the fragments supply the expression.
// When fragments are given, the main voice comes from the resolved main purpose
// rather than from the teller definition directly, so a story whose main style
// was changed through the purpose config is generated under the style it
// actually selected.
func StoryTellerSystemInput(definition teller.Definition, fragments *StoryStyleFragments, styleRules ...[]prompts.StyleRule) prompts.InteractiveStorySystemInstructionInput {
	var rules []prompts.StyleRule
	if len(styleRules) > 0 {
		rules = styleRules[0]
	}
	systemPrompt := definition.PromptForTargets(teller.TargetSystem)
	if fragments != nil && strings.TrimSpace(fragments.MainSystem) != "" {
		systemPrompt = fragments.MainSystem
	}
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           definition.ID,
		StoryTellerName:         definition.Name,
		StoryTellerDescription:  definition.Description,
		StoryTellerSystemPrompt: systemPrompt,
		DialogueStylePrompt:     dialogueStylePromptOf(fragments),
		CombatStylePrompt:       combatStylePromptOf(fragments),
		StyleRules:              rules,
	}
}

func dialogueStylePromptOf(fragments *StoryStyleFragments) string {
	if fragments == nil {
		return ""
	}
	return fragments.Dialogue
}

func combatStylePromptOf(fragments *StoryStyleFragments) string {
	if fragments == nil {
		return ""
	}
	return fragments.Combat
}

