package character

import (
	"fmt"
	"sort"
	"strings"
)

// MessageStatus tracks what the player did with an incoming message. Receiving
// a message never accepts an arrangement: only replying does, and that is a
// separate committed exchange.
type MessageStatus string

const (
	// MessageUnread is delivered and not yet looked at.
	MessageUnread MessageStatus = "unread"
	// MessageRead was opened. Opening it changes nothing in the world.
	MessageRead MessageStatus = "read"
	// MessageIgnored was dismissed without replying. The character sent it; the
	// character's own expectations are unaffected by the player's UI state.
	MessageIgnored MessageStatus = "ignored"
	// MessageHandled was answered or otherwise acted on in a committed exchange.
	MessageHandled MessageStatus = "handled"
)

func (s MessageStatus) Valid() bool {
	switch s {
	case MessageUnread, MessageRead, MessageIgnored, MessageHandled:
		return true
	default:
		return false
	}
}

// Message is an incoming message a character sent on their own initiative.
//
// The base build presents these as character messages; a cultivation pack shows
// them as 传音 and another pack uses whatever channel it declares. The channel
// name is presentation, so it lives on the pack, not here.
type Message struct {
	ID          string `json:"id"`
	StoryID     string `json:"story_id"`
	BranchID    string `json:"branch_id"`
	CharacterID string `json:"character_id"`
	Text        string `json:"text"`

	// Basis is why this message exists at all: the committed event, appointment
	// or circumstance that caused it. A message without a basis would be the
	// character contacting the player for no reason, which is exactly the
	// failure mode of messaging on a fixed schedule.
	Basis string `json:"basis"`
	// BasisEventID and BasisTurnID pin that reason to the journal.
	BasisEventID string `json:"basis_event_id,omitempty"`
	BasisTurnID  string `json:"basis_turn_id,omitempty"`

	// WorldTime is the original in-world send time. It is frozen at commit and
	// never rewritten, so an old message still reports when it was sent.
	WorldTime string `json:"world_time"`
	// Deadline is when the message's matter lapses, if it has one. It is the
	// story's own words, kept so the player reads what the character actually
	// said.
	Deadline string `json:"deadline,omitempty"`
	// DeadlineSeconds is the same limit as an absolute instant on the branch's
	// formal clock, stamped at commit from a deadline that stated an interval.
	// Zero means the label could not be read as an interval — an absolute
	// calendar position such as 第3日 酉时, or a label with no time in it — and
	// the record stays unbound rather than being guessed into expiring.
	//
	// It is stored rather than recomputed from WorldTime, because WorldTime is
	// display text and the clock is the only thing the arithmetic may read. A
	// fork inherits the instant its parent had; it never re-stamps.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`

	Status MessageStatus `json:"status"`
	// ReadWorldTime is the world time at which the player opened it. Reading
	// does not consume world time, so this equals the send time unless the story
	// moved on first.
	ReadWorldTime string `json:"read_world_time,omitempty"`

	SourceEventID string `json:"source_event_id,omitempty"`
}

func (m Message) Normalize() Message {
	m.ID = strings.TrimSpace(m.ID)
	m.StoryID = strings.TrimSpace(m.StoryID)
	m.BranchID = strings.TrimSpace(m.BranchID)
	m.CharacterID = strings.TrimSpace(m.CharacterID)
	m.Text = strings.TrimSpace(m.Text)
	m.Basis = strings.TrimSpace(m.Basis)
	m.BasisEventID = strings.TrimSpace(m.BasisEventID)
	m.BasisTurnID = strings.TrimSpace(m.BasisTurnID)
	m.WorldTime = strings.TrimSpace(m.WorldTime)
	m.Deadline = strings.TrimSpace(m.Deadline)
	if m.DeadlineSeconds < 0 {
		m.DeadlineSeconds = 0
	}
	m.ReadWorldTime = strings.TrimSpace(m.ReadWorldTime)
	m.SourceEventID = strings.TrimSpace(m.SourceEventID)
	if m.Status == "" {
		m.Status = MessageUnread
	}
	return m
}

func (m Message) Validate() error {
	m = m.Normalize()
	if m.ID == "" || m.CharacterID == "" {
		return fmt.Errorf("character message requires an id and a character id")
	}
	if m.StoryID == "" || m.BranchID == "" {
		return fmt.Errorf("character message %s requires a story and branch", m.ID)
	}
	if m.Text == "" {
		return fmt.Errorf("character message %s has no text", m.ID)
	}
	if m.Basis == "" {
		return fmt.Errorf("character message %s has no stated basis; messages are caused, not scheduled", m.ID)
	}
	if m.WorldTime == "" {
		return fmt.Errorf("character message %s has no world time", m.ID)
	}
	if !m.Status.Valid() {
		return fmt.Errorf("character message %s has an invalid status: %q", m.ID, m.Status)
	}
	return nil
}

