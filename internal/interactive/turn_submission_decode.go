package interactive

import (
	"context"
	interactivestate "ta/internal/interactive/state"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"ta/internal/character"
)

const turnSubmissionStateChangesField = "state_changes"

// TurnStateChangeInput is the model-facing state mutation shape. Stable IDs
// are separate fields so the model never has to construct or escape a JSON
// Pointer; the backend compiles this shape to the persisted interactivestate.Update.
type TurnStateChangeInput struct {
	Op           string         `json:"op" jsonschema:"enum=replace,enum=delta,enum=create,enum=archive,enum=restore" jsonschema_description:"replace writes a field's complete new value; delta changes an existing number; create adds an Actor; archive removes an Actor from runtime participation while preserving complete historical state; restore returns an archived Actor to active runtime participation."`
	ActorID      string         `json:"actor_id" jsonschema_description:"For an existing Actor, copy the ID from the state handbook exactly. For create, use the character name in the story's language and make it identical to name."`
	FieldID      string         `json:"field_id,omitempty" jsonschema_description:"Required only for replace/delta. Copy an exact Field ID from the target Actor template in the Actor State Handbook. For delta, the field or selected subpath must be an existing number."`
	Subpath      []string       `json:"subpath,omitempty" jsonschema:"maxItems=16" jsonschema_description:"Optional only for replace/delta nested updates inside object fields. For delta, target an existing nested number. Supply one string segment per level; do not construct a path string."`
	Value        any            `json:"value,omitempty" jsonschema_description:"Required only for replace/delta. For replace, provide the complete non-empty field value at the end of this turn. For delta, provide a finite amount to add to or subtract from an existing number. Match the target field type; number, bool, object, and list fields require native JSON values, not quoted strings. Zero and false are valid values."`
	TemplateID   string         `json:"template_id,omitempty" jsonschema_description:"Required only for create. Copy a Template ID from Templates Available to New Actors exactly."`
	Name         string         `json:"name,omitempty" jsonschema_description:"Required only for create. Use the character name in the story's language and make it identical to actor_id."`
	Role         string         `json:"role,omitempty" jsonschema_description:"Optional new Actor's role in the current story, used only for create."`
	Description  string         `json:"description,omitempty" jsonschema_description:"Brief Actor description used only for create."`
	// Gender, SpeechStyle and Tags are the same three facts a character_facts
	// profile carries, offered at the other formal entry point. A create that
	// settles them is how the story knows them about somebody it staged through
	// state_changes alone; leaving them out is honest when nothing settled them.
	Gender      string   `json:"gender,omitempty" jsonschema:"enum=male,enum=female" jsonschema_description:"Used only for create. This person's gender when the scene or the author settled it: male or female. Omit it when nobody did — an empty value is honest, and it must never be inferred from a name, a picture, a role or an occupation."`
	SpeechStyle string   `json:"speech_style,omitempty" jsonschema_description:"Used only for create. How this person actually talks, in two or three concrete features plus one short example line, when the scene showed it. Write what makes them recognisable against the others present, not a catchphrase and not a stereotype of their gender or occupation. Omit it when the scene has not let them speak."`
	Tags        []string `json:"tags,omitempty" jsonschema:"maxItems=6" jsonschema_description:"Used only for create. Short labels a reader would search this person by: the group or household they belong to, what they do for a living, where they are from. Two to four words each, in the story's language. Only what this appearance established — no craft nobody has named, and no region they merely passed through. Omit for someone the scene only sketched."`
	InitialState map[string]any `json:"initial_state,omitempty" jsonschema:"maxProperties=64" jsonschema_description:"Used only for create. Include every reliable initial field value; keys must be exact Field IDs from the selected template. Number, bool, object, and list fields require native JSON values, not quoted strings."`
	Reason       string         `json:"reason,omitempty" jsonschema_description:"Non-empty reason required only for archive/restore. For archive, cite facts established by prose that confirm death or permanent departure. For restore, cite facts established by prose that confirm the archived Actor's return."`
}

