package app

import (
	"strings"
	"testing"

	"ta/config"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// This file covers F19's age half: 年龄按实际游戏时间增加, on the same formal clock
// a rest, a training stretch or a journey already moves.
//
// The design (自制-时间寿元与长时世界推进 §2) says the whole world, one game and one
// branch share one formal time line, and that 年龄按实际游戏时间增加，显示整岁不
// 丢失底层已过时长. It also says a new character has 可追溯的初始年龄与初态时刻,
// and that 未知生日不由AI猜成精确日期 — 可用 "初态时刻＋已确认年龄" 计算.
//
// What the game already had was both halves and no join. The world book's authored
// age is frozen into the story's catalog at creation, and the branch's formal
// clock counts the seconds an activity really spent. Nothing read the two
// together, so a decade of seclusion moved the clock and left every person in the
// story exactly the age they were written at — the narrator, given a scene that
// says ten years have passed and a cast that says nobody aged, wrote a decade in
// which nobody grew older.
//
// The span of this batch is deliberately the age, not the lifespan. §3 and §10 are
// explicit that 参数尚未实际绑定并完成接入验收时，不启用自然寿尽结果, so no one dies
// of old age here and no lifespan table is consulted.

// aRuleYear is the interval the game already treats as one year when a player
// names 闭关十年: the cultivation module converts a rule year to twelve rule
// months of thirty days. Age reuses that one interpretation rather than making a
// second one, which is what §2 asks for when it says 同一句"闭关三年"只解释一次.
const aRuleYear = int64(interactive.DaysPerRuleYear) * 24 * 60 * 60

// secludeFor commits one long training interval on the story's formal clock.
func secludeFor(t *testing.T, fixture sectDuty, seconds int64) {
	t.Helper()
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我闭门打坐，不问外事。",
		Narrative: "你在洞府里坐了下来，外头的日子一天一天过去。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"继续坐下去", "出去看看", "想想别的事"},
			Rest: &interactive.TurnRestInput{
				Mode: string(cultivation.RecoverySafeMeditation), DurationSeconds: seconds,
			},
		},
	}); err != nil {
		t.Fatalf("commit the seclusion: %v", err)
	}
}

// castMemberBlock returns one character's own section of the cast block.
func castMemberBlock(t *testing.T, turn, name string) string {
	t.Helper()
	start := strings.Index(turn, "### "+name+" (id: ")
	if start < 0 {
		t.Fatalf("the cast block does not list %s:\n%s", name, turn)
	}
	if end := strings.Index(turn[start+1:], "\n### "); end >= 0 {
		return turn[start : start+1+end]
	}
	return turn[start:]
}

// TestALongSeclusionAgesTheCastOnTheFormalClock is the check §2 turns into: after
// a decade of the story's own formal time, the people in it are ten years older,
// and the block says so with the age they were established at rather than
// replacing it.
func TestALongSeclusionAgesTheCastOnTheFormalClock(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	secludeFor(t, fixture, 10*aRuleYear)

	action := "我出关了，去药园看看。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	// 慕沛灵 is authored at 46 in the world book the story froze.
	mu := castMemberBlock(t, all, "慕沛灵")
	if !strings.Contains(mu, "Age: 56") {
		t.Fatalf("a decade of formal time did not age the steward:\n%s", mu)
	}
	// The age she was established at is kept beside it, so the narrator can write
	// a decade of change without losing who she was.
	if !strings.Contains(mu, "46 when this story began") {
		t.Fatalf("the cast block replaced the established age instead of dating it:\n%s", mu)
	}

	// The protagonist was created at 22 and is the one who sat down.
	scene := dutySceneBlock(t, all)
	if !strings.Contains(scene, "32") || !strings.Contains(scene, "22 when this story began") {
		t.Fatalf("the scene block did not age the protagonist:\n%s", scene)
	}
	// And the interval that aged them is still stated as the committed fact it is.
	if !strings.Contains(scene, "- Formal time: ") {
		t.Fatalf("the scene block lost the formal interval:\n%s", scene)
	}
}

