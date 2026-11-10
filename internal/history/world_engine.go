package history

import (
	"fmt"
	"strings"
)

// PreviewWorld applies a complete AI proposal to a deep copy. This function
// neither advances the live save nor spends resources until Store.Apply commits.
func PreviewWorld(original State, plan QuarterPlan) (State, QuarterReport, error) {
	s := cloneState(original)
	report := QuarterReport{Turn: s.Turn, Year: s.Year, Quarter: s.Quarter, Narrative: plan.Narrative, World: plan.World, Outcomes: []Outcome{}}
	if s.Scenario == nil || s.World == nil || plan.World == nil || s.World.Ending != "" {
		return original, report, invalid("world proposal unavailable")
	}
	p := plan.World
	w := s.World

	results := map[string]OrderResult{}
	for _, r := range p.Results {
		o := orderByID(w, r.OrderID)
		if o == nil || !orderActive(o.Status) || results[r.OrderID].OrderID != "" || strings.TrimSpace(r.Reason) == "" || r.Progress < o.Progress || r.Progress > 100 {
			return original, report, invalid("invalid order result %s", r.OrderID)
		}
		if r.Status != "active" && r.Status != "waiting" && r.Status != "completed" {
			return original, report, invalid("invalid result status")
		}
		ready := orderReady(original.World, *o, s.Turn) && (o.Condition == "" || r.ConditionMet)
		if !ready && (r.Status != "waiting" || r.Progress != o.Progress) {
			return original, report, invalid("unmet order condition %s", o.ID)
		}
		if o.Kind == "policy" && r.Status == "completed" {
			return original, report, invalid("standing policy cannot auto complete")
		}
		if r.Status == "completed" && r.Progress != 100 {
			return original, report, invalid("completed order requires 100 progress")
		}
		results[r.OrderID] = r
	}
	for _, o := range w.Orders {
		if orderActive(o.Status) && results[o.ID].OrderID == "" {
			return original, report, invalid("missing active order result %s", o.ID)
		}
	}
	for _, e := range p.NewEntities {
		for key, value := range e.Attributes {
			if strings.TrimSpace(value.ZH) == "" || strings.TrimSpace(value.EN) == "" {
				return original, report, invalid("new entity %s attribute %s requires both zh and en display text", e.ID, key)
			}
		}
		if entityByID(w, e.ID) != nil {
			return original, report, invalid("new entity already exists")
		}
		w.Entities = append(w.Entities, e)
	}
	for _, a := range p.NewAccounts {
		if a.Balance != 0 || accountByID(w, a.ID) != nil || resourceDef(*s.Scenario, a.Resource) == nil || entityByID(w, a.Owner) == nil {
			return original, report, invalid("new account must be unique, owned and empty")
		}
		w.Accounts = append(w.Accounts, a)
	}
	for index, t := range p.Transfers {
		r := resourceDef(*s.Scenario, t.Resource)
		if t.CounterpartyHidden || r == nil || t.Amount <= 0 || t.Amount > r.MaxTransaction || t.From == t.To || strings.TrimSpace(t.Reason) == "" {
			return original, report, invalid("invalid ledger transfer")
		}
		from, to := accountByID(w, t.From), accountByID(w, t.To)
		if (t.From != "" && (from == nil || from.Resource != t.Resource)) || (t.To != "" && (to == nil || to.Resource != t.Resource)) {
			return original, report, invalid("transfer %d account/resource mismatch: from=%q to=%q resource=%q; both accounts must exist and hold this resource", index, t.From, t.To, t.Resource)
		}
		if t.OrderID != "" {
			o := orderByID(w, t.OrderID)
			res, ok := results[t.OrderID]
			if o == nil || !ok || !orderReady(original.World, *o, s.Turn) || res.Status == "waiting" || (o.Condition != "" && !res.ConditionMet) {
				return original, report, invalid("inactive order expenditure")
			}
			if orderTransferSpends(w, playerFinancialOwner(*s.Scenario), from, to) {
				next := o.Spent[t.Resource] + t.Amount
				if cap, has := o.Limits[t.Resource]; has && next > cap {
					return original, report, invalid("order budget exceeded")
				}
				if o.Spent == nil {
					o.Spent = map[string]int64{}
				}
				o.Spent[t.Resource] = next
			}
		}
		if from != nil {
			if from.Balance < t.Amount {
				return original, report, invalid("insufficient resources in %s", from.ID)
			}
			from.Balance -= t.Amount
		}
		if to != nil {
			if to.Balance > 1000000000000-t.Amount {
				return original, report, invalid("account balance overflow")
			}
			to.Balance += t.Amount
		}
	}

	seen := map[string]bool{}
	for _, c := range p.Changes {
		e := entityByID(w, c.ID)
		if e == nil || seen[c.ID] || strings.TrimSpace(c.Reason) == "" {
			return original, report, invalid("invalid entity change")
		}
		seen[c.ID] = true
		if e.Metrics == nil {
			e.Metrics = map[string]int64{}
		}
		for key, delta := range c.MetricDeltas {
			d := metricDef(*s.Scenario, key)
			if d == nil || delta < -1000000000000 || delta > 1000000000000 || e.Metrics[key]+delta < d.Min || e.Metrics[key]+delta > d.Max {
				return original, report, invalid("metric change out of range %s.%s", e.ID, key)
			}
			e.Metrics[key] += delta
		}
		if e.Attributes == nil {
			e.Attributes = map[string]Text{}
		}
		for key, v := range c.Attributes {
			if !safeID.MatchString(key) {
				return original, report, invalid("invalid attribute key")
			}
			if strings.TrimSpace(v.ZH) == "" || strings.TrimSpace(v.EN) == "" {
				return original, report, invalid("entity %s attribute %s requires both zh and en display text", e.ID, key)
			}
			e.Attributes[key] = v
		}
		if c.Status != "" {
			e.Status = c.Status
		}
		if c.Location != nil {
			e.Location = *c.Location
		}
		if c.Owner != nil {
			e.Owner = *c.Owner
		}
	}
	for _, r := range p.Results {
		o := orderByID(w, r.OrderID)
		o.Status = r.Status
		o.Progress = r.Progress
		o.LastResult = r.Reason
		report.Outcomes = append(report.Outcomes, Outcome{ProjectID: o.ID, Status: r.Status, Reason: r.Reason, Progress: r.Progress})
	}
	for i, event := range p.Events {
		if strings.TrimSpace(event.Title) == "" || strings.TrimSpace(event.Text) == "" || len(event.Text) > 12000 || (event.Status != "open" && event.Status != "resolved") {
			return original, report, invalid("invalid world event")
		}
		for _, id := range event.EntityIDs {
			if entityByID(w, id) == nil {
				return original, report, invalid("event %d references missing entity %q; entity_ids must reference entities, not accounts or display names", i, id)
			}
		}
		for _, id := range event.OrderIDs {
			if orderByID(w, id) == nil {
				return original, report, invalid("event %d references missing order %q", i, id)
			}
			if order := orderByID(w, id); order.Disclosure == "sealed" {
				for _, recipient := range event.Audience {
					if recipient != "player" && recipient != order.MinisterID {
						return original, report, invalid("event %d links sealed order %q but recipient %q is not authorized; keep sealed results private and report separately authorized summaries as separate events", i, id, recipient)
					}
				}
			}
		}
		if event.ID != "" {
			found := -1
			for j, old := range w.Events {
				if old.ID == event.ID {
					found = j
					break
				}
			}
			if found < 0 {
				return original, report, invalid("event update references missing event")
			}
			event.Turn = s.Turn
			w.Events[found] = event
			continue
		}
		event.ID = shortID("event-", fmt.Sprintf("%s:%d:%d:%d", s.ID, s.Revision, s.Turn, i))
		event.Turn = s.Turn
		w.Events = append(w.Events, event)
	}
	for _, g := range s.Scenario.Objectives {
		e := entityByID(w, g.EntityID)
		if e == nil {
			continue
		}
		v := e.Metrics[g.Metric]
		met := (g.Operator == "gte" && v >= g.Value) || (g.Operator == "lte" && v <= g.Value)
		if met && !contains(w.Achieved, g.ID) {
			w.Achieved = append(w.Achieved, g.ID)
			if g.Ending != "" {
				w.Ending = g.Ending
			}
		}
	}
	s.Turn++
	s.Quarter++
	if s.Quarter > 4 {
		s.Quarter = 1
		s.Year++
	}
	syncWorldMinisters(&s)
	if err := validateWorld(*s.Scenario, *w); err != nil {
		return original, report, err
	}
	return s, report, nil
}

func syncWorldMinisters(s *State) {
	old := map[string]Minister{}
	for _, m := range s.Ministers {
		old[m.ID] = m
	}
	s.Ministers = []Minister{}
	for _, e := range s.World.Entities {
		if e.Kind != "person" || e.Status != "active" {
			continue
		}
		office, ok := e.Attributes["office"]
		if !ok {
			continue
		}
		m := old[e.ID]
		m.ID = e.ID
		m.Name = e.Name.ZH
		m.NameEN = e.Name.EN
		m.Office = office.ZH
		m.OfficeEN = office.EN
		if bio, ok := e.Attributes["profile"]; ok {
			m.Profile = bio.ZH
		}
		s.Ministers = append(s.Ministers, m)
	}
}
