package character

import (
	"fmt"
	"sort"
	"strings"
)

// This file holds the activity record for the intimacy window.
//
// It is not a second character state. An activity is one more fact about people
// this branch already knows: who was invited, what each of them actually
// answered, and which of them the player confirmed into the room. The
// conversation itself is filed as ordinary group exchanges, so the same
// audience rule that keeps one group's talk out of another's memory applies
// here unchanged.
//
// What the type deliberately does not hold: any reward. §5 and §15 of the
// design close the cultivation-reward path for good, so there is no field for a
// multiplier, a gain, or a pending bonus that a later build could switch on.

// IntimacyPhase is where one activity stands. The phases are the design's own:
// choosing members, inviting, ready to start, in progress, ended.
type IntimacyPhase string

const (
	// IntimacySelecting is the player assembling a candidate list. Nothing has
	// been said to anyone yet, so nobody has consented.
	IntimacySelecting IntimacyPhase = "selecting"
	// IntimacyInviting has at least one invitation outstanding.
	IntimacyInviting IntimacyPhase = "inviting"
	// IntimacyReady has at least one actual acceptance and is waiting for the
	// player to confirm the roster. The confirmation is the player's act, never
	// the model's, and it is required even when everyone accepted.
	IntimacyReady IntimacyPhase = "ready"
	// IntimacyActive is an exchange in progress.
	IntimacyActive IntimacyPhase = "active"
	// IntimacyEnded is over. Its exchanges stay readable.
	IntimacyEnded IntimacyPhase = "ended"
)

func (p IntimacyPhase) Valid() bool {
	switch p {
	case IntimacySelecting, IntimacyInviting, IntimacyReady, IntimacyActive, IntimacyEnded:
		return true
	default:
		return false
	}
}

// IntimacyResponseStatus is one invited person's actual answer. A technical
// failure is not an answer and has no value here: the caller reports it as an
// error, so a timeout can never be recorded as a refusal.
type IntimacyResponseStatus string

const (
	// IntimacyPending means the person has not answered yet. It is not a yes.
	IntimacyPending IntimacyResponseStatus = "pending"
	// IntimacyAccepted is an unqualified yes.
	IntimacyAccepted IntimacyResponseStatus = "accepted"
	// IntimacyDeclined is a no.
	IntimacyDeclined IntimacyResponseStatus = "declined"
	// IntimacyConditional is a yes attached to a condition. Until the condition
	// is actually met it is still being negotiated, and it is never counted as
	// consent.
	IntimacyConditional IntimacyResponseStatus = "conditional"
)

func (s IntimacyResponseStatus) Valid() bool {
	switch s {
	case IntimacyPending, IntimacyAccepted, IntimacyDeclined, IntimacyConditional:
		return true
	default:
		return false
	}
}

// IntimacyParticipant is one invited person and the answer they actually gave.
type IntimacyParticipant struct {
	CharacterID string `json:"character_id"`
	// Status is what this person answered for themselves. One person's yes never
	// stands in for another's.
	Status IntimacyResponseStatus `json:"status"`
	// Condition is the stated condition on a conditional answer. It is kept
	// verbatim so the player can answer it rather than have it paraphrased away.
	Condition string `json:"condition,omitempty"`
	// Adult records that this person's own authored data stated an adult age when
	// they were invited. It is checked before the invitation exists, because a
	// world pack cannot lower the bar for who may be a participant. It is stored
	// so a replayed journal keeps the eligibility the invitation was made under.
	Adult bool `json:"adult"`
	// RespondedWorldTime is when the answer was given, not when the player read
	// it.
	RespondedWorldTime string `json:"responded_world_time,omitempty"`
}

func (p IntimacyParticipant) Normalize() IntimacyParticipant {
	p.CharacterID = strings.TrimSpace(p.CharacterID)
	p.Condition = strings.TrimSpace(p.Condition)
	p.RespondedWorldTime = strings.TrimSpace(p.RespondedWorldTime)
	if p.Status == "" {
		p.Status = IntimacyPending
	}
	return p
}

// IntimacyActivity is one activity's durable record.
type IntimacyActivity struct {
	ID       string        `json:"id"`
	StoryID  string        `json:"story_id"`
	BranchID string        `json:"branch_id"`
	Phase    IntimacyPhase `json:"phase"`
	// Participants is every invited person, in the order they were invited. A
	// person who declined stays on the list: the invitation happened, and a
	// roster that silently shrinks cannot show the player who said no.
	Participants []IntimacyParticipant `json:"participants,omitempty"`
	// GroupID names the conversation space the exchange is filed in. Its active
	// membership is the confirmed roster, which is what gives every line its
	// per-line audience.
	GroupID string `json:"group_id,omitempty"`
	// Version moves forward on every change. A confirmation naming an older
	// version is refused rather than applied to a roster it never saw.
	Version int `json:"version"`
	// Scene is where the activity is happening, captured at open. It is carried
	// on the activity rather than read from the story scene because a private
	// exchange must not be witnessed by everyone the story happens to have in
	// the room.
	Scene Scene `json:"scene"`
	// OpenedWorldTime and EndedWorldTime bound the activity in fiction time.
	OpenedWorldTime string `json:"opened_world_time,omitempty"`
	EndedWorldTime  string `json:"ended_world_time,omitempty"`
	// SourceEventID is the canonical journal event that first created it.
	SourceEventID string `json:"source_event_id,omitempty"`
}

