package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}
func cloneState(s State) State {
	b, _ := json.Marshal(s)
	var c State
	_ = json.Unmarshal(b, &c)
	return c
}
func shortID(prefix, input string) string {
	h := sha256.Sum256([]byte(input))
	return prefix + hex.EncodeToString(h[:12])
}
func minister(s State, id string) *Minister {
	for i := range s.Ministers {
		if s.Ministers[i].ID == id {
			return &s.Ministers[i]
		}
	}
	return nil
}
func region(s *State, id string) *Region {
	for i := range s.Regions {
		if s.Regions[i].ID == id {
			return &s.Regions[i]
		}
	}
	return nil
}
func project(s *State, id string) *Project {
	for i := range s.Projects {
		if s.Projects[i].ID == id {
			return &s.Projects[i]
		}
	}
	return nil
}
func isWork(kind string) bool {
	return kind == "relief" || kind == "training" || kind == "construction"
}
func clamp(value, lo, hi int) int { return min(hi, max(lo, value)) }

// ValidatePlan gives every intent its own result. Unknown or unsupported
// instructions remain visible and never claim another instruction's success.
func ValidatePlan(s State, plan EdictPlan) []Outcome {
	if s.World != nil {
		return validateWorldOrders(s, plan)
	}
	out := make([]Outcome, len(plan.Instructions))
	counts := map[string]int{}
	byID := map[string]Instruction{}
	for _, in := range plan.Instructions {
		counts[in.ID]++
		byID[in.ID] = in
	}
	checked := map[string]bool{}
	var cyclic func(string, map[string]bool) bool
	cyclic = func(id string, path map[string]bool) bool {
		if path[id] {
			return true
		}
		in, ok := byID[id]
		if !ok || checked[id] {
			return false
		}
		next := map[string]bool{}
		for k, v := range path {
			next[k] = v
		}
		next[id] = true
		for _, dep := range in.DependsOn {
			if cyclic(dep, next) {
				return true
			}
		}
		checked[id] = true
		return false
	}
	for i, in := range plan.Instructions {
		o := Outcome{InstructionID: in.ID, Status: "ready", Reason: "validated"}
		invalid := func(reason string) { o.Status = "unsupported"; o.Reason = reason }
		switch {
		case strings.TrimSpace(in.ID) == "" || len(in.ID) > 80 || counts[in.ID] != 1 || project(&s, in.ID) != nil:
			invalid("instruction_id_missing_or_duplicate")
		case in.Budget < 0 || in.Budget > 1000000000 || in.Grain < 0 || in.Grain > 1000000000:
			invalid("amount_out_of_bounds")
		case in.NotBeforeTurn < 0 || in.NotBeforeTurn > s.Turn+40:
			invalid("not_before_turn_out_of_bounds")
		default:
			switch in.Kind {
			case "relief", "training", "construction":
				if region(&s, in.Target) == nil {
					invalid("unknown_region")
				} else if minister(s, in.MinisterID) == nil {
					invalid("unknown_minister")
				} else if in.Budget <= 0 {
					invalid("positive_budget_required")
				} else if in.Quarters < 1 || in.Quarters > 20 {
					invalid("quarters_out_of_bounds")
				} else if in.Kind == "relief" && in.Grain <= 0 {
					invalid("relief_requires_grain")
				}
			case "pause", "resume", "reassign", "add_funds":
				p := project(&s, in.ProjectID)
				if p == nil {
					invalid("unknown_project")
				} else if p.Status == "completed" {
					invalid("project_already_completed")
				} else if in.Kind == "reassign" && minister(s, in.MinisterID) == nil {
					invalid("unknown_minister")
				} else if in.Kind == "add_funds" && in.Budget == 0 && in.Grain == 0 {
					invalid("positive_funding_required")
				} else if in.NotBeforeTurn > s.Turn || len(in.DependsOn) > 0 {
					invalid("conditional_amendment_requires_clarification")
				}
			case "unsupported":
				invalid(in.Reason)
				if o.Reason == "" {
					o.Reason = "instruction_requires_clarification"
				}
			default:
				invalid("unsupported_instruction_kind")
			}
		}
		if o.Status == "ready" {
			for _, dep := range in.DependsOn {
				other, ok := byID[dep]
				if (!ok || !isWork(other.Kind)) && project(&s, dep) == nil {
					invalid("unknown_dependency")
					break
				}
			}
			if cyclic(in.ID, map[string]bool{}) {
				invalid("dependency_cycle")
			}
		}
		out[i] = o
	}
	// An invalid intent is not a dependency that can eventually complete.
	for changed := true; changed; {
		changed = false
		for i, in := range plan.Instructions {
			if out[i].Status != "ready" {
				continue
			}
			for _, dep := range in.DependsOn {
				for j, other := range plan.Instructions {
					if other.ID == dep && out[j].Status != "ready" {
						out[i].Status = "unsupported"
						out[i].Reason = "unsupported_dependency"
						changed = true
					}
				}
			}
		}
	}
	return out
}

