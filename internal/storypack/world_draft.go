package storypack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/revisionfile"
)

// WorldDraft is the one editable document both creation entry points share.
//
// 自制-AI创建世界包与武侠规则复用 §2 asks the guided and automatic paths to
// keep a single draft: a player may start by answering questions, switch to
// typing one sentence, and come back without losing what was already agreed.
// The draft therefore holds only what a player may edit — names, prose, places,
// people and the offered choices — and never a number the rules read.
//
// Everything mechanical is built from this document by BuildPack: the module
// ids, the ready-made templates' initial state, the aptitude tiers and the
// starting purse are the program's, not the model's. That is §6's division of
// labour and the reason a generated world cannot start somebody richer than a
// built-in one.
type WorldDraft struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	// Title is the story's own title and Origin the premise the first scene
	// opens on. Both are required by a pack, so both are required of a draft.
	Title  string `json:"title"`
	Origin string `json:"origin"`

	ProtagonistName string `json:"protagonist_name,omitempty"`
	Protagonist     string `json:"protagonist,omitempty"`

	NarrativeRequirements string `json:"narrative_requirements,omitempty"`
	OpeningText           string `json:"opening_text,omitempty"`

	World      []DraftWorldEntry   `json:"world,omitempty"`
	Characters []DraftCharacter    `json:"characters,omitempty"`
	TopicTerms map[string][]string `json:"topic_terms,omitempty"`

	Origins      []DraftOrigin `json:"origins,omitempty"`
	Affiliations []DraftOption `json:"affiliations,omitempty"`
	Locations    []DraftOption `json:"locations,omitempty"`

	// AgeRange is the inclusive range the creation page rolls an age from.
	AgeRange [2]int `json:"age_range,omitempty"`

	// Notice is a localized message key about what this world can actually play.
	// It is set by the program, never by the model.
	Notice   string        `json:"notice,omitempty"`
	Gameplay WorldGameplay `json:"gameplay"`
}

// WorldGameplay is the small set of choices the supported generic-action and
// cultivation templates can actually run. Unsupported choices are rejected
// by BuildPack instead of being replaced with another rule family.
type WorldGameplay struct {
	// AuthoredStateFields declares persistent protagonist facts for rule-led
	// worlds. It uses the existing actor schema, not a world-specific engine.
	// Values come from the chosen origin or committed story, never this block.
	AuthoredStateFields []interactive.ActorStateField            `json:"authored_state_fields,omitempty"`
	CultivationRules    *cultivation.Rules                       `json:"cultivation_rules,omitempty"`
	WorldContent        *interactive.WorldContent                `json:"world_content,omitempty"`
	Presentation        interactive.WorldPresentation            `json:"presentation,omitempty"`
	DefinitionDetails   map[string]cultivation.DefinitionDetails `json:"definition_details,omitempty"`
	CraftMethods        []cultivation.CraftMethodDef             `json:"craft_methods,omitempty"`
	AdventureRules      *interactive.AdventureRules              `json:"adventure_rules,omitempty"`
	StatusTemplate      string                                   `json:"status_template"`
	NumericScale        string                                   `json:"numeric_scale"`
	Growth              string                                   `json:"growth"`
	Combat              string                                   `json:"combat"`
	Communication       string                                   `json:"communication"`
	LetterDeliveryDays  int                                      `json:"letter_delivery_days,omitempty"`
	Opening             string                                   `json:"opening"`
	StateFields         []WorldStateField                        `json:"state_fields,omitempty"`
	CheckBindings       []WorldCheckBinding                      `json:"check_bindings,omitempty"`
	Confirmed           bool                                     `json:"confirmed"`
}

// WorldStateField is a bounded numeric field carried by one confirmed world.
// The same definition builds the actor-state module and the character's
// initial values, so a label or range cannot drift away from the rules.
type WorldStateField struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Minimum     int    `json:"minimum"`
	Maximum     int    `json:"maximum"`
	Initial     int    `json:"initial"`
}

// WorldCheckBinding selects one of the registered general actions and
// gives it a bounded linear modifier from a field in the same world schema.
type WorldCheckBinding struct {
	Action  string  `json:"action"`
	Field   string  `json:"field"`
	Scale   float64 `json:"scale"`
	Offset  float64 `json:"offset"`
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
}

type WorldGameplayValidationError struct {
	Key   string
	Field string
}

func (err *WorldGameplayValidationError) Error() string {
	if err.Field != "" {
		return fmt.Sprintf("world gameplay validation failed: %s (%s)", err.Key, err.Field)
	}
	return "world gameplay validation failed: " + err.Key
}

const (
	WorldStatusWuxia                    = "wuxia-state-v1"
	WorldStatusSpace                    = "space-state-v1"
	WorldStatusFantasy                  = "fantasy-state-v1"
	WorldCommunicationRadio             = "radio"
	WorldStatusGeneralActions           = "world-actions-v1"
	WorldStatusCultivation              = "xiuxian-state"
	WorldScaleOneToTen                  = "1-10"
	WorldScaleCultivation               = "cultivation"
	WorldGrowthNone                     = "none"
	WorldGrowthCultivation              = "cultivation"
	WorldCombatNone                     = "none"
	WorldCombatCultivation              = "cultivation"
	WorldCommunicationLocal             = "local"
	WorldCommunicationLetter            = "letter"
	WorldCommunicationCultivationSignal = "cultivation-signal"
	MinLetterDeliveryDays               = 1
	MaxLetterDeliveryDays               = 30
	WorldOpeningCommission              = "commission"
	WorldOpeningScene                   = "scene"
	WorldOpeningDaily                   = "daily"
)

