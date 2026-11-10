package app

import (
	"log/slog"
	"maps"
	"strings"

	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// Lore knowledge is a player observation, not a second route to the author
// dossier. Missing audience context fails closed. The audience is reconstructed
// from this branch's committed character journal, never from actor existence.
type loreKnowledgeAudience struct {
	known     map[string]bool
	exchanges map[string]bool
}

func loreAudience(state character.State) loreKnowledgeAudience {
	out := loreKnowledgeAudience{known: map[string]bool{interactive.DefaultActorID: true}, exchanges: map[string]bool{}}
	for _, id := range state.CharacterIDs() {
		out.known[id] = true
	}
	for _, exchange := range state.Exchanges {
		if exchange.ID == "" || !exchange.Channel.Valid() {
			continue
		}
		if exchange.Speaker == character.SpeakerPlayer || exchange.AddressedTo == interactive.DefaultActorID || character.ContainsID(exchange.Audience, interactive.DefaultActorID) {
			out.exchanges[exchange.ID] = true
		}
	}
	return out
}

func loreAudienceForSnapshot(store *interactive.Store, snapshot interactive.Snapshot) loreKnowledgeAudience {
	state, err := store.CharacterState(snapshot.StoryID, snapshot.BranchID)
	if err != nil {
		slog.Warn("Cannot project lore knowledge audience", "story_id", snapshot.StoryID, "branch_id", snapshot.BranchID, "error", err)
		return loreKnowledgeAudience{}
	}
	// CharacterState is a separate journal read. Fence it against the snapshot
	// already selected for this response so a concurrent discovery cannot make
	// an older, previously hidden relation visible.
	current, err := store.Snapshot(snapshot.StoryID, snapshot.BranchID)
	if err != nil {
		slog.Warn("Cannot verify lore knowledge revision", "story_id", snapshot.StoryID, "error", err)
		return loreKnowledgeAudience{}
	}
	head := func(view interactive.Snapshot) string {
		for _, branch := range view.Graph.Branches {
			if branch.ID == view.BranchID {
				return branch.Head
			}
		}
		return ""
	}
	if current.ContextRevision != snapshot.ContextRevision || current.BranchID != snapshot.BranchID || head(current) != head(snapshot) {
		return loreKnowledgeAudience{}
	}
	return loreAudience(state)
}

// Names are persisted field identities; paths are accepted for legacy snapshots.
func loreKnowledgeFields(schema *interactive.ActorStateSchemaSnapshot) map[string]string {
	fields := map[string]string{"已知人物关系": "relationships", "knowledge.relationships": "relationships", "已知位置": "location", "knowledge.location": "location", "所属势力": "affiliations", "identity.affiliations": "affiliations"}
	if schema != nil {
		for _, template := range schema.System.Templates {
			for _, field := range template.Fields {
				kind := fields[field.Path]
				if kind == "" {
					kind = fields[field.Name]
				}
				if kind != "" {
					fields[field.Name] = kind
					if field.ID != "" {
						fields[field.ID] = kind
					}
				}
			}
		}
	}
	return fields
}

func playerLoreKnowledgeState(state map[string]any, schema *interactive.ActorStateSchemaSnapshot, audiences ...loreKnowledgeAudience) map[string]any {
	var audience loreKnowledgeAudience
	if len(audiences) > 0 {
		audience = audiences[0]
	}
	actors, ok := state[actorStateRoot].(map[string]any)
	if !ok {
		return state
	}
	out := maps.Clone(state)
	projected := maps.Clone(actors)
	fields := loreKnowledgeFields(schema)
	for id, raw := range actors {
		actor, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		values, ok := actor["state"].(map[string]any)
		if !ok {
			continue
		}
		valuesOut := maps.Clone(values)
		for key, value := range values {
			kind := fields[key]
			if kind == "" {
				continue
			}
			delete(valuesOut, key)
			if id != interactive.DefaultActorID && !audience.known[id] {
				continue
			}
			if kind == "affiliations" {
				if safe := playerLoreAffiliations(value, audience); safe != nil {
					valuesOut[key] = safe
				}
				continue
			}
			if kind == "location" {
				if fact := playerLoreFact(value, kind, audience); fact != nil {
					valuesOut[key] = fact
				}
				continue
			}
			relations, ok := value.(map[string]any)
			if !ok {
				continue
			}
			kept := map[string]any{}
			for target, fact := range relations {
				if target == id || !audience.known[target] {
					continue
				}
				if safe := playerLoreFact(fact, kind, audience); safe != nil {
					kept[target] = safe
				}
			}
			if len(kept) > 0 {
				valuesOut[key] = kept
			}
		}
		copy := maps.Clone(actor)
		copy["state"] = valuesOut
		projected[id] = copy
	}
	out[actorStateRoot] = projected
	return out
}

func playerLoreFact(value any, kind string, audience loreKnowledgeAudience) map[string]any {
	fact, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	// A single public source enum is required. Lists, prose and nested sources
	// cannot quietly blend private evidence into a public-looking record.
	// Unknown metadata may carry another source or a narrower audience. Reject
	// the record rather than silently discarding that qualification.
	for key := range fact {
		switch key {
		case "kind", "type", "类型", "关系类型", "location", "source", "nature", "confidence", "learned_at", "evidence", "evidence_refs", "faction_id", "name", "status", "role":
		default:
			return nil
		}
	}
	source, _ := fact["source"].(string)
	switch source {
	case "observation", "conversation", "public_record":
	default:
		return nil
	}
	out := map[string]any{"source": source}
	if kind == "location" {
		location, ok := fact["location"].(string)
		if !ok || strings.TrimSpace(location) == "" {
			return nil
		}
		out["location"] = location
	} else if kind == "relationships" {
		label := ""
		for _, key := range []string{"kind", "type", "类型", "关系类型"} {
			if s, ok := fact[key].(string); ok && strings.TrimSpace(s) != "" {
				label = s
				break
			}
		}
		if label == "" {
			return nil
		}
		out["kind"] = label
	}
	for _, key := range []string{"nature", "confidence"} {
		raw, exists := fact[key]
		if !exists {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return nil
		}
		valid := false
		if key == "nature" {
			switch text {
			case "experienced", "told", "hearsay", "conjecture":
				valid = true
			}
		} else {
			switch text {
			case "confirmed", "certain", "uncertain":
				valid = true
			}
		}
		if !valid {
			return nil
		}
		out[key] = text
	}
	if out["nature"] == "hearsay" || out["nature"] == "conjecture" {
		out["confidence"] = "uncertain"
	}
	if learned, ok := fact["learned_at"].(string); ok {
		out["learned_at"] = learned
	}
	if raw, exists := fact["evidence_refs"]; exists {
		var refs []string
		switch values := raw.(type) {
		case []string:
			refs = values
		case []any:
			for _, value := range values {
				id, ok := value.(string)
				if !ok {
					return nil
				}
				refs = append(refs, id)
			}
		default:
			return nil
		}
		if len(refs) == 0 {
			return nil
		}
		safe := make([]string, 0, len(refs))
		seen := map[string]bool{}
		for _, id := range refs {
			if !audience.exchanges[id] {
				return nil
			}
			if !seen[id] {
				safe = append(safe, id)
				seen[id] = true
			}
		}
		out["evidence_refs"] = safe
	}
	// Evidence, source prose and arbitrary extension keys are intentionally not
	// copied. This validates provenance shape, not the semantics of model prose.
	return out
}

// Affiliation values have a small identity/status shape. Legacy labels remain
// labels; object extension data must not smuggle an author's private dossier.
func playerLoreAffiliations(value any, audience loreKnowledgeAudience) any {
	if label, ok := value.(string); ok {
		return label
	}
	var values []any
	switch list := value.(type) {
	case []any:
		values = list
	case []string:
		for _, label := range list {
			values = append(values, label)
		}
	default:
		return nil
	}
	safe := make([]any, 0, len(values))
	for _, value := range values {
		if label, ok := value.(string); ok {
			safe = append(safe, label)
			continue
		}
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		base := map[string]any{}
		metadata := false
		for key, val := range item {
			switch key {
			case "faction_id", "name", "status", "role":
				if text, ok := val.(string); ok {
					base[key] = text
				}
			default:
				metadata = true
			}
		}
		if base["faction_id"] == nil && base["name"] == nil {
			continue
		}
		if metadata {
			details := playerLoreFact(item, "affiliations", audience)
			if details == nil {
				continue
			}
			for key, val := range details {
				base[key] = val
			}
		}
		safe = append(safe, base)
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}

// Historical writes cannot be checked with today's cast without revealing
// future discoveries. Omit knowledge mutations, including enclosing create or
// replace operations and their reasons/previous values, on all turn surfaces.
func playerLoreKnowledgeTurn(turn interactive.TurnEvent, schema *interactive.ActorStateSchemaSnapshot) interactive.TurnEvent {
	fields := loreKnowledgeFields(schema)
	if turn.StateDelta != nil {
		delta := *turn.StateDelta
		delta.ActorOps = make([]interactive.ActorStateOp, 0, len(delta.ActorOps))
		for _, op := range turn.StateDelta.ActorOps {
			if fields[op.FieldID] != "" || loreContainsKnowledge(op.Value, fields) || loreContainsKnowledge(op.PreviousValue, fields) {
				continue
			}
			delta.ActorOps = append(delta.ActorOps, op)
		}
		delta.Ops = make([]interactivestate.Op, 0, len(delta.Ops))
		for _, op := range turn.StateDelta.Ops {
			if loreKnowledgePath(op.Path, fields) || loreContainsKnowledge(op.Value, fields) {
				continue
			}
			delta.Ops = append(delta.Ops, op)
		}
		turn.StateDelta = &delta
	}
	if turn.TurnResult != nil {
		result := *turn.TurnResult
		result.StateUpdates = make([]interactivestate.Update, 0, len(result.StateUpdates))
		for _, update := range turn.TurnResult.StateUpdates {
			if loreKnowledgePath(update.Path, fields) || loreContainsKnowledge(update.Value, fields) {
				continue
			}
			result.StateUpdates = append(result.StateUpdates, update)
		}
		turn.TurnResult = &result
	}
	if turn.RuleResolution != nil {
		resolution := *turn.RuleResolution
		resolution.Result.StateChanges = nil
		for _, change := range turn.RuleResolution.Result.StateChanges {
			if fields[change.FieldID] == "" {
				resolution.Result.StateChanges = append(resolution.Result.StateChanges, change)
			}
		}
		if resolution.StateConsumption != nil {
			consumption := *resolution.StateConsumption
			consumption.AppliedActorOps = nil
			for _, op := range resolution.StateConsumption.AppliedActorOps {
				if fields[op.FieldID] == "" && !loreContainsKnowledge(op.Value, fields) && !loreContainsKnowledge(op.PreviousValue, fields) {
					consumption.AppliedActorOps = append(consumption.AppliedActorOps, op)
				}
			}
			resolution.StateConsumption = &consumption
		}
		turn.RuleResolution = &resolution
	}
	return turn
}

// Schema defaults are author input, not learned facts. Do not return knowledge
// defaults alongside otherwise safe current state.
func playerLoreKnowledgeSchema(schema *interactive.ActorStateSchemaSnapshot) *interactive.ActorStateSchemaSnapshot {
	if schema == nil {
		return nil
	}
	fields := loreKnowledgeFields(schema)
	out := *schema
	out.System.Templates = append([]interactive.ActorStateTemplate(nil), schema.System.Templates...)
	for index, template := range out.System.Templates {
		template.Fields = append([]interactive.ActorStateField(nil), template.Fields...)
		for fieldIndex, field := range template.Fields {
			if fields[field.Name] != "" {
				field.Default = nil
				template.Fields[fieldIndex] = field
			}
		}
		out.System.Templates[index] = template
	}
	return &out
}

func loreKnowledgePath(path string, fields map[string]string) bool {
	parts, err := interactivestate.ParsePath(strings.TrimSpace(path))
	if err != nil {
		// Generic legacy operations used dotted paths, while submitted updates
		// use JSON Pointers. Preserve unrelated legacy scene operations.
		for field := range fields {
			if strings.Contains(path, field) {
				return true
			}
		}
		return strings.Contains(path, "knowledge")
	}
	for _, part := range parts {
		if fields[part] != "" || part == "knowledge" {
			return true
		}
	}
	return false
}

func loreContainsKnowledge(value any, fields map[string]string) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if fields[key] != "" || key == "knowledge" || loreContainsKnowledge(item, fields) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if loreContainsKnowledge(item, fields) {
				return true
			}
		}
	}
	return false
}
