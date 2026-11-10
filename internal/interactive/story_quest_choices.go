package interactive

import (
	"fmt"
	"strings"

	"ta/internal/quest"
)

// planQuestChoice uses the same frozen actor and canonical task instance as
// normal world actions. A failed attempt still spends its authored interval and
// consumable costs and leaves a consequence; success-only handover objects stay
// with the player on failure. Neither a retry nor prose can reroll the attempt.
func planQuestChoice(p *storyBranchProjection, system StoryDirectorActorStateSystem, d quest.Definition, prior quest.Instance, exists bool, req QuestRequest) (questPlannedAction, error) {
	fail := func(message string) (questPlannedAction, error) {
		return questPlannedAction{}, fmt.Errorf("%w: %s", quest.ErrInvalidState, message)
	}
	if p == nil || system.AdventureRules == nil {
		return fail("task choice requires adventure rules")
	}
	if !exists || (prior.Status != quest.StatusAccepted && prior.Status != quest.StatusCompleted) {
		return fail("task must be accepted before choosing")
	}
	if prior.ExpiredAt(p.clockSeconds()) {
		return questPlannedAction{}, ErrQuestExpired
	}
	c, ok := d.Choice(strings.TrimSpace(req.ChoiceID))
	if !ok {
		return fail("choice is not authored for this task")
	}
	previous, retry := prior.Choices[c.ObjectiveID]
	if retry && (previous.Outcome != "failure" || !previous.EchoResolved) {
		return fail("this stage already has an outcome or is awaiting its consequence")
	}
	if len(c.LocationIDs) > 0 && !storyLocationIsAny(p.State, c.LocationIDs, p.worldContent()) {
		return fail("choice requires its authored location")
	}
	for _, sourceKey := range c.RequiredCharacterSourceKeys {
		if _, present := questEchoPartner(p, sourceKey); !present {
			return fail("choice requires its established present participant: " + sourceKey)
		}
	}
	for _, handover := range c.ItemHandovers {
		if _, present := questEchoPartner(p, handover.CharacterSourceKey); !present {
			return fail("choice requires its established present item recipient: " + handover.CharacterSourceKey)
		}
	}
	for _, id := range c.AfterObjectives {
		found := false
		for _, done := range prior.CompletedObjectiveIDs {
			if done == id {
				found = true
			}
		}
		if result, authored := prior.Choices[id]; authored && result.Outcome != "success" {
			found = false
		}
		if !found {
			return fail("earlier stage has not succeeded")
		}
	}
	requiredCount := c.RequiredTaskCount
	if requiredCount == 0 {
		requiredCount = len(c.RequiredTasks)
	}
	completedCount := 0
	for _, id := range c.RequiredTasks {
		other, ok := p.Quests[id]
		if ok && other.Status == quest.StatusCompleted && questHasSuccess(other) {
			completedCount++
		}
	}
	if completedCount < requiredCount {
		return fail("required earlier tasks have not succeeded")
	}
	if c.RequiredJourney != "" {
		arrived := false
		for _, trip := range p.Journeys {
			if trip.RouteID == c.RequiredJourney && trip.TotalMinutes > 0 && trip.ElapsedMinutes >= trip.TotalMinutes {
				arrived = true
			}
		}
		if !arrived {
			return fail("required journey has not arrived")
		}
	}
	a, err := AdventureActorFromState(p.State, DefaultActorID)
	if err != nil {
		return fail(err.Error())
	}
	if err := ValidateAdventureActor(*system.AdventureRules, a); err != nil {
		return fail(err.Error())
	}
	if a.Health <= 0 || (a.Battle != nil && a.Battle.Status == "active") {
		return fail("finish the active battle or recover first")
	}
	if a.Rank < c.RequiredRank {
		return fail("required experience rank has not been reached")
	}
	if c.RequiredSkill != "" && a.Skills[c.RequiredSkill] < max(1, c.RequiredSkillRank) {
		return fail("required skill has not been mastered")
	}
	if c.RequiredVictory != "" && !a.Defeated[c.RequiredVictory] {
		return fail("required opponent has not been defeated")
	}
	workingAssetID := ""
	if c.RequiredAsset != "" {
		id := adventureDeployedAssetID(a, c.RequiredAsset)
		workingAssetID = id
		if asset, ok := a.Assets[id]; id == "" || !ok || asset.Health <= 0 || asset.Fuel < max(1, c.AssetFuelCost) || asset.DisabledUntil > p.clockSeconds() {
			return fail("required asset is not deployed")
		}
	}
	energyCost := 0
	if c.UseSkillID != "" {
		rules, err := adventureRulesForActor(*system.AdventureRules, a)
		if err != nil {
			return fail(err.Error())
		}
		found := false
		for _, skill := range rules.Skills {
			if skill.ID != c.UseSkillID {
				continue
			}
			if a.Skills[skill.ID] < 1 || a.Rank < skill.RequiredRank || skill.RequiredTool != "" && AdventureOwnedItemCount(a, skill.RequiredTool) < 1 {
				return fail("task method requires its learned skill and real tool")
			}
			// The quest resolves the authored task, never a combat technique's
			// damage or effects outside the combat window.
			if skill.Kind != "engineering" && skill.Kind != "repair" && skill.Kind != "social" && skill.Kind != "pilot" {
				return fail("task effort must use a non-combat method")
			}
			energyCost, found = skill.Cost, true
			break
		}
		if !found || a.Energy < energyCost {
			return fail("task method or its energy is unavailable")
		}
	}
	for id, n := range c.RequiredItems {
		if AdventureOwnedItemCount(a, id) < n {
			return fail("required item is not owned")
		}
	}
	capability := a.Rank + a.Skills[c.SkillID]
	// A new attempt requires both the old consequence and demonstrably better
	// preparation. Replaying the original command still returns its old receipt.
	if retry && capability <= previous.Capability {
		return fail("failed stage requires improved preparation before another attempt")
	}
	items, err := adventureItemCosts(a, c.ItemCosts)
	if err != nil {
		return fail(err.Error())
	}
	handedCounts := map[string]int64{}
	for _, handover := range c.ItemHandovers {
		for id, n := range handover.Items {
			handedCounts[id] += int64(n)
		}
	}
	for id, n := range handedCounts {
		if -int64(items[id]) < n {
			return fail("handover requires the exact owned item, not a generated substitute")
		}
	}
	cost := c.CurrencyCost
	if terms := c.Debt; terms != nil {
		loan, exists := p.Quests[terms.TaskID]
		if !exists || !questHasSuccess(loan) || len(loan.Choices) != 1 {
			return fail("loan receipt is missing")
		}
		var borrowedAt int64
		for _, receipt := range loan.Choices {
			borrowedAt = receipt.AtSeconds
		}
		debt := int64(terms.Principal)
		for hour := int64(0); hour < (p.clockSeconds()-borrowedAt)/3600; hour++ {
			debt = (debt*int64(10000+terms.HourlyBasisPoints) + 9999) / 10000
			if debt > quest.MaxQuantity {
				return fail("accrued debt exceeds settlement limit")
			}
		}
		cost += int(debt)
	}
	ops, err := AdventureRewardOps(p.State, system, -cost, items, 0, d.ID+":"+c.ID)
	if err != nil {
		return fail(err.Error())
	}
	clock := NewStoryClock(storySceneText(p.State, storySceneTimeField))
	if p.Clock != nil {
		clock = *p.Clock
	}
	after, err := AdvanceStoryClock(clock, c.DurationSeconds)
	if err != nil {
		return fail(err.Error())
	}
	if adventureOpeningBoundary(p.State, clock.Seconds, after.Seconds, p.worldContent()) < after.Seconds {
		return fail("pending opening hazard interrupts this task interval")
	}
	if prior.ExpiredAt(after.Seconds) {
		return questPlannedAction{}, ErrQuestExpired
	}
	// Capability, not a request-provided success bit, decides the attempt.
	outcome := "success"
	if a.Rank+a.Skills[c.SkillID] < c.Difficulty {
		outcome = "failure"
		// Handovers transfer on success; they are not destroyed by a failed
		// attempt. Keep other authored consumables, fees and effort payable.
		// The full costs and exact originals were validated before this point.
		for id, quantity := range handedCounts {
			items[id] += int(quantity)
		}
	}
	var rewards []quest.Reward
	if outcome == "success" {
		rewards = questUnownedRewards(*system.AdventureRules, a, c.Rewards)
	}
	currency, experience := -cost, 0
	for _, reward := range rewards {
		switch reward.Kind {
		case quest.RewardCurrency:
			if reward.RefID != system.AdventureRules.Currency {
				return fail("choice reward currency differs from world")
			}
			currency += int(reward.Quantity)
		case quest.RewardItem:
			items[reward.RefID] += int(reward.Quantity)
		case quest.RewardExperience:
			experience += int(reward.Quantity)
		default:
			return fail("unsupported choice reward")
		}
	}
	ops, err = adventureQuestOwnedRewardOps(p, system, d, currency, items, experience, d.ID+":"+c.ID)
	if err != nil {
		return fail(err.Error())
	}
	if outcome == "success" && len(c.ItemHandovers) > 0 {
		handovers, handoverErr := adventureQuestItemHandoverOps(intervalProjectionAfterOps(p, ops), system, c, d.ID+":"+c.ID)
		if handoverErr != nil {
			return fail(handoverErr.Error())
		}
		ops = append(ops, handovers...)
	}
	if energyCost > 0 || c.AssetFuelCost > 0 {
		afterCosts, err := AdventureActorFromState(intervalProjectionAfterOps(p, ops).State, DefaultActorID)
		if err != nil {
			return fail(err.Error())
		}
		afterCosts.Energy -= energyCost
		if c.AssetFuelCost > 0 {
			asset := afterCosts.Assets[workingAssetID]
			asset.Fuel -= c.AssetFuelCost
			afterCosts.Assets[workingAssetID] = asset
		}
		if err := ValidateAdventureActor(*system.AdventureRules, afterCosts); err != nil {
			return fail(err.Error())
		}
		playerOps := adventureActorOps(afterCosts, d.ID+":"+c.ID)
		for _, op := range ops {
			if op.ActorID != DefaultActorID {
				playerOps = append(playerOps, op)
			}
		}
		ops = playerOps
	}
	next := cloneQuestInstance(prior)
	if next.Choices == nil {
		next.Choices = map[string]quest.ChoiceResult{}
	}
	due, err := adventureDeadline(after.Seconds, c.EchoAfterSeconds)
	if err != nil {
		return fail(err.Error())
	}
	result := quest.ChoiceResult{ChoiceID: c.ID, Outcome: outcome, AtSeconds: after.Seconds, EchoDueSeconds: due, Capability: capability, EchoEncounterRequired: c.Reaction(outcome) != nil}
	if retry {
		// A real, better-prepared attempt supersedes the prior pending personal
		// response. Do not force a failure conversation before making amends.
		// Previous keeps the spent attempt; already heard character facts stay
		// historical, while only the latest outcome opens new work.
		result.Previous = &previous
		next.SettledRewards = false
		next.SettledAt = ""
	}
	next.Choices[c.ObjectiveID] = result
	if !retry {
		next.CompletedObjectiveIDs = quest.SortedObjectiveIDs(append(next.CompletedObjectiveIDs, c.ObjectiveID))
	}
	if next.SourceRefs == nil {
		next.SourceRefs = map[string]string{}
	}
	next.SourceRefs[c.ObjectiveID] = "choice:" + c.ID + ":" + outcome
	if d.IsComplete(next.CompletedObjectiveIDs) {
		next.Status = quest.StatusCompleted
	}
	if diagnostics := quest.ValidateInstance(d, next); len(diagnostics) > 0 {
		return fail(diagnostics[0].Message)
	}
	return questPlannedAction{Definition: d, Action: QuestActionChoose, Instance: next, Rewards: rewards, Ops: ops, ClockBefore: &clock, ClockAfter: &after}, nil
}

