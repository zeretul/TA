package interactiveapp

import (
	"context"
	"strings"
	"testing"

	"ta/config"
	agentcontext "ta/internal/agents/context"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file covers C-NEXT-10's four points as one turn actually sees them.
//
// The design asks for a representative bound-world scene driven by ordinary
// natural input: the relevant material has to enter the turn, an arrangement
// that is only offered has to stay pending, a character has to answer in the
// voice their own entry describes, and every one of those has to be traceable to
// the input the model really received. Then an unrelated intent must not drag
// the old topic along forever.
//
// The fixtures are the shipped 凡人人界 pack and its own world book. Nothing
// here edits a pack asset, adds a second retrieval engine or invents lore: the
// assertions read what the real assembly path put into the final messages.

const (
	contextSceneAction     = "我在坊市外头碰见宋观澜，想请他看看手里这半块残阵"
	contextUnrelatedAction = "我回住处睡了一觉，第二天去坊市打听灵草的行价"
)

// boundWorldConversation builds a story bound to the shipped pack, seeds that
// pack's own world book into the workspace, and commits one turn that puts the
// protagonist and the array student in the same scene with an offer still open.
func boundWorldConversation(t *testing.T) (*Conversation, *interactive.Store, interactive.StorySummary, storypack.Pack) {
	t.Helper()
	workspace := t.TempDir()
	novaDir := t.TempDir()
	pack := fanrenFormalPack(t, novaDir)

	// The pack's world book, written exactly as applying the pack writes it.
	seeder := lore.NewStore(workspace)
	for _, input := range pack.WorldInputs() {
		if _, err := seeder.Create(input); err != nil {
			t.Fatalf("seeding the pack world book entry %s: %v", input.ID, err)
		}
	}

	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "凡人人界",
		Origin:           "越国黄枫谷外的坊市。",
		ReplyTargetChars: 800,
		PackBinding:      pack.Binding(),
	})
	if err != nil {
		t.Fatal(err)
	}
	local := pack.CatalogBySourceKey()["npc_song_guanlan"]
	if local.SourceKey == "" || local.Name == "" {
		t.Fatal("the shipped catalog lost the local array student")
	}
	// A second person stands in the scene with no topic in common with the
	// action. Their entry is reached only by being present, which is the path
	// that carries a character's own dialogue examples.
	family := pack.CatalogBySourceKey()["canon_chen_qiaoqian"]
	if family.SourceKey == "" || family.Name == "" {
		t.Fatal("the shipped catalog lost the family-born disciple")
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main",
		User:     "我在坊市外头遇见了宋观澜",
		Narrative: "坊市外头的土坡上，宋观澜正蹲在一块残破的石阵前。" +
			"他抬头看你手里的半块残阵，说愿意先看看，但你还没答应什么。" +
			"陈巧倩正从坊市那边过来，在几步外停下。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
				Value: "越国黄枫谷外的坊市", Reason: "本回合确认的地点",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: "入秋后的第三日午后", Reason: "本回合确认的时间",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID, "song_guanlan", "chen_qiaoqian"}, Reason: "本回合确认的在场角色",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"把残阵交给他", "先问他要什么", "收起残阵离开", "观察石阵", "打听坊市消息"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{
					{CharacterID: "song_guanlan", Name: local.Name, SourceKey: local.SourceKey},
					{CharacterID: "chen_qiaoqian", Name: family.Name, SourceKey: family.SourceKey},
				},
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "song_guanlan", Kind: "action", Status: "open",
					Statement:    "宋观澜答应先看那半块残阵，但还没看完，也没有说要不要修。",
					Counterparty: interactive.DefaultActorID,
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return NewConversation(store, novaDir, workspace, story.ID, "main", contextSceneAction, story.ReplyTargetChars, &config.Config{Workspace: workspace}), store, story, pack
}

