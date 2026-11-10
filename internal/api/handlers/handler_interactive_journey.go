package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/interactive"
)

// HandleInteractiveJourneyState serves the journey entry's read: where the
// character stands, the routes that start there, and any road already under way.
//
// It is a plain read with no model call, so opening the entry costs no world
// time and no generation. Everything it reports is already committed.
func (h *Handlers) HandleInteractiveJourneyState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.JourneyStateForLocale(storyID, c.Query("branch"), requestLocale(c))
	if err != nil {
		writeJourneyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractiveJourney settles one stretch of travel and commits it.
//
// The rules run before any prose: the distance, the in-fiction time and the new
// place commit in one transaction, so the entry never shows a road the journal
// does not have. command_id makes a retry safe: a repeated click, a lost response
// or a reopened window returns the stretch the first attempt committed rather
// than charging the same road twice.
func (h *Handlers) HandleInteractiveJourney(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		WorldTime string `json:"world_time"`
		// RouteID names the registered connection. Empty continues the branch's one
		// unfinished journey.
		RouteID string `json:"route_id"`
		// JourneyID continues a journey a real stop interrupted.
		JourneyID string `json:"journey_id"`
		// TargetMinutes is the absolute elapsed minute to reach, counted from the
		// route's origin. It is the mid-journey stop's point, not a duration.
		TargetMinutes int64 `json:"target_minutes"`
		// Arrive walks the whole route in one commit, which is the ordinary case.
		Arrive bool `json:"arrive"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// Travel is a committed fact, so it needs a command identity: without one a
	// retried request would walk the road twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.journey.commandIDRequired")
		return
	}
	result, err := h.app.ApplyJourney(novaApp.JourneyAppRequest{
		StoryID: storyID, BranchID: body.Branch, WorldTime: body.WorldTime,
		RouteID: body.RouteID, JourneyID: body.JourneyID,
		TargetMinutes: body.TargetMinutes, Arrive: body.Arrive,
		CommandID: body.CommandID,
	})
	if err != nil {
		writeJourneyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeJourneyError maps a journey failure onto a status the UI can act on.
//
// A refusal is not a technical error: the player asked to walk somewhere the
// registered data does not connect, or from somewhere the scene does not place
// them. That is a world fact the entry should say plainly rather than report as a
// generic failure.
func writeJourneyError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrJourneyCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.journey.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrJourneyNotAtOrigin):
		writeErrorKey(c, consts.StatusConflict, "api.journey.notAtOrigin", "detail", err.Error())
	case errors.Is(err, interactive.ErrJourneyNoProgress):
		writeErrorKey(c, consts.StatusConflict, "api.journey.noProgress", "detail", err.Error())
	case errors.Is(err, interactive.ErrJourneyAtLifespanEdge):
		// The road did not fail; the years did. Reporting it as a generic bad
		// request would read as a malformed command, and reporting it as
		// noProgress would claim the player asked to stand still. This is a fact
		// about the character that the entry has to say plainly.
		writeErrorKey(c, consts.StatusConflict, "api.journey.lifespanEdge", "detail", err.Error())
	case errors.Is(err, interactive.ErrJourneyUnknownJourney):
		writeErrorKey(c, consts.StatusNotFound, "api.journey.journeyNotFound", "detail", err.Error())
	case errors.Is(err, interactive.ErrJourneyUnknownRoute):
		writeErrorKey(c, consts.StatusNotFound, "api.journey.routeNotFound", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
