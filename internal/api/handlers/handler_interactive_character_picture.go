package handlers

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/media"
)

// HandleInteractiveCharacterPicture GET
// /api/interactive/stories/:id/characters/:character/picture — one person's
// picture as the window would draw it.
//
// The author's editor reads this to preview the change before and after an
// import. It answers the same chain the game uses rather than a second set of
// rules, so a preview cannot disagree with what the player sees.
func (h *Handlers) HandleInteractiveCharacterPicture(ctx context.Context, c *app.RequestContext) {
	storyID := string(c.Param("id"))
	characterID := string(c.Param("character"))
	picture, err := h.app.CharacterPicture(storyID, c.Query("branch"), characterID)
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, picture)
}

// HandleInteractiveCharacterPictureImport POST
// /api/interactive/stories/:id/characters/:character/picture — copies one
// uploaded picture into the media directory and binds it to this person.
//
// The binding is worked out from what the story already committed, so the request
// names a person and a purpose and nothing else. The bytes are what is stored: no
// path from the request is ever opened.
func (h *Handlers) HandleInteractiveCharacterPictureImport(ctx context.Context, c *app.RequestContext) {
	storyID := string(c.Param("id"))
	characterID := string(c.Param("character"))
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
	result, err := h.app.CharacterPictureImport(novaApp.CharacterPictureImport{
		StoryID:     storyID,
		BranchID:    c.Query("branch"),
		CharacterID: characterID,
		Purpose:     string(c.FormValue("purpose")),
		Filename:    fileHeader.Filename,
		Data:        data,
	})
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleInteractiveCharacterPictureClear POST
// /api/interactive/stories/:id/characters/:character/picture/clear — drops this
// person's external picture and reports what answers instead.
//
// Removing the override is what makes the next layer answer, so the answer
// carries the restored picture: the editor does not have to read again to know
// what to draw.
func (h *Handlers) HandleInteractiveCharacterPictureClear(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.CharacterPictureClear(novaApp.CharacterPictureImport{
		StoryID:     string(c.Param("id")),
		BranchID:    c.Query("branch"),
		CharacterID: string(c.Param("character")),
		Purpose:     string(c.FormValue("purpose")),
	})
	if err != nil {
		writePictureError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writePictureError keeps the two failures apart: a build with no media directory
// cannot store anything, which is not the same as a bad request.
func writePictureError(c *app.RequestContext, err error) {
	status := consts.StatusBadRequest
	key := "api.media.requestFailed"
	switch {
	case errors.Is(err, novaApp.ErrStoryPictureManaged):
		status = consts.StatusConflict
		key = "api.media.storyManaged"
	case errors.Is(err, media.ErrAssetStoryRestricted):
		status = consts.StatusConflict
		key = "api.media.storyRestricted"
	case errors.Is(err, media.ErrNoMediaDir):
		status = consts.StatusServiceUnavailable
		key = "api.media.noDirectory"
	case errors.Is(err, media.ErrAssetTooLarge):
		key = "api.media.tooLarge"
	case errors.Is(err, media.ErrAssetEmpty):
		key = "api.media.empty"
	case errors.Is(err, media.ErrAssetInvalid):
		key = "api.media.invalid"
	case errors.Is(err, media.ErrAssetUnknown), errors.Is(err, fs.ErrNotExist):
		status = consts.StatusNotFound
		key = "api.media.notFound"
	}
	slog.Warn("[media] request failed", "error", err)
	writeErrorKey(c, status, key)
}
