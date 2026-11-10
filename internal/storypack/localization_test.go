package storypack

import (
	"encoding/json"
	"strings"
	"testing"
)

// The delivered world ships English display text beside its own asset. These
// tests pin what "beside" has to mean: the translation covers the same ids, it
// moves no fact, and it never becomes part of a pack file.

func TestTheEnglishWorldCoversEveryStableID(t *testing.T) {
	pack, err := NewLibrary(t.TempDir()).Get(builtinPackIDFanren)
	if err != nil {
		t.Fatalf("read the delivered world: %v", err)
	}
	english, ok := pack.Localizations["en-US"]
	if !ok {
		t.Fatal("the delivered world ships no en-US display text")
	}
	if english.Name == "" || english.Description == "" {
		t.Fatalf("the English header is incomplete: %+v", english)
	}

	var missing []string
	for _, entry := range pack.World {
		text, found := english.World[entry.ID]
		if !found || strings.TrimSpace(text.Name) == "" || strings.TrimSpace(text.Content) == "" {
			missing = append(missing, "world."+entry.ID)
		}
	}
	for _, person := range pack.Characters {
		text, found := english.Characters[person.SourceKey]
		if !found || strings.TrimSpace(text.Name) == "" || strings.TrimSpace(text.Brief) == "" {
			missing = append(missing, "characters."+person.SourceKey)
		}
	}
	if pack.Creation != nil {
		missing = append(missing, missingCreationText(*pack.Creation, english.Creation)...)
	}
	if len(missing) > 0 {
		t.Fatalf("%d stable ids have no English display text, first: %v", len(missing), missing[:min(10, len(missing))])
	}
}

// missingCreationText reports the option ids an English block does not label.
// The protocol groups are skipped: they hold numbers and template ids, not text.
func missingCreationText(creation PackCreation, text *CreationLocalization) []string {
	if text == nil {
		return []string{"creation (absent)"}
	}
	check := func(group string, ids []string, labels map[string]CreationOptionText) []string {
		var missing []string
		for _, id := range ids {
			if strings.TrimSpace(labels[id].Name) == "" {
				missing = append(missing, "creation."+group+"."+id)
			}
		}
		return missing
	}
	ids := func(options []CreationOption) []string {
		out := make([]string, 0, len(options))
		for _, option := range options {
			out = append(out, option.ID)
		}
		return out
	}

	var missing []string
	missing = append(missing, check("genders", ids(creation.Genders), text.Genders)...)
	missing = append(missing, check("affiliations", ids(creation.Affiliations), text.Affiliations)...)
	missing = append(missing, check("locations", ids(creation.Locations), text.Locations)...)
	missing = append(missing, check("root_categories", ids(creation.RootCategories), text.RootCategories)...)
	missing = append(missing, check("root_attributes", ids(creation.RootAttributes), text.RootAttributes)...)
	missing = append(missing, check("variant_roots", ids(creation.VariantRoots), text.VariantRoots)...)
	missing = append(missing, check("weapon_directions", ids(creation.WeaponDirections), text.WeaponDirections)...)
	missing = append(missing, check("traits", ids(creation.Traits), text.Traits)...)
	missing = append(missing, check("talents", ids(creation.Talents), text.Talents)...)
	for _, origin := range creation.Origins {
		if strings.TrimSpace(text.Origins[origin.ID].Name) == "" {
			missing = append(missing, "creation.origins."+origin.ID)
		}
	}
	for _, aptitude := range creation.Aptitudes {
		if strings.TrimSpace(text.Aptitudes[aptitude.ID].Name) == "" {
			missing = append(missing, "creation.aptitudes."+aptitude.ID)
		}
	}
	return missing
}

