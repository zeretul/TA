package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	novaApp "ta/internal/app"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storytime"
)

// These tests cover the rest route's own contract: what it translates before the
// request reaches the rules, and which status a failure is reported as.
//
// The rules, the journal record and the entry's contents are asserted in the
// store and app suites. What only exists here is the translation — a boundary
// the story registered must reach the rules as itself rather than being dropped,
// and a refusal must arrive as a status and a sentence the interface can act on.

func TestRecoveryNodesCarryTheRegisteredBoundaries(t *testing.T) {
	nodes := recoveryNodes([]recoveryNodeBody{
		{ID: " decision-1 ", AtSeconds: 3600, Kind: "player_decision", RequiresResponse: true, PublicDescription: "有人来问话"},
		{ID: "resource-1", AtSeconds: 7200, Kind: "resource_change"},
		{ID: "calc-1", AtSeconds: 10800, Kind: "calculation"},
	})
	if len(nodes) != 3 {
		t.Fatalf("every boundary must be carried: %#v", nodes)
	}
	if nodes[0].Kind != storytime.DecisionNode || !nodes[0].RequiresResponse {
		t.Fatalf("a decision boundary must stay a decision that needs an answer: %#v", nodes[0])
	}
	if nodes[0].At != storytime.Instant(3600) {
		t.Fatalf("the boundary's instant must survive translation: %#v", nodes[0])
	}
	// The identity is trimmed so a client's stray space does not register a
	// different boundary from the one the story knows.
	if nodes[0].ID != "decision-1" {
		t.Fatalf("the boundary identity must be trimmed: %q", nodes[0].ID)
	}
	if nodes[1].Kind != storytime.ResourceNode || nodes[2].Kind != storytime.CalculationNode {
		t.Fatalf("the other kinds must survive translation: %#v", nodes)
	}
	if nodes == nil {
		t.Fatal("a non-empty request must not translate to nil")
	}
	// No boundaries is the ordinary "run straight through" case rather than an
	// error: §三.6 asks the ordinary action to proceed directly.
	if mapped := recoveryNodes(nil); mapped != nil {
		t.Fatalf("no boundaries must translate to nothing: %#v", mapped)
	}
}

// TestRecoveryNodesPassAnUnsupportedKindThrough is the boundary that matters: an
// unrecognised kind is handed to the rules rather than dropped here, so the
// rules refuse it with their own diagnostic instead of planning an interval that
// silently ignores a boundary the story registered.
func TestRecoveryNodesPassAnUnsupportedKindThrough(t *testing.T) {
	nodes := recoveryNodes([]recoveryNodeBody{{ID: "mystery", AtSeconds: 10, Kind: "intervention"}})
	if len(nodes) != 1 || nodes[0].Kind != storytime.NodeKind("intervention") {
		t.Fatalf("an unsupported kind must reach the rules as itself: %#v", nodes)
	}
	if _, err := storytime.PlanActivity(storytime.Activity{
		ID: "a", StoryID: "st_1", BranchID: "main", SourceID: "protagonist",
		Mode: storytime.EffectiveDuration, Status: storytime.ActivityRunning,
		StartedAt: 0, LastSettledAt: 0, RequiredEffective: 3600,
	}, 0, nodes); err == nil {
		t.Fatal("the rules must refuse a kind they do not define")
	}
}

func TestRecoveryErrorMapsOntoActableStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"no workspace", novaApp.ErrNoWorkspace, 409, "api.workspace.noWorkspace"},
		{"command reused", fmt.Errorf("%w: cmd-1", interactive.ErrRecoveryCommandConflict), 409, "api.recovery.commandConflict"},
		{"rest finished", fmt.Errorf("%w: rcv_1", interactive.ErrRecoveryActivityFinished), 409, "api.recovery.activityFinished"},
		// An unavailable rest is a condition of the world, not a malformed
		// request: the player's input was fine and the answer is "not now".
		{"ineligible", fmt.Errorf("%w: in_battle", cultivation.ErrRecoveryIneligible), 409, "api.recovery.ineligible"},
		{"condition changed", cultivation.ErrRecoveryEligibilityChanged, 409, "api.recovery.eligibilityChanged"},
		// A wound healing or a breakthrough mid-rest moves the pool maximum the
		// projection is based on. That is a condition of the body, not a bad
		// request, so it is a 409 the player can act on rather than the raw
		// English sentinel that used to reach them through the default branch.
		{"maximum changed", cultivation.ErrRecoveryMaximumChanged, 409, "api.recovery.maximumChanged"},
		{"no maximum", cultivation.ErrRecoveryMissingMaximum, 400, "api.recovery.maximumMissing"},
		{"unknown mode", cultivation.ErrRecoveryUnknownMode, 400, "api.recovery.unknownMode"},
		{"unknown condition", cultivation.ErrRecoveryUnknownEligibility, 400, "api.recovery.unknownEligibility"},
		{"no duration", fmt.Errorf("%w: 0", interactive.ErrRecoveryMissingDuration), 400, "api.recovery.durationRequired"},
		// A rest or an actor that is not part of this branch is a 404: the
		// request was well formed and the thing simply is not here.
		{"unknown rest", fmt.Errorf("%w: rcv_1", interactive.ErrRecoveryUnknownActivity), 404, "api.recovery.activityNotFound"},
		{"unknown actor", fmt.Errorf("%w: ghost", interactive.ErrRecoveryUnknownActor), 404, "api.recovery.actorNotFound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", "zh-CN")
			writeSafeRecoveryError(ctx, test.err)

			if ctx.Response.StatusCode() != test.want {
				t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), test.want)
			}
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q body=%s", body.Code, test.code, ctx.Response.Body())
			}
			if body.Error == "" {
				t.Fatalf("the failure must be reported in the player's language: %s", ctx.Response.Body())
			}
		})
	}
}

// TestRecoveryErrorLocalisesTheReason keeps the refusal in the player's own
// language and worded as a condition rather than as a character's misfortune:
// §三.6 asks that a rules or network failure never be narrated as the character
// losing control.
func TestRecoveryErrorLocalisesTheReason(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "当前状态无法进行普通休养或调息，先解决阻碍",
		"en-US": "Ordinary rest or meditation is not available in the current condition. Resolve what blocks it first.",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", locale)
			writeSafeRecoveryError(ctx, fmt.Errorf("%w: in_battle", cultivation.ErrRecoveryIneligible))

			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != want {
				t.Fatalf("localized error = %q, want %q", body.Error, want)
			}
		})
	}
}

// TestRecoveryErrorKeepsAForeignFailureGeneric is the boundary that stops an
// unanticipated failure from being reported as one of the rest's own refusals:
// a caller must not be told their rest was ineligible when the real problem was
// something else entirely.
func TestRecoveryErrorKeepsAForeignFailureGeneric(t *testing.T) {
	ctx := app.NewContext(0)
	writeSafeRecoveryError(ctx, errors.New("disk is full"))

	if ctx.Response.StatusCode() != 400 {
		t.Fatalf("status = %d, want 400", ctx.Response.StatusCode())
	}
	var body map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "disk is full" {
		t.Fatalf("an unanticipated failure must be reported as itself: %#v", body)
	}
}
