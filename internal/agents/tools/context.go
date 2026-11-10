package tools

import (
	"context"

	"ta/internal/agents/configresource"
	"ta/internal/interactive"
)

// InteractiveContext is the concrete tools' story-scoped dependency set.
// Agent lifecycle, conversation, and durable runtime collaborators are
// intentionally excluded; the agents package owns those concerns.
type InteractiveContext struct {
	// ObserveLoreRead records successful scoped body reads for opening-schema evidence.
	// The callback receives backend IDs, never model-claimed read receipts.
	ObserveLoreRead func([]string)
	Store           *interactive.Store
	StoryID         string
	BranchID        string
	// LoreScope is the world resource scope this story's lore reads and writes
	// are bound to, taken from the story's pack binding.
	LoreScope string
	// EventPackages are the event cards this story's bound modules resolve to.
	EventPackages          []interactive.EventPackage
	MaxResultBytes         int
	SubmitStateSchemaBatch func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	RequestTurnCompletion  func(context.Context) bool
	PrepareTurn            func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error)
	SelectStoryProtagonist func(context.Context, string) (interactive.StoryProtagonist, error)
	SubmitTurnResult       func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
	// ResetNarrativeCandidate discards the run's retained prose after the
	// backend refused the world action it narrated. It is nil in contexts with
	// no turn protocol middleware, where there is nothing retained to drop.
	ResetNarrativeCandidate func(context.Context)
}

// IDEContext binds the Writing Agent's product tools to one editable world.
// The zero value keeps the unfiltered editor behaviour that sees every world.
type IDEContext struct {
	// LoreScope binds lore reads and writes to exactly this world. A pointer
	// because the empty scope is meaningful: nil is an unbound editor, while a
	// pointer to "" binds a world with no pack binding to the shared entries.
	LoreScope *string
	// ConfigResources limits the configuration registry to these resource
	// names. Nil or empty keeps every resource.
	ConfigResources []string
	// SettingsTarget binds configuration writes to one story and one world.
	// When set, the shared preset libraries become read-only templates and the
	// only writable configuration is that story's own settings and that world's
	// new-game defaults. Nil keeps the editor's in-place preset editing.
	SettingsTarget *configresource.ScopedTarget
}
