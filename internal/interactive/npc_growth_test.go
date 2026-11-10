package interactive

import (
	"fmt"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/npcgrowth"
)

func TestNPCGrowthRequiresFrozenSpeciesSourceForNonHumanCultivation(t *testing.T) {
	profile := character.Profile{ID: "bird", SourceKey: "species.bird", Name: "青风雀"}
	projection := &storyBranchProjection{
		Characters: &character.State{Profiles: []character.Profile{profile}},
		PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{
			SourceKey: "species.bird", Name: "青风雀", Lifeform: PackCatalogLifeformSpiritBeast,
			CultivationMethodID: "method.spirit_beast_breathing",
		}}},
	}
	method := npcGrowthSpeciesMethod(projection, "bird")
	if method == nil || method.GrowthCoefficient != 0.60 {
		t.Fatalf("frozen species method = %+v, want registered 0.60 method", method)
	}
	conditions := npcGrowthConditions(map[string]any{}, "bird", cultivation.CharacterState{}, 10, 10, cultivation.BaseAttributes(cultivation.Qi(1)))
	conditions.GrowthCoefficient = method.GrowthCoefficient
	if conditions.GrowthCoefficient != 0.60 {
		t.Fatalf("species growth coefficient = %v, want 0.60", conditions.GrowthCoefficient)
	}

	projection.PackCatalog.Characters[0].Lifeform = PackCatalogLifeformAnimal
	projection.PackCatalog.Characters[0].CultivationMethodID = ""
	if got := npcGrowthSpeciesMethod(projection, "bird"); got != nil {
		t.Fatalf("ordinary animal received a cultivation method: %+v", got)
	}
	if got, ok := npcGrowthCatalogPerson(projection, "unregistered-bird"); ok || got != nil {
		t.Fatalf("unbound actor matched a species source: %+v, %v", got, ok)
	}
}

func TestNPCGrowthAllowsARegisteredHumanWithExplicitLifeform(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	store.mu.Lock()
	projection, err := store.storyBranchProjectionLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	profile := character.Profile{ID: "rival", SourceKey: "person.rival", Name: "沈青"}
	projection.Characters = &character.State{Profiles: []character.Profile{profile}}
	system := npcGrowthTestSystem(t, store, storyID)
	before := StoryClock{Version: StoryClockVersion}
	after := StoryClock{Version: StoryClockVersion, Seconds: 30 * 24 * 60 * 60}

	settle := func(lifeform string) []any {
		projection.PackCatalog = &PackCatalogSnapshot{Characters: []PackCatalogPerson{{
			SourceKey: "person.rival", Name: profile.Name, Lifeform: lifeform,
		}}}
		events, _ := npcGrowthActorEvents(projection, system, "main", projection.Head, "test-now", "turn-id", "rival", before, after)
		return events
	}
	blank := settle("")
	human := settle(PackCatalogLifeformHuman)
	if len(blank) == 0 {
		t.Fatal("the baseline human NPC did not settle growth")
	}
	if len(human) == 0 {
		t.Fatal("an explicitly classified human NPC was incorrectly treated as a non-human species")
	}
	blankRecord, blankOK := blank[0].(NPCGrowthRecordEvent)
	humanRecord, humanOK := human[0].(NPCGrowthRecordEvent)
	if !blankOK || !humanOK || blankRecord.Candidate.NextState.Accumulated != humanRecord.Candidate.NextState.Accumulated {
		t.Fatalf("classifying a human changed the same NPC growth outcome: blank=%#v human=%#v", blank, human)
	}
}

// These tests are 《自制-NPC自主成长》 as it happens during ordinary play: an
// interval of committed formal time settles every tracked NPC's growth through
// the rules, on the same pass that closes the branch's other registered matters.
//
// The rules package already proves the arithmetic. What these tests add is that
// the settlement really runs from a commit, really writes what the rules
// answered, really keeps one identity per window, and really rewinds with the
// record a reroll replaces.

