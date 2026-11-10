package handlers

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	"ta/internal/interactive"
)

func TestInteractiveStoryStructureBusyErrorUsesRequestLocale(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "故事正在生成，请在本轮结束后再修改主角或状态结构",
		"en-US": "The story is generating. Change the protagonist or state structure after this turn finishes.",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", locale)
			writeErrorKey(ctx, 409, "api.interactive.storyStructureBusy")

			var body map[string]string
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != want {
				t.Fatalf("localized error = %q, want %q", body["error"], want)
			}
		})
	}
}

// TestCombatActorConflictNamesTheRunningBattle keeps R1's refusal actionable: the
// response has to carry the battle that already holds the actor, or the window
// can only disable a button rather than open the fight the player is in.
func TestCombatActorConflictNamesTheRunningBattle(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("X-Denova-Locale", "en-US")
	writeCombatWindowError(ctx, &interactive.CombatActorConflict{
		Kind: interactive.ErrCombatActorAlreadyFighting,
		SessionID: "cbt-running",
		Actors: []string{"protagonist"},
	})

	if ctx.Response.StatusCode() != 409 {
		t.Fatalf("an actor already fighting must be a conflict, got %d", ctx.Response.StatusCode())
	}
	var body agentRuntimeErrorResponse
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "api.combatWindow.actorAlreadyFighting" {
		t.Fatalf("the refusal must name its own reason, got %q", body.Code)
	}
	if got := body.Details["session_id"]; got != "cbt-running" {
		t.Fatalf("the refusal must carry the running battle, got %#v", body.Details)
	}
}

// TestCombatStaleBattleIsItsOwnRefusal keeps a superseded round distinguishable
// from a second opening: the two need different wording and only one of them is
// the player's own duplicate request.
func TestCombatStaleBattleIsItsOwnRefusal(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.Set("X-Denova-Locale", "en-US")
	writeCombatWindowError(ctx, &interactive.CombatActorConflict{
		Kind: interactive.ErrCombatStaleBattle,
		SessionID: "cbt-later",
		Actors: []string{"protagonist"},
	})

	if ctx.Response.StatusCode() != 409 {
		t.Fatalf("a superseded battle must be a conflict, got %d", ctx.Response.StatusCode())
	}
	var body agentRuntimeErrorResponse
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "api.combatWindow.staleBattle" {
		t.Fatalf("a superseded round must say so, got %q", body.Code)
	}
	if got := body.Details["session_id"]; got != "cbt-later" {
		t.Fatalf("the refusal must carry the superseding battle, got %#v", body.Details)
	}
}
