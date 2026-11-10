package interactive

import (
	"encoding/json"
	"reflect"
	"testing"
)

func spaceMentalFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r := authoredAdventureRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Rank = 9
	a.MaxHealth = 1000
	a.Health = 1000
	a.MaxEnergy = 1000
	a.Energy = 1000
	a.Skills["xinghai-s19"] = 1
	a.Skills["xinghai-s21"] = 1
	a.Inventory["xinghai-i55"] = 1
	return r, a
}

func TestSpaceMentalDefensesAreConditionalNotOffensiveControl(t *testing.T) {
	r, a := spaceMentalFixture(t)
	start := func(id string) AdventureActor {
		t.Helper()
		n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: id}, "mental-start")
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	ordinary := start("xinghai-sparring")
	ordinary.Battle.Attack = 50
	n, _, e := ResolveAdventureAction(r, ordinary, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s19"}, "ordinary-mental-guard")
	if e != nil {
		t.Fatal(e)
	}
	if n.Battle.Health != ordinary.Battle.Health || n.Health >= ordinary.Health || n.Battle.EnemyControl != 0 {
		t.Fatal("mental protection must not damage or unconditionally stop an ordinary enemy")
	}
	storm := start("xinghai-e06")
	protected, _, e := ResolveAdventureAction(r, storm, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s21"}, "protected")
	if e != nil {
		t.Fatal(e)
	}
	exposed, _, e := ResolveAdventureAction(r, storm, AdventureAction{Action: "battle_round", Choice: "guard"}, "exposed")
	if e != nil {
		t.Fatal(e)
	}
	p, _ := json.Marshal(protected)
	u, _ := json.Marshal(exposed)
	if protected.Battle.Health != storm.Battle.Health || protected.Health <= exposed.Health || !jsonHasPositiveField(t, u, "mental_corruption") || jsonHasPositiveField(t, p, "mental_corruption") {
		t.Fatal("the registered mental storm was not conditionally resisted without attacking it")
	}
}

func jsonHasPositiveField(t *testing.T, b []byte, key string) bool {
	t.Helper()
	var m map[string]any
	if e := json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	n, _ := m[key].(float64)
	return n > 0
}

func TestSpaceWishStoneRequiresRealCorruptionAndHasOnlyThreeUses(t *testing.T) {
	r, a := spaceMentalFixture(t)
	original := a
	input := AdventureAction{Action: "use", ItemID: "xinghai-i55"}
	if got, _, e := ResolveAdventureAction(r, a, input, "no-corruption"); e == nil || !reflect.DeepEqual(got, a) {
		t.Fatal("wish stone became unconditional healing or paid for an ineligible use")
	}
	b, _ := json.Marshal(a)
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	raw["mental_corruption"] = 90
	raw["mental_exposure_id"] = "xinghai-e06"
	b, _ = json.Marshal(raw)
	_ = json.Unmarshal(b, &a)
	for use := 0; use < 3; use++ {
		n, _, e := ResolveAdventureAction(r, a, input, "use-actual-stone")
		if e != nil {
			t.Fatal(e)
		}
		if n.Health != original.Health || n.Injury != original.Injury {
			t.Fatal("mental recovery was substituted with full-body healing")
		}
		a = n
	}
	if a.Inventory["xinghai-i55"] != 0 {
		t.Fatal("third use did not turn the actual stone into useless powder")
	}
	if got, _, e := ResolveAdventureAction(r, a, input, "fourth"); e == nil || !reflect.DeepEqual(got, a) {
		t.Fatal("an exhausted stone was usable again")
	}
}

func TestSpaceMentalStormOverpowersWeakWardsAndListeningForfeitsProtection(t *testing.T) {
	r, a := spaceMentalFixture(t)
	a.Rank = 4
	storm, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-e06"}, "storm")
	if e != nil {
		t.Fatal(e)
	}
	broken, _, e := ResolveAdventureAction(r, storm, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s21"}, "shattered-ward")
	if e != nil {
		t.Fatal(e)
	}
	if broken.MentalCorruption == 0 || broken.MentalShock != 1 || broken.Injury < 5 || broken.Battle.Health != storm.Battle.Health {
		t.Fatal("an overwhelming storm was stopped by an unqualified ward or the ward became an attack")
	}
	if got, _, e := ResolveAdventureAction(r, broken, AdventureAction{Action: "battle_round", Choice: "attack"}, "shocked-offense"); e == nil || !reflect.DeepEqual(got, broken) {
		t.Fatal("mental shock allowed offense or partially charged a rejected action")
	}
	listened, _, e := ResolveAdventureAction(r, storm, AdventureAction{Action: "battle_round", Choice: "listen"}, "listen")
	if e != nil {
		t.Fatal(e)
	}
	if listened.MentalCorruption <= broken.MentalCorruption || listened.MentalShock != 1 {
		t.Fatal("deliberate listening did not forfeit the ward and worsen actual corruption")
	}
	r, a = spaceMentalFixture(t)
	ordinary, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "xinghai-sparring"}, "ordinary")
	if e != nil {
		t.Fatal(e)
	}
	if got, _, e := ResolveAdventureAction(r, ordinary, AdventureAction{Action: "battle_round", Choice: "listen"}, "fictional-signal"); e == nil || !reflect.DeepEqual(got, ordinary) {
		t.Fatal("a nonexistent mental signal generated usable corruption")
	}
}

