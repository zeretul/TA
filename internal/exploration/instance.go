package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const (
	InstanceSnapshotSchemaVersion = 1
	MaxInstanceDiscoveries        = MaxKnowledgeItems
	MaxInstanceProgressRecords    = MaxKnowledgeItems
)

// InstancePresence distinguishes a real interior position from an outside
// historical exit reference.
type InstancePresence string

const (
	InstanceInside  InstancePresence = "inside"
	InstanceOutside InstancePresence = "outside"
)

type InstanceDiscoveryKind string

const (
	DiscoveryArea       InstanceDiscoveryKind = "area"
	DiscoveryConnection InstanceDiscoveryKind = "connection"
	DiscoveryObjective  InstanceDiscoveryKind = "objective"
	DiscoveryFact       InstanceDiscoveryKind = "fact"
	DiscoveryRumor      InstanceDiscoveryKind = "rumor"
)

type InstanceProgressKind string

const (
	ProgressObjective InstanceProgressKind = "objective"
	ProgressChallenge InstanceProgressKind = "challenge"
)

// ActionDecision is a trusted caller's explicit action judgment. The empty
// value is never treated as permission.
type ActionDecision string

const (
	DecisionUnspecified ActionDecision = ""
	DecisionAllowed     ActionDecision = "allowed"
	DecisionBlocked     ActionDecision = "blocked"
)

// BattleState must explicitly report no active battle before travel actions.
type BattleState string

const (
	BattleUnknown BattleState = ""
	BattleNone    BattleState = "none"
	BattleActive  BattleState = "active"
)

// BlockingState must explicitly report no pending scene block before travel.
type BlockingState string

const (
	BlockingUnknown BlockingState = ""
	BlockingNone    BlockingState = "none"
	BlockingPending BlockingState = "pending"
)

// InstanceActionKind binds a permit to one operation family.
type InstanceActionKind string

const (
	ActionEnter    InstanceActionKind = "enter"
	ActionDiscover InstanceActionKind = "discover"
	ActionMove     InstanceActionKind = "move"
	ActionProgress InstanceActionKind = "progress"
	ActionExit     InstanceActionKind = "exit"
	ActionReenter  InstanceActionKind = "reenter"
)

// InstancePermitBinding identifies the exact instance revision for which a
// trusted action decision was prepared. It is a consistency guard, not a
// signature or a replacement for application-layer fact validation.
type InstancePermitBinding struct {
	InstanceID     string `json:"instance_id"`
	WorldID        string `json:"world_id"`
	BranchID       string `json:"branch_id"`
	DefinitionHash string `json:"definition_hash"`
	Revision       uint64 `json:"revision"`
}

// InstanceActionPermit binds a trusted source reference and decision to an
// exact action target and an exact instance revision; movement targets include
// directed endpoints.
type InstanceActionPermit struct {
	Binding    InstancePermitBinding `json:"binding"`
	Decision   ActionDecision        `json:"decision"`
	Action     InstanceActionKind    `json:"action"`
	TargetID   string                `json:"target_id"`
	FromAreaID string                `json:"from_area_id,omitempty"`
	ToAreaID   string                `json:"to_area_id,omitempty"`
	SourceRef  string                `json:"source_ref"`
}

// InstanceActionContext is supplied from facts already checked by the product
// layer. This package validates its shape and binding, not the existence or
// meaning of SourceRef, nor arbitrary natural-language conditions.
type InstanceActionContext struct {
	ExpectedRevision uint64               `json:"expected_revision"`
	Scope            DungeonScope         `json:"scope"`
	Permit           InstanceActionPermit `json:"permit"`
	Battle           BattleState          `json:"battle"`
	Blocking         BlockingState        `json:"blocking"`
}

// InstanceDiscovery records one caller-confirmed, sourced fact or rumor.
type InstanceDiscovery struct {
	Kind                InstanceDiscoveryKind `json:"kind"`
	ID                  string                `json:"id"`
	SourceRef           string                `json:"source_ref"`
	Text                string                `json:"text,omitempty"`
	TextKey             string                `json:"text_key,omitempty"`
	PubliclyDisplayable bool                  `json:"publicly_displayable,omitempty"`
}

// InstanceProgress accepts only an explicitly confirmed result.
type InstanceProgress struct {
	Kind      InstanceProgressKind `json:"kind"`
	ID        string               `json:"id"`
	SourceRef string               `json:"source_ref"`
	Outcome   ResultOutcome        `json:"outcome"`
}

