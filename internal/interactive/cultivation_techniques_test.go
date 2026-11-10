package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// xiuxianTechniqueTestSystem is the settlement fixture plus the method record,
// because the method rules write into a field the panel fixture does not
// declare.
func xiuxianTechniqueTestSystem() StoryDirectorActorStateSystem {
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianTechniquesField())
	return StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Name: "主角", Fields: fields,
	}}}
}

// masteredTechniqueState writes a method record directly, which is the state a
// character is in after an earlier turn's learning completed.
func masteredTechniqueState(t *testing.T, stage string, methods ...string) map[string]any {
	t.Helper()
	record := map[string]any{}
	for _, id := range methods {
		definition, ok := cultivation.TechniqueByID(id)
		if !ok {
			t.Fatalf("%s is not a published method", id)
		}
		record[definition.Name] = map[string]any{
			"定义":   definition.ID,
			"版本":   float64(definition.Version),
			"掌握程度": "已掌握",
			"运转":   "停止",
		}
	}
	return xiuxianTestState(t, stage, map[string]any{xiuxianFieldTechniques: record})
}

// TestAMethodIsLearnedOnlyByRealElapsedStudy is the core refusal of §4.5: prose
// cannot grant a method. A first interval of study makes progress but does not
// master it; only the accumulated time reaching the standard does.
func TestAMethodIsLearnedOnlyByRealElapsedStudy(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(3)), nil)

	// 温元诀 needs 14400 minutes at normal aptitude. Half of that is half the
	// method, not the whole of it.
	half := TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 7200}}}
	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist", half, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared field must study")
	}
	if len(result.Learned) != 0 {
		t.Fatalf("half the time mastered the method: %+v", result.Learned)
	}
	if len(result.Studied) != 1 || result.Studied[0].Mastered {
		t.Fatalf("studied = %+v, want one unfinished method", result.Studied)
	}
	if progress := result.Studied[0].Progress; progress < 0.49 || progress > 0.51 {
		t.Fatalf("progress = %v, want about 0.5", progress)
	}

	applied := appliedXiuxianState(t, state, ops)
	if circulating := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, applied)); len(circulating) != 0 {
		t.Fatalf("an unmastered method is circulating: %+v", circulating)
	}

	// The second interval completes it, because progress accumulates across
	// turns rather than resetting.
	rest := TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 7200}}}
	secondOps, secondResult, ok := xiuxianTechniqueOps(applied, system, "protagonist", rest, "ev2")
	if !ok {
		t.Fatal("the second interval must apply")
	}
	if len(secondResult.Learned) != 1 || secondResult.Learned[0].ID != "method.warm_origin" {
		t.Fatalf("learned = %+v, want 温元诀 mastered", secondResult.Learned)
	}
	final := appliedXiuxianState(t, applied, secondOps)
	if circulating := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, final)); len(circulating) != 0 {
		t.Fatal("mastering a method must not start it circulating on its own")
	}
}

func mustReadTechniques(t *testing.T, state map[string]any) map[string]techniqueRecord {
	t.Helper()
	records, ok := readTechniqueRecords(state, "protagonist")
	if !ok {
		t.Fatal("the method record must exist after a write")
	}
	return records
}

