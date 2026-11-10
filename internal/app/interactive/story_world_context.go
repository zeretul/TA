package interactiveapp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"ta/internal/interactive"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// worldActionsContext is the narrator's per-turn page of the world actions this
// turn can really settle.
//
// It exists for one reason: the turn module names a route, a task, an event, a
// realm, a connection or a reward by id, and the narrator cannot invent an id
// that the rules will accept. Without this page the natural input 去坊市 has no
// way to become a journey, and the module stays a shape nothing can fill.
//
// It is not a menu and it is not knowledge. Every line is an id, a name and the
// state that id is really in — the same list the entry would offer and the same
// predicates the commit applies — so a refusal the narrator could have foreseen
// is never a surprise. The prose still decides what the player does; this page
// only lets the settlement be named.
//
// The page is bounded by dropping whole entries from the tail, never by
// truncating one, because a half-written id is worse than an absent entry.
func (c *Conversation) storyWorldActionsContext(storyCtx interactive.StoryContext, query ...string) string {
	if c == nil || c.store == nil {
		return ""
	}
	var state interactive.WorldState
	var err error
	target := c.regenerateTargetSnapshot()
	if target != "" {
		state, err = c.store.WorldStateAtTurnParent(c.storyID, storyCtx.Snapshot.BranchID, target)
	} else {
		state, err = c.store.WorldStateFor(c.storyID, storyCtx.Snapshot.BranchID)
	}
	if err != nil {
		// No page rather than an empty one. An empty page would read as "this
		// world holds nothing you can do", which is a claim about the world; the
		// failure is about the install, so it is recorded as one and the narrator
		// keeps working from what it already has.
		slog.Warn("[interactive] world actions page withheld", "error", err)
		return ""
	}
	preferred := make(map[string]bool)
	for _, id := range interactive.PlayerAttentionFromState(storyCtx.Snapshot.State).Quests {
		preferred[id] = true
	}
	// Reorder only the already-known lists. Unknown offer candidates keep their
	// original eligibility and are never added by a player's preference.
	sort.SliceStable(state.Tasks.Tasks, func(i, j int) bool {
		return preferred[state.Tasks.Tasks[i].TaskID] && !preferred[state.Tasks.Tasks[j].TaskID]
	})
	sort.SliceStable(state.Tasks.Available, func(i, j int) bool {
		return preferred[state.Tasks.Available[i].TaskID] && !preferred[state.Tasks.Available[j].TaskID]
	})
	hazards := worldPresentationContext(storyCtx.Meta.PackBinding, c.outputLanguage)
	if actor, err := interactive.AdventureActorFromState(storyCtx.Snapshot.State, interactive.DefaultActorID); err == nil {
		hazards += "本局使用题材历练结算。装备、卸装、使用物品、休息、制作与部署均用 world.action=adventure，具体动作放 world.adventure；不要提交凡人的顶层 items/rest/craft/cultivation/skills。这条规则优先于旧回合的模块示例。\n"
		hazards += "装备示例 world={action:adventure,source_text:玩家原话,adventure:{action:equip,item_id:持有物品目录ID}}；休息填 adventure:{action:rest,hours:授权小时}；部署或收回填 adventure:{action:deploy或withdraw,item_id:机械成品ID}；维修填 adventure:{action:repair,target_id:机械成品ID,skill_id:已学调校ID,hours:授权小时}。这些正式行动不能用 world.action=none 或顶层 items/rest 代替。\n"
		for _, h := range actor.OpeningHazards {
			if !h.Resolved {
				hazards += fmt.Sprintf("- 开局时限：%s；正式到期秒 %d，剩余 %d 秒。等待至此会触发后果，跨过时限的整段训练或制作会被拒绝；可以先离开危险区域。\n", h.Description, h.DueSeconds, max(int64(0), h.DueSeconds-state.Clock.Seconds))
			} else if h.Outcome == "exposed" {
				hazards += "- 已发生的开局后果：" + h.Consequence + "\n"
			}
		}
	}
	affairs := ""
	var affairErr error
	if interactive.LocalAffairsSupported(storyCtx.Meta) {
		affairs, affairErr = c.store.LocalAffairsNarratorContext(c.storyID, storyCtx.Snapshot.BranchID, target, query...)
	}
	if affairErr != nil {
		slog.Warn("[interactive] local affair context withheld", "error", affairErr)
	}
	remaining := interactive.StoryContextMaxBytes - len(hazards) - len(affairs) - 2
	world := ""
	if remaining > 0 {
		world = renderWorldActions(state, remaining)
	}
	return boundedWorldContext(hazards+world+"\n\n"+affairs, interactive.StoryContextMaxBytes)
}