// DecodeInteractiveTurnSubmissionInput independently decodes state_changes,
// choices, and plan_update from one model-facing tool call. A malformed module
// does not discard valid siblings; valid plan sections also survive malformed
// siblings so later calls can provide only retry_modules and retry_sections.
func DecodeInteractiveTurnSubmissionInput(arguments string) TurnSubmissionInput {
	if len([]byte(arguments)) > maxTurnSubmissionArgumentsBytes {
		return invalidUnifiedTurnSubmissionInput("submission_too_large", "", fmt.Sprintf("%d bytes", len([]byte(arguments))), fmt.Sprintf("Tool arguments exceed %d bytes.", maxTurnSubmissionArgumentsBytes))
	}
	var root map[string]json.RawMessage
	if err := decodeStrictJSON([]byte(arguments), &root, false); err != nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidJSON, "", "invalid JSON", fmt.Sprintf("Turn submission arguments are not valid JSON: %v", err))
	}
	if root == nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidTopLevel, "", "null", "Turn submission arguments must be an object.")
	}
	allowed := map[string]bool{
		turnSubmissionStateChangesField:    true,
		TurnSubmissionModuleChoices:        true,
		TurnSubmissionModulePlanUpdate:     true,
		TurnSubmissionModuleCharacterFacts: true,
		TurnSubmissionModuleCultivation:    true,
		TurnSubmissionModuleSkills:         true,
		TurnSubmissionModuleTechniques:     true,
		TurnSubmissionModuleItems:          true,
		TurnSubmissionModuleTrade:          true,
		TurnSubmissionModuleRest:           true,
		TurnSubmissionModuleCraft:          true,
		TurnSubmissionModuleWorld:          true,
		TurnSubmissionModuleRecap:          true,
	}
	unknown := make([]string, 0)
	for key := range root {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return invalidUnifiedTurnSubmissionInput(
			TurnSubmissionDiagnosticInvalidTopLevel,
			"",
			strings.Join(unknown, ","),
			"Turn submission arguments may only contain state_changes, choices, character_facts, cultivation, skills, techniques, items, trade, rest, craft, world, optional plan_update, and optional recap.",
		)
	}

	input := TurnSubmissionInput{}
	if raw, exists := root[turnSubmissionStateChangesField]; exists {
		updates, diagnostics := decodeStructuredStateChangesModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if len(diagnostics) == 0 {
			input.StateUpdates = &updates
		}
	}
	if raw, exists := root[TurnSubmissionModuleChoices]; exists {
		choices, diagnostics := decodeChoicesModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleChoices) {
			input.Choices = &choices
		}
	}
	if raw, exists := root[TurnSubmissionModuleCharacterFacts]; exists {
		facts, diagnostics := decodeCharacterFactsModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleCharacterFacts) {
			input.CharacterFacts = facts
		}
	}
	if raw, exists := root[TurnSubmissionModuleCultivation]; exists {
		cultivation, diagnostics := decodeCultivationModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleCultivation) {
			input.Cultivation = cultivation
		}
	}
	if raw, exists := root[TurnSubmissionModuleSkills]; exists {
		skills, diagnostics := decodeSkillLearningModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleSkills) {
			input.Skills = skills
		}
	}
	if raw, exists := root[TurnSubmissionModuleTechniques]; exists {
		techniques, diagnostics := decodeTechniqueModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleTechniques) {
			input.Techniques = techniques
		}
	}
	if raw, exists := root[TurnSubmissionModuleItems]; exists {
		items, diagnostics := decodeItemsModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleItems) {
			input.Items = items
		}
	}
	if raw, exists := root[TurnSubmissionModuleTrade]; exists {
		trade, diagnostics := decodeTradeModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleTrade) {
			input.Trade = trade
		}
	}
	if raw, exists := root[TurnSubmissionModuleRest]; exists {
		rest, diagnostics := decodeRestModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleRest) {
			input.Rest = rest
		}
	}
	if raw, exists := root[TurnSubmissionModuleCraft]; exists {
		craft, diagnostics := decodeCraftModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleCraft) {
			input.Craft = craft
		}
	}
	if raw, exists := root[TurnSubmissionModuleWorld]; exists {
		world, diagnostics := decodeWorldModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleWorld) {
			input.World = world
		}
	}
	if raw, exists := root[TurnSubmissionModulePlanUpdate]; exists {
		plan, diagnostics := decodeTurnPlanUpdate(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		input.PlanUpdate = plan
	}
	// The recap never produces a diagnostic. It is display material rebuilt from
	// the committed turn, so a malformed one is dropped rather than reported: the
	// turn still publishes and the history shows an excerpt of the prose instead.
	if raw, exists := root[TurnSubmissionModuleRecap]; exists {
		input.Recap = decodeTurnRecap(raw)
	}
	return input
}

