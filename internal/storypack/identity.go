package storypack

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// IdentityChoice is what the creation page sends: the options the player picked,
// plus the prose the model drafted and the player confirmed.
//
// It is deliberately all ids and text. Nothing numeric, no realm, no purse, no
// carried thing: those are the pack's initial state, and the server reads them
// from the pack. A client that sends them is ignored rather than trusted.
type IdentityChoice struct {
	AdventureClassID  string `json:"adventure_class_id,omitempty"`
	AdventureWeaponID string `json:"adventure_weapon_id,omitempty"`
	AdventureMethodID string `json:"adventure_method_id,omitempty"`
	Name              string `json:"name"`
	Gender            string `json:"gender"`
	// Age is the roll the page was shown. Zero means "roll one now"; a
	// non-zero value is the locked roll the player already saw, echoed back so
	// a retry or a rewrite cannot silently reroll it.
	Age int `json:"age,omitempty"`
	// RerollAge is the one explicit way to ask for a new roll.
	RerollAge bool `json:"reroll_age,omitempty"`

	OriginID   string `json:"origin_id"`
	OriginText string `json:"origin_text,omitempty"`
	Locale     string `json:"-"`
	// StateBasisTemplateID is the ready-made initial state a custom origin
	// starts from. Empty means the origin's own template.
	StateBasisTemplateID string `json:"state_basis_template_id,omitempty"`

	AffiliationText string `json:"affiliation_text,omitempty"`
	LocationText    string `json:"location_text,omitempty"`
	OpeningMode     string `json:"opening_mode,omitempty"`
	AffiliationID   string `json:"affiliation_id"`
	LocationID      string `json:"location_id"`

	RootCategoryID string   `json:"root_category_id"`
	RootAttributes []string `json:"root_attributes,omitempty"`
	// VariantRootID is the mutated root (风／雷／冰) the player picked instead of
	// an ordinary one. A character has one or the other, never both.
	VariantRootID string   `json:"variant_root_id,omitempty"`
	AptitudeID    string   `json:"aptitude_id"`
	WeaponID      string   `json:"weapon_direction_id,omitempty"`
	TraitIDs      []string `json:"trait_ids,omitempty"`
	// TalentID is the optional 天赋特性. Empty means the player took none, which
	// is a real answer rather than a missing one.
	TalentID string `json:"talent_id,omitempty"`

	// The drafted prose. Only these fields travel from the model's draft into
	// the story; every mechanical value is re-derived below.
	Identity    string   `json:"identity,omitempty"`
	Appearance  string   `json:"appearance,omitempty"`
	Background  string   `json:"background,omitempty"`
	Situation   string   `json:"situation,omitempty"`
	Predicament string   `json:"predicament,omitempty"`
	Contacts    []string `json:"contacts,omitempty"`
	Opening     string   `json:"opening,omitempty"`
}