// TestAMethodKeepsItsWordingAndItsNumbersApart is §5's journal half for 功法: the
// record carries what the method feels like and where it came from, written once,
// and neither is ever priced.
func TestAMethodKeepsItsWordingAndItsNumbersApart(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(3)), nil)

	first := TurnTechniqueInput{Learn: []TurnTechniqueLearning{{
		DefID:       "method.warm_origin",
		Minutes:     7200,
		Description: "练到后来胸口像揣了个小炉子，冬天出门不用缩脖子。",
		Source:      "祖师堂里那面刻满字的墙，抄了三年才抄全。",
	}}}
	ops, _, ok := xiuxianTechniqueOps(state, system, "protagonist", first, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared field must study")
	}
	applied := appliedXiuxianState(t, state, ops)
	records := mustReadTechniques(t, applied)
	record, ok := records["温元诀"]
	if !ok {
		t.Fatalf("records = %+v, want 温元诀", records)
	}
	if !strings.Contains(record.Description, "小炉子") {
		t.Errorf("recorded description = %q, want the sentence the study supplied", record.Description)
	}
	if !strings.Contains(record.Source, "祖师堂") {
		t.Errorf("recorded source = %q, want the sentence the study supplied", record.Source)
	}

	// The second interval keeps them; a later turn cannot restyle a method the
	// player already read about.
	rest := TurnTechniqueInput{Learn: []TurnTechniqueLearning{{
		DefID: "method.warm_origin", Minutes: 7200,
		Description: "浑身发热。", Source: "不知道哪来的。",
	}}}
	secondOps, _, _ := xiuxianTechniqueOps(applied, system, "protagonist", rest, "ev2")
	final := appliedXiuxianState(t, applied, secondOps)
	finalRecord := mustReadTechniques(t, final)["温元诀"]
	if !strings.Contains(finalRecord.Description, "小炉子") {
		t.Errorf("description after the second interval = %q, want the first wording kept", finalRecord.Description)
	}
	if !strings.Contains(finalRecord.Source, "祖师堂") {
		t.Errorf("source after the second interval = %q, want the first wording kept", finalRecord.Source)
	}
}

// TestAMethodBelowItsMinimumStageIsRefused keeps the stage requirement real: a
// character cannot study 温元诀 at 炼气二层, however the prose describes it.
func TestAMethodBelowItsMinimumStageIsRefused(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(2)), nil)

	input := TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Minutes: 43200}}}
	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must still be processed so the refusal is reported")
	}
	if len(result.Learned) != 0 {
		t.Fatalf("a method was learned below its minimum stage: %+v", result.Learned)
	}
	if len(result.Refused) != 1 || result.Refused[0].Action != "learn" {
		t.Fatalf("refused = %+v, want one learn refusal", result.Refused)
	}
	if len(ops) != 0 {
		t.Fatalf("a refused study wrote state: %+v", ops)
	}
}

// TestACirculatingMethodChangesTheSettledNumbers is the end-to-end point: a
// method that is circulating is priced by the settlement, and the same character
// without it is priced differently.
func TestACirculatingMethodChangesTheSettledNumbers(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	stage := cultivation.StageKey(cultivation.Qi(6))

	plain := xiuxianTestState(t, stage, nil)
	baselineOps, ok := settleXiuxianActorState(plain, system, "protagonist", "ev0")
	if !ok {
		t.Fatal("the baseline must settle")
	}
	baseline := appliedXiuxianState(t, plain, baselineOps)
	baseAttack := xiuxianStoredInt(baseline, "protagonist", xiuxianFieldAttack)
	baseHealth := xiuxianStoredInt(baseline, "protagonist", xiuxianFieldHealthMax)

	// 温元诀 is +10% attack and −5% health maximum.
	state := masteredTechniqueState(t, stage, "method.warm_origin")
	startOps, startResult, ok := xiuxianTechniqueOps(state, system, "protagonist",
		TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}, "ev1")
	if !ok {
		t.Fatal("starting a mastered method must apply")
	}
	if len(startResult.Started) != 1 {
		t.Fatalf("started = %+v, want 温元诀", startResult.Started)
	}
	circulating := appliedXiuxianState(t, state, startOps)

	settledOps, ok := settleXiuxianActorState(circulating, system, "protagonist", "ev2")
	if !ok {
		t.Fatal("the circulating character must settle")
	}
	settled := appliedXiuxianState(t, circulating, settledOps)

	wantAttack := int(float64(baseAttack) * 1.15) // 10% specialty + 5% quality
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldAttack); got != wantAttack {
		t.Fatalf("attack = %d, want %d", got, wantAttack)
	}
	wantHealth := int(float64(baseHealth) * 0.95)
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldHealthMax); got != wantHealth {
		t.Fatalf("health max = %d, want %d", got, wantHealth)
	}
}

