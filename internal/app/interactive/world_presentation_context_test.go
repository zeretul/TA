package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
)

func TestNarratorUsesOnlyFrozenActiveLanguageVocabulary(t *testing.T) {
	binding := &interactive.StoryPackBinding{PackID: "castle", Scope: "pack:castle", Gameplay: &interactive.StoryGameplayProfile{
		Combat: "none", Communication: "letter",
		Presentation: interactive.WorldPresentation{
			"zh-CN": {Combat: "魔法对决", Exploration: "城堡探索"},
			"en-US": {Combat: "Magical duel", Exploration: "Castle exploration"},
		},
	}}
	chinese := worldPresentationContext(binding, "zh-CN")
	english := worldPresentationContext(binding, "en-GB")
	if !strings.Contains(chinese, "魔法对决") || strings.Contains(chinese, "Magical duel") ||
		!strings.Contains(english, "Magical duel") || strings.Contains(english, "魔法对决") {
		t.Fatal("narrator vocabulary leaked across UI languages")
	}
	if binding.Gameplay.Combat != "none" || binding.Gameplay.Communication != "letter" {
		t.Fatal("display wording mutated capabilities")
	}
	delete(binding.Gameplay.Presentation, "en-US")
	if worldPresentationContext(binding, "en-US") != "" {
		t.Fatal("missing English labels should retain normal English narration")
	}
}
