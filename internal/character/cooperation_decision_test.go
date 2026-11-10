package character

import (
	"encoding/json"
	"reflect"
	"testing"
)

func cooperationTestDecisionExchange(t *testing.T, speaker Speaker, channel Channel, decision map[string]any) Exchange {
	t.Helper()
	raw := map[string]any{
		"id": "exchange-agree", "story_id": "story-1", "branch_id": "branch-1",
		"character_id": "steward", "speaker": speaker, "channel": channel,
		"text":       "I can help review those notes.",
		"world_time": "day 1", "audience": []string{"steward"}, "cooperation_decision": decision,
	}
	if channel == ChannelGroup {
		raw["group_id"], raw["author_id"] = "group-1", "steward"
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var exchange Exchange
	if err := json.Unmarshal(data, &exchange); err != nil {
		t.Fatal(err)
	}
	return exchange
}

func cooperationTestDecision() map[string]any {
	return map[string]any{
		"project_id": "coop.herbcave.notes", "realm_instance_id": "realm-instance-1", "decision": "accept",
	}
}

func TestCooperationDecisionExchangeJSONRoundTrip(t *testing.T) {
	decision := cooperationTestDecision()
	exchange := cooperationTestDecisionExchange(t, SpeakerCharacter, ChannelPrivate, decision)
	if err := exchange.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(exchange.Normalize())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw["cooperation_decision"], decision) {
		t.Fatalf("exchange lost its scoped decision: got %#v, want %#v", raw["cooperation_decision"], decision)
	}
}

func TestCooperationDecisionRejectsWrongSpeakerOrChannel(t *testing.T) {
	for _, speaker := range []Speaker{SpeakerPlayer, SpeakerCharacter} {
		for _, channel := range []Channel{ChannelScene, ChannelPrivate, ChannelMessage, ChannelGroup} {
			valid := speaker == SpeakerCharacter && (channel == ChannelScene || channel == ChannelPrivate)
			err := cooperationTestDecisionExchange(t, speaker, channel, cooperationTestDecision()).Validate()
			if (err == nil) != valid {
				t.Errorf("speaker=%s channel=%s err=%v, valid=%t", speaker, channel, err, valid)
			}
		}
	}
}

func TestCooperationDecisionRejectsMissingScopeOrUnknownAnswer(t *testing.T) {
	for _, field := range []string{"project_id", "realm_instance_id", "decision"} {
		decision := cooperationTestDecision()
		decision[field] = " \t "
		if err := cooperationTestDecisionExchange(t, SpeakerCharacter, ChannelPrivate, decision).Validate(); err == nil {
			t.Errorf("decision missing %s was accepted", field)
		}
	}
	decision := cooperationTestDecision()
	decision["decision"] = "already-worked"
	if err := cooperationTestDecisionExchange(t, SpeakerCharacter, ChannelScene, decision).Validate(); err == nil {
		t.Error("unknown decision value was accepted")
	}
}

func TestCooperationDecisionProjectionPreservesCandidateNotWork(t *testing.T) {
	for _, decision := range []string{CooperationAccept, CooperationRefuse} {
		for _, channel := range []Channel{ChannelScene, ChannelPrivate} {
			candidate := &CooperationDecision{ProjectID: " coop.herbcave.notes ", RealmInstanceID: " realm-instance-1 ", Decision: " " + decision + " "}
			draft := ExchangeDraft{
				CharacterID: "steward", Speaker: SpeakerCharacter, Channel: channel,
				Text: "Here is my answer about reviewing those notes.", AuthorID: "guardsman",
				CooperationDecision: candidate,
			}
			// Model JSON may express a scoped answer, but it contains no costs,
			// capabilities, result or alternative character identity to apply.
			encoded, err := json.Marshal(draft)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ExchangeDraft
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			projected, err := Project(testProjectionContext(), TurnCharacterFacts{Exchanges: []ExchangeDraft{decoded}})
			if err != nil {
				t.Fatal(err)
			}
			if len(projected.Exchanges) != 1 || len(projected.Matters) != 0 || len(projected.Knowledge) != 0 || len(projected.Affinities) != 0 {
				t.Fatalf("a response generated work or other consequences: %#v", projected)
			}
			exchange := projected.Exchanges[0]
			want := candidate.Normalize()
			if exchange.CooperationDecision == nil || *exchange.CooperationDecision != want {
				t.Fatalf("decision changed during projection: %#v", exchange)
			}
			if exchange.CharacterID != "steward" || exchange.AuthorID != "" || exchange.ID != "xch-a" || exchange.SourceEventID != "commit-7" || exchange.StoryID != "story-1" || exchange.BranchID != "branch-1" || exchange.TurnID != "turn-7" {
				t.Fatalf("decision lost source or was attributed to another character: %#v", exchange)
			}
			exchange.CooperationDecision.Decision = "changed"
			if decoded.CooperationDecision.Decision != " "+decision+" " {
				t.Fatal("projected decision aliases its draft")
			}
			decoded.CharacterID = "unknown"
			if _, err := Project(testProjectionContext(), TurnCharacterFacts{Exchanges: []ExchangeDraft{decoded}}); err == nil {
				t.Fatal("decision bypassed existing character validation")
			}
		}
	}
}

