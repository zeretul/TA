package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/interactive"
	"ta/internal/quest"
)

// HandleInteractiveQuestState serves the task entry's read: the tasks this
// branch has taken on and the authored tasks it could still be offered.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (h *Handlers) HandleInteractiveQuestState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.QuestStateForLocale(storyID, c.Query("branch"), requestLocale(c))
	if err != nil {
		writeQuestError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractiveQuest settles one task action and commits it.
//
// The rules run before any prose: the record, the purse write and the pack write
// commit in one transaction, so the entry never shows a task the journal does not
// have. command_id makes a retry safe: a repeated click, a lost response or a
// reopened window returns the action the first attempt committed rather than
// paying the same reward out twice.
func (h *Handlers) HandleInteractiveQuest(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		WorldTime string `json:"world_time"`
		// TaskID names the authored task.
		TaskID   string `json:"task_id"`
		ChoiceID string `json:"choice_id"`
		// Action is offer, accept, abandon, complete or settle.
		Action string `json:"action"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// A task action is a committed fact, so it needs a command identity: without
	// one a retried request would settle the same reward twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.quest.commandIDRequired")
		return
	}
	if strings.TrimSpace(body.TaskID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.quest.taskIDRequired")
		return
	}
	if strings.TrimSpace(body.Action) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.quest.actionRequired")
		return
	}
	result, err := h.app.ApplyQuest(novaApp.QuestAppRequest{
		StoryID: storyID, BranchID: body.Branch, WorldTime: body.WorldTime, Locale: requestLocale(c),
		TaskID: body.TaskID, Action: body.Action, ChoiceID: body.ChoiceID, CommandID: body.CommandID,
	})
	if err != nil {
		writeQuestError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeQuestError maps a task failure onto a status the UI can act on.
//
// A refusal is not a technical error: the player asked to settle a task they
// never took on, or to complete one with work still open. That is a world fact
// the entry should say plainly rather than report as a generic failure.
func writeQuestError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrQuestCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.quest.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestUnknownAction):
		writeErrorKey(c, consts.StatusBadRequest, "api.quest.actionUnsupported", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestUnknownTask), errors.Is(err, quest.ErrInvalidTask):
		writeErrorKey(c, consts.StatusNotFound, "api.quest.taskNotFound", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestNotAccepted):
		writeErrorKey(c, consts.StatusConflict, "api.quest.notAccepted", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestAlreadyOffered):
		writeErrorKey(c, consts.StatusConflict, "api.quest.alreadyOffered", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestAlreadySettled):
		writeErrorKey(c, consts.StatusConflict, "api.quest.alreadySettled", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestExpired):
		// The work may have been done; the agreement ran out before it was handed
		// in. That is a conflict with the world's state, not a malformed request.
		writeErrorKey(c, consts.StatusConflict, "api.quest.expired", "detail", err.Error())
	case errors.Is(err, interactive.ErrQuestObjectiveUnmet), errors.Is(err, quest.ErrObjectiveNotMet):
		writeErrorKey(c, consts.StatusConflict, "api.quest.objectiveUnmet", "detail", err.Error())
	case errors.Is(err, quest.ErrInvalidState):
		writeErrorKey(c, consts.StatusConflict, "api.quest.actionRefused", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
