// Package projectbook owns Book-specific resources addressed by stable Project ID.
// Operations resolve the current content directory for every request and never
// activate or otherwise mutate the foreground Writing workspace.
package projectbook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	reviewapp "ta/internal/app/review"
	"ta/internal/book"
	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
	projectdomain "ta/internal/project"
	"ta/internal/workspace/documentreview"
)

var (
	ErrBookProjectRequired = errors.New("project is not a Book")
	// ErrUnknownLoreScope refuses a new entry aimed at a world this project's
	// stories do not use, so a request cannot file content into another world.
	ErrUnknownLoreScope = errors.New("lore scope does not belong to this project")
)

// Snapshot is the shared read model used by Writing and embedded project pages.
type Snapshot struct {
	ProjectID string                `json:"project_id"`
	Workspace string                `json:"workspace"`
	Tree      []*book.FileNode      `json:"tree"`
	Summary   book.WorkspaceSummary `json:"summary"`
}

// Service keeps all Book-only resource resolution behind one Project-ID seam.
// File content and assets remain in projectfiles because those also support
// General projects.
type Service struct {
	registry *projectdomain.Registry
	mu       sync.Mutex
	files    map[string]cachedBookFiles
	sequence uint64
}

func NewService(registry *projectdomain.Registry) *Service {
	return &Service{registry: registry, files: make(map[string]cachedBookFiles)}
}

func (service *Service) Snapshot(ctx context.Context, projectID string) (Snapshot, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	tree, err := runtime.files.Tree()
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	summary, err := runtime.files.Summary()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		ProjectID: runtime.record.ID,
		Workspace: runtime.layout.ContentRoot,
		Tree:      tree,
		Summary:   summary,
	}, nil
}

// LoreBook is the lore library as one project reads it: every entry this
// project's stories can see, plus the world the author has open.
//
// The library is split into two pages. WorldScope is what tells them apart: an
// entry in the open world's scope is that world's own setting, and an entry in
// any other scope belongs to a world this project is not currently editing. The
// page therefore cannot work the split out from the entry list alone — one
// world's scope looks like another's — so the scope travels with the list rather
// than being re-derived in the browser.
type LoreBook struct {
	Items []booklore.Item `json:"items"`
	// WorldScope is the scope of the world the author has open, or empty when no
	// story is open. Empty is not an error: the library is reachable before a
	// game starts.
	WorldScope string `json:"world_scope"`
	// WorldName is that world's display name, so the page can say which world it
	// is showing instead of printing a scope token.
	WorldName string `json:"world_name,omitempty"`
}

func (service *Service) LoreBook(projectID string) (LoreBook, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return LoreBook{}, err
	}
	worlds, current, name, err := service.projectWorldScopes(runtime)
	if err != nil {
		return LoreBook{}, err
	}
	items, err := booklore.NewStore(runtime.layout.ContentRoot).ListAll()
	if err != nil {
		return LoreBook{}, err
	}
	return LoreBook{
		Items:      itemsInWorldScopes(items, worlds),
		WorldScope: current,
		WorldName:  name,
	}, nil
}

func (service *Service) LoreItems(projectID string) ([]booklore.Item, error) {
	book, err := service.LoreBook(projectID)
	if err != nil {
		return nil, err
	}
	return book.Items, nil
}

// itemsInWorldScopes keeps the shared book and the world books this project's
// stories read, and drops every other scope.
//
// A pack writes one world book per content revision, so the store accumulates a
// complete set of entries each time the pack changes. Those older sets stay
// addressable on purpose — a story already running keeps reading the revision it
// started from — but the editor shows one book per world, so listing them all
// would show the same person three times under three revisions.
func itemsInWorldScopes(items []booklore.Item, worlds []string) []booklore.Item {
	used := make(map[string]bool, len(worlds))
	for _, world := range worlds {
		used[world] = true
	}
	kept := make([]booklore.Item, 0, len(items))
	for _, item := range items {
		scope := strings.TrimSpace(item.Scope)
		if scope == booklore.ScopeShared || used[scope] {
			kept = append(kept, item)
		}
	}
	return kept
}

func (service *Service) CreateLoreItem(projectID string, input booklore.ItemInput) (booklore.Item, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	scope, err := service.newLoreItemScope(runtime, input.Scope)
	if err != nil {
		return booklore.Item{}, err
	}
	input.Scope = scope
	return booklore.NewStore(runtime.layout.ContentRoot).Create(input)
}

// loreScopeSharedRequest asks for the shared book, the entries every story
// reads. It is spelled out rather than left to an empty string because an empty
// scope is what a client that says nothing sends, and saying nothing must not
// silently publish an entry to every world.
const loreScopeSharedRequest = "shared"

// newLoreItemScope decides which world a newly authored entry belongs to.
//
// An entry created while a story is open belongs to that story's world: the
// author is looking at one world's book, so an entry that landed in the shared
// scope would appear in every other world too. The shared scope stays reachable
// but has to be asked for by name, and a scope this project's stories do not
// use is refused rather than written, so a browser cannot file an entry into
// another world.
func (service *Service) newLoreItemScope(runtime runtime, requested string) (string, error) {
	worlds, current, _, err := service.projectWorldScopes(runtime)
	if err != nil {
		return "", err
	}
	return resolveNewLoreScope(worlds, current, requested)
}

