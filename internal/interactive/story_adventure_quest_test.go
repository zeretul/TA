package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/exploration"
	"ta/internal/quest"
)

func authoredQuestRules(t *testing.T, packID string) AdventureRules {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "storypack", "assets", packID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Story struct {
			Gameplay struct {
				Rules AdventureRules `json:"adventure_rules"`
			} `json:"gameplay"`
		} `json:"story"`
	}
	if err = json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if err = p.Story.Gameplay.Rules.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Story.Gameplay.Rules.world = authoredWorldContent(t, packID)
	return p.Story.Gameplay.Rules
}

func authoredQuestStory(t *testing.T, packID, location string, currency int, skills map[string]int, hazards ...AdventureOpeningHazard) (*Store, string, string, string) {
	t.Helper()
	r := authoredQuestRules(t, packID)
	r.StartingCurrency = currency
	a := NewAdventureActor(r)
	a.OpeningHazards = append([]AdventureOpeningHazard(nil), hazards...)
	for id, rank := range skills {
		a.Skills[id] = rank
	}
	initial := map[string]any{}
	for _, op := range adventureActorOps(a, "") {
		initial[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, packID), AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}}}}
	dir := t.TempDir()
	s := NewStore(dir)
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "任务正式链", ActorState: &system, PackBinding: &StoryPackBinding{PackID: packID, ContentVersion: 1, Scope: packID}})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "看看周围", Narrative: "听到了当地的消息。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	return s, story.ID, turn.ID, dir
}

