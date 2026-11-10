package execution

import (
	"context"
	"errors"

	agentchat "ta/internal/agents/chat"
	agentrun "ta/internal/agents/run"
	"ta/internal/book"

	agent "ta/agent"
)

// ErrCyclePreparationUnavailable means a durable public Agent turn cannot be
// rebound to its Text Adventure product dependencies in the current process.
var ErrCyclePreparationUnavailable = errors.New("Agent execution cycle preparation dependency is unavailable")

// Cycle is the process-local Text Adventure composition for one public Agent cycle.
// Durable recovery stores only stable Definition/host data and rebuilds these
// dependencies through the selected Profile.
type Cycle struct {
	Definition   agent.Definition
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options
	Successor    SuccessorPolicy
}

// CanonicalMessageSource returns the complete model-visible history from the
// product's sole conversation journal before public Run admission.
type CanonicalMessageSource interface {
	CanonicalMessages(context.Context) ([]*agent.Message, error)
}

type SuccessorPolicy func(context.Context, agentrun.OperationID, agentrun.Outcome) error

type CycleRestoreRequest struct {
	Binding          agentrun.RuntimeBinding
	Kind             CommandKind
	CommandID        agentrun.CommandID
	OperationID      agentrun.OperationID
	AfterOperationID agentrun.OperationID
	Request          agentchat.ChatRequest
	Options          agentrun.Options
	Deferred         bool
	Emit             func(agentrun.Event)
}

func emitExecutionError(emit func(agentrun.Event), err error) {
	if emit != nil && err != nil {
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
	}
}
