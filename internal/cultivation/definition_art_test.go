package cultivation

import (
	"strings"
	"ta/internal/catalogart"
	"testing"
)

func TestDefinitionArtSafeReferencesAndPresentationOnlyValidation(t *testing.T) {
	for _, path := range []string{"https://example.com/a.png", "/etc/x.png", "assets/image/generated/../secret.png", "assets/image/generated/payload.svg", "assets/image/generated/name\n.png", `C:\secret.png`, "assets/image/generated/a.png?key=x", "assets/image/generated/a<bad>.png"} {
		if SafeDefinitionImagePath(path) {
			t.Fatalf("unsafe generated art path accepted: %q", path)
		}
	}
	for _, art := range []DefinitionArt{{Source: "preset", PresetID: "cultivation-skill"}, {Source: "generated", ProjectID: "project", ImagePath: "assets/image/generated/art.png"}} {
		if err := art.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, art := range []DefinitionArt{{Source: "preset", PresetID: "arbitrary"}, {Source: "generated", ImagePath: "assets/image/generated/art.png"}, {Source: "preset", PresetID: "space-skill", ImagePath: "assets/image/generated/art.png"}} {
		if art.Validate() == nil {
			t.Fatalf("invalid art accepted: %+v", art)
		}
	}
}

func TestDefinitionArtCatalogReferencesAreRegisteredAndSingleScope(t *testing.T) {
	id := catalogart.Default().AssetID("lanchuan-qunxia", "definition", "lanchuan-i01", 1, "illustration")
	if id == "" {
		t.Fatal("reviewed definition image missing")
	}
	if err := (DefinitionArt{Source: "catalog", CatalogID: id}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, art := range []DefinitionArt{{Source: "catalog", CatalogID: "../bad"}, {Source: "catalog", CatalogID: strings.Repeat("0", 64)}, {Source: "catalog", CatalogID: id, ProjectID: "foreign"}, {Source: "generated", CatalogID: id, ProjectID: "p", ImagePath: "assets/image/generated/a.png"}, {Source: "preset", CatalogID: id, PresetID: "space-skill"}, {Source: "catalog", CatalogID: catalogart.Default().AssetID("lanchuan-qunxia", "npc", "lanchuan-p01", 1, "avatar")}} {
		if art.Validate() == nil {
			t.Fatalf("invalid catalog reference accepted: %+v", art)
		}
	}
}
