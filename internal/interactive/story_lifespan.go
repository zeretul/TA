package interactive

import (
	"fmt"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/storytime"
)

// This file joins the confirmed lifespan table to a story's own clock, which is
// the one thing the table deliberately could not do on its own.
//
// cultivation.LifespanYears answers "how many years does this realm live". A
// branch's clock answers "how many seconds has this story spent". Neither can be
// compared with the other without a year length, which is why the table had no
// caller until a story could freeze a calendar.
//
// Three boundaries are deliberate, and each one is a rule the design states.
//
//   - Nothing is enforced without all four facts. A story with no frozen
//     calendar, a protagonist whose age the pack never stated, a state with no
//     realm field, or a realm the table has no row for all report "unknown" and
//     no 寿尽 is ever declared. TIME-08 is explicit that incomplete data must
//     withhold the result rather than invent a death, and §4 repeats it.
//
//   - Reaching the edge is not dying. The boundary is the instant the
//     protagonist's final year is complete; the stop node cuts a training or
//     resting interval there, so the story arrives at the edge and the narrator
//     is given the moment to describe. Death is the clock moving *past* it. §4
//     asks exactly this: 玩家即将跨过本人明确寿尽边界时，应在行动真正越界之前自然
//     提示处境，允许其决定出关或继续.
//
//   - The realm is read as it stands now, not as it was at creation. §3 says a
//     successful breakthrough adopts the new realm's total and keeps the age, so
//     a character who reaches 筑基 at 110 has 200 years in total and 90 left. A
//     check that read the frozen initial stage would keep killing 炼气 characters
//     at 120 no matter how far they had advanced.

// storyLifespanBoundary is the resolved edge for one protagonist on one branch.
//
// It exists as a value rather than as a lookup so the three places that need it —
// the training plan, the rest plan and the commit's own crossing check — all read
// the same number instead of re-deriving it and disagreeing.
type storyLifespanBoundary struct {
	// At is the formal instant, in seconds from the branch origin, at which the
	// protagonist's final year is complete. Reaching it is alive; passing it is
	// not.
	At int64
	// TotalYears is the confirmed table row that produced At, kept so a refusal
	// or a log line can name the number rather than only the instant.
	TotalYears int
	// EstablishedAge is the age the pack stated at story start.
	EstablishedAge int
	// AgeAtEdge is the whole age the protagonist has when At is reached.
	AgeAtEdge int
}

// storyLifespanBoundaryFor resolves the edge, or reports that it cannot be
// resolved.
//
// The second result is false for every incomplete case described above. A caller
// that receives false must not cut an interval and must not end a branch.
func storyLifespanBoundaryFor(meta StoryMeta, state map[string]any, actorID string) (storyLifespanBoundary, bool) {
	if meta.Calendar == nil {
		// The story never froze a calendar. Old stories are not migrated and no
		// default is applied: a year length they never had cannot be used to end
		// them.
		return storyLifespanBoundary{}, false
	}
	if err := meta.Calendar.Validate(); err != nil {
		return storyLifespanBoundary{}, false
	}
	established := meta.ProtagonistAge
	if established <= 0 {
		// The pack never stated an age for this protagonist. §2 forbids guessing
		// a birthday, and a lifespan counted from a guessed birth is a guessed
		// death.
		return storyLifespanBoundary{}, false
	}
	realm, ok := currentRealmOf(state, actorID)
	if !ok {
		return storyLifespanBoundary{}, false
	}
	total, ok := cultivation.LifespanYears(realm, cultivationRulesForMeta(meta))
	if !ok {
		// Unknown realms and known realms without a finite natural lifespan
		// have no automatic age-death boundary. Never borrow a lower limit.
		return storyLifespanBoundary{}, false
	}
	remaining := total - established
	if remaining < 0 {
		// The pack already states an age past this realm's confirmed total. That
		// is not incomplete data, so the edge is the origin itself: the character
		// has no confirmed years left and any interval at all crosses. Clamping to
		// zero rather than refusing keeps the design's rule — 不能因为人物重要或是
		// 情缘就默认永不衰老 — without inventing a second rule for the case.
		remaining = 0
	}
	perYear := meta.Calendar.SecondsPerYear()
	if perYear <= 0 {
		return storyLifespanBoundary{}, false
	}
	return storyLifespanBoundary{
		At:             int64(remaining) * perYear,
		TotalYears:     total,
		EstablishedAge: established,
		AgeAtEdge:      established + remaining,
	}, true
}

