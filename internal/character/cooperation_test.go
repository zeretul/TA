package character

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func cooperationTestMatter(t *testing.T, terms map[string]any) Matter {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"id": "mat-coop", "story_id": "story-1", "branch_id": "branch-1",
		"character_id": "steward", "kind": "promise", "status": "open",
		"statement": "Review the observations together.", "world_time": "day 1",
		"source_event_id": "event-agreement", "cooperation": terms,
	})
	if err != nil {
		t.Fatal(err)
	}
	var matter Matter
	if err := json.Unmarshal(data, &matter); err != nil {
		t.Fatal(err)
	}
	return matter
}

func cooperationTestTerms() map[string]any {
	return map[string]any{
		"project_id": "coop.herbcave.notes", "project_version": "1.0",
		"realm_instance_id": "realm-instance-1", "consent_exchange_id": "exchange-agree",
		"shared_discovery_source_ref": "explore:instance-1:clue", "phase": "agreed",
	}
}

// The canonical matter must retain typed cooperation through JSON recovery.
// Before the feature, decoding silently discarded the entire optional record.
func TestCooperationMatterJSONRoundTrip(t *testing.T) {
	terms := cooperationTestTerms()
	matter := cooperationTestMatter(t, terms)
	if err := matter.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(matter.Normalize())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw["cooperation"], terms) {
		t.Fatalf("cooperation did not survive recovery: got %#v, want %#v", raw["cooperation"], terms)
	}
}

func TestCooperationMatterRejectsIncompleteSources(t *testing.T) {
	for _, field := range []string{"project_id", "project_version", "realm_instance_id", "consent_exchange_id", "shared_discovery_source_ref"} {
		t.Run(field, func(t *testing.T) {
			terms := cooperationTestTerms()
			terms[field] = " \t "
			if err := cooperationTestMatter(t, terms).Validate(); err == nil {
				t.Fatalf("cooperation without %s was accepted", field)
			}
		})
	}
}

func TestCooperationMatterRejectsIncompleteStages(t *testing.T) {
	for _, phase := range []string{"", "unknown", "worked", "used", "reported"} {
		t.Run(phase, func(t *testing.T) {
			terms := cooperationTestTerms()
			terms["phase"] = phase
			if err := cooperationTestMatter(t, terms).Validate(); err == nil {
				t.Fatalf("incomplete cooperation phase %q was accepted", phase)
			}
		})
	}
}

func TestCooperationPhaseSourceCombinations(t *testing.T) {
	for _, phase := range []CooperationPhase{CooperationAgreed, CooperationWorked, CooperationUsed, CooperationReported} {
		for bits := 0; bits < 8; bits++ {
			terms := cooperationTestMatter(t, cooperationTestTerms()).Cooperation.Normalize()
			terms.Phase = phase
			work, use, report := bits&1 != 0, bits&2 != 0, bits&4 != 0
			if work {
				terms.WorkSourceRef = "event-work"
			}
			if use {
				terms.UsedBy = "event-approach"
			}
			if report {
				terms.ReportSourceRefs = []string{"explore:instance-1:preserved"}
			}
			valid := phase == CooperationAgreed && !work && !use && !report ||
				phase == CooperationWorked && work && !use && !report ||
				phase == CooperationUsed && work && use && !report ||
				phase == CooperationReported && work && report
			if err := terms.Validate(); (err == nil) != valid {
				t.Errorf("phase=%s work=%t use=%t report=%t: err=%v, valid=%t", phase, work, use, report, err, valid)
			}
		}
	}
}

func TestCooperationReportSourcesMustRemainDistinctAndNonempty(t *testing.T) {
	for _, refs := range [][]string{{""}, {" \t "}, {"result-1", " result-1 "}, {"result-1", ""}} {
		terms := cooperationTestMatter(t, cooperationTestTerms()).Cooperation.Normalize()
		terms.Phase, terms.WorkSourceRef, terms.ReportSourceRefs = CooperationReported, "event-work", refs
		if err := terms.Validate(); err == nil {
			t.Errorf("invalid report sources accepted: %#v", refs)
		}
	}
}

