package app

import (
	"strings"
	"ta/internal/interactive"
)

// This read-only progression card is derived from the story's frozen rules.
// It neither starts an action nor exposes a different actor's capabilities.
type AdventureProgressionView struct {
	StageName          string   `json:"stage_name"`
	Rank               int      `json:"rank"`
	MaxRank            int      `json:"max_rank"`
	Experience         int      `json:"experience"`
	RequiredExperience int      `json:"required_experience"`
	NextStageName      string   `json:"next_stage_name,omitempty"`
	NextStageRank      int      `json:"next_stage_rank,omitempty"`
	FoundationIDs      []string `json:"-"`
	FoundationID       string   `json:"foundation_id,omitempty"`
	FoundationName     string   `json:"foundation_name,omitempty"`
	RequiredMastery    int      `json:"required_mastery"`
	ExperiencePerHour  int      `json:"experience_per_hour"`
	CurrencyPerHour    int      `json:"currency_per_hour"`
}

func adventureProgressionView(r interactive.AdventureRules, a interactive.AdventureActor, locale string) *AdventureProgressionView {
	stage := interactive.AdventureProgressionStageFor(r, a.Rank)
	if stage == nil {
		return nil
	}
	name := func(stage interactive.AdventureProgressionStage) string {
		if strings.HasPrefix(locale, "en") {
			return stage.EnglishName
		}
		return stage.Name
	}
	view := &AdventureProgressionView{StageName: name(*stage), Rank: a.Rank, MaxRank: interactive.AdventureMaxRank(r), Experience: a.Experience, FoundationID: stage.PracticeSkill, RequiredMastery: stage.PracticeMastery, ExperiencePerHour: r.Progression.ExperiencePerHour, CurrencyPerHour: r.Progression.CurrencyPerHour}
	if a.Rank < view.MaxRank {
		view.RequiredExperience = interactive.AdventureRankExperience(r, a.Rank)
	}
	for _, next := range r.Progression.Stages {
		if next.MinRank > a.Rank {
			view.NextStageName, view.NextStageRank = name(next), next.MinRank
			break
		}
	}
	view.FoundationIDs = append([]string{stage.PracticeSkill}, stage.PracticeAlternatives...)
	var names []string
	for _, id := range view.FoundationIDs {
		for _, skill := range r.Skills {
			if skill.ID == id {
				names = append(names, adventureDefinitionName(skill.ID, skill.Name, skill.DefinitionDetails, locale))
				break
			}
		}
	}
	view.FoundationName = strings.Join(names, " / ")
	return view
}
