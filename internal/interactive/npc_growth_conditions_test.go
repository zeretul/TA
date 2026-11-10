package interactive

import (
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
	"ta/internal/npcgrowth"
)

// These tests are §2 and §4 of 《自制-NPC自主成长》 as they reach ordinary play:
// an NPC is priced from its own ground and its own arrangement, never from where
// the protagonist happens to be standing and never from a rate the settlement
// picked for it.
//
// The rules package already proves the arithmetic behind every row of the
// allocation table. What these tests add is that the wiring submits the row the
// NPC's own committed facts support, and submits nothing it cannot source.
//
// The field identities and option wordings below are written as literals on
// purpose: they are the wire contract the model and the story actually write, so
// a rename in Go must not be able to change what the tests consider correct.

const (
	testArrangementField      = "修行安排"
	testTrainingWindowField   = "修炼间隙"
	testArrangementUnstated   = "未定"
	testArrangementOrdinary   = "正常生活兼修行"
	testArrangementSeclusion  = "专心闭关"
	testArrangementTravel     = "外出历练"
	testArrangementOccupied   = "事务繁忙"
	testArrangementRecovering = "养伤"
)

// npcGrowthConditionStory builds a story whose scene sits at the given location,
// whose 在场角色 list is the given set, and whose cast holds one tracked
// important character seeded with the given state.
//
// The real xiuxian preset is used rather than the combat fixture: the
// arrangement field is declared on the important-character template, and the
// scene fields on the story-context template, so a hand-built fixture would not
// carry the schema the application really freezes.
func npcGrowthConditionStory(t *testing.T, sceneLocation string, present []string, rivalState map[string]any) (*Store, string) {
	t.Helper()
	system := xiuxianTestSystem()
	for index := range system.InitialActors {
		if system.InitialActors[index].ID == DefaultActorID {
			// The interval is spent by the protagonist resting, so they need a
			// stage the recovery rules can price pools from.
			if system.InitialActors[index].State == nil {
				system.InitialActors[index].State = map[string]any{}
			}
			system.InitialActors[index].State[xiuxianFieldRealm] = cultivation.StageKey(cultivation.Qi(6))
			continue
		}
		if system.InitialActors[index].ID != DefaultStoryContextActorID {
			continue
		}
		if system.InitialActors[index].State == nil {
			system.InitialActors[index].State = map[string]any{}
		}
		if sceneLocation != "" {
			system.InitialActors[index].State[storySceneLocationField] = sceneLocation
		}
		if len(present) > 0 {
			items := make([]any, 0, len(present))
			for _, actorID := range present {
				items = append(items, actorID)
			}
			system.InitialActors[index].State[storyScenePresentField] = items
		}
	}
	state := map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6))}
	for key, value := range rivalState {
		state[key] = value
	}
	system.InitialActors = append(system.InitialActors, ActorStateInitialActor{
		ID: "rival", Name: "同门", TemplateID: ActorStateImportantCharacterTemplateID, State: state,
	})
	system = normalizeActorStateSystem(system)

	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "同门修行", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角停下脚步，同门各自过着日子。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// npcGrowthConditionRival spends one interval and returns the rival's committed
// progress and the one growth record the interval produced.
func npcGrowthConditionRival(t *testing.T, sceneLocation string, present []string, rivalState map[string]any, seconds int64, commandID string) (float64, *NPCGrowthRecordEvent) {
	t.Helper()
	store, storyID := npcGrowthConditionStory(t, sceneLocation, present, rivalState)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, seconds, commandID)
	records := npcGrowthTestRivalRecords(t, store, storyID)
	if len(records) != 1 {
		t.Fatalf("one ordinary interval must settle the rival exactly once; records=%s", npcGrowthTestDescribe(records))
	}
	record := records[0]
	return npcGrowthTestProgress(t, store, storyID, "rival"), &record
}

// TestADistantNPCTrainsOnItsOwnGround is the acceptance clause "固定远处NPC自己的
// 条件，只改变主角在普通地／灵地的位置，其成长相同".
//
// The rival is not in 在场角色, so the scene's ground is not its ground. Before
// this wiring the settlement read 当前详细地点 for every tracked actor, and the
// protagonist's cave dwelling lifted a distant NPC's rate.
func TestADistantNPCTrainsOnItsOwnGround(t *testing.T) {
	const interval = 30 * 24 * 60 * 60
	plain, plainRecord := npcGrowthConditionRival(t, "", nil, nil, interval, "cmd-own-ground-plain")
	spirit, spiritRecord := npcGrowthConditionRival(t, "越国灵脉洞府", nil, nil, interval, "cmd-own-ground-spirit")

	if plain <= 0 {
		t.Fatalf("an ordinary interval must still grow the NPC: %v", plain)
	}
	if plain != spirit {
		t.Fatalf("a distant NPC's growth must not follow the protagonist's ground: plain=%v spirit=%v", plain, spirit)
	}
	for name, record := range map[string]*NPCGrowthRecordEvent{"plain": plainRecord, "spirit": spiritRecord} {
		if got := record.Input.Interval.Segments[0].Conditions.Environment; got != string(cultivation.EnvironmentOrdinary) {
			t.Fatalf("%s: an NPC whose own place is unknown is settled on ordinary ground, not %s", name, got)
		}
	}
}

