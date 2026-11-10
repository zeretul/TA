package cultivation

import (
	"errors"
	"testing"
)

// These tests cover the boundary between the stored journal facts and the
// derived numbers. The design requires that the interface show real integers
// and that the model not be able to reword a character's cultivation; both
// hold only if the stored form is a value the rules recognise and the numbers
// are derived rather than stored. These tests are what pins that down.

func TestStageKeysRoundTripEverySupportedStage(t *testing.T) {
	keys := StageKeys()
	if len(keys) != 1+QiLayers+21 {
		t.Fatalf("expected %d stage keys, got %d", 1+QiLayers+21, len(keys))
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Fatalf("stage key %q is listed twice", key)
		}
		seen[key] = true
		parsed, ok := ParseStageKey(key)
		if !ok {
			t.Fatalf("stage key %q does not parse back", key)
		}
		if roundTrip := StageKey(parsed); roundTrip != key {
			t.Fatalf("stage key %q round-trips to %q", key, roundTrip)
		}
	}
}

func TestStageKeyNamesTheStageNotItsWording(t *testing.T) {
	// The key must survive a change of display wording, so it carries the
	// realm and layer only. A key that embedded prose would move when the
	// wording moved, which is the drift this design exists to prevent.
	cases := map[Cultivation]string{
		Mortal(): "mortal",
		Qi(1):    "qi_1",
		Qi(6):    "qi_6",
		Qi(13):   "qi_13",
		{Realm: RealmFoundation, Stage: StageEarly}: "foundation_early",
		{Realm: RealmFoundation, Stage: StageLate}:  "foundation_late",
		{Realm: RealmCore, Stage: StageMiddle}:      "core_middle",
		{Realm: RealmNascent, Stage: StageEarly}:    "nascent_early",
		{Realm: RealmSpirit, Stage: StageLate}:      "spirit_late",
	}
	for cultivation, want := range cases {
		if got := StageKey(cultivation); got != want {
			t.Errorf("StageKey(%+v) = %q, want %q", cultivation, got, want)
		}
	}
}

func TestUnknownStageKeyIsRefusedNotSilentlyReset(t *testing.T) {
	// A character whose recorded stage cannot be read is a real problem. It
	// must not become a mortal, because that would silently erase a
	// character's cultivation the first time a key was mistyped.
	for _, key := range []string{"", "   ", "qi_0", "qi_14", "qi_", "qi_x", "foundation", "foundation_mid", "immortal", "筑基"} {
		if parsed, ok := ParseStageKey(key); ok {
			t.Errorf("ParseStageKey(%q) unexpectedly succeeded as %+v", key, parsed)
		}
	}
}

func TestUnreadableStageKeyReportsTheStoredValue(t *testing.T) {
	_, err := CharacterStateFromRecord("qi_99", 0, 0, 0, 0, 0, "", "", nil)
	if err == nil {
		t.Fatal("expected an error for an unreadable stage key")
	}
	var unknown *UnknownStageKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownStageKeyError, got %T", err)
	}
	if unknown.Key != "qi_99" {
		t.Fatalf("error should name the stored key, got %q", unknown.Key)
	}
}

