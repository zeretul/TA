package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"ta/config"
	"ta/internal/agents/conversationconfig"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/quest"
	agent "ta/agent"
	"ta/agent/providers"
)

// The in-story assistant answers two read-only questions about the scene the
// player is already reading: what the choice in front of them really involves,
// and which directions the story could plausibly go from here.
//
// 自制-剧情助手与选项分析 fixes the boundary this file has to make structural
// rather than promised:
//
//   - It is advice. The call allocates no turn, commits nothing to the journal,
//     advances no branch head, moves no time and spends no resource. The only
//     way anything it says becomes real is the player sending it as their own
//     input, or explicitly adopting a direction as the branch's forward intent.
//
//   - It may read only what the protagonist knows. That is not a prompt rule
//     here: the context handed to the model is assembled from the same
//     player-facing projection the snapshot read uses, so a pack's private
//     fields cannot reach it by construction. What the model is told about the
//     story's own creative direction is marked author-side and forbidden from
//     surfacing.
//
//   - It never invents a probability. No formal calculation backs a win rate, so
//     the prompt forbids stating one rather than leaving the model to decide
//     whether a number would sound authoritative.
//
// The two actions share one call shape because they share that boundary; only
// the question put to the model differs.

const (
	storyAssistantContextMaxBytes = 12 * 1024
	storyAssistantReadMaxRunes    = 4000
	storyAssistantMaxSuggestions  = 6
)

// StoryAssistantRequest asks one read-only question about the current scene.
type StoryAssistantRequest struct {
	StoryID  string
	BranchID string
	// TurnID binds the answer to the turn the player is looking at. A response
	// arriving after the branch has moved on describes a situation that no
	// longer exists, so it is refused rather than shown as current advice.
	TurnID string
	// Action is analyze or inspire.
	Action string
	// PlayerInput is what the player has typed but not sent, if anything.
	PlayerInput string
	// ChosenOption is the option the player is considering, if they named one.
	ChosenOption string
	// Locale is the language the player's page is reading in. It decides the
	// language of the advice.
	Locale string
}

// StoryAssistantSuggestion is one concrete phrasing the player may drop into
// their input. It is a starting point they can rewrite, never a decision.
type StoryAssistantSuggestion struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Action string `json:"action"`
}

// StoryAssistantResult is one advisory answer.
type StoryAssistantResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	TurnID   string `json:"turn_id"`
	Action   string `json:"action"`
	// Read is the analysis itself, in the story's language.
	Read        string                     `json:"read"`
	Suggestions []StoryAssistantSuggestion `json:"suggestions"`
	// ContextRevision is the branch revision the answer was written against. A
	// client showing a different revision knows the advice may be describing a
	// situation it has already left.
	ContextRevision uint64 `json:"context_revision"`
}

// ErrStoryAssistantStaleTurn means the question was asked about a turn the
// branch has already left. Answering it would describe a scene the player is no
// longer in.
var ErrStoryAssistantStaleTurn = errors.New("the story has moved on since this question was asked")

// ErrStoryAssistantUnknownAction means the caller asked for something the
// assistant does not do.
var ErrStoryAssistantUnknownAction = errors.New("unknown story assistant action")

// ErrStoryAssistantNoPlan means this branch has no plan to record a direction
// in. It is a state the player can act on — planning is a story setting — not a
// malformed request.
var ErrStoryAssistantNoPlan = errors.New("this branch has no plan to record a direction in")

// ErrStoryAssistantEmptyAnswer and ErrStoryAssistantUnreadableAnswer are the two
// ways a completed model call can still carry no usable advice. They are
// distinct from a failed call: the request succeeded, the answer did not.
var ErrStoryAssistantEmptyAnswer = errors.New("the assistant returned an empty answer")
var ErrStoryAssistantUnreadableAnswer = errors.New("the assistant returned an unreadable answer")

