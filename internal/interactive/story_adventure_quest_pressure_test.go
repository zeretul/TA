package interactive

import (
	"fmt"
	"reflect"
	"testing"

	"ta/internal/quest"
)

// Thirty committed turns exercise persistence and delayed consumers, not model
// quality or thirty human play sessions. Each branch starts from the same fact.
func TestAdventureQuestThirtyTurnStatePressure(t *testing.T) {
	for _, pre := range []string{"lanchuan", "xinghai"} {
		t.Run(pre, func(t *testing.T) {
			pack, place, skill, chosen := "lanchuan-qunxia", "潮信渡", "lanchuan-s35", "c3"
			if pre == "xinghai" {
				pack, place, skill, chosen = "xinghai-689", "朱伯利外环47号减压长廊", "xinghai-s01", "c1"
			}
			s, id, opening, dir := authoredQuestStory(t, pack, place, 1000, map[string]int{skill: 1})
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "尚未处理"})
			if err != nil {
				t.Fatal(err)
			}
			task := pre + "-a01"
			questApplyOn(t, s, id, "main", task, QuestActionAccept, "", "accept")
			questApplyOn(t, s, id, fork.ID, task, QuestActionAccept, "", "accept-fork")
			first := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+"."+chosen, "resolve")
			before, _ := s.Snapshot(id, "main")
			for turn := 0; turn < 30; turn++ {
				if turn < 10 {
					_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "查看账册，不推进时间", Narrative: "账册仍是原来的记录。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
					if err != nil {
						t.Fatal(err)
					}
				} else {
					questWait(t, s, id, "main", 1800)
				}
				state, err := s.QuestStateFor(id, "main")
				if err != nil {
					t.Fatal(err)
				}
				if questAvailable(state, task) {
					t.Fatal("completed task resurfaced as a new promise")
				}
				if turn < 10 {
					snap, _ := s.Snapshot(id, "main")
					if !reflect.DeepEqual(snap.Clock, before.Clock) {
						t.Fatal("read-like turns advanced the formal clock")
					}
				}
				if turn == 14 {
					if err = s.Close(); err != nil {
						t.Fatal(err)
					}
					s = NewStore(dir)
					defer s.Close()
				}
			}
			questTestFollowUp(t, s, id, "main", task, task+"."+chosen)
			state, _ := s.QuestStateFor(id, "main")
			echo := pre + "-a01-echo-1"
			if pre == "lanchuan" {
				echo = pre + "-a01-echo-3"
			}
			if !questAvailable(state, echo) {
				t.Fatal("thirty-turn readback lost the delayed opportunity")
			}
			unresolved, _ := s.QuestStateFor(id, fork.ID)
			if questTestTask(t, unresolved, task).Status != string(quest.StatusAccepted) || questAvailable(unresolved, echo) {
				t.Fatal("pending promise inherited another branch's resolution")
			}
			after, _ := s.Snapshot(id, "main")
			replay := questApplyOn(t, s, id, "main", task, QuestActionChoose, task+"."+chosen, "resolve")
			again, _ := s.Snapshot(id, "main")
			if !replay.Replayed || replay.EventID != first.EventID || !reflect.DeepEqual(after.State, again.State) || !reflect.DeepEqual(after.Clock, again.Clock) {
				t.Fatal("old receipt paid or aged again")
			}
			t.Logf("%s: 30 committed turns, 10 without clock movement, 20 half-hours, cold reopen at turn 15, pending fork and replay isolated", pre)
		})
	}
}