// Identity is a confirmed character, resolved against one pack.
//
// Every field a rule reads is derived here from the pack: the initial state comes
// from a template, and the aptitude token in Foundation is the one the rules
// normalize. The prose fields are the player's confirmed text and grant nothing
// on their own.
type Identity struct {
	AdventureClassID    string `json:"adventure_class_id,omitempty"`
	AdventureClassName  string `json:"adventure_class_name,omitempty"`
	AdventureWeaponID   string `json:"adventure_weapon_id,omitempty"`
	AdventureWeaponName string `json:"adventure_weapon_name,omitempty"`
	AdventureMethodID   string `json:"adventure_method_id,omitempty"`
	AdventureMethodName string `json:"adventure_method_name,omitempty"`
	// Server-only resolved state, never accepted from a creation request.
	AdventureState *interactive.AdventureActor `json:"-"`
	Name           string                      `json:"name"`
	Gender         string                      `json:"gender"`
	GenderName     string                      `json:"gender_name"`
	Age            int                         `json:"age"`

	OriginID   string `json:"origin_id"`
	OriginName string `json:"origin_name"`
	OriginText string `json:"origin_text"`

	TemplateID string `json:"template_id"`

	AffiliationID      string `json:"affiliation_id"`
	AffiliationName    string `json:"affiliation_name"`
	AffiliationCountry string `json:"affiliation_country,omitempty"`
	LocationID         string `json:"location_id"`
	LocationName       string `json:"location_name"`
	LocationCountry    string `json:"location_country,omitempty"`

	RootCategoryID   string   `json:"root_category_id"`
	RootCategoryName string   `json:"root_category_name"`
	RootAttributes   []string `json:"root_attributes,omitempty"`
	VariantRootID    string   `json:"variant_root_id,omitempty"`
	VariantRootName  string   `json:"variant_root_name,omitempty"`
	AptitudeID       string   `json:"aptitude_id"`
	AptitudeName     string   `json:"aptitude_name"`
	Talent           string   `json:"talent"`
	WeaponID         string   `json:"weapon_direction_id,omitempty"`
	WeaponName       string   `json:"weapon_direction_name,omitempty"`
	TraitIDs         []string `json:"trait_ids,omitempty"`
	TraitNames       []string `json:"trait_names,omitempty"`
	// TalentID and TalentName are the optional 天赋特性 the character was born
	// with. TalentBonus states what it is worth in the player's terms; the size
	// itself is the rules' (internal/cultivation/innate_talent.go), never the
	// pack's and never the client's.
	TalentID    string `json:"talent_id,omitempty"`
	TalentName  string `json:"talent_name,omitempty"`
	TalentBonus string `json:"talent_bonus,omitempty"`

	// Foundation is the composed 灵根与特殊体质 text. It is built here, not
	// accepted from the client, so the aptitude the rules read and the aptitude
	// the page offered are the same word by construction.
	Foundation string `json:"foundation"`

	IdentityText string   `json:"identity_text,omitempty"`
	Appearance   string   `json:"appearance,omitempty"`
	Background   string   `json:"background,omitempty"`
	Situation    string   `json:"situation,omitempty"`
	Predicament  string   `json:"predicament,omitempty"`
	Contacts     []string `json:"contacts,omitempty"`
	Opening      string   `json:"opening,omitempty"`

	InitialState *CreationInitialState `json:"initial_state,omitempty"`

	// Profile is the protagonist snapshot's editable prose, and Premise is the
	// opening instruction's premise. Both are composed from the fields above so
	// the first scene starts from the same character the preview showed.
	Profile string `json:"profile"`
	Premise string `json:"premise"`
}

const (
	identityNameMaxRunes      = 24
	identityOriginMaxRunes    = 120
	identityProseMaxRunes     = 600
	identityContactMaxRunes   = 120
	identityMaxContacts       = 4
	identityMaxTraits         = 3
	identityMaxRootAttrs      = 5
	identityDefaultLocationID = "yue.market"
)

// CreationReady reports whether this pack offers a creation page at all.
func (p Pack) CreationReady() bool {
	return p.Creation != nil && len(p.Creation.Origins) > 0
}

// RollAge returns the one random age the creation page shows, from the pack's
// declared range.
func (p Pack) RollAge(roll int) int {
	if p.Creation == nil {
		return 0
	}
	low, high := p.Creation.AgeRange[0], p.Creation.AgeRange[1]
	if high < low {
		low, high = high, low
	}
	span := high - low + 1
	if span <= 0 {
		return low
	}
	if roll < 0 {
		roll = -roll
	}
	return low + roll%span
}

