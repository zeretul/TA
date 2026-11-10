package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

const actorStateRuntimeTruncatedNotice = "> Content was truncated at the context limit. Do not guess Actors, fields, or templates that are not shown."

// ActorStateRuntimeContext compiles the effective story schema and replayed
// values into a bounded Markdown write guide. JSON remains the backend
// contract; the model receives readable semantics, exact stable IDs, current
// values, and examples.
func ActorStateRuntimeContext(system StoryDirectorActorStateSystem, state map[string]any, limitBytes int, configuredChoiceCount ...int) string {
	return ActorStateRuntimeContextAt(system, state, limitBytes, 0, nil, configuredChoiceCount...)
}

// ActorStateRuntimeContextAt is ActorStateRuntimeContext with the branch's
// formal instant, so each Actor can say how long ago it was last written.
//
// A record is not restated every turn. One written a decade ago and never
// revisited still reads as present tense to a model that is not told otherwise,
// which is how a stale 当前行动 gets carried forward as though it were the world
// now. The instant is optional: without it the records are shown undated rather
// than dated to the wrong moment.
func ActorStateRuntimeContextAt(system StoryDirectorActorStateSystem, state map[string]any, limitBytes int, nowSeconds int64, calendar *StoryCalendar, configuredChoiceCount ...int) string {
	return ActorStateRuntimeContextWithSettings(system, state, limitBytes, nowSeconds, calendar, nil, configuredChoiceCount...)
}

// ActorStateRuntimeContextWithSettings is ActorStateRuntimeContextAt with the
// per-entity reading settings the player set on the 本局新建 page.
//
// An entity marked 常驻取用 is read every turn even after the scene moves away,
// so the bounded projection keeps it ahead of everyone the scene does not place.
// An entity marked 指定取用 is withheld unless the scene places it on stage.
// Neither reading says anything about whether the entity is present, awake, or
// able to act — that is still the narrative's to say.
func ActorStateRuntimeContextWithSettings(system StoryDirectorActorStateSystem, state map[string]any, limitBytes int, nowSeconds int64, calendar *StoryCalendar, settings map[string]string, configuredChoiceCount ...int) string {
	if limitBytes <= 0 || limitBytes > StoryContextMaxBytes {
		limitBytes = StoryContextMaxBytes
	}
	system = normalizeActorStateSystem(system)
	state = ActorStateRuntimeProjectionWithSettings(system, state, settings)
	choiceCount := DefaultStoryChoiceCount
	if len(configuredChoiceCount) > 0 {
		choiceCount = normalizeStoryChoiceCount(configuredChoiceCount[0])
	}
	if validateStoryChoiceCount(choiceCount) != nil {
		choiceCount = DefaultStoryChoiceCount
	}
	blocks := actorStateRuntimeMarkdownBlocks(system, state, choiceCount, nowSeconds, calendar, settings)
	if system.AdventureRules != nil {
		rules := *system.AdventureRules
		if actor, err := AdventureActorFromState(state, DefaultActorID); err == nil {
			if generated, err := adventureRulesForActor(rules, actor); err == nil {
				rules = generated
			}
		}
		blocks = append([]string{AdventureRulesContext(rules)}, blocks...)
	}
	return joinBoundedActorStateRuntimeBlocks(blocks, limitBytes)
}

