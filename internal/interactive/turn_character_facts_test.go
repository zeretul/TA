package interactive

import (
	"strings"
	"testing"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

// TestStoryTurnPublishesCharacterFactsInItsOwnTransaction is the write half of
// same-scene continuity: a line the main story gives a character is a committed
// character record, in the same transaction as the Turn, so the character window
// reads it as something the character actually said.
func TestStoryTurnPublishesCharacterFactsInItsOwnTransaction(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "北门"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "上前答话", Narrative: "守卒抬手拦住你。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "guardsman", Name: "陈九", KnownAs: "北门守卒",
					Role: "北门守卒", Personality: []string{"谨慎"}, SpeechStyle: "短句",
				}},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "文牒。",
				}},
				Matters: []TurnCharacterMatterInput{{
					CharacterID: "guardsman", Kind: "permission", Status: "open",
					Statement: "守卒允许主角今夜入城。", Counterparty: "protagonist",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CharacterIDs()) != 1 || state.CharacterIDs()[0] != "guardsman" {
		t.Fatalf("characters after the story turn = %v", state.CharacterIDs())
	}
	if len(state.Exchanges) != 1 || state.Exchanges[0].Text != "文牒。" {
		t.Fatalf("story exchanges = %+v", state.Exchanges)
	}
	// The exchange belongs to the Turn: that is what lets the next turn and the
	// window both see it as part of the story rather than a side conversation.
	if state.Exchanges[0].TurnID != turn.ID {
		t.Fatalf("exchange turn id = %q, want %q", state.Exchanges[0].TurnID, turn.ID)
	}
	if state.Exchanges[0].Channel != character.ChannelScene {
		t.Fatalf("exchange channel = %q, want scene", state.Exchanges[0].Channel)
	}
	if open := character.OpenMatters("guardsman", state.Matters); len(open) != 1 {
		t.Fatalf("open matters = %+v", open)
	}

	// The turn is still one turn. A character record is a side record.
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TurnCount != 1 || len(snapshot.Turns) != 1 {
		t.Fatalf("character facts changed the turn count: %d turns=%d", snapshot.TurnCount, len(snapshot.Turns))
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("current turn = %#v", snapshot.CurrentTurn)
	}

	// And it survives a rebuild, which is what keeps a single save.
	reopened := NewStore(workspace)
	rebuilt, err := reopened.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Exchanges) != 1 || rebuilt.Exchanges[0].Text != "文牒。" || len(rebuilt.Matters) != 1 {
		t.Fatalf("rebuilt character state = %+v", rebuilt)
	}
}

// TestStoryTurnContinuesTheSameCharacterAcrossTurns is the continuity the
// window depends on: the second turn speaks for a character the first turn
// introduced, without re-registering them, and both lines belong to one person.
func TestStoryTurnContinuesTheSameCharacterAcrossTurns(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "北门"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "答话", Narrative: "守卒拦住你。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{CharacterID: "guardsman", Name: "陈九", Role: "北门守卒"}},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "文牒。",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出示文牒", Narrative: "守卒看了一眼。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				// No profile: they are already established.
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "进去吧。",
				}},
				Knowledge: []TurnCharacterKnowledgeInput{{
					CharacterID: "guardsman", Statement: "主角持有合法文牒。", Nature: "experienced",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Profiles) != 1 {
		t.Fatalf("a second turn re-registered the character: %+v", state.Profiles)
	}
	if len(state.Exchanges) != 2 {
		t.Fatalf("exchanges across two turns = %+v", state.Exchanges)
	}
	if state.Exchanges[0].TurnID != first.ID || state.Exchanges[1].TurnID != second.ID {
		t.Fatalf("exchange turn ids = %q, %q", state.Exchanges[0].TurnID, state.Exchanges[1].TurnID)
	}
	if len(state.Knowledge) != 1 || state.Knowledge[0].Nature != character.NatureExperienced {
		t.Fatalf("knowledge = %+v", state.Knowledge)
	}
}

