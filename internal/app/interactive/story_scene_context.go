package interactiveapp

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/character"
	"ta/internal/interactive"
)

// storySceneContext is the short statement of what this turn's scene actually
// holds: where the protagonist is, when, who is present, and which arrangements
// are still open rather than settled.
//
// It exists because the committed scene is spread across three records — the
// story state's scene fields, the world clock and journeys, and the character
// projection's matters — and a narrator reading only prose reconstructs the
// gaps by inventing them. Everything here is read from committed state; nothing
// is inferred, and a field the story never set stays absent rather than being
// filled with a plausible value.
//
// The block is deliberately placed in the turn's dynamic context, not the stable
// prefix: it changes every turn, and moving it into the prefix would break the
// cache for every fixed rule and style fragment behind it.
func (c *Conversation) storySceneContext(storyCtx interactive.StoryContext, characters character.State, charactersOK bool, protagonistAge *character.AgeFact) string {
	if c == nil || c.store == nil {
		return ""
	}
	scene := interactive.StoryScene(storyCtx.Snapshot)
	world := c.storyWorldState(storyCtx)
	var members []character.CastMember
	if charactersOK {
		members = character.StoryCastAt(characters, scene, scene.WorldTime, interactive.SnapshotClockSeconds(storyCtx.Snapshot))
	}
	block := renderStoryScene(scene, world, members, protagonistAge, storyCtx.Meta.Calendar, interactive.LifespanEdgeFor(storyCtx.Meta, storyCtx.Snapshot.State, interactive.SnapshotClockSeconds(storyCtx.Snapshot)), StoryRuntimeContextMaxBytes)
	block = appendSceneSection(block, c.storySceneEngagementLine(storyCtx))
	if charactersOK {
		block = appendSceneSection(block, storySceneMessagesBlock(characters, scene.WorldTime, interactive.SnapshotClockSeconds(storyCtx.Snapshot), StoryRuntimeContextMaxBytes))
	}
	return block
}

// appendSceneSection adds a section to the scene block without letting an empty
// section create one. The header belongs to the block, so a section that is the
// first thing written still opens with it.
func appendSceneSection(block, section string) string {
	if section == "" {
		return block
	}
	if block == "" {
		return "## Current Scene (source: committed story state and character registry)\n\n" + section
	}
	return block + "\n" + section
}

