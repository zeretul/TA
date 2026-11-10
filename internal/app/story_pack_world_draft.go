package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"
	"ta/agent/providers"
	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// Creating a world drafts a world document and writes nothing.
//
// 自制-AI创建世界包与武侠规则复用 §2 puts two entry points in front of the same
// draft: a guided one that asks questions, and an automatic one that takes a
// sentence. A player may start either way, switch, edit what came back, ask for
// it again, and save only when they choose to. This file is the middle act, and
// it deliberately creates no Project, no Book, no story and no file.
//
// §6 divides the labour. The model writes prose: the premise, the places, the
// people, the situation. Everything a rule reads — the module ids, the aptitude
// tiers, the starting purse, the state a ready-made character begins with — is
// assembled by storypack.WorldDraft.BuildPack from a fixed, supported template.
// That is why the same draft can be edited by a client and still cannot start a
// character richer than a built-in world's.

// StoryPackWorldDraftRequest is one draft request.
//
// Mode names the entry point. Draft is the document as the page currently holds
// it, so a switch between entry points, a retry after a failure, or a request to
// regenerate all carry the player's own edits rather than starting over.
type StoryPackWorldDraftRequest struct {
	// Mode is guided or automatic.
	Mode string `json:"mode,omitempty"`
	// The player's seed material. Automatic play fills only Brief.
	Name        string                  `json:"name,omitempty"`
	Genre       string                  `json:"genre,omitempty"`
	Brief       string                  `json:"brief,omitempty"`
	Tone        string                  `json:"tone,omitempty"`
	Protagonist string                  `json:"protagonist,omitempty"`
	OpeningIdea string                  `json:"opening_idea,omitempty"`
	Gameplay    storypack.WorldGameplay `json:"gameplay"`
	Draft       *storypack.WorldDraft   `json:"draft,omitempty"`
	// Rewrite asks for the prose to be written again from the same seed. It is
	// the "重新生成" action; a failure leaves the draft untouched.
	Rewrite bool `json:"rewrite,omitempty"`
	// Locale is the page's language, taken from the request header rather than
	// the body. A refusal the player reads is written in it.
	Locale string `json:"-"`
}

// StoryPackWorldDraft is one drafted world plus what the page needs to render it.
type StoryPackWorldDraft struct {
	Draft   storypack.WorldDraft   `json:"draft"`
	Preview storypack.WorldPreview `json:"preview"`
	// Rewritten says which action produced this draft, so the page can label it.
	Rewritten bool `json:"rewritten,omitempty"`
	// Error is a draft that did not finish. Draft still carries what the player
	// had, because losing their edits on a failed call would be worse than the
	// failure itself.
	Error    string `json:"error,omitempty"`
	ErrorKey string `json:"error_key,omitempty"`
}

