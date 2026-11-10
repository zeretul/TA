// Package storypack loads game-only script packs (剧本包): declarative bundles
// that bind a playable world's opening, protagonist, world book and story
// director module references into one loadable unit.
//
// A pack is pure input. Loading one never writes to a catalog and never creates
// a second world state; it resolves to the same CreateStoryRequest and lore
// items the original game surface already submits, so packs stay a packaging
// format over existing entry points rather than a parallel gameplay path.
package storypack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
)

const (
	// Version is the pack document version this build reads. A pack declaring a
	// newer version is rejected rather than partially understood.
	Version = 1

	fileExtension = ".json"
)

// Pack is one loadable script pack.
type Pack struct {
	Version     int      `json:"version"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	// ContentVersion is the author's own content revision. It is deliberately
	// separate from Version, which is the document format: bumping the format
	// does not mean the world changed, and rewriting the world does not mean the
	// reader changed. A story binds to the content revision it started from.
	ContentVersion int `json:"content_version,omitempty"`

	// Story holds the initial values for the story this pack starts.
	Story PackStory `json:"story"`

	// Settings is the world's default configuration for a new game. It is a
	// default rather than a lock: every value is copied into the story at
	// creation, so a player can change their own game and a later edit here
	// leaves an existing story on what it started with.
	Settings PackSettings `json:"settings,omitempty"`

	// Localizations carries the same world in other languages, keyed by locale.
	// It is display text only, so it stays outside ContentHash: translating a
	// name does not change a fact, and deriving a new lore scope for a reader
	// who switched language would duplicate the world book for nothing.
	//
	// It is never written to a pack file or sent to a page. A delivered world
	// ships its translations beside its own asset and they are mounted here at
	// load; writing them into the pack would put a second copy of the world's
	// prose inside every user file that is saved back, and a page only ever reads
	// the one language it resolved.
	Localizations map[string]Localization `json:"-"`

	// RecommendedPlanningTemplateID is the author's own recommendation for how
	// this world's stories should be planned. It sits here rather than in
	// Settings on purpose: Settings is the player's block and a save replaces it
	// whole, so a recommendation stored there would disappear the moment the
	// player tuned anything. The page still shows it after the player picks
	// something else, which is what "this is what the world suggests" means.
	//
	// It is authoring metadata and outside ContentHash, so declaring or changing
	// it never moves a running story to a new lore scope.
	RecommendedPlanningTemplateID string `json:"recommended_planning_template_id,omitempty"`

	// ProtagonistTemplates are ready-made characters a player can start as
	// instead of writing a sheet. They are optional: a pack without them still
	// starts through the pack's own protagonist and the free-form card.
	ProtagonistTemplates []ProtagonistTemplate `json:"protagonist_templates,omitempty"`

	// Creation is the pack's authored options for the creation page. It is
	// authoring metadata and deliberately outside ContentHash, so editing an
	// option never moves a running story to a new lore scope.
	Creation *PackCreation `json:"creation,omitempty"`

	// World is materialized into the workspace lore book when the pack starts a
	// story, so the world's facts are available to the model from turn one.
	World []WorldEntry `json:"world,omitempty"`

	// Characters is the pack's discoverable-people index. It is deliberately not
	// a second copy of the world book: each row locates one person and points at
	// the World entry holding their body. A story reads this to find the right
	// person for the current scene, and reads the body only for the one who
	// actually appears.
	Characters []CatalogCharacter `json:"characters,omitempty"`

	// TopicTerms maps a topic name to the words that should also resolve to it,
	// so "残阵" and "布阵" find the people tagged 阵法. It lives in the pack
	// because the vocabulary belongs to the world, not to the matcher.
	TopicTerms map[string][]string `json:"topic_terms,omitempty"`

	// Builtin marks packs seeded from the binary. They can be read and started
	// like any other pack; a same-ID file in the data root overrides them.
	Builtin bool `json:"-"`

	// loadError carries a read failure for a listed-but-unusable pack so the
	// list stays honest instead of silently dropping a broken file.
	loadError string

	// settingsCustomized records that the player has saved defaults for this
	// pack. Once they have, their block is authoritative even where it is
	// empty: a cleared direction must stay cleared instead of quietly
	// resurrecting the author's text.
	settingsCustomized bool
}

// PackStory is the story-shaped half of a pack.
type PackStory struct {
	Title            string   `json:"title"`
	Origin           string   `json:"origin"`
	Protagonist      string   `json:"protagonist,omitempty"`
	ProtagonistName  string   `json:"protagonist_name,omitempty"`
	ReplyTargetChars int      `json:"reply_target_chars,omitempty"`
	ChoiceCount      int      `json:"choice_count,omitempty"`
	OpeningText      string   `json:"opening_text,omitempty"`
	ModuleRefs       PackRefs `json:"module_refs"`
	// NarrativeRequirements is the world's default creative direction, seeded
	// into a story this pack starts. It is a default, not a lock: the value is
	// copied into the story at creation, so the player's own later edit changes
	// only that story and editing the pack never rewrites a story already under
	// way. A pack that omits it leaves the workspace default in effect.
	NarrativeRequirements string        `json:"narrative_requirements,omitempty"`
	Gameplay              WorldGameplay `json:"gameplay"`
}

// PackRefs mirrors the story director module references a pack may bind. Empty
// fields fall back to the same defaults the original setup panel uses, so a
// pack only names what it actually changes.
//
// Narrative style is deliberately absent here. A pack's default voice belongs
// in Settings, which a player may override for their own copy of the world; the
// refs block stays the author's module binding. A pack file that still carries
// the old "narrative_style_id" key loads and ignores it rather than failing, so
// existing packs keep working.
type PackRefs struct {
	EventPackageIDs []string `json:"event_package_ids,omitempty"`
	RuleSystemID    string   `json:"rule_system_id,omitempty"`
	ActorStateID    string   `json:"actor_state_id,omitempty"`
	ImagePresetID   string   `json:"image_preset_id,omitempty"`
}

// WorldEntry is one lore item a pack contributes to the workspace.
type WorldEntry struct {
	Audience         string   `json:"audience,omitempty"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	Importance       string   `json:"importance,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	LoadMode         string   `json:"load_mode,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content"`

	// SourceRef records which story entity this entry was collected from, as
	// "<story id>/<entity id>". An authored entry has none. It is how a second
	// collection of the same entity is recognised as the same entry rather than
	// appended again, and it is deliberately absent from the world book the
	// model reads: a story does not need to know where its author filed a fact.
	SourceRef string `json:"source_ref,omitempty"`
}

// ModuleRef is one director module a pack binds, resolved against the installed
// catalog so the world-pack page can show whether the pack is actually
// startable instead of only listing the IDs it names.
type ModuleRef struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Version  int    `json:"version,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// Summary is the list projection of a pack.
type Summary struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Title            string   `json:"title"`
	Protagonist      string   `json:"protagonist,omitempty"`
	WorldCount       int      `json:"world_count"`
	PublicWorldCount int      `json:"public_world_count"`
	CharacterCount   int      `json:"character_count"`
	Builtin          bool     `json:"builtin"`
	Invalid          bool     `json:"invalid,omitempty"`
	Error            string   `json:"error,omitempty"`

	// ContentVersion is the author's own revision. A story keeps reading the
	// revision it started from, so this is shown next to the pack rather than
	// implying the pack upgrades in place.
	ContentVersion int `json:"content_version,omitempty"`
	// Modules is what the pack binds, already resolved. A pack whose modules are
	// not installed is listed but cannot start, and says which module is missing.
	Modules []ModuleRef `json:"modules,omitempty"`
	// Missing is the count of bound modules the catalog does not have.
	Missing int `json:"missing,omitempty"`
	// ProtagonistTemplates are the ready-made characters this pack offers.
	ProtagonistTemplates []TemplateSummary `json:"protagonist_templates,omitempty"`
	// Creation is the pack's authored creation-page options. It is a projection
	// of the same block the draft endpoint resolves, so the page cannot offer a
	// choice the server does not know, and no option carries a value a rule
	// reads.
	Creation *PackCreation `json:"creation,omitempty"`
	// Settings is the resolved new-game configuration: the pack's or the
	// player's saved defaults, with the application's fallbacks filled in. The
	// world page edits this block and starts games from it.
	Settings PackSettings `json:"settings"`
	// SettingsRevision is the overlay revision the page sends back when saving,
	// and SettingsCustomized says whether the player has saved their own defaults.
	SettingsRevision   string `json:"settings_revision,omitempty"`
	SettingsCustomized bool   `json:"settings_customized,omitempty"`
	// RecommendedPlanningTemplateID is the author's recommendation, kept
	// separate from Settings so a player's own defaults cannot erase it. The
	// page shows it beside the player's choice rather than replacing it.
	RecommendedPlanningTemplateID string `json:"recommended_planning_template_id,omitempty"`
}

