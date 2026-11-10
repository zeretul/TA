package cultivation

// This file is the one place where a stored cultivation stage is turned into
// the six numbers the player reads.
//
// The point of it is negative as much as positive: it establishes that the
// numbers are NOT stored. A journal holds a stage key and the real current
// pools; the maxima and the four derived attributes are recomputed from the
// rules every time they are shown or used. That is what makes the design's
// "the model may not reword the character's cultivation" requirement hold
// structurally rather than by instruction — there is no stored number for the
// model to reword, and no second copy to drift.
//
// The current pools ARE stored, because a wound is a real fact about the
// character that the stage table cannot reconstruct.

// CharacterState is a character's rules position plus the two real current
// values that no formula can recover.
type CharacterState struct {
	// Cultivation is where the character stands on the stage table.
	Cultivation Cultivation
	// Accumulated is progress banked toward the current stage. It is a real
	// accumulated quantity, so it is stored rather than recomputed.
	Accumulated float64
	// Health and Mana are the current pools. Negative stored values are clamped
	// to zero; a value above the current maximum is trimmed to it, because a
	// maximum can fall when a bonus is lost.
	Health int
	Mana   int
	// Burden is the meridian burden, 0–100.
	Burden int
	// Residual is the burden floor a meridian residual injury holds.
	Residual int
	// Specialisation is the technique template in effect.
	Specialisation TechniqueSpecialisation
	// Quality is the technique grade, which carries an attack correction.
	Quality TechniqueQuality
	// Techniques are the structured methods currently circulating.
	Techniques []TechniqueDef
	// TechniquesStructured records that the character's state carries a
	// structured method record at all, even if nothing is circulating right now.
	// It is what stops the free-text specialisation from being applied as well:
	// a character who deliberately stopped every method has no technique bonus,
	// which is different from a character whose record predates methods.
	TechniquesStructured bool
	// Modifiers are the real, sourced extras in effect: equipment, pills,
	// formations, ally help, event rewards.
	Modifiers []AttributeModifier
}

// Attributes derives the six displayed numbers for a character.
//
// Every value is a whole number and every value is derived here, so the panel,
// the combat round and the simulation all read the same numbers by construction
// rather than by agreeing to.
func (s CharacterState) Attributes() Attributes {
	base := BaseAttributes(s.Cultivation)
	modifiers := s.allModifiers()
	aggregated := AggregateAttributes(base, modifiers, s.Cultivation.rules)
	aggregated.Health = clampInt(s.Health, 0, aggregated.HealthMax)
	aggregated.Mana = clampInt(s.Mana, 0, aggregated.ManaMax)
	return aggregated
}

// allModifiers collects the modifiers that come from the character's own
// build, plus the caller's real extras, into one list for the single
// aggregation of §3.4.
//
// The specialisation and quality entries are generated rather than stored, so
// changing a technique template cannot leave a stale bonus behind.
//
// A structured method set and the free-text template are alternatives, not
// additions. The free text is how a character whose record predates structured
// methods is still priced; once the record exists, the method set is the whole
// answer, because the free text was derived from the same method and counting
// both would double it.
func (s CharacterState) allModifiers() []AttributeModifier {
	modifiers := make([]AttributeModifier, 0, len(s.Modifiers)+8)
	if s.TechniquesStructured {
		modifiers = append(modifiers, TechniqueModifiers(s.Techniques)...)
	} else {
		modifiers = append(modifiers, SpecialisationModifiers(s.Specialisation)...)
		modifiers = append(modifiers, TechniqueQualityModifier(s.Quality)...)
	}
	modifiers = append(modifiers, s.Modifiers...)
	return modifiers
}

// PrimaryTechniqueQuality returns the grade of the highest-grade circulating
// method, which is the grade the training and breakthrough budgets read.
//
// A structured set answers this itself; otherwise the free-text grade is the
// only source, and it is returned unchanged. An empty set with no free text is
// ordinary, which is the rules' own baseline.
func (s CharacterState) PrimaryTechniqueQuality() TechniqueQuality {
	if !s.TechniquesStructured {
		return s.Quality
	}
	best := QualityOrdinary
	for _, technique := range s.Techniques {
		if qualityRank(technique.Quality) > qualityRank(best) {
			best = technique.Quality
		}
	}
	return best
}

