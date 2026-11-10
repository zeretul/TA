package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/exploration"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/storypack"
)

func TestRealmCommandHashPreservesLegacyCommandsAndIncludesApproach(t *testing.T) {
	request := RealmAppRequest{StoryID: "st_legacy", RealmID: "guxiu", InstanceID: "instance-1", Action: "reward", ContentRefID: "herbs"}
	// Golden value from the pre-exploration wire contract, not recomputed with
	// today's hash helper: an old receipt must remain replayable after upgrade.
	const legacy = "sha256:a6c210008852f179e64f9ee1b0cf2f1f006ccbaa5d28f1b4a74e9f5b0fa2b5df"
	if got := realmCommandHash(request, "main", "cmd"); got != legacy {
		t.Fatalf("legacy receipt hash changed: %q", got)
	}
	request.ApproachID = " \t "
	if got := realmCommandHash(request, "main", "cmd"); got != legacy {
		t.Fatalf("empty approach must preserve legacy receipt: %q", got)
	}
	request.ApproachID = "slow"
	slow := realmCommandHash(request, "main", "cmd")
	request.ApproachID = " slow "
	request.Locale = "en-US"
	if got := realmCommandHash(request, "main", "cmd"); got != slow || got == legacy {
		t.Fatalf("trimmed choice identity must have its own stable hash: %q", got)
	}
	request.ApproachID = "force"
	if got := realmCommandHash(request, "main", "cmd"); got == slow || got == legacy {
		t.Fatalf("different approach reused another command's hash: %q", got)
	}
	if got := realmCommandHash(request, "main", ""); got != "" {
		t.Fatalf("a command without identity must not claim idempotency: %q", got)
	}
}

func TestRealmRewardViewUsesFrozenCatalogueNames(t *testing.T) {
	grant := exploration.InstanceReward{ID: "grant-1", RefID: "wuxia-herb", Quantity: 2, SourceRef: "place.herbs"}
	rules := &interactive.AdventureRules{Items: []interactive.AdventureItem{{ID: "wuxia-herb", Name: "本次存档冻结的药草名"}}}
	view := realmRewardView(grant, rules)
	if view.ID != grant.ID || view.RefID != grant.RefID || view.SourceRef != grant.SourceRef || view.Quantity != grant.Quantity || view.Name != rules.Items[0].Name || view.NameKey != "" {
		t.Fatalf("reward projection lost the frozen native identity or wording: %+v", view)
	}
	unknown := realmRewardView(grant, nil)
	if unknown.Name != "" || unknown.NameKey != "" {
		t.Fatalf("unknown catalogue entry became a fabricated display name: %+v", unknown)
	}
	cultivation := realmRewardView(exploration.InstanceReward{RefID: "material.herb.green_vine", Quantity: 1}, nil)
	if cultivation.NameKey == "" || cultivation.NameKey == cultivation.RefID {
		t.Fatalf("cultivation grant lost its real catalogue localization key: %+v", cultivation)
	}
}

