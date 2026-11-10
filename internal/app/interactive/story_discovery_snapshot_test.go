package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/storypack"
)

// snapshotStoryContext is a story context whose meta carries the frozen catalog a
// story would have written at creation.
func snapshotStoryContext(pack storypack.Pack, branchID, location string) interactive.StoryContext {
	ctx := discoveryStoryContext(pack, branchID, location)
	ctx.Meta.PackCatalog = catalogSnapshot(pack.FreezeCatalog())
	return ctx
}

// TestAFrozenCatalogKeepsThePageAfterThePackIsEdited is the U7 defect seen from
// the conversation.
//
// Before this, the catalog was only ever in the pack file. A story bound to one
// revision read the file every turn, and once the file's fingerprint moved on the
// page was withheld — so an in-progress story silently lost every preset person
// the moment its pack was edited, even though its world book was untouched.
//
// The story's own snapshot is the fix: the page keeps offering the people the
// story started with, from the revision it actually bound to.
func TestAFrozenCatalogKeepsThePageAfterThePackIsEdited(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	const action = "我想找一位懂阵法的人请教残阵"
	local := pack.CatalogBySourceKey()["npc_song_guanlan"]
	if local.SourceKey == "" {
		t.Fatal("the shipped catalog lost the local array teacher")
	}

	// A story that started on this pack, and whose file has since moved on. The
	// binding hash deliberately names a revision the file no longer holds.
	ctx := snapshotStoryContext(pack, "main", "越国黄枫谷外的坊市")
	ctx.Meta.PackBinding = &interactive.StoryPackBinding{
		PackID:      pack.ID,
		ContentHash: "000000000000000000000000",
		Scope:       "pack:fanren-jie@000000000000000000000000",
	}

	outcome := conversation.storyDiscoveryContext(ctx, action)
	if outcome.Failure != nil {
		t.Fatalf("a story with its own snapshot reported a catalog failure: %v", outcome.Failure)
	}
	if !strings.Contains(outcome.Block, local.Name) {
		t.Fatalf("%s was lost from the page after the pack file moved on", local.Name)
	}
}

// TestAFrozenCatalogKeepsSourceKeyVerificationWorking is the identity half: the
// key a story verifies submissions against has to come from the same revision as
// the page, or a first appearance binds against a catalog the story never had.
func TestAFrozenCatalogKeepsSourceKeyVerificationWorking(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	pack := fanrenFormalPack(t, "")
	local := pack.CatalogBySourceKey()["npc_song_guanlan"]
	if local.SourceKey == "" {
		t.Fatal("the shipped catalog lost the local array teacher")
	}
	ctx := snapshotStoryContext(pack, "main", "越国黄枫谷外的坊市")
	ctx.Meta.PackBinding = &interactive.StoryPackBinding{
		PackID:      pack.ID,
		ContentHash: "000000000000000000000000",
		Scope:       "pack:fanren-jie@000000000000000000000000",
	}
	if frozen := frozenCatalog(ctx.Meta.PackCatalog); frozen == nil || len(frozen.People) == 0 {
		t.Fatal("the story context carries no frozen catalog")
	}

	// The verification path reads the story's own context, so it must still
	// recognize the author's key for this person under the same stale file.
	frozen := frozenCatalog(ctx.Meta.PackCatalog)
	byKey := frozen.CatalogBySourceKey()
	if _, ok := byKey[local.SourceKey]; !ok {
		t.Fatalf("the frozen catalog does not know %s", local.SourceKey)
	}
	byName := frozen.CatalogByName()
	if got := byName[strings.ToLower(strings.ReplaceAll(local.Name, " ", ""))]; got != local.SourceKey {
		t.Fatalf("the frozen catalog resolves %s to %q, want %q", local.Name, got, local.SourceKey)
	}
	_ = conversation
}

// TestAMissingPackWithNoSnapshotIsAFailureNotAnEmptyWorld is the other half of
// the requirement: "no candidates" and "could not read" are different answers and
// must not be reported the same way.
//
// A story with no snapshot has nothing to fall back on. When its pack is gone the
// turn still plays, but the fact that the world's people became unreachable is a
// defect the turn has to carry out rather than disguise as an empty roster.
func TestAMissingPackWithNoSnapshotIsAFailureNotAnEmptyWorld(t *testing.T) {
	conversation, _, _ := newDiscoveryConversation(t)
	ctx := interactive.StoryContext{
		Meta: interactive.StoryMeta{PackBinding: &interactive.StoryPackBinding{
			PackID:      "a-pack-that-is-not-installed",
			ContentHash: "000000000000000000000000",
			Scope:       "pack:a-pack-that-is-not-installed@000000000000000000000000",
		}},
		Snapshot: interactive.Snapshot{State: map[string]any{"actors": map[string]any{
			interactive.DefaultStoryContextActorID: map[string]any{
				"state": map[string]any{"当前详细地点": "越国黄枫谷外的坊市"},
			},
		}}},
	}
	outcome := conversation.storyDiscoveryContext(ctx, "我想找一位懂阵法的人")
	if outcome.Block != "" {
		t.Fatalf("an unreadable catalog produced a page: %q", outcome.Block)
	}
	if outcome.Failure == nil {
		t.Fatal("an unreadable catalog was reported as an ordinary empty page")
	}
}

// TestAFrozenSnapshotIsNotASecondWorldState pins the boundary the queue set: the
// snapshot holds locating records and placement facts, never a profile body, and
// the body stays in the world book.
func TestAFrozenSnapshotIsNotASecondWorldState(t *testing.T) {
	pack := fanrenFormalPack(t, "")
	snapshot := catalogSnapshot(pack.FreezeCatalog())
	if snapshot == nil || len(snapshot.Characters) == 0 {
		t.Fatal("the shipped pack froze no catalog")
	}
	const beastSourceKey = "species_lingshoushan_qingfeng_que"
	var storedBeast *interactive.PackCatalogPerson
	for i := range snapshot.Characters {
		if snapshot.Characters[i].SourceKey == beastSourceKey {
			storedBeast = &snapshot.Characters[i]
			break
		}
	}
	if storedBeast == nil || storedBeast.Lifeform != interactive.PackCatalogLifeformSpiritBeast || storedBeast.CultivationMethodID != "method.spirit_beast_breathing" {
		t.Fatalf("the frozen source lost its cultivation qualification: %+v", storedBeast)
	}
	queryBeast, ok := frozenCatalog(snapshot).CatalogBySourceKey()[beastSourceKey]
	if !ok || queryBeast.CultivationMethodID != storedBeast.CultivationMethodID || queryBeast.Lifeform != storedBeast.Lifeform {
		t.Fatalf("the query catalog lost the frozen species facts: %+v", queryBeast)
	}
	var bodies []string
	for _, entry := range pack.World {
		if body := strings.TrimSpace(entry.Content); len([]byte(body)) >= 200 {
			bodies = append(bodies, body)
		}
	}
	if len(bodies) == 0 {
		t.Fatal("the shipped pack has no world-book bodies to check against")
	}
	for _, person := range snapshot.Characters {
		for _, body := range bodies {
			if strings.Contains(person.Brief, body) {
				t.Fatalf("the snapshot copied a world-book body into %s's brief", person.SourceKey)
			}
		}
	}
}
