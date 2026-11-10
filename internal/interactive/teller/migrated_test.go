package teller

import (
	"strings"
	"testing"
)

// recommendedStyleIDs are the six styles the picker offers, in the design's own
// order. The ids are stable storage keys: a display name may be retranslated but
// a story's saved selection must keep resolving.
var recommendedStyleIDs = []string{
	"source-drama", "straight-comedy", "slice-banter",
	"strange-tales", "hotblooded-combat", "cinematic-shots",
}

// TestTheSixRecommendedStylesExistAndAreDistinct is the assertion the design
// singles out: "do not pass off six different names with one identical prompt as
// done". A name list proves nothing, so this checks the prompt text itself is
// actually different for each style and each purpose.
func TestTheSixRecommendedStylesExistAndAreDistinct(t *testing.T) {
	if len(recommendedStyleIDs) != 6 {
		t.Fatalf("the recommended set is six styles, the list has %d", len(recommendedStyleIDs))
	}

	seenSystem := map[string]string{}
	for _, id := range recommendedStyleIDs {
		style, ok := builtinTellers[id]
		if !ok {
			t.Fatalf("recommended style %q is missing from the builtin set", id)
		}
		if style.Name == "" || style.NameEN == "" || style.Description == "" || style.DescriptionEN == "" {
			t.Fatalf("style %q is missing a zh or en display string: %+v", id, style)
		}
		if style.Group != groupRecommended {
			t.Fatalf("style %q is in group %q, so the picker would not show it as a "+
				"recommended style", id, style.Group)
		}
		// The internal prompt is Chinese: the project's language convention keeps
		// prompt text in the game's language and does not duplicate it in English.
		for _, slot := range style.Slots {
			if !containsHan(slot.Content) {
				t.Fatalf("style %q slot %q has no Chinese in the internal prompt; "+
					"prompts follow the game language and are not injected twice", id, slot.ID)
			}
		}

		// All four purposes must be carried by the style itself.
		targets := map[string]bool{}
		for _, slot := range style.Slots {
			if strings.TrimSpace(slot.Content) == "" {
				t.Fatalf("style %q slot %q is empty", id, slot.ID)
			}
			targets[slot.Target] = true
		}
		for _, target := range []string{TargetSystem, TargetTurnContext, TargetDialogue, TargetCombat} {
			if !targets[target] {
				t.Fatalf("style %q has no %s fragment, so that purpose would silently "+
					"reuse another style's voice", id, target)
			}
		}

		// Two styles sharing one main voice is the failure this guards.
		voice := style.PromptForTargets(TargetSystem)
		if other, dup := seenSystem[voice]; dup {
			t.Fatalf("styles %q and %q share an identical main prompt", other, id)
		}
		seenSystem[voice] = id
	}
}

// TestRecommendedStylesAreSeededButLegacyIDsAreNot pins the two-part recovery
// promise. The six are written to disk on first use, so the picker lists exactly
// them. The six ids they absorbed are still resolvable — a story that saved one
// keeps loading — but have no seeded file, so they cannot quietly reappear in
// the picker and turn six names back into twelve.
func TestRecommendedStylesAreSeededButLegacyIDsAreNot(t *testing.T) {
	for _, id := range recommendedStyleIDs {
		if _, ok := seededTellers[id]; !ok {
			t.Fatalf("recommended style %q is not seeded, so the picker would not show it", id)
		}
	}
	legacyIDs := []string{
		"marketplace-light", "flippant-banter", "deadpan-humor",
		"restrained-hard", "intrigue-ensemble", "dialogue-piece",
	}
	if len(legacyIDs) != len(legacyStyleAliases) {
		t.Fatalf("the alias table has %d entries, the test names %d",
			len(legacyStyleAliases), len(legacyIDs))
	}
	for _, id := range legacyIDs {
		if _, ok := seededTellers[id]; ok {
			t.Fatalf("merged-away id %q is seeded, so the picker would show twelve "+
				"names for six styles again", id)
		}
		resolved, ok := builtinTellers[id]
		if !ok {
			t.Fatalf("merged-away id %q no longer resolves, so a story that saved it "+
				"cannot load", id)
		}
		target := legacyStyleAliases[id].Target
		if resolved.Group != builtinTellers[target].Group {
			t.Fatalf("merged-away id %q did not inherit %q's group", id, target)
		}
		if resolved.Name == "" || resolved.NameEN == "" || resolved.Description == "" || resolved.DescriptionEN == "" {
			t.Fatalf("merged-away id %q lost its display strings: %+v", id, resolved)
		}
		if got := resolved.PromptForTargets(TargetsForPurpose(PurposeMain)...); got == "" {
			t.Fatalf("merged-away id %q assembled no main narrative", id)
		}
	}
}

// TestMigratedStylesCarryNoOldWorldOrSettlementRules pins the exclusion list.
// The old library's files bound prices, currencies, a calendar, forced cast
// rotation and per-turn settlement into the style. Re-importing any of it would
// let a style change what happens rather than only how it is told.
func TestMigratedStylesCarryNoOldWorldOrSettlementRules(t *testing.T) {
	forbidden := []string{
		// Currency and prices.
		"灵石", "灵砂", "月俸", "物价",
		// Calendar and timestamps.
		"越历", "时间戳",
		// Forced cast rotation and per-turn settlement.
		"轮换", "必须换人", "每回合必须",
		// The old world premise.
		"修仙世界", "主角年岁浅",
	}
	for _, id := range recommendedStyleIDs {
		style := builtinTellers[id]
		for _, slot := range style.Slots {
			for _, banned := range forbidden {
				if strings.Contains(slot.Content, banned) {
					t.Fatalf("style %q slot %q still carries %q, which belongs to a "+
						"world pack or gameplay module, not to a narrative style",
						id, slot.ID, banned)
				}
			}
		}
	}
}

