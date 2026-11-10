package character

import (
	"fmt"
	"sort"
	"strings"
)

// GroupType is what a group is for and where its members come from. It is not a
// second management engine: a party group does not create a party, and a sect
// group does not grant sect authority. The type only says which existing world
// facts the membership was drawn from and what the conversation is for.
type GroupType string

const (
	// GroupSelf is a group the player built from people this story already knows.
	GroupSelf GroupType = "self"
	// GroupParty is drawn from an established party's real member record.
	GroupParty GroupType = "party"
	// GroupSect is drawn from an organization identity and the channels that
	// identity can actually reach.
	GroupSect GroupType = "sect"
	// GroupOther covers world-defined kinds such as a guild or an academy. A
	// world may name the type; it does not thereby gain powers the build lacks.
	GroupOther GroupType = "other"
)

func (t GroupType) Valid() bool {
	switch t {
	case GroupSelf, GroupParty, GroupSect, GroupOther:
		return true
	default:
		return false
	}
}

// GroupMember is one tenure of one person's membership, with the positions that
// make later knowledge answerable: when they joined and, if they left, when.
//
// A person who leaves and comes back has two records rather than one overwritten
// one. Rejoining does not erase the first tenure, and the gap between them is
// exactly the stretch whose conversation they were not in the audience for.
//
// The versions are the group's own member version at the moment of the change.
// They are what lets a message written before someone joined stay out of their
// memory: the message's audience was captured then, and the join position is
// compared against it rather than reconstructed later.
type GroupMember struct {
	CharacterID string `json:"character_id"`
	// JoinedVersion is the group version at which this tenure began.
	JoinedVersion int `json:"joined_version"`
	// JoinedWorldTime is the in-fiction time of the join.
	JoinedWorldTime string `json:"joined_world_time,omitempty"`
	// LeftVersion is the group version at which this tenure ended. Zero means it
	// is still in force.
	LeftVersion   int    `json:"left_version,omitempty"`
	LeftWorldTime string `json:"left_world_time,omitempty"`
}

func (m GroupMember) Normalize() GroupMember {
	m.CharacterID = strings.TrimSpace(m.CharacterID)
	m.JoinedWorldTime = strings.TrimSpace(m.JoinedWorldTime)
	m.LeftWorldTime = strings.TrimSpace(m.LeftWorldTime)
	if m.JoinedVersion < 1 {
		m.JoinedVersion = 1
	}
	return m
}

// Active reports whether this membership is still in force.
func (m GroupMember) Active() bool {
	m = m.Normalize()
	return m.CharacterID != "" && m.LeftVersion == 0
}

// Group is one conversation space. It stores membership and the contact
// condition; it does not store the conversation, which lives on the same story
// journal as every other exchange.
type Group struct {
	ID       string `json:"id"`
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	Type     GroupType `json:"type"`
	Name     string    `json:"name"`
	// Channel is how the group reaches its members. A self-built group in a world
	// with no defined remote channel is a scene group: everyone in it is in the
	// same place, and nobody is reached across a distance that does not exist.
	Channel Channel `json:"channel"`
	// Members is the membership history, oldest first. A character appears once
	// and carries their own join and leave positions rather than being removed.
	Members []GroupMember `json:"members,omitempty"`
	// Version moves forward on every membership change. It is the member version
	// a message is stamped against.
	Version int `json:"version"`
	// CreatedWorldTime is the in-fiction time the group was made.
	CreatedWorldTime string `json:"created_world_time,omitempty"`
	SourceEventID    string `json:"source_event_id,omitempty"`
}

func (g Group) Normalize() Group {
	g.ID = strings.TrimSpace(g.ID)
	g.StoryID = strings.TrimSpace(g.StoryID)
	g.BranchID = strings.TrimSpace(g.BranchID)
	g.Name = strings.TrimSpace(g.Name)
	g.CreatedWorldTime = strings.TrimSpace(g.CreatedWorldTime)
	g.SourceEventID = strings.TrimSpace(g.SourceEventID)
	if g.Type == "" {
		g.Type = GroupSelf
	}
	if g.Channel == "" {
		g.Channel = ChannelScene
	}
	if g.Version < 1 {
		g.Version = 1
	}
	members := make([]GroupMember, 0, len(g.Members))
	for _, member := range g.Members {
		member = member.Normalize()
		if member.CharacterID == "" {
			continue
		}
		members = append(members, member)
	}
	g.Members = members
	return g
}

func (g Group) Validate() error {
	g = g.Normalize()
	if g.ID == "" || g.StoryID == "" || g.BranchID == "" {
		return fmt.Errorf("a group requires an id, a story and a branch")
	}
	if g.Name == "" {
		return fmt.Errorf("group %s has no name", g.ID)
	}
	if !g.Type.Valid() {
		return fmt.Errorf("group %s has an invalid type: %q", g.ID, g.Type)
	}
	if !g.Channel.Valid() {
		return fmt.Errorf("group %s has an invalid channel: %q", g.ID, g.Channel)
	}
	if len(g.Members) == 0 {
		return fmt.Errorf("group %s has no members", g.ID)
	}
	// Only the latest tenure of each person can be open. Two open tenures would
	// make "who is in the group now" ambiguous, and the audience rule reads it.
	open := make(map[string]bool, len(g.Members))
	for _, member := range g.Members {
		if member.LeftVersion != 0 && member.LeftVersion < member.JoinedVersion {
			return fmt.Errorf("group %s member %s left before joining", g.ID, member.CharacterID)
		}
		if !member.Active() {
			continue
		}
		if open[member.CharacterID] {
			return fmt.Errorf("group %s has two open memberships for %s", g.ID, member.CharacterID)
		}
		open[member.CharacterID] = true
	}
	return nil
}

