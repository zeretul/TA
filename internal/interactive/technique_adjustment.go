package interactive

import (
	"fmt"
	"ta/internal/cultivation"
)

// Preparation is paid once per adjusted active set, not once per method.
// A stopped warm/cold method remembers its unsettled circulation across turns.
func techniqueAdjustmentSeconds(branch *storyBranchProjection, state map[string]any, actorID string, result TechniqueResult) (int64, bool) {
	if len(result.Started) == 0 {
		return 0, false
	}
	records, _ := readTechniqueRecords(state, actorID)
	seconds := int64(600)
	warmCold := false
	for _, started := range result.Started {
		for _, old := range records {
			if !old.Active && !old.SwitchPending {
				continue
			}
			previous, ok := resolveTechnique(branch.cultivationRules(),
				old.DefID, old.Version, state)
			if !ok {
				continue
			}
			pair := (previous.Circulation == cultivation.CirculationWarm && started.Circulation == cultivation.CirculationCold) || (previous.Circulation == cultivation.CirculationCold && started.Circulation == cultivation.CirculationWarm)
			if !pair {
				continue
			}
			// A compatible simultaneous warm/cold set still pays normal preparation.
			stopped := !old.Active
			for _, def := range result.Stopped {
				if def.ID == old.DefID {
					stopped = true
				}
			}
			if !stopped {
				continue
			}
			warmCold = true
			paid := false
			if !old.Active && branch != nil {
				for _, recovery := range branch.Recoveries {
					if normalizeStatePanelActorID(recovery.ActorID) == actorID && recovery.Mode == cultivation.RecoverySafeMeditation && recovery.Eligibility == cultivation.RecoveryQualified && recovery.ClockSeconds >= old.StoppedAtSeconds && int64(recovery.Activity.CompletedEffective) >= 3600 {
						paid = true
						break
					}
				}
			}
			if !paid {
				return 3600, true
			}
			seconds = 0 // The qualifying sixty minutes already include preparation.
		}
	}
	return seconds, warmCold
}

func techniquePreparationRecovery(branch *storyBranchProjection, system StoryDirectorActorStateSystem, state map[string]any, actorID, turnID string, seconds int64, requireZero bool) ([]ActorStateOp, error) {
	view := recoveryViewAt(branch, state)
	if view == nil {
		view = &storyBranchProjection{State: state}
	}
	eligibility, err := recoveryEligibilityFor(view, actorID, "")
	if err != nil {
		return nil, err
	}
	if eligibility != cultivation.RecoveryQualified {
		return nil, fmt.Errorf("circulation adjustment requires a safe, living character outside battle")
	}
	if seconds <= 0 {
		if requireZero {
			c, err := xiuxianCharacterFromState(system.CultivationRules,
				state, actorID)
			if err != nil {
				return nil, err
			}
			if c.BurdenState().Current != 0 || c.BurdenState().Residual != 0 {
				return nil, fmt.Errorf("warm/cold change requires zero meridian burden")
			}
		}
		return nil, nil
	}
	origin := recoveryCurrentAttributes(view, actorID)
	candidate, err := cultivation.PlanSafeRecovery(cultivation.RecoveryRequest{Rules: system.CultivationRules, ActivityID: "technique-" + turnID, Mode: cultivation.RecoverySafeMeditation, EligibilityAtStart: eligibility, EligibilityNow: eligibility, StartAttributes: origin, CurrentAttributes: origin, TargetSeconds: seconds})
	if err != nil {
		return nil, err
	}
	ops, err := recoveryPoolWriteOps(state, system, actorID, candidate, turnID)
	if err != nil {
		return nil, err
	}
	before, after, cleared, err := planMeditationBurden(view, actorID, cultivation.RecoverySafeMeditation, seconds, eligibility)
	if err != nil {
		return nil, err
	}
	if requireZero && (after.Current != 0 || after.Residual != 0) {
		return nil, fmt.Errorf("warm/cold change cannot start while meridian burden remains")
	}
	burdenOps, err := recoveryBurdenOps(state, system, actorID, before, after, cleared, turnID)
	if err != nil {
		return nil, err
	}
	ops = append(ops, burdenOps...)
	for i := range ops {
		ops[i].SourceKind = StateOpSourceTechniqueLearning
		ops[i].SourceID = "technique_preparation"
	}
	return ops, nil
}

func annotateTechniqueAdjustment(cultivationRules *cultivation.Rules, ops []ActorStateOp, actorID string, stoppedAt int64, result TechniqueResult, completed bool, settledWarmCold bool, state map[string]any) {
	for i := range ops {
		if ops[i].ActorID != actorID || ops[i].FieldID != normalizeActorStateFieldName(xiuxianFieldTechniques) {
			continue
		}
		entries, _ := ops[i].Value.(map[string]any)
		for _, value := range entries {
			entry, _ := value.(map[string]any)
			id := xiuxianEntryText(entry, "定义")
			for _, def := range result.Stopped {
				if id == def.ID {
					entry["待平息"] = true
					entry["停运秒"] = float64(stoppedAt)
				}
			}
			clear := settledWarmCold
			old, _ := resolveTechnique(cultivationRules,
				id, 0, state)
			for _, started := range result.Started {
				if old.Circulation == started.Circulation {
					clear = true
				}
			}
			if completed && len(result.Started) > 0 && clear {
				delete(entry, "待平息")
				delete(entry, "停运秒")
			}
		}
	}
}
