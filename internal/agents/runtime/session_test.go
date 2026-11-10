package agentruntime

import (
	"context"
	"errors"
	"testing"

	"ta/config"
	"ta/internal/agents/conversationconfig"
	"ta/internal/agents/execution"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/session"
	agent "ta/agent"
)

func TestSessionGoalIsRefusedAcrossRuntimeSelections(t *testing.T) {
	for _, kind := range []config.RuntimeID{config.RuntimeNative, config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(kind), func(t *testing.T) {
			store, err := session.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			selection := config.RuntimeSelection{Kind: kind}
			if kind == config.RuntimeCodex {
				selection.Codex = &config.CodexRuntimeSettings{Model: "fixture"}
			}
			if kind == config.RuntimeClaude {
				selection.Claude = &config.ClaudeRuntimeSettings{Model: "fixture"}
			}
			sess, err := store.GetOrCreateWithRuntimeConfig("session", conversationconfig.Config{AgentKind: config.AgentKindIDE, ProfileID: "fixture", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalWrite, Runtime: &selection})
			if err != nil {
				t.Fatal(err)
			}
			native := execution.NewEphemeralRuntime()
			defer native.Close(context.Background())
			engines := NewEngines()
			defer engines.Close()
			options := agentrun.Options{ProjectID: "project", Workspace: t.TempDir(), AgentKind: config.AgentKindIDE, SessionID: sess.ID, Mode: "ide"}
			bound, err := engines.ConversationSession(native, options, sess, selection)
			if err != nil {
				t.Fatal(err)
			}
			if _, present, err := bound.Goal(t.Context()); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) || present {
				t.Fatalf("Goal read was not refused: present=%v err=%v", present, err)
			}
			if _, err := bound.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Finish and verify"}); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
				t.Fatalf("Goal mutation was not refused: %v", err)
			}
			if _, err := bound.Submit(t.Context(), Command{Kind: execution.CommandKind("unknown")}, nil); !errors.Is(err, agentrun.ErrInvalidCommand) {
				t.Fatalf("invalid command accepted: %v", err)
			}
			status, err := bound.Status(t.Context())
			if err != nil || status.Phase != agentrun.RunPhaseIdle {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
}
