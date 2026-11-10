package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"ta/internal/agents/configresource"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/revisionfile"
	"ta/internal/storypack"
)

// agentScopedSettings is the write path a purpose-scoped game assistant uses.
//
// It adds no storage: a story edit goes through the same UpdateInteractiveStory
// the Game settings page calls, and a world edit goes through the same
// SaveSettings the world page calls. What it adds is the binding — every call
// names one story or one world, and a name that does not match the story the
// conversation was opened for is refused before any record is touched.
type agentScopedSettings struct {
	app *App
}

func (s agentScopedSettings) ReadStorySettings(_ context.Context, storyID string) (configresource.StorySettingsValue, error) {
	story, err := s.story(storyID)
	if err != nil {
		return configresource.StorySettingsValue{}, err
	}
	return storySettingsValue(story), nil
}

func (s agentScopedSettings) UpdateStorySettings(_ context.Context, storyID string, value configresource.StorySettingsValue) (configresource.StorySettingsValue, error) {
	current, err := s.story(storyID)
	if err != nil {
		return configresource.StorySettingsValue{}, err
	}
	revision := storySettingsRevision(storySettingsValue(current))
	if expected := strings.TrimSpace(value.Revision); expected != "" && expected != revision {
		return configresource.StorySettingsValue{}, fmt.Errorf(
			"story %q settings changed since they were read: expected revision %s, current %s",
			storyID, expected, revision,
		)
	}
	request := interactive.UpdateStoryRequest{
		ReplyTargetChars: value.ReplyTargetChars,
		ChoiceCount:      value.ChoiceCount,
		PlanningMode:     value.PlanningMode,
	}
	if value.PlanningTemplateID != nil {
		request.PlanningTemplateID = *value.PlanningTemplateID
	}
	if value.StylePurposes != nil {
		request.StylePurposes = &interactive.StoryStylePurposesUpdate{
			Main: value.StylePurposes.Main, Dialogue: value.StylePurposes.Dialogue, Combat: value.StylePurposes.Combat,
		}
	}
	if value.CheckSettings != nil {
		request.CheckSettings = &interactive.StoryCheckSettings{
			DifficultyShift:          intOrZero(value.CheckSettings.DifficultyShift),
			RollModifier:             intOrZero(value.CheckSettings.RollModifier),
			RuleStateConsumptionMode: stringOrEmpty(value.CheckSettings.RuleStateConsumptionMode),
			RuleVisibilityMode:       stringOrEmpty(value.CheckSettings.RuleVisibilityMode),
		}
	}
	updated, err := s.app.UpdateInteractiveStory(storyID, request)
	if err != nil {
		return configresource.StorySettingsValue{}, err
	}
	return storySettingsValue(updated), nil
}

func (s agentScopedSettings) ReadWorldDefaults(_ context.Context, worldID string) (configresource.WorldDefaultsValue, error) {
	pack, err := s.app.StoryPacks().Get(worldID)
	if err != nil {
		return configresource.WorldDefaultsValue{}, err
	}
	revision, _ := s.app.StoryPacks().SettingsRevision(worldID)
	// EffectiveSettings is what the world page shows: the saved block with the
	// application's fallbacks filled in. Reporting the raw block instead would
	// show blank fields for values that are actually in force.
	return worldDefaultsValue(pack.EffectiveSettings(), revision), nil
}

