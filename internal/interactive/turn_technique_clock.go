package interactive

import (
	"fmt"
	"math"
	"strings"

	"ta/internal/storytime"
)

const turnClockActionTechnique = "technique_study"

// Technique study shares the existing turn clock and journal. Only minutes
// accepted by the learning rules count; rejected methods never buy progress or
// consume time. A real world boundary truncates both facts together.
type turnTechniqueOutcome struct {
	ops    []ActorStateOp
	result TechniqueResult
	clock  *TurnClockAdvance
}

func techniqueHasStudy(input *TurnTechniqueInput) bool {
	if input != nil {
		for _, request := range input.Learn {
			if request.Minutes > 0 {
				return true
			}
		}
	}
	return false
}

func techniqueIntervalConflict(result *TurnResult) bool {
	if result == nil {
		return false
	}
	count := 0
	if techniqueHasStudy(result.Techniques) || (result.Techniques != nil && len(result.Techniques.Circulate) > 0) {
		count++
	}
	if skillLearningHasStudy(result.Skills) {
		count++
	}
	if count == 0 {
		return false
	}
	if result.Cultivation != nil && !result.Cultivation.Empty() {
		count++
	}
	if result.Rest != nil && !result.Rest.Empty() {
		count++
	}
	if result.Craft != nil && !result.Craft.Empty() {
		count++
	}
	if result.World != nil && !result.World.Empty() {
		count++
	}
	return count > 1
}

func techniqueStudySeconds(before, after map[string]any, actorID string) (int64, error) {
	old, _ := readTechniqueRecords(before, actorID)
	next, _ := readTechniqueRecords(after, actorID)
	var seconds int64
	for key, record := range next {
		delta := math.Round(record.Minutes*60) - math.Round(old[key].Minutes*60)
		if math.IsNaN(delta) || math.IsInf(delta, 0) || delta >= float64(math.MaxInt64-seconds) {
			return 0, fmt.Errorf("technique study duration is outside the supported clock range")
		}
		if delta > 0 {
			seconds += int64(delta)
		}
	}
	return seconds, nil
}

