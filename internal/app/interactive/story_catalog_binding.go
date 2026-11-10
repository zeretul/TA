// This file binds a first appearance to the author's own source key.
//
// §7.1 of the discovery design requires the source-key-to-actor mapping to not
// depend on the model minting an id: the model was told to copy the printed key,
// and in a real run it read the page, used the catalog's name, and still
// submitted no key at all. The prose was right and the identity was lost, which
// is what lets the same preset person be cast twice.
//
// The binding is deliberately narrow. It resolves a first appearance only against
// the page this same turn printed, only for a person the page marked as being at
// this place, and only when that name identifies exactly one such person. A name
// is not an identity key — two packs may ship a 陈巧倩 and they are different
// people — so nothing here consults a global name index, and an ambiguous name is
// left unbound rather than guessed at. That keeps the design's rule that two
// people are never merged for sharing a name, a place or a job.
package interactiveapp

import (
	"regexp"
	"strings"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// offeredCard matches one card's heading, which is the only place the page prints
// a person's name and author key together.
var offeredCard = regexp.MustCompile(`(?m)^### (.+?) \(source_key: (.+?)\)$`)

// normalizePrintedName collapses a name to the form the page and the submitted
// profile can be compared in: case-folded, with all whitespace removed.
func normalizePrintedName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), ""))
}

// recordOfferedPeople captures the people this turn's discovery page actually
// printed, so a first appearance can be bound to the author's own key.
//
// It reads the rendered block rather than the candidate list on purpose. The
// renderer drops whole cards that do not fit its byte budget, and binding a
// person the model was never shown would invent a correspondence the turn never
// established.
func (c *Conversation) recordOfferedPeople(candidates []storypack.Candidate, block string) {
	if c == nil {
		return
	}
	// Which printed names the page placed at this scene, and which key each one
	// carries. Only a person offered as being here may be bound: one offered as
	// being in another country is a lead, and attaching their key would present a
	// teleport as a correct identity.
	//
	// A name is a usable handle only while it identifies exactly one local person.
	// The page prints the author's own name and their verified aliases, so two
	// people who share either make the name ambiguous and it is dropped rather
	// than resolved by guess — the design forbids merging people for sharing a
	// name, and guessing is the same error with one fewer witness.
	keyByName := make(map[string]string)
	ambiguous := make(map[string]bool)
	claim := func(name, key string) {
		name = normalizePrintedName(name)
		if name == "" || key == "" || ambiguous[name] {
			return
		}
		if existing, ok := keyByName[name]; ok && existing != key {
			delete(keyByName, name)
			ambiguous[name] = true
			return
		}
		keyByName[name] = key
	}
	for _, candidate := range candidates {
		if candidate.Reach != storypack.ReachLocal {
			continue
		}
		character := candidate.Character
		key := strings.TrimSpace(character.SourceKey)
		if key == "" {
			continue
		}
		claim(character.Name, key)
		for _, alias := range character.Aliases {
			claim(alias, key)
		}
	}
	// The page is authoritative about what the narrator was shown: the renderer
	// drops whole cards that do not fit its byte budget, so a name that never
	// reached the block must not be bindable.
	offered := make(map[string]string)
	for _, card := range offeredCard.FindAllStringSubmatch(block, -1) {
		name := normalizePrintedName(card[1])
		key, ok := keyByName[name]
		if !ok || key != strings.TrimSpace(card[2]) {
			continue
		}
		offered[name] = key
	}
	// Assigned unconditionally, including empty: the recorded page must be this
	// turn's page, and a turn that offered nobody must not keep binding against
	// whatever an earlier pass recorded.
	c.mu.Lock()
	c.offeredPeople = offered
	c.mu.Unlock()
}

// offeredPeopleSnapshot copies the recorded page for the turn's submit path.
func (c *Conversation) offeredPeopleSnapshot() map[string]string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.offeredPeople) == 0 {
		return nil
	}
	out := make(map[string]string, len(c.offeredPeople))
	for name, key := range c.offeredPeople {
		out[name] = key
	}
	return out
}

// bindStagedPeople attaches the author's key to a first appearance the model
// staged without one.
//
// Only a profile that arrived with no key is touched, so a key the model did
// supply is never overridden, and a person the story invented — whose name the
// page does not print — passes through unchanged. This runs before the module is
// validated and committed, so the committed profile carries the key and the
// existing source-key-to-actor remap can recognise this person the next time they
// appear instead of registering a second actor for them.
func (c *Conversation) bindStagedPeople(facts *interactive.TurnCharacterFactsInput) {
	if c == nil || facts == nil || len(facts.Profiles) == 0 {
		return
	}
	offered := c.offeredPeopleSnapshot()
	if len(offered) == 0 {
		return
	}
	for index := range facts.Profiles {
		profile := &facts.Profiles[index]
		if strings.TrimSpace(profile.SourceKey) != "" {
			continue
		}
		key, ok := offered[normalizePrintedName(profile.Name)]
		if !ok {
			continue
		}
		profile.SourceKey = key
	}
}