func (s agentScopedSettings) UpdateWorldDefaults(_ context.Context, worldID string, value configresource.WorldDefaultsValue, expectedRevision string) (configresource.WorldDefaultsValue, error) {
	// The block is written as a whole, so the read value is the base and only
	// the fields this request names replace it. Anything else — the author's
	// creative direction in particular — survives a partial edit.
	current, err := s.app.StoryPacks().Get(worldID)
	if err != nil {
		return configresource.WorldDefaultsValue{}, err
	}
	next := current.Settings
	if value.PlanningMode != nil {
		next.PlanningMode = *value.PlanningMode
	}
	if value.PlanningTemplateID != nil {
		next.PlanningTemplateID = *value.PlanningTemplateID
	}
	if value.StylePurposes != nil {
		if value.StylePurposes.Main != nil {
			next.StylePurposes.Main = *value.StylePurposes.Main
		}
		if value.StylePurposes.Dialogue != nil {
			next.StylePurposes.Dialogue = *value.StylePurposes.Dialogue
		}
		if value.StylePurposes.Combat != nil {
			next.StylePurposes.Combat = *value.StylePurposes.Combat
		}
	}
	if value.ReplyTargetChars != nil {
		next.ReplyTargetChars = *value.ReplyTargetChars
	}
	if value.CheckSettings != nil {
		if value.CheckSettings.DifficultyShift != nil {
			next.CheckSettings.DifficultyShift = *value.CheckSettings.DifficultyShift
		}
		if value.CheckSettings.RollModifier != nil {
			next.CheckSettings.RollModifier = *value.CheckSettings.RollModifier
		}
		if value.CheckSettings.RuleStateConsumptionMode != nil {
			next.CheckSettings.RuleStateConsumptionMode = *value.CheckSettings.RuleStateConsumptionMode
		}
		if value.CheckSettings.RuleVisibilityMode != nil {
			next.CheckSettings.RuleVisibilityMode = *value.CheckSettings.RuleVisibilityMode
		}
	}
	result, err := s.app.StoryPacks().SaveSettings(context.Background(), worldID, next, expectedRevision)
	if err != nil {
		return configresource.WorldDefaultsValue{}, err
	}
	updated, err := s.app.StoryPacks().Get(worldID)
	if err != nil {
		return configresource.WorldDefaultsValue{}, err
	}
	return worldDefaultsValue(updated.EffectiveSettings(), result.Revision), nil
}

// ReadWorldBook reads the authored entries of the world the conversation is
// bound to, and reports whether that world can take a collection.
func (s agentScopedSettings) ReadWorldBook(_ context.Context, worldID string) (configresource.WorldBookValue, error) {
	snapshot, err := s.app.StoryPacks().WorldBook(worldID)
	if err != nil {
		return configresource.WorldBookValue{}, err
	}
	value := configresource.WorldBookValue{
		WorldID:      snapshot.PackID,
		CraftMethods: snapshot.CraftMethods,
		Writable:     snapshot.Writable,
		Entries:      make([]configresource.WorldEntryValue, 0, len(snapshot.Entries)),
	}
	for _, entry := range snapshot.Entries {
		value.Entries = append(value.Entries, configresource.WorldEntryValue{
			Audience:         lore.NormalizeAudience(entry.Audience),
			EntryID:          entry.ID,
			Name:             entry.Name,
			Type:             entry.Type,
			Importance:       entry.Importance,
			Tags:             entry.Tags,
			Keywords:         entry.Keywords,
			LoadMode:         entry.LoadMode,
			BriefDescription: entry.BriefDescription,
			Content:          entry.Content,
			SourceRef:        entry.SourceRef,
		})
	}
	return value, nil
}

// CollectWorldEntry files one story entity into the world the conversation is
// bound to.
//
// It writes the pack file the world page edits, so the entry a new game of this
// world reads and the entry this assistant filed are one record. A builtin world
// is refused by the library rather than shadowed, and the error travels back
// unchanged so the assistant can tell the player to save their own copy first.
func (s agentScopedSettings) CollectWorldEntry(ctx context.Context, worldID string, value configresource.WorldEntryValue) (configresource.WorldEntryCollected, error) {
	result, err := s.app.StoryPacks().CollectWorldEntry(ctx, worldID, storypack.WorldEntryDraft{
		Audience:         lore.NormalizeAudience(value.Audience),
		CraftMethod:      value.CraftMethod,
		ID:               value.EntryID,
		Name:             value.Name,
		Type:             value.Type,
		Importance:       value.Importance,
		Tags:             value.Tags,
		Keywords:         value.Keywords,
		LoadMode:         value.LoadMode,
		BriefDescription: value.BriefDescription,
		Content:          value.Content,
		SourceRef:        value.SourceRef,
	})
	if err != nil {
		return configresource.WorldEntryCollected{}, err
	}
	return configresource.WorldEntryCollected{
		PackID:         result.PackID,
		EntryID:        result.EntryID,
		Created:        result.Created,
		ContentVersion: result.ContentVersion,
		Entry: configresource.WorldEntryValue{
			Audience:         lore.NormalizeAudience(result.Entry.Audience),
			CraftMethod:      result.CraftMethod,
			EntryID:          result.Entry.ID,
			Name:             result.Entry.Name,
			Type:             result.Entry.Type,
			Importance:       result.Entry.Importance,
			Tags:             result.Entry.Tags,
			Keywords:         result.Entry.Keywords,
			LoadMode:         result.Entry.LoadMode,
			BriefDescription: result.Entry.BriefDescription,
			Content:          result.Entry.Content,
			SourceRef:        result.Entry.SourceRef,
		},
	}, nil
}

