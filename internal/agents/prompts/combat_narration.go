package prompts

import (
	"fmt"
	"strings"
)

// CombatNarrationSystemInstructionInput is the system prompt for one round of
// battle prose.
//
// The rules have already settled the exchange. This request describes it and
// nothing else: it does not roll, does not choose, and does not decide who was
// hurt. §6.6 of the battle design puts the AI after the settlement for exactly
// that reason, and §8 requires a technical failure to surface as a failure
// rather than as a story consequence — so the model is given the settled facts
// and told they are not negotiable.
type CombatNarrationSystemInstructionInput struct {
	Workspace string
	// NarrativeTargetChars is the target length of one round's prose. The design
	// recommends roughly 300–600 characters as an experiment, with important
	// exchanges allowed to run longer and simple ones shorter.
	NarrativeTargetChars int
	// SettledFacts is the exchange as the rules settled it: who acted, in what
	// order, what each action produced, what was spent, and how the combatants
	// stand afterwards. It is the whole factual basis of the passage.
	SettledFacts string
	// SceneContext is where the fight is happening, so the prose does not invent
	// a location or a time of day the story never established.
	SceneContext string
	// CombatStylePrompt is the combat purpose's expression fragment. It governs
	// how the fight is described and nothing else: the settled result, who knows
	// what, and who is still standing all outrank it.
	CombatStylePrompt string
	// OutputLanguage is the locale the player's page is reading in. It decides
	// the language of the round's prose.
	OutputLanguage string
}

// DefaultCombatNarrativeTargetChars is the default length target for one round
// of battle prose. It is deliberately far shorter than a narrative turn: a round
// is one exchange, not a scene.
const DefaultCombatNarrativeTargetChars = 450

const combatNarrationBuiltIn = "You are the narrator of an interactive story, describing one round of a fight that has already been resolved. The exchange is settled. Write what happened."

// BuildCombatNarrationSystemInstruction composes the round narration prompt.
//
// It is deliberately separate from the main narrative prompt. A battle round is
// a bounded passage about one settled exchange: it must not advance the story,
// must not choose for the player, and must not introduce an outcome the rules
// did not produce. The main narrative's long-form requirements and its freedom
// to move the scene on are both wrong here.
func BuildCombatNarrationSystemInstruction(in CombatNarrationSystemInstructionInput) string {
	var sb strings.Builder
	writeLanguageFirst(&sb, in.OutputLanguage, "下方已定的事实", "适用于你写的这段叙述。")
	sb.WriteString(combatNarrationBuiltIn)
	sb.WriteString("\n\n## The exchange as it was settled\n\n")
	sb.WriteString("These are the facts. They are already decided and they are not yours to change: the same exchange resolves the same way every time it is replayed, so an outcome you invent would contradict the battle's own record.\n\n")
	facts := strings.TrimSpace(in.SettledFacts)
	if facts == "" {
		facts = "(no settled facts were supplied)"
	}
	sb.WriteString(facts)
	sb.WriteString("\n")
	if scene := strings.TrimSpace(in.SceneContext); scene != "" {
		sb.WriteString("\n## Where this is happening\n\n")
		sb.WriteString(scene)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## What to write\n")
	sb.WriteString("- Describe this round and only this round: the actions in the order given, what each one produced, and what an onlooker could see. Do not write several exchanges, and do not carry the fight forward to its end.\n")
	sb.WriteString("- The result is fixed. Nobody who was not hurt is hurt, nobody who did not spend is spent, and nobody who is still standing falls. Do not soften a landed blow and do not invent a wound the facts do not report.\n")
	sb.WriteString("- A blow a shield absorbed is a blow the shield took. A cast that was refused produced nothing and cost nothing. Write what actually happened rather than a general impression of violence.\n")
	sb.WriteString("- Show the observable consequences the facts support: a shield thinning, a prepared technique interrupted, a way out opening or closing, a combatant's breathing or footing changing. Do not report bare numbers.\n")
	sb.WriteString("- You may describe what the protagonist perceives and does. Do not decide what the protagonist intends, chooses, or feels about it: that belongs to the player, and the next round has not been chosen yet.\n")
	sb.WriteString("- An opponent speaks, moves and reacts only from what it actually did in the facts. Do not give it knowledge it was not shown to have, and do not have it counter an ability the facts never put into play.\n")
	sb.WriteString("- Do not adjudicate rules, roll dice, announce damage figures, or address the player as a game system. You are describing a scene, not reporting a result table.\n")
	sb.WriteString("- Do not end the fight, and do not narrate a surrender, a flight or a death the facts do not report. An offer to yield is an offer, not an accepted one.\n")
	sb.WriteString("- Write in the story's own voice. The wording of the exchange is yours; the exchange is not.\n")
	if style := strings.TrimSpace(in.CombatStylePrompt); style != "" {
		sb.WriteString("\n## How this story's fights are told\n\n")
		sb.WriteString("The following describes the manner of this story's combat prose. It governs expression only. What actually happened, who could see it and what it cost all outrank it: never bend the settled result to match a manner.\n\n")
		sb.WriteString(style)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Output\n")
	sb.WriteString(playerFacingLanguageLine(in.OutputLanguage, "上方已定的事实") + "\n")
	sb.WriteString("Output only the prose of this round. No headings, no labels, no lists, no JSON, no summary of the numbers, and no explanation of your reasoning.\n")
	writeCombatNarrativeTargetInstruction(&sb, in.NarrativeTargetChars, in.OutputLanguage)
	return sb.String()
}

func writeCombatNarrativeTargetInstruction(sb *strings.Builder, value int, locale string) {
	if value <= 0 {
		value = DefaultCombatNarrativeTargetChars
	}
	fmt.Fprintf(sb, "\n一次正常的交手是%s。短促干脆的一击可以更短，要紧的一击可以更长。不要为凑目标而注水，也不要把一次交手拉成一整场戏。\n", proseLengthTarget(locale, value))
}
