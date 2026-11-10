//go:build !windows

package localfs

func retryableReplaceError(error) bool { return false }
