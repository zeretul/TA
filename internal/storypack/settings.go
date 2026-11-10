package storypack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ta/internal/interactive"
	"ta/internal/interactive/teller"
	"ta/internal/presetlayout"
	"ta/internal/revisionfile"
	"ta/internal/style"
)

// PackSettings is a world's default story configuration: the values a new game
// started from this pack begins with when the player makes no explicit choice.
//
// It is a default, not a lock. Every field is copied into the story at creation,
// so the player can change their own game and a later edit to the pack never
// rewrites a story already under way. Precedence is: the player's explicit
// choice for this game, then this block, then the application's own fallback
// for fields left empty.
type PackSettings struct {
	// RuleSystemID is a player override; empty follows the pack recommendation.
	RuleSystemID string `json:"rule_system_id,omitempty"`
	// PlanningMode is "enabled" or "disabled". It is a string rather than a bool
	// on purpose: an omitted value must not silently mean "off", because the
	// application falls back to planning enabled and the old behaviour of
	// hard-coding it off is exactly what this replaces.
	PlanningMode string `json:"planning_mode,omitempty"`
	// PlanningTemplateID names an installed planning template.
	PlanningTemplateID string `json:"planning_template_id,omitempty"`
	// StylePurposes selects the style for the main narrative, dialogue and
	// combat. An empty Dialogue or Combat follows the main narrative.
	StylePurposes PackStylePurposes `json:"style_purposes"`
	// ReplyTargetChars is the length tendency for replies.
	ReplyTargetChars int `json:"reply_target_chars,omitempty"`
	// NarrativeRequirements is the world's default creative direction. It is
	// seeded into a new story and then belongs to that story, so editing it here
	// changes the next game and never one already under way.
	NarrativeRequirements string `json:"narrative_requirements,omitempty"`
	// WritingPreferences is presentation for every turn of a bound world.
	WritingPreferences string `json:"writing_preferences,omitempty"`
	// CheckSettings tunes checks and rule-state handling for this world.
	CheckSettings PackCheckSettings `json:"check_settings"`
}

// PackStylePurposes mirrors interactive.StoryStylePurposes in the pack format.
type PackStylePurposes struct {
	Main     string `json:"main,omitempty"`
	Dialogue string `json:"dialogue,omitempty"`
	Combat   string `json:"combat,omitempty"`
}

