package character

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Scene is where a conversation is happening right now. It is the same scene
// the main story is in, which is what lets the window and the story share
// state: an answer given in the window belongs to the scene the story is
// reading.
type Scene struct {
	// NodeID identifies the scene node the story is currently at.
	NodeID string `json:"node_id"`
	// Title is a short player-facing label for the current scene.
	Title string `json:"title,omitempty"`
	// Location is where it is happening, in the story's own words.
	Location string `json:"location,omitempty"`
	// WorldTime is the current in-fiction time.
	WorldTime string `json:"world_time"`
	// TurnID is the latest committed turn, the formal commit position the
	// window's exchanges are attached to.
	TurnID string `json:"turn_id,omitempty"`
	// Present lists the characters who are in the scene. It decides who may
	// witness a line spoken here.
	Present []string `json:"present,omitempty"`
}

func (s Scene) Normalize() Scene {
	s.NodeID = strings.TrimSpace(s.NodeID)
	s.Title = strings.TrimSpace(s.Title)
	s.Location = strings.TrimSpace(s.Location)
	s.WorldTime = strings.TrimSpace(s.WorldTime)
	s.TurnID = strings.TrimSpace(s.TurnID)
	s.Present = normalizeIDs(s.Present)
	return s
}

// AudienceFor returns the perceivers of an exchange on a channel in this scene.
// This is the single place the audience rule lives, so the story and the window
// cannot disagree about who heard what.
func (s Scene) AudienceFor(channel Channel, characterID string) []string {
	s = s.Normalize()
	characterID = strings.TrimSpace(characterID)
	switch channel {
	case ChannelPrivate:
		// A private conversation is private because of where it happened, not
		// because it was stored in a separate place.
		return normalizeIDs([]string{characterID})
	case ChannelMessage:
		// An incoming message reaches the player alone unless the narrative
		// places someone else at the reading.
		return normalizeIDs([]string{characterID})
	case ChannelScene:
		present := append([]string{}, s.Present...)
		present = append(present, characterID)
		return normalizeIDs(present)
	default:
		return normalizeIDs([]string{characterID})
	}
}

// IdentityRevision is the binding version a prepared context was built against.
// A context built for one revision must never be served for another: that is
// how a character's edited personality silently keeps answering in the old
// voice.
type IdentityRevision struct {
	// PackID and WorldID bind the character to the pack and world that created
	// it, so a same-named character in another world never enters this context.
	PackID  string `json:"pack_id,omitempty"`
	WorldID string `json:"world_id,omitempty"`
	// BranchID scopes memory to one branch; two branches of one story must not
	// share conversations.
	BranchID string `json:"branch_id"`
	// ProfileRevision is the character's own profile revision.
	ProfileRevision int `json:"profile_revision"`
	// FormalCommitID is the latest canonical commit position included.
	FormalCommitID string `json:"formal_commit_id,omitempty"`
	// LoreRevision is the lore revision the context was built against.
	LoreRevision string `json:"lore_revision,omitempty"`
}

// CacheKey is the preparation cache identity. Every input that could change the
// prepared material is in it, so a stale context is a cache miss rather than a
// wrong answer.
func (r IdentityRevision) CacheKey(characterID, storyID string) string {
	return strings.Join([]string{
		"character-context",
		"story=" + strings.TrimSpace(storyID),
		"character=" + strings.TrimSpace(characterID),
		"pack=" + strings.TrimSpace(r.PackID),
		"world=" + strings.TrimSpace(r.WorldID),
		"branch=" + strings.TrimSpace(r.BranchID),
		"profile=" + fmt.Sprint(r.ProfileRevision),
		"commit=" + strings.TrimSpace(r.FormalCommitID),
		"lore=" + strings.TrimSpace(r.LoreRevision),
	}, "|")
}