// ResolveIdentity validates the player's choices against the pack and derives
// every value a rule reads.
//
// The errors are concrete on purpose: the design asks the page to point at the
// one conflict and keep the rest of the input, not to reject the whole page.
func (p Pack) ResolveIdentity(choice IdentityChoice) (Identity, error) {
	c := p.Creation
	if c == nil {
		return Identity{}, identityError("api.storyPack.identity.notReady", "pack", p.ID)
	}
	identity := Identity{}

	name := strings.TrimSpace(choice.Name)
	if name == "" {
		return Identity{}, identityError("api.storyPack.identity.nameRequired")
	}
	if utf8.RuneCountInString(name) > identityNameMaxRunes {
		return Identity{}, identityError("api.storyPack.identity.nameTooLong", "max", identityNameMaxRunes)
	}
	identity.Name = name

	gender, ok := c.Gender(choice.Gender)
	if !ok {
		gender, ok = DefaultOption(c.Genders)
		if !ok {
			return Identity{}, identityError("api.storyPack.identity.genderMissing")
		}
	}
	identity.Gender = gender.ID
	identity.GenderName = gender.Name

	if choice.Age <= 0 {
		return Identity{}, identityError("api.storyPack.identity.ageMissing")
	}
	if choice.Age < c.AgeRange[0] || choice.Age > c.AgeRange[1] {
		return Identity{}, identityError("api.storyPack.identity.ageOutOfRange",
			"age", choice.Age, "min", c.AgeRange[0], "max", c.AgeRange[1])
	}
	identity.Age = choice.Age

	origin, ok := c.Origin(choice.OriginID)
	if !ok {
		return Identity{}, identityError("api.storyPack.identity.originUnknown", "id", strings.TrimSpace(choice.OriginID))
	}
	identity.OriginID = origin.ID
	identity.OriginName = origin.Name

	// The state basis is resolved before the affiliation because a custom origin
	// states no affiliation kind of its own: what it does state is the basis it
	// starts from, and that basis is the same one an authored origin uses.
	templateID := strings.TrimSpace(choice.StateBasisTemplateID)
	if templateID == "" {
		templateID = strings.TrimSpace(origin.TemplateID)
	}
	if templateID == "" {
		templateID = strings.TrimSpace(origin.DefaultTemplateID)
	}
	if templateID == "" {
		return Identity{}, identityError("api.storyPack.identity.originStateMissing", "origin", origin.Name)
	}

	affiliation, err := p.identityAffiliation(choice, origin, templateID, name)
	if err != nil {
		return Identity{}, err
	}
	if err := p.checkOriginAffiliation(origin, affiliation, templateID); err != nil {
		return Identity{}, err
	}
	identity.AffiliationID = affiliation.ID
	identity.AffiliationName = affiliation.Name
	identity.AffiliationCountry = affiliation.Country

	if origin.ID == "custom" && len(c.CustomStateBasisTemplates) > 0 && !creationHasID(c.CustomStateBasisTemplates, templateID) {
		return Identity{}, identityError("api.storyPack.identity.originTemplateMismatch", "origin", origin.Name, "template", templateID)
	}
	if !p.isStateBasisTemplate(templateID) {
		return Identity{}, identityError("api.storyPack.identity.originTemplateMismatch",
			"origin", origin.Name, "template", templateID)
	}
	if _, ok := p.ProtagonistTemplate(templateID); !ok {
		return Identity{}, identityError("api.storyPack.identity.templateMissing", "template", templateID)
	}
	identity.TemplateID = templateID
	if template, ok := p.ProtagonistTemplate(templateID); ok {
		identity.InitialState, err = p.identityInitialState(template)
		if err != nil {
			return Identity{}, identityError("api.storyPack.identity.originStateMissing", "origin", origin.Name)
		}
	}
	if identity.InitialState != nil && identity.InitialState.Family == "" && strings.TrimSpace(identity.InitialState.RealmLabel) == "" {
		// The template state carries the rule's realm id (qi_6), not the wording
		// a player reads. The pack's creation block states the label per origin;
		// take it from the origin that uses this template.
		identity.InitialState.RealmLabel = p.creationRealmLabelFor(templateID)
	}

	location, err := p.identityLocation(choice, affiliation)
	if err != nil {
		return Identity{}, err
	}
	identity.LocationID = location.ID
	identity.LocationName = location.Name
	identity.LocationCountry = location.Country
	if err := identityCheckAffiliationLocation(affiliation, location); err != nil {
		return Identity{}, err
	}

	originText := strings.TrimSpace(choice.OriginText)
	// Limit player-supplied text, not the pack's authored description. The
	// creation page leaves this empty for authored choices, whose descriptions
	// can legitimately be longer than the custom-input field allows.
	if utf8.RuneCountInString(originText) > identityOriginMaxRunes {
		return Identity{}, identityError("api.storyPack.identity.originTooLong", "max", identityOriginMaxRunes)
	}
	if originText == "" {
		// An authored origin may refer to the sect or family the player picked.
		// Substituting the resolved name keeps the copy from naming a sect this
		// character did not join.
		originText = applyOriginPlaceholders(origin.Description, affiliation.Name)
	}
	identity.OriginText = originText

	// A variant root answers the same question as an ordinary one, so a character
	// has exactly one of the two. Accepting both would describe a root the world
	// does not have; accepting a variant without dropping the ordinary one would
	// let the page's own filtering be the only thing keeping them apart.
	//
	// The variant is read first because an ordinary root's arity is checked
	// against the category: a player who picked 风灵根 has no ordinary attributes
	// to count, and counting them would reject a legal choice.
	variantID := strings.TrimSpace(choice.VariantRootID)
	if variantID != "" {
		variant, ok := c.VariantRoot(variantID)
		if !ok {
			return Identity{}, identityError("api.storyPack.identity.variantRootUnknown", "id", variantID)
		}
		if len(choice.RootAttributes) > 0 {
			return Identity{}, identityError("api.storyPack.identity.variantRootExclusive", "variant", variant.Name)
		}
		identity.VariantRootID = variant.ID
		identity.VariantRootName = variant.Name
	} else {
		category, ok := c.RootCategory(strings.TrimSpace(choice.RootCategoryID))
		if !ok {
			if len(c.RootCategories) > 0 {
				category = c.RootCategories[0]
			}
		}
		identity.RootCategoryID = category.ID
		identity.RootCategoryName = category.Name
		attributes, err := p.resolveRootAttributes(category, choice.RootAttributes)
		if err != nil {
			return Identity{}, err
		}
		identity.RootAttributes = attributes
	}

	if len(c.Aptitudes) > 0 {
		aptitude, ok := c.Aptitude(strings.TrimSpace(choice.AptitudeID))
		if !ok {
			aptitude, ok = defaultAptitude(c.Aptitudes)
		}
		if !ok {
			return Identity{}, identityError("api.storyPack.identity.aptitudeMissing")
		}
		identity.AptitudeID = aptitude.ID
		identity.AptitudeName = aptitude.Name
		identity.Talent = aptitude.Talent
	}

	weapon, _ := c.WeaponDirection(strings.TrimSpace(choice.WeaponID))
	if weapon.ID == "" {
		weapon, _ = DefaultOption(c.WeaponDirections)
	}
	identity.WeaponID = weapon.ID
	identity.WeaponName = weapon.Name

	traits, traitNames, err := p.resolveTraits(choice.TraitIDs)
	if err != nil {
		return Identity{}, err
	}
	identity.TraitIDs = traits
	identity.TraitNames = traitNames

	if talentID := strings.TrimSpace(choice.TalentID); talentID != "" {
		// The pack decides whether a talent is offered at all; the rules decide
		// what it is worth. A pack with no talents simply never sees one here.
		if _, ok := c.Talent(talentID); !ok {
			return Identity{}, identityError("api.storyPack.identity.talentUnknown", "id", talentID)
		}
		talent, ok := cultivation.InnateTalentByID(talentID, p.Story.Gameplay.CultivationRules)
		if !ok {
			return Identity{}, identityError("api.storyPack.identity.talentUnsupported", "id", talentID)
		}
		identity.TalentID = talent.ID
		identity.TalentName = talent.Name
		identity.TalentBonus = talentBonusText(talent)
	}

	if len(c.Aptitudes) > 0 {
		identity.Foundation = composeFoundation(identity.RootCategoryName, identity.RootAttributes, identity.VariantRootName, identity.AptitudeName, identity.WeaponName)
	}

	identity.IdentityText = clipProse(choice.Identity, identityProseMaxRunes)
	if origin.ID != "custom" {
		identity.IdentityText = origin.Name
		if localized, ok := p.forLocale(choice.Locale); ok && localized.Creation != nil {
			if name := localized.Creation.Origins[origin.ID].Name; name != "" {
				identity.IdentityText = name
			}
		}
	}
	identity.Appearance = clipProse(choice.Appearance, identityProseMaxRunes)
	identity.Background = clipProse(choice.Background, identityProseMaxRunes)
	identity.Situation = clipProse(choice.Situation, identityProseMaxRunes)
	identity.Predicament = clipProse(choice.Predicament, identityProseMaxRunes)
	identity.Contacts = clipContacts(choice.Contacts)
	identity.Opening = clipProse(choice.Opening, identityProseMaxRunes)

	if err := p.resolveAdventureCreation(choice, &identity); err != nil {
		return Identity{}, err
	}
	identity.Profile = composeProfile(identity)
	identity.Premise = composePremise(identity)
	return identity, nil
}

