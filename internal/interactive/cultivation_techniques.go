package interactive

import (
	"maps"
	"math"
	"sort"
	"strconv"
	"strings"

	"ta/internal/cultivation"
)

// This file makes §4 of 《自制-功法与技能内容》 real during play: a method is
// obtained, actually learned over real elapsed time, then mastered, then
// circulated — and a circulating method is what produces the six attributes.
//
// Before it, 功法 existed only as one free-text field. The settlement read a
// specialisation and a grade out of that prose, which meant exactly one method
// could ever be in effect, no second method could contribute, and a story that
// wrote "他已学会温元诀" changed the character's numbers without any learning
// having happened.
//
// The three facts are kept apart exactly as §4.1 requires:
//
//   - A manual exists. Reading it is prose and grants nothing.
//   - A method is registered. The table is the rules' own, so this is fixed.
//   - A character has mastered it. Only elapsed learning time produces this,
//     and only the learning module writes it.
//
// Circulation is a fourth, separate fact: a mastered method may be circulating
// or stopped, and only circulating methods contribute. That separation is what
// lets a character own a method, pause it, and pick it up again without losing
// what they learned.

// StateOpSourceTechniqueLearning marks an Actor State operation the method rules
// produced while recording mastery or changing circulation.
//
// It is its own source kind because the reroll path drops and re-derives it
// without touching anything the narrative wrote, exactly as it does for skill
// learning and the settlement.
const StateOpSourceTechniqueLearning = "technique_learning"

// TurnTechniqueInput is the model-facing method module of one story turn.
//
// It carries no percentage, no growth figure and no attribute. Those belong to
// the registered definition, which is what makes "不能因正文写学会就直接授予"
// structural: there is nothing here for the prose to put numbers into, and the
// learning amount is real elapsed time rather than an assertion.
type TurnTechniqueInput struct {
	// ActorID is whose method record changes. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"The character whose methods change. Omit for the protagonist."`
	// Learn names methods the character spends this turn's real time learning.
	Learn []TurnTechniqueLearning `json:"learn,omitempty" jsonschema_description:"Methods the character actually studies this turn. Omit when nothing is studied; reading a manual or hearing a method described does not count. When the prose covers time spent on a method, submit those minutes here — the narration alone grants nothing."`
	// Circulate starts a mastered method circulating.
	Circulate []TurnTechniqueSwitch `json:"circulate,omitempty" jsonschema_description:"Mastered methods the character starts sustaining this turn. A method that completes its study this turn counts as mastered. The backend adds one safe 10-minute preparation per changed active set, or one safe 60-minute meditation for warm/cold replacement; never submit the same preparation again as rest. No passive-set switching in battle."`
	// Stop ends a circulating method without losing mastery.
	Stop []TurnTechniqueSwitch `json:"stop,omitempty" jsonschema_description:"Circulating methods the character stops sustaining. The method stays mastered and may be circulated again later."`
}