// TestTwoCirculatingMethodsFollowThePublishedWorkedExample is §4.3's acceptance
// example carried through the real settlement: 温元诀 plus 凝元护体法 must give
// the design's own numbers, and the current health must not be refilled.
func TestTwoCirculatingMethodsFollowThePublishedWorkedExample(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	// 炼气六层: base 40 attack, 200 health.
	stage := cultivation.StageKey(cultivation.Qi(6))
	state := masteredTechniqueState(t, stage, "method.warm_origin", "method.origin_guard")

	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist", TurnTechniqueInput{
		Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}, {DefID: "method.origin_guard"}},
	}, "ev1")
	if !ok || len(result.Started) != 2 {
		t.Fatalf("both compatible methods must start: %+v", result.Refused)
	}
	circulating := appliedXiuxianState(t, state, ops)

	// Record a real wound first, so the test can prove a bonus does not heal it.
	circulating["actors"].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)[xiuxianFieldHealth] = float64(150)

	settledOps, ok := settleXiuxianActorState(circulating, system, "protagonist", "ev2")
	if !ok {
		t.Fatal("the character must settle")
	}
	settled := appliedXiuxianState(t, circulating, settledOps)

	// The design's own worked example: 温元诀's +10% attack and +5% grade are
	// carried, but 凝元护体法's −10% attack cost is still real, so attack is
	// +5% net — 40 → 42, not 46. Health is +10% and −5% = +5%: 200 → 210.
	// Defense +20% and −20% cancel, so it stays at the base 20.
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldAttack); got != 42 {
		t.Fatalf("attack = %d, want 42 (10 percent specialty + 5 percent grade − 10 percent guard cost)", got)
	}
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldHealthMax); got != 210 {
		t.Fatalf("health max = %d, want 210", got)
	}
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldDefense); got != 20 {
		t.Fatalf("defense = %d, want 20 (net zero)", got)
	}
	// The wound is still 150: a higher maximum is not a heal.
	if got := xiuxianStoredInt(settled, "protagonist", xiuxianFieldHealth); got != 150 {
		t.Fatalf("current health = %d, want the recorded wound 150", got)
	}
}

// TestWarmAndColdAreRefusedTogether is the one conflicting pair, answered from
// the character's real state rather than from prose.
func TestWarmAndColdAreRefusedTogether(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	state := masteredTechniqueState(t, cultivation.StageKey(cultivation.Qi(6)), "method.warm_origin", "method.cold_spring")

	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist", TurnTechniqueInput{
		Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}, {DefID: "method.cold_spring"}},
	}, "ev1")
	if !ok {
		t.Fatal("the module must be processed so the refusal is reported")
	}
	if len(result.Started) != 1 || result.Started[0].ID != "method.warm_origin" {
		t.Fatalf("started = %+v, want only the first method", result.Started)
	}
	if len(result.Refused) != 1 || result.Refused[0].Action != "circulate" {
		t.Fatalf("refused = %+v, want the cold method refused", result.Refused)
	}
	if len(ops) != 1 {
		t.Fatalf("ops = %+v, want only the accepted switch", ops)
	}
}

