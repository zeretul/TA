package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// TurnCharacterFactsInput is the model-facing character-side module of one
// story turn. It carries what the turn's prose established about the people in
// it: who appeared for the first time, what was said, what was learned, and what
// was promised, granted or left unresolved.
//
// It is deliberately a draft shape. The model never supplies record IDs, world
// time, commit position or audience: the store assigns identity, resolves who
// could perceive each line, and stamps the branch's own clock. A model that
// supplied those itself would be able to invent a witness list and backdate a
// memory.
//
// The module is optional. A turn whose prose introduced nobody new and settled
// nothing omits it entirely rather than submitting empty arrays.
type TurnCharacterFactsInput struct {
	Memberships []TurnFactionMembershipInput `json:"memberships,omitempty" jsonschema_description:"主角本轮已经知情的势力申请、正式/临时接纳或退出。只沿本局冻结的接纳资格与本人实际回应，不凭称呼、好感、合作或所属势力文本授予身份。"`
	// Profiles registers a character on first formal appearance. A character
	// must exist before they can speak or learn, so a first appearance arrives
	// here rather than being rejected as an unknown name.
	Profiles []TurnCharacterProfileInput `json:"profiles,omitempty" jsonschema_description:"Characters who appear formally for the first time this turn, or whose established identity changed. On a first appearance, submit lifeform as human, animal or spirit_beast when clear; ordinary animals do not cultivate. A spirit_beast may use only an exact cultivation_method_id listed among this story's eligible methods. Keep both fields unchanged after registration, and never borrow a preset source_key for an invented creature. This includes a named or continuing nonhuman companion: retain one identity for its observed condition, learned cues, cooperation and departure using the existing facts and matters; human speech and an affinity score are not required. Omit for characters the Established Characters block already lists unless their attitude, role or circumstances changed."`
	// Exchanges are lines the prose actually gave a character.
	Exchanges []TurnCharacterExchangeInput `json:"exchanges,omitempty" jsonschema_description:"Lines this turn's prose actually gave a named character, in order. Record only speech the prose contains, not speech you plan to write later."`
	// Knowledge is what a character came to know this turn, with how they came to
	// know it.
	Knowledge []TurnCharacterKnowledgeInput `json:"knowledge,omitempty" jsonschema_description:"What a character learned this turn and how. Record a fact only for the characters who could actually perceive it; a line spoken on a private channel or in an incoming message did not reach anyone outside its audience."`
	// Matters are unfinished threads: permissions, promises, disclosures,
	// misunderstandings and durable attitude changes.
	Matters []TurnCharacterMatterInput `json:"matters,omitempty" jsonschema_description:"Unfinished business this turn created or settled: a permission actually granted, a promise made, information disclosed, a misunderstanding formed, or a durable change of stance. A permission is not the act it permits; a plan is not a completed trade."`
	// Messages are things a character sent the protagonist on their own
	// initiative, rather than lines spoken in the scene.
	Messages []TurnCharacterMessageInput `json:"messages,omitempty" jsonschema_description:"Incoming messages a character sent the protagonist on their own initiative this turn, when the turn's own events, an existing appointment, or their situation gave them a reason to write. Omit unless the prose actually shows the message arriving. Do not send one every turn and do not have every character write."`
	// Affinities are how the turn's real events changed a character's feeling
	// toward the protagonist. This is not a per-line score: one complete
	// interaction, settled once.
	Affinities []TurnCharacterAffinityInput `json:"affinities,omitempty" jsonschema_description:"How this turn's real events changed a character's feeling toward the protagonist. Omit unless something actually happened that they perceived and that bears on what they value. One entry per complete interaction, never one per line of dialogue. Ordinary courtesy, restated thanks and recalling an event already counted are worth nothing: submit nothing, or submit kind=none. A repeated gift or the same praise is not a new fact and must not be submitted again."`
	// Relationships are formal changes: a friendship declared, a bond sworn, a
	// parting. They need the words that offered it and the answer that took it.
	Relationships []TurnCharacterRelationshipInput `json:"relationships,omitempty" jsonschema_description:"本回合实际成立或解除的主角与此人物的正式关系。建立关系必须引用本回合先提出、后明确接受的两条不同话语，分别由主角与本人说出；双方可以任一方先提出。scene话语必须用addressed_to明确指向对方：NPC指向protagonist，主角指向该人物精确ID。不能引用对第三者或对全场说的话；私聊已有明确的双方。高好感、合理请求、沉默、拒绝或附条件回应都不等于接受。不替玩家发明答应；关系没有真正成立就不提交。"`
}

// TurnCharacterAffinityInput is one proposed change of feeling.
//
// The model proposes a scale and a size within that scale, plus the one fact
// the change is for. The backend computes the value, checks the size against
// the scale, and drops a fact it has already settled. A model that invents a
// number the scale does not allow is corrected rather than believed.
type TurnCharacterAffinityInput struct {
	CharacterID string `json:"character_id" jsonschema_description:"The character whose feeling changed. Must match an established or newly submitted id."`
	Kind        string `json:"kind" jsonschema:"enum=none,enum=pleasant,enum=meaningful,enum=major,enum=offense,enum=betrayal" jsonschema_description:"The scale of this one complete interaction. none: ordinary courtesy or evidence too thin to judge, which changes nothing. pleasant: an ordinary good conversation or a fitting small help. meaningful: real help, a kept promise, or an act that suits what this person actually values. major: a hardship faced together, or an important promise honoured. offense: a slight, a broken word, or a clear touch on what they dislike. betrayal: deception, serious bad faith, or damage to something they hold important."`
	Delta       int    `json:"delta" jsonschema_description:"How far this moves the feeling, within the scale's own range. pleasant 1 to 3; meaningful 4 to 8; major 9 to 15; offense -8 to -2; betrayal -30 to -10; none 0. A size outside its scale is rejected, so use the scale that matches what happened rather than stretching one."`
	Basis       string `json:"basis" jsonschema_description:"The one real fact this change is for, in a sentence, from the world's point of view. It is what makes the change settle once: the same fact proposed again does not score again. State what happened, not what the protagonist hoped it meant."`
	BasisRef    string `json:"basis_ref,omitempty" jsonschema_description:"Optional: the id of this turn's own exchange that the fact already exists as. Use it only when one line is the whole fact; otherwise describe the fact in basis."`
}

// TurnCharacterRelationshipInput is one proposed formal relationship change.
//
// The relationship is established by what was said, so the model cites the
// exchange positions rather than describing the feeling. Positions are 1-based
// indexes into this same call's exchanges array.
type TurnCharacterRelationshipInput struct {
	CharacterID      string `json:"character_id" jsonschema_description:"与主角建立或解除关系的人物，使用已登记或同回合新登记的精确ID。"`
	Kind             string `json:"kind" jsonschema:"enum=friend,enum=close_friend,enum=sworn,enum=romantic,enum=partner,enum=estranged,enum=hostile" jsonschema_description:"friend：双方正式结为朋友；close_friend：亲近信任的朋友；sworn：结义同伴；romantic：双方明确承认恋情；partner：正式道侣；estranged：关系解除；hostile：公开为敌。恋爱和道侣不是好感数值自动升级的结果。"`
	Statement        string `json:"statement" jsonschema_description:"从故事内视角用一句话记录实际变化，不把愿望写成结果。"`
	ExpressedByIndex int    `json:"expressed_by_index" jsonschema_description:"本次exchanges中提出、声明或解除关系的话语位置，从1起。scene话语须明确addressed_to为对方；不能引用旁人、转述或说给全场的话。"`
	AnsweredByIndex  int    `json:"answered_by_index" jsonschema_description:"本次exchanges中另一方明确接受的话语位置，从1起，须大于expressed_by_index且speaker不同。friend、close_friend、sworn、romantic、partner必须有真实接受；estranged、hostile可单方解除或公开为敌，没有回答时写0。"`
}

