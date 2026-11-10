package interactive

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
)

// CombatHandoffFragmentLimit is §7's ceiling for a single handoff fragment.
//
// The design fixes it at 8KiB of UTF-8 and requires it to count against the real
// model budget rather than being an aspiration, so the builder enforces it and
// reports what it had to leave out rather than quietly trimming a condition.
const CombatHandoffFragmentLimit = 8 * 1024

// combatHandoffResultSentence words the formal result of a battle.
//
// §2 requires the main story to acknowledge the result that was actually
// settled — a fled enemy stays fled — so this is the rules' own result rather
// than a paraphrase, and an unrecognised result is reported as itself instead of
// being rounded to the nearest one the wording knows.
func combatHandoffResultSentence(result cultivation.CombatResult) string {
	switch result {
	case cultivation.ResultPlayerWon:
		return "The battle ended: the player's side won."
	case cultivation.ResultPlayerLost:
		return "The battle ended: the player's side lost."
	case cultivation.ResultPlayerFled:
		return "The battle ended: the player's side disengaged and left the fight."
	case cultivation.ResultEnemyFled:
		return "The battle ended: the opposing side disengaged and left the fight."
	case cultivation.ResultTruce:
		return "The battle ended: both sides stopped fighting."
	case cultivation.ResultDraw:
		return "The battle ended: neither side prevailed."
	case cultivation.ResultUnresolved:
		return ""
	default:
		return fmt.Sprintf("The battle ended with the recorded result %q.", string(result))
	}
}

// combatHandoffStandingSentence words what happened to one combatant.
//
// The four standings stay apart for the reason the rules keep them apart: a
// yielded opponent is alive and present, and wording them as defeated would have
// the narrator write a corpse. An opponent still on their feet is reported as
// still on their feet, because "the fight is over" does not mean everyone in it
// was beaten.
func combatHandoffStandingSentence(combatant cultivation.Combatant) string {
	name := combatant.ID
	switch combatant.Standing() {
	case cultivation.StandingFled:
		return fmt.Sprintf("%s has left the fight.", name)
	case cultivation.StandingSurrendered:
		return fmt.Sprintf("%s yielded and is alive and present; the fight ended before they were defeated.", name)
	case cultivation.StandingDefeated:
		return fmt.Sprintf("%s was defeated and is out of the fight.", name)
	default:
		return fmt.Sprintf("%s is still on their feet.", name)
	}
}

// CombatHandoffFragment renders the battles a branch has not yet absorbed into
// the model-visible handoff fragment §7 describes.
//
// Three properties are deliberate:
//
//   - It states only what the rules settled. The pools, the standings and the
//     result come from the committed session. Nothing here decides whether the
//     player pursues a fugitive, accepts a surrender, loots a body or helps
//     someone: §5.5 forbids "continue the story" from choosing those for the
//     player, so the fragment names them as open rather than resolving them.
//
//   - It is English, per §7, while the narrative stays in the game's language.
//     The prose the model writes is unaffected; only this instruction is.
//
//   - It is bounded and says so. An over-long handoff drops the oldest battles
//     and reports the drop, rather than truncating a condition mid-sentence and
//     letting the narrator believe it has the whole picture.
func CombatHandoffFragment(handoffs []PendingCombatHandoff) string {
	if len(handoffs) == 0 {
		return ""
	}
	kept := handoffs
	var dropped int
	for {
		// The notice is part of the fragment, so it is reserved for while the
		// battles are being fitted: a reduced handoff that only says what it
		// dropped once it has already overrun the limit has not respected it.
		notice := combatHandoffDropNotice(dropped)
		if len(renderCombatHandoff(kept))+len(notice) <= CombatHandoffFragmentLimit {
			return renderCombatHandoff(kept) + notice
		}
		if len(kept) == 0 {
			// Even the newest battle alone does not fit. §7 forbids trimming a
			// condition and presenting the remainder as complete, so the honest
			// answer is to say the handoff could not be prepared rather than to
			// describe the fight partially and let the narrator believe that is
			// all of it.
			return combatHandoffUnavailableNotice(len(handoffs))
		}
		// Drop the oldest battle first: the most recent fight is the one the
		// player is most likely continuing from.
		kept = kept[1:]
		dropped++
	}
}

// combatHandoffDropNotice says which battles were left out and that they still
// happened, so a narrator reading a reduced handoff does not read it as the whole
// record.
func combatHandoffDropNotice(dropped int) string {
	if dropped == 0 {
		return ""
	}
	return fmt.Sprintf(
		"\n\n%d earlier battle(s) are not summarised here because this handoff is over its size limit. They remain on the record and can be read back; do not assume nothing happened in them.\n",
		dropped,
	)
}

