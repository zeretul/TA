package character

import (
	"strings"
	"testing"
)

func TestRelationshipProposalRequiresTwoDirectedParticipants(t *testing.T) {
	player := ExchangeDraft{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我愿与你结为道侣。"}
	npc := ExchangeDraft{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "我也愿意。"}
	for _, test := range []struct {
		name          string
		lines         []ExchangeDraft
		offer, answer int
	}{
		{"same line twice", []ExchangeDraft{npc}, 1, 1},
		{"only character speaks", []ExchangeDraft{npc, npc}, 1, 2},
		{"only player speaks", []ExchangeDraft{player, player}, 1, 2},
		{"answer predates offer", []ExchangeDraft{player, npc}, 2, 1},
		{"character addresses someone else", []ExchangeDraft{player, {CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelScene, AddressedTo: "steward", Text: "我也愿意。"}}, 1, 2},
		{"player addresses someone else", []ExchangeDraft{{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelScene, AddressedTo: "steward", Text: "我愿与你结为道侣。"}, npc}, 1, 2},
		{"ambiguous public reply", []ExchangeDraft{player, {CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelScene, Text: "我也愿意。"}}, 1, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Project(testProjectionContextWithHighAffinity(), TurnCharacterFacts{
				Exchanges:     test.lines,
				Relationships: []RelationshipDraft{{CharacterID: "guardsman", Kind: RelationshipPartner, Statement: "两人结为道侣", ExpressedByIndex: test.offer, AnsweredByIndex: test.answer}},
			})
			if err == nil {
				t.Fatal("unsupported relationship proposal was accepted")
			}
			if !strings.Contains(err.Error(), "relationship") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRelationshipProposalAcceptsActualParticipantsWithPendingIDs(t *testing.T) {
	for _, channel := range []Channel{ChannelPrivate, ChannelScene} {
		for _, first := range []Speaker{SpeakerPlayer, SpeakerCharacter} {
			t.Run(string(channel)+"/"+string(first), func(t *testing.T) {
				ctx := testProjectionContextWithHighAffinity()
				ctx.NextID = func(prefix string) string { return prefix + "_pending" }
				player := ExchangeDraft{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: channel, Text: "我愿意。"}
				npc := ExchangeDraft{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: channel, Text: "我也愿意。"}
				if channel == ChannelScene {
					player.AddressedTo, npc.AddressedTo = "guardsman", "protagonist"
				}
				lines := []ExchangeDraft{player, npc}
				if first == SpeakerCharacter {
					lines = []ExchangeDraft{npc, player}
				}
				got, err := Project(ctx, TurnCharacterFacts{Exchanges: lines, Relationships: []RelationshipDraft{{
					CharacterID: "guardsman", Kind: RelationshipPartner, Statement: "两人结为道侣", ExpressedByIndex: 1, AnsweredByIndex: 2,
				}}})
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Relationships) != 1 {
					t.Fatalf("legitimate bond missing: %+v", got)
				}
			})
		}
	}
}

func TestRelationshipPartingRemainsUnilateralAndDirected(t *testing.T) {
	for _, kind := range []RelationshipKind{RelationshipEstranged, RelationshipHostile} {
		for _, speaker := range []Speaker{SpeakerCharacter, SpeakerPlayer} {
			ctx := testProjectionContext()
			target := "guardsman"
			if speaker == SpeakerCharacter {
				target = "protagonist"
			}
			facts := TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "guardsman", Speaker: speaker, Channel: ChannelScene, AddressedTo: target, Text: "从此各行其路。"}},
				Relationships: []RelationshipDraft{{CharacterID: "guardsman", Kind: kind, Statement: "双方决裂", ExpressedByIndex: 1}}}
			got, err := Project(ctx, facts)
			if err != nil || len(got.Relationships) != 1 {
				t.Fatalf("unilateral parting: %+v %v", got, err)
			}
			facts.Exchanges[0].AddressedTo = "steward"
			if _, err := Project(ctx, facts); err == nil {
				t.Fatal("a parting with someone else changed the protagonist relationship")
			}
		}
	}
}

func TestRelationshipProposalUsesLatestBondWithinSameTurn(t *testing.T) {
	for _, initial := range []RelationshipKind{RelationshipCloseFriend, RelationshipEstranged} {
		t.Run(string(initial), func(t *testing.T) {
			ctx := testProjectionContextWithHighAffinity()
			ctx.ExistingRelationships = []Relationship{{CharacterID: "guardsman", Kind: initial}}
			lines := []ExchangeDraft{
				{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "我们不再是朋友。"},
				{CharacterID: "guardsman", Speaker: SpeakerPlayer, Channel: ChannelPrivate, Text: "误会已说清，我们仍愿相交吗？"},
				{CharacterID: "guardsman", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "好，我们重新相交。"},
			}
			parting := RelationshipDraft{CharacterID: "guardsman", Kind: RelationshipEstranged, Statement: "双方断交", ExpressedByIndex: 1}
			bond := RelationshipDraft{CharacterID: "guardsman", Kind: RelationshipCloseFriend, Statement: "双方重新结交", ExpressedByIndex: 2, AnsweredByIndex: 3}
			drafts := []RelationshipDraft{parting, bond}
			if initial == RelationshipEstranged {
				lines = append(lines[1:], lines[0])
				bond.ExpressedByIndex, bond.AnsweredByIndex, parting.ExpressedByIndex = 1, 2, 3
				drafts = []RelationshipDraft{bond, parting}
			}
			got, err := Project(ctx, TurnCharacterFacts{Exchanges: lines, Relationships: drafts})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Relationships) != 2 || got.Relationships[1].Kind != initial {
				t.Fatalf("latest change was dropped against the pre-turn state: %+v", got.Relationships)
			}
		})
	}
}
