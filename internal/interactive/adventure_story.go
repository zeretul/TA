package interactive

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// AdventureWindowState exposes the frozen catalog and canonical protagonist.
// Head is a compare-and-swap token; every window action must send it back.
type AdventureWindowState struct {
	ClockSeconds int64          `json:"clock_seconds"`
	Location     string         `json:"location"`
	Rules        AdventureRules `json:"rules"`
	Actor        AdventureActor `json:"actor"`
	Head         string         `json:"head"`
}
type AdventureActionResult struct {
	EnemyDecision   *EnemyDecisionReceipt `json:"enemy_decision,omitempty"`
	TerminalOutcome *TerminalOutcome      `json:"terminal_outcome,omitempty"`
	EventID         string                `json:"event_id"`
	Actor           AdventureActor        `json:"actor"`
	Outcome         AdventureOutcome      `json:"outcome"`
}
type AdventureActionReceipt struct {
	CommandID   string                `json:"command_id"`
	CommandHash string                `json:"command_hash"`
	Result      AdventureActionResult `json:"result"`
}

func AdventureOwnsField(id string) bool {
	switch id {
	case AdventureStateField, "历练等级", "生命", "生命上限", "内力", "内力上限", "气力", "气力上限", "精力", "精力上限", "伤势程度", "钱财":
		return true
	}
	return false
}

func validateAdventureSchemaAdaptation(adaptation ActorStateSchemaAdaptation) error {
	for _, op := range adaptation.TemplateOps {
		if op.TemplateID != DefaultActorID && op.Template.ID != DefaultActorID && op.TemplateID != AdventureNPCTemplateID && op.Template.ID != AdventureNPCTemplateID {
			continue
		}
		if op.Op != "fields" {
			return fmt.Errorf("adventure protagonist template is frozen")
		}
		for _, field := range op.FieldOps {
			if AdventureOwnsField(field.FieldID) || AdventureOwnsField(field.Field.Name) {
				return fmt.Errorf("adventure rule fields cannot be adapted")
			}
		}
	}
	for _, op := range adaptation.InitialActorOps {
		for field := range op.Actor.State {
			if AdventureOwnsField(field) {
				return fmt.Errorf("adventure initialization is frozen")
			}
		}
		if op.ActorID == DefaultActorID || op.Actor.ID == DefaultActorID {
			return fmt.Errorf("adventure protagonist initialization is frozen by its selected origin")
		}
	}
	for _, op := range adaptation.ActorOps {
		if AdventureOwnsField(op.FieldID) {
			return fmt.Errorf("adventure fields cannot be changed through schema adaptation")
		}
		for field := range op.Actor.State {
			if AdventureOwnsField(field) {
				return fmt.Errorf("adventure initialization cannot be changed through schema adaptation")
			}
		}
		if op.ActorID == DefaultActorID || op.Actor.ID == DefaultActorID {
			if op.Op != "set" || AdventureOwnsField(op.FieldID) {
				return fmt.Errorf("adventure state cannot be changed through schema adaptation")
			}
		}
	}
	return nil
}

// Preconditions use the committed scene, never the proposed turn's relocation.
func validateAdventureScene(r AdventureRules, state map[string]any, input AdventureAction) error {
	location := strings.TrimSpace(StorySceneFromState(state, "").Location)
	switch input.Action {
	case "buy", "sell", "repair", "work":
		if input.Action == "repair" && input.MarketID == "" {
			return nil
		}
		for _, market := range r.Markets {
			if market.ID == input.MarketID && matchesAdventureLocation(location, market.Location, market.LocationAliases, r.world) {
				return nil
			}
		}
		return fmt.Errorf("adventure market is not at the committed scene location")
	case "battle_start":
		for _, enemy := range r.Opponents {
			if enemy.ID == input.OpponentID && (enemy.RequiresPresence || matchesAdventureLocation(location, enemy.Location, enemy.LocationAliases, r.world)) {
				return nil
			}
		}
		return fmt.Errorf("adventure opponent is not at the committed scene location")
	}
	return nil
}

