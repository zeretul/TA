package interactiveapp

import (
	"encoding/json"
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// Copy to internal/app/interactive/fantasy_conversation_local_affair_test.go.
// Static review candidate, not executed or gofmt'd by the independent reviewer.
// This is the Conversation boundary, not merely the Store's own private accessor.
func TestFantasyConversationRetainsPrivateAffairAfterColdRead(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	pack, err := storypack.NewLibrary(root).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	if err = pack.ValidateModules(root); err != nil {
		t.Fatal(err)
	}
	module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatal(err)
	}
	frozen := pack.FreezeCatalog()
	catalog := &interactive.PackCatalogSnapshot{ContentVersion: frozen.ContentVersion, TopicTerms: frozen.TopicTerms}
	witnessName := ""
	for _, p := range frozen.People {
		catalog.Characters = append(catalog.Characters, interactive.PackCatalogPerson{SourceKey: p.SourceKey, Name: p.Name, Country: p.Country, PlaceCandidate: p.PlaceCandidate, PlaceLabel: p.PlaceLabel})
		if p.SourceKey == "sg-p01" {
			witnessName = p.Name
		}
	}
	for _, p := range frozen.Places {
		catalog.Places = append(catalog.Places, interactive.PackCatalogPlace{Token: p.Token, Country: p.Country, Place: p.Place})
	}
	if witnessName == "" {
		t.Fatal("real pack lost sg-p01")
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "Conversation private local affair", ChoiceCount: 3, ActorState: &module.ActorState,
		PackBinding: pack.Binding(), PackCatalog: catalog,
		ProtagonistInitialState: pack.ProtagonistTemplates[0].State,
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	choices := []string{"观察周围", "当面询问", "暂时离开"}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "在渡鸦桥镇当面见玛蕾", Narrative: witnessName + "在桥头与你当面见面。",
		ActorOps: []interactive.ActorStateOp{
			{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点", Value: "渡鸦桥镇"},
			{Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色", Value: []any{"audit-holder"}},
		},
		TurnResult: &interactive.TurnResult{Choices: choices, CharacterFacts: &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "audit-holder", Name: witnessName, SourceKey: "sg-p01"}}}},
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	d := interactive.LocalAffairDefinition{
		ID: "audit-conversation-wet-basket", Title: "桥头湿篮", LocationID: "sg-l01",
		Hook: "同批谷袋只有一只底部湿了。", Cause: "只有一只篮曾放在漏雨棚角，水由篮底浸入。",
		Structure: interactive.LocalAffairStructure{Goal: "understand", Obstacle: "conflicting_accounts", Decision: "verify_or_trust", Consequence: "corrected_belief"},
		Methods: []interactive.LocalAffairMethod{
			{ID: "basket", Kind: "observation", LocationID: "sg-l01", Label: "检查篮底", Evidence: "外侧湿痕与棚角滴水形状相合。", Finding: "水从外侧篮底浸入。"},
			{ID: "holder", Kind: "witness", LocationID: "sg-l01", CharacterSourceKey: "sg-p01", Label: "询问经手人", Evidence: "我只把那一篮放在漏雨棚角，另一篮一直留在屋里。", Finding: "经手人确认两篮存放位置不同。"},
		},
	}
	source := "留意桥头湿篮，不替任何人接下义务。"
	opening, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: source, Narrative: d.Hook,
		TurnResult: &interactive.TurnResult{Choices: choices, World: &interactive.TurnWorldActionInput{Action: interactive.TurnWorldActionLocalAffair, SourceText: source, LocalAffair: &interactive.LocalAffairActionInput{Action: "encounter", ID: d.ID, Definition: &d}}},
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: opening.ID, Title: "未听证言的分支"})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "请当面说明两篮如何存放", Narrative: d.Methods[1].Evidence,
		TurnResult: &interactive.TurnResult{Choices: choices, CharacterFacts: &interactive.TurnCharacterFactsInput{Exchanges: []interactive.TurnCharacterExchangeInput{{CharacterID: "audit-holder", Speaker: "character", Channel: "scene", AddressedTo: interactive.DefaultActorID, Text: d.Methods[1].Evidence}}}},
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	chars, err := store.CharacterState(story.ID, "main")
	if err != nil || len(chars.Exchanges) != 1 {
		_ = store.Close()
		t.Fatalf("witness was not committed: %v %+v", err, chars.Exchanges)
	}
	exchangeID := chars.Exchanges[0].ID
	public, err := store.WorldStateFor(story.ID, "main")
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	raw, err := json.Marshal(public.LocalAffairs)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if strings.Contains(string(raw), d.Cause) || strings.Contains(string(raw), d.Methods[0].Evidence) {
		_ = store.Close()
		t.Fatal("private answer leaked to public projection", string(raw))
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	cold := interactive.NewStore(workspace)
	defer cold.Close()
	conversation := &Conversation{store: cold, storyID: story.ID}
	for _, branch := range []string{"main", fork.ID} {
		ctx, e := cold.StoryContext(story.ID, branch)
		if e != nil {
			t.Fatal(e)
		}
		text := conversation.storyWorldActionsContext(ctx, d.ID)
		for _, want := range []string{"地方小事", "frozen ", d.ID, d.Cause, d.Methods[0].Evidence, d.Methods[1].Evidence} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s narrator lost %q", branch, want)
			}
		}
		witness := "possible_witness method=holder exchange_id=" + exchangeID
		if strings.Contains(text, witness) != (branch == "main") {
			t.Fatalf("branch %s lost its own witness ID or received another branch's witness ID", branch)
		}
	}
}
