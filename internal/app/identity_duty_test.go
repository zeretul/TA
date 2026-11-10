package app

import (
	"context"
	"strings"
	"testing"

	"ta/config"
	agentcontext "ta/internal/agents/context"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file covers F15's remaining gap: one identity opportunity and the
// obligation it creates, carried by the records the game already keeps rather
// than by a new daily-life system.
//
// The design (自制-身份与日常生活 §2–§4, §8) asks that a 宗门弟子's access
// follow their real position: a bounded duty handed over by the person who
// actually runs the place, not a blanket permission to the sect's stores. It
// also asks that an accepted arrangement be recorded as accepted and delivered
// as delivered — "申请、承诺和真正交付分开" — and that the same thread survive a
// retry, a settlement and a branch fork.
//
// Everything here runs on what already ships: the 凡人人界 pack's own 黄枫谷药园
// material, the world book's organization rule, the character projection's
// matters, and the scene block. No new engine and no new state.

const (
	dutySceneAction = "我到黄枫谷药园找慕管事，问今日交接的差事该怎么分。"
	dutyWorldTime   = "第3日 午后"
	dutyDeadline    = "第3日 酉时"
	// The thread states what was handed over and what is still outstanding. It
	// is deliberately not phrased as the work being finished: a permission or an
	// assignment is not the act it authorises.
	dutyStatement = "慕沛灵把药园东畦的查验交给顾承川，说好今日收工前回报，人还没回来。"
)

// sectDuty is one story started through the real creation path, with the pack's
// world book seeded exactly as applying the pack writes it.
type sectDuty struct {
	store     *interactive.Store
	story     interactive.StorySummary
	pack      storypack.Pack
	dataRoot  string
	workspace string
}

func startSectDutyStory(t *testing.T) sectDuty {
	t.Helper()
	dataRoot := t.TempDir()
	workspace := t.TempDir()
	service := &InteractiveAppService{app: &App{
		workspace: dataRoot,
		cfg:       &config.Config{DenovaDir: dataRoot},
	}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatalf("read the 凡人 pack: %v", err)
	}

	// A real 黄枫谷 外门弟子: the identity is what grounds the duty, so it has to
	// come from the same creation path the game uses rather than being written
	// straight into the request.
	req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), storypack.IdentityChoice{
		Name:                 "顾承川",
		Gender:               "male",
		Age:                  22,
		OriginID:             "custom",
		OriginText:           "越国小镇的普通人家",
		StateBasisTemplateID: "sect-disciple",
		AffiliationID:        "yue.huangfeng",
		LocationID:           "yue.huangfeng.approach",
		RootCategoryID:       "triple",
		RootAttributes:       []string{"wood", "water", "earth"},
		AptitudeID:           "normal",
		WeaponID:             "fist",
		TraitIDs:             []string{"steady"},
		Identity:             "黄枫谷外门弟子，腰间挂着外门弟子牌。",
		Appearance:           "身形偏瘦，眉眼平和，穿一件洗得发白的弟子服。",
		Background:           "入谷后没有靠山，靠按时的差事换取修行资源。",
		Situation:            "今日的差事已经派下。",
		Predicament:          "月底前凑不齐那笔灵石。",
	})
	if err != nil {
		t.Fatalf("fold the confirmed identity into the request: %v", err)
	}
	req, err = service.withStoryCreationDefaults(req)
	if err != nil {
		t.Fatalf("apply creation defaults: %v", err)
	}
	req, err = service.withPackTemplate(req)
	if err != nil {
		t.Fatalf("seed the state basis: %v", err)
	}

	seeder := lore.NewStore(workspace)
	for _, input := range pack.WorldInputs() {
		if _, err := seeder.Create(input); err != nil {
			t.Fatalf("seeding the pack world book entry %s: %v", input.ID, err)
		}
	}

	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	return sectDuty{store: store, story: story, pack: pack, dataRoot: dataRoot, workspace: workspace}
}

