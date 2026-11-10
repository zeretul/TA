package app

import (
	"context"
	"errors"
	"testing"

	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
)

// This file pins the opposite-gender rule the intimacy window now states.
//
// The window's eligibility is the one place the program decides who may be
// asked, so both halves of the rule live here rather than only in the surface:
// an unknown gender on either side is not permission, and two settled genders
// that agree are not permission either. A direct API call is refused on the same
// terms as the form, because the form is only a view of this list.

func TestIntimacyOppositeGenderRefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name              string
		protagonistGender string
		personGender      string
		want              bool
	}{
		{"settled and different", character.GenderMale, character.GenderFemale, true},
		{"settled and different, other way", character.GenderFemale, character.GenderMale, true},
		{"same gender", character.GenderMale, character.GenderMale, false},
		{"protagonist unknown", "", character.GenderFemale, false},
		{"person unknown", character.GenderMale, "", false},
		{"both unknown", "", "", false},
		// The stored vocabulary is normalised before the comparison, so a value
		// written in another form is read rather than silently treated as a
		// different gender.
		{"written in Chinese", "男", character.GenderFemale, true},
		{"an unreadable value is not a third gender", character.GenderMale, "unknown", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intimacyOppositeGender(tc.protagonistGender, tc.personGender); got != tc.want {
				t.Fatalf("intimacyOppositeGender(%q, %q) = %v, want %v", tc.protagonistGender, tc.personGender, got, tc.want)
			}
		})
	}
}

// intimacyGenderTestApp is a story with one man and one woman standing in the
// same room, both adult. Only the protagonist's own gender varies between the
// cases below, so whatever changes is the gender rule and nothing else.
func intimacyGenderTestApp(t *testing.T, protagonistGender string, profiles []character.Profile) (*App, string) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{NovaDir: root, Workspace: root}
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	system, err := interactive.NewActorStateLibrary(t.TempDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("load the xiuxian module: %v", err)
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:             "异性门槛",
		ActorState:        &system.ActorState,
		ProtagonistGender: protagonistGender,
		PackCatalog: &interactive.PackCatalogSnapshot{Characters: []interactive.PackCatalogPerson{
			{SourceKey: "npc_chen_qiaoqian", Name: "陈巧倩", Age: 26},
			{SourceKey: "npc_han_li", Name: "韩立", Age: 24},
			{SourceKey: "npc_wu_ming", Name: "无名", Age: 30},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "推门进客舍", Narrative: "陈巧倩坐在灯下，韩立在窗边，无名在一旁听着。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID,
			FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "chen_qiaoqian", "han_li", "wu_ming"},
			Reason: "本回合确认的在场角色",
		}},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{
		BranchID: "main", WorldTime: "午后",
		Profiles: profiles,
	}); err != nil {
		t.Fatal(err)
	}
	return &App{cfg: cfg, workspace: root, interactive: store}, story.ID
}

func intimacyGenderTestProfiles() []character.Profile {
	return []character.Profile{
		{ID: "chen_qiaoqian", SourceKey: "npc_chen_qiaoqian", Name: "陈巧倩", Role: "掩月宗修士", Gender: character.GenderFemale},
		{ID: "han_li", SourceKey: "npc_han_li", Name: "韩立", Role: "散修", Gender: character.GenderMale},
		// 无名 is an adult the story never settled a gender for. That is a real
		// state, and it is not a licence to include them.
		{ID: "wu_ming", SourceKey: "npc_wu_ming", Name: "无名", Role: "散修"},
	}
}

func TestIntimacyWindowOffersOnlyTheOppositeSettledGender(t *testing.T) {
	application, storyID := intimacyGenderTestApp(t, character.GenderMale, intimacyGenderTestProfiles())
	window, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatalf("read the intimacy window: %v", err)
	}
	if window.ProtagonistGender != character.GenderMale {
		t.Fatalf("the window reports protagonist gender %q, want %q", window.ProtagonistGender, character.GenderMale)
	}
	if !character.ContainsID(window.Eligible, "chen_qiaoqian") {
		t.Fatalf("the woman in the room is not offered to a male protagonist: %+v", window.Eligible)
	}
	if character.ContainsID(window.Eligible, "han_li") {
		t.Fatalf("a man is offered to a male protagonist: %+v", window.Eligible)
	}
	if character.ContainsID(window.Eligible, "wu_ming") {
		t.Fatalf("a person whose gender the story never settled is offered: %+v", window.Eligible)
	}
}

func TestIntimacyInvitationRefusesSomeoneOfTheSameGender(t *testing.T) {
	application, storyID := intimacyGenderTestApp(t, character.GenderMale, intimacyGenderTestProfiles())
	_, err := application.InviteIntimacy(context.Background(), IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "han_li", OneOnOne: true, CommandID: "invite-same-gender",
	})
	if !errors.Is(err, ErrIntimacySameGender) {
		t.Fatalf("inviting someone of the same gender returned %v, want ErrIntimacySameGender", err)
	}
}

func TestIntimacyInvitationRefusesWhenTheProtagonistGenderIsUnknown(t *testing.T) {
	application, storyID := intimacyGenderTestApp(t, "", intimacyGenderTestProfiles())
	window, err := application.IntimacyWindow(IntimacyWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatalf("read the intimacy window: %v", err)
	}
	if window.ProtagonistGender != "" {
		t.Fatalf("the window invented a protagonist gender: %q", window.ProtagonistGender)
	}
	if len(window.Eligible) != 0 {
		t.Fatalf("an unsettled protagonist gender still offers people: %+v", window.Eligible)
	}
	_, err = application.InviteIntimacy(context.Background(), IntimacyInviteRequest{
		StoryID: storyID, CharacterID: "chen_qiaoqian", OneOnOne: true, CommandID: "invite-unknown-protagonist",
	})
	if !errors.Is(err, ErrIntimacyProtagonistGenderUnknown) {
		t.Fatalf("inviting with an unsettled protagonist gender returned %v, want ErrIntimacyProtagonistGenderUnknown", err)
	}
}

// TestIntimacyProtagonistGenderReadsAnExplicitStatementOnly keeps the reading of
// a story created before the frozen field existed: the body it was created from
// is still on the story, and an explicit statement there is read the same way
// the creation path reads it. A body that says nothing stays empty.
func TestIntimacyProtagonistGenderReadsAnExplicitStatementOnly(t *testing.T) {
	cases := []struct {
		name string
		meta interactive.StoryMeta
		want string
	}{
		{
			name: "the frozen field wins",
			meta: interactive.StoryMeta{
				ProtagonistGender: character.GenderFemale,
				Protagonist:       interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom, Profile: "性别：男。"},
			},
			want: character.GenderFemale,
		},
		{
			name: "a story created before the field falls back to its own body",
			meta: interactive.StoryMeta{
				Protagonist: interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom, Profile: "姓名：顾承川。性别：男，年龄：22 岁。"},
			},
			want: character.GenderMale,
		},
		{
			name: "a body that states nothing stays unknown",
			meta: interactive.StoryMeta{
				Protagonist: interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom, Profile: "姓名：顾承川。一个散修。"},
			},
			want: "",
		},
		{
			name: "a name is not evidence",
			meta: interactive.StoryMeta{
				Protagonist: interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeCustom, Name: "陈巧倩", Profile: "姓名：陈巧倩。"},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intimacyProtagonistGender(interactive.StoryContext{Meta: tc.meta}); got != tc.want {
				t.Fatalf("intimacyProtagonistGender = %q, want %q", got, tc.want)
			}
		})
	}
}