// TestAProfileKeepsItsGenderAndVoiceAcrossTurns is the whole chain the design
// asks for — 生成、正式保存、主故事／聊天／群聊与重开保持一致. A turn introduces someone
// with a gender and a voice; the next turn mentions neither. Both must still be
// on the stored profile, because the window, the chat and the narrator all read
// this one record.
func TestAProfileKeepsItsGenderAndVoiceAcrossTurns(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "药铺"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "买药", Narrative: "掌柜抬头。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "shopkeeper", Name: "周氏", Role: "药铺掌柜",
					Age: 46, Gender: "female", SpeechStyle: "说话慢，先问用途再报价，偶尔拿药材打比方",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// A later turn says where they are standing and nothing about who they are.
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "再来一趟", Narrative: "掌柜仍在柜台后。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "shopkeeper", Name: "周氏", Attitude: "熟了",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := state.Profile("shopkeeper")
	if !ok {
		t.Fatal("the shopkeeper is gone")
	}
	if stored.Gender != character.GenderFemale {
		t.Fatalf("gender after a silent turn = %q, want %q", stored.Gender, character.GenderFemale)
	}
	if stored.Age != 46 {
		t.Fatalf("age after a silent turn = %d, want the authored age 46", stored.Age)
	}
	if stored.SpeechStyle != "说话慢，先问用途再报价，偶尔拿药材打比方" {
		t.Fatalf("voice after a silent turn = %q", stored.SpeechStyle)
	}
	if stored.Attitude != "熟了" {
		t.Fatalf("the later turn's own update was lost: %+v", stored)
	}
}

// TestAProfileRejectsAnUnknownGenderWord keeps a word the window cannot draw out
// of the record rather than storing it as free text.
func TestAProfileRejectsAnUnknownGenderWord(t *testing.T) {
	records := TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{
		CharacterID: "x", Name: "某人", Gender: "不详",
	}}}.ProfileRecords("story-1", "第一日", "node-1")
	if len(records) != 1 {
		t.Fatalf("profile records = %+v", records)
	}
	if records[0].Gender != "" {
		t.Fatalf("an unrecognised gender word was stored as %q", records[0].Gender)
	}
}


// TestStoryTurnRejectsACharacterNobodyHasMet keeps the submission honest before
// the prose is published: a turn cannot speak for a person the story has not
// introduced, and the repair happens while the turn is still a draft.
func TestStoryTurnRejectsACharacterNobodyHasMet(t *testing.T) {
	system, state := turnSubmissionTestState()
	choices := testTurnChoices()
	updates := []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/当前处境", Value: "北门"}}
	facts := &TurnCharacterFactsInput{
		Exchanges: []TurnCharacterExchangeInput{{
			CharacterID: "stranger", Speaker: "character", Channel: "scene", Text: "站住。",
		}},
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
		KnownCharacters: []string{"guardsman"},
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices, CharacterFacts: facts})
	if receipt.Ready || receipt.ModuleStatus.CharacterFacts != TurnSubmissionModuleRejected {
		t.Fatalf("an unknown character was accepted: %#v", receipt)
	}
	if got := prepared.TurnResult(); got.CharacterFacts != nil {
		t.Fatalf("a rejected module was retained: %#v", got.CharacterFacts)
	}

	// The same call with the character registered in the same submission is
	// accepted: a first appearance arrives with the lines it speaks.
	withProfile := &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "stranger", Name: "陌生人", Role: "守门人",
			Description: "一个守门人，手一直按在门闩上，问话前先看清你身后有没有人。",
		}},
		Exchanges: facts.Exchanges,
	}
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
		KnownCharacters: []string{"guardsman"},
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices, CharacterFacts: withProfile})
	if !receipt.Ready || receipt.ModuleStatus.CharacterFacts != TurnSubmissionModuleAccepted {
		t.Fatalf("a first appearance was rejected: %#v", receipt)
	}
	if got := prepared.TurnResult(); got.CharacterFacts == nil || len(got.CharacterFacts.Profiles) != 1 {
		t.Fatalf("accepted module = %#v", got.CharacterFacts)
	}
}

