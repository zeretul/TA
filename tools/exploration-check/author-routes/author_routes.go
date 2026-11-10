package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"ta/internal/exploration"
)

const (
	maxCatalogBytes = 32 << 20
	maxSourceBytes  = 16 << 20
	maxRouteRows    = 10_000
	maxPlaceRows    = 20_000
)

var supportedMovementMethods = []string{"ordinary_low_tier_travel_with_normal_rests"}

type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type AuthorRoute struct {
	ID                     string `json:"id"`
	From                   string `json:"from"`
	To                     string `json:"to"`
	Bidirectional          bool   `json:"bidirectional"`
	BaselineElapsedMinutes int64  `json:"baseline_elapsed_minutes"`
	BaselineMethod         string `json:"baseline_method"`
	Scale                  string `json:"scale"`
	AccessCondition        string `json:"access_condition"`
	Disclosure             string `json:"disclosure"`
	Status                 string `json:"status"`
	ZeroTimeConnection     bool   `json:"zero_time_connection,omitempty"`
}

type RouteIdentity struct {
	ID    string
	Index int
}

type ParsedRoutes struct {
	Routes      []AuthorRoute
	RowIndexes  []int
	Identities  []RouteIdentity
	Diagnostics []Diagnostic
}

type PlaceIdentity struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type CandidateRoute struct {
	DirectionID         string                      `json:"direction_id"`
	SourceRouteID       string                      `json:"source_route_id"`
	Direction           string                      `json:"direction"`
	SourceVersion       string                      `json:"source_version"`
	CatalogVersion      string                      `json:"catalog_version"`
	SourcePath          string                      `json:"source_path"`
	SourceSHA256        string                      `json:"source_sha256"`
	Scale               string                      `json:"scale"`
	AccessCondition     string                      `json:"access_condition"`
	Disclosure          string                      `json:"disclosure"`
	Status              string                      `json:"status"`
	SourceBidirectional bool                        `json:"source_bidirectional"`
	FromPlaceKind       string                      `json:"from_place_kind"`
	ToPlaceKind         string                      `json:"to_place_kind"`
	DefinitionHash      string                      `json:"definition_hash"`
	Route               exploration.RouteDefinition `json:"route"`
}

type InputHashes struct {
	StaticCatalogSHA256    string `json:"static_catalog_sha256"`
	AuthorSourceSHA256     string `json:"author_source_sha256"`
	ManifestDeclaredSHA256 string `json:"manifest_declared_sha256"`
}

type RouteDemonstration struct {
	DirectionID                  string `json:"direction_id"`
	SourceRouteID                string `json:"source_route_id"`
	Scale                        string `json:"scale"`
	TotalMinutes                 int64  `json:"total_minutes"`
	ZeroTargetAddedMinutes       int64  `json:"zero_target_added_minutes"`
	ZeroSnapshotJSONRoundTrip    bool   `json:"zero_snapshot_json_round_trip"`
	MidTargetElapsedMinutes      int64  `json:"mid_target_elapsed_minutes"`
	MidTargetAddedMinutes        int64  `json:"mid_target_added_minutes"`
	ContinueTargetElapsedMinutes int64  `json:"continue_target_elapsed_minutes"`
	ContinueAddedMinutes         int64  `json:"continue_added_minutes"`
	SameTargetRetryAddedMinutes  int64  `json:"same_target_retry_added_minutes"`
	RemainingMinutes             int64  `json:"remaining_minutes"`
	Done                         bool   `json:"done"`
	FinalSnapshotJSONRoundTrip   bool   `json:"final_snapshot_json_round_trip"`
	RoundTripIsGameRecovery      bool   `json:"round_trip_is_game_recovery"`
}

type Report struct {
	SchemaVersion  int                  `json:"schema_version"`
	Contract       string               `json:"contract"`
	Valid          bool                 `json:"valid"`
	NotInstallable bool                 `json:"not_installable"`
	CatalogVersion string               `json:"catalog_version,omitempty"`
	AuthorVersion  string               `json:"author_version,omitempty"`
	SourcePath     string               `json:"source_path,omitempty"`
	Hashes         InputHashes          `json:"hashes"`
	Counts         map[string]int       `json:"counts"`
	Demonstrations []RouteDemonstration `json:"demonstrations"`
	Diagnostics    []Diagnostic         `json:"diagnostics"`
}

type CandidateFile struct {
	SchemaVersion  int              `json:"schema_version"`
	Contract       string           `json:"contract"`
	Valid          bool             `json:"valid"`
	NotInstallable bool             `json:"not_installable"`
	Hashes         InputHashes      `json:"hashes"`
	Routes         []CandidateRoute `json:"routes"`
	Diagnostics    []Diagnostic     `json:"diagnostics"`
}

type conversion struct {
	Report     Report
	Candidates []CandidateRoute
}

type routeRecord struct {
	Route AuthorRoute
	Index int
}

