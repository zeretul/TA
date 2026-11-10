package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validRouteJSON = `[{"id":"route_test","from":"place-a","to":"place-b","bidirectional":true,"baseline_elapsed_minutes":30,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"internal","access_condition":"public_only","disclosure":"known_route","status":"author_candidate"}]`

func TestManifestedSyntheticAuthorRoutesConvertWithoutInventingEdges(t *testing.T) {
	result := loadConversion("testdata", "catalog.json")
	if !result.Report.Valid {
		t.Fatalf("manifested synthetic inputs did not validate: %+v", result.Report.Diagnostics)
	}
	if result.Report.AuthorVersion != "1.0" || result.Report.CatalogVersion != "1.1.0" {
		t.Fatalf("source versions were not preserved: report=%+v", result.Report)
	}
	if result.Report.Counts["author_route_records"] == 0 || result.Report.Counts["candidate_routes"] != result.Report.Counts["candidate_forward_routes"]+result.Report.Counts["candidate_reverse_routes"] {
		t.Fatalf("route counts are inconsistent: %+v", result.Report.Counts)
	}
	if result.Report.Counts["route_endpoints_not_in_places"] != 0 || result.Report.Counts["route_endpoints_not_in_authored_locations"] == 0 {
		t.Fatalf("place versus authored-location semantics were lost: %+v", result.Report.Counts)
	}
	if len(result.Report.Demonstrations) < 2 {
		t.Fatalf("fewer than two distinct route scale demonstrations: %+v", result.Report.Demonstrations)
	}
	for _, candidate := range result.Candidates {
		if candidate.Route.Version != result.Report.AuthorVersion || candidate.SourceSHA256 != result.Report.Hashes.AuthorSourceSHA256 || candidate.SourceRouteID == candidate.DirectionID {
			t.Fatalf("candidate provenance was not retained: %+v", candidate)
		}
		if candidate.Route.Segments[0].DurationMinutes <= 0 || candidate.AccessCondition == "" || candidate.Disclosure == "" || candidate.Status == "" {
			t.Fatalf("candidate lost author constraints or baseline minutes: %+v", candidate)
		}
	}
}

func TestBidirectionalAndOneWayRoutesPreserveDirectionAndDuration(t *testing.T) {
	data := []byte(`[
		{"id":"bi","from":"place-a","to":"place-b","bidirectional":true,"baseline_elapsed_minutes":4320,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"regional","access_condition":"member_or_guest","disclosure":"public_when_known","status":"not_verified_canon"},
		{"id":"one","from":"place-b","to":"place-c","bidirectional":false,"baseline_elapsed_minutes":14400,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"domestic_long","access_condition":"permission","disclosure":"private","status":"not_verified_canon"}
	]`)
	parsed := parseRouteArray(data, "routes")
	if len(parsed.Diagnostics) != 0 || len(parsed.Routes) != 2 {
		t.Fatalf("valid route rows rejected: %+v", parsed.Diagnostics)
	}
	duplicates := duplicateRouteIDs(&parsed, "routes")
	places := map[string]PlaceIdentity{"place-a": {ID: "place-a", Kind: "regional_hub"}, "place-b": {ID: "place-b", Kind: "travel_junction"}, "place-c": {ID: "place-c", Kind: "authored_place"}}
	candidates, diagnostics := convertAuthorRoutes(parsed.Routes, parsed.RowIndexes, duplicates, "1.0", "1.1.0", "design-source.json", strings.Repeat("a", 64), places)
	if len(diagnostics) != 0 || len(candidates) != 3 {
		t.Fatalf("direction conversion failed: candidates=%+v diagnostics=%+v", candidates, diagnostics)
	}
	byID := make(map[string]CandidateRoute, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.DirectionID] = candidate
	}
	forward := byID["bi:forward"]
	reverse := byID["bi:reverse"]
	oneWay := byID["one:forward"]
	if forward.Route.OriginLocationID != "place-a" || forward.Route.DestinationLocationID != "place-b" || reverse.Route.OriginLocationID != "place-b" || reverse.Route.DestinationLocationID != "place-a" {
		t.Fatalf("bidirectional endpoints changed incorrectly: forward=%+v reverse=%+v", forward.Route, reverse.Route)
	}
	if forward.Route.Segments[0].DurationMinutes != 4320 || reverse.Route.Segments[0].DurationMinutes != 4320 || oneWay.Route.Segments[0].DurationMinutes != 14400 || oneWay.Route.DestinationLocationID != "place-c" {
		t.Fatalf("author baseline duration or one-way destination changed: %+v", candidates)
	}
	if _, exists := byID["one:reverse"]; exists || forward.AccessCondition != "member_or_guest" || reverse.Disclosure != "public_when_known" {
		t.Fatalf("one-way restriction or route metadata lost: %+v", candidates)
	}
}

