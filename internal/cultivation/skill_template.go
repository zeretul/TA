package cultivation

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file makes §6 of 《自制-技能与施展》 expressible: a generated technique.
//
// The design allows the model to propose a concrete technique from a template the
// rules have already validated, and forbids it everything else:
//
//	第一版AI只能给批准模板填名称、属性、表现和合法来源，选择已经明确验证的变体，
//	不能自由改倍率、消耗、负担和控制叠加。
//	技能品级与稀有度须匹配该模板许可，不由模型改标签后自动增加预算。
//
// So a template is a whole numeric definition plus the grade and rarity the
// content document publishes for it. Instantiating one copies every number, sets
// a name and an identity, and runs the same validation a hand-authored definition
// faces. There is no field here a caller could use to raise a multiplier.
//
// The template set is deliberately the built-in table rather than a second
// catalogue: §7.2 requires that only a template whose effect implementation has
// passed acceptance may enter the model's selectable set, and the built-ins are
// the definitions the engine actually settles.

// SkillTemplate is one approved numeric template a generated technique may be
// built from.
//
// Quality and Rarity are the pair the content document publishes for this
// template. They are the only pair a generated technique may carry, which is what
// stops "把普通模板改称独特" from buying headroom the template never had.
type SkillTemplate struct {
	// Definition is the validated numeric definition every instance copies.
	Definition SkillDef
	// Quality and Rarity are the template's published grade and acquisition tier.
	Quality TechniqueQuality
	Rarity  ItemRarity
}

// skillTemplateOrder fixes the listing order so the model-facing guidance and the
// refusal message name the same set in the same order every run.
var skillTemplateOrder = append([]string{
	"fire_bolt", "condensed_blade", "heavy_thunder",
	"flame_wave", "shield", "meridian_secret_blade",
}, highRealmSkillIDs()...)

// skillTemplateTable is the selectable template set.
//
// It is built from skillTable so a template cannot drift from the definition the
// engine settles: there is one set of numbers, and the template only adds the
// published grade and rarity.
var skillTemplateTable = buildSkillTemplateTable()

func buildSkillTemplateTable() map[string]SkillTemplate {
	table := make(map[string]SkillTemplate, len(skillTemplateOrder))
	for _, template := range []struct {
		id      string
		quality TechniqueQuality
		rarity  ItemRarity
	}{
		// §5.2 of 《自制-功法与技能内容》 publishes these pairs. 寻常出手 is the
		// ordinary attack and is deliberately absent: it is not a technique anyone
		// is taught, so it is never a generation target.
		{id: "fire_bolt", quality: QualityOrdinary, rarity: RarityCommon},
		{id: "condensed_blade", quality: QualityGood, rarity: RarityUncommon},
		{id: "heavy_thunder", quality: QualityAdvanced, rarity: RarityRare},
		{id: "flame_wave", quality: QualityGood, rarity: RarityUncommon},
		{id: "shield", quality: QualityOrdinary, rarity: RarityCommon},
		{id: "meridian_secret_blade", quality: QualityAdvanced, rarity: RarityPrecious},
	} {
		definition, ok := SkillByID(template.id)
		if !ok {
			continue
		}
		table[template.id] = SkillTemplate{
			Definition: definition, Quality: template.quality, Rarity: template.rarity,
		}
	}
	for _, definition := range highRealmSkills {
		table[definition.ID] = SkillTemplate{Definition: definition, Quality: QualityAdvanced, Rarity: RarityRare}
	}
	return table
}

// SkillTemplates lists the selectable templates in a stable order.
func SkillTemplates(profiles ...*Rules) []SkillTemplate {
	if len(profiles) > 0 && profiles[0] != nil {
		return append([]SkillTemplate(nil), profiles[0].SkillTemplates...)
	}
	out := make([]SkillTemplate, 0, len(skillTemplateOrder))
	for _, id := range skillTemplateOrder {
		if template, ok := skillTemplateTable[id]; ok {
			out = append(out, template)
		}
	}
	return out
}