// DraftWorld asks the model for the world's prose and merges it into the draft.
//
// A rewrite regenerates the prose from the seed. Anything else fills only what
// is missing, so a player who edited one place and then switched entry point
// keeps that edit.
func (s *StoryPackService) DraftWorld(ctx context.Context, request StoryPackWorldDraftRequest) (StoryPackWorldDraft, error) {
	dataRoot, err := s.dataRoot()
	if err != nil {
		return StoryPackWorldDraft{}, err
	}

	seed := worldDraftSeed{
		Mode:        normalizeWorldDraftMode(request.Mode),
		Name:        strings.TrimSpace(request.Name),
		Genre:       strings.TrimSpace(request.Genre),
		Brief:       strings.TrimSpace(request.Brief),
		Tone:        strings.TrimSpace(request.Tone),
		Protagonist: strings.TrimSpace(request.Protagonist),
		OpeningIdea: strings.TrimSpace(request.OpeningIdea),
		Gameplay:    request.Gameplay,
	}
	if seed.Gameplay.StatusTemplate == "" {
		labels := seed.Gameplay.Presentation
		seed.Gameplay = storypack.RecommendedWorldGameplay()
		seed.Gameplay.Presentation = labels
	}
	draft := storypack.WorldDraft{}
	if request.Draft != nil {
		draft = *request.Draft
	}
	if seed.Name != "" {
		draft.Name = seed.Name
	}
	if seed.Gameplay.Presentation == nil {
		// Older clients omit the new field when rewriting prose. Only an explicit
		// empty map clears authored vocabulary; omission preserves the draft.
		seed.Gameplay.Presentation = draft.Gameplay.Presentation
	}
	if seed.Gameplay.StatusTemplate == draft.Gameplay.StatusTemplate {
		if seed.Gameplay.WorldContent == nil {
			seed.Gameplay.WorldContent = draft.Gameplay.WorldContent
		}
		if seed.Gameplay.CultivationRules == nil {
			seed.Gameplay.CultivationRules = draft.Gameplay.CultivationRules
		}
	}
	seed.Gameplay.Presentation = seed.Gameplay.Presentation.Clone()
	if draft.Gameplay.StatusTemplate == "" || request.Rewrite || request.Gameplay.Confirmed {
		draft.Gameplay = seed.Gameplay
	}
	draft.Gameplay.Presentation = draft.Gameplay.Presentation.Clone()

	// The seed alone is not enough to build a world: without a premise there is
	// nothing for a first scene to open on. That is a refusal the player can act
	// on, so it is reported before a model call is spent.
	if strings.TrimSpace(seed.Brief) == "" && strings.TrimSpace(draft.Origin) == "" {
		return StoryPackWorldDraft{
			Draft:    draft,
			Preview:  previewWorldDraft(dataRoot, draft),
			ErrorKey: "worldCreation.premiseRequired",
		}, nil
	}
	if gameplayErr := storypack.ValidateWorldGameplay(draft.Gameplay); gameplayErr != nil {
		var validationErr *storypack.WorldGameplayValidationError
		if errors.As(gameplayErr, &validationErr) {
			return StoryPackWorldDraft{Draft: draft, Preview: previewWorldDraft(dataRoot, draft), ErrorKey: validationErr.Key}, nil
		}
		return StoryPackWorldDraft{Draft: draft, Preview: previewWorldDraft(dataRoot, draft), ErrorKey: "worldCreation.gameplayInvalid"}, nil
	}

	runtimeCfg, err := s.app.storyPackRuntimeConfig()
	if err != nil {
		return StoryPackWorldDraft{Draft: draft, Preview: previewWorldDraft(dataRoot, draft)}, err
	}

	gameplayProfile := draft.Gameplay.StatusTemplate + "; communication=" + draft.Gameplay.Communication
	if terms := draft.Gameplay.Presentation.ForLocale(request.Locale); terms != (interactive.WorldPresentationTerms{}) {
		labels, _ := json.Marshal(terms)
		gameplayProfile += "\n本局作者词汇：" + string(labels) + "。只按这些名称表达，不因名称新增成长、法术、通信或战斗能力。"
	}
	instruction := prompts.BuildWorldCreationSystemInstruction(prompts.WorldCreationSystemInstructionInput{
		Mode:            seed.Mode,
		Name:            seed.Name,
		Genre:           seed.Genre,
		Brief:           seed.Brief,
		Tone:            seed.Tone,
		Protagonist:     seed.Protagonist,
		OpeningIdea:     seed.OpeningIdea,
		GameplayProfile: gameplayProfile,
		OutputLanguage:  request.Locale,
		Existing:        worldDraftPromptInput(draft, request.Rewrite),
	})

	content, err := s.app.generateWorldCreationDraft(ctx, &runtimeCfg, instruction, request.Locale)
	if err != nil {
		slog.WarnContext(ctx, "[story-pack] failed to draft a world", "phase", "model", "error", err)
		return StoryPackWorldDraft{
			Draft: draft, Preview: previewWorldDraft(dataRoot, draft), ErrorKey: worldDraftErrorValue(err),
		}, nil
	}
	drafted, err := parseWorldDraftPayload(content)
	if err != nil {
		slog.WarnContext(ctx, "[story-pack] failed to parse a world draft", "phase", "response", "error", err)
		return StoryPackWorldDraft{
			Draft: draft, Preview: previewWorldDraft(dataRoot, draft), ErrorKey: worldDraftErrorValue(err),
		}, nil
	}

	draft = mergeWorldDraft(draft, drafted, request.Rewrite, seed)
	draft.Notice = storypack.DraftNoticeForGameplay(draft.Gameplay)
	slog.InfoContext(ctx, "[story-pack] drafted a world",
		"mode", seed.Mode, "rewrite", request.Rewrite,
		"world_entries", len(draft.World), "characters", len(draft.Characters))
	return StoryPackWorldDraft{
		Draft:     draft,
		Preview:   previewWorldDraft(dataRoot, draft),
		Rewritten: request.Rewrite,
	}, nil
}

