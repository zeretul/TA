package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// StoryPackBinding records which script pack started a story, at which content
// revision, and what that revision's world book looked like.
//
// It exists because a pack is editable content. Without a binding, editing a
// pack file or a global preset silently rewrites an in-progress story: the next
// turn would read the new world book, the new modules and the new opening as if
// they had always been there. The binding freezes the story to the revision it
// actually started from.
//
// The scope is the isolation key every lore read for this story is filtered by.
// It is derived from the pack identity rather than the story ID so that two
// stories of the same pack share one world book, and two different packs in one
// workspace never see each other's entries.
type StoryPackBinding struct {
	// PackID is the pack's stable identity.
	PackID string `json:"pack_id"`
	// PackName is the pack name as of binding time, kept for display even if the
	// pack is later renamed or removed.
	PackName string `json:"pack_name,omitempty"`
	// ContentVersion is the pack document's own version field. It is distinct
	// from the format version: it is what the author bumps when the content
	// changes meaningfully.
	ContentVersion int `json:"content_version,omitempty"`
	// ContentHash fingerprints the pack content that was actually applied, so a
	// later edit to the same declared version is still detectable.
	ContentHash string `json:"content_hash,omitempty"`
	// Scope is the lore resource scope this story reads and writes. It is
	// derived from PackID and is the value every lore lookup for this story
	// filters by.
	Scope string `json:"scope"`
	// ModuleRefs is the resolved module set at binding time. A later global
	// preset change must not silently repoint an existing story.
	ModuleRefs *StoryDirectorModuleRefs `json:"module_refs,omitempty"`
	// Gameplay is the package's frozen capability declaration. Nil is kept for
	// stories created before world-specific capabilities were recorded.
	Gameplay *StoryGameplayProfile `json:"gameplay,omitempty"`
	// BoundAt is when the story took this binding.
	BoundAt string `json:"bound_at,omitempty"`
}

// StoryGameplayProfile is the small runtime capability snapshot the application
// may enforce at API and UI boundaries. Numeric definitions remain frozen by
// the package content hash and the immutable, content-addressed module refs.
type StoryGameplayProfile struct {
	Presentation          WorldPresentation `json:"presentation,omitempty"`
	StatusTemplate        string            `json:"status_template"`
	Growth                string            `json:"growth"`
	Combat                string            `json:"combat"`
	Communication         string            `json:"communication,omitempty"`
	LetterDeliverySeconds int64             `json:"letter_delivery_seconds,omitempty"`
	Opening               string            `json:"opening"`
}

// PackScopeFromID derives the lore resource scope for a pack, ignoring content
// revision. It addresses "this pack" rather than one revision of it, so it is
// the right scope only when no revision is known: a hand-authored request that
// names a pack but no content, and bindings stored before scopes carried a
// revision.
func PackScopeFromID(packID string) string {
	packID = sanitizeScopeToken(packID)
	if packID == "" {
		return ""
	}
	return "pack:" + packID
}

// PackScopeForRevision derives the lore resource scope for one content revision
// of a pack. The revision participates in the address, so a story reads the
// world book its own revision wrote.
//
// This is what makes editing a pack in place safe. A pack's world book is
// written once, when a story starts, and entries whose names already exist are
// left untouched so a later user edit is never overwritten. Were the scope
// derived from the pack ID alone, that "leave existing names alone" rule would
// defeat the new revision: after an edit, a new game's entries would all collide
// with the previous revision's names and be skipped, so the new game would read
// the old world book. Qualifying the scope by the content fingerprint gives each
// revision its own book, so a new game reads the new content while a game
// already running keeps reading the entries its own revision wrote.
//
// An empty revision falls back to the pack-only scope, which is the correct
// address for a pack that declares no content of its own.
func PackScopeForRevision(packID, contentHash string) string {
	base := PackScopeFromID(packID)
	if base == "" {
		return ""
	}
	revision := sanitizeScopeToken(contentHash)
	if revision == "" {
		return base
	}
	return base + "@" + revision
}

