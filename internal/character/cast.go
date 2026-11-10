package character

import (
	"fmt"
	"sort"
	"strings"
)

// CastMember is one character as a main-story turn sees them: who they are, how
// they speak, where they stand with the protagonist, and what is still
// unfinished. It is deliberately not a Prepared context — the narrator is not a
// character, so the narrator is not limited to one character's knowledge.
type CastMember struct {
	Profile Profile
	// Present reports whether the story's own scene lists them, which is what
	// decides whether the turn may write them a line without moving them there.
	Present bool
	// OpenMatters is this character's unfinished business with the protagonist
	// that is still in force. It is the same record the character window shows,
	// which is what lets an arrangement made in the window be carried by the next
	// story turn.
	OpenMatters []Matter
	// LapsedMatters are threads whose deadline has passed. They are reported
	// separately because the narrator must know they happened without treating
	// them as still authorizing anything.
	LapsedMatters []Matter
	// ResolvedMatters are threads that are over: carried out, refused, or
	// abandoned. They are not outstanding work, so they are not listed as such,
	// but they are what lets the narrator acknowledge help already given instead
	// of asking for it again (§5.5 后果必须有消费者).
	ResolvedMatters []Matter
	// Affinity is this character's current feeling toward the protagonist and
	// the band it falls into. It is stated so the narrator does not write
	// everyone at the same warmth, and so a change the player earned shows in
	// behaviour rather than only in a number.
	Affinity     int
	AffinityBand AffinityBand
	// Relationship is the formal relationship currently standing, if any.
	Relationship *Relationship
	// Age is this person's age on the branch's formal time line, or nil when
	// the story never established one. It is not derived here: the year length
	// is a rule constant this package must not own a second copy of, so the
	// caller that can see both the frozen age and the branch clock supplies it.
	Age *AgeFact
}

// AgeFact is one person's age as the branch's formal clock now reads it.
//
// The design (自制-时间寿元与长时世界推进 §2) asks for two things at once: 年龄按
// 实际游戏时间增加，显示整岁不丢失底层已过时长, and 新人物有可追溯的初始年龄与初态
// 时刻. Established is the second half — the age the world book or the creation
// page stated — kept beside the current one so a narrator writing a decade of
// change still knows who the person was. Years is the first half, floored to
// whole years: an interval that has not reached a full year leaves the age
// where it was rather than rounding a birthday nobody had.
type AgeFact struct {
	Years       int
	Established int
}

// StoryCast lists the characters a story turn should know about, ordered by what
// this turn needs from each of them. The set comes from the branch's own
// registry, so it grows with the story instead of being a fixed pair of names.
// currentWorldTime splits each character's unfinished business into what still
// binds and what has expired.
func StoryCast(state State, scene Scene, currentWorldTime string) []CastMember {
	return StoryCastAt(state, scene, currentWorldTime, 0)
}

// StoryCastAt is StoryCast with the branch's formal instant supplied.
//
// A thread whose deadline was bound to the clock is decided by that number, and
// a reader that passes zero here cannot decide it at all: LapsedAt treats "no
// clock reading" as "not yet due" rather than guessing, so a bound thread would
// stay live forever in every block that forgot to pass the instant. Every caller
// that has a snapshot has the number and must pass it.
func StoryCastAt(state State, scene Scene, currentWorldTime string, clockSeconds int64) []CastMember {
	state = state.Normalize()
	scene = scene.Normalize()
	present := make(map[string]bool, len(scene.Present))
	for _, id := range scene.Present {
		present[id] = true
	}
	members := make([]CastMember, 0, len(state.Profiles))
	for _, profile := range state.Profiles {
		profile = profile.Normalize()
		if profile.ID == "" {
			continue
		}
		live, lapsed := LiveMattersAt(profile.ID, currentWorldTime, clockSeconds, state.Matters)
		value := CurrentAffinity(profile.ID, state.Affinities)
		member := CastMember{
			Profile:         profile,
			Present:         present[profile.ID],
			OpenMatters:     live,
			LapsedMatters:   lapsed,
			ResolvedMatters: ResolvedMatters(profile.ID, state.Matters),
			Affinity:        value,
			AffinityBand:    BandFor(value),
		}
		if relationship, ok := CurrentRelationship(profile.ID, state.Relationships); ok {
			member.Relationship = &relationship
		}
		members = append(members, member)
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Present != members[j].Present {
			return members[i].Present
		}
		// Relevance, not the alphabet. The cast block is trimmed from the end, so
		// whoever sorts last is the one a long story stops seeing — and ordering
		// everyone else by id made that a fixed fact about their name. A thread
		// still in force is the strongest claim a person has on this turn: the
		// arrangement is what the scene has to honour, whether or not they happen
		// to be standing in it. Settled history comes next, because §5.5 后果必须
		// 有消费者 asks the narrator to pay it off. The id only breaks ties.
		iRank, jRank := castRelevance(members[i]), castRelevance(members[j])
		if iRank != jRank {
			return iRank < jRank
		}
		return members[i].Profile.ID < members[j].Profile.ID
	})
	return members
}

