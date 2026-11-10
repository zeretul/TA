package app

import (
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"

	booklore "ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
	"ta/internal/portrait"
)

// CharacterWindowCharacter is one row of the character list: who the player can
// talk to, and what they are looking at. It is built from the story's own actor
// registry, so a story with a dozen characters lists a dozen.
type CharacterWindowCharacter struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	SourceKey           string   `json:"source_key,omitempty"`
	IdentitySource      string   `json:"identity_source"`
	Lifeform            string   `json:"lifeform,omitempty"`
	CultivationMethodID string   `json:"cultivation_method_id,omitempty"`
	KnownAs             string   `json:"known_as,omitempty"`
	Aliases             []string `json:"aliases,omitempty"`
	Avatar              string   `json:"avatar,omitempty"`
	// AvatarLorePath is the world book entry's own picture for this person, as a
	// project-relative asset path. It is the picture the world gave them when no
	// avatar was chosen for the actor, and it is reported relative because an
	// absolute path would be one machine's layout; the client addresses it
	// through the project it is already reading.
	AvatarLorePath string `json:"avatar_lore_path,omitempty"`
	// AvatarDisplay is the same picture at 192px, for the open conversation's
	// portrait. It is empty when the person has no shipped portrait, and the
	// window then draws the placeholder rather than a broken image.
	AvatarDisplay string `json:"avatar_display,omitempty"`
	ImageCaption  string `json:"image_caption,omitempty"`
	Role          string `json:"role,omitempty"`
	Description   string `json:"description,omitempty"`
	// Gender is what the story settled about this person, in the closed
	// vocabulary the profile stores. Empty means nobody decided, and the window
	// then shows nothing: an unknown gender is not a third gender, and printing
	// "unknown" would say the player failed to find something out.
	Gender   string `json:"gender,omitempty"`
	Attitude string `json:"attitude,omitempty"`
	// Tags are the short labels the story attached to this person — the group
	// they belong to, what they do, where they are from. The window's search
	// reads them, so a player who remembers "药铺" finds the clerk whose name
	// they forgot. They are only ever what a turn established.
	Tags []string `json:"tags,omitempty"`
	// Present says whether the character is in the current scene. A character who
	// is not present can still be written to, but through a message rather than
	// face to face.
	Present bool `json:"present"`
	// Channel is the channel a conversation with them currently runs on.
	Channel string `json:"channel"`
	// UnreadMessages counts incoming messages the player has not read.
	UnreadMessages int `json:"unread_messages"`
	// OpenMatters counts the unresolved threads with this character.
	OpenMatters int `json:"open_matters"`
	// FirstSeenWorldTime is when the story first gave them a formal appearance.
	FirstSeenWorldTime string `json:"first_seen_world_time,omitempty"`
	// LastExchange and LastWorldTime preview the newest line of the private
	// conversation, for the unified chat list. They come from the same
	// transcript the window shows, so a preview cannot describe a line the
	// player would not find when opening it. Empty when nothing was ever said.
	LastExchange  string `json:"last_exchange,omitempty"`
	LastWorldTime string `json:"last_world_time,omitempty"`
	// LastSequence is where that line sits in the branch journal, counting from
	// one. The unified chat list mixes private and group conversations, and the
	// committed order is the only honest way to interleave them: WorldTime is a
	// display string rather than a comparable instant, so sorting on it would
	// put conversations in an order the story never told them. Zero means
	// nothing has been said yet.
	LastSequence int `json:"last_sequence,omitempty"`
	// Affinity is how this character currently feels about the protagonist, on
	// the -100..100 scale, and AffinityBand is the range it falls in. The number
	// is shown because the design asks the window to state it; it is a reading of
	// what happened, never a permission the player bought.
	Affinity     int    `json:"affinity"`
	AffinityBand string `json:"affinity_band"`
	// Relationship is the formal bond that currently stands, if any. It is a
	// separate fact from the number: a high affinity is not a romance, and the
	// window must be able to show a friendship that never became one.
	Relationship *character.Relationship `json:"relationship,omitempty"`
	// Clips are the reviewed clips this window may show for this person, already
	// ordered for the scene on screen. The first one is what the window plays;
	// the rest are the person's other reviewed moments, which the player may
	// step through. It is empty for anyone this build has no reviewed clip for,
	// and the window then keeps the still picture.
	Clips []MediaClip `json:"clips,omitempty"`
}

