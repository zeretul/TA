package interactive

import (
	"testing"

	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// lifespanTestState builds the smallest state a lifespan question reads: one
// actor with a realm field. A realm of "" writes no field at all, which is the
// "this story is not on the cultivation rules" case.
func lifespanTestState(realmKey string) map[string]any {
	values := map[string]any{}
	if realmKey != "" {
		values[xiuxianFieldRealm] = realmKey
	}
	return map[string]any{
		"actors": map[string]any{
			DefaultActorID: map[string]any{
				"id":          DefaultActorID,
				"template_id": DefaultActorID,
				"state":       values,
			},
		},
	}
}

// lifespanTestMeta freezes the confirmed calendar and states an age.
func lifespanTestMeta(age int) StoryMeta {
	calendar := DefaultStoryCalendar()
	return StoryMeta{
		Calendar:       &calendar,
		ProtagonistAge: age,
	}
}

func qiStageKey(layer int) string {
	return cultivation.StageKey(cultivation.Qi(layer))
}

func foundationStageKey() string {
	return cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly})
}

// TestCalendarConvertsSecondsToConfirmedUnits checks the arithmetic the whole
// binding rests on: 24 hours a day, 30 days a month, 12 months a year.
func TestCalendarConvertsSecondsToConfirmedUnits(t *testing.T) {
	calendar := DefaultStoryCalendar()
	if err := calendar.Validate(); err != nil {
		t.Fatalf("default calendar must validate: %v", err)
	}
	const day = int64(24 * 60 * 60)
	if got := calendar.SecondsPerDay(); got != day {
		t.Fatalf("SecondsPerDay = %d, want %d", got, day)
	}
	if got := calendar.SecondsPerYear(); got != 360*day {
		t.Fatalf("SecondsPerYear = %d, want %d (360 days)", got, 360*day)
	}
	if StoryCalendarDaysPerYear != 360 {
		t.Fatalf("StoryCalendarDaysPerYear = %d, want 360", StoryCalendarDaysPerYear)
	}
}

// TestCalendarDescribeElapsed checks the display conversion, including the rule
// that a partial day is rounded up rather than dropped.
func TestCalendarDescribeElapsed(t *testing.T) {
	calendar := DefaultStoryCalendar()
	const day = int64(24 * 60 * 60)
	perYear := calendar.SecondsPerYear()

	cases := []struct {
		name    string
		seconds int64
		want    string
	}{
		{name: "zero", seconds: 0, want: ""},
		{name: "negative", seconds: -1, want: ""},
		{name: "partial day rounds up", seconds: day - 1, want: "1日"},
		{name: "one day", seconds: day, want: "1日"},
		{name: "one month", seconds: 30 * day, want: "1个月"},
		{name: "one year", seconds: perYear, want: "1年"},
		{name: "three years two months", seconds: 3*perYear + 2*30*day, want: "3年2个月"},
		{name: "year month day", seconds: perYear + 30*day + day, want: "1年1个月1日"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := calendar.DescribeElapsed(testCase.seconds); got != testCase.want {
				t.Fatalf("DescribeElapsed(%d) = %q, want %q", testCase.seconds, got, testCase.want)
			}
		})
	}
}

// TestCalendarValidateRejectsUnusableUnits keeps an invalid calendar from being
// treated as a year length. A zero unit would divide by zero; a wrong version is
// a convention this build does not know how to read.
func TestCalendarValidateRejectsUnusableUnits(t *testing.T) {
	good := DefaultStoryCalendar()
	if err := good.Validate(); err != nil {
		t.Fatalf("default calendar must validate: %v", err)
	}
	cases := []struct {
		name     string
		calendar StoryCalendar
	}{
		{name: "wrong version", calendar: StoryCalendar{Version: 99, HoursPerDay: 24, DaysPerMonth: 30, MonthsPerYear: 12}},
		{name: "zero hours", calendar: StoryCalendar{Version: StoryCalendarVersion, HoursPerDay: 0, DaysPerMonth: 30, MonthsPerYear: 12}},
		{name: "zero days per month", calendar: StoryCalendar{Version: StoryCalendarVersion, HoursPerDay: 24, DaysPerMonth: 0, MonthsPerYear: 12}},
		{name: "negative months", calendar: StoryCalendar{Version: StoryCalendarVersion, HoursPerDay: 24, DaysPerMonth: 30, MonthsPerYear: -1}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.calendar.Validate(); err == nil {
				t.Fatalf("expected %s to be rejected", testCase.name)
			}
		})
	}
}

