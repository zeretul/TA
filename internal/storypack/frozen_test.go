package storypack

import (
	"reflect"
	"testing"
)

// TestAFrozenCatalogAnswersLikeThePackItCameFrom is the invariant that makes the
// snapshot safe to serve instead of the pack: for every question the discovery
// path asks, the frozen catalog must give the same answer the live pack gives.
//
// It matters because the two are built by different code. The live index reads
// the world book to decide which authored labels are placement evidence, while
// the frozen one replays resolved placement facts. If they ever disagree, a
// story would silently start placing people differently the day it was frozen —
// which is the same class of silent failure the snapshot exists to prevent.
//
// It also pins the stored shape: placement facts must survive being written as
// rows and read back, including the token/country pairing that keeps one
// country's place-class word from placing another country's people.
func TestAFrozenCatalogAnswersLikeThePackItCameFrom(t *testing.T) {
	pack := builtinPacks()[0]
	frozen := pack.FreezeCatalog()
	if frozen == nil || len(frozen.People) == 0 {
		t.Fatal("the shipped pack froze no catalog")
	}
	if frozen.ContentHash != pack.ContentHash() {
		t.Fatalf("the snapshot names revision %q, the pack is %q",
			frozen.ContentHash, pack.ContentHash())
	}

	// Rows: same people, same keys, same order-independent content.
	liveByKey := pack.CatalogBySourceKey()
	frozenByKey := frozen.CatalogBySourceKey()
	if len(liveByKey) != len(frozenByKey) {
		t.Fatalf("the snapshot holds %d people, the pack holds %d", len(frozenByKey), len(liveByKey))
	}
	for key, live := range liveByKey {
		got, ok := frozenByKey[key]
		if !ok {
			t.Fatalf("%s exists in the pack and not in the snapshot", key)
		}
		if !reflect.DeepEqual(got, live) {
			t.Fatalf("%s differs between pack and snapshot:\npack:   %+v\nfrozen: %+v", key, live, got)
		}
	}

	// Names, including the ambiguity rule.
	if !reflect.DeepEqual(pack.CatalogByName(), frozen.CatalogByName()) {
		t.Fatal("the snapshot resolves names differently from the pack")
	}

	// Placement: the same scene must name the same places, and the same people
	// must be offered with the same reach.
	scenes := []string{
		"越国黄枫谷外的坊市",
		"越国公共坊市的外围。",
		"元武国天星宗的坊市",
		"紫金国云秀山门",
		"一个没有名字的地方",
	}
	for _, scene := range scenes {
		if got, want := frozen.LocationNamesAPlace(scene), pack.LocationNamesAPlace(scene); got != want {
			t.Fatalf("scene %q: snapshot says names-a-place=%v, pack says %v", scene, got, want)
		}
		req := CandidateRequest{Location: scene, Action: "我想找个人打听消息"}
		got := frozen.CharacterCandidates(req)
		want := pack.CharacterCandidates(req)
		if len(got) != len(want) {
			t.Fatalf("scene %q: snapshot offers %d candidates, pack offers %d", scene, len(got), len(want))
		}
		for index := range want {
			if got[index].Character.SourceKey != want[index].Character.SourceKey {
				t.Fatalf("scene %q: candidate %d is %s in the snapshot and %s in the pack",
					scene, index, got[index].Character.SourceKey, want[index].Character.SourceKey)
			}
			if got[index].Reach != want[index].Reach {
				t.Fatalf("scene %q: %s is %v in the snapshot and %v in the pack",
					scene, got[index].Character.SourceKey, got[index].Reach, want[index].Reach)
			}
		}
	}
}

// TestAFrozenPlacementRowKeepsItsCountry is the narrow version of the same
// invariant, stated on the stored row rather than the behaviour.
//
// The live index keys each place by "country\x00candidate\x00label" so that a
// place-class word two countries share (坊市, 山门) cannot place one country's
// people in the other's scene. A snapshot that stored the place without its
// country would silently lose that separation, so the row must carry both and
// the rebuild must restore them together.
func TestAFrozenPlacementRowKeepsItsCountry(t *testing.T) {
	pack := builtinPacks()[0]
	frozen := pack.FreezeCatalog()
	if len(frozen.Places) == 0 {
		t.Fatal("the shipped pack froze no placement facts")
	}
	shared := map[string]map[string]bool{}
	for _, place := range frozen.Places {
		if shared[place.Token] == nil {
			shared[place.Token] = map[string]bool{}
		}
		shared[place.Token][place.Country] = true
	}
	// At least one token must be used by more than one country, or this test
	// proves nothing about separation.
	var crossCountry int
	for _, countries := range shared {
		if len(countries) > 1 {
			crossCountry++
		}
	}
	if crossCountry == 0 {
		t.Fatal("no placement token is shared across countries; the test would prove nothing")
	}
	index := frozen.regionIndex()
	for token, countries := range shared {
		for country := range countries {
			if !index.ownsToken(country, token) {
				t.Fatalf("token %q lost its %s ownership in the frozen index", token, country)
			}
		}
	}
}
