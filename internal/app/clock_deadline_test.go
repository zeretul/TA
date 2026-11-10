package app

import (
	"strings"
	"testing"

	"ta/config"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/interactive"
	"ta/internal/quest"
)

// This file covers F19's deadline half as the player actually meets it: an
// activity that really spends formal time crosses an agreed limit, and the next
// turn is told the agreement ran out rather than being handed a day that looks
// ordinary.
//
// The rules layer already proves the deadline arithmetic and the expiry record
// (internal/interactive/story_interval_test.go). What was missing is the join to
// the turn's own context, and the two branch properties that decide whether the
// player sees the same world after a restart or a fork. 约定已到期 is the sentence
// the narrator is given; without it a crossed limit is a fact in the journal and
// a silence in the prose.
//
// 自制-时间寿元与长时世界推进 §五 asks the rules to settle every registered
// matter an interval crossed, whether or not the player was watching, and the
// acceptance adds 期限后果保存恢复. A task is the one matter with a real instant
// today: acceptance stamps DeadlineSeconds from the committed clock, so this is
// the half that can be closed without a calendar.

// acceptSuQianMaterials takes the two-day task through the store the game uses.
func acceptSuQianMaterials(t *testing.T, fixture sectDuty) {
	t.Helper()
	context, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	present := []any{}
	for _, id := range interactive.StoryScene(context.Snapshot).Present {
		present = append(present, id)
	}
	present = append(present, "su-qian")
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "听苏浅说起药材", Narrative: "苏浅当面说明待核对的药材，并约定两日内给出答复。",
		ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色", Value: present}},
		TurnResult: &interactive.TurnResult{Choices: []string{"应下差事", "问清细节", "暂时谢绝"}, CharacterFacts: &interactive.TurnCharacterFactsInput{
			Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "su-qian", Name: "苏浅", SourceKey: "npc_su_qian"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyQuest(fixture.story.ID, interactive.QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: interactive.QuestActionOffer}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyQuest(fixture.story.ID, interactive.QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: interactive.QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	}); err != nil {
		t.Fatalf("accept the task: %v", err)
	}
}

// TestALongActivityClosesAnAgreedDeadlineAndTheTurnSaysSo is the end-to-end
// path: the task allows two days from acceptance, the seclusion spends three,
// and the turn the player then takes is told the agreed moment passed.
func TestALongActivityClosesAnAgreedDeadlineAndTheTurnSaysSo(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)
	secludeFor(t, fixture, 3*24*60*60)

	action := "我出关了。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	if !strings.Contains(all, "约定已到期") {
		t.Fatalf("a seclusion that outlasted the agreed limit left the turn reading as an ordinary day:\n%s", all)
	}
	if !strings.Contains(all, quest.TaskSuQianMaterials) {
		t.Fatalf("the turn does not name the task whose limit ran out:\n%s", all)
	}

	// The expiry is a committed fact, not a reading the panel computed.
	state, err := fixture.store.QuestStateFor(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the quest state: %v", err)
	}
	var status string
	for _, task := range state.Tasks {
		if task.TaskID == quest.TaskSuQianMaterials {
			status = task.Status
		}
	}
	if status != string(quest.StatusExpired) {
		t.Fatalf("the task is %q, want the interval to have expired it", status)
	}
}

// TestAnActivityThatEndsBeforeTheDeadlineLeavesTheTurnAlone is the A/B pair the
// design asks a batch to demonstrate: the same action, one day short of the
// limit, must not close anything or teach the narrator an expiry that did not
// happen.
func TestAnActivityThatEndsBeforeTheDeadlineLeavesTheTurnAlone(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)
	secludeFor(t, fixture, 24*60*60)

	action := "我出关了。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	if strings.Contains(all, "约定已到期") {
		t.Fatalf("a seclusion inside the limit was reported as having missed it:\n%s", all)
	}
	if !strings.Contains(all, "Agreed deadline:") {
		t.Fatalf("the turn lost the limit the player is still working to:\n%s", all)
	}
}

// TestTheExpiredDeadlineSurvivesAReopen is 保存恢复 for this half: the expiry is
// on the journal, so a reopened store tells the next turn the same thing instead
// of offering the player a limit that already passed.
func TestTheExpiredDeadlineSurvivesAReopen(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)
	secludeFor(t, fixture, 3*24*60*60)

	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	action := "我出关了。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	if !strings.Contains(dutyMessageText(assembled), "约定已到期") {
		t.Fatal("reopening forgot the expiry the interval committed")
	}
}

// TestAForkBeforeTheSeclusionKeepsItsOwnDeadline keeps time branch-scoped, the
// same rule the clock follows: a branch cut before the seclusion never spent
// those days, so its task is still inside its limit.
func TestAForkBeforeTheSeclusionKeepsItsOwnDeadline(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)

	// One ordinary turn after acceptance gives the fork a point that already has
	// the task, so the two branches differ in the interval rather than in whether
	// the task exists at all.
	forkPoint, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我先回居所收拾东西。",
		Narrative: "你把明日的用具收拾好，天已经黑了。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"睡下", "再看看册子", "去问问药材的事"},
		},
	})
	if err != nil {
		t.Fatalf("commit the turn the fork starts from: %v", err)
	}
	secludeFor(t, fixture, 3*24*60*60)

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: forkPoint.ID,
		Title:         "不闭关的那一支",
	})
	if err != nil {
		t.Fatalf("fork before the seclusion: %v", err)
	}
	conversation := interactiveapp.NewConversation(
		fixture.store, fixture.dataRoot, fixture.workspace, fixture.story.ID, branch.ID,
		"我接着去问药材的事。", fixture.story.ReplyTargetChars,
		&config.Config{DenovaDir: fixture.dataRoot, Workspace: fixture.workspace},
	)
	action := "我接着去问药材的事。"
	assembled, _ := assembleDutyTurn(t, conversation, action)
	if strings.Contains(dutyMessageText(assembled), "约定已到期") {
		t.Fatal("a branch that never spent the seclusion reported the limit as missed")
	}
	state, err := fixture.store.QuestStateFor(fixture.story.ID, branch.ID)
	if err != nil {
		t.Fatalf("read the fork's quest state: %v", err)
	}
	for _, task := range state.Tasks {
		if task.TaskID == quest.TaskSuQianMaterials && task.Status != string(quest.StatusAccepted) {
			t.Fatalf("the fork's task is %q, want it still inside its limit", task.Status)
		}
	}
}