// GrowthCoefficient returns the training multiplier of the method actually being
// trained.
//
// Only one method's coefficient applies to a training segment (§4.3). With a
// structured set the selected method is used, falling back to the highest
// coefficient among the circulating ones when the caller does not name one. With
// no structured set the answer is 1.0: the free-text template records a
// specialisation and a grade, never a growth figure, and inventing one would be
// a number the record does not contain.
func (s CharacterState) GrowthCoefficient(selectedID string) float64 {
	if !s.TechniquesStructured {
		return 1.0
	}
	if selectedID != "" {
		for _, technique := range s.Techniques {
			if technique.ID == selectedID && technique.GrowthCoefficient > 0 {
				return technique.GrowthCoefficient
			}
		}
	}
	best := 0.0
	for _, technique := range s.Techniques {
		if technique.GrowthCoefficient > best {
			best = technique.GrowthCoefficient
		}
	}
	if best <= 0 {
		return 1.0
	}
	return best
}

// ManaReduction returns the real casting cost reduction the circulating methods
// grant.
//
// Only methods that carry one contribute, and the total is capped at the rules'
// 20% ceiling here as well as in the definition check: several methods could
// each be legal on their own and still add past the ceiling together.
func (s CharacterState) ManaReduction() float64 {
	total := 0.0
	for _, technique := range s.Techniques {
		if technique.MPCostReduction > 0 {
			total += technique.MPCostReduction
		}
	}
	return clampFloat(total, 0, 0.20)
}

// EffectiveModifiers returns the modifiers the character's own build
// contributes: the technique specialisation, the technique grade and the real
// sourced extras, without any battle-side addition.
//
// It is exported so a battle can carry them onto its combatant. A dose taken
// mid-fight has to be aggregated with them in one pass, and a battle that
// re-derived its attributes from the stage base alone would silently drop the
// specialisation and the technique grade the settled numbers had already
// included. Reading this list rather than rebuilding it keeps the one-pass
// rule's input in one place instead of a second copy that could drift.
func (s CharacterState) EffectiveModifiers() []AttributeModifier {
	return s.allModifiers()
}

// BurdenState returns the character's meridian burden as the casting rules
// expect it.
func (s CharacterState) BurdenState() BurdenState {
	return BurdenState{
		Current:  clampInt(s.Burden, 0, 100),
		Residual: clampInt(s.Residual, 0, 100),
	}
}

// CharacterStateFromRecord rebuilds a character's rules position from the
// values a story stores.
//
// The stage key is the one required input. An unreadable key is reported as
// not-found together with the key itself, so the caller can tell the player
// which recorded value is wrong instead of silently resetting them to a mortal.
func CharacterStateFromRecord(stageKey string, health, mana, burden, residual int, accumulated float64, specialisation string, quality string, modifiers []AttributeModifier) (CharacterState, error) {
	cultivation, ok := ParseStageKey(stageKey)
	if !ok {
		return CharacterState{}, &UnknownStageKeyError{Key: stageKey}
	}
	return CharacterState{
		Cultivation:    cultivation,
		Accumulated:    maxFloat(accumulated, 0),
		Health:         maxInt(health, 0),
		Mana:           maxInt(mana, 0),
		Burden:         clampInt(burden, 0, 100),
		Residual:       clampInt(residual, 0, 100),
		Specialisation: TechniqueSpecialisation(specialisation),
		Quality:        TechniqueQuality(quality),
		Modifiers:      modifiers,
	}, nil
}

// WithTechniques attaches a structured method set to a character.
//
// It is separate from CharacterStateFromRecord rather than another parameter on
// it, so the callers that price a character from the free-text record keep
// working unchanged and only the ones that hold a structured set opt in.
func (s CharacterState) WithTechniques(techniques []TechniqueDef) CharacterState {
	s.Techniques = techniques
	s.TechniquesStructured = true
	return s
}

// UnknownStageKeyError reports a stored stage key the rules cannot read.
//
// It carries the offending key so the message names the actual stored value
// rather than only saying that something was wrong.
type UnknownStageKeyError struct {
	Key string
}

func (e *UnknownStageKeyError) Error() string {
	if e.Key == "" {
		return "cultivation: unrecognised stage key (empty)"
	}
	return "cultivation: unrecognised stage key " + e.Key
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
