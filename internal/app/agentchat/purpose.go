package agentchat

import (
	"context"
	"encoding/json"
	"strings"

	agents "ta/internal/agents"
	"ta/internal/agents/configresource"
	agentruntime "ta/internal/agents/runtime"
	"ta/internal/agents/session"
	producttools "ta/internal/agents/tools"
	"ta/internal/interactive"
)

// Purpose names the product surface that opened a project Agent conversation.
// The caller names the surface; the server derives which world and which
// configuration resources that surface may touch. It is a narrowing decision,
// never a grant: an absent or unknown purpose keeps the ordinary editor
// surface, and no purpose can enable a capability project settings disabled.
type Purpose string

const (
	// PurposeGameSettings is the Game settings assistant. It maintains the
	// content of one world and one story; it is not a process-wide editor.
	PurposeGameSettings Purpose = "game-settings"
	// PurposeLore is the same assistant reached from the lore library, where the
	// entry being edited belongs to a world rather than to the story in play.
	PurposeLore Purpose = "lore"
)

// gameAssistantResources are the configuration resources a game-scoped
// assistant keeps: the world's and the story's own content. Skills and Agent
// Profiles are process-wide capability configuration, so they are absent from
// the registry rather than merely discouraged in the prompt.
var gameAssistantResources = []string{
	"style_reference",
	"narrative_style",
	"game_planning",
	"event_package",
	"rule_system",
	"state_system",
	"image_preset",
}

// purposeCapability persists the surface on the conversation. RuntimeBinding
// carries no story identity for project sessions, so the session key cannot
// express the purpose; without this record a steered, queued, compacted or
// recovered cycle would be rebuilt with the wider editor surface.
const purposeCapability = "agentchat.purpose"

type purposeState struct {
	Purpose Purpose `json:"purpose"`
	// StoryID is the story the surface was opened to edit. Empty means the
	// conversation follows the project's currently selected story.
	StoryID string `json:"story_id,omitempty"`
	// WorldID is the world those settings belong to, resolved by the server from
	// the story's own pack binding when the purpose was recorded. Persisting it
	// keeps a follow-up cycle writing the same world even if the story is later
	// repointed or the project's selection changes.
	WorldID string `json:"world_id,omitempty"`
}

// ParsePurpose maps one transport value onto a known surface. An unrecognised
// value is the zero Purpose, which is the ordinary editor surface.
func ParsePurpose(value string) Purpose {
	switch Purpose(strings.TrimSpace(value)) {
	case PurposeGameSettings:
		return PurposeGameSettings
	case PurposeLore:
		return PurposeLore
	}
	return ""
}

// conversationToolScope is the single place the tool surface of a conversation
// is decided.
func (service *Service) conversationToolScope(ctx context.Context, binding Binding, sess *session.Session, policy TurnPolicy) (*producttools.IDEContext, error) {
	state, err := service.resolvePurpose(ctx, binding, sess, policy)
	if err != nil {
		return nil, err
	}
	return purposeToolScope(binding.Workspace, state, service.settingsWriter()), nil
}

// settingsWriter is the host's persistence for the two bound settings records.
// A service without a host — a unit test, or a host that owns no story records —
// has none, which makes a scoped conversation's configuration read-only rather
// than falling back to the process-wide preset libraries.
func (service *Service) settingsWriter() configresource.ScopedSettingsWriter {
	if service == nil || service.host == nil {
		return nil
	}
	return service.host.ScopedSettingsWriter()
}

// applyToolScope narrows the host's product tools to this conversation's
// purpose. Every path that builds a project Agent calls it — the turn, the
// resumed cycle, compaction, a delegated child and external recovery — so a
// follow-up cycle cannot widen what the first turn was narrowed to.
func (service *Service) applyToolScope(ctx context.Context, binding Binding, sess *session.Session, policy TurnPolicy, host *agents.AgentHostCapabilities) error {
	if host == nil {
		return nil
	}
	scope, err := service.conversationToolScope(ctx, binding, sess, policy)
	if err != nil {
		return err
	}
	host.ToolScope = scope
	return nil
}

// resolvePurpose prefers the surface named by this turn and records it, so the
// follow-up cycles of the same conversation inherit it. A turn that names no
// surface reads the recorded one instead.
func (service *Service) resolvePurpose(ctx context.Context, binding Binding, sess *session.Session, policy TurnPolicy) (purposeState, error) {
	requested := purposeState{Purpose: policy.Purpose, StoryID: strings.TrimSpace(policy.StoryID)}
	if requested.Purpose == "" {
		return service.recordedPurpose(ctx, binding, sess), nil
	}
	// The world is resolved here, once, from the story's own pack binding. A
	// follow-up cycle then writes the world this conversation was opened for
	// even if the story is repointed or the project's selection changes.
	requested.WorldID = storyPackID(binding.Workspace, requested.StoryID)
	if err := service.recordPurpose(ctx, binding, sess, requested); err != nil {
		return requested, err
	}
	return requested, nil
}

