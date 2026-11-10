package interactive

import (
	"errors"
	"testing"

	"ta/internal/exploration"
)

// These tests are §4 of 自制-探索与行路 as it happens during play: the player sets
// out from where the character really stands, the registered connection fixes the
// distance and the in-fiction time, and the travel and the new place are committed
// once.
//
// The rules layer already proves the segment arithmetic — exploration's own suite
// covers AdvanceTo's boundaries and offsets. What these tests add is that the same
// arithmetic is reachable from the story's one formal clock and one journal, and
// that a stop, a continuation, a retry and a fork all read the same numbers.

// journeyTestSystem is the real xiuxian preset with the protagonist standing at a
// named place, so the route's origin check has a scene to read.
func journeyTestSystem(location string) StoryDirectorActorStateSystem {
	system := xiuxianTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State[storySceneLocationField] = location
	}
	return system
}

// journeyTestStory builds a story whose scene places the protagonist at the
// route's origin and whose branch has a head a side record can be parented to.
func journeyTestStory(t *testing.T, location string) (*Store, string) {
	t.Helper()
	system := journeyTestSystem(location)
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "行路", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出门", Narrative: "主角在散修人家门前收拾行囊，准备前往坊市。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// journeyTestApply settles one stretch and fails the test if the store refuses it.
func journeyTestApply(t *testing.T, store *Store, storyID string, req JourneyRequest) JourneyResult {
	t.Helper()
	if req.BranchID == "" {
		req.BranchID = "main"
	}
	result, err := store.ApplyJourney(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// journeyTestLocation reads the committed scene location, which is what the story
// window and the next turn both read.
func journeyTestLocation(t *testing.T, store *Store, storyID, branchID string) string {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	return storySceneText(snapshot.State, storySceneLocationField)
}

// journeyTestRecordCount counts the journey records on the branch's journal.
func journeyTestRecordCount(t *testing.T, store *Store, storyID, branchID string) int {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, branchID)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeJourneyRecord {
			count++
		}
	}
	return count
}

// journeyTestOpeningTurnID is the turn a fork must be cut at to sit before any
// travel the branch committed after it.
func journeyTestOpeningTurnID(t *testing.T, store *Store, storyID string) string {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeTurn {
			return record.Envelope.ID
		}
	}
	t.Fatal("the opening turn must be on the journal")
	return ""
}

// TestAJourneyCommitsItsDistanceAndPlaceOnce is the batch's whole point: one
// ordinary journey moves the formal clock by the route's own duration and writes
// the place the character reached, in one transaction.
func TestAJourneyCommitsItsDistanceAndPlaceOnce(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	result := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-walk", CommandHash: "h-walk",
	})

	if result.AddedMinutes != 30 || result.ElapsedSeconds != 30*60 {
		t.Fatalf("the 30-minute route must settle 30 minutes: added=%d elapsed=%d",
			result.AddedMinutes, result.ElapsedSeconds)
	}
	if result.ClockAfter.Seconds != 30*60 {
		t.Fatalf("the branch's formal instant must move by the road's own duration: %d", result.ClockAfter.Seconds)
	}
	if !result.Arrived || result.Remaining != 0 {
		t.Fatalf("walking to the end must report an arrival: %#v", result)
	}
	if result.LocationLabel != "公共坊市" {
		t.Fatalf("the arrival must name the destination: %q", result.LocationLabel)
	}
	// The place write is the scene fact the next turn reads.
	if got := journeyTestLocation(t, store, storyID, "main"); got != "公共坊市" {
		t.Fatalf("the committed scene must name where the character stands: %q", got)
	}
	// The journey itself is on the branch, finished.
	clock, journeys, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != 30*60 {
		t.Fatalf("the read must report the same instant the commit wrote: %d", clock.Seconds)
	}
	snapshot, ok := journeys[result.JourneyID]
	if !ok || snapshot.ElapsedMinutes != 30 || snapshot.TotalMinutes != 30 {
		t.Fatalf("the committed journey must read back finished: %#v", journeys)
	}
}

