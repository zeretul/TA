package character

import (
	"fmt"
	"sort"
	"strings"
)

// State is the whole character-side record of one story branch.
//
// It is a rebuildable projection: every field is derived from committed
// exchanges, knowledge records, matters and messages, so it can be recomputed
// from the canonical journal rather than being authoritative itself. That is
// what keeps a single save: the journal owns the truth, and this is a view of
// it keyed by story, branch and character.
type State struct {
	FactionMemberships []FactionMembership `json:"faction_memberships,omitempty"`
	// Version gates the shape so a future change can migrate rather than guess.
	Version int `json:"version"`

	Profiles  []Profile         `json:"profiles,omitempty"`
	Exchanges []Exchange        `json:"exchanges,omitempty"`
	Knowledge []KnowledgeRecord `json:"knowledge,omitempty"`
	Matters   []Matter          `json:"matters,omitempty"`
	Messages  []Message         `json:"messages,omitempty"`
	// Groups are the conversation spaces this branch holds. Their membership is
	// part of the same projection as the exchanges, so a group and its history
	// are restored together rather than from a second store.
	Groups []Group `json:"groups,omitempty"`
	// Affinities is every settled change to how a character feels about the
	// protagonist, oldest first. The current value is the last change for that
	// character; no change means 0, which is a stranger who has not decided.
	Affinities []AffinityChange `json:"affinities,omitempty"`
	// Relationships is the formal relationship history. The last record for a
	// character is the one that currently stands.
	Relationships []Relationship `json:"relationships,omitempty"`
	// IntimacyActivities is the record of who was invited, what each of them
	// actually answered, and which roster the player confirmed. The exchanges
	// themselves are ordinary group exchanges, not a separate conversation
	// store, so an activity is a fact about a conversation rather than a second
	// one.
	IntimacyActivities []IntimacyActivity `json:"intimacy_activities,omitempty"`

	// Cursor is the last canonical commit position this projection has consumed.
	// Preparation and rebuild both start here, so an interrupted rebuild resumes
	// instead of reprocessing the journal.
	Cursor string `json:"cursor,omitempty"`
	// Revision increments on every accepted write. It participates in the
	// preparation cache key.
	Revision int `json:"revision"`
}

// StateVersion is the current shape of the character projection.
const StateVersion = 1

func (s State) Normalize() State {
	if s.Version <= 0 {
		s.Version = StateVersion
	}
	s.Cursor = trimSpace(s.Cursor)
	s.FactionMemberships = append([]FactionMembership(nil), s.FactionMemberships...)
	profiles := make([]Profile, 0, len(s.Profiles))
	seenProfiles := map[string]bool{}
	for _, profile := range s.Profiles {
		profile = profile.Normalize()
		if profile.ID == "" || seenProfiles[profile.ID] {
			continue
		}
		seenProfiles[profile.ID] = true
		profiles = append(profiles, profile)
	}
	s.Profiles = profiles

	exchanges := make([]Exchange, 0, len(s.Exchanges))
	seenExchanges := map[string]bool{}
	for _, exchange := range s.Exchanges {
		exchange = exchange.Normalize()
		if exchange.ID == "" || seenExchanges[exchange.ID] {
			continue
		}
		seenExchanges[exchange.ID] = true
		exchanges = append(exchanges, exchange)
	}
	s.Exchanges = exchanges

	knowledge := make([]KnowledgeRecord, 0, len(s.Knowledge))
	seenKnowledge := map[string]bool{}
	for _, record := range s.Knowledge {
		record = record.Normalize()
		if record.ID == "" || seenKnowledge[record.ID] {
			continue
		}
		seenKnowledge[record.ID] = true
		knowledge = append(knowledge, record)
	}
	s.Knowledge = knowledge

	matters := make([]Matter, 0, len(s.Matters))
	seenMatters := map[string]bool{}
	for _, matter := range s.Matters {
		matter = matter.Normalize()
		if matter.ID == "" || seenMatters[matter.ID] {
			continue
		}
		seenMatters[matter.ID] = true
		matters = append(matters, matter)
	}
	s.Matters = matters

	messages := make([]Message, 0, len(s.Messages))
	seenMessages := map[string]bool{}
	for _, message := range s.Messages {
		message = message.Normalize()
		if message.ID == "" || seenMessages[message.ID] {
			continue
		}
		seenMessages[message.ID] = true
		messages = append(messages, message)
	}
	s.Messages = messages

	groups := make([]Group, 0, len(s.Groups))
	seenGroups := map[string]bool{}
	for _, group := range s.Groups {
		group = group.Normalize()
		if group.ID == "" || seenGroups[group.ID] {
			continue
		}
		seenGroups[group.ID] = true
		groups = append(groups, group)
	}
	s.Groups = groups

	affinities := make([]AffinityChange, 0, len(s.Affinities))
	seenAffinities := map[string]bool{}
	for _, change := range s.Affinities {
		change = change.Normalize()
		if change.ID == "" || seenAffinities[change.ID] || change.Delta == 0 {
			continue
		}
		seenAffinities[change.ID] = true
		affinities = append(affinities, change)
	}
	s.Affinities = affinities

	relationships := make([]Relationship, 0, len(s.Relationships))
	seenRelationships := map[string]bool{}
	for _, relationship := range s.Relationships {
		relationship = relationship.Normalize()
		if relationship.ID == "" || seenRelationships[relationship.ID] {
			continue
		}
		seenRelationships[relationship.ID] = true
		relationships = append(relationships, relationship)
	}
	s.Relationships = relationships

	activities := make([]IntimacyActivity, 0, len(s.IntimacyActivities))
	seenActivities := map[string]bool{}
	for _, activity := range s.IntimacyActivities {
		activity = activity.Normalize()
		if activity.ID == "" || seenActivities[activity.ID] {
			continue
		}
		seenActivities[activity.ID] = true
		activities = append(activities, activity)
	}
	s.IntimacyActivities = activities
	if s.Revision < 0 {
		s.Revision = 0
	}
	return s
}

