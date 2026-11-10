package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/config"
	"ta/internal/exploration"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

func TestRealmCooperationViewLocalizesOnlyBoundKnownCopy(t *testing.T) {
	root := t.TempDir()
	application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
	pack, err := storypack.NewLibrary(root).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	binding := &interactive.StoryPackBinding{PackID: pack.ID, ContentHash: pack.ContentHash()}
	var partner string
	for _, profile := range pack.Characters {
		if profile.SourceKey == "xinghai-p25" {
			partner = profile.Name
		}
	}
	if partner == "" {
		t.Fatal("authored partner is missing")
	}
	source := []interactive.CooperationProjectView{{PartnerName: partner, PartnerSourceKey: "xinghai-p25", ClueText: "already discovered clue", ClueTextKey: "game.realm.explore.serviceBay.facts.clue", ClueSourceRef: "clue:model-only-source", LocationID: "xinghai-l01", LocationName: "47号长廊", PlayerCosts: []exploration.ExploreResourceCost{{Kind: exploration.ExploreConsumeItem, RefID: "xinghai-i34", Quantity: 1, Name: "合金丝"}}, ReportFacts: []interactive.CooperationReportFactView{{Kind: "ingress", SourceRef: "ingress:committed-event"}}}}
	english := pack.Localizations["en-US"]
	if got := application.realmLocationName(binding, "en-US", "xinghai-l01"); got != english.World["xinghai-l01"].Name {
		t.Fatalf("registered current location ID not localized: %q", got)
	}
	if got := application.realmLocationName(binding, "zh-CN", "xinghai-l01"); got == "xinghai-l01" || got == "" {
		t.Fatalf("registered location lacks readable Chinese name: %q", got)
	}
	for _, prose := range []string{"an unfamiliar corridor", "只知其别称", "xinghai-unregistered"} {
		if got := application.realmLocationName(binding, "en-US", prose); got != prose {
			t.Fatalf("location prose or unknown identity changed: %q", got)
		}
	}
	view := application.realmCooperationViews(source, binding, "en-US", english.Adventure)
	if view[0].PartnerName != english.Characters["xinghai-p25"].Name || view[0].LocationName != english.World["xinghai-l01"].Name || view[0].PlayerCosts[0].Name != english.Adventure["xinghai-i34"] {
		t.Fatalf("known cooperation labels not localized: %+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, internal := range []string{"xinghai-p25", "clue:model-only-source"} {
		if strings.Contains(string(encoded), internal) {
			t.Fatalf("model-only metadata leaked to the API: %s", internal)
		}
	}
	for _, public := range []string{`"clue_text":"already discovered clue"`, `"clue_text_key":"game.realm.explore.serviceBay.facts.clue"`} {
		if !strings.Contains(string(encoded), public) {
			t.Fatalf("already-known sharing scope omitted from the API: %s", public)
		}
	}
	view[0].PlayerCosts[0].Name = "changed"
	view[0].ReportFacts[0].SourceRef = "changed"
	if source[0].PartnerName != partner || source[0].PlayerCosts[0].Name != "合金丝" || source[0].ReportFacts[0].SourceRef != "ingress:committed-event" {
		t.Fatal("localized read mutated canonical known view")
	}
	source[0].PartnerName = "只知其别称"
	view = application.realmCooperationViews(source, binding, "en-US", english.Adventure)
	if view[0].PartnerName != source[0].PartnerName {
		t.Fatal("localization disclosed an unknown canonical name")
	}
	stale := *binding
	stale.ContentHash = "another-frozen-revision"
	if got := application.realmLocationName(&stale, "en-US", "xinghai-l01"); got != "xinghai-l01" {
		t.Fatal("current location used a different frozen revision")
	}
	if view := application.realmCooperationViews(source, &stale, "en-US", nil); !reflect.DeepEqual(source, view) {
		t.Fatal("current sidecar overwrote an unrelated frozen story")
	}
	if view := application.realmCooperationViews(nil, binding, "en-US", nil); view == nil || len(view) != 0 {
		t.Fatal("empty API collection is not stable")
	}
}

func TestRealmCurrentLocationUsesAuthoredCultivationPlaceLabelOnly(t *testing.T) {
	root := t.TempDir()
	application := &App{workspace: root, cfg: &config.Config{DenovaDir: root}}
	pack, err := storypack.NewLibrary(root).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	binding := &interactive.StoryPackBinding{PackID: pack.ID, ContentHash: pack.ContentHash()}
	for _, person := range pack.Characters {
		if person.SourceKey != "npc_zhao_ling" {
			continue
		}
		if got := application.realmLocationName(binding, "zh-CN", person.PlaceCandidate); got != person.PlaceLabel {
			t.Fatalf("cultivation endpoint did not use registered label: %q", got)
		}
		if got := application.realmLocationName(binding, "en-US", person.PlaceCandidate); got != pack.Localizations["en-US"].Characters[person.SourceKey].PlaceName {
			t.Fatalf("cultivation endpoint not localized: %q", got)
		}
		return
	}
	t.Fatal("authored cultivation endpoint missing")
}
