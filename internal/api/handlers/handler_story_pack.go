package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	appsvc "ta/internal/app"
	"ta/internal/storypack"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleStoryPacks(ctx context.Context, c *app.RequestContext) {
	packs, err := h.app.StoryPacks().ListLocale(requestLocale(c))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"packs": packs})
}

// HandleStoryPack returns one pack for the editor page. The world book and the
// character catalog are translated for display only: the ids, the canonical
// values and the numbers the rules read stay exactly as the pack declares them,
// so a page shown in English cannot start a story in a different world.
func (h *Handlers) HandleStoryPack(ctx context.Context, c *app.RequestContext) {
	pack, err := h.app.StoryPacks().Get(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	locale := requestLocale(c)
	if locale != "" {
		pack = pack.Localize(locale)
	}
	writeJSON(c, consts.StatusOK, pack)
}

// HandleProjectStoryPackApply writes a pack's world book into the scoped
// Project and returns the story request for the caller to submit through the
// unchanged story-creation endpoint.
func (h *Handlers) HandleProjectStoryPackApply(ctx context.Context, c *app.RequestContext) {
	if c.Param("id") == storypack.HistoricalSanguoPackID {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	draft, err := h.app.StoryPacks().Apply(ctx, scope.ProjectID, c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, draft)
}

// HandleStoryPackStart creates a Project for a pack and the story it starts, in
// one request: the pack binding is only accepted at story creation, and the
// protagonist and modules resolve against the Project that is already open.
//
// The body is optional and carries only which ready-made character to play. It
// never carries that character's state: the server reads the template from the
// pack file.
func (h *Handlers) HandleStoryPackStart(ctx context.Context, c *app.RequestContext) {
	if c.Param("id") == storypack.HistoricalSanguoPackID {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	var body appsvc.StoryPackStartRequest
	// An empty body means "start the pack's own protagonist", so a decode
	// failure on an absent body is not an error, but a malformed one is.
	if raw := c.Request.Body(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
			return
		}
	}
	var identity *storypack.IdentityChoice
	if body.Identity != nil {
		identity = body.Identity
		identity.Locale = requestLocale(c)
	}
	started, err := h.app.StoryPacks().PrepareStartIdentity(ctx, c.Param("id"), body.TemplateID, identity)
	if err != nil {
		writeStoryPackError(c, consts.StatusBadRequest, err)
		return
	}
	writeJSON(c, consts.StatusOK, started)
}

// HandleStoryPackIdentityDraft resolves the player's creation choices and has
// the model draft the fields they left blank. Nothing is created: no Project, no
// story, no journal write. The page may call it again to modify or rewrite.
func (h *Handlers) HandleStoryPackIdentityDraft(ctx context.Context, c *app.RequestContext) {
	if c.Param("id") == storypack.HistoricalSanguoPackID {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	var body appsvc.StoryPackIdentityDraftRequest
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
		return
	}
	body.Locale = requestLocale(c)
	draft, err := h.app.StoryPacks().DraftStoryPackIdentity(ctx, c.Param("id"), body)
	if err != nil {
		message := storypack.IdentityErrorText(err, requestLocale(c))
		if strings.TrimSpace(draft.Identity.Name) == "" {
			// Nothing was resolved, so there is no draft to keep: the choices
			// themselves are the problem and the page must show it.
			writeError(c, consts.StatusBadRequest, message)
			return
		}
		// The choices resolved and the prose did not. The page keeps the rolled
		// age and the fixed choices, and shows the failure on top of them.
		draft.Error = message
		writeJSON(c, consts.StatusOK, draft)
		return
	}
	writeJSON(c, consts.StatusOK, draft)
}

// HandleStoryPackSettingsSave stores the player's new-game defaults for a world.
//
// The body is the complete settings block the page shows, plus the revision it
// loaded. A whole block rather than a patch is what lets a player deliberately
// set a value back to the application default without it reading as "unset".
func (h *Handlers) HandleStoryPackSettingsSave(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Settings storypack.PackSettings `json:"settings"`
		Revision string                 `json:"revision"`
	}
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
		return
	}
	result, err := h.app.StoryPacks().SaveSettings(ctx, c.Param("id"), body.Settings, strings.TrimSpace(body.Revision))
	if err != nil {
		if storypack.IsSettingsRevisionConflict(err) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeStoryPackError(c, consts.StatusBadRequest, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleStoryPackWorldDraft has the model draft a world's premise, places and
// people. Nothing is created: no Project, no story, no file. The page may call
// it again to edit, switch entry point, or regenerate.
//
// A draft that could not finish still returns 200 with the player's own edits
// and an `error` field, because the page keeps what they wrote. Only a request
// with no seed at all is refused.
func (h *Handlers) HandleStoryPackWorldDraft(ctx context.Context, c *app.RequestContext) {
	var body appsvc.StoryPackWorldDraftRequest
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
		return
	}
	body.Locale = requestLocale(c)
	draft, err := h.app.StoryPacks().DraftWorld(ctx, body)
	if err != nil {
		writeStoryPackError(c, consts.StatusBadRequest, err)
		return
	}
	writeJSON(c, consts.StatusOK, draft)
}

// HandleStoryPackWorldPreview re-checks an edited draft without saving it.
//
// The preview page lets a player rewrite prose between drafting and saving.
// That edit must be checked by the same three rules a save applies, so the page
// can say "still not startable, because…" instead of offering a start that the
// save would refuse. Nothing is written.
func (h *Handlers) HandleStoryPackWorldPreview(ctx context.Context, c *app.RequestContext) {
	var draft storypack.WorldDraft
	if err := decodeStrictJSONRequest(c.Request.Body(), &draft); err != nil {
		writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
		return
	}
	preview, err := h.app.StoryPacks().PreviewWorldDraft(draft)
	if err != nil {
		if writeWorldGameplayError(c, err) {
			return
		}
		writeStoryPackError(c, consts.StatusBadRequest, err)
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

// HandleStoryPackWorldSave writes an edited draft as a new user world.
//
// The body is the draft itself, because that is what the page holds: the player
// may edit any prose field between drafting and saving, and the server rebuilds
// the mechanical half from a fixed template rather than trusting the client
// with it.
func (h *Handlers) HandleStoryPackWorldSave(ctx context.Context, c *app.RequestContext) {
	var draft storypack.WorldDraft
	if err := decodeStrictJSONRequest(c.Request.Body(), &draft); err != nil {
		writeError(c, consts.StatusBadRequest, "请求体格式无效: "+err.Error())
		return
	}
	result, err := h.app.StoryPacks().SaveWorldDraft(ctx, draft)
	if err != nil {
		if writeWorldGameplayError(c, err) {
			return
		}
		writeStoryPackError(c, consts.StatusBadRequest, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// writeStoryPackError writes a pack refusal in the page's own language.
//
// The pack's validation sentences are keyed rather than finished text, so this
// is the one place that decides which language the player reads them in. A
// failure the pack did not write is passed through unchanged.
func writeStoryPackError(c *app.RequestContext, code int, err error) {
	writeError(c, code, storypack.IdentityErrorText(err, requestLocale(c)))
}

func writeWorldGameplayError(c *app.RequestContext, err error) bool {
	var validationErr *storypack.WorldGameplayValidationError
	if !errors.As(err, &validationErr) {
		return false
	}
	if validationErr.Field != "" {
		writeErrorKey(c, consts.StatusBadRequest, validationErr.Key, "field", validationErr.Field)
	} else {
		writeErrorKey(c, consts.StatusBadRequest, validationErr.Key)
	}
	return true
}

// HandleStoryPackTemplatePreview returns one ready-made character in full, so
// the setup panel can prefill the editable name, profile and opening without
// shipping every template in the pack list.
func (h *Handlers) HandleStoryPackTemplatePreview(ctx context.Context, c *app.RequestContext) {
	if c.Param("id") == storypack.HistoricalSanguoPackID {
		c.SetStatusCode(consts.StatusNotFound)
		return
	}
	pack, err := h.app.StoryPacks().Get(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	template, ok := pack.Localize(requestLocale(c)).ProtagonistTemplate(c.Param("template_id"))
	if !ok {
		writeError(c, consts.StatusNotFound, storypack.IdentityErrorText(
			&storypack.IdentityError{Key: "api.storyPack.templateMissing", Args: []any{"template", c.Param("template_id")}},
			requestLocale(c)))
		return
	}
	// The state map is not returned. It is a rule outcome the client has no use
	// for and no business editing; the panel prefills only what a player may
	// change.
	writeJSON(c, consts.StatusOK, map[string]any{
		"id":             template.ID,
		"name":           template.Name,
		"description":    template.Description,
		"character_name": template.CharacterName,
		"profile":        template.Profile,
		"origin":         template.Origin,
		"opening_text":   template.OpeningText,
	})
}
