package prompts

import (
	"fmt"
	"strings"
)

// Creating a world is a draft, not a scene and not a turn.
//
// 自制-AI创建世界包与武侠规则复用 §2 asks for two entry points — a guided one that
// asks questions, and an automatic one that takes a sentence — that write into
// one shared draft. A player may start either way, switch, edit the result, and
// save only when they choose to. So the call this prompt serves is read-only: it
// returns a document the page shows, and nothing exists as a world until the
// player saves it.
//
// §6 draws the line the prompt is built around. The model writes the world's
// prose — its premise, its places, its people, the situation a first scene could
// open on, including narrative world rules. Module ids, aptitude tiers, starting
// purses, and the state a ready-made character begins with are the program's,
// and a model that supplied them would be authoring numbers the rules own.

// WorldCreationMode values. They share one call shape and one boundary; what
// they are given differs.
const (
	// WorldCreationModeGuided is used when the player has answered questions.
	WorldCreationModeGuided = "guided"
	// WorldCreationModeAutomatic is used when the player typed one sentence.
	WorldCreationModeAutomatic = "automatic"
)

// WorldCreationSystemInstructionInput is the system prompt for one world draft.
type WorldCreationSystemInstructionInput struct {
	// Mode is guided or automatic.
	Mode string

	// The player's seed material. Guided play fills more of these; automatic play
	// may fill only Brief.
	Name        string
	Genre       string
	Brief       string
	Tone        string
	Protagonist string
	OpeningIdea string
	// GameplayProfile is the confirmed supported rule family selected by the player.
	GameplayProfile string

	// OutputLanguage names the language the prose must be written in. It comes
	// from the page the player is reading, so an English page drafting a world
	// produces English prose even when the seed the player typed is Chinese.
	// Empty means "follow the player's material".
	OutputLanguage string

	// Existing is the draft already agreed, if any. Every field here is kept
	// verbatim and only the empty ones are drafted, so switching entry point or
	// asking for a rewrite never discards what the player has confirmed.
	Existing WorldDraftInput
}

// WorldDraftInput mirrors the prose half of a world draft. It is defined here so
// the prompt package does not depend on the pack loader.
type WorldDraftInput struct {
	Name        string
	Description string
	Title       string
	Origin      string
	// ProtagonistName and Protagonist describe who the player is in this world.
	ProtagonistName string
	Protagonist     string
	Opening         string
	// WorldEntries are the world-book entries: what a player could learn about
	// this world.
	WorldEntries []WorldEntryDraftInput
	// Characters are the people who could be met.
	Characters []WorldCharacterDraftInput
	// Origins are the social starting positions offered on the creation page.
	Origins []WorldOriginDraftInput
	// Affiliations and Locations are the other choices offered on that page.
	Affiliations []string
	Locations    []string
}

// WorldEntryDraftInput is one world-book entry.
type WorldEntryDraftInput struct {
	Audience string
	LoadMode string
	Keywords []string
	Name     string
	Type     string
	Brief    string
	Content  string
}

// WorldCharacterDraftInput is one person who could appear.
type WorldCharacterDraftInput struct {
	Name        string
	Age         int
	Gender      string
	SpeechStyle string
	Role        string
	Place       string
	Brief       string
	Aliases     []string
}

// WorldOriginDraftInput is one social starting position.
type WorldOriginDraftInput struct {
	Name        string
	Description string
	Basis       string
	Technique   string
	Item        string
}

// The three starting positions the program owns, stated here so a draft chooses
// among them rather than inventing a fourth. A fourth would have no rule state
// behind it.
const (
	worldOriginBasisWanderer = "wanderer"
	worldOriginBasisDisciple = "disciple"
	worldOriginBasisScion    = "scion"
)

