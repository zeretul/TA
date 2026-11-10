package app

import (
	"strings"
	"testing"

	"ta/config"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// This file covers F18's remaining gap: an arrangement made inside a real
// organization has to come due in real game time and produce a consequence in
// the story, rather than quietly disappearing from the narrator's view.
//
// The design (自制-家族与宗门经营 §2, §4, §5) asks for one real organization to
// carry 商议 → 权限内安排一件事 → 经过必要时间 → 收到结果 → 继续故事, and is
// explicit that 未执行不算完成: an arrangement whose hour passed without being
// carried out is a real outcome, not a completed one. It also forbids building
// a second time axis or a permission platform for this.
//
// The mechanism that decides whether a thread still binds already exists
// (character.Matter.Lapsed, character.LiveMatters) and already reports the split
// to the cast block. What it never did was reach the scene block, which is the
// half of the context that says what this turn's scene actually holds. A thread
// that came due therefore stopped being listed as open and was stated nowhere
// else in the scene, so the narrator reading the scene saw silence.

// lapseTheAgreedHour commits the turn in which the story's own clock moves past
// the hour the duty was due at. It is an ordinary committed turn writing the
// scene's time field, which is how the game's own prose moves the day on.
func lapseTheAgreedHour(t *testing.T, fixture sectDuty) {
	t.Helper()
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我把东畦的事搁下了，第二天才回药园。",
		Narrative: "第二日午后你才回到药园，东畦的事已经没人再提。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
			Value: "第4日 午后", Reason: "本回合确认的时间",
		}},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"去找慕管事认个错", "当作没这回事", "先问别人"},
		},
	}); err != nil {
		t.Fatalf("commit the turn that passes the agreed hour: %v", err)
	}
}

// TestADutyThatCameDueIsReportedInTheSceneRatherThanDropped is the check the
// design's 未执行不算完成 turns into: once the agreed hour has passed, the scene
// must say the arrangement came due and was not carried out, must stop listing
// it as still open, and must not let the narrator quietly treat the day as
// uneventful.
func TestADutyThatCameDueIsReportedInTheSceneRatherThanDropped(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	lapseTheAgreedHour(t, fixture)

	action := "我回药园看看还有什么事。"
	conversation := fixture.conversation(action)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)
	scene := dutySceneBlock(t, all)

	// The hour has passed, so the thread must not still be offered as open.
	if strings.Contains(scene, "- Still open") {
		t.Fatalf("the scene still lists a thread as open after its hour passed:\n%s", scene)
	}
	// And the scene must say what became of it, in the half of the context that
	// describes this turn's scene.
	for _, want := range []string{
		"- Came due —",
		"[action] " + dutyStatement,
		"with 慕沛灵",
		"the hour passed and it was not carried out",
		dutyDeadline,
	} {
		if !strings.Contains(scene, want) {
			t.Fatalf("the scene block is missing %q after the agreed hour passed:\n%s", want, scene)
		}
	}
	// The cast block still carries the same verdict; the two halves of the
	// context must not disagree about whether the thread binds.
	if !strings.Contains(all, "Lapsed with the protagonist") {
		t.Fatalf("the cast block does not report the thread as lapsed:\n%s", all)
	}
	if strings.Contains(all, "- Unfinished with the protagonist:") {
		t.Fatalf("the cast block still lists the lapsed thread as unfinished:\n%s", all)
	}
}

// TestAForkBeforeTheAgreedHourStillOwesTheDuty keeps the branch rule the
// parallel-matters design states: a fork inherits the matters that stood at the
// fork point and advances independently, so a branch cut before the hour still
// owes the work even though the main line let it lapse.
func TestAForkBeforeTheAgreedHourStillOwesTheDuty(t *testing.T) {
	fixture := startSectDutyStory(t)
	dutyTurn := fixture.takeUpTheDuty(t)
	lapseTheAgreedHour(t, fixture)

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: dutyTurn.ID,
		Title:         "先把东畦查完再说",
	})
	if err != nil {
		t.Fatalf("fork at the turn that took the duty: %v", err)
	}

	action := "我接着去查东畦。"
	conversation := interactiveapp.NewConversation(
		fixture.store, fixture.dataRoot, fixture.workspace, fixture.story.ID, branch.ID,
		action, fixture.story.ReplyTargetChars,
		&config.Config{DenovaDir: fixture.dataRoot, Workspace: fixture.workspace},
	)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)
	scene := dutySceneBlock(t, all)

	if strings.Contains(scene, "- Came due —") {
		t.Fatalf("a branch cut before the agreed hour already reports the thread as due:\n%s", scene)
	}
	for _, want := range []string{
		"- Still open — these are not agreed, not done, and not decided:",
		"[action] " + dutyStatement,
	} {
		if !strings.Contains(scene, want) {
			t.Fatalf("the fork lost the duty it inherited (%q):\n%s", want, scene)
		}
	}
}

