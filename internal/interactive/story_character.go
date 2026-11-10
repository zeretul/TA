package interactive

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"ta/internal/character"
)

// CharacterRecordEvent is a side record on the canonical story journal. It
// carries character-side facts — exchanges, knowledge, matters, profiles,
// messages — without creating a story Turn and without consuming a turn number.
//
// This is what keeps a single save. Character memory is not a second store that
// can drift from the story: it is a projection of the same JSONL, keyed by the
// same branch, and a rebuild from the journal reproduces it exactly.
type CharacterRecordEvent struct {
	FactionMemberships []character.FactionMembership `json:"faction_memberships,omitempty"`
	V                  int                           `json:"v"`
	Type               string                        `json:"type"`
	ID                 string                        `json:"id"`
	ParentID           string                        `json:"parent_id"`
	BranchID           string                        `json:"branch_id"`
	Ts                 string                        `json:"ts"`
	// TurnID is the main-story turn this record belongs to. It is empty for a
	// window-only exchange, which deliberately does not advance the story.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position the records were projected at. It
	// is the projection's resume point, so an interrupted rebuild continues
	// rather than reprocessing the journal.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key this record was written
	// under, and CommandHash fingerprints the content behind it. Together they
	// let a retried append find what the first attempt committed instead of
	// writing the same exchange twice.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`

	Profiles  []character.Profile         `json:"profiles,omitempty"`
	Exchanges []character.Exchange        `json:"exchanges,omitempty"`
	Knowledge []character.KnowledgeRecord `json:"knowledge,omitempty"`
	Matters   []character.Matter          `json:"matters,omitempty"`
	// FactionMatter is authored only by the formal faction action, never by prose.
	FactionMatter *character.Matter   `json:"faction_matter,omitempty"`
	Messages      []character.Message `json:"messages,omitempty"`
	// Groups carries membership changes. A group's conversation is filed as
	// ordinary group exchanges on this same record, so a group and its history
	// are restored from one journal rather than two stores.
	Groups []character.Group `json:"groups,omitempty"`
	// Affinities are the settled changes to how a character feels about the
	// protagonist. They ride this record rather than a second ledger so a task's
	// reward, the story's advance and the feeling it caused commit together.
	Affinities []character.AffinityChange `json:"affinities,omitempty"`
	// Relationships are formal relationship changes: a friendship declared, a
	// bond sworn, a parting. They are separate from Affinities because a
	// relationship is established by what was said, not by a number.
	Relationships []character.Relationship `json:"relationships,omitempty"`
	// MessageStatuses records the player's own actions on delivered messages.
	// They are kept apart from Messages because a status change is the player's
	// state, not the character's act: the same message can be re-delivered by a
	// replay without losing what the player already did with it.
	MessageStatuses []character.MessageStatusChange `json:"message_statuses,omitempty"`
	// IntimacyActivities are the activity records: who was invited, what each
	// person answered, and the roster the player confirmed. They ride this record
	// because an activity is a fact about a conversation this branch already
	// files here — not a second journal and not a second character state.
	IntimacyActivities []character.IntimacyActivity `json:"intimacy_activities,omitempty"`
}

// AppendCharacterFactsRequest is one committed character-side outcome.
//
// The caller supplies drafts, not records: identity, audience and timestamps are
// assigned by the store from the branch it is writing to, so a model cannot
// choose its own witnesses or backdate a memory.
type AppendCharacterFactsRequest struct {
	BranchID string
	// TurnID attaches the records to a story turn. Empty is legitimate for a
	// window conversation.
	TurnID string
	// WorldTime is the current in-fiction time. It is frozen onto every record,
	// which is what makes the player's real waiting cost nothing.
	WorldTime string
	// Scene resolves the audience of every exchange.
	Scene character.Scene
	// Profiles registers or updates characters in the same transaction. A
	// character must exist before they can speak or learn, so a first appearance
	// arrives here rather than being rejected as unknown.
	Profiles []character.Profile
	Facts    character.TurnCharacterFacts
	// Groups registers or updates conversation spaces in the same transaction as
	// the exchanges they carry. A group is stamped with this story and branch by
	// the store, so a caller cannot create one on a branch it is not writing to.
	Groups []character.Group
	// IntimacyActivities registers or updates activity records in the same
	// transaction as the conversation they describe. An activity and its opening
	// exchange therefore commit together: there is no state in which the room is
	// open but the roster that justified it was never written.
	IntimacyActivities []character.IntimacyActivity
	// CommandID is the caller-owned idempotency key for this append. Empty is a
	// legitimate one-shot write. When it is set, a second append carrying the
	// same command ID and the same CommandHash returns the records the first one
	// committed instead of writing a duplicate; the same command ID with a
	// different hash is a conflict rather than a silent overwrite.
	CommandID string
	// CommandHash is the caller's fingerprint of the content behind CommandID.
	// The store treats it as opaque, because what counts as "the same player
	// action" is a product decision the caller owns.
	CommandHash string
}

