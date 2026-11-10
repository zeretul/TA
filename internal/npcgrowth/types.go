// Package npcgrowth derives NPC cultivation candidates from sourced,
// already-completed world intervals. It does not own actors, time, or persistence.
package npcgrowth

import (
	"encoding/json"

	"ta/internal/cultivation"
)

const InputVersion = 1
const maxSourceRefLength = 256

type Activity string

const (
	ActivityFocusedSeclusion  Activity = "focused_seclusion"
	ActivityOrdinaryTraining  Activity = "ordinary_life_with_training"
	ActivityTravelOpportunity Activity = "travel_with_training_opportunity"
	ActivityTravelNoWindow    Activity = "travel_without_training_window"
	ActivityInjuryPrevents    Activity = "injury_prevents_training"
	// ActivityOccupied is §4's 0% row for a character whose arrangement leaves no
	// training time at all. It is a separate value from ActivityInjuryPrevents
	// because the two have different causes and different exits: a wound clears
	// under the recovery rules, while an arrangement changes when the character's
	// own committed plan changes. Both are 0%, but a record that says "busy" must
	// not read as "wounded" to anyone auditing the interval.
	ActivityOccupied         Activity = "occupied_without_training"
	ActivityBreakthroughPrep Activity = "breakthrough_preparation"
	ActivityHealthRest        Activity = "health_rest"
	ActivitySafeMeditation    Activity = "safe_meditation"
)

// Input combines one formal actor snapshot with a completed, sourced interval.
// Its revision is the optimistic concurrency token the Story commit must check.
type Input struct {
	SchemaVersion      int                 `json:"schema_version"`
	ActorID            string              `json:"actor_id"`
	ActorSource        string              `json:"actor_source_ref"`
	BaseRevision       uint64              `json:"base_revision"`
	State              Snapshot            `json:"state"`
	Interval           Interval            `json:"interval"`
	BreakthroughIntent *BreakthroughIntent `json:"breakthrough_intent,omitempty"`
}

// Snapshot is a projection of the authoritative actor state and settlement
// watermark. The caller must read it from the current Story branch.
type Snapshot struct {
	SourceRef             string                  `json:"source_ref"`
	Revision              uint64                  `json:"revision"`
	Cultivation           cultivation.Cultivation `json:"cultivation"`
	Accumulated           float64                 `json:"accumulated"`
	SettledThroughSeconds int64                   `json:"settled_through_seconds"`
	LastIntervalID        string                  `json:"last_interval_id"`
	LastIntervalHash      string                  `json:"last_interval_hash"`
	Breakthrough          *BreakthroughState      `json:"breakthrough,omitempty"`
	Resources             *ActorPools             `json:"resources,omitempty"`
	SupportDoses          []SupportDose           `json:"support_doses,omitempty"`
	ActiveSupport         *ActiveSupport          `json:"active_support,omitempty"`
	Recovery              *RecoveryActivity       `json:"recovery,omitempty"`
}

// MarshalJSON keeps the cultivation projection symmetric with DecodeInput.
// The formal cultivation type omits zero-valued stage and layer fields, while
// the growth input contract requires all three coordinates to be present.
func (snapshot Snapshot) MarshalJSON() ([]byte, error) {
	type cultivationState struct {
		Realm cultivation.Realm `json:"realm"`
		Stage cultivation.Stage `json:"stage"`
		Layer int               `json:"layer"`
	}
	type snapshotState struct {
		SourceRef             string             `json:"source_ref"`
		Revision              uint64             `json:"revision"`
		Cultivation           cultivationState   `json:"cultivation"`
		Accumulated           float64            `json:"accumulated"`
		SettledThroughSeconds int64              `json:"settled_through_seconds"`
		LastIntervalID        string             `json:"last_interval_id"`
		LastIntervalHash      string             `json:"last_interval_hash"`
		Breakthrough          *BreakthroughState `json:"breakthrough,omitempty"`
		Resources             *ActorPools        `json:"resources,omitempty"`
		SupportDoses          []SupportDose      `json:"support_doses,omitempty"`
		ActiveSupport         *ActiveSupport     `json:"active_support,omitempty"`
		Recovery              *RecoveryActivity  `json:"recovery,omitempty"`
	}
	return json.Marshal(snapshotState{
		SourceRef: snapshot.SourceRef,
		Revision:  snapshot.Revision,
		Cultivation: cultivationState{
			Realm: snapshot.Cultivation.Realm,
			Stage: snapshot.Cultivation.Stage,
			Layer: snapshot.Cultivation.Layer,
		},
		Accumulated:           snapshot.Accumulated,
		SettledThroughSeconds: snapshot.SettledThroughSeconds,
		LastIntervalID:        snapshot.LastIntervalID,
		LastIntervalHash:      snapshot.LastIntervalHash,
		Breakthrough:          snapshot.Breakthrough,
		Resources:             snapshot.Resources,
		SupportDoses:          snapshot.SupportDoses,
		ActiveSupport:         snapshot.ActiveSupport,
		Recovery:              snapshot.Recovery,
	})
}

// Interval is one caller-approved portion of canonical world time.
type Interval struct {
	ID        string    `json:"id"`
	SourceRef string    `json:"source_ref"`
	From      int64     `json:"from_seconds"`
	To        int64     `json:"to_seconds"`
	Segments  []Segment `json:"segments"`
}

