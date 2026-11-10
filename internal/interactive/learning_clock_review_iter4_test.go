package interactive

import (
	"testing"

	"ta/internal/quest"
)

func iter4LearningDeadlineBranch(state map[string]any, seconds int64) *storyBranchProjection {
	clock := NewStoryClock("")
	return &storyBranchProjection{State: state, Clock: &clock, Quests: map[string]quest.Instance{"deadline": {TaskID: quest.TaskSuQianMaterials, TaskVersion: "1.0", Status: quest.StatusAccepted, DeadlineSeconds: seconds}}}
}

func TestIter4LearningDeadlineDoesNotAllocateTimeToARefusedEarlierRow(t *testing.T) {
	store, id, _ := talismanStory(t)
	before, err := store.Snapshot(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	branch := iter4LearningDeadlineBranch(before.State, 600)
	system := combatTestSystemFromStore(t, store, id)
	t.Run("skill", func(t *testing.T) {
		out, err := planTurnSkillCommit(branch, StoryMeta{}, system, before.State, &TurnSkillLearningInput{Skills: []TurnSkillLearning{
			{DefID: "fire_bolt", Minutes: 120},
			{DefID: "fire_bolt", Source: "随后实际取得的完整火术注本", Minutes: 60},
		}}, "skill-row-boundary", "按可用注本研习到约定时刻")
		if err != nil {
			t.Fatal(err)
		}
		after := appliedXiuxianState(t, before.State, out.ops)
		records, _ := xiuxianEquipmentEntry(after, DefaultActorID, xiuxianFieldLearnedSkills)
		_, record := skillLearningRecord(records, "fire_bolt")
		if out.clock == nil || out.clock.DurationSeconds != 600 || skillRecordStudyMinutes(record) != 10 {
			t.Fatalf("refused first row consumed the valid later row's boundary allocation: clock=%+v record=%#v result=%+v", out.clock, record, out.result)
		}
	})
	t.Run("technique", func(t *testing.T) {
		out, err := planTurnTechniqueCommit(branch, StoryMeta{}, system, before.State, &TurnTechniqueInput{Learn: []TurnTechniqueLearning{
			{DefID: "method.warm_origin", Minutes: 120},
			{DefID: "method.warm_origin", Source: "随后实际取得的完整温元注本", Minutes: 60},
		}}, "method-row-boundary", "按可用注本研习到约定时刻")
		if err != nil {
			t.Fatal(err)
		}
		after := appliedXiuxianState(t, before.State, out.ops)
		records, _ := readTechniqueRecords(after, DefaultActorID)
		minutes := 0.0
		for _, record := range records {
			if record.DefID == "method.warm_origin" {
				minutes = record.Minutes
			}
		}
		if out.clock == nil || out.clock.DurationSeconds != 600 || minutes != 10 {
			t.Fatalf("refused first row consumed the valid later row's boundary allocation: clock=%+v minutes=%v result=%+v", out.clock, minutes, out.result)
		}
	})
}
