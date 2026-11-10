package character

import "testing"

func membershipFixture(status string) FactionMembership {
	return FactionMembership{ID: "mem-1", FactionID: "faction-1", FactionName: "同名组织", Status: status, Statement: "本人听到结果", SourceKind: "scene", SourceExchangeID: "exchange-1", TurnID: "turn-1", SourceEventID: "record-1", WorldTime: "午后"}
}
func TestFactionMembershipKeepsCurrentStandingAndHistory(t *testing.T) {
	s := State{}
	first := membershipFixture("formal")
	if err := s.ApplyFactionMemberships([]FactionMembership{first}, "record-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyFactionMemberships([]FactionMembership{first}, "record-1"); err != nil || len(s.FactionMemberships) != 1 {
		t.Fatalf("retry: %+v %v", s, err)
	}
	clone := s.Normalize()
	clone.FactionMemberships[0].Status = "left"
	if s.FactionMemberships[0].Status != "formal" {
		t.Fatal("clone aliases source")
	}
	second := membershipFixture("left")
	second.ID = "mem-2"
	other := membershipFixture("pending")
	other.ID = "mem-3"
	other.FactionID = "faction-2"
	if err := s.ApplyFactionMemberships([]FactionMembership{second, other}, "record-2"); err != nil {
		t.Fatal(err)
	}
	got := CurrentFactionMemberships(s.FactionMemberships)
	if len(got) != 2 || got[0].Status != "left" || got[1].Status != "pending" || len(s.FactionMemberships) != 3 {
		t.Fatalf("current/history = %+v / %+v", got, s.FactionMemberships)
	}
}
func TestFactionMembershipRejectsInvalidBatchAtomically(t *testing.T) {
	s := State{}
	bad := membershipFixture("leader")
	bad.ID = "bad"
	if err := s.ApplyFactionMemberships([]FactionMembership{membershipFixture("formal"), bad}, "r"); err == nil || len(s.FactionMemberships) != 0 {
		t.Fatalf("bad batch changed state: %+v %v", s, err)
	}
	for _, status := range []string{"pending", "temporary", "left"} {
		m := membershipFixture(status)
		m.SourceKind = "identity"
		m.TurnID = ""
		m.SourceExchangeID = ""
		if m.Validate() == nil {
			t.Fatalf("identity minted %s", status)
		}
	}
}
