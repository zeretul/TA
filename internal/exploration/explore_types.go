package exploration

// ExploreRules is the small authored action set of one persistent place. It is
// not an expression language: facts, choices, time and declared results are the
// only predicates, and actor eligibility is checked by the Story caller.
type ExploreRules struct {
	NameKey            string            `json:"name_key"`
	DescriptionKey     string            `json:"description_key"`
	PressureLabel      string            `json:"pressure_label"`
	PressureLabelKey   string            `json:"pressure_label_key"`
	PressureLimit      int               `json:"pressure_limit"`
	ReturnAfterSeconds int64             `json:"return_after_seconds"`
	Approaches         []ExploreApproach `json:"approaches"`
}

type ExploreApproachKind string

const (
	ExploreSurvey        ExploreApproachKind = "survey"
	ExplorePrepare       ExploreApproachKind = "prepare"
	ExploreApproachRoute ExploreApproachKind = "approach"
	ExploreResolve       ExploreApproachKind = "resolve"
)

type ExploreCostKind string

const (
	ExploreConsumeItem ExploreCostKind = "consume_item"
	ExploreRequireItem ExploreCostKind = "require_item"
	ExploreUseSkill    ExploreCostKind = "use_skill"
)

// ExploreResourceCost requests a registered item or one real skill invocation.
// Skill cost is resolved from the frozen actor rules by Story, not supplied by
// a client or duplicated as an independently adjustable energy value here.
type ExploreResourceCost struct {
	Kind     ExploreCostKind `json:"kind"`
	RefID    string          `json:"ref_id"`
	Quantity int64           `json:"quantity"`
	// Presentation fields are filled by Story only after projecting known costs.
	Name       string `json:"name,omitempty"`
	NameKey    string `json:"name_key,omitempty"`
	EnergyCost int    `json:"energy_cost,omitempty"`
}

type ExploreApproach struct {
	ID                  string                `json:"id"`
	AreaID              string                `json:"area_id"`
	Kind                ExploreApproachKind   `json:"kind"`
	Name                string                `json:"name"`
	NameKey             string                `json:"name_key"`
	Description         string                `json:"description"`
	DescriptionKey      string                `json:"description_key"`
	ExclusiveGroup      string                `json:"exclusive_group,omitempty"`
	RequiredFactIDs     []string              `json:"required_fact_ids,omitempty"`
	ExcludedFactIDs     []string              `json:"excluded_fact_ids,omitempty"`
	DurationSeconds     int64                 `json:"duration_seconds"`
	PressureDelta       int                   `json:"pressure_delta"`
	Costs               []ExploreResourceCost `json:"costs,omitempty"`
	Discoveries         []InstanceDiscovery   `json:"discoveries,omitempty"`
	ResolveChallengeIDs []string              `json:"resolve_challenge_ids,omitempty"`
	RewardRefIDs        []string              `json:"reward_ref_ids,omitempty"`
	AllowedAfterSetback bool                  `json:"allowed_after_setback,omitempty"`
	RequiresReturn      bool                  `json:"requires_return,omitempty"`
	RequiresSetback     bool                  `json:"requires_setback,omitempty"`
	StartsReturn        bool                  `json:"starts_return,omitempty"`

	// Story verifies completed, unused work for this project and exact instance.
	// The project identifier is metadata, not an independently usable capability.
	CooperationProjectID string `json:"cooperation_project_id,omitempty"`
}

type ExploreDecision struct {
	GroupID    string `json:"group_id"`
	ApproachID string `json:"approach_id"`
}

// ExploreState lives only on the existing instance snapshot. A return does not
// clear pressure, choices or grants; ReturnDueSeconds is an absolute Story clock.
type ExploreState struct {
	Pressure         int               `json:"pressure"`
	UsedApproachIDs  []string          `json:"used_approach_ids"`
	Decisions        []ExploreDecision `json:"decisions"`
	Setback          bool              `json:"setback"`
	ReturnDueSeconds int64             `json:"return_due_seconds,omitempty"`
}

// ExploreApproachInput is internal to the trusted Story integration. Capabilities
// contains approach IDs whose full item/skill/resource and cooperation eligibility
// Story checked against the exact actor and instance revisions being committed.
// Never decode it from an API.
type ExploreApproachInput struct {
	ApproachID       string
	StoryTimeSeconds int64
	Capabilities     []string
}

type ExploreTransition struct {
	InstanceTransition
	DurationSeconds int64                 `json:"duration_seconds"`
	Costs           []ExploreResourceCost `json:"costs"`
	Rewards         []InstanceReward      `json:"rewards"`
}

type ExploreViewContext struct {
	StoryTimeSeconds int64
	Capabilities     []string
}

type ExploreApproachView struct {
	ID              string                `json:"id"`
	Kind            ExploreApproachKind   `json:"kind"`
	Name            string                `json:"name"`
	NameKey         string                `json:"name_key"`
	Description     string                `json:"description"`
	DescriptionKey  string                `json:"description_key"`
	DurationSeconds int64                 `json:"duration_seconds"`
	PressureDelta   int                   `json:"pressure_delta"`
	Costs           []ExploreResourceCost `json:"costs"`
	Available       bool                  `json:"available"`
	CausesSetback   bool                  `json:"causes_setback"`
	BlockedReason   string                `json:"blocked_reason,omitempty"`
}

type ExploreView struct {
	Pressure         int                   `json:"pressure"`
	PressureLimit    int                   `json:"pressure_limit"`
	PressureLabel    string                `json:"pressure_label"`
	PressureLabelKey string                `json:"pressure_label_key"`
	Setback          bool                  `json:"setback"`
	ReturnDueSeconds int64                 `json:"return_due_seconds,omitempty"`
	ReturnAvailable  bool                  `json:"return_available"`
	ReturnOpened     bool                  `json:"return_opened"`
	Approaches       []ExploreApproachView `json:"approaches"`
}

const ActionExplore InstanceActionKind = "explore"
