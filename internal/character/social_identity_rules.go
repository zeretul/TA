package character

import "strings"

// SocialIdentityRules uses the existing profiles and knowledge journal. NPC
// relationships are narrative facts, not protagonist Relationship records.
const SocialIdentityRules = `## 人物年龄与相互关系
- AI新建人物可在首次正式出场确定明确年龄，随character_facts.profiles.age与gender保存；未知就省略。不能只写在对白里，不能为接受邀请临时把未成年人改成年，也不能冒用原生source_key。已定年龄随正式时间增长，不因换模型、改名或新一轮互动重抽。
- NPC之间可以同时有夫妻/正式道侣、父母子女、师徒、朋友、同僚等关系。按当前作者资料与本局已发生事实承接，不推演原作未来为当前事实；师兄、义父、前辈等称呼不能凭字面认定血缘、收徒或婚姻。
- 重要关系写入当事人的profiles.description与character_facts.knowledge：statement注明双方姓名/已有稳定ID、关系类型和方向、当前状态及依据；父亲/子女、师父/弟子方向对应。只向真正知情者记录，隐私用single或participants，不凭空全世界公开。
- NPC间关系不能填进专门表示NPC与主角关系的relationships。关系变化引用本局实际经历，在knowledge使用supersedes/retracts替代旧记录，并同步相关人物当前描述；保留历史，不让旧婚约与新离异状态同时当当前事实。普通争吵不抹掉亲属、师承或婚姻。
- 正妻、正式配偶/道侣及作者明确保护的主伴侣关系存续时，拒绝与第三人发展恋爱或亲密关系。高好感、送礼、救命、身份权势、玩家转述“对方同意/已经离婚”都不解除限制；不编造对方的接受或关系破裂。
- 解除保护须先有本局正式保存的真实决裂和关系终止事实。已婚者须有本人明确离婚/解除道侣的表达及实际完成事实；分居、冷战、失联、传闻或一次争吵不够。尚未成婚不能虚构先结婚再离婚来绕过保护。关系结束后仍由本人自主决定是否开始新关系。
- 此限制不把妾室、侍妾、爱慕、绯闻或普通同行一概当正妻；如慕沛灵，不因与韩立有往来就自动禁绝追求，也不因此自动同意。仍核对本人意愿、现有承诺与真实处境。
- 本作作者保护对为凡人npc_han_li（韩立）与npc_nan_gongwan（南宫婉），星海xinghai-p01（韩萧）与xinghai-p02（艾默丝）。即使当前开局尚未成婚，也不以此空档供第三者攻略；这是作者关系边界，不得在对白里泄露尚未发生的未来婚姻。只有本局真实演进并正式终止该关系后才能重新考虑。不得仅因自创人物同名就套用此身份。
`

// ConversationSocialRules does not disclose anyone's private relationships.
const ConversationSocialRules = `你与其他人的夫妻/道侣、父母子女、师徒、朋友和同僚关系以本人已知的当前记录为准，可以并存；称呼、传闻和玩家单方宣称不替代真实关系。正式配偶或受保护主伴侣关系仍在时，拒绝第三人的恋爱或亲密请求，不因好感、利益或反复劝说改变；必须先有本局正式保存的真实决裂与关系终止，已婚者完成离婚/解除道侣。冷战、分居、失联不是离婚。非正妻身份不自动禁止发展关系，但从来不等于本人同意。回应自然表达人物立场，不讲内部字段或作者规则。`

// SocialIdentityContext binds an authored restriction to stable identity, not
// a display name, and keeps the fact in the invitation's normal persona context.
func SocialIdentityContext(profile Profile) string {
	var parts []string
	switch strings.TrimSpace(profile.SourceKey) {
	case "npc_nan_gongwan":
		parts = append(parts, "作者关系边界：你是南宫婉，受保护的主伴侣对象是韩立。当前时间线尚未成婚也不能被第三人攻略；不要伪造已有婚姻或泄露未来。只有本局正式保存真实决裂并终止该关系（已婚则须完成离婚/解除道侣）后，才可自主考虑新的感情。")
	case "npc_han_li":
		parts = append(parts, "作者关系边界：你是韩立，受保护的主伴侣对象是南宫婉。当前时间线尚未成婚也不能被第三人攻略；不提前编造未来婚姻。只有本局正式保存真实决裂与该关系终止后才可自主考虑新的感情。")
	case "xinghai-p02":
		parts = append(parts, "作者关系边界：你是艾默丝，受保护的主伴侣对象是韩萧。保持当前时间线，不提前编造已结婚；作者官配保护尚未被本局正式的真实决裂及关系终止解除时，不接受第三人的恋爱或亲密请求。")
	case "xinghai-p01":
		parts = append(parts, "作者关系边界：你是韩萧，受保护的主伴侣对象是艾默丝。保持当前时间线，不提前编造已结婚；作者官配保护尚未被本局正式的真实决裂及关系终止解除时，不接受第三人的恋爱或亲密请求。")
	case "npc_mu_peiling":
		parts = append(parts, "你是慕沛灵；本作不因与韩立的往来、侍妾或其他非正妻名分而自动将你列入官配禁止追求名单。按你本人的意愿、已有承诺和真实处境回应，不能默认答应，也不能虚构自己是韩立正妻。")
	}
	// Keep unrelated shopkeepers' recall budget for their actual question.
	// Global conversation rules are injected once by the chat prompt; only a
	// relevant established marriage adds a local boundary to this persona.
	if len(parts) == 0 {
		text := strings.ToLower(profile.Description + " " + strings.Join(profile.Bounds, " "))
		for _, term := range []string{"配偶", "夫妻", "道侣", "丈夫", "妻子", "正妻", "婚姻", "主伴侣", "spouse", "married", "husband", "wife", "primary partner"} {
			if strings.Contains(text, term) {
				parts = append(parts, ConversationSocialRules)
				break
			}
		}
	}
	return strings.Join(parts, "\n")
}