func TestRouteParserRejectsMissingNullFractionDirectionAndUnknownEndpoint(t *testing.T) {
	cases := []struct {
		name string
		json string
		code string
	}{
		{"missing minutes", strings.Replace(validRouteJSON, `,"baseline_elapsed_minutes":30`, "", 1), "invalid_route_duration"},
		{"null minutes", strings.Replace(validRouteJSON, `"baseline_elapsed_minutes":30`, `"baseline_elapsed_minutes":null`, 1), "invalid_route_duration"},
		{"fractional minutes", strings.Replace(validRouteJSON, `"baseline_elapsed_minutes":30`, `"baseline_elapsed_minutes":30.5`, 1), "invalid_route_duration"},
		{"missing direction", strings.Replace(validRouteJSON, `,"bidirectional":true`, "", 1), "invalid_route_direction"},
		{"null direction", strings.Replace(validRouteJSON, `"bidirectional":true`, `"bidirectional":null`, 1), "invalid_route_direction"},
		{"string direction", strings.Replace(validRouteJSON, `"bidirectional":true`, `"bidirectional":"both"`, 1), "invalid_route_direction"},
		{"unknown author field", strings.Replace(validRouteJSON, `"scale":"internal"`, `"invented_duration":15,"scale":"internal"`, 1), "unknown_route_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseRouteArray([]byte(tc.json), "routes")
			if !containsCode(parsed.Diagnostics, tc.code) || len(parsed.Routes) != 0 {
				t.Fatalf("invalid row accepted or expected diagnostic missing: %+v", parsed)
			}
		})
	}
	valid := parseRouteArray([]byte(validRouteJSON), "routes")
	duplicates := duplicateRouteIDs(&valid, "routes")
	places := map[string]PlaceIdentity{"place-a": {ID: "place-a", Kind: "regional_hub"}}
	candidates, diagnostics := convertAuthorRoutes(valid.Routes, valid.RowIndexes, duplicates, "1.0", "1.1.0", "source.json", strings.Repeat("a", 64), places)
	if len(candidates) != 0 || !containsCode(diagnostics, "unknown_route_endpoint") {
		t.Fatalf("unknown route endpoint was not rejected: candidates=%+v diagnostics=%+v", candidates, diagnostics)
	}
}

func TestZeroMinutesNeedsExplicitZeroTimeConnection(t *testing.T) {
	withoutFlag := strings.Replace(validRouteJSON, `"baseline_elapsed_minutes":30`, `"baseline_elapsed_minutes":0`, 1)
	parsed := parseRouteArray([]byte(withoutFlag), "routes")
	duplicates := duplicateRouteIDs(&parsed, "routes")
	places := map[string]PlaceIdentity{"place-a": {ID: "place-a", Kind: "travel_junction"}, "place-b": {ID: "place-b", Kind: "regional_hub"}}
	candidates, diagnostics := convertAuthorRoutes(parsed.Routes, parsed.RowIndexes, duplicates, "1.0", "1.1.0", "source.json", strings.Repeat("a", 64), places)
	if len(candidates) != 0 || !containsCode(diagnostics, "zero_duration_not_declared") {
		t.Fatalf("undeclared zero-time route passed: candidates=%+v diagnostics=%+v", candidates, diagnostics)
	}
	withFlag := strings.Replace(withoutFlag, `"scale":"internal"`, `"zero_time_connection":true,"scale":"internal"`, 1)
	parsed = parseRouteArray([]byte(withFlag), "routes")
	duplicates = duplicateRouteIDs(&parsed, "routes")
	candidates, diagnostics = convertAuthorRoutes(parsed.Routes, parsed.RowIndexes, duplicates, "1.0", "1.1.0", "source.json", strings.Repeat("a", 64), places)
	if len(diagnostics) != 0 || len(candidates) != 2 || !candidates[0].Route.Segments[0].ZeroTimeConnection {
		t.Fatalf("explicit zero-time connection did not follow core contract: candidates=%+v diagnostics=%+v", candidates, diagnostics)
	}
}