func (p Pack) isStateBasisTemplate(templateID string) bool {
	if p.Creation == nil {
		return false
	}
	for _, id := range p.Creation.StateBasisTemplates {
		if strings.TrimSpace(id) == templateID {
			return true
		}
	}
	return false
}

// resolveAffiliation accepts a concrete affiliation, "none", "own" or "auto".
// "auto" resolves to the pack's own "none" only when the player left the choice
// entirely to the draft and the draft named nothing legal; a draft that names a
// real affiliation is validated by ResolveIdentity on the second pass.
//
// "own" is the household the character's own surname belongs to. Its name is
// derived from the player's name rather than authored, so a player can start
// from a family of their own without being able to type the name of a family
// this world already has.
func (p Pack) resolveAffiliation(id, playerName string) (CreationOption, error) {
	id = strings.TrimSpace(id)
	if id == "" || id == "auto" {
		if option, ok := p.Creation.Affiliation("none"); ok {
			return option, nil
		}
		return CreationOption{}, identityError("api.storyPack.identity.affiliationNoneMissing")
	}
	option, ok := p.Creation.Affiliation(id)
	if !ok {
		return CreationOption{}, identityError("api.storyPack.identity.affiliationUnknown", "id", id)
	}
	if option.ID == ownFamilyAffiliationID {
		family := ownFamilyName(playerName)
		if family == "" {
			return CreationOption{}, identityError("api.storyPack.identity.ownFamilyNeedsName")
		}
		option.Name = family
		option.Description = "按你的姓氏另立一族，不与这个世界已有的家族混同。"
	}
	return option, nil
}

