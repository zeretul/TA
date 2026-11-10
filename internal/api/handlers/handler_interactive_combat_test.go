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
)

// These tests cover the battle route's own contract: which requests it refuses
// before they reach the rules, and which status a failure is reported as.
//
// The rules themselves and the window's contents are asserted in the app and
// store suites. What only exists here is the translation — a technique named by
// the client is looked up in the published table rather than trusted, a side
// the caller left unnamed is refused rather than defaulted, and a failure is
// mapped onto a status the interface can act on.

type combatRequestBody = struct {
	ActorID     string `json:"actor_id"`
	Side        string `json:"side"`
	RouteExists bool   `json:"route_exists"`
	Retreat     string `json:"retreat"`
}

type combatActionBody = struct {
	ActorID  string `json:"actor_id"`
	Kind     string `json:"kind"`
	SkillID  string `json:"skill_id"`
	Variant  string `json:"variant"`
	TargetID string `json:"target_id"`
	// TargetIDs names the further opponents a multi-target technique reaches.
	TargetIDs []string `json:"target_ids"`
	OfferText string   `json:"offer_text"`
	// ItemInstanceID is the only thing a use-item action may carry about the
	// dose. The effect is deliberately absent, so there is no field for a client
	// to write a stronger number into.
	ItemInstanceID string `json:"item_instance_id"`
	// SwapInstanceID names the carried spare a change-equipment action takes up.
	// Like ItemInstanceID it is an identity, not a number: the position and the
	// bonus come from the copy's own record.
	SwapInstanceID string `json:"swap_instance_id"`
}

func TestCombatSpecsRefuseAnUnnamedSide(t *testing.T) {
	// A side the caller did not name must not be defaulted: placing a character
	// on the player's side silently would put them into a fight the player did
	// not put them in.
	if _, err := combatSpecsFromRequest([]combatRequestBody{{ActorID: "protagonist"}}); !errors.Is(err, errCombatSideInvalid) {
		t.Fatalf("an unnamed side must be refused, got %v", err)
	}
	if _, err := combatSpecsFromRequest([]combatRequestBody{{ActorID: "protagonist", Side: "neutral"}}); !errors.Is(err, errCombatSideInvalid) {
		t.Fatalf("an unknown side must be refused, got %v", err)
	}
	if _, err := combatSpecsFromRequest([]combatRequestBody{{Side: string(cultivation.SidePlayer)}}); !errors.Is(err, errCombatActorRequired) {
		t.Fatalf("a participant with no actor must be refused, got %v", err)
	}
	if specs, err := combatSpecsFromRequest(nil); err != nil || specs != nil {
		t.Fatalf("a round with no participants must translate to nothing: %#v %v", specs, err)
	}
}

func TestCombatSpecsTranslateTheParticipants(t *testing.T) {
	specs, err := combatSpecsFromRequest([]combatRequestBody{
		{ActorID: " protagonist ", Side: string(cultivation.SidePlayer), RouteExists: true},
		{ActorID: "rival", Side: string(cultivation.SideEnemy), Retreat: string(cultivation.RetreatSurvival)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("both sides must be carried: %#v", specs)
	}
	// Whitespace is trimmed so a client's stray space does not become a
	// different actor.
	if specs[0].ActorID != "protagonist" || specs[0].Side != cultivation.SidePlayer {
		t.Fatalf("the player's side must be translated as declared: %#v", specs[0])
	}
	if specs[1].Retreat != cultivation.RetreatSurvival {
		t.Fatalf("the retreat template must survive translation: %#v", specs[1])
	}
}

// TestCombatActionsCarryOnlyTheCopyForAUseItem covers the one thing a use-item
// action is allowed to say about a dose: which real copy is spent.
//
// The effect is priced from the catalogue when the dose is taken, so the body
// has no field for a number and this translation has nothing to trust. §7's
// definition-versus-instance split reaches the wire as an instance name, which
// is also what lets two copies of one pill stay two separate stocks.
func TestCombatActionsCarryOnlyTheCopyForAUseItem(t *testing.T) {
	actions, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionUseItem), ItemInstanceID: " 回春丹 "},
	})
	if err != nil {
		t.Fatal(err)
	}
	translated := actions["protagonist"]
	if translated.Kind != cultivation.ActionUseItem {
		t.Fatalf("kind = %q, want a use of an item", translated.Kind)
	}
	if translated.ItemInstanceID != "回春丹" {
		t.Fatalf("instance = %q, want the named copy with whitespace trimmed", translated.ItemInstanceID)
	}
	// The definition is not accepted from the body at all. A caller that could
	// send one could point a 下品 copy at a 极品 definition's numbers, so the
	// translated action has no definition field for one to arrive in.
	if translated.Skill.ID != "" {
		t.Fatalf("a dose is not a technique: %#v", translated.Skill)
	}
}

