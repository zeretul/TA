package interactive

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// This file makes the battle exchange loop reachable during play.
//
// The engine in internal/cultivation/combat_session.go settles an exchange from
// real state and returns the settled result; nothing there reads or writes the
// story. This is the other half: it builds a battle from the actors' own stored
// numbers, commits the settled outcome as a side record on the same journal as
// everything else, and writes the wounds and the expenditure back through the
// story's own atomic commit path.
//
// Three properties are deliberate:
//
//   - There is no second world. A battle is a side record on the canonical
//     journal, rebuilt from it on replay, exactly as character memory is. The
//     window design's §8 forbids a parallel store, and a battle is the most
//     tempting place to build one.
//
//   - The draws are derived from the command identity, not from a random source.
//     §七.2 requires that a refresh or a retry re-use the draws the exchange
//     already used, so a player cannot re-roll a bad round by clicking again.
//     Deriving them from CommandID makes that structural.
//
//   - A side record creates no Turn and consumes no turn number. A battle is
//     fought in its own window; the main story learns about it when the player
//     chooses to continue, which is a separate, explicit act.

// StateOpSourceCombatSettlement marks an Actor State operation the battle rules
// produced.
//
// It is its own source kind so a reader of the journal can tell a wound a battle
// dealt from a wound the narrative described, and so a re-settlement can discard
// its own previous writes without touching anything else.
const StateOpSourceCombatSettlement = "combat_settlement"

func battleRoundCommitHash(requestHash string, intent PlayerCombatIntent) string {
	if intent == "" {
		intent = PlayerCombatIntentSpare
	}
	digest := sha256.Sum256([]byte("denova.combat.player-intent\x00" + strings.TrimSpace(requestHash) + "\x00" + string(intent)))
	return fmt.Sprintf("sha256:%x", digest)
}

// CombatRecordEvent is a side record on the canonical story journal. It carries
// one battle's settled state — every combatant's real pools, lasting effects and
// standing, plus the last exchange's outcome — without creating a story Turn.
type CombatRecordEvent struct {
	EnemyDecision *EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	V             int                   `json:"v"`
	Type          string                `json:"type"`
	ID            string                `json:"id"`
	ParentID      string                `json:"parent_id"`
	BranchID      string                `json:"branch_id"`
	Ts            string                `json:"ts"`
	// TurnID is the main-story turn this battle belongs to. It is empty for a
	// battle fought entirely in the window, which deliberately does not advance
	// the story.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key this record was written
	// under, and CommandHash fingerprints the content behind it. Together they
	// let a retried commit find what the first attempt settled instead of
	// resolving the exchange a second time.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// WorldTime is the in-fiction time the exchange happened at. The base
	// exchange is 20 seconds; the player's real waiting costs nothing.
	WorldTime string `json:"world_time,omitempty"`
	// Session is the whole battle after the exchange: §七.1 requires the window
	// to be able to show both sides' numbers, the round order and what each
	// action produced, and it has to survive the window being closed and
	// reopened.
	Session cultivation.CombatSession `json:"session"`
	// Outcome is the exchange this record settled, kept so a reopened window can
	// show what just happened without re-deriving it from the session.
	Outcome cultivation.ExchangeOutcome `json:"outcome"`
	// LethalIntents is frozen on the opening record and repeated on later rounds
	// so replay can decide deaths without consulting mutable scene state.
	LethalIntents   []CombatLethalIntent `json:"lethal_intents,omitempty"`
	Fatalities      []CombatFatality     `json:"fatalities,omitempty"`
	TerminalOutcome *TerminalOutcome     `json:"terminal_outcome,omitempty"`
}

// CombatRecordResult is what a committed battle record reports back.
type CombatRecordResult struct {
	EnemyDecision *EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	EventID       string
	SessionID     string
	Session       cultivation.CombatSession
	Outcome       cultivation.ExchangeOutcome
	// SettlementOps are the Actor State writes this commit produced, so the
	// caller can report what moved without re-deriving it.
	SettlementOps   []ActorStateOp
	Fatalities      []CombatFatality
	TerminalOutcome *TerminalOutcome
}

// ErrCombatCommandConflict means a command ID was reused for a different
// exchange. Refusing is the only honest answer: accepting would either lose the
// first exchange or record the second one under a false identity, and either
// way the battle's history would be a lie.
var ErrCombatCommandConflict = errors.New("combat command identity conflict")

// ErrCombatTechniqueNotLearned means a round declared a technique its actor has
// not learned. It is separate from the engine's own refusals because it is not a
// rules outcome — no exchange was settled — but a rejection of the request
// itself, the same way a caller cannot name a combatant who is not in the fight.
var ErrCombatTechniqueNotLearned = errors.New("combat technique not learned")

// ErrCombatActionRejected means the declared player action failed a rule check.
// The store returns it before committing so the player can correct the request.
var ErrCombatActionRejected = errors.New("combat action rejected")

// ErrCombatNarrationUnattached means the passage describes a round that is not
// committed on this branch. Committing it would attach prose to nothing, and the
// window would have no exchange to show it against.
var ErrCombatNarrationUnattached = errors.New("combat narration describes no committed round")

// ErrCombatNarrationEmpty means the model returned no prose. It is reported
// rather than stored: §8 requires a technical failure to be visible as a
// failure, and an empty passage in the journal would read as a round that was
// described with nothing.
var ErrCombatNarrationEmpty = errors.New("combat narration is empty")

// CombatantSpec names one battle participant and the story-side facts about them
// that the numbers do not carry.
//
// RouteExists and Retreat are supplied by the caller because they are facts
// about the scene and the opponent's disposition, not about their attributes:
// §14.2 forbids retreating without a real way out, and §七.5 requires the
// death-fight and mindless types to come from a real declared source rather than
// being assumed for everyone.
type CombatantSpec struct {
	ActorID string
	Side    cultivation.CombatSide
	// RouteExists reports whether this combatant really has a way out of this
	// position.
	RouteExists bool
	// Retreat is the opponent's declared evaluation template. RetreatNone is the
	// honest value for anyone the design gives no template.
	Retreat cultivation.RetreatTemplate
}

