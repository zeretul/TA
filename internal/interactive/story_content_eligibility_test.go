package interactive

import (
	"errors"
	"strconv"
	"testing"

	"ta/internal/exploration"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// These tests are the five content-boundary gaps the independent review found in
// 独立复核结论.md. Each one is a place where a piece of authored content could be
// reached from a story it does not belong to, or where the scene's prose was
// read as a location.
//
// Every case here fails against the code the review examined: the journey entry
// offered a road from inside a cave, the realm entry accepted any scene, the
// task entry ignored which world and which person the task came from, and the
// public event matched its region by substring.

// eligibilityTestStory builds a story at a registered place with a caller-chosen
// world binding and frozen catalog.
func eligibilityTestStory(t *testing.T, location string, binding *StoryPackBinding, catalog *PackCatalogSnapshot, contents ...*WorldContent) (*Store, string) {
	t.Helper()
	system := journeyTestSystem(location)
	if binding != nil {
		switch binding.PackID {
		case "fanren-jie", "lanchuan-qunxia", "xinghai-689", "sui-guan-realms":
			system.WorldContent = authoredWorldContent(t, binding.PackID)
		}
	}
	if len(contents) > 0 {
		system.WorldContent = contents[0]
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "资格", ActorState: &system,
		PackBinding: binding, PackCatalog: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "看看四周", Narrative: "主角站定，打量四周。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// fanrenBinding is the real pack revision the authored content was written
// against, as the story stores it.
func fanrenBinding(version int) *StoryPackBinding {
	return &StoryPackBinding{
		PackID: "fanren-jie", PackName: "凡人人界",
		ContentVersion: version,
		ContentHash:    "6e2d86de2f0f9cd22a7d28a3",
		Scope:          PackScopeForRevision("fanren-jie", "6e2d86de2f0f9cd22a7d28a3"),
	}
}

// TestAJourneyIsNotOfferedFromInsideARealm is §11.2's separation of a realm's
// own rooms from the world's roads.
//
// The review found the journey entry still offering the market route while the
// character stood inside the cave. The entry and the commit ask one question, so
// the route cannot be shown and then refused.
func TestAJourneyIsNotOfferedFromInsideARealm(t *testing.T) {
	store, storyID := realmTestStory(t)
	realmTestEnter(t, store, storyID)

	options, err := store.JourneyOptionsFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Routes) != 0 {
		t.Fatalf("no road leads out of a realm the character is standing in: %#v", options.Routes)
	}
	_, err = store.ApplyJourney(storyID, JourneyRequest{
		BranchID: "main", RouteID: "route_036:reverse", Arrive: true,
		CommandID: "cmd-walk", CommandHash: "h-walk",
	})
	if !errors.Is(err, ErrJourneyInsideRealm) {
		t.Fatalf("walking a world road from inside a realm must be refused: %v", err)
	}
}

// TestARealmCannotBeEnteredFromSomewhereElse is §11.2's entry condition: the
// cave has one entrance, in the hills outside the market.
func TestARealmCannotBeEnteredFromSomewhereElse(t *testing.T) {
	store, storyID := publicEventTestStory(t)

	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Available) != 0 {
		t.Fatalf("a realm whose entrance is elsewhere must not be offered: %#v", state.Available)
	}
	_, err = store.ApplyRealm(storyID, RealmRequest{
		BranchID: "main", RealmID: exploration.RealmGuXiuDongFu, Action: RealmActionEnter,
		CommandID: "cmd-enter", CommandHash: "h-enter",
	})
	if !errors.Is(err, ErrRealmNotAtEntrance) {
		t.Fatalf("entering a realm from the market must be refused: %v", err)
	}
}

// TestATaskFromAnotherWorldIsNotOffered is §4.10's formal source. A story bound
// to a different revision of the world is not playing the content that revision
// authored.
func TestATaskFromAnotherWorldIsNotOffered(t *testing.T) {
	for name, binding := range map[string]*StoryPackBinding{
		"another pack": {PackID: "other-world", Scope: "pack:other-world"},
	} {
		t.Run(name, func(t *testing.T) {
			store, storyID := eligibilityTestStory(t, "越国坊市外围散修人家", binding, nil)

			state, err := store.QuestStateFor(storyID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Available) != 0 {
				t.Fatalf("a task from %s must not be offered: %#v", name, state.Available)
			}
			_, err = store.ApplyQuest(storyID, QuestRequest{
				BranchID: "main", TaskID: quest.TaskHerbHandover, Action: QuestActionOffer,
				CommandID: "cmd-offer", CommandHash: "h-offer",
			})
			if !errors.Is(err, ErrQuestNotHere) && !errors.Is(err, ErrQuestUnknownTask) {
				t.Fatalf("offering a task from %s must be refused: %v", name, err)
			}
		})
	}
}

// TestAStoryOnALaterContentRevisionKeepsItsTasks is the other half of the
// revision rule: content 5 added world-life rule entries, 6 and 7 rewrote how
// people are described, 26 rewrote the five rule bodies, 27 replaced the
// travel-scale identifiers with wording, and none of them changed an authored
// task, so a story bound to any of them is playing the same gameplay and must
// still be offered it. Bumping the pack without this would empty the task list
// of every new game, which is exactly the regression the migration had to avoid.
func TestAStoryOnALaterContentRevisionKeepsItsTasks(t *testing.T) {
	for _, version := range []int{3, 31, 999} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			store, storyID := eligibilityTestStory(t, "越国坊市外围散修人家", fanrenBinding(version), nil)

			state, err := store.QuestStateFor(storyID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Available) != 1 || state.Available[0].TaskID != quest.TaskHerbHandover {
				t.Fatalf("a story on content %d lost its authored errand: %#v", version, state.Available)
			}
		})
	}
}

