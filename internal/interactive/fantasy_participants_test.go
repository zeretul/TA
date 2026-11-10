package interactive

import (
	"encoding/json"
	"reflect"
	"ta/internal/character"
	"ta/internal/quest"
	"testing"
)

func TestFantasyChoiceParticipantsRejectInvalidIdentityAtomically(t *testing.T) {
	for _, bad := range []string{"absent", "wrong-source", "elsewhere", "dead", "occupied", "duplicate", "archived"} {
		t.Run(bad, func(t *testing.T) {
			r, a := fantasyOrigin(t, "sg-start-smith")
			p := questEffortProjectionIter4(a)
			setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), "倒旗墓道")
			p.Characters = &character.State{Profiles: []character.Profile{{ID: "keeper", StoryID: "fixture", SourceKey: "sg-p12", Name: "乔文"}}}
			setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{"keeper"})
			switch bad {
			case "absent":
				p.Characters.Profiles = nil
			case "wrong-source":
				p.Characters.Profiles[0].SourceKey = "sg-p13"
			case "elsewhere":
				setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{})
			case "dead":
				setPath(p.State, actorStateFieldPath("keeper", xiuxianFieldHealth), 0)
			case "occupied":
				setPath(p.State, actorStateFieldPath("keeper", xiuxianFieldArrangement), xiuxianArrangementOccupied)
			case "archived":
				setPath(p.State, actorArchiveStatePath("keeper"), map[string]any{"reason": "left"})
			case "duplicate":
				p.Characters.Profiles = append(p.Characters.Profiles, character.Profile{ID: "impostor", StoryID: "fixture", SourceKey: "sg-p12"})
			}
			d, ok := quest.DefinitionByID("sg-crypt-inquiry")
			if !ok {
				t.Fatal("missing quest")
			}
			prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
			before, _ := json.Marshal(p)
			_, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, prior, true, QuestRequest{ChoiceID: d.ID + ".stage1.c2"})
			after, _ := json.Marshal(p)
			if err == nil || !reflect.DeepEqual(before, after) {
				t.Fatal("invalid participant consumed or fabricated action", err)
			}
		})
	}
}

func TestFantasyCampRepairUsesActualToolsIronEffortAndPayment(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-smith")
	s, id := fantasyBoundStory(t, r, a, "冷锤工坊")
	defer s.Close()
	fantasyMeet(t, s, id, "main", "sg-p23", "smith")
	_, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "买两份铁料修件", Narrative: "按柜台标价购入铁料。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "买两份铁料修件", Adventure: &AdventureAction{Action: "buy", MarketID: "sg-market-23", ItemID: "sg-iron", Quantity: 2}}}})
	bought, readErr := s.AdventureWindow(id, "main")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "sg-q18", QuestActionAccept, "", "take-real-repair")
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "sg-q18", QuestActionChoose, "sg-q18.c2", "do-real-repair")
	after, err := s.AdventureWindow(id, "main")
	if err != nil || after.Actor.Inventory["sg-iron"] != bought.Actor.Inventory["sg-iron"]-2 || after.Actor.Inventory["sg-forge-tools"] != 1 || after.Actor.Energy != before.Actor.Energy-5 || after.Actor.Currency != before.Actor.Currency+24 || after.ClockSeconds != before.ClockSeconds+3600 {
		t.Fatalf("repair did not settle real owned resources: %+v %v", after, err)
	}
}

func TestFantasyNamedSparBindsActualNPCAndRejectsAbsent(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	s, id := fantasyBoundStory(t, r, a, "白棘演武场")
	defer s.Close()
	window, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	input := AdventureAction{Action: "battle_start", OpponentID: "sg-encounter-p08"}
	act := func() error {
		_, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "请教头当面作非致命练习", Narrative: "双方确认非致命练习并摆开架势。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "请教头当面作非致命练习", Adventure: &input}}})
		return e
	}
	if err = act(); err == nil {
		t.Fatal("absent person spawned through combat")
	}
	after, err := s.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(window.Actor, after.Actor) || window.ClockSeconds != after.ClockSeconds {
		t.Fatal("absent combat consumed state", err)
	}
	fantasyMeet(t, s, id, "main", "sg-p08", "instructor")
	window, err = s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	err = act()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	b := result.Actor.Battle
	if b == nil || b.CharacterSourceKey != "sg-p08" || b.EnemyActorID != "instructor" || b.Lethal || b.EnemyRank != 23 || b.EnemyInventory["sg-travel-blade"] != 1 || b.EnemyEquipment["person:weapon"] != "sg-travel-blade" {
		t.Fatalf("named combat lost actual source or loadout: %+v", b)
	}
}

