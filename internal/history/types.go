// Package history owns independent historical state and its canonical journal.
package history

import "errors"

var ErrInvalid = errors.New("invalid history request")
var ErrNotFound = errors.New("history campaign not found")
var ErrConflict = errors.New("history revision conflict")
var ErrCommandConflict = errors.New("history command id reused for another request")

type Region struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Heart      int    `json:"heart"`
	TaxBase    int64  `json:"tax_base"`
	Disaster   int    `json:"disaster"`
	Refugees   int64  `json:"refugees"`
	Irrigation int    `json:"irrigation"`
	Training   int    `json:"training"`
}
type Minister struct {
	NameEN    string   `json:"name_en,omitempty"`
	OfficeEN  string   `json:"office_en,omitempty"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Office    string   `json:"office"`
	Profile   string   `json:"profile"`
	Domain    string   `json:"domain"`
	Capacity  int64    `json:"capacity"`
	Knowledge []string `json:"knowledge"`
}

// Budget and Grain are total commitments; Spent tracks consumed escrow.
type Project struct {
	ID            string   `json:"id"`
	InstructionID string   `json:"instruction_id"`
	EdictID       string   `json:"edict_id"`
	Kind          string   `json:"kind"`
	Target        string   `json:"target"`
	MinisterID    string   `json:"minister_id"`
	Budget        int64    `json:"budget"`
	Grain         int64    `json:"grain"`
	Spent         int64    `json:"spent"`
	GrainSpent    int64    `json:"grain_spent"`
	Reserved      bool     `json:"reserved"`
	Quarters      int      `json:"quarters"`
	Progress      int      `json:"progress"`
	Required      int      `json:"required"`
	NotBeforeTurn int      `json:"not_before_turn"`
	DependsOn     []string `json:"depends_on"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason"`
}
type Instruction struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Target        string   `json:"target,omitempty"`
	MinisterID    string   `json:"minister_id,omitempty"`
	Budget        int64    `json:"budget,omitempty"`
	Grain         int64    `json:"grain,omitempty"`
	Quarters      int      `json:"quarters,omitempty"`
	ProjectID     string   `json:"project_id,omitempty"`
	NotBeforeTurn int      `json:"not_before_turn,omitempty"`
	DependsOn     []string `json:"depends_on,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}
type EdictPlan struct {
	SourceAdviceID string        `json:"source_advice_id,omitempty"`
	Orders         []OrderIntent `json:"orders,omitempty"`
	SourceChatID   string        `json:"source_chat_id,omitempty"`
	Text           string        `json:"text"`
	Instructions   []Instruction `json:"instructions"`
}
type Outcome struct {
	InstructionID string `json:"instruction_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	Spent         int64  `json:"spent,omitempty"`
	GrainSpent    int64  `json:"grain_spent,omitempty"`
	Progress      int    `json:"progress,omitempty"`
}
type Edict struct {
	ID       string    `json:"id"`
	Turn     int       `json:"turn"`
	Plan     EdictPlan `json:"plan"`
	Outcomes []Outcome `json:"outcomes"`
}
type Reply struct {
	MinisterID string `json:"minister_id"`
	Text       string `json:"text"`
}

// A turn has one shared audience. Raw dialogue is never deleted by a restore.
type ChatTurn struct {
	SourceReportID string   `json:"source_report_id,omitempty"`
	Discarded      bool     `json:"discarded"`
	ID             string   `json:"id"`
	Turn           int      `json:"turn"`
	Mode           string   `json:"mode"`
	MinisterIDs    []string `json:"minister_ids"`
	UserMessage    string   `json:"user_message"`
	Replies        []Reply  `json:"replies"`
	Revision       uint64   `json:"revision"`
}
type ProjectAdjustment struct {
	ProjectID     string `json:"project_id"`
	EfficiencyPct int    `json:"efficiency_pct"`
	Delay         bool   `json:"delay"`
	Reason        string `json:"reason"`
}
type RegionalEvent struct {
	RegionID      string `json:"region_id"`
	SeverityDelta int    `json:"severity_delta"`
	Reason        string `json:"reason"`
}

