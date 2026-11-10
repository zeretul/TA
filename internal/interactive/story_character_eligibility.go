package interactive

import "strings"

// This reads only the Story's frozen catalog, never the current author pack.
// Names and verified aliases also match so omitting source_key cannot turn a
// historical-only identity into an invented live participant.
func recordOnlyCatalogPerson(catalog *PackCatalogSnapshot, sourceKey, name string) (PackCatalogPerson, bool) {
	if catalog == nil {
		return PackCatalogPerson{}, false
	}
	sourceKey = strings.TrimSpace(sourceKey)
	for _, person := range catalog.Characters {
		if !person.RecordOnly {
			continue
		}
		if sourceKey != "" && sourceKey == person.SourceKey || strings.TrimSpace(name) != "" && catalogPersonHasName(person, name) {
			return person, true
		}
	}
	return PackCatalogPerson{}, false
}
