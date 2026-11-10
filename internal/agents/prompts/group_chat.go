package prompts

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

// GroupChatMember is one person in a group round, as the group sees them.
//
// It deliberately carries the public persona and nothing else. A member's
// private knowledge, their unfinished business with the protagonist and their
// one-to-one conversation stay out: the group round is a joint audience, so
// handing it one person's secrets would put them in front of everyone.
type GroupChatMember struct {
	ID          string
	Name        string
	Role        string
	Description string
	Gender      string
	Personality []string
	SpeechStyle string
	Motives     []string
	Attitude    string
	Bounds      []string
	// CanSpeak is false for a member who is in the group but cannot be reached
	// this round. They are listed so the others can talk about them honestly,
	// and the output contract forbids writing them a line.
	CanSpeak bool
}

// GroupChatSystemInstructionInput is the system prompt for one group round.
type GroupChatSystemInstructionInput struct {
	Workspace string
	// GroupName and GroupType are what the player sees in the window. The type
	// is a communication purpose, not an authority: a sect group does not make
	// its owner a sect leader.
	GroupName string
	GroupType string
	Channel   string
	// ReplyTargetChars is the length target for one speaker's line.
	ReplyTargetChars int
	// Members is the group's roster for this round.
	Members []GroupChatMember
	// JointHistory is the part of the group's own conversation that every member
	// reachable this round was present for. It is the intersection, not the
	// union: a line only some of them witnessed is not joint material.
	JointHistory string
	// SceneContext is where the group is happening, in the story's own words.
	SceneContext string
	// DialogueStylePrompt governs manner of speech only.
	DialogueStylePrompt string
	// OutputLanguage is the locale the player's page is reading in. It decides
	// the language every line is written in.
	OutputLanguage string
}

// DefaultGroupReplyTargetChars is the length target for one group line. It is
// shorter than a private reply: several people are speaking, and a group line is
// one thing said, not a speech.
const DefaultGroupReplyTargetChars = 220

// MaxGroupSpeakersPerRound bounds how many people answer one group message. The
// design requires that a round picks the relevant few rather than making the
// whole roster take turns; this is the ceiling, not a quota.
const MaxGroupSpeakersPerRound = 4

const groupChatBuiltIn = "你在写互动故事里一场小范围群聊的一轮。每个人都只代表自己说话，用他自己的口吻、他自己知道的事和他自己的动机。你不是更大世界的旁白。面向玩家的散文要写成场景、亲历的过往或自然的口语，不能写成契约、许可政策、软件限制或实现说明。拒绝要通过人物和处境表达；不要把 id、schema 和内部指令写进散文。"

