package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func TestFreezeCombatLethalIntentsRequiresCommittedDirectedEvidence(t *testing.T) {
	session := fatalityTestSession(30)
	for _, test := range []struct {
		name      string
		user      string
		narrative string
		actorID   string
		targetID  string
		want      int
	}{
		{name: "player dialogue alone does not authorize a killing blow", user: "我要杀死对手", actorID: DefaultActorID, targetID: "rival", want: 0},
		{name: "sparring does not become lethal", user: "我要和对手切磋", actorID: DefaultActorID, targetID: "rival", want: 0},
		{name: "nonlethal qualifier rejects mixed wording", user: "我要杀死对手，只是切磋", actorID: DefaultActorID, targetID: "rival", want: 0},
		{name: "private thought is not exposed intent", user: "继续交战", narrative: "对手心中决定杀死主角。", actorID: "rival", targetID: DefaultActorID, want: 0},
		{name: "spoken threat is exposed intent", user: "继续交战", narrative: "对手喝道：‘我要杀死主角！’", actorID: "rival", targetID: DefaultActorID, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := TurnEvent{
				V: 1, Type: StoryEventTypeTurn, ID: "turn-lethal", User: test.user, Narrative: test.narrative,
				StateDelta: &StateDelta{Ops: []interactivestate.Op{{
					Op: "set", Path: "/story/本次交战", SourceTurnID: "turn-lethal", SourceKind: interactivestate.SourceTurnResult,
					Value: map[string]any{"rival": map[string]any{"basis": "已显露杀意", "lethal_intents": []any{map[string]any{
						"actor_id": test.actorID, "target_id": test.targetID, "source_exchange_index": 1,
					}}}},
				}}},
			}
			encoded, err := json.Marshal(turn)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			record := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeTurn, ID: turn.ID}, Raw: raw}
			lines := []StoryEventRecord{record}
			if test.want == 1 {
				speaker, characterID := character.SpeakerCharacter, test.actorID
				utterance := "I will kill you."
				if test.actorID == DefaultActorID {
					speaker, characterID = character.SpeakerPlayer, test.targetID
					// The player's line is the player's own action: a line the model
					// wrote for the player that the player never sent proves nothing.
					utterance = test.user
				}
				characterEvent := CharacterRecordEvent{
					V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: "turn-lethal-characters", ParentID: turn.ID, TurnID: turn.ID,
					Exchanges: []character.Exchange{{ID: "source-line", TurnID: turn.ID, CharacterID: characterID, Speaker: speaker, Channel: character.ChannelScene, AddressedTo: test.targetID, Text: utterance}},
				}
				characterRaw, err := json.Marshal(characterEvent)
				if err != nil {
					t.Fatal(err)
				}
				var characterRecordRaw map[string]any
				if err := json.Unmarshal(characterRaw, &characterRecordRaw); err != nil {
					t.Fatal(err)
				}
				lines = append(lines, StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeCharacterRecord, ID: characterEvent.ID}, Raw: characterRecordRaw})
			}
			head := turn.ID
			if len(lines) > 1 {
				head = lines[1].Envelope.ID
			}
			got := freezeCombatLethalIntents(lines, head, session)
			if len(got) != test.want {
				t.Fatalf("frozen lethal intents = %#v, want %d", got, test.want)
			}
			if len(got) == 1 && (got[0].SourceTurnID != turn.ID || got[0].SourceEventID != turn.ID || got[0].ActorID != test.actorID || got[0].TargetID != test.targetID) {
				t.Fatalf("frozen intent lost directed source: %#v", got[0])
			}
		})
	}
}