// Library reads packs from the configured data root, falling back to the packs
// compiled into this build.
type Library struct {
	dataRoot string
}

func NewLibrary(dataRoot string) *Library {
	return &Library{dataRoot: dataRoot}
}

func (l *Library) dir() string {
	return presetlayout.StoryPacks(l.dataRoot)
}

// List returns every readable pack, user packs first, then builtins that no
// user pack overrides.
func (l *Library) List() ([]Summary, error) {
	return l.ListLocale("")
}

// ListLocale is List with one language's display text applied. An empty locale
// returns the packs' own text, which is what every non-UI caller wants: the
// model reads the world the author wrote, not a translation of it.
func (l *Library) ListLocale(locale string) ([]Summary, error) {
	packs, err := l.loadAll()
	if err != nil {
		return nil, err
	}
	summaries := make([]Summary, 0, len(packs))
	for _, pack := range packs {
		if pack.ID == HistoricalSanguoPackID {
			continue
		}
		summaries = append(summaries, pack.DescribeLocale(l.dataRoot, locale))
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Builtin != summaries[j].Builtin {
			return !summaries[i].Builtin
		}
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

// Get returns one pack by ID.
func (l *Library) Get(id string) (Pack, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Pack{}, fmt.Errorf("剧本包 ID 不能为空")
	}
	if err := validatePackID(id); err != nil {
		return Pack{}, err
	}
	path := filepath.Join(l.dir(), id+fileExtension)
	if pack, err := l.read(path); err == nil {
		if err := l.applySettingsOverlay(&pack); err != nil {
			return Pack{}, err
		}
		return pack, nil
	} else if !os.IsNotExist(err) {
		return Pack{}, err
	}
	for _, builtin := range builtinPacks() {
		if builtin.ID != id {
			continue
		}
		if _, err := l.materializeGameplayModules(&builtin); err != nil {
			return Pack{}, err
		}
		if err := builtin.ValidateModules(l.dataRoot); err != nil {
			return Pack{}, fmt.Errorf("剧本包无效: %s: %w", id, err)
		}
		if err := l.applySettingsOverlay(&builtin); err != nil {
			return Pack{}, err
		}
		return builtin, nil
	}
	return Pack{}, fmt.Errorf("剧本包不存在: %s", id)
}

// GetContent returns one pack by ID without checking the director modules it
// binds.
//
// Get answers "can this pack be started", so a pack naming a module the user has
// not installed is an error there. A reader that only needs the pack's content
// is asking a different question: the catalog that gives a person their stable
// author key is in the file, and it does not depend on any module being
// installed. Failing that read would take every face out of the author's lore
// library because a module was uninstalled, which has nothing to do with who
// these people are.
func (l *Library) GetContent(id string) (Pack, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Pack{}, fmt.Errorf("剧本包 ID 不能为空")
	}
	if err := validatePackID(id); err != nil {
		return Pack{}, err
	}
	path := filepath.Join(l.dir(), id+fileExtension)
	if pack, err := readPackFile(path); err == nil {
		return pack, nil
	} else if !os.IsNotExist(err) {
		return Pack{}, err
	}
	for _, builtin := range builtinPacks() {
		if builtin.ID == id {
			return builtin, nil
		}
	}
	return Pack{}, fmt.Errorf("剧本包不存在: %s", id)
}

