package app

import (
	"bytes"
	"encoding/json"
	"strings"
)

// The author snapshot keeps both languages. Model input only needs the active
// display language; removing the duplicate translation preserves every fact,
// ID and exact number while leaving room for standing orders and memory.
func historyContextPayload(input any, locale string) ([]byte, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return nil, err
	}
	language := "zh"
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		language = "en"
	}
	var localize func(any) any
	localize = func(value any) any {
		switch node := value.(type) {
		case map[string]any:
			if len(node) == 2 {
				if _, ok := node["zh"].(string); ok {
					if _, ok := node["en"].(string); ok {
						return map[string]any{language: node[language]}
					}
				}
			}
			for key, child := range node {
				node[key] = localize(child)
			}
		case []any:
			for index, child := range node {
				node[index] = localize(child)
			}
		}
		return value
	}
	return json.Marshal(localize(value))
}