// TestStoppingAMethodKeepsTheMastery is §4.6's "retain mastered and pause". The
// method leaves the active set and the numbers go back, but the record still
// holds it and it can circulate again later.
func TestStoppingAMethodKeepsTheMastery(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	stage := cultivation.StageKey(cultivation.Qi(6))
	state := masteredTechniqueState(t, stage, "method.warm_origin")
	state["actors"].(map[string]any)["protagonist"].(map[string]any)["state"].(map[string]any)[xiuxianFieldTechniques].(map[string]any)["温元诀"].(map[string]any)["运转"] = "运转"

	if circulating := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, state)); len(circulating) != 1 {
		t.Fatalf("the fixture must start with one circulating method: %+v", circulating)
	}

	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist",
		TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}, "ev1")
	if !ok || len(result.Stopped) != 1 {
		t.Fatalf("stopped = %+v", result.Stopped)
	}
	stopped := appliedXiuxianState(t, state, ops)
	if circulating := circulatingTechniques(cultivation.ReferenceRules(),
		mustReadTechniques(t, stopped)); len(circulating) != 0 {
		t.Fatalf("the method is still circulating: %+v", circulating)
	}

	// Mastery survives: the record is still there and can circulate again.
	record := mustReadTechniques(t, stopped)["温元诀"]
	if !record.Mastered {
		t.Fatal("stopping a method lost the mastery")
	}
	restartOps, restartResult, ok := xiuxianTechniqueOps(stopped, system, "protagonist",
		TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}, "ev2")
	if !ok || len(restartResult.Started) != 1 || len(restartOps) != 1 {
		t.Fatalf("a stopped method could not circulate again: %+v", restartResult.Refused)
	}
}

// TestAnUnmasteredMethodCannotCirculate keeps the three facts apart: owning the
// record is not mastering the method.
func TestAnUnmasteredMethodCannotCirculate(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldTechniques: map[string]any{
			"温元诀": map[string]any{"定义": "method.warm_origin", "版本": float64(1), "掌握程度": "修习中", "运转": "停止"},
		},
	})

	ops, result, ok := xiuxianTechniqueOps(state, system, "protagonist",
		TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}}, "ev1")
	if !ok {
		t.Fatal("the module must be processed")
	}
	if len(result.Started) != 0 || len(result.Refused) != 1 || len(ops) != 0 {
		t.Fatalf("an unfinished method circulated: started=%+v refused=%+v", result.Started, result.Refused)
	}
}

// TestTheMethodRecordIsNotWritableByTheNarrative is the structural refusal: the
// model cannot set 功法 through state_changes, so a story cannot hand out a
// method by writing the record.
func TestTheMethodRecordIsNotWritableByTheNarrative(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	if !IsXiuxianSettledField(system, xiuxianFieldTechniques) {
		t.Fatal("the method record must be a rules-owned field")
	}
}