func TestCooperationNormalizationDoesNotAlterInput(t *testing.T) {
	terms := cooperationTestMatter(t, cooperationTestTerms()).Cooperation.Normalize()
	terms.ProjectID = " " + terms.ProjectID + " "
	terms.ProjectVersion = " 1.0 "
	terms.RealmInstanceID = " realm-instance-1 "
	terms.ConsentExchangeID = " exchange-agree "
	terms.SharedDiscoverySourceRef = " explore:instance-1:clue "
	terms.Phase, terms.WorkSourceRef, terms.UsedBy = " reported ", " event-work ", " event-approach "
	terms.ReportSourceRefs = []string{" result-1 ", " result-2 "}
	original, _ := json.Marshal(terms)
	normalized := terms.Normalize()
	if err := normalized.Validate(); err != nil {
		t.Fatal(err)
	}
	want := CooperationTerms{
		ProjectID: "coop.herbcave.notes", ProjectVersion: "1.0", RealmInstanceID: "realm-instance-1",
		ConsentExchangeID: "exchange-agree", SharedDiscoverySourceRef: "explore:instance-1:clue",
		Phase: CooperationReported, WorkSourceRef: "event-work", UsedBy: "event-approach",
		ReportSourceRefs: []string{"result-1", "result-2"},
	}
	if !reflect.DeepEqual(normalized, want) {
		t.Fatalf("normalized terms = %#v, want %#v", normalized, want)
	}
	normalized.ReportSourceRefs[0] = "changed"
	after, _ := json.Marshal(terms)
	if string(after) != string(original) {
		t.Fatalf("normalizing, validating or editing a clone changed its source: %s -> %s", original, after)
	}
}

func TestCooperationStatusRemainsIndependentOfPhase(t *testing.T) {
	// Settling or abandoning a thread must not fabricate a report or forbid a
	// player from deciding not to use a plan. The story chooses real transitions.
	for _, status := range []MatterStatus{MatterOpen, MatterSettled, MatterAbandoned} {
		for _, phase := range []CooperationPhase{CooperationAgreed, CooperationWorked, CooperationUsed, CooperationReported} {
			matter := cooperationTestMatter(t, cooperationTestTerms())
			matter.Status, matter.Cooperation.Phase = status, phase
			if phase != CooperationAgreed {
				matter.Cooperation.WorkSourceRef = "event-work"
			}
			if phase == CooperationUsed {
				matter.Cooperation.UsedBy = "event-approach"
			}
			if phase == CooperationReported {
				matter.Cooperation.ReportSourceRefs = []string{"result-alternative-route"}
			}
			if err := matter.Validate(); err != nil {
				t.Errorf("status=%s phase=%s rejected: %v", status, phase, err)
			}
		}
	}
}

