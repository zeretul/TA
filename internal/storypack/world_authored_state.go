package storypack

import (
	"fmt"
	"ta/internal/interactive"
)

// Reuse the opening schema validator so imported declarations obey the same
// field contract as an adapted story. This does not execute an opening or grant
// values; the selected protagonist template remains the source of initial facts.
func validateAuthoredWorldStateFields(gameplay WorldGameplay) error {
	if len(gameplay.AuthoredStateFields) > 64 {
		return fmt.Errorf("too many authored protagonist state fields")
	}
	seen := map[string]bool{}
	for _, field := range gameplay.StateFields {
		seen[worldFieldKey(field.Name)] = true
	}
	ops := make([]interactive.ActorStateFieldSchemaOp, 0, len(gameplay.AuthoredStateFields))
	for _, field := range gameplay.AuthoredStateFields {
		key := worldFieldKey(field.Name)
		if seen[key] || field.Default != nil {
			return fmt.Errorf("authored state field duplicates an action field or grants a default: %s", field.Name)
		}
		seen[key] = true
		ops = append(ops, interactive.ActorStateFieldSchemaOp{Op: "add", Field: field})
	}
	base := interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{ID: interactive.DefaultActorID, Name: "主角事实"}}}
	_, _, err := interactive.ApplyActorStateSchemaAdaptation(base, interactive.StoryDirectorTRPGSystem{}, interactive.ActorStateSchemaAdaptation{
		TemplateOps: []interactive.ActorStateTemplateSchemaOp{{Op: "fields", TemplateID: interactive.DefaultActorID, FieldOps: ops}},
	})
	return err
}