func TestAttributesAreDerivedFromTheStoredStage(t *testing.T) {
	// Qi layer 6: base health 200, mana 160, attack 40, defense/agility/
	// perception 20. These are the design's own published values for the
	// layer, reached here through the stored-key path rather than directly.
	state, err := CharacterStateFromRecord("qi_6", 200, 160, 0, 0, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := state.Attributes()
	want := Attributes{HealthMax: 200, Health: 200, ManaMax: 160, Mana: 160, Attack: 40, Defense: 20, Agility: 20, Perception: 20}
	if attrs != want {
		t.Fatalf("derived attributes = %+v, want %+v", attrs, want)
	}
}

func TestStoredCurrentPoolsSurviveDerivation(t *testing.T) {
	// A wound is a real fact the stage table cannot reconstruct, so the
	// current pools are stored. Deriving the maxima must not refill them.
	state, err := CharacterStateFromRecord("qi_6", 57, 12, 0, 0, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := state.Attributes()
	if attrs.Health != 57 || attrs.Mana != 12 {
		t.Fatalf("current pools = %d/%d, want 57/12", attrs.Health, attrs.Mana)
	}
	if attrs.HealthMax != 200 || attrs.ManaMax != 160 {
		t.Fatalf("maxima = %d/%d, want 200/160", attrs.HealthMax, attrs.ManaMax)
	}
}

func TestDerivedAttributesObeyTheUnifiedBudget(t *testing.T) {
	// The character's own specialisation and grade are generated, not stored,
	// and they enter the same single aggregation as equipment. The design's
	// worked example is base attack 100 with +10/+20/+15/+15 reaching 160 and
	// no combination exceeding the cap; this checks the stored-path entry
	// point reaches that same engine rather than a parallel one.
	modifiers := []AttributeModifier{
		{ID: "implement", Kind: SourceMainImplement, Attribute: AttrAttack, Value: 0.10},
		{ID: "armor", Kind: SourceArmor, Attribute: AttrAttack, Value: 0.20},
		{ID: "formation", Kind: SourceFormation, Attribute: AttrAttack, Value: 0.15},
		{ID: "reward", Kind: SourceEventReward, Attribute: AttrAttack, Value: 0.15},
	}
	state, err := CharacterStateFromRecord("qi_6", 200, 160, 0, 0, 0, "", "", modifiers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Base attack at qi 6 is 40; +60% is the published cap, so 64 is the most
	// this character can reach whatever the sources are called.
	if got := state.Attributes().Attack; got != 64 {
		t.Fatalf("attack = %d, want 64 (40 at the 60%% cap)", got)
	}
}

func TestSpecialisationIsGeneratedNotStored(t *testing.T) {
	// A stored specialisation bonus could outlive a change of technique
	// template. Generating it from the template means changing the template
	// cannot leave a stale bonus behind.
	base, err := CharacterStateFromRecord("qi_6", 200, 160, 0, 0, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attack, err := CharacterStateFromRecord("qi_6", 200, 160, 0, 0, 0, string(SpecialisationAttack), "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base.Attributes().Attack != 40 {
		t.Fatalf("balanced base attack = %d, want 40", base.Attributes().Attack)
	}
	if got := attack.Attributes().Attack; got != 44 {
		t.Fatalf("attack specialisation attack = %d, want 44 (40 x 1.10)", got)
	}
	// The same template lowers health, so the panel shows the trade rather
	// than only the gain.
	if got := attack.Attributes().HealthMax; got != 190 {
		t.Fatalf("attack specialisation health = %d, want 190 (200 x 0.95)", got)
	}
}

func TestBurdenStateClampsStoredValues(t *testing.T) {
	state, err := CharacterStateFromRecord("qi_6", 200, 160, 130, 60, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	burden := state.BurdenState()
	if burden.Current != 100 {
		t.Fatalf("burden = %d, want clamped to 100", burden.Current)
	}
	if burden.Residual != 60 {
		t.Fatalf("residual = %d, want 60", burden.Residual)
	}
}

func TestMortalStageDerivesMortalNumbers(t *testing.T) {
	// A starting character with no cultivation still reads real numbers, and
	// cannot perform techniques: zero mana must not become free casting.
	state, err := CharacterStateFromRecord("mortal", 80, 0, 0, 0, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := state.Attributes()
	if attrs.HealthMax != 80 || attrs.ManaMax != 0 || attrs.Attack != 10 {
		t.Fatalf("mortal attributes = %+v", attrs)
	}
	if CanUseTechniques(state.Cultivation) {
		t.Fatal("a mortal must not be able to use techniques")
	}
}

func TestAdvanceFromTheTopRefiningLayerStopsAtTheRealBoundary(t *testing.T) {
	// The design requires that reaching an unimplemented stage stop at the
	// real boundary rather than inventing progress past it. Foundation is a
	// major breakthrough needing an authorisation and a roll, so a training
	// period from the top refining layer must NOT roll into it however long it
	// runs — it banks progress and reports the boundary.
	state, err := CharacterStateFromRecord("qi_13", 340, 272, 0, 0, 0, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Cultivation.Realm != RealmQi || state.Cultivation.Layer != QiLayers {
		t.Fatalf("stored stage = %+v, want qi layer %d", state.Cultivation, QiLayers)
	}

	progress := AdvanceCultivation(state.Cultivation, 10_000, TrainingModifiers{
		Talent: TalentNormal, Quality: QualityOrdinary,
		Environment: EnvironmentOrdinary, Resource: SupportNone, HealthFraction: 1,
	})
	if progress.Cultivation.Realm != RealmQi || progress.Cultivation.Layer != QiLayers {
		t.Fatalf("ten thousand days advanced past the boundary into %+v", progress.Cultivation)
	}
	if !progress.StoppedAtBoundary {
		t.Fatal("stopping at the top refining layer must be reported as the boundary")
	}
	if progress.Accumulated <= 0 {
		t.Fatal("time spent at the boundary still banks progress")
	}

	// The filled threshold is what authorises the major breakthrough, so
	// discarding it would leave the character permanently one step short of a
	// breakthrough they had in fact earned.
	attrs := state.Attributes()
	requirement := CanAttemptBreakthrough(progress.Cultivation, progress.Accumulated, attrs, BreakthroughFoundation, false)
	if !requirement.Eligible {
		t.Fatalf("a full refining threshold must authorise the foundation breakthrough, got %v", requirement.Reasons)
	}
}

func TestTheHighestImplementedStageIsTheLateMahayanaRealm(t *testing.T) {
	// AtTerminalStage marks the end of what this rule version implements, which
	// is a different question from the top refining layer: past it progress
	// still accumulates but no transition exists at all.
	top, ok := ParseStageKey("mahayana_late")
	if !ok {
		t.Fatal("mahayana_late must be a supported stage")
	}
	if !AtTerminalStage(top) {
		t.Fatal("mahayana late must report as the terminal stage")
	}
	if AtTerminalStage(Qi(QiLayers)) {
		t.Fatal("the top refining layer is a breakthrough boundary, not the terminal stage")
	}
}
