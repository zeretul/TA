package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"ta/config"
	"ta/internal/history"
	"unicode/utf8"
)

func historyAdvanceWorld(ctx context.Context, cfg *config.Config, id, locale string, state history.State) (history.Command, error) {
	input := history.SimulationFacts(state)
	var proposal history.WorldProposal
	var next history.State
	var report history.QuarterReport
	var validationError error
	seenRejected := map[[32]byte]bool{}
	for attempt := 0; ; attempt++ {
		var request any = input
		if attempt > 0 {
			request = struct {
				Facts       any                   `json:"unchanged_opening_facts"`
				Rejected    history.WorldProposal `json:"rejected_proposal"`
				Error       string                `json:"validation_error"`
				Instruction string                `json:"instruction"`
			}{input, proposal, validationError.Error(), "候选未通过一致性检查，尚未结算。只修复错误及其必需的连带更正并返回完整候选；保留其余已核对结果、金额、ID与进度，不要重新随机推演其他部分。不得更改季初事实或删除玩家政令约束。"}
		}
		var err error
		proposal, err = historyGenerateJSON[history.WorldProposal](ctx, cfg, id, "world_simulation", historyWorldQuarterPrompt, locale, request)
		if err != nil {
			return history.Command{}, err
		}
		next, report, validationError = history.PreviewAdvance(state, history.QuarterPlan{World: &proposal})
		if validationError == nil {
			break
		}
		encoded, _ := json.Marshal(proposal)
		fingerprint := sha256.Sum256(encoded)
		slog.WarnContext(ctx, "[history] quarterly candidate rejected", "attempt", attempt+1, "reason", validationError)
		if seenRejected[fingerprint] {
			return history.Command{}, fmt.Errorf("%w: repeated invalid candidate: %v", ErrHistoryModelOutput, validationError)
		}
		seenRejected[fingerprint] = true
	}
	if validationError != nil {
		return history.Command{}, fmt.Errorf("%w: %v", ErrHistoryModelOutput, validationError)
	}
	view := history.PlayerState(next)
	report.World = history.VisibleProposal(proposal, *view.World)
	summary, err := historyGenerateJSON[historyReportPayload](ctx, cfg, id, "world_report", historyWorldReportPrompt, locale, struct {
		Report history.QuarterReport `json:"report"`
		Next   any                   `json:"next_state"`
	}{report, historyCurrentFacts(view)})
	if err != nil {
		return history.Command{}, err
	}
	if strings.TrimSpace(summary.Narrative) == "" || utf8.RuneCountInString(summary.Narrative) > 16000 {
		return history.Command{}, fmt.Errorf("%w: invalid world report", ErrHistoryModelOutput)
	}
	return history.Command{Quarter: &history.QuarterPlan{World: &proposal, Narrative: summary.Narrative}}, nil
}

// DirectEdict interprets the explicit instruction and registers it in the same
// guarded command. It has the same receipts/CAS/cancellation as previewed edicts.
func (s *HistoryAppService) DirectEdict(ctx context.Context, projectID, campaignID string, req HistoryPrepareRequest) (history.Receipt, error) {
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" || utf8.RuneCountInString(req.Text) > 12000 || req.SourceChatID != "" {
		return history.Receipt{}, ErrHistoryInput
	}
	return s.historyMutate(ctx, projectID, campaignID, "edict", req.HistoryCommandRequest, req, func(ctx context.Context, _ *history.Store, cfg config.Config, state history.State) (history.Command, error) {
		if state.World == nil {
			return history.Command{}, ErrHistoryInput
		}
		plan, err := historyPrepareWorldEdict(ctx, &cfg, campaignID, "direct_edict", req.Locale, state, req.Text, nil, nil)
		if err != nil {
			return history.Command{}, err
		}
		plan.Text = req.Text
		plan.SourceChatID = ""
		return history.Command{Edict: &plan}, nil
	})
}

// The two decree entry points share model correction before preview or commit.
// Facts and selected discussion remain the unchanged prefix of every attempt.
func historyPrepareWorldEdict(ctx context.Context, cfg *config.Config, campaignID, purpose, locale string, state history.State, text string, source *history.ChatTurn, advice *history.SavedAdvice) (history.EdictPlan, error) {
	var rejected *history.EdictPlan
	var outcomes []history.Outcome
	seen := map[[32]byte]bool{}
	for attempt := 0; ; attempt++ {
		request := struct {
			State    any                  `json:"current_state"`
			Text     string               `json:"edict"`
			Source   *history.ChatTurn    `json:"source_discussion,omitempty"`
			Advice   *history.SavedAdvice `json:"selected_advice,omitempty"`
			Rejected *history.EdictPlan   `json:"rejected_plan,omitempty"`
			Outcomes []history.Outcome    `json:"validation_outcomes,omitempty"`
		}{historyCurrentFacts(state), text, source, advice, rejected, outcomes}
		plan, err := historyGenerateJSON[history.EdictPlan](ctx, cfg, campaignID, purpose, historyWorldEdictPrompt, locale, request)
		if err != nil {
			return history.EdictPlan{}, err
		}
		plan.Text = text
		plan.SourceChatID = ""
		if source != nil {
			plan.SourceChatID = source.ID
		}
		outcomes = history.ValidatePlan(state, plan)
		if len(plan.Orders) < 1 || len(plan.Orders) > 32 || len(plan.Instructions) > 0 {
			outcomes = []history.Outcome{{Status: "unsupported", Reason: "world decree requires 1 to 32 orders and no legacy instructions"}}
		}
		ready := true
		for _, outcome := range outcomes {
			if outcome.Status != "ready" {
				ready = false
			}
		}
		if ready {
			return plan, nil
		}
		encoded, _ := json.Marshal(plan)
		fingerprint := sha256.Sum256(encoded)
		if seen[fingerprint] {
			return history.EdictPlan{}, fmt.Errorf("%w: repeated invalid decree: %+v", ErrHistoryModelOutput, outcomes)
		}
		seen[fingerprint] = true
		slog.WarnContext(ctx, "[history] decree candidate rejected", "attempt", attempt+1, "outcomes", outcomes)
		rejected = &plan
	}
}
