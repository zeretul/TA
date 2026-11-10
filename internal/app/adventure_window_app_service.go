package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// AdventureWindow projects the frozen catalog and the player's canonical state.
// Opponent internals stay server-side, as in the cultivation battle window.
type AdventureWindow struct {
	Progression    *AdventureProgressionView    `json:"progression,omitempty"`
	CraftMethods   []AdventureWindowCraftMethod `json:"craft_methods,omitempty"`
	ClockSeconds   int64                        `json:"clock_seconds"`
	Head           string                       `json:"head"`
	Family         string                       `json:"family"`
	Actor          AdventureWindowActor         `json:"actor"`
	Skills         []AdventureWindowSkill       `json:"skills"`
	Items          []AdventureWindowItem        `json:"items"`
	EnemyCondition string                       `json:"enemy_condition,omitempty"`
}

// Names referenced by a learned method are readable even before acquisition.
// This narrow dictionary does not grant stock or expose the rest of the catalog.
type AdventureWindowCraftMethod struct {
	interactive.AdventureRecipe
	ItemNames map[string]string `json:"item_names"`
	SkillName string            `json:"skill_name"`
}

type AdventureWindowSkill struct {
	MobilityUsable *bool `json:"mobility_usable,omitempty"`
	interactive.AdventureSkill
	Learned   bool   `json:"learned"`
	Trainable bool   `json:"trainable"`
	Usable    *bool  `json:"usable,omitempty"`
	ReasonKey string `json:"reason_key,omitempty"`
}
type AdventureWindowItem struct {
	Equippable *bool `json:"equippable,omitempty"`
	Deployable *bool `json:"deployable,omitempty"`
	interactive.AdventureItem
	Usable    bool   `json:"usable"`
	ReasonKey string `json:"reason_key,omitempty"`
}

// The embedded actor contains only the player's own state. Its battle is
// shadowed by this explicit view so new enemy fields never leak by default.
type AdventureWindowActor struct {
	interactive.AdventureActor
	Battle *AdventureWindowBattle `json:"battle,omitempty"`
}

type AdventureWindowBattle struct {
	PlayerScale               string `json:"player_scale,omitempty"`
	PersonalCombatSkillID     string `json:"personal_combat_skill_id,omitempty"`
	SpaceCombatEnergyPerRound int    `json:"space_combat_energy_per_round,omitempty"`
	PlayerRetreatProgress     int    `json:"player_retreat_progress,omitempty"`
	EnemyRetreating           bool   `json:"enemy_retreating,omitempty"`
	EnemyCeasefireOffer       bool   `json:"enemy_ceasefire_offer,omitempty"`
	PlayerCeasefireOffer      bool   `json:"player_ceasefire_offer,omitempty"`
	LastEnemyAction           string `json:"last_enemy_action,omitempty"`

	LastEnemySkillName    string `json:"last_enemy_skill_name,omitempty"`
	CanSurrender          bool   `json:"can_surrender,omitempty"`
	CanDisruptElectronics bool   `json:"can_disrupt_electronics,omitempty"`
	ID                    string `json:"id"`
	Scale                 string `json:"scale"`
	OpponentID            string `json:"opponent_id"`
	OpponentName          string `json:"opponent_name"`
	Round                 int    `json:"round"`
	Status                string `json:"status"`
	AssetID               string `json:"asset_id,omitempty"`
}

