package history

// VisibleWorld is a knowledge projection, not an alternative truth store.
// Delivered events retain their original turn; current visible entities always
// override historical claims when answering a question about today's situation.
func VisibleWorld(w World, audience string) World {
	view := World{Entities: []Entity{}, Accounts: []Account{}, Orders: []Order{}, Events: []WorldEvent{}, Ending: w.Ending, Achieved: w.Achieved}
	for _, e := range w.Entities {
		if visibleTo(e.Audience, audience) {
			view.Entities = append(view.Entities, e)
		}
	}
	for _, a := range w.Accounts {
		if visibleTo(a.Audience, audience) {
			view.Accounts = append(view.Accounts, a)
		}
	}
	for _, o := range w.Orders {
		if audience == "player" || o.MinisterID == audience {
			view.Orders = append(view.Orders, o)
		}
	}
	for _, e := range w.Events {
		if visibleTo(e.Audience, audience) {
			view.Events = append(view.Events, e)
		}
	}
	return view
}

func visibleTo(audience []string, id string) bool {
	return contains(audience, "public") || contains(audience, id)
}

// CouncilWorld retains the speaker's official accounts and assigned work.
// Other participants learn new information only through the recorded replies;
// private transcripts and unshared narrative events are not injected.
func CouncilWorld(w World, audience []string, speakerID string) World {
	view := w
	view.Events = []WorldEvent{}
	view.Accounts = []Account{}
	view.Entities = []Entity{}
	view.Orders = []Order{}
	shared := func(ids []string) bool {
		for _, id := range audience {
			if !visibleTo(ids, id) {
				return false
			}
		}
		return true
	}
	for _, e := range w.Events {
		if shared(e.Audience) {
			view.Events = append(view.Events, e)
		}
	}
	for _, a := range w.Accounts {
		owner := entityByID(&w, a.Owner)
		official := owner != nil && owner.Kind != "person"
		if shared(a.Audience) || (official && visibleTo(a.Audience, speakerID)) {
			view.Accounts = append(view.Accounts, a)
		}
	}
	for _, e := range w.Entities {
		if shared(e.Audience) || (e.Kind == "disposition" && visibleTo(e.Audience, speakerID)) {
			view.Entities = append(view.Entities, e)
		}
	}
	for _, order := range w.Orders {
		if order.MinisterID == speakerID && order.Disclosure != "sealed" {
			view.Orders = append(view.Orders, order)
		}
	}
	return view
}

// PlayerState must be used at every app/HTTP response boundary, including
// duplicate command receipts and checkpoint restores.
func PlayerState(s State) State {
	if s.World == nil {
		return s
	}
	s = cloneState(s)
	v := VisibleWorld(*s.World, "player")
	s.World = &v
	if s.Scenario != nil {
		s.Scenario.Initial = VisibleWorld(s.Scenario.Initial, "player")
		s.Scenario.Rules = ""
		for i := range s.Scenario.Ministers {
			s.Scenario.Ministers[i].Capacity = 0
			s.Scenario.Ministers[i].Knowledge = nil
		}
	}
	for i := range s.Ministers {
		s.Ministers[i].Capacity = 0
		s.Ministers[i].Knowledge = nil
	}
	for i := range s.Reports {
		if s.Reports[i].World != nil {
			s.Reports[i].World = VisibleProposal(*s.Reports[i].World, v)
		}
	}
	return s
}

func VisibleProposal(p WorldProposal, view World) *WorldProposal {
	out := &WorldProposal{Results: p.Results, Transfers: []Transfer{}, Changes: []EntityChange{}, NewEntities: []Entity{}, Events: []WorldEvent{}}
	for _, a := range p.NewAccounts {
		if visibleTo(a.Audience, "player") {
			out.NewAccounts = append(out.NewAccounts, a)
		}
	}
	for _, t := range p.Transfers {
		fromKnown := t.From != "" && accountByID(&view, t.From) != nil
		toKnown := t.To != "" && accountByID(&view, t.To) != nil
		if !fromKnown && !toKnown {
			continue
		}
		if (t.From != "" && !fromKnown) || (t.To != "" && !toKnown) {
			// Keep the visible account's real cash flow, but do not disclose the
			// hidden counterparty or a reason that may identify its private source.
			t.CounterpartyHidden = true
			t.Reason = ""
			if !fromKnown {
				t.From = ""
			}
			if !toKnown {
				t.To = ""
			}
		}
		out.Transfers = append(out.Transfers, t)
	}
	for _, c := range p.Changes {
		if entityByID(&view, c.ID) != nil {
			out.Changes = append(out.Changes, c)
		}
	}
	for _, e := range p.NewEntities {
		if visibleTo(e.Audience, "player") {
			out.NewEntities = append(out.NewEntities, e)
		}
	}
	for _, e := range p.Events {
		if visibleTo(e.Audience, "player") {
			out.Events = append(out.Events, e)
		}
	}
	return out
}

// ContextWorld builds bounded model context without deleting the raw archive.
// Every active order, prerequisite and unresolved crisis remains available.
func ContextWorld(source World) WorldContext {
	w := source
	w.Events = []WorldEvent{}
	w.Orders = []Order{}
	for i, e := range source.Events {
		if e.Status == "open" || i >= len(source.Events)-24 {
			w.Events = append(w.Events, e)
		}
	}
	for i, o := range source.Orders {
		if orderActive(o.Status) || o.Status == "paused" || i >= len(source.Orders)-24 {
			w.Orders = append(w.Orders, o)
		}
	}
	needed := map[string]bool{}
	for _, o := range w.Orders {
		needed[o.ID] = true
	}
	for changed := true; changed; {
		changed = false
		for _, o := range source.Orders {
			if needed[o.ID] {
				for _, id := range o.DependsOn {
					if !needed[id] {
						needed[id] = true
						changed = true
					}
				}
			}
		}
	}
	w.Orders = []Order{}
	for _, o := range source.Orders {
		if needed[o.ID] {
			w.Orders = append(w.Orders, o)
		}
	}
	return archiveWorldContext(w)
}

func SimulationFacts(s State) any {
	w := ContextWorld(*s.World)
	return struct {
		PlayerEntityID string               `json:"player_entity_id"`
		Year           int                  `json:"year"`
		Quarter        int                  `json:"quarter"`
		Turn           int                  `json:"turn"`
		Rules          string               `json:"rules"`
		Metrics        []MetricDefinition   `json:"metric_definitions"`
		Resources      []ResourceDefinition `json:"resource_definitions"`
		World          WorldContext         `json:"world"`
		Objectives     []Objective          `json:"objectives"`
	}{playerFinancialOwner(*s.Scenario), s.Year, s.Quarter, s.Turn, s.Scenario.Rules, s.Scenario.Metrics, s.Scenario.Resources, w, s.Scenario.Objectives}
}