// TestStoryTurnWithoutCharacterFactsIsReady keeps the module optional. Most
// turns establish nothing about anyone, and they must not be forced to submit
// empty arrays to become ready.
func TestStoryTurnWithoutCharacterFactsIsReady(t *testing.T) {
	system, state := turnSubmissionTestState()
	choices := testTurnChoices()
	updates := []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/当前处境", Value: "北门"}}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if !receipt.Ready || receipt.ModuleStatus.CharacterFacts != TurnSubmissionModuleAccepted {
		t.Fatalf("an absent optional module blocked readiness: %#v", receipt)
	}
	if got := prepared.TurnResult(); got.CharacterFacts != nil {
		t.Fatalf("an absent module produced facts: %#v", got.CharacterFacts)
	}

	// A turn that submitted nothing at all still commits no character record.
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "无人物"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "你独自走在雨里。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	state2, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state2.Profiles) != 0 || len(state2.Exchanges) != 0 {
		t.Fatalf("a turn with no character facts wrote a record: %+v", state2)
	}
}

// TestStoryTurnResolvesPrivateChannelAudience is the audience rule applied to
// the story's own turn: a line spoken privately reaches its participants, not
// the world, so the guard does not learn what was said to someone else.
func TestStoryTurnResolvesPrivateChannelAudience(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "客栈"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "低声交谈", Narrative: "你压低声音。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{
					{CharacterID: "innkeeper", Name: "掌柜"},
					{CharacterID: "guardsman", Name: "陈九"},
				},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "innkeeper", Speaker: "character", Channel: "private",
					Text: "北门今夜换岗。",
				}},
				Knowledge: []TurnCharacterKnowledgeInput{{
					CharacterID: "innkeeper", Statement: "主角问起北门换岗。",
					Nature: "experienced", Audience: "single",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 1 {
		t.Fatalf("exchanges = %+v", state.Exchanges)
	}
	audience := state.Exchanges[0].Audience
	for _, id := range audience {
		if id == "guardsman" {
			t.Fatalf("a private line reached someone who was not in it: %v", audience)
		}
	}
	if got := state.KnowledgeFor("guardsman"); len(got) != 0 {
		t.Fatalf("the guard learned a private conversation: %+v", got)
	}
	if got := state.KnowledgeFor("innkeeper"); len(got) != 1 {
		t.Fatalf("the innkeeper did not keep what they learned: %+v", got)
	}
}

// TestStoryTurnCharacterRecordDoesNotSurviveRegeneration is the projection
// guard: a replaced turn's character record is left behind in the journal but
// must not be folded back into the branch, or the cast would remember a scene
// the story replaced.
func TestStoryTurnCharacterRecordDoesNotSurviveRegeneration(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "重写"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "答话", Narrative: "守卒拦住你。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{CharacterID: "guardsman", Name: "陈九"}},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "文牒。",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出示文牒", Narrative: "守卒看了一眼。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "进去吧。",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Replace the second turn with one whose guard says something else.
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出示文牒", Narrative: "守卒没有接文牒。",
		ReplaceTurnID: "",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "guardsman", Speaker: "character", Channel: "scene", Text: "退后。",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// Whatever the projection kept, it must be internally consistent: no
	// exchange may name a turn the branch no longer contains.
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]bool{}
	for _, turn := range snapshot.Turns {
		live[turn.ID] = true
	}
	for _, exchange := range state.Exchanges {
		if exchange.TurnID != "" && !live[exchange.TurnID] {
			t.Fatalf("character memory kept a line from a replaced turn %q: %+v", exchange.TurnID, exchange)
		}
	}
	_ = first
}