// npcGrowthTestStory builds a story whose cast has one non-protagonist
// cultivator beside the player, so there is something for the pass to settle.
func npcGrowthTestStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "NPC成长", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，同门在旁练剑。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// npcGrowthTestSystem returns the story's frozen actor-state schema as the
// settlement reads it.
func npcGrowthTestSystem(t *testing.T, store *Store, storyID string) StoryDirectorActorStateSystem {
	t.Helper()
	store.mu.Lock()
	meta, _, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return actorStateSystemFromSnapshot(meta.ActorStateSchema, store.storyDirectorForMeta(meta).ActorState)
}

// npcGrowthTestRecords returns the growth records on the branch's journal, in
// journal order.
func npcGrowthTestRecords(t *testing.T, store *Store, storyID string) []NPCGrowthRecordEvent {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var growth []NPCGrowthRecordEvent
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeNPCGrowthRecord {
			continue
		}
		var event NPCGrowthRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		growth = append(growth, event)
	}
	return growth
}

// npcGrowthTestProgress reads one actor's banked progress from the branch state.
func npcGrowthTestProgress(t *testing.T, store *Store, storyID, actorID string) float64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	value, _ := actorStateNumber(actorStateFieldValue(snapshot.State, actorID, xiuxianFieldRealmProgress))
	return value
}

// TestAnOrdinaryIntervalGrowsEveryTrackedNPC is the core of §5: real elapsed
// time really moves a character who is not the protagonist, and the movement is
// a committed record rather than a number the panel derives while reading.
func TestAnOrdinaryIntervalGrowsEveryTrackedNPC(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)

	before := npcGrowthTestProgress(t, store, storyID, "rival")
	// A turn that rests spends real time on the branch's formal clock. The rest
	// is the protagonist's own activity; the rival's growth rides the same
	// interval because the interval is the branch's, not the protagonist's.
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-rest")
	after := npcGrowthTestProgress(t, store, storyID, "rival")

	records := npcGrowthTestRecords(t, store, storyID)
	if len(records) == 0 {
		t.Fatal("an interval of committed time must settle the tracked NPC's growth")
	}
	var rival *NPCGrowthRecordEvent
	for index := range records {
		if records[index].Candidate.ActorID == "rival" {
			rival = &records[index]
			break
		}
	}
	if rival == nil {
		t.Fatalf("the rival must have a growth record; records=%+v", records)
	}
	if after <= before {
		t.Fatalf("the rival must have gained progress: %v -> %v", before, after)
	}
	if rival.Candidate.NextState.Accumulated != after {
		t.Fatalf("the committed record and the stored state must agree: record=%v state=%v",
			rival.Candidate.NextState.Accumulated, after)
	}
	// The protagonist's own interval is paid by the rest record. Settling them
	// here as well would pay the same hour twice under two names.
	for _, record := range records {
		if record.Candidate.ActorID == DefaultActorID {
			t.Fatal("the protagonist's time is the player's own interval and must not be settled twice")
		}
	}
}

// TestTheSameIntervalIsNeverPaidTwice is §9's retry rule: a re-emitted pass
// presents one identity, and the rules answer 重复 rather than granting the
// same stretch of life a second time.
func TestTheSameIntervalIsNeverPaidTwice(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-once")

	first := npcGrowthTestRecords(t, store, storyID)
	if len(first) == 0 {
		t.Fatal("the first interval must settle")
	}
	settled := npcGrowthTestProgress(t, store, storyID, "rival")

	// Re-emit the same pass over the same window from the same branch position.
	// This is what a retry, a reopened panel and a re-derived reroll all do.
	store.mu.Lock()
	projection, err := store.storyBranchProjectionLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	system := npcGrowthTestSystem(t, store, storyID)

	var last NPCGrowthRecordEvent
	for _, record := range first {
		if record.Candidate.ActorID == "rival" {
			last = record
		}
	}
	events, _ := npcGrowthEvents(projection, system, "main", projection.Head, "now", "", "", last.ClockBefore, last.ClockAfter)
	for _, event := range events {
		if record, ok := event.(NPCGrowthRecordEvent); ok && record.Candidate.ActorID == "rival" {
			t.Fatalf("the same window must not be settled a second time; got %+v", record)
		}
	}
	if got := npcGrowthTestProgress(t, store, storyID, "rival"); got != settled {
		t.Fatalf("a duplicate pass must not move the stored position: %v -> %v", settled, got)
	}
}