// combatSessionFromState builds a battle from the participants' own stored
// numbers.
//
// Every number comes from the actor's own state through the same reader the
// panel and the turn settlement use, so a battle cannot be fought at numbers the
// character does not have. The learned technique list is read the same way: an
// opponent uses only what it actually learned, at the version it learned, which
// is what makes §14.2's "只使用本人已学、可操控、资源足够的手段" structural rather
// than a convention the caller has to respect.
//
// A participant the rules do not track is refused rather than silently given
// mortal numbers. §七.1 requires the window to show real attributes for both
// sides, and inventing a row of the stage table for an untracked bystander would
// put numbers in front of the player that no rule produced.
func combatSessionFromState(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	sessionID string,
	specs []CombatantSpec,
) (cultivation.CombatSession, error) {
	if len(specs) == 0 {
		return cultivation.CombatSession{}, fmt.Errorf("一场战斗至少需要一名参与者 / a battle needs at least one combatant")
	}
	session := cultivation.CombatSession{
		ID:           strings.TrimSpace(sessionID),
		Round:        1,
		Combatants:   make([]cultivation.Combatant, 0, len(specs)),
		Preparations: map[string]cultivation.Preparation{},
	}
	seen := make(map[string]bool, len(specs))
	// The registry is the story's own: its published definitions plus whatever it
	// generated. Building it here rather than reading a process-global is what §6
	// asks for — a definition generated in one game is not visible in another, and
	// a branch sees exactly what it inherited.
	registry := storySkillRegistryFromState(system.CultivationRules,
		state)
	for _, spec := range specs {
		actorID := normalizeStatePanelActorID(spec.ActorID)
		if actorID == "" {
			return cultivation.CombatSession{}, fmt.Errorf("参战者缺少 actor_id / a combatant needs an actor id")
		}
		if seen[actorID] {
			// Two entries for one actor would let the same character be shot at
			// twice and act twice, which is not a battle the rules can settle.
			return cultivation.CombatSession{}, fmt.Errorf("参战者重复: %s / duplicate combatant", actorID)
		}
		seen[actorID] = true
		switch spec.Side {
		case cultivation.SidePlayer, cultivation.SideEnemy:
		default:
			return cultivation.CombatSession{}, fmt.Errorf("参战者阵营无效: %q / invalid combatant side", spec.Side)
		}

		templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
		if !found {
			return cultivation.CombatSession{}, fmt.Errorf("参战者 %s 没有可解析的人物模板 / combatant has no resolvable actor template", actorID)
		}
		template := actorStateTemplateByID(system, templateID)
		if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
			return cultivation.CombatSession{}, fmt.Errorf("参战者 %s 不受修炼数值规则跟踪 / combatant is not tracked by the cultivation rules", actorID)
		}

		combatant, err := combatantFromState(system.CultivationRules,
			state, registry, actorID, spec)
		if err != nil {
			return cultivation.CombatSession{}, err
		}
		session.Combatants = append(session.Combatants, combatant)
	}
	return session, nil
}

// combatantFromState reads one participant's live battle state.
func combatantFromState(cultivationRules *cultivation.Rules, state map[string]any,
	registry *cultivation.SkillRegistry,
	actorID string,
	spec CombatantSpec,
) (cultivation.Combatant, error) {
	character, err := xiuxianCharacterFromState(cultivationRules,
		state, actorID)
	if err != nil {
		return cultivation.Combatant{}, err
	}
	// A pool that has never been written means no wound has been recorded, so
	// the character enters the fight at full. This mirrors the turn settlement
	// exactly: the rules never read an unwritten pool as zero, because zero is
	// the one value that means "already down".
	attributes := character.Attributes()
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldHealth)); !ok {
		character.Health = attributes.HealthMax
	}
	if _, ok := actorStateNumber(actorStateFieldValue(state, actorID, xiuxianFieldMana)); !ok {
		character.Mana = attributes.ManaMax
	}

	combatant := cultivation.Combatant{
		ID:          actorID,
		Side:        spec.Side,
		Cultivation: character.Cultivation,
		Attributes:  character.Attributes(),
		Burden:      character.BurdenState(),
		Retreat:     spec.Retreat,
		RouteExists: spec.RouteExists,
		// The learned set is resolved at the version the character learned, so an
		// opponent's technique is the one they actually hold.
		Skills: learnedSkillsFromState(state, actorID, registry).KnownDefinitions(registry),
		// Whether the focus a technique needs is really in hand is read from the
		// same equipment entry the panel reads, so a technique requiring one is
		// refused in battle exactly as it is outside it.
		HasImplement: xiuxianHasMainImplement(state, actorID),
		// The real cost of driving the implement, from its registered definition.
		// 数值规则 line 128 makes this real: a plain attack with a treasure in hand
		// is not free merely because it used the plain-attack action.
		OperatingManaCost: xiuxianImplementOperatingCost(cultivationRules,
			state, actorID),
		// The character's own sourced extras — the technique specialisation and the
		// technique grade — are carried so a dose taken mid-fight aggregates with
		// them in ONE pass. The equipped copies are NOT in this list: they travel as
		// live positions below, because §2 lets a spare replace one mid-fight and a
		// folded copy would keep paying the item it replaced. The deployed formation
		// is excluded for the same reason: §4.3 lets failed upkeep end it mid-fight,
		// so it too travels as live state.
		Modifiers: xiuxianIntrinsicModifiers(character.EffectiveModifiers()),
		// What is really in each of the five positions, as live state.
		Equipment: xiuxianEquippedCopies(cultivationRules,
			state, actorID),
		// What is really in the pack. It contributes nothing while it sits there —
		// §1's 不能因为放在包里就加属性 — and exists so a mid-fight swap can name one.
		Spares: xiuxianEquipmentSpares(cultivationRules,
			state, actorID),
		// The formation as a state with a real upkeep, so the battle can end it when
		// the operator can no longer pay. Its bonus enters through recompute.
		Formation: xiuxianFormationState(cultivationRules,
			state, actorID),
		// What the character really carries, priced from the catalogue. Only pills
		// become doses; an implement recorded under 重要物品 stays out of the
		// numbers, which is §10's 背包法器不全加攻.
		Carried: xiuxianCarriedDoses(cultivationRules,
			state, actorID),
		// A circulating method that reduces casting cost applies to this
		// combatant's own casts. It is read from the same method record the
		// settlement prices, so 节息诀 lowers the cost here exactly as the panel
		// says it does.
		ManaReduction:   character.ManaReduction(),
		StrainReduction: character.StrainReduction(),
	}
	return combatant, nil
}

