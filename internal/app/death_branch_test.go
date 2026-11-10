package app

import (
	"errors"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/quest"
)

// This file covers F21: 死亡与分支结局, END-01…END-06 of
// 设计稿/自制-死亡与分支结局.md.
//
// The carrier already existed and is not rebuilt here. A committed
// TerminalOutcome stops further turns on the same branch, projects onto the
// plot node, and disables the composer in the stage; a fork cut before the
// ending continues normally. What was missing is §3's "结束后分支可读，不能继续
// 接收该已死主角的普通行动、聊天或经营命令": only AppendTurn checked. Every other
// command that moves the branch head — task, journey, public event, realm,
// cultivation, recovery, battle, and the two character record writes — accepted
// its request and stamped it onto a branch whose protagonist was already dead.
//
// The tests below state that rule and the two properties around it that a naive
// fix would break: a command committed before the ending must still replay
// (END-04), and the history must stay readable with a usable fork (END-02,
// END-03).
//
// This test seeds TerminalOutcome directly to exercise branch closure. Natural
// 寿尽 candidates are produced from confirmed realm, calendar, and branch-clock
// data in the story turn path. Other 真死 sources remain out of scope until a
// domain result can produce a legal terminal candidate; the store must not infer
// death from narration or transport/model failure.

// aFinishedBranch commits the ending the design describes: the character takes
// a blow they cannot survive, and the turn that narrates it is the branch's last.
func aFinishedBranch(t *testing.T) (sectDuty, interactive.TurnEvent) {
	t.Helper()
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	turn, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我不闪不避，硬接他这一掌。",
		Narrative: "掌风落在胸口，你听见自己骨头断裂的声音，眼前一黑，再没有醒来。",
		TerminalOutcome: &interactive.TerminalOutcome{
			Terminal: true, Type: "death", Reason: "生命归零，伤势超出承受",
			FinalNarrativeSummary: "你死在了药园外。",
		},
	})
	if err != nil {
		t.Fatalf("commit the ending: %v", err)
	}
	return fixture, turn
}

// liveFork cuts a branch from the turn before the ending. §4 allows exactly
// this, and it is also the control the command table compares against: the same
// request on a branch that has not ended must fail for its own reasons, not
// because of the terminal state.
func liveFork(t *testing.T, fixture sectDuty) string {
	t.Helper()
	ctx, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the story: %v", err)
	}
	if len(ctx.Snapshot.Turns) < 2 {
		t.Fatalf("want a turn before the ending, got %d turns", len(ctx.Snapshot.Turns))
	}
	before := ctx.Snapshot.Turns[len(ctx.Snapshot.Turns)-2].ID
	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: before, Title: "不硬接的那一支",
	})
	if err != nil {
		t.Fatalf("fork before the ending: %v", err)
	}
	return branch.ID
}

