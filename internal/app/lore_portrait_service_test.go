package app

import (
	"strings"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/portrait"
	"ta/internal/storypack"
)

// testPackWithPeople builds a pack whose world book holds one entry per person
// and whose catalog holds the author keys those entries are about. It is the
// smallest shape that exercises the join: an entry ID the pack wrote, a scope
// derived from this revision, and a catalog row naming the author key.
func testPackWithPeople(t *testing.T, packID string, people map[string]string) storypack.Pack {
	t.Helper()
	pack := storypack.Pack{
		Version:        1,
		ID:             packID,
		Name:           packID,
		ContentVersion: 1,
		Story:          storypack.PackStory{Title: packID, Origin: "起点", OpeningText: "开场"},
	}
	for loreID, name := range people {
		pack.World = append(pack.World, storypack.WorldEntry{ID: loreID, Name: name, Content: name + "的正文"})
		pack.Characters = append(pack.Characters, storypack.CatalogCharacter{
			SourceKey: strings.TrimPrefix(loreID, "char-"),
			Name:      name,
			LoreID:    loreID,
		})
	}
	return pack
}

func loreItemsForPack(pack storypack.Pack, withImage map[string]bool) []booklore.Item {
	hash := pack.ContentHash()
	scope := interactive.PackScopeForRevision(pack.ID, hash)
	items := make([]booklore.Item, 0, len(pack.World))
	for _, entry := range pack.World {
		item := booklore.Item{
			ID:    interactive.PackEntryIDForRevision(entry.ID, hash),
			Scope: scope,
			Type:  "character",
			Name:  entry.Name,
		}
		if withImage[entry.ID] {
			item.Image = &booklore.Image{Schema: "lore_item_image.v1", ImagePath: "assets/lore/" + entry.ID + ".png"}
		}
		items = append(items, item)
	}
	return items
}

// TestTheAuthorLoreLibraryShowsTheShippedFaces is the gap R2 named, stated the
// way the author meets it: the world book a pack wrote carries no image field at
// all, and the lore library still has to show each person's face. The join runs
// on the author key through the revision-qualified entry ID, never on a name.
func TestTheAuthorLoreLibraryShowsTheShippedFaces(t *testing.T) {
	library := portrait.NewLibrary()
	keys := library.Keys()
	if len(keys) < 2 {
		t.Fatalf("the shipped set has %d portraits", len(keys))
	}
	pack := testPackWithPeople(t, "fanren-jie", map[string]string{
		"char-" + keys[0]: "陈巧倩",
		"char-" + keys[1]: "南宫婉",
	})
	items := loreItemsForPack(pack, nil)

	got := loreEntryPortraitsByEntryID(items, []storypack.Pack{pack}, library)

	for _, entry := range pack.World {
		itemID := interactive.PackEntryIDForRevision(entry.ID, pack.ContentHash())
		face, ok := got[itemID]
		if !ok {
			t.Fatalf("no portrait for %s", entry.Name)
		}
		key := strings.TrimPrefix(entry.ID, "char-")
		if want := library.URL(key); face.ThumbnailURL != want {
			t.Fatalf("%s thumbnail is %q, want %q", entry.Name, face.ThumbnailURL, want)
		}
		if want := library.DisplayURL(key); face.DisplayURL != want {
			t.Fatalf("%s display portrait is %q, want %q", entry.Name, face.DisplayURL, want)
		}
		if !strings.HasPrefix(face.ThumbnailURL, portrait.PathPrefix) {
			t.Fatalf("%s is not served by the portrait route: %q", entry.Name, face.ThumbnailURL)
		}
	}
}

