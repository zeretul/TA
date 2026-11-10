package interactive

import (
	"ta/internal/character"
	"testing"
)

func TestWrittenReplyCanFollowReceiptInALaterTurn(t *testing.T) {
	existing := []character.Matter{{
		ID: "mat_sent", CharacterID: "周伯", Kind: character.MatterAction, Status: character.MatterSettled,
		SettledBy: "mat_receipt", Letter: &character.LetterDelivery{Body: "请告知驿站开门时辰", EarliestDeliverySeconds: 86400},
	}, {
		ID: "mat_receipt", CharacterID: "周伯", Kind: character.MatterAction, Status: character.MatterSettled,
		SettledBy: "mat_sent", Statement: "周伯签收了来信，还没有回信。",
	}}
	reply := &TurnCharacterFactsInput{Matters: []TurnCharacterMatterInput{{
		CharacterID: "周伯", Kind: "action", Status: "settled", Resolves: "mat_sent",
		Statement: "周伯的书面回信已经送来。", LetterReply: &character.LetterReply{Body: "辰时开门。"},
	}}}
	if diagnostic := validateLetterMatterSettlements(reply, existing, 172800); diagnostic != nil {
		t.Fatalf("a delivered letter cannot be answered in a later turn: %+v", diagnostic)
	}
	projected := character.Matter{ID: "mat_reply", CharacterID: "周伯", Kind: character.MatterAction, Status: character.MatterSettled,
		SettledBy: "mat_sent", Statement: "周伯的书面回信已经送来。", LetterReply: &character.LetterReply{Body: "辰时开门。"}}
	result := character.ApplyMatters(existing, []character.Matter{projected})
	if len(result) != 3 || result[0].SettledBy != "mat_receipt" || result[2].SettledBy != "mat_sent" || result[2].LetterReply.Body != "辰时开门。" {
		t.Fatalf("the separate receipt and written reply lost their source links: %+v", result)
	}
	if diagnostic := validateLetterMatterSettlements(reply, result, 172800); diagnostic == nil {
		t.Fatal("another command could append a second reply to an already answered letter")
	}
	wrongRecipient := *reply
	wrongRecipient.Matters = append([]TurnCharacterMatterInput(nil), reply.Matters...)
	wrongRecipient.Matters[0].CharacterID = "别人"
	if diagnostic := validateLetterMatterSettlements(&wrongRecipient, existing, 172800); diagnostic == nil {
		t.Fatal("another person answered a delivered letter")
	}
	existing[0].Status = character.MatterAbandoned
	if diagnostic := validateLetterMatterSettlements(reply, existing, 172800); diagnostic == nil {
		t.Fatal("an abandoned letter became an answered letter")
	}
}
