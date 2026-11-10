package interactive

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// This report begins from the real authored starting grants. The separate
// creation integration suite covers all origin selections and frozen journals.
func TestAdventureStartingLoadoutCombatBalance(t *testing.T) {
	before := balanceSourceHashes(t)
	rows := []map[string]any{}
	inputs := map[string]string{}
	for _, world := range []string{"xinghai-689", "lanchuan-qunxia"} {
		raw, err := os.ReadFile("../storypack/assets/" + world + ".json")
		if err != nil {
			t.Fatal(err)
		}
		inputs[world] = fmt.Sprintf("%x", sha256.Sum256(raw))
		r := authoredAdventureRules(t, world)
		kits := [][]AdventureStartingLoadout{}
		for _, kit := range r.StartingLoadouts {
			if kit.Group == "class" {
				kits = append(kits, []AdventureStartingLoadout{kit})
			}
			if kit.Group == "weapon" {
				for _, method := range r.StartingLoadouts {
					if method.Group == "method" {
						kits = append(kits, []AdventureStartingLoadout{kit, method})
					}
				}
			}
		}
		expected := 5
		if r.Family == "wuxia" {
			expected = 15
		}
		if len(kits) != expected {
			t.Fatalf("expected all %d starting builds, got %d", expected, len(kits))
		}
		for _, selection := range kits {
			ids := []string{}
			budget := 0
			for _, kit := range selection {
				ids = append(ids, kit.ID)
				for id, n := range kit.Items {
					for _, item := range r.Items {
						if item.ID == id {
							budget += item.Price * n
						}
					}
				}
			}
			initial, e := ApplyAdventureStartingLoadouts(r, NewAdventureActor(r), ids...)
			if e != nil {
				t.Fatal(e)
			}
			if initial.Rank != 1 || initial.Health != 100 || initial.Energy != 100 {
				t.Fatal("starting choice bypassed ordinary progression")
			}
			expectedBudget := 130
			if r.Family == "wuxia" {
				expectedBudget = 160
			}
			if budget != expectedBudget {
				t.Fatalf("%v: authored grant budget=%d expected=%d", ids, budget, expectedBudget)
			}
			authoredID := "xinghai-sparring"
			if r.Family == "wuxia" {
				authoredID = "lanchuan-sparring"
			}
			probes := []AdventureOpponent{{ID: authoredID}, {ID: "balance-soft-person", Name: "Conscious unarmored comparison", Scale: "person", Location: "isolated test", Rank: 1, Health: 100, Attack: 10, Defense: 0}, {ID: "balance-armored-person", Name: "Conscious armored comparison", Scale: "person", Location: "isolated test", Rank: 1, Health: 100, Attack: 10, Defense: 35}}
			for _, probe := range probes {
				for _, policy := range []string{"ordinary", "signature", "signature_and_method"} {
					if r.Family == "space" && policy == "signature_and_method" {
						continue
					}
					t.Run(ids[0]+"/"+func() string {
						if len(ids) > 1 {
							return ids[1]
						}
						return "class"
					}()+"/"+probe.ID+"/"+policy, func(t *testing.T) {
						rules := r
						if probe.Health > 0 {
							rules.Opponents = append(append([]AdventureOpponent(nil), r.Opponents...), probe)
						}
						actor, _, err := ResolveAdventureAction(rules, initial, AdventureAction{Action: "battle_start", OpponentID: probe.ID}, "starting-balance")
						if err != nil {
							t.Fatal(err)
						}
						rejected := 0
						for turn := 0; turn < 30 && actor.Battle.Status == "active"; turn++ {
							action := AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "ordinary"}
							if policy != "ordinary" {
								action.Choice = "skill"
								action.SkillID = selection[0].Skills[0]
							}
							if policy == "signature_and_method" && len(selection) > 1 && turn%2 == 1 {
								action.SkillID = selection[1].Skills[0]
							}
							next, _, e := ResolveAdventureAction(rules, actor, action, fmt.Sprintf("starting-%d", turn))
							if e != nil {
								rejected++
								if !reflect.DeepEqual(next, actor) {
									t.Fatal("refused starter action was not atomic")
								}
								next, _, e = ResolveAdventureAction(rules, actor, AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "ordinary"}, fmt.Sprintf("starting-%d", turn))
							}
							if e != nil {
								t.Fatal(e)
							}
							actor = next
						}
						row := map[string]any{"world": world, "loadouts": ids, "grant_list_value": budget, "target": probe.ID, "target_is_isolated_probe": probe.Health > 0, "policy": policy, "status": actor.Battle.Status, "rounds": actor.Battle.Round, "health": actor.Health, "energy": actor.Energy, "enemy_health": actor.Battle.Health, "refused_then_plain": rejected, "initial_equipment": initial.Equipment, "final_inventory": actor.Inventory}
						rows = append(rows, row)
						if probe.Health == 0 && actor.Battle.Status != "victory" {
							t.Fatalf("actual starting loadout cannot finish introductory training: %+v", row)
						}
						t.Logf("budget=%d status=%s rounds=%d hp=%d energy=%d enemy=%d rejected=%d", budget, actor.Battle.Status, actor.Battle.Round, actor.Health, actor.Energy, actor.Battle.Health, rejected)
					})
				}
			}
		}
	}
	after := balanceSourceHashes(t)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("mechanical sources changed during starter report")
	}
	for world, hash := range inputs {
		raw, _ := os.ReadFile("../storypack/assets/" + world + ".json")
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != hash {
			t.Fatal("author changed during starter report")
		}
	}
	if path := os.Getenv("TA_STARTER_REPORT"); path != "" {
		report := map[string]any{"format": 1, "scope": "20 actual starting loadouts; three finite opposition cases; 30-round ceiling reports active without forced winner", "assumptions": []string{"Default authored starting actor plus selected frozen grant, not all origin options. Real creation/origin/branch consumers are tested separately.", "All ranks start at one. Two named comparison opponents are isolated conscious probes, not invented author encounters. Raw price sums are grant budgets, not acquisition times.", "Policies are deliberately simple, not optimal play or live-model win rates. Invalid/unaffordable signature falls back to plain attack from the unchanged snapshot; rejected count is explicit.", "Wuxia signature_and_method alternates the selected weapon form and actual inner method. Resources, real equipment and costs are never replenished during a fight."}, "source_sha256": before, "author_sha256": inputs, "rows": rows}
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
