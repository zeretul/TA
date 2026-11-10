package quest

import "fmt"

// Choice is an authored way to address one objective, not a mandatory menu.
// The story may continue freely; only an explicit attempt settles its costs,
// qualification check and outcome. The backend never accepts a claimed result.
type Choice struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	ObjectiveID       string         `json:"objective_id"`
	LocationIDs       []string       `json:"location_ids,omitempty"`
	AfterObjectives   []string       `json:"after_objectives,omitempty"`
	RequiredTaskCount int            `json:"required_task_count,omitempty"`
	RequiredTasks     []string       `json:"required_tasks,omitempty"`
	RequiredItems     map[string]int `json:"required_items,omitempty"`
	RequiredRank      int            `json:"required_rank,omitempty"`
	RequiredSkill     string         `json:"required_skill,omitempty"`
	RequiredSkillRank int            `json:"required_skill_rank,omitempty"`
	RequiredJourney   string         `json:"required_journey,omitempty"`
	RequiredVictory   string         `json:"required_victory,omitempty"`
	// Sources whose actual present participation is part of this action.
	// Empty preserves observation and independent work without summoned NPCs.
	RequiredCharacterSourceKeys []string `json:"required_character_source_keys,omitempty"`
	// UseSkillID charges the frozen effort of a learned non-combat method.
	// AssetFuelCost is one bounded use of the already required working asset.
	UseSkillID    string         `json:"use_skill_id,omitempty"`
	AssetFuelCost int            `json:"asset_fuel_cost,omitempty"`
	RequiredAsset string         `json:"required_asset,omitempty"`
	SkillID       string         `json:"skill_id,omitempty"`
	Difficulty    int            `json:"difficulty,omitempty"`
	Debt          *DebtTerms     `json:"debt,omitempty"`
	CurrencyCost  int            `json:"currency_cost,omitempty"`
	ItemCosts     map[string]int `json:"item_costs,omitempty"`
	// ItemHandovers move an explicit subset of these costs to actual recipients
	// on success; a failed attempt retains these objects and consumes other costs.
	ItemHandovers    []ItemHandover `json:"item_handovers,omitempty"`
	DurationSeconds  int64          `json:"duration_seconds"`
	Rewards          []Reward       `json:"rewards,omitempty"`
	Success          string         `json:"success"`
	Failure          string         `json:"failure"`
	EchoAfterSeconds int64          `json:"echo_after_seconds"`
	SuccessEcho      string         `json:"success_echo"`
	FailureEcho      string         `json:"failure_echo"`
	SuccessReaction  *EchoReaction  `json:"success_reaction,omitempty"`
	FailureReaction  *EchoReaction  `json:"failure_reaction,omitempty"`
}

// ItemHandover names the receiving person and the exact authored physical items.
// Participation, prose and a reward's giver never imply a receiving owner.
type ItemHandover struct {
	CharacterSourceKey string         `json:"character_source_key"`
	Items              map[string]int `json:"items"`
}

// DebtTerms settles an authored loan against its original committed receipt.
// The ledger remains the quest journal; no wall clock or secondary balance is used.
type DebtTerms struct {
	TaskID            string `json:"task_id"`
	Principal         int    `json:"principal"`
	HourlyBasisPoints int    `json:"hourly_basis_points"`
}

// EchoGate makes a delayed consequence consumable by another opportunity.
// Outcome may be either, success or failure. ChoiceID is optional, so a region
// can continue after any handling of its earlier affair without forcing a path.
type EchoGate struct {
	RequiresAcknowledged bool   `json:"requires_acknowledged,omitempty"`
	UnlessTaskID         string `json:"unless_task_id,omitempty"`
	TaskID               string `json:"task_id"`
	ChoiceID             string `json:"choice_id,omitempty"`
	Outcome              string `json:"outcome"`
}

// ChoiceResult is canonical branch history stored inside the existing task
// instance. Reads never age it; an interval commit sets EchoResolved once.
type ChoiceResult struct {
	Capability     int           `json:"capability"`
	Previous       *ChoiceResult `json:"previous,omitempty"`
	ChoiceID       string        `json:"choice_id"`
	Outcome        string        `json:"outcome"`
	AtSeconds      int64         `json:"at_seconds"`
	EchoDueSeconds int64         `json:"echo_due_seconds"`
	// EchoEncounterRequired is stamped by new choices. Older revealed receipts keep their original knowledge boundary.
	EchoEncounterRequired     bool   `json:"echo_encounter_required,omitempty"`
	EchoResolved              bool   `json:"echo_resolved,omitempty"`
	EchoAcknowledgedBy        string `json:"echo_acknowledged_by,omitempty"`
	EchoAcknowledgedAtSeconds int64  `json:"echo_acknowledged_at_seconds,omitempty"`
}

func (d Definition) Choice(id string) (Choice, bool) {
	for _, c := range d.Choices {
		if c.ID == id {
			return c, true
		}
	}
	return Choice{}, false
}

