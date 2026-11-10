package app

import (
	"strings"
	"testing"

	"ta/config"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file covers F20: a faction that really changes over a long interval,
// with a source the player can see, and one world that keeps moving whether or
// not the player takes part.
//
// The design (自制-长期势力演变 FAC-01…FAC-06) asks for three things this build
// can already carry, and the tests below are about whether they are joined to a
// real turn rather than existing as separate pieces:
//
//   - FAC-01: 长时行动后相关势力可合理改变，变化有来源和实际条件. The world
//     entities Actor already owns a 势力记录 object whose instruction names 存续
//     状态 and 当前行动. What was unproven is that a write to it survives the
//     commit, comes back in the next turn's handbook, and stays on its own branch.
//   - FAC-03: 不参与不冻结世界. 太岳灵潮 derives its stage from the branch's formal
//     clock alone, so a player who spends a week elsewhere comes back to a
//     different stage of it. Nothing schedules that; it is a pure function of
//     time, which is exactly what the design asks for.
//   - FAC-05/FAC-06: 幕后与传闻不当全知公开事实, and 重启及分支读取同一事件结果.
//
// What these tests deliberately do not claim is that the model always writes a
// sourced change. The record's own instruction asks for 当前行动 but does not
// require a cause, and a long interval does not tell the narrator that a record
// written before it may have expired. Both are registered in
// work/repair-batches.md as open rather than papered over here.

// aFactionStory starts the same real story as the other batches and stands the
// character in the one place the authored public event can be heard of from.
func aFactionStory(t *testing.T) sectDuty {
	t.Helper()
	fixture := startSectDutyStory(t)
	fixture.standInTheMarket(t)
	return fixture
}

// standInTheMarket commits the turn that puts the character at 公共坊市, which is
// the region 太岳灵潮 is registered against. A story somewhere else is not told
// about the event at all — that is FAC-05, not an omission.
func (f sectDuty) standInTheMarket(t *testing.T) interactive.TurnEvent {
	t.Helper()
	turn, _, err := f.store.AppendTurnWithState(f.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我下山去坊市一趟。",
		Narrative: "你沿山路走到坊市口，摊子挨着摊子。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
			Value: "公共坊市", Reason: "本回合确认的地点",
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"四处看看", "找个摊子坐下", "回山"}},
	})
	if err != nil {
		t.Fatalf("stand in the market: %v", err)
	}
	return turn
}

// recordFaction writes one faction record the way the model does: a whole
// object under the field the handbook names, on the world entities Actor.
func recordFaction(t *testing.T, fixture sectDuty, narrative string, record map[string]any) {
	t.Helper()
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我去问份例的事。",
		Narrative: narrative,
		TurnResult: &interactive.TurnResult{
			Choices: []string{"再等等", "去问别人", "先回去打坐"},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/world/势力记录",
				Value: map[string]any{"黄枫谷": record},
			}},
		},
	}); err != nil {
		t.Fatalf("commit the faction record: %v", err)
	}
}

// readTurn assembles and commits one turn, returning everything the narrator
// was given.
func readTurn(t *testing.T, conversation *interactiveapp.Conversation, action string) string {
	t.Helper()
	assembled, _ := assembleDutyTurn(t, conversation, action)
	return dutyMessageText(assembled)
}

// TestAFactionChangeReachesTheNextTurn is FAC-01's floor: a change the story
// really committed is in the handbook the next turn reads, with the state the
// narrative gave it rather than the empty object it started as.
func TestAFactionChangeReachesTheNextTurn(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	recordFaction(t, fixture, "执事翻了翻册子，说这个月的灵石还没到。", map[string]any{
		"类型":   "宗门",
		"核心特征": "越国七大派之一，以灵植与丹道见长",
		"存续状态": "存续",
		"对主角立场": "门下外门弟子",
		"主要范围": "越国黄枫谷一带",
		"当前行动": "本月灵石份例迟发，执事只说过几日再来",
	})

	all := readTurn(t, fixture.conversation("我在药园里等消息。"), "我在药园里等消息。")

	if !strings.Contains(all, "黄枫谷") {
		t.Fatalf("the next turn was not told the faction it is standing in:\n%s", all)
	}
	if !strings.Contains(all, "份例迟发") {
		t.Fatalf("the committed change did not reach the narrator:\n%s", all)
	}
	// The record is the one the handbook names, not a second structure.
	if !strings.Contains(all, "势力记录") {
		t.Fatal("the faction record is missing from the Actor State handbook")
	}
}

