package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"ta/internal/exploration"
	"ta/internal/interactive"
)

// This file makes one real journey reachable during play.
//
// §4 of 自制-探索与行路 asks for the ordinary case to be one complete flow: the
// player names a destination, the program finds the registered connection from
// where the character really stands, settles the distance and the in-fiction time
// from that connection, and commits the travel and the new place once along the
// single story journal. Three properties are deliberate here, and they are the
// same three a rest has.
//
//   - The rules run before the prose. The distance and the time are the route's
//     own numbers; a narrative afterwards describes what was committed. A model
//     failure cannot leave the character half-way along a road the journal does
//     not show them walking.
//
//   - The route is never taken from the request as authority. The caller names a
//     route; the store resolves it against the registered set and refuses one
//     whose origin is not where the scene places the character. §4.1 starts from
//     the current formal position rather than from whichever road was asked for.
//
//   - The journey owns no clock. The in-fiction time it spends is the distance it
//     covered, so the player's real waiting costs nothing and the two numbers
//     cannot drift apart.

// JourneyAppRequest asks for one stretch of travel on a registered route.
type JourneyAppRequest struct {
	StoryID  string
	BranchID string
	// WorldTime is the story's display text for this moment. Empty leaves the
	// scene's own wording alone.
	WorldTime string
	// RouteID names the registered connection. Empty continues the branch's one
	// unfinished journey, which is what "继续赶路" means without repeating the
	// destination.
	RouteID string
	// JourneyID continues a journey a real stop interrupted. Empty continues the
	// branch's one unfinished journey, or starts a new one when there is none.
	JourneyID string
	// TargetMinutes is the absolute elapsed minute to reach, counted from the
	// route's origin. A stop at 10 on a 30-minute route covers 10; continuing to
	// 30 then covers only the remaining 20.
	TargetMinutes int64
	// Arrive asks to walk the whole route, which is the ordinary "去坊市" case:
	// §4.2 lets one action complete a whole ordinary journey rather than clicking
	// through each stretch.
	Arrive bool
	// CommandID identifies one attempt, so a retry adds no second stretch of time.
	CommandID string
}

// JourneyAppResult is one committed stretch as the UI reports it.
type JourneyAppResult struct {
	StoryID      string `json:"story_id"`
	BranchID     string `json:"branch_id"`
	EventID      string `json:"event_id"`
	JourneyID    string `json:"journey_id"`
	RouteID      string `json:"route_id"`
	RouteVersion string `json:"route_version"`
	// Origin and Destination are the route's endpoints in the story's own
	// wording, so the UI never has to translate a location id.
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	// AddedMinutes is the distance this stretch really covered and Elapsed the
	// natural wording for it. A retry reports zero, because the first attempt
	// already moved this time.
	AddedMinutes int64  `json:"added_minutes"`
	Elapsed      string `json:"elapsed,omitempty"`
	// ElapsedMinutes and TotalMinutes are the journey's cumulative position on the
	// road, so a stop and a continuation read the same numbers.
	ElapsedMinutes int64 `json:"elapsed_minutes"`
	TotalMinutes   int64 `json:"total_minutes"`
	Remaining      int64 `json:"remaining_minutes"`
	// Arrived reports that the character reached the destination.
	Arrived bool `json:"arrived"`
	// Location is the place the character now stands at, empty when the stop is
	// mid-segment and they are between places.
	Location string `json:"location,omitempty"`
	// MidSegment reports that the stretch stopped on the road rather than at a
	// place, which is what a real interruption inside a segment looks like.
	MidSegment bool `json:"mid_segment"`
	// StoppedAtLifespan reports that the road was cut short by the protagonist's
	// confirmed lifespan edge, so the journey is unfinished and the character
	// stands at the brink. §4 asks for the situation to be put in front of the
	// player before the action crosses, and this is how the client learns that
	// the shorter road is that moment rather than a plain stop.
	StoppedAtLifespan bool `json:"stopped_at_lifespan,omitempty"`
	// LifespanEdgeSeconds is the instant the cut stopped short of, on the same
	// axis as ClockSeconds. Zero when the stretch was not cut.
	LifespanEdgeSeconds int64 `json:"lifespan_edge_seconds,omitempty"`
	// ClockSeconds is the branch's formal instant after this stretch. It is the
	// single comparable moment every later activity is planned against.
	ClockSeconds int64 `json:"clock_seconds"`
	// Replayed reports that this answer came from a stretch already committed
	// under the same command, so the caller knows no second travel happened.
	Replayed bool `json:"replayed"`
}

