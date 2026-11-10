package interactive

import (
	"reflect"
	"testing"

	"ta/internal/character"
)

func TestOrdinaryDelayedMatterUsesStoryClockAndKeepsFailureAtomic(t *testing.T) {
	store, id := questTestStory(t)
	request := AppendCharacterFactsRequest{BranchID: "main", WorldTime: "当前", Scene: character.Scene{Present: []string{"su-qian"}}, CommandID: "agree", CommandHash: "agree", Facts: character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{{CharacterID: "su-qian", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, Text: "过一日我再把手边的旧记录拿来核对。"}}, Matters: []character.MatterDraft{{CharacterID: "su-qian", Kind: character.MatterPromise, Status: character.MatterOpen, Statement: "苏浅约定一日后复查手边记录。", AfterSeconds: 86400, SourceExchangeIndex: 1}}}}
	agreed, err := store.AppendCharacterFacts(id, request)
	if err != nil {
		t.Fatal(err)
	}
	matter := agreed.Matters[0]
	repeat, err := store.AppendCharacterFacts(id, request)
	if err != nil || repeat.Matters[0].ID != matter.ID {
		t.Fatalf("agreement replay changed identity: %v", err)
	}
	before, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	callback := AppendTurnWithStateRequest{BranchID: "main", User: "问她核对好了没有", Narrative: "她拿出手边的旧记录，说明核对的结果。", TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "su-qian", Speaker: "character", Channel: "scene", Text: "这是我刚核对过的记录。"}}, Matters: []TurnCharacterMatterInput{{CharacterID: "su-qian", Kind: "action", Status: "settled", Statement: "苏浅拿出核对后的记录。", Resolves: matter.ID, SourceExchangeIndex: 1}}}}}
	if _, _, err := store.AppendTurnWithState(id, callback); err == nil {
		t.Fatal("early natural-language confirmation accepted")
	}
	after, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Clock, after.Clock) || before.TurnCount != after.TurnCount || before.CurrentTurn.ID != after.CurrentTurn.ID {
		t.Fatal("failed response left partial state or time")
	}
	questWait(t, store, id, "main", 86400)
	state, err := store.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if state.Matters[0].Status != character.MatterOpen {
		t.Fatal("maturity automatically completed the promise")
	}
	if _, _, err := store.AppendTurnWithState(id, callback); err != nil {
		t.Fatal(err)
	}
	state, err = store.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	original, ok := character.MatterByID(state.Matters, matter.ID)
	if !ok || original.Status != character.MatterSettled || original.EarliestSeconds != 86400 {
		t.Fatal("actual callback did not consume its original delayed promise")
	}
}
