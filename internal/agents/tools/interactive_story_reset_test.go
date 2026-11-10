package tools

import (
	"context"
	"testing"

	"ta/internal/interactive"
)

// A refused world action makes the retained prose describe a success the rules
// denied. The submit tool is the only place that sees the receipt, so it is the
// only place that can discard that prose before the run locks it as the turn's
// narrative. Without the reset the protocol keeps the refused text as the first
// candidate and the turn can never finish.
func TestRefusedWorldActionDiscardsTheRetainedNarrative(t *testing.T) {
	resets := 0
	definitions, err := NewInteractiveTurn(InteractiveContext{
		SubmitTurnResult: func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			return interactive.TurnSubmissionReceipt{
				ModuleStatus:             interactive.TurnSubmissionModuleStatus{World: interactive.TurnSubmissionModuleRejected},
				NarrativeRewriteRequired: true,
				NarrativeRewriteReason:   "journey does not start here",
			}, nil
		},
		ResetNarrativeCandidate: func(context.Context) { resets++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := definitions[0].Tool.Run(context.Background(), `{"world":{"action":"journey","route_id":"route_036:reverse","arrive":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Fatalf("the receipt itself must reach the model: %+v", result)
	}
	if resets != 1 {
		t.Fatalf("a refused world action must discard the retained prose exactly once: %d", resets)
	}
}

// The common accepted turn must leave the retained prose alone: it is the
// narrative the turn will publish.
func TestAcceptedTurnKeepsTheRetainedNarrative(t *testing.T) {
	resets := 0
	definitions, err := NewInteractiveTurn(InteractiveContext{
		SubmitTurnResult: func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			return interactive.TurnSubmissionReceipt{Ready: true}, nil
		},
		ResetNarrativeCandidate: func(context.Context) { resets++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definitions[0].Tool.Run(context.Background(), `{"world":{"action":"none"}}`); err != nil {
		t.Fatal(err)
	}
	if resets != 0 {
		t.Fatalf("an accepted turn must not discard its prose: %d", resets)
	}
}