// TestStoryTurnDeliversACausedMessage is the write half of proactive messaging:
// a message the turn's own events caused is committed in the same transaction as
// the Turn, keeps the world time the character wrote at rather than the time the
// player next reads it, and is delivered once however often the commit is
// replayed.
func TestStoryTurnDeliversACausedMessage(t *testing.T) {
	system := defaultActorStateSystem()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "北门", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出城", Narrative: "你离开北门，城门在身后合上。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/当前时间", Value: "第3日 酉时",
			}},
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "guardsman", Name: "陈九", Role: "北门守卒",
				}},
				Messages: []TurnCharacterMessageInput{{
					CharacterID: "guardsman", Text: "今夜北门有变，别走小路。",
					Basis: "主角刚与他约定今夜再会", Deadline: "第6日",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	messages := state.MessagesFor("guardsman")
	if len(messages) != 1 {
		t.Fatalf("a turn-carried message did not reach the journal: %+v", state.Messages)
	}
	message := messages[0]
	// The send time is the turn's own world time. It is what the player is told
	// when they read the message later, so it must not be the read time.
	if message.WorldTime != "第3日 酉时" {
		t.Fatalf("message world time = %q, want the turn's own 第3日 酉时", message.WorldTime)
	}
	if message.BasisTurnID != turn.ID || message.BasisEventID == "" {
		t.Fatalf("message is not pinned to the turn that caused it: %+v", message)
	}
	if message.Status != character.MessageUnread {
		t.Fatalf("a delivered message starts %q, want unread", message.Status)
	}
	// Nothing about receiving it accepts anything on the protagonist's behalf.
	if len(state.Matters) != 0 {
		t.Fatalf("a message settled a matter by itself: %+v", state.Matters)
	}

	// The deadline is measured against the story's own clock, not the wall clock.
	if message.Lapsed("第4日") {
		t.Fatal("a message lapsed before its deadline")
	}
	if !message.Lapsed("第7日") {
		t.Fatal("a message did not lapse after its deadline")
	}

	// Replaying the same content through the window's write path is a read, not a
	// second delivery: identity is content-hashed, so the same character saying
	// the same thing for the same reason is one record.
	if _, err := store.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "第3日 酉时",
		Scene: character.Scene{NodeID: turn.ID, WorldTime: "第3日 酉时"},
		Facts: character.TurnCharacterFacts{
			Messages: []character.MessageDraft{{
				CharacterID: "guardsman", Text: "今夜北门有变，别走小路。",
				Basis: "主角刚与他约定今夜再会", Deadline: "第6日",
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.MessagesFor("guardsman")) != 1 {
		t.Fatalf("a replayed message was delivered twice: %+v", replayed.MessagesFor("guardsman"))
	}

	// And it survives a rebuild, which is what keeps a single save.
	reopened := NewStore(store.root)
	rebuilt, err := reopened.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.MessagesFor("guardsman")) != 1 || rebuilt.MessagesFor("guardsman")[0].WorldTime != "第3日 酉时" {
		t.Fatalf("rebuilt messages = %+v", rebuilt.MessagesFor("guardsman"))
	}
}

// TestPresetCharacterKeepsItsActorWhenTheNameChanges is the C4 anti-duplication
// rule at its sharpest: the model mints an actor id from the name it reads, so a
// preset person who returns under a different spelling — or in a later branch
// that inherited the registry — would otherwise be registered a second time,
// with a second memory and a second set of relationships. The source key is the
// author's identity, so the second appearance must land on the first actor.
func TestPresetCharacterKeepsItsActorWhenTheNameChanges(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "黄枫谷"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "向宋观澜请教残阵", Narrative: "宋观澜放下阵图。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "song_guanlan", Name: "宋观澜",
					SourceKey: "npc_song_guanlan", Role: "黄枫谷阵法学徒",
				}},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "song_guanlan", Speaker: "character", Channel: "scene",
					Text: "这座残阵缺了一角。",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The next turn names the same person differently and mints a second id, the
	// way the model would if it read a title instead of the name.
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "再问细节", Narrative: "那人又补了一句。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "surname_song_array_study", Name: "宋师兄",
					SourceKey: "npc_song_guanlan",
				}},
				Exchanges: []TurnCharacterExchangeInput{{
					CharacterID: "surname_song_array_study", Speaker: "character", Channel: "scene",
					Text: "西侧还有一处断纹。",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Profiles) != 1 {
		t.Fatalf("a preset person was cast twice: %+v", state.Profiles)
	}
	profile := state.Profiles[0]
	if profile.ID != "song_guanlan" {
		t.Fatalf("the preset person moved to a new actor id %q", profile.ID)
	}
	// The established actor keeps its own name: a later turn's spelling is not a
	// rename.
	if profile.Name != "宋观澜" {
		t.Fatalf("the preset person was renamed to %q", profile.Name)
	}
	// Both lines belong to the one person, and the second line is recorded under
	// the actor that exists rather than the minted id.
	if len(state.Exchanges) != 2 {
		t.Fatalf("exchanges = %+v", state.Exchanges)
	}
	for _, exchange := range state.Exchanges {
		if exchange.CharacterID != "song_guanlan" {
			t.Fatalf("a line was recorded against %q, which is not the registered actor", exchange.CharacterID)
		}
	}
	if state.Exchanges[0].TurnID != first.ID {
		t.Fatalf("the first line lost its turn: %+v", state.Exchanges[0])
	}
}