// TestAFactionRecordIsReadAsTheWorldEntitiesState keeps the two records apart:
// a faction is not a location and the handbook must not have merged them, or a
// later batch would find one field where the design asks for two.
func TestAFactionRecordIsReadAsTheWorldEntitiesState(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	recordFaction(t, fixture, "执事说本月灵石迟发。", map[string]any{
		"类型": "宗门", "存续状态": "存续", "当前行动": "本月灵石份例迟发",
	})

	all := readTurn(t, fixture.conversation("我先回去打坐。"), "我先回去打坐。")

	// 地点记录 is still its own empty object; the faction write did not land there.
	locationStart := strings.Index(all, "#### 地点记录")
	factionStart := strings.Index(all, "#### 势力记录")
	if locationStart < 0 || factionStart < 0 || factionStart < locationStart {
		t.Fatalf("the world entities template lost its two records:\n%s", all)
	}
	locationBlock := all[locationStart:factionStart]
	if strings.Contains(locationBlock, "黄枫谷") {
		t.Fatalf("the faction was written into the location record:\n%s", locationBlock)
	}
}

// TestTheWorldMovesWhileThePlayerIsAway is FAC-03 with a real clock: 太岳灵潮 is
// at 风声 when the character arrives, a week of seclusion passes elsewhere, and
// the same branch comes back to a closed 潮落 without anybody scheduling it.
func TestTheWorldMovesWhileThePlayerIsAway(t *testing.T) {
	fixture := aFactionStory(t)

	before := readTurn(t, fixture.conversation("我在坊市里走走。"), "我在坊市里走走。")
	if !strings.Contains(before, "太岳灵潮") {
		t.Fatalf("the event the region is talking about is not in the turn:\n%s", before)
	}
	if !strings.Contains(before, "current stage 风声") {
		t.Fatalf("a fresh story is not at the event's first stage:\n%s", before)
	}

	secludeFor(t, fixture, 7*24*60*60)

	after := readTurn(t, fixture.conversation("我出关了。"), "我出关了。")
	if !strings.Contains(after, "current stage 潮落") {
		t.Fatalf("a week of formal time left the event where it was:\n%s", after)
	}
	if !strings.Contains(after, "Closed: no new participation") {
		t.Fatalf("the event reached its final stage without closing:\n%s", after)
	}
}

// TestTheEventIsOnlyKnownWhereItCanBeHeard is FAC-05's other half: the event is
// really happening, but a character in 黄枫谷 has not been told about it. The
// design separates 事态已发生 from 玩家是否获知.
func TestTheEventIsOnlyKnownWhereItCanBeHeard(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	all := readTurn(t, fixture.conversation("我在药园里等消息。"), "我在药园里等消息。")

	if strings.Contains(all, "太岳灵潮") {
		t.Fatalf("a character who never went to the market was told about the market's event:\n%s", all)
	}
}

// TestTheSecretBehindTheEventStaysBehindIt is the rest of FAC-05: the authored
// secret lines and motives never reach the turn, even at the stage they belong
// to, because nothing the player did uncovered them.
func TestTheSecretBehindTheEventStaysBehindIt(t *testing.T) {
	fixture := aFactionStory(t)
	secludeFor(t, fixture, 7*24*60*60)

	all := readTurn(t, fixture.conversation("我出关了。"), "我出关了。")

	for _, secret := range []string{
		"东家让他压着价收", // the herb clerk's motive
		"他想借这场灵潮抽一笔过路钱",
		"执事扣下的一半货物",
	} {
		if strings.Contains(all, secret) {
			t.Fatalf("a secret line reached the narrator: %q\n%s", secret, all)
		}
	}
}

// TestTheFactionRecordSurvivesAReopen is 保存恢复 for FAC-06: the record is on
// the journal, so a reopened store reads the same faction state rather than the
// empty object the story started with.
func TestTheFactionRecordSurvivesAReopen(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	recordFaction(t, fixture, "执事说本月灵石迟发。", map[string]any{
		"类型": "宗门", "存续状态": "存续", "当前行动": "本月灵石份例迟发",
	})

	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	all := readTurn(t, fixture.conversation("我回来了。"), "我回来了。")
	if !strings.Contains(all, "份例迟发") {
		t.Fatalf("reopening lost the faction change the story committed:\n%s", all)
	}
}

