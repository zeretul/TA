package exploration

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
)

func finishInstanceTransition(before, after DungeonInstanceSnapshot, actionSourceRef string) (InstanceTransition, error) {
	before = cloneInstanceSnapshot(before)
	after = cloneInstanceSnapshot(after)
	if reflect.DeepEqual(before, after) {
		return InstanceTransition{Snapshot: cloneInstanceSnapshot(before), Diff: InstanceDiff{FromRevision: before.Revision, ToRevision: before.Revision, Changes: []InstanceChange{}}}, nil
	}
	if before.Revision == math.MaxUint64 {
		return InstanceTransition{}, ErrRevisionOverflow
	}
	after.Revision = before.Revision + 1
	changes := diffInstanceSnapshots(before, after, actionSourceRef)
	return InstanceTransition{Snapshot: cloneInstanceSnapshot(after), Diff: InstanceDiff{FromRevision: before.Revision, ToRevision: after.Revision, Changes: changes}}, nil
}

func diffInstanceSnapshots(before, after DungeonInstanceSnapshot, actionSourceRef string) []InstanceChange {
	changes := []InstanceChange{}
	if before.Presence != after.Presence {
		changes = append(changes, InstanceChange{Kind: "presence_changed", From: string(before.Presence), To: string(after.Presence), SourceRef: actionSourceRef})
	}
	if before.CurrentAreaID != after.CurrentAreaID {
		changes = append(changes, InstanceChange{Kind: "position_changed", From: before.CurrentAreaID, To: after.CurrentAreaID, SourceRef: actionSourceRef})
	}
	if before.LastExitAreaID != after.LastExitAreaID {
		changes = append(changes, InstanceChange{Kind: "last_exit_changed", From: before.LastExitAreaID, To: after.LastExitAreaID, SourceRef: actionSourceRef})
	}
	prior := map[string]InstanceDiscovery{}
	for _, item := range before.Discoveries {
		prior[string(item.Kind)+"\x00"+item.ID] = item
	}
	for _, item := range after.Discoveries {
		key := string(item.Kind) + "\x00" + item.ID
		if old, exists := prior[key]; exists {
			delete(prior, key)
			_ = old
			continue
		}
		if item.Kind == DiscoveryFact {
			if _, wasRumor := prior[string(DiscoveryRumor)+"\x00"+item.ID]; wasRumor {
				delete(prior, string(DiscoveryRumor)+"\x00"+item.ID)
				changes = append(changes, InstanceChange{Kind: "rumor_confirmed", ID: item.ID, SourceRef: actionSourceRef})
				continue
			}
		}
		change := discoveryChange(item)
		change.SourceRef = actionSourceRef
		changes = append(changes, change)
	}
	for _, item := range after.CompletedObjectives {
		if !containsSourcedID(before.CompletedObjectives, item.ID) {
			changes = append(changes, InstanceChange{Kind: "objective_completed", ID: item.ID, SourceRef: actionSourceRef})
		}
	}
	for _, item := range after.ResolvedChallenges {
		if !containsSourcedID(before.ResolvedChallenges, item.ID) {
			changes = append(changes, InstanceChange{Kind: "challenge_resolved", ID: item.ID, SourceRef: actionSourceRef})
		}
	}
	for _, reward := range after.GrantedRewards {
		if containsRewardID(before.GrantedRewards, reward.ID) {
			continue
		}
		changes = append(changes, InstanceChange{Kind: "reward_granted", ID: reward.ID, To: reward.RefID, SourceRef: actionSourceRef})
	}
	if !reflect.DeepEqual(before.ExploreState, after.ExploreState) && after.ExploreState != nil {
		changes = append(changes, InstanceChange{Kind: "exploration_changed", To: strconv.Itoa(after.ExploreState.Pressure), SourceRef: actionSourceRef})
	}
	sortChanges(changes)
	return changes
}

