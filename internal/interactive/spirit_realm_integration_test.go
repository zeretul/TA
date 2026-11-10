package interactive

import (
	"fmt"
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

func spiritRealmLegacySystem() StoryDirectorActorStateSystem {
	system := xiuxianTechniqueTestSystem()
	for i := range system.Templates {
		for j := range system.Templates[i].Fields {
			f := &system.Templates[i].Fields[j]
			if actorStateFieldID(*f) == xiuxianFieldRealm {
				f.Options = append([]string(nil), cultivation.StageKeys()[:26]...)
			}
		}
	}
	return system
}

func TestSpiritRealmFrozenHumanSchemaRemainsRecognizedWithoutExpansion(t *testing.T) {
	system := spiritRealmLegacySystem()
	if !ActorStateSystemIsXiuxian(system) {
		t.Fatal("old frozen story lost cultivation rules")
	}
	state := xiuxianTestState(t, "spirit_late", nil)
	before := cloneStoryState(state)
	ops, result, ok := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{Days: 30, AttemptBreakthrough: "void"}, xiuxianFormalFacts{}, advanceTestIdentity(), "frozen")
	if !ok || len(ops) != 0 || result.BreakthroughRolled || result.IntervalDays() != 0 || len(result.BreakthroughReasons) == 0 || !reflect.DeepEqual(before, state) {
		t.Fatalf("old frozen cap changed or paid: %+v", result)
	}
	// A duplicated/foreign enum is not the old exact schema.
	for j := range system.Templates[0].Fields {
		f := &system.Templates[0].Fields[j]
		if actorStateFieldID(*f) == xiuxianFieldRealm {
			f.Options[0] = "foreign_realm"
		}
	}
	if ActorStateSystemIsXiuxian(system) {
		t.Fatal("unrelated enum adopted cultivation rules")
	}
}

func TestSpiritRealmPaidLearningSponsorsNewBreakthroughs(t *testing.T) {
	system := xiuxianTechniqueTestSystem()
	for _, tc := range []struct{ stage, method, target, next string }{
		{"foundation_late", "method.core_continuation", "core", "core_early"},
		{"core_late", "method.nascent_continuation", "nascent", "nascent_early"},
		{"nascent_late", "method.spirit_continuation", "spirit", "spirit_early"},
		{"spirit_late", "method.void_unification", "void", "void_early"},
		{"void_late", "method.integration_return", "integration", "integration_early"},
		{"integration_late", "method.mahayana_insight", "mahayana", "mahayana_early"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			state := xiuxianTestState(t, tc.stage, nil)
			def, _ := cultivation.TechniqueByID(tc.method)
			// Isolated qualification fixture; acquisition is exercised by SpiritWorld tests.
			if def.RequiredManualID != "" {
				applyActorStateOp(state, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldImportantItems, Value: map[string]any{def.RequiredManualID: map[string]any{"定义": def.RequiredManualID, "数量": float64(1)}}})
			}
			ops, learned, ok := xiuxianTechniqueOps(state, system, DefaultActorID, TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: tc.method, Source: "本局已实际取得的完整续修传承", Minutes: float64(def.StandardLearningMinutes)}}}, "learn")
			if !ok || len(learned.Learned) != 1 {
				t.Fatalf("paid study failed: %+v", learned)
			}
			state = appliedXiuxianState(t, state, ops)
			ops, started, ok := xiuxianTechniqueOps(state, system, DefaultActorID, TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: tc.method}}}, "circulate")
			if !ok || len(started.Started) != 1 {
				t.Fatalf("mastered method unavailable: %+v", started)
			}
			state = appliedXiuxianState(t, state, ops)
			stage, _ := cultivation.ParseStageKey(tc.stage)
			need, _, _ := cultivation.ProgressRequired(stage)
			applyActorStateOp(state, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldRealmProgress, Value: need})
			// Find a reproducible successful draw for this isolated rule fixture; no
			// actual failed attempt is rewound or retried in a live Story.
			identity := advanceTestIdentity()
			for i := 0; i < 10000; i++ {
				identity.CommandID = fmt.Sprintf("spirit-fixture-%s-%d", tc.target, i)
				if cultivationBreakthroughRoll(identity, DefaultActorID) < 0.01 {
					break
				}
			}
			ops, out, ok := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{MethodID: tc.method, AttemptBreakthrough: tc.target}, xiuxianFormalFacts{}, identity, "promote")
			if !ok || !out.BreakthroughSucceeded || out.RealmAfter != tc.next || out.BreakthroughDays <= 0 {
				t.Fatalf("learned continuation failed: %+v", out)
			}
			after := appliedXiuxianState(t, state, ops)
			if xiuxianStoredStageKey(after, DefaultActorID) != tc.next {
				t.Fatal("result did not become formal state")
			}
			retryOps, retry, _ := advanceXiuxianCultivation(state, system, DefaultActorID, TurnCultivationInput{MethodID: tc.method, AttemptBreakthrough: tc.target}, xiuxianFormalFacts{}, identity, "promote")
			if !reflect.DeepEqual(ops, retryOps) || out.BreakthroughSucceeded != retry.BreakthroughSucceeded {
				t.Fatal("same attempt rerolled")
			}
			if err := validateTurnCultivationModule(&TurnCultivationInput{AttemptBreakthrough: tc.target}); err != nil {
				t.Fatal("model module rejects supported target")
			}
		})
	}
}