// TurnCharacterMessageInput is one incoming message a character sent on their own
// initiative. The model supplies the words and the reason; the backend supplies
// the message's identity, its original send time and the commit position it is
// caused by, so a replayed turn delivers one message rather than two.
type TurnCharacterMessageInput struct {
	CharacterID string `json:"character_id" jsonschema_description:"The character who wrote. Must match an id in the Established Characters block or a profile submitted in this same call."`
	Text        string `json:"text" jsonschema_description:"What they actually wrote, in their own voice. Keep it in character: a short note, not a scene."`
	Basis       string `json:"basis" jsonschema_description:"Why they wrote now: the committed event, appointment, unfinished matter or circumstance that caused this. A message with no reason is a character contacting the protagonist out of nowhere."`
	Deadline    string `json:"deadline,omitempty" jsonschema_description:"The in-fiction time this lapses, when the message names one, copied from the story's own scene. Omit when it is open-ended."`
}

// TurnCharacterProfileInput is the minimal identity a first appearance needs.
// Everything a reader would need to hear this person as themselves rather than
// as a second copy of someone already in the story.
type TurnCharacterProfileInput struct {
	RadioContact        *character.RadioContact `json:"radio_contact,omitempty" jsonschema_description:"仅在支持无线电的世界、主角确有通讯设备并实际交换联络方式或通道状态变化时记录。不能靠人物目录、背景关系或公开资料假定可通话；失联写offline，重新接通写connected。"`
	CharacterID         string                  `json:"character_id" jsonschema_description:"Stable identity for this character. Use the exact id from the Established Characters block when they are already listed; otherwise mint a short stable id from their name in the story's language."`
	Name                string                  `json:"name" jsonschema_description:"The character's own name, in the story's language."`
	Age                 int                     `json:"age,omitempty" jsonschema_description:"已明确的人物实际年龄。原生人物按本局冻结目录；AI新建人物可在正式初次出场明确年龄并保存，未知则省略。不得按外貌或职业推断，不得为邀请把未成年人改成年；已保存年龄按正式时间承接。"`
	SourceKey           string                  `json:"source_key,omitempty" jsonschema_description:"Only when this person came from the Discoverable People index: the source_key printed for them there, copied exactly. This binds the author's preset character to this story's actor so they are never cast twice. Omit it entirely for characters you invented — an invented person must not borrow a preset key."`
	KnownAs             string                  `json:"known_as,omitempty" jsonschema_description:"The name the protagonist would actually use, when it differs from their own name. Omit when the protagonist does not know them yet. A visual observation requires this actually recognized name even when it equals name."`
	Aliases             []string                `json:"aliases,omitempty" jsonschema_description:"Other names the protagonist has actually learned for them. Do not list names nobody has spoken."`
	Role                string                  `json:"role,omitempty" jsonschema_description:"Their part in the story, such as gate guard, innkeeper, or caravan master."`
	Description         string                  `json:"description,omitempty" jsonschema_description:"One or two sentences of concrete description."`
	Personality         []string                `json:"personality,omitempty" jsonschema_description:"Traits that change how they behave, not adjectives that could describe anyone."`
	SpeechStyle         string                  `json:"speech_style,omitempty" jsonschema_description:"How they actually talk, in two or three concrete features plus one short example line: sentence length, word choice, what they call the protagonist, whether they joke and how. Write what makes this person recognisable against the others in the scene — a slow shopkeeper who asks the use before quoting a price is not the same voice as a gate guard who answers in four words and relaxes only with people he knows. Do not give everyone the same dry wit, and do not reduce a person to one catchphrase. Gender is not a voice: a woman and a man may both be terse, and neither must be written soft."`
	Gender              string                  `json:"gender,omitempty" jsonschema:"enum=male,enum=female" jsonschema_description:"This person's gender, decided once and kept for the rest of the story. Use male or female. Fill it when you are introducing someone new and their gender is genuinely settled — by the prose, by the author's entry, or by your own decision as the author of this person. Leave it empty when the story has not settled it and nothing forces a choice; an empty value is honest and is not an error. Never infer it from a name, a picture, a role or an occupation, and never change it later without a real in-story reason."`
	Lifeform            string                  `json:"lifeform,omitempty" jsonschema:"enum=human,enum=animal,enum=spirit_beast" jsonschema_description:"The identity's established lifeform: human, ordinary animal, or spirit_beast. State it on first formal appearance when clear; do not infer cultivation from a name, appearance, care, affinity or cooperation. For an author-catalog person, the frozen catalog is authoritative."`
	CultivationMethodID string                  `json:"cultivation_method_id,omitempty" jsonschema_description:"Only for a spirit_beast whose frozen world catalog explicitly permits a registered method. Use the exact method id and only when the world's eligible methods block lists it. Never submit a preset source_key for an invented creature. This identity and method are fixed after first registration."`
	Motives             []string                `json:"motives,omitempty" jsonschema_description:"What they want in this situation, which is what makes them disagree with each other."`
	Attitude            string                  `json:"attitude,omitempty" jsonschema_description:"Their current stance toward the protagonist after this turn."`
	Bounds              []string                `json:"bounds,omitempty" jsonschema_description:"What they will not do: refusals, topics they avoid, and the conditions under which they end a conversation."`
	Tags                []string                `json:"tags,omitempty" jsonschema:"maxItems=6" jsonschema_description:"Short labels a reader would search this person by: the group or household they belong to, what they do for a living, and where they are from. Two to four words each, in the story's language. Write only what this appearance actually established — do not tag a craft nobody has named, and do not tag a region they merely passed through. Leave it empty for someone the scene only sketched; an empty list is honest, and these labels are never inferred from a name."`
}

type TurnCharacterExchangeInput struct {
	ParticipationDecision *character.ParticipationDecision `json:"participation_decision,omitempty" jsonschema_description:"本人对这次助战或这条行程的明确接受/拒绝。kind=combat时target_id为本场敌方Actor，kind=journey时为本次路线ID，kind=faction时为本件共同制作project_id（每次工作独立回应）；decision=accept或refuse。须与本人实际scene回应一致，可用符合物种灵智的非语言行为，不把主角命令或普通承诺写成同意。"`
	CooperationDecision   *character.CooperationDecision   `json:"cooperation_decision,omitempty" jsonschema_description:"仅本人在现场或私下对明确的局部协作作出的接受/拒绝候选。必须匹配已知项目与实例；不是通用Promise，不可指定费用、能力或收益。"`
	CharacterID           string                           `json:"character_id" jsonschema_description:"The character this line belongs to. Must match an id in the Established Characters block or a profile submitted in this same call."`
	Speaker               string                           `json:"speaker" jsonschema:"enum=character,enum=player" jsonschema_description:"character when the character spoke; player when the protagonist spoke to them."`
	Channel               string                           `json:"channel" jsonschema:"enum=scene,enum=private,enum=message" jsonschema_description:"scene for speech inside the main narrative; private for a one-to-one conversation; message for something the character sent on their own initiative."`
	AddressedTo           string                           `json:"addressed_to,omitempty" jsonschema_description:"Optional: the one character this line was actually spoken to, when the prose directed it at someone in particular. Use the exact character id, or protagonist for the protagonist. Omit when the line was said to the room rather than aimed at a person. Do not put the speaker here: this says who the words were aimed at, not who said them."`
	Text                  string                           `json:"text" jsonschema_description:"The line itself, as the prose wrote it. Do not summarise it."`
}

