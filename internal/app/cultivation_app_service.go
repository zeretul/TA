package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storytime"
)

// This file makes §五's player-named cultivation interval reachable during play.
//
// The rules already existed and were already correct: AdvanceCultivationFrom
// carries its accumulator so one long interval equals several short ones, the
// small-stage table is entered by the rules rather than by prose, and a major
// breakthrough is never rolled without an explicit authorisation. What was
// missing was a way for the player to name an interval at all. Cultivation could
// only ride inside a story Turn, and a Turn does not move the branch's formal
// clock, so a character could train for a rule year and the story's single time
// axis would not have noticed.
//
// Three properties are deliberate here.
//
//   - The interval is named in the player's own unit and converted once. §五
//     permits the rule month and the rule year as units of account and forbids
//     treating them as a calendar, so the conversion lives next to the constants
//     that define them rather than here.
//
//   - The rules run before the prose. The advance and the time it spent commit in
//     one transaction, so the entry can never show a stage whose training time
//     was never spent, and a model failure leaves the position exactly as it was.
//
//   - Eligibility is never taken from the request. §五 reuses the character's
//     existing cultivation qualification, so the caller may only declare a
//     condition the store cannot derive, and even then only a stricter one.

// CultivationAppRequest asks for one player-named cultivation interval.
type CultivationAppRequest struct {
	StoryID  string
	BranchID string
	// ActorID names the character who trains. Empty means the story's protagonist,
	// which is what the ordinary main-story input implies.
	ActorID string
	// Value and Unit name the interval in the player's own terms. §五 permits
	// hours, days, rule months and rule years.
	Value float64
	Unit  string
	// Conditions are the real training conditions. On a continuation a nil value
	// inherits the activity's recorded conditions.
	Conditions *interactive.CultivationConditions
	// ActivityID continues an interval a real node stopped. Empty starts a new one.
	ActivityID string
	// Nodes are the registered story boundaries this interval must stop at.
	Nodes []storytime.Node
	// DeclaredEligibility lets a caller report a condition the store cannot
	// derive, such as a creator-marked seal. It can only make the finding
	// stricter, never weaker.
	DeclaredEligibility string
	// CommandID identifies one attempt. The content fingerprint is derived here
	// rather than sent, so a caller cannot accidentally reuse one identity for two
	// different intervals.
	CommandID string
}

// CultivationAppResult is one committed interval as the UI reports it.
type CultivationAppResult struct {
	StoryID    string `json:"story_id"`
	BranchID   string `json:"branch_id"`
	EventID    string `json:"event_id"`
	ActivityID string `json:"activity_id"`
	ActorID    string `json:"actor_id"`
	// ElapsedSeconds is the in-fiction time this interval really consumed, and
	// Elapsed is the natural wording §三.2 allows for it. §五 requires the
	// training result and the formal time to agree, so both are reported from the
	// one interval rather than re-derived.
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	Elapsed        string `json:"elapsed,omitempty"`
	// Days is the same interval in the rules' own unit: the number the advance
	// integrated.
	Days float64 `json:"days"`
	// RealmBefore and RealmAfter are the stage keys at each end of the interval,
	// and StagesAdvanced counts the small-stage thresholds it crossed.
	RealmBefore    string `json:"realm_before"`
	RealmAfter     string `json:"realm_after"`
	StagesAdvanced int    `json:"stages_advanced"`
	// RealmProgress is the banked position after the interval.
	RealmProgress float64 `json:"realm_progress"`
	// StoppedAtBoundary reports that the character reached the top of the
	// implemented stage table and could not advance further by training.
	StoppedAtBoundary bool `json:"stopped_at_boundary"`
	// BreakthroughRequested, BreakthroughRolled and BreakthroughSucceeded report an
	// authorised attempt. §五 forbids a time-targeted interval from attempting one
	// on its own, so these are false unless the caller authorised it.
	BreakthroughRequested bool   `json:"breakthrough_requested"`
	BreakthroughEligible  bool   `json:"breakthrough_eligible"`
	BreakthroughRolled    bool   `json:"breakthrough_rolled"`
	BreakthroughSucceeded bool   `json:"breakthrough_succeeded"`
	// BreakthroughReasons lists, in player-readable terms, the preconditions that
	// failed when an authorised attempt could not be made.
	BreakthroughReasons []string `json:"breakthrough_reasons,omitempty"`
	// Completed reports whether the interval reached its target. A false value
	// means a real node stopped it and the player must resume explicitly.
	Completed bool `json:"completed"`
	// PendingNodeIDs are the registered boundaries that stopped this interval.
	PendingNodeIDs []string `json:"pending_node_ids,omitempty"`
	// ActivityStatus is the activity's state after this interval.
	ActivityStatus string `json:"activity_status"`
	// ClockSeconds is the branch's formal instant after the interval.
	ClockSeconds int64 `json:"clock_seconds"`
	// Replayed reports that this answer came from an interval already committed
	// under the same command, so no second interval was spent.
	Replayed bool `json:"replayed"`
	// Eligibility is the safety finding the interval was admitted under.
	Eligibility string `json:"eligibility"`
}