// PrepareInput is everything the preparation service needs. It is data, not a
// live store handle, so the service stays pure and testable and cannot reach
// around the caller's scoping.
type PrepareInput struct {
	StoryID string
	// Revision binds the prepared context to a pack, world, branch, profile
	// revision and commit position.
	Revision IdentityRevision
	Profile  Profile
	Scene    Scene

	// Exchanges is the recent conversation, oldest first, already scoped to
	// this story and branch by the caller.
	Exchanges []Exchange
	// Knowledge is this character's records, already scoped to story and branch.
	Knowledge []KnowledgeRecord
	// Matters is this character's open and settled threads.
	Matters []Matter
	// Lore is the world knowledge this character could plausibly hold. The
	// caller resolves it through the story's lore scope, so another world's
	// resident lore cannot appear here.
	Lore []string
	// Affinity is this character's current feeling toward the protagonist and
	// the formal relationship that stands, if any. It is supplied rather than
	// derived so the window, the main story and an incoming message all speak
	// from one reading.
	Affinity     int
	AffinityBand AffinityBand
	Relationship *Relationship
	// RecentAffinities is what this character's feeling was last moved by. It is
	// shown so a later turn does not count the same fact again and so the
	// character can refer to what actually changed their mind.
	RecentAffinities []AffinityChange

	// Question is the player's current line. It is the retrieval condition for
	// the recall pass: it is what decides which of the character's older records
	// are read back when the standing memory block cannot carry them all. Without
	// it the character has no route to anything outside that block.
	Question string

	// Bounds. Zero values fall back to the defaults below.
	MaxRecentExchanges int
	MaxKnowledge       int
	MaxLoreEntries     int
	// MaxRecalledItems bounds the recall pass.
	MaxRecalledItems int
	// MaxBytes bounds the whole prepared block.
	MaxBytes int

	// ClockSeconds is the branch's formal instant, so a thread whose deadline
	// was bound to the clock is split into live and lapsed by that number. Zero
	// means the caller has no reading, and a bound thread then stays live rather
	// than being guessed into having expired.
	ClockSeconds int64
}

const (
	DefaultMaxRecentExchanges = 12
	DefaultMaxKnowledge       = 24
	DefaultMaxLoreEntries     = 12
	// DefaultMaxPreparedBytes bounds one character's prepared context. It is
	// far below the story's own context budget because a character needs enough
	// to be themselves, not the whole world.
	DefaultMaxPreparedBytes = 24 * 1024
)

// Prepared is the six categories the design requires, in the order the model
// reads them. Keeping them as separate fields rather than one blob is what lets
// the ledger record which material was actually supplied, and lets a caller
// assert that a character was not handed director planning or another
// character's private conversation.
type Prepared struct {
	CharacterID string `json:"character_id"`

	// 1. Identity and self-knowledge.
	Identity string `json:"identity"`
	// 2. Personality and style.
	Voice string `json:"voice"`
	// 3. Current circumstances.
	Circumstances string `json:"circumstances"`
	// 4. Current important interactions and unfinished matters.
	Interactions string `json:"interactions"`
	// 5. Recent necessary dialogue.
	RecentDialogue string `json:"recent_dialogue"`
	// 6. Relevant long-term memory and graspable common knowledge.
	Memory string `json:"memory"`
	// 7. Recalled material: the character's own older records that the current
	// question reached and the bounded memory block above did not carry. It is a
	// separate category because it is retrieved per question rather than
	// standing, and because it is the material the budget must not silently drop.
	Recall string `json:"recall"`
	// RecallDiagnostics records what the recall pass considered and selected, so
	// a thin recall is visible as a thin recall rather than read as the character
	// having no past.
	RecallDiagnostics Recall `json:"recall_diagnostics"`

	CacheKey string `json:"cache_key"`
	// Truncated lists the categories that were cut to fit the budget, so a
	// bounded context is visible rather than silent.
	Truncated []string `json:"truncated,omitempty"`
}

// Markdown renders the prepared material for the model, category by category.
func (p Prepared) Markdown() string {
	sections := []struct {
		title string
		body  string
	}{
		{"Identity and self-knowledge", p.Identity},
		{"Personality and voice", p.Voice},
		{"Current circumstances", p.Circumstances},
		{"Important interactions and unfinished matters", p.Interactions},
		{"Recent dialogue", p.RecentDialogue},
		{"Long-term memory and common knowledge", p.Memory},
		{"Earlier memories recalled for this question", p.Recall},
	}
	var sb strings.Builder
	for _, section := range sections {
		body := strings.TrimSpace(section.body)
		if body == "" {
			continue
		}
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", section.title, body)
	}
	return strings.TrimSpace(sb.String())
}

