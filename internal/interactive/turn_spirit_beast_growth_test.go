package interactive

import (
	"math"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/npcgrowth"
)

// This file is the numeric acceptance for 灵兽成长 as it reaches ordinary play.
//
// 《自制-灵兽与御兽》§2 gives a creature its own species and parameters, and the
// growth rules price every tracked actor from its own stage, its own arrangement
// and its own body. What these tests add is that a spirit beast registered in the
// frozen pack catalog really grows on those rules and on nothing else:
//
//   - the 0.60 coefficient of 本能吐纳 enters the scale once, not twice;
//   - a healthy seclusion interval of 30 rule days is exactly 12×L progress at
//     refining layer L, and a split window pays the same as the whole one;
//   - fractions survive, no real time pays nothing, and one window is never paid
//     twice;
//   - the protagonist's own stage does not scale the creature;
//   - the creature stops at the real ceiling of its own method;
//   - the committed record, the stored state and a cold reopen agree, and a
//     sibling branch never inherits the interval.
//
// The arithmetic is asserted as the rules' own numbers rather than as a rate the
// settlement happens to pick, so a change in either the wiring or the rules has
// to show up here.

const (
	spiritBeastGrowthName      = "青风雀"
	spiritBeastGrowthSourceKey = "test_species_qingfeng_que_growth"
	// spiritBeastGrowthMethod is the registered species method 本能吐纳.
	spiritBeastGrowthMethod = "method.spirit_beast_breathing"
	// spiritBeastGrowthCoefficient is its GrowthCoefficient. The design's §一.4
	// fixes it at 0.60, and it must appear in the product exactly once.
	spiritBeastGrowthCoefficient = 0.60
	// spiritBeastGrowthDays is one rule month: the span the design's 12×L figure
	// is stated over.
	spiritBeastGrowthDays = 30 * 24 * 60 * 60
)