type ResultOutcome string

const (
	OutcomeUnspecified ResultOutcome = ""
	OutcomeConfirmed   ResultOutcome = "confirmed"
	OutcomeUnconfirmed ResultOutcome = "unconfirmed"
)

// SourcedProgress preserves the result source for an objective or challenge.
type SourcedProgress struct {
	ID        string `json:"id"`
	SourceRef string `json:"source_ref"`
}

// DungeonInstanceSnapshot is a proposal representation. Product storage must
// bind and commit it through the canonical journal with an expected revision.
type DungeonInstanceSnapshot struct {
	SchemaVersion       int                    `json:"schema_version"`
	Binding             DungeonInstanceBinding `json:"binding"`
	DefinitionHash      string                 `json:"definition_hash"`
	Revision            uint64                 `json:"revision"`
	Presence            InstancePresence       `json:"presence"`
	CurrentAreaID       string                 `json:"current_area_id,omitempty"`
	LastExitAreaID      string                 `json:"last_exit_area_id,omitempty"`
	Discoveries         []InstanceDiscovery    `json:"discoveries"`
	CompletedObjectives []SourcedProgress      `json:"completed_objectives"`
	ResolvedChallenges  []SourcedProgress      `json:"resolved_challenges"`
	// GrantedRewards is what this instance has really handed over. It is part of
	// the snapshot rather than the caller's memory so a re-entry, a restart and a
	// retry all read the same answer: a taken reward stays taken.
	GrantedRewards []InstanceReward `json:"granted_rewards,omitempty"`
	ExploreState   *ExploreState    `json:"explore_state,omitempty"`
}

