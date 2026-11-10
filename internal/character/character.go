// Package character holds the game's shared character identity and memory
// model. It is deliberately pure: no store, no model calls, no file access.
//
// One process serves every character in every story. Nothing here is bound to
// a particular NPC, so adding a character costs data rather than a resident
// agent, and the main story, the character window and incoming messages all
// prepare context through the same code.
package character

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Channel is how an exchange reached the player. It decides who could overhear
// it, which is the whole basis of the audience rules in memory.go: a private
// channel is private because of where it happened, not because it is stored
// separately.
type Channel string

const (
	// ChannelScene is speech inside the main story, witnessed by everyone the
	// narrative places in the scene.
	ChannelScene Channel = "scene"
	// ChannelPrivate is a one-to-one exchange in the character window.
	ChannelPrivate Channel = "private"
	// ChannelMessage is an incoming message the character sent on their own
	// initiative. It reaches the player alone unless the narrative says
	// otherwise.
	ChannelMessage Channel = "message"
	// ChannelGroup is speech inside a group conversation. Its audience is the
	// group's members as they stood when the line was committed, which is why a
	// later joiner cannot remember it: the audience was captured, not
	// reconstructed from who belongs now.
	ChannelGroup Channel = "group"
)

func (c Channel) Valid() bool {
	switch c {
	case ChannelScene, ChannelPrivate, ChannelMessage, ChannelGroup:
		return true
	default:
		return false
	}
}

// Speaker is who produced one side of an exchange.
type Speaker string

const (
	SpeakerPlayer    Speaker = "player"
	SpeakerCharacter Speaker = "character"
)

// Exchange is one committed utterance. It is the unit the main story and the
// character window share: a line spoken in the story is the same record the
// window shows, and a line spoken in the window is the same record the next
// story turn reads.
type Exchange struct {
	ID      string `json:"id"`
	StoryID string `json:"story_id"`
	// BranchID scopes the exchange to one branch. Two branches of the same story
	// must not see each other's conversations.
	BranchID string `json:"branch_id"`
	// CharacterID is the stable identity of the character spoken to or by. A group
	// line is filed under its group instead, so this is empty for one and AuthorID
	// names whoever actually spoke.
	CharacterID string  `json:"character_id"`
	Speaker     Speaker `json:"speaker"`
	Channel     Channel `json:"channel"`
	// GroupID is the group a line was spoken in. It is set exactly when the
	// channel is ChannelGroup, and it is what keeps one group's conversation out
	// of another's.
	GroupID string `json:"group_id,omitempty"`
	// AuthorID is the character who actually produced a group line. It is empty
	// for a line the player spoke. Keeping it apart from CharacterID is what lets
	// a group record carry its real author without pretending the group is a
	// character.
	AuthorID string `json:"author_id,omitempty"`
	// AddressedTo names the one person this line was actually directed at, when
	// the prose directed it at someone in particular. CharacterID cannot answer
	// that for a character's own line: there it names the speaker, so a threat
	// made to a bystander and a threat made to the protagonist would otherwise
	// read identically. It is empty for a line spoken to the room.
	AddressedTo string `json:"addressed_to,omitempty"`
	Text        string `json:"text"`
	// WorldTime is the in-fiction time this was said. It is frozen at commit, so
	// re-reading an old conversation reports when it happened rather than when
	// the player happened to open the window.
	WorldTime string `json:"world_time"`
	// TurnID is the main-story turn this exchange belongs to, when it came from
	// the story. It is empty for window-only exchanges, which deliberately do
	// not consume a story turn number.
	TurnID string `json:"turn_id,omitempty"`
	// SceneNode identifies the story scene the exchange happened in, so the
	// window can report the current scene and continuity can be restored.
	SceneNode string `json:"scene_node,omitempty"`
	// Audience lists the characters who could perceive this exchange. The
	// speaker is always included. It is captured at commit because the cast of a
	// scene cannot be reconstructed later.
	Audience []string `json:"audience,omitempty"`
	// SourceEventID is the canonical journal event this exchange was projected
	// from, so a saved game can be replayed without duplicating it.
	SourceEventID string `json:"source_event_id,omitempty"`
	// CooperationDecision records only this speaker's scoped answer. It never
	// creates work or a plan without the story's separate validation and commit.
	CooperationDecision   *CooperationDecision   `json:"cooperation_decision,omitempty"`
	ParticipationDecision *ParticipationDecision `json:"participation_decision,omitempty"`
}

