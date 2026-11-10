package interactive

import (
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/npcgrowth"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// TurnNarrativeRevisedEvent replaces only the creator-visible prose of the
// latest logical turn. State, choices, images, trace, and turn identity remain
// attached to the immutable original turn.
type TurnNarrativeRevisedEvent struct {
	V         int    `json:"v"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	ParentID  string `json:"parent_id"`
	BranchID  string `json:"branch_id"`
	Ts        string `json:"ts"`
	TurnID    string `json:"turn_id"`
	Narrative string `json:"narrative"`
}

type TurnDisplayAppendedEvent struct {
	V        int          `json:"v"`
	Type     string       `json:"type"`
	ID       string       `json:"id"`
	ParentID string       `json:"parent_id"`
	BranchID string       `json:"branch_id"`
	Ts       string       `json:"ts"`
	TurnID   string       `json:"turn_id"`
	Display  DisplayEvent `json:"display"`
}

// TurnStateRevisedEvent is the single append-only settlement seam for async
// state generation, failure reporting, and rule rerolls.
type TurnStateRevisedEvent struct {
	V                    int              `json:"v"`
	Type                 string           `json:"type"`
	ID                   string           `json:"id"`
	ParentID             string           `json:"parent_id"`
	BranchID             string           `json:"branch_id"`
	Ts                   string           `json:"ts"`
	TurnID               string           `json:"turn_id"`
	StateDelta           *StateDelta      `json:"state_delta,omitempty"`
	ClearStateDelta      bool             `json:"clear_state_delta,omitempty"`
	StateStatus          string           `json:"state_status"`
	StateError           string           `json:"state_error,omitempty"`
	RuleResolution       *RuleResolution  `json:"rule_resolution,omitempty"`
	ClearRuleResolution  bool             `json:"clear_rule_resolution,omitempty"`
	TerminalOutcome      *TerminalOutcome `json:"terminal_outcome,omitempty"`
	ClearTerminalOutcome bool             `json:"clear_terminal_outcome,omitempty"`
	Reason               string           `json:"reason,omitempty"`
}

type StoryConfigUpdatedEvent struct {
	V        int      `json:"v"`
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id,omitempty"`
	BranchID string   `json:"branch_id"`
	Ts       string   `json:"ts"`
	Fields   []string `json:"fields,omitempty"`
}

type BranchSwitchedEvent struct {
	V          int    `json:"v"`
	Type       string `json:"type"`
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	BranchID   string `json:"branch_id"`
	Ts         string `json:"ts"`
	FromBranch string `json:"from_branch,omitempty"`
	ToBranch   string `json:"to_branch"`
}

type BranchArchivedEvent struct {
	V                     int    `json:"v"`
	Type                  string `json:"type"`
	ID                    string `json:"id"`
	ParentID              string `json:"parent_id,omitempty"`
	BranchID              string `json:"branch_id"`
	Ts                    string `json:"ts"`
	PreviousCurrentBranch string `json:"previous_current_branch,omitempty"`
	NextCurrentBranch     string `json:"next_current_branch,omitempty"`
}

type BranchHeadMovedEvent struct {
	V                int            `json:"v"`
	Type             string         `json:"type"`
	ID               string         `json:"id"`
	ParentID         string         `json:"parent_id,omitempty"`
	BranchID         string         `json:"branch_id"`
	Ts               string         `json:"ts"`
	PreviousHead     string         `json:"previous_head"`
	NextHead         string         `json:"next_head,omitempty"`
	NextLatestTurnID string         `json:"next_latest_turn_id,omitempty"`
	NextDepth        int            `json:"next_depth,omitempty"`
	StateCheckpoint  map[string]any `json:"state_checkpoint"`
	PlanCheckpoint   *BranchPlan    `json:"plan_checkpoint,omitempty"`
	Reason           string         `json:"reason"`
	// CharacterCheckpoint is the character memory at the new head, so rewinding
	// a branch also rewinds what its characters know.
	CharacterCheckpoint *character.State `json:"character_checkpoint,omitempty"`
	// CombatCheckpoint is the live battles at the new head, so rewinding a branch
	// also rewinds the fights it was in the middle of.
	CombatCheckpoint map[string]cultivation.CombatSession `json:"combat_checkpoint,omitempty"`
	// ClockCheckpoint is the formal time axis at the new head, so rewinding a
	// branch also rewinds how much in-fiction time it has spent. Restoring the
	// pools without it would leave a character recovered at an instant the branch
	// no longer claims to have reached.
	ClockCheckpoint *StoryClock `json:"clock_checkpoint,omitempty"`
	// RecoveryCheckpoint is the safe recovery activities at the new head, for the
	// same reason: a rewound branch must not keep resting toward a target it no
	// longer has.
	RecoveryCheckpoint map[string]RecoveryActivity `json:"recovery_checkpoint,omitempty"`
	// CultivationCheckpoint is the player-named cultivation intervals at the new
	// head, for the same reason: a rewound branch must not keep training toward a
	// stage it no longer reached.
	CultivationCheckpoint map[string]CultivationActivity `json:"cultivation_checkpoint,omitempty"`
	// JourneyCheckpoint is the branch's journeys at the new head, for the same
	// reason: a rewound branch must not keep standing on a road it no longer
	// walked.
	JourneyCheckpoint map[string]exploration.JourneySnapshot `json:"journey_checkpoint,omitempty"`
	// RealmCheckpoint is the branch's realm instances at the new head, for the
	// same reason: a rewound branch must not keep standing in a room it no longer
	// entered, or hold a reward it no longer took.
	LocalAffairCheckpoint []LocalAffairState            `json:"local_affair_checkpoint,omitempty"`
	RealmCheckpoint       map[string]RealmInstanceState `json:"realm_checkpoint,omitempty"`
	// QuestCheckpoint is the branch's task instances at the new head, for the
	// same reason: a rewound branch must not keep an agreement it no longer made,
	// or a payment it no longer received.
	QuestCheckpoint map[string]quest.Instance `json:"quest_checkpoint,omitempty"`
	// PublicEventCheckpoint is the branch's public event participation at the new
	// head, for the same reason the task checkpoint is: a rewound branch must not
	// keep a participation that belongs to a path it no longer contains.
	PublicEventCheckpoint map[string]publiquevent.Instance `json:"public_event_checkpoint,omitempty"`
	// NPCGrowthCheckpoint is the growth watermark at the new head, for the same
	// reason: a rewound branch must not keep the stage growth paid for over time
	// it no longer claims to have spent.
	NPCGrowthCheckpoint map[string]npcgrowth.Snapshot `json:"npc_growth_checkpoint,omitempty"`
}
