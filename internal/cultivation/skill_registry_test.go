package cultivation

import (
	"errors"
	"testing"
)

// The tests here cover 《自制-技能与施展》V0.2 §6 and §10. The design keeps a
// definition existing, a definition being registered and a character having
// learned it as three separate facts, and its acceptance list asks for the
// distinction to be observable: a learned technique must be distinguishable
// from an unlearned manual, and a generated technique's numbers must be stable
// across a later session, a restart and a branch.

func TestRegistrationIsNotLearning(t *testing.T) {
	// A world pack shipping a technique template registers it; that must not
	// give anyone the technique. The design's sentence is explicit that 秘籍存在、
	// 技能被登记、人物已学 are three different things.
	registry := NewSkillRegistryWithBuiltins()
	learned := NewLearnedSkills()

	if _, registered := registry.Current("fire_bolt"); !registered {
		t.Fatal("the built-in fire bolt must be registered")
	}
	if learned.Knows("fire_bolt") {
		t.Error("registering a definition must not learn it for anyone")
	}
	if len(learned.KnownDefinitions(registry)) != 0 {
		t.Error("a character who has learned nothing must have an empty list")
	}
}

func TestLearnRequiresARegisteredDefinition(t *testing.T) {
	registry := NewSkillRegistryWithBuiltins()
	learned := NewLearnedSkills()

	// A generated definition that was never registered has no fixed numbers to
	// learn, so granting it must be refused rather than inventing an entry.
	if err := learned.Learn(registry, "generated_but_unregistered", 1); !errors.Is(err, ErrSkillNotRegistered) {
		t.Errorf("learning an unregistered technique returned %v, want ErrSkillNotRegistered", err)
	}
	if learned.Knows("generated_but_unregistered") {
		t.Error("a refused learn must not leave the technique known")
	}

	if err := learned.Learn(registry, "fire_bolt", 1); err != nil {
		t.Fatalf("learning a registered technique failed: %v", err)
	}
	if !learned.Knows("fire_bolt") {
		t.Error("a granted technique must be known")
	}
}

func TestAnOutOfBudgetDefinitionIsRefusedBeforeRegistration(t *testing.T) {
	registry := NewSkillRegistry()
	// The design requires the check to happen before registration: a definition
	// that is merely capped at use would still exist as an out-of-budget entry.
	overBudget := SkillDef{
		ID: "over_budget", Version: 1, Power: 2.0,
		ManaFraction: 0.10, Burden: 20, Targets: 1,
		ExecutionActions: 1, IgnoreDefense: 0.35,
	}
	err := registry.Register(overBudget)
	if !errors.Is(err, ErrSkillIgnoreDefenseOverCap) {
		t.Errorf("registering an over-cap definition returned %v, want ErrSkillIgnoreDefenseOverCap", err)
	}
	if _, registered := registry.Current("over_budget"); registered {
		t.Error("a refused definition must not be registered")
	}
}

func TestARefusedRegistrationNamesTheDefinition(t *testing.T) {
	// The player has to be told which generated technique was rejected, not
	// merely that one was.
	registry := NewSkillRegistry()
	err := registry.Register(SkillDef{ID: "bad", Version: 1, Power: 1, Targets: 0, ExecutionActions: 1})
	var registration *SkillRegistrationError
	if !errors.As(err, &registration) {
		t.Fatalf("registration error is %T, want *SkillRegistrationError", err)
	}
	if registration.ID != "bad" {
		t.Errorf("registration error names %q, want %q", registration.ID, "bad")
	}
}

func TestAVersionUpgradeReplacesRatherThanRewrites(t *testing.T) {
	registry := NewSkillRegistry()
	first := SkillDef{ID: "condensed_blade", Version: 1, Power: 1.60, ManaFraction: 0.12, Burden: 22, Targets: 1, ExecutionActions: 1}
	if err := registry.Register(first); err != nil {
		t.Fatalf("registering version 1 failed: %v", err)
	}

	learned := NewLearnedSkills()
	if err := learned.Learn(registry, "condensed_blade", 1); err != nil {
		t.Fatalf("learning version 1 failed: %v", err)
	}
	knownBefore, ok := learned.Known(registry, "condensed_blade")
	if !ok || knownBefore.Power != 1.60 {
		t.Fatalf("learned technique = %+v, want version 1 at power 1.60", knownBefore)
	}

	// A balance change is a new version. The old definition must still be
	// readable, because a resolved exchange and a branch that diverged before
	// the change both have to keep the numbers they were fought with.
	second := first
	second.Version = 2
	second.Power = 1.70
	if err := registry.Register(second); err != nil {
		t.Fatalf("registering version 2 failed: %v", err)
	}

	old, ok := registry.Version("condensed_blade", 1)
	if !ok || old.Power != 1.60 {
		t.Errorf("version 1 after the upgrade = %+v, want power 1.60 preserved", old)
	}
	current, ok := registry.Current("condensed_blade")
	if !ok || current.Power != 1.70 {
		t.Errorf("current definition = %+v, want version 2 at power 1.70", current)
	}

	// The character keeps using the version they actually learned until the new
	// one is formally granted. This is the whole point of §6's replacement rule:
	// the upgrade does not silently change what they know.
	stillKnown, ok := learned.Known(registry, "condensed_blade")
	if !ok || stillKnown.Power != 1.60 {
		t.Errorf("a character must keep the version they learned: got %+v, want power 1.60", stillKnown)
	}

	if err := learned.Learn(registry, "condensed_blade", 2); err != nil {
		t.Fatalf("formally granting version 2 failed: %v", err)
	}
	upgraded, ok := learned.Known(registry, "condensed_blade")
	if !ok || upgraded.Power != 1.70 {
		t.Errorf("after the formal replacement the character = %+v, want power 1.70", upgraded)
	}
}

