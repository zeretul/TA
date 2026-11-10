package teller

// The six recommended narrative styles.
//
// The reference library's twelve public styles collapsed into six after the
// same-scene trials showed the pairs saying the same thing twice. Each of the
// six keeps one existing stable id, so a story that already selected it keeps
// resolving; the six that were merged away stay readable through
// legacyStyleAliases below.
//
// Two things are deliberately excluded:
//
//   - The old library's world rules, prices, currencies, timestamps, forced
//     cast rotation, per-turn settlement and per-style word limits. Those
//     belong to a world pack, a gameplay module or the base's own length
//     behaviour; carrying them here would let a style change what happens
//     instead of only how it is told.
//   - The old hard_rules (regex/word-count/pattern validators). The expression
//     requirements they encoded are stated as prompt guidance, which is what
//     they actually were.
//
// Every style is theme-neutral: none assumes a cultivation setting, a young
// protagonist, spirit stones, or a particular calendar.

var migratedTellers = map[string]Definition{
	"source-drama": migratedStyle(migratedStyleSpec{
		ID: "source-drama", Name: "朴实正剧", NameEN: "Grounded Drama",
		Description:   "叙述清楚，人物踏实，动作与对白自然接续。",
		DescriptionEN: "Clear narration, grounded characters, action and dialogue that follow each other naturally.",
		Order:         1,
		System:        sourceDramaSystem, Turn: sourceDramaTurn,
		Dialogue: sourceDramaDialogue, Combat: sourceDramaCombat,
	}),
	"straight-comedy": migratedStyle(migratedStyleSpec{
		ID: "straight-comedy", Name: "轻松诙谐", NameEN: "Light Humour",
		Description:   "吐槽更足，反差有包袱，旁白更有笑意。",
		DescriptionEN: "More banter, situational contrast that lands, and a narrator with a sense of fun.",
		Order:         2,
		System:        straightComedySystem, Turn: straightComedyTurn,
		Dialogue: straightComedyDialogue, Combat: straightComedyCombat,
	}),
	"slice-banter": migratedStyle(migratedStyleSpec{
		ID: "slice-banter", Name: "温情日常", NameEN: "Everyday Warmth",
		Description:   "烟火小事与真实相处，有亲疏，也有人情温度。",
		DescriptionEN: "Small daily business and real company, with distance kept where it belongs and warmth where it is earned.",
		Order:         3,
		System:        sliceBanterSystem, Turn: sliceBanterTurn,
		Dialogue: sliceBanterDialogue, Combat: sliceBanterCombat,
	}),
	"strange-tales": migratedStyle(migratedStyleSpec{
		ID: "strange-tales", Name: "悬疑冷峻", NameEN: "Suspense and Restraint",
		Description:   "措辞克制，细节留有余味，已知与未知分得清。",
		DescriptionEN: "Restrained wording, detail that lingers, and a clear line between what is known and what is not.",
		Order:         4,
		System:        strangeTalesSystem, Turn: strangeTalesTurn,
		Dialogue: strangeTalesDialogue, Combat: strangeTalesCombat,
	}),
	"hotblooded-combat": migratedStyle(migratedStyleSpec{
		ID: "hotblooded-combat", Name: "热血动作", NameEN: "Kinetic Action",
		Description:   "动词有力，方位清晰，紧要处节奏利落。",
		DescriptionEN: "Strong verbs, clear positions, and a clean rhythm where it counts.",
		Order:         5,
		System:        hotbloodedCombatSystem, Turn: hotbloodedCombatTurn,
		Dialogue: hotbloodedCombatDialogue, Combat: hotbloodedCombatCombat,
	}),
	"cinematic-shots": migratedStyle(migratedStyleSpec{
		ID: "cinematic-shots", Name: "电影叙事", NameEN: "Cinematic Narrative",
		Description:   "视听、动作与对白带着场景走，读来如在眼前。",
		DescriptionEN: "Sight, sound, action and speech carry the scene, so it reads as if it were in front of you.",
		Order:         6,
		System:        cinematicShotsSystem, Turn: cinematicShotsTurn,
		Dialogue: cinematicShotsDialogue, Combat: cinematicShotsCombat,
	}),
}

