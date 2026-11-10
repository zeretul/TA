package interactive

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ta/internal/exploration"
	"ta/internal/storytime"
)

// StateOpSourceJourney marks an Actor State operation a committed journey
// produced.
//
// It is its own source kind for the same reason a rest and a trade have one: a
// reader of the journal can tell a place the rules moved the character to from a
// place a narrative described, and a re-settlement can discard its own previous
// write without touching anything else.
const StateOpSourceJourney = "journey"

// StoryEventTypeJourneyRecord is a side record on the canonical story journal.
// It carries one real stretch of travel — the route, the segment progress, the
// stop the character reached and the in-fiction time it took — without creating
// a story Turn and without consuming a turn number.
//
// Three properties are deliberate, and they are the same three a rest has.
//
//   - There is no second clock. The record carries the branch's formal clock
//     before and after, and the interval it moved is exactly the interval the
//     route rules paid for. §4.2 of 自制-探索与行路 binds the elapsed time to a
//     concrete connection, so the two numbers are one number.
//
//   - The journey snapshot is the idempotency. §4.2 requires that only the
//     distance really walked is committed at a stop and that the continuation
//     does not charge it again, and that a retry adds nothing. All three fall
//     out of the snapshot's cumulative ElapsedMinutes being the only input
//     AdvanceTo reads: the same target produces AddedMinutes zero, and a
//     continuation starts from the committed offset rather than from the origin.
//
//   - The record and the place write are one transaction. A journey either
//     exists in the journal with the place it moved to or does not exist at all,
//     so a lost response cannot leave the character charged for a road the story
//     does not show them having walked.
type JourneyRecordEvent struct {
	CompanionSources []JourneyCompanionInput `json:"companion_sources,omitempty"`
	V                int                     `json:"v"`
	Type             string                  `json:"type"`
	ID               string                  `json:"id"`
	ParentID         string                  `json:"parent_id"`
	BranchID         string                  `json:"branch_id"`
	Ts               string                  `json:"ts"`
	// TurnID is the main-story turn this stretch belongs to. It is empty when
	// the travel was settled from the journey entry rather than inside a turn,
	// which is a legitimate way to move without a turn.
	TurnID string `json:"turn_id,omitempty"`
	// Cursor is the canonical commit position this record was projected at.
	Cursor string `json:"cursor,omitempty"`
	// CommandID is the caller-owned idempotency key this record was written
	// under, and CommandHash fingerprints the content behind it.
	CommandID   string `json:"command_id,omitempty"`
	CommandHash string `json:"command_hash,omitempty"`
	// WorldTime is the story's own display text for this moment. It is a label;
	// every calculation uses ClockAfter, which is the formal axis.
	WorldTime string `json:"world_time,omitempty"`
	// RouteID and RouteVersion name the registered connection. The snapshot
	// carries them too; they are repeated here so a reader can find the record
	// without decoding a nested object.
	RouteID      string `json:"route_id"`
	RouteVersion string `json:"route_version"`
	// Journey is the whole journey after this stretch: its origin, its
	// cumulative elapsed minutes and the segment it currently stands in. It is
	// what makes a stop, a continuation and a retry all read the same number.
	Journey exploration.JourneySnapshot `json:"journey"`
	// Position is where this stretch ended: a stop, or an offset inside a
	// segment. A mid-segment stop deliberately has no location, because the
	// character is on the road between two places and claiming otherwise would
	// teleport them to the last stop they left.
	Position exploration.JourneyPosition `json:"position"`
	// AddedMinutes is the distance this stretch really covered. A retry and a
	// continuation past an already-committed target both report zero here.
	AddedMinutes int64 `json:"added_minutes"`
	// Arrived reports that this stretch reached the route's destination.
	Arrived bool `json:"arrived"`
	// StoppedAtLifespan reports that this stretch was cut short because the
	// protagonist's confirmed lifespan edge fell inside it, so the road is not
	// finished and the character is standing at the brink rather than past it.
	//
	// It is recorded rather than only logged because the next turn has to be able
	// to say why the character is still on the road: without it a branch reads as
	// having simply stopped walking, and the narrator has no reason to write the
	// moment §4 asks for.
	StoppedAtLifespan bool `json:"stopped_at_lifespan,omitempty"`
	// LifespanEdgeSeconds is the instant the cut stopped short of, on the same
	// axis as ClockAfter. Zero when the stretch was not cut.
	//
	// The edge is stored rather than recomputed because the realm may change
	// before the record is read, and a rebuilt branch must report the edge the
	// player was actually shown rather than one derived from a later breakthrough.
	LifespanEdgeSeconds int64 `json:"lifespan_edge_seconds,omitempty"`
	// ClockBefore and ClockAfter are the branch's formal time axis around this
	// stretch. ClockAfter - ClockBefore.Seconds is the in-fiction time the travel
	// really consumed, and the journey owns no other clock.
	ClockBefore StoryClock `json:"clock_before"`
	ClockAfter  StoryClock `json:"clock_after"`
}

// ErrJourneyUnknownRoute means the named route is not registered. Guessing a
// connection would put the character somewhere no authored data places them.
var ErrJourneyUnknownRoute = errors.New("journey route is not registered")

// ErrJourneyUnknownJourney means a continuation named a journey this branch does
// not have. Guessing a replacement would apply the remainder to a different road.
var ErrJourneyUnknownJourney = errors.New("journey is not on this branch")

// ErrJourneyCommandConflict means a command ID was reused for different
// content. Refusing is the only honest answer: accepting would either lose the
// first stretch or grant the second one under a false identity.
var ErrJourneyCommandConflict = errors.New("journey command identity conflict")