// decodeTurnRecap reads the turn's own short recollection. Anything that is not
// a string, is empty after trimming, or exceeds the byte bound is treated as no
// recap at all, which the history renders as a prose excerpt.
func decodeTurnRecap(raw json.RawMessage) *string {
	var value string
	if err := decodeStrictJSON(raw, &value, false); err != nil {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len([]byte(value)) > maxTurnRecapBytes {
		runes := []rune(value)
		if len(runes) > maxTurnRecapRunes {
			runes = runes[:maxTurnRecapRunes]
		}
		value = strings.TrimSpace(string(runes))
		if value == "" {
			return nil
		}
	}
	return &value
}

// decodeCharacterFactsModule decodes the optional character-side module. A
// malformed module is reported against its own path so the model can repair it
// without resubmitting the state changes and choices it already got right.
func decodeCharacterFactsModule(raw json.RawMessage) (*TurnCharacterFactsInput, []TurnSubmissionDiagnostic) {
	var facts TurnCharacterFactsInput
	if err := decodeStrictJSON(raw, &facts, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
			"/character_facts", "object with optional profiles, exchanges, knowledge, matters, messages, affinities and relationships",
			jsonValueKind(raw), fmt.Sprintf("character_facts could not be decoded: %v", err),
		)}
	}
	if facts.Empty() {
		// An explicitly empty module is accepted as "this turn established
		// nothing about anyone", which is a real and common outcome. It commits
		// no record.
		return nil, nil
	}
	for index, exchange := range facts.Exchanges {
		switch character.Channel(strings.TrimSpace(exchange.Channel)) {
		case character.ChannelScene, character.ChannelPrivate, character.ChannelMessage:
		default:
			return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCharacterFacts, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/exchanges/%d/channel", index), "scene, private or message",
				strings.TrimSpace(exchange.Channel), "Each exchange must name a channel the story records.",
			)}
		}
		switch character.Speaker(strings.TrimSpace(exchange.Speaker)) {
		case character.SpeakerPlayer, character.SpeakerCharacter:
		default:
			return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCharacterFacts, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/exchanges/%d/speaker", index), "character or player",
				strings.TrimSpace(exchange.Speaker), "Each exchange must name who spoke.",
			)}
		}
	}
	return &facts, nil
}

