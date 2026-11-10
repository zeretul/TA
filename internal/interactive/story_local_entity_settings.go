package interactive

import (
	"fmt"
	"strings"
)

// LocalEntitySettings is what the player decided about one entity this story
// registered, as opposed to what a turn established about it.
//
// The distinction matters. Everything else the 本局新建 page shows is projected
// from the formal story — the person, the place, the faction, and every fact the
// narrator wrote. These two fields are the only ones the player owns:
//
//   - Tags are the labels a reader would file the entity under, so a player who
//     remembers "药铺" finds the clerk whose name they forgot. A turn's own tags
//     stay on the entity; these are added to them, never instead of them.
//   - LoadMode says how much of this entity the model is handed each turn. It is
//     a reading budget, not a stage direction: Resident puts the entity in front
//     of the model every turn, and nothing about that makes the entity present,
//     able to act, or forced into the scene.
//
// A nil map is the ordinary case: a story whose player has never opened this
// page has decided nothing, and every entity then reads with the defaults.
type LocalEntitySettings struct {
	Tags []string `json:"tags,omitempty"`
	// LoadMode is one of the lore load modes. Empty means the entity keeps the
	// default it had before this page existed, which is the same treatment as
	// Auto: the model sees it when the turn is about it.
	LoadMode string `json:"load_mode,omitempty"`
}

// The three readings an entity can have. They reuse the world book's vocabulary
// rather than inventing a second one, because a player who set 常驻取用 on a
// world entry and then meets the same setting here should not have to learn two
// names for one idea.
const (
	LocalEntityLoadAuto     = "auto"
	LocalEntityLoadResident = "resident"
	LocalEntityLoadManual   = "manual"
)

// NormalizeLocalEntityLoadMode folds a load mode to one of the three known
// readings. An unrecognized value is refused rather than silently stored: a
// setting the runtime cannot act on would look saved and do nothing.
func NormalizeLocalEntityLoadMode(value string) (string, error) {
	switch mode := strings.TrimSpace(value); mode {
	case "":
		return "", nil
	case LocalEntityLoadAuto, LocalEntityLoadResident, LocalEntityLoadManual:
		return mode, nil
	default:
		return "", fmt.Errorf("取用方式只能是自动、常驻或指定 / unknown load mode: %q", mode)
	}
}

// normalizeLocalEntitySettings cleans one entity's settings for storage.
//
// Tags are trimmed, de-duplicated, and capped so a paste cannot grow the story
// journal without bound. An entity that ends up with neither tags nor a load
// mode is removed from the map entirely: keeping an empty record would make the
// file claim a decision nobody made.
func normalizeLocalEntitySettings(settings LocalEntitySettings) (LocalEntitySettings, bool, error) {
	mode, err := NormalizeLocalEntityLoadMode(settings.LoadMode)
	if err != nil {
		return LocalEntitySettings{}, false, err
	}
	tags := make([]string, 0, len(settings.Tags))
	seen := make(map[string]struct{}, len(settings.Tags))
	for _, raw := range settings.Tags {
		tag := strings.Join(strings.Fields(raw), " ")
		if tag == "" {
			continue
		}
		tagRunes := []rune(tag)
		if len(tagRunes) > 32 {
			tag = string(tagRunes[:32])
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
		if len(tags) >= 12 {
			break
		}
	}
	cleaned := LocalEntitySettings{Tags: tags, LoadMode: mode}
	return cleaned, len(tags) > 0 || mode != "", nil
}

// normalizeLocalEntitySettingsMap cleans a whole map, dropping entities that
// decided nothing and keys that name nothing.
func normalizeLocalEntitySettingsMap(input map[string]LocalEntitySettings) (map[string]LocalEntitySettings, error) {
	if len(input) == 0 {
		return nil, nil
	}
	out := make(map[string]LocalEntitySettings, len(input))
	for key, settings := range input {
		id := strings.TrimSpace(key)
		if id == "" {
			continue
		}
		cleaned, keep, err := normalizeLocalEntitySettings(settings)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if keep {
			out[id] = cleaned
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// cloneLocalEntitySettings deep-copies the map, because the tag slices are the
// one part of a story's metadata a caller could otherwise alias and mutate
// through the copy.
func cloneLocalEntitySettings(input map[string]LocalEntitySettings) map[string]LocalEntitySettings {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]LocalEntitySettings, len(input))
	for id, settings := range input {
		out[id] = LocalEntitySettings{
			Tags:     append([]string(nil), settings.Tags...),
			LoadMode: settings.LoadMode,
		}
	}
	return out
}

// LocalEntityLoadModes returns the load mode each entity the player set one on
// should be read with. Entities with no mode are absent, which the projection
// treats as the default reading.
func (meta StoryMeta) LocalEntityLoadModes() map[string]string {
	if len(meta.LocalEntitySettings) == 0 {
		return nil
	}
	modes := make(map[string]string, len(meta.LocalEntitySettings))
	for id, settings := range meta.LocalEntitySettings {
		if mode := strings.TrimSpace(settings.LoadMode); mode != "" {
			modes[id] = mode
		}
	}
	if len(modes) == 0 {
		return nil
	}
	return modes
}
