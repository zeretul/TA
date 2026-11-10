package interactive

import "ta/internal/character"

// WorldState is every world action the main story's turn can settle, read from
// one journal projection.
//
// The four domains already have their own entry readers, and those stay: an
// interface may still ask about one of them. This exists for the one caller that
// must not read the journal four times — the narrator's per-turn page. It calls
// the same builders the entry readers call, so the page and the entry cannot
// disagree about what is offered, and neither can disagree with the commit,
// which asks the same eligibility predicates.
//
// Nothing here is knowledge the character does not have. The route list is only
// what starts where they stand, the tasks are only what is really offered, the
// events are only what they are placed to hear about, and the realms are only
// what they have entered plus what they could really enter from here.
type WorldState struct {
	FactionMemberships []character.FactionMembership `json:"faction_memberships,omitempty"`
	MembershipContacts []MembershipAdmissionContact  `json:"-"`
	LocalAffairs       []LocalAffairView             `json:"local_affairs,omitempty"`
	FactionAffairs     FactionAffairsView            `json:"faction_affairs"`
	// Location is the scene's committed place, which is what every one of these
	// lists was filtered against.
	Location string `json:"location,omitempty"`
	// Clock is the branch's formal instant, the one a journey or a participation
	// would move.
	Clock StoryClock `json:"clock"`
	// Journeys are the roads walkable from where the character stands, each with
	// its authored duration.
	Journeys JourneyOptions `json:"journeys"`
	// ActiveJourneys are the branch's unfinished travels, keyed by journey id.
	// They are what a turn continues when it names no route.
	ActiveJourneys map[string]string `json:"active_journeys,omitempty"`
	// Tasks are the accepted tasks with their real state, and Available are the
	// authored ones that may still be taken.
	Tasks QuestState `json:"tasks"`
	// Events are the public events this character knows about.
	Events PublicEventState `json:"events"`
	// Realms are the instances this character has entered and the authored realms
	// they could enter from here.
	Realms RealmState `json:"realms"`
}

// WorldStateFor reads every world action surface for one branch.
//
// It takes the store lock once and reads the projection once, because the
// narrator's page is assembled on every turn and four separate reads would both
// cost four journal scans and let the four lists describe four different
// moments.
func (s *Store) WorldStateFor(storyID, branchID string) (WorldState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return WorldState{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return WorldState{}, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return WorldState{}, err
	}
	meta := normalizeStoryMeta(handle.projection.Meta)
	return worldStateFromProjection(projection, meta, actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState))
}

// All current and historical readers share these exact eligibility builders.
func worldStateFromProjection(projection *storyBranchProjection, meta StoryMeta, system StoryDirectorActorStateSystem) (WorldState, error) {
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID, system)
	if err != nil {
		return WorldState{}, err
	}
	state := WorldState{
		LocalAffairs:   localAffairPublicViews(projection.LocalAffairs),
		Location:       storySceneText(projection.State, storySceneLocationField),
		Clock:          clock,
		Journeys:       journeyOptionsFrom(projection),
		Tasks:          questStateFrom(meta, projection, clock),
		Events:         publicEventStateFrom(meta, projection, clock),
		Realms:         realmStateFrom(meta, projection),
		FactionAffairs: factionAffairsView(projection, meta),
	}
	state.FactionMemberships, state.MembershipContacts = membershipContextFrom(projection, meta.PackCatalog)
	// A travel that has not finished is reported by the place it is between, not
	// by its route id alone, because the narrator has to write the character
	// standing on that road.
	for id, snapshot := range projection.Journeys {
		if journeyFinished(snapshot) {
			continue
		}
		if state.ActiveJourneys == nil {
			state.ActiveJourneys = make(map[string]string, len(projection.Journeys))
		}
		state.ActiveJourneys[id] = worldPlaceLabel(projection.worldContent(), snapshot.CurrentLocationID)
	}
	return state, nil
}
