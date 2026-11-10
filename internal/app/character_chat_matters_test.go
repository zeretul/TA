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

// characterChatMattersApp builds a story whose window can be opened on one
// person, with the character-chat purpose bound to a local model that answers
// through `answer`. Everything below the model is the production path:
// SendCharacterMessage, AppendCharacterFacts, the journal.
func characterChatMattersApp(t *testing.T, answer func(prompt string) string) (*App, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		reply := answer(string(body))
		if reply == "" {
			http.Error(writer, "unexpected prompt", 400)
			return
		}
		encoded, _ := json.Marshal(map[string]any{
			"id": "fixture", "object": "chat.completion",
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": reply},
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
		Title: "渡口纪事", ActorState: &system.ActorState,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "我在渡口坐下",
		Narrative: "陈婆婆挎着竹篮坐到船边，说她儿子阿九被扣在北岸东码头第三间货棚里。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "陈婆婆"},
			Reason: "本回合确认的在场角色",
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后",
		Profiles: []character.Profile{{
			ID: "陈婆婆", Name: "陈婆婆", Role: "渡口卖饼的老妇",
			Gender: character.GenderFemale,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return &App{cfg: cfg, workspace: root, interactive: store}, story.ID
}

// answerFor is a fixture that replies with one fixed object to any prompt.
func answerFor(reply string) func(string) string {
	return func(string) string { return reply }
}

// TestPrivateConversationRecordsWhatWasAgreed is the window's half of the
// negotiated-action entry.
//
// The story turn and the group window both commit the threads they create; the
// private window did not, so an arrangement made one-to-one was invisible to the
// next story turn and the character window never offered "add to the next
// action". The agreement is a committed fact, not something the panel infers
// from the prose, so it has to be written here.
func TestPrivateConversationRecordsWhatWasAgreed(t *testing.T) {
	application, storyID := characterChatMattersApp(t, answerFor(
		`{"reply":"哎，哎，应下就好。","affinities":[],"relationships":[],"matters":[`+
			`{"character_id":"陈婆婆","kind":"action","status":"open","statement":"沈舟应下过河后去东码头第三间货棚看一眼阿九。"}]}`))

	result, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆",
		Message: "我应下了，过河就去东码头第三间货棚看一眼。",
		CommandID: "cmd-private-agree",
	})
	if err != nil {
		t.Fatalf("a private agreement: %v", err)
	}
	if strings.TrimSpace(result.Reply) == "" {
		t.Fatal("the reply is empty")
	}

	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	open := character.OpenMatters("陈婆婆", state.MattersFor("陈婆婆"))
	if len(open) != 1 {
		t.Fatalf("open matters = %+v, want the one thread this exchange agreed", open)
	}
	agreed := open[0]
	if agreed.Kind != character.MatterAction || agreed.Status != character.MatterOpen {
		t.Fatalf("recorded matter = %+v, want an open action", agreed)
	}
	if !strings.Contains(agreed.Statement, "东码头") {
		t.Fatalf("recorded statement = %q, want the arrangement itself", agreed.Statement)
	}
	// The thread is stamped with the same world time the exchange was, so a
	// deadline bound to it lands on the branch's own clock rather than on the
	// wall clock of the machine that happened to send the message.
	if agreed.WorldTime != result.WorldTime {
		t.Fatalf("matter world time = %q, exchange world time = %q", agreed.WorldTime, result.WorldTime)
	}
	// The window reads its entry from committed facts, so the thread has to be
	// visible there too rather than only in the full state.
	visible := characterWindowMatters("陈婆婆", state.MattersFor("陈婆婆"))
	if len(visible) != 1 || visible[0].ID != agreed.ID {
		t.Fatalf("window matters = %+v, want the committed thread %q", visible, agreed.ID)
	}
}

// TestPrivateConversationKeepsAPriceOrAHesitationOutOfTheFacts is the negative
// half. Asking what something would cost, naming a price, and thinking it over
// are not an agreement, and a thread recorded for them would put "add to the
// next action" in front of the player for something nobody actually undertook.
func TestPrivateConversationKeepsAPriceOrAHesitationOutOfTheFacts(t *testing.T) {
	application, storyID := characterChatMattersApp(t, answerFor(
		`{"reply":"船钱？这我可做不了主，得问船家。","affinities":[],"relationships":[],"matters":[]}`))

	if _, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆",
		Message: "帮你问一声要多少船钱，这总行吧？",
		CommandID: "cmd-private-quote",
	}); err != nil {
		t.Fatalf("a question about a price: %v", err)
	}

	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if open := character.OpenMatters("陈婆婆", state.MattersFor("陈婆婆")); len(open) != 0 {
		t.Fatalf("open matters = %+v, want none: a price and a hesitation are not an agreement", open)
	}
}