// PreviewWorldDraft reports what an edited draft would become.
//
// The preview page lets a player rewrite any prose field without asking the
// model again. That edit has to be checked the same way a save would check it,
// or the page would offer a start that the save then refuses; so this runs the
// same three checks and writes nothing.
func (s *StoryPackService) PreviewWorldDraft(draft storypack.WorldDraft) (storypack.WorldPreview, error) {
	dataRoot, err := s.dataRoot()
	if err != nil {
		return storypack.WorldPreview{}, err
	}
	return previewWorldDraft(dataRoot, draft), nil
}

// SaveWorldDraft writes a draft as a new user world.
//
// Saving is the only act that creates anything. It is create-only and never
// replaces an existing world, so pressing save twice produces two worlds rather
// than destroying the first.
func (s *StoryPackService) SaveWorldDraft(ctx context.Context, draft storypack.WorldDraft) (storypack.WorldSaveResult, error) {
	dataRoot, err := s.dataRoot()
	if err != nil {
		return storypack.WorldSaveResult{}, err
	}
	if strings.TrimSpace(draft.Notice) == "" {
		draft.Notice = storypack.DraftNoticeForGameplay(draft.Gameplay)
	}
	result, err := storypack.NewLibrary(dataRoot).SaveWorld(ctx, draft)
	if err != nil {
		return storypack.WorldSaveResult{}, err
	}
	slog.InfoContext(ctx, "[story-pack] saved a generated world", "pack_id", result.PackID)
	return result, nil
}

// worldDraftSeed is the player's own material, kept apart from the draft so a
// rewrite can regenerate prose without discarding what they typed.
type worldDraftSeed struct {
	Mode        string
	Name        string
	Genre       string
	Brief       string
	Tone        string
	Protagonist string
	OpeningIdea string
	Gameplay    storypack.WorldGameplay
}

func normalizeWorldDraftMode(mode string) string {
	if strings.TrimSpace(mode) == prompts.WorldCreationModeAutomatic {
		return prompts.WorldCreationModeAutomatic
	}
	return prompts.WorldCreationModeGuided
}

// worldDraftPromptInput is the "already agreed" block. On a rewrite it is empty,
// because regenerating means replacing the prose, not keeping it.
func worldDraftPromptInput(draft storypack.WorldDraft, rewrite bool) prompts.WorldDraftInput {
	if rewrite {
		return prompts.WorldDraftInput{}
	}
	input := prompts.WorldDraftInput{
		Name:            draft.Name,
		Description:     draft.Description,
		Title:           draft.Title,
		Origin:          draft.Origin,
		ProtagonistName: draft.ProtagonistName,
		Protagonist:     draft.Protagonist,
		Opening:         draft.OpeningText,
	}
	for _, entry := range draft.World {
		input.WorldEntries = append(input.WorldEntries, prompts.WorldEntryDraftInput{
			Audience: lore.NormalizeAudience(entry.Audience),
			Name: entry.Name, Type: entry.Type, Brief: entry.BriefDescription, Content: entry.Content, LoadMode: entry.LoadMode, Keywords: entry.Keywords,
		})
	}
	for _, character := range draft.Characters {
		input.Characters = append(input.Characters, prompts.WorldCharacterDraftInput{
			Name: character.Name, Age: character.Age, Gender: character.Gender,
			SpeechStyle: character.SpeechStyle, Role: character.Role, Place: character.Place,
			Brief: character.Brief, Aliases: character.Aliases,
		})
	}
	for _, origin := range draft.Origins {
		input.Origins = append(input.Origins, prompts.WorldOriginDraftInput{
			Name: origin.Name, Description: origin.Description, Basis: origin.Basis,
			Technique: origin.Technique, Item: origin.Item,
		})
	}
	for _, option := range draft.Affiliations {
		input.Affiliations = append(input.Affiliations, option.Name)
	}
	for _, option := range draft.Locations {
		input.Locations = append(input.Locations, option.Name)
	}
	return input
}