// decodeCultivationModule decodes the optional rules-side module. Like character
// facts, an explicitly empty module is accepted as "this turn held no authorised
// training", which commits nothing and is the common case.
func decodeCultivationModule(raw json.RawMessage) (*TurnCultivationInput, []TurnSubmissionDiagnostic) {
	var input TurnCultivationInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCultivation, nil, TurnSubmissionDiagnosticInvalidModule,
			"/cultivation", "object with optional days, environment, resource and attempt_breakthrough",
			jsonValueKind(raw), fmt.Sprintf("cultivation could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnCultivationModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeRestModule decodes the optional rest module. Like cultivation, an
// explicitly empty module means "this turn spent no time resting", which is the
// common case and commits nothing.
func decodeRestModule(raw json.RawMessage) (*TurnRestInput, []TurnSubmissionDiagnostic) {
	var input TurnRestInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleRest, nil, TurnSubmissionDiagnosticInvalidModule,
			"/rest", "object with optional actor_id, mode and duration_seconds",
			jsonValueKind(raw), fmt.Sprintf("rest could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnRestModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeCraftModule decodes the optional craft module. Like rest, an explicitly
// empty module means "this turn worked no batch", which is the common case and
// commits nothing.
func decodeCraftModule(raw json.RawMessage) (*TurnCraftInput, []TurnSubmissionDiagnostic) {
	var input TurnCraftInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticInvalidModule,
			"/craft", "object with optional actor_id, method_id and duration_seconds",
			jsonValueKind(raw), fmt.Sprintf("craft could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnCraftModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeWorldModule decodes the world decision module. An explicitly empty
// object carries no decision and stays nil, so the submission reports the
// module missing; action=none is the explicit declaration that this turn
// settles no journey, task, event or realm.
func decodeWorldModule(raw json.RawMessage) (*TurnWorldActionInput, []TurnSubmissionDiagnostic) {
	var input TurnWorldActionInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticInvalidModule,
			"/world", "object with action, or action=none",
			jsonValueKind(raw), fmt.Sprintf("world could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnWorldActionModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeSkillLearningModule decodes the optional technique-learning module. Like
// cultivation, an explicitly empty module means "this turn taught nothing", which
// is the common case and commits nothing.
func decodeSkillLearningModule(raw json.RawMessage) (*TurnSkillLearningInput, []TurnSubmissionDiagnostic) {
	var input TurnSkillLearningInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleSkills, nil, TurnSubmissionDiagnosticInvalidModule,
			"/skills", "object with an optional actor_id and a skills list",
			jsonValueKind(raw), fmt.Sprintf("skills could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnSkillLearningModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

func decodeTurnPlanUpdate(raw json.RawMessage) (*TurnPlanUpdateInput, []TurnSubmissionDiagnostic) {
	var root map[string]json.RawMessage
	if err := decodeStrictJSON(raw, &root, false); err != nil || root == nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
			"/plan_update", "plan update object", jsonValueKind(raw), "plan_update must be an object with mode and matching content.",
		)}
	}
	allowed := map[string]bool{"mode": true, "markdown": true, "sections": true}
	unknown := make([]string, 0)
	for key := range root {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
			"/plan_update", "only mode, markdown, and sections", strings.Join(unknown, ","), "plan_update contains unsupported fields.",
		)}
	}

	var mode string
	modeRaw, hasMode := root["mode"]
	if !hasMode || decodeStrictJSON(modeRaw, &mode, false) != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidPlanMode,
			"/plan_update/mode", "replace_document or replace_sections", jsonValueKind(modeRaw), "plan_update.mode is required and must be a string.",
		)}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	update := &TurnPlanUpdateInput{Mode: mode}
	switch mode {
	case TurnPlanUpdateModeReplaceDocument:
		if _, hasSections := root["sections"]; hasSections {
			return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/sections", "omitted for replace_document", "present", "replace_document accepts markdown only.",
			)}
		}
		markdownRaw, exists := root["markdown"]
		if !exists || decodeStrictJSON(markdownRaw, &update.Markdown, false) != nil {
			return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/markdown", "complete Markdown string", jsonValueKind(markdownRaw), "replace_document requires a Markdown string.",
			)}
		}
		return update, nil

	case TurnPlanUpdateModeReplaceSections:
		if _, hasMarkdown := root["markdown"]; hasMarkdown {
			return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/markdown", "omitted for replace_sections", "present", "replace_sections accepts sections only.",
			)}
		}
		sectionsRaw, exists := root["sections"]
		if !exists {
			return update, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/sections", "non-empty array", "missing", "replace_sections requires at least one section update.",
			)}
		}
		var items []json.RawMessage
		if err := decodeStrictJSON(sectionsRaw, &items, false); err != nil || items == nil {
			return update, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/sections", "non-empty array", jsonValueKind(sectionsRaw), "replace_sections.sections must be a native array.",
			)}
		}
		if len(items) == 0 {
			return update, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/sections", "non-empty array", "empty array", "replace_sections requires at least one section update.",
			)}
		}
		diagnostics := make([]TurnSubmissionDiagnostic, 0)
		for index, item := range items {
			section, diagnostic := decodeTurnPlanSectionUpdate(item, index)
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
				continue
			}
			update.Sections = append(update.Sections, section)
		}
		return update, diagnostics

	default:
		return update, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidPlanMode,
			"/plan_update/mode", "replace_document or replace_sections", mode, "plan_update.mode is not supported.",
		)}
	}
}