// spiritBeastGrowthStory builds a story on the real xiuxian preset whose cast
// holds one registered spirit beast. The creature arrives through the ordinary
// turn path — an Actor, a known person, and a catalog identity — so the growth
// settlement reads the same facts a real game would have committed.
func spiritBeastGrowthStory(t *testing.T, beastState map[string]any, protagonistStage cultivation.Cultivation) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(cultivation.ReferenceRules(),
				spec),
			storyContextStateTemplate(spec),
			importantCharacterStateTemplate(cultivation.ReferenceRules(),
				spec),
			opponentStateTemplate(cultivation.ReferenceRules(),
				spec),
			worldEntitiesStateTemplate(spec),
		},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{
				xiuxianFieldRealm: cultivation.StageKey(protagonistStage),
			}},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
			{ID: DefaultWorldEntitiesActorID, Name: "世界实体", TemplateID: ActorStateWorldEntitiesTemplateID},
		},
	}
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "灵兽成长", ActorState: &system,
		PackCatalog: &PackCatalogSnapshot{Characters: []PackCatalogPerson{{
			SourceKey: spiritBeastGrowthSourceKey, Name: spiritBeastGrowthName,
			Lifeform: PackCatalogLifeformSpiritBeast, CultivationMethodID: spiritBeastGrowthMethod,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入林", Narrative: "主角沿溪入林。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "在溪边停下",
		Narrative: "溪石上立着一只青风雀，正对着水面一吐一纳，灵气随它的呼吸散开又收拢。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Create, Path: "/" + spiritBeastGrowthName,
				Value: map[string]any{
					"template_id": ActorStateImportantCharacterTemplateID,
					"name":        spiritBeastGrowthName,
					"state":       beastState,
				},
			}},
			CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{
				CharacterID: spiritBeastGrowthName, Name: spiritBeastGrowthName,
				Role: "溪石上吐纳的灵雀", SourceKey: spiritBeastGrowthSourceKey,
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// spiritBeastGrowthSeclusionState is a creature that is spending its whole
// interval cultivating. Everything else about the interval — talent, ground,
// method grade, body — stays at the rules' baseline, so the only factor moving
// the rate is the species coefficient itself.
func spiritBeastGrowthSeclusionState(stage cultivation.Cultivation) map[string]any {
	return map[string]any{
		xiuxianFieldRealm:       cultivation.StageKey(stage),
		xiuxianFieldArrangement: xiuxianArrangementSeclusion,
	}
}

// spiritBeastGrowthRecords returns the creature's growth records on one branch.
func spiritBeastGrowthRecords(t *testing.T, store *Store, storyID, branchID string) []NPCGrowthRecordEvent {
	t.Helper()
	store.mu.Lock()
	_, records, err := store.readStoryRecentLocked(storyID, branchID)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	var beast []NPCGrowthRecordEvent
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeNPCGrowthRecord {
			continue
		}
		var event NPCGrowthRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Candidate.ActorID == spiritBeastGrowthName {
			beast = append(beast, event)
		}
	}
	return beast
}

// spiritBeastGrowthProgress reads the creature's committed 境界进度 on a branch.
func spiritBeastGrowthProgress(t *testing.T, store *Store, storyID, branchID string) float64 {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := actorStateNumber(actorStateFieldValue(snapshot.State, spiritBeastGrowthName, xiuxianFieldRealmProgress))
	return value
}

// spiritBeastGrowthSpend commits one interval of real time on the branch's
// formal clock. The rest is the protagonist's own activity; the creature's
// growth rides the same interval because the interval belongs to the branch.
func spiritBeastGrowthSpend(t *testing.T, store *Store, storyID, branchID string, seconds int64, commandID string) {
	t.Helper()
	if _, err := store.ApplySafeRecovery(storyID, SafeRecoveryRequest{
		BranchID: branchID, ActorID: DefaultActorID, Mode: cultivation.RecoverySafeMeditation,
		DurationSeconds: seconds, CommandID: commandID, CommandHash: commandID + "-hash",
	}); err != nil {
		t.Fatalf("spend %d seconds on %s: %v", seconds, branchID, err)
	}
}

// TestASpiritBeastEarnsTwelveTimesItsLayerInThirtyRuleDays is §一.4's headline
// number. A healthy creature in seclusion at refining layer L gains 12×L progress
// over one rule month, because the rules' own monthly rate 20×L is scaled by the
// 0.60 species coefficient exactly once.
func TestASpiritBeastEarnsTwelveTimesItsLayerInThirtyRuleDays(t *testing.T) {
	for layer := 1; layer <= 3; layer++ {
		stage := cultivation.Qi(layer)
		store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(stage), cultivation.Qi(6))
		spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "cmd-beast-30d")

		want := 12 * float64(layer)
		got := spiritBeastGrowthProgress(t, store, storyID, "main")
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("layer %d: 30 seclusion days must bank %v progress, got %v", layer, want, got)
		}

		records := spiritBeastGrowthRecords(t, store, storyID, "main")
		if len(records) != 1 {
			t.Fatalf("layer %d: one interval is one record, got %d", layer, len(records))
		}
		record := records[0]
		if record.Input.Interval.Segments[0].Activity != npcgrowth.ActivityFocusedSeclusion {
			t.Fatalf("layer %d: 专心闭关 must reach the rules as focused seclusion, got %s",
				layer, record.Input.Interval.Segments[0].Activity)
		}
		// The coefficient enters once. A second multiplication — reading the
		// species rate and the method grade's own 0.60 together — would show up as
		// a third of the expected gain, so the record is checked directly.
		if record.Input.Interval.Segments[0].Conditions.GrowthCoefficient != spiritBeastGrowthCoefficient {
			t.Fatalf("layer %d: species coefficient must be submitted once as %v, got %v",
				layer, spiritBeastGrowthCoefficient, record.Input.Interval.Segments[0].Conditions.GrowthCoefficient)
		}
		// The method's identity is the frozen catalog entry, not a made-up ref.
		if record.Input.Interval.Segments[0].Conditions.TechniqueRef == "" ||
			record.Input.Interval.Segments[0].Conditions.TechniqueRef == npcGrowthActorRef(spiritBeastGrowthName, "method") {
			t.Fatalf("layer %d: the species method must be sourced from the pack catalog, got %q",
				layer, record.Input.Interval.Segments[0].Conditions.TechniqueRef)
		}
		if record.Candidate.Gained != got {
			t.Fatalf("layer %d: the record and the stored state must agree: record=%v state=%v", layer, record.Candidate.Gained, got)
		}
	}
}

