package interactive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	agent "ta/agent"

	producttools "ta/internal/agents/tools"
)

const (
	interactiveTurnSubmissionToolName = producttools.SubmitInteractiveTurnToolName
	legacyActorStatePatchesToolName   = "submit_actor_state_patches"
	legacyInteractiveChoicesToolName  = "submit_choices"
	interactiveCompletionRetryCode    = "interactive_turn_result_missing"
	interactiveRetryDraftMaxBytes     = 16 * 1024
	interactiveRetryFeedbackMaxBytes  = 1024
	interactiveRetryCandidatePrefix   = "[Retained narrative candidate; source=first accepted model prose;"
	interactiveRetryFeedbackPrefix    = "[Interactive turn protocol feedback; source=backend completion guard]"
)

const (
	CompletionRetryCode    = interactiveCompletionRetryCode
	TurnSubmissionToolName = interactiveTurnSubmissionToolName
)

type interactiveTurnProtocolStateKey struct{}

type interactiveTurnProtocolRunState struct {
	narrativeCandidateReady atomic.Bool
	mu                      sync.Mutex
	narrativeCandidate      string
	restored                bool
	// rewriteRequired records that the backend discarded the retained prose
	// because the world refused the action it narrated. The candidate is empty
	// again on purpose: the turn needs new prose, not a completion reminder.
	rewriteRequired bool
}

func (s *interactiveTurnProtocolRunState) retainNarrativeCandidate(content string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.narrativeCandidate == "" && strings.TrimSpace(content) != "" {
		s.narrativeCandidate = content
		s.narrativeCandidateReady.Store(true)
	}
	return s.narrativeCandidate
}

func (s *interactiveTurnProtocolRunState) retainedNarrativeCandidate() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.narrativeCandidate
}

// discardNarrativeCandidate drops the retained prose and marks the turn as one
// that has to be rewritten.
//
// Without this the native path deadlocks after a refusal: ReviewModelOutput
// only accepts prose while no candidate is retained, so the model's new
// narrative would be ignored, the draft would stay empty, and the turn could
// never become ready. The refused text must stop being "the locked first
// candidate" the moment the world refuses what it narrates.
func (s *interactiveTurnProtocolRunState) discardNarrativeCandidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.narrativeCandidate = ""
	s.narrativeCandidateReady.Store(false)
	s.rewriteRequired = true
}

func (s *interactiveTurnProtocolRunState) rewritePending() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rewriteRequired && s.narrativeCandidate == ""
}

func interactiveTurnProtocolState(ctx context.Context) *interactiveTurnProtocolRunState {
	state, _ := ctx.Value(interactiveTurnProtocolStateKey{}).(*interactiveTurnProtocolRunState)
	return state
}

func RequestTurnCompletion(ctx context.Context) bool {
	state := interactiveTurnProtocolState(ctx)
	if state == nil || !state.narrativeCandidateReady.Load() {
		return false
	}
	return agent.RequestCompletionAfterTools(ctx)
}

// RequestNarrativeRewrite is called when the backend refused the world action a
// submitted turn narrated. The retained prose is dropped and the turn is marked
// for a rewrite, so the model's next narrative is accepted instead of being
// ignored as "the locked first candidate".
func RequestNarrativeRewrite(ctx context.Context) {
	if state := interactiveTurnProtocolState(ctx); state != nil {
		state.discardNarrativeCandidate()
	}
}

// TurnProtocolMiddleware keeps the tool schema stable for prompt
// caching and provides a narrative-only fallback when a model submits before
// producing a prose candidate.
type TurnProtocolMiddleware struct {
	*agent.BaseMiddleware
	ready           func() bool
	loadNarrative   func(context.Context) (string, error)
	acceptNarrative func(context.Context, string) error
}

func NewTurnProtocolMiddleware(config InteractiveStoryToolContext) *TurnProtocolMiddleware {
	return &TurnProtocolMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{},
		ready:          config.TurnResultReady, loadNarrative: config.LoadNarrativeCandidate, acceptNarrative: config.AcceptNarrativeCandidate,
	}
}

