package interactive

import (
	"reflect"
	"ta/internal/character"
	"ta/internal/quest"
	"testing"
)

func TestFantasySealHasPaidMentalPressureBeyondFrost(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-learner")
	a.Rank = 8
	a.Skills["sg-frost"] = 1
	a.Skills["sg-seal"] = 1
	started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-op19"}, "seal-start")
	if err != nil {
		t.Fatal(err)
	}
	frost, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-frost"}, "frost")
	if err != nil {
		t.Fatal(err)
	}
	seal, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-seal"}, "seal")
	if err != nil {
		t.Fatal(err)
	}
	if frost.Battle.Health-seal.Battle.Health != 14 || started.Energy-seal.Energy != 16 || started.Inventory["sg-charcoal"]-seal.Inventory["sg-charcoal"] != 1 || seal.Battle.EnemyControlRecovery != frost.Battle.EnemyControlRecovery {
		t.Fatal("advanced sigil lacks its distinct paid mental effect")
	}
	for _, target := range []string{"sg-practice", "sg-crypt-warden", "sg-drowned-keeper"} {
		mindless, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: target}, "mindless")
		if e != nil {
			t.Fatal(e)
		}
		rejected, _, e := ResolveAdventureAction(r, mindless, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-seal"}, "no-mind")
		if e == nil || !reflect.DeepEqual(mindless, rejected) {
			t.Fatal("sigil affected a mindless target or spent rejected resources", target)
		}
		if _, _, e = ResolveAdventureAction(r, mindless, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-frost"}, "physical-control"); e != nil {
			t.Fatal("frost lost its distinct physical-control use", e)
		}
	}
}

func TestFantasyResolveCountersAnActualCopyistTechnique(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-carer")
	a.Rank = 11
	a.Skills["sg-resolve"] = 1
	started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "sg-op19"}, "copyist")
	if err != nil {
		t.Fatal(err)
	}
	legal := false
	for _, c := range adventureEnemyDecisionCandidates(r, &started) {
		legal = legal || c.ID == "skill:sg-seal"
	}
	if !legal {
		t.Fatal("actual copyist cannot use the mental technique")
	}
	plain, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "guard"}, "ordinary-guard")
	if err != nil {
		t.Fatal(err)
	}
	warded, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "sg-resolve"}, "real-ward")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Battle.EnemyLastSkillID != "sg-seal" || warded.Battle.EnemyLastSkillID != "sg-seal" || warded.Health-plain.Health != 18 {
		t.Fatalf("real default enemy/45-percent ward did not interact: plain=%d warded=%d skills=%s/%s", plain.Health, warded.Health, plain.Battle.EnemyLastSkillID, warded.Battle.EnemyLastSkillID)
	}
	if started.Battle.EnemyEnergy-warded.Battle.EnemyEnergy != 16 || started.Battle.EnemyInventory["sg-charcoal"]-warded.Battle.EnemyInventory["sg-charcoal"] != 1 || started.Energy-warded.Energy != 11 || started.Inventory["sg-salt"]-warded.Inventory["sg-salt"] != 1 {
		t.Fatal("mental offense or defense ignored its actual costs")
	}
	if warded.MentalCorruption != 0 || warded.MentalShock != 0 || warded.Control != plain.Control {
		t.Fatal("mental damage invented corruption or changed the separate control effect")
	}
	t.Logf("ordinary_guard_damage=%d mental_ward_damage=%d", started.Health-plain.Health, started.Health-warded.Health)
}