// takeUpTheDuty commits the turn in which the 药园管事 hands over one bounded
// piece of work. The prose offers a specific inspection and a time to report
// back; it does not hand over the garden, its rare stock, or a standing right
// to the sect's supplies.
func (f sectDuty) takeUpTheDuty(t *testing.T) interactive.TurnEvent {
	t.Helper()
	mu := f.pack.CatalogBySourceKey()["npc_mu_peiling"]
	if mu.SourceKey == "" || mu.Name == "" {
		t.Fatal("the shipped catalog lost the 药园管事")
	}
	turn, _, err := f.store.AppendTurnWithState(f.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main",
		User:     dutySceneAction,
		Narrative: "药园的东畦刚浇过水。慕沛灵把手里那本记着畦号的册子合上，说今日只查东畦这一片，" +
			"看有没有病株，收工前把结果报回来。她把话说得很清楚：查的是这一片，不是整座药园，" +
			"园里的药材一株也不许动。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
				Value: "越国黄枫谷药园", Reason: "本回合确认的地点",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: dutyWorldTime, Reason: "本回合确认的时间",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "在场角色",
				Value: []any{interactive.DefaultActorID, "mu_peiling"}, Reason: "本回合确认的在场角色",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"先去东畦看看", "问清楚病株是什么样子", "说今日怕是赶不及"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Profiles: []interactive.TurnCharacterProfileInput{{
					CharacterID: "mu_peiling", Name: mu.Name, SourceKey: mu.SourceKey,
					Role: "药园管事",
				}},
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "open",
					Statement:    dutyStatement,
					Counterparty: interactive.DefaultActorID,
					Deadline:     dutyDeadline,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("commit the turn that takes up the duty: %v", err)
	}
	return turn
}

func (f sectDuty) conversation(action string) *interactiveapp.Conversation {
	return interactiveapp.NewConversation(
		f.store, f.dataRoot, f.workspace, f.story.ID, "main", action, f.story.ReplyTargetChars,
		&config.Config{DenovaDir: f.dataRoot, Workspace: f.workspace},
	)
}

