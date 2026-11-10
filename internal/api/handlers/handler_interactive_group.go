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

// HandleInteractiveGroupWindow serves the group surface: the scene, every group
// on this branch, and one group's conversation when one was named.
//
// It is a plain read. Opening a group, reading its history or switching between
// groups costs no world time and runs no model: a group is a place to talk, and
// looking at it is not an act.
func (h *Handlers) HandleInteractiveGroupWindow(ctx context.Context, c *app.RequestContext) {
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
	window, err := h.app.GroupWindow(novaApp.GroupWindowRequest{
		StoryID: storyID, BranchID: c.Query("branch"),
		GroupID: c.Query("group_id"), BeforeCursor: c.Query("before"), Limit: limit,
	})
	if err != nil {
		writeGroupError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

// HandleInteractiveGroupCreate makes a self-built group from people the story
// already knows.
//
// The request names real characters by id; a name that is not in this branch's
// cast is refused rather than quietly created. Creating a group makes a place to
// talk and changes nothing else: no party, no rank, no relationship.
func (h *Handlers) HandleInteractiveGroupCreate(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string   `json:"command_id"`
		Branch    string   `json:"branch"`
		Name      string   `json:"name"`
		Type      string   `json:"type"`
		Members   []string `json:"members"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.groupChat.nameRequired")
		return
	}
	group, err := h.app.CreateGroup(novaApp.GroupCreateRequest{
		StoryID: storyID, BranchID: body.Branch, Name: body.Name,
		Type: body.Type, MemberIDs: body.Members, CommandID: body.CommandID,
	})
	if err != nil {
		writeGroupError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, group)
}

// HandleInteractiveGroupMembers adds and removes people from a group.
//
// A removal ends that person's membership at this point; adding them later opens
// a new one. Neither rewrites what was already said, so a person who was away
// does not gain the conversation they missed.
func (h *Handlers) HandleInteractiveGroupMembers(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	groupID := strings.TrimSpace(c.Param("group"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if groupID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.groupChat.groupIDRequired")
		return
	}
	var body struct {
		CommandID string   `json:"command_id"`
		Branch    string   `json:"branch"`
		Add       []string `json:"add"`
		Remove    []string `json:"remove"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	group, err := h.app.ChangeGroupMembers(novaApp.GroupMemberChangeRequest{
		StoryID: storyID, BranchID: body.Branch, GroupID: groupID,
		Add: body.Add, Remove: body.Remove, CommandID: body.CommandID,
	})
	if err != nil {
		writeGroupError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, group)
}

// HandleInteractiveGroupMessage sends one message to a group and returns the
// round the members actually committed.
//
// One round is one model task, so the whole request is one call. command_id
// makes a retry a read: a repeated click or a lost response returns the round the
// first attempt committed rather than asking the group a second time.
func (h *Handlers) HandleInteractiveGroupMessage(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		GroupID   string `json:"group_id"`
		Message   string `json:"message"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.GroupID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.groupChat.groupIDRequired")
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
	result, err := h.app.SendGroupMessage(ctx, novaApp.GroupSendRequest{
		StoryID: storyID, BranchID: body.Branch, GroupID: body.GroupID,
		Message: body.Message, CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeGroupError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeGroupError maps a group failure onto a status the window can act on.
//
// An unreachable group is a 409 rather than an error: the request was valid, the
// world simply does not carry a conversation there right now, and the window says
// so instead of inventing replies.
func writeGroupError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrRadioContactUnavailable):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.radioUnavailable")
	case errors.Is(err, novaApp.ErrGroupUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.groupChat.groupNotFound", "detail", err.Error())
	case errors.Is(err, novaApp.ErrCharacterUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.characterWindow.characterNotFound", "detail", err.Error())
	case errors.Is(err, novaApp.ErrGroupUnsupported):
		writeErrorKey(c, consts.StatusConflict, "api.groupChat.unsupported", "detail", err.Error())
	case errors.Is(err, novaApp.ErrGroupNoReachableMember):
		writeErrorKey(c, consts.StatusConflict, "api.groupChat.noReachableMember", "detail", err.Error())
	case errors.Is(err, character.ErrGroupVersionConflict):
		writeErrorKey(c, consts.StatusConflict, "api.groupChat.versionConflict", "detail", err.Error())
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrCharacterCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.characterWindow.commandConflict", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
