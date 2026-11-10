package interactive

import (
	"context"
	"testing"

	agent "ta/agent"
)

type recordingChatModel struct {
	options *agent.Options
}

func (m *recordingChatModel) Generate(_ context.Context, _ []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	m.options = agent.GetCommonOptions(nil, options...)
	return &agent.Message{Role: agent.Assistant, ToolCalls: []agent.ToolCall{{ID: "unexpected"}}}, nil
}

func (*recordingChatModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return nil, nil
}

func TestAcceptedTurnSuppressesProviderToolCallAndKeepsAcceptedNarrative(t *testing.T) {
	middleware := &TurnProtocolMiddleware{
		ready:         func() bool { return true },
		loadNarrative: func(context.Context) (string, error) { return "The accepted scene remains canonical.", nil },
	}
	base := &recordingChatModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := wrapped.Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if base.options == nil || base.options.ToolChoice == nil || *base.options.ToolChoice != agent.ToolChoiceForbidden {
		t.Fatalf("narrative-only request did not forbid tools: %+v", base.options)
	}
	state := &agent.RunState{Messages: []*agent.Message{response}}
	_, state, err = middleware.AfterModelRewriteState(context.Background(), state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Messages[0].ToolCalls; len(got) != 0 {
		t.Fatalf("provider-forbidden tools survived the dispatch boundary: %+v", got)
	}
	if got := state.Messages[0].Content; got != "The accepted scene remains canonical." {
		t.Fatalf("transcript content = %q, want the accepted narrative", got)
	}
}

func TestAcceptedTurnCannotPublishUnretainedProviderProseAfterForbiddenTool(t *testing.T) {
	for _, name := range []string{"missing loader", "empty retained narrative"} {
		t.Run(name, func(t *testing.T) {
			middleware := &TurnProtocolMiddleware{ready: func() bool { return true }}
			if name == "empty retained narrative" {
				middleware.loadNarrative = func(context.Context) (string, error) { return " ", nil }
			}
			state := &agent.RunState{Messages: []*agent.Message{{
				Role: agent.Assistant, Content: "An unaccepted replacement scene.",
				ToolCalls: []agent.ToolCall{{ID: "unexpected"}},
			}}}
			if _, _, err := middleware.AfterModelRewriteState(context.Background(), state, nil); err == nil {
				t.Fatal("provider tool misuse published prose without an accepted narrative")
			}
		})
	}
}
