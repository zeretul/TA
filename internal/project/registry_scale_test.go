package project

import (
	"fmt"
	"os"
	"path/filepath"
	"ta/internal/testfs"
	"testing"
)

// BenchmarkRegistryListAtScale measures one registry read against a populated
// data directory.
//
// Every registry read discovers content directories, and discovery used to
// canonicalise each candidate and probe seven markers inside it, registered
// Projects included. That cost was paid per read, so a caller asking about
// every Project in turn paid it once per Project. The Automations view does
// exactly that, and on a machine with 86 Projects the round trip exceeded the
// browser suite's assertion window. The benchmark records the per-read cost the
// fix had to bring down: 407ms before, 7ms after.
func BenchmarkRegistryListAtScale(b *testing.B) {
	denovaDir := b.TempDir()
	for index := 0; index < 86; index++ {
		workspace := filepath.Join(denovaDir, ContentDirectoryName, fmt.Sprintf("Book %d", index))
		if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "book.json"), []byte("{}"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	registry := NewRegistry(denovaDir)
	if _, err := registry.List(false); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		records, err := registry.List(false)
		if err != nil {
			b.Fatal(err)
		}
		if len(records) != 86 {
			b.Fatalf("registry = %d records, want 86", len(records))
		}
	}
}

// A directory the user created outside the application must still be adopted.
//
// Skipping the registered names is only safe if it skips exactly the candidates
// that would have been recognized as already known. A new content directory has
// a name no Project records, so it must still reach the full check and be
// registered; if the skip were too broad it would silently stop adopting, and
// the book would disappear from the application.
func TestRegistryStillDiscoversADirectoryCreatedOutsideTheApplication(t *testing.T) {
	denovaDir := t.TempDir()
	registered := filepath.Join(denovaDir, ContentDirectoryName, "Existing Book")
	if err := os.MkdirAll(filepath.Join(registered, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(denovaDir)
	if _, err := registry.List(false); err != nil {
		t.Fatal(err)
	}

	adopted := filepath.Join(denovaDir, ContentDirectoryName, "Hand Made Book")
	if err := os.MkdirAll(filepath.Join(adopted, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	projects, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %#v, want the hand-made directory adopted alongside the registered one", projects)
	}
	found := false
	for _, project := range projects {
		if project.Name == "Hand Made Book" {
			found = true
		}
	}
	if !found {
		t.Fatalf("projects = %#v, want the hand-made directory adopted", projects)
	}
}

// Managed content discovery must not import a symlink as portable managed
// data. An explicitly registered external target still resolves canonically.

func TestRegistryDiscoveryRejectsManagedSymlinkAndKeepsExplicitExternalTarget(t *testing.T) {
	checkManagedDirectoryLink(t, testfs.Symlink)
}
func TestRegistryDiscoveryRejectsManagedJunction(t *testing.T) {
	checkManagedDirectoryLink(t, testfs.DirectoryJunction)
}
func checkManagedDirectoryLink(t *testing.T, createLink func(*testing.T, string, string)) {
	t.Helper()
	denovaDir := t.TempDir()
	real := filepath.Join(t.TempDir(), "Real Book")
	if err := os.MkdirAll(filepath.Join(real, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(denovaDir, ContentDirectoryName, "Alias Book")
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		t.Fatal(err)
	}
	createLink(t, real, alias)
	registry := NewRegistry(denovaDir)
	projects, err := registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("managed symlink was silently imported: %#v", projects)
	}
	registered, err := registry.Add(real, TypeBook, "Real Book")
	if err != nil {
		t.Fatal(err)
	}
	projects, err = registry.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != registered.ID {
		t.Fatalf("external target duplicated or lost: %#v", projects)
	}
	canonical, err := canonicalDirectory(real, true)
	if err != nil {
		t.Fatal(err)
	}
	if projects[0].WorkspacePath != canonical {
		t.Fatalf("workspace = %q, want the resolved directory %q", projects[0].WorkspacePath, canonical)
	}
}
