package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type RouteDefinition struct {
	ID                    string         `json:"id"`
	Version               string         `json:"version"`
	OriginLocationID      string         `json:"origin_location_id"`
	DestinationLocationID string         `json:"destination_location_id"`
	Segments              []RouteSegment `json:"segments"`
}

type RouteSegment struct {
	ID                 string `json:"id"`
	FromLocationID     string `json:"from_location_id"`
	ToLocationID       string `json:"to_location_id"`
	DurationMinutes    int64  `json:"duration_minutes"`
	MovementMethodID   string `json:"movement_method_id"`
	ZeroTimeConnection bool   `json:"zero_time_connection"`
}

type JourneySnapshot struct {
	// TravelerIDs is an optional caller-owned roster for this one journey. The
	// route engine only preserves it; the Story layer validates participation.
	TravelerIDs           []string `json:"traveler_ids,omitempty"`
	JourneyID             string   `json:"journey_id"`
	RouteID               string   `json:"route_id"`
	RouteVersion          string   `json:"route_version"`
	DefinitionHash        string   `json:"definition_hash"`
	OriginLocationID      string   `json:"origin_location_id"`
	DestinationLocationID string   `json:"destination_location_id"`
	TotalMinutes          int64    `json:"total_minutes"`
	ElapsedMinutes        int64    `json:"elapsed_minutes"`
	CompletedSegmentIDs   []string `json:"completed_segment_ids"`
	CurrentSegmentID      string   `json:"current_segment_id,omitempty"`
	CurrentLocationID     string   `json:"current_location_id"`
	SegmentElapsedMinutes int64    `json:"segment_elapsed_minutes"`
}

type JourneyPosition struct {
	ReferenceID           string `json:"reference_id"`
	LocationID            string `json:"location_id,omitempty"`
	RouteID               string `json:"route_id,omitempty"`
	RouteVersion          string `json:"route_version,omitempty"`
	SegmentID             string `json:"segment_id,omitempty"`
	SegmentElapsedMinutes int64  `json:"segment_elapsed_minutes,omitempty"`
	AtBoundary            bool   `json:"at_boundary"`
}

type AdvanceResult struct {
	Snapshot         JourneySnapshot `json:"snapshot"`
	Position         JourneyPosition `json:"position"`
	AddedMinutes     int64           `json:"added_minutes"`
	RemainingMinutes int64           `json:"remaining_minutes"`
	Done             bool            `json:"done"`
}

func ValidateRoute(route RouteDefinition, supportedMethods []string) (int64, string, []Diagnostic) {
	var ds []Diagnostic
	for _, d := range requireID(route.ID, "id") {
		ds = append(ds, d)
	}
	for _, d := range requireID(route.Version, "version") {
		ds = append(ds, d)
	}
	for _, d := range requireID(route.OriginLocationID, "origin_location_id") {
		ds = append(ds, d)
	}
	for _, d := range requireID(route.DestinationLocationID, "destination_location_id") {
		ds = append(ds, d)
	}
	if len(route.Segments) == 0 || len(route.Segments) > MaxRouteSegments {
		ds = append(ds, Diagnostic{"invalid_segments", "segments", "must contain between 1 and 4096 segments"})
	}
	methods := make(map[string]bool, len(supportedMethods))
	for i, method := range supportedMethods {
		if !validID(method) {
			ds = append(ds, Diagnostic{"invalid_supported_method", fmt.Sprintf("supported_methods[%d]", i), "must be a stable identifier"})
			continue
		}
		methods[method] = true
	}
	seen := map[string]bool{}
	var total int64
	previous := route.OriginLocationID
	for i, segment := range route.Segments {
		p := fmt.Sprintf("segments[%d]", i)
		for _, d := range requireID(segment.ID, p+".id") {
			ds = append(ds, d)
		}
		if seen[segment.ID] {
			ds = append(ds, Diagnostic{"duplicate_segment_id", p + ".id", "segment identifiers must be unique"})
		}
		seen[segment.ID] = true
		for _, d := range requireID(segment.FromLocationID, p+".from_location_id") {
			ds = append(ds, d)
		}
		for _, d := range requireID(segment.ToLocationID, p+".to_location_id") {
			ds = append(ds, d)
		}
		if segment.FromLocationID != previous {
			ds = append(ds, Diagnostic{"disconnected_route", p + ".from_location_id", "must match the previous stop"})
		}
		if segment.FromLocationID == segment.ToLocationID {
			ds = append(ds, Diagnostic{"same_location_edge", p, "a route segment must connect distinct stops"})
		}
		if !validID(segment.MovementMethodID) || !methods[segment.MovementMethodID] {
			ds = append(ds, Diagnostic{"unsupported_movement_method", p + ".movement_method_id", "movement method is not in the caller's supported set"})
		}
		if segment.DurationMinutes < 0 {
			ds = append(ds, Diagnostic{"negative_duration", p + ".duration_minutes", "must be non-negative"})
		}
		if segment.DurationMinutes == 0 && !segment.ZeroTimeConnection {
			ds = append(ds, Diagnostic{"unmarked_zero_duration", p + ".duration_minutes", "zero duration requires an explicit zero-time connection"})
		}
		if segment.DurationMinutes > 0 && segment.ZeroTimeConnection {
			ds = append(ds, Diagnostic{"mislabelled_zero_connection", p + ".zero_time_connection", "must be false for positive duration"})
		}
		if segment.DurationMinutes > math.MaxInt64-total {
			ds = append(ds, Diagnostic{"duration_overflow", p + ".duration_minutes", "sum exceeds int64"})
		} else {
			total += segment.DurationMinutes
		}
		previous = segment.ToLocationID
	}
	if previous != route.DestinationLocationID {
		ds = append(ds, Diagnostic{"wrong_destination", "destination_location_id", "must match the final route stop"})
	}
	hash := routeHash(route)
	return total, hash, ds
}

