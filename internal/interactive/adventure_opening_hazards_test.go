package interactive

import (
	"reflect"
	"testing"
)

func TestAdventureOpeningDeadlineWaitEscapeAndReload(t *testing.T) {
	h := AdventureOpeningHazard{ID: "xinghai-c01-vacuum", DueSeconds: 6000, LocationIDs: []string{"xinghai-l01"}, EscapeLocationID: "xinghai-l02", Damage: 40, Injury: 20, Description: "真空吹扫还剩一小时四十分。", Consequence: "强制疏散时受伤，已转移至安全区。"}
	s, id, opening, dir := authoredQuestStory(t, "xinghai-689", "朱伯利外环47号减压长廊", 0, nil, h)
	escaped, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "提前离开"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(id, "main")
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "原地练习维修四小时", Narrative: "试图开始练习。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "原地练习维修四小时", Adventure: &AdventureAction{Action: "train", SkillID: "xinghai-s01", Hours: 4}}}})
	if err == nil {
		t.Fatal("productive work crossed the opening deadline")
	}
	after, _ := s.Snapshot(id, "main")
	if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
		t.Fatal("refused training paid progress or time")
	}
	// The long explicit wait stops at the actual event, not hours after it.
	questWait(t, s, id, "main", 14400)
	snap, _ := s.Snapshot(id, "main")
	a, err := AdventureActorFromState(snap.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Clock.Seconds != 6000 || a.Health != 60 || a.Injury != 20 || a.OpeningHazards[0].Outcome != "exposed" || journeyTestLocation(t, s, id, "main") != "老锚机械酒吧与二手船材集市" {
		t.Fatalf("deadline did not settle at 100 minutes: clock=%+v actor=%+v", snap.Clock, a)
	}
	_, err = s.ApplyJourney(id, JourneyRequest{BranchID: escaped.ID, RouteID: "xinghai-route-l01-l02", Arrive: true})
	if err != nil {
		t.Fatal(err)
	}
	questWait(t, s, id, escaped.ID, 14400)
	safe, _ := s.Snapshot(id, escaped.ID)
	other, _ := AdventureActorFromState(safe.State, DefaultActorID)
	if other.Health != 100 || other.OpeningHazards[0].Outcome != "escaped" {
		t.Fatal("leaving the danger area did not prevent harm")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(dir)
	defer s.Close()
	questWait(t, s, id, "main", 14400)
	reload, _ := s.Snapshot(id, "main")
	final, _ := AdventureActorFromState(reload.State, DefaultActorID)
	if final.Health != 60 || final.Injury != 20 || final.OpeningHazards[0].ResolvedAtSeconds != 6000 {
		t.Fatal("reload or later time paid the hazard twice")
	}
	tasks, _ := s.QuestStateFor(id, "main")
	if len(tasks.Tasks) != 0 {
		t.Fatal("opening hazard silently accepted a quest")
	}
	t.Log("100-minute deadline: four-hour training refused atomically; wait stops at 6000s and settles harm once; early journey avoids harm; reopen preserves independent branches")
}
