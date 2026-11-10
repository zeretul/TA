package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/presetlayout"
	"ta/internal/quest"
	"ta/internal/storypack"
)

// A model-free domain campaign: keep the real worker's zero-cash origin, earn
// authored rewards, sell one reward, then buy the radio in the actual market.
// Only the opening's descriptive scene is supplied; no resources are injected.
func TestSpaceRadioNormalAcquisitionKeepsContactSeparate(t *testing.T) {
	const device = "xinghai-radio"
	root := t.TempDir()
	application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
	service := &InteractiveAppService{app: application}
	pack, err := storypack.NewLibrary(root).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := pack.StoryRequestForTemplate("xinghai-c01")
	if err != nil {
		t.Fatal(err)
	}
	req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: pack.ID, TemplateID: "xinghai-c01"}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatal(err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(root)
	defer func() { _ = store.Close() }()
	application.interactive = store
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察周围", Narrative: "你看清了所在地点。", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: "朱伯利外环47号减压长廊"}}, Choices: []string{"观察", "询问", "出发"}}})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	command := func() string { sequence++; return fmt.Sprintf("radio-acquisition-%d", sequence) }
	read := func() interactive.AdventureWindowState {
		t.Helper()
		window, e := store.AdventureWindow(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		return window
	}
	action := func(input interactive.AdventureAction) error {
		_, _, e := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: input.Action, Narrative: "你按商定的方式办事。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: input.Action, Adventure: &input}}})
		return e
	}
	choose := func(task, choice string) {
		t.Helper()
		for _, step := range []string{"accept", "choose"} {
			result, e := store.ApplyQuest(story.ID, interactive.QuestRequest{BranchID: "main", TaskID: task, Action: step, ChoiceID: choice, CommandID: command(), CommandHash: interactive.ContentHashOf([]string{task, step, choice})})
			if e != nil {
				t.Fatal(e)
			}
			if step == "choose" {
				found := false
				for _, resolution := range result.Instance.Choices {
					if resolution.ChoiceID == choice {
						found = true
						if resolution.Outcome != "success" {
							t.Fatalf("earned-resource choice failed: %+v", resolution)
						}
					}
				}
				if !found {
					t.Fatal("choice has no committed resolution")
				}
			}
		}
	}
	available := func(want bool) {
		t.Helper()
		ctx, e := store.StoryContext(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		state, e := store.CharacterState(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		if got := radioContactAvailable(ctx, state, "remote"); got != want {
			t.Fatalf("radio available = %v, want %v", got, want)
		}
		if !want {
			_, e = application.SendCharacterMessage(context.Background(), CharacterChatRequest{StoryID: story.ID, BranchID: "main", CharacterID: "remote", Message: "能听见吗？", CommandID: command()})
			if !errors.Is(e, ErrRadioContactUnavailable) {
				t.Fatalf("unavailable radio send error = %v", e)
			}
		}
	}
	contact := func(status string) {
		t.Helper()
		_, _, e := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "确认联络状态", Narrative: "对方确认了约定频道的当前状态。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "等待"}, CharacterFacts: &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "remote", Name: "哨站联络员", Description: "曾与你实际交换联络方式的哨站联络员。", RadioContact: &character.RadioContact{DeviceItemID: device, Channel: "agreed-local-channel", Status: status}}}}}})
		if e != nil {
			t.Fatal(e)
		}
	}
	initial := read()
	if initial.Actor.Currency != 0 || initial.Actor.Inventory[device] != 0 {
		t.Fatal("worker was given free funds or a radio")
	}
	// A committed contact still cannot transmit without an owned device.
	contact("connected")
	available(false)
	buy := interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: device, Quantity: 1}
	if e := action(buy); e == nil || !reflect.DeepEqual(read().Actor, initial.Actor) {
		t.Fatal("remote-market refusal mutated the actor")
	}
	choose("xinghai-a01", "xinghai-a01.c1")
	options, err := store.JourneyOptionsFor(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	arrived := false
	for _, route := range options.Routes {
		if route.DestinationID == "xinghai-l02" {
			result, e := store.ApplyJourney(story.ID, interactive.JourneyRequest{BranchID: "main", RouteID: route.RouteID, Arrive: true, CommandID: command(), CommandHash: interactive.ContentHashOf(route.RouteID)})
			if e != nil || !result.Arrived {
				t.Fatalf("normal market journey: %+v %v", result, e)
			}
			arrived = true
			break
		}
	}
	if !arrived {
		t.Fatal("no registered route to the radio market")
	}
	before := read()
	if before.Actor.Currency != 0 {
		t.Fatal("debt settlement unexpectedly granted spending money")
	}
	if e := action(buy); e == nil || !reflect.DeepEqual(read().Actor, before.Actor) {
		t.Fatal("insufficient-funds refusal mutated the actor")
	}
	choose("xinghai-a16", "xinghai-a16.c2")
	if read().Actor.Inventory["xinghai-i07"] != 3 {
		t.Fatal("normal work did not pay its authored reward")
	}
	if e := action(interactive.AdventureAction{Action: "sell", MarketID: buy.MarketID, ItemID: "xinghai-i07", Quantity: 1}); e != nil {
		t.Fatal(e)
	}
	before = read()
	if e := action(buy); e != nil {
		t.Fatalf("buy source-authored radio with earned funds: %v", e)
	}
	after := read()
	if after.Actor.Currency != before.Actor.Currency-120 || after.Actor.Inventory[device] != 1 || after.Actor.Inventory["xinghai-i07"] != 2 {
		t.Fatal("radio purchase did not debit its price and preserve remaining earned stock")
	}
	if !reflect.DeepEqual(after.Actor.Skills, before.Actor.Skills) {
		t.Fatal("buying a radio granted a decryption skill or other training")
	}
	for locale, want := range map[string]string{"zh-CN": "全频段便携无线电", "en-US": "Portable Full-Band Radio"} {
		view, e := application.AdventureWindow(story.ID, "main", locale)
		if e != nil {
			t.Fatal(e)
		}
		name := ""
		for _, item := range view.Items {
			if item.ID == device {
				name = item.Name
			}
		}
		if name != want {
			t.Fatalf("%s owned-radio display = %q, want %q", locale, name, want)
		}
	}
	available(true)
	// Ownership does not connect another known person or repair an offline link.
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "结识商贩", Narrative: "你认得了新的商贩，但没有交换频道。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "等待"}, CharacterFacts: &interactive.TurnCharacterFactsInput{Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "unconnected", Name: "二手商贩", Description: "没有交换通信方式的商贩。"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := store.StoryContext(story.ID, "main")
	state, _ := store.CharacterState(story.ID, "main")
	if radioContactAvailable(ctx, state, "unconnected") {
		t.Fatal("buying hardware created a contact channel")
	}
	_, err = application.SendCharacterMessage(context.Background(), CharacterChatRequest{StoryID: story.ID, BranchID: "main", CharacterID: "unconnected", Message: "能听见吗？", CommandID: command()})
	if !errors.Is(err, ErrRadioContactUnavailable) {
		t.Fatalf("owned hardware without a contact channel was not refused: %v", err)
	}
	contact("offline")
	available(false)
	contact("connected")
	available(true)
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(root)
	application.interactive = store
	available(true)
	if e := action(interactive.AdventureAction{Action: "sell", MarketID: buy.MarketID, ItemID: device, Quantity: 1}); e != nil {
		t.Fatal(e)
	}
	available(false)
}

// The original 96 tasks retain their revision-1 source contract. The optional
// beacon investigation starts at revision 4 rather than entering old stories.
// New pack content admits the old contract, while hashes and frozen snapshots
// isolate already-created stories from new devices and local work services.
func TestSpaceRadioNewCatalogKeepsOldStoryAndTaskBindings(t *testing.T) {
	for _, addition := range []struct {
		name           string
		items          map[string]bool
		recipe, market string
	}{
		{name: "radio", items: map[string]bool{"xinghai-radio": true}},
		{name: "signal-processor", items: map[string]bool{"xinghai-signal-processor": true, "xinghai-hopping-recording": true, "xinghai-signal-fragments": true}, recipe: "xinghai-decode-hopping", market: "xinghai-market-l21"},
	} {
		t.Run(addition.name, func(t *testing.T) {
			root := t.TempDir()
			current, err := storypack.NewLibrary(root).Get("xinghai-689")
			if err != nil {
				t.Fatal(err)
			}
			if current.ContentVersion < 4 {
				t.Fatalf("the shipped catalog predates the revision-4 additions: %d", current.ContentVersion)
			}
			count, additions := 0, 0
			for _, definition := range quest.Definitions() {
				if definition.PackageID == current.ID {
					if definition.ID == "xinghai-repeating-beacon" || definition.ID == "quest.blackstar_member_office_visit" {
						additions++
						if definition.PackageVersion != "4" {
							t.Fatal("new affair leaked into revision-1 source content")
						}
						continue
					}
					count++
					if definition.PackageVersion != "1" {
						t.Fatalf("task %s no longer binds exact space revision 1", definition.ID)
					}
				}
			}
			if count != 96 || additions != 2 {
				t.Fatalf("space tasks: legacy=%d new=%d, want 96 preserved source contracts and two new revision-4 tasks", count, additions)
			}
			raw, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			var previous storypack.Pack
			if err = json.Unmarshal(raw, &previous); err != nil {
				t.Fatal(err)
			}
			// Build a synthetic earlier device catalog at the original revision-4
			// boundary. This is not an archived complete revision-4 world pack.
			previous.ContentVersion = 4
			world := previous.World[:0]
			for _, entry := range previous.World {
				if !addition.items[entry.ID] {
					world = append(world, entry)
				}
			}
			previous.World = world
			rules := previous.Story.Gameplay.AdventureRules
			if addition.name == "signal-processor" {
				for index := range rules.Items {
					if rules.Items[index].ID == "xinghai-radio" {
						rules.Items[index].RadioDevice = false
					}
				}
			}

			items := rules.Items[:0]
			for _, item := range rules.Items {
				if !addition.items[item.ID] {
					items = append(items, item)
				}
			}
			rules.Items = items
			markets := rules.Markets[:0]
			for _, market := range rules.Markets {
				if market.ID == addition.market {
					continue
				}
				stock := market.Items[:0]
				for _, id := range market.Items {
					if !addition.items[id] {
						stock = append(stock, id)
					}
				}
				market.Items = stock
				markets = append(markets, market)
			}
			rules.Markets = markets
			recipes := rules.Recipes[:0]
			for _, recipe := range rules.Recipes {
				if recipe.ID != addition.recipe {
					recipes = append(recipes, recipe)
				}
			}
			rules.Recipes = recipes
			if current.ContentHash() == previous.ContentHash() || current.WorldScope() == previous.WorldScope() {
				t.Fatal("adding the radio did not change the content hash and lore scope")
			}
			store := interactive.NewStore(root)
			defer func() { _ = store.Close() }()
			service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}, interactive: store}}
			expectedHashes := map[string]string{}
			create := func(pack storypack.Pack) interactive.StorySummary {
				t.Helper()
				if e := os.MkdirAll(presetlayout.StoryPacks(root), 0o755); e != nil {
					t.Fatal(e)
				}
				if e := writePackFileForTest(t, filepath.Join(presetlayout.StoryPacks(root), pack.ID+".json"), pack); e != nil {
					t.Fatal(e)
				}
				loaded, e := storypack.NewLibrary(root).Get(pack.ID)
				if e != nil {
					t.Fatal(e)
				}
				req, _, e := loaded.StoryRequestForTemplate("xinghai-c01")
				if e != nil {
					t.Fatal(e)
				}
				req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: loaded.ID, TemplateID: "xinghai-c01"}
				req, e = service.withStoryCreationDefaults(req)
				if e != nil {
					t.Fatal(e)
				}
				req, e = service.withPackTemplate(req)
				if e != nil {
					t.Fatal(e)
				}
				story, e := store.CreateStory(req)
				if e != nil {
					t.Fatal(e)
				}
				_, _, e = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察周围", Narrative: "你看清了所在地点。", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: "朱伯利外环47号减压长廊"}}, Choices: []string{"观察", "询问", "出发"}}})
				if e != nil {
					t.Fatal(e)
				}
				// Loading materializes rule-derived module IDs. The resolved author pack,
				// not its pre-materialization reconstruction, is the hash to freeze.
				expectedHashes[story.ID] = loaded.ContentHash()
				return story
			}
			oldStory := create(previous)
			before, err := store.AdventureWindow(oldStory.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			beforeContext, err := store.StoryContext(oldStory.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			newStory := create(current)
			newContext, err := store.StoryContext(newStory.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if expectedHashes[oldStory.ID] == expectedHashes[newStory.ID] || beforeContext.Meta.PackBinding.ContentHash != expectedHashes[oldStory.ID] || newContext.Meta.PackBinding.ContentHash != expectedHashes[newStory.ID] {
				t.Fatal("old/new stories did not freeze their own exact content hashes")
			}
			newWindow, err := store.AdventureWindow(newStory.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			contains := func(items []interactive.AdventureItem, id string) bool {
				for _, item := range items {
					if item.ID == id {
						return true
					}
				}
				return false
			}
			for id := range addition.items {
				if contains(before.Rules.Items, id) || !contains(newWindow.Rules.Items, id) {
					t.Fatalf("%s availability did not respect frozen rule catalog", id)
				}
			}
			oldQuests, err := store.QuestStateFor(oldStory.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			newQuests, err := store.QuestStateFor(newStory.ID, "main")
			if err != nil || !reflect.DeepEqual(oldQuests, newQuests) {
				t.Fatalf("radio authoring changed task availability: %v", err)
			}
			found := false
			for _, option := range newQuests.Available {
				found = found || option.TaskID == "xinghai-a01"
			}
			if !found {
				t.Fatal("representative public task is no longer available in the new story")
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			store = interactive.NewStore(root)
			after, err := store.AdventureWindow(oldStory.ID, "main")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("new author catalog changed the old story after cold replay: %v", err)
			}
			afterContext, err := store.StoryContext(oldStory.ID, "main")
			if err != nil || !reflect.DeepEqual(beforeContext.Meta.PackBinding, afterContext.Meta.PackBinding) || !reflect.DeepEqual(beforeContext.Meta.PackCatalog, afterContext.Meta.PackCatalog) {
				t.Fatalf("old story's content binding or frozen people catalog changed: %v", err)
			}
		})
	}
}

// These are minimal rule fixtures, not reconstructed historical saves. Real
// Store reads and commits must respect each frozen revision even when the
// process has loaded today's author catalog. The catalog-isolation test above
// separately covers application creation and cold reopening.
func TestSpaceRadioTaskRevisionCompatibilityUsesFrozenBinding(t *testing.T) {
	current, err := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int{1, 4, current.ContentVersion, current.ContentVersion + 1000} {
		for _, task := range []struct {
			id, location string
			introduced   int
		}{
			{"xinghai-a01", "xinghai-l01", 1},
			{"xinghai-repeating-beacon", "xinghai-l21", 4},
		} {
			t.Run(fmt.Sprintf("revision-%d/%s", revision, task.id), func(t *testing.T) {
				definition, ok := quest.DefinitionByID(task.id)
				if !ok || definition.PackageVersion != fmt.Sprint(task.introduced) {
					t.Fatalf("task source revision changed: %+v", definition)
				}
				place := interactive.JourneyPlaceLabel(task.location)
				rules := current.Story.Gameplay.AdventureRules
				actor := interactive.NewAdventureActor(*rules)
				raw, err := json.Marshal(actor)
				if err != nil {
					t.Fatal(err)
				}
				var state map[string]any
				if err := json.Unmarshal(raw, &state); err != nil {
					t.Fatal(err)
				}
				content := interactive.CloneWorldContent(current.Story.Gameplay.WorldContent)
				content.Quests = nil
				content.RoutePolicies = nil
				content.QuestEnglish = nil
				if revision >= task.introduced {
					content.Quests = []quest.Definition{definition}
				}
				system := interactive.StoryDirectorActorStateSystem{
					WorldContent:   content,
					AdventureRules: rules,
					Templates: []interactive.ActorStateTemplate{
						{ID: interactive.DefaultActorID, Name: "Player", Fields: interactive.AdventureStateFields(*rules)},
						{ID: "story", Name: "Scene", Fields: []interactive.ActorStateField{{Name: "当前详细地点", Type: "string", Default: place}}},
					},
					InitialActors: []interactive.ActorStateInitialActor{
						{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: state}},
						{ID: "story", Name: "Scene", TemplateID: "story", State: map[string]any{"当前详细地点": place}},
					},
				}
				store := interactive.NewStore(t.TempDir())
				defer store.Close()
				story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Frozen task revision", ActorState: &system, PackBinding: &interactive.StoryPackBinding{PackID: current.ID, ContentVersion: revision, Scope: fmt.Sprintf("xinghai-revision-fixture-%d", revision)}})
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "查看当地告示", Narrative: "你看见了当地的公开告示。", TurnResult: &interactive.TurnResult{Choices: []string{"观察", "询问", "离开", "等待", "查看告示"}}})
				if err != nil {
					t.Fatal(err)
				}
				quests, err := store.QuestStateFor(story.ID, "main")
				if err != nil {
					t.Fatal(err)
				}
				available := false
				for _, option := range quests.Available {
					available = available || option.TaskID == task.id
				}
				want := revision >= task.introduced
				if available != want {
					t.Fatalf("task availability = %t, want %t for frozen revision %d", available, want, revision)
				}
				before, err := store.Snapshot(story.ID, "main")
				if err != nil {
					t.Fatal(err)
				}
				_, err = store.ApplyQuest(story.ID, interactive.QuestRequest{BranchID: "main", TaskID: task.id, Action: interactive.QuestActionAccept})
				if want {
					if err != nil {
						t.Fatalf("eligible task could not be accepted: %v", err)
					}
				} else {
					if !errors.Is(err, interactive.ErrQuestNotHere) && !errors.Is(err, interactive.ErrQuestUnknownTask) {
						t.Fatalf("ineligible task acceptance returned %v", err)
					}
					after, err := store.Snapshot(story.ID, "main")
					if err != nil {
						t.Fatal(err)
					}
					// Snapshot caching round-trips optional empty schema fields
					// through JSON. Compare the complete serialized contract, not
					// nil versus empty Go slices which encode identically.
					beforeJSON, err := json.Marshal(before)
					if err != nil {
						t.Fatal(err)
					}
					afterJSON, err := json.Marshal(after)
					if err != nil || string(beforeJSON) != string(afterJSON) {
						t.Fatalf("rejected task changed the frozen story: %v", err)
					}
				}
			})
		}
	}
}