// TestASpiritBeastSplitWindowPaysTheSameAsTheWholeOne is §10's GROW-02 for the
// creature: nothing changed between the two halves, so 15+15 must equal 30 and
// the fraction must not be lost on the way.
func TestASpiritBeastSplitWindowPaysTheSameAsTheWholeOne(t *testing.T) {
	stage := cultivation.Qi(2)
	whole, wholeStory := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(stage), cultivation.Qi(6))
	spiritBeastGrowthSpend(t, whole, wholeStory, "main", spiritBeastGrowthDays, "cmd-beast-whole")

	split, splitStory := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(stage), cultivation.Qi(6))
	half := int64(15 * 24 * 60 * 60)
	spiritBeastGrowthSpend(t, split, splitStory, "main", half, "cmd-beast-half-one")
	spiritBeastGrowthSpend(t, split, splitStory, "main", half, "cmd-beast-half-two")

	wholeValue := spiritBeastGrowthProgress(t, whole, wholeStory, "main")
	splitValue := spiritBeastGrowthProgress(t, split, splitStory, "main")
	if splitValue != wholeValue {
		t.Fatalf("15+15 must pay the same as 30 for a creature: whole=%v split=%v", wholeValue, splitValue)
	}
	if math.Abs(wholeValue-24) > 1e-9 {
		t.Fatalf("layer 2 over one rule month must bank 24 progress, got %v", wholeValue)
	}

	// A span that does not divide into whole progress keeps its fraction: seven
	// days at layer 1 is 0.4 per day.
	fractional, fractionalStory := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(1)), cultivation.Qi(6))
	week := int64(7 * 24 * 60 * 60)
	spiritBeastGrowthSpend(t, fractional, fractionalStory, "main", week, "cmd-beast-week-one")
	if got := spiritBeastGrowthProgress(t, fractional, fractionalStory, "main"); math.Abs(got-2.8) > 1e-9 {
		t.Fatalf("seven seclusion days at layer 1 must bank 2.8 progress, got %v", got)
	}
	spiritBeastGrowthSpend(t, fractional, fractionalStory, "main", week, "cmd-beast-week-two")
	if got := spiritBeastGrowthProgress(t, fractional, fractionalStory, "main"); math.Abs(got-5.6) > 1e-9 {
		t.Fatalf("the earlier fraction must carry into the next week: want 5.6, got %v", got)
	}
}

// TestASpiritBeastDoesNotGrowWithoutRealTime is the other half of §一.4's time
// requirement: a turn that changes nothing about the clock is not an interval,
// and a creature cannot bank progress from narration.
func TestASpiritBeastDoesNotGrowWithoutRealTime(t *testing.T) {
	store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(6))
	before := spiritBeastGrowthProgress(t, store, storyID, "main")
	if before != 0 {
		t.Fatalf("a creature that has spent no interval starts at zero, got %v", before)
	}
	for index, user := range []string{"再看一会儿", "记下它的吐纳节奏", "退开几步"} {
		if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
			BranchID: "main", User: user, Narrative: "它仍对着水面吐纳，天色未变。",
			TurnResult: &TurnResult{Choices: testTurnChoices()},
		}); err != nil {
			t.Fatalf("turn %d: %v", index, err)
		}
	}
	if got := spiritBeastGrowthProgress(t, store, storyID, "main"); got != before {
		t.Fatalf("a turn that spends no formal time must not grow the creature: %v -> %v", before, got)
	}
	if records := spiritBeastGrowthRecords(t, store, storyID, "main"); len(records) != 0 {
		t.Fatalf("no interval was spent, so no settlement may be committed: %+v", records)
	}
}

// TestASpiritBeastWindowIsNeverPaidTwice is §9's retry rule read against the
// creature: the same window presents one identity and the rules answer 重复.
func TestASpiritBeastWindowIsNeverPaidTwice(t *testing.T) {
	store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(6))
	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "cmd-beast-once")

	records := spiritBeastGrowthRecords(t, store, storyID, "main")
	if len(records) != 1 {
		t.Fatalf("the first interval must settle exactly once, got %d", len(records))
	}
	settled := spiritBeastGrowthProgress(t, store, storyID, "main")

	store.mu.Lock()
	projection, err := store.storyBranchProjectionLocked(storyID, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	system := npcGrowthTestSystem(t, store, storyID)
	events, _ := npcGrowthEvents(projection, system, "main", projection.Head, "now", "", "",
		records[0].ClockBefore, records[0].ClockAfter)
	for _, event := range events {
		if replayed, ok := event.(NPCGrowthRecordEvent); ok && replayed.Candidate.ActorID == spiritBeastGrowthName {
			t.Fatalf("the same window must not be settled a second time: %+v", replayed.Candidate)
		}
	}
	if got := spiritBeastGrowthProgress(t, store, storyID, "main"); got != settled {
		t.Fatalf("a duplicate pass must not move the creature's position: %v -> %v", settled, got)
	}
}

