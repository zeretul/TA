package imageapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"

	apptask "ta/internal/app/task"
	booklore "ta/internal/book/lore"
	"ta/internal/character"
	imageasset "ta/internal/image/asset"
	imagegen "ta/internal/image/generation"
	imagepreset "ta/internal/image/preset"
	"ta/internal/interactive"
	"ta/internal/media"
)

const (
	interactiveImageToolName     = "generate_interactive_image"
	interactiveImageSkill        = "interactive-image"
	interactiveImageSourceAuto   = "auto"
	interactiveImageSourceManual = "manual"
)

type InteractiveGenerateResult struct {
	Enabled       bool                          `json:"enabled"`
	Skipped       bool                          `json:"skipped,omitempty"`
	SkippedReason string                        `json:"skipped_reason,omitempty"`
	Image         *imageasset.InteractiveResult `json:"image,omitempty"`
}

func (service *Service) GenerateInteractiveImage(ctx context.Context, storyID string, req interactive.InteractiveImageGenerateRequest) (InteractiveGenerateResult, error) {
	req.CommandID = strings.TrimSpace(req.CommandID)
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return InteractiveGenerateResult{}, err
	}
	runtime, err := service.AcquireRuntime(ctx, "")
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	defer runtime.Release()
	store := runtime.Interactive
	if store == nil {
		return InteractiveGenerateResult{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	turn, turnIndex, err := interactiveImageTargetTurn(storyCtx.Snapshot.Turns, req.TurnID)
	if err != nil {
		return InteractiveGenerateResult{}, err
	}
	source := normalizeInteractiveImageSource(req.Source)
	eventID := interactiveImageEventID(req.CommandID)
	if image, settled, projectionErr := interactiveImageCommandProjection(turn.DisplayEvents, eventID); settled {
		if projectionErr != nil {
			return InteractiveGenerateResult{}, projectionErr
		}
		return InteractiveGenerateResult{Enabled: true, Image: image}, nil
	}
	should, reason := shouldGenerateInteractiveImage(storyCtx.Meta.ImageSettings, storyCtx.Snapshot.Turns, turnIndex, source, req.Force)
	if !should {
		return InteractiveGenerateResult{Enabled: storyCtx.Meta.ImageSettings.Mode != interactive.StoryImageModeManual, Skipped: true, SkippedReason: reason}, nil
	}
	if existing := interactiveImageDisplayEvent(turn.DisplayEvents); existing != nil && !req.Force {
		return InteractiveGenerateResult{Enabled: true, Skipped: true, SkippedReason: "already_exists"}, nil
	}

	preset := loadImagePreset(runtime.Config.DataDir(), storyCtx.Meta.ImageSettings.PresetID)
	cast := interactiveImageCast(runtime, storyCtx.Meta, store, storyID, storyCtx.Snapshot)
	sourceContext := interactiveImageSourceContext(storyCtx.Meta, storyCtx.Snapshot.Turns, turnIndex, cast)
	systemPrompt := interactiveImageSystemPrompt(preset)
	toolPrompt := preset.PromptForTargets(imagepreset.TargetToolRequest)
	result, err := service.generateWithAgentUsingHooks(runtime, AgentGenerateRequest{
		CommandID:     req.CommandID,
		Purpose:       "interactive_image",
		SourceContext: sourceContext,
		SystemPrompt:  systemPrompt,
		ToolPrompt:    toolPrompt,
		SkillName:     interactiveImageSkill,
		StoryID:       storyID,
		BranchID:      storyCtx.Snapshot.BranchID,
		TurnID:        turn.ID,
		AltText:       interactiveImageAltText(storyCtx.Meta.Title, turnIndex),
		// The tool runs inside the Agent, so the cast it may attach travels with
		// the run rather than in the model-visible schema: which pictures exist is
		// the product's decision, not something the model should name.
		ImageReferencePeople: interactiveImageReferencePeople(cast),
	}, imageAgentRunHooks{
		OnAccepted: func() error {
			return store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
				ID: eventID, Role: "tool_call", Content: interactiveImageToolName, Name: interactiveImageToolName,
				Status: "running", Args: interactiveImageEventArgs(req.CommandID, source, req.Force),
				ToolPresentation: interactiveImageToolPresentation(),
			})
		},
		OnInteractiveImage: func(image *imageasset.InteractiveResult) error {
			return appendInteractiveImageSuccess(store, storyID, storyCtx.Snapshot.BranchID, turn.ID, eventID, req.CommandID, source, req.Force, image)
		},
	})
	if err != nil {
		if errors.Is(err, apptask.ErrCommandConflict) {
			return InteractiveGenerateResult{}, err
		}
		resultErr := err
		if runtime.Context().Err() != nil {
			resultErr = runtime.Context().Err()
		}
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:               eventID,
			Role:             "tool_call",
			Content:          interactiveImageToolName,
			Name:             interactiveImageToolName,
			Status:           "error",
			Args:             interactiveImageEventArgs(req.CommandID, source, req.Force),
			Result:           interactiveImageErrorResult(resultErr),
			ToolPresentation: interactiveImageToolPresentation(),
		})
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, resultErr)
	}
	if result.InteractiveImage == nil {
		err := fmt.Errorf("image Agent returned no interactive image")
		_ = store.AppendTurnDisplayEvent(storyID, storyCtx.Snapshot.BranchID, turn.ID, interactive.DisplayEvent{
			ID:               eventID,
			Role:             "tool_call",
			Content:          interactiveImageToolName,
			Name:             interactiveImageToolName,
			Status:           "error",
			Args:             interactiveImageEventArgs(req.CommandID, source, req.Force),
			Result:           interactiveImageErrorResult(err),
			ToolPresentation: interactiveImageToolPresentation(),
		})
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	if err := runtime.Context().Err(); err != nil {
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	if err := appendInteractiveImageSuccess(store, storyID, storyCtx.Snapshot.BranchID, turn.ID, eventID, req.CommandID, source, req.Force, result.InteractiveImage); err != nil {
		return InteractiveGenerateResult{}, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	return InteractiveGenerateResult{Enabled: true, Image: result.InteractiveImage}, nil
}

func normalizeInteractiveImageSource(source string) string {
	switch strings.TrimSpace(source) {
	case interactiveImageSourceAuto:
		return interactiveImageSourceAuto
	default:
		return interactiveImageSourceManual
	}
}

func interactiveImageTargetTurn(turns []interactive.TurnEvent, turnID string) (interactive.TurnEvent, int, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		if len(turns) == 0 {
			return interactive.TurnEvent{}, -1, fmt.Errorf("interactive image target turn is required")
		}
		return turns[len(turns)-1], len(turns) - 1, nil
	}
	for i, turn := range turns {
		if turn.ID == turnID {
			return turn, i, nil
		}
	}
	return interactive.TurnEvent{}, -1, fmt.Errorf("interactive image target turn does not exist: %s", turnID)
}

func shouldGenerateInteractiveImage(settings interactive.StoryImageSettings, turns []interactive.TurnEvent, turnIndex int, source string, force bool) (bool, string) {
	if force {
		return true, ""
	}
	settings = normalizeStoryImageSettings(settings)
	if source != interactiveImageSourceAuto {
		return true, ""
	}
	switch settings.Mode {
	case interactive.StoryImageModeManual:
		return false, "manual_mode"
	case interactive.StoryImageModeInterval:
		if turnIndex < 0 || turnIndex >= len(turns) {
			return false, "turn_not_found"
		}
		if (turnIndex+1)%settings.IntervalTurns == 0 {
			return true, ""
		}
		return false, "interval"
	default:
		return false, "disabled"
	}
}

func normalizeStoryImageSettings(settings interactive.StoryImageSettings) interactive.StoryImageSettings {
	if settings.Mode == "every_turn" {
		settings.Mode = interactive.StoryImageModeInterval
		settings.IntervalTurns = 1
	}
	if settings.Mode == "" {
		settings.Mode = interactive.StoryImageModeManual
	}
	if settings.IntervalTurns <= 0 {
		settings.IntervalTurns = 3
	}
	if imagepreset.NormalizeID(settings.PresetID) == "" {
		settings.PresetID = imagepreset.DefaultID
	}
	return settings
}

func interactiveImageDisplayEvent(events []interactive.DisplayEvent) *interactive.DisplayEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if strings.TrimSpace(events[i].Name) == interactiveImageToolName || strings.TrimSpace(events[i].Content) == interactiveImageToolName {
			return &events[i]
		}
	}
	return nil
}