// read loads one pack file and checks that the modules it binds are installed.
// The module check is why this is a Library method rather than a bare function:
// it needs the data root the catalogs live under.
func (l *Library) read(path string) (Pack, error) {
	pack, err := readPackFile(path)
	if err != nil {
		return Pack{}, err
	}
	if _, err := l.materializeGameplayModules(&pack); err != nil {
		return Pack{}, fmt.Errorf("世界规则模块无法从包内容恢复 path=%s: %w", path, err)
	}
	if err := pack.ValidateModules(l.dataRoot); err != nil {
		return Pack{}, fmt.Errorf("剧本包无效 path=%s: %w", path, err)
	}
	return pack, nil
}

func (l *Library) loadAll() ([]Pack, error) {
	packs := make([]Pack, 0, len(builtinPacks()))
	seen := map[string]bool{}
	entries, err := os.ReadDir(l.dir())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), fileExtension) {
			continue
		}
		// A saved-defaults overlay sits beside the pack it belongs to and is not a
		// pack itself. Listing it would surface a phantom broken world.
		if strings.HasSuffix(strings.ToLower(entry.Name()), settingsOverlaySuffix) {
			continue
		}
		path := filepath.Join(l.dir(), entry.Name())
		pack, readErr := l.read(path)
		if readErr != nil {
			// A malformed user pack must stay visible so it can be fixed, rather
			// than silently disappearing from the list.
			id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			packs = append(packs, Pack{ID: id, Name: id, loadError: readErr.Error()})
			seen[id] = true
			continue
		}
		seen[pack.ID] = true
		packs = append(packs, pack)
	}
	for _, builtin := range builtinPacks() {
		if seen[builtin.ID] {
			continue
		}
		if _, err := l.materializeGameplayModules(&builtin); err != nil {
			builtin.loadError = err.Error()
			packs = append(packs, builtin)
			continue
		}
		if err := builtin.ValidateModules(l.dataRoot); err != nil {
			// A builtin whose modules cannot be installed is a broken data root,
			// not a broken pack. Surface it instead of offering a pack that would
			// start on the wrong modules.
			builtin.loadError = err.Error()
		}
		packs = append(packs, builtin)
	}
	// The player's saved defaults are applied after the list is assembled, so a
	// single unreadable overlay fails the list instead of silently presenting
	// the author's defaults as if the player had chosen them.
	for index := range packs {
		if err := l.applySettingsOverlay(&packs[index]); err != nil {
			return nil, err
		}
	}
	return packs, nil
}

