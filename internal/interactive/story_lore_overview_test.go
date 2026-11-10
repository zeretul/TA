package interactive

import (
	"fmt"
	"testing"
)

// The old fan-out can pair a pre-journey snapshot with a post-journey route set
// on the very same branch. The aggregate must always choose one side of a commit.
func TestLoreOverviewConcurrentJourneyIsOneCommit(t *testing.T) {
	store, storyID := journeyTestStory(t, "越国坊市外围散修人家")
	writer := NewStore(store.root)
	defer writer.Close()
	old, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("writer panic: %v", r)
			}
		}()
		_, err := writer.ApplyJourney(storyID, JourneyRequest{BranchID: "main", RouteID: "route_036:reverse", Arrive: true, CommandID: "lore-walk", CommandHash: "lore-walk"})
		done <- err
	}()
	for i := 0; i < 8; i++ {
		view, err := store.ReadLoreOverview(storyID, "main")
		if err != nil {
			t.Fatal(err)
		}
		assertLoreOverviewCommit(t, view)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	final, err := store.ReadLoreOverview(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertLoreOverviewCommit(t, final)
	if storySceneText(old.State, storySceneLocationField) == final.JourneyOptions.CurrentLocation {
		t.Fatal("fixture did not reproduce same-branch movement")
	}
	if final.Clock.Seconds != 1800 {
		t.Fatalf("unexpected settled clock: %d", final.Clock.Seconds)
	}
}

func assertLoreOverviewCommit(t *testing.T, view LoreOverview) {
	t.Helper()
	location := storySceneText(view.Context.Snapshot.State, storySceneLocationField)
	if view.JourneyOptions.CurrentLocation != location || view.Quests.CurrentLocation != location || view.Events.Location != location {
		t.Fatalf("mixed locations: snapshot=%q journey=%q quests=%q events=%q", location, view.JourneyOptions.CurrentLocation, view.Quests.CurrentLocation, view.Events.Location)
	}
	if SnapshotClockSeconds(view.Context.Snapshot) != view.Clock.Seconds || view.Events.ClockSeconds != view.Clock.Seconds {
		t.Fatalf("mixed clocks: snapshot=%d clock=%d events=%d", SnapshotClockSeconds(view.Context.Snapshot), view.Clock.Seconds, view.Events.ClockSeconds)
	}
}
