package exploration

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestExploreCooperationRequiresObservedClueAndTrustedCapability(t *testing.T) {
	for _, realmID := range []string{RealmMountainHerbCave, RealmStrandedTideBoat, RealmShaftServiceBay} {
		t.Run(realmID, func(t *testing.T) {
			def, scope, s := createExploreTestInstance(t, realmID)
			id := realmID + ".assisted_slow"
			a, ok := FindExploreApproach(def, id)
			if !ok {
				t.Fatal("authored cooperation route is missing")
			}
			if len(a.Costs) != 0 {
				t.Fatal("onsite route must not charge the prior joint work again")
			}
			before := cloneInstanceSnapshot(s)
			view, ds := ProjectExploreView(def, scope, s, ExploreViewContext{Capabilities: []string{id}})
			if len(ds) > 0 {
				t.Fatal(ds)
			}
			raw, _ := json.Marshal(view)
			if strings.Contains(string(raw), "assisted_slow") || strings.Contains(string(raw), "coop.") {
				t.Fatal("unobserved cooperation method leaked")
			}
			ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.cooperation"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, Capabilities: []string{id}}); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatalf("capability bypassed missing clue: %v", err)
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("failed action or projection mutated snapshot")
			}
			s = exploreTestApply(t, def, scope, s, realmID+".survey", 0).Snapshot
			before = cloneInstanceSnapshot(s)
			view, ds = ProjectExploreView(def, scope, s, ExploreViewContext{})
			if len(ds) > 0 {
				t.Fatal(ds)
			}
			raw, _ = json.Marshal(view)
			for _, unknownName := range []string{"赵灵", "宋老橹", "席尔瓦"} {
				if strings.Contains(string(raw), unknownName) {
					t.Fatalf("known route disclosed an unencountered partner: %s", unknownName)
				}
			}
			found := false
			for _, option := range view.Approaches {
				if option.ID == id {
					found = true
					if option.Available || option.BlockedReason != "cooperation_required" {
						t.Fatalf("missing cooperation eligibility is unclear: %+v", option)
					}
				}
			}
			if !found {
				t.Fatal("known cooperation preparation target is hidden")
			}
			ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.cooperation"))
			for _, capabilities := range [][]string{nil, {realmID + ".slow"}, {a.CooperationProjectID}} {
				if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: 600, Capabilities: capabilities}); !errors.Is(err, ErrActionNotAllowed) || !strings.Contains(err.Error(), "cooperation_required") {
					t.Fatalf("cost-free method bypassed cooperation gate: %v", err)
				}
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("refusal changed exploration state")
			}
			input := ExploreApproachInput{ApproachID: id, StoryTimeSeconds: 600, Capabilities: []string{id}}
			result, err := ApplyExploreApproach(def, scope, s, ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			// Pure replay uses the same pre-state; journal retry deduplication belongs to Story.
			replay, err := ApplyExploreApproach(def, scope, s, ctx, input)
			if err != nil || !reflect.DeepEqual(result, replay) {
				t.Fatalf("pure replay changed: %v", err)
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("successful transition aliased its input")
			}
			if result.DurationSeconds != 1200 || len(result.Costs) != 0 || len(result.Rewards) != 0 || result.Snapshot.ExploreState.Pressure != 1 || result.Snapshot.ExploreState.Setback || result.Snapshot.ExploreState.ReturnDueSeconds != 0 {
				t.Fatalf("cooperation route altered costs, pressure or rewards: %+v", result)
			}
			if !containsSourcedID(result.Snapshot.ResolvedChallenges, realmID+".obstacle") || !ExploreConnectionAvailable(def, result.Snapshot, realmID+".entry_core", realmID+".core") || ExploreConnectionAvailable(def, result.Snapshot, realmID+".core_side", realmID+".side") {
				t.Fatal("cooperation did not open exactly the original ingress")
			}
			s = result.Snapshot
			ctx = testInstanceContext(s, scope, testInstancePermit(ActionExplore, id, "", "", "scene.repeat"))
			if _, err := ApplyExploreApproach(def, scope, s, ctx, input); !errors.Is(err, ErrActionNotAllowed) || !strings.Contains(err.Error(), "used") {
				t.Fatalf("second commit reused a consumed route: %v", err)
			}
			s = exploreTestMove(t, def, scope, s, realmID+".core")
			if s.CurrentAreaID != realmID+".core" {
				t.Fatal("original core is not reachable")
			}
		})
	}
}

