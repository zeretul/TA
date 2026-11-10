package interactive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ta/internal/quest"
)

func TestAnAuthoredPersonalMatterIsNotPlayerKnowledge(t *testing.T) {
	catalog := &PackCatalogSnapshot{ContentVersion: 17, Characters: []PackCatalogPerson{{SourceKey: "npc_su_qian", Name: "苏浅"}}}
	store, id := eligibilityTestStory(t, "越国坊市外围散修人家", fanrenBinding(17), catalog)
	state, err := store.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "苏浅") || strings.Contains(string(data), quest.TaskSuQianMaterials) {
		t.Fatalf("the player's task view revealed an unencountered person's matter: %s", data)
	}
}

func TestMeetingTheGiverIsNotKnowingOrAcceptingTheirPrivateTask(t *testing.T) {
	store, id := questTestStory(t)
	state := questTestState(t, store, id)
	if len(state.OfferCandidates) != 1 || state.OfferCandidates[0].TaskID != quest.TaskSuQianMaterials {
		t.Fatalf("narrator lost the present giver's possible conversation: %+v", state.OfferCandidates)
	}
	data, _ := json.Marshal(state)
	if strings.Contains(string(data), quest.TaskSuQianMaterials) {
		t.Fatalf("a private candidate became player knowledge: %s", data)
	}
	_, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept})
	if !errors.Is(err, ErrQuestNotHere) {
		t.Fatalf("ID alone accepted an unheard offer: %v", err)
	}
	if _, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionAccept}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicOpportunitiesHaveAnActualLocalSource(t *testing.T) {
	store, id := eligibilityTestStory(t, "黄枫谷外门居所", fanrenBinding(17), nil)
	state, err := store.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Available) != 0 {
		t.Fatalf("a distant notice appeared as local knowledge: %+v", state.Available)
	}
	store, id = eligibilityTestStory(t, "越国坊市外围散修人家", fanrenBinding(17), nil)
	state, err = store.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Available) != 1 || strings.Contains(state.Available[0].Brief, "托你") {
		t.Fatalf("public notice invented a prior commission: %+v", state.Available)
	}
}

func TestAForkBeforeTheDisclosureDoesNotKnowThePrivateTask(t *testing.T) {
	store, id := questTestStory(t)
	before, err := store.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer}); err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Snapshot.CurrentTurn.ID, Title: "告知之前"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.QuestStateFor(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 0 {
		t.Fatalf("a pre-disclosure fork inherited the private offer: %+v", state.Tasks)
	}
	data, _ := json.Marshal(state)
	if strings.Contains(string(data), quest.TaskSuQianMaterials) {
		t.Fatalf("fork leaked its parent's later disclosure: %s", data)
	}
}

func TestAnUnopenedStoryHasNoKnownOffers(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	system := journeyTestSystem("越国坊市外围散修人家")
	system.WorldContent = authoredWorldContent(t, "fanren-jie")
	story, err := store.CreateStory(CreateStoryRequest{Title: "未开场", ActorState: &system, PackBinding: fanrenBinding(17)})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.QuestStateFor(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Available) != 0 || len(state.Tasks) != 0 {
		t.Fatalf("a story that has not opened already knows tasks: %+v", state)
	}
}

func TestAReplacementDoesNotKnowThePersonIntroducedByItsOldTurn(t *testing.T) {
	store, id := eligibilityTestStory(t, "越国坊市外围散修人家", fanrenBinding(18), nil)
	before, err := store.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	questTestSource(t, store, id)
	after, err := store.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	live, err := store.TurnWorldView(id, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, known := live.Characters.Profile("su-qian"); !known {
		t.Fatal("the live view lost the person introduced by the committed turn")
	}
	replacement, err := store.TurnWorldView(id, "main", after.Snapshot.CurrentTurn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.LatestTurnID != before.Snapshot.CurrentTurn.ID {
		t.Fatalf("replacement discovery turn = %q, want %q", replacement.LatestTurnID, before.Snapshot.CurrentTurn.ID)
	}
	if replacement.Characters != nil {
		if _, known := replacement.Characters.Profile("su-qian"); known {
			t.Fatal("the replacement borrowed an introduction from the turn being discarded")
		}
	}
}