// ContentHash fingerprints the pack content a story binds to. It covers the
// story half, the world book, and the discoverable-people index — the parts that
// shape an existing story's model context — and deliberately not the
// display-only fields: renaming a pack or editing its description does not
// invalidate a running story.
//
// The catalog is inside the fingerprint on purpose. A story binds to the
// revision it started from, so a later revision that adds, removes or rewrites
// people must not silently change who an already-running story can find.
//
// Settings are outside it. They are a preference for the next new game, and the
// story creation path already copies the effective values into the story's own
// meta and summary. Folding them in would make changing a default derive a new
// lore scope, which duplicates the world book for a world whose facts did not
// change, and would make the discovery path refuse the live catalog over a
// preference edit.
func (p Pack) ContentHash() string {
	return interactive.ContentHashOf(struct {
		Story      PackStory             `json:"story"`
		World      []WorldEntry          `json:"world"`
		Characters []CatalogCharacter    `json:"characters"`
		TopicTerms map[string][]string   `json:"topic_terms"`
		Templates  []ProtagonistTemplate `json:"protagonist_templates"`
	}{Story: p.Story, World: p.World, Characters: p.Characters, TopicTerms: p.TopicTerms, Templates: p.ProtagonistTemplates})
}

// Binding is the story-pack binding this pack contributes to a story started
// from it. The scope is derived from the pack ID and the content fingerprint, so
// every story of one pack revision shares a world book, a new revision of the
// same pack gets its own, and no two packs share one.
func (p Pack) Binding() *interactive.StoryPackBinding {
	binding := &interactive.StoryPackBinding{
		PackID:         p.ID,
		PackName:       p.Name,
		ContentVersion: p.ContentVersion,
		ContentHash:    p.ContentHash(),
		Scope:          p.WorldScope(),
	}
	gameplay := p.Story.Gameplay
	// Legacy cultivation packs predate the capability declaration. Authored
	// labels may opt into a full profile without manufacturing new mechanics.
	if !gameplay.Confirmed && len(gameplay.Presentation) > 0 && p.Story.ModuleRefs.ActorStateID == interactive.ActorStateXiuxianID {
		gameplay.StatusTemplate = WorldStatusCultivation
		gameplay.Growth = WorldGrowthCultivation
		gameplay.Combat = WorldCombatCultivation
	}
	if gameplay.Confirmed || (len(gameplay.Presentation) > 0 && gameplay.StatusTemplate == WorldStatusCultivation) {
		letterDeliverySeconds := int64(gameplay.LetterDeliveryDays) * 24 * 60 * 60
		binding.Gameplay = &interactive.StoryGameplayProfile{
			Presentation:   gameplay.Presentation.Clone(),
			StatusTemplate: gameplay.StatusTemplate,
			Growth:         gameplay.Growth, Combat: gameplay.Combat,
			Communication: gameplay.Communication, LetterDeliverySeconds: letterDeliverySeconds, Opening: gameplay.Opening,
		}
	}
	return binding
}

