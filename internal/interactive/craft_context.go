package interactive

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/cultivation"
)

// CraftMethodsContext is the narrator's per-turn page of the craft methods this
// branch can really work.
//
// It exists for the same reason the world actions page does: the craft module
// names a method by id, and the narrator cannot invent an id the rules accept.
// Without this page a natural input like 炼一炉丹 has nothing to become, and the
// module stays a shape nothing fills.
//
// It is an index, not permission. Every line states the method's real cost, its
// real yield, its loss rate and the stage it needs, together with how much of
// each material the character actually carries — the same facts the commit
// reads — so a refusal the narrator could have foreseen is never a surprise.
// Whether the batch is allowed is still the rules' answer.
//
// The page is bounded by dropping whole methods from the tail, never by
// truncating one, because a half-written method is worse than an absent one.
//
// A character who has reached no method's stage gets no page at all. Every line
// would then read "not reached", which is prompt budget spent on every turn of a
// story that cannot craft yet, and the one fact it conveyed — "not yet" — is
// already carried by the character's own stage in the state handbook.
func CraftMethodsContext(system StoryDirectorActorStateSystem, state map[string]any, actorID string, limitBytes int) string {
	if limitBytes <= 0 {
		limitBytes = StoryContextMaxBytes
	}
	if actorID = normalizeStatePanelActorID(actorID); actorID == "" {
		actorID = DefaultActorID
	}
	stage, stageOK := cultivation.ParseStageKey(xiuxianStoredStageKey(state, actorID), system.CultivationRules)
	if !stageOK {
		return ""
	}
	registry := storyCraftRegistryFromState(system, state)
	ids, omitted := craftContextMethodIDs(system, state, actorID)
	if len(ids) == 0 {
		return ""
	}
	reachable := false
	for _, id := range ids {
		if method, ok := registry.Method(id); ok && stageReaches(stage, method.MinimumStage) {
			reachable = true
			break
		}
	}
	if !reachable {
		return ""
	}
	catalog := storyItemCatalog(system.CultivationRules,
		state)
	held := craftHeldByDef(craftHeldFromState(system.CultivationRules,
		state, actorID))

	// The block heading is supplied by the prompt writer, so this page carries
	// only its own body. Printing the heading here would put it in the prompt
	// twice and spend budget saying the same thing.
	header := "只有玩家实际投入制作时间时才提交 craft：填写下列 method_id 与玩家要求的 duration_seconds。允许分段制作：开工扣除该批材料并保存实际工时，未完工没有成品；下次同一method_id自动续作，duration_seconds只填本次新增投入。暂停、读档或重开不会自动推进时间或跨夜，也不会自动报废；一小时是3600秒，一时辰是两小时，时间叙述以本次正式推进为准。正式进度可以继续；历史正文或人物关于“一口气做完”“断了就废”的说法不能覆盖当前规则，不能以墨干、断气等叙事自行判废。不能把已付材料再卖掉、重复扣除或虚构完成。完整批次、材料消耗、失败及成品由规则决定，不自行写产物或数量，也不要同时改重要物品。制作不与修炼、休养、世界行动共用一个时间段。要求学习的方法先经 skills.skills 的 def_id 与真实 source 学会，不能因列表存在就视为掌握。\n"
	var sb strings.Builder
	sb.WriteString(header)
	for _, id := range ids {
		method, ok := registry.Method(id)
		if !ok {
			continue
		}
		entry := craftMethodLine(method, catalog, held, stage)
		if work, ok := craftWorkFor(state, actorID, method.ID); ok {
			entry += fmt.Sprintf("    进行中：已投入%d/%d分钟，本批材料已经扣除；继续本方法只投入剩余工时，不需要再备同一批材料。\n", work.WorkedMinutes, method.MinutesPerBatch)
		}
		if method.Source != "" {
			entry += fmt.Sprintf("    来源：%s；工具：%v；地点：%s；失败消耗本次投入材料。\n", method.Source, method.RequiredTools, method.RequiredLocation)
		}
		if method.RequiresLearning {
			status := "未学会"
			if craftMethodKnown(state, actorID, method) {
				status = "已学会"
			}
			entry += "    学习状态：" + status + "；经 skills 正式学习后才能制作。\n"
		}
		if sb.Len()+len(entry) > limitBytes {
			break
		}
		sb.WriteString(entry)
	}
	if sb.Len() == len(header) {
		// Every method was dropped for budget. Saying so is better than an empty
		// page that reads as "this world holds no craft".
		sb.WriteString("- 本回合的上下文容量不足以列出完整方法。\n")
	}
	if omitted > 0 {
		note := fmt.Sprintf("\n另有%d项生成方法未在本轮展开；档案保留，可用search_generated_definitions按名称查找，再用稳定method_id取用。", omitted)
		if sb.Len()+len(note) <= limitBytes {
			sb.WriteString(note)
		}
	}
	return strings.TrimSpace(sb.String())
}

