package app

import (
	"encoding/json"
	"maps"
	"strings"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// The protagonist owns the adventure ledger, but the nested battle also holds
// the opponent's private settlement state. Only the encounter identity crosses
// the player boundary. A whitelist keeps future opponent fields private.
func playerAdventureValue(value any) any {
	if value == nil {
		return nil
	}
	actor := adventurePlayerMap(value)
	if actor == nil {
		return nil
	}
	out := maps.Clone(actor)
	delete(out, "acquired")
	delete(out, "defeated")
	if battle := adventurePlayerMap(actor["battle"]); battle != nil {
		known := map[string]any{}
		for _, key := range []string{"id", "scale", "opponent_id", "opponent_name", "round", "status", "asset_id"} {
			if v, exists := battle[key]; exists {
				known[key] = v
			}
		}
		out["battle"] = known
	}
	return out
}

func adventurePlayerMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]any
	if json.Unmarshal(encoded, &result) != nil {
		return nil
	}
	return result
}

func playerAdventureFields(fields map[string]any, protagonist bool) map[string]any {
	if _, exists := fields[interactive.AdventureStateField]; !exists {
		return fields
	}
	out := maps.Clone(fields)
	if protagonist {
		out[interactive.AdventureStateField] = playerAdventureValue(fields[interactive.AdventureStateField])
	} else {
		for field := range out {
			if interactive.AdventureOwnsField(field) {
				delete(out, field)
			}
		}
	}
	return out
}

func playerAdventureState(state map[string]any) map[string]any {
	actors, ok := state[actorStateRoot].(map[string]any)
	if !ok {
		return state
	}
	out := maps.Clone(state)
	projected := maps.Clone(actors)
	for id, raw := range actors {
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fields, ok := record["state"].(map[string]any)
		if !ok {
			continue
		}
		copy := maps.Clone(record)
		copy["state"] = playerAdventureFields(fields, id == interactive.DefaultActorID)
		projected[id] = copy
	}
	out[actorStateRoot] = projected
	return out
}

func playerAdventureOps(ops []interactive.ActorStateOp) []interactive.ActorStateOp {
	if len(ops) == 0 {
		return ops
	}
	out := make([]interactive.ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.FieldID == interactive.AdventureStateField {
			if op.ActorID != interactive.DefaultActorID {
				continue
			}
			op.Value = playerAdventureValue(op.Value)
			op.PreviousValue = playerAdventureValue(op.PreviousValue)
		}
		out = append(out, op)
	}
	return out
}

func playerAdventureTurn(turn interactive.TurnEvent) interactive.TurnEvent {
	if turn.StateDelta != nil {
		delta := *turn.StateDelta
		delta.ActorOps = playerAdventureOps(delta.ActorOps)
		delta.Ops = make([]interactivestate.Op, 0, len(turn.StateDelta.Ops))
		for _, op := range turn.StateDelta.Ops {
			// Adventure writes use ActorOps. Generic legacy paths must not expose
			// a nested secret by projecting only the top-level actor field.
			if strings.Contains(op.Path, interactive.AdventureStateField) {
				continue
			}
			if value, ok := op.Value.(map[string]any); ok {
				op.Value = playerAdventureState(value)
				own := strings.Contains(op.Path, interactive.DefaultActorID)
				if fields, ok := value["state"].(map[string]any); ok {
					record := maps.Clone(value)
					record["state"] = playerAdventureFields(fields, own)
					op.Value = record
				} else if _, exists := value[interactive.AdventureStateField]; exists {
					op.Value = playerAdventureFields(value, own)
				}
			}
			delta.Ops = append(delta.Ops, op)
		}
		turn.StateDelta = &delta
	}
	if turn.TurnResult != nil {
		result := *turn.TurnResult
		result.StateUpdates = make([]interactivestate.Update, 0, len(turn.TurnResult.StateUpdates))
		for _, update := range turn.TurnResult.StateUpdates {
			if !strings.Contains(update.Path, interactive.AdventureStateField) {
				result.StateUpdates = append(result.StateUpdates, update)
			}
		}
		turn.TurnResult = &result
	}
	return turn
}