// renderWorldActions writes the page, measuring each entry before adding it.
//
// The page is assembled on every turn, including turns that settle nothing, so
// it is written in the narrator's own shorthand rather than as an essay: the
// ids and the action that settles them are what the model cannot derive, and
// everything else is kept to one line. Empty categories are named in a single
// line instead of one "do not invent" paragraph each, which is where this page
// used to spend most of its budget.
func renderWorldActions(state interactive.WorldState, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = interactive.StoryContextMaxBytes
	}
	var sb strings.Builder
	sb.WriteString("## World Actions (source: the same registries the turn settlement validates against)\n\n")
	if !worldHasAnything(state) {
		sb.WriteString("本页没有可用行路、事务、事件或探索条目。按本局其他规则结算适用的行动，包括已配置题材的买卖、学习、训练、劳作和制作。只有本轮没有正式世界行动时才提交 world.action=none。\n")
		return boundedWorldContext(sb.String(), maxBytes)
	}
	sb.WriteString("每回合提交 world 模块。行路、事务、事件和探索使用本页可用的 ID；题材买卖、劳作、学习、训练和制作使用本局冻结题材目录。每回合最多结算一种正式世界行动；闲谈、看价和没有正式世界行动的回合提交 action=none。 玩家说等一会儿但未给时长时可追问；已明确等一个小时就提交wait和3600秒，不能只写时间标签。一小时或一钟头为3600秒，一时辰为7200秒，正文保留玩家原本单位和实际时长。\n")
	fmt.Fprintf(&sb, "- Formal time %s (%ds); 旅程、正式活动、事务处理与探索方法按已登记耗时推进；查看不推进。\n", worldClockText(state.Clock), state.Clock.Seconds)
	if location := strings.TrimSpace(state.Location); location != "" {
		if inside, scene := worldSceneInsideRealm(state); inside {
			// 当前详细地点 still holds the entrance the character came in from,
			// because that is what a realm exit restores. Printing it as the scene
			// told the narrator the character stood outside while the instance said
			// inside, and one live turn then reconciled the two by narrating a room
			// the world never entered.
			fmt.Fprintf(&sb, "- Scene %s. The character is inside this realm; %s is the entrance they came from and is restored when they leave. Do not write 当前详细地点 to a place the realm did not move them to.\n", scene, location)
		} else {
			fmt.Fprintf(&sb, "- Scene %s. A road is offered only from here, so do not write 当前详细地点 in a turn that walks one.\n", location)
		}
	}
	if absent := worldAbsentCategories(state); absent != "" {
		fmt.Fprintf(&sb, "- Nothing offered here: %s. Do not invent one in prose.\n", absent)
	}
	sb.WriteString("\n")

	writeWorldMembershipContext(&sb, state, min(maxBytes, sb.Len()+maxBytes/3))
	inside, _ := worldSceneInsideRealm(state)
	if inside {
		writeWorldRealms(&sb, state, maxBytes)
	}
	writeWorldLocalAffairs(&sb, state.LocalAffairs, maxBytes)
	writeWorldFactionAffairs(&sb, state.FactionAffairs, maxBytes)
	writeWorldCooperations(&sb, state.Realms.Cooperations, maxBytes)
	writeWorldJourneys(&sb, state, maxBytes)
	writeWorldTasks(&sb, state, maxBytes)
	writeWorldEvents(&sb, state, maxBytes)
	if !inside {
		writeWorldRealms(&sb, state, maxBytes)
	}
	return boundedWorldContext(sb.String(), maxBytes)
}

// Keep complete lines and UTF-8 text when even the base scene description is
// larger than a caller's budget. Never return a clipped identifier.
func boundedWorldContext(page string, maxBytes int) string {
	if len(page) > maxBytes {
		const omitted = "\n- 其余条目因篇幅省略（omitted for length）。"
		if maxBytes < len(omitted) {
			return ""
		}
		last := strings.LastIndexByte(page[:maxBytes-len(omitted)], '\n')
		if last < 0 {
			return strings.TrimSpace(omitted)
		}
		page = strings.TrimSpace(page[:last]) + omitted
	}
	return strings.TrimSpace(page)
}

