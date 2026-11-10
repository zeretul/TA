package interactive

import (
	"math"
	"sort"
	"strings"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// StateOpSourceCultivationSettlement marks an Actor State operation the
// cultivation rules produced.
//
// The source is recorded rather than implied so a reader of the journal can
// tell a settled number from a narrated one, which is the whole point of the
// settlement owning the field.
const StateOpSourceCultivationSettlement = "cultivation_settlement"

// xiuxianFieldMethod is the free-text field that records the character's
// primary technique together with its specialisation and grade.
const xiuxianFieldMethod = "功法与专精"

// settleXiuxianActorState derives a character's attributes from the stage they
// are actually at and returns the derived numbers, with the clamped current
// pools, as Actor State operations.
//
// It is the one code path that produces these numbers during play, so the panel,
// the battle round and the offline simulation cannot disagree about them. The
// maxima are recomputed rather than stored: the stage key is the only input that
// decides them, and a second stored copy would be a number the narrative could
// drift away from. That is what makes the design's "the model may not reword the
// character's cultivation" requirement hold structurally — there is no stored
// number to reword.
//
// The current pools are written too, but they are read first and only trimmed to
// the derived maximum. A wound is a real fact the stage table cannot
// reconstruct, so the settlement's job there is to clamp, not to author. A pool
// that has never been written means no wound has been recorded, so it starts
// full rather than at zero.
//
// It reports false for an actor the rules do not track, which is the correct
// answer for a character outside the cultivation system rather than a reason to
// zero someone's health.
func settleXiuxianActorState(state map[string]any, system StoryDirectorActorStateSystem, actorID, turnID string) ([]ActorStateOp, bool) {
	actorID = normalizeStatePanelActorID(actorID)
	if actorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return nil, false
	}
	template := actorStateTemplateByID(system, templateID)
	// Only an actor whose own template carries the stage field is on the rules'
	// table. A bystander created under a template without one is not a
	// cultivation-tracked character, and writing numbers into them would invent
	// fields their template never declared.
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, false
	}
	character, err := xiuxianCharacterFromState(system.CultivationRules,
		state, actorID)
	if err != nil {
		// The stored stage is not one the rules can read. Refusing to write is
		// what keeps a bad key visible to the player instead of silently
		// resetting them to a mortal with a mortal's numbers.
		return nil, false
	}

	// Reading Attributes() once with the stored pools clamps them to the current
	// maxima, so the maxima are known before the pools are defaulted. A character
	// who has never been wounded therefore starts at the real maximum rather than
	// at zero, and one whose maximum fell keeps only what still fits.
	attributes := character.Attributes()
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldHealth)); !ok {
		character.Health = attributes.HealthMax
	}
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldMana)); !ok {
		character.Mana = attributes.ManaMax
	}
	attributes = character.Attributes()

	ops := make([]ActorStateOp, 0, len(cultivation.AllAttributes)+2)
	for _, attribute := range cultivation.AllAttributes {
		fieldID, ok := xiuxianFieldNameForAttribute(attribute)
		if !ok || !actorStateTemplateDeclaresField(template, fieldID) {
			continue
		}
		ops = append(ops, xiuxianSetOp(actorID, fieldID, xiuxianAttributeValue(attributes, attribute), turnID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldHealth) {
		ops = append(ops, xiuxianSetOp(actorID, xiuxianFieldHealth, attributes.Health, turnID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldMana) {
		ops = append(ops, xiuxianSetOp(actorID, xiuxianFieldMana, attributes.Mana, turnID))
	}
	return xiuxianChangedOps(state, actorID, ops), true
}

// actorStateTemplateDeclaresField reports whether a template carries a field.
func actorStateTemplateDeclaresField(template ActorStateTemplate, fieldID string) bool {
	_, ok := actorStateFieldByID(template, fieldID)
	return ok
}

// xiuxianChangedOps drops writes that would not change anything.
//
// The settlement runs on every turn, but a character who did not cultivate,
// fight, or heal has the same numbers they had before. Writing them again would
// put eight redundant entries in the journal per actor per turn, and the journal
// is the record the player can read and branch from. Emitting only real changes
// keeps that record honest about what actually moved.
func xiuxianChangedOps(state map[string]any, actorID string, ops []ActorStateOp) []ActorStateOp {
	changed := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		next, ok := actorStateNumber(op.Value)
		if !ok {
			changed = append(changed, op)
			continue
		}
		current, ok := actorStateNumber(actorStateFieldValue(state, actorID, op.FieldID))
		// Progress is fractional. Comparing rounded values would consume a
		// settled interval while silently discarding its earned progress.
		if ok && current == next {
			continue
		}
		changed = append(changed, op)
	}
	return changed
}

// removeCultivationSettlementActorOps drops a previous settlement's writes.
//
// It mirrors removeRuleResolutionActorOps for the settlement's own source kind.
// A reroll has to discard both, because the settlement was derived from the rule
// outcome being replaced.
func removeCultivationSettlementActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceCultivationSettlement {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}

// settleXiuxianTurnState settles the cast against the state this turn produced.
//
// state must be the pre-turn state, with none of ops or actorOps applied to it.
// The turn's operations are projected onto a copy here, so passing a map that
// already carries some of them would apply those a second time. Rule consumption
// is the reason the distinction matters: it writes its own operations into the
// live state in place, so callers snapshot the state before invoking it and hand
// that snapshot here.
//
// Settling against the pre-turn state would publish the attributes of the stage
// the character was at before the turn, alongside the stage key the turn just
// advanced them to — the panel would show a foundation-establishment key with
// qi-refining numbers.
func settleXiuxianTurnState(state map[string]any, system StoryDirectorActorStateSystem, ops []interactivestate.Op, actorOps []ActorStateOp, turnID string) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	projected := cloneStoryState(state)
	for _, op := range ops {
		applyStateOp(projected, op)
	}
	for _, op := range actorOps {
		applyActorStateOp(projected, op)
	}
	return settleXiuxianStoryState(projected, system, turnID)
}

// settleXiuxianCreationState settles the cast at the moment the story is
// created, before any turn exists.
//
// Without it the six settled attributes and the two maxima are still at the
// template's zero defaults when the player first looks at the panel: the
// settlement only ran per turn, so a story with no turn yet showed a
// qi-refining protagonist with 0 mana, 0 health and 0 in every attribute. The
// design requires the pre-opening snapshot to carry the real numbers, because
// otherwise the model's first scene is written against a character who reads as
// a mortal with nothing, and the panel contradicts the prose.
//
// The pools are the reason this cannot simply be "run the settlement later": a
// pool that is unwritten means "no wound recorded" and settles to its maximum,
// which is exactly the fact a new character needs.
func settleXiuxianCreationState(system StoryDirectorActorStateSystem, ops []interactivestate.Op, actorOps []ActorStateOp) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	projected := initialStoryState()
	for _, op := range ops {
		applyStateOp(projected, op)
	}
	for _, op := range actorOps {
		applyActorStateOp(projected, op)
	}
	return settleXiuxianStoryState(projected, system, "")
}

