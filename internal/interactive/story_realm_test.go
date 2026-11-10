package interactive

import (
	"errors"
	"testing"

	"ta/internal/exploration"
)

// These tests are §11 of 自制-探索与行路 as it happens during play: the player walks
// into an authored realm, moves through the rooms the instance really discovered,
// takes a declared reward, leaves, and comes back to the same realm.
//
// The rules layer already proves the instance arithmetic — exploration's own suite
// covers the move, exit, re-entry and grant boundaries. What these tests add is
// that the same facts are reachable from the story's one journal: the reward is
// written into the character's own pack in the same transaction, leaving restores
// the place the character walked in from, a retry grants nothing twice, and a
// re-entry continues the instance rather than generating a second copy.

const (
	realmTestOrigin  = "越国坊市外围散修人家"
	realmTestHall    = "realm.outer_hall"
	realmTestPlot    = "realm.herb_plot"
	realmTestAlchemy = "realm.alchemy_chamber"
	realmTestPassage = "realm.back_passage"
)

// realmTestStory builds a story whose scene places the protagonist outside the
// realm, so entering has a real place to record as the origin.
func realmTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := journeyTestSystem(realmTestOrigin)
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "秘境", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "进山", Narrative: "主角在山中寻到一处塌了半边的洞府入口。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// realmTestWays reads the doors the entry offers for the branch's only instance.
func realmTestWays(t *testing.T, store *Store, storyID string) []RealmWay {
	t.Helper()
	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 {
		t.Fatalf("expected one realm instance, got %#v", state.Instances)
	}
	return state.Instances[0].Ways
}

// realmTestApply settles one realm action and fails the test if the store refuses
// it.
func realmTestApply(t *testing.T, store *Store, storyID string, req RealmRequest) RealmResult {
	t.Helper()
	if req.BranchID == "" {
		req.BranchID = "main"
	}
	result, err := store.ApplyRealm(storyID, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// realmTestRecordCount counts the realm records on the branch's journal.
func realmTestRecordCount(t *testing.T, store *Store, storyID, branchID string) int {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, branchID)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeRealmRecord {
			count++
		}
	}
	return count
}

