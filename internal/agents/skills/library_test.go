package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedLibraryOptInAppliesToAllAgentPaths(t *testing.T) {
	ctx := context.Background()
	home, data, builtin := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	shared := filepath.Join(home, ".agents", "skills")
	writeSkillFile(t, shared, "shared-only", "shared-only", "Shared workflow")
	writeSkillFile(t, shared, "collision", "collision", "Shared version")
	writeSkillFile(t, builtin, "collision", "collision", "Builtin version")
	dirs := NewDirectories(builtin, data, "")
	snapshot, err := SnapshotFor(ctx, dirs)
	if err != nil || len(snapshot.Skills) != 3 || snapshot.SharedEnabled {
		t.Fatalf("unexpected library: %+v %v", snapshot, err)
	}
	for _, kind := range []string{"ide", "general", "interactive_story"} {
		backend := NewAgentBackend(dirs, kind, map[string]bool{"shared-only": true})
		if _, err := backend.Get(ctx, "shared-only"); err == nil {
			t.Fatalf("%s loaded shared Skill without opt-in", kind)
		}
		if got := backend.ResolveExplicitInvocations(ctx, "/shared-only"); len(got) != 0 {
			t.Fatalf("%s resolved disabled slash command", kind)
		}
	}
	if err := SetSharedEnabled(ctx, dirs, true); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ide", "general", "interactive_story"} {
		backend := NewAgentBackend(dirs, kind, nil)
		if _, err := backend.Get(ctx, "shared-only"); err != nil {
			t.Fatal(err)
		}
		got, err := backend.Get(ctx, "collision")
		if err != nil || got.Description != "Builtin version" {
			t.Fatalf("precedence: %+v %v", got, err)
		}
	}
	if err := SetLibraryEnabled(ctx, dirs, ScopeShared, "shared-only", false); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentBackend(NewDirectories(builtin, data, ""), "ide", map[string]bool{"shared-only": true}).Get(ctx, "shared-only"); err == nil {
		t.Fatal("Agent override bypassed library disable after reopen")
	}
	if _, err := SaveDocument(ctx, dirs, ScopeShared, "shared-only", "changed"); err == nil {
		t.Fatal("shared library is writable")
	}
	entries, err := os.ReadDir(shared)
	if err != nil || len(entries) != 2 {
		t.Fatalf("The product wrote into shared directory: %v %v", entries, err)
	}
}

func TestLegacyLibrarySettingsAreReadThenReplaced(t *testing.T) {
	ctx := context.Background()
	dirs := []Directory{{Scope: ScopeUser, Path: t.TempDir(), Writable: true}}
	root := dirs[0].Path
	dirs[0].settingsRoot = root
	legacy := filepath.Join(root, legacyLibrarySettingsFile)
	if err := os.WriteFile(legacy, []byte(`{"shared_enabled":true,"disabled":{"user:old":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, root, "old", "old", "Old skill")

	settings, err := readLibrarySettings(root)
	if err != nil || !settings.SharedEnabled || !settings.Disabled["user:old"] {
		t.Fatalf("legacy settings were not read: %+v %v", settings, err)
	}

	if err := SetLibraryEnabled(ctx, dirs, ScopeUser, "old", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy settings file survived the first write: %v", err)
	}
	settings, err = readLibrarySettings(root)
	if err != nil || !settings.SharedEnabled || settings.Disabled["user:old"] {
		t.Fatalf("settings were not carried over: %+v %v", settings, err)
	}
}

func TestSharedSymlinkReadIsBoundedAndReadOnly(t *testing.T) {
	home, data, source := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeSkillFile(t, source, "linked", "different-name", "Linked skill")
	shared := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "linked"), filepath.Join(shared, "linked")); err != nil {
		t.Skip(err)
	}
	dirs := NewDirectories("", data, "")
	doc, err := ReadDocument(context.Background(), dirs, ScopeShared, "different-name")
	if err != nil || doc.Editable || doc.Enabled {
		t.Fatalf("linked document: %+v %v", doc, err)
	}
}