// Prepare builds one character's turn context. It is the shared preparation
// used by main-story character speech, the character window and incoming
// messages, so all three speak from the same identity and the same memory.
func Prepare(input PrepareInput) Prepared {
	profile := input.Profile.Normalize()
	scene := input.Scene.Normalize()
	if input.MaxRecentExchanges <= 0 {
		input.MaxRecentExchanges = DefaultMaxRecentExchanges
	}
	if input.MaxKnowledge <= 0 {
		input.MaxKnowledge = DefaultMaxKnowledge
	}
	if input.MaxLoreEntries <= 0 {
		input.MaxLoreEntries = DefaultMaxLoreEntries
	}
	if input.MaxRecalledItems <= 0 {
		input.MaxRecalledItems = DefaultMaxRecalledItems
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = DefaultMaxPreparedBytes
	}

	// Recall runs against the character's own projections with the player's
	// current question as the retrieval condition. It is what gives the model a
	// route to material the bounded memory block could not carry.
	recall := BuildRecall(RecallRequest{
		CharacterID: profile.ID,
		Question:    input.Question,
		Scene:       scene,
		Knowledge:   input.Knowledge,
		Exchanges:   input.Exchanges,
		Matters:     input.Matters,
		Limit:       input.MaxRecalledItems,
	})

	prepared := Prepared{
		CharacterID:       profile.ID,
		Identity:          prepareIdentity(profile),
		Voice:             prepareVoice(profile),
		Circumstances:     prepareCircumstances(profile, scene, input.Matters, input.Affinity, input.AffinityBand, input.Relationship, input.RecentAffinities, input.ClockSeconds),
		Interactions:      prepareInteractions(profile, input.Matters, scene.WorldTime, input.ClockSeconds),
		RecentDialogue:    prepareRecentDialogue(profile, input.Exchanges, input.MaxRecentExchanges),
		Memory:            prepareMemory(profile, input.Knowledge, input.Lore, input.MaxKnowledge, input.MaxLoreEntries),
		Recall:            RenderRecall(recall),
		RecallDiagnostics: recall,
		CacheKey:          input.Revision.CacheKey(profile.ID, input.StoryID),
	}
	prepared.Truncated = applyPreparedBudget(&prepared, input.MaxBytes)
	return prepared
}

func prepareIdentity(profile Profile) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- You are %s.\n", profile.DisplayName())
	if profile.Name != "" && profile.Name != profile.DisplayName() {
		fmt.Fprintf(&sb, "- Your own name is %s. The player knows you as %s.\n", profile.Name, profile.DisplayName())
	}
	if len(profile.Aliases) > 0 {
		fmt.Fprintf(&sb, "- You are also known as: %s.\n", strings.Join(profile.Aliases, ", "))
	}
	if profile.Role != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", profile.Role)
	}
	if profile.Age > 0 {
		fmt.Fprintf(&sb, "- Age: %d (established fact)\n", profile.Age)
	}
	if profile.Description != "" {
		fmt.Fprintf(&sb, "- %s\n", profile.Description)
	}
	sb.WriteString("- You know only what appears in this context. Do not invent shared history, and do not claim to know what you were never told.\n")
	sb.WriteString("- You may hold your own secrets and may lie about them. You must not state planning, secrets you do not hold, or private conversations you were not part of.\n")
	sb.WriteString("\n" + SocialIdentityContext(profile) + "\n")
	return strings.TrimSpace(sb.String())
}

