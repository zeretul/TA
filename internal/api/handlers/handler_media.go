package handlers

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	runtimeapp "ta/internal/app"
	"ta/internal/media"
)

// clipStream is a byte span of an open clip that closes the file when the
// response writer is done with it. Hertz writes the body after the handler
// returns and closes the stream if it is an io.Closer, so the file must be
// owned by the stream rather than by a defer in the handler.
type clipStream struct {
	reader *io.SectionReader
	file   *os.File
}

func (s *clipStream) Read(p []byte) (int, error) { return s.reader.Read(p) }

func (s *clipStream) Close() error { return s.file.Close() }

// HandleMediaClip GET /api/media/:file — serves one reviewed or imported file.
//
// Pictures and clips share the route because they share a manifest and a rule:
// the library answers only for a file its manifest names, so a request for
// anything else is a 404 whether or not a file of that name happens to exist on
// disk. The media type comes from the file the manifest listed, never from the
// request.
//
// A browser seeks inside a video while it plays, so a range request has to be
// answered with a 206 and the exact byte span. Without that a clip would either
// not start or would restart from zero on every seek. The whole file is streamed
// from disk rather than read into memory: a clip is tens of megabytes.
//
// The name of an asset is derived from its content hash — shipped files are
// named by it, and an import is written as u-<hash> — so its bytes never change
// under one name and the response is cacheable forever. Replacing a picture
// produces a different name, which is what stops a replaced picture from being
// served from a stale cache.
func (h *Handlers) HandleMediaClip(ctx context.Context, c *app.RequestContext) {
	handle, asset, err := h.app.Media().Open(c.Param("file"))
	if err != nil {
		writePictureError(c, err)
		return
	}
	streamMediaFile(c, handle, asset.File, "public, max-age=31536000, immutable")
}

// HandleIntimacyClip serves a participant's clip after checking the live activity.
func (h *Handlers) HandleIntimacyClip(_ context.Context, c *app.RequestContext) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("X-Content-Type-Options", "nosniff")
	handle, asset, err := h.app.OpenIntimacyClip(runtimeapp.IntimacyClipRequest{
		StoryID: c.Param("id"), BranchID: c.Query("branch"), ActivityID: c.Query("activity_id"),
		ActorID: c.Query("actor"), File: c.Param("file"),
	})
	if err != nil {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	streamMediaFile(c, handle, asset.File, "private, no-store, max-age=0")
}

func streamMediaFile(c *app.RequestContext, handle *os.File, file, cacheControl string) {
	// The response body is written after this handler returns, so the file has to
	// stay open until the stream is drained. Hertz closes a body stream that is an
	// io.Closer once it has written it; closing here instead would hand the writer
	// a closed file and the player would receive a Content-Length with no bytes.
	handedOff := false
	defer func() {
		if !handedOff {
			_ = handle.Close()
		}
	}()

	info, err := handle.Stat()
	if err != nil {
		writePictureError(c, err)
		return
	}
	size := info.Size()

	c.Header("Content-Type", media.ContentType(file))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", cacheControl)

	start, length := int64(0), size
	status := consts.StatusOK
	if raw := c.Request.Header.Peek("Range"); len(raw) > 0 {
		from, to, parseErr := app.ParseByteRange(raw, int(size))
		if parseErr != nil {
			// The requested span cannot exist. Tell the browser how large the
			// clip really is so it can ask again with a valid range.
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", size))
			c.Header("Content-Length", "0")
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
	// SetBodyStream takes the length separately so the response carries a
	// Content-Length and the player can size its buffer before the first byte.
	// The stream owns the file from here: hertz closes an io.Closer body stream
	// after writing it, so handing it off must not also close it here.
	handedOff = true
	c.Response.SetBodyStream(&clipStream{reader: io.NewSectionReader(handle, start, length), file: handle}, int(length))
}
