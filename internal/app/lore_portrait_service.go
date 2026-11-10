package app

import (
	"strings"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/portrait"
	"ta/internal/storypack"
)

// loreEntryPortraits is the two pictures one entry can show: the roster-sized
// thumbnail and the larger one for the entry's own page. They are separate
// fields because they are separate derivatives of the same shipped picture, the
// same way the character window already distinguishes an avatar from its
// display image.
type loreEntryPortraits struct {
	ThumbnailURL string
	DisplayURL   string
}

// LoreItemPortraitURLs maps the lore entries of one project to the shipped
// default portrait of the person each entry is about.
//
// The author's lore library shows the world book, and the shipped world book
// carries no pictures: 102 people and not one image field. The character window
// has always been able to draw their faces anyway, because it reaches the
// shipped portrait set by the catalog's stable author key. This is that same
// join read from the entry side, so the author sees the faces the player will.
//
// A pack that cannot be read is skipped rather than failing the read: the
// entries keep their placeholders, which is what an absent face already means.
func (a *App) LoreItemPortraitURLs(projectID string, items []booklore.Item) map[string]loreEntryPortraits {
	if a == nil || len(items) == 0 {
		return nil
	}
	portraits := a.Portraits()
	if portraits == nil {
		return nil
	}
	packs := make([]storypack.Pack, 0, 2)
	seenPack := make(map[string]bool, 2)
	for _, scope := range unresolvedLoreScopes(items) {
		packID, ok := packIDFromRevisionScope(scope)
		if !ok || seenPack[packID] {
			continue
		}
		seenPack[packID] = true
		pack, err := a.StoryPacks().GetContent(packID)
		if err != nil {
			continue
		}
		packs = append(packs, pack)
	}
	return loreEntryPortraitsByEntryID(items, packs, portraits)
}

// unresolvedLoreScopes lists the scopes worth looking up: one per entry that has
// no picture of its own. An entry the author gave a picture to is answered by
// that picture, so its world is never even read.
func unresolvedLoreScopes(items []booklore.Item) []string {
	scopes := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	for _, item := range items {
		if item.Image != nil {
			continue
		}
		scope := strings.TrimSpace(item.Scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	return scopes
}

// loreEntryPortraitsByEntryID is the join itself, kept pure so the isolation
// rules can be stated without a data root on disk.
//
// The join is revision-qualified on both sides. An entry is addressed by the
// revision-qualified ID the pack wrote, and the pack must still derive the same
// scope, which means the entry belongs to the revision now on disk. Two worlds
// that each hold a 陈巧倩 therefore get their own faces — the match is on the
// author key, never on a display name — and an entry left behind by an older
// revision of the same pack matches nothing rather than borrowing a face.
//
// A picture chosen, generated or uploaded in the lore library is the author's
// decision and always outranks the shipped default, so entries that already
// have one are not answered here at all. Entries the story invented, and entries
// the author wrote by hand, belong to no catalog and get no face: that is the
// unknown-appearance rule, and it stays exactly as it was.
func loreEntryPortraitsByEntryID(items []booklore.Item, packs []storypack.Pack, portraits *portrait.Library) map[string]loreEntryPortraits {
	if len(items) == 0 || len(packs) == 0 || portraits == nil {
		return nil
	}
	byEntryID := make(map[string]loreEntryPortraits, len(items))
	for _, pack := range packs {
		hash := pack.ContentHash()
		scope := interactive.PackScopeForRevision(pack.ID, hash)
		if scope == "" {
			continue
		}
		for loreID, person := range pack.CatalogByLoreID() {
			entryID := interactive.PackEntryIDForRevision(loreID, hash)
			if entryID == "" {
				continue
			}
			thumbnail := portraits.WorldURL(pack.ID, person.SourceKey, false)
			display := portraits.WorldURL(pack.ID, person.SourceKey, true)
			if thumbnail == "" && display == "" {
				continue
			}
			byEntryID[entryID] = loreEntryPortraits{ThumbnailURL: thumbnail, DisplayURL: display}
		}
	}
	if len(byEntryID) == 0 {
		return nil
	}
	out := make(map[string]loreEntryPortraits, len(byEntryID))
	for _, item := range items {
		if item.Image != nil {
			continue
		}
		if entry, ok := byEntryID[item.ID]; ok {
			out[item.ID] = entry
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// packIDFromRevisionScope reads the pack identity out of a revision-qualified
// world scope. A scope with no revision names a pack rather than one revision of
// it, and carries no catalog to join against, so it is not answered here.
//
// The split takes the last "@" because a pack ID may itself contain one, while
// PackScopeForRevision always appends the revision at the end.
func packIDFromRevisionScope(scope string) (string, bool) {
	const prefix = "pack:"
	if !strings.HasPrefix(scope, prefix) {
		return "", false
	}
	rest := scope[len(prefix):]
	at := strings.LastIndex(rest, "@")
	if at <= 0 || at == len(rest)-1 {
		return "", false
	}
	return rest[:at], true
}
