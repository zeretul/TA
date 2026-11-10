package storypack

import (
	"errors"
	"fmt"
	"strings"
)

// IdentityError is a refusal the creation page shows the player.
//
// The wording belongs to the page's language, so the error carries a message key
// and the values it interpolates instead of a finished sentence: the handler
// resolves it against the request's own locale, and a page shown in English
// reads an English refusal. Error() reports the key, which is what a log wants —
// the sentence is not something to grep for.
type IdentityError struct {
	Key  string
	Args []any // name, value pairs, the form IdentityErrorText interpolates
}

func (err *IdentityError) Error() string { return err.Key }

func identityError(key string, args ...any) error {
	return &IdentityError{Key: key, Args: args}
}

// IdentityErrorText writes a refusal in the language the page is reading.
//
// A refusal that is not an IdentityError is returned as it stands: it is either
// a technical failure the handler logs, or a sentence a lower layer already
// wrote. Only the sentences this package owns are translated, so a message can
// never be half-translated by a wrapper.
func IdentityErrorText(err error, locale string) string {
	if err == nil {
		return ""
	}
	var keyed *IdentityError
	if !errors.As(err, &keyed) {
		return err.Error()
	}
	template := identityMessages[keyed.Key][identityLanguageIndex(locale)]
	if template == "" {
		template = identityMessages[keyed.Key][0]
	}
	if template == "" {
		return keyed.Key
	}
	out := template
	for i := 0; i+1 < len(keyed.Args); i += 2 {
		name, ok := keyed.Args[i].(string)
		if !ok || name == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+name+"}}", fmt.Sprint(keyed.Args[i+1]))
	}
	return out
}

// identityLanguageIndex is the column of identityMessages one locale reads. The
// table is a fixed pair rather than a map per language so a new sentence cannot
// be added in one language only.
func identityLanguageIndex(locale string) int {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return 1
	}
	return 0
}

