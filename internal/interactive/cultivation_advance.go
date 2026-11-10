package interactive

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
)

// StateOpSourceCultivationAdvance marks an Actor State operation the cultivation
// rules produced while advancing a character's position on the stage table.
//
// It is a separate source kind from the settlement's because the two answer
// different questions. The settlement re-derives numbers that are a function of
// the stage; an advance is what moves the stage itself, from an authorised
// training interval or a resolved breakthrough. A reader of the journal can tell
// "this character trained" from "these numbers were recomputed", and a reroll can
// drop both without touching anything the narrative wrote.
const StateOpSourceCultivationAdvance = "cultivation_advance"

// TurnCultivationInput is the model-facing cultivation module of one story turn.
//
// It is the structured form of a player command like "在此闭关半年，修习现有功法，
// 不服用丹药". The design's player-input model requires the duration, the method and
// the known conditions to be enough to proceed without a confirmation prompt, and
// story world time is free text that cannot yield rule-days, so the interval has to
// arrive as a structured value. The model translates the player's command into
// this shape; the rules own every number derived from it.
//
// The module carries no stage, no attribute and no progress figure. Those are the
// settlement's and the stage table's to produce, which is what makes the design's
// "the model may not reword the character's cultivation" hold structurally.
type TurnCultivationInput struct {
	MethodID string `json:"method_id,omitempty" jsonschema_description:"本段实际修炼或用于突破的已掌握且正在运转功法ID。省略沿用当前最高成长系数的一门，不叠加多门收益；不能用名称或未学内容获取系数。"`
	// ActorID is whose cultivation is being advanced. Omit for the protagonist.
	ActorID string `json:"actor_id,omitempty" jsonschema_description:"The character who cultivates. Omit for the protagonist."`
	// Days is the real in-fiction duration of the authorised training interval.
	// Fractional days are accepted so an interval shorter than a day is still
	// expressible. Zero means no training happened this turn.
	Days float64 `json:"days,omitempty" jsonschema_description:"Real in-fiction days of authorised cultivation this turn, from the player's own command. Use 0 when the player did not train. Fractional days are allowed. Never invent a duration the player did not ask for."`
	// Environment is the spiritual quality of the place the interval is spent in.
	Environment string `json:"environment,omitempty" jsonschema:"enum=poor,enum=ordinary,enum=stable_spiritual_site" jsonschema_description:"Where the training happened: poor for a barren or corrupted place, ordinary for an unremarkable one, stable_spiritual_site for a genuinely qualified spiritual site. Omit for ordinary."`
	// Resource is the strength of the real auxiliary resources actually consumed
	// during the interval. A pill that is not consumed is not a resource.
	Resource string `json:"resource,omitempty" jsonschema:"enum=none,enum=normal_support,enum=strong_support" jsonschema_description:"Auxiliary resources actually consumed during this interval: none, normal_support, or strong_support. A pill merely held or mentioned is none."`
	// AttemptBreakthrough is the realm the player authorised an attempt to enter.
	// An absent value means no attempt was authorised, and the rules then do not
	// roll at all: the design requires an explicit authorisation, and a missing
	// precondition must not consume anything.
	AttemptBreakthrough string `json:"attempt_breakthrough,omitempty" jsonschema:"enum=foundation,enum=core,enum=nascent,enum=spirit,enum=void,enum=integration,enum=mahayana" jsonschema_description:"Set only when the player explicitly attempts a major breakthrough this turn, or previously authorised one on reaching the conditions. The value is the realm being entered. Omit to leave the character where they are; a small-stage advance needs no authorisation."`
	// AuxiliaryMedicine is true only when one appropriate breakthrough pill
	// actually exists and is actually consumed by this attempt.
	//
	// It is retained on the module so a caller can express the intent, but the
	// rules do not take it as truth: xiuxianBreakthroughBonus derives the bonus
	// from the catalogue and the carried stock, and no registered breakthrough
	// pill exists yet.
	AuxiliaryMedicine bool `json:"auxiliary_medicine,omitempty" jsonschema_description:"True only when a suitable breakthrough pill is really consumed by this attempt. A pill described but not consumed does not count."`
	// StableSite is a genuinely qualified cave dwelling for the attempt.
	//
	// Like AuxiliaryMedicine this is intent, not truth: the bonus comes from the
	// committed scene, so a turn that claims a cave dwelling while standing in a
	// market earns nothing.
	StableSite bool `json:"stable_site,omitempty" jsonschema_description:"True only when the attempt happens somewhere genuinely qualified for it, such as the character's own established cave dwelling."`
	// QualifiedGuardian is a guardian whose qualification is real, not merely
	// asserted in the narration.
	QualifiedGuardian bool `json:"qualified_guardian,omitempty" jsonschema_description:"True only when a genuinely qualified guardian is actually present and helping. A bystander who is described as helping does not count."`
}