// TestLifespanBoundaryWithholdsIncompleteData is TIME-08 and §4 in one place:
// every missing fact must produce "unknown", never an invented death.
func TestLifespanBoundaryWithholdsIncompleteData(t *testing.T) {
	withAge := lifespanTestMeta(20)
	noCalendar := lifespanTestMeta(20)
	noCalendar.Calendar = nil

	cases := []struct {
		name  string
		meta  StoryMeta
		state map[string]any
	}{
		{name: "no frozen calendar", meta: noCalendar, state: lifespanTestState(qiStageKey(1))},
		{name: "invalid calendar", meta: StoryMeta{Calendar: &StoryCalendar{Version: 99, HoursPerDay: 24, DaysPerMonth: 30, MonthsPerYear: 12}, ProtagonistAge: 20}, state: lifespanTestState(qiStageKey(1))},
		{name: "pack stated no age", meta: lifespanTestMeta(0), state: lifespanTestState(qiStageKey(1))},
		{name: "state has no realm field", meta: withAge, state: lifespanTestState("")},
		{name: "realm is not a stage key", meta: withAge, state: lifespanTestState("not-a-realm")},
		{name: "nil state", meta: withAge, state: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if boundary, ok := storyLifespanBoundaryFor(testCase.meta, testCase.state, DefaultActorID); ok {
				t.Fatalf("expected unknown, got boundary %+v", boundary)
			}
			if candidate := LifespanTerminalCandidate(testCase.meta, testCase.state, 1<<40); candidate != nil {
				t.Fatalf("incomplete data must not declare an ending, got %+v", candidate)
			}
		})
	}
}

// TestLifespanBoundaryCountsFromBirthNotFromBreakthrough checks the confirmed
// table is a total, and that a breakthrough adopts the new total while keeping
// the age (§3).
func TestLifespanBoundaryCountsFromBirthNotFromBreakthrough(t *testing.T) {
	calendar := DefaultStoryCalendar()
	perYear := calendar.SecondsPerYear()

	// 110 years old, still 炼气: 120 total, 10 left.
	qiBoundary, ok := storyLifespanBoundaryFor(lifespanTestMeta(110), lifespanTestState(qiStageKey(3)), DefaultActorID)
	if !ok {
		t.Fatal("expected a boundary for an established age and a known realm")
	}
	if qiBoundary.TotalYears != 120 {
		t.Fatalf("炼气 total = %d, want 120", qiBoundary.TotalYears)
	}
	if want := 10 * perYear; qiBoundary.At != want {
		t.Fatalf("炼气 At = %d, want %d (10 years)", qiBoundary.At, want)
	}
	if qiBoundary.AgeAtEdge != 120 {
		t.Fatalf("AgeAtEdge = %d, want 120", qiBoundary.AgeAtEdge)
	}

	// The same person after reaching 筑基: 200 total, 90 left, still 110.
	foundationBoundary, ok := storyLifespanBoundaryFor(lifespanTestMeta(110), lifespanTestState(foundationStageKey()), DefaultActorID)
	if !ok {
		t.Fatal("expected a boundary for 筑基")
	}
	if foundationBoundary.TotalYears != 200 {
		t.Fatalf("筑基 total = %d, want 200", foundationBoundary.TotalYears)
	}
	if want := 90 * perYear; foundationBoundary.At != want {
		t.Fatalf("筑基 At = %d, want %d (90 years)", foundationBoundary.At, want)
	}
	if foundationBoundary.EstablishedAge != 110 {
		t.Fatalf("EstablishedAge = %d, want the pack's 110 unchanged by the breakthrough", foundationBoundary.EstablishedAge)
	}
}

// TestLifespanBoundaryClampsAnAgePastItsTotal covers the pack that already states
// an age beyond the realm's confirmed total. The edge is the origin, not a
// refusal: §3 forbids letting an important character simply never age.
func TestLifespanBoundaryClampsAnAgePastItsTotal(t *testing.T) {
	boundary, ok := storyLifespanBoundaryFor(lifespanTestMeta(150), lifespanTestState(qiStageKey(1)), DefaultActorID)
	if !ok {
		t.Fatal("expected a boundary rather than a refusal")
	}
	if boundary.At != 0 {
		t.Fatalf("At = %d, want 0", boundary.At)
	}
	if boundary.AgeAtEdge != 150 {
		t.Fatalf("AgeAtEdge = %d, want the established 150", boundary.AgeAtEdge)
	}
}

