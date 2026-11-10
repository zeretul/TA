package lore

import "strings"

// A lore item may belong to a resource scope. Scope is how one workspace can
// hold several worlds without their setting books colliding: a script pack
// writes its entries under its own scope, and a story bound to that pack reads
// only shared entries plus its own scope.
//
// Scope is an opaque, program-assigned token. It is not shown to the model and
// not edited in the lore editor; the editor sees every scope so the user can
// still review and fix everything in one place.
const (
	// ScopeShared marks user-authored entries that belong to no particular
	// world. They stay visible to every reader, which is the behaviour every
	// pre-scope workspace already relied on.
	ScopeShared = ""

	// scopeUnbound marks a store that is not restricted to any world. It is a
	// reader-side value only and is never written to an item, so it must not be
	// confusable with ScopeShared: an unbound reader is the lore editor, while a
	// reader bound to ScopeShared is a story that belongs to no pack and must
	// not see pack-written entries. The NUL prefix cannot survive
	// normalizeLoreScope, so no item can ever claim this scope.
	scopeUnbound = "\x00unbound"
)

// normalizeLoreScope trims a scope token and drops characters that cannot be
// part of a stable identifier. An unrecognisable token collapses to the shared
// scope rather than creating a scope nothing can address.
func normalizeLoreScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" || strings.HasPrefix(scope, "\x00") {
		return ScopeShared
	}
	var sb strings.Builder
	for _, r := range scope {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-', r == '_', r == ':', r == '@', r == '.':
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return ScopeShared
	}
	return sb.String()
}

// scopeAllows reports whether an item in itemScope is visible to a reader
// bound to readScope. The unbound reader sees everything, which is what the
// lore editor needs to review and repair every scope from one screen. A bound
// reader sees shared entries plus its own scope only.
func scopeAllows(readScope, itemScope string) bool {
	if readScope == scopeUnbound {
		return true
	}
	itemScope = normalizeLoreScope(itemScope)
	return itemScope == ScopeShared || itemScope == readScope
}
