package interactive

import (
	"fmt"
	"strings"
)

// Fog is a finite authored scene condition, not a weather/poison simulator.
// Treatment lasts for this visit; leaving and re-entering is a new exposure.
type AdventureFogArea struct {
	ID              string   `json:"id"`
	Location        string   `json:"location"`
	LocationAliases []string `json:"location_aliases,omitempty"`
	AntidoteID      string   `json:"antidote_id"`
}

func validateAdventureFogAreas(r AdventureRules, items map[string]bool) error {
	if len(r.FogAreas) > 16 {
		return fmt.Errorf("too many fog areas")
	}
	seen := map[string]bool{}
	for _, area := range r.FogAreas {
		if r.Family != "wuxia" || area.ID == "" || seen[area.ID] || area.Location == "" || !items[area.AntidoteID] {
			return fmt.Errorf("invalid fog area %s", area.ID)
		}
		seen[area.ID] = true
		valid := false
		for _, item := range r.Items {
			valid = valid || item.ID == area.AntidoteID && item.Effect == "antidote"
		}
		if !valid {
			return fmt.Errorf("fog cure is not an antidote %s", area.ID)
		}
	}
	return nil
}

func refreshAdventureFogExposure(r AdventureRules, a *AdventureActor, state map[string]any) bool {
	location := strings.TrimSpace(StorySceneFromState(state, "").Location)
	return setAdventureFogLocation(r, a, location)
}

func setAdventureFogLocation(r AdventureRules, a *AdventureActor, location string) bool {
	id := ""
	for _, area := range r.FogAreas {
		if matchesAdventureLocation(location, area.Location, area.LocationAliases, r.world) {
			id = area.ID
		}
	}
	if a.FogAreaID == id {
		return false
	}
	a.FogAreaID = id
	a.FogTreated = false
	return true
}

func cureAdventureFog(r AdventureRules, a *AdventureActor, itemID string) {
	for _, area := range r.FogAreas {
		if area.ID == a.FogAreaID && area.AntidoteID == AdventureItemTemplateID(*a, itemID) {
			a.FogTreated = true
		}
	}
}

func adventureRushUnavailable(a AdventureActor) bool {
	return a.RushBlocked > 0 || a.FogAreaID != "" && !a.FogTreated
}

func adventureEffectsSeal(effects []AdventureEffect) bool {
	for _, e := range effects {
		if e.Kind == "seal" {
			return true
		}
	}
	return false
}

func adventureEffectsOffensive(effects []AdventureEffect) bool {
	for _, e := range effects {
		switch e.Kind {
		case "attack", "mental_attack", "control", "poison", "seal", "reflect", "drain", "electronic_disrupt":
			return true
		}
	}
	return false
}

// With softened limbs, only inward recovery is possible; a mixed guard or a
// tool technique still requires physical action even if it also restores energy.
func adventureSoftRecovery(s AdventureSkill, effects []AdventureEffect) bool {
	if s.RequiredTool != "" || s.RequiredWeapon != "" || s.RequiredWeaponClass != "" || s.WeaponStyle || s.Rush || len(effects) == 0 {
		return false
	}
	for _, e := range effects {
		switch e.Kind {
		case "heal", "energy", "antidote":
		default:
			return false
		}
	}
	return true
}

func finishAdventureEncounter(a *AdventureActor) {
	if a.Battle == nil || a.Battle.Status == "active" {
		return
	}
	a.Control = 0
	a.ControlRecovery = 0
	a.Battle.EnemyControl = 0
	a.Battle.EnemyControlRecovery = 0
	a.SoftLimbs = false
	a.RushBlocked = 0
	a.LastSealTarget = ""
	a.Battle.EnemySoftLimbs = false
	a.Battle.EnemyRushBlocked = 0
	a.Battle.EnemyLastSeal = false
}

// Full control can deny one response, then leaves the target one exchange to
// act before it can be applied again. The paid damage component still resolves.
// Both counters live in the canonical actor/battle state for retry and restore.
func applyAdventureHardControl(control, recovery *int) bool {
	if *recovery > 0 {
		return false
	}
	*control = 1
	*recovery = 2
	return true
}