// settleXiuxianStoryState settles every tracked actor on the current branch.
//
// The whole cast is settled rather than only the protagonist because the rules
// are per character: an opponent's attributes come from their own stage, and
// deriving them in the same pass is what stops one actor's numbers from being
// computed by a different rule path than another's.
func settleXiuxianStoryState(state map[string]any, system StoryDirectorActorStateSystem, turnID string) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	actors, _ := state[actorStateRoot].(map[string]any)
	if len(actors) == 0 {
		return nil
	}
	ids := make([]string, 0, len(actors))
	for actorID := range actors {
		ids = append(ids, actorID)
	}
	// Sorted so the committed operation order is stable across replays and
	// across map iteration, which is what lets two runs of the same branch
	// produce the same journal.
	sort.Strings(ids)

	ops := make([]ActorStateOp, 0, len(ids)*(len(cultivation.AllAttributes)+2))
	for _, actorID := range ids {
		settled, ok := settleXiuxianActorState(state, system, actorID, turnID)
		if !ok {
			continue
		}
		ops = append(ops, settled...)
	}
	return normalizeActorStateOps(ops)
}

// xiuxianCharacterFromState reads the character's rules position out of state.
//
// The stage key is read through xiuxianStoredStageKey so an unwritten stage is
// the rules' own starting point rather than an error.
//
// The real, sourced extras — the equipped implement, the worn armour, the
// deployed formation and the lasting boosts — are read here and passed to the
// rules, so the aggregation of §3.4 sums every effective source in one pass. It
// has to happen at this point rather than at display time: the panel, the battle
// round and the simulation all read the settled fields, and a bonus applied
// anywhere else would be a second, disagreeing total.
func xiuxianCharacterFromState(cultivationRules *cultivation.Rules, state map[string]any, actorID string) (cultivation.CharacterState, error) {
	method, _ := actorStateFieldValue(state, actorID, xiuxianFieldMethod).(string)
	specialisation, _ := normalizeCultivationSpecialisationText(method)
	quality, _ := normalizeCultivationQualityText(method)
	character, err := cultivation.CharacterStateFromRecord(
		xiuxianStoredStageKey(state, actorID),
		xiuxianStoredInt(state, actorID, xiuxianFieldHealth),
		xiuxianStoredInt(state, actorID, xiuxianFieldMana),
		xiuxianStoredInt(state, actorID, xiuxianFieldBurden),
		xiuxianStoredInt(state, actorID, xiuxianFieldBurdenFloor),
		xiuxianStoredFloat(state, actorID, xiuxianFieldRealmProgress),
		string(specialisation),
		string(quality),
		xiuxianEquipmentModifiers(cultivationRules,
			state, actorID),
	)
	if err != nil {
		return character, err
	}
	if cultivationRules != nil {
		if _, exists := cultivationRules.Stages[cultivation.StageKey(character.Cultivation)]; !exists {
			return character, &cultivation.UnknownStageKeyError{Key: cultivation.StageKey(character.Cultivation)}
		}
	}
	character.Cultivation = character.Cultivation.WithRules(cultivationRules)
	// A structured method record is authoritative once it holds a method. The
	// free-text specialisation is what a character whose record predates methods
	// is priced from, and it is read above; applying both would count the same
	// method twice.
	//
	// The field itself is always present — the preset declares it with an empty
	// default — so presence alone cannot be the test, or every new character
	// would silently lose the prose they were created with. A record that holds
	// entries but circulates nothing is a character who deliberately stopped
	// every method: the entries stay in the record, so the distinction survives.
	if records, ok := readTechniqueRecords(state, actorID); ok && len(records) > 0 {
		character = character.WithTechniques(circulatingTechniques(cultivationRules,
			records, state))
	}
	return character, nil
}

