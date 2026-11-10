package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/character"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

func TestLoreKnowledgeProjectionRejectsUnknownAndMixedSources(t *testing.T) {
	facts := map[string]any{
		"known":  map[string]any{"kind": "同门", "source": "conversation", "nature": "told", "confidence": "uncertain", "learned_at": "第三日", "evidence": "PRIVATE_EVIDENCE"},
		"hidden": map[string]any{"kind": "UNKNOWN_TARGET", "source": "conversation"},
		"mixed":  map[string]any{"kind": "MIXED_SOURCE", "source": []any{"conversation", "private"}},
	}
	raw := map[string]any{"actors": map[string]any{"npc": map[string]any{"template_id": "important_character", "state": map[string]any{
		"已知人物关系": facts, "已知位置": map[string]any{"location": "码头", "source": "observation", "nature": "experienced", "evidence": "PRIVATE_LOCATION_EVIDENCE"},
	}}}}
	before, _ := json.Marshal(raw)
	projected := playerKnownState(raw, nil, loreKnowledgeAudience{known: map[string]bool{"npc": true, "known": true, "mixed": true}})
	data, _ := json.Marshal(projected)
	for _, secret := range []string{"PRIVATE", "UNKNOWN_TARGET", "MIXED_SOURCE", "hidden"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret %q escaped: %s", secret, data)
		}
	}
	for _, public := range []string{"同门", "码头", "第三日", "uncertain"} {
		if !strings.Contains(string(data), public) {
			t.Fatalf("public fact %q missing: %s", public, data)
		}
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("projection mutated canonical state")
	}
	empty, _ := json.Marshal(playerKnownState(raw, nil))
	if strings.Contains(string(empty), "同门") || strings.Contains(string(empty), "码头") {
		t.Fatalf("missing known cast failed open: %s", empty)
	}
}

func TestLoreKnowledgeProjectionRemovesTurnWriteChannels(t *testing.T) {
	secret := map[string]any{"hidden": "PRIVATE_TARGET"}
	turn := interactive.TurnEvent{
		StateDelta: &interactive.StateDelta{
			ActorOps: []interactive.ActorStateOp{{ActorID: "npc", FieldID: "已知人物关系", Value: secret, PreviousValue: secret, Reason: "PRIVATE_REASON"}, {ActorID: "protagonist", FieldID: "生命", Value: 10}},
			Ops:      []interactivestate.Op{{Path: "/actors/npc/state/已知位置", Value: "PRIVATE_LOCATION"}, {Path: "/actors/npc", Value: map[string]any{"state": map[string]any{"已知人物关系": secret}}}},
		},
		TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Path: "/npc/已知人物关系", Value: secret}, {Path: "/npc", Value: map[string]any{"state": map[string]any{"已知位置": "PRIVATE_CREATE"}}}}},
		RuleResolution: &interactive.RuleResolution{
			Result:           interactive.RuleResult{StateChanges: []interactive.TurnStateChange{{ActorID: "npc", FieldID: "已知人物关系", Reason: "PRIVATE_CHANGE"}}},
			StateConsumption: &interactive.RuleStateConsumption{AppliedActorOps: []interactive.ActorStateOp{{ActorID: "protagonist", FieldID: "已知位置", Value: "PRIVATE_RULE_LOCATION"}}},
		},
	}
	before, _ := json.Marshal(turn)
	projected := playerKnownTurn(turn, nil)
	data, _ := json.Marshal(projected)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatalf("turn leaked knowledge: %s", data)
	}
	if len(projected.StateDelta.ActorOps) != 1 {
		t.Fatal("unrelated public operation disappeared")
	}
	after, _ := json.Marshal(turn)
	if string(before) != string(after) {
		t.Fatal("turn projection mutated canonical state")
	}
	snapshot := interactive.Snapshot{Turns: []interactive.TurnEvent{turn}, CurrentTurn: &turn}
	projectInteractiveSnapshotForPlayer(&snapshot)
	if !reflect.DeepEqual(snapshot.Turns[0], projected) || !reflect.DeepEqual(*snapshot.CurrentTurn, projected) {
		t.Fatal("snapshot/history/current projections diverged")
	}
	// Even the original snapshot slice must remain suitable for replay.
	if string(before) != string(after) {
		t.Fatal("canonical turn changed")
	}
}

