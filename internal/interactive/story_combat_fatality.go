package interactive

import (
	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// CombatLethalIntent is the source-bound intent frozen into the battle opening.
// It is story evidence, not a combat rule: only ResolveBattleRound can turn it
// into a fatal result after the engine records a real positive-to-zero hit.
type CombatLethalIntent struct {
	ActorID       string `json:"actor_id"`
	TargetID      string `json:"target_id"`
	SourceTurnID  string `json:"source_turn_id"`
	SourceEventID string `json:"source_event_id"`
}

// PlayerCombatIntent is an explicit choice attached to one submitted battle
// action. Narrative text and model-authored state updates cannot grant this
// authority; only the combat window's formal action request can.
type PlayerCombatIntent string

const (
	PlayerCombatIntentSpare    PlayerCombatIntent = "spare"
	PlayerCombatIntentTakeLife PlayerCombatIntent = "take_life"
)

// CombatFatality is the deterministic aftermath of one qualifying engine hit.
type CombatFatality struct {
	ActorID       string `json:"actor_id"`
	KillerActorID string `json:"killer_actor_id"`
	SourceTurnID  string `json:"source_turn_id"`
	SourceEventID string `json:"source_event_id"`
	Round         int    `json:"round"`
	StepIndex     int    `json:"step_index"`
}

type combatLethalIntentDeclaration struct {
	ActorID             string `json:"actor_id"`
	TargetID            string `json:"target_id"`
	SourceExchangeIndex int    `json:"source_exchange_index"`
}

func freezeCombatLethalIntents(lines []StoryEventRecord, head string, session cultivation.CombatSession) []CombatLethalIntent {
	path, _, err := walkEventPath(head, eventsByID(lines))
	if err != nil {
		return nil
	}
	participants := make(map[string]cultivation.CombatSide, len(session.Combatants))
	for _, combatant := range session.Combatants {
		participants[combatant.ID] = combatant.Side
	}

	for index := len(path) - 1; index >= 0; index-- {
		if path[index].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if mapToStruct(path[index].Raw, &turn) != nil || turn.StateDelta == nil {
			continue
		}
		value, source := lethalIntentValueFromTurn(turn)
		if source == "" {
			continue
		}
		declarations := decodeCombatLethalIntentDeclarations(value)
		if len(declarations) == 0 {
			return nil
		}
		frozen := make([]CombatLethalIntent, 0, len(declarations))
		seen := map[string]bool{}
		for _, declaration := range declarations {
			actorID := normalizeStatePanelActorID(declaration.ActorID)
			targetID := normalizeStatePanelActorID(declaration.TargetID)
			actorSide, actorPresent := participants[actorID]
			targetSide, targetPresent := participants[targetID]
			if !actorPresent || !targetPresent || actorID == targetID || actorSide == targetSide {
				continue
			}
			key := actorID + "\x00" + targetID
			if seen[key] || !combatLethalIntentHasPublicExchange(path, turn, declaration.SourceExchangeIndex, actorID, targetID) {
				continue
			}
			seen[key] = true
			frozen = append(frozen, CombatLethalIntent{
				ActorID: actorID, TargetID: targetID,
				SourceTurnID: source, SourceEventID: turn.ID,
			})
		}
		return frozen
	}
	return nil
}

// combatLethalIntentHasPublicExchange decides whether one declaration is backed
// by the committed utterance it cites. A citation alone is not evidence: the
// index must resolve to a public scene line spoken by the actor itself, aimed at
// the named target, and — for the player — carrying the player's own action
// rather than a line the model wrote on the player's behalf.
//
// The direction check is what stops a bystander threat from being read as a
// threat against the protagonist: for a character's own line CharacterID names
// the speaker, so only AddressedTo can say who the words were aimed at.
func combatLethalIntentHasPublicExchange(path []StoryEventRecord, turn TurnEvent, sourceIndex int, actorID, targetID string) bool {
	if sourceIndex < 1 {
		return false
	}
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCharacterRecord {
			continue
		}
		var event CharacterRecordEvent
		if mapToStruct(record.Raw, &event) != nil || event.TurnID != turn.ID || sourceIndex > len(event.Exchanges) {
			continue
		}
		exchange := event.Exchanges[sourceIndex-1]
		if exchange.TurnID != turn.ID || exchange.Channel != character.ChannelScene {
			continue
		}
		// The utterance has to be aimed at the target the declaration names. A
		// line with no recorded direction is ambiguous, and an ambiguous source
		// stays nonfatal.
		if exchange.AddressedTo != targetID {
			continue
		}
		if actorID == DefaultActorID {
			// A dialogue line, even one copied exactly from the player's input,
			// cannot authorize killing. The player makes that choice in the
			// existing combat action and it is recorded with the settled round.
			return false
		}
		if exchange.Speaker != character.SpeakerCharacter || exchange.CharacterID != actorID {
			continue
		}
		return true
	}
	return false
}