// Empty reports whether the module carries nothing to apply.
func (input TurnCultivationInput) Empty() bool {
	return input.Days <= 0 && strings.TrimSpace(input.AttemptBreakthrough) == ""
}

// cloneTurnCultivationInput copies a cultivation module so an accepted one can
// be retained in a draft without the caller's slice being aliased.
func cloneTurnCultivationInput(input *TurnCultivationInput) *TurnCultivationInput {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

// validateTurnCultivationModule rejects a cultivation module the rules could not
// act on, so the model repairs it rather than the turn silently doing nothing.
//
// It checks shape, not outcome. Whether an interval gains any progress, and
// whether an authorised breakthrough is eligible, are the rules' answers: an
// ineligible attempt is a legitimate result the player reads, not a submission
// error. What must not pass is a duration that is negative, infinite or NaN, or a
// target realm that is not one the rules implement.
func validateTurnCultivationModule(input *TurnCultivationInput) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	if math.IsNaN(input.Days) || math.IsInf(input.Days, 0) || input.Days < 0 {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCultivation, nil, TurnSubmissionDiagnosticInvalidModule,
			"/cultivation/days", "a finite, non-negative number of days", fmt.Sprint(input.Days),
			"cultivation.days must be a finite, non-negative number of real in-fiction days.",
		)
	}
	target := strings.TrimSpace(input.AttemptBreakthrough)
	if target == "" {
		return nil
	}
	if _, ok := cultivation.BreakthroughChance(cultivation.BreakthroughTarget(target), cultivation.BreakthroughModifiers{}); !ok {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCultivation, nil, TurnSubmissionDiagnosticInvalidModule,
			"/cultivation/attempt_breakthrough", "foundation, core, nascent, spirit, void, integration or mahayana", target,
			"cultivation.attempt_breakthrough must name a realm the rules implement.",
		)
	}
	return nil
}

// CultivationAdvanceResult reports what one authorised interval did, in
// player-readable terms.
//
// It exists so the interface and the tests can see the real numbers the rules
// produced rather than re-deriving them: the panel reads the settled fields, and
// this is what changed them.
type CultivationAdvanceResult struct {
	MethodID      string
	RefusalReason string
	// ActorID is the character the interval applied to.
	ActorID string
	// Days is the interval actually applied.
	Days float64
	// ConsumedDays is how much of the named interval the rules really spent.
	// A character already standing at a major-realm bottleneck spends no further
	// time training, so the caller advances the story clock by the rules' own
	// interval rather than by the interval that was merely named.
	ConsumedDays float64
	// BreakthroughDays is the real time an eligible authorised attempt takes,
	// from the rules' own table: seven days for 筑基, thirty for 结丹, and so on.
	// It is zero when no attempt was made or the attempt's preconditions failed.
	//
	// The attempt is an interval of its own, so a player who names an hour still
	// spends the seven days the rules price a 筑基 attempt at.
	BreakthroughDays float64
	// Trained reports whether any progress was gained.
	Trained bool
	// StagesAdvanced counts the small-stage transitions the interval completed.
	StagesAdvanced int
	// StoppedAtBoundary reports that the character reached the top of the
	// implemented stage table and could not advance further by training.
	StoppedAtBoundary bool
	// RealmBefore and RealmAfter are the stage keys at each end of the interval.
	RealmBefore string
	RealmAfter  string
	// RealmProgressAfter is the banked position toward the next small stage when
	// the interval ended. It is carried rather than re-read because the interval
	// is what banked it, and it is what lets a caller see that a long interval and
	// several short ones leave the character in the same place.
	RealmProgressAfter float64
	// BreakthroughRequested reports that the player authorised an attempt.
	BreakthroughRequested bool
	// BreakthroughEligible reports whether the attempt's preconditions held. When
	// it is false and the attempt was requested, nothing was rolled and nothing
	// was consumed.
	BreakthroughEligible bool
	// BreakthroughReasons lists, in player-readable terms, the preconditions that
	// failed when an authorised attempt could not be made.
	BreakthroughReasons []string
	// BreakthroughRolled reports that a die was actually cast.
	BreakthroughRolled bool
	// BreakthroughChance is the probability the roll was compared against.
	BreakthroughChance float64
	// BreakthroughSucceeded reports the outcome of a rolled attempt.
	BreakthroughSucceeded bool
	// BreakthroughRecoveryDays is the real recovery a failed attempt imposed.
	BreakthroughRecoveryDays float64
	// ResidualCleared reports that a meridian residual injury was regulated away.
	ResidualCleared bool
	// SupportOps writes the auxiliary doses this interval really consumed. They
	// are part of the advance's own commit so the stock and the trained days
	// cannot disagree after a crash or a branch fork.
	SupportOps []ActorStateOp
}

