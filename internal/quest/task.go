// Package quest provides deterministic, storage-free task rules: what an
// authored task requires, when a committed fact satisfies one of its
// objectives, and what a settled task may pay.
//
// It plans no prose and persists nothing. A task instance is a story fact and
// lives on the story journal; this package only decides whether a fact really
// answers an objective and what the authored task says should follow.
package quest

import (
	"fmt"
	"sort"
	"strings"
)

const (
	SchemaVersion      = 1
	MaxTaskBytes       = 1 << 20
	MaxIdentifierBytes = 128
	MaxTextBytes       = 8 << 10
	MaxObjectives      = 32
	MaxRewards         = 16
	MaxQuantity        = 1_000_000_000
	// Frozen adventure tables permit at most 100 ranks and ten skill ranks.
	// The owning world still enforces its smaller bound before recording a choice.
	MaxChoiceRank       = 100
	MaxChoiceSkillRank  = 10
	MaxChoiceCapability = MaxChoiceRank + MaxChoiceSkillRank
	// MaxDeadlineSeconds bounds an authored limit at one hundred years, which is
	// longer than any mortal life in this world and therefore never the real
	// constraint. It exists only so an interval added to a clock cannot overflow.
	MaxDeadlineSeconds = 100 * 365 * 24 * 60 * 60
)

// ErrorCode is an immutable sentinel suitable for errors.Is.
type ErrorCode string

func (code ErrorCode) Error() string { return string(code) }

const (
	ErrInvalidTask     ErrorCode = "invalid task definition"
	ErrInvalidState    ErrorCode = "invalid task state"
	ErrObjectiveNotMet ErrorCode = "objective is not satisfied"
	ErrTaskNotAccepted ErrorCode = "task is not accepted"
	ErrTaskSettled     ErrorCode = "task is already settled"
	ErrInputTooLarge   ErrorCode = "input exceeds supported size"
)

// Diagnostic points to one invalid field or reference. Paths and messages are
// deterministic English intended for tools and logs.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// TaskKind separates the three content layers of 自制-事件与任务 §4.2.
//
// They share one rule set and one instance shape; the kind only says who the
// task is about, which is what a reader needs to file it under the right
// heading in 见闻与事务.
type TaskKind string

const (
	// KindErrand is an ordinary chore: a delivery, a check, a piece of work.
	KindErrand TaskKind = "errand"
	// KindHunt is a hunt, whose objective is met by a real battle result.
	KindHunt TaskKind = "hunt"
	// KindPersonal is a personal matter raised by one named person.
	KindPersonal TaskKind = "personal"
)

// EvidenceKind names the committed fact that can satisfy an objective.
//
// Only facts the game already settles exist here. A task cannot be completed by
// prose saying it was, for the same reason a realm reward cannot: the rules
// decide, and the narrative describes what they decided.
type EvidenceKind string

const (
	// EvidenceRealmReward is satisfied when a realm instance has really handed
	// over the named declared reward.
	EvidenceRealmReward EvidenceKind = "realm_reward"
	// EvidenceJourneyArrival is satisfied when a journey has really arrived at
	// the named place.
	EvidenceJourneyArrival EvidenceKind = "journey_arrival"
	// EvidenceCombatOutcome is satisfied when a battle has settled with the
	// named result.
	EvidenceCombatOutcome EvidenceKind = "combat_outcome"
	EvidenceChoice        EvidenceKind = "choice"
)

// Evidence is one committed fact an objective accepts.
type Evidence struct {
	Kind EvidenceKind `json:"kind"`
	// RefID is the reward's registered catalogue id, the place id, or the
	// combat result, depending on Kind.
	RefID string `json:"ref_id"`
}

// Objective is one thing the task asks for, judged by a committed fact.
//
// An objective is never ticked by a narrative sentence or by the interface. It
// is ticked when the instance records that the named fact happened, which is
// why Evidence only names facts the rules layer already settles.
type Objective struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Evidence lists the facts that satisfy this objective. Any one of them is
	// enough; an objective with none can never be completed, which the
	// validator refuses rather than shipping a goal nobody can reach.
	Evidence []Evidence `json:"evidence"`
}