func interactiveImageDisplayEventByID(events []interactive.DisplayEvent, eventID string) *interactive.DisplayEvent {
	eventID = strings.TrimSpace(eventID)
	for i := len(events) - 1; i >= 0; i-- {
		if strings.TrimSpace(events[i].ID) == eventID && strings.TrimSpace(events[i].Role) == "tool_call" {
			return &events[i]
		}
	}
	return nil
}

func interactiveImageCommandProjection(events []interactive.DisplayEvent, eventID string) (*imageasset.InteractiveResult, bool, error) {
	event := interactiveImageDisplayEventByID(events, eventID)
	if event == nil {
		return nil, false, nil
	}
	switch strings.TrimSpace(event.Status) {
	case "success":
		var image imageasset.InteractiveResult
		if err := json.Unmarshal([]byte(event.Result), &image); err != nil {
			return nil, true, fmt.Errorf("decode interactive image display result: %w", err)
		}
		if strings.TrimSpace(image.ImagePath) == "" {
			return nil, true, fmt.Errorf("interactive image display result has no image path")
		}
		return &image, true, nil
	case "error":
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal([]byte(event.Result), &payload)
		return nil, true, errors.New(firstNonEmpty(strings.TrimSpace(payload.Error), "interactive image generation failed"))
	default:
		return nil, false, nil
	}
}

