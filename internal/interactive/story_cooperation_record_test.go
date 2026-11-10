package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"ta/internal/agents/conversationjournal"
	"ta/internal/character"
)

func TestCooperationRecordSchemaRecognizesCanonicalSideEvent(t *testing.T) {
	raw := map[string]any{
		"v": schemaVersion, "type": "cooperation_record", "id": "coop-event-1",
		"parent_id": "turn-1", "branch_id": "main", "ts": "2026-11-03T02:45:00Z", "turn_id": "turn-1", "cursor": "coop-event-1",
		"matter": map[string]any{
			"id": "mat-coop-1", "story_id": "story-1", "branch_id": "main", "character_id": "steward",
			"kind": "promise", "status": "open", "statement": "Review the observations together.",
			"world_time": "day 1", "turn_id": "turn-1", "source_event_id": "coop-event-1",
			"cooperation": map[string]any{
				"project_id": "coop.herbcave.notes", "project_version": "1.0", "realm_instance_id": "instance-1",
				"consent_exchange_id": "exchange-1", "shared_discovery_source_ref": "clue:survey-1", "phase": "agreed",
			},
		},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeStoryEventRecord(data); err != nil {
		t.Fatalf("canonical cooperation event cannot be recovered: %v", err)
	}
}

func cooperationRecordFixture(phase character.CooperationPhase) CooperationRecordEvent {
	id := "coop-" + string(phase)
	terms := &character.CooperationTerms{
		ProjectID: "coop.herbcave.notes", ProjectVersion: "1.0", RealmInstanceID: "instance-1",
		ConsentExchangeID: "exchange-1", SharedDiscoverySourceRef: "clue:survey-1", Phase: phase,
	}
	if phase != character.CooperationAgreed {
		terms.WorkSourceRef = "coop-worked"
	}
	if phase == character.CooperationUsed || phase == character.CooperationReported {
		terms.UsedBy = "realm-use-1"
	}
	if phase == character.CooperationReported {
		terms.ReportSourceRefs = []string{"core:realm-result-1"}
	}
	event := CooperationRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCooperationRecord, ID: id, ParentID: "turn-1", BranchID: "main",
		Ts: "2026-11-03T02:45:00Z", TurnID: "turn-1", Cursor: id,
		Matter: &character.Matter{
			ID: "mat-coop-1", StoryID: "story-1", BranchID: "main", CharacterID: "steward",
			Kind: character.MatterPromise, Status: character.MatterOpen, Statement: "Review the observations together.",
			WorldTime: "day 1", TurnID: "turn-1", SourceEventID: id, Cooperation: terms,
		},
		Knowledge: []character.KnowledgeRecord{{
			ID: "knowledge-" + id, CharacterID: "steward", Statement: "The player described one observed clue.",
			Nature: character.NatureTold, Audience: character.AudienceSingle, LearnedFrom: "protagonist",
			SourceEventID: id, SourceTurnID: "turn-1", FormalCommitID: id, WorldTime: "day 1",
		}},
	}
	if phase == character.CooperationWorked {
		before := NewStoryClock("day 1")
		after, _ := AdvanceStoryClock(before, 600)
		event.ClockBefore, event.ClockAfter = &before, &after
	}
	return event
}