func TestARegisteredVersionCannotGoBackwards(t *testing.T) {
	registry := NewSkillRegistry()
	first := SkillDef{ID: "fire_bolt", Version: 2, Power: 1.20, ManaFraction: 0.05, Burden: 6, Targets: 1, ExecutionActions: 1}
	if err := registry.Register(first); err != nil {
		t.Fatalf("registering version 2 failed: %v", err)
	}
	// A replayed journal must not be able to rewind a technique to an earlier
	// balance, so a lower version is refused rather than reordering history.
	older := first
	older.Version = 1
	if err := registry.Register(older); !errors.Is(err, ErrSkillVersionNotAdvancing) {
		t.Errorf("registering an earlier version returned %v, want ErrSkillVersionNotAdvancing", err)
	}
	current, _ := registry.Current("fire_bolt")
	if current.Version != 2 {
		t.Errorf("current version = %d, want 2", current.Version)
	}
}

func TestRelearningAtTheSameVersionIsIdempotent(t *testing.T) {
	registry := NewSkillRegistryWithBuiltins()
	learned := NewLearnedSkills()
	if err := learned.Learn(registry, "shield", 1); err != nil {
		t.Fatalf("learning failed: %v", err)
	}
	// A retried commit must not double an entry or fail the turn.
	if err := learned.Learn(registry, "shield", 1); err != nil {
		t.Errorf("re-learning the same version returned %v, want no error", err)
	}
	if got := len(learned.IDs()); got != 1 {
		t.Errorf("learned list has %d entries after a retry, want 1", got)
	}
}

func TestALearnedTechniqueStaysStableAcrossSessionsAndBranches(t *testing.T) {
	// §10 asks for a generated technique's numbers to be stable across a later
	// session, a restart and a branch. Two registries rebuilt from the same
	// recorded definitions must resolve the same numbers, and a branch taken
	// before an upgrade must keep the pre-upgrade numbers.
	generated := SkillDef{
		ID: "generated.flame_lance", Version: 1, Power: 1.35,
		ManaFraction: 0.15, Burden: 24, Targets: 1, ExecutionActions: 1, Element: "fire",
	}
	recorded := []SkillDef{generated}

	rebuild := func() (*SkillRegistry, *LearnedSkills) {
		registry := NewSkillRegistry()
		for _, def := range recorded {
			if err := registry.Register(def); err != nil {
				t.Fatalf("replaying a recorded definition failed: %v", err)
			}
		}
		learned := NewLearnedSkills()
		if err := learned.Learn(registry, generated.ID, 1); err != nil {
			t.Fatalf("replaying a learned technique failed: %v", err)
		}
		return registry, learned
	}

	firstRun, firstLearned := rebuild()
	secondRun, secondLearned := rebuild()
	a, _ := firstLearned.Known(firstRun, generated.ID)
	b, _ := secondLearned.Known(secondRun, generated.ID)
	if a != b {
		t.Errorf("a technique's numbers must not depend on which session rebuilt them: %+v vs %+v", a, b)
	}

	// The branch that diverged before the balance change keeps version 1.
	upgraded := generated
	upgraded.Version = 2
	upgraded.Power = 1.45
	if err := secondRun.Register(upgraded); err != nil {
		t.Fatalf("registering the upgrade failed: %v", err)
	}
	onTheOldBranch, _ := firstLearned.Known(firstRun, generated.ID)
	if onTheOldBranch.Power != 1.35 {
		t.Errorf("the branch taken before the upgrade = power %v, want 1.35", onTheOldBranch.Power)
	}
}

func TestKnownOmitsATechniqueWhoseDefinitionIsGone(t *testing.T) {
	// A branch may hold a learned technique whose definition the current
	// registry no longer lists. Showing another version's numbers would be
	// worse than showing nothing, so the entry is omitted.
	registry := NewSkillRegistry()
	learned := NewLearnedSkills()
	def := SkillDef{ID: "temporary", Version: 1, Power: 1.0, ManaFraction: 0.05, Burden: 5, Targets: 1, ExecutionActions: 1}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registering failed: %v", err)
	}
	if err := learned.Learn(registry, "temporary", 1); err != nil {
		t.Fatalf("learning failed: %v", err)
	}

	empty := NewSkillRegistry()
	// The learned fact survives: the character really did learn it, and the
	// story records that. Only the resolvable definition is gone, which is why
	// Known is the call that reports false.
	if !learned.Knows("temporary") {
		t.Error("the learned set must keep the fact that the technique was learned")
	}
	if _, ok := learned.Known(empty, "temporary"); ok {
		t.Error("a technique with no registered version must not resolve to a definition")
	}
	if definitions := learned.KnownDefinitions(empty); len(definitions) != 0 {
		t.Errorf("a technique with no resolvable definition must be omitted, got %+v", definitions)
	}
}

func TestBuiltinsAreRegisteredAndValid(t *testing.T) {
	registry := NewSkillRegistryWithBuiltins()
	for _, skill := range skillTable {
		if _, ok := registry.Current(skill.ID); !ok {
			t.Errorf("built-in %s was not registered", skill.ID)
		}
	}
}
