package cultivation

import "testing"

func TestWardTalismanConsumesOneCopyAndAbsorbsRealDamage(t *testing.T) {
	definition, _ := NewItemCatalogWithFirstVersion().Definition("talisman.ward.lower")
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.Carried = []CarriedDose{{InstanceID: "ward", DefID: definition.ID, Quantity: 2, Talisman: definition.Talisman}}
	next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("ward")}, fixedDraws(0.5))
	if !findStep(t, outcome, "hero").ItemApplied || next.CombatantByID("hero").Carried[0].Quantity != 1 {
		t.Fatalf("ward did not consume exactly one copy: %+v", outcome)
	}
	shield := next.CombatantByID("hero").Shield
	if shield.Amount != definition.Talisman.ShieldFor(Qi(6), "").Amount || shield.RemainingExchanges != 2 {
		t.Fatalf("wrong fixed effect or activation-round expiry: %+v", shield)
	}
	before := next.CombatantByID("hero").Attributes.Health
	struck, hit := ResolveExchange(next, map[string]CombatAction{"foe": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"}}, drawByPurpose(map[string]float64{"hit": 0}))
	if step := findStep(t, hit, "foe"); step.Resolution.ShieldAbsorbed <= 0 {
		t.Fatalf("ward absorbed no real attack: %+v", hit)
	}
	if struck.CombatantByID("hero").Attributes.Health > before {
		t.Fatal("a ward must not heal")
	}
	expired, _ := ResolveExchange(struck, nil, fixedDraws(0.5))
	if expired.CombatantByID("hero").Shield.Amount != 0 {
		t.Fatal("ward did not expire after three exchanges")
	}
}

func TestWardTalismanDoesNotSpendAgainstStrongerShieldOrEmptyStock(t *testing.T) {
	definition, _ := NewItemCatalogWithFirstVersion().Definition("talisman.ward.lower")
	for _, exhausted := range []bool{false, true} {
		session := twoWayBattle(t)
		hero := session.CombatantByID("hero")
		quantity := 2
		if exhausted {
			quantity = 0
		} else {
			hero.Shield = Shield{Amount: 60, RemainingExchanges: 3, SourceID: "other"}
		}
		hero.Carried = []CarriedDose{{InstanceID: "ward", DefID: definition.ID, Quantity: quantity, Talisman: definition.Talisman}}
		next, outcome := ResolveExchange(session, map[string]CombatAction{"hero": useItem("ward")}, fixedDraws(0.5))
		if findStep(t, outcome, "hero").ItemApplied || next.CombatantByID("hero").Carried[0].Quantity != quantity {
			t.Fatalf("ineffective use spent stock: %+v", outcome)
		}
	}
}
