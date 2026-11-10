package interactive

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	interactivestate "ta/internal/interactive/state"
)

// AdventureNPC freezes authored ability and a sourced plan. A declared ceiling
// preserves a fixed character's period-specific power instead of extrapolating
// every famous character to the end of their original work.
type AdventureNPC struct {
	CharacterSourceKey string           `json:"character_source_key"`
	Actor              AdventureActor   `json:"actor"`
	Plan               AdventureNPCPlan `json:"plan"`
}
type AdventureNPCPlan struct {
	Action          string `json:"action"`
	SkillID         string `json:"skill_id,omitempty"`
	TrainingPercent int    `json:"training_percent"`
	RankLimit       int    `json:"rank_limit"`
}

// The watermark and fractional work are part of the actor's canonical value,
// so existing StateDelta replay, rollback and branches restore them together.
type AdventureNPCProgress struct {
	SettledThrough int64  `json:"settled_through"`
	WorkUnits      int64  `json:"work_units"`
	LastInterval   string `json:"last_interval"`
	Status         string `json:"status"`
}

func validateAdventureNPCs(r AdventureRules) error {
	if len(r.NPCs) > 256 {
		return fmt.Errorf("NPC catalog exceeds capacity")
	}
	seen := map[string]bool{}
	uniqueOwners := map[string]string{}
	unique := map[string]bool{}
	for _, item := range r.Items {
		unique[item.ID] = item.Unique
		if item.Unique && r.StartingItems[item.ID] > 0 {
			uniqueOwners[item.ID] = DefaultActorID
		}
	}
	for _, npc := range r.NPCs {
		for id, quantity := range npc.Actor.Inventory {
			if !unique[id] || quantity < 1 {
				continue
			}
			if uniqueOwners[id] != "" {
				return fmt.Errorf("unique item has multiple initial owners: %s", id)
			}
			uniqueOwners[id] = npc.CharacterSourceKey
		}
		if npc.CharacterSourceKey == "" || seen[npc.CharacterSourceKey] || npc.Plan.RankLimit < npc.Actor.Rank || npc.Plan.RankLimit > AdventureMaxRank(r) || npc.Plan.TrainingPercent < 0 || npc.Plan.TrainingPercent > 100 {
			return fmt.Errorf("invalid NPC growth plan %s", npc.CharacterSourceKey)
		}
		seen[npc.CharacterSourceKey] = true
		if npc.Actor.Growth != nil || npc.Actor.Battle != nil {
			return fmt.Errorf("NPC initial state cannot contain runtime progress")
		}
		if err := ValidateAdventureActor(r, npc.Actor); err != nil {
			return fmt.Errorf("NPC %s: %w", npc.CharacterSourceKey, err)
		}
		switch npc.Plan.Action {
		case "none", "rest":
		case "train", "learn":
			found := false
			for _, skill := range r.Skills {
				if skill.ID == npc.Plan.SkillID {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown NPC growth skill")
			}
		default:
			return fmt.Errorf("unsupported NPC plan action")
		}
	}
	return nil
}

// Only already established actors are visited. A catalog entry is not a live
// universe simulation, and no hour before first appearance is awarded.
func adventureNPCGrowthEvents(p *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now, turnID, settledActorID string, before, after StoryClock) ([]any, string) {
	if p == nil || p.Characters == nil || system.AdventureRules == nil || after.Seconds <= before.Seconds {
		return nil, parentID
	}
	rules := *system.AdventureRules
	actors, _ := p.State[actorStateRoot].(map[string]any)
	profiles := []string{}
	player, _ := AdventureActorFromState(p.State, DefaultActorID)
	for _, profile := range p.Characters.Profiles {
		if _, blocked := recordOnlyCatalogPerson(p.PackCatalog, profile.SourceKey, profile.Name); blocked {
			continue
		}
		id := profile.ID
		if id != DefaultActorID && id != settledActorID && !actorIsArchived(p.State, id) {
			if player.Battle != nil && player.Battle.Status == "active" && player.Battle.EnemyActorID == id {
				continue
			}
			profiles = append(profiles, id)
		}
	}
	sort.Strings(profiles)
	var events []any
	head := parentID
	for _, actorID := range profiles {
		profile, ok := p.Characters.Profile(actorID)
		if !ok || profile.SourceKey == "" {
			continue
		}
		var def *AdventureNPC
		for i := range rules.NPCs {
			if rules.NPCs[i].CharacterSourceKey == profile.SourceKey {
				def = &rules.NPCs[i]
				break
			}
		}
		if def == nil {
			continue
		}
		actor, err := AdventureActorFromState(p.State, actorID)
		if err != nil || actor.Rank == 0 || actor.SourceKey != profile.SourceKey {
			raw, _ := json.Marshal(def.Actor)
			if json.Unmarshal(raw, &actor) != nil {
				continue
			}
		}
		if actor.Health <= 0 || actor.Battle != nil && actor.Battle.Status == "active" {
			continue
		}
		progress := AdventureNPCProgress{SettledThrough: before.Seconds}
		if actor.Growth != nil {
			progress = *actor.Growth
		}
		start := max(before.Seconds, progress.SettledThrough)
		if start >= after.Seconds {
			continue
		}
		progress.SettledThrough = after.Seconds
		progress.LastInterval = fmt.Sprintf("%s/%s/%d/%d", branchID, actorID, start, after.Seconds)
		progress.Status = "idle"
		percent := def.Plan.TrainingPercent
		// Existing committed arrangement can suspend an authored plan; it cannot
		// manufacture a new teacher, skill, resource or larger allocation.
		arrangement, _ := actorStateFieldValue(p.State, actorID, xiuxianFieldArrangement).(string)
		if arrangement == xiuxianArrangementOccupied || arrangement == xiuxianArrangementTravel {
			percent = 0
		}
		action := def.Plan.Action
		if arrangement == xiuxianArrangementRecovering {
			action = "rest"
			percent = 100
		}
		if action != "none" && percent > 0 {
			progress.WorkUnits += (after.Seconds - start) * int64(percent)
			hours := progress.WorkUnits / (3600 * 100)
			progress.WorkUnits %= 3600 * 100
			for hours > 0 {
				batch := min(hours, 100000)
				hours -= batch
				// Reuse the exact player solver. Failed plans spend the elapsed window,
				// retaining only sub-hour fractions; missing sources never bank free study.
				candidate := actor
				candidate.Growth = nil
				next, _, resolveErr := ResolveAdventureAction(rules, candidate, AdventureAction{Action: action, SkillID: def.Plan.SkillID, Hours: int(batch)}, progress.LastInterval)
				if resolveErr != nil {
					progress.Status = "blocked"
					slog.Debug("NPC adventure growth blocked", "actor_id", actorID, "reason", resolveErr)
					break
				} else {
					if next.Rank > def.Plan.RankLimit {
						next.Rank = def.Plan.RankLimit
						next.MaxHealth = actor.MaxHealth + (next.Rank-actor.Rank)*20
						next.MaxEnergy = actor.MaxEnergy + (next.Rank-actor.Rank)*10
						next.Health = min(next.Health, next.MaxHealth)
						next.Energy = min(next.Energy, next.MaxEnergy)
						next.Experience = 0
					}
					actor = next
					progress.Status = "settled"
					if action == "rest" && actor.Health == actor.MaxHealth && actor.Energy == actor.MaxEnergy && actor.Injury == 0 {
						break
					}
				}
			}
		}
		actor.Family = rules.Family
		actor.SourceKey = profile.SourceKey
		actor.Growth = &progress
		ops := adventureActorOps(actor, turnID)
		for i := range ops {
			ops[i].ActorID = actorID
			ops[i].SourceKind = StateOpSourceNPCGrowth
			ops[i].SourceID = progress.LastInterval
		}
		var metadata []interactivestate.Op
		metadata = append(metadata, interactivestate.Op{Op: "set", Path: actorStateRoot + "." + actorID + ".template_id", Value: AdventureNPCTemplateID, SourceKind: StateOpSourceNPCGrowth})
		if _, exists := actors[actorID]; !exists {
			metadata = []interactivestate.Op{{Op: "set", Path: actorStateRoot + "." + actorID, Value: map[string]any{"id": actorID, "name": profile.Name, "template_id": AdventureNPCTemplateID, "state": map[string]any{}}, SourceKind: StateOpSourceNPCGrowth}}
		}
		id := newID("sd")
		events = append(events, newStateDeltaEventWithActorOps(id, head, branchID, now, metadata, ops))
		head = id
	}
	return events, head
}

const AdventureNPCTemplateID = "adventure-npc"

// The encounter remains bound to one established character. Archived people
// cannot be respawned by selecting an authored opponent ID.
func adventureEncounterActor(r AdventureRules, p *storyBranchProjection, input AdventureAction) (string, AdventureActor, error) {
	if input.Action != "battle_start" {
		return "", AdventureActor{}, nil
	}
	source := ""
	requiresPresence := false
	for _, enemy := range r.Opponents {
		if enemy.ID == input.OpponentID {
			source = enemy.CharacterSourceKey
			requiresPresence = enemy.RequiresPresence
		}
	}
	if source == "" {
		return "", AdventureActor{}, nil
	}
	if p != nil {
		if _, blocked := recordOnlyCatalogPerson(p.PackCatalog, source, ""); blocked {
			return "", AdventureActor{}, fmt.Errorf("record-only historical character cannot enter a live encounter")
		}
	}
	if p == nil || p.Characters == nil {
		if requiresPresence {
			return "", AdventureActor{}, fmt.Errorf("named opponent has not been established on this branch")
		}
		return "", AdventureActor{}, nil
	}
	for _, profile := range p.Characters.Profiles {
		if profile.SourceKey != source {
			continue
		}
		if requiresPresence {
			present := false
			for _, id := range StorySceneFromState(p.State, "").Present {
				present = present || id == profile.ID
			}
			if !present {
				return "", AdventureActor{}, fmt.Errorf("named opponent is not present in the committed scene")
			}
		}
		if actorIsArchived(p.State, profile.ID) {
			return "", AdventureActor{}, fmt.Errorf("opponent is no longer active")
		}
		actor, err := AdventureActorFromState(p.State, profile.ID)
		if err == nil && actor.Rank > 0 && actor.SourceKey == profile.SourceKey {
			return profile.ID, actor, nil
		}
		for _, npc := range r.NPCs {
			if npc.CharacterSourceKey == source {
				return profile.ID, npc.Actor, nil
			}
		}
	}
	if requiresPresence {
		return "", AdventureActor{}, fmt.Errorf("named opponent has not been established on this branch")
	}
	return "", AdventureActor{}, nil
}
func bindAdventureEncounter(b *AdventureBattle, actorID string, actor AdventureActor, r AdventureRules) {
	if b == nil || actorID == "" || b.Scale != "person" {
		return
	}
	b.EnemyActorID = actorID
	b.EnemyRank = actor.Rank
	b.EnemyMaxEnergy = actor.MaxEnergy
	b.Health = actor.Health
	b.MaxHealth = actor.MaxHealth
	b.EnemyEnergy = actor.Energy
	b.EnemyInjury = actor.Injury
	b.EnemyPoison = actor.Poison
	b.EnemySoftLimbs = actor.SoftLimbs
	b.EnemyRushBlocked = actor.RushBlocked
	b.EnemyLastSeal = actor.LastSealTarget == DefaultActorID
	b.EnemyInventory = map[string]int{}
	b.EnemyEquipment = map[string]string{}
	for id, value := range actor.Equipment {
		b.EnemyEquipment[id] = value
	}
	b.EnemyItemCharges = map[string]int{}
	for id, value := range actor.ItemCharges {
		b.EnemyItemCharges[id] = value
	}
	b.EnemyItemReadyAt = map[string]int64{}
	for id, value := range actor.ItemReadyAt {
		b.EnemyItemReadyAt[id] = value
	}

	b.EnemySkillRanks = map[string]int{}
	for id, n := range actor.Inventory {
		b.EnemyInventory[id] = n
	}
	for id, n := range actor.Skills {
		b.EnemySkillRanks[id] = n
	}
	for _, enemy := range r.Opponents {
		if enemy.ID == b.OpponentID {
			b.Attack += max(0, actor.Rank-max(1, enemy.Rank)) * 3
			b.Defense += max(0, actor.Rank-max(1, enemy.Rank))
			break
		}
	}
}
func adventureEnemySettlementOps(p *storyBranchProjection, r AdventureRules, b *AdventureBattle, sourceID string) []ActorStateOp {
	if b == nil || b.EnemyActorID == "" || b.Scale != "person" {
		return nil
	}
	actor, err := AdventureActorFromState(p.State, b.EnemyActorID)
	if err != nil || actor.Rank == 0 || actor.SourceKey != b.CharacterSourceKey {
		for _, npc := range r.NPCs {
			if npc.CharacterSourceKey == b.CharacterSourceKey {
				actor = npc.Actor
				break
			}
		}
	}
	if actor.Rank == 0 {
		return nil
	}
	actor.SourceKey = b.CharacterSourceKey
	actor.Health = max(1, b.Health)
	actor.Energy = b.EnemyEnergy
	actor.Injury = b.EnemyInjury
	actor.Inventory = b.EnemyInventory
	actor.Equipment = b.EnemyEquipment
	actor.ItemCharges = b.EnemyItemCharges
	actor.ItemReadyAt = b.EnemyItemReadyAt

	actor.Poison = b.EnemyPoison
	actor.SoftLimbs = b.EnemySoftLimbs
	actor.RushBlocked = b.EnemyRushBlocked
	actor.LastSealTarget = ""
	if b.EnemyLastSeal {
		actor.LastSealTarget = DefaultActorID
	}
	if b.Status == "victory" {
		actor.Injury = max(50, actor.Injury)
	}
	ops := adventureActorOps(actor, sourceID)
	for i := range ops {
		ops[i].ActorID = b.EnemyActorID
	}
	// Successful withdrawal changes presence, not identity or home location.
	// The same committed scene must not immediately summon the departed NPC.
	if b.Status == "enemy_escaped" || (b.Status == "standoff" && b.EnemyLastAction == "escape") {
		present := StorySceneFromState(p.State, "").Present
		remaining := make([]any, 0, len(present))
		for _, id := range present {
			if id != b.EnemyActorID {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) != len(present) {
			ops = append(ops, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: remaining, SourceKind: "adventure_settlement", SourceID: sourceID, Reason: "The bound opponent left this encounter."})
		}
	}
	return ops
}

// Private fields are declared in the frozen schema as well as withheld by the
// API projection. Empty defaults cannot grant a new NPC the player's inventory.
func AdventureNPCStateFields(r AdventureRules) []ActorStateField {
	fields := AdventureStateFields(r)
	for i := range fields {
		fields[i].Visibility = ActorStateVisibilityPrivate
		if fields[i].Type == "object" {
			fields[i].Default = map[string]any{}
		} else {
			fields[i].Default = 0
		}
	}
	fields = append(fields, ActorStateField{Name: xiuxianFieldArrangement, Type: "enum", Default: "照常", Options: []string{"照常", xiuxianArrangementOccupied, xiuxianArrangementTravel, xiuxianArrangementRecovering}, Visibility: ActorStateVisibilityPrivate, Description: "本人当前生活安排；杂务与旅行暂停既有学习计划，养伤按休息规则恢复。", UpdateInstruction: "只依据已成立的人物行动改变安排；不得用安排补造技能、师承或资源。"})
	return fields
}

func adventureEnemyMetadataOps(p *storyBranchProjection, b *AdventureBattle) []interactivestate.Op {
	if b == nil || b.EnemyActorID == "" || b.Scale != "person" {
		return nil
	}
	actorID := b.EnemyActorID
	return []interactivestate.Op{{Op: "set", Path: actorStateRoot + "." + actorID + ".id", Value: actorID, SourceKind: "adventure_settlement"}, {Op: "set", Path: actorStateRoot + "." + actorID + ".template_id", Value: AdventureNPCTemplateID, SourceKind: "adventure_settlement"}}
}