func (m *TurnProtocolMiddleware) BeforeAgent(ctx context.Context, runCtx *agent.RunContext) (context.Context, *agent.RunContext, error) {
	state := &interactiveTurnProtocolRunState{}
	if m.loadNarrative != nil {
		narrative, err := m.loadNarrative(ctx)
		if err != nil {
			return ctx, runCtx, err
		}
		state.retainNarrativeCandidate(narrative)
		state.restored = narrative != ""
	}
	return context.WithValue(ctx, interactiveTurnProtocolStateKey{}, state), runCtx, nil
}

func (m *TurnProtocolMiddleware) BeforeModelRewriteState(ctx context.Context, state *agent.RunState, model *agent.ModelContext) (context.Context, *agent.RunState, error) {
	progress := interactiveTurnProtocolState(ctx)
	if progress == nil || !progress.restored || model.Iteration != 0 || (m.ready != nil && m.ready()) {
		return ctx, state, nil
	}
	// The canonical draft stays complete in Story storage. Only this explicitly
	// attributed request-local reminder uses the bounded feedback projection.
	for _, fragment := range interactiveProtocolFeedback(progress.retainedNarrativeCandidate(), progress.rewritePending()) {
		state.Messages = append(state.Messages, &agent.Message{Role: fragment.Role, Content: fragment.Content})
	}
	return ctx, state, nil
}

func (m *TurnProtocolMiddleware) WrapModel(_ context.Context, wrapped agent.BaseChatModel, _ *agent.ModelContext) (agent.BaseChatModel, error) {
	if m == nil || m.ready == nil || !m.ready() {
		return wrapped, nil
	}
	return &interactiveNarrativeOnlyModel{BaseChatModel: wrapped}, nil
}

func (m *TurnProtocolMiddleware) AfterModelRewriteState(ctx context.Context, state *agent.RunState, _ *agent.ModelContext) (context.Context, *agent.RunState, error) {
	if m == nil || m.ready == nil || !m.ready() || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last != nil && len(last.ToolCalls) > 0 {
		// Some providers still return a tool call despite tool_choice=none. The
		// accepted TurnResult is already canonical, so discard the invalid calls
		// before the loop can dispatch them and finish with its retained prose.
		slog.WarnContext(ctx, "discarding provider tool calls after turn acceptance", "tool_call_count", len(last.ToolCalls))
		last.ToolCalls = nil
		narrative := ""
		if m.loadNarrative != nil {
			var err error
			narrative, err = m.loadNarrative(ctx)
			if err != nil {
				return ctx, state, fmt.Errorf("load accepted narrative after forbidden tool call: %w", err)
			}
		}
		if strings.TrimSpace(narrative) == "" {
			return ctx, state, fmt.Errorf("cannot recover accepted narrative after forbidden provider tool calls")
		}
		last.Content = narrative
	}
	return ctx, state, nil
}

type interactiveNarrativeOnlyModel struct {
	agent.BaseChatModel
}

func (m *interactiveNarrativeOnlyModel) Generate(ctx context.Context, messages []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	narrativeOpts := append([]agent.ModelOption(nil), opts...)
	narrativeOpts = append(narrativeOpts, agent.WithToolChoice(agent.ToolChoiceForbidden))
	return m.BaseChatModel.Generate(ctx, messages, narrativeOpts...)
}

func (m *interactiveNarrativeOnlyModel) Stream(ctx context.Context, messages []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	narrativeOpts := append([]agent.ModelOption(nil), opts...)
	narrativeOpts = append(narrativeOpts, agent.WithToolChoice(agent.ToolChoiceForbidden))
	return m.BaseChatModel.Stream(ctx, messages, narrativeOpts...)
}

// ReviewModelOutput retains complete prose and asks for missing modules using
// bounded feedback. Network failures never enter this product acceptance seam.
func (m *TurnProtocolMiddleware) ReviewModelOutput(ctx context.Context, output agent.ModelOutput) (agent.ModelOutputReview, error) {
	state := interactiveTurnProtocolState(ctx)
	if interactiveOutputContainsNarrativeCandidate(output.Message) && state != nil {
		if state.retainedNarrativeCandidate() == "" && m.acceptNarrative != nil {
			if err := m.acceptNarrative(ctx, output.Message.Content); err != nil {
				return agent.ModelOutputReview{}, err
			}
		}
		state.retainNarrativeCandidate(output.Message.Content)
	}
	if m.ready == nil || m.ready() {
		return agent.ModelOutputReview{Action: agent.ModelOutputAccept}, nil
	}
	if output.Message != nil && len(output.Message.ToolCalls) > 0 {
		return agent.ModelOutputReview{Action: agent.ModelOutputAccept}, nil
	}

	candidate := ""
	rewrite := false
	if state != nil {
		candidate = state.retainedNarrativeCandidate()
		rewrite = state.rewritePending()
	}
	return agent.ModelOutputReview{Action: agent.ModelOutputRepair, Feedback: interactiveProtocolFeedback(candidate, rewrite), Reason: interactiveCompletionRetryCode}, nil
}