func TestCooperationRecordValidatesSourcesAndClock(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*CooperationRecordEvent)
	}{
		{"wrong-type", func(e *CooperationRecordEvent) { e.Type = StoryEventTypeCharacterRecord }},
		{"missing-cursor", func(e *CooperationRecordEvent) { e.Cursor = "" }},
		{"ordinary-matter", func(e *CooperationRecordEvent) { e.Matter.Cooperation = nil }},
		{"matter-branch", func(e *CooperationRecordEvent) { e.Matter.BranchID = "other" }},
		{"matter-event", func(e *CooperationRecordEvent) { e.Matter.SourceEventID = "other" }},
		{"matter-turn", func(e *CooperationRecordEvent) { e.Matter.TurnID = "other" }},
		{"work-source", func(e *CooperationRecordEvent) { e.Matter.Cooperation.WorkSourceRef = "other" }},
		{"knowledge-event", func(e *CooperationRecordEvent) { e.Knowledge[0].SourceEventID = "other" }},
		{"knowledge-turn", func(e *CooperationRecordEvent) { e.Knowledge[0].SourceTurnID = "other" }},
		{"knowledge-cursor", func(e *CooperationRecordEvent) { e.Knowledge[0].FormalCommitID = "other" }},
		{"knowledge-partner", func(e *CooperationRecordEvent) { e.Knowledge[0].CharacterID = "other" }},
		{"false-experience", func(e *CooperationRecordEvent) { e.Knowledge[0].Nature = character.NatureExperienced }},
		{"shared-with-world", func(e *CooperationRecordEvent) { e.Knowledge[0].Audience = character.AudienceWorld }},
		{"duplicate-knowledge", func(e *CooperationRecordEvent) { e.Knowledge = append(e.Knowledge, e.Knowledge[0]) }},
		{"missing-clock", func(e *CooperationRecordEvent) { e.ClockBefore, e.ClockAfter = nil, nil }},
		{"half-clock", func(e *CooperationRecordEvent) { e.ClockBefore = nil }},
		{"clock-duration", func(e *CooperationRecordEvent) { e.ClockAfter.Seconds = 1200 }},
		{"clock-anchor", func(e *CooperationRecordEvent) { e.ClockAfter.Anchor = "new-origin" }},
		{"clock-version", func(e *CooperationRecordEvent) { e.ClockAfter.Version = 999 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := cooperationRecordFixture(character.CooperationWorked)
			test.edit(&event)
			if err := validateCooperationRecordEvent(event); err == nil {
				t.Fatal("invalid cooperation event accepted")
			}
		})
	}
	for _, phase := range []character.CooperationPhase{character.CooperationAgreed, character.CooperationUsed, character.CooperationReported} {
		event := cooperationRecordFixture(phase)
		before := NewStoryClock("day 1")
		after, _ := AdvanceStoryClock(before, 600)
		event.ClockBefore, event.ClockAfter = &before, &after
		if err := validateCooperationRecordEvent(event); err == nil {
			t.Errorf("phase %s charged work time", phase)
		}
	}
}

func TestCooperationRecordRefusalSharesOnlyKnowledge(t *testing.T) {
	event := cooperationRecordFixture(character.CooperationAgreed)
	event.Matter = nil
	branch := &storyBranchProjection{State: map[string]any{"untouched-inventory": 7}}
	if err := applyCooperationRecord(branch, event, "story-1"); err != nil {
		t.Fatal(err)
	}
	if branch.Clock != nil || len(branch.Characters.Matters) != 0 || len(branch.Characters.Knowledge) != 1 || branch.State["untouched-inventory"] != 7 {
		t.Fatalf("refusal granted work, charged time or changed resources: %#v", branch)
	}
	before, _ := json.Marshal(branch)
	if err := applyCooperationRecord(branch, event, "story-1"); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(branch)
	if string(before) != string(after) {
		t.Fatal("replaying knowledge-only refusal changed the projection")
	}
	event.Knowledge = nil
	if err := validateCooperationRecordEvent(event); err == nil {
		t.Fatal("empty cooperation event accepted")
	}
}

func TestCooperationRecordAtomicFailureAndWaitCannotDoubleClock(t *testing.T) {
	work := cooperationRecordFixture(character.CooperationWorked)
	for _, name := range []string{"different-story", "stale-clock", "first-origin", "invalid-phase"} {
		t.Run(name, func(t *testing.T) {
			branch := &storyBranchProjection{State: map[string]any{"untouched": 5}}
			storyID := "story-1"
			event := cooperationRecordFixture(character.CooperationWorked)
			switch name {
			case "different-story":
				storyID = "other-story"
			case "stale-clock":
				branch.Clock = cloneStoryClock(event.ClockAfter)
			case "first-origin":
				event.ClockBefore.Seconds, event.ClockAfter.Seconds = 600, 1200
			case "invalid-phase":
				event = cooperationRecordFixture(character.CooperationReported)
			}
			before, _ := json.Marshal(branch)
			if err := applyCooperationRecord(branch, event, storyID); err == nil {
				t.Fatal("invalid event applied")
			}
			after, _ := json.Marshal(branch)
			if string(before) != string(after) {
				t.Fatal("failed event partially changed time or character facts")
			}
		})
	}
	wait := TurnEvent{User: "wait ten minutes", ClockAdvance: &TurnClockAdvance{
		Action: TurnWorldActionWait, SourceText: "wait ten minutes", DurationSeconds: 600,
		Before: *work.ClockBefore, After: *work.ClockAfter,
	}}
	branch := &storyBranchProjection{}
	if err := applyTurnClockAdvance(branch, wait); err != nil {
		t.Fatal(err)
	}
	if err := applyCooperationRecord(branch, work, "story-1"); err == nil || branch.clockSeconds() != 600 || branch.Characters != nil {
		t.Fatal("a separate wait paid cooperation time a second time or partly published work")
	}
	branch = &storyBranchProjection{}
	if err := applyCooperationRecord(branch, work, "story-1"); err != nil {
		t.Fatal(err)
	}
	if err := applyTurnClockAdvance(branch, wait); err == nil || branch.clockSeconds() != 600 {
		t.Fatal("wait reapplied an interval already owned by cooperation")
	}
}