// storySceneMessagesBlock states the word that has already reached the
// protagonist, which is the one part of the record the scene block cannot carry.
//
// A letter is not a scene fact. It was written at its own hour, and the story may
// have moved on for days since; the scene block describes where the turn stands,
// so a narrator reading only it has no way to know the word arrived at all. That
// gap is the visible half of a seclusion: the player leaves, an invitation
// arrives, and the return to the main story was written as though the day had
// passed silently, or as though the offer were being made now. Both are wrong in
// the same direction, and the record holds the correction.
//
// Only messages the protagonist was sent and has not handled are listed.
// character.PendingMessages drops ignored and handled ones, so a thread the
// player already dealt with is not reopened here. A message is addressed to the
// protagonist alone, so listing it broadcasts nothing the sender kept private.
//
// Each line keeps its own sending hour. A letter that says its hour has passed is
// marked as no longer offered, because an invitation whose window closed must not
// be accepted or paid out afterwards, and a narrator that read it as current
// would do exactly that.
func storySceneMessagesBlock(state character.State, worldTime string, clockSeconds int64, maxBytes int) string {
	messages := character.PendingMessages(state.Messages, worldTime)
	if len(messages) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = StoryRuntimeContextMaxBytes
	}
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		if len(lines) >= sceneMessageLimit {
			break
		}
		message = message.Normalize()
		text := strings.Join(strings.Fields(message.Text), " ")
		if text == "" {
			continue
		}
		name := message.CharacterID
		if profile, ok := state.Profile(message.CharacterID); ok {
			name = profile.DisplayName()
		}
		sent := message.WorldTime
		if sent == "" {
			sent = "an hour the record does not name"
		}
		line := fmt.Sprintf("  - %s (sent %s): %s", name, sent, clipSceneRunes(text, sceneMessageRunes))
		deadline := strings.TrimSpace(message.Deadline)
		switch {
		case message.LapsedAt(worldTime, clockSeconds):
			if deadline == "" {
				deadline = "an hour the record does not name"
			}
			line += fmt.Sprintf(" — its hour passed at %s; what it offered no longer stands, so do not accept it or hand it over as though it did", deadline)
		case deadline != "":
			line += fmt.Sprintf(" — its hour runs to %s", deadline)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	header := "- Letters that reached you — written at the hour named, not happening in this scene. Receiving one accepts nothing, so one still stands as an open word until you answer it. Do not write one as spoken here, and do not treat one whose hour has passed as still offered:\n"
	var sb strings.Builder
	if len([]byte(header)) > maxBytes {
		return ""
	}
	sb.WriteString(header)
	for _, line := range lines {
		if sb.Len()+len([]byte(line))+1 > maxBytes {
			sb.WriteString("  - (further letters omitted for budget)\n")
			return sb.String()
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

// clipSceneRunes bounds one quoted letter. A message is bounded at commit, so
// this only trims the unusual long one rather than policing the sender.
func clipSceneRunes(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

// storySceneEngagementLine states that the scene has committed an exchange of
// blows, which is the one scene fact the rest of the block cannot express.
//
// It is stated because the fight is not the narrator's to write. A committed
// declaration is a fight the window owns, and a model reading only Place, Time
// and Present has no way to know that its next sentence is the outcome of a
// battle the rules have not settled. The line names the opponents so the narrator
// knows who the fight is with, and it says where the exchange is decided rather
// than leaving the model to invent a result.
func (c *Conversation) storySceneEngagementLine(storyCtx interactive.StoryContext) string {
	system := storyDirectorForSnapshot(c.StoryRuntimeForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema).ActorState
	if !interactive.StoryDeclaresEngagement(storyCtx.Snapshot.State, system) {
		return ""
	}
	opponents := interactive.StoryEngagementActorIDs(storyCtx.Snapshot.State, system)
	if len(opponents) == 0 {
		return ""
	}
	return fmt.Sprintf("- In combat: %s — blows are being exchanged in this scene. The exchange is settled round by round in the combat window, not written here; do not narrate its outcome.\n", strings.Join(opponents, "、"))
}

// storyWorldState reads this branch's world projection once for a whole context
// assembly.
//
// The scene block and the cast block both need the same formal clock, and a
// second read could land after a commit and let the two halves of the context
// describe different instants — the same drift the single character read exists
// to prevent. A failed read narrows the block rather than removing it: the scene
// fields are still committed state.
func (c *Conversation) storyWorldState(storyCtx interactive.StoryContext) interactive.WorldState {
	world, err := c.store.WorldStateFor(c.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.WorldState{}
	}
	return world
}

// A scene block lists a few letters at most. The character window is where the
// whole inbox lives; this is the reminder that word arrived, not a second inbox.
const (
	sceneMessageLimit = 6
	sceneMessageRunes = 240
)

// renderStoryScene writes the block, measuring each section before adding it so
// a long list is dropped whole rather than cut mid-entry.
func renderStoryScene(scene character.Scene, world interactive.WorldState, members []character.CastMember, protagonistAge *character.AgeFact, calendar *interactive.StoryCalendar, lifespan *interactive.LifespanEdge, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = StoryRuntimeContextMaxBytes
	}
	scene = scene.Normalize()
	// Only a scene field can open the block. A journey or an open matter on its
	// own describes where someone was going or what they once offered, not where
	// this turn stands, and stating it without a place reads as a scene that is
	// not there.
	if scene.Location == "" && scene.WorldTime == "" && len(scene.Present) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Current Scene (source: committed story state and character registry)\n\n")
	// The block states only what the committed records hold. A field the story
	// never wrote stays absent rather than being asserted empty: "no one is
	// recorded here" is not the same fact as "no one is here", and a narrator
	// given the second will write an empty room.

	if location := strings.TrimSpace(world.Location); location != "" {
		fmt.Fprintf(&sb, "- Place: %s\n", location)
	} else if scene.Location != "" {
		fmt.Fprintf(&sb, "- Place: %s\n", scene.Location)
	}
	if time := strings.TrimSpace(scene.WorldTime); time != "" {
		fmt.Fprintf(&sb, "- Time: %s\n", time)
	}
	writeSceneFormalTime(&sb, scene, world, calendar)
	writeSceneProtagonistAge(&sb, protagonistAge)
	writeSceneLifespanEdge(&sb, lifespan)
	// The committed event title is deliberately not repeated here: the Actor
	// State handbook already lists 当前事件 as a field the turn must submit, and
	// saying it twice spends the same budget twice.
	if journey := activeJourneyLine(world); journey != "" {
		fmt.Fprintf(&sb, "- On the road: %s\n", journey)
	}

	writeScenePresent(&sb, scene, members, maxBytes)
	writeSceneOpenMatters(&sb, members, world.Clock.Seconds, maxBytes)
	writeSceneLapsedMatters(&sb, members, maxBytes)
	if sb.Len() == len("## Current Scene (source: committed story state and character registry)\n\n") {
		// Nothing but the header: the story has committed no scene yet.
		return ""
	}
	return strings.TrimSpace(sb.String())
}

// writeSceneFormalTime reports the branch's formal clock beside the scene's own
// display time.
//
// The two are not the same fact, and that is why both are printed. 当前时间 is a
// label the story wrote; the formal clock is the one number a rest, a training
// stretch, a craft batch or a journey really moves. Nothing writes the label
// when formal time advances, so a narrator reading only the label sees the
// afternoon the character set out in, even after a day of rest has passed, and
// writes the missed appointment as though its hour had not come.
//
// The interval is stated in the year/month/day units the story froze when it
// started, so the narrator has a duration it can carry into prose rather than a
// bare second count it would have to convert by guesswork.
//
// This line does not reconcile the two axes by itself — the scene block's own
// date line does that. What it does is stop the scene block from asserting a
// stillness the records contradict: the elapsed interval is printed as the
// committed fact it is, and the narrator is told which of the two moved.
func writeSceneFormalTime(sb *strings.Builder, scene character.Scene, world interactive.WorldState, calendar *interactive.StoryCalendar) {
	if world.Clock.Seconds <= 0 {
		// No formal interval has ever been spent on this branch, so there is no
		// second number to report and the label stands on its own.
		return
	}
	elapsed := strings.TrimSpace(interactive.DescribeElapsed(world.Clock.Seconds))
	hasCalendar := calendar != nil && calendar.Validate() == nil
	if hasCalendar {
		elapsed = strings.TrimSpace(calendar.DescribeElapsed(world.Clock.Seconds))
	}
	if elapsed == "" {
		return
	}
	if strings.TrimSpace(scene.WorldTime) == "" {
		fmt.Fprintf(sb, "- Formal time: %s since %s\n", elapsed, strings.TrimSpace(world.Clock.Anchor))
	} else {
		fmt.Fprintf(sb, "- Formal time: %s since the story began. 当前时间 above is a label the story wrote, not a reading of this clock; rest, training, craft and travel move this one, so if they disagree the interval here is what has really passed.\n", elapsed)
	}
	writeSceneStoryDate(sb, world, calendar, hasCalendar)
}

// writeSceneStoryDate states which day the story has reached, counting the day
// it began as 第1年1月1日.
//
// The interval above answers "how long has passed"; this answers "what day is
// it", and only the second one can be written into 当前时间. The label the pack
// wrote was true at the start and has never moved since, so a narrator asked for
// today's date has nothing else to read: without this it either repeats a date
// the clock has left behind or invents one, and §审查采纳补充 forbids taking the
// first number in a date as the comparison.
//
// It is a position on this branch's own axis, not a claim about the world's
// calendar. A pack that writes 三月初七 still writes its own date in prose; this
// says only how far into this story that day is, which is what makes two
// otherwise incomparable records comparable.
func writeSceneStoryDate(sb *strings.Builder, world interactive.WorldState, calendar *interactive.StoryCalendar, hasCalendar bool) {
	if !hasCalendar {
		return
	}
	date := strings.TrimSpace(calendar.DescribeDate(world.Clock.Seconds))
	if date == "" {
		return
	}
	fmt.Fprintf(sb, "- Story date: %s — the day this story began counts as 第1年1月1日. Write 当前时间 from this date rather than repeating the label above, and keep the world's own names for months and festivals.\n", date)
}

// writeSceneProtagonistAge states the protagonist's own age on the formal time
// line, which the cast block cannot: the protagonist is not a character and has
// no profile, so their age is never in that roster.
//
// It exists because a long seclusion is the one case where the narrator must
// write the protagonist differently without any prose saying so. The creation
// page established an age, the branch clock has since spent an interval, and
// neither number alone says what the person is now. §2 is explicit that 未知生日
// 不由AI猜成精确日期, so this states whole years since the story began and keeps
// the established age beside it rather than inventing a birthday.
func writeSceneProtagonistAge(sb *strings.Builder, age *character.AgeFact) {
	if age == nil || age.Years <= 0 || age.Established <= 0 {
		return
	}
	fmt.Fprintf(sb, "- Your own age: %d — you were %d when this story began. It follows the formal time above, not the label.\n",
		age.Years, age.Established)
}

// writeSceneLifespanEdge states how much of the protagonist's confirmed lifespan
// this branch has left.
//
// It exists because the crossing is decided at commit, after the model has
// already written the turn. §4 requires 玩家即将跨过本人明确寿尽边界时，应在行动真正
// 越界之前自然提示处境，允许其决定出关或继续, and a narrator that learns of the edge
// only from a refused or rewritten turn cannot write that moment: it either
// describes a seclusion the rules cut short, or says nothing and the player finds
// the brink without warning.
//
// The numbers are the character's own. A cultivator knows their realm's limit and
// their own age, so stating them is not an omniscient broadcast; §4 forbids that
// for other people's exact death dates, and this block never states anyone else's.
// The instruction against a countdown matters as much as the numbers: without it
// a model writes 他只剩三年可活 into every turn, which is neither the situation §4
// asks for nor something the prose should keep repeating.
func writeSceneLifespanEdge(sb *strings.Builder, edge *interactive.LifespanEdge) {
	if edge != nil && edge.NoFixedLimit {
		sb.WriteString("- 寿元：此境不按固定年龄判定自然寿尽，实际年龄仍随正式时间增长；这不等于免疫伤亡或天劫。周期天劫和飞升尚无自动结算，不得凭一段年数自动宣告渡劫成功、飞升或死亡。\n")
		return
	}
	if edge == nil || edge.TotalYears <= 0 {
		return
	}
	if edge.Reached {
		// Standing on the edge is alive, and the turn is where the player chooses.
		// Saying so is what stops the narrator from writing a death the rules have
		// not declared.
		fmt.Fprintf(sb, "- Lifespan: this realm's confirmed limit is %d years and you are %d. The formal time above has reached that edge — you are alive and still choose; the story ends only if the clock moves past it.\n",
			edge.TotalYears, edge.AgeAtEdge)
		return
	}
	fmt.Fprintf(sb, "- Lifespan: this realm's confirmed limit is %d years, so this life ends at %d. You are %d now, which leaves %d whole years of formal time. The character knows this; do not turn it into a countdown in the prose, and state it only when the scene makes it relevant. If the player asks for an interval long enough to reach the edge, stop the scene short of it and let them choose whether to go on.\n",
		edge.TotalYears, edge.AgeAtEdge, edge.CurrentAge, edge.RemainingYears)
}

// activeJourneyLine states an unfinished journey, which is the one world record
// that changes what "where they are" means.
func activeJourneyLine(world interactive.WorldState) string {
	ids := make([]string, 0, len(world.ActiveJourneys))
	for id := range world.ActiveJourneys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		destination := strings.TrimSpace(world.ActiveJourneys[id])
		if destination == "" {
			continue
		}
		return destination + " (journey " + id + ")"
	}
	return ""
}

// writeScenePresent names who the committed scene places here, with the stable
// id the turn writes facts against. People the registry does not hold are
// reported by their scene name alone: a name in the scene field is a committed
// fact even when the profile has not been written yet.
func writeScenePresent(sb *strings.Builder, scene character.Scene, members []character.CastMember, maxBytes int) {
	if len(scene.Present) == 0 {
		return
	}
	byID := make(map[string]character.CastMember, len(members))
	for _, member := range members {
		byID[member.Profile.ID] = member
	}
	present := append([]string(nil), scene.Present...)
	sort.Strings(present)
	sb.WriteString("- Present:\n")
	for _, id := range present {
		line := "  - " + id
		if member, ok := byID[id]; ok {
			line = fmt.Sprintf("  - %s (id: %s)", member.Profile.DisplayName(), member.Profile.ID)
		}
		if len([]byte(sb.String()))+len([]byte(line))+1 > maxBytes {
			sb.WriteString("  - (further names omitted for budget)\n")
			return
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

// writeSceneLapsedMatters states the arrangements whose agreed hour has passed.
//
// It exists because silence was being read as permission to proceed. A thread
// that came due drops out of the open list, and before this the scene block said
// nothing else about it, so a narrator reading the scene saw an ordinary day and
// carried the arrangement out as though it still stood. The design is explicit
// that 未执行不算完成: an hour that passed without the work being done is an
// outcome of its own, and the half of the context that describes the scene has
// to say so. The cast block already reports the same verdict, so the two halves
// of the context agree.
//
// The wording is careful about the two things a lapse is not: it is not a
// failure the rules declared, and it is not an erasure. What it states is that
// the agreed hour passed and the thread no longer binds, which is what the
// records hold. The person it was owed to can still react, refuse to trust the
// protagonist again, or accept late work — those are ordinary story outcomes.
// What the narrator must not do is write the arrangement as though it had been
// kept. A permission is stated separately because what it loses is the right
// itself, not an act that went undone.
func writeSceneLapsedMatters(sb *strings.Builder, members []character.CastMember, maxBytes int) {
	lines := make([]string, 0)
	for _, member := range members {
		for _, matter := range member.LapsedMatters {
			statement := strings.TrimSpace(matter.Statement)
			if statement == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("  - [%s] %s (with %s; %s, lapsed at %s, id: %s)",
				matter.Kind, statement, member.Profile.DisplayName(), character.LapseClause(matter.Kind), matter.Deadline, matter.ID))
		}
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	header := "- Came due — the agreed hour has passed and these no longer bind. Do not write one as done, and do not act as though it still stood:\n"
	if len([]byte(sb.String()))+len([]byte(header)) > maxBytes {
		return
	}
	sb.WriteString(header)
	for _, line := range lines {
		if len([]byte(sb.String()))+len([]byte(line))+1 > maxBytes {
			sb.WriteString("  - (further lapsed matters omitted for budget)\n")
			return
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

// lapsedMatterClause is gone: the verdict lives in character.LapseClause, which
// the character window and the in-story assistant read too.

// writeSceneOpenMatters states the arrangements that are still open. It is the
// half of the scene a narrator most often turns into a settled fact by accident:
// an offer neither answered nor withdrawn is still an offer, and this says so
// with the matter's own kind.
func writeSceneOpenMatters(sb *strings.Builder, members []character.CastMember, clockSeconds int64, maxBytes int) {
	lines := make([]string, 0)
	for _, member := range members {
		for _, matter := range member.OpenMatters {
			statement := strings.TrimSpace(matter.Statement)
			if statement == "" {
				continue
			}
			if matter.Letter != nil {
				arrivalState := "before arrival threshold; in transit"
				if clockSeconds >= matter.Letter.EarliestDeliverySeconds {
					arrivalState = "arrival threshold reached; receipt and response remain unrecorded"
				}
				lines = append(lines, fmt.Sprintf("  - [letter; matter id %s] to %s, sent at formal clock %d; cannot arrive before clock %d, current clock %d (%s; open, not settled): %s", matter.ID, member.Profile.DisplayName(), matter.Letter.SentAtSeconds, matter.Letter.EarliestDeliverySeconds, clockSeconds, arrivalState, statement))
				continue
			}
			lines = append(lines, fmt.Sprintf("  - [%s] %s (with %s; open, not settled)", matter.Kind, statement, member.Profile.DisplayName()))
		}
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	header := "- Still open — these are not agreed, not done, and not decided:\n"
	if len([]byte(sb.String()))+len([]byte(header)) > maxBytes {
		return
	}
	sb.WriteString(header)
	for _, line := range lines {
		if len([]byte(sb.String()))+len([]byte(line))+1 > maxBytes {
			sb.WriteString("  - (further open matters omitted for budget)\n")
			return
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}
