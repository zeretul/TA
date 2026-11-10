package exploration

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestExploreAllPlacesKeepRealDistinctCostsAndOrdinaryPaths(t *testing.T) {
	tests := []struct {
		id, special string
		costs       []ExploreResourceCost
		delay       int64
	}{
		{RealmMountainHerbCave, "ward", []ExploreResourceCost{{Kind: ExploreConsumeItem, RefID: "talisman.ward.lower", Quantity: 1}}, 3600},
		{RealmStrandedTideBoat, "rope", []ExploreResourceCost{{Kind: ExploreRequireItem, RefID: "lanchuan-i28", Quantity: 1}}, 21600},
		{RealmStrandedTideBoat, "boatcraft", []ExploreResourceCost{{Kind: ExploreUseSkill, RefID: "lanchuan-s34", Quantity: 1}}, 21600},
		{RealmShaftServiceBay, "repair", []ExploreResourceCost{{Kind: ExploreUseSkill, RefID: "xinghai-s02", Quantity: 1}, {Kind: ExploreConsumeItem, RefID: "xinghai-i34", Quantity: 1}}, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.id+"/"+tt.special, func(t *testing.T) {
			def, scope, s := createExploreTestInstance(t, tt.id)
			id := def.ID + "." + tt.special
			v, ds := ProjectExploreView(def, scope, s, ExploreViewContext{})
			if len(ds) > 0 {
				t.Fatal(ds)
			}
			found := false
			for _, a := range v.Approaches {
				if a.ID == id {
					found = true
					if a.Available || a.BlockedReason != "capability" || !reflect.DeepEqual(a.Costs, tt.costs) {
						t.Fatalf("missing preparation goal: %+v", a)
					}
				}
			}
			if !found {
				t.Fatal("known specialist route was hidden")
			}
			before := cloneInstanceSnapshot(s)
			ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.missing"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id}); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatalf("missing capability accepted: %v", err)
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("failed eligibility changed state")
			}
			r := exploreTestApply(t, def, scope, s, id, 0)
			if !reflect.DeepEqual(r.Costs, tt.costs) || r.DurationSeconds != 600 || r.Snapshot.ExploreState.Pressure != 0 {
				t.Fatalf("special route lost its actual cost: %+v", r)
			}
			s = r.Snapshot
			// A different ingress cannot be selected after one method was consumed.
			forceID := def.ID + ".force"
			ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, forceID, "", "", "scene.force"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: forceID, Capabilities: []string{forceID}}); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatal("second ingress accepted")
			}
			s = exploreTestApply(t, def, scope, s, def.ID+".outer", 600).Snapshot
			s = exploreTestMove(t, def, scope, s, def.ID+".core")
			s = exploreTestApply(t, def, scope, s, def.ID+".preserve", 900).Snapshot
			if s.ExploreState.Pressure != 2 || s.ExploreState.ReturnDueSeconds != 2100+tt.delay {
				t.Fatalf("prepared route did not leave room for partial gain: %+v", s.ExploreState)
			}
		})
	}
	for _, id := range []string{RealmMountainHerbCave, RealmStrandedTideBoat, RealmShaftServiceBay} {
		t.Run(id+"/ordinary", func(t *testing.T) {
			def, scope, s := createExploreTestInstance(t, id)
			s = exploreTestApply(t, def, scope, s, id+".survey", 0).Snapshot
			if s.ExploreState.ReturnDueSeconds != 0 {
				t.Fatal("survey started a return without a preservation choice")
			}
			s = exploreTestApply(t, def, scope, s, id+".slow", 600).Snapshot
			s = exploreTestMove(t, def, scope, s, id+".core")
			s = exploreTestApply(t, def, scope, s, id+".preserve", 1800).Snapshot
			if s.ExploreState.Setback || s.ExploreState.Pressure != 3 {
				t.Fatal("ordinary preparation-free route failed")
			}
		})
	}
}

func TestExploreRejectsBypassesAndLeaks(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmShaftServiceBay)
	p := def.ID
	checkUnchanged := func() {
		t.Helper()
		if s.Revision != 1 || s.ExploreState.Pressure != 0 || len(s.Discoveries) != 1 {
			t.Fatal("refused action partially changed snapshot")
		}
	}
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionMove, p+".entry_core", s.CurrentAreaID, p+".core", "scene.guess"))
	if _, err := MoveDungeonInstance(def, scope, s, ctx, MoveDungeonInstanceInput{ConnectionID: p + ".entry_core", DestinationAreaID: p + ".core"}); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatal("guessed connection bypassed obstacle")
	}
	checkUnchanged()
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionDiscover, p+".outer_reward", "", "", "scene.guess"))
	if _, _, err := GrantInstanceReward(def, scope, s, ctx, RealmRewardRequest{ContentRefID: p + ".outer_reward", SourceRef: "scene.guess"}); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatal("legacy grant bypassed exploration cost")
	}
	checkUnchanged()
	if _, err := RegisterInstanceDiscovery(def, scope, s, ctx, InstanceDiscovery{Kind: DiscoveryFact, ID: p + ".clue", Text: "forged", SourceRef: "scene.guess", PubliclyDisplayable: true}); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatal("legacy discovery bypassed survey")
	}
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionProgress, p+".obstacle", "", "", "scene.guess"))
	if _, err := RecordInstanceProgress(def, scope, s, ctx, InstanceProgress{Kind: ProgressChallenge, ID: p + ".obstacle", SourceRef: "scene.guess", Outcome: OutcomeConfirmed}); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatal("legacy progress bypassed obstacle")
	}
	v, ds := ProjectExploreView(def, scope, s, ExploreViewContext{})
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	raw, _ := json.Marshal(v)
	for _, secret := range []string{"xinghai-i37", p + ".side", p + ".cache_reward", "reward_ref_ids", "discoveries"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("hidden outcome leaked: %s", secret)
		}
	}
	id := p + ".survey"
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.survey"))
	ctx.Battle = BattleActive
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, Capabilities: []string{id}}); !errors.Is(err, ErrActionBlocked) {
		t.Fatal("battle did not block exploration")
	}
	ctx.Battle = BattleNone
	ctx.ExpectedRevision++
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, Capabilities: []string{id}}); !errors.Is(err, ErrStaleInstanceRevision) {
		t.Fatal("stale approach accepted")
	}
	ctx.ExpectedRevision = s.Revision
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: math.MaxInt64, Capabilities: []string{id}}); !errors.Is(err, ErrTimeOverflow) {
		t.Fatal("overflow accepted")
	}
	checkUnchanged()
}

