package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"strings"

	agent "ta/agent"
	"ta/agent/providers"
	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	appsettings "ta/internal/app/settings"
	"ta/internal/character"
	"ta/internal/i18n"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// Character creation drafts one character and writes nothing.
//
// 自制-开局建角与AI身份生成 splits the start into two acts: the player picks the
// options, the model drafts the prose the player left blank, and only the
// player's confirmation goes on to create a story. This file is the middle act.
// It deliberately does not create a Project, a Book or a story, and it does not
// touch the actor-state module: the draft is a proposal, and the same request
// can be made again (modified or rewritten) without leaving anything behind.
//
// The split between what the model writes and what the server derives is the
// point of the whole flow. The model writes prose. Every value a rule reads —
// realm, spirit stones, carried items, techniques, the aptitude token in
// 灵根与特殊体质 — is derived from the pack by storypack.ResolveIdentity. A draft
// that invents a treasure grants nothing, because the story start never reads
// the prose for numbers.

// StoryPackStartRequest is the start endpoint's body. TemplateID names a
// ready-made character; Identity names the choices of a character the player
// created, which the server resolves against the pack again. The two are
// alternatives: a request with an identity starts that character.
type StoryPackStartRequest struct {
	TemplateID string                    `json:"template_id"`
	Identity   *storypack.IdentityChoice `json:"identity,omitempty"`
}

// StoryPackIdentityDraftRequest is one draft request. Choice is exactly what the
// creation page holds, so a retry after a failure re-sends the same input.
type StoryPackIdentityDraftRequest struct {
	Choice storypack.IdentityChoice `json:"choice"`
	// Rewrite asks for the unlocked prose to be written again. It is the
	// "重写背景" action: the fixed choices and the age stay, and only fields the
	// player left empty are replaced.
	Rewrite bool `json:"rewrite,omitempty"`
	// Locale is the page's language, taken from the request header rather than
	// the body. A refusal the player reads is written in it.
	Locale string `json:"-"`
}

// StoryPackIdentityDraft is a drafted character plus what the page needs to
// render it.
type StoryPackIdentityDraft struct {
	// Identity carries both the player's fixed choices and the drafted prose,
	// already validated against the pack.
	Identity storypack.Identity `json:"identity"`
	// ContactCandidates are the people this world places near the start, which
	// the draft was allowed to use for acquaintances. They are an offer to the
	// player, not a fact about the character.
	ContactCandidates []string `json:"contact_candidates,omitempty"`
	// Rewritten says which action produced this draft, so the page can label it.
	Rewritten bool `json:"rewritten,omitempty"`
	// Error is a draft that did not finish: the model failed or wrote something
	// unreadable. Identity still carries the player's choices and the rolled age,
	// because losing the roll on a failed retry would reroll the character the
	// page already showed.
	Error string `json:"error,omitempty"`
}