func questHasSuccess(i quest.Instance) bool {
	if len(i.Choices) == 0 {
		return true
	}
	for _, c := range i.Choices {
		if c.Outcome != "success" {
			return false
		}
	}
	return true
}

func questEarnedRewards(d quest.Definition, i quest.Instance) []quest.Reward {
	var rewards []quest.Reward
	if len(d.Choices) == 0 || questHasSuccess(i) {
		rewards = append(rewards, d.Rewards...)
	}
	// Choice rewards were paid atomically with that choice; settlement only pays the task-wide promise.
	return rewards
}

func adventureQuestRewardOps(p *storyBranchProjection, system StoryDirectorActorStateSystem, d quest.Definition, rewards []quest.Reward) ([]ActorStateOp, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: no branch for reward", quest.ErrInvalidState)
	}
	a, err := AdventureActorFromState(p.State, DefaultActorID)
	if err != nil {
		return nil, err
	}
	rewards = questUnownedRewards(*system.AdventureRules, a, rewards)
	currency, experience := 0, 0
	items := map[string]int{}
	for _, r := range rewards {
		switch r.Kind {
		case quest.RewardCurrency:
			if r.RefID != system.AdventureRules.Currency {
				return nil, fmt.Errorf("%w: reward currency does not match world", quest.ErrInvalidTask)
			}
			currency += int(r.Quantity)
		case quest.RewardItem:
			items[r.RefID] += int(r.Quantity)
		case quest.RewardExperience:
			experience += int(r.Quantity)
		default:
			return nil, fmt.Errorf("%w: unsupported reward kind", quest.ErrInvalidTask)
		}
	}
	return adventureQuestOwnedRewardOps(p, system, d, currency, items, experience, d.ID)
}