func TestFantasyAllNamedCounterpartsUseTheirActualLoadouts(t *testing.T) {
	r, _ := fantasyOrigin(t, "sg-start-road")
	count := 0
	for _, op := range r.Opponents {
		if !op.RequiresPresence {
			continue
		}
		count++
		t.Run(op.ID, func(t *testing.T) {
			var npc AdventureActor
			for _, n := range r.NPCs {
				if n.CharacterSourceKey == op.CharacterSourceKey {
					npc = n.Actor
				}
			}
			if npc.Rank != op.Rank || npc.MaxHealth != op.Health || npc.MaxEnergy != op.Energy {
				t.Fatal("stale authored encounter")
			}
			a := NewAdventureActor(r)
			a.Rank = npc.Rank
			a.MaxHealth = 10000
			a.Health = 10000
			a.MaxEnergy = 1000
			a.Energy = 1000
			started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "named-fixture")
			if err != nil {
				t.Fatal(err)
			}
			bindAdventureEncounter(started.Battle, "established-person", npc, r)
			if !reflect.DeepEqual(started.Battle.EnemySkillRanks, npc.Skills) || !reflect.DeepEqual(started.Battle.EnemyEquipment, npc.Equipment) {
				t.Fatal("binding lost actual skills or loadout")
			}
			next, _, err := ResolveAdventureAction(r, started, AdventureAction{Action: "battle_round", Choice: "attack"}, "named-round")
			if err != nil {
				t.Fatal(err)
			}
			b := next.Battle
			if b.EnemyLastSkillID == "" || npc.Skills[b.EnemyLastSkillID] == 0 {
				t.Fatal("default opponent solver did not use a learned method", b.EnemyLastSkillID)
			}
			t.Logf("source=%s rank=%d method=%s mastery=%d effort_spent=%d player_damage=%d opponent_damage=%d", op.CharacterSourceKey, npc.Rank, b.EnemyLastSkillID, npc.Skills[b.EnemyLastSkillID], npc.Energy-b.EnemyEnergy, a.Health-next.Health, npc.Health-b.Health)
		})
	}
	if count != 19 {
		t.Fatal("unexpected named encounter count", count)
	}
}

// The observer is a controlled qualified fixture; no keeper is present and no
// later reaction acknowledgment can retroactively authorize an immediate wage.
func TestFantasyIndependentBeaconObservationHasNoRemotePayment(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-learner")
	a.Rank = 10
	a.Skills["sg-lore"] = 1
	s, id := fantasyBoundStory(t, r, a, "最后古灯塔")
	defer s.Close()
	questApplyOn(t, s, id, "main", "sg-q24", QuestActionAccept, "", "observe-alone-accept")
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "sg-q24", QuestActionChoose, "sg-q24.c2", "observe-alone-choose")
	after, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Actor.Currency != before.Actor.Currency || after.Actor.Experience != before.Actor.Experience+12 || after.Actor.Energy != before.Actor.Energy-5 || after.ClockSeconds != before.ClockSeconds+1800 {
		t.Fatalf("independent observation fabricated payment or lost its real cost: before=%+v after=%+v", before, after)
	}
}

func TestFantasySharedMapNeedsActualSurveyParticipant(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-scout")
	a.Rank = 10
	a.Skills["sg-survey"] = 1
	a.Inventory["sg-survey-kit"] = 1
	s, id := fantasyBoundStory(t, r, a, "磨石渡")
	defer s.Close()
	questApplyOn(t, s, id, "main", "sg-q04", QuestActionAccept, "", "map-accept")
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "sg-q04", Action: QuestActionChoose, ChoiceID: "sg-q04.c1", CommandID: "map-absent", CommandHash: ContentHashOf("map-absent")})
	if err == nil {
		t.Fatal("absent surveyor gave a shared map")
	}
	after, err := s.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(before.Actor, after.Actor) || before.ClockSeconds != after.ClockSeconds {
		t.Fatal("absent map handover changed state", err)
	}
	fantasyMeet(t, s, id, "main", "sg-p04", "surveyor")
	questApplyOn(t, s, id, "main", "sg-q04", QuestActionChoose, "sg-q04.c1", "map-present")
	after, err = s.AdventureWindow(id, "main")
	if err != nil || after.Actor.Inventory["sg-shared-map"] != before.Actor.Inventory["sg-shared-map"]+1 {
		t.Fatal("present surveyor did not hand over the actual map", err)
	}
}
