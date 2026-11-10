package storytime

import (
	"encoding/json"
	"errors"
	"testing"
)

func testActivity(mode ActivityMode, start Instant, end Instant, required, done Duration) Activity {
	return Activity{ID: "activity-1", StoryID: "story-1", BranchID: "branch-1", SourceID: "command-1", Mode: mode, Status: ActivityRunning, StartedAt: start, LastSettledAt: start, FixedEndAt: end, RequiredEffective: required, CompletedEffective: done}
}

func TestFixedAndEffectiveGoalsDifferAcrossPause(t *testing.T) {
	thirtyDays, _ := CalculationDays(30)
	tenDays, _ := CalculationDays(10)
	twoDays, _ := CalculationDays(2)
	fixed := testActivity(FixedEndpoint, 0, Instant(thirtyDays), 0, 0)
	first, err := PlanActivity(fixed, 0, []Node{{ID: "decision", At: Instant(tenDays), Kind: DecisionNode, RequiresResponse: true}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != ActivityAwaitingDecision || first.WorldElapsed != tenDays {
		t.Fatalf("fixed pause plan = %+v", first)
	}
	fixed, err = ResumeActivity(first.Candidate, Instant(tenDays+twoDays))
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanActivity(fixed, fixed.LastSettledAt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Candidate.LastSettledAt != Instant(thirtyDays) || second.WorldElapsed != 18*CalculationDay || second.Status != ActivityCompleted {
		t.Fatalf("fixed endpoint moved or counted pause as activity: %+v", second)
	}

	effective := testActivity(EffectiveDuration, 0, 0, thirtyDays, 0)
	first, err = PlanActivity(effective, 0, []Node{{ID: "decision", At: Instant(tenDays), Kind: DecisionNode, RequiresResponse: true}})
	if err != nil {
		t.Fatal(err)
	}
	if first.EffectiveCompleted != tenDays || first.EffectiveRemaining != 20*CalculationDay {
		t.Fatalf("effective pause plan = %+v", first)
	}
	effective, err = ResumeActivity(first.Candidate, Instant(tenDays+twoDays))
	if err != nil {
		t.Fatal(err)
	}
	second, err = PlanActivity(effective, effective.LastSettledAt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.EffectiveCompleted != thirtyDays || second.EffectiveRemaining != 0 || second.Candidate.LastSettledAt != Instant(32*CalculationDay) {
		t.Fatalf("effective target did not retain only remaining activity time: %+v", second)
	}
}

func TestSegmentingDoesNotChangeCumulativeResult(t *testing.T) {
	target, _ := CalculationDays(90)
	oneShot := testActivity(EffectiveDuration, 0, 0, target, 0)
	whole, err := PlanActivity(oneShot, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []Node
	for _, day := range []int64{15, 30, 45, 60, 75} {
		at, _ := CalculationDays(day)
		nodes = append(nodes, Node{ID: string(rune('a' + day/15)), At: Instant(at), Kind: CalculationNode})
	}
	segmented, err := PlanActivity(testActivity(EffectiveDuration, 0, 0, target, 0), 0, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if whole.EffectiveCompleted != target || segmented.EffectiveCompleted != target || segmented.WorldElapsed != target || len(segmented.Segments) != 6 {
		t.Fatalf("whole=%+v segmented=%+v target=%d", whole, segmented, target)
	}
}

func TestNodesBoundariesOrderingOverdueAndCurrentBlock(t *testing.T) {
	twentyDays, _ := CalculationDays(20)
	tenDays, _ := CalculationDays(10)
	activity := testActivity(EffectiveDuration, 0, 0, twentyDays, 0)
	nodes := []Node{
		{ID: "late-secret", At: Instant(tenDays), Kind: CalculationNode},
		{ID: "resource", At: Instant(tenDays), Kind: ResourceNode},
	}
	plan, err := PlanActivity(activity, 0, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != ActivityPaused || plan.WorldElapsed != tenDays || plan.EffectiveCompleted != tenDays {
		t.Fatalf("resource boundary not respected: %+v", plan)
	}
	if len(plan.PendingNodes) != 2 || plan.PendingNodes[0].ID != "late-secret" || plan.PendingNodes[1].ID != "resource" {
		t.Fatalf("same-time nodes not stable/complete: %+v", plan.PendingNodes)
	}
	overdueActivity := activity
	overdueActivity.LastSettledAt = 2
	overdue, err := PlanActivity(overdueActivity, 2, []Node{{ID: "old", At: 1, Kind: CalculationNode}})
	if err != nil {
		t.Fatal(err)
	}
	if len(overdue.OverdueNodes) != 1 || overdue.OverdueNodes[0].ID != "old" {
		t.Fatalf("overdue nodes = %+v", overdue.OverdueNodes)
	}

	blocked, err := PlanActivity(activity, 0, []Node{
		{ID: "now-decision", At: 0, Kind: DecisionNode, RequiresResponse: true},
		{ID: "now-calculation", At: 0, Kind: CalculationNode},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.WorldElapsed != 0 || blocked.Status != ActivityAwaitingDecision {
		t.Fatalf("current-time block was bypassed: %+v", blocked)
	}
	if len(blocked.PendingNodes) != 2 || blocked.PendingNodes[0].ID != "now-calculation" || blocked.PendingNodes[1].ID != "now-decision" {
		t.Fatalf("same-time pending nodes were not retained in stable order: %+v", blocked.PendingNodes)
	}

	ordinary, err := PlanActivity(activity, 0, []Node{{ID: "ordinary", At: Instant(tenDays), Kind: CalculationNode}})
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Status != ActivityCompleted || ordinary.EffectiveCompleted != twentyDays || len(ordinary.Segments) != 2 {
		t.Fatalf("ordinary calculation node forced a pause: %+v", ordinary)
	}
}

func TestExpiredFixedActivityDoesNotRewindAndInvalidStatesFail(t *testing.T) {
	activity := testActivity(FixedEndpoint, 0, 10, 0, 0)
	plan, err := PlanActivity(activity, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != ActivityExpired || plan.Candidate.LastSettledAt != 20 || plan.WorldElapsed != 10 {
		t.Fatalf("expired activity rewound or gained excess time: %+v", plan)
	}
	activity.Status = ActivityStatus("mystery")
	if _, err := PlanActivity(activity, 0, nil); err == nil {
		t.Fatal("unknown activity state accepted")
	}
	activity = testActivity(EffectiveDuration, 0, 0, 10, 0)
	if _, err := PlanActivity(activity, 0, []Node{{ID: "same", At: 1, Kind: CalculationNode}, {ID: "same", At: 2, Kind: CalculationNode}}); err == nil {
		t.Fatal("duplicate node identity accepted")
	}
}

func TestReviewR01ResolvedBoundariesResumeAndContinue(t *testing.T) {
	target := Duration(100)
	activity := testActivity(EffectiveDuration, 0, 0, target, 0)
	resource := Node{ID: "resource", At: 10, Kind: ResourceNode}
	ordinary := Node{ID: "ordinary", At: 10, Kind: CalculationNode}
	decision := Node{ID: "decision", At: 50, Kind: DecisionNode, RequiresResponse: true}

	first, err := PlanActivity(activity, 0, []Node{resource, ordinary, decision})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != ActivityPaused || first.Candidate.LastSettledAt != 10 || first.EffectiveCompleted != 10 {
		t.Fatalf("first resource boundary = %+v", first)
	}
	activity, err = ResumeActivity(first.Candidate, 10)
	if err != nil {
		t.Fatal(err)
	}
	resource.Resolved = true
	second, err := PlanActivity(activity, 10, []Node{resource, ordinary, decision})
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != ActivityAwaitingDecision || second.Candidate.LastSettledAt != 50 || second.EffectiveCompleted != 50 {
		t.Fatalf("resolved resource should allow continuation to the next decision: %+v", second)
	}
	activity, err = ResumeActivity(second.Candidate, 50)
	if err != nil {
		t.Fatal(err)
	}
	decision.Resolved = true
	third, err := PlanActivity(activity, 50, []Node{resource, ordinary, decision})
	if err != nil {
		t.Fatal(err)
	}
	if third.Status != ActivityCompleted || third.Candidate.LastSettledAt != 100 || third.EffectiveCompleted != target {
		t.Fatalf("resolved decision should not block again: %+v", third)
	}
	if err := (Node{ID: "malformed-decision", At: 0, Kind: DecisionNode, Resolved: true}).validate(); err == nil {
		t.Fatal("decision node without its required-response classification was accepted")
	}
}

func TestReviewR02RejectsStaleActivitySnapshots(t *testing.T) {
	activity := testActivity(EffectiveDuration, 0, 0, 100, 0)
	if _, err := PlanActivity(activity, 80, []Node{{ID: "old-decision", At: 50, Kind: DecisionNode, RequiresResponse: true}}); !errors.Is(err, ErrStaleActivitySnapshot) {
		t.Fatalf("stale activity snapshot error = %v", err)
	}
	if _, err := PlanActivity(activity, 80, nil); !errors.Is(err, ErrStaleActivitySnapshot) {
		t.Fatalf("stale activity without nodes error = %v", err)
	}
	fixed := testActivity(FixedEndpoint, 0, 100, 0, 0)
	if _, err := PlanActivity(fixed, 80, nil); !errors.Is(err, ErrStaleActivitySnapshot) {
		t.Fatalf("unfinished fixed activity with stale snapshot error = %v", err)
	}
	// Fixed deadlines already crossed remain safely terminal without moving the
	// returned official time backwards.
	fixed.FixedEndAt = 10
	plan, err := PlanActivity(fixed, 20, nil)
	if err != nil || plan.Status != ActivityExpired || plan.Candidate.LastSettledAt != 20 {
		t.Fatalf("crossed fixed endpoint was not safely expired: plan=%+v err=%v", plan, err)
	}
}

func TestReviewR03UnresolvedOverdueBoundariesRequireReconciliation(t *testing.T) {
	activity := testActivity(EffectiveDuration, 0, 0, 100, 20)
	activity.LastSettledAt = 20
	cases := []struct {
		name   string
		node   Node
		status ActivityStatus
	}{
		{name: "resource", node: Node{ID: "old-resource", At: 10, Kind: ResourceNode}, status: ActivityPaused},
		{name: "decision", node: Node{ID: "old-decision", At: 10, Kind: DecisionNode, RequiresResponse: true}, status: ActivityAwaitingDecision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanActivity(activity, 20, []Node{tc.node})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Status != tc.status || plan.WorldElapsed != 0 || plan.EffectiveCompleted != 20 || plan.Candidate.LastSettledAt != 20 || plan.EffectiveRemaining != 80 {
				t.Fatalf("unresolved overdue boundary allowed new gains: %+v", plan)
			}
			if len(plan.OverdueNodes) != 1 || len(plan.PendingNodes) != 1 {
				t.Fatalf("overdue boundary not retained for reconciliation: %+v", plan)
			}
		})
	}

	for _, node := range []Node{
		{ID: "resolved-resource", At: 10, Kind: ResourceNode, Resolved: true},
		{ID: "resolved-decision", At: 10, Kind: DecisionNode, RequiresResponse: true, Resolved: true},
		{ID: "ordinary-history", At: 10, Kind: CalculationNode},
	} {
		plan, err := PlanActivity(activity, 20, []Node{node})
		if err != nil {
			t.Fatalf("%s: %v", node.ID, err)
		}
		if plan.Status != ActivityCompleted || plan.EffectiveCompleted != 100 {
			t.Fatalf("processed/history node %s should not hold or repeat old work: %+v", node.ID, plan)
		}
	}
}

func TestReviewR04RejectsImpossibleProgressAndContradictoryTerminalStates(t *testing.T) {
	impossible := testActivity(EffectiveDuration, 0, 0, 100, 80)
	if err := impossible.Validate(); err == nil {
		t.Fatal("effective progress exceeded real elapsed time but snapshot was accepted")
	}
	legitimatePause := testActivity(EffectiveDuration, 0, 0, 100, 30)
	legitimatePause.LastSettledAt = 80
	legitimatePause.Status = ActivityPaused
	legitimatePause.PendingNodeIDs = []string{"pause-node"}
	if err := legitimatePause.Validate(); err != nil {
		t.Fatalf("pause gap must remain valid when effective work is below elapsed time: %v", err)
	}

	completeButShort := testActivity(EffectiveDuration, 0, 0, 100, 50)
	completeButShort.Status = ActivityCompleted
	completeButShort.LastSettledAt = 50
	if err := completeButShort.Validate(); err == nil {
		t.Fatal("completed effective activity below its target was accepted")
	}
	completeButOverdue := testActivity(FixedEndpoint, 0, 10, 0, 0)
	completeButOverdue.Status = ActivityCompleted
	completeButOverdue.LastSettledAt = 11
	if err := completeButOverdue.Validate(); err == nil {
		t.Fatal("fixed activity marked completed after its missed endpoint was accepted")
	}
}

func TestReviewB01ExpiredFixedEndpointRequiresHistoricalNodeReconciliation(t *testing.T) {
	activity := testActivity(FixedEndpoint, 0, 10, 0, 0)
	activity.LastSettledAt = 0
	for _, node := range []Node{
		{ID: "expired-resource", At: 5, Kind: ResourceNode},
		{ID: "expired-decision", At: 5, Kind: DecisionNode, RequiresResponse: true},
	} {
		_, err := PlanActivity(activity, 20, []Node{node})
		if !errors.Is(err, ErrActivityReconciliation) {
			t.Fatalf("unresolved historical node %s must require reconciliation, got %v", node.ID, err)
		}
	}

	for _, node := range []Node{
		{ID: "handled-resource", At: 5, Kind: ResourceNode, Resolved: true},
		{ID: "handled-decision", At: 5, Kind: DecisionNode, RequiresResponse: true, Resolved: true},
	} {
		plan, err := PlanActivity(activity, 20, []Node{node})
		if err != nil {
			t.Fatalf("processed expired node %s blocked expiry: %v", node.ID, err)
		}
		if plan.Status != ActivityExpired || plan.Candidate.LastSettledAt != 20 || plan.WorldElapsed != 10 {
			t.Fatalf("processed node %s produced an unsafe expired candidate: %+v", node.ID, plan)
		}
		if err := plan.Candidate.Validate(); err != nil {
			t.Fatalf("processed expired candidate is invalid: %v", err)
		}
	}
}

func TestReviewB02GoalBoundaryCandidatesValidateAndResume(t *testing.T) {
	target := Duration(100)
	cases := []struct {
		name             string
		mode             ActivityMode
		kind             NodeKind
		requiresResponse bool
		waiting          ActivityStatus
	}{
		{name: "fixed-resource", mode: FixedEndpoint, kind: ResourceNode, waiting: ActivityPaused},
		{name: "fixed-decision", mode: FixedEndpoint, kind: DecisionNode, requiresResponse: true, waiting: ActivityAwaitingDecision},
		{name: "effective-resource", mode: EffectiveDuration, kind: ResourceNode, waiting: ActivityPaused},
		{name: "effective-decision", mode: EffectiveDuration, kind: DecisionNode, requiresResponse: true, waiting: ActivityAwaitingDecision},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			activity := testActivity(tc.mode, 0, 100, target, 0)
			if tc.mode == FixedEndpoint {
				activity = testActivity(FixedEndpoint, 0, 100, 0, 0)
			} else {
				activity = testActivity(EffectiveDuration, 0, 0, target, 0)
			}
			nodes := []Node{
				{ID: "ordinary-at-goal", At: 100, Kind: CalculationNode},
				{ID: "blocking-at-goal", At: 100, Kind: tc.kind, RequiresResponse: tc.requiresResponse},
			}
			plan, err := PlanActivity(activity, 0, nodes)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Status != tc.waiting || plan.Candidate.LastSettledAt != 100 || plan.WorldElapsed != 100 {
				t.Fatalf("goal-boundary plan = %+v", plan)
			}
			if tc.mode == EffectiveDuration && (plan.Candidate.CompletedEffective != target || plan.EffectiveRemaining != 0) {
				t.Fatalf("effective goal-boundary progress = %+v", plan)
			}
			if len(plan.PendingNodes) != 2 {
				t.Fatalf("same-time ordinary and blocking nodes were not retained: %+v", plan.PendingNodes)
			}
			if len(plan.Candidate.PendingNodeIDs) != 1 || plan.Candidate.PendingNodeIDs[0] != "blocking-at-goal" {
				t.Fatalf("candidate did not retain the blocking node identity: %+v", plan.Candidate.PendingNodeIDs)
			}
			if err := plan.Candidate.Validate(); err != nil {
				t.Fatalf("returned candidate does not validate: %v", err)
			}
			snapshotJSON, err := json.Marshal(plan.Candidate)
			if err != nil {
				t.Fatal(err)
			}
			var replay Activity
			if err := json.Unmarshal(snapshotJSON, &replay); err != nil {
				t.Fatal(err)
			}
			if err := replay.Validate(); err != nil {
				t.Fatalf("serialized candidate does not replay as a valid snapshot: %v", err)
			}

			// The caller handles the same-time node in its domain and replays the
			// candidate at the unchanged official time before the core closes it.
			for i := range nodes {
				if nodes[i].ID == "blocking-at-goal" {
					nodes[i].Resolved = true
				}
			}
			for _, pendingID := range replay.PendingNodeIDs {
				resolved := false
				for _, node := range nodes {
					if node.ID == pendingID && node.Resolved {
						resolved = true
						break
					}
				}
				if !resolved {
					t.Fatalf("pending boundary %s was not handled before resumption", pendingID)
				}
			}
			resumed, err := ResumeActivity(replay, 100)
			if err != nil {
				t.Fatalf("resolved goal-boundary candidate could not resume: %v", err)
			}
			if resumed.Status != ActivityCompleted || resumed.LastSettledAt != 100 || (tc.mode == EffectiveDuration && resumed.CompletedEffective != target) {
				t.Fatalf("goal-boundary resume changed completed work or time: %+v", resumed)
			}
			if err := resumed.Validate(); err != nil {
				t.Fatalf("resumed completion is invalid: %v", err)
			}
		})
	}
}
