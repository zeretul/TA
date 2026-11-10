package cultivation

import "sort"

// DecideEnemyAction chooses what one enemy does this exchange.
//
// §14.2 requires each enemy to use only its own registered abilities and real
// resources, and permits it to retreat, beg or offer a truce under a template.
// The evaluation is deliberately not a fixed health line that always fires: a
// survival-templated enemy at or below 30% health evaluates the options, and a
// credible reason to keep fighting — a route it does not have, or an opponent it
// can still beat — leaves it fighting. That is what makes the design's "not
// automatic success" requirement hold.
//
// The technique it uses is chosen from the enemy's own learned set, and only a
// technique it can actually pay for is used. §14.2's other half is what the
// enemy must not do: read the player's hidden abilities or the director's truth
// and counter them precisely. Nothing here looks at the player's side at all
// except through the public attributes any combatant could see — the target's
// standing, health fraction, mobility, visible effects and committed preparations — so there is no hidden
// information for it to exploit.
//
// The returned action is the enemy's intent. The caller resolves it through the
// same ResolveExchange every other combatant goes through, so an enemy cannot
// obtain an outcome the rules would refuse the player.
func DecideEnemyAction(session CombatSession, enemyID string, fallback SkillDef) CombatAction {
	enemy := session.CombatantByID(enemyID)
	if enemy == nil || !enemy.Active() {
		return CombatAction{Kind: ActionTechnique, Skill: fallback}
	}
	target := firstOpponent(&session, enemy)
	_, preparing := session.Preparations[enemyID]
	action := enemyTechnique(session, *enemy, fallback)
	if preparing {
		action = CombatAction{Kind: ActionContinuePreparation}
	}
	if target != nil && action.TargetID == "" && !skillShields(action.Skill) {
		action.TargetID = target.ID
	}

	if enemy.Retreat == RetreatNone || target == nil {
		return action
	}

	// An offer already on the table is not repeated. §14.3 makes the offer a
	// real action that occupies the offering side's exchange, so re-offering
	// every exchange would let a losing enemy stall the fight indefinitely
	// without ever spending anything. It waits for the answer instead, and the
	// caller is the one who decides whether the answer ever comes.
	if enemy.OfferText != "" {
		return CombatAction{Kind: ActionRest}
	}

	// §14.2 gives the survival template two triggers: 生命不高于30%, or 没有可用主要
	// 术法且无法继续完成目的 — no usable primary technique and no way to keep
	// pursuing the objective. The second is a real check against the enemy's own
	// pools, so an exhausted enemy evaluates retreat even at full health.
	cornered := enemy.HealthFraction() <= 0.30 || (!preparing && !enemyCanPerform(*enemy, action.Skill))

	if enemy.Retreat == RetreatCautious {
		// The cautious template evaluates earlier, at 50%, but only once a real
		// disadvantage has been observed.
		disadvantaged := enemy.HealthFraction() <= 0.50 && enemy.HealthFraction() < target.HealthFraction()
		if !cornered && !disadvantaged {
			return action
		}
	} else if !cornered {
		return action
	}
	if !enemy.RouteExists {
		// No route out: it may still yield, but it cannot run.
		return CombatAction{Kind: ActionSurrender, OfferText: "求饶"}
	}
	return CombatAction{Kind: ActionEscape}
}