// ErrCharacterCommandConflict means a command ID was reused for different
// content. Refusing is the only honest answer: accepting would either lose the
// first exchange or write the second one under a false identity.
var ErrCharacterCommandConflict = errors.New("character command identity conflict")

// AppendCharacterFacts projects one turn's character-side drafts into durable
// records and appends them as a single side record. Projection and append are
// one transaction, so a record either exists in the journal or does not exist.
//
// The append is idempotent on CommandID. A retry after a lost response, a
// repeated click or a recovery re-read returns the records the first attempt
// committed, so the same conversation turn is never settled twice.
func (s *Store) AppendCharacterFacts(storyID string, req AppendCharacterFactsRequest) (character.Projected, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return character.Projected{}, err
	}
	defer releaseStory()

	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return character.Projected{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrCharacterCommandConflict, commandID)
	}

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return character.Projected{}, err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return character.Projected{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := characterFactsForCommand(records, branchID, commandID, commandHash)
		if err != nil {
			return character.Projected{}, err
		}
		if found {
			// The first attempt already committed this exact content. Returning
			// its records makes the retry a read rather than a second write.
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return character.Projected{}, branchTerminalError()
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return character.Projected{}, err
	}
	state := character.State{}
	if projection.Characters != nil {
		state = projection.Characters.Normalize()
	}
	for _, profile := range req.Profiles {
		if person, blocked := recordOnlyCatalogPerson(projection.PackCatalog, profile.SourceKey, profile.Name); blocked {
			return character.Projected{}, fmt.Errorf("catalog character is record-only in this frozen world: %s", person.SourceKey)
		}
		profile.StoryID = strings.TrimSpace(storyID)
		if err := state.UpsertProfile(profile); err != nil {
			return character.Projected{}, err
		}
	}
	// A membership change is applied before projection so a line committed in the
	// same transaction resolves its audience against the membership that just
	// changed, rather than against the one it replaced.
	groupsToWrite := make([]character.Group, 0, len(req.Groups))
	for _, group := range req.Groups {
		group.StoryID = strings.TrimSpace(storyID)
		group.BranchID = branchID
		if err := state.UpsertGroup(group); err != nil {
			return character.Projected{}, err
		}
		stored, ok := state.Group(group.Normalize().ID)
		if !ok {
			return character.Projected{}, fmt.Errorf("group %q vanished during upsert", group.ID)
		}
		groupsToWrite = append(groupsToWrite, stored)
	}
	// Activities are applied before projection for the same reason: a line filed
	// in this transaction resolves its audience against the roster this
	// transaction confirms, not against the one it replaced. An older activity
	// version is refused, so a retry cannot roll a confirmed roster back.
	activitiesToWrite := make([]character.IntimacyActivity, 0, len(req.IntimacyActivities))
	for _, activity := range req.IntimacyActivities {
		activity.StoryID = strings.TrimSpace(storyID)
		activity.BranchID = branchID
		if activity.Scene.WorldTime == "" {
			activity.Scene.WorldTime = strings.TrimSpace(req.WorldTime)
		}
		if err := state.UpsertIntimacyActivity(activity); err != nil {
			return character.Projected{}, err
		}
		stored, ok := state.IntimacyActivityFor(activity.Normalize().ID)
		if !ok {
			return character.Projected{}, fmt.Errorf("intimacy activity %q vanished during upsert", activity.ID)
		}
		activitiesToWrite = append(activitiesToWrite, stored)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("chr")
	scene := req.Scene.Normalize()
	if scene.NodeID == "" {
		scene.NodeID = projection.Head
	}
	if scene.WorldTime == "" {
		scene.WorldTime = strings.TrimSpace(req.WorldTime)
	}
	projected, err := character.Project(character.ProjectionContext{
		StoryID:         strings.TrimSpace(storyID),
		BranchID:        branchID,
		TurnID:          strings.TrimSpace(req.TurnID),
		WorldTime:       strings.TrimSpace(req.WorldTime),
		CommitID:        eventID,
		ClockSeconds:    projection.clockSeconds(),
		Calendar:        meta.Calendar.DeadlineUnits(),
		Scene:           scene,
		KnownCharacters: state.CharacterIDs(),
		Groups:          state.GroupsFor(),
		// The branch's own settled history, so a repeated fact is recognised as
		// repeated rather than scored again, and the resulting value is computed
		// from what this branch actually holds.
		ExistingMatters:       state.Matters,
		ExistingAffinities:    state.Affinities,
		ExistingRelationships: state.Relationships,
		NextID:                newID,
	}, req.Facts)
	if err != nil {
		return character.Projected{}, err
	}
	projected.Groups = groupsToWrite
	projected.IntimacyActivities = activitiesToWrite

	event := CharacterRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: eventID,
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: eventID,
		CommandID: commandID, CommandHash: commandHash,
		// Only the profiles this record actually touches are carried, so one
		// window conversation does not rewrite every profile in the story.
		Profiles:  profilesToWrite(state.Profiles, req.Profiles),
		Exchanges: projected.Exchanges, Knowledge: projected.Knowledge, Matters: projected.Matters,
		// A message submitted through this path rides the record that projected it.
		// Leaving it out would project the message, return it to the caller and
		// then drop it on the way to the journal, so the sender's own window would
		// show a letter the story never recorded.
		Messages:           projected.Messages,
		Groups:             groupsToWrite,
		Affinities:         projected.Affinities,
		Relationships:      projected.Relationships,
		IntimacyActivities: activitiesToWrite,
	}

	branch.Head = eventID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return character.Projected{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return character.Projected{}, err
	}
	return projected, nil
}

