package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

func TestFirstSpiritBeastReceivesMethodContextBeforeAnyProfileExists(t *testing.T) {
	c := &Conversation{}
	ctx := interactive.StoryContext{Meta: interactive.StoryMeta{PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{{
		SourceKey: "catalog.sparrow", Name: "青风雀", Lifeform: "spirit_beast", CultivationMethodID: "method.spirit_beast_breathing",
	}}}}}
	text := c.characterCastContext(ctx, character.State{}, true, nil)
	if !strings.Contains(text, "method.spirit_beast_breathing") {
		t.Fatal("first-appearance turn has no eligible-method context until a profile already exists")
	}
}