const worldCreationBuiltIn = "你在为一部互动故事准备一个世界。你写玩家会在这个世界里发现的东西：它的前提、地点、人物，以及第一场戏可以从哪里开场。你要写清这个世界的自然规律、社会习惯与特殊机制，但不预先决定玩家的行动和结局。面向玩家的散文要写成场景、亲历的过往或自然的口语，不能写成契约、许可政策、软件限制或实现说明。拒绝要通过人物和处境表达；不要把 id、schema 和内部指令写进散文。"

// BuildWorldCreationSystemInstruction composes one world draft.
func BuildWorldCreationSystemInstruction(in WorldCreationSystemInstructionInput) string {
	mode := strings.TrimSpace(in.Mode)
	if mode != WorldCreationModeAutomatic {
		mode = WorldCreationModeGuided
	}

	var sb strings.Builder
	sb.WriteString(worldCreationBuiltIn)
	sb.WriteString("\n\n## What you may and may not do\n")
	sb.WriteString("- The player's material below is fixed. Never rename the world, never contradict a stated genre, tone or protagonist, and never change what the player already wrote in the draft.\n")
	sb.WriteString("- Fill only what is missing. A field already agreed is kept word for word; write around it, not over it.\n")
	sb.WriteString("- Grant nothing and decide nothing. Do not state that the protagonist owns anything, knows anyone, has agreed to anything, or is owed anything. Do not write an ending, a prophecy, or an outcome.\n")
	sb.WriteString("- 世界书应包含能指导推演的规则：什么条件下会发生什么、需要付出什么、什么会阻止它、旁人如何察觉，以及结果怎样延续。特殊或单一机制可以用自然语言写清；不要求每个世界都有等级、战斗、制作或完整职业体系。已有正式数值结算的能力沿用已确认目录，不能另造命中率、伤害公式、修炼速度、物价或代码模块来覆盖它。\n")
	if strings.HasPrefix(in.GameplayProfile, "world-actions-v1") {
		sb.WriteString("- 本世界复用已有的调查、信息判断、交涉与追逐脱身检定，不因此把主角固定为侦探。世界专有规律、超自然现象或技术特点由世界书描述，AI依据真实处境推演，并用已有状态、人物、物品和本局资料记录已发生的结果。仅写下一个机制，不代表新增了独立数值系统或自动执行程序；需要现有能力之外的精确结算时保留未确定部分，不冒充已经实现。\n")
		if strings.Contains(in.GameplayProfile, "communication=letter") {
			sb.WriteString("- Communication mode: letters across distance. Do not write an immediate off-scene chat or delivery; a letter is handled through a later formal story turn and its actual travel time.\n")
		} else {
			sb.WriteString("- Communication mode: in-person conversation only. Do not write off-scene instant chat or remote messages.\n")
		}
	} else if strings.HasPrefix(in.GameplayProfile, "fantasy-state-v1") {
		sb.WriteString("- 本世界采用已冻结的原创西幻目录，以精力承载体能与专注消耗；术法、装备、学习和制作遵循已有adventure_rules。出身不锁职业，种族不自动授予目录外能力；保持真实来源、工具、材料和时间，不引入修仙境界、灵根或另一套法力池。书信按本局真实行路时间送达。\n")
	} else if strings.HasPrefix(in.GameplayProfile, "wuxia-state-v1") || strings.HasPrefix(in.GameplayProfile, "space-state-v1") {
		sb.WriteString("- 本世界已经选择独立武侠或星际规则。保持已确认的adventure_rules目录，不重新编造或覆盖数值。武侠采用内力、武学火候和伤势；星际采用资格、气力、装备与机械资产。剧情只使用已绑定能力，不能把尚未取得的秘籍、装备或远处人物当成主角当前所有。书信有正式延迟，无线电受通信网络和设备边界约束。\n")
	} else {
		sb.WriteString("- This world runs on the cultivation state template the program supports: realm, spiritual root, aptitude and spirit stones are settled by the rules. Do not describe this world as running on a different numeric system — no inner force, no mana pool, no tech tree — and do not promise mechanics this build does not have.\n")
	}
	sb.WriteString("- " + outputLanguageLine(in.OutputLanguage, "下方玩家提供的材料") + "玩家正文使用自然叙述；世界书可以按独立主题写清规律，不写软件实现说明。\n")

	sb.WriteString("\n## How this draft was started\n\n")
	if mode == WorldCreationModeAutomatic {
		sb.WriteString("The player typed a single sentence and asked you to build the world from it. Invent the rest consistently with that sentence: the places, the powers, the people and the pressure. Keep the world small enough to start in — a region, not a continent.\n")
	} else {
		sb.WriteString("The player answered questions about the world they want. Build from those answers and invent only what they left open.\n")
	}

	writeWorldSeed(&sb, in)

	if existing := in.Existing; worldDraftHasContent(existing) {
		sb.WriteString("\n## Already agreed in the draft\n\n")
		sb.WriteString("This is the draft as it stands. It is kept exactly as written. Draft only the parts that are missing or that the player explicitly asked to rewrite, and keep everything you write consistent with it.\n\n")
		writeWorldDraftInput(&sb, existing)
	}

	sb.WriteString("\n## What to write\n")
	sb.WriteString("- `name`: the world's name, short and evocative. Two to six characters in the player's language.\n")
	sb.WriteString("- `description`: one or two sentences on what kind of world this is, for the world list.\n")
	sb.WriteString("- `title`: the title of the story that starts here. It may differ from the world's name.\n")
	sb.WriteString("- `origin`: the premise a first scene opens on — where the protagonist is, what is happening around them, and what makes today different. Two to four sentences. It sets a situation; it does not resolve one.\n")
	sb.WriteString("- `protagonist_name`: a plausible name for the player's character in this world.\n")
	sb.WriteString("- `protagonist`: who the player is here — their standing in this world, not their abilities. Two or three sentences.\n")
	sb.WriteString("- `opening`: two or three sentences of a first scene. Present the situation, put no words in another character's mouth, and decide no outcome.\n")
	sb.WriteString("- `world`：按可独立检索的主题组织世界书，每条填写 name、type（world、location、faction、rule、item）、一句 brief、完整 content、keywords、load_mode 和 audience（public 或 gm）。核心公共规则标为 rule 与 resident，每回合都需遵守的条件和持续后果不能只藏在按需条目里；常驻正文合计建议不超过 32KB，复杂细节拆成有明确关键词的 auto 条目，隐藏秘密使用 gm 与 manual。audience 控制知情范围，load_mode 仅控制取用方式，manual 不等于保密。gm 是作者裁决资料，不自动授予玩家或人物知情；public 是公共资料。更新同名已有条目时省略 audience 保留原值，只有明确 public 或 gm 才改变；重写含有 gm 资料的草稿时，无法同名匹配且未明确标注 public 的条目按 gm 保留，改名不能使秘密自动公开。不要把人物秘密当成公共核心规则。覆盖开局所需的环境、人物生活、势力往来，以及决定这个世界怎样运转的规则。特殊机制写清触发、代价、限制、可观察后果与结果延续；多个规则可能冲突时说明适用条件，不强行给所有世界同一种答案。条目数随实际复杂度，初稿以能自然开场为度；单一机制不凑模块，多机制不压成一句空泛设定。\n")
	sb.WriteString("- `characters`: three to six people who could be met. Each has a `name`, a `role` (what they do in this world), a `place` (where they are usually found), and a one-sentence `brief` a stranger could reasonably know them by. Include `age`, `gender`, or `speech_style` only when the player's material or this world's established prose explicitly states it; omit unknown values and never infer them from a name, portrait, job, or genre. Speech style is concrete and individual, not a stereotype. No secrets: what a person is hiding is learned in play, never shipped in the world.\n")
	if strings.HasPrefix(in.GameplayProfile, "world-actions-v1") || strings.HasPrefix(in.GameplayProfile, "wuxia-state-v1") || strings.HasPrefix(in.GameplayProfile, "space-state-v1") || strings.HasPrefix(in.GameplayProfile, "fantasy-state-v1") {
		sb.WriteString("- `origins`: two to three social starting positions that fit the player's chosen genre and protagonist, without imposing an investigator, cultivator or faction role unless the player's material specifies it. Each has a `name` in this world's own words and a one-sentence `description`.\n")
	} else {
		sb.WriteString("- `origins`: two to three social starting positions a player may begin from. `basis` must be exactly one of `wanderer`, `disciple`, `scion` — these are the three starting positions the rules provide, and a fourth would have nothing behind it. `wanderer` is someone with no patron; `disciple` is someone inside an organisation; `scion` is someone from an established family. Each also has a `name` in this world's own words and a one-sentence `description`.\n")
	}
	sb.WriteString("- `affiliations`: two to five organisations a player could belong to, named in this world's own words. Do not include 'none' — the program adds an unaffiliated option itself.\n")
	sb.WriteString("- `locations`: three to six places a story could start in, named in this world's own words, each with a one-sentence `description`. The first one is where a player who does not choose begins.\n")

	sb.WriteString("\n## Output\n")
	sb.WriteString("Output only a JSON object, with no Markdown fence and no commentary:\n\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"name\": \"string\",\n")
	sb.WriteString("  \"description\": \"string\",\n")
	sb.WriteString("  \"title\": \"string\",\n")
	sb.WriteString("  \"origin\": \"string\",\n")
	sb.WriteString("  \"protagonist_name\": \"string\",\n")
	sb.WriteString("  \"protagonist\": \"string\",\n")
	sb.WriteString("  \"opening\": \"string\",\n")
	sb.WriteString("  \"world\": [\n")
	sb.WriteString("    {\"name\": \"string\", \"type\": \"string\", \"audience\": \"public|gm\", \"brief\": \"string\", \"content\": \"string\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"characters\": [\n")
	sb.WriteString("    {\"name\": \"string\", \"age\": 0, \"gender\": \"male|female\", \"speech_style\": \"string\", \"role\": \"string\", \"place\": \"string\", \"brief\": \"string\", \"aliases\": [\"string\"]}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"origins\": [\n")
	sb.WriteString("    {\"name\": \"string\", \"description\": \"string\", \"basis\": \"wanderer\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"affiliations\": [\"string\"],\n")
	sb.WriteString("  \"locations\": [\n")
	sb.WriteString("    {\"name\": \"string\", \"description\": \"string\"}\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n\n")
	sb.WriteString("- Omit a field the player already agreed; do not repeat it.\n")
	sb.WriteString("- Keep `name` under 12 characters, `description` under 120, `origin` and `protagonist` under 200, each `brief` under 80, and each `content` under 400.\n")
	sb.WriteString("- Do not add fields, do not wrap the object, and do not write anything outside it.\n")
	return sb.String()
}

