package history

// WorldContext is a read projection. Archived rows remain complete and can be
// retrieved by the bounded model reader; the canonical World is never pruned.
type WorldContext struct {
	World
	ArchivedEntities []Entity  `json:"archived_entities,omitempty"`
	ArchivedAccounts []Account `json:"archived_accounts,omitempty"`
}

func archiveWorldContext(w World) WorldContext {
	keep := map[string]bool{}
	for _, e := range w.Entities {
		switch e.Status {
		case "dead", "retired", "destroyed", "closed":
		default:
			keep[e.ID] = true
		}
	}
	for _, o := range w.Orders {
		if orderActive(o.Status) || o.Status == "paused" {
			keep[o.MinisterID] = true
			for _, id := range o.Targets {
				keep[id] = true
			}
		}
	}
	for _, e := range w.Events {
		if e.Status == "open" {
			for _, id := range e.EntityIDs {
				keep[id] = true
			}
		}
	}
	for _, a := range w.Accounts {
		if a.Balance != 0 {
			keep[a.Owner] = true
		}
	}
	// A retained row's ownership/location is itself a current fact dependency.
	for changed := true; changed; {
		changed = false
		for _, e := range w.Entities {
			if keep[e.ID] {
				for _, id := range []string{e.Owner, e.Location} {
					if id != "" && !keep[id] {
						keep[id] = true
						changed = true
					}
				}
			}
		}
	}
	view := WorldContext{World: w}
	view.Entities = []Entity{}
	view.Accounts = []Account{}
	for _, e := range w.Entities {
		if keep[e.ID] {
			view.Entities = append(view.Entities, e)
		} else {
			view.ArchivedEntities = append(view.ArchivedEntities, e)
		}
	}
	for _, a := range w.Accounts {
		if a.Balance != 0 || keep[a.Owner] {
			view.Accounts = append(view.Accounts, a)
		} else {
			view.ArchivedAccounts = append(view.ArchivedAccounts, a)
		}
	}
	return view
}