// checkOriginAffiliation rejects a combination the origin's own kind rules out:
// a loose cultivator cannot belong to a sect, a sect disciple must name one, and
// a family heir must name a family. An origin with no kind (a custom origin) is
// free to pair with anything, because the player stated the origin themselves.
//
// The state basis settles that last case. A custom origin picks the basis it
// starts from, and that basis belongs to one of the authored origins, so the
// same rule applies to it: a player who states 世家出身的外门弟子 and starts from
// the sect template cannot be given a family. Without this the page's own
// filtering would be the only thing standing between a forged request and a
// character whose origin text and affiliation disagree.
func (p Pack) checkOriginAffiliation(origin CreationOrigin, affiliation CreationOption, templateID string) error {
	want := strings.TrimSpace(origin.AffiliationKind)
	if want == "" {
		want = p.affiliationKindForTemplate(templateID)
	}
	if want == "" {
		return nil
	}
	got := strings.TrimSpace(affiliation.Kind)
	if got == want {
		return nil
	}
	switch want {
	case "none":
		return identityError("api.storyPack.identity.affiliationNoneForbidden", "origin", origin.Name)
	case "sect":
		if got == "" || affiliation.ID == "none" || affiliation.ID == ownFamilyAffiliationID {
			return identityError("api.storyPack.identity.sectRequired", "origin", origin.Name)
		}
		return identityError("api.storyPack.identity.sectMismatch", "origin", origin.Name, "affiliation", affiliation.Name)
	case "family":
		if got == "" || affiliation.ID == "none" {
			return identityError("api.storyPack.identity.familyRequired", "origin", origin.Name)
		}
		return identityError("api.storyPack.identity.familyMismatch", "origin", origin.Name, "affiliation", affiliation.Name)
	}
	return identityError("api.storyPack.identity.affiliationKindMismatch",
		"origin", origin.Name, "kind", want, "affiliation", affiliation.Name)
}