func prepareVoice(profile Profile) string {
	var sb strings.Builder
	if profile.Gender != "" {
		// Stated so a person the player has already met keeps the same answer
		// across every later conversation. It says who they are and nothing
		// about how they should behave: the design forbids writing every man
		// one way and every woman another, so the line is followed by that
		// warning rather than left to be read as a character note.
		fmt.Fprintf(&sb, "- Gender: %s. It is not a personality: speak from the traits below, not from a habit you think this gender has.\n", profile.Gender)
	}
	if len(profile.Personality) > 0 {
		fmt.Fprintf(&sb, "- Character: %s\n", strings.Join(profile.Personality, "; "))
	}
	if profile.SpeechStyle != "" {
		fmt.Fprintf(&sb, "- Speech: %s\n", profile.SpeechStyle)
	}
	if len(profile.Motives) > 0 {
		fmt.Fprintf(&sb, "- What you want: %s\n", strings.Join(profile.Motives, "; "))
	}
	if profile.Attitude != "" {
		fmt.Fprintf(&sb, "- Your current stance toward the player: %s\n", profile.Attitude)
	}
	if len(profile.Bounds) > 0 {
		fmt.Fprintf(&sb, "- You will not: %s\n", strings.Join(profile.Bounds, "; "))
		sb.WriteString("- You may refuse, deflect, or end the conversation. Doing so in character is correct; agreeing to everything is not.\n")
	}
	if sb.Len() == 0 {
		return ""
	}
	sb.WriteString("- Speak as this person. Do not narrate for the player, and do not decide what the player does or says.\n")
	return strings.TrimSpace(sb.String())
}

func prepareCircumstances(profile Profile, scene Scene, matters []Matter, affinity int, band AffinityBand, relationship *Relationship, recent []AffinityChange, clockSeconds int64) string {
	var sb strings.Builder
	if scene.Location != "" {
		fmt.Fprintf(&sb, "- You are at %s.\n", scene.Location)
	}
	if scene.Title != "" {
		fmt.Fprintf(&sb, "- Current scene: %s\n", scene.Title)
	}
	if scene.WorldTime != "" {
		fmt.Fprintf(&sb, "- It is %s.\n", scene.WorldTime)
	}
	if profile.Attitude != "" {
		fmt.Fprintf(&sb, "- You currently regard the player with %s.\n", profile.Attitude)
	}
	// The number is a reference for the model, not a script. It is stated with
	// its band so the model knows what the value means without inventing a
	// meaning for it, and it is stated as this person's own feeling rather than
	// as a rule the player has earned.
	if !band.Valid() {
		band = BandFor(affinity)
	}
	fmt.Fprintf(&sb, "- You feel toward the player: %d (%s). A reference, not a command.\n", affinity, affinityBandGuidance(band))
	if relationship != nil && relationship.Statement != "" {
		fmt.Fprintf(&sb, "- What stands between you: %s. %s\n", relationship.Kind, oneLine(relationship.Statement))
	} else if relationship != nil {
		fmt.Fprintf(&sb, "- What stands between you: %s.\n", relationship.Kind)
	}
	if len(recent) > 0 {
		sb.WriteString("- Already counted; do not be moved by these again:\n")
		for _, change := range recent {
			fmt.Fprintf(&sb, "  - %s (%+d): %s\n", change.Kind, change.Delta, oneLine(change.Basis))
		}
	}
	if profile.FirstSeenWorldTime != "" {
		fmt.Fprintf(&sb, "- You first met the player at %s.\n", profile.FirstSeenWorldTime)
	}
	live, lapsed := LiveMattersAt(profile.ID, scene.WorldTime, clockSeconds, matters)
	if len(live) == 0 && len(lapsed) == 0 && sb.Len() == 0 {
		return ""
	}
	if len(live) > 0 {
		sb.WriteString("- Unfinished business with the player is listed under interactions below; treat it as still in force.\n")
	}
	if len(lapsed) > 0 {
		sb.WriteString("- Some business with the player has lapsed; it is listed under interactions below as no longer in force.\n")
	}
	return strings.TrimSpace(sb.String())
}