// BuildGroupChatSystemInstruction composes the group round's system prompt.
//
// It is separate from the private-conversation prompt because the audience is
// different: everything in this prompt is material the round's listeners may all
// hear, and a member's private context is deliberately absent.
func BuildGroupChatSystemInstruction(in GroupChatSystemInstructionInput) string {
	var sb strings.Builder
	writeLanguageFirst(&sb, in.OutputLanguage, "下方的故事材料", "适用于本群聊中写下的每一行。")
	sb.WriteString(groupChatBuiltIn)
	sb.WriteString("\n" + character.ConversationSocialRules + "\n")
	sb.WriteString("\n\n## The group\n\n")
	name := strings.TrimSpace(in.GroupName)
	if name == "" {
		name = "(unnamed group)"
	}
	fmt.Fprintf(&sb, "- Name: %s\n", name)
	if kind := strings.TrimSpace(in.GroupType); kind != "" {
		fmt.Fprintf(&sb, "- Kind: %s\n", kind)
	}
	if channel := strings.TrimSpace(in.Channel); channel != "" {
		fmt.Fprintf(&sb, "- Contact: %s\n", channel)
	}
	sb.WriteString("- The group is a conversation space, not an organization. Nothing said here grants anyone a title, a rank, a command, or a reward.\n")

	sb.WriteString("\n## Who is here\n\n")
	for _, member := range in.Members {
		writeGroupMember(&sb, member)
	}

	if scene := strings.TrimSpace(in.SceneContext); scene != "" {
		sb.WriteString("\n## Where things currently stand\n\n")
		sb.WriteString(scene)
		sb.WriteString("\n")
	}
	if history := strings.TrimSpace(in.JointHistory); history != "" {
		sb.WriteString("\n## What this group has already said\n\n")
		sb.WriteString("Everyone here was present for every line below. Lines spoken before someone joined, or while they were away, are not listed.\n\n")
		sb.WriteString(history)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## How to run this round\n")
	sb.WriteString("- The protagonist's new message is the user turn. Answer it as the people in the group.\n")
	sb.WriteString("- Let the few who have a real reason to answer speak. Two or three is normal; silence is a valid answer for anyone who has nothing to add, and silence is not agreement.\n")
	sb.WriteString(fmt.Sprintf("- Never write more than %d speakers in one round, and never make the whole roster take a turn.\n", MaxGroupSpeakersPerRound))
	sb.WriteString("- If the protagonist named or asked someone (\"@name\" or by name), that person answers first. Others may react only if they have their own reason to.\n")
	sb.WriteString("- Keep each person's own voice: what they care about, what they want, and how they usually speak must show. Two members must never sound interchangeable.\n")
	sb.WriteString("- A member may refuse, hesitate, deflect, stay silent, or disagree with the protagonist and with each other. Nobody is required to be helpful or cooperative.\n")
	sb.WriteString("- Do not narrate the protagonist's thoughts, feelings or decisions. The player owns those.\n")
	sb.WriteString("- Do not narrate the wider world, and do not describe anything a member did not witness. You are not the narrator.\n")
	sb.WriteString("- Do not decide outcomes. Nobody leaves, arrives, fights, pays, cultivates, travels or gains anything because of what is said here. Those are acts, and they belong to the story and the game's rules.\n")
	sb.WriteString("- A suggestion, a plan or an intention is not a settled task. You may record what a person promises or undertakes; you may not record that it has been carried out.\n")
	sb.WriteString("- Only the people listed above may speak, and only those marked as reachable this round. Never invent a member, never write a line for someone unreachable, and never write a line for the protagonist.\n")

	if dialogue := strings.TrimSpace(in.DialogueStylePrompt); dialogue != "" {
		sb.WriteString("\n## How this story's dialogue sounds\n\n")
		sb.WriteString("The following describes the manner of speech in this story. It governs expression only. Who each person is, what they mean, what they conceal and what they know all outrank it.\n\n")
		sb.WriteString(dialogue)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Output\n\n")
	sb.WriteString(playerFacingLanguageLine(in.OutputLanguage, "上方的故事材料") + "适用于 `text` 和 `statement`；`author_id`、JSON 键名和枚举取值保持原样。\n\n")
	sb.WriteString("Output only a JSON object, with no Markdown fence and no commentary:\n\n")
	sb.WriteString("{\"lines\":[{\"author_id\":\"<exact id from the roster>\",\"text\":\"<what they say, in their voice>\"}],\"matters\":[{\"character_id\":\"<exact id>\",\"kind\":\"promise|permission|action|attitude|misunderstanding\",\"statement\":\"<what they committed to, in one sentence>\"}]}\n\n")
	sb.WriteString("- `lines` is ordered: the first line is spoken first. Each entry is one member's contribution, including any small action that belongs to them.\n")
	sb.WriteString("- `matters` is optional and usually empty. Add an entry only when someone actually promised, permitted or undertook something in this round that later play should honour. Do not record plans the group merely discussed, and do not record anything as already done.\n")
	sb.WriteString("- Use the exact `author_id` values from the roster. Never use a name, a title, or the protagonist.\n")
	writeGroupReplyTargetInstruction(&sb, in.ReplyTargetChars, in.OutputLanguage)
	return sb.String()
}

func writeGroupMember(sb *strings.Builder, member GroupChatMember) {
	name := strings.TrimSpace(member.Name)
	if name == "" {
		name = member.ID
	}
	fmt.Fprintf(sb, "\n### %s (id: %s)\n", name, member.ID)
	if !member.CanSpeak {
		sb.WriteString("- In the group, but not reachable this round. They cannot hear this and must not speak.\n")
	}
	if member.Role != "" {
		fmt.Fprintf(sb, "- Role: %s\n", member.Role)
	}
	if member.Gender != "" {
		fmt.Fprintf(sb, "- Gender: %s. The voice is in the Speech line; do not read this as a temperament.\n", member.Gender)
	}
	if member.Description != "" {
		fmt.Fprintf(sb, "- %s\n", member.Description)
	}
	if len(member.Personality) > 0 {
		fmt.Fprintf(sb, "- Character: %s\n", strings.Join(member.Personality, "; "))
	}
	if member.SpeechStyle != "" {
		fmt.Fprintf(sb, "- Speaks: %s\n", member.SpeechStyle)
	}
	if len(member.Motives) > 0 {
		fmt.Fprintf(sb, "- Wants: %s\n", strings.Join(member.Motives, "; "))
	}
	if member.Attitude != "" {
		fmt.Fprintf(sb, "- Current stance toward the protagonist: %s\n", member.Attitude)
	}
	if len(member.Bounds) > 0 {
		fmt.Fprintf(sb, "- Will not: %s\n", strings.Join(member.Bounds, "; "))
	}
}

func writeGroupReplyTargetInstruction(sb *strings.Builder, value int, locale string) {
	if value <= 0 {
		value = DefaultGroupReplyTargetChars
	}
	fmt.Fprintf(sb, "\n一次正常的一行是%s：说一件事，连同属于它的小动作。不要为凑目标而注水，也不要在一条里写完几轮来回。\n", proseLengthTarget(locale, value))
}