// TestTheProtagonistsStageDoesNotScaleASpiritBeast is §2's 不以主人等级自动缩放.
// Two stories differ only in where the protagonist stands; the creature's own
// interval must pay the same either way.
func TestTheProtagonistsStageDoesNotScaleASpiritBeast(t *testing.T) {
	low, lowStory := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(1))
	high, highStory := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(13))
	spiritBeastGrowthSpend(t, low, lowStory, "main", spiritBeastGrowthDays, "cmd-beast-master-low")
	spiritBeastGrowthSpend(t, high, highStory, "main", spiritBeastGrowthDays, "cmd-beast-master-high")

	lowValue := spiritBeastGrowthProgress(t, low, lowStory, "main")
	highValue := spiritBeastGrowthProgress(t, high, highStory, "main")
	if lowValue != highValue {
		t.Fatalf("the protagonist's stage must not change the creature's growth: qi1=%v qi13=%v", lowValue, highValue)
	}
	if math.Abs(lowValue-24) > 1e-9 {
		t.Fatalf("the creature's own rate is the only rate: want 24, got %v", lowValue)
	}
}

// TestASpiritBeastGrowsOnlyAsFarAsItsOwnMethodReaches is §6's 没有合法突破计划时
// 停在瓶颈, read against the registered species method.
//
// 本能吐纳 supports growth through foundation late. A creature at the top
// refining layer therefore makes its foundation attempt, and a creature already
// at foundation late has nothing further it may attempt: the interval stops at
// the real bottleneck instead of rolling into a realm its method never reaches.
func TestASpiritBeastGrowsOnlyAsFarAsItsOwnMethodReaches(t *testing.T) {
	topRefining := cultivation.Qi(cultivation.QiLayers)
	required, _, ok := cultivation.ProgressRequired(topRefining)
	if !ok {
		t.Fatal("the top refining layer must accumulate progress")
	}
	atThreshold := spiritBeastGrowthSeclusionState(topRefining)
	atThreshold[xiuxianFieldRealmProgress] = required

	store, storyID := spiritBeastGrowthStory(t, atThreshold, cultivation.Qi(6))
	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "cmd-beast-foundation")

	records := spiritBeastGrowthRecords(t, store, storyID, "main")
	if len(records) == 0 {
		t.Fatal("a creature at its threshold with a supporting method must make the attempt")
	}
	attempted := false
	for _, record := range records {
		if record.Input.BreakthroughIntent != nil {
			attempted = true
			if record.Input.BreakthroughIntent.Target != cultivation.BreakthroughFoundation {
				t.Fatalf("the attempt must target the realm the method supports, got %s", record.Input.BreakthroughIntent.Target)
			}
		}
	}
	if !attempted {
		t.Fatalf("the interval must carry the sourced attempt, records=%s", npcGrowthTestDescribe(records))
	}

	// The same creature one realm higher has no legal next step.
	foundationLate := cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageLate}
	ceilingRequired, _, ok := cultivation.ProgressRequired(foundationLate)
	if !ok {
		t.Fatal("foundation late must accumulate progress")
	}
	atCeiling := spiritBeastGrowthSeclusionState(foundationLate)
	atCeiling[xiuxianFieldRealmProgress] = ceilingRequired
	capped, cappedStory := spiritBeastGrowthStory(t, atCeiling, cultivation.Qi(6))
	spiritBeastGrowthSpend(t, capped, cappedStory, "main", spiritBeastGrowthDays, "cmd-beast-ceiling")

	for _, record := range spiritBeastGrowthRecords(t, capped, cappedStory, "main") {
		if record.Input.BreakthroughIntent != nil {
			t.Fatalf("本能吐纳 supports only foundation late; a core attempt must not be invented: %+v", record.Input.BreakthroughIntent)
		}
	}
	if got := spiritBeastGrowthProgress(t, capped, cappedStory, "main"); math.Abs(got-ceilingRequired) > 1e-9 {
		t.Fatalf("a creature at its method's ceiling keeps its banked threshold: want %v, got %v", ceilingRequired, got)
	}
	// And the ceiling is the method's own number, not a constant in the wiring.
	store.mu.Lock()
	projection, err := capped.storyBranchProjectionLocked(cappedStory, "main")
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	method := npcGrowthSpeciesMethod(projection, spiritBeastGrowthName)
	if method == nil || method.SupportsThrough != "foundation_late" {
		t.Fatalf("the species method's ceiling must be foundation_late, got %+v", method)
	}
	if npcGrowthSupportsTarget(cultivation.ReferenceRules(),
		projection.State, spiritBeastGrowthName, cultivation.BreakthroughCore, method) {
		t.Fatal("本能吐纳 must not be read as supporting the core realm")
	}
}

