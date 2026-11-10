//go:build !windows

package localfs

import "path/filepath"

// CanonicalPath resolves an existing file or directory to its absolute host path.
// The result is a runtime location, not a persistent ID.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
