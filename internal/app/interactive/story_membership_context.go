package interactiveapp

import (
	"fmt"
	"strings"
	"ta/internal/interactive"
)

func writeWorldMembershipContext(sb *strings.Builder, state interactive.WorldState, maxBytes int) {
	if len(state.FactionMemberships) == 0 && len(state.MembershipContacts) == 0 {
		return
	}
	header := "\n### 势力身份（character_facts.memberships）\n仅记录主角本人已知的身份；身份不授指挥权、公库、好感或其他人的服从；temporary尚无结构化期限，不自动授予通行或正式事务资格。申请不保证成功。有权者仍可拒绝或提出条件；目录权限是校验依据，不是主角已经知道的秘密，也不是接纳承诺。\n本轮实际申请可记pending；formal/temporary须引用真实玩家原话source_text、较早的player→本人request_exchange_index及较后的本人→protagonist明确回应source_exchange_index，均为scene且已在场。退出可引用玩家实际原话并将source_exchange_index置0。无依据只保留对话，不能凭叙事授予身份。\n"
	if sb.Len()+len(header) > maxBytes {
		return
	}
	sb.WriteString(header)
	for i, m := range state.FactionMemberships {
		if i >= 16 {
			break
		}
		line := fmt.Sprintf("- 已提交身份 faction_id=%s，%s，status=%s，依据=%s。\n", m.FactionID, m.FactionName, m.Status, m.Statement)
		if sb.Len()+len(line) > maxBytes {
			return
		}
		sb.WriteString(line)
	}
	for i, c := range state.MembershipContacts {
		if i >= 16 {
			break
		}
		line := fmt.Sprintf("- 在场的已识别联系人 character_id=%s，faction_id=%s，%s；冻结的可接纳范围=%s。这是权限上限，不是同意。\n", c.CharacterID, c.FactionID, c.FactionName, strings.Join(c.Statuses, "/"))
		if sb.Len()+len(line) > maxBytes {
			return
		}
		sb.WriteString(line)
	}
}