// TestLifespanBoundaryUnlistedRealmStaysUnknown keeps 化神以上 out of the table
// rather than borrowing its neighbour's number.
func TestLifespanBoundaryUnlistedRealmStaysUnknown(t *testing.T) {
	for _, realm := range []cultivation.Realm{cultivation.Realm("beyond"), cultivation.Realm("")} {
		state := lifespanTestState(string(realm) + "_early")
		if boundary, ok := storyLifespanBoundaryFor(lifespanTestMeta(20), state, DefaultActorID); ok {
			t.Fatalf("realm %q must stay unknown, got %+v", realm, boundary)
		}
	}
}

// TestLifespanEndingNeedsACrossingNotTheEdge is §4's rule: standing at the edge
// is alive and owes the player a choice; only a clock past it is 寿尽.
func TestLifespanEndingNeedsACrossingNotTheEdge(t *testing.T) {
	meta := lifespanTestMeta(110)
	calendar := meta.Calendar
	boundary := lifespanBoundary(meta, lifespanTestState(qiStageKey(3)), DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}

	if ending := lifespanEnding(boundary, boundary.At, calendar, "正文"); ending != nil {
		t.Fatalf("standing at the edge must not end the branch, got %+v", ending)
	}
	if ending := lifespanEnding(boundary, boundary.At-1, calendar, "正文"); ending != nil {
		t.Fatalf("before the edge must not end the branch, got %+v", ending)
	}
	ending := lifespanEnding(boundary, boundary.At+1, calendar, "正文")
	if ending == nil {
		t.Fatal("a clock past the edge must end the branch")
	}
	if !ending.Terminal {
		t.Fatal("the ending must be terminal")
	}
	if ending.Type != TerminalOutcomeTypeLifespan {
		t.Fatalf("ending type = %q, want %q", ending.Type, TerminalOutcomeTypeLifespan)
	}
	if ending.FinalNarrativeSummary != "正文" {
		t.Fatalf("FinalNarrativeSummary = %q, want the turn's narrative", ending.FinalNarrativeSummary)
	}
	if len(ending.RestartSuggestions) == 0 {
		t.Fatal("the ending must offer the standard ways out")
	}
	if ending := lifespanEnding(nil, 1<<40, calendar, "正文"); ending != nil {
		t.Fatalf("an unresolved boundary must never end the branch, got %+v", ending)
	}
}

// TestLifespanTerminalCandidateMatchesTheEndingCheck keeps the two callers that
// decide the same thing from drifting: the turn check (before the prose) and the
// commit (after the interval) must agree on the boundary.
func TestLifespanTerminalCandidateMatchesTheEndingCheck(t *testing.T) {
	meta := lifespanTestMeta(110)
	state := lifespanTestState(qiStageKey(3))
	boundary := lifespanBoundary(meta, state, DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}
	if candidate := LifespanTerminalCandidate(meta, state, boundary.At); candidate != nil {
		t.Fatalf("the edge itself is alive, got %+v", candidate)
	}
	candidate := LifespanTerminalCandidate(meta, state, boundary.At+1)
	if candidate == nil {
		t.Fatal("a clock past the edge must declare the ending")
	}
	if candidate.Type != TerminalOutcomeTypeLifespan {
		t.Fatalf("candidate type = %q, want %q", candidate.Type, TerminalOutcomeTypeLifespan)
	}
}

// TestLifespanStopNodeOnlyInsideTheInterval keeps the interval planners honest:
// an edge strictly inside the interval is cut, and an edge at either end is not,
// because a zero-length interval is not a stop the player can act on.
func TestLifespanStopNodeOnlyInsideTheInterval(t *testing.T) {
	meta := lifespanTestMeta(110)
	boundary := lifespanBoundary(meta, lifespanTestState(qiStageKey(3)), DefaultActorID)
	if boundary == nil {
		t.Fatal("expected a resolved boundary")
	}
	edge := storytime.Instant(boundary.At)

	if _, ok := lifespanStopNode(boundary, edge, edge+100); ok {
		t.Fatal("an interval starting at the edge must not be cut to zero")
	}
	if _, ok := lifespanStopNode(boundary, edge-100, edge); ok {
		t.Fatal("an interval ending at the edge must not be cut")
	}
	if _, ok := lifespanStopNode(boundary, edge+1, edge+100); ok {
		t.Fatal("an interval entirely past the edge must not produce a stop")
	}
	node, ok := lifespanStopNode(boundary, edge-100, edge+100)
	if !ok {
		t.Fatal("an edge strictly inside the interval must stop it")
	}
	if node.At != edge {
		t.Fatalf("node instant = %d, want %d", node.At, edge)
	}
	if node.Kind != storytime.DecisionNode {
		t.Fatalf("node kind = %v, want a decision node", node.Kind)
	}
	if !node.RequiresResponse {
		t.Fatal("the lifespan edge must require the player's response")
	}
	if node.ID != storyLifespanNodeID {
		t.Fatalf("node id = %q, want %q", node.ID, storyLifespanNodeID)
	}
}

