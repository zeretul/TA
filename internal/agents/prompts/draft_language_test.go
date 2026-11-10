package prompts

import (
	"strings"
	"testing"
)

// The page's language decides what the prose is written in, not the world's.
// These tests pin that for the two drafts a player reads before any story
// exists: the character sheet and the world.

func TestACharacterDraftFollowsThePageLanguageNotTheWorld(t *testing.T) {
	instruction := BuildStoryCreationSystemInstruction(StoryCreationSystemInstructionInput{
		WorldName:       "凡人人界",
		WorldBrief:      "一个以灵石、灵根与境界衡量一切的世界。",
		RealmLabel:      "炼气一层",
		Name:            "Shen Yanzhou",
		Gender:          "male",
		Age:             24,
		Origin:          "散修",
		Affiliation:     "无宗门",
		Location:        "坊市",
		Roots:           "金水双灵根",
		Aptitude:        "良好",
		WeaponDirection: "剑",
		Traits:          "沉稳、好奇",
		OutputLanguage:  "en-US",
	})
	if !strings.Contains(instruction, "所有散文字段都用英文书写") {
		t.Fatal("an English page asked for a draft in the world's language")
	}
	if strings.Contains(instruction, "与下方的世界材料相同的语言") {
		t.Fatal("the language line still follows the world material")
	}
	if !strings.Contains(instruction, "沿用材料给出的写法") {
		t.Fatal("the instruction does not keep established names as the world wrote them")
	}
}

func TestACharacterDraftWithoutAPageLanguageFollowsTheWorld(t *testing.T) {
	instruction := BuildStoryCreationSystemInstruction(StoryCreationSystemInstructionInput{
		WorldName: "凡人人界", WorldBrief: "一个以灵石衡量的世界。",
	})
	if !strings.Contains(instruction, "与下方的世界材料相同的语言") {
		t.Fatal("a request that names no language must still follow the world material")
	}
}

func TestAWorldDraftFollowsThePageLanguage(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode:           WorldCreationModeAutomatic,
		Brief:          "一座靠潮汐运转的港口城市。",
		OutputLanguage: "en-US",
	})
	if !strings.Contains(instruction, "所有散文字段都用英文书写") {
		t.Fatal("an English page asked for a world draft in the player's own language")
	}
	if strings.Contains(instruction, "与下方玩家提供的材料相同的语言") {
		t.Fatal("the language line still follows the player's material")
	}
}

func TestAChinesePageAsksForChineseProse(t *testing.T) {
	if got := proseLanguage("zh-CN"); got != "Chinese (Simplified)" {
		t.Fatalf("zh-CN resolves to %q", got)
	}
	if got := proseLanguage(""); got != "" {
		t.Fatalf("an unset locale must not name a language, got %q", got)
	}
	if got := proseLanguage("auto"); got != "" {
		t.Fatalf("auto must follow the material, got %q", got)
	}
	if got := proseLanguage("en-US"); got != "English" {
		t.Fatalf("en-US resolves to %q", got)
	}
}