// Lapsed reports whether the message's matter has expired at a given world
// time. World time advances only through committed story turns, so a player who
// leaves the window open, or closes the program entirely, does not age it.
//
// A message whose deadline was bound to the branch clock is decided by that
// instant and nothing else. Only an unbound one falls back to reading the text,
// which is the honest answer for a label the rules cannot place on the clock.
func (m Message) Lapsed(currentWorldTime string) bool {
	return m.LapsedAt(currentWorldTime, 0)
}

// LapsedAt is Lapsed with the branch's formal instant supplied, so a message
// stamped against the clock is decided by arithmetic rather than by its label.
//
// nowSeconds of zero means the caller has no clock reading — either the branch
// has never spent formal time, or this record predates the clock — and then the
// label is all there is. It falls back to the text comparison rather than
// reporting "not lapsed": the stamp was derived from that same label, so
// declining to decide would make a message the label already orders as past
// silently live again. That is what keeps an old save readable.
func (m Message) LapsedAt(currentWorldTime string, nowSeconds int64) bool {
	m = m.Normalize()
	if m.DeadlineSeconds > 0 && nowSeconds > 0 {
		return nowSeconds > m.DeadlineSeconds
	}
	currentWorldTime = strings.TrimSpace(currentWorldTime)
	if m.Deadline == "" || currentWorldTime == "" {
		return false
	}
	return compareWorldTime(currentWorldTime, m.Deadline) > 0
}

// compareWorldTime orders two world-time values. Stories author their own time
// labels, so this reads the two things a label can state unambiguously — the
// day number and, within that day, which 时辰 it is — and treats everything else
// as unordered. It never returns an error and never falls back to string order:
// an unorderable pair compares equal, which keeps an unknown format from
// silently expiring every message.
//
// The day is the first integer in the label ("第3日" and "Day 3, dawn" both mean
// day 3). Within one day the 时辰 name is an exact ordinal, not a guess: 子丑寅卯
// 辰巳午未申酉戌亥 is a fixed sequence, and 酉时 precedes 戌时 by definition. The
// comparison used to stop at the day, so a thread due at 第3日 酉时 was still
// live at 第3日 戌时 and the deadline silently never came. That is the failure
// this second key exists to answer.
//
// Two cases stay unordered on purpose, because the labels do not state an order:
//
//   - A day with no 时辰 on one side. "第3日" as a deadline most likely means
//     the day as a whole, and reading it as midnight would expire a thread hours
//     before the prose said it would.
//
//   - Two labels with no digits at all. They share no scale.
//
// 子时 straddles midnight, so a label naming it is ordered within its own named
// day rather than being split across two. That is how the story writes it, and
// inventing a rollover would make 第3日 子时 later than 第4日 丑时.
//
// The string fallback that used to be here did not do any of that. Two labels
// with no digits were ordered by code point, and in the acceptance story that ran
// "测灵台开张当日，入夜" (a promise's own creation time) past "今日日落前" (its
// deadline), because 测 sorts after 今. Every such thread was therefore filed as
// already lapsed the moment it was created — the cast block showed it under
// "Lapsed" with the message hook that only appears under "Unfinished", so the
// one place a character had a concrete reason to write was never reachable, and
// no message was ever produced. Comparing equal is the honest answer: without a
// shared numeric scale these labels say nothing about order, and an unknown
// order must not be read as expiry.
func compareWorldTime(a, b string) int {
	aNumber, aOK := firstInt(a)
	bNumber, bOK := firstInt(b)
	if aOK && bOK {
		switch {
		case aNumber < bNumber:
			return -1
		case aNumber > bNumber:
			return 1
		}
		// Same day. The 时辰 name is the only finer key the label states.
		aHour, aHasHour := shichenOrdinal(a)
		bHour, bHasHour := shichenOrdinal(b)
		if !aHasHour || !bHasHour {
			return 0
		}
		switch {
		case aHour < bHour:
			return -1
		case aHour > bHour:
			return 1
		}
		return 0
	}
	return 0
}