func TestFantasyAdvancedNamedPeopleOwnHighRankLoadouts(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-road")
	for _, tc := range []struct {
		source        string
		rank          int
		weapon, armor string
	}{{"sg-p07", 28, "sg-thorn-sword", "sg-thorn-mail"}, {"sg-p56", 30, "sg-focus-staff", "sg-rune-coat"}} {
		t.Run(tc.source, func(t *testing.T) {
			var npc AdventureActor
			for _, n := range r.NPCs {
				if n.CharacterSourceKey == tc.source {
					npc = n.Actor
				}
			}
			if npc.Rank != tc.rank || npc.Equipment["person:weapon"] != tc.weapon || npc.Equipment["person:armor"] != tc.armor || npc.Inventory[tc.weapon] != 1 || npc.Inventory[tc.armor] != 1 {
				t.Fatal("high-rank counterpart is missing actual qualified equipment")
			}
			if err := ValidateAdventureActor(r, npc); err != nil {
				t.Fatal(err)
			}
			for _, o := range r.Opponents {
				if o.CharacterSourceKey == tc.source && (!o.RequiresPresence || o.Lethal || o.Rank != npc.Rank || o.Health != npc.MaxHealth) {
					t.Fatal("encounter diverged from the actual optional nonlethal person")
				}
			}
		})
	}
}

// Qualified novice/proficient fixtures isolate authored difficulty. No random
// roll, global difficulty inflation or free player state upgrade is introduced.
func TestFantasyAdvancedWorkHasNoviceFailureAndPreparedSuccess(t *testing.T) {
	for _, tc := range []struct{ task, choice string }{{"sg-q22", "sg-q22.c2"}, {"sg-q23", "sg-q23.c3"}, {"sg-q24", "sg-q24.c1"}} {
		t.Run(tc.choice, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-road")
			d, _ := quest.DefinitionByID(tc.task)
			c, _ := d.Choice(tc.choice)
			var method AdventureSkill
			for _, s := range r.Skills {
				if s.ID == c.SkillID {
					method = s
				}
			}
			a.Rank = method.RequiredRank
			a.Skills[method.ID] = 1
			if method.Prerequisite != "" {
				a.Skills[method.Prerequisite] = 1
			}
			if method.RequiredTool != "" {
				a.Inventory[method.RequiredTool] = 1
			}
			for id, n := range c.ItemCosts {
				a.Inventory[id] = n
			}
			p := questEffortProjectionIter4(a)
			setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), c.LocationIDs[0])
			present := []any{}
			p.Characters = &character.State{}
			for _, source := range c.RequiredCharacterSourceKeys {
				p.Characters.Profiles = append(p.Characters.Profiles, character.Profile{ID: source, StoryID: "fixture", SourceKey: source, Name: source})
				present = append(present, source)
			}
			setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), present)
			prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
			failed, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: c.ID})
			if err != nil {
				t.Fatal(err)
			}
			if failed.Instance.Choices[c.ObjectiveID].Outcome != "failure" {
				t.Fatal("advanced novice still bypasses the authored failure")
			}
			trained, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "train", SkillID: method.ID, Hours: method.TrainingHours * 2}, "prepare")
			if err != nil {
				t.Fatal(err)
			}
			for _, op := range adventureActorOps(trained, "") {
				applyActorStateOp(p.State, op)
			}
			passed, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: c.ID})
			if err != nil || passed.Instance.Choices[c.ObjectiveID].Outcome != "success" {
				t.Fatal("real mastery preparation did not reach the authored success", err)
			}
		})
	}
}

