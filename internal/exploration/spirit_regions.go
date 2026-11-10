package exploration

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

const (
	RealmSpiritBoundary = "realm.spirit_boundary"
	RealmSpiritRift     = "realm.spirit_rift"
	RealmSpiritArray    = "realm.spirit_array"
	RealmSpiritArchive  = "realm.spirit_archive"
)

// New immutable adaptation records; older cave definitions and hashes are intact.
//
//go:embed assets/spirit-regions.json
var spiritRegionData []byte

func init() {
	var definitions []DungeonDefinition
	if err := json.Unmarshal(spiritRegionData, &definitions); err != nil {
		panic(err)
	}
	for _, d := range definitions {
		if diagnostics := ValidateDungeonDefinition(d); len(diagnostics) > 0 {
			panic(fmt.Sprintf("invalid spirit region %s: %+v", d.ID, diagnostics))
		}
		realmDefinitions = append(realmDefinitions, d)
	}
}
