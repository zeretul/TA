package interactive

import "testing"

// These are isolated ownership regressions, not earned campaigns. Supplies and
// the witness qualification are explicit fixtures; quest stages, journeys,
// participants, branch commits and cold replay use the actual authored content.
func TestFantasyReturnedKeyHasActualRecipientOnBothBranches(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Inventory["sg-iron"], a.Inventory["sg-crystal"] = 2, 1
	s, id := fantasyBoundStory(t, r, a, "倒旗墓道")
	defer s.Close()
	fantasyMeet(t, s, id, "main", "sg-p12", "joven")
	task := "sg-crypt-inquiry"
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "key-own-accept")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage1.c2", "key-own-investigate")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage2.c1", "key-own-repair")
	anchor, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "考虑旧钥的去处", Narrative: "你拿着刚取得的旧闸钥。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "Retain the key"})
	if err != nil {
		t.Fatal(err)
	}
	paid := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c1", "key-own-return")
	questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".stage3.c2", "key-own-retain")
	assertOwner := func(store *Store, branch string, playerCount, keeperCount int) {
		t.Helper()
		snap, err := store.Snapshot(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		player, err := AdventureActorFromState(snap.State, DefaultActorID)
		if err != nil {
			t.Fatal(err)
		}
		keeper, err := AdventureActorFromState(snap.State, "joven")
		if err != nil {
			t.Fatal(err)
		}
		if player.Inventory["sg-oath-key"] != playerCount || keeper.Inventory["sg-oath-key"] != keeperCount {
			t.Fatalf("returned/retained key lost its actual owner: branch=%s player=%d keeper=%d", branch, player.Inventory["sg-oath-key"], keeper.Inventory["sg-oath-key"])
		}
	}
	assertOwner(s, "main", 0, 1)
	assertOwner(s, fork.ID, 1, 0)
	replay := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c1", "key-own-return")
	if !replay.Replayed || replay.EventID != paid.EventID {
		t.Fatal("return replay lost its original receipt")
	}
	assertOwner(s, "main", 0, 1)
	root := s.Root()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	assertOwner(cold, "main", 0, 1)
	assertOwner(cold, fork.ID, 1, 0)
}

func TestFantasyLedgerEndingsTransferToTheirDifferentActualRecipients(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Rank = 7
	a.Skills["sg-witness"] = 1
	a.Inventory["sg-manual-witness"], a.Inventory["sg-rope"] = 1, 1
	s, id := fantasyBoundStory(t, r, a, "沉王水宫")
	defer s.Close()
	fantasyMeet(t, s, id, "main", "sg-p35", "asha")
	task := "sg-watergate-inquiry"
	questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "ledger-own-accept")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage1.c2", "ledger-own-route")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage2.c1", "ledger-own-retrieve")
	anchor, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "考虑原账的去处", Narrative: "刮改运粮账尚在你手里。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: anchor.ID, Title: "Sell the original ledger"})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"main", fork.ID} {
		if _, err := s.ApplyJourney(id, JourneyRequest{BranchID: branch, RouteID: "sg-route-l35-l31", Arrive: true}); err != nil {
			t.Fatal(err)
		}
	}
	fantasyMeet(t, s, id, "main", "sg-p31", "fio")
	questApplyOn(t, s, id, "main", task, QuestActionChoose, task+".stage3.c1", "ledger-own-give")
	if _, err := s.ApplyJourney(id, JourneyRequest{BranchID: fork.ID, RouteID: "sg-route-l31-l34", Arrive: true}); err != nil {
		t.Fatal(err)
	}
	fantasyMeet(t, s, id, fork.ID, "sg-p34", "mute")
	questApplyOn(t, s, id, fork.ID, task, QuestActionChoose, task+".stage3.c2", "ledger-own-sell")
	assertRecipient := func(store *Store, branch, receiver string) {
		t.Helper()
		snap, err := store.Snapshot(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		player, err := AdventureActorFromState(snap.State, DefaultActorID)
		if err != nil {
			t.Fatal(err)
		}
		npc, err := AdventureActorFromState(snap.State, receiver)
		if err != nil {
			t.Fatal(err)
		}
		if player.Inventory["sg-drowned-ledger"] != 0 || npc.Inventory["sg-drowned-ledger"] != 1 {
			t.Fatalf("ledger ending has no actual receiving owner: branch=%s receiver=%s player=%d recipient=%d", branch, receiver, player.Inventory["sg-drowned-ledger"], npc.Inventory["sg-drowned-ledger"])
		}
		// The rope was intentionally consumed during salvage, not handed over.
		if npc.Inventory["sg-rope"] != 0 {
			t.Fatal("salvage material was treated as a handover")
		}
	}
	assertRecipient(s, "main", "fio")
	assertRecipient(s, fork.ID, "mute")
	root := s.Root()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	assertRecipient(cold, "main", "fio")
	assertRecipient(cold, fork.ID, "mute")
}