// CharacterIDs lists every character the story knows, in stable order. The
// character window lists from here, so a story with a dozen characters shows a
// dozen rather than the two that happened to be hardcoded.
func (s State) CharacterIDs() []string {
	s = s.Normalize()
	ids := make([]string, 0, len(s.Profiles))
	for _, profile := range s.Profiles {
		ids = append(ids, profile.ID)
	}
	sort.Strings(ids)
	return ids
}

func (s State) Profile(id string) (Profile, bool) {
	id = trimSpace(id)
	for _, profile := range s.Profiles {
		if profile.ID == id {
			return profile.Normalize(), true
		}
	}
	return Profile{}, false
}

// ProfileBySourceKey finds the actor this story already bound to an author's
// preset character. It is the lookup that makes a preset person reuse their
// actor: without it, the second mention of 陈巧倩 mints a second 陈巧倩, and the
// two share neither memory nor relationships.
//
// Only a non-empty key matches. Story-invented characters carry no source key,
// so they can never be mistaken for a preset person who happens to share a name.
func (s State) ProfileBySourceKey(sourceKey string) (Profile, bool) {
	sourceKey = trimSpace(sourceKey)
	if sourceKey == "" {
		return Profile{}, false
	}
	for _, profile := range s.Profiles {
		profile = profile.Normalize()
		if profile.SourceKey == sourceKey {
			return profile, true
		}
	}
	return Profile{}, false
}

// SourceKeys lists the author keys this branch has already bound, in stable
// order. Candidate preparation reads it so it does not re-offer someone the
// story has already met.
func (s State) SourceKeys() []string {
	s = s.Normalize()
	keys := make([]string, 0, len(s.Profiles))
	for _, profile := range s.Profiles {
		if profile.SourceKey != "" {
			keys = append(keys, profile.SourceKey)
		}
	}
	sort.Strings(keys)
	return keys
}