type manifestEntry struct {
	ID              string
	Format          string
	DeclaredVersion string
	Path            string
	SHA256          string
}

func parseRouteArray(data []byte, diagnosticPath string) ParsedRoutes {
	parsed := ParsedRoutes{Routes: []AuthorRoute{}, Identities: []RouteIdentity{}, Diagnostics: []Diagnostic{}}
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil || rawRows == nil {
		parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"routes_not_array", diagnosticPath, "must be a non-null JSON array"})
		return parsed
	}
	if len(rawRows) > maxRouteRows {
		parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"route_limit_exceeded", diagnosticPath, fmt.Sprintf("route count exceeds %d", maxRouteRows)})
		return parsed
	}
	for i, raw := range rawRows {
		rowPath := fmt.Sprintf("%s[%d]", diagnosticPath, i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"route_not_object", rowPath, "route entry must be a JSON object"})
			continue
		}
		id, idOK := rawString(fields, "id")
		if idOK && strings.TrimSpace(id) != "" {
			parsed.Identities = append(parsed.Identities, RouteIdentity{ID: id, Index: i})
		}
		allowed := map[string]bool{"id": true, "from": true, "to": true, "bidirectional": true, "baseline_elapsed_minutes": true, "baseline_method": true, "scale": true, "access_condition": true, "disclosure": true, "status": true, "zero_time_connection": true}
		unknown := make([]string, 0)
		for key := range fields {
			if !allowed[key] {
				unknown = append(unknown, key)
			}
		}
		sort.Strings(unknown)
		rowValid := true
		for _, key := range unknown {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"unknown_route_field", rowPath + "." + key, "field is not part of the author-route schema"})
			rowValid = false
		}
		row := AuthorRoute{ID: id}
		if !idOK || strings.TrimSpace(id) == "" {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_string", rowPath + ".id", "required field must be a non-empty JSON string"})
			rowValid = false
		}
		for _, field := range []struct {
			name string
			to   *string
		}{{"from", &row.From}, {"to", &row.To}, {"baseline_method", &row.BaselineMethod}, {"scale", &row.Scale}, {"access_condition", &row.AccessCondition}, {"disclosure", &row.Disclosure}, {"status", &row.Status}} {
			value, ok := rawString(fields, field.name)
			if !ok {
				parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_string", rowPath + "." + field.name, "required field must be a non-null JSON string"})
				rowValid = false
				continue
			}
			*field.to = value
		}
		if raw, ok := fields["bidirectional"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_direction", rowPath + ".bidirectional", "must be an explicitly supplied non-null JSON boolean"})
			rowValid = false
		} else if err := json.Unmarshal(raw, &row.Bidirectional); err != nil {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_direction", rowPath + ".bidirectional", "must be a JSON boolean"})
			rowValid = false
		}
		if raw, ok := fields["baseline_elapsed_minutes"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_duration", rowPath + ".baseline_elapsed_minutes", "must be an explicitly supplied non-null integer"})
			rowValid = false
		} else if err := json.Unmarshal(raw, &row.BaselineElapsedMinutes); err != nil {
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_route_duration", rowPath + ".baseline_elapsed_minutes", "must be an integer representable as int64"})
			rowValid = false
		}
		if raw, ok := fields["zero_time_connection"]; ok {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_zero_time_declaration", rowPath + ".zero_time_connection", "explicit declaration cannot be null"})
				rowValid = false
			} else if err := json.Unmarshal(raw, &row.ZeroTimeConnection); err != nil {
				parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"invalid_zero_time_declaration", rowPath + ".zero_time_connection", "explicit declaration must be a JSON boolean"})
				rowValid = false
			}
		}
		if rowValid {
			parsed.Routes = append(parsed.Routes, row)
			parsed.RowIndexes = append(parsed.RowIndexes, i)
		}
	}
	return parsed
}

