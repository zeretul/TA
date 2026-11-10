package app

import (
	"testing"

	"ta/internal/exploration"
	"ta/internal/interactive"
)

// These tests cover the realm entry's own contract at this layer: what the read
// reports, what a settled action names back to the panel, and that an empty
// branch and the current branch are the same command rather than a conflict.
//
// The store layer beneath already has its own suite for the rules, the journal
// record and the item write. What it cannot reach is what this layer decides —
// resolving the branch before fingerprinting the command, and turning a granted
// item into the catalogue's own wording rather than an invented name.

// realmAppTestApp builds a story whose scene places the protagonist outside the
// realm, so entering records a real origin.
func realmAppTestApp(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	system := combatTestSystem(t)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != interactive.DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		system.InitialActors[index].State["当前详细地点"] = "越国坊市外围散修人家"
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "秘境", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "进山", Narrative: "主角在山中寻到一处塌了半边的洞府入口。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

// TestTheRealmEntryReadsWhatIsCommitted proves opening the entry is a plain read
// and that the offered realm carries the definition's own wording.
func TestTheRealmEntryReadsWhatIsCommitted(t *testing.T) {
	application, storyID := realmAppTestApp(t)

	state, err := application.RealmStateFor(storyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if state.BranchID != "main" {
		t.Fatalf("the read must resolve the branch: %q", state.BranchID)
	}
	if state.Location != "越国坊市外围散修人家" {
		t.Fatalf("the read must report the scene's own place: %q", state.Location)
	}
	if len(state.Instances) != 0 {
		t.Fatalf("a story that has not entered a realm holds none: %#v", state.Instances)
	}
	var option interactive.RealmOption
	for _, available := range state.Available {
		if available.RealmID == exploration.RealmGuXiuDongFu {
			option = available
			break
		}
	}
	if option.RealmID != exploration.RealmGuXiuDongFu || option.Name != "外殿" {
		t.Fatalf("the offer must use the definition's own wording: %#v", option)
	}
	if option.EntryAreaID != "realm.outer_hall" {
		t.Fatalf("the offer must name the declared entry: %q", option.EntryAreaID)
	}
}

// TestASettledRealmActionNamesItsRewardInTheCatalogueWording is the panel's
// contract: it reports the item by the catalogue's localisation key, never by an
// invented name, so the interface can render it.
func TestASettledRealmActionNamesItsRewardInTheCatalogueWording(t *testing.T) {
	application, storyID := realmAppTestApp(t)

	entered, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, RealmID: exploration.RealmGuXiuDongFu, Action: interactive.RealmActionEnter,
		CommandID: "cmd-enter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entered.Presence != "inside" || entered.InstanceID == "" {
		t.Fatalf("entering must report being inside a named instance: %#v", entered)
	}

	if _, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, InstanceID: entered.InstanceID, Action: interactive.RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot",
		CommandID: "cmd-plot",
	}); err != nil {
		t.Fatal(err)
	}

	took, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, InstanceID: entered.InstanceID, Action: interactive.RealmActionReward,
		ContentRefID: "realm.herb_ref",
		CommandID:    "cmd-herb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if took.Reward == nil {
		t.Fatal("taking a declared reward must report the grant")
	}
	if took.Reward.RefID != "pill.restore.health.lower" {
		t.Fatalf("the grant must name the catalogue entry: %q", took.Reward.RefID)
	}
	if took.Reward.NameKey != "item.pill.restore_health_lower.name" {
		t.Fatalf("the grant must carry the catalogue's localisation key: %q", took.Reward.NameKey)
	}
	if took.Reward.Quantity != 1 {
		t.Fatalf("the grant must carry the declared quantity: %d", took.Reward.Quantity)
	}
}

// TestAnEmptyBranchAndTheCurrentBranchAreOneRealmCommand keeps a retry from
// reading as a conflict merely because the first attempt named the branch and the
// retry left it to the current one.
func TestAnEmptyBranchAndTheCurrentBranchAreOneRealmCommand(t *testing.T) {
	application, storyID := realmAppTestApp(t)

	first, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, BranchID: "main", RealmID: exploration.RealmGuXiuDongFu,
		Action: interactive.RealmActionEnter, CommandID: "cmd-enter",
	})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, BranchID: "", RealmID: exploration.RealmGuXiuDongFu,
		Action: interactive.RealmActionEnter, CommandID: "cmd-enter",
	})
	if err != nil {
		t.Fatalf("a retry that leaves the branch to the current one must not conflict: %v", err)
	}
	if !retry.Replayed {
		t.Fatal("the retry must report itself as already on record")
	}
	if retry.EventID != first.EventID || retry.InstanceID != first.InstanceID {
		t.Fatalf("the retry must report the first attempt: %#v vs %#v", retry, first)
	}
}

// TestAReplayedRewardStillNamesTheItem keeps a repeated click from reading as an
// empty grant: the answer carries the same item the first attempt committed.
func TestAReplayedRewardStillNamesTheItem(t *testing.T) {
	application, storyID := realmAppTestApp(t)

	entered, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, RealmID: exploration.RealmGuXiuDongFu, Action: interactive.RealmActionEnter,
		CommandID: "cmd-enter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.ApplyRealm(RealmAppRequest{
		StoryID: storyID, InstanceID: entered.InstanceID, Action: interactive.RealmActionMove,
		ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot",
		CommandID: "cmd-plot",
	}); err != nil {
		t.Fatal(err)
	}
	request := RealmAppRequest{
		StoryID: storyID, InstanceID: entered.InstanceID, Action: interactive.RealmActionReward,
		ContentRefID: "realm.herb_ref", CommandID: "cmd-herb",
	}
	first, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Replayed {
		t.Fatal("the retry must report itself as already on record")
	}
	if retry.Reward == nil || retry.Reward.RefID != first.Reward.RefID {
		t.Fatalf("a replay must name the reward it already granted: %#v", retry.Reward)
	}
	if retry.Reward.NameKey != "item.pill.restore_health_lower.name" {
		t.Fatalf("a replay must carry the same catalogue wording: %q", retry.Reward.NameKey)
	}
	if retry.View.CurrentArea == nil || retry.View.CurrentArea.ID != "realm.herb_plot" {
		t.Fatalf("a replay must show the room the first attempt reported: %#v", retry.View.CurrentArea)
	}
}