// affiliationKindForTemplate is the affiliation kind the origin using this
// state basis declares. It lets a custom origin inherit the same rule as the
// authored origin whose start the player picked.
func (p Pack) affiliationKindForTemplate(templateID string) string {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" || p.Creation == nil {
		return ""
	}
	for _, origin := range p.Creation.Origins {
		if strings.TrimSpace(origin.TemplateID) != templateID && strings.TrimSpace(origin.DefaultTemplateID) != templateID {
			continue
		}
		if kind := strings.TrimSpace(origin.AffiliationKind); kind != "" {
			return kind
		}
	}
	return ""
}

// resolveLocation accepts a concrete place or "auto", which starts at the
// pack's marked default. A concrete location that belongs to a faction other
// than the chosen affiliation is a real conflict and is reported as one.
func (p Pack) resolveLocation(id string, affiliation CreationOption) (CreationOption, error) {
	id = strings.TrimSpace(id)
	if id == "" || id == "auto" {
		for _, option := range p.Creation.Locations {
			if option.ID == "auto" {
				continue
			}
			if option.Default {
				return option, nil
			}
		}
		if option, ok := p.Creation.Location(identityDefaultLocationID); ok {
			return option, nil
		}
		if len(p.Creation.Locations) > 0 {
			return p.Creation.Locations[0], nil
		}
		return CreationOption{}, identityError("api.storyPack.identity.locationMissing")
	}
	option, ok := p.Creation.Location(id)
	if !ok {
		return CreationOption{}, identityError("api.storyPack.identity.locationUnknown", "id", id)
	}
	if faction := placeFactionID(option.ID); faction != "" && affiliation.ID != "none" && faction != affiliation.ID {
		factionName := faction
		if owner, found := p.Creation.Affiliation(faction); found {
			factionName = owner.Name
		}
		return CreationOption{}, identityError("api.storyPack.identity.locationFactionMismatch",
			"location", option.Name, "faction", factionName, "affiliation", affiliation.Name)
	}
	return option, nil
}

// identityCheckAffiliationLocation reports a country mismatch between a chosen
// affiliation and a chosen location.
//
// The catalog's faction approach places carry no faction id, so the only
// authored fact tying them to a country is the place's own country. A character
// who belongs to a 越国 sect cannot be introduced living in 元武国.
func identityCheckAffiliationLocation(affiliation, location CreationOption) error {
	if affiliation.ID == "none" {
		return nil
	}
	affiliationCountry := strings.TrimSpace(affiliation.Country)
	locationCountry := strings.TrimSpace(location.Country)
	if affiliationCountry == "" || locationCountry == "" || affiliationCountry == locationCountry {
		return nil
	}
	return identityError("api.storyPack.identity.locationCountryMismatch",
		"affiliation", affiliation.Name, "affiliation_country", affiliationCountry,
		"location", location.Name, "location_country", locationCountry)
}

// defaultAptitude returns the aptitude tier marked default, or the first one
// when none is marked. DefaultOption only knows plain options; aptitude tiers
// carry a talent value and need their own lookup.
func defaultAptitude(options []CreationAptitude) (CreationAptitude, bool) {
	if len(options) == 0 {
		return CreationAptitude{}, false
	}
	for _, option := range options {
		if option.Default {
			return option, true
		}
	}
	return options[0], true
}

// placeFactionID derives the faction a faction-approach place belongs to from
// its authored id ("yue.huangfeng.approach" → "yue.huangfeng").
func placeFactionID(placeID string) string {
	const suffix = ".approach"
	placeID = strings.TrimSpace(placeID)
	if !strings.HasSuffix(placeID, suffix) {
		return ""
	}
	return strings.TrimSuffix(placeID, suffix)
}

