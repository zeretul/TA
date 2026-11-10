// Package cultivation implements the cultivation (修仙) numeric rules of
// 《自制-修仙数值规则》V0.3.
//
// The rules are self-contained and deterministic: given a stage, a set of
// real elapsed days and a set of real modifiers, they return the same numbers
// every time. Nothing here reads the story journal or writes state; callers
// pass current values in and commit the returned changes through the story's
// own atomic commit path. That separation is deliberate — the same code backs
// the offline simulation, the story turn and the combat round, so a simulated
// result and a played result cannot disagree about the rules.
//
// This is a design value set, not a claim about any published work.
package cultivation

import "math"

// Realm identifies a major cultivation realm. The zero value is RealmMortal,
// so a character with no cultivation is a mortal by default rather than an
// error case.
type Realm string

const (
	RealmMortal      Realm = "mortal"
	RealmQi          Realm = "qi"
	RealmFoundation  Realm = "foundation"
	RealmCore        Realm = "core"
	RealmNascent     Realm = "nascent"
	RealmSpirit      Realm = "spirit"
	RealmVoid        Realm = "void"
	RealmIntegration Realm = "integration"
	RealmMahayana    Realm = "mahayana"
)

// Stage identifies the sub-stage within a major realm. Qi refining uses
// numbered layers instead; see Layer.
type Stage string

const (
	StageEarly  Stage = "early"
	StageMiddle Stage = "middle"
	StageLate   Stage = "late"
)

// QiLayers is the number of refining layers (炼气一至十三层) below Foundation.
const QiLayers = 13

// Cultivation is a character's position on the stage table: a major realm, a
// sub-stage for realms that have them, and a layer for Qi refining.
//
// It is the only place the "which row of the table am I on" question is
// answered, so the stage table below is the single source of the base numbers.
//
// The JSON tags exist because a battle's live state is persisted in the story
// journal, and a battle records the stage each combatant was actually at. The
// alternative — a mirrored struct on the story side — would be a second copy of
// the table's addressing that could drift from this one.
type Cultivation struct {
	rules *Rules
	Realm Realm `json:"realm"`
	Stage Stage `json:"stage,omitempty"`
	// Layer is 1..QiLayers while Realm is RealmQi and ignored otherwise.
	Layer int `json:"layer,omitempty"`
}

// Mortal is the starting position: no cultivation at all.
func Mortal() Cultivation { return Cultivation{Realm: RealmMortal} }

// Qi returns the given refining layer, clamped into the supported range.
func Qi(layer int) Cultivation {
	return Cultivation{Realm: RealmQi, Layer: clampInt(layer, 1, QiLayers)}
}

// RealmOrder lists the major realms from lowest to highest. It is the only
// ordering used to decide whether an attack crosses a realm boundary.
var RealmOrder = []Realm{RealmMortal, RealmQi, RealmFoundation, RealmCore, RealmNascent, RealmSpirit, RealmVoid, RealmIntegration, RealmMahayana}

// realmIndex returns the position of a realm in RealmOrder, or -1 if unknown.
func realmIndex(realm Realm) int {
	for index, candidate := range RealmOrder {
		if candidate == realm {
			return index
		}
	}
	return -1
}

// RealmGap returns how many major realms separate attacker from target:
// positive when the attacker is higher, negative when lower. Unknown realms
// report 0 rather than inventing a gap.
func RealmGap(attacker, target Realm) int {
	from, to := realmIndex(attacker), realmIndex(target)
	if from < 0 || to < 0 {
		return 0
	}
	return from - to
}

// stageIndex orders the sub-stages within a major realm.
func stageIndex(stage Stage) int {
	switch stage {
	case StageEarly:
		return 0
	case StageMiddle:
		return 1
	case StageLate:
		return 2
	}
	return -1
}

// Attributes are the six values shown directly to the player. Health and Mana
// have both a maximum and a current value; the other four are maxima only.
//
// Every value is a whole number because the player sees whole numbers: the
// rules round once, here, instead of leaving fractional values to leak into
// the interface and disagree with each other.
type Attributes struct {
	HealthMax  int `json:"health_max"`
	Health     int `json:"health"`
	ManaMax    int `json:"mana_max"`
	Mana       int `json:"mana"`
	Attack     int `json:"attack"`
	Defense    int `json:"defense"`
	Agility    int `json:"agility"`
	Perception int `json:"perception"`
}

