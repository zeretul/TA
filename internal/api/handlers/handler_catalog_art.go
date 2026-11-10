package handlers

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"strconv"
	"ta/internal/catalogart"
)

// HandleCatalogArt serves only content-addressed registered PNG or lossless WebP derivatives.
// There is deliberately no route exposing the manifest or author identities.
func (h *Handlers) HandleCatalogArt(ctx context.Context, c *app.RequestContext) {
	library := catalogart.Default()
	file := c.Param("file")
	canonical := library.CanonicalFile(file)
	if canonical == "" {
		c.Status(consts.StatusNotFound)
		return
	}
	if canonical != file {
		c.Header("Location", catalogart.PathPrefix+canonical)
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Status(consts.StatusPermanentRedirect)
		return
	}
	data, err := library.Read(canonical)
	if err != nil {
		c.Status(consts.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	contentType := library.ContentType(canonical)
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.SetStatusCode(consts.StatusOK)
	if c.IsHead() {
		c.Response.SkipBody = true
		return
	}
	c.Data(consts.StatusOK, contentType, data)
}