// AppendCharacterMessages delivers incoming messages. Receiving one never
// accepts an arrangement; only a committed reply can, and that goes through
// AppendCharacterFacts.
func (s *Store) AppendCharacterMessages(storyID, branchID string, messages []character.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if branchIsTerminal(records, branch.Head) {
		return branchTerminalError()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("chm")
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return err
	}
	known := make(map[string]bool)
	if projection.Characters != nil {
		for _, id := range projection.Characters.Normalize().CharacterIDs() {
			known[id] = true
		}
	}
	normalized := make([]character.Message, 0, len(messages))
	clockSeconds := projection.clockSeconds()
	calendar := meta.Calendar.DeadlineUnits()
	for _, message := range messages {
		message.StoryID = strings.TrimSpace(storyID)
		message.BranchID = branchID
		message.SourceEventID = eventID
		message = message.Normalize()
		// A message comes from someone the story has met. An unregistered sender
		// is a stranger the player has no way to place, so the record is refused
		// rather than shown as an anonymous caller.
		if !known[message.CharacterID] {
			return fmt.Errorf("unknown character %q cannot send a message", message.CharacterID)
		}
		// The deadline a letter names has to be placed on the branch clock here,
		// the same way the turn path places one when it projects the same field.
		// Keeping only the label would leave LapsedAt to compare two labels, and
		// that comparison orders 第3日 酉时 against 第3日 戌时 but cannot order
		// 三日内 at all — so a letter that said three days would read as open
		// forever. A label the rules cannot place stays unbound on purpose.
		if seconds, ok := character.BindDeadlineSecondsOnCalendar(message.Deadline, clockSeconds, calendar); ok {
			message.DeadlineSeconds = seconds
		}
		if err := message.Validate(); err != nil {
			return err
		}
		normalized = append(normalized, message)
	}
	parentID := branch.Head
	branch.Head = eventID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, CharacterRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: eventID,
		ParentID: parentID, BranchID: branchID, Ts: now, Cursor: eventID,
		Messages: normalized,
	}); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}

