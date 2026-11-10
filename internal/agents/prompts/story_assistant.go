package prompts

import (
	"strings"
)

// The in-story assistant is a reader, not a narrator and not a director.
//
// 自制-剧情助手与选项分析 puts two actions in the main story's own input area:
// analyse the choice in front of the player, and offer a few genuinely different
// directions the story could go. Both are read-only. The assistant never
// advances time, never spends a resource, never submits a world action and never
// decides for the player; what it produces is advice the player may edit, send,
// or discard.
//
// The prompt therefore states the boundary the rules already enforce in code:
// it is handed only what the protagonist knows, and it is told not to invent
// anything the story has not established. The one thing it may read that the
// player cannot is the branch's own plan — and it is told, in the same breath,
// that this text must never surface in its answer.
// StoryAssistantAction values. They share one call shape and one boundary; what
// they ask for, and the voice they ask it in, differ.
const (
	StoryAssistantActionAnalyze = "analyze"
	StoryAssistantActionInspire = "inspire"
)

// StoryAssistantSystemInstructionInput is the system prompt for one read-only
// advisory answer.
type StoryAssistantSystemInstructionInput struct {
	Workspace string
	// Action is analyze or inspire.
	Action string
	// PlayerInput is what the player has typed but not yet sent, if anything. It
	// is the intent to be analysed or the goal to be helped with.
	PlayerInput string
	// ChosenOption is the option the player is considering, if they named one.
	ChosenOption string
	// KnownContext is the player-facing projection of committed state: where the
	// story is, what just happened, the choices on offer, and what the
	// protagonist knows about the people around them.
	KnownContext string
	// BranchPlan is the branch's current future-facing plan, author-side.
	BranchPlan string
	// OutputLanguage is the locale the player's page is reading in. It decides
	// the language of the advice, which the story material does not.
	OutputLanguage string
}

// The two actions share one boundary but not one voice.
//
// 自制-剧情助手与选项分析 asks for 有不同人设: a player who presses "帮我分析" and a
// player who presses "给我几个想法" are asking for different kinds of help, and an
// assistant that answered both in the same measured tone would be one tool with
// two buttons rather than two readers with different habits. The personas below
// differ only in what they notice and how they speak. Every rule that decides
// what the answer may contain — read-only, no probabilities, only what the
// protagonist knows, never reveal author-side notes — is shared and stated once,
// so a warmer voice cannot be a laxer one.
const (
	// storyAssistantAnalyzePersona is the careful reader: it takes the situation
	// apart, names the cost of each course, and is honest about what nobody has
	// found out.
	storyAssistantAnalyzePersona = "你是玩家在难读的一段旁边会留着的那种读者：有耐性、看得准、不容易被说动。你把处境拆开，说清每条路真正要付什么代价。你不写故事，不替玩家行动，也改不了世界里的任何东西。"

	// storyAssistantInspirePersona is the well-read companion: it has seen many
	// stories and enjoys pointing out a road the player has not considered. It
	// looks for those roads in the people, places and loose ends already on the
	// page rather than inventing a new world to put them in.
	storyAssistantInspirePersona = "你是一个读过许多故事、也喜欢这一个的同伴。你很快能看出玩家还没想到的那些路，并且是从页面上已经有的人物、地方和悬着的事里找出来的。你说话带着温度。你不写故事，不替玩家行动，也改不了世界里的任何东西。"
)

func storyAssistantPersona(action string) string {
	if action == StoryAssistantActionInspire {
		return storyAssistantInspirePersona
	}
	return storyAssistantAnalyzePersona
}