// affinityBandGuidance says what a band means in behavioural terms. It is
// deliberately about what this person is willing to consider rather than about
// what they must do: the band is the same for every character, but the profile
// and the situation decide what it comes to for this one.
func affinityBandGuidance(band AffinityBand) string {
	switch band {
	case AffinityHostile:
		return "hostile. You want nothing to do with them and may work against them, though whether you act still depends on your goals, your strength and your situation"
	case AffinityWary:
		return "wary and resentful. You will deal with them when you must, on explicit terms, and you owe them no favours"
	case AffinityNeutral:
		return "neutral, barely acquainted. You will talk, trade fairly and answer what you know, and nothing more"
	case AffinityFamiliar:
		return "familiar, with a good impression of them. You will talk longer, offer ordinary advice and accept a low-risk invitation"
	case AffinityFriendly:
		return "friendly and trusting. You will share some of your own past, make a reasonable introduction and work with them on something concrete"
	case AffinityClose:
		return "close. You will reach out first, care about their safety and take on some personal cost or risk for them; an ease that suits the two of you may show"
	default:
		return "deeply attached. You will stand by them, support them and make commitments; if the feeling is genuinely mutual this may become romance, and if it is not, it is a deep friendship"
	}
}

func prepareInteractions(profile Profile, matters []Matter, currentWorldTime string, clockSeconds int64) string {
	live, lapsed := LiveMattersAt(profile.ID, currentWorldTime, clockSeconds, matters)
	var settled []Matter
	for _, matter := range matters {
		matter = matter.Normalize()
		if matter.CharacterID == profile.ID && matter.Status != MatterOpen {
			settled = append(settled, matter)
		}
	}
	if len(live) == 0 && len(lapsed) == 0 && len(settled) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(live) > 0 {
		sb.WriteString("Unresolved:\n")
		for _, matter := range live {
			// The id is shown so a later turn can name the thread it settles.
			fmt.Fprintf(&sb, "- [%s] %s (id: %s)\n", matter.Kind, matter.Statement, matter.ID)
			if matter.EarliestSeconds > 0 {
				if clockSeconds < matter.EarliestSeconds {
					fmt.Fprintf(&sb, "  - 最早复查尚余 %d 秒游戏时间；不得提前宣布履约。\n", matter.EarliestSeconds-clockSeconds)
				} else {
					sb.WriteString("  - 已到最早复查时刻；仍须实际回应与结果，不得按时间自动判定完成。\n")
				}
			}
			if matter.Deadline != "" {
				fmt.Fprintf(&sb, "  - Lapses at %s.\n", matter.Deadline)
			}
			sb.WriteString("  - " + matterGuidance(matter) + "\n")
		}
	}
	// An expired arrangement is kept rather than dropped, because it is part of
	// what this character knows happened. What changes is that it no longer
	// authorizes anything: the instruction is explicit so the model cannot carry
	// out an invitation that has run out.
	if len(lapsed) > 0 {
		sb.WriteString("Lapsed — these were real, and you remember them, but they are no longer in force. Do not carry one out, and do not treat it as still offered:\n")
		for _, matter := range lapsed {
			fmt.Fprintf(&sb, "- [%s] %s (lapsed at %s)\n", matter.Kind, matter.Statement, matter.Deadline)
		}
	}
	if len(settled) > 0 {
		sb.WriteString("Already resolved — do not treat these as new, and do not settle them again:\n")
		sort.Slice(settled, func(i, j int) bool { return settled[i].ID < settled[j].ID })
		for _, matter := range settled {
			fmt.Fprintf(&sb, "- [%s/%s] %s\n", matter.Kind, matter.Status, matter.Statement)
		}
	}
	return strings.TrimSpace(sb.String())
}

// matterGuidance states the consequence of a matter's kind. The distinction
// between a permission and the act it permits is the one that decides whether
// the story may announce success, so it is spelled out rather than left to the
// model to infer.
func matterGuidance(matter Matter) string {
	switch matter.Kind {
	case MatterPermission:
		return "This is a permission that was granted. It does not mean the permitted act has happened. Do not describe it as already done."
	case MatterAction:
		return "This is an act the player committed to. It stays open until the world shows it carried out."
	case MatterPromise:
		return "This is an undertaking. Honour it, and do not treat it as a fact about the world."
	case MatterDisclosure:
		return "This information was disclosed. You may act on it; you must not reveal it to anyone outside the participants."
	case MatterMisunderstanding:
		return "This is a false belief. It persists until something actually corrects it."
	case MatterAttitude:
		return "This is a durable change in how you regard the player. Keep it consistent."
	default:
		return ""
	}
}