// TestLocalizingKeepsEveryIdentityAndCount is the line the translation must not
// cross: words change, the world does not.
func TestLocalizingKeepsEveryIdentityAndCount(t *testing.T) {
	pack, err := NewLibrary(t.TempDir()).Get(builtinPackIDFanren)
	if err != nil {
		t.Fatalf("read the delivered world: %v", err)
	}
	localized := pack.Localize("en-US")
	if localized.Name == pack.Name {
		t.Fatal("the localized copy still has the source name; the test would prove nothing")
	}

	if len(localized.World) != len(pack.World) {
		t.Fatalf("localized world entries = %d, want %d", len(localized.World), len(pack.World))
	}
	for index := range localized.World {
		got, want := localized.World[index], pack.World[index]
		if got.ID != want.ID || got.Type != want.Type || got.Importance != want.Importance || got.LoadMode != want.LoadMode {
			t.Fatalf("world entry %d changed identity: %+v vs %+v", index, got, want)
		}
	}

	if len(localized.Characters) != len(pack.Characters) {
		t.Fatalf("localized catalog = %d, want %d", len(localized.Characters), len(pack.Characters))
	}
	for index := range localized.Characters {
		got, want := localized.Characters[index], pack.Characters[index]
		if got.SourceKey != want.SourceKey || got.Level != want.Level ||
			got.InitialStage != want.InitialStage || got.Lifeform != want.Lifeform ||
			got.CultivationMethodID != want.CultivationMethodID {
			t.Fatalf("catalog row %d changed identity or a rule value: %+v vs %+v", index, got, want)
		}
	}

	if localized.Creation == nil || pack.Creation == nil {
		t.Fatal("the delivered world lost its creation block")
	}
	if len(localized.Creation.RootCategories) != len(pack.Creation.RootCategories) {
		t.Fatalf("localized root categories = %d, want %d",
			len(localized.Creation.RootCategories), len(pack.Creation.RootCategories))
	}
	for index := range localized.Creation.RootCategories {
		if localized.Creation.RootCategories[index].ID != pack.Creation.RootCategories[index].ID {
			t.Fatalf("root category %d changed id", index)
		}
	}
	// The protocol groups are values a rule compares, so they must come through
	// byte for byte.
	if len(localized.Creation.AgeRange) != len(pack.Creation.AgeRange) {
		t.Fatal("localizing changed the age range")
	}
	for index := range localized.Creation.AgeRange {
		if localized.Creation.AgeRange[index] != pack.Creation.AgeRange[index] {
			t.Fatal("localizing changed an age range bound")
		}
	}
	if len(localized.Creation.StateBasisTemplates) != len(pack.Creation.StateBasisTemplates) {
		t.Fatal("localizing changed the state basis templates")
	}
}

// TestAWorldWithoutATranslationKeepsItsOwnText is the honest result for a world
// the player wrote: there is no English to show, so the page shows what the
// author wrote rather than a machine translation made up on the spot.
func TestAWorldWithoutATranslationKeepsItsOwnText(t *testing.T) {
	pack, err := NewLibrary(t.TempDir()).Get(builtinPackIDFanren)
	if err != nil {
		t.Fatalf("read the delivered world: %v", err)
	}
	// A user world carries the same shape with no Localizations, which is what
	// copying a pack's own document back produces.
	var userWorld Pack
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &userWorld); err != nil {
		t.Fatal(err)
	}
	if len(userWorld.Localizations) != 0 {
		t.Fatal("a pack document carried its translations into a saved file")
	}
	localized := userWorld.Localize("en-US")
	if localized.Name != userWorld.Name || len(localized.World) != len(userWorld.World) {
		t.Fatal("a world with no translation must read exactly as its author wrote it")
	}
	for index := range localized.World {
		if localized.World[index].Name != userWorld.World[index].Name {
			t.Fatalf("world entry %d was translated without a translation", index)
		}
	}
}

// TestATranslationNeverEntersThePackFile keeps the delivered prose in one place.
// If it were serialized, every save of a world derived from the delivered one
// would carry a second copy of the whole world book.
func TestATranslationNeverEntersThePackFile(t *testing.T) {
	pack, err := NewLibrary(t.TempDir()).Get(builtinPackIDFanren)
	if err != nil {
		t.Fatalf("read the delivered world: %v", err)
	}
	document, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document), `"localizations"`) {
		t.Fatal("the pack document carries its translations")
	}
	if strings.Contains(string(document), `"Azure Wind Sparrow"`) {
		t.Fatal("the pack document carries translated prose")
	}
}