// combatDrawSource derives every random draw in one exchange from the command
// identity that authorised it.
//
// §七.2 requires the randomness, the costs and the preparation to be bound to
// the command and the commit identity so that a refresh or a retry re-uses the
// draws already used. Deriving each draw from a digest of the command, the
// session, the round and the participants means the same exchange resolves to
// the same numbers every time it is replayed, and a second click cannot fish for
// a better round.
func combatDrawSource(commandID, sessionID string, round int) cultivation.DrawSource {
	return func(purpose, actorID, targetID string) float64 {
		digest := sha256.Sum256([]byte(strings.Join([]string{
			"denova.combat.draw", commandID, sessionID, strconv.Itoa(round), purpose, actorID, targetID,
		}, "\x00")))
		// The top 53 bits of the digest are exactly what a float64 mantissa holds
		// without rounding, so the result is a uniform fraction in [0,1).
		return float64(binary.BigEndian.Uint64(digest[:8])>>11) / float64(uint64(1)<<53)
	}
}

// ResolveCombatExchange settles one exchange against the real state of a story.
//
// It is the single entry point the window uses: it reads the participants from
// their own stored numbers, resolves the declared actions against the draws the
// command identity fixes, and returns the settled session and outcome. Nothing
// is committed here — the caller commits through AppendCombatRecord, so a
// resolved-but-uncommitted exchange can be abandoned or replayed.
func ResolveCombatExchange(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	sessionID, commandID string,
	specs []CombatantSpec,
	previous *cultivation.CombatSession,
	actions map[string]cultivation.CombatAction,
) (cultivation.CombatSession, cultivation.ExchangeOutcome, error) {
	session := previous
	if session == nil {
		built, err := combatSessionFromState(state, system, sessionID, specs)
		if err != nil {
			return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, err
		}
		session = &built
	} else if session.Ended {
		// A battle that has already reached a result is not re-fought. §七.8
		// requires a settled outcome to stand, so reopening the window shows the
		// result rather than offering another round.
		return *session, cultivation.ExchangeOutcome{Round: session.Round, Ended: true, Result: session.Result}, nil
	}
	// A declared technique must be one the combatant has actually learned. The
	// registry holds every definition in the game, so looking a technique up
	// there proves only that it exists — §6 of the casting design keeps 秘籍存在、
	// 技能被登记、人物已学 as three separate facts, and this is the point where a
	// caller's declaration meets the session's real learned sets. Checking here
	// rather than in the engine keeps the rules engine's own contract intact: it
	// resolves the session it is handed, and the store is what decides whether
	// the declaration was legal.
	if err := validateDeclaredTechniques(*session, actions); err != nil {
		return cultivation.CombatSession{}, cultivation.ExchangeOutcome{}, err
	}
	actions = resolveDeclaredTechniques(*session, actions)
	next, outcome := cultivation.ResolveExchange(*session, actions, combatDrawSource(commandID, session.ID, session.Round))
	return next, outcome, nil
}

// resolveDeclaredTechniques replaces a bare generated name with the definition the
// story registered.
//
// A declaration may carry only a technique's identity, because the caller that
// translated it had no story in scope and could not resolve a definition this
// story generated. The round is settled from the learned definition regardless:
// the declared numbers are never the ones used, which is what keeps a client from
// re-pricing a technique by sending different values.
func resolveDeclaredTechniques(session cultivation.CombatSession, actions map[string]cultivation.CombatAction) map[string]cultivation.CombatAction {
	if len(actions) == 0 {
		return actions
	}
	resolved := make(map[string]cultivation.CombatAction, len(actions))
	for actorID, action := range actions {
		if action.Kind == cultivation.ActionTechnique {
			if actor := session.CombatantByID(normalizeStatePanelActorID(actorID)); actor != nil {
				if held, ok := heldTechnique(actor.Skills, action.Skill); ok {
					action.Skill = held
				}
			}
		}
		resolved[actorID] = action
	}
	return resolved
}

// validateDeclaredTechniques refuses a round that names a technique its actor
// never learned, or one whose numbers disagree with what the actor holds.
//
// The ordinary attack is exempt: it is the one action that requires no study, so
// a character who has learned nothing — including a mortal, who may cast nothing
// else — can still fight. Everything else must be in the actor's own learned set
// at the version they hold, which is the same list the window shows the player
// and the same list DecideEnemyAction chooses from.
//
// The check is on the identity AND the numbers, not the identity alone. §6 fixes a
// cast at the version the character learned, so a declaration that names a known
// technique while carrying a different definition's numbers would settle the round
// at values the character never had — and a generated technique makes that a real
// possibility rather than a theoretical one, because two stories can both invent a
// technique of the same name from different templates.
//
// A declaration for an actor not in this battle is left alone: it cannot be
// resolved anyway, and inventing a refusal for a character who is not fighting
// would report a fault that has no effect.
func validateDeclaredTechniques(session cultivation.CombatSession, actions map[string]cultivation.CombatAction) error {
	for actorID, action := range actions {
		if action.Kind != cultivation.ActionTechnique {
			continue
		}
		if cultivation.IsOrdinaryAttack(action.Skill) {
			continue
		}
		actor := session.CombatantByID(normalizeStatePanelActorID(actorID))
		if actor == nil {
			continue
		}
		held, learned := heldTechnique(actor.Skills, action.Skill)
		if !learned {
			return fmt.Errorf(
				"%w: 参战者 %s 未习得 %s / combatant has not learned the declared technique",
				ErrCombatTechniqueNotLearned, actor.ID, action.Skill.ID,
			)
		}
		// A bare identity is what the API layer produces for a name it could not
		// resolve, and it is resolved from the actor's own set below. Anything that
		// does carry numbers must carry the learned ones.
		if !isBareTechniqueReference(action.Skill) && !sameTechniqueNumbers(held, action.Skill) {
			return fmt.Errorf(
				"%w: 参战者 %s 掌握的 %s 与声明数值不一致 / the declared numbers differ from the learned version",
				ErrCombatActionRejected, actor.ID, action.Skill.ID,
			)
		}
	}
	return nil
}

