package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"
	"ta/agent/providers"
	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
)

// CharacterChatRequest is one message the player sends in a character window.
type CharacterChatRequest struct {
	StoryID     string
	BranchID    string
	CharacterID string
	Message     string
	// CommandID is the caller-owned idempotency key. A retry with the same
	// command ID must not produce a second exchange or a second model call.
	CommandID string
	Locale    string
}

// CharacterChatResult is one committed reply.
type CharacterChatResult struct {
	StoryID     string `json:"story_id"`
	BranchID    string `json:"branch_id"`
	CharacterID string `json:"character_id"`
	// Reply is what the character said.
	Reply string `json:"reply"`
	// WorldTime is the in-fiction time the exchange was stamped with. It is the
	// time the conversation happened, not the time the player read the reply.
	WorldTime string `json:"world_time"`
	// ExchangeIDs are the committed exchange records, so the window can merge
	// them without a full reload.
	ExchangeIDs []string `json:"exchange_ids"`
	// Affinity is how this character feels about the protagonist after the
	// exchange, and AffinityDelta is how far this exchange moved it. Both are
	// read back from the committed record, so a repeated or already-settled fact
	// reports 0 rather than the number the model proposed.
	Affinity      int    `json:"affinity"`
	AffinityBand  string `json:"affinity_band"`
	AffinityDelta int    `json:"affinity_delta"`
	// Relationship is the formal bond now standing, if any.
	Relationship *character.Relationship `json:"relationship,omitempty"`
	// CharacterState is the refreshed projection after the reply was committed.
	CharacterState character.State `json:"character_state"`
}

// ErrCharacterUnknown means the requested character is not part of this story's
// cast. The window must not invent a conversation partner.
var ErrCharacterUnknown = fmt.Errorf("character is not part of this story")

// ErrCharacterContactRequiresStoryTurn means remote contact in a letter world
// must be carried by a main-story turn. Committed co-presence permits local talk.
var ErrCharacterContactRequiresStoryTurn = fmt.Errorf("this world sends letters through the main story")

// ErrCharacterNotPresent means the world allows character-window conversation
// only when the person is actually in the current scene.
var ErrCharacterNotPresent = fmt.Errorf("the character is not present in this scene")

// recentAffinityLimit bounds how many of a character's already-counted changes
// are shown to the model. It is a reminder of what has already been settled, not
// a history: enough that a repeated gift is visible, short enough not to become
// a ledger the model tries to continue.
const recentAffinityLimit = 6