// TestANPCPresentInTheSceneSharesItsGround is the other half of the same rule:
// §三 says "明确同场者可用该场景条件". Someone the scene really lists as present
// is standing where the protagonist stands.
func TestANPCPresentInTheSceneSharesItsGround(t *testing.T) {
	const interval = 30 * 24 * 60 * 60
	distant, distantRecord := npcGrowthConditionRival(t, "越国灵脉洞府", nil, nil, interval, "cmd-present-distant")
	present, presentRecord := npcGrowthConditionRival(t, "越国灵脉洞府", []string{"rival"}, nil, interval, "cmd-present-together")

	if got := distantRecord.Input.Interval.Segments[0].Conditions.Environment; got != string(cultivation.EnvironmentOrdinary) {
		t.Fatalf("an absent NPC does not inherit the scene: %s", got)
	}
	if got := presentRecord.Input.Interval.Segments[0].Conditions.Environment; got != string(cultivation.EnvironmentStable) {
		t.Fatalf("a listed present NPC trains where the scene stands: %s", got)
	}
	if present <= distant {
		t.Fatalf("the same interval on a spiritual site must outgrow ordinary ground: present=%v distant=%v", present, distant)
	}
}

// TestAnNPCFollowsItsOwnArrangement is §4's allocation table read from the
// character's own committed arrangement: 50% for ordinary life, 100% for
// seclusion, 20% for an outing with a real safe window, and 0% for an
// arrangement that leaves no training time at all.
func TestAnNPCFollowsItsOwnArrangement(t *testing.T) {
	const interval = 30 * 24 * 60 * 60
	ordinary, ordinaryRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementOrdinary,
	}, interval, "cmd-arrangement-ordinary")
	seclusion, seclusionRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementSeclusion,
	}, interval, "cmd-arrangement-seclusion")
	travel, travelRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField:    testArrangementTravel,
		testTrainingWindowField: "每夜在废弃矿洞外调息两个时辰，无人打扰。",
	}, interval, "cmd-arrangement-travel")
	busy, busyRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementOccupied,
	}, interval, "cmd-arrangement-busy")
	recovering, recoveringRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementRecovering,
	}, interval, "cmd-arrangement-recovering")

	for name, want := range map[string]struct {
		record   *NPCGrowthRecordEvent
		activity npcgrowth.Activity
	}{
		"ordinary":   {ordinaryRecord, npcgrowth.ActivityOrdinaryTraining},
		"seclusion":  {seclusionRecord, npcgrowth.ActivityFocusedSeclusion},
		"travel":     {travelRecord, npcgrowth.ActivityTravelOpportunity},
		"busy":       {busyRecord, npcgrowth.ActivityOccupied},
		"recovering": {recoveringRecord, npcgrowth.ActivityInjuryPrevents},
	} {
		if got := want.record.Input.Interval.Segments[0].Activity; got != want.activity {
			t.Fatalf("%s: the arrangement must reach the rules as %s, got %s", name, want.activity, got)
		}
	}
	if got := travelRecord.Input.Interval.Segments[0].OpportunitySourceRef; got == "" {
		t.Fatal("an outing earns training time only with a sourced opportunity, and the window must be that source")
	}
	// The rows are shares of effective training time, not a second rate table.
	if seclusion < ordinary*1.99 || seclusion > ordinary*2.01 {
		t.Fatalf("seclusion is 100%% of the interval and ordinary life 50%%: ordinary=%v seclusion=%v", ordinary, seclusion)
	}
	if travel < ordinary*0.39 || travel > ordinary*0.41 {
		t.Fatalf("an outing with a safe window is 20%% against ordinary life's 50%%: ordinary=%v travel=%v", ordinary, travel)
	}
	if busy != 0 || busyRecord.Candidate.Gained != 0 {
		t.Fatalf("an arrangement that fills the whole interval grants nothing: progress=%v gained=%v", busy, busyRecord.Candidate.Gained)
	}
	if recovering != 0 || recoveringRecord.Candidate.Gained != 0 {
		t.Fatalf("recovering grants no training on top of the formal recovery rules: progress=%v gained=%v", recovering, recoveringRecord.Candidate.Gained)
	}
}

