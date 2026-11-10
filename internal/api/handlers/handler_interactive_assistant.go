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

// The in-story assistant.
//
// 自制-剧情助手与选项分析 puts two read-only actions inside the main story's own
// input area. Neither one is a story turn, so neither route here commits a turn,
// advances a branch, or allocates a command id: asking a question and choosing
// not to act must be indistinguishable to the journal. The one write — adopting
// a direction — is a separate route, because the design requires reading advice
// and accepting it to be different acts.

// HandleInteractiveStoryAssistant answers one read-only question about the scene
// in front of the player.
//
// The request carries the turn it was asked about. If the branch has moved on
// since — the player advanced, rewound, or switched branches — the answer would
// describe a scene that no longer exists, so it is refused with a conflict the
// interface can explain rather than shown as current advice.
func (h *Handlers) HandleInteractiveStoryAssistant(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		BranchID     string `json:"branch_id"`
		Branch       string `json:"branch"`
		TurnID       string `json:"turn_id"`
		Action       string `json:"action"`
		PlayerInput  string `json:"player_input"`
		ChosenOption string `json:"chosen_option"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	branchID := body.BranchID
	if strings.TrimSpace(branchID) == "" {
		branchID = body.Branch
	}
	result, err := h.app.AskStoryAssistant(ctx, novaApp.StoryAssistantRequest{
		StoryID: storyID, BranchID: branchID, TurnID: body.TurnID,
		Action: body.Action, PlayerInput: body.PlayerInput, ChosenOption: body.ChosenOption,
		Locale: requestLocale(c),
	})
	if err != nil {
		writeStoryAssistantError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveStoryAssistantAdopt records an explicitly accepted
// direction as this branch's forward intent.
//
// This is the only assistant route that writes. It goes through the branch
// plan's own versioned commit path, so the direction reaches the next turn by
// the same route as every other part of the plan and a concurrent edit is
// reported as a conflict rather than overwritten.
func (h *Handlers) HandleInteractiveStoryAssistantAdopt(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		BranchID     string `json:"branch_id"`
		Branch       string `json:"branch"`
		TurnID       string `json:"turn_id"`
		Direction    string `json:"direction"`
		BaseRevision string `json:"base_revision"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if err := interactive.ValidateStoryAssistantDirection(body.Direction); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.storyAssistant.invalidDirection", "detail", err.Error())
		return
	}
	branchID := body.BranchID
	if strings.TrimSpace(branchID) == "" {
		branchID = body.Branch
	}
	result, err := h.app.AdoptStoryAssistantDirection(ctx, novaApp.StoryAssistantAdoptRequest{
		StoryID: storyID, BranchID: branchID, TurnID: body.TurnID,
		Direction: body.Direction, BaseRevision: body.BaseRevision,
	})
	if err != nil {
		writeStoryAssistantError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeStoryAssistantError maps an assistant failure onto a status the interface
// can act on.
//
// The stale-turn case is a conflict rather than a bad request: the question was
// well formed when it was asked, and the only thing wrong with it is that the
// story has moved. The interface reacts to that by discarding the answer and
// offering to ask again, not by telling the player they typed something invalid.
func writeStoryAssistantError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrStoryAssistantStaleTurn):
		writeErrorKey(c, consts.StatusConflict, "api.storyAssistant.staleTurn", "detail", err.Error())
	case errors.Is(err, novaApp.ErrStoryAssistantUnknownAction):
		writeErrorKey(c, consts.StatusBadRequest, "api.storyAssistant.unknownAction", "detail", err.Error())
	case errors.Is(err, novaApp.ErrStoryAssistantNoPlan):
		// The branch has no plan to record a direction in. That is a story setting
		// the player can change, not a malformed request.
		writeErrorKey(c, consts.StatusConflict, "api.storyAssistant.noPlan", "detail", err.Error())
	case errors.Is(err, interactive.ErrBranchPlanRevisionConflict):
		writeErrorKey(c, consts.StatusConflict, "api.storyAssistant.planConflict", "detail", err.Error())
	case errors.Is(err, novaApp.ErrStoryAssistantEmptyAnswer):
		writeErrorKey(c, consts.StatusBadGateway, "api.storyAssistant.emptyAnswer", "detail", err.Error())
	case errors.Is(err, novaApp.ErrStoryAssistantUnreadableAnswer):
		writeErrorKey(c, consts.StatusBadGateway, "api.storyAssistant.unreadableAnswer", "detail", err.Error())
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.storyAssistant.failed", "detail", err.Error())
	}
}