// TestWholeYearsAtFloors keeps §2's 显示整岁不丢失底层已过时长: a partial year must
// not advance the age, and the remainder stays on the clock.
func TestWholeYearsAtFloors(t *testing.T) {
	calendar := DefaultStoryCalendar()
	perYear := calendar.SecondsPerYear()
	if got := wholeYearsAt(calendar, perYear-1); got != 0 {
		t.Fatalf("wholeYearsAt(perYear-1) = %d, want 0", got)
	}
	if got := wholeYearsAt(calendar, perYear); got != 1 {
		t.Fatalf("wholeYearsAt(perYear) = %d, want 1", got)
	}
	if got := wholeYearsAt(calendar, 3*perYear+perYear/2); got != 3 {
		t.Fatalf("wholeYearsAt(3.5 years) = %d, want 3", got)
	}
	if got := wholeYearsAt(calendar, 0); got != 0 {
		t.Fatalf("wholeYearsAt(0) = %d, want 0", got)
	}
}

// TestCalendarDescribeDate checks the ordinal date the narrator is shown.
//
// The story's first instant is 第1年1月1日 — the origin is 以该局已提交起始日为
// 展示原点, an ordinal position on the branch's own axis rather than an epoch.
// Days are floored: twenty-three hours is still the first day, and the remainder
// stays on the clock rather than being rounded into a date that never happened.
func TestCalendarDescribeDate(t *testing.T) {
	calendar := DefaultStoryCalendar()
	const day = int64(24 * 60 * 60)
	perYear := calendar.SecondsPerYear()

	cases := []struct {
		name     string
		elapsed  int64
		want     string
	}{
		{name: "origin", elapsed: 0, want: "第1年1月1日"},
		{name: "still the first day", elapsed: day - 1, want: "第1年1月1日"},
		{name: "second day", elapsed: day, want: "第1年1月2日"},
		{name: "end of first month", elapsed: 30*day - 1, want: "第1年1月30日"},
		{name: "second month", elapsed: 30 * day, want: "第1年2月1日"},
		{name: "last day of first year", elapsed: perYear - 1, want: "第1年12月30日"},
		{name: "second year", elapsed: perYear, want: "第2年1月1日"},
	}
	for _, testCase := range cases {
		if got := calendar.DescribeDate(testCase.elapsed); got != testCase.want {
			t.Fatalf("%s: DescribeDate(%d) = %q, want %q", testCase.name, testCase.elapsed, got, testCase.want)
		}
	}

	// A negative instant is not a date, and an invalid calendar cannot divide.
	if got := calendar.DescribeDate(-1); got != "" {
		t.Fatalf("DescribeDate(-1) = %q, want empty", got)
	}
	broken := StoryCalendar{Version: StoryCalendarVersion, HoursPerDay: 0, DaysPerMonth: 30, MonthsPerYear: 12}
	if got := broken.DescribeDate(day); got != "" {
		t.Fatalf("an invalid calendar produced the date %q", got)
	}
}

// TestDeadlineUnitsMirrorsTheCalendar keeps the conversion the character package
// binds with honest: the units are the calendar's own, and a story with no
// calendar (or a broken one) converts to the zero value, which that package
// reads as "leave the label unbound".
func TestDeadlineUnitsMirrorsTheCalendar(t *testing.T) {
	calendar := DefaultStoryCalendar()
	units := calendar.DeadlineUnits()
	if units.SecondsPerDay != calendar.SecondsPerDay() {
		t.Fatalf("SecondsPerDay = %d, want %d", units.SecondsPerDay, calendar.SecondsPerDay())
	}
	if units.DaysPerMonth != StoryCalendarDaysPerMonth || units.MonthsPerYear != StoryCalendarMonthsPerYear {
		t.Fatalf("units = %+v, want 30/12", units)
	}
	if !units.Usable() {
		t.Fatal("the default calendar's units must be usable")
	}
	if got := (*StoryCalendar)(nil).DeadlineUnits(); got != (character.CalendarUnits{}) {
		t.Fatalf("a nil calendar converted to %+v, want the zero value", got)
	}
	broken := StoryCalendar{Version: StoryCalendarVersion, HoursPerDay: 24, DaysPerMonth: 0, MonthsPerYear: 12}
	if got := broken.DeadlineUnits(); got != (character.CalendarUnits{}) {
		t.Fatalf("an invalid calendar converted to %+v, want the zero value", got)
	}
}

