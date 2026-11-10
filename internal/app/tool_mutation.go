package app

import (
	"context"

	agenttoolruntime "ta/internal/agents/toolruntime"
	agenttool "ta/internal/agents/tool"
	"ta/internal/book"
)

// acceptToolMutation satisfies the Agent runtime's required Tool mutation
// destination. A tool commits its own workspace mutation before the effect is
// reported, so there is no second durable write to make here; the outbox this
// replaced existed only to wake the automation trigger scheduler.
func acceptToolMutation(context.Context, agenttoolruntime.CommittedToolMutation) error {
	return nil
}

// verifiedWorkspaceMutationCallback schedules versioning only after the same
// verified mutation event reached the output commit receipt.
func (a *App) verifiedWorkspaceMutationCallback(
	_ string,
	versionService *book.VersionService,
	settings book.VersionAutoSettings,
) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	return func(ctx context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		_ = ctx
		_ = verification
		if len(mutations) > 0 {
			scheduleAutoVersion(versionService, settings)
		}
	}
}
