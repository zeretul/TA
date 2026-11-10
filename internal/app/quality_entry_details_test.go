package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

func qualityFixtureDetails(grade cultivation.QualityGrade) cultivation.DefinitionDetails {
	return cultivation.DefinitionDetails{QualityGrade: grade, Description: "测试简介", Background: "测试背景", Localizations: &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Description: "Fixture description", Background: "Fixture background"}}}
}

// This golden is serialized from the real store and application DTO. Frontend
// consumers import these bytes rather than inventing a mock response shape.
func TestQualityCultivationPublicSerializationAndColdBranch(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	root := t.TempDir()
	module, err := interactive.NewActorStateLibrary(root).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	system := module.ActorState
	system.DefinitionDetails = map[string]cultivation.DefinitionDetails{"fire_bolt": qualityFixtureDetails(cultivation.QualityWhite), "method.warm_origin": qualityFixtureDetails(cultivation.QualityBlue), "implement.upper.rare.effect": qualityFixtureDetails(cultivation.QualityPurple), "pill.restore.health.lower": qualityFixtureDetails(cultivation.QualityWhite), "implement.lower.common": qualityFixtureDetails(cultivation.QualityWhite), "secret.unlearned": {QualityGrade: cultivation.QualityRed, Description: "private-catalog-description", Background: "private-catalog-background"}}
	owned := map[string]any{"当前境界": "qi_3", "已学功法": map[string]any{"fire_bolt": map[string]any{"定义": "fire_bolt", "版本": float64(1), "掌握程度": "已学会", "描述": "own skill wording", "来历": "actual teacher"}}, "功法": map[string]any{"温元诀": map[string]any{"定义": "method.warm_origin", "版本": float64(1), "掌握程度": "已掌握", "运转": "停止", "描述": "own method wording", "来历": "actual manual"}}, "重要物品": map[string]any{"Owned blade": map[string]any{"定义": "implement.upper.rare.effect", "数量": float64(1), "描述": "real worn hilt", "来历": "actual gift"}, "Restoring pill": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2), "来源": "actual apothecary"}}}
	owned["主法器"] = map[string]any{"定义": "implement.lower.common", "名称": "Equipped focus", "数量": float64(1), "描述": "old grip wrapping", "来历": "actual school issue"}
	system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: owned}, {ID: "rival", Name: "Rival", TemplateID: interactive.DefaultActorID, State: map[string]any{"已学功法": map[string]any{"secret.enemy": map[string]any{"定义": "heavy_thunder", "版本": float64(1)}}}}}
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Quality fixture", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: root, interactive: store}
	first, err := application.CultivationEntryDetails(story.ID, "main", "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Skills) != 1 || len(first.Techniques) != 1 || len(first.Items) != 3 {
		t.Fatalf("owned projection: %+v", first)
	}
	if first.Skills[0].QualityGrade != cultivation.QualityWhite || first.Skills[0].Description != "Fixture description" || first.Skills[0].Source != "actual teacher" || first.Skills[0].InstanceDescription != "own skill wording" {
		t.Fatalf("metadata or provenance dropped: %+v", first.Skills[0])
	}
	if first.Items[2].Slot != "weapon" || first.Items[2].Source != "actual school issue" {
		t.Fatalf("equipped source/slot dropped: %+v", first.Items[2])
	}
	if first.Items[1].RestoreAmount == nil || *first.Items[1].RestoreAmount <= 0 || first.Items[1].Source != "actual apothecary" {
		t.Fatalf("derived pill effect or source dropped: %+v", first.Items[1])
	}
	if first.Items[0].StageEligible {
		t.Fatal("high-stage item incorrectly eligible")
	}
	data, err := json.MarshalIndent(first, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"secret.unlearned", "private-catalog", "secret.enemy", "localizations"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private field leaked: %s", private)
		}
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	projectInteractiveSnapshotForPlayer(&snapshot)
	projected, _ := json.Marshal(snapshot)
	if strings.Contains(string(projected), "private-catalog") {
		t.Fatal("metadata catalog leaked via schema")
	}
	// Mutating the input after creation cannot rewrite the frozen story.
	d := system.DefinitionDetails["fire_bolt"]
	d.Localizations.English.Background = "changed author background"
	system.DefinitionDetails["fire_bolt"] = d
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "You wait."})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Fork"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	application.interactive = store
	reopened, err := application.CultivationEntryDetails(story.ID, branch.ID, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	zh, err := application.CultivationEntryDetails(story.ID, branch.ID, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	checkQualityFixture(t, "cultivation-entry-details-zh-CN", zh)
	checkQualityFixture(t, "cultivation-entry-details-en-US", reopened)
	first.Head = "fixture-head"
	reopened.Head = "fixture-head"
	if !reflect.DeepEqual(first, reopened) {
		t.Fatalf("cold fork changed frozen definitions: first=%+v reopened=%+v", first, reopened)
	}

}

func checkQualityFixture(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["head"] = "fixture-head"
	data, err = json.MarshalIndent(object, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("testdata", name+".json")
	if os.Getenv("UPDATE_QUALITY_FIXTURE") == "1" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(data) {
		t.Fatalf("real DTO changed %s\n%s", name, data)
	}
}

func TestQualityAdventurePublicSerializationAndPrivacy(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	root := t.TempDir()
	rules := interactive.AdventureRules{Family: "wuxia", Currency: "Coins", StartingSkills: []string{"owned"}, StartingItems: map[string]int{"potion": 1}, Skills: []interactive.AdventureSkill{{DefinitionDetails: qualityFixtureDetails(cultivation.QualityWhite), ID: "owned", Name: "Known ability", Kind: "attack", Scale: "person", Power: 10, Cost: 5, RequiredRank: 1, TrainingHours: 2}, {DefinitionDetails: cultivation.DefinitionDetails{QualityGrade: cultivation.QualityRed, Description: "secret-unlearned-prose", Background: "secret-unlearned-background"}, ID: "secret-unlearned", RequiredItem: "secret-enemy-item", Name: "secret-unlearned-name", Kind: "attack", Scale: "person", Power: 40, Cost: 5, RequiredRank: 2, TrainingHours: 10}}, Items: []interactive.AdventureItem{{DefinitionDetails: qualityFixtureDetails(cultivation.QualityGreen), ID: "potion", Name: "Owned potion", Kind: "consumable", Scale: "person", Acquisition: "market", Price: 5, Effect: "heal", Power: 20}, {DefinitionDetails: qualityFixtureDetails(cultivation.QualityRed), ID: "secret-enemy-item", Name: "secret-enemy-item-name", Kind: "weapon", Scale: "person", Acquisition: "reward", Power: 50}}}
	actor := interactive.NewAdventureActor(rules)
	actor.Battle = &interactive.AdventureBattle{ID: "battle", OpponentID: "rival", OpponentName: "Visible rival", Scale: "person", Status: "active", Health: 100, MaxHealth: 100, Attack: 5, Defense: 2, EnemyRank: 2, EnemyMaxEnergy: 100, EnemyEnergy: 100, EnemySkillRanks: map[string]int{"secret-unlearned": 3}, EnemyInventory: map[string]int{"secret-enemy-item": 1}}
	raw, _ := json.Marshal(actor)
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(interactive.DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	system := module.ActorState
	system.AdventureRules = &rules
	for i := range system.Templates {
		if system.Templates[i].ID == interactive.DefaultActorID {
			system.Templates[i].Fields = interactive.AdventureStateFields(rules)
		}
	}
	system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Adventure fixture", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: root, interactive: store}
	before, err := store.AdventureWindow(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		view, err := application.AdventureWindow(story.ID, "main", locale)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Skills) != 1 || len(view.Items) != 1 {
			t.Fatalf("leaked unowned catalog: %+v", view)
		}
		// The legacy saved battle omits player_scale. Its public projection must
		// report the player's effective scale without exposing enemy mechanics.
		if view.Actor.Battle == nil || view.Actor.Battle.PlayerScale != "person" {
			t.Fatalf("legacy battle lost its effective player scale: %+v", view.Actor.Battle)
		}
		battleData, err := json.Marshal(view.Actor.Battle)
		if err != nil {
			t.Fatal(err)
		}
		var battleFields map[string]any
		if err := json.Unmarshal(battleData, &battleFields); err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{"health", "max_health", "attack", "defense", "enemy_rank", "enemy_inventory", "enemy_equipment", "enemy_skill_ranks", "enemy_energy", "enemy_max_energy", "enemy_retreat_progress", "ship_combat", "no_progress_rounds", "mindless", "character_source_key", "lethal"} {
			if _, exists := battleFields[private]; exists {
				t.Fatalf("private battle field leaked: %s", private)
			}
		}
		data, _ := json.Marshal(view)
		for _, private := range []string{"secret-unlearned", "secret-enemy-item", "enemy_inventory", "enemy_skill_ranks", "enemy_energy", "localizations"} {
			if strings.Contains(string(data), private) {
				t.Fatalf("private field leaked %s", private)
			}
		}
		if view.Skills[0].QualityGrade != cultivation.QualityWhite || view.Items[0].QualityGrade != cultivation.QualityGreen || view.Skills[0].Usable == nil || !*view.Skills[0].Usable {
			t.Fatalf("metadata or real preflight dropped: %+v", view)
		}
		checkQualityFixture(t, "adventure-window-"+locale, view)
	}
	after, err := store.AdventureWindow(story.ID, "main")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("details read mutated canonical state: %+v %v", after, err)
	}
}