// TestAPermissionThatLapsedNoLongerAuthorisesAnything covers §4's 玩家失去权限后
// 不得继续调资源. The design is explicit that a permission is usually expressed by
// the world book's post description, actual appointments and current matters,
// and that no role-permission platform should be built for it. A granted
// permission is therefore recorded as the matter kind the game already has, and
// when its term ends it stops authorising the act it covered.
//
// This is a check of the recorded mechanism, not a new gate: the items module
// still prices what it is told was obtained. What the turn must not do is read a
// spent permission as still standing.
func TestAPermissionThatLapsedNoLongerAuthorisesAnything(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	const permission = "慕沛灵许顾承川从东畦取一份药苗回去试种，只此一份，今日之内。"
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我问她能不能取一份东畦的药苗。",
		Narrative: "慕沛灵想了一会儿，说东畦的药苗可以取一份，今日之内，多了不行。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"现在就去取", "等查完东畦再说", "先问清是哪一种药苗"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "permission", Status: "open",
					Statement:    permission,
					Counterparty: interactive.DefaultActorID,
					Deadline:     dutyDeadline,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the granted permission: %v", err)
	}

	// While the day still holds, the permission is live and reported as such.
	action := "我去东畦看看药苗。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	live := dutySceneBlock(t, dutyMessageText(assembled))
	if !strings.Contains(live, "[permission] "+permission) || !strings.Contains(live, "open, not settled") {
		t.Fatalf("a live permission is not reported as in force:\n%s", live)
	}

	lapseTheAgreedHour(t, fixture)

	action = "我又想起那份药苗。"
	assembled, _ = assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)
	scene := dutySceneBlock(t, all)

	if strings.Contains(scene, "[permission] "+permission+" (with 慕沛灵; open, not settled)") {
		t.Fatalf("the spent permission is still offered as in force:\n%s", scene)
	}
	if !strings.Contains(scene, "[permission] "+permission) {
		t.Fatalf("the spent permission vanished from the scene instead of being reported as spent:\n%s", scene)
	}
	// A right that ran out did not go undone; it stopped being a right. Saying it
	// "was not carried out" would leave the permitted act looking merely delayed.
	if !strings.Contains(scene, "its term ended and it no longer authorises anything") {
		t.Fatalf("the spent permission is not stated as no longer authorising:\n%s", scene)
	}
	if strings.Contains(scene, "[permission] "+permission+" (with 慕沛灵; the hour passed and it was not carried out") {
		t.Fatalf("a spent permission is described as an act left undone:\n%s", scene)
	}
}

// TestATurnThatSpentADayDoesNotLeaveTheSceneLookingStill covers the other half
// of 未执行不算完成 for the one path that moves formal time without writing the
// scene's own label.
//
// A rest, a training stretch, a craft batch or a journey advances the branch's
// formal clock and writes no 当前时间. Before this, the scene block printed only
// that label, so a day of rest left the block reading exactly as it had before —
// the same afternoon, the same open thread — and a narrator given that scene
// wrote the appointment as though its hour had not come. The elapsed interval is
// now printed beside the label, with the disagreement named.
//
// This does not make the thread lapse on the formal axis; deciding that needs a
// bound calendar, which no world pack has yet. What it stops is the scene
// asserting a stillness the committed records contradict.
func TestATurnThatSpentADayDoesNotLeaveTheSceneLookingStill(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我回房歇一夜。",
		Narrative: "你把册子放在枕边，合衣睡下。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"天亮了再说", "先去看看东畦", "去找慕管事"},
			Rest: &interactive.TurnRestInput{
				Mode: string(cultivation.RecoveryHealthRest), DurationSeconds: 24 * 60 * 60,
			},
		},
	}); err != nil {
		t.Fatalf("commit the rest: %v", err)
	}

	action := "我醒来，看看天色。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	scene := dutySceneBlock(t, dutyMessageText(assembled))

	// The label the story wrote is still the afternoon the duty was taken on.
	if !strings.Contains(scene, "- Time: "+dutyWorldTime) {
		t.Fatalf("the scene lost the label the story wrote:\n%s", scene)
	}
	// A full day has really passed, and the block has to say so.
	if !strings.Contains(scene, "- Formal time: 1日") {
		t.Fatalf("the scene does not report the day the rest spent:\n%s", scene)
	}
	// And it has to name which of the two numbers moved, or the disagreement
	// reads as one of them being wrong.
	if !strings.Contains(scene, "not a reading of this clock") {
		t.Fatalf("the scene does not say the label is not a clock reading:\n%s", scene)
	}
}