// sourceKeyAliases corrects a key the model wrote from memory against the world
// pack this story is actually bound to.
//
// §U3 requires the key to be verified by the bound pack rather than trusted:
// a key the pack does not ship, a key from another pack, and a key that belongs
// to a different person than the one the profile names are all the same failure —
// the identity the next turn matches on would point at someone else, and two
// people would share one actor or one person would become two. The repair is
// narrow on purpose:
//
//   - A key the bound pack ships, for the name (or a verified alias) this
//     profile carries, is accepted as written.
//   - A key the bound pack does not ship, or ships for a different name, is
//     replaced by the key this pack gives the submitted name, when exactly one
//     person carries that name. That is the same evidence the page uses, so a
//     correction never invents a correspondence the turn did not establish.
//   - A key that cannot be resolved either way is cleared, so the profile is
//     registered as the person the prose named rather than bound to a stranger.
//     A cleared key is the safe failure: it loses an identity rather than
//     granting one.
//
// Nothing here consults a global name index across packs, and an ambiguous name
// is never resolved by guessing — two people who share a name are not merged.
func (c *Conversation) sourceKeyAliases(facts *interactive.TurnCharacterFactsInput) {
	if c == nil || facts == nil || len(facts.Profiles) == 0 {
		return
	}
	catalog, byName, ok := c.boundCatalogIndexes()
	if !ok {
		return
	}
	for index := range facts.Profiles {
		profile := &facts.Profiles[index]
		submitted := strings.TrimSpace(profile.SourceKey)
		name := normalizePrintedName(profile.Name)
		if submitted != "" {
			if character, ok := catalog[submitted]; ok && characterNamesPerson(character, name) {
				continue
			}
			profile.SourceKey = ""
		}
		if name == "" {
			continue
		}
		if key, ok := byName[name]; ok {
			profile.SourceKey = key
		}
	}
}

// characterNamesPerson reports whether a catalog entry really carries this name.
// The pack's verified aliases count; an unverified nickname does not exist in the
// catalog at all, which is what keeps this check to author-verified spellings.
func characterNamesPerson(character storypack.CatalogCharacter, normalizedName string) bool {
	if normalizedName == "" {
		return false
	}
	if normalizePrintedName(character.Name) == normalizedName {
		return true
	}
	for _, alias := range character.Aliases {
		if normalizePrintedName(alias) == normalizedName {
			return true
		}
	}
	return false
}

// boundCatalogIndexes is the identity index this story verifies submitted keys
// against: the author's own source-key map and their name map.
//
// It reads the story's frozen catalog snapshot first. A story started after
// snapshots existed keeps verifying keys against the revision it began on, so a
// pack edited afterwards cannot repoint an identity mid-story. Only a story with
// no snapshot falls back to the live pack, and only while that pack is still the
// revision the story bound to — a later revision's keys are keys this story
// never agreed to.
func (c *Conversation) boundCatalogIndexes() (map[string]storypack.CatalogCharacter, map[string]string, bool) {
	storyCtx, ok := c.boundStoryContext()
	if !ok {
		return nil, nil, false
	}
	binding := storyCtx.Meta.PackBinding
	if frozen := frozenCatalog(storyCtx.Meta.PackCatalog); frozen != nil {
		if len(frozen.People) == 0 {
			return nil, nil, false
		}
		return frozen.CatalogBySourceKey(), frozen.CatalogByName(), true
	}
	pack, err := storypack.NewLibrary(c.novaDir).Get(binding.PackID)
	if err != nil {
		return nil, nil, false
	}
	if binding.ContentHash != "" && pack.ContentHash() != binding.ContentHash {
		return nil, nil, false
	}
	return pack.CatalogBySourceKey(), pack.CatalogByName(), true
}

// boundStoryContext reads the current story's context for the binding helpers.
func (c *Conversation) boundStoryContext() (interactive.StoryContext, bool) {
	if c == nil || c.store == nil || c.novaDir == "" {
		return interactive.StoryContext{}, false
	}
	c.mu.Lock()
	storyID := c.storyID
	branchID := c.branchID
	c.mu.Unlock()
	storyCtx, err := c.store.StoryContext(storyID, branchID)
	if err != nil {
		return interactive.StoryContext{}, false
	}
	binding := storyCtx.Meta.PackBinding
	if binding == nil || strings.TrimSpace(binding.PackID) == "" {
		return interactive.StoryContext{}, false
	}
	return storyCtx, true
}