// MediaClip is one reviewed clip as a window reports it. It carries only what a
// player surface needs to lay the picture out before it has loaded; the file
// itself is fetched from the media route by the URL.
type MediaClip struct {
	URL        string `json:"url"`
	Role       string `json:"role"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
}

// CharacterWindow is the whole window: the scene it is happening in, the cast,
// and the player's own conversation with one of them.
type CharacterWindow struct {
	FactionMemberships []character.FactionMembership `json:"faction_memberships,omitempty"`
	RadioAvailable     bool                          `json:"radio_available"`
	StoryID            string                        `json:"story_id"`
	BranchID           string                        `json:"branch_id"`
	ClockSeconds       int64                         `json:"clock_seconds,omitempty"`
	// CommunicationMode is read from the story's frozen world profile. Empty
	// preserves the established behaviour of stories created before profiles.
	CommunicationMode string `json:"communication_mode,omitempty"`
	// ProtagonistGender is what this story settled for the protagonist, and it is
	// the only basis on which the surface offers the intimacy view. Empty means
	// the story never settled it, so the view is not offered at all rather than
	// offered and then refusing every invitation.
	ProtagonistGender string `json:"protagonist_gender,omitempty"`
	// Scene is where the story currently stands. The window reports it so the
	// player can see that a line spoken here belongs to the scene on screen.
	Scene CharacterWindowScene `json:"scene"`
	// Characters is every character this story knows, in stable order.
	Characters []CharacterWindowCharacter `json:"characters"`
	// CharacterID is the character the window is currently open on.
	CharacterID string `json:"character_id,omitempty"`
	// Exchanges is the player's conversation with CharacterID, oldest first,
	// bounded to the requested page.
	Exchanges []character.Exchange `json:"exchanges,omitempty"`
	// Matters are this character's unresolved threads, so the window can show
	// what is still outstanding rather than making the player remember.
	Matters []character.Matter `json:"matters,omitempty"`
	// Messages are the incoming messages from this character, newest first. They
	// carry their original send time, their deadline and their current status, so
	// an invitation sent a month of world time ago still reads as old rather than
	// as today's news.
	Messages []CharacterWindowMessage `json:"messages,omitempty"`
	// Pending counts the whole cast's messages the player has not dealt with. The
	// stage shows its quiet hint from this, which is why it is not scoped to the
	// selected character.
	Pending []CharacterWindowMessage `json:"pending,omitempty"`
	// HasMore says the conversation continues past this page.
	HasMore bool `json:"has_more"`
	// Cursor pages further back when HasMore is set.
	Cursor string `json:"cursor,omitempty"`
}

// CharacterWindowMessage is one incoming message as the stage list shows it: who
// wrote, what they said, when, and whether it is still live.
type CharacterWindowMessage struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	// Name is the name the player knows the sender by, so the hint reads as a
	// person rather than as an id.
	Name string `json:"name"`
	Text string `json:"text"`
	// Basis is why they wrote, kept alongside the message so the player can judge
	// whether it concerns them.
	Basis     string `json:"basis"`
	WorldTime string `json:"world_time"`
	Deadline  string `json:"deadline,omitempty"`
	Status    string `json:"status"`
	// Lapsed says the message's deadline has passed at the story's current world
	// time. It is computed on read rather than stored, so it stays correct as the
	// story advances without rewriting the message.
	Lapsed bool `json:"lapsed"`
}

// CharacterWindowScene is the scene block the window displays.
type CharacterWindowScene struct {
	NodeID    string   `json:"node_id,omitempty"`
	Title     string   `json:"title,omitempty"`
	Location  string   `json:"location,omitempty"`
	WorldTime string   `json:"world_time,omitempty"`
	Present   []string `json:"present,omitempty"`
	// Place is the registered place this scene stands in, when the story's own
	// frozen catalog has one the location text names. Location is prose the story
	// wrote; Place is the identity a picture may bind to. Empty means the location
	// is not one of the pack's places, and then no picture can be attached to it.
	Place string `json:"place,omitempty"`
	// Image is the place's own picture, when the author supplied one for this
	// world and this registered place. It is empty for a place with no picture,
	// and the window then shows the place as text alone rather than a blank
	// frame. A picture existing does not mean the player has been told anything
	// about the place: it only illustrates where the story already put them.
	Image string `json:"image,omitempty"`
}

// CharacterWindowRequest selects what to read. CharacterID is optional: without
// it the window returns the cast and the scene, which is what the entry point
// needs before the player picks anyone.
type CharacterWindowRequest struct {
	Locale      string
	StoryID     string
	BranchID    string
	CharacterID string
	// BeforeCursor pages backwards through a conversation. Empty starts at the
	// most recent exchanges.
	BeforeCursor string
	Limit        int
}

const (
	defaultCharacterWindowLimit = 40
	maxCharacterWindowLimit     = 200
)

// CharacterWindow reads the character window. It is one read so the cast, the
// scene and the conversation cannot disagree with each other, and it is the
// same projection the main story's character speech is prepared from.
func (a *App) CharacterWindow(request CharacterWindowRequest) (CharacterWindow, error) {
	if a == nil {
		return CharacterWindow{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return CharacterWindow{}, fmt.Errorf("the character window requires a story")
	}
	a.mu.RLock()
	store := a.interactive
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if store == nil {
		return CharacterWindow{}, ErrNoWorkspace
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return CharacterWindow{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return CharacterWindow{}, err
	}
	return a.characterWindowFrom(request, projectID, storyContext, state)
}

// characterWindowFrom only projects the supplied committed facts.
func (a *App) characterWindowFrom(request CharacterWindowRequest, projectID string, storyContext interactive.StoryContext, state character.State) (CharacterWindow, error) {
	storyID, branchID := storyContext.Snapshot.StoryID, storyContext.Snapshot.BranchID
	scene := interactive.StoryScene(storyContext.Snapshot)
	worldTime := characterWorldTime(storyContext)
	// The branch's formal instant, so a message whose deadline was stamped
	// against the clock is decided by that number rather than by its own label.
	clockSeconds := interactive.SnapshotClockSeconds(storyContext.Snapshot)

	pictures := a.characterPictures(storyID, projectID, storyContext, scene)
	window := CharacterWindow{
		FactionMemberships: character.CurrentFactionMemberships(state.FactionMemberships),
		StoryID:            storyID, BranchID: branchID, ClockSeconds: clockSeconds,
		Scene: CharacterWindowScene{
			NodeID: scene.NodeID, Title: scene.Title, Location: scene.Location,
			WorldTime: worldTime, Present: scene.Present,
			Place: pictures.placeName(scene.Location),
			Image: pictures.place(scene.Location),
		},
		Characters:        characterWindowCast(state, scene, pictures),
		Pending:           characterWindowPending(state, worldTime, clockSeconds),
		ProtagonistGender: intimacyProtagonistGender(storyContext),
	}
	if binding := storyContext.Meta.PackBinding; binding != nil && binding.Gameplay != nil {
		window.CommunicationMode = binding.Gameplay.Communication
	}
	// An ordinary window shows a person going about an ordinary day. The private
	// clip belongs to an activity that has already established the adult, consent
	// and privacy conditions, so it is not offered here.
	attachWindowClips(window.Characters, state, pictures, media.RoleGeneral, sceneSeed(scene))

	characterID := strings.TrimSpace(request.CharacterID)
	if characterID == "" {
		return window, nil
	}
	if _, ok := state.Profile(characterID); !ok {
		return CharacterWindow{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, characterID)
	}
	window.CharacterID = characterID
	window.RadioAvailable = radioContactAvailable(storyContext, state, characterID)
	window.Matters = characterWindowMatters(characterID, state.Matters)
	for index := range window.Matters {
		matter := &window.Matters[index]
		if strings.HasPrefix(request.Locale, "en") && matter.StatementTranslations["en-US"] != "" {
			matter.Statement = matter.StatementTranslations["en-US"]
		}
		matter.StatementTranslations = nil
	}
	window.Messages = characterWindowMessages(state, state.MessagesFor(characterID), worldTime, clockSeconds)
	page := characterWindowExchangePage(characterWindowExchanges(state, characterID), request.BeforeCursor, characterWindowLimit(request.Limit))
	window.Exchanges = page.exchanges
	window.HasMore = page.hasMore
	window.Cursor = page.cursor
	return window, nil
}

// characterWindowMatters keeps ordinary open threads visible and retains the
// sent letter plus its linked, committed reply after both have been settled.
// The story journal remains the only source of truth; this is a read projection
// so a delivered letter does not disappear from the character window.
func characterWindowMatters(characterID string, matters []character.Matter) []character.Matter {
	characterID = strings.TrimSpace(characterID)
	letterIDs := make(map[string]struct{})
	for _, matter := range matters {
		matter = matter.Normalize()
		if matter.CharacterID == characterID && matter.Letter != nil {
			letterIDs[matter.ID] = struct{}{}
		}
	}
	visible := make([]character.Matter, 0, len(matters))
	for _, matter := range matters {
		matter = matter.Normalize()
		if matter.CharacterID != characterID {
			continue
		}
		_, linkedLetter := letterIDs[matter.SettledBy]
		if matter.Status == character.MatterOpen || matter.Letter != nil || linkedLetter {
			visible = append(visible, matter)
		}
	}
	return visible
}

// characterWindowPending lists every message the player has not dealt with,
// across the whole cast, newest first. It is what the stage's quiet hint is
// built from, so it deliberately excludes anything already read to the end,
// ignored or handled.
func characterWindowPending(state character.State, worldTime string, clockSeconds int64) []CharacterWindowMessage {
	pending := character.PendingMessages(state.Messages, worldTime)
	out := make([]CharacterWindowMessage, 0, len(pending))
	for _, message := range pending {
		out = append(out, characterWindowMessage(state, message, worldTime, clockSeconds))
	}
	return out
}

// characterWindowMessages presents one character's messages, newest first. The
// order is by world time rather than by commit order, because what the player
// needs to know is when the character wrote, not when the turn that carried it
// happened to be committed.
func characterWindowMessages(state character.State, messages []character.Message, worldTime string, clockSeconds int64) []CharacterWindowMessage {
	ordered := make([]character.Message, 0, len(messages))
	for _, message := range messages {
		ordered = append(ordered, message.Normalize())
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return character.MessageIsNewer(ordered[i], ordered[j])
	})
	out := make([]CharacterWindowMessage, 0, len(ordered))
	for _, message := range ordered {
		out = append(out, characterWindowMessage(state, message, worldTime, clockSeconds))
	}
	return out
}

func characterWindowMessage(state character.State, message character.Message, worldTime string, clockSeconds int64) CharacterWindowMessage {
	message = message.Normalize()
	name := message.CharacterID
	if profile, ok := state.Profile(message.CharacterID); ok {
		name = profile.DisplayName()
	}
	return CharacterWindowMessage{
		ID: message.ID, CharacterID: message.CharacterID, Name: name,
		Text: message.Text, Basis: message.Basis, WorldTime: message.WorldTime,
		Deadline: message.Deadline, Status: string(message.Status),
		Lapsed: message.LapsedAt(worldTime, clockSeconds),
	}
}

// characterLoreImages maps an author key to the picture the world book carries
// for that person's entry.
//
// A person's face has two possible sources. The shipped set answers by author
// key alone and is what every world gets for free. A picture attached to the
// lore entry is the other: it is chosen, generated or uploaded in the lore
// library, and it belongs to this project's entry, not to the author. The
// catalog's own relation — a person row's LoreID — is the only honest way to
// join the two, because the entry ID is not the actor ID and two worlds may each
// hold a person of the same name.
//
// The value is a project-relative asset path, never a filesystem path and never
// a host: the client turns it into a URL for the project it is already reading.
// A read failure yields no pictures rather than failing the window; the roster
// stays usable with its placeholders.
func (a *App) characterLoreImages(projectID string, storyContext interactive.StoryContext) map[string]string {
	binding := storyContext.Meta.PackBinding
	catalog := storyContext.Meta.PackCatalog
	if a == nil || projectID == "" || binding == nil || catalog == nil {
		return nil
	}
	items, err := a.ProjectBook().LoreItems(projectID)
	if err != nil {
		slog.Warn("[character-window] read world book pictures failed", "project_id", projectID, "error", err)
		return nil
	}
	return characterLoreImageSources(binding, catalog, items)
}

// characterLoreImageSources joins the frozen catalog to the world book and
// returns the author key to picture path map. It is pure so the isolation rules
// can be stated without a project on disk.
//
// Two things keep a face from being lent across worlds. The entry is addressed by
// the story's own revision-qualified ID, and it must live in the scope this
// binding names. Either alone would usually be enough; together they mean an
// entry of the same authored ID from another world, or from a later revision of
// this one, cannot supply this cast's picture.
func characterLoreImageSources(binding *interactive.StoryPackBinding, catalog *interactive.PackCatalogSnapshot, items []booklore.Item) map[string]string {
	if binding == nil || catalog == nil {
		return nil
	}
	byID := make(map[string]booklore.Item, len(items))
	scope := strings.TrimSpace(binding.Scope)
	for _, item := range items {
		if !booklore.IsPublic(item) {
			continue
		}
		if scope != "" && strings.TrimSpace(item.Scope) != scope {
			continue
		}
		byID[item.ID] = item
	}
	if len(byID) == 0 {
		return nil
	}
	out := make(map[string]string, len(catalog.Characters))
	for _, person := range catalog.Characters {
		sourceKey := strings.TrimSpace(person.SourceKey)
		loreID := strings.TrimSpace(person.LoreID)
		if sourceKey == "" || loreID == "" {
			continue
		}
		entryID := interactive.PackEntryIDForRevision(loreID, binding.ContentHash)
		if entryID == "" {
			continue
		}
		item, ok := byID[entryID]
		if !ok || item.Image == nil {
			continue
		}
		if path, ok := projectAssetPath(item.Image.ImagePath); ok {
			out[sourceKey] = path
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// projectAssetPath accepts only a project-relative asset path. An absolute path,
// a URL with a scheme or host, or a path that climbs out of the project is not a
// picture this window can show, so it is dropped rather than handed to the
// browser as a src.
func projectAssetPath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`) {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "", false
	}
	for _, segment := range strings.Split(strings.ReplaceAll(parsed.Path, `\`, "/"), "/") {
		if segment == ".." {
			return "", false
		}
	}
	if strings.Contains(parsed.Path, ":") {
		return "", false
	}
	return parsed.Path, true
}

// characterWindowCast lists the story's characters. The player sees the names
// they have actually learned, which is why DisplayName prefers the alias.
//
// A portrait is resolved per row from the person's author key, not from their
// name. The order matters: a picture the player chose is kept even when the
// shipped default would also match, because choosing is the one thing the player
// owns here. A story-invented person has no author key and therefore no default;
// the window draws its placeholder, which is a normal state rather than a fault.
func characterWindowCast(state character.State, scene character.Scene, pictures pictureSources) []CharacterWindowCharacter {
	ids := state.CharacterIDs()
	present := make(map[string]bool, len(scene.Present))
	for _, id := range scene.Present {
		present[strings.TrimSpace(id)] = true
	}
	out := make([]CharacterWindowCharacter, 0, len(ids))
	for _, id := range ids {
		profile, ok := state.Profile(id)
		if !ok {
			continue
		}
		row := CharacterWindowCharacter{
			ID: id, Name: profile.DisplayName(), KnownAs: profile.KnownAs,
			SourceKey:           profile.SourceKey,
			IdentitySource:      "story",
			Lifeform:            profile.Lifeform,
			CultivationMethodID: profile.CultivationMethodID,
			Aliases:             profile.Aliases, Avatar: pictures.avatar(profile, scene),
			AvatarLorePath: pictures.lorePath(profile),
			AvatarDisplay:  pictures.display(profile, scene),
			ImageCaption:   pictures.storyPictures[profile.ID].Caption,
			Role:           profile.Role,
			Description:    profile.Description, Attitude: profile.Attitude,
			Gender:             profile.Gender,
			Tags:               append([]string(nil), profile.Tags...),
			FirstSeenWorldTime: profile.FirstSeenWorldTime,
		}
		if profile.SourceKey != "" {
			row.IdentitySource = "author_catalog"
		}
		value, band := state.AffinityFor(id)
		row.Affinity = value
		row.AffinityBand = string(band)
		if relationship, ok := state.RelationshipFor(id); ok {
			row.Relationship = &relationship
		}
		row.Present = present[id]
		// A character in the scene can be spoken to face to face. One who is not
		// present is reached by message, which is what the proactive channel is
		// for. The window says which, so the player knows what they are doing.
		if row.Present {
			row.Channel = string(character.ChannelScene)
		} else {
			row.Channel = string(character.ChannelMessage)
		}
		for _, message := range state.MessagesFor(id) {
			if message.Status == character.MessageUnread {
				row.UnreadMessages++
			}
		}
		row.OpenMatters = len(character.OpenMatters(id, state.Matters))
		// The list previews the newest line of the same transcript the open
		// conversation draws, so what the list promises is what opening it shows.
		if exchanges := characterWindowExchanges(state, id); len(exchanges) > 0 {
			last := exchanges[len(exchanges)-1]
			row.LastExchange = oneLineText(last.Text)
			row.LastWorldTime = last.WorldTime
			row.LastSequence = exchangeCommitOrdinal(state.Exchanges, last.ID)
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Present != out[j].Present {
			return out[i].Present
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// pictureSources is everything the window may draw one person from, in the
// order the design fixes: the player's own choice, a picture the author bound to
// this scene, the world book's picture for this person, the author's ordinary
// external picture, and only then the shipped default that lives inside the
// binary. A caller assembles it once per window so every row is resolved by the
// same rules.
//
// media is nil in tests and in a build with no media directory; every lookup
// then simply finds nothing and the chain continues to the built-in set.
type pictureSources struct {
	storyManaged  map[string]bool
	storyPictures map[string]storyPicture
	portraits     *portrait.Library
	loreImages    map[string]string
	media         *media.Library
	// world is the pack the story is bound to. An external picture that names a
	// different world never answers.
	world string
	// sceneKey is the scene on screen. A picture bound to it outranks the
	// person's ordinary picture, and one bound to another scene is not a
	// candidate at all.
	sceneKey string
	// places are the places this story may draw, longest first: the world pack's
	// frozen catalog plus the ones this branch registered as it played. A place
	// picture binds to one of these; the scene's location is prose and has to be
	// resolved to one.
	places []placeRef
}

// characterPictures assembles the sources for one window. It reads nothing: the
// binding and the scene are already committed, so this only packages them.
func (a *App) characterPictures(storyID string, projectID string, storyContext interactive.StoryContext, scene character.Scene) pictureSources {
	world := ""
	if binding := storyContext.Meta.PackBinding; binding != nil {
		world = strings.TrimSpace(binding.PackID)
	}
	managed, pictures := a.storyPictures(storyContext)
	return pictureSources{
		storyManaged: managed, storyPictures: pictures,
		portraits:  a.Portraits(),
		loreImages: a.characterLoreImages(projectID, storyContext),
		media:      a.Media(),
		world:      world,
		sceneKey:   sceneSeed(scene),
		places:     placeRefs(storyID, storyContext.Snapshot.State, storyContext.Meta.PackCatalog),
	}
}

// packPlaceNames lists the places this story's frozen catalog registered,
// longest first.
//
// Longest first is what makes a nested name resolve to the more specific place:
// a world that registers both 坊市 and 越国西境坊市 means the latter when the
// story says the player is in 越国西境坊市.
func packPlaceNames(catalog *interactive.PackCatalogSnapshot) []string {
	if catalog == nil {
		return nil
	}
	names := make([]string, 0, len(catalog.Places))
	seen := make(map[string]struct{}, len(catalog.Places))
	for _, entry := range catalog.Places {
		name := strings.Join(strings.Fields(entry.Place), " ")
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	return names
}

// placeName resolves the scene's location text to the registered place it
// names, or "" when it names none.
//
// The location is prose the story wrote — 青石渡口码头 — while a picture binds
// to the registered place — 青石渡口. Matching them by equality would make a
// place's picture appear and disappear as the narrator added a detail, so the
// location is searched for a registered name instead.
func (s pictureSources) placeName(location string) string {
	if ref, ok := s.placeRef(location); ok {
		return ref.Name
	}
	return ""
}

// placeRef is placeName with the story that owns the place. A place the world
// pack registered names no scope and every story in that world draws it; a place
// one story invented names that story, so a sibling story that made up the same
// name does not inherit this picture.
func (s pictureSources) placeRef(location string) (placeRef, bool) {
	name := strings.Join(strings.Fields(location), " ")
	if name == "" {
		return placeRef{}, false
	}
	for _, place := range s.places {
		if strings.Contains(name, place.Name) {
			return place, true
		}
	}
	return placeRef{}, false
}

// external finds an author-supplied picture for this person at one purpose. The
// scene-bound picture and the ordinary one are the same lookup: Picture already
// prefers the scene and refuses a picture bound to a different one.
func (s pictureSources) external(profile character.Profile, purpose string) string {
	if s.storyManaged[profile.SourceKey] {
		return s.storyPictures[profile.ID].URL
	}
	if s.media == nil {
		return ""
	}
	asset, ok := s.media.Picture(media.PictureQuery{
		World: s.world, SourceKey: strings.TrimSpace(profile.SourceKey),
		Scope: strings.TrimSpace(profile.StoryID), Actor: strings.TrimSpace(profile.ID),
		Purpose: purpose, SceneKey: s.sceneKey,
	})
	if !ok {
		return ""
	}
	return media.URL(asset)
}

// lore is the world book's picture for this person, which the client addresses
// through the project it is already reading.
func (s pictureSources) lore(profile character.Profile) string {
	if s.storyManaged[profile.SourceKey] {
		return ""
	}
	key := strings.TrimSpace(profile.SourceKey)
	if key == "" {
		return ""
	}
	return s.loreImages[key]
}

// place is the picture of the place the story already put the player in. It is
// empty for a place with no picture, and the window then shows the place as text
// alone rather than a blank frame. A picture existing does not tell the player
// anything about the place: it only illustrates where the story already stands.
func (s pictureSources) place(location string) string {
	if s.media == nil {
		return ""
	}
	ref, ok := s.placeRef(location)
	if !ok {
		return ""
	}
	asset, ok := s.media.Picture(media.PictureQuery{
		World: s.world, Scope: ref.Scope, Place: ref.Name,
		Purpose: media.PurposeScene, SceneKey: s.sceneKey,
	})
	if !ok {
		return ""
	}
	return media.URL(asset)
}

// avatar decides which picture the list row draws for one person.
//
// A chosen avatar is already a URL. An external picture is served from the media
// route, so it is a ready URL too. A world-book picture is a project-relative
// asset and is reported through AvatarLorePath instead of being pasted into a
// field the client treats as a ready URL. A person with none of these gets the
// shipped default, and a person the story invented gets "" and the window draws
// its placeholder.
func (s pictureSources) avatar(profile character.Profile, scene character.Scene) string {
	// Authored auto_show:false keeps the roster unillustrated; the player
	// opens the gated portrait explicitly.
	if s.storyManaged[profile.SourceKey] {
		return ""
	}
	if chosen := strings.TrimSpace(profile.Avatar); chosen != "" {
		return chosen
	}
	if url := s.external(profile, media.PurposeAvatar); url != "" {
		return url
	}
	if s.lore(profile) != "" {
		return ""
	}
	if s.portraits == nil {
		return ""
	}
	return s.portraits.WorldURL(s.world, profile.SourceKey, false)
}

// lorePath is the world book's picture for this person, when nothing outranks
// it. It stays relative: an absolute path would be one machine's layout.
func (s pictureSources) lorePath(profile character.Profile) string {
	if strings.TrimSpace(profile.Avatar) != "" {
		return ""
	}
	if s.external(profile, media.PurposeAvatar) != "" {
		return ""
	}
	return s.lore(profile)
}

// display is the larger picture for the open conversation.
//
// It asks for the portrait purpose first, which is what an author supplies for a
// full-height figure, and falls back to the shipped display set. A player-chosen
// avatar is whatever they set — an import may be any size — so it is used as-is
// and the window scales it. Returning "" here leaves the small avatar in place
// rather than drawing a broken image.
func (s pictureSources) display(profile character.Profile, scene character.Scene) string {
	if s.storyManaged[profile.SourceKey] {
		return s.storyPictures[profile.ID].URL
	}
	if strings.TrimSpace(profile.Avatar) != "" {
		return ""
	}
	if url := s.external(profile, media.PurposePortrait); url != "" {
		return url
	}
	if url := s.external(profile, media.PurposeAvatar); url != "" {
		return url
	}
	if s.lore(profile) != "" {
		return ""
	}
	if s.portraits == nil {
		return ""
	}
	return s.portraits.WorldURL(s.world, profile.SourceKey, true)
}

// attachWindowClips binds each character's reviewed clips of one role to their
// row, the one chosen for this scene first.
//
// It is a separate step from building the cast because the two surfaces want
// different roles: a person's ordinary window may only show their ordinary-day
// clip, while the private clip belongs to an activity that has already
// established the adult, consent and privacy conditions. Nothing here decides
// eligibility; the caller has already chosen the role it is entitled to.
//
// A person with no clip of that role is left with no clips at all, and the
// window then keeps their still picture. The window never borrows another
// person's clip to fill the gap.
func attachWindowClips(rows []CharacterWindowCharacter, state character.State, pictures pictureSources, role, seed string) {
	library := pictures.media
	if library == nil || len(rows) == 0 {
		return
	}
	for i := range rows {
		profile, ok := state.Profile(rows[i].ID)
		if !ok {
			continue
		}
		rows[i].Clips = windowClipsFor(library, media.PictureQuery{
			World: pictures.world, SourceKey: profile.SourceKey, Scope: profile.StoryID,
			Actor: profile.ID, SceneKey: pictures.sceneKey, Role: role,
		}, seed+"/"+rows[i].ID)
	}
}

// windowClipsFor is the one place a clip set is ordered for a surface: the
// scene's chosen clip first, then the person's other reviewed moments of that
// role. The seed is a stable name, so reopening the same scene shows the same
// clip and a cold restart does not reshuffle it.
func windowClipsFor(library *media.Library, query media.PictureQuery, seed string) []MediaClip {
	if library == nil {
		return nil
	}
	clips := library.ClipsFor(query)
	chosen, ok := media.Pick(clips, seed)
	if !ok {
		return nil
	}
	ordered := make([]MediaClip, 0, len(clips))
	ordered = append(ordered, mediaClipView(chosen))
	for _, clip := range clips {
		if clip.File != chosen.File {
			ordered = append(ordered, mediaClipView(clip))
		}
	}
	return ordered
}

func clipsOfRole(clips []media.Asset, role string) []media.Asset {
	out := make([]media.Asset, 0, len(clips))
	for _, clip := range clips {
		if clip.Role == role {
			out = append(out, clip)
		}
	}
	return out
}

func mediaClipView(clip media.Asset) MediaClip {
	return MediaClip{
		URL: media.URL(clip), Role: clip.Role,
		DurationMS: clip.DurationMS, Width: clip.Width, Height: clip.Height,
	}
}

// sceneSeed is what a clip choice is drawn from: the same scene keeps the same
// clip when the window is reopened, and a different scene shows a different one.
// It is a stable name rather than a counter so it survives a cold reopen.
func sceneSeed(scene character.Scene) string {
	// A new turn is not a new location. Dialogue must not reshuffle a picture
	// or restart its clip merely because the latest TurnID changed.
	return strings.TrimSpace(scene.Location)
}

type characterWindowPage struct {
	exchanges []character.Exchange
	hasMore   bool
	cursor    string
}

// characterWindowExchanges is the transcript the window shows: the conversation
// that actually happened between the player and this character, not the main
// story's own prose.
//
// State.ExchangesFor deliberately returns more than that. A scene line reaches
// every witness, so the narrator's established-character block and the recall
// pass can both read what a character heard in the story. That is memory, and it
// must keep working. The window is a different surface: printing those lines
// again as chat bubbles repeats dialogue the player has just read as prose, and
// it makes the transcript look like the character said the same thing twice.
//
// The filter is therefore made here, on the read, and before the page is cut:
// paging a list that still contains the story's dialogue would spend a page's
// budget on lines the window does not draw, so "load earlier" would appear to
// skip. Only the scene channel is removed. Private exchanges are the window's
// own conversation and message-channel lines were addressed to the player alone,
// so both stay. Nothing is deleted from the journal, and the knowledge records
// that grew out of a scene line are untouched.
func characterWindowExchanges(state character.State, characterID string) []character.Exchange {
	reached := state.ExchangesFor(characterID)
	out := make([]character.Exchange, 0, len(reached))
	for _, exchange := range reached {
		exchange = exchange.Normalize()
		// This window is the one-to-one conversation. Scene speech is the main
		// story's, and a group line belongs to the group's own window even when
		// this character heard it; mixing them here would put the group's
		// conversation into a private transcript.
		if exchange.Channel == character.ChannelScene || exchange.Channel == character.ChannelGroup {
			continue
		}
		out = append(out, exchange)
	}
	return out
}

// exchangeCommitOrdinal is where one committed line sits in the branch journal,
// counting from one. The journal is the branch projection's own append order, so
// this number is stable across a reload and comparable between two
// conversations, which a display time string is not. It returns zero for a line
// that is not in the slice.
func exchangeCommitOrdinal(journal []character.Exchange, exchangeID string) int {
	exchangeID = strings.TrimSpace(exchangeID)
	if exchangeID == "" {
		return 0
	}
	for index, exchange := range journal {
		if exchange.ID == exchangeID {
			return index + 1
		}
	}
	return 0
}

// characterWindowExchangePage returns the most recent exchanges, or the ones
// before a cursor when the player has scrolled back. It pages on the committed
// order rather than on wall-clock time, so a retry or a restart pages the same
// way.
//
// The caller already hands the exchanges in commit order: the branch projection
// appends each record as it is replayed, and ExchangesFor preserves that slice
// order. That order is what the page keeps. It deliberately does not sort by ID:
// IDs are minted as prefix + timestamp + random entropy, so two exchanges
// committed in the same instant differ only in their random tail, and an ID sort
// would shuffle a question and the answer it prompted into an arbitrary order.
func characterWindowExchangePage(exchanges []character.Exchange, beforeCursor string, limit int) characterWindowPage {
	ordered := make([]character.Exchange, 0, len(exchanges))
	for _, exchange := range exchanges {
		ordered = append(ordered, exchange.Normalize())
	}

	before := strings.TrimSpace(beforeCursor)
	if before != "" {
		cut := len(ordered)
		for index, exchange := range ordered {
			if exchange.ID == before {
				cut = index
				break
			}
		}
		ordered = ordered[:cut]
	}
	if limit <= 0 {
		limit = defaultCharacterWindowLimit
	}
	page := characterWindowPage{}
	if len(ordered) <= limit {
		page.exchanges = ordered
		return page
	}
	page.exchanges = ordered[len(ordered)-limit:]
	page.hasMore = true
	page.cursor = page.exchanges[0].ID
	return page
}

func characterWindowLimit(value int) int {
	if value <= 0 {
		return defaultCharacterWindowLimit
	}
	if value > maxCharacterWindowLimit {
		return maxCharacterWindowLimit
	}
	return value
}

// MarkCharacterMessageRead records that the player has read one incoming
// message. Reading is a player action, not a world event: it advances no story
// time and accepts no arrangement.
func (a *App) MarkCharacterMessageRead(storyID, branchID, characterID, messageID string) (character.State, error) {
	return a.markCharacterMessage(storyID, branchID, characterID, messageID, character.MessageRead)
}

// MarkCharacterMessageHandled records that the player has dealt with a message
// outside the window. It is a status change on the message, never an acceptance
// of whatever the message proposed.
func (a *App) MarkCharacterMessageHandled(storyID, branchID, characterID, messageID string) (character.State, error) {
	return a.markCharacterMessage(storyID, branchID, characterID, messageID, character.MessageHandled)
}

// MarkCharacterMessageIgnored records that the player dismissed a message
// without replying. The character still sent it, and the character's own
// expectations are unaffected by the player's UI state.
func (a *App) MarkCharacterMessageIgnored(storyID, branchID, characterID, messageID string) (character.State, error) {
	return a.markCharacterMessage(storyID, branchID, characterID, messageID, character.MessageIgnored)
}

func (a *App) markCharacterMessage(storyID, branchID, characterID, messageID string, status character.MessageStatus) (character.State, error) {
	if a == nil {
		return character.State{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return character.State{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(branchID))
	if err != nil {
		return character.State{}, err
	}
	// The world time stamped on the action is the story's own current time. The
	// player's reading consumes none of it, which is what makes an old message
	// still report its original send time after a week of not playing.
	return store.MarkCharacterMessage(storyID, storyContext.Snapshot.BranchID, characterID, messageID, status, characterWorldTime(storyContext))
}