func TestCooperationProjectionIsTrustedOnlyAndCloned(t *testing.T) {
	terms := cooperationTestMatter(t, cooperationTestTerms()).Cooperation.Normalize()
	terms.Phase, terms.WorkSourceRef = CooperationReported, "event-work"
	terms.ReportSourceRefs = []string{"result-alternative-route"}
	draft := MatterDraft{
		CharacterID: "steward", Kind: MatterPromise, Statement: "Review the observations together.",
		Cooperation: &terms,
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "cooperation") {
		t.Fatalf("trusted fields leaked into model draft JSON: %s", encoded)
	}
	var untrusted MatterDraft
	if err := json.Unmarshal([]byte(`{"character_id":"steward","kind":"promise","statement":"We already worked together.","cooperation":{"project_id":"coop.herbcave.notes","phase":"worked","work_source_ref":"forged"}}`), &untrusted); err != nil {
		t.Fatal(err)
	}
	if untrusted.Cooperation != nil {
		t.Fatal("model JSON populated trusted cooperation")
	}
	plain, err := Project(testProjectionContext(), TurnCharacterFacts{Matters: []MatterDraft{untrusted}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Matters) != 1 || plain.Matters[0].Cooperation != nil {
		t.Fatalf("ordinary prose was upgraded to cooperation: %#v", plain.Matters)
	}
	projected, err := Project(testProjectionContext(), TurnCharacterFacts{Matters: []MatterDraft{draft}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Matters) != 1 || !reflect.DeepEqual(projected.Matters[0].Cooperation, &terms) {
		t.Fatalf("trusted cooperation was not preserved: %#v", projected.Matters)
	}
	matter := projected.Matters[0]
	if matter.StoryID != "story-1" || matter.BranchID != "branch-1" || matter.CharacterID != "steward" || matter.SourceEventID != "commit-7" {
		t.Fatalf("projection lost trusted identity/source: %#v", matter)
	}
	matter.Cooperation.ReportSourceRefs[0] = "changed"
	if terms.ReportSourceRefs[0] != "result-alternative-route" {
		t.Fatal("projected report aliases the application draft")
	}
	bad := draft
	badTerms := terms.Normalize()
	badTerms.WorkSourceRef = ""
	bad.Cooperation = &badTerms
	if _, err := Project(testProjectionContext(), TurnCharacterFacts{Matters: []MatterDraft{bad}}); err == nil {
		t.Fatal("projection accepted an incomplete trusted record")
	}
	bad = draft
	bad.CharacterID = "never-met"
	if _, err := Project(testProjectionContext(), TurnCharacterFacts{Matters: []MatterDraft{bad}}); err == nil {
		t.Fatal("cooperation bypassed normal known-character validation")
	}
}

func TestCooperationMatterViewsAndMergesDoNotAlias(t *testing.T) {
	terms := cooperationTestTerms()
	terms["phase"], terms["work_source_ref"] = "reported", "event-work"
	terms["report_source_refs"] = []string{"result-1", "result-2"}
	for _, view := range []struct {
		name string
		read func(Matter) Matter
	}{
		{"normalize", func(m Matter) Matter { return m.Normalize() }},
		{"state", func(m Matter) Matter { return (State{Matters: []Matter{m}}).Normalize().Matters[0] }},
		{"for-character", func(m Matter) Matter { return (State{Matters: []Matter{m}}).MattersFor("steward")[0] }},
		{"open", func(m Matter) Matter { return OpenMatters("steward", []Matter{m})[0] }},
		{"resolved", func(m Matter) Matter { m.Status = MatterSettled; return ResolvedMatters("steward", []Matter{m})[0] }},
		{"by-id", func(m Matter) Matter { found, _ := MatterByID([]Matter{m}, m.ID); return found }},
		{"existing-merge", func(m Matter) Matter { return ApplyMatters([]Matter{m}, nil)[0] }},
		{"projected-merge", func(m Matter) Matter { return ApplyMatters(nil, []Matter{m})[0] }},
	} {
		t.Run(view.name, func(t *testing.T) {
			original := cooperationTestMatter(t, terms)
			before, _ := json.Marshal(original)
			copy := view.read(original)
			copy.Cooperation.ReportSourceRefs[0] = "changed"
			copy.Cooperation.ProjectID = "other-project"
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatalf("editing a read result changed the source: %s -> %s", before, after)
			}
		})
	}
}

func TestCooperationRecoveryAndRetryRetainSources(t *testing.T) {
	terms := cooperationTestTerms()
	terms["phase"], terms["work_source_ref"] = "reported", "event-work"
	terms["report_source_refs"] = []string{"result-1"}
	matter := cooperationTestMatter(t, terms)
	state := State{Matters: []Matter{matter}}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	replayed := ApplyMatters(restored.Normalize().Matters, []Matter{matter})
	if !reflect.DeepEqual(replayed, []Matter{matter}) {
		t.Fatalf("recovery/retry changed or duplicated cooperation: %#v", replayed)
	}
}

func TestCooperationOrdinaryMatterCannotSwallowTrustedRecord(t *testing.T) {
	trusted := cooperationTestMatter(t, cooperationTestTerms())
	ordinary := trusted.Normalize()
	ordinary.ID, ordinary.Cooperation = "mat-ordinary", nil
	for _, input := range []struct{ existing, projected []Matter }{
		{[]Matter{ordinary}, []Matter{trusted}},
		{[]Matter{trusted}, []Matter{ordinary}},
	} {
		merged := ApplyMatters(input.existing, input.projected)
		if len(merged) != 2 {
			t.Fatalf("identical prose collapsed ordinary and trusted matters: %#v", merged)
		}
		for _, m := range merged {
			if m.ID == ordinary.ID && m.Cooperation != nil {
				t.Fatal("ordinary matter was upgraded by matching text")
			}
		}
	}
}

func TestCooperationRejectsTooManyReportSources(t *testing.T) {
	terms := cooperationTestMatter(t, cooperationTestTerms()).Cooperation.Normalize()
	terms.Phase, terms.WorkSourceRef = CooperationReported, "event-work"
	terms.ReportSourceRefs = []string{"a", "b", "c", "d", "e"}
	if err := terms.Validate(); err == nil {
		t.Fatal("more than four report sources were accepted")
	}
}