// currentRealmOf reads the actor's realm as the state stands.
//
// An unwritten realm field is read the way the settlement reads it — as the
// rules' own starting point, mortal — because that is what the schema itself
// would answer. A state with no realm field at all is a different fact: the story
// is not on the cultivation rules, and its characters have no lifespan table.
func currentRealmOf(state map[string]any, actorID string) (cultivation.Realm, bool) {
	raw, ok := actorStateFieldValue(state, actorID, xiuxianFieldRealm).(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", false
	}
	position, ok := cultivation.ParseStageKey(strings.TrimSpace(raw))
	if !ok {
		return "", false
	}
	return position.Realm, true
}

// lifespanStopNode reports the boundary as a training or rest stop.
//
// It is a decision node, not a resource node: what changes at the edge is what
// the player may still choose to do, and §4 asks for the situation to be put in
// front of them before the interval runs past it. The node is only produced when
// the edge lies strictly inside the interval, so an interval that ends before it
// is untouched and one that starts at it is left to the crossing check rather
// than cut to zero length.
func lifespanStopNode(boundary *storyLifespanBoundary, from, to storytime.Instant) (storytime.Node, bool) {
	if boundary == nil {
		return storytime.Node{}, false
	}
	at := storytime.Instant(boundary.At)
	if at <= from || at >= to {
		return storytime.Node{}, false
	}
	return storytime.Node{
		ID: storyLifespanNodeID,
		At: at,
		// The interval must not run past the edge on its own, and the player is
		// the one who decides whether to continue. A decision node both stops the
		// interval and states that a response is owed, which is precisely §4's
		// 允许其决定出关或继续.
		Kind:              storytime.DecisionNode,
		RequiresResponse:  true,
		PublicDescription: fmt.Sprintf("寿元将尽：按已确认寿元表，此身共有 %d 年，如今已至边缘。", boundary.TotalYears),
	}, true
}

// storyLifespanNodeID names the lifespan edge in an activity's node list.
//
// It carries the same story: prefix as every other backend-derived stop, so a
// caller cannot submit a node under it and mark the edge resolved.
const storyLifespanNodeID = cultivationStopNodePrefix + "lifespan"

// lifespanEnding reports the terminal outcome a turn's committed clock earns.
//
// It is called after every module has published its interval, with the clock the
// turn really left the branch on. A turn that stops at the edge returns nil: the
// story stands at the brink and the player still chooses. A turn whose interval
// carried the clock past the edge returns the ending, and the caller attaches it
// to the turn.
//
// The outcome is written by the backend rather than by the model because it is a
// deterministic consequence of two committed numbers. §4 forbids letting the AI
// declare a 生死 result for drama, and the model has no way to know the table.
func lifespanEnding(boundary *storyLifespanBoundary, clockSeconds int64, calendar *StoryCalendar, narrative string) *TerminalOutcome {
	if boundary == nil || clockSeconds <= boundary.At {
		return nil
	}
	elapsed := ""
	if calendar != nil && calendar.Validate() == nil {
		elapsed = calendar.DescribeElapsed(clockSeconds)
	}
	reason := fmt.Sprintf("按已确认寿元表，%d 岁为此境寿元上限；这一段时间走完，寿元已尽。", boundary.TotalYears)
	if elapsed != "" {
		reason = fmt.Sprintf("按已确认寿元表，%d 岁为此境寿元上限；自故事开始已过%s，寿元已尽。", boundary.TotalYears, elapsed)
	}
	return normalizeTerminalOutcomePointer(&TerminalOutcome{
		Terminal:              true,
		Type:                  TerminalOutcomeTypeLifespan,
		Reason:                reason,
		FinalNarrativeSummary: strings.TrimSpace(narrative),
	})
}

