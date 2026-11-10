package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"ta/internal/api/sse"
	// The product application layer is aliased because the Hertz request
	// context package above already claims the name app in this file.
	appsvc "ta/internal/app"
	"ta/internal/update"
)

// updateDisabled reports that this build has no configured TA channel.
func updateDisabled(c *app.RequestContext, err error) bool {
	if !errors.Is(err, appsvc.ErrUpstreamUpdateDisabled) {
		return false
	}
	writeErrorKey(c, consts.StatusForbidden, "api.update.upstreamDisabled")
	return true
}

func (h *Handlers) HandleUpdateCheck(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.CheckUpdate(ctx)
	if err != nil {
		if updateDisabled(c, err) {
			return
		}
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update check failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.checkFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateStatus(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.UpdateStatus(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Update status failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.statusFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstall(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.InstallUpdate(ctx)
	if err != nil {
		if errors.Is(err, update.ErrUpdateBusy) {
			writeErrorKey(c, consts.StatusConflict, "api.update.busy")
			return
		}
		if updateDisabled(c, err) {
			return
		}
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update install failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.installFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateApply(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ApplyUpdate(ctx)
	if err != nil {
		if errors.Is(err, update.ErrUpdateBusy) {
			writeErrorKey(c, consts.StatusConflict, "api.update.busy")
			return
		}
		if updateDisabled(c, err) {
			return
		}
		slog.ErrorContext(ctx, "[handlers/handler_update.go] update apply failed", "error", err)
		writeErrorKey(c, consts.StatusBadGateway, "api.update.applyFailed")
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstallStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.StartInstallUpdateTask(requestLocale(c))
	slog.InfoContext(ctx, fmt.Sprintf("[update-sse] attach install task_id=%s", task.ID()))
	sse.StreamTask(ctx, c, task)
}

func (h *Handlers) HandleUpdateUpload(ctx context.Context, c *app.RequestContext) {
	header, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.update.uploadRequired")
		return
	}
	if header.Size > update.MaxLocalArchiveBytes {
		writeErrorKey(c, consts.StatusRequestEntityTooLarge, "api.update.tooLarge")
		return
	}
	file, err := header.Open()
	if err != nil {
		slog.ErrorContext(ctx, "Could not open local update upload", "error", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.update.invalidPackage")
		return
	}
	defer file.Close()
	checksumHeader, err := c.FormFile("checksums")
	if err != nil || checksumHeader.Size > 1<<20 {
		writeErrorKey(c, consts.StatusBadRequest, "api.update.checksumRequired")
		return
	}
	checksumFile, err := checksumHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.update.checksumRequired")
		return
	}
	defer checksumFile.Close()
	result, err := h.app.InstallLocalUpdate(ctx, header.Filename, file, checksumFile)
	if err != nil {
		if updateDisabled(c, err) {
			return
		}
		slog.ErrorContext(ctx, "Local update installation failed", "asset", header.Filename, "error", err)
		status, key := consts.StatusBadRequest, "api.update.invalidPackage"
		switch {
		case errors.Is(err, update.ErrPackageChecksum):
			key = "api.update.checksumRequired"
		case errors.Is(err, update.ErrPackageVersion):
			key = "api.update.newerRequired"
		case errors.Is(err, update.ErrPackagePlatform):
			key = "api.update.wrongPlatform"
		case errors.Is(err, update.ErrPackageTooLarge):
			status, key = consts.StatusRequestEntityTooLarge, "api.update.tooLarge"
		case errors.Is(err, update.ErrUpdateBusy):
			status, key = consts.StatusConflict, "api.update.busy"
		case errors.Is(err, update.ErrInvalidPackage):
		default:
			status, key = consts.StatusInternalServerError, "api.update.installFailed"
		}
		writeErrorKey(c, status, key)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
