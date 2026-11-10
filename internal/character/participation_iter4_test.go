package character

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestIter4ParticipationDecisionsKeepTheirOwnSourceAcrossProjectionAndJSON(t *testing.T) {
	for _, kind := range []string{ParticipationCombat, ParticipationJourney} {
		for _, answer := range []string{ParticipationAccept, ParticipationRefuse} {
			t.Run(kind+"/"+answer, func(t *testing.T) {
				decision := &ParticipationDecision{Kind: kind, TargetID: "target", Decision: answer}
				projected, err := Project(ProjectionContext{StoryID: "story", BranchID: "main", WorldTime: "now", CommitID: "commit", TurnID: "turn",
					KnownCharacters: []string{"beast"}, Scene: Scene{NodeID: "scene", Present: []string{"beast"}}, NextID: func(prefix string) string { return prefix + "-1" }},
					TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "beast", Speaker: SpeakerCharacter, Channel: ChannelScene, AddressedTo: "protagonist", Text: "（轻鸣回应。）", ParticipationDecision: decision}}})
				if err != nil {
					t.Fatal(err)
				}
				x := projected.Exchanges[0]
				if x.SourceEventID != "commit" || x.TurnID != "turn" || x.ParticipationDecision == nil || *x.ParticipationDecision != *decision {
					t.Fatalf("projection lost the speaker's sourced answer: %+v", x)
				}
				encoded, err := json.Marshal(x)
				if err != nil {
					t.Fatal(err)
				}
				var restored Exchange
				if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored, x) {
					t.Fatalf("JSON recovery changed the answer: err=%v", err)
				}
				copy := x.Normalize()
				copy.ParticipationDecision.Decision = "changed"
				if x.ParticipationDecision.Decision != answer || decision.Decision != answer {
					t.Fatal("a view mutated the original decision")
				}
			})
		}
	}
}

func TestIter4ParticipationDecisionCannotBeAPlayerCommandOrRemoteMessage(t *testing.T) {
	for _, test := range []struct {
		name    string
		speaker Speaker
		channel Channel
		dir     string
	}{
		{"player", SpeakerPlayer, ChannelScene, "protagonist"},
		{"private", SpeakerCharacter, ChannelPrivate, "protagonist"},
		{"remote", SpeakerCharacter, ChannelMessage, "protagonist"},
		{"undirected", SpeakerCharacter, ChannelScene, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			x := Exchange{ID: "x", StoryID: "s", BranchID: "b", CharacterID: "beast", Speaker: test.speaker, Channel: test.channel, AddressedTo: test.dir, Text: "response", WorldTime: "now", Audience: []string{"beast"}, ParticipationDecision: &ParticipationDecision{Kind: ParticipationCombat, TargetID: "opponent", Decision: ParticipationAccept}}
			if err := x.Validate(); err == nil {
				t.Fatal("an unsupported speaker or channel established a participation decision")
			}
		})
	}
}
