// Package publiquevent provides the rules for a public event: one situation that
// many people can care about, whose stage is decided by committed world time
// rather than by anyone's narration.
//
// It plans no prose and persists nothing. The event instance is a story fact and
// lives on the story journal; this package only says what stage the elapsed world
// clock puts the event in, what a participant can know at that stage, and what
// taking part in one way rather than another means.
//
// Three properties are deliberate, and they are what 自制-事件与任务 §4.4, §4.5
// and §6 ask for:
//
//   - The public situation is separate from the player's participation. §4.4
//     keeps the event's own state apart from a participant's record. An event
//     reaches its next stage because the world clock really moved, whether or not
//     the player was anywhere near it; the player's own choice to take part, stay
//     out or leave is recorded on their own participation record and changes
//     nothing about what the event is doing.
//
//   - A stage is a function of committed time, not a timer somebody sets. There
//     is no scheduler here and no per-round roll. The branch's formal clock is the
//     only input, so the same journal always reports the same stage, a fork that
//     happened earlier sees an earlier stage, and nobody can be told the event
//     advanced when no time was spent.
//
//   - Knowledge is per audience. §6 requires the player view to show only what
//     they witnessed or were told. A stage has facts that are publicly
//     observable, facts a participant learns by being present, and facts that
//     stay behind the scenes until the event is over; this package decides which
//     is which, so a panel cannot show the hidden ones because it never receives
//     them.
package publiquevent

import (
	"fmt"
	"strings"
)

const (
	SchemaVersion      = 1
	MaxIdentifierBytes = 128
	MaxTextBytes       = 8 << 10
	MaxStages          = 16
	MaxLines           = 32
	MaxParticipants    = 16
)

// ErrorCode is an immutable sentinel suitable for errors.Is.
type ErrorCode string

func (code ErrorCode) Error() string { return string(code) }

const (
	ErrInvalidEvent  ErrorCode = "invalid public event definition"
	ErrInvalidState  ErrorCode = "invalid public event state"
	ErrEventUnknown  ErrorCode = "public event is not registered"
	ErrEventClosed   ErrorCode = "public event has already closed"
	ErrNotJoined     ErrorCode = "the character has not taken part in this event"
	ErrAlreadyJoined ErrorCode = "the character has already taken part in this event"
	ErrUnknownAction ErrorCode = "public event action is not supported"
)

// Diagnostic points to one invalid field or reference.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// StageStatus says whether a stage is reachable, current, or already past.
//
// It is derived, never stored: a stage's status is a reading of the clock, so a
// journal that is replayed produces the same statuses without a second write.
type StageStatus string

const (
	// StagePast means the clock is already beyond this stage.
	StagePast StageStatus = "past"
	// StageCurrent means the clock sits inside this stage's window.
	StageCurrent StageStatus = "current"
	// StageAhead means the clock has not reached this stage yet.
	StageAhead StageStatus = "ahead"
)

// Stage is one authored phase of a public event.
//
// OpenAfterSeconds is measured from the branch's clock origin, which is the same
// axis every activity that spends in-fiction time moves. It is not a wall-clock
// delay and not a number of turns: §4.8 forbids inventing a deadline that the
// event did not state, and the clock is what makes the deadline real.
type Stage struct {
	ID string `json:"id"`
	// Title is the event's own wording for this stage, so a panel never invents
	// one.
	Title string `json:"title"`
	// Summary is what an ordinary person could observe or hear about at this
	// stage. It is public: anyone in the region may know it.
	Summary string `json:"summary"`
	// ParticipantLines are things someone present at this stage could find out.
	// They are shown only to a character who took part while this stage was
	// current.
	ParticipantLines []string `json:"participant_lines,omitempty"`
	// SecretLines are what stays behind the scenes. §6 keeps them out of every
	// player view, so the read below never returns them; they exist on the
	// definition so a later stage or a narrator can be written against them
	// without the panel ever seeing them.
	SecretLines []string `json:"secret_lines,omitempty"`
	// OpenAfterSeconds is the branch clock instant at which this stage becomes
	// current.
	OpenAfterSeconds int64 `json:"open_after_seconds"`
	// Final marks the stage the event settles at. It is what makes the event
	// close: past this point there is nothing left to take part in, and the
	// panel says so rather than offering a join that would mean nothing.
	Final bool `json:"final,omitempty"`
}