// ErrJourneyNoProgress means the request asked for an instant the journey is
// already at. Committing a record that adds nothing would show the player a road
// they did not walk, so it is refused instead.
var ErrJourneyNoProgress = errors.New("journey is already at the requested point")

// ErrJourneyAtLifespanEdge means the protagonist's confirmed lifespan ends
// before the road does, so there is no stretch of travel left to settle.
//
// It is separate from ErrJourneyNoProgress on purpose: the player did not ask to
// stand still, and telling them the road is already at the requested point would
// describe the wrong reason. What happened is that the years ran out, which is a
// fact about the character rather than about the route.
var ErrJourneyAtLifespanEdge = errors.New("the road outlasts the protagonist's remaining lifespan")

// ErrJourneyNotAtOrigin means the story places the character somewhere other
// than the route's origin. §4.1 starts from the current formal position rather
// than from whichever route the caller named, so a mismatch is refused rather
// than silently teleporting.
var ErrJourneyNotAtOrigin = errors.New("character is not at the route's origin")

// ErrJourneyInsideRealm refuses a new journey while the character is in a realm.
var ErrJourneyInsideRealm = errors.New("character is inside a realm")

// JourneyRequest is one player request to walk a stretch of a registered route.
//
// The caller supplies the route and the point to reach. It does not supply the
// elapsed minutes the clock should move: those are the route's own duration for
// the distance actually covered, which is what makes §4.2's "经过时间按既有路线
// 尺度" a property of the data rather than of the request.
type JourneyRequest struct {
	// Only the main-turn planner supplies sources after validating the actual
	// participant and their own public response. Window requests remain solo.
	companionSources []JourneyCompanionInput
	BranchID         string
	TurnID           string
	// WorldTime is the story's display text for this moment. Empty leaves the
	// caller's scene text alone.
	WorldTime string
	// RouteID names the connection to walk. Empty continues the branch's single
	// unfinished journey, which is what "继续赶路" means without repeating the
	// destination.
	RouteID string
	// JourneyID continues a journey a real stop interrupted. Empty starts a new
	// one when the branch has no unfinished journey, and continues the one it has
	// otherwise.
	JourneyID string
	// TargetMinutes is the absolute elapsed minute to reach, counted from the
	// route's origin. A stop at 10 on a 30-minute route covers 10; the
	// continuation to 30 then covers only the remaining 20.
	TargetMinutes int64
	// Arrive asks to walk to the route's destination, which is the ordinary
	// "去坊市" case: §4.2 lets one turn complete the whole ordinary journey
	// rather than clicking through each half-day.
	Arrive bool
	// CommandID identifies one attempt. It is the caller's, because only the
	// caller knows that a retry is the same attempt rather than a new one.
	CommandID   string
	CommandHash string
}

// JourneyResult is what a committed stretch reports back.
type JourneyResult struct {
	EventID      string
	JourneyID    string
	RouteID      string
	RouteVersion string
	Journey      exploration.JourneySnapshot
	Position     exploration.JourneyPosition
	ClockAfter   StoryClock
	// AddedMinutes is the distance this stretch really covered, and
	// ElapsedSeconds the same interval on the formal axis.
	AddedMinutes   int64
	ElapsedSeconds int64
	Arrived        bool
	Remaining      int64
	// StoppedAtLifespan reports that the requested stretch was cut short by the
	// protagonist's confirmed lifespan edge, so the road is unfinished and the
	// character stands at the brink. The caller says so rather than reporting a
	// shorter journey for no stated reason.
	StoppedAtLifespan bool
	// LifespanEdgeSeconds is the instant the cut stopped short of. Zero when the
	// stretch was not cut.
	LifespanEdgeSeconds int64
	// LocationLabel is the place the character now stands at, empty when the
	// stop is mid-segment and the character is between places.
	LocationLabel string
	// SettlementOps are the Actor State writes this commit produced, so the
	// caller can report what moved without re-deriving it.
	SettlementOps []ActorStateOp
	// Replayed reports that this result came from a stretch already committed
	// under the same command. The caller can then say that nothing was added
	// rather than presenting a second journey that never happened.
	Replayed bool
}

// ApplyJourney settles one stretch of travel and commits it as a side record
// plus its place write in a single journal transaction.
//
// The append is idempotent on CommandID. A retry after a lost response, a
// repeated click or a reopened window returns what the first attempt committed,
// so the same road is never paid for twice.
func (s *Store) ApplyJourney(storyID string, req JourneyRequest) (JourneyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return JourneyResult{}, err
	}
	defer releaseStory()

	meta, records, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return JourneyResult{}, err
	}
	return s.applyJourneyLocked(storyID, meta, records, req)
}

