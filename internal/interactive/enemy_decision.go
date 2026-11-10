package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
)

// These are explicit audience projections, not aliases of canonical actors.
// They never include the player's submitted intent, hidden stock or abilities.
type EnemyDecisionCandidate struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type EnemyDecisionActor struct {
	ActorID        string                   `json:"actor_id"`
	Name           string                   `json:"name"`
	Attitude       string                   `json:"attitude,omitempty"`
	Personality    []string                 `json:"personality,omitempty"`
	Motives        []string                 `json:"motives,omitempty"`
	Bounds         []string                 `json:"bounds,omitempty"`
	KnownSituation []string                 `json:"known_situation"`
	Candidates     []EnemyDecisionCandidate `json:"candidates"`
}
type EnemyDecisionSnapshot struct {
	BranchID string               `json:"branch_id"`
	Head     string               `json:"head"`
	Round    int                  `json:"round"`
	Enemies  []EnemyDecisionActor `json:"enemies"`
}
type EnemyDecisionPreparation struct {
	Snapshot  EnemyDecisionSnapshot
	Replayed  bool
	Battle    *BattleRoundOutcome
	Adventure *AdventureActionResult
}

func enemyDecisionProfile(id, name string, characters *character.State) EnemyDecisionActor {
	view := EnemyDecisionActor{ActorID: id, Name: name}
	if characters != nil {
		for _, profile := range characters.Profiles {
			if profile.ID == id {
				view.Name = profile.Name
				view.Attitude = profile.Attitude
				view.Personality = append([]string(nil), profile.Personality...)
				view.Motives = append([]string(nil), profile.Motives...)
				view.Bounds = append([]string(nil), profile.Bounds...)
				break
			}
		}
	}
	return view
}
func visibleDecisionCondition(current, maximum int) string {
	if current <= 0 {
		return "倒下"
	}
	if current*4 <= maximum {
		return "重伤"
	}
	if current*2 <= maximum {
		return "明显受伤"
	}
	if current < maximum {
		return "轻伤"
	}
	return "未见伤势"
}