// maxExchangeTextRunes bounds one utterance. A conversation line is a line,
// not a chapter; the story turn carries the long-form prose.
const maxExchangeTextRunes = 4000

func (e Exchange) Normalize() Exchange {
	e.ID = strings.TrimSpace(e.ID)
	e.StoryID = strings.TrimSpace(e.StoryID)
	e.BranchID = strings.TrimSpace(e.BranchID)
	e.CharacterID = strings.TrimSpace(e.CharacterID)
	e.GroupID = strings.TrimSpace(e.GroupID)
	e.AuthorID = strings.TrimSpace(e.AuthorID)
	e.AddressedTo = strings.TrimSpace(e.AddressedTo)
	e.Text = strings.TrimSpace(e.Text)
	e.WorldTime = strings.TrimSpace(e.WorldTime)
	e.TurnID = strings.TrimSpace(e.TurnID)
	e.SceneNode = strings.TrimSpace(e.SceneNode)
	e.SourceEventID = strings.TrimSpace(e.SourceEventID)
	e.Audience = normalizeIDs(e.Audience)
	if e.CooperationDecision != nil {
		decision := e.CooperationDecision.Normalize()
		e.CooperationDecision = &decision
	}
	if e.ParticipationDecision != nil {
		decision := e.ParticipationDecision.Normalize()
		e.ParticipationDecision = &decision
	}
	return e
}

func (e Exchange) Validate() error {
	e = e.Normalize()
	if e.ID == "" {
		return fmt.Errorf("character exchange requires an id")
	}
	if e.StoryID == "" || e.BranchID == "" {
		return fmt.Errorf("character exchange requires a story and branch")
	}
	// A group line is filed under its group rather than under a character, and
	// carries its real author separately. Requiring a character id here would
	// force the group to be recorded as if it were a person.
	if e.Channel == ChannelGroup {
		if e.GroupID == "" {
			return fmt.Errorf("a group exchange requires a group id")
		}
		if e.Speaker == SpeakerCharacter && e.AuthorID == "" {
			return fmt.Errorf("a group exchange by a character requires its author")
		}
	} else if e.CharacterID == "" {
		return fmt.Errorf("character exchange requires a character id")
	}
	switch e.Speaker {
	case SpeakerPlayer, SpeakerCharacter:
	default:
		return fmt.Errorf("character exchange has an invalid speaker: %q", e.Speaker)
	}
	if !e.Channel.Valid() {
		return fmt.Errorf("character exchange has an invalid channel: %q", e.Channel)
	}
	if e.Text == "" {
		return fmt.Errorf("character exchange has no text")
	}
	if utf8.RuneCountInString(e.Text) > maxExchangeTextRunes {
		return fmt.Errorf("character exchange exceeds %d characters", maxExchangeTextRunes)
	}
	if e.WorldTime == "" {
		return fmt.Errorf("character exchange requires a world time")
	}
	if e.Speaker == SpeakerCharacter && len(e.Audience) == 0 {
		return fmt.Errorf("character speech requires an audience")
	}
	if e.ParticipationDecision != nil {
		if e.Speaker != SpeakerCharacter || e.Channel != ChannelScene || e.AddressedTo == "" {
			return fmt.Errorf("a participation decision must be the character's own directed scene response")
		}
		if err := e.ParticipationDecision.Validate(); err != nil {
			return err
		}
	}
	if e.CooperationDecision != nil {
		if e.Speaker != SpeakerCharacter || (e.Channel != ChannelScene && e.Channel != ChannelPrivate) {
			return fmt.Errorf("a cooperation decision must be the character's own scene or private reply")
		}
		if err := e.CooperationDecision.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Audience returns the perceivers of this exchange, always including the
// character it is filed under. A record whose audience omitted its own
// character would let the character forget what they themselves said.
//
// A group line is filed under its group, so the group's id is not a person and
// is not added here. Its real author is added instead: whoever spoke was in the
// room by definition, even if a later leave has since been recorded.
func (e Exchange) AudienceMembers() []string {
	if e.Channel == ChannelGroup {
		return normalizeIDs(append(append([]string{}, e.Audience...), e.AuthorID))
	}
	return normalizeIDs(append(append([]string{}, e.Audience...), e.CharacterID))
}

func normalizeIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// ContainsID reports whether a normalized ID list already holds a value.
func ContainsID(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
