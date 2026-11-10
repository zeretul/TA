package exploration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type DungeonDefinition struct {
	SchemaVersion  int                 `json:"schema_version"`
	ID             string              `json:"id"`
	Version        string              `json:"version"`
	PackageID      string              `json:"package_id"`
	PackageVersion string              `json:"package_version"`
	RulesetID      string              `json:"ruleset_id"`
	RulesetVersion string              `json:"ruleset_version"`
	Theme          string              `json:"theme"`
	Objectives     []Objective         `json:"objectives"`
	Areas          []Area              `json:"areas"`
	Connections    []DungeonConnection `json:"connections"`
	EntryAreaIDs   []string            `json:"entry_area_ids"`
	// EntryLocationIDs are the registered outside places this realm can really be
	// entered from. §11.2 of 自制-探索与行路 requires each realm to declare its
	// entry conditions, and a cave that could be walked into from anywhere would
	// not have one: the character has to be where the entrance is.
	//
	// It is separate from EntryAreaIDs, which name rooms inside the realm.
	EntryLocationIDs []string         `json:"entry_location_ids"`
	ExitAreaIDs      []string         `json:"exit_area_ids"`
	Challenges       []Challenge      `json:"challenges"`
	ContentRefs      []ContentRef     `json:"content_refs"`
	CompletionPaths  []CompletionPath `json:"completion_paths"`
	ExploreRules     *ExploreRules    `json:"explore_rules,omitempty"`
}

type Objective struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	DescriptionKey string `json:"description_key,omitempty"`
}
type Area struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NameKey        string `json:"name_key,omitempty"`
	Description    string `json:"description"`
	DescriptionKey string `json:"description_key,omitempty"`
}
type DungeonConnection struct {
	ID            string `json:"id"`
	FromAreaID    string `json:"from_area_id"`
	ToAreaID      string `json:"to_area_id"`
	Bidirectional bool   `json:"bidirectional"`
	PlayerVisible bool   `json:"player_visible"`
}
type Challenge struct {
	ID          string `json:"id"`
	AreaID      string `json:"area_id"`
	ReferenceID string `json:"reference_id"`
}
type ContentRef struct {
	ID          string `json:"id"`
	AreaID      string `json:"area_id"`
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id"`
	Quantity    int64  `json:"quantity"`
	// ObjectiveID names the objective this reward completes, empty when taking it
	// completes none.
	//
	// It is authored here rather than inferred from the room, because whether a
	// found thing answers the reason the character came is a fact about the realm,
	// not a guess the interface may make. Taking the reward is what records the
	// completion, so an objective can never tick itself.
	ObjectiveID string `json:"objective_id,omitempty"`
}
type CompletionPath struct {
	ID                 string   `json:"id"`
	ObjectiveIDs       []string `json:"objective_ids"`
	ChallengeIDs       []string `json:"challenge_ids"`
	RequiredAbilityIDs []string `json:"required_ability_ids"`
}

// ParseDungeonDefinition accepts only the local candidate JSON contract. It
// rejects unknown fields and missing/null required fields before semantic checks.
func ParseDungeonDefinition(data []byte) (DungeonDefinition, []Diagnostic, error) {
	if len(data) > MaxDefinitionBytes {
		return DungeonDefinition{}, nil, ErrInputTooLarge
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return DungeonDefinition{}, nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	if raw == nil {
		return DungeonDefinition{}, nil, fmt.Errorf("%w: top level must be an object", ErrInvalidDefinition)
	}
	required := []string{"schema_version", "id", "version", "package_id", "package_version", "ruleset_id", "ruleset_version", "theme", "objectives", "areas", "connections", "entry_area_ids", "entry_location_ids", "exit_area_ids", "challenges", "content_refs", "completion_paths"}
	for _, key := range required {
		value, ok := raw[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return DungeonDefinition{}, nil, fmt.Errorf("%w: required field %q is missing or null", ErrInvalidDefinition, key)
		}
	}
	objectFields := []struct {
		name   string
		fields []string
	}{
		{"objectives", []string{"id", "description"}},
		{"areas", []string{"id", "name", "description"}},
		{"connections", []string{"id", "from_area_id", "to_area_id", "bidirectional", "player_visible"}},
		{"challenges", []string{"id", "area_id", "reference_id"}},
		{"content_refs", []string{"id", "area_id", "kind", "reference_id", "quantity"}},
		{"completion_paths", []string{"id", "objective_ids", "challenge_ids", "required_ability_ids"}},
	}
	for _, recordType := range objectFields {
		list := recordType.name
		var records []json.RawMessage
		if err := json.Unmarshal(raw[list], &records); err != nil || records == nil {
			return DungeonDefinition{}, nil, fmt.Errorf("%w: %s must be an array", ErrInvalidDefinition, list)
		}
		for i, record := range records {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(record, &object); err != nil || object == nil {
				return DungeonDefinition{}, nil, fmt.Errorf("%w: %s[%d] must be an object", ErrInvalidDefinition, list, i)
			}
			for _, field := range recordType.fields {
				value, ok := object[field]
				if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return DungeonDefinition{}, nil, fmt.Errorf("%w: %s[%d].%s is missing or null", ErrInvalidDefinition, list, i, field)
				}
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var def DungeonDefinition
	if err := decoder.Decode(&def); err != nil {
		return DungeonDefinition{}, nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return DungeonDefinition{}, nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	limits := []struct {
		name   string
		n, max int
	}{{"objectives", len(def.Objectives), MaxObjectives}, {"areas", len(def.Areas), MaxAreas}, {"connections", len(def.Connections), MaxConnections}, {"entry_area_ids", len(def.EntryAreaIDs), MaxAreas}, {"exit_area_ids", len(def.ExitAreaIDs), MaxAreas}, {"challenges", len(def.Challenges), MaxChallenges}, {"content_refs", len(def.ContentRefs), MaxContentRefs}, {"completion_paths", len(def.CompletionPaths), MaxCompletionPaths}}
	for _, item := range limits {
		if item.n > item.max {
			return DungeonDefinition{}, nil, fmt.Errorf("%w: %s exceeds limit %d", ErrInvalidDefinition, item.name, item.max)
		}
	}
	return def, ValidateDungeonDefinition(def), nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}
