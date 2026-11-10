package loreapp

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	booklore "ta/internal/book/lore"
)

type seedTestHost struct{ store *booklore.Store }

func (h seedTestHost) WithLoreStore(_ context.Context, _ string, action func(*booklore.Store) error) (string, error) {
	return "test", action(h.store)
}
func (h seedTestHost) ClassifyLoreItems(context.Context, string, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	return nil, nil
}
func seedService(root string) (*Service, *booklore.Store) {
	store := booklore.NewStore(root)
	return NewService(seedTestHost{store}, nil), store
}

func TestCreateItemsDistinctIDNameCollisionIsAtomic(t *testing.T) {
	root := t.TempDir()
	service, store := seedService(root)
	if _, err := store.Create(booklore.ItemInput{ID: "saved", Name: "Saved", Content: "unchanged"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(booklore.ItemsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	inputs := []booklore.ItemInput{{ID: "first", Scope: "one", Name: "Same", Content: "first facet"}, {ID: "second", Scope: "one", Name: "Same", Content: "second facet"}}
	if _, err := service.CreateItems(context.Background(), "project", inputs); err == nil {
		t.Fatal("distinct same-name IDs must return an explicit conflict")
	}
	after, err := os.ReadFile(booklore.ItemsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed batch partially changed the persisted collection")
	}
}

func TestCreateItemsKeepsStableIDAndUserEditOnReorderedReplay(t *testing.T) {
	root := t.TempDir()
	service, store := seedService(root)
	inputs := []booklore.ItemInput{{ID: "facet-a", Scope: "one", Name: "Person〔record-a〕", Content: "first"}, {ID: "facet-b", Scope: "one", Name: "Person〔record-b〕", Content: "second"}}
	made, err := service.CreateItems(context.Background(), "project", inputs)
	if err != nil || len(made) != 2 {
		t.Fatalf("first seed: %d %v", len(made), err)
	}
	if _, err := store.Update("facet-a", booklore.ItemInput{Name: "User renamed", Content: "User edited"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(booklore.ItemsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	made, err = service.CreateItems(context.Background(), "project", []booklore.ItemInput{inputs[1], inputs[0]})
	if err != nil || len(made) != 0 {
		t.Fatalf("replay: %d %v", len(made), err)
	}
	after, err := os.ReadFile(booklore.ItemsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("no-op replay rewrote user edits or names")
	}
	got, err := store.Get("facet-a")
	if err != nil || got.Name != "User renamed" || got.Content != "User edited" {
		t.Fatalf("edited identity changed: %+v %v", got, err)
	}
}

func TestCreateItemsLegacyNamesAndScopes(t *testing.T) {
	service, store := seedService(t.TempDir())
	made, err := service.CreateItems(context.Background(), "project", []booklore.ItemInput{{Name: "Legacy", Content: "first"}, {Name: " legacy ", Content: "second"}})
	if err != nil || len(made) != 1 {
		t.Fatalf("legacy duplicate: %d %v", len(made), err)
	}
	made, err = service.CreateItems(context.Background(), "project", []booklore.ItemInput{{ID: "a", Scope: "one", Name: "Local", Content: "one"}, {ID: "b", Scope: "two", Name: "Local", Content: "two"}})
	if err != nil || len(made) != 2 {
		t.Fatalf("independent scopes: %d %v", len(made), err)
	}
	if _, err = service.CreateItems(context.Background(), "project", []booklore.ItemInput{{ID: "a", Scope: "two", Name: "Other", Content: "wrong scope"}}); err == nil {
		t.Fatal("same ID must not be reused across scopes")
	}
	all, err := store.ListAll()
	if err != nil || len(all) != 3 {
		t.Fatalf("stored records: %d %v", len(all), err)
	}
}

func TestCreateItemsRejectsLateInvalidAndConflictingInputsAtomically(t *testing.T) {
	for _, inputs := range [][]booklore.ItemInput{
		{{ID: "first", Name: "First", Content: "valid"}, {ID: "last", Name: "", Content: "invalid"}},
		{{ID: "same", Name: "One", Content: "first"}, {ID: "same", Name: "Two", Content: "conflict"}},
	} {
		root := t.TempDir()
		service, store := seedService(root)
		if _, err := service.CreateItems(context.Background(), "project", inputs); err == nil {
			t.Fatal("invalid batch was accepted")
		}
		all, err := store.ListAll()
		if err != nil || len(all) != 0 {
			t.Fatalf("failed batch leaked records: %d %v", len(all), err)
		}
	}
}

func TestCreateItemsPreservesAudienceThroughBatchAndColdRead(t *testing.T) {
	root := t.TempDir()
	service, _ := seedService(root)
	var input booklore.ItemInput
	if err := json.Unmarshal([]byte(`{"id":"gm","scope":"one","name":"GM record","content":"GM only","audience":"gm"}`), &input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateItems(context.Background(), "project", []booklore.ItemInput{input}); err != nil {
		t.Fatal(err)
	}
	item, err := booklore.NewStore(root).Get("gm")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["audience"] != "gm" {
		t.Fatalf("GM audience was lost: %s", raw)
	}
}
