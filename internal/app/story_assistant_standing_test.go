package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/quest"
)

// This file covers the half of 自制-剧情助手与选项分析 that says the assistant reads
// 当前相关上下文、历史承诺与期限.
//
// Before this the assistant read only the current turn's prose tail: a promise
// made three turns ago, a permission with an hour attached, or a task whose
// limit had already run out were all invisible to it, and it advised as though
// the scene had no history. The records existed and the scene block and the task
// panel already showed them; the advisor simply never asked.
//
// Everything here reads the context builder directly rather than going through a
// model call, because what can be proven offline is what the model is allowed to
// see. The read-only requirement is covered separately by
// TestAskingTheAssistantWritesNothing, which compares the journal around a real
// call.

// assistantStandingApp wraps one story fixture as the App the assistant reads
// through. The assistant needs only the store and the workspace; the fixture's
// own creation service is not part of the read path.
func assistantStandingApp(fixture sectDuty) *App {
	return &App{workspace: fixture.workspace, interactive: fixture.store}
}

// TestTheAssistantReadsAStandingPromiseAndItsHour is the core gap: an open
// thread with an agreed hour has to reach the advisor, with the hour stated.
//
// The wording is checked rather than only the presence of the statement, because
// the point of the block is that an open arrangement is not a settled fact. An
// advisor told "查验东畦" with no qualifier would read it as work already done.
func TestTheAssistantReadsAStandingPromiseAndItsHour(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	app := assistantStandingApp(fixture)
	storyContext, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	known := storyAssistantKnownContext(app.interactive, storyContext)

	if !strings.Contains(known, "### What is still standing") {
		t.Fatalf("the assistant is not told what is still standing:\n%s", known)
	}
	for _, want := range []string{
		"- Open — these are not agreed, not done, and not decided:",
		dutyStatement,
		"with 慕沛灵",
		"open, not settled",
		"due at " + dutyDeadline,
	} {
		if !strings.Contains(known, want) {
			t.Fatalf("the assistant's context is missing %q:\n%s", want, known)
		}
	}
	if strings.Contains(known, "Came due") {
		t.Fatalf("a thread whose hour has not come is reported as lapsed:\n%s", known)
	}
}

// TestTheAssistantReadsAThreadThatCameDue covers the other side of 期限: once the
// hour passes, the advisor must be told the arrangement no longer binds, and
// must be told it in the same words the story and the character window use.
func TestTheAssistantReadsAThreadThatCameDue(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	lapseTheAgreedHour(t, fixture)

	app := assistantStandingApp(fixture)
	storyContext, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	known := storyAssistantKnownContext(app.interactive, storyContext)

	if !strings.Contains(known, "- Came due —") {
		t.Fatalf("the assistant is not told the hour passed:\n%s", known)
	}
	if !strings.Contains(known, character.LapseClause(character.MatterAction)) {
		t.Fatalf("the lapsed thread is not stated in the shared wording:\n%s", known)
	}
	if !strings.Contains(known, dutyStatement) {
		t.Fatalf("the lapsed thread vanished instead of being reported:\n%s", known)
	}
	// It must not still be offered as open work, which is what would have the
	// advisor plan around an obligation that no longer exists.
	if strings.Contains(known, dutyStatement+" (with 慕沛灵; open, not settled") {
		t.Fatalf("a lapsed thread is still listed as open:\n%s", known)
	}
}

// TestTheAssistantReadsASpentPermissionAsSpent is §4's 玩家失去权限后不得继续调资源
// applied to the advisor: a right that ran out is not an act left undone, and the
// advice must not be built on the right still standing.
func TestTheAssistantReadsASpentPermissionAsSpent(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	const permission = "慕沛灵许顾承川从东畦取一份药苗回去试种，只此一份，今日之内。"
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我问她能不能取一份东畦的药苗。",
		Narrative: "慕沛灵想了一会儿，说东畦的药苗可以取一份，今日之内，多了不行。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"现在就去取", "等查完东畦再说", "先问清是哪一种药苗"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "permission", Status: "open",
					Statement:    permission,
					Counterparty: interactive.DefaultActorID,
					Deadline:     dutyDeadline,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the granted permission: %v", err)
	}
	lapseTheAgreedHour(t, fixture)

	app := assistantStandingApp(fixture)
	storyContext, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	known := storyAssistantKnownContext(app.interactive, storyContext)

	if !strings.Contains(known, permission) {
		t.Fatalf("the spent permission is missing from the assistant's context:\n%s", known)
	}
	if !strings.Contains(known, character.LapseClause(character.MatterPermission)) {
		t.Fatalf("the spent permission is not stated as no longer authorising:\n%s", known)
	}
	if strings.Contains(known, permission+" (with 慕沛灵; open, not settled") {
		t.Fatalf("the spent permission is still offered as in force:\n%s", known)
	}
}

