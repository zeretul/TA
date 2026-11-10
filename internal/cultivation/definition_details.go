package cultivation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// QualityGrade is authored display metadata, never a numeric settlement tier.
type QualityGrade string

const (
	QualityWhite  QualityGrade = "white"
	QualityGreen  QualityGrade = "green"
	QualityBlue   QualityGrade = "blue"
	QualityPurple QualityGrade = "purple"
	QualityOrange QualityGrade = "orange"
	QualityRed    QualityGrade = "red"
)

func (q QualityGrade) Valid() bool {
	switch q {
	case QualityWhite, QualityGreen, QualityBlue, QualityPurple, QualityOrange, QualityRed:
		return true
	}
	return false
}

// UnmarshalJSON rejects unsupported authored colors at the data boundary.
func (q *QualityGrade) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	grade := QualityGrade(value)
	if !grade.Valid() {
		return fmt.Errorf("invalid quality_grade %q", value)
	}
	*q = grade
	return nil
}

// DefinitionProse is authored player-safe text, distinct from instance provenance.
type DefinitionProse struct {
	// Name optionally localizes a builtin title; story-authored names are preserved.
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Background  string `json:"background,omitempty"`
}

// DefinitionDetails is frozen with the world definition. Localization only changes
// prose; it cannot change quality, requirements, effects, or acquisition facts.
type DefinitionLocalizations struct {
	English DefinitionProse `json:"en-US"`
}
type DefinitionDetails struct {
	Art           *DefinitionArt           `json:"art,omitempty"`
	QualityGrade  QualityGrade             `json:"quality_grade,omitempty"`
	Description   string                   `json:"description,omitempty"`
	Background    string                   `json:"background,omitempty"`
	Localizations *DefinitionLocalizations `json:"localizations,omitempty"`
}

func (d DefinitionDetails) ForLocale(locale string) DefinitionDetails {
	if strings.HasPrefix(locale, "en") {
		if d.Localizations != nil {
			prose := d.Localizations.English
			d.Description, d.Background = prose.Description, prose.Background
		}
	}
	d.Localizations = nil
	return d
}

// ValidateComplete is used for formal authored catalogs, not inferred defaults.
func (d DefinitionDetails) ValidateComplete() error {
	if err := d.Validate(); err != nil {
		return err
	}
	if !d.QualityGrade.Valid() || strings.TrimSpace(d.Description) == "" || strings.TrimSpace(d.Background) == "" {
		return fmt.Errorf("definition details require explicit quality, description and background")
	}
	return nil
}

// BuiltinDefinitionDetails returns a fresh authored metadata snapshot. Callers
// may overlay world-specific prose before freezing a newly created story.
func BuiltinDefinitionDetails() map[string]DefinitionDetails {
	out := map[string]DefinitionDetails{}
	for _, d := range TechniqueTable() {
		out[d.ID] = d.DefinitionDetails
	}
	for _, d := range SkillTable() {
		out[d.ID] = d.DefinitionDetails
	}
	for _, d := range NewItemCatalogWithFirstVersion().Definitions() {
		out[d.ID] = d.DefinitionDetails
	}
	return out
}

// Validate bounds display-only author text separately from mechanical context.
// Limits apply to complete UTF-8 strings; no consumer silently truncates prose.
func (d DefinitionDetails) Validate() error {
	if d.Art != nil {
		if err := d.Art.Validate(); err != nil {
			return err
		}
	}
	if d.QualityGrade != "" && !d.QualityGrade.Valid() {
		return fmt.Errorf("invalid quality_grade %q", d.QualityGrade)
	}
	for _, text := range []string{d.Description, d.Background} {
		if len(text) > 8192 {
			return fmt.Errorf("definition prose exceeds 8 KiB per field")
		}
	}
	if d.Localizations != nil {
		for _, text := range []string{d.Localizations.English.Name, d.Localizations.English.Description, d.Localizations.English.Background} {
			if len(text) > 8192 {
				return fmt.Errorf("localized definition prose exceeds 8 KiB per field")
			}
		}
	}
	return nil
}