// SendCharacterMessage runs one character-window reply and commits it.
//
// This is a one-shot model call rather than an admitted Agent cycle. A window
// conversation is not a story turn: it must not allocate a turn number, must
// not run the narrative tool protocol, and must not advance the branch head as
// a Turn. Committing the exchange through AppendCharacterFacts keeps the result
// on the same canonical journal, so one save still covers everything.
func (a *App) SendCharacterMessage(ctx context.Context, request CharacterChatRequest) (CharacterChatResult, error) {
	if a == nil {
		return CharacterChatResult{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	characterID := strings.TrimSpace(request.CharacterID)
	message := strings.TrimSpace(request.Message)
	if storyID == "" || characterID == "" {
		return CharacterChatResult{}, fmt.Errorf("a character message requires a story and a character")
	}
	if message == "" {
		return CharacterChatResult{}, fmt.Errorf("a character message requires text")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return CharacterChatResult{}, ErrNoWorkspace
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return CharacterChatResult{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	// The command fingerprint is resolved against the branch the store will
	// actually write to, not against the raw request, so an empty branch ID and
	// an explicit current branch are the same command rather than a conflict.
	commandID := strings.TrimSpace(request.CommandID)
	commandHash := characterChatCommandHash(request, branchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return CharacterChatResult{}, err
	}
	profile, ok := state.Profile(characterID)
	if !ok {
		return CharacterChatResult{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, characterID)
	}

	// A retry must not buy a second model call. The committed record is the
	// durable answer, so it is consulted before anything is generated: a lost
	// response, a repeated click or a recovery re-read all return the reply the
	// player already has rather than asking the character again.
	if commandID != "" {
		if committed, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return CharacterChatResult{}, err
		} else if found {
			return characterChatResultFrom(storyID, branchID, characterID, committed, state), nil
		}
	}

	// A committed command is a replay, not new contact with an absent person.
	if storyCommunicationMode(storyContext.Meta.PackBinding) == "letter" &&
		!character.ContainsID(interactive.StoryScene(storyContext.Snapshot).Present, characterID) {
		return CharacterChatResult{}, ErrCharacterChatUnavailable
	}

	worldTime := characterWorldTime(storyContext)
	scene := characterSceneFor(storyContext, characterID)
	if binding := storyContext.Meta.PackBinding; binding != nil && binding.Gameplay != nil {
		switch binding.Gameplay.Communication {
		case "letter":
			if !character.ContainsID(interactive.StoryScene(storyContext.Snapshot).Present, characterID) {
				return CharacterChatResult{}, ErrCharacterContactRequiresStoryTurn
			}
		case "local":
			if !character.ContainsID(scene.Present, characterID) {
				return CharacterChatResult{}, ErrCharacterNotPresent
			}
		case "radio":
			if !radioContactAvailable(storyContext, state, characterID) {
				return CharacterChatResult{}, ErrRadioContactUnavailable
			}
		}
	}
	loreScope := ""
	if binding := storyContext.Meta.PackBinding; binding != nil {
		loreScope = binding.Scope
	}
	loreRevision := ""
	if loreStore := lore.NewScopedStore(workspace, loreScope); loreStore != nil {
		if revision, err := loreStore.Revision(); err == nil {
			loreRevision = revision
		}
	}
	affinity, band := state.AffinityFor(characterID)
	var relationship *character.Relationship
	if current, ok := state.RelationshipFor(characterID); ok {
		relationship = &current
	}
	prepared := character.Prepare(character.PrepareInput{
		StoryID:   storyID,
		Profile:   profile,
		Scene:     scene,
		Revision:  characterIdentityFor(storyContext, branchID, profile, loreRevision),
		Exchanges: state.ExchangesFor(characterID),
		Knowledge: state.KnowledgeFor(characterID),
		Matters:   state.MattersFor(characterID),
		Lore:      characterLoreFor(workspace, loreScope, profile, scene, 0, storyContext),
		// The player's own line is the retrieval condition for the recall pass.
		// Without it a window reply is a tool-less one-shot with no route to
		// anything outside the bounded memory block, and the character answers
		// "I don't remember" about things they lived through.
		Question: message,
		// The same reading the main story's cast block and the character's own
		// circumstances use, so a number earned in the window and a number earned
		// in the story cannot disagree.
		Affinity:         affinity,
		AffinityBand:     band,
		Relationship:     relationship,
		RecentAffinities: character.RecentAffinityChanges(characterID, state.Affinities, recentAffinityLimit),
		// The branch's formal instant, so a thread whose deadline was bound to the
		// clock is split by that number rather than by its own label.
		ClockSeconds: interactive.SnapshotClockSeconds(storyContext.Snapshot),
	})

	runtimeCfg := *cfg
	runtimeCfg.Workspace = workspace
	replyTarget := storyContext.Meta.ReplyTargetChars
	if replyTarget <= 0 {
		replyTarget = prompts.DefaultCharacterReplyTargetChars
	}
	// A window reply is deliberately shorter than a narrative turn: it is a line
	// of conversation, not a scene.
	if replyTarget > 600 {
		replyTarget = 600
	}
	systemInstruction := prompts.BuildCharacterChatSystemInstruction(prompts.CharacterChatSystemInstructionInput{
		Workspace:        workspace,
		ReplyTargetChars: replyTarget,
		CharacterContext: prepared.Markdown(),
		StoryContext:     characterStoryContext(storyContext, scene, profile),
		CharacterID:      characterID,
		// A window reply uses the dialogue purpose's expression fragment. It does
		// not import the narrative voice, the ensemble viewpoint, or the full
		// long-form requirements: this is one person speaking, and their own
		// profile and knowledge boundaries outrank any expression rule.
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		// The page the player is reading decides the reply's language. The world
		// material does not: a Chinese world read on an English page is answered
		// in English.
		OutputLanguage: request.Locale,
	})
	if storyCommunicationMode(storyContext.Meta.PackBinding) == "radio" {
		systemInstruction += "\n\n当前交流通过无线电进行，只有双方接收本次私聊。远端人物不因通话而到场，不能看到本地动作、物品或第三者，除非玩家在通信中描述。只依据人物已知事实与此次通信作答；不把无线电写成传音、书信或面对面接触。"
	}

	reply, err := a.generateCharacterReply(ctx, &runtimeCfg, storyID, branchID, systemInstruction, message, config.CharacterChatThinkingLevel(runtimeCfg.AgentModels))
	if err != nil {
		return CharacterChatResult{}, err
	}
	payload, err := parseCharacterReplyPayload(reply, characterID, character.OpenMatters(characterID, state.MattersFor(characterID)))
	if err != nil {
		return CharacterChatResult{}, err
	}

	// Both halves of the exchange are committed together, so a reply can never be
	// stored without the message it answers.
	//
	// Matters belong to the same record as the words that created them. An
	// arrangement reached in the window is what the next story turn has to honour,
	// and the group window already commits its own; leaving them out here would
	// make a private agreement invisible to the story that follows it.
	facts := character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{{
			CharacterID: characterID, Speaker: character.SpeakerPlayer,
			Channel: character.ChannelPrivate, Text: message,
		}, {
			CharacterID: characterID, Speaker: character.SpeakerCharacter,
			Channel: character.ChannelPrivate, Text: payload.Reply,
		}},
		Affinities:    payload.Affinities,
		Relationships: payload.Relationships,
		Matters:       payload.Matters,
	}
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:    branchID,
		WorldTime:   worldTime,
		Scene:       scene,
		CommandID:   commandID,
		CommandHash: commandHash,
		Facts:       facts,
	})
	if err != nil {
		return CharacterChatResult{}, err
	}
	refreshed, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return CharacterChatResult{}, err
	}
	exchangeIDs := make([]string, 0, len(projected.Exchanges))
	for _, exchange := range projected.Exchanges {
		exchangeIDs = append(exchangeIDs, exchange.ID)
	}
	// The reply the caller is shown is read back from what was committed, not
	// taken from this attempt's generation. A concurrent click, a retried
	// request or a recovery re-read can lose the race to commit, and the store
	// then returns the records the winner wrote: answering with the reply this
	// attempt happened to generate would show the player a line the journal does
	// not have, and the two clients would disagree about what the character said.
	committed := committedCharacterReply(projected.Exchanges, characterID)
	if committed == "" {
		committed = payload.Reply
	}
	result := characterChatResultFrom(storyID, branchID, characterID, projected, refreshed)
	result.Reply = committed
	result.WorldTime = worldTime
	result.ExchangeIDs = exchangeIDs
	slog.InfoContext(ctx, fmt.Sprintf("[character-chat] reply committed story_id=%s branch_id=%s character_id=%s reply_chars=%d affinity_delta=%d", storyID, branchID, characterID, len([]rune(committed)), result.AffinityDelta))
	return result, nil
}

