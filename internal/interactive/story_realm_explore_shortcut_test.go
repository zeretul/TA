package interactive

import (
	"reflect"
	"testing"

	"ta/internal/exploration"
)

func realmExploreRouteOffered(t *testing.T, s *Store, id, branch, route string) bool {
	t.Helper()
	state, err := s.WorldStateFor(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range state.Journeys.Routes {
		if option.RouteID == route {
			return true
		}
	}
	return false
}

func realmExploreRejectJourneyUnchanged(t *testing.T, s *Store, id, branch, route, command string) {
	t.Helper()
	before, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	journeys := journeyTestRecordCount(t, s, id, branch)
	_, err = s.ApplyJourney(id, JourneyRequest{BranchID: branch, RouteID: route, Arrive: true, CommandID: command, CommandHash: command})
	if err == nil {
		t.Fatalf("unavailable shortcut %s was accepted", route)
	}
	after, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) || before.ContextRevision != after.ContextRevision || !reflect.DeepEqual(before.Graph, after.Graph) || journeyTestRecordCount(t, s, id, branch) != journeys {
		t.Fatalf("guessed or misplaced shortcut %s partially spent time or changed story state", route)
	}
}

// A discovered side area is not the same as walking it. This test uses formal
// Store actions to establish a durable route fact, with no cache pickup required.
func TestRealmExploreShortcutNeedsSideVisitAndPersistsAcrossForkAndReload(t *testing.T) {
	realm := exploration.RealmShaftServiceBay
	forward, reverse := "realm-shortcut-servicebay:forward", "realm-shortcut-servicebay:reverse"
	s, id, dir := realmExploreStory(t, "xinghai-689", "朱伯利外环47号减压长廊", nil, nil)
	opening := journeyTestOpeningTurnID(t, s, id)
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "未探索分支"})
	if err != nil {
		t.Fatal(err)
	}
	if realmExploreRouteOffered(t, s, id, "main", forward) {
		t.Fatal("secret shortcut leaked before it was discovered")
	}
	realmExploreRejectJourneyUnchanged(t, s, id, "main", forward, "guess-known")
	realmExploreRejectJourneyUnchanged(t, s, id, "main", "realm-shortcut-invented:forward", "guess-unknown")
	journeyTestApply(t, s, id, JourneyRequest{RouteID: "xinghai-route-l01-l05", Arrive: true, CommandID: "normal-road", CommandHash: "normal-road"})
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	realmExploreDo(t, s, id, entered.InstanceID, realm, "survey", "survey")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "approach", "slow")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "preserve")
	if realmExploreViewAt(t, s, id, "main").Explore.Pressure != 3 {
		t.Fatal("ordinary survey/slow/preserve path must fit under the risk limit")
	}
	questWait(t, s, id, "main", 3600)
	realmExploreDo(t, s, id, entered.InstanceID, realm, "approach", "return")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "leave-before-side")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit-before-side"))
	if realmExploreRouteOffered(t, s, id, "main", reverse) {
		t.Fatal("return method revealed the side area but no one walked it; shortcut must remain closed")
	}
	realmExploreRejectJourneyUnchanged(t, s, id, "main", reverse, "guess-after-return")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionReenter, "", "reenter"))
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "return-core")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "side", "visit-side")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "back-core")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "back-entry")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
	if !realmExploreRouteOffered(t, s, id, "main", reverse) {
		t.Fatal("a real side visit did not unlock the local shortcut")
	}
	if realmExploreQuantity(t, s, id, "main", "xinghai-689", "xinghai-i37") != 0 {
		t.Fatal("walking a permanent shortcut also granted unclaimed cache materials")
	}
	if realmExploreRouteOffered(t, s, id, fork.ID, forward) {
		t.Fatal("shortcut knowledge leaked into an earlier branch")
	}
	realmExploreRejectJourneyUnchanged(t, s, id, fork.ID, forward, "fork-guess")
	realmExploreRejectJourneyUnchanged(t, s, id, "main", forward, "wrong-end")
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(dir)
	defer reopened.Close()
	if !realmExploreRouteOffered(t, reopened, id, "main", reverse) || realmExploreRouteOffered(t, reopened, id, fork.ID, forward) {
		t.Fatal("cold reopen lost branch-local shortcut discovery")
	}
	before := worldFlowClock(t, reopened, id).Seconds
	request := JourneyRequest{RouteID: reverse, Arrive: true, CommandID: "short-reverse", CommandHash: "short-reverse"}
	reverseTrip := journeyTestApply(t, reopened, id, request)
	if reverseTrip.ElapsedSeconds != 300 || worldFlowClock(t, reopened, id).Seconds != before+300 {
		t.Fatal("shortcut reverse must actually take five minutes, not the old road's fifteen")
	}
	if !realmExploreRouteOffered(t, reopened, id, "main", forward) || realmExploreRouteOffered(t, reopened, id, "main", reverse) {
		t.Fatal("shortcut offered the wrong endpoint after travel")
	}
	journeyTestApply(t, reopened, id, request)
	if worldFlowClock(t, reopened, id).Seconds != before+300 {
		t.Fatal("shortcut retry spent a second interval")
	}
	forwardTrip := journeyTestApply(t, reopened, id, JourneyRequest{RouteID: forward, Arrive: true, CommandID: "short-forward", CommandHash: "short-forward"})
	if forwardTrip.ElapsedSeconds != 300 || worldFlowClock(t, reopened, id).Seconds != before+600 {
		t.Fatal("shortcut must remain usable in both directions")
	}
	if realmExploreQuantity(t, reopened, id, "main", "xinghai-689", "xinghai-i37") != 0 {
		t.Fatal("permanent route traversal regenerated finite cache rewards")
	}
}