func rawString(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func duplicateRouteIDs(parsed *ParsedRoutes, diagnosticPath string) map[string]bool {
	counts := make(map[string]int, len(parsed.Identities))
	for _, identity := range parsed.Identities {
		counts[identity.ID]++
	}
	duplicates := make(map[string]bool)
	for _, identity := range parsed.Identities {
		if counts[identity.ID] > 1 {
			duplicates[identity.ID] = true
			parsed.Diagnostics = append(parsed.Diagnostics, Diagnostic{"duplicate_route_id", fmt.Sprintf("%s[%d].id", diagnosticPath, identity.Index), "duplicate route identity invalidates every row with that ID"})
		}
	}
	return duplicates
}

func parsePlaceArray(data []byte, diagnosticPath string) ([]PlaceIdentity, []Diagnostic) {
	places := []PlaceIdentity{}
	var diagnostics []Diagnostic
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil || rawRows == nil {
		return places, []Diagnostic{{"places_not_array", diagnosticPath, "must be a non-null JSON array"}}
	}
	if len(rawRows) > maxPlaceRows {
		return places, []Diagnostic{{"place_limit_exceeded", diagnosticPath, fmt.Sprintf("place count exceeds %d", maxPlaceRows)}}
	}
	seen := make(map[string]bool, len(rawRows))
	for i, raw := range rawRows {
		p := fmt.Sprintf("%s[%d]", diagnosticPath, i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			diagnostics = append(diagnostics, Diagnostic{"place_not_object", p, "place entry must be a JSON object"})
			continue
		}
		id, idOK := rawString(fields, "id")
		kind, kindOK := rawString(fields, "kind")
		valid := true
		if !idOK || strings.TrimSpace(id) == "" {
			diagnostics = append(diagnostics, Diagnostic{"invalid_place_id", p + ".id", "place ID must be a non-empty JSON string"})
			valid = false
		}
		if !kindOK || strings.TrimSpace(kind) == "" {
			diagnostics = append(diagnostics, Diagnostic{"invalid_place_kind", p + ".kind", "place kind must be a non-empty JSON string"})
			valid = false
		}
		if !valid {
			continue
		}
		if seen[id] {
			diagnostics = append(diagnostics, Diagnostic{"duplicate_place_id", p + ".id", "place IDs must be unique"})
			continue
		}
		seen[id] = true
		places = append(places, PlaceIdentity{ID: id, Kind: kind})
	}
	return places, diagnostics
}

func comparePlaces(source, catalog []PlaceIdentity) []Diagnostic {
	var diagnostics []Diagnostic
	sourceByID := make(map[string]string, len(source))
	catalogByID := make(map[string]string, len(catalog))
	for _, item := range source {
		sourceByID[item.ID] = item.Kind
	}
	for _, item := range catalog {
		catalogByID[item.ID] = item.Kind
	}
	sourceIDs := make([]string, 0, len(sourceByID))
	for id := range sourceByID {
		sourceIDs = append(sourceIDs, id)
	}
	sort.Strings(sourceIDs)
	for _, id := range sourceIDs {
		kind, exists := catalogByID[id]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{"catalog_place_missing", "places." + id, "place from the author source is missing in the static catalog"})
		} else if kind != sourceByID[id] {
			diagnostics = append(diagnostics, Diagnostic{"catalog_place_kind_mismatch", "places." + id + ".kind", "place kind differs from the author source"})
		}
	}
	catalogIDs := make([]string, 0, len(catalogByID))
	for id := range catalogByID {
		catalogIDs = append(catalogIDs, id)
	}
	sort.Strings(catalogIDs)
	for _, id := range catalogIDs {
		if _, exists := sourceByID[id]; !exists {
			diagnostics = append(diagnostics, Diagnostic{"author_place_missing", "places." + id, "static-catalog place is missing in the author source"})
		}
	}
	return diagnostics
}

func convertAuthorRoutes(routes []AuthorRoute, rowIndexes []int, duplicateIDs map[string]bool, sourceVersion, catalogVersion, sourcePath, sourceHash string, places map[string]PlaceIdentity) ([]CandidateRoute, []Diagnostic) {
	candidates := []CandidateRoute{}
	diagnostics := []Diagnostic{}
	for index, row := range routes {
		rowPath := fmt.Sprintf("author_source.routes[%d]", rowIndexes[index])
		if duplicateIDs[row.ID] {
			continue
		}
		if strings.TrimSpace(row.Scale) == "" || strings.TrimSpace(row.AccessCondition) == "" || strings.TrimSpace(row.Disclosure) == "" || strings.TrimSpace(row.Status) == "" {
			diagnostics = append(diagnostics, Diagnostic{"missing_route_constraint", rowPath, "scale and all author restrictions must be preserved as non-empty strings"})
			continue
		}
		_, fromOK := places[row.From]
		_, toOK := places[row.To]
		if !fromOK {
			diagnostics = append(diagnostics, Diagnostic{"unknown_route_endpoint", rowPath + ".from", "route endpoint does not resolve to a stable place ID"})
		}
		if !toOK {
			diagnostics = append(diagnostics, Diagnostic{"unknown_route_endpoint", rowPath + ".to", "route endpoint does not resolve to a stable place ID"})
		}
		if !fromOK || !toOK {
			continue
		}
		if row.BaselineElapsedMinutes == 0 && !row.ZeroTimeConnection {
			diagnostics = append(diagnostics, Diagnostic{"zero_duration_not_declared", rowPath + ".baseline_elapsed_minutes", "zero time is valid only with an explicit zero_time_connection declaration"})
			continue
		}
		for _, direction := range routeDirections(row.Bidirectional) {
			from, to := row.From, row.To
			if direction == "reverse" {
				from, to = to, from
			}
			directionID := row.ID + ":" + direction
			route := exploration.RouteDefinition{ID: directionID, Version: sourceVersion, OriginLocationID: from, DestinationLocationID: to, Segments: []exploration.RouteSegment{{ID: row.ID + ":" + direction + ":segment", FromLocationID: from, ToLocationID: to, DurationMinutes: row.BaselineElapsedMinutes, MovementMethodID: row.BaselineMethod, ZeroTimeConnection: row.ZeroTimeConnection}}}
			_, definitionHash, routeDiagnostics := exploration.ValidateRoute(route, supportedMovementMethods)
			if len(routeDiagnostics) > 0 {
				for _, item := range routeDiagnostics {
					diagnostics = append(diagnostics, Diagnostic{"route_" + item.Code, directionID + "." + item.Path, item.Message})
				}
				continue
			}
			candidates = append(candidates, CandidateRoute{DirectionID: directionID, SourceRouteID: row.ID, Direction: direction, SourceVersion: sourceVersion, CatalogVersion: catalogVersion, SourcePath: sourcePath, SourceSHA256: sourceHash, Scale: row.Scale, AccessCondition: row.AccessCondition, Disclosure: row.Disclosure, Status: row.Status, SourceBidirectional: row.Bidirectional, FromPlaceKind: places[from].Kind, ToPlaceKind: places[to].Kind, DefinitionHash: definitionHash, Route: route})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].DirectionID < candidates[j].DirectionID })
	return candidates, diagnostics
}

