package interactive

import (
	"reflect"
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

func cooperationFlowSurveyExit(t *testing.T, store *Store, storyID string, d CooperationProjectDefinition) string {
	t.Helper()
	if d.PlayerItemID != "" {
		cooperationFlowSpaceJourney(t, store, storyID, true, "guard-survey-route")
	}
	entered := realmTestApply(t, store, storyID, realmExploreRequest("", d.RealmID, RealmActionEnter, "", "guard-enter"))
	realmExploreDo(t, store, storyID, entered.InstanceID, d.RealmID, RealmActionSurvey, "survey")
	realmTestApply(t, store, storyID, realmExploreRequest(entered.InstanceID, d.RealmID, RealmActionExit, "", "guard-exit"))
	if d.PlayerItemID != "" {
		cooperationFlowSpaceJourney(t, store, storyID, false, "guard-return-route")
	}
	return entered.InstanceID
}

func TestCooperationGuardsResourceFailureKeepsConsentWithoutCharge(t *testing.T) {
	for _, tc := range []struct {
		name, project string
		wire, energy  int
	}{
		{"missing_player_wire", "coop.servicebay.markers", 0, -1},
		{"low_partner_energy", "coop.tideboat.current", 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, id, d := cooperationAdventureFlowStory(t, tc.project, tc.wire, tc.energy)
			instance := cooperationFlowSurveyExit(t, store, id, d)
			before, _ := store.Snapshot(id, "main")
			req := cooperationFlowRequest(t, store, id, d.ID, instance, "guard-work", "work_if_agreed", character.CooperationAccept, true, nil)
			turn, _, err := store.AppendTurnWithState(id, req)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := store.Snapshot(id, "main")
			chars, _ := store.CharacterState(id, "main")
			if len(chars.Matters) != 1 || chars.Matters[0].Cooperation.Phase != character.CooperationAgreed || len(chars.Knowledge) != 1 || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) || !reflect.DeepEqual(before.State, after.State) {
				t.Fatalf("blocked work lost consent or charged resources/time: %#v", chars)
			}
			// Neither command replay nor cold journal replay may retry the failed work.
			again, _, err := store.AppendTurnWithState(id, req)
			if err != nil || again.ID != turn.ID {
				t.Fatalf("retry: %v", err)
			}
			store = cooperationFlowCold(t, store, id)
			cold, _ := store.Snapshot(id, "main")
			coldChars, _ := store.CharacterState(id, "main")
			if !reflect.DeepEqual(cold.State, after.State) || !reflect.DeepEqual(coldChars, chars) || SnapshotClockSeconds(cold) != SnapshotClockSeconds(after) {
				t.Fatal("cold replay changed agreed but unperformed work")
			}
			again, _, err = store.AppendTurnWithState(id, req)
			if err != nil || again.ID != turn.ID {
				t.Fatalf("cold retry: %v", err)
			}
		})
	}
}

func TestCooperationGuardsSharingRefusalAndOrdinaryPromise(t *testing.T) {
	for _, tc := range []struct {
		name, decision string
		share          bool
		knowledge      int
	}{
		{"refusal_can_receive_shared_fact", character.CooperationRefuse, true, 1},
		{"acceptance_without_sharing_is_not_work", character.CooperationAccept, false, 0},
		{"refusal_without_sharing_learns_nothing", character.CooperationRefuse, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, id := cooperationFanrenFlowStory(t)
			d, _ := cooperationProject("coop.herbcave.notes")
			instance := cooperationFlowSurveyExit(t, store, id, d)
			before, _ := store.Snapshot(id, "main")
			req := cooperationFlowRequest(t, store, id, d.ID, instance, "decision-only", "propose", tc.decision, tc.share, nil)
			if _, _, err := store.AppendTurnWithState(id, req); err != nil {
				t.Fatal(err)
			}
			after, _ := store.Snapshot(id, "main")
			chars, _ := store.CharacterState(id, "main")
			if len(chars.Matters) != 0 || len(chars.Knowledge) != tc.knowledge || len(chars.Affinities) != 0 || SnapshotClockSeconds(after) != SnapshotClockSeconds(before) || !reflect.DeepEqual(after.State, before.State) {
				t.Fatalf("reply upgraded to work or leaked knowledge: %#v", chars)
			}
			for _, k := range chars.Knowledge {
				if k.Nature != character.NatureTold || k.Audience != character.AudienceSingle {
					t.Fatal("shared clue became experience/public knowledge")
				}
			}
		})
	}
	t.Run("ordinary_promise_never_authorizes", func(t *testing.T) {
		store, id := cooperationFanrenFlowStory(t)
		d, _ := cooperationProject("coop.herbcave.notes")
		instance := cooperationFlowSurveyExit(t, store, id, d)
		if _, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "说定以后帮忙", Narrative: "她说以后愿意帮忙。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{CharacterID: cooperationFlowPartner, Kind: "promise", Status: "open", Statement: "共同核对亲自调查所得，为这一处场地准备一次行进方案", Counterparty: DefaultActorID}}}}}); err != nil {
			t.Fatal(err)
		}
		before, _ := store.Snapshot(id, "main")
		charsBefore, _ := store.CharacterState(id, "main")
		req := cooperationFlowRequest(t, store, id, d.ID, instance, "promise-is-not-consent", "work", "", true, nil)
		if _, _, err := store.AppendTurnWithState(id, req); err == nil {
			t.Fatal("ordinary promise authorized work")
		}
		after, _ := store.Snapshot(id, "main")
		charsAfter, _ := store.CharacterState(id, "main")
		if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(charsBefore, charsAfter) || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) {
			t.Fatal("rejected promise work changed committed facts")
		}
	})
}

