package app

import (
	"testing"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

func TestCombatWindowShowsFrozenLethalRisk(t *testing.T) {
	application, storyID := combatTestApp(t)
	if _, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "询问对手意图", Narrative: "对手喝道：‘我要杀死主角！’",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{CharacterID: "rival", Name: "对手"}},
				Exchanges: []interactive.TurnCharacterExchangeInput{{
					CharacterID: "rival", Speaker: "character", Channel: "scene",
					AddressedTo: interactive.DefaultActorID, Text: "我要杀死主角！",
				}},
			},
			StateUpdates: []interactivestate.Update{{
				Op:   interactivestate.Replace,
				Path: interactivestate.FormatPath([]string{interactive.DefaultStoryContextActorID, "本次交战"}),
				Value: map[string]any{"rival": map[string]any{"basis": "公开取命威胁", "lethal_intents": []any{map[string]any{
					"actor_id": "rival", "target_id": interactive.DefaultActorID, "source_exchange_index": 1,
				}}}},
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := application.interactive.StartBattle(storyID, interactive.BattleStartRequest{
		BranchID: "main", SessionID: "window-lethal-battle", Combatants: combatTestSpecs(), CommandID: "window-lethal-open", CommandHash: "window-lethal-open-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: "main", SessionID: opened.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !window.LethalRisk {
		t.Fatal("the battle window must show a frozen lethal threat to the player's side")
	}
}
