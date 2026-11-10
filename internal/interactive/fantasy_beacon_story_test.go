package interactive

import (
	"reflect"
	"testing"

	"ta/internal/exploration"
)

// These are explicit synthetic arrival and qualification fixtures. They test
// real Story transactions and authored resources, not natural-model gameplay.
func TestFantasyBeaconWardCostsAndMissingResourceAtomicity(t *testing.T) {
	for _, missing := range []string{"", "sg-focus", "sg-charcoal", "skill"} {
		t.Run("missing_"+missing, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-learner")
			if missing == "skill" {
				delete(a.Skills, "sg-ward")
			} else if missing != "" {
				delete(a.Inventory, missing)
			}
			s, id := fantasyBoundStory(t, r, a, "最后古灯塔")
			defer s.Close()
			realm := exploration.RealmShatteredBeacon
			entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
			before, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			count := realmTestRecordCount(t, s, id, "main")
			req := realmExploreRequest(entered.InstanceID, realm, "approach", realm+".ward", "ward")
			first, err := s.ApplyRealm(id, req)
			after, readErr := s.AdventureWindow(id, "main")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if missing != "" {
				if err == nil || !reflect.DeepEqual(before.Actor, after.Actor) || before.ClockSeconds != after.ClockSeconds || realmTestRecordCount(t, s, id, "main") != count {
					t.Fatalf("missing %s did not fail atomically: %v", missing, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if after.Actor.Energy != before.Actor.Energy-8 || after.Actor.Inventory["sg-charcoal"] != before.Actor.Inventory["sg-charcoal"]-1 || after.Actor.Inventory["sg-focus"] != 1 || after.ClockSeconds != before.ClockSeconds+600 {
				t.Fatal("ward did not settle exactly eight effort, one charcoal and ten minutes while retaining focus")
			}
			replay, err := s.ApplyRealm(id, req)
			if err != nil || !replay.Replayed || first.EventID != replay.EventID || len(first.Rewards) != 0 || len(replay.Rewards) != 0 {
				t.Fatal("ward retry did not replay", err)
			}
			retried, err := s.AdventureWindow(id, "main")
			if err != nil || !reflect.DeepEqual(after.Actor, retried.Actor) || after.ClockSeconds != retried.ClockSeconds {
				t.Fatal("ward retry charged twice", err)
			}
		})
	}
}

func TestFantasyBeaconReturnChoiceColdReplayAndBranchIsolation(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-learner")
	// The tools are an explicit qualified fixture for the alternate grinding choice.
	a.Inventory["sg-forge-tools"] = 1
	s, id := fantasyBoundStory(t, r, a, "最后古灯塔")
	defer s.Close()
	realm := exploration.RealmShatteredBeacon
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	worldFlowTurn(t, s, id, "用护幕挡住余光", "以石纹护幕遮住余光后通过。", TurnWorldActionInput{Action: TurnWorldActionRealm, InstanceID: entered.InstanceID, RealmAction: "approach", ApproachID: realm + ".ward"})
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	before := worldFlowClock(t, s, id).Seconds
	realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "preserve")
	due := realmExploreViewAt(t, s, id, "main").Explore.ReturnDueSeconds
	if due != before+900+14400 {
		t.Fatal("return was not scheduled four world hours after shuttering")
	}
	if _, err := s.ApplyRealm(id, realmExploreRequest(entered.InstanceID, realm, "approach", realm+".return", "early-return")); err == nil {
		t.Fatal("early return was accepted")
	}
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "retreat")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
	questWait(t, s, id, "main", due-worldFlowClock(t, s, id).Seconds-1)
	if realmExploreViewAt(t, s, id, "main").Explore.ReturnAvailable {
		t.Fatal("return opened one second early")
	}
	questWait(t, s, id, "main", 1)
	if !realmExploreViewAt(t, s, id, "main").Explore.ReturnAvailable {
		t.Fatal("return did not become available at its exact deadline")
	}
	preserved, err := s.RealmStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	dir := s.Root()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(dir)
	defer cold.Close()
	restored, err := cold.RealmStateFor(id, "main")
	if err != nil || !reflect.DeepEqual(preserved, restored) {
		t.Fatal("cold replay lost preservation or return deadline", err)
	}
	realmTestApply(t, cold, id, realmExploreRequest(entered.InstanceID, realm, RealmActionReenter, "", "reenter"))
	realmExploreMove(t, cold, id, entered.InstanceID, realm, "core", "return-core")
	realmExploreDo(t, cold, id, entered.InstanceID, realm, "approach", "return")
	realmExploreMove(t, cold, id, entered.InstanceID, realm, "record", "record")
	anchor, _, err := cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "比较两层刻纹", Narrative: "你停在刻纹前决定如何处理。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := cold.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "Grind the record"})
	if err != nil {
		t.Fatal(err)
	}
	beforeCopy, err := cold.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	copied := realmExploreDo(t, cold, id, entered.InstanceID, realm, "resolve", "copy")
	afterCopy, err := cold.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if afterCopy.Actor.Energy != beforeCopy.Actor.Energy-5 || afterCopy.Actor.Inventory["sg-journal"] != beforeCopy.Actor.Inventory["sg-journal"] || afterCopy.Actor.Inventory["sg-glass"] != 1 || afterCopy.ClockSeconds != beforeCopy.ClockSeconds+1800 {
		t.Fatal("copy did not consume real reading effort, preserve the journal and grant one fragment")
	}
	if _, err := cold.ApplyRealm(id, realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".grind", "main-grind")); err == nil {
		t.Fatal("main branch ground a preserved original after copying")
	}
	req := realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".grind", "fork-grind")
	req.BranchID = fork.ID
	realmTestApply(t, cold, id, req)
	forkWindow, err := cold.AdventureWindow(id, fork.ID)
	if err != nil || forkWindow.Actor.Inventory["sg-glass"] != 3 || forkWindow.Actor.Inventory["sg-forge-tools"] != 1 {
		t.Fatal("alternate branch did not salvage three pieces with retained tools", err)
	}
	req = realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".copy", "fork-copy")
	req.BranchID = fork.ID
	if _, err := cold.ApplyRealm(id, req); err == nil {
		t.Fatal("ground branch copied erased inscriptions")
	}
	retry := realmExploreDo(t, cold, id, entered.InstanceID, realm, "resolve", "copy")
	if !retry.Replayed || !reflect.DeepEqual(copied.Rewards, retry.Rewards) {
		t.Fatal("copy did not replay its single reward")
	}
	if _, err := cold.ApplyRealm(id, realmExploreRequest(entered.InstanceID, realm, "resolve", realm+".copy", "new-copy-command")); err == nil {
		t.Fatal("fresh command farmed a second fragment")
	}
	realmExploreMove(t, cold, id, entered.InstanceID, realm, "core", "leave-record")
	realmExploreMove(t, cold, id, entered.InstanceID, realm, "entry", "leave-core")
	realmTestApply(t, cold, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "final-exit"))
	final, err := cold.RealmStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := cold.Close(); err != nil {
		t.Fatal(err)
	}
	again := NewStore(dir)
	defer again.Close()
	reopened, err := again.RealmStateFor(id, "main")
	if err != nil || !reflect.DeepEqual(final, reopened) {
		t.Fatal("second cold replay lost finite rewards or exit", err)
	}
	if realmExploreQuantity(t, again, id, "main", "sui-guan-realms", "sg-glass") != 1 || realmExploreQuantity(t, again, id, fork.ID, "sui-guan-realms", "sg-glass") != 3 {
		t.Fatal("reward quantities leaked across branches or refreshed on reopen")
	}
}
