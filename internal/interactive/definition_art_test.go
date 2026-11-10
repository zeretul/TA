package interactive

import (
	"reflect"
	"sync"
	"sync/atomic"
	"ta/internal/cultivation"
	"testing"
)

func TestDefinitionArtReceiptAtMostOnceColdReadAndFacts(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(definitionArtStoryRequest(t, "Art receipt"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "A skill is registered."})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	job := DefinitionArtJob{DefinitionID: "fire_bolt", Version: 1, Category: "skill", Name: "Fire", Style: "realistic"}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.TransitionDefinitionArt(story.ID, "main", first.ID, job, "", "queued", nil)
			if err != nil {
				t.Error(err)
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("duplicate claims: %d", winners.Load())
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "The story continues while art runs."})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := store.TransitionDefinitionArt(story.ID, "main", first.ID, job, "queued", "executing", nil); err != nil || !ok {
		t.Fatalf("start %v %v", ok, err)
	}
	art := &cultivation.DefinitionArt{Source: "generated", ImagePath: "assets/image/generated/fire/image.png", ProjectID: "project-art"}
	if ok, err := store.TransitionDefinitionArt(story.ID, "main", first.ID, job, "executing", "succeeded", art); err != nil || !ok {
		t.Fatalf("settle after newer turn %v %v", ok, err)
	}
	if ok, _ := store.TransitionDefinitionArt(story.ID, "main", first.ID, job, "queued", "executing", nil); ok {
		t.Fatal("settled image could be billed again")
	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || after.CurrentTurn.ID != second.ID {
		t.Fatal("art changed game facts or moved head")
	}
	fork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: second.ID, Title: "Art fork"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.TransitionDefinitionArt(story.ID, fork.ID, first.ID, job, "", "queued", nil); ok {
		t.Fatal("fork rebilled same immutable definition")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	defer store.Close()
	refs, err := store.DefinitionArtReferences(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(refs["fire_bolt:1"], *art) {
		t.Fatalf("cold read lost art: %+v", refs)
	}
}

func TestDefinitionArtRejectsUnknownDefinitionAndKeepsFailureTerminal(t *testing.T) {
	store := NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(definitionArtStoryRequest(t, "Art fail"))
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Wait"})
	if err != nil {
		t.Fatal(err)
	}
	job := DefinitionArtJob{DefinitionID: "unknown", Version: 1, Category: "equipment", Name: "Ghost", Style: "realistic"}
	if ok, err := store.TransitionDefinitionArt(story.ID, "main", turn.ID, job, "", "queued", nil); err == nil || ok {
		t.Fatal("unregistered object generated art")
	}
	job.DefinitionID = "implement.lower.common"
	if ok, err := store.TransitionDefinitionArt(story.ID, "main", turn.ID, job, "", "queued", nil); err != nil || !ok {
		t.Fatalf("registered equipment unavailable: %v", err)
	}
	if ok, err := store.TransitionDefinitionArt(story.ID, "main", turn.ID, job, "queued", "failed", nil); err != nil || !ok {
		t.Fatal(err)
	}
	if ok, _ := store.TransitionDefinitionArt(story.ID, "main", turn.ID, job, "", "queued", nil); ok {
		t.Fatal("failed image automatically retried")
	}
	refs, err := store.DefinitionArtReferences(story.ID, "main")
	if err != nil || len(refs) != 0 {
		t.Fatalf("failure published image: %+v %v", refs, err)
	}
}

func TestNewDefinitionArtJobsOnlyFormalGeneration(t *testing.T) {
	system := xiuxianSkillTestSystem()
	before := xiuxianSkillTestState(t, "qi_3", nil)
	ops, result, ok := xiuxianSkillLearningOps(before, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: "Moonfire", Source: "Teacher", Description: "Pale flame"}}}, "art-test")
	if !ok || len(result.Refused) > 0 {
		t.Fatalf("learning refused: %+v", result)
	}
	after := appliedXiuxianState(t, before, ops)
	jobs := NewGeneratedDefinitionArtJobs(before, after, "realistic")
	if len(jobs) != 1 || jobs[0].Name != "Moonfire" || jobs[0].Version != 1 {
		t.Fatalf("formal generation not discovered: %+v", jobs)
	}
	if jobs := NewGeneratedDefinitionArtJobs(after, after, "realistic"); len(jobs) != 0 {
		t.Fatalf("old catalog bulk generated: %+v", jobs)
	}
}

func TestDefinitionArtAlternativeBranchDoesNotLeakPixels(t *testing.T) {
	store := NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(definitionArtStoryRequest(t, "Art branch"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Crossroads."})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: base.ID, Title: "Alternative"})
	if err != nil {
		t.Fatal(err)
	}
	alternative, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: fork.ID, Narrative: "A different technique is seen here."})
	if err != nil {
		t.Fatal(err)
	}
	job := DefinitionArtJob{DefinitionID: "fire_bolt", Version: 1, Category: "skill", Name: "Fire", Style: "realistic"}
	for _, transition := range [][2]string{{"", "queued"}, {"queued", "executing"}} {
		if ok, err := store.TransitionDefinitionArt(story.ID, fork.ID, alternative.ID, job, transition[0], transition[1], nil); err != nil || !ok {
			t.Fatal(err)
		}
	}
	art := &cultivation.DefinitionArt{Source: "generated", ProjectID: "project", ImagePath: "assets/image/generated/alternative.png"}
	if ok, err := store.TransitionDefinitionArt(story.ID, fork.ID, alternative.ID, job, "executing", "succeeded", art); err != nil || !ok {
		t.Fatal(err)
	}
	mainRefs, err := store.DefinitionArtReferences(story.ID, "main")
	if err != nil || len(mainRefs) != 0 {
		t.Fatalf("alternative art leaked: %+v %v", mainRefs, err)
	}
	forkRefs, err := store.DefinitionArtReferences(story.ID, fork.ID)
	if err != nil || len(forkRefs) != 1 {
		t.Fatalf("own branch art missing: %+v %v", forkRefs, err)
	}
}

func definitionArtStoryRequest(t *testing.T, title string) CreateStoryRequest {
	t.Helper()
	module, err := NewActorStateLibrary(t.TempDir()).Get(ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	return CreateStoryRequest{Title: title, ActorState: &module.ActorState}
}
