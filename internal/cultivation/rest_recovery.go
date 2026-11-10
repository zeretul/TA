package cultivation

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// RecoveryMode names one safe, off-battle activity. Each call advances exactly
// one pool; rest never also meditates, and meditation never heals health.
type RecoveryMode string

const (
	RecoveryHealthRest     RecoveryMode = "health_rest"
	RecoverySafeMeditation RecoveryMode = "safe_meditation"
)

// RecoveryEligibility is a domain fact confirmed by the caller for both the
// activity origin and its current snapshot. Zero health is not a death verdict:
// the caller may mark a legally surviving, incapacitated character as qualified.
type RecoveryEligibility string

const (
	RecoveryQualified             RecoveryEligibility = "qualified_safe"
	RecoveryInBattle              RecoveryEligibility = "in_battle"
	RecoveryInDanger              RecoveryEligibility = "in_danger"
	RecoveryDead                  RecoveryEligibility = "dead"
	RecoveryNeedsSpecialTreatment RecoveryEligibility = "needs_special_treatment"
)

var (
	ErrRecoveryMissingActivityID  = errors.New("cultivation: recovery activity requires a stable id")
	ErrRecoveryUnknownMode        = errors.New("cultivation: recovery mode is unsupported")
	ErrRecoveryUnknownEligibility = errors.New("cultivation: recovery eligibility is unsupported")
	ErrRecoveryEligibilityChanged = errors.New("cultivation: recovery eligibility changed; start a new activity segment")
	ErrRecoveryIneligible         = errors.New("cultivation: character is not eligible for ordinary safe recovery")
	ErrRecoveryMissingMaximum     = errors.New("cultivation: recovery requires a positive pool maximum")
	ErrRecoveryMaximumChanged     = errors.New("cultivation: pool maximum changed; start a new activity segment")
	ErrRecoveryNegativeValue      = errors.New("cultivation: recovery snapshot contains a negative pool or duration")
	ErrRecoveryPoolOutOfRange     = errors.New("cultivation: recovery pool is outside its confirmed maximum")
	ErrRecoveryTimeReversal       = errors.New("cultivation: recovery target precedes settled effective duration")
	ErrRecoverySnapshotMismatch   = errors.New("cultivation: recovery snapshot does not match cumulative settled progress")
)

// RecoveryRequest describes one activity snapshot. StartAttributes and
// CurrentAttributes must come from caller-confirmed Attributes projections.
// SettledSeconds and TargetSeconds are cumulative effective safe time from this
// activity's origin, not wall-clock timestamps. If safety or the relevant pool
// maximum changes, the caller must start a new activity segment.
type RecoveryRequest struct {
	Rules      *Rules `json:"-"`
	ActivityID string
	Mode       RecoveryMode

	EligibilityAtStart RecoveryEligibility
	EligibilityNow     RecoveryEligibility

	StartAttributes   Attributes
	CurrentAttributes Attributes

	SettledSeconds int64
	TargetSeconds  int64
}

// RecoveryCandidate contains only the changed resource pool and the cumulative
// interval that produced it. It is a proposal; the caller owns validation and
// committing the pool with the formal character state.
type RecoveryCandidate struct {
	ActivityID string       `json:"activity_id"`
	Mode       RecoveryMode `json:"mode"`

	FromElapsedSeconds int64 `json:"from_elapsed_seconds"`
	ToElapsedSeconds   int64 `json:"to_elapsed_seconds"`

	PoolMaximum         int `json:"pool_maximum"`
	PreviousPool        int `json:"previous_pool"`
	CandidatePool       int `json:"candidate_pool"`
	Increment           int `json:"increment"`
	CumulativeRecovered int `json:"cumulative_recovered"`
}