type TurnCharacterKnowledgeInput struct {
	Visual      *character.VisualObservationDraft `json:"visual,omitempty" jsonschema_description:"仅记录本回合主角实际看见并已识别的公开形态，不是显示图片的许可。必须有本回合同场接触和身份识别话语来源，并先在该人物状态外貌与装扮中保存实际外观；缺证据、看不清、伪装未识别、私聊或仅听说时省略。不得为了配图创造见面、地点、外貌或知情。"`
	CharacterID string                            `json:"character_id" jsonschema_description:"The character who now knows this. Must match an established or newly submitted id. Main-turn visual observations use protagonist as the holder."`
	Statement   string                            `json:"statement" jsonschema_description:"本人实际获知的事实。NPC间关系注明双方姓名/稳定ID、夫妻/道侣/父子/师徒/朋友等类型与方向、当前状态和依据；只写实际知情，变化用supersedes/retracts更正旧记录，不把NPC间关系误记为与主角的relationships。"`
	Nature      string                            `json:"nature" jsonschema:"enum=experienced,enum=told,enum=hearsay,enum=conjecture,enum=promise,enum=secret" jsonschema_description:"How they came to know it. experienced: they personally perceived it. told: a specific person told them. hearsay: second-hand or unattributed. conjecture: their own inference, which may be wrong. promise: an undertaking they made or received. secret: their own private knowledge, which they may lie about."`
	Audience    string                            `json:"audience,omitempty" jsonschema:"enum=world,enum=participants,enum=single" jsonschema_description:"Who else could learn this from the same event. world: the event was public within its scene. participants: only the people present. single: only this character. Omit to let the backend resolve it from the channel."`
	Channel     string                            `json:"channel,omitempty" jsonschema:"enum=scene,enum=private,enum=message" jsonschema_description:"Where they came to know it, used when audience is omitted: scene for something that happened in the narrative, private for a one-to-one conversation, message for an incoming message."`
	LearnedFrom string                            `json:"learned_from,omitempty" jsonschema_description:"Required for nature=told: the character or person who told them."`
	Holders     []string                          `json:"holders,omitempty" jsonschema_description:"For audience=participants, the characters entitled to this record. The subject is always included."`
	Supersedes  string                            `json:"supersedes,omitempty" jsonschema_description:"The id of an earlier knowledge record this one replaces, when the character's belief changed. Use the id shown for that record in the prepared context."`
	Retracts    string                            `json:"retracts,omitempty" jsonschema_description:"The id of an earlier knowledge record this one disproves. Use the id shown for that record in the prepared context."`
}

type TurnCharacterMatterInput struct {
	AfterSeconds        int64                  `json:"after_seconds,omitempty" jsonschema_description:"仅本人本轮明确约定的最早复查/回应间隔，单位为游戏秒；用于开放promise/action，需source_exchange_index指向本轮本人原话。不是截止时限，不表示届时必然完成，也不自动发任务。未约定则省略。"`
	SourceExchangeIndex int                    `json:"source_exchange_index,omitempty" jsonschema_description:"本轮exchanges中本人实际说出这项延迟约定、或到时确认完成的1起算序号。设置after_seconds或settled解决已有延迟事务时必填；放弃可不填。"`
	CharacterID         string                 `json:"character_id" jsonschema_description:"The character this thread is with. Must match an established or newly submitted id."`
	Kind                string                 `json:"kind" jsonschema:"enum=permission,enum=action,enum=promise,enum=disclosure,enum=misunderstanding,enum=attitude" jsonschema_description:"permission: a right actually granted. action: an act someone committed to carry out. promise: an undertaking by either side. disclosure: information handed over. misunderstanding: a false belief one side holds. attitude: a durable change of stance."`
	Status              string                 `json:"status" jsonschema:"enum=open,enum=settled,enum=abandoned" jsonschema_description:"open while it still constrains behaviour; settled when it was actually carried out; abandoned when it lapsed, expired or was refused."`
	Statement           string                 `json:"statement" jsonschema_description:"The thread in one sentence, from the world's point of view. A permission must not be phrased as the act it permits."`
	Counterparty        string                 `json:"counterparty,omitempty" jsonschema_description:"The other party: protagonist, or another character's id."`
	Deadline            string                 `json:"deadline,omitempty" jsonschema_description:"The in-fiction time this lapses, copied from the story's own scene when the prose named one. Omit when it is open-ended."`
	Resolves            string                 `json:"resolves,omitempty" jsonschema_description:"The id of an earlier open matter this one settles, copied exactly from the id shown for that matter in the prepared context. Leave empty unless the prose actually discharged it. This is an id, never a description of what happened."`
	LetterReply         *character.LetterReply `json:"letter_reply,omitempty" jsonschema_description:"Only when the recipient actually wrote and sent a reply to a letter that has reached its arrival threshold. Include the exact words of the written note. Omit for receipt alone, an oral answer, or a reply that has not arrived. This must settle the original letter for its recipient."`
}

