package interactive

import (
	"ta/internal/character"
	"testing"
)

func TestLetterReplyUsesValidatedWaitClock(t *testing.T) {
	system, state := turnSubmissionTestState()
	clock := NewStoryClock("初一晨")
	validation := TurnSubmissionContext{
		ActorState: system, CurrentState: state, CurrentPlayerInput: "我等一天再读周伯的回信", CommunicationMode: "letter",
		KnownCharacters: []string{"周伯"}, WorldView: &TurnWorldView{Clock: &clock},
		ExistingMatters: []character.Matter{{ID: "mat_sent", CharacterID: "周伯", Kind: character.MatterAction, Status: character.MatterOpen,
			Letter: &character.LetterDelivery{Body: "请告知开门时辰", EarliestDeliverySeconds: 86400}}},
	}
	for _, tc := range []struct {
		name     string
		duration int64
		source   string
		accepted bool
	}{
		{"arrival", 86400, "等一天", true}, {"too early", 86399, "等一天", false}, {"unauthorized wait", 86400, "等两天", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := TurnSubmissionInput{World: &TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: tc.duration, SourceText: tc.source},
				CharacterFacts: &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{CharacterID: "周伯", Kind: "action", Status: "settled", Resolves: "mat_sent", Statement: "周伯的回信已送到。", LetterReply: &character.LetterReply{Body: "辰时开门。"}}}}}
			prepared, receipt := PrepareTurnSubmission(validation, nil, input)
			got := prepared.TurnResult().CharacterFacts != nil
			if got != tc.accepted {
				t.Fatalf("reply accepted=%v want %v, receipt=%+v", got, tc.accepted, receipt)
			}
		})
	}
}
