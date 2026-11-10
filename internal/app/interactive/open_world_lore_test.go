package interactiveapp

import (
	"reflect"
	"strings"
	"testing"

	"ta/internal/book/lore"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// These are actual frozen-catalog/world-book selection tests. They do not call
// a model or assert that narrative suggestions became skills, jobs or knowledge.
func TestOpenWorldOrdinaryInputsReachFrozenLifeRulesAndLocalPeople(t *testing.T) {
	for _, spec := range []struct {
		pack, prefix string
		cases        []struct {
			action, place string
			ids           []string
		}
	}{
		{"lanchuan-qunxia", "lanchuan", []struct {
			action, place string
			ids           []string
		}{
			{"我想先问问有没有人肯教拳。", "澜口城", []string{"lanchuan-life-work", "lanchuan-p04"}},
			{"I want a lesson before taking escort work.", "鹤渡镇", []string{"lanchuan-life-work", "lanchuan-p09", "lanchuan-p10"}},
		}},
		{"xinghai-689", "xinghai", []struct {
			action, place string
			ids           []string
		}{
			{"I want to ask about repair work, not join a mercenary company.", "枢纽三号真空改装船坞", []string{"xinghai-life-work", "xinghai-p01"}},
			{"我想问清私信是从哪里来的，先不接这个活。", "老锚机械酒吧与二手船材集市", []string{"xinghai-life-news", "xinghai-p54"}},
		}},
	} {
		t.Run(spec.pack, func(t *testing.T) {
			root := t.TempDir()
			pack, e := storypack.NewLibrary(t.TempDir()).Get(spec.pack)
			if e != nil {
				t.Fatal(e)
			}
			book := lore.NewStore(root)
			for _, input := range pack.WorldInputs() {
				if _, e = book.Create(input); e != nil {
					t.Fatal(e)
				}
			}
			store := interactive.NewStore(root)
			story, e := store.CreateStory(interactive.CreateStoryRequest{Title: "Frozen open-world context fixture", PackBinding: pack.Binding(), PackCatalog: catalogSnapshot(pack.FreezeCatalog())})
			if e != nil {
				t.Fatal(e)
			}
			if e = store.Close(); e != nil {
				t.Fatal(e)
			}
			store = interactive.NewStore(root)
			defer store.Close()
			ctx, e := store.StoryContext(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			catalog := frozenCatalog(ctx.Meta.PackCatalog)
			if catalog == nil {
				t.Fatal("new Story lost its frozen catalog")
			}
			if spec.prefix == "lanchuan" {
				yunshu := catalog.CatalogBySourceKey()["lanchuan-p37"]
				if yunshu.PlaceCandidate != "lanchuan-l24" {
					t.Fatal("Yunshu's source placement was dropped by freeze/persistence")
				}
				for _, c := range catalog.CharacterCandidates(storypack.CandidateRequest{Location: "紫霄观", Action: "找云舒", Named: []string{"云舒"}, Established: map[string]bool{"lanchuan-p37": true}}) {
					if c.Character.SourceKey == "lanchuan-p37" {
						t.Fatal("author placement reintroduced an established Story person")
					}
				}
			} else {
				action := "我在朱伯利想找人照顾伤员。"
				for _, c := range catalog.CharacterCandidates(storypack.CandidateRequest{Location: "朱伯利外环47号减压长廊", Action: action}) {
					if c.Character.SourceKey == "xinghai-p56" && c.Reach == storypack.ReachLocal {
						t.Fatal("medical topic teleported Lina from the Sunil front")
					}
				}
			}
			before, e := store.Snapshot(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			scoped := lore.NewScopedStore(root, pack.WorldScope())
			resident, e := assembleResidentLore(scoped)
			if e != nil {
				t.Fatal(e)
			}
			bodies := map[string]string{}
			for _, entry := range pack.World {
				bodies[entry.ID] = strings.TrimSpace(entry.Content)
			}
			if !strings.Contains(resident.Content, bodies[spec.prefix+"-life-identity"]) {
				t.Fatal("short autonomy rule did not enter actual resident context")
			}
			for _, c := range spec.cases {
				out, e := buildInteractiveStoryLoreContext(loreSelectionInput{Workspace: root, Scope: pack.WorldScope(), ContentHash: pack.ContentHash(), Catalog: catalog, UserAction: c.action, Location: c.place})
				if e != nil {
					t.Fatal(e)
				}
				for _, id := range c.ids {
					if bodies[id] == "" || !strings.Contains(out.Context, bodies[id]) {
						t.Errorf("%q did not retrieve %s; actual ledger: %s", c.action, id, out.Note)
					}
				}
			}
			unrelated, e := buildInteractiveStoryLoreContext(loreSelectionInput{Workspace: root, Scope: pack.WorldScope(), ContentHash: pack.ContentHash(), Catalog: catalog, UserAction: "我坐着吃完饭，擦了擦桌子。", Location: spec.cases[0].place})
			if e != nil {
				t.Fatal(e)
			}
			for _, id := range []string{spec.prefix + "-life-work", spec.prefix + "-life-news"} {
				if strings.Contains(unrelated.Context, bodies[id]) {
					t.Errorf("unrelated meal dragged in %s", id)
				}
			}
			after, e := store.Snapshot(story.ID, "main")
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("context selection changed Story state/permissions/head", e)
			}
		})
	}
}
