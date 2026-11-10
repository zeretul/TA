package history

import (
	"context"
	"testing"
)

func TestWorldExternalPaymentCannotBypassOrderLimit(t *testing.T) {
	store, s := worldCampaign(t)
	s = worldIssue(t, store, s, "pay", OrderIntent{Action: "create", Kind: "one_shot", Text: "Reward no more than 100", Limits: map[string]int64{"silver": 100}})
	p := quietProposal(s)
	p.Transfers = []Transfer{{From: "treasury", To: "port-estate-wei-zhongxian", Resource: "silver", Amount: 200, OrderID: s.World.Orders[0].ID, Reason: "Personal reward"}}
	if _, err := store.Apply(context.Background(), s.ID, Command{ID: "pay-quarter", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &QuarterPlan{World: p}}); err == nil {
		t.Fatal("External payment bypassed the cap")
	}
	cold, _ := store.Read(context.Background(), s.ID)
	if cold.Revision != s.Revision {
		t.Fatal("Rejected quarter committed")
	}
	p.Transfers[0].Amount = 80
	r, err := store.Apply(context.Background(), s.ID, Command{ID: "valid-pay", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &QuarterPlan{World: p}})
	if err != nil {
		t.Fatal(err)
	}
	if r.State.World.Orders[0].Spent["silver"] != 80 {
		t.Fatal("External payment not counted")
	}
}

func TestWorldTreasuryTransfersHaveOwnershipSemantics(t *testing.T) {
	w := World{Entities: []Entity{{ID: "realm", Kind: "state"}, {ID: "rival", Kind: "state"}, {ID: "depot", Kind: "institution", Owner: "realm"}, {ID: "person", Kind: "person", Owner: "realm"}, {ID: "business", Kind: "institution", Owner: "person"}}}
	cases := []struct {
		from, to string
		want     bool
	}{{"realm", "depot", false}, {"depot", "realm", false}, {"realm", "rival", true}, {"realm", "person", true}, {"realm", "business", true}, {"person", "realm", false}, {"rival", "realm", false}}
	for _, c := range cases {
		if got := orderTransferSpends(&w, "realm", &Account{Owner: c.from}, &Account{Owner: c.to}); got != c.want {
			t.Errorf("%s -> %s = %v", c.from, c.to, got)
		}
	}
	if !orderTransferSpends(&w, "realm", &Account{Owner: "depot"}, nil) {
		t.Fatal("Consumption must count")
	}
}