// ActionKind is one way a character can take part.
//
// Each kind is a real, distinguishable choice rather than a flavour line: §4.5
// and the review addendum ask for at least two reasonable approaches with
// genuinely different consequences.
type ActionKind string

const (
	// ActionObserve is watching and asking around without committing.
	ActionObserve ActionKind = "observe"
	// ActionHelp is actively helping the people at the centre of the event.
	ActionHelp ActionKind = "help"
	// ActionInvestigate is looking for what is really going on.
	ActionInvestigate ActionKind = "investigate"
	// ActionWithdraw is stepping back, which is a real choice and not a failure.
	ActionWithdraw ActionKind = "withdraw"
)

func validAction(action ActionKind) bool {
	switch action {
	case ActionObserve, ActionHelp, ActionInvestigate, ActionWithdraw:
		return true
	}
	return false
}

// RewardKind is what a reward hands over.
type RewardKind string

const (
	// RewardCurrency moves the character's purse.
	RewardCurrency RewardKind = "currency"
)

// Reward is what taking part in one particular way really pays.
//
// It is authored per action rather than per event because the choices are not
// equivalent: standing by and lending a hand are different decisions and the
// world answers them differently. A reward is paid once, in the same commit as
// the participation record, so a retry cannot pay twice.
type Reward struct {
	Kind     RewardKind `json:"kind"`
	RefID    string     `json:"ref_id"`
	Quantity int64      `json:"quantity"`
}

// Action is one way a character can take part, with its own wording and its own
// consequence.
type Action struct {
	Kind ActionKind `json:"kind"`
	// StageID limits this action to one stage. Empty allows every stage.
	StageID string `json:"stage_id,omitempty"`
	// Label and Brief are the event's own wording for this choice, so the panel
	// never has to invent a button caption.
	Label string `json:"label"`
	Brief string `json:"brief"`
	// Reward is what this choice really pays, nil when it pays nothing. Standing
	// aside paying nothing is a consequence too, not a missing value.
	Reward *Reward `json:"reward,omitempty"`
}

// Participant is one party with a stake in the event.
//
// PublicRole is what anybody in the region could tell you. SecretMotive is what
// they are actually after, and it is never returned to a player view: §6 keeps
// an unobserved motive out of the panel entirely rather than trusting the panel
// to hide it.
type Participant struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PublicRole   string `json:"public_role"`
	SecretMotive string `json:"secret_motive,omitempty"`
}

// Definition is one authored public event.
type Definition struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Version       string `json:"version"`
	// PackageID and PackageVersion name the world pack revision this event was
	// authored against. An event is part of a world rather than a universal
	// fact, so a story in another pack must not be told it is happening.
	PackageID      string `json:"package_id"`
	PackageVersion string `json:"package_version"`
	Title          string `json:"title"`
	// Brief is why an ordinary character would care about this at all.
	Brief string `json:"brief"`
	// Region is the place the event belongs to, in the story's own wording. It is
	// what makes "did the player hear about this" answerable.
	Region string `json:"region"`
	// RegionLocationIDs are the registered places this event can be heard of
	// from. A character standing anywhere else has not been told about it, so §6
	// keeps the event out of their view. Naming the places makes that a fact
	// rather than a guess about the scene's free text.
	RegionLocationIDs []string      `json:"region_location_ids,omitempty"`
	Participants      []Participant `json:"participants,omitempty"`
	Stages            []Stage       `json:"stages"`
	// Actions are the kinds of participation this event really supports. An
	// action the event does not list is refused rather than recorded as a
	// meaningless join.
	Actions []Action `json:"actions"`
}

// Instance is one public event as this branch really has it.
//
// It deliberately carries no stage. The stage is read from the clock, so a
// record can never claim a stage the elapsed time does not support.
type Instance struct {
	EventID string `json:"event_id"`
	// EventVersion pins the definition the instance was raised against, so a
	// definition that changed underneath is detected rather than re-read under
	// new meaning.
	EventVersion string `json:"event_version"`
	// JoinedAtSeconds is the branch clock instant the character took part, and
	// JoinedStage is the stage that was current then. Both are fixed once: a
	// later replay must not move them, because what the character could have
	// known is decided by when they joined.
	JoinedAtSeconds int64  `json:"joined_at_seconds"`
	JoinedStage     string `json:"joined_stage"`
	// Action is how the character took part.
	Action ActionKind `json:"action"`
	// Withdrawn reports that the character later stepped back. Their earlier
	// participation still happened, which is why this is a flag rather than a
	// deletion.
	Withdrawn bool `json:"withdrawn,omitempty"`
}