// The actual learned and equipped named person enters only after being present.
// The player is a qualified fixture, not a claim of naturally earned rank 30.
func TestFantasyAdvancedChallengesCommitAndColdRestoreActualPeople(t *testing.T) {
	for _, tc := range []struct {
		source, opponent, place string
		rank                    int
	}{{"sg-p07", "sg-encounter-p07", "断旗堡", 28}, {"sg-p56", "sg-encounter-p56", "第六观星塔", 30}} {
		t.Run(tc.source, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-road")
			a.Rank = 30
			a.Health = 680
			a.MaxHealth = 680
			a.Energy = 390
			a.MaxEnergy = 390
			s, id := fantasyBoundStory(t, r, a, tc.place)
			defer func() { s.Close() }()
			action := func(in AdventureAction) error {
				_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "当面约定非致命练习并防守这一轮", Narrative: "双方按约定处理这一轮练习。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "当面约定非致命练习并防守这一轮", Adventure: &in}}})
				return err
			}
			if err := action(AdventureAction{Action: "battle_start", OpponentID: tc.opponent}); err == nil {
				t.Fatal("high-rank person spawned without being present")
			}
			fantasyMeet(t, s, id, "main", tc.source, "advanced-person")
			snap, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: snap.CurrentTurn.ID, Title: "Without sparring"})
			if err != nil {
				t.Fatal(err)
			}
			if err = action(AdventureAction{Action: "battle_start", OpponentID: tc.opponent}); err != nil {
				t.Fatal(err)
			}
			before, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if before.Actor.Battle.EnemyActorID != "advanced-person" || before.Actor.Battle.EnemyRank != tc.rank || before.Actor.Battle.Lethal {
				t.Fatal("bound challenge did not use the actual optional person")
			}
			if _, err = s.CommitAdventureAction(id, "main", "advanced-guard", before.Head, AdventureAction{Action: "battle_round", Choice: "guard"}); err != nil {
				t.Fatal(err)
			}
			after, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			b := after.Actor.Battle
			if b.EnemyLastSkillID == "" || b.EnemyEnergy >= before.Actor.Battle.EnemyEnergy || after.Actor.Health >= before.Actor.Health {
				t.Fatal("high-rank challenge had no real paid response")
			}
			state, err := s.Snapshot(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			npc, err := AdventureActorFromState(state.State, "advanced-person")
			if err != nil || npc.Energy != b.EnemyEnergy {
				t.Fatal("battle payment did not reach the actual person", err)
			}
			root := s.Root()
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			s = NewStore(root)
			cold, err := s.AdventureWindow(id, "main")
			if err != nil || !reflect.DeepEqual(cold.Actor, after.Actor) {
				t.Fatal("cold restore lost the high-rank exchange", err)
			}
			old, err := s.AdventureWindow(id, fork.ID)
			if err != nil || old.Actor.Battle != nil {
				t.Fatal("branch inherited an unplayed challenge", err)
			}
			t.Logf("source=%s rank=%d method=%s player_damage=%d enemy_effort=%d", tc.source, tc.rank, b.EnemyLastSkillID, before.Actor.Health-after.Actor.Health, before.Actor.Battle.EnemyEnergy-b.EnemyEnergy)
		})
	}
}

