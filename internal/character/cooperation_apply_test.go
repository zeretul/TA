package character

import (
	"encoding/json"
	"reflect"
	"testing"
)

func cooperationTestPhase(t *testing.T, phase CooperationPhase) Matter {
	t.Helper()
	matter := cooperationTestMatter(t, cooperationTestTerms())
	matter.Cooperation.Phase = phase
	if phase != CooperationAgreed {
		matter.Cooperation.WorkSourceRef = "event-work"
	}
	if phase == CooperationUsed || phase == CooperationReported {
		matter.Cooperation.UsedBy = "event-use"
	}
	if phase == CooperationReported {
		matter.Cooperation.ReportSourceRefs = []string{"result-1"}
	}
	return matter
}

func TestApplyCooperationMatterTransitionsAndReplay(t *testing.T) {
	var matters []Matter
	for _, phase := range []CooperationPhase{CooperationAgreed, CooperationWorked, CooperationUsed, CooperationReported} {
		next := cooperationTestPhase(t, phase)
		next.SourceEventID = "event-" + string(phase)
		var err error
		matters, err = ApplyCooperationMatter(matters, next)
		if err != nil {
			t.Fatalf("phase %s: %v", phase, err)
		}
		if !reflect.DeepEqual(matters, []Matter{next}) {
			t.Fatalf("phase %s did not retain one exact canonical record: %#v", phase, matters)
		}
		replayed, err := ApplyCooperationMatter(matters, next)
		if err != nil || !reflect.DeepEqual(replayed, matters) {
			t.Fatalf("phase %s replay changed the result: %#v, %v", phase, replayed, err)
		}
	}
	// Reopening the canonical JSON then adding a genuinely new report retains
	// the earlier report and consumes no new matter ID.
	data, _ := json.Marshal(matters)
	var restored []Matter
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	next := restored[0].Normalize()
	next.SourceEventID = "event-report-later"
	next.Cooperation.ReportSourceRefs = append(next.Cooperation.ReportSourceRefs, "result-2", "result-3", "result-4")
	updated, err := ApplyCooperationMatter(restored, next)
	if err != nil || !reflect.DeepEqual(updated, []Matter{next}) {
		t.Fatalf("additional known consequences were not recorded: %#v, %v", updated, err)
	}
	updated[0].Cooperation.ReportSourceRefs[0] = "changed"
	if restored[0].Cooperation.ReportSourceRefs[0] != "result-1" || next.Cooperation.ReportSourceRefs[0] != "result-1" {
		t.Fatal("cooperation merge aliases existing or next report sources")
	}
}

func TestApplyCooperationMatterAllowsAtomicWorkAndUnusedReport(t *testing.T) {
	worked := cooperationTestPhase(t, CooperationWorked)
	matters, err := ApplyCooperationMatter(nil, worked)
	if err != nil {
		t.Fatal(err)
	}
	report := worked.Normalize()
	report.Cooperation.Phase, report.Status = CooperationReported, MatterSettled
	report.Cooperation.ReportSourceRefs = []string{"ordinary-route-result"}
	matters, err = ApplyCooperationMatter(matters, report)
	if err != nil || len(matters) != 1 || matters[0].Cooperation.UsedBy != "" {
		t.Fatalf("reporting an alternative route implied plan use: %#v, %v", matters, err)
	}
}

func TestApplyCooperationMatterRejectsInvalidTransitions(t *testing.T) {
	for _, before := range []CooperationPhase{CooperationAgreed, CooperationWorked, CooperationUsed, CooperationReported} {
		for _, after := range []CooperationPhase{CooperationAgreed, CooperationWorked, CooperationUsed, CooperationReported} {
			previous, next := cooperationTestPhase(t, before), cooperationTestPhase(t, after)
			next.SourceEventID = "event-new-command"
			if before == CooperationWorked && after == CooperationReported {
				next.Cooperation.UsedBy = ""
			}
			allowed := before == CooperationAgreed && after == CooperationWorked ||
				before == CooperationWorked && (after == CooperationUsed || after == CooperationReported) ||
				before == CooperationUsed && after == CooperationReported
			_, err := ApplyCooperationMatter([]Matter{previous}, next)
			if (err == nil) != allowed {
				t.Errorf("%s -> %s: error=%v, allowed=%t", before, after, err, allowed)
			}
		}
	}
	for _, phase := range []CooperationPhase{CooperationUsed, CooperationReported} {
		if _, err := ApplyCooperationMatter(nil, cooperationTestPhase(t, phase)); err == nil {
			t.Errorf("new cooperation started at %s without its work record", phase)
		}
	}
}

