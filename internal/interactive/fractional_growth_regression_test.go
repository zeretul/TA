package interactive

import (
	"math"
	"testing"

	"ta/internal/cultivation"
)

func TestFractionalCultivationProgressIsARealStateChange(t *testing.T) {
	state := xiuxianTestState(t, "qi_6", map[string]any{xiuxianFieldRealmProgress: 0.77})
	op := xiuxianAdvanceOp(DefaultActorID, xiuxianFieldRealmProgress, 0.8672222222222223, "turn-small-step")
	changed := xiuxianChangedOps(state, DefaultActorID, []ActorStateOp{op})
	if len(changed) != 1 {
		t.Fatalf("fractional growth was discarded: old=0.77 next=%v writes=%v", op.Value, changed)
	}
}

func TestOneHourNPCGrowthMatchesItsDurableRecord(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 3600, "fractional-hour")
	records := npcGrowthTestRecords(t, store, storyID)
	for _, record := range records {
		if record.Candidate.ActorID != "rival" {
			continue
		}
		actual := npcGrowthTestProgress(t, store, storyID, "rival")
		if math.Abs(actual-record.Candidate.NextState.Accumulated) > 1e-10 {
			t.Fatalf("one-hour progress differs: journal=%v state=%v", record.Candidate.NextState.Accumulated, actual)
		}
		return
	}
	t.Fatal("one real hour produced no NPC growth record")
}

func TestSplittingAnNPCDayDoesNotEraseTheTraining(t *testing.T) {
	whole, wholeID := npcGrowthTestStory(t)
	split, splitID := npcGrowthTestStory(t)
	recoveryTestApply(t, whole, wholeID, cultivation.RecoverySafeMeditation, 24*3600, "whole-day")
	for hour := range 24 {
		recoveryTestApply(t, split, splitID, cultivation.RecoverySafeMeditation, 3600, "hour-"+string(rune('a'+hour)))
	}
	wholeProgress := npcGrowthTestProgress(t, whole, wholeID, "rival")
	splitProgress := npcGrowthTestProgress(t, split, splitID, "rival")
	if math.Abs(wholeProgress-splitProgress) > 1e-10 {
		t.Fatalf("splitting a day changed the result: whole=%v split=%v", wholeProgress, splitProgress)
	}
}
