package exploration

import (
	"sort"
	"strconv"
)

type KnownFact struct {
	ID                  string `json:"id"`
	Text                string `json:"text"`
	TextKey             string `json:"text_key,omitempty"`
	PubliclyDisplayable bool   `json:"publicly_displayable"`
}
type KnowledgeSnapshot struct {
	CurrentAreaID           string      `json:"current_area_id"`
	KnownObjectiveIDs       []string    `json:"known_objective_ids"`
	DiscoveredAreaIDs       []string    `json:"discovered_area_ids"`
	DiscoveredConnectionIDs []string    `json:"discovered_connection_ids"`
	ConfirmedFacts          []KnownFact `json:"confirmed_facts"`
	Rumors                  []KnownFact `json:"rumors"`
}
type KnownView struct {
	CurrentArea    Area                `json:"current_area"`
	Objectives     []Objective         `json:"objectives"`
	Areas          []Area              `json:"areas"`
	Connections    []DungeonConnection `json:"connections"`
	ConfirmedFacts []KnownFact         `json:"confirmed_facts"`
	Rumors         []KnownFact         `json:"rumors"`
}

// ProjectKnownView is the low-level projection for callers that have already
// established the instance binding against their trusted world/branch scope.
// Prefer ProjectBoundKnownView at an application boundary.
func ProjectKnownView(def DungeonDefinition, snapshot KnowledgeSnapshot) (KnownView, []Diagnostic) {
	view := emptyKnownView()
	var ds []Diagnostic
	if len(snapshot.KnownObjectiveIDs)+len(snapshot.DiscoveredAreaIDs)+len(snapshot.DiscoveredConnectionIDs)+len(snapshot.ConfirmedFacts)+len(snapshot.Rumors) > MaxKnowledgeItems {
		return view, []Diagnostic{{"too_many_knowledge_items", "snapshot", "combined knowledge exceeds supported limit"}}
	}
	areaByID := map[string]Area{}
	objectiveByID := map[string]Objective{}
	connectionByID := map[string]DungeonConnection{}
	for _, a := range def.Areas {
		areaByID[a.ID] = a
	}
	for _, o := range def.Objectives {
		objectiveByID[o.ID] = o
	}
	for _, c := range def.Connections {
		connectionByID[c.ID] = c
	}
	current, ok := areaByID[snapshot.CurrentAreaID]
	if !validID(snapshot.CurrentAreaID) || !ok {
		ds = append(ds, Diagnostic{"invalid_current_area", "current_area_id", "current location must reference a defined area"})
	} else {
		view.CurrentArea = current
	}
	knownAreas := map[string]bool{snapshot.CurrentAreaID: true}
	discoveredAreaIDs := map[string]bool{}
	for i, id := range snapshot.DiscoveredAreaIDs {
		if _, ok := areaByID[id]; !validID(id) || !ok {
			ds = append(ds, Diagnostic{"unknown_area", indexPath("discovered_area_ids", i), "discovered area does not exist"})
			continue
		}
		if discoveredAreaIDs[id] {
			ds = append(ds, Diagnostic{"duplicate_knowledge_id", indexPath("discovered_area_ids", i), "area is duplicated"})
			continue
		}
		discoveredAreaIDs[id] = true
		knownAreas[id] = true
	}
	knownObjectiveIDs := map[string]bool{}
	for i, id := range snapshot.KnownObjectiveIDs {
		o, ok := objectiveByID[id]
		if !validID(id) || !ok {
			ds = append(ds, Diagnostic{"unknown_objective", indexPath("known_objective_ids", i), "known objective does not exist"})
			continue
		}
		if knownObjectiveIDs[id] {
			ds = append(ds, Diagnostic{"duplicate_knowledge_id", indexPath("known_objective_ids", i), "objective is duplicated"})
			continue
		}
		knownObjectiveIDs[id] = true
		view.Objectives = append(view.Objectives, o)
	}
	discoveredConnectionIDs := map[string]bool{}
	for i, id := range snapshot.DiscoveredConnectionIDs {
		c, ok := connectionByID[id]
		if !validID(id) || !ok {
			ds = append(ds, Diagnostic{"unknown_connection", indexPath("discovered_connection_ids", i), "discovered connection does not exist"})
			continue
		}
		if discoveredConnectionIDs[id] {
			ds = append(ds, Diagnostic{"duplicate_knowledge_id", indexPath("discovered_connection_ids", i), "connection is duplicated"})
			continue
		}
		discoveredConnectionIDs[id] = true
		if c.PlayerVisible && knownAreas[c.FromAreaID] && knownAreas[c.ToAreaID] {
			view.Connections = append(view.Connections, c)
		}
	}
	for id := range knownAreas {
		if a, ok := areaByID[id]; ok {
			view.Areas = append(view.Areas, a)
		}
	}
	view.ConfirmedFacts = projectFacts(snapshot.ConfirmedFacts, "confirmed_facts", &ds)
	view.Rumors = projectFacts(snapshot.Rumors, "rumors", &ds)
	confirmedIDs := map[string]bool{}
	for _, fact := range view.ConfirmedFacts {
		confirmedIDs[fact.ID] = true
	}
	for i, rumor := range view.Rumors {
		if confirmedIDs[rumor.ID] {
			ds = append(ds, Diagnostic{"conflicting_knowledge_kind", indexPath("rumors", i), "an item cannot be both confirmed and rumor"})
		}
	}
	sort.Slice(view.Objectives, func(i, j int) bool { return view.Objectives[i].ID < view.Objectives[j].ID })
	sort.Slice(view.Areas, func(i, j int) bool { return view.Areas[i].ID < view.Areas[j].ID })
	sort.Slice(view.Connections, func(i, j int) bool { return view.Connections[i].ID < view.Connections[j].ID })
	return view, ds
}

// ProjectBoundKnownView verifies the instance against the caller's trusted
// current scope before projecting any knowledge for that instance.
func ProjectBoundKnownView(def DungeonDefinition, binding DungeonInstanceBinding, scope DungeonScope, snapshot KnowledgeSnapshot) (KnownView, []Diagnostic) {
	if diagnostics := ValidateInstanceBinding(binding, scope, def); len(diagnostics) > 0 {
		return emptyKnownView(), diagnostics
	}
	return ProjectKnownView(def, snapshot)
}

func emptyKnownView() KnownView {
	return KnownView{Objectives: []Objective{}, Areas: []Area{}, Connections: []DungeonConnection{}, ConfirmedFacts: []KnownFact{}, Rumors: []KnownFact{}}
}

func projectFacts(items []KnownFact, path string, ds *[]Diagnostic) []KnownFact {
	out := make([]KnownFact, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		p := indexPath(path, i)
		if !validID(item.ID) || !validText(item.Text, MaxDescriptionBytes) {
			*ds = append(*ds, Diagnostic{"invalid_knowledge_item", p, "fact requires a stable id and non-empty bounded text"})
			continue
		}
		if seen[item.ID] {
			*ds = append(*ds, Diagnostic{"duplicate_knowledge_id", p + ".id", "knowledge ids must be unique within the list"})
			continue
		}
		seen[item.ID] = true
		if item.PubliclyDisplayable {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func indexPath(list string, index int) string { return list + "[" + strconv.Itoa(index) + "]" }
