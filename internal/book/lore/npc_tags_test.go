// G28: the lore index is a real search field, so changing tags must be measured
// rather than argued about.
//
// For each person this asks the index the two questions a player and the model
// actually ask: "find this person by name" and "find whoever works with this
// craft". It runs both against the pack as HEAD has it and against the rewritten
// pack, so a tag rewrite that drops a person out of a search fails here instead
// of surfacing as an unexplained hole later.
//
// The pack is read here directly rather than through internal/storypack, because
// that package imports this one; the fields copied into Item are the same ones
// storypack.Pack.WorldInputs copies.
package lore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type npcPackFile struct {
	ContentVersion int `json:"content_version"`
	World          []struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Type             string   `json:"type"`
		Importance       string   `json:"importance"`
		Tags             []string `json:"tags"`
		Keywords         []string `json:"keywords"`
		LoadMode         string   `json:"load_mode"`
		BriefDescription string   `json:"brief_description"`
		Content          string   `json:"content"`
	} `json:"world"`
	Characters []struct {
		SourceKey    string `json:"source_key"`
		Name         string `json:"name"`
		Role         string `json:"role"`
		Level        string `json:"level"`
		InitialStage string `json:"initial_stage"`
		LoreID       string `json:"lore_id"`
		Brief        string `json:"brief"`
	} `json:"characters"`
}

func readNPCProbe(t *testing.T, data []byte, label string) ([]Item, npcPackFile) {
	t.Helper()
	var pack npcPackFile
	if err := json.Unmarshal(data, &pack); err != nil {
		t.Fatalf("解析 %s：%v", label, err)
	}
	items := make([]Item, 0, len(pack.World))
	for _, entry := range pack.World {
		items = append(items, Item{
			ID: entry.ID, Type: entry.Type, Name: entry.Name, Importance: entry.Importance,
			Tags: entry.Tags, Keywords: entry.Keywords, LoadMode: entry.LoadMode,
			BriefDescription: entry.BriefDescription, Content: entry.Content, Enabled: true,
		})
	}
	return items, pack
}

func currentPackBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "storypack", "assets", "fanren-jie.json"))
	if err != nil {
		t.Fatalf("读剧本包：%v", err)
	}
	return data
}

// previousPackBytes returns the pack as it stood before the tag rewrite, from a
// fixture checked in beside the test rather than from git.
//
// It used to run `git show HEAD:...`, which after the rewrite was committed read
// the rewritten pack itself: the "before" side became a copy of the "after" side
// and the comparison could no longer fail. It also skipped outright in an
// exported tree with no git, so the claim rested on an environment rather than
// on evidence. The fixture is the pre-rewrite revision ff799107f reduced to the
// fields the index reads; it is fixed content, so the test asks the same
// question in a clean checkout, in CI, and in an exported tarball.
func previousPackBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pack-before-g28.json"))
	if err != nil {
		t.Fatalf("读改标签前的基线剧本包：%v", err)
	}
	return data
}

func findByName(items []Item, name string) []Item {
	matches, _, _ := filterLoreIndexEntries(items, IndexOptions{
		Keywords: []string{name}, Match: IndexMatchAny, Limit: 5,
	})
	result := make([]Item, 0, len(matches))
	for _, match := range matches {
		result = append(result, match.Item)
	}
	return result
}

func TestTagRewriteKeepsEveryPersonFindableByName(t *testing.T) {
	before, _ := readNPCProbe(t, previousPackBytes(t), "上一版")
	after, pack := readNPCProbe(t, currentPackBytes(t), "当前版")

	// Entries may be added — a world grows by carrying more than the last
	// revision did — but none may be dropped, because a person the previous
	// revision could find and this one cannot is a hole the rewrite dug.
	if len(after) < len(before) {
		t.Fatalf("条目数 %d -> %d：改标签不该删条目", len(before), len(after))
	}
	if pack.ContentVersion < 7 {
		t.Fatalf("改过作者源之后 content_version 仍是 %d", pack.ContentVersion)
	}

	// Aligned by name rather than by position: an inserted entry shifts every
	// index after it, and a positional comparison would then report every later
	// entry as a rename instead of the one insertion that really happened.
	beforeByName := make(map[string]Item, len(before))
	for _, item := range before {
		beforeByName[item.Name] = item
	}

	lost, moved := []string{}, []string{}
	for _, newItem := range after {
		oldItem, existed := beforeByName[newItem.Name]
		if !existed {
			// This entry is new in this revision; there is no earlier wording to
			// compare it against, and it must still be findable on its own.
			if len(findByName(after, newItem.Name)) == 0 {
				lost = append(lost, newItem.Name+"（本轮新增）")
			}
			continue
		}
		oldHits := findByName(before, oldItem.Name)
		newHits := findByName(after, newItem.Name)
		if len(oldHits) == 0 || len(newHits) == 0 {
			lost = append(lost, oldItem.Name)
			continue
		}
		if oldHits[0].Name != newHits[0].Name {
			moved = append(moved, fmt.Sprintf("%s：%s -> %s", oldItem.Name, oldHits[0].Name, newHits[0].Name))
		}
	}
	for _, item := range before {
		if _, kept := findByNameItem(after, item.Name); !kept {
			lost = append(lost, item.Name+"（本轮被删）")
		}
	}
	if len(lost) > 0 {
		t.Fatalf("按姓名检索不到：%s", strings.Join(lost, "、"))
	}
	if len(moved) > 0 {
		t.Fatalf("按姓名检索的首位结果变了：%s", strings.Join(moved, "；"))
	}
}