func TestCooperationGuardsFirstEstablishedProfileInitializesOnlyMissingActor(t *testing.T) {
	for _, mode := range []string{"uninitialized", "corrupt_canonical", "partial_fields"} {
		t.Run(mode, func(t *testing.T) {
			store, id, d := cooperationAdventureFlowStoryOptions(t, "coop.tideboat.current", 0, -1, false)
			instance := cooperationFlowSurveyExit(t, store, id, d)
			if _, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "当前", Profiles: []character.Profile{{ID: cooperationFlowPartner, Name: "船工", SourceKey: d.PartnerSourceKey, Revision: 1}}}); err != nil {
				t.Fatal(err)
			}
			// The actor was not known during survey intervals, so no automatic growth
			// has initialized it. A profile is not a fabricated full-energy actor.
			before, _ := store.Snapshot(id, "main")
			if getPath(before.State, actorStateFieldPath(cooperationFlowPartner, AdventureStateField)) != nil {
				t.Fatal("fixture unexpectedly initialized NPC before work")
			}
			if mode != "uninitialized" {
				field, value := AdventureStateField, any(map[string]any{})
				if mode == "partial_fields" {
					field, value = "生命", float64(3)
				}
				if _, err := store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: []ActorStateOp{{Op: "set", ActorID: cooperationFlowPartner, FieldID: field, Value: value}}}); err != nil {
					t.Fatal(err)
				}
				before, _ = store.Snapshot(id, "main")
			}
			req := cooperationFlowRequest(t, store, id, d.ID, instance, "first-real-npc-work", "work_if_agreed", character.CooperationAccept, true, nil)
			_, _, commitErr := store.AppendTurnWithState(id, req)
			after, _ := store.Snapshot(id, "main")
			chars, _ := store.CharacterState(id, "main")
			if mode == "corrupt_canonical" {
				// Canonical {} has zero health. The common participant guard must
				// refuse the turn, not initialize or heal the known damaged actor.
				if commitErr == nil || len(chars.Matters) != 0 || len(chars.Knowledge) != 0 || !reflect.DeepEqual(after.State, before.State) || SnapshotClockSeconds(after) != SnapshotClockSeconds(before) {
					t.Fatal("corrupt zero-health actor was accepted or restored")
				}
				return
			}
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			if len(chars.Matters) != 1 {
				t.Fatalf("lost accepted matter: %#v", chars)
			}
			if mode == "uninitialized" {
				actor, err := AdventureActorFromState(after.State, cooperationFlowPartner)
				// Charge the actual frozen NPC's learned skill, not an obsolete
				// hard-coded pool from an earlier content revision.
				frozen := combatTestSystemFromStore(t, store, id).AdventureRules
				initialEnergy, cost := -1, -1
				for _, npc := range frozen.NPCs {
					if npc.CharacterSourceKey == d.PartnerSourceKey {
						initialEnergy = npc.Actor.Energy
					}
				}
				for _, skill := range frozen.Skills {
					if skill.ID == d.PartnerSkillID {
						cost = skill.Cost
					}
				}
				if initialEnergy < 0 || cost <= 0 {
					t.Fatal("fixture lacks a real frozen NPC and paid skill")
				}
				if err != nil || actor.SourceKey != d.PartnerSourceKey || actor.Energy != initialEnergy-cost || actor.Skills[d.PartnerSkillID] < 1 || chars.Matters[0].Cooperation.Phase != character.CooperationWorked || SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != 600 {
					t.Fatalf("first established actor did not settle frozen skill cost: %+v %v", actor, err)
				}
				store = cooperationFlowCold(t, store, id)
				cold, _ := store.Snapshot(id, "main")
				if !reflect.DeepEqual(after.State, cold.State) || SnapshotClockSeconds(after) != SnapshotClockSeconds(cold) {
					t.Fatal("initial actor or cost was not journaled")
				}
			} else if chars.Matters[0].Cooperation.Phase != character.CooperationAgreed || !reflect.DeepEqual(after.State, before.State) || SnapshotClockSeconds(after) != SnapshotClockSeconds(before) {
				t.Fatal("damaged existing actor was silently restored")
			}
		})
	}
}

