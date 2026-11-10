package app

import (
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

func TestRadioContactRequiresOwnedDeviceAndEstablishedLiveChannel(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		device  int
		contact *character.RadioContact
		want    bool
	}{
		{"no contact", 1, nil, false},
		{"no device", 0, &character.RadioContact{DeviceItemID: "terminal", Channel: "confirmed", Status: "connected"}, false},
		{"offline", 1, &character.RadioContact{DeviceItemID: "terminal", Channel: "confirmed", Status: "offline"}, false},
		{"no channel", 1, &character.RadioContact{DeviceItemID: "terminal", Status: "connected"}, false},
		{"connected", 1, &character.RadioContact{DeviceItemID: "terminal", Channel: "confirmed", Status: "connected"}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackBinding: &interactive.StoryPackBinding{Gameplay: &interactive.StoryGameplayProfile{Communication: "radio"}}}, Snapshot: interactive.Snapshot{State: map[string]any{"actors": map[string]any{"protagonist": map[string]any{"state": map[string]any{"重要物品": map[string]any{"terminal": scenario.device}}}}}}}
			state := character.State{Profiles: []character.Profile{{ID: "remote", RadioContact: scenario.contact}}}
			if got := radioContactAvailable(ctx, state, "remote"); got != scenario.want {
				t.Fatalf("available=%v want=%v", got, scenario.want)
			}
		})
	}
}

// Establish and revise contact through the normal main-turn facts module, then
// rebuild the Store. This catches wiring bugs that a hand-built State cannot.
func TestRadioMainTurnFactsReconnectAndColdReplay(t *testing.T) {
	root := t.TempDir()
	module, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range module.ActorState.InitialActors {
		if module.ActorState.InitialActors[i].ID == interactive.DefaultActorID {
			module.ActorState.InitialActors[i].State = map[string]any{"重要物品": map[string]any{"terminal": 1}}
		}
	}
	store := interactive.NewStore(root)
	defer func() { _ = store.Close() }()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Radio lifecycle", ChoiceCount: 3, ActorState: &module.ActorState, PackBinding: &interactive.StoryPackBinding{PackID: "radio-test", Scope: "pack:radio-test", Gameplay: &interactive.StoryGameplayProfile{Communication: "radio"}}})
	if err != nil {
		t.Fatal(err)
	}
	available := func(want bool) {
		t.Helper()
		ctx, e := store.StoryContext(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		state, e := store.CharacterState(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		if got := radioContactAvailable(ctx, state, "remote"); got != want {
			t.Fatalf("available=%v want=%v", got, want)
		}
	}
	available(false)
	var checkpoint string
	for _, status := range []string{"connected", "offline", "connected"} {
		turn, _, e := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "确认联络频道", Narrative: "双方确认了终端频道的当前状态。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "等待"}, CharacterFacts: &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "remote", Name: "远方联系人", Description: "与你交换过终端频道的维修工。", RadioContact: &character.RadioContact{DeviceItemID: "terminal", Channel: "confirmed", Status: status}}}}}})
		if e != nil {
			t.Fatal(e)
		}
		available(status == "connected")
		if status == "offline" {
			checkpoint = turn.ID
		}
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: checkpoint, Title: "Offline branch"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	available(true)
	ctx, err := store.StoryContext(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if radioContactAvailable(ctx, state, "remote") {
		t.Fatal("reconnect leaked into offline branch")
	}

	// Ordinary item facts use the same turn journal in this non-adventure
	// communications fixture. Losing the device must revoke a saved channel.
	for _, quantity := range []int{0, 1} {
		_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "检查终端", Narrative: "你确认了当前携带的终端。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "等待"}, StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/protagonist/重要物品", Value: map[string]any{"terminal": quantity}}}}})
		if err != nil {
			t.Fatal(err)
		}
		available(quantity > 0)
	}
}
