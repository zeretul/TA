package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

// intimacyInviteTestApp builds a story whose scene already holds one adult
// person, with the intimacy purpose bound to a local model that answers as that
// person. Everything below the model is the production path.
func intimacyInviteTestApp(t *testing.T) (*App, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		text := string(body)
		answer := ""
		switch {
		case strings.Contains(text, "inviting you to a private"):
			answer = `{"status":"accepted","reply":"（她沉默片刻，点了点头。）你若真只是想说说话，那就说吧。"}`
		case strings.Contains(text, "Write the opening"):
			answer = `{"lines":[{"author_id":"chen_qiaoqian","text":"她坐在灯下，声音放得很轻。"}]}`
		case strings.Contains(text, "The protagonist's new line is the user turn"):
			answer = `{"lines":[{"author_id":"chen_qiaoqian","text":"她把灯往你这边挪了挪，没有接话。"}]}`
		default:
			http.Error(writer, "unexpected prompt", 400)
			return
		}
		encoded, _ := json.Marshal(map[string]any{
			"id": "fixture", "object": "chat.completion",
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": answer},
			}},
		})
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(encoded)
	}))
	t.Cleanup(server.Close)

	root := t.TempDir()
	cfg := &config.Config{
		NovaDir: root, Workspace: root,
		OpenAIBaseURL: server.URL + "/v1",
		ModelEndpoints: []config.ModelEndpointSettings{{
			ID: "default", Provider: "openai-compatible", Protocol: "openai-chat-completions",
			BaseURL: server.URL + "/v1", APIKey: "fixture-only",
		}},
		ModelProfiles: []config.ModelProfileSettings{{ID: "default", EndpointID: "default", Model: "fixture-model"}},
	}
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	system, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "亲密邀请", ActorState: &system.ActorState,
		// The protagonist is a man, and both people in the room are women, so the
		// only thing these tests ever vary is age. The gender gate has its own
		// cases in intimacy_gender_test.go.
		ProtagonistGender: character.GenderMale,
		IntimacySettings:  interactive.StoryIntimacySettings{ProfileID: "default"},
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "npc_chen_qiaoqian", Name: "陈巧倩", Age: 26},
			// 林晚 stands in the same room with her own authored age, and that
			// age is below the bar. She is here so the eligibility rule is tested
			// against a person the story really has in the scene rather than
			// against an id the window would never see.
			{SourceKey: "npc_lin_wan", Name: "林晚", Age: 16},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "推门进客舍", Narrative: "陈巧倩坐在灯下，林晚在窗边研墨。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "chen_qiaoqian", "lin_wan"},
			Reason: "本回合确认的在场角色",
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后",
		Profiles: []character.Profile{{
			ID: "chen_qiaoqian", SourceKey: "npc_chen_qiaoqian", Name: "陈巧倩", Role: "掩月宗修士",
			Gender: character.GenderFemale,
		}, {
			ID: "lin_wan", SourceKey: "npc_lin_wan", Name: "林晚", Role: "掩月宗弟子",
			Gender: character.GenderFemale,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{cfg: cfg, workspace: root, interactive: store}, story.ID
}

// TestOneToOneInvitationEntersTheActivityWhenThePersonAgrees is §3.1's direct
// entry at the write side.
//
// The invitation commits the person's own answer. Entering is a second write in
// the same request, and it must carry its own command identity: reusing the
// invitation's makes the store read the confirm as a replay of the record it
// just wrote, so the roster stays "ready", the room is never opened, and the
// opening is dropped without an error. The player sees a successful invitation
// and a window that never starts.
func TestOneToOneInvitationEntersTheActivityWhenThePersonAgrees(t *testing.T) {
	application, storyID := intimacyInviteTestApp(t)

	response, err := application.InviteIntimacy(context.Background(), IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "invite-1",
	})
	if err != nil {
		t.Fatalf("a one-to-one invitation: %v", err)
	}
	if response.Activity.Phase != character.IntimacyActive {
		t.Fatalf("phase = %q, want active: the person agreed and the invitation is the player's intent", response.Activity.Phase)
	}
	if response.Activity.GroupID == "" {
		t.Fatal("the running activity has no room to hold its conversation")
	}

	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	group, ok := state.Group(response.Activity.GroupID)
	if !ok {
		t.Fatalf("the room %q the activity names does not exist", response.Activity.GroupID)
	}
	lines := state.ExchangesFor("chen_qiaoqian")
	opening := 0
	for _, exchange := range lines {
		if exchange.Channel == character.ChannelGroup && exchange.GroupID == group.ID {
			opening++
		}
	}
	if opening == 0 {
		t.Fatalf("the opening was never committed: %+v", lines)
	}
}

// TestRetriedInvitationStillEntersTheActivity is the same path after a lost
// response. The retry reads the invitation back instead of asking the person
// twice, and it must still finish entering rather than leave the roster ready.
func TestRetriedInvitationStillEntersTheActivity(t *testing.T) {
	application, storyID := intimacyInviteTestApp(t)
	request := IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "invite-retry",
	}
	if _, err := application.InviteIntimacy(context.Background(), request); err != nil {
		t.Fatalf("the first attempt: %v", err)
	}
	retried, err := application.InviteIntimacy(context.Background(), request)
	if err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if retried.Activity.Phase != character.IntimacyActive {
		t.Fatalf("phase after a retry = %q, want active", retried.Activity.Phase)
	}
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	opening := 0
	for _, exchange := range state.ExchangesFor("chen_qiaoqian") {
		if exchange.Channel == character.ChannelGroup && exchange.GroupID == retried.Activity.GroupID {
			opening++
		}
	}
	if opening != 1 {
		t.Fatalf("the retry wrote %d opening lines, want exactly the one already on file", opening)
	}
}