// realmTestHeldItems reads the protagonist's pack as the story committed it.
func realmTestHeldItems(t *testing.T, store *Store, storyID, branchID string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := xiuxianEquipmentEntry(snapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	return pack
}

// realmTestEnter walks the character in and returns the instance id.
func realmTestEnter(t *testing.T, store *Store, storyID string) RealmResult {
	t.Helper()
	result := realmTestApply(t, store, storyID, RealmRequest{
		RealmID: exploration.RealmGuXiuDongFu, Action: RealmActionEnter,
		CommandID: "cmd-enter", CommandHash: "h-enter",
	})
	if result.Instance.Presence != exploration.InstanceInside {
		t.Fatalf("entering must leave the character inside: %s", result.Instance.Presence)
	}
	if result.Instance.CurrentAreaID != realmTestHall {
		t.Fatalf("entering must stand in the declared entry area: %q", result.Instance.CurrentAreaID)
	}
	return result
}

// TestARealmRunCommitsRoomsRewardsAndTheWayOut is the batch's whole point: one
// ordinary realm run moves the character through the rooms the instance really
// discovered, writes the reward into the pack once, and restores the place the
// character came from when they leave.
func TestARealmRunCommitsRoomsRewardsAndTheWayOut(t *testing.T) {
	store, storyID := realmTestStory(t)

	entered := realmTestEnter(t, store, storyID)
	instanceID := entered.InstanceID

	// The entry area is known and the objectives are known; nothing deeper is.
	if len(entered.View.Areas) != 1 || entered.View.Areas[0].ID != realmTestHall {
		t.Fatalf("entering must know only the entry area: %#v", entered.View.Areas)
	}
	if len(entered.View.Objectives) != 2 {
		t.Fatalf("the brief's objectives must be known on the way in: %#v", entered.View.Objectives)
	}
	if len(entered.View.Connections) != 0 {
		t.Fatalf("no passage is known before it is walked: %#v", entered.View.Connections)
	}
	// But the doors out of the room are visible, and they are what a move names.
	// Without them the narrator has no connection_id and cannot move at all: the
	// first live run entered the hall and stayed there for every later turn.
	ways := realmTestWays(t, store, storyID)
	if len(ways) != 2 || ways[0].ConnectionID != "realm.hall_to_formation" || ways[1].ConnectionID != "realm.hall_to_plot" {
		t.Fatalf("the outer hall's two visible doors must be offered: %#v", ways)
	}
	if ways[1].DestinationAreaID != realmTestPlot || ways[1].DestinationName == "" {
		t.Fatalf("a door must name the far side: %#v", ways[1])
	}

	// A player names the room they walk to; the door is the visible one between.
	toPlot := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionMove,
		DestinationAreaID: realmTestPlot,
		CommandID:         "cmd-plot", CommandHash: "h-plot",
	})
	if toPlot.Instance.CurrentAreaID != realmTestPlot {
		t.Fatalf("the move must land in the herb plot: %q", toPlot.Instance.CurrentAreaID)
	}

	took := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionReward, ContentRefID: "realm.herb_ref",
		CommandID: "cmd-herb", CommandHash: "h-herb",
	})
	if took.Reward == nil || took.Reward.RefID != "pill.restore.health.lower" || took.Reward.Quantity != 1 {
		t.Fatalf("the declared pill must be the reward: %#v", took.Reward)
	}
	// The reward is a real item: the pack carries it after the same transaction.
	pack := realmTestHeldItems(t, store, storyID, "main")
	if _, held := pack["pill.restore.health.lower"]; !held {
		t.Fatalf("a granted reward must be in the character's pack: %#v", pack)
	}
	// Taking the thing the objective was tied to is what completes it.
	if len(took.View.CompletedObjectives) != 1 || took.View.CompletedObjectives[0] != "realm.herb" {
		t.Fatalf("taking the declared reward must complete its objective: %#v", took.View.CompletedObjectives)
	}

	// The main prize is two rooms further in, through the hall.
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestHall,
		CommandID: "cmd-hall", CommandHash: "h-hall",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_formation", DestinationAreaID: "realm.formation_room",
		CommandID: "cmd-formation", CommandHash: "h-formation",
	})
	toAlchemy := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionMove,
		ConnectionID: "realm.formation_to_alchemy", DestinationAreaID: realmTestAlchemy,
		CommandID: "cmd-alchemy", CommandHash: "h-alchemy",
	})
	if toAlchemy.Instance.CurrentAreaID != realmTestAlchemy {
		t.Fatalf("the formation room's spent seal must not block the alchemy chamber: %q", toAlchemy.Instance.CurrentAreaID)
	}
	prize := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionReward, ContentRefID: "realm.reward_ref",
		CommandID: "cmd-prize", CommandHash: "h-prize",
	})
	if prize.Reward == nil || prize.Reward.RefID != "implement.middle.rare" {
		t.Fatalf("the chamber must hand over the declared implement: %#v", prize.Reward)
	}
	pack = realmTestHeldItems(t, store, storyID, "main")
	if _, held := pack["implement.middle.rare"]; !held {
		t.Fatalf("the main prize must be in the character's pack: %#v", pack)
	}

	// Leaving is only legal from the declared exit, and it restores the place the
	// character walked in from rather than inventing one.
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionMove,
		ConnectionID: "realm.alchemy_to_passage", DestinationAreaID: realmTestPassage,
		CommandID: "cmd-passage", CommandHash: "h-passage",
	})
	left := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: instanceID, Action: RealmActionExit,
		CommandID: "cmd-exit", CommandHash: "h-exit",
	})
	if left.Instance.Presence != exploration.InstanceOutside {
		t.Fatalf("leaving must put the character outside: %s", left.Instance.Presence)
	}
	if got := journeyTestLocation(t, store, storyID, "main"); got != realmTestOrigin {
		t.Fatalf("leaving must restore the place the character entered from: %q", got)
	}

	// The whole run is on the journal, and the instance is the one that ran.
	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 {
		t.Fatalf("one realm run must leave one instance: %#v", state.Instances)
	}
	if state.Instances[0].InstanceID != instanceID || state.Instances[0].Presence != "outside" {
		t.Fatalf("the instance must read back as the one that ran: %#v", state.Instances[0])
	}
	if len(state.Instances[0].Granted) != 2 {
		t.Fatalf("both declared rewards must read back as taken: %#v", state.Instances[0].Granted)
	}
	if len(state.Instances[0].Rewards) != 0 {
		t.Fatalf("nothing is takeable from outside the realm: %#v", state.Instances[0].Rewards)
	}
}

