package prompts

import (
	"fmt"
	"strings"
)

// Character creation is a draft, not a scene and not a turn.
//
// 自制-开局建角与AI身份生成 asks the model for one thing: given the options the
// player picked, polish and complete the character's prose —
// background, situation, a present predicament, plausible acquaintances, and a
// short opening seed. The player confirms the draft before any story exists, so
// the call is read-only and commits nothing.
//
// The prompt is written around the failure the design names: a model that
// "improves" a character by adding what the pack did not give them. It therefore
// repeats, in the terms of this call, that the choices are fixed, that the draft
// grants no items, techniques, money, bloodlines or numeric bonuses, and that a
// weapon direction is a tendency rather than training.

// StoryCreationSystemInstructionInput is the system prompt for one identity draft.
type StoryCreationSystemInstructionInput struct {
	// WorldName and WorldBrief introduce the pack, so the draft stays in its world.
	WorldName  string
	WorldBrief string
	// RealmLabel is the cultivation tier the chosen origin really starts at. It is
	// stated so the prose cannot describe a different one.
	RealmLabel string

	// The player's fixed choices, already resolved to human names.
	Name            string
	Gender          string
	Age             int
	Origin          string
	SelectedRole    string
	Affiliation     string
	Location        string
	InferLocation   bool
	Roots           string
	Aptitude        string
	WeaponDirection string
	Traits          string

	// ContactCandidates are people this world's author index places at or near the
	// starting location. They are offered as the only named people the draft may
	// use for acquaintances.
	ContactCandidates []string
	// Existing is the player's raw prose. Its facts carry into the polished draft.
	Existing IdentityDraftInput

	// OutputLanguage names the language the prose must be written in. It comes
	// from the page the player is reading, not from the world material: an
	// English page reading a Chinese world still asks for English prose. Empty
	// means "follow the world material", which is what a request that names no
	// language gets.
	OutputLanguage string
}

// IdentityDraftInput mirrors the prose half of a draft. It is defined here so
// the prompt package does not depend on the pack loader.
type IdentityDraftInput struct {
	Identity    string
	Appearance  string
	Background  string
	Situation   string
	Predicament string
	Contacts    []string
	Opening     string
}

const storyCreationBuiltIn = "你在当前世界的互动故事里准备一份人物档案。保留已确认的设定，润色玩家给的草稿，并补齐留空的部分。用具体动作、衣物、器物或经历呈现人物，语气贴合世界。省去工程术语、合同式提示和内部指令。"

// outputLanguageLine states which language the prose fields are written in.
//
// The page the player is reading decides this, and the world's own material is
// only the fallback. Saying it explicitly keeps an English page that opens a
// Chinese world from drafting Chinese prose the player cannot read, and a
// Chinese page from an English world from the reverse. Names, places and terms
// the world already established are kept as the world wrote them: translating a
// stable name would break the lookup the rules and the author index do on it.
func outputLanguageLine(locale, fallbackSource string) string {
	if language := proseLanguageName(locale); language != "" {
		return "所有散文字段都用" + language + "书写，写成平实的叙述文字。" +
			"材料已经确立的人名、地名、宗门和术语沿用材料给出的写法，只翻译它们周围的散文。"
	}
	return "所有散文字段都用与" + fallbackSource + "相同的语言书写，写成平实的叙述文字。"
}

// playerFacingLanguageLine states the language the player-facing prose is
// written in, for the prompts that drive the story itself rather than a
// creation draft.
//
// It says the same thing outputLanguageLine says for the part that matters —
// the page the player is reading decides the language, and names the story
// already established keep their established form — and adds the rule those
// prompts need and a creation draft does not: ids, keys and protocol values are
// not prose. A narrator told only to "follow the user's input" writes in the
// language of whatever the player typed, which on an English page over a
// Chinese world is still Chinese prose; naming the language is what closes that.
func playerFacingLanguageLine(locale, fallbackSource string) string {
	// Dialogue is named explicitly because it is the half of a turn a model will
	// otherwise treat as quoted material rather than as prose of its own: an
	// English page over a Chinese world came back with English narration and
	// every spoken line still in Chinese, and the quoted original was read as
	// the "established form" the next clause protects. Only names, places,
	// titles and terms keep the story's spelling; what someone says does not.
	if language := proseLanguageName(locale); language != "" {
		return "所有面向玩家的散文都用" + language + "书写，人物开口说的每一句对白和引号里的原话也算在内。" +
			"故事已经确立的人名、地名、称号和术语沿用故事给出的写法；id、字段名和协议取值原样保留，只翻译它们周围的散文。"
	}
	return "所有面向玩家的散文都用与" + fallbackSource + "相同的语言书写，人物开口说的每一句对白和引号里的原话也算在内。"
}

