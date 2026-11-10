package interactive

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"ta/internal/cultivation"
	"testing"
)

// This opt-in quantitative report exercises the real solvers on explicitly
// staged loadouts. It measures rules, not acquisition, natural-model play, or
// an assertion that equal ranks in different genres should have equal power.
// Run with TA_BALANCE_REPORT=/absolute/report.json and -run TestCombatBalanceReport.
func TestCombatBalanceReport(t *testing.T) {
	path := os.Getenv("TA_BALANCE_REPORT")
	if path == "" {
		t.Skip("set TA_BALANCE_REPORT to write the deterministic rules report")
	}
	sourceBefore := balanceSourceHashes(t)
	report := map[string]any{"format": 5, "revision": os.Getenv("TA_BALANCE_REVISION"), "assumptions": []string{
		"Staged snapshots explicitly grant listed learned skills, equipment and 100 operating units; they are not acquisition proofs. Mastered trial profiles keep the same rank/equipment and raise only learned skill mastery to the legal world cap.",
		"Adventure ranks 1/4/7/10, every authored stage boundary and maximum use canonical protagonist pools; named encounters also get exact same-rank comparisons. Actual bound NPC pools, learned mastery, equipment and owned consumables are retained.",
		"Adventure repeats one offensive technique; selected advanced trial policies also use the strongest legal heal only at <=60% own health with a full missing-heal amount. No hidden opponent search. Invalid skills fall back to ordinary attack, then guard; actual resolved skill rounds, control-forced guard and rejection reasons are explicit.",
		"Cultivation uses 16 deterministic draw streams paired with actor draws swapped (32 fights), 80 exchanges maximum, and the existing tactic policy. An invalid player intent is retried from the same prior snapshot with plain attack, then rest, never committed as a rejected command.",
		"Each world keeps its own scale; raw damage and calendar time are not forced equal across genres.",
		"This legacy-comparable matrix uses same-scale battles. Personal-versus-fleet, retreat and creation builds are separate actual-consumer reports/tests, not silently represented by ship controls.",
		"Weapon-bound skills use a real owned qualifying weapon. Class alternatives stay within the staged ordinary weapon power envelope where possible; the weakest qualifying item is explicit otherwise. Bare profiles omit weapon-bound skills. Mental skills omit explicitly mindless targets. These new legal rows are not one-to-one with the v5 enumeration.",
		"The all-method stacking stress case explicitly grants the split-channel trait at Foundation or above; it measures capped legal combinations, not acquisition.",
	}}
	report["scope"] = os.Getenv("TA_BALANCE_SCOPE")
	report["cultivation"] = cultivationBalanceRows(t)
	report["rated_equipment"] = ratedEquipmentBalanceRows(t)
	var fights, growth, economy, ranks, npcSnapshots []map[string]any
	inputs := map[string]string{}
	for _, pack := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689"} {
		raw, err := os.ReadFile("../storypack/assets/" + pack + ".json")
		if err != nil {
			t.Fatal(err)
		}
		inputs[pack] = fmt.Sprintf("%x", sha256.Sum256(raw))
		if pack == "fanren-jie" {
			continue
		}
		r := authoredAdventureRules(t, pack)
		totalXP := 0
		for rank := 1; rank <= AdventureMaxRank(r); rank++ {
			row := map[string]any{"family": r.Family, "rank": rank, "cumulative_xp": totalXP, "base_health": 100 + 20*(rank-1), "base_energy": 100 + 10*(rank-1), "base_attack": 10 + 3*rank, "base_defense": rank}
			if stage := AdventureProgressionStageFor(r, rank); stage != nil {
				row["stage"] = stage.ID
				row["stage_name"] = stage.Name
			}
			if r.Progression != nil {
				row["refine_hours_only"] = float64(totalXP) / float64(r.Progression.ExperiencePerHour)
				row["refine_supplies_only"] = float64(totalXP*r.Progression.CurrencyPerHour) / float64(r.Progression.ExperiencePerHour)
			}
			ranks = append(ranks, row)
			totalXP += AdventureRankExperience(r, rank)
		}
		npcSnapshots = append(npcSnapshots, adventureNPCBalanceSnapshots(t, r)...)
		fights = append(fights, adventureBalanceRows(t, r)...)
		t.Logf("completed %s; total adventure samples now %d", r.Family, len(fights))
		for _, skill := range r.Skills {
			cap := 10
			if r.Family == "wuxia" {
				cap = 4
			}
			growth = append(growth, map[string]any{"family": r.Family, "skill": skill.ID, "name": skill.Name, "required_rank": skill.RequiredRank, "hours_zero_to_max": skill.TrainingHours * cap * (cap + 1) / 2, "max_mastery": cap, "cost_per_use": skill.Cost, "base_power": skill.Power})
		}
		items := map[string]AdventureItem{}
		for _, item := range r.Items {
			items[item.ID] = item
		}
		for _, market := range r.Markets {
			if market.WorkService == nil {
				continue
			}
			w := market.WorkService
			for _, item := range r.Items {
				if item.Price <= 0 || (item.Kind != "weapon" && item.Kind != "armor" && item.Kind != "machine" && item.Kind != "ship") {
					continue
				}
				economy = append(economy, map[string]any{"family": r.Family, "market": market.ID, "item": item.ID, "item_price": item.Price, "labor_hours_price_only": float64(item.Price*w.Hours) / float64(w.Pay), "work_energy_per_hour": float64(w.EnergyCost) / float64(w.Hours), "excludes": "rest, materials, travel and source qualification"})
			}
		}
		for _, recipe := range r.Recipes {
			inputValue := 0
			for id, n := range recipe.Materials {
				inputValue += items[id].Price * n
			}
			output := items[recipe.OutputID]
			economy = append(economy, map[string]any{"family": r.Family, "recipe": recipe.ID, "hours": recipe.Hours, "material_list_value": inputValue, "sale_value": output.Price / 2 * recipe.OutputCount, "net_list_value": output.Price/2*recipe.OutputCount - inputValue, "fuel": recipe.FuelCost, "required_asset": recipe.RequiredAsset})
		}
	}
	report["input_sha256"], report["adventure"], report["skill_growth"], report["economy"], report["rank_growth"] = inputs, fights, growth, economy, ranks
	solvers := balanceSourceHashes(t)
	for name, before := range sourceBefore {
		if solvers[name] != before {
			t.Fatalf("balance source changed during report: %s; rerun on a stable checkpoint", name)
		}
	}
	for pack, before := range inputs {
		raw, err := os.ReadFile("../storypack/assets/" + pack + ".json")
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != before {
			t.Fatalf("author pack changed during report: %s", pack)
		}
	}
	report["npc_snapshots"] = npcSnapshots
	report["solver_sha256"] = sourceBefore
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d adventure fights and cultivation stage samples to %s", len(fights), path)
}