// assembleDutyTurn assembles and commits one turn's context, which is what makes
// the run's ledger readable.
func assembleDutyTurn(t *testing.T, conversation *interactiveapp.Conversation, action string) (agentcontext.ModelContextResult, []agentcontext.AuditPart) {
	t.Helper()
	assembled, err := conversation.AssembleModelContext(context.Background(), action, agentcontext.ModelContextInput{
		UserMessage: action,
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatalf("assemble the turn's model context: %v", err)
	}
	if err := conversation.CommitModelInput(context.Background(), action, assembled); err != nil {
		t.Fatalf("commit the assembled context: %v", err)
	}
	return assembled, conversation.ContextLedgerParts()
}

func dutyMessageText(assembled agentcontext.ModelContextResult) string {
	var sb strings.Builder
	for _, message := range assembled.Messages {
		sb.WriteString(message.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

func dutyLedgerNote(parts []agentcontext.AuditPart, source string) string {
	for _, part := range parts {
		if part.Source == source {
			return part.Note
		}
	}
	return ""
}

func dutySceneBlock(t *testing.T, turn string) string {
	t.Helper()
	const header = "## Current Scene (source: committed story state and character registry)"
	start := strings.Index(turn, header)
	if start < 0 {
		t.Fatal("the turn carries no scene block")
	}
	end := strings.Index(turn[start:], "\n## ")
	if end < 0 {
		return turn[start:]
	}
	return turn[start : start+end]
}

// dutyMatter finds the committed thread by the statement the prose stated.
func dutyMatter(t *testing.T, state character.State) character.Matter {
	t.Helper()
	for _, matter := range state.Matters {
		if strings.TrimSpace(matter.Normalize().Statement) == dutyStatement {
			return matter.Normalize()
		}
	}
	t.Fatalf("the story holds no committed thread for %q; it holds %d matters", dutyStatement, len(state.Matters))
	return character.Matter{}
}

// TestASectDutyIsGroundedInTheIdentityAndKeptOpen is the first half of F15: an
// ordinary sentence takes up one bounded piece of work, the turn is answered
// from the identity and the world's own rule, and what it leaves outstanding is
// recorded as outstanding rather than as done.
func TestASectDutyIsGroundedInTheIdentityAndKeptOpen(t *testing.T) {
	fixture := startSectDutyStory(t)
	turn := fixture.takeUpTheDuty(t)

	// The obligation is committed in the same transaction as the turn, stamped
	// with where and when it was made — not written later by a window.
	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	matter := dutyMatter(t, state)
	if matter.Kind != character.MatterAction {
		t.Fatalf("the thread is a %q, want an act someone committed to", matter.Kind)
	}
	if matter.Status != character.MatterOpen {
		t.Fatalf("accepting the duty settled it: status = %q", matter.Status)
	}
	if matter.CharacterID != "mu_peiling" {
		t.Fatalf("the thread is with %q, want the 药园管事", matter.CharacterID)
	}
	if matter.Counterparty != interactive.DefaultActorID {
		t.Fatalf("counterparty = %q, want the protagonist", matter.Counterparty)
	}
	if matter.TurnID != turn.ID {
		t.Fatalf("the thread is stamped to turn %q, want %q", matter.TurnID, turn.ID)
	}
	if strings.TrimSpace(matter.SourceEventID) == "" {
		t.Fatal("the thread carries no source event")
	}
	if strings.TrimSpace(matter.WorldTime) != dutyWorldTime {
		t.Fatalf("the thread's world time = %q, want %q", matter.WorldTime, dutyWorldTime)
	}
	if matter.Deadline != dutyDeadline {
		t.Fatalf("the thread's deadline = %q, want the one the prose named, %q", matter.Deadline, dutyDeadline)
	}

	// The next turn is told who the protagonist is in the sect's own terms, so
	// the duty is grounded in position rather than in a keyword.
	nextAction := "我先去东畦看头几垄，回来再向慕管事回差事。"
	conversation := fixture.conversation(nextAction)
	assembled, parts := assembleDutyTurn(t, conversation, nextAction)
	all := dutyMessageText(assembled)
	for _, want := range []string{"所属：黄枫谷", "落脚：黄枫谷对外入口"} {
		if !strings.Contains(all, want) {
			t.Fatalf("the turn does not state the protagonist's real position (%q)", want)
		}
	}

	// The world's own rule about duties reaches the turn through the ordinary
	// words of the sentence, and the ledger says how it was selected.
	entry := dutyWorldEntry(t, fixture.pack, "world-life-organization")
	if body := strings.TrimSpace(entry.Content); body != "" && !strings.Contains(all, body) {
		t.Fatalf("the organization rule did not reach the turn; note: %s", dutyLedgerNote(parts, "LoreContext"))
	}
	if note := dutyLedgerNote(parts, "LoreContext"); !strings.Contains(note, "world-life-organization") || !strings.Contains(note, "lore index match") {
		t.Fatalf("the rule arrived without the index pass naming it: %q", note)
	}

	// The scene block repeats the thread as open, which is the sentence that
	// stops the narrator writing it as finished.
	scene := dutySceneBlock(t, all)
	for _, want := range []string{
		"- Place: 越国黄枫谷药园",
		"- Time: " + dutyWorldTime,
		"慕沛灵 (id: mu_peiling)",
		"- Still open — these are not agreed, not done, and not decided:",
		"[action] " + dutyStatement,
		"open, not settled",
	} {
		if !strings.Contains(scene, want) {
			t.Fatalf("the scene block is missing %q", want)
		}
	}

	// The cast block carries the id a later turn settles by, and the rule that
	// separates the assignment from the work.
	for _, want := range []string{
		"- Unfinished with the protagonist:",
		"- id: " + matter.ID,
		"- Lapses at " + dutyDeadline + ".",
		"This is an act the player committed to. It stays open until the world shows it carried out.",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("the cast block is missing %q", want)
		}
	}
}

// TestADutyIsRecordedOnceAndDeliveredOnce is the second half: a retry does not
// create a second obligation, carrying the work out settles the one that exists,
// and repeating the delivery does not settle it again.
func TestADutyIsRecordedOnceAndDeliveredOnce(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)

	// The same arrangement stated again — the narrator restating it on a later
	// turn — is the same thread, not a second one.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我把东畦走过一遍，还没查完。",
		Narrative: "慕沛灵在畦边又叮嘱了一句，还是东畦那一片，收工前回报。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"接着查剩下的几垄", "先去问她一株病株的样子", "回居所取工具"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "open",
					Statement:    dutyStatement,
					Counterparty: interactive.DefaultActorID,
					Deadline:     dutyDeadline,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the restated duty: %v", err)
	}
	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	if len(state.Matters) != 1 {
		t.Fatalf("restating the duty created %d threads, want the one that already existed", len(state.Matters))
	}
	if dutyMatter(t, state).ID != original.ID {
		t.Fatal("the restatement replaced the thread instead of matching it")
	}

	// Carrying the inspection out is a second record that settles the first. The
	// acceptance and the delivery stay two facts.
	settledStatement := "东畦查完了，病株记在册子上交回给慕沛灵。"
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我查完东畦，把册子交回给慕管事。",
		Narrative: "你把东畦的病株一株株记进册子，收工时交回慕沛灵手里。她翻了一遍，点了点头。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前时间",
				Value: "第3日 酉时", Reason: "本回合确认的时间",
			},
		},
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她明日还有什么差事", "回居所歇一歇", "去外门事务处走一趟"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    settledStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the delivery: %v", err)
	}
	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	delivered := dutyMatter(t, state)
	if delivered.Status != character.MatterSettled {
		t.Fatalf("the delivered duty is still %q", delivered.Status)
	}
	if delivered.SettledBy == "" || delivered.SettledBy == original.ID {
		t.Fatalf("the delivery is not recorded as its own fact: settled_by = %q", delivered.SettledBy)
	}
	var delivery character.Matter
	for _, matter := range state.Matters {
		if matter.Normalize().ID == delivered.SettledBy {
			delivery = matter.Normalize()
		}
	}
	if delivery.ID == "" {
		t.Fatalf("the delivery record %q is not on file", delivered.SettledBy)
	}
	if delivery.Status != character.MatterSettled || delivery.Statement != settledStatement {
		t.Fatalf("the delivery record is %+v, want the settled inspection", delivery)
	}

	// Replaying the delivery — a retry, or a turn that restates what already
	// happened — settles nothing twice and adds no record.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我又说了一遍东畦已经交回。",
		Narrative: "你把交回册子的事又说了一遍，慕沛灵只应了一声。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"告辞", "再问一句", "去看看西畦"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    settledStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the repeated delivery: %v", err)
	}
	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	if len(state.Matters) != 2 {
		t.Fatalf("repeating the delivery left %d records, want the duty and its one delivery", len(state.Matters))
	}
	replayed := dutyMatter(t, state)
	if replayed.SettledBy != delivered.SettledBy {
		t.Fatalf("the repeated delivery re-settled the duty: settled_by = %q, want %q", replayed.SettledBy, delivered.SettledBy)
	}

	// The next turn no longer presents it as outstanding work, in either place
	// that reports open threads. With nothing open the scene block omits its
	// open list and the cast block omits its unfinished list, so the test states
	// the duty's absence rather than the header's.
	conversation := fixture.conversation("我回居所把今日的事记下来。")
	assembled, _ := assembleDutyTurn(t, conversation, "我回居所把今日的事记下来。")
	all := dutyMessageText(assembled)
	if strings.Contains(all, "[action] "+dutyStatement) {
		t.Fatal("a delivered duty is still presented as an open act")
	}
	if strings.Contains(all, "- Unfinished with the protagonist:") {
		t.Fatalf("a delivered duty is still listed among the unfinished threads:\n%s", all)
	}
}

