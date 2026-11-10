package storypack

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
)

// Localization carries one language's display text for a pack. It exists so a
// delivered world can be read in a second language without a second copy of the
// world: every entry is keyed by the stable identity the Chinese text already
// uses, and carries only prose. Numbers, rule tokens, canonical realm names,
// item ids and the world's own facts are not translated here, so switching
// language can never move a value a mechanic reads.
//
// A key with no entry falls back to the pack's own Chinese text. That is the
// honest result for a user-authored world, which ships one language; the page
// shows what the author wrote rather than inventing a translation.
type Localization struct {
	// Definitions is display-only prose keyed by stable skill/item/method ID.
	// It is joined to a private gameplay copy only when a new Story is frozen.
	Definitions map[string]cultivation.DefinitionProse `json:"definitions,omitempty"`
	// Name, Description and Tags replace the pack's own header text.
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Title and Protagonist replace the story block's own title and the
	// protagonist's own name. The protagonist's name is display only: the
	// actor's identity stays the one the story was created with.
	Title       string `json:"title,omitempty"`
	Protagonist string `json:"protagonist,omitempty"`
	// World is keyed by WorldEntry.ID.
	World map[string]WorldLocalization `json:"world,omitempty"`
	// Characters is keyed by CatalogCharacter.SourceKey.
	Characters map[string]CharacterLocalization `json:"characters,omitempty"`
	// Creation carries the creation page's option labels, keyed by group then
	// option id. It is a map of maps so a language can cover only the groups
	// that need it.
	Creation *CreationLocalization `json:"creation,omitempty"`
	// Templates is keyed by ProtagonistTemplate.ID.
	Templates map[string]TemplateLocalization `json:"templates,omitempty"`
	// InitialState carries the ready-made start's display text. It is keyed by
	// the pack's own Chinese wording rather than by an id, because a template's
	// state stores the realm label, the items and the techniques as the words a
	// player reads. Only the display copy changes; the state the story starts
	// with is still the template's.
	InitialState *InitialStateLocalization `json:"initial_state,omitempty"`
	// Adventure maps frozen skill, item, recipe, opponent and market IDs to
	// display names. Callers translate a view copy; canonical rules keep IDs
	// and values unchanged. Missing keys preserve user-authored source text.
	Adventure map[string]string `json:"adventure,omitempty"`
}

// InitialStateLocalization replaces the display text of a ready-made start.
type InitialStateLocalization struct {
	CurrencyLabel string `json:"currency_label,omitempty"`
	// Realms is keyed by the pack's realm label, e.g. 炼气6层.
	Realms map[string]string `json:"realms,omitempty"`
	// Items and Techniques are keyed by the name the pack's state uses, which
	// covers carried things, equipment and learned methods alike.
	Items      map[string]string `json:"items,omitempty"`
	Techniques map[string]string `json:"techniques,omitempty"`
}

