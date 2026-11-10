// Package storytime contains pure time arithmetic and long-running activity
// planning. Callers remain responsible for committing returned candidates to
// the canonical story journal.
package storytime

import (
	"errors"
	"fmt"
	"math"
)

// Instant is an elapsed second count from a caller-confirmed story origin.
// It is not a calendar date and must not be persisted as an independent clock.
type Instant int64

// Duration is an elapsed number of seconds.
type Duration int64

const (
	Second           Duration = 1
	Minute           Duration = 60 * Second
	Hour             Duration = 60 * Minute
	CalculationDay   Duration = 24 * Hour
	CalculationMonth Duration = 30 * CalculationDay
	CalculationYear  Duration = 12 * CalculationMonth
)

var (
	ErrNegativeTime           = errors.New("time values must not be negative")
	ErrTimeOverflow           = errors.New("time arithmetic exceeds the supported integer range")
	ErrTimeReversal           = errors.New("time cannot move backwards")
	ErrUnknownCalendar        = errors.New("calendar labels require an explicit caller-provided mapping")
	ErrStaleActivitySnapshot  = errors.New("activity snapshot is stale relative to the latest official time; reconcile before planning")
	ErrActivityReconciliation = errors.New("unresolved historical activity boundary requires domain reconciliation")
)

func NewInstant(seconds int64) (Instant, error) {
	if seconds < 0 {
		return 0, ErrNegativeTime
	}
	return Instant(seconds), nil
}

func NewDuration(seconds int64) (Duration, error) {
	if seconds < 0 {
		return 0, ErrNegativeTime
	}
	return Duration(seconds), nil
}

func Seconds(value int64) (Duration, error)           { return checkedScale(value, Second) }
func Minutes(value int64) (Duration, error)           { return checkedScale(value, Minute) }
func CalculationDays(value int64) (Duration, error)   { return checkedScale(value, CalculationDay) }
func CalculationMonths(value int64) (Duration, error) { return checkedScale(value, CalculationMonth) }
func CalculationYears(value int64) (Duration, error)  { return checkedScale(value, CalculationYear) }

func checkedScale(value int64, unit Duration) (Duration, error) {
	if value < 0 {
		return 0, ErrNegativeTime
	}
	if value > math.MaxInt64/int64(unit) {
		return 0, ErrTimeOverflow
	}
	return Duration(value * int64(unit)), nil
}

func (i Instant) Add(d Duration) (Instant, error) {
	if i < 0 || d < 0 {
		return 0, ErrNegativeTime
	}
	if int64(i) > math.MaxInt64-int64(d) {
		return 0, ErrTimeOverflow
	}
	return i + Instant(d), nil
}

func (i Instant) Since(earlier Instant) (Duration, error) {
	if i < 0 || earlier < 0 {
		return 0, ErrNegativeTime
	}
	if i < earlier {
		return 0, ErrTimeReversal
	}
	return Duration(i - earlier), nil
}

// Compare orders two supported relative instants.
func (i Instant) Compare(other Instant) (int, error) {
	if i < 0 || other < 0 {
		return 0, ErrNegativeTime
	}
	switch {
	case i < other:
		return -1, nil
	case i > other:
		return 1, nil
	default:
		return 0, nil
	}
}

// ParseCalendarLabel intentionally rejects unbound strings. Calendar meaning
// belongs to a world pack and must be converted by its explicit adapter.
func ParseCalendarLabel(string) (Instant, error) {
	return 0, fmt.Errorf("%w: no world calendar adapter is registered", ErrUnknownCalendar)
}

// Projection reports the new candidate for a cumulative activity target.
type Projection struct {
	Increment Duration `json:"increment_seconds"`
	Completed Duration `json:"completed_seconds"`
	Remaining Duration `json:"remaining_seconds"`
	TargetAt  Instant  `json:"target_at_seconds"`
}

// AdvanceCumulative projects one cumulative target from its confirmed origin.
// Replaying the same target produces a zero increment.
func AdvanceCumulative(start Instant, completed, target Duration) (Projection, error) {
	if start < 0 || completed < 0 || target < 0 {
		return Projection{}, ErrNegativeTime
	}
	if target < completed {
		return Projection{}, fmt.Errorf("cumulative target %d is behind completed duration %d", target, completed)
	}
	endpoint, err := start.Add(target)
	if err != nil {
		return Projection{}, err
	}
	return Projection{Increment: target - completed, Completed: target, Remaining: 0, TargetAt: endpoint}, nil
}