func prepareRecentDialogue(profile Profile, exchanges []Exchange, limit int) string {
	scoped := make([]Exchange, 0, len(exchanges))
	for _, exchange := range exchanges {
		exchange = exchange.Normalize()
		// The character may read their own exchanges and any scene exchange they
		// witnessed. A private exchange between the player and someone else is
		// not theirs to remember.
		if !exchangeReaches(exchange, profile.ID) {
			continue
		}
		scoped = append(scoped, exchange)
	}
	if len(scoped) == 0 {
		return ""
	}
	// Most recent last so the model reads it as a conversation, trimmed from
	// the front: the newest lines are the ones that matter.
	if limit > 0 && len(scoped) > limit {
		scoped = scoped[len(scoped)-limit:]
	}
	var sb strings.Builder
	for _, exchange := range scoped {
		who := profile.DisplayName()
		if exchange.Channel == ChannelGroup {
			// A group line's author is not necessarily this character. Labelling
			// every group line with the reader's own name would let them remember
			// someone else's words as their own.
			if exchange.Speaker == SpeakerPlayer {
				who = "Player"
			} else if exchange.AuthorID != "" && exchange.AuthorID != profile.ID {
				who = exchange.AuthorID
			}
		} else if exchange.Speaker == SpeakerPlayer {
			who = "Player"
		}
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", exchange.Channel, who, oneLine(exchange.Text))
	}
	return strings.TrimSpace(sb.String())
}

// exchangeReaches reports whether a character may remember an exchange. It is
// the same audience rule the knowledge records use, applied to dialogue.
//
// The audience is what was captured at commit, not who belongs now. That is the
// whole answer for a group line: a member who joined after it was spoken is not
// in its audience and cannot recall it, and a member who left afterwards still
// can, because they were there when it happened.
func exchangeReaches(exchange Exchange, characterID string) bool {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return false
	}
	if exchange.Channel == ChannelGroup {
		return ContainsID(exchange.AudienceMembers(), characterID)
	}
	if exchange.CharacterID == characterID {
		return true
	}
	if exchange.Channel != ChannelScene {
		return false
	}
	return ContainsID(exchange.AudienceMembers(), characterID)
}

func prepareMemory(profile Profile, records []KnowledgeRecord, lore []string, knowledgeLimit, loreLimit int) string {
	view := BuildKnowledgeView(profile.ID, records, knowledgeLimit)
	var sb strings.Builder
	if len(view.Records) > 0 {
		sb.WriteString("What you know, and how you know it:\n")
		for _, record := range view.Records {
			// The id is shown so a later record can name the belief it replaces.
			fmt.Fprintf(&sb, "- (%s, %s, id: %s) %s\n", record.Nature, record.Nature.Certainty(), record.ID, oneLine(record.Statement))
			if record.LearnedFrom != "" {
				fmt.Fprintf(&sb, "  - Learned from %s at %s.\n", record.LearnedFrom, record.WorldTime)
			}
		}
		sb.WriteString("Assert experienced and secret knowledge plainly. Mark hearsay as something you heard. State conjecture as your own guess. Never present a guess as established fact.\n")
	}
	if len(lore) > 0 {
		if loreLimit > 0 && len(lore) > loreLimit {
			lore = lore[:loreLimit]
		}
		sb.WriteString("\nCommon knowledge you could plausibly hold about this world:\n")
		for _, entry := range lore {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			fmt.Fprintf(&sb, "- %s\n", oneLine(entry))
		}
	}
	return strings.TrimSpace(sb.String())
}

