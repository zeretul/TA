package interactive

import (
	"ta/internal/character"
	"testing"
)

func TestCharacterRadioContactDraftCloneIsIndependent(t *testing.T) {
	input := &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{RadioContact: &character.RadioContact{DeviceItemID: "terminal", Channel: "agreed", Status: "connected"}}}}
	accepted := cloneTurnCharacterFactsInput(input)
	input.Profiles[0].RadioContact.Status = "offline"
	if accepted.Profiles[0].RadioContact.Status != "connected" {
		t.Fatal("later draft changed accepted contact")
	}
	accepted.Profiles[0].RadioContact.Channel = "other"
	if input.Profiles[0].RadioContact.Channel != "agreed" {
		t.Fatal("accepted draft shares contact with caller")
	}
}