// TestAMortalStillSettlesItsWatermark is the "don't invent growth" half of §3.
//
// TestAMortalIsNotInventedIntoAGrowthRecord is the "don't invent growth" half
// of §3.
//
// The rules answer a mortal's interval with no state change at all: becoming a
// cultivator is a story event, not a threshold crossed by waiting. Committing a
// record for them would be the settlement authoring a fact the rules refused to
// produce, so nothing is written — and because the watermark does not move, the
// skipped window reads as still-unpaid time rather than as a settled one.
func TestAMortalIsNotInventedIntoAGrowthRecord(t *testing.T) {
	system := combatTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID == "rival" {
			system.InitialActors[index].State[xiuxianFieldRealm] = cultivation.StageKey(cultivation.Mortal())
		}
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "凡人同门", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，凡人的同门正在挑水。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryTestApply(t, store, story.ID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-mortal")
	for _, record := range npcGrowthTestRecords(t, store, story.ID) {
		if record.Candidate.ActorID == "rival" {
			t.Fatalf("a mortal has no growth the rules produced, so none may be recorded: %+v", record)
		}
	}
	if got := npcGrowthTestProgress(t, store, story.ID, "rival"); got != 0 {
		t.Fatalf("a mortal cannot accumulate cultivation progress: %v", got)
	}
}

// npcGrowthTestFailingDuration finds a window length whose bound draw fails the
// foundation attempt.
//
// The draw is a pure function of the attempt identity, so which way it lands is
// a property of the window rather than of the test run. A test that has to
// exercise the failure path therefore picks a window that fails instead of
// hoping for one — and it picks it by asking the same derivation the production
// code asks, not by reaching into the record after the fact.
func npcGrowthTestFailingDuration(t *testing.T, from int64) int64 {
	t.Helper()
	chance, ok := cultivation.BreakthroughChance(cultivation.BreakthroughFoundation, cultivation.BreakthroughModifiers{
		Talent: cultivation.TalentNormal, Quality: cultivation.QualityOrdinary,
	})
	if !ok {
		t.Fatal("the foundation breakthrough chance must be published")
	}
	// Start past the seven-day preparation so the window can hold the attempt and
	// at least part of the seven-day recovery that follows a failure.
	for days := int64(10); days <= 200; days++ {
		to := from + days*24*60*60
		if npcGrowthBreakthroughRoll("main", "rival", from, to, cultivation.BreakthroughFoundation) >= chance {
			return days * 24 * 60 * 60
		}
	}
	t.Fatal("no window in the scanned range produces a failing draw")
	return 0
}

// npcGrowthTestBoundaryStory builds a story whose rival stands at the qi-13
// threshold with a full pool and a recorded method.
//
// The stage, the accumulation and the pools are seeded through the initial actor
// because 当前境界 and 境界进度 are settlement-owned: the rules accept a
// character's starting position as a story fact and refuse a turn update that
// rewrites it, so the boundary has to exist before the first turn.
func npcGrowthTestBoundaryStory(t *testing.T) (*Store, string) {
	t.Helper()
	system := combatTestSystem()
	qi := cultivation.Qi(cultivation.QiLayers)
	requirement := cultivation.BaseAttributes(qi)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != "rival" {
			continue
		}
		system.InitialActors[index].State[xiuxianFieldRealm] = cultivation.StageKey(qi)
		system.InitialActors[index].State[xiuxianFieldRealmProgress] = 16900.0
		system.InitialActors[index].State[xiuxianFieldHealth] = float64(requirement.HealthMax)
		system.InitialActors[index].State[xiuxianFieldMana] = float64(requirement.ManaMax)
		system.InitialActors[index].State[xiuxianFieldMethod] = "无名散修功法，均衡，普通。"
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "瓶颈同门", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，同门已在瓶颈上坐了三年。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// npcGrowthTestRivalRecords returns only the rival's growth records.
func npcGrowthTestRivalRecords(t *testing.T, store *Store, storyID string) []NPCGrowthRecordEvent {
	t.Helper()
	var rival []NPCGrowthRecordEvent
	for _, record := range npcGrowthTestRecords(t, store, storyID) {
		if record.Candidate.ActorID == "rival" {
			rival = append(rival, record)
		}
	}
	return rival
}

// npcGrowthTestRealm reads one actor's committed stage key.
func npcGrowthTestRealm(t *testing.T, store *Store, storyID, actorID string) string {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	value, _ := actorStateFieldValue(snapshot.State, actorID, xiuxianFieldRealm).(string)
	return value
}

// TestABottleneckNPCMakesOneSourcedBreakthroughAttempt is the missing承接 §6 asks
// for: a character who has reached the threshold, whose method reaches the next
// realm, and whose body and mana are whole makes the attempt its own position
// licenses — and the attempt is a committed record, not a panel reading.
//
// The draw is bound to the attempt identity, so the outcome is fixed for this
// window. Both outcomes are legitimate; what must hold either way is that the
// attempt really happened, really cost its preparation time, and really left a
// trace the rules can see next time.
func TestABottleneckNPCMakesOneSourcedBreakthroughAttempt(t *testing.T) {
	store, storyID := npcGrowthTestBoundaryStory(t)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-bottleneck")

	records := npcGrowthTestRivalRecords(t, store, storyID)
	if len(records) == 0 {
		t.Fatal("a character at the threshold must have a growth record for the interval")
	}

	var attempt *NPCGrowthRecordEvent
	for index := range records {
		if records[index].Input.BreakthroughIntent != nil {
			attempt = &records[index]
			break
		}
	}
	if attempt == nil {
		t.Fatalf("an eligible NPC at the threshold must make an attempt; records=%s", npcGrowthTestDescribe(records))
	}
	intent := *attempt.Input.BreakthroughIntent
	if intent.Target != cultivation.BreakthroughFoundation {
		t.Fatalf("qi-13 stands before foundation: %s", intent.Target)
	}
	if intent.RandomReceipt == nil {
		t.Fatal("the committed attempt must carry the draw it was resolved with")
	}
	if attempt.Input.Interval.Segments[0].Activity != npcgrowth.ActivityBreakthroughPrep {
		t.Fatalf("the attempt must occupy an explicit preparation segment: %s", attempt.Input.Interval.Segments[0].Activity)
	}
	// §6: the draw is bound to a stable attempt identity, so a replay cannot
	// re-roll. Deriving it again from the same window must give the same number.
	again := npcGrowthBreakthroughRoll("main", "rival", attempt.ClockBefore.Seconds, attempt.ClockAfter.Seconds, intent.Target)
	if again != intent.RandomReceipt.Roll {
		t.Fatalf("the draw must be bound to the attempt identity: %v != %v", again, intent.RandomReceipt.Roll)
	}

	candidate := attempt.Candidate
	if candidate.Breakthrough == nil {
		t.Fatal("the record must carry the rules' breakthrough answer")
	}
	switch candidate.Status {
	case npcgrowth.StatusBreakthroughSucceeded:
		realm := npcGrowthTestRealm(t, store, storyID, "rival")
		if realm != cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}) {
			t.Fatalf("a successful attempt must commit the new realm: %s", realm)
		}
		if progress := npcGrowthTestProgress(t, store, storyID, "rival"); progress != 0 {
			t.Fatalf("a new realm starts with nothing banked: %v", progress)
		}
	case npcgrowth.StatusBreakthroughFailed:
		if candidate.Breakthrough.RecoveryUntilSeconds == 0 {
			t.Fatal("a failed attempt must impose the rules' recovery period")
		}
		if progress := npcGrowthTestProgress(t, store, storyID, "rival"); progress >= 16900 {
			t.Fatalf("a failure costs part of the accumulation: %v", progress)
		}
	default:
		t.Fatalf("the committed attempt must be resolved, not merely priced: %s", candidate.Status)
	}
}