// worldAbsentCategories names the action kinds this turn cannot settle, in one
// line. It keeps the narrator from staging a road or an event the world does not
// hold without spending a paragraph saying so.
func worldAbsentCategories(state interactive.WorldState) string {
	var absent []string
	if len(state.Journeys.Routes) == 0 && len(state.ActiveJourneys) == 0 {
		absent = append(absent, "no road starts here")
	}
	if len(state.Events.Events) == 0 {
		absent = append(absent, "no public event is known here")
	}
	if len(state.Realms.Available) == 0 && len(state.Realms.Instances) == 0 {
		absent = append(absent, "no realm can be entered here")
	}
	return strings.Join(absent, "; ")
}

// worldHasAnything reports whether any world action could be settled right now.
//
// A task already taken is not itself an action — its objective still needs a
// fact, and that fact is the action — but its state is what the narrator needs
// in order to know whether the turn finished it, so a taken task keeps the full
// page.
func worldHasAnything(state interactive.WorldState) bool {
	return len(state.FactionMemberships) > 0 || len(state.MembershipContacts) > 0 || len(state.LocalAffairs) > 0 || len(state.Journeys.Routes) > 0 || len(state.ActiveJourneys) > 0 ||
		len(state.Tasks.Available) > 0 || len(state.Tasks.Tasks) > 0 || len(state.Tasks.OfferCandidates) > 0 ||
		len(state.Events.Events) > 0 ||
		len(state.FactionAffairs.Projects) > 0 || len(state.FactionAffairs.Recipes) > 0 || len(state.Realms.Available) > 0 || len(state.Realms.Instances) > 0 || len(state.Realms.Cooperations) > 0
}

// worldSceneInsideRealm reports whether the character stands inside a realm, and
// which room, so the page can say where they really are instead of repeating a
// scene field that still holds the entrance.
func worldSceneInsideRealm(state interactive.WorldState) (bool, string) {
	for _, instance := range state.Realms.Instances {
		if instance.Presence != "inside" {
			continue
		}
		if area := instance.View.CurrentArea; area != nil {
			return true, area.Name + " (" + area.ID + ")"
		}
		return true, instance.Name
	}
	return false, ""
}

func worldClockText(clock interactive.StoryClock) string {
	elapsed := interactive.DescribeElapsed(clock.Seconds)
	anchor := strings.TrimSpace(clock.Anchor)
	switch {
	case anchor != "" && elapsed != "":
		return anchor + " 后 " + elapsed
	case anchor != "":
		return anchor
	case elapsed != "":
		return elapsed
	default:
		return "故事起点"
	}
}

// worldEntry fits one entry into what is left of the budget.
//
// It reports whether the entry was written. A caller stops at the first entry
// that does not fit: dropping a later entry keeps the list a prefix, so the
// narrator reads a complete list of a smaller world rather than a list with a
// hole in the middle.
func worldEntry(sb *strings.Builder, maxBytes int, entry string) bool {
	if sb.Len()+len([]byte(entry)) > maxBytes {
		return false
	}
	sb.WriteString(entry)
	return true
}

func writeWorldJourneys(sb *strings.Builder, state interactive.WorldState, maxBytes int) {
	sb.WriteString("### Roads (action=journey)\n\n")
	if len(state.ActiveJourneys) > 0 {
		ids := make([]string, 0, len(state.ActiveJourneys))
		for id := range state.ActiveJourneys {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(sb, "- Unfinished travel %s, currently at %s. Continue it by omitting route_id.\n", id, state.ActiveJourneys[id])
		}
	}
	if len(state.Journeys.Routes) == 0 {
		// The absence is already named in the page's own one-line summary; a
		// second paragraph saying "do not invent one" is the same instruction.
		sb.WriteString("\n")
		return
	}
	for _, route := range state.Journeys.Routes {
		entry := fmt.Sprintf("- route_id `%s` (version %s): %s → %s, %d minutes on foot by the authored method. Set arrive=true to finish it in one turn, or target_minutes to walk part of it.\n",
			route.RouteID, route.RouteVersion, route.OriginLabel, route.DestinationLabel, route.TotalMinutes)
		if route.Name != "" {
			entry += "    - 发现的路径：" + route.Name + "。耗时按这条路的独立定义结算。\n"
		}
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further roads omitted for length.\n")
			break
		}
	}
	sb.WriteString("\n")
}

