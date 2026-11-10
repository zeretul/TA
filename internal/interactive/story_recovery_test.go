package interactive

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/storytime"
)

// These tests cover §三.7's acceptance list for the safe rest and meditation
// loop: the rates, the cap, segmentation, idempotency, the refusals, the
// transaction, restart, and branch isolation.
//
// The pure rules already have their own suite in internal/cultivation and
// internal/storytime. What those cannot reach is everything the journal layer
// decides — that the recovery and the pool write land in one transaction, that a
// retry adds nothing, that a branch which forked before a rest does not inherit
// it, and that a failed append leaves no half state. Those are asserted here and
// not restated.

// recoveryTestStory builds a story on the cultivation rules whose protagonist
// has a real recorded wound, so a rest has something to restore.
//
// The wound arrives through the turn's own state updates, which is the path a
// narrative really uses: the current pools are deliberately writable, so a
// recorded injury is a story fact rather than something only the rules can
// author.
func recoveryTestStory(t *testing.T, updates ...interactivestate.Update) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "休养记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山洞中停下。",
		TurnResult: &TurnResult{StateUpdates: updates, Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// recoveryTestPool writes one pool through the same update path the model uses.
func recoveryTestPool(fieldID string, value float64) interactivestate.Update {
	return interactivestate.Update{
		Op:    interactivestate.Replace,
		Path:  interactivestate.FormatPath([]string{DefaultActorID, fieldID}),
		Value: value,
	}
}

// recoveryTestReplace writes any other field the same way, so a fixture can seed
// the object-form lasting effect that caps a pool.
func recoveryTestReplace(fieldID string, value any) interactivestate.Update {
	return interactivestate.Update{
		Op:    interactivestate.Replace,
		Path:  interactivestate.FormatPath([]string{DefaultActorID, fieldID}),
		Value: value,
	}
}

// recoveryTestApply is the ordinary request: a fresh activity, no node.
func recoveryTestApply(t *testing.T, store *Store, storyID string, mode cultivation.RecoveryMode, seconds int64, commandID string) SafeRecoveryResult {
	t.Helper()
	result, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: mode,
		DurationSeconds: seconds, CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func recoveryTestRecordCount(t *testing.T, store *Store, storyID string) int {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range records {
		if record.Envelope.Type == StoryEventTypeRecoveryRecord {
			count++
		}
	}
	return count
}

// recoveryTestStoredPool reads one pool out of committed state.
func recoveryTestStoredPool(t *testing.T, store *Store, storyID, fieldID string) int {
	t.Helper()
	state := combatTestState(t, store, storyID)
	value, ok := actorStateNumber(actorStateFieldValue(state, DefaultActorID, fieldID))
	if !ok {
		t.Fatalf("the committed state must carry %s: %#v", fieldID, state)
	}
	return int(value)
}

// TestSafeRecoveryRestoresAFifthOfHealthAcrossOneCalculationDay is §三.3's rest
// rate: 5% of the health maximum per 24-hour calculation day.
//
// The character is at qi-refining layer 6, whose table fixes health at 200, so a
// full day of rest is 10 health — and the number has to be the settled one in
// the save, not only in the answer.
func TestSafeRecoveryRestoresAFifthOfHealthAcrossOneCalculationDay(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-day")

	if result.Candidate.PoolMaximum != 200 {
		t.Fatalf("the pool maximum must be the stage table's, got %d", result.Candidate.PoolMaximum)
	}
	if result.Candidate.CandidatePool != 47 {
		t.Fatalf("a day of rest must add a twentieth of the maximum: got %d, want 47", result.Candidate.CandidatePool)
	}
	if result.Elapsed != 24*60*60 {
		t.Fatalf("the interval settled must be the interval asked for, got %d", result.Elapsed)
	}
	if !result.Completed {
		t.Fatalf("a rest with no boundary must complete: %#v", result)
	}
	if result.ElapsedDescription != "1日" {
		t.Fatalf("the wording must be natural rather than a calendar stamp, got %q", result.ElapsedDescription)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("the restored pool must reach the save, got %d", got)
	}
	// The clock is the story's one formal axis, and the rest moved it by exactly
	// the interval it settled. §三.1 forbids a recovery that owns a time of its own.
	if result.ClockAfter.Seconds != 24*60*60 {
		t.Fatalf("the branch clock must advance by the settled interval, got %d", result.ClockAfter.Seconds)
	}
}

// TestSafeMeditationRestoresATenthOfManaAcrossOneCalculationHour is §三.3's
// meditation rate: 10% of the mana maximum per calculation hour.
func TestSafeMeditationRestoresATenthOfManaAcrossOneCalculationHour(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldMana, 0))

	result := recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 60*60, "cmd-hour")

	if result.Candidate.PoolMaximum != 160 {
		t.Fatalf("the mana maximum must be the stage table's, got %d", result.Candidate.PoolMaximum)
	}
	if result.Candidate.CandidatePool != 16 {
		t.Fatalf("an hour of meditation must add a tenth of the maximum: got %d, want 16", result.Candidate.CandidatePool)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldMana); got != 16 {
		t.Fatalf("the restored mana must reach the save, got %d", got)
	}
	// One pool per activity: §三.3 forbids restoring both at once for free, so a
	// meditation must leave health exactly where the turn left it.
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 200 {
		t.Fatalf("meditation must not also heal, got health %d", got)
	}
}

