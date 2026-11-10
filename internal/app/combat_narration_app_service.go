package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	agent "ta/agent"
	"ta/agent/providers"
)

// This file turns a settled exchange into prose.
//
// §6.6 of the battle design puts the AI after the rules: the round is settled
// and committed first, then the passage is written from what the settlement
// produced, and both are saved. §8 fixes the failure behaviour — a committed
// result is not rolled back because its expression failed, and a retry of a
// round whose prose was already written is a read rather than a second
// rendering. Three consequences are deliberate here:
//
//   - The narration never re-resolves anything. It is handed the committed
//     outcome and describes it. A model call that fails leaves the settled
//     numbers exactly as they were, which is what makes §8's "不重跑原行动"
//     structural rather than a rule this file has to remember.
//
//   - It is written under the round's own command identity, so a repeated click,
//     a lost response or a recovery re-read finds the passage already committed
//     instead of asking for a differently-worded second version.
//
//   - Its thinking level follows the narrative capability, not the character
//     window's. §8 requires the battle prose to run on the story's real thinking
//     setting — 战斗正文属于叙事调用 — and the character-chat setting defaults to
//     off, so borrowing it here would silently downgrade every fight.

// CombatNarrationRequest asks for one settled round to be described.
type CombatNarrationRequest struct {
	StoryID  string
	BranchID string
	// SessionID and RoundEventID name the committed round to describe. The round
	// is not resolved here and must already be on the journal.
	SessionID    string
	RoundEventID string
	// CommandID is the caller-owned idempotency key. It is the round's own, so a
	// retry of a round whose prose exists is answered from the committed passage.
	CommandID string
	Locale    string
}

// CombatNarrationResult is one committed passage.
type CombatNarrationResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// SessionID and RoundEventID identify the round this prose describes.
	SessionID    string `json:"session_id"`
	RoundEventID string `json:"round_event_id"`
	Round        int    `json:"round"`
	// Narrative is the prose as it was committed. A retry returns the passage
	// already on the journal rather than this attempt's generation.
	Narrative string `json:"narrative"`
	// StyleID names the combat style the passage was written in, so a hot style
	// switch can be seen to leave existing prose alone.
	StyleID string `json:"style_id,omitempty"`
	// EventID is the committed narration record.
	EventID string `json:"event_id,omitempty"`
	// Replayed reports that this answer came from a passage already committed
	// under the same command, so the caller knows no second rendering happened.
	Replayed bool `json:"replayed"`
}

// ErrCombatRoundUnknown means the round this prose would describe is not
// committed on this branch. Prose about a round that does not exist would be
// shown against nothing.
var ErrCombatRoundUnknown = fmt.Errorf("the round this narration describes is not committed on this branch")