// shichenOrdinal reads which 时辰 a label names, as its position in the fixed
// sequence 子丑寅卯辰巳午未申酉戌亥 (子 = 0). It reports false when the label names
// no 时辰, which keeps a bare day label from being read as midnight.
//
// The character must be written as "X时" — 酉时, 戌时 — rather than appearing
// alone. A single 酉 in a place name or a person's name is not a time, and
// reading it as one would order two labels by an accident of vocabulary.
func shichenOrdinal(label string) (int, bool) {
	const names = "子丑寅卯辰巳午未申酉戌亥"
	runes := []rune(strings.TrimSpace(label))
	for index, r := range runes {
		position := strings.IndexRune(names, r)
		if position < 0 {
			continue
		}
		if index+1 < len(runes) && (runes[index+1] == '时' || runes[index+1] == '時') {
			// IndexRune reports a byte offset, and every 时辰 character is three
			// bytes, so 酉 comes back as 27 rather than 9. Ordering survives that
			// — byte offsets sort the same way rune positions do — but placing a
			// label at an instant does not: 酉时 is the tenth slot, not the
			// twenty-eighth. Convert to the ordinal before returning it.
			return len([]rune(names[:position])), true
		}
	}
	return 0, false
}

// firstInt extracts the first run of digits anywhere in a label, so "第3日" and
// "Day 3, dawn" both order by 3. Scanning the whole label rather than requiring
// a leading digit is what keeps "第10日" after "第9日" instead of between "第1"
// and "第2".
func firstInt(value string) (int64, bool) {
	runes := []rune(strings.TrimSpace(value))
	start := -1
	for index, r := range runes {
		if r >= '0' && r <= '9' {
			start = index
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	var result int64
	for _, r := range runes[start:] {
		if r < '0' || r > '9' {
			break
		}
		result = result*10 + int64(r-'0')
	}
	return result, true
}

// MessageStatusChange is a player action on one delivered message: read it,
// ignore it, or record that it was dealt with. It is a durable record because
// the player's own state has to survive a restart, but it is never an acceptance
// of what the message proposed — only a committed reply can accept anything.
type MessageStatusChange struct {
	MessageID string        `json:"message_id"`
	Status    MessageStatus `json:"status"`
	// WorldTime is the world time at which the player acted. Reading consumes no
	// world time, so this is usually the send time.
	WorldTime string `json:"world_time,omitempty"`
}

func (c MessageStatusChange) Normalize() MessageStatusChange {
	c.MessageID = strings.TrimSpace(c.MessageID)
	c.WorldTime = strings.TrimSpace(c.WorldTime)
	return c
}

func (c MessageStatusChange) Validate() error {
	c = c.Normalize()
	if c.MessageID == "" {
		return fmt.Errorf("message status change requires a message id")
	}
	if !c.Status.Valid() {
		return fmt.Errorf("message status change %s has an invalid status: %q", c.MessageID, c.Status)
	}
	return nil
}

// ErrMessageNotFound means a status change names a message this state does not
// hold. A rebuild treats it as a stale write rather than a corrupt journal.
var ErrMessageNotFound = fmt.Errorf("character message not found")

// ApplyMessageStatus applies one player action to a delivered message. A status
// that moves backwards (handled back to unread) is refused, so a replayed or
// out-of-order request cannot resurrect a message the player already dealt with.
func (s *State) ApplyMessageStatus(change MessageStatusChange) error {
	change = change.Normalize()
	if err := change.Validate(); err != nil {
		return err
	}
	for index := range s.Messages {
		message := s.Messages[index].Normalize()
		if message.ID != change.MessageID {
			continue
		}
		if messageStatusRank(change.Status) < messageStatusRank(message.Status) {
			return fmt.Errorf("message %s cannot move from %q back to %q", message.ID, message.Status, change.Status)
		}
		s.Messages[index].Status = change.Status
		if change.Status == MessageRead || change.Status == MessageHandled {
			s.Messages[index].ReadWorldTime = change.WorldTime
		}
		return nil
	}
	return fmt.Errorf("%w: %s", ErrMessageNotFound, change.MessageID)
}

// messageStatusRank orders the player's own progress through a message, so a
// stale request cannot undo a newer one.
func messageStatusRank(status MessageStatus) int {
	switch status {
	case MessageUnread:
		return 0
	case MessageRead:
		return 1
	case MessageIgnored:
		return 2
	case MessageHandled:
		return 3
	default:
		return -1
	}
}

// MessageIsNewer orders two messages newest first. World time leads because it
// is when the character wrote, which is what the player is being told; the id
// breaks ties so the order is stable across a rebuild instead of depending on
// the order the journal happened to be replayed in.
func MessageIsNewer(a, b Message) bool {
	a, b = a.Normalize(), b.Normalize()
	switch compareWorldTime(a.WorldTime, b.WorldTime) {
	case 1:
		return true
	case -1:
		return false
	default:
		return a.ID > b.ID
	}
}

// PendingMessages returns the messages the player has not dealt with, newest
// first, marking which have lapsed. The stage shows a quiet hint from this, so
// it must not include anything already handled or ignored.
func PendingMessages(messages []Message, currentWorldTime string) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		message = message.Normalize()
		switch message.Status {
		case MessageUnread, MessageRead:
			out = append(out, message)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return MessageIsNewer(out[i], out[j]) })
	return out
}
