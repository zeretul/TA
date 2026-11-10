package cultivation

import (
	"math"
	"testing"
)

func TestRealmEquipmentEffectBudgetsAndRatedCapacity(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	realms := []string{"foundation", "core", "nascent", "spirit", "void", "integration", "mahayana"}
	attacks := []float64{.25, .26, .27, .28, .29, .30, .30}
	previousPierce := 0
	for i, realm := range realms {
		blade, ok := catalog.Definition("equipment." + realm + "_blade")
		if !ok {
			t.Fatalf("missing validated blade %s", realm)
		}
		robe, ok := catalog.Definition("equipment." + realm + "_robe")
		if !ok {
			t.Fatalf("missing validated robe %s", realm)
		}
		early, _ := ParseStageKey(realm + "_early")
		late, _ := ParseStageKey(realm + "_late")
		if math.Abs(blade.Implement.AttackBonus-attacks[i]) > 1e-9 || blade.Rarity == RarityUnique {
			t.Fatal("wrong split budget or generated-item exclusion")
		}
		if blade.RaritySpentOnMainStat+blade.RaritySpentOnSpecialEffect > rarityExtraBudget[blade.Rarity]+1e-9 {
			t.Fatal("rarity allowance was expanded")
		}
		pierce := blade.DefensePierceAmount(early)
		if pierce <= previousPierce || pierce != BaseAttributes(late).Defense/2 || blade.DefensePierceAmount(late) != pierce {
			t.Fatal("penetration is not a fixed increasing rated capacity")
		}
		previousPierce = pierce
		if math.Abs(robe.EquipmentStrainReduction(late)-.15) > 1e-9 || blade.DefensePierceAmount(Qi(13)) != 0 || robe.EquipmentStrainReduction(Qi(13)) != 0 {
			t.Fatal("effect bypassed floor or lost within-rating capacity")
		}
		if i+1 < len(realms) {
			higher, _ := ParseStageKey(realms[i+1] + "_early")
			if robe.EquipmentStrainReduction(higher) >= .15 || blade.DefensePierceAmount(higher) != pierce {
				t.Fatal("old item grew an unbounded special effect")
			}
		}
		double := blade
		double.RaritySpentOnMainStat = rarityExtraBudget[blade.Rarity]
		if double.Validate() == nil {
			t.Fatal("same rarity budget spent twice")
		}
		fake := blade
		fake.EffectStage = "mortal"
		if fake.Validate() == nil {
			t.Fatal("unrelated effect capacity accepted")
		}
		free := blade
		payload := *blade.Implement
		payload.OperatingManaFraction = 0
		free.Implement = &payload
		if free.Validate() == nil {
			t.Fatal("paid special effect became free")
		}
		excess := blade
		payload = *blade.Implement
		payload.DefensePierce++
		excess.Implement = &payload
		if excess.Validate() == nil {
			t.Fatal("unbudgeted absolute penetration")
		}
	}
	old, _ := catalog.Definition("implement.top.unique")
	if old.Implement.AttackBonus != .35 || old.OperatingManaFraction() != 0 || old.EffectStage != "" || old.Implement.DefensePierce != 0 || old.BasePrice != 500 {
		t.Fatal("legacy unique was silently nerfed or repriced")
	}
}