// Only the player's observed item/skill names are needed to render the ledger.
// Frozen enemy definitions and NPC plans belong to canonical storage and model
// context, not the browser schema. Historical own ops retain familiar names.
func playerAdventureSchema(schema *interactive.ActorStateSchemaSnapshot, snapshot interactive.Snapshot) *interactive.ActorStateSchemaSnapshot {
	if schema == nil {
		return nil
	}
	if schema.System.AdventureRules == nil {
		copy := *schema
		copy.System = schema.System
		copy.System.DefinitionDetails = nil
		return &copy
	}
	skills, items := map[string]bool{}, map[string]bool{}
	collect := func(value any) {
		actor := adventurePlayerMap(value)
		for _, field := range []string{"skills", "training"} {
			for id := range adventurePlayerMap(actor[field]) {
				skills[id] = true
			}
		}
		for _, field := range []string{"inventory", "assets", "acquired"} {
			for id := range adventurePlayerMap(actor[field]) {
				items[id] = true
			}
		}
		for _, value := range adventurePlayerMap(actor["equipment"]) {
			if id, ok := value.(string); ok {
				items[id] = true
			}
		}
	}
	actors := adventurePlayerMap(snapshot.State[actorStateRoot])
	actor := adventurePlayerMap(actors[interactive.DefaultActorID])
	collect(adventurePlayerMap(actor["state"])[interactive.AdventureStateField])
	for _, turn := range snapshot.Turns {
		if turn.StateDelta != nil {
			for _, op := range turn.StateDelta.ActorOps {
				if op.ActorID == interactive.DefaultActorID && op.FieldID == interactive.AdventureStateField {
					collect(op.Value)
					collect(op.PreviousValue)
				}
			}
		}
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var out interactive.ActorStateSchemaSnapshot
	if json.Unmarshal(encoded, &out) != nil {
		return nil
	}
	out.System.DefinitionDetails = nil
	original := schema.System.AdventureRules
	known := &interactive.AdventureRules{Family: original.Family, Currency: original.Currency}
	for _, skill := range original.Skills {
		if skills[skill.ID] {
			known.Skills = append(known.Skills, skill)
		}
	}
	for _, item := range original.Items {
		if items[item.ID] {
			known.Items = append(known.Items, item)
		}
	}
	out.System.AdventureRules = known
	out.Adaptation = nil
	private := actorStatePrivateFields(schema)
	for i := range out.System.InitialActors {
		initial := &out.System.InitialActors[i]
		initial.State = playerAdventureFields(initial.State, initial.ID == interactive.DefaultActorID)
		if initial.ID != interactive.DefaultActorID {
			initial.State = actorStateWithout(initial.State, private[initial.TemplateID])
		}
	}
	for i := range out.System.Templates {
		for j := range out.System.Templates[i].Fields {
			field := &out.System.Templates[i].Fields[j]
			if out.System.Templates[i].ID != interactive.DefaultActorID && field.Visibility == interactive.ActorStateVisibilityPrivate {
				field.Default = nil
			}
			if field.Name == interactive.AdventureStateField {
				if out.System.Templates[i].ID == interactive.DefaultActorID {
					field.Default = playerAdventureValue(field.Default)
				} else {
					field.Default = nil
				}
			}
		}
	}
	return &out
}

// A total or target can reveal hidden state-binding adjustments even when the
// original audit field is absent. Preserve the committed public outcome and
// dice, never an alternate recomputed total or the private adjudicator input.
func playerPublicRuleResolution(original *interactive.RuleResolution) *interactive.RuleResolution {
	if original == nil {
		return nil
	}
	r := original.Result
	out := &interactive.RuleResolution{ID: original.ID, CreatedAt: original.CreatedAt, Result: interactive.RuleResult{ID: r.ID, Label: r.Label, Kind: r.Kind, Mode: r.Mode, Dice: r.Dice, Rolls: append([]int(nil), r.Rolls...), RollMode: r.RollMode, KeptRoll: r.KeptRoll, StoryRollModifier: r.StoryRollModifier, Outcome: r.Outcome, Result: r.Result, StateChanges: r.StateChanges}}
	// Preserve just the finite source identity needed to display the public
	// caption in the interface language; never restore the adjudicator request.
	id := original.Request.Rule.TemplateID
	if id == "" {
		id = original.DisplayTemplateID
	}
	out.DisplayTemplateID = interactive.BuiltinRuleCheckDisplayID(id, r.Label)
	if original.StateConsumption != nil {
		consumption := *original.StateConsumption
		consumption.AppliedOps = nil
		consumption.Warnings = nil
		consumption.AppliedActorOps = nil
		for _, op := range playerAdventureOps(original.StateConsumption.AppliedActorOps) {
			if op.ActorID == interactive.DefaultActorID {
				consumption.AppliedActorOps = append(consumption.AppliedActorOps, op)
			}
		}
		out.StateConsumption = &consumption
	}
	return out
}
