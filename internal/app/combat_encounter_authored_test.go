package app

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file covers the one path a regional boss really takes: the story invents
// the opponent in a turn and the window fights it.
//
// Every other combat fixture builds its cast at CreateStory time through
// InitialActors. That is not how a story meets a stranger — the model creates the
// Actor in the turn that introduces them — so a fight that only works for a
// pre-seeded cast would pass every test and still fail the first time a player
// ran into a boss.

// TestCombatWindowFightsAnOpponentTheTurnCreated is the premise the authored boss
// rests on. The turn creates the opponent, states its realm and the technique it
// knows; the window must derive the encounter from the scene and read the numbers
// the cultivation rules settle from that realm.
func TestCombatWindowFightsAnOpponentTheTurnCreated(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	system := combatTestSystem(t, "fire_bolt")
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "地域首领", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}

	// The turn introduces the boss the way a real one does: a create operation
	// naming the opponent template and the realm, plus the technique it knows.
	stage := cultivation.StageKey(cultivation.Qi(9))
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "追到山口", Narrative: "断崖下有人等着你，刀鞘横在膝上。",
		TurnResult: &interactive.TurnResult{
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Create, Path: "/刀客", Value: map[string]any{
					"template_id": interactive.ActorStateOpponentTemplateID,
					"name":        "刀客",
					"state": map[string]any{
						combatTestRealmField:  stage,
						combatTestSkillsField: learnedSkillRecords([]string{"fire_bolt"}),
					},
				}},
				{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{interactive.DefaultActorID, "刀客"}},
				{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"刀客": "刀客起身拦住去路"}},
			},
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
		},
	})
	if err != nil {
		t.Fatalf("a turn could not create the opponent: %v", err)
	}

	application := &App{workspace: root, interactive: store}
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "" {
		t.Fatalf("the created opponent must settle a fight: %#v", window.Encounter)
	}
	sides := map[string]string{}
	for _, combatant := range window.Encounter.Combatants {
		sides[combatant.ActorID] = combatant.Side
	}
	if sides["刀客"] != string(cultivation.SideEnemy) {
		t.Fatalf("the created opponent must be on the enemy side: %#v", window.Encounter.Combatants)
	}

	// Opening the fight reads the rules' own numbers for that realm. The window
	// reports an opponent's pools only as observed, so the check is on the session
	// rather than the read: an enemy whose realm the rules never settled would
	// open with no pool at all.
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: story.ID, CommandID: "cmd-boss"})
	if err != nil {
		t.Fatalf("the created opponent could not be fought: %v", err)
	}
	if opened.SessionID == "" || opened.Ended {
		t.Fatalf("the fight must open and be under way: %#v", opened)
	}
	rival := combatWindowSide(t, opened, "刀客")
	if rival.Side != string(cultivation.SideEnemy) {
		t.Fatalf("the opened fight must place the created opponent as the enemy: %#v", rival)
	}

	// The boss's key technique must be the one the turn declared, not a bare
	// strike: an opponent with no skills cannot express a readable key move.
	sessions, err := store.CombatSessions(story.ID, "main")
	if err != nil {
		t.Fatalf("the fight must be readable after it opens: %v", err)
	}
	session, ok := sessions[opened.SessionID]
	if !ok {
		t.Fatalf("the opened fight must be in the branch's record: %#v", sessions)
	}
	var found *cultivation.Combatant
	for index := range session.Combatants {
		if session.Combatants[index].ID == "刀客" {
			found = &session.Combatants[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("the created opponent must be a combatant: %#v", session.Combatants)
	}
	if found.Attributes.HealthMax <= 0 {
		t.Fatalf("the realm the turn stated must settle real pools, got %#v", found)
	}
	hasKeyMove := false
	for _, skill := range found.Skills {
		if skill.ID == "fire_bolt" {
			hasKeyMove = true
			break
		}
	}
	if !hasKeyMove {
		t.Fatalf("the technique the turn recorded must reach the fight: %#v", found.Skills)
	}
}
