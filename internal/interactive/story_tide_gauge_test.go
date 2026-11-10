package interactive

import (
	"errors"
	"testing"

	"ta/internal/publiquevent"
)

func TestTideGaugePreventionAndRecoveryFromSameSave(t *testing.T) {
	store, id, dir := realmExploreStory(t, "lanchuan-qunxia", "澜口城", nil, nil)
	walk := func(branch, route string, minutes int64) {
		t.Helper()
		before, _, err := store.JourneyState(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyJourney(id, JourneyRequest{BranchID: branch, RouteID: route, Arrive: true}); err != nil {
			t.Fatal(err)
		}
		after, _, err := store.JourneyState(id, branch)
		if err != nil || after.Seconds-before.Seconds != minutes*60 {
			t.Fatalf("route %s spent %d seconds, want %d: %v", route, after.Seconds-before.Seconds, minutes*60, err)
		}
	}
	event := func(branch string) PublicEventView {
		t.Helper()
		state, err := store.PublicEventStateFor(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		for _, view := range state.Events {
			if view.EventID == publiquevent.PublicEventIDTideGauge {
				return view
			}
		}
		return PublicEventView{}
	}
	join := func(branch string, action publiquevent.ActionKind) error {
		_, err := store.ApplyPublicEvent(id, PublicEventRequest{
			BranchID: branch, EventID: publiquevent.PublicEventIDTideGauge,
			Action: PublicEventActionJoin, ParticipationAction: string(action),
		})
		return err
	}
	hasRoute := func(branch, routeID string) bool {
		t.Helper()
		options, err := store.JourneyOptionsFor(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		for _, option := range options.Routes {
			if option.RouteID == routeID {
				return true
			}
		}
		return false
	}

	if event("main").EventID != "" {
		t.Fatal("the local notice leaked to another town")
	}
	walk("main", "lanchuan-route-l01-l02", 720)
	turn, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{
		BranchID: "main", User: "去看渡头木牌", Narrative: "你站到木牌前。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: turn.ID, Title: "先回澜口城"})
	if err != nil {
		t.Fatal(err)
	}
	early := event("main")
	if early.CurrentStageID != publiquevent.TideGaugeBeforeRise || len(early.Actions) != 1 ||
		early.Actions[0].Kind != string(publiquevent.ActionHelp) || len(early.Stages[1].Lines) != 0 {
		t.Fatalf("early notice offers wrong action or exposes the future: %+v", early)
	}
	if hasRoute("main", tideGaugeShortRoute) || !hasRoute("main", "lanchuan-route-l02-l03") {
		t.Fatal("unearned ferry or missing fallback")
	}
	if err := join("main", publiquevent.ActionInvestigate); !errors.Is(err, ErrPublicEventUnknownAction) {
		t.Fatalf("late action before the tide: %v", err)
	}
	if err := join("main", publiquevent.ActionHelp); err != nil {
		t.Fatal(err)
	}

	walk(fork.ID, "lanchuan-route-l02-l01", 720)
	if event(fork.ID).EventID != "" {
		t.Fatal("the notice followed the player away")
	}
	walk(fork.ID, "lanchuan-route-l01-l02", 720)
	late := event(fork.ID)
	if late.CurrentStageID != publiquevent.TideGaugeAfterRise || late.Joined || len(late.Actions) != 1 ||
		late.Actions[0].Kind != string(publiquevent.ActionInvestigate) || len(late.Stages[0].Lines) != 0 ||
		len(late.Stages[1].Lines) != 1 {
		t.Fatalf("return must expose only the notice and recovery action: %+v", late)
	}
	if err := join(fork.ID, publiquevent.ActionHelp); !errors.Is(err, ErrPublicEventUnknownAction) {
		t.Fatalf("the missed calibration was still actionable: %v", err)
	}
	if _, err := store.ApplyJourney(id, JourneyRequest{BranchID: fork.ID, RouteID: tideGaugeShortRoute, Arrive: true}); err == nil {
		t.Fatal("the hidden short route could be traveled directly")
	}
	if err := join(fork.ID, publiquevent.ActionInvestigate); err != nil {
		t.Fatal(err)
	}

	// The deadline applies to calibration. A confirmed short route remains
	// usable if the player comes back after the same tide rise.
	walk("main", "lanchuan-route-l02-l01", 720)
	walk("main", "lanchuan-route-l01-l02", 720)
	if view := event("main"); view.CurrentStageID != publiquevent.TideGaugeAfterRise || !view.Joined {
		t.Fatalf("early calibration must survive the tide rise: %+v", view)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(dir)
	defer store.Close()
	if !hasRoute("main", tideGaugeShortRoute) || hasRoute("main", tideGaugeOuterRoute) ||
		!hasRoute(fork.ID, tideGaugeOuterRoute) || hasRoute(fork.ID, tideGaugeShortRoute) ||
		!hasRoute(fork.ID, "lanchuan-route-l02-l03") {
		t.Fatal("cold reopen did not preserve branch-specific ferry options and the fallback")
	}
	walk("main", tideGaugeShortRoute, 600)
	walk(fork.ID, tideGaugeOuterRoute, 660)
}
