package interactive

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WorldPresentation is authored display vocabulary, frozen with each story.
// It never changes action identifiers, resource accounting or capability gates.
type WorldPresentation map[string]WorldPresentationTerms

// WorldPresentationTerms names shared UI concepts without redefining rules.
type WorldPresentationTerms struct {
	Combat      string `json:"combat,omitempty"`
	Exploration string `json:"exploration,omitempty"`
	Techniques  string `json:"techniques,omitempty"`
	Partner     string `json:"partner,omitempty"`
	Energy      string `json:"energy,omitempty"`
}

// Clone prevents mutable authoring or API projections from changing a frozen game.
func (p WorldPresentation) Clone() WorldPresentation {
	if p == nil {
		return nil
	}
	out := make(WorldPresentation, len(p))
	for locale, terms := range p {
		out[locale] = terms
	}
	return out
}

// Validate permits only the two UI languages and short single-line labels.
func (p WorldPresentation) Validate() error {
	for locale, terms := range p {
		limit := 40
		switch locale {
		case "zh-CN":
		case "en-US":
			limit = 80
		default:
			return fmt.Errorf("unsupported world presentation locale: %s", locale)
		}
		for field, label := range map[string]string{
			"combat": terms.Combat, "exploration": terms.Exploration,
			"techniques": terms.Techniques, "partner": terms.Partner, "energy": terms.Energy,
		} {
			if !utf8.ValidString(label) || utf8.RuneCountInString(label) > limit ||
				strings.TrimSpace(label) != label || strings.ContainsAny(label, "<>") {
				return fmt.Errorf("invalid world presentation label: %s.%s", locale, field)
			}
			for _, r := range label {
				if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
					return fmt.Errorf("world presentation label must be single-line: %s.%s", locale, field)
				}
			}
		}
	}
	return nil
}

// ForLocale does not borrow another language's labels. The caller retains its
// ordinary localized vocabulary when the selected language is not authored.
func (p WorldPresentation) ForLocale(locale string) WorldPresentationTerms {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return p["en-US"]
	}
	return p["zh-CN"]
}