// MarkCharacterMessage applies one player action to a delivered message and
// commits it. Reading, ignoring or handling a message is the player's own state,
// so it is durable, but it is never an acceptance: only a committed reply can
// accept an arrangement, and that goes through AppendCharacterFacts.
//
// worldTime is the story's current in-fiction time. Reading consumes none of it,
// so this is the send time unless the story moved on first; the caller supplies
// it because the store cannot tell a slow reader from a fast one.
func (s *Store) MarkCharacterMessage(storyID, branchID, characterID, messageID string, status character.MessageStatus, worldTime string) (character.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return character.State{}, err
	}
	defer releaseStory()

	messageID = strings.TrimSpace(messageID)
	characterID = strings.TrimSpace(characterID)
	if messageID == "" {
		return character.State{}, fmt.Errorf("a message status change requires a message id")
	}
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return character.State{}, err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return character.State{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return character.State{}, err
	}
	if projection.Characters == nil {
		return character.State{}, fmt.Errorf("%w: %s", character.ErrMessageNotFound, messageID)
	}
	state := projection.Characters.Normalize()
	// The message must belong to the character the window is open on, so one
	// character's inbox cannot be edited from another's window.
	owned := false
	for _, message := range state.MessagesFor(characterID) {
		if message.ID == messageID {
			owned = true
			break
		}
	}
	if !owned {
		return character.State{}, fmt.Errorf("%w: %s", character.ErrMessageNotFound, messageID)
	}
	change := character.MessageStatusChange{
		MessageID: messageID, Status: status,
		WorldTime: strings.TrimSpace(worldTime),
	}
	if err := state.ApplyMessageStatus(change); err != nil {
		return character.State{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("chm")
	parentID := branch.Head
	branch.Head = eventID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, CharacterRecordEvent{
		V: schemaVersion, Type: StoryEventTypeCharacterRecord, ID: eventID,
		ParentID: parentID, BranchID: branchID, Ts: now, Cursor: eventID,
		MessageStatuses: []character.MessageStatusChange{change},
	}); err != nil {
		return character.State{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return character.State{}, err
	}
	return state, nil
}

// CharacterState reads one branch's character projection. It is the read the
// character window and the next story turn share, so both see the same
// identity, the same memory and the same unfinished business.
func (s *Store) CharacterState(storyID, branchID string) (character.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return character.State{}, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return character.State{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return character.State{}, err
	}
	if projection.Characters == nil {
		return character.State{Version: character.StateVersion}, nil
	}
	return projection.Characters.Normalize(), nil
}

// CharacterExchangesForCommand reads back what a previous attempt committed
// under one command ID. It is the durable half of the window's idempotency: the
// caller checks it before generating, so a retry replays the committed exchange
// instead of asking the character again.
func (s *Store) CharacterExchangesForCommand(storyID, branchID, commandID, commandHash string) (character.Projected, bool, error) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return character.Projected{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return character.Projected{}, false, err
	}
	defer release()
	meta, records, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return character.Projected{}, false, err
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = meta.CurrentBranch
	}
	return characterFactsForCommand(records, strings.TrimSpace(branchID), commandID, strings.TrimSpace(commandHash))
}

// characterFactsForCommand looks for the character record a previous attempt
// committed under this command ID. It scans the branch's own recent records
// rather than trusting a process-local index, so the answer survives a restart:
// the journal is what remembers, not the process.
//
// A found command ID with a different hash is a conflict. The caller reused one
// identity for two different actions, and either answer would be wrong.
func characterFactsForCommand(records []StoryEventRecord, branchID, commandID, commandHash string) (character.Projected, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeCharacterRecord {
			continue
		}
		var event CharacterRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return character.Projected{}, false, fmt.Errorf("decode committed character record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return character.Projected{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q", ErrCharacterCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return character.Projected{
			Exchanges: event.Exchanges, Knowledge: event.Knowledge, Matters: event.Matters,
			Messages: event.Messages,
			Groups:   event.Groups, Affinities: event.Affinities, Relationships: event.Relationships,
			IntimacyActivities: event.IntimacyActivities,
		}, true, nil
	}
	return character.Projected{}, false, nil
}

// profilesToWrite trims the carried profile set to the characters this record
// actually touches, so one window conversation does not rewrite every profile
// in the story.
func profilesToWrite(all []character.Profile, touched []character.Profile) []character.Profile {
	if len(touched) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(touched))
	for _, profile := range touched {
		wanted[strings.TrimSpace(profile.ID)] = true
	}
	out := make([]character.Profile, 0, len(touched))
	for _, profile := range all {
		if wanted[profile.Normalize().ID] {
			out = append(out, profile.Normalize())
		}
	}
	return out
}