func cultivationBalanceRows(t *testing.T) map[string]any {
	t.Helper()
	var stages, duels, coverage []map[string]any
	strike, _ := cultivation.SkillByID("strike")
	var arsenal []cultivation.SkillDef
	for _, id := range []string{"fire_bolt", "heavy_thunder", "condensed_blade", "shield"} {
		skill, ok := cultivation.SkillByID(id)
		if !ok {
			t.Fatal(id)
		}
		arsenal = append(arsenal, skill)
	}
	var representatives []cultivation.Cultivation
	for _, key := range cultivation.StageKeys() {
		stage, _ := cultivation.ParseStageKey(key)
		attrs := cultivation.BaseAttributes(stage)
		required, monthly, advances := cultivation.ProgressRequired(stage)
		life, known := cultivation.LifespanRuleFor(stage.Realm)
		row := map[string]any{"stage": key, "attributes": attrs, "lifespan_years": life.Years, "lifespan_known": known, "no_fixed_lifespan": life.NoFixedLimit, "growth_defined": advances, "terminal": cultivation.AtTerminalStage(stage)}
		if monthly > 0 {
			row["ordinary_months_to_boundary"] = required / monthly
		}
		stages = append(stages, row)
		if stage.Realm == cultivation.RealmQi && (stage.Layer == 1 || stage.Layer == 6 || stage.Layer == 13) || stage.Stage == cultivation.StageEarly {
			representatives = append(representatives, stage)
		}
	}
	for i, stage := range representatives {
		legal := append([]cultivation.SkillDef{}, arsenal...)
		var presetIDs []string
		for _, suffix := range []string{"_pierce", "_wave", "_ward"} {
			if skill, ok := cultivation.SkillByID(string(stage.Realm) + suffix); ok && skill.CanCast(stage) {
				legal = append(legal, skill)
				presetIDs = append(presetIDs, skill.ID)
			}
		}
		var legalMethods, legalItems []string
		var equipmentEconomy []map[string]any
		for _, method := range cultivation.TechniqueTable() {
			if stageAtLeastForBalance(stage, method.MinimumStage) {
				legalMethods = append(legalMethods, method.ID)
			}
		}
		for _, item := range cultivation.NewItemCatalogWithFirstVersion().Definitions() {
			if item.UsableBy(stage) == nil && strings.HasPrefix(item.ID, "equipment."+string(stage.Realm)+"_") {
				legalItems = append(legalItems, item.ID)
				equipmentEconomy = append(equipmentEconomy, map[string]any{"id": item.ID, "base_price": item.BasePrice, "operating_mana": item.OperatingManaCost(stage), "rated_pierce": item.DefensePierceAmount(stage), "rated_strain_reduction": item.EquipmentStrainReduction(stage)})
			}
		}
		coverage = append(coverage, map[string]any{"stage": cultivation.StageKey(stage), "realm_skill_ids": presetIDs, "eligible_method_ids": legalMethods, "realm_equipment_ids": legalItems, "realm_equipment_economy": equipmentEconomy})
		modes := []string{"ordinary_mirror", "learned_vs_ordinary", "learned_mirror", "method_vs_bare", "equipment_vs_bare", "build_vs_bare", "lower_vs_next_stage"}
		if len(presetIDs) > 0 {
			modes = append(modes, "realm_build_vs_bare", "realm_build_mirror", "realm_stack_vs_bare")
			if _, ok := cultivation.TechniqueByID("method." + string(stage.Realm) + "_wind_path"); ok {
				modes = append(modes, "realm_wind_vs_still", "realm_still_vs_wind")
			}
		}
		for _, mode := range modes {
			target := stage
			if mode == "lower_vs_next_stage" {
				if i+1 == len(representatives) {
					continue
				}
				target = representatives[i+1]
			}
			wins, rounds, unresolved, mana, rejected := 0, 0, 0, 0, 0
			reasons := map[string]int{}
			for sample := 0; sample < 32; sample++ {
				a := cultivation.Combatant{ID: "hero", Side: cultivation.SidePlayer, Cultivation: stage, Attributes: cultivation.BaseAttributes(stage)}
				b := cultivation.Combatant{ID: "foe", Side: cultivation.SideEnemy, Cultivation: target, Attributes: cultivation.BaseAttributes(target)}
				if mode == "learned_vs_ordinary" || mode == "learned_mirror" || mode == "build_vs_bare" || mode == "lower_vs_next_stage" {
					a.Skills = arsenal
				}
				applyBalanceCultivationBuild(&a, mode)
				if strings.HasPrefix(mode, "realm_") {
					a.Skills = legal
					if mode != "realm_build_vs_bare" && mode != "realm_stack_vs_bare" {
						b.Skills = legal
						other := "realm_build_mirror"
						if mode == "realm_wind_vs_still" {
							other = "realm_still_vs_wind"
						}
						if mode == "realm_still_vs_wind" {
							other = "realm_wind_vs_still"
						}
						applyBalanceCultivationBuild(&b, other)
					}
				}
				if mode == "learned_mirror" || mode == "lower_vs_next_stage" {
					b.Skills = arsenal
				}
				s := cultivation.CombatSession{ID: "balance", Combatants: []cultivation.Combatant{a, b}, Round: 1}
				for n := 0; n < 80 && !s.Ended; n++ {
					actions := map[string]cultivation.CombatAction{"hero": cultivation.DecideEnemyAction(s, "hero", strike), "foe": cultivation.DecideEnemyAction(s, "foe", strike)}
					draws := func(purpose, actor, target string) float64 {
						if sample%2 == 1 {
							swap := func(id string) string {
								if id == "hero" {
									return "foe"
								}
								if id == "foe" {
									return "hero"
								}
								return id
							}
							actor, target = swap(actor), swap(target)
						}
						h := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%s/%s/%s", sample/2, n, purpose, actor, target)))
						return float64(binary.BigEndian.Uint64(h[:8])>>11) / float64(uint64(1)<<53)
					}
					for attempt := 0; attempt < 3; attempt++ {
						next, result := cultivation.ResolveExchange(s, actions, draws)
						invalidPlayer := false
						for _, step := range result.Steps {
							if step.ActionRejected {
								rejected++
								reasons[step.ReasonKey]++
								invalidPlayer = invalidPlayer || step.ActorID == "hero"
							}
						}
						if invalidPlayer && attempt < 2 {
							if attempt == 0 {
								actions["hero"] = cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "foe"}
							} else {
								actions["hero"] = cultivation.CombatAction{Kind: cultivation.ActionRest}
							}
							continue
						}
						if invalidPlayer {
							t.Fatalf("uncommittable balance action: %s/%s", cultivation.StageKey(stage), mode)
						}
						s = next
						break
					}
					rounds++
				}
				if !s.Ended {
					unresolved++
				}
				if s.CombatantByID("hero").Attributes.Health > 0 && s.CombatantByID("foe").Attributes.Health <= 0 {
					wins++
				}
				mana += a.Attributes.Mana - s.CombatantByID("hero").Attributes.Mana
			}
			profile := cultivation.Combatant{Cultivation: stage, Attributes: cultivation.BaseAttributes(stage)}
			applyBalanceCultivationBuild(&profile, mode)
			duels = append(duels, map[string]any{"stage": cultivation.StageKey(stage), "target": cultivation.StageKey(target), "policy": mode, "samples": 32, "wins": wins, "mean_rounds": float64(rounds) / 32, "mean_mana_spent": float64(mana) / 32, "unresolved": unresolved, "rejected_actions": rejected, "rejection_reasons": reasons, "player_attributes": profile.Attributes, "player_equipment": profile.Equipment, "player_method_modifiers": profile.Modifiers, "mana_reduction": profile.ManaReduction, "strain_reduction": profile.StrainReduction, "split_channel_trait_assumed": mode == "realm_stack_vs_bare"})
		}
	}
	return map[string]any{"stages": stages, "duels": duels, "methods": cultivation.TechniqueTable(), "coverage": coverage}
}