// proseLengthTarget states a length target in the unit the output language
// actually counts in.
//
// Every length in this product is calibrated in Chinese characters, and the
// prompts used to say so outright. On an English page that sentence contradicts
// the language instruction above it and gives the model two incompatible
// numbers to satisfy, so the unit follows the language while the calibration
// stays the same: a target calibrated in characters is restated as the word
// count that covers about the same amount of story.
func proseLengthTarget(locale string, chars int) string {
	if chars <= 0 {
		return ""
	}
	// A request that named no locale is the product's default page, and the
	// default page is Simplified Chinese, so the calibrated unit is the one that
	// page counts in. Falling through to a bare "characters" would drop the unit
	// the number was calibrated against.
	if strings.TrimSpace(locale) == "" {
		return fmt.Sprintf("%d 个汉字", chars)
	}
	switch proseLanguage(locale) {
	case "Chinese (Simplified)":
		return fmt.Sprintf("%d 个汉字", chars)
	case "English":
		words := (chars*65 + 50) / 100
		if words < 20 {
			words = 20
		}
		return fmt.Sprintf("约 %d 个英文单词", words)
	}
	return fmt.Sprintf("%d 个字符", chars)
}

// proseLanguageName names a language in the prompt's own language, for the
// sentences that tell the model which language to write in.
func proseLanguageName(locale string) string {
	switch proseLanguage(locale) {
	case "Chinese (Simplified)":
		return "简体中文"
	case "English":
		return "英文"
	}
	return ""
}

// proseLanguage names the language a locale asks for, or "" when the request
// named none and the material is the only signal there is.
func proseLanguage(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "":
		return ""
	case "zh", "zh-cn", "zh-hans", "zh-hans-cn":
		return "Chinese (Simplified)"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		return "Chinese (Simplified)"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en") {
		return "English"
	}
	return ""
}

