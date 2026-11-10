package character

import (
	"fmt"
	"strings"
)

// ParticipationDecision records one speaker's answer about one fight or route.
// It grants no control, membership, movement or free action. The story validates
// actual participants and sources before the relevant rule consumes this reply.
type ParticipationDecision struct {
	Kind     string `json:"kind" jsonschema:"enum=combat,enum=journey,enum=faction"`
	TargetID string `json:"target_id"`
	Decision string `json:"decision" jsonschema:"enum=accept,enum=refuse"`
}

const (
	ParticipationFaction = "faction"
	ParticipationCombat  = "combat"
	ParticipationJourney = "journey"
	ParticipationAccept  = "accept"
	ParticipationRefuse  = "refuse"
)

func (d ParticipationDecision) Normalize() ParticipationDecision {
	d.Kind = strings.TrimSpace(d.Kind)
	d.TargetID = strings.TrimSpace(d.TargetID)
	d.Decision = strings.TrimSpace(d.Decision)
	return d
}

func (d ParticipationDecision) Validate() error {
	d = d.Normalize()
	if d.Kind != ParticipationCombat && d.Kind != ParticipationJourney && d.Kind != ParticipationFaction {
		return fmt.Errorf("a participation decision requires combat, journey or faction scope")
	}
	if d.TargetID == "" {
		return fmt.Errorf("a participation decision requires its opponent or route")
	}
	if d.Decision != ParticipationAccept && d.Decision != ParticipationRefuse {
		return fmt.Errorf("a participation decision must be accept or refuse")
	}
	return nil
}
