package interactive

import (
	"context"

	story "ta/internal/interactive"
)

// InteractiveStoryToolContext is the application-facing input used to build
// the Game Agent tool surface. Runtime-only collaborators stay here in
// the agents package; concrete tools receive a narrower projection so the
// tools package does not depend on agent orchestration internals.
type InteractiveStoryToolContext struct {
	// ObserveLoreRead records successful scoped body reads for opening-schema evidence.
	// The callback receives backend IDs, never model-claimed read receipts.
	ObserveLoreRead func([]string)
	Store           *story.Store
	StoryID         string
	BranchID        string
	// LoreScope is the world resource scope this story's lore reads and writes
	// are bound to. It comes from the story's pack binding, so a story never
	// reads another world's book through its tools.
	LoreScope string
	// EventPackages are the event cards this story's bound modules resolve to.
	// They reach the model as a directory in the runtime context plus a full
	// read tool, independent of whether branch planning is enabled.
	EventPackages          []story.EventPackage
	SubmitStateSchemaBatch func(
		context.Context,
		story.ActorStateSchemaBatch,
	) (story.ActorStateSchemaBatchResult, error)
	PrepareTurn              func(context.Context, story.TurnCheckRequest) (story.RuleResolution, error)
	SelectProtagonist        func(context.Context, string) (story.StoryProtagonist, error)
	SubmitTurnResult         func(context.Context, story.TurnSubmissionInput) (story.TurnSubmissionReceipt, error)
	TurnResultReady          func() bool
	LoadNarrativeCandidate   func(context.Context) (string, error)
	AcceptNarrativeCandidate func(context.Context, string) error
	// ResetNarrativeCandidate discards the run's retained prose after the
	// backend refused the world action that prose narrated. The next model
	// output is then accepted as the replacement narrative instead of being
	// ignored as the already-locked first candidate.
	ResetNarrativeCandidate func(context.Context)
}
