package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// HandleInteractiveCombatWindow serves the battle window read: the scene, every
// live battle on the branch, and — once one is selected — both sides with the
// numbers the fight is actually being fought at.
//
// It is a plain read with no model call, so opening the window costs no world
// time and no generation. Everything it shows is already committed.
func (h *Handlers) HandleInteractiveCombatWindow(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = parsed
	}
	window, err := h.app.CombatWindow(novaApp.CombatWindowRequest{
		StoryID: storyID, BranchID: c.Query("branch"),
		SessionID: c.Query("session_id"), BeforeCursor: c.Query("before"),
		Limit: limit, Locale: requestLocale(c),
	})
	if err != nil {
		writeCombatWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

// HandleInteractiveCombatRound plays one exchange of a battle.
//
// The round is resolved and committed in one call, so the window never shows a
// round the journal does not have. command_id makes a retry safe: a repeated
// click, a lost response or a recovery re-read returns the exchange the first
// attempt settled rather than resolving a second one or drawing again.
//
// Closing the window has no route here at all. §七.1 requires closing to be a
// display act that neither flees nor ends the fight, and the way to keep that
// true is for the close path to have nothing to call.
func (h *Handlers) HandleInteractiveCombatRound(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID    string `json:"command_id"`
		Branch       string `json:"branch"`
		SessionID    string `json:"session_id"`
		TurnID       string `json:"turn_id"`
		PlayerIntent string `json:"player_intent"`
		// Narrate decides whether this call also writes the round's prose. The
		// default is to write it, which is the one-call path older clients use.
		// A caller that wants the settled result on screen before the passage is
		// written sends false, then asks for the prose on the narration route under
		// the same command identity — the round itself is settled either way.
		Narrate *bool `json:"narrate"`
		// Combatants opens a battle. It is required only when session_id is empty,
		// which is the round that starts the fight.
		Combatants []struct {
			ActorID     string `json:"actor_id"`
			Side        string `json:"side"`
			RouteExists bool   `json:"route_exists"`
			Retreat     string `json:"retreat"`
		} `json:"combatants"`
		// Actions is the round's declared intents, by actor ID. The player's side
		// supplies its own; an enemy left out is decided from its own state, so a
		// client cannot make an enemy act outside what it has learned.
		Actions []struct {
			ActorID  string `json:"actor_id"`
			Kind     string `json:"kind"`
			SkillID  string `json:"skill_id"`
			Variant  string `json:"variant"`
			TargetID string `json:"target_id"`
			// TargetIDs names the further opponents a technique that reaches more
			// than one target is aimed at. §9 settles the reach from the actual
			// choice, so the extra names are declared here rather than inferred
			// from the enemy line.
			TargetIDs []string `json:"target_ids"`
			OfferText string   `json:"offer_text"`
			// ItemInstanceID names which carried copy is spent. It is an instance and
			// not a definition, because §7 keeps the two apart: a character may hold two
			// copies of one pill, and the round has to say which one it used up. Nothing
			// else about the dose is accepted from the body.
			ItemInstanceID string `json:"item_instance_id"`
			// SwapInstanceID names the spare a swap_equipment action takes into its
			// position. The position is not accepted from the body: §4 maps an item's
			// real use to its row, so the server reads the row from the copy itself and
			// a caller cannot hang a blade in a treasure position by asking to.
			SwapInstanceID string `json:"swap_instance_id"`
		} `json:"actions"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// The round needs a command identity for the same reason a message does: a
	// retry must be answered from the committed record rather than re-resolved.
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	combatants, err := combatSpecsFromRequest(body.Combatants)
	if err != nil {
		writeCombatWindowError(c, err)
		return
	}
	actions, err := combatActionsFromRequest(body.Actions)
	if err != nil {
		writeCombatWindowError(c, err)
		return
	}
	window, err := h.app.PlayCombatRound(novaApp.CombatRoundRequest{
		StoryID: storyID, BranchID: body.Branch, SessionID: body.SessionID,
		Combatants: combatants, Actions: actions, PlayerIntent: interactive.PlayerCombatIntent(strings.TrimSpace(body.PlayerIntent)),
		CommandID: body.CommandID, TurnID: body.TurnID, Locale: requestLocale(c),
		SkipNarration: body.Narrate != nil && !*body.Narrate,
		Context:       ctx,
	})
	if err != nil {
		writeCombatWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, window)
}

// HandleInteractiveCombatNarration writes the prose for a round that is already
// settled.
//
// It exists because §8 keeps the round and its expression apart. A round whose
// passage failed is still a committed round, and the way to recover it is to ask
// for the missing prose against the same round record — never to play the round
// again. The round is not resolved here and cannot be: the request names a
// committed round record, and the answer is a passage about it.
func (h *Handlers) HandleInteractiveCombatNarration(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID    string `json:"command_id"`
		Branch       string `json:"branch"`
		SessionID    string `json:"session_id"`
		RoundEventID string `json:"round_event_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// The passage is written under a command identity for the same reason the
	// round is: a retry must be answered from the committed record rather than
	// rendering the same exchange twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	result, err := h.app.NarrateCombatRound(ctx, novaApp.CombatNarrationRequest{
		StoryID: storyID, BranchID: body.Branch,
		SessionID: body.SessionID, RoundEventID: body.RoundEventID,
		CommandID: body.CommandID, Locale: requestLocale(c),
	})
	if err != nil {
		writeCombatWindowError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// combatSpecsFromRequest translates the request's participants.
//
// A side the caller did not name is refused rather than defaulted: §七.1 requires
// the window to show both sides, and silently placing an unnamed combatant on the
// player's side would put a character into a fight the player did not put them in.
func combatSpecsFromRequest(specs []struct {
	ActorID     string `json:"actor_id"`
	Side        string `json:"side"`
	RouteExists bool   `json:"route_exists"`
	Retreat     string `json:"retreat"`
}) ([]interactive.CombatantSpec, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]interactive.CombatantSpec, 0, len(specs))
	for _, spec := range specs {
		actorID := strings.TrimSpace(spec.ActorID)
		if actorID == "" {
			return nil, errCombatActorRequired
		}
		side := cultivation.CombatSide(strings.TrimSpace(spec.Side))
		switch side {
		case cultivation.SidePlayer, cultivation.SideEnemy:
		default:
			return nil, errCombatSideInvalid
		}
		out = append(out, interactive.CombatantSpec{
			ActorID: actorID, Side: side,
			RouteExists: spec.RouteExists,
			Retreat:     cultivation.RetreatTemplate(strings.TrimSpace(spec.Retreat)),
		})
	}
	return out, nil
}