// TestARealmRewardIsGrantedOnce is §11.4's "奖励幂等".
//
// The attempt may have committed before the response was lost, so a retry has to
// address the same grant. It must report the committed answer and add nothing
// rather than handing over a second copy.
func TestARealmRewardIsGrantedOnce(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-plot", CommandHash: "h-plot",
	})

	request := RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionReward, ContentRefID: "realm.herb_ref",
		CommandID: "cmd-herb", CommandHash: "h-herb",
	}
	first := realmTestApply(t, store, storyID, request)
	retry := realmTestApply(t, store, storyID, request)

	if !retry.Replayed {
		t.Fatal("a retry under the same command must report itself as already on record")
	}
	if retry.EventID != first.EventID || retry.InstanceID != first.InstanceID {
		t.Fatalf("a retry must report the first attempt: %#v vs %#v", retry, first)
	}
	// The replay still names the reward and still shows the room, so a repeated
	// click reads as the same fact rather than an empty realm.
	if retry.Reward == nil || retry.Reward.RefID != first.Reward.RefID {
		t.Fatalf("a replay must report the reward it already granted: %#v", retry.Reward)
	}
	if retry.View.CurrentArea == nil || retry.View.CurrentArea.ID != realmTestPlot {
		t.Fatalf("a replay must show the room the first attempt reported: %#v", retry.View.CurrentArea)
	}
	if got := len(retry.Instance.GrantedRewards); got != 1 {
		t.Fatalf("a retry must not grant a second copy: %d", got)
	}
}

// TestATakenRewardCannotBeTakenAgain refuses the same declared reward under a new
// command, which is the case the caller's own idempotency cannot cover.
func TestATakenRewardCannotBeTakenAgain(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-plot", CommandHash: "h-plot",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionReward, ContentRefID: "realm.herb_ref",
		CommandID: "cmd-herb", CommandHash: "h-herb",
	})

	_, err := store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", InstanceID: entered.InstanceID, Action: RealmActionReward,
		ContentRefID: "realm.herb_ref",
		CommandID:    "cmd-herb-again", CommandHash: "h-herb-again",
	})
	if !errors.Is(err, exploration.ErrInstanceConflict) {
		t.Fatalf("a second take of the same declared reward must be refused: %v", err)
	}
	pack := realmTestHeldItems(t, store, storyID, "main")
	if entry, held := pack["pill.restore.health.lower"].(map[string]any); held {
		if quantity := xiuxianCarriedQuantity(entry); quantity != 1 {
			t.Fatalf("a refused take must not add to the count: %d", quantity)
		}
	}
}