// TestARetryAddsNoSecondJourney is §4.2's "重试不重复计时".
//
// The attempt may have committed before the response was lost, so the retry has
// to address the same stretch. It must report the committed answer and add
// nothing, rather than walking the road a second time.
func TestARetryAddsNoSecondJourney(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	first := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-retry", CommandHash: "h-retry",
	})
	retry := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-retry", CommandHash: "h-retry",
	})

	if !retry.Replayed {
		t.Fatal("a retry under the same command must report itself as already on record")
	}
	if retry.AddedMinutes != 0 || retry.ElapsedSeconds != 0 {
		t.Fatalf("a retry must add no time: added=%d elapsed=%d", retry.AddedMinutes, retry.ElapsedSeconds)
	}
	if retry.ClockAfter.Seconds != first.ClockAfter.Seconds {
		t.Fatalf("a retry must not move the clock again: %d vs %d",
			retry.ClockAfter.Seconds, first.ClockAfter.Seconds)
	}
	if retry.JourneyID != first.JourneyID || retry.EventID != first.EventID {
		t.Fatalf("a retry must report the first attempt: %#v vs %#v", retry, first)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("a retry must not commit a second record: %d", count)
	}
}

// TestAStopCommitsOnlyTheDistanceWalked is §4.2's "只提交实际走到停点的路程".
//
// A stop between two places covers only what it walked, writes no place — the
// character has not reached anywhere — and leaves the remainder owing. The
// continuation then covers only that remainder.
func TestAStopCommitsOnlyTheDistanceWalked(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	stopped := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", TargetMinutes: 15,
		CommandID: "cmd-stop", CommandHash: "h-stop",
	})
	if stopped.AddedMinutes != 15 || stopped.ElapsedSeconds != 15*60 {
		t.Fatalf("a stop at 15 must cover 15 minutes: added=%d elapsed=%d",
			stopped.AddedMinutes, stopped.ElapsedSeconds)
	}
	if stopped.Arrived {
		t.Fatal("a stop between two places must not report an arrival")
	}
	if stopped.LocationLabel != "" {
		t.Fatalf("a mid-segment stop must name no place: %q", stopped.LocationLabel)
	}
	if stopped.Remaining != 15 {
		t.Fatalf("a stop at 15 of 30 must leave 15 owing: %d", stopped.Remaining)
	}
	// The scene still says where the character really is; claiming the
	// destination would teleport them past the road they have not walked.
	if got := journeyTestLocation(t, store, storyID, "main"); got != "越国坊市外围散修人家" {
		t.Fatalf("a mid-segment stop must not write a place: %q", got)
	}

	// The continuation covers only the remainder, and the total is reached once.
	continued := journeyTestApply(t, store, storyID, JourneyRequest{
		JourneyID: stopped.JourneyID, Arrive: true,
		CommandID: "cmd-continue", CommandHash: "h-continue",
	})
	if continued.AddedMinutes != 15 {
		t.Fatalf("the continuation must cover only the remainder: %d", continued.AddedMinutes)
	}
	if continued.ClockAfter.Seconds != 30*60 {
		t.Fatalf("the two stretches together must be one route's worth of time: %d", continued.ClockAfter.Seconds)
	}
	if !continued.Arrived || continued.LocationLabel != "公共坊市" {
		t.Fatalf("the continuation must reach the destination: %#v", continued)
	}
	if got := journeyTestLocation(t, store, storyID, "main"); got != "公共坊市" {
		t.Fatalf("arriving must write the destination: %q", got)
	}
}

// TestAJourneyRefusesToStartFromElsewhere is §4.1: travel starts from the current
// formal position, not from whichever road the caller named.
func TestAJourneyRefusesToStartFromElsewhere(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国黄枫谷药圃")

	_, err := store.ApplyJourney(storyID, JourneyRequest{
		BranchID: "main", RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-elsewhere", CommandHash: "h-elsewhere",
	})
	if !errors.Is(err, ErrJourneyNotAtOrigin) {
		t.Fatalf("a route whose origin is not the scene's place must be refused: %v", err)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 0 {
		t.Fatalf("a refused journey must commit nothing: %d", count)
	}
}

// TestAJourneyRefusesAPointAlreadyReached keeps a no-op from reading as travel.
func TestAJourneyRefusesAPointAlreadyReached(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	stopped := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", TargetMinutes: 15,
		CommandID: "cmd-half", CommandHash: "h-half",
	})
	_, err := store.ApplyJourney(storyID, JourneyRequest{
		BranchID: "main", JourneyID: stopped.JourneyID, TargetMinutes: 15,
		CommandID: "cmd-half-again", CommandHash: "h-half-again",
	})
	if !errors.Is(err, ErrJourneyNoProgress) {
		t.Fatalf("a target the journey is already at must be refused: %v", err)
	}
	clock, _, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != 15*60 {
		t.Fatalf("a refused journey must not move the clock: %d", clock.Seconds)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("a refused journey must commit no record: %d", count)
	}
}