// turnContextFor assembles one turn and returns the final model-visible messages
// together with the ledger the run records for them.
func turnContextFor(t *testing.T, conversation *Conversation, action string) (string, []agentcontext.AuditPart) {
	t.Helper()
	assembled, err := conversation.AssembleModelContext(context.Background(), action, agentcontext.ModelContextInput{
		UserMessage: action,
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The ledger is what the run records for the messages it is about to send, so
	// it is only populated once the assembled context is committed.
	if err := conversation.CommitModelInput(context.Background(), action, assembled); err != nil {
		t.Fatal(err)
	}
	var turn string
	for _, message := range assembled.Messages {
		if strings.Contains(message.Content, "Current Scene (source: committed story state") {
			turn = message.Content
		}
	}
	if turn == "" {
		t.Fatal("the turn context carries no scene block")
	}
	return turn, conversation.ContextLedgerParts()
}

// TestOrdinaryInputSelectsTheMaterialItNeeds is C-NEXT-10's first and third
// points: the player's ordinary sentence, not a keyword command, is what brings
// the relevant lore in, and a character present in the scene arrives with the
// entry that carries their own way of speaking.
func TestOrdinaryInputSelectsTheMaterialItNeeds(t *testing.T) {
	conversation, _, _, pack := boundWorldConversation(t)
	turn, _ := turnContextFor(t, conversation, contextSceneAction)

	// The array student's entry is in the turn, selected because the player named
	// them and because they are standing in this scene.
	entry := worldEntryByID(t, pack, "fanren-char-npc_song_guanlan")
	body := strings.TrimSpace(entry.Content)
	if body == "" {
		t.Fatal("the shipped pack lost the array student's entry body")
	}
	if !strings.Contains(turn, body) {
		t.Fatalf("the array student's own entry did not reach the turn; selected reasons were: %s", loreNoteFromLedger(t, conversation))
	}
	if !strings.Contains(turn, "present in this scene") && !strings.Contains(turn, "named in the player's action") {
		t.Fatalf("the selected entry carries no traceable selection reason: %s", loreNoteFromLedger(t, conversation))
	}

	// A person in another country matches the same topic but must not be put on
	// stage by it.
	remote := worldEntryByID(t, pack, "fanren-char-canon_qi_yunxiao")
	if remote.Content != "" && strings.Contains(turn, strings.TrimSpace(remote.Content)) {
		t.Fatal("an array worker in 元武国 was selected for a 越国 scene")
	}

	// The voice examples in a present character's own entry are what let the
	// narrator write them in character. They arrive as that entry's body, never as
	// a second generated sample. The family-born disciple is in the scene but
	// shares no topic with the action, so only presence can have selected her.
	voice := worldEntryByID(t, pack, "fanren-char-canon_chen_qiaoqian")
	if body := strings.TrimSpace(voice.Content); body != "" && !strings.Contains(turn, body) {
		t.Fatalf("a present character's own entry did not reach the turn: %s", loreNoteFromLedger(t, conversation))
	}
	if !strings.Contains(turn, "## 4. 说话方式与情境反应") {
		t.Fatalf("the present character's dialogue examples did not reach the turn: %s", loreNoteFromLedger(t, conversation))
	}
}

// TestSceneBlockKeepsAnOfferPending is C-NEXT-10's second point: the short
// scene block states the committed place, who is there, and which arrangement is
// still open — so the narrator cannot read an offer as an agreement.
func TestSceneBlockKeepsAnOfferPending(t *testing.T) {
	conversation, _, _, _ := boundWorldConversation(t)
	turn, _ := turnContextFor(t, conversation, contextSceneAction)

	for _, want := range []string{
		"## Current Scene (source: committed story state and character registry)",
		"- Place: 越国黄枫谷外的坊市",
		"- Time: 入秋后的第三日午后",
		"- Present:",
		"宋观澜 (id: song_guanlan)",
		"- Still open — these are not agreed, not done, and not decided:",
		"[action] 宋观澜答应先看那半块残阵，但还没看完，也没有说要不要修。",
		"open, not settled",
	} {
		if !strings.Contains(turn, want) {
			t.Fatalf("the scene block is missing %q", want)
		}
	}
	// The block is a statement of the scene, not a second knowledge base: it must
	// not carry the character's whole entry again.
	for _, entry := range []string{"## 1. 身份与近况", "## 7. 与谁有往来"} {
		if strings.Contains(sceneBlockOf(t, turn), entry) {
			t.Fatalf("the scene block copied the character entry section %q", entry)
		}
	}
}

// TestContextSourcesAreAuditable is C-NEXT-10's fourth point: the run ledger can
// name what was selected, why, and how large it was, without storing a second
// copy of the body.
func TestContextSourcesAreAuditable(t *testing.T) {
	conversation, _, _, _ := boundWorldConversation(t)
	turn, ledger := turnContextFor(t, conversation, contextSceneAction)

	var lorePart, scenePart *agentcontext.AuditPart
	for index := range ledger {
		switch ledger[index].Source {
		case "LoreContext":
			lorePart = &ledger[index]
		case "SceneContext":
			scenePart = &ledger[index]
		}
	}
	if lorePart == nil || lorePart.Bytes == 0 || !lorePart.Included {
		t.Fatalf("the lore working set is not audited as included: %#v", ledger)
	}
	if !strings.Contains(lorePart.Note, "selected=") || !strings.Contains(lorePart.Note, "source=") {
		t.Fatalf("the lore ledger note does not record what was selected and why: %q", lorePart.Note)
	}
	if !strings.Contains(lorePart.Note, "fanren-char-npc_song_guanlan") {
		t.Fatalf("the lore ledger note does not name the selected entry: %q", lorePart.Note)
	}
	if scenePart == nil || scenePart.Bytes == 0 || !scenePart.Included {
		t.Fatalf("the scene block is not audited as included: %#v", ledger)
	}
	// The ledger measures the body rather than duplicating it: the note stays
	// metadata-sized even when the entry is long.
	if len([]byte(lorePart.Note)) > 2000 {
		t.Fatalf("the lore ledger note is a second copy of the body: %d bytes", len([]byte(lorePart.Note)))
	}
	// The selection record must also be reachable from the assembled turn, which
	// is what the audit is for: a reader of the input can tell why the entry is
	// there.
	if !strings.Contains(turn, "named in the player's action") && !strings.Contains(turn, "present in this scene") {
		t.Fatal("the final model input does not state why the entry was selected")
	}
}

// TestUnrelatedIntentDoesNotKeepTheOldTopic is the last half of the delivery
// scenario: after the scene changes to a different intent, the previous topic's
// material stops being carried every turn.
func TestUnrelatedIntentDoesNotKeepTheOldTopic(t *testing.T) {
	first, store, story, pack := boundWorldConversation(t)
	// The protagonist leaves before the unrelated intent. Without this the array
	// student is still standing in the committed scene, and carrying their entry
	// would be the scene being right rather than the old topic being sticky.
	leaveTheScene(t, store, story)
	next := NewConversation(store, first.novaDir, first.workspace, story.ID, "main", contextUnrelatedAction, story.ReplyTargetChars, first.cfg)
	turn, _ := turnContextFor(t, next, contextUnrelatedAction)

	entry := worldEntryByID(t, pack, "fanren-char-npc_song_guanlan")
	if body := strings.TrimSpace(entry.Content); body != "" && strings.Contains(turn, body) {
		t.Fatalf("the array student's body was carried into an unrelated turn; note: %s", loreNoteFromLedger(t, next))
	}
	// The pack's own 丹药 material is what this intent is about, and the index
	// pass is the path that reaches it.
	if !strings.Contains(turn, "lore index match") && !strings.Contains(turn, "matches this request") {
		t.Fatalf("no material matched the unrelated intent either; note: %s", loreNoteFromLedger(t, next))
	}
}

// TestOrdinaryInputReachesTheWorldsOwnRules is N8's wiring: the world book now
// states rules about ordinary life — how a craft is learned, how a beast is
// approached, what a promise does over time — and an ordinary sentence about
// that life has to reach the rule without naming it.
//
// The words come from the entries themselves. The pack's topic vocabulary does
// not contain 炼丹 or 炼器, so before this the sentence selected nothing at all;
// the index pass now asks about the entries' own keywords too, which is the
// same retrieval the tool uses rather than a second one.
func TestOrdinaryInputReachesTheWorldsOwnRules(t *testing.T) {
	conversation, _, _, pack := boundWorldConversation(t)

	for _, test := range []struct {
		action string
		entry  string
	}{
		{"我想学炼丹，先看看坊市里有没有人肯教。", "world-life-cultivation-craft"},
		{"我在山里遇见一头受伤的灵兽，想看看能不能帮它。", "world-life-beast-companionship"},
		{"我去找同门问问师父这几日有没有差事派下来。", "world-life-organization"},
		{"我想打听一下最近山里出的消息，还有先前答应过的事。", "world-life-time-and-consequences"},
	} {
		t.Run(test.entry, func(t *testing.T) {
			entry := worldEntryByID(t, pack, test.entry)
			body := strings.TrimSpace(entry.Content)
			if body == "" {
				t.Fatalf("the shipped pack lost %s", test.entry)
			}
			turn, _ := turnContextFor(t, conversation, test.action)
			if !strings.Contains(turn, body) {
				t.Fatalf("%q did not reach %s; note: %s", test.action, test.entry, loreNoteFromLedger(t, conversation))
			}
			if !strings.Contains(turn, "lore index match") {
				t.Fatalf("%s arrived without the index pass naming it; note: %s", test.entry, loreNoteFromLedger(t, conversation))
			}
		})
	}
}

// TestAnUnrelatedSentenceBringsNoWorldRule is the other half: widening the
// vocabulary must not turn every sentence into a rules lookup. A sentence about
// sleeping and walking to the market matches no authored keyword, so no rule
// entry may be selected for it.
func TestAnUnrelatedSentenceBringsNoWorldRule(t *testing.T) {
	conversation, _, _, pack := boundWorldConversation(t)
	turn, _ := turnContextFor(t, conversation, "我回住处睡了一觉，第二天早起扫了扫院子。")

	for _, id := range []string{
		"world-life-cultivation-craft",
		"world-life-beast-companionship",
		"world-life-organization",
		"world-life-time-and-consequences",
	} {
		entry := worldEntryByID(t, pack, id)
		if body := strings.TrimSpace(entry.Content); body != "" && strings.Contains(turn, body) {
			t.Fatalf("an unrelated sentence selected %s; note: %s", id, loreNoteFromLedger(t, conversation))
		}
	}
}

// TestManualLoreIsReadOnlyWhenItIsReferenced is the §2 deviation the design
// names outright: the shared selector excluded resident entries but not manual
// ones, so a manual entry was pulled in by being present in the scene or by a
// topic match — exactly the two paths "指定时参考" says must not fetch it. The
// setting then did nothing for any entry the index could reach.
//
// The family-born disciple is the sharpest witness: she stands in the committed
// scene and carries her own topic, so both silent paths reach her entry.
func TestManualLoreIsReadOnlyWhenItIsReferenced(t *testing.T) {
	conversation, _, _, pack := boundWorldConversation(t)
	const entryID = "fanren-char-canon_chen_qiaoqian"
	entry := worldEntryByID(t, pack, entryID)
	body := strings.TrimSpace(entry.Content)
	if body == "" {
		t.Fatalf("the shipped pack lost %s", entryID)
	}
	scoped := lore.NewScopedStore(conversation.workspace, pack.WorldScope())
	item, err := scoped.Get(interactive.PackEntryIDForRevision(entryID, pack.ContentHash()))
	if err != nil {
		t.Fatalf("the seeded pack entry is unreadable: %v", err)
	}
	if _, err := scoped.Update(item.ID, lore.ItemInput{
		Type:             item.Type,
		Name:             item.Name,
		Importance:       item.Importance,
		Tags:             item.Tags,
		BriefDescription: item.BriefDescription,
		Keywords:         item.Keywords,
		LoadMode:         lore.LoadModeManual,
		Content:          item.Content,
	}); err != nil {
		t.Fatalf("marking the entry manual: %v", err)
	}

	// Present, and her topic is untouched by this action, but neither is an ask.
	turn, _ := turnContextFor(t, conversation, contextSceneAction)
	if strings.Contains(turn, body) {
		t.Fatalf("a manual entry was fetched by the scene's cast alone; note: %s",
			loreNoteFromLedger(t, conversation))
	}

	// The player naming her is the explicit reference the setting allows.
	named := NewConversation(conversation.store, conversation.novaDir, conversation.workspace,
		conversation.storyID, "main", "我去找陈巧倩问问她家里的事",
		conversation.replyTargetChars, conversation.cfg)
	namedTurn, _ := turnContextFor(t, named, "我去找陈巧倩问问她家里的事")
	if !strings.Contains(namedTurn, body) {
		t.Fatalf("a manual entry was withheld even though the player named her; note: %s",
			loreNoteFromLedger(t, named))
	}
}

// leaveTheScene commits the protagonist walking away, so the next turn's scene
// no longer holds the array student. Their open arrangement survives — it is a
// thread, not a presence — which is exactly the distinction this file is about.
func leaveTheScene(t *testing.T, store *interactive.Store, story interactive.StorySummary) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我谢过宋观澜，先回坊市去",
		Narrative: "你把残阵收进袖里，跟他约好改日再说，便沿土坡回了坊市。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
				Value: "越国黄枫谷外的坊市", Reason: "本回合确认的地点",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID}, Reason: "宋观澜留在土坡，未随行",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"打听灵草行价", "回住处歇息", "去药铺看看", "再出坊市", "找地方修炼"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func worldEntryByID(t *testing.T, pack storypack.Pack, id string) storypack.WorldEntry {
	t.Helper()
	for _, entry := range pack.World {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("the shipped pack no longer holds world entry %s", id)
	return storypack.WorldEntry{}
}

func loreNoteFromLedger(t *testing.T, conversation *Conversation) string {
	t.Helper()
	for _, part := range conversation.ContextLedgerParts() {
		if part.Source == "LoreContext" {
			return part.Note
		}
	}
	return "(no LoreContext part was recorded)"
}

func sceneBlockOf(t *testing.T, turn string) string {
	t.Helper()
	start := strings.Index(turn, "## Current Scene (source: committed story state")
	if start < 0 {
		t.Fatal("the turn carries no scene block")
	}
	end := strings.Index(turn[start:], "\n## ")
	if end < 0 {
		return turn[start:]
	}
	return turn[start : start+end]
}