// applyJourneyLocked is the commit itself, for callers that already hold the
// store lock and have already read the branch.
//
// It exists for the same reason the rest's and the interval's do: the lock is
// not reentrant, and a caller that reads the branch, derives the distance from
// it and then commits has to do all three under one hold, or another write could
// move the character between the read and the write.
func (s *Store) applyJourneyLocked(storyID string, meta StoryMeta, records []StoryEventRecord, req JourneyRequest) (JourneyResult, error) {
	commandID := strings.TrimSpace(req.CommandID)
	commandHash := strings.TrimSpace(req.CommandHash)
	if commandID != "" && commandHash == "" {
		return JourneyResult{}, fmt.Errorf("%w: command_id=%q requires a command hash", ErrJourneyCommandConflict, commandID)
	}

	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return JourneyResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if commandID != "" {
		committed, found, err := journeyRecordForCommand(records, branchID, commandID, commandHash, worldContentForMeta(meta))
		if err != nil {
			return JourneyResult{}, err
		}
		if found {
			// The first attempt already walked this stretch. Returning its result
			// makes the retry a read rather than a second charge.
			committed.Replayed = true
			return committed, nil
		}
	}
	if branchIsTerminal(records, branch.Head) {
		return JourneyResult{}, branchTerminalError()
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return JourneyResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("jnyrec")
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	plan, err := planJourneyStretch(projection, projection.State, branchID, system, req, eventID, lifespanBoundary(meta, projection.State, DefaultActorID))
	if err != nil {
		return JourneyResult{}, err
	}

	planReq := req
	planReq.CommandID, planReq.CommandHash = commandID, commandHash
	newEvents, headID, err := journeySettlementEvents(projection, system, branchID, branch.Head, now, planReq, plan)
	if err != nil {
		return JourneyResult{}, err
	}

	branch.Head = headID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return JourneyResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return JourneyResult{}, err
	}
	return JourneyResult{
		EventID: eventID, JourneyID: plan.Advanced.Snapshot.JourneyID,
		RouteID: plan.Route.ID, RouteVersion: plan.Route.Version,
		Journey: plan.Advanced.Snapshot, Position: plan.Advanced.Position,
		ClockAfter:   plan.ClockAfter,
		AddedMinutes: plan.Advanced.AddedMinutes, ElapsedSeconds: plan.Advanced.AddedMinutes * 60,
		Arrived:       plan.Advanced.Done,
		Remaining:     plan.Advanced.RemainingMinutes,
		LocationLabel: worldPlaceLabel(worldContentForMeta(meta), plan.Advanced.Position.LocationID),
		SettlementOps: plan.Ops,

		StoppedAtLifespan:   plan.StoppedAtLifespan,
		LifespanEdgeSeconds: plan.LifespanEdgeSeconds,
	}, nil
}

// journeyPriorSnapshot finds the journey this request continues.
//
// An explicit JourneyID wins. Otherwise the branch's one unfinished journey is
// the continuation, which is what "继续赶路" means without naming anything: a
// branch can only be standing on one road at a time. Two unfinished journeys are
// refused rather than guessed between, because picking one would charge the
// remainder of a road the player may not mean.
func journeyPriorSnapshot(projection *storyBranchProjection, req JourneyRequest, branchID string) (exploration.JourneySnapshot, bool, error) {
	if projection == nil {
		return exploration.JourneySnapshot{}, false, nil
	}
	if id := strings.TrimSpace(req.JourneyID); id != "" {
		snapshot, ok := projection.Journeys[id]
		if !ok {
			return exploration.JourneySnapshot{}, false, fmt.Errorf("%w: %s", ErrJourneyUnknownJourney, id)
		}
		return snapshot, true, nil
	}
	var found []exploration.JourneySnapshot
	for _, snapshot := range projection.Journeys {
		if journeyFinished(snapshot) {
			continue
		}
		found = append(found, snapshot)
	}
	switch len(found) {
	case 0:
		return exploration.JourneySnapshot{}, false, nil
	case 1:
		return found[0], true, nil
	default:
		return exploration.JourneySnapshot{}, false,
			fmt.Errorf("%w: branch %s stands on %d unfinished journeys", ErrJourneyUnknownJourney, branchID, len(found))
	}
}

// journeyFinished reports whether a snapshot already reached its destination.
func journeyFinished(snapshot exploration.JourneySnapshot) bool {
	return snapshot.ElapsedMinutes >= snapshot.TotalMinutes
}

// journeyResolveRoute picks the route a request walks.
//
// A continuation without a route id uses the journey's own route: the player said
// "continue", not "switch roads". An explicit route that disagrees with the
// journey being continued is refused rather than silently substituting one road
// for another.
func journeyResolveRoute(req JourneyRequest, prior exploration.JourneySnapshot, priorFound bool, projection *storyBranchProjection) (exploration.RouteDefinition, error) {
	routeID := strings.TrimSpace(req.RouteID)
	if routeID == "" && priorFound {
		routeID = prior.RouteID
	}
	if routeID == "" {
		return exploration.RouteDefinition{}, fmt.Errorf("%w: no route named and no journey in progress", ErrJourneyUnknownRoute)
	}
	route, ok := worldJourneyRoute(projection, routeID)
	if !ok {
		return exploration.RouteDefinition{}, fmt.Errorf("%w: %s", ErrJourneyUnknownRoute, routeID)
	}
	if priorFound && route.ID != prior.RouteID {
		return exploration.RouteDefinition{}, fmt.Errorf("%w: continuing %s as %s", ErrJourneyUnknownJourney, prior.RouteID, route.ID)
	}
	if priorFound && route.Version != prior.RouteVersion {
		return exploration.RouteDefinition{}, fmt.Errorf("%w: %s changed version from %s to %s",
			ErrJourneyUnknownRoute, route.ID, prior.RouteVersion, route.Version)
	}
	return route, nil
}

// journeyTarget is the absolute elapsed minute this request reaches.
func journeyTarget(route exploration.RouteDefinition, snapshot exploration.JourneySnapshot, req JourneyRequest) (int64, error) {
	if req.Arrive {
		return snapshot.TotalMinutes, nil
	}
	if req.TargetMinutes < 0 {
		return 0, fmt.Errorf("%w: negative target", ErrJourneyNoProgress)
	}
	if req.TargetMinutes > snapshot.TotalMinutes {
		return 0, fmt.Errorf("%w: target %d beyond route total %d",
			ErrJourneyNoProgress, req.TargetMinutes, snapshot.TotalMinutes)
	}
	if req.TargetMinutes <= snapshot.ElapsedMinutes {
		return 0, fmt.Errorf("%w: target %d is not past the committed %d",
			ErrJourneyNoProgress, req.TargetMinutes, snapshot.ElapsedMinutes)
	}
	return req.TargetMinutes, nil
}

