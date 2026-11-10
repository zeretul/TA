package history

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLongWorldKeepsFactsPastOldCountLimits(t *testing.T) {
	store, s := worldCampaign(t)
	proposal := quietProposal(s)
	for i := 0; i < 350; i++ {
		id := fmt.Sprintf("old-person-%d", i)
		proposal.NewEntities = append(proposal.NewEntities, Entity{ID: id, Kind: "person", Name: Text{ZH: "旧任官员", EN: "Former official"}, Status: "retired", Owner: "ming", Attributes: map[string]Text{"record": {ZH: "完整旧任履历", EN: "Complete previous service record"}}, Audience: []string{"public"}})
		if i < 260 {
			proposal.NewAccounts = append(proposal.NewAccounts, Account{ID: fmt.Sprintf("old-account-%d", i), Name: Text{ZH: "旧账", EN: "Old account"}, Owner: id, Resource: "silver", Audience: []string{"player"}})
		}
	}
	proposal.Transfers = []Transfer{{From: "treasury", To: "old-account-0", Resource: "silver", Amount: 1, Reason: "Recorded reserve"}}
	r, err := store.Apply(context.Background(), s.ID, Command{ID: "large-world", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &QuarterPlan{World: proposal, Narrative: "A larger world"}})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	s = worldIssue(t, store, s, "old-task", OrderIntent{Action: "create", Kind: "policy", Text: "Review former service", Targets: []string{"old-person-1"}})
	view := ContextWorld(*s.World)
	if len(s.World.Entities) <= 768 || len(s.World.Accounts) <= 256 {
		t.Fatal("Fixture did not cross old limits")
	}
	if entityByID(&view.World, "old-person-0") == nil || entityByID(&view.World, "old-person-1") == nil {
		t.Fatal("Active dependency or nonzero account owner was archived")
	}
	if entityByID(&view.World, "old-person-2") != nil || len(view.ArchivedEntities) == 0 || len(view.ArchivedAccounts) == 0 {
		t.Fatal("Retired records were not layered")
	}
	if len(view.Entities)+len(view.ArchivedEntities) != len(s.World.Entities) || len(view.Accounts)+len(view.ArchivedAccounts) != len(s.World.Accounts) {
		t.Fatal("Archive projection lost facts")
	}
	cold, err := NewStore(filepath.Dir(store.root)).Read(context.Background(), s.ID)
	if err != nil || !reflect.DeepEqual(cold.World, s.World) {
		t.Fatalf("Long world cold read failed: %v", err)
	}
}
