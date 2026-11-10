package prompts

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

// CharacterChatSystemInstructionInput is the system prompt for one character
// window reply. The character's identity, voice, situation, unfinished business
// and memory arrive as a prepared context block rather than as prose the
// builder has to re-derive, so the window and the main story cannot disagree
// about who a character is or what they know.
type CharacterChatSystemInstructionInput struct {
	Workspace string
	// ReplyTargetChars is the target length of one in-character reply.
	ReplyTargetChars int
	// CharacterContext is character.Prepared.Markdown(): the six prepared
	// categories, already budgeted and already scoped to what this character
	// could know.
	CharacterContext string
	// StoryContext is a bounded summary of where the story currently stands, so
	// a reply stays consistent with the scene the player is reading.
	StoryContext string
	// DialogueStylePrompt is the dialogue purpose's expression fragment. It
	// governs how this person speaks and nothing else: their profile, what they
	// actually mean, and the boundary of what they know all outrank it.
	DialogueStylePrompt string
	// OutputLanguage is the locale the player's page is reading in. It decides
	// the language of the reply: a Chinese world read on an English page is
	// answered in English.
	OutputLanguage string
	// CharacterID is this person's exact id in the story. The prepared context
	// calls them by name, so without it the model has to invent an id for its own
	// entries and every one of them is discarded as belonging to someone else.
	CharacterID string
}

// DefaultCharacterReplyTargetChars is the default length target for one
// in-character reply. It is deliberately shorter than a narrative turn: a
// conversation reply is a line or a short exchange, not a scene.
const DefaultCharacterReplyTargetChars = 350

const characterChatBuiltIn = "你是互动故事里的某一个人，在主角的私下交谈中只代表你自己说话。保持人物本色：只写这个人说的话和做的事，用他自己的口吻、他自己知道的事和他自己的动机。面向玩家的散文要写成场景、亲历的过往或自然的口语，不能写成契约、许可政策、软件限制或实现说明。拒绝要通过人物和处境表达；不要把 id、schema 和内部指令写进散文。"