// journeyRequireOrigin refuses a new journey the scene does not start from.
//
// §4.1 starts from the current formal position. The scene text is resolved to a
// registered place first, so a description that merely mentions a road is not
// the same as standing at its beginning.
//
// It reads the state the caller hands it rather than the projection, because the
// question is about the place the road starts from. During a turn that is the
// committed place the turn began on; the narrator's own description of the
// arrival is not evidence that the character was ever at the origin.
func journeyRequireOrigin(scene map[string]any, route exploration.RouteDefinition, worlds ...*WorldContent) error {
	current, ok := storyLocationID(scene, worlds...)
	if !ok {
		// A scene that names no registered place cannot be shown to start at
		// this route's origin, and a route begun from an unknown place would
		// charge time for a road the story never placed the character on.
		return fmt.Errorf("%w: the scene names no registered place, route starts at %s",
			ErrJourneyNotAtOrigin, worldPlaceLabel(firstWorldContent(worlds), route.OriginLocationID))
	}
	if current == route.OriginLocationID {
		return nil
	}
	return fmt.Errorf("%w: scene says %q, route starts at %s",
		ErrJourneyNotAtOrigin, storySceneText(scene, storySceneLocationField), route.OriginLocationID)
}

// journeyRequireOutside refuses a new journey while the character is inside a
// realm.
//
// §11.2 keeps a realm's own places separate from the world's routes: a road
// between two market places does not run through a cave the character is
// standing in, so walking it from in there would spend world time on a distance
// they never crossed. Leaving is the action that puts them back on the map.
func journeyRequireOutside(projection *storyBranchProjection) error {
	if projection == nil {
		return nil
	}
	for _, instance := range projection.Realms {
		if instance.Snapshot.Presence == exploration.InstanceInside {
			return fmt.Errorf("%w: leave %s first", ErrJourneyInsideRealm, instance.Snapshot.Binding.DefinitionID)
		}
	}
	return nil
}

// journeyLocationOps writes the place the character now stands at.
//
// A mid-segment stop produces no write. The character is on the road between two
// places, and naming either one would teleport them to a stop they have not
// reached. The record's Position carries the road and the offset, which is the
// honest fact a later continuation reads.
func journeyLocationOps(position exploration.JourneyPosition, worlds ...*WorldContent) []ActorStateOp {
	var content *WorldContent
	if len(worlds) > 0 {
		content = worlds[0]
	}
	if strings.TrimSpace(position.LocationID) == "" {
		return nil
	}
	return []ActorStateOp{{
		Op:         "set",
		ActorID:    DefaultStoryContextActorID,
		FieldID:    normalizeActorStateFieldName(storySceneLocationField),
		Value:      worldPlaceLabel(content, position.LocationID),
		Reason:     "由已提交的行程结算写入当前位置，不由叙述改写。",
		SourceKind: StateOpSourceJourney,
		SourceID:   position.LocationID,
	}}
}

// journeyRecordForCommand looks for the stretch a previous attempt committed
// under this command ID. It scans the branch's own recent records rather than
// trusting a process-local index, so the answer survives a restart.
func journeyRecordForCommand(records []StoryEventRecord, branchID, commandID, commandHash string, worlds ...*WorldContent) (JourneyResult, bool, error) {
	for _, record := range records {
		if record.Envelope.Type != StoryEventTypeJourneyRecord {
			continue
		}
		var event JourneyRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return JourneyResult{}, false, fmt.Errorf("decode committed journey record: %w", err)
		}
		if event.BranchID != branchID || strings.TrimSpace(event.CommandID) != commandID {
			continue
		}
		if strings.TrimSpace(event.CommandHash) != commandHash {
			return JourneyResult{}, false, fmt.Errorf("%w: command_id=%q existing_hash=%q requested_hash=%q",
				ErrJourneyCommandConflict, commandID, event.CommandHash, commandHash)
		}
		return JourneyResult{
			EventID: event.ID, JourneyID: event.Journey.JourneyID,
			RouteID: event.RouteID, RouteVersion: event.RouteVersion,
			Journey: event.Journey, Position: event.Position,
			ClockAfter: event.ClockAfter,
			// The first attempt already moved this time, so a retry adds none. The
			// distance it covered is still reported, because the caller has to be
			// able to say what the journey stands at.
			AddedMinutes:   0,
			ElapsedSeconds: 0,
			Arrived:        event.Arrived,
			Remaining:      event.Journey.TotalMinutes - event.Journey.ElapsedMinutes,
			LocationLabel:  worldPlaceLabel(firstWorldContent(worlds), event.Position.LocationID),
			// A replay reports the cut the first attempt committed. The distance
			// added is zero because this call added none, but the reason the road
			// stopped is a fact about the committed stretch and stays true.
			StoppedAtLifespan:   event.StoppedAtLifespan,
			LifespanEdgeSeconds: event.LifespanEdgeSeconds,
		}, true, nil
	}
	return JourneyResult{}, false, nil
}