// TestAnOutingWithoutARealWindowIsNotPaid is the second half of §三's 20% row:
// the rate needs an established opportunity, and an arrangement that merely says
// "out travelling" does not by itself create one.
func TestAnOutingWithoutARealWindowIsNotPaid(t *testing.T) {
	const interval = 30 * 24 * 60 * 60
	_, record := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementTravel,
	}, interval, "cmd-arrangement-travel-bare")
	segment := record.Input.Interval.Segments[0]
	if segment.Activity != npcgrowth.ActivityTravelNoWindow {
		t.Fatalf("an outing with no established window trains not at all, got %s", segment.Activity)
	}
	if record.Candidate.Gained != 0 {
		t.Fatalf("an unsourced opportunity must not be paid: gained=%v", record.Candidate.Gained)
	}
}

// TestAnUnstatedArrangementFallsBackToOrdinaryLife is §三's conservative
// default: a character with no recorded arrangement and no contrary fact is
// living their life, not secluded.
func TestAnUnstatedArrangementFallsBackToOrdinaryLife(t *testing.T) {
	const interval = 30 * 24 * 60 * 60
	unstated, record := npcGrowthConditionRival(t, "", nil, nil, interval, "cmd-arrangement-unstated")
	stated, _ := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementOrdinary,
	}, interval, "cmd-arrangement-stated")
	if record.Input.Interval.Segments[0].Activity != npcgrowth.ActivityOrdinaryTraining {
		t.Fatalf("an unstated arrangement is ordinary life: %s", record.Input.Interval.Segments[0].Activity)
	}
	if unstated != stated {
		t.Fatalf("an unstated arrangement must price the same as the ordinary row: unstated=%v stated=%v", unstated, stated)
	}
}

// TestTheSameArrangementPaysTheSameAcrossALongOrSplitWindow is §10's GROW-02:
// when nothing changes, one long interval and two shorter ones must agree.
func TestTheSameArrangementPaysTheSameAcrossALongOrSplitWindow(t *testing.T) {
	arrangement := map[string]any{testArrangementField: testArrangementSeclusion}
	whole, _ := npcGrowthConditionRival(t, "", nil, arrangement, 30*24*60*60, "cmd-split-whole")

	store, storyID := npcGrowthConditionStory(t, "", nil, arrangement)
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 15*24*60*60, "cmd-split-first")
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, 15*24*60*60, "cmd-split-second")
	split := npcGrowthTestProgress(t, store, storyID, "rival")

	if split != whole {
		t.Fatalf("one window and two halves of the same arrangement must pay the same: whole=%v split=%v", whole, split)
	}
}

// TestAnArrangementWrittenByATurnReachesTheSettlement is the end-to-end leg: the
// arrangement is a fact the story writes through its ordinary turn path, not a
// fixture the test plants directly into state. A field the model could never
// write would be a dead letter.
func TestAnArrangementWrittenByATurnReachesTheSettlement(t *testing.T) {
	const day = 24 * 60 * 60
	store, storyID := npcGrowthConditionStory(t, "", nil, nil)

	before := npcGrowthTestProgress(t, store, storyID, "rival")
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "同门说要闭一次关",
		Narrative: "同门收拾了洞府，说这一阵子不再见客。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op:    interactivestate.Replace,
				Path:  interactivestate.FormatPath([]string{"rival", xiuxianFieldArrangement}),
				Value: xiuxianArrangementSeclusion,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, day, "cmd-turn-arrangement")
	records := npcGrowthTestRivalRecords(t, store, storyID)
	if len(records) == 0 {
		t.Fatal("the interval must settle the rival")
	}
	last := records[len(records)-1]
	if got := last.Input.Interval.Segments[0].Activity; got != npcgrowth.ActivityFocusedSeclusion {
		t.Fatalf("an arrangement written by a turn must reach the settlement, got %s", got)
	}
	if npcGrowthTestProgress(t, store, storyID, "rival") <= before {
		t.Fatal("a day of seclusion must still grow the rival")
	}
}