// ActiveMembers lists who is in the group now, in stable order. It is the
// audience of a line spoken in the group and the set the window may address.
func (g Group) ActiveMembers() []string {
	g = g.Normalize()
	out := make([]string, 0, len(g.Members))
	seen := make(map[string]bool, len(g.Members))
	for _, member := range g.Members {
		if !member.Active() || seen[member.CharacterID] {
			continue
		}
		seen[member.CharacterID] = true
		out = append(out, member.CharacterID)
	}
	sort.Strings(out)
	return out
}

// Member returns the person's current tenure, if they are still in the group.
func (g Group) Member(characterID string) (GroupMember, bool) {
	characterID = strings.TrimSpace(characterID)
	for _, member := range g.Members {
		if member.CharacterID == characterID && member.Active() {
			return member.Normalize(), true
		}
	}
	return GroupMember{}, false
}

// FormerMember reports whether this person was ever in the group. It is what
// lets a rejoin be recorded as a new tenure instead of silently reviving the old
// one, which would hand them the conversation they missed.
func (g Group) FormerMember(characterID string) bool {
	characterID = strings.TrimSpace(characterID)
	for _, member := range g.Members {
		if member.CharacterID == characterID {
			return true
		}
	}
	return false
}

// AudienceFor resolves who perceives one line spoken in this group.
//
// A group on the scene channel is a conversation happening where everyone
// stands: its audience is the active membership that is actually here, plus the
// author, who was there by definition. A line spoken here therefore cannot reach
// a member who has walked out of the scene, and no one is moved to hear it. A
// group with a real remote channel reaches its whole active membership.
//
// The result is captured onto the exchange at commit. That is what makes a later
// joiner unable to remember it: the audience was written down when the line was
// spoken, so re-reading the group's membership now cannot quietly add them.
func (g Group) AudienceFor(authorID string, scenePresent []string) []string {
	g = g.Normalize()
	members := g.ActiveMembers()
	if g.Channel == ChannelScene {
		present := make(map[string]bool, len(scenePresent))
		for _, id := range scenePresent {
			if id = strings.TrimSpace(id); id != "" {
				present[id] = true
			}
		}
		kept := make([]string, 0, len(members))
		for _, id := range members {
			if present[id] {
				kept = append(kept, id)
			}
		}
		members = kept
	}
	return normalizeIDs(append(members, strings.TrimSpace(authorID)))
}

// Join adds a person to the group as a new tenure and advances the member
// version. Joining when already present is a no-op that returns the group
// unchanged, so a retried request cannot open a second tenure.
func (g Group) Join(characterID, worldTime string) Group {
	g = g.Normalize()
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return g
	}
	if _, ok := g.Member(characterID); ok {
		return g
	}
	g.Version++
	g.Members = append(g.Members, GroupMember{
		CharacterID: characterID, JoinedVersion: g.Version,
		JoinedWorldTime: strings.TrimSpace(worldTime),
	})
	return g.Normalize()
}

// Leave ends the person's current tenure and advances the member version. They
// keep the record of having been there, which is why they still remember what
// was said while they were, and why what is said after they go never reaches
// them. Leaving when not present is a no-op.
func (g Group) Leave(characterID, worldTime string) Group {
	g = g.Normalize()
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return g
	}
	index := -1
	for i, member := range g.Members {
		if member.CharacterID == characterID && member.Active() {
			index = i
			break
		}
	}
	if index < 0 {
		return g
	}
	g.Version++
	g.Members[index].LeftVersion = g.Version
	g.Members[index].LeftWorldTime = strings.TrimSpace(worldTime)
	return g.Normalize()
}

// ErrGroupUnknown means a group id names no group on this branch.
var ErrGroupUnknown = fmt.Errorf("group not found")

// ErrGroupVersionConflict means a membership change was based on a version the
// group has already moved past. Refusing is the honest answer: applying it would
// write a join or a leave at a position that no longer means what the caller
// thought it did.
var ErrGroupVersionConflict = fmt.Errorf("group member version conflict")

// UpsertGroup registers or replaces one group record. A record older than the one
// on file is refused rather than applied, so a retried or out-of-order write
// cannot roll the membership back.
func (s *State) UpsertGroup(group Group) error {
	group = group.Normalize()
	if err := group.Validate(); err != nil {
		return err
	}
	for index := range s.Groups {
		existing := s.Groups[index].Normalize()
		if existing.ID != group.ID {
			continue
		}
		if group.Version < existing.Version {
			return fmt.Errorf("%w: group=%s on_file=%d requested=%d", ErrGroupVersionConflict, group.ID, existing.Version, group.Version)
		}
		if group.CreatedWorldTime == "" {
			group.CreatedWorldTime = existing.CreatedWorldTime
		}
		if group.SourceEventID == "" {
			group.SourceEventID = existing.SourceEventID
		}
		s.Groups[index] = group
		return nil
	}
	s.Groups = append(s.Groups, group)
	return nil
}

// Group returns one group by id.
func (s State) Group(id string) (Group, bool) {
	id = strings.TrimSpace(id)
	for _, group := range s.Groups {
		if group.Normalize().ID == id {
			return group.Normalize(), true
		}
	}
	return Group{}, false
}

// GroupsFor lists the groups this branch holds, in stable order. The window lists
// from here, so a story with several groups shows all of them.
func (s State) GroupsFor() []Group {
	s = s.Normalize()
	out := make([]Group, 0, len(s.Groups))
	for _, group := range s.Groups {
		group = group.Normalize()
		if group.ID == "" {
			continue
		}
		out = append(out, group)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
