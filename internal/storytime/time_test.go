package storytime

import (
	"errors"
	"math"
	"testing"
)

func TestCumulativeTwentySecondsReplayAndValidation(t *testing.T) {
	twenty, err := Seconds(20)
	if err != nil {
		t.Fatal(err)
	}
	completed := Duration(0)
	for range 10 {
		completed += twenty
	}
	if completed != 200 {
		t.Fatalf("ten rounds completed %d seconds, want 200", completed)
	}
	p, err := AdvanceCumulative(100, completed, completed)
	if err != nil {
		t.Fatal(err)
	}
	if p.Increment != 0 || p.TargetAt != 300 {
		t.Fatalf("replay projection = %+v", p)
	}
	if _, err := AdvanceCumulative(100, completed, completed-1); err == nil {
		t.Fatal("backwards cumulative target accepted")
	}
	if _, err := Seconds(-1); !errors.Is(err, ErrNegativeTime) {
		t.Fatalf("negative seconds error = %v", err)
	}
	if _, err := Minutes(math.MaxInt64); !errors.Is(err, ErrTimeOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
	if _, err := Instant(math.MaxInt64).Add(1); !errors.Is(err, ErrTimeOverflow) {
		t.Fatalf("instant overflow error = %v", err)
	}
}

func TestCumulativeMinutePrecision(t *testing.T) {
	from, _ := Minutes(135)
	to, _ := Minutes(180)
	p, err := AdvanceCumulative(0, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if p.Increment != 45*Minute || p.TargetAt != Instant(180*Minute) {
		t.Fatalf("projection = %+v", p)
	}
	withSeconds := from + 17
	p, err = AdvanceCumulative(0, withSeconds, to)
	if err != nil {
		t.Fatal(err)
	}
	if p.Increment != 44*Minute+43 {
		t.Fatalf("fractional minute was lost: %+v", p)
	}
}

func TestCumulativeShortSegmentsMatchOneLongTarget(t *testing.T) {
	target, _ := CalculationDays(90)
	whole, err := AdvanceCumulative(400, 0, target)
	if err != nil {
		t.Fatal(err)
	}
	completed := Duration(0)
	var total Duration
	for _, days := range []int64{15, 15, 15, 15, 15, 15} {
		completed += Duration(days) * CalculationDay
		part, err := AdvanceCumulative(400, total, completed)
		if err != nil {
			t.Fatal(err)
		}
		total += part.Increment
		if part.TargetAt != Instant(400+int64(completed)) {
			t.Fatalf("segment target time = %d, want %d", part.TargetAt, 400+completed)
		}
	}
	if total != whole.Increment || completed != whole.Completed {
		t.Fatalf("segmented=%d/%d, one-shot=%d/%d", total, completed, whole.Increment, whole.Completed)
	}
}

func TestCalendarLabelsRequireExplicitMapping(t *testing.T) {
	for _, label := range []string{"Year 4, day 2", "三年后", "測靈台開張當日", "2026-10-16"} {
		if _, err := ParseCalendarLabel(label); !errors.Is(err, ErrUnknownCalendar) {
			t.Fatalf("%q error = %v", label, err)
		}
	}
}