func questApplyOn(t *testing.T, s *Store, id, branch, task, action, choice, command string) QuestResult {
	t.Helper()
	result, err := s.ApplyQuest(id, QuestRequest{BranchID: branch, TaskID: task, Action: action, ChoiceID: choice, CommandID: command, CommandHash: ContentHashOf([]string{task, action, choice})})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func questWait(t *testing.T, s *Store, id, branch string, seconds int64) {
	t.Helper()
	text := fmt.Sprintf("等候%d秒", seconds)
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: text, Narrative: "约定的时间过去了。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: seconds, SourceText: text}}})
	if err != nil {
		t.Fatal(err)
	}
}

func questAvailable(state QuestState, id string) bool {
	for _, o := range state.Available {
		if o.TaskID == id {
			return true
		}
	}
	return false
}

// Character registration/presence are explicit synthetic scene fixtures. The
// player's chosen work, formal waiting and acknowledgement use normal commits.
func questTestFollowUp(t *testing.T, s *Store, id, branch, task, choiceID string) {
	t.Helper()
	d, _ := quest.DefinitionByID(task)
	c, ok := d.Choice(choiceID)
	if !ok {
		t.Fatal("missing source choice")
	}
	state, err := s.QuestStateFor(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	view := questTestTask(t, state, task)
	r := view.Resolutions[c.ObjectiveID]
	reaction := c.Reaction(r.Outcome)
	if reaction == nil {
		return
	}
	partner := "echo-source-" + reaction.CharacterSourceKey
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: "回到本人面前询问旧事", Narrative: "你在这里再次遇到那位当事人，准备核实旧事。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{partner}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: partner, Name: reaction.CharacterSourceKey, SourceKey: reaction.CharacterSourceKey}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: "问起上回的事后来如何", Narrative: reaction.Statement, TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: task, QuestAction: QuestActionFollowUp, QuestChoiceID: choiceID}}}); err != nil {
		t.Fatal(err)
	}
}

// This is P1-02's same-initial-fact comparison through the public Store API and
// main-turn commit. The later opportunities consume the resolved echo, so a
// saved field with no gameplay reader cannot make this test pass.
func TestAdventureQuestDelayedChoiceBranchesReloadAndRetry(t *testing.T) {
	s, id, opening, dir := authoredQuestStory(t, "xinghai-689", "朱伯利外环47号减压长廊", 1000, map[string]int{"xinghai-s25": 1})
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "另一种处理"})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"main", fork.ID} {
		questApplyOn(t, s, id, branch, "xinghai-a01", QuestActionAccept, "", "accept-"+branch)
	}
	first := questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionChoose, "xinghai-a01.c1", "work")
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: fork.ID, User: "尝试伪造通行代码", Narrative: "控制台暂时接受了记录，真实欠款仍在。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "xinghai-a01", QuestAction: QuestActionChoose, QuestChoiceID: "xinghai-a01.c3"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"main", fork.ID} {
		state, err := s.QuestStateFor(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		if questAvailable(state, "xinghai-a01-echo-1") || questAvailable(state, "xinghai-a01-echo-3") {
			t.Fatal("future consequence leaked before its clock")
		}
	}
	sourceDefinition, _ := quest.DefinitionByID("xinghai-a01")
	workChoice, _ := sourceDefinition.Choice("xinghai-a01.c1")
	forgedChoice, _ := sourceDefinition.Choice("xinghai-a01.c3")
	questWait(t, s, id, "main", workChoice.EchoAfterSeconds)
	questTestFollowUp(t, s, id, "main", "xinghai-a01", "xinghai-a01.c1")
	state, err := s.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !questAvailable(state, "xinghai-a01-echo-1") || questAvailable(state, "xinghai-a01-echo-3") {
		t.Fatalf("honest work did not unlock its own continuation: %+v", state.Available)
	}
	other, err := s.QuestStateFor(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if questAvailable(other, "xinghai-a01-echo-3") {
		t.Fatal("parent clock advanced the alternate branch")
	}
	before, err := s.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	replay := questApplyOn(t, s, id, "main", "xinghai-a01", QuestActionChoose, "xinghai-a01.c1", "work")
	after, _ := s.Snapshot(id, "main")
	if !replay.Replayed || replay.EventID != first.EventID || !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
		t.Fatal("retry changed money, items or time")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(dir)
	defer reopened.Close()
	state, err = reopened.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !questAvailable(state, "xinghai-a01-echo-1") {
		t.Fatal("reload lost resolved opportunity")
	}
	questWait(t, reopened, id, fork.ID, forgedChoice.EchoAfterSeconds)
	questTestFollowUp(t, reopened, id, fork.ID, "xinghai-a01", "xinghai-a01.c3")
	other, err = reopened.QuestStateFor(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !questAvailable(other, "xinghai-a01-echo-3") || questAvailable(other, "xinghai-a01-echo-1") {
		t.Fatal("different choice lost its distinct future")
	}
	questApplyOn(t, reopened, id, fork.ID, "xinghai-a01-echo-3", QuestActionAccept, "", "take-debt")
	questApplyOn(t, reopened, id, fork.ID, "xinghai-a01-echo-3", QuestActionChoose, "xinghai-a01-echo-3.c1", "pay-debt")
	altSnapshot, _ := reopened.Snapshot(id, fork.ID)
	actor, err := AdventureActorFromState(altSnapshot.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.Currency != 520 {
		t.Fatalf("delayed debt was not paid: %d", actor.Currency)
	}
	parentSnapshot, _ := reopened.Snapshot(id, "main")
	parent, _ := AdventureActorFromState(parentSnapshot.State, DefaultActorID)
	if parent.Currency != 1000 {
		t.Fatal("alternate debt changed parent balance")
	}
	questWait(t, reopened, id, fork.ID, 21600)
	paid, _ := reopened.Snapshot(id, fork.ID)
	paidActor, _ := AdventureActorFromState(paid.State, DefaultActorID)
	if paidActor.Currency != 520 {
		t.Fatal("repeated interval charged debt twice")
	}
	t.Log("same initial debt: labor opens a workshop job; forged payment opens a real 480-Inar debit after its authored audit interval and a real follow-up; retry, cold reload and branches remain isolated")
}

func TestAdventureQuestCostsRewardsAndFailureAdvance(t *testing.T) {
	for _, test := range []struct {
		name     string
		skill    int
		currency int
		want     int
		outcome  string
	}{{"paid-success", 1, 500, 400, "success"}, {"paid-failure", 0, 500, 400, "failure"}} {
		t.Run(test.name, func(t *testing.T) {
			s, id, _, _ := authoredQuestStory(t, "xinghai-689", "老锚机械酒吧与二手船材集市", test.currency, map[string]int{"xinghai-s30": test.skill})
			questApplyOn(t, s, id, "main", "xinghai-a02", QuestActionAccept, "", "accept")
			r := questApplyOn(t, s, id, "main", "xinghai-a02", QuestActionChoose, "xinghai-a02.c1", "pay")
			if r.Instance.Choices["xinghai-a02.resolve"].Outcome != test.outcome {
				t.Fatalf("wrong outcome: %+v", r.Instance)
			}
			snap, _ := s.Snapshot(id, "main")
			a, err := AdventureActorFromState(snap.State, DefaultActorID)
			if err != nil {
				t.Fatal(err)
			}
			if a.Currency != test.want {
				t.Fatalf("currency %d", a.Currency)
			}
			if test.outcome == "success" && a.Inventory["xinghai-i25"] != 1 {
				t.Fatal("real item reward is missing")
			}
			if test.outcome == "failure" && a.Inventory["xinghai-i25"] != 0 {
				t.Fatal("failed attempt was paid")
			}
			questWait(t, s, id, "main", 21600)
			state, _ := s.QuestStateFor(id, "main")
			view := questTestTask(t, state, "xinghai-a02")
			if len(view.Consequences) != 2 {
				t.Fatal("outcome did not survive to its delayed consumer")
			}
		})
	}
	s, id, _, _ := authoredQuestStory(t, "xinghai-689", "老锚机械酒吧与二手船材集市", 20, map[string]int{"xinghai-s30": 1})
	questApplyOn(t, s, id, "main", "xinghai-a02", QuestActionAccept, "", "accept")
	before, _ := s.Snapshot(id, "main")
	_, err := s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "xinghai-a02", Action: QuestActionChoose, ChoiceID: "xinghai-a02.c1"})
	if err == nil {
		t.Fatal("insufficient funds succeeded")
	}
	after, _ := s.Snapshot(id, "main")
	if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) {
		t.Fatal("rejected payment changed branch")
	}
}