// TestAChosenPictureOutranksTheShippedFace keeps the precedence rule. A picture
// generated or uploaded in the lore library is the author's own decision, so the
// shipped default must not be offered beside it — and the entry that has one is
// not answered at all, which is what lets the page keep its clear control
// honest.
func TestAChosenPictureOutranksTheShippedFace(t *testing.T) {
	library := portrait.NewLibrary()
	key := library.Keys()[0]
	pack := testPackWithPeople(t, "fanren-jie", map[string]string{"char-" + key: "陈巧倩"})
	items := loreItemsForPack(pack, map[string]bool{"char-" + key: true})

	got := loreEntryPortraitsByEntryID(items, []storypack.Pack{pack}, library)

	itemID := interactive.PackEntryIDForRevision("char-"+key, pack.ContentHash())
	if face, ok := got[itemID]; ok {
		t.Fatalf("an entry with its own picture was given the shipped default %q", face.ThumbnailURL)
	}
}

// TestTwoWorldsWithTheSamePersonDoNotShareAFace is the isolation rule. Both
// worlds ship a 陈巧倩; a name match would put one world's face on the other's
// person, so the second world's person is matched by their own author key and
// the first world's entry never answers for them.
func TestTwoWorldsWithTheSamePersonDoNotShareAFace(t *testing.T) {
	library := portrait.NewLibrary()
	keys := library.Keys()
	if len(keys) < 2 {
		t.Fatalf("the shipped set has %d portraits", len(keys))
	}
	first := testPackWithPeople(t, "fanren-jie", map[string]string{"char-" + keys[0]: "陈巧倩"})
	// The second world has its own 陈巧倩 under a different author key.
	second := testPackWithPeople(t, "lanchuan-qunxia", map[string]string{"char-lanchuan-p01": "陈巧倩"})

	got := loreEntryPortraitsByEntryID(
		append(loreItemsForPack(first, nil), loreItemsForPack(second, nil)...),
		[]storypack.Pack{first, second},
		library,
	)

	firstID := interactive.PackEntryIDForRevision("char-"+keys[0], first.ContentHash())
	secondID := interactive.PackEntryIDForRevision("char-lanchuan-p01", second.ContentHash())
	if face := got[firstID]; face.ThumbnailURL != library.URL(keys[0]) {
		t.Fatalf("world-a's person got %q, want %q", face.ThumbnailURL, library.URL(keys[0]))
	}
	if face := got[secondID]; face.ThumbnailURL != library.WorldURL("lanchuan-qunxia", "lanchuan-p01", false) {
		t.Fatalf("second world's person got %q", face.ThumbnailURL)
	}
	foreign := testPackWithPeople(t, "xinghai-689", map[string]string{"char-" + keys[0]: "陈巧倩"})
	if faces := loreEntryPortraitsByEntryID(loreItemsForPack(foreign, nil), []storypack.Pack{foreign}, library); len(faces) != 0 {
		t.Fatal("a different world borrowed the same source key's portrait")
	}
}

// TestAnEntryFromAnOlderRevisionMatchesNothing is why the join is revision
// qualified. A pack edited in place writes a new revision's book under a new
// entry ID; the old entries stay in the library. They must not borrow the new
// revision's face, because the catalog that face comes from is not the catalog
// those entries were written by.
func TestAnEntryFromAnOlderRevisionMatchesNothing(t *testing.T) {
	library := portrait.NewLibrary()
	key := library.Keys()[0]
	older := testPackWithPeople(t, "fanren-jie", map[string]string{"char-" + key: "陈巧倩"})
	olderItems := loreItemsForPack(older, nil)

	newer := older
	newer.World = []storypack.WorldEntry{{ID: "char-" + key, Name: "陈巧倩", Content: "改写后的正文"}}
	if older.ContentHash() == newer.ContentHash() {
		t.Fatal("the edit did not change the content fingerprint")
	}

	got := loreEntryPortraitsByEntryID(olderItems, []storypack.Pack{newer}, library)

	if len(got) != 0 {
		t.Fatalf("an entry from the previous revision was given %d faces", len(got))
	}
}

