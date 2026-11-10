package interactiveapp

import (
	"encoding/json"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestRecordOnlyCatalogSnapshotRoundTripAndLegacyIsolation(t *testing.T) {
	pack := storypack.Pack{ContentVersion: 6, Characters: []storypack.CatalogCharacter{{SourceKey: "ancient", Name: "Ancient", RecordOnly: true}}}
	frozen := pack.FreezeCatalog()
	snapshot := catalogSnapshot(frozen)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var stored interactive.PackCatalogSnapshot
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	clone := interactive.ClonePackCatalogSnapshot(&stored)
	restored := frozenCatalog(clone)
	if !restored.CatalogCharacters()[0].RecordOnly {
		t.Fatal("historical-only flag lost in production snapshot conversions")
	}
	pack.Characters[0].RecordOnly = false
	frozen.People[0].RecordOnly = false
	stored.Characters[0].RecordOnly = false
	if !snapshot.Characters[0].RecordOnly || !restored.People[0].RecordOnly {
		t.Fatal("frozen availability mutated with source")
	}
	var old interactive.PackCatalogSnapshot
	if err = json.Unmarshal([]byte(`{"characters":[{"source_key":"ancient","name":"Ancient"}]}`), &old); err != nil {
		t.Fatal(err)
	}
	if catalogSnapshot(frozenCatalog(&old)).Characters[0].RecordOnly {
		t.Fatal("legacy availability changed")
	}
}