// TestATurnThatBothRecordsAndSpendsUsesTheCommittedArrangement pins the ordering
// rule the settlement depends on: §三 says to reuse the "已提交" facts, and a
// turn's own write is not committed when its interval settles.
//
// So a turn that records a retreat and spends that same turn's days is priced on
// the arrangement that stood when the turn began — ordinary life here — and the
// retreat starts paying from the next interval. The alternative, reading the
// turn's in-flight writes, would let a single sentence about a retreat reach back
// over the days the turn had already spent, which is the retroactive repricing
// §三 forbids.
func TestATurnThatBothRecordsAndSpendsUsesTheCommittedArrangement(t *testing.T) {
	const day = 24 * 60 * 60
	store, storyID := npcGrowthConditionStory(t, "", nil, map[string]any{
		testArrangementField: testArrangementOrdinary,
	})

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "同门说要闭一次关",
		Narrative: "同门收拾了洞府，说这一阵子不再见客。",
		TurnResult: &TurnResult{
			Choices:     testTurnChoices(),
			Cultivation: &TurnCultivationInput{Days: 1},
			StateUpdates: []interactivestate.Update{{
				Op:    interactivestate.Replace,
				Path:  interactivestate.FormatPath([]string{"rival", xiuxianFieldArrangement}),
				Value: xiuxianArrangementSeclusion,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	records := npcGrowthTestRivalRecords(t, store, storyID)
	if len(records) != 1 {
		t.Fatalf("the turn's interval settles the rival once; records=%s", npcGrowthTestDescribe(records))
	}
	if got := records[0].Input.Interval.Segments[0].Activity; got != npcgrowth.ActivityOrdinaryTraining {
		t.Fatalf("the interval that was already under way is priced on the committed arrangement, got %s", got)
	}

	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, day, "cmd-same-turn-after")
	records = npcGrowthTestRivalRecords(t, store, storyID)
	last := records[len(records)-1]
	if got := last.Input.Interval.Segments[0].Activity; got != npcgrowth.ActivityFocusedSeclusion {
		t.Fatalf("the retreat recorded last turn pays from the next interval, got %s", got)
	}
}

// TestAnArrangementChangeDoesNotRepriceTheEarlierSpan is §三's "变更发生前的区间
// 不得按新条件倒算".
//
// The rival trains for a day on ordinary life, then the story records a retreat,
// then a second day is spent. The first day is already a committed record and
// must not be re-settled at the seclusion rate; the two records together must
// equal what each arrangement really earned on its own span.
func TestAnArrangementChangeDoesNotRepriceTheEarlierSpan(t *testing.T) {
	const day = 24 * 60 * 60
	ordinaryOnly, ordinaryRecord := npcGrowthConditionRival(t, "", nil, map[string]any{
		testArrangementField: testArrangementOrdinary,
	}, day, "cmd-reprice-ordinary")

	store, storyID := npcGrowthConditionStory(t, "", nil, map[string]any{
		testArrangementField: testArrangementOrdinary,
	})
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, day, "cmd-reprice-first")
	firstDay := npcGrowthTestProgress(t, store, storyID, "rival")
	if firstDay != ordinaryOnly {
		t.Fatalf("the first day is ordinary life: got %v want %v", firstDay, ordinaryOnly)
	}

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "同门开始闭关",
		Narrative: "同门关上了洞府石门。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []interactivestate.Update{{
				Op:    interactivestate.Replace,
				Path:  interactivestate.FormatPath([]string{"rival", xiuxianFieldArrangement}),
				Value: xiuxianArrangementSeclusion,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	recoveryTestApply(t, store, storyID, cultivation.RecoverySafeMeditation, day, "cmd-reprice-second")

	records := npcGrowthTestRivalRecords(t, store, storyID)
	if len(records) != 2 {
		t.Fatalf("each span is its own committed interval; records=%s", npcGrowthTestDescribe(records))
	}
	// The earlier record is untouched, so its activity still reads ordinary life.
	if got := records[0].Input.Interval.Segments[0].Activity; got != npcgrowth.ActivityOrdinaryTraining {
		t.Fatalf("the span before the change must not be repriced, got %s", got)
	}
	if got := records[1].Input.Interval.Segments[0].Activity; got != npcgrowth.ActivityFocusedSeclusion {
		t.Fatalf("the span after the change is seclusion, got %s", got)
	}
	// Two days at each arrangement's own rate is the whole gain; a retroactive
	// repricing would pay the first day a second time at double rate.
	total := npcGrowthTestProgress(t, store, storyID, "rival")
	want := ordinaryRecord.Candidate.Gained + records[1].Candidate.Gained
	if got := records[0].Candidate.Gained + records[1].Candidate.Gained; got != want {
		t.Fatalf("the committed gains must be the two spans' own rates: got %v want %v", got, want)
	}
	if total != want {
		t.Fatalf("the stored progress must equal the two committed gains: got %v want %v", total, want)
	}
}
