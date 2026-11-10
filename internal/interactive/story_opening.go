package interactive

import (
	"fmt"
	"strings"
)

// The two general-action openings a world can declare. They mirror the world
// pack's own values; the interactive package cannot import the pack format, so
// the strings are repeated here rather than shared.
const (
	gameplayOpeningScene = "scene"
	gameplayOpeningDaily = "daily"
)

// StoryOpeningInstruction derives the first-turn model instruction entirely
// from durable story metadata. It is host-owned, English-only, and is committed
// as model-only input so it never appears as a player-authored action.
func StoryOpeningInstruction(meta StoryMeta) (string, error) {
	meta = normalizeStoryMeta(meta)
	opening := meta.Opening
	var source string
	switch opening.Mode {
	case StoryOpeningModeAI:
		source = "Create the opening from the story premise."
	case StoryOpeningModePreset:
		if opening.PresetText == "" {
			return "", fmt.Errorf("preset opening text is required")
		}
		source = "Preserve the core scene and hook from this story-owned preset snapshot:\n" + opening.PresetText
	case StoryOpeningModeCustom:
		if opening.CustomText == "" {
			return "", fmt.Errorf("custom opening text is required")
		}
		source = "Treat this user-authored opening as authoritative and continue from it:\n" + opening.CustomText
	default:
		return "", fmt.Errorf("unsupported story opening mode: %q", opening.Mode)
	}
	premise := strings.TrimSpace(meta.Origin)
	if premise == "" {
		premise = "No additional premise was provided."
	}
	parts := []string{"[Source: story opening configuration; purpose: generate the first playable turn]"}
	if meta.TitleSource != StoryTitleSourcePending {
		parts = append(parts, "Story title: "+strings.TrimSpace(meta.Title))
	}
	parts = append(parts, "Story premise: "+premise, source)
	if meta.Protagonist.Mode == StoryProtagonistModeDefault {
		parts = append(parts, "No protagonist tag matched. Before writing prose, choose the best player-controlled protagonist from the provided enabled Lore character catalog and call select_story_protagonist with that character's exact Lore item ID. Every enabled Lore character is eligible; tags are recommendations, never restrictions.")
	}
	if guidance := gameplayOpeningGuidance(meta.PackBinding); guidance != "" {
		parts = append(parts, guidance)
	}
	parts = append(parts, "Generate the first playable interactive-story scene. Establish the immediate situation, a meaningful objective or pressure, and actionable space for the protagonist. Do not explain rules or provide an outline. Output narrative only and stop at a meaningful decision point.")
	return strings.Join(parts, "\n\n"), nil
}

// gameplayOpeningGuidance carries the world's own opening form into the first
// turn. It used to travel inside the story's narrative requirements, which no
// longer reach the model; without this the world would declare "enter on a
// scene already under way" or "start with the protagonist's ordinary day" and
// the opening would ignore it.
func gameplayOpeningGuidance(binding *StoryPackBinding) string {
	if binding == nil || binding.Gameplay == nil {
		return ""
	}
	switch binding.Gameplay.Opening {
	case gameplayOpeningScene:
		return "本世界指定的开局方式是直接进入现场：从已经发生的现场或行动写起，不要先由委托人讲述尚未出现的事情。"
	case gameplayOpeningDaily:
		return "本世界指定的开局方式是日常起手：先写主角的日常，再由已确认的委托或事件自然引入后续行动。"
	default:
		return ""
	}
}