// TestTheSceneStatesTheLifespanEdgeBeforeTheClockReachesIt is §4's 越界前提示, read
// off the assembled context rather than off the helper.
//
// The rules cut the interval at the edge and the commit declares 寿尽 past it, but
// both of those happen after the model has written the turn. What makes the
// warning possible is that the scene block states the numbers beforehand: this
// realm's confirmed total, the age the life ends at, and how much formal time is
// left. A narrator given only the clock would write a decade of seclusion and be
// told afterwards that it had killed the character mid-sentence.
func TestTheSceneStatesTheLifespanEdgeBeforeTheClockReachesIt(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	secludeFor(t, fixture, 10*aRuleYear)

	action := "我出关了，去药园看看。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	scene := dutySceneBlock(t, dutyMessageText(assembled))

	// The 外门弟子 template seeds 当前境界 as 炼气六层, whose confirmed row is 120
	// years, and the creation page established 22.
	if !strings.Contains(scene, "- Lifespan: ") {
		t.Fatalf("the scene block never states the lifespan edge:\n%s", scene)
	}
	if !strings.Contains(scene, "confirmed limit is 120 years") {
		t.Fatalf("the scene block lost the 炼气 row:\n%s", scene)
	}
	// Ten years spent: 32 now, 88 whole years left. Both numbers are the
	// character's own, and both have to move with the clock rather than with the
	// age the pack wrote.
	if !strings.Contains(scene, "You are 32 now") || !strings.Contains(scene, "leaves 88 whole years") {
		t.Fatalf("the scene block did not follow the clock to the edge:\n%s", scene)
	}
	// The warning is not a countdown. §4 asks for the situation to be stated when
	// the scene makes it relevant, and a model told only the numbers repeats them
	// in every turn.
	if !strings.Contains(scene, "do not turn it into a countdown") {
		t.Fatalf("the scene block gave the numbers without the instruction that keeps them out of the prose:\n%s", scene)
	}
}

// TestAShortIntervalDoesNotAgeAnyone keeps 显示整岁 honest: an interval that has
// not reached a whole year leaves the age where it was, and the block does not
// claim a birthday nobody had.
func TestAShortIntervalDoesNotAgeAnyone(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	secludeFor(t, fixture, 3*30*24*60*60)

	action := "我出关了。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	mu := castMemberBlock(t, all, "慕沛灵")
	if strings.Contains(mu, "Age:") {
		t.Fatalf("a three-month interval claimed a birthday nobody had:\n%s", mu)
	}
	if scene := dutySceneBlock(t, all); strings.Contains(scene, "when this story began") {
		t.Fatalf("a three-month interval aged the protagonist:\n%s", scene)
	}
}

// TestAForkKeepsTheAgeItsOwnClockReached keeps time branch-scoped, the same rule
// the clock itself follows: a branch cut before the seclusion never spent those
// years, so nobody on it is older.
func TestAForkKeepsTheAgeItsOwnClockReached(t *testing.T) {
	fixture := startSectDutyStory(t)
	dutyTurn := fixture.takeUpTheDuty(t)
	secludeFor(t, fixture, 10*aRuleYear)

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: dutyTurn.ID,
		Title:         "不闭关的那一支",
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

	if mu := castMemberBlock(t, all, "慕沛灵"); strings.Contains(mu, "Age:") {
		t.Fatalf("a branch that never spent the decade aged its people anyway:\n%s", mu)
	}
	if scene := dutySceneBlock(t, all); strings.Contains(scene, "when this story began") {
		t.Fatalf("a branch that never spent the decade aged the protagonist:\n%s", scene)
	}
}