// TestAFinishedBranchRefusesEveryCharacterCommand is END-02's second half.
//
// Each request is run twice: once on a branch that has not ended and once on the
// ended one. The live run is the control — whatever it reports, it must not be
// the terminal refusal. The ended run must be the terminal refusal. Because the
// store checks the branch before it validates anything else, this holds for
// well-formed and malformed requests alike, which is why a payload that would be
// refused on its own merits is still enough to prove the gate is there.
func TestAFinishedBranchRefusesEveryCharacterCommand(t *testing.T) {
	fixture, _ := aFinishedBranch(t)
	forkID := liveFork(t, fixture)

	commands := []struct {
		name string
		run  func(branchID string) error
	}{
		{"task", func(branchID string) error {
			_, err := fixture.store.ApplyQuest(fixture.story.ID, interactive.QuestRequest{
				BranchID: branchID, TaskID: quest.TaskSuQianMaterials, Action: interactive.QuestActionAccept,
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"journey", func(branchID string) error {
			_, err := fixture.store.ApplyJourney(fixture.story.ID, interactive.JourneyRequest{
				BranchID: branchID, RouteID: "route-after-death", Arrive: true,
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"public event", func(branchID string) error {
			_, err := fixture.store.ApplyPublicEvent(fixture.story.ID, interactive.PublicEventRequest{
				BranchID: branchID, EventID: "event-after-death", Action: "join",
				ParticipationAction: "observe",
				CommandID:           "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"realm", func(branchID string) error {
			_, err := fixture.store.ApplyRealm(fixture.story.ID, interactive.RealmRequest{
				BranchID: branchID, RealmID: "realm-after-death", Action: "enter",
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"cultivation", func(branchID string) error {
			_, err := fixture.store.ApplyCultivationInterval(fixture.story.ID, interactive.CultivationRequest{
				BranchID: branchID, ActorID: interactive.DefaultActorID,
				Interval:  interactive.CultivationInterval{Value: 1, Unit: "day"},
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"recovery", func(branchID string) error {
			_, err := fixture.store.ApplySafeRecovery(fixture.story.ID, interactive.SafeRecoveryRequest{
				BranchID: branchID, ActorID: interactive.DefaultActorID,
				Mode: "health_rest", DurationSeconds: 3600,
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"battle", func(branchID string) error {
			_, err := fixture.store.AppendCombatRecord(fixture.story.ID, interactive.AppendCombatRecordRequest{
				BranchID: branchID, WorldTime: dutyWorldTime,
				Session:     battleAfterDeath(),
				CommandID:   "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"character facts", func(branchID string) error {
			_, err := fixture.store.AppendCharacterFacts(fixture.story.ID, interactive.AppendCharacterFactsRequest{
				BranchID: branchID, WorldTime: dutyWorldTime,
				Profiles: []character.Profile{{ID: "mu_peiling", Name: "慕沛灵", Role: "药园管事"}},
				CommandID: "cmd-after-death", CommandHash: "h-after-death",
			})
			return err
		}},
		{"character message", func(branchID string) error {
			return fixture.store.AppendCharacterMessages(fixture.story.ID, branchID, []character.Message{{
				CharacterID: "mu_peiling", Text: "东畦的账还没对完。",
				Basis: "差事未结", WorldTime: dutyWorldTime,
			}})
		}},
	}

	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			live := command.run(forkID)
			if errors.Is(live, interactive.ErrBranchTerminal) {
				t.Fatalf("a branch that never ended reported the terminal refusal: %v", live)
			}
			if live != nil {
				// The control is allowed to fail on its own merits; what matters is
				// that the refusal below is caused by the ending and not by this.
				t.Logf("control on the live branch: %v", live)
			}

			ended := command.run("main")
			if !errors.Is(ended, interactive.ErrBranchTerminal) {
				t.Fatalf("a finished branch accepted a %s command: err=%v", command.name, ended)
			}
		})
	}
}

// TestACommandCommittedBeforeTheEndingStillReplays is END-04's ordering
// requirement, and the reason the terminal gate sits after the idempotency
// replay rather than before it.
//
// The task was accepted while the character was alive. A client that retries the
// same command id after the ending is asking for the receipt it already has, not
// for a new act, so it must read the committed result instead of being told the
// branch is closed. Refusing here would turn a lost response into a lost result.
func TestACommandCommittedBeforeTheEndingStillReplays(t *testing.T) {
	fixture := startSectDutyStory(t)
	fixture.takeUpTheDuty(t)
	acceptSuQianMaterials(t, fixture)

	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我不闪不避，硬接他这一掌。",
		Narrative: "掌风落在胸口，眼前一黑。",
		TerminalOutcome: &interactive.TerminalOutcome{
			Terminal: true, Type: "death", Reason: "生命归零",
		},
	}); err != nil {
		t.Fatalf("commit the ending: %v", err)
	}

	replayed, err := fixture.store.ApplyQuest(fixture.story.ID, interactive.QuestRequest{
		BranchID: "main", TaskID: quest.TaskSuQianMaterials, Action: interactive.QuestActionAccept,
		CommandID: "cmd-accept", CommandHash: "h-accept",
	})
	if err != nil {
		t.Fatalf("a retry of the task accepted before the ending was refused: %v", err)
	}
	if !replayed.Replayed {
		t.Fatal("the retry replayed nothing; the first attempt's receipt was lost")
	}
}

// TestAFinishedBranchStaysReadableAndForkable is END-02's first half and END-03:
// the branch ends, the story does not. The turns are still there to read, and a
// fork cut before the ending takes the story on without overwriting it.
func TestAFinishedBranchStaysReadableAndForkable(t *testing.T) {
	fixture, ending := aFinishedBranch(t)

	read, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("a finished branch could not be read: %v", err)
	}
	if len(read.Snapshot.Turns) == 0 {
		t.Fatal("the finished branch lost its history")
	}
	var sawEnding bool
	for _, node := range read.Snapshot.Graph.Nodes {
		if node.ID == ending.ID && node.Terminal && node.TerminalType == "death" {
			sawEnding = true
		}
	}
	if !sawEnding {
		t.Fatal("the ending is not projected as a terminal node of its type")
	}

	branch, err := fixture.store.CreateBranch(fixture.story.ID, interactive.CreateBranchRequest{
		ParentEventID: read.Snapshot.Turns[len(read.Snapshot.Turns)-2].ID, Title: "不硬接的那一支",
	})
	if err != nil {
		t.Fatalf("fork before the ending: %v", err)
	}
	if _, _, err := fixture.store.AppendTurnWithState(fixture.story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: branch.ID, User: "我侧身让开。", Narrative: "你侧身让开，掌风擦着肩头过去。",
		TurnResult: &interactive.TurnResult{Choices: []string{"再退一步", "出声喝止", "转身就走"}},
	}); err != nil {
		t.Fatalf("the fork could not continue: %v", err)
	}

	// END-03: the old ending is not overwritten by the fork going on.
	after, err := fixture.store.StoryContext(fixture.story.ID, "main")
	if err != nil {
		t.Fatalf("read the finished branch after the fork moved on: %v", err)
	}
	if len(after.Snapshot.Turns) != len(read.Snapshot.Turns) {
		t.Fatalf("the fork rewrote the finished branch: %d turns, want %d",
			len(after.Snapshot.Turns), len(read.Snapshot.Turns))
	}
	if !strings.Contains(after.Snapshot.Turns[len(after.Snapshot.Turns)-1].Narrative, "再没有醒来") {
		t.Fatal("the finished branch no longer ends where it ended")
	}
}

// battleAfterDeath is a minimal battle record: the point of the request is the
// branch it lands on, not the exchange.
func battleAfterDeath() cultivation.CombatSession {
	return cultivation.CombatSession{ID: "bt-after-death", Round: 1}
}