// applyJourneyRecord merges a side record into the branch projection. It is the
// only place a journey and the formal clock are written during a rebuild, so a
// replay reproduces the road and the time that were live when the record was
// committed.
//
// The stored snapshot is not trusted on its own: the route rules are re-run from
// the previous committed offset, and a record whose claimed progress disagrees
// with them is refused. A corrupt row therefore cannot hand the branch a
// position the route does not have.
func applyJourneyRecord(branch *storyBranchProjection, event JourneyRecordEvent) error {
	if err := event.ClockAfter.Validate(); err != nil {
		return err
	}
	if event.ClockBefore.Validate() != nil {
		return fmt.Errorf("行程记录缺少合法的起始时刻 / a journey record needs a valid clock before it")
	}
	if event.ClockAfter.Seconds < branch.clockSeconds() {
		return fmt.Errorf("%w: journey record moves the story clock backwards", storytime.ErrTimeReversal)
	}
	if event.ClockAfter.Seconds-event.ClockBefore.Seconds != event.AddedMinutes*60 {
		return fmt.Errorf("行程记录的时间与所走路程不一致 / journey clock and distance disagree")
	}
	route, ok := worldJourneyRoute(branch, event.RouteID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrJourneyUnknownRoute, event.RouteID)
	}
	if route.Version != event.RouteVersion {
		return fmt.Errorf("%w: %s version %s", ErrJourneyUnknownRoute, event.RouteID, event.RouteVersion)
	}

	prior, found := branch.Journeys[event.Journey.JourneyID]
	var start exploration.JourneySnapshot
	if found {
		start = prior
	} else {
		prepared, err := exploration.PrepareJourney(route, event.Journey.JourneyID, journeySupportedMethods())
		if err != nil {
			return err
		}
		start = prepared
	}
	advanced, err := exploration.AdvanceTo(route, start, event.Journey.ElapsedMinutes, journeySupportedMethods())
	if err != nil {
		return err
	}
	if advanced.Snapshot.ElapsedMinutes != event.Journey.ElapsedMinutes || advanced.AddedMinutes != event.AddedMinutes {
		return fmt.Errorf("行程记录与路线规则不一致 / journey record disagrees with the route rules")
	}

	if err := validateJourneyTravelersRecord(branch, event, start, found); err != nil {
		return err
	}
	advanced.Snapshot.TravelerIDs = append([]string(nil), event.Journey.TravelerIDs...)
	if branch.Journeys == nil {
		branch.Journeys = map[string]exploration.JourneySnapshot{}
	}
	branch.Journeys[advanced.Snapshot.JourneyID] = advanced.Snapshot
	clock := event.ClockAfter
	branch.Clock = &clock
	return nil
}

// validateJourneyRecordEvent rejects a record that could not have been honestly
// attributed. A malformed row is an error rather than a silent drop, because a
// dropped record is a road the character walked that the story would deny.
func validateJourneyRecordEvent(event JourneyRecordEvent) error {
	if strings.TrimSpace(event.BranchID) == "" {
		return fmt.Errorf("行程记录缺少 branch_id / a journey record requires a branch")
	}
	if strings.TrimSpace(event.RouteID) == "" {
		return fmt.Errorf("行程记录缺少 route_id / a journey record requires a route")
	}
	if strings.TrimSpace(event.Journey.JourneyID) == "" {
		return fmt.Errorf("行程记录缺少 journey_id / a journey record requires a journey")
	}
	if event.AddedMinutes < 0 {
		return fmt.Errorf("行程记录的路程为负 / a journey record cannot cover negative distance")
	}
	if event.StoppedAtLifespan {
		// A cut says the stretch stopped short of an edge. A record claiming a cut
		// with no edge, or an edge the clock has already passed, would tell the
		// next turn the character is at the brink when the branch is really past
		// it — the opposite of what §4 asks the backend to show.
		if event.LifespanEdgeSeconds <= 0 {
			return fmt.Errorf("行程记录称被寿元边界截断，却没有边界时刻 / a lifespan cut needs an edge instant")
		}
		if event.ClockAfter.Validate() == nil && event.ClockAfter.Seconds > event.LifespanEdgeSeconds {
			return fmt.Errorf("行程记录称停在寿元边界之前，时钟却已越过边界 / a lifespan cut must not pass the edge")
		}
	}
	return nil
}

// cloneJourneySnapshots copies the journey map so one branch's travel cannot be
// mutated through an alias into another.
func cloneJourneySnapshots(journeys map[string]exploration.JourneySnapshot) map[string]exploration.JourneySnapshot {
	if len(journeys) == 0 {
		return nil
	}
	cloned := make(map[string]exploration.JourneySnapshot, len(journeys))
	for id, snapshot := range journeys {
		copied := snapshot
		copied.CompletedSegmentIDs = append([]string{}, snapshot.CompletedSegmentIDs...)
		copied.TravelerIDs = append([]string(nil), snapshot.TravelerIDs...)
		cloned[id] = copied
	}
	return cloned
}

// journeyPlannedStretch is one stretch of travel the rules have already accepted.
//
// It is the result of everything a journey commit decides from the branch alone:
// the route, the snapshot before and after, the minutes really covered and the
// formal clock the stretch ends on. Nothing here has been written yet, which is
// what lets a main-story turn commit the travel inside its own transaction.
type journeyPlannedStretch struct {
	CompanionSources []JourneyCompanionInput
	Route            exploration.RouteDefinition
	Advanced         exploration.AdvanceResult
	Clock            StoryClock
	ClockAfter       StoryClock
	EventID          string
	Ops              []ActorStateOp

	// StoppedAtLifespan reports that the requested stretch was cut short
	// because the protagonist's confirmed lifespan edge fell inside it, so the
	// character is standing at the brink rather than years past it.
	//
	// Without this cut a single 去坊市 would carry a character whose span was
	// nearly spent straight over the edge, and the commit's crossing check would
	// then declare 寿尽 for a road the player chose as ordinary travel. §4 asks
	// for the situation to be put in front of the player before the action
	// crosses, which is the same thing the training and rest intervals get from
	// their stop node; a journey has no activity to pause, so the cut is how it
	// stops.
	StoppedAtLifespan bool
	// LifespanEdgeSeconds is the instant the cut stops at or short of, on the same
	// axis as ClockAfter. Zero when the stretch was not cut.
	LifespanEdgeSeconds int64
}