// isBareTechniqueReference reports whether a declaration names a technique without
// carrying its definition.
//
// The two fields are enough to tell: every real definition is registered at a
// version of at least one and reaches at least one target, and ValidateSkillDefinition
// refuses anything else. A declaration with neither cannot have come from the
// published table.
func isBareTechniqueReference(skill cultivation.SkillDef) bool {
	return skill.Version == 0 && skill.Targets == 0
}

// heldTechnique finds the learned entry a declaration refers to.
//
// A declaration that names a version must match it; one that names only the
// identity is answered with the version the character holds.
func heldTechnique(learned []cultivation.SkillDef, declared cultivation.SkillDef) (cultivation.SkillDef, bool) {
	for _, skill := range learned {
		if skill.ID != declared.ID {
			continue
		}
		if declared.Version == 0 || skill.Version == declared.Version {
			return skill, true
		}
	}
	return cultivation.SkillDef{}, false
}

// sameTechniqueNumbers reports whether a declaration carries the learned
// definition's numbers.
//
// The comparison covers every field a cast is settled from, so a caller cannot
// keep a known ID and quietly raise the power, lower the cost or add targets. The
// identity and the name are compared too: the name is part of a generated
// definition's identity and is what the interface shows.
func sameTechniqueNumbers(held, declared cultivation.SkillDef) bool {
	return held.ID == declared.ID &&
		held.Version == declared.Version &&
		held.NameKey == declared.NameKey &&
		held.Name == declared.Name &&
		held.Power == declared.Power &&
		held.ManaFraction == declared.ManaFraction &&
		held.Burden == declared.Burden &&
		held.Targets == declared.Targets &&
		held.HitOffset == declared.HitOffset &&
		held.ExecutionActions == declared.ExecutionActions &&
		held.IgnoreDefense == declared.IgnoreDefense &&
		held.RequiresFocus == declared.RequiresFocus &&
		held.Element == declared.Element &&
		held.ShieldBaseHealthFraction == declared.ShieldBaseHealthFraction &&
		held.DurationExchanges == declared.DurationExchanges &&
		held.ResidualBurdenFloor == declared.ResidualBurdenFloor &&
		held.LeavesResidual == declared.LeavesResidual &&
		held.SafeRecoveryMinutes == declared.SafeRecoveryMinutes &&
		held.RecoveryMinHealth == declared.RecoveryMinHealth &&
		held.RecoveryMinMana == declared.RecoveryMinMana &&
		held.VariantBase == declared.VariantBase
}

// combatSettlementOps derives the Actor State writes a settled battle produces.
//
// §七.2 requires the settled numbers to reach the save through the story's own
// commit path, and §七.8 requires a defeated or fled enemy to keep its real
// wounds and remaining resources. Both follow from writing the four pools the
// rules own — health, mana, burden and the residual floor — as absolute values
// derived from the session, and letting the settlement re-derive the maxima and
// the four attributes afterwards.
//
// A participant whose template does not declare the cultivation fields is
// skipped: the rules have no owned pool to write for them, and inventing one
// would put a number in a schema that never declared it.
func combatSettlementOps(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	session cultivation.CombatSession,
	eventID string,
) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	ops := make([]ActorStateOp, 0, len(session.Combatants)*4)
	for _, combatant := range session.Combatants {
		actorID := normalizeStatePanelActorID(combatant.ID)
		if actorID == "" {
			continue
		}
		templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
		if !found {
			continue
		}
		template := actorStateTemplateByID(system, templateID)
		if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
			continue
		}
		writes := []struct {
			fieldID string
			value   any
		}{
			{xiuxianFieldHealth, combatant.Attributes.Health},
			{xiuxianFieldMana, combatant.Attributes.Mana},
			{xiuxianFieldBurden, combatant.Burden.Current},
			{xiuxianFieldBurdenFloor, combatant.Burden.Residual},
		}
		actorOps := make([]ActorStateOp, 0, len(writes))
		for _, write := range writes {
			if !actorStateTemplateDeclaresField(template, write.fieldID) {
				continue
			}
			actorOps = append(actorOps, combatSettlementOp(actorID, write.fieldID, write.value, eventID))
		}
		// A formation whose upkeep failed in the fight is over. §4.3 makes losing
		// the ability to maintain it one of the three ways it stops working, and
		// the recorded slot has to say so: leaving 维持中 as 是 would have the
		// settlement pay its bonus again on the next turn, for a formation nobody
		// is still paying for.
		if ended, ok := xiuxianFormationEndedByBattle(state, actorID, combatant); ok {
			actorOps = append(actorOps, combatSettlementOp(actorID, xiuxianFieldFormation, ended, eventID))
		}
		// Writes that would not move a number are dropped, so a round in which
		// nothing landed does not fill the journal with the same values again.
		ops = append(ops, xiuxianChangedOps(state, actorID, actorOps)...)
	}
	return normalizeActorStateOps(ops)
}

// xiuxianFormationEndedByBattle reports the recorded formation slot a battle has
// ended, and whether there is one to write.
//
// It only reports a change for a formation that was running when the fight began
// and is not running now. A formation that was already recorded as stopped is
// left alone — restating it would be a write that changes nothing — and an actor
// who never had one produces no operation at all.
func xiuxianFormationEndedByBattle(state map[string]any, actorID string, combatant cultivation.Combatant) (map[string]any, bool) {
	entry, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldFormation)
	if !ok || !xiuxianFormationActive(entry) || combatant.Formation.Active {
		return nil, false
	}
	ended := make(map[string]any, len(entry)+1)
	for key, value := range entry {
		ended[key] = value
	}
	ended["维持中"] = "否"
	return ended, true
}

