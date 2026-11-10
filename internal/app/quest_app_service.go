package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"ta/internal/interactive"
	"ta/internal/quest"
	"ta/internal/storypack"
)

// This file makes one ordinary task reachable during play.
//
// 自制-事件与任务 §9 asks for a real loop rather than a framework: a task is
// offered, the player takes it on, doing the thing advances it, and finishing it
// pays once. Three properties are deliberate.
//
//   - The offer is not the acceptance. §4.4 keeps them apart, so this entry can
//     offer a task and the player can still decline; nothing advances until they
//     accept it.
//
//   - Progress comes from committed facts. The tick is written by the journey,
//     battle or realm commit that really produced the fact, not by this service.
//     A request here can only accept, abandon or settle.
//
//   - Payment is one fact. §4.7 requires the agreed reward to be settled once;
//     the purse and pack writes are committed in the same transaction as the
//     settled flag, so a retry or a second visit cannot pay twice.

// QuestAppRequest asks for one action on a task.
type QuestAppRequest struct {
	// Locale affects response prose only; it is not part of command identity.
	Locale   string
	StoryID  string
	BranchID string
	// WorldTime is the story's display text for this moment.
	WorldTime string
	// TaskID names the authored task.
	TaskID   string
	ChoiceID string
	// Action is offer, accept, abandon, complete or settle.
	Action string
	// CommandID identifies one attempt, so a retry pays no second time.
	CommandID string
}

// QuestAppResult is one committed task action as the UI reports it.
type QuestAppResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	EventID  string `json:"event_id"`
	TaskID   string `json:"task_id"`
	Action   string `json:"action"`
	// Status is the task's status after this action.
	Status string `json:"status"`
	// Objectives is the task's own wording plus whether each is satisfied.
	Objectives []interactive.QuestObjectiveView `json:"objectives"`
	// Rewards are what this action really paid, empty for any other action.
	Rewards []QuestRewardView `json:"rewards"`
	// Settled reports that the payment was handed over.
	Settled bool `json:"settled"`
	// Replayed reports that this answer came from an action already committed
	// under the same command, so nothing was applied twice.
	Replayed bool `json:"replayed"`
}

// QuestRewardView is one payment in the story's own wording.
type QuestRewardView struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	NameKey  string `json:"name_key,omitempty"`
	Quantity int64  `json:"quantity"`
}

// QuestAppState is the task entry's read: the tasks this branch has taken on,
// and the authored tasks it could still be offered.
type QuestAppState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// Location is the scene's committed location text.
	Location string `json:"location,omitempty"`
	// Tasks are the tasks in play on this branch, in stable id order.
	Tasks []interactive.QuestStateView `json:"tasks"`
	// Available are the authored tasks not yet offered here.
	Available []interactive.QuestOption `json:"available"`
}

// QuestStateFor reads the branch's tasks.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (a *App) QuestStateFor(storyID, branchID string) (QuestAppState, error) {
	return a.QuestStateForLocale(storyID, branchID, "")
}

// QuestStateForLocale translates a read-only view, never the canonical journal.
func (a *App) QuestStateForLocale(storyID, branchID, locale string) (QuestAppState, error) {
	if a == nil {
		return QuestAppState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return QuestAppState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return QuestAppState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.QuestStateFor(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return QuestAppState{}, err
	}
	return a.questStateFrom(storyID, resolvedBranch, locale, storyContext, state), nil
}

func (a *App) questStateFrom(storyID, resolvedBranch, locale string, storyContext interactive.StoryContext, state interactive.QuestState) QuestAppState {
	if state.Tasks == nil {
		state.Tasks = []interactive.QuestStateView{}
	}
	if state.Available == nil {
		state.Available = []interactive.QuestOption{}
	}
	localizeQuestViews(&state, locale, storyContext.Meta)
	if strings.HasPrefix(locale, "en") && storyContext.Meta.PackBinding != nil {
		a.mu.RLock()
		cfg := a.cfg
		a.mu.RUnlock()
		binding := storyContext.Meta.PackBinding
		if cfg != nil {
			if pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID); err == nil && pack.ContentHash() == binding.ContentHash {
				localizeQuestScene(&state, pack)
			}
		}
	}
	return QuestAppState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch,
		Location: state.CurrentLocation,
		Tasks:    state.Tasks, Available: state.Available,
	}
}