// validID mirrors the identifier rule the rest of the product uses.
func validID(value string) bool {
	if value == "" || len(value) > MaxIdentifierBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':' || r == '@' || r == '#' {
			continue
		}
		return false
	}
	return true
}

func validText(value string, maxBytes int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxBytes
}

// ValidateDefinition checks an authored event before it can be raised.
func ValidateDefinition(definition Definition) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(code, path, message string) {
		diagnostics = append(diagnostics, Diagnostic{Code: code, Path: path, Message: message})
	}
	if definition.SchemaVersion != SchemaVersion {
		add("unsupported_schema", "schema_version", "unsupported public event schema version")
	}
	for path, value := range map[string]string{"id": definition.ID, "version": definition.Version} {
		if !validID(value) {
			add("invalid_id", path, "must be a stable non-empty identifier")
		}
	}
	if !validText(definition.Title, MaxTextBytes) {
		add("invalid_text", "title", "must be non-empty and within the text limit")
	}
	if !validText(definition.Brief, MaxTextBytes) {
		add("invalid_text", "brief", "must be non-empty and within the text limit")
	}
	if !validText(definition.Region, MaxTextBytes) {
		add("invalid_text", "region", "a public event must name the place it belongs to")
	}
	for index, locationID := range definition.RegionLocationIDs {
		if !validID(locationID) {
			add("invalid_id", fmt.Sprintf("region_location_ids[%d]", index), "must be a registered location identifier")
		}
	}
	if len(definition.Stages) == 0 {
		add("missing_stages", "stages", "a public event must have at least one stage")
	}
	if len(definition.Stages) > MaxStages {
		add("limit_exceeded", "stages", "stage count exceeds the supported limit")
	}
	if len(definition.Participants) > MaxParticipants {
		add("limit_exceeded", "participants", "participant count exceeds the supported limit")
	}
	seenParticipant := make(map[string]bool, len(definition.Participants))
	for index, participant := range definition.Participants {
		path := fmt.Sprintf("participants[%d]", index)
		if !validID(participant.ID) {
			add("invalid_id", path+".id", "must be a stable non-empty identifier")
		} else if seenParticipant[participant.ID] {
			add("duplicate_id", path+".id", "participant identifier is duplicated")
		}
		seenParticipant[participant.ID] = true
		if !validText(participant.Name, MaxTextBytes) {
			add("invalid_text", path+".name", "must be non-empty and within the text limit")
		}
		if !validText(participant.PublicRole, MaxTextBytes) {
			add("invalid_text", path+".public_role", "a participant must say what the region knows it is doing")
		}
	}
	seen := make(map[string]bool, len(definition.Stages))
	var lastOpen int64
	for index, stage := range definition.Stages {
		path := fmt.Sprintf("stages[%d]", index)
		if !validID(stage.ID) {
			add("invalid_id", path+".id", "must be a stable non-empty identifier")
		} else if seen[stage.ID] {
			add("duplicate_id", path+".id", "stage identifier is duplicated")
		}
		seen[stage.ID] = true
		if !validText(stage.Title, MaxTextBytes) {
			add("invalid_text", path+".title", "must be non-empty and within the text limit")
		}
		if !validText(stage.Summary, MaxTextBytes) {
			add("invalid_text", path+".summary", "a stage must say what is publicly observable")
		}
		if len(stage.ParticipantLines) > MaxLines || len(stage.SecretLines) > MaxLines {
			add("limit_exceeded", path, "line count exceeds the supported limit")
		}
		if stage.OpenAfterSeconds < 0 {
			add("invalid_time", path+".open_after_seconds", "must not be negative")
		}
		// Stages are ordered, so an earlier stage may not open after a later one:
		// otherwise the clock would reach stage three while stage two was still
		// ahead and the event would have no honest current stage.
		if index > 0 && stage.OpenAfterSeconds < lastOpen {
			add("unordered_stage", path+".open_after_seconds", "stages must open in order")
		}
		lastOpen = stage.OpenAfterSeconds
		// Only the last stage may settle the event. A final stage in the middle
		// would leave later stages that could never be reached.
		if stage.Final && index != len(definition.Stages)-1 {
			add("final_stage_not_last", path+".final", "only the last stage may settle the event")
		}
	}
	if len(definition.Actions) == 0 {
		add("missing_actions", "actions", "a public event must say how a character can take part")
	}
	seenAction := make(map[ActionKind]bool, len(definition.Actions))
	for index, action := range definition.Actions {
		path := fmt.Sprintf("actions[%d]", index)
		if !validAction(action.Kind) {
			add("invalid_action", path+".kind", "must be observe, help, investigate or withdraw")
		} else if seenAction[action.Kind] {
			add("duplicate_id", path+".kind", "action kind is duplicated")
		}
		seenAction[action.Kind] = true
		if action.StageID != "" && !seen[action.StageID] {
			add("unknown_stage", path+".stage_id", "action stage is not one this event has")
		}
		if !validText(action.Label, MaxTextBytes) {
			add("invalid_text", path+".label", "must be non-empty and within the text limit")
		}
		if !validText(action.Brief, MaxTextBytes) {
			add("invalid_text", path+".brief", "must be non-empty and within the text limit")
		}
		if action.Reward != nil {
			rewardPath := path + ".reward"
			if action.Reward.Kind != RewardCurrency {
				add("invalid_reward", rewardPath+".kind", "must be currency")
			}
			if !validText(action.Reward.RefID, MaxTextBytes) {
				add("invalid_reward", rewardPath+".ref_id", "must name what is handed over")
			}
			if action.Reward.Quantity <= 0 {
				add("invalid_reward", rewardPath+".quantity", "must be positive")
			}
		}
	}
	// The world identity is checked last so the wording diagnostics a caller
	// reads first stay about the event's own text.
	for path, value := range map[string]string{"package_id": definition.PackageID, "package_version": definition.PackageVersion} {
		if !validID(value) {
			add("invalid_id", path, "must be a stable non-empty identifier")
		}
	}
	return diagnostics
}