// WorldScope is the lore scope this pack's world book belongs to. It is the one
// address both the writer (WorldInputs) and the reader (the story binding) must
// agree on; deriving it in one place keeps a pack's book and the story that
// reads it from drifting apart.
func (p Pack) WorldScope() string {
	return interactive.PackScopeForRevision(p.ID, p.ContentHash())
}

// invalid carries a read failure on a listed pack.
func readPackFile(path string) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, err
	}
	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return Pack{}, fmt.Errorf("解析剧本包失败 path=%s: %w", path, err)
	}
	fileID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if pack.ID == "" {
		pack.ID = fileID
	} else if pack.ID != fileID {
		// The catalog lists a pack by its declared ID but reads it back by
		// filename, so a mismatch would produce a pack that is visible in the
		// list and impossible to select. Rejecting it here keeps the two
		// lookups consistent instead of leaving a dead entry.
		return Pack{}, fmt.Errorf("剧本包文件名与 id 不一致 path=%s: 文件名为 %s，包内 id 为 %s", path, fileID, pack.ID)
	}
	if err := pack.Validate(); err != nil {
		return Pack{}, fmt.Errorf("剧本包无效 path=%s: %w", path, err)
	}
	return pack, nil
}

// ValidateModules reports whether every director module this pack binds exists
// in the catalog at dataRoot.
//
// Without this a pack naming a rule system the user never installed would load
// cleanly and then play on the original default: the pack would appear in the
// list, start, and present a different game than its author shipped. Resolving
// the references at load time turns that into a reported failure while the pack
// is still just a file.
//
// A pack with no data root behind it cannot be checked and is not failed here;
// the builtin packs are validated against the same catalogs by tests instead.
func (p Pack) ValidateModules(dataRoot string) error {
	if strings.TrimSpace(dataRoot) == "" {
		return nil
	}
	if (p.Story.Gameplay.StatusTemplate == WorldStatusGeneralActions && p.Story.Gameplay.Confirmed) || p.Story.Gameplay.CultivationRules != nil || p.Story.Gameplay.WorldContent != nil || p.Story.Gameplay.AdventureRules != nil || len(p.Story.Gameplay.CraftMethods) > 0 {
		if err := ValidateWorldGameplay(p.Story.Gameplay); err != nil {
			return err
		}
		stateID, ruleID := gameplayModuleIDs(p.ID, p.Story.Gameplay)
		if p.Story.ModuleRefs.ActorStateID != stateID || p.Story.ModuleRefs.RuleSystemID != ruleID {
			return fmt.Errorf("adventure module binding differs from frozen rule catalog")
		}
		if _, err := NewLibrary(dataRoot).materializeGameplayModules(&p); err != nil {
			return err
		}
	}
	director := interactive.StoryDirector{ModuleRefs: interactive.StoryDirectorModuleRefs{
		EventPackageIDs: p.Story.ModuleRefs.EventPackageIDs,
		RuleSystemID:    p.Story.ModuleRefs.RuleSystemID,
		ActorStateID:    p.Story.ModuleRefs.ActorStateID,
		ImagePresetID:   p.Story.ModuleRefs.ImagePresetID,
	}}
	warnings := interactive.ResolveStoryDirectorModuleErrors(dataRoot, director)
	if len(warnings) == 0 {
		return nil
	}
	missing := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		missing = append(missing, fmt.Sprintf("%s=%s", warning.Module, warning.ID))
	}
	return fmt.Errorf("所需模块不存在: %s", strings.Join(missing, ", "))
}