// TestAPersonTheStoryInventedKeepsThePlaceholder is the unknown-appearance rule.
// A hand-written entry belongs to no catalog, so it is answered by no face.
func TestAPersonTheStoryInventedKeepsThePlaceholder(t *testing.T) {
	library := portrait.NewLibrary()
	pack := testPackWithPeople(t, "fanren-jie", map[string]string{"char-" + library.Keys()[0]: "陈巧倩"})
	items := append(loreItemsForPack(pack, nil), booklore.Item{
		ID:    "hand-written",
		Scope: interactive.PackScopeForRevision(pack.ID, pack.ContentHash()),
		Type:  "character",
		Name:  "我编的人",
	})

	got := loreEntryPortraitsByEntryID(items, []storypack.Pack{pack}, library)

	if _, ok := got["hand-written"]; ok {
		t.Fatal("an entry the story invented was given a shipped face")
	}
}

func TestPackIDFromRevisionScope(t *testing.T) {
	cases := []struct {
		scope string
		want  string
		ok    bool
	}{
		{"pack:fanren-jie@abc123", "fanren-jie", true},
		{"pack:fanren-jie", "", false},
		{"pack:@abc123", "", false},
		{"pack:world@a@b", "world@a", true},
		{"user:shared", "", false},
		{"", "", false},
	}
	for _, testCase := range cases {
		got, ok := packIDFromRevisionScope(testCase.scope)
		if got != testCase.want || ok != testCase.ok {
			t.Fatalf("packIDFromRevisionScope(%q) = %q, %v; want %q, %v", testCase.scope, got, ok, testCase.want, testCase.ok)
		}
	}
}

// TestTheShippedWorldGetsFacesForEveryCatalogPerson runs the join against the
// real shipped pack rather than a fixture: 103 catalog rows and 109 world
// entries, none of which carries an image. Every catalog person the shipped
// portrait set has a picture for must come out with a face, because the two are
// keyed by the same author keys.
//
// These authored people lack legacy portraits. They may receive dedicated art,
// but must never borrow a face by display name. Only these exact identities may
// remain empty; losing any other catalog person's picture still fails.
func TestTheShippedWorldGetsFacesForEveryCatalogPerson(t *testing.T) {
	library := portrait.NewLibrary()
	pack, err := storypack.NewLibrary("").GetContent("fanren-jie")
	if err != nil {
		t.Fatalf("read the shipped pack: %v", err)
	}
	catalog := pack.CatalogByLoreID()
	if len(catalog) == 0 {
		t.Fatal("the shipped pack has no catalog")
	}
	authoredWithoutAPicture := map[string]bool{
		"npc_g11_duandao_ke":                true,
		"species_lingshoushan_qingfeng_que": true,
		"spirit_qiao_yan":                   true,
		"spirit_ruan_qingluo":               true,
		"spirit_lu_shiheng":                 true,
		"spirit_ning_shutong":               true,
	}
	inputs := pack.WorldInputs()
	items := make([]booklore.Item, 0, len(inputs))
	for _, input := range inputs {
		if input.Image != nil {
			t.Fatalf("the shipped world book carries an image for %s", input.ID)
		}
		items = append(items, booklore.Item{ID: input.ID, Scope: input.Scope, Type: input.Type, Name: input.Name})
	}

	got := loreEntryPortraitsByEntryID(items, []storypack.Pack{pack}, library)

	missing := make([]string, 0)
	withFace := 0
	allowedMissing := 0
	for loreID, person := range catalog {
		entryID := interactive.PackEntryIDForRevision(loreID, pack.ContentHash())
		face, ok := got[entryID]
		expected := library.WorldURL("fanren-jie", person.SourceKey, false)
		if !ok || face.ThumbnailURL == "" {
			if authoredWithoutAPicture[person.SourceKey] && expected == "" {
				allowedMissing++
				continue
			}
			missing = append(missing, person.Name+" ("+person.SourceKey+")")
			continue
		}
		if expected == "" || face.ThumbnailURL != expected {
			t.Fatalf("catalog person %s received a different identity's picture", person.SourceKey)
		}
		withFace++
	}
	if len(missing) > 0 {
		t.Fatalf("%d of %d catalog people got no face: %v", len(missing), len(catalog), missing)
	}
	if withFace != len(catalog)-allowedMissing {
		t.Fatalf("%d entries got a face, the catalog has %d people and %d authored ones without a picture",
			withFace, len(catalog), allowedMissing)
	}
}
