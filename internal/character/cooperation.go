package character

import (
	"fmt"
	"reflect"
	"strings"
)

// CooperationPhase describes the recorded work, not the matter's open or
// resolved status. A player may leave a completed plan unused or unreported.
type CooperationPhase string

const (
	CooperationAgreed   CooperationPhase = "agreed"
	CooperationWorked   CooperationPhase = "worked"
	CooperationUsed     CooperationPhase = "used"
	CooperationReported CooperationPhase = "reported"
)

// CooperationDecision is one character's scoped response, not an agreement or
// permission to spend resources. The story must verify the actual speaker,
// presence, player intent and shared information before accepting it as consent.
type CooperationDecision struct {
	ProjectID       string `json:"project_id"`
	RealmInstanceID string `json:"realm_instance_id"`
	Decision        string `json:"decision" jsonschema:"enum=accept,enum=refuse"`
}

const (
	CooperationAccept = "accept"
	CooperationRefuse = "refuse"
)

func (d CooperationDecision) Normalize() CooperationDecision {
	d.ProjectID = strings.TrimSpace(d.ProjectID)
	d.RealmInstanceID = strings.TrimSpace(d.RealmInstanceID)
	d.Decision = strings.TrimSpace(d.Decision)
	return d
}

func (d CooperationDecision) Validate() error {
	d = d.Normalize()
	if d.ProjectID == "" || d.RealmInstanceID == "" {
		return fmt.Errorf("a cooperation decision requires a project and realm instance")
	}
	if d.Decision != CooperationAccept && d.Decision != CooperationRefuse {
		return fmt.Errorf("a cooperation decision must be accept or refuse")
	}
	return nil
}

// CooperationTerms binds one specific agreement and its consequences to one
// realm instance. The partner is Matter.CharacterID; there is no second owner.
// Only the trusted application path may create these terms after checking the
// actual consent, shared discovery, participants and costs against the story.
type CooperationTerms struct {
	ProjectID                string           `json:"project_id"`
	ProjectVersion           string           `json:"project_version"`
	RealmInstanceID          string           `json:"realm_instance_id"`
	ConsentExchangeID        string           `json:"consent_exchange_id"`
	SharedDiscoverySourceRef string           `json:"shared_discovery_source_ref"`
	Phase                    CooperationPhase `json:"phase"`
	WorkSourceRef            string           `json:"work_source_ref,omitempty"`
	UsedBy                   string           `json:"used_by,omitempty"`
	ReportSourceRefs         []string         `json:"report_source_refs,omitempty"`
}

// Normalize returns an independent value. It preserves invalid entries and
// duplicates for Validate to reject rather than silently repairing provenance.
func (c CooperationTerms) Normalize() CooperationTerms {
	c.ProjectID = strings.TrimSpace(c.ProjectID)
	c.ProjectVersion = strings.TrimSpace(c.ProjectVersion)
	c.RealmInstanceID = strings.TrimSpace(c.RealmInstanceID)
	c.ConsentExchangeID = strings.TrimSpace(c.ConsentExchangeID)
	c.SharedDiscoverySourceRef = strings.TrimSpace(c.SharedDiscoverySourceRef)
	c.Phase = CooperationPhase(strings.TrimSpace(string(c.Phase)))
	c.WorkSourceRef = strings.TrimSpace(c.WorkSourceRef)
	c.UsedBy = strings.TrimSpace(c.UsedBy)
	if len(c.ReportSourceRefs) == 0 {
		c.ReportSourceRefs = nil
	} else {
		refs := make([]string, len(c.ReportSourceRefs))
		for i, ref := range c.ReportSourceRefs {
			refs[i] = strings.TrimSpace(ref)
		}
		c.ReportSourceRefs = refs
	}
	return c
}

// Validate checks the shape of a recorded phase. Nonempty references are not
// proof that events happened: only the story layer can verify their existence,
// branch, partner, consent scope and atomic resource/time settlement.
func (c CooperationTerms) Validate() error {
	c = c.Normalize()
	if c.ProjectID == "" || c.ProjectVersion == "" || c.RealmInstanceID == "" {
		return fmt.Errorf("cooperation requires a project, version and realm instance")
	}
	if c.ConsentExchangeID == "" || c.SharedDiscoverySourceRef == "" {
		return fmt.Errorf("cooperation requires consent and shared discovery sources")
	}
	switch c.Phase {
	case CooperationAgreed:
		if c.WorkSourceRef != "" || c.UsedBy != "" || len(c.ReportSourceRefs) != 0 {
			return fmt.Errorf("agreed cooperation cannot contain work, use or report sources")
		}
	case CooperationWorked:
		if c.WorkSourceRef == "" || c.UsedBy != "" || len(c.ReportSourceRefs) != 0 {
			return fmt.Errorf("worked cooperation requires a work source and no use or report sources")
		}
	case CooperationUsed:
		if c.WorkSourceRef == "" || c.UsedBy == "" || len(c.ReportSourceRefs) != 0 {
			return fmt.Errorf("used cooperation requires work and use sources and no report sources")
		}
	case CooperationReported:
		// Choosing another route after working is allowed. A report needs real
		// outcomes, but must not falsely imply that this plan was used.
		if c.WorkSourceRef == "" || len(c.ReportSourceRefs) == 0 {
			return fmt.Errorf("reported cooperation requires work and report sources")
		}
	default:
		return fmt.Errorf("cooperation has an invalid phase: %q", c.Phase)
	}
	if len(c.ReportSourceRefs) > 4 {
		return fmt.Errorf("cooperation allows at most four report sources")
	}
	seen := make(map[string]bool, len(c.ReportSourceRefs))
	for _, ref := range c.ReportSourceRefs {
		if ref == "" || seen[ref] {
			return fmt.Errorf("cooperation report sources must be nonempty and distinct")
		}
		seen[ref] = true
	}
	return nil
}