func containsRewardID(items []InstanceReward, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func discoveryChange(item InstanceDiscovery) InstanceChange {
	return InstanceChange{Kind: "discovered_" + string(item.Kind), ID: item.ID, SourceRef: item.SourceRef}
}

func sortChanges(changes []InstanceChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		return changes[i].ID < changes[j].ID
	})
}

func sortDiscoveries(items []InstanceDiscovery) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ID < items[j].ID
	})
}

func cloneInstanceSnapshot(snapshot DungeonInstanceSnapshot) DungeonInstanceSnapshot {
	snapshot.ExploreState = cloneExploreState(snapshot.ExploreState)
	snapshot.Discoveries = append([]InstanceDiscovery{}, snapshot.Discoveries...)
	snapshot.CompletedObjectives = append([]SourcedProgress{}, snapshot.CompletedObjectives...)
	snapshot.ResolvedChallenges = append([]SourcedProgress{}, snapshot.ResolvedChallenges...)
	snapshot.GrantedRewards = append([]InstanceReward{}, snapshot.GrantedRewards...)
	return snapshot
}

func emptyInstanceView(instanceID string, presence InstancePresence) DungeonInstanceView {
	return DungeonInstanceView{InstanceID: instanceID, Presence: presence, Objectives: []Objective{}, Areas: []Area{}, Connections: []DungeonConnection{}, ConfirmedFacts: []KnownFact{}, Rumors: []KnownFact{}, CompletedObjectives: []string{}, ResolvedChallenges: []string{}, GrantedRewards: []InstanceReward{}}
}

func instanceDiagnosticError(ds []Diagnostic) error {
	if len(ds) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s at %s", ErrInvalidInstance, ds[0].Message, ds[0].Path)
}

func definitionIndexes(def DungeonDefinition) (map[string]Area, map[string]DungeonConnection, map[string]Objective, map[string]Challenge) {
	areas := make(map[string]Area, len(def.Areas))
	connections := make(map[string]DungeonConnection, len(def.Connections))
	objectives := make(map[string]Objective, len(def.Objectives))
	challenges := make(map[string]Challenge, len(def.Challenges))
	for _, area := range def.Areas {
		areas[area.ID] = area
	}
	for _, connection := range def.Connections {
		connections[connection.ID] = connection
	}
	for _, objective := range def.Objectives {
		objectives[objective.ID] = objective
	}
	for _, challenge := range def.Challenges {
		challenges[challenge.ID] = challenge
	}
	return areas, connections, objectives, challenges
}

func findConnection(def DungeonDefinition, id string) (DungeonConnection, bool) {
	_, connections, _, _ := definitionIndexes(def)
	item, ok := connections[id]
	return item, ok
}

func findChallenge(def DungeonDefinition, id string) Challenge {
	_, _, _, challenges := definitionIndexes(def)
	return challenges[id]
}

func hasArea(def DungeonDefinition, id string) bool {
	areas, _, _, _ := definitionIndexes(def)
	_, ok := areas[id]
	return ok
}
func hasConnection(def DungeonDefinition, id string) bool {
	_, connections, _, _ := definitionIndexes(def)
	_, ok := connections[id]
	return ok
}
func hasObjective(def DungeonDefinition, id string) bool {
	_, _, objectives, _ := definitionIndexes(def)
	_, ok := objectives[id]
	return ok
}
func hasChallenge(def DungeonDefinition, id string) bool {
	_, _, _, challenges := definitionIndexes(def)
	_, ok := challenges[id]
	return ok
}

func isFactKind(kind InstanceDiscoveryKind) bool {
	return kind == DiscoveryFact || kind == DiscoveryRumor
}

func sameDiscoveryContent(left, right InstanceDiscovery) bool {
	return left.Kind == right.Kind && left.ID == right.ID && left.Text == right.Text && left.TextKey == right.TextKey && left.PubliclyDisplayable == right.PubliclyDisplayable
}

func containsSourcedID(items []SourcedProgress, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