// applyCharacterRecord merges a side record into the branch projection. It is
// the only place character state is written during a rebuild, so a replay
// reproduces the state that was live when the record was committed.
func applyCharacterRecord(branch *storyBranchProjection, event CharacterRecordEvent) error {
	for _, matter := range event.Matters {
		if matter.Faction != nil {
			return fmt.Errorf("ordinary character facts cannot write faction work")
		}
	}
	if branch.Characters == nil {
		state := character.State{Version: character.StateVersion}
		branch.Characters = &state
	}
	state := branch.Characters.Normalize()
	for _, profile := range event.Profiles {
		if err := state.UpsertProfile(profile); err != nil {
			return err
		}
	}
	if len(event.Groups) > 0 {
		for _, group := range event.Groups {
			if err := state.UpsertGroup(group); err != nil {
				return err
			}
		}
	}
	if len(event.Exchanges) > 0 || len(event.Knowledge) > 0 || len(event.Matters) > 0 || len(event.Messages) > 0 ||
		len(event.Affinities) > 0 || len(event.Relationships) > 0 || len(event.IntimacyActivities) > 0 {
		if err := state.Apply(character.Projected{
			Exchanges: event.Exchanges, Knowledge: event.Knowledge,
			Matters: event.Matters, Messages: event.Messages,
			Affinities: event.Affinities, Relationships: event.Relationships,
			IntimacyActivities: event.IntimacyActivities,
		}, event.Cursor); err != nil {
			return err
		}
	}
	if err := state.ApplyFactionMemberships(event.FactionMemberships, event.Cursor); err != nil {
		return err
	}
	if event.FactionMatter != nil {
		if err := validateFactionRecordSource(state, event); err != nil {
			return err
		}
		matters, err := character.ApplyFactionMatter(state.Matters, *event.FactionMatter)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(state.Matters, matters) {
			state.Revision++
		}
		state.Matters = matters
		state.Cursor = event.Cursor
	}
	for _, change := range event.MessageStatuses {
		if err := state.ApplyMessageStatus(change); err != nil {
			// A status for a message this branch never received is a stale write
			// from a fork or a rebuilt journal. It is dropped rather than failing
			// the whole rebuild, because the message it refers to is simply not
			// here and nothing about this branch is wrong.
			if errors.Is(err, character.ErrMessageNotFound) {
				continue
			}
			return err
		}
	}
	branch.Characters = &state
	return nil
}

// validateCharacterRecordEvent rejects a record that could not have been
// honestly attributed. A malformed row is an error rather than a silent drop,
// because a dropped record is a character who forgets.
func validateCharacterRecordEvent(event CharacterRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("character record requires a branch")
	}
	for _, membership := range event.FactionMemberships {
		if err := membership.Validate(); err != nil {
			return err
		}
		if membership.SourceEventID != event.ID || membership.TurnID != event.TurnID {
			return fmt.Errorf("faction membership must belong to its canonical character record")
		}
		if membership.SourceKind == "scene" {
			found := false
			for _, exchange := range event.Exchanges {
				if exchange.ID != membership.SourceExchangeID {
					continue
				}
				found = exchange.Channel == character.ChannelScene && exchange.TurnID == event.TurnID
				if membership.Status == "pending" {
					found = found && exchange.Speaker == character.SpeakerPlayer && exchange.AddressedTo == exchange.CharacterID && membership.AuthorityID == ""
				} else {
					found = found && exchange.Speaker == character.SpeakerCharacter && exchange.AddressedTo == DefaultActorID && membership.AuthorityID == exchange.CharacterID
				}
				break
			}
			if !found {
				return fmt.Errorf("faction membership must cite its actual directed scene exchange")
			}
		}
	}
	for _, exchange := range event.Exchanges {
		if err := exchange.Validate(); err != nil {
			return err
		}
	}
	for _, record := range event.Knowledge {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	if event.FactionMatter != nil {
		m := event.FactionMatter
		if m.Faction == nil || m.BranchID != event.BranchID || m.SourceEventID != event.ID || m.TurnID != event.TurnID || event.Cursor != event.ID {
			return fmt.Errorf("faction matter must belong to its canonical character record")
		}
		if err := m.Validate(); err != nil {
			return err
		}
	}
	for _, matter := range event.Matters {
		if matter.Faction != nil {
			return fmt.Errorf("ordinary character facts cannot write faction work")
		}
		if err := matter.Validate(); err != nil {
			return err
		}
	}
	for _, message := range event.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	for _, profile := range event.Profiles {
		if err := profile.Validate(); err != nil {
			return err
		}
	}
	for _, group := range event.Groups {
		if err := group.Validate(); err != nil {
			return err
		}
	}
	for _, change := range event.Affinities {
		if err := change.Validate(); err != nil {
			return err
		}
	}
	for _, relationship := range event.Relationships {
		if err := relationship.Validate(); err != nil {
			return err
		}
	}
	for _, activity := range event.IntimacyActivities {
		if err := activity.Validate(); err != nil {
			return err
		}
	}
	return nil
}