func lethalIntentValueFromTurn(turn TurnEvent) (any, string) {
	if turn.StateDelta == nil {
		return nil, ""
	}
	for _, op := range turn.StateDelta.Ops {
		segments, err := interactivestate.ParsePath(op.Path)
		if err == nil && len(segments) == 2 && segments[0] == DefaultStoryContextActorID && normalizeActorStateFieldName(segments[1]) == storySceneEngagementField && op.Op == "set" {
			if op.SourceTurnID == turn.ID && op.SourceKind == "turn_result" {
				return op.Value, op.SourceTurnID
			}
		}
	}
	for _, op := range turn.StateDelta.ActorOps {
		if op.ActorID == DefaultStoryContextActorID && normalizeActorStateFieldName(op.FieldID) == storySceneEngagementField && op.Op == "set" && op.SourceTurnID == turn.ID && op.SourceKind == "turn_result" {
			return op.Value, op.SourceTurnID
		}
	}
	return nil, ""
}

func decodeCombatLethalIntentDeclarations(value any) []combatLethalIntentDeclaration {
	engagements, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	// Two shapes carry the same declarations and both are legitimate readings of
	// the field contract: the list under each actor's own key, or one list at the
	// top level beside the per-actor basis strings. Only the first was decoded,
	// so a model that chose the second had every declaration silently dropped and
	// no fight could ever become fatal. Each declaration names its own actor and
	// target, so the container a list sits in carries no extra meaning.
	var declarations []combatLethalIntentDeclaration
	collect := func(raw any) {
		items, ok := raw.([]any)
		if !ok {
			return
		}
		for _, item := range items {
			fields, ok := item.(map[string]any)
			if !ok {
				continue
			}
			actorID, actorOK := fields["actor_id"].(string)
			targetID, targetOK := fields["target_id"].(string)
			sourceIndex, sourceOK := fields["source_exchange_index"].(float64)
			if actorOK && targetOK && sourceOK && sourceIndex == float64(int(sourceIndex)) {
				declarations = append(declarations, combatLethalIntentDeclaration{ActorID: actorID, TargetID: targetID, SourceExchangeIndex: int(sourceIndex)})
			}
		}
	}
	for key, raw := range engagements {
		if key == combatLethalIntentsField {
			collect(raw)
			continue
		}
		if entry, ok := raw.(map[string]any); ok {
			collect(entry[combatLethalIntentsField])
		}
	}
	return declarations
}

func combatFatalities(previous, settled cultivation.CombatSession, outcome cultivation.ExchangeOutcome, intents []CombatLethalIntent, sourceEventID string) []CombatFatality {
	if previous.ID == "" || previous.ID != settled.ID || outcome.Result == cultivation.ResultTruce {
		return nil
	}
	intentTargets := make(map[string]CombatLethalIntent, len(intents))
	for _, intent := range intents {
		intentTargets[intent.ActorID+"\x00"+intent.TargetID] = intent
	}
	var fatalities []CombatFatality
	for index, step := range outcome.Steps {
		if step.ActionRejected || step.TargetID == "" || step.Resolution.HealthLost <= 0 || !step.Defeated || (step.Kind != cultivation.ActionTechnique && step.Kind != cultivation.ActionContinuePreparation) {
			continue
		}
		if !registeredDamageStep(previous, step) {
			continue
		}
		if combatantCrossedToDefeatedZero(previous, settled, step.ActorID, step.TargetID) {
			if intent, ok := intentTargets[step.ActorID+"\x00"+step.TargetID]; ok {
				fatalities = append(fatalities, CombatFatality{ActorID: step.TargetID, KillerActorID: step.ActorID, SourceTurnID: intent.SourceTurnID, SourceEventID: sourceEventID, Round: outcome.Round, StepIndex: index})
			}
		}
		for _, extra := range step.ExtraTargets {
			if extra.Resolution.HealthLost <= 0 || !extra.Defeated || !registeredExtraDamageStep(previous, step) || !combatantCrossedToDefeatedZero(previous, settled, step.ActorID, extra.TargetID) {
				continue
			}
			if intent, ok := intentTargets[step.ActorID+"\x00"+extra.TargetID]; ok {
				fatalities = append(fatalities, CombatFatality{ActorID: extra.TargetID, KillerActorID: step.ActorID, SourceTurnID: intent.SourceTurnID, SourceEventID: sourceEventID, Round: outcome.Round, StepIndex: index})
			}
		}
	}
	return fatalities
}