// craftMethodLine writes one method as one line of ids, numbers and the
// character's real position against them.
func craftMethodLine(
	method cultivation.CraftMethodDef,
	catalog *cultivation.ItemCatalog,
	held map[string]int,
	stage cultivation.Cultivation,
) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- method_id `%s` (v%d, %s, %s): 每批 %d 分钟",
		method.ID, method.Version, craftArtLabel(method.Art), method.NameKey, method.MinutesPerBatch)
	if method.LossChance > 0 {
		fmt.Fprintf(&sb, "; 失败率 %.0f%%", method.LossChance*100)
	}
	sb.WriteString(".\n")
	stageState := method.MinimumStage
	if stageReaches(stage, method.MinimumStage) {
		stageState += ", 已达到"
	} else {
		stageState += ", 未达到"
	}
	fmt.Fprintf(&sb, "    境界 %s；消耗 ", stageState)
	for index, input := range method.Inputs {
		if index > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%d x `%s` (%s, 持有 %d)",
			input.Quantity, input.DefID, craftDefLabel(catalog, input.DefID), held[input.DefID])
	}
	sb.WriteString("；产出 ")
	for index, output := range method.Outputs {
		if index > 0 {
			sb.WriteString(", ")
		}
		if output.Chance >= 1 {
			fmt.Fprintf(&sb, "%d x `%s` (%s)", output.Quantity, output.DefID, craftDefLabel(catalog, output.DefID))
			continue
		}
		fmt.Fprintf(&sb, "%d x `%s` (%s)，概率 %.0f%%",
			output.Quantity, output.DefID, craftDefLabel(catalog, output.DefID), output.Chance*100)
	}
	sb.WriteString(".\n")
	return sb.String()
}

// craftDefLabel names a definition without inventing a wording for it: the
// catalogue's own localisation key is the only name the server can publish.
func craftDefLabel(catalog *cultivation.ItemCatalog, defID string) string {
	if definition, ok := catalog.Definition(defID); ok && strings.TrimSpace(definition.NameKey) != "" {
		return definition.NameKey
	}
	return defID
}

func craftArtLabel(art cultivation.CraftArt) string {
	switch art {
	case cultivation.CraftArtAlchemy:
		return "炼丹"
	case cultivation.CraftArtSmithing:
		return "炼器"
	case cultivation.CraftArtTalisman:
		return "制符"
	case cultivation.CraftArtFormation:
		return "阵法"
	}
	return string(art)
}

// craftHeldByDef totals the carried copies of each material definition. Two
// stacks of the same herb are one affordability question, which is how the rules
// read them.
func craftHeldByDef(held []cultivation.CraftHeld) map[string]int {
	totals := make(map[string]int, len(held))
	for _, entry := range held {
		totals[entry.DefID] += entry.Quantity
	}
	return totals
}

// Select identities before resolving any generated mechanics. The prompt does
// not need to rebuild thousands of old recipes to show a small useful page.
func craftContextMethodIDs(system StoryDirectorActorStateSystem, state map[string]any, actorID string) ([]string, int) {
	base := storyCraftRegistry(system).IDs()
	sort.Strings(base)
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	learned, _ := xiuxianEquipmentEntry(state, actorID, xiuxianFieldLearnedSkills)
	known := map[string]bool{}
	for _, value := range learned {
		record, _ := value.(map[string]any)
		known[xiuxianEntryText(record, "定义")] = true
	}
	archived := generatedRecordIDs(records, "craft")
	selected := []string{}
	for _, id := range archived {
		if known[id] && len(selected) < generatedContextDefinitionLimit {
			selected = append(selected, id)
		}
	}
	included := map[string]bool{}
	for _, id := range selected {
		included[id] = true
	}
	for _, id := range archived {
		if !included[id] && len(selected) < generatedContextDefinitionLimit {
			selected = append(selected, id)
			included[id] = true
		}
	}
	return append(selected, base...), len(archived) - len(selected)
}