// attributeRatios are the fractions of a stage's base health that become the
// other five attributes. They come from attribute_ratios in the parameter set.
var attributeRatios = struct {
	mana       float64
	attack     float64
	defense    float64
	agility    float64
	perception float64
}{mana: 0.8, attack: 0.2, defense: 0.1, agility: 0.1, perception: 0.1}

// mortalBase is the one row of the table that does not follow the ratios: a
// mortal has no mana and a perception of zero, and the design gives explicit
// values rather than deriving them.
var mortalBase = struct {
	health     int
	mana       int
	attack     int
	defense    int
	agility    int
	perception int
}{health: 80, mana: 0, attack: 10, defense: 5, agility: 8, perception: 0}

// realmBaseHealth maps a major realm to its early-stage base health.
var realmBaseHealth = map[Realm]int{
	RealmFoundation: 1000,
	RealmCore:       5000,
	RealmNascent:    25000,
	RealmSpirit:     125000,
	// Spirit-realm values extend the game's authored fivefold curve, not canon statistics.
	RealmVoid:        625000,
	RealmIntegration: 3125000,
	RealmMahayana:    15625000,
}

// stageAttributeScale is the health multiplier for a sub-stage: early 1.0,
// middle 1.4, late 1.9. It matches the published table exactly.
func stageAttributeScale(stage Stage) float64 {
	switch stage {
	case StageMiddle:
		return 1.4
	case StageLate:
		return 1.9
	}
	return 1.0
}

// baseHealth returns the unequipped, healthy base health for a position on the
// table. It returns 0 for an unrecognised position so a caller cannot silently
// treat a typo as a real stage.
func baseHealth(c Cultivation) int {
	switch c.Realm {
	case RealmMortal:
		return mortalBase.health
	case RealmQi:
		layer := clampInt(c.Layer, 1, QiLayers)
		// 炼气生命上限 = 100 + 20 × (层数 − 1)
		return 100 + 20*(layer-1)
	default:
		base, ok := realmBaseHealth[c.Realm]
		if !ok {
			return 0
		}
		return int(roundHalfUp(float64(base) * stageAttributeScale(c.Stage)))
	}
}

// BaseAttributes returns the six attributes for a stage with no equipment,
// no technique specialisation and full health. Current health and mana are
// set to their maxima; callers that have real current values overwrite them.
func BaseAttributes(c Cultivation) Attributes {
	if c.rules != nil {
		row, ok := c.rules.stage(c)
		if !ok {
			return Attributes{}
		}
		a := row.Base
		a.Health = a.HealthMax
		a.Mana = a.ManaMax
		return a
	}
	if c.Realm == RealmMortal {
		return Attributes{
			HealthMax: mortalBase.health, Health: mortalBase.health,
			ManaMax: mortalBase.mana, Mana: mortalBase.mana,
			Attack: mortalBase.attack, Defense: mortalBase.defense,
			Agility: mortalBase.agility, Perception: mortalBase.perception,
		}
	}
	health := baseHealth(c)
	if health == 0 {
		return Attributes{}
	}
	mana := int(roundHalfUp(float64(health) * attributeRatios.mana))
	return Attributes{
		HealthMax: health, Health: health,
		ManaMax: mana, Mana: mana,
		Attack:     int(roundHalfUp(float64(health) * attributeRatios.attack)),
		Defense:    int(roundHalfUp(float64(health) * attributeRatios.defense)),
		Agility:    int(roundHalfUp(float64(health) * attributeRatios.agility)),
		Perception: int(roundHalfUp(float64(health) * attributeRatios.perception)),
	}
}

// CanUseTechniques reports whether a character at this stage is allowed to
// perform techniques at all.
//
// A mortal has zero mana, and the rules are explicit that zero mana must not
// become free casting just because a technique's cost formula happens to
// round to zero. Eligibility is therefore a property of the stage, checked
// before any cost arithmetic.
func CanUseTechniques(c Cultivation) bool {
	return c.Realm != RealmMortal && baseHealth(c) > 0
}

// roundHalfUp rounds a non-negative value to the nearest whole number, with
// halves going up. The design specifies floor(x+0.5); this keeps that rule in
// one place so every derived number agrees.
func roundHalfUp(value float64) float64 {
	if value < 0 {
		return -math.Floor(-value + 0.5)
	}
	return math.Floor(value + 0.5)
}

// ceilNonNegative rounds a non-negative value up to a whole number, which is
// how the rules round every real cost.
func ceilNonNegative(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Ceil(value)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
