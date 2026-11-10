package interactive

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

func TestIter4CultivationContinuationKeepsChosenMethodWhenNotChanged(t *testing.T) {
	state := iter4MethodsState(t, "qi_6")
	clock := NewStoryClock("")
	clock.Seconds = 86400
	activity := CultivationActivity{ID: "chosen-low", ActorID: DefaultActorID, Conditions: CultivationConditions{MethodID: "method.basic_breathing"}, Eligibility: cultivation.RecoveryQualified,
		Activity: storytime.Activity{ID: "chosen-low", StoryID: "review", BranchID: "main", SourceID: DefaultActorID, Mode: storytime.EffectiveDuration, Status: storytime.ActivityPaused, RequiredEffective: 2 * 86400, CompletedEffective: 86400, LastSettledAt: 86400, PendingNodeIDs: []string{"real-interruption"}}}
	branch := &storyBranchProjection{State: state, Clock: &clock, Cultivations: map[string]CultivationActivity{activity.ID: activity}}
	t.Run("main_story_turn", func(t *testing.T) {
		plan := planTurnCultivation(branch, DefaultActorID, &TurnCultivationInput{Days: 1}, nil)
		_, advanced, ok := advanceXiuxianCultivation(state, xiuxianTestSystem(), DefaultActorID, plan.input, xiuxianFormalFacts{}, b1Identity("continue-chosen", 1), "continued")
		if !ok || advanced.MethodID != activity.Conditions.MethodID {
			t.Fatalf("continue without a new choice changed the paused method: input=%+v result=%+v", plan.input, advanced)
		}
	})
	t.Run("interval_with_other_new_conditions", func(t *testing.T) {
		continued, err := (&Store{}).planCultivationActivity("review", "main", branch, DefaultActorID, CultivationRequest{ActivityID: activity.ID, Conditions: &CultivationConditions{Environment: "stable_spiritual_site"}}, clock, cultivation.RecoveryQualified)
		if err != nil {
			t.Fatal(err)
		}
		if continued.Conditions.MethodID != activity.Conditions.MethodID {
			t.Fatalf("changing a different condition reset the chosen method: %+v", continued.Conditions)
		}
	})
}