func actorStateRuntimeMarkdownBlocks(system StoryDirectorActorStateSystem, state map[string]any, choiceCount int, nowSeconds int64, calendar *StoryCalendar, settings map[string]string) []string {
	blocks := []string{strings.Join([]string{
		"# Actor State Handbook",
		"",
		"> Source: `effective_actor_state_schema` + `Snapshot.State.actors` + `Snapshot.State.actor_archives`. Missing schema-defined initial Actors are projected at runtime only; event history is not rewritten.",
		"",
		"- Submit only changes that actually occurred in this turn's narrative. Do not repeat unchanged fields or clear them with empty values.",
		"- When referencing an existing Actor, copy the `actor_id` shown below in backticks exactly. Reuse `template_id` and `field_id` exactly as well.",
		"- When creating an Actor, actor_id and name must be identical and must use the character name in the story's language. Do not generate English translations, pinyin, or slug IDs.",
		"- `description` explains what a field means; `update_instruction` defines when and how to update it. Follow both.",
		"- Current State lists only values writable this turn. Field semantics and update rules appear once under Templates Available to New Actors to avoid duplicate context.",
		"- `replace` writes the complete end-of-turn value. Use `delta` only for an existing number. Do not resubmit fields already consumed by a rule check.",
		"- Use `archive` only for a non-system Actor whose death or permanent departure is confirmed in the narrative. The full historical state is preserved, but the Actor cannot be written or used in checks next turn.",
		"- 仅在已归档且仍然存活的 Actor 确实归来时使用 `restore`。archive 与 restore 都需要真实依据的 `reason`，不能只凭生命值或措辞推断。若归档摘要含 `death_event_id`，该主体已由正式规则结算死亡，普通 restore 不能撤销死亡。",
	}, "\n")}

	rawActors, _ := state[actorStateRoot].(map[string]any)
	actorIDs := make([]string, 0, len(rawActors))
	for actorID := range rawActors {
		actorIDs = append(actorIDs, actorID)
	}
	// The cap keeps the foundational Actors and the characters this scene places
	// on stage before it keeps anyone else, so a story past the limit still hands
	// the model the state it must write and the people it is writing about.
	actorIDs = actorProjectionOrderWithSettings(actorIDs, StorySceneFromState(state, "").Present, settings)
	if len(actorIDs) > maxInteractiveListItems {
		actorIDs = actorIDs[:maxInteractiveListItems]
	}
	blocks = append(blocks, "## Current State")
	for _, actorID := range actorIDs {
		if block := actorStateRuntimeActorMarkdown(system, state, rawActors, actorID, nowSeconds, calendar); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(actorIDs) == 0 {
		blocks = append(blocks, "> There are no active Actors. Restore an archived Actor only when it truly returns, or create a character from Templates Available to New Actors only when persistent tracking is necessary.")
	}
	if archives, ok := state[actorArchiveRoot].([]ActorArchiveSummary); ok && len(archives) > 0 {
		blocks = append(blocks, "## Archived Actors (Read-only Index)")
		blocks = append(blocks, "> This section includes only identity and archive provenance needed to decide whether to restore an Actor. Full archived state is excluded from the next-turn context.")
		for _, archive := range archives {
			blocks = append(blocks, actorStateRuntimeArchiveMarkdown(archive))
		}
	}

	blocks = append(blocks, actorStateRuntimeSubmissionTemplate(system, rawActors, actorIDs, choiceCount))
	blocks = append(blocks, "## Templates Available to New Actors")
	for index, template := range system.Templates {
		if index >= maxInteractiveListItems {
			break
		}
		blocks = append(blocks, actorStateRuntimeTemplateMarkdown(template, system.CultivationRules))
	}
	return blocks
}

func actorStateRuntimeArchiveMarkdown(archive ActorArchiveSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(archive.Name, archive.ActorID)))
	fmt.Fprintf(&sb, "- Actor ID: %s\n", actorStateRuntimeCode(archive.ActorID))
	if archive.TemplateID != "" {
		fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(archive.TemplateID))
	}
	if archive.Reason != "" {
		fmt.Fprintf(&sb, "- Archive reason: %s\n", actorStateRuntimeText(archive.Reason))
	}
	if archive.SourceTurnID != "" {
		fmt.Fprintf(&sb, "- Source turn: %s\n", actorStateRuntimeCode(archive.SourceTurnID))
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeActorMarkdown(system StoryDirectorActorStateSystem, state map[string]any, rawActors map[string]any, actorID string, nowSeconds int64, calendar *StoryCalendar) string {
	record, _ := rawActors[actorID].(map[string]any)
	if record == nil {
		return ""
	}
	templateID := normalizeActorStateID(fmt.Sprint(record["template_id"]))
	template := actorStateTemplateByID(system, templateID)
	if template.ID == "" {
		return ""
	}
	name, _ := record["name"].(string)
	role, _ := record["role"].(string)
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(name, actorID)))
	fmt.Fprintf(&sb, "- Actor ID: %s\n", actorStateRuntimeCode(actorID))
	fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(templateID))
	if strings.TrimSpace(role) != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", actorStateRuntimeText(role))
	}
	if description := actorStateRuntimeText(fmt.Sprint(record["description"])); description != "" && description != "<nil>" {
		fmt.Fprintf(&sb, "- Actor description: %s\n", description)
	}
	if line := actorStateRuntimeWrittenLine(record, nowSeconds, calendar); line != "" {
		sb.WriteString(line)
	}

	rawState, _ := record["state"].(map[string]any)
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		value := rawState[fieldID]
		if value == nil && strings.TrimSpace(field.LegacyPath) != "" {
			value = getPathExact(rawState, field.LegacyPath)
		}
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- Field ID: %s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- Current value: %s\n", actorStateRuntimeValue(generatedRuntimeFieldValue(fieldID, value)))
		fmt.Fprintf(&sb, "- Type: %s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
	}

	traits := actorTraitInstancesFromState(state, actorID)
	if len(traits) > 0 {
		sb.WriteString("\n#### Assigned Traits (Read-only)\n")
		for _, trait := range traits {
			fmt.Fprintf(&sb, "\n- %s (Trait ID: %s)", actorStateRuntimeText(trait.Name), actorStateRuntimeCode(trait.TraitID))
			if summary := actorStateRuntimeText(trait.Summary); summary != "" {
				fmt.Fprintf(&sb, ": %s", summary)
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

// actorStateRuntimeWrittenLine states how old this Actor's record is.
//
// It is the answer to a question the record itself cannot answer: a 当前行动
// written before a decade of seclusion still reads as present tense. The line
// gives the story's own reading of when it was written — the date on this
// branch's calendar and how long ago that was — so a narrator can tell a
// decade-old note from this morning's. A record from before this was recorded
// at all says nothing rather than claiming to be current.
//
// An instant of zero is a real answer, not a missing one: it is the story's own
// start, and a record written in the opening turns was written then. Treating it
// as unrecorded left exactly the records this line exists for — the faction note
// written on the first day and never revisited — undated a decade later.
func actorStateRuntimeWrittenLine(record map[string]any, nowSeconds int64, calendar *StoryCalendar) string {
	eventID := strings.TrimSpace(fmt.Sprint(record[actorWrittenAtEventField]))
	if eventID == "" || eventID == "<nil>" {
		return ""
	}
	atSeconds, ok := actorStateNumber(record[actorWrittenAtSecondsField])
	if !ok {
		return ""
	}
	written := int64(atSeconds)
	if written < 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if calendar != nil {
		if date := calendar.DescribeDate(written); date != "" {
			parts = append(parts, date)
		}
	}
	aged := false
	if nowSeconds > written {
		if elapsed := DescribeElapsed(nowSeconds - written); elapsed != "" {
			parts = append(parts, "此后又过了"+elapsed)
			aged = true
		}
	}
	if len(parts) == 0 {
		return ""
	}
	line := "- Written: " + strings.Join(parts, "，") + "。"
	if aged {
		// The caveat belongs only to a record that really predates the present.
		// Said about one written this turn it is noise on every Actor in every
		// turn, and a warning that is always on is one the narrator learns to skip.
		line += "这条记录写的是那个时候的样子，不一定是现在的样子。"
	}
	return line + "\n"
}

func actorStateRuntimeTemplateMarkdown(template ActorStateTemplate, profiles ...*cultivation.Rules) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(template.Name, template.ID)))
	fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(template.ID))
	if description := actorStateRuntimeText(template.Description); description != "" {
		fmt.Fprintf(&sb, "- Template description: %s\n", description)
	}
	if len(template.TraitRules) > 0 {
		sb.WriteString("- Traits are assigned automatically by the backend according to template rules. Do not fabricate traits in create parameters.\n")
	}
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- Field ID: %s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- Type: %s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
		if field.Default != nil {
			fmt.Fprintf(&sb, "- Default: %s\n", actorStateRuntimeValue(field.Default))
		}
		if description := actorStateRuntimeText(field.Description); description != "" {
			fmt.Fprintf(&sb, "- Field description: %s\n", description)
		}
		if instruction := actorStateRuntimeText(field.UpdateInstruction); instruction != "" {
			fmt.Fprintf(&sb, "- Update instruction: %s\n", instruction)
		}
		if actorStateTemplateIsXiuxian(template, profiles...) && xiuxianDerivedFields()[fieldID] {
			// Said at the field, because the create example alone did not stop it:
			// a model meeting the boss wrote 攻击/生命上限/灵力上限/神识/身法/防御 into
			// initial_state, the frozen-schema check refused the whole state_changes
			// module, and the encounter's Actor was never created at all. These six
			// come out of 当前境界 through the stage table, so stating them is not a
			// stronger version of the truth — it is a second, conflicting source.
			sb.WriteString("- Derived from 当前境界 by the cultivation rules. Do not set it in create or in a later update; state the stage and the rules fill these in.\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeSubmissionTemplate(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string, choiceCount int) string {
	actorID, fieldID, exampleValue := actorStateRuntimeExample(system, rawActors, actorIDs)
	choices := make([]string, normalizeStoryChoiceCount(choiceCount))
	for index := range choices {
		choices[index] = fmt.Sprintf("{{next_action_%d}}", index+1)
	}
	var sb strings.Builder
	sb.WriteString("## Submission Parameter Templates\n\n")
	sb.WriteString("Example update for an existing Actor field (submit only fields that actually changed):\n\n```json\n")
	example := map[string]any{
		"state_changes": []map[string]any{{"op": interactivestate.Replace, "actor_id": actorID, "field_id": fieldID, "value": exampleValue}},
		"choices":       choices,
	}
	data, _ := json.MarshalIndent(example, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\n")
	sb.WriteString("When no state changed, submit `\"state_changes\": []`. To create a new Actor, use:\n\n")
	sb.WriteString("`initial_state` must use real Field IDs from the target template. The keys below are type placeholders and must not be copied literally. number, bool, object, and list fields require native JSON values, not quoted strings. Omit a field that has a default when no reliable new fact overrides it.\n\n")
	sb.WriteString("A continuing nonhuman companion also needs one stable character identity. Use character_facts.profiles and the existing matters or knowledge records to retain its observed injury, care, learned cues, cooperation and departure; reuse that identity on a return. Create an Actor only when applicable state fields are needed. A passing wild animal need not be registered. Do not turn an animal's gestures into human speech, invent loyalty or hunger meters, or treat kindness and training as an automatic numerical or cultivation gain.\n\n")
	if ActorStateSystemIsXiuxian(system) {
		// The create example alone did not stop a real-model turn from writing the
		// six stage-derived attributes into initial_state; the whole state_changes
		// module was then refused and the opponent the scene had just met was never
		// created. Said here as well, because a model that reads the example before
		// the field list never reaches the per-field note.
		sb.WriteString("Where a field is marked derived from 当前境界, leave it out entirely. The cultivation rules compute it from the stage, and a create that also states it is refused — which rejects every other change in the same submission, including the Actor itself. State 当前境界 and nothing else about their strength.\n\n")
		// The purse is the one settled field whose opening value the prose may
		// already have stated, and create is the only turn that may write it. The
		// live run showed the cost of leaving this unsaid: the prose counted six
		// 灵石 into a new character's hand, the create omitted the field because
		// its instruction said the rules own it, the default 0 was materialized,
		// and the story and the panel disagreed from that moment on with no later
		// turn able to repair it.
		sb.WriteString("灵石 is settled by the trade rules after the opening, but its opening value is a story fact: if the scene already says how much money this person carries, write that exact count into `initial_state` at create. This is the only turn that may set it, and an omitted 灵石 means they truly carry none rather than that the amount is still unknown. Never invent an amount the scene did not state.\n\n")
	}
	sb.WriteString("`name`, `role` and `description` are the person as the player meets them. `name` is what they are called. `role` is their part in this story, in the story's own words — a shopkeeper, a gate guard, a caravan master — never a template ID; leave it out when the scene has not settled it. `description` is the one or two sentences the player reads on this person's card; it is what the scene actually showed about them, and leaving it out leaves the card with a name and nothing else. Both are optional on the record but are the only place these two facts can be stated.\n\n```json\n")
	create := map[string]any{
		"op":          interactivestate.Create,
		"actor_id":    "{{new_actor_name}}",
		"template_id": "{{template_id}}",
		"name":        "{{new_actor_name}}",
		"role":        "{{their part in this story, in the story's own words}}",
		"description": "{{one or two sentences: who this person is, from what the scene showed}}",
		"initial_state": map[string]any{
			"{{string_field_id}}": "{{text_value}}",
			"{{number_field_id}}": float64(0),
			"{{bool_field_id}}":   false,
			"{{object_field_id}}": map[string]any{},
			"{{list_field_id}}":   []any{},
		},
	}
	data, _ = json.MarshalIndent(create, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\n确认 Actor 死亡或永久离场后才能 archive；仍然存活的归档主体真实归来时才能 restore。含 death_event_id 的正式死亡不能用普通 restore 撤销：\n\n```json\n")
	lifecycle := []map[string]any{
		{"op": interactivestate.Archive, "actor_id": "{{existing_actor_id}}", "reason": "{{confirmed_exit_reason}}"},
		{"op": interactivestate.Restore, "actor_id": "{{archived_actor_id}}", "reason": "{{confirmed_return_reason}}"},
	}
	data, _ = json.MarshalIndent(lifecycle, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\nFor nested object fields, use the optional `subpath` string array. Do not construct path strings manually.")
	return sb.String()
}

func actorStateRuntimeExample(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string) (string, string, any) {
	for _, actorID := range actorIDs {
		record, _ := rawActors[actorID].(map[string]any)
		template := actorStateTemplateByID(system, normalizeActorStateID(fmt.Sprint(record["template_id"])))
		for _, field := range template.Fields {
			return actorID, actorStateFieldID(field), actorStateRuntimeExampleValue(field)
		}
	}
	return "{{actor_id}}", "{{field_id}}", "{{new_value_matching_field_type}}"
}

func actorStateRuntimeExampleValue(field ActorStateField) any {
	if len(field.Options) > 0 {
		return field.Options[0]
	}
	switch field.Type {
	case "number":
		value := float64(1)
		if field.Min != nil {
			value = *field.Min
		}
		if field.Max != nil && value > *field.Max {
			value = *field.Max
		}
		return value
	case "bool":
		return true
	case "list":
		return []any{"{{new_item}}"}
	case "object":
		return map[string]any{"{{key}}": "{{new_value}}"}
	default:
		return "{{new_string_value}}"
	}
}

func actorStateRuntimeConstraints(field ActorStateField) string {
	parts := make([]string, 0, 3)
	if field.Min != nil {
		parts = append(parts, fmt.Sprintf("minimum %v", *field.Min))
	}
	if field.Max != nil {
		parts = append(parts, fmt.Sprintf("maximum %v", *field.Max))
	}
	if len(field.Options) > 0 {
		options := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, actorStateRuntimeCode(option))
		}
		parts = append(parts, "allowed values "+strings.Join(options, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func actorStateRuntimeValue(value any) string {
	if value == nil {
		return "_unset_"
	}
	if text, ok := value.(string); ok {
		return actorStateRuntimeCode(text)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "_unreadable_"
	}
	return actorStateRuntimeCode(string(data))
}

func actorStateRuntimeText(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return trimBytes(value, maxInteractiveTextBytes)
}

func actorStateRuntimeCode(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if strings.Contains(value, "`") {
		return "``" + value + "``"
	}
	return "`" + value + "`"
}

func joinBoundedActorStateRuntimeBlocks(blocks []string, limitBytes int) string {
	if limitBytes <= 0 {
		return ""
	}
	var sb strings.Builder
	truncated := false
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		separator := ""
		if sb.Len() > 0 {
			separator = "\n\n"
		}
		reserve := len([]byte("\n\n" + actorStateRuntimeTruncatedNotice))
		if sb.Len()+len([]byte(separator))+len([]byte(block))+reserve > limitBytes {
			truncated = true
			break
		}
		sb.WriteString(separator)
		sb.WriteString(block)
	}
	if truncated {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(actorStateRuntimeTruncatedNotice)
	}
	if sb.Len() == 0 {
		return trimBytes(actorStateRuntimeTruncatedNotice, limitBytes)
	}
	return sb.String()
}
