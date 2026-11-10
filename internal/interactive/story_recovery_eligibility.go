package interactive

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
)

// recoveryEligibilityFor derives the safety finding for one character from the
// branch's own committed facts.
//
// §三.4 is explicit that eligibility comes from formal state and the scene, not
// from a frontend or a model reporting safe=true. So the derivation reads only
// things the story itself committed:
//
//   - A live, unfinished battle on this branch means the character is in battle.
//     branch.Combats is a projection of the canonical journal, so a battle that
//     was closed out of the window but never committed does not make anyone
//     unsafe, and one that was committed does.
//
//   - A pool at or below zero is a real injury the story recorded. §三.4 is
//     equally explicit that zero health is not this function's death verdict —
//     a legally surviving, incapacitated character may still be resting — so it
//     is reported as needing special treatment rather than as dead.
//
// declared may only make the answer stricter, never weaker. It exists for a
// creator-marked condition the store cannot derive, such as a seal the story
// wrote in prose; a caller cannot use it to claim a safety the facts deny.
func recoveryEligibilityFor(projection *storyBranchProjection, actorID string, declared cultivation.RecoveryEligibility) (cultivation.RecoveryEligibility, error) {
	if declared != "" && !validRecoveryEligibilityValue(declared) {
		return "", cultivation.ErrRecoveryUnknownEligibility
	}
	derived := deriveRecoveryEligibility(projection, actorID)
	if recoveryEligibilityRank(declared) > recoveryEligibilityRank(derived) {
		return declared, nil
	}
	return derived, nil
}

// recoveryEligibilityRank orders findings by how much they block ordinary safe
// recovery. A higher rank is stricter, so a declared finding can only ever raise
// the answer.
func recoveryEligibilityRank(eligibility cultivation.RecoveryEligibility) int {
	switch eligibility {
	case cultivation.RecoveryQualified:
		return 0
	case cultivation.RecoveryNeedsSpecialTreatment:
		return 1
	case cultivation.RecoveryInDanger:
		return 2
	case cultivation.RecoveryInBattle:
		return 3
	case cultivation.RecoveryDead:
		return 4
	default:
		return 0
	}
}

// deriveRecoveryEligibility reads the safety finding out of committed state.
func deriveRecoveryEligibility(projection *storyBranchProjection, actorID string) cultivation.RecoveryEligibility {
	if projection == nil {
		return cultivation.RecoveryInDanger
	}
	// A battle that is committed and not yet ended means this character is
	// fighting. §三.4 refuses ordinary safe recovery in battle, and the check has
	// to be against the journal's own record of the fight rather than a flag the
	// window passes in.
	for _, session := range projection.Combats {
		if session.Ended {
			continue
		}
		for _, combatant := range session.Combatants {
			if normalizeStatePanelActorID(combatant.ID) != actorID {
				continue
			}
			if combatant.Attributes.Health <= 0 {
				// Down in a live fight is not ordinary rest either. It is reported
				// as needing special treatment, which is the honest answer: the
				// rules have no safe-rest path back from it.
				return cultivation.RecoveryNeedsSpecialTreatment
			}
			return cultivation.RecoveryInBattle
		}
	}
	attributes := recoveryCurrentAttributes(projection, actorID)
	if attributes.Health <= 0 {
		// Zero health is a real recorded injury, not a death verdict. §三.4 keeps
		// that distinction so an incapacitated but living character is not
		// silently treated as gone.
		return cultivation.RecoveryNeedsSpecialTreatment
	}
	return cultivation.RecoveryQualified
}