func combatHandoffUnavailableNotice(total int) string {
	return fmt.Sprintf(
		"### A settled battle could not be summarised within the handoff size limit\n\n%d battle(s) happened during play and are on the record, but the most recent one is too large to summarise here. Do not restage any of them and do not restore anyone to their pre-battle health. Read the battle records back before narrating anything about them, and do not assume the fight had no consequences.\n",
		total,
	)
}

func renderCombatHandoff(handoffs []PendingCombatHandoff) string {
	var builder strings.Builder
	builder.WriteString("### Battles that have already been settled and must not be fought again\n\n")
	builder.WriteString("These fights happened during play and their results are formal. Continue from what they settled. Do not restage any of them, do not restore anyone to the health they had before the fight, and do not re-award anything the fight already produced.\n")
	for _, handoff := range handoffs {
		builder.WriteString("\n")
		renderCombatHandoffEntry(&builder, handoff)
	}
	builder.WriteString("\nWhat the player does next about these battles — whether to pursue someone who left, accept or refuse a surrender, search the fallen, help anyone hurt, or leave — is the player's decision and is not settled here. Stop at that choice rather than choosing for them.\n")
	return builder.String()
}

func renderCombatHandoffEntry(builder *strings.Builder, handoff PendingCombatHandoff) {
	session := handoff.Session
	builder.WriteString(fmt.Sprintf("Battle %s", handoff.SessionID))
	if handoff.WorldTime != "" {
		builder.WriteString(fmt.Sprintf(" (at %s)", handoff.WorldTime))
	}
	builder.WriteString(":\n")
	if sentence := combatHandoffResultSentence(session.Result); sentence != "" {
		builder.WriteString("- " + sentence + "\n")
	} else {
		builder.WriteString("- This battle has not reached a result yet; it is still in progress. Do not restage it or move the scene as though it were over.\n")
	}
	for _, combatant := range session.Combatants {
		builder.WriteString(fmt.Sprintf("- Side %s — %s", combatant.Side, combatHandoffStandingSentence(combatant)))
		if pools := combatHandoffPoolSentence(combatant); pools != "" {
			builder.WriteString(" " + pools)
		}
		builder.WriteString("\n")
	}
	if offer := combatHandoffOfferSentence(session); offer != "" {
		builder.WriteString("- " + offer + "\n")
	}
	if handoff.Settled && len(session.Combatants) > 0 {
		builder.WriteString("- The fight is over. The last exchange and the result remain on the record and are not to be rewritten.\n")
	}
}

// combatHandoffPoolSentence reports a combatant's remaining pools.
//
// A battle's pools are what the exchanges have been settling against, so they
// are the numbers the story must continue from: a wound the last round dealt is
// on the battle and not yet on the character's stored value. The player's own
// side is stated exactly, because those are the player's own figures.
//
// An opponent's pools are not stated. §三.2 keeps an enemy's exact backend pools
// off the player's screen, and this fragment is model-shared input: handing the
// model "37/120" would let the prose state a figure the player was never shown.
// What the story needs of an opponent is how hurt they plainly are and how they
// stand, which combatHandoffStandingSentence and the visible band already give.
func combatHandoffPoolSentence(combatant cultivation.Combatant) string {
	if combatant.Side != cultivation.SidePlayer {
		return ""
	}
	attributes := combatant.Attributes
	if attributes.HealthMax <= 0 && attributes.ManaMax <= 0 {
		return ""
	}
	return fmt.Sprintf(
		"Remaining health %d/%d, remaining mana %d/%d.",
		attributes.Health, attributes.HealthMax, attributes.Mana, attributes.ManaMax,
	)
}

// combatHandoffOfferSentence reports an unanswered offer to yield.
//
// §4 lists this as exactly the kind of thing the return must not auto-complete:
// an offer is a proposal, and the fight continues until someone answers it. It
// is named as outstanding so the narrator asks rather than assumes.
func combatHandoffOfferSentence(session cultivation.CombatSession) string {
	for _, combatant := range session.Combatants {
		if offer := strings.TrimSpace(combatant.OfferText); offer != "" && !combatant.Surrendered {
			return fmt.Sprintf(
				"%s has offered terms that have not been answered yet (%q). This offer is still open and the player has not decided it.",
				combatant.ID, offer,
			)
		}
	}
	return ""
}
