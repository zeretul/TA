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

// These tests cover the cultivation route's own contract: what it translates
// before the request reaches the rules, and which status a failure is reported
// as.
//
// The rules, the journal record and the interval's arithmetic are asserted in the
// store and app suites. What only exists here is the translation — an interval
// named in the player's own unit must reach the rules as that unit rather than as
// a number of seconds the client chose, a registered boundary must arrive as
// itself, and a refusal must arrive as a status and a sentence the interface can
// act on.

// TestCultivationConditionsKeepUnspecifiedSeparate is the boundary that decides
// whether a continuation inherits anything: §五 lets a resumed interval keep the
// conditions it was started under, so "the client said nothing" must not arrive at
// the rules as "the client asked for all defaults".
func TestCultivationConditionsKeepUnspecifiedSeparate(t *testing.T) {
	if conditions := cultivationConditions(nil); conditions != nil {
		t.Fatalf("an unspecified condition set must stay unspecified so a continuation inherits: %#v", conditions)
	}
	conditions := cultivationConditions(&cultivationConditionsBody{
		Environment: " 灵脉洞府 ", Resource: "聚灵丹", AttemptBreakthrough: " 破境 ",
		StableSite: true,
	})
	if conditions == nil {
		t.Fatal("a specified condition set must survive translation")
	}
	if conditions.Environment != "灵脉洞府" || conditions.Resource != "聚灵丹" {
		t.Fatalf("the conditions must be trimmed and carried: %#v", conditions)
	}
	if conditions.AttemptBreakthrough != "破境" || !conditions.StableSite {
		t.Fatalf("the authorisation and the site must survive translation: %#v", conditions)
	}
	// An empty body is a real request for defaults, which is a different thing from
	// a missing body, so it must arrive as an empty set rather than as nil.
	empty := cultivationConditions(&cultivationConditionsBody{})
	if empty == nil {
		t.Fatal("an explicit empty condition set must not collapse into unspecified")
	}
}

func TestCultivationNodesCarryTheRegisteredBoundaries(t *testing.T) {
	nodes := cultivationNodes([]cultivationNodeBody{
		{ID: " appointed ", AtSeconds: 432000, Kind: "player_decision", RequiresResponse: true, PublicDescription: "师父来传话"},
		{ID: "resource-1", AtSeconds: 864000, Kind: "resource_change"},
		{ID: "calc-1", AtSeconds: 1296000, Kind: "calculation"},
	})
	if len(nodes) != 3 {
		t.Fatalf("every boundary must be carried: %#v", nodes)
	}
	if nodes[0].Kind != storytime.DecisionNode || !nodes[0].RequiresResponse {
		t.Fatalf("a decision boundary must stay a decision that needs an answer: %#v", nodes[0])
	}
	if nodes[0].At != storytime.Instant(432000) {
		t.Fatalf("the boundary's instant must survive translation: %#v", nodes[0])
	}
	// The identity is trimmed so a client's stray space does not register a
	// different boundary from the one the story knows.
	if nodes[0].ID != "appointed" {
		t.Fatalf("the boundary identity must be trimmed: %q", nodes[0].ID)
	}
	if nodes[1].Kind != storytime.ResourceNode || nodes[2].Kind != storytime.CalculationNode {
		t.Fatalf("the other kinds must survive translation: %#v", nodes)
	}
	// No boundaries is the ordinary "train straight through" case rather than an
	// error.
	if mapped := cultivationNodes(nil); mapped != nil {
		t.Fatalf("no boundaries must translate to nothing: %#v", mapped)
	}
}

// TestCultivationNodesPassAnUnsupportedKindThrough keeps an unrecognised boundary
// with the rules rather than dropping it here, so the rules refuse it with their
// own diagnostic instead of planning an interval that silently ignores a boundary
// the story registered.
func TestCultivationNodesPassAnUnsupportedKindThrough(t *testing.T) {
	nodes := cultivationNodes([]cultivationNodeBody{{ID: "mystery", AtSeconds: 10, Kind: "intervention"}})
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

func TestCultivationErrorMapsOntoActableStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"no workspace", novaApp.ErrNoWorkspace, 409, "api.workspace.noWorkspace"},
		{"command reused", fmt.Errorf("%w: cmd-1", interactive.ErrCultivationCommandConflict), 409, "api.cultivation.commandConflict"},
		{"interval finished", fmt.Errorf("%w: clt_1", interactive.ErrCultivationActivityFinished), 409, "api.cultivation.activityFinished"},
		// An unavailable interval is a condition of the world, not a malformed
		// request: the player's input was fine and the answer is "not now". §五
		// reuses the character's existing cultivation qualification, so these are
		// the rest entry's own sentinels.
		{"ineligible", fmt.Errorf("%w: in_battle", cultivation.ErrRecoveryIneligible), 409, "api.cultivation.ineligible"},
		{"condition changed", cultivation.ErrRecoveryEligibilityChanged, 409, "api.cultivation.eligibilityChanged"},
		// A wound healing or a breakthrough mid-interval moves the pool maximum
		// the projection is based on. Same reasoning as the rest entry: it is a
		// condition of the body, not a malformed request.
		{"maximum changed", cultivation.ErrRecoveryMaximumChanged, 409, "api.cultivation.maximumChanged"},
		{"no maximum", cultivation.ErrRecoveryMissingMaximum, 400, "api.cultivation.maximumMissing"},
		{"unknown condition", cultivation.ErrRecoveryUnknownEligibility, 400, "api.cultivation.unknownEligibility"},
		{"no duration", fmt.Errorf("%w: 0", interactive.ErrCultivationMissingDuration), 400, "api.cultivation.durationRequired"},
		{"unknown unit", fmt.Errorf("%w: fortnight", interactive.ErrCultivationUnknownUnit), 400, "api.cultivation.unknownUnit"},
		// An activity or an actor that is not part of this branch is a 404: the
		// request was well formed and the thing simply is not here.
		{"unknown activity", fmt.Errorf("%w: clt_1", interactive.ErrCultivationUnknownActivity), 404, "api.cultivation.activityNotFound"},
		{"unknown actor", fmt.Errorf("%w: ghost", interactive.ErrCultivationUnknownActor), 404, "api.cultivation.actorNotFound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", "zh-CN")
			writeCultivationError(ctx, test.err)

			if ctx.Response.StatusCode() != test.want {
				t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), test.want)
			}
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q body=%s", test.code, test.code, ctx.Response.Body())
			}
			if body.Error == "" {
				t.Fatalf("the failure must be reported in the player's language: %s", ctx.Response.Body())
			}
		})
	}
}

// TestCultivationErrorLocalisesTheReason keeps the refusal in the player's own
// language and worded as a condition rather than as a character's misfortune.
func TestCultivationErrorLocalisesTheReason(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "当前状态无法进行普通修炼，先解决阻碍",
		"en-US": "Ordinary cultivation is not available in the current condition. Resolve what blocks it first.",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", locale)
			writeCultivationError(ctx, fmt.Errorf("%w: in_battle", cultivation.ErrRecoveryIneligible))

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

// TestCultivationErrorKeepsAForeignFailureGeneric stops an unanticipated failure
// from being reported as one of the interval's own refusals: a caller must not be
// told their training was ineligible when the real problem was something else.
func TestCultivationErrorKeepsAForeignFailureGeneric(t *testing.T) {
	ctx := app.NewContext(0)
	writeCultivationError(ctx, errors.New("disk is full"))

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