// UpsertProfile registers or updates a character. The revision always moves
// forward so a prepared context built for the previous revision cannot be
// reused for the new voice.
func (s *State) UpsertProfile(profile Profile) error {
	profile = profile.Normalize()
	if profile.ID == "" || profile.StoryID == "" {
		return fmt.Errorf("character profile requires an id and a story id")
	}
	switch profile.Lifeform {
	case "", LifeformHuman, LifeformAnimal, LifeformSpiritBeast:
	default:
		return fmt.Errorf("character profile %s has an invalid lifeform: %q", profile.ID, profile.Lifeform)
	}
	if profile.CultivationMethodID != "" && profile.Lifeform != LifeformSpiritBeast {
		return fmt.Errorf("character profile %s can only bind a cultivation method as a spirit beast", profile.ID)
	}
	for index := range s.Profiles {
		if s.Profiles[index].ID != profile.ID {
			continue
		}
		existing := s.Profiles[index].Normalize()
		if profile.Lifeform != "" && profile.Lifeform != existing.Lifeform {
			return fmt.Errorf("character profile %s cannot change its lifeform from %q to %q", profile.ID, existing.Lifeform, profile.Lifeform)
		}
		if profile.CultivationMethodID != "" && profile.CultivationMethodID != existing.CultivationMethodID {
			return fmt.Errorf("character profile %s cannot change its cultivation method from %q to %q", profile.ID, existing.CultivationMethodID, profile.CultivationMethodID)
		}
		if profile.Revision <= existing.Revision {
			profile.Revision = existing.Revision + 1
		}
		if profile.CreatedAt == "" {
			profile.CreatedAt = existing.CreatedAt
		}
		// A later turn that says nothing about who this person is must not unmask
		// them. The source key is the author's identity and the only field that can
		// find this actor again: a returning person is usually described without
		// one, because the discovery page that supplied it no longer offers someone
		// the story has already met. Clearing it here would make the next turn
		// register a second actor for the same preset person, with a second memory
		// and a second set of relationships.
		//
		// Only the absence is filled in. A key the turn did state is written as
		// given, so a correction still lands.
		if profile.SourceKey == "" {
			profile.SourceKey = existing.SourceKey
		}
		if profile.Lifeform == "" {
			profile.Lifeform = existing.Lifeform
		}
		if profile.CultivationMethodID == "" {
			profile.CultivationMethodID = existing.CultivationMethodID
		}
		// The same rule holds for gender. A turn that describes someone without
		// mentioning it must not unset what an earlier turn settled, and the
		// story must not re-decide it every time the person comes back. Only a
		// stated value is written, so a real correction still lands.
		if profile.Gender == "" {
			profile.Gender = existing.Gender
		}
		if profile.Age <= 0 {
			profile.Age = existing.Age
		}
		// And for the voice. The design asks that a person sound the same on
		// re-appearance, across windows and after a restart: a turn that only
		// reports where someone is standing must not flatten them back to a
		// generic narrator. Personality and motives are deliberately not kept
		// this way — those are meant to move as the story does.
		if profile.SpeechStyle == "" {
			profile.SpeechStyle = existing.SpeechStyle
		}
		// The introduction and the role are the two fields the person card shows,
		// and a returning person is normally described by what they just did
		// rather than by who they are. A turn that only reports a change of
		// attitude would otherwise blank both, and the player would open the card
		// on somebody the story can no longer say anything about. They are kept
		// on the same rule as the three above: an unstated field is filled from
		// the earlier revision, a stated one is written as given.
		if profile.Description == "" {
			profile.Description = existing.Description
		}
		if profile.Role == "" {
			profile.Role = existing.Role
		}
		// Tags accumulate rather than replace. They are the labels a reader would
		// search by — affiliation, craft, region — and each one is a fact the story
		// established at some point. A later turn that names one more of them did
		// not retract the earlier ones, and dropping them would quietly make a
		// person unfindable by the thing the player learned first. Order is kept so
		// the list does not churn between revisions for no reason.
		profile.Tags = mergeTextList(existing.Tags, profile.Tags)
		if profile.RadioContact == nil {
			profile.RadioContact = existing.RadioContact
		}
		s.Profiles[index] = profile
		return nil
	}
	if profile.Revision < 1 {
		profile.Revision = 1
	}
	s.Profiles = append(s.Profiles, profile)
	return nil
}

// AffinityFor returns one character's current feeling toward the protagonist
// and the band it falls into. No settled change means 0: a stranger has not
// decided anything, and 0 is what that looks like on the same scale that later
// carries real feeling.
func (s State) AffinityFor(characterID string) (int, AffinityBand) {
	value := CurrentAffinity(characterID, s.Affinities)
	return value, BandFor(value)
}

// AffinityChangesFor returns one character's settlements, oldest first.
func (s State) AffinityChangesFor(characterID string) []AffinityChange {
	return RecentAffinityChanges(characterID, s.Affinities, 0)
}

// RelationshipFor returns the formal relationship currently standing with one
// character, if there is one.
func (s State) RelationshipFor(characterID string) (Relationship, bool) {
	return CurrentRelationship(characterID, s.Relationships)
}

// ExchangesFor returns the exchanges this character can remember, in commit
// order. It is the read both the window transcript and the character's own
// preparation take, so they cannot disagree about what was said.
func (s State) ExchangesFor(characterID string) []Exchange {
	characterID = trimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]Exchange, 0, len(s.Exchanges))
	for _, exchange := range s.Exchanges {
		exchange = exchange.Normalize()
		if exchangeReaches(exchange, characterID) {
			out = append(out, exchange)
		}
	}
	return out
}

// GroupExchanges lists one group's conversation in commit order. It is what the
// group window shows, and it is deliberately separate from ExchangesFor: a
// character's memory is filtered by audience, while the group's own transcript
// is the whole conversation that happened in that group.
func (s State) GroupExchanges(groupID string) []Exchange {
	groupID = trimSpace(groupID)
	if groupID == "" {
		return nil
	}
	out := make([]Exchange, 0, len(s.Exchanges))
	for _, exchange := range s.Exchanges {
		exchange = exchange.Normalize()
		if exchange.Channel != ChannelGroup || exchange.GroupID != groupID {
			continue
		}
		out = append(out, exchange)
	}
	return out
}

