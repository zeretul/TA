package interactive

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/publiquevent"
	"ta/internal/storytime"
)

// This file is §B1's stop-point half: an interval must settle only up to the
// moment the story really stops at, save the rest of the intent, and continue it
// only when the player says so.
//
// 设计稿/Claude_返修优先与剧情助手续工_20261018.md line 37: "到真实瓶颈、资源/条件
// 变化、已登记重要节点时仅结算到边界，保存剩余意图；明确授权继续巩固等才继续相应
// 时间。普通信息合并，遇真正决定才停，不造全世界调度。停点从后端正式事实收集，不能
// 依赖客户端自报nodes/resolved而跳过事项。读取/关窗不推进时间。"
//
// The stop points here come from the branch's own committed facts: the rule
// table's major-realm bottleneck, and the stages of a public event the character
// already took part in. The tests prove both, and prove that a caller-supplied
// node list can neither invent a stop nor resolve one away.

// b2StopStoryAt builds a story whose scene is in the market (so the public event
// is known) and whose protagonist sits on the last refining layer with the
// threshold met and a whole body.
//
// The two conditions are independent on purpose: the bottleneck test needs a
// character who cannot train further, and the event test needs a clock that a
// real event stage crosses.
func b2StopStoryAt(t *testing.T, state map[string]any) (*Store, string) {
	t.Helper()
	system := publicEventTestCombatSystem(publicEventTestMarket)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != DefaultActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		for field, value := range state {
			system.InitialActors[index].State[field] = value
		}
	}
	return publicEventTestStoryOn(t, system)
}

// b2StopActivities returns the branch's committed cultivation activities.
func b2StopActivities(t *testing.T, store *Store, storyID string) map[string]CultivationActivity {
	t.Helper()
	_, activities, err := store.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return activities
}

// TestB2TheBottleneckSavesTheRestOfTheIntent is §7 step 4 at the rules' own
// boundary.
//
// A year named at a full threshold trains nothing: the rules report the boundary
// and spend no time. The turn must still publish the intent, paused with the
// whole year still owed, or the player's next line would be read as a brand new
// request and the year they asked for would be lost.
func TestB2TheBottleneckSavesTheRestOfTheIntent(t *testing.T) {
	store, storyID := b2StopStoryAt(t, b1BreakthroughFixture(t))

	clock := b1TurnTrain(t, store, storyID, "cmd-b2-bottleneck", TurnCultivationInput{Days: 365})
	activities := b2StopActivities(t, store, storyID)

	var paused []CultivationActivity
	for _, activity := range activities {
		if activity.Activity.Status == storytime.ActivityPaused {
			paused = append(paused, activity)
		}
	}
	if len(paused) != 1 {
		t.Fatalf("a year at a real bottleneck must leave exactly one paused intent: %#v", activities)
	}
	intent := paused[0]
	if intent.ActorID != DefaultActorID {
		t.Fatalf("the saved intent belongs to the protagonist: %#v", intent)
	}
	const year = int64(365 * 24 * 60 * 60)
	if got := int64(intent.Activity.RequiredEffective); got != year {
		t.Fatalf("the saved intent must be the interval the player named: got=%d want=%d", got, year)
	}
	if got := int64(intent.Activity.CompletedEffective); got != 0 {
		t.Fatalf("a boundary that spent nothing must not claim progress: got=%d", got)
	}
	if len(intent.Activity.PendingNodeIDs) == 0 {
		t.Fatalf("the pause must name what it is waiting on: %#v", intent)
	}
	if clock.Seconds != 0 {
		t.Fatalf("the boundary is a decision, not time that passed: clock=%d", clock.Seconds)
	}
}