// combatEngagementRetirementOp removes the scene's declaration of a fight that
// has just ended.
//
// 本次交战 is what the window reads to know a fight exists, and the instruction
// that tells the model to write {} when a fight ends is a request, not a
// guarantee: a live run showed the declaration surviving the settlement, so the
// window went on offering the same opponent and a second battle could be opened
// for a fight already won. The settlement is the one place that knows the fight
// reached a result, so it is the place that retires the claim.
//
// Only the opponents this session actually fought are removed, and only the ones
// the scene still declares. A declaration naming someone else is a different
// fight the settlement has no verdict on, and removing it would end a fight
// nobody had. The field is left absent rather than written as {} when the story
// cannot declare engagements at all: a schema without the field has no fight to
// retire, and writing one would put a value in a field it never declared.
func combatEngagementRetirementOp(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	session cultivation.CombatSession,
	outcome cultivation.ExchangeOutcome,
	eventID string,
) (ActorStateOp, bool) {
	if !outcome.Ended || !storyContextHasEngagementField(state, system) {
		return ActorStateOp{}, false
	}
	declared := storyContextEngagementRecords(storyContextActorState(state))
	if len(declared) == 0 {
		return ActorStateOp{}, false
	}
	// End this encounter's enemy and consenting ally declarations together.
	// An ally's acceptance belongs to this fight, not a future rematch.
	fought := make(map[string]bool, len(session.Combatants))
	for _, combatant := range session.Combatants {
		if normalizeStatePanelActorID(combatant.ID) == DefaultActorID {
			continue
		}
		if actorID := normalizeStatePanelActorID(combatant.ID); actorID != "" {
			fought[actorID] = true
		}
	}
	if len(fought) == 0 {
		return ActorStateOp{}, false
	}
	next := make(map[string]any, len(declared))
	changed := false
	for key, value := range declared {
		if fought[normalizeStatePanelActorID(key)] {
			changed = true
			continue
		}
		next[key] = value
	}
	if !changed {
		return ActorStateOp{}, false
	}
	return ActorStateOp{
		Op:           "set",
		ActorID:      normalizeStatePanelActorID(DefaultStoryContextActorID),
		FieldID:      normalizeActorStateFieldName(storySceneEngagementField),
		Value:        next,
		Reason:       "这场交手已经结束，由战斗结算撤销交战声明，不由叙述改写。",
		SourceTurnID: eventID,
		SourceKind:   StateOpSourceCombatSettlement,
		SourceID:     "combat",
	}, true
}

// combatSettlementOp builds one battle write.
//
// Every field is written as an absolute value rather than a delta, because the
// session holds the settled pool rather than the change to it. A delta would
// accumulate the difference between two settle attempts and drift away from the
// battle that was actually fought.
func combatSettlementOp(actorID, fieldID string, value any, eventID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       "由战斗结算写入，不由叙述改写。",
		SourceTurnID: eventID,
		SourceKind:   StateOpSourceCombatSettlement,
		SourceID:     "combat",
	}
}

// A battle's settlement is deliberately NOT part of the reroll discard chain.
//
// Every other rules-owned write — the rule resolution, the settlement, the
// advance, the learning — is dropped and re-derived when a turn is rerolled,
// because each of them is a consequence of the rule outcome the reroll replaced.
// A battle is not: it is a fight the player actually fought, resolved from its
// own command identity and already on the journal as its own side record. §七.8
// requires a settled outcome to stand, and §七.2 requires its draws to be bound
// to the command that authorised it. Discarding a battle because a later turn
// was rewritten would do exactly what both forbid — un-fight a fight, and let a
// retry produce a different one.
//
// Consistency is kept without a discard instead, by writing the four pools as
// absolute values: a later exchange's record simply states the pools it settled
// at, and the turn settlement re-derives the maxima and the attributes from
// whatever the state then holds.

// AppendCombatRecordRequest is one committed exchange.
//
// The caller supplies the settled session and outcome the rules produced, not
// the writes: the settlement's operations are derived by the store from the
// session, so a caller cannot put numbers into the save that the battle did not
// produce.
type AppendCombatRecordRequest struct {
	enemyDecision *EnemyDecisionReceipt
	BranchID      string
	// TurnID attaches the battle to a story turn. Empty is legitimate for a
	// battle fought entirely in the window.
	TurnID string
	// WorldTime is the current in-fiction time, frozen onto the record.
	WorldTime string
	// Session is the battle after the exchange.
	Session cultivation.CombatSession
	// Outcome is the exchange that was settled.
	Outcome cultivation.ExchangeOutcome
	// CommandID is the caller-owned idempotency key for this exchange. When it is
	// set, a second append carrying the same command ID and the same CommandHash
	// returns what the first one settled instead of resolving the exchange again;
	// the same command ID with a different hash is a conflict rather than a
	// silent overwrite.
	CommandID string
	// CommandHash is the caller's fingerprint of the content behind CommandID.
	CommandHash            string
	previousSession        *cultivation.CombatSession
	lethalIntents          []CombatLethalIntent
	playerTakeLifeTargetID string
}

// AppendCombatRecord commits one settled exchange as a side record plus the
// state change it produced, in a single journal transaction.
//
// The record and the settlement are one transaction so a battle either exists in
// the journal with its wounds applied or does not exist at all. A record without
// its settlement would be a fight that happened to nobody; a settlement without
// its record would be a wound with no battle behind it.
//
// The append is idempotent on CommandID. A retry after a lost response, a
// repeated click or a recovery re-read returns what the first attempt committed,
// so the same exchange is never resolved twice.
func (s *Store) AppendCombatRecord(storyID string, req AppendCombatRecordRequest) (CombatRecordResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return CombatRecordResult{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return CombatRecordResult{}, err
	}
	return s.appendCombatRecordLocked(storyID, meta, records, req)
}