func TestAdventureQuestFailedPreparationCanRecover(t *testing.T) {
	r := authoredQuestRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Currency = 500
	state := map[string]any{}
	projectActor := func() {
		for _, op := range adventureActorOps(a, "") {
			setPath(state, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
		}
	}
	projectActor()
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), "老锚机械酒吧与二手船材集市")
	d, _ := quest.DefinitionByID("xinghai-a02")
	p := &storyBranchProjection{State: state, Quests: map[string]quest.Instance{d.ID: {TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}}}
	system := StoryDirectorActorStateSystem{AdventureRules: &r}
	req := QuestRequest{TaskID: d.ID, Action: QuestActionChoose, ChoiceID: d.ID + ".c1"}
	failed, err := planQuestAction(p, system, req)
	if err != nil {
		t.Fatal(err)
	}
	p.Quests[d.ID] = failed.Instance
	p.Clock = failed.ClockAfter
	if _, err = planQuestAction(p, system, req); err == nil {
		t.Fatal("failure rerolled before consequence")
	}
	result := failed.Instance.Choices[d.ID+".resolve"]
	result.EchoResolved = true
	failed.Instance.Choices[d.ID+".resolve"] = result
	p.Quests[d.ID] = failed.Instance
	if _, err = planQuestAction(p, system, req); err == nil {
		t.Fatal("failure rerolled without improved preparation")
	}
	a.Skills["xinghai-s30"] = 1
	projectActor()
	recovered, err := planQuestAction(p, system, req)
	if err != nil {
		t.Fatal(err)
	}
	result = recovered.Instance.Choices[d.ID+".resolve"]
	if result.Outcome != "success" || result.Previous == nil || result.Previous.Outcome != "failure" || result.AtSeconds <= result.Previous.AtSeconds {
		t.Fatal("recovery erased prior consequences")
	}
	if !questHasSuccess(recovered.Instance) || questHasSuccess(quest.Instance{Choices: map[string]quest.ChoiceResult{"one": {Outcome: "success"}, "two": {Outcome: "failure"}}}) {
		t.Fatal("partial success counted as a completed pursuit")
	}
}

func TestAdventureQuestLoanInterestRepaymentAndCollection(t *testing.T) {
	s, id, _, dir := authoredQuestStory(t, "xinghai-689", "无限财团全宇宙物流金融中继站", 1000, nil)
	loan := "xinghai-source-i56"
	questApplyOn(t, s, id, "main", loan, QuestActionAccept, "", "accept-loan")
	questApplyOn(t, s, id, "main", loan, QuestActionChoose, loan+".work", "borrow")
	snap, _ := s.Snapshot(id, "main")
	a, _ := AdventureActorFromState(snap.State, DefaultActorID)
	if a.Currency != 11000 || a.Inventory["xinghai-i56"] != 1 {
		t.Fatal("loan did not write actual balance and debt paper")
	}
	questWait(t, s, id, "main", 7200)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(dir)
	defer s.Close()
	repayment := "xinghai-loan-repayment"
	questApplyOn(t, s, id, "main", repayment, QuestActionAccept, "", "accept-repay")
	questApplyOn(t, s, id, "main", repayment, QuestActionChoose, repayment+".pay", "repay")
	snap, _ = s.Snapshot(id, "main")
	a, _ = AdventureActorFromState(snap.State, DefaultActorID)
	// 10000 -> 10001 -> 10003, each completed hour rounds up as disclosed.
	if a.Currency != 997 {
		t.Fatalf("formal-clock compounded repayment: got %d", a.Currency)
	}
	questWait(t, s, id, "main", 90*86400)
	view, _ := s.QuestStateFor(id, "main")
	if questAvailable(view, "xinghai-loan-collection") {
		t.Fatal("paid debt created a collection promise")
	}
	before, _ := s.Snapshot(id, "main")
	again := questApplyOn(t, s, id, "main", repayment, QuestActionChoose, repayment+".pay", "repay")
	after, _ := s.Snapshot(id, "main")
	if !again.Replayed || !reflect.DeepEqual(before.State, after.State) {
		t.Fatal("repayment charged twice")
	}
	t.Log(fmt.Sprintf("loan 10000, two hourly interest accruals 3, payment 10003, balance %d; collection suppressed after 90 days", a.Currency))
}