func TestExploreCooperationRejectsInvalidProjectReferences(t *testing.T) {
	for _, projectID := range []string{"not a project", "coop.\nforged", strings.Repeat("x", MaxIdentifierBytes+1)} {
		def, _ := RealmDefinition(RealmMountainHerbCave)
		for i := range def.ExploreRules.Approaches {
			if def.ExploreRules.Approaches[i].ID == def.ID+".assisted_slow" {
				def.ExploreRules.Approaches[i].CooperationProjectID = projectID
			}
		}
		if ds := ValidateDungeonDefinition(def); len(ds) == 0 {
			t.Errorf("invalid cooperation project reference accepted: %q", projectID)
		}
	}
}

func TestExploreCooperationMutualExclusionAndReentry(t *testing.T) {
	for _, realmID := range []string{RealmMountainHerbCave, RealmStrandedTideBoat, RealmShaftServiceBay} {
		t.Run(realmID, func(t *testing.T) {
			def, scope, entry := createExploreTestInstance(t, realmID)
			surveyed := exploreTestApply(t, def, scope, entry, realmID+".survey", 0).Snapshot
			id := realmID + ".assisted_slow"
			assisted := exploreTestApply(t, def, scope, surveyed, id, 600).Snapshot
			for _, method := range def.ExploreRules.Approaches {
				if method.ExclusiveGroup != "ingress" || method.ID == id {
					continue
				}
				t.Run(method.ID, func(t *testing.T) {
					for _, pair := range []struct {
						state  DungeonInstanceSnapshot
						target string
					}{
						{assisted, method.ID},
						{exploreTestApply(t, def, scope, surveyed, method.ID, 600).Snapshot, id},
					} {
						before := cloneInstanceSnapshot(pair.state)
						ctx := testInstanceContext(pair.state, scope, testInstancePermit(ActionExplore, pair.target, "", "", "scene.second-ingress"))
						_, err := ApplyExploreApproach(def, scope, pair.state, ctx, ExploreApproachInput{ApproachID: pair.target, StoryTimeSeconds: 1800, Capabilities: []string{pair.target}})
						if !errors.Is(err, ErrActionNotAllowed) || !strings.Contains(err.Error(), "choice_made") || !reflect.DeepEqual(before, pair.state) {
							t.Fatalf("ingress choice could be stacked: %v", err)
						}
					}
				})
			}
			ctx := testInstanceContext(assisted, scope, testInstancePermit(ActionExit, realmID+".entry", "", "", "scene.exit"))
			exited, err := ExitDungeonInstance(def, scope, assisted, ctx)
			if err != nil {
				t.Fatal(err)
			}
			ctx = testInstanceContext(exited.Snapshot, scope, testInstancePermit(ActionReenter, realmID+".entry", "", "", "scene.reentry"))
			reentered, err := ReenterDungeonInstance(def, scope, exited.Snapshot, ctx, realmID+".entry")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(assisted.ExploreState, reentered.Snapshot.ExploreState) {
				t.Fatal("reentry refreshed the cooperation method")
			}
			view, ds := ProjectExploreView(def, scope, reentered.Snapshot, ExploreViewContext{Capabilities: []string{id}})
			if len(ds) > 0 {
				t.Fatal(ds)
			}
			for _, method := range view.Approaches {
				if method.ID == id && (method.Available || method.BlockedReason != "used") {
					t.Fatal("reentry offered consumed cooperation again")
				}
			}
			setback := exploreTestApply(t, def, scope, surveyed, realmID+".outer", 600).Snapshot
			setback = exploreTestApply(t, def, scope, setback, realmID+".force", 900).Snapshot
			ctx = testInstanceContext(setback, scope, testInstancePermit(ActionExplore, id, "", "", "scene.late-assistance"))
			if _, err := ApplyExploreApproach(def, scope, setback, ctx, ExploreApproachInput{ApproachID: id, StoryTimeSeconds: 1200, Capabilities: []string{id}}); !errors.Is(err, ErrActionNotAllowed) || !strings.Contains(err.Error(), "setback") {
				t.Fatalf("cooperation reset a prior setback: %v", err)
			}
			// The ordinary route remains independently usable without cooperation.
			solo := exploreTestApply(t, def, scope, surveyed, realmID+".slow", 600).Snapshot
			solo = exploreTestMove(t, def, scope, solo, realmID+".core")
			solo = exploreTestApply(t, def, scope, solo, realmID+".preserve", 1800).Snapshot
			if solo.ExploreState.Setback || solo.ExploreState.Pressure != 3 {
				t.Fatal("cooperation became mandatory for preservation")
			}
			// Cooperation adds room for a partial take but does not grant it itself.
			assisted = exploreTestApply(t, def, scope, assisted, realmID+".outer", 1800).Snapshot
			assisted = exploreTestMove(t, def, scope, assisted, realmID+".core")
			assisted = exploreTestApply(t, def, scope, assisted, realmID+".preserve", 2100).Snapshot
			if assisted.ExploreState.Setback || assisted.ExploreState.Pressure != 3 || len(assisted.GrantedRewards) != 1 {
				t.Fatal("cooperation lost its bounded route advantage")
			}
		})
	}
}