func interactiveProtocolFeedback(candidate string, rewrite bool) []agent.ContextFragment {
	var fragments []agent.ContextFragment
	if strings.TrimSpace(candidate) != "" {
		draft := truncateUTF8StringBytes(candidate, interactiveRetryDraftMaxBytes)
		fragments = append(fragments, agent.ContextFragment{
			Source: "interactive_turn_protocol", Purpose: "retained_narrative", Role: agent.Assistant,
			HardLimit: interactiveRetryDraftMaxBytes,
			Content: truncateUTF8StringBytes(fmt.Sprintf(
				"%s limit=%d bytes]\n%s",
				interactiveRetryCandidatePrefix,
				interactiveRetryDraftMaxBytes,
				draft,
			), interactiveRetryDraftMaxBytes)})
	}
	if rewrite {
		fragments = append(fragments, agent.ContextFragment{
			Source: "interactive_turn_protocol", Purpose: "narrative_rewrite", Role: agent.User,
			Content: truncateUTF8StringBytes(strings.Join([]string{
				interactiveRetryFeedbackPrefix,
				"上一版正文已作废。请依据最近的规则回执重写完整正文，准确交代实际结果。",
				"保留已接受的结算，不能额外增加生效条件；被拒绝的行动不能写成成功。",
				"补交尚未接受的模块。只有在本回合确无正式行动时才提交 world.action=none。",
			}, "\n"), interactiveRetryFeedbackMaxBytes),
			HardLimit: interactiveRetryFeedbackMaxBytes,
		})
		return fragments
	}
	feedback := truncateUTF8StringBytes(strings.Join([]string{
		interactiveRetryFeedbackPrefix,
		"正文已显示并锁定。只调用 submit_interactive_turn 补交，不复述正文。必填 state_changes、choices、world；按本局规则与玩家行动结算，不能只改文字。",
		"等待用 world.action=wait、duration_seconds、source_text；1小时/钟头=3600秒，1时辰=7200秒。",
		"武侠/星海/西幻的买卖、学练、休息、装备、制作、部署、收回、维修、开战统一用 world.action=adventure，填写 adventure 与 source_text；不填凡人顶层 rest/items/craft/skills/cultivation。",
		"凡人按目录使用上述顶层模块。行路journey、事务quest、事件public_event、探索realm也属world。人物事实用character_facts。纯闲谈观察才none。",
		"只补retry_modules；没有回执则完整提交。ready=true后不重复或改写。",
	}, "\n"), interactiveRetryFeedbackMaxBytes)
	fragments = append(fragments, agent.ContextFragment{
		Source: "interactive_turn_protocol", Purpose: "missing_modules", Role: agent.User,
		Content: feedback, HardLimit: interactiveRetryFeedbackMaxBytes,
	})
	return fragments
}

func interactiveOutputContainsNarrativeCandidate(message *agent.Message) bool {
	if message == nil || strings.TrimSpace(message.Content) == "" {
		return false
	}
	for _, call := range message.ToolCalls {
		if !IsInteractiveTurnSubmissionTool(call.Function.Name) {
			return false
		}
	}
	return true
}

// IsInteractiveTurnSubmissionTool reports whether the tool finalizes the
// current interactive turn. Submission tool calls always come after the
// narrative prose, so they anchor the narrative position in display events.
func IsInteractiveTurnSubmissionTool(name string) bool {
	switch strings.TrimSpace(name) {
	case interactiveTurnSubmissionToolName, legacyActorStatePatchesToolName, legacyInteractiveChoicesToolName:
		return true
	default:
		return false
	}
}

func truncateUTF8StringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && (value[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}
