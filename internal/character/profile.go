package character

import (
	"fmt"
	"sort"
	"strings"
)

const (
	LifeformHuman       = "human"
	LifeformAnimal      = "animal"
	LifeformSpiritBeast = "spirit_beast"
)

// Profile is the minimal identity a character needs to exist as a persistent
// character rather than a name that happened to appear in one paragraph. An
// important NPC gets one on first formal appearance; a temporary NPC gets one
// when the story first gives them a formal line, so the character window can
// open for them without a second registry.
type Profile struct {
	RadioContact *RadioContact `json:"radio_contact,omitempty"`
	ID           string        `json:"id"`
	StoryID      string        `json:"story_id"`
	// Name is the character's own name. Aliases are the other names the player
	// has actually learned, and KnownAs is the one the player would use. The
	// window shows what the player knows, not what the narrator knows.
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	KnownAs string   `json:"known_as,omitempty"`
	// SourceKey is the author's stable identity for a preset character, and it is
	// empty for everyone the story invented. It is what makes re-appearance reuse
	// the same actor instead of casting a second person with the same name, and
	// what keeps two worlds' same-named characters apart. It is not an actor ID:
	// the actor ID belongs to this story, and one source key maps to one actor per
	// branch.
	SourceKey           string `json:"source_key,omitempty"`
	Lifeform            string `json:"lifeform,omitempty"`
	CultivationMethodID string `json:"cultivation_method_id,omitempty"`
	// Avatar is a reference to an existing image asset. An empty value is a
	// legitimate state and the window renders a placeholder; it is not an error.
	Avatar      string `json:"avatar,omitempty"`
	Role        string `json:"role,omitempty"`
	Description string `json:"description,omitempty"`

	// Voice and motive. Two characters with the same role must still read
	// differently, so style is stored per character rather than derived from
	// the template they were created from.
	Personality []string `json:"personality,omitempty"`
	SpeechStyle string   `json:"speech_style,omitempty"`
	Motives     []string `json:"motives,omitempty"`
	// Gender is what this person is, as decided once and kept. Empty means the
	// author or the story never settled it, and that is a legitimate state: it
	// is not the same as a person whose gender the player has not been told.
	//
	// It is stored rather than derived because it must survive re-opening,
	// re-appearing and a restart without being re-rolled, and because a name,
	// a picture or a role is not evidence of it. It must also never become a
	// personality: the design forbids writing every man one way and every
	// woman another.
	Gender string `json:"gender,omitempty"`
	// Age is an explicitly stated age from a frozen world catalog or a formal
	// story appearance. Zero means the story never established it.
	Age int `json:"age,omitempty"`
	// Attitude is the character's current stance toward the player. It changes
	// with committed exchanges, which is why it is revisioned rather than
	// recomputed.
	Attitude string `json:"attitude,omitempty"`
	// Bounds states what this character will not do: refusals, topics they
	// avoid, and the conditions under which they end a conversation. A
	// character who can never refuse is not a character.
	Bounds []string `json:"bounds,omitempty"`
	// Tags are the short labels a reader would use to find this person again:
	// who they belong to, what they do, where they are from. They are the same
	// kind of locator the author catalog gives a preset person (affiliation,
	// identity, region, craft), and they exist for the same reason — a name
	// alone cannot be searched for a role, and the overview's search reads this
	// list rather than guessing from the introduction's wording.
	//
	// They are what the story actually established, never an inference: a
	// person whose craft nobody has named gets no craft tag, and an empty list
	// is the honest value rather than a gap.
	Tags []string `json:"tags,omitempty"`

	// FirstSeen* pin when the character entered the story, so a window opened
	// much later can still say where they came from.
	FirstSeenWorldTime string `json:"first_seen_world_time,omitempty"`
	FirstSeenTurnID    string `json:"first_seen_turn_id,omitempty"`
	FirstSeenSceneNode string `json:"first_seen_scene_node,omitempty"`

	// Revision increments on every accepted profile change. It participates in
	// the preparation cache key, so an edited character is never served from a
	// context built for the previous revision.
	Revision  int    `json:"revision"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RadioContact is a main-story contact fact, carried by the same profile
// journal as the person's identity. It grants no device or remote presence.
type RadioContact struct {
	DeviceItemID string `json:"device_item_id" jsonschema_description:"主角实际持有的通讯设备物品ID，或重要物品中设备的完整名称；普通物品不能充当设备。"`
	Channel      string `json:"channel" jsonschema_description:"本回合实际交换并确认可用的联络地址或频道，不得仅凭认识对方推定。"`
	Status       string `json:"status" jsonschema:"enum=connected,enum=offline" jsonschema_description:"connected表示已建立且目前可达，offline表示失联、断网、屏蔽或超出有效范围。场景发生此类变化时正式更新。"`
}

func (p Profile) Normalize() Profile {
	p.ID = strings.TrimSpace(p.ID)
	p.StoryID = strings.TrimSpace(p.StoryID)
	p.Name = strings.TrimSpace(p.Name)
	p.KnownAs = strings.TrimSpace(p.KnownAs)
	p.SourceKey = strings.TrimSpace(p.SourceKey)
	p.Lifeform = strings.ToLower(strings.TrimSpace(p.Lifeform))
	p.CultivationMethodID = strings.TrimSpace(p.CultivationMethodID)
	p.Avatar = strings.TrimSpace(p.Avatar)
	p.Role = strings.TrimSpace(p.Role)
	p.Description = strings.TrimSpace(p.Description)
	p.SpeechStyle = strings.TrimSpace(p.SpeechStyle)
	p.Gender = NormalizeGender(p.Gender)
	if p.Age < 0 {
		p.Age = 0
	}
	p.Attitude = strings.TrimSpace(p.Attitude)
	p.FirstSeenWorldTime = strings.TrimSpace(p.FirstSeenWorldTime)
	p.FirstSeenTurnID = strings.TrimSpace(p.FirstSeenTurnID)
	p.FirstSeenSceneNode = strings.TrimSpace(p.FirstSeenSceneNode)
	p.CreatedAt = strings.TrimSpace(p.CreatedAt)
	p.UpdatedAt = strings.TrimSpace(p.UpdatedAt)
	p.Aliases = normalizeIDs(p.Aliases)
	p.Personality = normalizeTextList(p.Personality)
	p.Motives = normalizeTextList(p.Motives)
	p.Bounds = normalizeTextList(p.Bounds)
	p.Tags = normalizeTextList(p.Tags)
	if p.RadioContact != nil {
		contact := *p.RadioContact
		contact.DeviceItemID = strings.TrimSpace(contact.DeviceItemID)
		contact.Channel = strings.TrimSpace(contact.Channel)
		contact.Status = strings.TrimSpace(contact.Status)
		p.RadioContact = &contact
	}
	if p.Revision < 0 {
		p.Revision = 0
	}
	return p
}

func (p Profile) Validate() error {
	p = p.Normalize()
	if contact := p.RadioContact; contact != nil {
		if contact.DeviceItemID == "" || len(contact.DeviceItemID) > 200 || contact.Channel == "" || len(contact.Channel) > 200 || (contact.Status != "connected" && contact.Status != "offline") {
			return fmt.Errorf("character radio contact requires a device, channel and valid status")
		}
	}
	if p.ID == "" {
		return fmt.Errorf("character profile requires an id")
	}
	if p.StoryID == "" {
		return fmt.Errorf("character profile %s requires a story id", p.ID)
	}
	if p.Name == "" {
		return fmt.Errorf("character profile %s requires a name", p.ID)
	}
	if p.Revision < 1 {
		return fmt.Errorf("character profile %s requires a revision of at least 1", p.ID)
	}
	switch p.Lifeform {
	case "", LifeformHuman, LifeformAnimal, LifeformSpiritBeast:
	default:
		return fmt.Errorf("character profile %s has an invalid lifeform: %q", p.ID, p.Lifeform)
	}
	if p.CultivationMethodID != "" && p.Lifeform != LifeformSpiritBeast {
		return fmt.Errorf("character profile %s can only bind a cultivation method as a spirit beast", p.ID)
	}
	return nil
}

// DisplayName is what the character window titles the conversation with. The
// player sees the name they have learned; before that, the name is still the
// only honest label available.
func (p Profile) DisplayName() string {
	p = p.Normalize()
	return firstNonEmpty(p.KnownAs, p.Name, p.ID)
}

func normalizeTextList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// mergeTextList keeps what an earlier revision settled and appends what this
// one adds. Used for labels, which accumulate as the story learns more about a
// person rather than being restated whole each time.
func mergeTextList(existing, added []string) []string {
	if len(existing) == 0 {
		return normalizeTextList(added)
	}
	return normalizeTextList(append(append([]string(nil), existing...), added...))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// MatterKind classifies an unresolved thread. The kinds exist because the game
// must not let one stand in for another: a permission granted is not an action
// carried out, and a promise is not a fact about the world.
type MatterKind string

const (
	// MatterPermission is a right that was actually granted, such as passage
	// through a gate. It records that permission exists, never that the act it
	// permits has happened.
	MatterPermission MatterKind = "permission"
	// MatterAction is an act the player committed to carry out. It stays open
	// until the world shows it done.
	MatterAction MatterKind = "action"
	// MatterPromise is an undertaking by either side.
	MatterPromise MatterKind = "promise"
	// MatterDisclosure is information a character let slip or handed over.
	MatterDisclosure MatterKind = "disclosure"
	// MatterMisunderstanding is a false belief one side holds about the other.
	MatterMisunderstanding MatterKind = "misunderstanding"
	// MatterAttitude is a durable change of stance.
	MatterAttitude MatterKind = "attitude"
)

func (k MatterKind) Valid() bool {
	switch k {
	case MatterPermission, MatterAction, MatterPromise, MatterDisclosure, MatterMisunderstanding, MatterAttitude:
		return true
	default:
		return false
	}
}

type MatterStatus string

const (
	// MatterOpen is unresolved and still constrains behaviour.
	MatterOpen MatterStatus = "open"
	// MatterSettled was carried out or discharged. It is retained so a repeated
	// request is answered from the record instead of being settled twice.
	MatterSettled MatterStatus = "settled"
	// MatterAbandoned lapsed, expired, or was refused.
	MatterAbandoned MatterStatus = "abandoned"
)

func (s MatterStatus) Valid() bool {
	switch s {
	case MatterOpen, MatterSettled, MatterAbandoned:
		return true
	default:
		return false
	}
}

// Matter is one thread between the player and a character: a permission, a
// promise, an unpaid debt, a misunderstanding. It is what lets the window say
// "you already have that permission" and the story carry it without re-rolling,
// and what stops a repeated request from being settled a second time.
type Matter struct {
	ID          string       `json:"id"`
	StoryID     string       `json:"story_id"`
	BranchID    string       `json:"branch_id"`
	CharacterID string       `json:"character_id"`
	Kind        MatterKind   `json:"kind"`
	Status      MatterStatus `json:"status"`
	Statement   string       `json:"statement"`
	// StatementTranslations carries authored display translations, never a second fact.
	StatementTranslations map[string]string `json:"statement_translations,omitempty"`
	// Counterparty is the other party: the player, or another character's ID.
	Counterparty string `json:"counterparty,omitempty"`
	// Deadline is the world time at which this lapses. Empty means open-ended.
	// It is compared against world time, so closing the program or leaving the
	// window open does not age it.
	Deadline string `json:"deadline,omitempty"`
	// DeadlineSeconds is the same limit as an absolute instant on the branch's
	// formal clock, stamped at commit from a deadline that stated an interval.
	// Zero means the label could not be read as an interval, and the thread stays
	// unbound rather than being guessed into expiring.
	DeadlineSeconds int64 `json:"deadline_seconds,omitempty"`
	// EarliestSeconds is a promised earliest revisit time, not a deadline or proof of work.
	EarliestSeconds int64 `json:"earliest_seconds,omitempty"`

	WorldTime string `json:"world_time"`
	TurnID    string `json:"turn_id,omitempty"`
	// ExchangeID is the exchange that established it. Retries replaying that
	// exchange must not create a second matter.
	ExchangeID    string `json:"exchange_id,omitempty"`
	SourceEventID string `json:"source_event_id,omitempty"`
	// SettledBy names the matter or exchange that resolved this one.
	SettledBy string `json:"settled_by,omitempty"`
	// Letter is present only for a formally sent letter. It keeps the exact
	// recipient, body and formal delivery threshold on the same matter record.
	Letter *LetterDelivery `json:"letter,omitempty"`
	// LetterReply is present only when the recipient actually sent a written
	// reply. A resolved letter without it records receipt, not a written answer.
	LetterReply *LetterReply `json:"letter_reply,omitempty"`
	// Cooperation is present only for an agreement verified by the dedicated
	// application path. An ordinary promise is not a work qualification.
	Cooperation *CooperationTerms `json:"cooperation,omitempty"`
	Faction     *FactionTerms     `json:"faction,omitempty"`
}

// LetterDelivery is the transport fact attached to the one open matter for a
// letter. A letter is not delivered by opening a window: a later committed turn
// must reach EarliestDeliverySeconds and settle the matter for this recipient.
type LetterDelivery struct {
	Body                    string `json:"body"`
	SentAtSeconds           int64  `json:"sent_at_seconds"`
	EarliestDeliverySeconds int64  `json:"earliest_delivery_seconds"`
}

// LetterReply is the body of a written reply delivered after the original
// letter reached its formal arrival threshold.
type LetterReply struct {
	Body string `json:"body"`
}

func (m Matter) Normalize() Matter {
	m.ID = strings.TrimSpace(m.ID)
	m.StoryID = strings.TrimSpace(m.StoryID)
	m.BranchID = strings.TrimSpace(m.BranchID)
	m.CharacterID = strings.TrimSpace(m.CharacterID)
	m.Statement = strings.TrimSpace(m.Statement)
	if m.StatementTranslations != nil {
		translations := make(map[string]string, len(m.StatementTranslations))
		for locale, text := range m.StatementTranslations {
			translations[strings.TrimSpace(locale)] = strings.TrimSpace(text)
		}
		m.StatementTranslations = translations
	}
	m.Counterparty = strings.TrimSpace(m.Counterparty)
	m.Deadline = strings.TrimSpace(m.Deadline)
	m.WorldTime = strings.TrimSpace(m.WorldTime)
	m.TurnID = strings.TrimSpace(m.TurnID)
	m.ExchangeID = strings.TrimSpace(m.ExchangeID)
	m.SourceEventID = strings.TrimSpace(m.SourceEventID)
	m.SettledBy = strings.TrimSpace(m.SettledBy)
	if m.Letter != nil {
		letter := *m.Letter
		letter.Body = strings.TrimSpace(letter.Body)
		if letter.SentAtSeconds < 0 {
			letter.SentAtSeconds = 0
		}
		if letter.EarliestDeliverySeconds < 0 {
			letter.EarliestDeliverySeconds = 0
		}
		m.Letter = &letter
	}
	if m.LetterReply != nil {
		reply := *m.LetterReply
		reply.Body = strings.TrimSpace(reply.Body)
		m.LetterReply = &reply
	}
	if m.Faction != nil {
		f := *m.Faction
		m.Faction = &f
	}
	if m.Cooperation != nil {
		cooperation := m.Cooperation.Normalize()
		m.Cooperation = &cooperation
	}
	if m.Status == "" {
		m.Status = MatterOpen
	}
	return m
}

func (m Matter) Validate() error {
	m = m.Normalize()
	if m.ID == "" || m.CharacterID == "" {
		return fmt.Errorf("matter requires an id and a character id")
	}
	if m.StoryID == "" || m.BranchID == "" {
		return fmt.Errorf("matter %s requires a story and branch", m.ID)
	}
	if !m.Kind.Valid() {
		return fmt.Errorf("matter %s has an invalid kind: %q", m.ID, m.Kind)
	}
	if !m.Status.Valid() {
		return fmt.Errorf("matter %s has an invalid status: %q", m.ID, m.Status)
	}
	if m.EarliestSeconds < 0 || m.EarliestSeconds > 0 && m.DeadlineSeconds > 0 && m.EarliestSeconds >= m.DeadlineSeconds {
		return fmt.Errorf("matter %s has invalid maturity bounds", m.ID)
	}
	if m.Statement == "" {
		return fmt.Errorf("matter %s has no statement", m.ID)
	}
	if m.ExchangeID == "" && m.SourceEventID == "" {
		return fmt.Errorf("matter %s has no source exchange or event", m.ID)
	}
	if m.WorldTime == "" {
		return fmt.Errorf("matter %s has no world time", m.ID)
	}
	if m.Letter != nil && (m.Kind != MatterAction || m.Letter.Body == "" || m.Letter.SentAtSeconds < 0 || m.Letter.EarliestDeliverySeconds <= m.Letter.SentAtSeconds) {
		return fmt.Errorf("matter %s has invalid letter delivery facts", m.ID)
	}
	if m.LetterReply != nil && (m.LetterReply.Body == "" || m.Status != MatterSettled || m.SettledBy == "") {
		return fmt.Errorf("matter %s has invalid written letter reply facts", m.ID)
	}
	if m.Faction != nil {
		if err := m.Faction.Validate(); err != nil {
			return err
		}
	}
	if m.Cooperation != nil {
		if err := m.Cooperation.Validate(); err != nil {
			return fmt.Errorf("matter %s: %w", m.ID, err)
		}
	}
	// A resolved matter is attributed by its own source event and world time.
	// SettledBy is a link to an earlier matter this one resolves, not a
	// requirement: a permission granted, or an act the world itself carried out,
	// ends a thread without resolving a different one.
	return nil
}

// OpenMatters returns the unresolved threads for one character, stable-ordered.
// The story and the window both read this, so an arrangement the player made in
// the window is visible to the next story turn.
func OpenMatters(characterID string, matters []Matter) []Matter {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]Matter, 0, len(matters))
	for _, matter := range matters {
		matter = matter.Normalize()
		if matter.CharacterID != characterID || matter.Status != MatterOpen {
			continue
		}
		out = append(out, matter)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && matterLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ResolvedMatters returns the threads that are over: carried out, refused, or
// abandoned. Newest first.
//
// They are not open work, so they are not listed as outstanding, but they are
// not nothing either. 自制-叙事演绎 §5.5 states 后果必须有消费者 and 自制-游玩循环与选择后果
// §5 gives the example the narrator needs: NPC后续提及确实答应的事、在相关事件中认可已经履行
// 的帮助. A person who cannot see that a thread was carried out has no way to
// acknowledge the help, and asks for work already done.
//
// A lapsed thread is not here: its deadline passed but it is still open, and it
// is reported separately as a thread that no longer binds.
func ResolvedMatters(characterID string, matters []Matter) []Matter {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]Matter, 0, len(matters))
	for _, matter := range matters {
		matter = matter.Normalize()
		if matter.CharacterID != characterID || matter.Status == MatterOpen {
			continue
		}
		out = append(out, matter)
	}
	// Newest first: the help just given is the one the next scene can refer to.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].WorldTime != out[j].WorldTime {
			return out[i].WorldTime > out[j].WorldTime
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// Lapsed reports whether the matter's deadline has passed at a given world
// time. A lapsed matter is not erased: it happened, and the character remembers
// being invited or having promised. What ends is its force — neither side may
// carry it out as though it were still live, which is what stops an invitation
// sent months ago from being executed today.
//
// World time advances only through committed story turns, so leaving the window
// open or closing the program does not age it.
func (m Matter) Lapsed(currentWorldTime string) bool {
	return m.LapsedAt(currentWorldTime, 0)
}

// LapsedAt is Lapsed with the branch's formal instant supplied, so a thread
// stamped against the clock is decided by arithmetic rather than by its label.
//
// nowSeconds of zero means the caller has no clock reading — either the branch
// has never spent formal time, or this record predates the clock — and then the
// label is all there is. It falls back to the text comparison rather than
// reporting "not lapsed": the stamp was derived from that same label, so
// declining to decide would make a thread that the label already orders as past
// silently live again. Nothing was migrated, so a record with no stamp behaves
// exactly as it did.
func (m Matter) LapsedAt(currentWorldTime string, nowSeconds int64) bool {
	m = m.Normalize()
	if m.DeadlineSeconds > 0 && nowSeconds > 0 {
		return nowSeconds > m.DeadlineSeconds
	}
	currentWorldTime = strings.TrimSpace(currentWorldTime)
	if m.Deadline == "" || currentWorldTime == "" {
		return false
	}
	return compareWorldTime(currentWorldTime, m.Deadline) > 0
}

// LiveMatters returns the open threads that are still in force at a world time,
// and the ones whose deadline has passed. Both are unresolved; the split is
// about whether they still bind. The story and the window read this, so an
// expired arrangement is presented as expired rather than as still standing.
func LiveMatters(characterID, currentWorldTime string, matters []Matter) (live, lapsed []Matter) {
	return LiveMattersAt(characterID, currentWorldTime, 0, matters)
}

// LiveMattersAt is LiveMatters with the branch's formal instant supplied, so a
// thread whose deadline was stamped against the clock is decided by that
// instant. A zero reading keeps the text comparison, which is what an unbound
// or pre-clock record gets.
func LiveMattersAt(characterID, currentWorldTime string, nowSeconds int64, matters []Matter) (live, lapsed []Matter) {
	for _, matter := range OpenMatters(characterID, matters) {
		if matter.LapsedAt(currentWorldTime, nowSeconds) {
			lapsed = append(lapsed, matter)
			continue
		}
		live = append(live, matter)
	}
	return live, lapsed
}

// LapseClause states what a lapsed thread no longer does.
//
// A permission is a right rather than a task, and the two lapse differently.
// Nothing was "left undone" when a granted right runs out — the term simply
// ended and the right is gone. Saying a permission was not carried out would
// leave a reader free to think it still stands for as long as the permitted act
// has not happened, which is exactly the reading 自制-时间寿元与长时世界推进 forbids
// when it says 玩家失去权限后不得继续调资源.
//
// It lives here rather than in either renderer because the scene block, the
// character window and the in-story assistant all describe the same record, and
// three phrasings of one verdict is three chances to disagree about it.
func LapseClause(kind MatterKind) string {
	if kind == MatterPermission {
		return "its term ended and it no longer authorises anything"
	}
	return "the hour passed and it was not carried out"
}

func matterLess(a, b Matter) bool {
	if a.WorldTime != b.WorldTime {
		return a.WorldTime < b.WorldTime
	}
	return a.ID < b.ID
}

// MatterByID finds a matter so a repeat of an already-settled action can be
// answered from the record rather than resolved again.
func MatterByID(matters []Matter, id string) (Matter, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Matter{}, false
	}
	for _, matter := range matters {
		if matter.Normalize().ID == id {
			return matter.Normalize(), true
		}
	}
	return Matter{}, false
}