// DraftStoryPackIdentity resolves the player's choices, rolls the age if the
// page has not locked one yet, and drafts or polishes the prose fields.
//
// The returned draft is also returned alongside a model error, because the
// rolled age and the validated choices are the player's input and must survive
// a failed draft: the page keeps what they typed and shows the failure.
func (s *StoryPackService) DraftStoryPackIdentity(ctx context.Context, packID string, request StoryPackIdentityDraftRequest) (StoryPackIdentityDraft, error) {
	pack, err := s.Get(packID)
	if err != nil {
		return StoryPackIdentityDraft{}, err
	}
	if !pack.CreationReady() {
		return StoryPackIdentityDraft{}, &storypack.IdentityError{Key: "api.storyPack.identity.notReady", Args: []any{"pack", pack.ID}}
	}
	choice := request.Choice
	choice.Locale = request.Locale
	if request.Rewrite {
		// A rewrite changes prose only. The age the player confirmed is what the
		// rewrite keeps; there is no path here that rolls a new one.
		if choice.RerollAge {
			return StoryPackIdentityDraft{}, &storypack.IdentityError{Key: "api.storyPack.draft.rewriteKeepsAge"}
		}
		if choice.Age <= 0 {
			return StoryPackIdentityDraft{}, &storypack.IdentityError{Key: "api.storyPack.draft.rewriteNeedsAge"}
		}
	}
	if choice.Age <= 0 || choice.RerollAge {
		// Only a page that sent no age gets one from the server; an age the
		// player typed or saw is validated below, never replaced.
		choice.Age = pack.RollAge(randomRoll())
	}

	// Resolve before the model call. A conflict the pack can name — an unknown
	// option, a sect in another country — is the player's to fix, and asking the
	// model to write around it would hide it.
	fixed, err := pack.ResolveIdentity(choice)
	if err != nil {
		return StoryPackIdentityDraft{}, err
	}

	runtimeCfg, err := s.app.storyPackRuntimeConfig()
	if err != nil {
		return StoryPackIdentityDraft{Identity: fixed}, err
	}

	// The words the page reads follow its language; the ids and the rule values
	// do not. Both copies are read-only: the pack that resolved the identity and
	// the contacts above is the one every number and every id comes from, so a
	// page shown in English still starts the character the Chinese page would.
	shownPack := pack.Localize(request.Locale)
	contacts := shownPack.CreationContactCandidates(fixed.AffiliationID, fixed.LocationCountry, 6)
	fixed, err = identityAuthoredPreview(fixed)
	if err != nil {
		return StoryPackIdentityDraft{}, err
	}
	shown := pack.LocalizeIdentity(fixed, request.Locale)
	instruction := prompts.BuildStoryCreationSystemInstruction(prompts.StoryCreationSystemInstructionInput{
		WorldName:         shownPack.Name,
		WorldBrief:        shownPack.Description,
		RealmLabel:        shownRealmLabel(shown, pack, fixed),
		Name:              fixed.Name,
		Gender:            shown.GenderName,
		Age:               fixed.Age,
		Origin:            strings.TrimSpace(shown.OriginText + "\n" + creationAdventureDraftFacts(shown, request.Locale)),
		SelectedRole:      shown.OriginName,
		Affiliation:       shown.AffiliationName,
		Location:          shown.LocationName,
		InferLocation:     strings.TrimSpace(choice.LocationText) == "" && (choice.LocationID == "" || choice.LocationID == "auto"),
		Roots:             creationRootsLine(shown, request.Locale),
		Aptitude:          shown.AptitudeName,
		WeaponDirection:   shown.WeaponName,
		Traits:            strings.Join(shown.TraitNames, listSeparator(request.Locale)),
		ContactCandidates: contactCandidateLines(contacts, request.Locale),
		OutputLanguage:    request.Locale,
		Existing: prompts.IdentityDraftInput{
			Identity: choice.Identity, Appearance: fixed.Appearance, Background: fixed.Background,
			Situation: fixed.Situation, Predicament: fixed.Predicament, Contacts: fixed.Contacts, Opening: fixed.Opening,
		},
	})

	instruction += creationAuthoredInstructions(pack, choice, fixed)
	content, err := s.app.generateStoryCreationDraft(ctx, &runtimeCfg, instruction, request.Locale)
	if err != nil {
		return StoryPackIdentityDraft{Identity: shown, ContactCandidates: contactCandidateLines(contacts, request.Locale)}, characterDraftError(ctx, err)
	}
	drafted, err := parseIdentityDraftPayload(content)
	if err != nil {
		return StoryPackIdentityDraft{Identity: shown, ContactCandidates: contactCandidateLines(contacts, request.Locale)}, characterDraftError(ctx, err)
	}

	// The player confirms the polished prose separately from the fixed choices.
	if strings.TrimSpace(choice.LocationText) == "" && (choice.LocationID == "" || choice.LocationID == "auto") {
		choice.LocationText = strings.TrimSpace(drafted.Location)
	}
	choice.Identity = firstIdentityText(drafted.Identity, choice.Identity)
	choice.Appearance = firstIdentityText(drafted.Appearance, choice.Appearance)
	choice.Background = firstIdentityText(drafted.Background, choice.Background)
	choice.Situation = firstIdentityText(drafted.Situation, choice.Situation)
	choice.Predicament = firstIdentityText(drafted.Predicament, choice.Predicament)
	choice.Opening = firstIdentityText(drafted.Opening, choice.Opening)
	if len(choice.Contacts) == 0 {
		choice.Contacts = drafted.Contacts
	}
	choice.Age = fixed.Age

	identity, err := pack.ResolveIdentity(choice)
	if err != nil {
		return StoryPackIdentityDraft{Identity: shown, ContactCandidates: contactCandidateLines(contacts, request.Locale)}, err
	}
	identity, err = identityAuthoredPreview(identity)
	if err != nil {
		return StoryPackIdentityDraft{}, err
	}
	shown = pack.LocalizeIdentity(identity, request.Locale)
	slog.InfoContext(ctx, "[story-pack] drafted a character",
		"pack_id", pack.ID, "origin", identity.OriginID, "age", identity.Age, "rewrite", request.Rewrite)
	return StoryPackIdentityDraft{
		Identity:          shown,
		ContactCandidates: contactCandidateLines(contacts, request.Locale),
		Rewritten:         request.Rewrite,
	}, nil
}