// TestCreateStoryFreezesTheCalendar checks the freeze reaches the stored meta,
// because every lifespan question reads it from there rather than from the pack.
func TestCreateStoryFreezesTheCalendar(t *testing.T) {
	calendar := DefaultStoryCalendar()
	cloned := cloneStoryCalendar(&calendar)
	if cloned == nil {
		t.Fatal("clone must not drop a frozen calendar")
	}
	if cloned == &calendar {
		t.Fatal("clone must not share the pointer")
	}
	if *cloned != calendar {
		t.Fatalf("clone = %+v, want %+v", *cloned, calendar)
	}
	if cloneStoryCalendar(nil) != nil {
		t.Fatal("cloning a story with no calendar must stay nil")
	}
}

// TestLifespanEdgeReportsWhatTheNarratorNeeds is the shape §4 depends on: the
// scene block can only warn before the crossing if it is given the numbers, and
// every one of them has to agree with the boundary the rules cut at.
func TestLifespanEdgeReportsWhatTheNarratorNeeds(t *testing.T) {
	meta := lifespanTestMeta(110)
	state := lifespanTestState(qiStageKey(3))
	calendar := meta.Calendar
	perYear := calendar.SecondsPerYear()

	// At the origin: 110 years old, ten whole years left, 120 at the edge.
	edge := LifespanEdgeFor(meta, state, 0)
	if edge == nil {
		t.Fatal("a story with a calendar, an age and a realm must answer")
	}
	if edge.TotalYears != 120 || edge.AgeAtEdge != 120 {
		t.Fatalf("edge = %+v, want the 炼气 row's 120", edge)
	}
	if edge.CurrentAge != 110 {
		t.Fatalf("CurrentAge = %d, want the established 110", edge.CurrentAge)
	}
	if edge.RemainingYears != 10 {
		t.Fatalf("RemainingYears = %d, want 10", edge.RemainingYears)
	}
	if edge.Reached {
		t.Fatal("the origin is not the edge")
	}

	// Three whole years spent: 113 now, seven left. The current age follows the
	// clock rather than staying at the established number, because the narrator
	// has to be able to write the years that really passed.
	edge = LifespanEdgeFor(meta, state, 3*perYear)
	if edge.CurrentAge != 113 || edge.RemainingYears != 7 {
		t.Fatalf("after three years edge = %+v, want 113 with 7 left", edge)
	}

	// Eleven months into the fourth year. Both numbers floor, and they must floor
	// in a way that still adds up: 113 now, and 6 years 1 month of formal time
	// left, which is 6 whole years. Deriving the current age from TotalYears minus
	// RemainingYears would say 114 here.
	edge = LifespanEdgeFor(meta, state, 3*perYear+11*30*24*60*60)
	if edge.CurrentAge != 113 {
		t.Fatalf("CurrentAge = %d, want 113 — a partial year is not a birthday", edge.CurrentAge)
	}
	if edge.RemainingYears != 6 {
		t.Fatalf("RemainingYears = %d, want 6 — six years and one month is six whole years", edge.RemainingYears)
	}

	// Standing exactly on the edge is alive and is reported as reached rather than
	// as past it, which is what lets the scene say "you are alive and still choose".
	edge = LifespanEdgeFor(meta, state, 10*perYear)
	if edge == nil || !edge.Reached {
		t.Fatalf("the edge itself must report Reached, got %+v", edge)
	}
	if edge.CurrentAge != 120 {
		t.Fatalf("CurrentAge at the edge = %d, want 120", edge.CurrentAge)
	}
}

// TestLifespanEdgeStaysUnknownWhenTheStoryCannotAnswer keeps TIME-08 intact for
// the context block too: an incomplete story is told nothing rather than given a
// plausible-looking number.
func TestLifespanEdgeStaysUnknownWhenTheStoryCannotAnswer(t *testing.T) {
	cases := []struct {
		name  string
		meta  StoryMeta
		state map[string]any
	}{
		{"no frozen calendar", StoryMeta{ProtagonistAge: 30}, lifespanTestState(qiStageKey(3))},
		{"no stated age", StoryMeta{Calendar: func() *StoryCalendar { c := DefaultStoryCalendar(); return &c }()}, lifespanTestState(qiStageKey(3))},
		{"no realm field", lifespanTestMeta(30), lifespanTestState("")},
		{"unlisted realm", lifespanTestMeta(30), lifespanTestState("beyond_early")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if edge := LifespanEdgeFor(tc.meta, tc.state, 0); edge != nil {
				t.Fatalf("an unanswerable story produced an edge: %+v", edge)
			}
		})
	}
}