func appendInteractiveImageSuccess(
	store *interactive.Store,
	storyID, branchID, turnID, eventID, commandID, source string,
	force bool,
	image *imageasset.InteractiveResult,
) error {
	if image == nil {
		return fmt.Errorf("image Agent returned no interactive image")
	}
	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	return store.AppendTurnDisplayEvent(storyID, branchID, turnID, interactive.DisplayEvent{
		ID: eventID, Role: "tool_call", Content: interactiveImageToolName, Name: interactiveImageToolName,
		Status: "success", Args: interactiveImageEventArgs(commandID, source, force), Result: string(data),
		ToolPresentation: interactiveImageToolPresentation(),
	})
}

func interactiveImageToolPresentation() *agent.ToolPresentation {
	presentation := agent.UniformToolPresentation(agent.ToolPresentationInteractiveMedia)
	return &presentation
}

func interactiveImageEventID(commandID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(commandID)))
	return fmt.Sprintf("interactive-image-command-%x", digest[:16])
}

func interactiveImageEventArgs(commandID, source string, force bool) string {
	data, _ := json.Marshal(map[string]any{
		"command_id": strings.TrimSpace(commandID),
		"source":     source,
		"force":      force,
	})
	return string(data)
}

func interactiveImageErrorResult(err error) string {
	message := "interactive image generation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	data, _ := json.Marshal(map[string]string{
		"schema": "interactive_image_error.v1",
		"error":  message,
	})
	return string(data)
}

func loadImagePreset(novaDir, id string) imagepreset.Preset {
	presetID := imagepreset.NormalizeID(id)
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	if strings.TrimSpace(novaDir) == "" {
		return imagepreset.DefaultPreset()
	}
	preset, err := imagepreset.NewLibrary(novaDir).Get(presetID)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-image] load image preset failed id=%s err=%v; fallback=%s", presetID, err, imagepreset.DefaultID))
		return imagepreset.DefaultPreset()
	}
	return preset
}

