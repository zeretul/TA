package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DataDirName is the current workspace-private data directory.
	DataDirName = ".ta-game"
	// LegacyDataDirName is the previous workspace-private data directory,
	// kept readable so an installation that predates the Text Adventure
	// rename keeps resolving its existing state.
	LegacyDataDirName = ".denova"
	// OldestDataDirName is the earliest workspace-private data directory.
	OldestDataDirName = ".nova"
)

// DataDirNames lists every accepted workspace-private directory name,
// newest first. New state is only ever written under DataDirName.
var DataDirNames = []string{DataDirName, LegacyDataDirName, OldestDataDirName}

// DirName returns the active workspace-private directory name.
// A fresh workspace uses .ta-game. When several historical names exist, the
// newest one holding real state wins so an upgraded workspace keeps reading
// the data it already has instead of silently starting over.
func DirName(workspace string) string {
	return dirNameFor(workspace)
}

func dirNameFor(workspace string, elem ...string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return DataDirName
	}
	existing := make([]string, 0, len(DataDirNames))
	for _, name := range DataDirNames {
		if isDir(filepath.Join(workspace, name)) {
			existing = append(existing, name)
		}
	}
	if len(existing) == 0 {
		return DataDirName
	}
	if len(existing) == 1 {
		return existing[0]
	}
	// Several candidates exist. Prefer the newest one that already holds the
	// specific target being read, so a rename never orphans existing state.
	if len(elem) > 0 {
		for _, name := range existing {
			target := filepath.Join(append([]string{filepath.Join(workspace, name)}, elem...)...)
			if hasWorkspaceData(target, false) {
				return name
			}
		}
	}
	for _, name := range existing {
		if hasWorkspaceData(filepath.Join(workspace, name), true) {
			return name
		}
	}
	return existing[0]
}

// Dir returns the absolute or workspace-relative active data directory path.
func Dir(workspace string) string {
	return filepath.Join(workspace, DirName(workspace))
}

// Path joins elem under the active workspace-private data directory.
func Path(workspace string, elem ...string) string {
	parts := append([]string{filepath.Join(workspace, dirNameFor(workspace, elem...))}, elem...)
	return filepath.Join(parts...)
}

// Rel joins elem under the active workspace-private data directory name.
func Rel(workspace string, elem ...string) string {
	parts := append([]string{dirNameFor(workspace, elem...)}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

// CurrentRel joins elem under the current Text Adventure data directory name.
func CurrentRel(elem ...string) string {
	parts := append([]string{DataDirName}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

// LegacyRel joins elem under the legacy Nova data directory name.
func LegacyRel(elem ...string) string {
	parts := append([]string{LegacyDataDirName}, elem...)
	return filepath.ToSlash(filepath.Join(parts...))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// DataDirRels returns elem under every accepted data directory name, so
// callers that must recognise historical layouts keep working after the
// rename without hard-coding each generation.
func DataDirRels(elem ...string) []string {
	rels := make([]string, 0, len(DataDirNames))
	for _, name := range DataDirNames {
		parts := append([]string{name}, elem...)
		rels = append(rels, filepath.ToSlash(filepath.Join(parts...)))
	}
	return rels
}

func hasWorkspaceData(path string, ignoreEphemeral bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return hasWorkspaceDataWithInfo(path, info, ignoreEphemeral)
}

func hasWorkspaceDataWithInfo(path string, info os.FileInfo, ignoreEphemeral bool) bool {
	if !info.IsDir() {
		return fileHasWorkspaceData(path, info)
	}
	found := false
	_ = filepath.WalkDir(path, func(child string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if child == path {
			return nil
		}
		if ignoreEphemeral && entry.IsDir() && isEphemeralRoot(path, child) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if fileHasWorkspaceData(child, info) {
			found = true
		}
		return nil
	})
	return found
}

func isEphemeralRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	switch filepath.ToSlash(rel) {
	case "runs", "checkpoints":
		return true
	default:
		return false
	}
}

func fileHasWorkspaceData(path string, info os.FileInfo) bool {
	if info.Size() == 0 || filepath.Base(path) == ".DS_Store" {
		return false
	}
	return !isEmptyLoreItemsFile(path, info.Size())
}

func isEmptyLoreItemsFile(path string, size int64) bool {
	if filepath.Base(path) != "items.json" || filepath.Base(filepath.Dir(path)) != "lore" {
		return false
	}
	if size > 256*1024 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(data)) == "" {
		return true
	}
	var collection struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(data, &collection); err == nil && collection.Items != nil {
		rawItems := strings.TrimSpace(string(collection.Items))
		if rawItems == "" || rawItems == "null" {
			return true
		}
		var items []json.RawMessage
		if err := json.Unmarshal(collection.Items, &items); err == nil {
			return len(items) == 0
		}
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil {
		return len(items) == 0
	}
	return false
}
