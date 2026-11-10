package history

// Internal public transfers retain funds within the same treasury. Payments
// to a person or another polity consume authorization even when both accounts
// are tracked. Personal assets remain separate from their owner's allegiance.
func orderTransferSpends(w *World, playerID string, from, to *Account) bool {
	if from == nil {
		return false
	}
	source := financialOwner(w, from.Owner)
	if source == nil || source.ID != playerID {
		return false
	}
	if to == nil {
		return true
	}
	target := financialOwner(w, to.Owner)
	return target == nil || source.ID != target.ID
}

func financialOwner(w *World, id string) *Entity {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		entity := entityByID(w, id)
		if entity == nil {
			return nil
		}
		if entity.Kind == "state" || entity.Kind == "person" || entity.Owner == "" {
			return entity
		}
		id = entity.Owner
	}
	return nil
}

// Older frozen packs used their first treasury account as the player realm.
// New packs declare the realm explicitly; existing journals are not rewritten.
func playerFinancialOwner(p Scenario) string {
	if p.PlayerEntityID != "" {
		return p.PlayerEntityID
	}
	if len(p.Initial.Accounts) > 0 {
		if owner := financialOwner(&p.Initial, p.Initial.Accounts[0].Owner); owner != nil {
			return owner.ID
		}
	}
	return ""
}
