package interactive

import (
	"fmt"
	"ta/internal/character"
)

// The lightweight character preflight uses a sentinel time and cannot prove a
// visual observation. Visual submissions instead run the real formal-turn
// projector against accepted state changes. The final commit runs it again.
func validateTurnCharacterFactsWithVisuals(input *TurnCharacterFactsInput, validation TurnSubmissionContext, prepared *PreparedTurnSubmission, clockSeconds int64) *TurnSubmissionDiagnostic {
	hasVisual := false
	if input != nil {
		for _, k := range input.Knowledge {
			if k.Visual != nil {
				hasVisual = true
			}
		}
	}
	if !hasVisual {
		return validateTurnCharacterFactsModule(input, validation.KnownCharacters, validation.Scene, validation.ExistingAffinities, validation.ExistingRelationships, validation.ExistingMatters, clockSeconds)
	}
	fail := func(err error) *TurnSubmissionDiagnostic {
		return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule, "/character_facts/knowledge/visual", "actual same-scene contact, recognized identity and visible form with accepted state sources", err.Error(), "观察尚无完整事实来源。不得为配图创造会面、身份、地点或形态；补实际发生的来源，或省略visual。")
	}
	// Preserve the ordinary module's introduction and matter checks. This local
	// copy is only structural preflight, never the committed output.
	structural := cloneTurnCharacterFactsInput(input)
	for i := range structural.Knowledge {
		structural.Knowledge[i].Visual = nil
	}
	known := append(append([]string{}, validation.KnownCharacters...), DefaultActorID)
	if d := validateTurnCharacterFactsModule(structural, known, validation.Scene, validation.ExistingAffinities, validation.ExistingRelationships, validation.ExistingMatters, clockSeconds); d != nil {
		return d
	}
	if !prepared.stateUpdatesAccepted {
		return fail(fmt.Errorf("state_changes must be accepted before visual knowledge"))
	}
	compiled, err := CompileTurnStateUpdates(validation.ActorState, validation.CurrentState, prepared.result.StateUpdates, TurnStateUpdateCompileOptions{})
	if err != nil {
		return fail(err)
	}
	post := cloneStoryState(validation.CurrentState)
	for _, op := range compiled.Ops {
		applyStateOp(post, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(post, op)
	}
	state := character.State{Profiles: validation.ExistingProfiles, Matters: validation.ExistingMatters, Affinities: validation.ExistingAffinities, Relationships: validation.ExistingRelationships, FactionMemberships: validation.ExistingFactionMemberships}
	projection := &storyBranchProjection{Characters: &state, PackCatalog: validation.Meta.PackCatalog, State: validation.CurrentState}
	projection.bindWorld(validation.Meta)
	if validation.WorldView != nil {
		projection.Clock = cloneStoryClock(validation.WorldView.Clock)
	}
	storyID := validation.Meta.StoryID
	if storyID == "" {
		storyID = turnSubmissionPendingIdentity
	}
	branchID := validation.BranchID
	if branchID == "" {
		branchID = turnSubmissionPendingIdentity
	}
	turn := TurnEvent{ID: turnSubmissionPendingIdentity, User: validation.CurrentPlayerInput, TurnResult: &TurnResult{CharacterFacts: input}}
	_, err = buildTurnCharacterRecord(storyID, branchID, turn, projection, post, validation.Meta.Calendar, nil, validation.Meta.PackBinding)
	if err != nil {
		return fail(err)
	}
	return nil
}
