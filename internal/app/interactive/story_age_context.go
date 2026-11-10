package interactiveapp

import (
	"strings"

	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file joins the two halves of a person's age that the game already had
// and never connected.
//
// The world book states an age for each catalog person, and the story freezes
// that number when it starts, so a preset person's age at the story's beginning
// is committed data. The branch's formal clock counts the seconds a rest, a
// training stretch, a craft batch or a journey really spent, so the interval
// since that beginning is committed data too. Nothing read the two together,
// which meant a decade of seclusion advanced the clock and left every person in
// the story exactly the age they were written at. The narrator, given a scene
// that says ten years have passed and a cast that says nobody aged, wrote a
// decade in which nobody grew older.
//
// The join lives here because this is the only package that can see both: the
// age is on the frozen catalog, the clock is on the branch projection, and the
// cast is in the character package — which imports neither of the other two and
// must not start. Passing a number in keeps the year length out of it.
//
// Two boundaries are deliberate. The first is that this file computes an age
// and never a death: the confirmed lifespan table is bound elsewhere
// (internal/interactive/story_lifespan.go), and this cast block only states how
// old each person is. Keeping the two apart is what stops an age display from
// quietly becoming an ending. The second is the year length. §2 says
// 修炼稿的"规则月＝30天、规则年＝360天"是计算单位，不能未经转换直接当作另一套生日
// or 历法, so the year comes from the calendar the story froze when it started
// and falls back to DaysPerRuleYear only for stories created before calendars
// existed. Age still carries its own uncertainty: it is whole years since the
// story began, not a birthday, so nobody is aged by a month.

// castAges reads each person's established age and the branch's spent interval.
//
// It returns an empty map rather than an error when either half is missing. A
// story with no frozen catalog predates freezing, and a branch that never spent
// an interval has nothing to add; in both cases the cast block simply states no
// age, which is the honest reading and not a zero.
func castAges(storyCtx interactive.StoryContext, state character.State, world interactive.WorldState) map[string]character.AgeFact {
	if world.Clock.Seconds <= 0 {
		// No formal interval has been spent on this branch. The established age
		// is still true, but printing it alone would read as an age that never
		// moves, so the block keeps its older shape.
		return nil
	}
	frozen := frozenCatalog(storyCtx.Meta.PackCatalog)
	if frozen == nil {
		return nil
	}
	established := make(map[string]int, len(frozen.People))
	for _, person := range frozen.People {
		if person.Age > 0 {
			established[person.SourceKey] = person.Age
		}
	}
	protagonistAge := protagonistEstablishedAge(storyCtx.Meta.Protagonist)
	if len(established) == 0 && protagonistAge == 0 {
		return nil
	}
	years := spentWholeYears(storyCtx.Meta, world.Clock.Seconds)
	if years == 0 {
		return nil
	}

	ages := make(map[string]character.AgeFact, len(state.Profiles))
	for _, profile := range state.Profiles {
		key := strings.TrimSpace(profile.SourceKey)
		if key == "" {
			// A person the story invented has no established age. §2 is explicit
			// that 未知生日不由AI猜成精确日期, so this leaves them without one rather
			// than deriving an age from the clock.
			continue
		}
		base, ok := established[key]
		if !ok || base <= 0 {
			continue
		}
		ages[profile.ID] = character.AgeFact{Years: base + years, Established: base}
	}
	return ages
}

// spentWholeYears converts a formal interval to whole years under the calendar
// this story froze.
//
// The floor is the point: §2 asks for 显示整岁不丢失底层已过时长, so an interval
// short of a year must not advance anybody's age. The interval itself stays on
// the clock, which is what keeps the remainder from being lost.
func spentWholeYears(meta interactive.StoryMeta, seconds int64) int {
	return interactive.WholeYearsAt(meta, seconds)
}

// protagonistEstablishedAge reads the age the creation page established for the
// protagonist.
//
// The protagonist's age never reaches the actor's state: identityStateOverlay's
// list is closed on purpose, and age is not on it. It survives in the
// protagonist snapshot's profile prose, which the creation path composes from
// the resolved identity, so the established age is parsed back from the same
// authored sentence the world book uses. A protagonist started from a lore
// character or a template has no parsed age and gets none.
func protagonistEstablishedAge(protagonist interactive.StoryProtagonist) int {
	if protagonist.Mode != interactive.StoryProtagonistModeCustom {
		return 0
	}
	return storypack.ParseAge(protagonist.Profile)
}

// protagonistAgeFact is the same reading for the protagonist alone, which the
// scene block states because the protagonist is not in the cast roster.
func protagonistAgeFact(storyCtx interactive.StoryContext, world interactive.WorldState) *character.AgeFact {
	if world.Clock.Seconds <= 0 {
		return nil
	}
	established := protagonistEstablishedAge(storyCtx.Meta.Protagonist)
	if established <= 0 {
		return nil
	}
	years := spentWholeYears(storyCtx.Meta, world.Clock.Seconds)
	if years == 0 {
		return nil
	}
	return &character.AgeFact{Years: established + years, Established: established}
}
