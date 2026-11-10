package cultivation

import (
	"strings"
)

// This file implements 《自制-技能与施展》V0.2 §6: a technique's definition, a
// technique's registration, and a character having learned it are three
// different facts.
//
// The design states the three explicitly:
//
//	秘籍存在、技能被登记、人物已学，是三件不同的事。
//
// A registry holds the second fact and nothing else. It never implies the
// third: a definition can be registered — because a world pack shipped it, or
// because a generation was validated — while no character has learned it, and a
// character can have learned a technique whose definition the registry no
// longer lists (a branch that diverged before the registration). Keeping the
// two apart is what stops "the catalogue mentions it" from silently becoming
// "the protagonist can cast it".
//
// Versioning is the other half. §6 requires that an upgrade create a new version
// and formally replace the character's known version rather than rewriting the
// old definition in place, so a registered definition is immutable: the only
// mutation the registry offers is registering a further version.

// SkillRegistrationError reports a definition the registry refused.
//
// It carries the definition ID and the reason so the caller can tell the player
// which generated technique was rejected and why, rather than reporting that
// "a technique" failed.
type SkillRegistrationError struct {
	ID     string
	Reason error
}

func (e *SkillRegistrationError) Error() string {
	if e.ID == "" {
		return "cultivation: skill registration refused: " + e.Reason.Error()
	}
	return "cultivation: skill " + e.ID + " refused: " + e.Reason.Error()
}

// Unwrap exposes the underlying reason so callers can test for the specific
// budget error with errors.Is.
func (e *SkillRegistrationError) Unwrap() error { return e.Reason }

// SkillRegistry holds the definitions registered on one story branch.
//
// The zero value is not usable; call NewSkillRegistry. The type is deliberately
// not safe for concurrent use, because a registry belongs to one story's commit
// path and a lock there would hide an ordering problem rather than fix it.
type SkillRegistry struct {
	lookup    func(string) (SkillDef, bool)
	lookupIDs func() []string
	// versions maps a definition ID to every registered version, in ascending
	// version order. The last entry is the current definition of that ID.
	versions map[string][]SkillDef
	// order records first-registration order per ID, so a listing is stable
	// across runs rather than following map iteration.
	order []string
}

// NewSkillRegistry returns an empty registry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{versions: map[string][]SkillDef{}}
}

// NewSkillRegistryWithBuiltins returns a registry pre-loaded with the published
// demonstration template set of §4.2.
//
// The built-ins are registered, not learned. A world pack that ships different
// names for the same templates replaces them through a further version rather
// than by mutating the built-ins, which stay available as the rules' own
// reference definitions.
func NewSkillRegistryWithBuiltins(rules ...*Rules) *SkillRegistry {
	registry := NewSkillRegistry()
	if len(rules) > 0 && rules[0] != nil {
		for _, def := range rules[0].Skills {
			_ = registry.Register(def)
		}
		return registry
	}
	for _, skill := range skillTable {
		// The built-in table is authored against the same validation a
		// generated definition faces, so an editing mistake in the table is
		// caught here rather than at the first cast.
		_ = registry.Register(skill)
	}
	return registry
}

// Register validates a definition and adds it as the next version of its ID.
//
// Validation runs first and a refused definition is not stored: §6 requires the
// program to check supported effects, budget and numeric range before
// registering, because registering first and relying on the global cap
// afterwards would let an out-of-budget technique exist and merely be capped.
//
// A version lower than one already registered is refused rather than
// reordering the history, so a replayed journal cannot rewind a technique to an
// earlier balance.
func (r *SkillRegistry) Register(skill SkillDef) error {
	if r.versions == nil {
		return &SkillRegistrationError{ID: skill.ID, Reason: ErrSkillRegistryNotInitialised}
	}
	if err := ValidateSkillDefinition(skill); err != nil {
		return &SkillRegistrationError{ID: skill.ID, Reason: err}
	}
	if skill.Version < 1 {
		return &SkillRegistrationError{ID: skill.ID, Reason: ErrSkillVersionOutOfRange}
	}
	if r.lookup != nil {
		_, _ = r.Current(skill.ID)
	}
	existing := r.versions[skill.ID]
	if len(existing) > 0 && skill.Version <= existing[len(existing)-1].Version {
		return &SkillRegistrationError{ID: skill.ID, Reason: ErrSkillVersionNotAdvancing}
	}
	if len(existing) == 0 {
		r.order = append(r.order, skill.ID)
	}
	r.versions[skill.ID] = append(existing, skill)
	return nil
}

// Current returns the highest registered version of an ID.
//
// It reports false for an ID with no registration. A registered definition is
// not a learned technique: callers asking "may this character perform it" must
// consult the character's own learned list as well, which is why this returns a
// definition rather than an answer.
func (r *SkillRegistry) Current(id string) (SkillDef, bool) {
	if r == nil {
		return SkillDef{}, false
	}
	id = strings.TrimSpace(id)
	versions := r.versions[id]
	if len(versions) == 0 && r.lookup != nil {
		skill, ok := r.lookup(id)
		if !ok || skill.ID != id || skill.Version < 1 || ValidateSkillDefinition(skill) != nil {
			return SkillDef{}, false
		}
		r.versions[id] = []SkillDef{skill}
		versions = r.versions[id]
	}
	if len(versions) == 0 {
		return SkillDef{}, false
	}
	return versions[len(versions)-1], true
}

