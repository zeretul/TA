package app

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// This file covers the encounter the window opens from the scene's own facts.
//
// The window used to lay the whole cast out and make the player side every name.
// That turned a bystander into an enemy and let someone who is elsewhere join a
// fight they are not in. The scene states who is present (在场角色) and, in this
// turn, who is actually attacking the protagonist (本次交战), so the encounter is
// read off those instead of asked for.
//
// The fixture is one natural encounter: the protagonist meets an opponent and an
// important character in the same place. Nothing here edits rules or picks the
// numbers that make a claim convenient — the pools come from the real preset.

// combatEncounterFixture builds a story whose scene has settled presence.
func combatEncounterFixture(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	stage := cultivation.StageKey(cultivation.Qi(6))
	system := combatTestSystem(t, "fire_bolt")
	system.InitialActors = append(system.InitialActors,
		interactive.ActorStateInitialActor{
			ID: "狼王", Name: "狼王", TemplateID: interactive.ActorStateOpponentTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
		interactive.ActorStateInitialActor{
			ID: "沈青", Name: "沈青", TemplateID: interactive.ActorStateImportantCharacterTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
		// Someone the story has tracked but who is not in this scene. They must not
		// join the fight from another room.
		interactive.ActorStateInitialActor{
			ID: "陆远", Name: "陆远", TemplateID: interactive.ActorStateImportantCharacterTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
	)

	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "自然遭遇", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	// 在场角色 is the story's own presence fact, written the way a turn writes it.
	// 本次交战 is the same turn's claim about who is actually attacking: the scene
	// states it, so the encounter is not guessed from anyone's template.
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "进山", Narrative: "狼王扑上来，沈青退到一旁。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "狼王", "沈青"},
				Reason: "本回合确认的在场角色",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "本次交战", Value: map[string]any{"狼王": "狼王扑上来拦住去路"},
				Reason: "本回合确认的交战",
			},
		},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

// TestCombatWindowDerivesTheEncounterFromTheScene is the acceptance item: the
// entry read reports who is fighting and on which side, without the player siding
// the cast.
func TestCombatWindowDerivesTheEncounterFromTheScene(t *testing.T) {
	application, storyID := combatEncounterFixture(t)
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil {
		t.Fatal("the entry read must report the encounter the scene settles")
	}
	if window.Encounter.ReasonKey != "" {
		t.Fatalf("this scene settles a fight, so no reason is owed: %q", window.Encounter.ReasonKey)
	}
	sides := map[string]string{}
	for _, combatant := range window.Encounter.Combatants {
		sides[combatant.ActorID] = combatant.Side
	}
	if sides[interactive.DefaultActorID] != string(cultivation.SidePlayer) {
		t.Fatalf("the protagonist fights on the player's side: %#v", window.Encounter.Combatants)
	}
	if sides["狼王"] != string(cultivation.SideEnemy) {
		t.Fatalf("an 敌对对象 is on the enemy side: %#v", window.Encounter.Combatants)
	}
	// 中立不变敌人: a present important character is not conscripted onto either side.
	if _, joined := sides["沈青"]; joined {
		t.Fatalf("a neutral bystander must not be sided by the derivation: %#v", window.Encounter.Combatants)
	}
	if len(window.Encounter.Bystanders) != 1 || window.Encounter.Bystanders[0] != "沈青" {
		t.Fatalf("the bystander must be reported as standing there: %#v", window.Encounter.Bystanders)
	}
	// 异地不参战: tracked but not present.
	if _, joined := sides["陆远"]; joined {
		t.Fatalf("an actor who is elsewhere must not join: %#v", window.Encounter.Combatants)
	}
}

// TestCombatRoundOpensTheDerivedEncounterWithNoRoster covers the write side. A
// round with no combatants named is opened from the same derivation, so the
// normal button needs no roster.
func TestCombatRoundOpensTheDerivedEncounterWithNoRoster(t *testing.T) {
	application, storyID := combatEncounterFixture(t)
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-encounter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if window.SessionID == "" || window.Ended {
		t.Fatalf("the fight must open and be under way: %#v", window)
	}
	if window.PlayerActorID != interactive.DefaultActorID {
		t.Fatalf("the player composes the protagonist's actions, got %q", window.PlayerActorID)
	}
	rival := combatWindowSide(t, window, "狼王")
	if rival.Side != string(cultivation.SideEnemy) {
		t.Fatalf("the derived opponent must be on the enemy side: %#v", rival)
	}
	for _, side := range window.Sides {
		if side.ActorID == "沈青" || side.ActorID == "陆远" {
			t.Fatalf("only the settled participants may be in the fight: %#v", window.Sides)
		}
	}
}

// TestCombatRoundRefusesASceneWithNoEncounter keeps the derivation from opening
// an empty battle. Presence is settled, but nobody present is hostile.
func TestCombatRoundRefusesASceneWithNoEncounter(t *testing.T) {
	application, storyID := combatEncounterFixture(t)
	// A second story whose present list has no opponent in it.
	store := application.interactive
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := combatTestSystem(t)
	system.InitialActors = append(system.InitialActors, interactive.ActorStateInitialActor{
		ID: "沈青", Name: "沈青", TemplateID: interactive.ActorStateImportantCharacterTemplateID,
		State: map[string]any{combatTestRealmField: stage},
	})
	peaceful, err := store.CreateStory(interactive.CreateStoryRequest{Title: "无战事", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(peaceful.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "饮茶", Narrative: "沈青在茶棚里说话。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "沈青"},
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: peaceful.ID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("a scene with no opponent must say so: %#v", window.Encounter)
	}
	_, err = application.PlayCombatRound(CombatRoundRequest{
		StoryID: peaceful.ID, CommandID: "cmd-peace",
	})
	if !errors.Is(err, ErrCombatNoEncounter) {
		t.Fatalf("a scene with no fight must refuse rather than open an empty battle: %v", err)
	}
	_ = storyID
}

// TestCombatRoundKeepsAnExplicitRosterWhenOneIsGiven covers the caller's own
// choice. The derivation is a default, not an authority: an interface that names
// participants still gets those participants.
func TestCombatRoundKeepsAnExplicitRosterWhenOneIsGiven(t *testing.T) {
	application, storyID := combatEncounterFixture(t)
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-explicit",
		Combatants: []interactive.CombatantSpec{
			{ActorID: interactive.DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
			{ActorID: "沈青", Side: cultivation.SideEnemy, RouteExists: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := combatWindowSideOptional(window, "狼王"); ok {
		t.Fatalf("an explicit roster replaces the derivation: %#v", window.Sides)
	}
	if side := combatWindowSide(t, window, "沈青"); side.Side != string(cultivation.SideEnemy) {
		t.Fatalf("the named combatant keeps the side the caller gave: %#v", side)
	}
}

// The tests below cover the other half of the encounter: a person who is
// already tracked as an important character can be fought without becoming a
// second Actor, and a fight only exists once the scene has committed that it
// does.
//
// R2's defect was that the hostile side came from the creation-time template
// alone. A template is fixed when the Actor is created, so a person the
// protagonist met as an important character could never be fought however
// openly they attacked, and an opponent whose fight had ended stayed hostile
// forever. The scene now states the engagement in its own committed field
// (本次交战): the actor id is the key, the value is the traceable basis. The
// template still says what someone is; it no longer decides whether a fight is
// happening.

// combatEngagementFixture seeds the same cast as combatEncounterFixture, but
// starts with an ordinary scene the caller then advances. 狼王 is tracked and
// elsewhere, so the opponent template cannot carry a fight into a scene it is
// not in.
func combatEngagementFixture(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	stage := cultivation.StageKey(cultivation.Qi(6))
	system := combatTestSystem(t, "fire_bolt")
	system.InitialActors = append(system.InitialActors,
		interactive.ActorStateInitialActor{
			ID: "狼王", Name: "狼王", TemplateID: interactive.ActorStateOpponentTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
		interactive.ActorStateInitialActor{
			ID: "沈青", Name: "沈青", TemplateID: interactive.ActorStateImportantCharacterTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
		interactive.ActorStateInitialActor{
			ID: "陆远", Name: "陆远", TemplateID: interactive.ActorStateImportantCharacterTemplateID,
			State: map[string]any{combatTestRealmField: stage},
		},
	)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "交恶", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	return &App{workspace: root, interactive: store}, story.ID
}

// settleCombatScene commits one turn whose only content is the scene the
// encounter is read from. An engagement of nil leaves the declaration absent,
// which is what a story with no fight in it writes.
func settleCombatScene(t *testing.T, store *interactive.Store, storyID string, present []any, engagement map[string]any) {
	t.Helper()
	ops := []interactive.ActorStateOp{{
		Op: "set", ActorID: interactive.DefaultStoryContextActorID,
		FieldID: "在场角色", Value: present, Reason: "本回合确认的在场角色",
	}}
	if engagement != nil {
		ops = append(ops, interactive.ActorStateOp{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "本次交战", Value: engagement, Reason: "本回合确认的交战",
		})
	}
	if _, _, err := store.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "向前", Narrative: "场面继续。",
		ActorOps:    ops,
		TurnResult:  &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCombatEncounterSidesADeclaredAttacker is R2's acceptance item: someone
// created as an important character attacks, and fights as themselves.
func TestCombatEncounterSidesADeclaredAttacker(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "沈青"},
		map[string]any{"沈青": "沈青拔刀劈向主角"})

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "" {
		t.Fatalf("a declared attack settles a fight: %#v", window.Encounter)
	}
	sides := map[string]string{}
	for _, combatant := range window.Encounter.Combatants {
		sides[combatant.ActorID] = combatant.Side
	}
	if sides["沈青"] != string(cultivation.SideEnemy) {
		t.Fatalf("the declared attacker fights on the enemy side: %#v", window.Encounter.Combatants)
	}
	if len(window.Encounter.Bystanders) != 0 {
		t.Fatalf("nobody else is standing by: %#v", window.Encounter.Bystanders)
	}

	// The same actor id is the fighter: no 敌人版分身, and no template change.
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "cmd-attack"})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatWindowSide(t, opened, "沈青")
	if rival.Side != string(cultivation.SideEnemy) {
		t.Fatalf("the original Actor is the enemy: %#v", rival)
	}
	context, err := application.interactive.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicated := combatActorRecord(context.Snapshot.State, "沈青"); !duplicated {
		t.Fatalf("the fight must not create a second Actor for the same person: %#v", context.Snapshot.State)
	}
}

// TestCombatEncounterKeepsALowFavorabilityBystander covers the false positive
// R2 names: dislike is not a fight. 沈青 is present, tracked, and hostile in
// favorability, and the scene has declared no attack.
func TestCombatEncounterKeepsALowFavorabilityBystander(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	if _, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "靠近", Narrative: "沈青冷着脸站在一旁。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "沈青"}, Reason: "本回合确认的在场角色",
			},
			{
				Op: "set", ActorID: "沈青",
				FieldID: "对主角好感度", Value: float64(-85), Reason: "此前的冲突",
			},
		},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("low favorability alone is not a fight: %#v", window.Encounter)
	}
	if len(window.Encounter.Bystanders) != 1 || window.Encounter.Bystanders[0] != "沈青" {
		t.Fatalf("the disliked bystander is reported as standing there: %#v", window.Encounter.Bystanders)
	}
}

// TestCombatEncounterDoesNotJoinAnAbsentDeclaredAttacker keeps a declaration
// from reaching across rooms: presence is still its own committed fact.
func TestCombatEncounterDoesNotJoinAnAbsentDeclaredAttacker(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID},
		map[string]any{"陆远": "陆远在别处放了冷箭"})

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("someone who is elsewhere cannot join: %#v", window.Encounter)
	}
	for _, combatant := range window.Encounter.Combatants {
		if combatant.ActorID == "陆远" {
			t.Fatalf("an absent actor must not be sided: %#v", window.Encounter.Combatants)
		}
	}
}

