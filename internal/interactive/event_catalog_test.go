package interactive

import (
	"strings"
	"testing"
)

func eventCatalogFixture() []EventPackage {
	long := "## 触发场景\n\n" + strings.Repeat("铺垫。", 200) + "\n\n## 硬限制\n\n玩家必须已经取得入城许可，且此地没有巡夜人。"
	return []EventPackage{
		{
			ID: "academy", Name: "学院事件包", Enabled: true,
			Events: []EventCard{
				{ID: "trial", TypeName: "外门考核打脸", Enabled: true, Category: "冲突", Intensity: "high",
					Tags: []string{"学院", "比试"}, DescriptionMarkdown: "## 触发场景\n外门考核中同门当众质疑主角。"},
				{ID: "herbs", TypeName: "后山采药", Enabled: true, Category: "探索", Intensity: "low",
					Tags: []string{"学院", "采集"}, DescriptionMarkdown: "## 触发场景\n后山药圃需要人手。"},
				{ID: "disabled", TypeName: "已停用卡", Enabled: false, DescriptionMarkdown: "不应出现。"},
			},
		},
		{
			ID: "city", Name: "城门口事件包", Enabled: true,
			Events: []EventCard{
				{ID: "gate", TypeName: "夜半盘查", Enabled: true, Category: "交涉", Intensity: "medium",
					Tags: []string{"城门"}, DescriptionMarkdown: long},
			},
		},
		{
			ID: "off", Name: "停用事件包", Enabled: false,
			Events: []EventCard{
				{ID: "never", TypeName: "不该出现", Enabled: true, DescriptionMarkdown: "停用包内的卡。"},
			},
		},
	}
}

// TestEventCatalogIsIndependentOfPlanning documents the NAR-01 guarantee at the
// level the model actually sees: the catalog is built from the story's bound
// event packages and knows nothing about the planning switch. Event material
// that only appeared when planning was on made the story's own content depend
// on an unrelated setting.
func TestEventCatalogIsIndependentOfPlanning(t *testing.T) {
	entries := EventCatalog(eventCatalogFixture())
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.CardID)
	}
	want := []string{"trial", "herbs", "gate"}
	if len(ids) != len(want) {
		t.Fatalf("catalog = %v, want %v", ids, want)
	}
	for index, id := range want {
		if ids[index] != id {
			t.Fatalf("catalog = %v, want %v", ids, want)
		}
	}
	// Authored order is preserved: a large library must not systematically favour
	// cards whose IDs sort first.
	if ids[0] != "trial" || ids[2] != "gate" {
		t.Fatalf("catalog reordered the authored cards: %v", ids)
	}
}

// TestEventCatalogOmitsNoCardAndStatesItsOwnTruncation is the NAR-04 guarantee.
// The old projection concatenated card bodies and trimmed the result, so a long
// card lost its tail — and a card's limits and consequences live in that tail.
// The directory now bounds itself and says what it left out.
func TestEventCatalogOmitsNoCardAndStatesItsOwnTruncation(t *testing.T) {
	packages := eventCatalogFixture()
	entries := EventCatalog(packages)
	if len(entries) != 3 {
		t.Fatalf("catalog dropped cards: %d entries", len(entries))
	}

	full := EventCatalogMarkdown(packages, StoryContextMaxBytes)
	for _, want := range []string{"trial", "herbs", "gate"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full catalog is missing %q:\n%s", want, full)
		}
	}
	if strings.Contains(full, "not listed here") {
		t.Fatalf("full catalog claimed truncation it did not do:\n%s", full)
	}
	// The directory never carries a card body, so no tail can be lost.
	if strings.Contains(full, "硬限制") || strings.Contains(full, "玩家必须已经取得入城许可") {
		t.Fatalf("catalog leaked a card body instead of a directory line:\n%s", full)
	}

	tight := EventCatalogMarkdown(packages, 300)
	if !strings.Contains(tight, "not listed here") {
		t.Fatalf("a truncated catalog must say so:\n%s", tight)
	}
	if !strings.Contains(tight, "list_event_cards") {
		t.Fatalf("a truncated catalog must point at the search tool:\n%s", tight)
	}
	if len([]byte(tight)) > 300+512 {
		t.Fatalf("catalog exceeded its budget by more than the explanation: %d bytes", len([]byte(tight)))
	}
}