// characterDraftError keeps actionable, localized failures at the product boundary.
func characterDraftError(ctx context.Context, err error) error {
	slog.WarnContext(ctx, "[story-creation] character draft failed", "error", err)
	var identityError *storypack.IdentityError
	if errors.As(err, &identityError) || errors.Is(err, ErrNoDraftingModel) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return &storypack.IdentityError{Key: "api.storyPack.draft.cancelled"}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return &storypack.IdentityError{Key: "api.storyPack.draft.connectionFailed"}
	}
	return &storypack.IdentityError{Key: "api.storyPack.draft.failed"}
}

// shownRealmLabel is the starting realm as the page's language writes it. The
// label a story starts at is the pack's own word; only the copy shown changes.
func shownRealmLabel(shown storypack.Identity, pack storypack.Pack, fixed storypack.Identity) string {
	if shown.InitialState != nil && strings.TrimSpace(shown.InitialState.RealmLabel) != "" {
		return shown.InitialState.RealmLabel
	}
	return creationRealmLabel(pack, fixed.OriginID, fixed.InitialState)
}

// storyPackRuntimeConfig is the model configuration a creation draft runs on.
//
// It is not a story's configuration: no story exists yet. When a workspace is
// bound its project settings are layered on top, because that is the
// configuration the player sees on every other model surface; with no workspace
// the app-level configuration is what is left, and it is used as-is.
func (a *App) storyPackRuntimeConfig() (config.Config, error) {
	a.mu.RLock()
	cfg := a.cfg
	workspace := strings.TrimSpace(a.workspace)
	stateRoot := ""
	if cfg != nil {
		stateRoot = cfg.ProjectStoreDir
	}
	var snapshot config.Config
	if cfg != nil {
		snapshot = *cfg
	}
	a.mu.RUnlock()
	if cfg == nil {
		return config.Config{}, ErrNoDataRoot
	}
	if workspace == "" {
		return snapshot, nil
	}
	fresh, err := appsettings.RefreshProject(snapshot, workspace, stateRoot)
	if err != nil {
		return config.Config{}, err
	}
	return fresh, nil
}

// ErrNoDraftingModel is what a creation draft reports when this machine has no
// model to write with.
//
// A creation draft is the first place a player can reach the model, so it is
// where "nothing is set up yet" surfaces. Provider resolution names a protocol
// and a missing base URL; that is a diagnostic, not something a player can act
// on. The detail still reaches the log through the wrapped cause, and the
// player gets the one instruction that fixes it.
//
// The sentinel carries the identity, not the wording: the same refusal is shown
// in whatever language the player is using, so the message is built from the
// i18n catalog when the error is raised.
var ErrNoDraftingModel = errors.New("no drafting model")

// draftingModelMissing is that refusal in the player's own words. It matches
// ErrNoDraftingModel under errors.Is so callers keep one check.
type draftingModelMissing struct {
	message string
}

func (e draftingModelMissing) Error() string { return e.message }

func (e draftingModelMissing) Is(target error) bool { return target == ErrNoDraftingModel }

// modelUnavailable turns a provider-resolution failure into the one refusal a
// player can act on, keeping the technical cause in the log.
//
// An input that is too large is not this: the model exists and the draft is
// simply too long, which is a different answer for the player.
func modelUnavailable(ctx context.Context, area, locale string, err error) error {
	var limit *modelio.ProviderInputLimitError
	if errors.As(err, &limit) {
		return err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[%s] no model to draft with err=%v", area, err))
	return draftingModelMissing{message: i18n.New(locale).T("api.storyPack.draftingModelMissing")}
}

// generateStoryCreationDraft performs the one-shot model call. It mirrors the
// assistant call: standalone trace, one system instruction, one user message,
// no tools, no turn, no write.
func (a *App) generateStoryCreationDraft(ctx context.Context, cfg *config.Config, systemInstruction, locale string) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindInteractiveStory)
	if err != nil {
		return "", modelUnavailable(ctx, "story-creation", locale, err)
	}
	level := providers.ThinkingLevel(strings.TrimSpace(config.ResolveAgentModel(cfg, config.AgentKindInteractiveStory).ThinkingLevel))
	if level != "" {
		modelCfg.ThinkingLevel = level
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "assistant", cfg.Workspace,
		"story_creation", "Character Creation",
		"draft a character's background for a player who has already chosen their options",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(prompts.StoryCreationUserMessage),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindInteractiveStory, messages, nil); err != nil {
		return "", modelUnavailable(ctx, "story-creation", locale, err)
	}
	content, err := a.callStoryCreationModel(ctx, cfg, modelio.WithJSONObjectOutput(modelCfg), messages, locale)
	if err == nil {
		return content, nil
	}
	var networkError net.Error
	if ctx.Err() != nil || errors.As(err, &networkError) {
		return "", err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[story-creation] json_mode failed, retry without response_format err=%v", err))
	return a.callStoryCreationModel(ctx, cfg, modelCfg, messages, locale)
}

