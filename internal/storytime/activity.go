package storytime

import (
	"fmt"
	"sort"
	"strings"
)

type ActivityMode string

const (
	FixedEndpoint     ActivityMode = "fixed_endpoint"
	EffectiveDuration ActivityMode = "effective_duration"
)

type ActivityStatus string

const (
	ActivityRunning          ActivityStatus = "running"
	ActivityPaused           ActivityStatus = "paused"
	ActivityAwaitingDecision ActivityStatus = "awaiting_decision"
	ActivityCompleted        ActivityStatus = "completed"
	ActivityExpired          ActivityStatus = "expired"
	ActivityInvalid          ActivityStatus = "invalid"
)

type Activity struct {
	ID                 string         `json:"id"`
	StoryID            string         `json:"story_id"`
	BranchID           string         `json:"branch_id"`
	SourceID           string         `json:"source_id"`
	Mode               ActivityMode   `json:"mode"`
	Status             ActivityStatus `json:"status"`
	StartedAt          Instant        `json:"started_at_seconds"`
	LastSettledAt      Instant        `json:"last_settled_at_seconds"`
	FixedEndAt         Instant        `json:"fixed_end_at_seconds,omitempty"`
	RequiredEffective  Duration       `json:"required_effective_seconds,omitempty"`
	CompletedEffective Duration       `json:"completed_effective_seconds"`
	// PendingNodeIDs make paused snapshots self-describing. They identify the
	// registered boundaries that must be handled before explicit resumption.
	PendingNodeIDs []string `json:"pending_node_ids,omitempty"`
}

func (a Activity) Validate() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.StoryID) == "" || strings.TrimSpace(a.BranchID) == "" || strings.TrimSpace(a.SourceID) == "" {
		return fmt.Errorf("activity requires stable id, story, branch, and source identities")
	}
	if a.StartedAt < 0 || a.LastSettledAt < 0 || a.FixedEndAt < 0 || a.RequiredEffective < 0 || a.CompletedEffective < 0 {
		return ErrNegativeTime
	}
	if a.LastSettledAt < a.StartedAt {
		return fmt.Errorf("activity last-settled time precedes its start")
	}
	switch a.Mode {
	case FixedEndpoint:
		if a.FixedEndAt < a.StartedAt || a.RequiredEffective != 0 || a.CompletedEffective != 0 {
			return fmt.Errorf("fixed-endpoint activity requires one immutable endpoint and no effective-duration counters")
		}
	case EffectiveDuration:
		if a.RequiredEffective <= 0 || a.CompletedEffective > a.RequiredEffective || a.FixedEndAt != 0 {
			return fmt.Errorf("effective-duration activity requires a positive target, valid progress, and no fixed endpoint")
		}
	default:
		return fmt.Errorf("unsupported activity mode %q", a.Mode)
	}
	switch a.Status {
	case ActivityRunning, ActivityPaused, ActivityAwaitingDecision, ActivityCompleted, ActivityExpired:
	case ActivityInvalid:
		return fmt.Errorf("activity snapshot is explicitly invalid")
	default:
		return fmt.Errorf("unsupported activity status %q", a.Status)
	}
	seenPending := make(map[string]struct{}, len(a.PendingNodeIDs))
	for _, id := range a.PendingNodeIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("activity %s has an empty pending node id", a.ID)
		}
		if _, exists := seenPending[id]; exists {
			return fmt.Errorf("activity %s repeats pending node id %q", a.ID, id)
		}
		seenPending[id] = struct{}{}
	}
	switch a.Status {
	case ActivityPaused, ActivityAwaitingDecision:
		if len(a.PendingNodeIDs) == 0 {
			return fmt.Errorf("activity %s is %q without a pending node", a.ID, a.Status)
		}
	case ActivityRunning, ActivityCompleted, ActivityExpired:
		if len(a.PendingNodeIDs) != 0 {
			return fmt.Errorf("activity %s is %q with unresolved pending nodes", a.ID, a.Status)
		}
	}
	if a.Mode == EffectiveDuration {
		elapsed := Duration(a.LastSettledAt - a.StartedAt)
		if a.CompletedEffective > elapsed {
			return fmt.Errorf("effective progress %d exceeds real elapsed duration %d", a.CompletedEffective, elapsed)
		}
		switch a.Status {
		case ActivityRunning:
			if a.CompletedEffective == a.RequiredEffective {
				return fmt.Errorf("activity %s reached its effective target but is still running", a.ID)
			}
		case ActivityPaused, ActivityAwaitingDecision:
			// A pending node may coincide with the exact effective target. The
			// caller must handle it before ResumeActivity closes the activity.
		case ActivityCompleted:
			if a.CompletedEffective != a.RequiredEffective {
				return fmt.Errorf("completed activity %s has not reached its effective target", a.ID)
			}
		case ActivityExpired:
			return fmt.Errorf("effective-duration activity %s cannot expire without a fixed endpoint", a.ID)
		}
	} else {
		switch a.Status {
		case ActivityRunning:
			if a.LastSettledAt >= a.FixedEndAt {
				return fmt.Errorf("activity %s is running at or after its fixed endpoint", a.ID)
			}
		case ActivityPaused, ActivityAwaitingDecision:
			if a.LastSettledAt > a.FixedEndAt {
				return fmt.Errorf("activity %s is waiting after its fixed endpoint", a.ID)
			}
		case ActivityCompleted:
			if a.LastSettledAt != a.FixedEndAt {
				return fmt.Errorf("completed fixed activity %s must be settled exactly at its endpoint", a.ID)
			}
		case ActivityExpired:
			if a.LastSettledAt <= a.FixedEndAt {
				return fmt.Errorf("expired fixed activity %s must be settled after its endpoint", a.ID)
			}
		}
	}
	return nil
}

