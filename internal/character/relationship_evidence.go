package character

import "fmt"

// validateRelationshipExchange keeps a protagonist relationship attached to
// words directed between its two participants. Being able to overhear a bond
// offered to someone else does not make the protagonist a party to it. Private
// dialogue already fixes the counterpart; public dialogue must name them.
// This only validates new proposals, not historical journal records. Whether
// the words actually express agreement remains the narrator's judgement.
func validateRelationshipExchange(exchange Exchange, characterID string) error {
	target := characterID
	if exchange.Speaker == SpeakerCharacter {
		target = "protagonist"
	}
	if exchange.AddressedTo != "" && exchange.AddressedTo != target {
		return fmt.Errorf("cited relationship speech must address %q, not %q", target, exchange.AddressedTo)
	}
	switch exchange.Channel {
	case ChannelPrivate:
		// The character window owns both sides of this one-to-one exchange.
		return nil
	case ChannelScene:
		if exchange.AddressedTo == "" {
			return fmt.Errorf("cited public relationship speech must explicitly address %q", target)
		}
		return nil
	case ChannelMessage:
		// An incoming note can establish one person's declaration or parting,
		// but it cannot manufacture a response from its recipient.
		if exchange.Speaker != SpeakerCharacter {
			return fmt.Errorf("an incoming message cannot be the protagonist's relationship response")
		}
		return nil
	case ChannelGroup:
		return fmt.Errorf("a group exchange does not establish a protagonist relationship")
	default:
		return fmt.Errorf("unsupported relationship speech channel %q", exchange.Channel)
	}
}