// TestCombatEncounterDropsAnOpponentThatStoodDown is the other half of R2: a
// fight that ended is over. 狼王 is still an opponent by template and still in
// the room, and the scene says nobody is attacking.
func TestCombatEncounterDropsAnOpponentThatStoodDown(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "狼王"},
		map[string]any{})

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("an ended fight must not be reopened by the template: %#v", window.Encounter)
	}
	if len(window.Encounter.Bystanders) != 1 || window.Encounter.Bystanders[0] != "狼王" {
		t.Fatalf("the stood-down opponent is standing there, not fighting: %#v", window.Encounter.Bystanders)
	}
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-stood-down",
	}); !errors.Is(err, ErrCombatNoEncounter) {
		t.Fatalf("an ended fight must refuse to reopen: %v", err)
	}
}

// TestCombatEncounterKeepsTheTemplateRuleForAnOlderStory covers the story whose
// frozen schema predates the declaration field. It cannot write the field, so
// the rule it was created under still decides — the new derivation must not
// leave such a story unable to fight at all.
func TestCombatEncounterKeepsTheTemplateRuleForAnOlderStory(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	stage := cultivation.StageKey(cultivation.Qi(6))
	system := combatTestSystem(t)
	// A story frozen before 本次交战 existed: the story-context template carries
	// only the fields it had then.
	for index := range system.Templates {
		if system.Templates[index].ID != interactive.ActorStateStoryContextTemplateID {
			continue
		}
		fields := system.Templates[index].Fields[:0]
		for _, field := range system.Templates[index].Fields {
			if field.Name != "本次交战" {
				fields = append(fields, field)
			}
		}
		system.Templates[index].Fields = fields
	}
	system.InitialActors = append(system.InitialActors, interactive.ActorStateInitialActor{
		ID: "狼王", Name: "狼王", TemplateID: interactive.ActorStateOpponentTemplateID,
		State: map[string]any{combatTestRealmField: stage},
	})
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "旧档", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "进山", Narrative: "狼王挡在路前。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "狼王"}, Reason: "本回合确认的在场角色",
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: root, interactive: store}
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "" {
		t.Fatalf("a story without the declaration field keeps its own rule: %#v", window.Encounter)
	}
	sides := map[string]string{}
	for _, combatant := range window.Encounter.Combatants {
		sides[combatant.ActorID] = combatant.Side
	}
	if sides["狼王"] != string(cultivation.SideEnemy) {
		t.Fatalf("the older story still fights its opponent: %#v", window.Encounter.Combatants)
	}
}

// combatActorRecord reports whether the state holds a record for this actor.
func combatActorRecord(state map[string]any, actorID string) (any, bool) {
	actors, _ := state["actors"].(map[string]any)
	record, ok := actors[actorID]
	return record, ok
}

func combatWindowSideOptional(window CombatWindow, actorID string) (CombatWindowSide, bool) {
	for _, side := range window.Sides {
		if side.ActorID == actorID {
			return side, true
		}
	}
	return CombatWindowSide{}, false
}