// Validate reports whether a pack can start a story as written.
func (p Pack) Validate() error {
	if err := p.Story.Gameplay.Presentation.Validate(); err != nil {
		return &WorldGameplayValidationError{Key: "api.storyPack.invalidWorldPresentation", Field: err.Error()}
	}
	if err := validateDefinitionLocaleSeparation(p.Story.Gameplay); err != nil {
		return err
	}
	if p.Story.Gameplay.StatusTemplate == WorldStatusCultivation && p.Story.Gameplay.CultivationRules == nil {
		return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
	}
	if p.Story.Gameplay.StatusTemplate != "" {
		if err := ValidateWorldGameplay(p.Story.Gameplay); err != nil {
			return err
		}
	}
	if p.Version <= 0 || p.Version > Version {
		return fmt.Errorf("不支持的剧本包版本: %d", p.Version)
	}
	if p.Story.Gameplay.WorldContent != nil && p.Story.Gameplay.WorldContent.PackID != p.ID {
		return fmt.Errorf("world content belongs to another pack")
	}
	if err := validatePackID(p.ID); err != nil {
		return err
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("剧本包缺少 name")
	}
	if strings.TrimSpace(p.Story.Title) == "" {
		return fmt.Errorf("剧本包缺少 story.title")
	}
	if strings.TrimSpace(p.Story.Origin) == "" {
		return fmt.Errorf("剧本包缺少 story.origin")
	}
	if p.Story.Gameplay.AdventureRules != nil {
		for _, template := range p.ProtagonistTemplates {
			raw, err := json.Marshal(template.State[interactive.AdventureStateField])
			if err != nil {
				return err
			}
			var actor interactive.AdventureActor
			if err := json.Unmarshal(raw, &actor); err != nil {
				return err
			}
			if err := interactive.ValidateAdventureActor(*p.Story.Gameplay.AdventureRules, actor); err != nil {
				return fmt.Errorf("invalid adventure origin %s: %w", template.ID, err)
			}
		}
	}
	if err := p.validateProtagonistTemplates(); err != nil {
		return err
	}
	if err := p.validateCreation(); err != nil {
		return err
	}
	if err := p.Settings.normalize().validate(); err != nil {
		return err
	}
	// A recommendation naming a template that does not exist would show a
	// suggestion nobody can act on. Format is checked here; existence needs the
	// data root and is checked where the recommendation is resolved.
	if id := strings.TrimSpace(p.RecommendedPlanningTemplateID); id != "" {
		if id != interactive.NormalizeGamePlanningTemplateID(id) {
			return fmt.Errorf("剧本包推荐规划模板 ID 无效: %q", p.RecommendedPlanningTemplateID)
		}
	}
	seen := map[string]bool{}
	for _, entry := range p.World {
		if strings.TrimSpace(entry.ID) == "" {
			return fmt.Errorf("世界书条目缺少 id")
		}
		if seen[entry.ID] {
			return fmt.Errorf("世界书条目 id 重复: %s", entry.ID)
		}
		seen[entry.ID] = true
		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Errorf("世界书条目缺少 name: %s", entry.ID)
		}
	}
	// The catalog is checked against the world book rather than accepted on its
	// own. A row pointing at a missing entry is worse than a missing row: the
	// story would offer the person, fail to load their body, and let the model
	// write them from the one-line summary.
	worldIDs := map[string]bool{}
	for _, entry := range p.World {
		worldIDs[strings.TrimSpace(entry.ID)] = true
	}
	if content := p.Story.Gameplay.WorldContent; content != nil {
		for id := range content.LoreGates {
			if !worldIDs[id] {
				return identityError("api.storyPack.world.loreGateMissing", "id", id)
			}
		}
	}
	seenSources := map[string]bool{}
	for _, character := range p.Characters {
		if err := character.Validate(); err != nil {
			return err
		}
		character = character.Normalize()
		if seenSources[character.SourceKey] {
			return fmt.Errorf("人物目录 source_key 重复: %s", character.SourceKey)
		}
		seenSources[character.SourceKey] = true
		if character.LoreID != "" && !worldIDs[character.LoreID] {
			return fmt.Errorf("人物目录条目 %s 指向不存在的世界书条目: %s", character.SourceKey, character.LoreID)
		}
	}
	if rules := p.Story.Gameplay.AdventureRules; rules != nil {
		for _, npc := range rules.NPCs {
			if !seenSources[npc.CharacterSourceKey] {
				return fmt.Errorf("adventure NPC references unknown character: %s", npc.CharacterSourceKey)
			}
		}
		places := map[string]bool{}
		for _, entry := range p.World {
			if entry.Type == "location" {
				places[entry.ID] = true
				places[entry.Name] = true
			}
		}
		for _, market := range rules.Markets {
			if !places[market.Location] {
				return fmt.Errorf("adventure market references unknown location: %s", market.ID)
			}
		}
		for _, enemy := range rules.Opponents {
			// A presence-bound person may meet the player away from their authored
			// home. Keep a sourced home hint, never invent a local spawn for a distant name.
			namedHome := false
			if enemy.RequiresPresence {
				for _, person := range p.Characters {
					namedHome = namedHome || person.SourceKey == enemy.CharacterSourceKey && person.PlaceLabel == enemy.Location
				}
			}
			if !places[enemy.Location] && !namedHome {
				return fmt.Errorf("adventure opponent references unknown location: %s", enemy.ID)
			}
			if enemy.CharacterSourceKey != "" && !seenSources[enemy.CharacterSourceKey] {
				return fmt.Errorf("adventure opponent references unknown character: %s", enemy.ID)
			}
		}
	}
	return nil
}

