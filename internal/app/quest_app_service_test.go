package app

import (
	"testing"

	"ta/internal/interactive"
	"ta/internal/quest"
)

// These tests cover the task entry's own contract at this layer: what the read
// reports, what a settled action names back to the panel, and that an empty
// branch and the current branch are the same command rather than a conflict.
//
// The store layer beneath already has its own suite for the rules, the journal
// record and the payment writes. What it cannot reach is what this layer decides
// — resolving the branch before fingerprinting the command, and reporting the
// task's own wording rather than an invented one.

// questAppTestApp builds a story with a head a task record can be parented to.
func questAppTestApp(t *testing.T) (*App, string) {
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
		system.InitialActors[index].State["在场角色"] = []any{"su-qian"}
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "事务", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "听人说起", Narrative: "主角在散修人家门前听人提起坊市里托付的一件小事。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "su-qian", Name: "苏浅", SourceKey: "npc_su_qian"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

// TestTheTaskEntryReadsWhatIsCommitted proves opening the entry is a plain read
// and that the offered tasks carry the authored wording.
func TestTheTaskEntryReadsWhatIsCommitted(t *testing.T) {
	application, storyID := questAppTestApp(t)

	state, err := application.QuestStateFor(storyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if state.BranchID != "main" {
		t.Fatalf("the read must resolve the branch: %q", state.BranchID)
	}
	if state.Location != "越国坊市外围散修人家" {
		t.Fatalf("the read must report the scene's own place: %q", state.Location)
	}
	if len(state.Tasks) != 0 {
		t.Fatalf("a story that has taken on nothing holds no tasks: %#v", state.Tasks)
	}
	if len(state.Available) != 1 || state.Available[0].TaskID != quest.TaskHerbHandover {
		t.Fatalf("only the public notice is known before the personal disclosure: %#v", state.Available)
	}
	for _, option := range state.Available {
		if option.Title == "" || option.Brief == "" {
			t.Fatalf("an offered task must carry its authored wording: %#v", option)
		}
	}
}

// TestAnOfferedTaskIsReadBackWithItsOwnWording proves an action reports the
// task's objectives rather than a generic acknowledgement.
func TestAnOfferedTaskIsReadBackWithItsOwnWording(t *testing.T) {
	application, storyID := questAppTestApp(t)

	result, err := application.ApplyQuest(QuestAppRequest{
		StoryID: storyID, TaskID: quest.TaskSuQianMaterials, Action: "offer",
		CommandID: "cmd-offer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BranchID != "main" {
		t.Fatalf("the action must resolve the branch: %q", result.BranchID)
	}
	if result.Status != string(quest.StatusOffered) {
		t.Fatalf("an offer must stay offered: %q", result.Status)
	}
	if len(result.Objectives) != 2 {
		t.Fatalf("the answer must carry both objectives: %#v", result.Objectives)
	}
	for _, objective := range result.Objectives {
		if objective.Completed {
			t.Fatalf("an offer must not tick anything: %#v", objective)
		}
		if objective.Description == "" {
			t.Fatalf("an objective must carry its authored wording: %#v", objective)
		}
	}
	if result.Settled || len(result.Rewards) != 0 {
		t.Fatalf("an offer must not pay: %#v", result)
	}
}

// TestAnEmptyBranchAndTheCurrentBranchAreTheSameCommand keeps a retry from being
// mistaken for a conflict after the branch is resolved.
func TestAnEmptyBranchAndTheCurrentBranchAreTheSameCommand(t *testing.T) {
	application, storyID := questAppTestApp(t)

	first, err := application.ApplyQuest(QuestAppRequest{
		StoryID: storyID, TaskID: quest.TaskHerbHandover, Action: "offer",
		CommandID: "cmd-offer",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.ApplyQuest(QuestAppRequest{
		StoryID: storyID, BranchID: "main", TaskID: quest.TaskHerbHandover, Action: "offer",
		CommandID: "cmd-offer",
	})
	if err != nil {
		t.Fatalf("the explicit current branch must be the same command: %v", err)
	}
	if !second.Replayed {
		t.Fatal("the repeated command must report itself as replayed")
	}
	if second.EventID != first.EventID {
		t.Fatalf("a replay must return the first attempt: %q vs %q", first.EventID, second.EventID)
	}
}

// TestAnActionWithoutACommandStillAppliesOnce proves the no-command path is not
// a way around the rules: without an identity the offer simply cannot be made
// twice, because the second one finds the task already offered.
func TestAnActionWithoutACommandStillAppliesOnce(t *testing.T) {
	application, storyID := questAppTestApp(t)
	if _, err := application.ApplyQuest(QuestAppRequest{
		StoryID: storyID, TaskID: quest.TaskHerbHandover, Action: "offer",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.ApplyQuest(QuestAppRequest{
		StoryID: storyID, TaskID: quest.TaskHerbHandover, Action: "offer",
	}); err == nil {
		t.Fatal("a second offer without a command must be refused, not applied twice")
	}
}