// castRelevance ranks a member by how much this turn needs them, low being more
// relevant. Presence is already the primary key, so it is not repeated here.
func castRelevance(member CastMember) int {
	switch {
	case len(member.OpenMatters) > 0:
		return 0
	case len(member.LapsedMatters) > 0:
		// A lapsed thread still binds what the narrator may write: the hour passed
		// and the arrangement must not be carried out. That is closer to an open
		// thread than to settled history.
		return 1
	case len(member.ResolvedMatters) > 0:
		return 2
	default:
		return 3
	}
}

// CastMarkdown renders the cast for the model, with the stable ID it must use
// when recording what a character said or learned. The IDs are the whole point:
// without them the model can only invent a name, and an invented name is a
// second character who shares the first one's face.
//
// Exchanges are listed across every channel because the narrator, unlike a
// character, is not confined to one audience. It must still not let a character
// act on what they were never told, which is why the block says so.
func CastMarkdown(members []CastMember, exchanges []Exchange, maxBytes int) string {
	if len(members) == 0 && len(exchanges) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxCastBytes
	}
	var sb strings.Builder
	sb.WriteString("Characters this story has established. Use the exact id when recording what one of them says, learns or promises.\n")
	for _, member := range members {
		writeCastMember(&sb, member)
	}
	writeRecentExchanges(&sb, exchanges)
	out := strings.TrimSpace(sb.String())
	if len([]byte(out)) <= maxBytes {
		return out
	}
	// Over budget: drop the conversation first, then members from the end. The
	// roster and its IDs survive longest because they are what the turn writes
	// facts against.
	//
	// Each card is measured before it is written, so the block loses a whole
	// member rather than half of one — and the block that comes back is inside the
	// budget it was given, which is what lets a caller size the rest of its
	// context around this number.
	var trimmed strings.Builder
	header := "Characters this story has established. Use the exact id when recording what one of them says, learns or promises.\n"
	trimmed.WriteString(header)
	written := 0
	for _, member := range members {
		var card strings.Builder
		writeCastMember(&card, member)
		if trimmed.Len()+card.Len() > maxBytes {
			break
		}
		trimmed.WriteString(card.String())
		written++
	}
	if written == 0 {
		// Not even one card fits. An empty roster would tell the narrator nobody
		// exists, which is worse than an over-budget roster of one: keep the first
		// member and let the caller's own budget handle it.
		writeCastMember(&trimmed, members[0])
		return strings.TrimSpace(trimmed.String())
	}
	// Whoever did not fit keeps a one-line index entry. Dropping them entirely is
	// how a person who has been in this story for twenty turns becomes someone the
	// narrator may not mention at all — and the ids are what stops a re-entry from
	// minting a second actor with the same face. The line says only what a reader
	// needs to know they exist and where to look; their full card is still in the
	// record.
	if written < len(members) {
		writeCastIndex(&trimmed, members[written:], maxBytes)
	}
	return strings.TrimSpace(trimmed.String())
}

