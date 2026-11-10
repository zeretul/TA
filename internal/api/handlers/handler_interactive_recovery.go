package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storytime"
)

// HandleInteractiveSafeRecoveryState serves the rest entry's read: the branch's
// formal instant, the character's live pools, and whether ordinary rest is
// available.
//
// It is a plain read with no model call, so opening the entry costs no world
// time and no generation. §三.3 is explicit that opening a window must not
// itself restore anything, so everything it reports is already committed.
func (h *Handlers) HandleInteractiveSafeRecoveryState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.SafeRecoveryStateFor(storyID, c.Query("branch"), c.Query("actor_id"))
	if err != nil {
		writeSafeRecoveryError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractiveSafeRecovery settles one safe rest or meditation interval and
// commits it.
//
// The rules run before any prose: the interval and the pool it restored commit in
// one transaction, so the entry never shows a recovery the journal does not
// have. command_id makes a retry safe: a repeated click, a lost response or a
// reopened window returns the interval the first attempt committed rather than
// granting a second one.
func (h *Handlers) HandleInteractiveSafeRecovery(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		ActorID   string `json:"actor_id"`
		Mode      string `json:"mode"`
		// DurationSeconds is the in-fiction interval the player asked to spend. It
		// is not real waiting time: §三.1 forbids using the player's own clock as
		// the effective duration.
		DurationSeconds int64 `json:"duration_seconds"`
		// ActivityID continues a rest a real node stopped. Empty starts a new one.
		ActivityID string `json:"activity_id"`
		// DeclaredEligibility reports a condition the store cannot derive, such as a
		// creator-marked seal. It can only make the finding stricter.
		DeclaredEligibility string `json:"declared_eligibility"`
		// Nodes are the registered story boundaries this interval must stop at.
		Nodes []recoveryNodeBody `json:"nodes"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// A rest is a committed fact, so it needs a command identity: without one a
	// retried request would settle the interval twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.commandIDRequired")
		return
	}
	if strings.TrimSpace(body.Mode) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.modeRequired")
		return
	}
	result, err := h.app.ApplySafeRecovery(novaApp.SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: body.Branch, ActorID: body.ActorID, Mode: body.Mode,
		DurationSeconds: body.DurationSeconds, ActivityID: body.ActivityID,
		Nodes:               recoveryNodes(body.Nodes),
		DeclaredEligibility: body.DeclaredEligibility,
		CommandID:           body.CommandID,
	})
	if err != nil {
		writeSafeRecoveryError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// recoveryNodeBody is one registered story boundary in a request.
//
// The interval has to stop where the story really needs the player, so the
// boundaries come from the caller and are validated by the storytime rules
// rather than invented here.
type recoveryNodeBody struct {
	ID                string `json:"id"`
	AtSeconds         int64  `json:"at_seconds"`
	Kind              string `json:"kind"`
	RequiresResponse  bool   `json:"requires_response"`
	Resolved          bool   `json:"resolved"`
	PublicDescription string `json:"public_description"`
}

// recoveryNodes maps the request's boundaries onto the rules' own shape. An
// unsupported kind is passed through rather than dropped, so the rules refuse it
// with their own diagnostic instead of silently planning an interval that
// ignores a boundary the story registered.
func recoveryNodes(nodes []recoveryNodeBody) []storytime.Node {
	if len(nodes) == 0 {
		return nil
	}
	mapped := make([]storytime.Node, 0, len(nodes))
	for _, node := range nodes {
		mapped = append(mapped, storytime.Node{
			ID: strings.TrimSpace(node.ID), At: storytime.Instant(node.AtSeconds),
			Kind: storytime.NodeKind(strings.TrimSpace(node.Kind)),
			RequiresResponse: node.RequiresResponse, Resolved: node.Resolved,
			PublicDescription: node.PublicDescription,
		})
	}
	return mapped
}

// writeSafeRecoveryError maps a rest failure onto a status the UI can act on.
//
// A refusal is not a technical error: §三.6 asks for clear wording that does not
// turn a network or rules failure into the character's misfortune, so an
// ineligible rest is reported as the world condition that blocks it rather than
// as a generic failure.
func writeSafeRecoveryError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrRecoveryCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.recovery.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrRecoveryActivityFinished):
		writeErrorKey(c, consts.StatusConflict, "api.recovery.activityFinished")
	case errors.Is(err, cultivation.ErrRecoveryIneligible):
		writeErrorKey(c, consts.StatusConflict, "api.recovery.ineligible")
	case errors.Is(err, cultivation.ErrRecoveryEligibilityChanged):
		writeErrorKey(c, consts.StatusConflict, "api.recovery.eligibilityChanged")
	// A pool maximum can move under a running rest when a wound heals or a
	// breakthrough raises it. The rest stops there rather than projecting from a
	// maximum the body no longer has, and the player is told to start a new one.
	case errors.Is(err, cultivation.ErrRecoveryMaximumChanged):
		writeErrorKey(c, consts.StatusConflict, "api.recovery.maximumChanged")
	case errors.Is(err, cultivation.ErrRecoveryMissingMaximum):
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.maximumMissing")
	case errors.Is(err, cultivation.ErrRecoveryUnknownMode):
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.unknownMode")
	case errors.Is(err, cultivation.ErrRecoveryUnknownEligibility):
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.unknownEligibility")
	case errors.Is(err, interactive.ErrRecoveryMissingDuration):
		writeErrorKey(c, consts.StatusBadRequest, "api.recovery.durationRequired")
	case errors.Is(err, interactive.ErrRecoveryUnknownActivity):
		writeErrorKey(c, consts.StatusNotFound, "api.recovery.activityNotFound")
	case errors.Is(err, interactive.ErrRecoveryUnknownActor):
		writeErrorKey(c, consts.StatusNotFound, "api.recovery.actorNotFound")
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