// TestALapsedDutyCanStillBeDeliveredLate keeps §4's other half: 已成立安排按真实
// 交接和执行条件处理，不统一全部取消. A lapse is not the rules cancelling the
// arrangement — it is the hour having passed. The work can still be handed over
// late, and once it is, the story continues from the delivery rather than from
// the missed hour.
func TestALapsedDutyCanStillBeDeliveredLate(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	lapseTheAgreedHour(t, fixture)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)

	lateStatement := "东畦是隔日才查完的，病株册子迟了一天才交回给慕沛灵。"
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我把东畦查完，隔日把册子交回给慕管事。",
		Narrative: "你把东畦补查了一遍，隔日把册子放到慕沛灵案上。她看了半晌，只说了一句：晚了。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
			Value: "第5日 清晨", Reason: "本回合确认的时间",
		}},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"解释为什么迟了", "什么也不说，等她开口", "问她往后还有没有差事"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    lateStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the late delivery: %v", err)
	}

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after the late delivery: %v", err)
	}
	delivered := dutyMatter(t, state)
	if delivered.Status != character.MatterSettled {
		t.Fatalf("a late delivery did not settle the arrangement: status = %q", delivered.Status)
	}

	action := "我在药园里等她再开口。"
	conversation := fixture.conversation(action)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)
	scene := dutySceneBlock(t, all)

	// Once it really happened, the scene must stop calling it missed. The lapse
	// was true while it lasted; it is not true after the work was handed over.
	if strings.Contains(scene, "Came due and was not carried out") {
		t.Fatalf("the scene still reports the duty as missed after it was delivered late:\n%s", scene)
	}
	if strings.Contains(scene, "- Came due —") {
		t.Fatalf("the scene still reports the duty as missed after it was delivered late:\n%s", scene)
	}
	if strings.Contains(scene, "- Still open") {
		t.Fatalf("the scene still lists the delivered duty as open:\n%s", scene)
	}
	// The delivery is its own record and the story keeps it, so a later turn can
	// name what really happened rather than the hour that was missed.
	held := false
	for _, matter := range state.Matters {
		if strings.TrimSpace(matter.Normalize().Statement) == lateStatement {
			held = true
			break
		}
	}
	if !held {
		t.Fatalf("the late delivery was not kept on record; the story holds %d threads", len(state.Matters))
	}
}

// TestALapsedDutySurvivesAReopen covers 读档保留结果: the split between what still
// binds and what came due is derived from committed records, so closing and
// reopening the program must not reset it or duplicate it.
func TestALapsedDutySurvivesAReopen(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	lapseTheAgreedHour(t, fixture)

	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	action := "我回药园看看还有什么事。"
	conversation := fixture.conversation(action)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)
	scene := dutySceneBlock(t, all)

	if strings.Contains(scene, "- Still open") {
		t.Fatalf("the reopened story offers the lapsed duty as still open:\n%s", scene)
	}
	if !strings.Contains(scene, "- Came due —") {
		t.Fatalf("the reopened story forgot the duty came due:\n%s", scene)
	}
	// The record itself is unchanged: one thread, still open in the ledger, and
	// the lapse is a reading of it rather than a rewrite of it.
	state, err := reopened.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after reopening: %v", err)
	}
	if len(state.Matters) != 1 {
		t.Fatalf("reopening produced %d threads, want the one that was committed", len(state.Matters))
	}
	if matter := dutyMatter(t, state); matter.Status != character.MatterOpen {
		t.Fatalf("reopening rewrote the thread's status to %q; a lapse is a reading, not a rewrite", matter.Status)
	}
}