func TestLoreKnowledgeEvidenceRequiresEveryReferenceToBePlayerVisible(t *testing.T) {
	audience := loreAudience(character.State{Profiles: []character.Profile{{ID: "npc", Name: "NPC"}}, Exchanges: []character.Exchange{
		{ID: "public", Channel: character.ChannelScene, Speaker: character.SpeakerCharacter, Audience: []string{"protagonist"}},
		{ID: "direct", Channel: character.ChannelPrivate, Speaker: character.SpeakerCharacter, AddressedTo: "protagonist"},
		{ID: "other-room", Channel: character.ChannelScene, Speaker: character.SpeakerCharacter, Audience: []string{"npc"}},
		{ID: "secret", Channel: character.ChannelPrivate, Speaker: character.SpeakerCharacter, AddressedTo: "npc"},
	}})
	for _, refs := range [][]string{{"public", "direct"}, {"public", "secret"}, {"public", "missing"}, {"other-room"}} {
		value := map[string]any{"kind": "同门", "source": "conversation", "evidence_refs": refs, "evidence": "PRIVATE_FREE_TEXT"}
		result := playerLoreFact(value, "relationships", audience)
		if refs[0] == "public" && refs[1] == "direct" {
			if result == nil {
				t.Fatal("verified references lost")
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "PRIVATE") {
				t.Fatal("raw evidence escaped")
			}
		} else if result != nil {
			t.Fatalf("mixed or invisible evidence accepted: %#v", result)
		}
	}
}

