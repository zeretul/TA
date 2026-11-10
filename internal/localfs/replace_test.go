package localfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFilePreservesDestinationUntilReplacement(t *testing.T) {
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "candidate"), filepath.Join(dir, "current")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(source, destination); err == nil {
		t.Fatal("missing source unexpectedly replaced destination")
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "old" {
		t.Fatalf("failed replace changed destination: %q, %v", data, err)
	}
	if err := os.WriteFile(source, []byte("complete new value"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(source, destination); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "complete new value" {
		t.Fatalf("wrong replacement: %q, %v", data, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source retained after rename: %v", err)
	}
}