func interactiveImageSystemPrompt(preset imagepreset.Preset) string {
	var sb strings.Builder
	sb.WriteString("This request is for an interactive-story image. Generate exactly one image from events already established in the interactive turns. Do not reveal future plot or rewrite the narrative prose.")
	if systemPrompt := strings.TrimSpace(preset.PromptForTargets(imagepreset.TargetAgentSystem)); systemPrompt != "" {
		sb.WriteString("\n\n## Image Preset\n\n")
		if strings.TrimSpace(preset.ID) != "" {
			fmt.Fprintf(&sb, "- ID: %s\n", limitInteractiveImageRunes(preset.ID, 120))
		}
		if strings.TrimSpace(preset.Name) != "" {
			fmt.Fprintf(&sb, "- Name: %s\n", limitInteractiveImageRunes(preset.Name, 120))
		}
		sb.WriteString("\n")
		sb.WriteString(limitInteractiveImageRunes(systemPrompt, imagepreset.MaxPromptChars))
	}
	return sb.String()
}

func interactiveImageSourceContext(meta interactive.StoryMeta, turns []interactive.TurnEvent, turnIndex int, cast []imagegen.ReferencePerson) string {
	var sb strings.Builder
	writeContextLine(&sb, "Story title", meta.Title)
	writeContextLine(&sb, "Story origin", meta.Origin)
	writeContextLine(&sb, "Narrative style", meta.StoryTellerID)
	if len(cast) > 0 {
		sb.WriteString("\n## People In This Scene\n\n")
		sb.WriteString("Each person below is in this scene and has a reference picture attached to the request. Keep the face, hair and build of each one, and do not merge two people into one figure.\n\n")
		for _, person := range cast {
			fmt.Fprintf(&sb, "### %s\n", person.Name)
			if person.Appearance != "" {
				fmt.Fprintf(&sb, "Appearance: %s\n", limitInteractiveImageRunes(person.Appearance, 600))
			}
			sb.WriteString("\n")
		}
	}
	start := turnIndex - 2
	if start < 0 {
		start = 0
	}
	if start < turnIndex {
		sb.WriteString("\n## Preceding Turns\n\n")
		for i := start; i < turnIndex; i++ {
			fmt.Fprintf(&sb, "### Turn %d\nUser: %s\nNarrative: %s\n\n", i+1, limitInteractiveImageRunes(turns[i].User, 600), limitInteractiveImageRunes(turns[i].Narrative, 1200))
		}
	}
	if turnIndex >= 0 && turnIndex < len(turns) {
		turn := turns[turnIndex]
		sb.WriteString("\n## Current Turn\n\n")
		fmt.Fprintf(&sb, "User: %s\n\nNarrative: %s\n", limitInteractiveImageRunes(turn.User, 800), limitInteractiveImageRunes(turn.Narrative, 2400))
	}
	return strings.TrimSpace(sb.String())
}

func writeContextLine(sb *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "- %s: %s\n", label, limitInteractiveImageRunes(value, 600))
}

func interactiveImageAltText(title string, turnIndex int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "互动图像"
	}
	if turnIndex >= 0 {
		return fmt.Sprintf("%s 第 %d 轮互动图像", title, turnIndex+1)
	}
	return title + " 互动图像"
}

func limitInteractiveImageRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// interactiveImageCastMax bounds how many faces one picture is asked to keep.
// Beyond three the model starts blending them, and the picture stops being of
// anyone in particular.
const interactiveImageCastMax = 3