// TestTheAgeSurvivesAReopen covers 读档保留结果 for the age: it is derived from
// the frozen catalog and the branch's committed clock, so closing and reopening
// must read back the same age rather than resetting it or ageing twice.
func TestTheAgeSurvivesAReopen(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	secludeFor(t, fixture, 10*aRuleYear)

	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	reopened := interactive.NewStore(fixture.workspace)
	t.Cleanup(func() { _ = reopened.Close() })
	fixture.store = reopened

	action := "我出关了，去药园看看。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	if mu := castMemberBlock(t, all, "慕沛灵"); !strings.Contains(mu, "Age: 56") {
		t.Fatalf("reopening reset or double-counted the age:\n%s", mu)
	}
	if scene := dutySceneBlock(t, all); !strings.Contains(scene, "32") {
		t.Fatalf("reopening reset the protagonist's age:\n%s", scene)
	}
}

// TestSegmentedSeclusionAgesTheSameAsOneLongOne covers TIME-02's 多年与同条件分段
// 推进结果一致 for the age: a decade taken in two halves must leave the same age as
// a decade taken at once, so a player who was interrupted and resumed is not
// older or younger than one who was not.
func TestSegmentedSeclusionAgesTheSameAsOneLongOne(t *testing.T) {
	oneLong := startSectDutyStory(t)
	oneLong.takeUpTheDuty(t)
	secludeFor(t, oneLong, 10*aRuleYear)

	segmented := startSectDutyStory(t)
	segmented.takeUpTheDuty(t)
	secludeFor(t, segmented, 4*aRuleYear)
	secludeFor(t, segmented, 6*aRuleYear)

	read := func(fixture sectDuty) (string, string) {
		action := "我出关了，去药园看看。"
		assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
		all := dutyMessageText(assembled)
		return castMemberBlock(t, all, "慕沛灵"), dutySceneBlock(t, all)
	}
	longCast, longScene := read(oneLong)
	segmentedCast, segmentedScene := read(segmented)

	// Only the age readings are compared. The two stories mint their own matter
	// ids, so the blocks differ in exactly the way they are supposed to.
	if got, want := ageLines(segmentedCast), ageLines(longCast); got != want {
		t.Fatalf("a decade in two halves aged the cast differently from a decade at once:\nsegmented: %q\none long:  %q", got, want)
	}
	if got, want := ageLines(segmentedScene), ageLines(longScene); got != want {
		t.Fatalf("a decade in two halves left a different age in the scene than a decade at once:\nsegmented: %q\none long:  %q", got, want)
	}
	if !strings.Contains(segmentedCast, "Age: 56") {
		t.Fatalf("the segmented story did not reach the decade's age:\n%s", segmentedCast)
	}
}

// ageLines keeps only the lines that state an age, so a comparison between two
// stories is about the ages and not about the ids they minted.
func ageLines(block string) string {
	lines := make([]string, 0, 2)
	for _, line := range strings.Split(block, "\n") {
		if strings.Contains(line, "Age: ") || strings.Contains(line, "age: ") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return strings.Join(lines, "\n")
}

// TestAnInventedCharacterGetsNoInventedAge keeps §2's 未知生日不由AI猜成精确日期: a
// person the story invented has no authored age, and the block must say nothing
// rather than derive one from the clock.
func TestAnInventedCharacterGetsNoInventedAge(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我问药园里那个新来的杂役叫什么。",
		Narrative: "杂役说自己叫阿石，来药园才半年。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问他从哪来", "让他去挑水", "不再理会"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "a_shi", Name: "阿石", Role: "药园杂役",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the invented character's first appearance: %v", err)
	}
	secludeFor(t, fixture, 10*aRuleYear)

	action := "我出关了，去药园看看。"
	assembled, _ := assembleDutyTurn(t, fixture.conversation(action), action)
	all := dutyMessageText(assembled)

	if block := castMemberBlock(t, all, "阿石"); strings.Contains(block, "Age:") {
		t.Fatalf("the story invented an age for a person whose age nobody stated:\n%s", block)
	}
}