// TestAFailedAttemptIsNotRetriedInsideItsRecovery is §6's "不得在同一个时刻无限重复
// 尝试": the recovery the failure imposed is a real interval of the actor's life,
// and a fresh attempt waits for it to end.
func TestAFailedAttemptIsNotRetriedInsideItsRecovery(t *testing.T) {
	store, storyID := npcGrowthTestBoundaryStory(t)
	// Enough time for the preparation, the attempt and part of the recovery, but
	// not for the recovery to end. The window is chosen so its bound draw fails,
	// because the rule under test is the failure path.
	duration := npcGrowthTestFailingDuration(t, 0)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, duration, "cmd-npc-fail-window")

	records := npcGrowthTestRivalRecords(t, store, storyID)
	var failure *NPCGrowthRecordEvent
	for index := range records {
		if records[index].Candidate.Status == npcgrowth.StatusBreakthroughFailed {
			failure = &records[index]
		}
	}
	if failure == nil {
		t.Fatalf("this window's bound draw was chosen to fail but no failure was recorded; records=%s", npcGrowthTestDescribe(records))
	}
	recoveryUntil := failure.Candidate.Breakthrough.RecoveryUntilSeconds

	// The next interval begins inside the recovery and must settle it rather than
	// roll again. No record may carry a fresh intent before the deadline.
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 3*24*60*60, "cmd-npc-in-recovery")
	for _, record := range npcGrowthTestRivalRecords(t, store, storyID) {
		if record.ID == failure.ID {
			continue
		}
		if record.ClockBefore.Seconds < recoveryUntil && record.Input.BreakthroughIntent != nil {
			t.Fatalf("an attempt was made inside the imposed recovery (until %d) at %d", recoveryUntil, record.ClockBefore.Seconds)
		}
	}
}