// JourneyState is the journey entry's read: where the character stands, the
// routes that start there, and any road already under way.
type JourneyState struct {
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	// ClockSeconds is the branch's formal instant, and Anchor the story's own
	// wording for its origin.
	ClockSeconds int64  `json:"clock_seconds"`
	Anchor       string `json:"anchor,omitempty"`
	// Location is the scene's committed location text, which is what the entry
	// shows as where the character stands.
	Location   string `json:"location,omitempty"`
	LocationID string `json:"location_id,omitempty"`
	// Routes are the registered connections, each with its authored duration.
	Routes []interactive.JourneyRouteOption `json:"routes"`
	// Active is the branch's unfinished journey, so a road a real stop
	// interrupted can be continued rather than restarted.
	Active *JourneyProgressView `json:"active,omitempty"`
}

// JourneyProgressView is one journey as the entry shows it.
type JourneyProgressView struct {
	JourneyID string `json:"journey_id"`
	RouteID   string `json:"route_id"`
	// Origin and Destination are the route's endpoints in the story's wording.
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	// ElapsedMinutes and TotalMinutes are the distance walked and the road's
	// length, both the route rules' own numbers.
	ElapsedMinutes int64 `json:"elapsed_minutes"`
	TotalMinutes   int64 `json:"total_minutes"`
	Remaining      int64 `json:"remaining_minutes"`
	// MidSegment reports that the character is between two places rather than at
	// one, so the entry does not name a location they have not reached.
	MidSegment bool `json:"mid_segment"`
}

// JourneyStateFor reads the branch's formal clock, the routes and any road under
// way.
//
// It is a plain read with no model call, so opening the entry costs no world time
// and no generation. Everything it reports is already committed.
func (a *App) JourneyStateFor(storyID, branchID string) (JourneyState, error) {
	return a.JourneyStateForLocale(storyID, branchID, "")
}