// projectWorldScopes reports every world scope this project's stories read, the
// scope of the story the author has open, and that world's display name.
func (service *Service) projectWorldScopes(runtime runtime) ([]string, string, string, error) {
	index, err := interactive.NewStore(runtime.layout.ContentRoot).Index()
	if err != nil {
		return nil, "", "", err
	}
	worlds := make([]string, 0, len(index.Stories))
	current := ""
	name := ""
	for _, story := range index.Stories {
		scope := interactive.StoryPackBindingScope(story.PackBinding)
		if scope != "" {
			worlds = append(worlds, scope)
		}
		if story.ID == strings.TrimSpace(index.CurrentStoryID) {
			current = scope
			if story.PackBinding != nil {
				name = strings.TrimSpace(story.PackBinding.PackName)
			}
		}
	}
	return worlds, current, name, nil
}

// resolveNewLoreScope is the decision half of newLoreItemScope: worlds lists
// every world scope this project's stories read, current is the scope of the
// story the author has open.
func resolveNewLoreScope(worlds []string, current, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	switch requested {
	case "":
		return current, nil
	case loreScopeSharedRequest:
		return booklore.ScopeShared, nil
	}
	for _, world := range worlds {
		if world == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrUnknownLoreScope, requested)
}

func (service *Service) UpdateLoreItem(projectID, id string, input booklore.ItemInput) (booklore.Item, error) {
	store, err := service.loreStore(projectID)
	if err != nil {
		return booklore.Item{}, err
	}
	return store.Update(id, input)
}

func (service *Service) DeleteLoreItem(projectID, id string) error {
	store, err := service.loreStore(projectID)
	if err != nil {
		return err
	}
	return store.Delete(id)
}

// WithDocumentReview resolves the review ledger and target snapshots from the
// same Book runtime, so comments can never bind to a different foreground Book.
func (service *Service) WithDocumentReview(
	projectID string,
	action func(*documentreview.Service, documentreview.SnapshotResolver) error,
) (string, error) {
	if action == nil {
		return "", errors.New("document review action is nil")
	}
	runtime, err := service.resolve(projectID)
	if err != nil {
		return "", err
	}
	reviews, err := documentreview.ForWorkspaceAt(runtime.layout.ContentRoot, runtime.layout.StoreRoot)
	if err != nil {
		return "", err
	}
	resolver := reviewapp.NewTargetResolver(runtime.layout.ContentRoot, runtime.files)
	if err := action(reviews, resolver); err != nil {
		return "", err
	}
	return runtime.layout.ContentRoot, nil
}

type runtime struct {
	record projectdomain.Record
	layout projectdomain.Layout
	files  *book.Service
}

func (service *Service) loreStore(projectID string) (*booklore.Store, error) {
	runtime, err := service.resolve(projectID)
	if err != nil {
		return nil, err
	}
	return booklore.NewStore(runtime.layout.ContentRoot), nil
}

func (service *Service) resolve(projectID string) (runtime, error) {
	if service == nil || service.registry == nil {
		return runtime{}, fmt.Errorf("project registry is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return runtime{}, fmt.Errorf("project ID is required")
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return runtime{}, err
	}
	if record.Type != projectdomain.TypeBook {
		return runtime{}, fmt.Errorf("%w: project_id=%s", ErrBookProjectRequired, record.ID)
	}
	return runtime{record: record, layout: layout, files: service.bookFiles(record.ID, layout.ContentRoot)}, nil
}

const bookFilesCacheLimit = 8

type cachedBookFiles struct {
	workspace string
	files     *book.Service
	used      uint64
}

func (service *Service) bookFiles(projectID, workspace string) *book.Service {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sequence++
	if cached, exists := service.files[projectID]; exists && cached.workspace == workspace {
		cached.used = service.sequence
		service.files[projectID] = cached
		return cached.files
	}
	files := book.NewService(workspace)
	service.files[projectID] = cachedBookFiles{workspace: workspace, files: files, used: service.sequence}
	if len(service.files) <= bookFilesCacheLimit {
		return files
	}
	oldestID := ""
	oldestUse := service.sequence
	for id, candidate := range service.files {
		if id != projectID && candidate.used <= oldestUse {
			oldestID, oldestUse = id, candidate.used
		}
	}
	delete(service.files, oldestID)
	return files
}

// InvalidateSummary forwards an ephemeral filesystem invalidation to an
// already-open Book projection. Cold projects need no work: their first read
// builds from canonical files.
func (service *Service) InvalidateSummary(projectID string, paths []string, resync bool) {
	if service == nil {
		return
	}
	service.mu.Lock()
	cached, exists := service.files[strings.TrimSpace(projectID)]
	service.mu.Unlock()
	if exists {
		cached.files.InvalidateSummary(paths, resync)
	}
}