func (a IntimacyActivity) Normalize() IntimacyActivity {
	a.ID = strings.TrimSpace(a.ID)
	a.StoryID = strings.TrimSpace(a.StoryID)
	a.BranchID = strings.TrimSpace(a.BranchID)
	a.GroupID = strings.TrimSpace(a.GroupID)
	a.OpenedWorldTime = strings.TrimSpace(a.OpenedWorldTime)
	a.EndedWorldTime = strings.TrimSpace(a.EndedWorldTime)
	a.SourceEventID = strings.TrimSpace(a.SourceEventID)
	a.Scene = a.Scene.Normalize()
	if a.Phase == "" {
		a.Phase = IntimacySelecting
	}
	if a.Version < 1 {
		a.Version = 1
	}
	participants := make([]IntimacyParticipant, 0, len(a.Participants))
	seen := make(map[string]bool, len(a.Participants))
	for _, participant := range a.Participants {
		participant = participant.Normalize()
		if participant.CharacterID == "" || seen[participant.CharacterID] {
			continue
		}
		seen[participant.CharacterID] = true
		participants = append(participants, participant)
	}
	a.Participants = participants
	return a
}

func (a IntimacyActivity) Validate() error {
	a = a.Normalize()
	if a.ID == "" || a.StoryID == "" || a.BranchID == "" {
		return fmt.Errorf("an intimacy activity requires an id, a story and a branch")
	}
	if !a.Phase.Valid() {
		return fmt.Errorf("intimacy activity %s has an invalid phase: %q", a.ID, a.Phase)
	}
	if len(a.Participants) == 0 {
		return fmt.Errorf("intimacy activity %s has no participants", a.ID)
	}
	if len(a.Participants) > MaxIntimacyParticipants {
		return fmt.Errorf("intimacy activity %s has %d participants; at most %d are supported", a.ID, len(a.Participants), MaxIntimacyParticipants)
	}
	for _, participant := range a.Participants {
		if !participant.Status.Valid() {
			return fmt.Errorf("intimacy activity %s participant %s has an invalid answer: %q", a.ID, participant.CharacterID, participant.Status)
		}
		// A person whose own authored data does not establish an adult age is not
		// a participant at all. This is checked structurally rather than only at
		// the entry point, so a record built by any path still has to carry it.
		if !participant.Adult {
			return fmt.Errorf("intimacy activity %s participant %s is not an established adult", a.ID, participant.CharacterID)
		}
	}
	// A ready activity is one somebody said yes to; that is what "ready" means.
	// An activity that ended because everybody declined has nobody in it, and
	// that is a legitimate ending rather than an invalid record — the design
	// requires a full refusal to be representable ("若所有人拒绝则不能开始").
	if a.Phase == IntimacyReady || a.Phase == IntimacyActive {
		if len(a.ConsentedRoster()) == 0 {
			return fmt.Errorf("intimacy activity %s is %s with nobody having accepted", a.ID, a.Phase)
		}
	}
	// Only an activity that actually ran has a conversation space. One that was
	// never started has none, and requiring one would make a refused invitation
	// unwritable.
	if a.Phase == IntimacyActive && a.GroupID == "" {
		return fmt.Errorf("intimacy activity %s is %s with no conversation space", a.ID, a.Phase)
	}
	return nil
}

// MaxIntimacyParticipants is the most NPCs one activity admits. The player is
// not counted: the design's "player plus one to four" means four here.
const MaxIntimacyParticipants = 4

// ConsentedRoster lists the people who actually said yes, in invitation order.
// A conditional answer is not here: it is still being negotiated, and treating
// it as consent is exactly the failure the design forbids.
func (a IntimacyActivity) ConsentedRoster() []string {
	a = a.Normalize()
	out := make([]string, 0, len(a.Participants))
	for _, participant := range a.Participants {
		if participant.Status == IntimacyAccepted {
			out = append(out, participant.CharacterID)
		}
	}
	return out
}

// Invited lists everyone who was asked, whatever they answered.
func (a IntimacyActivity) Invited() []string {
	a = a.Normalize()
	out := make([]string, 0, len(a.Participants))
	for _, participant := range a.Participants {
		out = append(out, participant.CharacterID)
	}
	return out
}