// PlanSafeRecovery calculates an uncommitted cumulative recovery projection.
// It uses floor(maximum * elapsed / rateDenominator) from the activity origin,
// then subtracts the already-settled cumulative result. This makes repeated
// requests idempotent and prevents small intervals from earning rounded-up
// recovery. Exact integer products use big.Int; the final pool is capped at its
// validated int maximum without overflowing intermediate arithmetic.
func PlanSafeRecovery(request RecoveryRequest) (RecoveryCandidate, error) {
	if strings.TrimSpace(request.ActivityID) == "" {
		return RecoveryCandidate{}, ErrRecoveryMissingActivityID
	}
	denominator, err := recoveryRateDenominator(request.Mode)
	if request.Rules != nil {
		var found bool
		denominator, found = request.Rules.RecoverySeconds[request.Mode]
		if !found || denominator <= 0 {
			return RecoveryCandidate{}, ErrRecoveryUnknownMode
		}
	}
	if err != nil {
		return RecoveryCandidate{}, err
	}
	if !validRecoveryEligibility(request.EligibilityAtStart) || !validRecoveryEligibility(request.EligibilityNow) {
		return RecoveryCandidate{}, ErrRecoveryUnknownEligibility
	}
	if request.EligibilityAtStart != request.EligibilityNow {
		return RecoveryCandidate{}, ErrRecoveryEligibilityChanged
	}
	if request.EligibilityNow != RecoveryQualified {
		return RecoveryCandidate{}, ErrRecoveryIneligible
	}
	if request.SettledSeconds < 0 || request.TargetSeconds < 0 {
		return RecoveryCandidate{}, ErrRecoveryNegativeValue
	}
	if request.TargetSeconds < request.SettledSeconds {
		return RecoveryCandidate{}, ErrRecoveryTimeReversal
	}

	startPool, startMaximum := recoveryPool(request.StartAttributes, request.Mode)
	currentPool, currentMaximum := recoveryPool(request.CurrentAttributes, request.Mode)
	if startPool < 0 || currentPool < 0 || startMaximum < 0 || currentMaximum < 0 {
		return RecoveryCandidate{}, ErrRecoveryNegativeValue
	}
	if startMaximum == 0 || currentMaximum == 0 {
		return RecoveryCandidate{}, ErrRecoveryMissingMaximum
	}
	if startMaximum != currentMaximum {
		return RecoveryCandidate{}, ErrRecoveryMaximumChanged
	}
	if startPool > startMaximum || currentPool > currentMaximum {
		return RecoveryCandidate{}, ErrRecoveryPoolOutOfRange
	}

	settledPool := projectedRecoveryPool(startPool, startMaximum, request.SettledSeconds, denominator)
	if currentPool != settledPool {
		return RecoveryCandidate{}, fmt.Errorf("%w: current pool is %d, want %d at %d settled seconds", ErrRecoverySnapshotMismatch, currentPool, settledPool, request.SettledSeconds)
	}
	candidatePool := projectedRecoveryPool(startPool, startMaximum, request.TargetSeconds, denominator)
	if candidatePool < currentPool {
		return RecoveryCandidate{}, ErrRecoverySnapshotMismatch
	}
	return RecoveryCandidate{
		ActivityID: request.ActivityID, Mode: request.Mode,
		FromElapsedSeconds: request.SettledSeconds, ToElapsedSeconds: request.TargetSeconds,
		PoolMaximum: startMaximum, PreviousPool: currentPool, CandidatePool: candidatePool,
		Increment: candidatePool - currentPool, CumulativeRecovered: candidatePool - startPool,
	}, nil
}

func recoveryRateDenominator(mode RecoveryMode) (int64, error) {
	switch mode {
	case RecoveryHealthRest:
		// 5% of maximum per 24-hour calculation day.
		return 20 * 24 * 60 * 60, nil
	case RecoverySafeMeditation:
		// 10% of maximum per calculation hour.
		return 10 * 60 * 60, nil
	default:
		return 0, ErrRecoveryUnknownMode
	}
}

func validRecoveryEligibility(eligibility RecoveryEligibility) bool {
	switch eligibility {
	case RecoveryQualified, RecoveryInBattle, RecoveryInDanger, RecoveryDead, RecoveryNeedsSpecialTreatment:
		return true
	default:
		return false
	}
}

func recoveryPool(attributes Attributes, mode RecoveryMode) (current, maximum int) {
	switch mode {
	case RecoveryHealthRest:
		return attributes.Health, attributes.HealthMax
	case RecoverySafeMeditation:
		return attributes.Mana, attributes.ManaMax
	default:
		return 0, 0
	}
}

func projectedRecoveryPool(startPool, maximum int, elapsedSeconds, denominator int64) int {
	available := int64(maximum - startPool)
	if available == 0 || elapsedSeconds == 0 {
		return startPool
	}
	var recovered big.Int
	recovered.Mul(big.NewInt(int64(maximum)), big.NewInt(elapsedSeconds))
	recovered.Quo(&recovered, big.NewInt(denominator))
	if recovered.Cmp(big.NewInt(available)) >= 0 {
		return maximum
	}
	return startPool + int(recovered.Int64())
}