// TestTheMethodRecordTakesOverOnlyOnceItHoldsAMethod is the fallback rule.
//
// The preset declares the record field with an empty default, so every new
// character carries an empty object. Presence alone therefore cannot be the
// test: a character created from prose must keep being priced from that prose
// until they actually study something. Once the record holds a method, the
// prose is not counted a second time — including when every method in it has
// been stopped, because the stopped entries stay in the record.
func TestTheMethodRecordTakesOverOnlyOnceItHoldsAMethod(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	stage := cultivation.StageKey(cultivation.Qi(6))

	// The free text says 强攻/良好, which on its own is +10% attack and −20%
	// defense and +5% attack from the grade.
	legacy := xiuxianTestState(t, stage, map[string]any{xiuxianFieldMethod: "散修功法，强攻，良好。"})
	legacyOps, ok := settleXiuxianActorState(legacy, system, "protagonist", "ev0")
	if !ok {
		t.Fatal("the legacy record must settle")
	}
	legacySettled := appliedXiuxianState(t, legacy, legacyOps)
	legacyAttack := xiuxianStoredInt(legacySettled, "protagonist", xiuxianFieldAttack)
	if legacyAttack == 40 {
		t.Fatalf("the free-text specialisation did not apply: attack = %d", legacyAttack)
	}

	// A newly created character: the field exists but holds nothing. The prose
	// they were created with must still be priced, or the preset's empty default
	// would silently strip every new story of its opening specialisation.
	created := xiuxianTestState(t, stage, map[string]any{
		xiuxianFieldMethod:     "散修功法，强攻，良好。",
		xiuxianFieldTechniques: map[string]any{},
	})
	createdOps, ok := settleXiuxianActorState(created, system, "protagonist", "ev1")
	if !ok {
		t.Fatal("the created character must settle")
	}
	createdSettled := appliedXiuxianState(t, created, createdOps)
	if got := xiuxianStoredInt(createdSettled, "protagonist", xiuxianFieldAttack); got != legacyAttack {
		t.Fatalf("attack = %d, want %d: an empty method record suppressed the prose the character was created with", got, legacyAttack)
	}

	// Once a method is on the record, the prose stops counting. The method here
	// is stopped, which is the "deliberately stopped everything" state: the
	// record still holds it, so the character is priced by the record and has no
	// technique bonus at all.
	structured := xiuxianTestState(t, stage, map[string]any{
		xiuxianFieldMethod: "散修功法，强攻，良好。",
		xiuxianFieldTechniques: map[string]any{
			"温元诀": map[string]any{
				"定义": "method.warm_origin", "版本": float64(1),
				"掌握程度": "已掌握", "运转": "停止",
			},
		},
	})
	structuredOps, ok := settleXiuxianActorState(structured, system, "protagonist", "ev2")
	if !ok {
		t.Fatal("the structured record must settle")
	}
	structuredSettled := appliedXiuxianState(t, structured, structuredOps)
	if got := xiuxianStoredInt(structuredSettled, "protagonist", xiuxianFieldAttack); got != 40 {
		t.Fatalf("attack = %d, want the stage base 40: the free text was counted alongside a stopped method record", got)
	}
}

// TestTheGrowthCoefficientScalesTraining is §4.3's growth rule: the selected
// method's coefficient multiplies the training rate, and no structured method
// means the neutral 1.0.
func TestTheGrowthCoefficientScalesTraining(t *testing.T) {
	warm, _ := cultivation.TechniqueByID("method.warm_origin")
	state := cultivation.CharacterState{}.WithTechniques([]cultivation.TechniqueDef{warm})
	if got := state.GrowthCoefficient(""); got != 1.15 {
		t.Fatalf("growth coefficient = %v, want 1.15", got)
	}
	if got := (cultivation.CharacterState{}).GrowthCoefficient(""); got != 1.0 {
		t.Fatalf("a character with no structured method got growth %v, want 1.0", got)
	}
}

// TestTheSelectedMethodSuppliesTheTrainingGrade checks that training reads the
// best circulating grade rather than the free-text one.
func TestTheSelectedMethodSuppliesTheTrainingGrade(t *testing.T) {
	advanced, _ := cultivation.TechniqueByID("method.clear_mind")
	state := cultivation.CharacterState{}.WithTechniques([]cultivation.TechniqueDef{advanced})
	if got := state.PrimaryTechniqueQuality(); got != cultivation.QualityAdvanced {
		t.Fatalf("primary quality = %q, want advanced", got)
	}
	// With no structured record the free-text grade is the only source.
	if got := (cultivation.CharacterState{Quality: cultivation.QualityGood}).PrimaryTechniqueQuality(); got != cultivation.QualityGood {
		t.Fatalf("free-text quality = %q, want good", got)
	}
}

