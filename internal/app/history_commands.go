package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"ta/config"
	"ta/internal/history"
)

type HistoryCommandRequest struct {
	CommandID        string `json:"command_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Locale           string `json:"locale,omitempty"`
}

type HistoryChatRequest struct {
	SourceReportID string `json:"source_report_id,omitempty"`
	HistoryCommandRequest
	MinisterIDs []string `json:"minister_ids"`
	Message     string   `json:"message"`
}

// The public court roster contains identities and offices, never another
// minister's private context, capability estimate, memory or assigned secrets.
type historyPublicMinister struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Office string `json:"office"`
}

type HistoryPrepareRequest struct {
	HistoryCommandRequest
	Text           string `json:"text"`
	SourceChatID   string `json:"source_chat_id,omitempty"`
	SourceAdviceID string `json:"source_advice_id,omitempty"`
}

type HistoryPreparedEdict struct {
	ExpectedRevision uint64            `json:"expected_revision"`
	Plan             history.EdictPlan `json:"plan"`
	Outcomes         []history.Outcome `json:"outcomes"`
}

type HistoryEdictRequest struct {
	HistoryCommandRequest
	Plan history.EdictPlan `json:"plan"`
}

type HistoryStateRequest struct {
	Advice *history.AdviceChange `json:"advice,omitempty"`
	HistoryCommandRequest
	CheckpointID string `json:"checkpoint_id,omitempty"`
	Label        string `json:"label,omitempty"`
	Target       string `json:"target,omitempty"`
}

// historyMutate checks the durable receipt before model work. RequestHash is
// derived from caller intent, not nondeterministic model output. A cancelled or
// stale model result never appends a partial decree, conversation or quarter.
func (s *HistoryAppService) historyMutate(ctx context.Context, projectID, campaignID, kind string, base HistoryCommandRequest, request any, prepare func(context.Context, *history.Store, config.Config, history.State) (history.Command, error)) (history.Receipt, error) {
	hash := historyRequestHash(kind, request)
	receipt, err := historyRun(ctx, s, projectID, campaignID, base.CommandID, hash, func(ctx context.Context) (history.Receipt, error) {
		store, cfg, err := s.binding(projectID)
		if err != nil {
			return history.Receipt{}, err
		}
		previous, found, err := store.LookupCommand(ctx, campaignID, base.CommandID, hash)
		if err != nil {
			return history.Receipt{}, err
		}
		if found {
			return previous, nil
		}
		state, err := store.Read(ctx, campaignID)
		if err != nil {
			return history.Receipt{}, err
		}
		if state.Archived && kind != "unarchive" {
			return history.Receipt{}, history.ErrNotFound
		}
		if state.Revision != base.ExpectedRevision {
			return history.Receipt{}, history.ErrConflict
		}
		command, err := prepare(ctx, store, cfg, state)
		if err != nil {
			return history.Receipt{}, err
		}
		if err := ctx.Err(); err != nil {
			return history.Receipt{}, err
		}
		command.ID = base.CommandID
		command.ExpectedRevision = base.ExpectedRevision
		command.Kind = kind
		command.RequestHash = hash
		return store.Apply(ctx, campaignID, command)
	})
	receipt.State = history.PlayerState(receipt.State)
	return receipt, err
}

func (s *HistoryAppService) Chat(ctx context.Context, projectID, campaignID string, req HistoryChatRequest) (history.Receipt, error) {
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > 12000 || len(req.MinisterIDs) < 1 || len(req.MinisterIDs) > 5 {
		return history.Receipt{}, ErrHistoryInput
	}
	return s.historyMutate(ctx, projectID, campaignID, "chat", req.HistoryCommandRequest, req, func(ctx context.Context, store *history.Store, cfg config.Config, state history.State) (history.Command, error) {
		var sourceReport *history.QuarterReport
		if req.SourceReportID != "" {
			view := history.PlayerState(state)
			for _, report := range view.Reports {
				if report.ID == req.SourceReportID && !report.Discarded {
					r := report
					sourceReport = &r
					break
				}
			}
			if sourceReport == nil {
				return history.Command{}, ErrHistoryInput
			}
		}
		known := map[string]bool{}
		for _, minister := range state.Ministers {
			known[minister.ID] = true
		}
		seen := map[string]bool{}
		for _, id := range req.MinisterIDs {
			if !known[id] || seen[id] {
				return history.Command{}, ErrHistoryInput
			}
			seen[id] = true
		}
		mode := "private"
		if len(req.MinisterIDs) > 1 {
			mode = "council"
		}
		turn := history.ChatTurn{SourceReportID: req.SourceReportID, Mode: mode, MinisterIDs: req.MinisterIDs, UserMessage: req.Message, Replies: []history.Reply{}}
		roster := make([]historyPublicMinister, 0, len(state.Ministers))
		for _, minister := range state.Ministers {
			roster = append(roster, historyPublicMinister{ID: minister.ID, Name: minister.Name, Office: minister.Office})
		}
		availableActions := []string{"relief", "training", "construction", "pause", "resume", "add_funds", "reassign"}
		ministerPrompt := historyMinisterPrompt
		if state.World != nil {
			ministerPrompt = historyWorldMinisterPrompt
			availableActions = []string{"natural_language_orders"}
		}
		for _, id := range req.MinisterIDs {
			personal, err := store.Context(ctx, campaignID, id, req.Message)
			if err != nil {
				return history.Command{}, err
			}
			if mode == "council" {
				if state.World != nil {
					view := history.ContextWorld(history.CouncilWorld(*state.World, req.MinisterIDs, id))
					personal.CurrentFacts["world"] = view
					personal.CurrentFacts["decision_briefing"] = map[string]any{"current_accounts": view.Accounts, "rule": "本次廷议提供公共局势，以及本人职责内账目和经办政令。可以据实汇报公务，不能把只由本人掌握的密令或私人信息主动公开；其他人仅从实际发言获知新增内容。已生效政策继续，讨论本身不产生新支出或撤销授权。"}
				}
				// Private audience transcripts are not an implicit publication
				// channel. Only already-shared witnessed discussions enter a
				// council; this also makes non-disclosure mechanically testable.
				visible := make([]history.ChatTurn, 0, len(personal.Memories))
				for _, memory := range personal.Memories {
					if historyMemorySharedWith(memory, req.MinisterIDs) {
						visible = append(visible, memory)
					} else {
						personal.OmittedTurns++
					}
				}
				personal.Memories = visible
			}
			input := struct {
				SourceReport       *history.QuarterReport  `json:"selected_report,omitempty"`
				Mode               string                  `json:"mode"`
				Context            history.MinisterContext `json:"context"`
				MinisterIDs        []string                `json:"minister_ids"`
				Message            string                  `json:"message"`
				EarlierReplies     []history.Reply         `json:"earlier_replies"`
				AvailableMinisters []historyPublicMinister `json:"available_ministers"`
				AvailableActions   []string                `json:"available_actions"`
			}{SourceReport: sourceReport, Mode: mode, Context: personal, MinisterIDs: req.MinisterIDs, Message: req.Message, EarlierReplies: turn.Replies, AvailableMinisters: roster, AvailableActions: availableActions}
			reply, err := historyGenerateJSON[historyMinisterPayload](ctx, &cfg, campaignID, "minister", ministerPrompt, req.Locale, input)
			if err != nil {
				return history.Command{}, err
			}
			reply.Reply = strings.TrimSpace(reply.Reply)
			if reply.Reply == "" || utf8.RuneCountInString(reply.Reply) > 8000 {
				return history.Command{}, fmt.Errorf("%w: invalid minister reply length", ErrHistoryModelOutput)
			}
			turn.Replies = append(turn.Replies, history.Reply{MinisterID: id, Text: reply.Reply})
		}
		return history.Command{Chat: &turn}, nil
	})
}

func (s *HistoryAppService) PrepareEdict(ctx context.Context, projectID, campaignID string, req HistoryPrepareRequest) (HistoryPreparedEdict, error) {
	req.Text = strings.TrimSpace(req.Text)
	req.SourceChatID = strings.TrimSpace(req.SourceChatID)
	if req.Text == "" || utf8.RuneCountInString(req.Text) > 12000 || len(req.SourceChatID) > 100 {
		return HistoryPreparedEdict{}, ErrHistoryInput
	}
	return historyRun(ctx, s, projectID, campaignID, req.CommandID, historyRequestHash("prepare-edict", req), func(ctx context.Context) (HistoryPreparedEdict, error) {
		store, cfg, err := s.binding(projectID)
		if err != nil {
			return HistoryPreparedEdict{}, err
		}
		state, err := store.Read(ctx, campaignID)
		if err != nil {
			return HistoryPreparedEdict{}, err
		}
		if state.Revision != req.ExpectedRevision {
			return HistoryPreparedEdict{}, history.ErrConflict
		}
		var advice *history.SavedAdvice
		if req.SourceAdviceID != "" {
			for _, item := range state.Advice {
				if item.ID == req.SourceAdviceID {
					copy := item
					advice = &copy
					break
				}
			}
			if advice == nil || (req.SourceChatID != "" && req.SourceChatID != advice.ChatID) {
				return HistoryPreparedEdict{}, ErrHistoryInput
			}
			req.SourceChatID = advice.ChatID
		}
		var source *history.ChatTurn
		if req.SourceChatID != "" {
			for _, chat := range state.Chats {
				if chat.ID != req.SourceChatID || chat.Discarded {
					continue
				}
				// Use canonical branch ranges as well as the derived display
				// marker; a stale index must not admit a discarded proposal.
				discarded := false
				for _, span := range state.DiscardedRanges {
					if chat.Revision >= span.Start && chat.Revision <= span.End {
						discarded = true
						break
					}
				}
				if !discarded {
					selected := chat
					source = &selected
					break
				}
			}
			if source == nil {
				return HistoryPreparedEdict{}, ErrHistoryInput
			}
		}
		var plan history.EdictPlan
		var generationErr error
		if state.World != nil {
			plan, generationErr = historyPrepareWorldEdict(ctx, &cfg, campaignID, "edict", req.Locale, state, req.Text, source, advice)
		} else {
			plan, generationErr = historyGenerateJSON[history.EdictPlan](ctx, &cfg, campaignID, "edict", historyEdictPrompt, req.Locale, struct {
				State            any               `json:"current_state"`
				Text             string            `json:"edict"`
				SourceDiscussion *history.ChatTurn `json:"source_discussion,omitempty"`
			}{historyCurrentFacts(state), req.Text, source})
		}
		err = generationErr
		if err != nil {
			return HistoryPreparedEdict{}, err
		}
		plan.Text = req.Text
		// Provenance is bound by the caller's selected canonical discussion,
		// never by an ID invented or echoed by the model.
		plan.SourceChatID = req.SourceChatID
		plan.SourceAdviceID = req.SourceAdviceID
		if (state.World == nil && (len(plan.Instructions) < 1 || len(plan.Instructions) > 32 || len(plan.Orders) > 0)) || (state.World != nil && (len(plan.Orders) < 1 || len(plan.Orders) > 32 || len(plan.Instructions) > 0)) {
			return HistoryPreparedEdict{}, fmt.Errorf("%w: invalid instruction count", ErrHistoryModelOutput)
		}
		for index := range plan.Instructions {
			if strings.TrimSpace(plan.Instructions[index].ID) == "" {
				plan.Instructions[index].ID = fmt.Sprintf("order-%d", index+1)
			}
		}
		if err := ctx.Err(); err != nil {
			return HistoryPreparedEdict{}, err
		}
		latest, err := store.Read(ctx, campaignID)
		if err != nil {
			return HistoryPreparedEdict{}, err
		}
		if latest.Revision != state.Revision {
			return HistoryPreparedEdict{}, history.ErrConflict
		}
		return HistoryPreparedEdict{ExpectedRevision: state.Revision, Plan: plan, Outcomes: history.ValidatePlan(state, plan)}, nil
	})
}

func (s *HistoryAppService) CommitEdict(ctx context.Context, projectID, campaignID string, req HistoryEdictRequest) (history.Receipt, error) {
	return s.historyMutate(ctx, projectID, campaignID, "edict", req.HistoryCommandRequest, req, func(context.Context, *history.Store, config.Config, history.State) (history.Command, error) {
		return history.Command{Edict: &req.Plan}, nil
	})
}

func (s *HistoryAppService) Advance(ctx context.Context, projectID, campaignID string, req HistoryCommandRequest) (history.Receipt, error) {
	return s.historyMutate(ctx, projectID, campaignID, "advance", req, req, func(ctx context.Context, _ *history.Store, cfg config.Config, state history.State) (history.Command, error) {
		if state.World != nil {
			return historyAdvanceWorld(ctx, &cfg, campaignID, req.Locale, state)
		}
		plan, err := historyGenerateJSON[history.QuarterPlan](ctx, &cfg, campaignID, "quarter_plan", historyQuarterPrompt, req.Locale, historyCurrentFacts(state))
		if err != nil {
			return history.Command{}, err
		}
		plan.Narrative = ""
		next, report, err := history.PreviewAdvance(state, plan)
		if err != nil {
			return history.Command{}, fmt.Errorf("%w: %v", ErrHistoryModelOutput, err)
		}
		summary, err := historyGenerateJSON[historyReportPayload](ctx, &cfg, campaignID, "quarter_report", historyReportPrompt, req.Locale, struct {
			Report  history.QuarterReport `json:"report"`
			Current any                   `json:"next_state"`
		}{report, historyCurrentFacts(next)})
		if err != nil {
			return history.Command{}, err
		}
		summary.Narrative = strings.TrimSpace(summary.Narrative)
		if summary.Narrative == "" || utf8.RuneCountInString(summary.Narrative) > 12000 {
			return history.Command{}, fmt.Errorf("%w: invalid report length", ErrHistoryModelOutput)
		}
		plan.Narrative = summary.Narrative
		return history.Command{Quarter: &plan}, nil
	})
}

func (s *HistoryAppService) StateCommand(ctx context.Context, projectID, campaignID, kind string, req HistoryStateRequest) (history.Receipt, error) {
	if kind != "advice" && kind != "checkpoint" && kind != "restore" && kind != "inspect" && kind != "archive" && kind != "unarchive" {
		return history.Receipt{}, ErrHistoryInput
	}
	receipt, err := s.historyMutate(ctx, projectID, campaignID, kind, req.HistoryCommandRequest, req, func(context.Context, *history.Store, config.Config, history.State) (history.Command, error) {
		return history.Command{Advice: req.Advice, CheckpointID: req.CheckpointID, Label: req.Label, InspectionTarget: req.Target}, nil
	})
	if err == nil && kind == "advice" {
		slog.InfoContext(ctx, "[history] advice collection committed", "campaign_id", campaignID, "command_id", req.CommandID, "revision", receipt.Revision, "count", len(receipt.State.Advice))
	}
	return receipt, err
}

// The ruler's current management view has no private audience transcripts and
// no superseded balance. Those historical statements never become current fact.
func historyCurrentFacts(state history.State) any {
	if state.World != nil {
		return map[string]any{"year": state.Year, "quarter": state.Quarter, "turn": state.Turn, "world": history.ContextWorld(history.VisibleWorld(*state.World, "player")), "scenario_name": state.Scenario.Name, "resources": state.Scenario.Resources, "metrics": state.Scenario.Metrics}
	}
	return struct {
		Year       int                `json:"year"`
		Quarter    int                `json:"quarter"`
		Turn       int                `json:"turn"`
		Treasury   int64              `json:"treasury"`
		PrivyPurse int64              `json:"privy_purse"`
		Grain      int64              `json:"grain"`
		Regions    []history.Region   `json:"regions"`
		Ministers  []history.Minister `json:"ministers"`
		Projects   []history.Project  `json:"projects"`
	}{state.Year, state.Quarter, state.Turn, state.Treasury, state.PrivyPurse, state.Grain, state.Regions, state.Ministers, state.Projects}
}

func historyMemorySharedWith(memory history.ChatTurn, audience []string) bool {
	if memory.Mode != "council" {
		return false
	}
	witnessed := map[string]bool{}
	for _, id := range memory.MinisterIDs {
		witnessed[id] = true
	}
	for _, id := range audience {
		if !witnessed[id] {
			return false
		}
	}
	return true
}