func TestCooperationGuardsSameTurnFirstAppearanceCannotAuthorize(t *testing.T) {
	store, id, d := cooperationAdventureFlowStoryOptions(t, "coop.tideboat.current", 0, -1, false)
	instance := cooperationFlowSurveyExit(t, store, id, d)
	before, _ := store.Snapshot(id, "main")
	req := cooperationFlowRequest(t, store, id, d.ID, instance, "new-appearance-is-not-established", "work_if_agreed", character.CooperationAccept, true, nil)
	req.TurnResult.CharacterFacts.Profiles = []TurnCharacterProfileInput{{CharacterID: cooperationFlowPartner, Name: "船工", SourceKey: d.PartnerSourceKey}}
	if _, _, err := store.AppendTurnWithState(id, req); err == nil {
		t.Fatal("same-turn first appearance authorized cooperation")
	}
	after, _ := store.Snapshot(id, "main")
	chars, _ := store.CharacterState(id, "main")
	if len(chars.Profiles) != 0 || len(chars.Matters) != 0 || len(chars.Knowledge) != 0 || !reflect.DeepEqual(after.State, before.State) || SnapshotClockSeconds(after) != SnapshotClockSeconds(before) {
		t.Fatal("refused first appearance leaked a character or work")
	}
}

func TestCooperationGuardsEarlierSpecificAcceptanceNeedsNoSecondApproval(t *testing.T) {
	store, id := cooperationFanrenFlowStory(t)
	d, _ := cooperationProject("coop.herbcave.notes")
	instance := cooperationFlowSurveyExit(t, store, id, d)
	ask := cooperationFlowRequest(t, store, id, d.ID, instance, "ask-without-share", "propose", character.CooperationAccept, false, nil)
	if _, _, err := store.AppendTurnWithState(id, ask); err != nil {
		t.Fatal(err)
	}
	accepted, _ := store.CharacterState(id, "main")
	if len(accepted.Matters) != 0 || len(accepted.Knowledge) != 0 || len(accepted.Exchanges) != 1 {
		t.Fatal("asking silently shared the clue or performed work")
	}
	before, _ := store.Snapshot(id, "main")
	work := cooperationFlowRequest(t, store, id, d.ID, instance, "later-share-and-work", "work", "", true, nil)
	if _, _, err := store.AppendTurnWithState(id, work); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Snapshot(id, "main")
	state, _ := store.CharacterState(id, "main")
	if len(state.Exchanges) != 1 || len(state.Matters) != 1 || state.Matters[0].Cooperation.ConsentExchangeID != accepted.Exchanges[0].ID || state.Matters[0].Cooperation.Phase != character.CooperationWorked || len(state.Knowledge) != 1 || SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != 600 {
		t.Fatalf("earlier specific acceptance required another response: %#v", state)
	}
}

func TestCooperationGuardsNewestReacceptanceReplacesRevokedConsent(t *testing.T) {
	for _, energy := range []int{-1, 5} {
		name := "work"
		if energy == 5 {
			name = "still_no_energy"
		}
		t.Run(name, func(t *testing.T) {
			store, id, d := cooperationAdventureFlowStory(t, "coop.tideboat.current", 0, energy)
			instance := cooperationFlowSurveyExit(t, store, id, d)
			a := cooperationFlowRequest(t, store, id, d.ID, instance, "accept-A", "propose", character.CooperationAccept, true, nil)
			if _, _, err := store.AppendTurnWithState(id, a); err != nil {
				t.Fatal(err)
			}
			first, _ := store.CharacterState(id, "main")
			firstID := first.Matters[0].Cooperation.ConsentExchangeID
			b := cooperationFlowRequest(t, store, id, d.ID, instance, "refuse-B", "propose", character.CooperationRefuse, false, nil)
			if _, _, err := store.AppendTurnWithState(id, b); err != nil {
				t.Fatal(err)
			}
			blocked := cooperationFlowRequest(t, store, id, d.ID, instance, "cannot-reuse-A", "work", "", false, nil)
			if _, _, err := store.AppendTurnWithState(id, blocked); err == nil {
				t.Fatal("revoked acceptance A still authorized work")
			}
			before, _ := store.Snapshot(id, "main")
			c := cooperationFlowRequest(t, store, id, d.ID, instance, "accept-C", "work_if_agreed", character.CooperationAccept, true, nil)
			turn, _, err := store.AppendTurnWithState(id, c)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := store.Snapshot(id, "main")
			current, _ := store.CharacterState(id, "main")
			latestID := ""
			for _, e := range current.Exchanges {
				if e.TurnID == turn.ID && e.CooperationDecision != nil && e.CooperationDecision.Decision == character.CooperationAccept {
					latestID = e.ID
				}
			}
			phase, seconds := character.CooperationWorked, int64(600)
			if energy == 5 {
				phase, seconds = character.CooperationAgreed, 0
			}
			if len(current.Matters) != 1 || current.Matters[0].Cooperation.Phase != phase || latestID == "" || latestID == firstID || current.Matters[0].Cooperation.ConsentExchangeID != latestID || SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != seconds {
				t.Fatalf("new concrete C response retained obsolete A or charged blocked work: %#v", current)
			}
		})
	}
}

