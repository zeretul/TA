package app

import (
	"strings"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// A single read-side policy covers complete records and field mutations. The
// journal is never rewritten. Unknown NPC fields are withheld, not presumed
// public just because they are absent from a private-name blacklist.
type playerActorProjection struct {
	projectedKnowledge map[string]string
	fields             map[string]map[string]bool
	actors             map[string]string
	public             map[string]bool
	private            map[string]bool
}

func newPlayerActorProjection(schema *interactive.ActorStateSchemaSnapshot) *playerActorProjection {
	p := &playerActorProjection{fields: map[string]map[string]bool{}, actors: map[string]string{}, public: map[string]bool{}, private: map[string]bool{}}
	if schema == nil {
		return p
	}
	for _, t := range schema.System.Templates {
		fields := map[string]bool{}
		for _, f := range t.Fields {
			open := f.Visibility == interactive.ActorStateVisibilityOpen
			fields[f.Name] = open
			if open {
				p.public[f.Name] = true
			} else {
				p.private[f.Name] = true
			}
		}
		p.fields[t.ID] = fields
	}
	for _, a := range schema.System.InitialActors {
		p.actors[a.ID] = a.TemplateID
	}
	return p
}

func (p *playerActorProjection) owner(actor string) bool {
	return actor == interactive.DefaultActorID || actor == interactive.DefaultStoryContextActorID || actor == interactive.DefaultWorldEntitiesActorID || actorStateBookkeepingTemplate(p.actors[actor])
}
func (p *playerActorProjection) allowed(actor, field string) bool {
	if p.owner(actor) {
		return true
	}
	if template, known := p.actors[actor]; known {
		return p.fields[template][field]
	}
	// Historical field-only records may not carry a template. Retain only
	// declared fields that no template classifies as private.
	return p.public[field] && !p.private[field]
}
func playerActorIdentityKey(key string) bool {
	switch key {
	case "id", "name", "template_id", "role", "description", "gender", "speech_style", "tags":
		return true
	}
	return false
}
func (p *playerActorProjection) remember(actor string, value any) {
	if record := adventurePlayerMap(value); record != nil {
		if template, ok := record["template_id"].(string); ok && template != "" {
			p.actors[actor] = template
		}
	}
}
func (p *playerActorProjection) fieldsValue(actor string, value any) any {
	if value == nil {
		return nil
	}
	fields := adventurePlayerMap(value)
	if fields == nil {
		return nil
	}
	out := map[string]any{}
	for name, v := range fields {
		if p.allowed(actor, name) || (p.projectedKnowledge[name] != "" && !p.private[name]) {
			out[name] = v
		}
	}
	return playerAdventureFields(out, actor == interactive.DefaultActorID)
}
func (p *playerActorProjection) record(actor string, value any) any {
	if value == nil {
		return nil
	}
	record := adventurePlayerMap(value)
	if record == nil {
		return nil
	}
	p.remember(actor, record)
	out := map[string]any{}
	for key, v := range record {
		if key == "state" {
			out[key] = p.fieldsValue(actor, v)
		} else if p.owner(actor) || playerActorIdentityKey(key) {
			out[key] = v
		}
	}
	return out
}
func (p *playerActorProjection) actorMap(value any) any {
	if value == nil {
		return nil
	}
	records := adventurePlayerMap(value)
	if records == nil {
		return nil
	}
	out := map[string]any{}
	for actor, v := range records {
		out[actor] = p.record(actor, v)
	}
	return out
}
func (p *playerActorProjection) state(state map[string]any) map[string]any {
	if state == nil {
		return nil
	}
	out := map[string]any{}
	for key, v := range state {
		if key == actorStateRoot {
			out[key] = p.actorMap(v)
		} else {
			out[key] = v
		}
	}
	return out
}

// Reducer records use dotted paths; newer records can use RFC6901. Decode
// escaped field names before checking them, including unset/delete operations.
func playerReducerPath(path string) ([]string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return nil, true
	}
	if strings.HasPrefix(path, "/") {
		parts, err := interactivestate.ParsePath(path)
		return parts, err == nil
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
	}
	return parts, true
}
func (p *playerActorProjection) reducerValue(parts []string, value any) (any, bool) {
	if len(parts) == 0 {
		if value == nil {
			return nil, true
		}
		state := adventurePlayerMap(value)
		if state == nil {
			return nil, false
		}
		return p.state(state), true
	}
	if parts[0] != actorStateRoot {
		return value, true
	}
	if len(parts) == 1 {
		return p.actorMap(value), true
	}
	actor := parts[1]
	if len(parts) == 2 {
		return p.record(actor, value), true
	}
	if parts[2] == "state" {
		if len(parts) == 3 {
			return p.fieldsValue(actor, value), true
		}
		if !p.allowed(actor, parts[3]) {
			return nil, false
		}
		return value, true
	}
	if p.owner(actor) || (len(parts) == 3 && playerActorIdentityKey(parts[2])) {
		return value, true
	}
	return nil, false
}