func TestCombatActionsRefuseAUseItemWithNoCopy(t *testing.T) {
	// Without a named copy there is nothing to spend, and defaulting to "some
	// dose" would let a click consume whichever copy happened to be first.
	_, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionUseItem)},
	})
	if !errors.Is(err, errCombatItemRequired) {
		t.Fatalf("a use with no named copy must be refused, got %v", err)
	}
	_, err = combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionUseItem), ItemInstanceID: "   "},
	})
	if !errors.Is(err, errCombatItemRequired) {
		t.Fatalf("a blank copy name must be refused, got %v", err)
	}
}

func TestCombatActionsRefuseAnUnpublishedTechnique(t *testing.T) {
	// The technique is looked up in the published table rather than accepted
	// from the body, so a client cannot invent one or hand itself a version
	// that was never published.
	_, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionTechnique), SkillID: "made_up_blade", TargetID: "rival"},
	})
	if !errors.Is(err, errCombatSkillUnknown) {
		t.Fatalf("an unpublished technique must be refused, got %v", err)
	}
}

func TestCombatActionsRefuseAnUnknownActionKind(t *testing.T) {
	_, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: "full_power_assault"},
	})
	if !errors.Is(err, errCombatActionUnknown) {
		t.Fatalf("an action that is not one a combatant may take must be refused, got %v", err)
	}
	if _, err := combatActionsFromRequest([]combatActionBody{{Kind: string(cultivation.ActionRest)}}); !errors.Is(err, errCombatActorRequired) {
		t.Fatalf("an action with no actor must be refused, got %v", err)
	}
}

func TestCombatActionsTranslateExplicitPreparationIntents(t *testing.T) {
	actions, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionContinuePreparation)},
		{ActorID: "rival", Kind: string(cultivation.ActionCancelPreparation)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if actions["protagonist"].Kind != cultivation.ActionContinuePreparation || actions["rival"].Kind != cultivation.ActionCancelPreparation {
		t.Fatalf("preparation intents were not preserved: %#v", actions)
	}
}

func TestCombatActionsRejectDuplicateActorDeclarations(t *testing.T) {
	_, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionContinuePreparation)},
		{ActorID: "protagonist", Kind: string(cultivation.ActionRest)},
	})
	if !errors.Is(err, errCombatActorDuplicate) {
		t.Fatalf("one combatant cannot declare both continue and rest, got %v", err)
	}
}

func TestCombatActionsTakeTheTechniqueFromTheRegistry(t *testing.T) {
	actions, err := combatActionsFromRequest([]combatActionBody{
		{ActorID: "protagonist", Kind: string(cultivation.ActionTechnique), SkillID: "fire_bolt", TargetID: "rival"},
		{ActorID: "rival", Kind: string(cultivation.ActionSurrender), OfferText: "求饶"},
		{ActorID: "third", Kind: string(cultivation.ActionEscape)},
	})
	if err != nil {
		t.Fatal(err)
	}
	published, ok := cultivation.SkillByID("fire_bolt")
	if !ok {
		t.Fatal("the published table must contain the named technique")
	}
	// The definition is the registry's own, not a bare ID from the body, so a
	// technique cannot be re-priced by the client.
	if actions["protagonist"].Skill.Power != published.Power || actions["protagonist"].Skill.ManaFraction != published.ManaFraction {
		t.Fatalf("the technique must come from the registry: %#v", actions["protagonist"].Skill)
	}
	if actions["protagonist"].TargetID != "rival" {
		t.Fatalf("the target must be carried: %#v", actions["protagonist"])
	}
	// An offer of surrender carries its own words, and the player's answer is a
	// separate action rather than assumed.
	if actions["rival"].OfferText != "求饶" || actions["rival"].Kind != cultivation.ActionSurrender {
		t.Fatalf("the offer must be carried as declared: %#v", actions["rival"])
	}
	// A non-technique action needs no skill, and its empty definition must not
	// be mistaken for a technique.
	if actions["third"].Skill.ID != "" || actions["third"].Kind != cultivation.ActionEscape {
		t.Fatalf("an escape must carry no technique: %#v", actions["third"])
	}
	if empty, err := combatActionsFromRequest(nil); err != nil || empty != nil {
		t.Fatalf("a round with no declared actions must translate to nothing: %#v %v", empty, err)
	}
}

