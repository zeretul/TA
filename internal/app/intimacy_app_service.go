package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kaptinlin/jsonrepair"
	agent "ta/agent"
	"ta/agent/providers"
	"ta/config"
	"ta/internal/agents/modelio"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// This file is the intimacy window's service layer.
//
// The shape is the character window's, deliberately: the same story, the same
// branch, the same committed journal, the same character state, the same chat
// container in the UI. What is new is only what the design actually asks for —
// who was invited, what each person answered, and a dedicated model purpose that
// is resolved, budget-checked and called from one configuration.
//
// What is absent on purpose, because §5 and §15 closed it: any reward. There is
// no cultivation multiplier here, no gain record, no "not yet wired" field. The
// exchanges are filed as ordinary group exchanges on the same character record,
// so the next story turn sees them through the cast block with no new plumbing.

// ErrIntimacyModelUnconfigured means the story has no dedicated intimacy model
// bound. §4.1 forbids silently inheriting the main story's model, so this is a
// refusal rather than a fallback: the player is offered the settings entry and
// the invitation itself stays valid.
var ErrIntimacyModelUnconfigured = fmt.Errorf("no intimacy model is configured for this story")

// ErrIntimacyNotInScene means the person is not where the protagonist is. §3.1
// forbids teleporting someone into a window; a person who is elsewhere must be
// met first.
var ErrIntimacyNotInScene = fmt.Errorf("participant is not in the current scene")

// ErrIntimacySceneUnsafe means the scene is one where opening the window would
// pause a danger for free. A fight or a trap is not a place to start.
var ErrIntimacySceneUnsafe = fmt.Errorf("the current scene does not allow this")

// ErrIntimacyNotOpen means the activity is not in progress, so it admits no new
// line.
var ErrIntimacyNotOpen = fmt.Errorf("the activity is not in progress")

// ErrIntimacyUnknownActivity means the id names no activity on this branch.
var ErrIntimacyUnknownActivity = fmt.Errorf("intimacy activity not found")

// ErrIntimacyProtagonistGenderUnknown means this story never settled the
// protagonist's own gender, so "the opposite gender" is not a question that can
// be answered. It is a refusal rather than a pass: the design says 未知不猜, and
// a rule that quietly admits everyone when it cannot read the protagonist is a
// rule that admits everyone.
var ErrIntimacyProtagonistGenderUnknown = fmt.Errorf("this story has not settled the protagonist's gender")

// ErrIntimacySameGender means the person and the protagonist are the same
// gender, which this window does not open for. Both sides are read from what
// each of them was authored as; neither is inferred.
var ErrIntimacySameGender = fmt.Errorf("this invitation is only for someone of the opposite gender to the protagonist")

// IntimacyWindowRequest selects what the intimacy window reads. It is separate
// from GroupWindowRequest because an activity is not a group: the same group id
// is the activity's space, but the activity carries the roster and the phase.
type IntimacyWindowRequest struct {
	StoryID      string
	BranchID     string
	ActivityID   string
	BeforeCursor string
	Limit        int
}

// IntimacyInviteRequest is one invitation, either opening an activity or adding
// a person to one.
type IntimacyInviteRequest struct {
	StoryID  string
	BranchID string
	// ActivityID is empty when this invitation opens the activity.
	ActivityID string
	// CharacterID is the person being invited. A 1:1 activity is one invitation;
	// a multi-person activity is several.
	CharacterID string
	// OneOnOne enters the activity directly once this person agrees, as §3.1
	// requires: no invitation parley, no second start button. The person is still
	// really asked — their own answer is what makes the roster — but the player
	// has already stated the intent by inviting them, so there is nothing left to
	// confirm.
	OneOnOne  bool
	CommandID string
	Locale    string
}

// IntimacyRespondRequest is one invited person's answer. It is produced by the
// dedicated model from that person's own context, and it is the only way a
// person becomes part of a roster.
type IntimacyRespondRequest struct {
	StoryID    string
	BranchID   string
	ActivityID string
	// CharacterID answers for themselves. One person's answer never stands in for
	// another's, which is why this is per person rather than per activity.
	CharacterID string
	CommandID   string
	Locale      string
}

// IntimacyConfirmRequest is the player's own act of starting with a roster.
type IntimacyConfirmRequest struct {
	StoryID    string
	BranchID   string
	ActivityID string
	// ExpectedVersion is the activity version the player was shown. A stale
	// confirmation is refused rather than applied to a roster it never saw.
	ExpectedVersion int
	CommandID       string
	Locale          string
}

// IntimacyEndRequest closes an activity.
type IntimacyEndRequest struct {
	StoryID    string
	BranchID   string
	ActivityID string
	CommandID  string
}

// IntimacySendRequest is one line the player says in an open activity.
type IntimacySendRequest struct {
	StoryID    string
	BranchID   string
	ActivityID string
	Message    string
	CommandID  string
	Locale     string
}

// IntimacyResponse is one committed answer.
type IntimacyResponse struct {
	StoryID    string `json:"story_id"`
	BranchID   string `json:"branch_id"`
	ActivityID string `json:"activity_id"`
	// Activity is the record after the change, so the window redraws from the
	// committed state rather than from what this attempt believed.
	Activity character.IntimacyActivity `json:"activity"`
	// Reply is what this person said when answering, if they said anything. An
	// invitation is answered in words; the words are committed so a refusal is
	// visible rather than a silent status change.
	Reply string `json:"reply,omitempty"`
	// ExchangeIDs are the committed lines, so the window can merge them.
	ExchangeIDs []string `json:"exchange_ids,omitempty"`
}

// IntimacyRound is one committed exchange in an open activity.
type IntimacyRound struct {
	StoryID    string `json:"story_id"`
	BranchID   string `json:"branch_id"`
	ActivityID string `json:"activity_id"`
	// Lines are the exchanges this round committed, in spoken order. They are read
	// back from the journal, so a concurrent commit cannot show the player a line
	// the save does not have.
	Lines     []character.Exchange `json:"lines"`
	WorldTime string               `json:"world_time"`
}

// IntimacyWindowActivity is one activity as the window lists it.
type IntimacyWindowActivity struct {
	character.IntimacyActivity
	// Participants names each invited person as the player knows them, so the
	// roster shows a real name rather than an id.
	ParticipantNames map[string]string `json:"participant_names,omitempty"`
	// Roster is the confirmed roster while the activity is in progress, and the
	// consented roster while it is waiting for confirmation.
	Roster []string `json:"roster,omitempty"`
}

// IntimacyWindow is the whole intimacy surface: the scene, the open activities,
// one activity's conversation, and the dedicated model's display state.
type IntimacyWindow struct {
	StoryID  string           `json:"story_id"`
	BranchID string           `json:"branch_id"`
	Scene    GroupWindowScene `json:"scene"`
	// Activities are every activity this branch holds, oldest first, so a
	// finished one can still be read back.
	Activities []IntimacyWindowActivity `json:"activities,omitempty"`
	// ActivityID is the activity the window is open on, when one was requested.
	ActivityID string `json:"activity_id,omitempty"`
	// Exchanges is the activity's own conversation, oldest first.
	Exchanges []character.Exchange `json:"exchanges,omitempty"`
	// Invitations holds the separate invitation threads of people asked to join an
	// activity that is already running. They are kept apart from Exchanges because
	// those people have not agreed yet and the room is not theirs to hear.
	Invitations []IntimacyInvitationThread `json:"invitations,omitempty"`
	// Participants names every author in the loaded page, including people who
	// declined or have left. A line stays readable after its speaker is gone.
	Participants map[string]string `json:"participants,omitempty"`
	// Cast is the story's known people, so the invitation form offers real
	// characters rather than a free-text name that would create a stranger.
	Cast []CharacterWindowCharacter `json:"cast,omitempty"`
	// Eligible lists the people who may be invited right now: present, adult by
	// their own authored data, and known. It is the list the form should offer,
	// and it is computed by the program rather than by the model.
	Eligible []string `json:"eligible,omitempty"`
	// ProtagonistGender is what this story froze for the protagonist at creation,
	// and it is what the opposite-gender rule reads. Empty means the story never
	// settled it; the surface then offers no intimacy view at all rather than one
	// that would refuse every invitation.
	ProtagonistGender string `json:"protagonist_gender,omitempty"`
	// Model is the dedicated model's display state. It never carries a key.
	Model IntimacyModelState `json:"model"`
	// RosterClips holds the reviewed private clips of the people actually in the
	// open activity, by character id. It is filled only while the activity is
	// running and only for people whose own answer put them in the room, so the
	// clip appears under exactly the conditions the activity itself already
	// required: adult by their own authored data, present, and consented. On
	// every other surface — the roster of an invitation, a person's ordinary
	// window, the character list — the key is absent and the still picture stays.
	RosterClips map[string][]MediaClip `json:"roster_clips,omitempty"`
}

// IntimacyInvitationThread is one person's outstanding invitation into a
// running activity: their answer so far, and what was said between them and the
// protagonist. It is shown apart from the room because they have not agreed to
// join it.
type IntimacyInvitationThread struct {
	CharacterID string                           `json:"character_id"`
	Name        string                           `json:"name,omitempty"`
	Status      character.IntimacyResponseStatus `json:"status"`
	Condition   string                           `json:"condition,omitempty"`
	Exchanges   []character.Exchange             `json:"exchanges,omitempty"`
}

