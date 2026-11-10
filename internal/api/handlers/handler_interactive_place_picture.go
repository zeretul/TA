package handlers

import (
	"context"
	"io"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/media"
)

// HandleInteractivePlacePicture GET
// /api/interactive/stories/:id/places/picture?place=... — one place's picture as
// the window would draw it.
func (h *Handlers) HandleInteractivePlacePicture(ctx context.Context, c *app.RequestContext) {
	picture, err := h.app.PlacePicture(string(c.Param("id")), c.Query("place"))
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, picture)
}

// HandleInteractivePlacePictureImport POST
// /api/interactive/stories/:id/places/picture — copies one uploaded picture into
// the media directory and binds it to a place this story already knows.
func (h *Handlers) HandleInteractivePlacePictureImport(ctx context.Context, c *app.RequestContext) {
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
	result, err := h.app.PlacePictureImport(novaApp.PlacePictureImport{
		StoryID:  string(c.Param("id")),
		Place:    string(c.FormValue("place")),
		Filename: fileHeader.Filename,
		Data:     data,
	})
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractivePlacePictureClear POST
// /api/interactive/stories/:id/places/picture/clear — drops this place's external
// picture.
func (h *Handlers) HandleInteractivePlacePictureClear(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.PlacePictureClear(novaApp.PlacePictureImport{
		StoryID: string(c.Param("id")),
		Place:   string(c.FormValue("place")),
	})
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
