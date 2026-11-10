package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	agent "ta/agent"
	"ta/agent/providers"
	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
)

// ErrGroupUnknown means the requested group is not part of this story's branch.
var ErrGroupUnknown = character.ErrGroupUnknown

// ErrGroupUnsupported means the requested group kind or contact channel is not
// something this build can honestly provide. Refusing is the point: a group that
// pretended to be a sect channel, or to reach someone across a distance the
// world has no way to cross, would be a capability the game does not have.
var ErrGroupUnsupported = errors.New("group kind or contact channel is not available")

// ErrGroupNoReachableMember means nobody in the group can be reached from the
// current scene. The window says so instead of generating replies from people
// who are not there.
var ErrGroupNoReachableMember = errors.New("no group member can be reached from here")

// GroupCreateRequest is the player building a small group from people the story
// already knows.
type GroupCreateRequest struct {
	StoryID   string
	BranchID  string
	Name      string
	Type      string
	MemberIDs []string
	CommandID string
}

// GroupMemberChangeRequest adds or removes people from a group the player owns.
type GroupMemberChangeRequest struct {
	StoryID   string
	BranchID  string
	GroupID   string
	Add       []string
	Remove    []string
	CommandID string
}

// GroupSendRequest is one message the player sends to a group.
type GroupSendRequest struct {
	StoryID   string
	BranchID  string
	GroupID   string
	Message   string
	CommandID string
	// Locale is the language the player's page is reading in. It decides the
	// language of the round's lines.
	Locale string
}

// GroupWindowMember is one row of a group's roster as the window shows it.
type GroupWindowMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	// AvatarLorePath is the world book's picture for this member, relative to the
	// project. It is used when no avatar was chosen for the actor.
	AvatarLorePath string `json:"avatar_lore_path,omitempty"`
	Role           string `json:"role,omitempty"`
	// Gender is what this member was authored as, so the window can decide
	// whether the intimacy view applies to them. Empty means the story never
	// settled it, which is not the same as either value.
	Gender  string `json:"gender,omitempty"`
	Present bool   `json:"present"`
	// Reachable says whether this member can be addressed from the current scene
	// on the group's contact channel. A member who is not reachable is still in
	// the group; they simply cannot hear what is said right now.
	Reachable bool `json:"reachable"`
}

// GroupWindowGroup is one group as the list and the header show it.
type GroupWindowGroup struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	// Contact says in player-facing terms how the group reaches its members.
	Contact string `json:"contact"`
	// Members is the active roster.
	Members []GroupWindowMember `json:"members"`
	// ReachableMembers counts who can be addressed right now.
	ReachableMembers int `json:"reachable_members"`
	// LastExchange is a one-line preview of the newest line the player may see.
	LastExchange  string `json:"last_exchange,omitempty"`
	LastWorldTime string `json:"last_world_time,omitempty"`
	// LastSequence is where the newest line sits in the branch journal, counting
	// from one. The unified chat list interleaves private and group
	// conversations on committed order; WorldTime is a display string and would
	// order them by something the story never said.
	LastSequence int `json:"last_sequence,omitempty"`
	Version      int `json:"version"`
}

// GroupWindow is the whole group surface: the scene, every group on this
// branch, and one group's conversation.
type GroupWindow struct {
	StoryID  string             `json:"story_id"`
	BranchID string             `json:"branch_id"`
	Scene    GroupWindowScene   `json:"scene"`
	Groups   []GroupWindowGroup `json:"groups"`
	// ProtagonistGender is what this story settled for the protagonist. The
	// intimacy view is offered only when it is set, because every invitation it
	// could make depends on it.
	ProtagonistGender string `json:"protagonist_gender,omitempty"`
	// Cast is the story's known people, so the create form offers real
	// characters rather than a free-text name that would create a stranger.
	Cast []CharacterWindowCharacter `json:"cast,omitempty"`
	// GroupID is the group the window is open on, when one was requested.
	GroupID string `json:"group_id,omitempty"`
	// Exchanges is the group's own conversation, oldest first, bounded to a page.
	Exchanges []character.Exchange `json:"exchanges,omitempty"`
	// Participants names every author in the loaded page, including people who
	// have since left. A line stays readable after its speaker is gone, and it
	// keeps showing the name they spoke under rather than an id.
	Participants map[string]string `json:"participants,omitempty"`
	// Matters are the group members' unresolved threads with the player. They are
	// what the existing negotiated-action entry reads, so a promise made in the
	// group can be carried into the story by the same path as one made in a
	// private conversation.
	Matters []character.Matter `json:"matters,omitempty"`
	HasMore bool               `json:"has_more"`
	Cursor  string             `json:"cursor,omitempty"`
}