func planTurnTechniqueCommit(branch *storyBranchProjection, meta StoryMeta, system StoryDirectorActorStateSystem, state map[string]any, input *TurnTechniqueInput, turnID, source string) (turnTechniqueOutcome, error) {
	if input == nil || input.Empty() {
		return turnTechniqueOutcome{}, nil
	}
	if diagnostic := validateTurnTechniqueModule(input); diagnostic != nil {
		return turnTechniqueOutcome{}, fmt.Errorf("invalid technique study: %s", diagnostic.Message)
	}
	actorID := normalizeStatePanelActorID(input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	ops, result, _ := xiuxianTechniqueOps(state, system, actorID, *input, turnID)
	for _, refusal := range result.Refused {
		if refusal.Action == "circulate" && len(input.Stop) > 0 {
			withoutSwitch := *cloneTurnTechniqueInput(input)
			withoutSwitch.Stop = nil
			withoutSwitch.Circulate = nil
			ops, result, _ = xiuxianTechniqueOps(state, system, actorID, withoutSwitch, turnID)
			result.Refused = append(result.Refused, refusal)
			break
		}
	}
	after := cloneStoryState(state)
	for _, op := range ops {
		applyActorStateOp(after, op)
	}
	seconds, err := techniqueStudySeconds(state, after, actorID)
	if err != nil {
		return turnTechniqueOutcome{}, err
	}
	out := turnTechniqueOutcome{ops: ops, result: result}
	preparation, warmCold := techniqueAdjustmentSeconds(branch, state, actorID, result)
	studySeconds := seconds
	if preparation > math.MaxInt64-seconds {
		return turnTechniqueOutcome{}, fmt.Errorf("technique interval overflow")
	}
	seconds += preparation
	// Even a separately paid preparation must recheck present safety and burden.
	if len(result.Started) > 0 {
		if _, err := techniquePreparationRecovery(branch, system, state, actorID, turnID, 0, false); err != nil {
			return turnTechniqueOutcome{}, err
		}
	}
	if branch != nil {
		for _, combat := range branch.Combats {
			if !combat.Ended {
				for _, participant := range combat.Combatants {
					if normalizeStatePanelActorID(participant.ID) == actorID {
						return turnTechniqueOutcome{}, fmt.Errorf("finish the active battle before studying a cultivation method")
					}
				}
			}
		}
	}
	clock := NewStoryClock(storySceneText(state, storySceneTimeField))
	if branch != nil && branch.Clock != nil {
		clock = *branch.Clock
	}
	interruptedPreparation := false
	if branch != nil {
		if node, ok := cultivationBoundaryWithin(branch, storytime.Instant(clock.Seconds), seconds, lifespanBoundary(meta, state, actorID)); ok {
			cut := int64(node.At) - clock.Seconds
			if cut >= 0 && cut < seconds {
				interruptedPreparation = len(result.Started) > 0
				clipped, err := clipTechniqueStudyInput(state, system, actorID, input, min(cut, studySeconds), turnID)
				if err != nil {
					return turnTechniqueOutcome{}, err
				}
				out.ops, out.result, _ = xiuxianTechniqueOps(state, system, actorID, clipped, turnID)
				after = cloneStoryState(state)
				for _, op := range out.ops {
					applyActorStateOp(after, op)
				}
				seconds, err = techniqueStudySeconds(state, after, actorID)
				if err != nil {
					return turnTechniqueOutcome{}, err
				}
				if seconds < studySeconds {
					out.result.Refused = append(out.result.Refused, TechniqueRefusal{Action: "learn", Reason: "study stopped at a real world boundary; only the elapsed minutes were learned"})
				}
				studySeconds = seconds
				preparation = max(int64(0), cut-studySeconds)
				seconds = cut
				if interruptedPreparation {
					out.result.Refused = append(out.result.Refused, TechniqueRefusal{Action: "circulate", Reason: "a real world boundary interrupted preparation; the old circulation remains unchanged"})
				}
			}
		}
	}
	if warmCold {
		recoveryOps, err := techniquePreparationRecovery(branch, system, state, actorID, turnID, preparation, !interruptedPreparation)
		if err != nil {
			return turnTechniqueOutcome{}, err
		}
		out.ops = append(out.ops, recoveryOps...)
	}
	annotateTechniqueAdjustment(cultivationRulesForMeta(meta),
		out.ops, actorID, clock.Seconds+studySeconds, out.result, !interruptedPreparation, warmCold, state)
	if seconds == 0 {
		return out, nil
	}
	next, err := AdvanceStoryClock(clock, seconds)
	if err != nil {
		return turnTechniqueOutcome{}, err
	}
	out.clock = &TurnClockAdvance{Action: turnClockActionTechnique, SourceText: strings.TrimSpace(source), DurationSeconds: seconds, Before: clock, After: next}
	return out, nil
}

func clipTechniqueStudyInput(state map[string]any, system StoryDirectorActorStateSystem, actorID string, input *TurnTechniqueInput, remaining int64, turnID string) (TurnTechniqueInput, error) {
	clipped := *cloneTurnTechniqueInput(input)
	clipped.Learn = nil
	clipped.Stop = nil
	clipped.Circulate = nil
	current := cloneStoryState(state)
	for _, request := range input.Learn {
		evaluate := func(request TurnTechniqueLearning) (map[string]any, int64, error) {
			ops, _, _ := xiuxianTechniqueOps(current, system, actorID, TurnTechniqueInput{ActorID: actorID, Learn: []TurnTechniqueLearning{request}}, turnID)
			next := cloneStoryState(current)
			for _, op := range ops {
				applyActorStateOp(next, op)
			}
			seconds, err := techniqueStudySeconds(current, next, actorID)
			return next, seconds, err
		}
		next, seconds, err := evaluate(request)
		if err != nil {
			return clipped, err
		}
		if seconds > remaining {
			request.Minutes = float64(remaining) / 60
			next, seconds, err = evaluate(request)
			if err != nil {
				return clipped, err
			}
		}
		remaining -= seconds
		current = next
		clipped.Learn = append(clipped.Learn, request)
	}
	return clipped, nil
}