func validateChoices(d Definition) []Diagnostic {
	var out []Diagnostic
	add := func(path, message string) {
		out = append(out, Diagnostic{Code: "invalid_choice", Path: path, Message: message})
	}
	if len(d.Choices) > MaxObjectives*4 {
		add("choices", "too many authored choices")
	}
	seen := map[string]bool{}
	for i, c := range d.Choices {
		p := fmt.Sprintf("choices[%d]", i)
		if !validID(c.ID) || seen[c.ID] || !d.HasObjective(c.ObjectiveID) {
			add(p, "choice identity or objective is invalid")
		}
		seen[c.ID] = true
		for _, v := range []string{c.Title, c.Description, c.Success, c.Failure, c.SuccessEcho, c.FailureEcho} {
			if !validText(v, MaxTextBytes) {
				add(p, "choice text is missing or too large")
			}
		}
		if c.DurationSeconds < 1 || c.DurationSeconds > MaxDeadlineSeconds || c.EchoAfterSeconds < 1 || c.EchoAfterSeconds > MaxDeadlineSeconds || c.CurrencyCost < 0 || c.CurrencyCost > MaxQuantity || c.Difficulty < 0 || c.Difficulty > 100 || c.RequiredRank < 0 || c.RequiredRank > MaxChoiceRank || c.RequiredSkillRank < 0 || c.RequiredSkillRank > MaxChoiceSkillRank {
			add(p, "choice numeric bounds are invalid")
		}
		if c.AssetFuelCost < 0 || c.AssetFuelCost > 100 || c.AssetFuelCost > 0 && c.RequiredAsset == "" {
			add(p, "asset fuel cost requires an asset and must be between zero and one hundred")
		}
		if c.RequiredSkillRank > 0 && c.RequiredSkill == "" {
			add(p, "skill rank needs a skill")
		}
		for _, id := range c.AfterObjectives {
			if id == c.ObjectiveID || !d.HasObjective(id) {
				add(p, "stage dependency is invalid")
			}
		}
		if c.RequiredTaskCount < 0 || c.RequiredTaskCount > len(c.RequiredTasks) {
			add(p, "task prerequisite count is invalid")
		}
		if len(c.RequiredCharacterSourceKeys) > 8 {
			add(p, "too many required participants")
		}
		participants := map[string]bool{}
		for _, source := range c.RequiredCharacterSourceKeys {
			if !validID(source) || participants[source] {
				add(p, "required participant is invalid or duplicated")
			}
			participants[source] = true
		}
		for _, list := range [][]string{c.LocationIDs, c.RequiredTasks} {
			for _, id := range list {
				if !validID(id) {
					add(p, "reference is invalid")
				}
			}
		}
		for _, m := range []map[string]int{c.RequiredItems, c.ItemCosts} {
			for id, n := range m {
				if !validID(id) || n < 1 || n > MaxQuantity {
					add(p, "item requirement is invalid")
				}
			}
		}
		if len(c.ItemHandovers) > 8 {
			add(p, "too many item handover recipients")
		}
		recipients := map[string]bool{}
		handed := map[string]int64{}
		for j, handover := range c.ItemHandovers {
			hp := fmt.Sprintf("%s.item_handovers[%d]", p, j)
			if !validID(handover.CharacterSourceKey) || recipients[handover.CharacterSourceKey] || len(handover.Items) < 1 || len(handover.Items) > MaxRewards {
				add(hp, "handover requires one distinct recipient and bounded items")
			}
			recipients[handover.CharacterSourceKey] = true
			for id, n := range handover.Items {
				if !validID(id) || n < 1 || n > MaxQuantity {
					add(hp, "handover item quantity is invalid")
					continue
				}
				handed[id] += int64(n)
				if handed[id] > int64(c.ItemCosts[id]) {
					add(hp, "handover quantities exceed the authored item costs")
				}
			}
		}
		for _, v := range []string{c.SkillID, c.UseSkillID, c.RequiredSkill, c.RequiredVictory, c.RequiredAsset, c.RequiredJourney} {
			if v != "" && !validID(v) {
				add(p, "qualification reference is invalid")
			}
		}
		if c.Debt != nil && (!validID(c.Debt.TaskID) || c.Debt.Principal < 1 || c.Debt.Principal > MaxQuantity || c.Debt.HourlyBasisPoints < 0 || c.Debt.HourlyBasisPoints > 100) {
			add(p, "debt terms are invalid")
		}
		for _, reaction := range []*EchoReaction{c.SuccessReaction, c.FailureReaction} {
			if reaction != nil && !reaction.Valid() {
				add(p, "echo reaction requires one character source and bounded text")
			}
		}
		for _, r := range c.Rewards {
			if r.Kind == RewardItem && handed[r.RefID] > 0 {
				add(p, "a handed-over item cannot also be a choice reward")
			}
			if r.Quantity < 1 || r.Quantity > MaxQuantity || !validText(r.RefID, MaxTextBytes) {
				add(p, "reward is invalid")
			}
			switch r.Kind {
			case RewardItem, RewardCurrency, RewardExperience:
			default:
				add(p, "reward kind is invalid")
			}
		}
	}
	if g := d.RequiresEcho; g != nil {
		if !validID(g.TaskID) || (g.ChoiceID != "" && !validID(g.ChoiceID)) || (g.Outcome != "either" && g.Outcome != "success" && g.Outcome != "failure") {
			add("requires_echo", "invalid consequence gate")
		}
	}
	return out
}
