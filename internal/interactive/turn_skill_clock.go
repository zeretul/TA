package interactive

import (
	"fmt"
	"math"
	"strings"
	"ta/internal/storytime"
)

const turnClockActionSkill = "skill_study"

type turnSkillOutcome struct {
	ops    []ActorStateOp
	result SkillLearningResult
	clock  *TurnClockAdvance
}

func skillStudySeconds(before, after map[string]any, actorID string) (int64, error) {
	old, _ := xiuxianEquipmentEntry(before, actorID, xiuxianFieldLearnedSkills)
	next, _ := xiuxianEquipmentEntry(after, actorID, xiuxianFieldLearnedSkills)
	var seconds int64
	for key, value := range next {
		record, _ := value.(map[string]any)
		previous, _ := old[key].(map[string]any)
		delta := math.Round(skillRecordStudyMinutes(record)*60) - math.Round(skillRecordStudyMinutes(previous)*60)
		if delta >= float64(math.MaxInt64-seconds) {
			return 0, fmt.Errorf("skill study exceeds the supported clock range")
		}
		if delta > 0 {
			seconds += int64(delta)
		}
	}
	return seconds, nil
}

func planTurnSkillCommit(branch *storyBranchProjection, meta StoryMeta, system StoryDirectorActorStateSystem, state map[string]any, input *TurnSkillLearningInput, turnID, source string) (turnSkillOutcome, error) {
	if input == nil || input.Empty() {
		return turnSkillOutcome{}, nil
	}
	if d := validateTurnSkillLearningModule(input); d != nil {
		return turnSkillOutcome{}, fmt.Errorf("invalid skill study: %s", d.Message)
	}
	actorID := normalizeStatePanelActorID(input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	ops, result, _ := xiuxianSkillLearningOps(state, system, actorID, *input, turnID)
	after := cloneStoryState(state)
	for _, op := range ops {
		applyActorStateOp(after, op)
	}
	seconds, err := skillStudySeconds(state, after, actorID)
	if err != nil {
		return turnSkillOutcome{}, err
	}
	out := turnSkillOutcome{ops: ops, result: result}
	if seconds == 0 {
		return out, nil
	}
	clock := NewStoryClock(storySceneText(state, storySceneTimeField))
	if branch != nil && branch.Clock != nil {
		clock = *branch.Clock
	}
	if branch != nil {
		for _, combat := range branch.Combats {
			if !combat.Ended {
				for _, participant := range combat.Combatants {
					if normalizeStatePanelActorID(participant.ID) == actorID {
						return turnSkillOutcome{}, fmt.Errorf("finish the active battle before studying a skill")
					}
				}
			}
		}
		if node, ok := cultivationBoundaryWithin(branch, storytime.Instant(clock.Seconds), seconds, lifespanBoundary(meta, state, actorID)); ok {
			cut := int64(node.At) - clock.Seconds
			if cut >= 0 && cut < seconds {
				clipped, err := clipSkillStudyInput(state, system, actorID, input, cut, turnID)
				if err != nil {
					return turnSkillOutcome{}, err
				}
				out.ops, out.result, _ = xiuxianSkillLearningOps(state, system, actorID, clipped, turnID)
				out.result.Refused = append(out.result.Refused, SkillLearningRefusal{Reason: "研习到真实世界节点中止，只记录已过去的学习分钟。"})
				after = cloneStoryState(state)
				for _, op := range out.ops {
					applyActorStateOp(after, op)
				}
				seconds, err = skillStudySeconds(state, after, actorID)
				if err != nil {
					return turnSkillOutcome{}, err
				}
			}
		}
	}
	if seconds == 0 {
		return out, nil
	}
	next, err := AdvanceStoryClock(clock, seconds)
	if err != nil {
		return turnSkillOutcome{}, err
	}
	out.clock = &TurnClockAdvance{Action: turnClockActionSkill, SourceText: strings.TrimSpace(source), DurationSeconds: seconds, Before: clock, After: next}
	return out, nil
}

// Simulate each row in order. An earlier refused row must not consume the
// budget of a later accepted row merely because they name the same definition.
func clipSkillStudyInput(state map[string]any, system StoryDirectorActorStateSystem, actorID string, input *TurnSkillLearningInput, remaining int64, turnID string) (TurnSkillLearningInput, error) {
	clipped := *cloneTurnSkillLearningInput(input)
	clipped.Skills = nil
	current := cloneStoryState(state)
	for _, request := range input.Skills {
		evaluate := func(request TurnSkillLearning) (map[string]any, int64, error) {
			ops, _, _ := xiuxianSkillLearningOps(current, system, actorID, TurnSkillLearningInput{ActorID: actorID, Skills: []TurnSkillLearning{request}}, turnID)
			next := cloneStoryState(current)
			for _, op := range ops {
				applyActorStateOp(next, op)
			}
			seconds, err := skillStudySeconds(current, next, actorID)
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
		clipped.Skills = append(clipped.Skills, request)
	}
	return clipped, nil
}
