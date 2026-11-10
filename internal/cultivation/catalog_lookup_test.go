package cultivation

import "testing"

func TestCatalogLookupIsLazyValidatedAndBranchLocal(t *testing.T) {
	base, _ := NewItemCatalogWithFirstVersion().Definition("implement.lower.common")
	base.ID = "generated.item"
	reads := 0
	catalog := NewItemCatalogWithFirstVersion().WithLookup(func(id string) (ItemDef, bool) {
		reads++
		return base, id == base.ID
	}, func() []string { return []string{base.ID} })
	if reads != 0 {
		t.Fatal("catalog construction read the archive")
	}
	if _, ok := catalog.Definition(base.ID); !ok || reads != 1 {
		t.Fatalf("lookup: %v reads=%d", ok, reads)
	}
	if _, ok := catalog.Definition(base.ID); !ok || reads != 1 {
		t.Fatal("resolved item was not cached")
	}
	if _, ok := catalog.Definition("wrong.identity"); ok {
		t.Fatal("lookup aliased a different identity")
	}
	if _, ok := NewItemCatalogWithFirstVersion().Definition(base.ID); ok {
		t.Fatal("definition leaked across catalogs")
	}
	if err := catalog.Register(base); err == nil {
		t.Fatal("duplicate item identity accepted")
	}

	template, _ := SkillTemplateByID("fire_bolt")
	skill, _ := template.Instantiate("雪火")
	reads = 0
	skills := NewSkillRegistryWithBuiltins().WithLookup(func(id string) (SkillDef, bool) { reads++; return skill, id == skill.ID }, func() []string { return []string{skill.ID} })
	if reads != 0 {
		t.Fatal("skill construction read the archive")
	}
	if _, ok := skills.Version(skill.ID, skill.Version); !ok || reads != 1 {
		t.Fatal("pinned skill version did not resolve lazily")
	}
	if _, ok := skills.Version(skill.ID, skill.Version+1); ok {
		t.Fatal("lookup fabricated a later version")
	}
	if err := skills.Register(skill); err == nil {
		t.Fatal("duplicate version accepted")
	}

	method, _ := NewCraftRegistryWithBuiltins().Method("craft.smithing.iron_sword")
	method.ID = "generated.method"
	reads = 0
	methods := NewCraftRegistryWithBuiltins().WithLookup(func(id string) (CraftMethodDef, bool) { reads++; return method, id == method.ID }, func() []string { return []string{method.ID} })
	if reads != 0 {
		t.Fatal("craft construction read archive")
	}
	found, ok := methods.Method(method.ID)
	if !ok || reads != 1 {
		t.Fatal("craft lookup failed")
	}
	found.Inputs[0].Quantity++
	repeated, _ := methods.Method(method.ID)
	if repeated.Inputs[0].Quantity != method.Inputs[0].Quantity || reads != 1 {
		t.Fatal("lookup did not isolate returned recipe slices")
	}
}