// PackCheckSettings mirrors interactive.StoryCheckSettings in the pack format.
type PackCheckSettings struct {
	DifficultyShift          int    `json:"difficulty_shift,omitempty"`
	RollModifier             int    `json:"roll_modifier,omitempty"`
	RuleStateConsumptionMode string `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       string `json:"rule_visibility_mode,omitempty"`
}

func (s PackSettings) normalize() PackSettings {
	return PackSettings{
		RuleSystemID:       strings.TrimSpace(s.RuleSystemID),
		PlanningMode:       strings.ToLower(strings.TrimSpace(s.PlanningMode)),
		PlanningTemplateID: strings.TrimSpace(s.PlanningTemplateID),
		StylePurposes: PackStylePurposes{
			Main:     strings.TrimSpace(s.StylePurposes.Main),
			Dialogue: strings.TrimSpace(s.StylePurposes.Dialogue),
			Combat:   strings.TrimSpace(s.StylePurposes.Combat),
		},
		ReplyTargetChars:      s.ReplyTargetChars,
		NarrativeRequirements: strings.ReplaceAll(strings.TrimSpace(s.NarrativeRequirements), "\r\n", "\n"),
		WritingPreferences:    strings.ReplaceAll(strings.TrimSpace(s.WritingPreferences), "\r\n", "\n"),
		CheckSettings: PackCheckSettings{
			DifficultyShift:          s.CheckSettings.DifficultyShift,
			RollModifier:             s.CheckSettings.RollModifier,
			RuleStateConsumptionMode: strings.TrimSpace(s.CheckSettings.RuleStateConsumptionMode),
			RuleVisibilityMode:       strings.TrimSpace(s.CheckSettings.RuleVisibilityMode),
		},
	}
}

// withFallbacks fills the values the application supplies when a pack omits
// them. The world page shows this resolved block, so the page and the story it
// starts cannot disagree about what the defaults are.
func (s PackSettings) withFallbacks(story PackStory) PackSettings {
	if s.PlanningMode == "" {
		s.PlanningMode = interactive.StoryPlanningModeEnabled
	}
	if s.PlanningTemplateID == "" {
		s.PlanningTemplateID = interactive.DefaultGamePlanningTemplateID
	}
	if s.StylePurposes.Main == "" {
		s.StylePurposes.Main = style.DefaultID
	}
	if s.ReplyTargetChars <= 0 {
		s.ReplyTargetChars = firstNonZero(story.ReplyTargetChars, DefaultReplyTargetChars)
	}
	if s.CheckSettings.RuleStateConsumptionMode == "" {
		s.CheckSettings.RuleStateConsumptionMode = interactive.DefaultRuleStateConsumptionMode
	}
	if s.CheckSettings.RuleVisibilityMode == "" {
		s.CheckSettings.RuleVisibilityMode = interactive.RuleVisibilityModeAuditOnly
	}
	return s
}

func (s PackSettings) validate() error {
	switch s.PlanningMode {
	case "", interactive.StoryPlanningModeEnabled, interactive.StoryPlanningModeDisabled:
	default:
		return fmt.Errorf("剧本包 planning_mode 无效: %q", s.PlanningMode)
	}
	if s.ReplyTargetChars < 0 {
		return fmt.Errorf("剧本包 reply_target_chars 不能为负数: %d", s.ReplyTargetChars)
	}
	if err := interactive.ValidateStoryNarrativeRequirements(s.NarrativeRequirements); err != nil {
		return fmt.Errorf("剧本包叙事要求无效: %w", err)
	}
	if err := interactive.ValidateStoryNarrativeRequirements(s.WritingPreferences); err != nil {
		return fmt.Errorf("invalid world writing preferences: %w", err)
	}
	return interactive.ValidateStoryCheckSettings(s.interactiveCheckSettings())
}

func (s PackSettings) interactiveCheckSettings() interactive.StoryCheckSettings {
	return interactive.StoryCheckSettings{
		DifficultyShift:          s.CheckSettings.DifficultyShift,
		RollModifier:             s.CheckSettings.RollModifier,
		RuleStateConsumptionMode: s.CheckSettings.RuleStateConsumptionMode,
		RuleVisibilityMode:       s.CheckSettings.RuleVisibilityMode,
	}
}

func (s PackSettings) interactiveStylePurposes() interactive.StoryStylePurposes {
	return interactive.StoryStylePurposes{
		Main:     s.StylePurposes.Main,
		Dialogue: s.StylePurposes.Dialogue,
		Combat:   s.StylePurposes.Combat,
	}
}

// validateReferences checks that the styles and planning template this block
// names are installed. Without it a pack could name a style the library no
// longer has and a new game would silently be told in a different voice than
// the page showed.
func (s PackSettings) validateReferences(dataRoot string) error {
	if strings.TrimSpace(dataRoot) == "" {
		return nil
	}
	if s.RuleSystemID != "" {
		if _, err := interactive.NewRuleSystemLibrary(dataRoot).Get(s.RuleSystemID); err != nil {
			return identityError("api.storyPack.settings.ruleUnavailable", "id", s.RuleSystemID)
		}
	}
	if s.PlanningTemplateID != "" {
		if _, err := interactive.NewGamePlanningTemplateLibrary(dataRoot).Get(s.PlanningTemplateID); err != nil {
			return fmt.Errorf("剧本包规划的模板不存在: %s: %w", s.PlanningTemplateID, err)
		}
	}
	styles := teller.NewLibrary(dataRoot)
	for _, id := range []string{s.StylePurposes.Main, s.StylePurposes.Dialogue, s.StylePurposes.Combat} {
		if id == "" {
			continue
		}
		if _, err := styles.Get(id); err != nil {
			return fmt.Errorf("剧本包选择的文风不存在: %s: %w", id, err)
		}
	}
	return nil
}

// EffectiveSettings is the configuration a new game actually starts with.
//
// The page shows this resolved block, so it is also the single place the
// author's creative direction is folded in: while the player has saved nothing,
// the author's text is the default; once they have saved, their block is
// authoritative and a cleared direction stays cleared.
//
// The world's recommended planning template seeds a new game before the
// application's generic default. It is a seed only: CreateStory copies it into
// the story, so the player's later choice belongs to that story and a world
// that revises its recommendation never moves a game already under way. An
// explicit planning_template_id — the author's in the pack or the player's in
// their saved defaults — is a real choice and wins over the recommendation.
func (p Pack) EffectiveSettings() PackSettings {
	settings := p.Settings.normalize()
	if settings.PlanningTemplateID == "" {
		settings.PlanningTemplateID = strings.TrimSpace(p.RecommendedPlanningTemplateID)
	}
	settings = settings.withFallbacks(p.Story)
	settings.NarrativeRequirements = p.narrativeRequirements(settings)
	return settings
}

// settingsOverlaySuffix names the player's own saved defaults for a pack. It is
// a separate file on purpose: a pack may be compiled into the binary, and the
// player's defaults must never overwrite the author's content.
const settingsOverlaySuffix = ".settings.json"

func settingsOverlayPath(dataRoot, packID string) string {
	return filepath.Join(presetlayout.StoryPacks(dataRoot), packID+settingsOverlaySuffix)
}

// settingsOverlay is what the overlay file holds. It is the same shape as the
// pack's own block, so an author default and a player default are read by one
// code path.
type settingsOverlay struct {
	Settings PackSettings `json:"settings"`
}

// applySettingsOverlay replaces the author's defaults with the player's saved
// ones, when the player has saved any.
//
// A whole-block replacement rather than a field-by-field merge is what "保存新局
// 默认" means: the page always shows and submits the complete block, so a field
// the player deliberately set back to the application default (a zero difficulty
// shift, say) must not be read as "unset" and resurrect the author's value.
//
// A missing overlay is the ordinary case and is not an error. An unreadable one
// is reported: silently falling back to the author's defaults would present a
// game the player did not configure.
func (l *Library) applySettingsOverlay(pack *Pack) error {
	if pack == nil || strings.TrimSpace(pack.ID) == "" || pack.loadError != "" {
		return nil
	}
	data, err := os.ReadFile(settingsOverlayPath(l.dataRoot, pack.ID))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取剧本包用户默认设置失败 pack=%s: %w", pack.ID, err)
	}
	var overlay settingsOverlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		return fmt.Errorf("解析剧本包用户默认设置失败 pack=%s: %w", pack.ID, err)
	}
	overlay.Settings = overlay.Settings.normalize()
	// Range-check here rather than at start: a hand-edited overlay must not turn
	// into a world that looks fine in the list and refuses every new game.
	if err := overlay.Settings.validate(); err != nil {
		return fmt.Errorf("剧本包用户默认设置无效 pack=%s: %w", pack.ID, err)
	}
	pack.Settings = overlay.Settings
	pack.settingsCustomized = true
	return nil
}

// WritingPreferences reads only the user overlay for the bound world.
func (l *Library) WritingPreferences(packID string) (string, error) {
	if err := validatePackID(packID); err != nil {
		return "", err
	}
	pack := Pack{ID: packID}
	if err := l.applySettingsOverlay(&pack); err != nil {
		return "", err
	}
	return pack.Settings.WritingPreferences, nil
}

// SettingsSaveResult is what a successful save returns: the resolved settings
// the next new game will start with, and the overlay revision the page must use
// as the base of its next save.
type SettingsSaveResult struct {
	Settings PackSettings `json:"settings"`
	Revision string       `json:"revision"`
}

// SaveSettings writes the player's defaults for a pack as a separate overlay.
//
// The author's pack file is never touched. The write is a compare-and-swap on
// the overlay's bytes, so two pages saving at once cannot silently lose one
// edit; a stale base revision is reported instead of overwriting.
func (l *Library) SaveSettings(ctx context.Context, packID string, settings PackSettings, expectedRevision string) (SettingsSaveResult, error) {
	pack, err := l.Get(packID)
	if err != nil {
		return SettingsSaveResult{}, err
	}
	settings = settings.normalize()
	if err := settings.validate(); err != nil {
		return SettingsSaveResult{}, err
	}
	resolved := settings.withFallbacks(pack.Story)
	if err := resolved.validateReferences(l.dataRoot); err != nil {
		return SettingsSaveResult{}, err
	}
	refs := pack.StoryRequest().ModuleRefs
	if settings.RuleSystemID != "" {
		refs.RuleSystemID = settings.RuleSystemID
	} else {
		refs.RuleSystemID = pack.Story.ModuleRefs.RuleSystemID
	}
	if err := interactive.ValidateStoryModuleCompatibility(l.dataRoot, *refs); err != nil {
		return SettingsSaveResult{}, identityError("api.storyPack.settings.ruleIncompatible")
	}
	// Store what the player actually configured, not the resolved block: the
	// fallbacks belong to the application and may change, and freezing today's
	// fallback into a user file would make a later improvement invisible.
	content, err := json.MarshalIndent(settingsOverlay{Settings: settings}, "", "  ")
	if err != nil {
		return SettingsSaveResult{}, fmt.Errorf("序列化剧本包默认设置失败: %w", err)
	}
	result, err := revisionfile.ReplaceIfRevision(ctx, settingsOverlayPath(l.dataRoot, pack.ID), expectedRevision, content, revisionfile.Options{
		FileMode:      0o644,
		DirectoryMode: 0o755,
	})
	if err != nil {
		return SettingsSaveResult{}, err
	}
	return SettingsSaveResult{Settings: resolved, Revision: result.Revision}, nil
}

// SettingsRevision is the overlay revision the page sends back as the base of
// its next save, and whether the player has saved defaults at all.
func (l *Library) SettingsRevision(packID string) (string, bool) {
	snapshot, err := revisionfile.Read(context.Background(), settingsOverlayPath(l.dataRoot, strings.TrimSpace(packID)))
	if err != nil || !snapshot.Exists {
		return revisionfile.MissingRevision, false
	}
	return snapshot.Revision, true
}

// IsSettingsRevisionConflict reports whether a save lost a compare-and-swap.
func IsSettingsRevisionConflict(err error) bool {
	return errors.Is(err, revisionfile.ErrRevisionConflict)
}