// BuildStoryCreationSystemInstruction composes one identity draft.
func BuildStoryCreationSystemInstruction(in StoryCreationSystemInstructionInput) string {
	var sb strings.Builder
	sb.WriteString(storyCreationBuiltIn)
	sb.WriteString("\n\n## 写作要求\n")
	sb.WriteString("- 保留玩家已选定的姓名、年龄、性别、出身、所属、根基、资质、兵器倾向与性格，不得自行改动。已确认地点不改；只有下方明确要求推演地点时才安排开场。\n")
	sb.WriteString("- 将手填的短语和粗稿润色为自然文字，保留其中已有的人物、出身、衣着、体貌与经历。外貌用可见细节，少用评语和排比。\n")
	sb.WriteString("- 已经由出身确定的地位、财富、境界、家传装备和传承必须如实体现，不把少主或继承人降格成穷苦杂役。只描写已确认的资源和身份，不另送物品、头衔、师承、血脉或数值奖励。 不要杜撰修行速度、成功率或数值加成。\n")
	sb.WriteString("- 兵器倾向不等于已经掌握招式；所选出身中已经确认的技艺可以如实体现，其他高级能力不能凭空补上。\n")
	sb.WriteString("- 不替玩家决定将来，不擅自接任务、欠债、订婚、花钱或作出承诺，不代写杀人救人结果、结局或预言。\n")
	sb.WriteString("- 遵守当前世界和已确认出身，不另造重要国家、势力或有名人物。出身已经确认的自家家族照实写；可补普通住处、邻居、同门、店主等生活细节。\n")
	sb.WriteString("- " + outputLanguageLine(in.OutputLanguage, "下方的世界材料") + "不要标题、不要列表、不要数值区块。\n")

	sb.WriteString("\n## 世界材料\n\n")
	fmt.Fprintf(&sb, "世界：%s\n", strings.TrimSpace(in.WorldName))
	if brief := strings.TrimSpace(in.WorldBrief); brief != "" {
		fmt.Fprintf(&sb, "%s\n", brief)
	}

	sb.WriteString("\n## 玩家已确认选择\n\n")
	writeChoice := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(&sb, "- %s: %s\n", label, value)
		}
	}
	writeChoice("姓名", in.Name)
	writeChoice("性别", in.Gender)
	if in.Age > 0 {
		fmt.Fprintf(&sb, "- 年龄：%d\n", in.Age)
	}
	writeChoice("出身", in.Origin)
	writeChoice("已选身份", in.SelectedRole)
	writeChoice("所属", in.Affiliation)
	if in.InferLocation {
		writeChoice("开场地点线索", in.Location)
		sb.WriteString("请按所选出身、所属和人物经历推演合理的开场地点，并写入 location。宗门弟子优先从本门生活处起步，世家成员从自己家族的居处起步，散修可从合理的坊市、旅舍或野外生活场景起步，不默认站在无关宗门山门外。地点可细化到世界中普通居所、街巷，不另造重要势力。开场与 situation 必须在同一地点。下方起点只作参考。\n")
	} else {
		writeChoice("已确认地点", in.Location)
	}
	if realm := strings.TrimSpace(in.RealmLabel); realm != "" {
		writeChoice("初始境界", realm)
	}
	writeChoice("根基", in.Roots)
	writeChoice("资质", in.Aptitude)
	writeChoice("兵器倾向", in.WeaponDirection)
	writeChoice("性格", in.Traits)

	if len(in.ContactCandidates) > 0 {
		sb.WriteString("\n## 附近人物参考\n\n")
		sb.WriteString("有名人物从这些参考中选取，也可不写。不要凭空声称对方欠人情、已答应帮忙或必定出场。\n\n")
		for _, candidate := range in.ContactCandidates {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				fmt.Fprintf(&sb, "- %s\n", candidate)
			}
		}
	}

	existing := in.Existing
	written := make([]string, 0, 7)
	appendIf := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			written = append(written, fmt.Sprintf("- %s: %s", label, value))
		}
	}
	appendIf("身份补充", existing.Identity)
	appendIf("外貌", existing.Appearance)
	appendIf("背景", existing.Background)
	appendIf("处境", existing.Situation)
	appendIf("眼前事项", existing.Predicament)
	if len(existing.Contacts) > 0 {
		appendIf("熟人", strings.Join(existing.Contacts, "；"))
	}
	appendIf("开场", existing.Opening)
	if len(written) > 0 {
		sb.WriteString("\n## 玩家已写内容\n\n")
		sb.WriteString("保留以下文字里的事实，改写为可直接显示在人物档案里的成稿。身份补充中属于资质或经历的内容可在相应背景中自然交代。\n\n")
		sb.WriteString(strings.Join(written, "\n"))
		sb.WriteString("\n")
	}

	sb.WriteString("\n## 字段内容\n")
	sb.WriteString("- `identity`: 写出身、职业或所属中的实际身份与地位。资质、天赋、容貌、性格归各自段落。已选预设身份按上方身份填写；自定义出身沿用玩家确定的身份。\n")
	sb.WriteString("- `appearance`: 描写体态、衣着与气质，不杜撰秘密印记或隐藏血脉。\n")
	sb.WriteString("- `background`: 用一小段交代与出身相符的成长经历，不强加宿命。\n")
	sb.WriteString("- `situation`: 眼下在开场地点的具体处境，与身份和家业相称。\n")
	sb.WriteString("- `predicament`: 眼前可自由选择处理的一件事；富贵出身也可只是会客、出游或修习，不强制制造贫困、欠债或灭门危机。\n")
	sb.WriteString("- `contacts`: 至多三位合理认识的人，优先参考上方名单，简述来往缘由，不附加债务或承诺。\n")
	sb.WriteString("- `opening`: 用两三句呈现开场地点和眼前情形，不替玩家定结果，不预演人物后续对白。\n")

	sb.WriteString("\n## 输出格式\n")
	sb.WriteString("只返回 JSON 对象，不加 Markdown 围栏和额外解释：\n\n")
	sb.WriteString("{\n")
	if in.InferLocation {
		sb.WriteString("  \"location\": \"具体开场地点\",\n")
	}
	sb.WriteString("  \"identity\": \"string\",\n")
	sb.WriteString("  \"appearance\": \"string\",\n")
	sb.WriteString("  \"background\": \"string\",\n")
	sb.WriteString("  \"situation\": \"string\",\n")
	sb.WriteString("  \"predicament\": \"string\",\n")
	sb.WriteString("  \"contacts\": [\"string\"],\n")
	sb.WriteString("  \"opening\": \"string\"\n")
	sb.WriteString("}\n\n")
	sb.WriteString("- 返回润色后的外貌、背景与处境等字段，保留玩家原意，避免照抄原来的短语。\n")
	sb.WriteString("- 身份和眼前事项建议各60字以内，外貌与处境各120字以内，背景与开场各200字以内；location 为具体地点名称，200字以内。\n")
	sb.WriteString("- 不要增加其他字段或外层对象，不在对象外写文字。\n")
	return sb.String()
}

// StoryCreationUserMessage is the request itself.
const StoryCreationUserMessage = "根据已确认选择润色并补完人物资料，返回约定的 JSON 对象。"

// writeLanguageFirst states the page's language before anything else in a
// prompt, then restates it where the prompt describes its output.
//
// The output protocol at the bottom of these prompts already names the
// language. In practice that was not enough: a long instruction between the two
// points — a world's own material, a scene, a style fragment — reads as the
// thing to continue, and a narrator handed Chinese context writes Chinese prose
// however the output protocol is worded. Saying it first gives the rule the
// same footing as the material it is about to override.
func writeLanguageFirst(sb *strings.Builder, locale, fallbackSource, appliesTo string) {
	line := playerFacingLanguageLine(locale, fallbackSource)
	if strings.TrimSpace(line) == "" {
		return
	}
	sb.WriteString("# 语言\n\n")
	sb.WriteString(line)
	if strings.TrimSpace(appliesTo) != "" {
		sb.WriteString(" " + appliesTo)
	}
	sb.WriteString("\n\n---\n\n")
}
