package exploration

import (
	"fmt"
	"sort"
)

type Catalog struct {
	PackageVersions map[string]string `json:"package_versions"`
	RulesetVersions map[string]string `json:"ruleset_versions"`
	AbilityIDs      []string          `json:"ability_ids"`
	ChallengeIDs    []string          `json:"challenge_ids"`
	ContentIDs      []string          `json:"content_ids"`
}

type DungeonValidation struct {
	Playable             bool             `json:"playable"`
	ReachableExitAreaIDs []string         `json:"reachable_exit_area_ids"`
	PathDiagnostics      []PathDiagnostic `json:"path_diagnostics"`
	Diagnostics          []Diagnostic     `json:"diagnostics"`
}

type PathDiagnostic struct {
	PathID            string   `json:"path_id"`
	Available         bool     `json:"available"`
	MissingAbilityIDs []string `json:"missing_ability_ids"`
}

type DungeonInstanceBinding struct {
	InstanceID        string `json:"instance_id"`
	WorldID           string `json:"world_id"`
	BranchID          string `json:"branch_id"`
	DefinitionID      string `json:"definition_id"`
	DefinitionVersion string `json:"definition_version"`
	PackageID         string `json:"package_id"`
	PackageVersion    string `json:"package_version"`
	RulesetID         string `json:"ruleset_id"`
	RulesetVersion    string `json:"ruleset_version"`
}

// DungeonScope is supplied by the trusted caller from the currently selected
// world and branch; values from the untrusted instance binding are not scope.
type DungeonScope struct {
	WorldID  string `json:"world_id"`
	BranchID string `json:"branch_id"`
}

func ValidateDungeonDefinition(def DungeonDefinition) []Diagnostic {
	return ValidateDungeon(def, Catalog{}, nil).Diagnostics
}

