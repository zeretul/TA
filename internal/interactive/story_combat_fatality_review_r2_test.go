package interactive

import (
	"encoding/json"
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

// A valid source index must not turn an unrelated scene exchange into consent
// to lethal combat, or change the target named by an NPC's public threat.
func TestReviewSourcedExchangeDoesNotInventPlayerIntentOrRetargetNPC(t *testing.T) {
	for _, test := range []struct {
		name, user, text, actorID, targetID, characterID string
		addressedTo                                      string
		speaker                                          character.Speaker
	}{
		{
			name: "player asks about a threat without choosing lethal action",
			user: "我问对手为什么有人要杀死我。", text: "为什么有人要杀死我？",
			actorID: DefaultActorID, targetID: "rival", characterID: "rival", speaker: character.SpeakerPlayer,
			addressedTo: "rival",
		},
		{
			// The quoted public exchange can exactly match the user's full action
			// while still being a question about a threat, not an attack choice.
			name: "player exact question quote is not lethal authorization",
			user: "我问他：为什么有人要杀死我？", text: "我问他：为什么有人要杀死我？",
			actorID: DefaultActorID, targetID: "rival", characterID: "rival", speaker: character.SpeakerPlayer,
			addressedTo: "rival",
		},
		{
			name: "NPC threatens a bystander and tells the protagonist to leave",
			user: "留意动静", text: "我要杀死那名旁观者！主角，你先走，这事与你无关。",
			actorID: "rival", targetID: DefaultActorID, characterID: "rival", speaker: character.SpeakerCharacter,
			addressedTo: "bystander",
		},
		{
			// The model wrote a player line the player never sent. A line it
			// authored on the player's behalf cannot prove the player chose this.
			name: "player line absent from the player's own action",
			user: "我后退一步，观察对手。", text: "我要杀死你。",
			actorID: DefaultActorID, targetID: "rival", characterID: "rival", speaker: character.SpeakerPlayer,
			addressedTo: "rival",
		},
		{
			// No recorded direction: the words may have been aimed anywhere, so
			// the ambiguity stays nonfatal.
			name: "NPC line with no recorded direction",
			user: "留意动静", text: "我要杀死你。",
			actorID: "rival", targetID: DefaultActorID, characterID: "rival", speaker: character.SpeakerCharacter,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := TurnEvent{
				V: 1, Type: StoryEventTypeTurn, ID: "review-r2-turn", User: test.user, Narrative: test.text,
				StateDelta: &StateDelta{Ops: []interactivestate.Op{{
					Op: "set", Path: "/story/本次交战", SourceTurnID: "review-r2-turn", SourceKind: interactivestate.SourceTurnResult,
					Value: map[string]any{"rival": map[string]any{"lethal_intents": []any{map[string]any{
						"actor_id": test.actorID, "target_id": test.targetID, "source_exchange_index": 1,
					}}}},
				}}},
			}
			characters := CharacterRecordEvent{
				V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: "review-r2-characters", ParentID: turn.ID, TurnID: turn.ID,
				Exchanges: []character.Exchange{{
					ID: "review-r2-source", TurnID: turn.ID, CharacterID: test.characterID,
					Speaker: test.speaker, Channel: character.ChannelScene, AddressedTo: test.addressedTo, Text: test.text,
				}},
			}
			var records []StoryEventRecord
			for _, value := range []any{turn, characters} {
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				if err := json.Unmarshal(encoded, &raw); err != nil {
					t.Fatal(err)
				}
				records = append(records, StoryEventRecord{Envelope: StoryEventEnvelope{
					ID: raw["id"].(string), Type: raw["type"].(string),
				}, Raw: raw})
			}
			got := freezeCombatLethalIntents(records, characters.ID, fatalityTestSession(30))
			if len(got) != 0 {
				t.Fatalf("an unrelated committed exchange established directed lethal intent: %#v", got)
			}
		})
	}
}

// The engagement contract allows the declaration list under each actor's own key
// or once at the top level beside the per-actor basis strings. Both are the same
// declaration; only the nested shape used to be decoded, so a real model that
// wrote the flat one had its intent dropped and no fight could become fatal.
func TestReviewEngagementAcceptsEitherDeclarationShape(t *testing.T) {
	const (
		turnID    = "review-r2-shape-turn"
		user      = "我观察对手的动作。"
		utterance = "我要杀死你！"
	)
	declarations := []any{map[string]any{
		"actor_id": "rival", "target_id": DefaultActorID, "source_exchange_index": 1,
	}}
	for _, test := range []struct {
		name       string
		engagement map[string]any
	}{
		{
			name:       "list under the actor's own key",
			engagement: map[string]any{"rival": map[string]any{"basis": "已明确取命", "lethal_intents": declarations}},
		},
		{
			name:       "list at the top level beside the per-actor basis",
			engagement: map[string]any{"lethal_intents": declarations, "rival": "已被制住"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := TurnEvent{
				V: 1, Type: StoryEventTypeTurn, ID: turnID, User: user, Narrative: "主角抽刀扑向对手。",
				StateDelta: &StateDelta{Ops: []interactivestate.Op{{
					Op: "set", Path: "/story/本次交战", SourceTurnID: turnID, SourceKind: interactivestate.SourceTurnResult,
					Value: test.engagement,
				}}},
			}
			characters := CharacterRecordEvent{
				V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: "review-r2-shape-characters", ParentID: turnID, TurnID: turnID,
				Exchanges: []character.Exchange{{
					ID: "review-r2-shape-source", TurnID: turnID, CharacterID: "rival",
					Speaker: character.SpeakerCharacter, Channel: character.ChannelScene, AddressedTo: DefaultActorID, Text: utterance,
				}},
			}
			var records []StoryEventRecord
			for _, value := range []any{turn, characters} {
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				if err := json.Unmarshal(encoded, &raw); err != nil {
					t.Fatal(err)
				}
				records = append(records, StoryEventRecord{Envelope: StoryEventEnvelope{
					ID: raw["id"].(string), Type: raw["type"].(string),
				}, Raw: raw})
			}
			got := freezeCombatLethalIntents(records, characters.ID, fatalityTestSession(30))
			if len(got) != 1 || got[0].ActorID != "rival" || got[0].TargetID != DefaultActorID {
				t.Fatalf("the sourced declaration was not frozen: %#v", got)
			}
		})
	}
}
