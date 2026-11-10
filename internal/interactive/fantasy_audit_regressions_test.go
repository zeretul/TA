package interactive

import "testing"

// Regressions derived from the independent pre-fix review. These exercise
// actual committed world actions, not live model choices.
func TestFantasyAuditGenericLocalAffairUsesRegisteredPlaces(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	s, id := fantasyBoundStory(t, r, a, "渡鸦桥镇")
	defer s.Close()
	d := LocalAffairDefinition{
		ID: "fantasy-audit-wet-basket", Title: "桥头湿篮", LocationID: "sg-l01",
		Hook: "同批谷袋只有一只底部湿了。", Cause: "其中一只暂放在漏雨棚角，水从篮底浸入。",
		Structure: LocalAffairStructure{Goal: "understand", Obstacle: "conflicting_accounts", Decision: "verify_or_trust", Consequence: "corrected_belief"},
		Methods: []LocalAffairMethod{
			{ID: "basket", Kind: "observation", LocationID: "sg-l01", Label: "看看篮底", Evidence: "湿痕只在一只篮底。", Finding: "水由外侧篮底浸入。"},
			{ID: "roof", Kind: "observation", LocationID: "sg-l02", Label: "看看寄放棚角", Evidence: "棚角漏雨，地上的篮印与湿痕相合。", Finding: "寄放位置解释这只篮的湿痕。"},
		},
	}

	// Foreign world places and non-place definition IDs cannot become destinations.
	for _, foreign := range []string{"xinghai-l01", "sg-blade", "sg-l99"} {
		bad := d
		bad.Methods = append([]LocalAffairMethod(nil), d.Methods...)
		bad.Methods[1].LocationID = foreign
		localAffairReject(t, s, id, "main", &TurnWorldActionInput{Action: TurnWorldActionLocalAffair, SourceText: "看看附近", LocalAffair: &LocalAffairActionInput{Action: "encounter", ID: d.ID, Definition: &bad}}, "看看附近")
	}
	opening := localAffairCommit(t, s, id, "main", "encounter", d.ID, "", &d)
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening.ID, Title: "Check the roof"})
	if err != nil {
		t.Fatal(err)
	}
	shown := localAffairPublic(t, s, id, "main")
	if len(shown) != 1 || shown[0].Cause != "" {
		t.Fatal("encounter leaked hidden cause", shown)
	}
	localAffairCommit(t, s, id, "main", "verify", d.ID, "basket", nil)
	localAffairCommit(t, s, id, "main", "resolve", d.ID, "", nil)
	if _, err = s.ApplyJourney(id, JourneyRequest{BranchID: fork.ID, RouteID: "sg-route-l01-l02", Arrive: true}); err != nil {
		t.Fatal(err)
	}
	localAffairCommit(t, s, id, fork.ID, "verify", d.ID, "roof", nil)
	localAffairCommit(t, s, id, fork.ID, "resolve", d.ID, "", nil)
	mainClock, _, err := s.JourneyState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	otherClock, _, err := s.JourneyState(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != 600 || otherClock.Seconds != 2400 {
		t.Fatal("branches did not pay their real observation and travel intervals", mainClock, otherClock)
	}
	root := s.Root()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	for _, branch := range []string{"main", fork.ID} {
		got := localAffairPublic(t, cold, id, branch)
		if len(got) != 1 || got[0].Cause != d.Cause || got[0].Status != "resolved" || len(got[0].Findings) != 1 {
			t.Fatal("cold branch lost actual independent finding", got)
		}
	}

}

func TestFantasyAuditCryptRejectsUnestablishedKeeper(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	s, id := fantasyBoundStory(t, r, a, "倒旗墓道")
	defer s.Close()
	questApplyOn(t, s, id, "main", "sg-crypt-inquiry", QuestActionAccept, "", "audit-accept")
	before, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "sg-crypt-inquiry", Action: QuestActionChoose, ChoiceID: "sg-crypt-inquiry.stage1.c2", CommandID: "audit-absent-keeper", CommandHash: "audit-absent-keeper"})
	if e == nil {
		t.Fatal("named keeper supplied testimony without an established present source")
	}
	after, e := s.AdventureWindow(id, "main")
	if e != nil || after.ClockSeconds != before.ClockSeconds || after.Actor.Energy != before.Actor.Energy || after.Actor.Currency != before.Actor.Currency {
		t.Fatal("missing-source refusal changed clock or resources", e)
	}
}

func TestFantasyAuditSparringInstructorHasBoundEncounter(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-road")
	for _, enemy := range r.Opponents {
		if enemy.CharacterSourceKey == "sg-p08" && enemy.RequiresPresence && !enemy.Lethal {
			return
		}
	}
	t.Fatal("authored sparring instructor sg-p08 cannot enter a source-bound nonlethal encounter")
}

func TestFantasyNarratorCapabilityUsesActualRegisteredPlaces(t *testing.T) {
	cases := []struct {
		name    string
		meta    StoryMeta
		allowed bool
	}{
		{"unbound", StoryMeta{}, false},
		{"named-without-catalog", StoryMeta{PackBinding: &StoryPackBinding{PackID: "sui-guan-realms"}}, false},
		{"forged-place", StoryMeta{PackBinding: &StoryPackBinding{PackID: "sui-guan-realms"}, PackCatalog: &PackCatalogSnapshot{Places: []PackCatalogPlace{{Country: "碎冠诸境", Place: "碎冠诸境\x00not-registered\x00假地点"}}}}, false},
		{"frozen-registered-place", StoryMeta{ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, "sui-guan-realms")}}, PackBinding: &StoryPackBinding{PackID: "sui-guan-realms"}, PackCatalog: &PackCatalogSnapshot{Places: []PackCatalogPlace{{Country: "碎冠诸境", Place: "碎冠诸境\x00sg-l01\x00渡鸦桥镇"}}}}, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if LocalAffairsSupported(test.meta) != test.allowed {
				t.Fatal("narrator capability disagrees with actual settlement placement")
			}
		})
	}
	for _, id := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689"} {
		if !LocalAffairsSupported(StoryMeta{PackBinding: &StoryPackBinding{PackID: id}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, id)}}}) {
			t.Fatal("existing world lost local affairs", id)
		}
	}
}
