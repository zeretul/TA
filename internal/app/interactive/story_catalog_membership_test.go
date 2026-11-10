package interactiveapp

import (
	"encoding/json"
	"reflect"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestCatalogMembershipSnapshotRoundTrip(t *testing.T) {
	pack := storypack.Pack{ContentVersion: 7, Characters: []storypack.CatalogCharacter{{SourceKey: "elder", Name: "Elder", FactionID: "sect", FactionName: "Sect", MembershipAdmission: []string{"formal", "temporary"}}}}
	frozen := pack.FreezeCatalog()
	snapshot := catalogSnapshot(frozen)
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var stored interactive.PackCatalogSnapshot
	if err = json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	clone := interactive.ClonePackCatalogSnapshot(&stored)
	restored := frozenCatalog(clone)
	if got := restored.CatalogCharacters()[0]; got.FactionName != "Sect" || !reflect.DeepEqual(got.MembershipAdmission, []string{"formal", "temporary"}) {
		t.Fatalf("round trip lost admission fields: %#v", got)
	}
	frozen.People[0].MembershipAdmission[0] = "original changed"
	stored.Characters[0].MembershipAdmission[0] = "stored changed"
	clone.Characters[0].MembershipAdmission[0] = "clone changed"
	if snapshot.Characters[0].MembershipAdmission[0] != "formal" || restored.People[0].MembershipAdmission[0] != "formal" {
		t.Fatal("snapshot conversion or clone shares admission slices")
	}
}

func TestLegacyCatalogSnapshotDoesNotGainMembershipAuthority(t *testing.T) {
	var stored interactive.PackCatalogSnapshot
	if err := json.Unmarshal([]byte(`{"characters":[{"source_key":"head","name":"Head","faction_id":"sect","role":"sect head authorized to admit members"}]}`), &stored); err != nil {
		t.Fatal(err)
	}
	frozen := frozenCatalog(&stored)
	roundTrip := catalogSnapshot(frozen)
	person := roundTrip.Characters[0]
	if person.FactionName != "" || len(person.MembershipAdmission) != 0 {
		t.Fatalf("legacy snapshot gained authority: %#v", person)
	}
	if frozenCatalog(nil) != nil || catalogSnapshot(nil) != nil {
		t.Fatal("missing snapshot gained a catalog")
	}
}