// combatActionsFromRequest translates the round's declared intents.
//
// A technique action is looked up in the published registry here rather than
// accepted from the body, so a client cannot invent a technique or hand itself a
// version of one that was never published. The focus requirement is deliberately
// not taken from the body either: the store fills it from the combatant's real
// equipment, which is what keeps a focus technique from being cast bare-handed.
//
// A use-item action carries an instance name and nothing more. The effect is
// priced from the catalogue at the moment it is taken, so a caller that could
// send the numbers would be able to write a 极品 dose's 30% onto a 下品 pill —
// which is the one thing §4.2's fixed budgets exist to prevent.
func combatActionsFromRequest(actions []struct {
	ActorID  string `json:"actor_id"`
	Kind     string `json:"kind"`
	SkillID  string `json:"skill_id"`
	Variant  string `json:"variant"`
	TargetID string `json:"target_id"`
	// TargetIDs names the further opponents a technique that reaches more than
	// one target is aimed at. §9 settles the reach from the actual choice, so the
	// extra names are declared here rather than inferred from the enemy line.
	TargetIDs []string `json:"target_ids"`
	OfferText string   `json:"offer_text"`
	// ItemInstanceID names which carried copy is spent. It is an instance and
	// not a definition, because §7 keeps the two apart: a character may hold two
	// copies of one pill, and the round has to say which one it used up. Nothing
	// else about the dose is accepted from the body.
	ItemInstanceID string `json:"item_instance_id"`
	// SwapInstanceID names the spare a swap_equipment action takes into its
	// position. The position is not accepted from the body: §4 maps an item's
	// real use to its row, so the server reads the row from the copy itself and
	// a caller cannot hang a blade in a treasure position by asking to.
	SwapInstanceID string `json:"swap_instance_id"`
}) (map[string]cultivation.CombatAction, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	out := make(map[string]cultivation.CombatAction, len(actions))
	for _, action := range actions {
		actorID := strings.TrimSpace(action.ActorID)
		if actorID == "" {
			return nil, errCombatActorRequired
		}
		if _, exists := out[actorID]; exists {
			return nil, errCombatActorDuplicate
		}
		kind := cultivation.CombatActionKind(strings.TrimSpace(action.Kind))
		translated := cultivation.CombatAction{
			Kind: kind, TargetID: strings.TrimSpace(action.TargetID),
			Variant:   cultivation.CombatVariant(strings.TrimSpace(action.Variant)),
			OfferText: action.OfferText,
		}
		for _, extraID := range action.TargetIDs {
			if trimmed := strings.TrimSpace(extraID); trimmed != "" {
				translated.ExtraTargetIDs = append(translated.ExtraTargetIDs, trimmed)
			}
		}
		switch kind {
		case cultivation.ActionTechnique:
			// The declaration names a technique; it does not define one. The
			// definition is resolved from the story's own registry further down the
			// call — which is where a technique this story generated is in scope —
			// and a name that resolves nowhere is refused there rather than here.
			skillID := strings.TrimSpace(action.SkillID)
			if skillID == "" {
				return nil, errCombatSkillUnknown
			}
			// A published definition is resolved immediately, because most
			// declarations name one and a clear "not published" answer is better
			// than a round that fails later. A generated name is left for the store,
			// where the story's own registry is in scope; a name that is neither is
			// refused here, because it could never resolve anywhere.
			if skill, ok := cultivation.SkillByID(skillID); ok {
				translated.Skill = skill
			} else if cultivation.IsGeneratedSkillID(skillID) {
				translated.Skill = cultivation.SkillDef{ID: skillID}
			} else {
				return nil, errCombatSkillUnknown
			}
		case cultivation.ActionUseItem:
			translated.ItemInstanceID = strings.TrimSpace(action.ItemInstanceID)
			if translated.ItemInstanceID == "" {
				return nil, errCombatItemRequired
			}
		case cultivation.ActionSwapEquipment:
			// The spare is named here and matched against what the combatant really
			// carries when the round resolves, so a name that is not in the pack is
			// refused there with the same wording a client can show. What is checked
			// here is only that a copy was named at all.
			translated.SwapInstanceID = strings.TrimSpace(action.SwapInstanceID)
			if translated.SwapInstanceID == "" {
				return nil, errCombatSpareRequired
			}
		case cultivation.ActionRest, cultivation.ActionEscape,
			cultivation.ActionSurrender, cultivation.ActionAcceptSurrender,
			cultivation.ActionContinuePreparation, cultivation.ActionCancelPreparation:
		default:
			return nil, errCombatActionUnknown
		}
		out[actorID] = translated
	}
	return out, nil
}