// worldDraftPayload is the model's structured answer.
type worldDraftPayload struct {
	Name            string                   `json:"name"`
	Description     string                   `json:"description"`
	Title           string                   `json:"title"`
	Origin          string                   `json:"origin"`
	ProtagonistName string                   `json:"protagonist_name"`
	Protagonist     string                   `json:"protagonist"`
	Opening         string                   `json:"opening"`
	World           []worldDraftEntryPayload `json:"world"`
	Characters      []struct {
		Name        string   `json:"name"`
		Age         int      `json:"age,omitempty"`
		Gender      string   `json:"gender,omitempty"`
		SpeechStyle string   `json:"speech_style,omitempty"`
		Role        string   `json:"role"`
		Place       string   `json:"place"`
		Brief       string   `json:"brief"`
		Aliases     []string `json:"aliases"`
	} `json:"characters"`
	Origins []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Basis       string `json:"basis"`
		Technique   string `json:"technique"`
		Item        string `json:"item"`
	} `json:"origins"`
	Affiliations []string `json:"affiliations"`
	Locations    []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"locations"`
}

// parseWorldDraftPayload reads one draft. A fenced object is read as if it were
// not fenced; a response that is not an object at all is a failure rather than
// an empty world, because an empty world would look like a draft that came back
// with nothing in it.
func parseWorldDraftPayload(content string) (worldDraftPayload, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return worldDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.world.unreadable"}
	}
	var payload worldDraftPayload
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return worldDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.world.unparsable", Args: []any{"detail", err.Error()}}
	}
	if strings.TrimSpace(payload.Origin) == "" {
		return worldDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.world.premiseEmpty"}
	}
	return payload, nil
}

// mergeWorldDraft applies the model's answer to the draft.
//
// On a rewrite the prose is replaced; otherwise a field the player already has
// is kept word for word, so switching entry point or retrying a failed call can
// never overwrite an edit. The player's seed always wins over the model.
func mergeWorldDraft(draft storypack.WorldDraft, payload worldDraftPayload, rewrite bool, seed worldDraftSeed) storypack.WorldDraft {
	keep := func(current, drafted string) string {
		if !rewrite && strings.TrimSpace(current) != "" {
			return current
		}
		if strings.TrimSpace(drafted) != "" {
			return strings.TrimSpace(drafted)
		}
		return current
	}

	draft.Name = keep(draft.Name, payload.Name)
	draft.Description = keep(draft.Description, payload.Description)
	draft.Title = keep(draft.Title, payload.Title)
	draft.Origin = keep(draft.Origin, payload.Origin)
	draft.ProtagonistName = keep(draft.ProtagonistName, payload.ProtagonistName)
	draft.Protagonist = keep(draft.Protagonist, payload.Protagonist)
	draft.OpeningText = keep(draft.OpeningText, payload.Opening)

	// The player's own answers are not the model's to change.
	if seed.Name != "" {
		draft.Name = seed.Name
	}
	if seed.Protagonist != "" {
		draft.Protagonist = seed.Protagonist
	}

	if rewrite || len(draft.World) == 0 {
		// Preserve known audiences by name. A renamed secret cannot be
		// identified reliably, so unmatched entries inherit a GM default when
		// rewriting a draft containing GM material unless explicitly public.
		hasExistingGM := false
		for _, previous := range draft.World {
			if lore.NormalizeAudience(previous.Audience) == "gm" {
				hasExistingGM = true
				break
			}
		}
		world := append([]worldDraftEntryPayload(nil), payload.World...)
		for index := range world {
			audience := strings.TrimSpace(world[index].Audience)
			if audience == "gm" || audience == "public" {
				continue
			}
			matched := false
			for _, previous := range draft.World {
				if strings.EqualFold(strings.TrimSpace(previous.Name), strings.TrimSpace(world[index].Name)) {
					world[index].Audience = previous.Audience
					matched = true
					break
				}
			}
			if !matched && hasExistingGM {
				world[index].Audience = "gm"
			}
		}
		if entries := mergeWorldEntries(world); len(entries) > 0 {
			draft.World = entries
		}
	}
	if rewrite || len(draft.Characters) == 0 {
		if characters := mergeWorldCharacters(payload.Characters); len(characters) > 0 {
			draft.Characters = characters
		}
	}
	if rewrite || len(draft.Origins) == 0 {
		if origins := mergeWorldOrigins(payload.Origins); len(origins) > 0 {
			draft.Origins = origins
		}
	}
	if rewrite || len(draft.Affiliations) == 0 {
		if options := mergeWorldOptions(payload.Affiliations); len(options) > 0 {
			draft.Affiliations = options
		}
	}
	if rewrite || len(draft.Locations) == 0 {
		locations := make([]storypack.DraftOption, 0, len(payload.Locations))
		for index, location := range payload.Locations {
			if name := strings.TrimSpace(location.Name); name != "" {
				locations = append(locations, storypack.DraftOption{
					Name: name, Description: strings.TrimSpace(location.Description), Default: index == 0,
				})
			}
		}
		if len(locations) > 0 {
			draft.Locations = locations
		}
	}
	return draft
}

type worldDraftEntryPayload struct {
	Audience string   `json:"audience,omitempty"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Brief    string   `json:"brief"`
	Content  string   `json:"content"`
	Keywords []string `json:"keywords,omitempty"`
	LoadMode string   `json:"load_mode,omitempty"`
}