func TestDuplicateRouteIDsInvalidateEveryDuplicateButKeepOtherRows(t *testing.T) {
	data := []byte(`[
		{"id":"duplicate","from":"place-a","to":"place-b","bidirectional":true,"baseline_elapsed_minutes":30,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"internal","access_condition":"public_only","disclosure":"known_route","status":"author_candidate"},
		{"id":"duplicate","from":"place-b","to":"place-a","bidirectional":false,"baseline_elapsed_minutes":30,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"internal","access_condition":"public_only","disclosure":"known_route","status":"author_candidate"},
		{"id":"unique","from":"place-a","to":"place-b","bidirectional":false,"baseline_elapsed_minutes":30,"baseline_method":"ordinary_low_tier_travel_with_normal_rests","scale":"internal","access_condition":"public_only","disclosure":"known_route","status":"author_candidate"}
	]`)
	parsed := parseRouteArray(data, "routes")
	duplicates := duplicateRouteIDs(&parsed, "routes")
	if !duplicates["duplicate"] || !containsCode(parsed.Diagnostics, "duplicate_route_id") || len(parsed.Diagnostics) != 2 {
		t.Fatalf("duplicate route ID not diagnosed: %+v", parsed.Diagnostics)
	}
	places := map[string]PlaceIdentity{"place-a": {ID: "place-a", Kind: "regional_hub"}, "place-b": {ID: "place-b", Kind: "travel_junction"}}
	candidates, _ := convertAuthorRoutes(parsed.Routes, parsed.RowIndexes, duplicates, "1.0", "1.1.0", "source.json", strings.Repeat("a", 64), places)
	if len(candidates) != 1 || candidates[0].SourceRouteID != "unique" {
		t.Fatalf("invalid duplicate rows were not omitted while valid rows survived: %+v", candidates)
	}
}

func TestSameBadRouteBytesProduceStableDiagnosticsAndInputIsUnchanged(t *testing.T) {
	bad := []byte(strings.Replace(validRouteJSON, `"baseline_elapsed_minutes":30`, `"baseline_elapsed_minutes":null`, 1))
	before := bytes.Clone(bad)
	first := ""
	for i := 0; i < 100; i++ {
		parsed := parseRouteArray(bad, "routes")
		encoded, err := json.Marshal(parsed.Diagnostics)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(encoded)
		} else if first != string(encoded) {
			t.Fatalf("same bad bytes changed diagnostics: first=%s now=%s", first, encoded)
		}
	}
	if !bytes.Equal(before, bad) {
		t.Fatal("route parsing mutated input bytes")
	}
}

func TestPlacesAndRouteCatalogComparisonAreStableAndStrict(t *testing.T) {
	sourcePlaces := []PlaceIdentity{{ID: "a", Kind: "authored_place"}, {ID: "b", Kind: "travel_junction"}}
	catalogPlaces := []PlaceIdentity{{ID: "a", Kind: "authored_place"}, {ID: "b", Kind: "travel_junction"}}
	if ds := comparePlaces(sourcePlaces, catalogPlaces); len(ds) != 0 {
		t.Fatalf("identical place identities mismatch: %+v", ds)
	}
	catalogPlaces[1].Kind = "faction_external_approach"
	if ds := comparePlaces(sourcePlaces, catalogPlaces); !containsCode(ds, "catalog_place_kind_mismatch") {
		t.Fatalf("changed endpoint kind was not rejected: %+v", ds)
	}
	parsed := parseRouteArray([]byte(validRouteJSON), "routes")
	changed := parsed.Routes[0]
	changed.BaselineElapsedMinutes++
	if ds := compareRoutes(parsed.Routes, []AuthorRoute{changed}, map[string]bool{}, map[string]bool{}); !containsCode(ds, "catalog_route_mismatch") {
		t.Fatalf("catalog/source route time mismatch not rejected: %+v", ds)
	}
}

func containsCode(items []Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestInvalidBindingToCountryOrFactionCannotMasqueradeAsPlace(t *testing.T) {
	parsed := parseRouteArray([]byte(strings.Replace(validRouteJSON, "place-b", "yue", 1)), "routes")
	duplicates := duplicateRouteIDs(&parsed, "routes")
	places := map[string]PlaceIdentity{"place-a": {ID: "place-a", Kind: "travel_junction"}}
	candidates, diagnostics := convertAuthorRoutes(parsed.Routes, parsed.RowIndexes, duplicates, "1.0", "1.1.0", "source.json", strings.Repeat("a", 64), places)
	if len(candidates) != 0 || !containsCode(diagnostics, "unknown_route_endpoint") {
		t.Fatalf("country-like ID bypassed place lookup: candidates=%+v diagnostics=%+v", candidates, diagnostics)
	}
	if parsed.Routes[0].To != "yue" {
		t.Fatal("route source identity unexpectedly changed")
	}
}

func TestMissingAuthorCatalogIsReported(t *testing.T) {
	result := loadConversion(t.TempDir(), "missing-catalog.json")
	if result.Report.Valid || !containsCode(result.Report.Diagnostics, "catalog_read_failed") {
		t.Fatalf("missing catalog was not reported: %+v", result.Report)
	}
}

func TestManifestedSyntheticAuthorRejectsChangedSourceBytes(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "设计稿", "synthetic-author.json")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	result := loadConversion(root, "catalog.json")
	if result.Report.Valid || !containsCode(result.Report.Diagnostics, "author_source_hash_mismatch") {
		t.Fatalf("changed source bytes were accepted: %+v", result.Report)
	}
}
