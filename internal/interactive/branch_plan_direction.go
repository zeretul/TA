package interactive

import (
	"fmt"
	"strings"
)

// adoptedDirectionHeading is appended when the current plan has no section that
// naturally holds a player-stated direction.
//
// It is a new H2 rather than a note buried in prose because a plan is a modular
// document: the model reads it as sections, and a direction the player
// explicitly adopted has to be as visible to the next turn as any other part of
// the plan.
const adoptedDirectionHeading = "Player-adopted directions"

// directionSectionHints are the headings that already mean "where this could
// go next", most specific first.
//
// A plan usually has more than one forward-looking section — a long-term
// direction and a near-term beat list are both about the future. A direction the
// player adopted right now belongs in the nearer one, because that is where the
// next turn looks, so the two tiers are searched in order rather than taking
// whichever heading happens to come first in the document.
var (
	nearTermDirectionHints = []string{
		"near-term", "near term", "next", "opportunit", "branch", "possibilit",
		"deviation", "recovery", "beat",
	}
	generalDirectionHints = []string{"direction", "replanning", "arc", "phase", "episode"}
)

// AddBranchPlanDirection records one player-accepted direction in a plan.
//
// This is the only place the assistant's advice can become part of the story's
// forward intent, and it is deliberately an addition rather than a replacement:
// the plan the story's own turns have been maintaining stays intact, and the
// player's stated wish is appended where the model will read it.
//
// 自制-剧情助手与选项分析 requires the direction to be expressed as an intention,
// never as an event: it says where the story should look, not what has happened.
// The caller is responsible for that wording; this function only places it.
//
// Re-adding a direction that is already present returns the plan unchanged, so a
// retried request whose response was lost cannot write the same line twice.
func AddBranchPlanDirection(markdown, direction string) string {
	direction = strings.Join(strings.Fields(strings.TrimSpace(direction)), " ")
	if direction == "" {
		return markdown
	}
	draft := normalizeBranchPlanMarkdown(markdown)
	if draft == "" {
		return markdown
	}
	sections, err := parseBranchPlanSections(draft)
	if err != nil {
		// A plan without a usable modular structure cannot take an additive edit.
		// Returning it untouched is the honest answer: the caller still has the
		// direction, and the plan is not silently rewritten into something the
		// creator did not author.
		return markdown
	}

	if key, ok := branchPlanDirectionSection(sections); ok {
		section := sections[key]
		body := draft[section.bodyStart:section.bodyEnd]
		if branchPlanBodyMentionsDirection(body, direction) {
			return draft
		}
		return appendBranchPlanBullet(draft, section, direction)
	}

	if branchPlanBodyMentionsDirection(draft, direction) {
		return draft
	}
	addition := "\n## " + adoptedDirectionHeading + "\n\n- " + direction + "\n"
	candidate := normalizeBranchPlanMarkdown(draft + addition)
	if err := validateEditableBranchPlanMarkdown(candidate); err != nil {
		return markdown
	}
	return candidate
}

// branchPlanDirectionSection picks the existing section a direction belongs in.
//
// The choice is deterministic and stable: the earliest section, in document
// order, whose heading reads as a near-term direction; failing that, the earliest
// long-term one. If the plan has neither the caller appends a module rather than
// guessing at a meaning.
func branchPlanDirectionSection(sections map[string]branchPlanSection) (string, bool) {
	for _, hints := range [][]string{nearTermDirectionHints, generalDirectionHints} {
		if key, ok := branchPlanEarliestSection(sections, hints); ok {
			return key, true
		}
	}
	return "", false
}

func branchPlanEarliestSection(sections map[string]branchPlanSection, hints []string) (string, bool) {
	bestKey, bestPosition := "", -1
	for key, section := range sections {
		if !branchPlanHeadingLooksForward(section.heading, hints) {
			continue
		}
		if bestPosition < 0 || section.bodyStart < bestPosition {
			bestKey, bestPosition = key, section.bodyStart
		}
	}
	return bestKey, bestPosition >= 0
}

func branchPlanHeadingLooksForward(heading string, hints []string) bool {
	lower := strings.ToLower(normalizePlanSectionHeading(heading))
	for _, hint := range hints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// appendBranchPlanBullet adds one bullet to the end of a section's body, keeping
// the blank line that separates it from the following H2.
func appendBranchPlanBullet(markdown string, section branchPlanSection, direction string) string {
	body := strings.TrimRight(markdown[section.bodyStart:section.bodyEnd], "\n")
	body = strings.TrimRight(body, " \t")
	replacement := body + "\n- " + direction + "\n\n"
	candidate := normalizeBranchPlanMarkdown(markdown[:section.bodyStart] + replacement + markdown[section.bodyEnd:])
	if err := validateEditableBranchPlanMarkdown(candidate); err != nil {
		return markdown
	}
	return candidate
}

// branchPlanBodyMentionsDirection reports whether the same direction is already
// recorded. Comparison ignores spacing and case so a retry that reflowed the
// text does not add a second copy.
func branchPlanBodyMentionsDirection(body, direction string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(body), " "))
	needle := strings.ToLower(strings.Join(strings.Fields(direction), " "))
	if needle == "" {
		return false
	}
	return strings.Contains(normalized, needle)
}

// StoryAssistantDirectionIsRecorded reports whether a direction is already part
// of the plan, which lets the caller report a retry as an unchanged result
// rather than a second write.
func StoryAssistantDirectionIsRecorded(plan BranchPlan, direction string) bool {
	direction = strings.Join(strings.Fields(strings.TrimSpace(direction)), " ")
	if direction == "" {
		return false
	}
	return branchPlanBodyMentionsDirection(normalizeBranchPlanMarkdown(plan.Markdown), direction)
}

// ValidateStoryAssistantDirection keeps the adopted wording inside the bounds a
// plan line can carry. It is checked at the API boundary, not only here, so a
// caller that never reaches the plan still gets a useful refusal.
//
// The line break is rejected rather than collapsed: a direction is a single
// plan bullet, and quietly joining a pasted heading onto it would let the
// player's text become plan structure they never wrote.
func ValidateStoryAssistantDirection(direction string) error {
	if strings.ContainsAny(direction, "\r\n") {
		return fmt.Errorf("adopted direction must be a single line")
	}
	direction = strings.Join(strings.Fields(strings.TrimSpace(direction)), " ")
	if direction == "" {
		return fmt.Errorf("adopted direction must be non-empty")
	}
	if len([]rune(direction)) > 600 {
		return fmt.Errorf("adopted direction must be at most 600 characters")
	}
	return nil
}