// TestSafeRecoveryNeverExceedsThePoolMaximum covers §三.7's "满池不越界": rest
// enough to overshoot and the pool stops at its maximum rather than running past
// it.
func TestSafeRecoveryNeverExceedsThePoolMaximum(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 195))

	// A full month of rest is far more than the five points still missing.
	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 30*24*60*60, "cmd-month")

	if result.Candidate.CandidatePool != 200 {
		t.Fatalf("the pool must stop at its maximum, got %d", result.Candidate.CandidatePool)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 200 {
		t.Fatalf("the saved pool must stop at its maximum, got %d", got)
	}
	// A pool that was already full stays full rather than being pushed over.
	full, fullStory := recoveryTestStory(t)
	fullResult := recoveryTestApply(t, full, fullStory, cultivation.RecoveryHealthRest, 24*60*60, "cmd-full")
	if fullResult.Candidate.CandidatePool != 200 {
		t.Fatalf("a full pool must stay at its maximum, got %d", fullResult.Candidate.CandidatePool)
	}
}

// TestSegmentedRestAndOneLongRestReachTheSamePool is §三.7's "一长段/多短段相同",
// and it is also §三.5's "部分完成再继续只算剩余".
//
// The rest is cut short by a real decision node halfway through, another activity
// spends an hour in between, and the rest is then resumed. Two properties have to
// hold at once: the cumulative effective seconds decide the pool, so the two
// halves reach the same number one long rest would; and the hour spent meditating
// in between is not rest, so it can never be spliced onto the eligible stretch.
func TestSegmentedRestAndOneLongRestReachTheSamePool(t *testing.T) {
	const halfDay = int64(12 * 60 * 60)

	segmented, segmentedStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37), recoveryTestPool(xiuxianFieldMana, 0))
	// A decision the player has to answer at the halfway point.
	first, err := segmented.ApplySafeRecovery(segmentedStory, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60,
		Nodes: []storytime.Node{{
			ID: "decision-1", At: storytime.Instant(halfDay),
			Kind: storytime.DecisionNode, RequiresResponse: true,
		}},
		CommandID: "cmd-seg-1", CommandHash: "cmd-seg-1-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Completed {
		t.Fatal("an unresolved decision node must stop the interval rather than run past it")
	}
	if len(first.PendingNodeIDs) != 1 || first.PendingNodeIDs[0] != "decision-1" {
		t.Fatalf("the boundary that stopped the rest must be reported: %#v", first.PendingNodeIDs)
	}
	if first.Elapsed != halfDay {
		t.Fatalf("only the settled half is time spent, got %d", first.Elapsed)
	}
	if first.Candidate.CandidatePool != 42 {
		t.Fatalf("half a day must add half of the day's recovery: got %d, want 42", first.Candidate.CandidatePool)
	}

	// The hour between the pause and the resumption is spent meditating, not
	// resting. It moves the clock, and it must not move the rest's progress.
	recoveryTestApply(t, segmented, segmentedStory, cultivation.RecoverySafeMeditation, 60*60, "cmd-gap")

	resumed, err := segmented.ApplySafeRecovery(segmentedStory, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		ActivityID: first.ActivityID,
		Nodes: []storytime.Node{{
			ID: "decision-1", At: storytime.Instant(halfDay),
			Kind: storytime.DecisionNode, RequiresResponse: true, Resolved: true,
		}},
		CommandID: "cmd-seg-2", CommandHash: "cmd-seg-2-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Completed {
		t.Fatalf("a resumed rest must finish its remainder: %#v", resumed)
	}
	if resumed.Elapsed != halfDay {
		t.Fatalf("a continuation settles only the remainder, got %d", resumed.Elapsed)
	}

	// The whole day of rest plus the hour of meditation is the time really spent.
	if resumed.ClockAfter.Seconds != 24*60*60+60*60 {
		t.Fatalf("the clock must count both activities and nothing else, got %d", resumed.ClockAfter.Seconds)
	}

	whole, wholeStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37), recoveryTestPool(xiuxianFieldMana, 0))
	oneLong := recoveryTestApply(t, whole, wholeStory, cultivation.RecoveryHealthRest, 24*60*60, "cmd-whole")

	if resumed.Candidate.CandidatePool != oneLong.Candidate.CandidatePool {
		t.Fatalf("segmented rest reached %d and one long rest reached %d; they must agree",
			resumed.Candidate.CandidatePool, oneLong.Candidate.CandidatePool)
	}
	if got := recoveryTestStoredPool(t, segmented, segmentedStory, xiuxianFieldHealth); got != oneLong.Candidate.CandidatePool {
		t.Fatalf("the segmented branch saved %d, want %d", got, oneLong.Candidate.CandidatePool)
	}
}