func routeDirections(bidirectional bool) []string {
	if bidirectional {
		return []string{"forward", "reverse"}
	}
	return []string{"forward"}
}

func compareRoutes(source, catalog []AuthorRoute, sourceDuplicates, catalogDuplicates map[string]bool) []Diagnostic {
	var diagnostics []Diagnostic
	catalogByID := make(map[string]AuthorRoute, len(catalog))
	for _, row := range catalog {
		if !catalogDuplicates[row.ID] {
			catalogByID[row.ID] = row
		}
	}
	sourceByID := make(map[string]AuthorRoute, len(source))
	for _, row := range source {
		if sourceDuplicates[row.ID] {
			continue
		}
		sourceByID[row.ID] = row
	}
	ids := make([]string, 0, len(sourceByID))
	for id := range sourceByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		catalogRow, exists := catalogByID[id]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{"catalog_route_missing", "catalog.routes." + id, "author route is missing in the static catalog"})
		} else if !reflect.DeepEqual(sourceByID[id], catalogRow) {
			diagnostics = append(diagnostics, Diagnostic{"catalog_route_mismatch", "catalog.routes." + id, "route values differ from the manifested author source"})
		}
	}
	catalogIDs := make([]string, 0, len(catalogByID))
	for id := range catalogByID {
		catalogIDs = append(catalogIDs, id)
	}
	sort.Strings(catalogIDs)
	for _, id := range catalogIDs {
		if _, exists := sourceByID[id]; !exists {
			diagnostics = append(diagnostics, Diagnostic{"author_route_missing", "author_source.routes." + id, "static-catalog route is missing in the manifested author source"})
		}
	}
	return diagnostics
}

func parseSourceManifest(raw json.RawMessage) ([]manifestEntry, []Diagnostic) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		return nil, []Diagnostic{{"source_manifest_invalid", "source_manifest", "must be a non-null JSON array"}}
	}
	var matches []manifestEntry
	for i, item := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil || fields == nil {
			continue
		}
		id, ok := rawString(fields, "id")
		if !ok || id != "handoff" {
			continue
		}
		entry := manifestEntry{}
		for _, field := range []struct {
			name string
			to   *string
		}{{"format", &entry.Format}, {"declared_version", &entry.DeclaredVersion}, {"path", &entry.Path}, {"sha256", &entry.SHA256}} {
			value, found := rawString(fields, field.name)
			if !found {
				return nil, []Diagnostic{{"source_manifest_invalid", fmt.Sprintf("source_manifest[%d].%s", i, field.name), "handoff manifest field must be a non-null JSON string"}}
			}
			*field.to = value
		}
		entry.ID = id
		matches = append(matches, entry)
	}
	if len(matches) != 1 {
		return nil, []Diagnostic{{"handoff_manifest_count", "source_manifest", fmt.Sprintf("expected exactly one handoff source entry, found %d", len(matches))}}
	}
	return matches, nil
}