func PrepareJourney(route RouteDefinition, journeyID string, supportedMethods []string) (JourneySnapshot, error) {
	total, hash, ds := ValidateRoute(route, supportedMethods)
	for _, d := range requireID(journeyID, "journey_id") {
		ds = append(ds, d)
	}
	if len(ds) > 0 {
		return JourneySnapshot{}, fmt.Errorf("%w: %s", ErrInvalidRoute, ds[0].Message)
	}
	return JourneySnapshot{JourneyID: journeyID, RouteID: route.ID, RouteVersion: route.Version, DefinitionHash: hash,
		OriginLocationID: route.OriginLocationID, DestinationLocationID: route.DestinationLocationID, TotalMinutes: total,
		CompletedSegmentIDs: []string{}, CurrentSegmentID: route.Segments[0].ID, CurrentLocationID: route.OriginLocationID}, nil
}

func AdvanceTo(route RouteDefinition, snapshot JourneySnapshot, targetElapsed int64, supportedMethods []string) (AdvanceResult, error) {
	total, hash, ds := ValidateRoute(route, supportedMethods)
	if len(ds) > 0 {
		return AdvanceResult{}, fmt.Errorf("%w: %s", ErrInvalidRoute, ds[0].Message)
	}
	if snapshot.JourneyID == "" || snapshot.RouteID != route.ID || snapshot.RouteVersion != route.Version || snapshot.DefinitionHash != hash ||
		snapshot.OriginLocationID != route.OriginLocationID || snapshot.DestinationLocationID != route.DestinationLocationID || snapshot.TotalMinutes != total {
		return AdvanceResult{}, fmt.Errorf("%w: snapshot does not match route identity and content", ErrInvalidProgress)
	}
	if snapshot.ElapsedMinutes < 0 || snapshot.ElapsedMinutes > total || targetElapsed < snapshot.ElapsedMinutes || targetElapsed > total {
		return AdvanceResult{}, fmt.Errorf("%w: target must be between current elapsed time and route total", ErrInvalidTarget)
	}
	if err := validateSnapshotProgress(route, snapshot); err != nil {
		return AdvanceResult{}, err
	}
	var completedMinutes int64
	for _, segment := range route.Segments[:len(snapshot.CompletedSegmentIDs)] {
		completedMinutes += segment.DurationMinutes
	}
	remainingTarget := targetElapsed - completedMinutes
	completed := append([]string{}, snapshot.CompletedSegmentIDs...)
	location := snapshot.CurrentLocationID
	currentSegment := ""
	segmentElapsed := int64(0)
	position := JourneyPosition{}
	for _, seg := range route.Segments[len(completed):] {
		if remainingTarget == 0 && seg.DurationMinutes > 0 {
			currentSegment = seg.ID
			segmentElapsed = 0
			position = JourneyPosition{ReferenceID: positionReference(route.ID, route.Version, seg.ID, 0), LocationID: location, RouteID: route.ID, RouteVersion: route.Version, SegmentID: seg.ID, AtBoundary: true}
			break
		}
		if seg.DurationMinutes == 0 || remainingTarget >= seg.DurationMinutes {
			if seg.DurationMinutes > 0 {
				remainingTarget -= seg.DurationMinutes
			}
			completed = append(completed, seg.ID)
			location = seg.ToLocationID
			continue
		}
		currentSegment = seg.ID
		segmentElapsed = remainingTarget
		remainingTarget = 0
		position = JourneyPosition{ReferenceID: positionReference(route.ID, route.Version, seg.ID, segmentElapsed), RouteID: route.ID, RouteVersion: route.Version, SegmentID: seg.ID, SegmentElapsedMinutes: segmentElapsed}
		break
	}
	if len(completed) == len(route.Segments) {
		location = route.DestinationLocationID
		position = JourneyPosition{ReferenceID: "location:" + location, LocationID: location, AtBoundary: true}
	}
	if currentSegment == "" && len(completed) < len(route.Segments) {
		// A zero-time segment may follow the target boundary; consume only those
		// explicit edges and stop at the next positive segment boundary.
		for len(completed) < len(route.Segments) && route.Segments[len(completed)].DurationMinutes == 0 {
			seg := route.Segments[len(completed)]
			completed = append(completed, seg.ID)
			location = seg.ToLocationID
		}
		if len(completed) == len(route.Segments) {
			position = JourneyPosition{ReferenceID: "location:" + location, LocationID: location, AtBoundary: true}
		} else {
			seg := route.Segments[len(completed)]
			currentSegment = seg.ID
			position = JourneyPosition{ReferenceID: positionReference(route.ID, route.Version, seg.ID, 0), LocationID: location, RouteID: route.ID, RouteVersion: route.Version, SegmentID: seg.ID, AtBoundary: true}
		}
	}
	result := JourneySnapshot{TravelerIDs: append([]string(nil), snapshot.TravelerIDs...), JourneyID: snapshot.JourneyID, RouteID: route.ID, RouteVersion: route.Version, DefinitionHash: hash,
		OriginLocationID: route.OriginLocationID, DestinationLocationID: route.DestinationLocationID, TotalMinutes: total,
		ElapsedMinutes: targetElapsed, CompletedSegmentIDs: completed, CurrentSegmentID: currentSegment, CurrentLocationID: location, SegmentElapsedMinutes: segmentElapsed}
	return AdvanceResult{Snapshot: result, Position: position, AddedMinutes: targetElapsed - snapshot.ElapsedMinutes, RemainingMinutes: total - targetElapsed, Done: len(completed) == len(route.Segments)}, nil
}

