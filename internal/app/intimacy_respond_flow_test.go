package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

// intimacyRespondTestApp builds a story whose scene already holds three adults,
// with the intimacy purpose bound to a local model. Three people are needed
// because the defect this file pins down only appears once a running activity
// already has a room and someone new is admitted into it: with one person there
// is nobody for the answer to reach.
//
// 白露 answers with a condition the first time she is asked, which is the real
// path to RespondIntimacy: §13.1 keeps a conditional answer as 待商议, and it is
// asked again once the condition is settled.
func intimacyRespondTestApp(t *testing.T) (*App, string) {
	t.Helper()
	// 白露 is the only person who is invited into an already-running room, so the
	// roster line naming the two people already in it is her question and nobody
	// else's. Her first answer carries a condition and her second does not, which
	// is the real shape of a re-ask: §13.1 keeps a conditional answer as 待商议
	// until the condition is settled.
	var mu sync.Mutex
	conditionalAnswers := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		text := string(body)
		answer := ""
		switch {
		case strings.Contains(text, "inviting you to a private") && strings.Contains(text, "Others who have been invited: 陈巧倩, 苏月"):
			mu.Lock()
			conditionalAnswers++
			first := conditionalAnswers == 1
			mu.Unlock()
			if first {
				answer = `{"status":"conditional","reply":"（她看了一眼窗外的天色。）等这阵雨停了，我就来。","condition":"等这阵雨停"}`
			} else {
				answer = `{"status":"accepted","reply":"（雨声渐歇，她把窗推开。）走吧。"}`
			}
		case strings.Contains(text, "inviting you to a private"):
			answer = `{"status":"accepted","reply":"（她略一沉吟，点了头。）那就一起吧。"}`
		case strings.Contains(text, "Write the opening"):
			answer = `{"lines":[{"author_id":"chen_qiaoqian","text":"她把窗掩了半扇。"}]}`
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
		Title: "三人商议", ActorState: &system.ActorState,
		// A man and three women: the roster mechanics are what this file tests.
		ProtagonistGender: character.GenderMale,
		IntimacySettings:  interactive.StoryIntimacySettings{ProfileID: "default"},
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "npc_chen_qiaoqian", Name: "陈巧倩", Age: 26},
			{SourceKey: "npc_su_yue", Name: "苏月", Age: 24},
			{SourceKey: "npc_bai_lu", Name: "白露", Age: 22},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "推门进客舍", Narrative: "陈巧倩坐在灯下，苏月在添茶，白露靠在窗边。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "chen_qiaoqian", "su_yue", "bai_lu"},
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
			ID: "su_yue", SourceKey: "npc_su_yue", Name: "苏月", Role: "散修",
			Gender: character.GenderFemale,
		}, {
			ID: "bai_lu", SourceKey: "npc_bai_lu", Name: "白露", Role: "散修",
			Gender: character.GenderFemale,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{cfg: cfg, workspace: root, interactive: store}, story.ID
}

// intimacyRunningRoomWithAConditional is the state just before RespondIntimacy:
// two people are in the running room, and the third has answered with a
// condition and is therefore still outside it (§13.1: 条件未满足仍是待商议).
func intimacyRunningRoomWithAConditional(t *testing.T) (*App, string, string) {
	t.Helper()
	application, storyID := intimacyRespondTestApp(t)
	ctx := context.Background()

	first, err := application.InviteIntimacy(ctx, IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", CommandID: "invite-first",
	})
	if err != nil {
		t.Fatalf("invite the first person: %v", err)
	}
	activityID := first.ActivityID
	second, err := application.InviteIntimacy(ctx, IntimacyInviteRequest{
		StoryID: storyID, ActivityID: activityID, CharacterID: "su_yue", CommandID: "invite-second",
	})
	if err != nil {
		t.Fatalf("invite the second person: %v", err)
	}
	if _, err := application.ConfirmIntimacy(ctx, IntimacyConfirmRequest{
		StoryID: storyID, ActivityID: activityID, ExpectedVersion: second.Activity.Version, CommandID: "confirm-roster",
	}); err != nil {
		t.Fatalf("confirm the roster: %v", err)
	}
	third, err := application.InviteIntimacy(ctx, IntimacyInviteRequest{
		StoryID: storyID, ActivityID: activityID, CharacterID: "bai_lu", CommandID: "invite-third",
	})
	if err != nil {
		t.Fatalf("invite the third person into the running activity: %v", err)
	}
	participant, ok := third.Activity.Participant("bai_lu")
	if !ok {
		t.Fatal("the third person is not on the roster at all")
	}
	if participant.Status != character.IntimacyConditional {
		t.Fatalf("the third person's answer = %q, want the condition the fixture stated", participant.Status)
	}
	return application, storyID, activityID
}

