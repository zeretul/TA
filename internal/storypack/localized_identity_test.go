package storypack

import (
	"strings"
	"testing"
	"unicode"
)

// A character sheet shown on an English page must read as English, and it must
// still be the same character. These tests pin both halves: every word a player
// reads is translated, and every id and rule value the story starts from is
// byte-for-byte what the Chinese page would have produced.

func hanCount(text string) int {
	count := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			count++
		}
	}
	return count
}

func englishChoice() IdentityChoice {
	return IdentityChoice{
		Name:           "Lin Zhao",
		Gender:         "female",
		Age:            23,
		OriginID:       "sect",
		AffiliationID:  "yue.huangfeng",
		LocationID:     "yue.huangfeng.approach",
		RootCategoryID: "dual",
		RootAttributes: []string{"wood", "water"},
		AptitudeID:     "good",
		WeaponID:       "sword",
		TraitIDs:       []string{"steady", "curious"},
	}
}

// TestALocalizedSheetSpeaksOneLanguage walks every field a page prints. A single
// Chinese word left in any of them is the failure this test exists to catch.
func TestALocalizedSheetSpeaksOneLanguage(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(englishChoice())
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	if hanCount(identity.Foundation) == 0 {
		t.Fatal("the source foundation carries no Chinese; the test would prove nothing")
	}

	shown := pack.LocalizeIdentity(identity, "en-US")
	fields := map[string]string{
		"gender_name":          shown.GenderName,
		"origin_name":          shown.OriginName,
		"origin_text":          shown.OriginText,
		"affiliation_name":     shown.AffiliationName,
		"location_name":        shown.LocationName,
		"root_category_name":   shown.RootCategoryName,
		"variant_root_name":    shown.VariantRootName,
		"aptitude_name":        shown.AptitudeName,
		"weapon_direction_name": shown.WeaponName,
		"talent_name":          shown.TalentName,
		"talent_bonus":         shown.TalentBonus,
		"foundation":           shown.Foundation,
		"root_attributes":      strings.Join(shown.RootAttributes, ", "),
		"trait_names":          strings.Join(shown.TraitNames, ", "),
	}
	if shown.InitialState != nil {
		fields["initial_state.realm_label"] = shown.InitialState.RealmLabel
		fields["initial_state.items"] = strings.Join(shown.InitialState.Items, ", ")
		fields["initial_state.techniques"] = strings.Join(shown.InitialState.Techniques, ", ")
	}
	for name, text := range fields {
		if count := hanCount(text); count > 0 {
			t.Fatalf("%s still carries %d Chinese characters: %q", name, count, text)
		}
	}
	if !strings.Contains(shown.Foundation, "Aptitude:") {
		t.Fatalf("the English foundation was not rebuilt from the structured names: %q", shown.Foundation)
	}
}