func TestExploreCooperationVersionBindingAndRoundTrip(t *testing.T) {
	// These are pre-ITER3 content hashes, used by existing instance bindings.
	legacyHashes := map[string]string{
		RealmGuXiuDongFu:      "f0c3e516f2cf1df55df134fb6e6e08035f4e900405a6076c4742cb3c032ff32f",
		RealmMountainHerbCave: "335e6eb1ddf4a1611003739531e5cc1cb9abfaf3ce37a6faa9c91263bb576b07",
		RealmStrandedTideBoat: "4ff730b240d31417999a92c97565b0bca0713ce06ee126fdf5ffeaf1a55628ef",
		RealmShaftServiceBay:  "bad6022b32cfaffff5eb2d875c38015b006166e3af8786fd95119818a3edc8df",
	}
	projects := map[string]string{
		RealmMountainHerbCave: "coop.herbcave.notes",
		RealmStrandedTideBoat: "coop.tideboat.current",
		RealmShaftServiceBay:  "coop.servicebay.markers",
	}
	for _, def := range RealmDefinitions() {
		// This regression binds only the four previously shipped definitions.
		// Newly authored regions have their own first version and no prior hash.
		if _, wasShipped := legacyHashes[def.ID]; !wasShipped {
			continue
		}

		t.Run(def.ID, func(t *testing.T) {
			hash, err := DungeonDefinitionContentHash(def)
			if err != nil {
				t.Fatal(err)
			}
			if def.ID == RealmGuXiuDongFu {
				if def.Version != "1.0" || def.ExploreRules != nil || hash != legacyHashes[def.ID] {
					t.Fatal("legacy cave binding or content changed")
				}
				return
			}
			if def.Version != "1.1" || hash == legacyHashes[def.ID] {
				t.Fatal("new methods silently reused the old definition version")
			}
			id := def.ID + ".assisted_slow"
			a, ok := FindExploreApproach(def, id)
			if !ok || a.CooperationProjectID != projects[def.ID] || a.Kind != ExploreApproachRoute || a.AreaID != def.ID+".entry" || a.ExclusiveGroup != "ingress" || !reflect.DeepEqual(a.RequiredFactIDs, []string{def.ID + ".clue"}) {
				t.Fatalf("cooperation did not bind the expected observed route: %+v", a)
			}
			encoded, err := json.Marshal(def)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(encoded), `"cooperation_project_id"`) != 1 {
				t.Fatal("project metadata is not an optional snake-case field")
			}
			parsed, ds, err := ParseDungeonDefinition(encoded)
			if err != nil || len(ds) != 0 {
				t.Fatalf("strict cooperation roundtrip failed: %v %+v", err, ds)
			}
			parsedHash, err := DungeonDefinitionContentHash(parsed)
			if err != nil || parsedHash != hash {
				t.Fatal("project metadata was lost during parsing")
			}
			legacy := cloneDungeonDefinition(def)
			legacy.Version = "1.0"
			legacy.ExploreRules.Approaches = nil
			for _, approach := range def.ExploreRules.Approaches {
				if approach.ID != id {
					legacy.ExploreRules.Approaches = append(legacy.ExploreRules.Approaches, cloneExploreApproach(approach))
				}
			}
			legacyHash, err := DungeonDefinitionContentHash(legacy)
			if err != nil || legacyHash != legacyHashes[def.ID] {
				t.Fatal("existing actions, costs or authored content changed beyond the new method")
			}
			scope := instanceScope()
			permit := testCreateInstancePermit(t, legacy, scope, "instance.legacy", "scene.enter")
			permit.TargetID = legacy.EntryAreaIDs[0]
			created, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: legacy, Scope: scope, InstanceID: "instance.legacy", EntryAreaID: legacy.EntryAreaIDs[0], Permit: permit, Battle: BattleNone, Blocking: BlockingNone})
			if err != nil {
				t.Fatal(err)
			}
			if ds := ValidateDungeonInstance(def, scope, created.Snapshot); !containsDiagnostic(ds, "definition_content_mismatch") {
				t.Fatal("old developer instance silently accepted new content")
			}
			// Returned metadata and nested discovery slices must remain detached.
			a.CooperationProjectID = "coop.changed"
			a.RequiredFactIDs[0] = "changed"
			a.Discoveries[0].ID = "changed"
			fresh, _ := RealmDefinition(def.ID)
			freshHash, err := DungeonDefinitionContentHash(fresh)
			if err != nil || freshHash != hash {
				t.Fatal("trusted metadata lookup aliased the registered definition")
			}
		})
	}
}