// A person who agrees only after their condition is met is admitted by that
// answer, and the answer is spoken in the room they are joining. The people
// already in it are standing there, so the moment is theirs too: §13.3 requires
// each line to record the audience it really had, and 同一经历可由不同参与者各自
// 知情.
//
// Writing the answer against a scene that holds only the speaker makes consent
// read as a private aside. The others are in the room and hear nothing, so the
// memory and the scene disagree about what happened. InviteIntimacy already
// resolves the audience from the group for exactly this reason; the two entry
// points must agree, or which button the player pressed decides whether a shared
// moment becomes a shared memory.
func TestAnswerAdmittingSomeoneIsHeardByTheRoom(t *testing.T) {
	application, storyID, activityID := intimacyRunningRoomWithAConditional(t)
	ctx := context.Background()

	answered, err := application.RespondIntimacy(ctx, IntimacyRespondRequest{
		StoryID: storyID, ActivityID: activityID, CharacterID: "bai_lu", CommandID: "answer-settled",
	})
	if err != nil {
		t.Fatalf("record the settled answer: %v", err)
	}
	if answered.Activity.Phase != character.IntimacyActive {
		t.Fatalf("phase = %q after a settled yes, want the activity still running", answered.Activity.Phase)
	}

	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		t.Fatal("the running activity disappeared")
	}
	room, ok := state.Group(activity.GroupID)
	if !ok {
		t.Fatal("the activity's room is missing")
	}
	roster := room.ActiveMembers()
	if len(roster) != 3 {
		t.Fatalf("the room holds %v, want all three who agreed", roster)
	}

	var answer *character.Exchange
	for _, exchange := range state.GroupExchanges(activity.GroupID) {
		if exchange.AuthorID == "bai_lu" {
			found := exchange
			answer = &found
			break
		}
	}
	if answer == nil {
		t.Fatal("the answer that admitted her is not in the room at all")
	}
	for _, id := range roster {
		if !character.ContainsID(answer.AudienceMembers(), id) {
			t.Fatalf("audience = %v, want %q, who was in the room, to have heard it", answer.AudienceMembers(), id)
		}
	}
	// The memory follows the audience, not the invitation.
	for _, id := range []string{"chen_qiaoqian", "su_yue"} {
		heard := false
		for _, exchange := range state.ExchangesFor(id) {
			if exchange.ID == answer.ID {
				heard = true
				break
			}
		}
		if !heard {
			t.Fatalf("%q was in the room and does not remember the answer spoken in it", id)
		}
	}
}

// The other half of the same rule: a person who has not agreed is not in the
// room, so what they said while still negotiating stays between them and the
// protagonist. §13.1 keeps 获准前不能插入活动演绎, and a condition is a thing said
// to the protagonist rather than a fact handed to the room.
func TestAConditionalAnswerStaysOutOfTheRoom(t *testing.T) {
	application, storyID, activityID := intimacyRunningRoomWithAConditional(t)
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		t.Fatal("the running activity disappeared")
	}
	for _, exchange := range state.GroupExchanges(activity.GroupID) {
		if exchange.AuthorID == "bai_lu" {
			t.Fatalf("someone still negotiating is already speaking in the room: %+v", exchange)
		}
	}
	for _, id := range []string{"chen_qiaoqian", "su_yue"} {
		for _, exchange := range state.ExchangesFor(id) {
			if strings.Contains(exchange.Text, "等这阵雨停") {
				t.Fatalf("%q was handed the condition she stated to the protagonist: %+v", id, exchange)
			}
		}
	}
	// It is not enough to keep the condition out of the room: the answer has to
	// stay readable somewhere, or a conditional reply looks like a click that
	// vanished. §3.3 lists the invitation threads separately for exactly this.
	window, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID, ActivityID: activityID})
	if err != nil {
		t.Fatalf("read the window: %v", err)
	}
	found := false
	for _, thread := range window.Invitations {
		if thread.CharacterID != "bai_lu" {
			continue
		}
		for _, exchange := range thread.Exchanges {
			if strings.Contains(exchange.Text, "等这阵雨停") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("her conditional answer is not readable in her own invitation thread: %+v", window.Invitations)
	}
}