// TurnTechniqueLearning is one method being studied over this turn's real time.
type TurnTechniqueLearning struct {
	Design *GeneratedTechniqueDesign `json:"design,omitempty" jsonschema_description:"新功法可在模板已有正收益与负面代价预算内重分配六项专长；保留兼修模型、成长、学习时长和境界门槛。"`
	// DefID names a registered method definition.
	DefID      string `json:"def_id,omitempty" jsonschema_description:"The stable identifier of the method, such as method.basic_breathing, method.warm_origin, method.cold_spring, method.origin_guard, method.flowing_wind, method.clear_mind, method.body_cycle or method.economical_breath."`
	TemplateID string `json:"template_id,omitempty" jsonschema_description:"新修炼功法的同效模板，如 method.basic_breathing、method.warm_origin、method.cold_spring、method.origin_guard、method.flowing_wind、method.clear_mind、method.body_cycle、method.economical_breath。续修模板 method.core_continuation、method.nascent_continuation、method.spirit_continuation、method.void_unification、method.integration_return、method.mahayana_insight 分别从前一境后期支持至结丹、元婴、化神、炼虚、合体、大乘后期。与def_id二选一；学习时间、兼修条件与成长沿模板；可用design在原预算内调整实际专长。"`
	Name       string `json:"name,omitempty" jsonschema_description:"新功法名称，配合template_id及实际source使用。登记不等于学会，仍须投入真实研习时间。"`
	// Version is the definition revision. Omit for the registered one.
	Version int `json:"version,omitempty" jsonschema_description:"The method version being studied. Omit for the registered version."`
	// Minutes is the real elapsed time spent studying this method this turn.
	// It is the only thing that produces progress: the design's formula is
	// actual minutes × aptitude ÷ the method's standard minutes.
	Minutes float64 `json:"minutes,omitempty" jsonschema_description:"Real in-fiction minutes spent studying this method this turn. Six one-hour sessions and one six-hour session are the same amount. Never invent time the player did not spend."`
	// Source records where the method came from in the story — an elder, a cave
	// wall, a family copy. It is prose for the journal and is never priced.
	Source string `json:"source,omitempty" jsonschema_description:"Actual complete readable material or effective teaching for this method. Required before first positive study unless its source is already recorded; registration alone grants no progress. A partial manual missing essential content is not effective teaching."`
	// Description is what the method feels like when it runs, in the story's own
	// words, with a little wit where it fits. It is journal material: the effect,
	// the cost and every number stay on the registered definition, so a vivid
	// sentence cannot buy an unregistered benefit. Written once, when the method is
	// first taken up.
	Description string `json:"description,omitempty" jsonschema_description:"Optional. One or two sentences on what practising this method looks or feels like in the story's own words. Do not restate its effect, cost or numbers — those come from the definition. Omit for an unremarkable method."`
}

