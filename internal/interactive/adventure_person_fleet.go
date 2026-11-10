package interactive

import "fmt"

// Personal deep-space combat is a learned capability with ongoing exertion,
// not a rank-based permission to defeat every vessel. Values are frozen in the
// skill catalog; callers may select a known skill but never submit these values.
type AdventureSpaceCombatProfile struct {
	HullPower      int `json:"hull_power"`
	Protection     int `json:"protection"`
	Mobility       int `json:"mobility"`
	EnergyPerRound int `json:"energy_per_round"`
}

// One ship or a bounded aggregate formation. VesselCount is descriptive: this
// solver does not invent individually targetable ships, reinforcements or loot.
type AdventureShipCombatProfile struct {
	HullThreshold int `json:"hull_threshold"`
	Tracking      int `json:"tracking"`
	Interdiction  int `json:"interdiction"`
	VesselCount   int `json:"vessel_count"`
}

// Scale retains the opponent's scale. Missing PlayerScale is the previous
// release's symmetric battle, including already-active saved encounters.
func AdventurePlayerBattleScale(b *AdventureBattle) string {
	if b == nil {
		return "person"
	}
	if b.PlayerScale != "" {
		return b.PlayerScale
	}
	return b.Scale
}

func adventurePersonalShipBattle(b *AdventureBattle) bool {
	return b != nil && b.Scale == "ship" && AdventurePlayerBattleScale(b) == "person"
}

// Returns the capability selected for this encounter with actual mastery. It
// does not inspect an unselected skill or guess a capability from names/rank.
func AdventurePersonalSpaceCombat(r AdventureRules, a AdventureActor) (AdventureSpaceCombatProfile, bool) {
	b := a.Battle
	if r.Family != "space" || !adventurePersonalShipBattle(b) || b.PersonalCombatSkillID == "" {
		return AdventureSpaceCombatProfile{}, false
	}
	for _, s := range r.Skills {
		level := a.Skills[s.ID]
		if s.ID == b.PersonalCombatSkillID && s.SpaceCombat != nil && s.Scale == "person" && level > 0 && a.Rank >= s.RequiredRank {
			p := *s.SpaceCombat
			p.HullPower *= level
			p.Protection *= level
			return p, true
		}
	}
	return AdventureSpaceCombatProfile{}, false
}

func validateAdventureSpaceCombatSkill(r AdventureRules, s AdventureSkill) error {
	if s.Mobility < 0 || s.Mobility > 100 {
		return fmt.Errorf("invalid skill mobility %s", s.ID)
	}
	p := s.SpaceCombat
	if p == nil {
		return nil
	}
	if r.Family != "space" || s.Scale != "person" || p.HullPower < 0 || p.HullPower > 1000 || p.Protection < 0 || p.Protection > 1000 || p.Mobility < 0 || p.Mobility > 100 || p.EnergyPerRound < 1 || p.EnergyPerRound > 100 {
		return fmt.Errorf("invalid personal space combat capability %s", s.ID)
	}
	return nil
}

func validateAdventureShipCombat(p *AdventureShipCombatProfile) error {
	if p == nil {
		return nil
	}
	if p.HullThreshold < 0 || p.HullThreshold > 10000 || p.Tracking < 1 || p.Tracking > 100 || p.Interdiction < 0 || p.Interdiction > 100 || p.VesselCount < 1 || p.VesselCount > 1000 {
		return fmt.Errorf("invalid ship combat profile")
	}
	return nil
}

func validateAdventureBattleScales(r AdventureRules, a AdventureActor) error {
	b := a.Battle
	if b == nil {
		return nil
	}
	for _, opponent := range r.Opponents {
		if opponent.ID == b.OpponentID && opponent.Mindless != b.Mindless {
			return fmt.Errorf("battle mental target does not match its frozen source")
		}
	}
	playerScale := AdventurePlayerBattleScale(b)
	if playerScale == b.Scale {
		if b.PersonalCombatSkillID != "" {
			return fmt.Errorf("personal space capability requires a mixed-scale battle")
		}
		if b.ShipCombat == nil {
			return nil
		}
		if err := validateAdventureShipCombat(b.ShipCombat); err != nil {
			return err
		}
		for _, op := range r.Opponents {
			if op.ID == b.OpponentID && op.Scale == "ship" && op.ShipCombat != nil && *op.ShipCombat == *b.ShipCombat {
				return nil
			}
		}
		return fmt.Errorf("ship profile does not match its frozen source")
	}
	if !adventurePersonalShipBattle(b) || b.AssetID != "" || b.ShipCombat == nil {
		return fmt.Errorf("unsupported mixed-scale battle")
	}
	if _, ok := AdventurePersonalSpaceCombat(r, a); !ok {
		return fmt.Errorf("personal space combat capability is not learned or qualified")
	}
	if err := validateAdventureShipCombat(b.ShipCombat); err != nil {
		return err
	}
	for _, op := range r.Opponents {
		if op.ID == b.OpponentID && op.Scale == "ship" && op.ShipCombat != nil && *op.ShipCombat == *b.ShipCombat {
			return nil
		}
	}
	return fmt.Errorf("mixed-scale opponent has no matching frozen ship profile")
}

// An ordinary personal consumable never inherits a superhuman's structural
// force. Biological controls also cannot become whole-fleet shutdowns.
func adventurePersonalShipEffectsAllowed(b *AdventureBattle, effects []AdventureEffect) bool {
	if !adventurePersonalShipBattle(b) {
		return true
	}
	for _, effect := range effects {
		switch effect.Kind {
		case "attack", "guard", "pilot", "heal", "energy", "antidote", "mental_guard", "shield_boost", "recoil":
		default:
			return false
		}
	}
	return true
}

// Tracking and maneuver affect exposure, not a guaranteed hit/miss based on
// level. Full protection may stop all damage; no one-point erosion bypasses it.
func adventureShipIncomingForce(force int, ship *AdventureShipCombatProfile, person AdventureSpaceCombatProfile) int {
	if force <= 0 || ship == nil {
		return force
	}
	exposure := max(15, min(100, 50+ship.Tracking-person.Mobility))
	return force * exposure / 100
}
