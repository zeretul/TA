package app

import (
	"strings"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// The player-facing projection of story state.
//
// §3.1 of 战斗建议复核与NPC信息边界 keeps three things apart: the program holds
// the full truth so it can settle and recover, the player sees only what the
// protagonist could actually know, and a field the player has not found out
// reads as unknown rather than as nothing. §四 adds the mechanical rule that
// makes the first two real: the boundary is enforced where the data leaves the
// process, not by a stylesheet.
//
// So the whole projection lives here, at the two places state leaves for the
// player — the snapshot read and the persisted-turn event — and it withholds
// the pack's private fields for actors the player is not playing. The journal
// and the director console keep the full record; nothing is deleted to create
// the unknown.

// actorStateRoot is the story-state key holding every Actor. It mirrors the
// interactive package's own root so this file can walk the map it is given
// without the store exposing internals.
const actorStateRoot = "actors"

// projectInteractiveSnapshotForPlayer applies the player-facing boundary to a
// snapshot in place, on every field of it that carries story state: the current
// state map, the current turn, the turns already in the page, and the schema the
// interface reads to know what a private field is.
func projectInteractiveSnapshotForPlayer(snapshot *interactive.Snapshot, audiences ...loreKnowledgeAudience) {
	if snapshot == nil {
		return
	}
	schema := snapshot.ActorStateSchema
	playerSchema := projectPlayerActorSchema(playerAdventureSchema(playerLoreKnowledgeSchema(schema), *snapshot))
	snapshot.State = playerKnownState(snapshot.State, schema, audiences...)
	snapshot.Turns = append([]interactive.TurnEvent(nil), snapshot.Turns...)
	for index := range snapshot.Turns {
		snapshot.Turns[index] = playerKnownTurn(snapshot.Turns[index], schema)
	}
	if snapshot.CurrentTurn != nil {
		current := playerKnownTurn(*snapshot.CurrentTurn, schema)
		snapshot.CurrentTurn = &current
	}
	snapshot.ActorStateSchema = playerSchema
}

// playerKnownState projects story state for the player.
//
// Lore observations are first restricted to the committed player audience.
// The protagonist and bookkeeping actors otherwise retain their own fields.
// Every other actor loses the fields
// its template marks private — learned arts, carried things, means and
// relations — while the rest of what the story already says about them stays,
// because a name, a public identity and a place are not a dossier.
func playerKnownState(state map[string]any, schema *interactive.ActorStateSchemaSnapshot, audiences ...loreKnowledgeAudience) map[string]any {
	state = playerLoreKnowledgeState(state, schema, audiences...)
	state = playerAdventureState(state)
	projection := newPlayerActorProjection(schema)
	// These values have already passed the committed-audience/source filter.
	// Preserve legacy known facts without declaring arbitrary NPC fields public.
	projection.projectedKnowledge = loreKnowledgeFields(schema)
	return projection.state(state)
}

// actorStatePrivateFields collects the private field IDs each template
// declares. Only fields the frozen schema actually names are withheld, so a
// field the story added later cannot be caught by a name guess.
func actorStatePrivateFields(schema *interactive.ActorStateSchemaSnapshot) map[string]map[string]bool {
	if schema == nil {
		return nil
	}
	fields := make(map[string]map[string]bool, len(schema.System.Templates))
	for _, template := range schema.System.Templates {
		var private map[string]bool
		for _, field := range template.Fields {
			if field.Visibility != interactive.ActorStateVisibilityPrivate {
				continue
			}
			if private == nil {
				private = map[string]bool{}
			}
			private[field.Name] = true
		}
		if len(private) > 0 {
			fields[template.ID] = private
		}
	}
	return fields
}

func actorStateWithout(state map[string]any, withheld map[string]bool) map[string]any {
	if len(state) == 0 {
		return state
	}
	projected := make(map[string]any, len(state))
	for key, value := range state {
		if withheld[key] {
			continue
		}
		projected[key] = value
	}
	return projected
}

func actorStateActorTemplateID(record map[string]any) string {
	templateID, _ := record["template_id"].(string)
	return strings.TrimSpace(templateID)
}

// actorStateBookkeepingTemplate reports the two templates that describe where
// the player is and what the world is doing. They are not dossiers, so hiding
// their fields would hide the story from its own reader.
func actorStateBookkeepingTemplate(templateID string) bool {
	switch strings.TrimSpace(templateID) {
	case interactive.ActorStateStoryContextTemplateID, interactive.ActorStateWorldEntitiesTemplateID:
		return true
	default:
		return false
	}
}

// playerKnownTurn projects one turn's own state record.
//
// A turn's actor ops name the field each write touched. Withholding an op is
// what keeps the change summary from naming an opponent's art or a dose they
// swallowed, and the journal keeps the op for the rules and the director.
//
// TurnResult is nested inside the turn and is persisted with it, so it is
// projected the same way: StateUpdates is the model's own write list, and it
// carries the same private fields the dossier projection withholds.
func playerKnownTurn(turn interactive.TurnEvent, schema *interactive.ActorStateSchemaSnapshot) interactive.TurnEvent {
	turn = playerLoreKnowledgeTurn(turn, schema)
	turn = playerAdventureTurn(turn)
	turn.RuleResolution = playerPublicRuleResolution(turn.RuleResolution)
	return projectPlayerActorCarriers(turn, schema)
}

func playerKnownActorOps(ops []interactive.ActorStateOp, privateFields map[string]map[string]bool) []interactive.ActorStateOp {
	if len(ops) == 0 {
		return ops
	}
	kept := make([]interactive.ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if actorFieldIsPrivate(op.ActorID, op.FieldID, privateFields) {
			continue
		}
		kept = append(kept, op)
	}
	return kept
}

func playerKnownStateOps(ops []interactivestate.Op, privateFields map[string]map[string]bool) []interactivestate.Op {
	if len(ops) == 0 {
		return ops
	}
	kept := make([]interactivestate.Op, 0, len(ops))
	for _, op := range ops {
		actorID, fieldID, scoped := actorStateOpTarget(op.Path)
		if scoped && actorFieldIsPrivate(actorID, fieldID, privateFields) {
			continue
		}
		kept = append(kept, op)
	}
	return kept
}

// playerKnownTurnResultUpdates projects the model's own write list.
//
// TurnResult.StateUpdates is persisted inside the turn and is the one nested
// object the snapshot projection did not walk, so the stream and every history
// page carried an opponent's private paths and values verbatim. Each path is a
// JSON Pointer whose first segment is the actor and whose second is the field,
// which is the same pair the actor ops already name.
func playerKnownTurnResultUpdates(updates []interactivestate.Update, privateFields map[string]map[string]bool) []interactivestate.Update {
	if len(updates) == 0 {
		return updates
	}
	kept := make([]interactivestate.Update, 0, len(updates))
	for _, update := range updates {
		actorID, fieldID, scoped := actorStateUpdateTarget(update.Path)
		if scoped && actorFieldIsPrivate(actorID, fieldID, privateFields) {
			continue
		}
		kept = append(kept, update)
	}
	return kept
}

// actorStateUpdateTarget reads a submission pointer of the form
// /<actor_id>/<field_id> back into its actor and field.
func actorStateUpdateTarget(path string) (string, string, bool) {
	segments, err := interactivestate.ParsePath(strings.TrimSpace(path))
	if err != nil || len(segments) < 2 {
		return "", "", false
	}
	return segments[0], segments[1], true
}

func playerKnownRuleResult(result interactive.RuleResult, privateFields map[string]map[string]bool) interactive.RuleResult {
	if len(result.StateChanges) == 0 {
		return result
	}
	kept := make([]interactive.TurnStateChange, 0, len(result.StateChanges))
	for _, change := range result.StateChanges {
		if actorFieldIsPrivate(change.ActorID, change.FieldID, privateFields) {
			continue
		}
		kept = append(kept, change)
	}
	result.StateChanges = kept
	return result
}

// actorFieldIsPrivate answers the one question the boundary needs: is this a
// field the pack marked private, on an actor the player is not playing?
func actorFieldIsPrivate(actorID, fieldID string, privateFields map[string]map[string]bool) bool {
	if len(privateFields) == 0 {
		return false
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || actorID == interactive.DefaultActorID {
		return false
	}
	// The schema's map is keyed by template and an op names the actor, so the
	// field is matched across every template's private set. Field IDs are the
	// pack's own names and unique within a template; matching across templates
	// can only ever withhold more.
	for _, fields := range privateFields {
		if fields[strings.TrimSpace(fieldID)] {
			return true
		}
	}
	return false
}

// actorStateOpTarget reads an RFC 6901 path of the form
// /actors/<actor_id>/state/<field_id> back into its actor and field.
func actorStateOpTarget(path string) (string, string, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(path), "/"), "/")
	if len(parts) < 4 || parts[0] != actorStateRoot || parts[2] != "state" {
		return "", "", false
	}
	return parts[1], parts[3], true
}
