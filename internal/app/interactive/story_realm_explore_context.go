package interactiveapp

import (
	"fmt"
	"strings"
	"ta/internal/interactive"
)

// This consumes the same known-only view as the player panel. No definition,
// hidden cache, capability token or freely adjustable cost goes to the model.
func worldRealmExploration(instance interactive.RealmStateView, now int64) string {
	if instance.Explore == nil {
		return ""
	}
	v := instance.Explore
	var b strings.Builder
	fmt.Fprintf(&b, "    - %s：%d/%d；达到界线会留下受挫记录而非死亡，已到手所得保留；已受挫=%t。\n", v.PressureLabel, v.Pressure, v.PressureLimit, v.Setback)
	for _, fact := range instance.View.ConfirmedFacts {
		fmt.Fprintf(&b, "    - 已确认迹象：%s\n", fact.Text)
	}
	if v.ReturnDueSeconds > 0 {
		fmt.Fprintf(&b, "    - 回访变化在正式时刻 %d 秒可检查；当前 %d 秒；到时=%t。真实行路、劳作、休整等推进同一时钟，阅读与现实等待不推进。\n", v.ReturnDueSeconds, now, v.ReturnAvailable)
	}
	for _, a := range v.Approaches {
		fmt.Fprintf(&b, "    - realm_action=%s, approach_id `%s`：%s；%s；耗时 %d 秒，风险变化 %+d；可用=%t", a.Kind, a.ID, a.Name, a.Description, a.DurationSeconds, a.PressureDelta, a.Available)
		if a.BlockedReason != "" {
			fmt.Fprintf(&b, "，暂不可用原因=%s", a.BlockedReason)
		}
		if a.CausesSetback {
			b.WriteString("；警告：这次会触及界线，核心受挫，不发成功所得")
		}
		for _, cost := range a.Costs {
			fmt.Fprintf(&b, "；%s %s ×%d", cost.Kind, firstNonEmpty(cost.Name, cost.NameKey, cost.RefID), cost.Quantity)
			if cost.EnergyCost > 0 {
				fmt.Fprintf(&b, "，实际精力成本 %d", cost.EnergyCost)
			}
		}
		b.WriteString("。\n")
	}
	b.WriteString("    - 玩家可调查、按现有准备选方法、只带部分所得撤离，也可不进入；不强制走完流程。实际已持有物品/已学技能才构成资格，本轮正文或Acquire不能倒填资格。\n")
	return b.String()
}
