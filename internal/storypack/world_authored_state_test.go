package storypack

import (
	"encoding/json"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
)

func TestAuthoredWorldStateSchemaValidation(t *testing.T) {
	g := RecommendedWorldGameplay()
	g.Confirmed = true
	valid := interactive.ActorStateField{Name: "世界事实", Type: "object"}
	cases := []struct {
		name    string
		fields  []interactive.ActorStateField
		invalid bool
	}{
		{"object", []interactive.ActorStateField{valid}, false},
		{"unknown type", []interactive.ActorStateField{{Name: "事实", Type: "magic"}}, true},
		{"duplicate", []interactive.ActorStateField{valid, valid}, true},
		{"action collision", []interactive.ActorStateField{{Name: "观察", Type: "string"}}, true},
		{"path separator", []interactive.ActorStateField{{Name: "nested/value", Type: "object"}}, true},
		{"implicit grant", []interactive.ActorStateField{{Name: "钱财", Type: "number", Default: 99}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g.AuthoredStateFields = tc.fields
			err := ValidateWorldGameplay(g)
			if (err != nil) != tc.invalid {
				t.Fatalf("validation=%v invalid=%v", err, tc.invalid)
			}
		})
	}
	g.AuthoredStateFields = []interactive.ActorStateField{valid}
	a, _ := gameplayModuleIDs("probe", g)
	g.AuthoredStateFields[0].Description = "changed schema contract"
	b, _ := gameplayModuleIDs("probe", g)
	if a == b {
		t.Fatal("authored schema was omitted from module identity")
	}
}

// This checks exact author initialization through real module persistence and
// Store cold load. It is a deterministic fixture, not an AI opening test.
func TestRuleWorldCandidateInitialFactsRoundTrip(t *testing.T) {
	for _, p := range ruleWorldCandidates(t) {
		t.Run(p.ID, func(t *testing.T) {
			var err error
			if err = p.Validate(); err != nil {
				t.Fatal("candidate validation:", err)
			}
			root := t.TempDir()
			lib := NewLibrary(root)
			if _, err = lib.materializeGameplayModules(&p); err != nil {
				t.Fatal("module materialization:", err)
			}
			if _, err = lib.materializeGameplayModules(&p); err != nil {
				t.Fatal("module reload:", err)
			}
			module, err := interactive.NewActorStateLibrary(root).Get(p.Story.ModuleRefs.ActorStateID)
			if err != nil {
				t.Fatal(err)
			}
			for _, origin := range p.ProtagonistTemplates {
				t.Run(origin.ID, func(t *testing.T) {
					raw, _ := json.Marshal(module.ActorState)
					var system interactive.StoryDirectorActorStateSystem
					if err = json.Unmarshal(raw, &system); err != nil {
						t.Fatal(err)
					}
					declared := map[string]bool{}
					for _, template := range system.Templates {
						if template.ID == interactive.DefaultActorID {
							for _, field := range template.Fields {
								declared[field.Name] = true
							}
						}
					}
					for field := range origin.State {
						if !declared[field] {
							t.Fatalf("author initial field missing from module: %s", field)
						}
					}
					found := false
					for i := range system.InitialActors {
						if system.InitialActors[i].ID == interactive.DefaultActorID {
							system.InitialActors[i].State = origin.State
							found = true
						}
					}
					if !found {
						t.Fatal("protagonist actor missing")
					}
					request, _, err := p.StoryRequestForTemplate(origin.ID)
					if err != nil {
						t.Fatal(err)
					}
					request.ActorState = &system
					// No model is requested in this storage probe; adapted opening remains a
					// separate validation requirement, so do not label this a playable start.
					request.StateSchemaPolicy = &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate}
					store := interactive.NewStore(root)
					story, err := store.CreateStory(request)
					if err != nil {
						store.Close()
						t.Fatal(err)
					}
					if err = store.Close(); err != nil {
						t.Fatal(err)
					}
					store = interactive.NewStore(root)
					ctx, err := store.StoryContext(story.ID, "main")
					store.Close()
					if err != nil {
						t.Fatal(err)
					}
					actors := ctx.Snapshot.State["actors"].(map[string]any)
					actor := actors[interactive.DefaultActorID].(map[string]any)
					state := actor["state"].(map[string]any)
					for key, want := range origin.State {
						if interactive.ContentHashOf(state[key]) != interactive.ContentHashOf(want) {
							t.Fatalf("cold load changed %s: got=%v want=%v", key, state[key], want)
						}
					}
				})
			}
		})
	}
}

func TestRuleWorldCandidateFullLorePersistence(t *testing.T) {
	for _, p := range ruleWorldCandidates(t) {
		t.Run(p.ID, func(t *testing.T) {
			var err error
			if err := p.Validate(); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			store := booklore.NewScopedStore(root, p.WorldScope())
			inputs := p.WorldInputs()
			ops := make([]booklore.Operation, 0, len(inputs))
			public := 0
			for _, input := range inputs {
				ops = append(ops, booklore.Operation{Op: "create", Item: input})
				if input.Audience != "gm" {
					public++
				}
			}
			if _, err := store.ApplyOperations("", ops); err != nil {
				t.Fatal("full lore import:", err)
			}
			cold := booklore.NewScopedStore(root, p.WorldScope())
			all, err := cold.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != len(inputs) {
				t.Fatalf("lost lore records: got %d want %d", len(all), len(inputs))
			}
			visible, err := cold.WithVisibility(booklore.IsPublic).List()
			if err != nil {
				t.Fatal(err)
			}
			if len(visible) != public {
				t.Fatalf("public projection: got %d want %d", len(visible), public)
			}
			if len(p.Characters) == 0 {
				t.Fatal("author discovery catalog missing")
			}
			hash := p.ContentHash()
			for _, person := range p.Characters {
				id := interactive.PackEntryIDForRevision(person.LoreID, hash)
				item, err := cold.Read(id)
				if err != nil {
					t.Fatal("author catalog body missing:", person.SourceKey, err)
				}
				if item.Audience != "gm" {
					t.Fatalf("unreviewed character body became public: %s", person.SourceKey)
				}
			}
			t.Logf("actual imported lore=%d public=%d gm=%d author characters=%d", len(all), public, len(all)-public, len(p.Characters))
		})
	}
}

// Read the same compiled assets the catalog exposes, without host-specific paths.
func ruleWorldCandidates(t *testing.T) []Pack {
	t.Helper()
	wanted := map[string]bool{
		"doupo-continent-ta07-full-gm-candidate":           true,
		"harry-potter-1991-ta07-full-gm-candidate":         true,
		"harry-potter-1991-student-ta07-full-gm-candidate": true,
		"lord-of-mysteries-ta07-full-gm-candidate":         true,
		"zichuan-xichuan-ta07-full-gm-candidate":           true,
		"sanguo-190-ta07-full-gm-candidate":                true,
	}
	var packs []Pack
	for _, p := range builtinPacks() {
		if wanted[p.ID] {
			packs = append(packs, p)
			delete(wanted, p.ID)
		}
	}
	if len(wanted) != 0 {
		t.Fatalf("compiled rule world candidates missing: %v", wanted)
	}
	return packs
}