// TestBreakthroughRecoveryEndsAndTheBottleneckIsRechecked is the other half of
// the same rule: the recovery is finite, and when it ends the character is
// re-examined rather than frozen.
//
// Re-examination is not the same as a second attempt. A failure costs a tenth
// of the health maximum, and this pass has no passive mana recovery, so the
// honest answer after a recovery is usually that the body no longer meets §6's
// floor and the character stays at the bottleneck — which the design permits
// explicitly ("没有合法突破计划时停在瓶颈"). What the test can require is that the
// recovery ended, that the character's time was not stopped by it, and that any
// later attempt happened only on terms the rules could actually grant.
func TestBreakthroughRecoveryEndsAndTheBottleneckIsRechecked(t *testing.T) {
	store, storyID := npcGrowthTestBoundaryStory(t)
	duration := npcGrowthTestFailingDuration(t, 0)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, duration, "cmd-npc-first-window")

	records := npcGrowthTestRivalRecords(t, store, storyID)
	var failure *NPCGrowthRecordEvent
	for index := range records {
		if records[index].Candidate.Status == npcgrowth.StatusBreakthroughFailed {
			failure = &records[index]
		}
	}
	if failure == nil {
		t.Fatalf("this window's bound draw was chosen to fail but no failure was recorded; records=%s", npcGrowthTestDescribe(records))
	}
	recoveryUntil := failure.Candidate.Breakthrough.RecoveryUntilSeconds

	// A window long enough to outlast the recovery.
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-after-recovery")

	var ended bool
	for _, record := range npcGrowthTestRivalRecords(t, store, storyID) {
		if record.ID == failure.ID {
			continue
		}
		if record.Candidate.Status == npcgrowth.StatusRecoveryEnded {
			ended = true
		}
		if record.Input.BreakthroughIntent == nil {
			continue
		}
		if record.ClockBefore.Seconds < recoveryUntil {
			t.Fatalf("an attempt was made before the recovery ended at %d", recoveryUntil)
		}
		// A later attempt is legal only if the rules granted it: the record's own
		// preparation segment and receipt are the proof.
		if record.Input.Interval.Segments[0].Activity != npcgrowth.ActivityBreakthroughPrep {
			t.Fatalf("an attempt outside a preparation segment: %s", record.Input.Interval.Segments[0].Activity)
		}
		if record.Input.BreakthroughIntent.RandomReceipt == nil {
			t.Fatalf("a committed attempt must carry the draw it was resolved with: %+v", record.Input.BreakthroughIntent)
		}
	}
	if !ended {
		t.Fatalf("the imposed recovery must settle to its end rather than stop the actor's life; records=%s", npcGrowthTestDescribe(npcGrowthTestRivalRecords(t, store, storyID)))
	}
	// The recovery really did cost something: the attempt that follows may not be
	// a silent repeat of the first, so the character's accumulation must have
	// moved or its pools must reflect the loss.
	if progress := npcGrowthTestProgress(t, store, storyID, "rival"); progress >= 16900 {
		t.Fatalf("a failed attempt must cost part of the accumulation: %v", progress)
	}
}