func TestCooperationRecordPathGuardAndTimeOnlyReplay(t *testing.T) {
	projection := newStoryJournalProjection("story-1", "test-generation")
	branch := projection.branch("main")
	work := cooperationRecordFixture(character.CooperationWorked)
	branch.Head = work.ParentID
	encode := func(event CooperationRecordEvent) StoryEventRecord {
		t.Helper()
		data, _ := json.Marshal(event)
		record, err := decodeStoryEventRecord(data)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	record := encode(work)
	if err := projection.applyEvent(1, record); err != nil {
		t.Fatal(err)
	}
	before := cloneCharacterState(branch.Characters)
	if err := projection.applyEvent(2, record); err != nil {
		t.Fatal(err)
	}
	if branch.Head != work.ID || branch.clockSeconds() != 600 || !reflect.DeepEqual(branch.Characters, before) {
		t.Fatal("replaying the same side record changed canonical facts or charged time twice")
	}
	orphan := cooperationRecordFixture(character.CooperationUsed)
	orphan.ParentID = "a-replaced-turn"
	if err := projection.applyEvent(3, encode(orphan)); err != nil {
		t.Fatal(err)
	}
	if branch.Head != work.ID || !reflect.DeepEqual(branch.Characters, before) {
		t.Fatal("an orphaned old action consumed the live plan")
	}
	checkpoint, err := storyTimeFromRecords([]StoryEventRecord{record})
	if err != nil || checkpoint.Clock == nil || checkpoint.Clock.Seconds != 600 {
		t.Fatalf("time-only recovery tried to replay character phases or lost its interval: %#v %v", checkpoint.Clock, err)
	}
	// The clock is copied, not an alias to a caller-owned publication.
	work.ClockAfter.Seconds = 999
	if branch.clockSeconds() != 600 || checkpoint.Clock.Seconds != 600 {
		t.Fatal("editing the event changed its projected clock")
	}
}

func TestCooperationRecordCannotOverwriteEarlierKnowledge(t *testing.T) {
	first := cooperationRecordFixture(character.CooperationAgreed)
	first.Matter = nil
	branch := &storyBranchProjection{}
	if err := applyCooperationRecord(branch, first, "story-1"); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(branch)
	rewrite := cooperationRecordFixture(character.CooperationAgreed)
	rewrite.ID, rewrite.Cursor, rewrite.Matter = "rewrite-event", "rewrite-event", nil
	rewrite.Knowledge[0].SourceEventID, rewrite.Knowledge[0].FormalCommitID = rewrite.ID, rewrite.Cursor
	rewrite.Knowledge[0].Statement = "Changed facts under the earlier knowledge ID."
	if err := applyCooperationRecord(branch, rewrite, "story-1"); err == nil {
		t.Fatal("new event silently replaced or discarded a conflicting knowledge record")
	}
	after, _ := json.Marshal(branch)
	if string(before) != string(after) {
		t.Fatal("refused knowledge overwrite changed the projection")
	}
}

// This writes explicit rule fixtures through the real journal. It tests
// persistence, not NPC consent, acquisition or natural-model gameplay.
func TestCooperationRecordStoreRebuildForkAndHistory(t *testing.T) {
	store, storyID := characterTestStory(t)
	t.Cleanup(func() { _ = store.Close() })
	appendTurn := func(text string) TurnEvent {
		turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
			BranchID: "main", User: text, Narrative: text, TurnResult: &TurnResult{Choices: testTurnChoices()},
		})
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}
	beforeTurn := appendTurn("Before cooperation.")
	workTurn := appendTurn("Work together on the notes.")
	work := cooperationRecordFixture(character.CooperationWorked)
	bindCooperationRecordTestSource(&work, storyID, workTurn.ID)
	appendCooperationRecordTestEvent(t, store, storyID, &work)
	workState, err := store.CharacterState(storyID, "main")
	if err != nil || len(workState.Matters) != 1 || workState.Matters[0].Cooperation.Phase != character.CooperationWorked {
		t.Fatalf("canonical work not visible: %#v %v", workState, err)
	}
	usedTurn := appendTurn("Apply the plan at the original obstacle.")
	used := cooperationRecordFixture(character.CooperationUsed)
	bindCooperationRecordTestSource(&used, storyID, usedTurn.ID)
	used.Knowledge = nil
	appendCooperationRecordTestEvent(t, store, storyID, &used)
	reportTurn := appendTurn("Tell only the actual known outcome.")
	report := cooperationRecordFixture(character.CooperationReported)
	bindCooperationRecordTestSource(&report, storyID, reportTurn.ID)
	report.Knowledge[0].Statement = "The player reported preserving the core."
	appendCooperationRecordTestEvent(t, store, storyID, &report)
	final, err := store.CharacterState(storyID, "main")
	if err != nil || len(final.Matters) != 1 || len(final.Knowledge) != 2 || final.Matters[0].Cooperation.Phase != character.CooperationReported {
		t.Fatalf("use/report did not update canonical facts: %#v %v", final, err)
	}
	finalSnapshot, err := store.Snapshot(storyID, "main")
	if err != nil || finalSnapshot.TurnCount != 4 || SnapshotClockSeconds(finalSnapshot) != 600 {
		t.Fatalf("side events added turns or charged multiple intervals: %#v %v", finalSnapshot.Clock, err)
	}
	// Both are historical cuts now, not the live-head shortcut. Side records
	// belonging to workTurn must travel with it, but later use/report must not.
	beforeFork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: beforeTurn.ID, Title: "Before work"})
	if err != nil {
		t.Fatal(err)
	}
	workFork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: workTurn.ID, Title: "After work"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id      string
		count   int
		seconds int64
	}{{beforeFork.ID, 0, 0}, {workFork.ID, 1, 600}} {
		state, err := store.CharacterState(storyID, test.id)
		if err != nil || len(state.Matters) != test.count {
			t.Fatalf("fork %s inherited wrong matters: %#v %v", test.id, state, err)
		}
		snapshot, err := store.Snapshot(storyID, test.id)
		if err != nil || SnapshotClockSeconds(snapshot) != test.seconds {
			t.Fatalf("fork %s inherited wrong clock: %#v %v", test.id, snapshot.Clock, err)
		}
		if test.count != 0 && (!reflect.DeepEqual(state.Matters, workState.Matters) || state.Matters[0].BranchID != "main") {
			t.Fatal("historical cooperation was rebound to the new branch or learned future use/report")
		}
	}
	page, err := store.ReadHistoryPage(storyID, "main", "", 2)
	if err != nil || len(page.Turns) != 2 {
		t.Fatalf("bounded history failed after cooperation side events: %#v %v", page, err)
	}
	root, journalPath := store.Root(), store.storyPath(storyID)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(journalPath)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	cold, err := reopened.CharacterState(storyID, "main")
	if err != nil || !reflect.DeepEqual(cold, final) {
		t.Fatalf("cold canonical replay changed cooperation: %#v %v", cold, err)
	}
	coldSnapshot, err := reopened.Snapshot(storyID, "main")
	if err != nil || !reflect.DeepEqual(coldSnapshot.State, finalSnapshot.State) || SnapshotClockSeconds(coldSnapshot) != 600 {
		t.Fatalf("cold replay changed state or spent time twice: %#v %v", coldSnapshot.Clock, err)
	}
	coldFork, err := reopened.CharacterState(storyID, workFork.ID)
	if err != nil || !reflect.DeepEqual(coldFork.Matters, workState.Matters) {
		t.Fatalf("cold replay changed the historical fork: %#v %v", coldFork, err)
	}
}

func bindCooperationRecordTestSource(event *CooperationRecordEvent, storyID, turnID string) {
	event.TurnID = turnID
	if event.Matter != nil {
		event.Matter.StoryID, event.Matter.TurnID = storyID, turnID
	}
	for i := range event.Knowledge {
		event.Knowledge[i].SourceTurnID = turnID
	}
}

func appendCooperationRecordTestEvent(t *testing.T, store *Store, storyID string, event *CooperationRecordEvent) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	release, err := store.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	handle, err := store.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		t.Fatal(err)
	}
	meta := handle.projection.Meta
	branch := meta.Branches[event.BranchID]
	event.ParentID = branch.Head
	branch.Head = event.ID
	meta.Branches[event.BranchID] = branch
	meta.UpdatedAt = event.Ts
	if err := store.appendStoryTransactionLocked(storyID, meta, *event); err != nil {
		t.Fatal(err)
	}
}