// TestB2AContinuationSettlesOnlyTheRemainder is §7 step 5.
//
// The paused intent is continued rather than reopened: the activity keeps its
// identity and its origin, and the progress accumulates toward the same target.
// A player who names a short continuation trains the short continuation and
// leaves the rest owed; naming a longer one settles the remainder and no more.
//
// The pause here is a committed event stage rather than the rules' bottleneck,
// because a character at the bottleneck cannot train at all: the rules spend
// nothing, so there would be no progress to accumulate.
func TestB2AContinuationSettlesOnlyTheRemainder(t *testing.T) {
	store, storyID := b2StopStoryAt(t, nil)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionObserve),
	})

	clock := b1TurnTrain(t, store, storyID, "cmd-b2-first", TurnCultivationInput{Days: 1})
	const rushStage = int64(6 * 60 * 60)
	if clock.Seconds != rushStage {
		t.Fatalf("the first turn must stop at the event's stage: clock=%d want=%d", clock.Seconds, rushStage)
	}
	activities := b2StopActivities(t, store, storyID)
	if len(activities) != 1 {
		t.Fatalf("the first turn must publish one intent: %#v", activities)
	}
	var paused CultivationActivity
	for _, activity := range activities {
		paused = activity
	}

	// One hour is less than the eighteen still owed, so the intent stays paused
	// with the rest of the day owed.
	clock = b1TurnTrain(t, store, storyID, "cmd-b2-second", TurnCultivationInput{Days: float64(1) / 24})
	after := b2StopActivities(t, store, storyID)
	continued, ok := after[paused.ID]
	if !ok {
		t.Fatalf("a continuation must reuse the interrupted activity's id, not add a new one: %#v", after)
	}
	if len(after) != 1 {
		t.Fatalf("the branch must read one uninterrupted intent: %#v", after)
	}
	const day = int64(24 * 60 * 60)
	if got := int64(continued.Activity.RequiredEffective); got != day {
		t.Fatalf("the target stays the original named interval: got=%d want=%d", got, day)
	}
	if got := int64(continued.Activity.CompletedEffective); got != rushStage+3600 {
		t.Fatalf("the continuation settles only what it trained: got=%d want=%d", got, rushStage+3600)
	}
	if continued.Activity.Status != storytime.ActivityPaused {
		t.Fatalf("the rest of the day is still owed, so the intent stays paused: %#v", continued.Activity)
	}
	if continued.Activity.StartedAt != paused.Activity.StartedAt {
		t.Fatalf("the continuation keeps the interrupted origin: %v -> %v", paused.Activity.StartedAt, continued.Activity.StartedAt)
	}
	if clock.Seconds != rushStage+3600 {
		t.Fatalf("the continuation spends its own hour: clock=%d", clock.Seconds)
	}

	// Naming more than is owed settles the remainder and completes the intent.
	b1TurnTrain(t, store, storyID, "cmd-b2-third", TurnCultivationInput{Days: 100})
	final := b2StopActivities(t, store, storyID)[paused.ID]
	if got := int64(final.Activity.CompletedEffective); got != day {
		t.Fatalf("a longer continuation settles only the remainder: got=%d want=%d", got, day)
	}
	if final.Activity.Status != storytime.ActivityCompleted {
		t.Fatalf("the intent is finished once its target is met: %#v", final.Activity)
	}
}

// TestB2ABreakthroughIsNotFoldedIntoThePausedSeclusion is the boundary between
// the two operations.
//
// The player's next line is an authorised attempt, not more of the interrupted
// seclusion. Its seven days are the attempt's own, and the seclusion's remainder
// must survive it rather than be overwritten by the attempt's record.
func TestB2ABreakthroughIsNotFoldedIntoThePausedSeclusion(t *testing.T) {
	store, storyID := b2StopStoryAt(t, b1BreakthroughFixture(t))

	b1TurnTrain(t, store, storyID, "cmd-b2-seclusion", TurnCultivationInput{Days: 30})
	activities := b2StopActivities(t, store, storyID)
	var paused CultivationActivity
	for _, activity := range activities {
		paused = activity
	}

	identity := b1FailingCycle(t)
	b1TurnCultivate(t, store, storyID, identity, TurnCultivationInput{
		AttemptBreakthrough: string(cultivation.BreakthroughFoundation),
	})

	after := b2StopActivities(t, store, storyID)
	kept, ok := after[paused.ID]
	if !ok {
		t.Fatalf("the interrupted seclusion must survive the attempt: %#v", after)
	}
	if kept.Activity.Status != storytime.ActivityPaused {
		t.Fatalf("the seclusion is still owed after the attempt: %#v", kept.Activity)
	}
	if got := int64(kept.Activity.CompletedEffective); got != 0 {
		t.Fatalf("the attempt's days are not training progress: got=%d", got)
	}
}