func writeWorldSeed(sb *strings.Builder, in WorldCreationSystemInstructionInput) {
	seed := make([]string, 0, 6)
	appendSeed := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			seed = append(seed, fmt.Sprintf("- %s: %s", label, value))
		}
	}
	appendSeed("World name", in.Name)
	appendSeed("Genre", in.Genre)
	appendSeed("Tone", in.Tone)
	appendSeed("The player's character", in.Protagonist)
	appendSeed("How it should open", in.OpeningIdea)
	if len(seed) > 0 {
		sb.WriteString("\n## What the player asked for\n\n")
		sb.WriteString(strings.Join(seed, "\n"))
		sb.WriteString("\n")
	}
	if brief := strings.TrimSpace(in.Brief); brief != "" {
		sb.WriteString("\n## The player's own words\n\n")
		sb.WriteString(brief)
		sb.WriteString("\n")
	}
}

// worldDraftHasContent reports whether a draft says anything worth keeping. An
// empty draft is the ordinary first call, and it must not produce an "already
// agreed" section that tells the model to keep nothing.
func worldDraftHasContent(d WorldDraftInput) bool {
	for _, value := range []string{
		d.Name, d.Description, d.Title, d.Origin,
		d.ProtagonistName, d.Protagonist, d.Opening,
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return len(d.WorldEntries) > 0 || len(d.Characters) > 0 ||
		len(d.Origins) > 0 || len(d.Affiliations) > 0 || len(d.Locations) > 0
}

func writeWorldDraftInput(sb *strings.Builder, d WorldDraftInput) {
	writeDraftLine := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(sb, "- %s (keep verbatim): %s\n", label, value)
		}
	}
	writeDraftLine("World name", d.Name)
	writeDraftLine("Description", d.Description)
	writeDraftLine("Story title", d.Title)
	writeDraftLine("Premise", d.Origin)
	writeDraftLine("Protagonist name", d.ProtagonistName)
	writeDraftLine("Protagonist", d.Protagonist)
	writeDraftLine("Opening", d.Opening)

	if len(d.WorldEntries) > 0 {
		sb.WriteString("\nWorld entries already written (keep verbatim):\n")
		for _, entry := range d.WorldEntries {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				continue
			}
			fmt.Fprintf(sb, "- %s", name)
			if kind := strings.TrimSpace(entry.Type); kind != "" {
				fmt.Fprintf(sb, " [%s]", kind)
			}
			if brief := strings.TrimSpace(entry.Brief); brief != "" {
				fmt.Fprintf(sb, ": %s", brief)
			}
			if audience := strings.TrimSpace(entry.Audience); audience != "" {
				fmt.Fprintf(sb, "（audience=%s）", audience)
			}
			if entry.LoadMode != "" || len(entry.Keywords) > 0 {
				fmt.Fprintf(sb, "（加载方式=%s，关键词=%s）", entry.LoadMode, strings.Join(entry.Keywords, "、"))
			}
			sb.WriteString("\n")
		}
	}
	if len(d.Characters) > 0 {
		sb.WriteString("\nPeople already written (keep verbatim):\n")
		for _, character := range d.Characters {
			name := strings.TrimSpace(character.Name)
			if name == "" {
				continue
			}
			fmt.Fprintf(sb, "- %s", name)
			if character.Age > 0 {
				fmt.Fprintf(sb, " (age: %d)", character.Age)
			}
			if gender := strings.TrimSpace(character.Gender); gender != "" {
				fmt.Fprintf(sb, " (gender: %s)", gender)
			}
			if voice := strings.TrimSpace(character.SpeechStyle); voice != "" {
				fmt.Fprintf(sb, " (voice: %s)", voice)
			}
			if role := strings.TrimSpace(character.Role); role != "" {
				fmt.Fprintf(sb, " — %s", role)
			}
			if place := strings.TrimSpace(character.Place); place != "" {
				fmt.Fprintf(sb, "（常在 %s）", place)
			}
			sb.WriteString("\n")
		}
	}
	if len(d.Origins) > 0 {
		sb.WriteString("\nStarting positions already offered (keep verbatim):\n")
		for _, origin := range d.Origins {
			name := strings.TrimSpace(origin.Name)
			if name == "" {
				continue
			}
			fmt.Fprintf(sb, "- %s", name)
			if basis := strings.TrimSpace(origin.Basis); basis != "" {
				fmt.Fprintf(sb, " [%s]", basis)
			}
			sb.WriteString("\n")
		}
	}
	if len(d.Affiliations) > 0 {
		fmt.Fprintf(sb, "\nAffiliations already offered (keep verbatim): %s\n",
			strings.Join(clipDraftWords(d.Affiliations), "、"))
	}
	if len(d.Locations) > 0 {
		fmt.Fprintf(sb, "\nPlaces already offered (keep verbatim): %s\n",
			strings.Join(clipDraftWords(d.Locations), "、"))
	}
}

func clipDraftWords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// WorldCreationUserMessage is the request itself.
const WorldCreationUserMessage = "Draft the world from the material above."