// TestACirculatingMethodLowersTheManaACastReallySpends is the live path for
// §4.3's cost reduction. 节息诀 is a 5% reduction, and it must reach the cast
// rather than only sitting in the record.
func TestACirculatingMethodLowersTheManaACastReallySpends(t *testing.T) {
	fireBolt, ok := storySkillRegistry(cultivation.ReferenceRules()).Current("condensed_blade")
	if !ok {
		t.Fatal("the rules' published table must contain condensed_blade")
	}
	// A cheap technique rounds the reduction away — 5% of an 8-point cost is
	// still 8 after the rules' round-up — so this uses one whose cost is large
	// enough for the reduction to be a whole point.
	stage := cultivation.Qi(6)

	plain := cultivation.Combatant{Cultivation: stage}
	plainReduced := fireBolt.ReducedManaCost(stage, plain.ManaReduction)

	thrifty, _ := cultivation.TechniqueByID("method.economical_breath")
	circulating := cultivation.Combatant{Cultivation: stage}
	character := cultivation.CharacterState{Cultivation: stage}.WithTechniques([]cultivation.TechniqueDef{thrifty})
	circulating.ManaReduction = character.ManaReduction()

	if circulating.ManaReduction != 0.05 {
		t.Fatalf("reduction = %v, want 0.05", circulating.ManaReduction)
	}
	withMethod := fireBolt.ReducedManaCost(stage, circulating.ManaReduction)
	if withMethod >= plainReduced {
		t.Fatalf("cost = %d with 节息诀 and %d without: the method changed nothing", withMethod, plainReduced)
	}
}

// TestTheManaReductionStopsWhenTheMethodStops is the other half: the reduction is
// a consequence of circulating, not of owning the record.
func TestTheManaReductionStopsWhenTheMethodStops(t *testing.T) {
	thrifty, _ := cultivation.TechniqueByID("method.economical_breath")
	circulating := cultivation.CharacterState{}.WithTechniques([]cultivation.TechniqueDef{thrifty})
	if got := circulating.ManaReduction(); got != 0.05 {
		t.Fatalf("circulating reduction = %v, want 0.05", got)
	}
	// The settlement attaches only circulating methods, so an empty structured
	// record is a character who stopped it.
	stopped := cultivation.CharacterState{}.WithTechniques(nil)
	if got := stopped.ManaReduction(); got != 0 {
		t.Fatalf("stopped reduction = %v, want 0", got)
	}
}

// techniqueStoryStore builds a real story whose protagonist opens at 练气六层
// with a full pool, so the commit path is exercised end to end rather than
// through xiuxianTechniqueOps alone.
//
// The opening state is seeded through the initial actor, which is the one place
// a story fact arrives without a settlement having produced it — the same
// convention the cultivation and item acceptance tests use.
func techniqueStoryStore(t *testing.T) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: DefaultActorID, Name: "主角", Fields: protagonistStateTemplate(cultivation.ReferenceRules(),
			spec).Fields,
	}}}
	system.InitialActors = []ActorStateInitialActor{{
		ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID,
		State: map[string]any{
			xiuxianFieldRealm:  cultivation.StageKey(cultivation.Qi(6)),
			xiuxianFieldHealth: 200, xiuxianFieldMana: 160,
		},
	}}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "功法验收", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// techniqueStoryFields reads the protagonist's stored fields after a turn.
func techniqueStoryFields(t *testing.T, store *Store, storyID string) map[string]any {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	actors, _ := snapshot.State[actorStateRoot].(map[string]any)
	actor, _ := actors[DefaultActorID].(map[string]any)
	fields, _ := actor["state"].(map[string]any)
	return fields
}

// TestTheCommitPathPrizesAMethodInTheSameTurn is the join the unit tests cannot
// cover: the method module runs before the settlement, so the attributes the
// turn commits already include the method that turn started circulating.
func TestTheCommitPathPrizesAMethodInTheSameTurn(t *testing.T) {
	store, storyID := techniqueStoryStore(t)

	// A first turn establishes the character with no method circulating. The
	// settlement writes the stage table's own numbers.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "他在洞府中静坐。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	before := techniqueStoryFields(t, store, storyID)
	if got := xiuxianStoredInt(map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"state": before}}}, DefaultActorID, xiuxianFieldAttack); got != 40 {
		t.Fatalf("baseline attack = %d, want the stage table's 40", got)
	}

	// The same turn that completes 温元诀's study and starts it circulating must
	// commit the settled numbers that include it.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "修炼", Narrative: "他把温元诀从头练到尾，随后运转起来。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Techniques: &TurnTechniqueInput{
				Learn:     []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 14400}},
				Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	after := techniqueStoryFields(t, store, storyID)
	root := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"state": after}}}
	if got := xiuxianStoredInt(root, DefaultActorID, xiuxianFieldAttack); got != 46 {
		t.Fatalf("attack = %d, want 46: 温元诀's +10 percent and 5 percent grade must be priced in the turn that started it", got)
	}
	if got := xiuxianStoredInt(root, DefaultActorID, xiuxianFieldHealthMax); got != 190 {
		t.Fatalf("health max = %d, want 190: the method's -5 percent applies in the same turn", got)
	}
	records, ok := readTechniqueRecords(root, DefaultActorID)
	if !ok {
		t.Fatal("the method record must be committed")
	}
	record, ok := records["温元诀"]
	if !ok || !record.Mastered || !record.Active {
		t.Fatalf("record = %+v, want 温元诀 mastered and circulating", record)
	}
}