// This is a deterministic App -> Story -> journal integration, not natural AI
// play. It checks the transport/read contract rather than duplicating the full
// authored-place and resource settlement suite below it.
func TestRealmExploreAppProjectsCommittedChoicesTimeAndRewards(t *testing.T) {
	application, storyID := realmAppTestApp(t)
	initial, err := application.RealmStateFor(storyID, "")
	if err != nil {
		t.Fatal(err)
	}
	var offered *interactive.RealmOption
	for index := range initial.Available {
		if initial.Available[index].RealmID == exploration.RealmMountainHerbCave {
			offered = &initial.Available[index]
		}
	}
	if offered == nil || offered.NameKey == "" || offered.DescriptionKey == "" {
		t.Fatalf("authored place offer lost its localization keys: %+v", initial.Available)
	}
	entered, err := application.ApplyRealm(RealmAppRequest{StoryID: storyID, RealmID: exploration.RealmMountainHerbCave, Action: "enter", CommandID: "cave-enter"})
	if err != nil {
		t.Fatal(err)
	}
	if entered.Rewards == nil || len(entered.Rewards) != 0 || entered.Reward != nil || entered.DurationSeconds != 0 {
		t.Fatalf("entering must not invent a grant or time: %+v", entered)
	}
	before, err := application.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	beforeJSON, err := json.Marshal(before.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := application.RealmStateFor(storyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 || state.ClockSeconds != initial.ClockSeconds {
		t.Fatalf("read lost the entered instance or changed the clock: %+v", state)
	}
	instance := state.Instances[0]
	if instance.Explore == nil || instance.Explore.Approaches == nil || instance.NameKey == "" || instance.DescriptionKey == "" {
		t.Fatalf("read omitted authored exploration projection: %+v", instance)
	}
	var wardSeen bool
	for _, approach := range instance.Explore.Approaches {
		if approach.Costs == nil {
			t.Fatalf("choice costs must serialize as an array: %+v", approach)
		}
		if approach.ID == exploration.RealmMountainHerbCave+".preserve" || approach.ID == exploration.RealmMountainHerbCave+".slow" {
			t.Fatalf("undiscovered route leaked through App state: %+v", approach)
		}
		if approach.ID == exploration.RealmMountainHerbCave+".ward" {
			wardSeen = true
			if len(approach.Costs) != 1 || approach.Costs[0].NameKey == "" || approach.Costs[0].RefID != "talisman.ward.lower" || approach.Available || approach.BlockedReason == "" {
				t.Fatalf("visible cost must preserve native name and real missing-item eligibility: %+v", approach)
			}
		}
	}
	if !wardSeen {
		t.Fatal("known resource alternative is missing")
	}
	if _, err := application.RealmStateFor(storyID, "main"); err != nil {
		t.Fatal(err)
	}
	afterRead, err := application.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(afterRead.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("opening the realm entry wrote Story state, clock or history")
	}
	request := RealmAppRequest{StoryID: storyID, BranchID: "main", InstanceID: entered.InstanceID, Action: "survey", ApproachID: exploration.RealmMountainHerbCave + ".survey", CommandID: "cave-survey"}
	survey, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	if survey.DurationSeconds != 600 || survey.Rewards == nil || len(survey.Rewards) != 0 {
		t.Fatalf("survey must report only the authored time: %+v", survey)
	}
	request.BranchID = ""
	retry, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Replayed || retry.EventID != survey.EventID || retry.DurationSeconds != survey.DurationSeconds || !reflect.DeepEqual(retry.Rewards, survey.Rewards) {
		t.Fatalf("retry changed settled result: %+v versus %+v", retry, survey)
	}
	request.ApproachID = exploration.RealmMountainHerbCave + ".force"
	if _, err := application.ApplyRealm(request); !errors.Is(err, interactive.ErrRealmCommandConflict) {
		t.Fatalf("same command with changed authored choice must conflict: %v", err)
	}
	request = RealmAppRequest{StoryID: storyID, InstanceID: entered.InstanceID, Action: "resolve", ApproachID: exploration.RealmMountainHerbCave + ".outer", CommandID: "cave-herb"}
	granted, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	if granted.DurationSeconds != 300 || len(granted.Rewards) != 1 {
		t.Fatalf("resolved approach lost time or plural grant receipt: %+v", granted)
	}
	reward := granted.Rewards[0]
	if reward.ID == "" || reward.RefID != "material.herb.green_vine" || reward.NameKey == "" || reward.Quantity != 1 || reward.SourceRef == "" {
		t.Fatalf("grant lost its catalogue identity: %+v", reward)
	}
	grantRetry, err := application.ApplyRealm(request)
	if err != nil {
		t.Fatal(err)
	}
	if !grantRetry.Replayed || grantRetry.EventID != granted.EventID || !reflect.DeepEqual(grantRetry.Rewards, granted.Rewards) {
		t.Fatalf("grant retry lost original plural receipt: %+v", grantRetry)
	}
	state, err = application.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if state.ClockSeconds != initial.ClockSeconds+900 {
		t.Fatalf("App must expose exact committed time, not charge retries: %d -> %d", initial.ClockSeconds, state.ClockSeconds)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`"approaches":null`, `"costs":null`, `"rewards":null`, `"granted":null`, `"ways":null`} {
		if strings.Contains(string(encoded), invalid) {
			t.Fatalf("nullable collection at App boundary: %s", encoded)
		}
	}
}

func TestRealmNativeLocaleUsesBoundSidecarWithoutChangingSettlement(t *testing.T) {
	for _, test := range []struct{ packID, realmID, locationID string }{
		{"lanchuan-qunxia", exploration.RealmStrandedTideBoat, "lanchuan-l02"},
		{"xinghai-689", exploration.RealmShaftServiceBay, "xinghai-l05"},
	} {
		t.Run(test.packID, func(t *testing.T) {
			root := t.TempDir()
			application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
			service := &InteractiveAppService{app: application}
			pack, err := storypack.NewLibrary(root).Get(test.packID)
			if err != nil {
				t.Fatal(err)
			}
			req, _, err := pack.StoryRequestForTemplate(pack.ProtagonistTemplates[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: test.packID, TemplateID: pack.ProtagonistTemplates[0].ID}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatal(err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(root)
			t.Cleanup(func() { _ = store.Close() })
			application.interactive = store
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			var location string
			for _, place := range pack.World {
				if place.ID == test.locationID {
					location = place.Name
				}
			}
			if location == "" {
				t.Fatal("authored entry location is missing")
			}
			// Establish only a descriptive scene, not a resource or successful
			// approach. This is an automatic App integration, not model play.
			_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察周围", Narrative: "你看清了所在地点。", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: location}}, Choices: []string{"观察", "询问", "出发"}}})
			if err != nil {
				t.Fatal(err)
			}
			entered, err := application.ApplyRealm(RealmAppRequest{StoryID: story.ID, RealmID: test.realmID, Action: "enter", CommandID: "enter"})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			names := pack.Localizations["en-US"].Adventure
			zh, err := application.RealmStateForLocale(story.ID, "main", "zh-CN")
			if err != nil {
				t.Fatal(err)
			}
			en, err := application.RealmStateForLocale(story.ID, "main", "en-US")
			if err != nil {
				t.Fatal(err)
			}
			if len(zh.Instances) != 1 || len(en.Instances) != 1 || zh.Instances[0].Explore == nil || en.Instances[0].Explore == nil {
				t.Fatal("actual authored exploration view missing")
			}
			var costsChecked int
			for i, choice := range en.Instances[0].Explore.Approaches {
				for j, cost := range choice.Costs {
					native := zh.Instances[0].Explore.Approaches[i].Costs[j]
					if names[cost.RefID] == "" || cost.Name != names[cost.RefID] || cost.Name == native.Name {
						t.Fatalf("cost did not use this frozen pack's exact sidecar: en=%+v zh=%+v", cost, native)
					}
					if cost.RefID != native.RefID || cost.Quantity != native.Quantity || cost.EnergyCost != native.EnergyCost || choice.Available != zh.Instances[0].Explore.Approaches[i].Available {
						t.Fatal("localization changed rule eligibility or payment")
					}
					costsChecked++
				}
			}
			if costsChecked == 0 {
				t.Fatal("test did not exercise a real item or skill cost")
			}
			zhAgain, err := application.RealmStateFor(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(zhAgain, zh) {
				t.Fatal("English projection mutated later native reads")
			}
			after, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			beforeJSON, err := json.Marshal(before.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			afterJSON, err := json.Marshal(after.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if string(beforeJSON) != string(afterJSON) {
				t.Fatalf("localized read wrote canonical Story state:\nbefore: %s\nafter: %s", beforeJSON, afterJSON)
			}
			staleBinding := *before.Meta.PackBinding
			staleBinding.ContentHash = "sha256:another-revision"
			if got := application.realmAdventureNames(&staleBinding, "en-US"); len(got) != 0 {
				t.Fatal("a different pack revision renamed the saved catalogue")
			}
			request := RealmAppRequest{StoryID: story.ID, InstanceID: entered.InstanceID, Action: "resolve", ApproachID: test.realmID + ".outer", CommandID: "outer", Locale: "en-US"}
			granted, err := application.ApplyRealm(request)
			if err != nil {
				t.Fatal(err)
			}
			if len(granted.Rewards) != 1 || names[granted.Rewards[0].RefID] == "" || granted.Rewards[0].Name != names[granted.Rewards[0].RefID] {
				t.Fatalf("action response lost authored English grant name: %+v", granted.Rewards)
			}
			request.Locale = "zh-CN"
			retry, err := application.ApplyRealm(request)
			if err != nil {
				t.Fatal(err)
			}
			if !retry.Replayed || retry.EventID != granted.EventID || len(retry.Rewards) != 1 || retry.Rewards[0].Name == granted.Rewards[0].Name || retry.Rewards[0].RefID != granted.Rewards[0].RefID || retry.DurationSeconds != granted.DurationSeconds {
				t.Fatalf("language-only retry changed settlement or failed to restore native name: %+v", retry)
			}
			zh, err = application.RealmStateFor(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			en, err = application.RealmStateForLocale(story.ID, "main", "en-US")
			if err != nil {
				t.Fatal(err)
			}
			if len(zh.Instances[0].Granted) != 1 || len(en.Instances[0].Granted) != 1 || zh.Instances[0].Granted[0].Name != retry.Rewards[0].Name || en.Instances[0].Granted[0].Name != granted.Rewards[0].Name || zh.ClockSeconds != en.ClockSeconds {
				t.Fatalf("saved grant projection did not preserve identity and locale: zh=%+v en=%+v", zh.Instances[0].Granted, en.Instances[0].Granted)
			}
		})
	}
}