// KnowledgeFor returns the records this character holds. It is the read the
// preparation service takes, so a window reply is built from the same knowledge
// the main story's version of this character would be given.
//
// The audience is applied here, not only at the model boundary. A record is
// written once under the point of view that states it, and the people who were
// in the same moment read that one record rather than holding copies; filtering
// by subject alone would leave every listed holder with nothing and make the
// audience value decoration. HolderCanKnow is the same gate the prepared context
// and the recall pass use, so all three answer the same question.
func (s State) KnowledgeFor(characterID string) []KnowledgeRecord {
	characterID = trimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]KnowledgeRecord, 0, len(s.Knowledge))
	for _, record := range s.Knowledge {
		record = record.Normalize()
		if !HolderCanKnow(record, characterID) {
			continue
		}
		out = append(out, record)
	}
	return out
}

// MattersFor returns this character's own threads. A matter belongs to the
// character who holds it, so another character's unfinished business never
// reaches this context.
func (s State) MattersFor(characterID string) []Matter {
	characterID = trimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]Matter, 0, len(s.Matters))
	for _, matter := range s.Matters {
		matter = matter.Normalize()
		if matter.CharacterID == characterID {
			out = append(out, matter)
		}
	}
	return out
}

// Apply merges one turn's projected records into the state. It is the only
// write path, which is why retries and recovery cannot double-write: the same
// record ID is dropped rather than appended twice.
func (s *State) Apply(projected Projected, cursor string) error {
	if len(projected.Exchanges) > 0 {
		seen := make(map[string]bool, len(s.Exchanges))
		for _, exchange := range s.Exchanges {
			seen[exchange.Normalize().ID] = true
		}
		for _, exchange := range projected.Exchanges {
			exchange = exchange.Normalize()
			if seen[exchange.ID] {
				continue
			}
			if err := exchange.Validate(); err != nil {
				return err
			}
			seen[exchange.ID] = true
			s.Exchanges = append(s.Exchanges, exchange)
		}
	}
	if len(projected.Knowledge) > 0 {
		s.Knowledge = ApplyKnowledge(s.Knowledge, projected.Knowledge)
	}
	if len(projected.Matters) > 0 {
		s.Matters = ApplyMatters(s.Matters, projected.Matters)
	}
	if len(projected.Messages) > 0 {
		s.Messages = ApplyMessages(s.Messages, projected.Messages)
	}
	if len(projected.Groups) > 0 {
		for _, group := range projected.Groups {
			if err := s.UpsertGroup(group); err != nil {
				return err
			}
		}
	}
	if len(projected.Affinities) > 0 {
		s.Affinities = ApplyAffinities(s.Affinities, projected.Affinities)
	}
	if len(projected.Relationships) > 0 {
		s.Relationships = ApplyRelationships(s.Relationships, projected.Relationships)
	}
	if len(projected.IntimacyActivities) > 0 {
		for _, activity := range projected.IntimacyActivities {
			if err := s.UpsertIntimacyActivity(activity); err != nil {
				return err
			}
		}
	}
	if cursor = trimSpace(cursor); cursor != "" {
		s.Cursor = cursor
	}
	s.Revision++
	return nil
}

func (s State) MessagesFor(characterID string) []Message {
	characterID = trimSpace(characterID)
	if characterID == "" {
		return nil
	}
	out := make([]Message, 0, len(s.Messages))
	for _, message := range s.Messages {
		message = message.Normalize()
		if message.CharacterID == characterID {
			out = append(out, message)
		}
	}
	return out
}

// AddMessages appends incoming messages, dropping any whose ID is already
// present so a retried generation cannot deliver the same message twice.
func (s *State) AddMessages(messages []Message) error {
	seen := make(map[string]bool, len(s.Messages))
	for _, message := range s.Messages {
		seen[message.Normalize().ID] = true
	}
	for _, message := range messages {
		message = message.Normalize()
		if seen[message.ID] {
			continue
		}
		if err := message.Validate(); err != nil {
			return err
		}
		seen[message.ID] = true
		s.Messages = append(s.Messages, message)
	}
	return nil
}

// MarkMessage applies a player action to one message. Marking read never
// accepts an arrangement; only a committed reply can do that, and it does so by
// creating a matter through the normal projection path.
func (s *State) MarkMessage(messageID string, status MessageStatus, worldTime string) error {
	if !status.Valid() {
		return fmt.Errorf("invalid message status: %q", status)
	}
	messageID = trimSpace(messageID)
	for index := range s.Messages {
		if s.Messages[index].Normalize().ID != messageID {
			continue
		}
		s.Messages[index].Status = status
		if status == MessageRead || status == MessageHandled {
			s.Messages[index].ReadWorldTime = trimSpace(worldTime)
		}
		return nil
	}
	return fmt.Errorf("character message not found: %s", messageID)
}

func trimSpace(value string) string {
	return strings.TrimSpace(value)
}