// advanceXiuxianCultivation applies one authorised cultivation interval and any
// authorised breakthrough to a character, and returns the operations that move
// their rules position.
//
// It is the missing producer the settlement was waiting for. Without it the stage
// key had no writer during play, so the only thing that could move it was the
// narrative — exactly the case the design forbids.
//
// The division of labour is deliberate and is what keeps one number from having
// two authors:
//
//   - This function writes the stored position: the stage key, the banked
//     progress, the meridian burden, the residual floor, and the two current
//     pools when a real cost or wound moved them.
//   - The settlement then derives every displayed number — the maxima and the
//     four attributes — from the stage key this function wrote.
//
// So a breakthrough is one write of a stage key here, and the new maxima appear
// because the settlement recomputes them, not because two code paths agreed on a
// number.
//
// xiuxianFormalFacts are the branch-scoped facts the cultivation rules read that
// do not live on the character's own state map.
//
// They are passed in rather than looked up here because the advance receives only
// the state map, and the two facts it needs — whether a failed attempt's recovery
// is still running, and who this character is formally bound to — are owned by the
// branch projection. Neither may be taken from the request: §7.1 refuses a new
// attempt inside the recovery window, and it refuses a guardian bonus that rests
// on a claim rather than on a committed bond.
type xiuxianFormalFacts struct {
	Convalescing  bool
	Relationships []character.Relationship
}

// xiuxianFormalFactsFromBranch collects the two facts for one actor.
func xiuxianFormalFactsFromBranch(branch *storyBranchProjection, actorID string) xiuxianFormalFacts {
	facts := xiuxianFormalFacts{Convalescing: cultivationConvalescing(branch, actorID)}
	if branch != nil && branch.Characters != nil {
		facts.Relationships = append(facts.Relationships, branch.Characters.Relationships...)
	}
	return facts
}