func TestAdventureQuestDelayedWaterRouteUsesActualJourney(t *testing.T) {
	s, id, opening, _ := authoredQuestStory(t, "lanchuan-qunxia", "浮萍坞", 1000, map[string]int{"lanchuan-s35": 1})
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "保留旧制"})
	if err != nil {
		t.Fatal(err)
	}
	for index, branch := range []string{"main", fork.ID} {
		questApplyOn(t, s, id, branch, "lanchuan-a06", QuestActionAccept, "", "accept-"+branch)
		choiceID := fmt.Sprintf("lanchuan-a06.c%d", index+1)
		questApplyOn(t, s, id, branch, "lanchuan-a06", QuestActionChoose, choiceID, "choice-"+branch)
		journey, err := s.ApplyJourney(id, JourneyRequest{BranchID: branch, RouteID: "lanchuan-route-l07-l06", Arrive: true, CommandID: "to-town-" + branch, CommandHash: "same-route"})
		if err != nil {
			t.Fatal(err)
		}
		if journey.AddedMinutes != 720 {
			t.Fatalf("authored half-day crossing changed: %d", journey.AddedMinutes)
		}
		options, err := s.JourneyOptionsFor(id, branch)
		if err != nil {
			t.Fatal(err)
		}
		water := false
		for _, route := range options.Routes {
			if route.RouteID == "lanchuan-route-l06-l11-water" {
				water = true
			}
		}
		if water != (index == 0) {
			t.Fatalf("delayed policy did not change the actual route: %s %+v", branch, options.Routes)
		}
	}
	_, err = s.ApplyJourney(id, JourneyRequest{BranchID: fork.ID, RouteID: "lanchuan-route-l06-l11-water", Arrive: true})
	if err == nil {
		t.Fatal("hidden water route was accepted through a direct request")
	}
	trip, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: "lanchuan-route-l06-l11-water", Arrive: true})
	if err != nil {
		t.Fatal(err)
	}
	if trip.AddedMinutes != 4*1440 || journeyTestLocation(t, s, id, "main") != "清河城" {
		t.Fatal("water route did not move the canonical scene or spend four days")
	}
}

func TestAdventureRegionalTravelCommits(t *testing.T) {
	for _, pre := range []string{"lanchuan", "xinghai"} {
		pack := "lanchuan-qunxia"
		if pre == "xinghai" {
			pack = "xinghai-689"
		}
		for region := 1; region <= 8; region++ {
			t.Run(fmt.Sprintf("%s-r%02d", pre, region), func(t *testing.T) {
				origin := fmt.Sprintf("%s-l%02d", pre, (region-1)*5+1)
				var chosen string
				var minutes int64
				var destination string
				for _, route := range journeyRouteDefinitions {
					if route.OriginLocationID == origin && !strings.HasSuffix(route.ID, "-water") {
						chosen = route.ID
						destination = route.DestinationLocationID
						for _, segment := range route.Segments {
							minutes += segment.DurationMinutes
						}
						break
					}
				}
				if chosen == "" {
					t.Fatal("region has no outgoing authored connection")
				}
				s, id, _, _ := authoredQuestStory(t, pack, journeyPlaceLabel(origin), 1000, nil)
				result, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: chosen, Arrive: true, CommandID: "regional-trip", CommandHash: chosen})
				if err != nil {
					t.Fatal(err)
				}
				if result.AddedMinutes != minutes || journeyTestLocation(t, s, id, "main") != journeyPlaceLabel(destination) {
					t.Fatal("regional connection did not commit its actual destination and time")
				}
				replay, err := s.ApplyJourney(id, JourneyRequest{BranchID: "main", RouteID: chosen, Arrive: true, CommandID: "regional-trip", CommandHash: chosen})
				if err != nil || !replay.Replayed {
					t.Fatalf("regional retry: %+v %v", replay, err)
				}
			})
		}
	}
}

