package history

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAdviceJournalLifecycleAndRestore(t *testing.T) {
	store, s := worldCampaign(t)
	ctx := context.Background()
	apply := func(id, kind string, advice *AdviceChange) {
		t.Helper()
		r, err := store.Apply(ctx, s.ID, Command{ID: id, Kind: kind, ExpectedRevision: s.Revision, Advice: advice})
		if err != nil {
			t.Fatal(err)
		}
		s = r.State
	}
	r, err := store.Apply(ctx, s.ID, Command{ID: "discussion", Kind: "chat", ExpectedRevision: s.Revision, Chat: &ChatTurn{Mode: "private", MinisterIDs: []string{"bi-ziyan"}, UserMessage: "What would help?", Replies: []Reply{{MinisterID: "bi-ziyan", Text: "An unapproved relief proposal"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	before := cloneState(s)
	apply("save", "advice", &AdviceChange{Action: "save", ChatID: "discussion", MinisterID: "bi-ziyan"})
	if len(s.Advice) != 1 || s.Advice[0].Text != "An unapproved relief proposal" || !reflect.DeepEqual(s.World, before.World) {
		t.Fatal("Bookmark changed facts or lost provenance")
	}
	id := s.Advice[0].ID
	apply("same-reply", "advice", &AdviceChange{Action: "save", ChatID: "discussion", MinisterID: "bi-ziyan"})
	if len(s.Advice) != 1 {
		t.Fatal("Duplicate bookmark")
	}
	apply("annotate", "advice", &AdviceChange{Action: "update", ID: id, Title: "Disaster relief", Notes: "Consider next season"})
	r, err = store.Apply(ctx, s.ID, Command{ID: "checkpoint", Kind: "checkpoint", Label: "Saved advice", ExpectedRevision: s.Revision})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	checkpoint := s.Checkpoints[len(s.Checkpoints)-1].ID
	apply("remove", "advice", &AdviceChange{Action: "remove", ID: id})
	if len(s.Advice) != 0 || len(s.Chats) != 1 {
		t.Fatal("Removal affected the source")
	}
	r, err = store.Apply(ctx, s.ID, Command{ID: "restore", Kind: "restore", CheckpointID: checkpoint, ExpectedRevision: s.Revision})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	cold, err := NewStore(filepath.Dir(store.root)).Read(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cold.Advice) != 1 || cold.Advice[0].Notes != "Consider next season" {
		t.Fatal("Cold restore lost advice")
	}
	if _, err = store.Apply(ctx, s.ID, Command{ID: "fake", Kind: "advice", ExpectedRevision: s.Revision, Advice: &AdviceChange{Action: "save", ChatID: "other-campaign", MinisterID: "bi-ziyan"}}); err == nil {
		t.Fatal("Forged source accepted")
	}
}