// legacyStyleAliases are the ids the six styles absorbed. They are not offered
// in the picker; they exist so a story that saved one of them still loads.
//
// The prompt comes from the style it merged into, because the six names must
// not be six ways of saying the same thing twice over. Only the display strings
// stay as they were, so a returning player still sees the name they chose.
var legacyStyleAliases = map[string]legacyStyleSpec{
	"marketplace-light": {
		Name: "市井轻快", NameEN: "Marketplace Light",
		Description:   "生活细节、交涉人情与利落对白，轻快而不轻浮。",
		DescriptionEN: "Daily detail, negotiation, obligation and brisk dialogue — light without being weightless.",
		Target:        "slice-banter",
	},
	"flippant-banter": {
		Name: "轻佻吐槽", NameEN: "Flippant Banter",
		Description:   "俏皮叙述、自嘲、嘴硬与处境反差，不替玩家挑衅或吹牛。",
		DescriptionEN: "Playful narration, self-mockery, stubborn front and situational contrast — never taunting or boasting on the player's behalf.",
		Target:        "straight-comedy",
	},
	"deadpan-humor": {
		Name: "冷面幽默", NameEN: "Deadpan Humour",
		Description:   "平实冷静地陈述事实，荒谬由事实并置自行浮现。",
		DescriptionEN: "Facts stated flatly and calmly, so the absurdity surfaces from the juxtaposition itself.",
		Target:        "straight-comedy",
	},
	"restrained-hard": {
		Name: "克制冷硬", NameEN: "Restrained Hard",
		Description:   "动作、选择与已确定的代价，少形容词，不抒情。",
		DescriptionEN: "Action, choice and settled cost; few adjectives and no lyricism.",
		Target:        "strange-tales",
	},
	"intrigue-ensemble": {
		Name: "权谋群像", NameEN: "Intrigue Ensemble",
		Description:   "立场、条件、动机与信息差，不强迫所有人算计。",
		DescriptionEN: "Position, terms, motive and unequal knowledge — without making everyone a schemer.",
		Target:        "source-drama",
	},
	"dialogue-piece": {
		Name: "对白戏", NameEN: "Dialogue Piece",
		Description:   "对白交锋、简短必要的动作、称谓差异与沉默。",
		DescriptionEN: "Dialogue as exchange, with only the action it needs, attention to how people address each other, and room for silence.",
		Target:        "cinematic-shots",
	},
}

// groupRecommended is the single group the six recommended styles occupy, so
// the picker shows them in the design's own order rather than an alphabetical
// accident.
const groupRecommended = "recommended"

type migratedStyleSpec struct {
	ID, Name, NameEN, Description, DescriptionEN string
	Order                                        int
	System, Turn, Dialogue, Combat               string
}

// migratedStyle assembles one style's four purpose fragments. A style with no
// dialogue or combat fragment would silently fall back to the main voice for
// that purpose, which is the "six names, one prompt" failure the design
// forbids — so the fragments are required rather than optional.
func migratedStyle(spec migratedStyleSpec) Definition {
	return Normalize(Definition{
		Version:       tellerVersion,
		ID:            spec.ID,
		Name:          spec.Name,
		NameEN:        spec.NameEN,
		Description:   spec.Description,
		DescriptionEN: spec.DescriptionEN,
		Group:         groupRecommended,
		GroupOrder:    0,
		Order:         spec.Order,
		ContextPolicy: ContextPolicy{Creator: "always", Lore: "relevant", RuntimeState: "always"},
		Slots: []PromptSlot{
			{ID: "voice", Name: "叙事声口", Target: TargetSystem, Enabled: true, Content: spec.System},
			{ID: "turn", Name: "本轮推进", Target: TargetTurnContext, Enabled: true, Content: spec.Turn},
			{ID: "dialogue", Name: "对白表达", Target: TargetDialogue, Enabled: true, Content: spec.Dialogue},
			{ID: "combat", Name: "交手描写", Target: TargetCombat, Enabled: true, Content: spec.Combat},
		},
	})
}

type legacyStyleSpec struct {
	Name, NameEN, Description, DescriptionEN string
	Target                                   string
}

// legacyStyle resolves one merged-away id to the style that now carries its
// expression, keeping the old display strings.
func legacyStyle(id string, spec legacyStyleSpec) Definition {
	style, ok := migratedTellers[spec.Target]
	if !ok {
		return Definition{}
	}
	style.ID = id
	style.Name = spec.Name
	style.NameEN = spec.NameEN
	style.Description = spec.Description
	style.DescriptionEN = spec.DescriptionEN
	return Normalize(style)
}

const sourceDramaSystem = `用清楚、踏实的叙述讲眼前的事，平衡动作、必要细节和自然对白。人物分量来自具体选择与反应，不靠旁白赞叹。词语随世界调整，不预设主角年轻、贫穷、天才或平庸。多人的牵挂可以从称呼和反应显出，不强加权谋与谈判。`

const sourceDramaTurn = `跟随玩家当前意图，写清眼前的回应，停在下一步需要玩家决定的地方。一句回答或安静片刻也成立，不强求数字变化、付费或事件升级。`

const sourceDramaDialogue = `人物按身份与熟悉程度说普通的话。允许自然停顿、抱怨、闲聊和直截了当的答复，不把每句对白写成漂亮论点或条件清单。`

const sourceDramaCombat = `朴实地呈现已经结算的交手，交代清楚方位、接触与真实后果。人物始终在场，不添额外招式、损伤或声势。`

const straightComedySystem = `把叙事写得明显有趣，而不只是偶尔换个俏皮词。适合的日常、交涉和小尴尬场景里，主动从已有处境提炼笑点：一本正经与现实的落差、话说得漂亮而手上忙乱、人物认真过头的小执念、物件与遭遇的贴切比拟。旁白可以更爱吐槽，敢于用短促的插话、反高潮和干脆收尾形成包袱，让读者读得到喜感。吐槽贴着眼前人事，不插入通用段子；同一种反转句和比喻不用来回套。人物仍各有脾气，不变成齐声接梗的相声班。`

