package agentruntime

import (
	"context"
	"encoding/json"

	"ta/internal/agents/conversationconfig"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/session"
	"ta/internal/interactive"
	agent "ta/agent"
)

// ProductState gives a selected peer runtime access to the same journal and
// capability schemas as Native, without constructing a Native Agent Session.
// Mutation callbacks are pure and run under the product's canonical CAS fence.
type ProductState struct {
	Read   func(context.Context, string) (json.RawMessage, bool, error)
	Update func(context.Context, string, func(json.RawMessage, bool) (json.RawMessage, error)) error
}

func SessionState(options agentrun.Options, sess *session.Session) (ProductState, error) {
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		return ProductState{}, err
	}
	return ProductState{
		Read: func(ctx context.Context, capability string) (json.RawMessage, bool, error) {
			return sess.LoadCapability(ctx, key, capability)
		},
		Update: func(ctx context.Context, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
			return sess.UpdateCapability(ctx, key, capability, update)
		},
	}, nil
}

func GameState(options agentrun.Options, store *interactive.Store) (ProductState, error) {
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		return ProductState{}, err
	}
	return ProductState{
		Read: func(ctx context.Context, capability string) (json.RawMessage, bool, error) {
			return store.LoadCapability(ctx, options.StoryID, key, capability)
		},
		Update: func(ctx context.Context, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
			return store.UpdateCapability(ctx, options.StoryID, key, capability, update)
		},
	}, nil
}

// Goal uses the published Goal state schema, shared at the product boundary.
// Its state machine is independent of which executor supplies the evaluator.
//
// The product itself has no Goal, so every accessor refuses here rather than
// letting a caller reach the stored capability directly. This is the handle the
// external path holds, and it is the last place a Goal could otherwise be read
// or written after the entry points are gone.
func (store ProductState) Goal(context.Context) (agent.GoalState, bool, error) {
	return agent.GoalState{}, false, conversationconfig.ErrRuntimeCapabilityUnsupported
}

func (store ProductState) UpdateGoal(context.Context, agent.GoalMutation) (agent.GoalState, error) {
	return agent.GoalState{}, conversationconfig.ErrRuntimeCapabilityUnsupported
}
