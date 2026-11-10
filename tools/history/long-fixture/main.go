// Explicit fixture generator for isolated native acceptance, never a runtime API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"ta/internal/history"
)

func main() {
	storePath := flag.String("store", "", "isolated project store")
	output := flag.String("output", "", "fixture identity JSON")
	inspect := flag.String("inspect", "", "read an existing isolated campaign without mutation")
	flag.Parse()
	abs, err := filepath.Abs(*storePath)
	must(err)
	norm := strings.ToLower(filepath.ToSlash(abs))
	if !strings.Contains(norm, "/.ta-dev/history-limits/data/stores/") {
		panic("Fixture requires the designated isolated store")
	}
	store := history.NewStore(abs)
	ctx := context.Background()
	if *inspect != "" {
		s, err := store.Read(ctx, *inspect)
		must(err)
		writeSummary(s, *output)
		return
	}
	s, err := store.Create(ctx, history.CreateRequest{Name: "长局容量隔离验收", ScenarioID: "song-1127", CommandID: "long-fixture-create"})
	must(err)
	p := &history.WorldProposal{}
	for i := 0; i < 700; i++ {
		id := fmt.Sprintf("former-%03d", i)
		note := fmt.Sprintf("旧任文书第%d号。", i) + strings.Repeat("已离任，原任内公开账册均已交割，无现行差事；资料保留供后任查考。", 12)
		if i == 699 {
			note = "旧任文书第699号保存的四字原文为古井晴川。此记录公开可查。" + note
		}
		p.NewEntities = append(p.NewEntities, history.Entity{ID: id, Kind: "person", Name: history.Text{ZH: fmt.Sprintf("旧任文书%d", i), EN: fmt.Sprintf("Former clerk %d", i)}, Status: "retired", Attributes: map[string]history.Text{"record": {ZH: note, EN: "Retired clerk; original public records remain available."}}, Audience: []string{"public"}})
		if i < 260 {
			p.NewAccounts = append(p.NewAccounts, history.Account{ID: fmt.Sprintf("closed-account-%03d", i), Name: history.Text{ZH: "旧任零余额账户", EN: "Former clerk zero account"}, Owner: id, Resource: "cash", Audience: []string{"public"}})
		}
	}
	r, err := store.Apply(ctx, s.ID, history.Command{ID: "long-fixture-archives", Kind: "advance", ExpectedRevision: s.Revision, Quarter: &history.QuarterPlan{World: p, Narrative: "隔离容量数据准备：旧任档案全部保留。"}})
	must(err)
	s = r.State
	for batch := 0; batch < 5; batch++ {
		orders := []history.OrderIntent{}
		for n := 0; n < 30; n++ {
			i := batch*30 + n
			orders = append(orders, history.OrderIntent{Action: "create", Kind: "project", Text: fmt.Sprintf("公开文书第%d号：等到第50回合后才由现有人手核对，不支出、不采购。本季保持等待。", i), MinisterID: "li_gang", NotBeforeTurn: 50, Limits: map[string]int64{"cash": 0, "grain": 0}})
		}
		r, err = store.Apply(ctx, s.ID, history.Command{ID: fmt.Sprintf("long-fixture-orders-%d", batch), Kind: "edict", ExpectedRevision: s.Revision, Edict: &history.EdictPlan{Text: "登记分批到期核对的公开文书", Orders: orders}})
		must(err)
		s = r.State
	}
	writeSummary(s, *output)
}
func writeSummary(s history.State, output string) {
	b, err := json.MarshalIndent(map[string]any{"id": s.ID, "revision": s.Revision, "turn": s.Turn, "entities": len(s.World.Entities), "accounts": len(s.World.Accounts), "orders": len(s.World.Orders)}, "", "  ")
	must(err)
	must(os.WriteFile(output, b, 0600))
	fmt.Println(string(b))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
