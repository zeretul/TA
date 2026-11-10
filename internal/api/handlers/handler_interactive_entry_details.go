package handlers

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"strings"
)

func (h *Handlers) HandleInteractiveCultivationEntryDetails(ctx context.Context, c *app.RequestContext) {
	view, err := h.app.CultivationEntryDetails(strings.TrimSpace(c.Param("id")), c.Query("branch"), requestLocale(c))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	writeJSON(c, consts.StatusOK, view)
}