func projectPlayerActorCarriers(turn interactive.TurnEvent, schema *interactive.ActorStateSchemaSnapshot) interactive.TurnEvent {
	p := newPlayerActorProjection(schema)
	// Resolve identities before filtering field ops in this same turn.
	if turn.TurnResult != nil {
		for _, u := range turn.TurnResult.StateUpdates {
			if path, err := interactivestate.ParsePath(u.Path); err == nil && len(path) == 1 {
				p.remember(path[0], u.Value)
			}
		}
	}
	if turn.StateDelta != nil {
		for _, op := range turn.StateDelta.Ops {
			parts, ok := playerReducerPath(op.Path)
			if !ok {
				continue
			}
			if len(parts) == 2 && parts[0] == actorStateRoot {
				p.remember(parts[1], op.Value)
			}
			if len(parts) == 3 && parts[0] == actorStateRoot && parts[2] == "template_id" {
				if id, ok := op.Value.(string); ok {
					p.actors[parts[1]] = id
				}
			}
			if len(parts) == 1 && parts[0] == actorStateRoot {
				for id, v := range adventurePlayerMap(op.Value) {
					p.remember(id, v)
				}
			}
		}
	}
	if turn.TurnResult != nil {
		result := *turn.TurnResult
		result.StateUpdates = nil
		for _, u := range turn.TurnResult.StateUpdates {
			parts, err := interactivestate.ParsePath(u.Path)
			if err != nil {
				continue
			}
			if len(parts) == 1 {
				u.Value = p.record(parts[0], u.Value)
			} else if !p.allowed(parts[0], parts[1]) {
				continue
			}
			result.StateUpdates = append(result.StateUpdates, u)
		}
		turn.TurnResult = &result
	}
	if turn.StateDelta != nil {
		delta := *turn.StateDelta
		delta.Ops = nil
		delta.ActorOps = nil
		for _, op := range turn.StateDelta.Ops {
			parts, ok := playerReducerPath(op.Path)
			if !ok {
				continue
			}
			value, keep := p.reducerValue(parts, op.Value)
			if !keep {
				continue
			}
			op.Value = value
			if len(parts) == 0 || parts[0] == actorStateRoot {
				op.Reason = ""
			}
			delta.Ops = append(delta.Ops, op)
		}
		for _, op := range turn.StateDelta.ActorOps {
			if !p.allowed(op.ActorID, op.FieldID) {
				continue
			}
			if !p.owner(op.ActorID) {
				op.Reason = ""
			}
			delta.ActorOps = append(delta.ActorOps, op)
		}
		turn.StateDelta = &delta
	}
	if turn.RuleResolution != nil {
		resolution := *turn.RuleResolution
		resolution.Result.StateChanges = nil
		for _, change := range turn.RuleResolution.Result.StateChanges {
			if p.allowed(change.ActorID, change.FieldID) {
				resolution.Result.StateChanges = append(resolution.Result.StateChanges, change)
			}
		}
		turn.RuleResolution = &resolution
	}
	return turn
}

// Schema seeds are also player-facing state carriers, even in worlds without
// AdventureRules. Copy slices before redaction so frozen GM seeds stay intact.
func projectPlayerActorSchema(schema *interactive.ActorStateSchemaSnapshot) *interactive.ActorStateSchemaSnapshot {
	if schema == nil {
		return nil
	}
	out := *schema
	p := newPlayerActorProjection(schema)
	out.System.InitialActors = append([]interactive.ActorStateInitialActor(nil), schema.System.InitialActors...)
	for i := range out.System.InitialActors {
		a := &out.System.InitialActors[i]
		a.State = adventurePlayerMap(p.fieldsValue(a.ID, a.State))
	}
	out.System.Templates = append([]interactive.ActorStateTemplate(nil), schema.System.Templates...)
	for i := range out.System.Templates {
		template := &out.System.Templates[i]
		template.Fields = append([]interactive.ActorStateField(nil), template.Fields...)
		for j := range template.Fields {
			field := &template.Fields[j]
			if template.ID != interactive.DefaultActorID && !actorStateBookkeepingTemplate(template.ID) && field.Visibility != interactive.ActorStateVisibilityOpen {
				field.Default = nil
			}
		}
	}
	return &out
}
