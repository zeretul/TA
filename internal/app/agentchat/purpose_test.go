package agentchat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agents "ta/internal/agents"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/session"
	"ta/internal/interactive"
)

// storyIndexVersionForTest mirrors the interactive store's current index schema.
const storyIndexVersionForTest = 1

func writeStoryIndex(t *testing.T, workspace string, index interactive.Index) {
	t.Helper()
	dir := filepath.Join(workspace, "interactive", "story")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func purposeTestWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	writeStoryIndex(t, workspace, interactive.Index{
		Version:        storyIndexVersionForTest,
		CurrentStoryID: "story-current",
		Stories: []interactive.StorySummary{
			{ID: "story-current", PackBinding: &interactive.StoryPackBinding{PackID: "pack-current", Scope: "world-current"}},
			{ID: "story-other", PackBinding: &interactive.StoryPackBinding{PackID: "pack-other", Scope: "world-other"}},
			{ID: "story-unbound"},
		},
	})
	return workspace
}

func TestParsePurposeAcceptsOnlyKnownSurfaces(t *testing.T) {
	for _, testCase := range []struct {
		value string
		want  Purpose
	}{
		{"game-settings", PurposeGameSettings},
		{" game-settings ", PurposeGameSettings},
		{"lore", PurposeLore},
		{"", ""},
		{"teller", ""},
		{"skills", ""},
		{"editor", ""},
	} {
		if got := ParsePurpose(testCase.value); got != testCase.want {
			t.Fatalf("ParsePurpose(%q) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

func TestPurposeToolScopeKeepsEditorSurfaceWithoutPurpose(t *testing.T) {
	workspace := purposeTestWorkspace(t)
	if scope := purposeToolScope(workspace, purposeState{}, nil); scope != nil {
		t.Fatalf("unpurpose scope = %#v, want nil editor surface", scope)
	}
}

func TestPurposeToolScopeBindsGameSettingsToTheNamedStoryWorld(t *testing.T) {
	workspace := purposeTestWorkspace(t)
	scope := purposeToolScope(workspace, purposeState{Purpose: PurposeGameSettings, StoryID: "story-other"}, nil)
	if scope == nil {
		t.Fatal("game settings scope is nil")
	}
	if scope.LoreScope == nil || *scope.LoreScope != "world-other" {
		t.Fatalf("lore scope = %v, want world-other", scope.LoreScope)
	}
	if len(scope.ConfigResources) != len(gameAssistantResources) {
		t.Fatalf("config resources = %v, want %v", scope.ConfigResources, gameAssistantResources)
	}
	for _, name := range scope.ConfigResources {
		if name == "skill" || name == "agent_profile" {
			t.Fatalf("game settings kept process-wide resource %q", name)
		}
	}
}

func TestPurposeToolScopeRefusesToReachAnUnknownStoryWorld(t *testing.T) {
	workspace := purposeTestWorkspace(t)
	for _, storyID := range []string{"story-missing", "story-unbound", ""} {
		scope := purposeToolScope(workspace, purposeState{Purpose: PurposeGameSettings, StoryID: storyID}, nil)
		if scope == nil || scope.LoreScope == nil {
			t.Fatalf("story %q: scope = %#v, want a bound shared scope", storyID, scope)
		}
		if *scope.LoreScope != "" {
			t.Fatalf("story %q resolved to world %q, want the shared scope", storyID, *scope.LoreScope)
		}
	}
}

func TestPurposeToolScopeLoreFollowsTheStoryInPlay(t *testing.T) {
	workspace := purposeTestWorkspace(t)
	scope := purposeToolScope(workspace, purposeState{Purpose: PurposeLore}, nil)
	if scope == nil || scope.LoreScope == nil {
		t.Fatalf("lore scope = %#v, want the world of the story in play", scope)
	}
	if *scope.LoreScope != "world-current" {
		t.Fatalf("lore scope = %q, want world-current", *scope.LoreScope)
	}
	if len(scope.ConfigResources) != len(gameAssistantResources) {
		t.Fatalf("lore config resources = %v", scope.ConfigResources)
	}
}

func TestPurposeToolScopeLoreWithoutAStoryStaysUnbound(t *testing.T) {
	workspace := t.TempDir()
	writeStoryIndex(t, workspace, interactive.Index{Version: storyIndexVersionForTest})
	scope := purposeToolScope(workspace, purposeState{Purpose: PurposeLore}, nil)
	if scope == nil {
		t.Fatal("lore scope is nil")
	}
	if scope.LoreScope != nil {
		t.Fatalf("lore scope = %v, want an unbound editor with no story in play", *scope.LoreScope)
	}
	if len(scope.ConfigResources) != len(gameAssistantResources) {
		t.Fatalf("lore config resources = %v", scope.ConfigResources)
	}
}

func TestRecordedPurposeSurvivesASecondTurn(t *testing.T) {
	root := t.TempDir()
	workspace := purposeTestWorkspace(t)
	store, err := session.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ProjectID: "project-1", SessionID: "session-1", Workspace: workspace, agentKind: agentrun.AgentKindIDE}
	service := &Service{}
	ctx := context.Background()

	first, err := service.resolvePurpose(ctx, binding, sess, TurnPolicy{Purpose: PurposeGameSettings, StoryID: "story-other"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Purpose != PurposeGameSettings || first.StoryID != "story-other" {
		t.Fatalf("first turn purpose = %#v", first)
	}

	// A follow-up cycle (queued, steered, compacted or recovered) carries no
	// policy. It must inherit the surface rather than widen to the editor.
	second, err := service.resolvePurpose(ctx, binding, sess, TurnPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Purpose != PurposeGameSettings || second.StoryID != "story-other" {
		t.Fatalf("follow-up purpose = %#v, want the recorded surface", second)
	}
	scope := purposeToolScope(workspace, second, nil)
	if scope == nil || scope.LoreScope == nil || *scope.LoreScope != "world-other" {
		t.Fatalf("follow-up scope = %#v, want world-other", scope)
	}
}

func TestApplyToolScopeNarrowsTheHostForEveryCycle(t *testing.T) {
	root := t.TempDir()
	workspace := purposeTestWorkspace(t)
	store, err := session.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ProjectID: "project-1", SessionID: "session-1", Workspace: workspace, agentKind: agentrun.AgentKindIDE}
	service := &Service{}
	ctx := context.Background()

	first := agents.AgentHostCapabilities{Interactive: true}
	if err := service.applyToolScope(ctx, binding, sess, TurnPolicy{Purpose: PurposeGameSettings, StoryID: "story-other"}, &first); err != nil {
		t.Fatal(err)
	}
	if first.ToolScope == nil || first.ToolScope.LoreScope == nil || *first.ToolScope.LoreScope != "world-other" {
		t.Fatalf("turn host scope = %#v, want world-other", first.ToolScope)
	}

	// The resumed cycle, compaction, a delegated child and external recovery
	// all pass an empty policy. None of them may widen back to the editor.
	for name, host := range map[string]*agents.AgentHostCapabilities{
		"resumed cycle":     {Interactive: true},
		"compaction":        {Interactive: true},
		"delegated child":   {Interactive: true},
		"external recovery": {Interactive: true},
	} {
		if err := service.applyToolScope(ctx, binding, sess, TurnPolicy{}, host); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if host.ToolScope == nil || host.ToolScope.LoreScope == nil || *host.ToolScope.LoreScope != "world-other" {
			t.Fatalf("%s host scope = %#v, want the recorded world-other", name, host.ToolScope)
		}
	}
}

func TestApplyToolScopeLeavesTheEditorSurfaceWithoutAPurpose(t *testing.T) {
	root := t.TempDir()
	store, err := session.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ProjectID: "project-1", SessionID: "session-1", Workspace: t.TempDir(), agentKind: agentrun.AgentKindIDE}
	host := agents.AgentHostCapabilities{Interactive: true}
	if err := (&Service{}).applyToolScope(context.Background(), binding, sess, TurnPolicy{}, &host); err != nil {
		t.Fatal(err)
	}
	if host.ToolScope != nil {
		t.Fatalf("writing agent host scope = %#v, want the full editor surface", host.ToolScope)
	}
}

func TestRecordPurposeIsIdempotent(t *testing.T) {
	root := t.TempDir()
	store, err := session.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ProjectID: "project-1", SessionID: "session-1", Workspace: t.TempDir(), agentKind: agentrun.AgentKindIDE}
	service := &Service{}
	ctx := context.Background()
	state := purposeState{Purpose: PurposeLore, StoryID: "story-1"}
	for attempt := 0; attempt < 3; attempt++ {
		if err := service.recordPurpose(ctx, binding, sess, state); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	got := service.recordedPurpose(ctx, binding, sess)
	if got != state {
		t.Fatalf("recorded purpose = %#v, want %#v", got, state)
	}
}
