package cultivation

import "strings"

// InnateTalent is one optional trait a character may be born with.
//
// The table below is the rules' own, not the pack's: a pack offers these to its
// players, but what a talent is worth is fixed here so a client cannot send its
// own percentage and an authored pack cannot quietly price one talent above
// another. The size is small on purpose — a talent is a starting difference, not
// a second progression system, and it enters the same §3.4 aggregation under the
// same caps as equipment and pills.
type InnateTalent struct {
	ID        string
	Name      string
	Attribute AttributeName
	// Value is the signed fraction the aggregation applies (+0.05 is 5%).
	Value float64
}

// innateTalents is the published list of 天赋特性.
//
// One attribute each, and every one of them is a real field the settlement
// derives, so a talent never claims a number the rules do not display.
var innateTalents = []InnateTalent{
	{ID: "strong_body", Name: "体魄强健", Attribute: AttrHealthMax, Value: 0.05},
	{ID: "deep_breath", Name: "气脉绵长", Attribute: AttrManaMax, Value: 0.05},
	{ID: "keen_sense", Name: "眼明心细", Attribute: AttrPerception, Value: 0.05},
	{ID: "light_step", Name: "身轻步快", Attribute: AttrAgility, Value: 0.05},
	{ID: "sharp_strike", Name: "出手凌厉", Attribute: AttrAttack, Value: 0.05},
	{ID: "tough_frame", Name: "筋骨坚实", Attribute: AttrDefense, Value: 0.05},
}

// InnateTalents lists every talent the rules know, in display order.
func InnateTalents(rules ...*Rules) []InnateTalent {
	if len(rules) > 0 && rules[0] != nil {
		return append([]InnateTalent(nil), rules[0].Talents...)
	}
	out := make([]InnateTalent, len(innateTalents))
	copy(out, innateTalents)
	return out
}

// InnateTalentByID returns one talent by its stable id.
func InnateTalentByID(id string, rules ...*Rules) (InnateTalent, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return InnateTalent{}, false
	}
	for _, talent := range InnateTalents(rules...) {
		if talent.ID == id {
			return talent, true
		}
	}
	return InnateTalent{}, false
}

// InnateTalentByName returns one talent by the name the player was shown.
//
// The character's state records the name rather than the id because that field
// is read by the player in the character panel, where a stable id would be a
// code word. The name is part of the rules' table, so it is just as fixed as
// the id; this lookup exists so a save written with either form still resolves.
func InnateTalentByName(name string, rules ...*Rules) (InnateTalent, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return InnateTalent{}, false
	}
	for _, talent := range InnateTalents(rules...) {
		if talent.Name == name {
			return talent, true
		}
	}
	return InnateTalent{}, false
}

// InnateTalentModifier turns a talent into the modifier the aggregation reads.
//
// The id is prefixed so a breakdown names the talent rather than an attribute,
// and Kind is its own source: a talent is neither an equipped copy nor a story
// reward, and a breakdown that named one as the other would tell the player their
// birth granted what their sword granted.
func InnateTalentModifier(talent InnateTalent) AttributeModifier {
	return AttributeModifier{
		ID:        "talent:" + talent.ID,
		Kind:      SourceInnateTalent,
		Attribute: talent.Attribute,
		Value:     talent.Value,
	}
}
