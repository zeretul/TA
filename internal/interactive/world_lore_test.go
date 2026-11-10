package interactive

import (
	"strings"
	"ta/internal/book/lore"
	"ta/internal/cultivation"
	"testing"
)

func TestWorldLoreGateCoversResidentSearchAndDirectReads(t *testing.T) {
	root := t.TempDir()
	scope := "bound-world"
	hash := "test-revision"
	id := PackEntryIDForRevision("hidden", hash)
	editor := lore.NewScopedStore(root, scope)
	_, err := editor.Create(lore.ItemInput{ID: id, Name: "Hidden crossing", Content: "Secret destination and treasure", LoadMode: lore.LoadModeResident})
	if err != nil {
		t.Fatal(err)
	}
	content := &WorldContent{PackID: "custom", Labels: map[string]string{"gate": "界隙"}, LoreGates: map[string]WorldLoreGate{"hidden": {MinimumStage: "spirit_late", Locations: []string{"gate"}}}}
	meta := StoryMeta{PackBinding: &StoryPackBinding{PackID: "custom", Scope: scope, ContentHash: hash}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: content, CultivationRules: cultivation.ReferenceRules()}}}
	scene := func(stage, location string) map[string]any {
		return map[string]any{"actors": map[string]any{DefaultActorID: map[string]any{"state": map[string]any{xiuxianFieldRealm: stage}}, DefaultStoryContextActorID: map[string]any{"state": map[string]any{storySceneLocationField: location}}}}
	}
	for _, s := range []map[string]any{scene("qi_1", "界隙"), scene("spirit_late", "远处")} {
		reader := editor.WithVisibility(StoryLoreVisibility(meta, s))
		if list, err := reader.List(); err != nil || len(list) != 0 {
			t.Fatalf("hidden entry listed: %v %v", list, err)
		}
		if _, err := reader.Read(id); err == nil {
			t.Fatal("direct ID bypassed gate")
		}
		if _, err := reader.ReadAny(id); err == nil {
			t.Fatal("disabled-inclusive read bypassed gate")
		}
		if _, err := reader.Get(id); err == nil {
			t.Fatal("identity lookup bypassed gate")
		}
		if text, err := reader.ResidentContextMarkdown(); err != nil || strings.Contains(text, "Secret destination") {
			t.Fatalf("resident leak: %s %v", text, err)
		}
		if result, err := reader.ReadManyNames([]string{"Hidden crossing"}); err != nil || len(result.Items) != 0 {
			t.Fatalf("name bypassed gate: %v %v", result, err)
		}
		if result, err := reader.QueryLoreItems(lore.IndexOptions{Keywords: []string{"crossing"}}); err != nil || len(result) != 0 {
			t.Fatalf("search bypassed gate: %v %v", result, err)
		}
	}
	reader := editor.WithVisibility(StoryLoreVisibility(meta, scene("spirit_late", "界隙")))
	if _, err := reader.Read(id); err != nil {
		t.Fatal("qualified reader lost entry", err)
	}
	if _, err := editor.Read(id); err != nil {
		t.Fatal("author editor was restricted", err)
	}
}

func TestRemovedSkillTemplateCannotReappearInGeneratedCatalog(t *testing.T) {
	rules := cultivation.ReferenceRules()
	rules.Skills = nil
	rules.SkillTemplates = nil
	record := map[string]any{"模板": "fire_bolt", "名称": "Borrowed flame"}
	if _, ok := generatedSkillFromRecord(record, rules); ok {
		t.Fatal("removed template regenerated")
	}
	system := xiuxianTestSystem()
	system.CultivationRules = rules
	system.CultivationRules.Items = nil
	normalized := normalizeActorStateSystem(system)
	for _, template := range normalized.Templates {
		for _, field := range template.Fields {
			if strings.Contains(field.UpdateInstruction, "material.ore.iron") {
				t.Fatal("removed catalog exposed in model instructions")
			}
		}
	}
}