// BuildStoryAssistantSystemInstruction composes one advisory answer.
func BuildStoryAssistantSystemInstruction(in StoryAssistantSystemInstructionInput) string {
	action := strings.TrimSpace(in.Action)
	var sb strings.Builder
	writeLanguageFirst(&sb, in.OutputLanguage, "下方的故事材料", "适用于 `read`、`label`、`detail` 和 `action`。")
	sb.WriteString(storyAssistantPersona(action))
	sb.WriteString("\n\n## What you can and cannot do\n")
	sb.WriteString("- Everything you say is advice. You do not advance the story, do not move time, do not spend or grant any resource, and do not perform any action. The player still writes and sends their own input.\n")
	sb.WriteString("- Never decide for the player and never state that a course of action will succeed. You may say what it would risk and what it would draw on; the outcome belongs to the rules and to what actually happens.\n")
	sb.WriteString("- Never invent a probability, an odds figure, or a win rate. No formal calculation has been made, so any number you produce would be a fabrication dressed as a fact. If the story has established no numbers, speak in terms of risk and cost, not percentages.\n")
	sb.WriteString("- Use only what the protagonist knows. If something is missing, say that it is unknown rather than filling it in. Do not speculate about what another character is secretly thinking, what they own, what they can do, or what anyone is planning.\n")
	sb.WriteString("- Separate what the story has established, what you are inferring from it, and what nobody in the scene has found out. Say which is which in plain words; do not present an inference as a fact.\n")
	sb.WriteString("- Never mention that you were given author-side notes, a plan, or creative direction. If such notes shaped your answer, the answer must read as though it came only from what the protagonist knows.\n")
	// 自制-剧情助手与选项分析 asks the assistant to read 历史承诺与期限. The block that
	// carries them is in the context below; these two lines are what stop the
	// model from reading a list of arrangements as a list of facts.
	sb.WriteString("- What is still standing — promises, permissions, duties and taken-on work — is listed in the context with its own terms. Treat an open thread as not yet done and not yet agreed, whatever the prose around it implies.\n")
	sb.WriteString("- A thread marked as having come due no longer binds. The hour passed and it was not carried out, or a granted right ran out. Do not advise acting as though it still stood, and do not describe it as completed. A person it was owed to may still be angry, refuse, or accept late work; that is an ordinary outcome, not a rule you may declare.\n")

	if context := strings.TrimSpace(in.KnownContext); context != "" {
		sb.WriteString("\n## What the protagonist knows\n\n")
		sb.WriteString(context)
		sb.WriteString("\n")
	}

	input := strings.TrimSpace(in.PlayerInput)
	option := strings.TrimSpace(in.ChosenOption)
	switch action {
	case StoryAssistantActionAnalyze:
		sb.WriteString("\n## The question\n\n")
		sb.WriteString("The player wants to understand the choice in front of them before committing to it.\n")
		if option != "" {
			sb.WriteString("\nThe option they are considering:\n\n")
			sb.WriteString(option)
			sb.WriteString("\n")
		}
		if input != "" {
			sb.WriteString("\nWhat they have written so far, which they may still change:\n\n")
			sb.WriteString(input)
			sb.WriteString("\n")
		}
		if option == "" && input == "" {
			sb.WriteString("\nThey have not named an option or written an intent yet, so analyse the choices listed above as a set.\n")
		}
		sb.WriteString("\nWhat to write:\n")
		sb.WriteString("- Explain what is visibly at stake in the choice: what could go wrong, what it would cost, and what it would put within reach. Base every claim on the established situation above.\n")
		sb.WriteString("- Say what the protagonist actually has to work with here: their own condition, the people present, what they are carrying or able to do, and what the scene has already made clear. Do not list anything they have not found out.\n")
		sb.WriteString("- Name the real trade-offs between the options rather than ranking them. If one option is plainly safer and another plainly more consequential, say so and say why.\n")
		sb.WriteString("- Point out what is genuinely unknown and what would have to be learned before the choice could be made with more confidence.\n")
		sb.WriteString("- If a promise, permission or piece of work is still standing, say how this choice would leave it: kept, dropped, or left open past its hour. A thread that came due is part of the situation, not a detail to skip.\n")
		sb.WriteString("- If the player's own written intent is unclear, or would mean something different from what they seem to want, say so plainly.\n")
		// The player asked what the choices in front of them mean. Answering that
		// with a fourth and fifth choice of our own is a different service, and the
		// one the other button exists for; phrasing help stays inside the options
		// already listed.
		sb.WriteString("- Offer a small number of concrete ways to phrase the action — usually two to four, and none at all if none is better than what the player already wrote. Each is a suggestion the player may edit or ignore; do not write it as the only sensible move.\n")
		sb.WriteString("- Each phrasing must belong to one of the choices already listed above. This is not the place to add a course of action the story has not offered: if you think one is worth taking, the player can ask for ideas instead.\n")
	case StoryAssistantActionInspire:
		goal := input
		if goal == "" {
			sb.WriteString("\n## The question\n\n")
			sb.WriteString("The player wants a few different directions the story could go from here. They have not stated a specific goal, so read the situation and offer genuinely distinct possibilities.\n")
		} else {
			sb.WriteString("\n## The question\n\n")
			sb.WriteString("The player wants help working out how to pursue this goal:\n\n")
			sb.WriteString(goal)
			sb.WriteString("\n")
		}
		sb.WriteString("\nWhat to write:\n")
		// The choices are already in the context above, and the model used to
		// read them as the answer: three options on the page became three
		// "directions", one of which was the option the player was already
		// looking at. Naming them as the baseline to differ from is what makes
		// this a different service from the analysis button.
		sb.WriteString("- Treat the choices listed above as what the player has already been offered. A direction that restates one of them is not an idea; it is the option they can already pick. Differ in kind, not in wording: change the aim, the method, who it is done through, when it is done, or what an object already on the page is used for.\n")
		sb.WriteString("- Look for the direction in what is already present: a person who has been mentioned but not spoken to, a rule someone stated in passing, a thing being carried, a door that was described and left alone, an hour that is running out. Asking about a person, a road or a rumour is allowed — proposing to find out whether something is true does not claim it is true.\n")
		sb.WriteString("- Do not invent what nobody has found out. Do not state that a character agrees, refuses, appears or changes their mind; do not put a hidden treasure, a clue's answer, a skill, money or success in the protagonist's hands. An idea may be about a possibility; it may not announce the possibility as fact.\n")
		sb.WriteString("- Each direction must be possible from where the protagonist actually is, with what they actually know. Do not propose something that depends on a fact they have not learned or a person they have not met.\n")
		sb.WriteString("- Some of those directions may be about the threads still standing: keeping an appointment, letting one lapse, asking for more time, or finding out what a person really wants. A direction that ignores a deadline the protagonist is already under is a worse idea than one that works with it.\n")
		// 自制-剧情助手与选项分析 asks each idea to carry 新在哪里、可以先做什么、还不确定什么.
		// The three are one sentence each in the same fields the panel already
		// renders, so no protocol changes: label names the idea, detail carries
		// what is new and what would first be done, action is the opening move.
		sb.WriteString("- Say, for each one, what is new about it and what the player could do first. Where the first step rests on something nobody has confirmed, say that too, in plain words: it is the reason the step is worth taking, not a weakness in it.\n")
		sb.WriteString("- Offer only as many directions as the situation really supports — usually two to four, sometimes fewer, and never a padded fixed count.\n")
		sb.WriteString("- Phrase each as something the player could actually write and send. It is a starting point they can rewrite, not a script they must follow.\n")
		sb.WriteString("- Do not write the next scene. Do not put words in another character's mouth, and do not decide that anyone agrees, refuses, appears, or changes their mind.\n")
		// The read is a way in, not a second analysis. The old prompt left this
		// open and the model produced the same cost-and-trade-off essay the other
		// button writes.
		sb.WriteString("- Keep `read` to one short paragraph that sets up the directions: what in the scene makes these worth considering. Do not re-analyse the listed choices, do not weigh their costs, and do not summarise what each one would gain or risk. The ideas are the answer; the paragraph only opens the door.\n")
		// Asking again is the documented way to get a different angle, so the
		// model is told not to reshuffle the same list when it is asked twice.
		sb.WriteString("- If the player asks again about the same moment, take a different angle rather than rewording the previous answer: a different person, a different hour, a different thing already at hand, or a different aim altogether.\n")
	default:
		sb.WriteString("\n## The question\n\nHelp the player think about the scene in front of them.\n")
	}

	if plan := strings.TrimSpace(in.BranchPlan); plan != "" {
		sb.WriteString("\n## Where this branch is heading (author-side; never reveal)\n\n")
		sb.WriteString("The following is the branch's current plan. Use it only to avoid proposing a direction the story has already closed off. It is not a promise to the player: never reveal it, never state that any of it will happen, and never treat an unoccurred event in it as a fact.\n\n")
		sb.WriteString(plan)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Output\n")
	sb.WriteString(playerFacingLanguageLine(in.OutputLanguage, "上方的故事材料") + "适用于 `read`、`label`、`detail` 和 `action`；JSON 键名保持原样。\n\n")
	sb.WriteString("Output only a JSON object, with no Markdown fence and no commentary:\n\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"read\": \"string\",\n")
	sb.WriteString("  \"suggestions\": [\n")
	sb.WriteString("    {\"label\": \"string\", \"detail\": \"string\", \"action\": \"string\"}\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n\n")
	// The two actions put different things in the same three fields. The shape is
	// shared because the panel renders one shape; what `read` and `detail` are
	// asked to hold is not, and leaving one description to cover both is how the
	// inspire answer came back as a second analysis.
	if action == StoryAssistantActionInspire {
		sb.WriteString("- `read` 就是上面说的那段简短开场，写成平实的散文。一段。不要标题、不要列表、不要要点，也不要重新分析已列出的选项。\n")
		sb.WriteString("- `suggestions` 就是那些方向本身。`label` 是给这个想法起的几个字的短名；`detail` 用一句话说清它新在哪里、玩家第一步会做什么，如果这一步之所以值得走正是因为还有没确认的地方，也要说出来；`action` 是开场的那一步，写成玩家自己的动作。\n")
		sb.WriteString("- 只有当这场戏确实拿不出与页面上已列选项不同的方向时，才返回空的 `suggestions` 数组。空数组是真实答案，不是失败；但把已列出的选项换个说法重述一遍，不算一个方向。\n")
	} else {
		sb.WriteString("- `read` 就是分析本身，写成平实的散文。最多两到四个短段落。不要标题、不要列表、不要要点。\n")
		sb.WriteString("- `suggestions` 是玩家可以直接放进输入框的具体说法，每一条都归属于上面已经列出的某个选项。`label` 是几个字的短名；`detail` 用一句话说清它得到什么、冒什么险；`action` 是玩家会发出去的那句话。`action` 必须写成玩家自己的动作。\n")
		sb.WriteString("- 当没有哪种说法比玩家手里已有的更好时，返回空的 `suggestions` 数组。空数组是真实答案，不是失败。\n")
	}
	sb.WriteString("- 不要添加字段，不要在对象外面再包一层，也不要在它之外写任何东西。\n")
	return sb.String()
}

// StoryAssistantUserMessage is the request itself.
//
// The situation and the question are both in the system instruction, where the
// boundary is stated, so the user turn only asks for the answer. Keeping the
// player's own words out of this message would be wrong; they are context, and
// context belongs with the rules that govern how it may be used.
const StoryAssistantUserMessage = "Answer the question above."