// RewardKind names what a settled task may pay.
type RewardKind string

const (
	// RewardItem pays one registered catalogue item.
	RewardItem RewardKind = "item"
	// RewardCurrency pays spirit stones into the character's own purse.
	RewardCurrency   RewardKind = "currency"
	RewardExperience RewardKind = "experience"
)

// Reward is one payment the task promises.
//
// §4.7 requires an agreed reward to be stated before acceptance and to be
// settled once. Both properties are why the amount lives here, in the authored
// definition, rather than in a request.
type Reward struct {
	Kind RewardKind `json:"kind"`
	// RefID is the catalogue entry for RewardItem and the currency field for
	// RewardCurrency.
	RefID    string `json:"ref_id"`
	Quantity int64  `json:"quantity"`
}

// Definition is one authored task.
type Definition struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Version       string `json:"version"`
	// PackageID and PackageVersion name the world pack revision this task was
	// authored against. §4.10 of 自制-事件与任务 requires a task to declare its
	// formal source and the world version it applies to; a task from another
	// world has no business being offered in this story.
	PackageID      string   `json:"package_id"`
	PackageVersion string   `json:"package_version"`
	Kind           TaskKind `json:"kind"`
	// Title and Brief are the task's own wording, so a panel never invents one.
	Title string `json:"title"`
	Brief string `json:"brief"`
	// Giver is the person who raises it, empty for an ordinary errand that has
	// no one person behind it.
	Giver string `json:"giver,omitempty"`
	// PublicLocationIDs identifies places where an authored public notice makes
	// this opportunity known. A personal matter has no public locations: being
	// in the author's catalog is not evidence that the player heard about it.
	PublicLocationIDs []string `json:"public_location_ids,omitempty"`
	// DeadlineSeconds is the time the task allows from the moment it is
	// accepted, in formal story seconds. Zero means no deadline is agreed, which
	// the panel reports as 未约定期限 rather than inventing a countdown.
	//
	// It is a duration rather than a fixed world instant because §五 of
	// 自制-并行事项与到期事件 keeps "accepted at" and "closes at" apart: the same
	// authored task taken a week later runs a week later. A fixed world instant
	// belongs to a public event's stage table, not to a personal agreement.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`
	// Objectives are all required: a task is complete when every one is
	// satisfied, which keeps "partial" an honest state rather than a second
	// definition.
	Objectives                []Objective     `json:"objectives"`
	Rewards                   []Reward        `json:"rewards"`
	Choices                   []Choice        `json:"choices,omitempty"`
	RequiresFactionMembership string          `json:"requires_faction_membership,omitempty"`
	RequiresEcho              *EchoGate       `json:"requires_echo,omitempty"`
	CompletionEcho            *CompletionEcho `json:"completion_echo,omitempty"`
	// FreshFactsOnly requires a newly agreed repetition, not credit for an earlier visit.
	FreshFactsOnly bool `json:"fresh_facts_only,omitempty"`
}

// validID mirrors the identifier rule the rest of the product uses, so a task id
// can be carried in a record without escaping.
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