func (p Pack) resolveRootAttributes(category CreationOption, ids []string) ([]string, error) {
	names := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		attribute, ok := p.Creation.RootAttribute(id)
		if !ok {
			return nil, identityError("api.storyPack.identity.rootAttributeUnknown", "id", strings.TrimSpace(id))
		}
		if seen[attribute.ID] {
			continue
		}
		seen[attribute.ID] = true
		names = append(names, attribute.Name)
	}
	min, max, constrained := p.Creation.rootArity(category.ID)
	if constrained && (len(names) < min || len(names) > max) {
		if min == max {
			return nil, identityError("api.storyPack.identity.rootArityExact",
				"category", category.Name, "count", min, "chosen", len(names))
		}
		return nil, identityError("api.storyPack.identity.rootArityRange",
			"category", category.Name, "min", min, "max", max, "chosen", len(names))
	}
	if len(names) > identityMaxRootAttrs {
		return nil, identityError("api.storyPack.identity.rootAttributeTooMany", "max", identityMaxRootAttrs)
	}
	return names, nil
}

func (p Pack) resolveTraits(ids []string) ([]string, []string, error) {
	resolved := make([]string, 0, len(ids))
	names := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		trait, ok := p.Creation.Trait(id)
		if !ok {
			return nil, nil, identityError("api.storyPack.identity.traitUnknown", "id", strings.TrimSpace(id))
		}
		if seen[trait.ID] {
			continue
		}
		seen[trait.ID] = true
		resolved = append(resolved, trait.ID)
		names = append(names, trait.Name)
	}
	if len(resolved) > identityMaxTraits {
		return nil, nil, identityError("api.storyPack.identity.traitTooMany", "max", identityMaxTraits)
	}
	return resolved, names, nil
}

// composeFoundation writes the one field the cultivation rules read.
//
// The aptitude word is the only tier word in the text, and it is the pack's own
// wording for the chosen tier, so normalizeCultivationTalentText reads back
// exactly the tier the page offered. The root and weapon are described around it
// and carry no tier wording, because the rules do not read them.
//
// A variant root replaces the ordinary root rather than joining it: the two are
// the same fact stated two ways, and a character has one. Neither kind grants a
// realm, a technique or a training rate this round.
//
// It reads as a sentence about the character rather than a statement about what
// the rules will allow. Whether a technique is learned is decided by the turn's
// own skill records, never by this line.
func composeFoundation(category string, attributes []string, variant, aptitude, weapon string) string {
	parts := make([]string, 0, 4)
	root := strings.TrimSpace(variant)
	if root == "" && category != "" {
		root = category
		if len(attributes) > 0 {
			root += "（" + strings.Join(attributes, "、") + "）"
		}
	}
	if root != "" {
		parts = append(parts, root)
	}
	parts = append(parts, "资质"+aptitude)
	if weapon != "" && weapon != "无偏好" {
		parts = append(parts, "偏好"+weapon)
	}
	return strings.Join(parts, "，") + "。"
}

// talentBonusText states a talent's size the way a player reads it. The number
// comes from the rules' table, never from the pack or the request, so the page,
// the profile and the settlement cannot disagree about what a talent is worth.
func talentBonusText(talent cultivation.InnateTalent) string {
	label := map[cultivation.AttributeName]string{
		cultivation.AttrHealthMax:  "生命上限",
		cultivation.AttrManaMax:    "灵力上限",
		cultivation.AttrAttack:     "攻击",
		cultivation.AttrDefense:    "防御",
		cultivation.AttrAgility:    "身法",
		cultivation.AttrPerception: "神识",
	}[talent.Attribute]
	if label == "" {
		return ""
	}
	return fmt.Sprintf("%s+%g%%", label, talent.Value*100)
}