func TestPlayerTakeLifeChoiceNeedsAnActualAttackAndLiveOpponent(t *testing.T) {
	session := fatalityTestSession(30)
	strike := combatTestStrike(t)
	for _, test := range []struct {
		name    string
		intent  PlayerCombatIntent
		action  cultivation.CombatAction
		prep    bool
		want    string
		wantErr error
	}{
		{name: "default spares", intent: "", want: ""},
		{name: "explicit spare", intent: PlayerCombatIntentSpare, want: ""},
		{name: "attack targets live opponent", intent: PlayerCombatIntentTakeLife, action: cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "rival"}, want: "rival"},
		{name: "finishes targeted preparation", intent: PlayerCombatIntentTakeLife, action: cultivation.CombatAction{Kind: cultivation.ActionContinuePreparation}, prep: true, want: "rival"},
		{name: "rest cannot authorize", intent: PlayerCombatIntentTakeLife, action: cultivation.CombatAction{Kind: cultivation.ActionRest}, wantErr: ErrCombatTakeLifeActionRequired},
		{name: "surrender cannot authorize", intent: PlayerCombatIntentTakeLife, action: cultivation.CombatAction{Kind: cultivation.ActionSurrender}, wantErr: ErrCombatTakeLifeActionRequired},
		{name: "unknown target refuses", intent: PlayerCombatIntentTakeLife, action: cultivation.CombatAction{Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "missing"}, wantErr: ErrCombatTakeLifeTargetInvalid},
		{name: "invalid choice refuses", intent: "finish_them", wantErr: ErrCombatPlayerIntentInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := session
			if test.prep {
				current.Preparations = map[string]cultivation.Preparation{DefaultActorID: {Skill: strike, TargetID: "rival"}}
			}
			got, err := playerTakeLifeTarget(current, map[string]cultivation.CombatAction{DefaultActorID: test.action}, test.intent)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("intent error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("target = %q, error = %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestCombatFatalitiesRequireEngineDefeatedCrossingAndFrozenPair(t *testing.T) {
	previous := fatalityTestSession(20)
	settled := fatalityTestSession(0)
	settled.Combatants[0].Defeated = true
	intent := CombatLethalIntent{ActorID: "rival", TargetID: DefaultActorID, SourceTurnID: "turn-risk"}
	step := cultivation.ExchangeStep{
		ActorID: "rival", TargetID: DefaultActorID, Kind: cultivation.ActionTechnique, SkillID: "strike",
		Resolution: cultivation.AttackResolution{HealthLost: 20}, Defeated: true,
	}
	outcome := cultivation.ExchangeOutcome{Round: 3, Ended: true, Steps: []cultivation.ExchangeStep{step}}
	got := combatFatalities(previous, settled, outcome, []CombatLethalIntent{intent}, "combat-round")
	if len(got) != 1 || got[0].ActorID != DefaultActorID || got[0].KillerActorID != "rival" || got[0].SourceEventID != "combat-round" {
		t.Fatalf("qualified engine defeat did not produce sourced fatality: %#v", got)
	}

	withoutIntent := combatFatalities(previous, settled, outcome, nil, "combat-round")
	if len(withoutIntent) != 0 {
		t.Fatalf("an uncommitted or late intent cannot cause death: %#v", withoutIntent)
	}
	withUnregisteredSkill := outcome
	withUnregisteredSkill.Steps = []cultivation.ExchangeStep{step}
	withUnregisteredSkill.Steps[0].SkillID = "model-invented-skill"
	if got := combatFatalities(previous, settled, withUnregisteredSkill, []CombatLethalIntent{intent}, "combat-round"); len(got) != 0 {
		t.Fatalf("an unregistered damage step cannot cause death: %#v", got)
	}
	withoutDefeatedStep := outcome
	withoutDefeatedStep.Steps = []cultivation.ExchangeStep{step}
	withoutDefeatedStep.Steps[0].Defeated = false
	if got := combatFatalities(previous, settled, withoutDefeatedStep, []CombatLethalIntent{intent}, "combat-round"); len(got) != 0 {
		t.Fatalf("HP zero without a formal defeated damage step cannot cause death: %#v", got)
	}
	for _, state := range []struct {
		name    string
		fled    bool
		yielded bool
	}{
		{name: "successful escape", fled: true},
		{name: "accepted surrender", yielded: true},
	} {
		t.Run(state.name, func(t *testing.T) {
			protected := settled
			protected.Combatants = append([]cultivation.Combatant(nil), settled.Combatants...)
			protected.Combatants[0].Fled = state.fled
			protected.Combatants[0].Surrendered = state.yielded
			if got := combatFatalities(previous, protected, outcome, []CombatLethalIntent{intent}, "combat-round"); len(got) != 0 {
				t.Fatalf("nonfatal ending produced a fatality: %#v", got)
			}
		})
	}
}

func TestFrozenLethalIntentsNeverComeFromACombatRound(t *testing.T) {
	lateIntent := CombatLethalIntent{ActorID: "rival", TargetID: DefaultActorID, SourceTurnID: "late-turn"}
	lateRound := CombatRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCombatRecord, ID: "round-1", BranchID: "main",
		Session: cultivation.CombatSession{ID: "battle-1"}, Outcome: cultivation.ExchangeOutcome{Round: 1},
		LethalIntents: []CombatLethalIntent{lateIntent},
	}
	encoded, err := json.Marshal(lateRound)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	record := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeCombatRecord, BranchID: "main"}, Raw: raw}
	if got := frozenLethalIntentsForBattle([]StoryEventRecord{record}, record.Envelope.ID, "battle-1"); len(got) != 0 {
		t.Fatalf("a later round must not establish or upgrade lethal intent: %#v", got)
	}
}

