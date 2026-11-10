package cultivation

// WithLookup attaches a branch-local, read-only archive to a fresh catalog.
// The caller must keep that snapshot immutable for this catalog's lifetime.
// Exact ID reads validate and cache only the requested definition. Enumerating
// all definitions remains explicit; no process-global cache can cross branches.
func (c *ItemCatalog) WithLookup(lookup func(string) (ItemDef, bool), ids func() []string) *ItemCatalog {
	c.lookup, c.lookupIDs = lookup, ids
	return c
}

func (r *CraftRegistry) WithLookup(lookup func(string) (CraftMethodDef, bool), ids func() []string) *CraftRegistry {
	r.lookup, r.lookupIDs = lookup, ids
	return r
}

func (r *SkillRegistry) WithLookup(lookup func(string) (SkillDef, bool), ids func() []string) *SkillRegistry {
	r.lookup, r.lookupIDs = lookup, ids
	return r
}

func lookupCatalogIDs(registered []string, lookup func() []string) []string {
	out := append([]string(nil), registered...)
	if lookup == nil {
		return out
	}
	seen := make(map[string]bool, len(out))
	for _, id := range out {
		seen[id] = true
	}
	for _, id := range lookup() {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}
