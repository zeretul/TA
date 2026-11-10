package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/interactive"
	"ta/internal/publiquevent"
)

// HandleInteractivePublicEventState serves the public event entry's read: the
// events this character has been told about and their current stage.
//
// It is a plain read with no model call. Each stage is derived from the branch's
// committed clock, so opening the entry costs no world time and cannot advance
// anything.
func (h *Handlers) HandleInteractivePublicEventState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.PublicEventStateForLocale(storyID, c.Query("branch"), requestLocale(c))
	if err != nil {
		writePublicEventError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractivePublicEvent settles one participation action and commits it.
//
// The rules run before any prose: the side record and whatever it paid commit in
// one transaction, so the entry never shows a participation the journal does not
// have. command_id makes a retry safe: a repeated click, a lost response or a
// reopened window returns the action the first attempt committed rather than
// paying the same reward out twice.
func (h *Handlers) HandleInteractivePublicEvent(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		WorldTime string `json:"world_time"`
		// EventID names the authored event.
		EventID string `json:"event_id"`
		// Action is join or withdraw.
		Action string `json:"action"`
		// ParticipationAction is how the character takes part when joining.
		ParticipationAction string `json:"participation_action"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// A participation is a committed fact, so it needs a command identity:
	// without one a retried request would settle the same reward twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.publicEvent.commandIDRequired")
		return
	}
	if strings.TrimSpace(body.EventID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.publicEvent.eventIDRequired")
		return
	}
	if strings.TrimSpace(body.Action) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.publicEvent.actionRequired")
		return
	}
	result, err := h.app.ApplyPublicEvent(novaApp.PublicEventAppRequest{
		StoryID: storyID, BranchID: body.Branch, WorldTime: body.WorldTime,
		EventID: body.EventID, Action: body.Action,
		ParticipationAction: body.ParticipationAction, CommandID: body.CommandID,
	})
	if err != nil {
		writePublicEventError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writePublicEventError maps a public event failure onto a status the UI can act
// on.
//
// A refusal is not a technical error: the player asked to take part in an event
// that already settled, or to withdraw from one they never joined. That is a
// world fact the entry should say plainly rather than report as a generic
// failure.
func writePublicEventError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrPublicEventCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.publicEvent.eventNotFound", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventNotHere):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.notHere", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventClosed):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.closed", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventNotJoined):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.notJoined", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventAlreadyJoined):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.alreadyJoined", "detail", err.Error())
	case errors.Is(err, interactive.ErrPublicEventUnknownAction), errors.Is(err, publiquevent.ErrUnknownAction):
		writeErrorKey(c, consts.StatusBadRequest, "api.publicEvent.actionUnsupported", "detail", err.Error())
	case errors.Is(err, publiquevent.ErrInvalidState), errors.Is(err, publiquevent.ErrInvalidEvent):
		writeErrorKey(c, consts.StatusConflict, "api.publicEvent.actionRefused", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
