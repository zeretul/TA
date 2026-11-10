package interactive

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	imagepreset "ta/internal/image/preset"
)

func defaultStoryTitle(stories []StorySummary) string {
	if len(stories) == 0 {
		return defaultFirstStoryTitle
	}
	next := len(stories) + 1
	for _, story := range stories {
		title := strings.TrimSpace(story.Title)
		if !strings.HasPrefix(title, "故事线") {
			continue
		}
		rawNumber := strings.TrimSpace(strings.TrimPrefix(title, "故事线"))
		if rawNumber == "" {
			continue
		}
		number, err := strconv.Atoi(rawNumber)
		if err == nil && number >= next {
			next = number + 1
		}
	}
	if next < 2 {
		next = 2
	}
	return fmt.Sprintf("故事线 %d", next)
}

func normalizeStoryReplyTargetChars(value int) int {
	if value <= 0 {
		return DefaultStoryReplyTargetChars
	}
	return value
}

func normalizeStoryChoiceCount(value int) int {
	if value == 0 {
		return DefaultStoryChoiceCount
	}
	return value
}

func validateStoryChoiceCount(value int) error {
	if value < MinStoryChoiceCount || value > MaxStoryChoiceCount {
		return fmt.Errorf("互动故事行动建议数量必须在 %d 到 %d 之间", MinStoryChoiceCount, MaxStoryChoiceCount)
	}
	return nil
}

func normalizeStorySummary(story StorySummary) StorySummary {
	story.TitleSource = normalizeStoryTitleSource(story.TitleSource)
	story.PlanningTemplateID = normalizedGamePlanningTemplateID(story.PlanningTemplateID)
	story.LegacyStoryDirectorID = ""
	story.PlanningMode = normalizeStoryPlanningMode(story.PlanningMode)
	story.ReplyTargetChars = normalizeStoryReplyTargetChars(story.ReplyTargetChars)
	story.ChoiceCount = normalizeStoryChoiceCount(story.ChoiceCount)
	story.Protagonist = normalizeStoryProtagonist(story.Protagonist)
	story.Opening = normalizeStoryOpeningConfig(story.Opening)
	story.ImageSettings = normalizeStoryImageSettings(story.ImageSettings)
	story.SpeechSettings = normalizeStorySpeechSettings(story.SpeechSettings)
	story.IntimacySettings = normalizeStoryIntimacySettings(story.IntimacySettings)
	story.CheckSettings = normalizeStoryCheckSettings(story.CheckSettings)
	story.ModuleRefs = cloneStoryDirectorModuleRefs(story.ModuleRefs)
	story.PackBinding = cloneStoryPackBinding(story.PackBinding)
	story.NarrativeRequirements = normalizeStoryNarrativeRequirements(story.NarrativeRequirements)
	story.LocalEntitySettings = cloneLocalEntitySettings(story.LocalEntitySettings)
	if story.StateSchemaPolicy == nil {
		story.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		story.StateSchemaPolicy = cloneStoryStateSchemaPolicy(story.StateSchemaPolicy)
	}
	return story
}

func normalizeStoryMeta(meta StoryMeta) StoryMeta {
	legacyFixedSchema := meta.StateSchemaPolicy == nil
	meta.TitleSource = normalizeStoryTitleSource(meta.TitleSource)
	meta.PlanningTemplateID = normalizedGamePlanningTemplateID(firstNonEmptyString(meta.PlanningTemplateID, meta.StoryDirectorID))
	meta.PlanningMode = normalizeStoryPlanningMode(meta.PlanningMode)
	meta.ReplyTargetChars = normalizeStoryReplyTargetChars(meta.ReplyTargetChars)
	meta.ChoiceCount = normalizeStoryChoiceCount(meta.ChoiceCount)
	meta.Protagonist = normalizeStoryProtagonist(meta.Protagonist)
	meta.Opening = normalizeStoryOpeningConfig(meta.Opening)
	meta.ImageSettings = normalizeStoryImageSettings(meta.ImageSettings)
	meta.SpeechSettings = normalizeStorySpeechSettings(meta.SpeechSettings)
	meta.IntimacySettings = normalizeStoryIntimacySettings(meta.IntimacySettings)
	meta.CheckSettings = normalizeStoryCheckSettings(meta.CheckSettings)
	meta.ActorStateSchema = normalizeActorStateSchemaSnapshot(meta.ActorStateSchema)
	meta.ModuleRefs = cloneStoryDirectorModuleRefs(meta.ModuleRefs)
	meta.FrozenModules = cloneStoryDirectorResolvedSnapshot(meta.FrozenModules)
	meta.PackBinding = cloneStoryPackBinding(meta.PackBinding)
	meta.PackCatalog = ClonePackCatalogSnapshot(meta.PackCatalog)
	meta.NarrativeRequirements = normalizeStoryNarrativeRequirements(meta.NarrativeRequirements)
	meta.LocalEntitySettings = cloneLocalEntitySettings(meta.LocalEntitySettings)
	if legacyFixedSchema {
		meta.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		meta.StateSchemaPolicy = cloneStoryStateSchemaPolicy(meta.StateSchemaPolicy)
	}
	normalizeFixedStoryStateSchemaInitialization(&meta)
	return meta
}

func storyContainsTurn(events []StoryEventRecord) bool {
	for _, event := range events {
		if event.Envelope.Type == StoryEventTypeTurn {
			return true
		}
	}
	return false
}