// ApplyQuest settles one task action and commits it.
//
// The rules run first and the commit is one transaction, so the record, the
// purse write and the pack write either all exist on the journal or none of them
// do. A retry under the same command ID returns the first attempt's answer with
// no second payment.
func (a *App) ApplyQuest(request QuestAppRequest) (QuestAppResult, error) {
	if a == nil {
		return QuestAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return QuestAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return QuestAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplyQuest(strings.TrimSpace(request.StoryID), interactive.QuestRequest{
		BranchID:  resolvedBranch,
		WorldTime: strings.TrimSpace(request.WorldTime),
		TaskID:    strings.TrimSpace(request.TaskID),
		Action:    strings.TrimSpace(request.Action),
		ChoiceID:  strings.TrimSpace(request.ChoiceID),
		CommandID: commandID,
		// The fingerprint covers what actually decides the action, resolved
		// against the branch the store will really write to, so an empty branch
		// and an explicit current branch are the same command rather than a
		// conflict.
		CommandHash: questCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return QuestAppResult{}, err
	}
	definition, ok := interactive.StoryQuestDefinition(storyContext.Meta, result.TaskID)
	if !ok {
		return QuestAppResult{}, ErrNoWorkspace
	}
	rewards := make([]QuestRewardView, 0, len(result.Rewards))
	for _, reward := range result.Rewards {
		view := QuestRewardView{
			Kind: string(reward.Kind), RefID: reward.RefID, Quantity: reward.Quantity,
		}
		if reward.Kind == quest.RewardItem {
			view.NameKey = interactive.StoryItemNameKey(interactive.StoryCultivationRules(storyContext.Meta), reward.RefID)
		}
		rewards = append(rewards, view)
	}
	return QuestAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, TaskID: result.TaskID, Action: result.Action,
		Status:     string(result.Instance.Status),
		Objectives: questObjectiveViews(interactive.LocalizedStoryQuest(storyContext.Meta, definition, request.Locale), result.Instance),
		Rewards:    rewards,
		Settled:    result.Instance.SettledRewards,
		Replayed:   result.Replayed,
	}, nil
}

// questObjectiveViews is the task's own wording with its real completion state,
// so the answer to an action shows what changed rather than only its status.
func questObjectiveViews(definition quest.Definition, instance quest.Instance) []interactive.QuestObjectiveView {
	completed := make(map[string]bool, len(instance.CompletedObjectiveIDs))
	for _, id := range instance.CompletedObjectiveIDs {
		completed[id] = true
	}
	views := make([]interactive.QuestObjectiveView, 0, len(definition.Objectives))
	for _, objective := range definition.Objectives {
		views = append(views, interactive.QuestObjectiveView{
			ID: objective.ID, Description: objective.Description,
			Completed: completed[objective.ID],
			SourceRef: instance.SourceRefs[objective.ID],
		})
	}
	return views
}

// questCommandHash fingerprints the content behind a task command ID.
//
// It covers what actually decides the action, so reusing one command ID for a
// genuinely different action is refused rather than answered with the wrong
// result. The branch is the resolved one, so an empty branch and an explicit
// current branch fingerprint the same and a retry is not mistaken for a conflict.
func questCommandHash(request QuestAppRequest, branchID, commandID string) string {
	if commandID == "" {
		// No identity means no idempotency to protect. The store refuses a command
		// id without a hash, and this keeps the two consistent.
		return ""
	}
	parts := []string{
		"denova.quest",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		"task\x00" + strings.TrimSpace(request.TaskID),
		"action\x00" + strings.TrimSpace(request.Action),
		"choice\x00" + strings.TrimSpace(request.ChoiceID),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
