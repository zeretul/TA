package interactive

import (
	"math"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func TestNewSpiritBeastIdentityGrowthAndBranchSurviveReopen(t *testing.T) {
	store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(1)), cultivation.Qi(6))
	root := store.root
	before, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	beforeIdentity, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "未遇新灵兽"})
	if err != nil {
		t.Fatal(err)
	}
	const actorID = "石豆"
	introduced, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "认识石豆", Narrative: "一只名叫石豆的新生灵雀已能以本能吐纳修行，炼气一层，留在安全溪谷专心修炼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{Op: interactivestate.Create, Path: "/" + actorID,
				Value: map[string]any{"template_id": ActorStateImportantCharacterTemplateID, "name": "石豆", "state": spiritBeastGrowthSeclusionState(cultivation.Qi(1))}}},
			CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{
				CharacterID: actorID, Name: "石豆", Lifeform: character.LifeformSpiritBeast, CultivationMethodID: spiritBeastGrowthMethod,
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeGrowth, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: introduced.ID, Title: "已认识尚未修行"})
	if err != nil {
		t.Fatal(err)
	}
	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "new-beast-month")
	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "new-beast-month")
	check := func(reader *Store) {
		t.Helper()
		state, err := reader.CharacterState(storyID, "main")
		if err != nil {
			t.Fatal(err)
		}
		profile, found := state.Profile(actorID)
		if !found || profile.SourceKey != "" || profile.Lifeform != character.LifeformSpiritBeast || profile.CultivationMethodID != spiritBeastGrowthMethod {
			t.Fatalf("new identity lost its own qualification: %+v", profile)
		}
		snapshot, err := reader.Snapshot(storyID, "main")
		if err != nil {
			t.Fatal(err)
		}
		progress, _ := actorStateNumber(actorStateFieldValue(snapshot.State, actorID, xiuxianFieldRealmProgress))
		if math.Abs(progress-12) > 1e-9 {
			t.Fatalf("one legal month paid %v, want 12 exactly once", progress)
		}
		count := 0
		for _, record := range npcGrowthTestRecords(t, reader, storyID) {
			if record.Candidate.ActorID == actorID {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("new creature has %d growth records, want one", count)
		}
		old, err := reader.CharacterState(storyID, beforeIdentity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := old.Profile(actorID); found {
			t.Fatal("pre-identity branch inherited the new creature")
		}
		oldSnapshot, err := reader.Snapshot(storyID, beforeGrowth.ID)
		if err != nil {
			t.Fatal(err)
		}
		oldProgress, _ := actorStateNumber(actorStateFieldValue(oldSnapshot.State, actorID, xiuxianFieldRealmProgress))
		if oldProgress != 0 {
			t.Fatalf("pre-growth branch inherited %v", oldProgress)
		}
	}
	check(store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	check(reopened)
}