// state must already carry the turn's own operations, so an interval is applied
// to the position the turn actually left the character in.
//
// facts carries the branch-scoped preconditions. They are formal facts the caller
// reads from the projection rather than values the request can assert, because
// §7.1 refuses a new attempt inside a failed breakthrough's recovery window and a
// model that could set that would be deciding its own eligibility.
func advanceXiuxianCultivation(
	state map[string]any,
	system StoryDirectorActorStateSystem,
	actorID string,
	input TurnCultivationInput,
	facts xiuxianFormalFacts,
	identity DomainCommitIdentity,
	turnID string,
) ([]ActorStateOp, CultivationAdvanceResult, bool) {
	result := CultivationAdvanceResult{ActorID: normalizeStatePanelActorID(actorID), Days: input.Days}
	if result.ActorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, result, false
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, result.ActorID)
	if !found {
		return nil, result, false
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, result, false
	}
	character, err := xiuxianCharacterFromState(system.CultivationRules,
		state, result.ActorID)
	if err != nil {
		// The stored stage is one the rules cannot read. Refusing is what keeps a
		// bad key visible rather than silently resetting the character to a mortal
		// and then advancing them from there.
		return nil, result, false
	}

	result.RealmBefore = cultivation.StageKey(character.Cultivation)
	if target := cultivation.BreakthroughTarget(strings.TrimSpace(input.AttemptBreakthrough)); target != "" && !xiuxianTemplateSupportsStage(template, cultivation.StageKey(npcGrowthEnteredRealm(target))) {
		result.BreakthroughRequested = true
		result.BreakthroughReasons = []string{"本局沿用开局时的境界范围，尚未包含目标境界；这次没有尝试，也没有消耗。"}
		result.RealmAfter = result.RealmBefore
		result.RealmProgressAfter = character.Accumulated
		return nil, result, true
	}
	method, refusal := selectedCultivationMethod(system.CultivationRules,
		state, result.ActorID, input.MethodID)
	if refusal != "" {
		result.RefusalReason = refusal
		result.RealmAfter = result.RealmBefore
		result.RealmProgressAfter = character.Accumulated
		return nil, result, true
	}
	quality, growth := character.PrimaryTechniqueQuality(), 0.0
	if method != nil {
		result.MethodID = method.ID
		quality, growth = method.Quality, method.GrowthCoefficient
	}
	ops := make([]ActorStateOp, 0, 8)

	// An unwritten pool reads as the real maximum, exactly as the settlement
	// reads it.
	//
	// This is not a convenience. The pools are written by the settlement, which
	// runs after the advance in the same commit, so on the turn a character is
	// first advanced their 生命 may not exist yet. Reading that absence as zero
	// health would make the body-condition scale zero and stop the character from
	// ever training — the one number that decides whether training happens at all
	// would then be decided by an unrelated ordering. Reading it as the maximum is
	// the same answer the schema default and the settlement both give.
	if _, ok := actorStateNumber(actorStateFieldValue(state, result.ActorID, xiuxianFieldHealth)); !ok {
		character.Health = character.Attributes().HealthMax
	}
	if _, ok := actorStateNumber(actorStateFieldValue(state, result.ActorID, xiuxianFieldMana)); !ok {
		character.Mana = character.Attributes().ManaMax
	}

	if input.Days > 0 {
		attributes := character.Attributes()
		// The bonus truth comes from the character's own committed facts, never
		// from the request. §六 lets the model translate the player's intent into
		// this module, but a place and an auxiliary resource only scale the rate
		// when they really exist: a claimed 稳定灵地 or a pill that was never held
		// and never spent is a sentence, not a condition.
		//
		// The auxiliary resource is the one case where existence is not enough:
		// §6.2 line 232 also requires it to be consumed and requires the rate to
		// change when it runs out. So the stock is planned against this interval
		// first, the supported days are trained at the supported rate, and the
		// remainder is trained at the ordinary one. The doses that paid for the
		// covered days are written back below.
		resource, supportDoses, supportedDays := xiuxianPlanTrainingSupport(system.CultivationRules,
			state, result.ActorID, input.Days)
		modifiers := cultivation.TrainingModifiers{
			Talent:      xiuxianTalentFromState(state, result.ActorID),
			Quality:     quality,
			Environment: xiuxianTrainingEnvironment(state),
			Resource:    resource,
			// The body condition is the character's real health fraction, read
			// after the turn's own operations. A wounded character trains slower,
			// and one badly enough hurt does not train at all.
			HealthFraction: healthFraction(attributes.Health, attributes.HealthMax),
			// Exactly one selected, mastered method supplies the coefficient.
			// Zero preserves the legacy quality fallback without inventing a method.
			GrowthCoefficient: growth,
		}
		progress := cultivation.AdvanceCultivationFrom(character.Cultivation, character.Accumulated, supportedDays, modifiers)
		if supportedDays < input.Days && !progress.StoppedAtBoundary {
			// The stock ran out, so the rest of the interval is ordinary training.
			// It starts from the position the supported segment reached, which is
			// what makes one interval equal the same interval split in two.
			//
			// A segment that hit the bottleneck ended the whole interval: the
			// character cannot train past it, so the unsupported remainder must
			// not be spent and no further dose is owed.
			ordinary := modifiers
			ordinary.Resource = cultivation.SupportNone
			rest := cultivation.AdvanceCultivationFrom(progress.Cultivation, progress.Accumulated, input.Days-supportedDays, ordinary)
			rest.Gained += progress.Gained
			rest.StagesAdvanced += progress.StagesAdvanced
			rest.StoppedAtBoundary = rest.StoppedAtBoundary || progress.StoppedAtBoundary
			// The supported segment spent all of its own days unless it hit the
			// bottleneck, so the total is the two answers added, not the second
			// one alone.
			rest.ConsumedDays += progress.ConsumedDays
			progress = rest
		}
		result.Trained = progress.Gained > 0
		result.ConsumedDays = progress.ConsumedDays
		result.StagesAdvanced = progress.StagesAdvanced
		result.StoppedAtBoundary = progress.StoppedAtBoundary
		character.Cultivation = progress.Cultivation
		character.Accumulated = progress.Accumulated
		// Only the days the rules really spent are supported, so a character
		// stopped at a bottleneck does not pay for doses that covered time they
		// never trained. The plan already truncated the stock to the named
		// interval; this truncates it to the consumed part.
		if progress.ConsumedDays < supportedDays {
			_, supportDoses, _ = xiuxianPlanTrainingSupport(system.CultivationRules,
				state, result.ActorID, progress.ConsumedDays)
		}
		// The carried record is written in the same commit as the advance, so a
		// dose either paid for trained days or was never consumed.
		result.SupportOps = xiuxianTrainingSupportOps(state, result.ActorID, supportDoses, turnID)
	} else {
		// No training interval was named, so no training time was spent. An
		// authorised attempt still takes its own days, which IntervalDays reports.
		result.ConsumedDays = 0
	}

	// Burden relief and residual regulation come after the integration, because
	// only the part of the interval the rules really spent counts as time the
	// character spent. A character stopped at a major-realm bottleneck did not
	// sit in seclusion for the rest of the year they named, and the body does not
	// recover as though they had.
	burden := character.BurdenState()
	burden = cultivation.RecoverBurdenOffBattle(burden, result.ConsumedDays*24*60)
	if cleared, ok := clearXiuxianResidual(character, burden, result.ConsumedDays); ok {
		burden = cleared
		result.ResidualCleared = true
	}

	target := cultivation.BreakthroughTarget(strings.TrimSpace(input.AttemptBreakthrough))
	if target != "" {
		result.BreakthroughRequested = true
		attempted := xiuxianBreakthrough(system.CultivationRules,
			state, character, target, facts, identity, result.ActorID, method)
		result.BreakthroughEligible = attempted.Eligible
		result.BreakthroughReasons = attempted.Reasons
		result.BreakthroughRolled = attempted.Rolled
		result.BreakthroughChance = attempted.Chance
		result.BreakthroughSucceeded = attempted.Succeeded
		result.BreakthroughRecoveryDays = attempted.RecoveryDays
		if attempted.Eligible {
			result.BreakthroughDays = attempted.Days
			character.Cultivation = attempted.Outcome.Cultivation
			character.Accumulated = attempted.Outcome.Accumulated
			character.Health = attempted.Outcome.Health
			character.Mana = attempted.Outcome.Mana
		}
	}

	result.RealmAfter = cultivation.StageKey(character.Cultivation)
	result.RealmProgressAfter = character.Accumulated

	// Only the stored position is written. The stage key and the banked progress
	// are the rules' own facts; the pools moved only if a real cost or wound did.
	ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldRealm, result.RealmAfter, turnID))
	ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldRealmProgress, character.Accumulated, turnID))
	if actorStateTemplateDeclaresField(template, xiuxianFieldBurden) {
		ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldBurden, burden.Current, turnID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldBurdenFloor) {
		ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldBurdenFloor, burden.Residual, turnID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldHealth) {
		ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldHealth, character.Health, turnID))
	}
	if actorStateTemplateDeclaresField(template, xiuxianFieldMana) {
		ops = append(ops, xiuxianAdvanceOp(result.ActorID, xiuxianFieldMana, character.Mana, turnID))
	}
	// The consumed doses are written with the position, not after it: §6.2 line
	// 232 requires the resource to be really deducted, and a stock write in a
	// separate transaction could survive while the advance it paid for did not.
	ops = append(ops, result.SupportOps...)
	return xiuxianChangedOps(state, result.ActorID, ops), result, true
}