// TestATaskFromAnAbsentGiverIsNotOffered is the second half of §4.10: the person
// who raises a task has to be part of this story's own cast.
func TestATaskFromAnAbsentGiverIsNotOffered(t *testing.T) {
	catalog := &PackCatalogSnapshot{
		ContentVersion: 5,
		Characters: []PackCatalogPerson{
			{SourceKey: "npc_someone_else", Name: "旁人"},
		},
	}
	store, storyID := eligibilityTestStory(t, "越国坊市外围散修人家", fanrenBinding(5), catalog)

	state, err := store.QuestStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// The errand has no giver, so it is still this world's business; only the
	// personal task the absent person raises is withheld.
	if len(state.Available) != 1 || state.Available[0].TaskID != quest.TaskHerbHandover {
		t.Fatalf("only the giverless errand may be offered: %#v", state.Available)
	}
	_, err = store.ApplyQuest(storyID, QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: QuestActionOffer,
		CommandID: "cmd-offer", CommandHash: "h-offer",
	})
	if !errors.Is(err, ErrQuestNotHere) {
		t.Fatalf("a task from a person this story does not have must be refused: %v", err)
	}
}

// TestAnEventFromAnotherWorldRevisionIsNotKnown is §4.5's binding half: the
// event belongs to the world revision that authored it.
func TestAnEventFromAnotherWorldRevisionIsNotKnown(t *testing.T) {
	store, storyID := eligibilityTestStory(t, publicEventTestMarket, fanrenBinding(2), nil, &WorldContent{PackID: "fanren-jie"})

	state, err := store.PublicEventStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != 0 {
		t.Fatalf("an event from another revision must not be known: %#v", state.Events)
	}
	_, err = store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionObserve),
		CommandID: "cmd-join", CommandHash: "h-join",
	})
	if !errors.Is(err, ErrPublicEventNotHere) && !errors.Is(err, ErrPublicEventUnknown) {
		t.Fatalf("joining another revision's event must be refused: %v", err)
	}
}

// TestASceneNamingTwoPlacesResolvesToTheMoreSpecificOne is the substring gap
// itself.
//
// The old check accepted any scene whose text contained the region's wording, so
// the question "where is the character" was never really answered. Resolving the
// text to one registered place answers it: a scene that walks out of the market
// and back to the settlement is at the settlement, and the longer wording is what
// says so.
func TestASceneNamingTwoPlacesResolvesToTheMoreSpecificOne(t *testing.T) {
	const returning = "从公共坊市出来，回到越国坊市外围散修人家"
	store, storyID := publicEventTestStoryAt(t, returning)

	// The market's event is not this character's: they are at the settlement
	// outside it, however the sentence mentions the market.
	state := publicEventTestState(t, store, storyID, "")
	if len(state.Events) != 0 {
		t.Fatalf("a character back at the settlement is not in the market: %#v", state.Events)
	}
	_, err := store.ApplyPublicEvent(storyID, PublicEventRequest{
		BranchID: "main", EventID: publiquevent.PublicEventIDTaiyueTide,
		Action: PublicEventActionJoin, ParticipationAction: string(publiquevent.ActionObserve),
		CommandID: "cmd-join", CommandHash: "h-join",
	})
	if !errors.Is(err, ErrPublicEventNotHere) {
		t.Fatalf("an event from the market must not be joined from the settlement: %v", err)
	}

	// The same answer governs the journey entry and the realm entrance, because
	// they read the same committed scene: the road starts here, and the cave's
	// mouth is in these hills.
	options, err := store.JourneyOptionsFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Routes) != 1 || options.Routes[0].RouteID != "route_036:reverse" {
		t.Fatalf("the road out of the settlement must be offered: %#v", options.Routes)
	}
	realms, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(realms.Available) != 2 || realms.Available[0].RealmID != "realm.gu_xiu_dong_fu" || realms.Available[1].RealmID != "realm.mountain_herb_cave" {
		t.Fatalf("the cave is enterable from these hills: %#v", realms.Available)
	}
}