func writeWorldTasks(sb *strings.Builder, state interactive.WorldState, maxBytes int) {
	sb.WriteString("### Tasks (action=quest)\n\n")
	wrote := false
	for _, task := range state.Tasks.Tasks {
		// An offered task is recorded but not taken. Calling it "taken" and
		// listing only complete/settle left the narrator no way to accept it: a
		// live turn where the player said 这桩差事我接了 wrote the promise into
		// prose and settled nothing, because the page had never told the model
		// that accept was the action for this state.
		offered := task.Status == string(quest.StatusOffered)
		standing := "taken"
		if offered {
			standing = "offered, not taken"
		}
		entry := fmt.Sprintf("- task_id `%s`, %s, status %s: %s — %s\n", task.TaskID, standing, task.Status, task.Title, task.Brief)
		// The deadline is stated as the panel states it, because the narrator has
		// to be able to warn the player before a long action crosses it. A task
		// with no agreed limit says so rather than leaving the narrator to guess,
		// and an expired one is not described as still running.
		switch {
		case offered:
			// No deadline is stamped until acceptance, so promising one now would
			// invent a countdown the rules have not started.
			entry += "    - Not taken yet. Set quest_action=accept when the player takes it; doing the work does not accept it on their behalf.\n"
		case task.Expired:
			entry += "    - 约定已到期: the agreed moment passed before the work was finished. Do not promise it can still be handed in as agreed.\n"
		case task.DeadlineSeconds > 0:
			entry += fmt.Sprintf("    - Agreed deadline: %s.\n", task.DeadlineText)
		default:
			entry += "    - 未约定期限: no deadline was agreed.\n"
		}
		for _, objective := range task.Objectives {
			mark := "未完成"
			if objective.Completed {
				mark = "已完成"
			}
			entry += fmt.Sprintf("    - objective `%s` (%s): %s\n", objective.ID, mark, objective.Description)
		}
		for _, followUp := range task.FollowUps {
			entry += fmt.Sprintf("    - 旧事可向本场人物 %s 核实：quest_action=follow_up，quest_choice_id=%q。只有玩家实际回访询问才结算；未核实的回答不能先写成已知，新机会也不自动接下。\n", followUp.CharacterID, followUp.ChoiceID)
		}
		for _, message := range task.Consequences {
			entry += "    - 已发生的后果：" + message + "\n"
		}
		if task.Status == string(quest.StatusAccepted) || task.Status == string(quest.StatusCompleted) {
			entry += worldQuestChoices(task.Choices, task.Resolutions)
		}
		if offered {
			// Nothing else applies to an offer: it cannot be completed or paid
			// before acceptance, so those lines would teach refusals.
		} else if task.Settled {
			entry += "    - Already settled: the payment was handed over. Do not pay it again.\n"
		} else if task.Status == string(quest.StatusCompleted) {
			// Every objective is already done, so complete is refused; naming it
			// here would teach the narrator a refusal it cannot foresee.
			entry += "    - Finished but unpaid. quest_action=settle hands over the agreed payment, once.\n"
		} else if task.Status == string(quest.StatusAccepted) || task.Status == string(quest.StatusCompleted) {
			entry += "    - Not yet settled. quest_action=complete records a finished goal once every objective above is done; quest_action=settle hands over the payment, once.\n"
		} else {
			entry += fmt.Sprintf("    - Status %s: neither complete nor settle applies. Do not offer the agreed payment as still available.\n", task.Status)
		}
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further taken tasks omitted for length.\n")
			return
		}
		wrote = true
	}
	for _, task := range state.Tasks.OfferCandidates {
		entry := fmt.Sprintf("- Potential personal matter from present giver %s: task_id `%s` — %s. This is NOT player knowledge or an existing offer. Only if the scene actually discloses this matter, record quest_action=offer. Do not accept it before the player has heard the offer and chosen to take it.\n", task.Giver, task.TaskID, task.Brief)
		if !worldEntry(sb, maxBytes, entry) {
			return
		}
		wrote = true
	}
	for _, task := range state.Tasks.Available {
		entry := fmt.Sprintf("- task_id `%s` (version %s), not yet taken: %s — %s", task.TaskID, task.TaskVersion, task.Title, task.Brief)
		if giver := strings.TrimSpace(task.Giver); giver != "" {
			entry += fmt.Sprintf(" Offered by %s.", giver)
		}
		// The terms are stated before acceptance, so the player can see what they
		// are agreeing to. The countdown starts at acceptance, not now.
		if task.DeadlineSeconds > 0 {
			entry += fmt.Sprintf(" Takes effect on acceptance and runs %s from that moment.", interactive.DescribeElapsed(task.DeadlineSeconds))
		} else {
			entry += " 未约定期限."
		}
		entry += " Set quest_action=accept when the player takes it.\n"
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further offered tasks omitted for length.\n")
			return
		}
		wrote = true
	}
	if !wrote {
		sb.WriteString("- No task is offered here and none is taken. Do not invent a commission; ordinary errands stay ordinary life and need no record.\n")
	}
	sb.WriteString("\n")
}