// findByNameItem reports whether an entry of this name still exists at all,
// which is the question a deletion asks. findByName answers a different one:
// what the index returns for a name query.
func findByNameItem(items []Item, name string) (Item, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return Item{}, false
}

// TestTagRewriteKeepsCraftWordsSearchable asks the question a player asks when
// they do not know anyone's name: "who here works with pills / arrays / beasts".
// The old tags carried the pack's own topic words and were the only thing that
// matched such a query, so the rewrite has to keep those people reachable.
func TestTagRewriteKeepsCraftWordsSearchable(t *testing.T) {
	after, _ := readNPCProbe(t, currentPackBytes(t), "当前版")

	probes := []struct {
		word  string
		names []string
	}{
		{"丹药", []string{"苏浅", "钱守信", "青霞真人"}},
		{"药材", []string{"马师伯", "苗细细", "孙氏"}},
		{"阵法", []string{"宋观澜", "辛如音"}},
		{"御兽", []string{"谢青黛", "钟吾", "菡云芝"}},
		{"剑", []string{"宋寒衣", "无名剑客", "顾铮"}},
		{"琴", []string{"银汐"}},
		{"符", []string{"王婆子"}},
	}

	for _, probe := range probes {
		matches, _, _ := filterLoreIndexEntries(after, IndexOptions{
			Keywords: []string{probe.word}, Match: IndexMatchAny, Limit: 60,
		})
		found := map[string]bool{}
		for _, match := range matches {
			found[match.Item.Name] = true
		}
		for _, name := range probe.names {
			if !found[name] {
				t.Errorf("检索「%s」不再返回 %s", probe.word, name)
			}
		}
	}
}

// TestEveryPersonHasTheirOwnTagsAndIntroduction pins the shape the author asked
// for: nobody is introduced by a role-and-place template, and nobody is filed
// under only the pack's generic routing words.
func TestEveryPersonHasTheirOwnTagsAndIntroduction(t *testing.T) {
	items, pack := readNPCProbe(t, currentPackBytes(t), "当前版")
	generic := map[string]bool{
		"人物交往": true, "势力正式事务": true, "弟子日常": true, "接引": true,
		"家族来往": true, "坊市问询": true, "凡俗生活": true, "游历见闻": true, "执法": true,
	}

	seenBrief := map[string]string{}
	problems := []string{}
	people := 0
	for _, item := range items {
		if item.Type != "character" {
			continue
		}
		people++
		if len(item.Tags) == 0 {
			problems = append(problems, item.Name+"：没有标签")
		}
		onlyGeneric := true
		for _, tag := range item.Tags {
			if !generic[tag] {
				onlyGeneric = false
			}
		}
		if onlyGeneric {
			problems = append(problems, item.Name+"：只有泛用标签")
		}
		brief := strings.TrimSpace(item.BriefDescription)
		if brief == "" {
			problems = append(problems, item.Name+"：没有简介")
			continue
		}
		if strings.Contains(brief, "一带）") {
			problems = append(problems, item.Name+"：简介仍是「身份（地域，地点一带）」模板")
		}
		if previous, ok := seenBrief[brief]; ok {
			problems = append(problems, fmt.Sprintf("%s 与 %s 简介完全相同", item.Name, previous))
		}
		seenBrief[brief] = item.Name
		for _, banned := range []string{"提示词", "字段", "配置", "模型", "source_key", "lore_id"} {
			if strings.Contains(brief, banned) {
				problems = append(problems, item.Name+"：简介含工程用语 "+banned)
			}
		}
	}
	if people != len(pack.Characters) {
		problems = append(problems, fmt.Sprintf("正文里有 %d 人，世界条目里有 %d 个角色", len(pack.Characters), people))
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("简介或标签不合格：\n%s", strings.Join(problems, "\n"))
	}
}

