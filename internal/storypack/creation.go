package storypack

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
)

// PackCreation is a pack's authored options for the start flow's character
// creation page.
//
// It is authoring metadata, not world content: it is deliberately excluded from
// ContentHash, so adding or editing an option does not move a running story to
// a new lore scope and does not invalidate the task, event and realm
// definitions that declare the pack's content revision. It describes what a
// player may choose. Native adventure options resolve by ID to loadouts inside
// the hashed, frozen AdventureRules; their numbers do not live here. The existing
// cultivation aptitude and talent keep their existing consumers. Other choices
// remain narrative unless explicitly marked as a real starting loadout.
type PackCreation struct {
	// These IDs resolve to numeric loadouts in the frozen AdventureRules.
	AdventureClasses []CreationOption `json:"adventure_classes,omitempty"`
	AdventureWeapons []CreationOption `json:"adventure_weapons,omitempty"`
	AdventureMethods []CreationOption `json:"adventure_methods,omitempty"`
	// AgeRange is the inclusive range the server rolls the age from, once.
	AgeRange [2]int `json:"age_range"`
	// Genders are the offered choices. The first one marked default is the
	// preselection; the rules store gender only as prose.
	Genders []CreationOption `json:"genders,omitempty"`
	// Origins are the social origins. Several may be combined by the player's
	// own wording; each names the template whose initial state it starts from.
	Origins []CreationOrigin `json:"origins,omitempty"`
	// StateBasisTemplates are the templates a custom origin may start from.
	StateBasisTemplates []string `json:"state_basis_templates,omitempty"`
	// Optional author restrictions retain every template and ordinary origin.
	// Visibility affects the picker only; custom bases restrict only custom origins.
	VisibleOriginIDs          []string `json:"visible_origin_ids,omitempty"`
	CustomStateBasisTemplates []string `json:"custom_state_basis_templates,omitempty"`
	CustomLocationPolicy      string   `json:"custom_location_policy,omitempty"`
	CustomBackgroundRule      string   `json:"custom_background_rule,omitempty"`
	// Affiliations are the sects and families a character may belong to. "auto"
	// lets the draft choose within the ordinary range.
	Affiliations []CreationOption `json:"affiliations,omitempty"`
	// Locations are the places a character may start at. "auto" lets the draft
	// choose inside the chosen affiliation's permitted living range.
	Locations []CreationOption `json:"locations,omitempty"`
	// RootCategories and RootAttributes describe a spiritual root. They are
	// narrative: no rule computes them.
	RootCategories []CreationOption `json:"root_categories,omitempty"`
	RootAttributes []CreationOption `json:"root_attributes,omitempty"`
	// VariantRoots are the mutated roots (风／雷／冰), offered as a single
	// choice. A character has either an ordinary root or a variant one, never
	// both: the two answer the same question, and mixing them would describe a
	// root the world does not have. Like the ordinary root, a variant is
	// narrative — this round grants it no realm, technique or training rate.
	VariantRoots []CreationOption `json:"variant_roots,omitempty"`
	// Aptitudes are the five tiers the cultivation rules really read.
	Aptitudes []CreationAptitude `json:"aptitudes,omitempty"`
	// WeaponDirections and Traits are narrative or content-tendency only. They are
	// never turned into a number here.
	WeaponDirections []CreationOption `json:"weapon_directions,omitempty"`
	Traits           []CreationOption `json:"traits,omitempty"`
	// Talents are the optional 天赋特性: at most one, and the only creation
	// choice besides the aptitude whose size is fixed by the rules. The pack
	// names the options; internal/cultivation/innate_talent.go states what each
	// is worth, so a client cannot send a percentage and an authored pack cannot
	// price one talent above another.
	Talents []CreationOption `json:"talents,omitempty"`
	// EffectNotes says, per group, what the choice actually does. Values are
	// real_cultivation_talent, real_attribute, narrative or content_tendency.
	EffectNotes map[string]string `json:"effect_notes,omitempty"`
}

// CreationOption is one labelled choice. Country, Kind and FactionID are set
// only on options derived from the world catalog.
type CreationOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
	Country     string `json:"country,omitempty"`
	Kind        string `json:"kind,omitempty"`
	FactionID   string `json:"faction_id,omitempty"`
}

// CreationOrigin is one social origin. Its InitialState is the state a start
// from this origin really commits, derived from the template by the authoring
// generator so the preview cannot promise a realm or a purse the start does not
// write.
type CreationOrigin struct {
	CreationOption
	OpeningTier       string                `json:"opening_tier,omitempty"`
	TemplateID        string                `json:"template_id,omitempty"`
	DefaultTemplateID string                `json:"default_template_id,omitempty"`
	DefaultLocationID string                `json:"default_location_id,omitempty"`
	InitialState      *CreationInitialState `json:"initial_state,omitempty"`
	// AffiliationKind is the kind of affiliation this origin can belong to:
	// "none" for a loose cultivator, "sect" for a sect disciple, "family" for a
	// family heir. Empty means the origin does not constrain the affiliation,
	// which is what a custom origin needs.
	AffiliationKind string `json:"affiliation_kind,omitempty"`
}

