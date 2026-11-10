package handlers

import (
	"context"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"ta/internal/media"
)

// HandleMediaAssets GET /api/media-assets — lists what the media directory
// currently binds.
//
// It is the author's own view of the directory: which subject each file belongs
// to and what it is for. The route names the managed directory rather than
// taking a path, so it cannot be pointed at anything else on disk.
func (h *Handlers) HandleMediaAssets(ctx context.Context, c *app.RequestContext) {
	library := h.app.Media()
	writeJSON(c, consts.StatusOK, media.Manifest{
		Schema: media.ManifestSchema,
		Note:   library.Note(),
		Assets: library.Entries(),
	})
}

// HandleMediaAssetImport POST /api/media-assets/import — copies one uploaded
// file into the managed directory and binds it to a subject.
//
// The bytes are what is stored: no path from the request is ever opened, so an
// import cannot read the rest of the disk. The file is named after its own
// content, which is what makes importing the same picture twice store one file
// and replacing a picture produce a new name rather than a stale cache hit.
func (h *Handlers) HandleMediaAssetImport(ctx context.Context, c *app.RequestContext) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writePictureError(c, media.ErrAssetEmpty)
		return
	}
	if fileHeader.Size > media.MaxAssetBytes {
		writePictureError(c, media.ErrAssetTooLarge)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writePictureError(c, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, media.MaxAssetBytes+1))
	if err != nil {
		writePictureError(c, err)
		return
	}
	if len(data) > media.MaxAssetBytes {
		writePictureError(c, media.ErrAssetTooLarge)
		return
	}
	asset, err := h.app.Media().Import(media.ImportRequest{
		World:     string(c.FormValue("world")),
		Subject:   string(c.FormValue("subject")),
		SourceKey: string(c.FormValue("source_key")),
		Scope:     string(c.FormValue("scope")),
		Actor:     string(c.FormValue("actor")),
		Place:     string(c.FormValue("place")),
		Purpose:   string(c.FormValue("purpose")),
		Scene:     string(c.FormValue("scene")),
		Role:      string(c.FormValue("role")),
		Filename:  fileHeader.Filename,
		Data:      data,
	})
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, media.Imported{Asset: asset, URL: media.URL(asset)})
}

// HandleMediaAssetRemove POST /api/media-assets/remove — drops one binding.
//
// The entry ID travels in the body rather than in the path: an ID names a place
// or an actor and may hold characters a path segment cannot carry safely. Once
// the entry is gone the subject falls back to the picture below it, which is how
// "restore the default" is one operation.
func (h *Handlers) HandleMediaAssetRemove(ctx context.Context, c *app.RequestContext) {
	var body struct {
		ID string `json:"id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writePictureError(c, err)
		return
	}
	removed, err := h.app.Media().Remove(body.ID)
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, removed)
}