func (s agentScopedSettings) story(storyID string) (interactive.StorySummary, error) {
	storyID = strings.TrimSpace(storyID)
	if storyID == "" {
		return interactive.StorySummary{}, fmt.Errorf("story id is required")
	}
	index, err := s.app.InteractiveStories()
	if err != nil {
		return interactive.StorySummary{}, err
	}
	for _, story := range index.Stories {
		if story.ID == storyID {
			return story, nil
		}
	}
	return interactive.StorySummary{}, fmt.Errorf("story %q does not exist in this project", storyID)
}

func storySettingsValue(story interactive.StorySummary) configresource.StorySettingsValue {
	planningMode := story.PlanningMode
	planningTemplate := story.PlanningTemplateID
	replyTargetChars := story.ReplyTargetChars
	choiceCount := story.ChoiceCount
	main := story.StylePurposes.Main
	dialogue := story.StylePurposes.Dialogue
	combat := story.StylePurposes.Combat
	difficultyShift := story.CheckSettings.DifficultyShift
	rollModifier := story.CheckSettings.RollModifier
	consumption := story.CheckSettings.RuleStateConsumptionMode
	visibility := story.CheckSettings.RuleVisibilityMode
	value := configresource.StorySettingsValue{
		PlanningMode:       &planningMode,
		PlanningTemplateID: &planningTemplate,
		StylePurposes: &configresource.StylePurposesValue{
			Main: &main, Dialogue: &dialogue, Combat: &combat,
		},
		ReplyTargetChars: &replyTargetChars,
		ChoiceCount:      &choiceCount,
		CheckSettings: &configresource.CheckSettingsValue{
			DifficultyShift: &difficultyShift, RollModifier: &rollModifier,
			RuleStateConsumptionMode: &consumption, RuleVisibilityMode: &visibility,
		},
	}
	value.Revision = storySettingsRevision(value)
	return value
}

func worldDefaultsValue(settings storypack.PackSettings, revision string) configresource.WorldDefaultsValue {
	planningMode := settings.PlanningMode
	planningTemplate := settings.PlanningTemplateID
	replyTargetChars := settings.ReplyTargetChars
	main := settings.StylePurposes.Main
	dialogue := settings.StylePurposes.Dialogue
	combat := settings.StylePurposes.Combat
	difficultyShift := settings.CheckSettings.DifficultyShift
	rollModifier := settings.CheckSettings.RollModifier
	consumption := settings.CheckSettings.RuleStateConsumptionMode
	visibility := settings.CheckSettings.RuleVisibilityMode
	value := configresource.WorldDefaultsValue{
		PlanningMode:       &planningMode,
		PlanningTemplateID: &planningTemplate,
		StylePurposes: &configresource.StylePurposesValue{
			Main: &main, Dialogue: &dialogue, Combat: &combat,
		},
		ReplyTargetChars: &replyTargetChars,
		CheckSettings: &configresource.CheckSettingsValue{
			DifficultyShift: &difficultyShift, RollModifier: &rollModifier,
			RuleStateConsumptionMode: &consumption, RuleVisibilityMode: &visibility,
		},
	}
	value.Revision = strings.TrimSpace(revision)
	if value.Revision == "" {
		value.Revision = revisionfile.MissingRevision
	}
	return value
}

// storySettingsRevision is a content revision rather than a timestamp.
//
// A turn appends to the story journal and moves the story's updated_at without
// touching any of these fields, so a timestamp would fail an edit the player
// asked for simply because a turn landed in between. Hashing exactly the fields
// this surface exposes reports the case that matters: another edit to the same
// settings.
func storySettingsRevision(value configresource.StorySettingsValue) string {
	value.Revision = ""
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return "settings-" + hex.EncodeToString(digest[:8])
}

func intOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
