package cultivation

import "testing"

// Preserve the six finite human-realm totals. Spirit-realm rules explicitly
// distinguish no fixed natural-age limit from unsupported realms.

func TestTheConfirmedLifespanTotalsAreTheOnesTheDesignFixed(t *testing.T) {
	want := map[Realm]int{
		RealmMortal:     100,
		RealmQi:         120,
		RealmFoundation: 200,
		RealmCore:       500,
		RealmNascent:    1000,
		RealmSpirit:     2000,
	}
	for realm, expected := range want {
		got, ok := LifespanYears(realm)
		if !ok {
			t.Fatalf("LifespanYears(%q) reported unknown", realm)
		}
		if got != expected {
			t.Errorf("LifespanYears(%q) = %d, want %d", realm, got, expected)
		}
	}
	if len(want) != 6 {
		t.Fatalf("the finite human-realm table covers %d realms, want 6", len(want))
	}
}

// Unsupported realms must not borrow a neighboring finite age limit.
func TestAnUnconfirmedRealmReportsUnknown(t *testing.T) {
	for _, realm := range []Realm{Realm("immortal"), Realm(""), Realm("化神")} {
		years, ok := LifespanYears(realm)
		if ok || years != 0 {
			t.Errorf("LifespanYears(%q) = %d, %v; want 0, false", realm, years, ok)
		}
	}
}

// TestTheTotalsRiseWithEachMajorRealm guards the table's shape rather than its
// numbers: a higher realm that lived less long would be a transposition typo.
func TestTheTotalsRiseWithEachMajorRealm(t *testing.T) {
	previous := 0
	for _, realm := range RealmOrder {
		rule, known := LifespanRuleFor(realm)
		if known && rule.NoFixedLimit {
			continue
		}
		years, ok := LifespanYears(realm)
		if !ok {
			t.Fatalf("LifespanYears(%q) reported unknown", realm)
		}
		if years <= previous {
			t.Fatalf("%q has %d years, which does not exceed the previous realm's %d", realm, years, previous)
		}
		previous = years
	}
}
