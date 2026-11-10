package storypack

import (
	"strings"
	"testing"
)

func TestDeriveSurnameReadsCompoundSurnamesWhole(t *testing.T) {
	cases := map[string]string{
		"叶流云": "叶",
		"欧阳明": "欧阳",
		"司马青": "司马",
		"林照":  "林",
		"":    "",
	}
	for name, want := range cases {
		if got := DeriveSurname(name); got != want {
			t.Errorf("DeriveSurname(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestDeriveFamilyNameUsesTheWorldsWording(t *testing.T) {
	cases := map[string]string{
		"叶流云": "叶氏",
		"欧阳明": "欧阳氏",
		"":    "",
	}
	for name, want := range cases {
		if got := DeriveFamilyName(name); got != want {
			t.Errorf("DeriveFamilyName(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestOwnFamilyIsNamedAfterThePlayer pins the "自己的家族" option: it is offered
// as one affiliation rather than a free-text field, and its name comes from the
// character's own name, so it can never be filled in with a family the world
// already has.
func TestOwnFamilyIsNamedAfterThePlayer(t *testing.T) {
	pack := fanrenPack(t)
	if _, ok := pack.Creation.Affiliation(ownFamilyAffiliationID); !ok {
		t.Fatalf("the pack does not offer the %s affiliation", ownFamilyAffiliationID)
	}
	option, err := pack.resolveAffiliation(ownFamilyAffiliationID, "欧阳明")
	if err != nil {
		t.Fatalf("resolve own family: %v", err)
	}
	if option.Name != "欧阳氏" {
		t.Fatalf("own family name = %q, want 欧阳氏", option.Name)
	}
	if option.Kind != "family" {
		t.Fatalf("own family kind = %q, want family", option.Kind)
	}
	if _, err := pack.resolveAffiliation(ownFamilyAffiliationID, "  "); err == nil {
		t.Fatal("own family resolved without a name")
	}
}

// TestOriginAffiliationKindsAreEnforced covers the rule the page relies on: a
// loose cultivator has no sect, a sect disciple must name one, and a family heir
// must name a family. The server checks the same rule, so a forged request
// cannot pair a 散修 with a 宗门.
func TestOriginAffiliationKindsAreEnforced(t *testing.T) {
	pack := fanrenPack(t)
	sect, _ := pack.Creation.Affiliation("yue.huangfeng")
	none, _ := pack.Creation.Affiliation("none")
	family, _ := pack.Creation.Affiliation("yue.yan")
	own, _ := pack.Creation.Affiliation(ownFamilyAffiliationID)

	cases := []struct {
		origin string
		with   CreationOption
		ok     bool
	}{
		{"loose", none, true},
		{"loose", sect, false},
		{"loose", family, false},
		{"sect", sect, true},
		{"sect", none, false},
		{"sect", family, false},
		{"sect", own, false},
		{"family", family, true},
		{"family", own, true},
		{"family", none, false},
		{"family", sect, false},
		// A custom origin states its own affiliation, so nothing constrains it.
		{"custom", none, true},
		{"custom", sect, true},
		{"custom", family, true},
	}
	for _, tc := range cases {
		origin, ok := pack.Creation.Origin(tc.origin)
		if !ok {
			t.Fatalf("pack has no %s origin", tc.origin)
		}
		err := pack.checkOriginAffiliation(origin, tc.with, origin.TemplateID)
		if tc.ok && err != nil {
			t.Errorf("origin %s with %s was rejected: %v", tc.origin, tc.with.ID, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("origin %s with %s was accepted", tc.origin, tc.with.ID)
		}
	}
}

// TestCustomOriginInheritsItsBasisRule closes the hole the page's filtering
// would otherwise be the only guard against: a custom origin names no
// affiliation kind, but the basis it starts from does. A request that states a
// 世家 origin while starting from the sect template must not be given a family.
func TestCustomOriginInheritsItsBasisRule(t *testing.T) {
	pack := fanrenPack(t)
	choice := IdentityChoice{
		Name:                 "沈砚",
		Gender:               "male",
		Age:                  24,
		OriginID:             "custom",
		OriginText:           "世家出身的外门弟子",
		StateBasisTemplateID: "sect-disciple",
		AffiliationID:        "yue.yan",
		LocationID:           "yue.yan.approach",
		RootCategoryID:       "dual",
		RootAttributes:       []string{"wood", "water"},
		AptitudeID:           "good",
	}
	if _, err := pack.ResolveIdentity(choice); err == nil {
		t.Fatal("a custom origin starting as a sect disciple was given a family")
	}

	choice.AffiliationID = "yue.huangfeng"
	choice.LocationID = "yue.huangfeng.approach"
	if _, err := pack.ResolveIdentity(choice); err != nil {
		t.Fatalf("the matching sect was refused: %v", err)
	}

	// The loose basis takes no affiliation at all, and the family basis takes a
	// family. Both must still be accepted for the custom origin that picks them.
	loose := choice
	loose.StateBasisTemplateID = "loose-cultivator"
	loose.AffiliationID = "none"
	loose.LocationID = "yue.market"
	if _, err := pack.ResolveIdentity(loose); err != nil {
		t.Fatalf("a custom loose cultivator was refused: %v", err)
	}
	if _, err := pack.ResolveIdentity(func() IdentityChoice {
		next := loose
		next.AffiliationID = "yue.huangfeng"
		next.LocationID = "yue.huangfeng.approach"
		return next
	}()); err == nil {
		t.Fatal("a custom loose cultivator was given a sect")
	}

	family := choice
	family.StateBasisTemplateID = "family-heir"
	family.AffiliationID = "own"
	family.LocationID = "yue.market"
	if _, err := pack.ResolveIdentity(family); err != nil {
		t.Fatalf("a custom family heir was refused their own household: %v", err)
	}
}

// TestSectOriginCopyNamesTheChosenSect is the acceptance item "宗门出身的文案要与
// 实际选择的宗门一致": the authored copy says 宗门 until a sect is picked, and the
// resolved text names the one the player actually chose.
func TestSectOriginCopyNamesTheChosenSect(t *testing.T) {
	pack := fanrenPack(t)
	identity, err := pack.ResolveIdentity(IdentityChoice{
		Name:            "叶流云",
		Gender:          "male",
		Age:             22,
		OriginID:        "sect",
		AffiliationID:   "yue.huangfeng",
		LocationID:      "yue.huangfeng.approach",
		RootCategoryID:  "dual",
		RootAttributes:  []string{"wood", "water"},
		AptitudeID:      "good",
	})
	if err != nil {
		t.Fatalf("resolve identity: %v", err)
	}
	if !strings.Contains(identity.OriginText, "黄枫谷") {
		t.Fatalf("sect origin text does not name the chosen sect: %q", identity.OriginText)
	}
	if strings.Contains(identity.OriginText, affiliationPlaceholder) {
		t.Fatalf("sect origin text still carries a placeholder: %q", identity.OriginText)
	}
}