func TestFantasyBeaconFailureMustBePreparedBeforeActualRetry(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	a.Rank = 18
	a.Skills["sg-beaconwork"] = 1
	a.Inventory["sg-focus"] = 1
	a.Inventory["sg-crystal"] = 6
	a.Inventory["sg-silver-wire"] = 4
	s, id := fantasyBoundStory(t, r, a, "最后古灯塔")
	defer func() { s.Close() }()
	fantasyMeet(t, s, id, "main", "sg-p38", "keeper")
	questApplyOn(t, s, id, "main", "sg-q24", QuestActionAccept, "", "beacon-accept")
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	first := questApplyOn(t, s, id, "main", "sg-q24", QuestActionChoose, "sg-q24.c1", "beacon-fail")
	state, err := s.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	initial := questTestTask(t, state, "sg-q24").Resolutions["sg-q24.resolve"]
	after, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if initial.Outcome != "failure" || after.Actor.Currency != before.Actor.Currency || after.Actor.Inventory["sg-crystal"] != 3 || after.Actor.Inventory["sg-silver-wire"] != 2 || after.Actor.Energy != before.Actor.Energy-12 || after.ClockSeconds-before.ClockSeconds != 3600 {
		t.Fatal("novice failure skipped actual costs or awarded success")
	}
	replay := questApplyOn(t, s, id, "main", "sg-q24", QuestActionChoose, "sg-q24.c1", "beacon-fail")
	if !replay.Replayed || replay.EventID != first.EventID {
		t.Fatal("failed command rerolled or spent twice")
	}
	if _, err = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "sg-q24", Action: QuestActionChoose, ChoiceID: "sg-q24.c1", CommandID: "beacon-early", CommandHash: "beacon-early"}); err == nil {
		t.Fatal("early retry skipped the unresolved consequence")
	}
	questWait(t, s, id, "main", 21600)
	// Use the already established real keeper, without registering a duplicate.
	if _, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "问守灯人上次试作留下的问题", Narrative: "守灯人说明旧导纹仍不稳定，需要更熟练地对齐后再试。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "sg-q24", QuestAction: QuestActionFollowUp, QuestChoiceID: "sg-q24.c1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "sg-q24", Action: QuestActionChoose, ChoiceID: "sg-q24.c1", CommandID: "beacon-unprepared", CommandHash: "beacon-unprepared"}); err == nil {
		t.Fatal("unchanged preparation rerolled after consequence")
	}
	snap, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: snap.CurrentTurn.ID, Title: "Keep the unresolved attempt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "花四十八小时练习已学古灯修护", Narrative: "你按已有工法反复练习对齐导纹。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "花四十八小时练习已学古灯修护", Adventure: &AdventureAction{Action: "train", SkillID: "sg-beaconwork", Hours: 48}}}}); err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "sg-q24", QuestActionChoose, "sg-q24.c1", "beacon-repaired")
	result, err := s.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	resolution := questTestTask(t, result, "sg-q24").Resolutions["sg-q24.resolve"]
	if resolution.Outcome != "success" || resolution.Previous == nil || resolution.Previous.Outcome != "failure" || resolution.Previous.EchoAcknowledgedBy != "keeper" {
		t.Fatal("prepared retry lost the actual failure and keeper response")
	}
	final, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if final.Actor.Skills["sg-beaconwork"] != 2 || final.Actor.Inventory["sg-crystal"] != 0 || final.Actor.Inventory["sg-silver-wire"] != 0 || final.Actor.Currency != a.Currency+12 || final.ClockSeconds != 56*3600 {
		t.Fatalf("retry did not pay the real preparation and second attempt: clock=%d", final.ClockSeconds)
	}
	root := s.Root()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	for _, branch := range []string{"main", fork.ID} {
		got, e := s.QuestStateFor(id, branch)
		if e != nil {
			t.Fatal(e)
		}
		outcome := questTestTask(t, got, "sg-q24").Resolutions["sg-q24.resolve"].Outcome
		want := "success"
		if branch != "main" {
			want = "failure"
		}
		if outcome != want {
			t.Fatal("cold branch rewrote the real consequence", branch, outcome)
		}
	}
}

func TestFantasyMentalExtensionKeepsPaidPersonAndCorruptionBoundaries(t *testing.T) {
	for _, mode := range []string{"allowed", "free", "ship", "wuxia", "corruption"} {
		t.Run(mode, func(t *testing.T) {
			r, _ := mentalAttackFixture(t)
			r.Family = "fantasy"
			switch mode {
			case "free":
				r.Skills[len(r.Skills)-3].Cost = 0
			case "ship":
				r.Skills[len(r.Skills)-3].Scale = "ship"
			case "wuxia":
				r.Family = "wuxia"
			case "corruption":
				r.Opponents[0].MentalAttack = true
				r.Opponents[0].MentalCorruptionPower = 10
			}
			err := r.Validate()
			if (mode == "allowed") != (err == nil) {
				t.Fatalf("unexpected mental boundary result for %s: %v", mode, err)
			}
		})
	}
	r, _ := fantasyOrigin(t, "sg-start-learner")
	for _, skill := range r.Skills {
		if skill.ID == "sg-seal" {
			if _, ok := adventurePlainSkillEffects(skill); ok {
				t.Fatal("mental composite entered unrestricted cheap recomposition")
			}
		}
	}
}
