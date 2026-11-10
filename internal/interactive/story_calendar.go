package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

// This file is the first-version calendar a story can freeze.
//
// Until now the world's calendar was unbound free text: storytime.ParseCalendarLabel
// refuses every label because no adapter is registered, and the formal clock only
// ever answered "how many seconds since this branch's origin". That was honest but
// incomplete, and it left one thing impossible: a lifespan is a number of years,
// and a number of years cannot be compared with a second count without a year
// length.
//
// 自制-时间寿元与长时世界推进 asks for the smallest calendar that makes the
// comparison honest — 一昼夜24小时、一月30日、一年12月360日 — and for it to be
// frozen per story rather than read live. Three properties follow from that.
//
//   - A story that never froze a calendar has none. Old stories are not migrated
//     and no default is applied to them: §三.2 forbids back-filling history, and
//     silently giving an in-progress story a year length it never had would
//     re-read numbers that were written under a different convention. A nil
//     calendar makes every lifespan question unanswerable rather than guessed.
//
//   - The origin is a label, not a date. StoryClock.Anchor already holds the text
//     the story called its starting moment. This file never parses it, and the
//     calendar deliberately carries no year number: the design says 以该局已提交
//     起始日为展示原点, which is a display convention, not an epoch.
//
//   - The units are a duration conversion and nothing more. They say how many
//     seconds a year is. They do not claim that the world's prose is wrong when it
//     writes 三月初七, and they are not a second date axis beside the story's own.

// StoryCalendarVersion identifies the shape of a frozen calendar.
//
// It is versioned for the same reason StoryClock is: a later batch that binds a
// real authored calendar must be able to tell one convention from another instead
// of re-reading old numbers under new meaning.
const StoryCalendarVersion = 1

// The first version's units. They are the values the design confirmed, and they
// are the same 30/360 the cultivation rules already integrate against
// (cultivation.DaysPerRuleMonth, DaysPerRuleYear), so a story does not end up
// with two year lengths that disagree by five days.
const (
	StoryCalendarHoursPerDay   = 24
	StoryCalendarDaysPerMonth  = 30
	StoryCalendarMonthsPerYear = 12
)

// StoryCalendarDaysPerYear is one calendar year in days.
const StoryCalendarDaysPerYear = StoryCalendarDaysPerMonth * StoryCalendarMonthsPerYear

// StoryCalendar is the calendar a story froze when it started.
//
// It is stored on StoryMeta rather than read from the world pack on every turn,
// for the same reason the module graph and the pack catalog are: editing a pack
// must change the next game rather than one already under way.
type StoryCalendar struct {
	Version      int `json:"version"`
	HoursPerDay  int `json:"hours_per_day"`
	DaysPerMonth int `json:"days_per_month"`
	MonthsPerYear int `json:"months_per_year"`
}

// DefaultStoryCalendar is the first version's calendar, ready to be frozen.
func DefaultStoryCalendar() StoryCalendar {
	return StoryCalendar{
		Version:       StoryCalendarVersion,
		HoursPerDay:   StoryCalendarHoursPerDay,
		DaysPerMonth:  StoryCalendarDaysPerMonth,
		MonthsPerYear: StoryCalendarMonthsPerYear,
	}
}

// cloneStoryCalendar copies a frozen calendar so two stories never share one
// value through a pointer.
func cloneStoryCalendar(calendar *StoryCalendar) *StoryCalendar {
	if calendar == nil {
		return nil
	}
	cloned := *calendar
	return &cloned
}

// Validate rejects a calendar whose units cannot honestly convert seconds.
//
// A zero or negative unit would divide by zero or produce a negative year, and a
// calendar that does either is worse than no calendar: the caller is expected to
// treat an invalid one as unknown rather than to repair it here.
func (calendar StoryCalendar) Validate() error {
	if calendar.Version != StoryCalendarVersion {
		return fmt.Errorf("故事历法版本不支持 / unsupported story calendar version: %d", calendar.Version)
	}
	if calendar.HoursPerDay <= 0 || calendar.DaysPerMonth <= 0 || calendar.MonthsPerYear <= 0 {
		return fmt.Errorf("故事历法的日、月、年单位必须为正 / story calendar units must be positive")
	}
	return nil
}