// interactiveImageCast reads who stands in this turn's scene and what the story
// already knows about how each of them looks.
//
// It reads committed state rather than the prose of the turn, for the same
// reason the character window does: the picture has to match the face the player
// is looking at, and that face is whatever the last accepted turn settled.
func interactiveImageCast(runtime *Runtime, meta interactive.StoryMeta, store *interactive.Store, storyID string, snapshot interactive.Snapshot) []imagegen.ReferencePerson {
	if runtime == nil || store == nil {
		return nil
	}
	scene := interactive.StoryScene(snapshot)
	if len(scene.Present) == 0 {
		return nil
	}
	state, err := store.CharacterState(storyID, snapshot.BranchID)
	if err != nil {
		slog.WarnContext(runtime.Context(), fmt.Sprintf("[interactive-image] read character state failed story=%s branch=%s err=%v", storyID, snapshot.BranchID, err))
		return nil
	}
	profiles := make(map[string]character.Profile, len(state.Profiles)*2)
	for _, profile := range state.Profiles {
		if id := strings.TrimSpace(profile.ID); id != "" {
			profiles[id] = profile
		}
		if name := strings.TrimSpace(profile.Name); name != "" {
			if _, ok := profiles[name]; !ok {
				profiles[name] = profile
			}
		}
	}
	bodies := interactiveImageCastLore(runtime.Workspace, meta)
	pictures := newReferencePictures(runtime, meta, scene)

	out := make([]imagegen.ReferencePerson, 0, interactiveImageCastMax)
	seen := make(map[string]bool, interactiveImageCastMax)
	for _, name := range scene.Present {
		if len(out) >= interactiveImageCastMax {
			break
		}
		profile, ok := profiles[strings.TrimSpace(name)]
		if !ok {
			continue
		}
		key := profileKey(profile)
		if seen[key] {
			continue
		}
		person := imagegen.ReferencePerson{
			Name:      firstNonEmpty(profile.Name, profile.ID),
			SourceKey: strings.TrimSpace(profile.SourceKey),
			Picture: pictures.source(profile),
		}
		if body, ok := bodies[person.SourceKey]; ok {
			person.Appearance = authorAppearance(body)
		}
		if person.Name == "" || (person.Appearance == "" && person.Picture == "") {
			continue
		}
		seen[key] = true
		out = append(out, person)
	}
	return out
}

func profileKey(profile character.Profile) string {
	if id := strings.TrimSpace(profile.ID); id != "" {
		return id
	}
	return strings.TrimSpace(profile.Name)
}

// interactiveImageReferencePeople encodes the cast for the run. It is the only
// place the encoding is decided, so the run record and the tool agree.
func interactiveImageReferencePeople(cast []imagegen.ReferencePerson) string {
	return imagegen.EncodeReferencePeople(cast)
}

// interactiveImageCastLore maps an author source key to that person's world book
// body. The entry is addressed by the story's own revision-qualified ID inside
// the story's own scope, so a person of the same key from another world, or from
// a later revision of this one, cannot describe this cast.
func interactiveImageCastLore(workspace string, meta interactive.StoryMeta) map[string]string {
	catalog := meta.PackCatalog
	if strings.TrimSpace(workspace) == "" || catalog == nil || len(catalog.Characters) == 0 {
		return nil
	}
	items, err := booklore.NewScopedStore(workspace, meta.LoreScope()).List()
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-image] read world book failed err=%v", err))
		return nil
	}
	byID := make(map[string]booklore.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	contentHash := ""
	if meta.PackBinding != nil {
		contentHash = strings.TrimSpace(meta.PackBinding.ContentHash)
	}
	out := make(map[string]string, len(catalog.Characters))
	for _, person := range catalog.Characters {
		key := strings.TrimSpace(person.SourceKey)
		loreID := strings.TrimSpace(person.LoreID)
		if key == "" || loreID == "" {
			continue
		}
		item, ok := byID[interactive.PackEntryIDForRevision(loreID, contentHash)]
		if !ok || strings.TrimSpace(item.Content) == "" {
			continue
		}
		out[key] = item.Content
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// authorAppearance pulls the one section of an authored body that describes how
// the person looks. The bodies number their sections, and the second one carries
// the appearance in every variant this build ships, but its title is not
// identical across them — some call it 外貌与辨识, others 形象与性格 — so the
// title is matched on those words rather than on a fixed heading.
//
// Only that section is returned. The rest of the body is voice, history and what
// the person knows, which the writing model already reads; handing it to the
// image model would put plot into a picture.
func authorAppearance(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var section strings.Builder
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			if inSection {
				break
			}
			inSection = strings.Contains(trimmed, "外貌") || strings.Contains(trimmed, "形象")
			continue
		}
		if !inSection || trimmed == "" {
			continue
		}
		section.WriteString(trimmed)
		section.WriteString("\n")
	}
	return limitInteractiveImageRunes(section.String(), 400)
}