type InstanceChange struct {
	Kind      string `json:"kind"`
	ID        string `json:"id,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

// InstanceDiff is the deterministic change proposal between two revisions.
type InstanceDiff struct {
	FromRevision uint64           `json:"from_revision"`
	ToRevision   uint64           `json:"to_revision"`
	Changes      []InstanceChange `json:"changes"`
}

type InstanceTransition struct {
	Snapshot DungeonInstanceSnapshot `json:"snapshot"`
	Diff     InstanceDiff            `json:"diff"`
}

type CreateDungeonInstanceInput struct {
	Definition         DungeonDefinition    `json:"definition"`
	Scope              DungeonScope         `json:"scope"`
	InstanceID         string               `json:"instance_id"`
	EntryAreaID        string               `json:"entry_area_id"`
	Permit             InstanceActionPermit `json:"permit"`
	Battle             BattleState          `json:"battle"`
	Blocking           BlockingState        `json:"blocking"`
	InitialDiscoveries []InstanceDiscovery  `json:"initial_discoveries"`
}

type MoveDungeonInstanceInput struct {
	ConnectionID      string `json:"connection_id"`
	DestinationAreaID string `json:"destination_area_id"`
}

type DungeonInstanceView struct {
	InstanceID          string              `json:"instance_id"`
	Presence            InstancePresence    `json:"presence"`
	CurrentArea         *Area               `json:"current_area,omitempty"`
	LastExitAreaID      string              `json:"last_exit_area_id,omitempty"`
	Objectives          []Objective         `json:"objectives"`
	Areas               []Area              `json:"areas"`
	Connections         []DungeonConnection `json:"connections"`
	ConfirmedFacts      []KnownFact         `json:"confirmed_facts"`
	Rumors              []KnownFact         `json:"rumors"`
	CompletedObjectives []string            `json:"completed_objectives"`
	ResolvedChallenges  []string            `json:"resolved_challenges"`
	// GrantedRewards is what the character has already taken from this realm, so
	// the detail panel can show it as obtained rather than as still available.
	GrantedRewards []InstanceReward `json:"granted_rewards"`
}

var (
	ErrInvalidInstance       ErrorCode = "invalid dungeon instance snapshot"
	ErrStaleInstanceRevision ErrorCode = "stale dungeon instance revision"
	ErrActionNotAllowed      ErrorCode = "dungeon instance action not allowed"
	ErrActionBlocked         ErrorCode = "dungeon instance action blocked"
	ErrInstanceConflict      ErrorCode = "conflicting dungeon instance fact"
	ErrInvalidInstanceAction ErrorCode = "invalid dungeon instance action context"
	ErrRevisionOverflow      ErrorCode = "dungeon instance revision overflow"
	ErrInstanceCapacity      ErrorCode = "dungeon instance capacity exceeded"
)

// DungeonDefinitionContentHash hashes the complete typed definition as stable
// JSON. The definition contains ordered slices and no maps; changing any
// semantic field or array element changes the hash.
func DungeonDefinitionContentHash(def DungeonDefinition) (string, error) {
	encoded, err := json.Marshal(def)
	if err != nil {
		return "", fmt.Errorf("%w: cannot encode definition: %v", ErrInvalidDefinition, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// CreateDungeonInstance proposes a new snapshot at a declared legal entry.
// Checking whether an instance already exists belongs to the application.
func CreateDungeonInstance(input CreateDungeonInstanceInput) (InstanceTransition, error) {
	validation := ValidateDungeon(input.Definition, Catalog{}, nil)
	if len(validation.Diagnostics) > 0 || !validation.Playable {
		return InstanceTransition{}, fmt.Errorf("%w: definition must have valid goals, paths, and a reachable exit", ErrInvalidDefinition)
	}
	if !validID(input.InstanceID) || !validID(input.Scope.WorldID) || !validID(input.Scope.BranchID) {
		return InstanceTransition{}, fmt.Errorf("%w: instance, world, and branch IDs must be valid", ErrInvalidBinding)
	}
	if !containsString(input.Definition.EntryAreaIDs, input.EntryAreaID) {
		return InstanceTransition{}, fmt.Errorf("%w: requested entry is not a defined entry area", ErrInvalidInstanceAction)
	}
	if err := validatePermit(input.Permit, ActionEnter, input.EntryAreaID, "", ""); err != nil {
		return InstanceTransition{}, err
	}
	if err := validateClearScene(input.Battle, input.Blocking); err != nil {
		return InstanceTransition{}, err
	}
	hash, err := DungeonDefinitionContentHash(input.Definition)
	if err != nil {
		return InstanceTransition{}, err
	}
	expectedPermitBinding := InstancePermitBinding{InstanceID: input.InstanceID, WorldID: input.Scope.WorldID, BranchID: input.Scope.BranchID, DefinitionHash: hash, Revision: 0}
	if input.Permit.Binding != expectedPermitBinding {
		return InstanceTransition{}, fmt.Errorf("%w: create permit must bind the new instance, trusted scope, definition content, and revision zero", ErrInvalidBinding)
	}
	binding := DungeonInstanceBinding{InstanceID: input.InstanceID, WorldID: input.Scope.WorldID, BranchID: input.Scope.BranchID,
		DefinitionID: input.Definition.ID, DefinitionVersion: input.Definition.Version, PackageID: input.Definition.PackageID,
		PackageVersion: input.Definition.PackageVersion, RulesetID: input.Definition.RulesetID, RulesetVersion: input.Definition.RulesetVersion}
	snapshot := DungeonInstanceSnapshot{SchemaVersion: InstanceSnapshotSchemaVersion, Binding: binding, DefinitionHash: hash, Revision: 1,
		Presence: InstanceInside, CurrentAreaID: input.EntryAreaID, Discoveries: []InstanceDiscovery{{Kind: DiscoveryArea, ID: input.EntryAreaID, SourceRef: input.Permit.SourceRef}},
		CompletedObjectives: []SourcedProgress{}, ResolvedChallenges: []SourcedProgress{}}
	if input.Definition.ExploreRules != nil {
		snapshot.ExploreState = &ExploreState{UsedApproachIDs: []string{}, Decisions: []ExploreDecision{}}
	}
	if err := applyInitialDiscoveries(&snapshot, input.Definition, input.InitialDiscoveries); err != nil {
		return InstanceTransition{}, err
	}
	if diagnostics := ValidateDungeonInstance(input.Definition, input.Scope, snapshot); len(diagnostics) > 0 {
		return InstanceTransition{}, instanceDiagnosticError(diagnostics)
	}
	changes := []InstanceChange{{Kind: "instance_created", ID: input.InstanceID, SourceRef: input.Permit.SourceRef}, {Kind: "presence_changed", From: "", To: string(InstanceInside), SourceRef: input.Permit.SourceRef}, {Kind: "position_changed", From: "", To: input.EntryAreaID, SourceRef: input.Permit.SourceRef}}
	for _, item := range snapshot.Discoveries {
		changes = append(changes, discoveryChange(item))
	}
	sortChanges(changes)
	return InstanceTransition{Snapshot: cloneInstanceSnapshot(snapshot), Diff: InstanceDiff{FromRevision: 0, ToRevision: 1, Changes: changes}}, nil
}

// ValidateDungeonInstance checks scope, exact definition content, revision,
// presence, knowledge references, and sourced progress.
func ValidateDungeonInstance(def DungeonDefinition, scope DungeonScope, snapshot DungeonInstanceSnapshot) []Diagnostic {
	var ds []Diagnostic
	if snapshot.SchemaVersion != InstanceSnapshotSchemaVersion {
		ds = append(ds, Diagnostic{"unsupported_instance_schema", "schema_version", "unsupported instance snapshot version"})
	}
	ds = append(ds, ValidateInstanceBinding(snapshot.Binding, scope, def)...)
	hash, err := DungeonDefinitionContentHash(def)
	if err != nil || snapshot.DefinitionHash != hash {
		ds = append(ds, Diagnostic{"definition_content_mismatch", "definition_hash", "instance content hash does not match the selected definition"})
	}
	if snapshot.Revision == 0 {
		ds = append(ds, Diagnostic{"invalid_revision", "revision", "instance revision must be positive"})
	}
	if len(snapshot.Discoveries) > MaxInstanceDiscoveries {
		ds = append(ds, Diagnostic{"too_many_discoveries", "discoveries", "discovery count exceeds the supported bound"})
	}
	if len(snapshot.CompletedObjectives)+len(snapshot.ResolvedChallenges) > MaxInstanceProgressRecords {
		ds = append(ds, Diagnostic{"too_many_progress_records", "progress", "progress count exceeds the supported bound"})
	}
	if len(snapshot.GrantedRewards) > MaxInstanceRewards {
		ds = append(ds, Diagnostic{"too_many_rewards", "granted_rewards", "granted reward count exceeds the supported bound"})
	}
	areaByID, connectionByID, objectiveByID, challengeByID := definitionIndexes(def)
	definitionCheck := ValidateDungeonDefinition(def)
	ds = append(ds, definitionCheck...)
	seenDiscovery := map[string]InstanceDiscovery{}
	seenFact := map[string]InstanceDiscovery{}
	knownAreas := map[string]bool{}
	knownObjectives := map[string]bool{}
	for i, item := range snapshot.Discoveries {
		path := fmt.Sprintf("discoveries[%d]", i)
		if !validID(item.ID) || !validID(item.SourceRef) {
			ds = append(ds, Diagnostic{"invalid_discovery", path, "discovery requires stable ID and source reference"})
			continue
		}
		key := string(item.Kind) + "\x00" + item.ID
		if prior, exists := seenDiscovery[key]; exists {
			if !sameDiscoveryContent(prior, item) {
				ds = append(ds, Diagnostic{"conflicting_discovery", path, "duplicate discovery has different content"})
			} else {
				ds = append(ds, Diagnostic{"duplicate_discovery", path, "discovery is duplicated"})
			}
			continue
		}
		seenDiscovery[key] = item
		switch item.Kind {
		case DiscoveryArea:
			if _, exists := areaByID[item.ID]; !exists {
				ds = append(ds, Diagnostic{"unknown_discovery_area", path + ".id", "area does not exist in the definition"})
			}
			if item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
				ds = append(ds, Diagnostic{"invalid_discovery_content", path, "area discovery cannot contain fact text or display flags"})
			}
			knownAreas[item.ID] = true
		case DiscoveryConnection:
			if _, exists := connectionByID[item.ID]; !exists {
				ds = append(ds, Diagnostic{"unknown_discovery_connection", path + ".id", "connection does not exist in the definition"})
			}
			if item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
				ds = append(ds, Diagnostic{"invalid_discovery_content", path, "connection discovery cannot contain fact text or display flags"})
			}
		case DiscoveryObjective:
			if _, exists := objectiveByID[item.ID]; !exists {
				ds = append(ds, Diagnostic{"unknown_discovery_objective", path + ".id", "objective does not exist in the definition"})
			}
			if item.Text != "" || item.TextKey != "" || item.PubliclyDisplayable {
				ds = append(ds, Diagnostic{"invalid_discovery_content", path, "objective discovery cannot contain fact text or display flags"})
			}
			knownObjectives[item.ID] = true
		case DiscoveryFact, DiscoveryRumor:
			if !validText(item.Text, MaxDescriptionBytes) {
				ds = append(ds, Diagnostic{"invalid_discovery_text", path + ".text", "fact requires non-empty bounded text"})
			}
			if prior, exists := seenFact[item.ID]; exists {
				if prior.Kind != item.Kind {
					ds = append(ds, Diagnostic{"conflicting_knowledge_kind", path, "an item cannot be both confirmed fact and rumor"})
				} else if prior.Text != item.Text || prior.TextKey != item.TextKey || prior.PubliclyDisplayable != item.PubliclyDisplayable {
					ds = append(ds, Diagnostic{"conflicting_discovery", path, "fact ID has conflicting content"})
				}
			} else {
				seenFact[item.ID] = item
			}
		default:
			ds = append(ds, Diagnostic{"invalid_discovery_kind", path + ".kind", "discovery kind is not supported"})
		}
	}
	for i, progress := range snapshot.CompletedObjectives {
		path := fmt.Sprintf("completed_objectives[%d]", i)
		if _, exists := objectiveByID[progress.ID]; !exists || !knownObjectives[progress.ID] || !validID(progress.SourceRef) {
			ds = append(ds, Diagnostic{"invalid_objective_progress", path, "completed objective must exist, be known, and have a source reference"})
		}
		if i > 0 && containsSourcedID(snapshot.CompletedObjectives[:i], progress.ID) {
			ds = append(ds, Diagnostic{"duplicate_progress", path + ".id", "objective progress is duplicated"})
		}
	}
	for i, progress := range snapshot.ResolvedChallenges {
		path := fmt.Sprintf("resolved_challenges[%d]", i)
		if _, exists := challengeByID[progress.ID]; !exists || !validID(progress.SourceRef) {
			ds = append(ds, Diagnostic{"invalid_challenge_progress", path, "resolved challenge must exist and have a source reference"})
		}
		if i > 0 && containsSourcedID(snapshot.ResolvedChallenges[:i], progress.ID) {
			ds = append(ds, Diagnostic{"duplicate_progress", path + ".id", "challenge progress is duplicated"})
		}
	}
	// A granted reward is checked against the definition it was declared in, so a
	// stored grant cannot name a catalogue entry the realm never offered or a
	// quantity the realm never fixed.
	declaredRewards := map[string]ContentRef{}
	for _, reference := range def.ContentRefs {
		if reference.Kind == "reward" {
			declaredRewards[reference.ID] = reference
		}
	}
	for i, reward := range snapshot.GrantedRewards {
		path := fmt.Sprintf("granted_rewards[%d]", i)
		reference, declared := declaredRewards[reward.ID]
		if !declared || reference.ReferenceID != reward.RefID || reference.Quantity != reward.Quantity || !validID(reward.SourceRef) {
			ds = append(ds, Diagnostic{"invalid_granted_reward", path, "granted reward must match a declared reward reference and carry a source"})
		}
		if i > 0 {
			for _, prior := range snapshot.GrantedRewards[:i] {
				if prior.ID == reward.ID {
					ds = append(ds, Diagnostic{"duplicate_reward", path + ".id", "a declared reward can only be granted once"})
					break
				}
			}
		}
	}
	if snapshot.Presence == InstanceInside {
		if !validID(snapshot.CurrentAreaID) || !knownAreas[snapshot.CurrentAreaID] {
			ds = append(ds, Diagnostic{"invalid_current_area", "current_area_id", "inside instance must have a known defined current area"})
		}
		if snapshot.LastExitAreaID != "" && (!containsString(def.ExitAreaIDs, snapshot.LastExitAreaID) || !knownAreas[snapshot.LastExitAreaID]) {
			ds = append(ds, Diagnostic{"invalid_last_exit_area", "last_exit_area_id", "historical exit must be a known legal exit"})
		}
	} else if snapshot.Presence == InstanceOutside {
		if snapshot.CurrentAreaID != "" {
			ds = append(ds, Diagnostic{"outside_has_current_area", "current_area_id", "outside instance cannot claim a current dungeon area"})
		}
		if !validID(snapshot.LastExitAreaID) || !containsString(def.ExitAreaIDs, snapshot.LastExitAreaID) || !knownAreas[snapshot.LastExitAreaID] {
			ds = append(ds, Diagnostic{"invalid_last_exit_area", "last_exit_area_id", "outside instance must retain a known legal exit as historical position"})
		}
	} else {
		ds = append(ds, Diagnostic{"invalid_instance_presence", "presence", "presence must be inside or outside"})
	}
	ds = append(ds, validateExploreState(def, snapshot)...)
	return ds
}