// TestAStudiedMethodIsPricedOnlyWhenItCompletes is the same path's refusal half:
// an unfinished study writes progress and changes no attribute.
func TestAStudiedMethodIsPricedOnlyWhenItCompletes(t *testing.T) {
	store, storyID := techniqueStoryStore(t)

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "他在洞府中静坐。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "研读", Narrative: "他照着玉简读了一夜的温元诀。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Techniques: &TurnTechniqueInput{
				Learn: []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 360}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	fields := techniqueStoryFields(t, store, storyID)
	root := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"state": fields}}}
	if got := xiuxianStoredInt(root, DefaultActorID, xiuxianFieldAttack); got != 40 {
		t.Fatalf("attack = %d, want the unchanged 40: a night of reading is not a mastery", got)
	}
	records, ok := readTechniqueRecords(root, DefaultActorID)
	if !ok {
		t.Fatal("the study must still be recorded")
	}
	record := records["温元诀"]
	if record.Mastered || record.Active {
		t.Fatalf("record = %+v, want neither mastered nor circulating", record)
	}
	if record.Minutes != 360 {
		t.Fatalf("recorded minutes = %v, want the 360 actually spent", record.Minutes)
	}
}

// TestAStoppedMethodStopsCostingTheCharacter is the closing half of the loop: a
// stop in a later turn removes the bonus from the committed numbers while the
// mastery stays on record.
func TestAStoppedMethodStopsCostingTheCharacter(t *testing.T) {
	store, storyID := techniqueStoryStore(t)

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "他在洞府中静坐。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "修炼", Narrative: "他练成温元诀并运转起来。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			Techniques: &TurnTechniqueInput{
				Learn:     []TurnTechniqueLearning{{DefID: "method.warm_origin", Source: "完整温元诀注本", Minutes: 14400}},
				Circulate: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	raised := techniqueStoryFields(t, store, storyID)
	root := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"state": raised}}}
	if got := xiuxianStoredInt(root, DefaultActorID, xiuxianFieldAttack); got != 46 {
		t.Fatalf("attack before stopping = %d, want 46", got)
	}

	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "收功", Narrative: "他收了功，只把温元诀记在心里。",
		TurnResult: &TurnResult{
			Choices:    testTurnChoices(),
			Techniques: &TurnTechniqueInput{Stop: []TurnTechniqueSwitch{{DefID: "method.warm_origin"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	fields := techniqueStoryFields(t, store, storyID)
	root = map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{"state": fields}}}
	if got := xiuxianStoredInt(root, DefaultActorID, xiuxianFieldAttack); got != 40 {
		t.Fatalf("attack after stopping = %d, want the base 40", got)
	}
	records, ok := readTechniqueRecords(root, DefaultActorID)
	if !ok {
		t.Fatal("the record must survive a stop")
	}
	record := records["温元诀"]
	if !record.Mastered || record.Active {
		t.Fatalf("record = %+v, want mastered but stopped", record)
	}
}