// TestAnEquallySpecificSceneIsRefused keeps the resolver from guessing when the
// scene names two places just as plainly. The rule is deterministic: a tie is not
// a place, and every rule built on the answer inherits the refusal.
func TestAnEquallySpecificSceneIsRefused(t *testing.T) {
	places := []placeCandidate{{ID: "a", Text: "东市"}, {ID: "b", Text: "西市"}}
	if id, ok := matchRegisteredPlace("在东市与西市之间", places); ok {
		t.Fatalf("a tie must not resolve, got %q", id)
	}
	if id, ok := matchRegisteredPlace("东市东市街口", places); !ok || id != "a" {
		t.Fatalf("one named place must resolve: %q %v", id, ok)
	}
	if _, ok := matchRegisteredPlace("在城门口", places); ok {
		t.Fatal("a scene naming no registered place must not resolve")
	}
}

// TestTheRealSettlementSceneResolvesOutsideTheMarket is the case that broke the
// first story played on this build.
//
// The scene field carries the place together with its parent scope, so real prose
// says 越国公共坊市外围… rather than the registry's 越国坊市外围散修人家. Matching
// only the label read that as the market and withheld the road that starts
// outside it.
func TestTheRealSettlementSceneResolvesOutsideTheMarket(t *testing.T) {
	const scene = "越国公共坊市外围，行旅落脚处最里头一间小屋"
	state := map[string]any{
		actorStateRoot: map[string]any{
			DefaultStoryContextActorID: map[string]any{
				"state": map[string]any{storySceneLocationField: scene},
			},
		},
	}
	id, ok := storyLocationID(state)
	if !ok || id != "yue.market.outskirts" {
		t.Fatalf("the settlement outside the market must resolve to its own place: %q %v", id, ok)
	}

	// And the road out of it is offered, while the market's event is not known
	// there.
	store, storyID := publicEventTestStoryAt(t, scene)
	options, err := store.JourneyOptionsFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Routes) != 1 {
		t.Fatalf("the road out of the settlement must be offered: %#v", options.Routes)
	}
	if events := publicEventTestState(t, store, storyID, ""); len(events.Events) != 0 {
		t.Fatalf("the market's event is not known at the settlement: %#v", events.Events)
	}
}

// TestABareOutskirtsWordingNamesNoCountrysOutskirts keeps the resolver from
// guessing at the one wording that looks specific and is not.
//
// 越国坊市外围散修家庭 is the world book's only authored account of yue.market's
// outskirts, and it carries its country. 坊市 on its own names a kind of place all
// three countries have, so 坊市外围 could be any of them — resolving it to 越国
// would put a scene in 元武国 outside a market it is not outside. A wording that
// names its country still resolves; the bare one is refused.
func TestABareOutskirtsWordingNamesNoCountrysOutskirts(t *testing.T) {
	if id, ok := registeredPlaceInText("坊市外围"); ok {
		t.Fatalf("a bare 坊市外围 resolved to %q, and it names no single country's outskirts", id)
	}
	if id, ok := registeredPlaceInText("坊市"); ok {
		t.Fatalf("a bare 坊市 resolved to %q", id)
	}
	for _, scene := range []string{
		"越国坊市外围散修人家",
		"越国坊市外围散修家庭",
		"越国坊市外围",
		"越国公共坊市外围，行旅落脚处最里头一间小屋",
		"公共坊市外围",
	} {
		id, ok := registeredPlaceInText(scene)
		if !ok || id != "yue.market.outskirts" {
			t.Fatalf("%q must resolve to the outskirts, got %q %v", scene, id, ok)
		}
	}
}

// TestThePacksOwnMarketWordingResolvesInsideTheMarket pins the wording the world
// book and the scene prose actually use.
//
// The registry calls the place 公共坊市, but every authored sentence that puts
// someone in it says 越国坊市: 越国坊市正街, 越国坊市西门外, and the rows' own
// place labels. Recognising only the label left the whole market unresolved, so
// a story that opened in 钱守信的丹药铺 offered no road out of it from the first
// turn — the narrator read "no road starts here" and settled nothing, and the
// character could not leave at all.
func TestThePacksOwnMarketWordingResolvesInsideTheMarket(t *testing.T) {
	for _, scene := range []string{
		"越国坊市正街，钱守信的丹药铺内",
		"越国坊市正街街口，王婆子的符摊前",
		"越国公共坊市",
	} {
		id, ok := registeredPlaceInText(scene)
		if !ok || id != "yue.market" {
			t.Fatalf("%q must resolve to the market, got %q %v", scene, id, ok)
		}
	}
	// The road out of the market is offered from the market and nowhere else, so
	// a scene the pack writes about standing in it must produce exactly the
	// forward route.
	store, storyID := publicEventTestStoryAt(t, "越国坊市正街街口，王婆子的符摊前")
	options, err := store.JourneyOptionsFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Routes) != 1 || options.Routes[0].RouteID != "route_036:forward" {
		t.Fatalf("the road out of the market must be offered: %#v", options.Routes)
	}
	// A scene that only mentions the market from outside it must not resolve to
	// the market. The aliases are the pack's own place names rather than the bare
	// prefix 越国坊市 for exactly this reason: a realm entry takes the character
	// off the map, and reading 越国坊市北面半里的山中 as standing in the market
	// would refuse the move a realm really settled.
	if id, ok := registeredPlaceInText("越国坊市北面半里的山中，古修洞府外殿"); ok {
		t.Fatalf("a cave north of the market resolved to %q", id)
	}
}