func ValidateDungeon(def DungeonDefinition, catalog Catalog, availableAbilities []string) DungeonValidation {
	result := DungeonValidation{}
	checks := []struct{ value, path string }{{def.ID, "id"}, {def.Version, "version"}, {def.PackageID, "package_id"}, {def.PackageVersion, "package_version"}, {def.RulesetID, "ruleset_id"}, {def.RulesetVersion, "ruleset_version"}}
	for _, c := range checks {
		result.Diagnostics = append(result.Diagnostics, requireID(c.value, c.path)...)
	}
	if !validText(def.Theme, MaxDescriptionBytes) {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_theme", "theme", "must be non-empty and within the text limit"})
	}
	if def.SchemaVersion != SchemaVersion {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"unsupported_schema", "schema_version", "unsupported local candidate schema version"})
	}
	limits := []struct {
		name           string
		count, maximum int
	}{{"objectives", len(def.Objectives), MaxObjectives}, {"areas", len(def.Areas), MaxAreas}, {"connections", len(def.Connections), MaxConnections}, {"entry_area_ids", len(def.EntryAreaIDs), MaxAreas}, {"entry_location_ids", len(def.EntryLocationIDs), MaxAreas}, {"exit_area_ids", len(def.ExitAreaIDs), MaxAreas}, {"challenges", len(def.Challenges), MaxChallenges}, {"content_refs", len(def.ContentRefs), MaxContentRefs}, {"completion_paths", len(def.CompletionPaths), MaxCompletionPaths}}
	for _, limit := range limits {
		if limit.count > limit.maximum {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"limit_exceeded", limit.name, "record count exceeds supported limit"})
		}
	}
	if catalog.PackageVersions != nil && catalog.PackageVersions[def.PackageID] != def.PackageVersion {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"unregistered_package_version", "package_version", "package version is not registered"})
	}
	if catalog.RulesetVersions != nil && catalog.RulesetVersions[def.RulesetID] != def.RulesetVersion {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"unregistered_ruleset_version", "ruleset_version", "ruleset version is not registered"})
	}
	areas := make(map[string]bool)
	objectives := make(map[string]bool)
	connections := make(map[string]bool)
	challenges := make(map[string]bool)
	content := make(map[string]bool)
	// completedBy records which reward each objective is tied to, so one goal
	// cannot be reached from two different finds.
	completedBy := make(map[string]string)
	addUnique := func(m map[string]bool, id, path string) {
		if !validID(id) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_id", path, "must be a stable non-empty identifier"})
			return
		}
		if m[id] {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_id", path, "identifier is duplicated"})
			return
		}
		m[id] = true
	}
	for i, a := range def.Areas {
		p := fmt.Sprintf("areas[%d]", i)
		addUnique(areas, a.ID, p+".id")
		if !validText(a.Name, MaxDescriptionBytes) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_text", p + ".name", "must be non-empty"})
		}
		if !validText(a.Description, MaxDescriptionBytes) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_text", p + ".description", "must be non-empty and within the text limit"})
		}
	}
	for i, o := range def.Objectives {
		p := fmt.Sprintf("objectives[%d]", i)
		addUnique(objectives, o.ID, p+".id")
		if !validText(o.Description, MaxDescriptionBytes) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_text", p + ".description", "must be non-empty"})
		}
	}
	if len(def.Objectives) == 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"missing_objectives", "objectives", "at least one explicit dungeon objective is required"})
	}
	if len(def.CompletionPaths) == 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"missing_completion_paths", "completion_paths", "at least one completion path is required"})
	}
	for i, c := range def.Connections {
		p := fmt.Sprintf("connections[%d]", i)
		addUnique(connections, c.ID, p+".id")
		requireArea(&result, areas, c.FromAreaID, p+".from_area_id")
		requireArea(&result, areas, c.ToAreaID, p+".to_area_id")
		if c.FromAreaID == c.ToAreaID {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"self_connection", p, "connection endpoints must differ"})
		}
	}
	for i, id := range def.EntryAreaIDs {
		requireArea(&result, areas, id, fmt.Sprintf("entry_area_ids[%d]", i))
		if i > 0 && containsString(def.EntryAreaIDs[:i], id) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_entry", fmt.Sprintf("entry_area_ids[%d]", i), "entry area is duplicated"})
		}
	}
	// An entry location is an outside place, not a room: it is checked as an
	// identifier only, because the registered-place registry belongs to the
	// product layer that knows this story's world.
	for i, id := range def.EntryLocationIDs {
		if !validID(id) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_entry_location", fmt.Sprintf("entry_location_ids[%d]", i), "entry location must be a stable identifier"})
		}
		if i > 0 && containsString(def.EntryLocationIDs[:i], id) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_entry_location", fmt.Sprintf("entry_location_ids[%d]", i), "entry location is duplicated"})
		}
	}
	for i, id := range def.ExitAreaIDs {
		requireArea(&result, areas, id, fmt.Sprintf("exit_area_ids[%d]", i))
		if i > 0 && containsString(def.ExitAreaIDs[:i], id) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_exit", fmt.Sprintf("exit_area_ids[%d]", i), "exit area is duplicated"})
		}
	}
	for i, c := range def.Challenges {
		p := fmt.Sprintf("challenges[%d]", i)
		addUnique(challenges, c.ID, p+".id")
		requireArea(&result, areas, c.AreaID, p+".area_id")
		if catalog.ChallengeIDs != nil && !registered(catalog.ChallengeIDs, c.ReferenceID) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"unregistered_challenge", p + ".reference_id", "challenge reference is not registered"})
		}
	}
	for i, c := range def.ContentRefs {
		p := fmt.Sprintf("content_refs[%d]", i)
		addUnique(content, c.ID, p+".id")
		requireArea(&result, areas, c.AreaID, p+".area_id")
		if c.Kind != "enemy" && c.Kind != "resource" && c.Kind != "reward" {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_content_kind", p + ".kind", "must be enemy, resource, or reward"})
		}
		if c.Quantity <= 0 || c.Quantity > MaxQuantity {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_quantity", p + ".quantity", "must be positive and within the supported bound"})
		}
		if catalog.ContentIDs != nil && !registered(catalog.ContentIDs, c.ReferenceID) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"unregistered_content", p + ".reference_id", "content reference is not registered"})
		}
		if c.ObjectiveID == "" {
			continue
		}
		if c.Kind != "reward" {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"content_completes_objective", p + ".objective_id", "only a reward can complete an objective"})
		}
		if !objectives[c.ObjectiveID] {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"unknown_objective", p + ".objective_id", "objective is not declared"})
		} else if completedBy[c.ObjectiveID] != "" {
			// Two rewards completing one objective would make the panel's tick depend
			// on which was taken first, and would let the same goal be reached twice.
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_objective_completion", p + ".objective_id", "objective is already completed by another reward"})
		} else {
			completedBy[c.ObjectiveID] = c.ID
		}
	}
	for _, id := range def.EntryAreaIDs {
		if !validID(id) || !areas[id] {
			continue
		}
	}
	if len(def.EntryAreaIDs) == 0 || len(def.ExitAreaIDs) == 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"missing_endpoints", "entry_area_ids", "at least one entry and exit are required"})
	}
	reachable := reachableAreas(def)
	for _, id := range def.ExitAreaIDs {
		if reachable[id] {
			result.ReachableExitAreaIDs = append(result.ReachableExitAreaIDs, id)
		} else {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"unreachable_exit", "exit_area_ids", "exit is not graph-reachable from any entry"})
		}
	}
	abilities := make(map[string]bool)
	for _, id := range catalog.AbilityIDs {
		abilities[id] = true
	}
	available := make(map[string]bool)
	for _, id := range availableAbilities {
		available[id] = true
	}
	playablePath := false
	pathIDs := make(map[string]bool)
	for i, path := range def.CompletionPaths {
		p := fmt.Sprintf("completion_paths[%d]", i)
		if !validID(path.ID) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"invalid_id", p + ".id", "must be a stable identifier"})
		}
		if pathIDs[path.ID] {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_id", p + ".id", "completion path identifier is duplicated"})
		}
		pathIDs[path.ID] = true
		if len(path.ObjectiveIDs) == 0 {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{"empty_completion_path", p + ".objective_ids", "each completion path must reference at least one objective"})
		}
		seenObjectives := make(map[string]bool, len(path.ObjectiveIDs))
		for _, id := range path.ObjectiveIDs {
			if seenObjectives[id] {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{"duplicate_objective_reference", p + ".objective_ids", "objective reference is duplicated"})
			}
			seenObjectives[id] = true
			if !objectives[id] {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{"unknown_objective", p + ".objective_ids", "objective reference does not exist"})
			}
		}
		missing := []string{}
		for _, id := range path.ChallengeIDs {
			if !challenges[id] {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{"unknown_path_challenge", p + ".challenge_ids", "challenge reference does not exist"})
			}
		}
		for _, id := range path.RequiredAbilityIDs {
			if catalog.AbilityIDs != nil && !abilities[id] {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{"unregistered_ability", p + ".required_ability_ids", "ability is not registered"})
			} else if availableAbilities != nil && !available[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		pathAvailable := len(missing) == 0
		if pathAvailable {
			playablePath = true
		}
		result.PathDiagnostics = append(result.PathDiagnostics, PathDiagnostic{PathID: path.ID, Available: pathAvailable, MissingAbilityIDs: missing})
	}
	result.Diagnostics = append(result.Diagnostics, validateExploreRules(def)...)
	result.Playable = len(result.Diagnostics) == 0 && len(result.ReachableExitAreaIDs) > 0 && playablePath
	return result
}