func mergeWorldEntries(entries []worldDraftEntryPayload) []storypack.DraftWorldEntry {
	out := make([]storypack.DraftWorldEntry, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		out = append(out, storypack.DraftWorldEntry{
			Audience:         lore.NormalizeAudience(entry.Audience),
			Name:             name,
			Type:             strings.TrimSpace(entry.Type),
			Keywords:         entry.Keywords,
			LoadMode:         strings.TrimSpace(entry.LoadMode),
			BriefDescription: strings.TrimSpace(entry.Brief),
			Content:          strings.TrimSpace(entry.Content),
		})
	}
	return out
}

func mergeWorldCharacters(characters []struct {
	Name        string   `json:"name"`
	Age         int      `json:"age,omitempty"`
	Gender      string   `json:"gender,omitempty"`
	SpeechStyle string   `json:"speech_style,omitempty"`
	Role        string   `json:"role"`
	Place       string   `json:"place"`
	Brief       string   `json:"brief"`
	Aliases     []string `json:"aliases"`
}) []storypack.DraftCharacter {
	out := make([]storypack.DraftCharacter, 0, len(characters))
	for _, character := range characters {
		name := strings.TrimSpace(character.Name)
		if name == "" {
			continue
		}
		out = append(out, storypack.DraftCharacter{
			Name:        name,
			Age:         character.Age,
			Gender:      character.Gender,
			SpeechStyle: strings.TrimSpace(character.SpeechStyle),
			Role:        strings.TrimSpace(character.Role),
			Place:       strings.TrimSpace(character.Place),
			Brief:       strings.TrimSpace(character.Brief),
			Aliases:     character.Aliases,
			Level:       "重要",
			PlaceLabel:  strings.TrimSpace(character.Place),
		})
	}
	return out
}