// TestPrivateConversationSettlesTheThreadItDischarges covers the other direction:
// an arrangement the story opened, which this conversation actually carried out.
// Without it the thread would stay open forever and the story would keep being
// told the player had not yet agreed.
func TestPrivateConversationSettlesTheThreadItDischarges(t *testing.T) {
	// The story itself opens the thread, exactly as a narrative turn does.
	application, storyID := characterChatMattersApp(t, func(string) string { return "" })
	if _, err := application.interactive.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后",
		Facts: character.TurnCharacterFacts{Matters: []character.MatterDraft{{
			CharacterID: "陈婆婆", Kind: character.MatterAction, Status: character.MatterOpen,
			Statement: "陈婆婆托沈舟过河后去东码头第三间货棚看一眼阿九，尚未得他应承。",
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	open := character.OpenMatters("陈婆婆", before.MattersFor("陈婆婆"))
	if len(open) != 1 {
		t.Fatalf("setup open matters = %+v", open)
	}
	pendingID := open[0].ID

	// The model is shown the real id in its prepared context; the fixture cannot
	// know it in advance, so the answer is composed once the story has minted it.
	server := answerServer(t, `{"reply":"哎，哎，应下就好。","affinities":[],"relationships":[],"matters":[`+
		`{"character_id":"陈婆婆","kind":"action","status":"settled","statement":"沈舟当面应下过河后去东码头第三间货棚看一眼阿九。",`+
		`"resolves":"`+pendingID+`"}]}`)
	application.cfg.ModelEndpoints[0].BaseURL = server.URL + "/v1"
	application.cfg.OpenAIBaseURL = server.URL + "/v1"

	if _, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆",
		Message: "我应下了，过河就去东码头第三间货棚看一眼。",
		CommandID: "cmd-private-settle",
	}); err != nil {
		t.Fatalf("a private agreement settling an open thread: %v", err)
	}

	after, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	var settled character.Matter
	for _, matter := range after.MattersFor("陈婆婆") {
		if matter.ID == pendingID {
			settled = matter
		}
	}
	if settled.ID == "" {
		t.Fatalf("the thread %q disappeared", pendingID)
	}
	if settled.Status != character.MatterSettled {
		t.Fatalf("thread status = %q, want settled", settled.Status)
	}
	if settled.SettledBy == "" || settled.SettledBy == pendingID {
		t.Fatalf("thread settled by %q, want the exchange's own matter id", settled.SettledBy)
	}
	if open := character.OpenMatters("陈婆婆", after.MattersFor("陈婆婆")); len(open) != 0 {
		t.Fatalf("open matters = %+v, want none left", open)
	}
}

// TestPrivateConversationDropsAMatterItCannotSettle keeps one malformed link
// from losing the player's message. A window reply is a single model call with
// no repair pass, so a `resolves` that names something not open here is dropped
// and the new thread kept, rather than failing the whole exchange.
func TestPrivateConversationDropsAMatterItCannotSettle(t *testing.T) {
	application, storyID := characterChatMattersApp(t, answerFor(
		`{"reply":"哎，应下就好。","affinities":[],"relationships":[],"matters":[`+
			`{"character_id":"陈婆婆","kind":"action","status":"settled","statement":"沈舟应下过河去看阿九。","resolves":"mat_does_not_exist"}]}`))

	result, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆",
		Message: "我应下了。",
		CommandID: "cmd-private-bad-link",
	})
	if err != nil {
		t.Fatalf("a dangling resolves link must not lose the exchange: %v", err)
	}
	if strings.TrimSpace(result.Reply) == "" {
		t.Fatal("the reply is empty")
	}
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	matters := state.MattersFor("陈婆婆")
	if len(matters) != 1 {
		t.Fatalf("matters = %+v, want the new thread kept", matters)
	}
	if matters[0].SettledBy != "" {
		t.Fatalf("kept matter settled by %q, want the dangling link dropped", matters[0].SettledBy)
	}
}

// TestPrivateConversationDropsAMatterAboutSomeoneElse keeps the one-to-one
// window from writing a thread onto a person who is not in this conversation.
func TestPrivateConversationDropsAMatterAboutSomeoneElse(t *testing.T) {
	application, storyID := characterChatMattersApp(t, answerFor(
		`{"reply":"哎，应下就好。","affinities":[],"relationships":[],"matters":[`+
			`{"character_id":"周管事","kind":"promise","statement":"周管事答应替沈舟留一间房。"}]}`))

	if _, err := application.SendCharacterMessage(context.Background(), CharacterChatRequest{
		StoryID: storyID, CharacterID: "陈婆婆",
		Message: "我应下了。",
		CommandID: "cmd-private-third-party",
	}); err != nil {
		t.Fatalf("a third-party matter must not lose the exchange: %v", err)
	}
	state, err := application.interactive.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if matters := state.MattersFor("陈婆婆"); len(matters) != 0 {
		t.Fatalf("matters = %+v, want none attributed to this character", matters)
	}
}

// answerServer is a one-answer fixture endpoint, used when a test has to know a
// story-minted id before it can compose the model's reply.
func answerServer(t *testing.T, answer string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	return server
}
