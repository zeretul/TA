package interactiveapp

import (
	"strings"
	"ta/internal/book/lore"
	"ta/internal/storypack"
	"testing"
)

func TestWorldAdaptationGeneratedCoreRuleReachesOrdinaryAction(t *testing.T) {
	g := storypack.RecommendedWorldGameplay()
	g.Confirmed = true
	draft := storypack.WorldDraft{Name: "灯印岛", Origin: "你来到见证台，尚未立约。", ProtagonistName: "叶澄", Gameplay: g, World: []storypack.DraftWorldEntry{{Name: "灯印规则", Type: "rule", BriefDescription: "约定的成立条件。", Content: "只有当事人自愿、真实见证人在场且灯正在点亮时，约定才生成灯印。灯熄灭不取消已成立的约定。"}}}
	p, err := draft.BuildPack()
	if err != nil {
		t.Fatal(err)
	}
	// Imported packs may omit the routing field; they need the same core-rule
	// delivery as the generated draft, without overriding explicit manual rules.
	p.World[0].LoadMode = ""
	p.World = append(p.World, storypack.WorldEntry{ID: "private-rule", Name: "未揭示的秘密", Type: "rule", LoadMode: "manual", Content: "不可自动公开的秘密"})
	root := t.TempDir()
	store := lore.NewScopedStore(root, p.WorldScope())
	for _, in := range p.WorldInputs() {
		if _, err = store.Create(in); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := store.List()
	t.Logf("generated rule load_mode=%s keywords=%v", items[0].LoadMode, items[0].Keywords)
	dynamic, err := buildInteractiveStoryLoreContext(loreSelectionInput{Workspace: root, Scope: p.WorldScope(), UserAction: "我自愿明天归还借书。", Location: "见证台"})
	if err != nil {
		t.Fatal(err)
	}
	resident, err := store.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resident+dynamic.Context, "不可自动公开的秘密") {
		t.Fatal("manual rule became public")
	}
	if !strings.Contains(resident+dynamic.Context, "灯熄灭不取消") {
		t.Fatalf("core rule omitted from both resident and ordinary-action working set; %s", dynamic.Note)
	}
}
