package app

import (
	"log/slog"
	"ta/internal/interactive"
)

func (a *App) PlayerAttention(storyID, branchID string) (interactive.PlayerAttentionView, error) {
	if a == nil {
		return interactive.PlayerAttentionView{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return interactive.PlayerAttentionView{}, ErrNoWorkspace
	}
	return store.PlayerAttention(storyID, branchID)
}

func (a *App) UpdatePlayerAttention(storyID string, in interactive.AttentionUpdate) (interactive.PlayerAttentionView, error) {
	if a == nil {
		return interactive.PlayerAttentionView{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return interactive.PlayerAttentionView{}, ErrNoWorkspace
	}
	result, err := store.UpdatePlayerAttention(storyID, in)
	if err != nil {
		slog.Warn("Player attention update rejected", "story_id", storyID, "branch_id", in.BranchID, "error", err)
	}
	return result, err
}