func TestCooperationFlowAgreedForkUsesAncestorConsentWithoutReasking(t *testing.T) {
	store, id := cooperationFanrenFlowStory(t)
	d, _ := cooperationProject("coop.herbcave.notes")
	instance := cooperationFlowSurveyExit(t, store, id, d)
	req := cooperationFlowRequest(t, store, id, d.ID, instance, "agree-before-fork", "propose", character.CooperationAccept, true, nil)
	agreed, _, err := store.AppendTurnWithState(id, req)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := store.CharacterState(id, "main")
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: agreed.ID, Title: "Work on inherited agreement"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.Snapshot(id, fork.ID)
	work := cooperationFlowRequestForBranch(t, store, id, fork.ID, d.ID, instance, "fork-work-accepted", "work", "", false, nil)
	if _, _, err := store.AppendTurnWithState(id, work); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Snapshot(id, fork.ID)
	chars, _ := store.CharacterState(id, fork.ID)
	main, _ := store.CharacterState(id, "main")
	if len(chars.Exchanges) != 1 || chars.Exchanges[0].BranchID != "main" || chars.Matters[0].BranchID != fork.ID || chars.Matters[0].Cooperation.Phase != character.CooperationWorked || chars.Matters[0].Cooperation.ConsentExchangeID != original.Matters[0].Cooperation.ConsentExchangeID || SnapshotClockSeconds(after)-SnapshotClockSeconds(before) != 600 || !reflect.DeepEqual(main, original) {
		t.Fatal("fork required new consent, rewrote original reply or charged the parent branch")
	}
}

func TestCooperationGuardsSameTurnNarratedWireAndSkillCannotAuthorize(t *testing.T) {
	for _, kind := range []string{"player_wire", "partner_skill"} {
		t.Run(kind, func(t *testing.T) {
			project := "coop.servicebay.markers"
			if kind == "partner_skill" {
				project = "coop.tideboat.current"
			}
			store, id, d := cooperationAdventureFlowStory(t, project, 0, -1)
			instance := cooperationFlowSurveyExit(t, store, id, d)
			target := DefaultActorID
			if kind == "partner_skill" {
				target = cooperationFlowPartner
			}
			snap, _ := store.Snapshot(id, "main")
			actor, err := AdventureActorFromState(snap.State, target)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "partner_skill" {
				delete(actor.Skills, d.PartnerSkillID)
				ops := adventureActorOps(actor, "explicit-no-skill-fixture")
				for i := range ops {
					ops[i].ActorID = target
				}
				if _, err := store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: ops}); err != nil {
					t.Fatal(err)
				}
				actor.Skills[d.PartnerSkillID] = 1
			} else {
				actor.Inventory[d.PlayerItemID] = 1
			}
			before, _ := store.Snapshot(id, "main")
			charsBefore, _ := store.CharacterState(id, "main")
			req := cooperationFlowRequest(t, store, id, d.ID, instance, "cannot-invent-"+kind, "work_if_agreed", character.CooperationAccept, true, nil)
			// This is the actual model-facing state_updates channel, not a custom
			// work API or an injected post-validation ActorOps list.
			req.TurnResult.StateUpdates = []interactivestate.Update{{Op: interactivestate.Replace, Path: "/" + target + "/" + AdventureStateField, Value: actor}}
			if _, _, err := store.AppendTurnWithState(id, req); err == nil {
				t.Fatal("same-turn narrative changed frozen qualification and authorized work")
			}
			after, _ := store.Snapshot(id, "main")
			charsAfter, _ := store.CharacterState(id, "main")
			if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(charsBefore, charsAfter) || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) {
				t.Fatal("rejected self-qualification leaked cost, consent or resources")
			}
		})
	}
}