func adventureBalanceRows(t *testing.T, rules AdventureRules) []map[string]any {
	t.Helper()
	var rows []map[string]any
	items := map[string]AdventureItem{}
	for _, item := range rules.Items {
		items[item.ID] = item
	}
	rankSet := map[int]bool{1: true, 4: true, 7: true, 10: true, AdventureMaxRank(rules): true}
	if rules.Progression != nil {
		for _, stage := range rules.Progression.Stages {
			rankSet[stage.MinRank] = true
		}
	}
	standardRanks := map[int]bool{}
	for rank := range rankSet {
		standardRanks[rank] = true
	}
	for _, opponent := range rules.Opponents {
		if opponent.CharacterSourceKey != "" {
			rankSet[opponent.Rank] = true
		}
	}
	var stageRanks []int
	for rank := range rankSet {
		if rank <= AdventureMaxRank(rules) {
			stageRanks = append(stageRanks, rank)
		}
	}
	sort.Ints(stageRanks)
	for _, rank := range stageRanks {
		mastery := min(rank, AdventureSkillMasteryCap(rules))
		if rules.Family == "wuxia" {
			mastery = min(AdventureSkillMasteryCap(rules), (rank+2)/3)
		}
		for _, opponent := range rules.Opponents {
			if !standardRanks[rank] && rank != opponent.Rank {
				continue
			}
			if os.Getenv("TA_BALANCE_SCOPE") == "trials" && (!strings.Contains(opponent.ID, "advanced-trial-") || rank != opponent.Rank && rank != AdventureMaxRank(rules)) {
				continue
			}
			var policies []AdventureSkill
			policies = append(policies, AdventureSkill{})
			for _, skill := range rules.Skills {
				if skill.RequiredRank > rank || skill.Scale != opponent.Scale {
					continue
				}
				effects := skill.Effects
				if len(effects) == 0 {
					effects = []AdventureEffect{{Kind: skill.Kind, Power: skill.Power}}
				}
				for _, effect := range effects {
					if effect.Kind == "attack" || effect.Kind == "mental_attack" && !opponent.Mindless || effect.Kind == "control" || effect.Kind == "poison" || effect.Kind == "reflect" {
						policies = append(policies, skill)
						if strings.Contains(opponent.ID, "advanced-trial-") && (strings.Contains(skill.ID, "advanced-strike-") || strings.Contains(skill.ID, "advanced-balance-")) {
							adaptive := skill
							adaptive.Name = "balance_recover_policy"
							policies = append(policies, adaptive)
						}
						break
					}
				}
			}
			for _, skill := range policies {
				for _, loadout := range []string{"bare", "equipped", "mastered"} {
					mastery := mastery
					if loadout == "mastered" {
						if !strings.Contains(opponent.ID, "advanced-trial-") || rank != opponent.Rank || mastery == AdventureSkillMasteryCap(rules) {
							continue
						}
						mastery = AdventureSkillMasteryCap(rules)
					}
					if loadout == "bare" && opponent.Scale != "person" {
						continue
					}
					r := rules
					a := NewAdventureActor(r)
					a.Rank, a.MaxHealth, a.MaxEnergy = rank, 100+20*(rank-1), 100+10*(rank-1)
					a.Health, a.Energy = a.MaxHealth, a.MaxEnergy
					weapon, armor, asset := "", "", ""
					if r.Family == "wuxia" {
						weapon = "lanchuan-rusty-blade"
						if rank >= 4 {
							weapon, armor = "lanchuan-i01", "lanchuan-i11"
						}
						if rank >= 7 {
							weapon = "lanchuan-i05"
						}
					} else {
						if rank >= 4 {
							weapon, armor = "xinghai-i01", "xinghai-i12"
						}
						if rank >= 7 {
							weapon = "xinghai-i03"
						}
						if rank >= 10 {
							weapon = "xinghai-i08"
						}
					}
					if r.Progression != nil {
						prefix := "xinghai"
						if r.Family == "wuxia" {
							prefix = "lanchuan"
						}
						for index, stage := range r.Progression.Stages {
							if index > 0 && stage.MinRank <= rank {
								candidate := fmt.Sprintf("%s-advanced-weapon-%d", prefix, index)
								if items[candidate].ID != "" {
									weapon = candidate
									armor = fmt.Sprintf("%s-advanced-armor-%d", prefix, index)
								}
							}
						}
					}
					if loadout == "bare" {
						if skill.RequiredWeapon != "" || skill.RequiredWeaponClass != "" {
							continue
						}
						weapon, armor = "", ""
					} else {
						weapon = adventureBalanceWeaponForSkill(r, skill, weapon)
					}
					if opponent.Scale != "person" {
						weapon, armor = "", ""
						asset = "xinghai-i09"
						if opponent.Scale == "ship" {
							asset = "xinghai-frigate"
						}
						a.Inventory[asset] = 1
						a.Skills[items[asset].RequiredSkill] = 1
						a.Equipment[opponent.Scale+":asset"] = asset
						a.Assets[asset] = AdventureAsset{Health: items[asset].Durability, MaxHealth: items[asset].Durability, Shield: items[asset].Power, Fuel: 100}
					}
					for slot, id := range map[string]string{"weapon": weapon, "armor": armor} {
						if id == "" {
							continue
						}
						a.Inventory[id], a.Equipment[opponent.Scale+":"+slot] = 1, id
						for material := range items[id].Materials {
							a.Inventory[material] = 100
						}
					}
					if skill.ID != "" {
						a.Skills[skill.ID] = mastery
						if skill.RequiredTool != "" {
							a.Inventory[skill.RequiredTool] = 1
						}
						for material := range skill.Materials {
							a.Inventory[material] = 100
						}
					}
					heal := AdventureSkill{}
					if skill.Name == "balance_recover_policy" {
						for _, candidate := range r.Skills {
							if candidate.Scale != opponent.Scale || candidate.RequiredRank > rank || candidate.RequiredTool != "" || len(candidate.Materials) > 0 {
								continue
							}
							effects := candidate.Effects
							if len(effects) == 0 {
								effects = []AdventureEffect{{Kind: candidate.Kind, Power: candidate.Power}}
							}
							if len(effects) == 1 && effects[0].Kind == "heal" && effects[0].Power > heal.Power {
								heal = candidate
								heal.Power = effects[0].Power
							}
						}
						if heal.ID != "" {
							a.Skills[heal.ID] = mastery
						}
					}
					if !adventureSkillWeaponAvailable(r, skill, a.Inventory, a.Equipment) {
						t.Fatalf("staged weapon does not qualify for %s/%s/%s", r.Family, loadout, skill.ID)
					}
					started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: opponent.ID}, "balance")
					if err != nil {
						t.Fatalf("start %s/%d/%s: %v", r.Family, rank, opponent.ID, err)
					}
					for _, npc := range r.NPCs {
						if npc.CharacterSourceKey == opponent.CharacterSourceKey {
							bindAdventureEncounter(started.Battle, "balance-npc", npc.Actor, r)
							break
						}
					}
					// Keep frozen NPC definitions: named encounters validate their source even after binding.
					// This solver fixture advances no NPC world clock.
					current := started
					rejected, rounds := 0, 0
					committedSkillRounds, ordinaryRounds, guardRounds, controlGuardRounds := 0, 0, 0, 0
					unresolvedInputError := ""
					rejectionReasons := map[string]int{}
					for rounds < 60 && current.Battle.Status == "active" {
						input := AdventureAction{Action: "battle_round", Choice: "attack"}
						if skill.ID != "" {
							input.Choice, input.SkillID = "skill", skill.ID
						}
						if heal.ID != "" && current.Health*100 <= current.MaxHealth*60 && current.MaxHealth-current.Health >= heal.Power*mastery && current.Energy >= heal.Cost {
							input.Choice, input.SkillID = "skill", heal.ID
						}
						next, _, err := ResolveAdventureActionAt(r, current, input, fmt.Sprintf("balance-%d", rounds), int64(rounds*20))
						committedChoice := input.Choice
						if err != nil {
							rejected++
							rejectionReasons[err.Error()]++
							committedChoice = "attack"
							next, _, err = ResolveAdventureActionAt(r, current, AdventureAction{Action: "battle_round", Choice: "attack"}, fmt.Sprintf("balance-%d", rounds), int64(rounds*20))
						}
						if err != nil {
							committedChoice = "guard"
							next, _, err = ResolveAdventureActionAt(r, current, AdventureAction{Action: "battle_round", Choice: "guard"}, fmt.Sprintf("balance-%d", rounds), int64(rounds*20))
						}
						if err != nil {
							unresolvedInputError = err.Error()
							break
						}
						if current.Control > 0 && !current.SoftLimbs {
							committedChoice = "guard"
							controlGuardRounds++
						}
						switch committedChoice {
						case "skill":
							committedSkillRounds++
						case "attack":
							ordinaryRounds++
						case "guard":
							guardRounds++
						}
						current = next
						rounds++
					}
					spent := map[string]int{}
					value := 0
					for id, n := range started.Inventory {
						if delta := n - current.Inventory[id]; delta > 0 {
							spent[id] = delta
							value += items[id].Price * delta
						}
					}
					policy := skill.ID
					if heal.ID != "" {
						policy += "+recover:" + heal.ID
					}
					if policy == "" {
						policy = "ordinary"
					}
					rows = append(rows, map[string]any{"family": r.Family, "rank": rank, "mastery": mastery, "loadout": loadout, "policy": policy, "opponent": opponent.ID, "bound_npc": started.Battle.EnemyActorID != "", "enemy_source_key": opponent.CharacterSourceKey, "enemy_name": opponent.Name, "enemy_rank": started.Battle.EnemyRank, "enemy_base_attack": started.Battle.Attack, "enemy_base_defense": started.Battle.Defense, "enemy_max_energy": started.Battle.EnemyMaxEnergy, "enemy_weapon": started.Battle.EnemyEquipment[opponent.Scale+":weapon"], "enemy_armor": started.Battle.EnemyEquipment[opponent.Scale+":armor"], "weapon": weapon, "armor": armor, "asset": asset, "recovery_skill": heal.ID, "rounds": rounds, "status": current.Battle.Status, "player_health": current.Health, "max_health": current.MaxHealth, "enemy_health": current.Battle.Health, "enemy_max_health": started.Battle.MaxHealth, "energy_spent": started.Energy - current.Energy, "materials_spent": spent, "material_list_value_spent": value, "rejected_skill_attempts": rejected, "rejection_reasons": rejectionReasons, "committed_skill_rounds": committedSkillRounds, "ordinary_rounds": ordinaryRounds, "guard_rounds": guardRounds, "control_forced_guard_rounds": controlGuardRounds, "unresolved_input_error": unresolvedInputError, "no_progress_rounds": current.Battle.NoProgressRounds, "enemy_ceasefire_offer": current.Battle.EnemyCeasefireOffer, "enemy_last_action": current.Battle.EnemyLastAction, "enemy_last_skill": current.Battle.EnemyLastSkillID, "enemy_energy_remaining": current.Battle.EnemyEnergy, "asset_state": current.Assets[asset]})
				}

			}
		}
		t.Logf("completed %s rank=%d, cumulative samples=%d", rules.Family, rank, len(rows))
	}
	sort.Slice(rows, func(i, j int) bool {
		return fmt.Sprint(rows[i]["rank"], rows[i]["opponent"], rows[i]["policy"]) < fmt.Sprint(rows[j]["rank"], rows[j]["opponent"], rows[j]["policy"])
	})
	return rows
}