// recoveryCurrentAttributes reads the character's current pools from committed
// state.
//
// It is deliberately the same reader the settlement uses, so the number the
// recovery projects from and the number the panel shows cannot disagree.
func recoveryCurrentAttributes(projection *storyBranchProjection, actorID string) cultivation.Attributes {
	if projection == nil {
		return cultivation.Attributes{}
	}
	character, err := xiuxianCharacterFromState(projection.cultivationRules(),
		projection.State, actorID)
	if err != nil {
		return cultivation.Attributes{}
	}
	attributes := character.Attributes()
	// A pool that has never been written means no wound has been recorded, so it
	// starts full rather than at zero. This mirrors settleXiuxianActorState's own
	// reading, and the two must agree or the recovery would project from a
	// different starting pool than the settlement publishes.
	if _, ok := actorStateNumber(actorStateFieldValue(projection.State, actorID, xiuxianFieldHealth)); !ok {
		character.Health = attributes.HealthMax
	}
	if _, ok := actorStateNumber(actorStateFieldValue(projection.State, actorID, xiuxianFieldMana)); !ok {
		character.Mana = attributes.ManaMax
	}
	return character.Attributes()
}

// recoverySettlementOps builds the Actor State writes for one settled interval.
//
// It writes the absolute pool PlanSafeRecovery projected, as its own source kind
// so a reader can tell a pool a rest restored from a pool a narrative described.
// The maxima and the four attributes are re-derived by the ordinary settlement
// on top of this write, so the panel, the battle and the recovery keep reading
// one set of numbers.
func recoverySettlementOps(state map[string]any, system StoryDirectorActorStateSystem, actorID string, candidate cultivation.RecoveryCandidate, eventID string) ([]ActorStateOp, error) {
	ops, err := recoveryPoolWriteOps(state, system, actorID, candidate, eventID)
	if err != nil {
		return nil, err
	}
	// The recovery moves the current pools, and the settlement re-derives the
	// maxima and the four attributes from the stage the character is still at.
	// Running it here rather than trusting the candidate's copy keeps the panel,
	// the battle and the recovery reading one set of numbers.
	projected := cloneStoryState(state)
	for _, op := range ops {
		applyActorStateOp(projected, op)
	}
	ops = append(ops, settleXiuxianStoryState(projected, system, eventID)...)
	return xiuxianChangedOps(state, actorID, ops), nil
}

// recoveryPoolWriteOps builds the absolute pool write alone, without the
// settlement that follows it.
//
// A turn settles its whole cast once, after every module has contributed its
// writes. A module that ran the settlement itself would settle twice: the second
// pass would re-derive the same maxima from the same stage and append a second
// copy of every attribute write, so the delta would carry the numbers twice.
func recoveryPoolWriteOps(state map[string]any, system StoryDirectorActorStateSystem, actorID string, candidate cultivation.RecoveryCandidate, eventID string) ([]ActorStateOp, error) {
	actorID = normalizeStatePanelActorID(actorID)
	if actorID == "" || !ActorStateSystemIsXiuxian(system) {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, actorID)
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}
	template := actorStateTemplateByID(system, templateID)
	if !actorStateTemplateIsXiuxian(template, system.CultivationRules) {
		return nil, fmt.Errorf("%w: %s", ErrRecoveryUnknownActor, actorID)
	}
	fieldID, err := recoveryPoolField(candidate.Mode)
	if err != nil {
		return nil, err
	}
	if !actorStateTemplateDeclaresField(template, fieldID) {
		return nil, fmt.Errorf("%w: template does not declare %s", ErrRecoveryUnknownActor, fieldID)
	}
	return []ActorStateOp{recoverySetOp(actorID, fieldID, candidate.CandidatePool, eventID)}, nil
}

// recoveryPoolField maps a recovery mode to the field it restores.
//
// The switch is exhaustive over the two modes the rules define, so a new mode is
// a compile-time absence here rather than a silently restored wrong pool.
func recoveryPoolField(mode cultivation.RecoveryMode) (string, error) {
	switch mode {
	case cultivation.RecoveryHealthRest:
		return xiuxianFieldHealth, nil
	case cultivation.RecoverySafeMeditation:
		return xiuxianFieldMana, nil
	default:
		return "", cultivation.ErrRecoveryUnknownMode
	}
}

