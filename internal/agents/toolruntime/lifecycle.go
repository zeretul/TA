package toolruntime

import (
	"context"

	agentrun "ta/internal/agents/run"
	agenttool "ta/internal/agents/tool"
)

// recordToolExecution feeds bounded product diagnostics. Canonical Tool
// mutations are committed separately through Agent effects.
func recordToolExecution(ctx context.Context, record agenttool.ExecutionRecord) {
	if observer := agentrun.ObserverFromContext(ctx); observer != nil {
		observer.RecordToolExecution(record)
	}
}
