package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorldRouteIsolationIncludesDirectCommit(t *testing.T) {
	for _, id := range []string{"lanchuan-qunxia", "xinghai-689", "sui-guan-realms"} {
		t.Run(id, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "storypack", "assets", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var pack struct {
				ContentVersion int `json:"content_version"`
				Story          struct {
					Gameplay struct {
						Rules   AdventureRules `json:"adventure_rules"`
						Content *WorldContent  `json:"world_content"`
					} `json:"gameplay"`
				} `json:"story"`
			}
			if err = json.Unmarshal(data, &pack); err != nil {
				t.Fatal(err)
			}
			rules := pack.Story.Gameplay.Rules
			for _, loc := range []string{rules.Markets[0].Location, "公共坊市"} {
				t.Run(loc, func(t *testing.T) {
					system := StoryDirectorActorStateSystem{WorldContent: pack.Story.Gameplay.Content, AdventureRules: &rules, Templates: []ActorStateTemplate{
						{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(rules)},
						{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: loc}}},
					}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: loc}}}}
					if ActorStateSystemIsXiuxian(system) {
						t.Fatal("audit fixture incorrectly enables cultivation")
					}
					store := NewStore(t.TempDir())
					t.Cleanup(func() { _ = store.Close() })
					story, err := store.CreateStory(CreateStoryRequest{Title: "Isolation audit", ActorState: &system, PackBinding: &StoryPackBinding{PackID: id, ContentVersion: pack.ContentVersion, Scope: "pack:" + id, Gameplay: &StoryGameplayProfile{Combat: rules.Family, Growth: rules.Family}}, PackCatalog: &PackCatalogSnapshot{ContentVersion: pack.ContentVersion}})
					if err != nil {
						t.Fatal(err)
					}
					options, err := store.JourneyOptionsFor(story.ID, "main")
					if err != nil {
						t.Fatal(err)
					}
					for _, r := range options.Routes {
						if strings.HasPrefix(r.OriginLocationID, "yue.") {
							t.Errorf("cross-world route offered: %s", r.RouteID)
						}
					}
					before, err := store.Snapshot(story.ID, "main")
					if err != nil {
						t.Fatal(err)
					}
					_, err = store.ApplyJourney(story.ID, JourneyRequest{BranchID: "main", RouteID: "route_036:forward", Arrive: true, CommandID: "audit-travel", CommandHash: "audit"})
					if err == nil {
						t.Fatal("foreign route committed through the direct API")
					}
					after, err := store.Snapshot(story.ID, "main")
					if err != nil {
						t.Fatal(err)
					}
					if before.ContextRevision != after.ContextRevision {
						t.Fatal("rejected foreign route changed the story")
					}

				})
			}
		})
	}
}

func authoredWorldContent(t *testing.T, id string) *WorldContent {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "storypack", "assets", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Story struct {
			Gameplay struct {
				Content *WorldContent `json:"world_content"`
			} `json:"gameplay"`
		} `json:"story"`
	}
	if err = json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Story.Gameplay.Content == nil {
		t.Fatal("missing authored content")
	}
	return p.Story.Gameplay.Content
}
