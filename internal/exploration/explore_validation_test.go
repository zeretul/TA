package exploration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExploreRejectsInvalidDefinitions(t *testing.T) {
	cases := map[string]func(*DungeonDefinition){
		"missing localization":      func(d *DungeonDefinition) { d.ExploreRules.PressureLabelKey = "" },
		"missing area localization": func(d *DungeonDefinition) { d.Areas[1].NameKey = "" },
		"missing fact localization": func(d *DungeonDefinition) { d.ExploreRules.Approaches[0].Discoveries[0].TextKey = "" },
		"zero limit":                func(d *DungeonDefinition) { d.ExploreRules.PressureLimit = 0 },
		"duplicate action": func(d *DungeonDefinition) {
			d.ExploreRules.Approaches = append(d.ExploreRules.Approaches, d.ExploreRules.Approaches[0])
		},
		"missing area":            func(d *DungeonDefinition) { d.ExploreRules.Approaches[0].AreaID = "missing" },
		"zero time":               func(d *DungeonDefinition) { d.ExploreRules.Approaches[0].DurationSeconds = 0 },
		"free pressure reduction": func(d *DungeonDefinition) { d.ExploreRules.Approaches[0].PressureDelta = -1 },
		"unknown fact":            func(d *DungeonDefinition) { d.ExploreRules.Approaches[1].RequiredFactIDs = []string{"unknown"} },
		"conflicting fact": func(d *DungeonDefinition) {
			d.ExploreRules.Approaches[1].ExcludedFactIDs = append([]string{}, d.ExploreRules.Approaches[1].RequiredFactIDs...)
		},
		"no retreat": func(d *DungeonDefinition) { d.ExitAreaIDs = []string{d.ID + ".side"} },
		"reversed tree": func(d *DungeonDefinition) {
			d.Connections[0].FromAreaID, d.Connections[0].ToAreaID = d.Connections[0].ToAreaID, d.Connections[0].FromAreaID
		},
		"one way trap":           func(d *DungeonDefinition) { d.Connections[0].Bidirectional = false },
		"negative cost":          func(d *DungeonDefinition) { d.ExploreRules.Approaches[2].Costs[0].Quantity = -1 },
		"invented cost kind":     func(d *DungeonDefinition) { d.ExploreRules.Approaches[2].Costs[0].Kind = "trust_client" },
		"client energy override": func(d *DungeonDefinition) { d.ExploreRules.Approaches[2].Costs[0].EnergyCost = 1 },
		"fixed source":           func(d *DungeonDefinition) { d.ExploreRules.Approaches[0].Discoveries[0].SourceRef = "fake" },
		"reward before resolve": func(d *DungeonDefinition) {
			d.ExploreRules.Approaches[0].RewardRefIDs = []string{d.ID + ".outer_reward"}
		},
		"return without delay": func(d *DungeonDefinition) { d.ExploreRules.ReturnAfterSeconds = 0 },
		"setback reduction": func(d *DungeonDefinition) {
			for i := range d.ExploreRules.Approaches {
				if d.ExploreRules.Approaches[i].RequiresSetback {
					d.ExploreRules.Approaches[i].PressureDelta = -1
				}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d, _ := RealmDefinition(RealmMountainHerbCave)
			mutate(&d)
			if ds := ValidateDungeonDefinition(d); len(ds) == 0 {
				t.Fatal("invalid exploration definition accepted")
			}
		})
	}
	d, _ := RealmDefinition(RealmMountainHerbCave)
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	parsed, ds, err := ParseDungeonDefinition(data)
	if err != nil || len(ds) > 0 || parsed.ExploreRules == nil {
		t.Fatalf("roundtrip failed: %v %+v", err, ds)
	}
	data = []byte(strings.Replace(string(data), `"pressure_limit":4`, `"pressure_limit":4,"client_success":true`, 1))
	if _, _, err := ParseDungeonDefinition(data); err == nil {
		t.Fatal("unknown exploration field accepted")
	}
}

