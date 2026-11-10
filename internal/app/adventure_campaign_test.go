package app

import (
	"fmt"
	"reflect"
	"ta/config"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/storypack"
	"testing"
)

// These are deterministic domain campaigns, not model/UI acceptance. Only the
// opening's descriptive location is supplied; every resource, skill, reward,
// journey, injury and recovery thereafter comes from a public settlement API.
func TestAdventureAuthoredResourceCampaigns(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(packID, func(t *testing.T) {
			root := t.TempDir()
			service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
			pack, err := storypack.NewLibrary(root).Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			req, _, err := pack.StoryRequestForTemplate(pack.ProtagonistTemplates[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: packID, TemplateID: pack.ProtagonistTemplates[0].ID}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatal(err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(root)
			service.app.interactive = store
			defer func() { _ = store.Close() }()
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			location := "潮信渡"
			if packID == "xinghai-689" {
				location = "嘉顿星系·朱伯利枢纽 外环47号减压长廊，旅人的集装箱泊位前"
			}
			_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "观察周围", Narrative: "你看清了所在地点。", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/story/当前详细地点", Value: location}}, Choices: []string{"观察", "询问", "出发"}}})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			command := func() string { seq++; return fmt.Sprintf("campaign-%d", seq) }
			read := func() interactive.AdventureWindowState {
				t.Helper()
				w, e := store.AdventureWindow(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				return w
			}
			action := func(in interactive.AdventureAction) {
				t.Helper()
				var e error
				if in.Action == "battle_round" {
					w := read()
					_, e = store.CommitAdventureAction(story.ID, "main", command(), w.Head, in)
				} else {
					_, _, e = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: fmt.Sprintf("执行%s %d小时", in.Action, in.Hours), Narrative: "你按计划采取行动。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: in.Action, Adventure: &in}}})
				}
				if e != nil {
					t.Fatalf("%+v: %v", in, e)
				}
			}
			journey := func(destination string) {
				t.Helper()
				options, e := store.JourneyOptionsFor(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				for _, r := range options.Routes {
					if r.DestinationID == destination {
						result, e := store.ApplyJourney(story.ID, interactive.JourneyRequest{BranchID: "main", RouteID: r.RouteID, Arrive: true, CommandID: command(), CommandHash: interactive.ContentHashOf(r.RouteID)})
						if e != nil {
							t.Fatal(e)
						}
						if !result.Arrived {
							t.Fatal("journey did not arrive")
						}
						return
					}
				}
				t.Fatalf("no registered route from %s to %s: %+v", options.CurrentLocation, destination, options.Routes)
			}
			choose := func(task, choice string) {
				t.Helper()
				for _, a := range []string{"accept", "choose"} {
					r, e := store.ApplyQuest(story.ID, interactive.QuestRequest{BranchID: "main", TaskID: task, Action: a, ChoiceID: choice, CommandID: command(), CommandHash: interactive.ContentHashOf([]string{task, a, choice})})
					if e != nil {
						t.Fatalf("%s %s: %v", task, a, e)
					}
					if a == "choose" {
						found := false
						for _, c := range r.Instance.Choices {
							if c.ChoiceID == choice {
								found = true
							}
							if c.ChoiceID == choice && c.Outcome != "success" {
								t.Fatalf("authored qualified choice failed: %+v", c)
							}
						}
						if !found {
							t.Fatal("choice produced no canonical resolution")
						}
					}
				}
			}
			initial := read()
			if packID == "lanchuan-qunxia" {
				if initial.Actor.Currency != 0 {
					t.Fatal("boatman origin must not be funded by fixture")
				}
				action(interactive.AdventureAction{Action: "train", SkillID: "lanchuan-basic-fist", Hours: 8})
				journey("lanchuan-l01")
				journey("lanchuan-l04")
				choose("lanchuan-a03", "lanchuan-a03.c1")
				if read().Actor.Currency != 1000 {
					t.Fatal("boat work did not pay authored reward")
				}
				journey("lanchuan-l01")
				journey("lanchuan-l02")
				action(interactive.AdventureAction{Action: "battle_start", OpponentID: "lanchuan-sparring", Scale: "person"})
			} else {
				if initial.Actor.Currency != 0 {
					t.Fatal("worker origin must not be funded by fixture")
				}
				choose("xinghai-a01", "xinghai-a01.c1")
				journey("xinghai-l02")
				choose("xinghai-a16", "xinghai-a16.c2")
				if read().Actor.Inventory["xinghai-i07"] != 3 {
					t.Fatal("salvage did not pay three authored grenades")
				}
				choose("xinghai-training-s03", "xinghai-training-s03.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s03", Hours: 4})
				action(interactive.AdventureAction{Action: "sell", MarketID: "xinghai-market-l02", ItemID: "xinghai-i07", Quantity: 1})
				for id, n := range map[string]int{"xinghai-i33": 4, "xinghai-i31": 2, "xinghai-i37": 2} {
					action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: id, Quantity: n})
				}
				journey("xinghai-l01")
				journey("xinghai-l03")
				before := read()
				action(interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-assemble-exoskeleton", Hours: 12})
				after := read()
				if after.Actor.Inventory["xinghai-i09"] != before.Actor.Inventory["xinghai-i09"]+1 || after.Actor.Inventory["xinghai-i33"] != before.Actor.Inventory["xinghai-i33"]-4 {
					t.Fatal("craft did not consume earned materials and produce asset")
				}
				action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-i09"})
				// Route through the registered graph, not an arbitrary scene teleport.
				journey("xinghai-l04")
				journey("xinghai-l21")
				journey("xinghai-l22")
				journey("xinghai-l23")
				journey("xinghai-l38")
				action(interactive.AdventureAction{Action: "battle_start", OpponentID: "xinghai-e05", Scale: "machine"})
			}
			// An honest loss is acceptable; a fabricated victory or an endless battle is not.
			rounds := 0
			for read().Actor.Battle.Status == "active" && rounds < 100 {
				action(interactive.AdventureAction{Action: "battle_round", Choice: "attack"})
				rounds++
			}
			checkpoint, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "查看战后情况", Narrative: "战斗已经结束。", TurnResult: &interactive.TurnResult{Choices: []string{"休息", "观察", "出发"}}})
			if err != nil {
				t.Fatal(err)
			}
			fought := read()
			if fought.Actor.Battle.Status == "active" || rounds == 0 {
				t.Fatalf("battle did not settle: %+v", fought.Actor.Battle)
			}
			t.Logf("normal-resource battle ended %s after %d rounds", fought.Actor.Battle.Status, rounds)
			fork, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "Before recovery"})
			if err != nil {
				t.Fatal(err)
			}
			action(interactive.AdventureAction{Action: "rest", Hours: 12})
			rested := read()
			if !reflect.DeepEqual(rested.Actor.Assets, fought.Actor.Assets) {
				t.Fatal("personal rest repaired or refuelled mechanical assets")
			}
			branchBefore, err := store.AdventureWindow(story.ID, fork.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(branchBefore.Actor, fought.Actor) {
				t.Fatal("recovery leaked across branch")
			}
			if rested.Actor.Health < rested.Actor.MaxHealth || rested.Actor.Injury != 0 {
				t.Fatal("normal rest did not recover protagonist")
			}

			if packID == "lanchuan-qunxia" {
				// A03 boat-work earnings buy both medicines before a normal
				// registered journey enters the source's poisonous fog area.
				journey("lanchuan-l01")
				action(interactive.AdventureAction{Action: "buy", MarketID: "lanchuan-market-l01", ItemID: "lanchuan-i20"})
				action(interactive.AdventureAction{Action: "buy", MarketID: "lanchuan-market-l01", ItemID: "lanchuan-i21"})
				for _, destination := range []string{"lanchuan-l11", "lanchuan-l06", "lanchuan-l07", "lanchuan-l21", "lanchuan-l28", "lanchuan-l29", "lanchuan-l30"} {
					journey(destination)
				}
				exposed := read()
				if exposed.Actor.FogAreaID != "lanchuan-mist-ravine-fog" || exposed.Actor.FogTreated || exposed.Actor.Poison != 0 || exposed.Actor.Health != rested.Actor.Health {
					t.Fatal("actual arrival did not register a non-damaging source fog exposure")
				}
				if err = store.Close(); err != nil {
					t.Fatal(err)
				}
				store = interactive.NewStore(root)
				service.app.interactive = store
				if !reflect.DeepEqual(read().Actor, exposed.Actor) {
					t.Fatal("opening/replaying the fog visit reset it")
				}
				action(interactive.AdventureAction{Action: "use", ItemID: "lanchuan-i20"})
				if read().Actor.FogTreated {
					t.Fatal("ordinary poison cure substituted for the source warding medicine")
				}
				action(interactive.AdventureAction{Action: "use", ItemID: "lanchuan-i21"})
				if !read().Actor.FogTreated || read().Actor.Inventory["lanchuan-i21"] != 0 {
					t.Fatal("warding medicine was not actually consumed")
				}
				journey("lanchuan-l29")
				if read().Actor.FogAreaID != "" || read().Actor.FogTreated {
					t.Fatal("normal departure did not clear source fog exposure")
				}
				journey("lanchuan-l30")
				if read().Actor.FogAreaID == "" || read().Actor.FogTreated {
					t.Fatal("a later visit inherited the prior visit's antidote")
				}
				journey("lanchuan-l29")
				rested = read()
			}

			if packID == "xinghai-689" {
				damaged := rested.Actor.Assets["xinghai-i09"]
				if damaged.Health >= damaged.MaxHealth || damaged.Fuel >= 100 {
					t.Fatal("mechanical fight consumed neither hull nor fuel")
				}
				journey("xinghai-l23")
				journey("xinghai-l22")
				action(interactive.AdventureAction{Action: "sell", MarketID: "xinghai-market-l22", ItemID: "xinghai-i07", Quantity: 1})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i33", Quantity: 8})
				action(interactive.AdventureAction{Action: "repair", SkillID: "xinghai-s03", TargetID: "xinghai-i09", Hours: 8})
				repaired := read().Actor.Assets["xinghai-i09"]
				if repaired.Health != repaired.MaxHealth || repaired.Fuel != 100 {
					t.Fatal("paid repair did not restore asset")
				}
				action(interactive.AdventureAction{Action: "withdraw", ItemID: "xinghai-i09"})
				action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-i09"})
				if read().Actor.Assets["xinghai-i09"] != repaired {
					t.Fatal("redeploy changed asset durability or fuel")
				}
				// Late-game ship chain: real source lessons and bounded practice, no state injection.
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s01", Hours: 216})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s02", Hours: 216})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s03", Hours: 216})
				journey("xinghai-l21")
				journey("xinghai-l04")
				journey("xinghai-l03")
				choose("xinghai-training-s04", "xinghai-training-s04.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s04", Hours: 4})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s04", Hours: 216})
				choose("xinghai-training-s05", "xinghai-training-s05.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s05", Hours: 4})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s05", Hours: 216})
				journey("xinghai-l04")
				journey("xinghai-l21")
				journey("xinghai-l22")
				journey("xinghai-l23")
				choose("xinghai-a06", "xinghai-a06.c2")
				journey("xinghai-l22")
				choose("xinghai-training-s28", "xinghai-training-s28.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s28", Hours: 4})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i19"})
				journey("xinghai-l21")
				// A qualified breaching job funds the listening rig; no cash or
				// learning credential is injected into the original worker.
				choose("xinghai-a18", "xinghai-a18.c1")
				choose("xinghai-training-s29", "xinghai-training-s29.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s29", Hours: 96})
				cashBefore := read().Actor.Currency
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l21", ItemID: "xinghai-signal-processor"})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l21", ItemID: "xinghai-hopping-recording", Quantity: 2})
				if read().Actor.Currency != cashBefore-1320 {
					t.Fatal("listening equipment was not bought with earned money")
				}
				action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-signal-processor"})
				signalBefore := read()
				contactsBefore, e := store.CharacterState(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				preSignal, e := store.Snapshot(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				signalFork, e := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: preSignal.CurrentTurn.ID, Title: "Before listening"})
				if e != nil {
					t.Fatal(e)
				}
				signalInput := interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-decode-hopping", Hours: 4}
				signalRequest := interactive.AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &signalBefore.Head, User: "监听四小时，并解算普通跳频录音", Narrative: "嘶声中只辨认出几句残话。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: "监听四小时", Adventure: &signalInput}}}
				if _, _, e = store.AppendTurnWithState(story.ID, signalRequest); e != nil {
					t.Fatal(e)
				}
				signalAfter := read()
				englishView, e := service.app.AdventureWindow(story.ID, "main", "en-US")
				if e != nil {
					t.Fatal(e)
				}
				for _, item := range englishView.Items {
					if item.ID == "xinghai-signal-processor" || item.ID == "xinghai-hopping-recording" || item.ID == "xinghai-signal-fragments" {
						if item.Name != pack.Localizations["en-US"].Adventure[item.ID] {
							t.Fatalf("English processing item not localized: %+v", item)
						}
					}
				}

				contactsAfter, e := store.CharacterState(story.ID, "main")
				if e != nil || !reflect.DeepEqual(contactsBefore, contactsAfter) {
					t.Fatalf("signal fragments granted a contact or channel: %v", e)
				}
				postSignal, e := store.Snapshot(story.ID, "main")
				if e != nil {
					t.Fatal(e)
				}
				if signalAfter.Actor.Inventory["xinghai-signal-fragments"] != 1 || signalAfter.Actor.Inventory["xinghai-hopping-recording"] != 1 || signalAfter.Actor.Assets["xinghai-signal-processor"].Fuel != 90 || postSignal.Clock.Seconds-preSignal.Clock.Seconds != 14400 {
					t.Fatal("formal listening did not consume its input, fuel and four hours")
				}
				if _, _, e = store.AppendTurnWithState(story.ID, signalRequest); e == nil {
					t.Fatal("stale retry was allowed to process twice")
				}
				if !reflect.DeepEqual(signalAfter, read()) {
					t.Fatal("stale retry changed listening results")
				}
				branchSignal, e := store.AdventureWindow(story.ID, signalFork.ID)
				if e != nil || !reflect.DeepEqual(branchSignal.Actor, signalBefore.Actor) {
					t.Fatalf("listening changed the earlier branch: %v", e)
				}
				if e = store.Close(); e != nil {
					t.Fatal(e)
				}
				store = interactive.NewStore(root)
				service.app.interactive = store
				if !reflect.DeepEqual(signalAfter, read()) {
					t.Fatal("cold replay changed listening results")
				}
				action(interactive.AdventureAction{Action: "withdraw", ItemID: "xinghai-signal-processor"})
				// Learn before accepting the one-hour A13 emergency. The
				// unchanged source qualification requires A06, not A13 itself.
				journey("xinghai-l39")
				choose("xinghai-training-s21", "xinghai-training-s21.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s21", Hours: 96})
				choose("xinghai-a13", "xinghai-a13.c3")
				wish := read()
				if wish.Actor.Inventory["xinghai-i55"] != 1 {
					t.Fatal("actual S21 qualification did not earn the source wish stone")
				}
				// A source reward is not an unconditional health potion.
				restore := interactive.AdventureAction{Action: "use", ItemID: "xinghai-i55"}
				if _, _, e = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "使用祈愿石子", Narrative: "不应凭空疗伤。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: "使用祈愿石子", Adventure: &restore}}}); e == nil {
					t.Fatal("uncorrupted actor used a free mental recovery charge")
				}
				if !reflect.DeepEqual(read(), wish) {
					t.Fatal("refused wish-stone use changed resources or history")
				}
				journey("xinghai-l21")
				journey("xinghai-l04")
				journey("xinghai-l11")
				journey("xinghai-l14")
				choose("xinghai-training-s23", "xinghai-training-s23.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s23", Hours: 96})
				journey("xinghai-l11")
				journey("xinghai-l04")
				journey("xinghai-l03")
				journey("xinghai-l01")
				journey("xinghai-l02")
				choose("xinghai-training-s06", "xinghai-training-s06.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s06", Hours: 96})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s06", Hours: 192})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: "xinghai-i11"})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: "xinghai-i38"})
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l02", ItemID: "xinghai-weapon-cell", Quantity: 3})
				action(interactive.AdventureAction{Action: "equip", ItemID: "xinghai-i11"})
				journey("xinghai-l01")
				journey("xinghai-l03")
				shieldBefore := read()
				action(interactive.AdventureAction{Action: "battle_start", OpponentID: "xinghai-sparring", Scale: "person"})
				action(interactive.AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s06"})
				shieldAfter := read()
				if shieldAfter.Actor.Inventory["xinghai-i38"] != shieldBefore.Actor.Inventory["xinghai-i38"]-1 || shieldAfter.Actor.Inventory["xinghai-weapon-cell"] != shieldBefore.Actor.Inventory["xinghai-weapon-cell"]-3 || shieldAfter.Actor.ItemCharges["xinghai-i11"] != 2 || shieldAfter.Actor.ItemReadyAt["xinghai-i11"] != shieldAfter.ClockSeconds+120 {
					t.Fatal("normally bought and equipped shield boost did not pay source costs and force cooldown")
				}
				for rounds := 0; read().Actor.Battle.Status == "active" && rounds < 10; rounds++ {
					action(interactive.AdventureAction{Action: "battle_round", Choice: "attack"})
				}
				if read().Actor.Battle.Status == "active" {
					t.Fatal("source laser calibration did not settle")
				}
				choose("xinghai-training-s07", "xinghai-training-s07.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s07", Hours: 96})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s07", Hours: 192})
				journey("xinghai-l04")
				journey("xinghai-l16")
				journey("xinghai-l17")
				choose("xinghai-training-s08", "xinghai-training-s08.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s08", Hours: 96})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s08", Hours: 192})
				journey("xinghai-l16")
				journey("xinghai-l04")
				journey("xinghai-l21")
				journey("xinghai-l25")
				choose("xinghai-training-s09", "xinghai-training-s09.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s09", Hours: 96})
				action(interactive.AdventureAction{Action: "train", SkillID: "xinghai-s09", Hours: 192})
				choose("xinghai-training-s32", "xinghai-training-s32.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s32", Hours: 4})
				if read().Actor.Rank < 7 {
					t.Fatal("normal study did not reach authored shipbuilding qualification")
				}
				journey("xinghai-l21")
				journey("xinghai-l22")
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i12", Quantity: 1})
				journey("xinghai-l23")
				journey("xinghai-l38")
				action(interactive.AdventureAction{Action: "battle_start", OpponentID: "xinghai-electronic-node", Scale: "person"})
				plasmaBefore := read()
				action(interactive.AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s23"})
				plasmaAfter := read()
				if plasmaAfter.Actor.Assets["xinghai-i09"].Health != plasmaBefore.Actor.Assets["xinghai-i09"].Health || plasmaAfter.Actor.Energy != plasmaBefore.Actor.Energy-9 || plasmaAfter.Actor.Health >= plasmaBefore.Actor.Health {
					t.Fatal("earned plasma did not trade energy/heat for a real nearby node interruption")
				}
				for rounds := 0; read().Actor.Battle.Status == "active" && rounds < 20; rounds++ {
					action(interactive.AdventureAction{Action: "battle_round", Choice: "escape"})
				}
				if read().Actor.Battle.Status == "active" {
					t.Fatal("plasma encounter failed to settle honestly")
				}
				choose("xinghai-a11", "xinghai-a11.c3")
				choose("xinghai-training-s10", "xinghai-training-s10.qualify")
				action(interactive.AdventureAction{Action: "learn", SkillID: "xinghai-s10", Hours: 168})
				journey("xinghai-l23")
				journey("xinghai-l22")
				journey("xinghai-l21")
				journey("xinghai-l22")
				for id, n := range map[string]int{"xinghai-i33": 30, "xinghai-i34": 20, "xinghai-i31": 10, "xinghai-i39": 10} {
					action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: id, Quantity: n})
				}
				action(interactive.AdventureAction{Action: "craft", RecipeID: "xinghai-assemble-frigate", Hours: 120})
				action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"})
				action(interactive.AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-frigate", Scale: "ship"})
				for rounds := 0; read().Actor.Battle.Status == "active" && rounds < 100; rounds++ {
					action(interactive.AdventureAction{Action: "battle_round", Choice: "attack"})
				}
				if read().Actor.Battle.Status == "active" {
					t.Fatal("ship battle did not settle")
				}
				ship := read().Actor.Assets["xinghai-frigate"]
				if ship.Fuel >= 100 {
					t.Fatal("ship battle did not consume fuel")
				}
				action(interactive.AdventureAction{Action: "buy", MarketID: "xinghai-market-l22", ItemID: "xinghai-i33", Quantity: 8})
				journey("xinghai-l21")
				journey("xinghai-l04")
				journey("xinghai-l03")
				action(interactive.AdventureAction{Action: "repair", MarketID: "xinghai-dock-l03", TargetID: "xinghai-frigate", Hours: 8})
				ship = read().Actor.Assets["xinghai-frigate"]
				if ship.Health != ship.MaxHealth || ship.Fuel != 100 {
					t.Fatal("paid ship repair and resupply incomplete")
				}
				action(interactive.AdventureAction{Action: "withdraw", ItemID: "xinghai-frigate"})
				action(interactive.AdventureAction{Action: "deploy", ItemID: "xinghai-frigate"})
				journey("xinghai-l01")
				careerBefore := read()
				choose("xinghai-q03", "xinghai-q03.c1")
				careerAfter := read()
				if !reflect.DeepEqual(careerBefore.Actor.Inventory, careerAfter.Actor.Inventory) || careerBefore.Actor.Currency != careerAfter.Actor.Currency {
					t.Fatal("professional ship check fabricated an asset, credential or debt payment")
				}
				action(interactive.AdventureAction{Action: "rest", Hours: 1})
				t.Logf("earned ship chain completed at rank %d, money %d", read().Actor.Rank, read().Actor.Currency)
				rested = read()
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			store = interactive.NewStore(root)
			service.app.interactive = store
			restored := read()
			branchAfter, err := store.AdventureWindow(story.ID, fork.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.Actor, rested.Actor) || !reflect.DeepEqual(branchAfter.Actor, branchBefore.Actor) || !reflect.DeepEqual(restored.Rules, initial.Rules) {
				t.Fatal("cold reopen changed earned state, branch or frozen rules")
			}
		})
	}
}
