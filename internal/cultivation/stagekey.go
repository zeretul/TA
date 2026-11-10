package cultivation

import (
	"strconv"
	"strings"
)

// This file gives a position on the stage table a single stable, language
// neutral text form.
//
// The story journal has to store where a character stands, and it has to store
// it as something the rules can read back without guessing. Free prose will not
// do: the design requires that the model not be able to reword a character's
// cultivation, and a reworded value is exactly what a prose field permits. A
// canonical key makes the stored stage a value the rules either recognise or
// reject.
//
// The key is deliberately not a display name. Wording belongs to the interface
// and may be translated, abbreviated or replaced by a world pack; the key does
// not move when the wording does.

// stageKeySuffix is the sub-stage component of a key for the major realms that
// have sub-stages.
var stageKeySuffix = map[Stage]string{
	StageEarly:  "early",
	StageMiddle: "middle",
	StageLate:   "late",
}

// StageKey returns the canonical text form of a position on the stage table.
//
// It returns an empty string for a position the rules do not recognise, so a
// caller cannot store a key it will later fail to read.
func StageKey(c Cultivation) string {
	switch c.Realm {
	case RealmMortal:
		return "mortal"
	case RealmQi:
		layer := clampInt(c.Layer, 1, QiLayers)
		return "qi_" + strconv.Itoa(layer)
	case RealmFoundation, RealmCore, RealmNascent, RealmSpirit, RealmVoid, RealmIntegration, RealmMahayana:
		suffix, ok := stageKeySuffix[c.Stage]
		if !ok {
			return ""
		}
		return string(c.Realm) + "_" + suffix
	default:
		return ""
	}
}

// ParseStageKey reads a canonical key back into a position on the stage table.
//
// An unrecognised key is reported as not-found rather than silently becoming a
// mortal: a character whose recorded stage cannot be read is a real problem the
// caller must surface, not a character who quietly lost their cultivation.
func ParseStageKey(key string, rules ...*Rules) (Cultivation, bool) {
	if len(rules) > 0 && rules[0] != nil {
		c, ok := ParseStageKey(key)
		if !ok {
			return c, false
		}
		if _, found := rules[0].Stages[StageKey(c)]; !found {
			return Cultivation{}, false
		}
		return c.WithRules(rules[0]), true
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return Cultivation{}, false
	}
	if key == "mortal" {
		return Mortal(), true
	}
	if layerText, ok := strings.CutPrefix(key, "qi_"); ok {
		layer, err := strconv.Atoi(layerText)
		if err != nil || layer < 1 || layer > QiLayers {
			return Cultivation{}, false
		}
		return Qi(layer), true
	}
	for _, realm := range []Realm{RealmFoundation, RealmCore, RealmNascent, RealmSpirit, RealmVoid, RealmIntegration, RealmMahayana} {
		rest, ok := strings.CutPrefix(key, string(realm)+"_")
		if !ok {
			continue
		}
		for stage, suffix := range stageKeySuffix {
			if rest == suffix {
				return Cultivation{Realm: realm, Stage: stage}, true
			}
		}
		return Cultivation{}, false
	}
	return Cultivation{}, false
}

// StageKeys lists every canonical key the rules support, in ascending order.
//
// This is the set a story may legitimately record. A schema that offers the
// player a stage therefore offers exactly this list, so the rules and the
// journal cannot disagree about which stages exist.
func StageKeys() []string {
	keys := make([]string, 0, 1+QiLayers+(len(RealmOrder)-2)*len(stageKeySuffix))
	keys = append(keys, StageKey(Mortal()))
	for layer := 1; layer <= QiLayers; layer++ {
		keys = append(keys, StageKey(Qi(layer)))
	}
	for _, realm := range []Realm{RealmFoundation, RealmCore, RealmNascent, RealmSpirit, RealmVoid, RealmIntegration, RealmMahayana} {
		for _, stage := range []Stage{StageEarly, StageMiddle, StageLate} {
			keys = append(keys, StageKey(Cultivation{Realm: realm, Stage: stage}))
		}
	}
	return keys
}
