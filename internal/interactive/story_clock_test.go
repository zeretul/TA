package interactive

import (
	"math"
	"testing"
)

func TestAdvanceStoryClockRejectsOverflow(t *testing.T) {
	clock := StoryClock{Version: StoryClockVersion, Seconds: math.MaxInt64 - 10}
	if _, err := AdvanceStoryClock(clock, 11); err == nil {
		t.Fatal("advancing the formal story clock past int64 range must be refused")
	}
}

func TestLegacyFirstWaitClockOriginUsesSceneAtTurnStart(t *testing.T) {
	branch := &storyBranchProjection{State: map[string]any{
		actorStateRoot: map[string]any{
			DefaultStoryContextActorID: map[string]any{"state": map[string]any{storySceneTimeField: "雾港清晨"}},
		},
	}}
	turn := TurnEvent{
		User: "在雾港等候一日",
		StateDelta: &StateDelta{ActorOps: []ActorStateOp{{
			Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneTimeField, Value: "雾港次日清晨",
		}}},
		ClockAdvance: &TurnClockAdvance{
			Action: TurnWorldActionWait, SourceText: "等候一日", DurationSeconds: 24 * 60 * 60,
			Before: StoryClock{Version: StoryClockVersion, Anchor: "雾港次日清晨"},
			After:  StoryClock{Version: StoryClockVersion, Anchor: "雾港次日清晨", Seconds: 24 * 60 * 60},
		},
	}
	if !normalizeLegacyFirstWaitClockOrigin(branch, &turn) {
		t.Fatal("legacy clock anchor matching the Turn's post-state scene must be normalized")
	}
	if turn.ClockAdvance.Before.Anchor != "雾港清晨" || turn.ClockAdvance.After.Anchor != "雾港清晨" {
		t.Fatalf("normalized clock anchors = %q -> %q, want the scene at turn start", turn.ClockAdvance.Before.Anchor, turn.ClockAdvance.After.Anchor)
	}
	if err := applyTurnClockAdvance(branch, turn); err != nil {
		t.Fatalf("replay normalized legacy clock advance: %v", err)
	}
	applyTurnState(branch.State, turn)
	if branch.Clock == nil || branch.Clock.Anchor != "雾港清晨" || branch.Clock.Seconds != 24*60*60 {
		t.Fatalf("replayed branch clock = %+v, want turn-start anchor and one day", branch.Clock)
	}
	if got := storySceneText(branch.State, storySceneTimeField); got != "雾港次日清晨" {
		t.Fatalf("replay changed the persisted scene time: %q", got)
	}
}

func TestLegacyFirstWaitClockOriginRejectsUnrelatedAnchor(t *testing.T) {
	branch := &storyBranchProjection{State: map[string]any{
		actorStateRoot: map[string]any{
			DefaultStoryContextActorID: map[string]any{"state": map[string]any{storySceneTimeField: "雾港清晨"}},
		},
	}}
	turn := TurnEvent{
		StateDelta: &StateDelta{ActorOps: []ActorStateOp{{
			Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneTimeField, Value: "雾港次日清晨",
		}}},
		ClockAdvance: &TurnClockAdvance{
			Before: StoryClock{Version: StoryClockVersion, Anchor: "不相关时间"},
			After:  StoryClock{Version: StoryClockVersion, Anchor: "不相关时间", Seconds: 24 * 60 * 60},
		},
	}
	if normalizeLegacyFirstWaitClockOrigin(branch, &turn) {
		t.Fatal("an anchor unrelated to both the branch and Turn's post-state scene must not be normalized")
	}
}