// SecondsPerDay is one calendar day in seconds.
func (calendar StoryCalendar) SecondsPerDay() int64 {
	return int64(calendar.HoursPerDay) * 60 * 60
}

// SecondsPerYear is one calendar year in seconds.
func (calendar StoryCalendar) SecondsPerYear() int64 {
	return int64(calendar.DaysPerMonth) * int64(calendar.MonthsPerYear) * calendar.SecondsPerDay()
}

// DeadlineUnits is this calendar in the shape the character package binds a
// deadline with.
//
// It exists so a record that states 第3日 酉时 is placed on the branch's own axis
// rather than ordered as text. The character package cannot import this one —
// the dependency runs the other way — so the conversion lives here, and a nil or
// invalid calendar converts to the zero value, which that package reads as "no
// calendar, leave the label unbound".
func (calendar *StoryCalendar) DeadlineUnits() character.CalendarUnits {
	if calendar == nil || calendar.Validate() != nil {
		return character.CalendarUnits{}
	}
	return character.CalendarUnits{
		SecondsPerDay: calendar.SecondsPerDay(),
		DaysPerMonth:  int64(calendar.DaysPerMonth),
		MonthsPerYear: int64(calendar.MonthsPerYear),
	}
}

// DescribeDate renders an instant as the story's own date, counting the day the
// story began as 1年1月1日.
//
// The origin is the story's committed start, which is what 以该局已提交起始日为
// 展示原点 means: the calendar carries no epoch, so this is an ordinal position
// on the branch's own axis rather than a claim about the world's calendar. A
// pack that writes 三月初七 is still writing its own date; this answers only
// "how far into the story are we", and a narrator that needs to write 当前时间
// needs that answer, because the label it inherited was written at the start and
// never moves.
//
// The day count is a floor, not a rounding: at 23 hours the story is still on
// its first day. The remainder is not lost — it stays on the clock, and
// DescribeElapsed states it beside this.
func (calendar StoryCalendar) DescribeDate(elapsedSeconds int64) string {
	if elapsedSeconds < 0 {
		return ""
	}
	if err := calendar.Validate(); err != nil {
		return ""
	}
	perDay := calendar.SecondsPerDay()
	perMonth := int64(calendar.DaysPerMonth)
	perYear := perMonth * int64(calendar.MonthsPerYear)
	if perDay <= 0 || perMonth <= 0 || perYear <= 0 {
		return ""
	}
	dayIndex := elapsedSeconds / perDay
	year := dayIndex/perYear + 1
	rest := dayIndex % perYear
	return fmt.Sprintf("第%d年%d月%d日", year, rest/perMonth+1, rest%perMonth+1)
}

// DescribeElapsed renders an interval as 年月日 under this calendar.
//
// The story's own prose is never overwritten with this. It exists so a reader —
// a scene block, a panel — can say "已经过去三年零两个月" without inventing a
// month length, and so the same number reads the same way everywhere.
//
// The wording is coarse on purpose. A year and a month are exact under this
// calendar; a day is not broken into 时辰 here, because the display already has
// DescribeElapsed for a sub-day interval and two renderings of the same seconds
// would disagree at the boundary.
func (calendar StoryCalendar) DescribeElapsed(elapsedSeconds int64) string {
	if elapsedSeconds <= 0 {
		return ""
	}
	if err := calendar.Validate(); err != nil {
		return ""
	}
	perYear := calendar.SecondsPerYear()
	perDay := calendar.SecondsPerDay()
	perMonth := perDay * int64(calendar.DaysPerMonth)

	years := elapsedSeconds / perYear
	rest := elapsedSeconds % perYear
	months := rest / perMonth
	rest %= perMonth
	days := rest / perDay
	if rest%perDay != 0 {
		// A partial day still happened. Rounding it away would let a journey of
		// one day and twenty-three hours read as one day, and the remainder would
		// then be lost from the display even though the clock keeps it.
		days++
	}

	parts := make([]string, 0, 3)
	if years > 0 {
		parts = append(parts, fmt.Sprintf("%d年", years))
	}
	if months > 0 {
		parts = append(parts, fmt.Sprintf("%d个月", months))
	}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d日", days))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "")
}
