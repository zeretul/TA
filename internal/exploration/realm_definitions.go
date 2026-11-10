package exploration

// This file registers the first authored secret realm, 古修洞府 (an old
// cultivator's cave dwelling), as a dungeon definition.
//
// §11.1 of 自制-探索与行路 asks for a small number of authored presets rather than
// one template wearing different names, and §11.2 asks each one to fix its entry,
// its regions, its main challenge, its rewards and its exit. The definition below
// is the whole of that: the instance rules already validate it, and nothing here
// decides what the player does inside.
//
// The realm is 越国's own ground, so it hangs off the same world the journey
// routes use. Its geography is deliberately small — outer hall, herb plot,
// formation room, alchemy chamber — because §11.3's example is a relationship, not
// a six-stage obligation.

// RealmGuXiuDongFu is the identifier the instance binding carries.
const RealmGuXiuDongFu = "realm.gu_xiu_dong_fu"

// guXiuDongFuDefinition is the authored cave dwelling.
//
// Both rewards are real registered catalogue entries — 中品稀有法器 and a
// 回春丹 — rather than unnamed treasures, because §11.2 requires a reward to be
// one that exists and can be held. The pill is what a player may find and keep
// even if the main prize is never reached, which is what makes "取得部分成果即可
// 合理撤离" a real outcome rather than a consolation sentence.
var guXiuDongFuDefinition = DungeonDefinition{
	SchemaVersion: SchemaVersion,
	ID:            RealmGuXiuDongFu,
	Version:       "1.0",
	// The realm is authored content of the world pack rather than a separate
	// package: its places and its people are the pack's.
	PackageID:      "fanren-jie",
	PackageVersion: "3",
	RulesetID:      "xiuxian",
	RulesetVersion: "1.0",
	Theme:          "越国山中的一座废弃古修洞府：外殿的旧禁制、半荒的药圃、残存的阵室与真正的丹房。",
	Objectives: []Objective{
		{ID: "realm.recover.manual", Description: "找到古修留下的法器，确认它确实存在并可取走。"},
		{ID: "realm.herb", Description: "在药圃里找到还能用的丹药。"},
	},
	Areas: []Area{
		{ID: "realm.outer_hall", Name: "外殿", Description: "洞口进来的第一进石殿，梁上积着旧尘，两侧各有一道门。"},
		{ID: "realm.herb_plot", Name: "药圃", Description: "外殿东侧的半荒药圃，石架上还留着一只封了口的药瓶。"},
		{ID: "realm.formation_room", Name: "阵室", Description: "外殿西侧的阵室，地面刻着磨得发浅的阵纹。西墙那道旧禁制早已失效，只剩一圈暗痕，禁制之后的门还半开着。"},
		{ID: "realm.alchemy_chamber", Name: "丹房", Description: "阵室之后的丹房，丹炉早已冷透，案上留着一只石匣。"},
		{ID: "realm.back_passage", Name: "后道", Description: "丹房后方一条窄道，通向山体的另一侧。"},
	},
	Connections: []DungeonConnection{
		{ID: "realm.hall_to_plot", FromAreaID: "realm.outer_hall", ToAreaID: "realm.herb_plot", Bidirectional: true, PlayerVisible: true},
		{ID: "realm.hall_to_formation", FromAreaID: "realm.outer_hall", ToAreaID: "realm.formation_room", Bidirectional: true, PlayerVisible: true},
		{ID: "realm.formation_to_alchemy", FromAreaID: "realm.formation_room", ToAreaID: "realm.alchemy_chamber", Bidirectional: true, PlayerVisible: true},
		{ID: "realm.alchemy_to_passage", FromAreaID: "realm.alchemy_chamber", ToAreaID: "realm.back_passage", Bidirectional: false, PlayerVisible: true},
	},
	EntryAreaIDs: []string{"realm.outer_hall"},
	// The entrance is in the hills beyond the settlement outside the market, so
	// that is the only place the character can walk in from. §11.2 asks each
	// realm to declare its entry conditions; being somewhere else in the world
	// means the cave is not reachable from there.
	EntryLocationIDs: []string{"yue.market.outskirts"},
	ExitAreaIDs:      []string{"realm.back_passage"},
	// There is deliberately no challenge. A challenge here would need a rule that
	// decides whether this character can get past the seal, and this build has no
	// number it could honestly decide that with; inventing a threshold would let
	// the realm refuse a move for a reason the player cannot see. The seal is
	// authored as a spent fact in the room's own description instead, and the
	// realm's real choices are where to go and what to take.
	Challenges: []Challenge{},
	ContentRefs: []ContentRef{
		{ID: "realm.herb_ref", AreaID: "realm.herb_plot", Kind: "reward", ReferenceID: "pill.restore.health.lower", Quantity: 1, ObjectiveID: "realm.herb"},
		{ID: "realm.reward_ref", AreaID: "realm.alchemy_chamber", Kind: "reward", ReferenceID: "implement.middle.rare", Quantity: 1, ObjectiveID: "realm.recover.manual"},
	},
	CompletionPaths: []CompletionPath{
		// The main path: reach the alchemy chamber, take the implement, leave by the
		// back way.
		{ID: "realm.main_path", ObjectiveIDs: []string{"realm.recover.manual"}, ChallengeIDs: []string{}},
		// The partial path: the pill alone is a legitimate outcome, which §11.3
		// requires to be a real completion rather than a failed one.
		{ID: "realm.partial_path", ObjectiveIDs: []string{"realm.herb"}, ChallengeIDs: []string{}},
	},
}