func RecommendedWorldGameplay() WorldGameplay {
	return WorldGameplay{
		StatusTemplate:     WorldStatusGeneralActions,
		NumericScale:       WorldScaleOneToTen,
		Growth:             WorldGrowthNone,
		Combat:             WorldCombatNone,
		Communication:      WorldCommunicationLocal,
		LetterDeliveryDays: 1,
		Opening:            WorldOpeningCommission,
		StateFields: []WorldStateField{
			{Name: "观察", Description: "留意环境、人和细节的能力。", Minimum: 1, Maximum: 10, Initial: 5},
			{Name: "判断", Description: "比较信息并作出判断的能力。", Minimum: 1, Maximum: 10, Initial: 5},
			{Name: "交际", Description: "与他人沟通、争取合作的能力。", Minimum: 1, Maximum: 10, Initial: 5},
			{Name: "身手", Description: "行动、追逐与脱身的能力。", Minimum: 1, Maximum: 10, Initial: 5},
		},
		CheckBindings: []WorldCheckBinding{
			{Action: "search", Field: "观察", Scale: 1, Offset: -5, Minimum: -4, Maximum: 5},
			{Action: "evidence_review", Field: "判断", Scale: 1, Offset: -5, Minimum: -4, Maximum: 5},
			{Action: "persuasion", Field: "交际", Scale: 1, Offset: -5, Minimum: -4, Maximum: 5},
			{Action: "chase_or_escape", Field: "身手", Scale: 1, Offset: -5, Minimum: -4, Maximum: 5},
		},
	}
}