// AskStoryAssistant answers one read-only question.
//
// Nothing is written. A question asked about a turn the branch has left is
// refused before any model call is made, because the answer would describe a
// scene that no longer exists.
func (a *App) AskStoryAssistant(ctx context.Context, request StoryAssistantRequest) (StoryAssistantResult, error) {
	if a == nil {
		return StoryAssistantResult{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return StoryAssistantResult{}, fmt.Errorf("an assistant question requires a story")
	}
	action := strings.TrimSpace(request.Action)
	if action != prompts.StoryAssistantActionAnalyze && action != prompts.StoryAssistantActionInspire {
		return StoryAssistantResult{}, fmt.Errorf("%w: %q", ErrStoryAssistantUnknownAction, action)
	}
	store, runtimeCfg, err := a.interactiveStoreRuntime()
	if err != nil {
		return StoryAssistantResult{}, err
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return StoryAssistantResult{}, err
	}
	snapshot := storyContext.Snapshot
	branchID := strings.TrimSpace(snapshot.BranchID)
	turnID := storyAssistantTurnID(snapshot)
	if requested := strings.TrimSpace(request.TurnID); requested != "" && requested != turnID {
		return StoryAssistantResult{}, fmt.Errorf("%w: asked about %q, the branch is at %q", ErrStoryAssistantStaleTurn, requested, turnID)
	}

	// The story's retired per-story direction is deliberately not read. Writing
	// direction is the shared preferences plus the chosen style, and the
	// assistant has no business reviving a third layer the player cannot see.
	branchPlan := ""
	if storyContext.Meta.PlanningMode == interactive.StoryPlanningModeEnabled && snapshot.BranchPlan != nil {
		branchPlan = snapshot.BranchPlan.Markdown
	}

	systemInstruction := prompts.BuildStoryAssistantSystemInstruction(prompts.StoryAssistantSystemInstructionInput{
		Workspace:    runtimeCfg.Workspace,
		Action:       action,
		PlayerInput:  request.PlayerInput,
		ChosenOption: request.ChosenOption,
		KnownContext: storyAssistantKnownContext(store, storyContext),
		BranchPlan:   branchPlan,
		OutputLanguage: request.Locale,
	})

	// The assistant runs on the story's own narrative model configuration,
	// because that is the setting the player has actually chosen for this story.
	if err := resolveStoryAssistantConfig(store, &runtimeCfg, storyID, branchID); err != nil {
		return StoryAssistantResult{}, err
	}
	level := providers.ThinkingLevel(strings.TrimSpace(config.ResolveAgentModel(&runtimeCfg, config.AgentKindInteractiveStory).ThinkingLevel))

	content, err := a.generateStoryAssistantAnswer(ctx, &runtimeCfg, storyID, branchID, turnID, action, systemInstruction, level)
	if err != nil {
		return StoryAssistantResult{}, err
	}
	read, suggestions, err := parseStoryAssistantPayload(content)
	if err != nil {
		return StoryAssistantResult{}, err
	}
	if suggestions == nil {
		suggestions = []StoryAssistantSuggestion{}
	}

	slog.InfoContext(ctx, fmt.Sprintf("[story-assistant] answered story_id=%s branch_id=%s turn_id=%s action=%s chars=%d suggestions=%d",
		storyID, branchID, turnID, action, len([]rune(read)), len(suggestions)))
	return StoryAssistantResult{
		StoryID: storyID, BranchID: branchID, TurnID: turnID, Action: action,
		Read: read, Suggestions: suggestions,
		ContextRevision: snapshot.ContextRevision,
	}, nil
}

// StoryAssistantAdoptRequest records one direction as this branch's forward
// intent.
type StoryAssistantAdoptRequest struct {
	StoryID  string
	BranchID string
	// TurnID is the turn the direction was adopted at. A direction adopted for a
	// turn the branch has left belongs to a situation that is over.
	TurnID string
	// Direction is the wording the player accepted. It is recorded as an
	// intention, not as an event: it says where the story should look, not what
	// has happened.
	Direction string
	// BaseRevision is the plan revision the player was shown. When it no longer
	// matches, the plan has changed elsewhere and the caller is told to reload
	// rather than having its older document written back.
	BaseRevision string
}

// StoryAssistantAdoptResult reports what was recorded and where.
type StoryAssistantAdoptResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	TurnID   string `json:"turn_id"`
	// Scope is always "branch" here: the direction is recorded in this branch's
	// plan, so a fork does not inherit a wish made in a story it never played.
	Scope      string                 `json:"scope"`
	BranchPlan interactive.BranchPlan `json:"branch_plan"`
	// ContextRevision is the branch revision after the write.
	ContextRevision uint64 `json:"context_revision"`
	// Applied is false when the direction was already recorded, which makes a
	// retry of a lost response a read rather than a second write.
	Applied bool `json:"applied"`
}

