package handlers

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"ta/internal/interactive"
)

func (h *Handlers) HandlePlayerAttention(ctx context.Context, c *app.RequestContext) {
	view, err := h.app.PlayerAttention(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.attention.unavailable")
		return
	}
	writeJSON(c, consts.StatusOK, view)
}

func (h *Handlers) HandleUpdatePlayerAttention(ctx context.Context, c *app.RequestContext) {
	var in interactive.AttentionUpdate
	if err := c.BindJSON(&in); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	view, err := h.app.UpdatePlayerAttention(c.Param("id"), in)
	if err != nil {
		key, status := "api.attention.unavailable", consts.StatusBadRequest
		switch {
		case errors.Is(err, interactive.ErrAttentionUnknown):
			key = "api.attention.unknown"
		case errors.Is(err, interactive.ErrAttentionLimit):
			key = "api.attention.limit"
		case errors.Is(err, interactive.ErrAttentionConflict):
			key = "api.attention.conflict"
			status = consts.StatusConflict
		case errors.Is(err, interactive.ErrAttentionInvalid):
			key = "api.common.invalidRequest"
		}
		writeErrorKey(c, status, key)
		return
	}
	writeJSON(c, consts.StatusOK, view)
}
