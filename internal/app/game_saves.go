package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"ta/internal/interactive"
)

// GameSave identifies one Story in an available Book. Workspace is a runtime
// projection for the existing workspace switch API, never a persisted identity.
type GameSave struct {
	ProjectID string                   `json:"project_id"`
	WorldName string                   `json:"world_name"`
	Workspace string                   `json:"workspace"`
	Story     interactive.StorySummary `json:"story"`
}

type GameSaveCatalog struct {
	Saves       []GameSave `json:"saves"`
	Unavailable []string   `json:"unavailable"`
}

// GameSaves reads all available worlds without changing the foreground Project.
// Each Project lease protects its layout during reads; active Stories reuse the
// live Store so discovery never competes with an open journal handle.
func (a *App) GameSaves(ctx context.Context) (GameSaveCatalog, error) {
	result := GameSaveCatalog{Saves: []GameSave{}, Unavailable: []string{}}
	if a.projectRegistry == nil {
		return result, fmt.Errorf("project registry is unavailable")
	}
	records, err := a.projectRegistry.Books()
	if err != nil {
		return result, err
	}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		op, err := a.AcquireProjectOperation(ctx, record.ID)
		if err != nil {
			slog.WarnContext(ctx, "[game-saves] Project unavailable", "project_id", record.ID, "error", err)
			result.Unavailable = append(result.Unavailable, record.Name)
			continue
		}
		layout := op.Layout()
		a.mu.RLock()
		store := a.interactive
		if store == nil || store.Root() != layout.ContentRoot {
			store = nil
		}
		a.mu.RUnlock()
		owned := store == nil
		if owned {
			store = interactive.NewStore(layout.ContentRoot)
		}
		index, readErr := store.Index()
		if owned {
			if closeErr := store.Close(); readErr == nil {
				readErr = closeErr
			}
		}
		op.Release()
		if readErr != nil {
			slog.WarnContext(ctx, "[game-saves] Story index unavailable", "project_id", record.ID, "error", readErr)
			result.Unavailable = append(result.Unavailable, record.Name)
			continue
		}
		for _, story := range index.Stories {
			result.Saves = append(result.Saves, GameSave{ProjectID: record.ID, WorldName: record.Name, Workspace: layout.ContentRoot, Story: story})
		}
	}
	sort.SliceStable(result.Saves, func(i, j int) bool { return result.Saves[i].Story.UpdatedAt > result.Saves[j].Story.UpdatedAt })
	return result, nil
}