// AdoptStoryAssistantDirection records an explicitly accepted direction.
//
// 自制-剧情助手与选项分析 requires this to be a separate, deliberate act from
// generating the idea: reading a suggestion or generating one must never write
// anything, and only this call does. It writes through the branch plan's own
// versioned commit path rather than inventing a second fact store, so the next
// turn's context sees the direction by the same route it sees every other part
// of the plan.
//
// The plan is read again here rather than trusting what the caller saw, because
// the edit is additive: a plan that moved on between the suggestion and the
// click keeps its other edits, and only a genuine concurrent write at the store
// is reported as a conflict.
func (a *App) AdoptStoryAssistantDirection(ctx context.Context, request StoryAssistantAdoptRequest) (StoryAssistantAdoptResult, error) {
	if a == nil {
		return StoryAssistantAdoptResult{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return StoryAssistantAdoptResult{}, fmt.Errorf("adopting a direction requires a story")
	}
	direction := strings.Join(strings.Fields(strings.TrimSpace(request.Direction)), " ")
	if direction == "" {
		return StoryAssistantAdoptResult{}, fmt.Errorf("adopting a direction requires the direction itself")
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return StoryAssistantAdoptResult{}, err
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return StoryAssistantAdoptResult{}, err
	}
	snapshot := storyContext.Snapshot
	branchID := strings.TrimSpace(snapshot.BranchID)
	turnID := storyAssistantTurnID(snapshot)
	if requested := strings.TrimSpace(request.TurnID); requested != "" && requested != turnID {
		return StoryAssistantAdoptResult{}, fmt.Errorf("%w: adopted at %q, the branch is at %q", ErrStoryAssistantStaleTurn, requested, turnID)
	}
	if storyContext.Meta.PlanningMode != interactive.StoryPlanningModeEnabled || snapshot.BranchPlan == nil {
		return StoryAssistantAdoptResult{}, ErrStoryAssistantNoPlan
	}
	plan := *snapshot.BranchPlan
	// Recorded-ness is checked before the revision. A retry whose response was
	// lost carries the revision the player was shown, which is necessarily the
	// one the first attempt replaced, so checking the revision first would answer
	// a completed adoption with a conflict. The direction is in the plan either
	// way, and reporting the existing plan is the honest answer.
	if interactive.StoryAssistantDirectionIsRecorded(plan, direction) {
		return StoryAssistantAdoptResult{
			StoryID: storyID, BranchID: branchID, TurnID: turnID, Scope: "branch",
			BranchPlan: plan, ContextRevision: snapshot.ContextRevision, Applied: false,
		}, nil
	}
	if base := strings.TrimSpace(request.BaseRevision); base != "" && base != plan.Revision {
		return StoryAssistantAdoptResult{}, fmt.Errorf("%w: shown %q, branch is at %q", interactive.ErrBranchPlanRevisionConflict, base, plan.Revision)
	}

	markdown := interactive.AddBranchPlanDirection(plan.Markdown, direction)
	if markdown == plan.Markdown {
		return StoryAssistantAdoptResult{}, fmt.Errorf("the branch plan could not take the direction")
	}
	result, err := a.UpdateInteractiveBranchPlan(storyID, interactive.UpdateBranchPlanRequest{
		BranchID: branchID, Markdown: markdown, BaseRevision: plan.Revision,
	})
	if err != nil {
		return StoryAssistantAdoptResult{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[story-assistant] direction adopted story_id=%s branch_id=%s turn_id=%s chars=%d",
		storyID, branchID, turnID, len([]rune(direction))))
	return StoryAssistantAdoptResult{
		StoryID: storyID, BranchID: branchID, TurnID: turnID,
		Scope: "branch", BranchPlan: result.BranchPlan,
		ContextRevision: result.ContextRevision, Applied: true,
	}, nil
}

// storyAssistantTurnID is the turn the branch currently stands on. An empty
// value means the branch has no committed turn yet, which is a real state for a
// story that has not started rather than an error.
func storyAssistantTurnID(snapshot interactive.Snapshot) string {
	if snapshot.CurrentTurn == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.CurrentTurn.ID)
}

// resolveStoryAssistantConfig applies this branch's saved model configuration to
// the runtime config.
//
// It is the read-only half of interactiveapp.ApplyConversationConfig. The
// difference matters: that helper seeds a branch that has no saved configuration
// yet, which appends a config record to the journal. Asking the assistant a
// question must not write, so an unconfigured branch is answered on the
// workspace default instead of being configured as a side effect of being asked.
func resolveStoryAssistantConfig(store *interactive.Store, runtimeCfg *config.Config, storyID, branchID string) error {
	snapshot, ok, err := store.BranchRuntimeConfig(storyID, branchID)
	if err != nil {
		return err
	}
	if !ok {
		// Nothing saved: conversationconfig.Apply would only write the defaults the
		// runtime config already carries.
		return nil
	}
	if err := conversationconfig.Apply(runtimeCfg, snapshot.Config); err != nil {
		return fmt.Errorf("apply interactive conversation runtime config: %w", err)
	}
	return nil
}

// generateStoryAssistantAnswer performs the one-shot model call.
//
// It mirrors the battle round's call: a standalone trace, one system
// instruction and one user message, and no tool protocol. An advisory answer is
// not a story turn — it must not allocate a turn number, must not run the
// narrative tool protocol, and must not advance the branch head.
func (a *App) generateStoryAssistantAnswer(ctx context.Context, cfg *config.Config, storyID, branchID, turnID, action, systemInstruction string, level providers.ThinkingLevel) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindInteractiveStory)
	if err != nil {
		return "", fmt.Errorf("resolve story assistant model configuration: %w", err)
	}
	if level != "" {
		modelCfg.ThinkingLevel = level
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "assistant", cfg.Workspace,
		"story_assistant", "In-Story Assistant",
		"answer a player's read-only question about the choice in front of them",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(prompts.StoryAssistantUserMessage),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindInteractiveStory, messages, nil); err != nil {
		return "", err
	}

	// The answer carries two things prose alone cannot: the analysis, and the
	// concrete phrasings the interface renders as suggestion chips. Asking the
	// provider for a JSON object makes the second half reliably readable, and
	// endpoints that reject the hint get one retry without it.
	content, err := a.callStoryAssistantModel(ctx, cfg, modelio.WithJSONObjectOutput(modelCfg), storyID, branchID, turnID, action, messages)
	if err == nil {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[story-assistant] json_mode failed, retry without response_format err=%v", err))
	return a.callStoryAssistantModel(ctx, cfg, modelCfg, storyID, branchID, turnID, action, messages)
}