func decodeTurnPlanSectionUpdate(raw json.RawMessage, index int) (TurnPlanSectionUpdate, *TurnSubmissionDiagnostic) {
	var root map[string]json.RawMessage
	path := fmt.Sprintf("/plan_update/sections/%d", index)
	if err := decodeStrictJSON(raw, &root, false); err != nil || root == nil {
		return TurnPlanSectionUpdate{}, newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
			path, "object with heading and markdown", jsonValueKind(raw), "Each plan section update must be an object.",
		)
	}
	if len(root) != 2 || root["heading"] == nil || root["markdown"] == nil {
		return TurnPlanSectionUpdate{}, newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
			path, "exactly heading and markdown", "missing or unsupported fields", "Each plan section update accepts exactly heading and markdown.",
		)
	}
	var section TurnPlanSectionUpdate
	if err := decodeStrictJSON(root["heading"], &section.Heading, false); err != nil {
		return TurnPlanSectionUpdate{}, newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
			path+"/heading", "string", jsonValueKind(root["heading"]), "Plan section heading must be a string.",
		)
	}
	if err := decodeStrictJSON(root["markdown"], &section.Markdown, false); err != nil {
		return TurnPlanSectionUpdate{}, newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
			path+"/markdown", "string", jsonValueKind(root["markdown"]), "Plan section markdown must be a string.",
		)
	}
	section.sourceIndex = intPointer(index)
	return section, nil
}

func decodeStructuredStateChangesModule(raw json.RawMessage) ([]interactivestate.Update, []TurnSubmissionDiagnostic) {
	items, err := decodeStructuredStateChangeItems(raw)
	if err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleStateChanges,
			nil,
			TurnSubmissionDiagnosticInvalidModule,
			"/state_changes",
			"array",
			jsonValueKind(raw),
			fmt.Sprintf("state_changes must be a native array; only one string layer containing valid array JSON is tolerated: %v", err),
		)}
	}
	updates := make([]interactivestate.Update, 0, len(items))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	for index, item := range items {
		var change TurnStateChangeInput
		if err := decodeStrictJSON(item, &change, true); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleStateChanges,
				intPointer(index),
				TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/state_changes/%d", index),
				"structured state change",
				jsonValueKind(item),
				fmt.Sprintf("The state change shape is invalid: %v", err),
			))
			continue
		}
		update, err := stateUpdateFromStructuredInput(change)
		if err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleStateChanges,
				intPointer(index),
				TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/state_changes/%d", index),
				"valid replace, delta, create, archive, or restore fields",
				"invalid state change",
				err.Error(),
			))
			continue
		}
		updates = append(updates, update)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return updates, nil
}

