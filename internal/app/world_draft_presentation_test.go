package app

import (
	"context"
	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestWorldDraftRewritePreservesOmittedVocabularyAndHonorsExplicitClear(t *testing.T) {
	for _, mode := range []string{"omitted", "clear", "replace"} {
		t.Run(mode, func(t *testing.T) {
			gameplay := storypack.RecommendedWorldGameplay()
			gameplay.Confirmed = true
			authored := interactive.WorldPresentation{"zh-CN": {Exploration: "城堡探索"}}
			draft := storypack.WorldDraft{Gameplay: gameplay}
			draft.Gameplay.Presentation = authored
			request := StoryPackWorldDraftRequest{Draft: &draft, Gameplay: gameplay, Rewrite: true}
			if mode == "clear" {
				request.Gameplay.Presentation = interactive.WorldPresentation{}
			}
			if mode == "replace" {
				request.Gameplay.Presentation = interactive.WorldPresentation{"zh-CN": {Exploration: "魔法遗迹"}}
			}
			application := &App{cfg: &config.Config{DenovaDir: t.TempDir()}}
			result, err := application.StoryPacks().DraftWorld(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.ErrorKey != "worldCreation.premiseRequired" {
				t.Fatalf("test unexpectedly reached model generation: %s", result.ErrorKey)
			}
			want := "城堡探索"
			if mode == "clear" {
				want = ""
			}
			if mode == "replace" {
				want = "魔法遗迹"
			}
			if got := result.Draft.Gameplay.Presentation["zh-CN"].Exploration; got != want {
				t.Fatalf("rewrite vocabulary=%q, want %q", got, want)
			}
			authored["zh-CN"] = interactive.WorldPresentationTerms{Exploration: "changed input"}
			if got := result.Draft.Gameplay.Presentation["zh-CN"].Exploration; got != want {
				t.Fatal("returned draft shares its input map")
			}
		})
	}
}

func TestWorldDraftRecommendedDefaultsRetainExplicitVocabulary(t *testing.T) {
	application := &App{cfg: &config.Config{DenovaDir: t.TempDir()}}
	request := StoryPackWorldDraftRequest{Gameplay: storypack.WorldGameplay{Presentation: interactive.WorldPresentation{
		"en-US": {Exploration: "Castle exploration"},
	}}}
	result, err := application.StoryPacks().DraftWorld(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ErrorKey != "worldCreation.premiseRequired" || result.Draft.Gameplay.Presentation["en-US"].Exploration != "Castle exploration" {
		t.Fatal("recommended gameplay discarded explicit author vocabulary")
	}
}