var (
	errCombatActorRequired  = errors.New("every combat action and participant needs an actor id")
	errCombatActorDuplicate = errors.New("a combatant may declare only one action per exchange")
	errCombatSideInvalid    = errors.New("a combatant's side must be player or enemy")
	errCombatSkillUnknown   = errors.New("the named technique is not published")
	errCombatActionUnknown  = errors.New("the named action is not one a combatant may take")
	errCombatItemRequired   = errors.New("using an item needs the carried copy to use")
	errCombatSpareRequired  = errors.New("changing equipment needs the carried spare to take up")
)

// writeCombatWindowError maps a battle window failure onto a status the UI can act
// on. An unknown battle is a 404 rather than a 400: the request was well formed,
// the fight simply is not live on this branch.
func writeCombatWindowError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrCombatUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.combatWindow.battleNotFound", "detail", err.Error())
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, novaApp.ErrCombatNoEncounter):
		// The scene has no fight in it. That is a state the player can act on — walk
		// somewhere, or meet someone — not a malformed request, so it is a conflict
		// rather than a bad request.
		writeErrorKey(c, consts.StatusConflict, "api.combatWindow.noEncounter", "detail", err.Error())
	case errors.Is(err, novaApp.ErrCombatUnavailableForWorld):
		writeErrorKey(c, consts.StatusConflict, "api.combatWindow.unavailableForWorld")
	case errors.Is(err, novaApp.ErrCombatOpeningHasActions):
		// §七.1's split, stated as a refusal: an opening shows the encounter and the
		// first exchange waits for the player's choice, so a request that does both
		// at once is not one the server can answer without settling an exchange the
		// player could not have seen the state of.
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.openingHasActions", "detail", err.Error())
	case errors.Is(err, interactive.ErrCombatPlayerActionRequired):
		// Distinct from actionRejected: nothing was refused, an action was simply
		// not chosen. The window asks for one rather than reporting a rule failure.
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.playerActionRequired", "detail", err.Error())
	case errors.Is(err, interactive.ErrCombatPlayerIntentInvalid):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.playerIntentInvalid")
	case errors.Is(err, interactive.ErrCombatTakeLifeActionRequired):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.takeLifeActionRequired")
	case errors.Is(err, interactive.ErrCombatTakeLifeTargetInvalid):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.takeLifeTargetInvalid")
	case errors.Is(err, interactive.ErrCombatCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.combatWindow.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrCombatActorAlreadyFighting),
		errors.Is(err, interactive.ErrCombatStaleBattle):
		// One person, one unfinished battle. The refusal names the battle that
		// already holds them, so the window can open it rather than leaving the
		// player with a disabled button and no way into the fight.
		key := "api.combatWindow.actorAlreadyFighting"
		if errors.Is(err, interactive.ErrCombatStaleBattle) {
			key = "api.combatWindow.staleBattle"
		}
		args := []any{"detail", err.Error()}
		var conflict *interactive.CombatActorConflict
		if errors.As(err, &conflict) && conflict.SessionID != "" {
			args = append(args, "session_id", conflict.SessionID)
		}
		writeErrorKey(c, consts.StatusConflict, key, args...)
	case errors.Is(err, interactive.ErrCombatTechniqueNotLearned):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.techniqueNotLearned", "detail", err.Error())
	case errors.Is(err, interactive.ErrCombatActionRejected):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.actionRejected", "detail", err.Error())
	case errors.Is(err, novaApp.ErrCombatRoundUnknown):
		writeErrorKey(c, consts.StatusNotFound, "api.combatWindow.roundNotFound", "detail", err.Error())
	case errors.Is(err, errCombatActorRequired),
		errors.Is(err, errCombatActorDuplicate),
		errors.Is(err, errCombatSideInvalid),
		errors.Is(err, errCombatSkillUnknown),
		errors.Is(err, errCombatActionUnknown),
		errors.Is(err, errCombatItemRequired):
		writeErrorKey(c, consts.StatusBadRequest, "api.combatWindow.invalidRound", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
