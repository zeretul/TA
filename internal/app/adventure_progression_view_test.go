package app

import (
	"ta/internal/interactive"
	"testing"
)

func TestAdventureProgressionViewUsesFrozenLocaleAndOpenCeiling(t *testing.T) {
	r := interactive.AdventureRules{Family: "space", Progression: &interactive.AdventureProgression{MaxRank: 60, ExperiencePerHour: 24, CurrencyPerHour: 2, Stages: []interactive.AdventureProgressionStage{{ID: "f", Name: "F级", EnglishName: "F", MinRank: 1, PracticeMastery: 1}, {ID: "divine", Name: "超神级", EnglishName: "Transcendent", MinRank: 51, PracticeMastery: 2}}}}
	a := interactive.AdventureActor{Rank: 10, Experience: 20}
	zh := adventureProgressionView(r, a, "zh-CN")
	if zh.StageName != "F级" || zh.NextStageName != "超神级" || zh.RequiredExperience != 1000 {
		t.Fatalf("wrong frozen growth view: %+v", zh)
	}
	a.Rank = 60
	en := adventureProgressionView(r, a, "en-US")
	if en.StageName != "Transcendent" || en.RequiredExperience != 0 || en.NextStageName != "" {
		t.Fatalf("ceiling became an extra stage: %+v", en)
	}
	r.Progression = nil
	if adventureProgressionView(r, a, "en-US") != nil {
		t.Fatal("legacy story acquired invented labels")
	}
}