func (a *App) callStoryCreationModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, messages []*agent.Message, locale string) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "story_creation", "draft", nil)
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", modelUnavailable(ctx, "story-creation", locale, err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "story_creation", "draft", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_creation", "draft", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate character draft: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the character creation model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_creation", "draft", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "story_creation", "draft", modelCfg.Model, 0, msg, nil, nil)
	return strings.TrimSpace(msg.Content), nil
}

// identityDraftPayload is the draft's structured shape.
type identityDraftPayload struct {
	Location    string   `json:"location"`
	Identity    string   `json:"identity"`
	Appearance  string   `json:"appearance"`
	Background  string   `json:"background"`
	Situation   string   `json:"situation"`
	Predicament string   `json:"predicament"`
	Contacts    []string `json:"contacts"`
	Opening     string   `json:"opening"`
}

// parseIdentityDraftPayload reads one draft. A model that wrapped the object in
// a fence is read as if it had not; a response that is not an object at all is
// a failure rather than silently empty prose, because an empty draft would let
// the page show a character with nothing in it.
func parseIdentityDraftPayload(content string) (identityDraftPayload, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return identityDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.draft.unreadable"}
	}
	var payload identityDraftPayload
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return identityDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.draft.unparsable", Args: []any{"detail", err.Error()}}
	}
	if strings.TrimSpace(payload.Identity) == "" && strings.TrimSpace(payload.Background) == "" &&
		strings.TrimSpace(payload.Situation) == "" && strings.TrimSpace(payload.Opening) == "" {
		return identityDraftPayload{}, &storypack.IdentityError{Key: "api.storyPack.draft.empty"}
	}
	return payload, nil
}

func firstIdentityText(playerText, drafted string) string {
	if strings.TrimSpace(playerText) != "" {
		return playerText
	}
	return drafted
}

// withConfirmedIdentity folds a confirmed creation-page character into a pack's
// story request.
//
// It is the whole difference between a templated start and a created one: the
// protagonist snapshot and the opening premise come from the resolved identity,
// and the initial state still comes from the template the identity resolved
// against. The client's identity choices are ids and text; every number the
// story starts with is read again from the pack here.
func withConfirmedIdentity(pack storypack.Pack, req interactive.CreateStoryRequest, choice storypack.IdentityChoice) (interactive.CreateStoryRequest, string, error) {
	resolved, err := pack.ResolveIdentity(choice)
	if err != nil {
		return req, "", err
	}
	// A confirmed identity is a custom protagonist: the story must not go looking
	// for a lore character to play instead.
	req.Protagonist = interactive.StoryProtagonist{
		Mode:    interactive.StoryProtagonistModeCustom,
		Name:    resolved.Name,
		Profile: resolved.Profile,
	}
	req.Origin = resolved.Premise
	if pack.Creation != nil && choice.OriginID == "custom" && strings.TrimSpace(pack.Creation.CustomBackgroundRule) != "" {
		req.NarrativeRequirements = strings.TrimSpace(req.NarrativeRequirements + "\n\n" + pack.Creation.CustomBackgroundRule)
	}
	// The opening is AI-written from the confirmed premise. The player's own
	// drafted opening, when there is one, is part of that premise; replaying a
	// fixed preset here would start the story as someone else.
	req.Opening = interactive.StoryOpeningConfig{Mode: interactive.StoryOpeningModeAI}
	// The player may confirm an opening of their own instead of asking the
	// narrator to draft one from the premise. Keep that choice in the existing
	// two-path opening configuration, never as a template default.
	if choice.OpeningMode == "custom" && strings.TrimSpace(resolved.Opening) != "" {
		req.Opening = interactive.StoryOpeningConfig{Mode: interactive.StoryOpeningModeCustom, CustomText: resolved.Opening}
	}
	overlay, err := identityStateOverlay(resolved)
	if err != nil {
		return req, "", err
	}
	if len(overlay) == 0 {
		// The template path keys on a non-nil overlay to know a created character
		// is being started, so an empty one would replay the template's opening
		// over this character.
		overlay = map[string]any{}
	}
	req.IdentityStateOverlay = overlay
	// The state basis must be the template the identity resolved against, not
	// whichever one the page happened to name first.
	req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
		PackID:     pack.ID,
		TemplateID: resolved.TemplateID,
	}
	// This is the only path that confirms a player-chosen protagonist, so it is
	// the only one allowed to say the story already knows who the player is.
	// The stage reads that flag instead of guessing from a non-empty name, which
	// a pack's own protagonist() fills in without asking anybody anything.
	req.IdentityConfirmed = true
	if resolved.AffiliationID != "" && resolved.AffiliationID != "none" {
		req.InitialFactionMembership = &character.FactionMembership{FactionID: resolved.AffiliationID, FactionName: resolved.AffiliationName, Status: "formal", Statement: "建角时确认的所属身份"}
	}
	return req, resolved.TemplateID, nil
}

