package storypack

import (
	"strings"
	"testing"
)

// TestCreationBlockDescribesOnlyWhatTheStartCommits pins the creation block
// against the templates it claims to describe. The page promises a realm, a
// purse and a set of carried things; those must be the template's own values,
// or the preview would promise a start the game does not perform.
func TestCreationBlockDescribesOnlyWhatTheStartCommits(t *testing.T) {
	pack := fanrenPack(t)
	if pack.Creation == nil {
		t.Fatal("the fanren-jie pack ships no creation block")
	}
	creation := pack.Creation
	if creation.AgeRange != [2]int{20, 28} {
		t.Fatalf("age range = %v, want [20 28]", creation.AgeRange)
	}
	if err := pack.validateCreation(); err != nil {
		t.Fatalf("creation block does not validate: %v", err)
	}
	for _, origin := range creation.Origins {
		templateID := strings.TrimSpace(origin.TemplateID)
		if templateID == "" {
			templateID = strings.TrimSpace(origin.DefaultTemplateID)
		}
		if templateID == "" {
			t.Fatalf("origin %s has no template to start from", origin.ID)
		}
		template, ok := pack.ProtagonistTemplate(templateID)
		if !ok {
			t.Fatalf("origin %s names missing template %s", origin.ID, templateID)
		}
		if origin.InitialState == nil {
			t.Fatalf("origin %s promises no initial state", origin.ID)
		}
		if got, want := origin.InitialState.Realm, template.State["当前境界"]; got != want {
			t.Fatalf("origin %s promises realm %v, template writes %v", origin.ID, got, want)
		}
		// The pack file is JSON, so the template's number arrives as a float64
		// while the preview's is an int. Compare them as numbers.
		templateCurrency, _ := template.State["灵石"].(float64)
		if got := origin.InitialState.Currency; float64(got) != templateCurrency {
			t.Fatalf("origin %s promises %d spirit stones, template writes %v", origin.ID, got, template.State["灵石"])
		}
	}
}

// TestResolveIdentityKeepsWhatThePlayerChose is the design's first acceptance
// item: "自定义输入未被AI覆盖". Every explicit field survives, and the mechanical
// values are re-derived from the pack rather than taken from the request.
func TestResolveIdentityKeepsWhatThePlayerChose(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(IdentityChoice{
		Name:                 "林照",
		Gender:               "female",
		Age:                  23,
		OriginID:             "custom",
		OriginText:           "世家出身的外门弟子",
		StateBasisTemplateID: "sect-disciple",
		AffiliationID:        "yue.huangfeng",
		LocationID:           "yue.huangfeng.approach",
		RootCategoryID:       "dual",
		RootAttributes:       []string{"wood", "water"},
		AptitudeID:           "good",
		WeaponID:             "sword",
		TraitIDs:             []string{"steady"},
		Identity:             "外门里没人记得她的姓。",
	})
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	if identity.Name != "林照" || identity.Gender != "female" || identity.Age != 23 {
		t.Fatalf("explicit basics were not preserved: %+v", identity)
	}
	if identity.OriginText != "世家出身的外门弟子" {
		t.Fatalf("custom origin text was replaced: %q", identity.OriginText)
	}
	if identity.TemplateID != "sect-disciple" {
		t.Fatalf("state basis = %q, want sect-disciple", identity.TemplateID)
	}
	if identity.AffiliationID != "yue.huangfeng" || identity.LocationID != "yue.huangfeng.approach" {
		t.Fatalf("affiliation or location was not preserved: %+v", identity)
	}
	if identity.Talent != "good" {
		t.Fatalf("talent = %q, want good", identity.Talent)
	}
	if !strings.Contains(identity.Foundation, "良好") {
		t.Fatalf("foundation does not carry the chosen tier: %q", identity.Foundation)
	}
	// The one thing the rules read must be unambiguous: exactly one tier word.
	for _, other := range []string{"极佳", "优秀", "较差", "普通"} {
		if strings.Contains(identity.Foundation, other) {
			t.Fatalf("foundation carries a second tier word %q: %q", other, identity.Foundation)
		}
	}
	// The initial state is the template's, not the client's.
	if identity.InitialState == nil || identity.InitialState.Realm != "qi_6" || identity.InitialState.Currency != 40 {
		t.Fatalf("initial state = %+v, want the sect template's qi_6/40", identity.InitialState)
	}
	// The preview shows the pack's wording, never the rules' realm id.
	if identity.InitialState.RealmLabel != "炼气6层" {
		t.Fatalf("realm label = %q, want the pack's 炼气6层", identity.InitialState.RealmLabel)
	}
}