func TestAdventureQuestSecondAffairChoicesUnlockDifferentWork(t *testing.T) {
	for _, pre := range []string{"lanchuan", "xinghai"} {
		t.Run(pre, func(t *testing.T) {
			pack, place, skills := "lanchuan-qunxia", "白鹭渔村", map[string]int{"lanchuan-s35": 1, "lanchuan-s34": 1}
			if pre == "xinghai" {
				pack, place, skills = "xinghai-689", "老锚机械酒吧与二手船材集市", map[string]int{"xinghai-s30": 1, "xinghai-s13": 1}
			}
			s, id, opening, _ := authoredQuestStory(t, pack, place, 1000, skills)
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: opening, Title: "另一种处理"})
			if err != nil {
				t.Fatal(err)
			}
			for index, branch := range []string{"main", fork.ID} {
				task := pre + "-a02"
				choice := fmt.Sprintf("%s.c%d", task, index+1)
				questApplyOn(t, s, id, branch, task, QuestActionAccept, "", "accept-"+branch)
				questApplyOn(t, s, id, branch, task, QuestActionChoose, choice, "choose-"+branch)
				state, _ := s.QuestStateFor(id, branch)
				if questAvailable(state, pre+"-a02-echo-1") || questAvailable(state, pre+"-a02-echo-2") {
					t.Fatal("later work leaked before its authored delay")
				}
				definition, _ := quest.DefinitionByID(task)
				method, _ := definition.Choice(choice)
				questWait(t, s, id, branch, method.EchoAfterSeconds)
				questTestFollowUp(t, s, id, branch, task, choice)
				state, _ = s.QuestStateFor(id, branch)
				expected := fmt.Sprintf("%s-a02-echo-%d", pre, index+1)
				other := fmt.Sprintf("%s-a02-echo-%d", pre, 2-index)
				if !questAvailable(state, expected) || questAvailable(state, other) {
					t.Fatal("two choices produced the same later opportunity")
				}
				questApplyOn(t, s, id, branch, expected, QuestActionAccept, "", "accept-echo-"+branch)
				result := questApplyOn(t, s, id, branch, expected, QuestActionChoose, expected+".work", "do-echo-"+branch)
				if result.Instance.Status != quest.StatusCompleted {
					t.Fatal("delayed opportunity was only display text")
				}
			}
		})
	}
}

func TestAdventurePursuitFinalCannotUseFailedPreparation(t *testing.T) {
	for _, pre := range []string{"lanchuan", "xinghai"} {
		pack := "lanchuan-qunxia"
		if pre == "xinghai" {
			pack = "xinghai-689"
		}
		r := authoredQuestRules(t, pack)
		for index := 1; index <= 3; index++ {
			t.Run(fmt.Sprintf("%s-q%02d", pre, index), func(t *testing.T) {
				d, _ := quest.DefinitionByID(fmt.Sprintf("%s-q%02d", pre, index))
				final := d.Choices[len(d.Choices)-1]
				a := NewAdventureActor(r)
				a.Rank = 10
				a.Currency = 1000000
				for _, sk := range r.Skills {
					a.Skills[sk.ID] = 4
				}
				for _, item := range r.Items {
					a.Inventory[item.ID] = 1
				}
				state := map[string]any{}
				for _, op := range adventureActorOps(a, "") {
					setPath(state, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
				}
				setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), journeyPlaceLabel(final.LocationIDs[0]))
				prior := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted, CompletedObjectiveIDs: append([]string(nil), final.AfterObjectives...), Choices: map[string]quest.ChoiceResult{}}
				for _, objective := range final.AfterObjectives {
					for _, c := range d.Choices {
						if c.ObjectiveID == objective {
							prior.Choices[objective] = quest.ChoiceResult{ChoiceID: c.ID, Outcome: "failure", EchoResolved: true}
							break
						}
					}
				}
				p := &storyBranchProjection{State: state, Quests: map[string]quest.Instance{d.ID: prior}}
				for _, task := range final.RequiredTasks {
					p.Quests[task] = quest.Instance{Status: quest.StatusCompleted}
				}
				if _, err := planQuestAction(p, StoryDirectorActorStateSystem{AdventureRules: &r}, QuestRequest{TaskID: d.ID, Action: QuestActionChoose, ChoiceID: final.ID}); err == nil {
					t.Fatal("final ending accepted a handled but failed earlier stage")
				}
			})
		}
	}
}