// TestAForkAtTheDutyStillOwesIt keeps the branch rule: forking at the turn that
// took the duty gives a branch that has the duty, not one that has already been
// delivered in the main line.
func TestAForkAtTheDutyStillOwesIt(t *testing.T) {
	fixture := startSectDutyStory(t)
	turn := fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)

	// Deliver it in the main line first, so the fork has something to differ from.
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我查完东畦，把册子交回给慕管事。",
		Narrative: "你把东畦的病株记进册子，收工时交回慕沛灵手里。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她明日还有什么差事", "回居所歇一歇", "去外门事务处走一趟"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    "东畦查完了，病株记在册子上交回给慕沛灵。",
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the delivery: %v", err)
	}

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: turn.ID,
		Title:         "先把东畦查完再说",
	})
	if err != nil {
		t.Fatalf("fork at the duty turn: %v", err)
	}
	forked, err := fixture.store.CharacterState(fixture.story.ID, branch.ID)
	if err != nil {
		t.Fatalf("read the fork's character registry: %v", err)
	}
	if len(forked.Matters) != 1 {
		t.Fatalf("the fork holds %d matters, want the one the duty created", len(forked.Matters))
	}
	matter := dutyMatter(t, forked)
	if matter.ID != original.ID {
		t.Fatalf("the fork's thread is %q, want the original %q", matter.ID, original.ID)
	}
	if matter.Status != character.MatterOpen {
		t.Fatalf("the fork inherited the main line's delivery: status = %q", matter.Status)
	}
	if matter.SettledBy != "" {
		t.Fatalf("the fork's thread is settled by %q, which happened only in the main line", matter.SettledBy)
	}

	// And the fork's own turn still states it as outstanding.
	conversation := interactiveapp.NewConversation(
		fixture.store, fixture.dataRoot, fixture.workspace, fixture.story.ID, branch.ID,
		"我先去东畦看头几垄。", fixture.story.ReplyTargetChars,
		&config.Config{DenovaDir: fixture.dataRoot, Workspace: fixture.workspace},
	)
	assembled, _ := assembleDutyTurn(t, conversation, "我先去东畦看头几垄。")
	if !strings.Contains(dutyMessageText(assembled), "[action] "+dutyStatement) {
		t.Fatal("the fork's turn does not carry the duty it still owes")
	}
}

func dutyWorldEntry(t *testing.T, pack storypack.Pack, id string) storypack.WorldEntry {
	t.Helper()
	for _, entry := range pack.World {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("the shipped pack no longer holds world entry %s", id)
	return storypack.WorldEntry{}
}