// GroupWindowScene is the scene block the group window displays.
type GroupWindowScene struct {
	NodeID    string   `json:"node_id,omitempty"`
	Title     string   `json:"title,omitempty"`
	Location  string   `json:"location,omitempty"`
	WorldTime string   `json:"world_time,omitempty"`
	Present   []string `json:"present,omitempty"`
}

// GroupWindowRequest selects what to read.
type GroupWindowRequest struct {
	StoryID      string
	BranchID     string
	GroupID      string
	BeforeCursor string
	Limit        int
}

// GroupRoundResult is one committed group round.
type GroupRoundResult struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	GroupID  string `json:"group_id"`
	// Lines are the exchanges this round committed, in spoken order.
	Lines []character.Exchange `json:"lines"`
	// WorldTime is the in-fiction time the round was stamped with.
	WorldTime string `json:"world_time"`
	// Silenced lists members who could not be reached this round, so the window
	// can say who did not hear rather than implying everyone answered.
	Silenced []string `json:"silenced,omitempty"`
}

// groupRoundPayload is the model's structured answer for one round.
type groupRoundPayload struct {
	Lines []struct {
		AuthorID string `json:"author_id"`
		Text     string `json:"text"`
	} `json:"lines"`
	Matters []struct {
		CharacterID string `json:"character_id"`
		Kind        string `json:"kind"`
		Statement   string `json:"statement"`
	} `json:"matters"`
}