// sanitizeScopeToken drops characters that cannot be part of a stable stored
// identifier, so a scope is always filesystem-safe and survives
// normalizeLoreScope unchanged.
func sanitizeScopeToken(value string) string {
	value = strings.TrimSpace(value)
	var sb strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-', r == '_', r == ':', r == '@', r == '.':
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// PackEntryIDForRevision namespaces a pack world entry ID by content revision.
//
// Lore entry IDs are unique across the whole collection, not per scope, so two
// revisions of one pack writing the same authored entry ID would collide and the
// second revision's book could not be written at all. Qualifying the ID keeps
// each revision's copy addressable, which is what lets an old game and a new
// game of the same pack hold different world books side by side.
func PackEntryIDForRevision(entryID, contentHash string) string {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return ""
	}
	revision := sanitizeScopeToken(contentHash)
	if revision == "" {
		return entryID
	}
	return entryID + "-rev-" + revision
}

// ContentHashOf fingerprints arbitrary pack content for change detection. It is
// an identity aid only: the binding's frozen module set and world book are what
// actually keep an existing story stable, never a re-read of current content.
func ContentHashOf(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

// validateStoryPackBinding rejects a binding that cannot address a world. An
// absent binding is valid and means the story belongs to no pack.
func validateStoryPackBinding(binding *StoryPackBinding) error {
	if binding == nil {
		return nil
	}
	if strings.TrimSpace(binding.PackID) == "" {
		return fmt.Errorf("故事剧本包绑定缺少 pack_id")
	}
	if strings.TrimSpace(binding.Scope) == "" {
		return fmt.Errorf("故事剧本包绑定缺少 scope: %s", binding.PackID)
	}
	if binding.Gameplay != nil {
		return binding.Gameplay.Presentation.Validate()
	}
	return nil
}

func normalizeStoryPackBinding(binding *StoryPackBinding) *StoryPackBinding {
	if binding == nil {
		return nil
	}
	normalized := &StoryPackBinding{
		PackID:         strings.TrimSpace(binding.PackID),
		PackName:       strings.TrimSpace(binding.PackName),
		ContentVersion: binding.ContentVersion,
		ContentHash:    strings.TrimSpace(binding.ContentHash),
		Scope:          strings.TrimSpace(binding.Scope),
		ModuleRefs:     cloneStoryDirectorModuleRefs(binding.ModuleRefs),
		Gameplay:       cloneStoryGameplayProfile(binding.Gameplay),
		BoundAt:        strings.TrimSpace(binding.BoundAt),
	}
	if normalized.PackID == "" && normalized.Scope == "" {
		return nil
	}
	// A binding stored without a scope predates revision-qualified scopes. Its
	// world book lives at the pack-only address, so keeping that scope is what
	// preserves the story's own book; re-deriving it would point an existing
	// story at a scope nothing was ever written to.
	if normalized.Scope == "" {
		normalized.Scope = PackScopeForRevision(normalized.PackID, normalized.ContentHash)
	}
	return normalized
}

func cloneStoryGameplayProfile(profile *StoryGameplayProfile) *StoryGameplayProfile {
	if profile == nil {
		return nil
	}
	copy := *profile
	copy.Presentation = profile.Presentation.Clone()
	return &copy
}

func cloneStoryPackBinding(binding *StoryPackBinding) *StoryPackBinding {
	return normalizeStoryPackBinding(binding)
}

// StoryPackBindingScope reports the lore scope a binding addresses, or the
// shared scope when there is no binding.
func StoryPackBindingScope(binding *StoryPackBinding) string {
	if binding == nil {
		return ""
	}
	return binding.Scope
}

// LoreScope reports the lore resource scope this story's context reads. A story
// with no pack binding is bound to the shared scope: it sees the user's own
// general entries and no pack's world book, which is what a manually created
// story has always effectively been.
func (meta StoryMeta) LoreScope() string {
	return StoryPackBindingScope(meta.PackBinding)
}
