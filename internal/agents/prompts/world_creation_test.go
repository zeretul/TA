package prompts

import (
	"strings"
	"testing"
)

// 自制-AI创建世界包与武侠规则复用 §6 draws a line between what the model writes
// and what the program owns. These tests hold the prompt to that line, and to the
// one thing §7 requires of the two entry points: the same boundary in both.

// worldCreationBoundaryRules are the sentences that must appear in every world
// draft prompt, whichever entry point produced it. A change that dropped one
// would let the model author a rule, grant something, or promise a system this
// build does not have.
var worldCreationBoundaryRules = []string{
	"- The player's material below is fixed.",
	"- Fill only what is missing.",
	"- Grant nothing and decide nothing.",
	"世界书应包含能指导推演的规则",
	"不能另造命中率、伤害公式、修炼速度、物价或代码模块来覆盖它",
	"do not promise mechanics this build does not have",
}

func TestBothEntryPointsShareOneBoundary(t *testing.T) {
	for _, mode := range []string{WorldCreationModeGuided, WorldCreationModeAutomatic} {
		instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
			Mode: mode, Brief: "一个被雪封住的驿镇。",
		})
		for _, rule := range worldCreationBoundaryRules {
			if !strings.Contains(instruction, rule) {
				t.Fatalf("%s prompt lost the shared rule %q:\n%s", mode, rule, instruction)
			}
		}
	}
}

func TestGenericWorldPromptUsesConfirmedActionChecksWithoutForcingAnArchetype(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeGuided, GameplayProfile: "world-actions-v1; communication=letter",
	})
	for _, phrase := range []string{
		"复用已有的调查、信息判断、交涉与追逐脱身检定",
		"不因此把主角固定为侦探",
		"starting positions that fit the player's chosen genre and protagonist",
		"Communication mode: letters across distance",
	} {
		if !strings.Contains(instruction, phrase) {
			t.Fatalf("generic world prompt is missing %q:\n%s", phrase, instruction)
		}
	}
	if strings.Contains(instruction, "This world runs on the cultivation state template") {
		t.Fatalf("generic world prompt still forces cultivation rules:\n%s", instruction)
	}
	if strings.Contains(instruction, "a fourth would have nothing behind it") {
		t.Fatalf("generic world prompt still imposes cultivation starting positions:\n%s", instruction)
	}
}

// TestTheTwoEntryPointsAreToldWhatTheyAreFor keeps a one-sentence start from
// being answered with a guided questionnaire's assumptions and the other way
// round.
func TestTheTwoEntryPointsAreToldWhatTheyAreFor(t *testing.T) {
	guided := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeGuided, Genre: "北境", Tone: "冷峻",
	})
	automatic := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeAutomatic, Brief: "一个被雪封住的驿镇。",
	})
	if !strings.Contains(guided, "The player answered questions") {
		t.Fatalf("the guided prompt does not say how the draft was started:\n%s", guided)
	}
	if !strings.Contains(automatic, "The player typed a single sentence") {
		t.Fatalf("the automatic prompt does not say how the draft was started:\n%s", automatic)
	}
	// The guided answers reach the model as fixed material.
	if !strings.Contains(guided, "Genre: 北境") || !strings.Contains(guided, "Tone: 冷峻") {
		t.Fatalf("the guided answers were not passed through:\n%s", guided)
	}
	// The automatic sentence does too, in its own section.
	if !strings.Contains(automatic, "一个被雪封住的驿镇。") {
		t.Fatalf("the player's sentence was not passed through:\n%s", automatic)
	}
}

// TestAnEmptyDraftDoesNotClaimAnythingWasAgreed keeps the first call honest: with
// nothing agreed yet, the prompt must not carry an "already agreed" section that
// tells the model to preserve a draft that does not exist.
func TestAnEmptyDraftDoesNotClaimAnythingWasAgreed(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeGuided, Brief: "一个被雪封住的驿镇。",
	})
	if strings.Contains(instruction, "Already agreed in the draft") {
		t.Fatalf("an empty draft produced an already-agreed section:\n%s", instruction)
	}
}

// TestAnAgreedDraftIsKeptVerbatim is the shared-draft requirement: a draft the
// player has edited must reach the model as material to preserve.
func TestAnAgreedDraftIsKeptVerbatim(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeGuided,
		Existing: WorldDraftInput{
			Name:   "寒山驿",
			Origin: "你是刚到寒山驿的旅人。",
			WorldEntries: []WorldEntryDraftInput{
				{Name: "寒山驿", Type: "setting", Brief: "北境驿镇。"},
			},
			Characters:   []WorldCharacterDraftInput{{Name: "柳四", Role: "驿丞"}},
			Origins:      []WorldOriginDraftInput{{Name: "行脚人", Basis: "wanderer"}},
			Affiliations: []string{"驿马帮"},
			Locations:    []string{"寒山驿"},
		},
	})
	for _, want := range []string{
		"Already agreed in the draft",
		"World name (keep verbatim): 寒山驿",
		"Premise (keep verbatim): 你是刚到寒山驿的旅人。",
		"- 寒山驿 [setting]: 北境驿镇。",
		"- 柳四 — 驿丞",
		"- 行脚人 [wanderer]",
		"驿马帮",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("the agreed draft is missing %q:\n%s", want, instruction)
		}
	}
}

// TestThePromptAsksForTheContentThePageNeeds pins the output contract to the
// fields the draft and the pack actually use. A field the prompt omits is a
// field the page shows empty.
func TestThePromptAsksForTheContentThePageNeeds(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: WorldCreationModeGuided, Brief: "一个被雪封住的驿镇。",
	})
	for _, field := range []string{
		`"name"`, `"description"`, `"title"`, `"origin"`,
		`"protagonist_name"`, `"protagonist"`, `"opening"`,
		`"world"`, `"characters"`, `"origins"`, `"affiliations"`, `"locations"`,
	} {
		if !strings.Contains(instruction, field) {
			t.Fatalf("the output contract does not ask for %s:\n%s", field, instruction)
		}
	}
	// The three positions the rules provide are named, and the prompt says a
	// fourth would have nothing behind it.
	for _, basis := range []string{"`wanderer`", "`disciple`", "`scion`"} {
		if !strings.Contains(instruction, basis) {
			t.Fatalf("the prompt does not name the supported position %s:\n%s", basis, instruction)
		}
	}
	if !strings.Contains(instruction, "a fourth would have nothing behind it") {
		t.Fatalf("the prompt does not warn against inventing a fourth position:\n%s", instruction)
	}
}

// TestAnUnknownModeFallsBackToGuided keeps a bad mode from producing a prompt
// written for neither entry point.
func TestAnUnknownModeFallsBackToGuided(t *testing.T) {
	instruction := BuildWorldCreationSystemInstruction(WorldCreationSystemInstructionInput{
		Mode: "freestyle", Brief: "一个被雪封住的驿镇。",
	})
	if !strings.Contains(instruction, "The player answered questions") {
		t.Fatalf("an unknown mode must fall back to the guided wording:\n%s", instruction)
	}
}
