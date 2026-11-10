package quest

import "math"

// EchoReaction is one person's authored response to a verified past outcome.
// It is neither organization-wide knowledge nor an item, skill or permission
// grant. The application commits it only during a real encounter after maturity.
type EchoReaction struct {
	CharacterSourceKey string `json:"character_source_key"`
	Statement          string `json:"statement"`
}

func (r EchoReaction) Valid() bool {
	return validID(r.CharacterSourceKey) && validText(r.Statement, MaxTextBytes)
}

// CompletionEcho binds an ordinary task's handover to a later personal response.
// It starts only when the real completed work is settled with its recipient.
type CompletionEcho struct {
	AfterSeconds int64        `json:"after_seconds"`
	Reaction     EchoReaction `json:"reaction"`
}

// EchoResult lives in the existing task instance and canonical story journal.
// Maturity merely permits a later encounter; acknowledgement is that encounter.
type EchoResult struct {
	AtSeconds             int64  `json:"at_seconds"`
	DueSeconds            int64  `json:"due_seconds"`
	Resolved              bool   `json:"resolved,omitempty"`
	AcknowledgedBy        string `json:"acknowledged_by,omitempty"`
	AcknowledgedAtSeconds int64  `json:"acknowledged_at_seconds,omitempty"`
}

func (e EchoResult) Valid(delay int64) bool {
	return e.AtSeconds >= 0 && delay > 0 && e.AtSeconds <= math.MaxInt64-delay && e.DueSeconds == e.AtSeconds+delay &&
		(e.AcknowledgedBy == "" && e.AcknowledgedAtSeconds == 0 || e.AcknowledgedBy != "" && e.Resolved && e.AcknowledgedAtSeconds >= e.DueSeconds)
}

func (c Choice) Reaction(outcome string) *EchoReaction {
	if outcome == "success" {
		return c.SuccessReaction
	}
	if outcome == "failure" {
		return c.FailureReaction
	}
	return nil
}

// VisibleChoices retains ways to act but discloses only their committed outcome.
// A different method for the same objective is not a known alternative history.
// Returned values are display copies; authored definitions remain unchanged.
func VisibleChoices(d Definition, results map[string]ChoiceResult) []Choice {
	choices := append([]Choice(nil), d.Choices...)
	for n := range choices {
		c := &choices[n]
		r, ok := results[c.ObjectiveID]
		known := ok && r.ChoiceID == c.ID
		knownEcho := known && r.EchoResolved && (!r.EchoEncounterRequired || r.EchoAcknowledgedBy != "")
		if !known || r.Outcome != "success" {
			c.Success = ""
		}
		if !known || r.Outcome != "failure" {
			c.Failure = ""
		}
		if !knownEcho || r.Outcome != "success" {
			c.SuccessEcho = ""
		}
		if !knownEcho || r.Outcome != "failure" {
			c.FailureEcho = ""
		}
		c.SuccessReaction, c.FailureReaction = nil, nil
	}
	return choices
}