func (a *App) AdventureWindow(storyID, branchID, locale string) (AdventureWindow, error) {
	if a == nil {
		return AdventureWindow{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	cfg := a.cfg
	projectID := ""
	if cfg != nil {
		projectID = cfg.ProjectID
	}
	a.mu.RUnlock()
	if store == nil {
		return AdventureWindow{}, ErrNoWorkspace
	}
	state, err := store.AdventureWindow(storyID, branchID)
	if err != nil {
		return AdventureWindow{}, err
	}
	out := AdventureWindow{Progression: adventureProgressionView(state.Rules, state.Actor, locale), Head: state.Head, ClockSeconds: state.ClockSeconds, Family: state.Rules.Family, Actor: AdventureWindowActor{AdventureActor: state.Actor}}
	knownItems := make(map[string]bool)
	for id := range state.Actor.Inventory {
		knownItems[id] = true
	}
	for _, id := range state.Actor.Equipment {
		knownItems[id] = true
	}
	for id := range state.Actor.Assets {
		knownItems[id] = true
	}
	refs, artErr := store.DefinitionArtReferences(storyID, branchID)
	if artErr != nil {
		slog.Warn("[definition-art] read optional adventure art failed", "story_id", storyID, "error", artErr)
	}
	for i := range state.Rules.Skills {
		if art, ok := refs[state.Rules.Skills[i].ID+":1"]; ok {
			state.Rules.Skills[i].Art = &art
		}
	}
	for i := range state.Rules.Items {
		if art, ok := refs[state.Rules.Items[i].ID+":1"]; ok {
			state.Rules.Items[i].Art = &art
		}
	}
	for _, recipe := range state.Rules.Recipes {
		// A frozen recipe has no separate unlock ledger: its authored skill
		// mastery is the same qualification the craft command checks. Show the
		// method, not its ingredients as possessions; missing supplies stay costs.
		if strings.HasPrefix(recipe.ID, "gen.craft.") || state.Actor.Skills[recipe.SkillID] >= max(1, recipe.SkillRank) {
			if art, ok := refs[recipe.ID+":1"]; ok {
				recipe.Art = &art
			}
			recipe.Name = adventureDefinitionName(recipe.ID, recipe.Name, recipe.DefinitionDetails, locale)
			recipe.DefinitionDetails = projectDefinitionArt(recipe.DefinitionDetails.ForLocale(locale), projectID)
			method := AdventureWindowCraftMethod{AdventureRecipe: recipe, ItemNames: map[string]string{}}
			for _, item := range state.Rules.Items {
				_, material := recipe.Materials[item.ID]
				if material || item.ID == recipe.OutputID || item.ID == recipe.RequiredAsset || item.ID == recipe.RequiredTool {
					method.ItemNames[item.ID] = adventureDefinitionName(item.ID, item.Name, item.DefinitionDetails, locale)
				}
			}
			for _, skill := range state.Rules.Skills {
				if skill.ID == recipe.SkillID {
					method.SkillName = adventureDefinitionName(skill.ID, skill.Name, skill.DefinitionDetails, locale)
					break
				}
			}
			out.CraftMethods = append(out.CraftMethods, method)
		}
	}
	out.Skills = []AdventureWindowSkill{}
	for _, skill := range state.Rules.Skills {
		if _, known := state.Actor.Skills[skill.ID]; known {
			skill.Name = adventureDefinitionName(skill.ID, skill.Name, skill.DefinitionDetails, locale)
			skill.DefinitionDetails = projectDefinitionArt(skill.DefinitionDetails.ForLocale(locale), projectID)
			view := AdventureWindowSkill{AdventureSkill: skill, Learned: state.Actor.Skills[skill.ID] > 0}
			_, _, trainErr := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "train", SkillID: skill.ID, Hours: 1}, "entry-details", state.ClockSeconds)
			view.Trainable = trainErr == nil
			if state.Actor.Battle != nil && state.Actor.Battle.Status == "active" {
				_, _, useErr := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "battle_round", Choice: "skill", SkillID: skill.ID}, "entry-details", state.ClockSeconds)
				usable := useErr == nil && (state.Actor.Control == 0 || state.Actor.SoftLimbs)
				view.Usable = &usable
				if skill.Mobility > 0 {
					_, _, movementErr := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "battle_round", Choice: "escape", SkillID: skill.ID}, "entry-details", state.ClockSeconds)
					movementUsable := movementErr == nil && state.Actor.Control == 0 && !state.Actor.SoftLimbs
					view.MobilityUsable = &movementUsable
				}
				if !usable {
					view.ReasonKey = "api.adventure.rejected"
				}
			}
			out.Skills = append(out.Skills, view)
		}
	}
	out.Items = []AdventureWindowItem{}
	for _, item := range state.Rules.Items {
		if knownItems[item.ID] {
			item.Name = adventureDefinitionName(item.ID, item.Name, item.DefinitionDetails, locale)
			item.DefinitionDetails = projectDefinitionArt(item.DefinitionDetails.ForLocale(locale), projectID)
			view := AdventureWindowItem{AdventureItem: item}
			if item.Kind == "weapon" || item.Kind == "armor" || item.Kind == "tool" {
				_, _, err := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "equip", ItemID: item.ID}, "entry-details", state.ClockSeconds)
				eligible := err == nil
				view.Equippable = &eligible
			}
			if item.Kind == "machine" || item.Kind == "ship" {
				_, _, err := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "deploy", ItemID: item.ID}, "entry-details", state.ClockSeconds)
				eligible := err == nil
				view.Deployable = &eligible
			}
			action := interactive.AdventureAction{Action: "use", ItemID: item.ID}
			if state.Actor.Battle != nil && state.Actor.Battle.Status == "active" {
				action.Action = "battle_round"
				action.Choice = "item"
			}
			_, _, useErr := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, action, "entry-details", state.ClockSeconds)
			view.Usable = useErr == nil && (state.Actor.Control == 0 || state.Actor.SoftLimbs)
			if !view.Usable {
				view.ReasonKey = "api.adventure.rejected"
			}
			out.Items = append(out.Items, view)
		}
	}
	if battle := state.Actor.Battle; battle != nil {
		out.EnemyCondition = "steady"
		if battle.EnemySoftLimbs {
			out.EnemyCondition = "softened"
		} else if battle.Health <= 0 {
			out.EnemyCondition = "fallen"
		} else if battle.Health*4 <= battle.MaxHealth {
			out.EnemyCondition = "critical"
		} else if battle.Health*2 <= battle.MaxHealth {
			out.EnemyCondition = "wounded"
		}
		out.Actor.Battle = &AdventureWindowBattle{ID: battle.ID, Scale: battle.Scale, OpponentID: battle.OpponentID, OpponentName: battle.OpponentName, Round: battle.Round, Status: battle.Status, AssetID: battle.AssetID}
		out.Actor.Battle.PlayerScale = interactive.AdventurePlayerBattleScale(battle)
		out.Actor.Battle.PersonalCombatSkillID = battle.PersonalCombatSkillID
		if profile, ok := interactive.AdventurePersonalSpaceCombat(state.Rules, state.Actor); ok {
			out.Actor.Battle.SpaceCombatEnergyPerRound = profile.EnergyPerRound
		}
		out.Actor.Battle.PlayerRetreatProgress = battle.PlayerRetreatProgress
		out.Actor.Battle.EnemyRetreating = battle.EnemyRetreatProgress > 0 || battle.EnemyLastAction == "escape"
		out.Actor.Battle.EnemyCeasefireOffer = battle.EnemyCeasefireOffer
		out.Actor.Battle.PlayerCeasefireOffer = battle.PlayerCeasefireOffer
		out.Actor.Battle.LastEnemyAction = battle.EnemyLastAction
		if battle.Round > 0 && battle.EnemyLastSkillID != "" {
			for _, skill := range state.Rules.Skills {
				if skill.ID == battle.EnemyLastSkillID {
					out.Actor.Battle.LastEnemySkillName = adventureDefinitionName(skill.ID, skill.Name, skill.DefinitionDetails, locale)
					break
				}
			}
		}
		// Ask the same deterministic rule as submission; UI visibility must not
		// invent an escape from lethal combat or leak the opponent's internals.
		_, _, surrenderErr := interactive.ResolveAdventureActionAt(state.Rules, state.Actor, interactive.AdventureAction{Action: "battle_round", Choice: "surrender"}, "entry-details", state.ClockSeconds)
		out.Actor.Battle.CanSurrender = surrenderErr == nil
		for _, skill := range out.Skills {
			if state.Actor.Skills[skill.ID] > 0 && interactive.AdventureElectronicTargetInRange(skill.AdventureSkill, battle) {
				out.Actor.Battle.CanDisruptElectronics = true
			}
		}

	}
	// Translate only a view copy, and only from the exact pack revision bound
	// by this story. A newer author's catalog cannot rename an old encounter.
	if cfg != nil && strings.HasPrefix(locale, "en") {
		if context, err := store.StoryContext(storyID, branchID); err == nil && context.Meta.PackBinding != nil {
			binding := context.Meta.PackBinding
			if pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID); err == nil && pack.ContentHash() == binding.ContentHash {
				names := pack.Localizations["en-US"].Adventure
				if out.Progression != nil {
					var foundations []string
					for _, id := range out.Progression.FoundationIDs {
						if name := names[id]; name != "" {
							foundations = append(foundations, name)
						}
					}
					out.Progression.FoundationName = strings.Join(foundations, " / ")
				}
				for i := range out.CraftMethods {
					method := &out.CraftMethods[i]
					for id := range method.ItemNames {
						if name := names[id]; name != "" {
							method.ItemNames[id] = name
						}
					}
					if name := names[method.SkillID]; name != "" {
						method.SkillName = name
					}
				}
				for i := range out.Skills {
					if name := names[out.Skills[i].ID]; name != "" {
						out.Skills[i].Name = name
					}
				}
				for i := range out.Items {
					if name := names[out.Items[i].ID]; name != "" {
						out.Items[i].Name = name
					}
				}
				if battle := out.Actor.Battle; battle != nil {
					if name := names[state.Actor.Battle.EnemyLastSkillID]; name != "" && battle.LastEnemySkillName != "" {
						battle.LastEnemySkillName = name
					}
					if name := names[battle.OpponentID]; name != "" {
						battle.OpponentName = name
					}
				}
			}
		}
	}
	return out, nil
}