// QuarterPlan carries legacy adjustments or a scenario-world proposal.
// The campaign binding selects the appropriate settlement path.
type QuarterPlan struct {
	World       *WorldProposal      `json:"world,omitempty"`
	Adjustments []ProjectAdjustment `json:"adjustments"`
	Events      []RegionalEvent     `json:"events"`
	Narrative   string              `json:"narrative"`
}
type RevisionRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}
type QuarterReport struct {
	World              *WorldProposal      `json:"world,omitempty"`
	AppliedAdjustments []ProjectAdjustment `json:"applied_adjustments"`
	Discarded          bool                `json:"discarded"`
	Revision           uint64              `json:"revision"`
	ID                 string              `json:"id"`
	Turn               int                 `json:"turn"`
	Year               int                 `json:"year"`
	Quarter            int                 `json:"quarter"`
	OpeningTreasury    int64               `json:"opening_treasury"`
	Income             int64               `json:"income"`
	Upkeep             int64               `json:"upkeep"`
	UpkeepArrears      int64               `json:"upkeep_arrears"`
	Reserved           int64               `json:"reserved"`
	Spent              int64               `json:"spent"`
	GrainReserved      int64               `json:"grain_reserved"`
	GrainSpent         int64               `json:"grain_spent"`
	ClosingTreasury    int64               `json:"closing_treasury"`
	Outcomes           []Outcome           `json:"outcomes"`
	Events             []RegionalEvent     `json:"events"`
	Narrative          string              `json:"narrative"`
}
type Checkpoint struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Turn     int    `json:"turn"`
	Revision uint64 `json:"revision"`
}
type Inspection struct {
	Revision  uint64         `json:"revision"`
	Discarded bool           `json:"discarded"`
	ID        string         `json:"id"`
	Target    string         `json:"target"`
	Turn      int            `json:"turn"`
	Facts     map[string]any `json:"facts"`
}
type State struct {
	Advice              []SavedAdvice   `json:"advice,omitempty"`
	Archived            bool            `json:"archived,omitempty"`
	Scenario            *Scenario       `json:"scenario,omitempty"`
	ScenarioDigest      string          `json:"scenario_digest,omitempty"`
	World               *World          `json:"world,omitempty"`
	Locale              string          `json:"locale,omitempty"`
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	ScenarioID          string          `json:"scenario_id"`
	Revision            uint64          `json:"revision"`
	Year                int             `json:"year"`
	Quarter             int             `json:"quarter"`
	Turn                int             `json:"turn"`
	Treasury            int64           `json:"treasury"`
	PrivyPurse          int64           `json:"privy_purse"`
	Grain               int64           `json:"grain"`
	Regions             []Region        `json:"regions"`
	Ministers           []Minister      `json:"ministers"`
	Projects            []Project       `json:"projects"`
	Edicts              []Edict         `json:"edicts"`
	Chats               []ChatTurn      `json:"chats"`
	Reports             []QuarterReport `json:"reports"`
	Checkpoints         []Checkpoint    `json:"checkpoints"`
	Inspections         []Inspection    `json:"inspections"`
	DiscardedRanges     []RevisionRange `json:"discarded_ranges"`
	LastRestoreRevision uint64          `json:"last_restore_revision,omitempty"`
}
type Summary struct {
	Archived    bool   `json:"archived,omitempty"`
	Unavailable bool   `json:"unavailable,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	ScenarioID  string `json:"scenario_id"`
	Revision    uint64 `json:"revision"`
	Year        int    `json:"year"`
	Quarter     int    `json:"quarter"`
	Turn        int    `json:"turn"`
}
type CreateRequest struct {
	ScenarioID string `json:"scenario_id,omitempty"`
	Locale     string `json:"locale,omitempty"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	CommandID  string `json:"command_id"`
}
type Command struct {
	Advice           *AdviceChange `json:"advice,omitempty"`
	ID               string        `json:"command_id"`
	ExpectedRevision uint64        `json:"expected_revision"`
	Kind             string        `json:"kind"`
	Chat             *ChatTurn     `json:"chat,omitempty"`
	Edict            *EdictPlan    `json:"edict,omitempty"`
	Quarter          *QuarterPlan  `json:"quarter,omitempty"`
	CheckpointID     string        `json:"checkpoint_id,omitempty"`
	Label            string        `json:"label,omitempty"`
	InspectionTarget string        `json:"inspection_target,omitempty"`
	// RequestHash identifies the user's input, excluding nondeterministic AI text.
	RequestHash string `json:"request_hash,omitempty"`
}
type Receipt struct {
	CommandID string    `json:"command_id"`
	Revision  uint64    `json:"revision"`
	Kind      string    `json:"kind"`
	Duplicate bool      `json:"duplicate"`
	State     State     `json:"state"`
	Outcomes  []Outcome `json:"outcomes"`
}
type MinisterContext struct {
	Minister         Minister       `json:"minister"`
	CurrentFacts     map[string]any `json:"current_facts"`
	AssignedProjects []Project      `json:"assigned_projects"`
	Memories         []ChatTurn     `json:"memories"`
	OmittedTurns     int            `json:"omitted_turns"`
	Rule             string         `json:"rule"`
}