// appendCombatRecordLocked is the commit itself, for callers that already hold
// the store lock and have already read the branch.
//
// It exists because the lock is not reentrant: a round that reads the branch,
// resolves against it and then commits has to do all three under one hold, or
// another round could advance the battle between the read and the write and the
// resolved session would overwrite a round that had already been fought.
func (s *Store) appendCombatRecordLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req AppendCombatRecordRequest) (CombatRecordResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return CombatRecordResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrCombatCommandConflict, commandID)
	}

	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return CombatRecordResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := combatRecordForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return CombatRecordResult{}, err
		}
		if found {
			// The first attempt already settled this exact exchange. Returning
			// its result makes the retry a read rather than a second resolution.
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return CombatRecordResult{}, branchTerminalError()
	}
	if strings.TrimSpace(req.Session.ID) == "" {
		return CombatRecordResult{}, fmt.Errorf("战斗记录缺少 session id / a combat record needs a session id")
	}
	if err := validateCombatSession(req.Session); err != nil {
		return CombatRecordResult{}, err
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return CombatRecordResult{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("cbt")
	lethalIntents := append([]CombatLethalIntent(nil), req.lethalIntents...)
	if req.playerTakeLifeTargetID != "" {
		if req.previousSession == nil || !combatRecordSettlesExchange(req.Outcome) {
			return CombatRecordResult{}, ErrCombatPlayerIntentInvalid
		}
		lethalIntents = append(lethalIntents, CombatLethalIntent{
			ActorID: DefaultActorID, TargetID: normalizeStatePanelActorID(req.playerTakeLifeTargetID),
			SourceTurnID: strings.TrimSpace(req.TurnID), SourceEventID: eventID,
		})
	}
	// An opening record settles no exchange, so it writes no pools and spends
	// nothing. §七.1's opening is the encounter itself: the fight exists and both
	// sides are standing at the numbers they already had. Applying a settlement
	// to it would charge the player for a round nobody played, which is exactly
	// what the round-zero record exists to prevent.
	var settlementOps []ActorStateOp
	if combatRecordSettlesExchange(req.Outcome) {
		settlementOps = combatSettlementOps(projection.State, system, req.Session, eventID)
		// The battle moves the current pools, and the settlement re-derives the
		// maxima and the four attributes from the stage the character is still at.
		// Running it here rather than trusting the session's copies keeps the panel,
		// the battle and the simulation reading one set of numbers.
		settlementOps = append(settlementOps, settleXiuxianBattleState(projection.State, system, settlementOps, eventID)...)
		// A dose spent in the exchange is a real consumption, and §4.2 requires the
		// current stock to be saved with it. It is written as its own operation on the
		// carried record rather than folded into the pools: an inventory is not a
		// number the settlement derives, so it is the battle that owns the write.
		settlementOps = append(settlementOps, xiuxianDoseSettlementOps(projection.State, system, req.Session, eventID)...)
		// A position changed mid-fight is a real arrangement the branch keeps. §2
		// refuses to let closing the window undo the swap, so the positions the
		// battle ended with are written back here rather than left to the narrative.
		settlementOps = append(settlementOps, xiuxianEquipmentSwapSettlementOps(projection.State, system, req.Session, eventID)...)
		// A fight that has reached a result stops being a fight the scene offers.
		// The turn's own instruction asks the model to write {} here, and a live
		// run showed it not happening, which left the window offering the same
		// opponent after the battle was won.
		if op, ok := combatEngagementRetirementOp(projection.State, system, req.Session, req.Outcome, eventID); ok {
			settlementOps = append(settlementOps, op)
		}
	}
	var fatalities []CombatFatality
	var terminalOutcome *TerminalOutcome
	var stateOps []interactivestate.Op
	if req.previousSession != nil && len(lethalIntents) > 0 {
		fatalities = combatFatalities(*req.previousSession, req.Session, req.Outcome, lethalIntents, eventID)
		for _, fatality := range fatalities {
			if fatality.ActorID == DefaultActorID {
				causedByTurnID := ""
				if turn := latestTurnForBranchHead(records, branch.Head); turn != nil {
					causedByTurnID = turn.ID
				}
				terminalOutcome = terminalOutcomeForCombatFatality(fatality, causedByTurnID)
				continue
			}
			if actorIsArchived(projection.State, fatality.ActorID) {
				continue
			}
			reason := "在正式致命交战中死亡"
			stateOps = append(stateOps, interactivestate.Op{
				Op: "set", Path: actorArchiveStatePath(fatality.ActorID),
				Value:  map[string]any{"reason": reason, "source_turn_id": fatality.SourceTurnID, "death_event_id": eventID},
				Reason: reason, SourceTurnID: fatality.SourceTurnID,
				SourceKind: StateOpSourceCombatSettlement, SourceID: eventID,
			})
			presenceOps, presenceActorOps := compileActorArchivePresenceCleanup(system, projection.State, actorLifecycleIntent{
				ActorID: fatality.ActorID, Op: interactivestate.Archive, Reason: reason,
			}, fatality.SourceTurnID)
			stateOps = append(stateOps, presenceOps...)
			settlementOps = append(settlementOps, presenceActorOps...)
		}
	}

	record := CombatRecordEvent{
		EnemyDecision: req.enemyDecision,
		V:             schemaVersion, Type: StoryEventTypeCombatRecord, ID: eventID,
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: commandID, CommandHash: commandHash,
		WorldTime: strings.TrimSpace(req.WorldTime),
		Session:   req.Session, Outcome: req.Outcome,
		LethalIntents:   lethalIntents,
		Fatalities:      append([]CombatFatality(nil), fatalities...),
		TerminalOutcome: normalizeTerminalOutcomePointer(terminalOutcome),
	}

	headID := eventID
	newEvents := []any{record}
	if len(settlementOps) > 0 || len(stateOps) > 0 {
		for _, op := range stateOps {
			if err := validateStateOp(op); err != nil {
				return CombatRecordResult{}, err
			}
		}
		for _, op := range settlementOps {
			if err := validateActorStateOp(op); err != nil {
				return CombatRecordResult{}, err
			}
		}
		// The settlement is a separate event parented to the record, so the branch
		// head advances through the battle and then through its consequence, and
		// the next turn is parented after both. It carries its own identity: an
		// event whose ID is also its parent closes the branch's parent chain into a
		// cycle, and every later walk of that chain would never reach an ancestor.
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, eventID, branchID, now, stateOps, settlementOps))
		headID = deltaID
	}
	// A settled battle is a fact, and a task objective may be waiting on its
	// result. Only an ended exchange is one: a round still in progress decided
	// nothing, and ticking a hunt on it would let a task complete mid-fight.
	if req.Outcome.Ended {
		ticks, _, err := questAdvanceEvents(projection, branchID, req.TurnID, questFactFromCombatOutcome(req.Outcome.Result), eventID, headID, now, projection.clockSeconds())
		if err != nil {
			return CombatRecordResult{}, err
		}
		for _, tick := range ticks {
			newEvents = append(newEvents, tick)
			if tickRecord, ok := tick.(QuestRecordEvent); ok {
				headID = tickRecord.ID
			}
		}
	}

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return CombatRecordResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return CombatRecordResult{}, err
	}
	return CombatRecordResult{
		EnemyDecision: req.enemyDecision,
		EventID:       eventID, SessionID: req.Session.ID,
		Session: req.Session, Outcome: req.Outcome, SettlementOps: settlementOps,
		Fatalities: fatalities, TerminalOutcome: terminalOutcome,
	}, nil
}