// TestAReentryContinuesTheSameRealm is §11.4: the realm is not regenerated, so a
// second visit finds the rooms already walked and the rewards already gone.
func TestAReentryContinuesTheSameRealm(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-plot", CommandHash: "h-plot",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionReward, ContentRefID: "realm.herb_ref",
		CommandID: "cmd-herb", CommandHash: "h-herb",
	})
	// Back out through the hall, then leave from the entry area, which is a
	// declared area but not a declared exit, so leaving is refused there.
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestHall,
		CommandID: "cmd-back", CommandHash: "h-back",
	})
	if _, err := store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", InstanceID: entered.InstanceID, Action: RealmActionExit,
		CommandID: "cmd-exit-early", CommandHash: "h-exit-early",
	}); !errors.Is(err, exploration.ErrInvalidInstanceAction) {
		t.Fatalf("leaving from a non-exit area must be refused: %v", err)
	}

	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-plot-again", CommandHash: "h-plot-again",
	})
	// Walking to the exit through the formation room, the same way a player would.
	for _, step := range []struct{ connection, destination, command string }{
		{"realm.hall_to_plot", realmTestHall, "cmd-return-hall"},
		{"realm.hall_to_formation", "realm.formation_room", "cmd-return-formation"},
		{"realm.formation_to_alchemy", realmTestAlchemy, "cmd-return-alchemy"},
		{"realm.alchemy_to_passage", realmTestPassage, "cmd-return-passage"},
	} {
		realmTestApply(t, store, storyID, RealmRequest{
			InstanceID: entered.InstanceID, Action: RealmActionMove,
			ConnectionID: step.connection, DestinationAreaID: step.destination,
			CommandID: step.command, CommandHash: "h-" + step.command,
		})
	}
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionExit,
		CommandID: "cmd-exit", CommandHash: "h-exit",
	})

	// Re-entry names the same instance and continues it.
	reentered := realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionReenter,
		CommandID: "cmd-reenter", CommandHash: "h-reenter",
	})
	if reentered.InstanceID != entered.InstanceID {
		t.Fatalf("a return must continue the same instance: %q vs %q", reentered.InstanceID, entered.InstanceID)
	}
	if reentered.Instance.Presence != exploration.InstanceInside {
		t.Fatalf("a return must put the character back inside: %s", reentered.Instance.Presence)
	}
	if reentered.Instance.CurrentAreaID != realmTestHall {
		t.Fatalf("a return starts at the declared entry, not the deep position: %q", reentered.Instance.CurrentAreaID)
	}
	// The history is kept: the herb plot is still known and its pill is still gone.
	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 {
		t.Fatalf("a return must not create a second instance: %#v", state.Instances)
	}
	if len(state.Instances[0].Granted) != 1 {
		t.Fatalf("the taken reward must still be recorded as taken: %#v", state.Instances[0].Granted)
	}
	// The herb plot's only declared reward is gone, so it offers nothing.
	for _, area := range state.Instances[0].View.Areas {
		if area.ID != realmTestPlot {
			continue
		}
		// Walking there again would be refused by the rules, but the read can be
		// checked without moving: the instance's granted list is what decides.
		if len(state.Instances[0].Granted) != 1 || state.Instances[0].Granted[0].ContentRefID != "realm.herb_ref" {
			t.Fatalf("the herb plot's pill must still read as taken: %#v", state.Instances[0].Granted)
		}
	}
}

// TestEnteringAgainIsAReturnNotASecondRealm keeps the panel's single "enter" button
// honest: a branch that already holds the realm continues it.
func TestEnteringAgainIsAReturnNotASecondRealm(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_formation", DestinationAreaID: "realm.formation_room",
		CommandID: "cmd-formation", CommandHash: "h-formation",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.formation_to_alchemy", DestinationAreaID: realmTestAlchemy,
		CommandID: "cmd-alchemy", CommandHash: "h-alchemy",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.alchemy_to_passage", DestinationAreaID: realmTestPassage,
		CommandID: "cmd-passage", CommandHash: "h-passage",
	})
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionExit,
		CommandID: "cmd-exit", CommandHash: "h-exit",
	})

	again := realmTestApply(t, store, storyID, RealmRequest{
		RealmID: exploration.RealmGuXiuDongFu, Action: RealmActionEnter,
		CommandID: "cmd-enter-again", CommandHash: "h-enter-again",
	})
	if again.Action != RealmActionReenter {
		t.Fatalf("entering a realm the branch already holds must be a return: %q", again.Action)
	}
	if again.InstanceID != entered.InstanceID {
		t.Fatalf("a return must not generate a second instance: %q vs %q", again.InstanceID, entered.InstanceID)
	}
	if count := realmTestRecordCount(t, store, storyID, "main"); count != 6 {
		// enter, formation, alchemy, passage, exit, reenter
		t.Fatalf("each settled action must be one record: %d", count)
	}
}

// TestARealmActionRefusesAPlaceTheCharacterIsNotIn refuses a move that names a
// connection the instance does not have from where the character stands.
func TestARealmActionRefusesAPlaceTheCharacterIsNotIn(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)

	_, err := store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.formation_to_alchemy", DestinationAreaID: realmTestAlchemy,
		CommandID: "cmd-skip", CommandHash: "h-skip",
	})
	if !errors.Is(err, exploration.ErrInvalidInstanceAction) {
		t.Fatalf("a move through a passage that does not start here must be refused: %v", err)
	}
	if count := realmTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("a refused action must commit nothing: %d", count)
	}

	// A reward is taken from the room it is declared in, not from the hall.
	_, err = store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", InstanceID: entered.InstanceID, Action: RealmActionReward,
		ContentRefID: "realm.herb_ref",
		CommandID:    "cmd-herb-far", CommandHash: "h-herb-far",
	})
	if !errors.Is(err, exploration.ErrInvalidInstanceAction) {
		t.Fatalf("a reward must not be taken from another room: %v", err)
	}
}