func TestApplyCooperationMatterRejectsIdentityAndProvenanceChanges(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Matter)
	}{
		{"story", func(m *Matter) { m.StoryID = "other-story" }},
		{"branch", func(m *Matter) { m.BranchID = "other-branch" }},
		{"partner", func(m *Matter) { m.CharacterID = "other-character" }},
		{"project", func(m *Matter) { m.Cooperation.ProjectID = "other-project" }},
		{"version", func(m *Matter) { m.Cooperation.ProjectVersion = "2.0" }},
		{"instance", func(m *Matter) { m.Cooperation.RealmInstanceID = "other-instance" }},
		{"consent", func(m *Matter) { m.Cooperation.ConsentExchangeID = "other-consent" }},
		{"discovery", func(m *Matter) { m.Cooperation.SharedDiscoverySourceRef = "other-discovery" }},
		{"work", func(m *Matter) { m.Cooperation.WorkSourceRef = "other-work" }},
		{"used-by", func(m *Matter) { m.Cooperation.UsedBy = "other-use" }},
		{"removed-used-by", func(m *Matter) { m.Cooperation.UsedBy = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous := cooperationTestPhase(t, CooperationUsed)
			next := cooperationTestPhase(t, CooperationReported)
			test.edit(&next)
			assertCooperationApplyRejectedWithoutMutation(t, []Matter{previous}, next)
		})
	}
}

func TestApplyCooperationMatterRejectsReportRewrites(t *testing.T) {
	for _, refs := range [][]string{
		nil, {"result-1"}, {"result-2", "result-3"}, {"result-1", "result-1"},
		{"result-1", ""}, {"result-1", "2", "3", "4", "5"},
	} {
		previous := cooperationTestPhase(t, CooperationReported)
		next := previous.Normalize()
		next.SourceEventID = "event-new-report"
		next.Cooperation.ReportSourceRefs = refs
		assertCooperationApplyRejectedWithoutMutation(t, []Matter{previous}, next)
	}
	// A later report cannot invent plan use after the player already reported
	// taking another route, even when its other report sources are valid.
	previous := cooperationTestPhase(t, CooperationReported)
	previous.Cooperation.UsedBy = ""
	next := previous.Normalize()
	next.Cooperation.UsedBy = "late-invented-use"
	next.Cooperation.ReportSourceRefs = append(next.Cooperation.ReportSourceRefs, "result-2")
	assertCooperationApplyRejectedWithoutMutation(t, []Matter{previous}, next)
	// A report cannot stand in for the separate committed scene consumption.
	previous = cooperationTestPhase(t, CooperationWorked)
	next = cooperationTestPhase(t, CooperationReported)
	assertCooperationApplyRejectedWithoutMutation(t, []Matter{previous}, next)
}

func TestApplyCooperationMatterCanonicalIdentityAndOrdinaryIsolation(t *testing.T) {
	trusted := cooperationTestPhase(t, CooperationWorked)
	ordinary := trusted.Normalize()
	ordinary.Cooperation = nil
	assertCooperationApplyRejectedWithoutMutation(t, nil, ordinary)
	assertCooperationApplyRejectedWithoutMutation(t, []Matter{ordinary}, trusted)
	ordinary.ID = "mat-ordinary"
	merged, err := ApplyCooperationMatter([]Matter{ordinary}, trusted)
	if err != nil || len(merged) != 2 || merged[0].Cooperation != nil || !reflect.DeepEqual(merged[0], ordinary) {
		t.Fatalf("ordinary prose was upgraded or swallowed trusted work: %#v, %v", merged, err)
	}
	duplicate := trusted.Normalize()
	duplicate.ID, duplicate.Statement = "mat-new-id", "Different wording for the same work."
	assertCooperationApplyRejectedWithoutMutation(t, merged, duplicate)
	assertCooperationApplyRejectedWithoutMutation(t, []Matter{trusted, trusted}, trusted)
	// A genuinely different instance may carry its own canonical agreement.
	duplicate.Cooperation.RealmInstanceID = "realm-instance-2"
	merged, err = ApplyCooperationMatter(merged, duplicate)
	if err != nil || len(merged) != 3 {
		t.Fatalf("a different instance was collapsed into old work: %#v, %v", merged, err)
	}
}