// Profiles converts the submitted first appearances into identity records. The
// backend fills the fields a model must not choose: the story, the world time
// and scene they were first seen in, the revision, and the timestamps.
func (input TurnCharacterFactsInput) ProfileRecords(storyID, worldTime, sceneNode string) []character.Profile {
	if len(input.Profiles) == 0 {
		return nil
	}
	now := strings.TrimSpace(worldTime)
	out := make([]character.Profile, 0, len(input.Profiles))
	for _, draft := range input.Profiles {
		out = append(out, character.Profile{
			ID: strings.TrimSpace(draft.CharacterID), StoryID: strings.TrimSpace(storyID),
			Name: strings.TrimSpace(draft.Name), KnownAs: strings.TrimSpace(draft.KnownAs),
			// The source key is carried through verbatim. It is what the next turn
			// matches on to find this same actor again, and what keeps a preset
			// person from being re-created under a second id.
			SourceKey: strings.TrimSpace(draft.SourceKey),
			Aliases:   append([]string(nil), draft.Aliases...), Role: strings.TrimSpace(draft.Role),
			Description: strings.TrimSpace(draft.Description),
			Personality: append([]string(nil), draft.Personality...),
			SpeechStyle: strings.TrimSpace(draft.SpeechStyle), Motives: append([]string(nil), draft.Motives...),
			Gender:   character.NormalizeGender(draft.Gender),
			Lifeform: strings.TrimSpace(draft.Lifeform), CultivationMethodID: strings.TrimSpace(draft.CultivationMethodID),
			Age:      draft.Age,
			Attitude: strings.TrimSpace(draft.Attitude), Bounds: append([]string(nil), draft.Bounds...),
			Tags:               append([]string(nil), draft.Tags...),
			RadioContact:       draft.RadioContact,
			FirstSeenWorldTime: now, FirstSeenSceneNode: strings.TrimSpace(sceneNode),
			// A submitted profile is a new revision of whoever holds that id, so an
			// edited character is never served from a context built for the old one.
			Revision: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	return out
}

// Facts converts the submitted drafts into the shape the projector consumes.
func (input TurnCharacterFactsInput) Facts() character.TurnCharacterFacts {
	facts := character.TurnCharacterFacts{}
	for _, draft := range input.Exchanges {
		facts.Exchanges = append(facts.Exchanges, character.ExchangeDraft{
			CharacterID:           strings.TrimSpace(draft.CharacterID),
			Speaker:               character.Speaker(strings.TrimSpace(draft.Speaker)),
			Channel:               character.Channel(strings.TrimSpace(draft.Channel)),
			AddressedTo:           strings.TrimSpace(draft.AddressedTo),
			Text:                  strings.TrimSpace(draft.Text),
			CooperationDecision:   draft.CooperationDecision,
			ParticipationDecision: draft.ParticipationDecision,
		})
	}
	for _, draft := range input.Knowledge {
		facts.Knowledge = append(facts.Knowledge, character.KnowledgeDraft{
			Visual:      draft.Visual,
			CharacterID: strings.TrimSpace(draft.CharacterID),
			Statement:   strings.TrimSpace(draft.Statement),
			Nature:      character.KnowledgeNature(strings.TrimSpace(draft.Nature)),
			Audience:    character.KnowledgeAudience(strings.TrimSpace(draft.Audience)),
			Channel:     character.Channel(strings.TrimSpace(draft.Channel)),
			LearnedFrom: strings.TrimSpace(draft.LearnedFrom),
			Holders:     append([]string(nil), draft.Holders...),
			Supersedes:  strings.TrimSpace(draft.Supersedes),
			Retracts:    strings.TrimSpace(draft.Retracts),
		})
	}
	for _, draft := range input.Matters {
		facts.Matters = append(facts.Matters, character.MatterDraft{
			AfterSeconds: draft.AfterSeconds, SourceExchangeIndex: draft.SourceExchangeIndex,
			CharacterID:  strings.TrimSpace(draft.CharacterID),
			Kind:         character.MatterKind(strings.TrimSpace(draft.Kind)),
			Status:       character.MatterStatus(strings.TrimSpace(draft.Status)),
			Statement:    strings.TrimSpace(draft.Statement),
			Counterparty: strings.TrimSpace(draft.Counterparty),
			Deadline:     strings.TrimSpace(draft.Deadline),
			Resolves:     strings.TrimSpace(draft.Resolves),
			LetterReply:  draft.LetterReply,
		})
	}
	for _, draft := range input.Messages {
		facts.Messages = append(facts.Messages, character.MessageDraft{
			CharacterID: strings.TrimSpace(draft.CharacterID),
			Text:        strings.TrimSpace(draft.Text),
			Basis:       strings.TrimSpace(draft.Basis),
			Deadline:    strings.TrimSpace(draft.Deadline),
		})
	}
	for _, draft := range input.Affinities {
		facts.Affinities = append(facts.Affinities, character.AffinityDraft{
			CharacterID: strings.TrimSpace(draft.CharacterID),
			Kind:        character.AffinityKind(strings.TrimSpace(draft.Kind)),
			Delta:       draft.Delta,
			Basis:       strings.TrimSpace(draft.Basis),
			BasisRef:    strings.TrimSpace(draft.BasisRef),
		})
	}
	for _, draft := range input.Relationships {
		facts.Relationships = append(facts.Relationships, character.RelationshipDraft{
			CharacterID:      strings.TrimSpace(draft.CharacterID),
			Kind:             character.RelationshipKind(strings.TrimSpace(draft.Kind)),
			Statement:        strings.TrimSpace(draft.Statement),
			ExpressedByIndex: draft.ExpressedByIndex,
			AnsweredByIndex:  draft.AnsweredByIndex,
		})
	}
	return facts
}

// Empty reports whether the module carried nothing. An empty module is accepted
// and commits no record, so a turn that introduced nobody does not fail.
func (input TurnCharacterFactsInput) Empty() bool {
	return len(input.Memberships) == 0 && len(input.Profiles) == 0 && len(input.Exchanges) == 0 && len(input.Knowledge) == 0 && len(input.Matters) == 0 && len(input.Messages) == 0 &&
		len(input.Affinities) == 0 && len(input.Relationships) == 0
}

// validateTurnCharacterFactsModule checks the submitted drafts before the prose
// is published, so a turn that names a character nobody has met is repaired
// while it is still a draft rather than after the player has read it.
//
// The check runs the real projector against sentinel identity. That is
// deliberate: submission-time validation and commit-time projection cannot
// disagree about what is acceptable, because they are the same code.
func validateTurnCharacterFactsModule(input *TurnCharacterFactsInput, known []string, scene character.Scene, existingAffinities []character.AffinityChange, existingRelationships []character.Relationship, existingMatters []character.Matter, clockSeconds int64) *TurnSubmissionDiagnostic {
	if input == nil || input.Empty() {
		return nil
	}
	// A person the story introduces is introduced in words, and the card the
	// player opens later shows exactly this field. An empty one is not a small
	// omission: it is a person the window can only name. The check runs before
	// the prose is published, so the model still has the chance to say who this
	// is rather than the player meeting a blank afterwards.
	if diagnostic := validateIntroducedIntroductions(input.Profiles, known); diagnostic != nil {
		return diagnostic
	}
	if diagnostic := validateLetterMatterSettlements(input, existingMatters, clockSeconds); diagnostic != nil {
		return diagnostic
	}
	// A first appearance registers the character in the same call, so the lines
	// it speaks are validated against the roster the turn itself creates.
	// Main-turn knowledge includes the player observer, including later
	// superseding or retracting a visual record without creating another one.
	roster := append(append([]string(nil), known...), DefaultActorID)
	for _, profile := range input.Profiles {
		roster = append(roster, strings.TrimSpace(profile.CharacterID))
	}
	scene = scene.Normalize()
	if scene.NodeID == "" {
		scene.NodeID = turnSubmissionPendingIdentity
	}
	if _, err := character.Project(character.ProjectionContext{
		StoryID: turnSubmissionPendingIdentity, BranchID: turnSubmissionPendingIdentity,
		WorldTime: turnSubmissionPendingIdentity, CommitID: turnSubmissionPendingIdentity,
		Scene: scene, KnownCharacters: roster,
		ClockSeconds: clockSeconds, ExistingMatters: existingMatters,
		ExistingAffinities:    existingAffinities,
		ExistingRelationships: existingRelationships,
		NextID:                func(prefix string) string { return prefix + "_" + turnSubmissionPendingIdentity },
	}, input.Facts()); err != nil {
		return newTurnSubmissionDiagnostic(
			TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
			"/character_facts", "drafts naming established characters with valid speech, knowledge and matters",
			"rejected draft", fmt.Sprintf("The character facts were rejected: %v", err),
		)
	}
	return nil
}

func validateLetterMatterSettlements(input *TurnCharacterFactsInput, existing []character.Matter, clockSeconds int64) *TurnSubmissionDiagnostic {
	byID := make(map[string]character.Matter, len(existing))
	for _, matter := range existing {
		matter = matter.Normalize()
		byID[matter.ID] = matter
	}
	settled := map[string]bool{}
	for index, draft := range input.Matters {
		id := strings.TrimSpace(draft.Resolves)
		if draft.LetterReply != nil && id == "" {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/matters/%d/letter_reply", index), "a written reply settling its original arrived letter", "written reply without resolved letter",
				"A written letter reply must settle the original arrived letter for its recipient.")
		}
		if id == "" {
			continue
		}
		matter, exists := byID[id]
		if !exists {
			if draft.LetterReply != nil {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/matters/%d/resolves", index), "the original open letter id", id,
					"The written reply does not reference an open letter on this story branch.")
			}
			if strings.HasPrefix(id, "mat_") {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/matters/%d/resolves", index), "an open matter id shown in the current story context", id,
					"The referenced matter is not open on this story branch. Use the exact id shown in the current story context.")
			}
			continue
		}
		if matter.Letter == nil {
			if draft.LetterReply != nil {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/matters/%d/resolves", index), "the original letter matter", id,
					"A written letter reply can only resolve an original letter.")
			}
			continue
		}
		if settled[id] {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/matters/%d/resolves", index), "one settlement for the referenced letter", id,
				"This letter is already being settled by another matter in the same turn.")
		}
		settled[id] = true
		// Receipt can precede an actual written answer. A later reply links to
		// the already delivered letter without settling delivery a second time.
		deliveredAwaitingReply := matter.Status == character.MatterSettled && draft.LetterReply != nil
		if (matter.Status != character.MatterOpen && !deliveredAwaitingReply) || draft.CharacterID != matter.CharacterID || draft.Kind != string(character.MatterAction) || draft.Status != string(character.MatterSettled) {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/matters/%d", index), "a settled action with the original recipient after delivery", "mismatched letter facts",
				"A letter can only be settled for its original recipient, and only as a completed action.")
		}
		if draft.LetterReply != nil {
			for _, previous := range existing {
				if previous.SettledBy == id && previous.LetterReply != nil {
					return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
						fmt.Sprintf("/character_facts/matters/%d/letter_reply", index), "one written reply for the original letter", "letter already answered",
						"This letter already has a recorded written reply. Reuse its history instead of appending another answer.")
				}
			}
		}
		if clockSeconds < matter.Letter.EarliestDeliverySeconds {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/matters/%d/resolves", index), fmt.Sprintf("formal clock at least %d seconds", matter.Letter.EarliestDeliverySeconds), fmt.Sprintf("%d seconds", clockSeconds),
				"This letter has not had enough time to arrive. Keep it in transit until a later story turn reaches its delivery time.")
		}
	}
	return nil
}