// TestMigratedStylesDropOldLengthAndHardRuleEnforcement records the other half
// of the exclusion: the old files enforced per-style word bands and a regex
// rule engine. Neither is migrated. The definitions must carry no style rules
// at all, so no style can retry, delete or rewrite prose on its own.
func TestMigratedStylesDropOldLengthAndHardRuleEnforcement(t *testing.T) {
	for _, id := range recommendedStyleIDs {
		style := builtinTellers[id]
		if len(style.StyleRules) != 0 {
			t.Fatalf("style %q carries %d style rules; the old hard-rule engine is "+
				"not migrated", id, len(style.StyleRules))
		}
		if len(style.StyleRefs) != 0 {
			t.Fatalf("style %q carries style refs %v", id, style.StyleRefs)
		}
	}
}

// TestMigratedStylesAreThemeNeutral keeps the six usable outside the setting
// they were written in. The acceptance plan requires all six in a
// non-cultivation world, and a prompt that hardcodes the old premise would
// inject it into a detective story.
func TestMigratedStylesAreThemeNeutral(t *testing.T) {
	// Setting-specific vocabulary the old files used as their default world.
	// 灵力 is deliberately not on this list: the approved candidate uses it in a
	// prohibition ("do not add new 灵力"), which forbids the setting's numbers
	// rather than assuming them.
	banned := []string{"修仙", "灵石", "宗门", "仙人", "弟子", "灵根"}
	for _, id := range recommendedStyleIDs {
		style := builtinTellers[id]
		prompt := style.PromptForTargets(TargetSystem, TargetTurnContext, TargetDialogue, TargetCombat)
		for _, term := range banned {
			if strings.Contains(prompt, term) {
				t.Fatalf("style %q hardcodes the setting term %q in its prompt; the "+
					"six must work in any world", id, term)
			}
		}
	}
}

// TestPurposesAssembleFromTheirOwnTargets pins the assembly rule: the three
// purposes take lean fragments rather than stacking three whole system prompts
// into one request. It also pins the fallback direction — a purpose with no
// fragment of its own inherits the main voice instead of contributing nothing.
func TestPurposesAssembleFromTheirOwnTargets(t *testing.T) {
	style := builtinTellers["source-drama"]

	main := style.PromptForTargets(TargetsForPurpose(PurposeMain)...)
	dialogue := style.PromptForTargets(TargetsForPurpose(PurposeDialogue)...)
	combat := style.PromptForTargets(TargetsForPurpose(PurposeCombat)...)

	if main == "" || dialogue == "" || combat == "" {
		t.Fatalf("a purpose assembled nothing: main=%d dialogue=%d combat=%d",
			len(main), len(dialogue), len(combat))
	}
	if dialogue == main || combat == main || dialogue == combat {
		t.Fatal("the three purposes assembled the same fragment")
	}
	// A purpose must not drag in the whole main narrative.
	if strings.Contains(dialogue, "## 叙事声口") || strings.Contains(combat, "## 叙事声口") {
		t.Fatalf("a purpose override pulled in the main narrative voice:\ndialogue=%s\ncombat=%s", dialogue, combat)
	}

	// The four pre-migration styles carry no dialogue or combat fragment, so
	// those purposes must yield nothing rather than a wrong fragment. The caller
	// is what falls back to the main voice.
	legacy := builtinTellers["rhythm"]
	if got := legacy.PromptForTargets(TargetsForPurpose(PurposeDialogue)...); got != "" {
		t.Fatalf("a pre-migration style produced a dialogue fragment: %q", got)
	}
	if got := legacy.PromptForTargets(TargetsForPurpose(PurposeCombat)...); got != "" {
		t.Fatalf("a pre-migration style produced a combat fragment: %q", got)
	}
	if got := legacy.PromptForTargets(TargetsForPurpose(PurposeMain)...); got == "" {
		t.Fatal("a pre-migration style produced no main narrative")
	}
}

// TestBuiltinSetKeepsTheFourNatives guards the recovery path. Existing stories
// store a style id; removing a builtin id would leave those stories unable to
// resolve their own saved selection.
func TestBuiltinSetKeepsTheFourNatives(t *testing.T) {
	for _, id := range []string{"rhythm", "classic", "screenwriter", "grimdark"} {
		style, ok := builtinTellers[id]
		if !ok {
			t.Fatalf("pre-migration style %q was dropped from the builtin set", id)
		}
		if !isBuiltinID(id) {
			t.Fatalf("style %q is no longer recognised as a builtin", id)
		}
		if style.NameEN == "" || style.DescriptionEN == "" {
			t.Fatalf("style %q has no English display text", id)
		}
	}
	// 4 natives + 6 recommended + 6 merged-away aliases.
	if len(builtinTellers) != 16 {
		t.Fatalf("the builtin set has %d styles, want 6 recommended + 4 native + 6 aliases", len(builtinTellers))
	}
	if len(seededTellers) != 10 {
		t.Fatalf("the seeded set has %d styles, want 6 recommended + 4 native", len(seededTellers))
	}
}

// containsHan reports whether a string contains CJK ideographs. It is a test
// helper rather than a product function: the rule it enforces is about prompt
// authoring, not about runtime behaviour.
func containsHan(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}