// decodeStructuredStateChangeItems keeps the model-facing contract strict while
// tolerating the one legacy shape observed in real runs: an otherwise valid
// array JSON value encoded once as a string. It intentionally does not recurse,
// repair malformed pseudo-JSON, or accept null so invalid facts still trigger a
// targeted state_changes retry.
func decodeStructuredStateChangeItems(raw json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	directErr := decodeStrictJSON(raw, &items, false)
	if directErr == nil && items != nil {
		return items, nil
	}
	if jsonValueKind(raw) != "string" {
		if directErr != nil {
			return nil, directErr
		}
		return nil, errors.New("state_changes cannot be null")
	}

	var encoded string
	if err := decodeStrictJSON(raw, &encoded, false); err != nil {
		return nil, err
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errors.New("state_changes string cannot be empty")
	}
	items = nil
	if err := decodeStrictJSON([]byte(encoded), &items, false); err != nil {
		return nil, fmt.Errorf("state_changes string does not contain valid array JSON: %w", err)
	}
	if items == nil {
		return nil, errors.New("state_changes string cannot contain null")
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-turn-submission] accepted one-layer string-encoded state_changes bytes=%d location=internal/interactive/turn_submission_decode.go", len(encoded)))
	return items, nil
}

func stateUpdateFromStructuredInput(change TurnStateChangeInput) (interactivestate.Update, error) {
	change.Op = strings.ToLower(strings.TrimSpace(change.Op))
	change.ActorID = strings.TrimSpace(change.ActorID)
	change.FieldID = strings.TrimSpace(change.FieldID)
	change.TemplateID = strings.TrimSpace(change.TemplateID)
	if change.ActorID == "" {
		return interactivestate.Update{}, fmt.Errorf("state_changes requires actor_id")
	}
	switch change.Op {
	case interactivestate.Replace, interactivestate.Delta:
		if change.FieldID == "" {
			return interactivestate.Update{}, fmt.Errorf("%s requires field_id", change.Op)
		}
		if change.Value == nil {
			return interactivestate.Update{}, fmt.Errorf("%s requires a non-null value", change.Op)
		}
		if change.TemplateID != "" || change.Name != "" || change.Role != "" || change.Description != "" || change.Gender != "" || change.SpeechStyle != "" || len(change.Tags) > 0 || change.InitialState != nil || strings.TrimSpace(change.Reason) != "" {
			return interactivestate.Update{}, fmt.Errorf("%s does not accept template_id, name, role, description, gender, speech_style, tags, initial_state, or reason", change.Op)
		}
		segments := []string{change.ActorID, change.FieldID}
		for _, segment := range change.Subpath {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				return interactivestate.Update{}, fmt.Errorf("subpath must not contain empty segments")
			}
			segments = append(segments, segment)
		}
		return interactivestate.Update{Op: change.Op, Path: interactivestate.FormatPath(segments), Value: change.Value}, nil
	case interactivestate.Create:
		if change.TemplateID == "" {
			return interactivestate.Update{}, fmt.Errorf("create requires template_id")
		}
		if strings.TrimSpace(change.Name) == "" {
			return interactivestate.Update{}, fmt.Errorf("create requires name matching actor_id")
		}
		if change.FieldID != "" || len(change.Subpath) > 0 || change.Value != nil || strings.TrimSpace(change.Reason) != "" {
			return interactivestate.Update{}, fmt.Errorf("create does not accept field_id, subpath, value, or reason")
		}
		value := map[string]any{"template_id": change.TemplateID}
		if name := strings.TrimSpace(change.Name); name != "" {
			value["name"] = name
		}
		if role := strings.TrimSpace(change.Role); role != "" {
			value["role"] = role
		}
		if description := strings.TrimSpace(change.Description); description != "" {
			value["description"] = description
		}
		if gender := character.NormalizeGender(change.Gender); gender != "" {
			value["gender"] = gender
		}
		if speech := strings.TrimSpace(change.SpeechStyle); speech != "" {
			value["speech_style"] = speech
		}
		if tags := normalizeIdentityTags(change.Tags); len(tags) > 0 {
			value["tags"] = tags
		}
		if change.InitialState != nil {
			value["state"] = change.InitialState
		}
		return interactivestate.Update{Op: interactivestate.Create, Path: interactivestate.FormatPath([]string{change.ActorID}), Value: value}, nil
	case interactivestate.Archive, interactivestate.Restore:
		reason := strings.TrimSpace(change.Reason)
		if reason == "" {
			return interactivestate.Update{}, fmt.Errorf("%s requires a non-empty reason", change.Op)
		}
		if len([]byte(reason)) > maxActorArchiveReasonBytes {
			return interactivestate.Update{}, fmt.Errorf("%s reason exceeds %d bytes", change.Op, maxActorArchiveReasonBytes)
		}
		if change.FieldID != "" || len(change.Subpath) > 0 || change.Value != nil || change.TemplateID != "" || change.Name != "" || change.Role != "" || change.Description != "" || change.Gender != "" || change.SpeechStyle != "" || len(change.Tags) > 0 || change.InitialState != nil {
			return interactivestate.Update{}, fmt.Errorf("%s accepts only op, actor_id, and reason", change.Op)
		}
		return interactivestate.Update{Op: change.Op, Path: interactivestate.FormatPath([]string{change.ActorID}), Value: map[string]any{"reason": reason}}, nil
	default:
		return interactivestate.Update{}, fmt.Errorf("op must be replace, delta, create, archive, or restore")
	}
}

