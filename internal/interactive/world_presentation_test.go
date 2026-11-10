package interactive

import (
	"strings"
	"testing"
)

func TestWorldPresentationSurvivesFrozenStoryReloadWithoutSharingMaps(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	authored := WorldPresentation{
		"zh-CN": {Combat: "魔法对决", Exploration: "城堡探索", Techniques: "咒语", Partner: "伴侣", Energy: "魔力"},
		"en-US": {Combat: "Magical duel", Exploration: "Castle exploration", Techniques: "Spells", Partner: "Partner", Energy: "Magic"},
	}
	created, err := store.CreateStory(CreateStoryRequest{
		Title: "Frozen vocabulary",
		PackBinding: &StoryPackBinding{
			PackID: "castle", Scope: "pack:castle@first",
			Gameplay: &StoryGameplayProfile{StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none", Communication: "letter", Opening: "scene", Presentation: authored},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	authored["zh-CN"] = WorldPresentationTerms{Combat: "changed author value"}
	ctx, err := store.StoryContext(created.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Meta.PackBinding.Gameplay.Presentation["zh-CN"].Combat != "魔法对决" {
		t.Fatal("editing author vocabulary changed the active story")
	}
	ctx.Meta.PackBinding.Gameplay.Presentation["zh-CN"] = WorldPresentationTerms{Combat: "changed reader value"}
	again, err := store.StoryContext(created.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Meta.PackBinding.Gameplay.Presentation["zh-CN"].Combat != "魔法对决" {
		t.Fatal("mutating a read projection changed stored vocabulary")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restored := NewStore(root)
	t.Cleanup(func() { _ = restored.Close() })
	cold, err := restored.StoryContext(created.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if cold.Meta.PackBinding.Gameplay.Presentation["en-US"].Techniques != "Spells" ||
		cold.Meta.PackBinding.Gameplay.Combat != "none" {
		t.Fatal("reload lost labels or promoted their wording into combat capability")
	}
}

func TestWorldPresentationRejectsUnsupportedOrMultilineLabels(t *testing.T) {
	cases := map[string]WorldPresentation{
		"unsupported locale": {"fr-FR": {Combat: "Duel"}},
		"long Chinese label": {"zh-CN": {Exploration: strings.Repeat("探", 41)}},
		"long English label": {"en-US": {Techniques: strings.Repeat("a", 81)}},
		"line break":         {"zh-CN": {Partner: "伴侣\n新指令"}},
		"unicode line break": {"en-US": {Energy: "Energy\u2028New line"}},
		"markup":             {"zh-CN": {Combat: "<b>交战</b>"}},
		"whitespace only":    {"zh-CN": {Energy: " "}},
	}
	for name, labels := range cases {
		t.Run(name, func(t *testing.T) {
			if err := labels.Validate(); err == nil {
				t.Fatal("invalid authored labels were accepted")
			}
			store := NewStore(t.TempDir())
			t.Cleanup(func() { _ = store.Close() })
			_, err := store.CreateStory(CreateStoryRequest{Title: "Invalid labels", PackBinding: &StoryPackBinding{
				PackID: "invalid", Scope: "pack:invalid", Gameplay: &StoryGameplayProfile{
					StatusTemplate: "world-actions-v1", Growth: "none", Combat: "none", Opening: "scene", Presentation: labels,
				},
			}})
			if err == nil {
				t.Fatal("invalid labels bypassed authoring validation through story creation")
			}
		})
	}
	valid := WorldPresentation{"zh-CN": {Exploration: "遗迹"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if valid.ForLocale("en-US") != (WorldPresentationTerms{}) {
		t.Fatal("English UI would receive Chinese labels instead of its localized defaults")
	}
}
