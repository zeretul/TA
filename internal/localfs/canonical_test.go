package localfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/internal/testfs"
)

func TestCanonicalPathResolvesNestedJunctionAndMissingTarget(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	testfs.DirectoryJunction(t, root, alias)
	// Exercise a path beyond the initial Windows buffer, including non-ASCII names.
	rel := filepath.Join(strings.Repeat("a", 90), strings.Repeat("b", 90), "资料")
	if err := os.MkdirAll(filepath.Join(root, rel), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, rel, "chapter.txt")
	if err := os.WriteFile(file, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{rel, filepath.Join(rel, "chapter.txt")} {
		got, err := CanonicalPath(filepath.Join(alias, suffix))
		if err != nil {
			t.Fatal(err)
		}
		want, err := CanonicalPath(filepath.Join(root, suffix))
		if err != nil || got != want {
			t.Fatalf("canonical alias=%q, want %q: %v", got, want, err)
		}
	}
	if _, err := CanonicalPath(filepath.Join(alias, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing path error=%v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatalf("canonicalization retained an open handle: %v", err)
	}
}
