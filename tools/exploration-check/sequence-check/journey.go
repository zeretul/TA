package main

import (
	"encoding/json"
	"fmt"

	"ta/internal/exploration"
)

type journeyStop struct {
	TargetElapsed int64                       `json:"target_elapsed_minutes"`
	AddedMinutes  int64                       `json:"added_minutes"`
	Snapshot      exploration.JourneySnapshot `json:"snapshot"`
	Position      exploration.JourneyPosition `json:"position"`
}

type detourResult struct {
	OriginReference   string                      `json:"origin_reference"`
	Destination       string                      `json:"destination"`
	AddedMinutes      int64                       `json:"added_minutes"`
	OriginalUnchanged bool                        `json:"original_journey_unchanged"`
	Snapshot          exploration.JourneySnapshot `json:"snapshot"`
}

type journeyScenarioReport struct {
	RouteTotalMinutes int64         `json:"route_total_minutes"`
	Targets           []int64       `json:"targets_elapsed_minutes"`
	Interrupted       []journeyStop `json:"interrupted_stops"`
	Uninterrupted     []journeyStop `json:"uninterrupted_stops"`
	AddedMinutesTotal int64         `json:"added_minutes_total"`
	SameTargetAdded   int64         `json:"same_target_retry_added_minutes"`
	Detour            detourResult  `json:"detour"`
	SnapshotsEqual    bool          `json:"interrupted_matches_uninterrupted"`
}

func runJourneyScenario(route exploration.RouteDefinition, total int64, report *suiteReport) journeyScenarioReport {
	targets := []int64{0, 5, 8, total}
	interrupted, interruptedStops, err := advanceStops(route, "journey.north-road", targets, true)
	if err != nil {
		recordCheck(report, "journey_interrupted_sequence", false, "all core advances succeed", err.Error())
		return journeyScenarioReport{RouteTotalMinutes: total, Targets: targets}
	}
	uninterrupted, uninterruptedStops, err := advanceStops(route, "journey.north-road", targets, false)
	if err != nil {
		recordCheck(report, "journey_uninterrupted_sequence", false, "all core advances succeed", err.Error())
		return journeyScenarioReport{RouteTotalMinutes: total, Targets: targets, Interrupted: interruptedStops}
	}
	journeyEqual := sameJourneyStops(uninterruptedStops, interruptedStops) == nil && sameJourneySnapshots(uninterrupted, interrupted) == nil
	recordCheck(report, "journey_json_round_trip_equivalence", journeyEqual, "core advance results and snapshot sequence equal uninterrupted calculation", fmt.Sprintf("equal=%t final_elapsed=%d", journeyEqual, interrupted[len(interrupted)-1].ElapsedMinutes))
	addedTotal := int64(0)
	for _, stop := range interruptedStops {
		addedTotal += stop.AddedMinutes
	}
	recordCheck(report, "journey_added_minutes_sum", addedTotal == total, fmt.Sprintf("sum of segment increments is %d", total), fmt.Sprintf("sum=%d", addedTotal))
	boundaryReached := len(interruptedStops) > 2 && interruptedStops[2].Snapshot.ElapsedMinutes == 8 && interruptedStops[2].Snapshot.CurrentLocationID == "ridge.foot"
	recordCheck(report, "journey_zero_time_boundary", boundaryReached, "elapsed 8 reaches ridge.foot and consumes explicit zero-time connection", fmt.Sprintf("elapsed=%d location=%s", interruptedStops[2].Snapshot.ElapsedMinutes, interruptedStops[2].Snapshot.CurrentLocationID))
	last := interrupted[len(interrupted)-1]
	retry, err := exploration.AdvanceTo(route, last, total, []string{"walk", "pass-through"})
	if err != nil {
		recordCheck(report, "journey_same_target_retry", false, "retry at current target succeeds with zero added minutes", err.Error())
	} else {
		recordCheck(report, "journey_same_target_retry", retry.AddedMinutes == 0 && retry.Snapshot.ElapsedMinutes == total, "retry at current target adds zero minutes", fmt.Sprintf("added=%d elapsed=%d", retry.AddedMinutes, retry.Snapshot.ElapsedMinutes))
	}

	mid := interruptedStops[1]
	originalBefore, _ := json.Marshal(mid.Snapshot)
	detourRoute := exploration.RouteDefinition{ID: "sequence.detour", Version: "1.0", OriginLocationID: mid.Position.ReferenceID, DestinationLocationID: "detour.camp",
		Segments: []exploration.RouteSegment{{ID: "midpoint-to-camp", FromLocationID: mid.Position.ReferenceID, ToLocationID: "detour.camp", DurationMinutes: 7, MovementMethodID: "walk"}}}
	detour, detourErr := exploration.PrepareJourney(detourRoute, "journey.detour", []string{"walk"})
	detourAdded := int64(0)
	if detourErr == nil {
		var advanced exploration.AdvanceResult
		advanced, detourErr = exploration.AdvanceTo(detourRoute, detour, 7, []string{"walk"})
		if detourErr == nil {
			detourAdded = advanced.AddedMinutes
			detour = advanced.Snapshot
		}
	}
	originalAfter, _ := json.Marshal(mid.Snapshot)
	unchanged := string(originalBefore) == string(originalAfter)
	recordCheck(report, "journey_mid_route_detour", detourErr == nil && detourAdded == 7 && detour.ElapsedMinutes == 7 && mid.Position.ReferenceID != "" && unchanged, "detour starts at the core mid-route reference, advances seven core-calculated minutes, and leaves original proposal unchanged", fmt.Sprintf("origin=%s added=%d elapsed=%d error=%v unchanged=%t", mid.Position.ReferenceID, detourAdded, detour.ElapsedMinutes, detourErr, unchanged))
	return journeyScenarioReport{RouteTotalMinutes: total, Targets: targets, Interrupted: interruptedStops, Uninterrupted: uninterruptedStops,
		AddedMinutesTotal: addedTotal, SameTargetAdded: retry.AddedMinutes, Detour: detourResult{OriginReference: mid.Position.ReferenceID, Destination: "detour.camp", AddedMinutes: detourAdded, OriginalUnchanged: unchanged, Snapshot: detour}, SnapshotsEqual: journeyEqual}
}

func advanceStops(route exploration.RouteDefinition, journeyID string, targets []int64, roundTripMidpoint bool) ([]exploration.JourneySnapshot, []journeyStop, error) {
	snapshot, err := exploration.PrepareJourney(route, journeyID, []string{"walk", "pass-through"})
	if err != nil {
		return nil, nil, err
	}
	snapshots := make([]exploration.JourneySnapshot, 0, len(targets))
	stops := make([]journeyStop, 0, len(targets))
	for _, target := range targets {
		advanced, advanceErr := exploration.AdvanceTo(route, snapshot, target, []string{"walk", "pass-through"})
		if advanceErr != nil {
			return snapshots, stops, advanceErr
		}
		snapshot = advanced.Snapshot
		if roundTripMidpoint && target == 5 {
			snapshot, err = jsonRoundTrip(snapshot)
			if err != nil {
				return snapshots, stops, err
			}
		}
		snapshots = append(snapshots, snapshot)
		stops = append(stops, journeyStop{TargetElapsed: target, AddedMinutes: advanced.AddedMinutes, Snapshot: snapshot, Position: advanced.Position})
	}
	return snapshots, stops, nil
}
