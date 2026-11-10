//go:build !windows

package testfs

import "testing"

func symlinkPrivilegeUnavailable(error) bool { return false }
func DirectoryJunction(t *testing.T, target, link string) {
	t.Helper()
	t.Skip("directory junctions are Windows-specific; symbolic-link cases cover this platform")
}