// settleXiuxianBattleState re-derives the cast's numbers on top of a battle's
// own writes.
//
// It projects the battle's operations onto the pre-commit state and then runs
// the ordinary settlement, so a battle reaches the same numbers through the same
// path a turn does. Settling against the live state instead would apply the
// battle's writes a second time.
func settleXiuxianBattleState(state map[string]any, system StoryDirectorActorStateSystem, combatOps []ActorStateOp, eventID string) []ActorStateOp {
	if !ActorStateSystemIsXiuxian(system) {
		return nil
	}
	projected := cloneStoryState(state)
	for _, op := range combatOps {
		applyActorStateOp(projected, op)
	}
	return settleXiuxianStoryState(projected, system, eventID)
}

// CombatSessions reads one branch's live battles, keyed by session id.
//
// It is the read the combat window opens on: a window that was closed and
// reopened has to find the same battle at the same round, and a battle that has
// ended has to still be there so the result can be read. The projection is
// rebuilt from the canonical journal, so this survives a restart without any
// process-local index.
func (s *Store) CombatSessions(storyID, branchID string) (map[string]cultivation.CombatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return nil, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return nil, err
	}
	sessions := cloneCombatSessions(projection.Combats)
	for id, session := range sessions {
		for i := range session.Combatants {
			session.Combatants[i].Cultivation = session.Combatants[i].Cultivation.WithRules(projection.cultivationRules())
		}
		sessions[id] = session
	}
	return sessions, nil
}

// CombatRoundRecord is one committed exchange of one battle.
//
// It is what the window reviews: §七.1 requires the player to be able to look
// back over the rounds, and the outcome the exchange settled is the record of
// what actually happened. The session is deliberately not carried here — the
// round-by-round pools are derivable from the outcomes, and repeating the whole
// battle once per round would make the history grow with the square of its length.
type CombatRoundRecord struct {
	EnemyDecision *EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	EventID       string                `json:"event_id"`
	// WorldTime is the in-fiction time this exchange happened at.
	WorldTime     string                      `json:"world_time,omitempty"`
	Outcome       cultivation.ExchangeOutcome `json:"outcome"`
	LethalIntents []CombatLethalIntent        `json:"lethal_intents,omitempty"`
	Fatalities    []CombatFatality            `json:"fatalities,omitempty"`
}

// CombatHistory returns one battle's committed rounds, oldest first.
//
// It reads the branch's own records rather than a process-local log, so the
// rounds a player can review after a restart are the rounds that were actually
// committed, in the order they were committed.
func (s *Store) CombatHistory(storyID, branchID, sessionID string) ([]CombatRoundRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	_, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return nil, err
	}
	return combatHistoryFor(records, strings.TrimSpace(sessionID)), nil
}

// CombatLethalIntents returns only the opening declaration frozen into this
// battle's first committed record. Later turns cannot upgrade a live fight.
func (s *Store) CombatLethalIntents(storyID, branchID, sessionID string) ([]CombatLethalIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[strings.TrimSpace(branchID)]
	if !ok {
		return nil, fmt.Errorf("interactive branch does not exist: %s", branchID)
	}
	return frozenLethalIntentsForBattle(records, branch.Head, strings.TrimSpace(sessionID)), nil
}

// combatRecordSettlesExchange reports whether a committed record is an exchange
// rather than the encounter's opening.
//
// Opening a battle establishes that the fight exists and writes nothing else: no
// round was played, so no pools moved and no dose was spent. The record still has
// to be committed — the window, a restart and the one-battle-per-actor guard all
// read the fight from the journal — but it is not a round, and every reader that
// counts, narrates or hands over rounds has to tell the two apart. Round is the
// discriminator because ResolveExchange always numbers the exchange it settles
// and an opening settles none.
func combatRecordSettlesExchange(outcome cultivation.ExchangeOutcome) bool {
	return outcome.Round > 0
}

// combatHistoryFor filters a branch's records down to one battle's rounds.
//
// A record whose session ID does not match belongs to a different fight and is
// skipped rather than reported, which is what keeps two battles fought on one
// branch from appearing as one. The opening record is skipped too: it is the
// encounter, not an exchange, and listing it would show the player a round that
// nobody played.
func combatHistoryFor(records []StoryEventRecord, sessionID string) []CombatRoundRecord {
	if sessionID == "" {
		return nil
	}
	history := make([]CombatRoundRecord, 0)
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		if event.Session.ID != sessionID || !combatRecordSettlesExchange(event.Outcome) {
			continue
		}
		history = append(history, CombatRoundRecord{
			EnemyDecision: event.EnemyDecision, EventID: event.ID, WorldTime: event.WorldTime, Outcome: event.Outcome,
			LethalIntents: append([]CombatLethalIntent(nil), event.LethalIntents...),
			Fatalities:    append([]CombatFatality(nil), event.Fatalities...),
		})
	}
	return history
}

// CombatRound returns one committed round together with the battle as it stood
// when that round settled.
//
// The session comes from the round's own record rather than from the branch's
// current projection. A later round has already moved the pools, so pairing an
// old outcome with the latest session would describe an exchange against numbers
// the exchange never used — and a passage written from that describes a fight the
// journal does not contain. Reading it back from the record is also what makes
// the answer survive a restart: the journal remembers the round, not the process.
//
// The found result is false when the branch holds no such round, which the caller
// reports rather than inventing an exchange to describe.
func (s *Store) CombatRound(storyID, branchID, sessionID, roundEventID string) (CombatRoundRecord, cultivation.CombatSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return CombatRoundRecord{}, cultivation.CombatSession{}, false, err
	}
	defer release()
	_, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return CombatRoundRecord{}, cultivation.CombatSession{}, false, err
	}
	return combatRoundFor(records, strings.TrimSpace(sessionID), strings.TrimSpace(roundEventID))
}