// RealmDefinitions is the registered set of authored realms.
//
// It is a slice rather than a map so the order the entry offers them is stable,
// and it is unexported content behind the accessors below so a caller cannot
// mutate an authored definition through a shared map.
var realmDefinitions = []DungeonDefinition{guXiuDongFuDefinition, herbCaveDefinition, tideBoatDefinition, serviceBayDefinition, shatteredBeaconDefinition}

// RealmDefinition returns one authored realm by identifier.
//
// The result is a copy all the way down. Returning the struct by value alone would
// still share every slice's backing array, so a caller that edited an area or a
// reward in place would rewrite the authored realm for everyone — including the
// definition hash an existing instance was bound to.
func RealmDefinition(id string) (DungeonDefinition, bool) {
	for _, definition := range realmDefinitions {
		if definition.ID == id {
			return cloneDungeonDefinition(definition), true
		}
	}
	return DungeonDefinition{}, false
}

// RealmDefinitions returns the authored realms in registration order.
func RealmDefinitions() []DungeonDefinition {
	cloned := make([]DungeonDefinition, 0, len(realmDefinitions))
	for _, definition := range realmDefinitions {
		cloned = append(cloned, cloneDungeonDefinition(definition))
	}
	return cloned
}

// cloneDungeonDefinition copies every slice the definition owns.
func cloneDungeonDefinition(definition DungeonDefinition) DungeonDefinition {
	definition.ExploreRules = cloneExploreRules(definition.ExploreRules)
	definition.Objectives = append([]Objective(nil), definition.Objectives...)
	definition.Areas = append([]Area(nil), definition.Areas...)
	definition.Connections = append([]DungeonConnection(nil), definition.Connections...)
	definition.EntryAreaIDs = append([]string(nil), definition.EntryAreaIDs...)
	definition.EntryLocationIDs = append([]string(nil), definition.EntryLocationIDs...)
	definition.ExitAreaIDs = append([]string(nil), definition.ExitAreaIDs...)
	definition.Challenges = append([]Challenge(nil), definition.Challenges...)
	definition.ContentRefs = append([]ContentRef(nil), definition.ContentRefs...)
	paths := definition.CompletionPaths
	definition.CompletionPaths = make([]CompletionPath, len(paths))
	for index, path := range paths {
		path.ObjectiveIDs = append([]string(nil), path.ObjectiveIDs...)
		path.ChallengeIDs = append([]string(nil), path.ChallengeIDs...)
		path.RequiredAbilityIDs = append([]string(nil), path.RequiredAbilityIDs...)
		// New exploration definitions round-trip through the strict candidate
		// parser. Preserve its required empty arrays without changing legacy
		// realm hashes already bound by existing journals.
		if definition.ExploreRules != nil {
			if path.ObjectiveIDs == nil {
				path.ObjectiveIDs = []string{}
			}
			if path.ChallengeIDs == nil {
				path.ChallengeIDs = []string{}
			}
			if path.RequiredAbilityIDs == nil {
				path.RequiredAbilityIDs = []string{}
			}
		}
		definition.CompletionPaths[index] = path
	}
	return definition
}

// RealmCatalog is the trusted content catalogue the definition is validated
// against.
//
// The maps are left nil where the world pack does not register a separate
// catalogue: a nil map means "not checked here" to ValidateDungeon, and the
// product layer checks the same references against its own registered content.
// The abilities are the ones this build really implements, so a completion path
// that needs an unimplemented ability is reported rather than offered.
func RealmCatalog() Catalog {
	return Catalog{
		PackageVersions: map[string]string{"fanren-jie": "3", "lanchuan-qunxia": "3", "xinghai-689": "3", "sui-guan-realms": "1"},
		RulesetVersions: map[string]string{"xiuxian": "1.0", "wuxia": "1.0", "space": "1.0", "fantasy": "1.0"},
	}
}

// RealmAvailableAbilities is what the realm's completion paths may require.
//
// The old seal is handled by ordinary investigation and a real formation
// understanding, neither of which this build models as a numeric ability, so the
// main path requires none. Keeping the list explicit means a later path that does
// need an ability has to be registered here before it can validate as playable.
func RealmAvailableAbilities() []string {
	return []string{}
}
