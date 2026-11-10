package quest_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"ta/internal/interactive"
	"ta/internal/quest"
	"ta/internal/storypack"
)

func TestMembershipTasksMatchAuthoredPeopleAndReachablePlaces(t *testing.T) {
	cases := []struct{ id, pack, version, giver, faction, home, origin, destination string }{
		{"quest.huangfeng_member_market_visit", "fanren-jie", "30", "leader_zhong_lingdao", "yue.huangfeng", "yue.huangfeng.sect", "yue.market.outskirts", "yue.market"},
		{"quest.canglang_member_ferry_visit", "lanchuan-qunxia", "4", "lanchuan-p11", "lanchuan-o17", "lanchuan-l07", "lanchuan-l07", "lanchuan-l06"},
		{"quest.blackstar_member_office_visit", "xinghai-689", "4", "xinghai-p01", "xinghai-o11", "xinghai-l03", "xinghai-l03", "xinghai-l04"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			d, ok := quest.DefinitionByID(tc.id)
			if !ok {
				t.Fatal("membership task not registered")
			}
			if diagnostics := quest.ValidateDefinition(d); len(diagnostics) > 0 {
				t.Fatalf("invalid definition: %+v", diagnostics)
			}
			if d.PackageID != tc.pack || d.PackageVersion != tc.version || d.Giver != tc.giver || d.RequiresFactionMembership != tc.faction || !d.FreshFactsOnly || len(d.PublicLocationIDs) != 0 || len(d.Choices) != 0 || len(d.Rewards) != 0 {
				t.Fatalf("unexpected membership contract: %+v", d)
			}
			dataRoot := t.TempDir()
			pack, err := storypack.NewLibrary(dataRoot).Get(tc.pack)
			if err != nil {
				t.Fatal(err)
			}
			giver, ok := pack.CatalogBySourceKey()[d.Giver]
			if !ok || giver.FactionID != tc.faction || giver.FactionName == "" || giver.PlaceCandidate != tc.home {
				t.Fatalf("giver is not the authored faction contact: %+v", giver)
			}
			formal := false
			for _, admission := range giver.MembershipAdmission {
				formal = formal || admission == "formal"
			}
			if !formal {
				t.Fatal("giver lacks authored formal admission authority")
			}
			if len(d.Objectives) != 1 || !reflect.DeepEqual(d.Objectives[0].Evidence, []quest.Evidence{{Kind: quest.EvidenceJourneyArrival, RefID: tc.destination}}) {
				t.Fatalf("task must require the actual arrival: %+v", d.Objectives)
			}
			if interactive.JourneyPlaceLabel(tc.destination) == tc.destination {
				t.Fatal("destination has no registered journey label")
			}
			module, err := interactive.NewActorStateLibrary(dataRoot).Get(pack.Story.ModuleRefs.ActorStateID)
			if err != nil {
				t.Fatal(err)
			}
			for i := range module.ActorState.InitialActors {
				actor := &module.ActorState.InitialActors[i]
				if actor.ID == interactive.DefaultStoryContextActorID {
					if actor.State == nil {
						actor.State = map[string]any{}
					}
					actor.State["当前详细地点"] = interactive.JourneyPlaceLabel(tc.origin)
				}
			}
			store := interactive.NewStore(t.TempDir())
			t.Cleanup(func() { _ = store.Close() })
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Membership route", ActorState: &module.ActorState})
			if err != nil {
				t.Fatal(err)
			}
			options, err := store.JourneyOptionsFor(story.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			connected := false
			for _, route := range options.Routes {
				connected = connected || route.OriginLocationID == tc.origin && route.DestinationID == tc.destination && route.TotalMinutes > 0
			}
			if !connected {
				t.Fatalf("no playable route from %s to %s: %+v", tc.origin, tc.destination, options)
			}
			if len(d.SatisfiedBy(quest.Fact{Kind: quest.EvidenceJourneyArrival, RefID: tc.destination})) != 1 || len(d.SatisfiedBy(quest.Fact{Kind: quest.EvidenceChoice, RefID: tc.destination})) != 0 || len(d.SatisfiedBy(quest.Fact{Kind: quest.EvidenceJourneyArrival, RefID: tc.origin})) != 0 {
				t.Fatal("task accepts non-arrival or wrong-place evidence")
			}
			english := quest.LocalizedDefinition(d, "en-US")
			if english.Title == d.Title || english.Brief == d.Brief || english.Objectives[0].Description == d.Objectives[0].Description {
				t.Fatal("missing English membership copy")
			}
			text := english.Title + english.Brief + english.Objectives[0].Description
			if strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
				t.Fatal("Chinese leaked into English copy")
			}
			english.Title = d.Title
			english.Brief = d.Brief
			english.Objectives[0].Description = d.Objectives[0].Description
			if !reflect.DeepEqual(english, d) {
				t.Fatal("localization changed membership rules")
			}
		})
	}
}
