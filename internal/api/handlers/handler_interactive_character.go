package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/interactive"
)

// HandleInteractiveCharacterWindow serves the character window read: the current
// scene, the story's cast, and one character's conversation with the player.
//
// It is a plain read with no model call, so opening the window costs no world
// time and no generation: everything it shows is already committed.
func (h *Handlers) HandleInteractiveCharacterWindow(ctx context.Context, c *app.RequestContext) {
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
	window, err := h.app.CharacterWindow(novaApp.CharacterWindowRequest{
		Locale:       requestLocale(c),
		StoryID:      storyID,
		BranchID:     c.Query("branch"),
		CharacterID:  c.Query("character_id"),
		BeforeCursor: c.Query("before"),
		Limit:        limit,
	})
	if err != nil {
		writeCharacterWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

// HandleInteractiveCharacterMessage sends one message in the character window and
// returns the character's reply.
//
// The reply is generated and committed in one call, so the window never shows a
// line the journal does not have. command_id makes a retry safe: a repeated
// click, a lost response or a recovery re-read returns the exchange the first
// attempt committed rather than asking the character again.
func (h *Handlers) HandleInteractiveCharacterMessage(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID   string `json:"command_id"`
		Branch      string `json:"branch"`
		CharacterID string `json:"character_id"`
		Message     string `json:"message"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.CharacterID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.characterWindow.characterIDRequired")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	// A character conversation is a conversation, not an authored turn: it must
	// not allocate a turn number or advance the story's head as a Turn. It still
	// needs a command identity so a retry cannot produce a second reply.
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.SendCharacterMessage(ctx, novaApp.CharacterChatRequest{
		StoryID: storyID, BranchID: body.Branch, CharacterID: body.CharacterID,
		Message: body.Message, CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeCharacterWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveCharacterMessageStatus records what the player did with one
// incoming message: read it, deal with it later, or ignore it.
//
// It is deliberately separate from sending a reply. Marking a message handled
// never accepts whatever the message proposed; only a committed reply can do
// that.
func (h *Handlers) HandleInteractiveCharacterMessageStatus(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	characterID := strings.TrimSpace(c.Param("character"))
	messageID := strings.TrimSpace(c.Param("message"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		Branch string `json:"branch"`
		Status string `json:"status"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	state, err := h.markCharacterMessage(storyID, body.Branch, characterID, messageID, body.Status)
	if err != nil {
		writeCharacterWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"status": "ok", "character_state": state})
}

func (h *Handlers) markCharacterMessage(storyID, branchID, characterID, messageID, status string) (any, error) {
	switch strings.TrimSpace(status) {
	case "read":
		return h.app.MarkCharacterMessageRead(storyID, branchID, characterID, messageID)
	case "handled":
		return h.app.MarkCharacterMessageHandled(storyID, branchID, characterID, messageID)
	case "ignored":
		return h.app.MarkCharacterMessageIgnored(storyID, branchID, characterID, messageID)
	default:
		return nil, errCharacterMessageStatusInvalid
	}
}

var errCharacterMessageStatusInvalid = errors.New("message status must be read, handled or ignored")

// writeCharacterWindowError maps a window failure onto a status the UI can act
// on. An unknown character is a 404 rather than a 400: the request was well
// formed, the character simply is not part of this story.
func writeCharacterWindowError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrRadioContactUnavailable):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.radioUnavailable")
	case errors.Is(err, novaApp.ErrCharacterUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.characterWindow.characterNotFound", "detail", err.Error())
	case errors.Is(err, novaApp.ErrCharacterChatUnavailable):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.letterRequiresStoryTurn")
	case errors.Is(err, novaApp.ErrCharacterContactRequiresStoryTurn):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.letterRequiresStoryTurn")
	case errors.Is(err, novaApp.ErrCharacterNotPresent):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.notPresent")
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, errCharacterMessageStatusInvalid):
		writeErrorKey(c, consts.StatusBadRequest, "api.characterWindow.invalidMessageStatus")
	case errors.Is(err, interactive.ErrCharacterCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.commandConflict", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