func validateSnapshotProgress(route RouteDefinition, s JourneySnapshot) error {
	if s.CompletedSegmentIDs == nil || len(s.CompletedSegmentIDs) > len(route.Segments) {
		return fmt.Errorf("%w: completed prefix is invalid", ErrInvalidProgress)
	}
	var sum int64
	location := route.OriginLocationID
	for i, id := range s.CompletedSegmentIDs {
		seg := route.Segments[i]
		if seg.ID != id || seg.FromLocationID != location {
			return fmt.Errorf("%w: completed segments are not a route prefix", ErrInvalidProgress)
		}
		if seg.DurationMinutes > math.MaxInt64-sum {
			return fmt.Errorf("%w: elapsed time overflows", ErrTimeOverflow)
		}
		sum += seg.DurationMinutes
		location = seg.ToLocationID
	}
	if len(s.CompletedSegmentIDs) == len(route.Segments) {
		if s.CurrentSegmentID != "" || s.SegmentElapsedMinutes != 0 || s.CurrentLocationID != route.DestinationLocationID || sum != s.ElapsedMinutes {
			return fmt.Errorf("%w: completed snapshot fields disagree", ErrInvalidProgress)
		}
		return nil
	}
	seg := route.Segments[len(s.CompletedSegmentIDs)]
	invalidOffset := s.SegmentElapsedMinutes < 0 || (seg.DurationMinutes == 0 && s.SegmentElapsedMinutes != 0) || (seg.DurationMinutes > 0 && s.SegmentElapsedMinutes >= seg.DurationMinutes)
	if s.CurrentSegmentID != seg.ID || s.CurrentLocationID != location || invalidOffset || sum+s.SegmentElapsedMinutes != s.ElapsedMinutes {
		return fmt.Errorf("%w: current segment progress disagrees with route", ErrInvalidProgress)
	}
	return nil
}

func routeHash(route RouteDefinition) string {
	b, _ := json.Marshal(route)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func positionReference(routeID, version, segmentID string, offset int64) string {
	canonical, _ := json.Marshal([]string{routeID, version, segmentID, strconv.FormatInt(offset, 10)})
	sum := sha256.Sum256(canonical)
	return "routepos:" + hex.EncodeToString(sum[:])
}