func TestCooperationDecisionDraftCannotCarryWritableCostsOrActors(t *testing.T) {
	var draft ExchangeDraft
	if err := json.Unmarshal([]byte(`{"id":"invented-exchange","story_id":"invented-story","branch_id":"invented-branch","source_event_id":"invented-source","character_id":"steward","speaker":"character","channel":"private","text":"I agree to review those notes.","cooperation_decision":{"project_id":"coop.herbcave.notes","realm_instance_id":"realm-instance-1","decision":"accept","character_id":"guardsman","costs":{"energy":0},"capabilities":["free-work"],"work_source_ref":"invented"}}`), &draft); err != nil {
		t.Fatal(err)
	}
	projected, err := Project(testProjectionContext(), TurnCharacterFacts{Exchanges: []ExchangeDraft{draft}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projected.Exchanges[0].CooperationDecision)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw, cooperationTestDecision()) || projected.Exchanges[0].CharacterID != "steward" || len(projected.Matters) != 0 {
		t.Fatalf("candidate retained executable or impersonated fields: %s, %#v", encoded, projected)
	}
	exchange := projected.Exchanges[0]
	if exchange.ID != "xch-a" || exchange.StoryID != "story-1" || exchange.BranchID != "branch-1" || exchange.SourceEventID != "commit-7" {
		t.Fatalf("draft overrode the real response identity/source: %#v", exchange)
	}
}

func TestCooperationDecisionDoesNotBackfillLegacyExchanges(t *testing.T) {
	var old Exchange
	if err := json.Unmarshal([]byte(`{"id":"legacy-exchange","story_id":"story-1","branch_id":"branch-1","character_id":"steward","speaker":"character","channel":"private","text":"Ordinary past speech.","world_time":"day 1","audience":["steward"]}`), &old); err != nil {
		t.Fatal(err)
	}
	if err := old.Validate(); err != nil {
		t.Fatalf("legacy exchange is no longer readable: %v", err)
	}
	recovered := (State{Exchanges: []Exchange{old}}).Normalize().Exchanges[0]
	if recovered.CooperationDecision != nil || recovered.SourceEventID != "" || !reflect.DeepEqual(recovered, old) {
		t.Fatalf("legacy speech received invented consent or a backfilled source: %#v", recovered)
	}
}

func TestCooperationDecisionViewsAndRecoveryDoNotAlias(t *testing.T) {
	for _, view := range []struct {
		name string
		read func(Exchange) Exchange
	}{
		{"normalize", func(e Exchange) Exchange { return e.Normalize() }},
		{"state", func(e Exchange) Exchange { return (State{Exchanges: []Exchange{e}}).Normalize().Exchanges[0] }},
		{"for-character", func(e Exchange) Exchange { return (State{Exchanges: []Exchange{e}}).ExchangesFor("steward")[0] }},
	} {
		t.Run(view.name, func(t *testing.T) {
			original := cooperationTestDecisionExchange(t, SpeakerCharacter, ChannelPrivate, cooperationTestDecision())
			encoded, _ := json.Marshal(original)
			copy := view.read(original)
			copy.CooperationDecision.Decision = CooperationRefuse
			after, _ := json.Marshal(original)
			if string(encoded) != string(after) {
				t.Fatal("editing a decision view changed the original exchange")
			}
			var recovered Exchange
			if err := json.Unmarshal(encoded, &recovered); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(recovered, original) {
				t.Fatalf("JSON recovery changed the decision: %#v", recovered)
			}
		})
	}
}