func TestSpiritRealmGeneratedMethodRetainsStageCeiling(t *testing.T) {
	record := generatedContentRecord("technique", "", "method.void_unification", "石壁炼虚法", "本局已取得的完整石壁传承", "气息沿五行缓缓收束。")
	id, err := generatedContentID("technique", "method.void_unification", "石壁炼虚法")
	if err != nil {
		t.Fatal(err)
	}
	record["定义"] = id
	def, ok := generatedTechniqueFromRecord(cultivation.ReferenceRules(),
		record)
	if !ok || def.MinimumStage != "spirit_late" || def.SupportsThrough != "void_late" {
		t.Fatalf("generated high method lost native rules: %+v %v", def, ok)
	}
	if cultivationMethodSupportsTarget(def, cultivation.BreakthroughIntegration) {
		t.Fatal("template crossed its actual ceiling")
	}
}

func TestSpiritRealmKnownLifespanDoesNotBorrowSpiritAgeDeath(t *testing.T) {
	meta := lifespanTestMeta(1900)
	for _, stage := range []string{"void_early", "integration_middle", "mahayana_late"} {
		state := lifespanTestState(stage)
		elapsed := meta.Calendar.SecondsPerYear() * 10000
		if _, ok := storyLifespanBoundaryFor(meta, state, DefaultActorID); ok {
			t.Fatal("invented finite boundary")
		}
		edge := LifespanEdgeFor(meta, state, elapsed)
		if edge == nil || !edge.NoFixedLimit || edge.CurrentAge != 11900 || edge.Reached {
			t.Fatalf("missing explicit lifespan distinction: %+v", edge)
		}
		if ending := LifespanTerminalCandidate(meta, state, elapsed); ending != nil {
			t.Fatalf("invented death/ending: %+v", ending)
		}
	}
}

func TestSpiritRealmSelectionRequiresRealMethodAndPrefersUsableContinuation(t *testing.T) {
	empty := xiuxianTestState(t, "void_early", nil)
	if _, reason := selectedCultivationMethod(cultivation.ReferenceRules(),
		empty, DefaultActorID, ""); reason == "" {
		t.Fatal("high realm trained from a free-text fallback")
	}
	if got := cultivationMethodSelectionDiagnostic(cultivation.ReferenceRules(),
		xiuxianTestState(t, "spirit_late", nil), &TurnCultivationInput{AttemptBreakthrough: "void"}); got == nil {
		t.Fatal("high breakthrough needs no actual method")
	}
	state := masteredTechniqueState(t, "spirit_late", "method.spirit_continuation", "method.void_unification", "method.clear_mind")
	for _, raw := range actorStateFieldValue(state, DefaultActorID, xiuxianFieldTechniques).(map[string]any) {
		raw.(map[string]any)["运转"] = "运转"
	}
	method, reason := selectedCultivationMethod(cultivation.ReferenceRules(),
		state, DefaultActorID, "")
	if reason != "" || method == nil || method.ID != "method.void_unification" {
		t.Fatalf("old method hides real continuation: %+v %s", method, reason)
	}
}

