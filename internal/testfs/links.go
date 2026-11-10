// Package testfs creates real filesystem link fixtures for integration tests.
// Production packages must not use these testing helpers.
package testfs

import (
	"os"
	"testing"
)

// Symlink skips only the known Windows privilege error; other creation failures fail the test.
func Symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if symlinkPrivilegeUnavailable(err) {
			t.Skipf("Windows symbolic-link privilege unavailable: %v", err)
		}
		t.Fatalf("create symbolic link: %v", err)
	}
	removeLinkOnCleanup(t, link)
}

func removeLinkOnCleanup(t *testing.T, link string) {
	t.Helper()
	// Remove the link itself before TempDir cleanup; never recursively remove its target.
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove link fixture: %v", err)
		}
	})
}
