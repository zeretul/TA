package handlers

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"log/slog"
)

func (h *Handlers) HandleGameSaves(ctx context.Context, c *app.RequestContext) {
	catalog, err := h.app.GameSaves(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "[game-saves] Catalog failed", "error", err)
		writeErrorKey(c, consts.StatusInternalServerError, "api.gameSaves.loadFailed")
		return
	}
	writeJSON(c, consts.StatusOK, catalog)
}