// CreationAuthoredField is an exact, display-only initial fact. It does not
// deserialize a legacy family marker as an executable gameplay system.
type CreationAuthoredField struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// CreationInitialState is what the pack seeds, stated in the preview's terms.
type CreationInitialState struct {
	AuthoredFields []CreationAuthoredField `json:"authored_fields,omitempty"`
	// Family and Rank describe native adventure progression; a missing family
	// keeps cultivation previews on their existing realm-based presentation.
	Family        string   `json:"family,omitempty"`
	Rank          int      `json:"rank,omitempty"`
	CurrencyLabel string   `json:"currency_label,omitempty"`
	Realm         string   `json:"realm,omitempty"`
	RealmLabel    string   `json:"realm_label,omitempty"`
	Currency      int      `json:"currency"`
	Items         []string `json:"items,omitempty"`
	Techniques    []string `json:"techniques,omitempty"`
	Equipped      []string `json:"equipped,omitempty"`
}

// CreationAptitude is one real aptitude tier. Talent is the value the
// cultivation rules use; Name is the wording normalizeCultivationTalentText
// matches, so the composed foundation text and the rules agree by construction.
type CreationAptitude struct {
	CreationOption
	Talent string `json:"talent"`
}

// creationArity is how many root attributes a category takes. A zero arity is
// not constrained here; the caller reads the category itself.
func (c *PackCreation) rootArity(categoryID string) (min, max int, ok bool) {
	switch strings.TrimSpace(categoryID) {
	case "single":
		return 1, 1, true
	case "dual":
		return 2, 2, true
	case "triple":
		return 3, 3, true
	case "quad":
		return 4, 4, true
	case "quinary":
		return 5, 5, true
	case "pseudo":
		return 1, 2, true
	case "waste":
		return 0, 0, true
	}
	return 0, 0, false
}

// Gender returns one gender option by ID.
func (c *PackCreation) Gender(id string) (CreationOption, bool) {
	return optionByID(c.Genders, id)
}

// Origin returns one origin by ID.
func (c *PackCreation) Origin(id string) (CreationOrigin, bool) {
	id = strings.TrimSpace(id)
	for _, origin := range c.Origins {
		if strings.TrimSpace(origin.ID) == id {
			return origin, true
		}
	}
	return CreationOrigin{}, false
}

// Affiliation returns one affiliation by ID.
func (c *PackCreation) Affiliation(id string) (CreationOption, bool) {
	return optionByID(c.Affiliations, id)
}

// Location returns one location by ID.
func (c *PackCreation) Location(id string) (CreationOption, bool) {
	return optionByID(c.Locations, id)
}

// RootCategory returns one root category by ID.
func (c *PackCreation) RootCategory(id string) (CreationOption, bool) {
	return optionByID(c.RootCategories, id)
}

// RootAttribute returns one root attribute by ID.
func (c *PackCreation) RootAttribute(id string) (CreationOption, bool) {
	return optionByID(c.RootAttributes, id)
}

// Aptitude returns one aptitude by ID.
func (c *PackCreation) Aptitude(id string) (CreationAptitude, bool) {
	id = strings.TrimSpace(id)
	for _, aptitude := range c.Aptitudes {
		if strings.TrimSpace(aptitude.ID) == id {
			return aptitude, true
		}
	}
	return CreationAptitude{}, false
}

// WeaponDirection and Trait return one narrative option by ID.
func (c *PackCreation) WeaponDirection(id string) (CreationOption, bool) {
	return optionByID(c.WeaponDirections, id)
}

func (c *PackCreation) Trait(id string) (CreationOption, bool) {
	return optionByID(c.Traits, id)
}

// VariantRoot returns one mutated root (风／雷／冰) by ID.
func (c *PackCreation) VariantRoot(id string) (CreationOption, bool) {
	return optionByID(c.VariantRoots, id)
}

// Talent returns one optional 天赋特性 by ID.
func (c *PackCreation) Talent(id string) (CreationOption, bool) {
	return optionByID(c.Talents, id)
}

// DefaultOption returns the option marked default, or the first one when none
// is marked. A pack with no options returns false.
func DefaultOption(options []CreationOption) (CreationOption, bool) {
	if len(options) == 0 {
		return CreationOption{}, false
	}
	for _, option := range options {
		if option.Default {
			return option, true
		}
	}
	return options[0], true
}

func optionByID(options []CreationOption, id string) (CreationOption, bool) {
	id = strings.TrimSpace(id)
	for _, option := range options {
		if strings.TrimSpace(option.ID) == id {
			return option, true
		}
	}
	return CreationOption{}, false
}