// recoverySetOp builds one recovery write.
//
// It is an absolute value rather than a delta for the same reason the settlement
// uses absolutes: the number is what the cumulative projection says the pool is,
// and a delta would accumulate the difference between two projections and drift.
func recoverySetOp(actorID, fieldID string, value any, eventID string) ActorStateOp {
	return ActorStateOp{
		Op:           "set",
		ActorID:      actorID,
		FieldID:      normalizeActorStateFieldName(fieldID),
		Value:        value,
		Reason:       "由安全休养／调息的累计时长按规则取整结算，不由叙述改写。",
		SourceTurnID: eventID,
		SourceKind:   StateOpSourceSafeRecovery,
		SourceID:     "safe_recovery",
	}
}

// branchStoryClock reads the branch's formal time axis, establishing an origin
// from the story's own display text the first time one is needed.
//
// §三.2 permits exactly this: while the world calendar is only unbound free
// text, the batch may establish a versioned per-branch relative-second origin as
// the single formal calculation instant, keeping the original date text as a
// display anchor only. It is not a migration — nothing is written here — and it
// does not parse the anchor into arithmetic.
func branchStoryClock(projection *storyBranchProjection, actorID string, system StoryDirectorActorStateSystem) (StoryClock, error) {
	if projection != nil && projection.Clock != nil {
		if err := projection.Clock.Validate(); err != nil {
			return StoryClock{}, err
		}
		return *projection.Clock, nil
	}
	anchor := ""
	if projection != nil {
		anchor = storySceneText(projection.State, storySceneTimeField)
	}
	return NewStoryClock(anchor), nil
}

// RecoveryState reads one branch's formal clock and safe recovery activities.
//
// It is the read a rest panel opens on: a window that was closed and reopened
// has to find the same activity at the same settled interval, and a branch that
// never rested has to read as having no activity rather than as an error.
func (s *Store) RecoveryState(storyID, branchID string) (StoryClock, map[string]RecoveryActivity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return StoryClock{}, nil, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	if projection.Clock == nil {
		return NewStoryClock(storySceneText(projection.State, storySceneTimeField)), nil, nil
	}
	activities := make(map[string]RecoveryActivity, len(projection.Recoveries))
	for id, activity := range projection.Recoveries {
		activities[id] = activity
	}
	return *projection.Clock, activities, nil
}

// RecoveryAttributes reads one character's live pools and the derived safety
// finding without writing anything.
//
// The rest entry needs both before it can offer an interval, and §三.3 is
// explicit that opening the entry must not itself restore or advance anything,
// so this is a pure read of committed state.
func (s *Store) RecoveryAttributes(storyID, branchID, actorID string, declared cultivation.RecoveryEligibility) (cultivation.Attributes, cultivation.RecoveryEligibility, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return cultivation.Attributes{}, "", err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return cultivation.Attributes{}, "", err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return cultivation.Attributes{}, "", err
	}
	actorID = normalizeRecoveryActorID(actorID)
	eligibility, err := recoveryEligibilityFor(projection, actorID, declared)
	if err != nil {
		return cultivation.Attributes{}, "", err
	}
	return recoveryCurrentAttributes(projection, actorID), eligibility, nil
}

// DescribeRecoveryEligibilityRefusal renders the refusal a player sees.
//
// §三.6 asks for clear errors that do not turn a technical failure into a
// character's misfortune, so a refusal names the real condition rather than
// blaming the story.
func DescribeRecoveryEligibilityRefusal(eligibility cultivation.RecoveryEligibility) string {
	switch eligibility {
	case cultivation.RecoveryInBattle:
		return "战斗中无法安全休养或调息，先结束战斗。"
	case cultivation.RecoveryInDanger:
		return "当前处境危险，无法安全休养或调息。"
	case cultivation.RecoveryDead:
		return "角色已不在世，无法休养或调息。"
	case cultivation.RecoveryNeedsSpecialTreatment:
		return "伤势需要专门处理，普通休养或调息无法恢复。"
	default:
		return ""
	}
}

// normalizeRecoveryActorID trims an actor identity the same way the state panel
// does, so a recovery and the panel cannot address different characters.
func normalizeRecoveryActorID(actorID string) string {
	return normalizeStatePanelActorID(strings.TrimSpace(actorID))
}