// Version returns one specific registered version.
//
// §8 requires a combat round to be bound to the version that was in force when
// it began, so a later balance update cannot retroactively change a resolved
// exchange. This is how a caller re-reads that pinned version.
func (r *SkillRegistry) Version(id string, version int) (SkillDef, bool) {
	if r == nil {
		return SkillDef{}, false
	}
	_, _ = r.Current(id)
	for _, skill := range r.versions[strings.TrimSpace(id)] {
		if skill.Version == version {
			return skill, true
		}
	}
	return SkillDef{}, false
}

// IDs lists the registered definition IDs in first-registration order.
func (r *SkillRegistry) IDs() []string {
	if r == nil {
		return nil
	}
	ids := lookupCatalogIDs(r.order, r.lookupIDs)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := r.Current(id); ok {
			out = append(out, id)
		}
	}
	return out
}

// Definitions lists the current definition of every registered ID, in
// first-registration order.
func (r *SkillRegistry) Definitions() []SkillDef {
	if r == nil {
		return nil
	}
	out := make([]SkillDef, 0, len(r.order))
	for _, id := range r.IDs() {
		current, ok := r.Current(id)
		if !ok {
			continue
		}
		out = append(out, current)
	}
	return out
}

// LearnedSkills is one character's set of learned techniques.
//
// It stores the definition ID together with the version the character actually
// learned. §6 requires that an upgrade formally replace the known version, so
// the pair is the record: a character who learned version 1 keeps using version
// 1's numbers until version 2 is formally granted to them, even though the
// registry's current definition has moved on.
type LearnedSkills struct {
	// versions maps definition ID to the learned version.
	versions map[string]int
	// order records learning order, so the interface lists a character's
	// techniques in the order they were acquired.
	order []string
}

// NewLearnedSkills returns an empty learned set.
func NewLearnedSkills() *LearnedSkills {
	return &LearnedSkills{versions: map[string]int{}}
}

// Learn grants mastery of a registered definition to this character.
//
// The definition must already be registered: §6 requires that only after
// learning or eligibility is formally confirmed may a technique join the
// character's learned list, and a technique that was never registered has no
// fixed numbers to learn. Registering a definition does not learn it — this is
// the only call that does.
//
// Re-learning the same ID at a higher version replaces the known version, which
// is the formal replacement §6 describes. Re-learning at the same or a lower
// version is a no-op rather than an error, so a replayed commit is idempotent.
func (l *LearnedSkills) Learn(registry *SkillRegistry, id string, version int) error {
	if l.versions == nil {
		return ErrLearnedSkillsNotInitialised
	}
	id = strings.TrimSpace(id)
	_, ok := registry.Version(id, version)
	if !ok {
		if _, registered := registry.Current(id); registered {
			return ErrSkillVersionNotRegistered
		}
		return ErrSkillNotRegistered
	}
	// Character stage eligibility is checked by the study consumer. This set
	// stores mastery only; a qi-stage probe would reject legitimate high arts.
	if current, known := l.versions[id]; known && current >= version {
		return nil
	} else if !known {
		l.order = append(l.order, id)
	}
	l.versions[id] = version
	return nil
}

// Knows reports whether the character has learned a definition ID.
//
// It is the answer to "may this character be offered the technique at all". It
// is not the answer to "may they perform it right now": mana, burden, focus and
// preparation are CanPerform's business.
func (l *LearnedSkills) Knows(id string) bool {
	if l == nil {
		return false
	}
	_, ok := l.versions[strings.TrimSpace(id)]
	return ok
}

// Known returns the definition the character has learned, resolved from the
// registry at the learned version.
//
// A learned technique whose definition is no longer in the registry reports
// false rather than substituting the current definition: §6 forbids silently
// rewriting what a character knows, and a branch that diverged before a
// registration is exactly the case where the learned version is the only
// truthful answer.
func (l *LearnedSkills) Known(registry *SkillRegistry, id string) (SkillDef, bool) {
	if l == nil {
		return SkillDef{}, false
	}
	version, ok := l.versions[strings.TrimSpace(id)]
	if !ok {
		return SkillDef{}, false
	}
	return registry.Version(strings.TrimSpace(id), version)
}

// IDs lists the learned definition IDs in learning order.
func (l *LearnedSkills) IDs() []string {
	if l == nil {
		return nil
	}
	out := make([]string, len(l.order))
	copy(out, l.order)
	return out
}

// KnownDefinitions lists the character's learned techniques, resolved at the
// version they learned, in learning order.
//
// This is the list an interface or an agent may offer the player. An entry that
// cannot be resolved is omitted rather than shown with another version's
// numbers, because a technique with the wrong numbers is worse than an absent
// one.
func (l *LearnedSkills) KnownDefinitions(registry *SkillRegistry) []SkillDef {
	if l == nil {
		return nil
	}
	out := make([]SkillDef, 0, len(l.order))
	for _, id := range l.order {
		definition, ok := l.Known(registry, id)
		if !ok {
			continue
		}
		out = append(out, definition)
	}
	return out
}
