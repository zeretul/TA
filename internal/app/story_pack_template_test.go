package app

import (
	"testing"

	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// TestEveryTemplateStartsALegalStory drives each of the 凡人 world's ready-made
// characters through the product's real creation path — the pack file, the
// server-side template resolution, the creation defaults, and the story store —
// and checks the opening state the game will actually show.
//
// The point of the test is the order of authority. The template names a stage and
// nothing numeric; the settlement derives the six attributes and fills both pools
// from that stage; and the model has not been asked anything yet. A start that
// got those numbers from anywhere else would be the "AI grants the realm" defect
// the design forbids.
func TestEveryTemplateStartsALegalStory(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	if len(pack.ProtagonistTemplates) != 4 {
		t.Fatalf("the pack offers %d templates, want 4", len(pack.ProtagonistTemplates))
	}

	for _, template := range pack.ProtagonistTemplates {
		t.Run(template.ID, func(t *testing.T) {
			req, _, err := pack.StoryRequestForTemplate(template.ID)
			if err != nil {
				t.Fatalf("resolve template: %v", err)
			}
			// The browser may only say which template it wants. Everything the
			// template is worth is resolved from the pack file on this side.
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
				PackID:     pack.ID,
				TemplateID: template.ID,
			}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatalf("apply creation defaults: %v", err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatalf("seed template state: %v", err)
			}

			store := interactive.NewStore(t.TempDir())
			t.Cleanup(func() { _ = store.Close() })
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatalf("create story: %v", err)
			}
			context, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatalf("reopen story: %v", err)
			}

			protagonist := actorStateOf(t, context.Snapshot.State, "protagonist")
			if got := protagonist["当前境界"]; got != template.State["当前境界"] {
				t.Fatalf("stage = %v, want %v", got, template.State["当前境界"])
			}
			if got := protagonist["姓名"]; got != template.CharacterName {
				t.Fatalf("name field = %v, want %q", got, template.CharacterName)
			}
			// Every derived attribute and both pools are already the stage table's
			// real numbers here, before the model has been asked anything. A
			// template that wrote them itself would be the "AI grants the realm"
			// defect; a character who reads as 0/0 until the first turn would be
			// the panel contradicting the prose. Neither is what creation does —
			// it seeds the stage, and the rules settle the numbers once.
			for _, field := range []string{"攻击", "防御", "身法", "神识"} {
				value, ok := actorStateNumberForTest(protagonist[field])
				if !ok {
					t.Fatalf("creation left the settled field %s unwritten", field)
				}
				if value <= 0 {
					t.Fatalf("creation wrote %s = %v; the stage table gives a real number", field, value)
				}
			}
			// The two pools are the fields the settlement fills to the real
			// maximum, and the maxima are settled in the same pass.
			for _, field := range []string{"生命", "灵力", "生命上限", "灵力上限"} {
				value, ok := actorStateNumberForTest(protagonist[field])
				if !ok {
					t.Fatalf("creation left the settled pool %s unwritten", field)
				}
				if value <= 0 {
					t.Fatalf("creation wrote %s = %v; an unwounded character starts full", field, value)
				}
			}
			if health, _ := actorStateNumberForTest(protagonist["生命"]); health != mustNumberForTest(t, protagonist["生命上限"]) {
				t.Fatalf("生命 = %v, want the settled maximum %v", health, protagonist["生命上限"])
			}
			if mana, _ := actorStateNumberForTest(protagonist["灵力"]); mana != mustNumberForTest(t, protagonist["灵力上限"]) {
				t.Fatalf("灵力 = %v, want the settled maximum %v", mana, protagonist["灵力上限"])
			}
			if story.Protagonist.Name != template.CharacterName {
				t.Fatalf("story protagonist = %q, want %q", story.Protagonist.Name, template.CharacterName)
			}
		})
	}
}

// TestATemplateCannotSupplyItsOwnNumbers pins the boundary the whole server-side
// resolution exists for: a request naming a template that is not in the pack, or
// a state key the module does not declare, fails instead of starting a character
// with a silently dropped field.
func TestATemplateCannotSupplyItsOwnNumbers(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}

	t.Run("unknown template", func(t *testing.T) {
		req, _, err := pack.StoryRequestForTemplate("loose-cultivator")
		if err != nil {
			t.Fatal(err)
		}
		req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
			PackID: pack.ID, TemplateID: "does-not-exist",
		}
		req, err = service.withStoryCreationDefaults(req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.withPackTemplate(req); err == nil {
			t.Fatal("an unknown template id was accepted")
		}
	})

	t.Run("undeclared state field", func(t *testing.T) {
		req, _, err := pack.StoryRequestForTemplate("loose-cultivator")
		if err != nil {
			t.Fatal(err)
		}
		req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
			PackID: pack.ID, TemplateID: "loose-cultivator",
		}
		req, err = service.withStoryCreationDefaults(req)
		if err != nil {
			t.Fatal(err)
		}
		// A pack that names a field the module does not declare is a pack bug,
		// and the normalizer would otherwise drop it without a word.
		if err := validateTemplateStateKeys(storypack.ProtagonistTemplate{
			ID: "typo", State: map[string]any{"境界": "qi_6"},
		}, map[string]any{"境界": "qi_6"}, *req.ActorState, "protagonist"); err == nil {
			t.Fatal("an undeclared state field was accepted")
		}
	})
}

// TestATemplateStartRequiresItsOwnPack keeps a templated start from reading a
// different world's book: the character is built for one world, so the binding
// must be that world.
func TestATemplateStartRequiresItsOwnPack(t *testing.T) {
	dataRoot := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}
	req, _, err := pack.StoryRequestForTemplate("sect-disciple")
	if err != nil {
		t.Fatal(err)
	}
	req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
		PackID: "some-other-world", TemplateID: "sect-disciple",
	}
	if _, err := service.withPackTemplate(req); err == nil {
		t.Fatal("a template was resolved against a pack it does not belong to")
	}
}

func actorStateOf(t *testing.T, state map[string]any, actorID string) map[string]any {
	t.Helper()
	actors, ok := state["actors"].(map[string]any)
	if !ok {
		t.Fatalf("story state has no actors object: %T", state["actors"])
	}
	actor, ok := actors[actorID].(map[string]any)
	if !ok {
		t.Fatalf("story state has no actor %q", actorID)
	}
	values, ok := actor["state"].(map[string]any)
	if !ok {
		t.Fatalf("actor %q has no state object", actorID)
	}
	return values
}

func mustNumberForTest(t *testing.T, value any) float64 {
	t.Helper()
	number, ok := actorStateNumberForTest(value)
	if !ok {
		t.Fatalf("value %v is not a number", value)
	}
	return number
}

func actorStateNumberForTest(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	}
	return 0, false
}