// TestOneCommandIDCannotCarryTwoJourneys refuses to answer a different road under
// an identity that already means something else.
func TestOneCommandIDCannotCarryTwoJourneys(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-one", CommandHash: "h-one",
	})
	_, err := store.ApplyJourney(storyID, JourneyRequest{
		BranchID: "main", RouteID: "route_036:reverse", TargetMinutes: 15,
		CommandID: "cmd-one", CommandHash: "h-different",
	})
	if !errors.Is(err, ErrJourneyCommandConflict) {
		t.Fatalf("reusing one command id for different content must be refused: %v", err)
	}
}

// TestAForkDoesNotInheritARoadItNeverWalked mirrors the cultivation and recovery
// fork rules: a fork cut before the travel must not be standing on the road, and
// the branch it was cut from must still hold it.
func TestAForkDoesNotInheritARoadItNeverWalked(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")
	openingTurnID := journeyTestOpeningTurnID(t, store, storyID)

	result := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-fork", CommandHash: "h-fork",
	})

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: openingTurnID, Title: "早于行路"})
	if err != nil {
		t.Fatal(err)
	}
	forkClock, forkJourneys, err := store.JourneyState(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := forkJourneys[result.JourneyID]; found {
		t.Fatalf("a fork cut before the journey must not inherit it: %#v", forkJourneys)
	}
	if forkClock.Seconds != 0 {
		t.Fatalf("a fork cut before the journey must not inherit its spent time: %d", forkClock.Seconds)
	}
	// Main is untouched by the fork.
	mainClock, mainJourneys, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != 30*60 {
		t.Fatalf("the fork must not move main's instant: %d", mainClock.Seconds)
	}
	if _, found := mainJourneys[result.JourneyID]; !found {
		t.Fatalf("main must still hold the journey: %#v", mainJourneys)
	}
}

// TestOpeningTheEntryMovesNothing is the read's whole contract: a window that was
// opened, closed and reopened must find the same road at the same distance and
// must not spend world time for the privilege.
func TestOpeningTheEntryMovesNothing(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")

	options, err := store.JourneyOptionsFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if options.CurrentLocation != "越国坊市外围散修人家" {
		t.Fatalf("the entry must report where the character really stands: %q", options.CurrentLocation)
	}
	if len(options.Routes) != 1 || options.Routes[0].RouteID != "route_036:reverse" {
		t.Fatalf("the entry must offer the registered route: %#v", options.Routes)
	}
	if options.Routes[0].TotalMinutes != 30 {
		t.Fatalf("the entry must report the route's authored duration: %d", options.Routes[0].TotalMinutes)
	}
	clock, journeys, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != 0 || len(journeys) != 0 {
		t.Fatalf("opening the entry must move nothing: clock=%d journeys=%#v", clock.Seconds, journeys)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 0 {
		t.Fatalf("opening the entry must commit no record: %d", count)
	}
}

// TestTheCommittedRecordIsReplayable checks the projection half: the record the
// commit wrote must rebuild the same journey and the same instant from the
// journal alone, which is what a restart and a fork both rely on.
func TestTheCommittedRecordIsReplayable(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")
	result := journeyTestApply(t, store, storyID, JourneyRequest{
		RouteID: "route_036:reverse", TargetMinutes: 15,
		CommandID: "cmd-replay", CommandHash: "h-replay",
	})

	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeJourneyRecord {
			continue
		}
		found = true
		var event JourneyRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		replayed := &storyBranchProjection{}
		if err := applyJourneyRecord(replayed, event); err != nil {
			t.Fatal(err)
		}
		if replayed.Clock == nil || replayed.Clock.Seconds != result.ClockAfter.Seconds {
			t.Fatalf("replaying the record must restore the same instant: %#v", replayed.Clock)
		}
		snapshot, ok := replayed.Journeys[result.JourneyID]
		if !ok || snapshot.ElapsedMinutes != 15 || snapshot.CurrentSegmentID == "" {
			t.Fatalf("replaying the record must restore the half-walked road: %#v", replayed.Journeys)
		}
		if snapshot.SegmentElapsedMinutes != 15 {
			t.Fatalf("the offset inside the segment must survive the replay: %#v", snapshot)
		}
	}
	if !found {
		t.Fatal("the commit must have written a journey record")
	}
	// The exploration rules are the ones that decide, so the snapshot must also be
	// one they accept.
	route, _ := journeyRoute("route_036:reverse")
	if _, _, diagnostics := exploration.ValidateRoute(route, journeySupportedMethods()); len(diagnostics) != 0 {
		t.Fatalf("the registered route must validate: %#v", diagnostics)
	}
}

