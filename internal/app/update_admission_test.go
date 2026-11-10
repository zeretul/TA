package app

import (
	"context"
	"errors"
	"testing"

	"ta/internal/update"
)

func TestUpdateAdmissionRejectsBusyAndFencesNewWork(t *testing.T) {
	a := &App{}
	release, err := a.AcquireUpdateUse()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("busy apply: %v", err)
	}
	release()
	old := applyStagedUpdate
	defer func() { applyStagedUpdate = old }()
	applyStagedUpdate = func(context.Context) (update.ApplyResult, error) {
		if _, err := a.AcquireUpdateUse(); !errors.Is(err, update.ErrUpdateBusy) {
			t.Fatalf("admission while applying: %v", err)
		}
		return update.ApplyResult{}, errors.New("injected start failure")
	}
	if _, err := a.ApplyUpdate(context.Background()); err == nil {
		t.Fatal("expected start failure")
	}
	done, err := a.AcquireUpdateUse()
	if err != nil {
		t.Fatalf("not released after failure: %v", err)
	}
	done()
}

func TestUpdateAdmissionTracksWorkspaceAndDetachedTask(t *testing.T) {
	a := &App{workspace: "workspace-a"}
	op, err := a.acquireWorkspaceOperation(context.Background(), "workspace-a", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("workspace operation: %v", err)
	}
	op.Release()
	task, err := registerReplayProductTask(a, "Game", false, 128)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("detached task: %v", err)
	}
	task.RejectStart(errors.New("test cleanup"))
	a.unregisterWorkspaceTask(task)
	done, err := a.AcquireUpdateUse()
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestUpdateAdmissionFencesConcurrentRequests(t *testing.T) {
	a := &App{}
	old := applyStagedUpdate
	defer func() { applyStagedUpdate = old }()
	entered := make(chan struct{})
	finish := make(chan struct{})
	result := make(chan error, 1)
	applyStagedUpdate = func(context.Context) (update.ApplyResult, error) {
		close(entered)
		<-finish
		return update.ApplyResult{}, errors.New("test failure")
	}
	go func() { _, err := a.ApplyUpdate(context.Background()); result <- err }()
	<-entered
	if _, err := a.acquireRootOperation(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("new root operation: %v", err)
	}
	if _, err := a.acquireWorkspaceOperation(context.Background(), "w", false); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("new workspace operation: %v", err)
	}
	close(finish)
	<-result
	done, err := a.AcquireUpdateUse()
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestSuccessfulApplyKeepsAdmissionClosed(t *testing.T) {
	a := &App{workspace: "workspace-a"}
	old := applyStagedUpdate
	defer func() { applyStagedUpdate = old }()
	applyStagedUpdate = func(context.Context) (update.ApplyResult, error) {
		return update.ApplyResult{Status: "restarting"}, nil
	}
	if _, err := a.ApplyUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AcquireUpdateUse(); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("request admitted: %v", err)
	}
	if _, err := a.acquireOwnedProjectOperation(context.Background(), "project-id"); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("project admitted: %v", err)
	}
	if _, err := registerReplayProductTask(a, "Game", false, 128); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("task admitted: %v", err)
	}
	if _, _, _, err := a.beginWorkspaceTransitionTo(); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("transition admitted: %v", err)
	}
}

func TestProjectOperationBlocksUpdate(t *testing.T) {
	a := newExecutionProfileTestApp(t)
	operation, err := a.AcquireProjectOperation(context.Background(), a.ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("active project admitted update: %v", err)
	}
	operation.Release()
}
