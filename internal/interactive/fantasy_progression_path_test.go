package interactive

import (
	"fmt"
	"reflect"
	"testing"
)

// A deterministic rule path from the actual starter, with no grants, repeated
// rewards, generated skills or skipped costs. All commerce is in Ravenbridge.
// This proves reachability and records pacing; it is not natural AI play or a
// judgment that thousands of in-world hours form an engaging campaign.
func TestFantasyStarterEarnsEveryFoundationStage(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	initial := a
	elapsed, earned, spent := int64(0), 0, 0
	actions := 0
	apply := func(action AdventureAction) AdventureOutcome {
		t.Helper()
		beforeCurrency := a.Currency
		next, out, err := ResolveAdventureActionAt(r, a, action, fmt.Sprintf("earned-stage-%d", actions), elapsed)
		if err != nil {
			t.Fatalf("rank=%d currency=%d energy=%d action=%+v: %v", a.Rank, a.Currency, a.Energy, action, err)
		}
		a = next
		actions++
		elapsed += out.DurationSeconds
		if action.Action == "work" {
			earned += a.Currency - beforeCurrency
		} else if a.Currency < beforeCurrency {
			spent += beforeCurrency - a.Currency
		}
		return out
	}
	fund := func(amount int) {
		t.Helper()
		for a.Currency < amount {
			if a.Energy < 6 {
				apply(AdventureAction{Action: "rest", Hours: (a.MaxEnergy - a.Energy + 19) / 20})
			}
			hours := min((amount-a.Currency+10)/11, a.Energy/6)
			apply(AdventureAction{Action: "work", MarketID: "sg-market-01", Hours: hours})
		}
	}
	for _, stage := range r.Progression.Stages {
		if a.Rank != stage.MinRank {
			t.Fatalf("did not arrive at the authored stage boundary: got %d want %d", a.Rank, stage.MinRank)
		}
		for a.Skills["sg-blade"] < stage.PracticeMastery {
			hours := 2*(a.Skills["sg-blade"]+1) - a.Training["sg-blade"]
			apply(AdventureAction{Action: "train", SkillID: "sg-blade", Hours: hours})
		}
		limit := AdventureMaxRank(r)
		for _, next := range r.Progression.Stages {
			if next.MinRank > a.Rank {
				limit = next.MinRank
				break
			}
		}
		xp := -a.Experience
		for rank := a.Rank; rank < limit; rank++ {
			xp += AdventureRankExperience(r, rank)
		}
		hours := (xp + r.Progression.ExperiencePerHour - 1) / r.Progression.ExperiencePerHour
		fund(hours * r.Progression.CurrencyPerHour)
		apply(AdventureAction{Action: "refine", SkillID: "sg-blade", Hours: hours})
		if a.Rank != limit {
			t.Fatalf("funded refinement did not reach the next stage: got %d want %d", a.Rank, limit)
		}
		t.Logf("stage=%s rank=%d elapsed_hours=%d earned=%d spent=%d", stage.ID, a.Rank, elapsed/3600, earned, spent)
	}
	var legacy AdventureSkill
	for _, skill := range r.Skills {
		if skill.ID == "sg-legacy" {
			legacy = skill
		}
	}
	if legacy.ID == "" {
		t.Fatal("missing advanced social route")
	}
	price := 0
	for _, item := range r.Items {
		if item.ID == legacy.RequiredItem {
			price = item.Price
		}
	}
	fund(price)
	apply(AdventureAction{Action: "buy", MarketID: "sg-market-01", ItemID: legacy.RequiredItem, Quantity: 1})
	apply(AdventureAction{Action: "learn", SkillID: legacy.ID, Hours: legacy.TrainingHours})
	if a.Skills[legacy.ID] != 1 || a.Currency != initial.Currency+earned-spent {
		t.Fatal("advanced learning or paid progression lost its actual state")
	}
	refused, _, err := ResolveAdventureActionAt(r, a, AdventureAction{Action: "refine", SkillID: "sg-blade", Hours: 1}, "beyond-finite-rank", elapsed)
	if err == nil || !reflect.DeepEqual(a, refused) {
		t.Fatal("finite maximum rank was bypassed or rejected with mutations")
	}
	t.Logf("final_rank=%d mastery=%d advanced_skill=%s in_world_hours=%d actions=%d earned=%d spent=%d balance=%d", a.Rank, a.Skills["sg-blade"], legacy.ID, elapsed/3600, actions, earned, spent, a.Currency)
}
