package app

import (
	"context"
	"log/slog"
	agentrun "ta/internal/agents/run"
	apptask "ta/internal/app/task"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// queueNewDefinitionArt runs only after a successful canonical game commit.
// Registered workspace tasks preserve cancellation/drain fencing and supply
// the existing recover boundary; optional art never delays the story result.
func (c *interactiveAgentCycle) queueNewDefinitionArt(ctx context.Context) {
	if !c.definitionArtEnabled {
		return
	}
	snapshot, err := c.store.Snapshot(c.storyID, c.branchID)
	if err != nil || snapshot.CurrentTurn == nil {
		return
	}
	if c.conversation == nil {
		return
	}
	committed, _, persisted := c.conversation.LastTurnForState()
	if !persisted || committed.ID != snapshot.CurrentTurn.ID {
		return
	}
	style := c.storyContext.Meta.ImageSettings.PresetID
	if style == "" {
		style = "realistic"
	}
	system := interactive.StoryDirectorActorStateSystem{}
	if snapshot.ActorStateSchema != nil {
		system = snapshot.ActorStateSchema.System
	}
	jobs := interactive.NewGeneratedDefinitionArtJobs(c.storyContext.Snapshot.State, snapshot.State, style, system)
	if len(jobs) == 0 {
		return
	}
	turnID := snapshot.CurrentTurn.ID
	_, err = apptask.NewRegisteredWithContext(ctx, func(task *apptask.Task) error {
		c.app.mu.Lock()
		defer c.app.mu.Unlock()
		if c.app.interactive != c.store {
			return ErrWorkspaceChanged
		}
		return c.app.registerWorkspaceTaskLocked(task, c.workspace, true)
	}, func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer c.app.unregisterWorkspaceTask(task)
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			if err := c.app.Images().GenerateDefinitionArt(ctx, c.workspace, c.storyID, c.branchID, turnID, job); err != nil {
				slog.WarnContext(ctx, "[definition-art] optional job ended without art", "definition_id", job.DefinitionID, "error", err)
			}
		}
	})
	if err != nil {
		slog.WarnContext(ctx, "[definition-art] optional task not admitted", "story_id", c.storyID, "error", err)
	}
}

// Public art references never choose their own authorization scope. Imported
// world metadata may contain another Project's stable ID; do not turn that
// into a cross-Project asset request. Presets are scope-free, and generated
// results must match the actual Project selected by the application.
func projectDefinitionArt(details cultivation.DefinitionDetails, projectID string) cultivation.DefinitionDetails {
	if details.Art != nil && (details.Art.Validate() != nil || details.Art.Source == "generated" && (projectID == "" || details.Art.ProjectID != projectID)) {
		details.Art = nil
	}
	return details
}