func TestExploreRejectsForgedSnapshotProgress(t *testing.T) {
	def, scope, s := createExploreTestInstance(t, RealmMountainHerbCave)
	p := def.ID
	cases := map[string]func(*DungeonInstanceSnapshot){
		"missing state":       func(s *DungeonInstanceSnapshot) { s.ExploreState = nil },
		"free pressure reset": func(s *DungeonInstanceSnapshot) { s.ExploreState.Pressure = 1 },
		"free setback":        func(s *DungeonInstanceSnapshot) { s.ExploreState.Setback = true },
		"free return":         func(s *DungeonInstanceSnapshot) { s.ExploreState.ReturnDueSeconds = 1 },
		"forged choice": func(s *DungeonInstanceSnapshot) {
			s.ExploreState.Decisions = []ExploreDecision{{GroupID: "ingress", ApproachID: p + ".slow"}}
		},
		"unobserved clue": func(s *DungeonInstanceSnapshot) {
			a, _ := FindExploreApproach(def, p+".survey")
			d := a.Discoveries[0]
			d.SourceRef = "forged"
			s.Discoveries = append(s.Discoveries, d)
		},
		"free hidden room": func(s *DungeonInstanceSnapshot) {
			s.Discoveries = append(s.Discoveries, InstanceDiscovery{Kind: DiscoveryArea, ID: p + ".side", SourceRef: "forged"})
		},
		"free reward": func(s *DungeonInstanceSnapshot) {
			s.GrantedRewards = []InstanceReward{{ID: p + ".outer_reward", RefID: "material.herb.green_vine", Quantity: 1, SourceRef: "forged"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := cloneInstanceSnapshot(s)
			mutate(&bad)
			if ds := ValidateDungeonInstance(def, scope, bad); len(ds) == 0 {
				t.Fatal("forged progress accepted")
			}
		})
	}
}

func TestExplorePreparationIsPaidOnceAndCannotResetSetback(t *testing.T) {
	def, _ := RealmDefinition(RealmMountainHerbCave)
	p := def.ID
	def.ExploreRules.Approaches = append(def.ExploreRules.Approaches, ExploreApproach{ID: p + ".prepare", AreaID: p + ".entry", Kind: ExplorePrepare, Name: "Test preparation", NameKey: "test.prepare.name", Description: "A bounded paid preparation.", DescriptionKey: "test.prepare.description", DurationSeconds: 300, PressureDelta: -1, Costs: []ExploreResourceCost{{Kind: ExploreConsumeItem, RefID: "material.herb.green_vine", Quantity: 1}}})
	scope := instanceScope()
	permit := testCreateInstancePermit(t, def, scope, "instance.prepare", "scene.entry")
	permit.TargetID = p + ".entry"
	created, err := CreateDungeonInstance(CreateDungeonInstanceInput{Definition: def, Scope: scope, InstanceID: "instance.prepare", EntryAreaID: p + ".entry", Permit: permit, Battle: BattleNone, Blocking: BlockingNone})
	if err != nil {
		t.Fatal(err)
	}
	s := exploreTestApply(t, def, scope, created.Snapshot, p+".outer", 0).Snapshot
	r := exploreTestApply(t, def, scope, s, p+".prepare", 300)
	s = r.Snapshot
	if s.ExploreState.Pressure != 0 || len(r.Costs) != 1 || r.DurationSeconds != 300 {
		t.Fatal("paid preparation did not apply")
	}
	ctx := testInstanceContext(s, scope, testInstancePermit(ActionExplore, p+".prepare", "", "", "scene.repeat"))
	if _, err := ApplyExploreApproach(def, scope, s, ctx, ExploreApproachInput{ApproachID: p + ".prepare", Capabilities: []string{p + ".prepare"}}); err == nil {
		t.Fatal("preparation could be farmed")
	}
	def.ExploreRules.Approaches = append(def.ExploreRules.Approaches, def.ExploreRules.Approaches[len(def.ExploreRules.Approaches)-1])
	def.ExploreRules.Approaches[len(def.ExploreRules.Approaches)-1].ID = p + ".prepare_again"
	if ds := ValidateDungeonDefinition(def); len(ds) == 0 {
		t.Fatal("second preparation slot allowed")
	}
}

func TestExploreRegisteredDefinitionsArePlayableAndKeyed(t *testing.T) {
	for _, id := range []string{RealmMountainHerbCave, RealmStrandedTideBoat, RealmShaftServiceBay} {
		def, ok := RealmDefinition(id)
		if !ok {
			t.Fatal("missing definition")
		}
		validation := ValidateDungeon(def, RealmCatalog(), RealmAvailableAbilities())
		if !validation.Playable || len(validation.Diagnostics) > 0 {
			t.Fatalf("registered place is not playable: %s %+v", id, validation)
		}
		encoded, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		_, ds, err := ParseDungeonDefinition(encoded)
		if err != nil || len(ds) > 0 {
			t.Fatalf("definition failed strict roundtrip: %s %v %+v", id, err, ds)
		}
	}
}