// Match real weapon gates without pretending that an arbitrary strongest blade
// can cast a sword form. This is an explicitly staged comparison, not a shop or
// acquisition solver; every selected item and all consumed ammunition are saved.
func adventureBalanceWeaponForSkill(r AdventureRules, skill AdventureSkill, ordinary string) string {
	if skill.RequiredWeapon != "" {
		return skill.RequiredWeapon
	}
	if skill.RequiredWeaponClass == "" {
		return ordinary
	}
	ceiling := 0
	for _, item := range r.Items {
		if item.ID == ordinary {
			ceiling = item.Power
		}
	}
	var weakest, strongest AdventureItem
	for _, item := range r.Items {
		if item.Kind != "weapon" || item.Scale != skill.Scale || item.WeaponClass != skill.RequiredWeaponClass {
			continue
		}
		if weakest.ID == "" || item.Power < weakest.Power {
			weakest = item
		}
		if item.Power <= ceiling && (strongest.ID == "" || item.Power > strongest.Power) {
			strongest = item
		}
	}
	if strongest.ID != "" {
		return strongest.ID
	}
	return weakest.ID
}

func TestCombatBalanceWeaponProfilesRespectRealRequirements(t *testing.T) {
	r := AdventureRules{Items: []AdventureItem{
		{ID: "stage-blade", Kind: "weapon", Scale: "person", WeaponClass: "blade", Power: 20},
		{ID: "starter-sword", Kind: "weapon", Scale: "person", WeaponClass: "sword", Power: 5},
		{ID: "stage-sword", Kind: "weapon", Scale: "person", WeaponClass: "sword", Power: 15},
		{ID: "legendary-sword", Kind: "weapon", Scale: "person", WeaponClass: "sword", Power: 50},
		{ID: "specific-gun", Kind: "weapon", Scale: "person", WeaponClass: "pistol", Power: 6},
	}}
	for _, tc := range []struct {
		skill          AdventureSkill
		ordinary, want string
	}{
		{AdventureSkill{Scale: "person", RequiredWeaponClass: "sword"}, "stage-blade", "stage-sword"},
		{AdventureSkill{Scale: "person", RequiredWeaponClass: "sword"}, "", "starter-sword"},
		{AdventureSkill{Scale: "person", RequiredWeapon: "specific-gun"}, "stage-blade", "specific-gun"},
		{AdventureSkill{Scale: "person"}, "stage-blade", "stage-blade"},
	} {
		got := adventureBalanceWeaponForSkill(r, tc.skill, tc.ordinary)
		if got != tc.want || !adventureSkillWeaponAvailable(r, tc.skill, map[string]int{got: 1}, map[string]string{"person:weapon": got}) {
			t.Fatalf("illegal staged weapon: got=%s want=%s", got, tc.want)
		}
	}
}