func composeProfile(identity Identity) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "姓名：%s。性别：%s，年龄：%d 岁。\n", identity.Name, identity.GenderName, identity.Age)
	fmt.Fprintf(&sb, "出身：%s。\n", identity.OriginText)
	fmt.Fprintf(&sb, "所属：%s；落脚：%s。\n", identity.AffiliationName, identity.LocationName)
	if names := identity.AdventureTrainingNames(); names != "" {
		fmt.Fprintf(&sb, "开局修习：%s。已学与装备以实际初态为准，后续可循来源修习其他路数。\n", names)
	}
	if identity.Foundation != "" {
		fmt.Fprintf(&sb, "灵根与资质：%s\n", identity.Foundation)
	}
	if len(identity.TraitNames) > 0 {
		fmt.Fprintf(&sb, "特性：%s。\n", strings.Join(identity.TraitNames, "、"))
	}
	if identity.TalentName != "" {
		if identity.TalentBonus != "" {
			fmt.Fprintf(&sb, "天赋特性：%s（%s）。\n", identity.TalentName, identity.TalentBonus)
		} else {
			fmt.Fprintf(&sb, "天赋特性：%s。\n", identity.TalentName)
		}
	}
	if identity.Background != "" {
		sb.WriteString(identity.Background)
		sb.WriteString("\n")
	}
	if identity.Predicament != "" {
		fmt.Fprintf(&sb, "眼前牵挂：%s\n", identity.Predicament)
	}
	return strings.TrimSpace(sb.String())
}

func composePremise(identity Identity) string {
	var sb strings.Builder
	if identity.OriginText != "" {
		sb.WriteString(identity.OriginText)
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "%s，%d 岁，%s，%s，从%s开始。\n",
		identity.Name, identity.Age, identity.GenderName, identity.AffiliationName, identity.LocationName)
	if names := identity.AdventureTrainingNames(); names != "" {
		fmt.Fprintf(&sb, "开局修习：%s。\n", names)
	}
	if identity.IdentityText != "" {
		sb.WriteString(identity.IdentityText)
		sb.WriteString("\n")
	}
	if identity.Situation != "" {
		sb.WriteString(identity.Situation)
		sb.WriteString("\n")
	}
	if identity.Opening != "" {
		sb.WriteString(identity.Opening)
	}
	return strings.TrimSpace(sb.String())
}

// creationRealmLabelFor is the wording the pack's creation page uses for a
// template's realm, so a preview never shows the rules' internal id.
func (p Pack) creationRealmLabelFor(templateID string) string {
	if p.Creation == nil {
		return ""
	}
	for _, origin := range p.Creation.Origins {
		if origin.InitialState == nil {
			continue
		}
		uses := strings.TrimSpace(origin.TemplateID)
		if uses == "" {
			uses = strings.TrimSpace(origin.DefaultTemplateID)
		}
		if uses == strings.TrimSpace(templateID) {
			return strings.TrimSpace(origin.InitialState.RealmLabel)
		}
	}
	return ""
}

func initialStateOf(template ProtagonistTemplate) *CreationInitialState {
	state := template.State
	if len(state) == 0 {
		return nil
	}
	initial := &CreationInitialState{}
	if realm, ok := state["当前境界"].(string); ok {
		initial.Realm = realm
	}
	if currency, ok := state["灵石"].(float64); ok {
		initial.Currency = int(currency)
	} else if currency, ok := state["灵石"].(int); ok {
		initial.Currency = currency
	}
	if items, ok := state["重要物品"].(map[string]any); ok {
		for name := range items {
			initial.Items = append(initial.Items, name)
		}
	}
	// Every equipment slot, including the three 法宝 ones of
	// 《自制-装备栏与法宝御使》§1. A copy that starts the story already in hand is
	// still something the character owns, so it belongs in this summary; listing
	// only 主法器 and 护具 left a formation focus and a treasure out of a creation
	// page that says what the character starts with.
	for _, slot := range []string{
		"主法器", "护具", "布阵",
		"攻击法宝", "防御法宝", "辅助法宝",
	} {
		if holder, ok := state[slot].(map[string]any); ok {
			if name, ok := holder["名称"].(string); ok && strings.TrimSpace(name) != "" {
				initial.Items = append(initial.Items, strings.TrimSpace(name))
			}
		}
	}
	if techniques, ok := state["已学功法"].(map[string]any); ok {
		for name := range techniques {
			initial.Techniques = append(initial.Techniques, name)
		}
	}
	return initial
}

func clipProse(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func clipContacts(contacts []string) []string {
	kept := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		contact = clipProse(contact, identityContactMaxRunes)
		if contact == "" {
			continue
		}
		kept = append(kept, contact)
		if len(kept) == identityMaxContacts {
			break
		}
	}
	return kept
}