// referencePictures resolves the one picture a person is actually shown with, in
// the same order the character window uses: an avatar the story bound to the
// profile, then a picture imported beside the binary, then a picture the world
// book gives the entry, then the shipped default portrait. A person with none of
// those gets no reference rather than a borrowed face.
type referencePictures struct {
	world    string
	sceneKey string
	lore     map[string]string
	media    *media.Library
}

func newReferencePictures(runtime *Runtime, meta interactive.StoryMeta, scene character.Scene) referencePictures {
	world := ""
	if meta.PackBinding != nil {
		world = strings.TrimSpace(meta.PackBinding.PackID)
	}
	mediaDir := ""
	workspace := ""
	if runtime != nil {
		mediaDir = runtime.Config.MediaDir
		workspace = runtime.Workspace
	}
	return referencePictures{
		world:    world,
		sceneKey: strings.TrimSpace(scene.Location),
		lore:     interactiveImageCastLoreImages(workspace, meta),
		media:    media.NewLibrary(mediaDir),
	}
}

func (p referencePictures) source(profile character.Profile) string {
	if chosen := strings.TrimSpace(profile.Avatar); chosen != "" {
		return imagegen.ReferencePicture(imagegen.PictureInWorkspace, chosen)
	}
	if p.media != nil {
		if asset, ok := p.media.Picture(media.PictureQuery{
			World: p.world, SourceKey: strings.TrimSpace(profile.SourceKey),
			Scope: strings.TrimSpace(profile.StoryID), Actor: strings.TrimSpace(profile.ID),
			Purpose: media.PurposeAvatar, SceneKey: p.sceneKey,
		}); ok && strings.TrimSpace(asset.File) != "" {
			return imagegen.ReferencePicture(imagegen.PictureInMedia, asset.File)
		}
	}
	key := strings.TrimSpace(profile.SourceKey)
	if key == "" {
		return ""
	}
	if path := p.lore[key]; path != "" {
		return imagegen.ReferencePicture(imagegen.PictureInWorkspace, path)
	}
	return imagegen.ReferencePicture(imagegen.PictureShipped, key)
}

// interactiveImageCastLoreImages joins the frozen catalog to the world book and
// returns the author key to picture path map. It restates the character window's
// two isolation rules rather than sharing them, because that service belongs to
// the window's own projection: the entry is addressed by the story's
// revision-qualified ID, and it must live in the scope this binding names.
func interactiveImageCastLoreImages(workspace string, meta interactive.StoryMeta) map[string]string {
	binding := meta.PackBinding
	catalog := meta.PackCatalog
	if strings.TrimSpace(workspace) == "" || binding == nil || catalog == nil {
		return nil
	}
	items, err := booklore.NewScopedStore(workspace, meta.LoreScope()).List()
	if err != nil {
		return nil
	}
	scope := strings.TrimSpace(binding.Scope)
	byID := make(map[string]booklore.Item, len(items))
	for _, item := range items {
		if scope != "" && strings.TrimSpace(item.Scope) != scope {
			continue
		}
		byID[item.ID] = item
	}
	if len(byID) == 0 {
		return nil
	}
	out := make(map[string]string, len(catalog.Characters))
	for _, person := range catalog.Characters {
		key := strings.TrimSpace(person.SourceKey)
		loreID := strings.TrimSpace(person.LoreID)
		if key == "" || loreID == "" {
			continue
		}
		item, ok := byID[interactive.PackEntryIDForRevision(loreID, binding.ContentHash)]
		if !ok || item.Image == nil {
			continue
		}
		if path, ok := projectAssetPath(item.Image.ImagePath); ok {
			out[key] = path
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// projectAssetPath accepts only a project-relative asset path. An absolute path,
// a URL with a scheme or host, or a path that climbs out of the project is not a
// picture this run can read, so it is dropped rather than handed to a reader.
func projectAssetPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`) {
		return "", false
	}
	if strings.Contains(raw, ":") {
		return "", false
	}
	for _, segment := range strings.Split(strings.ReplaceAll(raw, `\`, "/"), "/") {
		if segment == ".." {
			return "", false
		}
	}
	return raw, true
}