// TestALocalizedSheetKeepsEveryRuleValue is the other half. The story starts
// from the resolved identity, not from what the page printed, so the two must
// agree on every id and every number.
func TestALocalizedSheetKeepsEveryRuleValue(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(englishChoice())
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	shown := pack.LocalizeIdentity(identity, "en-US")

	same := []struct {
		field     string
		got, want string
	}{
		{"name", shown.Name, identity.Name},
		{"gender", shown.Gender, identity.Gender},
		{"origin_id", shown.OriginID, identity.OriginID},
		{"template_id", shown.TemplateID, identity.TemplateID},
		{"affiliation_id", shown.AffiliationID, identity.AffiliationID},
		{"location_id", shown.LocationID, identity.LocationID},
		{"root_category_id", shown.RootCategoryID, identity.RootCategoryID},
		{"variant_root_id", shown.VariantRootID, identity.VariantRootID},
		{"aptitude_id", shown.AptitudeID, identity.AptitudeID},
		{"talent", shown.Talent, identity.Talent},
		{"weapon_direction_id", shown.WeaponID, identity.WeaponID},
		{"talent_id", shown.TalentID, identity.TalentID},
		{"profile", shown.Profile, identity.Profile},
		{"premise", shown.Premise, identity.Premise},
	}
	for _, check := range same {
		if check.got != check.want {
			t.Fatalf("%s changed when the page's language did: %q vs %q", check.field, check.got, check.want)
		}
	}
	if shown.Age != identity.Age {
		t.Fatalf("age changed: %d vs %d", shown.Age, identity.Age)
	}
	if strings.Join(shown.TraitIDs, ",") != strings.Join(identity.TraitIDs, ",") {
		t.Fatalf("trait ids changed: %v vs %v", shown.TraitIDs, identity.TraitIDs)
	}

	if identity.InitialState == nil || shown.InitialState == nil {
		t.Fatal("the ready-made start is missing on one side")
	}
	if shown.InitialState.Realm != identity.InitialState.Realm {
		t.Fatalf("starting realm changed: %q vs %q", shown.InitialState.Realm, identity.InitialState.Realm)
	}
	if shown.InitialState.Currency != identity.InitialState.Currency {
		t.Fatalf("starting purse changed: %d vs %d", shown.InitialState.Currency, identity.InitialState.Currency)
	}
	if len(shown.InitialState.Items) != len(identity.InitialState.Items) {
		t.Fatalf("carried things changed count: %v vs %v", shown.InitialState.Items, identity.InitialState.Items)
	}
	if len(shown.InitialState.Techniques) != len(identity.InitialState.Techniques) {
		t.Fatalf("learned methods changed count: %v vs %v", shown.InitialState.Techniques, identity.InitialState.Techniques)
	}
}

// TestTheSourceIdentityIsUntouched is the safety property behind the copy: the
// identity a story is created from must not be the one the page was shown.
func TestTheSourceIdentityIsUntouched(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(englishChoice())
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	before := identity
	_ = pack.LocalizeIdentity(identity, "en-US")

	if identity.Foundation != before.Foundation || identity.Profile != before.Profile || identity.Premise != before.Premise {
		t.Fatal("localizing a copy wrote back into the identity the story starts from")
	}
	if hanCount(identity.Foundation) == 0 {
		t.Fatal("the source foundation lost its Chinese, which is what the rules read")
	}
}

// TestAChinesePageKeepsItsOwnWords keeps the no-op path honest: a Chinese page
// gets the pack's own text, not a translated-then-translated-back copy.
func TestAChinesePageKeepsItsOwnWords(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(englishChoice())
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	if shown := pack.LocalizeIdentity(identity, "zh-CN"); shown.Foundation != identity.Foundation || shown.OriginText != identity.OriginText {
		t.Fatal("a Chinese page was shown translated text")
	}
	if shown := pack.LocalizeIdentity(identity, ""); shown.Foundation != identity.Foundation {
		t.Fatal("a page with no locale was shown translated text")
	}
}

// TestTheEnglishStartCoversEveryTemplateName keeps the new resource honest. A
// name the pack's ready-made starts carry but the English block does not cover
// would reach a page as Chinese, so the coverage is checked rather than assumed.
func TestTheEnglishStartCoversEveryTemplateName(t *testing.T) {
	pack := fanrenPack(t)
	english, ok := pack.Localizations["en-US"]
	if !ok || english.InitialState == nil {
		t.Fatal("the delivered world ships no English text for the ready-made starts")
	}
	text := english.InitialState
	for _, template := range pack.ProtagonistTemplates {
		initial := initialStateOf(template)
		if initial == nil {
			t.Fatalf("template %s carries no state to start from", template.ID)
		}
		for _, name := range initial.Items {
			if strings.TrimSpace(text.Items[name]) == "" {
				t.Fatalf("template %s carries %q, which the English block does not name", template.ID, name)
			}
		}
		for _, name := range initial.Techniques {
			if strings.TrimSpace(text.Techniques[name]) == "" {
				t.Fatalf("template %s knows %q, which the English block does not name", template.ID, name)
			}
		}
	}
	for _, origin := range pack.Creation.Origins {
		if origin.InitialState == nil {
			continue
		}
		label := strings.TrimSpace(origin.InitialState.RealmLabel)
		if label == "" {
			continue
		}
		if strings.TrimSpace(text.Realms[label]) == "" {
			t.Fatalf("origin %s starts at %q, which the English block does not name", origin.ID, label)
		}
	}
}