// PlayAdventureRound admits only an existing battle's next intent. Catalog
// acquisition and starting encounters remain main-story actions.
func (a *App) PlayAdventureRound(storyID, branchID, commandID, head string, input interactive.AdventureAction) (string, error) {
	return a.PlayAdventureRoundContext(context.Background(), storyID, branchID, commandID, head, input)
}

// PlayAdventureRoundContext keeps model cancellation attached to the incoming
// request. Decisions are prepared before, and revalidated during, the commit.
func (a *App) PlayAdventureRoundContext(ctx context.Context, storyID, branchID, commandID, head string, input interactive.AdventureAction) (string, error) {
	if a == nil {
		return "", ErrNoWorkspace
	}
	if input.Action != "battle_round" || commandID == "" || head == "" {
		return "", fmt.Errorf("battle round requires command identity and expected head")
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return "", ErrNoWorkspace
	}
	result, err := a.commitAdventureWithEnemyDecision(ctx, store, storyID, branchID, commandID, head, input)
	if err != nil {
		slog.Warn("Adventure round rejected", "story_id", storyID, "branch_id", branchID, "error", err)
		return "", err
	}
	slog.Info("Adventure round committed", "story_id", storyID, "branch_id", branchID, "event_id", result.EventID)
	return result.EventID, nil
}

// Frozen authored labels remain available if the installed pack later changes.
// Story-local generated names belong to the player and are never translated
// into their template's title.
func adventureDefinitionName(id, name string, details cultivation.DefinitionDetails, locale string) string {
	if !strings.HasPrefix(id, "gen.") && strings.HasPrefix(locale, "en") && details.Localizations != nil && strings.TrimSpace(details.Localizations.English.Name) != "" {
		return details.Localizations.English.Name
	}
	return name
}
