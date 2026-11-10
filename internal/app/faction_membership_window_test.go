package app

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestConfirmedFactionIdentityReachesCharacterWindowWithEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, affiliation         string
		confirmed, wantMembership bool
	}{
		{"confirmed affiliation", "yue.huangfeng", true, true},
		{"confirmed unaffiliated", "none", true, false},
		{"unconfirmed text only", "yue.huangfeng", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := interactive.NewStore(t.TempDir())
			t.Cleanup(func() { _ = store.Close() })
			application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}, interactive: store}
			service := &InteractiveAppService{app: application}
			pack, err := storypack.NewLibrary(root).Get("fanren-jie")
			if err != nil {
				t.Fatal(err)
			}
			templateID, locationID := "sect-disciple", "yue.huangfeng.approach"
			if tc.affiliation == "none" {
				templateID, locationID = "loose-cultivator", "yue.market"
			}
			req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), storypack.IdentityChoice{
				Name: "林照", Age: 23, OriginID: "custom", OriginText: "远行者", StateBasisTemplateID: templateID, AffiliationID: tc.affiliation, LocationID: locationID, RootCategoryID: "dual", RootAttributes: []string{"wood", "water"}, AptitudeID: "good",
			})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.confirmed {
				req.IdentityConfirmed = false
				req.InitialFactionMembership = nil
			}
			if tc.affiliation == "none" && req.InitialFactionMembership != nil {
				t.Fatal("unaffiliated creation produced membership")
			}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatal(err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatal(err)
			}
			// Display prose, including a structured formal claim, is not admission evidence.
			for i := range req.ActorState.InitialActors {
				actor := &req.ActorState.InitialActors[i]
				if actor.ID == "protagonist" {
					if actor.State == nil {
						actor.State = map[string]any{}
					}
					actor.State["所属势力"] = []any{"黄枫谷", map[string]any{"faction_id": "yue.huangfeng", "name": "黄枫谷", "status": "formal"}}
				}
			}
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			context, err := store.StoryContext(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if actorStateOf(t, context.Snapshot.State, "protagonist")["所属势力"] == nil {
				t.Fatal("affiliation display fixture did not survive creation")
			}
			window, err := application.CharacterWindow(CharacterWindowRequest{StoryID: story.ID})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantMembership {
				if len(window.FactionMemberships) != 0 {
					t.Fatalf("display text granted membership: %+v", window.FactionMemberships)
				}
				return
			}
			if len(window.FactionMemberships) != 1 {
				t.Fatalf("confirmed membership missing: %+v", window.FactionMemberships)
			}
			m := window.FactionMemberships[0]
			if m.FactionID != "yue.huangfeng" || m.FactionName != "黄枫谷" || m.Status != "formal" || m.SourceKind != "identity" || m.SourceEventID == "" || m.ID == "" || m.Statement == "" || m.WorldTime == "" || m.ClockSeconds != 0 {
				t.Fatalf("known membership lost evidence: %+v", m)
			}
			if err := m.Validate(); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(window)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Memberships []character.FactionMembership `json:"faction_memberships"`
			}
			if err = json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Memberships) != 1 || !strings.Contains(string(raw), m.SourceEventID) {
				t.Fatal("window JSON dropped membership evidence")
			}
			window.FactionMemberships[0].Status = "left"
			again, err := application.CharacterWindow(CharacterWindowRequest{StoryID: story.ID})
			if err != nil || again.FactionMemberships[0].Status != "formal" {
				t.Fatalf("window mutation changed journal: %+v, %v", again.FactionMemberships, err)
			}
		})
	}
}
