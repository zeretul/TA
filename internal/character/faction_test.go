package character

import (
	"reflect"
	"testing"
)

func factionTestMatter() Matter {
	return Matter{ID: "faction:aid", StoryID: "story", BranchID: "main", CharacterID: "worker", Kind: MatterAction, Status: MatterOpen, Counterparty: "protagonist", Statement: "One voluntary production affair", WorldTime: "day one", SourceEventID: "start", ExchangeID: "consent", Faction: &FactionTerms{ProjectID: "aid", GroupName: "Mutual aid", RecipeID: "salve", RequiredHours: 4, Phase: "active", ConsentExchangeID: "consent"}}
}
func TestFactionMatterCanonicalTransitionsAndIsolation(t *testing.T) {
	start := factionTestMatter()
	state, err := ApplyFactionMatter(nil, start)
	if err != nil {
		t.Fatal(err)
	}
	next := start.Normalize()
	next.Faction.WorkedHours = 1
	next.Faction.MaterialsSourceID = "work-one"
	next.Faction.ConsentExchangeID = "consent-two"
	next.SourceEventID = "work-one"
	state, err = ApplyFactionMatter(state, next)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := Matter{ID: "prose", Kind: MatterAction, Status: MatterSettled, SettledBy: next.ID, Statement: "The prose claims it is finished"}
	if got := ApplyMatters(state, []Matter{ordinary}); got[0].Status != MatterOpen || got[0].Faction.Phase != "active" {
		t.Fatal("ordinary prose settled protected production")
	}
	fork := next.Normalize()
	fork.BranchID = "fork"
	fork.Faction.WorkedHours = 4
	fork.Faction.ConsentExchangeID = "fork-consent"
	fork.Faction.OutputSourceID = "output"
	fork.Faction.Phase = "completed"
	fork.Status = MatterSettled
	forked, err := ApplyFactionMatter(state, fork)
	if err != nil {
		t.Fatal(err)
	}
	if state[0].Faction.WorkedHours != 1 || forked[0].Faction.WorkedHours != 4 {
		t.Fatal("fork mutated original")
	}
	again, err := ApplyFactionMatter(forked, fork)
	if err != nil || !reflect.DeepEqual(again, forked) {
		t.Fatal("canonical replay changed work")
	}
	if _, err = ApplyFactionMatter(forked, next); err == nil {
		t.Fatal("completed affair reopened")
	}
}
func TestFactionMatterRejectsChangedTermsAndUnfundedProgress(t *testing.T) {
	original := factionTestMatter()
	original.Faction.WorkedHours = 1
	original.Faction.MaterialsSourceID = "spent"
	for _, mutate := range []func(*Matter){
		func(m *Matter) { m.Faction.RequiredHours = 2 }, func(m *Matter) { m.Faction.RecipeID = "free-gold" }, func(m *Matter) { m.Faction.MaterialsSourceID = "other" }, func(m *Matter) { m.Faction.ConsentExchangeID = original.Faction.ConsentExchangeID }, func(m *Matter) { m.CharacterID = "someone-else" }, func(m *Matter) { m.Faction.WorkedHours = 0 },
	} {
		next := original.Normalize()
		next.Faction.WorkedHours = 2
		next.Faction.ConsentExchangeID = "new-consent"
		mutate(&next)
		if _, err := ApplyFactionMatter([]Matter{original}, next); err == nil {
			t.Fatalf("invalid transition accepted: %+v", next)
		}
		if original.Faction.WorkedHours != 1 {
			t.Fatal("candidate mutated history")
		}
	}
}
