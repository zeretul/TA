package app

import (
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// This file covers the F22 gap that 自制-叙事演绎 §5.5 names as 后果必须有消费者 and
// that 自制-游玩循环与选择后果 §5 restates: 例如NPC后续提及确实答应的事、在相关事件中
// 认可已经履行的帮助. A thread the protagonist actually carried out has to remain
// readable afterwards, or the narrator writes a person who has forgotten the
// help they just received.
//
// The two halves of the context disagreed about exactly this. The character
// window's preparation (character.Prepare → prepareInteractions) prints an
// "Already resolved" section, while the main story's cast block printed only
// open and lapsed threads. So the same person remembered a delivered duty in the
// window and had no record of it in the story — and a narrator with no record
// writes the debt again, or asks for work already done.
//
// Everything here runs on F15's own fixture: a real 黄枫谷 外门弟子, the pack's
// own world book, and the ordinary turn path.

const (
	// dutyDeliveredStatement is what the delivery turn records. It is a separate
	// record from the acceptance, which is what keeps 承诺 and 履行 apart.
	dutyDeliveredStatement = "东畦查完了，病株记在册子上交回给慕沛灵。"
)

// deliverTheDuty commits the turn in which the protagonist actually carries out
// the inspection and hands the book back.
func (f sectDuty) deliverTheDuty(t *testing.T, resolves string) {
	t.Helper()
	if _, _, err := f.store.AppendTurnWithState(f.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我查完东畦，把册子交回给慕管事。",
		Narrative: "你把东畦的病株一株株记进册子，收工时交回慕沛灵手里。她翻了一遍，点了点头。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"问她明日还有什么差事", "回居所歇一歇", "去外门事务处走一趟"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "settled",
					Statement:    dutyDeliveredStatement,
					Counterparty: interactive.DefaultActorID,
					Resolves:     resolves,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the delivery: %v", err)
	}
}

// TestADeliveredDutyStaysReadableInTheStory is the gap: after the work is done,
// the next turn's context still has to say that this person was helped.
func TestADeliveredDutyStaysReadableInTheStory(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)
	fixture.deliverTheDuty(t, original.ID)

	// The record exists: the delivery settled the acceptance and is itself on
	// file. The failure below is therefore about the context, not about storage.
	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after delivery: %v", err)
	}
	delivered := dutyMatter(t, state)
	if delivered.Status != character.MatterSettled {
		t.Fatalf("the delivered duty is still %q", delivered.Status)
	}

	// The next turn must still know what was done. Both halves of the context
	// read one registry, so the window and the story must agree about it.
	action := "我回居所把今日的事记下来。"
	conversation := fixture.conversation(action)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)

	if !strings.Contains(all, dutyDeliveredStatement) {
		t.Fatalf("the turn no longer holds the help the protagonist actually gave; "+
			"a narrator with no record of it will ask for the work again:\n%s", all)
	}

	// It is reported as done, not as still owed: §5.5 结束事件不要求专门奖励面板, but
	// an ended thread must not keep presenting itself as outstanding work.
	if strings.Contains(all, "- Unfinished with the protagonist:") {
		t.Fatalf("the delivered duty is still listed among the unfinished threads:\n%s", all)
	}

	// The person the help was given to can refer to it, which is the consumer
	// §5.5 asks for. The cast block is where the narrator reads who this person
	// is, so the settled thread has to appear there beside their identity.
	cast := dutyCastBlock(t, all)
	if !strings.Contains(cast, dutyDeliveredStatement) {
		t.Fatalf("the cast block does not let the narrator acknowledge the delivered help:\n%s", cast)
	}
	if !strings.Contains(cast, "settled") {
		t.Fatalf("the cast block does not say the thread was carried out:\n%s", cast)
	}
}

// TestASettledThreadIsNotSettledAgain keeps the existing rule intact: making a
// settled thread visible must not make it look like work still outstanding, and
// must not give the narrator a second chance to record the same delivery.
func TestASettledThreadIsNotSettledAgain(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)
	fixture.deliverTheDuty(t, original.ID)
	fixture.deliverTheDuty(t, original.ID)

	state, err = fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry after the repeated delivery: %v", err)
	}
	if len(state.Matters) != 2 {
		t.Fatalf("repeating the delivery left %d records, want the duty and its one delivery", len(state.Matters))
	}
	settledCount := 0
	for _, matter := range state.Matters {
		if matter.Normalize().Status == character.MatterSettled {
			settledCount++
		}
	}
	if settledCount != 2 {
		t.Fatalf("%d settled records, want the acceptance and its one delivery", settledCount)
	}
}

// TestAnAbandonedThreadStaysReadable is the other half of the same gap: a thread
// that lapsed or was refused is an outcome too, and §5.5 失效或被放弃的线索保留历史.
func TestAnAbandonedThreadStaysReadable(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)

	state, err := fixture.store.CharacterState(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the character registry: %v", err)
	}
	original := dutyMatter(t, state)

	const refused = "东畦的差事顾承川没接，慕沛灵另派了别人。"
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我跟慕管事说这差事我接不了。",
		Narrative: "慕沛灵看了你一会儿，把册子收了回去，说那就另找人。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"去外门事务处问问别的差事", "回居所", "留在药园看看"},
			CharacterFacts: &interactive.TurnCharacterFactsInput{
				Matters: []interactive.TurnCharacterMatterInput{{
					CharacterID: "mu_peiling", Kind: "action", Status: "abandoned",
					Statement:    refused,
					Counterparty: interactive.DefaultActorID,
					Resolves:     original.ID,
				}},
			},
		},
	}); err != nil {
		t.Fatalf("commit the refusal: %v", err)
	}

	action := "我去外门事务处看看还有没有别的差事。"
	conversation := fixture.conversation(action)
	assembled, _ := assembleDutyTurn(t, conversation, action)
	all := dutyMessageText(assembled)

	if !strings.Contains(all, refused) {
		t.Fatalf("the turn lost the refusal, so the narrator can offer the same work again:\n%s", all)
	}
	if strings.Contains(all, "- Unfinished with the protagonist:") {
		t.Fatalf("a refused thread is still listed as unfinished:\n%s", all)
	}
}

// dutyCastBlock returns the established-characters section of a turn, which is
// where the narrator reads who a person is and what stands between them.
func dutyCastBlock(t *testing.T, turn string) string {
	t.Helper()
	const header = "Characters this story has established."
	start := strings.Index(turn, header)
	if start < 0 {
		t.Fatal("the turn carries no cast block")
	}
	rest := turn[start:]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}