func (a *App) callStoryAssistantModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, storyID, branchID, turnID, action string, messages []*agent.Message) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "story_assistant", "advice", map[string]any{
		"story_id": storyID, "branch_id": branchID, "turn_id": turnID, "action": action,
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create story assistant model: %w", err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "story_assistant", "advice", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_assistant", "advice", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate story assistant answer: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the story assistant model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_assistant", "advice", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_assistant", "advice", modelCfg.Model, 0, msg, nil, nil)
	return strings.TrimSpace(msg.Content), nil
}

// storyAssistantPayload is the answer's structured shape.
type storyAssistantPayload struct {
	Read        string                            `json:"read"`
	Suggestions []storyAssistantSuggestionPayload `json:"suggestions"`
}

type storyAssistantSuggestionPayload struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Action string `json:"action"`
}

// parseStoryAssistantPayload validates one answer.
//
// The answer must survive whatever the model wrapped it in: a JSON object, a
// fenced one, or (when the model ignored the contract) plain prose. Plain prose
// is accepted as the analysis with no suggestions, because the analysis itself
// is useful and refusing it would waste a completed call for a formatting
// fault. A suggestion without an action is dropped rather than shown: the
// interface offers these as things the player can send, and one that cannot be
// sent would be a button that does nothing.
func parseStoryAssistantPayload(content string) (string, []StoryAssistantSuggestion, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil, ErrStoryAssistantEmptyAnswer
	}
	if !strings.HasPrefix(text, "{") {
		return storyAssistantClip(text, storyAssistantReadMaxRunes), nil, nil
	}
	var payload storyAssistantPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		// The model sometimes closes a field and then keeps writing prose, which
		// makes the object invalid while every value in it is still readable. The
		// call already succeeded, so the fields are salvaged rather than thrown
		// away; only an answer with nothing readable in it is refused.
		read, suggestions := salvageStoryAssistantFields(text)
		if read == "" {
			return "", nil, fmt.Errorf("%w: %w", ErrStoryAssistantUnreadableAnswer, err)
		}
		return storyAssistantClip(read, storyAssistantReadMaxRunes), suggestions, nil
	}
	read := strings.TrimSpace(payload.Read)
	if read == "" {
		return "", nil, fmt.Errorf("the assistant returned an answer with no analysis")
	}
	suggestions := make([]StoryAssistantSuggestion, 0, len(payload.Suggestions))
	for _, item := range payload.Suggestions {
		action := strings.TrimSpace(item.Action)
		if action == "" {
			continue
		}
		suggestions = append(suggestions, StoryAssistantSuggestion{
			Label:  storyAssistantClip(strings.TrimSpace(item.Label), 80),
			Detail: storyAssistantClip(strings.TrimSpace(item.Detail), 300),
			Action: storyAssistantClip(action, 600),
		})
		if len(suggestions) == storyAssistantMaxSuggestions {
			break
		}
	}
	return storyAssistantClip(read, storyAssistantReadMaxRunes), suggestions, nil
}

// salvageStoryAssistantFields reads the contract's fields out of text that is not
// valid JSON as a whole. Keys are matched wherever they appear, so a stray comma,
// a duplicated key or a sentence after the closing brace does not cost the
// player the answer. Values are decoded as JSON strings, so escapes inside the
// prose still resolve; anything that does not decode is skipped.
func salvageStoryAssistantFields(text string) (string, []StoryAssistantSuggestion) {
	type pending struct {
		label, detail, action string
	}
	var (
		read        string
		suggestions []StoryAssistantSuggestion
		current     pending
		flush       = func() {
			if strings.TrimSpace(current.action) == "" {
				current = pending{}
				return
			}
			suggestions = append(suggestions, StoryAssistantSuggestion{
				Label:  storyAssistantClip(strings.TrimSpace(current.label), 80),
				Detail: storyAssistantClip(strings.TrimSpace(current.detail), 300),
				Action: storyAssistantClip(strings.TrimSpace(current.action), 600),
			})
			current = pending{}
		}
	)
	for offset := 0; offset < len(text); {
		open := strings.IndexByte(text[offset:], '"')
		if open < 0 {
			break
		}
		open += offset
		decoder := json.NewDecoder(strings.NewReader(text[open:]))
		var key string
		if err := decoder.Decode(&key); err != nil {
			offset = open + 1
			continue
		}
		rest := text[open+int(decoder.InputOffset()):]
		trimmed := strings.TrimLeft(rest, " \t\r\n")
		if !strings.HasPrefix(trimmed, ":") {
			offset = open + int(decoder.InputOffset())
			continue
		}
		valueStart := open + int(decoder.InputOffset()) + (len(rest) - len(trimmed)) + 1
		valueText := strings.TrimLeft(text[valueStart:], " \t\r\n")
		next := valueStart + (len(text[valueStart:]) - len(valueText))
		if !strings.HasPrefix(valueText, "\"") {
			offset = next
			continue
		}
		valueDecoder := json.NewDecoder(strings.NewReader(valueText))
		var value string
		if err := valueDecoder.Decode(&value); err != nil {
			offset = next + 1
			continue
		}
		switch key {
		case "read":
			if read == "" {
				read = value
			}
		case "label":
			flush()
			current.label = value
		case "detail":
			current.detail = value
		case "action":
			current.action = value
		}
		offset = next + int(valueDecoder.InputOffset())
	}
	flush()
	if len(suggestions) > storyAssistantMaxSuggestions {
		suggestions = suggestions[:storyAssistantMaxSuggestions]
	}
	return read, suggestions
}