// WorldLocalization replaces one world-book entry's display text.
type WorldLocalization struct {
	Name             string   `json:"name,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

// CharacterLocalization replaces one catalog row's display text.
type CharacterLocalization struct {
	Name        string `json:"name,omitempty"`
	Brief       string `json:"brief,omitempty"`
	Role        string `json:"role,omitempty"`
	PlaceName   string `json:"initial_place_label,omitempty"`
	FactionName string `json:"faction_name,omitempty"`
}

// TemplateLocalization replaces one ready-made character's display text.
type TemplateLocalization struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// CreationLocalization carries the creation page's option labels, keyed by
// option id inside each group. `effect_notes`, `age_range` and
// `state_basis_templates` are absent on purpose: the first holds protocol
// tokens the rules compare against, the other two hold numbers and template ids.
// None of them is display text, and translating one would break a lookup.
type CreationLocalization struct {
	AdventureClasses map[string]CreationOptionText `json:"adventure_classes,omitempty"`
	AdventureWeapons map[string]CreationOptionText `json:"adventure_weapons,omitempty"`
	AdventureMethods map[string]CreationOptionText `json:"adventure_methods,omitempty"`
	Origins          map[string]CreationOptionText `json:"origins,omitempty"`
	Affiliations     map[string]CreationOptionText `json:"affiliations,omitempty"`
	Locations        map[string]CreationOptionText `json:"locations,omitempty"`
	RootCategories   map[string]CreationOptionText `json:"root_categories,omitempty"`
	RootAttributes   map[string]CreationOptionText `json:"root_attributes,omitempty"`
	VariantRoots     map[string]CreationOptionText `json:"variant_roots,omitempty"`
	Aptitudes        map[string]CreationOptionText `json:"aptitudes,omitempty"`
	WeaponDirections map[string]CreationOptionText `json:"weapon_directions,omitempty"`
	Traits           map[string]CreationOptionText `json:"traits,omitempty"`
	Talents          map[string]CreationOptionText `json:"talents,omitempty"`
	Genders          map[string]CreationOptionText `json:"genders,omitempty"`
}

// CreationOptionText is one option's display text. A field left empty keeps the
// pack's own wording.
type CreationOptionText struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// forLocale returns the localization for one resolved locale, if the pack
// carries one. An empty locale means "use the pack's own text".
func (p Pack) forLocale(locale string) (Localization, bool) {
	if locale == "" || len(p.Localizations) == 0 {
		return Localization{}, false
	}
	localization, ok := p.Localizations[locale]
	if !ok {
		return Localization{}, false
	}
	return localization, true
}

// localizedSummary applies one language's display text to a summary. It only
// rewrites strings the reader sees; every id, count and rule value stays as the
// pack declared it.
func (p Pack) localizedSummary(summary Summary, locale string) Summary {
	localization, ok := p.forLocale(locale)
	if !ok {
		return summary
	}
	if localization.Name != "" {
		summary.Name = localization.Name
	}
	if localization.Description != "" {
		summary.Description = localization.Description
	}
	if len(localization.Tags) > 0 {
		summary.Tags = localization.Tags
	}
	if localization.Title != "" {
		summary.Title = localization.Title
	}
	if localization.Protagonist != "" {
		summary.Protagonist = localization.Protagonist
	}
	for index := range summary.ProtagonistTemplates {
		if template, found := localization.Templates[summary.ProtagonistTemplates[index].ID]; found {
			if template.Name != "" {
				summary.ProtagonistTemplates[index].Name = template.Name
			}
			if template.Description != "" {
				summary.ProtagonistTemplates[index].Description = template.Description
			}
		}
	}
	if summary.Creation != nil && localization.Creation != nil {
		summary.Creation = localizeCreation(*summary.Creation, *localization.Creation)
	}
	return summary
}

// localizeCreation returns a copy of the creation block with the option labels
// replaced. The option ids, template ids and every value the server resolves
// stay untouched.
func localizeCreation(creation PackCreation, text CreationLocalization) *PackCreation {
	localizeOptions := func(options []CreationOption, labels map[string]CreationOptionText) []CreationOption {
		if len(options) == 0 || len(labels) == 0 {
			return options
		}
		out := make([]CreationOption, len(options))
		copy(out, options)
		for index := range out {
			label, ok := labels[out[index].ID]
			if !ok {
				continue
			}
			if label.Name != "" {
				out[index].Name = label.Name
			}
			if label.Description != "" {
				out[index].Description = label.Description
			}
		}
		return out
	}

	creation.AdventureClasses = localizeOptions(creation.AdventureClasses, text.AdventureClasses)
	creation.AdventureWeapons = localizeOptions(creation.AdventureWeapons, text.AdventureWeapons)
	creation.AdventureMethods = localizeOptions(creation.AdventureMethods, text.AdventureMethods)
	creation.Genders = localizeOptions(creation.Genders, text.Genders)
	creation.Origins = localizeOrigins(creation.Origins, text.Origins)
	creation.Affiliations = localizeOptions(creation.Affiliations, text.Affiliations)
	creation.Locations = localizeOptions(creation.Locations, text.Locations)
	creation.RootCategories = localizeOptions(creation.RootCategories, text.RootCategories)
	creation.RootAttributes = localizeOptions(creation.RootAttributes, text.RootAttributes)
	creation.VariantRoots = localizeOptions(creation.VariantRoots, text.VariantRoots)
	creation.WeaponDirections = localizeOptions(creation.WeaponDirections, text.WeaponDirections)
	creation.Traits = localizeOptions(creation.Traits, text.Traits)
	creation.Talents = localizeOptions(creation.Talents, text.Talents)

	if len(creation.Aptitudes) > 0 && len(text.Aptitudes) > 0 {
		aptitudes := make([]CreationAptitude, len(creation.Aptitudes))
		copy(aptitudes, creation.Aptitudes)
		for index := range aptitudes {
			label, ok := text.Aptitudes[aptitudes[index].ID]
			if !ok {
				continue
			}
			if label.Name != "" {
				aptitudes[index].Name = label.Name
			}
			if label.Description != "" {
				aptitudes[index].Description = label.Description
			}
		}
		creation.Aptitudes = aptitudes
	}
	return &creation
}

func localizeOrigins(origins []CreationOrigin, labels map[string]CreationOptionText) []CreationOrigin {
	if len(origins) == 0 || len(labels) == 0 {
		return origins
	}
	out := make([]CreationOrigin, len(origins))
	copy(out, origins)
	for index := range out {
		label, ok := labels[out[index].ID]
		if !ok {
			continue
		}
		if label.Name != "" {
			out[index].Name = label.Name
		}
		if label.Description != "" {
			out[index].Description = label.Description
		}
	}
	return out
}

// LocalizeIdentity returns a copy of a resolved character whose display text is
// written in one language.
//
// Only words a player reads change. Every id stays, and so do the three fields
// the story actually starts from: Foundation is what the cultivation rules
// normalize an aptitude out of, and Profile and Premise are the persona and
// premise sentences written into the actor. Translating a tier word inside
// Foundation would make the settlement read a different aptitude than the page
// offered, so the page shows the structured names instead and this copy
// translates those.
//
// OriginText is rebuilt from the origin's own description, because the authored
// copy may refer to the sect or family the player picked and that name is
// translated too. A caller whose player wrote their own origin text overwrites
// this field afterwards with what they wrote.
func (p Pack) LocalizeIdentity(identity Identity, locale string) Identity {
	localization, ok := p.forLocale(locale)
	if !ok || localization.Creation == nil {
		return identity
	}
	creation := localization.Creation
	localized := identity

	if name := creation.Genders[identity.Gender].Name; name != "" {
		localized.GenderName = name
	}
	// Resolve the affiliation before expanding the origin's placeholder.
	if name := creation.Affiliations[identity.AffiliationID].Name; name != "" && identity.AffiliationID != ownFamilyAffiliationID {
		localized.AffiliationName = name
	}
	if origin, found := creation.Origins[identity.OriginID]; found {
		if origin.Name != "" {
			localized.OriginName = origin.Name
		}
		authored, _ := p.Creation.Origin(identity.OriginID)
		if origin.Description != "" && identity.OriginText == applyOriginPlaceholders(authored.Description, identity.AffiliationName) {
			localized.OriginText = applyOriginPlaceholders(origin.Description, localized.AffiliationName)
		}
	}
	if name := creation.Locations[identity.LocationID].Name; name != "" {
		localized.LocationName = name
	}
	if identity.LocationID == "auto" && identity.LocationName == identity.AffiliationName+"居处" {
		localized.LocationName = localized.AffiliationName + " residence"
	}
	if name := creation.RootCategories[identity.RootCategoryID].Name; name != "" {
		localized.RootCategoryName = name
	}
	if name := creation.VariantRoots[identity.VariantRootID].Name; name != "" {
		localized.VariantRootName = name
	}
	localized.RootAttributes = p.localizedRootAttributeNames(identity.RootAttributes, creation.RootAttributes)
	if name := creation.Aptitudes[identity.AptitudeID].Name; name != "" {
		localized.AptitudeName = name
	}
	if name := creation.WeaponDirections[identity.WeaponID].Name; name != "" {
		localized.WeaponName = name
	}
	if name := creation.AdventureClasses[identity.AdventureClassID].Name; name != "" {
		localized.AdventureClassName = name
	}
	if name := creation.AdventureWeapons[identity.AdventureWeaponID].Name; name != "" {
		localized.AdventureWeaponName = name
	}
	if name := creation.AdventureMethods[identity.AdventureMethodID].Name; name != "" {
		localized.AdventureMethodName = name
	}
	localized.TraitNames = localizedNames(identity.TraitIDs, identity.TraitNames, optionNames(creation.Traits))
	if name := creation.Talents[identity.TalentID].Name; name != "" {
		localized.TalentName = name
	}
	if bonus := localizedTalentBonus(identity.TalentID, locale, p.Story.Gameplay.CultivationRules); bonus != "" {
		localized.TalentBonus = bonus
	}
	localized.Foundation = localizedFoundation(localized, locale)
	if identity.InitialState != nil && localization.InitialState != nil {
		initial := *identity.InitialState
		text := localization.InitialState
		if initial.CurrencyLabel != "" && text.CurrencyLabel != "" {
			initial.CurrencyLabel = text.CurrencyLabel
		}
		if label := text.Realms[initial.RealmLabel]; label != "" {
			initial.RealmLabel = label
		}
		initial.Items = translatedNames(initial.Items, text.Items)
		initial.Equipped = translatedNames(initial.Equipped, text.Items)
		initial.Techniques = translatedNames(initial.Techniques, text.Techniques)
		localized.InitialState = &initial
	}
	return localized
}

// localizedFoundation rewrites the composed 灵根与特殊体质 sentence for display.
//
// The Chinese original is what the rules read, and it stays in the identity the
// story is created with. This copy is only what a page shows, so it can put the
// translated tier word in a translated frame; the settlement never sees it.
func localizedFoundation(identity Identity, locale string) string {
	if strings.TrimSpace(identity.Foundation) == "" || !isEnglishLocale(locale) {
		return identity.Foundation
	}
	root := strings.TrimSpace(identity.VariantRootName)
	if root == "" {
		root = strings.TrimSpace(identity.RootCategoryName)
		if len(identity.RootAttributes) > 0 {
			root += " (" + strings.Join(identity.RootAttributes, ", ") + ")"
		}
	}
	parts := make([]string, 0, 4)
	if root != "" {
		parts = append(parts, root)
	}
	if aptitude := strings.TrimSpace(identity.AptitudeName); aptitude != "" {
		parts = append(parts, "Aptitude: "+aptitude)
	}
	if weapon := strings.TrimSpace(identity.WeaponName); weapon != "" && !isNoPreferenceWeapon(identity.WeaponID) {
		parts = append(parts, "Leans toward "+weapon)
	}
	return strings.Join(parts, ". ") + "."
}

// isNoPreferenceWeapon reports the pack's "no particular weapon" option. It is
// read by id so the check does not depend on which language's word is in hand.
func isNoPreferenceWeapon(weaponID string) bool {
	return strings.TrimSpace(weaponID) == "none"
}

func isEnglishLocale(locale string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
}

// localizedRootAttributeNames translates root attributes, which a resolved
// identity carries as names rather than ids. The pack's own rows are the only
// place a name can be traced back to the id the language keys on.
func (p Pack) localizedRootAttributeNames(names []string, labels map[string]CreationOptionText) []string {
	if len(names) == 0 || len(labels) == 0 || p.Creation == nil {
		return names
	}
	byID := make(map[string]string, len(p.Creation.RootAttributes))
	for _, attribute := range p.Creation.RootAttributes {
		byID[attribute.Name] = attribute.ID
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if id := byID[name]; id != "" {
			if label := labels[id].Name; label != "" {
				out = append(out, label)
				continue
			}
		}
		out = append(out, name)
	}
	return out
}

// localizedNames translates a list that carries both ids and the names the
// pack resolved for them.
func localizedNames(ids, names []string, labels map[string]string) []string {
	if len(names) == 0 || len(labels) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for index, name := range names {
		if index < len(ids) {
			if label := labels[ids[index]]; label != "" {
				out = append(out, label)
				continue
			}
		}
		out = append(out, name)
	}
	return out
}

func optionNames(labels map[string]CreationOptionText) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for id, text := range labels {
		if text.Name != "" {
			out[id] = text.Name
		}
	}
	return out
}

func translatedNames(names []string, labels map[string]string) []string {
	if len(names) == 0 || len(labels) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if label := labels[name]; label != "" {
			out = append(out, label)
			continue
		}
		out = append(out, name)
	}
	return out
}

// localizedTalentBonus restates a talent's size in the language's attribute
// name. The size itself is the rules' and is never read from here.
func localizedTalentBonus(talentID, locale string, profiles ...*cultivation.Rules) string {
	talent, ok := cultivation.InnateTalentByID(talentID, profiles...)
	if !ok {
		return ""
	}
	label := attributeDisplayName(talent.Attribute, locale)
	if label == "" {
		return ""
	}
	return fmt.Sprintf("%s+%g%%", label, talent.Value*100)
}

// attributeDisplayName is the wording one language uses for a rule attribute.
// An unknown attribute or language yields "", which leaves the caller's own
// wording in place rather than inventing a name for a field.
func attributeDisplayName(attribute cultivation.AttributeName, locale string) string {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return ""
	}
	switch attribute {
	case cultivation.AttrHealthMax:
		return "Health"
	case cultivation.AttrManaMax:
		return "Spirit Power"
	case cultivation.AttrAttack:
		return "Attack"
	case cultivation.AttrDefense:
		return "Defense"
	case cultivation.AttrAgility:
		return "Agility"
	case cultivation.AttrPerception:
		return "Perception"
	}
	return ""
}

// Localize returns a copy of the pack with one language's display text applied.
// The copy is what a page reads; nothing derived from it is written back. Every
// id, count, canonical value and rule token stays as the pack declared it, so a
// page shown in another language cannot start a story in a different world.
func (p Pack) Localize(locale string) Pack {
	localization, ok := p.forLocale(locale)
	if !ok {
		return p
	}
	localized := p
	if localization.Name != "" {
		localized.Name = localization.Name
	}
	if localization.Description != "" {
		localized.Description = localization.Description
	}
	if len(localization.Tags) > 0 {
		localized.Tags = localization.Tags
	}
	if localization.Title != "" {
		localized.Story.Title = localization.Title
	}
	if localization.Protagonist != "" {
		localized.Story.ProtagonistName = localization.Protagonist
	}
	localized.World = p.localizedWorld(locale)
	localized.Characters = p.localizedCharacters(locale)
	if len(localization.Templates) > 0 {
		templates := make([]ProtagonistTemplate, len(p.ProtagonistTemplates))
		copy(templates, p.ProtagonistTemplates)
		for index := range templates {
			text, found := localization.Templates[templates[index].ID]
			if !found {
				continue
			}
			if text.Name != "" {
				templates[index].Name = text.Name
			}
			if text.Description != "" {
				templates[index].Description = text.Description
			}
		}
		localized.ProtagonistTemplates = templates
	}
	if p.Creation != nil && localization.Creation != nil {
		localized.Creation = localizeCreation(*p.Creation, *localization.Creation)
	}
	return localized
}

// LocalizedWorld returns the world entries with one language's display text
// applied. Entries the language does not cover keep the author's own text.
func (p Pack) LocalizedWorld(locale string) []WorldEntry {
	return p.localizedWorld(locale)
}

// LocalizedCharacters returns the catalog with one language's display text
// applied. SourceKey stays the identity; only the words change.
func (p Pack) LocalizedCharacters(locale string) []CatalogCharacter {
	return p.localizedCharacters(locale)
}

// localizedWorld returns the world entries with one language's display text
// applied. Entries the language does not cover keep the author's own text.
func (p Pack) localizedWorld(locale string) []WorldEntry {
	localization, ok := p.forLocale(locale)
	if !ok || len(localization.World) == 0 {
		return p.World
	}
	entries := make([]WorldEntry, len(p.World))
	copy(entries, p.World)
	for index := range entries {
		text, found := localization.World[entries[index].ID]
		if !found {
			continue
		}
		if text.Name != "" {
			entries[index].Name = text.Name
		}
		if text.BriefDescription != "" {
			entries[index].BriefDescription = text.BriefDescription
		}
		if text.Content != "" {
			entries[index].Content = text.Content
		}
		if len(text.Tags) > 0 {
			entries[index].Tags = text.Tags
		}
	}
	return entries
}

// localizedCharacters returns the catalog with one language's display text
// applied. SourceKey stays the identity; only the words change.
func (p Pack) localizedCharacters(locale string) []CatalogCharacter {
	localization, ok := p.forLocale(locale)
	if !ok || len(localization.Characters) == 0 {
		return p.Characters
	}
	characters := make([]CatalogCharacter, len(p.Characters))
	copy(characters, p.Characters)
	for index := range characters {
		text, found := localization.Characters[characters[index].SourceKey]
		if !found {
			continue
		}
		if text.Name != "" {
			characters[index].Name = text.Name
		}
		if text.Brief != "" {
			characters[index].Brief = text.Brief
		}
		if text.Role != "" {
			characters[index].Role = text.Role
		}
		if text.PlaceName != "" {
			characters[index].PlaceLabel = text.PlaceName
		}
		if text.FactionName != "" {
			characters[index].FactionName = text.FactionName
		}
	}
	return characters
}