// combatRoundFor finds one battle's committed round by its event ID.
//
// An opening record is not a round: it describes the encounter and settles
// nothing, so it is not a round a passage can be written about. The found result
// is false for it, which is what keeps a narration request from attaching prose
// to an exchange that never happened.
func combatRoundFor(records []StoryEventRecord, sessionID, roundEventID string) (CombatRoundRecord, cultivation.CombatSession, bool, error) {
	if sessionID == "" || roundEventID == "" {
		return CombatRoundRecord{}, cultivation.CombatSession{}, false, nil
	}
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return CombatRoundRecord{}, cultivation.CombatSession{}, false, fmt.Errorf("decode committed combat record: %w", err)
		}
		if event.Session.ID != sessionID || event.ID != roundEventID {
			continue
		}
		if !combatRecordSettlesExchange(event.Outcome) {
			return CombatRoundRecord{}, cultivation.CombatSession{}, false, nil
		}
		return CombatRoundRecord{
			EnemyDecision: event.EnemyDecision, EventID: event.ID, WorldTime: event.WorldTime, Outcome: event.Outcome,
			LethalIntents: append([]CombatLethalIntent(nil), event.LethalIntents...),
			Fatalities:    append([]CombatFatality(nil), event.Fatalities...),
		}, event.Session, true, nil
	}
	return CombatRoundRecord{}, cultivation.CombatSession{}, false, nil
}

// CombatRecordForCommand returns what a previous attempt committed under this
// command identity, so a retry is a read rather than a second settlement.
func (s *Store) CombatRecordForCommand(storyID, branchID, commandID, commandHash string) (CombatRecordResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return CombatRecordResult{}, false, err
	}
	defer release()
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return CombatRecordResult{}, false, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	return combatRecordForCommand(records, strings.TrimSpace(branchID), commandID, strings.TrimSpace(commandHash))
}

// combatRecordForCommandLocked is the read itself, for a caller that already
// holds the store lock and has already read the branch's records.
func combatRecordForCommandLocked(meta StoryMeta, records []StoryEventRecord, branchID, commandID, commandHash string) (CombatRecordResult, bool, error) {
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	return combatRecordForCommand(records, strings.TrimSpace(branchID), commandID, strings.TrimSpace(commandHash))
}

// combatRecordForCommand looks for the battle record a previous attempt
// committed under this command ID. It scans the branch's own recent records
// rather than trusting a process-local index, so the answer survives a restart:
// the journal is what remembers, not the process.
//
// A found command ID with a different hash is a conflict. The caller reused one
// identity for two different exchanges, and either answer would be wrong.
func combatRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (CombatRecordResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return CombatRecordResult{}, false, fmt.Errorf("decode committed combat record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return CombatRecordResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q", ErrCombatCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return CombatRecordResult{
			EnemyDecision: event.EnemyDecision, EventID: event.ID, SessionID: event.Session.ID,
			Session: event.Session, Outcome: event.Outcome,
			Fatalities:      append([]CombatFatality(nil), event.Fatalities...),
			TerminalOutcome: normalizeTerminalOutcomePointer(event.TerminalOutcome),
		}, true, nil
	}
	return CombatRecordResult{}, false, nil
}

// combatsFromPath rebuilds the live battles along a branch path.
//
// It mirrors stateFromPath, and deliberately takes the same input: the fights are
// derived from the same records the state is, so a rewound state can never carry
// wounds whose battle was left behind. Restoring the pools from the checkpoint
// while dropping the battle would show damage with nothing behind it, and the
// window would have no fight to reopen.
func combatsFromPath(path []StoryEventRecord) (map[string]cultivation.CombatSession, error) {
	branch := &storyBranchProjection{}
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, err
		}
		if err := applyCombatRecord(branch, event); err != nil {
			return nil, err
		}
	}
	return cloneCombatSessions(branch.Combats), nil
}

// applyCombatRecord folds a side record into the branch projection.
//
// It is the only place a battle enters the projection during a rebuild, so a
// replay reproduces the battle that was live when the record was committed. The
// record's own state change arrives as the paired state delta in the same
// transaction, so this function only has to remember the battle itself.
func applyCombatRecord(branch *storyBranchProjection, event CombatRecordEvent) error {
	if err := validateCombatSession(event.Session); err != nil {
		return err
	}
	if branch.Combats == nil {
		branch.Combats = map[string]cultivation.CombatSession{}
	}
	branch.Combats[event.Session.ID] = event.Session
	return nil
}

// validateCombatRecordEvent rejects a record that could not have been honestly
// attributed. A malformed row is an error rather than a silent drop, because a
// dropped record is a battle that never happened.
func validateCombatRecordEvent(event CombatRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("战斗记录缺少 branch_id / a combat record requires a branch")
	}
	if err := validateCombatSession(event.Session); err != nil {
		return err
	}
	return nil
}

// validateCombatSession checks that a battle is internally consistent enough to
// be shown and resumed.
//
// It checks shape, not balance: an empty identity, a combatant with no side, or
// two entries for one actor would each make the window show something the rules
// cannot account for, and those are the cases that must not reach the journal.
func validateCombatSession(session cultivation.CombatSession) error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("战斗记录缺少 session id / a combat session needs an id")
	}
	if len(session.Combatants) == 0 {
		return fmt.Errorf("战斗记录没有参战者 / a combat session needs combatants")
	}
	if session.Round < 1 {
		return fmt.Errorf("战斗回合数无效: %d / a combat session needs a positive round", session.Round)
	}
	seen := make(map[string]bool, len(session.Combatants))
	for _, combatant := range session.Combatants {
		actorID := normalizeStatePanelActorID(combatant.ID)
		if actorID == "" {
			return fmt.Errorf("战斗记录包含缺少 id 的参战者 / a combatant has no actor id")
		}
		if seen[actorID] {
			return fmt.Errorf("战斗记录包含重复参战者: %s / duplicate combatant", actorID)
		}
		seen[actorID] = true
		switch combatant.Side {
		case cultivation.SidePlayer, cultivation.SideEnemy:
		default:
			return fmt.Errorf("战斗记录包含阵营无效的参战者: %s / a combatant has an invalid side", actorID)
		}
		if combatant.Attributes.Health < 0 || combatant.Attributes.HealthMax < 0 {
			return fmt.Errorf("战斗记录包含负数生命值: %s / a combatant has a negative pool", actorID)
		}
	}
	return nil
}