// PrepareBattleEnemyDecision releases the read lease before the caller invokes
// a model. The selected IDs must return with this head/round and are checked
// again under the ordinary mutation lease; no model call holds the Store lock.
func (s *Store) PrepareBattleEnemyDecision(storyID string, req BattleRoundRequest) (EnemyDecisionPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	defer release()
	branchID, meta, records, err := s.battleBranchLocked(storyID, req.BranchID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	committed, found, err := combatRecordForCommandLocked(meta, records, branchID, strings.TrimSpace(req.CommandID), battleRoundCommitHash(req.CommandHash, req.PlayerIntent))
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	if found {
		result := battleOutcomeFromRecord(committed)
		return EnemyDecisionPreparation{Replayed: true, Battle: &result}, nil
	}
	p, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	live, ok := p.Combats[req.SessionID]
	if !ok {
		return EnemyDecisionPreparation{}, fmt.Errorf("battle does not exist")
	}
	if live.Ended {
		return EnemyDecisionPreparation{Replayed: true, Battle: &BattleRoundOutcome{Session: live}}, nil
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	live = battleWithCurrentParticipation(live, p.State, system)
	if err := requirePlayerAction(live, req.Actions); err != nil {
		return EnemyDecisionPreparation{}, err
	}
	// Match settlement's learned-version resolution before declaration checks.
	// The HTTP boundary carries generated techniques as an identity only.
	if err := validateDeclaredTechniques(live, req.Actions); err != nil {
		return EnemyDecisionPreparation{}, err
	}
	canonical := resolveDeclaredTechniques(live, req.Actions)
	if action, present := canonical[DefaultActorID]; present {
		if reason := cultivation.CombatDeclarationReason(live, DefaultActorID, action); reason != "" {
			return EnemyDecisionPreparation{}, fmt.Errorf("%w: actor=%s reason=%s", ErrCombatActionRejected, DefaultActorID, reason)
		}
	}
	if _, err := playerTakeLifeTarget(live, canonical, req.PlayerIntent); err != nil {
		return EnemyDecisionPreparation{}, err
	}
	snapshot := EnemyDecisionSnapshot{BranchID: branchID, Head: p.Head, Round: live.Round}
	for _, actor := range live.Combatants {
		if actor.Side != cultivation.SideEnemy || !actor.Active() {
			continue
		}
		view := enemyDecisionProfile(actor.ID, actor.ID, p.Characters)
		view.KnownSituation = []string{fmt.Sprintf("本轮%d；本人生命%d/%d，灵力%d/%d，经脉负担%d，已有退路%t。", live.Round, actor.Attributes.Health, actor.Attributes.HealthMax, actor.Attributes.Mana, actor.Attributes.ManaMax, actor.Burden.Current, actor.RouteExists)}
		for _, other := range live.Combatants {
			if other.ID == actor.ID || !other.Active() {
				continue
			}
			status := fmt.Sprintf("可见参战者%s：%s。", other.ID, visibleDecisionCondition(other.Attributes.Health, other.Attributes.HealthMax))
			if other.Burn.RemainingTicks > 0 {
				status += "身上有燃烧。"
			}
			if other.Shield.Amount > 0 {
				status += "有可见护盾。"
			}
			if _, ok := live.Preparations[other.ID]; ok {
				status += "上轮已经开始蓄势。"
			}
			if other.OfferText != "" {
				status += "已经提出求饶。"
			}
			view.KnownSituation = append(view.KnownSituation, status)
		}
		for _, option := range cultivation.EnemyActionOptions(live, actor.ID) {
			view.Candidates = append(view.Candidates, EnemyDecisionCandidate{ID: option.ID, Description: option.Description})
		}
		if len(view.Candidates) > 0 {
			snapshot.Enemies = append(snapshot.Enemies, view)
		}
	}
	return EnemyDecisionPreparation{Snapshot: snapshot}, nil
}

func (s *Store) PrepareAdventureEnemyDecision(storyID, branchID, commandID, expectedHead string, input AdventureAction) (EnemyDecisionPreparation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(commandID) == "" || len(commandID) > 160 || expectedHead == "" || input.Action != "battle_round" {
		return EnemyDecisionPreparation{}, fmt.Errorf("adventure decision requires a battle command and expected head")
	}
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	defer release()
	meta, records, err := s.readStoryLocked(storyID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return EnemyDecisionPreparation{}, fmt.Errorf("unknown adventure branch")
	}
	hash := ContentHashOf(struct {
		Head  string
		Input AdventureAction
	}{expectedHead, input})
	path, _, err := walkEventPath(branch.Head, eventsByID(records))
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeStateDelta {
			continue
		}
		var delta StateDeltaEvent
		if err := mapToStruct(record.Raw, &delta); err != nil {
			return EnemyDecisionPreparation{}, err
		}
		if delta.AdventureReceipt != nil && delta.AdventureReceipt.CommandID == commandID {
			if delta.AdventureReceipt.CommandHash != hash {
				return EnemyDecisionPreparation{}, fmt.Errorf("adventure command identity conflict")
			}
			result := delta.AdventureReceipt.Result
			return EnemyDecisionPreparation{Replayed: true, Adventure: &result}, nil
		}
	}
	if branch.Head != expectedHead {
		return EnemyDecisionPreparation{}, fmt.Errorf("adventure state changed; reload the window")
	}
	p, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if system.AdventureRules == nil {
		return EnemyDecisionPreparation{}, fmt.Errorf("story has no adventure rules")
	}
	actor, err := AdventureActorFromState(p.State, DefaultActorID)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	if actor.Battle == nil || actor.Battle.Status != "active" {
		return EnemyDecisionPreparation{}, fmt.Errorf("no active adventure battle")
	}
	rules, err := adventureRulesForActor(*system.AdventureRules, actor)
	if err != nil {
		return EnemyDecisionPreparation{}, err
	}
	// The detached preflight validates the player request only. No candidate or
	// simulated outcome from it enters the enemy's knowledge or committed state.
	preflight := input
	preflight.enemyChoice = "ordinary"
	if _, _, err := ResolveAdventureActionAt(rules, actor, preflight, commandID, p.clockSeconds()); err != nil {
		return EnemyDecisionPreparation{}, err
	}
	snapshot := EnemyDecisionSnapshot{BranchID: branchID, Head: p.Head, Round: actor.Battle.Round}
	b := actor.Battle
	if b.EnemyControl == 0 && !b.EnemySoftLimbs {
		id := adventureDecisionActorID(b)
		view := enemyDecisionProfile(id, b.OpponentID, p.Characters)
		for _, op := range rules.Opponents {
			if op.ID == b.OpponentID && view.Name == b.OpponentID {
				view.Name = op.Name
			}
		}
		view.KnownSituation = []string{fmt.Sprintf("本轮%d，尺度%s；本人生命%d/%d，气力%d/%d，伤势%d，中毒%d。", b.Round, b.Scale, b.Health, b.MaxHealth, b.EnemyEnergy, b.EnemyMaxEnergy, b.EnemyInjury, b.EnemyPoison)}
		view.KnownSituation = append(view.KnownSituation, adventureVisibleOpponent(actor)...)
		view.KnownSituation = append(view.KnownSituation, "可见对方参战尺度："+AdventurePlayerBattleScale(b)+"。")
		if b.PlayerCeasefireOffer {
			view.KnownSituation = append(view.KnownSituation, "对方先前已经提出停战，可选择接受；不是本轮尚未执行的意图。")
		}
		if b.PlayerRetreatProgress > 0 {
			view.KnownSituation = append(view.KnownSituation, "对方已经开始撤离，可选择追击；追击不同时攻击。")
		}
		if b.NoProgressRounds > 0 {
			view.KnownSituation = append(view.KnownSituation, fmt.Sprintf("此前连续%d轮未见有效伤势变化；可以提议不分胜负，但不能替对方同意。", b.NoProgressRounds))
		}
		if b.ShipCombat != nil {
			view.KnownSituation = append(view.KnownSituation, fmt.Sprintf("本人舰队已登记%d艘同质舰聚合，拦截能力%d、追踪能力%d；不可凭空获得援军。", b.ShipCombat.VesselCount, b.ShipCombat.Interdiction, b.ShipCombat.Tracking))
		}
		if actor.Control > 0 {
			view.KnownSituation = append(view.KnownSituation, "对手仍受已发生的控制影响。")
		}
		view.Candidates = adventureEnemyDecisionCandidates(rules, &actor)
		snapshot.Enemies = []EnemyDecisionActor{view}
	}
	return EnemyDecisionPreparation{Snapshot: snapshot}, nil
}

func adventureDecisionActorID(b *AdventureBattle) string {
	if b.EnemyActorID != "" {
		return b.EnemyActorID
	}
	return "opponent"
}

// Vehicle encounters observe the participating hull, not the hidden pilot's
// personal health. Fuel, cargo, unused modules and exact shield values stay out.
func adventureVisibleOpponent(actor AdventureActor) []string {
	health, maximum := actor.Health, actor.MaxHealth
	shield := false
	if actor.Battle != nil && AdventurePlayerBattleScale(actor.Battle) != "person" {
		asset := actor.Assets[actor.Battle.AssetID]
		health, maximum, shield = asset.Health, asset.MaxHealth, asset.Shield > 0
	}
	visible := []string{"可见对手：" + visibleDecisionCondition(health, maximum) + "。"}
	if shield {
		visible = append(visible, "对方参战载具有可见护盾。")
	}
	return visible
}
