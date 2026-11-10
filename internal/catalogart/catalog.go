// Package catalogart resolves shipped per-entry illustrations by world and stable
// author identity. It never enumerates a world's people or exposes author prose.
package catalogart

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

const PathPrefix = "/api/catalog-art/"

//go:embed assets/manifest.json assets/images/*
var assets embed.FS

// Variant is an immutable, content-addressed image derivative of one original.
// LegacySHA256 only names a previous PNG URL that redirects to the new bytes.
type Variant struct {
	SHA256       string `json:"sha256"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Format       string `json:"format,omitempty"`
	LegacySHA256 string `json:"legacy_sha256,omitempty"`
}

// Entry binds one identity, never a name, to reviewed or explicitly candidate
// artwork. Review state is authoring evidence, not a player-facing API field.
type Entry struct {
	World          string             `json:"world"`
	Kind           string             `json:"kind"`
	ID             string             `json:"id"`
	Version        int                `json:"version"`
	Category       string             `json:"category,omitempty"`
	ReviewState    string             `json:"review_state"`
	SourceSHA256   string             `json:"source_sha256"`
	OriginalSHA256 string             `json:"original_sha256"`
	Variants       map[string]Variant `json:"variants"`
}

type retainedImage struct {
	Variant
	Definition bool `json:"definition,omitempty"`
}

type manifest struct {
	Schema         string            `json:"schema"`
	Entries        []Entry           `json:"entries"`
	RetainedImages []retainedImage   `json:"retained_images,omitempty"`
	Aliases        map[string]string `json:"aliases,omitempty"`
}

type Library struct {
	entries     map[string]Entry
	files       map[string]encodedAsset
	source      fs.FS
	definitions map[string]bool
	aliases     map[string]string
}

var defaultOnce sync.Once
var defaultLibrary *Library

// Default returns the immutable shipped library. Bad rows are skipped so a
// missing or corrupt optional picture cannot prevent the game from starting.
func Default() *Library {
	defaultOnce.Do(func() {
		var err error
		defaultLibrary, err = load(assets)
		if err != nil {
			slog.Warn("[catalog-art] optional artwork omitted", "error", err)
		}
	})
	return defaultLibrary
}

func identity(world, kind, id string, version int) string {
	return fmt.Sprintf("%s/%s/%s/%d", world, kind, id, version)
}
func safeID(s string) bool {
	if s == "" || len(s) > 128 || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}
func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func load(source fs.FS) (*Library, error) {
	l := &Library{entries: map[string]Entry{}, files: map[string]encodedAsset{}, source: source, definitions: map[string]bool{}, aliases: map[string]string{}}
	raw, err := fs.ReadFile(source, "assets/manifest.json")
	if err != nil {
		return l, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return l, err
	}
	if m.Schema != "catalog-art.v1" {
		return l, fmt.Errorf("unsupported artwork manifest schema")
	}
	var rejected int
	seen := map[string]bool{}
	for _, e := range m.Entries {
		key := identity(e.World, e.Kind, e.ID, e.Version)
		if seen[key] {
			return &Library{entries: map[string]Entry{}, files: map[string]encodedAsset{}, definitions: map[string]bool{}, aliases: map[string]string{}}, fmt.Errorf("duplicate artwork identity %s", key)
		}
		seen[key] = true
		if !safeID(e.World) || !safeID(e.ID) || e.Version < 1 || (e.Kind != "npc" && e.Kind != "definition") || (e.ReviewState != "candidate" && e.ReviewState != "reviewed") || !validHash(e.SourceSHA256) || !validHash(e.OriginalSHA256) {
			rejected++
			continue
		}
		checked := map[string]Variant{}
		for role, variant := range e.Variants {
			if !(e.Kind == "npc" && (role == "avatar" || role == "display") || e.Kind == "definition" && role == "illustration") || !validHash(variant.SHA256) || variant.Width < 1 || variant.Height < 1 || variant.Width > 2048 || variant.Height > 2048 {
				rejected++
				continue
			}
			encoded, err := validateEncodedVariant(source, variant)
			if err != nil {
				rejected++
				continue
			}
			if previous, exists := l.files[variant.SHA256]; exists && previous != encoded {
				return nil, fmt.Errorf("conflicting artwork format for %s", variant.SHA256)
			}
			if variant.LegacySHA256 != "" {
				if previous := l.aliases[variant.LegacySHA256]; previous != "" && previous != variant.SHA256 {
					return nil, fmt.Errorf("conflicting legacy artwork identity")
				}
				l.aliases[variant.LegacySHA256] = variant.SHA256
			}
			checked[role] = variant
			l.files[variant.SHA256] = encoded
			if e.Kind == "definition" {
				l.definitions[variant.SHA256] = true
			}
		}
		if len(checked) == 0 {
			continue
		}
		e.Variants = checked
		l.entries[key] = e
	}
	// Retained images preserve frozen story references after authored art changes.
	// They do not create new world entries or reveal author metadata through APIs.
	for _, retired := range m.RetainedImages {
		v := retired.Variant
		if !validHash(v.SHA256) || v.Width < 1 || v.Height < 1 || v.Width > 2048 || v.Height > 2048 || v.LegacySHA256 != "" {
			return nil, fmt.Errorf("invalid retained artwork identity")
		}
		encoded, err := validateEncodedVariant(source, v)
		if err != nil {
			return nil, fmt.Errorf("invalid retained artwork: %w", err)
		}
		if previous, exists := l.files[v.SHA256]; exists && previous != encoded {
			return nil, fmt.Errorf("conflicting retained artwork format")
		}
		l.files[v.SHA256] = encoded
		if retired.Definition {
			l.definitions[v.SHA256] = true
		}
	}
	for alias, target := range m.Aliases {
		if !validHash(alias) || !validHash(target) || alias == target {
			return nil, fmt.Errorf("invalid historical artwork alias")
		}
		if _, exists := l.files[target]; !exists {
			return nil, fmt.Errorf("historical artwork target missing")
		}
		if previous := l.aliases[alias]; previous != "" && previous != target {
			return nil, fmt.Errorf("conflicting historical artwork alias")
		}
		l.aliases[alias] = target
	}
	for alias := range l.aliases {
		if _, exists := l.files[alias]; exists {
			return nil, fmt.Errorf("legacy artwork identity shadows a current image")
		}
	}
	if rejected > 0 {
		return l, fmt.Errorf("rejected %d invalid artwork rows or variants", rejected)
	}
	return l, nil
}

// AssetID resolves only the exact world, identity, version and role. Callers
// must first establish that the player can see the person or definition.
func (l *Library) AssetID(world, kind, id string, version int, role string) string {
	if l == nil {
		return ""
	}
	return l.entries[identity(world, kind, id, version)].Variants[role].SHA256
}
func (l *Library) URL(world, kind, id string, version int, role string) string {
	if id := l.AssetID(world, kind, id, version, role); id != "" {
		return PathPrefix + id + "." + l.files[id].format
	}
	return ""
}

// IsDefinitionAsset rejects unknown files and NPC portraits at the definition
// metadata boundary. Raw paths and URLs are never valid catalog identities.
func (l *Library) IsDefinitionAsset(id string) bool {
	if l == nil || !validHash(id) {
		return false
	}
	if current := l.aliases[id]; current != "" {
		id = current
	}
	return l.definitions[id]
}

// Read returns the exact bytes named by a canonical registered content hash.
// Legacy URLs are resolved by CanonicalFile and must redirect, not serve bytes.
func (l *Library) Read(file string) ([]byte, error) {
	canonical := l.CanonicalFile(file)
	if canonical == "" || canonical != file {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(l.source, "assets/images/"+canonical)
}

// ReviewState is used by offline coverage reports, not a player catalog route.
func (l *Library) ReviewState(world, kind, id string, version int) string {
	if l == nil {
		return ""
	}
	return l.entries[identity(world, kind, id, version)].ReviewState
}

// EntriesForAudit is an offline build diagnostic. Do not expose it through an
// HTTP endpoint or use it as a source of discovered characters/definitions.
func (l *Library) EntriesForAudit() []Entry {
	if l == nil {
		return nil
	}
	out := make([]Entry, 0, len(l.entries))
	for _, e := range l.entries {
		copy := e
		copy.Variants = make(map[string]Variant, len(e.Variants))
		for role, v := range e.Variants {
			copy.Variants[role] = v
		}
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return identity(out[i].World, out[i].Kind, out[i].ID, out[i].Version) < identity(out[j].World, out[j].Kind, out[j].ID, out[j].Version)
	})
	return out
}