func registeredDamageStep(previous cultivation.CombatSession, step cultivation.ExchangeStep) bool {
	actor := previous.CombatantByID(step.ActorID)
	if actor == nil || !actor.Active() || step.SkillID == "" {
		return false
	}
	return combatantHasRegisteredSkill(actor, step.SkillID) || step.SkillID == "strike"
}

func registeredExtraDamageStep(previous cultivation.CombatSession, step cultivation.ExchangeStep) bool {
	actor := previous.CombatantByID(step.ActorID)
	if actor == nil || !actor.Active() || step.Kind != cultivation.ActionTechnique || step.SkillID == "" {
		return false
	}
	return combatantHasRegisteredSkill(actor, step.SkillID)
}

func combatantHasRegisteredSkill(actor *cultivation.Combatant, skillID string) bool {
	for _, skill := range actor.Skills {
		if skill.ID == skillID {
			return true
		}
	}
	return false
}

func combatantCrossedToDefeatedZero(previous, settled cultivation.CombatSession, attackerID, targetID string) bool {
	attackerBefore, targetBefore := previous.CombatantByID(attackerID), previous.CombatantByID(targetID)
	attackerAfter, targetAfter := settled.CombatantByID(attackerID), settled.CombatantByID(targetID)
	return attackerBefore != nil && attackerBefore.Active() && targetBefore != nil && targetBefore.Active() && targetBefore.Attributes.Health > 0 && attackerAfter != nil && targetAfter != nil && targetAfter.Attributes.Health == 0 && targetAfter.Defeated && !targetAfter.Fled && !targetAfter.Surrendered
}

func frozenLethalIntentsForBattle(lines []StoryEventRecord, head, sessionID string) []CombatLethalIntent {
	path, _, err := walkEventPath(head, eventsByID(lines))
	if err != nil {
		return nil
	}
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if mapToStruct(record.Raw, &event) == nil && event.Session.ID == sessionID && event.Outcome.Round == 0 {
			return append([]CombatLethalIntent(nil), event.LethalIntents...)
		}
	}
	return nil
}

func terminalOutcomeForCombatFatality(fatality CombatFatality, sourceTurnID string) *TerminalOutcome {
	if fatality.ActorID != DefaultActorID {
		return nil
	}
	return normalizeTerminalOutcomePointer(&TerminalOutcome{
		Terminal: true, Type: "combat_death", ReasonKey: "storyStage.ended.reason.combatDeath",
		CausedByTurnID: sourceTurnID, RuleResolutionID: fatality.SourceEventID,
	})
}

func combatTerminalOutcomeFromPath(path []StoryEventRecord) *TerminalOutcome {
	for index := len(path) - 1; index >= 0; index-- {
		if path[index].Envelope.Type == StoryEventTypeStateDelta {
			var delta StateDeltaEvent
			if mapToStruct(path[index].Raw, &delta) == nil && delta.TerminalOutcome != nil && delta.TerminalOutcome.Terminal {
				return normalizeTerminalOutcomePointer(delta.TerminalOutcome)
			}
		}
		if path[index].Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if mapToStruct(path[index].Raw, &event) == nil && event.TerminalOutcome != nil && event.TerminalOutcome.Terminal {
			return normalizeTerminalOutcomePointer(event.TerminalOutcome)
		}
	}
	return nil
}