func TestFrozenLethalIntentsStayOnTheSelectedBranchPath(t *testing.T) {
	opening := CombatRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCombatRecord, ID: "left-opening", ParentID: "left-turn", BranchID: "left",
		Session: cultivation.CombatSession{ID: "sibling-battle"}, Outcome: cultivation.ExchangeOutcome{},
		LethalIntents: []CombatLethalIntent{{ActorID: "rival", TargetID: DefaultActorID, SourceTurnID: "risk-turn"}},
	}
	root := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeTurn, ID: "root-turn"}, Raw: map[string]any{"id": "root-turn"}}
	left := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeTurn, ID: "left-turn"}, Raw: map[string]any{"id": "left-turn", "parent_id": "root-turn"}}
	right := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeTurn, ID: "right-turn"}, Raw: map[string]any{"id": "right-turn", "parent_id": "root-turn"}}
	openingRaw, err := json.Marshal(opening)
	if err != nil {
		t.Fatal(err)
	}
	var openingMap map[string]any
	if err := json.Unmarshal(openingRaw, &openingMap); err != nil {
		t.Fatal(err)
	}
	openingRecord := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeCombatRecord, ID: opening.ID}, Raw: openingMap}
	lines := []StoryEventRecord{root, left, right, openingRecord}
	if got := frozenLethalIntentsForBattle(lines, "left-opening", "sibling-battle"); len(got) != 1 {
		t.Fatalf("the branch containing its opening lost the frozen intent: %#v", got)
	}
	if got := frozenLethalIntentsForBattle(lines, "right-turn", "sibling-battle"); len(got) != 0 {
		t.Fatalf("a sibling branch inherited an opening outside its ancestry: %#v", got)
	}
}

func TestCombatFatalityPersistsTerminalAndBranchBoundaryAcrossReopen(t *testing.T) {
	root, store, storyID, riskTurnID, safeTurnID := combatFatalityTestStory(t, DefaultActorID)
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", SessionID: "fatality-battle", Combatants: combatTestSpecs(), CommandID: "fatality-open", CommandHash: "fatality-open-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	intents, err := store.CombatLethalIntents(storyID, "main", opened.Session.ID)
	if err != nil || len(intents) != 1 || intents[0].SourceTurnID != riskTurnID {
		t.Fatalf("opening did not freeze the committed intent: intents=%#v err=%v", intents, err)
	}
	commandID := combatFatalityCommandForHit(opened.Session, "rival", DefaultActorID)
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionRest},
			"rival":        {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: DefaultActorID},
		}, CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(played.Fatalities) != 1 || played.TerminalOutcome == nil || !played.TerminalOutcome.Terminal {
		t.Fatalf("a source-bound fatal engine hit must close the protagonist's branch: %#v", played)
	}
	if played.Fatalities[0].SourceTurnID != riskTurnID || played.Fatalities[0].SourceEventID != played.EventID {
		t.Fatalf("fatality lost its source chain: %#v", played.Fatalities[0])
	}
	replayed, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID, CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil || replayed.EventID != played.EventID || len(replayed.Fatalities) != 1 || replayed.TerminalOutcome == nil {
		t.Fatalf("an idempotent retry must return the same committed fatal result: result=%#v err=%v", replayed, err)
	}
	postDeathFork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: safeTurnID, Title: "死后回到安全节点"})
	if err != nil {
		t.Fatal(err)
	}

	// Reopening reconstructs both the fatal result and terminality from the Story journal.
	reopened := NewStore(root)
	result, found, err := reopened.CombatRecordForCommand(storyID, "main", commandID, battleRoundCommitHash(commandID+"-hash", PlayerCombatIntentSpare))
	if err != nil || !found || len(result.Fatalities) != 1 || result.TerminalOutcome == nil {
		t.Fatalf("cold reopen lost the committed fatal result: result=%#v found=%v err=%v", result, found, err)
	}
	mainSnapshot, err := reopened.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainSnapshot.CurrentTurn == nil || mainSnapshot.CurrentTurn.TerminalOutcome == nil || !mainSnapshot.CurrentTurn.TerminalOutcome.Terminal {
		t.Fatalf("cold reopen lost the terminal overlay: %#v", mainSnapshot.CurrentTurn)
	}
	if snapshot, err := reopened.Snapshot(storyID, postDeathFork.ID); err != nil || snapshot.CurrentTurn == nil || snapshot.CurrentTurn.TerminalOutcome != nil {
		t.Fatalf("a branch created after death from an earlier turn inherited terminality: snapshot=%#v err=%v", snapshot.CurrentTurn, err)
	}
	if _, _, err := reopened.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: postDeathFork.ID, User: "从先前节点改走另一条路", Narrative: "新的分支继续前行。", TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatalf("a branch created after death from a pre-death node must remain playable: %v", err)
	}
	if _, _, err := reopened.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续行动", Narrative: "继续。", TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err == nil {
		t.Fatal("a terminal protagonist branch accepted another turn")
	}
}