// Participant returns one invited person's row.
func (a IntimacyActivity) Participant(characterID string) (IntimacyParticipant, bool) {
	characterID = strings.TrimSpace(characterID)
	for _, participant := range a.Normalize().Participants {
		if participant.CharacterID == characterID {
			return participant, true
		}
	}
	return IntimacyParticipant{}, false
}

// SpaceID names the conversation space this activity's lines are filed in. It
// is the confirmed room once the activity is running; before that it is the
// invitation parley, which exists from the first invitation so the answers are
// readable while the player is still deciding.
func (a IntimacyActivity) SpaceID() string {
	a = a.Normalize()
	if a.GroupID != "" {
		return a.GroupID
	}
	return a.ID + ":space"
}

// InvitationSpaceID names the space a person's invitation is spoken in when
// they are asked to join an activity that is already running. It is separate
// from the room on purpose: until they agree, the room is not theirs to hear.
func (a IntimacyActivity) InvitationSpaceID(characterID string) string {
	return a.Normalize().ID + ":invite:" + strings.TrimSpace(characterID)
}

// Open reports whether the activity admits new lines.
func (a IntimacyActivity) Open() bool {
	return a.Normalize().Phase == IntimacyActive
}

// WithResponse records one person's answer and advances the phase. Recording an
// answer is the only way a person enters the roster, and it never adds anyone
// who was not invited.
func (a IntimacyActivity) WithResponse(characterID string, status IntimacyResponseStatus, condition, worldTime string) IntimacyActivity {
	a = a.Normalize()
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return a
	}
	status = IntimacyResponseStatus(strings.TrimSpace(string(status)))
	if !status.Valid() {
		return a
	}
	found := false
	for index := range a.Participants {
		if a.Participants[index].CharacterID != characterID {
			continue
		}
		a.Participants[index].Status = status
		a.Participants[index].Condition = strings.TrimSpace(condition)
		if status == IntimacyPending {
			a.Participants[index].Condition = ""
		}
		a.Participants[index].RespondedWorldTime = strings.TrimSpace(worldTime)
		found = true
		break
	}
	if !found {
		// An answer from someone who was never invited is not a consent. The
		// caller has a bug; silently adding the person would let a model invite
		// itself into the room.
		return a
	}
	a.Version++
	// A running activity stays running. Adding someone mid-activity is an
	// invitation, not a restart: their answer changes the roster, not the phase.
	// Recomputing here would drop an active room back to "ready" and let a later
	// confirmation start it a second time.
	if a.Phase == IntimacySelecting || a.Phase == IntimacyInviting || a.Phase == IntimacyReady {
		a.Phase = a.phaseAfterAnswers()
	}
	return a.Normalize()
}

// phaseAfterAnswers picks the phase the answers imply. A single acceptance makes
// the activity ready, not active: the player's confirmation is still owed.
func (a IntimacyActivity) phaseAfterAnswers() IntimacyPhase {
	accepted := 0
	outstanding := 0
	for _, participant := range a.Participants {
		switch participant.Status {
		case IntimacyAccepted:
			accepted++
		case IntimacyPending:
			outstanding++
		}
	}
	switch {
	case accepted > 0:
		return IntimacyReady
	case outstanding > 0:
		return IntimacyInviting
	default:
		// Nobody accepted and nobody is still deciding. The invitation failed; the
		// activity ends rather than pretending it is still in progress.
		return IntimacyEnded
	}
}

// Confirm opens the activity with exactly the roster the player confirmed. The
// expectedVersion is the member version the player was shown; a stale
// confirmation is refused rather than applied to a roster it never saw.
func (a IntimacyActivity) Confirm(groupID, worldTime string, expectedVersion int) (IntimacyActivity, error) {
	a = a.Normalize()
	if a.Phase != IntimacyReady {
		return a, fmt.Errorf("%w: activity is %s, not ready", ErrIntimacyPhase, a.Phase)
	}
	if expectedVersion > 0 && expectedVersion != a.Version {
		return a, fmt.Errorf("%w: on file=%d requested=%d", ErrIntimacyVersionConflict, a.Version, expectedVersion)
	}
	roster := a.ConsentedRoster()
	if len(roster) == 0 {
		return a, fmt.Errorf("%w: nobody has accepted", ErrIntimacyNoConsent)
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return a, fmt.Errorf("an intimacy activity needs a conversation space to start")
	}
	// People who declined or are still negotiating are dropped from the roster
	// that actually runs. They stay on the activity as invited, so the record
	// still says who was asked and what they said.
	a.GroupID = groupID
	a.Phase = IntimacyActive
	a.OpenedWorldTime = strings.TrimSpace(worldTime)
	a.Version++
	a.Scene.Present = append([]string{}, roster...)
	a.Scene = a.Scene.Normalize()
	return a.Normalize(), nil
}

