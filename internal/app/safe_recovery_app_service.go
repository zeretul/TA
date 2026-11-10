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

// This file makes the safe rest and meditation loop reachable during play.
//
// §三 of the night-shift brief asks for one complete flow: the player names a
// real interval through the ordinary story input or a light rest entry, the
// program confirms the safety eligibility from formal state and computes the
// result, and the recovery, the elapsed time and the activity progress commit
// once along the single story journal. Three properties are deliberate here.
//
//   - The rules run before the prose, not instead of it. The interval is settled
//     and committed first; a narrative describes what was committed. A model
//     failure leaves the settled pools exactly as they were, which is what makes
//     §三.5's "模型失败可按已提交结果补叙事，不重新计算获得第二份收益"
//     structural rather than a rule this file has to remember.
//
//   - Eligibility is never taken from the request. §三.4 refuses to trust a
//     frontend or a model reporting safe=true, so the caller may only declare a
//     condition the store cannot derive, and even then it can only make the
//     answer stricter.
//
//   - The recovery owns no clock. §三.1 forbids it from advancing a time of its
//     own, so the interval it settles and the in-fiction time it spends are the
//     same number, and the player's real waiting costs nothing.

// SafeRecoveryAppRequest asks for one safe rest or meditation interval.
type SafeRecoveryAppRequest struct {
	StoryID  string
	BranchID string
	// ActorID names the character who rests. Empty means the story's protagonist,
	// which is what the ordinary main-story input implies.
	ActorID string
	// Mode is health rest or safe meditation. It is one pool per activity: §三.3
	// forbids restoring both at once for free.
	Mode string
	// DurationSeconds is the in-fiction interval the player asked to spend.
	DurationSeconds int64
	// ActivityID continues an activity a real node stopped. Empty starts a new
	// one.
	ActivityID string
	// Nodes are the registered story boundaries this interval must stop at. §三.6
	// asks the ordinary action to run straight through and then stop naturally
	// where the story really needs a choice, so the boundaries come from the story
	// rather than from a duration this layer invents.
	Nodes []storytime.Node
	// DeclaredEligibility lets a caller report a condition the store cannot
	// derive, such as a creator-marked seal. It can only make the finding
	// stricter, never weaker.
	DeclaredEligibility string
	// CommandID identifies one attempt. It is the caller's, because only the
	// caller knows that a retry is the same attempt rather than a new one. The
	// content fingerprint is derived here rather than sent, so a caller cannot
	// accidentally reuse one identity for two different intervals.
	CommandID string
}

// SafeRecoveryAppResult is one committed interval as the UI reports it.
type SafeRecoveryAppResult struct {
	StoryID    string `json:"story_id"`
	BranchID   string `json:"branch_id"`
	EventID    string `json:"event_id"`
	ActivityID string `json:"activity_id"`
	ActorID    string `json:"actor_id"`
	Mode       string `json:"mode"`
	// ElapsedSeconds is the in-fiction time this interval really consumed, and
	// Elapsed is the natural wording §三.2 allows for it.
	ElapsedSeconds int64  `json:"elapsed_seconds"`
	Elapsed        string `json:"elapsed,omitempty"`
	// PoolBefore, PoolAfter and PoolMaximum are the restored pool around the
	// interval, so the UI can show what actually moved without re-deriving it.
	PoolBefore  int `json:"pool_before"`
	PoolAfter   int `json:"pool_after"`
	PoolMaximum int `json:"pool_maximum"`
	// Completed reports whether the activity reached its target. A false value
	// means a real node stopped it and the player must resume explicitly.
	Completed bool `json:"completed"`
	// PendingNodeIDs are the registered boundaries that stopped this interval.
	PendingNodeIDs []string `json:"pending_node_ids,omitempty"`
	// ActivityStatus is the activity's state after this interval.
	ActivityStatus string `json:"activity_status"`
	// ClockSeconds is the branch's formal instant after the interval. It is the
	// single comparable moment every later activity is planned against.
	ClockSeconds int64 `json:"clock_seconds"`
	// Replayed reports that this answer came from an interval already committed
	// under the same command, so the caller knows no second recovery happened.
	Replayed bool `json:"replayed"`
	// ResidualCleared reports that this interval was also the safe regulation a
	// meridian residual injury required, so the injury is gone. §四 makes the
	// clearing the effect of the same interval rather than a second one.
	ResidualCleared bool `json:"residual_cleared"`
	// BurdenBefore, BurdenAfter and BurdenFloorAfter are the meridian burden
	// around the interval and the residual floor left afterwards. They are the
	// rules' own numbers, reported so the stage can show what the interval did to
	// the character rather than re-deriving it.
	BurdenBefore     int `json:"burden_before"`
	BurdenAfter      int `json:"burden_after"`
	BurdenFloorAfter int `json:"burden_floor_after"`
}