// TestRepeatedRecoveryCommandAddsNothing is §三.7's "同请求重复0新增" and §三.5's
// "同一活动同目标重试增量0".
//
// A lost response, a repeated click and a recovery re-read all retry the same
// command. The answer has to be the interval already committed, and the journal
// must not grow.
func TestRepeatedRecoveryCommandAddsNothing(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	first := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-once")
	before := recoveryTestRecordCount(t, store, storyID)

	second, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-once", CommandHash: "cmd-once-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed {
		t.Fatal("a retry of the same command must be reported as a replay, not as a second rest")
	}
	if second.EventID != first.EventID {
		t.Fatalf("the retry must return the committed interval: got %s, want %s", second.EventID, first.EventID)
	}
	if after := recoveryTestRecordCount(t, store, storyID); after != before {
		t.Fatalf("a retry must add nothing: records went %d -> %d", before, after)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("a retry must not recover a second time, got health %d", got)
	}

	// The same identity for different content is a conflict rather than a silent
	// overwrite: accepting it would either lose the first interval or grant the
	// second one under a false identity.
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 12 * 60 * 60, CommandID: "cmd-once", CommandHash: "different-hash",
	}); !errors.Is(err, ErrRecoveryCommandConflict) {
		t.Fatalf("a reused command with different content must conflict, got %v", err)
	}
	if after := recoveryTestRecordCount(t, store, storyID); after != before {
		t.Fatalf("a conflicted command must not reach the journal: %d -> %d", before, after)
	}
}

// TestRecoveryRefusesInBattleAndDangerousConditions is §三.4: ordinary safe
// recovery is refused while a battle is live, while the caller declares danger,
// and for an injury that needs special treatment — and eligibility comes from
// formal state rather than from anything the caller claims.
func TestRecoveryRefusesInBattleAndDangerousConditions(t *testing.T) {
	// A live battle on this branch, committed as a real exchange.
	fighting, fightingStory := combatTestStory(t)
	combatTestExchange(t, fighting, fightingStory, "cmd-fight", "cmd-fight-hash")
	if _, err := fighting.ApplySafeRecovery(fightingStory, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-rest-in-fight", CommandHash: "h",
	}); !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("a live battle must refuse ordinary rest, got %v", err)
	}
	if count := recoveryTestRecordCount(t, fighting, fightingStory); count != 0 {
		t.Fatalf("a refused rest must not reach the journal, got %d records", count)
	}

	// A declared condition may only make the answer stricter, never weaker.
	dangerous, dangerousStory := recoveryTestStory(t)
	if _, err := dangerous.ApplySafeRecovery(dangerousStory, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60,
		DeclaredEligibility: cultivation.RecoveryInDanger,
		CommandID: "cmd-danger", CommandHash: "h",
	}); !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("a declared danger must refuse ordinary rest, got %v", err)
	}

	// Zero health is a real injury, not this layer's death verdict. §三.4 keeps
	// that distinction, so it is reported as needing special treatment and the
	// rest is refused rather than the character being declared gone.
	downed, downedStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 0))
	_, eligibility, err := downed.RecoveryAttributes(downedStory, "main", DefaultActorID, "")
	if err != nil {
		t.Fatal(err)
	}
	if eligibility != cultivation.RecoveryNeedsSpecialTreatment {
		t.Fatalf("zero health must read as needing treatment, got %q", eligibility)
	}
	if _, err := downed.ApplySafeRecovery(downedStory, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-downed", CommandHash: "h",
	}); !errors.Is(err, cultivation.ErrRecoveryIneligible) {
		t.Fatalf("an injury needing special treatment must refuse ordinary rest, got %v", err)
	}
	if count := recoveryTestRecordCount(t, downed, downedStory); count != 0 {
		t.Fatalf("a refused rest must not reach the journal, got %d records", count)
	}

	// An eligible character is the control: the refusals above must come from the
	// conditions, not from the rest being unavailable to anyone.
	healthy, healthyStory := recoveryTestStory(t)
	if _, eligibility, err := healthy.RecoveryAttributes(healthyStory, "main", DefaultActorID, ""); err != nil || eligibility != cultivation.RecoveryQualified {
		t.Fatalf("an unhurt character out of battle must be qualified, got %q err=%v", eligibility, err)
	}
}