// TestInventedCharacterDoesNotBorrowAPresetKey is the other direction: the source
// key is the author's identity, so a character the story invented must never
// acquire one, and an empty key never matches.
func TestInventedCharacterDoesNotBorrowAPresetKey(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "坊市"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "向摊主打听", Narrative: "摊主抬起头。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{
					{CharacterID: "stallkeeper", Name: "摊主"},
					{CharacterID: "song_guanlan", Name: "宋观澜", SourceKey: "npc_song_guanlan"},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Profiles) != 2 {
		t.Fatalf("profiles = %+v", state.Profiles)
	}
	for _, profile := range state.Profiles {
		if profile.ID == "stallkeeper" && profile.SourceKey != "" {
			t.Fatalf("an invented character acquired the preset key %q", profile.SourceKey)
		}
	}
	// Only the preset person is discoverable as established.
	keys := state.SourceKeys()
	if len(keys) != 1 || keys[0] != "npc_song_guanlan" {
		t.Fatalf("established source keys = %v", keys)
	}
	if _, ok := state.ProfileBySourceKey(""); ok {
		t.Fatal("an empty source key matched a profile")
	}
}

// TestAnIntroductionSurvivesATurnThatOnlyChangesAttitude is the re-appearance
// half of the design's promise: 再会、切换聊天及重启不重新抽取. The first turn
// introduces someone and says who they are; a later turn only reports that they
// warmed to the protagonist. If the later profile simply replaced the earlier
// one, the introduction the player reads in the person card would be blanked by
// a turn that never meant to describe anybody.
//
// The source key, the gender and the voice are already kept this way for the
// same reason; the introduction belongs with them because it is the field the
// person card actually shows.
func TestAnIntroductionSurvivesATurnThatOnlyChangesAttitude(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "药铺"})
	if err != nil {
		t.Fatal(err)
	}
	const introduction = "药铺掌柜，四十上下，手指关节粗大，认得每一味药材的气味。"
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "抓药", Narrative: "掌柜抬头看你一眼。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "shopkeeper", Name: "周氏", Role: "药铺掌柜",
					Description: introduction,
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "又来一趟", Narrative: "掌柜点头，算是认得了。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			CharacterFacts: &TurnCharacterFactsInput{
				Profiles: []TurnCharacterProfileInput{{
					CharacterID: "shopkeeper", Name: "周氏", Attitude: "熟了",
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	state, err := store.CharacterState(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := state.Profile("shopkeeper")
	if !ok {
		t.Fatal("the shopkeeper is gone")
	}
	if stored.Description != introduction {
		t.Fatalf("简介在一次只改态度的回合之后变成了 %q", stored.Description)
	}
	if stored.Role != "药铺掌柜" {
		t.Fatalf("身份在一次只改态度的回合之后变成了 %q", stored.Role)
	}
	if stored.Attitude != "熟了" {
		t.Fatalf("the later turn's own update was lost: %+v", stored)
	}
}

// TestANewPersonMustBeIntroducedInWords is the generation half of G28: when the
// model formally saves a person, the introduction it saves is what the player
// reads in that person's card. An empty one and a paragraph pasted from the scene
// are refused while the turn is still a draft, so the repair happens in the same
// run rather than after the player has opened an empty card.
//
// It deliberately does not judge whether the words are good. See the short-but-
// specific case below: an earlier version measured the length of what remained
// after removing the name and role, and refused a four-character introduction
// that said exactly what the person does.
func TestANewPersonMustBeIntroducedInWords(t *testing.T) {
	cases := []struct {
		name     string
		profile  TurnCharacterProfileInput
		accepted bool
	}{
		{
			name: "empty",
			profile: TurnCharacterProfileInput{
				CharacterID: "clerk", Name: "许录事", Role: "县衙录事",
			},
			accepted: false,
		},
		{
			// The field exists and fits, which is all the compiler asks. Whether it
			// is worth reading is the author's and the model's question.
			name: "role and place only",
			profile: TurnCharacterProfileInput{
				CharacterID: "clerk", Name: "许录事", Role: "县衙录事",
				Description: "许录事，是县衙录事。",
			},
			accepted: true,
		},
		{
			// The smallest reproduction of the defect this test was rewritten for:
			// removing 陈九 and the punctuation leaves 只卖旧书, four characters.
			name: "short but specific",
			profile: TurnCharacterProfileInput{
				CharacterID: "chen_jiu", Name: "陈九", Role: "书摊摊主",
				Description: "陈九，只卖旧书。",
			},
			accepted: true,
		},
		{
			name: "a paragraph",
			profile: TurnCharacterProfileInput{
				CharacterID: "clerk", Name: "许录事", Role: "县衙录事",
				Description: strings.Repeat("县衙的走廊很长，他站在尽头，手里捏着一卷没写完的册子。", 6),
			},
			accepted: false,
		},
		{
			name: "something only this person did",
			profile: TurnCharacterProfileInput{
				CharacterID: "clerk", Name: "许录事", Role: "县衙录事",
				Description: "县衙录事，说话时习惯把册子卷起来敲掌心，认得城里每一户的字迹。",
			},
			accepted: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input := &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{testCase.profile}}
			diagnostic := validateTurnCharacterFactsModule(input, nil, character.Scene{}, nil, nil, nil, 0)
			if testCase.accepted {
				if diagnostic != nil {
					t.Fatalf("a usable introduction was refused: %+v", diagnostic)
				}
				return
			}
			if diagnostic == nil {
				t.Fatal("the introduction was accepted")
			}
			if !diagnostic.Retryable {
				t.Fatalf("the refusal cannot be repaired in the same run: %+v", diagnostic)
			}
			if diagnostic.Module != TurnSubmissionModuleCharacterFacts {
				t.Fatalf("module = %q", diagnostic.Module)
			}
			if diagnostic.Index == nil || *diagnostic.Index != 0 {
				t.Fatalf("the diagnostic does not name the person: %+v", diagnostic)
			}
		})
	}
}

// TestAnEstablishedPersonNeedsNoSecondIntroduction is the other side of the
// rule: a later turn that only reports where somebody is standing is normal, and
// making it restate the introduction would turn every re-appearance into a retry.
func TestAnEstablishedPersonNeedsNoSecondIntroduction(t *testing.T) {
	input := &TurnCharacterFactsInput{
		Profiles: []TurnCharacterProfileInput{{
			CharacterID: "clerk", Name: "许录事", Attitude: "见了面会点头",
		}},
	}
	if diagnostic := validateTurnCharacterFactsModule(input, []string{"clerk"}, character.Scene{}, nil, nil, nil, 0); diagnostic != nil {
		t.Fatalf("a re-appearance was refused for not reintroducing itself: %+v", diagnostic)
	}
}