// identityStateOverlay is the only part of a created character that reaches the
// actor's state: the persona fields the player's confirmed draft rewrote.
//
// The list is closed on purpose. 当前境界, 灵石, 已学功法, 主法器, 护具, 布阵, 攻击法宝,
// 防御法宝, 辅助法宝, 重要物品 and the rest of the template's state stay exactly as the
// template wrote them, so a created character is worth the same as the ready-made
// one and no draft — and no tampered request — can start somebody richer. A key
// outside this list is a bug in this file, not a client error: the overlay is
// assembled server-side.
func identityStateOverlay(identity storypack.Identity) (map[string]any, error) {
	overlay := make(map[string]any, 5)
	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			overlay[key] = value
		}
	}
	// Empty persona fields clear the optional preset's prose. Otherwise a
	// custom character with no appearance draft still wears its old sect uniform.
	if identity.AdventureState != nil {
		raw, err := json.Marshal(identity.AdventureState)
		if err != nil {
			return nil, err
		}
		var value map[string]any
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		overlay[interactive.AdventureStateField] = value
	}
	overlay["基本身份"] = firstIdentityText(identity.IdentityText, identity.OriginText)
	overlay["外貌与装扮"] = identity.Appearance
	overlay["性格与背景"] = identity.Background
	overlay["当前处境"] = firstIdentityText(identity.Situation, identity.Premise)
	if strings.TrimSpace(identity.Foundation) != "" {
		set("灵根与特殊体质", identity.Foundation)
	}
	// 天赋特性 is the one creation choice that changes a number, and the number
	// is the rules': the state carries the talent's name and the settlement
	// prices it from its own table. Writing it here rather than letting the
	// template seed it is what keeps it a fact about this character — an
	// unchosen talent writes nothing, which is a real answer rather than a
	// missing one, and an existing character is never granted one retroactively.
	set("天赋特性", identity.TalentName)
	if len(overlay) == 0 {
		return nil, nil
	}
	return overlay, nil
}
func randomRoll() int {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<31))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func creationRealmLabel(pack storypack.Pack, originID string, initial *storypack.CreationInitialState) string {
	if pack.Creation != nil {
		if origin, ok := pack.Creation.Origin(originID); ok && origin.InitialState != nil {
			if label := strings.TrimSpace(origin.InitialState.RealmLabel); label != "" {
				return label
			}
		}
	}
	if initial != nil {
		return strings.TrimSpace(initial.Realm)
	}
	return ""
}

// listSeparator is the mark a list of words is joined with. An English page
// reads a comma; a Chinese one reads the enumeration comma it expects.
func listSeparator(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return ", "
	}
	return "、"
}

func creationRootsLine(identity storypack.Identity, locale string) string {
	if identity.RootCategoryName == "" {
		return ""
	}
	open, close := "（", "）"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		open, close = " (", ")"
	}
	if len(identity.RootAttributes) == 0 {
		return identity.RootCategoryName
	}
	return identity.RootCategoryName + open + strings.Join(identity.RootAttributes, listSeparator(locale)) + close
}

func contactCandidateLines(characters []storypack.CatalogCharacter, locale string) []string {
	lines := make([]string, 0, len(characters))
	for _, character := range characters {
		if line := strings.TrimSpace(storypack.ContactCandidateLine(character, locale)); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// A prose draft receives the same real starting abilities the preview displays.
// It does not get to invent a different kit from a broad class name.
func creationAdventureDraftFacts(identity storypack.Identity, locale string) string {
	if identity.InitialState == nil || identity.InitialState.Family == "" {
		return identity.AdventureTrainingNames()
	}
	learned, equipped := "初始已学", "初始实装"
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		learned, equipped = "Initially learned", "Initially equipped"
	}
	return fmt.Sprintf("%s\n%s: %s\n%s: %s", identity.AdventureTrainingNames(), learned, strings.Join(identity.InitialState.Techniques, listSeparator(locale)), equipped, strings.Join(identity.InitialState.Equipped, listSeparator(locale)))
}