// TestRecoveryRecordAndSettlementCommitAsOneTransaction is §三.5's "失败不半扣半补"
// at the journal level: the interval and the pool it restored are one physical
// transaction, so a rest either exists with its recovery or does not exist.
func TestRecoveryRecordAndSettlementCommitAsOneTransaction(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-one-tx")

	store.mu.Lock()
	meta, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// The settlement is parented to the record. It is not necessarily the head:
	// the interval the rest spent may close other registered matters after it —
	// a task deadline, an NPC's growth — and those hang off the settlement.
	var delta *StoryEventRecord
	for index := range records {
		if parentIDFromRaw(records[index].Raw) != result.EventID {
			continue
		}
		if records[index].Envelope.Type == StoryEventTypeStateDelta {
			delta = &records[index]
			break
		}
	}
	if delta == nil {
		t.Fatalf("the settlement must be parented to the record %s", result.EventID)
	}
	head := meta.Branches["main"].Head
	if _, ok := eventsByID(records)[head]; !ok {
		t.Fatalf("the branch head %s must be a committed event", head)
	}

	// One physical transaction carries both, which is what makes the pair atomic.
	// The line is decoded rather than searched, so a payload that merely mentions
	// the other identity cannot be mistaken for a transaction that contains it.
	recordLine := recoveryTestTransactionLine(t, store, storyID, result.EventID)
	deltaLine := recoveryTestTransactionLine(t, store, storyID, delta.Envelope.ID)
	if deltaLine != recordLine {
		t.Fatalf("the record is on line %d and its settlement on line %d; they must commit together", recordLine, deltaLine)
	}
	// And the transaction really carries both records, rather than the record
	// being on the line and the settlement being absent altogether.
	payloads := recoveryTestTransactionPayloads(t, store, storyID, recordLine)
	if len(payloads) < 3 {
		t.Fatalf("the recovery transaction must carry the record, its settlement and the metadata, got %d payloads", len(payloads))
	}
}

// recoveryTestTransactionPayloads decodes one physical journal line into the
// payload records it committed.
func recoveryTestTransactionPayloads(t *testing.T, store *Store, storyID string, line int) []json.RawMessage {
	t.Helper()
	payloads, err := decodeConversationTransactionRecords([]byte(recoveryTestTransactionText(t, store, storyID, line)))
	if err != nil {
		t.Fatalf("line %d is not a conversation transaction: %v", line, err)
	}
	return payloads
}

// recoveryTestTransactionLine finds the physical JSONL line an event was
// committed on, by decoding each line's records and comparing identities.
func recoveryTestTransactionLine(t *testing.T, store *Store, storyID, eventID string) int {
	t.Helper()
	data, err := os.ReadFile(store.storyPath(storyID))
	if err != nil {
		t.Fatal(err)
	}
	for index, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		payloads, decodeErr := decodeConversationTransactionRecords([]byte(line))
		if decodeErr != nil {
			continue
		}
		for _, payload := range payloads {
			var envelope struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(payload, &envelope) == nil && envelope.ID == eventID {
				return index
			}
		}
	}
	t.Fatalf("event %s is not in the journal", eventID)
	return -1
}

