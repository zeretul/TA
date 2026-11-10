package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storytime"
)

// HandleInteractiveCultivationState serves the cultivation entry's read: the
// branch's formal instant, the character's live stage and pools, and whether
// ordinary cultivation is available.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. §五 has the player name a duration and the rules decide the
// outcome, so nothing here may advance anything: everything it reports is already
// committed.
func (h *Handlers) HandleInteractiveCultivationState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.CultivationStateFor(storyID, c.Query("branch"), c.Query("actor_id"))
	if err != nil {
		writeCultivationError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractiveCultivation settles one player-named cultivation interval and
// commits it.
//
// The rules run before any prose: the stage reached, the progress banked and the
// in-fiction time the interval consumed commit in one transaction, so the entry
// never shows an advance the journal does not have. §五 requires the training
// result and the formal time to agree, which is what one transaction buys.
//
// command_id makes a retry safe: a repeated click, a lost response or a reopened
// window returns the interval the first attempt committed rather than training
// the character a second time.
func (h *Handlers) HandleInteractiveCultivation(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body struct {
		CommandID string `json:"command_id"`
		Branch    string `json:"branch"`
		ActorID   string `json:"actor_id"`
		// Value and Unit name the interval in the player's own terms. §五 permits
		// hours, days, rule months and rule years; the unit is converted by the
		// rules rather than by the client, so a client cannot invent a length.
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
		// Conditions are the real training conditions: the place, what supports the
		// practice, and whether the player authorised a major breakthrough attempt.
		Conditions *cultivationConditionsBody `json:"conditions"`
		// ActivityID continues an interval a real node stopped. Empty starts a new one.
		ActivityID string `json:"activity_id"`
		// DeclaredEligibility reports a condition the store cannot derive, such as a
		// creator-marked seal. It can only make the finding stricter.
		DeclaredEligibility string `json:"declared_eligibility"`
		// Nodes are the registered story boundaries this interval must stop at.
		Nodes []cultivationNodeBody `json:"nodes"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// An interval is a committed fact, so it needs a command identity: without one
	// a retried request would train the character twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.commandIDRequired")
		return
	}
	if strings.TrimSpace(body.Unit) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.unitRequired")
		return
	}
	result, err := h.app.ApplyCultivationInterval(novaApp.CultivationAppRequest{
		StoryID: storyID, BranchID: body.Branch, ActorID: body.ActorID,
		Value: body.Value, Unit: body.Unit,
		Conditions:          cultivationConditions(body.Conditions),
		ActivityID:          body.ActivityID,
		Nodes:               cultivationNodes(body.Nodes),
		DeclaredEligibility: body.DeclaredEligibility,
		CommandID:           body.CommandID,
	})
	if err != nil {
		writeCultivationError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// cultivationConditionsBody is the training conditions in a request.
type cultivationConditionsBody struct {
	Environment string `json:"environment"`
	Resource    string `json:"resource"`
	// AttemptBreakthrough carries the player's explicit authorisation for a major
	// breakthrough. §五 forbids a timed interval from attempting one on its own,
	// so this is the only way the attempt can happen and its absence is the
	// ordinary case rather than an error.
	AttemptBreakthrough string `json:"attempt_breakthrough"`
	AuxiliaryMedicine   bool   `json:"auxiliary_medicine"`
	StableSite          bool   `json:"stable_site"`
	QualifiedGuardian   bool   `json:"qualified_guardian"`
}

// cultivationConditions maps the request's conditions onto the rules' own shape.
//
// A nil body stays nil rather than becoming a zero-valued set: §五 lets a
// continuation inherit the conditions its activity was started under, and
// "unspecified" and "all defaults" are different requests.
func cultivationConditions(body *cultivationConditionsBody) *interactive.CultivationConditions {
	if body == nil {
		return nil
	}
	return &interactive.CultivationConditions{
		Environment: strings.TrimSpace(body.Environment), Resource: strings.TrimSpace(body.Resource),
		AttemptBreakthrough: strings.TrimSpace(body.AttemptBreakthrough),
		AuxiliaryMedicine:   body.AuxiliaryMedicine, StableSite: body.StableSite,
		QualifiedGuardian: body.QualifiedGuardian,
	}
}

// cultivationNodeBody is one registered story boundary in a request.
//
// The interval has to stop where the story really needs the player, so the
// boundaries come from the caller and are validated by the storytime rules rather
// than invented here.
type cultivationNodeBody struct {
	ID                string `json:"id"`
	AtSeconds         int64  `json:"at_seconds"`
	Kind              string `json:"kind"`
	RequiresResponse  bool   `json:"requires_response"`
	Resolved          bool   `json:"resolved"`
	PublicDescription string `json:"public_description"`
}

// cultivationNodes maps the request's boundaries onto the rules' own shape. An
// unsupported kind is passed through rather than dropped, so the rules refuse it
// with their own diagnostic instead of silently planning an interval that ignores
// a boundary the story registered.
func cultivationNodes(nodes []cultivationNodeBody) []storytime.Node {
	if len(nodes) == 0 {
		return nil
	}
	mapped := make([]storytime.Node, 0, len(nodes))
	for _, node := range nodes {
		mapped = append(mapped, storytime.Node{
			ID: strings.TrimSpace(node.ID), At: storytime.Instant(node.AtSeconds),
			Kind: storytime.NodeKind(strings.TrimSpace(node.Kind)),
			RequiresResponse: node.RequiresResponse, Resolved: node.Resolved,
			PublicDescription: node.PublicDescription,
		})
	}
	return mapped
}

// writeCultivationError maps a cultivation failure onto a status the UI can act
// on.
//
// A refusal is not a technical error: §三.6 asks for clear wording that does not
// turn a network or rules failure into the character's misfortune, so an
// ineligible interval is reported as the world condition that blocks it rather
// than as a generic failure. The eligibility sentinels are the rest entry's own,
// because §五 reuses the character's existing cultivation qualification.
func writeCultivationError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrCultivationCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.cultivation.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrCultivationActivityFinished):
		writeErrorKey(c, consts.StatusConflict, "api.cultivation.activityFinished")
	case errors.Is(err, cultivation.ErrRecoveryIneligible):
		writeErrorKey(c, consts.StatusConflict, "api.cultivation.ineligible")
	case errors.Is(err, cultivation.ErrRecoveryEligibilityChanged):
		writeErrorKey(c, consts.StatusConflict, "api.cultivation.eligibilityChanged")
	// A pool maximum can move under a running interval when a wound heals or a
	// breakthrough raises it. The interval stops there rather than projecting from
	// a maximum the body no longer has, and the player is told to start a new one.
	case errors.Is(err, cultivation.ErrRecoveryMaximumChanged):
		writeErrorKey(c, consts.StatusConflict, "api.cultivation.maximumChanged")
	case errors.Is(err, cultivation.ErrRecoveryMissingMaximum):
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.maximumMissing")
	case errors.Is(err, cultivation.ErrRecoveryUnknownEligibility):
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.unknownEligibility")
	case errors.Is(err, interactive.ErrCultivationMissingDuration):
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.durationRequired")
	case errors.Is(err, interactive.ErrCultivationUnknownUnit):
		writeErrorKey(c, consts.StatusBadRequest, "api.cultivation.unknownUnit")
	case errors.Is(err, interactive.ErrCultivationUnknownActivity):
		writeErrorKey(c, consts.StatusNotFound, "api.cultivation.activityNotFound")
	case errors.Is(err, interactive.ErrCultivationUnknownActor):
		writeErrorKey(c, consts.StatusNotFound, "api.cultivation.actorNotFound")
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
