package cultivation

// Affordable declarations distinguish an invalid request from a real turn
// interrupted by earlier actors. A later wound can lower burden capacity, and
// a defensive item can spend mana before the declared cast gets its turn.
// Neither event may undo the already-settled opposition action.
func affordableTechniqueDeclarations(session CombatSession, actions map[string]CombatAction, refused map[string]string) map[string]CastCheck {
	affordable := map[string]CastCheck{}
	for id, action := range actions {
		actor := session.CombatantByID(id)
		if actor == nil || !actor.Active() || action.Kind != ActionTechnique {
			continue
		}
		check := CastPlanFor(*actor, action.Skill, action.ManaReduction).Check
		affordable[id] = check
		if refused[id] == "" && !check.Allowed {
			refused[id] = check.ReasonKey
		}
		if action.Variant != VariantNone {
			if _, ok := VariantPowerFor(action.Skill, action.Variant); !ok {
				refused[id] = "reason.variant_unsupported"
			}
		}
	}
	return affordable
}

func interruptedCastReason(reason string) bool {
	switch reason {
	case ReasonInsufficientMana, ReasonBurdenCapacity, ReasonMeridianUnrecovered:
		return true
	}
	return false
}

// Missing inventory and impossible equipment selections are input errors even
// when an earlier enemy would remove their owner. Pool-dependent uses (healing
// at full health or replacing a still-active shield) are deliberately not frozen:
// the preceding action can make that use productive.
func freezeOtherInputRefusals(session CombatSession, actions map[string]CombatAction, refused map[string]string) {
	for id, action := range actions {
		actor := session.CombatantByID(id)
		if actor == nil || !actor.Active() || refused[id] != "" {
			continue
		}
		switch action.Kind {
		case ActionTechnique, ActionRest, ActionEscape, ActionSurrender, ActionAcceptSurrender:
			continue
		case ActionContinuePreparation, ActionCancelPreparation:
			if _, ok := session.Preparations[id]; !ok {
				refused[id] = "reason.no_preparation"
			}
			continue
		case ActionUseItem, ActionSwapEquipment:
		default:
			refused[id] = "reason.unknown_action"
			continue
		}
		probe := session.clone()
		actor = probe.CombatantByID(id)
		var step ExchangeStep
		if action.Kind == ActionUseItem {
			step = resolveUseItem(actor, action)
		} else {
			step = resolveSwapEquipment(actor, action)
		}
		switch step.ReasonKey {
		case "reason.item_not_named", "reason.item_not_carried", "reason.item_exhausted", "reason.item_not_usable", "reason.item_stage_too_low", "reason.spare_not_named", "reason.spare_not_carried", "reason.spare_not_equipment", "reason.equipment_stage_too_low":
			refused[id] = step.ReasonKey
		}
	}
}
