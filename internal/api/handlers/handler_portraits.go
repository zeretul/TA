package handlers

import (
	"context"
	"errors"
	"io/fs"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// HandleDefaultPortrait GET /api/portraits/:file — serves one shipped default
// portrait.
//
// It is a lookup, not a file server: the library answers only for a file named
// by the shipped manifest, so a request for anything else is a 404 whether or
// not a file of that name exists in the embedded tree. The response is cacheable
// forever because a portrait's bytes never change under its name.
//
// ?size=display asks for the same person's 192px derivative, which is what a
// single-person window shows; any other value is ignored and the thumbnail is
// served.
func (h *Handlers) HandleDefaultPortrait(ctx context.Context, c *app.RequestContext) {
	display := string(c.Query("size")) == "display"
	data, contentType, err := h.app.Portraits().Read(c.Param("file"), display)
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) {
			status = consts.StatusNotFound
		}
		writeError(c, status, err.Error())
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(consts.StatusOK, contentType, data)
}
