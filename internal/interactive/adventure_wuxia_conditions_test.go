package interactive

import (
	"encoding/json"
	"reflect"
	"testing"
)

func wuxiaConditionFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	r.NPCs = nil
	r.Opponents = []AdventureOpponent{{ID: "normal", Name: "普通对手", Location: "船坞", Scale: "person", Health: 1000, Attack: 30, Rank: 10, Energy: 100}, {ID: "other", Name: "另一对手", Location: "船坞", Scale: "person", Health: 1000, Attack: 30, Rank: 10, Energy: 100}}
	a := NewAdventureActor(r)
	a.Rank = 10
	a.Health = 10000
	a.MaxHealth = 10000
	a.Energy = 1000
	a.MaxEnergy = 1000
	a.Skills["lanchuan-s12"] = 1
	a.Skills["lanchuan-s31"] = 1
	a.Skills["lanchuan-s26"] = 1
	a.Inventory["lanchuan-acupuncture-needles"] = 1
	a.Inventory["lanchuan-i26"] = 1
	a.Inventory["lanchuan-i21"] = 1
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "normal"}, "conditions-start")
	if e != nil {
		t.Fatal(e)
	}
	return r, n
}

func TestWuxiaAcupointSealIsNotDamageOrWholeTurnControl(t *testing.T) {
	r, a := wuxiaConditionFixture(t)
	before := a.Battle.Health
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}, "seal")
	if e != nil {
		t.Fatal(e)
	}
	if n.Battle.Health != before || n.Health >= a.Health {
		t.Fatalf("seal should restrict rush, not damage or cancel the ordinary reply: player %d→%d enemy%d→%d", a.Health, n.Health, before, n.Battle.Health)
	}
	if got, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}, "repeat"); e == nil || !reflect.DeepEqual(got, n) {
		t.Fatal("adjacent same-target sealing was not atomically refused")
	}
	if _, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s31", TargetID: "opponent"}, "alternate-seal-bypass"); e == nil {
		t.Fatal("switching sealing skills bypassed the adjacent-target restriction")
	}
	gap, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "guard"}, "gap")
	if e != nil {
		t.Fatal(e)
	}
	again, _, e := ResolveAdventureAction(r, gap, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}, "again")
	if e != nil {
		t.Fatal("a gap must allow the same target again", e)
	}
	again.Battle.OpponentID = "other"
	if _, _, e = ResolveAdventureAction(r, again, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}, "different"); e != nil {
		t.Fatal("different target was incorrectly lifetime-blocked", e)
	}
	a.Health = 500
	healed, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s31"}, "heal")
	if e != nil {
		t.Fatal(e)
	}
	sealed, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s31", TargetID: "opponent"}, "needle-seal")
	if e != nil {
		t.Fatal(e)
	}
	if healed.Health <= sealed.Health || sealed.Health >= a.Health || sealed.Battle.Health != a.Battle.Health {
		t.Fatal("acupuncture must distinguish healing from explicit enemy sealing")
	}
}

func TestWuxiaSofteningDoseCannotBecomePoisonDamage(t *testing.T) {
	r, a := wuxiaConditionFixture(t)
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "item", ItemID: "lanchuan-i26"}, "dose")
	if e != nil {
		t.Fatal(e)
	}
	if n.Battle.Health != a.Battle.Health || n.Battle.EnemyPoison != 0 || n.Inventory["lanchuan-i26"] != 0 || n.Health != a.Health {
		t.Fatal("real consumed softening dose must cause verbal standoff, not poison ticks or enemy weapon attacks")
	}
	// Drinking one's own actual stock is a separate, explicit ingestion fact.
	a.Battle.Status = "escaped"
	n, _, e = ResolveAdventureAction(r, a, AdventureAction{Action: "use", ItemID: "lanchuan-i26"}, "drink")
	if e != nil {
		t.Fatal(e)
	}
	n, _, e = ResolveAdventureAction(r, n, AdventureAction{Action: "battle_start", OpponentID: "normal"}, "drugged-start")
	if e != nil {
		t.Fatal(e)
	}
	n.Inventory["lanchuan-i01"] = 1
	if _, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "equip", ItemID: "lanchuan-i01"}, "drugged-draw"); e == nil {
		t.Fatal("softened actor drew an owned weapon")
	}
	before, _ := json.Marshal(n)
	got, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "attack"}, "drugged-attack")
	after, _ := json.Marshal(got)
	if e == nil || string(before) != string(after) {
		t.Fatal("drugged actor attacked or refusal was not atomic")
	}
	ended, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "talk"}, "standoff")
	if e != nil {
		t.Fatal("verbal standoff must remain possible", e)
	}
	if ended.SoftLimbs || ended.Battle.Status != "standoff" || ended.Currency != n.Currency || len(ended.Defeated) != len(n.Defeated) {
		t.Fatal("standoff must clear this encounter's condition without becoming a paid victory")
	}
}

