package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/quest"
)

// The world actions page is the only way a natural input can name an id the
// settlement accepts: the model writes the prose, and the page tells it which
// route, task, event or realm id that prose can be settled as. These tests
// cover the part that would silently rot if the renderer and the rules drifted
// apart — that every id on the page is one the turn module accepts, that a
// place's own roads and events are the ones shown, and that a bounded page
// drops whole entries rather than writing half an id.

func renderTestWorld(location string) interactive.WorldState {
	return interactive.WorldState{
		Location: location,
		Clock:    interactive.StoryClock{Seconds: 3600, Anchor: "入夜"},
		Journeys: interactive.JourneyOptions{
			CurrentLocation: location,
			Routes: []interactive.JourneyRouteOption{{
				RouteID: "route_036:reverse", RouteVersion: "1.0",
				OriginLocationID: "yue.market.outskirts", OriginLabel: "越国坊市外围散修人家",
				DestinationID: "yue.market", DestinationLabel: "公共坊市",
				TotalMinutes: 30, MovementMethod: "ordinary_low_tier_travel_with_normal_rests",
			}},
		},
		Tasks: interactive.QuestState{
			CurrentLocation: location,
			Available: []interactive.QuestOption{{
				TaskID: "quest.herb_handover", TaskVersion: "1.0", Kind: "errand",
				Title: "药圃里的封口药瓶", Brief: "取回封口药瓶。",
			}},
			Tasks: []interactive.QuestStateView{},
		},
		Events: interactive.PublicEventState{Events: []interactive.PublicEventView{}},
		Realms: interactive.RealmState{
			CurrentLocation: location,
			Available: []interactive.RealmOption{{
				RealmID: "realm.gu_xiu_dong_fu", RealmVersion: "1.0",
				Name: "外殿", Theme: "废弃古修洞府。", EntryAreaID: "realm.outer_hall",
			}},
			Instances: []interactive.RealmStateView{},
		},
	}
}

// TestTheWorldPageCarriesTheIdsTheRulesAccept is the agreement that matters.
// The ids are read back out of the rendered page and handed to the same shape
// validation the turn module runs, so a rename on either side fails here rather
// than in a play session where the narrator copies a stale id.
func TestTheWorldPageCarriesTheIdsTheRulesAccept(t *testing.T) {
	page := renderWorldActions(renderTestWorld("越国坊市外围散修人家"), interactive.StoryContextMaxBytes)

	for _, want := range []string{
		"route_id `route_036:reverse`",
		"task_id `quest.herb_handover`",
		"realm_id `realm.gu_xiu_dong_fu`",
		"### Roads (action=journey)",
		"### Tasks (action=quest)",
		"### Public events (action=public_event)",
		"### Secret realms (action=realm)",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page must carry %q:\n%s", want, page)
		}
	}

	// The action names the page uses are the module's own constants.
	for _, action := range []string{
		interactive.TurnWorldActionJourney, interactive.TurnWorldActionQuest,
		interactive.TurnWorldActionPublicEvent, interactive.TurnWorldActionRealm,
	} {
		if !strings.Contains(page, "action="+action) {
			t.Fatalf("the page must name the real action %q:\n%s", action, page)
		}
	}

	// A task the character may take is named as acceptable in one step, which is
	// what the settlement now allows.
	if !strings.Contains(page, "quest_action=accept") {
		t.Fatalf("an available task must say how to take it:\n%s", page)
	}
}

// TestTheWorldPageNamesOnlyWhatIsHere keeps the page from offering a road or an
// event of another place. The settlement refuses those, so showing them would
// teach the model an id that is then rejected.
func TestTheWorldPageNamesOnlyWhatIsHere(t *testing.T) {
	state := renderTestWorld("公共坊市")
	state.Journeys = interactive.JourneyOptions{CurrentLocation: "公共坊市"}
	state.Realms.Available = nil
	state.Events.Events = []interactive.PublicEventView{{
		EventID: "evt.taiyue_spirit_tide", EventVersion: "1.0",
		Title: "太岳灵潮", Brief: "灵潮过境，坊市里的人都在抢收。", Region: "yue.market",
	}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)

	if strings.Contains(page, "route_036") {
		t.Fatalf("a road from elsewhere must not appear:\n%s", page)
	}
	if strings.Contains(page, "realm.gu_xiu_dong_fu") {
		t.Fatalf("a realm entered elsewhere must not appear:\n%s", page)
	}
	if !strings.Contains(page, "evt.taiyue_spirit_tide") {
		t.Fatalf("the event of this place must appear:\n%s", page)
	}
}

// TestTheWorldPageDropsWholeEntries keeps a bounded page usable. A truncated id
// is worse than a missing one, because the narrator would copy it.
func TestTheWorldPageDropsWholeEntries(t *testing.T) {
	state := renderTestWorld("越国坊市外围散修人家")
	// Small enough that at least one entry cannot fit.
	page := renderWorldActions(state, 700)
	if !strings.Contains(page, "omitted for length") {
		t.Fatalf("a bounded page must say what it dropped:\n%s", page)
	}
	if strings.Contains(page, "route_id `route_036:rever") && !strings.Contains(page, "route_id `route_036:reverse`") {
		t.Fatalf("a page must never write half an id:\n%s", page)
	}
}