// writeCastIndex writes the condensed form of the members whose full cards did
// not fit. It is the 降温 half of the design's rule: less room, same identity.
func writeCastIndex(sb *strings.Builder, members []CastMember, maxBytes int) {
	const header = "\n### Others this story has established (index only — their full record is in the story)\n"
	if sb.Len()+len(header) > maxBytes {
		return
	}
	sb.WriteString(header)
	for _, member := range members {
		line := fmt.Sprintf("- %s (id: %s)", member.Profile.DisplayName(), member.Profile.ID)
		if member.Present {
			line += " — present in the current scene"
		}
		if len(member.OpenMatters) > 0 {
			line += fmt.Sprintf(" — %d thread(s) still open with the protagonist", len(member.OpenMatters))
		}
		line += "\n"
		if sb.Len()+len(line) > maxBytes {
			return
		}
		sb.WriteString(line)
	}
}

// DefaultMaxCastBytes bounds the cast block. It is a working set, not the whole
// registry: a story with fifty characters gets the ones it needs this turn.
const DefaultMaxCastBytes = 16 * 1024

func writeCastMember(sb *strings.Builder, member CastMember) {
	profile := member.Profile
	fmt.Fprintf(sb, "\n### %s (id: %s)\n", profile.DisplayName(), profile.ID)
	if profile.Name != "" && profile.Name != profile.DisplayName() {
		fmt.Fprintf(sb, "- Their own name is %s; the protagonist knows them as %s.\n", profile.Name, profile.DisplayName())
	}
	if len(profile.Aliases) > 0 {
		fmt.Fprintf(sb, "- Also known as: %s\n", strings.Join(profile.Aliases, ", "))
	}
	if member.Present {
		sb.WriteString("- Present in the current scene.\n")
	}
	if member.Age != nil && member.Age.Years > 0 {
		// The two numbers are printed together because either alone misleads. The
		// current age is what the turn must write; the established one is what the
		// person was authored as, so a decade of change reads as change rather
		// than as a different person.
		fmt.Fprintf(sb, "- Age: %d — %d when this story began. Age with the formal time this branch has really spent.\n",
			member.Age.Years, member.Age.Established)
	}
	if profile.Role != "" {
		fmt.Fprintf(sb, "- Role: %s\n", profile.Role)
	}
	if profile.Lifeform != "" {
		fmt.Fprintf(sb, "- Registered lifeform: %s. Keep this identity fixed.\n", profile.Lifeform)
	}
	if profile.CultivationMethodID != "" {
		fmt.Fprintf(sb, "- Registered species cultivation method: %s. Keep this method fixed; growth is settled by its registered rules.\n", profile.CultivationMethodID)
	}
	if len(profile.Tags) > 0 {
		// The labels the story attached to this person. They are printed so a
		// later turn can keep using the same ones instead of inventing a second
		// set for the same fact, and so a player who learned "药铺伙计" hears it
		// again rather than a synonym.
		fmt.Fprintf(sb, "- Labels: %s\n", strings.Join(profile.Tags, "; "))
	}
	if contact := profile.RadioContact; contact != nil {
		fmt.Fprintf(sb, "- 已登记无线电：设备 %s；频道 %s；状态 %s。设备丢失、断网或失联时不能即时联络；当前回合实际重新接通后才可更新为connected。\n", contact.DeviceItemID, contact.Channel, contact.Status)
	}
	if profile.Gender != "" {
		// Who this person is, kept from the turn that settled it. It is not a
		// cue for temperament: the voice is in the Speech line below, and the
		// warning keeps a narrator from turning it into one.
		fmt.Fprintf(sb, "- Gender: %s. Write the person, not a stereotype of it; the voice is in the line below.\n", profile.Gender)
	}
	if profile.Description != "" {
		fmt.Fprintf(sb, "- %s\n", profile.Description)
	}
	if len(profile.Personality) > 0 {
		fmt.Fprintf(sb, "- Character: %s\n", strings.Join(profile.Personality, "; "))
	}
	if profile.SpeechStyle != "" {
		fmt.Fprintf(sb, "- Speaks: %s\n", profile.SpeechStyle)
	}
	if len(profile.Motives) > 0 {
		fmt.Fprintf(sb, "- Wants: %s\n", strings.Join(profile.Motives, "; "))
	}
	if profile.Attitude != "" {
		fmt.Fprintf(sb, "- Current stance toward the protagonist: %s\n", profile.Attitude)
	}
	fmt.Fprintf(sb, "- Feeling toward the protagonist: %d (%s). It is a reference, not a permission: this person can still refuse at a high number and cooperate at a low one.\n", member.Affinity, member.AffinityBand)
	if member.Relationship != nil {
		fmt.Fprintf(sb, "- What stands between them and the protagonist: %s — %s\n", member.Relationship.Kind, oneLine(member.Relationship.Statement))
	}
	if len(profile.Bounds) > 0 {
		fmt.Fprintf(sb, "- Will not: %s\n", strings.Join(profile.Bounds, "; "))
	}
	if len(member.OpenMatters) > 0 {
		sb.WriteString("- Unfinished with the protagonist:\n")
		for _, matter := range member.OpenMatters {
			fmt.Fprintf(sb, "  - [%s] %s\n", matter.Kind, matter.Statement)
			// The id is printed so a later turn can name the thread it settles.
			// Without it the field asks for something the model has never seen.
			fmt.Fprintf(sb, "    - id: %s\n", matter.ID)
			if matter.EarliestSeconds > 0 {
				fmt.Fprintf(sb, "    - 最早可复查的正式秒数：%d。时间只是条件，未收到本人实际回应不能认定已完成；可放下或改约。\n", matter.EarliestSeconds)
			}
			if matter.Deadline != "" {
				fmt.Fprintf(sb, "    - Lapses at %s.\n", matter.Deadline)
			}
			if guidance := matterGuidance(matter); guidance != "" {
				fmt.Fprintf(sb, "    - %s\n", guidance)
			}
			// An unfinished thread is the one place a character has a concrete
			// reason to write rather than speak, so the message channel is named
			// here. It is deliberately not gated on the deadline field: the model
			// routinely leaves that field empty even when the prose named a time, and
			// a thread that came due then produced nothing at all. In the acceptance
			// run an appointment passed its hour and the turn recorded the promise as
			// abandoned with no word sent — 82 character records across every
			// journal, not one carrying a message. The conditions that keep this from
			// becoming the per-turn routine the schema rules out are stated in the
			// same breath.
			fmt.Fprintf(sb, "    - If this thread is coming due, or its own terms call for sending word, and %s cannot reach the protagonist in person, character_facts.messages is how the word they owe arrives: give the message this thread as its basis. Do not write merely because a thread is open, and do not send word whose time has already passed — a thread that lapsed unsent is recorded as abandoned instead.\n", member.Profile.DisplayName())
		}
	}
	// A lapsed thread is reported so the narrator does not contradict it, but it
	// is labelled as spent: an invitation that ran out must not be carried out as
	// though it were still open.
	if len(member.LapsedMatters) > 0 {
		sb.WriteString("- Lapsed with the protagonist — the agreed hour passed and these no longer bind either side. Do not carry one out, and do not write one as done:\n")
		for _, matter := range member.LapsedMatters {
			fmt.Fprintf(sb, "  - [%s] %s (lapsed at %s, id: %s; %s)\n", matter.Kind, matter.Statement, matter.Deadline, matter.ID, matterGuidance(matter))
		}
	}
	// What is already over is reported so the person can acknowledge it. §5.5
	// 后果必须有消费者, and 自制-游玩循环与选择后果 §5 names the consumer: a character
	// who later refers to a promise actually kept or a help actually given. Without
	// this line the narrator's only record of the thread is its absence, and the
	// absence reads as work never done — so the person asks for it again, or the
	// prose never pays it off at all.
	writeResolvedMatters(sb, member)
}

