// Package file provides Agent's built-in transcript Store. It keeps one
// checksummed append-only record stream and one exclusive lease per Session.
package file

import (
	sessionfile "ta/agent/internal/sessionfile"
	"ta/agent/session"
)

type Store = sessionfile.Store

func New(root string) (*Store, error) {
	return sessionfile.New(root)
}

var _ session.Store = (*Store)(nil)