func TestLoreKnowledgeAPIBoundariesKeepCanonicalJournal(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Lore boundary", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	characters, err := store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第一日",
		Profiles: []character.Profile{{ID: "npc", Name: "已知人物", Revision: 1}, {ID: "known", Name: "已知同门", Revision: 1}, {ID: "mixed", Name: "已知友人", Revision: 1}},
		Facts: character.TurnCharacterFacts{Exchanges: []character.ExchangeDraft{
			{CharacterID: "npc", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, AddressedTo: "protagonist", Text: "我与他同门。"},
			{CharacterID: "npc", Speaker: character.SpeakerCharacter, Channel: character.ChannelPrivate, AddressedTo: "mixed", Text: "仅告知另一人。"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	publicRef, privateRef := characters.Exchanges[0].ID, characters.Exchanges[1].ID
	conversation := interactiveapp.NewConversation(store, t.TempDir(), workspace, story.ID, "main", "继续", 800, nil)
	bindInteractiveCycleForTest(conversation)
	materializeInteractiveInputForTest(t, store, story.ID, "main", "继续", conversation.AgentCycleIdentitySnapshot())
	submitTestTurnResult(t, store, story.ID, "main", conversation, "继续", "观察")
	if err := commitInteractiveAssistantForTest(t, store, story.ID, "main", "继续", conversation, "风吹过码头。", ""); err != nil {
		t.Fatal(err)
	}
	turn, _, _ := conversation.LastTurnForState()
	facts := map[string]any{
		"hidden": map[string]any{"kind": "PRIVATE_TARGET", "source": "conversation"},
		"known":  map[string]any{"kind": "SAFE_RELATION", "source": "conversation", "evidence_refs": []any{publicRef}},
		"mixed":  map[string]any{"kind": "PRIVATE_MIXED", "source": "conversation", "evidence_refs": []any{publicRef, privateRef}},
	}
	if _, err := store.AppendStateDelta(story.ID, interactive.AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: []interactivestate.Op{{Op: "merge", Path: "actors", Value: map[string]any{"npc": map[string]any{"template_id": "important_character", "state": map[string]any{"已知人物关系": facts, "已知位置": map[string]any{"location": "PRIVATE_LOCATION", "source": []any{"observation", "private"}}}}}}}}); err != nil {
		t.Fatal(err)
	}

	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(before)
	if !strings.Contains(string(canonical), "PRIVATE_TARGET") {
		t.Fatal("fixture did not persist the counterexample")
	}
	app := &App{interactive: store}
	snapshot, err := app.InteractiveSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	history, err := app.InteractiveHistoryPage(story.ID, "main", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	var stream any
	_, err = emitInteractiveTurnPersistedResult(store, story.ID, conversation, func(event agentrun.Event) { stream = event.Data })
	if err != nil {
		t.Fatal(err)
	}
	overview, err := app.ReadLoreOverview(story.ID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	for label, value := range map[string]any{"overview": overview, "snapshot": snapshot, "history": history, "stream": stream} {
		data, _ := json.Marshal(value)
		if strings.Contains(string(data), "PRIVATE_") {
			t.Fatalf("%s leaked: %s", label, data)
		}
		if label != "history" && !strings.Contains(string(data), "SAFE_RELATION") {
			t.Fatalf("%s lost safe known relation: %s", label, data)
		}

	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := json.Marshal(after)
	if string(canonical) != string(saved) {
		t.Fatal("API reads modified canonical journal projection")
	}
}

func TestLoreKnowledgeSchemaDefaultsAndAliasedFieldsCannotLeak(t *testing.T) {
	schema := &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{ID: "important_character", Fields: []interactive.ActorStateField{{Name: "关系见闻", Path: "knowledge.relationships", Default: map[string]any{"secret": "PRIVATE_DEFAULT"}}}}}}}
	snapshot := interactive.Snapshot{ActorStateSchema: schema, State: map[string]any{"actors": map[string]any{"npc": map[string]any{"state": map[string]any{"关系见闻": map[string]any{"secret": "PRIVATE_ALIAS"}}}}}, Turns: []interactive.TurnEvent{{StateDelta: &interactive.StateDelta{ActorOps: []interactive.ActorStateOp{{FieldID: "关系见闻", Value: "PRIVATE_OP"}}}}}}
	originalTurns := snapshot.Turns
	projectInteractiveSnapshotForPlayer(&snapshot)
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatalf("schema or alias escaped: %s", data)
	}
	if schema.System.Templates[0].Fields[0].Default == nil {
		t.Fatal("canonical schema default was mutated")
	}
	if len(originalTurns[0].StateDelta.ActorOps) != 1 {
		t.Fatal("canonical snapshot turn slice was mutated")
	}
	for _, extra := range []map[string]any{{"sources": []any{"observation", "private"}}, {"visibility": "private"}, {"nature": "secret"}} {
		fact := map[string]any{"location": "PRIVATE_MIXED", "source": "observation"}
		for key, value := range extra {
			fact[key] = value
		}
		if playerLoreFact(fact, "location", loreKnowledgeAudience{}) != nil {
			t.Fatalf("mixed private provenance accepted: %#v", fact)
		}
	}
}

func TestLoreKnowledgeRumoursNeverBecomeConfirmed(t *testing.T) {
	for _, nature := range []string{"hearsay", "conjecture"} {
		fact := map[string]any{"location": "码头", "source": "conversation", "nature": nature, "confidence": "confirmed"}
		projected := playerLoreFact(fact, "location", loreKnowledgeAudience{})
		if projected["confidence"] != "uncertain" || projected["nature"] != nature {
			t.Fatalf("rumour hardened into fact: %#v", projected)
		}
		if fact["confidence"] != "confirmed" {
			t.Fatal("projection rewrote canonical confidence")
		}
	}
}

func TestLoreKnowledgeAudienceRejectsNewDiscoveryAfterSnapshot(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Revision fence", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{BranchID: "main", WorldTime: "第一日", Profiles: []character.Profile{{ID: "new-person", Name: "新认识的人", Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if audience := loreAudienceForSnapshot(store, before); len(audience.known) != 0 {
		t.Fatalf("new discovery authorized an old snapshot: %#v", audience)
	}
	current, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if audience := loreAudienceForSnapshot(store, current); !audience.known["new-person"] {
		t.Fatal("matching revision lost known person")
	}
}

func TestLoreKnowledgeAffiliationsHideUnknownOwnersAndPrivateMetadata(t *testing.T) {
	raw := map[string]any{"actors": map[string]any{
		"hidden": map[string]any{"state": map[string]any{"所属势力": []any{"PRIVATE_FACTION"}}},
		"npc":    map[string]any{"state": map[string]any{"所属势力": []any{"已知宗门", map[string]any{"faction_id": "safe", "name": "公开帮会", "status": "member", "role": "学徒"}, map[string]any{"name": "PRIVATE_META", "secrets": "secret"}}}},
	}}
	data, _ := json.Marshal(playerKnownState(raw, nil, loreKnowledgeAudience{known: map[string]bool{"npc": true}}))
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatalf("affiliation leaked: %s", data)
	}
	if !strings.Contains(string(data), "已知宗门") || !strings.Contains(string(data), "公开帮会") {
		t.Fatalf("known affiliation lost: %s", data)
	}
}