// IntervalDays is the real time this advance spent, in days.
//
// It is one number on purpose. The named training interval and the authorised
// breakthrough's own duration are both intervals of the same formal clock, so the
// caller advances the story by whichever of them the rules actually paid for — a
// 筑基 attempt is seven days even when the player named an hour, and a character
// stopped at a major-realm bottleneck does not spend the rest of the interval
// they asked for.
func (result CultivationAdvanceResult) IntervalDays() float64 {
	if result.BreakthroughDays > result.ConsumedDays {
		return result.BreakthroughDays
	}
	return result.ConsumedDays
}

// xiuxianBreakthroughAttempt is the internal outcome of one authorised attempt.
type xiuxianBreakthroughAttempt struct {
	Eligible     bool
	Reasons      []string
	Rolled       bool
	Chance       float64
	Succeeded    bool
	RecoveryDays float64
	// Days is the real time the attempt takes, from the rules' own table.
	Days    float64
	Outcome cultivation.BreakthroughOutcome
}

// xiuxianBreakthrough resolves an authorised major breakthrough.
//
// A missing precondition means no roll and no cost, which is why eligibility is
// checked before the die is cast: the design is explicit that an attempt whose
// conditions are not met is not made at all, rather than made and failed. A failed
// attempt carries real penalties, so rolling one that should not have happened
// would take real progress from the player.
//
// The die is derived from the command identity rather than drawn fresh, so a
// retried or refreshed request re-uses the same draw instead of re-rolling. That
// is the design's "同一命令不重掷", and it is why ResolveBreakthrough takes the
// roll as a caller-supplied value.
//
// facts carries the outstanding recovery a previous failed attempt imposed and
// the bonds the character actually holds. Both are formal facts the caller reads
// from committed records, so a retry cannot walk past the recovery by naming
// another attempt and a sentence cannot manufacture a guardian.
func xiuxianBreakthrough(cultivationRules *cultivation.Rules, state map[string]any,
	character cultivation.CharacterState,
	target cultivation.BreakthroughTarget,
	facts xiuxianFormalFacts,
	identity DomainCommitIdentity,
	actorID string,
	selected ...*cultivation.TechniqueDef,
) xiuxianBreakthroughAttempt {
	var method *cultivation.TechniqueDef
	if len(selected) > 0 {
		method = selected[0]
	}
	if method != nil {
		if !cultivationMethodSupportsTarget(*method, target) {
			return xiuxianBreakthroughAttempt{Reasons: []string{"所选功法不支持目标境界，未尝试、未扣费。"}}
		}
	}
	if method == nil && spiritRealmBreakthrough(target) {
		return xiuxianBreakthroughAttempt{Reasons: []string{"突破灵界高阶境界须有已经掌握、运转且支持目标境界的完整功法。"}}
	}
	attributes := character.Attributes()
	requirement := cultivation.CanAttemptBreakthrough(character.Cultivation, character.Accumulated, attributes, target, facts.Convalescing)
	if !requirement.Eligible {
		return xiuxianBreakthroughAttempt{Eligible: false, Reasons: requirement.Reasons}
	}
	modifiers := cultivation.BreakthroughModifiers{
		Talent:  xiuxianTalentFromState(state, actorID),
		Quality: character.Quality,
	}
	if method != nil {
		modifiers.Quality = method.Quality
	}
	// The three favourable conditions are read from committed facts, never from the
	// request. §7.1 is explicit that a claimed guardian or cave dwelling earns
	// nothing: "具体药名／配方和护法资格须以世界实际内容实现，不凭一句'有护法'获得加成".
	// The module's own booleans are the player's intent; these readers are the
	// truth, and a request that asserts a bonus without the fact behind it is a
	// sentence rather than a condition.
	modifiers.AuxiliaryMedicine = xiuxianBreakthroughMedicine(cultivationRules,
		state, actorID, target)
	modifiers.StableSite = xiuxianBreakthroughSite(state)
	modifiers.QualifiedGuardian = xiuxianBreakthroughGuardian(state, facts.Relationships)
	chance, ok := cultivation.BreakthroughChance(target, modifiers, cultivationRules)
	if !ok {
		return xiuxianBreakthroughAttempt{Eligible: false, Reasons: []string{"目标境界未实现"}}
	}
	roll := cultivationBreakthroughRoll(identity, actorID)
	outcome, ok := cultivation.ResolveBreakthrough(character.Cultivation, character.Accumulated, attributes, target, modifiers, roll)
	if !ok {
		return xiuxianBreakthroughAttempt{Eligible: false, Reasons: []string{"目标境界未实现"}}
	}
	// Preparation consumes the old stage's mana before the attempt resolves. The
	// design states the cost as 80% of the previous stage's mana maximum, rounded
	// up, and it is spent whether or not the attempt succeeds.
	if requirement.ManaCost > 0 && outcome.Mana > requirement.ManaCost {
		outcome.Mana -= requirement.ManaCost
	} else if requirement.ManaCost > 0 {
		outcome.Mana = 0
	}
	return xiuxianBreakthroughAttempt{
		Eligible:     true,
		Rolled:       true,
		Chance:       chance,
		Succeeded:    outcome.Succeeded,
		RecoveryDays: outcome.RecoveryDays,
		Days:         requirement.Days,
		Outcome:      outcome,
	}
}