// planJourneyStretch resolves and validates one stretch against a branch
// projection, without writing anything.
//
// It is the same arithmetic ApplyJourney commits: the origin, the realm
// exclusion, the target minute, the distance really covered and the formal
// interval it costs. ApplyJourney calls it so the entry and a main-story turn
// cannot disagree about what a road costs; the turn calls it so a player who
// simply writes 去坊市 gets the same route rules as the entry, rather than prose
// with no committed distance behind it.
//
// origin is where the character really stands when the road is judged. During a
// turn that is the committed place the turn began on: a narrator who writes the
// arrival has not moved them, and a road may only start from a place they were
// already in. A journey that is already under way is continued from its own
// snapshot, so this only answers the question a new road asks.
// boundary is the protagonist's resolved lifespan edge, or nil when this story
// cannot answer. A stretch that would cross it is cut at it, so the player is
// standing at the brink and still chooses; a stretch that stops short of it is
// untouched.
func planJourneyStretch(projection *storyBranchProjection, origin map[string]any, branchID string, system StoryDirectorActorStateSystem, req JourneyRequest, eventID string, boundary *storyLifespanBoundary) (journeyPlannedStretch, error) {
	// Roads and realm movement obey the same active-combat and living-body
	// boundary. Ordinary travel cannot substitute for a real escape action.
	movementMeta := StoryMeta{ActorStateSchema: &ActorStateSchemaSnapshot{Version: ActorStateSchemaVersion, System: system}}
	if err := realmActionBlocked(projection, origin, movementMeta); err != nil {
		return journeyPlannedStretch{}, err
	}
	if err := realmActionBlocked(projection, projection.State, movementMeta); err != nil {
		return journeyPlannedStretch{}, err
	}
	clock, err := branchStoryClock(projection, DefaultStoryContextActorID, system)
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	prior, priorFound, err := journeyPriorSnapshot(projection, req, branchID)
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	route, err := journeyResolveRoute(req, prior, priorFound, projection)
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	if !adventureRouteAvailable(projection, route.ID) {
		return journeyPlannedStretch{}, fmt.Errorf("%w: route permission has not arrived", ErrJourneyUnknownRoute)
	}

	var snapshot exploration.JourneySnapshot
	newJourney := !priorFound
	if newJourney {
		if err := journeyRequireOutside(projection); err != nil {
			return journeyPlannedStretch{}, err
		}
		if err := journeyRequireOrigin(origin, route, projection.worldContent()); err != nil {
			return journeyPlannedStretch{}, err
		}
		snapshot, err = exploration.PrepareJourney(route, newID("jny"), journeySupportedMethods())
		if err != nil {
			return journeyPlannedStretch{}, err
		}
		snapshot.TravelerIDs = []string{DefaultActorID}
		for _, companion := range req.companionSources {
			snapshot.TravelerIDs = append(snapshot.TravelerIDs, companion.CharacterID)
		}
	} else {
		if len(req.companionSources) > 0 {
			return journeyPlannedStretch{}, fmt.Errorf("an ongoing journey cannot add new companions")
		}
		snapshot = prior
		snapshot.TravelerIDs = continuingJourneyTravelers(origin, prior.TravelerIDs)
		snapshot.TravelerIDs = continuingJourneyTravelers(projection.State, snapshot.TravelerIDs)
	}

	target, err := journeyTarget(route, snapshot, req)
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	// A road that would carry the protagonist past their confirmed lifespan edge
	// is cut at the edge instead. §4 asks for the situation to be put in front of
	// the player before the action really crosses, and a journey spends its time
	// in one lump: without this the whole stretch commits and the crossing check
	// declares 寿尽 for an ordinary walk to the market.
	//
	// The cut is in whole minutes because AdvanceTo's target is minutes, and the
	// edge is in seconds. Flooring is the only safe direction: the stretch must
	// never overshoot the edge, because the crossing check ends the branch on the
	// first clock strictly past it. Landing exactly on the edge is alive and is
	// the state §4 asks for — the character stands at the brink and still chooses.
	// A cut that leaves zero minutes means the character is already at the brink,
	// and that is reported as a stop rather than as ErrJourneyNoProgress: the road
	// was not already walked, the years were already spent.
	cutTarget, cutAtEdge := journeyLifespanCut(target, snapshot, clock, boundary)
	if cutTarget <= snapshot.ElapsedMinutes {
		if cutAtEdge {
			return journeyPlannedStretch{}, fmt.Errorf("%w: %d", ErrJourneyAtLifespanEdge, boundary.TotalYears)
		}
		// Not a lifespan case: the caller asked for a point the road is already
		// at, which is the ordinary no-progress refusal.
		return journeyPlannedStretch{}, fmt.Errorf("%w: %s at %d minutes", ErrJourneyNoProgress, route.ID, target)
	}
	advanced, err := exploration.AdvanceTo(route, snapshot, cutTarget, journeySupportedMethods())
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	if advanced.AddedMinutes == 0 && !newJourney {
		// §4.2's "只提交实际走到停点的路程": a request that covers no ground is not a
		// stretch of travel, and committing it would show a road nobody walked.
		return journeyPlannedStretch{}, fmt.Errorf("%w: %s at %d minutes", ErrJourneyNoProgress, route.ID, cutTarget)
	}

	clockAfter, err := AdvanceStoryClock(clock, advanced.AddedMinutes*60)
	if err != nil {
		return journeyPlannedStretch{}, err
	}
	ops := journeyLocationOps(advanced.Position, projection.worldContent())
	if newJourney {
		costs, err := journeyDepartureCosts(projection, route.ID, snapshot.TravelerIDs)
		if err != nil {
			return journeyPlannedStretch{}, err
		}
		ops = append(ops, costs...)
	}
	ops = append(ops, journeyPresenceOps(projection.State, system, advanced.Snapshot, advanced.Position)...)
	for _, op := range ops {
		if err := validateActorStateOp(op); err != nil {
			return journeyPlannedStretch{}, err
		}
	}
	return journeyPlannedStretch{
		CompanionSources: append([]JourneyCompanionInput(nil), req.companionSources...),
		Route:            route, Advanced: advanced, Clock: clock, ClockAfter: clockAfter,
		EventID: eventID, Ops: ops,
		StoppedAtLifespan:   cutAtEdge,
		LifespanEdgeSeconds: lifespanEdgeSeconds(boundary, cutAtEdge),
	}, nil
}