// TestB2ACommittedEventStageStopsTheInterval is the second source of stop points.
//
// §7 step 3 stops at the node that really affects the calculation or the
// player's decision. A stage of a public event the character already took part in
// is such a node, and its instant is committed: the event definition fixes it
// against the branch clock. A thirty-day seclusion that crosses the six-hour
// stage must therefore settle six hours and pause, not run the month through.
func TestB2ACommittedEventStageStopsTheInterval(t *testing.T) {
	store, storyID := b2StopStoryAt(t, nil)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionObserve),
	})

	clock := b1TurnTrain(t, store, storyID, "cmd-b2-stage", TurnCultivationInput{Days: 30})
	const rushStage = int64(6 * 60 * 60)
	if clock.Seconds != rushStage {
		t.Fatalf("the interval must stop at the stage's committed instant: clock=%d want=%d", clock.Seconds, rushStage)
	}
	activities := b2StopActivities(t, store, storyID)
	var paused CultivationActivity
	found := false
	for _, activity := range activities {
		if activity.Activity.Status == storytime.ActivityPaused {
			paused, found = activity, true
		}
	}
	if !found {
		t.Fatalf("the rest of the month must be saved as a paused intent: %#v", activities)
	}
	if got := int64(paused.Activity.RequiredEffective); got != 30*24*60*60 {
		t.Fatalf("the target is still the month the player named: got=%d", got)
	}
	if got := int64(paused.Activity.CompletedEffective); got != rushStage {
		t.Fatalf("only the part before the stage was trained: got=%d want=%d", got, rushStage)
	}
}

// TestB2ACallerCannotResolveACommittedStage is §B1's "不能依赖客户端自报
// nodes/resolved而跳过事项".
//
// The interval entry point still accepts a node list, because a caller may know
// about a decision the backend does not model. It may not use that list to
// resolve a stage the backend derived from committed facts: the stage really
// arrived, and no request may train past it.
func TestB2ACallerCannotResolveACommittedStage(t *testing.T) {
	store, storyID := b2StopStoryAt(t, nil)
	publicEventTestApply(t, store, storyID, PublicEventRequest{
		Action:              PublicEventActionJoin,
		ParticipationAction: string(publiquevent.ActionObserve),
	})

	stageID := cultivationStopNodePrefix + publiquevent.PublicEventIDTaiyueTide + ":taiyue.rush"
	result := cultivationTestApply(t, store, storyID, CultivationRequest{
		Interval: CultivationInterval{Value: 30, Unit: CultivationUnitDay},
		Nodes: []storytime.Node{
			{ID: stageID, At: storytime.Instant(6 * 60 * 60), Kind: storytime.ResourceNode, Resolved: true},
		},
		CommandID: "cmd-b2-resolved", CommandHash: "h-b2-resolved",
	})

	const rushStage = int64(6 * 60 * 60)
	if result.Elapsed != rushStage {
		t.Fatalf("a caller-supplied resolved node must not skip a committed stage: elapsed=%d want=%d", result.Elapsed, rushStage)
	}
	if result.Completed {
		t.Fatalf("stopping at a committed stage is not completion: %#v", result)
	}
}