// TestEventCardReadReturnsTheCompleteBody covers the other half of NAR-04: the
// limit that a truncated concatenation used to lose is exactly what a full read
// must deliver.
func TestEventCardReadReturnsTheCompleteBody(t *testing.T) {
	packages := eventCatalogFixture()
	pkg, card, err := FindEventCard(packages, "gate")
	if err != nil {
		t.Fatal(err)
	}
	body := FormatEventCard(pkg, card)
	if !strings.Contains(body, "硬限制") || !strings.Contains(body, "玩家必须已经取得入城许可") {
		t.Fatalf("full card read lost the card's tail constraints:\n%s", body)
	}
	if len([]rune(card.DescriptionMarkdown)) > MaxEventCardDescriptionChars {
		t.Fatal("fixture is larger than the card limit and cannot prove a full read")
	}

	if _, _, err := FindEventCard(packages, "missing"); err == nil {
		t.Fatal("an unknown card ID must be reported, not silently empty")
	}
	// A disabled card is not material this story can use.
	if _, _, err := FindEventCard(packages, "disabled"); err == nil {
		t.Fatal("a disabled card was readable")
	}
	if _, _, err := FindEventCard(packages, "never"); err == nil {
		t.Fatal("a card inside a disabled package was readable")
	}
}

// TestFindEventCardReportsAmbiguousIDs keeps the identity contract honest: the
// same card ID in two packages is a real ambiguity, and answering with whichever
// package happened to be listed first would silently play the wrong material.
func TestFindEventCardReportsAmbiguousIDs(t *testing.T) {
	packages := []EventPackage{
		{ID: "alpha", Name: "甲包", Enabled: true, Events: []EventCard{{ID: "shared", TypeName: "甲版", Enabled: true, DescriptionMarkdown: "甲。"}}},
		{ID: "beta", Name: "乙包", Enabled: true, Events: []EventCard{{ID: "shared", TypeName: "乙版", Enabled: true, DescriptionMarkdown: "乙。"}}},
	}
	if _, _, err := FindEventCard(packages, "shared"); err == nil {
		t.Fatal("an ambiguous card ID was resolved silently")
	} else if !strings.Contains(err.Error(), "alpha:shared") || !strings.Contains(err.Error(), "beta:shared") {
		t.Fatalf("the ambiguity error must name the qualified IDs: %v", err)
	}
	pkg, card, err := FindEventCard(packages, "beta:shared")
	if err != nil {
		t.Fatalf("a qualified ID must resolve: %v", err)
	}
	if pkg.ID != "beta" || card.TypeName != "乙版" {
		t.Fatalf("qualified lookup returned %s/%s", pkg.ID, card.TypeName)
	}
}

func TestSearchEventCatalogFiltersWithoutReadingBodies(t *testing.T) {
	entries := EventCatalog(eventCatalogFixture())

	if got := SearchEventCatalog(entries, []string{"采药"}, nil, nil); len(got) != 1 || got[0].CardID != "herbs" {
		t.Fatalf("keyword search = %v", got)
	}
	if got := SearchEventCatalog(entries, nil, []string{"交涉"}, nil); len(got) != 1 || got[0].CardID != "gate" {
		t.Fatalf("category search = %v", got)
	}
	if got := SearchEventCatalog(entries, nil, nil, []string{"学院"}); len(got) != 2 {
		t.Fatalf("tag search = %v", got)
	}
	if got := SearchEventCatalog(entries, []string{"不存在的词"}, nil, nil); len(got) != 0 {
		t.Fatalf("an unmatched search must be empty, got %v", got)
	}
	// A keyword that only appears inside a card body must not match: the
	// directory is what is searchable, and search must not become a lossy second
	// way of reading a card.
	if got := SearchEventCatalog(entries, []string{"巡夜人"}, nil, nil); len(got) != 0 {
		t.Fatalf("search matched inside a card body: %v", got)
	}
}
