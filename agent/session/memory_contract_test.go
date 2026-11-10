package session_test

import (
	"testing"

	"ta/agent/session"
	"ta/agent/session/sessiontest"
)

func TestMemoryStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(testing.TB) session.Store {
		return session.Memory()
	})
}