func TestExploreReturnRequiresRealMoveAndRemainsOnceOnlyAcrossReentry(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmMountainHerbCave)
	p := def.ID
	s = exploreTestApply(t, def, scope, s, p+".ward", 0).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestApply(t, def, scope, s, p+".preserve", 600).Snapshot
	due := s.ExploreState.ReturnDueSeconds
	id := p + ".return"
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.early"))
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: due - 1, Capabilities: []string{id}}); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatal("early return accepted")
	}
	s = exploreTestApply(t, def, scope, s, id, due).Snapshot
	v, ds := ProjectExploreView(def, scope, s, ExploreViewContext{StoryTimeSeconds: due + 600})
	if len(ds) > 0 || !v.ReturnOpened || v.ReturnAvailable {
		t.Fatalf("opened return was advertised as a fresh opportunity: %+v %+v", v, ds)
	}
	for _, d := range s.Discoveries {
		if d.Kind == DiscoveryArea && d.ID == p+".side" {
			t.Fatal("opening a door falsely counted as visiting its room")
		}
	}
	s = exploreTestMove(t, def, scope, s, p+".side")
	s = exploreTestApply(t, def, scope, s, p+".cache", due+600).Snapshot
	s = exploreTestMove(t, def, scope, s, p+".core")
	s = exploreTestMove(t, def, scope, s, p+".entry")
	before := cloneExploreState(s.ExploreState)
	rewards := append([]InstanceReward{}, s.GrantedRewards...)
	ctx = testInstanceContext(s, scope, testInstancePermit(ActionExit, p+".entry", "", "", "scene.exit"))
	out, err := ExitDungeonInstance(def, scope, s, ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx = testInstanceContext(out.Snapshot, scope, testInstancePermit(ActionReenter, p+".entry", "", "", "scene.reenter"))
	back, err := ReenterDungeonInstance(def, scope, out.Snapshot, ctx, p+".entry")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, back.Snapshot.ExploreState) || !reflect.DeepEqual(rewards, back.Snapshot.GrantedRewards) {
		t.Fatal("reentry refreshed pressure, choice or loot")
	}
}

func TestExploreCoreAndResidueAreAlternatives(t *testing.T) {
	for _, realm := range []string{RealmMountainHerbCave, RealmStrandedTideBoat, RealmShaftServiceBay} {
		t.Run(realm, func(t *testing.T) {
			def, scope, s := createExploreTestInstance(t, realm)
			p := def.ID
			s = exploreTestApply(t, def, scope, s, p+".force", 0).Snapshot
			s = exploreTestMove(t, def, scope, s, p+".core")
			id := p + ".preserve"
			ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.preserve"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, Capabilities: []string{id}}); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatal("damaged structure was preserved")
			}
			s = exploreTestApply(t, def, scope, s, p+".strip", 300).Snapshot
			s = exploreTestMove(t, def, scope, s, p+".entry")
			s = exploreTestApply(t, def, scope, s, p+".survey", 600).Snapshot
			id = p + ".residue"
			ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.residue"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: 1200, Capabilities: []string{id}}); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatal("deliberate setback granted a bonus after the complete core")
			}
			if len(s.GrantedRewards) != 1 || s.GrantedRewards[0].Quantity != 2 {
				t.Fatal("core stock changed")
			}
		})
	}
}

func TestExploreDefinitionsSnapshotsAndViewsAreDetached(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmMountainHerbCave)
	canonical, _ := RealmDefinition(def.ID)
	a, _ := FindExploreApproach(def, def.ID+".ward")
	a.Costs[0].Quantity = 900
	def.ExploreRules.Approaches[0].Discoveries[0].Text = "mutated"
	all := RealmDefinitions()
	for i := range all {
		if all[i].ID == def.ID {
			all[i].ExploreRules.Approaches[1].RequiredFactIDs[0] = "changed"
		}
	}
	again, _ := RealmDefinition(def.ID)
	if !reflect.DeepEqual(canonical, again) {
		t.Fatal("definition copy aliases registered content")
	}
	result := exploreTestApply(t, again, scope, s, def.ID+".ward", 0)
	result.Snapshot.ExploreState.UsedApproachIDs[0] = "mutated"
	result.Snapshot.ExploreState.Decisions[0].ApproachID = "mutated"
	result.Costs[0].RefID = "mutated"
	if len(s.ExploreState.UsedApproachIDs) != 0 || len(s.ExploreState.Decisions) != 0 {
		t.Fatal("result aliases prior state")
	}
	again, _ = RealmDefinition(def.ID)
	view, ds := ProjectExploreView(again, scope, s, ExploreViewContext{})
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	for i := range view.Approaches {
		if len(view.Approaches[i].Costs) > 0 {
			view.Approaches[i].Costs[0].Quantity = 0
		}
	}
	if !reflect.DeepEqual(canonical, again) {
		t.Fatal("view cost aliases definition")
	}
}