const straightComedyTurn = `让本来发生的事自然展开，在有材料的地方多写出一点吐槽与叙事笑点，不用一味克制把此风格磨成普通正剧。可先平实铺一个已有细节，再用现实反差轻轻翻转；也可回扣前文已经建立的笑点，但要有新角度，不复读。不强制每段一个包袱，不靠新增事故、损失、荒唐手续或额外任务凑笑。玩家面前的真实问题仍要清楚答复，笑过之后事情接得下去。`

const straightComedyDialogue = `适合开玩笑的人物可以有更鲜活的抱怨、打趣、装镇定和自圆其说，熟人之间可顺势接话。严肃、拘谨或不熟的人照自己的口吻回答，叙述可以观察反差，不强迫他们讲笑话。保留玩家实际说法，不替玩家补挑衅、承诺、吐槽金句或内心结论；笑点不靠把人物写蠢或让别人受辱。`

const straightComedyCombat = `实际危险中保证动作清楚与紧张，合适的既定细节或暂歇处可以有一句干净的吐槽，战后也可回扣此前笑点。真正重伤、悲痛或紧要关头收住插科打诨；不靠滑倒、羞辱或新增损失博笑，不让笑点改写伤势与胜负。`

const sliceBanterSystem = `关注普通人的日子与相处，让职业习惯、街巷声气和细小举动带出生活温度。写得耐心而不拖沓，亲切而不煽情。交情远近依已有事实，不把陌生人写得过度亲密，也不把日常全部写成买卖、人情债或资源计算。`

const sliceBanterTurn = `按眼前小事的分量写，不强加差事、奖励或关系进度。可以只是有人把话听完、做完手上的活或安静等候；不替玩家决定感激、信任或留下。`

const sliceBanterDialogue = `对白贴近日常，熟人之间的省略、关心和适度玩笑自然出现，陌生人保留自己的分寸。职业口吻可以不同，不是人人温柔劝慰或讲人生道理。`

const sliceBanterCombat = `实际交战时收起温软语气，照实写清动作与后果。战后已有的人情反应可以留下，不凭温情补血、送物或抹去损失。`

const strangeTalesSystem = `用克制、准确的语言写可见动作和已有疑点，让停顿、前后不合或信息缺口留下余味。情绪多落在行为与措辞里，不堆恐怖形容。没有疑点时保持沉静，不凭空造谜；不否认世界中真实存在的超自然事物，也不凭风格添鬼怪、线索或坏人。`

const strangeTalesTurn = `写出当前事实允许的观察与回应，不替玩家推理定案，不藏起已给出的答案。普通结果仍可普通，不强制付出代价、失败或遭到新威胁。`

const strangeTalesDialogue = `对白简洁具体，回避或含混必须符合该人物与情境。该直说时直说，不让人人故作高深、互相试探或永远只答半句。`

const strangeTalesCombat = `用清楚的动作、声音和身体接触呈现已定交手，语气收紧但事实完整。不能借视线不清藏掉必要结算，也不新增残酷后果。`

const hotbloodedCombatSystem = `有真实动作时突出方向、距离、接触与回转，以句子长短控制紧迫感。强度随实际局势上升，不把普通问话写成誓师。主角是否勇敢、进攻或撤退由已有选择决定，气势与意志不能改变胜负。`

const hotbloodedCombatTurn = `让已经发生的行动按清楚次序展开，写明可见变化。平静场景可以保留干净利落的节奏，但不能为了热血开战、支配玩家或强迫马上行动。`

const hotbloodedCombatDialogue = `危险中的话可以短促，像提醒、呼喊或必要回应；闲聊时恢复人物平常口吻。不增加玩家没说过的豪言、宣誓和挑衅。`

const hotbloodedCombatCombat = `准确写出已定交手的接近、接触与回势，保持空间连续。用节奏呈现力度，不补新攻击、翻盘、伤害、灵力或胜负。`

const cinematicShotsSystem = `把已有画面、声音、人物动作和对白接续起来，让注意力有清楚落点。视听信息服务于场景，不把人物隐秘心思当成可拍到的事实。对话场景由人物声音领着走，动作场景保持位置连续。仍写小说自然段，不加镜头命令、场号或剧本标签。`

const cinematicShotsTurn = `选少量真正有意义的观察推进当前一拍，不逐件盘点所有物件。问题已经提出时可以直接进入NPC回应，停在自然的画面、动作或对白上，不为了结尾另造发现。`

const cinematicShotsDialogue = `用人物直接说的话承载交往，辅以必要的眼神、停顿或手上动作。称呼和应答方式可显关系，不把潜台词逐句翻译成旁白，也不替玩家补下一句。`

const cinematicShotsCombat = `让已定动作在空间中连贯可读，视点变化不改变人物位置或时间。必要对白可点到为止，不能靠台词决定原本没有的结果。`