func TestNPCCombatFatalityArchivesActorWithoutClosingStoryBranch(t *testing.T) {
	root, store, storyID, _, safeTurnID := combatFatalityTestStory(t, "rival")
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", SessionID: "npc-fatality-battle", Combatants: combatTestSpecs(), CommandID: "npc-fatality-open", CommandHash: "npc-fatality-open-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	commandID := combatFatalityCommandForHit(opened.Session, DefaultActorID, "rival")
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		PlayerIntent: PlayerCombatIntentTakeLife,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(played.Fatalities) != 1 || played.Fatalities[0].ActorID != "rival" || played.TerminalOutcome != nil {
		t.Fatalf("NPC death must be recorded without ending the protagonist's branch: %#v", played)
	}
	history, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil || len(history) != 1 || len(history[0].LethalIntents) != 1 || history[0].LethalIntents[0].ActorID != DefaultActorID || history[0].LethalIntents[0].TargetID != "rival" || history[0].LethalIntents[0].SourceEventID != played.EventID {
		t.Fatalf("the explicit player choice was not recorded with its settled round: history=%#v err=%v", history, err)
	}
	replay, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID, PlayerIntent: PlayerCombatIntentTakeLife,
		Actions:   map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"}},
		CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil || replay.EventID != played.EventID || len(replay.Fatalities) != 1 {
		t.Fatalf("an idempotent retry changed the authorized fatal result: result=%#v err=%v", replay, err)
	}
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID, PlayerIntent: PlayerCombatIntentSpare,
		Actions:   map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"}},
		CommandID: commandID, CommandHash: commandID + "-hash",
	}); !errors.Is(err, ErrCombatCommandConflict) {
		t.Fatalf("reusing a command with a different intent was not refused: %v", err)
	}
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !actorIsArchived(snapshot.State, "rival") {
		t.Fatalf("the killed NPC was not archived: %#v", snapshot.State[actorArchiveRoot])
	}
	present, _ := actorStateFieldValue(snapshot.State, DefaultStoryContextActorID, storyScenePresentField).([]any)
	for _, actorID := range present {
		if actorID == "rival" {
			t.Fatalf("the archived NPC remained present and eligible to participate: %#v", present)
		}
	}
	if snapshot.CurrentTurn != nil && snapshot.CurrentTurn.TerminalOutcome != nil {
		t.Fatal("an NPC fatality must not close the story branch")
	}
	postFightBranch, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: safeTurnID, Title: "另一条路"})
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	reopenedHistory, err := reopened.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil || len(reopenedHistory) != 1 || len(reopenedHistory[0].Fatalities) != 1 || len(reopenedHistory[0].LethalIntents) != 1 {
		t.Fatalf("cold reopen lost the explicit choice and its outcome: history=%#v err=%v", reopenedHistory, err)
	}
	if _, found, err := reopened.CombatRecordForCommand(storyID, postFightBranch.ID, commandID, battleRoundCommitHash(commandID+"-hash", PlayerCombatIntentTakeLife)); err != nil || found {
		t.Fatalf("a sibling branch inherited an uncommitted battle choice: found=%v err=%v", found, err)
	}
}