// cultivationBreakthroughRoll derives one attempt's random draw from the command
// identity.
//
// The derivation mirrors deterministicPlayerInputID: the same command, operation,
// cycle and character always produce the same draw, so a retried request cannot
// re-roll. A fresh command is a new draw, which is what makes a second, genuinely
// separate attempt a real gamble rather than a replay of the first.
func cultivationBreakthroughRoll(identity DomainCommitIdentity, actorID string) float64 {
	sum := sha256.Sum256([]byte(
		identity.CommandID + "\x00" + identity.OperationID + "\x00" +
			fmt.Sprint(identity.Cycle) + "\x00" + actorID + "\x00breakthrough",
	))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(1<<64)
}

// clearXiuxianResidual regulates away a meridian residual injury when the
// interval and the body conditions actually allow it.
//
// The design lists the conditions together and requires all of them: the full
// continuous safe interval, at least 60% health, at least 20% mana, and no real
// injury or seal blocking regulation. Splitting the same interval into several
// requests must not bypass it, so the caller passes the interval it actually
// observed rather than accumulating fragments.
//
// Clearing the residual also relieves ordinary burden to zero, and that relief IS
// the effect of the interval: the same minutes are not counted twice as ordinary
// per-minute recovery. Health and mana are not refilled.
func clearXiuxianResidual(character cultivation.CharacterState, burden cultivation.BurdenState, days float64) (cultivation.BurdenState, bool) {
	if burden.Residual <= 0 || days <= 0 {
		return burden, false
	}
	skill, ok := residualSkillForFloor(burden.Residual)
	if !ok {
		// A residual floor no registered technique declares. Keeping it is the
		// conservative answer: the rules cannot name the interval that would clear
		// an injury they cannot identify.
		return burden, false
	}
	attributes := character.Attributes()
	if !cultivation.CanClearResidual(skill, days*24*60, attributes, false) {
		return burden, false
	}
	return cultivation.ClearResidual(burden), true
}

