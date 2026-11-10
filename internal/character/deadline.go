package character

import (
	"strings"
)

// BindDeadlineSeconds turns a deadline stated as an interval into the absolute
// formal instant it runs to, using the branch clock reading at the moment the
// arrangement was committed.
//
// This is the shape the time brief asks for. §审查采纳补充 says an agreement
// whose time semantics are relative — 三日内, 明日 — must be given a structural
// instant rather than ordered by whatever number happens to appear in its text.
// The number a label carries is a description; the branch clock is the only
// thing arithmetic may read.
//
// It reports false for every label it cannot read as an interval, and the caller
// then leaves the deadline unbound. That is deliberate: an unbound record keeps
// its text comparison, and uncertainty never expires anything. Absolute calendar
// positions such as 第3日 酉时 are not read here — see
// BindDeadlineSecondsOnCalendar, which places them on the branch's own axis.
//
// Only an interval at the head of the label is read. Scanning the whole label
// would find a numeral inside a place or a person's name and turn it into a
// duration, so the scan stops at the first rune that is not part of one.
func BindDeadlineSeconds(deadline string, nowSeconds int64) (int64, bool) {
	label := strings.TrimSpace(deadline)
	// Zero is a reading, not a missing one. A branch that has not yet spent formal
	// time sits at its own origin, and that origin is the story's current display
	// moment — so 三日内 written there runs to three days after the origin.
	//
	// Refusing zero used to make the most ordinary case in the game unbound: a
	// letter arrives while nothing has moved the clock yet, the player then rests,
	// and the letter's own interval had never been placed. It stayed live forever
	// and the offer could still be taken up long after its hour. A negative value
	// is the one that means "no reading at all".
	if label == "" || nowSeconds < 0 {
		return 0, false
	}
	// 第 marks an ordinal position in a calendar ("第3日"), not an interval. It is
	// placed by BindDeadlineSecondsOnCalendar and must not be approximated here.
	if strings.ContainsRune(label, '第') {
		return 0, false
	}
	return bindIntervalSeconds(label, nowSeconds)
}

// CalendarUnits is the year length a story froze, in the shape this package can
// use without importing the story's own package.
//
// The zero value means "this story has no calendar", and then no absolute label
// can be placed: the branch has an origin and a second count, but no month
// length to divide it by.
type CalendarUnits struct {
	SecondsPerDay int64
	DaysPerMonth  int64
	MonthsPerYear int64
}

// Usable reports whether the units can convert seconds into a date.
func (units CalendarUnits) Usable() bool {
	return units.SecondsPerDay > 0 && units.DaysPerMonth > 0 && units.MonthsPerYear > 0
}

// BindDeadlineSecondsOnCalendar is BindDeadlineSeconds plus the absolute form.
//
// 第3日 酉时 is a position in the story's own calendar, not an interval. It
// becomes a comparable instant only once the story has frozen a year length and
// called its own first day 第1日, which is what 以该局已提交起始日为展示原点
// establishes. With both in hand the label is arithmetic: day N is N-1 whole
// days from the origin, and a named 时辰 is its ordinal two-hour slot within
// that day.
//
// The alternative — the text comparison in compareWorldTime — is what this
// replaces for the cases it can place. That comparison orders 第3日 酉时 against
// 第3日 戌时 but cannot order a relative deadline against an absolute one, so a
// story that wrote 三日内 beside 第5日 酉时 had two threads on two scales.
// Placing both on the clock puts them on one.
//
// Labels it cannot place keep the unbound behaviour: a bare 第3日 with no 时辰
// stays unbound on purpose, because reading the day as midnight would expire a
// thread hours before the prose said it would.
func BindDeadlineSecondsOnCalendar(deadline string, nowSeconds int64, units CalendarUnits) (int64, bool) {
	if seconds, ok := bindAbsoluteSeconds(deadline, units); ok {
		return seconds, true
	}
	return BindDeadlineSeconds(deadline, nowSeconds)
}

// bindAbsoluteSeconds reads 第N日[ 时辰] as an instant on the branch's axis.
func bindAbsoluteSeconds(deadline string, units CalendarUnits) (int64, bool) {
	if !units.Usable() {
		return 0, false
	}
	label := strings.TrimSpace(deadline)
	if !strings.HasPrefix(label, "第") {
		// The ordinal must open the label. A 第 anywhere else is prose that
		// happens to contain the character, and reading a number out of it would
		// place a deadline the label never stated.
		return 0, false
	}
	runes := []rune(label)[1:]
	day, width, ok := readChineseOrArabicInteger(runes)
	if !ok || day < 1 {
		return 0, false
	}
	rest := strings.TrimSpace(string(runes[width:]))
	if !strings.HasPrefix(rest, "日") && !strings.HasPrefix(rest, "天") {
		return 0, false
	}
	hour, hasHour := shichenOrdinal(rest)
	if !hasHour {
		// A day with no 时辰 states no instant within it. See the doc comment:
		// this stays unbound rather than becoming midnight.
		return 0, false
	}
	return int64(day-1)*units.SecondsPerDay + int64(hour)*secondsPerShichen, true
}