func resolveManifestPath(repoRoot, relative string) (string, error) {
	if strings.Contains(relative, `\`) || !strings.HasPrefix(relative, "设计稿/") || path.Clean(relative) != relative || strings.Contains(relative, ":") {
		return "", fmt.Errorf("manifest path is not a normalized design-source path")
	}
	local := filepath.FromSlash(relative)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("manifest path escapes the repository")
	}
	resolved := filepath.Join(repoRoot, local)
	current := repoRoot
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("manifest path contains a symbolic link")
		}
	}
	return resolved, nil
}

func manifestHashMatches(expected, actual string) bool {
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil || len(expectedBytes) != sha256.Size {
		return false
	}
	actualBytes, err := hex.DecodeString(actual)
	if err != nil || len(actualBytes) != sha256.Size {
		return false
	}
	return bytes.Equal(expectedBytes, actualBytes)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func decodeObject(data []byte, diagnosticPath string) (map[string]json.RawMessage, []Diagnostic) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, []Diagnostic{{"invalid_json", diagnosticPath, err.Error()}}
	}
	if object == nil {
		return nil, []Diagnostic{{"json_object_required", diagnosticPath, "top-level JSON value must be an object"}}
	}
	return object, nil
}

func requiredRaw(object map[string]json.RawMessage, key, diagnosticPath string) (json.RawMessage, []Diagnostic) {
	value, ok := object[key]
	if !ok {
		return nil, []Diagnostic{{"missing_field", diagnosticPath + "." + key, "required field is missing"}}
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, []Diagnostic{{"null_field", diagnosticPath + "." + key, "required field cannot be null"}}
	}
	return value, nil
}

func readRequiredString(object map[string]json.RawMessage, key, diagnosticPath string) (string, []Diagnostic) {
	raw, diagnostics := requiredRaw(object, key, diagnosticPath)
	if len(diagnostics) > 0 {
		return "", diagnostics
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return "", []Diagnostic{{"invalid_string", diagnosticPath + "." + key, "required field must be a non-empty JSON string"}}
	}
	return value, nil
}

func readRequiredArray(object map[string]json.RawMessage, key, diagnosticPath string) (json.RawMessage, []Diagnostic) {
	raw, diagnostics := requiredRaw(object, key, diagnosticPath)
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil || rows == nil {
		return nil, []Diagnostic{{"invalid_array", diagnosticPath + "." + key, "required field must be a non-null JSON array"}}
	}
	return raw, nil
}

func loadConversion(repoRoot, catalogPath string) conversion {
	result := conversion{Report: Report{SchemaVersion: 1, Contract: "denova author-route candidate v1 (not installed game data)", NotInstallable: true, Counts: map[string]int{}, Demonstrations: []RouteDemonstration{}, Diagnostics: []Diagnostic{}}, Candidates: []CandidateRoute{}}
	relativeCatalog := filepath.ToSlash(catalogPath)
	result.Report.Counts["catalog_routes"] = 0
	catalogFile := filepath.Join(repoRoot, filepath.FromSlash(catalogPath))
	catalogBytes, err := readAllBounded(catalogFile, maxCatalogBytes)
	if err != nil {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"catalog_read_failed", relativeCatalog, err.Error()})
		return result
	}
	result.Report.Hashes.StaticCatalogSHA256 = hashBytes(catalogBytes)
	catalogObject, diagnostics := decodeObject(catalogBytes, relativeCatalog)
	if len(diagnostics) > 0 {
		result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
		return result
	}
	catalogVersion, diagnostics := readRequiredString(catalogObject, "content_version", relativeCatalog)
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	catalogRoutesRaw, diagnostics := readRequiredArray(catalogObject, "routes", relativeCatalog)
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	catalogPlacesRaw, placeDiagnostics := readRequiredArray(catalogObject, "places", relativeCatalog)
	result.Report.Diagnostics = append(result.Report.Diagnostics, placeDiagnostics...)
	catalogLocationsRaw, locationDiagnostics := readRequiredArray(catalogObject, "locations", relativeCatalog)
	result.Report.Diagnostics = append(result.Report.Diagnostics, locationDiagnostics...)
	manifestRaw, manifestDiagnostics := readRequiredArray(catalogObject, "source_manifest", relativeCatalog)
	result.Report.Diagnostics = append(result.Report.Diagnostics, manifestDiagnostics...)
	result.Report.CatalogVersion = catalogVersion
	if len(result.Report.Diagnostics) > 0 {
		return result
	}
	manifestEntries, manifestDiagnostics := parseSourceManifest(manifestRaw)
	result.Report.Diagnostics = append(result.Report.Diagnostics, manifestDiagnostics...)
	if len(manifestDiagnostics) > 0 {
		return result
	}
	manifest := manifestEntries[0]
	result.Report.Hashes.ManifestDeclaredSHA256 = strings.ToLower(manifest.SHA256)
	if manifest.Format != "json" || manifest.DeclaredVersion == "" {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"unsupported_handoff_manifest", "source_manifest.handoff", "handoff source must have a JSON format and explicit declared version"})
		return result
	}
	authorPath, err := resolveManifestPath(repoRoot, manifest.Path)
	if err != nil {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"unsafe_handoff_path", "source_manifest.handoff.path", err.Error()})
		return result
	}
	result.Report.SourcePath = filepath.ToSlash(manifest.Path)
	sourceBytes, err := readAllBounded(authorPath, maxSourceBytes)
	if err != nil {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"author_source_read_failed", result.Report.SourcePath, err.Error()})
		return result
	}
	result.Report.Hashes.AuthorSourceSHA256 = hashBytes(sourceBytes)
	if !manifestHashMatches(manifest.SHA256, result.Report.Hashes.AuthorSourceSHA256) {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"author_source_hash_mismatch", result.Report.SourcePath, "author source bytes do not match the source_manifest SHA-256"})
	}
	sourceObject, sourceDiagnostics := decodeObject(sourceBytes, result.Report.SourcePath)
	result.Report.Diagnostics = append(result.Report.Diagnostics, sourceDiagnostics...)
	if len(sourceDiagnostics) > 0 {
		return result
	}
	authorVersion, authorVersionDiagnostics := readRequiredString(sourceObject, "author_version", result.Report.SourcePath)
	result.Report.Diagnostics = append(result.Report.Diagnostics, authorVersionDiagnostics...)
	result.Report.AuthorVersion = authorVersion
	if authorVersion != manifest.DeclaredVersion {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"author_version_mismatch", result.Report.SourcePath + ".author_version", "author_version differs from the manifested declared_version"})
	}
	authorRoutesRaw, authorRoutesDiagnostics := readRequiredArray(sourceObject, "routes", result.Report.SourcePath)
	result.Report.Diagnostics = append(result.Report.Diagnostics, authorRoutesDiagnostics...)
	authorPlacesRaw, authorPlacesDiagnostics := readRequiredArray(sourceObject, "places", result.Report.SourcePath)
	result.Report.Diagnostics = append(result.Report.Diagnostics, authorPlacesDiagnostics...)
	if len(authorVersionDiagnostics)+len(authorRoutesDiagnostics)+len(authorPlacesDiagnostics) > 0 {
		return result
	}
	authorRoutes := parseRouteArray(authorRoutesRaw, result.Report.SourcePath+".routes")
	catalogRoutes := parseRouteArray(catalogRoutesRaw, "catalog.routes")
	result.Report.Diagnostics = append(result.Report.Diagnostics, authorRoutes.Diagnostics...)
	result.Report.Diagnostics = append(result.Report.Diagnostics, catalogRoutes.Diagnostics...)
	authorDuplicates := duplicateRouteIDs(&authorRoutes, result.Report.SourcePath+".routes")
	catalogDuplicates := duplicateRouteIDs(&catalogRoutes, "catalog.routes")
	result.Report.Diagnostics = append(result.Report.Diagnostics, authorRoutes.Diagnostics[len(authorRoutes.Diagnostics)-duplicateCount(authorDuplicates, authorRoutes.Identities):]...)
	result.Report.Diagnostics = append(result.Report.Diagnostics, catalogRoutes.Diagnostics[len(catalogRoutes.Diagnostics)-duplicateCount(catalogDuplicates, catalogRoutes.Identities):]...)
	result.Report.Counts["author_route_records"] = countRawRows(authorRoutesRaw)
	result.Report.Counts["catalog_routes"] = countRawRows(catalogRoutesRaw)
	if result.Report.Counts["author_route_records"] != result.Report.Counts["catalog_routes"] {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"route_count_mismatch", "catalog.routes", "route record count differs from the manifested author source"})
	}
	result.Report.Diagnostics = append(result.Report.Diagnostics, compareRoutes(authorRoutes.Routes, catalogRoutes.Routes, authorDuplicates, catalogDuplicates)...)
	authorPlaces, diagnostics := parsePlaceArray(authorPlacesRaw, result.Report.SourcePath+".places")
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	catalogPlaces, diagnostics := parsePlaceArray(catalogPlacesRaw, "catalog.places")
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	result.Report.Diagnostics = append(result.Report.Diagnostics, comparePlaces(authorPlaces, catalogPlaces)...)
	catalogLocations, locationDiagnostics := parseLocationArray(catalogLocationsRaw, "catalog.locations")
	result.Report.Diagnostics = append(result.Report.Diagnostics, locationDiagnostics...)
	placeByID := make(map[string]PlaceIdentity, len(catalogPlaces))
	for _, place := range catalogPlaces {
		placeByID[place.ID] = place
	}
	result.Report.Counts["author_places"] = len(authorPlaces)
	result.Report.Counts["catalog_places"] = len(catalogPlaces)
	result.Report.Counts["catalog_locations"] = len(catalogLocations)
	result.Report.Counts["route_endpoints_not_in_authored_locations"] = countEndpointsOutsideLocations(authorRoutes.Routes, catalogLocations)
	result.Report.Counts["route_endpoints_not_in_places"] = countEndpointsOutsidePlaces(authorRoutes.Routes, placeByID)
	result.Report.Diagnostics = append(result.Report.Diagnostics, validateLocationsAgainstPlaces(catalogLocations, placeByID)...)
	result.Candidates, diagnostics = convertAuthorRoutes(authorRoutes.Routes, authorRoutes.RowIndexes, authorDuplicates, authorVersion, catalogVersion, result.Report.SourcePath, result.Report.Hashes.AuthorSourceSHA256, placeByID)
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	result.Report.Counts["candidate_forward_routes"] = countDirection(result.Candidates, "forward")
	result.Report.Counts["candidate_reverse_routes"] = countDirection(result.Candidates, "reverse")
	result.Report.Counts["candidate_routes"] = len(result.Candidates)
	if len(result.Candidates) == 0 {
		result.Report.Diagnostics = append(result.Report.Diagnostics, Diagnostic{"no_candidate_routes", "routes", "no route passed source, endpoint, direction, and movement validation"})
	}
	result.Report.Demonstrations, diagnostics = buildDemonstrations(result.Candidates)
	result.Report.Diagnostics = append(result.Report.Diagnostics, diagnostics...)
	result.Report.Valid = len(result.Report.Diagnostics) == 0 && len(result.Candidates) > 0
	return result
}

func duplicateCount(duplicates map[string]bool, identities []RouteIdentity) int {
	count := 0
	for _, identity := range identities {
		if duplicates[identity.ID] {
			count++
		}
	}
	return count
}

func countRawRows(raw json.RawMessage) int {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return 0
	}
	return len(rows)
}

func parseLocationArray(data json.RawMessage, diagnosticPath string) ([]PlaceIdentity, []Diagnostic) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(data, &rawRows); err != nil || rawRows == nil {
		return nil, []Diagnostic{{"locations_not_array", diagnosticPath, "must be a non-null JSON array"}}
	}
	if len(rawRows) > maxPlaceRows {
		return nil, []Diagnostic{{"location_limit_exceeded", diagnosticPath, fmt.Sprintf("location count exceeds %d", maxPlaceRows)}}
	}
	locations := []PlaceIdentity{}
	var diagnostics []Diagnostic
	seen := map[string]bool{}
	for i, raw := range rawRows {
		p := fmt.Sprintf("%s[%d]", diagnosticPath, i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			diagnostics = append(diagnostics, Diagnostic{"location_not_object", p, "location entry must be a JSON object"})
			continue
		}
		id, ok := rawString(fields, "id")
		if !ok || strings.TrimSpace(id) == "" {
			diagnostics = append(diagnostics, Diagnostic{"invalid_location_id", p + ".id", "location ID must be a non-empty JSON string"})
			continue
		}
		if seen[id] {
			diagnostics = append(diagnostics, Diagnostic{"duplicate_location_id", p + ".id", "location IDs must be unique"})
			continue
		}
		seen[id] = true
		locations = append(locations, PlaceIdentity{ID: id})
	}
	return locations, diagnostics
}

func validateLocationsAgainstPlaces(locations []PlaceIdentity, places map[string]PlaceIdentity) []Diagnostic {
	var diagnostics []Diagnostic
	locationIDs := make(map[string]bool, len(locations))
	for _, location := range locations {
		locationIDs[location.ID] = true
		place, exists := places[location.ID]
		if !exists || place.Kind != "authored_place" {
			diagnostics = append(diagnostics, Diagnostic{"location_place_mismatch", "catalog.locations." + location.ID, "location records must resolve to an authored_place"})
		}
	}
	placeIDs := make([]string, 0, len(places))
	for id := range places {
		placeIDs = append(placeIDs, id)
	}
	sort.Strings(placeIDs)
	for _, id := range placeIDs {
		if places[id].Kind == "authored_place" && !locationIDs[id] {
			diagnostics = append(diagnostics, Diagnostic{"authored_place_location_missing", "catalog.places." + id, "authored place is missing from the locations projection"})
		}
	}
	return diagnostics
}

func countEndpointsOutsideLocations(routes []AuthorRoute, locations []PlaceIdentity) int {
	known := make(map[string]bool, len(locations))
	for _, item := range locations {
		known[item.ID] = true
	}
	endpoints := map[string]bool{}
	for _, route := range routes {
		endpoints[route.From] = true
		endpoints[route.To] = true
	}
	count := 0
	for id := range endpoints {
		if !known[id] {
			count++
		}
	}
	return count
}

func countEndpointsOutsidePlaces(routes []AuthorRoute, places map[string]PlaceIdentity) int {
	endpoints := map[string]bool{}
	for _, route := range routes {
		endpoints[route.From] = true
		endpoints[route.To] = true
	}
	count := 0
	for id := range endpoints {
		if _, ok := places[id]; !ok {
			count++
		}
	}
	return count
}

func countDirection(routes []CandidateRoute, direction string) int {
	count := 0
	for _, route := range routes {
		if route.Direction == direction {
			count++
		}
	}
	return count
}

func buildDemonstrations(routes []CandidateRoute) ([]RouteDemonstration, []Diagnostic) {
	byScale := map[string]CandidateRoute{}
	scales := []string{}
	for _, candidate := range routes {
		if candidate.Direction != "forward" || len(candidate.Route.Segments) == 0 || candidate.Route.Segments[0].DurationMinutes < 2 {
			continue
		}
		if _, exists := byScale[candidate.Scale]; !exists {
			byScale[candidate.Scale] = candidate
			scales = append(scales, candidate.Scale)
		}
	}
	sort.Strings(scales)
	demos := make([]RouteDemonstration, 0, len(scales))
	diagnostics := []Diagnostic{}
	for _, scale := range scales {
		demo, err := demonstrateRoute(byScale[scale])
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{"route_demonstration_failed", byScale[scale].DirectionID, err.Error()})
			continue
		}
		demos = append(demos, demo)
	}
	if len(demos) < 2 {
		diagnostics = append(diagnostics, Diagnostic{"insufficient_demo_scales", "demonstrations", "at least two distinct route scales need positive-duration examples"})
	}
	return demos, diagnostics
}

func demonstrateRoute(candidate CandidateRoute) (RouteDemonstration, error) {
	methods := supportedMovementMethods
	snapshot, err := exploration.PrepareJourney(candidate.Route, "demo-"+candidate.DirectionID, methods)
	if err != nil {
		return RouteDemonstration{}, err
	}
	zero, err := exploration.AdvanceTo(candidate.Route, snapshot, 0, methods)
	if err != nil {
		return RouteDemonstration{}, err
	}
	zeroBytes, err := json.Marshal(zero.Snapshot)
	if err != nil {
		return RouteDemonstration{}, err
	}
	var restoredZero exploration.JourneySnapshot
	if err := json.Unmarshal(zeroBytes, &restoredZero); err != nil {
		return RouteDemonstration{}, err
	}
	zeroRoundTrip := reflect.DeepEqual(zero.Snapshot, restoredZero)
	if !zeroRoundTrip {
		return RouteDemonstration{}, fmt.Errorf("zero-target snapshot changed after JSON round trip")
	}
	total := candidate.Route.Segments[0].DurationMinutes
	if total < 2 {
		return RouteDemonstration{}, fmt.Errorf("route duration is too short for a mid-route continuation demonstration")
	}
	midpoint := total / 2
	if midpoint < 1 {
		midpoint = 1
	}
	mid, err := exploration.AdvanceTo(candidate.Route, restoredZero, midpoint, methods)
	if err != nil {
		return RouteDemonstration{}, err
	}
	continueTarget := midpoint + 1
	if continueTarget > total {
		continueTarget = total
	}
	continued, err := exploration.AdvanceTo(candidate.Route, mid.Snapshot, continueTarget, methods)
	if err != nil {
		return RouteDemonstration{}, err
	}
	retry, err := exploration.AdvanceTo(candidate.Route, continued.Snapshot, continueTarget, methods)
	if err != nil {
		return RouteDemonstration{}, err
	}
	finalBytes, err := json.Marshal(continued.Snapshot)
	if err != nil {
		return RouteDemonstration{}, err
	}
	var restoredFinal exploration.JourneySnapshot
	if err := json.Unmarshal(finalBytes, &restoredFinal); err != nil {
		return RouteDemonstration{}, err
	}
	finalRoundTrip := reflect.DeepEqual(continued.Snapshot, restoredFinal)
	if !finalRoundTrip || retry.AddedMinutes != 0 {
		return RouteDemonstration{}, fmt.Errorf("final route snapshot or same-target retry was not stable")
	}
	return RouteDemonstration{DirectionID: candidate.DirectionID, SourceRouteID: candidate.SourceRouteID, Scale: candidate.Scale, TotalMinutes: total, ZeroTargetAddedMinutes: zero.AddedMinutes, ZeroSnapshotJSONRoundTrip: zeroRoundTrip, MidTargetElapsedMinutes: midpoint, MidTargetAddedMinutes: mid.AddedMinutes, ContinueTargetElapsedMinutes: continueTarget, ContinueAddedMinutes: continued.AddedMinutes, SameTargetRetryAddedMinutes: retry.AddedMinutes, RemainingMinutes: continued.RemainingMinutes, Done: continued.Done, FinalSnapshotJSONRoundTrip: finalRoundTrip, RoundTripIsGameRecovery: false}, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func isWithinQ1Output(pathValue string, repoRoot string) bool {
	pathAbs, err := filepath.Abs(filepath.Join(repoRoot, filepath.FromSlash(pathValue)))
	if err != nil {
		return false
	}
	q1Abs, err := filepath.Abs(filepath.Join(repoRoot, "work", "luna6-exploration", "q1"))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(q1Abs, pathAbs)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

func catalogOverrideAllowed(value string, repoRoot string) bool {
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == filepath.Join("work", "luna6-world-content", "static-world-catalog.json") {
		return true
	}
	return isWithinQ1Output(clean, repoRoot)
}

func readAllBounded(file string, max int64) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("file exceeds %d bytes", max)
	}
	return data, nil
}
