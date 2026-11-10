package app

import (
	"context"
	"errors"
	"testing"

	"ta/internal/character"
	"ta/internal/interactive"
)

func TestGeneratedAdultEligibilityUsesCommittedProfile(t *testing.T) {
	profiles := []character.Profile{
		{ID: "chen_qiaoqian", Name: "林枝", Age: 24, Gender: character.GenderFemale},
		{ID: "han_li", Name: "小岚", Age: 17, Gender: character.GenderFemale},
		{ID: "wu_ming", Name: "未定年岁", Gender: character.GenderFemale},
	}
	a, storyID := intimacyGenderTestApp(t, character.GenderMale, profiles)
	window, err := a.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if !character.ContainsID(window.Eligible, "chen_qiaoqian") {
		t.Fatalf("committed adult missing: %v", window.Eligible)
	}
	if character.ContainsID(window.Eligible, "han_li") || character.ContainsID(window.Eligible, "wu_ming") {
		t.Fatalf("minor or unknown age admitted: %v", window.Eligible)
	}
	_, err = a.InviteIntimacy(context.Background(), IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "adult-invite",
	})
	if !errors.Is(err, ErrIntimacyModelUnconfigured) {
		t.Fatalf("adult should reach the separate model gate, got %v", err)
	}
	if err := a.interactive.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := interactive.NewStore(a.cfg.DataDir())
	defer reopened.Close()
	a.interactive = reopened
	window, err = a.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if !character.ContainsID(window.Eligible, "chen_qiaoqian") {
		t.Fatal("adult age lost on reopen")
	}
}

func TestGeneratedAgeCannotOverrideAuthoredMinor(t *testing.T) {
	if intimacyProfileIsAdult(character.Profile{SourceKey: "young", Age: 25}, map[string]int{"young": 17}) {
		t.Fatal("a generated age overrode the frozen catalog")
	}
	if intimacyProfileIsAdult(character.Profile{SourceKey: "missing", Age: 25}, nil) {
		t.Fatal("an unrecognized source key became a generated adult")
	}
	if !intimacyProfileIsAdult(character.Profile{Age: 18}, nil) {
		t.Fatal("explicit adult rejected")
	}
	for _, age := range []int{0, 17, -1} {
		if intimacyProfileIsAdult(character.Profile{Age: age}, nil) {
			t.Fatalf("unproven adult age %d", age)
		}
	}
}