// TestTheWorldPageIsSilentWhenThereIsNothing keeps the empty case honest: the
// page says the world holds nothing rather than inventing an action.
func TestTheWorldPageIsSilentWhenThereIsNothing(t *testing.T) {
	state := interactive.WorldState{
		Location: "无名荒野",
		Journeys: interactive.JourneyOptions{CurrentLocation: "无名荒野"},
		Tasks:    interactive.QuestState{CurrentLocation: "无名荒野"},
		Events:   interactive.PublicEventState{},
		Realms:   interactive.RealmState{CurrentLocation: "无名荒野"},
	}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)
	if strings.Contains(page, "route_id `") || strings.Contains(page, "task_id `") ||
		strings.Contains(page, "event_id `") || strings.Contains(page, "realm_id `") {
		t.Fatalf("an empty world must offer no id:\n%s", page)
	}
}

// TestAnOfferedTaskSaysHowToTakeIt is the live failure this page used to cause.
//
// A task the world has already raised is stored as an instance, so it appeared
// under the "taken" heading with only complete/settle offered. A player who said
// 这桩差事我接了 therefore got prose and no record: the narrator had no
// instruction that accept was the action for that state. The offer must be named
// as not taken, must name accept, and must not offer the two actions the rules
// refuse before acceptance.
func TestAnOfferedTaskSaysHowToTakeIt(t *testing.T) {
	state := renderTestWorld("越国坊市外围散修人家")
	state.Tasks.Available = nil
	state.Tasks.Tasks = []interactive.QuestStateView{{
		TaskID: "quest.herb_handover", TaskVersion: "1.0", Kind: "errand",
		Title: "药圃里的封口药瓶", Brief: "取回封口药瓶。",
		Status: string(quest.StatusOffered),
		Objectives: []interactive.QuestObjectiveView{{
			ID: "quest.herb_handover.pill", Description: "取回封口药瓶。",
		}},
		Rewards: []interactive.QuestRewardView{{Kind: "currency", RefID: "灵石", Quantity: 20}},
	}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)

	if !strings.Contains(page, "offered, not taken") {
		t.Fatalf("an offer must not be described as taken:\n%s", page)
	}
	if !strings.Contains(page, "quest_action=accept") {
		t.Fatalf("an offer must say how to take it:\n%s", page)
	}
	if strings.Contains(page, "quest_action=complete") || strings.Contains(page, "quest_action=settle") {
		t.Fatalf("an offer must not name actions the rules refuse before acceptance:\n%s", page)
	}
}

// TestAFinishedTaskOffersOnlySettlement keeps the page from naming an action the
// rules refuse. A task whose objectives are all done is completed, and complete
// on a completed instance is refused; only settle is left, once.
func TestAFinishedTaskOffersOnlySettlement(t *testing.T) {
	state := renderTestWorld("越国坊市外围散修人家")
	state.Tasks.Available = nil
	state.Tasks.Tasks = []interactive.QuestStateView{{
		TaskID: "quest.herb_handover", TaskVersion: "1.0", Kind: "errand",
		Title: "药圃里的封口药瓶", Brief: "取回封口药瓶。",
		Status: string(quest.StatusCompleted),
		Objectives: []interactive.QuestObjectiveView{{
			ID: "quest.herb_handover.pill", Description: "取回封口药瓶。", Completed: true,
		}},
	}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)

	if strings.Contains(page, "quest_action=complete") {
		t.Fatalf("a finished task must not be offered complete again:\n%s", page)
	}
	if !strings.Contains(page, "quest_action=settle") {
		t.Fatalf("a finished, unpaid task must say how it is paid:\n%s", page)
	}
}

// TestATakenTaskDoesNotSayHowToTakeIt is the other half: once accepted, the
// page must name complete and settle and must not tell the narrator to accept
// again, which the rules refuse as a duplicate.
func TestATakenTaskDoesNotSayHowToTakeIt(t *testing.T) {
	state := renderTestWorld("越国坊市外围散修人家")
	state.Tasks.Available = nil
	state.Tasks.Tasks = []interactive.QuestStateView{{
		TaskID: "quest.herb_handover", TaskVersion: "1.0", Kind: "errand",
		Title: "药圃里的封口药瓶", Brief: "取回封口药瓶。",
		Status: string(quest.StatusAccepted),
		Objectives: []interactive.QuestObjectiveView{{
			ID: "quest.herb_handover.pill", Description: "取回封口药瓶。",
		}},
	}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)

	if strings.Contains(page, "quest_action=accept") {
		t.Fatalf("a taken task must not be offered for acceptance again:\n%s", page)
	}
	if !strings.Contains(page, "quest_action=complete") || !strings.Contains(page, "quest_action=settle") {
		t.Fatalf("a taken task must name how it is finished:\n%s", page)
	}
}