// TestABottleneckNPCWithoutAMethodStaysAtTheBottleneck is §3's "画像没有提供的功法
// 不得由结算器任意补齐" at the one place it would be tempting to break it: a
// character at the threshold who has no method recorded.
//
// The rules refuse to roll without a method that reaches the target realm, so
// nothing is committed and the window stays unpaid. Inventing a method to let
// the character through would be the settlement authoring a story fact.
func TestABottleneckNPCWithoutAMethodStaysAtTheBottleneck(t *testing.T) {
	system := combatTestSystem()
	qi := cultivation.Qi(cultivation.QiLayers)
	attributes := cultivation.BaseAttributes(qi)
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != "rival" {
			continue
		}
		system.InitialActors[index].State[xiuxianFieldRealm] = cultivation.StageKey(qi)
		system.InitialActors[index].State[xiuxianFieldRealmProgress] = 16900.0
		system.InitialActors[index].State[xiuxianFieldHealth] = float64(attributes.HealthMax)
		system.InitialActors[index].State[xiuxianFieldMana] = float64(attributes.ManaMax)
		system.InitialActors[index].State[xiuxianFieldMethod] = "未定"
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "无功法同门", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角在山门中停下，同门在瓶颈上枯坐，却始终没有一部可修的功法。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryTestApply(t, store, story.ID, cultivation.RecoverySafeMeditation, 60*24*60*60, "cmd-npc-methodless")
	for _, record := range npcGrowthTestRivalRecords(t, store, story.ID) {
		if record.Input.BreakthroughIntent != nil {
			t.Fatalf("a character with no recorded method may not attempt: %+v", record.Input.BreakthroughIntent)
		}
	}
	if realm := npcGrowthTestRealm(t, store, story.ID, "rival"); realm != cultivation.StageKey(qi) {
		t.Fatalf("the character must stay where it stands: %s", realm)
	}
	if progress := npcGrowthTestProgress(t, store, story.ID, "rival"); progress != 16900 {
		t.Fatalf("nothing was spent, so nothing may have moved: %v", progress)
	}
}

// TestARewoundWatermarkDoesNotPayTheSameWindowTwice is §9's rewind rule at the
// breakthrough boundary, where the watermark stops short of the window's close.
//
// A reroll re-emits the interval from the position before it. An actor stopped
// partway through — at a boundary, or inside a recovery — carries the interval
// identity but not the window's end, so the rewind has to recognise the
// identity rather than the close, or the replacement pass would answer 重复 and
// the branch would lose the growth along with the record it replaced.
func TestARewoundWatermarkDoesNotPayTheSameWindowTwice(t *testing.T) {
	from := StoryClock{Version: StoryClockVersion, Seconds: 1000}
	to := StoryClock{Version: StoryClockVersion, Seconds: 2000}
	partial := npcgrowth.Snapshot{
		Revision: 3, SettledThroughSeconds: 1500,
		LastIntervalID: npcGrowthIntervalID("main", "rival", from.Seconds, to.Seconds),
		// The shape the rules actually store: a recovery deadline always belongs
		// to a completed attempt, and that attempt's start is what dates it.
		Breakthrough: &npcgrowth.BreakthroughState{
			RecoveryUntilSeconds: 1600,
			LastAttempt: &npcgrowth.CompletedBreakthrough{
				StartedAtSeconds: 1100, CompletedAtSeconds: 1200,
				Status: npcgrowth.StatusBreakthroughFailed,
			},
		},
	}
	other := npcgrowth.Snapshot{
		Revision: 7, SettledThroughSeconds: 900,
		LastIntervalID: npcGrowthIntervalID("main", "rival", 800, 900),
	}
	projection := &storyBranchProjection{NPCGrowth: map[string]npcgrowth.Snapshot{
		"rival": partial, "bystander": other,
	}}

	rewound := intervalRewindNPCGrowth(projection, "main", from, to)
	if got := rewound["rival"].SettledThroughSeconds; got != from.Seconds {
		t.Fatalf("the interval's own watermark must rewind to its start: %d", got)
	}
	if rewound["rival"].LastIntervalID != "" || rewound["rival"].Revision != 2 {
		t.Fatalf("the interval identity must be cleared and the revision stepped back: %+v", rewound["rival"])
	}
	if rewound["rival"].Breakthrough != nil {
		t.Fatalf("the recovery this window imposed must leave with it: %+v", rewound["rival"].Breakthrough)
	}
	if got := rewound["bystander"]; got.SettledThroughSeconds != 900 || got.Revision != 7 {
		t.Fatalf("another interval's settlement must be left alone: %+v", got)
	}
	// The projection itself must not be mutated: the caller copies it shallowly.
	if projection.NPCGrowth["rival"].SettledThroughSeconds != 1500 {
		t.Fatal("the rewind must not rewrite the live projection")
	}
}

