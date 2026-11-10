package quest

import (
	"embed"
	"encoding/json"
	"strings"
)

// Text contains display prose only. Stable IDs, costs, gates and journal facts
// always come from the registered definition, never from a translation.
type Text struct {
	CompletionReaction string                `json:"completion_reaction,omitempty"`
	Title              string                `json:"title"`
	Brief              string                `json:"brief"`
	Objectives         map[string]string     `json:"objectives"`
	Choices            map[string]ChoiceText `json:"choices"`
}
type ChoiceText struct {
	SuccessReaction string `json:"success_reaction,omitempty"`
	FailureReaction string `json:"failure_reaction,omitempty"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Success         string `json:"success"`
	Failure         string `json:"failure"`
	SuccessEcho     string `json:"success_echo"`
	FailureEcho     string `json:"failure_echo"`
}

//go:embed assets/*.en-US.json
var translatedAssets embed.FS
var englishText = loadEnglishText()

func loadEnglishText() map[string]Text {
	result := map[string]Text{}
	entries, err := translatedAssets.ReadDir("assets")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		data, err := translatedAssets.ReadFile("assets/" + entry.Name())
		if err != nil {
			panic(err)
		}
		var texts map[string]Text
		if err := json.Unmarshal(data, &texts); err != nil {
			panic(err)
		}
		for id, text := range texts {
			if _, exists := result[id]; exists {
				panic("duplicate translated task " + id)
			}
			result[id] = text
		}
	}
	return result
}

// LocalizedDefinition returns an independent display copy; untranslated custom
// content retains the author's prose. No language selection changes a rule.
func LocalizedDefinition(d Definition, locale string, translations ...map[string]Text) Definition {
	if !strings.HasPrefix(locale, "en") {
		return d
	}
	catalog := englishText
	if len(translations) > 0 {
		catalog = translations[0]
	}
	text, ok := catalog[d.ID]
	if !ok {
		return d
	}
	d.Title, d.Brief = text.Title, text.Brief
	if d.CompletionEcho != nil && text.CompletionReaction != "" {
		e := *d.CompletionEcho
		e.Reaction.Statement = text.CompletionReaction
		d.CompletionEcho = &e
	}
	d.Objectives = append([]Objective(nil), d.Objectives...)
	for i := range d.Objectives {
		if v := text.Objectives[d.Objectives[i].ID]; v != "" {
			d.Objectives[i].Description = v
		}
	}
	d.Choices = append([]Choice(nil), d.Choices...)
	for i := range d.Choices {
		if v, ok := text.Choices[d.Choices[i].ID]; ok {
			c := &d.Choices[i]
			if c.SuccessReaction != nil && v.SuccessReaction != "" {
				r := *c.SuccessReaction
				r.Statement = v.SuccessReaction
				c.SuccessReaction = &r
			}
			if c.FailureReaction != nil && v.FailureReaction != "" {
				r := *c.FailureReaction
				r.Statement = v.FailureReaction
				c.FailureReaction = &r
			}
			c.Title, c.Description, c.Success, c.Failure, c.SuccessEcho, c.FailureEcho = v.Title, v.Description, v.Success, v.Failure, v.SuccessEcho, v.FailureEcho
		}
	}
	return d
}

// ReferenceEnglishText exports authoring text; runtime worlds freeze their own copy.
func ReferenceEnglishText() map[string]Text {
	data, _ := json.Marshal(englishText)
	var out map[string]Text
	_ = json.Unmarshal(data, &out)
	return out
}
