package prompts

import "testing"

// TestIntimacyLineTargetFollowsModelFamily pins the per-model length targets.
//
// One shared number cannot serve both models: DeepSeek stops short of it and
// Gemini runs past it into a truncated response. The targets are therefore
// chosen from the resolved model id, and a wrong branch here shows up as a lost
// round rather than as a compile error.
func TestIntimacyLineTargetFollowsModelFamily(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		rosterSize int
		requested  int
		want       int
	}{
		{"deepseek solo", "deepseek-v4.1-flash", 1, 0, intimacyDeepSeekLineTarget},
		{"deepseek full roster", "deepseek-v4.1-flash", 4, 0, intimacyDeepSeekManySpeakerCap},
		{"gemini solo", "gemini-3.8-flash-high", 1, 0, intimacyGeminiLineTarget},
		{"gemini full roster", "gemini-3.8-flash-high", 4, 0, intimacyGeminiManySpeakerCap},
		{"unknown model solo", "some-other-model", 1, 0, DefaultIntimacyLineTargetChars},
		{"unknown model full roster", "some-other-model", 4, 0, intimacyManySpeakerLineCap},
		{"empty model", "", 2, 0, DefaultIntimacyLineTargetChars},
		// An explicit request wins, except where the generic cap would undo the
		// larger target DeepSeek needs.
		{"explicit request honoured", "gemini-3.8-flash-high", 1, 300, 300},
		{"explicit request capped by roster", "gemini-3.8-flash-high", 3, 900, intimacyManySpeakerLineCap},
		{"explicit request kept for deepseek", "deepseek-v4.1-flash", 3, 900, 900},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := intimacyLineTargetFor(c.model, c.rosterSize, c.requested)
			if got != c.want {
				t.Fatalf("intimacyLineTargetFor(%q, %d, %d) = %d, want %d",
					c.model, c.rosterSize, c.requested, got, c.want)
			}
		})
	}
}

// TestIntimacyModelAdapterIsModelSpecific guards the dispatch: each family gets
// its own addendum, and a model with nothing to correct gets none, so the caller
// sends the composed instruction unchanged.
func TestIntimacyModelAdapterIsModelSpecific(t *testing.T) {
	deepseek := IntimacyModelAdapter("deepseek-v4.1-flash")
	gemini := IntimacyModelAdapter("gemini-3.8-flash-high")

	if deepseek == "" || gemini == "" {
		t.Fatalf("an adapter is missing: deepseek=%q gemini=%q", deepseek, gemini)
	}
	if deepseek == gemini {
		t.Fatal("both families got the same addendum")
	}
	if IntimacyModelAdapter("some-other-model") != "" {
		t.Fatal("an unknown model got an addendum")
	}
	if IntimacyModelAdapter("") != "" {
		t.Fatal("an empty model got an addendum")
	}
}

// TestIntimacyRoundInstructionCarriesTheModelTarget checks the resolved model
// actually reaches the instruction the round is written from.
func TestIntimacyRoundInstructionCarriesTheModelTarget(t *testing.T) {
	instruction := BuildIntimacyRoundInstruction(IntimacyRoundInput{
		Members: []GroupChatMember{{ID: "lin", Name: "林晚"}},
		Model:   "deepseek-v4.1-flash",
	})
	generic := BuildIntimacyRoundInstruction(IntimacyRoundInput{
		Members: []GroupChatMember{{ID: "lin", Name: "林晚"}},
	})
	if instruction == generic {
		t.Fatal("the model did not change the instruction")
	}
}