func TestSpaceWishStoneChargesSurviveCASBranchAndColdReplay(t *testing.T) {
	r, a := spaceMentalFixture(t)
	a.MentalCorruption = 90
	a.MentalExposureID = "xinghai-e06"
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		values[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values}}}
	s := NewStore(t.TempDir())
	defer func() { _ = s.Close() }()
	story, e := s.CreateStory(CreateStoryRequest{Title: "Known mental encounter fixture", ActorState: &system})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "查看心神", Narrative: "你辨认出侵蚀留下的影响。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); e != nil {
		t.Fatal(e)
	}
	use := func() (TurnEvent, AppendTurnWithStateRequest) {
		t.Helper()
		w, e := s.AdventureWindow(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		req := AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &w.Head, User: "贴上祈愿石子唤醒心神", Narrative: "晶石的紫光淡去一些。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "贴上祈愿石子唤醒心神", Adventure: &AdventureAction{Action: "use", ItemID: "xinghai-i55"}}}}
		turn, _, e := s.AppendTurnWithState(story.ID, req)
		if e != nil {
			t.Fatal(e)
		}
		return turn, req
	}
	first, req := use()
	w, e := s.AdventureWindow(story.ID, "main")
	if e != nil {
		t.Fatal(e)
	}
	if w.Actor.ItemCharges["xinghai-i55"] != 2 || w.Actor.MentalCorruption != 60 {
		t.Fatal("first actual use did not preserve the partially faded object")
	}
	if _, _, e = s.AppendTurnWithState(story.ID, req); e == nil {
		t.Fatal("stale main-turn retry consumed another charge")
	}
	fork, e := s.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Two uses remain"})
	if e != nil {
		t.Fatal(e)
	}
	use()
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	w, e = s.AdventureWindow(story.ID, "main")
	if e != nil || w.Actor.ItemCharges["xinghai-i55"] != 1 || w.Actor.MentalCorruption != 30 {
		t.Fatal("cold replay replenished charges", e)
	}
	use()
	w, e = s.AdventureWindow(story.ID, "main")
	if e != nil || w.Actor.Inventory["xinghai-i55"] != 0 || w.Actor.MentalCorruption != 0 {
		t.Fatal("third charge did not exhaust the real object", e)
	}
	earlier, e := s.AdventureWindow(story.ID, fork.ID)
	if e != nil || earlier.Actor.ItemCharges["xinghai-i55"] != 2 || earlier.Actor.MentalCorruption != 60 {
		t.Fatal("later charge use leaked into an earlier branch", e)
	}
}
