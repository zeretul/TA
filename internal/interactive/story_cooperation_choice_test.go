package interactive

import (
	"ta/internal/character"
	"ta/internal/exploration"
	"testing"
)

// This is the playable tradeoff, not just a capability flag: after taking the
// outer materials and surveying, an ordinary slow approach leaves no pressure
// margin to preserve the core. Joint preparation buys that one point of margin.
// The fixture's prior acquaintance is explicit; no model was invoked.
func TestCooperationChangesOptionalOuterAndPreserveTradeoff(t *testing.T) {
	const realm = exploration.RealmMountainHerbCave
	for _, assisted := range []bool{false, true} {
		name := "solo"
		if assisted {
			name = "cooperate"
		}
		t.Run(name, func(t *testing.T) {
			s, id := cooperationFanrenFlowStory(t)
			entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
			realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionResolve, "outer")
			realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionSurvey, "survey")
			method := "slow"
			if assisted {
				realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
				before := worldFlowClock(t, s, id).Seconds
				req := cooperationFlowRequest(t, s, id, "coop.herbcave.notes", entered.InstanceID, "prepare-plan", "work_if_agreed", character.CooperationAccept, true, nil)
				if _, _, err := s.AppendTurnWithState(id, req); err != nil {
					t.Fatal(err)
				}
				if worldFlowClock(t, s, id).Seconds-before != 600 {
					t.Fatal("joint work skipped its time")
				}
				realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionReenter, "", "reenter"))
				method = "assisted_slow"
			}
			realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionApproach, method)
			realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "walk-core")
			outcome := realmExploreDo(t, s, id, entered.InstanceID, realm, RealmActionResolve, "preserve")
			want := 4
			if assisted {
				want = 3
			}
			if outcome.Instance.ExploreState.Pressure != want || outcome.Instance.ExploreState.Setback == assisted {
				t.Fatalf("no real choice difference: %+v", outcome.Instance.ExploreState)
			}
			if realmExploreQuantity(t, s, id, "main", "fanren-jie", "material.herb.green_vine") != 1 {
				t.Fatal("partial gain lost or doubled")
			}
			if (outcome.Instance.ExploreState.ReturnDueSeconds > 0) != assisted {
				t.Fatal("preservation did not change future return opportunity")
			}
			// Both results retain a real retreat. Cooperation is optional, not a gate.
			realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "retreat")
			realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "finish"))
		})
	}
}

func TestCooperationAllActionsRespectRealIncapacitatedNPC(t *testing.T) {
	for _, action := range []string{"propose", "report", "work_if_agreed"} {
		t.Run(action, func(t *testing.T) {
			p, meta, input, facts := cooperationPlannerFixture(t, "coop.tideboat.current")
			input.Cooperation.Action = action
			actual, _ := AdventureActorFromState(p.State, "partner")
			actual.Health = 0
			// Deliberately leave the mirrored display health positive: canonical health
			// owns eligibility, so a stale presentation field cannot keep an NPC active.
			for _, op := range adventureActorOps(actual, "fixture") {
				if op.FieldID == AdventureStateField {
					op.ActorID = "partner"
					applyActorStateOp(p.State, op)
				}
			}
			if _, err := planCooperationAction(p, p.State, meta, "main", "turn", input.Cooperation, facts); err == nil {
				t.Fatal("incapacitated NPC accepted an interaction")
			}
		})
	}
}
