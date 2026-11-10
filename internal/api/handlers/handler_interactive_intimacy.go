package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/character"
	"ta/internal/interactive"
)

// HandleInteractiveIntimacyWindow serves the intimacy window: the scene, this
// branch's activities with each person's answer, one activity's conversation and
// the dedicated model's display state.
//
// It is a plain read. Opening the window, re-reading a saved activity or
// resuming one costs no world time and runs no model, which is what makes
// "恢复历史不重放开场" true rather than a UI convention.
func (h *Handlers) HandleInteractiveIntimacyWindow(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = parsed
	}
	window, err := h.app.IntimacyWindow(novaApp.IntimacyWindowRequest{
		StoryID: storyID, BranchID: c.Query("branch"),
		ActivityID: c.Query("activity_id"), BeforeCursor: c.Query("before"), Limit: limit,
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

// HandleInteractiveIntimacyInvite asks one person to join an activity.
//
// The person really answers: their own words and their own status are what the
// response carries. A one-to-one invitation that is accepted enters the activity
// directly (§3.1), so the response may already be a started activity with its
// opening; a refusal or a condition stops at the answer.
func (h *Handlers) HandleInteractiveIntimacyInvite(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID   string `json:"command_id"`
		Branch      string `json:"branch"`
		ActivityID  string `json:"activity_id"`
		CharacterID string `json:"character_id"`
		OneOnOne    bool   `json:"one_on_one"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.CharacterID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.characterIDRequired")
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.InviteIntimacy(ctx, novaApp.IntimacyInviteRequest{
		StoryID: storyID, BranchID: body.Branch, ActivityID: body.ActivityID,
		CharacterID: body.CharacterID, OneOnOne: body.OneOnOne,
		CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveIntimacyRespond records one invited person's answer. It is
// used when a person was invited without being asked yet, or when a conditional
// answer's condition has since been settled and they are asked again.
func (h *Handlers) HandleInteractiveIntimacyRespond(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	activityID := strings.TrimSpace(c.Param("activity"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if activityID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.activityIDRequired")
		return
	}
	var body struct {
		CommandID   string `json:"command_id"`
		Branch      string `json:"branch"`
		CharacterID string `json:"character_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.CharacterID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.characterIDRequired")
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.RespondIntimacy(ctx, novaApp.IntimacyRespondRequest{
		StoryID: storyID, BranchID: body.Branch, ActivityID: activityID,
		CharacterID: body.CharacterID, CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveIntimacyConfirm starts a multi-person activity with the roster
// the player confirmed.
//
// The confirmation is the player's own act and is required even when everyone
// accepted (§3.3). expected_version is the roster version the player was shown;
// a stale confirmation is refused rather than applied to a roster it never saw.
func (h *Handlers) HandleInteractiveIntimacyConfirm(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	activityID := strings.TrimSpace(c.Param("activity"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if activityID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.activityIDRequired")
		return
	}
	var body struct {
		CommandID       string `json:"command_id"`
		Branch          string `json:"branch"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.ConfirmIntimacy(ctx, novaApp.IntimacyConfirmRequest{
		StoryID: storyID, BranchID: body.Branch, ActivityID: activityID,
		ExpectedVersion: body.ExpectedVersion, CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveIntimacyMessage sends one line in an open activity and returns
// the round the roster committed.
func (h *Handlers) HandleInteractiveIntimacyMessage(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	activityID := strings.TrimSpace(c.Param("activity"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if activityID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.activityIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		Message   string `json:"message"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.SendIntimacyMessage(ctx, novaApp.IntimacySendRequest{
		StoryID: storyID, BranchID: body.Branch, ActivityID: activityID,
		Message: body.Message, CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveIntimacyEnd closes an activity. Its conversation stays
// readable, which is why ending is not a delete.
func (h *Handlers) HandleInteractiveIntimacyEnd(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	activityID := strings.TrimSpace(c.Param("activity"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if activityID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.intimacy.activityIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.EndIntimacy(ctx, novaApp.IntimacyEndRequest{
		StoryID: storyID, BranchID: body.Branch, ActivityID: activityID, CommandID: body.CommandID,
	})
	if err != nil {
		writeIntimacyError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeIntimacyError maps a window failure onto a status the UI can act on.
//
// The unconfigured-model case is a 409 with its own key rather than a generic
// error: §4.1 forbids silently falling back to the main story's model, so the
// window must offer the settings entry instead of pretending the scene can run.
func writeIntimacyError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrIntimacyModelUnconfigured):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.modelUnconfigured")
	case errors.Is(err, novaApp.ErrIntimacyNotInScene):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.notInScene", "detail", err.Error())
	case errors.Is(err, novaApp.ErrIntimacySceneUnsafe):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.sceneUnsafe")
	case errors.Is(err, novaApp.ErrIntimacyNotOpen):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.notOpen")
	case errors.Is(err, novaApp.ErrIntimacyUnknownActivity):
		writeErrorKey(c, consts.StatusNotFound, "api.intimacy.activityNotFound")
	case errors.Is(err, character.ErrIntimacyNotAdult):
		writeErrorKey(c, consts.StatusForbidden, "api.intimacy.notAdult", "detail", err.Error())
	case errors.Is(err, novaApp.ErrIntimacyProtagonistGenderUnknown):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.protagonistGenderUnknown")
	case errors.Is(err, novaApp.ErrIntimacySameGender):
		writeErrorKey(c, consts.StatusForbidden, "api.intimacy.sameGender", "detail", err.Error())
	case errors.Is(err, character.ErrIntimacyNoConsent):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.noConsent")
	case errors.Is(err, character.ErrIntimacyVersionConflict):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.versionConflict", "detail", err.Error())
	case errors.Is(err, character.ErrIntimacyPhase):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.wrongPhase", "detail", err.Error())
	case errors.Is(err, character.ErrIntimacyTooMany):
		writeErrorKey(c, consts.StatusConflict, "api.intimacy.tooMany")
	case errors.Is(err, novaApp.ErrCharacterUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.characterWindow.characterNotFound", "detail", err.Error())
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrCharacterCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.commandConflict", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