func requireArea(result *DungeonValidation, areas map[string]bool, id, path string) {
	if !validID(id) || !areas[id] {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{"unknown_area", path, "area reference does not exist"})
	}
}
func registered(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func reachableAreas(def DungeonDefinition) map[string]bool {
	seen := map[string]bool{}
	queue := []string{}
	for _, id := range def.EntryAreaIDs {
		if !seen[id] {
			seen[id] = true
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		here := queue[0]
		queue = queue[1:]
		for _, edge := range def.Connections {
			next := ""
			if edge.FromAreaID == here {
				next = edge.ToAreaID
			} else if edge.Bidirectional && edge.ToAreaID == here {
				next = edge.FromAreaID
			}
			if next != "" && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

func ValidateInstanceBinding(binding DungeonInstanceBinding, scope DungeonScope, def DungeonDefinition) []Diagnostic {
	var ds []Diagnostic
	ds = append(ds, requireID(scope.WorldID, "expected_world_id")...)
	ds = append(ds, requireID(scope.BranchID, "expected_branch_id")...)
	fields := []struct{ v, p string }{{binding.InstanceID, "instance_id"}, {binding.WorldID, "world_id"}, {binding.BranchID, "branch_id"}, {binding.DefinitionID, "definition_id"}, {binding.DefinitionVersion, "definition_version"}, {binding.PackageID, "package_id"}, {binding.PackageVersion, "package_version"}, {binding.RulesetID, "ruleset_id"}, {binding.RulesetVersion, "ruleset_version"}}
	for _, f := range fields {
		ds = append(ds, requireID(f.v, f.p)...)
	}
	if binding.WorldID != scope.WorldID {
		ds = append(ds, Diagnostic{"world_scope_mismatch", "world_id", "instance belongs to a different trusted world scope"})
	}
	if binding.BranchID != scope.BranchID {
		ds = append(ds, Diagnostic{"branch_scope_mismatch", "branch_id", "instance belongs to a different trusted branch scope"})
	}
	if binding.DefinitionID != def.ID || binding.DefinitionVersion != def.Version || binding.PackageID != def.PackageID || binding.PackageVersion != def.PackageVersion || binding.RulesetID != def.RulesetID || binding.RulesetVersion != def.RulesetVersion {
		ds = append(ds, Diagnostic{"binding_mismatch", "binding", "instance references must exactly match the selected definition"})
	}
	return ds
}