// IntimacyModelState is what the window may show about the dedicated model. §4.1
// requires the credentials to stay out of the UI, the journal, the logs and the
// export; this type therefore has no field for one.
type IntimacyModelState struct {
	Configured bool `json:"configured"`
	// ProfileID, Model, Provider and ThinkingLevel are display facts, so the
	// player can see which model will actually answer.
	ProfileID     string `json:"profile_id,omitempty"`
	Model         string `json:"model,omitempty"`
	Provider      string `json:"provider,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
	// Reason explains why an unconfigured model is unconfigured, in the player's
	// words rather than as an error code.
	Reason string `json:"reason,omitempty"`
}

// intimacyInvitationPayload is the model's structured answer to an invitation.
//
// The model plays the invited person; it does not decide eligibility. Age,
// presence and the roster are the program's, which is why this type carries no
// field for any of them.
type intimacyInvitationPayload struct {
	Reply     string `json:"reply"`
	Status    string `json:"status"`
	Condition string `json:"condition,omitempty"`
}

// intimacyRoundPayload is the model's structured answer for one exchange round.
type intimacyRoundPayload struct {
	Lines []intimacyRoundLine `json:"lines"`
}

// intimacyRoundLine is one speaker's contribution in a round.
//
// The type accepts a bare string as well as an object. A model that writes a
// long answer sometimes flattens the tail of the array into plain strings —
// measured with Gemini, which did it once in eight rounds — and strict decoding
// threw the whole round away over the shape of one item. A string keeps its
// text; the round reassigns it to the speaker above it.
type intimacyRoundLine struct {
	AuthorID string `json:"author_id"`
	Text     string `json:"text"`
}

func (l *intimacyRoundLine) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		l.Text = text
		return nil
	}
	type plain intimacyRoundLine
	var decoded plain
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return err
	}
	*l = intimacyRoundLine(decoded)
	return nil
}

// IntimacyWindow reads the intimacy surface. It is one read, so the roster, the
// model state and the conversation cannot disagree.
func (a *App) IntimacyWindow(request IntimacyWindowRequest) (IntimacyWindow, error) {
	if a == nil {
		return IntimacyWindow{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return IntimacyWindow{}, fmt.Errorf("the intimacy window requires a story")
	}
	a.mu.RLock()
	store, cfg := a.interactive, a.cfg
	projectID := ""
	if cfg != nil {
		projectID = strings.TrimSpace(cfg.ProjectID)
	}
	a.mu.RUnlock()
	if store == nil {
		return IntimacyWindow{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyWindow{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyWindow{}, err
	}
	scene := interactive.StoryScene(storyContext.Snapshot)
	worldTime := characterWorldTime(storyContext)
	pictures := a.characterPictures(storyID, projectID, storyContext, scene)

	window := IntimacyWindow{
		StoryID: storyID, BranchID: branchID,
		Scene: GroupWindowScene{
			NodeID: scene.NodeID, Title: scene.Title, Location: scene.Location,
			WorldTime: worldTime, Present: scene.Present,
		},
		Model: a.intimacyModelState(cfg, storyContext.Meta.IntimacySettings),
		Cast:  characterWindowCast(state, scene, pictures),
	}
	window.ProtagonistGender = intimacyProtagonistGender(storyContext)
	dataDir := ""
	if cfg != nil {
		dataDir = cfg.DataDir()
	}
	window.Eligible = a.intimacyEligible(storyContext, state, scene, dataDir)

	activities := state.IntimacyActivities
	window.Activities = make([]IntimacyWindowActivity, 0, len(activities))
	for _, activity := range activities {
		row := IntimacyWindowActivity{IntimacyActivity: activity, ParticipantNames: make(map[string]string, len(activity.Participants))}
		for _, participant := range activity.Participants {
			name := participant.CharacterID
			if profile, ok := state.Profile(participant.CharacterID); ok {
				name = profile.DisplayName()
			}
			row.ParticipantNames[participant.CharacterID] = name
		}
		if activity.Phase == character.IntimacyActive {
			if group, ok := state.Group(activity.GroupID); ok {
				row.Roster = group.ActiveMembers()
			}
		} else {
			row.Roster = activity.ConsentedRoster()
		}
		window.Activities = append(window.Activities, row)
	}

	activityID := strings.TrimSpace(request.ActivityID)
	if activityID == "" {
		return window, nil
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		return IntimacyWindow{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
	}
	window.ActivityID = activityID
	window.RosterClips = a.intimacyActivityClips(storyContext, state, activity, dataDir)
	// The conversation the window shows is the activity's own space: the running
	// room once it is running, and the invitation parley before that. An
	// invitation parley exists from the first answer, so a multi-person activity
	// shows what each person said while the player is still deciding.
	window.Exchanges = characterWindowExchangePage(state.GroupExchanges(activity.SpaceID()), request.BeforeCursor, characterWindowLimit(request.Limit)).exchanges
	window.Participants = groupParticipants(state, window.Exchanges)
	// A person invited while the activity is running speaks in a space of their
	// own until they agree. Those lines are listed separately so the player can
	// answer them without the room's conversation being handed to someone who has
	// not consented to it.
	for _, participant := range activity.Participants {
		// Everyone who is not in the room has a thread of their own, including a
		// person who declined: their answer stays readable after the no, which is
		// what keeps a refusal a recorded answer rather than a vanished click.
		if participant.Status == character.IntimacyAccepted {
			continue
		}
		spaceID := activity.InvitationSpaceID(participant.CharacterID)
		if spaceID == activity.SpaceID() {
			continue
		}
		pending := state.GroupExchanges(spaceID)
		if len(pending) == 0 {
			continue
		}
		name := participant.CharacterID
		if profile, ok := state.Profile(participant.CharacterID); ok {
			name = profile.DisplayName()
		}
		window.Invitations = append(window.Invitations, IntimacyInvitationThread{
			CharacterID: participant.CharacterID,
			Name:        name,
			Status:      participant.Status,
			Condition:   participant.Condition,
			Exchanges:   pending,
		})
	}
	return window, nil
}

// intimacyModelState resolves the story's dedicated model for display. It never
// resolves the credentials into the returned value, and it never falls back to
// the main story's model: §4.1 requires the binding to be separate.
func (a *App) intimacyModelState(cfg *config.Config, settings interactive.StoryIntimacySettings) IntimacyModelState {
	profileID := strings.TrimSpace(settings.ProfileID)
	if profileID == "" {
		return IntimacyModelState{Reason: "no intimacy model has been chosen for this story"}
	}
	if cfg == nil || !config.ModelProfileExists(cfg, profileID) {
		return IntimacyModelState{ProfileID: profileID, Reason: "the chosen model profile no longer exists"}
	}
	resolved, err := config.ResolveModelProfile(cfg, config.ModelProfileSettings{ID: profileID})
	if err != nil {
		return IntimacyModelState{ProfileID: profileID, Reason: "the chosen model profile cannot be resolved"}
	}
	level := settings.ResolvedThinkingLevel()
	if level == "" {
		level = interactive.IntimacyThinkingDefault
	}
	return IntimacyModelState{
		Configured: true, ProfileID: resolved.ProfileID, Model: resolved.Model,
		Provider: resolved.Provider, ThinkingLevel: string(level),
	}
}

// intimacyProtagonistGender is the protagonist's gender as this story froze it.
//
// It reads the frozen field first. A story created before that field existed has
// none, and is not back-filled from the pack: the body it was created from is
// still on the story, so an explicit statement there is read the same way the
// creation path reads it. A body that states nothing still returns empty, which
// the caller must treat as unknown rather than as a pass.
func intimacyProtagonistGender(storyContext interactive.StoryContext) string {
	if gender := character.NormalizeGender(storyContext.Meta.ProtagonistGender); gender != "" {
		return gender
	}
	return character.NormalizeGender(storypack.ParseGender(storyContext.Meta.Protagonist.Profile))
}

// intimacyEligible lists who may be invited right now. The rule is the design's
// own: known to this story, present where the protagonist is, adult by their own
// authored data, and of the opposite gender to the protagonist.
//
// The adult check reads the story's frozen catalog — the revision this story
// started on, so a pack edited afterwards cannot lower the bar for a story
// already in progress — and brings the authored age forward by the whole years
// this branch has spent, which is the same age the cast block states.
func (a *App) intimacyEligible(storyContext interactive.StoryContext, state character.State, scene character.Scene, dataDir string) []string {
	present := make(map[string]bool, len(scene.Present))
	for _, id := range scene.Present {
		present[strings.TrimSpace(id)] = true
	}
	catalog := intimacyAdultAges(storyContext, dataDir)
	protagonistGender := intimacyProtagonistGender(storyContext)
	if protagonistGender == "" {
		// Nobody is offered while the protagonist's own gender is unsettled. The
		// list is what the form shows, and an empty list is the honest answer to
		// a question this story cannot answer yet.
		return nil
	}
	out := make([]string, 0, len(state.Profiles))
	for _, profile := range state.Profiles {
		profile = profile.Normalize()
		if profile.ID == "" || !present[profile.ID] {
			continue
		}
		if !intimacyProfileIsAdult(profile, catalog) {
			continue
		}
		if !intimacyOppositeGender(protagonistGender, profile.Gender) {
			continue
		}
		// A refusal of physical contact does not remove someone from the list:
		// they are present, adult and of the right gender, and the list is who is
		// here. The refusal is enforced where it would actually land, at the
		// invitation, so the player is told no by the person rather than never
		// seeing them at all.
		out = append(out, profile.ID)
	}
	return out
}

// intimacyBoundsExcludeCloseContact reports whether a character's authored
// refusals rule out physical intimacy altogether.
//
// Bounds are free text and may state the opposite of a refusal — a character can
// be authored as willing. The term alone therefore proves nothing: it counts
// only when a refusal word sits immediately in front of it ("不与人有肌肤之亲",
// "拒绝肌肤之亲"), not when it stands on its own ("愿意与人肌肤之亲"). A bound
// that carries no refusal word is not a refusal and imposes nothing.
func intimacyBoundsExcludeCloseContact(bounds []string) bool {
	for _, bound := range bounds {
		if intimacyBoundRefusesContact(strings.TrimSpace(bound)) {
			return true
		}
	}
	return false
}

func intimacyBoundRefusesContact(text string) bool {
	for _, term := range intimacyContactTerms {
		index := strings.Index(text, term)
		if index < 0 {
			continue
		}
		if containsAnyTerm(intimacyPrefixWindow(text[:index]), intimacyRefusalPrefixes) {
			return true
		}
	}
	return false
}

// intimacyRefusingBound returns the first authored bound that refuses physical
// contact, in the character's own wording, so the invitation can quote it back
// to her rather than paraphrase it into a rule.
func intimacyRefusingBound(bounds []string) string {
	for _, bound := range bounds {
		text := strings.TrimSpace(bound)
		if intimacyBoundRefusesContact(text) {
			return text
		}
	}
	return ""
}

// intimacyPrefixWindow is the stretch in front of the term where a refusal word
// has to sit. Six runes covers "绝不与人" and "从来不与" without reaching back
// across a clause boundary into an unrelated negation.
func intimacyPrefixWindow(prefix string) string {
	runes := []rune(prefix)
	if len(runes) > 6 {
		runes = runes[len(runes)-6:]
	}
	return string(runes)
}

// intimacyRefusalPrefixes are the words that turn the term into a refusal.
var intimacyRefusalPrefixes = []string{
	"绝不", "从不", "从未", "决不", "断不", "不肯", "不愿", "不会", "不能",
	"不敢", "不准", "不许", "不与", "不和", "不跟", "不同", "拒绝", "拒与",
	"休想", "莫想", "不再",
}

// intimacyContactTerms name the refusal itself: what the character will not do
// with their body. Only a refusal of the act belongs here. "以身相许" is a
// promise, not a prohibition; "清白之身" describes a state, not a rule — a
// character can be chaste and still say yes, and reading it as a refusal would
// bar people the story never barred. Vague emotional refusals ("no close
// relationships") stay out for the same reason.
var intimacyContactTerms = []string{
	"肌肤之亲", "肌肤接触", "身体接触", "肢体接触", "房事", "床笫",
	"守身", "不近男色", "不近女色",
}

func containsAnyTerm(text string, terms []string) bool {
	if text == "" {
		return false
	}
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// intimacyOppositeGender is the one comparison both the list and the invitation
// use, so the form can never offer someone the request would refuse.
//
// An empty gender on either side is not "different": a person whose gender the
// story never settled is unknown, and unknown is not a licence to include them.
func intimacyOppositeGender(protagonistGender, personGender string) bool {
	protagonistGender = character.NormalizeGender(protagonistGender)
	personGender = character.NormalizeGender(personGender)
	if protagonistGender == "" || personGender == "" {
		return false
	}
	return protagonistGender != personGender
}

// intimacyAdultAges indexes the story's authored ages by source key, brought
// forward to the branch's formal clock.
//
// The frozen snapshot is read first: it is the revision the story started on, so
// a pack edited afterwards cannot lower the bar for a story already in progress.
// Only a story with no snapshot falls back to the live pack, and only while that
// pack is still the revision the story bound to — the same rule the discovery
// path uses, because it is the same question of which revision this story knows.
//
// The spent interval is added because §2 of 自制-时间寿元与长时世界推进 says
// 年龄按实际游戏时间增加: a person authored at 17 who has lived two years in this
// story is 19, and reading only the frozen number would keep them out of every
// window forever. Ages only ever move up, so the addition can only admit someone
// the story itself has aged past the bar — it cannot lower it.
//
// A nil map means no authored age data could be established at all, and the
// caller must read that as "nobody is proven adult" rather than as "no age, no
// objection".
func intimacyAdultAges(storyContext interactive.StoryContext, dataDir string) map[string]int {
	spent := interactive.WholeYearsAt(storyContext.Meta, interactive.SnapshotClockSeconds(storyContext.Snapshot))
	snapshot := storyContext.Meta.PackCatalog
	ages := map[string]int{}
	if snapshot != nil {
		missing := false
		for _, person := range snapshot.Characters {
			if key := strings.TrimSpace(person.SourceKey); key != "" {
				ages[key] = intimacyAgeAt(person.Age, spent)
				missing = missing || person.Age <= 0
			}
		}
		if !missing {
			return ages
		}
	}
	binding := storyContext.Meta.PackBinding
	if binding == nil || dataDir == "" || (snapshot != nil && binding.ContentHash == "") {
		return ages
	}
	pack, err := storypack.NewLibrary(dataDir).Get(binding.PackID)
	if err != nil || (binding.ContentHash != "" && pack.ContentHash() != binding.ContentHash) {
		return ages
	}
	for key, person := range pack.CatalogBySourceKey() {
		// A legacy snapshot may have missed a stated age. Only the exact same
		// authored revision may fill an existing blank, never replace a fact.
		if age, present := ages[key]; snapshot != nil && (!present || age != 0) {
			continue
		}
		ages[key] = intimacyAgeAt(person.Age, spent)
	}
	return ages
}

// intimacyAgeAt brings an authored age forward by the whole years this branch
// has spent. An age nobody stated stays zero: 未知生日不由AI猜成精确日期, and adding
// years to "unknown" would invent a number rather than age a real one.
func intimacyAgeAt(established, spentYears int) int {
	if established <= 0 || spentYears <= 0 {
		return established
	}
	return established + spentYears
}

// intimacyProfileIsAdult reads established age facts. Authored identities must
// still use the frozen catalog; only locally generated identities use the age
// already committed in their profile. An invitation never supplies an age.
func intimacyProfileIsAdult(profile character.Profile, ages map[string]int) bool {
	sourceKey := strings.TrimSpace(profile.SourceKey)
	if sourceKey == "" {
		return storypack.AdultAge(profile.Age)
	}
	age, ok := ages[sourceKey]
	if !ok {
		return false
	}
	return storypack.AdultAge(age)
}

// InviteIntimacy opens or extends an activity by inviting one person.
//
// The invitation is where the program's checks live: the person must be known,
// present and an established adult, and the dedicated model must be configured.
// Only after those pass is the invited person asked, and only their own answer
// enters the roster.
func (a *App) InviteIntimacy(ctx context.Context, request IntimacyInviteRequest) (IntimacyResponse, error) {
	if a == nil {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	characterID := strings.TrimSpace(request.CharacterID)
	if storyID == "" || characterID == "" {
		return IntimacyResponse{}, fmt.Errorf("an invitation requires a story and a person")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return IntimacyResponse{}, ErrNoWorkspace
	}

	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyResponse{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	profile, ok := state.Profile(characterID)
	if !ok {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, characterID)
	}
	scene := interactive.StoryScene(storyContext.Snapshot)
	if !character.ContainsID(scene.Present, characterID) {
		// The invitation is refused where it would actually be blocked, rather
		// than opening a window on someone who is not there. The player is told
		// which condition failed, not given a silent failure.
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrIntimacyNotInScene, characterID)
	}
	ages := intimacyAdultAges(storyContext, cfg.DataDir())
	if !intimacyProfileIsAdult(profile, ages) {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", character.ErrIntimacyNotAdult, characterID)
	}
	// The gender gate sits beside the adult one and is refused for the same
	// reason: the caller is told which condition failed rather than being given a
	// window that would then behave oddly. A direct API call is refused here even
	// though the window never offers the person.
	protagonistGender := intimacyProtagonistGender(storyContext)
	if protagonistGender == "" {
		return IntimacyResponse{}, ErrIntimacyProtagonistGenderUnknown
	}
	if !intimacyOppositeGender(protagonistGender, profile.Gender) {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrIntimacySameGender, characterID)
	}
	// A refusal of physical contact is answered by the person, in words, like any
	// other no — not by an error the window has to render. The bound is handed to
	// the model as a settled fact so the answer is a refusal in her voice instead
	// of a status change with no one speaking.
	mustDecline := ""
	if intimacyBoundsExcludeCloseContact(profile.Bounds) {
		mustDecline = intimacyRefusingBound(profile.Bounds)
	}
	settings := storyContext.Meta.IntimacySettings
	if !settings.Configured() {
		return IntimacyResponse{}, ErrIntimacyModelUnconfigured
	}

	commandID := strings.TrimSpace(request.CommandID)
	commandHash := intimacyCommandHash("invite", storyID, branchID, strings.TrimSpace(request.ActivityID), characterID, "")

	activity, err := a.intimacyActivityForInvite(store, storyID, branchID, strings.TrimSpace(request.ActivityID), characterID, state)
	if err != nil {
		return IntimacyResponse{}, err
	}
	if commandID != "" {
		// A retry must not ask the person twice. The committed record is the
		// durable answer, so it is read back before anything is generated.
		if committed, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return IntimacyResponse{}, err
		} else if found {
			if refreshed, ok := state.IntimacyActivityFor(activity.ID); ok {
				activity = refreshed
			}
			reply := ""
			exchangeIDs := make([]string, 0, len(committed.Exchanges))
			for _, exchange := range committed.Exchanges {
				exchangeIDs = append(exchangeIDs, exchange.ID)
				if exchange.Speaker == character.SpeakerCharacter && exchange.AuthorID == characterID {
					reply = exchange.Text
				}
			}
			// The answer is already on file. A one-to-one invitation whose answer
			// was yes but whose entry never completed is finished here, so the
			// retry does not strand an accepted activity in a ready state.
			if request.OneOnOne && activity.Phase == character.IntimacyReady {
				if started, err := a.startIntimacy(ctx, cfg, workspace, storyID, branchID, storyContext, activity, 0, characterWorldTime(storyContext),
					commandID+"/start", intimacyCommandHash("start", storyID, branchID, activity.ID, characterID, ""), request.Locale); err == nil {
					return started, nil
				} else {
					slog.WarnContext(ctx, fmt.Sprintf("[intimacy] retry could not enter story_id=%s activity_id=%s err=%v", storyID, activity.ID, err))
				}
			}
			return IntimacyResponse{
				StoryID: storyID, BranchID: branchID, ActivityID: activity.ID,
				Activity: activity, Reply: reply, ExchangeIDs: exchangeIDs,
			}, nil
		}
	}

	worldTime := characterWorldTime(storyContext)
	// The invitation is answered out of the person's own private context: this is
	// a conversation with one person, and nobody else has consented to hear it.
	privateScene := characterSceneFor(storyContext, characterID)
	privateScene.Present = []string{characterID}
	// Whether this person wants to is answered out of how they actually feel
	// about the protagonist, so the same reading the character window uses is
	// supplied here. Without it the answer comes from a stranger's blank slate
	// even when the story has already established something between them.
	affinity, band := state.AffinityFor(characterID)
	var relationship *character.Relationship
	if current, ok := state.RelationshipFor(characterID); ok {
		relationship = &current
	}
	prepared := character.Prepare(character.PrepareInput{
		StoryID:          storyID,
		Profile:          profile,
		Scene:            privateScene,
		Revision:         characterIdentityFor(storyContext, branchID, profile, ""),
		Exchanges:        state.ExchangesFor(characterID),
		Knowledge:        state.KnowledgeFor(characterID),
		Matters:          state.MattersFor(characterID),
		Lore:             characterLoreFor(workspace, packScope(storyContext), profile, privateScene, 0, storyContext),
		Question:         intimacyInvitationQuestion(activity, state, characterID),
		Affinity:         affinity,
		AffinityBand:     band,
		Relationship:     relationship,
		RecentAffinities: character.RecentAffinityChanges(characterID, state.Affinities, recentAffinityLimit),
		// The branch's formal instant, so a thread whose deadline was bound to the
		// clock is split by that number rather than by its own label.
		ClockSeconds: interactive.SnapshotClockSeconds(storyContext.Snapshot),
	})
	systemInstruction := prompts.BuildIntimacyInvitationInstruction(prompts.IntimacyInvitationInput{
		Workspace:           workspace,
		CharacterContext:    prepared.Markdown(),
		StoryContext:        characterStoryContext(storyContext, privateScene, profile),
		ActivityNote:        intimacyActivityNote(activity, state, characterID),
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		OutputLanguage:      request.Locale,
		MustDecline:         mustDecline,
	})

	reply, err := a.generateIntimacyReply(ctx, cfg, storyID, branchID, settings, systemInstruction,
		intimacyInvitationQuestion(activity, state, characterID), "invite", "")
	if err != nil {
		return IntimacyResponse{}, err
	}
	payload, err := parseIntimacyInvitationPayload(reply)
	if err != nil {
		return IntimacyResponse{}, err
	}

	next, err := activity.AddParticipant(characterID, true, worldTime)
	if err != nil {
		return IntimacyResponse{}, err
	}
	next = next.WithResponse(characterID, character.IntimacyResponseStatus(payload.Status), payload.Condition, worldTime)
	next.SourceEventID = "" // assigned by the store

	// The invitation's words are filed in a space whose membership is exactly who
	// may hear them, so a refusal is readable later and the person's answer is part
	// of a conversation rather than a status change with no words.
	//
	// A one-to-one invitation is filed on the private channel instead. §3.4
	// allows the single-target case to reuse the private semantics, and it is the
	// truthful audience here: nobody else was asked and nobody else may hear it.
	group := intimacyInvitationGroup(next, state, characterID, worldTime)
	groups := []character.Group{group}
	// A person invited into a running activity is admitted the moment they
	// actually agree. Until then they are in the invitation space only, so the
	// room's conversation never reaches someone who has not consented to it.
	//
	// An acceptance is spoken in the room, because agreeing is what brings them
	// into it; a no or a condition stays between the protagonist and them.
	acceptedIntoRoom := false
	if next.Phase == character.IntimacyActive {
		if participant, ok := next.Participant(characterID); ok && participant.Status == character.IntimacyAccepted {
			room := intimacyRunningGroup(next, state, worldTime)
			groups = append(groups, room)
			group = room
			acceptedIntoRoom = true
		}
	}
	facts := character.TurnCharacterFacts{}
	// A group line's audience is its members who are present in the scene the
	// commit names. The invitation is a conversation among exactly the people in
	// its space, so that is the scene it is resolved against; the private channel
	// needs no such list and keeps the person's own private scene.
	audienceScene := privateScene
	channel := character.ChannelGroup
	if request.OneOnOne && !acceptedIntoRoom {
		channel = character.ChannelPrivate
	}
	if channel == character.ChannelGroup {
		audienceScene.Present = append([]string{}, group.ActiveMembers()...)
	}
	if strings.TrimSpace(payload.Reply) != "" {
		draft := character.ExchangeDraft{
			CharacterID: characterID, Speaker: character.SpeakerCharacter,
			Channel: channel, GroupID: group.ID, Text: payload.Reply,
		}
		if channel == character.ChannelPrivate {
			draft.GroupID = ""
		}
		facts.Exchanges = []character.ExchangeDraft{draft}
	}
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:           branchID,
		WorldTime:          worldTime,
		Scene:              audienceScene,
		Groups:             groups,
		IntimacyActivities: []character.IntimacyActivity{next},
		CommandID:          commandID,
		CommandHash:        commandHash,
		Facts:              facts,
	})
	if err != nil {
		return IntimacyResponse{}, err
	}
	refreshed, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	committed, _ := refreshed.IntimacyActivityFor(next.ID)
	exchangeIDs := make([]string, 0, len(projected.Exchanges))
	for _, exchange := range projected.Exchanges {
		exchangeIDs = append(exchangeIDs, exchange.ID)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[intimacy] invitation committed story_id=%s activity_id=%s character_id=%s status=%s", storyID, next.ID, characterID, payload.Status))
	response := IntimacyResponse{
		StoryID: storyID, BranchID: branchID, ActivityID: next.ID,
		Activity: committed, Reply: strings.TrimSpace(payload.Reply), ExchangeIDs: exchangeIDs,
	}
	// §3.1: a valid one-to-one invitation enters the activity directly. The
	// person has now really answered yes, and the player's own act of inviting
	// them is the intent a second button would only repeat. So the roster is
	// confirmed here, and the opening is generated in the same request. A refusal
	// or a conditional answer stops here and is reported as it was given.
	if request.OneOnOne && payload.Status == string(character.IntimacyAccepted) {
		// Entering is a second write in this same request, so it needs its own
		// command identity. Reusing the invitation's would make the store read the
		// confirm as a replay of the invitation it just committed and return that
		// record instead: the roster would stay ready and the opening would be
		// silently dropped. The retry path derives the same "start" identity, so a
		// lost response can still finish the entry without a second opening.
		started, err := a.startIntimacy(ctx, cfg, workspace, storyID, branchID, storyContext, committed, 0, worldTime,
			commandID+"/start", intimacyCommandHash("start", storyID, branchID, committed.ID, characterID, ""), request.Locale)
		if err != nil {
			// The invitation itself is committed. Reporting the failure leaves it
			// readable and lets the player retry entering, rather than making the
			// answer they already gave disappear.
			return response, err
		}
		return started, nil
	}
	return response, nil
}

// intimacyActivityForInvite resolves the activity an invitation belongs to, or
// opens a new one. A new activity starts with the person already invited; the
// caller applies the answer.
func (a *App) intimacyActivityForInvite(store *interactive.Store, storyID, branchID, activityID, characterID string, state character.State) (character.IntimacyActivity, error) {
	if activityID != "" {
		activity, ok := state.IntimacyActivityFor(activityID)
		if !ok {
			return character.IntimacyActivity{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
		}
		return activity, nil
	}
	// Opening a second activity while one is running would split the roster and
	// the conversation. The window offers to resume instead.
	if open := state.OpenIntimacyActivities(); len(open) > 0 {
		return open[0], nil
	}
	scene := character.Scene{NodeID: branchID, Present: []string{characterID}}
	return character.IntimacyActivity{
		ID:      newIntimacyActivityID(storyID, branchID, characterID, state.IntimacyActivities),
		StoryID: storyID, BranchID: branchID,
		Phase: character.IntimacySelecting, Scene: scene,
	}.Normalize(), nil
}

// intimacyInvitationGroup is the conversation space one invitation is spoken
// in. It is a real group whose membership is who may hear a line, so the
// audience rule that keeps one group's talk out of another's memory applies
// here unchanged.
//
// During the invitation phase every invitee still in the conversation is in the
// activity's shared parley group: that is the design's invitation conversation,
// where each person answers separately but in one exchange. Someone who has
// declined is left out of later parley, because a no ends their part in the
// invitation and the later parley is not theirs to hear. The answer that ends
// their part, though, was spoken in this space, so the speaker is kept in it for
// that one commit: otherwise an invitation everyone declined would have no room
// to record the refusals, and the answer would fail to commit at all.
//
// A running activity is not that. §13.1 requires a person added mid-activity to
// be invited first and admitted only once they agree, so their invitation gets
// a space of its own and the room is left exactly as it is. Joining them to the
// room now would let them hear the activity before consenting, which is the one
// thing the audience rule exists to prevent.
func intimacyInvitationGroup(activity character.IntimacyActivity, state character.State, characterID, worldTime string) character.Group {
	activity = activity.Normalize()
	characterID = strings.TrimSpace(characterID)
	group := intimacySpaceGroup(activity, state, "双修 · 邀请", worldTime)
	if activity.Phase == character.IntimacyActive {
		if characterID != "" && character.ContainsID(group.ActiveMembers(), characterID) {
			// Already in the room: a further answer belongs to it.
			return group
		}
		return character.Group{
			ID:      activity.ID + ":invite:" + characterID,
			StoryID: activity.StoryID, BranchID: activity.BranchID,
			Type: character.GroupOther, Name: "双修 · 邀请",
			Channel: character.ChannelScene, CreatedWorldTime: worldTime,
		}.Join(characterID, worldTime)
	}
	// A no ends the person's part in the parley, so they are taken out of it for
	// every later answer. Their own line was already spoken while they were here,
	// and its audience was captured then, so leaving now costs them nothing.
	for _, participant := range activity.Participants {
		if participant.Status == character.IntimacyDeclined && participant.CharacterID != characterID {
			group = group.Leave(participant.CharacterID, worldTime)
		}
	}
	for _, participant := range activity.Participants {
		if participant.Status == character.IntimacyDeclined {
			continue
		}
		group = group.Join(participant.CharacterID, worldTime)
	}
	if len(group.ActiveMembers()) == 0 && characterID != "" {
		// An invitation everyone declined still happened, and this answer has to
		// be spoken somewhere. A space with no member would be rejected on write,
		// which would turn a real refusal into a technical failure.
		group = group.Join(characterID, worldTime)
	}
	return group
}

// intimacySpaceGroup resolves an activity's conversation space, creating it
// when this is the first time it is written. The id is the activity's own, so a
// retry lands on the same space instead of opening a second one.
func intimacySpaceGroup(activity character.IntimacyActivity, state character.State, name, worldTime string) character.Group {
	activity = activity.Normalize()
	if activity.GroupID != "" {
		if group, ok := state.Group(activity.GroupID); ok {
			return group
		}
	}
	return character.Group{
		ID:      activity.SpaceID(),
		StoryID: activity.StoryID, BranchID: activity.BranchID,
		Type: character.GroupOther, Name: name,
		Channel: character.ChannelScene, CreatedWorldTime: worldTime,
	}
}

// intimacyRunningGroup is the running room with the consented roster in it and
// everyone else out. A person's own yes admits them at that moment; anyone who
// has not agreed stays outside, and a no keeps them outside for good.
func intimacyRunningGroup(activity character.IntimacyActivity, state character.State, worldTime string) character.Group {
	activity = activity.Normalize()
	group := intimacySpaceGroup(activity, state, "双修", worldTime)
	roster := activity.ConsentedRoster()
	for _, id := range roster {
		group = group.Join(id, worldTime)
	}
	for _, id := range group.ActiveMembers() {
		if !character.ContainsID(roster, id) {
			group = group.Leave(id, worldTime)
		}
	}
	return group
}

// RespondIntimacy records one invited person's answer. It is used when a person
// was invited without an answer yet, or when a conditional answer's condition has
// since been settled and they are asked again.
func (a *App) RespondIntimacy(ctx context.Context, request IntimacyRespondRequest) (IntimacyResponse, error) {
	if a == nil {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	activityID := strings.TrimSpace(request.ActivityID)
	characterID := strings.TrimSpace(request.CharacterID)
	if storyID == "" || activityID == "" || characterID == "" {
		return IntimacyResponse{}, fmt.Errorf("an answer requires a story, an activity and a person")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyResponse{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
	}
	if _, invited := activity.Participant(characterID); !invited {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, characterID)
	}
	settings := storyContext.Meta.IntimacySettings
	if !settings.Configured() {
		return IntimacyResponse{}, ErrIntimacyModelUnconfigured
	}
	profile, ok := state.Profile(characterID)
	if !ok {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrCharacterUnknown, characterID)
	}

	commandID := strings.TrimSpace(request.CommandID)
	commandHash := intimacyCommandHash("respond", storyID, branchID, activityID, characterID, "")
	if commandID != "" {
		if _, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return IntimacyResponse{}, err
		} else if found {
			if refreshed, ok := state.IntimacyActivityFor(activityID); ok {
				activity = refreshed
			}
			return IntimacyResponse{StoryID: storyID, BranchID: branchID, ActivityID: activityID, Activity: activity}, nil
		}
	}

	worldTime := characterWorldTime(storyContext)
	privateScene := characterSceneFor(storyContext, characterID)
	privateScene.Present = []string{characterID}
	question := intimacyInvitationQuestion(activity, state, characterID)
	affinity, band := state.AffinityFor(characterID)
	var relationship *character.Relationship
	if current, ok := state.RelationshipFor(characterID); ok {
		relationship = &current
	}
	prepared := character.Prepare(character.PrepareInput{
		StoryID:          storyID,
		Profile:          profile,
		Scene:            privateScene,
		Revision:         characterIdentityFor(storyContext, branchID, profile, ""),
		Exchanges:        state.ExchangesFor(characterID),
		Knowledge:        state.KnowledgeFor(characterID),
		Matters:          state.MattersFor(characterID),
		Lore:             characterLoreFor(workspace, packScope(storyContext), profile, privateScene, 0, storyContext),
		Question:         question,
		Affinity:         affinity,
		AffinityBand:     band,
		Relationship:     relationship,
		RecentAffinities: character.RecentAffinityChanges(characterID, state.Affinities, recentAffinityLimit),
		// The branch's formal instant, so a thread whose deadline was bound to the
		// clock is split by that number rather than by its own label.
		ClockSeconds: interactive.SnapshotClockSeconds(storyContext.Snapshot),
	})
	systemInstruction := prompts.BuildIntimacyInvitationInstruction(prompts.IntimacyInvitationInput{
		Workspace:           workspace,
		CharacterContext:    prepared.Markdown(),
		StoryContext:        characterStoryContext(storyContext, privateScene, profile),
		ActivityNote:        intimacyActivityNote(activity, state, characterID),
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		OutputLanguage:      request.Locale,
	})
	reply, err := a.generateIntimacyReply(ctx, cfg, storyID, branchID, settings, systemInstruction, question, "respond", "")
	if err != nil {
		return IntimacyResponse{}, err
	}
	payload, err := parseIntimacyInvitationPayload(reply)
	if err != nil {
		return IntimacyResponse{}, err
	}

	next := activity.WithResponse(characterID, character.IntimacyResponseStatus(payload.Status), payload.Condition, worldTime)
	group := intimacyInvitationGroup(next, state, characterID, worldTime)
	groups := []character.Group{group}
	// An answer given to a running activity's invitation admits the person when it
	// is a yes, and only then. A no or a condition leaves the room exactly as it
	// was, so nothing said in it reaches someone who has not agreed: their answer
	// stays in their own invitation thread, which is what the window lists it in.
	//
	// The yes is spoken in the room, not in the invitation thread. Agreeing is
	// what brings them into it, and the people already there are standing in the
	// scene the answer happens in. Filing it against the thread instead leaves the
	// room's own conversation missing the moment its newest member joined, and the
	// others cannot recall a thing they were present for. This is the same
	// resolution InviteIntimacy does; the two entry points differ only in which
	// button the player pressed, so they must not differ in who hears it (§13.3:
	// 活动消息明确记录每条受众、当时成员版本).
	if next.Phase == character.IntimacyActive {
		if participant, ok := next.Participant(characterID); ok && participant.Status == character.IntimacyAccepted {
			group = intimacyRunningGroup(next, state, worldTime)
			groups = append(groups, group)
		}
	}
	// A group line's audience is its members who are present in the scene the
	// commit names. The space it is filed in is a conversation among exactly the
	// people in it, so that is the scene it is resolved against.
	audienceScene := privateScene
	audienceScene.Present = append([]string{}, group.ActiveMembers()...)
	facts := character.TurnCharacterFacts{}
	if strings.TrimSpace(payload.Reply) != "" {
		facts.Exchanges = []character.ExchangeDraft{{
			CharacterID: characterID, Speaker: character.SpeakerCharacter,
			Channel: character.ChannelGroup, GroupID: group.ID, Text: payload.Reply,
		}}
	}
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:           branchID,
		WorldTime:          worldTime,
		Scene:              audienceScene,
		Groups:             groups,
		IntimacyActivities: []character.IntimacyActivity{next},
		CommandID:          commandID,
		CommandHash:        commandHash,
		Facts:              facts,
	})
	if err != nil {
		return IntimacyResponse{}, err
	}
	refreshed, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	committed, _ := refreshed.IntimacyActivityFor(activityID)
	exchangeIDs := make([]string, 0, len(projected.Exchanges))
	for _, exchange := range projected.Exchanges {
		exchangeIDs = append(exchangeIDs, exchange.ID)
	}
	return IntimacyResponse{
		StoryID: storyID, BranchID: branchID, ActivityID: activityID,
		Activity: committed, Reply: strings.TrimSpace(payload.Reply), ExchangeIDs: exchangeIDs,
	}, nil
}

// ConfirmIntimacy is the player's own act of starting with the consented roster.
// It is required even when everyone accepted: §3.3 keeps the confirmation rather
// than auto-starting.
func (a *App) ConfirmIntimacy(ctx context.Context, request IntimacyConfirmRequest) (IntimacyResponse, error) {
	if a == nil {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	activityID := strings.TrimSpace(request.ActivityID)
	if storyID == "" || activityID == "" {
		return IntimacyResponse{}, fmt.Errorf("confirmation requires a story and an activity")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyResponse{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
	}
	settings := storyContext.Meta.IntimacySettings
	if !settings.Configured() {
		return IntimacyResponse{}, ErrIntimacyModelUnconfigured
	}
	commandID := strings.TrimSpace(request.CommandID)
	commandHash := intimacyCommandHash("confirm", storyID, branchID, activityID, "", fmt.Sprintf("%d", request.ExpectedVersion))
	if commandID != "" {
		if _, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return IntimacyResponse{}, err
		} else if found {
			if refreshed, ok := state.IntimacyActivityFor(activityID); ok {
				activity = refreshed
			}
			return IntimacyResponse{StoryID: storyID, BranchID: branchID, ActivityID: activityID, Activity: activity}, nil
		}
	}

	worldTime := characterWorldTime(storyContext)
	return a.startIntimacy(ctx, cfg, workspace, storyID, branchID, storyContext, activity, request.ExpectedVersion, worldTime, commandID, commandHash, request.Locale)
}

// startIntimacy commits the roster the player confirmed and generates the
// activity's one opening.
//
// It is the shared half of both entry points. A multi-person activity reaches it
// from the player's confirmation button; a 1:1 activity reaches it straight from
// the person's acceptance, because §3.1 says a valid one-to-one invitation
// enters the activity directly rather than opening a parley. Both therefore
// commit the same record and generate the same single opening, and neither can
// generate a second one.
//
// The opening is a separate commit from the roster on purpose. If generation
// fails, the roster and the room are already saved, so the player can retry the
// opening instead of redoing consent that already happened. A retry reuses the
// command identity, so the opening is written at most once.
func (a *App) startIntimacy(ctx context.Context, cfg *config.Config, workspace, storyID, branchID string, storyContext interactive.StoryContext, activity character.IntimacyActivity, expectedVersion int, worldTime, commandID, commandHash, locale string) (IntimacyResponse, error) {
	store := a.interactive
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	if current, ok := state.IntimacyActivityFor(activity.ID); ok {
		activity = current
	}
	// The confirmed roster is the room's active membership. Everyone who declined
	// or is still negotiating is left out, so no later line can reach them.
	group := intimacyRunningGroup(activity, state, worldTime)
	next, err := activity.Confirm(group.ID, worldTime, expectedVersion)
	if err != nil {
		return IntimacyResponse{}, err
	}
	roster := next.ConsentedRoster()
	next.Scene.Present = append([]string{}, roster...)
	next.Scene.WorldTime = worldTime

	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:           branchID,
		WorldTime:          worldTime,
		Scene:              next.Scene,
		Groups:             []character.Group{group},
		IntimacyActivities: []character.IntimacyActivity{next},
		CommandID:          commandID,
		CommandHash:        commandHash,
	})
	if err != nil {
		return IntimacyResponse{}, err
	}

	// The opening is generated once, on confirmation, and never replayed. A
	// restored activity re-reads the committed opening instead of generating a
	// second one.
	opening, err := a.generateIntimacyOpening(ctx, cfg, storyID, branchID, storyContext, state, next, storyContext.Meta.IntimacySettings, workspace, locale)
	if err != nil {
		return IntimacyResponse{}, err
	}
	if len(opening) > 0 {
		opened, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
			BranchID:  branchID,
			WorldTime: worldTime,
			Scene:     next.Scene,
			Facts:     character.TurnCharacterFacts{Exchanges: opening},
		})
		if err != nil {
			return IntimacyResponse{}, err
		}
		projected.Exchanges = append(projected.Exchanges, opened.Exchanges...)
	}

	refreshed, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	committed, _ := refreshed.IntimacyActivityFor(activity.ID)
	exchangeIDs := make([]string, 0, len(projected.Exchanges))
	for _, exchange := range projected.Exchanges {
		exchangeIDs = append(exchangeIDs, exchange.ID)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[intimacy] started story_id=%s activity_id=%s roster=%d opening_lines=%d", storyID, activity.ID, len(roster), len(opening)))
	return IntimacyResponse{
		StoryID: storyID, BranchID: branchID, ActivityID: activity.ID,
		Activity: committed, ExchangeIDs: exchangeIDs,
	}, nil
}

// generateIntimacyOpening produces the activity's first exchange: one line from
// each confirmed participant, in the confirmed space. It is generated once.
func (a *App) generateIntimacyOpening(ctx context.Context, cfg *config.Config, storyID, branchID string, storyContext interactive.StoryContext, state character.State, activity character.IntimacyActivity, settings interactive.StoryIntimacySettings, workspace, locale string) ([]character.ExchangeDraft, error) {
	group, ok := state.Group(activity.GroupID)
	if !ok {
		return nil, fmt.Errorf("the activity's conversation space is missing")
	}
	roster := activity.ConsentedRoster()
	members := make([]prompts.GroupChatMember, 0, len(roster))
	for _, id := range roster {
		profile, ok := state.Profile(id)
		if !ok {
			continue
		}
		members = append(members, prompts.GroupChatMember{
			ID: id, Name: profile.DisplayName(), Role: profile.Role,
			Description: profile.Description, Personality: profile.Personality,
			Gender:      profile.Gender,
			SpeechStyle: profile.SpeechStyle, Motives: profile.Motives,
			Attitude: profile.Attitude, Bounds: profile.Bounds, CanSpeak: true,
		})
	}
	systemInstruction := prompts.BuildIntimacyRoundInstruction(prompts.IntimacyRoundInput{
		Workspace:           workspace,
		Members:             members,
		ProtagonistGender:   intimacyProtagonistGender(storyContext),
		SceneContext:        groupSceneContext(storyContext, activity.Scene),
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		Opening:             true,
		OutputLanguage:      locale,
		Model:               intimacyModelName(cfg, settings),
	})
	question := "The player has confirmed this roster and the activity is beginning. Write the opening: each person's first move and first words as the scene begins. The player has not spoken yet. 开场就是这场戏的开头本身，不要停在寒暄、对视或过渡上；人物可以主动靠近、开口、动手。"
	seed := ""
	if len(members) > 0 {
		if value, ok := prompts.IntimacyIgnitionSeed(intimacyModelName(cfg, settings), members[0]); ok {
			seed = value
		}
	}
	reply, err := a.generateIntimacyReply(ctx, cfg, storyID, branchID, settings, systemInstruction, question, "opening", seed)
	if err != nil {
		return nil, err
	}
	payload, err := parseIntimacyRoundPayload(reply, roster)
	if err != nil {
		return nil, err
	}
	drafts := make([]character.ExchangeDraft, 0, len(payload.Lines))
	for _, line := range payload.Lines {
		drafts = append(drafts, character.ExchangeDraft{
			CharacterID: line.AuthorID, Speaker: character.SpeakerCharacter,
			Channel: character.ChannelGroup, GroupID: group.ID, Text: line.Text,
		})
	}
	return drafts, nil
}

// SendIntimacyMessage is one line the player says, answered by the confirmed
// roster. It mirrors the group round: the few with a reason to answer speak.
func (a *App) SendIntimacyMessage(ctx context.Context, request IntimacySendRequest) (IntimacyRound, error) {
	if a == nil {
		return IntimacyRound{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	activityID := strings.TrimSpace(request.ActivityID)
	message := strings.TrimSpace(request.Message)
	if storyID == "" || activityID == "" || message == "" {
		return IntimacyRound{}, fmt.Errorf("a message requires a story, an activity and text")
	}
	a.mu.RLock()
	store, cfg, workspace := a.interactive, a.cfg, a.workspace
	a.mu.RUnlock()
	if store == nil || cfg == nil || workspace == "" {
		return IntimacyRound{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyRound{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyRound{}, err
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		return IntimacyRound{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
	}
	if !activity.Open() {
		return IntimacyRound{}, fmt.Errorf("%w: activity is %s", ErrIntimacyNotOpen, activity.Phase)
	}
	settings := storyContext.Meta.IntimacySettings
	if !settings.Configured() {
		return IntimacyRound{}, ErrIntimacyModelUnconfigured
	}
	group, ok := state.Group(activity.GroupID)
	if !ok {
		return IntimacyRound{}, fmt.Errorf("the activity's conversation space is missing")
	}
	roster := group.ActiveMembers()
	if len(roster) == 0 {
		return IntimacyRound{}, fmt.Errorf("the activity has nobody in it")
	}

	commandID := strings.TrimSpace(request.CommandID)
	commandHash := intimacyCommandHash("send", storyID, branchID, activityID, "", message)
	worldTime := characterWorldTime(storyContext)
	if commandID != "" {
		if committed, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return IntimacyRound{}, err
		} else if found {
			return intimacyRoundFrom(storyID, branchID, activityID, committed, worldTime), nil
		}
	}

	members := make([]prompts.GroupChatMember, 0, len(roster))
	for _, id := range roster {
		profile, ok := state.Profile(id)
		if !ok {
			continue
		}
		members = append(members, prompts.GroupChatMember{
			ID: id, Name: profile.DisplayName(), Role: profile.Role,
			Description: profile.Description, Personality: profile.Personality,
			Gender:      profile.Gender,
			SpeechStyle: profile.SpeechStyle, Motives: profile.Motives,
			Attitude: profile.Attitude, Bounds: profile.Bounds, CanSpeak: true,
		})
	}
	systemInstruction := prompts.BuildIntimacyRoundInstruction(prompts.IntimacyRoundInput{
		Workspace:           workspace,
		Members:             members,
		ProtagonistGender:   intimacyProtagonistGender(storyContext),
		SceneContext:        groupSceneContext(storyContext, activity.Scene),
		JointHistory:        groupJointHistory(state, group, roster),
		DialogueStylePrompt: interactiveapp.ResolveStoryStyleFragments(cfg.DataDir(), storyContext.Meta).Dialogue,
		OutputLanguage:      request.Locale,
		Model:               intimacyModelName(cfg, settings),
	})
	// Every round is prefilled for the models that need it, not just the
	// opening. Handed a document that has already begun, the model keeps going;
	// handed nothing, it re-approaches the scene and the register slips back a
	// step per round. The seed for a later round is only the JSON opening — the
	// content is the model's own, continued from the history above it.
	replySeed := ""
	if len(members) > 0 && prompts.IntimacyNeedsPrefill(intimacyModelName(cfg, settings)) {
		replySeed = prompts.IntimacyPrefillOpen(members[0].ID)
	}
	reply, err := a.generateIntimacyReply(ctx, cfg, storyID, branchID, settings, systemInstruction, message, "reply", replySeed)
	if err != nil {
		return IntimacyRound{}, err
	}
	payload, err := parseIntimacyRoundPayload(reply, roster)
	if err != nil {
		return IntimacyRound{}, err
	}

	// The player's line and the roster's answers commit together, so a reply can
	// never be stored without the message it answers.
	drafts := make([]character.ExchangeDraft, 0, len(payload.Lines)+1)
	drafts = append(drafts, character.ExchangeDraft{
		CharacterID: roster[0], Speaker: character.SpeakerPlayer,
		Channel: character.ChannelGroup, GroupID: group.ID, Text: message,
	})
	for _, line := range payload.Lines {
		drafts = append(drafts, character.ExchangeDraft{
			CharacterID: line.AuthorID, Speaker: character.SpeakerCharacter,
			Channel: character.ChannelGroup, GroupID: group.ID, Text: line.Text,
		})
	}
	projected, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:    branchID,
		WorldTime:   worldTime,
		Scene:       activity.Scene,
		Groups:      []character.Group{group},
		CommandID:   commandID,
		CommandHash: commandHash,
		Facts:       character.TurnCharacterFacts{Exchanges: drafts},
	})
	if err != nil {
		return IntimacyRound{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[intimacy] round committed story_id=%s activity_id=%s lines=%d", storyID, activityID, len(projected.Exchanges)))
	return intimacyRoundFrom(storyID, branchID, activityID, projected, worldTime), nil
}

// EndIntimacy closes an activity. The conversation stays readable.
func (a *App) EndIntimacy(ctx context.Context, request IntimacyEndRequest) (IntimacyResponse, error) {
	if a == nil {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyID := strings.TrimSpace(request.StoryID)
	activityID := strings.TrimSpace(request.ActivityID)
	if storyID == "" || activityID == "" {
		return IntimacyResponse{}, fmt.Errorf("ending an activity requires a story and an activity")
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return IntimacyResponse{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return IntimacyResponse{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	activity, ok := state.IntimacyActivityFor(activityID)
	if !ok {
		return IntimacyResponse{}, fmt.Errorf("%w: %s", ErrIntimacyUnknownActivity, activityID)
	}
	commandID := strings.TrimSpace(request.CommandID)
	commandHash := intimacyCommandHash("end", storyID, branchID, activityID, "", "")
	if commandID != "" {
		if _, found, err := store.CharacterExchangesForCommand(storyID, branchID, commandID, commandHash); err != nil {
			return IntimacyResponse{}, err
		} else if found {
			if refreshed, ok := state.IntimacyActivityFor(activityID); ok {
				activity = refreshed
			}
			return IntimacyResponse{StoryID: storyID, BranchID: branchID, ActivityID: activityID, Activity: activity}, nil
		}
	}
	worldTime := characterWorldTime(storyContext)
	next := activity.End(worldTime)
	if _, err := store.AppendCharacterFacts(storyID, interactive.AppendCharacterFactsRequest{
		BranchID:           branchID,
		WorldTime:          worldTime,
		Scene:              next.Scene,
		IntimacyActivities: []character.IntimacyActivity{next},
		CommandID:          commandID,
		CommandHash:        commandHash,
	}); err != nil {
		return IntimacyResponse{}, err
	}
	refreshed, err := store.CharacterState(storyID, branchID)
	if err != nil {
		return IntimacyResponse{}, err
	}
	committed, _ := refreshed.IntimacyActivityFor(activityID)
	slog.InfoContext(ctx, fmt.Sprintf("[intimacy] ended story_id=%s activity_id=%s", storyID, activityID))
	return IntimacyResponse{StoryID: storyID, BranchID: branchID, ActivityID: activityID, Activity: committed}, nil
}

// intimacyRoundFrom rebuilds the caller's view of a committed round. The lines
// are read back from the journal, so a retry or a lost race answers with what the
// save actually has.
func intimacyRoundFrom(storyID, branchID, activityID string, committed character.Projected, worldTime string) IntimacyRound {
	lines := make([]character.Exchange, 0, len(committed.Exchanges))
	for _, exchange := range committed.Exchanges {
		lines = append(lines, exchange.Normalize())
	}
	if len(lines) > 0 {
		worldTime = lines[len(lines)-1].WorldTime
	}
	return IntimacyRound{StoryID: storyID, BranchID: branchID, ActivityID: activityID, Lines: lines, WorldTime: worldTime}
}

// parseIntimacyInvitationPayload reads one person's answer.
//
// The status is the person's own: the model plays them and says yes, no, or yes
// if something. Anything it did not clearly state is treated as a refusal, not
// as consent — silence is never a yes, which is the design's rule and also the
// safe reading of a malformed answer.
func parseIntimacyInvitationPayload(content string) (intimacyInvitationPayload, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	text = strings.TrimSpace(text)
	if text == "" {
		return intimacyInvitationPayload{}, fmt.Errorf("the person returned an empty answer")
	}
	if !strings.HasPrefix(text, "{") {
		// Plain prose is a real answer in words but not a stated consent. It is
		// filed as a refusal rather than guessed into an acceptance.
		return intimacyInvitationPayload{Reply: text, Status: string(character.IntimacyDeclined)}, nil
	}
	var payload intimacyInvitationPayload
	if err := unmarshalIntimacyPayload(text, &payload); err != nil {
		return intimacyInvitationPayload{}, fmt.Errorf("the person returned an unreadable answer: %w", err)
	}
	payload.Reply = strings.TrimSpace(payload.Reply)
	payload.Condition = strings.TrimSpace(payload.Condition)
	switch strings.ToLower(strings.TrimSpace(payload.Status)) {
	case string(character.IntimacyAccepted):
		payload.Status = string(character.IntimacyAccepted)
	case string(character.IntimacyConditional):
		payload.Status = string(character.IntimacyConditional)
	case string(character.IntimacyDeclined):
		payload.Status = string(character.IntimacyDeclined)
	default:
		payload.Status = string(character.IntimacyDeclined)
	}
	if payload.Status == string(character.IntimacyConditional) && payload.Condition == "" {
		// A condition nobody stated cannot be settled, so it is not consent. It is
		// filed as a refusal with the words kept.
		payload.Status = string(character.IntimacyDeclined)
	}
	if payload.Reply == "" {
		payload.Reply = defaultIntimacyAnswer(payload.Status)
	}
	return payload, nil
}

// unmarshalIntimacyPayload decodes one model answer. Valid output takes the
// standard library's strict path; malformed output gets the same deterministic
// syntax repair the agent already applies to tool arguments and must then
// decode. A missing comma is a formatting fault, not the person refusing, and
// the design keeps technical faults apart from an answer.
func unmarshalIntimacyPayload(text string, target any) error {
	if err := json.Unmarshal([]byte(text), target); err == nil {
		return nil
	}
	repaired, err := jsonrepair.Repair(text)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(repaired), target)
}

func defaultIntimacyAnswer(status string) string {
	switch status {
	case string(character.IntimacyAccepted):
		return "（点了点头。）"
	case string(character.IntimacyConditional):
		return "（没有立刻答应。）"
	default:
		return "（摇了摇头。）"
	}
}

// parseIntimacyRoundPayload reads one round's lines. A line from someone not in
// the roster is dropped rather than committed: the model may not add a stranger
// to the room.
func parseIntimacyRoundPayload(content string, roster []string) (intimacyRoundPayload, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	text = strings.TrimSpace(text)
	if text == "" {
		return intimacyRoundPayload{}, fmt.Errorf("the round returned an empty answer")
	}
	if !strings.HasPrefix(text, "{") {
		return intimacyRoundPayload{}, fmt.Errorf("the round returned no readable lines")
	}
	var payload intimacyRoundPayload
	if err := unmarshalIntimacyPayload(text, &payload); err != nil {
		return intimacyRoundPayload{}, fmt.Errorf("the round returned an unreadable answer: %w", err)
	}
	lines := make([]intimacyRoundLine, 0, len(payload.Lines))
	for _, line := range payload.Lines {
		authorID := strings.TrimSpace(line.AuthorID)
		lineText := strings.TrimSpace(line.Text)
		if lineText == "" {
			continue
		}
		if !character.ContainsID(roster, authorID) {
			// A flattened tail carries text but no speaker. Dropping it would
			// lose the end of the scene, so it continues the line above.
			if len(lines) > 0 {
				lines[len(lines)-1].Text += "\n" + lineText
			}
			continue
		}
		lines = append(lines, intimacyRoundLine{AuthorID: authorID, Text: lineText})
	}
	if len(lines) == 0 {
		return intimacyRoundPayload{}, fmt.Errorf("the round named nobody in the room")
	}
	if len(lines) > prompts.MaxGroupSpeakersPerRound {
		lines = lines[:prompts.MaxGroupSpeakersPerRound]
	}
	payload.Lines = lines
	return payload, nil
}

// generateIntimacyReply performs the dedicated model's call.
//
// §7 requires assembly, budget check and the actual call to use one resolved
// configuration. They do: ResolveModelProfile yields the settings, the thinking
// level from the story's own setting is forced onto them, the budget check uses
// that same resolved context window, and the call is built from the same value.
// The main story's model is never consulted, which is what §4.1 forbids.
// intimacyModelName resolves the profile the story picked to the model id that
// will actually be called. It returns "" when the setting is missing or cannot
// be resolved, and the prompt then keeps the generic line target.
//
// It is resolved on every round rather than cached, so switching the intimacy
// profile in the game changes the next round's length target immediately.
func intimacyModelName(cfg *config.Config, settings interactive.StoryIntimacySettings) string {
	profileID := strings.TrimSpace(settings.ProfileID)
	if profileID == "" || !config.ModelProfileExists(cfg, profileID) {
		return ""
	}
	resolved, err := config.ResolveModelProfile(cfg, config.ModelProfileSettings{ID: profileID})
	if err != nil {
		return ""
	}
	return resolved.Model
}

func (a *App) generateIntimacyReply(ctx context.Context, cfg *config.Config, storyID, branchID string, settings interactive.StoryIntimacySettings, systemInstruction, message, action, seed string) (string, error) {
	profileID := strings.TrimSpace(settings.ProfileID)
	if profileID == "" {
		return "", ErrIntimacyModelUnconfigured
	}
	if !config.ModelProfileExists(cfg, profileID) {
		return "", fmt.Errorf("%w: profile %q does not exist", ErrIntimacyModelUnconfigured, profileID)
	}
	resolved, err := config.ResolveModelProfile(cfg, config.ModelProfileSettings{ID: profileID})
	if err != nil {
		return "", fmt.Errorf("resolve intimacy model configuration: %w", err)
	}
	level := settings.ResolvedThinkingLevel()
	if level == "" {
		level = interactive.IntimacyThinkingDefault
	}
	// §4.3: the story's saved thinking choice is what is actually sent. The
	// default is off, and an unsupported level is refused at save time rather
	// than quietly dropped here.
	resolved.ThinkingLevel = string(level)
	modelCfg, err := modelio.ConfigFromResolved(resolved)
	if err != nil {
		return "", fmt.Errorf("build intimacy model configuration: %w", err)
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindInteractiveStory, "intimacy", cfg.Workspace,
		"intimacy", "Intimacy Conversation Rules",
		"define how a private adult intimacy scene is written",
		systemInstruction,
	)
	if err != nil {
		return "", err
	}
	instruction := composition.Instruction()
	// The composed instruction is model-agnostic. The addendum corrects the one
	// habit the resolved model has that the shared text cannot reach; it is
	// empty for models it has nothing to say about. Chosen from resolved.Model
	// on every call, so the game's model switch needs no restart.
	if adapter := prompts.IntimacyModelAdapter(resolved.Model); adapter != "" {
		instruction += adapter
	}
	messages := []*agent.Message{
		agent.SystemMessage(instruction),
		agent.UserMessage(message),
	}
	// The opening is seeded rather than generated from nothing. Some models write
	// the body of a scene but never the sentence that enters it, and a full round
	// of scene followed by a retreat reads worse than no scene at all. Given the
	// first sentence of a document that has already crossed that line, the same
	// model continues without hesitating, so the seed is the smallest thing that
	// makes the rest possible. It is put in the assistant channel with prefix
	// completion, and the reply comes back as the whole document.
	//
	// The seeded call does not go through the provider runtime. Replayed
	// assistant messages are normalised there, and the prefix flag does not
	// survive the trip: measured against the live endpoint the model answered
	// the user turn from scratch and invented a speaker. JSON mode has the same
	// effect even on a bare request, so a seeded call also runs without
	// response_format; the prefix already pins the shape of the document.
	contextSettings := config.ResolveAgentContext(cfg, config.AgentKindInteractiveStory)
	if seed != "" {
		if err := modelio.ValidateInput(config.AgentKindInteractiveStory, modelCfg, messages, nil,
			contextSettings.MaxProviderInputBytes, resolved.ContextWindowTokens); err != nil {
			return "", err
		}
		return a.callIntimacyModelSeeded(ctx, cfg, modelCfg, storyID, branchID, action, message, messages, seed)
	}
	// The budget check uses the same resolved profile: its own context window and
	// the same input-byte ceiling the call is subject to. Checking against the
	// main story's limits and then calling this model would be the split §7
	// forbids.
	if err := modelio.ValidateInput(config.AgentKindInteractiveStory, modelCfg, messages, nil,
		contextSettings.MaxProviderInputBytes, resolved.ContextWindowTokens); err != nil {
		return "", err
	}
	content, err := a.callIntimacyModel(ctx, cfg, modelio.WithJSONObjectOutput(modelCfg), storyID, branchID, action, message, messages)
	// An empty body is not a usable answer. Some providers blank the content
	// instead of returning an error when the structured call is filtered, so
	// treating "no error" as success would fail the turn at the parser without
	// ever trying the plain call that works.
	if err == nil && strings.TrimSpace(content) != "" {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	slog.WarnContext(ctx, fmt.Sprintf("[intimacy] json_mode unusable, retry without response_format err=%v content_chars=%d", err, len(strings.TrimSpace(content))))
	return a.callIntimacyModel(ctx, cfg, modelCfg, storyID, branchID, action, message, messages)
}

func (a *App) callIntimacyModel(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, storyID, branchID, action, message string, messages []*agent.Message) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "intimacy", action, map[string]any{
		"story_id": storyID, "branch_id": branchID, "message_chars": len([]rune(message)),
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	chatModel, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create intimacy model: %w", err)
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "intimacy", action, modelCfg, messages, nil, false)
	msg, err := chatModel.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "intimacy", action, modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate intimacy reply: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("the intimacy model returned no response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "intimacy", action, modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "intimacy", action, modelCfg.Model, 0, msg, nil, nil)
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = strings.TrimSpace(msg.ReasoningContent)
	}
	return content, nil
}

// callIntimacyModelSeeded runs the seeded variant of a reply: the seed is sent
// as the assistant's half-written turn with prefix completion on, so the model
// finishes a document instead of starting one. Tracing matches the ordinary
// call, minus the runtime that would have normalised the request.
func (a *App) callIntimacyModelSeeded(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, storyID, branchID, action, message string, messages []*agent.Message, seed string) (string, error) {
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindInteractiveStory, "intimacy", action, map[string]any{
		"story_id": storyID, "branch_id": branchID, "message_chars": len([]rune(message)), "seeded": true,
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindInteractiveStory, "intimacy", action, modelCfg, messages, nil, false)
	content, err := modelio.SeededCall(llmTraceCtx, modelCfg, messages, seed)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "intimacy", action, modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("generate seeded intimacy reply: %w", err)
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindInteractiveStory, "intimacy", action, modelCfg.Model, 0, nil, nil, nil)
	return strings.TrimSpace(content), nil
}

// intimacyCommandHash fingerprints the player action a command ID stands for.
func intimacyCommandHash(action, storyID, branchID, activityID, characterID, extra string) string {
	payload := strings.Join([]string{action, storyID, branchID, activityID, characterID, extra}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// intimacyInvitationQuestion is what the invited person is asked. It states the
// activity as it stands, so a second invitation is a different question from the
// first.
//
// The other invitees are named the way the story names them. An actor id is a
// storage key, not a name anyone in the scene has ever heard, and a person asked
// to stand in a room with "lin" would be answering about a stranger. 名单须显示
// 实际姓名 (自制-双修窗口 §3.3).
func intimacyInvitationQuestion(activity character.IntimacyActivity, state character.State, characterID string) string {
	var sb strings.Builder
	sb.WriteString("The protagonist is inviting you to a private adult intimacy scene (双修). ")
	if others := activity.Invited(); len(others) > 1 {
		names := make([]string, 0, len(others))
		for _, id := range others {
			if id == characterID {
				continue
			}
			names = append(names, intimacyCharacterName(state, id))
		}
		if len(names) > 0 {
			fmt.Fprintf(&sb, "Others who have been invited: %s. ", strings.Join(names, ", "))
		}
	}
	sb.WriteString("Answer as yourself, in your own words, and say clearly whether you agree. ")
	sb.WriteString("You may refuse, or agree only if some condition is met. If you agree only on a condition, state the condition plainly.")
	return sb.String()
}

// intimacyActivityNote describes the roster as it currently stands, for a person
// being asked or re-asked.
//
// Each line is an answer in words, not the stored value. `pending` and
// `accepted` are protocol tokens; written raw into the prompt they read as a
// field dump, and a model handed a status column tends to narrate the column.
// The wording also has to keep three answers apart that the design keeps apart:
// 未回应者保持未回应，模型超时或连接失败保持技术错误；两者都不等于拒绝 (§3.3),
// and a conditional yes is still being negotiated rather than consent (§13.1).
// The condition itself is left out: it was said to the protagonist, and the
// prompt promises this person is told nothing private about the others.
func intimacyActivityNote(activity character.IntimacyActivity, state character.State, characterID string) string {
	var sb strings.Builder
	activity = activity.Normalize()
	for _, participant := range activity.Participants {
		if participant.CharacterID == characterID {
			continue
		}
		fmt.Fprintf(&sb, "- %s: %s\n", intimacyCharacterName(state, participant.CharacterID), intimacyAnswerWording(participant.Status))
	}
	return strings.TrimSpace(sb.String())
}

// intimacyCharacterName is the name the story uses for a person. It falls back
// to the id only when no profile is loaded, which is the one case where the id
// is all there is to say.
func intimacyCharacterName(state character.State, characterID string) string {
	if profile, ok := state.Profile(characterID); ok {
		return profile.DisplayName()
	}
	return characterID
}

// intimacyAnswerWording states one answer the way a person would hear it.
func intimacyAnswerWording(status character.IntimacyResponseStatus) string {
	switch status {
	case character.IntimacyAccepted:
		return "has agreed"
	case character.IntimacyDeclined:
		return "said no"
	case character.IntimacyConditional:
		return "agreed, but only once a condition is met, and that is still being settled"
	default:
		// Not an answer yet. It is never written as a refusal: the person may
		// simply not have been reached.
		return "has not answered yet"
	}
}

// packScope is the world resource identity every lore read for this story is
// filtered by.
func packScope(storyContext interactive.StoryContext) string {
	if binding := storyContext.Meta.PackBinding; binding != nil {
		return binding.Scope
	}
	return ""
}

// newIntimacyActivityID mints an id from the story, branch, first person and
// the activities already on file. The count is what keeps a later invitation
// from reusing an activity that has already ended: an id keyed only on the
// person would name the finished record, and the new answer would be written
// over the old one instead of opening a new activity.
//
// The count is stable for a retried request — the first attempt has not been
// committed yet, so the retry reads the same number — which is what keeps an
// empty activity ID in a retry opening the same activity rather than a second.
func newIntimacyActivityID(storyID, branchID, characterID string, existing []character.IntimacyActivity) string {
	ordinal := fmt.Sprintf("%d", len(existing))
	sum := sha256.Sum256([]byte(strings.Join([]string{storyID, branchID, characterID, ordinal}, "\x00")))
	return "itm_" + hex.EncodeToString(sum[:8])
}