// enemyTechnique chooses a useful paid action from this actor's learned set.
// It reads visible opponents and active preparations, never their hidden skills,
// inventory or next declaration. Sorting stable IDs makes catalog order irrelevant.
func enemyTechnique(session CombatSession, enemy Combatant, fallback SkillDef) CombatAction {
	targets := []Combatant{}
	for _, target := range session.Combatants {
		if legalTechniqueTarget(&enemy, &target) {
			targets = append(targets, target)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	best := CombatAction{Kind: ActionTechnique, Skill: fallback}
	bestScore := -1.0
	if enemyCanPerform(enemy, fallback) {
		bestScore = fallback.Power * clampFloat(0.80+fallback.HitOffset, 0.15, 0.97) / float64(maxInt(1, fallback.ExecutionActions))
	}
	for _, skill := range enemy.Skills {
		if skill.ID == "" || !enemyCanPerform(enemy, skill) {
			continue
		}
		action := CombatAction{Kind: ActionTechnique, Skill: skill}
		score := 0.0
		if skillShields(skill) {
			fresh := NewShield(skill, enemy.Cultivation, enemy.Cultivation, enemy.ID+":"+skill.ID)
			gain := fresh.Amount - enemy.Shield.Amount
			if gain <= 0 || (enemy.HealthFraction() > 0.5 && enemy.Burn.RemainingTicks == 0 && !enemyFacesPreparation(session, enemy)) {
				continue
			}
			// A shield is a defensive action on oneself, never a gift to the
			// opponent. A visible committed preparation warrants a stronger ward.
			action.TargetID = enemy.ID
			score = float64(gain) / float64(maxInt(1, enemy.Attributes.Attack))
			if enemyFacesPreparation(session, enemy) {
				score *= 3
			}
		} else {
			if len(targets) == 0 {
				continue
			}
			action.TargetID = targets[0].ID
			reach := minInt(maxInt(1, skill.Targets), len(targets))
			for _, target := range targets[1:reach] {
				action.ExtraTargetIDs = append(action.ExtraTargetIDs, target.ID)
			}
			power := skill.Power
			if reach == 1 {
				target := targets[0]
				if skill.VariantBase == VariantBurn && target.HealthFraction() > 0.5 && target.Shield.Amount == 0 && target.Burn.RemainingTicks == 0 {
					action.Variant = VariantBurn
					power = BurnVariantPower * 1.30
				} else if skill.VariantBase == VariantSlow && target.Attributes.Agility > enemy.Attributes.Agility && target.Shield.Amount == 0 && target.Slow.RemainingTargetActions == 0 && target.HealthFraction() > 0.5 {
					action.Variant = VariantSlow
					// Observed mobility makes exchanging some damage for delay useful.
					power = skill.Power
				}
			}
			score = power * float64(reach) * clampFloat(0.80+skill.HitOffset, 0.15, 0.97) / float64(maxInt(1, skill.ExecutionActions))
		}
		// Own finite costs break near-equal opportunities without turning the
		// cheapest attack into the only tactic. No opponent resources are read.
		score -= float64(skill.ReducedManaCost(enemy.Cultivation, enemy.ManaReduction)) / float64(maxInt(1, enemy.Attributes.ManaMax)) * 0.15
		if score > bestScore || (score == bestScore && skill.ID < best.Skill.ID) {
			best, bestScore = action, score
		}
	}
	if bestScore < 0 {
		// Even the ordinary swing needs the equipped implement's real cost.
		// Regulate instead of repeatedly declaring a known impossible attack.
		return CombatAction{Kind: ActionRest}
	}
	return best
}

func enemyFacesPreparation(session CombatSession, enemy Combatant) bool {
	for actorID, preparation := range session.Preparations {
		actor := session.CombatantByID(actorID)
		if actor != nil && actor.Active() && actor.Side != enemy.Side && preparation.TargetID == enemy.ID {
			return true
		}
	}
	return false
}

// enemyCanPerform answers whether the enemy could really carry out this
// technique right now, priced the way ResolveExchange will price it.
//
// The check has to include the enemy's own circulating-method reduction and the
// implement's operating cost, because settlement charges both. Asking the bare
// CanPerform with a zero reduction and no operating cost let the enemy pick a
// technique whose cast the very next step refuses — the exchange would record a
// refusal the enemy never intended, and its retreat trigger would read the
// enemy as still capable when it is not.
func enemyCanPerform(enemy Combatant, skill SkillDef) bool {
	return NextExchangeCastPlanFor(enemy, skill, 0).Check.Allowed
}