// Uses actual catalog payloads and eligibility, not hand-written combat bonuses.
func applyBalanceCultivationBuild(actor *cultivation.Combatant, mode string) {
	stageOrder := map[string]int{}
	for i, key := range cultivation.StageKeys() {
		stageOrder[key] = i
	}
	position := stageOrder[cultivation.StageKey(actor.Cultivation)]
	if mode == "method_vs_bare" || mode == "build_vs_bare" {
		var chosen cultivation.TechniqueDef
		best := 0.0
		for _, method := range cultivation.TechniqueTable() {
			if stageOrder[method.MinimumStage] > position {
				continue
			}
			score := method.Specialty[cultivation.AttrAttack] + method.QualityAttack
			if score > best {
				chosen, best = method, score
			}
		}
		if chosen.ID != "" {
			actor.Modifiers = cultivation.TechniqueModifiers([]cultivation.TechniqueDef{chosen})
			actor.ManaReduction = chosen.MPCostReduction
			actor.StrainReduction = chosen.StrainReduction
		}
	}
	if mode == "equipment_vs_bare" || mode == "build_vs_bare" {
		var weapon, armor cultivation.ItemDef
		for _, item := range cultivation.NewItemCatalogWithFirstVersion().Definitions() {
			if item.UsableBy(actor.Cultivation) != nil {
				continue
			}
			if item.Implement != nil && (weapon.Implement == nil || item.Implement.AttackBonus > weapon.Implement.AttackBonus) {
				weapon = item
			}
			if item.Armor != nil && (armor.Armor == nil || item.Armor.DefenseBonus > armor.Armor.DefenseBonus) {
				armor = item
			}
		}
		if weapon.Implement != nil {
			actor.Equipment = append(actor.Equipment, cultivation.EquipmentCopy{InstanceID: "balance-weapon", DefID: weapon.ID, Slot: cultivation.EquipmentSlotWeapon, ApplicableStage: weapon.ApplicableStage, OperatingManaCost: weapon.OperatingManaCost(actor.Cultivation), DefensePierce: weapon.DefensePierceAmount(actor.Cultivation), Modifiers: []cultivation.AttributeModifier{cultivation.EquipmentModifier(weapon.ID, cultivation.AttrAttack, weapon.Implement.AttackBonus)}})
		}
		if armor.Armor != nil {
			actor.Equipment = append(actor.Equipment, cultivation.EquipmentCopy{InstanceID: "balance-armor", DefID: armor.ID, Slot: cultivation.EquipmentSlotGarment, ApplicableStage: armor.ApplicableStage, OperatingManaCost: armor.OperatingManaCost(actor.Cultivation), StrainReduction: armor.EquipmentStrainReduction(actor.Cultivation), Modifiers: []cultivation.AttributeModifier{{ID: armor.ID, Kind: cultivation.SourceArmor, Attribute: cultivation.AttrDefense, Value: armor.Armor.DefenseBonus}}})
		}
	}
	if strings.HasPrefix(mode, "realm_") {
		prefix := string(actor.Cultivation.Realm)
		id := map[string]string{"foundation": "method.basic_breathing", "core": "method.core_continuation", "nascent": "method.nascent_continuation", "spirit": "method.spirit_continuation", "void": "method.void_unification", "integration": "method.integration_return", "mahayana": "method.mahayana_insight"}[prefix]
		if mode == "realm_wind_vs_still" {
			id = "method." + prefix + "_wind_path"
		}
		if mode == "realm_still_vs_wind" {
			id = "method." + prefix + "_still_path"
		}
		var methods []cultivation.TechniqueDef
		for _, method := range cultivation.TechniqueTable() {
			if method.ID == id || mode == "realm_stack_vs_bare" && stageAtLeastForBalance(actor.Cultivation, method.MinimumStage) {
				methods = append(methods, method)
				actor.ManaReduction = min(.20, actor.ManaReduction+method.MPCostReduction)
				actor.StrainReduction = max(actor.StrainReduction, method.StrainReduction)
			}
		}
		actor.Modifiers = cultivation.TechniqueModifiers(methods)
		for _, item := range cultivation.NewItemCatalogWithFirstVersion().Definitions() {
			if item.UsableBy(actor.Cultivation) != nil {
				continue
			}
			if item.ID == "equipment."+prefix+"_blade" {
				actor.Equipment = append(actor.Equipment, cultivation.EquipmentCopy{InstanceID: "realm-weapon", DefID: item.ID, Slot: cultivation.EquipmentSlotWeapon, ApplicableStage: item.ApplicableStage, OperatingManaCost: item.OperatingManaCost(actor.Cultivation), DefensePierce: item.DefensePierceAmount(actor.Cultivation), Modifiers: []cultivation.AttributeModifier{cultivation.EquipmentModifier(item.ID, cultivation.AttrAttack, item.Implement.AttackBonus)}})
			}
			if item.ID == "equipment."+prefix+"_robe" {
				actor.Equipment = append(actor.Equipment, cultivation.EquipmentCopy{InstanceID: "realm-armor", DefID: item.ID, Slot: cultivation.EquipmentSlotGarment, ApplicableStage: item.ApplicableStage, OperatingManaCost: item.OperatingManaCost(actor.Cultivation), StrainReduction: item.EquipmentStrainReduction(actor.Cultivation), Modifiers: []cultivation.AttributeModifier{{ID: item.ID, Kind: cultivation.SourceArmor, Attribute: cultivation.AttrDefense, Value: item.Armor.DefenseBonus}}})
			}
		}
	}
	actor.HasImplement = len(actor.Equipment) > 0
	modifiers := append([]cultivation.AttributeModifier{}, actor.Modifiers...)
	for _, item := range actor.Equipment {
		if item.Slot != cultivation.EquipmentSlotWeapon && item.OperatingManaCost > 0 {
			continue
		}
		modifiers = append(modifiers, item.Modifiers...)
	}
	actor.Attributes = cultivation.AggregateAttributes(cultivation.BaseAttributes(actor.Cultivation), modifiers)
	actor.Attributes.Health, actor.Attributes.Mana = actor.Attributes.HealthMax, actor.Attributes.ManaMax
}