// SkillTemplateByID looks up one selectable template.
func SkillTemplateByID(id string, profiles ...*Rules) (SkillTemplate, bool) {
	if len(profiles) > 0 && profiles[0] != nil {
		for _, t := range profiles[0].SkillTemplates {
			if t.Definition.ID == strings.TrimSpace(id) {
				return t, true
			}
		}
		return SkillTemplate{}, false
	}
	template, ok := skillTemplateTable[strings.TrimSpace(id)]
	return template, ok
}

// SupportsGrade reports whether a grade and rarity pair is the pair the template
// publishes.
//
// An empty pair means "use the template's own", which is the common case. A pair
// that names anything else is refused rather than corrected: silently replacing
// it would let a caller believe a 独特 technique had been granted while the rules
// paid the ordinary one, and correcting it upward is exactly what the design
// forbids.
func (t SkillTemplate) SupportsGrade(quality TechniqueQuality, rarity ItemRarity) bool {
	if quality != "" && quality != t.Quality {
		return false
	}
	if rarity != "" && rarity != t.Rarity {
		return false
	}
	return true
}

// GeneratedSkillPrefix marks a definition generated inside a story.
const GeneratedSkillPrefix = "gen."

// IsGeneratedSkillID reports whether an identifier could name a definition
// generated inside a story.
//
// It is a shape check, not a proof of existence: whether this story registered the
// definition is settled by that story's own registry. It exists so a caller that
// has no story in scope can still tell a name that could never resolve from one
// that must be looked up where the story's definitions live.
func IsGeneratedSkillID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), GeneratedSkillPrefix)
}

// GeneratedSkillID is the stable identity of a technique generated from a
// template under a given name.
//
// It embeds the template so two worlds cannot confuse a fire template's
// technique with a blade template's, and the name so a learned record, a
// committed round and the interface all read the same identity. Deriving it
// rather than accepting one is what keeps a generation from claiming an ID that
// already belongs to a published definition.
func GeneratedSkillID(templateID, name string) string {
	templateID = strings.TrimSpace(templateID)
	name = strings.TrimSpace(name)
	if templateID == "" || name == "" {
		return ""
	}
	return GeneratedSkillPrefix + templateID + "." + name
}

// Instantiate builds the definition a generated technique registers.
//
// Every number comes from the template. Only the identity, the name and the
// published form are supplied here, and the result faces ValidateSkillDefinition
// exactly as a hand-authored definition does.
func (t SkillTemplate) Instantiate(name string) (SkillDef, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SkillDef{}, ErrSkillGeneratedMissingName
	}
	if err := validateGeneratedSkillName(name); err != nil {
		return SkillDef{}, err
	}
	definition := t.Definition
	definition.ID = GeneratedSkillID(t.Definition.ID, name)
	if _, published := SkillByID(definition.ID); published {
		return SkillDef{}, ErrSkillGeneratedIDConflict
	}
	// A generated technique is registered at the first version of its own
	// identity. It never overwrites a published definition, because its ID cannot
	// equal one: every published ID is a bare key and every generated one starts
	// with gen.
	definition.Version = 1
	// No localisation key: the wording is the name the story supplied, and
	// inventing a key would make the interface look for a translation that does
	// not exist.
	definition.NameKey = ""
	definition.Name = name
	if err := ValidateSkillDefinition(definition); err != nil {
		return SkillDef{}, err
	}
	return definition, nil
}

// validateGeneratedSkillName refuses a name that could not be a stable identity.
//
// The name is part of the generated definition's ID, so a control character or an
// absurd length would put an unusable key in the journal and in every learned
// record that references it.
func validateGeneratedSkillName(name string) error {
	if !utf8.ValidString(name) {
		return ErrSkillGeneratedInvalidName
	}
	if utf8.RuneCountInString(name) > maxGeneratedSkillNameRunes {
		return ErrSkillGeneratedInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return ErrSkillGeneratedInvalidName
		}
	}
	return nil
}

// maxGeneratedSkillNameRunes bounds a generated technique's name. The bound is a
// journal-safety limit rather than a style rule: the name is part of the ID, and
// every learned record, committed round and window entry repeats it.
const maxGeneratedSkillNameRunes = 40
