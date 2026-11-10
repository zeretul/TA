package interactive

import (
	"testing"

	"ta/internal/character"
)

func TestRelationshipProposalCommitGuardianReopenAndFork(t *testing.T) {
	store, storyID := f17FlowStory(t)
	f17FlowMeet(t, store, storyID)
	for _, basis := range []string{"主角冒险替灵狐挡住落石", "主角守信把灵狐的幼崽送回溪谷", "主角在洪水中救下灵狐"} {
		f17FlowTurn(t, store, storyID, basis, basis, TurnCharacterFactsInput{
			Affinities: []TurnCharacterAffinityInput{{CharacterID: f17FlowBeast, Kind: "major", Delta: 15, Basis: basis}},
		})
	}
	facts := &TurnCharacterFactsInput{
		Exchanges:     f17FlowPair("从此一起走，可好？", "（灵狐点头，把尾巴搭在主角伸出的手上。）"),
		Relationships: []TurnCharacterRelationshipInput{{CharacterID: f17FlowBeast, Kind: "close_friend", Statement: "主角与灵狐成为亲近的同伴", ExpressedByIndex: 1, AnsweredByIndex: 1}},
	}
	cast, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	validation := TurnSubmissionContext{KnownCharacters: cast.CharacterIDs(), ExistingAffinities: cast.Affinities, ExistingProfiles: cast.Profiles,
		Scene: character.Scene{NodeID: "溪谷", Present: []string{DefaultActorID, f17FlowBeast}}, ChoiceCount: 5}
	prepared, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{CharacterFacts: facts})
	if prepared.TurnResult().CharacterFacts != nil || len(receipt.Diagnostics) == 0 {
		t.Fatalf("invalid proposal passed tool validation: %+v", receipt)
	}
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "一起走吗？", Narrative: "灵狐只看着主角，尚未答应。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}}); err == nil {
		t.Fatal("invalid proposal passed final commit")
	}
	unchanged, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Relationships) != 0 || len(unchanged.Exchanges) != len(cast.Exchanges) {
		t.Fatal("rejected relationship partially committed")
	}

	facts.Relationships[0].AnsweredByIndex = 2
	prepared, receipt = PrepareTurnSubmission(validation, prepared, TurnSubmissionInput{CharacterFacts: facts})
	if prepared.TurnResult().CharacterFacts == nil || len(receipt.Diagnostics) != 0 {
		t.Fatalf("repair was not accepted: %+v", receipt)
	}
	bondTurn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "从此一起走，可好？", Narrative: "灵狐点头，把尾巴搭在主角伸出的手上。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	before := f17FlowGuardianChance(t, store, storyID, "main")
	branch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: bondTurn.ID, Title: "仍在同行"})
	if err != nil {
		t.Fatal(err)
	}
	f17FlowTurn(t, store, storyID, "以后各走各的", "主角当面说明不再结伴，双方仍站在溪畔。", TurnCharacterFactsInput{
		Exchanges:     []TurnCharacterExchangeInput{{CharacterID: f17FlowBeast, Speaker: "player", Channel: "scene", AddressedTo: f17FlowBeast, Text: "以后各走各的。"}},
		Relationships: []TurnCharacterRelationshipInput{{CharacterID: f17FlowBeast, Kind: "estranged", Statement: "主角与灵狐已解除结伴", ExpressedByIndex: 1}},
	})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	mainCast, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	current, ok := mainCast.RelationshipFor(f17FlowBeast)
	if !ok || current.Kind != character.RelationshipEstranged || len(mainCast.Relationships) != 2 {
		t.Fatalf("relationship history/current projection: %+v", mainCast.Relationships)
	}
	if after := f17FlowGuardianChance(t, reopened, storyID, "main"); after >= before {
		t.Fatalf("ended relationship still gives a guardian bonus: before=%v after=%v", before, after)
	}
	if fork := f17FlowGuardianChance(t, reopened, storyID, branch.ID); fork != before {
		t.Fatalf("later parting leaked into earlier branch: before=%v fork=%v", before, fork)
	}
}

func TestRelationshipProposalPreservesSourceBoundDirectedTarget(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "同一人物"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "见到宋观澜", Narrative: "宋观澜抬起头。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{CharacterID: "宋观澜", Name: "宋观澜", SourceKey: "npc_song", Role: "阵法学徒"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "告诉宋师兄以后各行其路", Narrative: "主角对宋观澜说，以后各行其路。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
		Profiles:      []TurnCharacterProfileInput{{CharacterID: "宋师兄", Name: "宋师兄", SourceKey: "npc_song"}},
		Exchanges:     []TurnCharacterExchangeInput{{CharacterID: "宋师兄", Speaker: "player", Channel: "scene", AddressedTo: "宋师兄", Text: "以后各行其路。"}},
		Relationships: []TurnCharacterRelationshipInput{{CharacterID: "宋师兄", Kind: "estranged", Statement: "双方分道扬镳", ExpressedByIndex: 1}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	cast, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	current, ok := cast.RelationshipFor("宋观澜")
	if !ok || current.Kind != character.RelationshipEstranged || len(cast.Profiles) != 1 || cast.Exchanges[0].AddressedTo != "宋观澜" {
		t.Fatalf("source-bound target changed: %+v", cast)
	}
}