func validateTurnCharacterQualifications(cultivationRules *cultivation.Rules, input *TurnCharacterFactsInput, existing []character.Profile, catalog *PackCatalogSnapshot) *TurnSubmissionDiagnostic {
	if input == nil {
		return nil
	}
	profilesByID := make(map[string]character.Profile, len(existing))
	for _, profile := range existing {
		profile = profile.Normalize()
		profilesByID[profile.ID] = profile
	}
	var catalogPeople []PackCatalogPerson
	if catalog != nil {
		catalogPeople = catalog.Characters
	}
	catalogBySource := make(map[string]PackCatalogPerson, len(catalogPeople))
	for _, person := range catalogPeople {
		person.SourceKey = strings.TrimSpace(person.SourceKey)
		if person.SourceKey != "" {
			catalogBySource[person.SourceKey] = person
		}
	}
	for index, draft := range input.Profiles {
		if person, blocked := recordOnlyCatalogPerson(catalog, draft.SourceKey, draft.Name); blocked {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/source_key", index), "本局当前可实际接触的人物", person.SourceKey,
				"该人物在本局冻结资料中仅供史料引用，没有成立的当期存续或复苏事实。可以叙述读到的记载，不能把本人登记成到场人物、通信发言人或战斗参与者。")
		}
		id := strings.TrimSpace(draft.CharacterID)
		lifeform := strings.ToLower(strings.TrimSpace(draft.Lifeform))
		methodID := strings.TrimSpace(draft.CultivationMethodID)
		if lifeform != "" && lifeform != character.LifeformHuman && lifeform != character.LifeformAnimal && lifeform != character.LifeformSpiritBeast {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/lifeform", index), "human, animal, or spirit_beast", lifeform,
				"Use one supported lifeform value on a character's first formal appearance.")
		}
		if established, ok := profilesByID[id]; ok {
			if lifeform != "" && lifeform != established.Lifeform {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/lifeform", index), established.Lifeform, lifeform,
					"An established character's lifeform is fixed at first formal registration.")
			}
			if methodID != "" && methodID != established.CultivationMethodID {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/cultivation_method_id", index), established.CultivationMethodID, methodID,
					"An established character's cultivation method is fixed at first formal registration.")
			}
			continue
		}
		if sourceKey := strings.TrimSpace(draft.SourceKey); sourceKey != "" && lifeform == character.LifeformSpiritBeast {
			person, ok := catalogBySource[sourceKey]
			if !ok {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/source_key", index), "a source_key from this story's frozen catalog", sourceKey,
					"An author-catalog source key must come from this story's frozen catalog. Invented characters must omit source_key.")
			}
			if !catalogPersonHasName(person, draft.Name) {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/name", index), person.Name, strings.TrimSpace(draft.Name),
					"A catalog identity must keep the catalog person's own name; use a new id without source_key for an invented person.")
			}
			catalogLifeform := strings.TrimSpace(person.Lifeform)
			if catalogLifeform == "" {
				catalogLifeform = character.LifeformHuman
			}
			if lifeform != "" && lifeform != catalogLifeform {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/lifeform", index), catalogLifeform, lifeform,
					"The frozen author catalog is authoritative for a catalog character's lifeform.")
			}
			if methodID != "" && methodID != strings.TrimSpace(person.CultivationMethodID) {
				return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
					fmt.Sprintf("/character_facts/profiles/%d/cultivation_method_id", index), strings.TrimSpace(person.CultivationMethodID), methodID,
					"The frozen author catalog is authoritative for a catalog character's cultivation method.")
			}
			continue
		}
		if lifeform == "" {
			lifeform = character.LifeformHuman
		}
		if lifeform != character.LifeformSpiritBeast && methodID != "" {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/cultivation_method_id", index), "empty unless lifeform is spirit_beast", methodID,
				"Ordinary animals and humans cannot be assigned a species cultivation method.")
		}
		if methodID == "" {
			continue
		}
		if _, ok := cultivation.TechniqueByID(methodID, cultivationRules); !ok || !packCatalogAllowsSpiritBeastMethod(catalog, methodID) {
			return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, &index, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/cultivation_method_id", index), "a registered method explicitly enabled by this frozen world", methodID,
				"This method is not available to newly registered spirit beasts in this story's frozen world.")
		}
	}
	return nil
}

func catalogPersonHasName(person PackCatalogPerson, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.TrimSpace(person.Name) == name {
		return name != ""
	}
	for _, alias := range person.Aliases {
		if strings.TrimSpace(alias) == name {
			return true
		}
	}
	return false
}

func packCatalogAllowsSpiritBeastMethod(catalog *PackCatalogSnapshot, methodID string) bool {
	if catalog == nil {
		return false
	}
	for _, person := range catalog.Characters {
		if strings.TrimSpace(person.Lifeform) == character.LifeformSpiritBeast && strings.TrimSpace(person.CultivationMethodID) == methodID {
			return true
		}
	}
	return false
}

// Letter worlds have no instantaneous remote character channel. Their
// asynchronous contact is recorded as an open Matter by the player's story
// turn; arrival and any answer are then established by a later main-story turn.
// Scene dialogue remains available when the people are actually together.
func validateLetterWorldCharacterChannels(input *TurnCharacterFactsInput, communicationMode string) *TurnSubmissionDiagnostic {
	if input == nil || strings.TrimSpace(communicationMode) != "letter" {
		return nil
	}
	for index, exchange := range input.Exchanges {
		if strings.TrimSpace(exchange.Channel) == string(character.ChannelScene) {
			continue
		}
		return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
			fmt.Sprintf("/character_facts/exchanges/%d/channel", index), "scene for people speaking in the same place", strings.TrimSpace(exchange.Channel),
			"This world's characters cannot answer through an instant remote conversation. Use a face-to-face scene, or let a committed letter reach them first.")
	}
	if len(input.Messages) > 0 {
		return newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule,
			"/character_facts/messages", "no immediate incoming message in a letter-only world", "incoming message",
			"This world's incoming replies must arrive through a later main-story turn, after the letter's recorded delivery time.")
	}
	return nil
}

// turnSubmissionPendingIdentity is the placeholder identity the submission-time
// character check projects against. It never reaches a record: the check throws
// the projection away and keeps only its verdict.
const turnSubmissionPendingIdentity = "pending"

// introducedIntroductionMaxRunes bounds how long a new person's introduction may
// be. The author's own pack entries run one to three sentences and the longest
// is 66 characters, so the cap sits at roughly twice the longest shipped
// introduction: long enough for a model writing in the same voice, short enough
// that scene prose cannot be pasted into the caption the person card shows.
const introducedIntroductionMaxRunes = 120

