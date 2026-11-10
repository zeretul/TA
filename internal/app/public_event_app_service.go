package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"ta/internal/interactive"
)

// This file makes one public event reachable during play.
//
// 自制-事件与任务 §4.4, §4.5 and §6 ask for a situation that several parties care
// about, that moves in stages with real world time, and that each character knows
// only as much of as they were really told. The separation that keeps that honest
// lives in the store; this service only resolves the branch and fingerprints the
// command.
//
// Two properties are deliberate here.
//
//   - Reading is free. PublicEventStateFor derives every stage from the branch's
//     committed clock, so opening the entry costs no world time and cannot move
//     the event forward.
//
//   - One request is one fact. A join or a withdrawal commits a side record and
//     whatever it really paid in a single transaction, so a retry under the same
//     command ID returns the first answer instead of paying twice.

// PublicEventAppRequest asks for one participation action.
type PublicEventAppRequest struct {
	StoryID  string
	BranchID string
	// WorldTime is the story's display text for this moment.
	WorldTime string
	// EventID names the authored event.
	EventID string
	// Action is join or withdraw.
	Action string
	// ParticipationAction is how the character takes part when joining: observe,
	// help, investigate or withdraw. Empty means observe.
	ParticipationAction string
	// CommandID identifies one attempt, so a retry pays no second time.
	CommandID string
}

// PublicEventAppResult is one committed participation action as the UI reports it.
type PublicEventAppResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	EventID  string `json:"event_id"`
	Action   string `json:"action"`
	// Participation is the choice recorded on the participation.
	Participation string `json:"participation"`
	// JoinedStage is the stage the character was present at. It is what the entry
	// uses to say where they were, and it is not the current stage if the event
	// has moved on since.
	JoinedStage string `json:"joined_stage,omitempty"`
	// Withdrawn reports that the character stepped back from the participation.
	Withdrawn bool `json:"withdrawn"`
	// Rewards are what this action really paid, empty for a withdrawal.
	Rewards []PublicEventRewardView `json:"rewards"`
	// Replayed reports that this answer came from an action already committed
	// under the same command, so nothing was applied twice.
	Replayed bool `json:"replayed"`
}

// PublicEventRewardView is one payment in the story's own wording.
type PublicEventRewardView struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	Quantity int64  `json:"quantity"`
}

// PublicEventAppState is the event entry's read.
type PublicEventAppState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// Location is the scene's committed location text, which is what decides
	// which events this character has been told about.
	Location     string `json:"location,omitempty"`
	ClockSeconds int64  `json:"clock_seconds"`
	ClockText    string `json:"clock_text,omitempty"`
	// Events are the public events this character knows about, in stable id order.
	Events []interactive.PublicEventView `json:"events"`
}

// PublicEventStateFor reads the public events this branch knows about.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (a *App) PublicEventStateFor(storyID, branchID string) (PublicEventAppState, error) {
	if a == nil {
		return PublicEventAppState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return PublicEventAppState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return PublicEventAppState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.PublicEventStateFor(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return PublicEventAppState{}, err
	}
	return publicEventAppStateFrom(storyID, resolvedBranch, state), nil
}

func publicEventAppStateFrom(storyID, resolvedBranch string, state interactive.PublicEventState) PublicEventAppState {
	if state.Events == nil {
		state.Events = []interactive.PublicEventView{}
	}
	return PublicEventAppState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch,
		Location: state.Location, ClockSeconds: state.ClockSeconds, ClockText: state.ClockText,
		Events: state.Events,
	}
}

// ApplyPublicEvent settles one participation action and commits it.
//
// The rules run first and the commit is one transaction, so the record and the
// payment either both exist on the journal or neither does. A retry under the
// same command ID returns the first attempt's answer with no second payment.
func (a *App) ApplyPublicEvent(request PublicEventAppRequest) (PublicEventAppResult, error) {
	if a == nil {
		return PublicEventAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return PublicEventAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return PublicEventAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplyPublicEvent(strings.TrimSpace(request.StoryID), interactive.PublicEventRequest{
		BranchID:            resolvedBranch,
		WorldTime:           strings.TrimSpace(request.WorldTime),
		EventID:             strings.TrimSpace(request.EventID),
		Action:              strings.TrimSpace(request.Action),
		ParticipationAction: strings.TrimSpace(request.ParticipationAction),
		CommandID:           commandID,
		// The fingerprint covers what actually decides the action, resolved
		// against the branch the store will really write to, so an empty branch
		// and an explicit current branch are the same command rather than a
		// conflict.
		CommandHash: publicEventCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return PublicEventAppResult{}, err
	}
	rewards := make([]PublicEventRewardView, 0, len(result.Rewards))
	for _, reward := range result.Rewards {
		rewards = append(rewards, PublicEventRewardView{
			Kind: string(reward.Kind), RefID: reward.RefID, Quantity: reward.Quantity,
		})
	}
	return PublicEventAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, Action: result.Action,
		Participation: string(result.Instance.Action),
		JoinedStage:   result.Instance.JoinedStage,
		Withdrawn:     result.Instance.Withdrawn,
		Rewards:       rewards,
		Replayed:      result.Replayed,
	}, nil
}

// publicEventCommandHash fingerprints the content behind an event command ID.
//
// It covers what actually decides the action, so reusing one command ID for a
// genuinely different action is refused rather than answered with the wrong
// result. The branch is the resolved one, so an empty branch and an explicit
// current branch fingerprint the same and a retry is not mistaken for a conflict.
func publicEventCommandHash(request PublicEventAppRequest, branchID, commandID string) string {
	if commandID == "" {
		// No identity means no idempotency to protect. The store refuses a command
		// id without a hash, and this keeps the two consistent.
		return ""
	}
	parts := []string{
		"denova.public_event",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		"event\x00" + strings.TrimSpace(request.EventID),
		"action\x00" + strings.TrimSpace(request.Action),
		"participation\x00" + strings.TrimSpace(request.ParticipationAction),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