// applyPreparedBudget trims categories from the least to the most load-bearing
// until the block fits, recording what was cut.
//
// The order matters, and it is not the old one. Dropping the standing memory
// block first is only safe because the recall pass has already read back
// whatever in it bore on the current question; what is dropped is the part
// nothing asked about. Identity is never dropped, and the material the design
// names as load-bearing — the current circumstances, the key promises, and the
// records the current question reached — is trimmed from the tail rather than
// removed outright, so a tight budget shortens the character's answer instead
// of making them forget they ever had one.
func applyPreparedBudget(prepared *Prepared, maxBytes int) []string {
	if maxBytes <= 0 {
		return nil
	}
	total := func() int { return len([]byte(prepared.Markdown())) }
	if total() <= maxBytes {
		return nil
	}
	truncated := []string{}
	note := func(name string) {
		for _, existing := range truncated {
			if existing == name {
				return
			}
		}
		truncated = append(truncated, name)
	}

	// 1. The standing memory block and the recent dialogue go first. Neither is
	//    lost from the character's past: the question-relevant part of memory was
	//    read back by the recall pass, and the conversation itself is still on the
	//    journal and still readable.
	for _, category := range []struct {
		name string
		ptr  *string
	}{
		{"memory", &prepared.Memory},
		{"recent_dialogue", &prepared.RecentDialogue},
	} {
		if total() <= maxBytes {
			break
		}
		if strings.TrimSpace(*category.ptr) == "" {
			continue
		}
		*category.ptr = ""
		note(category.name)
	}

	// 2. Then the voice, trimmed from its tail. Losing a mannerism changes how
	//    the character sounds; losing what they know changes whether they can
	//    answer at all, so the voice goes before the recalled material does.
	if total() > maxBytes && strings.TrimSpace(prepared.Voice) != "" {
		trimCategoryTail(prepared, &prepared.Voice, maxBytes)
		note("voice")
	}

	// 3. Recalled material is trimmed from its weakest end rather than dropped.
	//    Losing the least relevant memory is not the same as losing the answer to
	//    the question, and the rendered block says more exist so the model does
	//    not read the gap as something that never happened.
	hadRecall := len(prepared.RecallDiagnostics.Items) > 0
	for total() > maxBytes && len(prepared.RecallDiagnostics.Items) > 0 {
		items := prepared.RecallDiagnostics.Items
		prepared.RecallDiagnostics.Items = items[:len(items)-1]
		prepared.RecallDiagnostics.Truncated = true
		prepared.RecallDiagnostics.Selected = len(prepared.RecallDiagnostics.Items)
		prepared.Recall = RenderRecall(prepared.RecallDiagnostics)
		note("recall")
	}
	if len(prepared.RecallDiagnostics.Items) == 0 {
		prepared.RecallDiagnostics.Selected = 0
		// A budget that removed every recalled item is not an empty past. The
		// flag keeps the block honest so the model hedges instead of denying.
		prepared.RecallDiagnostics.Dropped = hadRecall
		prepared.Recall = RenderRecall(prepared.RecallDiagnostics)
	}

	// 4. Unfinished business, then circumstances: the current situation and the
	//    key promises are the last material to be shortened, and even then they
	//    are trimmed from the tail so their most important lines survive longest.
	for _, category := range []struct {
		name string
		ptr  *string
	}{
		{"interactions", &prepared.Interactions},
		{"circumstances", &prepared.Circumstances},
	} {
		if total() <= maxBytes {
			break
		}
		if strings.TrimSpace(*category.ptr) == "" {
			continue
		}
		trimCategoryTail(prepared, category.ptr, maxBytes)
		note(category.name)
	}
	return truncated
}

// trimCategoryTail removes trailing lines from one category until the whole
// block fits or the category is empty. It is the gentler cut used for material
// that must not disappear outright.
func trimCategoryTail(prepared *Prepared, ptr *string, maxBytes int) {
	lines := strings.Split(strings.TrimSpace(*ptr), "\n")
	for len(lines) > 0 {
		lines = lines[:len(lines)-1]
		*ptr = strings.TrimSpace(strings.Join(lines, "\n"))
		if len([]byte(prepared.Markdown())) <= maxBytes {
			return
		}
	}
}

func oneLine(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if utf8.RuneCountInString(value) > 500 {
		runes := []rune(value)
		value = string(runes[:500]) + "…"
	}
	return value
}
