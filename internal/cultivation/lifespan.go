package cultivation

// This file holds the confirmed first-version lifespan scale from
// 自制-时间寿元与长时世界推进 §3. It is a table and nothing else: the join
// with a calendar, with a character's established age and with the branch clock
// lives in internal/interactive, because that is the only place that can see
// both a clock and a stage key.
//
// Three properties are deliberate and each has cost a real bug somewhere else.
//
//   - These are totals counted from birth, not years granted per breakthrough.
//     A character who reaches Foundation at 110 has 200 years in total and 90
//     left, not "10 left plus another 200". The table therefore answers "what is
//     this person's whole span", and never "what does this promotion add".
//
//   - A breakthrough changes which row applies and nothing else. The age stays
//     exactly where it was; nobody is reset to zero. That is why the lookup
//     takes a Realm and returns a total rather than an increment.
//
//   - Sub-stages and Qi layers do not move the row. Early, middle and late are
//     all one major realm, so all of them read the same number. A table keyed by
//     StageKey would silently give 炼气三层 and 炼气十二层 different lifespans,
//     which §3 forbids.
//
// These are design values the user confirmed for this project. They are not a
// claim about any published work, and nothing here is read by the game until a
// story has frozen a calendar: see interactive.LifespanEnding.

// LifespanRule distinguishes a finite natural lifespan from a supported realm
// with no fixed natural-age limit. Neither case grants immunity to injury or
// heavenly tribulations. Unknown realms remain unknown.
type LifespanRule struct {
	Years        int
	NoFixedLimit bool
}

func LifespanRuleFor(realm Realm, rules ...*Rules) (LifespanRule, bool) {
	if len(rules) > 0 && rules[0] != nil {
		for _, key := range rules[0].StageOrder {
			stage, _ := ParseStageKey(key)
			if stage.Realm == realm {
				return rules[0].Stages[key].Lifespan, true
			}
		}
		return LifespanRule{}, false
	}
	switch realm {
	case RealmMortal:
		return LifespanRule{Years: 100}, true
	case RealmQi:
		return LifespanRule{Years: 120}, true
	case RealmFoundation:
		return LifespanRule{Years: 200}, true
	case RealmCore:
		return LifespanRule{Years: 500}, true
	case RealmNascent:
		return LifespanRule{Years: 1000}, true
	case RealmSpirit:
		return LifespanRule{Years: 2000}, true
	case RealmVoid, RealmIntegration, RealmMahayana:
		return LifespanRule{NoFixedLimit: true}, true
	default:
		return LifespanRule{}, false
	}
}

// LifespanYears returns a finite natural lifespan only. The bool is false both
// for an unknown realm and for a known realm without a finite age boundary;
// callers displaying the distinction use LifespanRuleFor.
func LifespanYears(realm Realm, rules ...*Rules) (int, bool) {
	rule, known := LifespanRuleFor(realm, rules...)
	return rule.Years, known && !rule.NoFixedLimit
}