// journeyLifespanCut shortens a target so the stretch stops before the
// protagonist's lifespan edge.
//
// The second result reports that the edge really was inside the requested
// stretch, which is what lets the caller tell "the years ran out on the way" from
// "the request asked for a point already reached".
//
// Minutes are floored rather than rounded up so the stretch never lands past the
// edge: the crossing check ends the branch on any clock strictly beyond it, and a
// rounded-up minute would turn an ordinary walk into 寿尽. Landing exactly on the
// edge is alive and is the state §4 asks for, so a whole minute that reaches it
// is left as is. Flooring also keeps the route's own arithmetic honest —
// AdvanceTo is given a whole minute, and the clock advances by exactly that
// minute.
func journeyLifespanCut(target int64, snapshot exploration.JourneySnapshot, clock StoryClock, boundary *storyLifespanBoundary) (int64, bool) {
	if boundary == nil || boundary.At <= clock.Seconds {
		// No edge, or the clock is already at or past it. The crossing check owns
		// that case; a cut here would silently rewrite an already-decided branch.
		return target, false
	}
	remainingSeconds := boundary.At - clock.Seconds
	remainingMinutes := remainingSeconds / 60
	reachable := snapshot.ElapsedMinutes + remainingMinutes
	if target <= reachable {
		return target, false
	}
	return reachable, true
}

// lifespanEdgeSeconds reports the instant a cut stopped at, or zero when the
// stretch was not cut.
func lifespanEdgeSeconds(boundary *storyLifespanBoundary, cut bool) int64 {
	if !cut || boundary == nil {
		return 0
	}
	return boundary.At
}

// journeyRecordEvent is the side record a planned stretch publishes.
func journeyRecordEvent(branchID, parentID, now string, req JourneyRequest, plan journeyPlannedStretch) JourneyRecordEvent {
	return JourneyRecordEvent{
		CompanionSources: append([]JourneyCompanionInput(nil), plan.CompanionSources...),
		V:                schemaVersion, Type: StoryEventTypeJourneyRecord, ID: plan.EventID,
		ParentID: parentID, BranchID: branchID, Ts: now,
		TurnID: strings.TrimSpace(req.TurnID), Cursor: plan.EventID,
		CommandID: strings.TrimSpace(req.CommandID), CommandHash: strings.TrimSpace(req.CommandHash),
		WorldTime:    strings.TrimSpace(req.WorldTime),
		RouteID:      plan.Route.ID,
		RouteVersion: plan.Route.Version,
		Journey:      plan.Advanced.Snapshot,
		Position:     plan.Advanced.Position,
		AddedMinutes: plan.Advanced.AddedMinutes,
		Arrived:      plan.Advanced.Done,
		// The cut is a fact about the road the player actually got, so it is
		// recorded with the stretch rather than only returned to the caller: a
		// rebuilt branch has to reproduce both the shorter stop and the reason
		// for it.
		StoppedAtLifespan:   plan.StoppedAtLifespan,
		LifespanEdgeSeconds: plan.LifespanEdgeSeconds,
		ClockBefore:         plan.Clock,
		ClockAfter:          plan.ClockAfter,
	}
}

// journeySettlementEvents turns a planned stretch into the events a commit
// writes: the record, its place write, and any task objective the arrival ticks.
//
// It returns the events in commit order and the new branch head. The caller
// appends them in its own transaction, which is what makes a turn's travel and
// its prose one commit rather than two.
func journeySettlementEvents(projection *storyBranchProjection, system StoryDirectorActorStateSystem, branchID, parentID, now string, req JourneyRequest, plan journeyPlannedStretch) ([]any, string, error) {
	record := journeyRecordEvent(branchID, parentID, now, req, plan)
	headID := record.ID
	newEvents := []any{record}
	if len(plan.Ops) > 0 {
		// The place write is a separate event parented to the record, for the same
		// reason the rest's pool write is: an event whose ID is also its parent
		// closes the branch's parent chain into a cycle.
		deltaID := newID("sd")
		newEvents = append(newEvents, newStateDeltaEventWithActorOps(deltaID, record.ID, branchID, now, nil, plan.Ops))
		headID = deltaID
	}
	// An arrival is a fact, and a task objective may be waiting on it. The tick
	// is appended to this same transaction so the road and the task cannot be
	// separated by a crash, and so replaying the branch produces both or neither.
	if plan.Advanced.Done {
		ticks, _, err := questAdvanceEvents(projection, branchID, req.TurnID, questFactFromJourneyArrival(plan.Advanced.Position.LocationID), record.ID, headID, now, plan.ClockAfter.Seconds)
		if err != nil {
			return nil, "", err
		}
		for _, tick := range ticks {
			newEvents = append(newEvents, tick)
			if questRecord, ok := tick.(QuestRecordEvent); ok {
				headID = questRecord.ID
			}
		}
	}
	// The stretch of road spent the interval between the two instants, so every
	// registered matter whose instant it crossed closes here, whether or not the
	// player was thinking about it while walking.
	closed, headID := storyIntervalEvents(intervalProjectionAfterOps(projection, plan.Ops), system, branchID, headID, now, "", "", plan.Clock, plan.ClockAfter)
	newEvents = append(newEvents, closed...)
	return newEvents, headID, nil
}