func TestCombatBalanceSameStageBuildChangesResolvedDamage(t *testing.T) {
	strike, _ := cultivation.SkillByID("strike")
	fire, _ := cultivation.SkillByID("fire_bolt")
	for _, key := range cultivation.StageKeys() {
		if key != "qi_6" && !strings.HasSuffix(key, "_early") {
			continue
		}
		t.Run(key, func(t *testing.T) {
			stage, _ := cultivation.ParseStageKey(key)
			bare := cultivation.BaseAttributes(stage)
			baseHit := cultivation.ResolveAttack(bare, bare, stage.Realm, stage.Realm, strike, cultivation.NeutralDamageContext(), cultivation.Shield{}, cultivation.CombatRolls{Hit: 0, Damage: 0.5})
			for _, mode := range []string{"method_vs_bare", "equipment_vs_bare", "build_vs_bare"} {
				actor := cultivation.Combatant{Cultivation: stage, Attributes: bare}
				applyBalanceCultivationBuild(&actor, mode)
				hit := cultivation.ResolveAttack(actor.Attributes, bare, stage.Realm, stage.Realm, strike, cultivation.NeutralDamageContext(), cultivation.Shield{}, cultivation.CombatRolls{Hit: 0, Damage: 0.5})
				if hit.Damage <= baseHit.Damage {
					t.Fatalf("%s produced no real same-stage difference: %d <= %d", mode, hit.Damage, baseHit.Damage)
				}
			}
			skillHit := cultivation.ResolveAttack(bare, bare, stage.Realm, stage.Realm, fire, cultivation.NeutralDamageContext(), cultivation.Shield{}, cultivation.CombatRolls{Hit: 0, Damage: 0.5})
			if skillHit.Damage <= baseHit.Damage || fire.ManaCost(stage) <= 0 {
				t.Fatal("learned technique must change actual damage for a real cost")
			}
		})
	}
}

