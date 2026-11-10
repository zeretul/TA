package quest

import (
	"embed"
	"encoding/json"
	"fmt"
)

// Authored definitions are immutable version 1.0 content, just like the existing
// registered tasks. A revision changing their mechanics must use a new version
// and retain the prior definition for journals already bound to it.
//
//go:embed assets/lanchuan.json assets/xinghai.json assets/fanren.json assets/membership.json assets/spirit.json assets/sui-guan.json
var adventureDefinitions embed.FS

func init() {
	seen := map[string]bool{}
	for _, d := range taskDefinitions {
		seen[d.ID] = true
	}
	for _, path := range []string{"assets/lanchuan.json", "assets/xinghai.json", "assets/fanren.json", "assets/membership.json", "assets/spirit.json", "assets/sui-guan.json"} {
		data, err := adventureDefinitions.ReadFile(path)
		if err != nil {
			panic(err)
		}
		var definitions []Definition
		if err = json.Unmarshal(data, &definitions); err != nil {
			panic(err)
		}
		for _, d := range definitions {
			if diagnostics := ValidateDefinition(d); len(diagnostics) > 0 {
				panic(fmt.Sprintf("invalid authored task %s: %+v", d.ID, diagnostics))
			}
			if seen[d.ID] {
				panic("duplicate authored task " + d.ID)
			}
			seen[d.ID] = true
			taskDefinitions = append(taskDefinitions, d)
		}
	}
}
