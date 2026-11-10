package storypack

import "strings"

// ProtagonistTemplate is one ready-made character a pack offers.
//
// It exists so a player can start a world without writing a character sheet.
// It is deliberately not a second identity system: the template only fills in
// the same protagonist snapshot and the same Actor State fields a hand-written
// character would, so a templated start and a free-form start reach
// CreateStory through one request and one state system.
//
// State is keyed by the bound actor-state module's own field names. That is
// what keeps the pack honest about the module it binds: a template naming a
// field the module does not declare fails at creation with the field named,
// instead of starting a character whose realm silently went missing.
type ProtagonistTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// CharacterName is the default protagonist name. The player may edit it.
	CharacterName string `json:"character_name"`
	// Profile is the editable character sheet prose, in the same shape a
	// hand-written protagonist profile takes.
	Profile string `json:"profile"`
	// Origin replaces the pack's premise for this character. A sect disciple
	// and a family heir do not begin in the same place, so one shared origin
	// would put every template in the same scene.
	Origin string `json:"origin,omitempty"`
	// OpeningText is the authored first scene. Empty means the model writes it
	// from Origin, exactly as a pack without an opening behaves.
	OpeningText string `json:"opening_text,omitempty"`
	// State seeds the protagonist Actor. Only fields the rules do not settle
	// belong here: identity prose, the starting realm, the situation and the
	// carried things. The six attributes and the pools are derived from the
	// realm by the settlement, so a template that wrote them would be authoring
	// numbers the rules own.
	State map[string]any `json:"state,omitempty"`
}

// TemplateSummary is the list projection of a template: enough to render the
// choice, without shipping the world content a player has not picked.
type TemplateSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ProtagonistTemplate returns one template by ID.
func (p Pack) ProtagonistTemplate(id string) (ProtagonistTemplate, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ProtagonistTemplate{}, false
	}
	for _, template := range p.ProtagonistTemplates {
		if template.ID == id {
			return template, true
		}
	}
	return ProtagonistTemplate{}, false
}

func (p Pack) templateSummaries() []TemplateSummary {
	if len(p.ProtagonistTemplates) == 0 {
		return nil
	}
	summaries := make([]TemplateSummary, 0, len(p.ProtagonistTemplates))
	for _, template := range p.ProtagonistTemplates {
		summaries = append(summaries, TemplateSummary{
			ID:          template.ID,
			Name:        template.Name,
			Description: template.Description,
		})
	}
	return summaries
}

// validateProtagonistTemplates checks the templates a pack declares.
//
// A template missing its profile would start a character with no identity, and
// two templates sharing an ID would make the selection ambiguous, so both are
// rejected while the pack is still just a file.
func (p Pack) validateProtagonistTemplates() error {
	seen := map[string]bool{}
	for _, template := range p.ProtagonistTemplates {
		id := strings.TrimSpace(template.ID)
		if id == "" {
			return errTemplate("快捷模板缺少 id")
		}
		if seen[id] {
			return errTemplate("快捷模板 id 重复: " + id)
		}
		seen[id] = true
		if strings.TrimSpace(template.Name) == "" {
			return errTemplate("快捷模板缺少 name: " + id)
		}
		if strings.TrimSpace(template.Profile) == "" {
			return errTemplate("快捷模板缺少 profile: " + id)
		}
		if strings.TrimSpace(template.CharacterName) == "" {
			return errTemplate("快捷模板缺少 character_name: " + id)
		}
	}
	return nil
}

type templateError string

func (e templateError) Error() string { return string(e) }

func errTemplate(message string) error { return templateError(message) }
