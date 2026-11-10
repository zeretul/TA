package interactive

import (
	"fmt"
	"reflect"
	"testing"
)

// Qualified solver preconditions, not an earned ship claim. A06 itself is still
// committed through the actual source quest, then the existing route is taken.
func qualifiedOriginCareerStory(t *testing.T, origin, shipState string) (*Store, string) {
	t.Helper()
	r, a := spaceOverclockFixture(t)
	a.Battle = nil
	a.Skills["xinghai-s01"] = 1
	a.Skills["xinghai-s04"] = 1
	switch shipState {
	case "missing":
		delete(a.Assets, "xinghai-frigate")
		delete(a.Inventory, "xinghai-frigate")
		delete(a.Equipment, "ship:asset")
	case "dry":
		x := a.Assets["xinghai-frigate"]
		x.Fuel = 0
		a.Assets["xinghai-frigate"] = x
	case "locked":
		x := a.Assets["xinghai-frigate"]
		x.DisabledUntil = 1000000
		a.Assets["xinghai-frigate"] = x
	}
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "qualified fixture") {
		values[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, "xinghai-689"), AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: "xinghai-l23"}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: "xinghai-l23"}}}}
	s := NewStore(t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	story, e := s.CreateStory(CreateStoryRequest{Title: "Qualified career source fixture", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "xinghai-689", ContentVersion: 1, Scope: "xinghai-689"}, PackTemplate: &StoryProtagonistTemplateRef{PackID: "xinghai-689", TemplateID: origin}})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "查看接待大厅公告", Narrative: "公会说明了登记的办法。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if e != nil {
		t.Fatal(e)
	}
	questApplyOn(t, s, story.ID, "main", "xinghai-a06", QuestActionAccept, "", "registration-accept")
	questApplyOn(t, s, story.ID, "main", "xinghai-a06", QuestActionChoose, "xinghai-a06.c2", "registration-work")
	for step, dest := range []string{"xinghai-l22", "xinghai-l21", "xinghai-l04", "xinghai-l01"} {
		routes, e := s.JourneyOptionsFor(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		arrived := false
		for _, route := range routes.Routes {
			if route.DestinationID == dest {
				_, e = s.ApplyJourney(story.ID, JourneyRequest{BranchID: "main", RouteID: route.RouteID, Arrive: true, CommandID: fmt.Sprint("career-route-", step), CommandHash: ContentHashOf(route.RouteID)})
				if e != nil {
					t.Fatal(e)
				}
				arrived = true
				break
			}
		}
		if !arrived {
			t.Fatal("real registered route missing", dest)
		}
	}
	return s, story.ID
}

func TestCareerShipCheckUsesProfessionalFactNotUnrelatedOriginDebt(t *testing.T) {
	for _, origin := range []string{"xinghai-c01", "xinghai-c02", "xinghai-c03"} {
		t.Run(origin, func(t *testing.T) {
			s, id := qualifiedOriginCareerStory(t, origin, "working")
			questApplyOn(t, s, id, "main", "xinghai-q03", QuestActionAccept, "", "career-accept")
			before, e := s.Snapshot(id, "main")
			if e != nil {
				t.Fatal(e)
			}
			result, e := s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "xinghai-q03", Action: QuestActionChoose, ChoiceID: "xinghai-q03.c1", CommandID: "career-ship", CommandHash: "career-ship"})
			if e != nil {
				t.Fatal("registered professional with a real working ship was forced to pay or help an unrelated origin's berth debt", e)
			}
			if result.Instance.Choices["xinghai-q03.stage1"].Outcome != "success" {
				t.Fatal("qualified ship fact was not consumed")
			}
			after, _ := s.Snapshot(id, "main")
			a, _ := AdventureActorFromState(after.State, DefaultActorID)
			b, _ := AdventureActorFromState(before.State, DefaultActorID)
			if a.Currency != b.Currency || !reflect.DeepEqual(a.Inventory, b.Inventory) {
				t.Fatal("career check invented a debt payment, ship or credential")
			}
		})
	}
}

func TestCareerShipCheckRejectsMissingDryOrPowerLockedAsset(t *testing.T) {
	for _, condition := range []string{"missing", "dry", "locked"} {
		t.Run(condition, func(t *testing.T) {
			s, id := qualifiedOriginCareerStory(t, "xinghai-c02", condition)
			questApplyOn(t, s, id, "main", "xinghai-q03", QuestActionAccept, "", "career-accept")
			before, _ := s.AdventureWindow(id, "main")
			_, e := s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "xinghai-q03", Action: QuestActionChoose, ChoiceID: "xinghai-q03.c1", CommandID: fmt.Sprint("invalid-", condition), CommandHash: condition})
			after, _ := s.AdventureWindow(id, "main")
			if e == nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("invalid ship fact advanced the career or charged time/rewards: err=%v head %s -> %s", e, before.Head, after.Head)
			}
		})
	}
}