// hasAffiliationKind reports whether the pack offers any affiliation of this
// kind. The "own" option counts as a family: its name is derived at resolve
// time, so it is authored with the family kind.
func hasAffiliationKind(options []CreationOption, kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, option := range options {
		if strings.TrimSpace(option.Kind) == kind {
			return true
		}
	}
	return false
}

// validateCreation checks the options a pack declares.
//
// It is deliberately shallow: the block is optional, and a pack without one
// simply offers no creation page. What it does check is what would otherwise
// fail later with a worse message — duplicate IDs, an origin pointing at a
// template that does not exist, and an age range no roll could satisfy.
func (p Pack) validateCreation() error {
	c := p.Creation
	if c == nil {
		return nil
	}
	if c.AgeRange[0] <= 0 || c.AgeRange[1] < c.AgeRange[0] {
		return fmt.Errorf("建角年龄范围无效: %d–%d", c.AgeRange[0], c.AgeRange[1])
	}
	for name, options := range map[string][]CreationOption{
		"genders":           c.Genders,
		"affiliations":      c.Affiliations,
		"locations":         c.Locations,
		"root_categories":   c.RootCategories,
		"root_attributes":   c.RootAttributes,
		"variant_roots":     c.VariantRoots,
		"weapon_directions": c.WeaponDirections,
		"traits":            c.Traits,
		"talents":           c.Talents,
	} {
		seen := map[string]bool{}
		for _, option := range options {
			id := strings.TrimSpace(option.ID)
			if id == "" {
				return fmt.Errorf("建角选项缺少 id: %s", name)
			}
			if seen[id] {
				return fmt.Errorf("建角选项 id 重复: %s=%s", name, id)
			}
			seen[id] = true
			if strings.TrimSpace(option.Name) == "" {
				return fmt.Errorf("建角选项缺少 name: %s=%s", name, id)
			}
		}
	}
	if err := p.validateCreationRestrictions(); err != nil {
		return err
	}
	if err := p.validateAdventureCreation(); err != nil {
		return err
	}
	seenOrigins := map[string]bool{}
	for _, origin := range c.Origins {
		id := strings.TrimSpace(origin.ID)
		if id == "" {
			return fmt.Errorf("建角出身缺少 id")
		}
		if seenOrigins[id] {
			return fmt.Errorf("建角出身 id 重复: %s", id)
		}
		seenOrigins[id] = true
		if strings.TrimSpace(origin.Name) == "" {
			return fmt.Errorf("建角出身缺少 name: %s", id)
		}
		if templateID := strings.TrimSpace(origin.TemplateID); templateID != "" {
			if _, ok := p.ProtagonistTemplate(templateID); !ok {
				return fmt.Errorf("建角出身 %s 指向不存在的快捷模板: %s", id, templateID)
			}
		}
		if templateID := strings.TrimSpace(origin.DefaultTemplateID); templateID != "" {
			if _, ok := p.ProtagonistTemplate(templateID); !ok {
				return fmt.Errorf("建角出身 %s 指向不存在的默认模板: %s", id, templateID)
			}
		}
		// An origin that names the kind of affiliation it can belong to needs at
		// least one affiliation of that kind, or the page would offer a choice
		// that no option can satisfy.
		if kind := strings.TrimSpace(origin.AffiliationKind); kind != "" {
			if !hasAffiliationKind(c.Affiliations, kind) {
				return fmt.Errorf("建角出身 %s 需要 %s 类型的所属，但世界包没有提供", id, kind)
			}
		}
	}
	seenAptitudes := map[string]bool{}
	for _, aptitude := range c.Aptitudes {
		id := strings.TrimSpace(aptitude.ID)
		if id == "" {
			return fmt.Errorf("建角资质缺少 id")
		}
		if seenAptitudes[id] {
			return fmt.Errorf("建角资质 id 重复: %s", id)
		}
		seenAptitudes[id] = true
		if strings.TrimSpace(aptitude.Name) == "" || strings.TrimSpace(aptitude.Talent) == "" {
			return fmt.Errorf("建角资质缺少名称或对应档位: %s", id)
		}
	}
	for _, templateID := range c.StateBasisTemplates {
		if _, ok := p.ProtagonistTemplate(strings.TrimSpace(templateID)); !ok {
			return fmt.Errorf("建角初态模板不存在: %s", templateID)
		}
	}
	// A talent's size is fixed by the rules, so a pack may only offer talents the
	// rules know. An unknown id would otherwise be offered to the player and then
	// contribute nothing at settlement — a choice that silently does nothing.
	for _, talent := range c.Talents {
		if _, ok := cultivation.InnateTalentByID(talent.ID, p.Story.Gameplay.CultivationRules); !ok {
			return fmt.Errorf("建角天赋 %s 不是规则支持的天赋特性", strings.TrimSpace(talent.ID))
		}
	}
	return nil
}