type NodeKind string

const (
	CalculationNode NodeKind = "calculation"
	ResourceNode    NodeKind = "resource_change"
	DecisionNode    NodeKind = "player_decision"
)

type Node struct {
	ID   string   `json:"id"`
	At   Instant  `json:"at_seconds"`
	Kind NodeKind `json:"kind"`
	// RequiresResponse identifies a decision node's domain requirement; it stays
	// true after that decision is resolved. Resolved means the caller has already
	// handled this node and it must not block or cut the same activity again.
	RequiresResponse bool `json:"requires_response"`
	Resolved         bool `json:"resolved"`
	// PublicDescription is already approved by the caller. Internal node data
	// remains in the core result and must not be projected to a player directly.
	PublicDescription string `json:"public_description,omitempty"`
}

func (n Node) validate() error {
	if strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("node requires an id")
	}
	if n.At < 0 {
		return ErrNegativeTime
	}
	switch n.Kind {
	case CalculationNode, ResourceNode, DecisionNode:
	default:
		return fmt.Errorf("node %s has unsupported kind %q", n.ID, n.Kind)
	}
	if n.Kind == DecisionNode && !n.RequiresResponse {
		return fmt.Errorf("decision node %s must require a response", n.ID)
	}
	if n.Kind == CalculationNode && n.RequiresResponse {
		return fmt.Errorf("calculation node %s cannot require a player response", n.ID)
	}
	return nil
}

