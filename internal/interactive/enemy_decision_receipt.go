package interactive

import "sort"

// EnemyDecisionReceipt records only decision provenance, never model prompts,
// reasoning, provider errors or credentials. It is part of the same atomic
// journal event as settlement. Nil means historical provenance is unknown (or
// no decision was made), not that a model successfully chose the action.
// AIActors identifies actors whose selections passed rule revalidation;
// the exchange outcome remains authoritative for what actually executed.
type EnemyDecisionReceipt struct {
	V        int      `json:"v"`
	Status   string   `json:"status"`
	AIActors []string `json:"ai_actors,omitempty"`
}

func enemyDecisionReceipt(status string, accepted map[string]string, enemies int) *EnemyDecisionReceipt {
	if enemies == 0 {
		return nil
	}
	// Allowlist the server's coarse reason codes. Never persist arbitrary input.
	switch status {
	case "ai", "ai_partial_fallback", "fallback_unavailable", "fallback_context_limit", "fallback_invalid":
	case "":
		status = "rules"
	default:
		status = "unknown"
	}
	if len(accepted) > 0 {
		if len(accepted) == enemies {
			status = "ai"
		} else {
			status = "ai_partial_fallback"
		}
	} else if status == "ai" || status == "ai_partial_fallback" {
		status = "fallback_invalid"
	}
	var actors []string
	for actor := range accepted {
		actors = append(actors, actor)
	}
	sort.Strings(actors)
	return &EnemyDecisionReceipt{V: 1, Status: status, AIActors: actors}
}
