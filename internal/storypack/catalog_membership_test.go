package storypack

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogMembershipAdmissionValidation(t *testing.T) {
	valid := CatalogCharacter{SourceKey: "elder", Name: "Elder", FactionID: " sect ", FactionName: " Sect ", MembershipAdmission: []string{" formal ", "temporary", "formal"}}
	normalized := valid.Normalize()
	if normalized.FactionID != "sect" || normalized.FactionName != "Sect" || !reflect.DeepEqual(normalized.MembershipAdmission, []string{"formal", "temporary"}) {
		t.Fatalf("normalization lost authority: %#v", normalized)
	}
	if err := normalized.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized.MembershipAdmission[0] = "changed"
	if valid.MembershipAdmission[0] != " formal " {
		t.Fatal("normalize aliases the input")
	}
	for _, tc := range []struct {
		name string
		edit func(*CatalogCharacter)
	}{
		{"invalid", func(c *CatalogCharacter) { c.MembershipAdmission = []string{"leader"} }},
		{"wrong case", func(c *CatalogCharacter) { c.MembershipAdmission = []string{"Formal"} }},
		{"missing faction id", func(c *CatalogCharacter) { c.FactionID = " " }},
		{"missing faction name", func(c *CatalogCharacter) { c.FactionName = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.edit(&c)
			if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "membership_admission") {
				t.Fatalf("expected explicit admission error, got %v", err)
			}
		})
	}
	legacy := CatalogCharacter{SourceKey: "head", Name: "Head", FactionID: "sect", Role: "sect head empowered to admit formal members"}.Normalize()
	if err := legacy.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(legacy.MembershipAdmission) != 0 || legacy.FactionName != "" {
		t.Fatalf("legacy role gained authority: %#v", legacy)
	}
}

func TestFrozenMembershipAdmissionIsolation(t *testing.T) {
	pack := Pack{ContentVersion: 7, Characters: []CatalogCharacter{{SourceKey: "elder", Name: "Elder", FactionID: "sect", FactionName: "Sect", MembershipAdmission: []string{"formal", "temporary"}, Aliases: []string{"Teacher"}, Topics: []string{"admission"}, RegionTokens: []string{"Sect"}}}}
	frozen := pack.FreezeCatalog()
	pack.Characters[0].MembershipAdmission[0] = "changed"
	character := frozen.CatalogBySourceKey()["elder"]
	if character.FactionName != "Sect" || !reflect.DeepEqual(character.MembershipAdmission, []string{"formal", "temporary"}) {
		t.Fatalf("frozen fields lost: %#v", character)
	}
	character.MembershipAdmission[0] = "changed"
	character.Aliases[0] = "changed"
	character.Topics[0] = "changed"
	character.RegionTokens[0] = "changed"
	if got := frozen.CatalogCharacters()[0]; got.MembershipAdmission[0] != "formal" || got.Aliases[0] != "Teacher" || got.Topics[0] != "admission" || got.RegionTokens[0] != "Sect" {
		t.Fatalf("frozen query leaked writable slices: %#v", got)
	}
	thawed := NewFrozen(frozen.ContentVersion, frozen.People, frozen.TopicTerms, frozen.Places)
	frozen.People[0].MembershipAdmission[0] = "changed"
	if got := thawed.CatalogCharacters()[0]; got.MembershipAdmission[0] != "formal" {
		t.Fatalf("new frozen retained input slice: %#v", got)
	}
}