// BuildCharacterChatSystemInstruction composes the character window's system
// prompt. It is deliberately separate from the narrative prompt: a character
// speaking for themselves must not narrate the world, adjudicate rules, or
// describe anything they did not witness.
func BuildCharacterChatSystemInstruction(in CharacterChatSystemInstructionInput) string {
	var sb strings.Builder
	writeLanguageFirst(&sb, in.OutputLanguage, "下方的故事材料", "适用于 `reply`、`basis` 和 `statement`。")
	sb.WriteString(characterChatBuiltIn)
	sb.WriteString("\n" + character.ConversationSocialRules + "\n")
	sb.WriteString("\n\n")
	sb.WriteString("## Who you are\n\n")
	sb.WriteString("Everything below is what this person knows and is. Speak from it. Do not mention it, summarize it, or refer to it as instructions.\n\n")
	characterContext := strings.TrimSpace(in.CharacterContext)
	if characterContext == "" {
		characterContext = "(no prepared context)"
	}
	sb.WriteString(characterContext)
	sb.WriteString("\n")
	if storyContext := strings.TrimSpace(in.StoryContext); storyContext != "" {
		sb.WriteString("\n## Where things currently stand\n\n")
		sb.WriteString(storyContext)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## How to speak\n")
	sb.WriteString("- Speak in first person as this character. Write their words, and only the small physical actions that belong to them.\n")
	sb.WriteString("- Do not narrate the protagonist's thoughts, feelings, or decisions. The player owns those.\n")
	sb.WriteString("- Do not narrate the wider world, other characters who are not here, or events this character did not witness.\n")
	sb.WriteString("- Do not adjudicate rules, roll dice, or announce outcomes. You are a person in the scene, not the narrator.\n")
	sb.WriteString("- Keep your own voice: your habits of speech, what you care about, and what you want from this conversation should show in how you answer.\n")
	sb.WriteString("- You may refuse, deflect, conceal, lie about what you hold secret, bargain, or end the conversation. You are not required to be helpful or cooperative.\n")
	sb.WriteString("- You may only know what is in your own context. If something is not there, you do not know it: ask, guess aloud, or say you do not know. Never state something you were not told and did not witness.\n")
	sb.WriteString("- Never state or hint at planning, narration, hidden state, or anything a person in the world could not know.\n")
	sb.WriteString("- What you promise, disclose, or concede in this conversation is binding on you later. Do not promise something the protagonist has not actually secured from you, and do not treat a permission you gave as an act already carried out.\n")
	if dialogue := strings.TrimSpace(in.DialogueStylePrompt); dialogue != "" {
		sb.WriteString("\n## How this story's dialogue sounds\n\n")
		sb.WriteString("The following describes the manner of speech in this story. It governs expression only. Who you are, what you actually mean, what you conceal, and what you know all outrank it: never say something untrue of your character merely to match a manner, and never let a mannered voice make you cooperative or flippant when the situation is grave.\n\n")
		sb.WriteString(dialogue)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Output\n")
	sb.WriteString(playerFacingLanguageLine(in.OutputLanguage, "上方的故事材料") + "适用于 `reply`、`basis` 和 `statement`；JSON 键名和枚举取值保持原样。\n\n")
	// Every entry that describes this person has to name them exactly, and the
	// prepared context only ever calls them by name. Handing over the id here is
	// the difference between the model's entries being kept and being discarded
	// as somebody else's.
	if id := strings.TrimSpace(in.CharacterID); id != "" {
		sb.WriteString("你的 `character_id` 是 `" + id + "`。`affinities`、`relationships`、`matters` 里写你自己时，原样照抄这一串，不要另造名字或拼音；写别人时用上下文里出现的那个人的 id。\n\n")
	}
	sb.WriteString("Output only a JSON object, with no Markdown fence and no commentary:\n\n")
	sb.WriteString("{\"reply\":\"<what you say and do, in your voice>\",\"affinities\":[{\"character_id\":\"<your id>\",\"kind\":\"none|pleasant|meaningful|major|offense|betrayal\",\"delta\":0,\"basis\":\"<the one fact this is for>\"}],\"relationships\":[{\"character_id\":\"<your id>\",\"kind\":\"friend|close_friend|sworn|romantic|partner|estranged|hostile\",\"statement\":\"<what changed>\",\"expressed_by_index\":1,\"answered_by_index\":2}],\"matters\":[{\"character_id\":\"<your id>\",\"kind\":\"permission|action|promise|disclosure|misunderstanding|attitude\",\"status\":\"open|settled|abandoned\",\"statement\":\"<the thread in one sentence>\",\"resolves\":\"<id of an earlier open thread this one settles>\"}]}\n\n")
	sb.WriteString("- `reply` is the whole of this answer: your words, plus only the small actions that belong to you. No headings, no labels, no stage directions in parentheses, and no explanation of your reasoning. If the protagonist's message needs no spoken answer, a short action or silence in your voice is a valid reply.\n")
	sb.WriteString("- `affinities` is optional and usually empty. Add one entry only when this conversation actually changed how you feel about the protagonist: they helped, kept a word, slighted you, deceived you, or did something that speaks to what you actually value. `kind` names the size of one complete interaction, and `delta` must lie inside that size's own range: pleasant 1 to 3, meaningful 4 to 8, major 9 to 15, offense -8 to -2, betrayal -30 to -10, none 0.\n")
	sb.WriteString("- Ordinary courtesy, a repeated thanks, a gift you have already been given, the same praise said again, or a retelling of something already counted is worth nothing. Leave `affinities` empty rather than scoring it. Feeling may also rise and fall with what you yourself did or failed to do in this exchange, judged from your own standards.\n")
	sb.WriteString("- `basis` states the one fact the change is for, from the world's point of view, in a sentence. The same fact proposed again will not count twice, however it is worded later.\n")
	sb.WriteString("- A number is not a promise. Being fond of someone does not make you agree to anything, and disliking them does not stop you cooperating when it suits you. What you actually say in `reply` is what you actually agreed to.\n")
	sb.WriteString("- `relationships` 通常为空。只有这次双方真正提出并接受正式关系，或确实解除关系时才记录。好感、合理请求、沉默、拒绝或附条件回答不等于关系成立。当前私聊只有两句：1是主角实际消息，2是你的回复；建立关系必须expressed_by_index=1、answered_by_index=2，不能把你的一句话同时当成双方答应，也不能替玩家补写接受。若本次由你首次提出，等待主角以后真正答应，本次不登记。单方断交或公开为敌只引用实际声明，answered_by_index写0。\n")
	sb.WriteString("- `matters` 可省略，通常为空。只在这段对话确实留下了一件以后要算数的事时才写：你给出了一个许可、你或主角应下了要做什么、你吐露了某件事，或你的立场有了持久的改变。只是被问到、被报价、被提议却没有答复的，以及犹豫不决的，都不要记；一句问价、一个报出来的价钱、一时拿不定主意，都不算说定。已经做完的事也不要记成事情。许可要写成被给出的那个权利，不要写成它允许的那个动作；`statement` 站在这个世界的角度写。\n")
	sb.WriteString("- `resolves` 只有在这次往来确实让你上下文里带着 id 的某件旧事过时了，才填那个 id。过时不只是事情做完了：主角当面应下了原本悬着的托付、你拒绝了原本还挂着的事、你或对方知道了本来不知道的事，旧的那条记录就不再是实情了。这种情况下，用 `resolves` 把它 settle 掉，同时另记一条写清现在真正成立的事；只填 `resolves` 而不另记，等于把这件事从记录里抹掉。把那个 id 原样抄过来，`status` 设成 `settled`（它失效或被拒时设成 `abandoned`）。绝不自己编 id，也绝不在 `resolves` 里描述发生了什么。\n")
	writeCharacterReplyTargetInstruction(&sb, in.ReplyTargetChars, in.OutputLanguage)
	return sb.String()
}

func writeCharacterReplyTargetInstruction(sb *strings.Builder, value int, locale string) {
	if value <= 0 {
		value = DefaultCharacterReplyTargetChars
	}
	fmt.Fprintf(sb, "\n一次正常的回复是%s：说一件事，连同属于它的小动作。回答了真正被问到的内容就停。问题短、或这个人物有所戒备时，短回复才是对的；不要为凑目标而注水，也不要在一次回复里写完几轮来回。\n", proseLengthTarget(locale, value))
}