func applyEdict(s *State, id string, plan EdictPlan) ([]Outcome, error) {
	if s.World == nil && len(plan.Orders) > 0 {
		return nil, invalid("scenario orders require a bound world")
	}
	// A discussion-derived decree retains its exact canonical source. A restored
	// away conversation remains in the audit archive but cannot authorize new work.
	if plan.SourceAdviceID != "" {
		found := false
		for _, advice := range s.Advice {
			if advice.ID == plan.SourceAdviceID && advice.ChatID == plan.SourceChatID {
				found = true
				break
			}
		}
		if !found {
			return nil, invalid("saved advice source is unavailable")
		}
	}
	if plan.SourceChatID != "" {
		available := false
		for _, chat := range s.Chats {
			if chat.ID == plan.SourceChatID && !discarded(*s, chat.Revision) {
				available = true
				break
			}
		}
		if !available {
			return nil, invalid("history source chat is not available in the current branch")
		}
	}
	if strings.TrimSpace(plan.Text) == "" || utf8.RuneCountInString(plan.Text) > 12000 || (s.World == nil && (len(plan.Instructions) == 0 || len(plan.Instructions) > 32)) {
		return nil, invalid("history edict requires text and 1 to 32 instructions")
	}
	if s.World != nil {
		return applyWorldEdict(s, id, plan)
	}
	out := ValidatePlan(*s, plan)
	mapping := map[string]string{}
	for i, in := range plan.Instructions {
		if out[i].Status == "ready" && isWork(in.Kind) {
			mapping[in.ID] = shortID("project-", id+"/"+in.ID)
		}
	}
	for i, in := range plan.Instructions {
		if out[i].Status != "ready" {
			continue
		}
		if isWork(in.Kind) {
			deps := []string{}
			for _, dep := range in.DependsOn {
				if resolved, ok := mapping[dep]; ok {
					deps = append(deps, resolved)
				} else {
					deps = append(deps, dep)
				}
			}
			p := Project{ID: mapping[in.ID], InstructionID: in.ID, EdictID: id, Kind: in.Kind, Target: in.Target, MinisterID: in.MinisterID, Budget: in.Budget, Grain: in.Grain, Quarters: in.Quarters, Required: in.Quarters * 100, NotBeforeTurn: in.NotBeforeTurn, DependsOn: deps, Status: "pending", Reason: "awaiting_quarter"}
			s.Projects = append(s.Projects, p)
			out[i].ProjectID = p.ID
			out[i].Status = "pending"
			out[i].Reason = "awaiting_quarter"
			continue
		}
		p := project(s, in.ProjectID)
		out[i].ProjectID = p.ID
		// Amendments are applied in the order the player confirmed them.
		switch in.Kind {
		case "pause":
			p.Status = "paused"
			p.Reason = "paused_by_edict"
		case "resume":
			p.Status = "pending"
			if p.Reserved {
				p.Status = "active"
			}
			p.Reason = "resumed_by_edict"
		case "reassign":
			p.MinisterID = in.MinisterID
			p.Reason = "reassigned_by_edict"
		case "add_funds":
			if p.Budget+in.Budget > 1000000000 || p.Grain+in.Grain > 1000000000 {
				out[i].Status = "unsupported"
				out[i].Reason = "total_funding_out_of_bounds"
				continue
			}
			if p.Reserved {
				if s.Treasury < in.Budget || s.Grain < in.Grain {
					out[i].Status = "blocked"
					out[i].Reason = "insufficient_funding"
					continue
				}
				s.Treasury -= in.Budget
				s.Grain -= in.Grain
			}
			p.Budget += in.Budget
			p.Grain += in.Grain
			p.Reason = "funding_added"
		}
		out[i].Status = "applied"
		out[i].Reason = p.Reason
	}
	s.Edicts = append(s.Edicts, Edict{ID: id, Turn: s.Turn, Plan: plan, Outcomes: out})
	return out, nil
}

