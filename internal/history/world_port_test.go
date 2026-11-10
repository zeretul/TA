package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSealedOrderPersistsWithoutEnteringCouncil(t *testing.T) {
	store, s := worldCampaign(t)
	sealed := "sealed"
	s = worldIssue(t, store, s, "secret-audit", OrderIntent{Action: "create", Kind: "project", Text: "Secretly audit the treasury", MinisterID: "bi-ziyan", Disclosure: &sealed})
	if s.World.Orders[0].Disclosure != "sealed" {
		t.Fatal("Sealed authorization was not persisted")
	}
	if len(VisibleWorld(*s.World, "bi-ziyan").Orders) != 1 || len(VisibleWorld(*s.World, "player").Orders) != 1 {
		t.Fatal("Authorized recipient lost the order")
	}
	if len(CouncilWorld(*s.World, []string{"bi-ziyan", "sun-chengzong"}, "bi-ziyan").Orders) != 0 {
		t.Fatal("Secret mandate leaked to council")
	}
	o := s.World.Orders[0]
	s = worldIssue(t, store, s, "revise-secret", OrderIntent{Action: "revise", OrderID: o.ID, Kind: o.Kind, Text: "Continue the confidential audit", MinisterID: o.MinisterID})
	if s.World.Orders[0].Disclosure != "sealed" {
		t.Fatal("Amendment silently disclosed the mandate")
	}
	read, err := store.Read(context.Background(), s.ID)
	if err != nil || read.World.Orders[0].Disclosure != "sealed" {
		t.Fatalf("Cold-read lost secrecy: %v", err)
	}
	p := quietProposal(s)
	p.Events = []WorldEvent{{Title: "Public bulletin", Text: "Audit details", OrderIDs: []string{o.ID}, Audience: []string{"public"}, Status: "open"}}
	if _, _, err := PreviewWorld(s, QuarterPlan{World: p}); err == nil {
		t.Fatal("Sealed operation accepted a public event")
	}
}

func TestVisibleLedgerPreservesCashFlowWithoutExposingHiddenAssets(t *testing.T) {
	w := World{Accounts: []Account{{ID: "treasury", Resource: "silver", Balance: 120, Audience: []string{"player"}}, {ID: "private-estate", Resource: "silver", Balance: 80, Audience: []string{"private-owner"}}}}
	p := WorldProposal{Transfers: []Transfer{{From: "private-estate", To: "treasury", Resource: "silver", Amount: 20, Reason: "Private owner and undisclosed balance"}}}
	view := VisibleProposal(p, VisibleWorld(w, "player"))
	if len(view.Transfers) != 1 || view.Transfers[0].Amount != 20 || view.Transfers[0].To != "treasury" || view.Transfers[0].From != "" || !view.Transfers[0].CounterpartyHidden {
		t.Fatalf("Visible income was lost or its private source exposed: %+v", view.Transfers)
	}
	b, _ := json.Marshal(view)
	if strings.Contains(string(b), "private-estate") || strings.Contains(string(b), "undisclosed balance") {
		t.Fatal("Hidden financial identity leaked")
	}
	if p.Transfers[0].From != "private-estate" || p.Transfers[0].CounterpartyHidden {
		t.Fatal("Projection changed the canonical ledger")
	}
}

func TestCouncilKeepsOfficialAccountsButNotTheSpeakersPersonalEstate(t *testing.T) {
	w := World{Entities: []Entity{{ID: "realm", Kind: "state"}, {ID: "speaker", Kind: "person"}}, Accounts: []Account{{ID: "treasury", Owner: "realm", Audience: []string{"speaker"}}, {ID: "estate", Owner: "speaker", Audience: []string{"speaker"}}}}
	view := CouncilWorld(w, []string{"speaker", "peer"}, "speaker")
	if len(view.Accounts) != 1 || view.Accounts[0].ID != "treasury" {
		t.Fatalf("Council confused official and personal finances: %+v", view.Accounts)
	}
	if len(VisibleWorld(w, "speaker").Accounts) != 2 {
		t.Fatal("Private audience lost personal knowledge")
	}
}

func TestSongChancellorsReceiveCurrentPublicTreasuryFacts(t *testing.T) {
	s, err := NewStore(t.TempDir()).Create(context.Background(), CreateRequest{Name: "Song briefing", CommandID: "song-briefing", ScenarioID: "song-1127"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"li_gang", "xu_jingheng"} {
		v := VisibleWorld(*s.World, id)
		if accountByID(&v, "treasury") == nil || accountByID(&v, "granary") == nil {
			t.Fatalf("Chancellor %s lacks the certified public accounts", id)
		}
	}
}

func TestPresentationFrozenSafeAndComplete(t *testing.T) {
	_, s := worldCampaign(t)
	p := s.Scenario
	if p.Presentation == nil || len(p.Presentation.Regions) != 16 {
		t.Fatal("Reference geography missing")
	}
	for _, e := range p.Initial.Entities {
		if e.Kind == "person" {
			if _, ok := p.Presentation.Portraits[e.ID]; !ok {
				t.Fatalf("Portrait missing for %s", e.ID)
			}
		}
	}
	b, _ := json.Marshal(p)
	copy, err := DecodeScenario(b)
	if err != nil || copy.Presentation.Backdrop != p.Presentation.Backdrop {
		t.Fatalf("Presentation round-trip: %v", err)
	}
	copy.Presentation.Backdrop = "https://example.com/remote.png"
	if ValidateScenario(copy) == nil {
		t.Fatal("External executable asset path accepted")
	}
	copy.Presentation.Backdrop = p.Presentation.Backdrop
	copy.Presentation.Regions[0].EntityID = "missing-region"
	if ValidateScenario(copy) == nil {
		t.Fatal("Map points to a missing entity")
	}
}

func TestNewWorldAttributesMustRemainBilingual(t *testing.T) {
	_, state := worldCampaign(t)
	for _, create := range []bool{false, true} {
		value := Text{ZH: "新的驻地记录"}
		proposal := WorldProposal{}
		if create {
			proposal.NewEntities = []Entity{{ID: "test-outpost", Kind: "institution", Name: Text{ZH: "驻所", EN: "Outpost"}, Status: "active", Attributes: map[string]Text{"station": value}, Audience: []string{"public"}}}
		} else {
			proposal.Changes = []EntityChange{{ID: "army-1", Attributes: map[string]Text{"station": value}, Reason: "Arrived"}}
		}
		next, _, err := PreviewWorld(state, QuarterPlan{World: &proposal})
		if err == nil || next.Turn != state.Turn {
			t.Fatalf("Untranslated attribute committed: %v", err)
		}
		if create {
			proposal.NewEntities[0].Attributes["station"] = Text{ZH: "新的驻地记录", EN: "Updated station"}
		} else {
			proposal.Changes[0].Attributes["station"] = Text{ZH: "新的驻地记录", EN: "Updated station"}
		}
		if _, _, err := PreviewWorld(state, QuarterPlan{World: &proposal}); err != nil {
			t.Fatal(err)
		}
	}
}
