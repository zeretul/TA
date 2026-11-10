package interactive

import (
	"ta/internal/cultivation"
	"ta/internal/storytime"
	"testing"
)

func TestIter4CultivationPreviewUsesInterruptedMethodAndAllowsExplicitChange(t *testing.T) {
	state := iter4MethodsState(t, "qi_6")
	records, _ := xiuxianEquipmentEntry(state, DefaultActorID, xiuxianFieldTechniques)
	for _, raw := range records {
		entry, _ := raw.(map[string]any)
		if xiuxianEntryText(entry, "定义") == "method.basic_breathing" {
			entry["运转"] = "停止"
		}
	}
	clock := NewStoryClock("")
	clock.Seconds = 86400
	activity := CultivationActivity{ID: "chosen-low", ActorID: DefaultActorID, Conditions: CultivationConditions{MethodID: "method.basic_breathing"}, Eligibility: cultivation.RecoveryQualified, Activity: storytime.Activity{ID: "chosen-low", StoryID: "preview", BranchID: "main", SourceID: DefaultActorID, Mode: storytime.EffectiveDuration, Status: storytime.ActivityPaused, RequiredEffective: 2 * 86400, CompletedEffective: 86400, LastSettledAt: 86400, PendingNodeIDs: []string{"interruption"}}}
	validation := TurnSubmissionContext{ActorState: xiuxianTestSystem(), CurrentState: state, ChoiceCount: 5, CurrentPlayerInput: "继续修炼一天", WorldView: &TurnWorldView{Clock: &clock, Cultivations: map[string]CultivationActivity{activity.ID: activity}}}
	rejected, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{Cultivation: &TurnCultivationInput{Days: 1}})
	if rejected.TurnResult().Cultivation != nil || receipt.ModuleStatus.Cultivation != TurnSubmissionModuleRejected {
		t.Fatalf("preview silently replaced unavailable interrupted method: %+v", receipt)
	}
	accepted, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{Cultivation: &TurnCultivationInput{Days: 1, MethodID: "method.warm_origin"}})
	if accepted.TurnResult().Cultivation == nil || accepted.TurnResult().Cultivation.MethodID != "method.warm_origin" || receipt.ModuleStatus.Cultivation != TurnSubmissionModuleAccepted {
		t.Fatalf("explicit choice was overwritten by old activity: %+v", receipt)
	}
}
