package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	agentrun "ta/internal/agents/run"
	apptask "ta/internal/app/task"
	"ta/internal/buildinfo"
	"ta/internal/i18n"
	"ta/internal/update"
)

// ErrUpstreamUpdateDisabled indicates that no trusted TA release channel is configured.
var ErrUpstreamUpdateDisabled = errors.New("TA update channel is not configured")

// The channel is fixed by the release build; the upstream attribution is never a source.
var upstreamUpdatesAllowed = func() bool { return update.ChannelConfigured(buildinfo.UpdateRepository) }

func (a *App) CheckUpdate(ctx context.Context) (update.CheckResult, error) {
	release, err := a.AcquireUpdateUse()
	if err != nil {
		return update.CheckResult{}, err
	}
	defer release()
	return update.NewService().Check(ctx)
}

func (a *App) InstallUpdate(ctx context.Context) (update.InstallResult, error) {
	release, err := a.AcquireUpdateUse()
	if err != nil {
		return update.InstallResult{}, err
	}
	defer release()
	if !upstreamUpdatesAllowed() {
		return update.InstallResult{}, ErrUpstreamUpdateDisabled
	}
	return update.NewService().Install(ctx)
}

// InstallLocalUpdate validates a player-selected archive and its checksum before staging.
func (a *App) InstallLocalUpdate(ctx context.Context, name string, archive io.Reader, checksums io.Reader) (update.InstallResult, error) {
	release, err := a.AcquireUpdateUse()
	if err != nil {
		return update.InstallResult{}, err
	}
	defer release()
	return update.NewService().InstallLocal(ctx, name, archive, checksums)
}

// UpdateStatus reports local build and validated staged-package state without network access.
func (a *App) UpdateStatus(ctx context.Context) (update.Status, error) {
	release, err := a.AcquireUpdateUse()
	if err != nil {
		return update.Status{}, err
	}
	defer release()
	return update.NewService().Status(ctx)
}

// AcquireUpdateUse admits request or owned work until its release. Apply uses
// the exclusive side and never waits for or cancels active work.
func (a *App) AcquireUpdateUse() (func(), error) {
	if a == nil || !a.updateGate.TryRLock() {
		return nil, update.ErrUpdateBusy
	}
	var once sync.Once
	return func() { once.Do(a.updateGate.RUnlock) }, nil
}

var applyStagedUpdate = func(ctx context.Context) (update.ApplyResult, error) { return update.NewService().Apply(ctx) }

func (a *App) ApplyUpdate(ctx context.Context) (result update.ApplyResult, err error) {
	if a == nil || !a.updateGate.TryLock() {
		return update.ApplyResult{}, update.ErrUpdateBusy
	}
	started := false
	defer func() {
		if !started {
			a.updateGate.Unlock()
		}
	}()
	result, err = applyStagedUpdate(ctx)
	if err != nil {
		return result, err
	}
	started = true
	return result, nil
}

// StartInstallUpdateTask retains the initiating request's language throughout
// the detached download without exposing internal diagnostics to the player.
func (a *App) StartInstallUpdateTask(locale string) *apptask.Task {
	release, admissionErr := a.AcquireUpdateUse()
	return apptask.New(func(ctx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		if admissionErr != nil {
			emit(agentrun.Event{Type: "error", Data: map[string]any{"message": i18n.New(locale).T("api.update.busy"), "code": "api.update.busy"}})
			return
		}
		defer release()
		if !upstreamUpdatesAllowed() {
			emit(agentrun.Event{Type: "error", Data: map[string]any{"message": i18n.New(locale).T("api.update.upstreamDisabled"), "code": "api.update.upstreamDisabled"}})
			return
		}
		result, err := update.NewService().InstallWithProgress(ctx, func(progress update.InstallProgress) {
			emit(agentrun.Event{Type: "update_progress", Data: progress})
		})
		if err != nil {
			slog.ErrorContext(ctx, "[app/update_app_service.go] update installation failed", "error", err)
			key := "api.update.installFailed"
			if errors.Is(err, update.ErrUpdateBusy) {
				key = "api.update.busy"
			}
			emit(agentrun.Event{Type: "error", Data: map[string]any{"message": i18n.New(locale).T(key), "code": key}})
			return
		}
		emit(agentrun.Event{Type: "update_result", Data: result})
	})
}
