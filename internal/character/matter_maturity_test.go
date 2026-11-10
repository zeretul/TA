package character

import (
	"math"
	"strings"
	"testing"
)

func TestDelayedOrdinaryMatterNeedsSpokenSourceAndRealConfirmation(t *testing.T) {
	ctx := testProjectionContext()
	ctx.ClockSeconds = 100
	facts := TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "steward", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "我明天再同你核对这批货的来路。"}}, Matters: []MatterDraft{{CharacterID: "steward", Kind: MatterPromise, Status: MatterOpen, Statement: "管事约定明天再核对来路。", AfterSeconds: 86400, SourceExchangeIndex: 1}}}
	scheduled, err := Project(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	original := scheduled.Matters[0]
	if original.EarliestSeconds != 86500 || original.DeadlineSeconds != 0 || original.ExchangeID != scheduled.Exchanges[0].ID {
		t.Fatal("maturity was not a source-bound earliest time")
	}
	for _, now := range []int64{100, 86499, 86500, 100000} {
		live, _ := LiveMattersAt("steward", ctx.WorldTime, now, []Matter{original})
		if len(live) != 1 || live[0].Status != MatterOpen {
			t.Fatal("clock alone completed or expired an open agreement")
		}
	}
	pending := prepareInteractions(Profile{ID: "steward"}, []Matter{original}, ctx.WorldTime, 100)
	ready := prepareInteractions(Profile{ID: "steward"}, []Matter{original}, ctx.WorldTime, 86500)
	if !strings.Contains(pending, "尚余") || !strings.Contains(ready, "仍须实际回应") {
		t.Fatal("narrator does not distinguish waiting from completed work")
	}
	ctx.ExistingMatters = []Matter{original}
	reply := TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "steward", Speaker: SpeakerCharacter, Channel: ChannelPrivate, Text: "来路已经核对，我把原记录给你看。"}}, Matters: []MatterDraft{{CharacterID: "steward", Kind: MatterAction, Status: MatterSettled, Statement: "管事拿出核对后的原记录。", Resolves: original.ID, SourceExchangeIndex: 1}}}
	ctx.ClockSeconds = 86499
	if _, err := Project(ctx, reply); err == nil {
		t.Fatal("settled before the promised earliest time")
	}
	ctx.ClockSeconds = 86500
	noWords := reply
	noWords.Exchanges = nil
	if _, err := Project(ctx, noWords); err == nil {
		t.Fatal("time alone fabricated an actual response")
	}
	confirmed, err := Project(ctx, reply)
	if err != nil {
		t.Fatal(err)
	}
	merged := ApplyMatters([]Matter{original}, confirmed.Matters)
	prior, ok := MatterByID(merged, original.ID)
	if !ok || prior.Status != MatterSettled || prior.EarliestSeconds != original.EarliestSeconds {
		t.Fatal("confirmation lost the source agreement")
	}
	ctx.ClockSeconds = 100
	abandoned := TurnCharacterFacts{Matters: []MatterDraft{{CharacterID: "steward", Kind: MatterPromise, Status: MatterAbandoned, Statement: "不再等这次核对。", Resolves: original.ID}}}
	if _, err := Project(ctx, abandoned); err != nil {
		t.Fatalf("player cannot leave an unfinished thread: %v", err)
	}
}

func TestDelayedMatterRejectsInventedAgreementAndUnsafeTime(t *testing.T) {
	for _, test := range []struct {
		name         string
		clock, delay int64
		speaker      Speaker
		kind         MatterKind
	}{
		{"player_intent", 0, 100, SpeakerPlayer, MatterPromise},
		{"negative", 0, -1, SpeakerCharacter, MatterPromise},
		{"overflow", math.MaxInt64 - 1, 100, SpeakerCharacter, MatterPromise},
		{"permission_is_not_future_work", 0, 100, SpeakerCharacter, MatterPermission},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := testProjectionContext()
			ctx.ClockSeconds = test.clock
			_, err := Project(ctx, TurnCharacterFacts{Exchanges: []ExchangeDraft{{CharacterID: "steward", Speaker: test.speaker, Channel: ChannelPrivate, Text: "以后再说。"}}, Matters: []MatterDraft{{CharacterID: "steward", Kind: test.kind, Status: MatterOpen, Statement: "等待本人答复。", AfterSeconds: test.delay, SourceExchangeIndex: 1}}})
			if err == nil {
				t.Fatal("invalid delayed promise accepted")
			}
		})
	}
}

func TestAuthoredMatterTranslationIsDisplayOnlyAndCopied(t *testing.T) {
	original := Matter{ID: "mat-1", Statement: "愿意再商量一次。", StatementTranslations: map[string]string{"en-US": "I am willing to discuss it again."}}
	cloned := original.Normalize()
	cloned.StatementTranslations["en-US"] = "changed"
	if original.StatementTranslations["en-US"] == "changed" || original.Statement != "愿意再商量一次。" {
		t.Fatal("display translation aliases the committed matter")
	}
}