// ValidateDefinition checks an authored task before it can be offered.
//
// It refuses the two shapes that would make a task unwinnable or a payment
// ambiguous: an objective no fact can satisfy, and a reward that names nothing
// or pays nothing.
func ValidateDefinition(definition Definition) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(code, path, message string) {
		diagnostics = append(diagnostics, Diagnostic{Code: code, Path: path, Message: message})
	}
	if definition.SchemaVersion != SchemaVersion {
		add("unsupported_schema", "schema_version", "unsupported task schema version")
	}
	for path, value := range map[string]string{"id": definition.ID, "version": definition.Version, "package_id": definition.PackageID, "package_version": definition.PackageVersion} {
		if !validID(value) {
			add("invalid_id", path, "must be a stable non-empty identifier")
		}
	}
	switch definition.Kind {
	case KindErrand, KindHunt, KindPersonal:
	default:
		add("invalid_kind", "kind", "must be errand, hunt or personal")
	}
	if !validText(definition.Title, MaxTextBytes) {
		add("invalid_text", "title", "must be non-empty and within the text limit")
	}
	if !validText(definition.Brief, MaxTextBytes) {
		add("invalid_text", "brief", "must be non-empty and within the text limit")
	}
	if definition.RequiresFactionMembership != "" && !validID(definition.RequiresFactionMembership) {
		add("invalid_id", "requires_faction_membership", "must be a stable faction identifier")
	}
	if definition.Giver != "" && !validID(definition.Giver) {
		add("invalid_id", "giver", "must be a stable identifier when present")
	}
	if definition.DeadlineSeconds < 0 || definition.DeadlineSeconds > MaxDeadlineSeconds {
		// A negative limit would expire the moment it was taken, and an unbounded
		// one could overflow the clock it is added to. Neither is a deadline a
		// player could be told about before accepting.
		add("invalid_deadline", "deadline_seconds", "must be zero or a positive bounded interval")
	}
	if e := definition.CompletionEcho; e != nil && (len(definition.Choices) != 0 || e.AfterSeconds < 1 || e.AfterSeconds > MaxDeadlineSeconds || !e.Reaction.Valid()) {
		add("invalid_echo", "completion_echo", "completion echo needs a bounded delay and one reaction on a task without choices")
	}
	if definition.Kind == KindPersonal && definition.Giver == "" {
		// A personal task with no person is an errand wearing a label; the
		// distinction is what the panel files it under.
		add("missing_giver", "giver", "a personal task must name the person it is about")
	}
	if len(definition.Objectives) == 0 {
		add("missing_objectives", "objectives", "a task must ask for something")
	}
	if len(definition.Objectives) > MaxObjectives {
		add("limit_exceeded", "objectives", "objective count exceeds the supported limit")
	}
	seen := make(map[string]bool, len(definition.Objectives))
	for index, objective := range definition.Objectives {
		path := fmt.Sprintf("objectives[%d]", index)
		if !validID(objective.ID) {
			add("invalid_id", path+".id", "must be a stable non-empty identifier")
		} else if seen[objective.ID] {
			add("duplicate_id", path+".id", "objective identifier is duplicated")
		}
		seen[objective.ID] = true
		if !validText(objective.Description, MaxTextBytes) {
			add("invalid_text", path+".description", "must be non-empty and within the text limit")
		}
		if len(objective.Evidence) == 0 {
			add("missing_evidence", path+".evidence", "an objective must name at least one fact that satisfies it")
		}
		for evidenceIndex, evidence := range objective.Evidence {
			evidencePath := fmt.Sprintf("%s.evidence[%d]", path, evidenceIndex)
			switch evidence.Kind {
			case EvidenceRealmReward, EvidenceJourneyArrival, EvidenceCombatOutcome, EvidenceChoice:
			default:
				add("invalid_evidence_kind", evidencePath+".kind", "must be realm_reward, journey_arrival or combat_outcome")
			}
			if !validID(evidence.RefID) {
				add("invalid_id", evidencePath+".ref_id", "must name the fact that satisfies the objective")
			}
		}
	}
	if len(definition.Rewards) > MaxRewards {
		add("limit_exceeded", "rewards", "reward count exceeds the supported limit")
	}
	for index, reward := range definition.Rewards {
		path := fmt.Sprintf("rewards[%d]", index)
		switch reward.Kind {
		case RewardItem:
			// An item names a catalogue entry, so it must be a stable id.
			if !validID(reward.RefID) {
				add("invalid_id", path+".ref_id", "must name the catalogue entry being paid")
			}
		case RewardCurrency, RewardExperience:
			// A currency names the character's own field, and this product's
			// fields carry their display names ("灵石"), so this one is text
			// rather than an identifier. It is still required and bounded.
			if !validText(reward.RefID, MaxTextBytes) {
				add("invalid_id", path+".ref_id", "must name the currency being paid")
			}
		default:
			add("invalid_reward_kind", path+".kind", "must be item or currency")
		}
		if reward.Quantity <= 0 || reward.Quantity > MaxQuantity {
			add("invalid_quantity", path+".quantity", "must be positive and within the supported bound")
		}
	}
	diagnostics = append(diagnostics, validateChoices(definition)...)
	return diagnostics
}