// StageAt reports which stage the given branch clock instant sits in.
//
// It is a pure function of the clock, so the same instant always answers the same
// stage. An event whose first stage opens later than the instant is not yet
// public: it reports false, which is what keeps a candidate from being shown as
// happening before it does.
func (definition Definition) StageAt(seconds int64) (Stage, StageStatus, bool) {
	if len(definition.Stages) == 0 {
		return Stage{}, "", false
	}
	if seconds < definition.Stages[0].OpenAfterSeconds {
		return Stage{}, StageAhead, false
	}
	current := definition.Stages[0]
	for _, stage := range definition.Stages[1:] {
		if seconds >= stage.OpenAfterSeconds {
			current = stage
			continue
		}
		break
	}
	return current, StageCurrent, true
}

// StageStatuses reports every stage's status at one instant, in authored order.
//
// The panel needs the whole ladder rather than only the current rung: a
// participant wants to see what already happened, and an event that has closed
// should read as closed rather than as an error.
func (definition Definition) StageStatuses(seconds int64) []StageStatus {
	statuses := make([]StageStatus, 0, len(definition.Stages))
	currentID := definition.currentStageID(seconds)
	for _, stage := range definition.Stages {
		switch {
		case seconds < stage.OpenAfterSeconds:
			statuses = append(statuses, StageAhead)
		case stage.ID == currentID:
			statuses = append(statuses, StageCurrent)
		default:
			statuses = append(statuses, StagePast)
		}
	}
	return statuses
}

// currentStageID is the last stage the instant has reached, empty when none has.
func (definition Definition) currentStageID(seconds int64) string {
	stage, status, ok := definition.StageAt(seconds)
	if !ok || status != StageCurrent {
		return ""
	}
	return stage.ID
}

// CurrentStage is the stage the instant sits in.
func (definition Definition) CurrentStage(seconds int64) (Stage, bool) {
	stage, status, ok := definition.StageAt(seconds)
	if !ok || status != StageCurrent {
		return Stage{}, false
	}
	return stage, true
}

// Closed reports that the clock has reached the event's settling stage.
func (definition Definition) Closed(seconds int64) bool {
	stage, ok := definition.CurrentStage(seconds)
	return ok && stage.Final
}

// SupportsAction reports whether the event really offers this kind of taking
// part. A candidate action the event does not list is refused rather than
// recorded, because a join with no meaning would read as participation that
// never happened.
func (definition Definition) SupportsAction(action ActionKind) bool {
	_, ok := definition.ActionByKind(action)
	return ok
}

// ActionByKind finds the authored action of this kind.
func (definition Definition) ActionByKind(action ActionKind) (Action, bool) {
	for _, candidate := range definition.Actions {
		if candidate.Kind == action {
			return candidate, true
		}
	}
	return Action{}, false
}

