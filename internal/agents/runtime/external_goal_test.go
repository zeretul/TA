package agentruntime

import (
	"context"
	"errors"
	"testing"

	"ta/config"
	"ta/internal/agents/conversationconfig"
	"ta/internal/agents/runtime/external"
	agent "ta/agent"
)

type controlAdapter func(context.Context, external.Input, external.Host) (external.Result, error)

func (controlAdapter) Version() string { return "control-fixture-1" }
func (adapter controlAdapter) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	return adapter(ctx, input, host)
}

// The product has no Goal. Reading it and mutating it must both refuse, for the
// state handle the external path holds as well as for the session handle.
func TestProductStateRefusesGoal(t *testing.T) {
	_, _, state, _, _ := controlFixture(t)

	if _, present, err := state.Goal(t.Context()); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) || present {
		t.Fatalf("Goal read was not refused: present=%v err=%v", present, err)
	}
	if _, err := state.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Finish and verify the draft"}); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
		t.Fatalf("Goal mutation was not refused: %v", err)
	}
}

func TestEnginesAdvertiseNoGoalCapability(t *testing.T) {
	for _, id := range []config.RuntimeID{config.RuntimeNative, config.RuntimeCodex, config.RuntimeClaude} {
		for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral} {
			if engineDescriptor(id).ForAgent(kind).Goal {
				t.Fatalf("%s advertises Goal for agent kind %q", id, kind)
			}
		}
	}
}
