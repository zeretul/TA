package interactive

import (
	"fmt"
	"strings"
)

// MaxStoryNarrativeRequirementsBytes bounds a story's own creative direction.
// The value is injected verbatim into the leading model context, so it needs a
// hard ceiling the same way the workspace instruction files do; without one a
// pasted manuscript would silently consume the context budget on every turn.
const MaxStoryNarrativeRequirementsBytes = 16 * 1024

// normalizeStoryNarrativeRequirements trims a story's creative direction and
// normalizes line endings. An all-whitespace value becomes empty, which is the
// stored form of "this story has no override and reads the workspace default".
func normalizeStoryNarrativeRequirements(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSpace(value)
}

// ValidateStoryNarrativeRequirements is the exported form of the same check,
// for callers that hold a world's default direction before any story exists.
func ValidateStoryNarrativeRequirements(value string) error {
	return validateStoryNarrativeRequirements(value)
}

// validateStoryNarrativeRequirements rejects a direction too large to inject.
// It runs on create and update so an oversize value fails at the point the
// player saves it rather than at the next turn's context assembly.
func validateStoryNarrativeRequirements(value string) error {
	value = normalizeStoryNarrativeRequirements(value)
	if len([]byte(value)) > MaxStoryNarrativeRequirementsBytes {
		return fmt.Errorf("叙事要求超过 %d 字节上限", MaxStoryNarrativeRequirementsBytes)
	}
	return nil
}

// StoryNarrativeRequirements returns the direction the narrator should follow
// and where it came from.
//
// A story that stores its own direction reads only that value: sharing the
// workspace file between stories is exactly the leak the design forbids, so the
// override must not be merged with the default. A story with no override reads
// the workspace default supplied by the caller. The scope is returned rather
// than inferred so the UI can state plainly which text is actually in effect.
func StoryNarrativeRequirements(meta StoryMeta, workspaceDefault string) (string, string) {
	if own := normalizeStoryNarrativeRequirements(meta.NarrativeRequirements); own != "" {
		return own, StoryNarrativeRequirementsScopeStory
	}
	if fallback := normalizeStoryNarrativeRequirements(workspaceDefault); fallback != "" {
		return fallback, StoryNarrativeRequirementsScopeWorkspace
	}
	return "", StoryNarrativeRequirementsScopeNone
}

// NarrativeRequirementsScope names where the effective direction came from.
// The player settings surface shows this instead of implying a per-story freeze
// that does not exist.
const (
	StoryNarrativeRequirementsScopeStory     = "story"
	StoryNarrativeRequirementsScopeWorkspace = "workspace"
	StoryNarrativeRequirementsScopeNone      = "none"
)
