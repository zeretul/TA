package app

import (
	"ta/internal/cultivation"
	"testing"
)

func TestCombatWindowShowsWardEffectAndMatchesUseRefusal(t *testing.T) {
	definition, _ := cultivation.NewItemCatalogWithFirstVersion().Definition("talisman.ward.lower")
	actor := cultivation.Combatant{Cultivation: cultivation.Qi(6), Attributes: cultivation.BaseAttributes(cultivation.Qi(6)), Carried: []cultivation.CarriedDose{{InstanceID: "ward", DefID: definition.ID, NameKey: definition.NameKey, Quantity: 1, Talisman: definition.Talisman}}}
	items := combatWindowItems(actor)
	if len(items) != 1 || items[0].Purpose != "shield" || items[0].ShieldAmount == nil || *items[0].ShieldAmount <= 0 || items[0].DurationExchanges == nil || *items[0].DurationExchanges != 3 || !items[0].Usable {
		t.Fatalf("ward is not actionable in the window: %+v", items)
	}
	actor.Shield = cultivation.Shield{Amount: *items[0].ShieldAmount + 1, RemainingExchanges: 3, SourceID: "other"}
	items = combatWindowItems(actor)
	if items[0].Usable || items[0].ReasonKey != "reason.talisman_shield_active" {
		t.Fatalf("window diverges from the engine refusal: %+v", items)
	}
	actor.Carried[0].Quantity = 0
	items = combatWindowItems(actor)
	if items[0].Usable || items[0].ReasonKey != "reason.item_exhausted" {
		t.Fatalf("exhausted ward is actionable: %+v", items)
	}
}