// EvidenceMatches reports whether one committed fact satisfies one evidence
// clause.
//
// It is deliberately a plain equality on the kind and the reference: a fact
// either happened with that identity or it did not. There is no fuzzy match,
// because "close enough" is how a task would tick without the thing being done.
func EvidenceMatches(evidence Evidence, fact Fact) bool {
	if evidence.Kind != fact.Kind {
		return false
	}
	return strings.TrimSpace(evidence.RefID) == strings.TrimSpace(fact.RefID)
}

// Fact is one committed thing that happened, as the caller read it off the
// journal.
type Fact struct {
	Kind  EvidenceKind
	RefID string
}

// SatisfiedBy returns the objective ids a fact satisfies, in authored order.
func (definition Definition) SatisfiedBy(fact Fact) []string {
	var ids []string
	for _, objective := range definition.Objectives {
		for _, evidence := range objective.Evidence {
			if EvidenceMatches(evidence, fact) {
				ids = append(ids, objective.ID)
				break
			}
		}
	}
	return ids
}

// IsComplete reports whether every objective is satisfied.
func (definition Definition) IsComplete(completed []string) bool {
	if len(definition.Objectives) == 0 {
		return false
	}
	done := make(map[string]bool, len(completed))
	for _, id := range completed {
		done[id] = true
	}
	for _, objective := range definition.Objectives {
		if !done[objective.ID] {
			return false
		}
	}
	return true
}

// MissingObjectives lists what is still open, in authored order, so the panel
// can show what is left rather than only that something is.
func (definition Definition) MissingObjectives(completed []string) []string {
	done := make(map[string]bool, len(completed))
	for _, id := range completed {
		done[id] = true
	}
	missing := make([]string, 0, len(definition.Objectives))
	for _, objective := range definition.Objectives {
		if !done[objective.ID] {
			missing = append(missing, objective.ID)
		}
	}
	return missing
}

