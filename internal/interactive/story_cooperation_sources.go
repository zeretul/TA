package interactive

import (
	"ta/internal/character"
	"ta/internal/exploration"
)

// This cache is rebuilt solely from canonical Realm records. Snapshot discovery
// source labels are method names, not journal identities, and cannot stand in
// for an actual visit or result when knowledge is shared with somebody else.
func realmExploreSources(prior RealmInstanceState, event RealmRecordEvent) map[string]string {
	refs := cloneExploreSources(prior.ExploreSources)
	if refs == nil {
		refs = map[string]string{}
	}
	s := event.Instance
	if s.ExploreState == nil {
		return refs
	}
	add := func(kind string, yes bool) {
		if yes && refs[kind] == "" {
			refs[kind] = event.ID
		}
	}
	clue, side := false, false
	for _, d := range s.Discoveries {
		if d.Kind == exploration.DiscoveryFact && d.ID == event.RealmID+".clue" {
			clue = true
		}
		if d.Kind == exploration.DiscoveryArea && d.ID == event.RealmID+".side" {
			side = true
		}
	}
	add("clue", event.Action == RealmActionSurvey && event.ApproachID == event.RealmID+".survey" && clue)
	for _, d := range s.ExploreState.Decisions {
		add("ingress", d.GroupID == "ingress" && event.ApproachID == d.ApproachID && !s.ExploreState.Setback)
		add("disposition", d.GroupID == "disposition" && event.ApproachID == d.ApproachID && !s.ExploreState.Setback)
	}
	add("setback", s.ExploreState.Setback && (prior.Snapshot.ExploreState == nil || !prior.Snapshot.ExploreState.Setback))
	add("side", event.Action == RealmActionMove && s.CurrentAreaID == event.RealmID+".side" && side)
	return refs
}

func cloneExploreSources(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	copy := make(map[string]string, len(source))
	for k, v := range source {
		copy[k] = v
	}
	return copy
}

// A trusted fork inherits completed work exactly as it inherits an inventory.
// Only the current capability scope is rebound; original consent, work, use and
// knowledge journal sources are never rewritten. Ordinary character history is
// untouched, and a read/request can never invoke this fork operation.
func forkCooperationCharacters(state *character.State, realms map[string]RealmInstanceState, branchID string, worlds ...*WorldContent) *character.State {
	copy := cloneCharacterState(state)
	if copy == nil {
		return nil
	}
	for i, m := range copy.Matters {
		c := m.Cooperation
		if c == nil {
			continue
		}
		instance, ok := realms[c.RealmInstanceID]
		if !ok {
			continue
		}
		def, ok := cooperationProject(c.ProjectID, worlds...)
		if !ok || m.StoryID != instance.Snapshot.Binding.WorldID || def.RealmID != instance.Snapshot.Binding.DefinitionID || c.ProjectVersion != def.Version {
			continue
		}
		m.BranchID = branchID
		copy.Matters[i] = m
	}
	return copy
}