// ApplyCooperationMatter reduces a dedicated, application-verified cooperation
// event. Unlike ordinary matter projection, it may advance one canonical record
// in place. It never upgrades an ordinary promise or verifies the referenced
// events: the caller must first validate consent, costs and actual outcomes.
// Errors leave both inputs untouched, and successful results own their memory.
func ApplyCooperationMatter(existing []Matter, next Matter) ([]Matter, error) {
	next = next.Normalize()
	if next.Cooperation == nil {
		return nil, fmt.Errorf("a cooperation event requires cooperation terms")
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	out := make([]Matter, len(existing))
	target := -1
	for i, matter := range existing {
		matter = matter.Normalize()
		out[i] = matter
		if matter.ID == next.ID {
			if target != -1 {
				return nil, fmt.Errorf("cooperation matter %s is duplicated", next.ID)
			}
			if matter.Cooperation == nil {
				return nil, fmt.Errorf("ordinary matter %s cannot become cooperation", next.ID)
			}
			target = i
		} else if sameCooperationScope(matter, next) {
			return nil, fmt.Errorf("cooperation for this project, instance and partner already has matter %s", matter.ID)
		}
	}
	if target == -1 {
		if next.Cooperation.Phase != CooperationAgreed && next.Cooperation.Phase != CooperationWorked {
			return nil, fmt.Errorf("new cooperation must start agreed or worked")
		}
		return append(out, next), nil
	}
	previous := out[target]
	if err := previous.Validate(); err != nil {
		return nil, err
	}
	if reflect.DeepEqual(previous, next) {
		return out, nil
	}
	before, after := previous.Cooperation, next.Cooperation
	if previous.StoryID != next.StoryID || previous.BranchID != next.BranchID ||
		previous.CharacterID != next.CharacterID || before.ProjectID != after.ProjectID ||
		before.ProjectVersion != after.ProjectVersion || before.RealmInstanceID != after.RealmInstanceID ||
		before.SharedDiscoverySourceRef != after.SharedDiscoverySourceRef {
		return nil, fmt.Errorf("cooperation matter %s cannot change its identity or agreement sources", next.ID)
	}
	// A new concrete response may replace consent only before any work. The
	// trusted caller validates the actual exchange and whether it is latest.
	consentChanged := before.ConsentExchangeID != after.ConsentExchangeID
	if consentChanged && !(before.Phase == CooperationAgreed && (after.Phase == CooperationAgreed || after.Phase == CooperationWorked)) {
		return nil, fmt.Errorf("cooperation matter %s cannot replace consent after work", next.ID)
	}
	if before.WorkSourceRef != "" && before.WorkSourceRef != after.WorkSourceRef ||
		before.UsedBy != "" && before.UsedBy != after.UsedBy {
		return nil, fmt.Errorf("cooperation matter %s cannot change its work or use source", next.ID)
	}
	allowed := before.Phase == CooperationAgreed && (after.Phase == CooperationWorked || after.Phase == CooperationAgreed && consentChanged) ||
		before.Phase == CooperationWorked && (after.Phase == CooperationUsed || after.Phase == CooperationReported) ||
		before.Phase == CooperationUsed && after.Phase == CooperationReported ||
		before.Phase == CooperationReported && after.Phase == CooperationReported
	if !allowed {
		return nil, fmt.Errorf("cooperation cannot advance from %s to %s", before.Phase, after.Phase)
	}
	if after.Phase == CooperationReported && before.UsedBy != after.UsedBy {
		return nil, fmt.Errorf("a cooperation report cannot invent plan use")
	}
	if before.Phase == CooperationReported {
		// A report may later add another actual consequence, but cannot turn an
		// unused plan into a used one, erase earlier reports or repeat the same
		// report under a fresh command/event identity.
		if len(after.ReportSourceRefs) <= len(before.ReportSourceRefs) {
			return nil, fmt.Errorf("a further cooperation report must add a new source")
		}
		for _, ref := range before.ReportSourceRefs {
			found := false
			for _, candidate := range after.ReportSourceRefs {
				if ref == candidate {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("cooperation report cannot discard source %s", ref)
			}
		}
	}
	out[target] = next
	return out, nil
}

func sameCooperationScope(a, b Matter) bool {
	return a.Cooperation != nil && b.Cooperation != nil &&
		a.StoryID == b.StoryID && a.BranchID == b.BranchID &&
		a.CharacterID == b.CharacterID && a.Cooperation.ProjectID == b.Cooperation.ProjectID &&
		a.Cooperation.RealmInstanceID == b.Cooperation.RealmInstanceID
}
