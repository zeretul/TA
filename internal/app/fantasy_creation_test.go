package app

import (
	"fmt"
	"strings"
	"ta/config"
	"ta/internal/catalogart"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

// Product creation, not a model-driven opening or a browser test.
func TestFantasyEveryOriginStartsThroughProductDefaults(t *testing.T) {
	root := t.TempDir()
	service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
	p, err := storypack.NewLibrary(root).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range p.ProtagonistTemplates {
		t.Run(origin.ID, func(t *testing.T) {
			req, _, e := withConfirmedIdentity(p, p.StoryRequest(), storypack.IdentityChoice{Name: "Rowan", Age: 25, Gender: "unspecified", OriginID: origin.ID, AffiliationID: "none", LocationID: "auto"})
			if e != nil {
				t.Fatal(e)
			}
			expectedLocation := map[string]string{"sg-start-heir": "渡鸦桥镇", "sg-start-road": "渡鸦桥镇", "sg-start-learner": "镜湖镇", "sg-start-scout": "沉钟林口", "sg-start-smith": "冷锤工坊", "sg-start-carer": "春雪修院", "sg-start-grove": "根桥聚落", "sg-start-runner": "黑蜡下街"}[origin.ID]
			if expectedLocation == "" || !strings.Contains(req.Origin, expectedLocation) {
				t.Fatal("confirmed identity lost authored starting place", req.Origin)
			}
			req, e = service.withStoryCreationDefaults(req)
			if e != nil {
				t.Fatal(e)
			}
			req, e = service.withPackTemplate(req)
			if e != nil {
				t.Fatal(e)
			}
			store := interactive.NewStore(t.TempDir())
			defer store.Close()
			story, e := store.CreateStory(req)
			if e != nil {
				t.Fatal(e)
			}
			snapshot, e := store.Snapshot(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			a, e := interactive.AdventureActorFromState(snapshot.State, interactive.DefaultActorID)
			if e != nil {
				t.Fatal(e)
			}
			expectedRank, expectedCurrency := 1, 24
			if origin.ID == "sg-start-heir" {
				expectedRank, expectedCurrency = 10, 3000
			}
			if a.Family != "fantasy" || a.Rank != expectedRank || a.Currency != expectedCurrency {
				t.Fatalf("origin was replaced: %+v", a)
			}
			if e = interactive.ValidateAdventureActor(*p.Story.Gameplay.AdventureRules, a); e != nil {
				t.Fatal(e)
			}
			view, e := (&App{interactive: store}).AdventureWindow(story.ID, "main", "en-US")
			if e != nil {
				t.Fatal(e)
			}
			for _, skill := range view.Skills {
				expected := catalogart.Default().AssetID(p.ID, "definition", skill.ID, 1, "illustration")
				if expected != "" && (skill.Art == nil || skill.Art.Source != "catalog" || skill.Art.CatalogID != expected) {
					t.Fatal("created story lost exact learned-skill image", skill.ID)
				}
			}
			fields := actorStateOf(t, snapshot.State, "protagonist")
			if !strings.Contains(fmt.Sprint(fields["当前处境"]), expectedLocation) {
				t.Fatal("frozen identity lost actual starting place", fields["当前处境"])
			}
			if _, ok := fields["精力"]; !ok {
				t.Fatal("effort not projected")
			}
			if _, ok := fields["内力"]; ok {
				t.Fatal("wuxia state leaked")
			}
		})
	}
}

func TestFantasyProductCatalogSupportsGenericLocalAffair(t *testing.T) {
	root := t.TempDir()
	service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
	p, err := storypack.NewLibrary(root).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := p.StoryRequestForTemplate("sg-start-road")
	if err != nil {
		t.Fatal(err)
	}
	req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: p.ID, TemplateID: "sg-start-road"}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatal(err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	// Creation intentionally lets the first narrated turn establish its scene.
	// This deterministic opening records the known origin, not a live AI result.
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看看眼前", Narrative: "你站在渡鸦桥镇桥头。", ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点", Value: "渡鸦桥镇"}}, TurnResult: &interactive.TurnResult{Choices: []string{"检查篮底", "看看附近", "继续赶路"}}})
	if err != nil {
		t.Fatal(err)
	}
	d := interactive.LocalAffairDefinition{ID: "product-wet-basket", Title: "湿篮", LocationID: "sg-l01", Hook: "只有一只篮底湿了。", Cause: "漏雨处的积水浸湿了寄放的篮底。", Structure: interactive.LocalAffairStructure{Goal: "understand", Obstacle: "conflicting_accounts", Decision: "verify_or_trust", Consequence: "corrected_belief"}, Methods: []interactive.LocalAffairMethod{{ID: "basket", Kind: "observation", LocationID: "sg-l01", Label: "看篮底", Evidence: "湿痕在篮外侧。", Finding: "水由外侧浸入。"}, {ID: "roof", Kind: "observation", LocationID: "sg-l02", Label: "看寄放处", Evidence: "破棚角有积水。", Finding: "积水与篮底痕迹吻合。"}}}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看一看桥头", Narrative: "桥头有一只湿篮。", TurnResult: &interactive.TurnResult{Choices: []string{"检查篮底", "看看附近", "继续赶路"}, World: &interactive.TurnWorldActionInput{Action: interactive.TurnWorldActionLocalAffair, SourceText: "看一看桥头", LocalAffair: &interactive.LocalAffairActionInput{Action: "encounter", ID: d.ID, Definition: &d}}}})
	if err != nil {
		t.Fatal("actual product-frozen catalog blocked generic local affair", err)
	}
}
