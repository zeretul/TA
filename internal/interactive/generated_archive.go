package interactive

import (
	"encoding/json"
	"fmt"
)

// generatedRegistryCanAdd checks the prospective archive before any learning,
// stock or material changes. Reusing an identity consumes no additional space.
// This is a persistence safety guard, not a cap on the per-turn working set.
func generatedRegistryCanAdd(records map[string]any, additions ...map[string]any) bool {
	pending := make(map[string]any, len(additions))
	for _, record := range additions {
		if record == nil {
			continue
		}
		id := xiuxianEntryText(record, "定义")
		if id == "" {
			return false
		}
		if records[id] == nil {
			if len(xiuxianEntryText(record, "来源")) > 8192 || len(xiuxianEntryText(record, "描述")) > 8192 {
				return false
			}
			pending[id] = record
		}
	}
	if len(pending) == 0 {
		return true
	}
	if len(records)+len(pending) > generatedDefinitionLimit {
		return false
	}
	size := 2
	for _, values := range []map[string]any{records, pending} {
		for id, value := range values {
			key, _ := json.Marshal(id)
			data, err := json.Marshal(value)
			if err != nil {
				return false
			}
			size += len(key) + len(data) + 2
			if size > generatedArchiveMaxBytes {
				return false
			}
		}
	}
	return true
}

func validateAdventureGeneratedArchive(defs []AdventureGeneratedDefinition) error {
	if len(defs) > generatedDefinitionLimit {
		return fmt.Errorf("generated definition archive exceeds %d entries", generatedDefinitionLimit)
	}
	size := 2
	for _, d := range defs {
		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		size += len(data) + 1
		if size > generatedArchiveMaxBytes {
			return fmt.Errorf("generated definition archive exceeds %d bytes", generatedArchiveMaxBytes)
		}
	}
	return nil
}
