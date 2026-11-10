package cultivation

// TalismanDef describes a single-use ward. Its fixed registered stage prevents
// a low-grade consumable from scaling with the user's later cultivation.
// It reuses the combat shield's cap, strongest-effect rule and expiry.
type TalismanDef struct {
	RegisteredStage   Cultivation `json:"registered_stage"`
	ShieldFraction    float64     `json:"shield_fraction"`
	DurationExchanges int         `json:"duration_exchanges"`
}

func (d TalismanDef) Validate() error {
	if _, ok := ParseStageKey(StageKey(d.RegisteredStage)); !ok {
		return ErrItemUnknownStage
	}
	if !(d.ShieldFraction > 0 && d.ShieldFraction <= 0.35) || d.DurationExchanges < 1 || d.DurationExchanges > 3 {
		return ErrItemOverBudget
	}
	return nil
}

// ShieldFor returns the ward the carried copy can actually provide to its user.
func (d TalismanDef) ShieldFor(user Cultivation, sourceID string) Shield {
	return NewShield(SkillDef{ShieldBaseHealthFraction: d.ShieldFraction, DurationExchanges: d.DurationExchanges}, d.RegisteredStage, user, sourceID)
}

// TalismanUseReason is shared by the combat window and the exchange executor.
// An ineffective or unqualified use must keep the copy in inventory.
func TalismanUseReason(actor Combatant, d TalismanDef, defID string) string {
	if d.Validate() != nil {
		return "reason.item_not_usable"
	}
	if !stageAtLeast(actor.Cultivation, StageKey(d.RegisteredStage)) {
		return "reason.item_stage_too_low"
	}
	fresh := d.ShieldFor(actor.Cultivation, "talisman:"+defID)
	if actor.Shield.Amount >= fresh.Amount && actor.Shield.RemainingExchanges >= fresh.RemainingExchanges {
		return "reason.talisman_shield_active"
	}
	next := StackShield(actor.Shield, fresh, actor.Cultivation)
	if next == actor.Shield {
		return "reason.talisman_shield_active"
	}
	return ""
}
