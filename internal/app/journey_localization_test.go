package app

import (
	"encoding/json"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestJourneyEnglishUsesBoundAuthoredPlacesWithoutChangingFacts(t *testing.T) {
	for _, id := range []string{"fanren-jie", "lanchuan-qunxia", "xinghai-689", "sui-guan-realms"} {
		t.Run(id, func(t *testing.T) {
			pack, err := storypack.NewLibrary(t.TempDir()).Get(id)
			if err != nil {
				t.Fatal(err)
			}
			if id == "fanren-jie" {
				gate := localizeJourneyPlaces(JourneyState{Routes: []interactive.JourneyRouteOption{{OriginLabel: "黄枫谷对外入口", DestinationLabel: "黄枫谷山门内"}}}, pack)
				if gate.Routes[0].OriginLabel != "Huangfeng Valley Entrance" || gate.Routes[0].DestinationLabel != "Inside Huangfeng Valley" {
					t.Fatal("sect approach route not translated")
				}
				original := JourneyState{Routes: []interactive.JourneyRouteOption{{OriginLabel: "公共坊市", DestinationLabel: "越国坊市外围散修人家", TotalMinutes: 30}}}
				en := localizeJourneyPlaces(original, pack)
				if en.Routes[0].OriginLabel != "Public Market" || en.Routes[0].DestinationLabel != "Cultivators' Homes outside Yue Market" || en.Routes[0].TotalMinutes != 30 {
					t.Fatal("original cultivation route not translated")
				}
			}
			count := 0
			for _, place := range pack.World {
				expected := pack.Localizations["en-US"].World[place.ID].Name
				if expected == "" {
					continue
				}
				view := JourneyState{Location: place.Name, LocationID: place.ID, ClockSeconds: 777, Routes: []interactive.JourneyRouteOption{{RouteID: "stable", OriginLabel: place.Name, DestinationLabel: place.Name, TotalMinutes: 47}}, Active: &JourneyProgressView{Origin: place.Name, Destination: place.Name, ElapsedMinutes: 7, TotalMinutes: 47, Remaining: 40}}
				before, _ := json.Marshal(view)
				en := localizeJourneyPlaces(view, pack)
				after, _ := json.Marshal(view)
				if string(before) != string(after) {
					t.Fatal("English read mutated source")
				}
				if en.Location != expected || en.Routes[0].OriginLabel != expected || en.Routes[0].DestinationLabel != expected || en.Active.Origin != expected || en.Active.Destination != expected {
					t.Fatalf("missing place translation %s", place.ID)
				}
				if en.Routes[0].RouteID != "stable" || en.ClockSeconds != 777 || en.Routes[0].TotalMinutes != 47 || en.Active.Remaining != 40 {
					t.Fatal("localization changed mechanics")
				}
				count++
			}
			if count == 0 {
				t.Fatal("no authored places checked")
			}
			custom := JourneyState{Location: "玩家写的屋后小路"}
			if localizeJourneyPlaces(custom, pack).Location != custom.Location {
				t.Fatal("custom text rewritten")
			}
		})
	}
}