func (a *App) JourneyStateForLocale(storyID, branchID, locale string) (JourneyState, error) {
	if a == nil {
		return JourneyState{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return JourneyState{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return JourneyState{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	clock, journeys, err := store.JourneyState(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return JourneyState{}, err
	}
	options, err := store.JourneyOptionsFor(strings.TrimSpace(storyID), resolvedBranch)
	if err != nil {
		return JourneyState{}, err
	}
	return a.localizeJourneyState(journeyStateFrom(storyID, resolvedBranch, clock, journeys, options), storyContext, locale), nil
}

func journeyStateFrom(storyID, resolvedBranch string, clock interactive.StoryClock, journeys map[string]exploration.JourneySnapshot, options interactive.JourneyOptions) JourneyState {
	view := JourneyState{
		StoryID: strings.TrimSpace(storyID), BranchID: resolvedBranch,
		ClockSeconds: clock.Seconds, Anchor: clock.Anchor,
		Location: options.CurrentLocation, LocationID: options.CurrentLocationID, Routes: options.Routes,
	}
	// A branch can only be standing on one road at a time; the store refuses to
	// guess between two, so the entry shows the one there is and none otherwise.
	for _, journey := range journeys {
		if journey.ElapsedMinutes >= journey.TotalMinutes {
			continue
		}
		progress := JourneyProgressView{
			JourneyID: journey.JourneyID, RouteID: journey.RouteID,
			Origin:         interactive.JourneyPlaceLabel(journey.OriginLocationID),
			Destination:    interactive.JourneyPlaceLabel(journey.DestinationLocationID),
			ElapsedMinutes: journey.ElapsedMinutes, TotalMinutes: journey.TotalMinutes,
			Remaining:  journey.TotalMinutes - journey.ElapsedMinutes,
			MidSegment: strings.TrimSpace(journey.CurrentLocationID) == "" || journey.SegmentElapsedMinutes > 0,
		}
		if view.Active != nil {
			// Two unfinished journeys is a damaged branch rather than a choice. The
			// entry shows neither rather than presenting one as the road under way.
			view.Active = nil
			break
		}
		view.Active = &progress
	}
	return view
}

// ApplyJourney settles one stretch of travel and commits it.
//
// The rules run first and the commit is one transaction, so the journey, the
// place write and the elapsed time either all exist on the journal or none of
// them do. A retry under the same command ID returns the first attempt's answer
// with no added time.
func (a *App) ApplyJourney(request JourneyAppRequest) (JourneyAppResult, error) {
	if a == nil {
		return JourneyAppResult{}, ErrNoWorkspace
	}
	store, _, err := a.interactiveStoreRuntime()
	if err != nil {
		return JourneyAppResult{}, err
	}
	storyContext, err := store.StoryContext(strings.TrimSpace(request.StoryID), strings.TrimSpace(request.BranchID))
	if err != nil {
		return JourneyAppResult{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	commandID := strings.TrimSpace(request.CommandID)
	result, err := store.ApplyJourney(strings.TrimSpace(request.StoryID), interactive.JourneyRequest{
		BranchID:      resolvedBranch,
		WorldTime:     strings.TrimSpace(request.WorldTime),
		RouteID:       strings.TrimSpace(request.RouteID),
		JourneyID:     strings.TrimSpace(request.JourneyID),
		TargetMinutes: request.TargetMinutes,
		Arrive:        request.Arrive,
		CommandID:     commandID,
		// The fingerprint covers what actually decides the stretch — the branch,
		// the road, the point reached — and is resolved against the branch the
		// store will really write to, so an empty branch and an explicit current
		// branch are the same command rather than a conflict.
		CommandHash: journeyCommandHash(request, resolvedBranch, commandID),
	})
	if err != nil {
		return JourneyAppResult{}, err
	}
	return JourneyAppResult{
		StoryID: strings.TrimSpace(request.StoryID), BranchID: resolvedBranch,
		EventID: result.EventID, JourneyID: result.JourneyID,
		RouteID: result.RouteID, RouteVersion: result.RouteVersion,
		Origin:       interactive.JourneyPlaceLabel(result.Journey.OriginLocationID),
		Destination:  interactive.JourneyPlaceLabel(result.Journey.DestinationLocationID),
		AddedMinutes: result.AddedMinutes,
		// A replay reports no elapsed wording either, because no time moved.
		Elapsed:        interactive.DescribeElapsed(result.ElapsedSeconds),
		ElapsedMinutes: result.Journey.ElapsedMinutes,
		TotalMinutes:   result.Journey.TotalMinutes,
		Remaining:      result.Remaining,
		Arrived:        result.Arrived,
		Location:       result.LocationLabel,
		MidSegment:     strings.TrimSpace(result.LocationLabel) == "",
		ClockSeconds:   result.ClockAfter.Seconds,
		Replayed:       result.Replayed,

		StoppedAtLifespan:   result.StoppedAtLifespan,
		LifespanEdgeSeconds: result.LifespanEdgeSeconds,
	}, nil
}

// journeyCommandHash fingerprints the content behind a journey's command ID.
//
// It covers what actually decides the stretch, so reusing one command ID for a
// genuinely different road is refused rather than answered with the wrong
// distance. The branch is the resolved one, so an empty branch and an explicit
// current branch fingerprint the same and a retry is not mistaken for a
// conflict.
//
// A continuation is fingerprinted with its journey and its target too:
// continuing a road is a different command from starting one, and a stop at 10
// is a different command from walking to the end.
func journeyCommandHash(request JourneyAppRequest, branchID, commandID string) string {
	if commandID == "" {
		// No identity means no idempotency to protect. The store refuses a command
		// id without a hash, and this keeps the two consistent.
		return ""
	}
	parts := []string{
		"denova.journey",
		strings.TrimSpace(request.StoryID),
		strings.TrimSpace(branchID),
		"route\x00" + strings.TrimSpace(request.RouteID),
		"journey\x00" + strings.TrimSpace(request.JourneyID),
		fmt.Sprintf("target\x00%d", request.TargetMinutes),
		fmt.Sprintf("arrive\x00%t", request.Arrive),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x01")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