// TestTheIntroductionAgreesWithTheBody keeps a rewritten introduction from
// quietly promoting or demoting someone: the stage it names, the role it names
// and the entry's own body have to be the same person at the same cultivation
// level. Only the wording was meant to change.
func TestTheIntroductionAgreesWithTheBody(t *testing.T) {
	_, pack := readNPCProbe(t, currentPackBytes(t), "当前版")
	byID := map[string]string{}
	for _, entry := range pack.World {
		byID[entry.ID] = entry.Content
	}

	problems := []string{}
	for _, person := range pack.Characters {
		stage := strings.TrimSpace(person.InitialStage)
		body := byID[person.LoreID]
		if stage == "" {
			problems = append(problems, person.Name+"：没有 initial_stage")
			continue
		}
		if match := regexp.MustCompile(`境界：([^；。\n]+)`).FindStringSubmatch(body); match != nil {
			if strings.TrimSpace(match[1]) != stage {
				problems = append(problems, fmt.Sprintf("%s：正文境界 %q，initial_stage %q", person.Name, match[1], stage))
			}
		}
		if match := regexp.MustCompile(`身份：([^；。\n]+)`).FindStringSubmatch(body); match != nil {
			stated := strings.TrimSpace(match[1])
			if stated != person.Role {
				problems = append(problems, fmt.Sprintf("%s：正文身份 %q，目录身份 %q", person.Name, stated, person.Role))
			}
		}
		// If the introduction names a cultivation level, it has to be this
		// person's level and not a neighbouring one.
		if match := regexp.MustCompile(`(凡人|炼气|筑基|结丹|元婴|化神)([一二三四五六七八九十\d]+层|初期|中期|后期|大圆满)`).FindStringSubmatch(person.Brief); match != nil {
			tier, rest := match[1], match[2]
			if !strings.HasPrefix(stage, tier) {
				problems = append(problems, fmt.Sprintf("%s：简介说 %s，目录说 %s", person.Name, match[0], stage))
				continue
			}
			tail := strings.TrimPrefix(stage, tier)
			if strings.HasSuffix(rest, "层") {
				if stageNumber(rest) != stageNumber(tail) {
					problems = append(problems, fmt.Sprintf("%s：简介说 %s，目录说 %s", person.Name, match[0], stage))
				}
			} else if !strings.Contains(tail, rest) {
				problems = append(problems, fmt.Sprintf("%s：简介说 %s，目录说 %s", person.Name, match[0], stage))
			}
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("简介与正文不一致：\n%s", strings.Join(problems, "\n"))
	}
}

// stageNumber reads "十二层" and "12层" as the same number.
func stageNumber(text string) int {
	text = strings.TrimSuffix(strings.TrimSpace(text), "层")
	if value, err := strconv.Atoi(text); err == nil {
		return value
	}
	digits := map[rune]int{'零': 0, '一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	runes := []rune(text)
	switch {
	case len(runes) == 0:
		return -1
	case len(runes) == 1 && runes[0] == '十':
		return 10
	case len(runes) == 1:
		if value, ok := digits[runes[0]]; ok {
			return value
		}
		return -1
	case runes[0] == '十':
		return 10 + digits[runes[1]]
	case len(runes) >= 2 && runes[1] == '十':
		value := digits[runes[0]] * 10
		if len(runes) > 2 {
			value += digits[runes[2]]
		}
		return value
	}
	return -1
}

// TestThePersonCardAndTheNarratorAgree keeps the two surfaces that show a
// person's introduction from drifting: the catalog row handed to the narrator
// and the lore entry the player can open.
func TestThePersonCardAndTheNarratorAgree(t *testing.T) {
	_, pack := readNPCProbe(t, currentPackBytes(t), "当前版")
	entryBrief := map[string]string{}
	for _, entry := range pack.World {
		entryBrief[entry.ID] = entry.BriefDescription
	}
	for _, person := range pack.Characters {
		if person.Brief != entryBrief[person.LoreID] {
			t.Errorf("%s：候选卡简介与资料条目简介不一致", person.Name)
		}
	}
}
