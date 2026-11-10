package interactiveapp

import (
	"fmt"
	"log/slog"
	"strings"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// discoveryCatalog is the catalog a story may locate its people in, together
// with where that catalog came from.
//
// The two sources are not interchangeable and the difference is the point. A
// story started after snapshots existed reads its own frozen index and is
// unaffected by later pack edits. A story started before that has no snapshot,
// and reading the live pack is the only way to keep it working at all — but only
// while the pack is still the revision it bound to.
type discoveryCatalog struct {
	// catalog answers the discovery questions. It is either the story's frozen
	// snapshot or the live pack.
	catalog catalogSource
	// revisionMismatch is set when the only available source is a live pack whose
	// fingerprint differs from the story's binding. The story cannot be given a
	// page in that case, but it is a diagnosable state, not an empty world.
	revisionMismatch bool
}

// catalogSource is the query surface the discovery path needs. Both the frozen
// snapshot and the live pack answer all of it, which is what lets one path serve
// a story on its own snapshot and a legacy story on its pack.
type catalogSource interface {
	CatalogBySourceKey() map[string]storypack.CatalogCharacter
	CatalogByName() map[string]string
	MentionedNames(text string) []string
	CharacterCandidates(req storypack.CandidateRequest) []storypack.Candidate
	LocationNamesAPlace(location string) bool
}

// discoveryContextOutcome is what one turn's page preparation produced. The
// caller needs the rendered block and also needs to know whether a page was
// withheld because the world holds nobody fitting, or because the catalog could
// not be read at all: the first is an ordinary turn, the second is a defect the
// player should be told about rather than silently play through.
type discoveryContextOutcome struct {
	// Block is the rendered page, empty when no page was prepared.
	Block string
	// Failure is a real read or version failure, nil when the turn simply had no
	// candidates. It is separate from an empty Block on purpose: §8 of the
	// discovery design forbids a failed read becoming an empty roster, and the
	// only way to keep that distinction is to carry it out of the function.
	Failure error
}

// storyDiscoveryContext is the narrator's view of who this world could bring
// into the scene: a page of candidates selected for where the protagonist is and
// what the player just asked for.
//
// It is the other half of the cast block. The cast block says who is already
// here and what has been said to them; this block says who else exists, where
// they are relative to this scene, and what to read before any of them appears.
// Without it, a player asking for a local expert gets whoever the model happens
// to invent, and the world's verified people are unreachable no matter how well
// they fit.
//
// The block is an index, not knowledge, and it is written to be read that way:
// being listed means a person exists in this world, not that the protagonist has
// met them or that they can be summoned. Reach is stated per person in words the
// narrator must act on, which is what keeps a person in another country out of
// the scene.
//
// Failure produces no block rather than an empty one. A failed resource read that
// became an empty roster is exactly what lets the model fill the gap with an
// invented person of the same name; withholding the page leaves it working from
// what it already has. The failure itself is returned so the turn can record it.
func (c *Conversation) storyDiscoveryContext(storyCtx interactive.StoryContext, userAction string) discoveryContextOutcome {
	candidates, failure := c.discoveryCandidates(storyCtx, userAction)
	if failure != nil {
		// A page that could not be built is not an empty page: it is no page. The
		// recorded correspondence is cleared so a first appearance this turn is
		// never bound against a page the narrator was not shown.
		c.recordOfferedPeople(nil, "")
		return discoveryContextOutcome{Failure: failure}
	}
	if len(candidates) == 0 {
		c.recordOfferedPeople(nil, "")
		return discoveryContextOutcome{}
	}
	block := storypack.CandidateCardsMarkdown(candidates, storypack.DefaultCandidateMaxBytes)
	// The page the narrator is about to read is also the program's own record of
	// which author key each offered person carries. Recording it here, from the
	// rendered block, is what lets a first appearance be bound to the author's
	// identity when the model submits no key of its own.
	c.recordOfferedPeople(candidates, block)
	return discoveryContextOutcome{Block: block}
}

// discoveryCandidates resolves this turn's page of people, or reports why no
// page could be built. It is split from the rendering so the page can be both
// shown to the narrator and recorded as the turn's own binding evidence.
//
// A nil error with no candidates is an ordinary answer: this world holds nobody
// who fits this scene and this request. A non-nil error means the catalog itself
// could not be resolved, which is a fault rather than an absence.
func (c *Conversation) discoveryCandidates(storyCtx interactive.StoryContext, userAction string) ([]storypack.Candidate, error) {
	if c == nil || c.store == nil || c.novaDir == "" {
		return nil, nil
	}
	binding := storyCtx.Meta.PackBinding
	if binding == nil || strings.TrimSpace(binding.PackID) == "" {
		return nil, nil
	}
	resolved, err := c.resolveDiscoveryCatalog(storyCtx)
	if err != nil {
		return nil, err
	}
	if resolved.catalog == nil {
		return nil, nil
	}
	if resolved.revisionMismatch {
		return nil, fmt.Errorf("剧本包 %s 的目录已更新，与故事绑定的版本 %s 不一致；该故事建立于目录快照之前，只能保留原有阵容",
			binding.PackID, binding.ContentHash)
	}

	scene := interactive.StoryScene(storyCtx.Snapshot)
	location := strings.TrimSpace(scene.Location)
	if location == "" {
		// Before the first turn commits, the story has no recorded scene, and the
		// state schema is still waiting on the opening handshake. An empty location
		// is not a neutral input: every person then reads as being in another
		// country, including a shopkeeper standing in the market the story opens
		// in, and the narrator is told not to use any of them. The story's own
		// premise names where it begins, so the page is built from that instead.
		location = strings.TrimSpace(storyCtx.Meta.Origin)
	}
	if location == "" {
		// Nothing names a place, so there is no basis for saying who is here or
		// who is elsewhere. §8 requires a read that cannot place anyone to produce
		// no block rather than a roster that asserts everyone is foreign.
		return nil, nil
	}
	// A location that names no place the catalog knows is the same case: the page
	// would mark every person as being in another country, which is a claim about
	// the world the program cannot support.
	if !resolved.catalog.LocationNamesAPlace(location) {
		return nil, nil
	}
	candidates := resolved.catalog.CharacterCandidates(storypack.CandidateRequest{
		Location:    location,
		Action:      userAction,
		Named:       resolved.catalog.MentionedNames(userAction),
		Established: c.establishedSourceKeys(storyCtx),
	})
	return candidates, nil
}

// resolveDiscoveryCatalog picks the catalog this story reads and says why when it
// has none.
//
// The frozen snapshot wins whenever the story has one. It is the revision the
// story actually started on, so a pack edited afterwards neither adds people the
// story never knew nor removes the ones it did. Only a story created before
// snapshots existed falls back to the live pack, and only while that pack is
// still the revision the story bound to.
func (c *Conversation) resolveDiscoveryCatalog(storyCtx interactive.StoryContext) (discoveryCatalog, error) {
	binding := storyCtx.Meta.PackBinding
	if frozen := frozenCatalog(storyCtx.Meta.PackCatalog); frozen != nil {
		if len(frozen.People) == 0 {
			// A snapshot with no people is a story whose pack shipped no catalog.
			// That is an empty world, not a failure.
			return discoveryCatalog{}, nil
		}
		return discoveryCatalog{catalog: frozen}, nil
	}
	pack, err := storypack.NewLibrary(c.novaDir).Get(binding.PackID)
	if err != nil {
		// The pack file is gone or unreadable and this story has no snapshot to
		// fall back on. Reporting it is the whole point of this path: the story
		// still plays, but it can no longer find its own preset people, and that is
		// a fact about the install rather than about the scene.
		slog.Warn("[interactive] discovery catalog unavailable", "pack_id", binding.PackID, "error", err)
		return discoveryCatalog{}, fmt.Errorf("剧本包 %s 无法读取：%w", binding.PackID, err)
	}
	if len(pack.Characters) == 0 {
		return discoveryCatalog{}, nil
	}
	// A legacy story has no snapshot, so the only revision available is whatever
	// the file holds now. Offering a later revision's people would let a running
	// story find someone who did not exist when it began.
	if binding.ContentHash != "" && pack.ContentHash() != binding.ContentHash {
		slog.Warn("[interactive] discovery catalog revision differs from the story's binding",
			"pack_id", binding.PackID,
			"story_hash", binding.ContentHash, "pack_hash", pack.ContentHash())
		return discoveryCatalog{catalog: pack, revisionMismatch: true}, nil
	}
	return discoveryCatalog{catalog: pack}, nil
}

// establishedSourceKeys lists the author keys this branch has already turned into
// actors. They are left out of the candidate page: they are already in the cast
// block with their stable ids, and re-offering them is how a story introduces
// someone it has already met as if they were new.
func (c *Conversation) establishedSourceKeys(storyCtx interactive.StoryContext) map[string]bool {
	state, err := c.store.CharacterState(c.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return nil
	}
	keys := state.SourceKeys()
	if len(keys) == 0 {
		return nil
	}
	established := make(map[string]bool, len(keys))
	for _, key := range keys {
		established[key] = true
	}
	return established
}
