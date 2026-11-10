package handlers

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleInteractiveLoreOverview(ctx context.Context, c *app.RequestContext) {
	view, err := h.app.ReadLoreOverview(c.Param("id"), c.Query("branch"), requestLocale(c))
	if err != nil {
		writeCharacterWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, view)
}