// AvailableIn reports whether the action belongs to the given stage.
func (action Action) AvailableIn(stageID string) bool {
	return action.StageID == "" || action.StageID == stageID
}

// StageByID finds one authored stage.
func (definition Definition) StageByID(id string) (Stage, bool) {
	for _, stage := range definition.Stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return Stage{}, false
}

// KnownLines is what the character may be shown about one stage.
//
// §6 is the reason this is a function rather than a field read: the public
// summary is always visible, a participant's own lines are visible only to
// someone who joined, and the secret lines are never returned. A caller cannot
// leak them by accident because it is never handed them.
func KnownLines(stage Stage, joined bool) []string {
	lines := make([]string, 0, 1+len(stage.ParticipantLines))
	lines = append(lines, stage.Summary)
	if joined {
		lines = append(lines, stage.ParticipantLines...)
	}
	return lines
}

// JoinedStageLines is what a participant may be shown about the stage they
// joined at.
//
// A character who joined during an earlier stage keeps what they were present
// for. They do not retroactively learn the later stage's participant lines,
// because they were not there; that is the same rule §6 applies to a message
// sent to somebody else. The current stage's public summary is shown separately,
// because that is what everybody knows.
func JoinedStageLines(definition Definition, instance Instance) []string {
	stage, ok := definition.StageByID(instance.JoinedStage)
	if !ok {
		return nil
	}
	return KnownLines(stage, true)
}

// ValidateInstance checks a stored participation record against its definition.
func ValidateInstance(definition Definition, instance Instance) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(code, path, message string) {
		diagnostics = append(diagnostics, Diagnostic{Code: code, Path: path, Message: message})
	}
	if instance.EventID != definition.ID {
		add("mismatched_event", "event_id", "instance does not belong to this event")
	}
	if instance.EventVersion != definition.Version {
		add("mismatched_version", "event_version", "instance is bound to another definition version")
	}
	if _, ok := definition.StageByID(instance.JoinedStage); !ok {
		add("unknown_stage", "joined_stage", "joined stage is not one this event has")
	}
	if instance.JoinedAtSeconds < 0 {
		add("invalid_time", "joined_at_seconds", "must not be negative")
	}
	if !validAction(instance.Action) {
		add("invalid_action", "action", "must be observe, help, investigate or withdraw")
	} else if action, ok := definition.ActionByKind(instance.Action); ok && !action.AvailableIn(instance.JoinedStage) {
		add("wrong_action_stage", "action", "this action was not available at the joined stage")
	}
	return diagnostics
}

// Join records one character taking part at the given instant and stage.
//
// It refuses an event the clock has not opened yet, and one that has already
// settled, because a record of taking part in something that is not happening
// would be a claim the world does not support. It refuses a second join for the
// same reason a second offer is refused: the character's decision was already
// made and recorded.
func Join(definition Definition, existing *Instance, seconds int64, action ActionKind) (Instance, error) {
	if existing != nil {
		return Instance{}, fmt.Errorf("%w: %s", ErrAlreadyJoined, definition.ID)
	}
	stage, status, ok := definition.StageAt(seconds)
	if !ok || status != StageCurrent {
		return Instance{}, fmt.Errorf("%w: %s has not opened at %d seconds", ErrEventClosed, definition.ID, seconds)
	}
	if stage.Final {
		return Instance{}, fmt.Errorf("%w: %s settled at stage %s", ErrEventClosed, definition.ID, stage.ID)
	}
	if offered, ok := definition.ActionByKind(action); !ok || !offered.AvailableIn(stage.ID) {
		return Instance{}, fmt.Errorf("%w: %q at stage %s", ErrUnknownAction, action, stage.ID)
	}
	return Instance{
		EventID: definition.ID, EventVersion: definition.Version,
		JoinedAtSeconds: seconds, JoinedStage: stage.ID, Action: action,
	}, nil
}

// Withdraw records the character stepping back.
//
// Their participation is not erased: §4.4 says abandoning one's own commitment
// does not undo what really happened, and the same is true of standing aside. A
// reward already handed over for work already done is not clawed back either.
func Withdraw(existing Instance) (Instance, error) {
	if existing.EventID == "" {
		return Instance{}, fmt.Errorf("%w", ErrNotJoined)
	}
	if existing.Withdrawn {
		return Instance{}, fmt.Errorf("%w: already withdrawn", ErrAlreadyJoined)
	}
	existing.Withdrawn = true
	return existing, nil
}