func writeWorldEvents(sb *strings.Builder, state interactive.WorldState, maxBytes int) {
	sb.WriteString("### Public events (action=public_event)\n\n")
	if len(state.Events.Events) == 0 {
		sb.WriteString("\n")
		return
	}
	for _, event := range state.Events.Events {
		stage := event.CurrentStageTitle
		if stage == "" {
			stage = "尚未开始"
		}
		entry := fmt.Sprintf("- event_id `%s` (version %s): %s — %s. Region %s, current stage %s.\n",
			event.EventID, event.EventVersion, event.Title, event.Brief, event.Region, stage)
		if event.Closed {
			entry += "    - Closed: no new participation. Do not join it.\n"
		} else if event.Joined {
			state := "took part as " + event.Participation
			if event.Withdrawn {
				state += ", since withdrawn"
			}
			entry += fmt.Sprintf("    - This character already %s at stage %s.\n", state, event.JoinedStage)
		} else {
			entry += "    - 玩家明确选择参与或实施对应活动时才提交 event_action=join。问路、打听或经过此地不等于参加，也不发参与报酬。\n"
		}
		actions := event.Actions
		if event.EventID == publiquevent.PublicEventIDTideGauge {
			entry += "    - 路程使用现代小时（1 时辰等于 2 小时）。十二小时旱路是六个时辰，十一小时外汊是五个半时辰，十小时短渡是五个时辰。以行路的实际耗时和开放状态为准。\n"
			switch {
			case event.Joined:
				actions = nil
				entry += "    - 当前分支已完成渡口事务。此刻可出发的路线及耗时以已列出的行路选项为准。\n"
			case event.CurrentStageID == publiquevent.TideGaugeAfterRise:
				entry += "    - 当前分支错过校准，已无法取得短渡资格。现在可走十二小时旱路，或先请宋老橹复核外汊，复核成功才开放十一小时外汊水路。复核和开船不设额外时限。只询问或查看告示时不提交复核；不得描写复核已完成或水路已开。\n"
			default:
				entry += "    - 校准必须在涨水前完成。当前分支一旦校准成功，十小时短渡涨水后仍可出发；路线和停靠以实际行路选项为准。\n"
			}
			for _, stage := range event.Stages {
				if stage.ID == event.CurrentStageID || stage.ID == event.JoinedStage {
					for _, line := range stage.Lines {
						entry += "    - " + line + "\n"
					}
				}
			}
		}
		for _, action := range actions {
			line := fmt.Sprintf("    - %s (%s): %s\n", action.Kind, action.Label, action.Brief)
			if action.Reward != nil {
				line = fmt.Sprintf("    - %s (%s): %s Pays %d of %s.\n", action.Kind, action.Label, action.Brief, action.Reward.Quantity, action.Reward.RefID)
			}
			entry += line
		}
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further events omitted for length.\n")
			return
		}
	}
	sb.WriteString("\n")
}