// End closes the activity. It keeps every line and every answer.
func (a IntimacyActivity) End(worldTime string) IntimacyActivity {
	a = a.Normalize()
	if a.Phase == IntimacyEnded {
		return a
	}
	a.Phase = IntimacyEnded
	a.EndedWorldTime = strings.TrimSpace(worldTime)
	a.Version++
	return a.Normalize()
}

// AddParticipant invites one more person during an invitation phase.
func (a IntimacyActivity) AddParticipant(characterID string, adult bool, worldTime string) (IntimacyActivity, error) {
	a = a.Normalize()
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return a, fmt.Errorf("an invitation requires a person")
	}
	if a.Phase != IntimacySelecting && a.Phase != IntimacyInviting && a.Phase != IntimacyReady && a.Phase != IntimacyActive {
		return a, fmt.Errorf("%w: activity is %s", ErrIntimacyPhase, a.Phase)
	}
	if !adult {
		return a, fmt.Errorf("%w: %s", ErrIntimacyNotAdult, characterID)
	}
	if _, ok := a.Participant(characterID); ok {
		return a, nil
	}
	if len(a.Participants) >= MaxIntimacyParticipants {
		return a, fmt.Errorf("%w: at most %d people may be invited", ErrIntimacyTooMany, MaxIntimacyParticipants)
	}
	a.Participants = append(a.Participants, IntimacyParticipant{
		CharacterID: characterID, Status: IntimacyPending, Adult: true,
	})
	a.Version++
	// Inviting someone into a running activity does not stop it. §13.1 requires
	// a new member to be invited mid-activity and to be admitted only once they
	// agree, so the phase is left alone here and their answer is what joins them
	// to the room.
	if a.Phase != IntimacyActive {
		a.Phase = IntimacyInviting
	}
	return a.Normalize(), nil
}

// ErrIntimacyPhase means the activity is not in a phase that admits this action.
var ErrIntimacyPhase = fmt.Errorf("intimacy activity is not in the right phase")

// ErrIntimacyVersionConflict means a confirmation was based on a roster the
// activity has already moved past.
var ErrIntimacyVersionConflict = fmt.Errorf("intimacy activity roster version conflict")

// ErrIntimacyNoConsent means nobody actually accepted, so there is nothing to
// start. A conditional answer is not consent.
var ErrIntimacyNoConsent = fmt.Errorf("nobody has actually accepted")

// ErrIntimacyNotAdult means a person's own authored data does not establish an
// adult age. Realm, appearance and a passing model rewrite are not substitutes.
var ErrIntimacyNotAdult = fmt.Errorf("participant is not an established adult")

// ErrIntimacyTooMany means the roster would exceed what this build supports.
var ErrIntimacyTooMany = fmt.Errorf("too many participants")

// ErrIntimacyUnknown means an activity id names no activity on this branch.
var ErrIntimacyUnknown = fmt.Errorf("intimacy activity not found")

// IntimacyActivityFor returns one activity by id.
func (s State) IntimacyActivityFor(id string) (IntimacyActivity, bool) {
	id = strings.TrimSpace(id)
	for _, activity := range s.Normalize().IntimacyActivities {
		if activity.ID == id {
			return activity, true
		}
	}
	return IntimacyActivity{}, false
}

// OpenIntimacyActivities lists the activities still running, oldest first. The
// window offers to resume these rather than opening a second one.
func (s State) OpenIntimacyActivities() []IntimacyActivity {
	s = s.Normalize()
	out := make([]IntimacyActivity, 0, len(s.IntimacyActivities))
	for _, activity := range s.IntimacyActivities {
		if activity.Phase != IntimacyEnded {
			out = append(out, activity)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// UpsertIntimacyActivity registers or replaces one activity. A record older than
// the one on file is refused, so a retried or out-of-order write cannot roll the
// roster back to a version the player already moved past.
func (s *State) UpsertIntimacyActivity(activity IntimacyActivity) error {
	activity = activity.Normalize()
	if err := activity.Validate(); err != nil {
		return err
	}
	for index := range s.IntimacyActivities {
		existing := s.IntimacyActivities[index].Normalize()
		if existing.ID != activity.ID {
			continue
		}
		if activity.Version < existing.Version {
			return fmt.Errorf("%w: activity=%s on_file=%d requested=%d", ErrIntimacyVersionConflict, activity.ID, existing.Version, activity.Version)
		}
		if activity.SourceEventID == "" {
			activity.SourceEventID = existing.SourceEventID
		}
		s.IntimacyActivities[index] = activity
		return nil
	}
	s.IntimacyActivities = append(s.IntimacyActivities, activity)
	return nil
}