// TerminalOutcomeTypeLifespan is the ending type a natural 寿尽 writes.
//
// It is a plain string like every other type on TerminalOutcome; the web client
// only passes the value through, so adding one does not need a client change. It
// is named here rather than spelled inline at the one call site so a test can
// assert the same constant the game writes.
const TerminalOutcomeTypeLifespan = "寿尽"

// LifespanTerminalCandidate reports the 寿尽 ending this branch has already
// reached, or nil when it has not.
//
// It exists for the one caller that runs before the model writes: the turn check
// is where the model learns that this turn is an ending, because
// terminal_candidate is the only channel that tells it so — and a terminal turn
// must submit empty choices. Declaring the ending at commit time instead would
// leave the model writing an ordinary turn and the backend attaching an ending
// to prose that never described one.
//
// The comparison is strict, and that is the design's rule rather than an
// off-by-one. §4 asks for 玩家即将跨过本人明确寿尽边界时，应在行动真正越界之前自然提示
// 处境，允许其决定出关或继续: a branch standing exactly at the edge is alive and
// owes the player a choice, and only a clock that has moved past it is 寿尽.
// Callers that need the instant itself read the boundary rather than this
// verdict, which is what the journey planner does when it cuts a stretch.
func LifespanTerminalCandidate(meta StoryMeta, state map[string]any, clockSeconds int64) *TerminalCandidate {
	boundary, ok := storyLifespanBoundaryFor(meta, state, DefaultActorID)
	if !ok || clockSeconds <= boundary.At {
		return nil
	}
	reason := fmt.Sprintf("按已确认寿元表，%d 岁为此境寿元上限；这一段寿元已经走完。", boundary.TotalYears)
	return &TerminalCandidate{
		Type:   TerminalOutcomeTypeLifespan,
		Reason: reason,
	}
}

// LifespanEdge is where the protagonist's confirmed lifespan edge lies relative
// to a branch's formal clock, in the shape a context block can state.
//
// It exists for the narrator, not for the rules: the rules already have the
// boundary as a plain instant and cut intervals at it. What the narrator needs is
// the same fact put into numbers a scene can say — how many years this realm has,
// how old the character was when the story began, and how much formal time is
// left — because §4 asks that 玩家即将跨过本人明确寿尽边界时，应在行动真正越界之前
// 自然提示处境. A model that is told the ending only after the clock passed it can
// write neither the warning nor the choice.
type LifespanEdge struct {
	// NoFixedLimit is an explicit supported rule, not missing lifespan data.
	NoFixedLimit bool
	// TotalYears is the confirmed table row for the realm the character holds now.
	TotalYears int
	// EstablishedAge is the age the pack stated at story start.
	EstablishedAge int
	// AgeAtEdge is the whole age the character has when the edge is reached.
	AgeAtEdge int
	// CurrentAge is the whole age the character has now: the established age plus
	// the whole years the clock has really spent.
	//
	// It is stated rather than derived from TotalYears minus RemainingYears because
	// both of those floor, and a partial year floors them in opposite directions.
	// A character eleven months into a year would read one year older than they
	// are, and the scene block would contradict the cast block's own age.
	CurrentAge int
	// At is the instant the edge falls on, on the branch's own axis.
	At int64
	// RemainingYears is the whole years of formal time left before At. It floors,
	// so a character with eleven months left reports 0 rather than 1; the caller
	// says "less than a year" rather than claiming the edge is here.
	RemainingYears int
	// Reached reports that the clock stands at or past At. Standing exactly on the
	// edge is still alive — see LifespanTerminalCandidate for why the crossing is
	// strict.
	Reached bool
}