type Segment struct {
	From    Instant  `json:"from_seconds"`
	To      Instant  `json:"to_seconds"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

type Plan struct {
	Candidate          Activity       `json:"candidate"`
	Status             ActivityStatus `json:"status"`
	Reason             string         `json:"reason"`
	WorldElapsed       Duration       `json:"world_elapsed_seconds"`
	EffectiveCompleted Duration       `json:"effective_completed_seconds"`
	EffectiveRemaining Duration       `json:"effective_remaining_seconds"`
	Segments           []Segment      `json:"segments"`
	PendingNodes       []Node         `json:"pending_nodes,omitempty"`
	OverdueNodes       []Node         `json:"overdue_nodes,omitempty"`
}

// ResumeActivity requires a fresh official world time and an explicit caller
// decision. Paused effective work does not earn the interval before this call.
func ResumeActivity(a Activity, current Instant) (Activity, error) {
	if err := a.Validate(); err != nil {
		return Activity{}, err
	}
	if current < 0 {
		return Activity{}, ErrNegativeTime
	}
	if current < a.LastSettledAt {
		return Activity{}, ErrTimeReversal
	}
	if a.Status != ActivityPaused && a.Status != ActivityAwaitingDecision {
		return Activity{}, fmt.Errorf("activity %s is not paused or awaiting a decision", a.ID)
	}
	a.LastSettledAt = current
	a.PendingNodeIDs = nil
	if a.Mode == FixedEndpoint && current >= a.FixedEndAt {
		if current == a.FixedEndAt {
			a.Status = ActivityCompleted
		} else {
			a.Status = ActivityExpired
		}
		return a, nil
	}
	if a.Mode == EffectiveDuration && a.CompletedEffective == a.RequiredEffective {
		a.Status = ActivityCompleted
		return a, nil
	}
	a.Status = ActivityRunning
	return a, nil
}

// PlanActivity creates a deterministic, uncommitted plan to the activity goal
// or the nearest registered boundary. `current` must come from the caller's
// latest canonical story snapshot, never the host clock. It must equal
// LastSettledAt for active work; otherwise domain reconciliation is required.
// The sole exception is a fixed endpoint already passed in official time, which
// may be marked expired without moving the candidate time backwards.
func PlanActivity(a Activity, current Instant, nodes []Node) (Plan, error) {
	if err := a.Validate(); err != nil {
		return Plan{}, err
	}
	if current < 0 {
		return Plan{}, ErrNegativeTime
	}
	if current < a.LastSettledAt {
		return Plan{}, ErrTimeReversal
	}
	if a.Status != ActivityRunning {
		return Plan{}, fmt.Errorf("activity %s must be explicitly resumed before planning", a.ID)
	}
	if current > a.LastSettledAt && !(a.Mode == FixedEndpoint && current > a.FixedEndAt) {
		return Plan{}, ErrStaleActivitySnapshot
	}
	validated := make([]Node, len(nodes))
	copy(validated, nodes)
	seenNodeIDs := make(map[string]struct{}, len(validated))
	for _, n := range validated {
		if err := n.validate(); err != nil {
			return Plan{}, err
		}
		if _, exists := seenNodeIDs[n.ID]; exists {
			return Plan{}, fmt.Errorf("duplicate node id %q", n.ID)
		}
		seenNodeIDs[n.ID] = struct{}{}
	}
	sort.SliceStable(validated, func(i, j int) bool {
		if validated[i].At != validated[j].At {
			return validated[i].At < validated[j].At
		}
		if validated[i].ID != validated[j].ID {
			return validated[i].ID < validated[j].ID
		}
		return validated[i].Kind < validated[j].Kind
	})

	plan := Plan{Candidate: a, Status: ActivityRunning, Reason: "target reached", EffectiveCompleted: a.CompletedEffective}
	start := a.LastSettledAt
	for _, n := range validated {
		pastSettledInterval := n.At < start
		pastExpiredEndpoint := a.Mode == FixedEndpoint && current > a.FixedEndAt && n.At <= a.FixedEndAt
		if pastSettledInterval || pastExpiredEndpoint {
			plan.OverdueNodes = append(plan.OverdueNodes, n)
		}
	}
	var overdueBlockers []Node
	for _, n := range plan.OverdueNodes {
		if !n.Resolved && (n.Kind == ResourceNode || (n.Kind == DecisionNode && n.RequiresResponse)) {
			overdueBlockers = append(overdueBlockers, n)
		}
	}
	if len(overdueBlockers) != 0 {
		if a.Mode == FixedEndpoint && current >= a.FixedEndAt {
			return Plan{}, ErrActivityReconciliation
		}
		plan.Status, plan.Reason = ActivityPaused, "unresolved overdue boundary requires reconciliation"
		for _, n := range overdueBlockers {
			if n.Kind == DecisionNode && n.RequiresResponse {
				plan.Status, plan.Reason = ActivityAwaitingDecision, "unresolved overdue decision requires reconciliation"
				break
			}
		}
		plan.PendingNodes = append(plan.PendingNodes, overdueBlockers...)
		plan.Candidate.PendingNodeIDs = nodeIDs(overdueBlockers)
		plan.Candidate.LastSettledAt = current
		plan.Candidate.Status = plan.Status
		plan.EffectiveRemaining = remaining(a)
		return plan, nil
	}
	end, err := activityTarget(a)
	if err != nil {
		return Plan{}, err
	}
	if end <= start {
		plan.Candidate.LastSettledAt = current
		if a.Mode == EffectiveDuration {
			plan.Status, plan.Reason = ActivityCompleted, "effective target already reached"
		} else if end == current {
			plan.Status, plan.Reason = ActivityCompleted, "fixed endpoint reached"
		} else if end < current {
			plan.Status, plan.Reason = ActivityExpired, "fixed endpoint already passed"
		}
		plan.Candidate.Status = plan.Status
		plan.EffectiveRemaining = remaining(a)
		return plan, nil
	}
	cursor := start
	for i := 0; i < len(validated); {
		n := validated[i]
		if n.At > end {
			break
		}
		j := i + 1
		for j < len(validated) && validated[j].At == n.At {
			j++
		}
		group := validated[i:j]
		if n.At < cursor {
			i = j
			continue
		}
		ids := make([]string, 0, len(group))
		blocking := make([]Node, 0)
		resourceChange := false
		for _, item := range group {
			ids = append(ids, item.ID)
			if item.RequiresResponse && !item.Resolved {
				blocking = append(blocking, item)
			}
			if item.Kind == ResourceNode && !item.Resolved {
				resourceChange = true
			}
		}
		if n.At > cursor {
			plan.Segments = append(plan.Segments, Segment{From: cursor, To: n.At, NodeIDs: ids})
			addProgress(&plan, a.Mode, Duration(n.At-cursor))
		}
		if len(blocking) != 0 {
			plan.Status, plan.Reason = ActivityAwaitingDecision, "unresolved decision node"
			plan.PendingNodes = append(plan.PendingNodes, group...)
			plan.Candidate.PendingNodeIDs = pendingBoundaryIDs(group)
			cursor = n.At
			break
		}
		if resourceChange {
			plan.Status, plan.Reason = ActivityPaused, "resource or activity condition changed"
			plan.PendingNodes = append(plan.PendingNodes, group...)
			plan.Candidate.PendingNodeIDs = pendingBoundaryIDs(group)
			cursor = n.At
			break
		}
		cursor = n.At
		i = j
	}
	if plan.Status == ActivityRunning && cursor < end {
		plan.Segments = append(plan.Segments, Segment{From: cursor, To: end})
		addProgress(&plan, a.Mode, Duration(end-cursor))
		cursor = end
	}
	plan.WorldElapsed = Duration(cursor - start)
	plan.Candidate.LastSettledAt = cursor
	if current > cursor && (plan.Status == ActivityRunning || plan.Status == ActivityCompleted) {
		// Preserve the caller's current formal time after an endpoint was crossed;
		// the activity itself receives no extra time beyond its goal.
		plan.Candidate.LastSettledAt = current
	}
	plan.Candidate.Status = plan.Status
	if a.Mode == EffectiveDuration {
		plan.EffectiveRemaining = a.RequiredEffective - plan.EffectiveCompleted
		plan.Candidate.CompletedEffective = plan.EffectiveCompleted
		if plan.EffectiveRemaining == 0 && plan.Status == ActivityRunning {
			plan.Status, plan.Candidate.Status, plan.Reason = ActivityCompleted, ActivityCompleted, "effective target reached"
		}
	}
	if plan.Status == ActivityRunning && a.Mode == FixedEndpoint {
		if current > end {
			plan.Status, plan.Candidate.Status, plan.Reason = ActivityExpired, ActivityExpired, "fixed endpoint already passed"
		} else {
			plan.Status, plan.Candidate.Status = ActivityCompleted, ActivityCompleted
		}
	}
	return plan, nil
}

func activityTarget(a Activity) (Instant, error) {
	if a.Mode == FixedEndpoint {
		return a.FixedEndAt, nil
	}
	left := a.RequiredEffective - a.CompletedEffective
	return a.LastSettledAt.Add(left)
}

func addProgress(plan *Plan, mode ActivityMode, elapsed Duration) {
	if mode == EffectiveDuration {
		plan.EffectiveCompleted += elapsed
	}
}

func pendingBoundaryIDs(nodes []Node) []string {
	var pending []Node
	for _, node := range nodes {
		if !node.Resolved && (node.Kind == ResourceNode || node.RequiresResponse) {
			pending = append(pending, node)
		}
	}
	return nodeIDs(pending)
}

func nodeIDs(nodes []Node) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

func remaining(a Activity) Duration {
	if a.Mode == EffectiveDuration {
		return a.RequiredEffective - a.CompletedEffective
	}
	return 0
}
