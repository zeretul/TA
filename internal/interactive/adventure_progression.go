package interactive

import "fmt"

// AdventureProgression is an explicit, frozen game adaptation. A missing table
// preserves the last release's ten-rank rules rather than rewriting old saves.
type AdventureProgression struct {
	MaxRank           int                         `json:"max_rank"`
	ExperiencePerHour int                         `json:"experience_per_hour"`
	CurrencyPerHour   int                         `json:"currency_per_hour"`
	Stages            []AdventureProgressionStage `json:"stages"`
}

type AdventureProgressionStage struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	EnglishName          string   `json:"english_name"`
	MinRank              int      `json:"min_rank"`
	PracticeAlternatives []string `json:"practice_alternatives,omitempty"`
	PracticeSkill        string   `json:"practice_skill,omitempty"`
	PracticeMastery      int      `json:"practice_mastery"`
}

func AdventureMaxRank(r AdventureRules) int {
	if r.Progression == nil {
		return 10
	}
	return r.Progression.MaxRank
}

func AdventureSkillMasteryCap(r AdventureRules) int {
	if r.Family == "wuxia" {
		return 4
	}
	return 10
}

// AdventureRankExperience is the cost of the next rank, not lifetime XP. The
// first ten ranks keep their existing economy; later ranks grow quadratically.
func AdventureRankExperience(r AdventureRules, rank int) int {
	cost := rank * 100
	if r.Progression != nil && rank > 10 {
		cost += (rank - 10) * (rank - 10) * 10
	}
	return cost
}

func AdventureProgressionStageFor(r AdventureRules, rank int) *AdventureProgressionStage {
	if r.Progression == nil {
		return nil
	}
	var stage *AdventureProgressionStage
	for i := range r.Progression.Stages {
		if r.Progression.Stages[i].MinRank <= rank {
			stage = &r.Progression.Stages[i]
		}
	}
	return stage
}

func validateAdventureProgression(r AdventureRules) error {
	p := r.Progression
	if p == nil {
		return nil
	}
	if p.MaxRank < 11 || p.MaxRank > 100 || p.ExperiencePerHour < 1 || p.ExperiencePerHour > 100 || p.CurrencyPerHour < 1 || p.CurrencyPerHour > 100 || len(p.Stages) < 2 || len(p.Stages) > 16 {
		return fmt.Errorf("invalid adventure progression table")
	}
	ids := map[string]bool{}
	previous := 0
	for index, stage := range p.Stages {
		if stage.ID == "" || ids[stage.ID] || stage.Name == "" || stage.EnglishName == "" || stage.MinRank <= previous || stage.MinRank > p.MaxRank || index == 0 && stage.MinRank != 1 || stage.PracticeMastery < 1 || stage.PracticeMastery > AdventureSkillMasteryCap(r) {
			return fmt.Errorf("invalid adventure progression stage %s", stage.ID)
		}
		ids[stage.ID], previous = true, stage.MinRank
		if index > 0 && stage.PracticeSkill == "" {
			return fmt.Errorf("advanced progression requires a learned foundation")
		}
		if len(stage.PracticeAlternatives) > 4 {
			return fmt.Errorf("too many progression foundations")
		}
		for _, foundation := range append([]string{stage.PracticeSkill}, stage.PracticeAlternatives...) {
			if foundation == "" {
				continue
			}
			found := false
			for _, skill := range r.Skills {
				if skill.ID == foundation && skill.RequiredRank <= stage.MinRank {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown or unreachable progression foundation %s", foundation)
			}
		}
	}
	return nil
}

func advanceAdventureRank(r AdventureRules, a *AdventureActor) {
	for a.Rank < AdventureMaxRank(r) && a.Experience >= AdventureRankExperience(r, a.Rank) {
		a.Experience -= AdventureRankExperience(r, a.Rank)
		a.Rank++
		a.MaxHealth += 20
		a.MaxEnergy += 10
		a.Health = min(a.MaxHealth, a.Health+20)
		a.Energy = min(a.MaxEnergy, a.Energy+10)
	}
	if r.Progression != nil && a.Rank == AdventureMaxRank(r) {
		a.Experience = 0
	}
}

// Refinement advances the person, not skill mastery. The current stage's real
// learned foundation, paid supplies and authorized world time are all consumed.
// A stage boundary ends this interval so a new foundation cannot be skipped.
func refineAdventureFoundation(r AdventureRules, a *AdventureActor, in AdventureAction) (AdventureOutcome, error) {
	stage := AdventureProgressionStageFor(r, a.Rank)
	if stage == nil || a.Rank >= AdventureMaxRank(r) {
		return AdventureOutcome{}, fmt.Errorf("foundation progression is unavailable or complete")
	}
	if in.Hours < 1 || a.Injury >= 50 || a.Poison > 0 || a.SoftLimbs || a.MentalShock > 0 {
		return AdventureOutcome{}, fmt.Errorf("foundation refinement requires a positive interval and stable condition")
	}
	skillID := in.SkillID
	if stage.PracticeSkill != "" {
		if skillID == "" {
			skillID = stage.PracticeSkill
			if a.Skills[skillID] < stage.PracticeMastery {
				for _, alternative := range stage.PracticeAlternatives {
					if a.Skills[alternative] >= stage.PracticeMastery {
						skillID = alternative
						break
					}
				}
			}
		}
		allowed := skillID == stage.PracticeSkill
		for _, alternative := range stage.PracticeAlternatives {
			allowed = allowed || skillID == alternative
		}
		if !allowed {
			return AdventureOutcome{}, fmt.Errorf("the current stage requires its own foundation")
		}
	}
	if a.Skills[skillID] < stage.PracticeMastery {
		return AdventureOutcome{}, fmt.Errorf("foundation skill mastery is insufficient")
	}
	limit := AdventureMaxRank(r)
	for _, next := range r.Progression.Stages {
		if next.MinRank > a.Rank {
			limit = next.MinRank
			break
		}
	}
	remaining := -a.Experience
	for rank := a.Rank; rank < limit; rank++ {
		remaining += AdventureRankExperience(r, rank)
	}
	if remaining <= 0 {
		return AdventureOutcome{}, fmt.Errorf("foundation stage must advance before further refinement")
	}
	p := r.Progression
	hours := min(in.Hours, (remaining+p.ExperiencePerHour-1)/p.ExperiencePerHour, a.Currency/p.CurrencyPerHour)
	if hours < 1 {
		return AdventureOutcome{}, fmt.Errorf("foundation refinement supplies are unaffordable")
	}
	gain := min(remaining, hours*p.ExperiencePerHour)
	a.Currency -= hours * p.CurrencyPerHour
	a.Experience += gain
	return AdventureOutcome{DurationSeconds: int64(hours) * 3600, Message: fmt.Sprintf("深修%d小时，历练增加%d，耗费%d%s；抵达新阶段后须先掌握相应基础。", hours, gain, hours*p.CurrencyPerHour, r.Currency)}, nil
}