// TurnTechniqueSwitch is one circulation change.
type TurnTechniqueSwitch struct {
	DefID   string `json:"def_id" jsonschema_description:"The stable identifier of a method this character has already mastered."`
	Version int    `json:"version,omitempty" jsonschema_description:"The mastered version. Omit for the registered version."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnTechniqueInput) Empty() bool {
	return len(input.Learn) == 0 && len(input.Circulate) == 0 && len(input.Stop) == 0
}

// cloneTurnTechniqueInput copies a method module so an accepted one can be
// retained in a draft without the caller's slices being aliased.
func cloneTurnTechniqueInput(input *TurnTechniqueInput) *TurnTechniqueInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Learn = append([]TurnTechniqueLearning(nil), input.Learn...)
	for i, request := range cloned.Learn {
		if request.Design != nil {
			design := *request.Design
			design.Specialty = maps.Clone(design.Specialty)
			cloned.Learn[i].Design = &design
		}
	}
	cloned.Circulate = append([]TurnTechniqueSwitch(nil), input.Circulate...)
	cloned.Stop = append([]TurnTechniqueSwitch(nil), input.Stop...)
	return &cloned
}

// validateTurnTechniqueModule rejects a module the rules could not act on.
//
// It checks shape, not outcome. Whether a method exists, whether the character
// qualifies for it, and whether two models may run together are the rules'
// answers and are reported in the result, so the player reads a real reason
// rather than a submission error. What must not pass is an empty definition id,
// a negative version or a negative study time.
func validateTurnTechniqueModule(input *TurnTechniqueInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	for index, item := range input.Learn {
		if (strings.TrimSpace(item.DefID) == "") == (strings.TrimSpace(item.TemplateID) == "") || (item.TemplateID != "" && (strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Source) == "")) {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTechniques, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/techniques/learn/"+strconv.Itoa(index)+"/def_id", "a registered method definition identifier",
				"", "Each studied method must name a registered definition.",
			)
		}
		if item.Version < 0 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTechniques, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/techniques/learn/"+strconv.Itoa(index)+"/version", "a positive version, or omitted for the registered one",
				strconv.Itoa(item.Version), "A method version must be positive, or omitted to use the registered version.",
			)
		}
		if item.Minutes < 0 || math.IsNaN(item.Minutes) || math.IsInf(item.Minutes, 0) || item.Minutes >= float64(math.MaxInt64)/60 {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTechniques, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/techniques/learn/"+strconv.Itoa(index)+"/minutes", "a finite, non-negative number of minutes",
				strconv.FormatFloat(item.Minutes, 'g', -1, 64), "Study time must be a finite, non-negative number of real minutes.",
			)
		}
	}
	for index, item := range append(append([]TurnTechniqueSwitch(nil), input.Circulate...), input.Stop...) {
		if strings.TrimSpace(item.DefID) == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleTechniques, intPointer(index), TurnSubmissionDiagnosticInvalidModule,
				"/techniques/circulate", "a method definition identifier", "",
				"A circulation change must name a method.",
			)
		}
	}
	return nil
}

// TechniqueResult reports what one turn's method module did, in player-readable
// terms.
type TechniqueResult struct {
	// ActorID is the character the change applied to.
	ActorID string
	// Learned lists methods that completed this turn.
	Learned []cultivation.TechniqueDef
	// Studied lists methods that made progress but are not mastered yet.
	Studied []TechniqueStudy
	// Started lists methods that began circulating.
	Started []cultivation.TechniqueDef
	// Stopped lists methods that stopped circulating.
	Stopped []cultivation.TechniqueDef
	// Refused lists changes that were not applied, with the reason.
	Refused []TechniqueRefusal
}

// TechniqueStudy is one method's progress after a study interval.
type TechniqueStudy struct {
	DefID string
	// Progress is the fraction of the method learned, 0–1.
	Progress float64
	// Mastered is true when this interval completed it.
	Mastered bool
}

// TechniqueRefusal is one change that was not applied, with the reason the
// player needs to read.
type TechniqueRefusal struct {
	DefID  string
	Action string
	Reason string
}

// techniqueRecord is the stored shape of one method entry.
//
// The record keeps the definition and version rather than the numbers, so the
// attributes are always priced from the rules' table at the version the
// character learned. Editing this record cannot inflate a method.
type techniqueRecord struct {
	DefID            string
	Name             string
	SwitchPending    bool
	StoppedAtSeconds int64
	Version          int
	Mastered         bool
	Minutes          float64
	Active           bool
	RefusedBy        string
	// Description and Source are the method's own account of itself: what it
	// feels like to run and where it came from. They are journal material and
	// nothing prices them; they are carried here so a later turn writes them back
	// unchanged instead of dropping what the player already read.
	Description string
	Source      string
}

// readTechniqueRecords reads the method record out of an actor's state.
//
// The record is keyed by method name for the player to read, so it is walked in
// sorted key order: a replay must produce the same set of circulating methods,
// and therefore the same settled numbers, every time.
func readTechniqueRecords(state map[string]any, actorID string) (map[string]techniqueRecord, bool) {
	// The field's presence is the question, not whether it holds anything: an
	// empty record means the character has a method record and is circulating
	// nothing, which must not fall back to the free-text template. Reading it
	// through xiuxianEquipmentEntry would collapse the two, because that helper
	// reports an empty map as absent.
	raw := actorStateFieldValue(state, actorID, xiuxianFieldTechniques)
	entry, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	records := make(map[string]techniqueRecord, len(entry))
	for _, key := range sortedKeys(entry) {
		value, ok := entry[key].(map[string]any)
		if !ok {
			continue
		}
		defID := strings.TrimSpace(xiuxianEntryText(value, "定义"))
		if defID == "" {
			continue
		}
		record := techniqueRecord{DefID: defID, Name: xiuxianEntryText(value, "名称")}
		record.SwitchPending, _ = value["待平息"].(bool)
		if at, ok := actorStateNumber(value["停运秒"]); ok {
			record.StoppedAtSeconds = int64(at)
		}
		if raw, ok := actorStateNumber(value["版本"]); ok {
			record.Version = int(raw)
		}
		record.Mastered = strings.TrimSpace(xiuxianEntryText(value, "掌握程度")) == "已掌握"
		if raw, ok := actorStateNumber(value["已修习分钟"]); ok && raw > 0 {
			record.Minutes = raw
		}
		record.Active = strings.TrimSpace(xiuxianEntryText(value, "运转")) == "运转"
		record.Description = xiuxianEntryText(value, "描述")
		record.Source = xiuxianEntryText(value, "来历")
		records[key] = record
	}
	return records, true
}

// circulatingTechniques resolves the circulating methods to definitions.
//
// A record whose definition the rules do not hold is skipped rather than
// substituted: the design forbids silently rewriting what a character knows, and
// a branch that diverged before a definition existed keeps the honest answer.
func circulatingTechniques(cultivationRules *cultivation.Rules, records map[string]techniqueRecord, states ...map[string]any) []cultivation.TechniqueDef {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	active := make([]cultivation.TechniqueDef, 0, len(keys))
	for _, key := range keys {
		record := records[key]
		if !record.Active || !record.Mastered {
			continue
		}
		definition, ok := resolveTechnique(cultivationRules,
			record.DefID, record.Version, states...)
		if !ok {
			continue
		}
		active = append(active, definition)
	}
	return active
}

// xiuxianTechniqueOps applies one turn's method module and returns the writes
// that record it.
//
// Ordering inside the module is deliberate: study first, then stops, then
// starts. A character who studies a method and starts circulating it in the same
// turn circulates the version this turn produced, and a method being stopped is
// removed from the active set before a conflicting one is checked against it —
// which is what makes the design's "stop the old one, then start the new one"
// expressible without a separate turn.
func xiuxianTechniqueOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	input TurnTechniqueInput,
	turnID string,
) ([]ActorStateOp, TechniqueResult, bool) {
	result := TechniqueResult{ActorID: normalizeStatePanelActorID(actorID)}
	if result.ActorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, result, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, result.ActorID)
	if !found {
		return nil, result, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, result, false
	}
	if !actorStateTemplateDeclaresField(template, xiuxianFieldTechniques) {
		// A template without the field cannot record methods, and inventing the
		// field would put a value in a schema that never declared it.
		return nil, result, false
	}
	if input.Empty() {
		return nil, result, true
	}

	records, _ := readTechniqueRecords(state, result.ActorID)
	if records == nil {
		// An absent record is the normal first-study case, not an error: the
		// field is declared but empty, so there is nothing to read yet.
		records = map[string]techniqueRecord{}
	}
	next := make(map[string]any, len(records)+len(input.Learn))
	displayNames := make(map[string]string, len(records)+len(input.Learn))
	if entry, ok := xiuxianEquipmentEntry(state, result.ActorID, xiuxianFieldTechniques); ok {
		for key, value := range entry {
			if entry, ok := value.(map[string]any); ok {
				next[key] = cloneStoryState(entry)
			} else {
				next[key] = value
			}
			displayNames[key] = key
		}
	}

	registryRecord, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	registryRecord = maps.Clone(registryRecord)
	if registryRecord == nil {
		registryRecord = map[string]any{}
	}
	definitionsChanged := false
	definitionState := cloneStoryState(state)
	stage := xiuxianStoredStageKey(state, result.ActorID)
	characterStage, stageOK := cultivation.ParseStageKey(stage, system.CultivationRules)
	aptitude := xiuxianTalentFromState(state, result.ActorID)
	changed := false

	// Study. Only real elapsed time produces progress, and a method completes
	// only when the accumulated minutes reach the aptitude-adjusted requirement.
	for _, request := range input.Learn {
		defID := strings.TrimSpace(request.DefID)
		definition, generated, ok := techniqueLearningDefinition(system.CultivationRules,
			definitionState, registryRecord, request)
		if !ok {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "learn", Reason: "this method is not registered"})
			continue
		}
		key := techniqueRecordKey(next, definition)
		record := records[key]
		record.DefID = definition.ID
		record.Name = definition.Name
		record.Version = definition.Version
		// The wording is the method's first impression and is written once: a later
		// session, or a replayed commit, keeps the sentence the player already read
		// rather than restyling it under them.
		if record.Description == "" {
			record.Description = strings.TrimSpace(request.Description)
		}
		if record.Source == "" {
			record.Source = strings.TrimSpace(request.Source)
		}
		if request.Minutes > 0 {
			if !stageOK {
				result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "learn", Reason: "the character's recorded stage cannot be read"})
				continue
			}
			if !cultivation.CanUseTechniques(characterStage) {
				result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "learn", Reason: "a mortal cannot cultivate a method"})
				continue
			}
			if !stageReaches(characterStage, definition.MinimumStage) {
				result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "learn", Reason: "the character is below this method's minimum stage"})
				continue
			}
			if !record.Mastered && strings.TrimSpace(record.Source) == "" {
				result.Refused = append(result.Refused, TechniqueRefusal{DefID: definition.ID, Action: "learn", Reason: "study requires complete readable material or effective instruction recorded as its source"})
				continue
			}
			if !record.Mastered && !cultivationManualHeld(state, result.ActorID, definition.RequiredManualID) {
				result.Refused = append(result.Refused, TechniqueRefusal{DefID: definition.ID, Action: "learn", Reason: "尚未持有这门传承的完整实物注本；说明来历不能代替取得。"})
				continue
			}
			required := cultivation.LearningMinutesToComplete(aptitude, definition.StandardLearningMinutes, system.CultivationRules)
			if !record.Mastered {
				record.Minutes = min(required, record.Minutes+request.Minutes)
			}
			if required > 0 && record.Minutes >= required {
				record.Mastered = true
			}
			result.Studied = append(result.Studied, TechniqueStudy{
				DefID:    definition.ID,
				Progress: cultivation.LearningProgress(record.Minutes, aptitude, definition.StandardLearningMinutes, system.CultivationRules),
				Mastered: record.Mastered,
			})
		}
		if record.Mastered {
			result.Learned = append(result.Learned, definition)
		}
		if generated != nil && registryRecord[definition.ID] == nil {
			registryRecord[definition.ID] = generated
			definitionsChanged = true
			applyActorStateOp(definitionState, generatedDefinitionOp(registryRecord, turnID, StateOpSourceTechniqueLearning))
		}
		next[key] = techniqueRecordValue(record)
		displayNames[key] = definition.Name
		records[key] = record
		changed = true
	}

	// Stop before start, so a replacement within one turn does not collide with
	// the method it replaces.
	for _, request := range input.Stop {
		defID := strings.TrimSpace(request.DefID)
		key := techniqueRecordKey(next, cultivation.TechniqueDef{ID: defID})
		record, ok := records[key]
		if request.Version < 0 || (request.Version > 0 && request.Version != record.Version) {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "stop", Reason: "the requested method version is not the learned version"})
			continue
		}
		if !ok || !record.Active {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "stop", Reason: "this method is not circulating"})
			continue
		}
		record.Active = false
		record.SwitchPending = true
		records[key] = record
		next[key] = techniqueRecordValue(record)
		if definition, ok := resolveTechnique(system.CultivationRules,
			record.DefID, record.Version, definitionState); ok {
			result.Stopped = append(result.Stopped, definition)
		}
		changed = true
	}

	for _, request := range input.Circulate {
		defID := strings.TrimSpace(request.DefID)
		key := techniqueRecordKey(next, cultivation.TechniqueDef{ID: defID})
		record, ok := records[key]
		if request.Version < 0 || (request.Version > 0 && request.Version != record.Version) {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "circulate", Reason: "the requested method version is not the learned version"})
			continue
		}
		if !ok || !record.Mastered {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "circulate", Reason: "this method has not been mastered"})
			continue
		}
		if record.Active {
			continue
		}
		definition, ok := resolveTechnique(system.CultivationRules,
			record.DefID, record.Version, definitionState)
		if !ok {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "circulate", Reason: "this method is not registered"})
			continue
		}
		if reason := circulationConflict(system.CultivationRules,
			definition, records, result.ActorID, definitionState); reason != "" {
			result.Refused = append(result.Refused, TechniqueRefusal{DefID: defID, Action: "circulate", Reason: reason})
			continue
		}
		record.Active = true
		records[key] = record
		next[key] = techniqueRecordValue(record)
		result.Started = append(result.Started, definition)
		changed = true
	}

	if !changed {
		return nil, result, true
	}
	ops := []ActorStateOp{{
		Op:           "set",
		ActorID:      result.ActorID,
		FieldID:      normalizeActorStateFieldName(xiuxianFieldTechniques),
		Value:        next,
		Reason:       "由功法学习与运转结算写入，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceTechniqueLearning,
		SourceID:     "techniques",
	}}
	if definitionsChanged {
		ops = append([]ActorStateOp{generatedDefinitionOp(registryRecord, turnID, StateOpSourceTechniqueLearning)}, ops...)
	}
	return ops, result, true
}

// circulationConflict reports why a method may not start circulating, or an
// empty string when it may.
//
// It reads the actor's real traits and realm, so the one conditional exception in
// the published matrix — split channels plus Foundation — is answered from the
// character's actual state rather than from what the prose claimed.
func circulationConflict(cultivationRules *cultivation.Rules, definition cultivation.TechniqueDef, records map[string]techniqueRecord, actorID string, state map[string]any) string {
	realm := cultivation.RealmMortal
	if stage, ok := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), cultivationRules); ok {
		realm = stage.Realm
	}
	hasSplitChannels := actorHasSplitChannels(state, actorID)
	for _, record := range records {
		if !record.Active || !record.Mastered || record.DefID == definition.ID {
			continue
		}
		other, ok := resolveTechnique(cultivationRules,
			record.DefID, record.Version, state)
		if !ok {
			continue
		}
		if err := cultivation.CheckCirculationCompatibility(definition.Circulation, other.Circulation, realm, hasSplitChannels); err != nil {
			return "these circulation models cannot run together"
		}
	}
	return ""
}

// actorHasSplitChannels reads the innate trait that permits warm and cold to
// run together.
func actorHasSplitChannels(state map[string]any, actorID string) bool {
	for _, trait := range actorTraitInstancesFromState(state, actorID) {
		if strings.TrimSpace(trait.TraitID) == cultivation.TraitSplitChannels {
			return true
		}
	}
	return false
}

// resolveTechnique resolves a requested method to the rules' definition.
func resolveTechnique(cultivationRules *cultivation.Rules, defID string, version int, states ...map[string]any) (cultivation.TechniqueDef, bool) {
	definition, ok := cultivation.TechniqueByID(defID, cultivationRules)
	if !ok && len(states) > 0 {
		records, _ := xiuxianEquipmentEntry(states[0], DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
		record, _ := records[defID].(map[string]any)
		definition, ok = generatedTechniqueFromRecord(cultivationRules,
			record)
	}
	if !ok {
		return cultivation.TechniqueDef{}, false
	}
	if version > 0 && version != definition.Version {
		return cultivation.TechniqueDef{}, false
	}
	return definition, true
}

// techniqueRecordKey returns the key an entry is stored under.
//
// The record is keyed by the method's name so the player reads a name rather
// than an id, but the key must stay stable across turns. An existing entry for
// the same definition is reused, and a new one is added under its name; two
// methods that happen to share a name are kept apart by the definition id in
// their value, which is what the reader actually resolves.
func techniqueRecordKey(existing map[string]any, definition cultivation.TechniqueDef) string {
	for key, value := range existing {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(xiuxianEntryText(entry, "定义")) == definition.ID {
			return key
		}
	}
	if definition.Name != "" {
		if existing[definition.Name] == nil {
			return definition.Name
		}
		return definition.Name + " [" + definition.ID + "]"
	}
	return definition.ID
}

// techniqueRecordValue renders one record in the stored shape.
func techniqueRecordValue(record techniqueRecord) map[string]any {
	value := map[string]any{
		"定义":   record.DefID,
		"版本":   float64(record.Version),
		"掌握程度": "修习中",
		"运转":   "停止",
	}
	if record.Name != "" {
		value["名称"] = record.Name
	}
	if record.SwitchPending {
		value["待平息"] = true
		value["停运秒"] = float64(record.StoppedAtSeconds)
	}
	if record.Mastered {
		value["掌握程度"] = "已掌握"
	}
	if record.Active {
		value["运转"] = "运转"
	}
	if record.Minutes > 0 {
		value["已修习分钟"] = record.Minutes
	}
	if strings.TrimSpace(record.Description) != "" {
		value["描述"] = strings.TrimSpace(record.Description)
	}
	if strings.TrimSpace(record.Source) != "" {
		value["来历"] = strings.TrimSpace(record.Source)
	}
	return value
}

// stageReaches reports whether a character's stage is at or above a minimum
// canonical stage key.
//
// It reuses the rules' own ordering by comparing the stage keys' positions on
// the published table, so a method's minimum is read the same way the stage
// table reads it.
func stageReaches(current cultivation.Cultivation, minimumKey string) bool {
	minimum, ok := cultivation.ParseStageKey(minimumKey, current.Rules())
	if !ok {
		return false
	}
	order := cultivation.StageKeys()
	if current.Rules() != nil {
		order = current.Rules().StageOrder
	}
	position := func(value cultivation.Cultivation) int {
		key := cultivation.StageKey(value)
		for index, candidate := range order {
			if candidate == key {
				return index
			}
		}
		return -1
	}
	currentIndex, minimumIndex := position(current), position(minimum)
	if currentIndex < 0 || minimumIndex < 0 {
		return false
	}
	return currentIndex >= minimumIndex
}

// removeTechniqueLearningActorOps drops a previous method module's writes.
func removeTechniqueLearningActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceTechniqueLearning {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}