// TestResolveIdentityReportsARealConflict keeps a mismatched choice a concrete
// complaint instead of a silent correction.
func TestResolveIdentityReportsARealConflict(t *testing.T) {
	pack := fanrenPack(t)
	_, err := pack.ResolveIdentity(IdentityChoice{
		Name:           "陈九",
		Age:            22,
		OriginID:       "sect",
		AffiliationID:  "yue.huangfeng",
		LocationID:     "yuanwu.market",
		RootCategoryID: "waste",
		AptitudeID:     "normal",
	})
	if err == nil {
		t.Fatal("a 越国 sect starting in 元武国 was accepted")
	}
	// The refusal is keyed so the page can read it in its own language; the
	// sentence a Chinese page shows is what this check is about.
	if zh := IdentityErrorText(err, "zh-CN"); !strings.Contains(zh, "落脚地点") {
		t.Fatalf("the error does not name the location conflict: %v", zh)
	}

	_, err = pack.ResolveIdentity(IdentityChoice{
		Name:           "陈九",
		Age:            22,
		OriginID:       "sect",
		AffiliationID:  "yue.huangfeng",
		LocationID:     "yue.yan.approach",
		RootCategoryID: "waste",
		AptitudeID:     "normal",
	})
	if err == nil || !strings.Contains(IdentityErrorText(err, "zh-CN"), "与所选所属") {
		t.Fatalf("another sect's gate was accepted: %v", err)
	}
}

// TestResolveIdentityChecksRootArity keeps a root category and its attributes
// from disagreeing.
func TestResolveIdentityChecksRootArity(t *testing.T) {
	pack := fanrenPack(t)
	base := IdentityChoice{
		Name:          "许三",
		Age:           25,
		OriginID:      "loose",
		AffiliationID: "none",
		LocationID:    "yue.market",
		AptitudeID:    "normal",
	}
	bad := base
	bad.RootCategoryID = "dual"
	bad.RootAttributes = []string{"fire"}
	if _, err := pack.ResolveIdentity(bad); err == nil {
		t.Fatal("a dual root with one attribute was accepted")
	}
	good := base
	good.RootCategoryID = "dual"
	good.RootAttributes = []string{"fire", "earth"}
	identity, err := pack.ResolveIdentity(good)
	if err != nil {
		t.Fatalf("a legal dual root was refused: %v", err)
	}
	if len(identity.RootAttributes) != 2 {
		t.Fatalf("attributes = %v", identity.RootAttributes)
	}

	// A waste root takes no attributes, and its category must not be turned into
	// a stronger one by supplying some.
	waste := base
	waste.RootCategoryID = "waste"
	waste.RootAttributes = []string{"fire"}
	if _, err := pack.ResolveIdentity(waste); err == nil {
		t.Fatal("a waste root with attributes was accepted")
	}
}

// TestRollAgeStaysInRange is the one random value in the flow. It must never
// leave the pack's declared range, and the same roll must give the same age.
func TestRollAgeStaysInRange(t *testing.T) {
	pack := fanrenPack(t)
	low, high := pack.Creation.AgeRange[0], pack.Creation.AgeRange[1]
	for roll := 0; roll < 200; roll++ {
		age := pack.RollAge(roll)
		if age < low || age > high {
			t.Fatalf("roll %d produced %d, outside %d–%d", roll, age, low, high)
		}
		if again := pack.RollAge(roll); again != age {
			t.Fatalf("roll %d is not stable: %d then %d", roll, age, again)
		}
	}
}

// TestResolveIdentityKeepsTheTypedAge is the rule the creation page relies on:
// a number the player typed is the number the character gets, and one outside
// the pack's range is refused rather than quietly replaced by a roll.
func TestResolveIdentityKeepsTheTypedAge(t *testing.T) {
	pack := fanrenPack(t)
	low, high := pack.Creation.AgeRange[0], pack.Creation.AgeRange[1]

	for _, age := range []int{low, (low + high) / 2, high} {
		choice := ageChoice(age)
		identity, err := pack.ResolveIdentity(choice)
		if err != nil {
			t.Fatalf("age %d was refused: %v", age, err)
		}
		if identity.Age != age {
			t.Fatalf("age %d came back as %d", age, identity.Age)
		}
	}

	for _, age := range []int{0, low - 1, high + 1} {
		choice := ageChoice(age)
		if _, err := pack.ResolveIdentity(choice); err == nil {
			t.Fatalf("age %d was accepted outside %d–%d", age, low, high)
		}
	}
}

// ageChoice is one otherwise-valid creation choice with only the age varied.
func ageChoice(age int) IdentityChoice {
	return IdentityChoice{
		Name:           "林照",
		Gender:         "female",
		Age:            age,
		OriginID:       "loose",
		AffiliationID:  "none",
		LocationID:     "yue.market",
		RootCategoryID: "dual",
		RootAttributes: []string{"wood", "water"},
		AptitudeID:     "good",
	}
}
