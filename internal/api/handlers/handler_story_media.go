package handlers

import (
	"context"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"io"
	"ta/internal/media"
)

func (h *Handlers) HandleStoryMedia(ctx context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("X-Content-Type-Options", "nosniff")
	f, asset, err := h.app.OpenStoryMedia(c.Param("id"), c.Query("branch"), c.Query("head"), c.Query("revision"), c.Query("actor"), c.Param("asset_id"))
	if err != nil {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	handedOff := false
	defer func() {
		if !handedOff {
			f.Close()
		}
	}()
	info, err := f.Stat()
	if err != nil {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	size := info.Size()
	start, length := int64(0), size
	status := consts.StatusOK
	c.Header("Content-Type", media.ContentType(asset.File))
	c.Header("Accept-Ranges", "bytes")
	if raw := c.Request.Header.Peek("Range"); len(raw) > 0 {
		from, to, err := app.ParseByteRange(raw, int(size))
		if err != nil {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
			c.SetStatusCode(consts.StatusRequestedRangeNotSatisfiable)
			return
		}
		start, length = int64(from), int64(to-from+1)
		status = consts.StatusPartialContent
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, size))
	}
	c.SetStatusCode(status)
	if c.IsHead() {
		c.Header("Content-Length", fmt.Sprint(length))
		c.Response.SkipBody = true
		return
	}
	handedOff = true
	c.Response.SetBodyStream(&clipStream{reader: io.NewSectionReader(f, start, length), file: f}, int(length))
}