// committedCharacterReply picks the character's own line out of the exchanges
// that were actually written. The player's half is skipped so a reply can never
// be read back as the player's own message.
func committedCharacterReply(exchanges []character.Exchange, characterID string) string {
	for _, exchange := range exchanges {
		if exchange.CharacterID == characterID && exchange.Speaker == character.SpeakerCharacter {
			return exchange.Text
		}
	}
	return ""
}

// characterChatCommandHash fingerprints the player action a command ID stands
// for. It covers what the player asked and who they asked, and deliberately not
// the reply: a retry sends the same request, and the reply is whatever the first
// attempt got. Reusing one command ID for a different message is a conflict
// rather than a silent replay of the wrong answer.
func characterChatCommandHash(request CharacterChatRequest, branchID string) string {
	payload := strings.Join([]string{
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		strings.TrimSpace(request.CharacterID),
		strings.TrimSpace(request.Message),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// characterChatResultFrom rebuilds the caller's view of an exchange that is
// already committed. It is how a retry answers without generating: the reply is
// read back from the journal, so the player sees the same words a second time
// rather than a second, differently-worded reply.
func characterChatResultFrom(storyID, branchID, characterID string, committed character.Projected, state character.State) CharacterChatResult {
	exchangeIDs := make([]string, 0, len(committed.Exchanges))
	reply := ""
	worldTime := ""
	for _, exchange := range committed.Exchanges {
		exchangeIDs = append(exchangeIDs, exchange.ID)
		if exchange.Speaker == character.SpeakerCharacter {
			reply = exchange.Text
			worldTime = exchange.WorldTime
		}
	}
	result := CharacterChatResult{
		StoryID: storyID, BranchID: branchID, CharacterID: characterID,
		Reply: reply, WorldTime: worldTime, ExchangeIDs: exchangeIDs, CharacterState: state,
	}
	value, band := state.AffinityFor(characterID)
	result.Affinity = value
	result.AffinityBand = string(band)
	if relationship, ok := state.RelationshipFor(characterID); ok {
		result.Relationship = &relationship
	}
	// The delta is read back from the record this call wrote, not from what the
	// model proposed. A settled fact is a no-op, and the window must say nothing
	// happened rather than show a number the journal refused.
	for _, change := range committed.Affinities {
		if change.CharacterID == characterID {
			result.AffinityDelta += change.Delta
		}
	}
	return result
}

// characterReplyPayload is the window's structured answer: the spoken line plus
// whatever this exchange actually changed about the character's feeling, the
// formal bond between them, and the threads either side now owes.
type characterReplyPayload struct {
	Reply         string                        `json:"reply"`
	Affinities    []character.AffinityDraft     `json:"affinities"`
	Relationships []character.RelationshipDraft `json:"relationships"`
	Matters       []character.MatterDraft       `json:"matters"`
}

// parseCharacterReplyPayload validates one window answer.
//
// The reply must survive whatever the model wrapped it in: a JSON object, a
// fenced one, or (when the model ignored the contract) plain prose. Plain prose
// is accepted as the reply with no changes, because the conversation itself did
// happen and refusing it would lose the player's line for a formatting fault.
// Anything else that cannot be read is an error: a reply that was committed as
// raw JSON would put the whole object in the player's transcript.
//
// A change naming anyone but this character is dropped rather than committed.
// The window is a one-to-one conversation, so it has no standing to move a third
// party's feeling, and silently attributing it would invent an interaction that
// never happened.
//
// A matter this character does not own is dropped for the same reason, and a
// matter whose shape the journal would refuse is dropped rather than raised: a
// window reply is one model call with no repair pass, so an error here would
// discard the player's own message over a malformed extra.
func parseCharacterReplyPayload(content, characterID string, openMatters []character.Matter) (characterReplyPayload, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	text = strings.TrimSpace(text)
	if text == "" {
		return characterReplyPayload{}, fmt.Errorf("the character returned an empty reply")
	}
	if !strings.HasPrefix(text, "{") {
		return characterReplyPayload{Reply: text}, nil
	}
	var payload characterReplyPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return characterReplyPayload{}, fmt.Errorf("the character returned an unreadable answer: %w", err)
	}
	payload.Reply = strings.TrimSpace(payload.Reply)
	if payload.Reply == "" {
		return characterReplyPayload{}, fmt.Errorf("the character returned an answer with no reply")
	}
	affinities := make([]character.AffinityDraft, 0, len(payload.Affinities))
	for _, draft := range payload.Affinities {
		if id := strings.TrimSpace(draft.CharacterID); id != "" && id != characterID {
			continue
		}
		draft.CharacterID = characterID
		draft.Kind = character.AffinityKind(strings.TrimSpace(string(draft.Kind)))
		draft.Basis = strings.TrimSpace(draft.Basis)
		draft.BasisRef = strings.TrimSpace(draft.BasisRef)
		affinities = append(affinities, draft)
	}
	relationships := make([]character.RelationshipDraft, 0, len(payload.Relationships))
	for _, draft := range payload.Relationships {
		if id := strings.TrimSpace(draft.CharacterID); id != "" && id != characterID {
			continue
		}
		draft.CharacterID = characterID
		draft.Kind = character.RelationshipKind(strings.TrimSpace(string(draft.Kind)))
		draft.Statement = strings.TrimSpace(draft.Statement)
		relationships = append(relationships, draft)
	}
	matters := make([]character.MatterDraft, 0, len(payload.Matters))
	for _, draft := range payload.Matters {
		if id := strings.TrimSpace(draft.CharacterID); id != "" && id != characterID {
			continue
		}
		draft.CharacterID = characterID
		draft.Kind = character.MatterKind(strings.TrimSpace(string(draft.Kind)))
		draft.Status = character.MatterStatus(strings.TrimSpace(string(draft.Status)))
		draft.Statement = strings.TrimSpace(draft.Statement)
		draft.Counterparty = strings.TrimSpace(draft.Counterparty)
		draft.Deadline = strings.TrimSpace(draft.Deadline)
		draft.Resolves = strings.TrimSpace(draft.Resolves)
		if draft.Statement == "" || !draft.Kind.Valid() || (draft.Status != "" && !draft.Status.Valid()) {
			continue
		}
		if draft.Status == "" {
			draft.Status = character.MatterOpen
		}
		// The window is not a letter channel, so nothing here can be a written
		// reply to a delivered letter.
		draft.LetterReply = nil
		// Only a thread this character actually holds open can be settled, and only
		// by a draft that is itself resolved. Anything else would be refused by the
		// journal, so the link is dropped and the new thread kept as it stands.
		if draft.Resolves != "" {
			open := false
			for _, matter := range openMatters {
				if matter.ID == draft.Resolves && matter.CharacterID == characterID && matter.Status == character.MatterOpen {
					open = true
					break
				}
			}
			if !open || draft.Status == character.MatterOpen {
				draft.Resolves = ""
			}
		}
		matters = append(matters, draft)
	}
	return characterReplyPayload{Reply: payload.Reply, Affinities: affinities, Relationships: relationships, Matters: matters}, nil
}

// generateCharacterReply performs the window's single model call. It is a
// bounded one-shot generation: no tools, no session, no admitted turn.
//
// level comes from the saved character-chat purpose rather than being fixed
// here, so the control the player sees is the control that is sent. An unset
// purpose resolves to off: a character window is a conversation, not an
// authored scene, so the reply should be the character speaking rather than a
// draft the model reasons its way into.
func (a *App) generateCharacterReply(ctx context.Context, cfg *config.Config, storyID, branchID, systemInstruction, message string, level providers.ThinkingLevel) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindCharacterChat)
	if err != nil {
		return "", fmt.Errorf("resolve character model configuration: %w", err)
	}
	if level == "" {
		level = config.CharacterChatThinkingDefault
	}
	modelCfg.ThinkingLevel = level
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "character", cfg.Workspace,
		"character_chat", "Character Conversation Rules",
		"define how one story character speaks in a private conversation",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(characterChatTurnMessage(message)),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindCharacterChat, messages, nil); err != nil {
		return "", err
	}

	// The reply carries two things a person's words cannot: the settled change
	// of feeling, and any formal bond. A prose-only answer would leave the
	// affinity rule unenforced, so the call asks the provider for a JSON object
	// and retries once without that hint for endpoints that do not support it.
	content, err := a.callCharacterModel(ctx, cfg, modelio.WithJSONObjectOutput(modelCfg), storyID, branchID, message, messages)
	if err == nil {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[character-chat] json_mode failed, retry without response_format err=%v", err))
	return a.callCharacterModel(ctx, cfg, modelCfg, storyID, branchID, message, messages)
}

func (a *App) callCharacterModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, storyID, branchID, message string, messages []*agent.Message) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "character_chat", "reply", map[string]any{
		"story_id": storyID, "branch_id": branchID, "message_chars": len([]rune(message)),
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create character model: %w", err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "character_chat", "reply", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "character_chat", "reply", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate character reply: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the character model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "character_chat", "reply", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "character_chat", "reply", modelCfg.Model, 0, msg, nil, nil)
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = strings.TrimSpace(msg.ReasoningContent)
	}
	return content, nil
}

// characterChatTurnMessage repeats the answer shape beside the player's own
// line. The system prompt states it once, but a long in-character context reads
// as an instruction to write prose, and the reminder at the point of answering
// is what keeps the reply machine-readable. Only the words above the rule are
// the player's message; the committed exchange stores the player's line alone.
func characterChatTurnMessage(message string) string {
	return strings.TrimSpace(message) + "\n\n---\n\n" +
		"Answer as the JSON object specified in your instructions: " +
		`{"reply": "...", "affinities": [], "relationships": [], "matters": []}. ` +
		"Output nothing outside that object."
}
