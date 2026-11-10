package interactive

import (
	"fmt"
	"strings"

	"ta/internal/storytime"
)

// StoryClockVersion identifies the shape of a branch's formal time axis.
//
// It is versioned because §三.2 of the night-shift brief allows this batch to
// establish a relative-second origin while the world's own calendar is still
// unbound free text. A later batch that registers a real calendar adapter must
// be able to tell one convention from the other rather than silently re-reading
// old numbers under new meaning.
const StoryClockVersion = 1

// StoryClock is a branch's single formal time axis: an elapsed-second count from
// a versioned per-branch origin.
//
// Three properties are deliberate.
//
//   - It is one clock, not two. §三.1 and §三.2 forbid the recovery rules from
//     advancing a time of their own and forbid maintaining a second advanceable
//     rule clock beside the story's own. Every activity that consumes in-fiction
//     time — resting now, cultivating and travelling later — moves this same
//     number, so there is exactly one answer to "how much time has passed".
//
//   - The origin is relative, not a date. The world's calendar is still an
//     unbound display string (storytime.ParseCalendarLabel refuses every label
//     because no adapter is registered), so guessing that 初七 means a
//     particular year and month would be inventing a fact. Anchor keeps the
//     original text purely so a reader can see what the story called this
//     moment; it is never parsed into the arithmetic.
//
//   - It is branch-scoped. A fork inherits the clock its parent had at the fork
//     point and from then on the two advance separately, the same way branch
//     state and character memory do.
type StoryClock struct {
	Version int `json:"version"`
	// Anchor is the story's own display text for the origin moment, such as
	// 三月初七清晨. It is a label, not an input to any calculation.
	Anchor string `json:"anchor,omitempty"`
	// Seconds is the elapsed seconds since the origin. It only ever moves
	// forward, and only by an interval an activity actually committed.
	Seconds int64 `json:"seconds"`
}

// NewStoryClock establishes a branch's origin from the story's current display
// text.
//
// A story that has never spent in-fiction time has no clock yet. This is what
// creates one, and it is why an old playtest save is not migrated: §三.2
// forbids back-filling history for stories that were played before the formal
// axis existed.
func NewStoryClock(anchor string) StoryClock {
	return StoryClock{Version: StoryClockVersion, Anchor: strings.TrimSpace(anchor)}
}

// Validate rejects a clock the arithmetic cannot honestly use.
func (clock StoryClock) Validate() error {
	if clock.Version != StoryClockVersion {
		return fmt.Errorf("故事时钟版本不支持 / unsupported story clock version: %d", clock.Version)
	}
	if clock.Seconds < 0 {
		return storytime.ErrNegativeTime
	}
	return nil
}

// Instant reports the clock's current formal instant.
//
// The returned value is an offset from this branch's own origin. It is
// comparable with another instant from the same branch and with nothing else,
// which is exactly what the rules need: §三.1 asks for one comparable
// second-level moment, not a calendar date.
func (clock StoryClock) Instant() storytime.Instant {
	return storytime.Instant(clock.Seconds)
}

// AdvanceStoryClock returns the clock after an interval that really elapsed.
//
// elapsedSeconds must be the interval an activity committed, never the host
// clock or time inferred from narration. An explicit player request to wait is
// committed as its own source-bound activity; the time between player turns is
// never counted.
func AdvanceStoryClock(clock StoryClock, elapsedSeconds int64) (StoryClock, error) {
	if err := clock.Validate(); err != nil {
		return StoryClock{}, err
	}
	next, err := clock.Instant().Add(storytime.Duration(elapsedSeconds))
	if err != nil {
		return StoryClock{}, err
	}
	clock.Seconds = int64(next)
	return clock, nil
}

// cloneStoryClock copies a clock so a projection cannot be mutated through an
// alias. A nil clock stays nil: a branch that has never spent formal time has
// no axis, and inventing a zero one would claim an origin nobody established.
func cloneStoryClock(clock *StoryClock) *StoryClock {
	if clock == nil {
		return nil
	}
	copied := *clock
	return &copied
}

// SnapshotClockSeconds is the branch's formal instant for a reader that holds a
// snapshot rather than the branch projection.
//
// It is zero when the story has never spent formal time, which is a real answer:
// a deadline stamped from it would be a guess, so nothing is stamped and an
// unbound record keeps its text.
func SnapshotClockSeconds(snapshot Snapshot) int64 {
	if snapshot.Clock == nil {
		return 0
	}
	return snapshot.Clock.Seconds
}

// applyTurnClockAdvance folds the backend-authored clock interval carried by a
// committed wait turn. A TurnResult can propose a duration, but only the
// interval written on the Turn changes the projection.
func applyTurnClockAdvance(branch *storyBranchProjection, turn TurnEvent) error {
	advance := turn.ClockAdvance
	if advance == nil {
		return nil
	}
	if strings.TrimSpace(advance.Action) != TurnWorldActionWait && strings.TrimSpace(advance.Action) != "adventure" && strings.TrimSpace(advance.Action) != TurnWorldActionFaction && strings.TrimSpace(advance.Action) != turnClockActionTechnique && strings.TrimSpace(advance.Action) != turnClockActionSkill {
		return fmt.Errorf("unsupported committed story clock action %q", advance.Action)
	}
	if strings.TrimSpace(advance.SourceText) == "" || !strings.Contains(turn.User, strings.TrimSpace(advance.SourceText)) {
		return fmt.Errorf("wait clock source does not occur in the committed player input")
	}
	if err := advance.Before.Validate(); err != nil {
		return fmt.Errorf("wait clock before is invalid: %w", err)
	}
	if err := advance.After.Validate(); err != nil {
		return fmt.Errorf("wait clock after is invalid: %w", err)
	}
	if branch.Clock == nil {
		if advance.Before.Seconds != 0 {
			return fmt.Errorf("first wait clock origin must start at zero seconds")
		}
	} else if advance.Before != *branch.Clock {
		return fmt.Errorf("wait clock before does not match the branch clock")
	}
	clockAfter, err := AdvanceStoryClock(advance.Before, advance.DurationSeconds)
	if err != nil {
		return fmt.Errorf("wait clock advance is invalid: %w", err)
	}
	if clockAfter != advance.After {
		return fmt.Errorf("wait clock after does not match its proposed duration")
	}
	branch.Clock = cloneStoryClock(&advance.After)
	return nil
}

// DescribeElapsed renders an elapsed interval the way §三.2 allows: natural
// wording such as "一日" rather than a fabricated precise calendar stamp.
//
// It is deliberately coarse. The formal number is exact and lives on the clock;
// what the prose says is a human summary of it, and claiming "三月初八辰时"
// would be inventing a calendar the world pack has not defined.
func DescribeElapsed(elapsedSeconds int64) string {
	if elapsedSeconds <= 0 {
		return ""
	}
	switch {
	case elapsedSeconds%(24*60*60) == 0:
		return fmt.Sprintf("%d日", elapsedSeconds/(24*60*60))
	case elapsedSeconds%(2*60*60) == 0:
		return fmt.Sprintf("%d时辰", elapsedSeconds/(2*60*60))
	case elapsedSeconds%(60*60) == 0:
		return fmt.Sprintf("%d小时", elapsedSeconds/(60*60))
	case elapsedSeconds%(15*60) == 0:
		return fmt.Sprintf("%d刻", elapsedSeconds/(15*60))
	case elapsedSeconds%60 == 0:
		return fmt.Sprintf("%d分钟", elapsedSeconds/60)
	default:
		return fmt.Sprintf("%d秒", elapsedSeconds)
	}
}
