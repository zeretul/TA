package interactive

import (
	"ta/internal/exploration"
	"testing"
)

func TestRealmExploreExplicitIncapacityIsNotMissingHealth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		health  *int
		blocked bool
	}{{"missing", nil, false}, {"zero", intPointerForRealm(0), true}, {"positive", intPointerForRealm(1), false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, id := realmTestStory(t)
			snapshot, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			state := cloneStoryState(snapshot.State)
			if tc.health != nil {
				applyStateDeltaToProjection(state, newStateDeltaWithActorOps(nil, []ActorStateOp{{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldHealth, Value: *tc.health}}))
			}
			err = realmActionBlocked(&storyBranchProjection{State: state}, state, StoryMeta{})
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, got %v", tc.blocked, err)
			}
		})
	}
}
func intPointerForRealm(n int) *int { return &n }

func TestRealmExploreProjectedMethodsRespectRealIncapacity(t *testing.T) {
	s, id, _ := realmExploreStory(t, "fanren-jie", realmTestOrigin, nil, nil)
	entered := realmTestApply(t, s, id, RealmRequest{RealmID: exploration.RealmMountainHerbCave, Action: RealmActionEnter})
	snapshot, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	state := cloneStoryState(snapshot.State)
	applyStateDeltaToProjection(state, newStateDeltaWithActorOps(nil, []ActorStateOp{{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldHealth, Value: 0}}))
	def, _ := exploration.RealmDefinition(entered.RealmID)
	view := realmExploreView(StoryMeta{}, &storyBranchProjection{State: state}, def, exploration.DungeonScope{WorldID: entered.Instance.Binding.WorldID, BranchID: "main"}, entered.Instance)
	if view == nil || len(view.Approaches) == 0 {
		t.Fatal("known methods must remain visible")
	}
	found := false
	for _, a := range view.Approaches {
		if a.Available {
			t.Fatal("incapacitated actor was offered an executable method")
		}
		if a.BlockedReason == "cannot_act" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing truthful incapacity reason")
	}
}