// GroupWindow reads the group surface. It is one read, so the roster, the
// contact condition and the conversation cannot disagree.
func (a *App) GroupWindow(request GroupWindowRequest) (GroupWindow, error) {
	if a == nil {
		return GroupWindow{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return GroupWindow{}, fmt.Errorf("the group window requires a story")
	}
	a.mu.RLock()
	store := a.interactive
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if store == nil {
		return GroupWindow{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return GroupWindow{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return GroupWindow{}, err
	}
	scene := interactive.StoryScene(storyContext.Snapshot)
	worldTime := characterWorldTime(storyContext)
	pictures := a.characterPictures(storyID, projectID, storyContext, scene)

	window := GroupWindow{
		StoryID: storyID, BranchID: branchID,
		Scene: GroupWindowScene{
			NodeID: scene.NodeID, Title: scene.Title, Location: scene.Location,
			WorldTime: worldTime, Present: scene.Present,
		},
	}
	groups := groupWindowGroups(state, scene, pictures)
	for i := range groups {
		if groups[i].Channel != string(character.ChannelMessage) {
			continue
		}
		groups[i].ReachableMembers = 0
		for j := range groups[i].Members {
			member := &groups[i].Members[j]
			member.Reachable = radioContactAvailable(storyContext, state, member.ID)
			if member.Reachable {
				groups[i].ReachableMembers++
			}
		}
	}
	window.Groups = groups
	window.ProtagonistGender = intimacyProtagonistGender(storyContext)
	window.Cast = characterWindowCast(state, scene, pictures)
	attachWindowClips(window.Cast, state, pictures, media.RoleGeneral, sceneSeed(scene))

	groupID := strings.TrimSpace(request.GroupID)
	if groupID == "" {
		return window, nil
	}
	group, ok := state.Group(groupID)
	if !ok {
		return GroupWindow{}, fmt.Errorf("%w: %s", ErrGroupUnknown, groupID)
	}
	if intimacySpaceIDs(state)[group.ID] {
		// The room is read through the intimacy window, whose own read also
		// carries the roster and each person's answer. Serving it here would give
		// a second surface the ordinary group rules do not describe.
		return GroupWindow{}, fmt.Errorf("%w: %s is an intimacy space", ErrGroupUnknown, groupID)
	}
	window.GroupID = groupID
	window.Matters = groupWindowMatters(state, group)
	page := characterWindowExchangePage(state.GroupExchanges(groupID), request.BeforeCursor, characterWindowLimit(request.Limit))
	window.Exchanges = page.exchanges
	window.HasMore = page.hasMore
	window.Cursor = page.cursor
	window.Participants = groupParticipants(state, page.exchanges)
	return window, nil
}

// groupWindowGroups lists the conversation spaces the player may open in the
// group window. An intimacy activity's room and invitation parley are real
// groups with a real audience, but they are not the player's own groups: they
// belong to the intimacy window, so listing them here would offer the room's
// membership and newest line to anyone browsing the group list.
func groupWindowGroups(state character.State, scene character.Scene, pictures pictureSources) []GroupWindowGroup {
	spaces := intimacySpaceIDs(state)
	groups := state.GroupsFor()
	out := make([]GroupWindowGroup, 0, len(groups))
	for _, group := range groups {
		if spaces[group.ID] {
			continue
		}
		out = append(out, groupWindowGroup(state, group, scene, pictures))
	}
	return out
}

// intimacySpaceIDs names every group an intimacy activity speaks in: the room it
// runs in and each person's own invitation parley. Both are conversation spaces
// with an audience, not ordinary groups.
func intimacySpaceIDs(state character.State) map[string]bool {
	out := make(map[string]bool, len(state.IntimacyActivities)*2)
	for _, activity := range state.IntimacyActivities {
		activity = activity.Normalize()
		out[activity.SpaceID()] = true
		for _, participant := range activity.Participants {
			out[activity.InvitationSpaceID(participant.CharacterID)] = true
		}
	}
	return out
}

// groupParticipants maps every author in the loaded page to their name. It is
// built from the exchanges rather than from the roster, so a line spoken by
// someone who has since left still reads with their name.
func groupParticipants(state character.State, exchanges []character.Exchange) map[string]string {
	out := make(map[string]string, len(exchanges))
	for _, exchange := range exchanges {
		authorID := exchange.Normalize().AuthorID
		if authorID == "" {
			continue
		}
		if _, ok := out[authorID]; ok {
			continue
		}
		if profile, ok := state.Profile(authorID); ok {
			out[authorID] = profile.DisplayName()
		} else {
			out[authorID] = authorID
		}
	}
	return out
}

// groupWindowGroup builds one list row. Reachability is computed here from the
// scene, so the window never offers a conversation the world cannot carry.
func groupWindowGroup(state character.State, group character.Group, scene character.Scene, pictures pictureSources) GroupWindowGroup {
	group = group.Normalize()
	present := make(map[string]bool, len(scene.Present))
	for _, id := range scene.Present {
		present[strings.TrimSpace(id)] = true
	}
	members := make([]GroupWindowMember, 0, len(group.Members))
	reachable := 0
	for _, id := range group.ActiveMembers() {
		profile, ok := state.Profile(id)
		if !ok {
			// A group naming a character this branch no longer holds is a stale
			// record from another branch's history. It is reported by id rather
			// than silently dropped, so the roster does not shrink without reason.
			members = append(members, GroupWindowMember{ID: id, Name: id})
			continue
		}
		row := GroupWindowMember{
			ID: id, Name: profile.DisplayName(), Avatar: pictures.avatar(profile, scene),
			AvatarLorePath: pictures.lorePath(profile),
			Role:           profile.Role, Present: present[id], Gender: profile.Gender,
		}
		row.Reachable = groupMemberReachable(group, scene, id)
		if row.Reachable {
			reachable++
		}
		members = append(members, row)
	}
	row := GroupWindowGroup{
		ID: group.ID, Name: group.Name, Type: string(group.Type),
		Channel: string(group.Channel), Contact: groupContactLabel(group.Channel),
		Members: members, ReachableMembers: reachable, Version: group.Version,
	}
	exchanges := state.GroupExchanges(group.ID)
	if len(exchanges) > 0 {
		last := exchanges[len(exchanges)-1]
		row.LastExchange = oneLineText(last.Text)
		row.LastWorldTime = last.WorldTime
		row.LastSequence = exchangeCommitOrdinal(state.Exchanges, last.ID)
	}
	return row
}

// groupMemberReachable decides whether a member can be addressed right now.
//
// On the scene channel a group is a conversation where everyone stands, so only
// a member actually in the scene is reachable. This is what keeps a group
// message from moving someone across the world or making them answer from a
// place they are not.
func groupMemberReachable(group character.Group, scene character.Scene, characterID string) bool {
	if group.Normalize().Channel != character.ChannelScene {
		return false
	}
	return character.ContainsID(scene.Present, characterID)
}

// groupContactLabel names the contact condition in the player's terms.
func groupContactLabel(channel character.Channel) string {
	switch channel {
	case character.ChannelScene:
		return "same scene"
	case character.ChannelMessage:
		return "message"
	default:
		return string(channel)
	}
}

// groupWindowMatters lists the group's open threads with the player. They are
// the same records a private conversation produces, which is what lets the
// existing negotiated-action entry carry a group agreement into the story.
func groupWindowMatters(state character.State, group character.Group) []character.Matter {
	seen := make(map[string]bool)
	out := make([]character.Matter, 0)
	for _, id := range group.ActiveMembers() {
		for _, matter := range character.OpenMatters(id, state.Matters) {
			if seen[matter.ID] {
				continue
			}
			seen[matter.ID] = true
			out = append(out, matter)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CreateGroup makes a self-built conversation space from people this story
// already knows. It creates a place to talk, and nothing else: no party, no
// organization membership, no relationship change.
func (a *App) CreateGroup(request GroupCreateRequest) (character.Group, error) {
	if a == nil {
		return character.Group{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	name := strings.TrimSpace(request.Name)
	if storyID == "" {
		return character.Group{}, fmt.Errorf("a group requires a story")
	}
	if name == "" {
		return character.Group{}, fmt.Errorf("a group requires a name")
	}
	groupType := character.GroupType(strings.TrimSpace(request.Type))
	if groupType == "" {
		groupType = character.GroupSelf
	}
	if groupType != character.GroupSelf {
		// Party, sect and other groups draw their membership from records this
		// build does not have yet. Accepting the request would produce a group
		// that pretends to be a channel it is not.
		return character.Group{}, fmt.Errorf("%w: only a self-built group can be created until a real member source exists", ErrGroupUnsupported)
	}
	a.mu.RLock()
	store, workspace := a.interactive, a.workspace
	a.mu.RUnlock()
	if store == nil || workspace == "" {
		return character.Group{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return character.Group{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return character.Group{}, err
	}
	worldTime := characterWorldTime(storyContext)

	members := make([]string, 0, len(request.MemberIDs))
	seen := make(map[string]bool, len(request.MemberIDs))
	for _, raw := range request.MemberIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		if _, ok := state.Profile(id); !ok {
			return character.Group{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, id)
		}
		seen[id] = true
		members = append(members, id)
	}
	if len(members) == 0 {
		return character.Group{}, fmt.Errorf("a group needs at least one person the story knows")
	}
	sort.Strings(members)

	groupID := agentrun.NewID("grp")
	group := character.Group{
		ID: groupID, StoryID: storyID, BranchID: branchID,
		Type: character.GroupSelf, Name: name,
		// A self-built group in this world is people talking where they stand.
		// There is no defined remote channel yet, so claiming one would let a
		// message cross a distance the world cannot carry.
		Channel: character.ChannelScene,
		Version: 1, CreatedWorldTime: worldTime,
	}
	// A radio-capable world has a real remote channel; membership still bounds
	// the audience and the existing Story journal keeps the conversation.
	if storyCommunicationMode(storyContext.Meta.PackBinding) == "radio" {
		for _, id := range members {
			if !radioContactAvailable(storyContext, state, id) {
				return character.Group{}, ErrRadioContactUnavailable
			}
		}
		group.Channel = character.ChannelMessage
	}
	for _, id := range members {
		group.Members = append(group.Members, character.GroupMember{
			CharacterID: id, JoinedVersion: 1, JoinedWorldTime: worldTime,
		})
	}
	if err := group.Validate(); err != nil {
		return character.Group{}, err
	}
	commandID := strings.TrimSpace(request.CommandID)
	commandHash := groupCommandHash("create", storyID, branchID, "", name, members, nil)
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID: branchID, WorldTime: worldTime,
		CommandID: commandID, CommandHash: commandHash,
		Groups: []character.Group{group},
	})
	if err != nil {
		return character.Group{}, err
	}
	if len(projected.Groups) > 0 {
		return projected.Groups[0], nil
	}
	return group, nil
}

// ChangeGroupMembers adds and removes people. Removing someone ends their
// tenure; adding them back opens a new one. Neither erases the history: a
// person who was away does not learn what was said while they were gone.
func (a *App) ChangeGroupMembers(request GroupMemberChangeRequest) (character.Group, error) {
	if a == nil {
		return character.Group{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	groupID := strings.TrimSpace(request.GroupID)
	if storyID == "" || groupID == "" {
		return character.Group{}, fmt.Errorf("a membership change requires a story and a group")
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return character.Group{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return character.Group{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return character.Group{}, err
	}
	group, ok := state.Group(groupID)
	if !ok {
		return character.Group{}, fmt.Errorf("%w: %s", ErrGroupUnknown, groupID)
	}
	if intimacySpaceIDs(state)[group.ID] {
		// Membership of an intimacy room is the roster its participants agreed
		// to, and it changes only through their own answers. Editing it here
		// would let the group surface rewrite who consented.
		return character.Group{}, fmt.Errorf("%w: %s is an intimacy space", ErrGroupUnknown, groupID)
	}
	worldTime := characterWorldTime(storyContext)

	add := make([]string, 0, len(request.Add))
	for _, raw := range request.Add {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := state.Profile(id); !ok {
			return character.Group{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, id)
		}
		group = group.Join(id, worldTime)
		add = append(add, id)
	}
	remove := make([]string, 0, len(request.Remove))
	for _, raw := range request.Remove {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		group = group.Leave(id, worldTime)
		remove = append(remove, id)
	}
	if len(add) == 0 && len(remove) == 0 {
		return group, nil
	}
	if err := group.Validate(); err != nil {
		return character.Group{}, err
	}
	commandHash := groupCommandHash("members", storyID, branchID, groupID, "", add, remove)
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID: branchID, WorldTime: worldTime,
		CommandID: strings.TrimSpace(request.CommandID), CommandHash: commandHash,
		Groups: []character.Group{group},
	})
	if err != nil {
		return character.Group{}, err
	}
	if len(projected.Groups) > 0 {
		return projected.Groups[0], nil
	}
	return group, nil
}

// SendGroupMessage runs one group round and commits it.
//
// One round is one model task. The relevant few speak; the rest may stay silent,
// and silence is not agreement. Nothing said here moves anyone, settles anything
// or grants anything: a promise becomes an open matter, and carrying it out is
// the story's job through the normal action path.
func (a *App) SendGroupMessage(ctx context.Context, request GroupSendRequest) (GroupRoundResult, error) {
	if a == nil {
		return GroupRoundResult{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	groupID := strings.TrimSpace(request.GroupID)
	message := strings.TrimSpace(request.Message)
	if storyID == "" || groupID == "" {
		return GroupRoundResult{}, fmt.Errorf("a group message requires a story and a group")
	}
	if message == "" {
		return GroupRoundResult{}, fmt.Errorf("a group message requires text")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return GroupRoundResult{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return GroupRoundResult{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return GroupRoundResult{}, err
	}
	group, ok := state.Group(groupID)
	if !ok {
		return GroupRoundResult{}, fmt.Errorf("%w: %s", ErrGroupUnknown, groupID)
	}
	if intimacySpaceIDs(state)[group.ID] {
		// The room's conversation is read and written through the intimacy
		// surface, which carries the roster and each person's answer. Speaking
		// into it here would address an audience the ordinary group rules do
		// not describe.
		return GroupRoundResult{}, fmt.Errorf("%w: %s is an intimacy space", ErrGroupUnknown, groupID)
	}
	worldTime := characterWorldTime(storyContext)
	scene := interactive.StoryScene(storyContext.Snapshot)
	if scene.WorldTime == "" {
		scene.WorldTime = worldTime
	}
	if scene.NodeID == "" {
		scene.NodeID = branchID
	}

	commandID := strings.TrimSpace(request.CommandID)
	commandHash := groupCommandHash("send", storyID, branchID, groupID, message, nil, nil)
	// A retry must not buy a second model call. The committed round is the
	// durable answer, so it is read back before anything is generated.
	if commandID != "" {
		if committed, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return GroupRoundResult{}, err
		} else if found {
			return groupRoundResultFrom(storyID, branchID, groupID, committed, worldTime), nil
		}
	}

	members := group.ActiveMembers()
	reachable := make([]string, 0, len(members))
	silenced := make([]string, 0, len(members))
	for _, id := range members {
		if groupMemberReachable(group, scene, id) || (group.Channel == character.ChannelMessage && radioContactAvailable(storyContext, state, id)) {
			reachable = append(reachable, id)
		} else {
			silenced = append(silenced, id)
		}
	}
	if len(reachable) == 0 {
		return GroupRoundResult{}, fmt.Errorf("%w: nobody in %s is here", ErrGroupNoReachableMember, group.Name)
	}
	// The existing message channel delivers to its whole active roster. Do not
	// record offline members as witnesses: wait until this channel is complete.
	if group.Channel == character.ChannelMessage && len(silenced) > 0 {
		return GroupRoundResult{}, ErrRadioContactUnavailable
	}

	speakers := groupChatMembers(state, group, reachable, scene)
	joint := groupJointHistory(state, group, reachable)
	replyTarget := storyContext.Meta.ReplyTargetChars
	if replyTarget <= 0 || replyTarget > prompts.DefaultGroupReplyTargetChars {
		replyTarget = prompts.DefaultGroupReplyTargetChars
	}
	systemInstruction := prompts.BuildGroupChatSystemInstruction(prompts.GroupChatSystemInstructionInput{
		Workspace: workspace, GroupName: group.Name,
		GroupType: string(group.Type), Channel: groupContactLabel(group.Channel),
		ReplyTargetChars:    replyTarget,
		Members:             speakers,
		JointHistory:        joint,
		SceneContext:        groupConversationContext(state, group, reachable, scene),
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		OutputLanguage:      request.Locale,
	})
	if storyCommunicationMode(storyContext.Meta.PackBinding) == "radio" && group.Channel == character.ChannelMessage {
		systemInstruction += "\n\n本群通过无线电通信，只有当前成员接收；远端成员不因通信而到场，也不能看见未在通话中说明的本地动作或场景。不要将无线电写成传音或当面聚会。"
	}

	raw, err := a.generateGroupRound(ctx, cfg, storyID, branchID, groupID, systemInstruction, message, config.CharacterChatThinkingLevel(cfg.AgentModels))
	if err != nil {
		return GroupRoundResult{}, err
	}
	payload, err := parseGroupRoundPayload(raw, reachable)
	if err != nil {
		return GroupRoundResult{}, err
	}

	facts := character.TurnCharacterFacts{
		Exchanges: []character.ExchangeDraft{{
			Speaker: character.SpeakerPlayer, Channel: character.ChannelGroup,
			GroupID: group.ID, Text: message,
		}},
	}
	for _, line := range payload.Lines {
		facts.Exchanges = append(facts.Exchanges, character.ExchangeDraft{
			CharacterID: line.AuthorID, Speaker: character.SpeakerCharacter,
			Channel: character.ChannelGroup, GroupID: group.ID, Text: line.Text,
		})
	}
	for _, matter := range payload.Matters {
		facts.Matters = append(facts.Matters, character.MatterDraft{
			CharacterID: matter.CharacterID, Kind: character.MatterKind(matter.Kind),
			Status: character.MatterOpen, Statement: matter.Statement,
		})
	}

	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID: branchID, WorldTime: worldTime, Scene: scene,
		CommandID: commandID, CommandHash: commandHash,
		Facts: facts,
	})
	if err != nil {
		return GroupRoundResult{}, err
	}
	result := groupRoundResultFrom(storyID, branchID, groupID, projected, worldTime)
	result.Silenced = silenced
	slog.InfoContext(ctx, fmt.Sprintf("[group-chat] round committed story_id=%s branch_id=%s group_id=%s lines=%d matters=%d silenced=%d", storyID, branchID, groupID, len(result.Lines), len(payload.Matters), len(silenced)))
	return result, nil
}

// groupRoundResultFrom builds the caller's view of a committed round. The lines
// are read back from what was written rather than from this attempt's
// generation, so a concurrent commit cannot show the player a line the journal
// does not have.
func groupRoundResultFrom(storyID, branchID, groupID string, committed character.Projected, worldTime string) GroupRoundResult {
	lines := make([]character.Exchange, 0, len(committed.Exchanges))
	for _, exchange := range committed.Exchanges {
		lines = append(lines, exchange.Normalize())
	}
	if len(lines) > 0 {
		worldTime = lines[len(lines)-1].WorldTime
	}
	return GroupRoundResult{StoryID: storyID, BranchID: branchID, GroupID: groupID, Lines: lines, WorldTime: worldTime}
}

// groupChatMembers builds the roster block for one round. It carries the public
// persona only: a member's private knowledge, their one-to-one conversation and
// their secrets stay out, because everyone in the round can hear this prompt's
// material.
func groupChatMembers(state character.State, group character.Group, reachable []string, scene character.Scene) []prompts.GroupChatMember {
	canSpeak := make(map[string]bool, len(reachable))
	for _, id := range reachable {
		canSpeak[id] = true
	}
	out := make([]prompts.GroupChatMember, 0, len(group.ActiveMembers()))
	for _, id := range group.ActiveMembers() {
		profile, ok := state.Profile(id)
		if !ok {
			continue
		}
		out = append(out, prompts.GroupChatMember{
			ID: id, Name: profile.DisplayName(), Role: profile.Role,
			Description: profile.Description, Gender: profile.Gender, Personality: profile.Personality,
			SpeechStyle: profile.SpeechStyle, Motives: profile.Motives,
			Attitude: profile.Attitude, Bounds: profile.Bounds,
			CanSpeak: canSpeak[id],
		})
	}
	return out
}

// groupJointHistory renders the part of the group's conversation that every
// reachable member was present for.
//
// It is the intersection, not the union. A line only some of the listeners
// witnessed is not joint material: handing it to the round would tell the model
// something the others did not hear and invite it to use it as if they had.
func groupJointHistory(state character.State, group character.Group, reachable []string) string {
	exchanges := state.GroupExchanges(group.ID)
	if len(exchanges) == 0 {
		return ""
	}
	joint := make([]character.Exchange, 0, len(exchanges))
	for _, exchange := range exchanges {
		audience := exchange.AudienceMembers()
		shared := true
		for _, id := range reachable {
			if !character.ContainsID(audience, id) {
				shared = false
				break
			}
		}
		if shared {
			joint = append(joint, exchange)
		}
	}
	if len(joint) == 0 {
		return ""
	}
	const maxLines = 24
	if len(joint) > maxLines {
		joint = joint[len(joint)-maxLines:]
	}
	var sb strings.Builder
	for _, exchange := range joint {
		if exchange.Speaker == character.SpeakerPlayer {
			fmt.Fprintf(&sb, "- the protagonist: %s\n", oneLineText(exchange.Text))
			continue
		}
		name := exchange.AuthorID
		if profile, ok := state.Profile(exchange.AuthorID); ok {
			name = profile.DisplayName()
		}
		fmt.Fprintf(&sb, "- %s: %s\n", name, oneLineText(exchange.Text))
	}
	return strings.TrimSpace(sb.String())
}

// groupConversationContext contains only this round's common observations and
// live facts all reachable recipients are entitled to know. A remote channel
// does not grant a view of the protagonist's room or the narrator's prose.
func groupConversationContext(state character.State, group character.Group, reachable []string, scene character.Scene) string {
 if len(reachable) == 0 { return "" }
 projected := scene
 projected.Title = ""
 projected.Present = nil
 if group.Normalize().Channel == character.ChannelScene {
  for _, id := range reachable {
   if !character.ContainsID(scene.Present, id) { return "" }
   projected.Present = append(projected.Present, id)
  }
 } else {
  projected.Location = ""
 }
 var sb strings.Builder
 sb.WriteString(groupSceneContext(interactive.StoryContext{}, projected))
 count := 0
 for _, record := range character.BuildKnowledgeView(reachable[0], state.KnowledgeFor(reachable[0]), 0).Records {
  shared := true
  for _, id := range reachable[1:] {
   if !character.HolderCanKnow(record, id) { shared = false; break }
  }
  if !shared { continue }
  if count == 0 { sb.WriteString("\n\n共同知情记录（保留消息性质，不把推测当已证实事实）：\n") }
  fmt.Fprintf(&sb, "- [%s] %s\n", record.Nature, oneLineText(record.Statement))
  count++
  if count >= 24 { break }
 }
 return strings.TrimSpace(sb.String())
}

// groupSceneContext is the bounded "where things stand" block for a round.
func groupSceneContext(storyContext interactive.StoryContext, scene character.Scene) string {
	var sb strings.Builder
	if scene.Location != "" {
		fmt.Fprintf(&sb, "- Current location: %s\n", scene.Location)
	}
	if scene.Title != "" {
		fmt.Fprintf(&sb, "- Current scene: %s\n", scene.Title)
	}
	if scene.WorldTime != "" {
		fmt.Fprintf(&sb, "- Current time: %s\n", scene.WorldTime)
	}
	if len(scene.Present) > 0 {
		names := make([]string, 0, len(scene.Present))
		names = append(names, scene.Present...)
		fmt.Fprintf(&sb, "- Present: %s\n", strings.Join(names, ", "))
	}
	return strings.TrimSpace(sb.String())
}

// parseGroupRoundPayload validates one round's structured answer.
//
// A line naming someone who is not a reachable member is dropped rather than
// committed: the model may not add a stranger to the group or write a line for
// someone who is not there. Dropping is honest because nothing is attributed to
// that person; if nothing usable remains, the round fails rather than inventing
// a reply.
func parseGroupRoundPayload(content string, reachable []string) (groupRoundPayload, error) {
	var payload groupRoundPayload
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &payload); err != nil {
		return groupRoundPayload{}, fmt.Errorf("the group round returned an unreadable answer: %w", err)
	}
	allowed := make(map[string]bool, len(reachable))
	for _, id := range reachable {
		allowed[id] = true
	}
	lines := make([]struct {
		AuthorID string `json:"author_id"`
		Text     string `json:"text"`
	}, 0, len(payload.Lines))
	spoken := make(map[string]bool)
	speakers := 0
	for _, line := range payload.Lines {
		authorID := strings.TrimSpace(line.AuthorID)
		body := strings.TrimSpace(line.Text)
		if authorID == "" || body == "" {
			continue
		}
		if !allowed[authorID] {
			slog.Warn(fmt.Sprintf("[group-chat] dropped a line from an unreachable or unknown author id=%s", authorID))
			continue
		}
		if !spoken[authorID] {
			if speakers >= prompts.MaxGroupSpeakersPerRound {
				continue
			}
			spoken[authorID] = true
			speakers++
		}
		line.AuthorID = authorID
		line.Text = body
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return groupRoundPayload{}, fmt.Errorf("the group round produced no usable line")
	}
	payload.Lines = lines

	matters := make([]struct {
		CharacterID string `json:"character_id"`
		Kind        string `json:"kind"`
		Statement   string `json:"statement"`
	}, 0, len(payload.Matters))
	for _, matter := range payload.Matters {
		characterID := strings.TrimSpace(matter.CharacterID)
		kind := character.MatterKind(strings.TrimSpace(matter.Kind))
		statement := strings.TrimSpace(matter.Statement)
		if !allowed[characterID] || statement == "" || !kind.Valid() {
			continue
		}
		// A disclosure belongs to a private conversation, and an attitude change is
		// not something a round of talk can declare on the player's behalf. Only
		// commitments and beliefs are accepted here.
		switch kind {
		case character.MatterPromise, character.MatterPermission, character.MatterAction,
			character.MatterAttitude, character.MatterMisunderstanding:
		default:
			continue
		}
		matter.CharacterID = characterID
		matter.Kind = string(kind)
		matter.Statement = statement
		matters = append(matters, matter)
	}
	payload.Matters = matters
	return payload, nil
}

// generateGroupRound performs the round's single model call, trying JSON mode
// first and falling back to plain output.
//
// The fallback is not a second round: it is the same request asked again without
// the response-format hint, because some configured models reject the hint while
// still producing the object when asked in the prompt.
func (a *App) generateGroupRound(ctx context.Context, cfg *config.Config, storyID, branchID, groupID, systemInstruction, message string, level providers.ThinkingLevel) (string, error) {
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindCharacterChat)
	if err != nil {
		return "", fmt.Errorf("resolve group model configuration: %w", err)
	}
	if level == "" {
		level = config.CharacterChatThinkingDefault
	}
	modelCfg.ThinkingLevel = level
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "character", cfg.Workspace,
		"group_chat", "Group Conversation Rules",
		"define how a small group of characters answers one message",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(message),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindCharacterChat, messages, nil); err != nil {
		return "", err
	}

	jsonCfg := modelio.WithJSONObjectOutput(modelCfg)
	content, err := a.callGroupModel(ctx, cfg, jsonCfg, storyID, branchID, groupID, message, messages)
	if err == nil {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	slog.ErrorContext(ctx, fmt.Sprintf("[group-chat] json_mode failed, retry without response_format err=%v", err))
	return a.callGroupModel(ctx, cfg, modelCfg, storyID, branchID, groupID, message, messages)
}

func (a *App) callGroupModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, storyID, branchID, groupID, message string, messages []*agent.Message) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "group_chat", "round", map[string]any{
		"story_id": storyID, "branch_id": branchID, "group_id": groupID,
		"message_chars": len([]rune(message)),
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create group model: %w", err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "group_chat", "round", modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "group_chat", "round", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate group round: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the group model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "group_chat", "round", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "group_chat", "round", modelCfg.Model, 0, msg, nil, nil)
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = strings.TrimSpace(msg.ReasoningContent)
	}
	return content, nil
}

// groupCommandHash fingerprints one group action for idempotency.
func groupCommandHash(action, storyID, branchID, groupID, text string, add, remove []string) string {
	parts := []string{
		strings.TrimSpace(action), strings.TrimSpace(storyID), strings.TrimSpace(branchID),
		strings.TrimSpace(groupID), strings.TrimSpace(text),
		strings.Join(sortedCopy(add), ","), strings.Join(sortedCopy(remove), ","),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedCopy(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