func (service *Service) recordPurpose(ctx context.Context, binding Binding, sess *session.Session, state purposeState) error {
	store, err := agentruntime.SessionState(runtimeOptions(binding, ""), sess)
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return store.Update(ctx, purposeCapability, func(current json.RawMessage, present bool) (json.RawMessage, error) {
		if present && string(current) == string(data) {
			return nil, nil
		}
		return data, nil
	})
}

func (service *Service) recordedPurpose(ctx context.Context, binding Binding, sess *session.Session) purposeState {
	if sess == nil {
		return purposeState{}
	}
	store, err := agentruntime.SessionState(runtimeOptions(binding, ""), sess)
	if err != nil {
		return purposeState{}
	}
	raw, found, err := store.Read(ctx, purposeCapability)
	if err != nil || !found {
		return purposeState{}
	}
	var state purposeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return purposeState{}
	}
	return state
}

// purposeToolScope turns a surface into the world binding, the resource subset
// and the write target the product tools enforce. A surface that is not a game
// assistant keeps the editor surface by returning nil.
//
// The target carries the identifiers the server resolved, never a value read
// from message text. An empty identifier is a refusal to write, not a wildcard
// over the shared libraries: the writer is present but there is nothing it is
// allowed to name.
func purposeToolScope(workspace string, state purposeState, writer configresource.ScopedSettingsWriter) *producttools.IDEContext {
	switch state.Purpose {
	case PurposeGameSettings:
		scope := storyLoreScope(workspace, state.StoryID)
		return &producttools.IDEContext{
			LoreScope:       &scope,
			ConfigResources: append([]string(nil), gameAssistantResources...),
			SettingsTarget:  settingsTarget(state, writer),
		}
	case PurposeLore:
		// The lore library edits entries of whichever world they belong to, and
		// it is reachable with no story in play. A surface that named a story
		// binds to that story's world; one that named none falls back to the
		// project's current selection, which is the story the library is showing.
		// With neither there is no world to protect, so the scope stays unbound
		// and the handed-over entry is still reachable.
		storyID := strings.TrimSpace(state.StoryID)
		if storyID == "" {
			storyID = currentStoryID(workspace)
		}
		if storyID != "" {
			scope := storyLoreScope(workspace, storyID)
			return &producttools.IDEContext{
				LoreScope:       &scope,
				ConfigResources: append([]string(nil), gameAssistantResources...),
				SettingsTarget:  settingsTarget(purposeState{StoryID: storyID, WorldID: storyPackID(workspace, storyID)}, writer),
			}
		}
		return &producttools.IDEContext{
			ConfigResources: append([]string(nil), gameAssistantResources...),
			SettingsTarget:  settingsTarget(purposeState{}, writer),
		}
	default:
		return nil
	}
}

// settingsTarget is present for every game-assistant surface, including one
// with nothing bound. Its presence is what makes the shared preset libraries
// read-only; an empty target therefore narrows further than a populated one,
// never less.
func settingsTarget(state purposeState, writer configresource.ScopedSettingsWriter) *configresource.ScopedTarget {
	return &configresource.ScopedTarget{
		StoryID: strings.TrimSpace(state.StoryID),
		WorldID: strings.TrimSpace(state.WorldID),
		Writer:  writer,
	}
}

// storyPackID resolves the world a story is bound to. It is the same identifier
// the world page saves defaults under, so an assistant editing "this world"
// writes the record the page shows.
func storyPackID(workspace, storyID string) string {
	storyID = strings.TrimSpace(storyID)
	if storyID == "" {
		return ""
	}
	index, err := interactive.NewStore(workspace).Index()
	if err != nil {
		return ""
	}
	for _, story := range index.Stories {
		if story.ID == storyID {
			if story.PackBinding == nil {
				return ""
			}
			return strings.TrimSpace(story.PackBinding.PackID)
		}
	}
	return ""
}

// storyLoreScope resolves the lore scope of one story of this project. An
// unknown story, or a story with no pack binding, resolves to the shared scope:
// the assistant then sees the user's own entries and no world's book, which is
// a refusal to reach a world rather than a reach into one.
func storyLoreScope(workspace, storyID string) string {
	storyID = strings.TrimSpace(storyID)
	if storyID == "" {
		return ""
	}
	index, err := interactive.NewStore(workspace).Index()
	if err != nil {
		return ""
	}
	for _, story := range index.Stories {
		if story.ID == storyID {
			return interactive.StoryPackBindingScope(story.PackBinding)
		}
	}
	return ""
}

func currentStoryID(workspace string) string {
	index, err := interactive.NewStore(workspace).Index()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(index.CurrentStoryID)
}
