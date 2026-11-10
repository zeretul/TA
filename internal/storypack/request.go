package storypack

import (
	"strings"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
)

// Defaults mirror the original story setup panel so a pack only declares what
// it actually changes, and an omitted field behaves exactly like the manual
// creation path.
const (
	DefaultEventPackageID   = "default"
	DefaultRuleSystemID     = "default"
	DefaultActorStateID     = "default"
	DefaultImagePresetID    = "realistic"
	DefaultReplyTargetChars = 2500
	DefaultChoiceCount      = 3
)

// StoryRequest converts a pack into the request the original story-creation
// entry point already accepts.
//
// The pack's settings supply the new game's defaults: planning mode and
// template, the style of each purpose, the length tendency and the check
// settings. They are defaults, not locks — CreateStory copies them into the
// story, so a player who changes their own game afterwards keeps that change,
// and editing the pack later leaves a running story on what it started with.
func (p Pack) StoryRequest() interactive.CreateStoryRequest {
	refs := p.Story.ModuleRefs
	moduleRefs := &interactive.StoryDirectorModuleRefs{
		EventPackageIDs: firstNonEmptySlice(refs.EventPackageIDs, []string{DefaultEventPackageID}),
		RuleSystemID:    firstNonEmpty(refs.RuleSystemID, DefaultRuleSystemID),
		ActorStateID:    firstNonEmpty(refs.ActorStateID, DefaultActorStateID),
		ImagePresetID:   firstNonEmpty(refs.ImagePresetID, DefaultImagePresetID),
	}
	settings := p.EffectiveSettings()
	if settings.RuleSystemID != "" {
		moduleRefs.RuleSystemID = settings.RuleSystemID
	}
	schemaMode := interactive.StoryStateSchemaModeFixedTemplate
	if p.Story.Gameplay.StatusTemplate == WorldStatusGeneralActions {
		schemaMode = interactive.StoryStateSchemaModeAdaptTemplate
	}
	return interactive.CreateStoryRequest{
		Title:              p.Story.Title,
		Origin:             p.Story.Origin,
		Protagonist:        p.protagonist(),
		ModuleRefs:         moduleRefs,
		ReplyTargetChars:   settings.ReplyTargetChars,
		ChoiceCount:        firstNonZero(p.Story.ChoiceCount, DefaultChoiceCount),
		Opening:            p.opening(),
		StoryTellerID:      settings.StylePurposes.Main,
		PlanningTemplateID: settings.PlanningTemplateID,
		PlanningMode:       settings.PlanningMode,
		StylePurposes:      settings.interactiveStylePurposes(),
		CheckSettings:      settings.interactiveCheckSettings(),
		// The pack's direction is a seed, not a binding: CreateStory copies it
		// into the story, so a later edit to the pack leaves a running story on
		// the text it started with, and a player edit changes only their story.
		NarrativeRequirements: p.narrativeRequirements(settings),
		StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{
			Mode: schemaMode,
		},
		// The binding is what keeps an existing story on the pack revision it
		// started from, and what scopes its lore reads to this world.
		PackBinding: p.Binding(),
	}
}

// StoryRequestForTemplate converts a pack into a creation request for one of
// its ready-made characters.
//
// The template contributes the same two things a hand-written character does —
// a protagonist snapshot and an origin — and nothing else. Its State is
// returned beside the request rather than inside it because the protagonist
// snapshot carries no state: the app layer merges it into the bound actor-state
// module's protagonist actor, which is the one place initial state reaches
// CreateStory.
//
// The template's authored opening_text is deliberately not used. A start is
// either model-written from the character's premise or the player's own text;
// carrying the template's scene into the custom field would pass authored prose
// off as the player's draft.
func (p Pack) StoryRequestForTemplate(templateID string) (interactive.CreateStoryRequest, map[string]any, error) {
	template, ok := p.ProtagonistTemplate(templateID)
	if !ok {
		return interactive.CreateStoryRequest{}, nil, identityError("api.storyPack.templateMissing", "template", templateID)
	}
	req := p.StoryRequest()
	req.Protagonist = interactive.StoryProtagonist{
		Mode:    interactive.StoryProtagonistModeCustom,
		Name:    trimSpace(template.CharacterName),
		Profile: trimSpace(template.Profile),
	}
	if origin := trimSpace(template.Origin); origin != "" {
		req.Origin = origin
	}
	state := make(map[string]any, len(template.State))
	for key, value := range template.State {
		state[key] = value
	}
	if len(state) == 0 {
		state = nil
	}
	return req, state, nil
}

// opening is always the model-written path. A world's authored opening_text
// describes the scene its own template person wakes in, not a draft the player
// wrote, so it can no longer be submitted as one.
func (p Pack) opening() interactive.StoryOpeningConfig {
	return interactive.StoryOpeningConfig{Mode: interactive.StoryOpeningModeAI}
}

func (p Pack) protagonist() interactive.StoryProtagonist {
	if name := trimSpace(p.Story.ProtagonistName); name != "" {
		return interactive.StoryProtagonist{
			Mode:    "custom",
			Name:    name,
			Profile: trimSpace(p.Story.Protagonist),
		}
	}
	return interactive.StoryProtagonist{Mode: "default"}
}

// WorldInputs converts the pack's world book into lore inputs. Every entry is
// stamped with this pack revision's scope and revision-qualified ID, so a pack's
// world book never lands in the shared book, never contends with another pack's
// entries of the same name, and never collides with an earlier revision's copy
// of the same entry. A new revision therefore reads its own content while a game
// already running keeps reading the revision it started from.
func (p Pack) WorldInputs() []booklore.ItemInput {
	scope := p.WorldScope()
	hash := p.ContentHash()
	inputs := make([]booklore.ItemInput, 0, len(p.World))
	for _, entry := range p.World {
		inputs = append(inputs, booklore.ItemInput{
			Audience:         booklore.NormalizeAudience(entry.Audience),
			ID:               interactive.PackEntryIDForRevision(entry.ID, hash),
			Scope:            scope,
			Type:             firstNonEmpty(entry.Type, "world"),
			Name:             entry.Name,
			Importance:       firstNonEmpty(entry.Importance, "medium"),
			Tags:             entry.Tags,
			Keywords:         entry.Keywords,
			LoadMode:         worldEntryLoadMode(entry.Type, entry.LoadMode),
			BriefDescription: entry.BriefDescription,
			Content:          entry.Content,
		})
	}
	return inputs
}

// narrativeRequirements resolves the direction a new game starts with.
//
// The author's text is the default only while the player has saved no defaults
// of their own. Once they have, their block is the whole answer: a direction
// they cleared on the page must stay cleared, because reading the empty value
// as "unset" would quietly restore text the page no longer shows.
func (p Pack) narrativeRequirements(settings PackSettings) string {
	if p.settingsCustomized {
		return settings.NarrativeRequirements
	}
	return firstNonEmpty(settings.NarrativeRequirements, p.Story.NarrativeRequirements)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values []string, fallback []string) []string {
	for _, value := range values {
		if trimSpace(value) != "" {
			return values
		}
	}
	return fallback
}

func firstNonZero(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func trimSpace(value string) string { return strings.TrimSpace(value) }

// Explicit routing wins, including manual secrets and contextual auto rules.
// An unannotated rule is core context for both imported and generated packs.
func worldEntryLoadMode(kind, declared string) string {
	if mode := strings.TrimSpace(declared); mode != "" {
		return mode
	}
	if strings.TrimSpace(kind) == "rule" {
		return "resident"
	}
	return "auto"
}