// TestTheAssistantReadsTakenWorkAndItsDeadline covers the other record that
// carries a deadline: a task the protagonist has accepted.
//
// An offered-but-untaken task is deliberately absent — it is a choice still open,
// not a commitment — and the test pins that too, because an advisor planning
// around an obligation the player never took would be wrong in a way the player
// cannot see.
func TestTheAssistantReadsTakenWorkAndItsDeadline(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)

	app := assistantStandingApp(fixture)
	storyContext, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	known := storyAssistantKnownContext(app.interactive, storyContext)

	if !strings.Contains(known, "### Work the protagonist has taken on") {
		t.Fatalf("the assistant is not told what work is in hand:\n%s", known)
	}
	if !strings.Contains(known, "分清这一批药材") {
		t.Fatalf("the accepted task is missing from the assistant's context:\n%s", known)
	}
	if !strings.Contains(known, "due ") {
		t.Fatalf("the accepted task's limit is not stated:\n%s", known)
	}
	// The untaken errand is still only an offer.
	state, err := fixture.store.QuestStateFor(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range state.Available {
		if title := strings.TrimSpace(task.Title); title != "" && strings.Contains(known, title) {
			t.Fatalf("an untaken offer (%s) is presented as work in hand:\n%s", title, known)
		}
	}
}

// TestTheAssistantReadsAnExpiredTaskAsExpired keeps the advisor from promising a
// limit the rules have already closed.
func TestTheAssistantReadsAnExpiredTaskAsExpired(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)
	secludeFor(t, fixture, 3*24*60*60)

	app := assistantStandingApp(fixture)
	storyContext, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	known := storyAssistantKnownContext(app.interactive, storyContext)

	if !strings.Contains(known, "分清这一批药材") {
		t.Fatalf("the expired task is missing from the assistant's context:\n%s", known)
	}
	if !strings.Contains(known, "the agreed moment passed before the work was finished") {
		t.Fatalf("the expired task is not reported as expired:\n%s", known)
	}
	// The expired task's own line must not also carry a running countdown: the
	// rules closed the limit, and advising on the remaining time would be advice
	// about a moment that has already gone.
	for _, line := range strings.Split(known, "\n") {
		if !strings.Contains(line, "分清这一批药材") {
			continue
		}
		if strings.Contains(line, "due ") {
			t.Fatalf("an expired task still carries a running deadline: %q", line)
		}
	}
}

// TestTheAssistantTaskBlockSkipsUntakenOffers pins the filter on its own, so the
// rule is stated where it is implemented rather than only through a fixture.
func TestTheAssistantTaskBlockSkipsUntakenOffers(t *testing.T) {
	var sb strings.Builder
	writeStoryAssistantTasks(&sb, []interactive.QuestStateView{
		{TaskID: "offered", Title: "还没接的差事", Status: string(quest.StatusOffered)},
		{TaskID: "taken", Title: "已经接下的差事", Status: string(quest.StatusAccepted), DeadlineText: "约 2 天后"},
	})
	rendered := sb.String()
	if strings.Contains(rendered, "还没接的差事") {
		t.Fatalf("an untaken offer is listed as taken work:\n%s", rendered)
	}
	if !strings.Contains(rendered, "已经接下的差事") || !strings.Contains(rendered, "due 约 2 天后") {
		t.Fatalf("taken work and its limit are missing:\n%s", rendered)
	}
}
