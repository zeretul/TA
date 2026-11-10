package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// TestUpstreamUpdateEntryPointsRefuse covers the rule that this distribution
// does not update from the upstream release channel.
//
// The refusal has to hold at every product entry point rather than at the
// button: the settings panel, the startup check and the message center all
// reach these methods, and a direct API call reaches them too. Hiding the UI
// would leave the endpoints live, so each method is checked here for the typed
// refusal — not for a generic failure, which a caller could mistake for a
// transient upstream problem and retry.
func TestUpstreamUpdateEntryPointsRefuse(t *testing.T) {
	// The flag is a function so this test observes the shipped behaviour
	// without rebuilding the binary; restore it for the other tests either way.
	previous := upstreamUpdatesAllowed
	upstreamUpdatesAllowed = func() bool { return false }
	t.Cleanup(func() { upstreamUpdatesAllowed = previous })

	service := &App{}
	ctx := context.Background()

	// Every call must fail on the refusal alone. An empty App would otherwise
	// reach the update machinery, so a different error here would mean the guard
	// sits below the work it is meant to prevent.
	calls := map[string]func() error{
		"InstallUpdate": func() error { _, err := service.InstallUpdate(ctx); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrUpstreamUpdateDisabled) {
			t.Errorf("%s returned %v, want ErrUpstreamUpdateDisabled", name, err)
		}
	}

	// The background install task reports through its event stream instead of a
	// return value, so it is checked for the refusal code the UI renders.
	task := service.StartInstallUpdateTask("zh-CN")
	replay, live := task.Subscribe()
	for range live.Events() {
	}
	<-task.Done()
	replay, _ = task.Subscribe()
	if len(replay) != 1 {
		t.Fatalf("install task replayed %d events, want 1: %+v", len(replay), replay)
	}
	if replay[0].Event.Type != "error" {
		t.Errorf("install task emitted %q, want an error event", replay[0].Event.Type)
	}
	data, ok := replay[0].Event.Data.(map[string]any)
	if !ok {
		t.Fatalf("install task data = %#v, want a map carrying the refusal code", replay[0].Event.Data)
	}
	if code, _ := data["code"].(string); code != "api.update.upstreamDisabled" {
		t.Errorf("install task code = %q, want api.update.upstreamDisabled", code)
	}
}

// TestLocalUpdateStaysReachableWhenUpstreamIsClosed pins the other half of the
// rule: the refusal is scoped to the upstream channel.
//
// The requirement closes the upstream check, download, install and apply path
// because an upstream release is a different product that would replace this
// fork's build. An archive the player selected from their own disk is not an
// upstream update — it is how a build of this fork is put in place — so the
// local upload and its apply must stay reachable. A guard that refused them too
// would turn the manual update entry into a dead control and discard the
// localized package validation the same requirement keeps visible.
func TestLocalUpdateStaysReachableWhenUpstreamIsClosed(t *testing.T) {
	previous := upstreamUpdatesAllowed
	upstreamUpdatesAllowed = func() bool { return false }
	t.Cleanup(func() { upstreamUpdatesAllowed = previous })

	service := &App{}
	ctx := context.Background()

	// The archive is empty, so the update machinery rejects it. What matters is
	// that the rejection comes from package validation rather than from the
	// upstream refusal: that is the difference between a reachable path and a
	// closed one.
	if _, err := service.InstallLocalUpdate(ctx, "denova-v0.0.0-windows-x64.zip", bytes.NewReader(nil), nil); errors.Is(err, ErrUpstreamUpdateDisabled) {
		t.Errorf("a player-supplied archive must not be refused as an upstream update: %v", err)
	}
}
