package interactive

import (
	"fmt"
	"reflect"
	"ta/internal/character"
	"ta/internal/cultivation"
	"testing"
)

func TestRecordOnlyProfileRegistrationCannotBypassSourceNameOrAlias(t *testing.T) {
	catalog := &PackCatalogSnapshot{Characters: []PackCatalogPerson{{SourceKey: "history", Name: "Ancient", Aliases: []string{"Old Master"}, RecordOnly: true}}}
	for _, input := range []TurnCharacterProfileInput{{CharacterID: "a", SourceKey: "history", Name: "Another Name"}, {CharacterID: "a", Name: "Ancient"}, {CharacterID: "a", Name: "Old Master"}} {
		for _, existing := range [][]character.Profile{nil, {{ID: "a", Name: "Ancient", SourceKey: "history", Revision: 1}}} {
			if d := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
				&TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{input}}, existing, catalog); d == nil {
				t.Errorf("history registered live: %+v", input)
			}
		}
	}
	input := &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "living", Name: "Living Stranger"}}}
	if d := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
		input, nil, catalog); d != nil {
		t.Fatalf("unrelated person rejected: %+v", d)
	}
	legacy := ClonePackCatalogSnapshot(catalog)
	legacy.Characters[0].RecordOnly = false
	input.Profiles[0] = TurnCharacterProfileInput{CharacterID: "ancient", SourceKey: "history", Name: "Ancient"}
	if d := validateTurnCharacterQualifications(cultivation.ReferenceRules(),
		input, nil, legacy); d != nil {
		t.Fatalf("legacy availability rewritten: %+v", d)
	}
}

func TestRecordOnlyBothStoryWritersRejectAtomicallyAcrossColdReopenAndFork(t *testing.T) {
	for _, historical := range []bool{true, false} {
		t.Run(fmt.Sprint(historical), func(t *testing.T) {
			catalog := &PackCatalogSnapshot{ContentVersion: 6, Characters: []PackCatalogPerson{{SourceKey: "history", Name: "Ancient", Aliases: []string{"Old Master"}, RecordOnly: historical}}}
			store := NewStore(t.TempDir())
			defer store.Close()
			story, err := store.CreateStory(CreateStoryRequest{Title: "Frozen historical availability", PackCatalog: catalog})
			if err != nil {
				t.Fatal(err)
			}
			opening, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "Read", Narrative: "The traveler reads an old account of Ancient. The portrait is a historical record, not a meeting.", TurnResult: &TurnResult{Choices: testTurnChoices()}})
			if err != nil {
				t.Fatal(err)
			}
			fork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: opening.ID, Title: "Before any meeting"})
			if err != nil {
				t.Fatal(err)
			}
			// Changing current author data cannot revise either frozen branch, in either direction.
			catalog.Characters[0].RecordOnly = !historical
			cold := NewStore(store.Root())
			defer cold.Close()
			for _, branch := range []string{"main", fork.ID} {
				before, err := cold.StoryContext(story.ID, branch)
				if err != nil {
					t.Fatal(err)
				}
				if before.Meta.PackCatalog.Characters[0].RecordOnly != historical {
					t.Fatal("cold read rewrote frozen availability")
				}
				_, err = cold.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{BranchID: branch, WorldTime: "689", Scene: character.Scene{Location: "archive"}, Profiles: []character.Profile{{ID: "direct", Name: "Ancient", SourceKey: "history", Lifeform: PackCatalogLifeformHuman, Revision: 1}}})
				if (err != nil) != historical {
					t.Fatalf("direct writer historical=%v: %v", historical, err)
				}
				_, _, err = cold.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: branch, User: "Meet", Narrative: "Ancient appears.", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "main-person", Name: "Old Master", SourceKey: "history"}}}}})
				if (err != nil) != historical {
					t.Fatalf("main writer historical=%v: %v", historical, err)
				}
				after, err := cold.StoryContext(story.ID, branch)
				if err != nil {
					t.Fatal(err)
				}
				if historical && (!reflect.DeepEqual(before.Snapshot.State, after.Snapshot.State) || before.Snapshot.TurnCount != after.Snapshot.TurnCount || before.Meta.Branches[branch].Head != after.Meta.Branches[branch].Head) {
					t.Fatal("rejected appearance changed state, turn count or journal head")
				}
				state, err := cold.CharacterState(story.ID, branch)
				if err != nil {
					t.Fatal(err)
				}
				if historical && len(state.Profiles) != 0 {
					t.Fatal("rejected appearance left a registered participant")
				}
				if !historical && len(state.Profiles) == 0 {
					t.Fatal("old frozen live person disappeared")
				}
			}
		})
	}
}

func TestRecordOnlyNPCBackstopsPreventGrowthAndBattleWithoutRemovingLore(t *testing.T) {
	r := adventureTestRules()
	npc := NewAdventureActor(r)
	npc.Inventory = map[string]int{}
	r.NPCs = []AdventureNPC{{CharacterSourceKey: "history", Actor: npc, Plan: AdventureNPCPlan{Action: "train", SkillID: "strike", TrainingPercent: 100, RankLimit: 2}}}
	r.Opponents[0].CharacterSourceKey = "history"
	p := &storyBranchProjection{State: map[string]any{}, Characters: &character.State{Profiles: []character.Profile{{ID: "ancient", Name: "Ancient", SourceKey: "history", Revision: 1}}}, PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{SourceKey: "history", Name: "Ancient", RecordOnly: true}}}}
	system := StoryDirectorActorStateSystem{AdventureRules: &r}
	before := StoryClock{Version: StoryClockVersion}
	after := StoryClock{Version: StoryClockVersion, Seconds: 3600}
	events, head := adventureNPCGrowthEvents(p, system, "main", "head", "now", "", "", before, after)
	if len(events) != 0 || head != "head" {
		t.Fatal("historical profile acquired a living growth actor")
	}
	if _, _, err := adventureEncounterActor(r, p, AdventureAction{Action: "battle_start", OpponentID: r.Opponents[0].ID}); err == nil {
		t.Fatal("historical profile entered real combat")
	}
	p.PackCatalog.Characters[0].RecordOnly = false
	events, _ = adventureNPCGrowthEvents(p, system, "main", "head", "now", "", "", before, after)
	if len(events) == 0 {
		t.Fatal("old frozen living profile lost its ordinary growth")
	}
	id, _, err := adventureEncounterActor(r, p, AdventureAction{Action: "battle_start", OpponentID: r.Opponents[0].ID})
	if err != nil || id != "ancient" {
		t.Fatalf("old frozen participant cannot fight: %s %v", id, err)
	}
}
