package storypack

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

func TestWorldPresentationRoundTripsThroughDraftLibraryAndFrozenBinding(t *testing.T) {
	draft := draftTestWorld()
	draft.Gameplay.Presentation = interactive.WorldPresentation{
		"zh-CN": {Exploration: "迷雾调查", Techniques: "非凡能力", Partner: "伴侣"},
		"en-US": {Exploration: "Occult investigation", Techniques: "Abilities", Partner: "Partner"},
	}
	library := NewLibrary(t.TempDir())
	saved, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := library.Get(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pack.Story.Gameplay.Presentation, draft.Gameplay.Presentation) {
		t.Fatal("draft vocabulary was lost on save/load")
	}
	binding := pack.Binding()
	if binding.Gameplay == nil || !reflect.DeepEqual(binding.Gameplay.Presentation, draft.Gameplay.Presentation) {
		t.Fatal("runtime binding omitted authored vocabulary")
	}
	raw, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	var decoded interactive.StoryPackBinding
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Gameplay.Presentation, binding.Gameplay.Presentation) {
		t.Fatal("JSON boundary changed vocabulary")
	}
	oldHash := pack.ContentHash()
	oldModule := pack.Story.ModuleRefs.ActorStateID
	pack.Story.Gameplay.Presentation["zh-CN"] = interactive.WorldPresentationTerms{Exploration: "新修订名称"}
	if pack.ContentHash() == oldHash {
		t.Fatal("vocabulary edit failed to create a content revision")
	}
	if binding.Gameplay.Presentation["zh-CN"].Exploration != "迷雾调查" {
		t.Fatal("editing current author data changed a frozen binding")
	}
	if pack.Story.ModuleRefs.ActorStateID != oldModule {
		t.Fatal("display edit rewrote mechanical module identity")
	}
}

func TestCultivationCustomCraftWorldKeepsCapabilityAndDisplayProfile(t *testing.T) {
	draft := draftTestWorld()
	draft.Gameplay.StatusTemplate = WorldStatusCultivation
	draft.Gameplay.NumericScale = WorldScaleCultivation
	draft.Gameplay.Growth = WorldGrowthCultivation
	draft.Gameplay.Combat = WorldCombatCultivation
	draft.Gameplay.Communication = WorldCommunicationCultivationSignal
	draft.Gameplay.Presentation = interactive.WorldPresentation{"zh-CN": {Techniques: "斗技", Energy: "斗气"}}
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	method.ID = "world.presentation.ward"
	method.Source = "author manuscript"
	method.FailureCost = "consume_inputs"
	method.LossChance = 0
	draft.Gameplay.CraftMethods = []cultivation.CraftMethodDef{method}
	library := NewLibrary(t.TempDir())
	saved, err := library.SaveWorld(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := library.Get(saved.PackID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pack.Story.ModuleRefs.ActorStateID, "world-actor-state-") {
		t.Fatal("test did not materialize the custom state module")
	}
	binding := pack.Binding()
	if binding.Gameplay == nil || binding.Gameplay.StatusTemplate != WorldStatusCultivation || binding.Gameplay.Combat != WorldCombatCultivation || binding.Gameplay.Presentation["zh-CN"].Techniques != "斗技" {
		t.Fatalf("custom cultivation world lost its declared capability: %+v", binding.Gameplay)
	}
	built, err := draft.BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	draft.Gameplay.Presentation["zh-CN"] = interactive.WorldPresentationTerms{Techniques: "edited draft"}
	if built.Story.Gameplay.Presentation["zh-CN"].Techniques != "斗技" {
		t.Fatal("built pack shares mutable draft vocabulary")
	}
}

func TestLegacyCultivationPresentationDoesNotInventCommunication(t *testing.T) {
	pack := Pack{ID: "legacy", Story: PackStory{ModuleRefs: PackRefs{ActorStateID: interactive.ActorStateXiuxianID}}}
	if pack.Binding().Gameplay != nil {
		t.Fatal("legacy profile changed without an authored override")
	}
	pack.Story.Gameplay.Presentation = interactive.WorldPresentation{"zh-CN": {Exploration: "古迹"}}
	profile := pack.Binding().Gameplay
	if profile == nil || profile.StatusTemplate != WorldStatusCultivation || profile.Combat != WorldCombatCultivation || profile.Communication != "" {
		t.Fatalf("legacy vocabulary changed channel mechanics: %+v", profile)
	}
}

func TestWorldDraftPresentationValidationReturnsLocalizedErrorKey(t *testing.T) {
	draft := draftTestWorld()
	draft.Gameplay.Presentation = interactive.WorldPresentation{"xx": {Combat: "Duel"}}
	_, err := draft.BuildPack()
	var invalid *WorldGameplayValidationError
	if !errors.As(err, &invalid) || invalid.Key != "api.storyPack.invalidWorldPresentation" {
		t.Fatalf("invalid labels did not use a localized authoring refusal: %v", err)
	}
}
