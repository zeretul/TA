package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "ta/internal/app"
	"ta/internal/concurrency"
	"ta/internal/history"
	"ta/internal/i18n"
)

func (h *Handlers) HandleHistoryBootstrap(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.HistoryService().Bootstrap(ctx, requestLocale(c))
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryCampaigns(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	result, err := h.app.HistoryService().List(ctx, scope.ProjectID)
	historyResponse(c, map[string]any{"campaigns": result}, err)
}
func (h *Handlers) HandleHistoryCreate(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body struct {
		Name       string `json:"name"`
		CommandID  string `json:"command_id"`
		ScenarioID string `json:"scenario_id,omitempty"`
		Locale     string `json:"locale,omitempty"`
	}
	if !decodeHistoryRequest(c, &body) {
		return
	}

	result, err := h.app.HistoryService().Create(ctx, scope.ProjectID, history.CreateRequest{Name: body.Name, CommandID: body.CommandID, ScenarioID: body.ScenarioID, Locale: historyLocale(c, body.Locale)})
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryRead(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	result, err := h.app.HistoryService().Read(ctx, scope.ProjectID, c.Param("campaign_id"))
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryChat(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryChatRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().Chat(ctx, scope.ProjectID, c.Param("campaign_id"), body)
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryPrepareEdict(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryPrepareRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().PrepareEdict(ctx, scope.ProjectID, c.Param("campaign_id"), body)
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryEdict(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryEdictRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().CommitEdict(ctx, scope.ProjectID, c.Param("campaign_id"), body)
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryAdvance(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryCommandRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().Advance(ctx, scope.ProjectID, c.Param("campaign_id"), body)
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryCheckpoint(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "checkpoint")
}
func (h *Handlers) HandleHistoryRestore(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "restore")
}
func (h *Handlers) HandleHistoryInspect(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "inspect")
}
func (h *Handlers) handleHistoryStateCommand(ctx context.Context, c *app.RequestContext, kind string) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryStateRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().StateCommand(ctx, scope.ProjectID, c.Param("campaign_id"), kind, body)
	historyResponse(c, result, err)
}
func (h *Handlers) HandleHistoryCancel(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
	}
	if !decodeHistoryRequest(c, &body) {
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.history.invalidRequest")
		return
	}
	writeJSON(c, consts.StatusOK, map[string]bool{"cancelled": h.app.HistoryService().Cancel(scope.ProjectID, c.Param("campaign_id"), body.CommandID)})
}
func decodeHistoryRequest(c *app.RequestContext, target any) bool {
	if len(c.Request.Body()) > 128*1024 {
		writeErrorKey(c, consts.StatusBadRequest, "api.history.invalidRequest")
		return false
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), target); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.history.invalidRequest")
		return false
	}
	return true
}
func historyLocale(c *app.RequestContext, fallback string) string {
	if locale := requestLocale(c); locale != "" {
		return locale
	}
	return i18n.Resolve(fallback)
}
func historyResponse(c *app.RequestContext, result any, err error) {
	if err == nil {
		writeJSON(c, consts.StatusOK, result)
		return
	}
	status, key := consts.StatusInternalServerError, "api.history.unavailable"
	switch {
	case errors.Is(err, history.ErrNotFound):
		status, key = consts.StatusNotFound, "api.history.notFound"
	case errors.Is(err, history.ErrConflict):
		status, key = consts.StatusConflict, "api.history.conflict"
	case errors.Is(err, history.ErrCommandConflict), errors.Is(err, appsvc.ErrHistoryCommandConflict):
		status, key = consts.StatusConflict, "api.history.commandConflict"
	case errors.Is(err, appsvc.ErrHistoryBusy):
		status, key = consts.StatusConflict, "api.history.busy"
	case errors.Is(err, appsvc.ErrHistoryProject):
		status, key = consts.StatusBadRequest, "api.history.wrongProject"
	case errors.Is(err, appsvc.ErrHistoryInput), errors.Is(err, history.ErrInvalid):
		status, key = consts.StatusBadRequest, "api.history.invalidRequest"
	case errors.Is(err, context.Canceled), errors.Is(err, concurrency.ErrClosing), errors.Is(err, concurrency.ErrClosed):
		status, key = consts.StatusConflict, "api.history.cancelled"
	case errors.Is(err, appsvc.ErrHistoryModel):
		status, key = consts.StatusBadGateway, "api.history.modelFailed"
	case errors.Is(err, appsvc.ErrHistoryModelBudget):
		status, key = consts.StatusBadGateway, "api.history.modelOutputBudget"
	case errors.Is(err, appsvc.ErrHistoryModelOutput):
		status, key = consts.StatusBadGateway, "api.history.invalidModelOutput"
	}
	slog.WarnContext(context.Background(), "[history] request failed", "operation", string(c.Path()), "code", key, "error", err)
	writeErrorKey(c, status, key)
}

func (h *Handlers) HandleHistoryScenarioImport(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	if len(c.Request.Body()) > history.MaxScenarioBundleBytes {
		historyResponse(c, nil, history.ErrInvalid)
		return
	}
	p, err := h.app.HistoryService().ImportScenarioPackage(ctx, scope.ProjectID, c.Request.Body())
	historyResponse(c, p, err)
}
func (h *Handlers) HandleHistoryScenarioExport(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	p, err := h.app.HistoryService().ExportScenario(scope.ProjectID, c.Param("scenario_id"))
	historyResponse(c, p, err)
}

func (h *Handlers) HandleHistoryDirectEdict(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body appsvc.HistoryPrepareRequest
	if !decodeHistoryRequest(c, &body) {
		return
	}
	body.Locale = historyLocale(c, body.Locale)
	result, err := h.app.HistoryService().DirectEdict(ctx, scope.ProjectID, c.Param("campaign_id"), body)
	historyResponse(c, result, err)
}

func (h *Handlers) HandleHistoryArchive(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "archive")
}
func (h *Handlers) HandleHistoryUnarchive(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "unarchive")
}

func (h *Handlers) HandleHistoryAdvice(ctx context.Context, c *app.RequestContext) {
	h.handleHistoryStateCommand(ctx, c, "advice")
}

func (h *Handlers) HandleHistoryScenarioBundle(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	data, err := h.app.HistoryService().ExportScenarioBundle(scope.ProjectID, c.Param("scenario_id"))
	if err != nil {
		historyResponse(c, nil, err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=\""+c.Param("scenario_id")+".zip\"")
	c.SetStatusCode(consts.StatusOK)
	c.Response.SetBody(data)
}
func (h *Handlers) HandleHistoryScenarioAsset(ctx context.Context, c *app.RequestContext) {
	data, mime, err := h.app.HistoryService().ScenarioAsset(c.Param("asset_name"))
	if err != nil {
		historyResponse(c, nil, err)
		return
	}
	c.Header("Content-Type", mime)
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.SetStatusCode(consts.StatusOK)
	c.Response.SetBody(data)
}