func assertCooperationApplyRejectedWithoutMutation(t *testing.T, existing []Matter, next Matter) {
	t.Helper()
	beforeExisting, _ := json.Marshal(existing)
	beforeNext, _ := json.Marshal(next)
	if out, err := ApplyCooperationMatter(existing, next); err == nil {
		t.Fatalf("invalid cooperation update accepted: %#v", out)
	}
	afterExisting, _ := json.Marshal(existing)
	afterNext, _ := json.Marshal(next)
	if string(beforeExisting) != string(afterExisting) || string(beforeNext) != string(afterNext) {
		t.Fatal("rejected cooperation update mutated its input")
	}
}

func TestApplyCooperationMatterForkCreatesItsOwnScopeWithoutRebindingHistory(t *testing.T) {
	history := cooperationTestPhase(t, CooperationWorked)
	for _, scope := range []struct{ story, branch string }{{history.StoryID, "fork-1"}, {"other-story", history.BranchID}} {
		next := history.Normalize()
		next.ID, next.StoryID, next.BranchID = "mat-new-scope", scope.story, scope.branch
		next.Cooperation.Phase = CooperationAgreed
		next.Cooperation.WorkSourceRef = ""
		merged, err := ApplyCooperationMatter([]Matter{history}, next)
		if err != nil || len(merged) != 2 || !reflect.DeepEqual(merged[0], history) || !reflect.DeepEqual(merged[1], next) {
			t.Fatalf("independent scope reused or rebound inherited work: %#v %v", merged, err)
		}
		if ordinaryProjection := ApplyMatters([]Matter{history}, []Matter{next}); len(ordinaryProjection) != 2 {
			t.Fatalf("ordinary deduplication swallowed a different cooperation scope: %#v", ordinaryProjection)
		}
		// Reusing the inherited ID is not how a new branch establishes consent.
		next.ID = history.ID
		assertCooperationApplyRejectedWithoutMutation(t, []Matter{history}, next)
	}
}

func TestApplyCooperationMatterRefreshesOnlyUnperformedConsent(t *testing.T) {
	for _, phase := range []CooperationPhase{CooperationAgreed, CooperationWorked} {
		previous := cooperationTestPhase(t, CooperationAgreed)
		next := cooperationTestPhase(t, phase)
		next.Cooperation.ConsentExchangeID = "consent-C"
		next.SourceEventID = "event-current-C"
		next.TurnID = "turn-current-C"
		got, err := ApplyCooperationMatter([]Matter{previous}, next)
		if err != nil || !reflect.DeepEqual(got, []Matter{next}) {
			t.Errorf("agreed -> %s cannot use newer concrete consent: %#v %v", phase, got, err)
			continue
		}
		replayed, err := ApplyCooperationMatter(got, next)
		if err != nil || !reflect.DeepEqual(replayed, got) {
			t.Fatal("new consent replay was not idempotent")
		}
		if previous.Cooperation.ConsentExchangeID == next.Cooperation.ConsentExchangeID {
			t.Fatal("new consent mutated its predecessor")
		}
	}
	for _, phase := range []CooperationPhase{CooperationWorked, CooperationUsed, CooperationReported} {
		before := cooperationTestPhase(t, phase)
		after := before.Normalize()
		after.Cooperation.ConsentExchangeID = "replacement-after-work"
		after.SourceEventID = "replacement-event"
		if _, err := ApplyCooperationMatter([]Matter{before}, after); err == nil {
			t.Errorf("%s consent changed after work", phase)
		}
	}
	before := cooperationTestPhase(t, CooperationAgreed)
	same := before.Normalize()
	same.SourceEventID = "fresh-event-without-new-consent"
	same.TurnID = "fresh-turn"
	if _, err := ApplyCooperationMatter([]Matter{before}, same); err == nil {
		t.Fatal("metadata alone masqueraded as renewed consent")
	}
}