// CultivationState is the cultivation entry's read: the branch's formal clock and
// its cultivation activities.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation: §三.3's rule for the rest entry applies here too.
type CultivationState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	ActorID  string `json:"actor_id"`
	// ClockSeconds is the branch's formal instant. A branch that has never spent
	// formal time reads as being at its own origin.
	ClockSeconds int64 `json:"clock_seconds"`
	// Anchor is the story's own display text for the origin. It is a label, never
	// parsed into the arithmetic.
	Anchor string `json:"anchor,omitempty"`
	// Realm, RealmProgress, Health, HealthMax, Mana and ManaMax are the live rules
	// position, so the entry can show what an interval would reach without
	// guessing.
	Realm         string  `json:"realm"`
	RealmProgress float64 `json:"realm_progress"`
	Health        int     `json:"health"`
	HealthMax     int     `json:"health_max"`
	Mana          int     `json:"mana"`
	ManaMax       int     `json:"mana_max"`
	// Eligibility is the derived safety finding.
	Eligibility string `json:"eligibility"`
	// Refusal is the player-facing reason ordinary cultivation is unavailable, and
	// is empty when it is available.
	Refusal string `json:"refusal,omitempty"`
	// Activities are the branch's cultivation intervals, so an interval cut short
	// at a real node can be resumed rather than restarted.
	Activities []CultivationActivityView `json:"activities,omitempty"`
}