// Every authored choice is executable against real frozen catalogs. This
// supplements, rather than substitutes for, the full journal chains above.
func TestAllAdventureQuestChoicesUseRealCatalogs(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		r := authoredQuestRules(t, packID)
		a := NewAdventureActor(r)
		a.Rank = AdventureMaxRank(r)
		a.Currency = 1000000
		for _, skill := range r.Skills {
			a.Skills[skill.ID] = 4
		}
		for _, item := range r.Items {
			a.Inventory[item.ID] = 10
			if item.Unique {
				a.Inventory[item.ID] = 1
			}
		}
		for _, opponent := range r.Opponents {
			a.Defeated[opponent.ID] = true
		}
		for _, d := range quest.Definitions() {
			if d.PackageID != packID {
				continue
			}
			for _, c := range d.Choices {
				t.Run(c.ID, func(t *testing.T) {
					actor := a
					actor.Inventory = map[string]int{}
					for id, n := range a.Inventory {
						actor.Inventory[id] = n
					}
					actor.Acquired = map[string]bool{}
					actor.Assets = map[string]AdventureAsset{}
					actor.Equipment = map[string]string{}
					if c.RequiredAsset != "" {
						for _, item := range r.Items {
							if item.ID == c.RequiredAsset {
								actor.Assets[item.ID] = AdventureAsset{Health: item.Durability, MaxHealth: item.Durability, Fuel: 100}
								actor.Equipment[item.Scale+":asset"] = item.ID
							}
						}
					}
					for _, reward := range c.Rewards {
						if reward.Kind == quest.RewardItem {
							actor.Inventory[reward.RefID] = c.ItemCosts[reward.RefID]
						}
					}
					state := map[string]any{}
					for _, op := range adventureActorOps(actor, "") {
						setPath(state, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
					}
					if len(c.LocationIDs) > 0 {
						setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), journeyPlaceLabel(c.LocationIDs[0]))
					}
					instance := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted, CompletedObjectiveIDs: append([]string(nil), c.AfterObjectives...)}
					p := &storyBranchProjection{State: state, Quests: map[string]quest.Instance{d.ID: instance}}
					// Ownership-bearing gifts need their real giver, even in a fully
					// qualified catalog fixture. This does not waive any choice gate.
					if d.Giver != "" {
						p.Characters = &character.State{Version: character.StateVersion, Profiles: []character.Profile{{ID: "giver", StoryID: "catalog-fixture", Name: d.Giver, SourceKey: d.Giver, Revision: 1}}}
						setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{"giver"})
					}
					if c.RequiredJourney != "" {
						p.Journeys = map[string]exploration.JourneySnapshot{"trip": {RouteID: c.RequiredJourney, TotalMinutes: 7200, ElapsedMinutes: 7200}}
					}
					for _, id := range c.RequiredTasks {
						p.Quests[id] = quest.Instance{Status: quest.StatusCompleted}
						if c.Debt != nil && id == c.Debt.TaskID {
							p.Quests[id] = quest.Instance{Status: quest.StatusCompleted, Choices: map[string]quest.ChoiceResult{"loan": {Outcome: "success"}}}
						}
					}
					plan, err := planQuestAction(p, StoryDirectorActorStateSystem{AdventureRules: &r}, QuestRequest{TaskID: d.ID, Action: QuestActionChoose, ChoiceID: c.ID})
					if err != nil {
						t.Fatal(err)
					}
					if plan.Instance.Choices[c.ObjectiveID].Outcome != "success" || len(plan.Ops) == 0 || plan.ClockAfter.Seconds <= 0 {
						t.Fatal("choice lacks formal result")
					}
					if strings.Contains(fmt.Sprint(plan.Ops), "灵石") {
						t.Fatal("foreign cultivation resource leaked")
					}
				})
			}
		}
	}
}