func validateWorldGameplay(gameplay WorldGameplay) error {
	if len(gameplay.AuthoredStateFields) > 0 {
		if gameplay.StatusTemplate != WorldStatusGeneralActions {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		if err := validateAuthoredWorldStateFields(gameplay); err != nil {
			return err
		}
	}
	if err := gameplay.CultivationRules.Validate(); err != nil {
		return err
	}
	if gameplay.CultivationRules != nil && gameplay.StatusTemplate != WorldStatusCultivation {
		return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
	}
	if err := gameplay.WorldContent.Validate(); err != nil {
		return err
	}
	if err := gameplay.Presentation.Validate(); err != nil {
		return &WorldGameplayValidationError{Key: "api.storyPack.invalidWorldPresentation", Field: err.Error()}
	}
	if len(gameplay.CraftMethods) > 0 {
		if gameplay.StatusTemplate != WorldStatusCultivation {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		if err := cultivation.ValidateCraftMethods(gameplay.CraftMethods, cultivation.AuthoredCraftCatalog); err != nil {
			return fmt.Errorf("invalid craft methods: %w", err)
		}
	}
	if !gameplay.Confirmed {
		return &WorldGameplayValidationError{Key: "api.storyPack.gameplayNotConfirmed"}
	}
	if gameplay.StatusTemplate == WorldStatusWuxia || gameplay.StatusTemplate == WorldStatusSpace || gameplay.StatusTemplate == WorldStatusFantasy {
		if len(gameplay.StateFields) > 0 || len(gameplay.CheckBindings) > 0 {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		family := "wuxia"
		if gameplay.StatusTemplate == WorldStatusSpace {
			family = "space"
		} else if gameplay.StatusTemplate == WorldStatusFantasy {
			family = "fantasy"
		}
		if gameplay.NumericScale != family || gameplay.Growth != family || gameplay.Combat != family || gameplay.AdventureRules == nil || gameplay.AdventureRules.Family != family {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		if gameplay.Communication != WorldCommunicationLocal && !((family == "wuxia" || family == "fantasy") && gameplay.Communication == WorldCommunicationLetter) && !(family == "space" && gameplay.Communication == WorldCommunicationRadio) {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		if gameplay.Communication == WorldCommunicationLetter && (gameplay.LetterDeliveryDays < 1 || gameplay.LetterDeliveryDays > 30) {
			return &WorldGameplayValidationError{Key: "api.storyPack.invalidLetterDeliveryDays"}
		}
		switch gameplay.Opening {
		case WorldOpeningCommission, WorldOpeningScene, WorldOpeningDaily:
		default:
			return identityError("api.storyPack.world.openingMissing")
		}
		return gameplay.AdventureRules.Validate()
	}
	if gameplay.AdventureRules != nil {
		return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
	}
	if gameplay.StatusTemplate == WorldStatusCultivation && gameplay.NumericScale == WorldScaleCultivation &&
		gameplay.Growth == WorldGrowthCultivation && gameplay.Combat == WorldCombatCultivation {
		if gameplay.Communication != WorldCommunicationLocal && gameplay.Communication != WorldCommunicationCultivationSignal {
			return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
		}
		switch gameplay.Opening {
		case WorldOpeningCommission, WorldOpeningScene, WorldOpeningDaily:
		default:
			return identityError("api.storyPack.world.openingMissing")
		}
		return nil
	}
	if gameplay.StatusTemplate != WorldStatusGeneralActions || gameplay.NumericScale != WorldScaleOneToTen ||
		gameplay.Growth != WorldGrowthNone || gameplay.Combat != WorldCombatNone ||
		(gameplay.Communication != WorldCommunicationLocal && gameplay.Communication != WorldCommunicationLetter) {
		return &WorldGameplayValidationError{Key: "api.storyPack.unsupportedGameplay"}
	}
	if gameplay.Communication == WorldCommunicationLetter && (gameplay.LetterDeliveryDays < MinLetterDeliveryDays || gameplay.LetterDeliveryDays > MaxLetterDeliveryDays) {
		return &WorldGameplayValidationError{Key: "api.storyPack.invalidLetterDeliveryDays"}
	}
	switch gameplay.Opening {
	case WorldOpeningCommission, WorldOpeningScene, WorldOpeningDaily:
	default:
		return identityError("api.storyPack.world.openingMissing")
	}
	if len(gameplay.StateFields) == 0 || len(gameplay.StateFields) > 8 {
		return &WorldGameplayValidationError{Key: "api.storyPack.invalidStateFieldCount"}
	}
	fields := make(map[string]bool, len(gameplay.StateFields))
	reserved := map[string]bool{}
	baseState := interactive.DefaultActorStateModule().ActorState
	for _, template := range baseState.Templates {
		if template.ID != interactive.DefaultActorID {
			continue
		}
		for _, field := range template.Fields {
			reserved[worldFieldKey(field.Name)] = true
		}
	}
	for _, field := range gameplay.StateFields {
		name := strings.TrimSpace(field.Name)
		key := worldFieldKey(name)
		if name == "" || len([]rune(name)) > 16 || fields[key] || reserved[key] {
			return &WorldGameplayValidationError{Key: "api.storyPack.invalidStateField", Field: name}
		}
		if field.Minimum < 0 || field.Maximum > 20 || field.Maximum <= field.Minimum || field.Initial < field.Minimum || field.Initial > field.Maximum {
			return &WorldGameplayValidationError{Key: "api.storyPack.invalidStateField", Field: name}
		}
		fields[key] = true
	}
	allowedActions := map[string]bool{"search": true, "evidence_review": true, "persuasion": true, "chase_or_escape": true}
	seenActions := map[string]bool{}
	boundFields := map[string]bool{}
	for _, binding := range gameplay.CheckBindings {
		fieldKey := worldFieldKey(binding.Field)
		if !allowedActions[binding.Action] || seenActions[binding.Action] || !fields[fieldKey] {
			return &WorldGameplayValidationError{Key: "api.storyPack.invalidCheckBinding", Field: binding.Action}
		}
		if binding.Scale < 0 || binding.Scale > 2 || binding.Offset < -20 || binding.Offset > 20 || binding.Minimum < -4 || binding.Maximum > 5 || binding.Minimum > 0 || binding.Maximum < 0 || binding.Maximum < binding.Minimum {
			return &WorldGameplayValidationError{Key: "api.storyPack.invalidCheckBinding", Field: binding.Action}
		}
		seenActions[binding.Action] = true
		boundFields[fieldKey] = true
	}
	if len(gameplay.CheckBindings) == 0 || len(boundFields) != len(fields) {
		return &WorldGameplayValidationError{Key: "api.storyPack.invalidCheckBinding"}
	}
	return nil
}

func worldFieldKey(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

func ValidateWorldGameplay(gameplay WorldGameplay) error {
	return validateWorldGameplay(gameplay)
}

// DraftWorldEntry is one world-book entry.
type DraftWorldEntry struct {
	Audience         string   `json:"audience,omitempty"`
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	Importance       string   `json:"importance,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	LoadMode         string   `json:"load_mode,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content"`
}

// DraftCharacter is one person the story may bring on stage.
//
// Only the public half of a person is here. What they own, what they can do and
// who they are bound to are learned in play, never shipped in the pack.
type DraftCharacter struct {
	Name         string   `json:"name"`
	Age          int      `json:"age,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	SpeechStyle  string   `json:"speech_style,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Country      string   `json:"country,omitempty"`
	Faction      string   `json:"faction,omitempty"`
	Role         string   `json:"role,omitempty"`
	Topics       []string `json:"topics,omitempty"`
	Place        string   `json:"place,omitempty"`
	PlaceLabel   string   `json:"place_label,omitempty"`
	Brief        string   `json:"brief,omitempty"`
	InitialStage string   `json:"initial_stage,omitempty"`
	Level        string   `json:"level,omitempty"`
}

// DraftOrigin is one social origin the player may start from.
//
// Basis names which ready-made initial state it begins on. The three bases are
// the program's: a draft chooses among them and cannot invent a fourth, because
// the state behind a basis is a rule outcome and not a line of prose.
type DraftOrigin struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Basis is wanderer, disciple or scion.
	Basis string `json:"basis"`
	// Technique and Item name the one technique and the one thing this origin
	// starts with. The names are the world's; the definitions behind them are
	// the rules' and are supplied by BuildPack.
	Technique string `json:"technique,omitempty"`
	Item      string `json:"item,omitempty"`
}

// DraftOption is one labelled choice: an affiliation or a place.
type DraftOption struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Country     string `json:"country,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// The three ready-made starts a draft may offer. They are named by the program
// so a generated world always has a legal state basis, and a draft only chooses
// which ones it uses and what they are called in this world.
const (
	draftBasisWanderer = "wanderer"
	draftBasisDisciple = "disciple"
	draftBasisScion    = "scion"
)

// draftBasis is one program-owned starting position.
type draftBasis struct {
	ID          string
	Realm       string
	Currency    int
	DefaultName string
	DefaultTech string
	DefaultItem string
}

func draftBases() []draftBasis {
	return []draftBasis{
		{
			ID: draftBasisWanderer, Realm: "qi_6", Currency: 20,
			DefaultName: "散修", DefaultTech: "引气拳", DefaultItem: "疗伤丹",
		},
		{
			ID: draftBasisDisciple, Realm: "qi_6", Currency: 40,
			DefaultName: "门中弟子", DefaultTech: "引气拳", DefaultItem: "疗伤丹",
		},
		{
			ID: draftBasisScion, Realm: "qi_12", Currency: 150,
			DefaultName: "世家子弟", DefaultTech: "家传剑诀", DefaultItem: "疗伤丹",
		},
	}
}

func draftBasisByID(id string) (draftBasis, bool) {
	id = strings.TrimSpace(id)
	for _, basis := range draftBases() {
		if basis.ID == id {
			return basis, true
		}
	}
	return draftBasis{}, false
}

// IsDraftBasis reports whether a starting position is one the rules provide.
//
// A draft may only offer these three. Anything else would be a starting position
// with no state behind it, which is why an unknown basis is dropped rather than
// mapped onto a real one.
func IsDraftBasis(id string) bool {
	_, ok := draftBasisByID(id)
	return ok
}

// WorldPreview is what the creation page shows before anything is saved.
//
// It is built from the pack the draft would become, so the counts and the
// readiness are the ones a saved world would really have. §6 allows a thin
// draft to be saved and edited, so an unstartable draft still previews; it just
// says why it cannot start yet instead of offering a start that would fail.
type WorldPreview struct {
	Name           string   `json:"name,omitempty"`
	Title          string   `json:"title,omitempty"`
	Origin         string   `json:"origin,omitempty"`
	WorldCount     int      `json:"world_count"`
	CharacterCount int      `json:"character_count"`
	Origins        []string `json:"origins,omitempty"`
	Affiliations   []string `json:"affiliations,omitempty"`
	Locations      []string `json:"locations,omitempty"`
	Startable      bool     `json:"startable"`
	Reasons        []string `json:"reasons,omitempty"`
	ReasonKeys     []string `json:"reason_keys,omitempty"`
	Notice         string   `json:"notice"`
}

// PreviewWorldDraft builds the draft and reports whether it could start.
func PreviewWorldDraft(dataRoot string, draft WorldDraft) WorldPreview {
	preview := WorldPreview{Notice: DraftNoticeForGameplay(draft.Gameplay)}
	if name := strings.TrimSpace(draft.Name); name != "" {
		preview.Name = name
	}
	pack, err := draft.BuildPack()
	if err != nil {
		var gameplayError *WorldGameplayValidationError
		if errors.As(err, &gameplayError) {
			preview.ReasonKeys = []string{gameplayError.Key}
		} else {
			preview.Reasons = []string{err.Error()}
		}
		return preview
	}
	preview.Name = pack.Name
	preview.Title = pack.Story.Title
	preview.Origin = pack.Story.Origin
	preview.WorldCount = len(pack.World)
	preview.CharacterCount = len(pack.Characters)
	for _, origin := range pack.Creation.Origins {
		preview.Origins = append(preview.Origins, origin.Name)
	}
	for _, option := range pack.Creation.Affiliations {
		preview.Affiliations = append(preview.Affiliations, option.Name)
	}
	for _, option := range pack.Creation.Locations {
		preview.Locations = append(preview.Locations, option.Name)
	}

	// The checks a save performs, in the same order. Running them here means the
	// page says "cannot start yet" for the same reason a save would refuse, and
	// never shows a start that the next click would fail.
	reasons := make([]string, 0, 2)
	if err := pack.Validate(); err != nil {
		reasons = append(reasons, err.Error())
	}
	if err := pack.ValidateModules(dataRoot); err != nil {
		reasons = append(reasons, err.Error())
	}
	if err := pack.EffectiveSettings().validateReferences(dataRoot); err != nil {
		reasons = append(reasons, err.Error())
	}
	preview.Reasons = reasons
	preview.Startable = len(reasons) == 0
	return preview
}

// DraftNotice is the localized notice key for the cultivation template.
const DraftNotice = "worldCreation.notice.cultivation"

func DraftNoticeForGameplay(gameplay WorldGameplay) string {
	if gameplay.StatusTemplate == WorldStatusCultivation {
		return DraftNotice
	}
	if gameplay.Communication == WorldCommunicationLetter {
		return "worldCreation.notice.generalActionLetter"
	}
	return "worldCreation.notice.generalActionLocal"
}

// BuildPack assembles the pack a draft describes.
//
// The prose is the draft's. Every number, module id and file-facing identifier
// is the program's, so a draft — however it was produced, and whatever a client
// sends back after editing — cannot change what a character starts with.
func (d WorldDraft) BuildPack() (Pack, error) {
	if err := validateWorldGameplay(d.Gameplay); err != nil {
		return Pack{}, err
	}
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return Pack{}, identityError("api.storyPack.world.nameMissing")
	}
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = name
	}
	origin := strings.TrimSpace(d.Origin)
	if origin == "" {
		return Pack{}, identityError("api.storyPack.world.originMissing")
	}

	// Choosing the supported cultivation template copies its starting rules into
	// the authored pack. An empty world catalog never inherits its example world.
	if d.Gameplay.StatusTemplate == WorldStatusCultivation && d.Gameplay.CultivationRules == nil {
		d.Gameplay.CultivationRules = cultivation.ReferenceRules()
	}
	if d.Gameplay.WorldContent == nil {
		d.Gameplay.WorldContent = &interactive.WorldContent{PackID: d.DraftIDSuggestion()}
	}

	d.Gameplay.WorldContent = interactive.CloneWorldContent(d.Gameplay.WorldContent)
	d.Gameplay.CultivationRules = cultivation.CloneRules(d.Gameplay.CultivationRules)
	pack := Pack{
		Version:        Version,
		ID:             d.DraftIDSuggestion(),
		Name:           name,
		Description:    strings.TrimSpace(d.Description),
		Tags:           clipStrings(d.Tags, 8, 16),
		ContentVersion: 1,
		Story: PackStory{
			Title:                 title,
			Origin:                origin,
			ProtagonistName:       strings.TrimSpace(d.ProtagonistName),
			Protagonist:           strings.TrimSpace(d.Protagonist),
			ReplyTargetChars:      DefaultReplyTargetChars,
			ChoiceCount:           DefaultChoiceCount,
			OpeningText:           strings.TrimSpace(d.OpeningText),
			ModuleRefs:            draftModuleRefs(d.Gameplay),
			NarrativeRequirements: strings.TrimSpace(d.NarrativeRequirements),
			Gameplay:              d.Gameplay,
		},
		Creation: &PackCreation{
			AgeRange: draftAgeRange(d.AgeRange),
		},
		TopicTerms: draftTopicTerms(d.TopicTerms),
	}

	origins := d.Origins
	pack.Story.Gameplay.Presentation = d.Gameplay.Presentation.Clone()
	if d.Gameplay.StatusTemplate != WorldStatusCultivation {
		pack.assembleWorldActionCreation(origins, d.Affiliations, d.Locations, d.Gameplay, d.ProtagonistName, d.AgeRange)
	} else {
		if len(origins) == 0 {
			origins = draftDefaultOrigins()
		}
		usedBases, err := pack.assembleCreation(origins, d.Affiliations, d.Locations)
		if err != nil {
			return Pack{}, err
		}
		if err := pack.assembleTemplates(origins, usedBases, d.ProtagonistName); err != nil {
			return Pack{}, err
		}
	}

	world, err := draftWorldEntries(d.World)
	if err != nil {
		return Pack{}, err
	}
	pack.World = world
	pack.Characters = draftCharacters(d.Characters)
	if d.Gameplay.CultivationRules != nil || d.Gameplay.WorldContent != nil || d.Gameplay.AdventureRules != nil || len(d.Gameplay.CraftMethods) > 0 {
		pack.Story.ModuleRefs.ActorStateID, pack.Story.ModuleRefs.RuleSystemID = gameplayModuleIDs(pack.ID, d.Gameplay)
	}

	return pack, nil
}

// draftModuleRefs is the module set a generated world binds.
//
// It is deliberately fixed to the one complete template this build supports. A
// draft that named its own modules would let a model pick a rule system the
// story cannot actually run, which §6 forbids.
func draftModuleRefs(gameplay WorldGameplay) PackRefs {
	if gameplay.StatusTemplate == WorldStatusCultivation {
		return PackRefs{
			EventPackageIDs: []string{DefaultEventPackageID},
			RuleSystemID:    DefaultRuleSystemID,
			ActorStateID:    interactive.ActorStateXiuxianID,
			ImagePresetID:   DefaultImagePresetID,
		}
	}
	return PackRefs{
		EventPackageIDs: []string{DefaultEventPackageID},
		RuleSystemID:    interactive.RuleSystemDetectiveStatsID,
		ActorStateID:    interactive.ActorStateDetectiveID,
		ImagePresetID:   DefaultImagePresetID,
	}
}

func (p *Pack) assembleWorldActionCreation(origins []DraftOrigin, affiliations, locations []DraftOption, gameplay WorldGameplay, protagonistName string, ageRange [2]int) {
	name := strings.TrimSpace(protagonistName)
	if name == "" {
		name = "行旅者"
	}
	if len(origins) == 0 {
		origins = []DraftOrigin{{Name: "普通人", Description: "以普通人的身份开始旅程。"}}
	}
	p.Creation = &PackCreation{AgeRange: draftAgeRange(ageRange), Genders: draftGenders()}
	for index, origin := range origins {
		label := strings.TrimSpace(origin.Name)
		if label == "" {
			continue
		}
		p.Creation.Origins = append(p.Creation.Origins, CreationOrigin{
			CreationOption: CreationOption{ID: fmt.Sprintf("origin-%d", index+1), Name: clipProse(label, 24), Description: clipProse(origin.Description, 240), Default: index == 0},
			TemplateID:     "world-protagonist",
		})
	}
	p.Creation.Affiliations = draftCreationOptions(affiliations, "aff")
	if len(p.Creation.Affiliations) == 0 {
		p.Creation.Affiliations = []CreationOption{{ID: "none", Name: "独立", Default: true}}
	}
	p.Creation.Locations = draftCreationOptions(locations, "loc")
	if len(p.Creation.Locations) == 0 {
		p.Creation.Locations = []CreationOption{{ID: "loc-1", Name: "落脚处", Description: "故事开始时你所在的地方。", Default: true}}
	}
	state := map[string]any{"姓名": name, "基本身份": "", "外貌与装扮": "", "性格与背景": "", "当前处境": ""}
	for _, field := range gameplay.StateFields {
		state[field.Name] = field.Initial
	}
	if gameplay.AdventureRules != nil {
		for _, field := range interactive.AdventureStateFields(*gameplay.AdventureRules) {
			state[field.Name] = field.Default
		}
	}
	p.ProtagonistTemplates = []ProtagonistTemplate{{
		ID: "world-protagonist", Name: "主角", Description: "按本世界确认的经历与能力开始。",
		CharacterName: name, Profile: "依已确认的经历与能力行事。",
		Origin: p.Story.Origin, State: state,
	}}
	p.Creation.StateBasisTemplates = []string{"world-protagonist"}
}

func draftCreationOptions(options []DraftOption, prefix string) []CreationOption {
	result := make([]CreationOption, 0, len(options))
	for index, option := range options {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		result = append(result, CreationOption{
			ID: fmt.Sprintf("%s-%d", prefix, index+1), Name: clipProse(name, 24),
			Description: clipProse(option.Description, 240), Country: clipProse(option.Country, 24), Default: option.Default,
		})
	}
	if len(result) > 0 && !hasDefaultCreationOption(result) {
		result[0].Default = true
	}
	return result
}

func hasDefaultCreationOption(options []CreationOption) bool {
	for _, option := range options {
		if option.Default {
			return true
		}
	}
	return false
}

func draftAgeRange(given [2]int) [2]int {
	low, high := given[0], given[1]
	if low <= 0 || high <= 0 || high < low {
		return [2]int{18, 30}
	}
	if high-low > 30 {
		high = low + 30
	}
	return [2]int{low, high}
}

// assembleCreation fills the choice block and returns which bases the draft's
// origins actually used.
func (p *Pack) assembleCreation(origins []DraftOrigin, affiliations, locations []DraftOption) (map[string]bool, error) {
	c := p.Creation
	c.Genders = draftGenders()
	c.RootCategories = draftRootCategories()
	c.RootAttributes = draftRootAttributes()
	c.Aptitudes = draftAptitudes()
	c.WeaponDirections = draftWeaponDirections()
	c.Traits = draftTraits()
	c.EffectNotes = map[string]string{
		"aptitude": "real_cultivation_talent",
		"root":     "narrative",
		"weapon":   "content_tendency",
		"trait":    "narrative",
	}

	used := make(map[string]bool, len(origins))
	c.Origins = make([]CreationOrigin, 0, len(origins))
	for index, origin := range origins {
		basis, ok := draftBasisByID(origin.Basis)
		if !ok {
			// A draft naming no basis, or one the program does not own, starts
			// from the plainest position rather than failing the whole save: a
			// world with a thin origin is still a world a player may edit.
			basis, _ = draftBasisByID(draftBasisWanderer)
		}
		used[basis.ID] = true
		name := strings.TrimSpace(origin.Name)
		if name == "" {
			name = basis.DefaultName
		}
		technique := strings.TrimSpace(origin.Technique)
		if technique == "" {
			technique = basis.DefaultTech
		}
		item := strings.TrimSpace(origin.Item)
		if item == "" {
			item = basis.DefaultItem
		}
		option := CreationOption{
			ID:          fmt.Sprintf("origin-%d", index+1),
			Name:        clipProse(name, 24),
			Description: clipProse(origin.Description, 240),
			Default:     index == 0,
		}
		c.Origins = append(c.Origins, CreationOrigin{
			CreationOption: option,
			TemplateID:     basis.ID,
			InitialState: &CreationInitialState{
				Realm:      basis.Realm,
				RealmLabel: draftRealmLabel(basis.Realm),
				Currency:   basis.Currency,
				Items:      []string{item},
				Techniques: []string{technique},
			},
		})
	}

	// "无门无派" is not optional. ResolveIdentity falls back to it whenever the
	// player leaves the choice to the draft, and a world without it refuses
	// every such character with a hard error.
	c.Affiliations = []CreationOption{{
		ID: "none", Name: "无门无派", Description: "不归属任何势力，自己安排去处。", Default: true,
	}}
	for index, option := range affiliations {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		c.Affiliations = append(c.Affiliations, CreationOption{
			ID:          fmt.Sprintf("aff-%d", index+1),
			Name:        clipProse(name, 24),
			Description: clipProse(option.Description, 240),
			Country:     clipProse(option.Country, 24),
		})
	}

	// A world with no place to stand cannot resolve a character at all, so an
	// empty list becomes one plain place rather than a save that looks fine and
	// refuses every start.
	if len(locations) == 0 {
		locations = []DraftOption{{Name: "落脚处", Description: "故事开始时你所在的地方。"}}
	}
	c.Locations = make([]CreationOption, 0, len(locations))
	for index, option := range locations {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		c.Locations = append(c.Locations, CreationOption{
			ID:          fmt.Sprintf("loc-%d", index+1),
			Name:        clipProse(name, 24),
			Description: clipProse(option.Description, 240),
			Country:     clipProse(option.Country, 24),
			Default:     option.Default || index == 0,
		})
	}
	if len(c.Locations) == 0 {
		c.Locations = []CreationOption{{
			ID: "loc-1", Name: "落脚处", Description: "故事开始时你所在的地方。", Default: true,
		}}
	}
	return used, nil
}

// assembleTemplates writes one ready-made start per basis the draft used.
//
// The state is the program's: the realm, the purse, the carried thing and the
// technique definition are fixed per basis, so a generated world's character is
// worth exactly what a built-in world's character is worth at the same basis.
// Only the prose fields — who this person is, what they look like, where they
// stand — come from the draft.
func (p *Pack) assembleTemplates(origins []DraftOrigin, used map[string]bool, protagonistName string) error {
	name := strings.TrimSpace(protagonistName)
	if name == "" {
		name = "无名行者"
	}
	byBasis := make(map[string]DraftOrigin, len(origins))
	for _, origin := range origins {
		basis := strings.TrimSpace(origin.Basis)
		if _, seen := byBasis[basis]; !seen {
			byBasis[basis] = origin
		}
	}

	templates := make([]ProtagonistTemplate, 0, len(used))
	stateBasis := make([]string, 0, len(used))
	for _, basis := range draftBases() {
		if !used[basis.ID] {
			continue
		}
		origin, hasOrigin := byBasis[basis.ID]
		templateName := basis.DefaultName
		description := ""
		technique := basis.DefaultTech
		item := basis.DefaultItem
		if hasOrigin {
			if value := strings.TrimSpace(origin.Name); value != "" {
				templateName = value
			}
			description = clipProse(origin.Description, 240)
			if value := strings.TrimSpace(origin.Technique); value != "" {
				technique = value
			}
			if value := strings.TrimSpace(origin.Item); value != "" {
				item = value
			}
		}
		profile := description
		if profile == "" {
			profile = fmt.Sprintf("%s。%s出身，%s。", name, templateName, draftRealmLabel(basis.Realm))
		}
		templates = append(templates, ProtagonistTemplate{
			ID:            basis.ID,
			Name:          clipProse(templateName, 24),
			Description:   description,
			CharacterName: clipProse(name, 24),
			Profile:       profile,
			Origin:        strings.TrimSpace(p.Story.Origin),
			State:         draftTemplateState(name, basis, technique, item),
		})
		stateBasis = append(stateBasis, basis.ID)
	}
	if len(templates) == 0 {
		return identityError("api.storyPack.world.originsMissing")
	}
	p.ProtagonistTemplates = templates
	p.Creation.StateBasisTemplates = stateBasis
	return nil
}

// draftTemplateState is a starting position, in the shape the bound actor-state
// module declares.
//
// Every key here is one the xiuxian preset's protagonist template names; a key
// it does not declare would be silently dropped at normalization, which is why
// the set is fixed here rather than assembled from whatever the model wrote.
func draftTemplateState(name string, basis draftBasis, technique, item string) map[string]any {
	return map[string]any{
		"姓名":      name,
		"基本身份":    "",
		"外貌与装扮":   "",
		"性格与背景":   "",
		"当前处境":    "",
		"当前境界":    basis.Realm,
		"功法与专精":   "",
		"灵根与特殊体质": "",
		"已学功法":    map[string]any{technique: map[string]any{"定义": "strike", "版本": 1, "掌握程度": "熟练"}},
		"主法器":     map[string]any{},
		"护具":      map[string]any{},
		"布阵":      map[string]any{},
		// The three 法宝 slots are seeded empty, not filled: 《自制-装备栏与法宝御使》
		// §4 says 没有依据的装备不猜类别、不凭空发满3个法宝位. Seeding them makes the
		// starting sheet show the same six equipment rows the running rules read;
		// an empty object is the honest starting value for a character the draft
		// never gave a treasure to.
		"攻击法宝":  map[string]any{},
		"防御法宝":  map[string]any{},
		"辅助法宝":  map[string]any{},
		"重要物品":  map[string]any{item: map[string]any{"定义": "pill.restore.health.lower", "数量": 1, "作用": "疗伤"}},
		"灵石":    basis.Currency,
		"关系":    map[string]any{},
		"技能与能力": map[string]any{},
		"持续状态":  []any{},
		"冷却状态":  []any{},
	}
}

func draftDefaultOrigins() []DraftOrigin {
	bases := draftBases()
	origins := make([]DraftOrigin, 0, len(bases))
	for _, basis := range bases {
		origins = append(origins, DraftOrigin{Name: basis.DefaultName, Basis: basis.ID})
	}
	return origins
}

// draftWorldEntries assigns the stable ids the pack format requires.
//
// A model cannot be trusted to keep two ids unique across a long document, and
// a duplicate id fails validation after the player has already waited for the
// draft. Numbering here makes that impossible.
func draftWorldEntries(entries []DraftWorldEntry) ([]WorldEntry, error) {
	out := make([]WorldEntry, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		out = append(out, WorldEntry{
			Audience:         strings.TrimSpace(entry.Audience),
			ID:               fmt.Sprintf("world-%d", len(out)+1),
			Name:             clipProse(name, 40),
			Type:             firstNonEmpty(strings.TrimSpace(entry.Type), "world"),
			Importance:       firstNonEmpty(strings.TrimSpace(entry.Importance), "medium"),
			Tags:             clipStrings(entry.Tags, 8, 16),
			Keywords:         clipStrings(entry.Keywords, 16, 16),
			LoadMode:         worldEntryLoadMode(entry.Type, entry.LoadMode),
			BriefDescription: clipProse(entry.BriefDescription, 120),
			Content:          strings.TrimSpace(entry.Content),
		})
	}
	if len(out) == 0 {
		return nil, identityError("api.storyPack.world.entriesMissing")
	}
	return out, nil
}

func draftCharacters(characters []DraftCharacter) []CatalogCharacter {
	out := make([]CatalogCharacter, 0, len(characters))
	for _, character := range characters {
		name := strings.TrimSpace(character.Name)
		if name == "" {
			continue
		}
		place := clipProse(character.Place, 40)
		placeLabel := clipProse(character.PlaceLabel, 40)
		tokens := clipStrings([]string{place, placeLabel}, 8, 16)
		out = append(out, CatalogCharacter{
			SourceKey:      fmt.Sprintf("draft-%d", len(out)+1),
			Name:           clipProse(name, 24),
			Age:            character.Age,
			Gender:         character.Gender,
			SpeechStyle:    clipProse(character.SpeechStyle, 120),
			Aliases:        clipStrings(character.Aliases, 4, 24),
			Country:        clipProse(character.Country, 24),
			Role:           clipProse(character.Role, 40),
			Topics:         clipStrings(character.Topics, 6, 16),
			PlaceCandidate: place,
			PlaceLabel:     placeLabel,
			RegionTokens:   tokens,
			Level:          firstNonEmpty(strings.TrimSpace(character.Level), "重要"),
			Brief:          clipProse(character.Brief, 160),
			InitialStage:   clipProse(character.InitialStage, 24),
			ContactGate:    "contextual_local_encounter_or_known_contact",
		})
	}
	return out
}

func draftTopicTerms(terms map[string][]string) map[string][]string {
	if len(terms) == 0 {
		return nil
	}
	out := make(map[string][]string, len(terms))
	for category, words := range terms {
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		if kept := clipStrings(words, 12, 16); len(kept) > 0 {
			out[category] = kept
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// draftRealmLabel turns a stage key into the wording the creation page shows.
// The rules' own identifiers never reach a player.
func draftRealmLabel(key string) string {
	c, ok := cultivation.ParseStageKey(key)
	if !ok {
		return key
	}
	switch c.Realm {
	case cultivation.RealmMortal:
		return "凡人"
	case cultivation.RealmQi:
		return fmt.Sprintf("炼气%d层", c.Layer)
	case cultivation.RealmFoundation:
		return draftSubStageLabel("筑基", c.Stage)
	case cultivation.RealmCore:
		return draftSubStageLabel("结丹", c.Stage)
	case cultivation.RealmNascent:
		return draftSubStageLabel("元婴", c.Stage)
	case cultivation.RealmSpirit:
		return draftSubStageLabel("化神", c.Stage)
	case cultivation.RealmVoid:
		return draftSubStageLabel("炼虚", c.Stage)
	case cultivation.RealmIntegration:
		return draftSubStageLabel("合体", c.Stage)
	case cultivation.RealmMahayana:
		return draftSubStageLabel("大乘", c.Stage)
	}
	return key
}

func draftSubStageLabel(realm string, stage cultivation.Stage) string {
	switch stage {
	case cultivation.StageEarly:
		return realm + "初期"
	case cultivation.StageMiddle:
		return realm + "中期"
	case cultivation.StageLate:
		return realm + "后期"
	}
	return realm
}

// The option tables below are the same wording the built-in cultivation world
// offers. They are written here rather than read from an asset so a generated
// world has no file dependency, and so the tiers the rules read stay in one
// place per package.

func draftGenders() []CreationOption {
	return []CreationOption{
		{ID: "male", Name: "男"},
		{ID: "female", Name: "女"},
		{ID: "unspecified", Name: "未定", Default: true},
	}
}

func draftRootCategories() []CreationOption {
	return []CreationOption{
		{ID: "single", Name: "单灵根", Description: "一系独纯，修行快而偏。"},
		{ID: "dual", Name: "双灵根", Description: "两系并存，常见而稳。"},
		{ID: "triple", Name: "三灵根", Description: "三系驳杂，起步慢。"},
		{ID: "quad", Name: "四灵根", Description: "四系俱全，修行迟缓。"},
		{ID: "quinary", Name: "五灵根", Description: "五行俱全，最是吃力。"},
		{ID: "pseudo", Name: "伪灵根", Description: "有灵根而驳杂不纯，勉强可修。"},
		{ID: "waste", Name: "废灵根", Description: "灵根近乎闭塞，另寻他路。"},
	}
}

func draftRootAttributes() []CreationOption {
	return []CreationOption{
		{ID: "metal", Name: "金"},
		{ID: "wood", Name: "木"},
		{ID: "water", Name: "水"},
		{ID: "fire", Name: "火"},
		{ID: "earth", Name: "土"},
	}
}

// draftAptitudes is the one group the rules really read: the talent token is
// written into 灵根与特殊体质 and normalized by the cultivation settlement.
func draftAptitudes() []CreationAptitude {
	return []CreationAptitude{
		{CreationOption: CreationOption{ID: "poor", Name: "较差"}, Talent: "poor"},
		{CreationOption: CreationOption{ID: "normal", Name: "普通", Default: true}, Talent: "normal"},
		{CreationOption: CreationOption{ID: "good", Name: "良好"}, Talent: "good"},
		{CreationOption: CreationOption{ID: "excellent", Name: "优秀"}, Talent: "excellent"},
		{CreationOption: CreationOption{ID: "exceptional", Name: "极佳"}, Talent: "exceptional"},
	}
}

func draftWeaponDirections() []CreationOption {
	return []CreationOption{
		{ID: "none", Name: "无偏好", Default: true},
		{ID: "sword", Name: "剑"},
		{ID: "blade", Name: "刀"},
		{ID: "spear", Name: "枪"},
		{ID: "fist", Name: "拳掌"},
		{ID: "bow", Name: "弓"},
		{ID: "talisman", Name: "符"},
	}
}

func draftTraits() []CreationOption {
	return []CreationOption{
		{ID: "steady", Name: "沉稳"},
		{ID: "curious", Name: "好奇"},
		{ID: "guarded", Name: "戒备"},
		{ID: "softhearted", Name: "心软"},
		{ID: "taciturn", Name: "寡言"},
		{ID: "resentful", Name: "记怨"},
	}
}

// clipStrings trims, drops empties, de-duplicates and caps a list.
func clipStrings(values []string, limit, maxRunes int) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = clipProse(value, maxRunes)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// DraftIDSuggestion is the file-facing identifier a draft would be saved under.
//
// 自制-AI创建世界包与武侠规则复用 §7 asks that the stable id and version be the
// program's, with the model supplying only a name and content. The id is derived
// from the name here and confirmed against what is actually on disk at save
// time, so the same name never silently overwrites an earlier world.
func (d WorldDraft) DraftIDSuggestion() string {
	return draftIDFromName(d.Name)
}

// draftIDFromName keeps the readable part of a name and drops everything the
// pack format refuses in an id.
func draftIDFromName(name string) string {
	var sb strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-', r == '_':
			sb.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			// A Chinese name is the ordinary case, and dropping it would leave
			// every generated world called "world".
			sb.WriteRune(r)
		}
		if sb.Len() >= 32 {
			break
		}
	}
	base := strings.Trim(sb.String(), "-_")
	if base == "" {
		base = "world"
	}
	return "custom-" + base
}

// WorldSaveResult is what a save wrote.
type WorldSaveResult struct {
	PackID string `json:"pack_id"`
	// Summary is the pack as the world list will now show it, so the page can
	// move straight to it without a second round trip.
	Summary Summary `json:"summary"`
}

// SaveWorld writes a draft as a new user pack.
//
// The draft is validated with the same three checks a load performs — document
// shape, installed modules, and settings references — before anything is
// written, because a pack that saves and then refuses to start is worse than a
// save that failed and said why.
//
// The write is create-only: it never replaces an existing file, and a derived id
// that is already taken gets a suffix. That is §7's "stable id managed by the
// program": a second world with the same name is a second world, not an
// overwrite of the first.
func (l *Library) SaveWorld(ctx context.Context, draft WorldDraft) (WorldSaveResult, error) {
	pack, err := draft.BuildPack()
	if err != nil {
		return WorldSaveResult{}, err
	}
	if err := pack.Validate(); err != nil {
		return WorldSaveResult{}, err
	}
	if err := pack.ValidateModules(l.dataRoot); err != nil {
		return WorldSaveResult{}, err
	}
	if err := pack.EffectiveSettings().validateReferences(l.dataRoot); err != nil {
		return WorldSaveResult{}, err
	}

	base := draft.DraftIDSuggestion()
	// A handful of suffixes is enough for a player naming worlds, and a bounded
	// loop means a broken data root cannot hang the save.
	for attempt := 0; attempt < 200; attempt++ {
		id := base
		if attempt > 0 {
			id = fmt.Sprintf("%s-%d", base, attempt+1)
		}
		pack.ID = id
		if c := pack.Story.Gameplay.WorldContent; c != nil {
			c.PackID = id
			for i := range c.Quests {
				c.Quests[i].PackageID = id
			}
			for i := range c.Realms {
				c.Realms[i].PackageID = id
			}
			for i := range c.Events {
				c.Events[i].PackageID = id
			}
		}
		created, err := l.materializeGameplayModules(&pack)
		if err != nil {
			return WorldSaveResult{}, err
		}
		if err := pack.ValidateModules(l.dataRoot); err != nil {
			created.rollback(interactive.NewActorStateLibrary(l.dataRoot), interactive.NewRuleSystemLibrary(l.dataRoot))
			return WorldSaveResult{}, err
		}
		content, err := json.MarshalIndent(pack, "", "  ")
		if err != nil {
			created.rollback(interactive.NewActorStateLibrary(l.dataRoot), interactive.NewRuleSystemLibrary(l.dataRoot))
			return WorldSaveResult{}, err
		}
		content = append(content, '\n')
		path := filepath.Join(l.dir(), id+fileExtension)
		result, err := revisionfile.ReplaceIfRevision(ctx, path, revisionfile.MissingRevision, content,
			revisionfile.Options{FileMode: 0o644, DirectoryMode: 0o755})
		if errors.Is(err, revisionfile.ErrRevisionConflict) {
			// The id is taken. A builtin with this id also counts as taken: the
			// save would shadow it on the next load, which is exactly the
			// overwrite §7 forbids.
			created.rollback(interactive.NewActorStateLibrary(l.dataRoot), interactive.NewRuleSystemLibrary(l.dataRoot))
			continue
		}
		if err != nil {
			created.rollback(interactive.NewActorStateLibrary(l.dataRoot), interactive.NewRuleSystemLibrary(l.dataRoot))
			return WorldSaveResult{}, err
		}
		if !result.Changed {
			continue
		}
		return WorldSaveResult{PackID: id, Summary: pack.Describe(l.dataRoot)}, nil
	}
	return WorldSaveResult{}, identityError("api.storyPack.world.idUnavailable", "base", base)
}