func (p Pack) Summary() Summary {
	publicWorldCount := 0
	for _, entry := range p.World {
		if lore.NormalizeAudience(entry.Audience) != "gm" {
			publicWorldCount++
		}
	}
	return Summary{
		ID:                   p.ID,
		Name:                 p.Name,
		Description:          p.Description,
		Tags:                 p.Tags,
		Title:                p.Story.Title,
		Protagonist:          p.Story.ProtagonistName,
		WorldCount:           len(p.World),
		PublicWorldCount:     publicWorldCount,
		CharacterCount:       len(p.Characters),
		Builtin:              p.Builtin,
		Invalid:              p.loadError != "",
		Error:                p.loadError,
		ContentVersion:       p.ContentVersion,
		ProtagonistTemplates: p.templateSummaries(),
		Creation:             p.Creation,
		Settings:             p.EffectiveSettings(),

		RecommendedPlanningTemplateID: p.RecommendedPlanningTemplateID,
	}
}

// Describe returns the pack's list projection with its bound modules resolved
// against dataRoot. This is the world-pack page's row: the page is a directory
// of the worlds actually prepared on this machine, so what matters is the
// revision each one is and whether it can start here, not its internal file
// layout.
func (p Pack) Describe(dataRoot string) Summary {
	return p.DescribeLocale(dataRoot, "")
}