func invalidUnifiedTurnSubmissionInput(code, path, actual, message string) TurnSubmissionInput {
	diagnostics := make([]TurnSubmissionDiagnostic, 0, len(turnSubmissionModules))
	for _, module := range turnSubmissionModules {
		diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
			module,
			nil,
			code,
			path,
			"object containing state_changes and/or choices",
			actual,
			message,
		))
	}
	return TurnSubmissionInput{Diagnostics: diagnostics}
}

func turnSubmissionHasDiagnostic(diagnostics []TurnSubmissionDiagnostic, module string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Module == module {
			return true
		}
	}
	return false
}

// decodeTechniqueModule decodes the optional method module. Like cultivation and
// skills, an explicitly empty module means "this turn studied and switched
// nothing", which is the common case and commits nothing.
func decodeTechniqueModule(raw json.RawMessage) (*TurnTechniqueInput, []TurnSubmissionDiagnostic) {
	var input TurnTechniqueInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleTechniques, nil, TurnSubmissionDiagnosticInvalidModule,
			"/techniques", "object with optional actor_id, learn, circulate and stop lists",
			jsonValueKind(raw), fmt.Sprintf("techniques could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnTechniqueModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeTradeModule decodes the optional trade module. A malformed module is
// reported against its own path so the model can repair it without resubmitting
// the modules it already got right.
func decodeTradeModule(raw json.RawMessage) (*TurnTradeInput, []TurnSubmissionDiagnostic) {
	var input TurnTradeInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleTrade, nil, TurnSubmissionDiagnosticInvalidModule,
			"/trade", "object with optional actor_id, merchant, buys, sells and payments lists",
			jsonValueKind(raw), fmt.Sprintf("trade could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnTradeModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}

// decodeItemsModule decodes the optional item module. Like cultivation and
// learning, an explicitly empty module means "this turn changed no item", which
// is the common case and commits nothing.
func decodeItemsModule(raw json.RawMessage) (*TurnItemsInput, []TurnSubmissionDiagnostic) {
	var input TurnItemsInput
	if err := decodeStrictJSON(raw, &input, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleItems, nil, TurnSubmissionDiagnosticInvalidModule,
			"/items", "object with optional actor_id, acquire, equip, unequip and use lists",
			jsonValueKind(raw), fmt.Sprintf("items could not be decoded: %v", err),
		)}
	}
	if input.Empty() {
		return nil, nil
	}
	if diagnostic := validateTurnItemsModule(&input); diagnostic != nil {
		return nil, []TurnSubmissionDiagnostic{*diagnostic}
	}
	return &input, nil
}
