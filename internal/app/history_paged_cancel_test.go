package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"ta/internal/history"
	"testing"
)

func TestHistoryPagedCancellationCannotCommitHalfAQuarter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	service, projectID, _ := historyServiceFixture(t, func(req historyModelRequest) (string, int) {
		calls++
		cancel()
		return `{"action":"notes","notes":"处理中。"}`, 200
	})
	s, err := service.Create(context.Background(), projectID, history.CreateRequest{Name: "Large cancellation", CommandID: "large-cancel", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	store, _, _ := service.binding(projectID)
	orders := []history.OrderIntent{}
	for i := 0; i < 32; i++ {
		orders = append(orders, history.OrderIntent{Action: "create", Kind: "policy", Text: fmt.Sprintf("policy %d ", i) + strings.Repeat("Preserve exact existing instructions. ", 290), Targets: []string{"shaanxi"}})
	}
	r, err := store.Apply(context.Background(), s.ID, history.Command{ID: "many-instructions", Kind: "edict", ExpectedRevision: s.Revision, Edict: &history.EdictPlan{Text: "Keep all instructions", Orders: orders}})
	if err != nil {
		t.Fatal(err)
	}
	s = r.State
	_, err = service.Advance(ctx, projectID, s.ID, HistoryCommandRequest{CommandID: "cancelled-large-quarter", ExpectedRevision: s.Revision, Locale: "zh-CN"})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("Cancellation failed: %v calls=%d", err, calls)
	}
	cold, err := store.Read(context.Background(), s.ID)
	if err != nil || cold.Revision != s.Revision || cold.Turn != s.Turn || len(cold.Reports) != 0 {
		t.Fatalf("Partial quarter saved: %v", err)
	}
}