// Segment boundaries represent actual changes in an approved activity or condition.
// Segment is a contiguous span with one legal activity and condition set.
type Segment struct {
	From                       int64       `json:"from_seconds"`
	To                         int64       `json:"to_seconds"`
	Activity                   Activity    `json:"activity"`
	ActivitySourceRef          string      `json:"activity_source_ref"`
	OpportunitySourceRef       string      `json:"opportunity_source_ref"`
	Conditions                 Conditions  `json:"conditions"`
	SupportUse                 *SupportUse `json:"support_use,omitempty"`
	RecoveryActivityID         string      `json:"recovery_activity_id,omitempty"`
	RecoverySourceRef          string      `json:"recovery_source_ref,omitempty"`
	RecoveryEligibilityAtStart string      `json:"recovery_eligibility_at_start,omitempty"`
	RecoveryEligibilityNow     string      `json:"recovery_eligibility_now,omitempty"`
}

// Conditions are sourced values resolved by the game layer. Resource support is
// rejected until its consumption can be committed with the growth.
type Conditions struct {
	SourceRef         string  `json:"source_ref"`
	Talent            string  `json:"talent"`
	Technique         string  `json:"technique_quality"`
	TechniqueRef      string  `json:"technique_source_ref"`
	GrowthCoefficient float64 `json:"growth_coefficient"`
	Environment       string  `json:"environment"`
	Resource          string  `json:"support_resource"`
	CurrentHealth     int     `json:"current_health"`
	MaximumHealth     int     `json:"maximum_health"`
	CurrentMana       int     `json:"current_mana,omitempty"`
	MaximumMana       int     `json:"maximum_mana,omitempty"`
}

// SupportDose is an actual inventory stack held by this NPC, not a catalogue entry.
type SupportDose struct {
	InstanceID   string `json:"instance_id"`
	DefinitionID string `json:"definition_id"`
	Quantity     int    `json:"quantity"`
	SourceRef    string `json:"source_ref"`
}

// SupportUse is the caller's explicit decision to consume one held dose at a segment start.
type SupportUse struct {
	InstanceID string `json:"instance_id"`
	SourceRef  string `json:"source_ref"`
}

// ActiveSupport records the one consumed dose whose registered effect is still in force.
type ActiveSupport struct {
	InstanceID       string `json:"instance_id"`
	DefinitionID     string `json:"definition_id"`
	SourceRef        string `json:"source_ref"`
	StartedAtSeconds int64  `json:"started_at_seconds"`
	EndsAtSeconds    int64  `json:"ends_at_seconds"`
	Strength         string `json:"strength"`
}

// RecoveryActivity stores the cumulative origin needed by PlanSafeRecovery across snapshots.
type RecoveryActivity struct {
	ID                 string `json:"id"`
	SourceRef          string `json:"source_ref"`
	Mode               string `json:"mode"`
	EligibilityAtStart string `json:"eligibility_at_start"`
	StartedAtSeconds   int64  `json:"started_at_seconds"`
	SettledSeconds     int64  `json:"settled_seconds"`
	StartHealth        int    `json:"start_health"`
	StartMana          int    `json:"start_mana"`
	StartHealthMaximum int    `json:"start_health_maximum"`
	StartManaMaximum   int    `json:"start_mana_maximum"`
}

// ActorPools carries the NPC's actual current and maximum health and mana in the same actor snapshot.
type ActorPools struct {
	Health        int `json:"health"`
	HealthMaximum int `json:"health_maximum"`
	Mana          int `json:"mana"`
	ManaMaximum   int `json:"mana_maximum"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type DiagnosticError struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (e *DiagnosticError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "NPC growth request is invalid"
	}
	return e.Diagnostics[0].Message
}

type Status string

const (
	StatusApplied    Status = "applied"
	StatusDuplicate  Status = "duplicate"
	StatusAtBoundary Status = "awaiting_breakthrough_decision"
)

// Candidate is proposed state only; it becomes fact only after Story commits it.
type Candidate struct {
	Status             Status                         `json:"status"`
	ActorID            string                         `json:"actor_id"`
	IntervalID         string                         `json:"interval_id"`
	IntervalHash       string                         `json:"interval_hash"`
	ExpectedRevision   uint64                         `json:"expected_revision"`
	NextState          Snapshot                       `json:"next_state"`
	HasStateChange     bool                           `json:"has_state_change"`
	ConsumedThrough    int64                          `json:"consumed_through_seconds"`
	EffectiveSeconds10 int64                          `json:"effective_seconds_tenths"`
	Gained             float64                        `json:"gained"`
	StagesAdvanced     int                            `json:"stages_advanced"`
	Segments           []SegmentResult                `json:"segments"`
	RemainingIntent    *RemainingIntent               `json:"remaining_intent,omitempty"`
	Breakthrough       *BreakthroughCandidate         `json:"breakthrough,omitempty"`
	Recovery           *cultivation.RecoveryCandidate `json:"recovery,omitempty"`
}

type SegmentResult struct {
	From               int64                   `json:"from_seconds"`
	To                 int64                   `json:"to_seconds"`
	EffectiveSeconds10 int64                   `json:"effective_seconds_tenths"`
	Cultivation        cultivation.Cultivation `json:"cultivation"`
	Accumulated        float64                 `json:"accumulated"`
	Gained             float64                 `json:"gained"`
	StagesAdvanced     int                     `json:"stages_advanced"`
	SupportResource    string                  `json:"support_resource"`
}

type RemainingIntent struct {
	From     int64     `json:"from_seconds"`
	To       int64     `json:"to_seconds"`
	Reason   string    `json:"reason"`
	Segments []Segment `json:"segments"`
}
