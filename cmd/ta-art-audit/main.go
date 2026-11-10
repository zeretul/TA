// ta-art-audit reports per-entry coverage against current author catalogs and
// actual embedded bytes. A shared preset is never counted as dedicated artwork.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"ta/internal/catalogart"
	"ta/internal/cultivation"
	"ta/internal/portrait"
	"ta/internal/storypack"
)

// Keep the audit scoped to shipped worlds; development-only fixtures are excluded.
var deliveredWorldIDs = []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689", "sui-guan-realms"}

type coverageSummary struct {
	Total          int `json:"total"`
	Reviewed       int `json:"reviewed"`
	LegacyEmbedded int `json:"legacy_embedded"`
	Missing        int `json:"missing"`
	Candidate      int `json:"candidate"`
	CoreTotal      int `json:"core_total"`
	CoreMissing    int `json:"core_missing"`
	OtherTotal     int `json:"other_total"`
	OtherMissing   int `json:"other_missing"`
}

type auditReport struct {
	Scope       string                     `json:"scope"`
	Counts      map[string]map[string]int  `json:"counts"`
	PendingCore int                        `json:"pending_core"`
	PendingAll  int                        `json:"pending_all"`
	ByWorld     map[string]coverageSummary `json:"by_world"`
	Warnings    []string                   `json:"warnings"`
	Entries     []row                      `json:"entries"`
}

type row struct {
	World    string   `json:"world"`
	Kind     string   `json:"kind"`
	ID       string   `json:"id"`
	Version  int      `json:"version"`
	Category string   `json:"category,omitempty"`
	Core     bool     `json:"core_scope"`
	Status   string   `json:"status"`
	AssetIDs []string `json:"asset_ids,omitempty"`
}

func audit(packs *storypack.Library, art *catalogart.Library, portraits *portrait.Library) (auditReport, error) {
	result := []row{}
	add := func(world, kind, id, category string, core bool) {
		r := row{World: world, Kind: kind, ID: id, Version: 1, Category: category, Core: core, Status: "missing"}
		roles := []string{"illustration"}
		if kind == "npc" {
			roles = []string{"avatar", "display"}
		}
		complete := true
		for _, role := range roles {
			hash := art.AssetID(world, kind, id, 1, role)
			if hash == "" {
				complete = false
				continue
			}
			if _, err := art.Read(art.CanonicalFile(hash + ".png")); err != nil {
				complete = false
				continue
			}
			r.AssetIDs = append(r.AssetIDs, hash)
		}
		if complete {
			r.Status = art.ReviewState(world, kind, id, 1)
		} else if kind == "npc" && world == "fanren-jie" {
			if entry, ok := portraits.Lookup(id); ok {
				if _, _, err := portraits.Read(entry.File, false); err == nil {
					if _, _, err := portraits.Read(entry.DisplayFile, true); err == nil {
						r.Status = "legacy_embedded"
					}
				}
			}
		}
		result = append(result, r)
	}
	for _, world := range deliveredWorldIDs {
		pack, err := packs.GetContent(world)
		if err != nil {
			return auditReport{}, err
		}
		for _, p := range pack.Characters {
			add(world, "npc", p.SourceKey, "portrait", true)
		}
		if world == "fanren-jie" {
			for _, d := range cultivation.TechniqueTable() {
				add(world, "definition", d.ID, "technique", true)
			}
			for _, d := range cultivation.SkillTable() {
				add(world, "definition", d.ID, "skill", true)
			}
			for _, d := range cultivation.NewItemCatalogWithFirstVersion().Definitions() {
				core := d.Kind != cultivation.KindMaterial && d.Kind != cultivation.KindPill
				category := "other_item"
				if core {
					category = "equipment"
				}
				add(world, "definition", d.ID, category, core)
			}
		} else if rules := pack.Story.Gameplay.AdventureRules; rules != nil {
			for _, d := range rules.Skills {
				add(world, "definition", d.ID, "skill", true)
			}
			for _, d := range rules.Recipes {
				add(world, "definition", d.ID, "recipe", false)
			}
			for _, d := range rules.Items {
				core := d.Kind == "weapon" || d.Kind == "armor" || d.Kind == "tool" || d.Kind == "machine" || d.Kind == "ship"
				category := "other_item"
				if core {
					category = "equipment"
				}
				add(world, "definition", d.ID, category, core)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		return a.World+"/"+a.Kind+"/"+a.ID < b.World+"/"+b.Kind+"/"+b.ID
	})
	counts := map[string]map[string]int{}
	missingCore := 0
	missingAll := 0
	byWorld := map[string]coverageSummary{}
	for _, r := range result {
		key := r.Kind
		if !r.Core {
			key = "other_item"
		}
		if counts[key] == nil {
			counts[key] = map[string]int{}
		}
		counts[key][r.Status]++
		complete := r.Status == "reviewed" || r.Status == "legacy_embedded"
		summary := byWorld[r.World]
		summary.Total++
		switch r.Status {
		case "reviewed":
			summary.Reviewed++
		case "legacy_embedded":
			summary.LegacyEmbedded++
		case "candidate":
			summary.Candidate++
		default:
			summary.Missing++
		}
		if r.Core {
			summary.CoreTotal++
			if !complete {
				missingCore++
				summary.CoreMissing++
			}
		} else {
			summary.OtherTotal++
			if !complete {
				summary.OtherMissing++
			}
		}
		if !complete {
			missingAll++
		}
		byWorld[r.World] = summary
	}

	expected := map[string]bool{}
	for _, r := range result {
		expected[fmt.Sprintf("%s/%s/%s/%d", r.World, r.Kind, r.ID, r.Version)] = true
	}
	warnings := []string{}
	originalOwners := map[string]string{}
	for _, entry := range art.EntriesForAudit() {
		key := fmt.Sprintf("%s/%s/%s/%d", entry.World, entry.Kind, entry.ID, entry.Version)
		if !expected[key] {
			warnings = append(warnings, "orphan artwork identity: "+key)
		}
		if owner := originalOwners[entry.OriginalSHA256]; owner != "" && owner != key {
			warnings = append(warnings, "duplicate original artwork: "+owner+" and "+key)
		}
		originalOwners[entry.OriginalSHA256] = key
	}
	sort.Strings(warnings)
	return auditReport{
		Scope:  "All four delivered worlds, including every NPC, skill, technique, item and adventure recipe. Core is the historical NPC/skill/equipment subset; other definitions are reported separately and remain required for all-entry completion. Shared archetypes and candidates do not count as final dedicated coverage. Private or future Story definitions are excluded.",
		Counts: counts, PendingCore: missingCore, PendingAll: missingAll,
		ByWorld: byWorld, Warnings: warnings, Entries: result,
	}, nil
}

func main() {
	requireComplete := flag.Bool("require-complete", false, "fail unless every core entry has reviewed dedicated artwork")
	requireAll := flag.Bool("require-all", false, "fail unless every catalog entry, including other items and recipes, has reviewed dedicated artwork")
	flag.Parse()
	root, err := os.MkdirTemp("", "ta-art-audit-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)
	report, err := audit(storypack.NewLibrary(root), catalogart.Default(), portrait.NewLibrary())
	if err != nil {
		panic(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		panic(err)
	}
	if (*requireComplete && report.PendingCore > 0) || (*requireAll && report.PendingAll > 0) || ((*requireComplete || *requireAll) && len(report.Warnings) > 0) {
		fmt.Fprintf(os.Stderr, "%d core entries and %d total entries still lack final dedicated artwork\n", report.PendingCore, report.PendingAll)
		os.Exit(1)
	}
}
