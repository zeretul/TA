package interactiveapp

import (
	"reflect"
	"strings"
	"testing"

	"ta/internal/exploration"
	"ta/internal/interactive"
)

func TestWorldCooperationContextUsesKnownProjectionAndExactSources(t *testing.T) {
	project := interactive.CooperationProjectView{
		ProjectID: "coop.servicebay.markers", InstanceID: "known-instance", RealmID: exploration.RealmShaftServiceBay,
		Name: "离线标记整备", Description: "共同整理离线标记。", PlayerRole: "自付一份合金丝。", PartnerRole: "只整理标记，不远程抢修。",
		PartnerID: "known-partner", PartnerName: "已认识的技工", LocationID: "xinghai-l01", LocationName: "47号长廊", Phase: "worked",
		DurationSeconds: 600, PartnerEnergyCost: 6, BlockedReason: "not_present",
		ClueText: "隔离标识显示可逐段断电。", ClueSourceRef: "clue:actual-survey-event",
		PlayerCosts: []exploration.ExploreResourceCost{{Kind: exploration.ExploreConsumeItem, RefID: "xinghai-i34", Name: "合金丝", Quantity: 1}},
		ReportFacts: []interactive.CooperationReportFactView{{Kind: "ingress", SourceRef: "ingress:real-ingress", ValueKey: "game.realm.explore.serviceBay.slow.name"}, {Kind: "setback", SourceRef: "setback:real-setback"}},
	}
	state := interactive.WorldState{Realms: interactive.RealmState{Cooperations: []interactive.CooperationProjectView{project}}}
	page := renderWorldActions(state, interactive.StoryContextMaxBytes)
	for _, want := range []string{"action=cooperation", "project_id `coop.servicebay.markers`", "instance_id `known-instance`", "已认识的技工", "共同耗时 600 秒", "对方自付能量 6", "合金丝", "当前阻碍=not_present", "share_discovery=true", "source_text", "本人同意", "decision=accept|refuse", "不挪用NPC私物", "不是现实等待", "普通独行", "隔离标识显示可逐段断电", "clue:actual-survey-event", "ingress:real-ingress", "setback:real-setback", "report_source_refs"} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing cooperation context %q: %s", want, page)
		}
	}
	for _, hidden := range []string{"席尔瓦", "core_side", "cache_reward", "coop.herbcave.notes", "coop.tideboat.current"} {
		if strings.Contains(page, hidden) {
			t.Fatalf("renderer consulted hidden author content: %s", hidden)
		}
	}
	if !reflect.DeepEqual(state.Realms.Cooperations[0], project) {
		t.Fatal("render mutated known cooperation data")
	}
}

func TestWorldCooperationContextNoUnknownProjectsOrPartialEntries(t *testing.T) {
	var empty strings.Builder
	writeWorldCooperations(&empty, nil, 500)
	if empty.Len() != 0 {
		t.Fatal("empty knowledge exposed cooperation opportunities")
	}
	project := interactive.CooperationProjectView{ProjectID: "first", InstanceID: "one", Name: "A known project", ClueText: strings.Repeat("known observation ", 100)}
	var limited strings.Builder
	writeWorldCooperations(&limited, []interactive.CooperationProjectView{project, {ProjectID: "second", InstanceID: "two"}}, 250)
	if limited.Len() > 250 || strings.Contains(limited.String(), "project_id `") || strings.Contains(limited.String(), "known observation") {
		t.Fatal("budget split a cooperation entry or leaked a skipped clue")
	}
	if got := worldCooperationReportText(exploration.RealmMountainHerbCave, interactive.CooperationReportFactView{Description: "主角实际完成了“保住旧阵源”。", Kind: "disposition", ValueKey: "game.realm.explore.herbCave.preserve.name"}); !strings.Contains(got, "保住旧阵源") {
		t.Fatal("known report lacks its actual result wording")
	}
}