// storyAssistantKnownContext renders what the protagonist knows.
//
// Every value here comes through playerKnownState, which is the same projection
// the snapshot read uses, so a pack's private fields cannot reach the model by
// construction. The renderer itself is read-only and says nothing about writing
// state: it is handed to an advisor, not to a narrator.
func storyAssistantKnownContext(store *interactive.Store, storyContext interactive.StoryContext) string {
	snapshot := storyContext.Snapshot
	known := playerKnownState(snapshot.State, snapshot.ActorStateSchema)
	nodeID := strings.TrimSpace(snapshot.BranchID)
	if id := storyAssistantTurnID(snapshot); id != "" {
		nodeID = id
	}
	scene := interactive.StorySceneFromState(known, nodeID)

	var sb strings.Builder
	sb.WriteString("### Where the protagonist is\n\n")
	if location := strings.TrimSpace(scene.Location); location != "" {
		fmt.Fprintf(&sb, "- Place: %s\n", storyAssistantClip(location, 400))
	}
	if title := strings.TrimSpace(scene.Title); title != "" {
		fmt.Fprintf(&sb, "- Scene: %s\n", storyAssistantClip(title, 400))
	}
	if worldTime := characterWorldTime(storyContext); worldTime != "" {
		fmt.Fprintf(&sb, "- In-story time: %s\n", storyAssistantClip(worldTime, 200))
	}

	if narrative := latestTurnNarrative(snapshot); narrative != "" {
		sb.WriteString("\n### What just happened\n\n")
		sb.WriteString(narrative)
		sb.WriteString("\n")
	}

	if standing := storyAssistantStandingContext(store, storyContext); standing != "" {
		sb.WriteString("\n")
		sb.WriteString(standing)
		sb.WriteString("\n")
	}

	if choices := storyAssistantChoices(snapshot); len(choices) > 0 {
		sb.WriteString("\n### The choices the story is offering\n\n")
		for index, choice := range choices {
			fmt.Fprintf(&sb, "%d. %s\n", index+1, storyAssistantClip(oneLineText(choice), 400))
		}
	}

	if actors := storyAssistantActorContext(known, snapshot.ActorStateSchema, scene.Present); actors != "" {
		sb.WriteString("\n### What the protagonist knows about the people and the world\n\n")
		sb.WriteString(actors)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// storyAssistantStandingContext renders the promises, permissions, duties and
// tasks that still stand, and the ones whose agreed hour has passed.
//
// 自制-剧情助手与选项分析 asks the assistant to read 当前相关上下文、历史承诺与期限.
// Before this it read only the current turn's prose tail, so a promise made three
// turns ago or a task whose limit had already run out was invisible to it, and it
// advised as though the scene had no history. Everything here is read from a
// record the game already commits and already shows through the scene block and
// the task panel, so the advisor and the story cannot disagree about what is
// still owed.
//
// A failed read narrows the block instead of removing the answer: the scene is
// still committed state, and an advisor told nothing about deadlines is better
// than one that refuses to speak. Nothing here writes: CharacterState and
// QuestStateFor are plain reads, which is what keeps asking the assistant a
// read-only act.
func storyAssistantStandingContext(store *interactive.Store, storyContext interactive.StoryContext) string {
	snapshot := storyContext.Snapshot
	storyID := strings.TrimSpace(snapshot.StoryID)
	branchID := strings.TrimSpace(snapshot.BranchID)
	if store == nil || storyID == "" || branchID == "" {
		return ""
	}
	scene := interactive.StoryScene(snapshot)
	// A story that authors no clock still ages its threads, so the same fallback
	// the character window uses fills the scene's time: without it every deadline
	// would read as not yet reached and a lapsed arrangement would be advised on
	// as though it still bound.
	if scene.WorldTime == "" {
		scene.WorldTime = characterWorldTime(storyContext)
	}

	var sb strings.Builder
	if state, err := store.CharacterState(storyID, branchID); err == nil {
		// The same split the main story's scene block uses: StoryCastAt decides which
		// threads still bind at this world time, so a lapsed arrangement is reported
		// as lapsed rather than as work still owed. The formal instant is passed for
		// the same reason the scene block passes it: a thread bound to the clock
		// cannot be decided without it.
		writeStoryAssistantMatters(&sb, character.StoryCastAt(state, scene, scene.WorldTime, interactive.SnapshotClockSeconds(snapshot)))
		writeStoryAssistantMessages(&sb, state, scene.WorldTime, interactive.SnapshotClockSeconds(snapshot))
	}
	if quests, err := store.QuestStateFor(storyID, branchID); err == nil {
		writeStoryAssistantTasks(&sb, quests.Tasks)
	}
	return strings.TrimSpace(sb.String())
}

// writeStoryAssistantMatters lists the threads the protagonist is party to. The
// wording of a lapsed thread comes from character.LapseClause, which the scene
// block and the character window read too: a right that ran out did not go
// undone, and one package owning that distinction is what keeps the three
// renderings from disagreeing about it.
func writeStoryAssistantMatters(sb *strings.Builder, members []character.CastMember) {
	open := make([]string, 0)
	lapsed := make([]string, 0)
	for _, member := range members {
		name := storyAssistantClip(member.Profile.DisplayName(), 80)
		for _, matter := range member.OpenMatters {
			statement := strings.TrimSpace(matter.Statement)
			if statement == "" {
				continue
			}
			line := fmt.Sprintf("[%s] %s (with %s; open, not settled", matter.Kind, storyAssistantClip(statement, 240), name)
			if deadline := strings.TrimSpace(matter.Deadline); deadline != "" {
				line += fmt.Sprintf("; due at %s", storyAssistantClip(deadline, 80))
			}
			open = append(open, line+")")
		}
		for _, matter := range member.LapsedMatters {
			statement := strings.TrimSpace(matter.Statement)
			if statement == "" {
				continue
			}
			lapsed = append(lapsed, fmt.Sprintf("[%s] %s (with %s; %s; lapsed at %s)",
				matter.Kind, storyAssistantClip(statement, 240), name,
				character.LapseClause(matter.Kind), storyAssistantClip(matter.Deadline, 80)))
		}
	}
	if len(open) == 0 && len(lapsed) == 0 {
		return
	}
	sort.Strings(open)
	sort.Strings(lapsed)
	sb.WriteString("### What is still standing\n\n")
	if len(open) > 0 {
		sb.WriteString("- Open — these are not agreed, not done, and not decided:\n")
		for _, line := range open {
			fmt.Fprintf(sb, "  - %s\n", line)
		}
	}
	if len(lapsed) > 0 {
		sb.WriteString("- Came due — the agreed hour has passed and these no longer bind. Do not treat one as done, and do not advise acting as though it still stood:\n")
		for _, line := range lapsed {
			fmt.Fprintf(sb, "  - %s\n", line)
		}
	}
}

// writeStoryAssistantMessages lists the word that has reached the protagonist and
// has not been answered.
//
// 自制-剧情助手与选项分析 asks the assistant to read 历史承诺与期限, and a letter is
// where an offer with an hour often arrives: the promise was never spoken in a
// scene the advisor saw, so without this the advice planned around an invitation
// it had no way to know about. A message also carries its own sending hour, which
// is what keeps the advisor from advising on a lapsed offer as though it still
// stood.
//
// Only pending messages are listed. character.PendingMessages already drops the
// ones the player handled or set aside, so an answered letter is not reopened as
// outstanding. The text is quoted rather than summarised, because the advisor's
// job is to read what was actually written.
func writeStoryAssistantMessages(sb *strings.Builder, state character.State, worldTime string, clockSeconds int64) {
	messages := character.PendingMessages(state.Messages, worldTime)
	if len(messages) == 0 {
		return
	}
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		message = message.Normalize()
		text := oneLineText(message.Text)
		if text == "" {
			continue
		}
		name := message.CharacterID
		if profile, ok := state.Profile(message.CharacterID); ok {
			name = profile.DisplayName()
		}
		sent := message.WorldTime
		if sent == "" {
			sent = "an hour the record does not name"
		}
		line := fmt.Sprintf("- %s wrote at %s: %s", storyAssistantClip(name, 80), storyAssistantClip(sent, 80), storyAssistantClip(text, 300))
		deadline := strings.TrimSpace(message.Deadline)
		if message.LapsedAt(worldTime, clockSeconds) {
			if deadline == "" {
				deadline = "an hour the record does not name"
			}
			line += fmt.Sprintf(" — its hour passed at %s, so what it offered no longer stands", storyAssistantClip(deadline, 80))
		} else if deadline != "" {
			line += fmt.Sprintf(" — it names %s", storyAssistantClip(deadline, 80))
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return
	}
	sb.WriteString("### Word that has reached the protagonist\n\n")
	sb.WriteString("- Written at the hour named, not said in a scene; receiving one accepts nothing:\n")
	for _, line := range lines {
		fmt.Fprintf(sb, "  - %s\n", line)
	}
}

// writeStoryAssistantTasks lists the work the protagonist has actually taken on.
//
// An offered-but-untaken task is left out: it is a thing the player may still
// choose, not something they are carrying, and listing it as a commitment would
// have the advisor plan around an obligation that does not exist. An expired one
// is listed with what the rules actually decided, because "the hour passed" is a
// real outcome the player may need to be advised about.
func writeStoryAssistantTasks(sb *strings.Builder, tasks []interactive.QuestStateView) {
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == string(quest.StatusOffered) {
			continue
		}
		title := strings.TrimSpace(task.Title)
		if title == "" {
			title = task.TaskID
		}
		line := fmt.Sprintf("- %s", storyAssistantClip(oneLineText(title), 120))
		if brief := strings.TrimSpace(task.Brief); brief != "" {
			line += ": " + storyAssistantClip(oneLineText(brief), 240)
		}
		if giver := strings.TrimSpace(task.Giver); giver != "" {
			line += fmt.Sprintf(" (given by %s)", storyAssistantClip(giver, 80))
		}
		if done, total := storyAssistantObjectiveProgress(task.Objectives); total > 0 {
			line += fmt.Sprintf(" [%d/%d done]", done, total)
		}
		switch {
		case task.Expired:
			line += " — the agreed moment passed before the work was finished"
		case task.Settled:
			line += " — finished and paid"
		case task.Status == string(quest.StatusCompleted):
			line += " — finished, payment not yet handed over"
		case task.DeadlineText != "":
			line += fmt.Sprintf(" — due %s", storyAssistantClip(task.DeadlineText, 80))
		default:
			line += " — no deadline agreed"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	sb.WriteString("### Work the protagonist has taken on\n\n")
	for _, line := range lines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

func storyAssistantObjectiveProgress(objectives []interactive.QuestObjectiveView) (done, total int) {
	for _, objective := range objectives {
		total++
		if objective.Completed {
			done++
		}
	}
	return done, total
}

// storyAssistantChoices is the option list the player is looking at, with the
// same precedence the story stage uses: a committed turn result first, the hot
// choices after it.
func storyAssistantChoices(snapshot interactive.Snapshot) []string {
	if snapshot.CurrentTurn == nil {
		return nil
	}
	if result := snapshot.CurrentTurn.TurnResult; result != nil && len(result.Choices) > 0 {
		return result.Choices
	}
	if hot := snapshot.CurrentTurn.HotState; hot != nil && len(hot.Choices) > 0 {
		return hot.Choices
	}
	return nil
}

// storyAssistantActorContext renders the cast and the world from the projected
// state.
//
// The protagonist, the people on stage and the world entities are described by
// their visible fields, because those are the things the player is weighing.
// Someone the story has introduced but who is not in the scene is described
// only by name, role and description: their dossier is not what the player is
// deciding about, and dumping it would invite advice about a person who is not
// there.
func storyAssistantActorContext(known map[string]any, schema *interactive.ActorStateSchemaSnapshot, present []string) string {
	rawActors, _ := known[actorStateRoot].(map[string]any)
	if len(rawActors) == 0 {
		return ""
	}
	templates := map[string]interactive.ActorStateTemplate{}
	if schema != nil {
		// The runtime projection fills in initial Actors the journal has not
		// written yet and drops archived ones, so the assistant is not told about
		// someone who has left the story. It never adds a private field: the state
		// it is given has already been projected.
		projected := interactive.ActorStateRuntimeProjection(schema.System, known)
		if projectedActors, ok := projected[actorStateRoot].(map[string]any); ok {
			rawActors = projectedActors
		}
		for _, template := range schema.System.Templates {
			templates[strings.TrimSpace(template.ID)] = template
		}
	}

	onStage := map[string]bool{}
	for _, id := range present {
		if id = strings.TrimSpace(id); id != "" {
			onStage[id] = true
		}
	}
	ordered := make([]string, 0, len(rawActors))
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		if _, ok := rawActors[id]; !ok {
			return
		}
		seen[id] = true
		ordered = append(ordered, id)
	}
	add(interactive.DefaultActorID)
	for _, id := range present {
		add(id)
	}
	rest := make([]string, 0, len(rawActors))
	for id := range rawActors {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	for _, id := range rest {
		add(id)
	}

	blocks := make([]string, 0, len(ordered))
	for _, actorID := range ordered {
		record, _ := rawActors[actorID].(map[string]any)
		if record == nil {
			continue
		}
		templateID := strings.TrimSpace(fmt.Sprint(record["template_id"]))
		// The story-context Actor is the scene itself, already rendered above.
		if templateID == interactive.ActorStateStoryContextTemplateID {
			continue
		}
		full := actorID == interactive.DefaultActorID || onStage[actorID] ||
			templateID == interactive.ActorStateWorldEntitiesTemplateID
		if block := storyAssistantActorBlock(actorID, record, templates[templateID], full); block != "" {
			blocks = append(blocks, block)
		}
	}
	return storyAssistantBoundBlocks(blocks, storyAssistantContextMaxBytes/2)
}

func storyAssistantActorBlock(actorID string, record map[string]any, template interactive.ActorStateTemplate, full bool) string {
	name := strings.TrimSpace(fmt.Sprint(record["name"]))
	if name == "" || name == "<nil>" {
		name = actorID
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "#### %s\n\n", storyAssistantClip(name, 120))
	if role := strings.TrimSpace(fmt.Sprint(record["role"])); role != "" && role != "<nil>" {
		fmt.Fprintf(&sb, "- Role: %s\n", storyAssistantClip(role, 120))
	}
	if description := strings.TrimSpace(fmt.Sprint(record["description"])); description != "" && description != "<nil>" {
		fmt.Fprintf(&sb, "- Description: %s\n", storyAssistantClip(oneLineText(description), 400))
	}
	if !full {
		return strings.TrimSpace(sb.String())
	}
	rawState, _ := record["state"].(map[string]any)
	if len(rawState) == 0 {
		return strings.TrimSpace(sb.String())
	}
	if len(template.Fields) == 0 {
		// A story whose frozen schema is missing still has readable state; the
		// field names are the pack's own, so they are rendered as they are rather
		// than guessed into a meaning.
		keys := make([]string, 0, len(rawState))
		for key := range rawState {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if value := storyAssistantValue(interactive.ActorStateField{Name: key}, rawState[key]); value != "" {
				fmt.Fprintf(&sb, "- %s: %s\n", storyAssistantClip(key, 120), value)
			}
		}
		return strings.TrimSpace(sb.String())
	}
	for _, field := range template.Fields {
		fieldID := strings.TrimSpace(field.Name)
		if fieldID == "" {
			continue
		}
		value := storyAssistantValue(field, rawState[fieldID])
		if value == "" {
			continue
		}
		fmt.Fprintf(&sb, "- %s: %s\n", storyAssistantClip(fieldID, 120), value)
	}
	return strings.TrimSpace(sb.String())
}

// storyAssistantValue renders one field value.
//
// A value the story has not established reads as empty and is left out
// entirely. Sending a zero for a missing number would turn "nobody has said"
// into "the protagonist has none", which is exactly the confusion the
// player-facing projection exists to avoid.
func storyAssistantValue(field interactive.ActorStateField, value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return ""
		}
		if field.ValueFormat == interactive.ActorStateValueFormatStage {
			return storyAssistantClip(storyAssistantStageGloss(text), 120)
		}
		return storyAssistantClip(oneLineText(text), 240)
	case bool:
		if typed {
			return "yes"
		}
		return "no"
	case float64:
		return storyAssistantClip(fmt.Sprint(typed), 120)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	rendered := strings.TrimSpace(string(data))
	if rendered == "" || rendered == "null" || rendered == "[]" || rendered == "{}" {
		return ""
	}
	return storyAssistantClip(rendered, 240)
}

// storyAssistantStageGloss names a cultivation stage key so the model knows
// what the stored key means. The key stays in the text because it is the fact;
// the gloss is what makes the fact usable in advice.
func storyAssistantStageGloss(key string) string {
	stage, ok := cultivation.ParseStageKey(key)
	if !ok {
		return key
	}
	switch stage.Realm {
	case cultivation.RealmMortal:
		return key + " (mortal, no cultivation)"
	case cultivation.RealmQi:
		return fmt.Sprintf("%s (Qi Refining, layer %d)", key, stage.Layer)
	}
	realm := map[cultivation.Realm]string{
		cultivation.RealmFoundation: "Foundation Establishment",
		cultivation.RealmCore:       "Golden Core",
		cultivation.RealmNascent:    "Nascent Soul",
		cultivation.RealmSpirit:     "Spirit Transformation",
	}[stage.Realm]
	if realm == "" {
		return key
	}
	return fmt.Sprintf("%s (%s, %s)", key, realm, stage.Stage)
}

// storyAssistantBoundBlocks joins blocks under a byte ceiling and says so when
// it had to stop. A silently shortened context would let the model reason from
// a cast it believes is complete.
func storyAssistantBoundBlocks(blocks []string, limitBytes int) string {
	var sb strings.Builder
	truncated := false
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		separator := ""
		if sb.Len() > 0 {
			separator = "\n\n"
		}
		const notice = "\n\n> The rest of the cast was left out at the context limit. Do not assume anything about people who are not listed."
		if sb.Len()+len(separator)+len(block)+len(notice) > limitBytes {
			truncated = true
			break
		}
		sb.WriteString(separator)
		sb.WriteString(block)
	}
	if truncated {
		sb.WriteString("\n\n> The rest of the cast was left out at the context limit. Do not assume anything about people who are not listed.")
	}
	return strings.TrimSpace(sb.String())
}

// storyAssistantClip bounds one rendered value by rune count.
func storyAssistantClip(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