func questProjectionAfter(p *storyBranchProjection, plan questPlannedAction) *storyBranchProjection {
	view := *intervalProjectionAfterOps(p, plan.Ops)
	view.Quests = cloneQuestInstances(p.Quests)
	if view.Quests == nil {
		view.Quests = map[string]quest.Instance{}
	}
	view.Quests[plan.Definition.ID] = cloneQuestInstance(plan.Instance)
	return &view
}

func applyQuestClock(p *storyBranchProjection, e QuestRecordEvent) error {
	if e.ClockBefore == nil && e.ClockAfter == nil {
		return nil
	}
	if e.Action != QuestActionChoose || e.ClockBefore == nil || e.ClockAfter == nil {
		return fmt.Errorf("invalid quest clock action")
	}
	if err := e.ClockBefore.Validate(); err != nil {
		return err
	}
	if err := e.ClockAfter.Validate(); err != nil {
		return err
	}
	if e.ClockBefore.Seconds != p.clockSeconds() {
		return fmt.Errorf("quest clock origin differs from branch")
	}
	d, ok := worldQuestDefinition(p.worldContent(), e.TaskID)
	if !ok {
		return ErrQuestUnknownTask
	}
	matched := false
	for _, result := range e.Instance.Choices {
		c, ok := d.Choice(result.ChoiceID)
		if ok && result.AtSeconds == e.ClockAfter.Seconds && c.DurationSeconds == e.ClockAfter.Seconds-e.ClockBefore.Seconds {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("quest clock does not match authored choice")
	}
	after, err := AdvanceStoryClock(*e.ClockBefore, e.ClockAfter.Seconds-e.ClockBefore.Seconds)
	if err != nil || after != *e.ClockAfter {
		return fmt.Errorf("invalid quest clock interval")
	}
	p.Clock = cloneStoryClock(e.ClockAfter)
	return nil
}

func questEchoGateOpen(p *storyBranchProjection, g *quest.EchoGate) bool {
	if p != nil && g != nil && g.UnlessTaskID != "" {
		if task, exists := p.Quests[g.UnlessTaskID]; exists && task.Status == quest.StatusCompleted && questHasSuccess(task) {
			return false
		}
	}

	if g == nil {
		return true
	}
	if p == nil {
		return false
	}
	i, ok := p.Quests[g.TaskID]
	if !ok {
		return false
	}
	if e := i.CompletionEcho; e != nil && e.Resolved && g.ChoiceID == "" && (g.Outcome == "either" || g.Outcome == "success") && (!g.RequiresAcknowledged || e.AcknowledgedBy != "") {
		return true
	}
	for _, r := range i.Choices {
		if r.EchoResolved && (!g.RequiresAcknowledged || !r.EchoEncounterRequired || r.EchoAcknowledgedBy != "") && (g.ChoiceID == "" || g.ChoiceID == r.ChoiceID) && (g.Outcome == "either" || g.Outcome == r.Outcome) {
			return true
		}
	}
	return false
}

func questConsequences(d quest.Definition, i quest.Instance) []string {
	var messages []string
	for _, c := range d.Choices {
		r, ok := i.Choices[c.ObjectiveID]
		if !ok || r.ChoiceID != c.ID {
			continue
		}
		message := c.Success
		if r.Outcome == "failure" {
			message = c.Failure
		}
		messages = append(messages, message)
		if r.EchoResolved && (!r.EchoEncounterRequired || c.Reaction(r.Outcome) == nil || r.EchoAcknowledgedBy != "") {
			message = c.SuccessEcho
			if r.Outcome == "failure" {
				message = c.FailureEcho
			}
			messages = append(messages, message)
		}
	}
	if d.CompletionEcho != nil && i.CompletionEcho != nil && i.CompletionEcho.AcknowledgedBy != "" {
		messages = append(messages, d.CompletionEcho.Reaction.Statement)
	}
	return messages
}

// Multiple legitimate sources may offer the same unique relic. An owned relic
// is not duplicated and does not invalidate the other agreed remuneration.
func questUnownedRewards(r AdventureRules, a AdventureActor, rewards []quest.Reward) []quest.Reward {
	unique := map[string]bool{}
	for _, item := range r.Items {
		unique[item.ID] = item.Unique
	}
	seen := map[string]bool{}
	var result []quest.Reward
	for _, reward := range rewards {
		if reward.Kind == quest.RewardItem && unique[reward.RefID] {
			if a.Acquired[reward.RefID] || a.Inventory[reward.RefID] > 0 || seen[reward.RefID] {
				continue
			}
			seen[reward.RefID] = true
			reward.Quantity = 1
		}
		result = append(result, reward)
	}
	return result
}