// validateIntroducedIntroductions rejects a first appearance that carries no
// introduction at all, and one too long to be a person card caption.
//
// It deliberately does not judge whether the words are good. An earlier version
// also refused an introduction whose remainder, after the name and role were
// removed, was shorter than six characters. That measured the length of the
// leftover rather than whether it said anything: "陈九，只卖旧书。" is a complete,
// specific introduction of four characters and was refused, while a longer
// sentence of filler passed. What a person's introduction should say is an
// authoring and model question, answered by the pack's own people and by the
// repair loop; the compiler's job is only that the field exists and fits.
//
// Only a person this turn actually introduces is checked. A later turn that
// mentions an established character without describing them is normal: the
// introduction is already stored, and UpsertProfile keeps it. Requiring it again
// would make every re-appearance retry.
func validateIntroducedIntroductions(profiles []TurnCharacterProfileInput, known []string) *TurnSubmissionDiagnostic {
	established := make(map[string]bool, len(known))
	for _, id := range known {
		established[strings.TrimSpace(id)] = true
	}
	for index, profile := range profiles {
		id := strings.TrimSpace(profile.CharacterID)
		if id == "" || established[id] {
			continue
		}
		at := index
		description := strings.TrimSpace(profile.Description)
		if description == "" {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCharacterFacts, &at, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/description", index),
				"one or two sentences saying who this person is, written from what the scene showed",
				"empty",
				fmt.Sprintf("The introduction for %q is empty. The player will be able to open this person's card, and an empty introduction leaves them with a name and nothing else. Say who they are, in one or two sentences taken from what this scene actually showed.", displayPersonName(profile)),
			)
		}
		if runes := []rune(description); len(runes) > introducedIntroductionMaxRunes {
			return newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCharacterFacts, &at, TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/character_facts/profiles/%d/description", index),
				fmt.Sprintf("at most %d characters", introducedIntroductionMaxRunes),
				fmt.Sprintf("%d characters", len(runes)),
				fmt.Sprintf("The introduction for %q is longer than a person card caption. Keep it to one or two sentences about who they are; the scene prose belongs in the narrative.", displayPersonName(profile)),
			)
		}
	}
	return nil
}

// displayPersonName is what a diagnostic calls a person the model just invented,
// which may have neither a name nor a usable id.
func displayPersonName(profile TurnCharacterProfileInput) string {
	if name := strings.TrimSpace(profile.Name); name != "" {
		return name
	}
	if id := strings.TrimSpace(profile.CharacterID); id != "" {
		return id
	}
	return "this person"
}

// buildTurnCharacterRecord projects a turn's submitted character facts into a
// side record that the caller appends in the same transaction as the Turn.
//
// It is a free function rather than a Store method on purpose. The commit path
// already holds the store lock, so it cannot call AppendCharacterFacts; and the
// record it builds must be parented after the Turn it belongs to, not after the
// branch head as a window conversation's record is. Everything the record needs
// is passed in, so this stays a pure transformation.
//
// It returns nil when the turn established nothing about anyone, which is the
// common case and commits no record at all.
//
// A turn can introduce a person through either module: character_facts names
// them, or state_changes stages an Actor for them. Both are formal appearances,
// so both must leave one entry in the story's character registry. An Actor that
// only state_changes created used to stay invisible to the character window, the
// narrator's cast block and the discovery exclusion list, and the next turn then
// minted a second person for the same face.
func buildTurnCharacterRecord(storyID, branchID string, turn TurnEvent, projection *storyBranchProjection, postTurnState map[string]any, calendar *StoryCalendar, playerLetter *PlayerLetterIntent, packBinding *StoryPackBinding) (*CharacterRecordEvent, error) {
	var input *TurnCharacterFactsInput
	if turn.TurnResult != nil && turn.TurnResult.CharacterFacts != nil && !turn.TurnResult.CharacterFacts.Empty() {
		input = turn.TurnResult.CharacterFacts
	}
	// The scene is the turn's own: it is where the prose happened, and the
	// audience of every line resolves against it. It is read from the state the
	// turn just produced, so a turn that moved the protagonist somewhere is
	// recorded as happening where it ended.
	scene := StorySceneFromState(postTurnState, turn.ID)
	worldTime := scene.WorldTime
	if worldTime == "" {
		worldTime = scene.NodeID
	}
	var profiles []character.Profile
	if input != nil {
		profiles = input.ProfileRecords(storyID, worldTime, turn.ID)
	}

	state := character.State{}
	if projection != nil && projection.Characters != nil {
		state = projection.Characters.Normalize()
	}
	membershipState := state.Normalize()
	var catalog *PackCatalogSnapshot
	if projection != nil {
		catalog = projection.PackCatalog
	}
	if diagnostic := validateTurnCharacterQualifications(projection.cultivationRules(),
		input, state.Profiles, catalog); diagnostic != nil {
		return nil, fmt.Errorf("character profile qualification was rejected: %s", diagnostic.Message)
	}
	for index := range profiles {
		if profiles[index].SourceKey != "" && projection != nil && projection.PackCatalog != nil {
			for _, person := range projection.PackCatalog.Characters {
				if person.SourceKey != profiles[index].SourceKey {
					continue
				}
				profiles[index].Lifeform = strings.TrimSpace(person.Lifeform)
				if profiles[index].Lifeform == "" {
					profiles[index].Lifeform = character.LifeformHuman
				}
				profiles[index].CultivationMethodID = strings.TrimSpace(person.CultivationMethodID)
				break
			}
		}
		if _, established := state.Profile(profiles[index].ID); !established && profiles[index].Lifeform == "" {
			profiles[index].Lifeform = character.LifeformHuman
		}
	}
	// Bind each first appearance to the actor its author source key already owns.
	// The model mints an id from the name, so without this a preset person who
	// comes back under a different spelling — or in a later branch that inherited
	// the registry — would be registered a second time, with a second memory and
	// second set of relationships. The source key is the author's identity and
	// the actor id is this story's, so the mapping runs in that direction only.
	//
	// The remap covers the turn's lines and threads as well as the profile: they
	// were submitted under the minted id, and leaving them behind would record
	// speech by a character the story never registered.
	rebound := turnCharacterAliases(input, projection)
	for index := range profiles {
		establishedID, ok := rebound[profiles[index].ID]
		if !ok {
			continue
		}
		existing, exists := state.Profile(establishedID)
		profiles[index].ID = establishedID
		// The established actor keeps its own name: a later turn's spelling of it
		// is not a rename.
		if exists && existing.Name != "" {
			profiles[index].Name = existing.Name
		}
	}
	// A person staged through state_changes is as formally present as one named
	// in character_facts. Registering them here, in the same commit, is what
	// keeps one face from becoming two records: the window, the narrator's cast
	// block and the discovery exclusion list all read this registry.
	profiles = append(profiles, actorBackedProfiles(storyID, turn, postTurnState, worldTime, state, profiles)...)
	facts := character.TurnCharacterFacts{}
	if input != nil {
		facts = input.Facts()
	}
	if len(rebound) > 0 {
		facts = rebindCharacterFacts(facts, rebound)
	}
	if playerLetter != nil {
		var gameplay *StoryGameplayProfile
		if packBinding != nil {
			gameplay = packBinding.Gameplay
		}
		if gameplay == nil || gameplay.Communication != "letter" || gameplay.LetterDeliverySeconds < 24*60*60 || gameplay.LetterDeliverySeconds > 30*24*60*60 {
			return nil, ErrPlayerLetterUnavailable
		}
		recipientName := playerLetter.CharacterID
		if recipient, exists := state.Profile(playerLetter.CharacterID); exists {
			recipientName = recipient.DisplayName()
		}
		facts.Matters = append(facts.Matters, character.MatterDraft{
			CharacterID: playerLetter.CharacterID, Kind: character.MatterAction,
			Status:       character.MatterOpen,
			Statement:    fmt.Sprintf("一封寄给%s的信：%s", recipientName, playerLetter.Body),
			Counterparty: DefaultActorID,
			Letter:       &character.LetterDeliveryDraft{Body: playerLetter.Body, DeliveryAfterSeconds: gameplay.LetterDeliverySeconds},
		})
	}
	for _, profile := range profiles {
		if err := state.UpsertProfile(profile); err != nil {
			return nil, err
		}
	}
	eventID := newID("chr")
	projected, err := character.Project(character.ProjectionContext{
		StoryID: strings.TrimSpace(storyID), BranchID: branchID,
		TurnID: turn.ID, WorldTime: worldTime, CommitID: eventID,
		// The branch's formal instant, so a deadline that states an interval is
		// bound to the clock rather than left as a number in its own text.
		ClockSeconds:          projection.clockSeconds(),
		Calendar:              calendar.DeadlineUnits(),
		Scene:                 scene,
		VisualContext:         visualObservationContext(state, postTurnState),
		KnownCharacters:       state.CharacterIDs(),
		ExistingMatters:       state.Matters,
		ExistingAffinities:    state.Affinities,
		ExistingRelationships: state.Relationships,
		NextID:                newID,
	}, facts)
	if err != nil {
		return nil, err
	}
	membershipContext := factionMembershipContext{State: membershipState, Catalog: catalog, Scene: scene, PlayerInput: turn.User}
	if projection != nil {
		membershipContext.StateData, membershipContext.Combats = projection.State, projection.Combats
	}
	memberships, err := buildTurnFactionMemberships(membershipContext, input, projected.Exchanges, turn, eventID, worldTime, projection.clockSeconds())
	if err != nil {
		return nil, err
	}
	if projected.Empty() && len(profiles) == 0 && len(memberships) == 0 {
		return nil, nil
	}
	record := CharacterRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: eventID,
		// ParentID is the Turn, not the branch head: this record is part of what
		// that turn established, and a rebuild must place it with its turn.
		ParentID: turn.ID, BranchID: branchID, Ts: turn.Ts,
		TurnID: turn.ID, Cursor: eventID,
		CommandID: turn.AgentCommandID, CommandHash: turn.AgentCommitHash,
		FactionMemberships: memberships,
		Profiles:           profilesToWrite(state.Profiles, profiles),
		Exchanges:          projected.Exchanges, Knowledge: projected.Knowledge, Matters: projected.Matters,
		// Messages ride the same record as the turn's other character facts, so a
		// message the turn caused and the events that caused it commit together:
		// there is no window in which the story advanced but the message it
		// promised has not been sent yet.
		Messages: projected.Messages,
		// Feeling changes ride the same record for the same reason: a task's
		// reward, the story's advance and the feeling it caused are one commit,
		// so there is no state in which the reward landed but the relationship it
		// changed did not.
		Affinities:    projected.Affinities,
		Relationships: projected.Relationships,
	}
	if err := validateCharacterRecordEvent(record); err != nil {
		return nil, err
	}
	return &record, nil
}