// readChineseOrArabicInteger reads 一 through 九十九 or an ASCII integer at the
// head of runes, reusing the numeral reader the interval path already uses.
func readChineseOrArabicInteger(runes []rune) (int, int, bool) {
	value, width, ok := readAmount(runes)
	if !ok {
		return 0, 0, false
	}
	// 半 is a fraction, and a fractional day is not a calendar position.
	if value != float64(int(value)) {
		return 0, 0, false
	}
	return int(value), width, true
}

// bindIntervalSeconds reads a relative label from the moment it was written.
func bindIntervalSeconds(label string, nowSeconds int64) (int64, bool) {
	runes := []rune(label)
	// A label may open with a bound word rather than the quantity: 限三日内,
	// 须在三日内. They are skipped so the interval that follows is still read.
	for _, prefix := range []string{"限", "须在", "须於", "须于", "要在", "在于"} {
		if hasPrefixRunes(runes, prefix) {
			runes = runes[len([]rune(prefix)):]
			break
		}
	}
	if days, ok := relativeDayWords(runes); ok {
		return nowSeconds + days*secondsPerDay, true
	}
	amount, unit, ok := readInterval(runes)
	if !ok {
		return 0, false
	}
	seconds := int64(amount * float64(unit))
	if seconds <= 0 {
		return 0, false
	}
	return nowSeconds + seconds, true
}

const (
	secondsPerShichen = int64(2 * 60 * 60)
	secondsPerDay     = int64(24 * 60 * 60)
	// secondsPerMonth and secondsPerYear are the rules' own 30-day and 360-day
	// figures, which the brief permits for intervals. They are not a claim about
	// the world's calendar, and no calendar is consulted to reach them.
	secondsPerMonth = 30 * secondsPerDay
	secondsPerYear  = 360 * secondsPerDay
)

// relativeDayWords reads 明日 / 后日 / 次日 as whole-day offsets.
//
// These are relative by construction, which is why the brief names them. 今日 is
// deliberately absent: "by the end of today" does not say how many seconds that
// is, and binding it to zero would expire a thread the moment it was written.
func relativeDayWords(runes []rune) (int64, bool) {
	words := []struct {
		text string
		days int64
	}{
		{"明日", 1}, {"明天", 1}, {"次日", 1},
		{"后日", 2}, {"后天", 2},
	}
	for _, word := range words {
		if hasPrefixRunes(runes, word.text) {
			return word.days, true
		}
	}
	return 0, false
}

func hasPrefixRunes(runes []rune, text string) bool {
	prefix := []rune(text)
	if len(runes) < len(prefix) {
		return false
	}
	for index := range prefix {
		if runes[index] != prefix[index] {
			return false
		}
	}
	return true
}

// readInterval reads one "<number><unit>" pair at the head of runes.
//
// The unit must be a unit of elapsed time, which is what keeps a numeral that
// begins a word from being read as a duration: 一并 and 半路 fail the unit check
// even though their first rune parses as a quantity.
func readInterval(runes []rune) (float64, int64, bool) {
	amount, width, ok := readAmount(runes)
	if !ok {
		return 0, 0, false
	}
	rest := runes[width:]
	// 个 is a counter between the number and its unit: 三个时辰.
	if len(rest) > 0 && (rest[0] == '个' || rest[0] == '個') {
		rest = rest[1:]
	}
	for _, unit := range []struct {
		text    string
		seconds int64
	}{
		{"时辰", secondsPerShichen},
		{"日", secondsPerDay},
		{"天", secondsPerDay},
		{"月", secondsPerMonth},
		{"年", secondsPerYear},
	} {
		if hasPrefixRunes(rest, unit.text) {
			return amount, unit.seconds, true
		}
	}
	return 0, 0, false
}

// readAmount reads 半, a Chinese numeral, or an ASCII integer at the head of
// runes.
func readAmount(runes []rune) (float64, int, bool) {
	if len(runes) == 0 {
		return 0, 0, false
	}
	if runes[0] == '半' {
		return 0.5, 1, true
	}
	if runes[0] >= '0' && runes[0] <= '9' {
		var value float64
		width := 0
		for width < len(runes) && runes[width] >= '0' && runes[width] <= '9' {
			value = value*10 + float64(runes[width]-'0')
			width++
		}
		return value, width, true
	}
	return readChineseNumeral(runes)
}

// readChineseNumeral reads 一 through 九十九. It stops at the first rune that is
// not part of a numeral, so 三日 later yields three and leaves 日 for the unit
// check.
func readChineseNumeral(runes []rune) (float64, int, bool) {
	digits := map[rune]int{
		'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
	}
	if len(runes) == 0 {
		return 0, 0, false
	}
	// 十 on its own is ten; 十N is ten plus N; N十 is N tens; N十M is N tens plus M.
	if runes[0] == '十' {
		value := 10
		width := 1
		if len(runes) > 1 {
			if digit, ok := digits[runes[1]]; ok && digit > 0 {
				value += digit
				width = 2
			}
		}
		return float64(value), width, true
	}
	first, ok := digits[runes[0]]
	if !ok || first == 0 {
		return 0, 0, false
	}
	if len(runes) > 1 && runes[1] == '十' {
		value := first * 10
		width := 2
		if len(runes) > 2 {
			if digit, ok := digits[runes[2]]; ok && digit > 0 {
				value += digit
				width = 3
			}
		}
		return float64(value), width, true
	}
	return float64(first), 1, true
}