// TestOneCommandIDCannotCarryTwoRealmActions refuses to answer a different action
// under an identity that already means something else.
func TestOneCommandIDCannotCarryTwoRealmActions(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)

	_, err := store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-enter", CommandHash: "h-different",
	})
	if !errors.Is(err, ErrRealmCommandConflict) {
		t.Fatalf("reusing one command id for different content must be refused: %v", err)
	}
}

// TestARealmPanelReadCostsNoAction proves opening the entry changes nothing.
func TestARealmPanelReadCostsNoAction(t *testing.T) {
	store, storyID := realmTestStory(t)

	before, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Instances) != 0 {
		t.Fatalf("a story that has not entered a realm holds none: %#v", before.Instances)
	}
	if len(before.Available) != 2 || before.Available[0].RealmID != exploration.RealmGuXiuDongFu || before.Available[1].RealmID != exploration.RealmMountainHerbCave {
		t.Fatalf("the authored realm must be offered: %#v", before.Available)
	}
	if before.Available[0].Name != "外殿" {
		t.Fatalf("the offer must use the definition's own wording: %q", before.Available[0].Name)
	}
	if count := realmTestRecordCount(t, store, storyID, "main"); count != 0 {
		t.Fatalf("reading must commit nothing: %d", count)
	}

	entered := realmTestEnter(t, store, storyID)
	inside, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(inside.Instances) != 1 || inside.Instances[0].InstanceID != entered.InstanceID {
		t.Fatalf("the entered realm must read back: %#v", inside.Instances)
	}
	// Nothing is takeable from the entry hall, because no reward is declared there.
	if len(inside.Instances[0].Rewards) != 0 {
		t.Fatalf("the entry hall declares no reward: %#v", inside.Instances[0].Rewards)
	}
}

// TestABranchForkedInsideARealmCanStillAct is the live defect this fix was
// written for.
//
// A branch is the unit that can differ, so a fork inherits the realm the
// character was standing in. The instance snapshot is the one checkpoint that
// carries its trusted scope inside the fact: the binding names the branch. The
// fork used to copy the parent's snapshot unchanged, so every action on the
// fork — a move, a reward, the way out — was refused with "instance belongs to
// a different trusted branch scope", and the prose could not settle anything
// the fork really did. The run reproduced it on a branch forked from a turn
// taken inside the herb plot.
func TestABranchForkedInsideARealmCanStillAct(t *testing.T) {
	store, storyID := realmTestStory(t)
	entered := realmTestEnter(t, store, storyID)
	realmTestApply(t, store, storyID, RealmRequest{
		InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestPlot,
		CommandID: "cmd-plot", CommandHash: "h-plot",
	})
	parentTurn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在药圃里翻找", Narrative: "主角在药圃的石架前蹲下身。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: parentTurn.ID, Title: "从药圃重来"})
	if err != nil {
		t.Fatal(err)
	}

	// The fork sees the same realm and the same room.
	state, err := store.RealmStateFor(storyID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 || state.Instances[0].Presence != "inside" {
		t.Fatalf("a fork must inherit the realm the character stood in: %#v", state.Instances)
	}
	if state.Instances[0].View.CurrentArea == nil || state.Instances[0].View.CurrentArea.ID != realmTestPlot {
		t.Fatalf("a fork must inherit the room the character stood in: %#v", state.Instances[0].View.CurrentArea)
	}

	// And it can act: the binding belongs to the fork now.
	back := realmTestApply(t, store, storyID, RealmRequest{
		BranchID: branch.ID, InstanceID: entered.InstanceID, Action: RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: realmTestHall,
		CommandID: "cmd-back", CommandHash: "h-back",
	})
	if back.Instance.CurrentAreaID != realmTestHall {
		t.Fatalf("the fork's move must land in the hall: %q", back.Instance.CurrentAreaID)
	}

	// The parent's copy is untouched: a fork is its own life, not a shared one.
	parentState, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if parentState.Instances[0].View.CurrentArea == nil || parentState.Instances[0].View.CurrentArea.ID != realmTestPlot {
		t.Fatalf("the parent's realm must not move with the fork: %#v", parentState.Instances[0].View.CurrentArea)
	}
}