func writeWorldRealms(sb *strings.Builder, state interactive.WorldState, maxBytes int) {
	sb.WriteString("### Secret realms (action=realm)\n\n")
	wrote := false
	for _, realm := range state.Realms.Available {
		entry := fmt.Sprintf("- realm_id `%s` (version %s): %s, %s. Enter it with realm_action=enter, which places the character in area `%s`.\n",
			realm.RealmID, realm.RealmVersion, realm.Name, realm.Theme, realm.EntryAreaID)
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further realms omitted for length.\n")
			return
		}
		wrote = true
	}
	instances := append([]interactive.RealmStateView(nil), state.Realms.Instances...)
	sort.SliceStable(instances, func(i, j int) bool { return instances[i].Presence == "inside" && instances[j].Presence != "inside" })
	for _, instance := range instances {
		entry := fmt.Sprintf("- instance_id `%s` of realm_id `%s` (%s), presence %s", instance.InstanceID, instance.RealmID, instance.Name, instance.Presence)
		if instance.View.CurrentArea != nil {
			entry += fmt.Sprintf(", standing in `%s` (%s)", instance.View.CurrentArea.ID, instance.View.CurrentArea.Name)
		}
		entry += ".\n"
		if instance.Presence == "inside" && instance.View.CurrentArea != nil {
			// A live run narrated walking into the formation room and submitted no
			// realm action, so the instance stayed in the outer hall while the prose
			// claimed the new room. The room is the formal fact; saying so here keeps
			// the narrator from treating the scene's own wording as the place.
			entry += "    - The room above is where the character really is. Leaving it is realm_action=move; never narrate arriving in another room without that module.\n"
		}
		for _, area := range instance.View.Areas {
			entry += fmt.Sprintf("    - known area `%s`: %s\n", area.ID, area.Name)
		}
		for _, connection := range instance.View.Connections {
			entry += fmt.Sprintf("    - known connection `%s`: %s → %s\n", connection.ID, connection.FromAreaID, connection.ToAreaID)
		}
		for _, way := range instance.Ways {
			entry += fmt.Sprintf("    - door from this room, connection_id `%s` to destination_area_id `%s` (%s)\n", way.ConnectionID, way.DestinationAreaID, way.DestinationName)
		}
		entry += worldRealmExploration(instance, state.Clock.Seconds)
		for _, objective := range instance.View.Objectives {
			done := "未完成"
			for _, completed := range instance.View.CompletedObjectives {
				if completed == objective.ID {
					done = "已完成"
				}
			}
			entry += fmt.Sprintf("    - objective `%s` (%s): %s\n", objective.ID, done, objective.Description)
		}
		for _, reward := range instance.Rewards {
			entry += fmt.Sprintf("    - content_ref_id `%s` in this room: %s x%d, not yet taken.\n", reward.ContentRefID, reward.NameKey, reward.Quantity)
		}
		for _, granted := range instance.Granted {
			entry += fmt.Sprintf("    - content_ref_id `%s` already taken: %s x%d. The realm does not grow a second copy.\n", granted.ContentRefID, firstNonEmpty(granted.Name, granted.NameKey, granted.RefID), granted.Quantity)
		}
		if instance.Presence == "inside" && instance.Explore != nil {
			entry += "    - 行动只选当前已知方法的realm_action与approach_id；所得自动结算，不另提reward。移动只用此处可通行ways，退出需退回入口。\n"
		} else if instance.Presence == "inside" {
			entry += "    - Use realm_action=move with connection_id and destination_area_id, realm_action=reward with content_ref_id, or realm_action=exit to leave.\n"
		} else {
			entry += "    - Use realm_action=reenter to go back in.\n"
		}
		if !worldEntry(sb, maxBytes, entry) {
			sb.WriteString("- Further known realms omitted for length.\n")
			return
		}
		wrote = true
	}
	if !wrote {
		sb.WriteString("\n")
		return
	}
	sb.WriteString("\n")
}

// worldQuestChoices includes only executable terms. Future echoes are withheld
// until their canonical interval record resolves them, preserving knowledge.
func worldQuestChoices(choices []quest.Choice, resolved map[string]quest.ChoiceResult) string {
	var b strings.Builder
	for _, c := range choices {
		if result, done := resolved[c.ObjectiveID]; done && (result.Outcome != "failure" || !result.EchoResolved) {
			continue
		}
		terms := map[string]any{"地点": c.LocationIDs, "必须实际在场且可参与的角色来源": c.RequiredCharacterSourceKeys, "耗时秒": c.DurationSeconds, "费用": c.CurrencyCost, "借款计息约定": c.Debt, "耗材": c.ItemCosts, "成功后交给实际接收人的实物": c.ItemHandovers, "持有物": c.RequiredItems, "历练等级": c.RequiredRank, "必备技能": c.RequiredSkill, "必备火候": c.RequiredSkillRank, "检定技能": c.SkillID, "难度": c.Difficulty, "先前事务": c.RequiredTasks, "至少完成其中几项（零为全部）": c.RequiredTaskCount, "前置阶段": c.AfterObjectives, "战胜对手": c.RequiredVictory, "部署资产": c.RequiredAsset, "已走完旅程": c.RequiredJourney, "成功交付": c.Rewards}
		data, _ := json.Marshal(terms)
		fmt.Fprintf(&b, "    - 玩家自愿尝试时提交 quest_action=choose, quest_choice_id=%s：%s。条件 %s。成败由当前历练等级加该技能火候与难度比较，失败也花费普通耗材和时间，成功交付的实物在失败时仍由玩家持有，不能叙述补发奖励。失败回响已发生后，只有实际提高历练等级或该技能火候才能补救重试，原失败仍保留。\n", c.ID, c.Description, data)
	}
	return b.String()
}