// PreviewAdvance is pure: one complete quarter is calculated against a copy.
// A caller can ask a model to explain the returned report before committing the
// same bounded plan with the original revision. No partial quarter is saved.
func PreviewAdvance(original State, plan QuarterPlan) (State, QuarterReport, error) {
	if original.World != nil {
		return PreviewWorld(original, plan)
	}
	s := cloneState(original)
	report := QuarterReport{AppliedAdjustments: []ProjectAdjustment{}, Turn: s.Turn, Year: s.Year, Quarter: s.Quarter, OpeningTreasury: s.Treasury, Outcomes: []Outcome{}, Events: plan.Events, Narrative: plan.Narrative}
	if len(plan.Narrative) > 60000 {
		return State{}, report, invalid("history quarter narrative exceeds limit")
	}
	adjustments := map[string]ProjectAdjustment{}
	for _, a := range plan.Adjustments {
		if project(&s, a.ProjectID) == nil || a.EfficiencyPct < 75 || a.EfficiencyPct > 125 || strings.TrimSpace(a.Reason) == "" {
			return State{}, report, invalid("history invalid project adjustment")
		}
		if _, ok := adjustments[a.ProjectID]; ok {
			return State{}, report, invalid("history duplicate project adjustment")
		}
		adjustments[a.ProjectID] = a
	}
	eventSeen := map[string]bool{}
	for _, e := range plan.Events {
		r := region(&s, e.RegionID)
		if r == nil || e.SeverityDelta < -3 || e.SeverityDelta > 3 || eventSeen[e.RegionID] || strings.TrimSpace(e.Reason) == "" {
			return State{}, report, invalid("history invalid regional event")
		}
		eventSeen[e.RegionID] = true
		r.Disaster = clamp(r.Disaster+e.SeverityDelta, 0, 100)
	}
	// Only imported regions contribute income. Upkeep is an explicit slice rule.
	for _, r := range s.Regions {
		report.Income += r.TaxBase * int64(100-r.Disaster) / 400
	}
	s.Treasury += report.Income
	report.Upkeep = min(s.Treasury, int64(180000))
	report.UpkeepArrears = 180000 - report.Upkeep
	s.Treasury -= report.Upkeep
	completed := map[string]bool{}
	for _, p := range original.Projects {
		completed[p.ID] = p.Status == "completed"
	}
	capacity := map[string]int64{}
	for _, m := range s.Ministers {
		capacity[m.ID] = m.Capacity
	}
	for i := range s.Projects {
		p := &s.Projects[i]
		if p.Status == "completed" {
			continue
		}
		o := Outcome{InstructionID: p.InstructionID, ProjectID: p.ID, Status: p.Status, Reason: p.Reason, Progress: p.Progress}
		blocked := ""
		if p.Status == "paused" {
			blocked = "paused_by_edict"
		} else if p.NotBeforeTurn > s.Turn {
			blocked = "not_before_turn"
		} else {
			for _, dep := range p.DependsOn {
				if !completed[dep] {
					blocked = "dependency_incomplete"
					break
				}
			}
		}
		if blocked != "" {
			o.Reason = blocked
			if p.Status != "paused" {
				p.Status = "pending"
				o.Status = "pending"
			}
			p.Reason = blocked
			report.Outcomes = append(report.Outcomes, o)
			continue
		}
		adjustment, hasAdjustment := adjustments[p.ID]
		eff := 100
		if hasAdjustment {
			eff = adjustment.EfficiencyPct
		}
		if !p.Reserved && (s.Treasury < p.Budget || s.Grain < p.Grain) {
			p.Status = "blocked"
			p.Reason = "insufficient_funding"
			o.Status = p.Status
			o.Reason = p.Reason
			report.Outcomes = append(report.Outcomes, o)
			continue
		}
		if hasAdjustment && adjustment.Delay {
			p.Status = "pending"
			if p.Reserved {
				p.Status = "active"
			}
			p.Reason = "model_logistics_delay"
			o.Reason = p.Reason
			o.Status = "delayed"
			report.AppliedAdjustments = append(report.AppliedAdjustments, adjustment)
			report.Outcomes = append(report.Outcomes, o)
			continue
		}
		if !p.Reserved {
			s.Treasury -= p.Budget
			s.Grain -= p.Grain
			report.Reserved += p.Budget
			report.GrainReserved += p.Grain
			p.Reserved = true
		}
		work := min(eff, p.Required-p.Progress)
		remainingWork := p.Required - p.Progress
		remainingBudget := p.Budget - p.Spent
		if remainingBudget > 0 {
			work = min(work, int(capacity[p.MinisterID]*int64(remainingWork)/remainingBudget))
		}
		if work == 0 {
			p.Status = "active"
			p.Reason = "minister_capacity_exhausted"
			o.Status = p.Status
			o.Reason = p.Reason
			report.Outcomes = append(report.Outcomes, o)
			continue
		}
		if hasAdjustment {
			report.AppliedAdjustments = append(report.AppliedAdjustments, adjustment)
		}
		oldSpent, oldGrainSpent := p.Spent, p.GrainSpent
		p.Progress += work
		newSpent := p.Spent + (remainingBudget*int64(work)+int64(remainingWork)-1)/int64(remainingWork)
		newGrain := p.GrainSpent + ((p.Grain-p.GrainSpent)*int64(work)+int64(remainingWork)-1)/int64(remainingWork)
		o.Spent = newSpent - p.Spent
		o.GrainSpent = newGrain - p.GrainSpent
		p.Spent = newSpent
		p.GrainSpent = newGrain
		capacity[p.MinisterID] -= o.Spent
		report.Spent += o.Spent
		report.GrainSpent += o.GrainSpent
		p.Status = "active"
		p.Reason = "work_progressed"
		r := region(&s, p.Target)
		if p.Kind == "relief" {
			r.Disaster = max(0, r.Disaster-int(newGrain/2000-oldGrainSpent/2000))
			r.Refugees = max(int64(0), r.Refugees-(newGrain/10-oldGrainSpent/10))
			r.Heart = clamp(r.Heart+int(newSpent/20000-oldSpent/20000), 0, 100)
		}
		if p.Kind == "training" {
			r.Training = clamp(r.Training+int(newSpent/5000-oldSpent/5000), 0, 100)
		}
		if p.Progress == p.Required {
			p.Status = "completed"
			p.Reason = "work_completed"
			if p.Kind == "construction" {
				improvement := min(10, int(p.Budget/20000))
				r.Irrigation = clamp(r.Irrigation+improvement, 0, 100)
				r.Disaster = max(0, r.Disaster-improvement/2)
				r.TaxBase += p.Budget / 10
			}
		}
		o.Progress = p.Progress
		o.Status = p.Status
		o.Reason = p.Reason
		report.Outcomes = append(report.Outcomes, o)
	}
	for i := range s.Regions {
		r := &s.Regions[i]
		if r.Disaster > 0 {
			r.Refugees += int64(r.Disaster * 30)
			r.Heart = max(0, r.Heart-max(1, r.Disaster/20))
			if !eventSeen[r.ID] {
				r.Disaster = clamp(r.Disaster+max(0, 2-r.Irrigation/10), 0, 100)
			}
		}
		if report.UpkeepArrears > 0 {
			r.Heart = max(0, r.Heart-2)
		}
	}
	report.ClosingTreasury = s.Treasury
	s.Turn++
	s.Quarter++
	if s.Quarter > 4 {
		s.Quarter = 1
		s.Year++
	}
	if err := validateState(s); err != nil {
		return State{}, report, err
	}
	return s, report, nil
}

func validateState(s State) error {
	if s.Quarter < 1 || s.Quarter > 4 || s.Turn < 0 {
		return invalid("invalid world clock")
	}
	if s.World != nil {
		if s.Scenario == nil || digest(*s.Scenario) != s.ScenarioDigest {
			return invalid("frozen scenario digest mismatch")
		}
		return validateWorld(*s.Scenario, *s.World)
	}
	if s.Treasury < 0 || s.PrivyPurse < 0 || s.Grain < 0 || s.Quarter < 1 || s.Quarter > 4 {
		return invalid("history invalid resource or clock state")
	}
	for _, p := range s.Projects {
		if p.Budget < 0 || p.Grain < 0 || p.Spent < 0 || p.Spent > p.Budget || p.GrainSpent < 0 || p.GrainSpent > p.Grain || p.Progress < 0 || p.Progress > p.Required {
			return invalid("history invalid project accounting")
		}
	}
	return nil
}