// recoveryTestTransactionText returns one physical journal line verbatim.
func recoveryTestTransactionText(t *testing.T, store *Store, storyID string, line int) string {
	t.Helper()
	data, err := os.ReadFile(store.storyPath(storyID))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	if line < 0 || line >= len(lines) {
		t.Fatalf("line %d is outside the journal's %d lines", line, len(lines))
	}
	return lines[line]
}

// TestRecoveryAppendFailureLeavesNoHalfState is §三.7's "提交故障无半份状态".
//
// The journal is made unwritable, so the commit cannot land. Nothing may be half
// applied: no record, no pool write, and no time spent.
func TestRecoveryAppendFailureLeavesNoHalfState(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	path := store.storyPath(storyID)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-refused", CommandHash: "cmd-refused-hash",
	}); err == nil {
		t.Fatal("a journal that cannot be written must refuse the rest")
	}
	// The process must not guess. A journal that could not confirm its own append
	// does not know whether the write landed, so it refuses to write again rather
	// than risk settling the same interval twice. Refusing is the safe answer; a
	// second grant would be the unsafe one.
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-after", CommandHash: "cmd-after-hash",
	}); err == nil {
		t.Fatal("a journal whose append is unconfirmed must refuse further writes rather than guess")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	// Teardown sees the same refusal from the other side: a journal that could not
	// confirm its append will not checkpoint the projection either. Writing a
	// checkpoint for state it cannot vouch for is exactly how a half-applied rest
	// would survive a restart.
	if err := store.Close(); err == nil {
		t.Fatal("an unconfirmed journal must not checkpoint the projection it could not confirm")
	}

	// A fresh store reads the file rather than this process's caches, so what it
	// reports is what actually reached the disk.
	restarted := NewStore(store.Root())
	t.Cleanup(func() { _ = restarted.Close() })
	if count := recoveryTestRecordCount(t, restarted, storyID); count != 0 {
		t.Fatalf("a failed commit must leave no record behind, got %d", count)
	}
	if got := recoveryTestStoredPool(t, restarted, storyID, xiuxianFieldHealth); got != 37 {
		t.Fatalf("a failed commit must leave the pool alone, got %d", got)
	}
	clock, activities, err := restarted.RecoveryState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != 0 {
		t.Fatalf("a failed commit must not have spent time, got clock %d", clock.Seconds)
	}
	if len(activities) != 0 {
		t.Fatalf("a failed commit must not have left an activity, got %#v", activities)
	}

	// The story is not stuck: a reopened journal settles the same rest from the
	// untouched origin, which is only possible because nothing was half applied.
	settled, err := restarted.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: "main", ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-recovered", CommandHash: "cmd-recovered-hash",
	})
	if err != nil {
		t.Fatalf("a reopened journal must be able to settle the rest: %v", err)
	}
	if settled.Candidate.PreviousPool != 37 || settled.Candidate.CandidatePool != 47 {
		t.Fatalf("the retry must start from the untouched pool: %+v", settled.Candidate)
	}
}

// TestRecoverySurvivesAJournalRebuild is §三.7's "重启恢复": a reopened journal
// reproduces the rest, its activity and its instant from the record alone.
func TestRecoverySurvivesAJournalRebuild(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-restart")

	restarted := NewStore(store.Root())
	t.Cleanup(func() { _ = restarted.Close() })

	clock, activities, err := restarted.RecoveryState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clock.Seconds != result.ClockAfter.Seconds {
		t.Fatalf("a rebuilt journal must reproduce the instant: got %d, want %d", clock.Seconds, result.ClockAfter.Seconds)
	}
	activity, ok := activities[result.ActivityID]
	if !ok {
		t.Fatalf("a rebuilt journal must reproduce the activity: %#v", activities)
	}
	if activity.Activity.CompletedEffective != storytime.Duration(24*60*60) {
		t.Fatalf("the replayed activity must carry its settled seconds: %#v", activity.Activity)
	}
	if got := recoveryTestStoredPool(t, restarted, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("a rebuilt journal must reproduce the restored pool, got %d", got)
	}
}