// writeWorldCooperations consumes only the trusted known-partner projection.
// It does not consult the author's character index or invent available work.
func writeWorldCooperations(sb *strings.Builder, projects []interactive.CooperationProjectView, maxBytes int) {
	for index, project := range projects {
		var entry strings.Builder
		if index == 0 {
			entry.WriteString("### 合作研判（action=cooperation）\n\n")
			entry.WriteString("提交 world.action=cooperation，world.cooperation 使用下列 project_id、instance_id。source_text 必须准确引用玩家本轮真实意愿。action=propose 可只询问；只有玩家明确愿意分享亲查线索才设 share_discovery=true。work_if_agreed 表示明确分享并在本人同意时共同工作；work 需要已有正式同意。普通Promise、好感和同回合新登场不能自造资格。\n")
			entry.WriteString("现场本人可自然接受或拒绝，在 character_facts.exchanges.cooperation_decision 中写 project_id、realm_instance_id、decision=accept|refuse；只为真实在场的本人表达，不替玩家强制答应。拒绝或缺料不能叙述工作成功。共同作业按下列项目耗时推进世界钟，不是现实等待；NPC不因此进入场地或获知深处结果。普通独行、专长、强行和退出仍可选。\n")
			entry.WriteString("action=report 的 report_source_refs 仅复制玩家本次选择告知的下列来源；不选的类别不能顺带告知，报告不再发材料、技能或重复好感。\n")
		}
		fmt.Fprintf(&entry, "- project_id `%s`，instance_id `%s`：%s；已知伙伴 %s（actor_id `%s`），共同作业地点 %s（%s），当前阶段 %s。\n", project.ProjectID, project.InstanceID, project.Name, project.PartnerName, project.PartnerID, project.LocationName, project.LocationID, project.Phase)
		fmt.Fprintf(&entry, "    - %s 主角分工：%s 对方分工：%s\n", project.Description, project.PlayerRole, project.PartnerRole)
		fmt.Fprintf(&entry, "    - 共同耗时 %d 秒；对方自付能量 %d；可询问=%t，若同意可工作=%t，已有同意可工作=%t，可报告=%t，当前阻碍=%s。\n", project.DurationSeconds, project.PartnerEnergyCost, project.CanAsk, project.CanWorkIfAgreed, project.CanWork, project.CanReport, firstNonEmpty(project.BlockedReason, "无"))
		for _, cost := range project.PlayerCosts {
			fmt.Fprintf(&entry, "    - 主角自己的费用：%s %s（%s）×%d；不挪用NPC私物。\n", cost.Kind, firstNonEmpty(cost.Name, cost.NameKey, cost.RefID), cost.RefID, cost.Quantity)
		}
		if project.ClueText != "" {
			fmt.Fprintf(&entry, "    - 仅在玩家明确分享时告知这条亲查线索：%s（来源 `%s`）；未分享前不进入对方知情。\n", project.ClueText, project.ClueSourceRef)
		}
		for _, fact := range project.ReportFacts {
			fmt.Fprintf(&entry, "    - 可选报告 kind=%s，source_ref `%s`：%s\n", fact.Kind, fact.SourceRef, worldCooperationReportText(project.RealmID, fact))
		}
		if !worldEntry(sb, maxBytes, entry.String()) {
			worldEntry(sb, maxBytes, "- 其余已知合作因篇幅未列出；未列出的项目不能据此猜测提交。\n")
			return
		}
	}
}

func worldCooperationReportText(realmID string, fact interactive.CooperationReportFactView) string {
	switch fact.Kind {
	case "setback":
		return "这次现场探索已经受挫。"
	case "side":
		return "主角亲自走到了已打开的侧区。"
	case "ingress", "disposition":
		if fact.Description != "" {
			return fact.Description
		}
	}
	return "已提交的这一类结果；不推断其他未告知经历。"
}