func TestSpiritRealmActualStudyBreakthroughAndBranchSurviveColdReload(t *testing.T) {
	system := xiuxianTestSystem()
	system.DefinitionDetails = map[string]cultivation.DefinitionDetails{
		"method.void_unification": {Localizations: &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Name: "Five-Element Void Unification"}}},
	}
	prior, _ := cultivation.ParseStageKey("spirit_late")
	need, _, _ := cultivation.ProgressRequired(prior)
	system.InitialActors = []ActorStateInitialActor{{ID: DefaultActorID, Name: "续修者", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: "spirit_late", xiuxianFieldRealmProgress: need, xiuxianFieldImportantItems: map[string]any{"manual.spirit.void": map[string]any{"定义": "manual.spirit.void", "数量": float64(1)}}}}}
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "灵界规则隔离回归", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	appendTurn := func(user string, result *TurnResult) TurnEvent {
		t.Helper()
		result.Choices = testTurnChoices()
		turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: user, Narrative: "隔离自动测试中的已发生行动。", TurnResult: result})
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}
	appendTurn("读取既有高阶修行起点", &TurnResult{})
	def, _ := cultivation.TechniqueByID("method.void_unification")
	appendTurn("按已取得的完整传承研习一百八十日", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: def.ID, Source: "本测试初始已取得的完整五行炼虚诀", Minutes: float64(def.StandardLearningMinutes)}}}})
	before := appendTurn("安全运转已经学会的炼虚功法", &TurnResult{Techniques: &TurnTechniqueInput{Circulate: []TurnTechniqueSwitch{{DefID: def.ID}}}})
	identity := advanceTestIdentity()
	for i := 0; i < 10000; i++ {
		identity.CommandID = fmt.Sprintf("cold-spirit-%d", i)
		if cultivationBreakthroughRoll(identity, DefaultActorID) < .01 {
			break
		}
	}
	intent, err := NewPlayerInputIntent(identity, "main", "尝试突破炼虚")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, intent); err != nil {
		t.Fatal(err)
	}
	after, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "尝试突破炼虚", Narrative: "已有的积累与完整传承终于接上了下一段修行。", AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle, TurnResult: &TurnResult{Choices: testTurnChoices(), Cultivation: &TurnCultivationInput{MethodID: def.ID, AttemptBreakthrough: "void"}}})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if xiuxianStoredStageKey(snap.State, DefaultActorID) != "void_early" {
		t.Fatalf("formal breakthrough did not persist: %+v", f16ActorFields(snap.State))
	}
	if SnapshotClockSeconds(snap) != (180+360)*86400+600 {
		t.Fatalf("learning/preparation/breakthrough clock mismatch: %d", SnapshotClockSeconds(snap))
	}
	oldFork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: before.ID, Title: "突破之前"})
	if err != nil {
		t.Fatal(err)
	}
	newFork, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: after.ID, Title: "突破之后"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	defer store.Close()
	for _, tc := range []struct{ id, stage string }{{"main", "void_early"}, {oldFork.ID, "spirit_late"}, {newFork.ID, "void_early"}} {
		got, err := store.Snapshot(story.ID, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if xiuxianStoredStageKey(got.State, DefaultActorID) != tc.stage {
			t.Fatalf("cold branch %s lost realm", tc.id)
		}
		if d, why := selectedCultivationMethod(cultivation.ReferenceRules(),
			got.State, DefaultActorID, def.ID); why != "" || d == nil {
			t.Fatalf("cold branch lost paid mastery: %s", why)
		}
		for locale, wantName := range map[string]string{"zh-CN": def.Name, "en-US": "Five-Element Void Unification"} {
			details, err := store.CultivationEntryDetails(story.ID, tc.id, locale)
			if err != nil || len(details.Techniques) != 1 || details.Techniques[0].Name != wantName {
				t.Fatalf("cold branch lost localized builtin title: locale=%s details=%+v err=%v", locale, details, err)
			}
		}
		frozen, err := store.ActorStateSchema(story.ID, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if !ActorStateSystemIsXiuxian(frozen.System) || !xiuxianTemplateSupportsStage(frozen.System.Templates[0], "mahayana_late") {
			t.Fatal("new frozen schema lost expansion")
		}
	}
}

func TestSpiritRealmLegacySchemaSurvivesColdBranchWithoutRewrite(t *testing.T) {
	system := spiritRealmLegacySystem()
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧人界冻结范围", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "旧局继续", Narrative: "旧日修行仍然有效。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "旧局分支"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	defer store.Close()
	for _, id := range []string{"main", branch.ID} {
		frozen, err := store.ActorStateSchema(story.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if !ActorStateSystemIsXiuxian(frozen.System) {
			t.Fatal("legacy schema no longer runs cultivation")
		}
		for _, template := range frozen.System.Templates {
			if xiuxianTemplateSupportsStage(template, "void_early") {
				t.Fatal("legacy frozen range silently expanded")
			}
		}
	}
}

func TestSpiritRealmBuiltinTitleTranslationPreservesStoryNames(t *testing.T) {
	def, _ := cultivation.TechniqueByID("method.void_unification")
	for _, tc := range []struct{ stored, want string }{
		{def.Name, "Five-Element Void Unification"},
		{def.ID, "Five-Element Void Unification"},
		{"师父手抄的五行诀", "师父手抄的五行诀"},
	} {
		t.Run(tc.stored, func(t *testing.T) {
			system := xiuxianTechniqueTestSystem()
			system.DefinitionDetails = map[string]cultivation.DefinitionDetails{
				def.ID: {Localizations: &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Name: "Five-Element Void Unification"}}},
			}
			system.InitialActors = []ActorStateInitialActor{{ID: DefaultActorID, Name: "续修者", TemplateID: DefaultActorID, State: map[string]any{
				xiuxianFieldRealm:      "spirit_late",
				xiuxianFieldTechniques: map[string]any{tc.stored: techniqueRecordValue(techniqueRecord{DefID: def.ID, Version: 1, Name: def.Name, Source: "此前取得的完整传承"})},
			}}}
			store := NewStore(t.TempDir())
			defer store.Close()
			story, err := store.CreateStory(CreateStoryRequest{Title: "冻结功法名称", ActorState: &system})
			if err != nil {
				t.Fatal(err)
			}
			talismanTurn(t, store, story.ID, "main", "查看已持有的传承", &TurnResult{})
			details, err := store.CultivationEntryDetails(story.ID, "main", "en-US")
			if err != nil || len(details.Techniques) != 1 || details.Techniques[0].Name != tc.want {
				t.Fatalf("wrong translated or authored name: %+v %v", details, err)
			}
		})
	}
}