// resolvedMatterLimit bounds the settled threads one character carries into a
// turn. A long story accumulates them, and the newest are the ones a scene can
// still refer to; the rest stay in the record and are reachable by recall.
const resolvedMatterLimit = 6

// writeResolvedMatters states what this character and the protagonist have
// already been through, so the narrator can let it pay off.
//
// The wording does two jobs at once. It tells the narrator this happened — the
// consumer §5.5 requires — and it keeps the settled thread from being treated as
// new business: an entry the prose already delivered must not be delivered
// again, and a refusal must not be re-offered as though nobody had answered.
// Abandoned threads are included because a refusal is an outcome too: §5.5
// 失效或被放弃的线索保留历史, and a person who refused once can react to being asked
// again rather than having no memory of the first request.
func writeResolvedMatters(sb *strings.Builder, member CastMember) {
	if len(member.ResolvedMatters) == 0 {
		return
	}
	shown := member.ResolvedMatters
	if len(shown) > resolvedMatterLimit {
		shown = shown[:resolvedMatterLimit]
	}
	sb.WriteString("- Already settled with the protagonist — these happened. Let this person refer to them where it fits, and never present one as still owed or as new business:\n")
	for _, matter := range shown {
		fmt.Fprintf(sb, "  - [%s/%s] %s (id: %s)\n", matter.Kind, matter.Status, matter.Statement, matter.ID)
	}
	if len(member.ResolvedMatters) > len(shown) {
		fmt.Fprintf(sb, "  - (%d earlier settled threads omitted; they are still in the record)\n",
			len(member.ResolvedMatters)-len(shown))
	}
}

