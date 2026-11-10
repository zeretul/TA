package interactiveapp

import (
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

// These are the boundaries the cast-age join must not get wrong, kept next to
// the join rather than only in the end-to-end turn test. §2's two requirements —
// 年龄按实际游戏时间增加 and 显示整岁不丢失底层已过时长 — pull in opposite
// directions at the edges, so the floor and the off-by-one are worth stating.

func TestSpentWholeYearsFloorsAndKeepsTheRemainder(t *testing.T) {
	perYear := int64(interactive.DaysPerRuleYear) * 24 * 60 * 60
	// A story created before calendars existed still ages: the rule year is the
	// only year length it can honestly be read with.
	meta := interactive.StoryMeta{}
	cases := []struct {
		name    string
		seconds int64
		want    int
	}{
		{"nothing spent", 0, 0},
		{"a negative interval is not a year", -perYear, 0},
		{"one second short of a year", perYear - 1, 0},
		{"exactly one year", perYear, 1},
		{"ten years", 10 * perYear, 10},
		{"ten years and eleven months", 10*perYear + 11*30*24*60*60, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := spentWholeYears(meta, tc.seconds); got != tc.want {
				t.Fatalf("spentWholeYears(%d) = %d, want %d", tc.seconds, got, tc.want)
			}
		})
	}
}

// TestSpentWholeYearsUsesTheFrozenCalendar keeps the year length from drifting
// from the one the story actually froze. A calendar that disagrees with the rule
// year must win, because it is the convention the branch's own records were
// written under.
func TestSpentWholeYearsUsesTheFrozenCalendar(t *testing.T) {
	// Ten days a month, ten months a year: a hundred-day year, so 100 days is one
	// whole year rather than a fraction of a rule year.
	calendar := interactive.StoryCalendar{Version: interactive.StoryCalendarVersion, HoursPerDay: 24, DaysPerMonth: 10, MonthsPerYear: 10}
	meta := interactive.StoryMeta{Calendar: &calendar}
	perYear := calendar.SecondsPerYear()
	if got := spentWholeYears(meta, perYear); got != 1 {
		t.Fatalf("spentWholeYears(one frozen year) = %d, want 1", got)
	}
	if got := spentWholeYears(meta, 2*perYear-1); got != 1 {
		t.Fatalf("spentWholeYears(just under two frozen years) = %d, want 1", got)
	}
	// The rule year is longer than this calendar's year, so the same interval
	// under a story with no frozen calendar is a different number of years.
	if got := spentWholeYears(interactive.StoryMeta{}, perYear); got != 0 {
		t.Fatalf("the fallback must use the rule year, got %d", got)
	}
}

func TestCastAgesSkipsAnyoneWhoseAgeTheStoryNeverEstablished(t *testing.T) {
	storyCtx := interactive.StoryContext{Meta: interactive.StoryMeta{
		Protagonist: interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom,
			Profile: "姓名：顾承川。性别：男，年龄：22 岁。"},
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "mu_peiling", Name: "慕沛灵", Age: 46},
			{SourceKey: "no_stated_age", Name: "无名"},
		}},
	}}
	state := character.State{Profiles: []character.Profile{
		{ID: "mu_peiling", SourceKey: "mu_peiling", Name: "慕沛灵"},
		{ID: "no_stated_age", SourceKey: "no_stated_age", Name: "无名"},
		// A person the story invented has no source key at all.
		{ID: "a_shi", Name: "阿石"},
	}}
	perYear := int64(interactive.DaysPerRuleYear) * 24 * 60 * 60

	ages := castAges(storyCtx, state, interactive.WorldState{Clock: interactive.StoryClock{Seconds: 10 * perYear}})

	if got, want := ages["mu_peiling"], (character.AgeFact{Years: 56, Established: 46}); got != want {
		t.Fatalf("the authored age was not advanced by the spent interval: %+v", got)
	}
	if _, found := ages["no_stated_age"]; found {
		t.Fatal("an age nobody stated was invented from the clock")
	}
	if _, found := ages["a_shi"]; found {
		t.Fatal("an invented character was given an age")
	}
}

// TestCastAgesStaysSilentWhenNothingHasBeenSpent keeps the block from printing a
// number that would then look frozen: with no interval on the branch, the
// established age alone is not an age that moves.
func TestCastAgesStaysSilentWhenNothingHasBeenSpent(t *testing.T) {
	storyCtx := interactive.StoryContext{Meta: interactive.StoryMeta{
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "mu_peiling", Name: "慕沛灵", Age: 46},
		}},
	}}
	state := character.State{Profiles: []character.Profile{{ID: "mu_peiling", SourceKey: "mu_peiling"}}}

	if ages := castAges(storyCtx, state, interactive.WorldState{}); ages != nil {
		t.Fatalf("a branch that never spent an interval claimed ages: %+v", ages)
	}
}

// TestCastAgesNeedsAFrozenCatalog keeps a story that predates catalog freezing
// reading as "this story has no established ages", not as a zero.
func TestCastAgesNeedsAFrozenCatalog(t *testing.T) {
	state := character.State{Profiles: []character.Profile{{ID: "mu_peiling", SourceKey: "mu_peiling"}}}
	perYear := int64(interactive.DaysPerRuleYear) * 24 * 60 * 60

	if ages := castAges(interactive.StoryContext{}, state, interactive.WorldState{
		Clock: interactive.StoryClock{Seconds: perYear},
	}); ages != nil {
		t.Fatalf("a story with no frozen catalog produced ages: %+v", ages)
	}
}

func TestProtagonistEstablishedAgeOnlyReadsCreatedCharacters(t *testing.T) {
	cases := []struct {
		name   string
		meta   interactive.StoryProtagonist
		want   int
		reason string
	}{
		{"created from the creation page", interactive.StoryProtagonist{
			Mode: interactive.StoryProtagonistModeCustom, Profile: "姓名：顾承川。性别：男，年龄：22 岁。",
		}, 22, "the creation page established 22"},
		{"started from a lore character", interactive.StoryProtagonist{
			Mode: interactive.StoryProtagonistModeLore, Profile: "姓名：某人。性别：女，年龄：24 岁。",
		}, 0, "a lore snapshot is not a creation-page age"},
		{"default protagonist", interactive.StoryProtagonist{}, 0, "the default start establishes no age"},
		{"created without an age in the prose", interactive.StoryProtagonist{
			Mode: interactive.StoryProtagonistModeCustom, Profile: "姓名：顾承川。",
		}, 0, "nothing was stated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := protagonistEstablishedAge(tc.meta); got != tc.want {
				t.Fatalf("protagonistEstablishedAge = %d, want %d (%s)", got, tc.want, tc.reason)
			}
		})
	}
}