// Use the same registered place identity as journeys and authored tasks. Exact
// local names still work for custom worlds without a registered route catalog.
func matchesAdventureLocation(location, primary string, aliases []string, worlds ...*WorldContent) bool {
	currentID, currentKnown := registeredPlaceInText(location, worlds...)
	for _, place := range append([]string{primary}, aliases...) {
		if place == "" {
			continue
		}
		if targetID, targetKnown := registeredPlaceInText(place, worlds...); targetKnown {
			if currentKnown && currentID == targetID {
				return true
			}
			continue
		}
		if location == place || strings.HasPrefix(location, place+" · ") {
			return true
		}
	}
	return false
}

func adventureActorOps(a AdventureActor, sourceID string) []ActorStateOp {
	data, _ := json.Marshal(a)
	var canonical map[string]any
	_ = json.Unmarshal(data, &canonical)
	energyName := "气力"
	if a.Family == "wuxia" {
		energyName = "内力"
	} else if a.Family == "fantasy" {
		energyName = "精力"
	}
	values := []struct {
		name  string
		value any
	}{{AdventureStateField, canonical}, {"历练等级", a.Rank}, {"生命", a.Health}, {"生命上限", a.MaxHealth}, {energyName, a.Energy}, {energyName + "上限", a.MaxEnergy}, {"伤势程度", a.Injury}, {"钱财", a.Currency}}
	ops := make([]ActorStateOp, 0, len(values))
	for _, v := range values {
		ops = append(ops, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: v.name, Value: v.value, SourceKind: "adventure_settlement", SourceID: sourceID, SourceTurnID: sourceID, Reason: "题材规则正式结算"})
	}
	return ops
}

// AdventureRewardOps settles an already-authorized quest reward or payment.
// Signed amounts are allowed for objective costs, never below actual ownership.
func AdventureRewardOps(state map[string]any, system StoryDirectorActorStateSystem, currency int, items map[string]int, experience int, sourceID string) ([]ActorStateOp, error) {
	return adventureRewardOps(state, system, currency, items, experience, sourceID, nil)
}

// A transfer receipt is supplied only by the quest planner after its authored
// promise, successful conditions and actual owner have all been checked.
func adventureRewardOps(state map[string]any, system StoryDirectorActorStateSystem, currency int, items map[string]int, experience int, sourceID string, transferred map[string]string) ([]ActorStateOp, error) {
	if system.AdventureRules == nil {
		return nil, fmt.Errorf("adventure reward requires frozen rules")
	}
	a, err := AdventureActorFromState(state, DefaultActorID)
	if err != nil {
		return nil, err
	}
	rules, err := adventureRulesForActor(*system.AdventureRules, a)
	if err != nil {
		return nil, err
	}
	if currency < -a.Currency || currency > 1000000000-a.Currency || experience < 0 || experience > 1000000 {
		return nil, fmt.Errorf("adventure reward balance out of range")
	}
	known := map[string]bool{}
	for _, v := range rules.Items {
		known[v.ID] = true
	}
	if a.Inventory == nil {
		a.Inventory = map[string]int{}
	}
	for id, n := range items {
		if n < 0 {
			for _, equipped := range a.Equipment {
				if equipped == id {
					return nil, fmt.Errorf("equipped reward cost cannot be consumed: %s", id)
				}
			}
			if _, owned := a.Assets[id]; owned {
				return nil, fmt.Errorf("asset cannot be consumed as reward cost: %s", id)
			}
		}
		if !known[id] || n < -a.Inventory[id] || n > 1000000-a.Inventory[id] {
			return nil, fmt.Errorf("adventure reward item unavailable: %s", id)
		}
	}
	if a.Acquired == nil {
		a.Acquired = map[string]bool{}
	}
	for id, n := range items {
		if n > 0 {
			for _, def := range rules.Items {
				if def.ID != id || !def.Unique {
					continue
				}
				if a.Acquired[id] || a.Inventory[id] > 0 || n > 1 {
					return nil, fmt.Errorf("unique reward already acquired: %s", id)
				}
				owner, err := adventureUniqueItemOwner(rules, state, id)
				if err != nil {
					return nil, err
				}
				if owner != "" && transferred[id] != owner {
					return nil, fmt.Errorf("unique reward requires its actual owner's agreed handover: %s", id)
				}
			}
			a.Acquired[id] = true
		}
	}
	a.Family = system.AdventureRules.Family
	a.Currency += currency
	a.Experience += experience
	for id, n := range items {
		a.Inventory[id] += n
	}
	advanceAdventureRank(rules, &a)
	if err := ValidateAdventureActor(rules, a); err != nil {
		return nil, err
	}
	return adventureActorOps(a, sourceID), nil
}