func stageAtLeastForBalance(stage cultivation.Cultivation, floor string) bool {
	if floor == "" {
		return true
	}
	index := map[string]int{}
	for i, key := range cultivation.StageKeys() {
		index[key] = i
	}
	return index[cultivation.StageKey(stage)] >= index[floor]
}

func balanceSourceHashes(t *testing.T) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	for _, name := range []string{"combat_balance_report_test.go", "adventure_starting_balance_test.go", "combat_equipment_balance_report_test.go", "adventure_npc_balance_report_test.go", "adventure_npc_tactics_regression_test.go", "adventure_control_tradeoff_report_test.go", "adventure_control_isolation_report_test.go", "adventure_rules.go", "adventure_combat.go", "adventure_mental.go", "adventure_weapon_requirement.go", "adventure_creation.go", "adventure_disengagement.go", "adventure_person_fleet.go", "adventure_overclock.go", "adventure_enemy_decision.go", "adventure_catalog_validation.go", "adventure_conditions.go", "adventure_enemy_turn.go", "adventure_enemy_tactics.go", "adventure_npc_growth.go", "adventure_settlement.go", "adventure_progression.go", "adventure_generated_design.go", "cultivation_equipment.go", "story_combat.go", "../cultivation/combat.go", "../cultivation/combat_session.go", "../cultivation/combat_decision.go", "../cultivation/combat_declaration.go", "../cultivation/combat_cast_plan.go", "../cultivation/combat_equipment.go", "../cultivation/combat_strain.go", "../cultivation/stage.go", "../cultivation/stagekey.go", "../cultivation/modifier.go", "../cultivation/character.go", "../cultivation/item.go", "../cultivation/skill_registry.go", "../cultivation/skill_template.go", "../cultivation/technique.go", "../cultivation/technique_spirit_realm.go", "../cultivation/realm_presets.go", "../cultivation/skill.go", "../cultivation/item_catalog.go", "../cultivation/equipment_effects.go", "../cultivation/progress.go", "../cultivation/lifespan.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return hashes
}
