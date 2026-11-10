package interactive

import (
	"fmt"
	"strings"
)

// AdventureWorkService is an ordinary local livelihood, like a market's dock
// service, not a contract or a quest. Its wage and effort are frozen with the
// Story; the narrator chooses no amounts and this service owns no state store.
// Hours is the rate interval. Shorter work pays proportional whole currency,
// rounded down, while effort rounds up so splitting a shift cannot mint money.
type AdventureWorkService struct {
	Name          string `json:"name"`
	Hours         int    `json:"hours"`
	Pay           int    `json:"pay"`
	EnergyCost    int    `json:"energy_cost"`
	RequiredSkill string `json:"required_skill,omitempty"`
	SkillRank     int    `json:"skill_rank,omitempty"`
}

func (w AdventureWorkService) validate(skills map[string]bool) error {
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 256 || w.Hours < 1 || w.Hours > 24 || w.Pay < w.Hours || w.Pay > 1000000 || w.EnergyCost < 1 || w.EnergyCost > 100 {
		return fmt.Errorf("invalid work service terms")
	}
	if w.RequiredSkill == "" && w.SkillRank != 0 || w.RequiredSkill != "" && (!skills[w.RequiredSkill] || w.SkillRank < 1 || w.SkillRank > 10) {
		return fmt.Errorf("invalid work service qualification")
	}
	return nil
}

// The caller verifies the committed location, active battle and formal-time
// hazards. All checks precede writes so rejected work pays and consumes nothing.
func resolveAdventureWork(r AdventureRules, a *AdventureActor, in AdventureAction) (AdventureOutcome, error) {
	var work *AdventureWorkService
	for _, market := range r.Markets {
		if market.ID == in.MarketID {
			work = market.WorkService
			break
		}
	}
	if work == nil {
		return AdventureOutcome{}, fmt.Errorf("this market has no registered work")
	}
	if in.Hours < 1 {
		return AdventureOutcome{}, fmt.Errorf("work requires an authorized positive interval")
	}
	if work.RequiredSkill != "" && a.Skills[work.RequiredSkill] < work.SkillRank {
		return AdventureOutcome{}, fmt.Errorf("work skill qualification is missing")
	}
	if a.Injury >= 50 {
		return AdventureOutcome{}, fmt.Errorf("severe injury prevents work")
	}
	// Limits are checked in int64 before converting or mutating the actor.
	hours := int64(in.Hours)
	pay := int64(work.Pay) * hours / int64(work.Hours)
	effort := (int64(work.EnergyCost)*hours + int64(work.Hours) - 1) / int64(work.Hours)
	if effort > int64(a.Energy) {
		return AdventureOutcome{}, fmt.Errorf("insufficient energy for the work interval")
	}
	if pay > 1000000000-int64(a.Currency) {
		return AdventureOutcome{}, fmt.Errorf("work payment exceeds the currency limit")
	}
	a.Currency += int(pay)
	a.Energy -= int(effort)
	return AdventureOutcome{DurationSeconds: hours * 3600, Message: fmt.Sprintf("%s已完成%d小时，取得%d%s。", work.Name, in.Hours, pay, r.Currency)}, nil
}