func (s *Store) AdventureWindow(storyID, branchID string) (AdventureWindowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return AdventureWindowState{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return AdventureWindowState{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if system.AdventureRules == nil || CombatFamilyForWorld(meta.PackBinding, system) != system.AdventureRules.Family {
		return AdventureWindowState{}, ErrAdventureUnavailableForWorld
	}
	a, err := AdventureActorFromState(projection.State, DefaultActorID)
	if err != nil {
		return AdventureWindowState{}, err
	}
	rules, err := adventureRulesForActor(*system.AdventureRules, a)
	if err != nil {
		return AdventureWindowState{}, err
	}
	return AdventureWindowState{Rules: rules, Actor: a, Head: projection.Head, ClockSeconds: projection.clockSeconds(), Location: StorySceneFromState(projection.State, "").Location}, nil
}

// CommitAdventureAction commits one independent battle exchange through the
// existing journal transaction and interval machinery. No story turn is made.
func (s *Store) CommitAdventureAction(storyID, branchID, commandID, expectedHead string, input AdventureAction) (AdventureActionResult, error) {
	return s.CommitAdventureActionWithEnemyDecision(storyID, branchID, commandID, expectedHead, input, nil, "")
}

// CommitAdventureActionWithEnemyDecision accepts server-only candidate IDs. The
// client input fingerprint stays unchanged; retries return the committed outcome.
func (s *Store) CommitAdventureActionWithEnemyDecision(storyID, branchID, commandID, expectedHead string, input AdventureAction, choices map[string]string, status string) (AdventureActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(commandID) == "" || len(commandID) > 160 || expectedHead == "" {
		return AdventureActionResult{}, fmt.Errorf("adventure command requires identity and expected head")
	}
	if input.Action != "battle_round" {
		return AdventureActionResult{}, fmt.Errorf("only battle rounds may be committed from the window")
	}
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return AdventureActionResult{}, err
	}
	defer release()
	meta, records, err := s.readStoryLocked(storyID)
	if err != nil {
		return AdventureActionResult{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return AdventureActionResult{}, fmt.Errorf("unknown adventure branch")
	}
	hash := ContentHashOf(struct {
		Head  string
		Input AdventureAction
	}{expectedHead, input})
	path, _, err := walkEventPath(branch.Head, eventsByID(records))
	if err != nil {
		return AdventureActionResult{}, err
	}
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeStateDelta {
			continue
		}
		var delta StateDeltaEvent
		if err := mapToStruct(record.Raw, &delta); err != nil {
			return AdventureActionResult{}, err
		}
		if delta.AdventureReceipt != nil && delta.AdventureReceipt.CommandID == commandID {
			if delta.AdventureReceipt.CommandHash != hash {
				return AdventureActionResult{}, fmt.Errorf("adventure command identity conflict")
			}
			return delta.AdventureReceipt.Result, nil
		}
	}
	if branch.Head != expectedHead {
		return AdventureActionResult{}, fmt.Errorf("adventure state changed; reload the window")
	}
	if branchIsTerminal(records, branch.Head) {
		return AdventureActionResult{}, branchTerminalError()
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return AdventureActionResult{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	if system.AdventureRules == nil || CombatFamilyForWorld(meta.PackBinding, system) != system.AdventureRules.Family {
		return AdventureActionResult{}, ErrAdventureUnavailableForWorld
	}
	a, err := AdventureActorFromState(projection.State, DefaultActorID)
	if err != nil {
		return AdventureActionResult{}, err
	}
	refreshAdventureFogExposure(*system.AdventureRules, &a, projection.State)
	accepted := map[string]string{}
	enemies := 0
	if a.Battle != nil && a.Battle.Status == "active" {
		enemies = 1
		actorID := adventureDecisionActorID(a.Battle)
		for _, candidate := range adventureEnemyDecisionCandidates(*system.AdventureRules, &a) {
			if candidate.ID == choices[actorID] {
				input.enemyChoice = candidate.ID
				accepted[actorID] = candidate.ID
				break
			}
		}
	}
	next, outcome, err := ResolveAdventureActionAt(*system.AdventureRules, a, input, commandID, projection.clockSeconds())
	if err != nil {
		return AdventureActionResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("adv")
	result := AdventureActionResult{EventID: eventID, Actor: next, Outcome: outcome, EnemyDecision: enemyDecisionReceipt(status, accepted, enemies)}
	if next.Health == 0 && next.Battle.Lethal && next.Battle.Status == "defeat" {
		result.TerminalOutcome = normalizeTerminalOutcomePointer(&TerminalOutcome{Terminal: true, Type: "combat_death", ReasonKey: "storyStage.ended.reason.combatDeath", RuleResolutionID: eventID})
	}
	delta := StateDeltaEvent{V: schemaVersion, Type: StoryEventTypeStateDelta, ID: eventID, ParentID: branch.Head, BranchID: branchID, Ts: now, SchemaVersion: stateOpSchemaVersion, TerminalOutcome: result.TerminalOutcome, ActorOps: adventureActorOps(next, eventID), AdventureReceipt: &AdventureActionReceipt{CommandID: commandID, CommandHash: hash, Result: result}}
	delta.Ops = append(delta.Ops, adventureEnemyMetadataOps(projection, next.Battle)...)
	delta.ActorOps = append(delta.ActorOps, adventureEnemySettlementOps(projection, *system.AdventureRules, next.Battle, eventID)...)
	events := []any{delta}
	head := eventID
	if outcome.DurationSeconds > 0 {
		clock := NewStoryClock(storySceneText(projection.State, storySceneTimeField))
		if projection.Clock != nil {
			clock = *projection.Clock
		}
		after, err := AdvanceStoryClock(clock, outcome.DurationSeconds)
		if err != nil {
			return AdventureActionResult{}, err
		}
		delta.ClockAdvance = &TurnClockAdvance{Action: "adventure", SourceText: commandID, DurationSeconds: outcome.DurationSeconds, Before: clock, After: after}
		events[0] = delta
		interval, nextHead := storyIntervalEvents(projection, system, branchID, head, now, "", DefaultActorID, clock, after)
		events = append(events, interval...)
		head = nextHead
	}
	branch.Head = head
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, events...); err != nil {
		return AdventureActionResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return AdventureActionResult{}, err
	}
	slog.Info("Adventure battle round committed", "story_id", storyID, "branch_id", branchID, "command_id", commandID, "status", next.Battle.Status, "enemy_decision_status", status)
	return result, nil
}

// AdventureStateFields keeps display fields and the one canonical value in the
// same frozen schema. The scalar fields are projections, never model-writable.
func AdventureStateFields(r AdventureRules) []ActorStateField {
	a := NewAdventureActor(r)
	fields := []ActorStateField{}
	for _, op := range adventureActorOps(a, "") {
		kind := "number"
		display := "stat"
		if op.FieldID == AdventureStateField {
			kind = "object"
			display = "block"
		}
		fields = append(fields, ActorStateField{Name: op.FieldID, Type: kind, Default: op.Value, Group: "历练", Display: display, UpdateInstruction: "由world.adventure及正式奖励结算，禁止通过state_changes直接修改。"})
	}
	return fields
}
