package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "ta/internal/app"
	"ta/internal/exploration"
	"ta/internal/interactive"
)

// HandleInteractiveRealmState serves the realm entry's read: the realms this
// branch has entered and the authored realms it could still enter.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (h *Handlers) HandleInteractiveRealmState(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	state, err := h.app.RealmStateForLocale(storyID, c.Query("branch"), requestLocale(c))
	if err != nil {
		writeRealmError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

// HandleInteractiveRealm settles one realm action and commits it.
//
// The rules run before any prose: the room, the reward and the item write commit
// in one transaction, so the entry never shows a realm the journal does not have.
// command_id makes a retry safe: a repeated click, a lost response or a reopened
// window returns the action the first attempt committed rather than handing the
// same reward out twice.
func (h *Handlers) HandleInteractiveRealm(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Param("id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	var body realmActionBody
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	// A realm action is a committed fact, so it needs a command identity: without
	// one a retried request would take the same reward twice.
	if strings.TrimSpace(body.CommandID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.realm.commandIDRequired")
		return
	}
	if strings.TrimSpace(body.Action) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.realm.actionRequired")
		return
	}
	result, err := h.app.ApplyRealm(body.request(storyID, requestLocale(c)))
	if err != nil {
		writeRealmError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// The wire carries authored identities only. Capabilities, reward quantities and
// elapsed time are computed by the Story consumer, never accepted from a client.
type realmActionBody struct {
	CommandID         string `json:"command_id"`
	Branch            string `json:"branch"`
	WorldTime         string `json:"world_time"`
	RealmID           string `json:"realm_id"`
	InstanceID        string `json:"instance_id"`
	Action            string `json:"action"`
	ApproachID        string `json:"approach_id"`
	ConnectionID      string `json:"connection_id"`
	DestinationAreaID string `json:"destination_area_id"`
	ContentRefID      string `json:"content_ref_id"`
}

func (body realmActionBody) request(storyID, locale string) novaApp.RealmAppRequest {
	return novaApp.RealmAppRequest{
		StoryID: storyID, BranchID: body.Branch, WorldTime: body.WorldTime, Locale: locale,
		RealmID: body.RealmID, InstanceID: body.InstanceID, Action: body.Action,
		ApproachID:   body.ApproachID,
		ConnectionID: body.ConnectionID, DestinationAreaID: body.DestinationAreaID,
		ContentRefID: body.ContentRefID, CommandID: body.CommandID,
	}
}

// writeRealmError maps a realm failure onto a status the UI can act on.
//
// A refusal is not a technical error: the player asked to take a reward that was
// already taken, or to move through a door that does not exist. That is a world
// fact the entry should say plainly rather than report as a generic failure.
func writeRealmError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, novaApp.ErrNoWorkspace):
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	case errors.Is(err, interactive.ErrRealmCommandConflict):
		writeErrorKey(c, consts.StatusConflict, "api.realm.commandConflict", "detail", err.Error())
	case errors.Is(err, interactive.ErrRealmNotPresent):
		writeErrorKey(c, consts.StatusConflict, "api.realm.notPresent", "detail", err.Error())
	case errors.Is(err, interactive.ErrRealmUnknownAction):
		writeErrorKey(c, consts.StatusBadRequest, "api.realm.actionUnsupported", "detail", err.Error())
	case errors.Is(err, interactive.ErrRealmUnknownRealm):
		writeErrorKey(c, consts.StatusNotFound, "api.realm.realmNotFound", "detail", err.Error())
	case errors.Is(err, exploration.ErrInstanceConflict):
		// Already granted, or a conflicting fact. This is the "already taken"
		// answer the entry shows rather than a failure.
		writeErrorKey(c, consts.StatusConflict, "api.realm.alreadyTaken", "detail", err.Error())
	case errors.Is(err, exploration.ErrInvalidInstanceAction), errors.Is(err, exploration.ErrActionNotAllowed), errors.Is(err, exploration.ErrActionBlocked):
		writeErrorKey(c, consts.StatusConflict, "api.realm.actionRefused", "detail", err.Error())
	case errors.Is(err, exploration.ErrStaleInstanceRevision):
		writeErrorKey(c, consts.StatusConflict, "api.realm.staleInstance", "detail", err.Error())
	default:
		writeError(c, consts.StatusBadRequest, err.Error())
	}
}