// npcGrowthTestDescribe renders the rival's records compactly for a failure.
func npcGrowthTestDescribe(records []NPCGrowthRecordEvent) string {
	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, fmt.Sprintf("%s/%s@%d->%d", record.Candidate.Status, record.Candidate.NextState.Cultivation.Realm, record.ClockBefore.Seconds, record.Candidate.ConsumedThrough))
	}
	return strings.Join(parts, ", ")
}

// committed facts rather than asserted: a character too badly hurt to train is
// on the 0% row, and the record says which row was used.
func TestAnOrdinaryIntervalKeepsTheActorsOwnPools(t *testing.T) {
	// A defence method lifts the formal maximum by a tenth, and the character is
	// stored at that lifted maximum. Training neither wounds nor heals, so the
	// interval must leave those two numbers exactly where they were: the formal
	// basis is what the breakthrough rules are written against, not a number the
	// settlement may commit in place of the actor's own maximum.
	system := combatTestSystem()
	qi := cultivation.Qi(cultivation.QiLayers)
	lifted := cultivation.BaseAttributes(qi)
	lifted.HealthMax += lifted.HealthMax / 10
	for index := range system.InitialActors {
		if system.InitialActors[index].ID != "rival" {
			continue
		}
		system.InitialActors[index].State[xiuxianFieldRealm] = cultivation.StageKey(qi)
		system.InitialActors[index].State[xiuxianFieldRealmProgress] = 100.0
		system.InitialActors[index].State[xiuxianFieldHealth] = float64(lifted.HealthMax)
		system.InitialActors[index].State[xiuxianFieldMana] = float64(lifted.ManaMax)
		system.InitialActors[index].State[xiuxianFieldMethod] = "山门护法诀，防护，普通。"
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "护体功法", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "同门运转护体功法，气机沉稳。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryTestApply(t, store, story.ID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-pools")

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := actorStateNumber(actorStateFieldValue(snapshot.State, "rival", xiuxianFieldHealth)); int(value) != lifted.HealthMax {
		t.Fatalf("ordinary training must not trim a bonus-raised pool: want %d, got %v", lifted.HealthMax, value)
	}
	if value, _ := actorStateNumber(actorStateFieldValue(snapshot.State, "rival", xiuxianFieldMana)); int(value) != lifted.ManaMax {
		t.Fatalf("ordinary training must not trim a bonus-raised mana pool: want %d, got %v", lifted.ManaMax, value)
	}
}

func TestAnInjuredNPCTrainsAtTheInjuryRate(t *testing.T) {
	store, storyID := npcGrowthTestStory(t)
	// A real wound, written through the same update path the model uses.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "查看同门", Narrative: "同门替主角挡了一剑，血透重衣。",
		TurnResult: &TurnResult{
			StateUpdates: []interactivestate.Update{{
				Op:    interactivestate.Replace,
				Path:  interactivestate.FormatPath([]string{"rival", xiuxianFieldHealth}),
				Value: 1.0,
			}},
			Choices: testTurnChoices(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 30*24*60*60, "cmd-npc-injured")
	records := npcGrowthTestRecords(t, store, storyID)
	for _, record := range records {
		if record.Candidate.ActorID != "rival" {
			continue
		}
		segment := record.Input.Interval.Segments[0]
		if segment.Activity != npcgrowth.ActivityInjuryPrevents {
			t.Fatalf("a character at one health must be on the injury row: %s", segment.Activity)
		}
		if record.Candidate.Gained != 0 {
			t.Fatalf("the injury row grants nothing: gained=%v", record.Candidate.Gained)
		}
		return
	}
	t.Fatal("the injured rival must still have a record marking the interval as settled")
}