func writeRecentExchanges(sb *strings.Builder, exchanges []Exchange) {
	if len(exchanges) == 0 {
		return
	}
	scoped := make([]Exchange, 0, len(exchanges))
	for _, exchange := range exchanges {
		if exchange = exchange.Normalize(); exchange.ID != "" {
			scoped = append(scoped, exchange)
		}
	}
	if len(scoped) == 0 {
		return
	}
	if len(scoped) > DefaultMaxCastExchanges {
		scoped = scoped[len(scoped)-DefaultMaxCastExchanges:]
	}
	sb.WriteString("\n### Conversation so far (all channels, oldest first)\n")
	for _, exchange := range scoped {
		if exchange.Speaker == SpeakerPlayer {
			// The player is not a character and has no id. A line the protagonist
			// spoke is filed under the character it was spoken to, so the label
			// names that character rather than minting a protagonist id for the
			// model to copy back into character_facts. A line spoken in a group is
			// filed under the group, which is not a person either.
			subject := exchange.CharacterID
			if exchange.Channel == ChannelGroup {
				subject = "the group"
			}
			fmt.Fprintf(sb, "- [%s] the protagonist (spoken to %s): %s\n", exchange.Channel, subject, oneLine(exchange.Text))
			continue
		}
		// A group line carries its real speaker in AuthorID, not CharacterID: the
		// group is the conversation space, not the person. Printing CharacterID
		// here would show an empty name for every line said in a group.
		speaker := exchange.CharacterID
		if exchange.Channel == ChannelGroup && exchange.AuthorID != "" {
			speaker = exchange.AuthorID
		}
		fmt.Fprintf(sb, "- [%s] %s: %s\n", exchange.Channel, speaker, oneLine(exchange.Text))
	}
	sb.WriteString("\nA character may only act on what they witnessed or were told. A line spoken on a private channel or in an incoming message did not reach anyone who was not in its audience.\n")
	sb.WriteString("The protagonist is not a character and has no character_id. Never record the protagonist, the player, or 主角 as a character_id: a line the protagonist spoke belongs to the character they spoke to, with speaker set to player.\n")
}

// DefaultMaxCastExchanges bounds the shared conversation shown to a story turn.
const DefaultMaxCastExchanges = 20