// TestASpiritBeastRecordMatchesTheSnapshotAndSurvivesReopen keeps the committed
// record, the live state and a cold reopen on one number.
func TestASpiritBeastRecordMatchesTheSnapshotAndSurvivesReopen(t *testing.T) {
	store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(6))
	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "cmd-beast-reopen")

	records := spiritBeastGrowthRecords(t, store, storyID, "main")
	if len(records) != 1 {
		t.Fatalf("one interval is one record, got %d", len(records))
	}
	stored := spiritBeastGrowthProgress(t, store, storyID, "main")
	if records[0].Candidate.NextState.Accumulated != stored {
		t.Fatalf("the record and the snapshot must agree: record=%v snapshot=%v",
			records[0].Candidate.NextState.Accumulated, stored)
	}
	if got := records[0].Candidate.NextState.Cultivation; cultivation.StageKey(got) != cultivation.StageKey(cultivation.Qi(2)) {
		t.Fatalf("24 progress at layer 2 must not advance the stage: got %s", cultivation.StageKey(got))
	}

	reopened := NewStore(store.root)
	defer reopened.Close()
	if got := spiritBeastGrowthProgress(t, reopened, storyID, "main"); got != stored {
		t.Fatalf("a cold reopen must read the settled progress: %v -> %v", stored, got)
	}
	reopenedRecords := spiritBeastGrowthRecords(t, reopened, storyID, "main")
	if len(reopenedRecords) != 1 || reopenedRecords[0].Candidate.NextState.Accumulated != stored {
		t.Fatalf("a cold reopen must read the committed settlement: %+v", reopenedRecords)
	}
}

// TestASiblingBranchDoesNotInheritASpiritBeastsInterval is the branch half of
// §9: growth belongs to the branch that spent the time. A fork cut before the
// interval starts where the fork started, and pays its own window separately.
func TestASiblingBranchDoesNotInheritASpiritBeastsInterval(t *testing.T) {
	store, storyID := spiritBeastGrowthStory(t, spiritBeastGrowthSeclusionState(cultivation.Qi(2)), cultivation.Qi(6))
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	beforeInterval := snapshot.CurrentTurn.ID

	spiritBeastGrowthSpend(t, store, storyID, "main", spiritBeastGrowthDays, "cmd-beast-main-month")
	if got := spiritBeastGrowthProgress(t, store, storyID, "main"); math.Abs(got-24) > 1e-9 {
		t.Fatalf("main spent one rule month: want 24, got %v", got)
	}

	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: beforeInterval, Title: "未闭关"})
	if err != nil {
		t.Fatal(err)
	}
	if got := spiritBeastGrowthProgress(t, store, storyID, fork.ID); got != 0 {
		t.Fatalf("a fork cut before the interval must not inherit it, got %v", got)
	}
	if records := spiritBeastGrowthRecords(t, store, storyID, fork.ID); len(records) != 0 {
		t.Fatalf("a fork cut before the interval has no settlement of its own: %+v", records)
	}

	half := int64(15 * 24 * 60 * 60)
	spiritBeastGrowthSpend(t, store, storyID, fork.ID, half, "cmd-beast-fork-half")
	if got := spiritBeastGrowthProgress(t, store, storyID, fork.ID); math.Abs(got-12) > 1e-9 {
		t.Fatalf("the fork pays its own half month: want 12, got %v", got)
	}
	if got := spiritBeastGrowthProgress(t, store, storyID, "main"); math.Abs(got-24) > 1e-9 {
		t.Fatalf("the fork's interval must not move main: want 24, got %v", got)
	}
}