// TestRecoveryDoesNotCrossStoriesOrBranches is §三.7's "同包两局和分支不串".
//
// Two stories made from the same pack share the rules and nothing else, and a
// branch that forked before a rest must not inherit it — a rest is taken against
// a particular set of wounds, so a branch cut before it never had them.
func TestRecoveryDoesNotCrossStoriesOrBranches(t *testing.T) {
	first, firstStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	second, secondStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))

	recoveryTestApply(t, first, firstStory, cultivation.RecoveryHealthRest, 24*60*60, "cmd-a")

	if clock, activities, err := second.RecoveryState(secondStory, "main"); err != nil {
		t.Fatal(err)
	} else if clock.Seconds != 0 || len(activities) != 0 {
		t.Fatalf("another story in the same pack must be untouched: clock=%d activities=%#v", clock.Seconds, activities)
	}
	if got := recoveryTestStoredPool(t, second, secondStory, xiuxianFieldHealth); got != 37 {
		t.Fatalf("another story's pool must be untouched, got %d", got)
	}

	// A turn after the rest, so the rest is no longer "now" and a fork of the
	// earlier turn gets the world as it stood before it.
	if _, _, err := first.AppendTurnWithState(firstStory, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "主角起身离洞。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	forked, err := first.CreateBranch(firstStory, CreateBranchRequest{
		ParentEventID: combatTestLatestTurnID(t, first, firstStory),
		Title:         "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The fork's parent is the turn after the rest, so it legitimately inherits
	// the rest; what matters is that it holds its own copy rather than sharing
	// main's.
	forkedClock, forkedActivities, err := first.RecoveryState(firstStory, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkedActivities) != 1 {
		t.Fatalf("a fork of the turn after the rest must inherit it: %#v", forkedActivities)
	}
	before := forkedClock.Seconds

	// A later rest on main must not move the fork's instant or its progress.
	recoveryTestApply(t, first, firstStory, cultivation.RecoveryHealthRest, 24*60*60, "cmd-main-again")
	afterFork, afterActivities, err := first.RecoveryState(firstStory, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFork.Seconds != before {
		t.Fatalf("a rest on main must not move the fork's clock: %d -> %d", before, afterFork.Seconds)
	}
	if len(afterActivities) != 1 {
		t.Fatalf("a rest on main must not add an activity to the fork: %#v", afterActivities)
	}
	mainClock, _, err := first.RecoveryState(firstStory, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainClock.Seconds != before+24*60*60 {
		t.Fatalf("main's own clock must carry both rests, got %d", mainClock.Seconds)
	}

	// And the mirror image: a rest taken on the fork stays on the fork.
	rested, err := first.ApplySafeRecovery(firstStory, SafeRecoveryRequest{
		BranchID: forked.ID, ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-fork-rest", CommandHash: "cmd-fork-rest-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rested.ClockAfter.Seconds != before+24*60*60 {
		t.Fatalf("the fork's own rest must advance the fork, got %d", rested.ClockAfter.Seconds)
	}
	mainAfter, _, err := first.RecoveryState(firstStory, "main")
	if err != nil {
		t.Fatal(err)
	}
	if mainAfter.Seconds != mainClock.Seconds {
		t.Fatalf("a rest on the fork must not move main's clock: %d -> %d", mainClock.Seconds, mainAfter.Seconds)
	}
}

// TestRecoveryForkOfTheRestsTurnInheritsNoneOfIt is §三.5's "失败不半扣半补" for
// the fork path.
//
// A rest is committed as a record parented to the Turn it was taken after, so a
// fork cut at that Turn sits *before* the rest. Restoring the pools to the
// pre-rest wound while carrying the rest's clock and its completed activity
// would leave the branch claiming time it never spent and a rest it never took,
// and the pool would never be restored because the activity reads as finished.
// The fork has to get the whole rest or none of it, and at this cut it is none.
func TestRecoveryForkOfTheRestsTurnInheritsNoneOfIt(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-day")

	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 47 {
		t.Fatalf("the rest must reach main's save before the fork is cut, got %d", got)
	}

	forked, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: combatTestLatestTurnID(t, store, storyID),
		Title:         "另一条路",
	})
	if err != nil {
		t.Fatal(err)
	}

	forkedClock, forkedActivities, err := store.RecoveryState(storyID, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(storyID, forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	health, ok := actorStateNumber(actorStateFieldValue(snapshot.State, DefaultActorID, xiuxianFieldHealth))
	if !ok {
		t.Fatalf("the fork must carry a health pool: %#v", snapshot.State)
	}

	if int(health) != 37 {
		t.Fatalf("a fork cut before the rest must keep the wound: got %d, want 37", int(health))
	}
	if forkedClock.Seconds != 0 {
		t.Fatalf("a fork cut before the rest must not claim the rest's instant: got %d, want 0", forkedClock.Seconds)
	}
	if len(forkedActivities) != 0 {
		t.Fatalf("a fork cut before the rest must not carry its activity: %#v", forkedActivities)
	}
	// The fork's own rest must still be available: it starts from the wound, not
	// from a record that reads as already finished.
	rested, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: forked.ID, ActorID: DefaultActorID, Mode: cultivation.RecoveryHealthRest,
		DurationSeconds: 24 * 60 * 60, CommandID: "cmd-fork-day", CommandHash: "cmd-fork-day-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rested.Candidate.CandidatePool != 47 {
		t.Fatalf("the fork must be able to earn its own day of rest, got %d", rested.Candidate.CandidatePool)
	}
	if rested.ClockAfter.Seconds != result.ClockAfter.Seconds {
		t.Fatalf("the fork's own rest must move its own clock to the same interval: got %d, want %d",
			rested.ClockAfter.Seconds, result.ClockAfter.Seconds)
	}
}

// TestRecoveryRecordKeepsTheBranchParentChainWalkable is the same regression the
// combat settlement has: the recovery commits a record and then a settlement
// parented to it, so an event that reused its parent's identity would close the
// chain into a cycle and every later walk would hang.
func TestRecoveryRecordKeepsTheBranchParentChainWalkable(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-walk")

	store.mu.Lock()
	meta, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	byID := map[string]map[string]any{}
	for _, record := range records {
		id := record.Envelope.ID
		if id == "" {
			t.Fatalf("every event needs an identity: %#v", record.Envelope)
		}
		if seen[id] {
			t.Fatalf("event identity %q is used twice", id)
		}
		seen[id] = true
		byID[id] = record.Raw
		if parent := parentIDFromRaw(record.Raw); parent != "" && parent == id {
			t.Fatalf("event %q is its own parent, which closes the chain into a cycle", id)
		}
	}
	steps := 0
	for cursor := meta.Branches["main"].Head; cursor != ""; {
		steps++
		if steps > len(records)+1 {
			t.Fatalf("the parent chain did not terminate within %d steps; it is cyclic", len(records)+1)
		}
		raw, ok := byID[cursor]
		if !ok {
			break
		}
		cursor = parentIDFromRaw(raw)
	}
	if _, err := store.ReadHistoryPage(storyID, "main", "", 20); err != nil {
		t.Fatalf("history must be readable after a rest: %v", err)
	}
	if _, err := store.Snapshot(storyID, "main"); err != nil {
		t.Fatalf("the snapshot must be readable after a rest: %v", err)
	}
}

// TestAWoundCapsRecoveryAtWhatTheBodyStillAllows is T21's real gap: the residual
// floor only ever limited how far the meridian burden could fall. It never
// limited a pool, so a character carrying an unhealed injury still rested all
// the way back to the stage table's maximum and the interface called that
// "完全恢复" while the injury was still there.
//
// The cap is not a second system. A lasting effect recorded on 持续状态 in the
// object form the field's own instruction describes — 属性 生命上限, 数值 -30 —
// already flows through the equipment readers into the aggregated attributes, so
// the recovery pool's maximum falls with it and PlanSafeRecovery stops there.
// This test pins that chain: a 30% wound on a 200-point pool caps health at 140,
// a full month of rest stops at 140 rather than 200, and the character is still
// reported as wounded afterwards.
func TestAWoundCapsRecoveryAtWhatTheBodyStillAllows(t *testing.T) {
	wound := []any{map[string]any{
		"名称": "经脉内伤", "属性": "生命上限", "数值": float64(-30), "同类": "伤势",
	}}
	store, storyID := recoveryTestStory(t,
		recoveryTestPool(xiuxianFieldHealth, 37),
		recoveryTestReplace(xiuxianFieldEffects, wound),
	)

	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 30*24*60*60, "cmd-wounded")

	if result.Candidate.PoolMaximum != 140 {
		t.Fatalf("a 30%% wound must cap the pool at 140 of 200, got %d", result.Candidate.PoolMaximum)
	}
	if result.Candidate.CandidatePool != 140 {
		t.Fatalf("a month of rest must stop at the wounded maximum, got %d", result.Candidate.CandidatePool)
	}
	// §T21 forbids the wound quietly healing because the bar filled up.
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 140 {
		t.Fatalf("the saved pool must stop at the wounded maximum, got %d", got)
	}
	state := combatTestState(t, store, storyID)
	if _, present := actorStateFieldValue(state, DefaultActorID, xiuxianFieldEffects).([]any); !present {
		t.Fatalf("resting must not erase the injury it could not heal: %#v", state)
	}

	// The control: the same rest without the injury really does reach 200, so the
	// cap above comes from the wound and not from the rest being short.
	healthy, healthyStory := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 37))
	whole := recoveryTestApply(t, healthy, healthyStory, cultivation.RecoveryHealthRest, 30*24*60*60, "cmd-whole")
	if whole.Candidate.PoolMaximum != 200 || whole.Candidate.CandidatePool != 200 {
		t.Fatalf("an unwounded character must still reach the stage maximum: %+v", whole.Candidate)
	}
}