// journeyLifespanRealmState is the state a boundary question is asked against
// without a scene: the realm is the only field it reads.
func journeyLifespanRealmState() map[string]any {
	return lifespanTestState(qiStageKey(3))
}

// journeyLifespanTestProjection is a branch standing at a given formal instant,
// with the protagonist's realm and the scene's place both written.
func journeyLifespanTestProjection(seconds int64, location, realmKey string) *storyBranchProjection {
	return &storyBranchProjection{
		State: map[string]any{
			actorStateRoot: map[string]any{
				DefaultStoryContextActorID: map[string]any{
					"id": DefaultStoryContextActorID, "template_id": DefaultStoryContextActorID,
					"state": map[string]any{storySceneLocationField: location},
				},
				DefaultActorID: map[string]any{
					"id": DefaultActorID, "template_id": DefaultActorID,
					"state": map[string]any{xiuxianFieldRealm: realmKey},
				},
			},
		},
		Clock: &StoryClock{Version: StoryClockVersion, Seconds: seconds},
	}
}

// TestAJourneyStopsAtTheLifespanEdgeInsteadOfCrossingIt is §4's 在行动真正越界之前
// 自然提示处境 for the one activity that spends its time in a single lump.
//
// A rest and a training interval get a stop node inside the activity. A journey
// has nothing to pause, so without this cut a character with a quarter hour of
// life left would walk a half-hour road, cross the edge and have the crossing
// check declare 寿尽 for a walk to the market — an ordinary action turned into an
// ending by arithmetic the player never saw.
func TestAJourneyStopsAtTheLifespanEdgeInsteadOfCrossingIt(t *testing.T) {
	meta := lifespanTestMeta(119)
	boundary := lifespanBoundary(meta, journeyLifespanRealmState(), DefaultActorID)
	if boundary == nil {
		t.Fatal("119 years old at 炼气 must resolve a boundary")
	}
	if boundary.TotalYears != 120 {
		t.Fatalf("炼气 total = %d, want 120", boundary.TotalYears)
	}

	// A quarter of an hour short of the edge, with a half-hour road ahead.
	projection := journeyLifespanTestProjection(boundary.At-15*60, "越国坊市外围散修人家", qiStageKey(3))
	system := journeyTestSystem("越国坊市外围散修人家")

	plan, err := planJourneyStretch(projection, projection.State, "main", system, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
	}, "jnyrec-test", boundary)
	if err != nil {
		t.Fatalf("a cut stretch must still be settled, not refused: %v", err)
	}
	if !plan.StoppedAtLifespan {
		t.Fatal("the stretch crossed the edge without reporting the cut")
	}
	if plan.LifespanEdgeSeconds != boundary.At {
		t.Fatalf("LifespanEdgeSeconds = %d, want %d", plan.LifespanEdgeSeconds, boundary.At)
	}
	if plan.Advanced.AddedMinutes != 15 {
		t.Fatalf("the stretch must cover only the 15 minutes that remain: %d", plan.Advanced.AddedMinutes)
	}
	if plan.ClockAfter.Seconds != boundary.At {
		t.Fatalf("the cut must land exactly on the edge, which is alive: got %d, want %d",
			plan.ClockAfter.Seconds, boundary.At)
	}
	if plan.Advanced.Done {
		t.Fatal("a cut stretch has not reached the destination")
	}
	if plan.Advanced.Position.LocationID != "" {
		t.Fatalf("a cut stop is mid-segment and must name no place: %q", plan.Advanced.Position.LocationID)
	}
	// Standing on the edge is alive: the ending check must not fire for this clock.
	if ending := lifespanEnding(boundary, plan.ClockAfter.Seconds, meta.Calendar, "正文"); ending != nil {
		t.Fatalf("the cut must leave the character alive at the brink, got %+v", ending)
	}
}

// TestAJourneyWithLessThanAMinuteLeftIsRefusedForTheRightReason keeps the two
// refusals apart. A road the years have closed is not a road the player asked to
// stand still on, and reporting 无需再次结算 would describe the wrong cause.
func TestAJourneyWithLessThanAMinuteLeftIsRefusedForTheRightReason(t *testing.T) {
	meta := lifespanTestMeta(119)
	boundary := lifespanBoundary(meta, journeyLifespanRealmState(), DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}
	// Half a minute left: fewer than one whole minute of road.
	projection := journeyLifespanTestProjection(boundary.At-30, "越国坊市外围散修人家", qiStageKey(3))
	system := journeyTestSystem("越国坊市外围散修人家")

	_, err := planJourneyStretch(projection, projection.State, "main", system, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
	}, "jnyrec-test", boundary)
	if err == nil {
		t.Fatal("a road with no whole minute of life left must be refused")
	}
	if !errors.Is(err, ErrJourneyAtLifespanEdge) {
		t.Fatalf("err = %v, want ErrJourneyAtLifespanEdge", err)
	}
	if errors.Is(err, ErrJourneyNoProgress) {
		t.Fatalf("the refusal must not be reported as no progress: %v", err)
	}
}