// SafeRecoveryState is the light rest entry's read: the branch's formal clock
// and its activities, plus what the character could currently do.
type SafeRecoveryState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	ActorID  string `json:"actor_id"`
	// ClockSeconds is the branch's formal instant. A branch that has never spent
	// formal time reads as being at its own origin.
	ClockSeconds int64 `json:"clock_seconds"`
	// Anchor is the story's own display text for the origin. It is a label, never
	// parsed into the arithmetic.
	Anchor string `json:"anchor,omitempty"`
	// Health, HealthMax, Mana and ManaMax are the live pools, so the entry can
	// show what a rest would restore without guessing.
	Health    int `json:"health"`
	HealthMax int `json:"health_max"`
	Mana      int `json:"mana"`
	ManaMax   int `json:"mana_max"`
	// Eligibility is the derived safety finding. §三.4 makes it a formal fact
	// rather than a claim, so the entry shows the real one.
	Eligibility string `json:"eligibility"`
	// Refusal is the player-facing reason ordinary recovery is unavailable, and
	// is empty when it is available.
	Refusal string `json:"refusal,omitempty"`
	// Activities are the branch's safe recovery activities, so a rest cut short
	// at a real node can be resumed rather than restarted.
	Activities []SafeRecoveryActivityView `json:"activities,omitempty"`
}

// SafeRecoveryActivityView is one activity as the entry shows it.
type SafeRecoveryActivityView struct {
	ActivityID  string `json:"activity_id"`
	ActorID     string `json:"actor_id"`
	Mode        string `json:"mode"`
	Status      string `json:"status"`
	Required    int64  `json:"required_seconds"`
	Completed   int64  `json:"completed_seconds"`
	Remaining   int64  `json:"remaining_seconds"`
	Resumable   bool   `json:"resumable"`
	Eligibility string `json:"eligibility"`
}

// SafeRecoveryStateFor reads the branch's formal clock and recovery activities.
//
// It is a plain read with no model call, so opening the entry costs no world
// time and no generation: §三.3 is explicit that opening a window must not
// itself restore anything.
func (a *App) SafeRecoveryStateFor(storyID, branchID, actorID string) (SafeRecoveryState, error) {
	if a == nil {
		return SafeRecoveryState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return SafeRecoveryState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return SafeRecoveryState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	resolvedActor, allowed := playerNamedActor(actorID)
	if !allowed {
		return SafeRecoveryState{}, fmt.Errorf("%w: %s", interactive.ErrRecoveryUnknownActor, strings.TrimSpace(actorID))
	}
	clock, activities, err := store.RecoveryState(storyID, resolvedBranch)
	if err != nil {
		return SafeRecoveryState{}, err
	}
	view := SafeRecoveryState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch, ActorID: resolvedActor,
		ClockSeconds: clock.Seconds, Anchor: clock.Anchor,
	}
	attributes, eligibility, err := store.RecoveryAttributes(strings.TrimSpace(storyID), resolvedBranch, resolvedActor, "")
	if err != nil {
		return SafeRecoveryState{}, err
	}
	view.Health, view.HealthMax = attributes.Health, attributes.HealthMax
	view.Mana, view.ManaMax = attributes.Mana, attributes.ManaMax
	view.Eligibility = string(eligibility)
	view.Refusal = interactive.DescribeRecoveryEligibilityRefusal(eligibility)
	for _, activity := range activities {
		if normalizeRecoveryActor(activity.ActorID, storyContext) != resolvedActor {
			continue
		}
		view.Activities = append(view.Activities, SafeRecoveryActivityView{
			ActivityID: activity.Activity.ID, ActorID: activity.ActorID, Mode: string(activity.Mode),
			Status:   string(activity.Activity.Status),
			Required: int64(activity.Activity.RequiredEffective), Completed: int64(activity.Activity.CompletedEffective),
			Remaining:   int64(activity.Activity.RequiredEffective - activity.Activity.CompletedEffective),
			Resumable:   activity.Activity.Status == storytime.ActivityPaused || activity.Activity.Status == storytime.ActivityAwaitingDecision,
			Eligibility: string(activity.Eligibility),
		})
	}
	return view, nil
}

