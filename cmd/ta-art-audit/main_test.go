package main

import (
	"ta/internal/catalogart"
	"ta/internal/portrait"
	"ta/internal/storypack"
	"testing"
)

func TestAuditIncludesFourWorldsRecipesAndOtherItems(t *testing.T) {
	packs := storypack.NewLibrary(t.TempDir())
	report, err := audit(packs, catalogart.Default(), portrait.NewLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ByWorld) != 4 {
		t.Fatalf("world summaries = %v", report.ByWorld)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("unexpected catalog warnings: %v", report.Warnings)
	}
	rows := map[string]row{}
	for _, r := range report.Entries {
		key := r.World + "/" + r.Kind + "/" + r.ID
		if _, exists := rows[key]; exists {
			t.Fatalf("duplicate identity %s", key)
		}
		rows[key] = r
	}
	for _, world := range deliveredWorldIDs {
		pack, err := packs.GetContent(world)
		if err != nil {
			t.Fatal(err)
		}
		rules := pack.Story.Gameplay.AdventureRules
		if rules == nil {
			continue
		}
		for _, recipe := range rules.Recipes {
			r, ok := rows[world+"/definition/"+recipe.ID]
			if !ok || r.Core || r.Category != "recipe" {
				t.Errorf("recipe %s absent or incorrectly classified: %+v", recipe.ID, r)
			}
		}
		want := len(pack.Characters) + len(rules.Skills) + len(rules.Items) + len(rules.Recipes)
		if got := report.ByWorld[world].Total; got != want {
			t.Errorf("%s rows=%d, want every author entry=%d", world, got, want)
		}
	}
	// The newly authored designs resolve only their exact stable identities.
	for _, id := range []string{"treasure.attack.middle", "treasure.defense.middle", "treasure.support.upper.effect"} {
		r := rows["fanren-jie/definition/"+id]
		if r.Status != "reviewed" || len(r.AssetIDs) != 1 {
			t.Errorf("new treasure art missing: %+v", r)
		}
	}
	// Every newly completed batch has exact identity-bound dedicated artwork.
	for _, key := range []string{
		"fanren-jie/definition/material.talisman.ink",
		"fanren-jie/definition/pill.restore.health.lower",
		"lanchuan-qunxia/definition/lanchuan-training-s01",
		"lanchuan-qunxia/definition/lanchuan-advanced-manual-1",
		"lanchuan-qunxia/definition/lanchuan-lake-map",
		"lanchuan-qunxia/definition/lanchuan-make-advanced-weapon-2",
		"xinghai-689/definition/xinghai-training-s36",
		"xinghai-689/definition/xinghai-advanced-manual-2",
		"xinghai-689/definition/xinghai-water-medal",
	} {
		r, ok := rows[key]
		if !ok || r.Status != "reviewed" || len(r.AssetIDs) != 1 {
			t.Errorf("completed batch entry is not bound to dedicated art: %s %+v", key, r)
		}
	}
	// The newly completed portrait and recipe must resolve their own variants.
	for key, count := range map[string]int{"lanchuan-qunxia/npc/lanchuan-p49": 2, "sui-guan-realms/definition/sg-recipe-blade": 1} {
		r, ok := rows[key]
		if !ok || r.Status != "reviewed" || len(r.AssetIDs) != count {
			t.Errorf("completed artwork misreported: %s %+v", key, r)
		}
	}
	// Exercise missing entries independently of the shipped catalog's coverage.
	emptyReport, err := audit(packs, &catalogart.Library{}, portrait.NewLibrary())
	if err != nil {
		t.Fatal(err)
	}
	if emptyReport.PendingAll <= emptyReport.PendingCore {
		t.Fatalf("other-entry backlog was hidden: %+v", emptyReport.ByWorld)
	}
	for _, r := range emptyReport.Entries {
		if r.World == "fanren-jie" && r.Status == "legacy_embedded" {
			continue
		}
		if r.Status != "missing" || len(r.AssetIDs) != 0 {
			t.Errorf("empty catalog falsely reported artwork: %+v", r)
		}
	}
	core, all, total := 0, 0, 0
	for _, summary := range report.ByWorld {
		core += summary.CoreMissing
		all += summary.CoreMissing + summary.OtherMissing
		total += summary.Total
		if summary.Total != summary.CoreTotal+summary.OtherTotal {
			t.Errorf("invalid summary: %+v", summary)
		}
	}
	if core != report.PendingCore || all != report.PendingAll || total != len(report.Entries) {
		t.Fatalf("summary totals disagree: core %d/%d all %d/%d rows %d/%d", core, report.PendingCore, all, report.PendingAll, total, len(report.Entries))
	}
}
