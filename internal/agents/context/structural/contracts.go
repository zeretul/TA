// Package structural defines Text Adventure's small product-facing command vocabulary
// for manual Agent context mutations. The public Agent Session owns admission,
// execution, persistence, and recovery.
package structural

import (
	"ta/internal/agents/context/compaction"
	agentrun "ta/internal/agents/run"
)

// Action is closed because every manual structural command must map to one
// public Agent Session operation.
type Action string

const (
	Compact Action = "compact_context"
	Remove  Action = "remove_compaction"
)

type Result struct {
	Compaction compaction.Result
	Removed    bool
}

// Spec is a Text Adventure adapter request. It deliberately contains no frozen
// product-store mutation or runtime restore descriptor: the selected public
// Agent Session is the sole durable authority for the command.
type Spec struct {
	CommandID string
	Action    Action
	Ref       agentrun.ContextCompactionRef
}