// turnCharacterAliases resolves every id this turn submitted through the
// source-key binding, in one place so the profile, the lines and the Actor ops
// cannot disagree.
//
// A source key is the author's identity; the actor id is this story's. When a
// preset person comes back under a newly minted id — a different spelling, a
// title instead of a name — the established actor is the one they belong to.
// The mapping runs only in that direction: a key never re-points an established
// actor at someone else, and two people are never merged for sharing a name.
func turnCharacterAliases(input *TurnCharacterFactsInput, projection *storyBranchProjection) map[string]string {
	state := character.State{}
	if projection != nil && projection.Characters != nil {
		state = projection.Characters.Normalize()
	}
	return TurnCharacterAliases(input, state)
}

// TurnCharacterAliases is the exported entry point for the submission path,
// which holds the branch's character registry rather than the branch projection.
func TurnCharacterAliases(input *TurnCharacterFactsInput, state character.State) map[string]string {
	if input == nil || len(input.Profiles) == 0 {
		return nil
	}
	state = state.Normalize()
	aliases := map[string]string{}
	for _, draft := range input.Profiles {
		sourceKey := strings.TrimSpace(draft.SourceKey)
		submittedID := strings.TrimSpace(draft.CharacterID)
		if sourceKey == "" || submittedID == "" {
			continue
		}
		existing, ok := state.ProfileBySourceKey(sourceKey)
		if !ok || existing.ID == submittedID {
			continue
		}
		aliases[submittedID] = existing.ID
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

// actorBackedProfiles registers the people this turn's state_changes staged but
// character_facts did not name.
//
// Only a newly created, non-system Actor becomes a contact. A merely mentioned
// name has no Actor; the catalog's hundred-odd people have no Actor until one of
// them actually appears; an archived Actor is someone who left; and the three
// foundational Actors (protagonist, story, world) are not people the player
// talks to. The profile is built from the Actor's own identity fields and
// description, so it says no more than the state itself says.
//
// Two of those fields are not what they look like on the record. The schema
// writes the template id into `role` when a create stated no role, and that id
// is a label for the schema, not something this person is; writing it here put
// "important_character" under the name on the person card. And a create that
// described the person in the template's own identity field never filled the
// top-level `description`, so the card came out empty. Both are read as what
// they are: an unstated role stays empty, and the introduction falls back to
// the identity field the turn actually wrote.
//
// The rest of the card is read the same way. A create that stated the person's
// gender or voice stated a fact about them, and leaving it on the Actor while
// the profile stayed blank meant the window could not show it and a later turn
// had to state it again. Both are read from the Actor record when the create
// wrote them there, and both stay empty when it did not: an empty gender is the
// honest value for someone nobody decided, and it is never derived from a name,
// a picture or a role.
//
// A create that said nothing about who this person is produces no profile at
// all. Registering one would put an empty card in the window and in the
// discovery exclusion list, which is worse than not listing them yet: the next
// turn that does describe them registers them properly, and until then the
// Actor itself still holds whatever the state knows.
func actorBackedProfiles(storyID string, turn TurnEvent, postTurnState map[string]any, worldTime string, state character.State, submitted []character.Profile) []character.Profile {
	if postTurnState == nil || turn.TurnResult == nil {
		return nil
	}
	created := createdActorIDs(turn.TurnResult.StateUpdates)
	if len(created) == 0 {
		return nil
	}
	claimed := make(map[string]bool, len(submitted))
	for _, profile := range submitted {
		claimed[profile.ID] = true
	}
	actors, _ := postTurnState[actorStateRoot].(map[string]any)
	profiles := make([]character.Profile, 0, len(created))
	for actorID := range created {
		if actorID == "" || claimed[actorID] || actorStateSystemActor(actorID) {
			continue
		}
		if _, archived := actorArchiveRecordFromState(postTurnState, actorID); archived {
			continue
		}
		actor, _ := actors[actorID].(map[string]any)
		if actor == nil {
			continue
		}
		if existing, ok := state.Profile(actorID); ok && existing.ID != "" {
			continue
		}
		templateID := normalizeActorStateID(fmt.Sprint(actor["template_id"]))
		role := actorIdentityText(actor, "role")
		if role == templateID {
			// The create stated no role. Empty keeps whatever an earlier revision
			// settled (UpsertProfile fills an unstated field), and an empty role is
			// what the person card should show for someone whose part in the story
			// nobody has said yet.
			role = ""
		}
		description := actorIdentityText(actor, "description")
		if description == "" {
			description = actorIdentityDescription(actor)
		}
		if description == "" {
			// Nothing described this person. A card with only a name is not a
			// contact: the window would list someone the story has said nothing
			// about, and the discovery exclusion list would hide them from the
			// turn that does describe them. The Actor keeps whatever the state
			// knows, and a later create or profile submission registers them.
			continue
		}
		profile := character.Profile{
			ID: actorID, StoryID: strings.TrimSpace(storyID), Name: actorIdentityText(actor, "name"),
			Role: role, Description: description,
			Gender:             character.NormalizeGender(actorIdentityText(actor, "gender")),
			SpeechStyle:        actorIdentityText(actor, "speech_style"),
			Tags:               actorIdentityTags(actor),
			FirstSeenWorldTime: worldTime, FirstSeenSceneNode: turn.ID,
			Revision: 1, CreatedAt: worldTime, UpdatedAt: worldTime,
		}
		if profile.Name == "" {
			profile.Name = actorID
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

// actorIdentityDescription is the introduction a create wrote into the
// template's own identity field.
//
// Every preset template has one — 基本身份 for a person, 类型与外观 for a hostile
// entity — and it is where a model that filled `initial_state` and stopped put
// the sentence describing who this is. Reading only the top-level description
// left the person card blank for exactly the path that had said something. The
// fields are tried in template order so the same person always gets the same
// sentence, and a free-form field like 性格与背景 is not consulted: that is a
// different fact about them, and printing it as their introduction would say
// something the create did not.
func actorIdentityDescription(actor map[string]any) string {
	rawState, _ := actor["state"].(map[string]any)
	if rawState == nil {
		return ""
	}
	for _, fieldID := range actorIdentityFieldIDs {
		text, ok := rawState[fieldID].(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		// The identity field is a block: it can hold more than a card caption.
		// The card takes the opening sentences and no more, which is the same
		// capacity the submitted-profile path enforces.
		if runes := []rune(strings.TrimSpace(text)); len(runes) > introducedIntroductionMaxRunes {
			return strings.TrimSpace(string(runes[:introducedIntroductionMaxRunes-1])) + "…"
		}
		return strings.TrimSpace(text)
	}
	return ""
}

// actorIdentityFieldIDs are the preset template fields that hold who someone
// is, in the order they are tried. They are field names, not template ids: the
// same names are used across every genre preset. A template whose identity
// field is named something else simply falls back to the top-level description,
// which is where it was before.
var actorIdentityFieldIDs = []string{"基本身份", "类型与外观", "空间身份"}

func actorIdentityText(actor map[string]any, key string) string {
	value, _ := actor[key].(string)
	return strings.TrimSpace(value)
}

// actorIdentityTags reads the labels a create wrote on the Actor record. A
// replayed or hand-edited record can hold them as []any, so both shapes are
// accepted and normalized the same way a submitted list is.
func actorIdentityTags(actor map[string]any) []string {
	switch value := actor["tags"].(type) {
	case []string:
		return normalizeIdentityTags(value)
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return normalizeIdentityTags(values)
	}
	return nil
}

// createdActorIDs reads the actor roots this turn's accepted state_changes
// created. The audit input is what was accepted, so a rejected or rebound create
// never reaches this list.
func createdActorIDs(updates []interactivestate.Update) map[string]bool {
	ids := map[string]bool{}
	for _, update := range updates {
		if update.Op != interactivestate.Create {
			continue
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) != 1 {
			continue
		}
		if actorID := normalizeStatePanelActorID(segments[0]); actorID != "" {
			ids[actorID] = true
		}
	}
	return ids
}

// actorStateSystemActor reports whether an actor id belongs to the story's own
// bookkeeping rather than to a person in it.
func actorStateSystemActor(actorID string) bool {
	switch actorID {
	case DefaultActorID, DefaultStoryContextActorID, DefaultWorldEntitiesActorID:
		return true
	default:
		return false
	}
}

// rebindCharacterFacts rewrites the character ids in a turn's submitted facts
// through the source-key binding, so a line the model filed under the id it
// minted from a name lands on the actor the story already had for that person.
//
// Only ids that were actually rebound are touched. A character the story
// invented has no source key and passes through unchanged.
func rebindCharacterFacts(facts character.TurnCharacterFacts, rebound map[string]string) character.TurnCharacterFacts {
	for i := range facts.Knowledge {
		if visual := facts.Knowledge[i].Visual; visual != nil {
			copy := *visual
			if id, ok := rebound[copy.SubjectID]; ok {
				copy.SubjectID = id
			}
			facts.Knowledge[i].Visual = &copy
		}
	}
	remap := func(id string) string {
		if target, ok := rebound[strings.TrimSpace(id)]; ok {
			return target
		}
		return id
	}
	for index := range facts.Exchanges {
		facts.Exchanges[index].CharacterID = remap(facts.Exchanges[index].CharacterID)
		facts.Exchanges[index].AddressedTo = remap(facts.Exchanges[index].AddressedTo)
	}
	for index := range facts.Knowledge {
		facts.Knowledge[index].CharacterID = remap(facts.Knowledge[index].CharacterID)
		for holderIndex := range facts.Knowledge[index].Holders {
			facts.Knowledge[index].Holders[holderIndex] = remap(facts.Knowledge[index].Holders[holderIndex])
		}
	}
	for index := range facts.Matters {
		facts.Matters[index].CharacterID = remap(facts.Matters[index].CharacterID)
	}
	for index := range facts.Messages {
		facts.Messages[index].CharacterID = remap(facts.Messages[index].CharacterID)
	}
	for index := range facts.Affinities {
		facts.Affinities[index].CharacterID = remap(facts.Affinities[index].CharacterID)
	}
	for index := range facts.Relationships {
		facts.Relationships[index].CharacterID = remap(facts.Relationships[index].CharacterID)
	}
	return facts
}

// cloneTurnCharacterFactsInput deep-copies a submitted character module. The
// accepted module is retained across tool calls in a durable draft, so a later
// call must not be able to mutate what an earlier one accepted.
func cloneTurnCharacterFactsInput(input *TurnCharacterFactsInput) *TurnCharacterFactsInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Memberships = append([]TurnFactionMembershipInput(nil), input.Memberships...)
	cloned.Profiles = append([]TurnCharacterProfileInput(nil), input.Profiles...)
	for i := range cloned.Profiles {
		cloned.Profiles[i].Aliases = append([]string(nil), input.Profiles[i].Aliases...)
		cloned.Profiles[i].Personality = append([]string(nil), input.Profiles[i].Personality...)
		cloned.Profiles[i].Motives = append([]string(nil), input.Profiles[i].Motives...)
		cloned.Profiles[i].Bounds = append([]string(nil), input.Profiles[i].Bounds...)
		if input.Profiles[i].RadioContact != nil {
			contact := *input.Profiles[i].RadioContact
			cloned.Profiles[i].RadioContact = &contact
		}
	}
	cloned.Exchanges = append([]TurnCharacterExchangeInput(nil), input.Exchanges...)
	for i := range cloned.Exchanges {
		if input.Exchanges[i].ParticipationDecision != nil {
			d := input.Exchanges[i].ParticipationDecision.Normalize()
			cloned.Exchanges[i].ParticipationDecision = &d
		}
		if input.Exchanges[i].CooperationDecision != nil {
			d := input.Exchanges[i].CooperationDecision.Normalize()
			cloned.Exchanges[i].CooperationDecision = &d
		}
	}
	cloned.Knowledge = append([]TurnCharacterKnowledgeInput(nil), input.Knowledge...)
	for i := range cloned.Knowledge {
		if input.Knowledge[i].Visual != nil {
			visual := *input.Knowledge[i].Visual
			cloned.Knowledge[i].Visual = &visual
		}
		cloned.Knowledge[i].Holders = append([]string(nil), input.Knowledge[i].Holders...)
	}
	cloned.Matters = append([]TurnCharacterMatterInput(nil), input.Matters...)
	cloned.Messages = append([]TurnCharacterMessageInput(nil), input.Messages...)
	cloned.Affinities = append([]TurnCharacterAffinityInput(nil), input.Affinities...)
	cloned.Relationships = append([]TurnCharacterRelationshipInput(nil), input.Relationships...)
	return &cloned
}