func cloneStoryDirectorModuleRefs(refs *StoryDirectorModuleRefs) *StoryDirectorModuleRefs {
	if refs == nil {
		return nil
	}
	cloned := NormalizeStoryDirectorModuleRefs(*refs)
	cloned.EventPackageIDs = append([]string(nil), cloned.EventPackageIDs...)
	return &cloned
}

// cloneStoryDirectorResolvedSnapshot deep-copies a frozen module graph so a
// caller cannot mutate a stored story's frozen modules through a shared slice.
func cloneStoryDirectorResolvedSnapshot(snapshot *StoryDirectorResolvedSnapshot) *StoryDirectorResolvedSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := normalizeStoryDirectorResolvedSnapshot(*snapshot)
	if snapshot.EventPackages != nil {
		cloned.EventPackages = make([]EventPackage, len(snapshot.EventPackages))
		for index, pkg := range snapshot.EventPackages {
			copied := pkg
			copied.Events = append([]EventCard(nil), pkg.Events...)
			cloned.EventPackages[index] = copied
		}
	}
	cloned.TRPGSystem.RuleTemplates = append([]RuleCheck(nil), snapshot.TRPGSystem.RuleTemplates...)
	if snapshot.ActorState.Templates != nil {
		cloned.ActorState.Templates = make([]ActorStateTemplate, len(snapshot.ActorState.Templates))
		for index, template := range snapshot.ActorState.Templates {
			copied := template
			copied.Fields = append([]ActorStateField(nil), template.Fields...)
			cloned.ActorState.Templates[index] = copied
		}
	}
	cloned.ActorState.InitialActors = append([]ActorStateInitialActor(nil), snapshot.ActorState.InitialActors...)
	cloned.Warnings = append([]StoryDirectorModuleWarning(nil), snapshot.Warnings...)
	return &cloned
}

func normalizedGamePlanningTemplateID(id string) string {
	if id = NormalizeGamePlanningTemplateID(id); id != "" {
		return id
	}
	return DefaultGamePlanningTemplateID
}

func normalizeStoryOpeningConfig(config StoryOpeningConfig) StoryOpeningConfig {
	mode := strings.TrimSpace(config.Mode)
	switch mode {
	case StoryOpeningModePreset, StoryOpeningModeCustom:
	default:
		mode = StoryOpeningModeAI
	}
	normalized := StoryOpeningConfig{
		Mode:       mode,
		PresetID:   strings.TrimSpace(config.PresetID),
		PresetText: truncateStoryOpeningText(config.PresetText),
		CustomText: truncateStoryOpeningText(config.CustomText),
	}
	if mode != StoryOpeningModePreset {
		normalized.PresetID = ""
		normalized.PresetText = ""
	}
	if mode != StoryOpeningModeCustom {
		normalized.CustomText = ""
	}
	return normalized
}

// validateNewStoryOpeningConfig refuses an opening written from a preset.
//
// A start has two shapes: the model writes the first scene from the confirmed
// premise, or the player writes it. A preset snapshot is neither — it is a
// scene that already existed in a separate library — so a new story may no
// longer ask for one. Stories created while that library existed keep their
// stored preset text and still open from it: this refuses the write, not the
// record, which is why normalization and validation both still recognise the
// mode.
func validateNewStoryOpeningConfig(config StoryOpeningConfig) error {
	if strings.TrimSpace(config.Mode) == StoryOpeningModePreset {
		return fmt.Errorf("故事开场白只支持 AI 生成或玩家自定义")
	}
	return nil
}

// validateStoryOpeningUpdate refuses an edit that would turn a story's opening
// into a preset one.
//
// A story created while the preset library existed carries preset text, and the
// panel that edits it may hand the stored value back unchanged; that is a read,
// not a new preset configuration, so it passes. Anything else — a preset on a
// story that never had one, or a different preset on a story that did — is the
// removed third path arriving through the update door.
func validateStoryOpeningUpdate(current, next StoryOpeningConfig) error {
	if strings.TrimSpace(next.Mode) != StoryOpeningModePreset {
		return nil
	}
	if current.Mode == StoryOpeningModePreset &&
		strings.TrimSpace(current.PresetID) == strings.TrimSpace(next.PresetID) &&
		truncateStoryOpeningText(current.PresetText) == truncateStoryOpeningText(next.PresetText) {
		return nil
	}
	return fmt.Errorf("故事开场白只支持 AI 生成或玩家自定义")
}

func truncateStoryOpeningText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxStoryOpeningTextRunes {
		return text
	}
	return string(runes[:maxStoryOpeningTextRunes])
}

func normalizeStoryImageSettings(settings StoryImageSettings) StoryImageSettings {
	rawMode := strings.TrimSpace(settings.Mode)
	mode := StoryImageModeManual
	interval := settings.IntervalTurns
	switch rawMode {
	case "every_turn":
		mode = StoryImageModeInterval
		interval = 1
	case StoryImageModeInterval:
		mode = StoryImageModeInterval
	default:
		mode = StoryImageModeManual
	}
	if interval <= 0 {
		interval = 3
	}
	if interval > 50 {
		interval = 50
	}
	return StoryImageSettings{
		Mode:          mode,
		IntervalTurns: interval,
		PresetID:      normalizeStoryImagePresetID(settings.PresetID),
	}
}

func normalizeStoryImagePresetID(id string) string {
	id = imagepreset.NormalizeID(id)
	if id == "" {
		return imagepreset.DefaultID
	}
	return id
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(b[:])
}