// TestAForkKeepsTheFactionStateItSaw is the branch half of FAC-06: a branch cut
// before a faction changed keeps the older record, and the main line keeps the
// newer one. The two must not read each other's history.
func TestAForkKeepsTheFactionStateItSaw(t *testing.T) {
	fixture := startSectDutyStory(t)
	forkPoint := fixture.takeUpTheDuty(t)
	recordFaction(t, fixture, "执事说本月灵石迟发。", map[string]any{
		"类型": "宗门", "存续状态": "存续", "当前行动": "本月灵石份例迟发",
	})

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: forkPoint.ID, Title: "不领份例的那一支",
	})
	if err != nil {
		t.Fatalf("fork before the change: %v", err)
	}

	// The main line moves on; the fork must not see it.
	recordFaction(t, fixture, "执事说这个月灵石彻底不发了。", map[string]any{
		"类型": "宗门", "存续状态": "存续", "当前行动": "本月灵石彻底停发",
	})

	branchConversation := interactiveapp.NewConversation(
		fixture.store, fixture.dataRoot, fixture.workspace, fixture.story.ID, branch.ID,
		"我接着等。", fixture.story.ReplyTargetChars,
		&config.Config{DenovaDir: fixture.dataRoot, Workspace: fixture.workspace},
	)
	branchText := readTurn(t, branchConversation, "我接着等。")
	if strings.Contains(branchText, "彻底停发") {
		t.Fatalf("a branch cut before the change was told about it:\n%s", branchText)
	}

	mainText := readTurn(t, fixture.conversation("我再问问。"), "我再问问。")
	if !strings.Contains(mainText, "彻底停发") {
		t.Fatalf("the main line lost the change it committed:\n%s", mainText)
	}
}

// TestALongIntervalDatesTheFactionRecordRatherThanRestatingIt closes FAC-05's
// other half: 事态已发生、玩家是否获知、各NPC分别知道多少分开, and the part that
// follows from it — a record written a decade ago is not the world's present.
//
// The gap this file used to record is closed. The world entities Actor now
// carries the turn that wrote it and the branch instant it stood at, so a turn
// that reads the record after a long interval is told both. The record keeps its
// words — nothing rewrites history — and the turn is told how old they are, which
// is the only thing that stops a stale 当前行动 being written as current.
//
// The story froze a calendar at creation, so the interval is stated in its own
// units (10年) and the record's own instant is dated on that same calendar.
func TestALongIntervalDatesTheFactionRecordRatherThanRestatingIt(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	recordFaction(t, fixture, "执事说本月灵石迟发。", map[string]any{
		"类型": "宗门", "存续状态": "存续", "当前行动": "本月灵石份例迟发",
	})

	// Before any interval the record is this morning's: it is dated, and it says
	// nothing about having aged.
	action := "我去问问份例的事。"
	fresh := readTurn(t, fixture.conversation(action), action)
	if !strings.Contains(fresh, "本月灵石份例迟发") {
		t.Fatalf("the record did not reach the next turn at all:\n%s", fresh)
	}
	if !strings.Contains(fresh, "- Written: ") {
		t.Fatalf("the record does not say which turn wrote it:\n%s", fresh)
	}
	for _, aged := range []string{"此后又过了", "不一定是现在的样子"} {
		if strings.Contains(fresh, aged) {
			t.Fatalf("a record written this turn is already reported as old (%q):\n%s", aged, fresh)
		}
	}

	secludeFor(t, fixture, 10*aRuleYear)
	afterAction := "我出关了。"
	all := readTurn(t, fixture.conversation(afterAction), afterAction)

	// The decade is stated in the story's own units rather than as raw days, so
	// the check below is testing a decade and not an arbitrary interval.
	if !strings.Contains(all, "Formal time: 10年") {
		t.Fatalf("the decade was not stated, so this check is not testing what it claims:\n%s", all)
	}
	// The record itself is untouched: the words are the ones that were written.
	if !strings.Contains(all, "本月灵石份例迟发") {
		t.Fatalf("the record's words were rewritten or dropped:\n%s", all)
	}
	// And the turn is told how old it is, in the record's own terms. This is the
	// sentence a model needs in order not to carry it forward as current.
	for _, want := range []string{"- Written: ", "此后又过了", "不一定是现在的样子"} {
		if !strings.Contains(all, want) {
			t.Fatalf("the turn reading a decade-old record is missing %q:\n%s", want, all)
		}
	}
}