func mergeWorldOrigins(origins []struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Basis       string `json:"basis"`
	Technique   string `json:"technique"`
	Item        string `json:"item"`
}) []storypack.DraftOrigin {
	out := make([]storypack.DraftOrigin, 0, len(origins))
	seen := make(map[string]bool, len(origins))
	for _, origin := range origins {
		name := strings.TrimSpace(origin.Name)
		if name == "" {
			continue
		}
		basis := strings.TrimSpace(origin.Basis)
		// Only the three positions the rules provide may be offered. A fourth
		// would have no state behind it, so an unknown basis is dropped rather
		// than silently mapped onto one of the real ones.
		if !storypack.IsDraftBasis(basis) {
			continue
		}
		if seen[basis] {
			continue
		}
		seen[basis] = true
		out = append(out, storypack.DraftOrigin{
			Name: name, Description: strings.TrimSpace(origin.Description), Basis: basis,
			Technique: strings.TrimSpace(origin.Technique), Item: strings.TrimSpace(origin.Item),
		})
	}
	return out
}

func mergeWorldOptions(values []string) []storypack.DraftOption {
	out := make([]storypack.DraftOption, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		// "无门无派" is the program's, and offering it twice would put two
		// identical choices on the page.
		if name == "" || name == "无门无派" {
			continue
		}
		out = append(out, storypack.DraftOption{Name: name})
	}
	return out
}

// previewWorldDraft builds the pack the draft describes and reports whether it
// could start a character.
//
// It builds rather than inspects, because the counts and the readiness the page
// shows must be the ones the saved world really has. A draft too thin to build
// still previews, with the build's own complaint as the reason.
func previewWorldDraft(dataRoot string, draft storypack.WorldDraft) storypack.WorldPreview {
	return storypack.PreviewWorldDraft(dataRoot, draft)
}

func worldDraftErrorValue(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrNoDraftingModel) {
		return err.Error()
	}
	var inputLimit *modelio.ProviderInputLimitError
	if errors.As(err, &inputLimit) {
		return "worldCreation.draftTooLong"
	}
	return "worldCreation.draftFailed"
}

// generateWorldCreationDraft performs the one-shot model call. It mirrors the
// character draft: standalone trace, one system instruction, one user message,
// no tools, no turn, no write.
func (a *App) generateWorldCreationDraft(ctx context.Context, cfg *config.Config, systemInstruction, locale string) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindInteractiveStory)
	if err != nil {
		return "", modelUnavailable(ctx, "world-creation", locale, err)
	}
	level := providers.ThinkingLevel(strings.TrimSpace(config.ResolveAgentModel(cfg, config.AgentKindInteractiveStory).ThinkingLevel))
	if level != "" {
		modelCfg.ThinkingLevel = level
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "assistant", cfg.Workspace,
		"world_creation", "World Creation",
		"draft a world's premise, places and people for a player who is about to create one",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(prompts.WorldCreationUserMessage),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindInteractiveStory, messages, nil); err != nil {
		return "", modelUnavailable(ctx, "world-creation", locale, err)
	}
	content, err := a.callWorldCreationModel(ctx, cfg, modelio.WithJSONObjectOutput(modelCfg), messages, locale)
	if err == nil {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[world-creation] json_mode failed, retry without response_format err=%v", err))
	return a.callWorldCreationModel(ctx, cfg, modelCfg, messages, locale)
}

func (a *App) callWorldCreationModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, messages []*agent.Message, locale string) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "world_creation", "draft", nil)
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", modelUnavailable(ctx, "world-creation", locale, err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "world_creation", "draft", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "world_creation", "draft", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate world draft: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the world creation model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "world_creation", "draft", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "world_creation", "draft", modelCfg.Model, 0, msg, nil, nil)
	return strings.TrimSpace(msg.Content), nil
}