// identityMessages holds the two languages of every refusal the creation page
// and the world draft can show. The keys are grouped by surface so a reader can
// see the whole vocabulary of one page at a time.
var identityMessages = map[string][2]string{
	"api.storyPack.identity.locationAuthoredOnly":     {"此世界的自建角色需要选择已声明的开局地点；其他出生地与经历可以写入背景。", "Choose an authored starting location for a custom character in this world. Other birthplaces and experiences may still be described in the background."},
	"api.storyPack.world.loreGateMissing":             {"世界资料的知情限制指向不存在的条目：{{id}}。请一并更新条目与限制。", "A knowledge restriction references a missing world entry: {{id}}. Update the entry and its restriction together."},
	"api.storyPack.settings.ruleUnavailable":          {"所选判定系统不可用：{{id}}。请重新选择。", "The selected check system is unavailable: {{id}}. Choose another one."},
	"api.storyPack.settings.ruleIncompatible":         {"所选判定需要不同的角色状态，请选择适用于此世界的判定。", "The selected checks require a different character state. Choose checks compatible with this world."},
	"api.storyPack.draft.connectionFailed":            {"无法连接当前游戏模型服务。请到系统设置检查游戏模型的服务地址；如果使用本机模型代理，请先启动它，再重试。已填写的身份资料仍保留。", "Cannot connect to the current game model service. Check its address in System settings. If it uses a local proxy, start that service and retry. Your character details are preserved."},
	"api.storyPack.draft.cancelled":                   {"身份生成已取消，已填写的资料仍保留。", "Character generation was cancelled. Your details are preserved."},
	"api.storyPack.draft.failed":                      {"身份生成未完成。请检查系统设置中的游戏模型是否可用后重试；详细原因已记录在日志中，已填写的资料仍保留。", "Character generation did not finish. Check the game model in System settings and retry. Details are in the log; your character details are preserved."},
	"api.storyPack.identity.adventureOptionUnknown":   {"此开局没有可用的修习选项：{{id}}。", "This starting discipline is not available: {{id}}."},
	"api.storyPack.identity.customAffiliationInvalid": {"自定义宗门或家族名称最多120字，不能包含控制字符。", "A custom sect or family name must be at most 120 characters without control characters."},
	"api.storyPack.identity.customLocationInvalid":    {"自定义开场地点最多200字，不能包含控制字符。", "A custom starting location must be at most 200 characters without control characters."},
	// The creation page.
	"api.storyPack.identity.notReady":               {"剧本包 {{pack}} 没有建角选项", "World pack {{pack}} has no character creation page."},
	"api.storyPack.identity.nameRequired":           {"请填写姓名", "Enter a name."},
	"api.storyPack.identity.nameTooLong":            {"姓名过长（最多 {{max}} 字）", "The name is too long (up to {{max}} characters)."},
	"api.storyPack.identity.genderMissing":          {"该世界包没有提供性别选项", "This world pack offers no gender options."},
	"api.storyPack.identity.ageMissing":             {"年龄未生成", "No age was rolled yet."},
	"api.storyPack.identity.ageOutOfRange":          {"年龄 {{age}} 不在本世界包的范围 {{min}}–{{max}} 内", "Age {{age}} is outside this world pack's range of {{min}}–{{max}}."},
	"api.storyPack.identity.originUnknown":          {"未知出身: {{id}}", "Unknown origin: {{id}}"},
	"api.storyPack.identity.originStateMissing":     {"出身 {{origin}} 没有可用的初始状态", "Origin {{origin}} has no starting state."},
	"api.storyPack.identity.originTemplateMismatch": {"出身 {{origin}} 不能从模板 {{template}} 开始", "Origin {{origin}} cannot start from template {{template}}."},
	"api.storyPack.identity.templateMissing":        {"初始状态模板不存在: {{template}}", "That starting state template does not exist: {{template}}"},
	"api.storyPack.identity.originTooLong":          {"出身描述过长（最多 {{max}} 字）", "The origin description is too long (up to {{max}} characters)."},
	"api.storyPack.identity.variantRootUnknown":     {"未知变异灵根: {{id}}", "Unknown mutated root: {{id}}"},
	"api.storyPack.identity.variantRootExclusive":   {"已经选了{{variant}}，就不能再选普通灵根属性；变异灵根与普通灵根只能选一种", "You already picked {{variant}}, so no ordinary root attribute can be added; a character has one kind of root or the other."},
	"api.storyPack.identity.aptitudeMissing":        {"该世界包没有可用的修行资质", "This world pack offers no cultivation aptitudes."},
	"api.storyPack.identity.talentUnknown":          {"未知天赋特性: {{id}}", "Unknown innate gift: {{id}}"},
	"api.storyPack.identity.talentUnsupported":      {"天赋特性 {{id}} 没有规则支持的数值", "The innate gift {{id}} has no value the rules support."},

	// Affiliation and place.
	"api.storyPack.identity.affiliationNoneMissing":   {"该世界包没有“无宗门”选项，无法替你安排所属", "This world pack has no unaffiliated option, so no affiliation can be arranged for you."},
	"api.storyPack.identity.affiliationUnknown":       {"未知所属: {{id}}", "Unknown affiliation: {{id}}"},
	"api.storyPack.identity.ownFamilyNeedsName":       {"请先填写姓名，才能按姓氏另立一族", "Enter a name first, so a household can be founded under your surname."},
	"api.storyPack.identity.affiliationNoneForbidden": {"出身“{{origin}}”是散修，不能再有宗门或家族", "Origin “{{origin}}” is a loose cultivator, so it cannot also belong to a sect or family."},
	"api.storyPack.identity.sectRequired":             {"出身“{{origin}}”是宗门弟子，请选择所属宗门", "Origin “{{origin}}” is a sect disciple, so choose the sect you belong to."},
	"api.storyPack.identity.sectMismatch":             {"出身“{{origin}}”需要宗门所属，所选“{{affiliation}}”不是宗门", "Origin “{{origin}}” needs a sect, and the chosen “{{affiliation}}” is not one."},
	"api.storyPack.identity.familyRequired":           {"出身“{{origin}}”是世家子弟，请选择或另立家族", "Origin “{{origin}}” comes from a cultivation family, so choose a family or found one."},
	"api.storyPack.identity.familyMismatch":           {"出身“{{origin}}”需要家族所属，所选“{{affiliation}}”不是家族", "Origin “{{origin}}” needs a family, and the chosen “{{affiliation}}” is not one."},
	"api.storyPack.identity.affiliationKindMismatch":  {"出身“{{origin}}”的所属类型 {{kind}} 无法匹配“{{affiliation}}”", "Origin “{{origin}}” needs a {{kind}} affiliation, which “{{affiliation}}” is not."},
	"api.storyPack.identity.locationMissing":          {"该世界包没有提供落脚地点", "This world pack offers no starting places."},
	"api.storyPack.identity.locationUnknown":          {"未知落脚地点: {{id}}", "Unknown starting place: {{id}}"},
	"api.storyPack.identity.locationFactionMismatch":  {"落脚地点“{{location}}”在{{faction}}，与所选所属“{{affiliation}}”不一致", "The starting place “{{location}}” belongs to {{faction}}, which does not match the chosen affiliation “{{affiliation}}”."},
	"api.storyPack.identity.locationCountryMismatch":  {"所属“{{affiliation}}”在{{affiliation_country}}，落脚地点“{{location}}”却在{{location_country}}；请改选其中一个", "The affiliation “{{affiliation}}” is in {{affiliation_country}}, but the starting place “{{location}}” is in {{location_country}}; change one of them."},

	// Spiritual root and traits.
	"api.storyPack.identity.rootAttributeUnknown": {"未知灵根属性: {{id}}", "Unknown root attribute: {{id}}"},
	"api.storyPack.identity.rootArityExact":       {"{{category}}需要正好 {{count}} 种属性，当前选了 {{chosen}} 种", "{{category}} needs exactly {{count}} attributes; {{chosen}} are chosen."},
	"api.storyPack.identity.rootArityRange":       {"{{category}}需要 {{min}}–{{max}} 种属性，当前选了 {{chosen}} 种", "{{category}} needs {{min}}–{{max}} attributes; {{chosen}} are chosen."},
	"api.storyPack.identity.rootAttributeTooMany": {"灵根属性最多 {{max}} 种", "A root has at most {{max}} attributes."},
	"api.storyPack.identity.traitUnknown":         {"未知特性: {{id}}", "Unknown temperament: {{id}}"},
	"api.storyPack.identity.traitTooMany":         {"特性最多选 {{max}} 项", "Choose at most {{max}} temperaments."},

	// Drafting.
	"api.storyPack.draft.rewriteKeepsAge": {"重写背景不会重新抽取年龄", "Rewriting the background does not roll a new age."},
	"api.storyPack.draft.rewriteNeedsAge": {"重写背景需要带上当前年龄", "Rewriting the background needs the current age."},
	"api.storyPack.draft.unreadable":      {"身份草稿不是可读的 JSON 对象", "The identity draft was not a readable JSON object."},
	"api.storyPack.draft.unparsable":      {"身份草稿无法解析: {{detail}}", "The identity draft could not be read: {{detail}}"},
	"api.storyPack.draft.empty":           {"身份草稿没有写出任何可用内容", "The identity draft wrote nothing usable."},

	// The world draft.
	"api.storyPack.world.openingMissing": {"开局方式尚未选择", "No opening style was chosen."},
	"api.storyPack.world.nameMissing":    {"世界还没有名称", "The world has no name yet."},
	"api.storyPack.world.originMissing":  {"世界还没有开局前提", "The world has no opening premise yet."},
	"api.storyPack.world.originsMissing": {"世界草稿没有任何可用出身", "The world draft has no usable origin."},
	"api.storyPack.world.entriesMissing": {"世界草稿还没有任何世界书条目", "The world draft has no world book entries yet."},
	"api.storyPack.world.idUnavailable":  {"无法为这个世界分配可用的 ID：{{base}}", "No usable ID could be assigned to this world: {{base}}"},
	"api.storyPack.world.unreadable":     {"世界草稿不是可读的 JSON 对象", "The world draft was not a readable JSON object."},
	"api.storyPack.world.unparsable":     {"世界草稿无法解析: {{detail}}", "The world draft could not be read: {{detail}}"},
	"api.storyPack.world.premiseEmpty":   {"世界草稿没有写出开局前提", "The world draft wrote no opening premise."},

	// The ready-made character a pack offers.
	"api.storyPack.templateMissing": {"剧本包没有该快捷模板: {{template}}", "This world pack has no such ready-made character: {{template}}"},
}
