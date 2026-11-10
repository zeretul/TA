package history

import (
	"context"
	"sort"
	"strings"
)

// Context prioritizes fresh canonical facts over old dialogue and restricts
// retrieval to witnessed conversations. The full raw archive remains in State.
func (s *Store) Context(ctx context.Context, id, ministerID, query string) (MinisterContext, error) {
	state, err := s.Read(ctx, id)
	if err != nil {
		return MinisterContext{}, err
	}
	m := minister(state, ministerID)
	if m == nil {
		return MinisterContext{}, ErrNotFound
	}
	c := MinisterContext{Minister: *m, CurrentFacts: map[string]any{"year": state.Year, "quarter": state.Quarter, "turn": state.Turn, "revision": state.Revision}, AssignedProjects: []Project{}, Memories: []ChatTurn{}, Rule: "当前事实是本季已核定记录，优先于旧谈话中的数值。记忆只证明当时说过的话，不等于承诺已经执行。你只知道列出的个人资料、现行情报、本人经办事务和本人在场谈话；未知情报须说明尚未收到。标有回档后废弃的分支不参与当前判断。"}
	if state.World != nil {
		view := ContextWorld(VisibleWorld(*state.World, ministerID))
		c.CurrentFacts["world"] = view
		c.CurrentFacts["decision_briefing"] = map[string]any{"current_accounts": view.Accounts, "approved_orders": view.Orders, "rule": "账户是当前已核定可用余额，不是未经核实估计。approved_orders中active/waiting的长期政策已有持续授权，下季照旧，不须皇帝重发；暂停或修改须有新命令。已有余额但新增事项未获准时，只能说尚未获准，不能说无银。旧谈话及大臣建议不能新增审批或撤销既有授权。"}
		c.CurrentFacts["scenario"] = state.Scenario.Name
		c.CurrentFacts["resource_definitions"] = state.Scenario.Resources
	}
	for _, knowledge := range m.Knowledge {
		switch knowledge {
		case "finance":
			c.CurrentFacts["treasury"] = state.Treasury
			c.CurrentFacts["grain"] = state.Grain
			escrow, grainEscrow, spent := int64(0), int64(0), int64(0)
			for _, p := range state.Projects {
				if p.Reserved {
					escrow += p.Budget - p.Spent
					grainEscrow += p.Grain - p.GrainSpent
				}
				spent += p.Spent
			}
			c.CurrentFacts["project_escrow"] = escrow
			c.CurrentFacts["grain_escrow"] = grainEscrow
			c.CurrentFacts["project_spent"] = spent
		case "privy":
			c.CurrentFacts["privy_purse"] = state.PrivyPurse
		case "regions":
			brief := []map[string]any{}
			for _, r := range state.Regions {
				brief = append(brief, map[string]any{"id": r.ID, "name": r.Name, "disaster": r.Disaster, "refugees": r.Refugees, "heart": r.Heart, "irrigation": r.Irrigation})
			}
			c.CurrentFacts["regions"] = brief
		case "military":
			military := []map[string]any{}
			for _, r := range state.Regions {
				military = append(military, map[string]any{"region_id": r.ID, "training": r.Training})
			}
			c.CurrentFacts["military"] = military
		}
	}
	for _, p := range state.Projects {
		if p.MinisterID == ministerID {
			c.AssignedProjects = append(c.AssignedProjects, p)
		}
	}
	type candidate struct {
		chat  ChatTurn
		score int
	}
	eligible := []candidate{}
	queryTerms := bigrams(query)
	for _, chat := range state.Chats {
		if discarded(state, chat.Revision) || !contains(chat.MinisterIDs, ministerID) {
			continue
		}
		text := chat.UserMessage
		for _, r := range chat.Replies {
			text += " " + r.Text
		}
		score := 0
		terms := bigrams(text)
		for term := range queryTerms {
			if terms[term] {
				score += 3
			}
		}
		for _, promise := range []string{"承诺", "答应", "负责", "promise", "undertake"} {
			if strings.Contains(strings.ToLower(text), promise) {
				score += 8
			}
		}
		eligible = append(eligible, candidate{chat: chat, score: score})
	}
	for i := max(0, len(eligible)-10); i < len(eligible); i++ {
		eligible[i].score += 1000 + i
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].score != eligible[j].score {
			return eligible[i].score > eligible[j].score
		}
		return eligible[i].chat.Revision > eligible[j].chat.Revision
	})
	bytes := 0
	for _, candidate := range eligible {
		size := len(candidate.chat.UserMessage)
		for _, r := range candidate.chat.Replies {
			size += len(r.Text)
		}
		if len(c.Memories) >= 24 || bytes+size > 64000 {
			continue
		}
		c.Memories = append(c.Memories, candidate.chat)
		bytes += size
	}
	c.OmittedTurns = len(eligible) - len(c.Memories)
	sort.Slice(c.Memories, func(i, j int) bool { return c.Memories[i].Revision < c.Memories[j].Revision })
	return c, nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func discarded(s State, rev uint64) bool {
	for _, r := range s.DiscardedRanges {
		if rev >= r.Start && rev <= r.End {
			return true
		}
	}
	return false
}
func bigrams(s string) map[string]bool {
	runes := []rune(strings.ToLower(s))
	out := map[string]bool{}
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}