// LifespanEdgeFor reports the protagonist's confirmed lifespan edge, or nil when
// the story cannot answer.
//
// Nil is the same "unknown" every other lifespan question returns: no frozen
// calendar, no stated age, no readable realm, or a realm the table does not list.
// TIME-08 forbids inventing a death from incomplete data, and a context block is
// not a place to start guessing either.
func LifespanEdgeFor(meta StoryMeta, state map[string]any, clockSeconds int64) *LifespanEdge {
	if realm, ok := currentRealmOf(state, DefaultActorID); ok {
		if rule, known := cultivation.LifespanRuleFor(realm, cultivationRulesForMeta(meta)); known && rule.NoFixedLimit {
			edge := &LifespanEdge{NoFixedLimit: true, EstablishedAge: meta.ProtagonistAge}
			if meta.ProtagonistAge > 0 {
				edge.CurrentAge = meta.ProtagonistAge + WholeYearsAt(meta, clockSeconds)
			}
			return edge
		}
	}
	boundary, ok := storyLifespanBoundaryFor(meta, state, DefaultActorID)
	if !ok {
		return nil
	}
	edge := &LifespanEdge{
		TotalYears:     boundary.TotalYears,
		EstablishedAge: boundary.EstablishedAge,
		AgeAtEdge:      boundary.AgeAtEdge,
		At:             boundary.At,
		Reached:        clockSeconds >= boundary.At,
	}
	if meta.Calendar != nil {
		if perYear := meta.Calendar.SecondsPerYear(); perYear > 0 {
			edge.CurrentAge = boundary.EstablishedAge + int(clockSeconds/perYear)
			remaining := boundary.At - clockSeconds
			if remaining < 0 {
				remaining = 0
			}
			edge.RemainingYears = int(remaining / perYear)
		}
	}
	return edge
}

// lifespanBoundary is storyLifespanBoundaryFor as a pointer, which is the shape
// the interval planners take: nil reads as "this story cannot answer", and a
// caller that has to pass the answer through three functions should not have to
// carry a separate boolean.
func lifespanBoundary(meta StoryMeta, state map[string]any, actorID string) *storyLifespanBoundary {
	if strings.TrimSpace(actorID) == "" {
		actorID = DefaultActorID
	}
	boundary, ok := storyLifespanBoundaryFor(meta, state, actorID)
	if !ok {
		return nil
	}
	return &boundary
}

// wholeYearsAt converts an instant to whole years under a calendar.
//
// The floor is the point: §2 asks for 显示整岁不丢失底层已过时长, so an interval
// short of a year must not advance the age, and the remainder stays on the clock.
func wholeYearsAt(calendar StoryCalendar, clockSeconds int64) int {
	if clockSeconds <= 0 {
		return 0
	}
	perYear := calendar.SecondsPerYear()
	if perYear <= 0 {
		return 0
	}
	return int(clockSeconds / perYear)
}

// WholeYearsAt converts an instant to whole years under the calendar this story
// froze, and under the rule year for a story that froze none.
//
// The fallback is not a silent default calendar. A story created before the
// calendar existed still has an age that follows its clock, and the game already
// reads 闭关十年 as DaysPerRuleYear; refusing to age those stories would be a
// regression dressed as caution. What the fallback must never do is stand in for
// a lifespan — storyLifespanBoundaryFor reads meta.Calendar directly and reports
// unknown without one, so an unfrozen story ages but never dies of old age.
func WholeYearsAt(meta StoryMeta, clockSeconds int64) int {
	if clockSeconds <= 0 {
		return 0
	}
	if meta.Calendar != nil {
		if err := meta.Calendar.Validate(); err == nil {
			return wholeYearsAt(*meta.Calendar, clockSeconds)
		}
	}
	perYear := int64(DaysPerRuleYear) * StoryCalendarHoursPerDay * 60 * 60
	if perYear <= 0 {
		return 0
	}
	return int(clockSeconds / perYear)
}