// ApplySafeRecovery settles one safe rest or meditation interval and commits it.
//
// The rules run first and the commit is one transaction, so the recovery, the
// elapsed time and the activity progress either all exist on the journal or none
// of them do. §三.5's "失败不半扣半补" is that requirement.
func (a *App) ApplySafeRecovery(request SafeRecoveryAppRequest) (SafeRecoveryAppResult, error) {
	if a == nil {
		return SafeRecoveryAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return SafeRecoveryAppResult{}, err
	}
	mode, err := safeRecoveryMode(request.Mode)
	if err != nil {
		return SafeRecoveryAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return SafeRecoveryAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	resolvedActor, allowed := playerNamedActor(request.ActorID)
	if !allowed {
		return SafeRecoveryAppResult{}, fmt.Errorf("%w: %s", interactive.ErrRecoveryUnknownActor, strings.TrimSpace(request.ActorID))
	}
	declared := cultivation.RecoveryEligibility(strings.TrimSpace(request.DeclaredEligibility))
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplySafeRecovery(strings.TrimSpace(request.StoryID), interactive.SafeRecoveryRequest{
		BranchID: resolvedBranch,
		ActorID:  resolvedActor,
		Mode:     mode,
		// §三.1 forbids using real-world waiting as the effective duration, so the
		// interval is the one the player named and nothing else.
		DurationSeconds:     request.DurationSeconds,
		ActivityID:          strings.TrimSpace(request.ActivityID),
		Nodes:               request.Nodes,
		DeclaredEligibility: declared,
		CommandID:           commandID,
		// The fingerprint is derived from what the interval actually is and
		// resolved against the branch the store will really write to, so an empty
		// branch and an explicit current branch are the same command rather than a
		// conflict. A caller that reused one command id for a genuinely different
		// rest is refused instead of being answered with the wrong interval.
		CommandHash: recoveryCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return SafeRecoveryAppResult{}, err
	}
	return SafeRecoveryAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, ActivityID: result.ActivityID, ActorID: result.ActorID,
		Mode: string(result.Mode), ElapsedSeconds: result.Elapsed, Elapsed: result.ElapsedDescription,
		PoolBefore: result.Candidate.PreviousPool, PoolAfter: result.Candidate.CandidatePool,
		PoolMaximum:      result.Candidate.PoolMaximum,
		Completed:        result.Completed,
		PendingNodeIDs:   result.PendingNodeIDs,
		ActivityStatus:   string(result.Activity.Activity.Status),
		ClockSeconds:     result.ClockAfter.Seconds,
		Replayed:         result.Replayed,
		ResidualCleared:  result.ResidualCleared,
		BurdenBefore:     result.BurdenBefore,
		BurdenAfter:      result.BurdenAfter,
		BurdenFloorAfter: result.BurdenFloorAfter,
	}, nil
}

// safeRecoveryMode maps the API's mode names onto the rules' own.
func safeRecoveryMode(mode string) (cultivation.RecoveryMode, error) {
	switch strings.TrimSpace(mode) {
	case string(cultivation.RecoveryHealthRest):
		return cultivation.RecoveryHealthRest, nil
	case string(cultivation.RecoverySafeMeditation):
		return cultivation.RecoverySafeMeditation, nil
	default:
		return "", cultivation.ErrRecoveryUnknownMode
	}
}

// recoveryCommandHash fingerprints the content behind a rest's command ID.
//
// It covers what actually decides the interval — the branch, the character, the
// mode and the length — so reusing one command ID for a genuinely different rest
// is refused rather than answered with the wrong interval. The branch is the
// resolved one, so an empty branch and an explicit current branch fingerprint
// the same and a retry is not mistaken for a conflict.
//
// A continuation is fingerprinted with its activity too: continuing a rest is a
// different command from starting one, and a caller that reused an identity
// across the two would otherwise be handed the wrong answer.
func recoveryCommandHash(request SafeRecoveryAppRequest, branchID, commandID string) string {
	if commandID == "" {
		// No identity means no idempotency to protect, so there is nothing to
		// fingerprint. The store refuses a command id without a hash, and this
		// keeps the two consistent rather than inventing one here.
		return ""
	}
	parts := []string{
		"denova.safe_recovery",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		strings.TrimSpace(request.ActorID),
		strings.TrimSpace(request.Mode),
		fmt.Sprintf("duration\x00%d", request.DurationSeconds),
		"activity\x00" + strings.TrimSpace(request.ActivityID),
		"declared\x00" + strings.TrimSpace(request.DeclaredEligibility),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// normalizeRecoveryActor resolves an actor identity against the story's own
// cast, so an empty request means the protagonist the ordinary input addresses.
func normalizeRecoveryActor(actorID string, storyContext interactive.StoryContext) string {
	if trimmed := strings.TrimSpace(actorID); trimmed != "" {
		return trimmed
	}
	return interactive.DefaultActorID
}

// playerNamedActor resolves the character a player-facing entry may address.
//
// §四 of 战斗建议复核与NPC信息边界 forbids reading or steering another
// character's real pools, realm, arts or stock through an arbitrary actor_id,
// and requires the refusal to happen where the data leaves the process rather
// than in the panel. The player plays exactly one character, so every other
// name is refused; an empty name still means the protagonist the ordinary
// main-story input addresses.
func playerNamedActor(actorID string) (string, bool) {
	trimmed := strings.TrimSpace(actorID)
	if trimmed == "" || trimmed == interactive.DefaultActorID {
		return interactive.DefaultActorID, true
	}
	return "", false
}