// JourneyState reads one branch's formal clock and journeys.
//
// It is the read the journey entry opens on: a window that was closed and
// reopened has to find the same road at the same distance, and a branch that
// never travelled has to read as standing nowhere rather than as an error. Like
// the rest's read it writes nothing, so opening the entry costs no world time.
func (s *Store) JourneyState(storyID, branchID string) (StoryClock, map[string]exploration.JourneySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return StoryClock{}, nil, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return StoryClock{}, nil, err
	}
	journeys := cloneJourneySnapshots(projection.Journeys)
	if projection.Clock == nil {
		return NewStoryClock(storySceneText(projection.State, storySceneTimeField)), journeys, nil
	}
	return *projection.Clock, journeys, nil
}

// JourneyOptions is the route set the entry can offer, with each route's own
// duration and the place wording the player reads.
//
// It exists so the window never invents a destination or a number: everything it
// shows comes from the same registered definitions ApplyJourney validates
// against.
type JourneyOptions struct {
	// CurrentLocation is the scene's committed location text, which is what the
	// entry shows as where the character stands.
	CurrentLocation   string `json:"current_location,omitempty"`
	CurrentLocationID string `json:"current_location_id,omitempty"`
	// Routes are the registered connections, each with its authored duration.
	Routes []JourneyRouteOption `json:"routes"`
}

// JourneyRouteOption is one registered route as the entry offers it.
type JourneyRouteOption struct {
	Name               string `json:"name,omitempty"`
	NameKey            string `json:"name_key,omitempty"`
	OriginNameKey      string `json:"origin_name_key,omitempty"`
	DestinationNameKey string `json:"destination_name_key,omitempty"`
	RouteID            string `json:"route_id"`
	RouteVersion       string `json:"route_version"`
	OriginLocationID   string `json:"origin_location_id"`
	OriginLabel        string `json:"origin_label"`
	DestinationID      string `json:"destination_location_id"`
	DestinationLabel   string `json:"destination_label"`
	TotalMinutes       int64  `json:"total_minutes"`
	// MovementMethod is the authored travel method the route was written for.
	MovementMethod string `json:"movement_method"`
}

// JourneyOptionsFor reads the routes the entry can offer.
//
// A route whose definition does not validate is left out rather than shown as
// walkable: §10 requires an unimplemented or broken path to be reported as a real
// situation, not as a button that fails when pressed.
func (s *Store) JourneyOptionsFor(storyID, branchID string) (JourneyOptions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return JourneyOptions{}, err
	}
	defer release()
	if _, err := s.refreshStoryJournalLocked(storyID, true); err != nil {
		return JourneyOptions{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return JourneyOptions{}, err
	}
	return journeyOptionsFrom(projection), nil
}

// journeyOptionsFrom builds the walkable-route list from a projection the caller
// has already read.
//
// It is split out so the main story's World Actions page can report the same
// roads from one journal read. The origin filter and the inside-a-realm refusal
// are the commit's own answers, so a road the page names is one the turn can
// really walk.
func journeyOptionsFrom(projection *storyBranchProjection) JourneyOptions {
	options := JourneyOptions{CurrentLocation: storySceneText(projection.State, storySceneLocationField)}
	options.CurrentLocationID, _ = storyLocationID(projection.State, projection.worldContent())
	// A journey is not offered from inside a realm, because the same refusal the
	// commit applies must not be a surprise the entry never mentioned.
	insideRealm := journeyRequireOutside(projection) != nil
	for _, route := range worldJourneyRoutes(projection) {
		if !adventureRouteAvailable(projection, route.ID) {
			continue
		}
		total, _, diagnostics := exploration.ValidateRoute(route, journeySupportedMethods())
		if len(diagnostics) > 0 {
			continue
		}
		if insideRealm {
			continue
		}
		if current, ok := storyLocationID(projection.State, projection.worldContent()); !ok || current != route.OriginLocationID {
			continue
		}
		label, nameKey := "", ""
		if route.ID == spiritAscentRoute {
			label, nameKey = "跨界前往灵界（单向，耗护界锚、生命与灵力）", "game.spirit.ascent.name"
		}
		if shortcut, ok := realmShortcutFor(route.ID); ok {
			label, nameKey = shortcut.Name, shortcut.NameKey
		}
		switch route.ID {
		case tideGaugeShortRoute:
			label, nameKey = "潮信渡短渡", "game.tideGauge.shortRoute"
		case tideGaugeOuterRoute:
			label, nameKey = "潮信渡外汊水路", "game.tideGauge.outerRoute"
		}
		originNameKey, destinationNameKey := spiritPlaceNameKey(route.OriginLocationID), spiritPlaceNameKey(route.DestinationLocationID)
		if route.ID == tideGaugeShortRoute || route.ID == tideGaugeOuterRoute {
			originNameKey, destinationNameKey = "game.tideGauge.ferry", "game.tideGauge.egretVillage"
		}
		if content := projection.worldContent(); content != nil {
			policy := content.RoutePolicies[route.ID]
			label, nameKey = policy.Name, policy.NameKey
			originNameKey, destinationNameKey = policy.OriginNameKey, policy.DestinationNameKey
		}
		options.Routes = append(options.Routes, JourneyRouteOption{
			Name: label, NameKey: nameKey,
			OriginNameKey: originNameKey, DestinationNameKey: destinationNameKey,
			RouteID: route.ID, RouteVersion: route.Version,
			OriginLocationID: route.OriginLocationID, OriginLabel: worldPlaceLabel(projection.worldContent(), route.OriginLocationID),
			DestinationID: route.DestinationLocationID, DestinationLabel: worldPlaceLabel(projection.worldContent(), route.DestinationLocationID),
			TotalMinutes:   total,
			MovementMethod: route.Segments[0].MovementMethodID,
		})
	}
	return options
}
