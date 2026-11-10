package app

import (
	"fmt"
	"strings"
	"testing"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file closes 第20项's remaining gap. The faction batch already proves that
// a committed change reaches the next turn, survives a reopen and stays on its
// own branch; the message batch already proves a letter's hour can pass. What
// neither joins is the pair the design actually asks about — 自制-长期势力演变
// FAC-05 and FAC-06:
//
//   - 事态已发生、玩家是否获知、各NPC分别知道多少分开. A change the player saw is
//     the faction's state; a change they were only told about is not, and a
//     stale note is neither.
//   - 重启及分支读取同一事件结果. The same record, read again, says the same thing.
//
// So this is one chain: the player stands in the sect and sees one change; a
// rumour they cannot verify leaves it alone; a decade passes; the same record is
// still there and is now dated as old rather than restated as the world's
// present; a letter that named an hour has run out and is reported as spent
// rather than as a live offer; and a reopened store and a fork read all of it
// the same way.

const (
	// What the player saw happen, in the turn that commits it.
	f20SeenChange = "本月灵石份例迟发"
	// What they were only told about, by someone who heard it elsewhere.
	f20Rumour = "山下有人说黄枫谷要闭山三年"
)

// f20RecordFaction commits one faction record the way the model does: a whole
// object under the field the handbook names, on the world entities Actor.
func f20RecordFaction(t *testing.T, fixture sectDuty, action, narrative, currentAction string) interactive.TurnEvent {
	t.Helper()
	turn, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      action,
		Narrative: narrative,
		TurnResult: &interactive.TurnResult{
			Choices: []string{"再等等", "去问别人", "先回去打坐"},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/world/势力记录",
				Value: map[string]any{"黄枫谷": map[string]any{
					"类型":   "宗门",
					"存续状态": "存续",
					"对主角立场": "门下外门弟子",
					"当前行动": currentAction,
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("commit the faction record: %v", err)
	}
	return turn
}

// f20FactionState reads the record back off the journal rather than off the
// prose, so the assertions are about what was saved.
func f20FactionState(t *testing.T, fixture sectDuty, branchID string) (action, writtenAt string) {
	t.Helper()
	snapshot, err := fixture.store.Snapshot(fixture.story.ID, branchID)
	if err != nil {
		t.Fatalf("read the branch snapshot: %v", err)
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	world, _ := actors[interactive.DefaultWorldEntitiesActorID].(map[string]any)
	if world == nil {
		t.Fatalf("the branch has no world entities Actor: %v", snapshot.State["actors"])
	}
	state, _ := world["state"].(map[string]any)
	records, _ := state["势力记录"].(map[string]any)
	faction, _ := records["黄枫谷"].(map[string]any)
	return strings.TrimSpace(fmt.Sprint(faction["当前行动"])), strings.TrimSpace(fmt.Sprint(world["written_at_event"]))
}

// f20Window reads the character window the way the server does, so the message
// assertions are about the same projection the player sees.
func f20Window(t *testing.T, fixture sectDuty) CharacterWindow {
	t.Helper()
	window, err := characterWindowApp(fixture).CharacterWindow(CharacterWindowRequest{
		StoryID: fixture.story.ID, BranchID: "main", CharacterID: "mu_peiling",
	})
	if err != nil {
		t.Fatalf("read the character window: %v", err)
	}
	return window
}

// TestAFactionChangeIsSavedDatedAndNotRestatedAsCurrent is 第20项's chain.
func TestAFactionChangeIsSavedDatedAndNotRestatedAsCurrent(t *testing.T) {
	fixture := startSectDutyStory(t)
	dutyTurn := fixture.takeUpTheDuty(t)

	// --- 变化有来源：the player stood there and saw it -------------------------
	f20RecordFaction(t, fixture,
		"我去问份例的事。",
		"执事翻了翻册子，说这个月的灵石还没到，让你过几日再来。",
		f20SeenChange)

	action, writtenAt := f20FactionState(t, fixture, "main")
	if action != f20SeenChange {
		t.Fatalf("the change was not saved: 当前行动 = %q, want %q", action, f20SeenChange)
	}
	// The record says which turn wrote it, which is what lets a later turn tell
	// it from a note written this morning.
	if writtenAt == "" || writtenAt == "<nil>" {
		t.Fatal("the record does not say which turn wrote it")
	}

	// --- 传闻不成为当前事实 ---------------------------------------------------
	//
	// The turn says someone repeated a rumour. Nothing in it is something the
	// player saw, so the record must keep the state they did see. This is the
	// half FAC-05 asks for: 事态已发生 and 玩家是否获知 are different facts, and a
	// note that cannot tell them apart turns every retold line into the world.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "路上听见有人议论黄枫谷。",
		Narrative: "回药园的路上，一个担水的杂役压低声音说，" + f20Rumour + "。你不知真假。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"去问慕管事", "先不理会", "自己下山看看"},
		},
	}); err != nil {
		t.Fatalf("commit the turn that carries only a rumour: %v", err)
	}
	if got, _ := f20FactionState(t, fixture, "main"); got != f20SeenChange {
		t.Fatalf("a rumour rewrote the faction's state: 当前行动 = %q, want the change the player saw %q", got, f20SeenChange)
	}

	// --- 一条会过期的来讯 -----------------------------------------------------
	//
	// 慕沛灵 writes with an hour attached. It is live now and must be spent once
	// that hour passes, without losing its words.
	fixture.sendProactiveMessage(t, proactiveMessageIdentity())
	if live := proactiveWindowMessage(t, f20Window(t, fixture), "mu_peiling"); live.Lapsed {
		t.Fatalf("a fresh letter is already reported as spent: %+v", live)
	}

	// --- 经过时间：a decade passes and the record is now old news --------------
	secludeFor(t, fixture, 10*aRuleYear)

	afterText := readTurn(t, fixture.conversation("我出关了。"), "我出关了。")
	// The record is still the record — nothing silently rewrote it.
	if got, _ := f20FactionState(t, fixture, "main"); got != f20SeenChange {
		t.Fatalf("the seclusion changed the saved record: %q", got)
	}
	// And the turn is told how old it is, which is the only thing that stops a
	// decade-old 当前行动 from being written as the world's present.
	for _, want := range []string{"- Written: ", "此后又过了", "不一定是现在的样子"} {
		if !strings.Contains(afterText, want) {
			t.Fatalf("the turn reading the old record is missing %q:\n%s", want, afterText)
		}
	}

	// --- 过期消息不再当作当前事实 ---------------------------------------------
	spent := proactiveWindowMessage(t, f20Window(t, fixture), "mu_peiling")
	if !spent.Lapsed {
		t.Fatalf("a decade passed and the letter is still offered as current: %+v", spent)
	}
	if spent.Text != proactiveMessageText || spent.Deadline != proactiveMessageDeadline {
		t.Fatalf("a spent letter lost its words or its hour: %+v", spent)
	}

	// --- 保存：the same record reads the same after a reopen -------------------
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	if got, _ := f20FactionState(t, fixture, "main"); got != f20SeenChange {
		t.Fatalf("reopening changed the record: %q", got)
	}
	reopenedText := readTurn(t, fixture.conversation("我回来了。"), "我回来了。")
	for _, want := range []string{"- Written: ", "此后又过了"} {
		if !strings.Contains(reopenedText, want) {
			t.Fatalf("reopening lost how old the record is (%q):\n%s", want, reopenedText)
		}
	}
	if again := proactiveWindowMessage(t, f20Window(t, fixture), "mu_peiling"); !again.Lapsed {
		t.Fatal("reopening made the spent letter live again")
	}

	// --- 分支：a fork cut before the change has the older state ---------------
	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: dutyTurn.ID,
		Title:         "没听到份例消息的那一支",
	})
	if err != nil {
		t.Fatalf("fork before the change: %v", err)
	}
	if got, _ := f20FactionState(t, fixture, branch.ID); got == f20SeenChange {
		t.Fatalf("a branch cut before the change inherited it: %q", got)
	}
}