// NarrateCombatRound writes one settled round's prose.
//
// The round must already be committed. This is the half of §6.6 that runs after
// the settlement: it reads the outcome the rules produced and asks for a
// description of it, and it never asks the model what happened.
func (a *App) NarrateCombatRound(ctx context.Context, request CombatNarrationRequest) (CombatNarrationResult, error) {
	if a == nil {
		return CombatNarrationResult{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	sessionID := strings.TrimSpace(request.SessionID)
	roundEventID := strings.TrimSpace(request.RoundEventID)
	if storyID == "" || sessionID == "" || roundEventID == "" {
		return CombatNarrationResult{}, fmt.Errorf("a combat narration requires a story, a session and a round record")
	}
	store, runtimeCfg, err := a.interactiveStoreRuntime()
	if err != nil {
		return CombatNarrationResult{}, err
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return CombatNarrationResult{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	commandHash := combatNarrationCommandHash(storyID, branchID, sessionID, roundEventID)

	// A retry must not buy a second model call. The committed passage is the
	// durable answer, so it is consulted before anything is generated: a lost
	// response, a repeated click or a recovery re-read all return the prose the
	// player already has rather than a second version of the same exchange.
	if commandID != "" {
		committed, found, err := store.CombatNarrationForCommand(storyID, branchID, commandID, commandHash)
		if err != nil {
			return CombatNarrationResult{}, err
		}
		if found {
			return combatNarrationResultFrom(storyID, branchID, sessionID, committed, true), nil
		}
	}

	session, outcome, err := committedCombatRound(storyID, branchID, sessionID, roundEventID, store)
	if err != nil {
		return CombatNarrationResult{}, err
	}

	// The style is resolved once, before the call, so a style change committed
	// while the model runs cannot alter the passage already being written. §8
	// requires a hot switch to take effect on the next generation.
	styleFragments := interactiveapp.ResolveStoryStyleFragments(runtimeCfg.DataDir(), storyContext.Meta)
	characterState, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return CombatNarrationResult{}, err
	}
	systemInstruction := prompts.BuildCombatNarrationSystemInstruction(prompts.CombatNarrationSystemInstructionInput{
		Workspace:            runtimeCfg.Workspace,
		NarrativeTargetChars: combatNarrationTargetChars(),
		SettledFacts:         combatNarrationFacts(outcome, session, characterState),
		SceneContext:         combatNarrationScene(storyContext),
		CombatStylePrompt:    styleFragments.Combat,
		OutputLanguage:       request.Locale,
	})

	// §8: the passage is a narrative generation, so it runs at the story's own
	// narrative thinking level. The level comes from the branch's persisted
	// conversation config, which is the same setting the main story runs at.
	if _, err := interactiveapp.ApplyConversationConfig(store, &runtimeCfg, storyID, branchID); err != nil {
		return CombatNarrationResult{}, err
	}
	level := providers.ThinkingLevel(strings.TrimSpace(config.ResolveAgentModel(&runtimeCfg, config.AgentKindInteractiveStory).ThinkingLevel))

	narrative, err := a.generateCombatNarration(ctx, &runtimeCfg, storyID, branchID, roundEventID, systemInstruction, level)
	if err != nil {
		return CombatNarrationResult{}, err
	}
	if strings.TrimSpace(narrative) == "" {
		// An empty passage is not committed. §8 requires a failure to be reported
		// rather than stored, and a stored empty passage would read as a round
		// that had no description when in fact none was written.
		return CombatNarrationResult{}, fmt.Errorf("the combat narration returned an empty passage")
	}

	committed, err := store.AppendCombatNarration(storyID, interactive.AppendCombatNarrationRequest{
		BranchID: branchID, SessionID: sessionID, RoundEventID: roundEventID,
		Narrative: narrative, StyleID: styleFragments.CombatStyleID,
		CommandID: commandID, CommandHash: commandHash,
	})
	if err != nil {
		return CombatNarrationResult{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[combat-narration] passage committed story_id=%s branch_id=%s session_id=%s round=%d chars=%d style_id=%s",
		storyID, branchID, sessionID, committed.Round, len([]rune(committed.Narrative)), committed.StyleID))
	// The prose the caller is shown is read back from what was committed, not
	// taken from this attempt's generation. A concurrent request can lose the
	// race to commit, and the store then returns the record the winner wrote:
	// answering with this attempt's text would show prose the journal does not
	// have, and two clients would disagree about what happened.
	return combatNarrationResultFrom(storyID, branchID, sessionID, committed, false), nil
}

// combatNarrationResultFrom reports a committed passage.
func combatNarrationResultFrom(storyID, branchID, sessionID string, narration interactive.CombatNarration, replayed bool) CombatNarrationResult {
	return CombatNarrationResult{
		StoryID: storyID, BranchID: branchID,
		SessionID:    sessionID,
		RoundEventID: narration.RoundEventID, Round: narration.Round,
		Narrative: narration.Narrative, StyleID: narration.StyleID,
		EventID: narration.EventID, Replayed: replayed,
	}
}

// combatNarrationCommandHash fingerprints the round a passage describes.
//
// It is deliberately not the round's own command hash: that identity belongs to
// resolving the exchange, and reusing it would make a narration request collide
// with the round request it describes. What it does share is the guarantee that
// the same round always fingerprints the same way, so a retry is a read.
func combatNarrationCommandHash(storyID, branchID, sessionID, roundEventID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"denova.combat.narration", storyID, branchID, sessionID, roundEventID,
	}, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// committedCombatRound reads the round a passage will describe.
//
// It reads the committed record rather than re-resolving the exchange, which is
// the whole point of §6.6's ordering: the facts are already settled, and a
// narration that resolved its own exchange would draw again and describe a fight
// the journal does not contain.
//
// The session comes back as of the round's own commit, not from the branch's
// current projection. A battle that has played on has already moved its pools, so
// answering with the latest session would write the passage from numbers the
// described exchange never used — and, worse, would rewrite an earlier round's
// prose every time a later round changed the state. The round's own record is the
// snapshot that belongs to it.
func committedCombatRound(storyID, branchID, sessionID, roundEventID string, store *interactive.Store) (cultivation.CombatSession, cultivation.ExchangeOutcome, error) {
	record, session, found, err := store.CombatRound(storyID, branchID, sessionID, roundEventID)
	if err != nil {
		return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, err
	}
	if !found {
		// Distinguish "no such battle" from "no such round in that battle": the
		// caller reports the first as an unknown battle and the second as an
		// unknown round, and a single message would send the player looking in
		// the wrong place.
		sessions, err := store.CombatSessions(storyID, branchID)
		if err != nil {
			return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, err
		}
		if _, ok := sessions[sessionID]; !ok {
			return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, fmt.Errorf("%w: %s", ErrCombatUnknown, sessionID)
		}
		return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, fmt.Errorf("%w: %s", ErrCombatRoundUnknown, roundEventID)
	}
	return session, record.Outcome, nil
}

// combatNarrationTargetChars is the length target for one round's prose.
//
// The design recommends roughly 300–600 characters as an experiment and leaves
// the main story's own length untouched, so this is deliberately independent of
// the story's narrative reply target. A round is one exchange, not a scene.
func combatNarrationTargetChars() int {
	return prompts.DefaultCombatNarrativeTargetChars
}

// combatNarrationFacts renders the settled exchange as the factual basis of the
// passage.
//
// The technique is named by its definition ID rather than by a display name.
// The wording lives in the interface and may be translated or replaced by a
// world pack, and the state preset already addresses the model with these IDs
// when it describes a character's learned techniques — so naming the technique
// this way is the same contract the story is already written against, rather
// than a second vocabulary invented here.
//
// The passage is player-facing prose, so §四's audience constraint reaches it
// too: the player's own side is described with its real numbers, and an opponent
// only through what the fight put in front of the player — the act that was
// visible, the wound that landed, the effect on their body. An opponent's exact
// pools, burden, technique identifiers and cast costs are not facts this passage
// is allowed to be written from, because prose that states them tells the player
// something the window deliberately withheld.
//
// Everything else is what the rules actually settled. A value the rules never
// established is left out rather than sent as a zero, because a zero the model
// reads as fact becomes a fact in the prose.
func combatNarrationFacts(outcome cultivation.ExchangeOutcome, session cultivation.CombatSession, characters character.State) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Round %d of the fight.\n", outcome.Round)

	if len(outcome.Order) > 0 {
		names := make([]string, 0, len(outcome.Order))
		for _, actorID := range outcome.Order {
			names = append(names, combatNarrationActor(characters, actorID))
		}
		fmt.Fprintf(&sb, "Order of action: %s.\n", strings.Join(names, " → "))
	}

	sb.WriteString("\nWhat each combatant did:\n")
	for _, step := range outcome.Steps {
		sb.WriteString("- ")
		sb.WriteString(combatNarrationStep(step, session, characters))
		sb.WriteString("\n")
	}

	sb.WriteString("\nHow the combatants stand after this round:\n")
	for _, combatant := range session.Combatants {
		sb.WriteString("- ")
		sb.WriteString(combatNarrationCombatant(combatant, characters))
		sb.WriteString("\n")
	}

	if outcome.Ended {
		fmt.Fprintf(&sb, "\nThe fight has ended. The result is: %s.\n", combatNarrationResult(outcome.Result))
	} else {
		sb.WriteString("\nThe fight has not ended. It continues, and the next round has not been chosen yet.\n")
	}
	return sb.String()
}

// combatNarrationStep describes one action and what it produced.
func combatNarrationStep(step cultivation.ExchangeStep, session cultivation.CombatSession, characters character.State) string {
	actor := combatNarrationActor(characters, step.ActorID)
	target := ""
	if strings.TrimSpace(step.TargetID) != "" {
		target = combatNarrationActor(characters, step.TargetID)
	}
	// §四: the passage is player-facing, so an opponent's own business is not
	// among the facts it is written from. The player still sees that something
	// happened; what they do not see is the definition, the price or the amount.
	if !combatNarrationActorIsPlayer(session, step.ActorID) {
		return combatNarrationObservedStep(step, target, characters)
	}

	switch step.Kind {
	case cultivation.ActionRest:
		if step.ManaRecovered > 0 {
			return fmt.Sprintf("%s rested and recovered %d spiritual power.", actor, step.ManaRecovered)
		}
		return fmt.Sprintf("%s rested.", actor)
	case cultivation.ActionEscape:
		if step.Escaped {
			return fmt.Sprintf("%s broke off and got away from the fight.", actor)
		}
		return fmt.Sprintf("%s tried to break off and failed to get away.", actor)
	case cultivation.ActionSurrender:
		if step.Surrendered {
			return fmt.Sprintf("%s offered to yield. The offer has not been answered, so the fight continues.", actor)
		}
		return fmt.Sprintf("%s tried to yield, and nothing came of it.", actor)
	case cultivation.ActionAcceptSurrender:
		if step.Surrendered {
			return fmt.Sprintf("%s accepted %s's surrender, and %s is out of the fight.", actor, target, target)
		}
		return fmt.Sprintf("%s tried to accept a surrender that was not on offer, and nothing changed.", actor)
	case cultivation.ActionUseItem:
		// A dose is described from what the dose settled, and never through the
		// technique branch below. §4.2's 增攻药 takes hold without moving any pool,
		// so a step read only through its ReasonKey would report a real
		// consumption as a refusal — and the story would then write the pill
		// back into the bottle it was taken out of.
		return combatNarrationDoseStep(step, session, actor)
	}

	// A technique. The name is the definition's stable ID, which is how the
	// story's own state already refers to a character's learned techniques.
	technique := "an ordinary attack"
	if skill, ok := cultivation.SkillByID(step.SkillID); ok {
		technique = fmt.Sprintf("the technique %s", skill.ID)
	} else if strings.TrimSpace(step.SkillID) != "" {
		technique = fmt.Sprintf("the technique %s", step.SkillID)
	}
	// §5 makes a variant a way of casting rather than a second technique, so the
	// passage has to say which way. A round that read only the shared identifier
	// would call the burning form's weaker hit the base technique's, and the prose
	// would then describe a blow the rules never settled.
	if step.Variant != "" {
		technique = fmt.Sprintf("the %s form of %s", step.Variant, technique)
	}
	lead := actor
	if target != "" {
		lead = fmt.Sprintf("%s against %s", actor, target)
	}

	if step.ReasonKey != "" {
		switch step.ReasonKey {
		case "reason.preparing":
			return fmt.Sprintf("%s began preparing %s. It has not struck yet: the cost is paid now and the blow lands next round.", actor, technique)
		case "reason.no_target":
			return fmt.Sprintf("%s committed %s with no live target, so it struck nothing. The cost was still paid.", actor, technique)
		default:
			return fmt.Sprintf("%s attempted %s and it was refused: %s. A refused cast produced no effect and spent nothing.", lead, technique, step.ReasonKey)
		}
	}
	if !step.Resolution.Hit {
		if len(step.ExtraTargets) == 0 {
			return fmt.Sprintf("%s used %s and it missed. Nothing was wounded.", lead, technique)
		}
		lines := []string{fmt.Sprintf("%s used %s. The blow on %s missed", actor, technique, target)}
		lines = append(lines, combatNarrationExtraTargets(step.ExtraTargets, characters)...)
		return strings.Join(lines, "; ") + "."
	}
	lines := []string{fmt.Sprintf("%s used %s and it landed", lead, technique)}
	if step.Resolution.ShieldAbsorbed > 0 {
		lines = append(lines, fmt.Sprintf("an existing shield took %d of the blow", step.Resolution.ShieldAbsorbed))
	}
	if step.Resolution.HealthLost > 0 {
		lines = append(lines, fmt.Sprintf("%s really lost %d health", target, step.Resolution.HealthLost))
	}
	if step.BurnApplied {
		lines = append(lines, "the wound set them alight")
	}
	if step.SlowApplied {
		lines = append(lines, "the blow slowed them down")
	}
	if step.Defeated {
		lines = append(lines, fmt.Sprintf("%s was dropped and is out of the fight", target))
	}
	lines = append(lines, combatNarrationExtraTargets(step.ExtraTargets, characters)...)
	return strings.Join(lines, "; ") + "."
}

// combatNarrationExtraTargets words the further combatants a multi-target
// technique was aimed at.
//
// §9 settles the reach from the player's actual choice, so the passage has to
// say what happened to each chosen target. Folding them into one clause would
// let the prose describe a strike the round did not settle — and the player who
// chose two opponents is owed both outcomes, including a miss.
func combatNarrationExtraTargets(strikes []cultivation.ExtraTargetStrike, characters character.State) []string {
	lines := make([]string, 0, len(strikes))
	for _, strike := range strikes {
		target := combatNarrationActor(characters, strike.TargetID)
		if !strike.Resolution.Hit {
			lines = append(lines, fmt.Sprintf("the blow on %s missed", target))
			continue
		}
		line := fmt.Sprintf("it also landed on %s", target)
		if strike.Resolution.HealthLost > 0 {
			line += fmt.Sprintf(", who really lost %d health", strike.Resolution.HealthLost)
		}
		if strike.BurnApplied {
			line += " and was set alight"
		}
		if strike.SlowApplied {
			line += " and was slowed down"
		}
		if strike.Defeated {
			line += " and was dropped"
		}
		lines = append(lines, line)
	}
	return lines
}

// combatNarrationActorIsPlayer reports whether a step was taken by the player's
// own side. An actor the session no longer lists is not the player: the
// projection withholds rather than guesses, because guessing here would be the
// one place an opponent's figures could still reach the prose.
func combatNarrationActorIsPlayer(session cultivation.CombatSession, actorID string) bool {
	combatant := session.CombatantByID(actorID)
	return combatant != nil && combatant.Side == cultivation.SidePlayer
}

// combatNarrationObservedStep words an opponent's action as the player saw it.
//
// The player watched something happen and watched its visible consequence; they
// did not read the definition, the price, or how much was given back. So the
// facts say the act and the wound, and leave out everything the window also
// withholds — otherwise the prose would hand the player what the panel just
// refused to.
func combatNarrationObservedStep(step cultivation.ExchangeStep, target string, characters character.State) string {
	lead := "the opponent"
	if target != "" {
		lead = fmt.Sprintf("the opponent against %s", target)
	}
	switch step.Kind {
	case cultivation.ActionRest:
		return "The opponent paused and did not attack."
	case cultivation.ActionEscape:
		if step.Escaped {
			return "The opponent broke off and got away from the fight."
		}
		return "The opponent tried to break off and did not get away."
	case cultivation.ActionSurrender:
		if step.Surrendered {
			return "The opponent offered to yield. The offer has not been answered, so the fight continues."
		}
		return "The opponent tried to yield, and nothing came of it."
	case cultivation.ActionUseItem:
		if step.ItemApplied {
			return "The opponent took something and swallowed it. What it was and what it did are not visible."
		}
		return "The opponent reached for something and thought better of it."
	}
	if !step.Resolution.Hit {
		return fmt.Sprintf("%s used an art that is not known to you, and it missed. Nothing was wounded.", lead)
	}
	lines := []string{fmt.Sprintf("%s used an art that is not known to you, and it landed", lead)}
	if step.Resolution.HealthLost > 0 {
		lines = append(lines, fmt.Sprintf("%s was wounded", target))
	}
	if step.BurnApplied {
		lines = append(lines, "the wound set them alight")
	}
	if step.SlowApplied {
		lines = append(lines, "the blow slowed them down")
	}
	if step.Defeated {
		lines = append(lines, fmt.Sprintf("%s was dropped and is out of the fight", target))
	}
	// An opponent's reach is visible — the player watched a second companion take
	// a blow — but the amount and the definition stay withheld exactly as they are
	// for the primary strike.
	for _, extra := range step.ExtraTargets {
		extraTarget := combatNarrationActor(characters, extra.TargetID)
		if !extra.Resolution.Hit {
			lines = append(lines, fmt.Sprintf("the blow on %s missed", extraTarget))
			continue
		}
		line := fmt.Sprintf("it also landed on %s", extraTarget)
		if extra.Resolution.HealthLost > 0 {
			line += ", who was wounded"
		}
		if extra.Defeated {
			line += " and was dropped"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "; ") + "."
}

// combatNarrationDoseStep describes one settled dose, taken or refused.
//
// §4.2's three pill kinds do not all move a pool: a 增攻药 leaves health and
// spiritual power exactly where they were and works through an effect that
// outlasts the round. A step therefore cannot be worded from its pools alone,
// and it cannot be worded from its ReasonKey alone either — a boost that took
// hold carries reason.item_boost_applied, which is an account of what the dose
// did rather than of a refusal. Reading either one by itself is how a consumed
// pill gets written back into the bottle, so the two outcomes are separated here
// on ItemApplied, which is the field the rules actually set when a dose was
// spent.
func combatNarrationDoseStep(step cultivation.ExchangeStep, session cultivation.CombatSession, actor string) string {
	if !step.ItemApplied {
		reason := strings.TrimSpace(step.ReasonKey)
		if reason == "" {
			reason = "no effect followed"
		}
		// The refusal is stated with its consequence, because §8's whole point is
		// that a refused dose is not a spent one: the story must not write the
		// pill as consumed, and saying so here is what keeps it from having to
		// infer it.
		return fmt.Sprintf("%s tried to take a dose and it was refused: %s. Nothing was consumed.", actor, reason)
	}
	switch {
	case step.HealthRecovered > 0:
		return fmt.Sprintf("%s took a dose and recovered %d health.", actor, step.HealthRecovered)
	case step.ManaRecovered > 0:
		return fmt.Sprintf("%s took a dose and recovered %d spiritual power.", actor, step.ManaRecovered)
	}
	// A dose that took hold without moving a pool: §4.2's boost. Its real
	// strength and remaining term are read off the effect it left in force, so
	// the prose reports the published budget rather than a number of its own.
	if combatant := session.CombatantByID(step.ActorID); combatant != nil {
		for _, dose := range combatant.ActiveDoses {
			if dose.DefID != step.SkillID {
				continue
			}
			return fmt.Sprintf(
				"%s took a dose. It is in force: %s +%.0f%%, with %d exchanges of its term remaining.",
				actor, dose.Attribute, dose.Value*100, dose.RemainingExchanges,
			)
		}
	}
	return fmt.Sprintf("%s took a dose, and it is in force.", actor)
}

// combatNarrationCombatant reports one combatant's standing.
//
// The player's own side is stated with its real pools, because those are the
// player's own figures. An opponent is stated only as far as the player can see
// them: whether they are still in the fight and how hurt they plainly are. Their
// pools, burden and shield strength are not stated, for the same reason the
// window does not send them.
func combatNarrationCombatant(combatant cultivation.Combatant, characters character.State) string {
	name := combatNarrationActor(characters, combatant.ID)
	standing := combatWindowStanding(combatant)
	switch standing {
	case "fled":
		return fmt.Sprintf("%s has left the fight, still alive.", name)
	case "surrendered":
		return fmt.Sprintf("%s has yielded and is out of the fight.", name)
	case "defeated":
		return fmt.Sprintf("%s is down at zero health and out of the fight. Whether they live is not settled by this.", name)
	}
	if combatant.Side != cultivation.SidePlayer {
		parts := []string{fmt.Sprintf("%s is still standing", name)}
		parts = append(parts, fmt.Sprintf("visibly %s", combatNarrationVisibleCondition(combatant)))
		if combatant.Shield.Amount > 0 {
			parts = append(parts, "something is turning blows aside")
		}
		if combatant.Burn.RemainingTicks > 0 {
			parts = append(parts, "still alight")
		}
		if combatant.Slow.RemainingTargetActions > 0 {
			parts = append(parts, "moving slowly")
		}
		if combatant.OfferText != "" {
			parts = append(parts, "has an outstanding offer to yield that has not been answered")
		}
		return strings.Join(parts, ", ") + "."
	}
	parts := []string{fmt.Sprintf("%s is still standing", name)}
	parts = append(parts, fmt.Sprintf("health %d of %d", combatant.Attributes.Health, combatant.Attributes.HealthMax))
	parts = append(parts, fmt.Sprintf("spiritual power %d of %d", combatant.Attributes.Mana, combatant.Attributes.ManaMax))
	if combatant.Shield.Amount > 0 {
		parts = append(parts, fmt.Sprintf("shield %d", combatant.Shield.Amount))
	}
	if combatant.Burn.RemainingTicks > 0 {
		parts = append(parts, fmt.Sprintf("burning for %d more rounds", combatant.Burn.RemainingTicks))
	}
	if combatant.Slow.RemainingTargetActions > 0 {
		parts = append(parts, "slowed")
	}
	if combatant.Burden.Current > 0 {
		parts = append(parts, fmt.Sprintf("meridian burden %d", combatant.Burden.Current))
	}
	if combatant.OfferText != "" {
		parts = append(parts, "has an outstanding offer to yield that has not been answered")
	}
	return strings.Join(parts, ", ") + "."
}

// combatNarrationVisibleCondition is the coarse band the player can read off an
// opponent at a glance, in the same words the window uses.
func combatNarrationVisibleCondition(combatant cultivation.Combatant) string {
	switch combatWindowVisibleCondition(combatant) {
	case "combatWindow.condition.critical":
		return "close to dropping"
	case "combatWindow.condition.wounded":
		return "badly hurt"
	case "combatWindow.condition.hurt":
		return "marked but standing"
	case "combatWindow.condition.unharmed":
		return "unhurt"
	}
	return "in the fight"
}

// combatNarrationResult words a settled battle result for the facts block.
func combatNarrationResult(result cultivation.CombatResult) string {
	switch result {
	case cultivation.ResultPlayerWon:
		return "the protagonist's side won"
	case cultivation.ResultPlayerLost:
		return "the protagonist's side lost"
	case cultivation.ResultPlayerFled:
		return "the protagonist's side withdrew from the fight"
	case cultivation.ResultEnemyFled:
		return "the opposing side withdrew from the fight"
	case cultivation.ResultTruce:
		return "the two sides came to terms"
	case cultivation.ResultDraw:
		return "the fight reached a draw"
	}
	return string(result)
}

// combatNarrationScene is where the fight is happening.
func combatNarrationScene(storyContext interactive.StoryContext) string {
	scene := interactive.StoryScene(storyContext.Snapshot)
	parts := make([]string, 0, 3)
	if location := strings.TrimSpace(scene.Location); location != "" {
		parts = append(parts, fmt.Sprintf("Location: %s.", location))
	}
	if title := strings.TrimSpace(scene.Title); title != "" {
		parts = append(parts, fmt.Sprintf("Scene: %s.", title))
	}
	if worldTime := characterWorldTime(storyContext); worldTime != "" {
		parts = append(parts, fmt.Sprintf("In-story time: %s.", worldTime))
	}
	// The tail of the current turn is the immediate lead-in, so the passage
	// continues the scene rather than starting a new one. It is context, not an
	// instruction: the settled facts above still decide what happened.
	if narrative := latestTurnNarrative(storyContext.Snapshot); narrative != "" {
		parts = append(parts, fmt.Sprintf("The story immediately before this round reads: %s", narrative))
	}
	return strings.Join(parts, "\n")
}

// combatNarrationActor is the name the story knows an actor by.
//
// A combatant the cast has never introduced keeps its ID, which is honest: the
// player has not been told a name, and the prose must not invent one the story
// never gave. This is the same rule the window follows.
func combatNarrationActor(characters character.State, actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "someone"
	}
	if profile, ok := characters.Profile(actorID); ok {
		if name := strings.TrimSpace(profile.DisplayName()); name != "" {
			return name
		}
	}
	return actorID
}

// generateCombatNarration performs the one-shot model call.
//
// It mirrors the character window's call: a standalone trace, one system
// instruction and one user message, and no tool protocol. A round's prose is not
// a story turn — it must not allocate a turn number, must not run the narrative
// tool protocol, and must not advance the branch head.
func (a *App) generateCombatNarration(ctx context.Context, cfg *config.Config, storyID, branchID, roundEventID, systemInstruction string, level providers.ThinkingLevel) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindInteractiveStory)
	if err != nil {
		return "", fmt.Errorf("resolve combat narration model configuration: %w", err)
	}
	if level != "" {
		modelCfg.ThinkingLevel = level
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "combat", cfg.Workspace,
		"combat_narration", "Battle Round Narration",
		"describe one already-settled round of a fight in an interactive story",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(combatNarrationUserMessage),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindInteractiveStory, messages, nil); err != nil {
		return "", err
	}
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "combat_narration", "round", map[string]any{
		"story_id": storyID, "branch_id": branchID, "round_event_id": roundEventID,
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create combat narration model: %w", err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "combat_narration", "round", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "combat_narration", "round", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate combat narration: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the combat narration model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "combat_narration", "round", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "combat_narration", "round", modelCfg.Model, 0, msg, nil, nil)
	return strings.TrimSpace(msg.Content), nil
}

// combatNarrationUserMessage is the request itself.
//
// It is deliberately language-neutral about output: the passage is written in
// the story's own language, which the story's context and the surrounding prose
// already establish. The instruction to write prose and nothing else lives in
// the system prompt, where the rest of the round's rules are.
const combatNarrationUserMessage = "Describe the round above."