// xiuxianStoredStageKey reads the stage, treating an unwritten stage as the
// rules' own starting point.
//
// An unwritten field is not an error: the preset declares mortal as the default
// and a story may not have written it yet, so reading it as mortal is the same
// answer the schema itself would give.
func xiuxianStoredStageKey(state map[string]any, actorID string) string {
	if raw, ok := actorStateFieldValue(state, actorID, xiuxianFieldRealm).(string); ok {
		if key := strings.TrimSpace(raw); key != "" {
			return key
		}
	}
	return cultivation.StageKey(cultivation.Mortal())
}

func xiuxianStoredInt(state map[string]any, actorID, fieldID string) int {
	if value, ok := actorStateNumber(actorStateFieldValue(state, actorID, fieldID)); ok {
		return int(math.Round(value))
	}
	return 0
}

func xiuxianStoredFloat(state map[string]any, actorID, fieldID string) float64 {
	if value, ok := actorStateNumber(actorStateFieldValue(state, actorID, fieldID)); ok {
		return value
	}
	return 0
}

// xiuxianSetOp builds one settlement write.
//
// Every settled field is written as an absolute value rather than a delta: the
// number is derived from the stage, so a delta would accumulate the difference
// between two derivations and drift away from the rules.
func xiuxianSetOp(actorID, fieldID string, value any, turnID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       "由境界阶段表与有效加成结算，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceCultivationSettlement,
		SourceID:     "cultivation",
	}
}

// xiuxianAttributeValue returns the settled number for one rules attribute.
//
// The switch is exhaustive over the six attributes the rules define, so a new
// attribute is a compile-time absence here rather than a silently settled zero.
func xiuxianAttributeValue(attributes cultivation.Attributes, attribute cultivation.AttributeName) int {
	switch attribute {
	case cultivation.AttrHealthMax:
		return attributes.HealthMax
	case cultivation.AttrManaMax:
		return attributes.ManaMax
	case cultivation.AttrAttack:
		return attributes.Attack
	case cultivation.AttrDefense:
		return attributes.Defense
	case cultivation.AttrAgility:
		return attributes.Agility
	case cultivation.AttrPerception:
		return attributes.Perception
	}
	return 0
}