// HasObjective reports whether the definition declares an objective id.
func (definition Definition) HasObjective(id string) bool {
	for _, objective := range definition.Objectives {
		if objective.ID == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

// SortedObjectiveIDs returns the completed ids in a stable order, so a stored
// list does not depend on the order the player happened to do things.
func SortedObjectiveIDs(ids []string) []string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return sorted
}

// TaskStatus is the state of one task instance.
//
// §4.4's status names are kept: an offer is not an acceptance, and a settled
// task is not reopened by walking past the person again.
type TaskStatus string

const (
	StatusOffered   TaskStatus = "offered"
	StatusAccepted  TaskStatus = "accepted"
	StatusCompleted TaskStatus = "completed"
	StatusAbandoned TaskStatus = "abandoned"
	// StatusExpired is an acceptance whose agreed instant passed before the work
	// was finished. It is distinct from abandoned because the player did not step
	// back: the world moved on.
	StatusExpired TaskStatus = "expired"
)

// Instance is one task really in play on a branch.
type Instance struct {
	TaskID      string     `json:"task_id"`
	TaskVersion string     `json:"task_version"`
	Status      TaskStatus `json:"status"`
	// CompletedObjectiveIDs are the objectives a committed fact has satisfied.
	CompletedObjectiveIDs []string `json:"completed_objective_ids"`
	// SourceRefs records which fact satisfied each objective, so a reader can
	// trace a tick back to the thing that caused it.
	SourceRefs map[string]string `json:"source_refs,omitempty"`
	// SettledRewards reports that the payment was handed over. It is a fact on
	// the instance rather than the caller's memory, so returning to the giver
	// cannot pay twice.
	SettledRewards bool `json:"settled_rewards,omitempty"`
	// DeadlineSeconds is the absolute formal instant this acceptance runs to,
	// stamped from the committed clock when the task was taken. Zero means no
	// deadline was agreed.
	//
	// It is stored on the instance rather than recomputed from AcceptedAt,
	// because AcceptedAt is display text and the clock is the only thing the
	// arithmetic may read. A fork inherits the instant its parent had; it never
	// re-stamps, so two branches cannot disagree about when one promise expires.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`
	// AcceptedAt and SettledAt are the story's display text for those moments.
	AcceptedAt     string                  `json:"accepted_at,omitempty"`
	SettledAt      string                  `json:"settled_at,omitempty"`
	Choices        map[string]ChoiceResult `json:"choices,omitempty"`
	CompletionEcho *EchoResult             `json:"completion_echo,omitempty"`
}

// ApplyFact ticks every objective the fact satisfies and reports whether the
// instance changed.
//
// Only an accepted task advances. A task the player has not accepted is still
// an offer, and §4.4 is explicit that finding the thing does not silently
// accept it on their behalf — the player may still say yes afterwards.
//
// atSeconds is the committed formal instant the fact happened at. A fact that
// lands after an agreed deadline does not tick: the work was really done, but it
// was not done in time, and §五 of 自制-并行事项与到期事件 says an expired promise
// closes rather than being quietly fulfilled.
func (instance Instance) ApplyFact(definition Definition, fact Fact, sourceRef string, atSeconds int64) (Instance, []string) {
	if instance.Status != StatusAccepted {
		return instance, nil
	}
	if instance.ExpiredAt(atSeconds) {
		return instance, nil
	}
	already := make(map[string]bool, len(instance.CompletedObjectiveIDs))
	for _, id := range instance.CompletedObjectiveIDs {
		already[id] = true
	}
	var added []string
	for _, id := range definition.SatisfiedBy(fact) {
		if already[id] {
			continue
		}
		added = append(added, id)
	}
	if len(added) == 0 {
		return instance, nil
	}
	completed := append([]string(nil), instance.CompletedObjectiveIDs...)
	completed = append(completed, added...)
	instance.CompletedObjectiveIDs = SortedObjectiveIDs(completed)
	if instance.SourceRefs == nil {
		instance.SourceRefs = map[string]string{}
	}
	for _, id := range added {
		instance.SourceRefs[id] = strings.TrimSpace(sourceRef)
	}
	if definition.IsComplete(instance.CompletedObjectiveIDs) {
		instance.Status = StatusCompleted
	}
	return instance, added
}

// Deadline reports whether the acceptance has an agreed limit, and the formal
// instant it runs to.
func (instance Instance) Deadline() (int64, bool) {
	if instance.DeadlineSeconds <= 0 {
		return 0, false
	}
	return instance.DeadlineSeconds, true
}

// ExpiredAt reports whether a deadline had already passed at one committed
// instant.
//
// An instance with no deadline is never expired. A completed task is not
// expired by a later instant either: §五 keeps expiry from erasing work that
// was already finished, and the instant it finished at is the one that counts.
// An offered task has no stamped deadline, so it is never expired.
func (instance Instance) ExpiredAt(atSeconds int64) bool {
	deadline, has := instance.Deadline()
	return has && atSeconds >= deadline && instance.Status == StatusAccepted
}

// Expire marks an accepted task whose agreed instant has passed.
//
// It changes only the instance's own status. It does not touch objectives that
// were already ticked and does not erase the record of the work: §五 says expiry
// closes the opportunity rather than rewriting what happened. Paying afterwards
// is refused by the ordinary status checks, which is the real consequence of
// letting the moment pass.
//
// An instance with no deadline, or one already completed, abandoned or offered,
// is returned unchanged. That keeps a single "time moved" pass from changing
// anything it was not entitled to.
func (instance Instance) Expire(atSeconds int64) (Instance, bool) {
	if instance.Status != StatusAccepted || !instance.ExpiredAt(atSeconds) {
		return instance, false
	}
	instance.Status = StatusExpired
	return instance, true
}

// StampDeadline returns the instance with its absolute deadline stamped from an
// acceptance that happened at one committed instant.
//
// It is separate from the caller's action planning so both the entry and the
// turn stamp the same way. A task with no authored limit stays at zero rather
// than borrowing the current time, because "no agreed limit" is a real answer the
// panel has to be able to show.
func StampDeadline(definition Definition, instance Instance, acceptedAtSeconds int64) Instance {
	if definition.DeadlineSeconds <= 0 {
		instance.DeadlineSeconds = 0
		return instance
	}
	instance.DeadlineSeconds = acceptedAtSeconds + definition.DeadlineSeconds
	return instance
}

// ValidateInstance checks a stored instance against the definition it names.
//
// A stored row is re-validated rather than trusted: a completed objective the
// definition does not declare, or a settlement on a task that is not complete,
// would let the journal pay for something that never happened.
func ValidateInstance(definition Definition, instance Instance) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(code, path, message string) {
		diagnostics = append(diagnostics, Diagnostic{Code: code, Path: path, Message: message})
	}
	if instance.TaskID != definition.ID {
		add("task_mismatch", "task_id", "instance does not belong to the named task")
	}
	if instance.TaskVersion != definition.Version {
		add("version_mismatch", "task_version", "instance was bound to a different task version")
	}
	switch instance.Status {
	case StatusOffered, StatusAccepted, StatusCompleted, StatusAbandoned, StatusExpired:
	default:
		add("invalid_status", "status", "unknown task status")
	}
	seen := make(map[string]bool, len(instance.CompletedObjectiveIDs))
	for index, id := range instance.CompletedObjectiveIDs {
		path := fmt.Sprintf("completed_objective_ids[%d]", index)
		if !definition.HasObjective(id) {
			add("unknown_objective", path, "objective is not declared by the task")
		}
		if seen[id] {
			add("duplicate_objective", path, "objective completion is duplicated")
		}
		seen[id] = true
	}
	complete := definition.IsComplete(instance.CompletedObjectiveIDs)
	if instance.Status == StatusCompleted && !complete {
		add("incomplete_completion", "status", "a completed task must satisfy every objective")
	}
	if instance.SettledRewards && !complete {
		add("settled_incomplete", "settled_rewards", "a task may only pay once it is complete")
	}
	if instance.Status == StatusAbandoned && instance.SettledRewards {
		add("settled_abandoned", "settled_rewards", "an abandoned task cannot have paid")
	}
	if instance.Status == StatusExpired && instance.SettledRewards {
		add("settled_expired", "settled_rewards", "an expired task cannot have paid")
	}
	if instance.DeadlineSeconds < 0 {
		add("invalid_deadline", "deadline_seconds", "must be a non-negative formal instant")
	}
	if e := instance.CompletionEcho; e != nil {
		if definition.CompletionEcho == nil || !instance.SettledRewards || !e.Valid(definition.CompletionEcho.AfterSeconds) {
			add("invalid_echo", "completion_echo", "completion echo must follow a settled task at its original formal instant")
		}
	}
	// A missing stamp is not refused: an acceptance recorded before this task
	// carried a deadline keeps the terms it was taken under, which is the same
	// rule that keeps old saves on their old binding. The instance's own instant,
	// not the authored one, is what the arithmetic reads.
	for objective, result := range instance.Choices {
		for current := &result; current != nil; current = current.Previous {
			if current.Capability < 0 || current.Capability > MaxChoiceCapability {
				add("invalid_preparation", "choices", "choice preparation is out of bounds")
				break
			}
			if prior := current.Previous; prior != nil && (prior.Capability >= current.Capability || prior.Outcome != "failure" || !prior.EchoResolved || prior.AtSeconds >= current.AtSeconds) {
				add("invalid_retry", "choices", "choice retry lacks improved preparation")
				break
			}
		}
		if result.EchoAcknowledgedBy != "" && (!result.EchoResolved || result.EchoAcknowledgedAtSeconds < result.EchoDueSeconds) || result.EchoAcknowledgedBy == "" && result.EchoAcknowledgedAtSeconds != 0 {
			add("invalid_echo", "choices", "echo acknowledgement must follow maturity")
		}
		choice, ok := definition.Choice(result.ChoiceID)
		if !ok || choice.ObjectiveID != objective || !seen[objective] || (result.Outcome != "success" && result.Outcome != "failure") || result.AtSeconds < 0 || result.EchoDueSeconds <= result.AtSeconds || result.EchoDueSeconds-result.AtSeconds > MaxDeadlineSeconds {
			add("invalid_choice_result", "choices", "choice outcome does not match the authored stage")
		}
	}
	return diagnostics
}