func TestWuxiaSealBlocksRushButLeavesOrdinaryRepliesAndNPCCosts(t *testing.T) {
	r, a := wuxiaConditionFixture(t)
	r.Opponents[0].SkillIDs = []string{"lanchuan-s26"}
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}, "block-rush")
	if e != nil {
		t.Fatal(e)
	}
	if n.Health >= a.Health || n.Battle.EnemyEnergy != a.Battle.EnemyEnergy {
		t.Fatal("blocked enemy rush must fall back to ordinary reply without paying for the refused technique")
	}
	r.Opponents[0].SkillIDs = []string{"lanchuan-s12"}
	n, _, e = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:lanchuan-s12"}, "enemy-seals")
	if e != nil {
		t.Fatal(e)
	}
	if n.RushBlocked != 1 || n.Control != 0 {
		t.Fatal("enemy seal did not restrict only rush")
	}
	if got, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s26"}, "refused-rush"); e == nil || !reflect.DeepEqual(got, n) {
		t.Fatal("sealed protagonist rushed or paid for refusal")
	}
	ordinary, _, e := ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "attack"}, "ordinary-reply")
	if e != nil {
		t.Fatal(e)
	}
	if ordinary.Battle.Health >= n.Battle.Health || ordinary.RushBlocked != 0 || ordinary.Battle.EnemyEnergy != n.Battle.EnemyEnergy {
		t.Fatal("ordinary action was swallowed or NPC repeated sealing consecutively")
	}
	if _, _, e = ResolveAdventureAction(r, ordinary, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s26"}, "rush-after-gap"); e != nil {
		t.Fatal("one exchange restriction became permanent", e)
	}
}

// Explicit bounded solver fixture, not a normal earned progression campaign.
func wuxiaConditionStory(t *testing.T) (*Store, string) {
	t.Helper()
	r, a := wuxiaConditionFixture(t)
	for _, id := range []string{"lanchuan-s03", "lanchuan-s07", "lanchuan-s10", "lanchuan-s11"} {
		a.Skills[id] = 1
	}
	a.Battle = nil
	state := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		state[op.FieldID] = op.Value
	}
	system := StoryDirectorActorStateSystem{WorldContent: r.world, AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: "船坞"}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: state}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: "船坞"}}}}
	s := NewStore(t.TempDir())
	story, e := s.CreateStory(CreateStoryRequest{Title: "Source condition solver", ActorState: &system})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "开始", Narrative: "双方在场。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); e != nil {
		t.Fatal(e)
	}
	return s, story.ID
}

func TestWuxiaSealMarkerColdReplayAndEarlierBranch(t *testing.T) {
	s, id := wuxiaConditionStory(t)
	defer func() { _ = s.Close() }()
	start, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "normal"}}}})
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.CommitAdventureAction(id, "main", "seal-marker", w.Head, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"})
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.CommitAdventureAction(id, "main", "seal-marker", w.Head, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"})
	if e != nil || !reflect.DeepEqual(first, retry) {
		t.Fatal("seal command was not idempotent", e)
	}
	fork, e := s.CreateBranch(id, CreateBranchRequest{ParentEventID: start.ID, Title: "Before first seal"})
	if e != nil {
		t.Fatal(e)
	}
	branch, e := s.AdventureWindow(id, fork.ID)
	if e != nil || branch.Actor.LastSealTarget != "" {
		t.Fatal("seal leaked into earlier branch", e)
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	restored, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(restored.Actor, first.Actor) {
		t.Fatal("cold replay lost seal marker", e)
	}
	if _, e = s.CommitAdventureAction(id, "main", "seal-again", restored.Head, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s12"}); e == nil {
		t.Fatal("reopening reset adjacent-target restriction")
	}
}

func TestWuxiaFogBlocksRushUntilSpecificAntidoteWithoutPoisonTicks(t *testing.T) {
	r, a := wuxiaConditionFixture(t)
	a.FogAreaID = r.FogAreas[0].ID
	a.Inventory["lanchuan-i20"] = 1
	in := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s26"}
	if got, _, e := ResolveAdventureAction(r, a, in, "fog-rush"); e == nil || !reflect.DeepEqual(got, a) {
		t.Fatal("fog did not atomically block an actually learned rush technique")
	}
	n, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "item", ItemID: "lanchuan-i20"}, "wrong-antidote")
	if e != nil {
		t.Fatal(e)
	}
	if n.FogTreated || n.Poison != 0 {
		t.Fatal("generic poison removal either cured source-specific fog or fog became HP poison")
	}
	n, _, e = ResolveAdventureAction(r, n, AdventureAction{Action: "battle_round", Choice: "item", ItemID: "lanchuan-i21"}, "warding-antidote")
	if e != nil {
		t.Fatal(e)
	}
	if !n.FogTreated || n.Inventory["lanchuan-i21"] != 0 {
		t.Fatal("actual warding dose was not consumed")
	}
	if _, _, e = ResolveAdventureAction(r, n, in, "treated-rush"); e != nil {
		t.Fatal("treated actor still cannot rush", e)
	}
}

func TestWuxiaSoftLimbsCannotPhysicallyGuardOrUseDefensiveTechniques(t *testing.T) {
	r, a := wuxiaConditionFixture(t)
	a.SoftLimbs = true
	a.Skills["lanchuan-s03"] = 1
	a.Skills["lanchuan-s06"] = 1
	a.Skills["lanchuan-basic-breath"] = 1
	for _, in := range []AdventureAction{{Action: "battle_round", Choice: "guard"}, {Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s03"}, {Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s31"}, {Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s06"}} {
		got, _, err := ResolveAdventureAction(r, a, in, "soft-no-defense")
		if err == nil || !reflect.DeepEqual(got, a) {
			t.Fatal("softened limbs made a physical defense or needle technique, or rejection charged a cost")
		}
	}
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-basic-breath"}, "inward-recovery"); err != nil {
		t.Fatal("inward breathing recovery was wrongly treated as physical defense", err)
	}
	a.Control = 1
	spoken, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "talk"}, "soft-can-speak")
	if err != nil || spoken.Battle.Status != "standoff" {
		t.Fatal("coincident generic control trapped a softened participant instead of allowing verbal standoff", err)
	}
}