// residualSkillForFloor finds the registered technique that declares a given
// residual floor.
//
// The floor is the stored trace a residual injury leaves, so it is what
// identifies which technique's regulation conditions apply. A floor no technique
// declares has no known conditions, and the caller keeps the injury rather than
// guessing an interval.
func residualSkillForFloor(floor int) (cultivation.SkillDef, bool) {
	for _, skill := range cultivation.SkillTable() {
		if skill.LeavesResidual && skill.ResidualBurdenFloor == floor {
			return skill, true
		}
	}
	return cultivation.SkillDef{}, false
}

// xiuxianTalentFromState reads the character's aptitude out of the free-text
// foundation field.
//
// The field is prose because a spiritual root has a name and a description, but
// the rules need the enumerated aptitude. Like the specialisation and grade
// readers, an unreadable value is reported as the ordinary aptitude rather than
// guessed: a character whose root the story has not characterised trains at the
// ordinary rate, which is the same answer the parameter set gives for 普通.
func xiuxianTalentFromState(state map[string]any, actorID string) cultivation.Talent {
	for _, fieldID := range []string{"cultivation.foundation", "cultivation.profile", "cultivation.threat_profile"} {
		if text, ok := actorStateFieldValue(state, actorID, fieldID).(string); ok {
			if talent, found := normalizeCultivationTalentText(text); found {
				return talent
			}
		}
	}
	return cultivation.TalentNormal
}

// normalizeCultivationTalentText reads an aptitude out of prose.
//
// It accepts both the enumerated wording and the published Chinese terms, because
// the foundation field is written in the story's own language. Longer wordings are
// tested first so 较差 cannot be matched as 较 and 极佳 cannot be shadowed by a
// shorter term.
func normalizeCultivationTalentText(text string) (cultivation.Talent, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	byWording := []struct {
		wording string
		talent  cultivation.Talent
	}{
		{"极佳", cultivation.TalentExceptional},
		{"优秀", cultivation.TalentExcellent},
		{"良好", cultivation.TalentGood},
		{"较差", cultivation.TalentPoor},
		{"普通", cultivation.TalentNormal},
		{"exceptional", cultivation.TalentExceptional},
		{"excellent", cultivation.TalentExcellent},
		{"good", cultivation.TalentGood},
		{"poor", cultivation.TalentPoor},
		{"normal", cultivation.TalentNormal},
	}
	for _, candidate := range byWording {
		if strings.Contains(text, candidate.wording) {
			return candidate.talent, true
		}
	}
	return "", false
}

// xiuxianTrainingEnvironment reads the spiritual quality of the place the
// character is actually standing in, from committed scene facts.
//
// §六 prices 贫乏 and 稳定灵地 as real places, so the scale must come from where
// the character is rather than from a word the model put in the request. The
// story has no registered site-quality field yet, so the scene's own location
// text is the only committed evidence: a place the prose calls a barren or
// corrupted ground trains poorly, and one it names as a qualified spiritual
// site trains well. Everything else is ordinary.
//
// This is deliberately conservative. A word the narrative wrote into 当前详细地点
// is a committed fact about the scene; a bonus claimed in a request is not.
func xiuxianTrainingEnvironment(state map[string]any) cultivation.Environment {
	location := strings.ToLower(storySceneText(state, storySceneLocationField))
	if location == "" {
		return cultivation.EnvironmentOrdinary
	}
	for _, wording := range []string{"贫瘠", "贫乏", "荒芜", "死地", "煞气", "污秽", "绝灵", "barren", "corrupted", "blighted"} {
		if strings.Contains(location, wording) {
			return cultivation.EnvironmentPoor
		}
	}
	for _, wording := range []string{"灵地", "灵脉", "洞府", "洞天", "福地", "灵眼", "spiritual_site", "spiritual site"} {
		if strings.Contains(location, wording) {
			return cultivation.EnvironmentStable
		}
	}
	return cultivation.EnvironmentOrdinary
}

// xiuxianBreakthroughSite reports whether the character is standing somewhere
// §7.1's "稳定适用洞府" describes.
//
// It reuses the training environment reader, which is the same committed scene
// fact §六 prices the training rate from: a place the prose names as a qualified
// spiritual site or cave dwelling is one place, and both rules read it the same
// way. A stable site is the middle band or better; a barren ground is not a cave
// dwelling whatever the request says.
func xiuxianBreakthroughSite(state map[string]any) bool {
	return xiuxianTrainingEnvironment(state) == cultivation.EnvironmentStable
}