// DescribeLocale is Describe with one language's display text applied. A pack
// that ships no text for that language reads exactly as Describe returns it.
func (p Pack) DescribeLocale(dataRoot, locale string) Summary {
	summary := p.Summary()
	if summary.Invalid {
		return summary
	}
	summary.Modules = p.resolveModules(dataRoot)
	for _, module := range summary.Modules {
		if module.Missing {
			summary.Missing++
		}
	}
	if dataRoot != "" {
		summary.SettingsRevision, summary.SettingsCustomized = NewLibrary(dataRoot).SettingsRevision(p.ID)
	}
	return p.localizedSummary(summary, locale)
}

// resolveModules names and versions each bound module. A missing module is
// reported rather than dropped: the pack file is intact, this machine just
// cannot play it, and the two are different things for the reader to fix.
func (p Pack) resolveModules(dataRoot string) []ModuleRef {
	refs := p.Story.ModuleRefs
	modules := make([]ModuleRef, 0, 4)

	for _, id := range firstNonEmptySlice(refs.EventPackageIDs, []string{DefaultEventPackageID}) {
		module := ModuleRef{Kind: "event_package", ID: id}
		if dataRoot != "" {
			if found, err := interactive.NewEventPackageLibrary(dataRoot).Get(id); err == nil {
				module.Name, module.Version = found.Name, found.Version
			} else {
				module.Missing = true
			}
		}
		modules = append(modules, module)
	}
	module := ModuleRef{Kind: "rule_system", ID: firstNonEmpty(refs.RuleSystemID, DefaultRuleSystemID)}
	if dataRoot != "" {
		if found, err := interactive.NewRuleSystemLibrary(dataRoot).Get(module.ID); err == nil {
			module.Name, module.Version = found.Name, found.Version
		} else {
			module.Missing = true
		}
	}
	modules = append(modules, module)

	module = ModuleRef{Kind: "actor_state", ID: firstNonEmpty(refs.ActorStateID, DefaultActorStateID)}
	if dataRoot != "" {
		if found, err := interactive.NewActorStateLibrary(dataRoot).Get(module.ID); err == nil {
			module.Name, module.Version = found.Name, found.Version
		} else {
			module.Missing = true
		}
	}
	modules = append(modules, module)

	// The image preset is presentation only; it is listed so the page shows
	// everything the pack binds, but a missing one never blocks a start.
	module = ModuleRef{Kind: "image_preset", ID: firstNonEmpty(refs.ImagePresetID, DefaultImagePresetID)}
	modules = append(modules, module)
	return modules
}

func validatePackID(id string) error {
	if id == "" {
		return fmt.Errorf("剧本包 ID 不能为空")
	}
	if strings.ContainsAny(id, `/\:*?"<>|`) || strings.Contains(id, "..") {
		return fmt.Errorf("剧本包 ID 含非法字符: %s", id)
	}
	return nil
}