// TestCombatWindowErrorMapsOntoActableStatuses covers the mapping the interface
// depends on: a battle that is not there is a 404 rather than a 400, because
// the request was well formed and retrying it unchanged will not help, while a
// malformed round is a 400 the client can fix.
func TestCombatWindowErrorMapsOntoActableStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"unknown battle", fmt.Errorf("%w: cbt-1", novaApp.ErrCombatUnknown), 404, "api.combatWindow.battleNotFound"},
		// The scene has no fight in it. That is a state the player can change by
		// moving on, not a malformed request, so it is a 409.
		{"no encounter", fmt.Errorf("%w: combatWindow.encounter.noEnemy", novaApp.ErrCombatNoEncounter), 409, "api.combatWindow.noEncounter"},
		{"no workspace", novaApp.ErrNoWorkspace, 409, "api.workspace.noWorkspace"},
		{"command reused", fmt.Errorf("%w: cmd-1", interactive.ErrCombatCommandConflict), 409, "api.combatWindow.commandConflict"},
		// A technique the combatant never learned is the caller's mistake and
		// fixable by sending a legal action, so it is a 400 rather than a 404.
		{"technique not learned", fmt.Errorf("%w: rival", interactive.ErrCombatTechniqueNotLearned), 400, "api.combatWindow.techniqueNotLearned"},
		{"action rejected", fmt.Errorf("%w: actor=protagonist", interactive.ErrCombatActionRejected), 400, "api.combatWindow.actionRejected"},
		{"player intent invalid", interactive.ErrCombatPlayerIntentInvalid, 400, "api.combatWindow.playerIntentInvalid"},
		{"take-life action required", interactive.ErrCombatTakeLifeActionRequired, 400, "api.combatWindow.takeLifeActionRequired"},
		{"take-life target invalid", interactive.ErrCombatTakeLifeTargetInvalid, 400, "api.combatWindow.takeLifeTargetInvalid"},
		// Nothing was refused here: the round arrived without the player's own
		// choice. That is a different correction — pick an action — so it gets its
		// own key rather than reading as a rule failure.
		{"player action required", fmt.Errorf("%w: session=cbt-1", interactive.ErrCombatPlayerActionRequired), 400, "api.combatWindow.playerActionRequired"},
		{"opening with actions", novaApp.ErrCombatOpeningHasActions, 400, "api.combatWindow.openingHasActions"},
		{"unnamed actor", errCombatActorRequired, 400, "api.combatWindow.invalidRound"},
		{"bad side", errCombatSideInvalid, 400, "api.combatWindow.invalidRound"},
		{"unknown technique", errCombatSkillUnknown, 400, "api.combatWindow.invalidRound"},
		{"unknown action", errCombatActionUnknown, 400, "api.combatWindow.invalidRound"},
		{"use with no copy", errCombatItemRequired, 400, "api.combatWindow.invalidRound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", "zh-CN")
			writeCombatWindowError(ctx, test.err)

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
			// A localized message the player can read, rather than a bare code.
			if body.Error == "" {
				t.Fatalf("the failure must be reported in the player's language: %s", ctx.Response.Body())
			}
		})
	}
}

// TestCombatWindowErrorLocalisesTheReason keeps the message the player sees in
// their own language, which is what makes a failure locatable rather than a
// silent dead end.
func TestCombatWindowErrorLocalisesTheReason(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "该战斗不在当前分支上进行",
		"en-US": "That battle is not under way on this branch.",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", locale)
			writeCombatWindowError(ctx, fmt.Errorf("%w: cbt-1", novaApp.ErrCombatUnknown))

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

// TestCombatWindowErrorKeepsAForeignFailureGeneric is the boundary that stops a
// failure the route did not anticipate from being reported as one of the
// round's own refusals: a caller must not be told their action was invalid when
// the real problem was something else.
func TestCombatWindowErrorKeepsAForeignFailureGeneric(t *testing.T) {
	ctx := app.NewContext(0)
	writeCombatWindowError(ctx, errors.New("disk is full"))

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