// xiuxianBreakthroughMedicine reports whether one real breakthrough pill exists
// and is spent by this attempt.
//
// §7.1's bonus is "选用该境有效辅助丹药的加成", and its own footnote requires the
// name, the recipe and the eligibility to come from real world content. The
// first-version catalogue registers only restoring and attack-boosting pills, so
// no dose can support a breakthrough yet and this reader answers false. It is
// written as a catalogue query rather than a bare false so that registering such
// a pill is enough to make the bonus real, and so a story that merely asserts
// one cannot collect it.
func xiuxianBreakthroughMedicine(cultivationRules *cultivation.Rules, state map[string]any, actorID string, target cultivation.BreakthroughTarget) bool {
	for _, dose := range xiuxianCarriedDoses(cultivationRules,
		state, actorID) {
		if dose.Quantity > 0 && xiuxianDoseSupportsBreakthrough(dose.Pill, target) {
			return true
		}
	}
	return false
}

// xiuxianDoseSupportsBreakthrough reports whether a registered pill is an
// effective auxiliary for one breakthrough.
//
// The catalogue has no such purpose yet, so the predicate is false for every
// definition it currently holds. It exists so the bonus is decided by the
// catalogue's own vocabulary rather than by a request.
func xiuxianDoseSupportsBreakthrough(pill cultivation.PillDef, target cultivation.BreakthroughTarget) bool {
	switch pill.Purpose {
	case cultivation.PillRestoreHealth, cultivation.PillRestoreMana, cultivation.PillBoostAttack,
		cultivation.PillSupportCultivation:
		// A pool restorative, an attack booster or an ordinary training aid is not
		// a breakthrough auxiliary. §7.1's bonus names a pill registered for that
		// realm specifically, and the catalogue holds none yet.
		return false
	}
	return false
}

// xiuxianBreakthroughGuardian reads the existing close-companion bonus: a
// currently qualifying bond and presence in the scene. It does not establish a
// separate decision to assist with this particular attempt.
//
// §7.1 requires the qualification to be a real thing rather than a claim, so this
// reads two committed facts together: the scene's own 在场角色 list, and the
// branch's formal relationship projection. A sworn or partnered companion standing
// in the scene is the case the rules can actually establish — a bond that was
// expressed and answered — and a bystander the prose called helpful has no such
// record. The relationships come from the projection rather than from the state
// map because that is where the formal record is owned; the state map carries no
// relationship history of its own.
func xiuxianBreakthroughGuardian(state map[string]any, relationships []character.Relationship) bool {
	present := storySceneFieldList(storyContextActorState(state), storyScenePresentField)
	if len(present) == 0 || len(relationships) == 0 {
		return false
	}
	for _, name := range present {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// The journal retains former bonds. Eligibility follows only the
		// current one, so a parting or hostile turn revokes the old standing.
		if relationship, ok := character.CurrentRelationship(name, relationships); ok && xiuxianGuardianBond(relationship.Kind) {
			return true
		}
	}
	return false
}

// xiuxianGuardianBond reports whether a formal relationship is close enough to
// stand beside someone through a breakthrough.
func xiuxianGuardianBond(kind character.RelationshipKind) bool {
	switch kind {
	case character.RelationshipSworn, character.RelationshipPartner, character.RelationshipCloseFriend:
		return true
	default:
		return false
	}
}

// xiuxianAdvanceOp builds one write to the stored rules position.
//
// Like the settlement's writes these are absolute values, because each one is
// derived from the rules rather than accumulated from the narrative, and a delta
// would drift away from the stage table.
func xiuxianAdvanceOp(actorID, fieldID string, value any, turnID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       "由授权的修炼时长与突破判定结算，不由叙述改写。",
		SourceTurnID: turnID,
		SourceKind:   StateOpSourceCultivationAdvance,
		SourceID:     "cultivation",
	}
}

// removeCultivationAdvanceActorOps drops a previous advance's writes.
//
// A reroll replaces the rule outcome the advance was derived from, so the advance
// has to be discarded with it and re-derived from the replacement. Keeping it
// would leave a stage the new outcome never reached.
func removeCultivationAdvanceActorOps(ops []ActorStateOp) []ActorStateOp {
	if len(ops) == 0 {
		return nil
	}
	out := make([]ActorStateOp, 0, len(ops))
	for _, op := range ops {
		if op.SourceKind == StateOpSourceCultivationAdvance {
			continue
		}
		out = append(out, op)
	}
	return normalizeActorStateOps(out)
}

func healthFraction(health, healthMax int) float64 {
	if healthMax <= 0 {
		return 0
	}
	return float64(health) / float64(healthMax)
}

// turnIdentity is the command identity a turn was committed under.
//
// It is what binds a breakthrough's random draw to the command rather than to the
// moment of execution, so a retried or refreshed request re-uses the same draw
// instead of re-rolling.
func turnIdentity(turn TurnEvent) DomainCommitIdentity {
	return DomainCommitIdentity{
		CommandID:   strings.TrimSpace(turn.AgentCommandID),
		OperationID: strings.TrimSpace(turn.AgentOperationID),
		Cycle:       turn.AgentCycle,
	}
}
