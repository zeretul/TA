package interactive

import (
	"strings"
	"testing"

	"ta/internal/character"
)

func TestNPCSocialFactsKeepDirectionPrivacyAndCurrentStateOnReopen(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "人物关系记录"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第1日",
		Profiles: []character.Profile{
			{ID: "father", Name: "林远", Age: 48, Description: "林远是林枝的父亲。"},
			{ID: "daughter", Name: "林枝", Age: 24, Gender: character.GenderFemale, Description: "林枝是林远的女儿。"},
			{ID: "other", Name: "路人"},
		},
		Facts: character.TurnCharacterFacts{Knowledge: []character.KnowledgeDraft{
			{CharacterID: "father", Statement: "林远(father)是林枝(daughter)的父亲；当前关系已确认。", Nature: character.NatureExperienced, Audience: character.AudienceParticipants, Holders: []string{"father", "daughter"}},
			{CharacterID: "daughter", Statement: "林枝(daughter)与陈舟目前为正式配偶；这是本人确认的既有关系。", Nature: character.NatureExperienced, Audience: character.AudienceSingle},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Knowledge) != 2 {
		t.Fatalf("initial facts %d", len(initial.Knowledge))
	}
	_, err = store.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第2日",
		Facts: character.TurnCharacterFacts{Knowledge: []character.KnowledgeDraft{
			{CharacterID: "daughter", Statement: "林枝与陈舟已正式离婚；双方明确结束婚姻并完成离异，当前不是配偶。", Nature: character.NatureExperienced, Audience: character.AudienceSingle, Supersedes: initial.Knowledge[1].ID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	defer reopened.Close()
	state, err := reopened.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := state.Profile("daughter")
	if !ok || p.Age != 24 {
		t.Fatalf("identity not retained: %+v", p)
	}
	seenReplacement := false
	for _, k := range state.Knowledge {
		if k.ID == initial.Knowledge[1].ID && k.SupersededBy == "" {
			t.Fatal("previous marriage still current")
		}
		if k.Supersedes == initial.Knowledge[1].ID && strings.Contains(k.Statement, "正式离婚") {
			seenReplacement = true
		}
	}
	if !seenReplacement {
		t.Fatal("relationship change missing")
	}
	outsider := character.Prepare(character.PrepareInput{Profile: character.Profile{ID: "other", Name: "路人"}, Knowledge: state.Knowledge}).Markdown()
	if strings.Contains(outsider, "林枝与陈舟") || strings.Contains(outsider, "林远(father)") {
		t.Fatal("private family facts leaked to outsider")
	}
}
