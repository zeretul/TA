package storypack

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// TestShippedPackOffersFiveReadyMadeCharacters pins the U2b contract: the
// 凡人 world offers a 散修, a 宗门弟子 and a 修仙世家嫡系, each startable
// without writing a character sheet, and none of them is a second identity
// system — every template resolves to the same CreateStoryRequest a hand-written
// character produces.
func TestShippedPackOffersFiveReadyMadeCharacters(t *testing.T) {
	pack := mustBuiltinPack(t, "fanren-jie")
	if len(pack.ProtagonistTemplates) != 5 {
		t.Fatalf("pack offers %d templates, want 5", len(pack.ProtagonistTemplates))
	}
	want := map[string]bool{"loose-cultivator": false, "sect-disciple": false, "family-heir": false, "clan-young-master": false, "fanren-foundation-escort": false}
	for _, template := range pack.ProtagonistTemplates {
		if _, ok := want[template.ID]; !ok {
			t.Fatalf("unexpected template id %q", template.ID)
		}
		want[template.ID] = true
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("template %q is missing", id)
		}
	}
}

// TestEveryTemplateResolvesToOneOrdinaryCreateRequest checks that a templated
// start is not a parallel path: it carries the pack's binding, its modules and
// its world scope exactly as the pack's own protagonist does, and differs only
// in the three things a character choice legitimately changes.
func TestEveryTemplateResolvesToOneOrdinaryCreateRequest(t *testing.T) {
	pack := mustBuiltinPack(t, "fanren-jie")
	base := pack.StoryRequest()
	for _, template := range pack.ProtagonistTemplates {
		req, state, err := pack.StoryRequestForTemplate(template.ID)
		if err != nil {
			t.Fatalf("template %s: %v", template.ID, err)
		}
		if req.PackBinding == nil || req.PackBinding.Scope != base.PackBinding.Scope {
			t.Fatalf("template %s does not bind to its pack's world scope", template.ID)
		}
		if req.ModuleRefs == nil || req.ModuleRefs.ActorStateID != base.ModuleRefs.ActorStateID {
			t.Fatalf("template %s changed the bound modules", template.ID)
		}
		if req.Protagonist.Mode != interactive.StoryProtagonistModeCustom {
			t.Fatalf("template %s protagonist mode = %q, want custom", template.ID, req.Protagonist.Mode)
		}
		if req.Protagonist.Name != template.CharacterName {
			t.Fatalf("template %s protagonist name = %q, want %q", template.ID, req.Protagonist.Name, template.CharacterName)
		}
		if req.Opening.Mode != interactive.StoryOpeningModeAI {
			t.Fatalf("template %s should open on the model-written path, got %q", template.ID, req.Opening.Mode)
		}
		if req.Opening.CustomText != "" {
			t.Fatalf("template %s carried its authored opening into the player's draft", template.ID)
		}
		if req.Origin == "" {
			t.Fatalf("template %s has no origin", template.ID)
		}
		// The state travels beside the request, never inside it, so the browser
		// cannot supply it.
		if _, ok := state["当前境界"]; !ok {
			t.Fatalf("template %s does not seed a starting stage", template.ID)
		}
	}
}

// TestEveryTemplateStartsOnALegalStage is the "真实硬结果先结算" check.
//
// The template seeds only 当前境界. The settlement must be able to derive every
// attribute and both pools from that key alone, which is what lets the opening
// show real numbers before the model has spoken, and what keeps a realm
// something the rules grant rather than the narrative.
func TestEveryTemplateStartsOnALegalStage(t *testing.T) {
	pack := mustBuiltinPack(t, "fanren-jie")
	valid := map[string]bool{}
	for _, key := range cultivation.StageKeys() {
		valid[key] = true
	}
	for _, template := range pack.ProtagonistTemplates {
		_, state, err := pack.StoryRequestForTemplate(template.ID)
		if err != nil {
			t.Fatalf("template %s: %v", template.ID, err)
		}
		stage, _ := state["当前境界"].(string)
		if !valid[stage] {
			t.Fatalf("template %s starts on %q, which the rules cannot settle", template.ID, stage)
		}
		// Every other seeded number would be a value the rules own. A template
		// writing one would be authoring an outcome.
		for _, settled := range []string{"生命", "生命上限", "灵力", "灵力上限", "攻击", "防御", "身法", "神识", "境界进度"} {
			if _, ok := state[settled]; ok {
				t.Fatalf("template %s authors the settled field %s", template.ID, settled)
			}
		}
	}
}

// TestTemplateFieldsAreDeclaredByTheBoundModule is the guard against a silently
// dropped field.
//
// The initial-actor normalizer drops keys the bound module does not declare, so a
// template typo would start the character at the module's defaults with nothing
// reported. Checking every key against the real xiuxian module here means the
// pack cannot ship such a typo.
func TestTemplateFieldsAreDeclaredByTheBoundModule(t *testing.T) {
	pack := mustBuiltinPack(t, "fanren-jie")
	system := xiuxianSystemForTest(t, pack)
	declared := map[string]bool{}
	for _, template := range system.Templates {
		if template.ID != interactive.DefaultActorID {
			continue
		}
		for _, field := range template.Fields {
			declared[field.Name] = true
		}
	}
	if len(declared) == 0 {
		t.Fatal("the xiuxian module declares no protagonist fields")
	}
	for _, template := range pack.ProtagonistTemplates {
		_, state, err := pack.StoryRequestForTemplate(template.ID)
		if err != nil {
			t.Fatalf("template %s: %v", template.ID, err)
		}
		for key := range state {
			if !declared[key] {
				t.Fatalf("template %s seeds %q, which the bound module does not declare", template.ID, key)
			}
		}
	}
}

// TestTemplateNamesDoNotCollideWithCatalogPeople keeps the player from meeting
// themselves: a template character is a new person in the world, not a rename of
// someone already in its catalogue.
func TestTemplateNamesDoNotCollideWithCatalogPeople(t *testing.T) {
	pack := mustBuiltinPack(t, "fanren-jie")
	catalog := map[string]bool{}
	for _, character := range pack.Characters {
		catalog[character.Name] = true
	}
	for _, template := range pack.ProtagonistTemplates {
		if catalog[template.CharacterName] {
			t.Fatalf("template %s reuses the catalog person %q", template.ID, template.CharacterName)
		}
	}
}

func mustBuiltinPack(t *testing.T, id string) Pack {
	t.Helper()
	for _, pack := range builtinPacks() {
		if pack.ID == id {
			return pack
		}
	}
	t.Fatalf("builtin pack %q is missing", id)
	return Pack{}
}

// xiuxianSystemForTest loads the real bound actor-state module from the builtin
// catalog, so the field check runs against the schema the game actually freezes.
//
// The library materialises the builtin modules under the root it is given, so it
// is handed a temporary root. An empty root would resolve against the package
// directory and leave `presets/` behind in the source tree.
func xiuxianSystemForTest(t *testing.T, pack Pack) interactive.StoryDirectorActorStateSystem {
	t.Helper()
	root := t.TempDir()
	if _, err := NewLibrary(root).materializeGameplayModules(&pack); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatalf("loading actor-state module %q: %v", pack.Story.ModuleRefs.ActorStateID, err)
	}
	return module.ActorState
}
