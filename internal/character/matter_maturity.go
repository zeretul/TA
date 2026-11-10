package character

import (
	"fmt"
	"math"
	"strings"
)

// bindMatterMaturity binds a spoken agreement to the existing branch clock.
// It schedules no work. A later real confirmation is still needed, and earlier
// cancellation remains possible. The original matter and its history are kept.
func bindMatterMaturity(ctx ProjectionContext, exchanges []Exchange, draft MatterDraft, matter *Matter) error {
	if draft.AfterSeconds < 0 || draft.AfterSeconds > 100*365*24*60*60 {
		return fmt.Errorf("matter revisit interval is out of bounds")
	}
	needsConfirmation := draft.AfterSeconds > 0
	if draft.AfterSeconds > 0 {
		if draft.Kind != MatterPromise && draft.Kind != MatterAction || draft.Status != "" && draft.Status != MatterOpen || strings.TrimSpace(draft.Resolves) != "" || draft.Letter != nil || draft.Cooperation != nil {
			return fmt.Errorf("a revisit time requires a new ordinary open promise or action")
		}
		if ctx.ClockSeconds < 0 || ctx.ClockSeconds > math.MaxInt64-draft.AfterSeconds {
			return fmt.Errorf("matter revisit instant overflows")
		}
		matter.EarliestSeconds = ctx.ClockSeconds + draft.AfterSeconds
	}
	for _, prior := range ctx.ExistingMatters {
		if prior.ID != strings.TrimSpace(draft.Resolves) || prior.EarliestSeconds == 0 {
			continue
		}
		if prior.CharacterID != matter.CharacterID {
			return fmt.Errorf("a delayed matter must be resolved by its original person")
		}
		if draft.Status == MatterSettled {
			if ctx.ClockSeconds < prior.EarliestSeconds {
				return fmt.Errorf("the delayed matter has not reached its earliest revisit instant")
			}
			needsConfirmation = true
		}
	}
	if needsConfirmation {
		exchange, err := exchangeAt(exchanges, draft.SourceExchangeIndex, matter.CharacterID)
		if err != nil {
			return fmt.Errorf("delayed matter needs the person's actual words: %w", err)
		}
		if exchange.Speaker != SpeakerCharacter {
			return fmt.Errorf("a player's expectation is not the other person's agreement")
		}
		matter.ExchangeID = exchange.ID
	}
	return nil
}