// TestAHealedWoundLiftsTheCapWithoutRefillingThePool is §T21's other half of the
// same sentence: 伤愈后才解除限制；解除伤势不自动补满生命/灵力.
//
// The cap above is a consequence of the wound, so removing the wound has to lift
// it — otherwise a character who heals is stuck at the reduced maximum forever.
// But lifting the cap is not a recovery. The pool the character had actually
// earned stays where it was, and the rest that follows earns its own points at
// the full maximum's rate rather than being handed the difference for free.
func TestAHealedWoundLiftsTheCapWithoutRefillingThePool(t *testing.T) {
	wound := []any{map[string]any{
		"名称": "经脉内伤", "属性": "生命上限", "数值": float64(-30), "同类": "伤势",
	}}
	store, storyID := recoveryTestStory(t,
		recoveryTestPool(xiuxianFieldHealth, 37),
		recoveryTestReplace(xiuxianFieldEffects, wound),
	)
	wounded := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 30*24*60*60, "cmd-before-heal")
	if wounded.Candidate.CandidatePool != 140 {
		t.Fatalf("the wounded rest must stop at 140 first: %+v", wounded.Candidate)
	}

	// The wound heals the way the story heals one: a turn records that 持续状态
	// no longer holds it. Nothing about that turn touches 生命.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "伤愈", Narrative: "伤势平复，主角试着起身。",
		TurnResult: &TurnResult{
			StateUpdates: []interactivestate.Update{recoveryTestReplace(xiuxianFieldEffects, []any{})},
			Choices:      testTurnChoices(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 140 {
		t.Fatalf("healing the wound must not refill the pool, got %d, want the 140 already earned", got)
	}

	// One day of rest after the heal is worth a twentieth of the restored 200 —
	// ten points — so the pool climbs from 140 to 150 and is not topped up.
	after := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-after-heal")
	if after.Candidate.PoolMaximum != 200 {
		t.Fatalf("a healed character's maximum must be the stage table's again, got %d", after.Candidate.PoolMaximum)
	}
	if after.Candidate.CandidatePool != 150 {
		t.Fatalf("a day of rest must add ten points to the 140 earned, got %d", after.Candidate.CandidatePool)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 150 {
		t.Fatalf("the post-heal rest must reach the save, got %d", got)
	}
}

// TestAnOrdinaryShortfallRecoversToTheMaximumButNotPastIt is §T21's 普通轻微消耗
// 可恢复, and the boundary that keeps the wound cap from becoming a general
// ceiling.
//
// A character who has spent a little health and carries no wound must be able to
// rest that back, and the rest must stop at the maximum rather than overshooting
// it — the cap belongs to the stage table, not to the arithmetic of adding a
// percentage.
func TestAnOrdinaryShortfallRecoversToTheMaximumButNotPastIt(t *testing.T) {
	store, storyID := recoveryTestStory(t, recoveryTestPool(xiuxianFieldHealth, 195))

	result := recoveryTestApply(t, store, storyID, cultivation.RecoveryHealthRest, 24*60*60, "cmd-light")

	if result.Candidate.PoolMaximum != 200 {
		t.Fatalf("a light shortfall must be measured against the full maximum, got %d", result.Candidate.PoolMaximum)
	}
	if result.Candidate.CandidatePool != 200 {
		t.Fatalf("five points short must rest back to 200 and stop there, got %d", result.Candidate.CandidatePool)
	}
	if got := recoveryTestStoredPool(t, store, storyID, xiuxianFieldHealth); got != 200 {
		t.Fatalf("the settled pool must be the maximum, got %d", got)
	}
}