// TestAJourneyShortOfTheEdgeIsUntouched keeps the cut from firing on ordinary
// travel: a road that finishes well before the edge commits exactly as before.
func TestAJourneyShortOfTheEdgeIsUntouched(t *testing.T) {
	meta := lifespanTestMeta(119)
	boundary := lifespanBoundary(meta, journeyLifespanRealmState(), DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}
	// A year of life left, a half-hour road: nothing to cut.
	projection := journeyLifespanTestProjection(0, "越国坊市外围散修人家", qiStageKey(3))
	system := journeyTestSystem("越国坊市外围散修人家")

	plan, err := planJourneyStretch(projection, projection.State, "main", system, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
	}, "jnyrec-test", boundary)
	if err != nil {
		t.Fatal(err)
	}
	if plan.StoppedAtLifespan || plan.LifespanEdgeSeconds != 0 {
		t.Fatalf("a road that ends long before the edge must not be cut: %#v", plan)
	}
	if plan.Advanced.AddedMinutes != 30 || !plan.Advanced.Done {
		t.Fatalf("the whole road must be walked: %#v", plan.Advanced)
	}
	// A story that cannot answer has no edge at all, and must behave the same.
	unbound, err := planJourneyStretch(projection, projection.State, "main", system, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
	}, "jnyrec-test-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if unbound.StoppedAtLifespan {
		t.Fatal("an unanswerable story must never cut a road")
	}
}

// TestALifespanCutSurvivesTheJournal is 保存恢复 for the cut: the reason the road
// stopped is on the record, so a rebuilt branch reports the same brink rather
// than a road that simply ended.
func TestALifespanCutSurvivesTheJournal(t *testing.T) {
	meta := lifespanTestMeta(119)
	boundary := lifespanBoundary(meta, journeyLifespanRealmState(), DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}
	projection := journeyLifespanTestProjection(boundary.At-15*60, "越国坊市外围散修人家", qiStageKey(3))
	system := journeyTestSystem("越国坊市外围散修人家")
	plan, err := planJourneyStretch(projection, projection.State, "main", system, JourneyRequest{
		RouteID: "route_036:reverse", Arrive: true,
	}, "jnyrec-test", boundary)
	if err != nil {
		t.Fatal(err)
	}

	event := journeyRecordEvent("main", "", "2026-01-01T00:00:00Z", JourneyRequest{
		BranchID: "main", RouteID: "route_036:reverse", Arrive: true,
	}, plan)
	if !event.StoppedAtLifespan || event.LifespanEdgeSeconds != boundary.At {
		t.Fatalf("the record must carry the cut: %#v", event)
	}
	if err := validateJourneyRecordEvent(event); err != nil {
		t.Fatalf("a well-formed cut must validate: %v", err)
	}
	replayed := &storyBranchProjection{}
	if err := applyJourneyRecord(replayed, event); err != nil {
		t.Fatalf("a cut record must replay: %v", err)
	}
	if replayed.Clock == nil || replayed.Clock.Seconds != boundary.At {
		t.Fatalf("the replay must restore the brink: %#v", replayed.Clock)
	}
	snapshot, ok := replayed.Journeys[plan.Advanced.Snapshot.JourneyID]
	if !ok || snapshot.ElapsedMinutes != 15 {
		t.Fatalf("the replay must restore the shorter road: %#v", replayed.Journeys)
	}

	// A record that claims a cut it cannot substantiate is refused rather than
	// silently believed: the next turn reads this flag to decide what to write.
	noEdge := event
	noEdge.LifespanEdgeSeconds = 0
	if err := validateJourneyRecordEvent(noEdge); err == nil {
		t.Fatal("a cut with no edge instant must be refused")
	}
	pastEdge := event
	pastEdge.LifespanEdgeSeconds = event.ClockAfter.Seconds - 1
	if err := validateJourneyRecordEvent(pastEdge); err == nil {
		t.Fatal("a cut whose clock has passed the edge must be refused")
	}
}