func combatFatalityTestStory(t *testing.T, targetID string) (string, *Store, string, string, string) {
	t.Helper()
	root := t.TempDir()
	store := NewStore(root)
	system := combatTestSystem()
	for index := range system.Templates {
		if system.Templates[index].ID == ActorStateStoryContextTemplateID {
			system.Templates[index].Fields = append(system.Templates[index].Fields,
				objectStateFieldWithInstruction("scene.engagement", "本次交战", engagementFieldDescription, "当前场景", engagementFieldInstruction),
				listStateField("scene.present_actors", "在场角色", "当前在场人物。", "当前场景"),
			)
		}
	}
	for index := range system.InitialActors {
		if system.InitialActors[index].ID == targetID {
			system.InitialActors[index].State[xiuxianFieldHealth] = float64(1)
		}
	}
	story, err := store.CreateStory(CreateStoryRequest{Title: "正式致命交战", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	safeTurn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "观察对手", Narrative: "主角与对手在山道相遇。", TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	actorID, victimID, user, narrative := DefaultActorID, "rival", "我要杀死对手", "主角已明确要取对手性命。"
	speaker, exchangeCharacterID, exchangeText := "player", "rival", user
	if targetID == DefaultActorID {
		actorID, victimID, user, narrative = "rival", DefaultActorID, "对手扬言取命", "对手喝道：‘我要杀死主角！’"
		speaker, exchangeCharacterID, exchangeText = "character", "rival", "我要杀死主角！"
	}
	engagement := map[string]any{"rival": map[string]any{
		"basis":          "本回合已明确表达取命意图",
		"lethal_intents": []any{map[string]any{"actor_id": actorID, "target_id": victimID, "source_exchange_index": 1}},
	}}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: user, Narrative: narrative,
		TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{
			Profiles: []TurnCharacterProfileInput{{CharacterID: "rival", Name: "对手"}},
			Exchanges: []TurnCharacterExchangeInput{{
				CharacterID: exchangeCharacterID, Speaker: speaker, Channel: "scene", AddressedTo: victimID, Text: exchangeText,
			}},
		}, StateUpdates: []interactivestate.Update{
			{Op: interactivestate.Replace, Path: interactivestate.FormatPath([]string{DefaultStoryContextActorID, storySceneEngagementField}), Value: engagement},
			{Op: interactivestate.Replace, Path: interactivestate.FormatPath([]string{DefaultStoryContextActorID, storyScenePresentField}), Value: []any{DefaultActorID, "rival"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, store, story.ID, turn.ID, safeTurn.ID
}

func combatFatalityCommandForHit(session cultivation.CombatSession, attackerID, targetID string) string {
	attacker, target := session.CombatantByID(attackerID), session.CombatantByID(targetID)
	skill, _ := cultivation.SkillByID("strike")
	chance := cultivation.HitChance(attacker.Attributes, target.Attributes, skill.HitOffset)
	for index := range 500 {
		commandID := fmt.Sprintf("fatality-hit-%s-%d", attackerID, index)
		if combatDrawSource(commandID, session.ID, session.Round)("hit", attackerID, targetID) < chance {
			return commandID
		}
	}
	panic("could not choose a deterministic combat hit")
}

func fatalityTestSession(targetHealth int) cultivation.CombatSession {
	strike, _ := cultivation.SkillByID("strike")
	return cultivation.CombatSession{
		ID: "battle-1",
		Combatants: []cultivation.Combatant{
			{ID: DefaultActorID, Side: cultivation.SidePlayer, Attributes: cultivation.Attributes{Health: targetHealth}},
			{ID: "rival", Side: cultivation.SideEnemy, Attributes: cultivation.Attributes{Health: 100}, Skills: []cultivation.SkillDef{strike}},
		},
	}
}