// CultivationActivityView is one cultivation interval as the entry shows it.
type CultivationActivityView struct {
	ActivityID string `json:"activity_id"`
	ActorID    string `json:"actor_id"`
	Status     string `json:"status"`
	Required   int64  `json:"required_seconds"`
	Completed  int64  `json:"completed_seconds"`
	Remaining  int64  `json:"remaining_seconds"`
	Resumable  bool   `json:"resumable"`
	// Environment and Resource are the conditions the segment runs under, so the
	// entry can show what a remainder would be trained at.
	Environment string `json:"environment,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Eligibility string `json:"eligibility"`
}

// CultivationStateFor reads the branch's formal clock and cultivation intervals.
func (a *App) CultivationStateFor(storyID, branchID, actorID string) (CultivationState, error) {
	if a == nil {
		return CultivationState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return CultivationState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return CultivationState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	resolvedActor, allowed := playerNamedActor(actorID)
	if !allowed {
		return CultivationState{}, fmt.Errorf("%w: %s", interactive.ErrCultivationUnknownActor, strings.TrimSpace(actorID))
	}
	clock, activities, err := store.CultivationState(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return CultivationState{}, err
	}
	view := CultivationState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch, ActorID: resolvedActor,
		ClockSeconds: clock.Seconds, Anchor: clock.Anchor,
	}
	attributes, eligibility, err := store.RecoveryAttributes(strings.TrimSpace(storyID), resolvedBranch, resolvedActor, "")
	if err != nil {
		return CultivationState{}, err
	}
	view.Health, view.HealthMax = attributes.Health, attributes.HealthMax
	view.Mana, view.ManaMax = attributes.Mana, attributes.ManaMax
	view.Eligibility = string(eligibility)
	view.Refusal = interactive.DescribeRecoveryEligibilityRefusal(eligibility)
	position, err := store.CultivationPosition(strings.TrimSpace(storyID), resolvedBranch, resolvedActor)
	if err != nil {
		return CultivationState{}, err
	}
	view.Realm, view.RealmProgress = position.Realm, position.Progress
	for _, activity := range activities {
		if normalizeRecoveryActor(activity.ActorID, storyContext) != resolvedActor {
			continue
		}
		view.Activities = append(view.Activities, CultivationActivityView{
			ActivityID: activity.Activity.ID, ActorID: activity.ActorID,
			Status:   string(activity.Activity.Status),
			Required: int64(activity.Activity.RequiredEffective), Completed: int64(activity.Activity.CompletedEffective),
			Remaining:   int64(activity.Activity.RequiredEffective - activity.Activity.CompletedEffective),
			Resumable:   activity.Activity.Status == storytime.ActivityPaused || activity.Activity.Status == storytime.ActivityAwaitingDecision,
			Environment: activity.Conditions.Environment, Resource: activity.Conditions.Resource,
			Eligibility: string(activity.Eligibility),
		})
	}
	return view, nil
}

// ApplyCultivationInterval settles one player-named cultivation interval and
// commits it.
//
// The rules run first and the commit is one transaction, so the advance, the
// banked progress and the in-fiction time either all exist on the journal or none
// of them do. §五's "有效时段只计一次" is that requirement.
func (a *App) ApplyCultivationInterval(request CultivationAppRequest) (CultivationAppResult, error) {
	if a == nil {
		return CultivationAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return CultivationAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return CultivationAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	resolvedActor, allowed := playerNamedActor(request.ActorID)
	if !allowed {
		return CultivationAppResult{}, fmt.Errorf("%w: %s", interactive.ErrCultivationUnknownActor, strings.TrimSpace(request.ActorID))
	}
	declared := cultivation.RecoveryEligibility(strings.TrimSpace(request.DeclaredEligibility))
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplyCultivationInterval(strings.TrimSpace(request.StoryID), interactive.CultivationRequest{
		BranchID: resolvedBranch,
		ActorID:  resolvedActor,
		// §五 permits the rule month and the rule year as units of account, so the
		// interval is converted from the player's own unit and nothing else.
		Interval:            interactive.CultivationInterval{Value: request.Value, Unit: request.Unit},
		Conditions:          request.Conditions,
		ActivityID:          strings.TrimSpace(request.ActivityID),
		Nodes:               request.Nodes,
		DeclaredEligibility: declared,
		CommandID:           commandID,
		// The fingerprint is derived from what the interval actually is and
		// resolved against the branch the store will really write to, so an empty
		// branch and an explicit current branch are the same command rather than a
		// conflict.
		CommandHash: cultivationCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return CultivationAppResult{}, err
	}
	return CultivationAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, ActivityID: result.ActivityID, ActorID: result.ActorID,
		ElapsedSeconds: result.Elapsed, Elapsed: result.ElapsedDescription, Days: result.Days,
		RealmBefore: result.Advance.RealmBefore, RealmAfter: result.Advance.RealmAfter,
		StagesAdvanced:    result.Advance.StagesAdvanced,
		RealmProgress:     result.Advance.RealmProgressAfter,
		StoppedAtBoundary: result.Advance.StoppedAtBoundary,
		// A time-targeted interval never attempts a major breakthrough on its own,
		// so these are false unless the caller authorised the attempt.
		BreakthroughRequested: result.Advance.BreakthroughRequested,
		BreakthroughEligible:  result.Advance.BreakthroughEligible,
		BreakthroughRolled:    result.Advance.BreakthroughRolled,
		BreakthroughSucceeded: result.Advance.BreakthroughSucceeded,
		BreakthroughReasons:   result.Advance.BreakthroughReasons,
		Completed:             result.Completed,
		PendingNodeIDs:        result.PendingNodeIDs,
		ActivityStatus:        string(result.Activity.Activity.Status),
		ClockSeconds:          result.ClockAfter.Seconds,
		Replayed:              result.Replayed,
		Eligibility:           string(result.Eligibility),
	}, nil
}

// cultivationCommandHash fingerprints the content behind a cultivation command ID.
//
// It covers what actually decides the interval — the branch, the character, the
// unit, the length, the conditions and the activity being continued — so reusing
// one command ID for a genuinely different interval is refused rather than
// answered with the wrong one. The branch is the resolved one, so an empty branch
// and an explicit current branch fingerprint the same.
func cultivationCommandHash(request CultivationAppRequest, branchID, commandID string) string {
	if commandID == "" {
		return ""
	}
	parts := []string{
		"denova.cultivation_interval",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		strings.TrimSpace(request.ActorID),
		// The unit is fingerprinted as written rather than as seconds, so the same
		// interval named two different ways is two commands. §五 keeps the two
		// intents — a fixed endpoint and an accumulated duration — distinct, and
		// this keeps the identity honest about which one was asked for.
		"interval\x00" + fmt.Sprintf("%v\x00%s", request.Value, strings.TrimSpace(request.Unit)),
		"activity\x00" + strings.TrimSpace(request.ActivityID),
		"declared\x00" + strings.TrimSpace(request.DeclaredEligibility),
	}
	if request.Conditions != nil {
		parts = append(parts,
			"environment\x00"+strings.TrimSpace(request.Conditions.Environment),
			"resource\x00"+strings.TrimSpace(request.Conditions.Resource),
			"breakthrough\x00"+strings.TrimSpace(request.Conditions.AttemptBreakthrough),
		)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
