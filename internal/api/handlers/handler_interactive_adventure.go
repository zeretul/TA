package handlers

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"strings"
	"ta/internal/interactive"
)

func (h *Handlers) HandleInteractiveAdventureWindow(ctx context.Context, c *app.RequestContext) {
	window, err := h.app.AdventureWindow(strings.TrimSpace(c.Param("id")), c.Query("branch"), requestLocale(c))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.adventure.unavailable")
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

func (h *Handlers) HandleInteractiveAdventureAction(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Branch    string                      `json:"branch"`
		CommandID string                      `json:"command_id"`
		Head      string                      `json:"expected_head"`
		Action    interactive.AdventureAction `json:"action"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	// A retry must name the original branch explicitly. A current-branch alias
	// could otherwise target a different fork with the same head after a switch.
	body.Branch = strings.TrimSpace(body.Branch)
	if body.Branch == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	eventID, err := h.app.PlayAdventureRoundContext(ctx, strings.TrimSpace(c.Param("id")), body.Branch, body.CommandID, body.Head, body.Action)
	if err != nil {
		writeErrorKey(c, consts.StatusConflict, "api.adventure.rejected")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"event_id": eventID})
}
