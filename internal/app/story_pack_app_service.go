package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	appsettings "ta/internal/app/settings"
	"ta/internal/interactive"
	"ta/internal/portablepath"
	projectdomain "ta/internal/project"
	"ta/internal/revisionfile"
	"ta/internal/storypack"
)

// ErrNoDataRoot means the pack catalog was requested before a durable Text Adventure
// data directory was configured.
var ErrNoDataRoot = errors.New("Text Adventure data directory is required")

// StoryPackService exposes the game-only script pack (剧本包) catalog.
//
// A pack is a packaging format over the original game entry points, not a
// parallel one: applying a pack writes its world book into the Project and
// returns a draft the caller submits through the unchanged story-creation
// request. There is still exactly one place a story is created and one place
// turn state is committed.
type StoryPackService struct {
	app *App
}

func (a *App) StoryPacks() *StoryPackService {
	return &StoryPackService{app: a}
}

func (s *StoryPackService) dataRoot() (string, error) {
	if s.app == nil {
		return "", ErrNoDataRoot
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.cfg == nil {
		return "", ErrNoDataRoot
	}
	root := strings.TrimSpace(s.app.cfg.DataDir())
	if root == "" {
		return "", ErrNoDataRoot
	}
	return root, nil
}

// List returns the available packs, user packs first.
func (s *StoryPackService) List() ([]storypack.Summary, error) {
	return s.ListLocale("")
}

// ListLocale returns the available packs with one language's display text
// applied. An empty locale returns each pack's own text, which is what a
// non-UI caller wants.
func (s *StoryPackService) ListLocale(locale string) ([]storypack.Summary, error) {
	root, err := s.dataRoot()
	if err != nil {
		return nil, err
	}
	return storypack.NewLibrary(root).ListLocale(locale)
}

// Get returns one pack in full.
func (s *StoryPackService) Get(id string) (storypack.Pack, error) {
	root, err := s.dataRoot()
	if err != nil {
		return storypack.Pack{}, err
	}
	return storypack.NewLibrary(root).Get(id)
}

// GetContent returns one pack's authored content without requiring the director
// modules it binds to be installed. Reads that only need the catalog — a
// person's stable author key, for instance — use this so an uninstalled module
// cannot take the whole world's faces away.
func (s *StoryPackService) GetContent(id string) (storypack.Pack, error) {
	root, err := s.dataRoot()
	if err != nil {
		return storypack.Pack{}, err
	}
	return storypack.NewLibrary(root).GetContent(id)
}

// SaveSettings stores the player's new-game defaults for one world.
//
// It writes a separate overlay beside the pack, never the pack file: a builtin
// is compiled into the binary and a user pack may be someone else's world, and
// neither may be rewritten by a preference. The expected revision is the one the
// page loaded, so a second page saving first is reported instead of overwritten.
func (s *StoryPackService) SaveSettings(ctx context.Context, packID string, settings storypack.PackSettings, expectedRevision string) (storypack.SettingsSaveResult, error) {
	root, err := s.dataRoot()
	if err != nil {
		return storypack.SettingsSaveResult{}, err
	}
	result, err := storypack.NewLibrary(root).SaveSettings(ctx, packID, settings, expectedRevision)
	if err != nil {
		if storypack.IsSettingsRevisionConflict(err) {
			slog.WarnContext(ctx, "[story-pack] settings save conflicted with another edit", "pack_id", packID)
			return storypack.SettingsSaveResult{}, err
		}
		slog.ErrorContext(ctx, "[story-pack] saving world defaults failed", "pack_id", packID, "err", err)
		return storypack.SettingsSaveResult{}, err
	}
	slog.InfoContext(ctx, "[story-pack] saved world defaults", "pack_id", packID, "revision", result.Revision)
	return result, nil
}

// SettingsRevision is the overlay revision SaveSettings expects as its base.
// A world with no saved defaults reports the missing revision, which is what
// lets a first save be a compare-and-swap against "nothing written yet" rather
// than an unconditional overwrite of someone else's file.
func (s *StoryPackService) SettingsRevision(packID string) (string, bool) {
	root, err := s.dataRoot()
	if err != nil {
		return revisionfile.MissingRevision, false
	}
	return storypack.NewLibrary(root).SettingsRevision(packID)
}

// WorldBook reads one world's authored entries.
//
// It goes through GetContent so an uninstalled director module cannot take the
// whole book away: a collection only needs the entries, and a world the player
// is playing should stay readable even when a module it binds to is missing.
func (s *StoryPackService) WorldBook(packID string) (storypack.WorldBookSnapshot, error) {
	root, err := s.dataRoot()
	if err != nil {
		return storypack.WorldBookSnapshot{}, err
	}
	return storypack.NewLibrary(root).WorldBook(packID)
}

// CollectWorldEntry files one story entity into an existing world's book.
//
// It is the assistant's write for 收录到世界包, and it goes through the same pack
// file the world page edits rather than a second store: the world book a new
// game reads and the book this writes are one record.
//
// A builtin world is refused, because writing it would shadow a delivered asset
// for every story started from it. The error says so and the assistant relays
// it; saving the world as the player's own copy is the answer, not an overwrite.
func (s *StoryPackService) CollectWorldEntry(ctx context.Context, packID string, draft storypack.WorldEntryDraft) (storypack.WorldEntryCollectResult, error) {
	root, err := s.dataRoot()
	if err != nil {
		return storypack.WorldEntryCollectResult{}, err
	}
	result, err := storypack.NewLibrary(root).CollectWorldEntry(ctx, packID, draft)
	if err != nil {
		if storypack.IsBuiltinPackReadOnly(err) {
			slog.WarnContext(ctx, "[story-pack] refused to write a builtin world", "pack_id", packID)
			return storypack.WorldEntryCollectResult{}, err
		}
		slog.ErrorContext(ctx, "[story-pack] collecting a story entity failed", "pack_id", packID, "err", err)
		return storypack.WorldEntryCollectResult{}, err
	}
	slog.InfoContext(ctx, "[story-pack] collected a story entity",
		"pack_id", packID, "entry_id", result.EntryID, "created", result.Created)
	return result, nil
}

// StoryPackStart is the result of starting a world from a pack: a Project now
// exists, its world book is written, and it holds exactly one story bound to
// this pack revision.
type StoryPackStart struct {
	ProjectID string                   `json:"project_id"`
	Workspace string                   `json:"workspace"`
	PackID    string                   `json:"pack_id"`
	PackName  string                   `json:"pack_name"`
	Story     interactive.StorySummary `json:"story"`
}

// PrepareStart creates a new Project for a pack and the story it starts.
//
// It is one operation because the two halves cannot be separated: the story
// must be created through CreateStory (the pack binding is only accepted there),
// and CreateStory resolves the protagonist and modules against the Project that
// is already open. Creating the Book first and repointing its auto-created story
// afterwards is impossible — the update request carries no binding — so this
// method creates the Book, writes the pack's world book into it, creates the
// bound story, and removes the empty placeholder story CreateBook seeded.
//
// The empty placeholder is removed only after the bound story is durable, and
// only while it is still the unbound, turn-less story CreateBook made; a story
// the user started meanwhile is never touched.
func (s *StoryPackService) PrepareStart(ctx context.Context, packID string) (StoryPackStart, error) {
	return s.PrepareStartTemplate(ctx, packID, "")
}

// PrepareStartTemplate starts a world as one of the pack's ready-made
// characters. An empty templateID starts the pack's own protagonist, so the
// world page's plain start and a templated start share this one path.
func (s *StoryPackService) PrepareStartTemplate(ctx context.Context, packID, templateID string) (StoryPackStart, error) {
	return s.PrepareStartIdentity(ctx, packID, templateID, nil)
}

// PrepareStartIdentity starts a world as a character the player created.
//
// It shares the whole creation path with a templated start — one Book, one
// applied world book, one CreateStory — and differs only in what it adds to the
// request: the confirmed prose, and the opening the player confirmed. The
// character is resolved again here from the pack, so the request cannot hand the
// story a state or a number: the client sends what it chose to the draft
// endpoint and then asks to start, and everything a rule reads is derived on the
// server from the same pack file.
func (s *StoryPackService) PrepareStartIdentity(ctx context.Context, packID, templateID string, identity *storypack.IdentityChoice) (StoryPackStart, error) {
	pack, err := s.Get(packID)
	if err != nil {
		return StoryPackStart{}, err
	}
	req := pack.StoryRequest()
	if strings.TrimSpace(templateID) != "" {
		// The template's state is deliberately not carried in the request here.
		// It is resolved again on the server inside CreateInteractiveStoryContext
		// from the same pack file, so the browser never sees or supplies it.
		req, _, err = pack.StoryRequestForTemplate(templateID)
		if err != nil {
			return StoryPackStart{}, err
		}
		req.PackTemplate = &interactive.StoryProtagonistTemplateRef{
			PackID:     pack.ID,
			TemplateID: strings.TrimSpace(templateID),
		}
	}
	if identity != nil {
		req, templateID, err = withConfirmedIdentity(pack, req, *identity)
		if err != nil {
			return StoryPackStart{}, err
		}
	}
	denovaDir, err := s.denovaDir()
	if err != nil {
		return StoryPackStart{}, err
	}
	title, err := uniqueBookTitle(s.app, denovaDir, pack.Name)
	if err != nil {
		return StoryPackStart{}, err
	}
	created, err := s.app.CreateBook(ctx, denovaDir, title, "", pack.Description)
	if err != nil {
		return StoryPackStart{}, err
	}
	placeholderIDs := s.defaultStoryIDs(created.Workspace)
	if _, err := s.Apply(ctx, created.ProjectID, pack.ID); err != nil {
		return StoryPackStart{}, err
	}
	story, err := s.app.CreateInteractiveStoryContext(ctx, req)
	if err != nil {
		return StoryPackStart{}, err
	}
	s.removePlaceholderStories(placeholderIDs, story.ID)
	slog.InfoContext(ctx, "[story-pack] started a world from a pack",
		"pack_id", pack.ID, "project_id", created.ProjectID, "story_id", story.ID,
		"template_id", strings.TrimSpace(templateID))
	return StoryPackStart{
		ProjectID: created.ProjectID,
		Workspace: created.Workspace,
		PackID:    pack.ID,
		PackName:  pack.Name,
		Story:     story,
	}, nil
}

// denovaDir is where CreateBook places the new Project: the data directory the
// settings service already resolved, which is what the manual creation path
// uses.
func (s *StoryPackService) denovaDir() (string, error) {
	layered, err := s.app.SettingsService().Snapshot(appsettings.Global())
	if err != nil {
		return "", fmt.Errorf("读取数据目录失败: %w", err)
	}
	if strings.TrimSpace(layered.Paths.DenovaDir) == "" {
		return "", ErrNoDataRoot
	}
	return layered.Paths.DenovaDir, nil
}

// defaultStoryIDs lists the stories a freshly created Book already holds, so the
// pack start can recognise the placeholder it may remove.
func (s *StoryPackService) defaultStoryIDs(workspace string) []string {
	store := interactive.NewStore(workspace)
	index, err := store.Index()
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(index.Stories))
	for _, story := range index.Stories {
		ids = append(ids, story.ID)
	}
	return ids
}

// removePlaceholderStories deletes the empty, unbound stories CreateBook seeds.
// They exist only so a hand-created Book opens on a setup screen; once a pack
// story exists the placeholder would show up as a second, empty story in the
// world's saved list.
func (s *StoryPackService) removePlaceholderStories(storyIDs []string, keep string) {
	store := s.app.interactiveService().store()
	if store == nil {
		return
	}
	index, err := store.Index()
	if err != nil {
		return
	}
	byID := make(map[string]interactive.StorySummary, len(index.Stories))
	for _, story := range index.Stories {
		byID[story.ID] = story
	}
	for _, id := range storyIDs {
		if id == keep {
			continue
		}
		story, ok := byID[id]
		if !ok || story.PackBinding != nil || story.TurnCount > 0 || story.Events > 0 {
			continue
		}
		if err := s.app.DeleteInteractiveStory(id); err != nil {
			slog.ErrorContext(context.Background(), "[story-pack] remove placeholder story failed",
				"story_id", id, "err", err)
		}
	}
}

// uniqueBookTitle keeps repeated starts of one pack from colliding on a
// directory name, without renaming the world the reader picked.
func uniqueBookTitle(app *App, denovaDir, base string) (string, error) {
	parent, err := projectdomain.BookCreationParent(denovaDir, denovaDir)
	if err != nil {
		return "", err
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = "新世界"
	}
	title := base
	for suffix := 2; ; suffix++ {
		name, err := portablepath.NormalizeComponent(title)
		if err != nil {
			return "", fmt.Errorf("世界名称不能作为跨平台目录名: %w", err)
		}
		if _, statErr := os.Stat(filepath.Join(parent, name)); os.IsNotExist(statErr) {
			return title, nil
		}
		title = fmt.Sprintf("%s %d", base, suffix)
		if suffix > 200 {
			return "", fmt.Errorf("无法为世界找到可用的目录名: %s", base)
		}
	}
}

// StoryPackDraft is what a pack contributes to story creation: the world book
// has already been written, and the draft is submitted through the ordinary
// story-creation request.
type StoryPackDraft struct {
	PackID      string `json:"pack_id"`
	PackName    string `json:"pack_name"`
	WorldAdded  int    `json:"world_added"`
	WorldExists int    `json:"world_exists"`
	Request     any    `json:"request"`
}

// Apply writes a pack's world book into the Project and returns the story
// request the caller should submit.
//
// The world book is written first so the opening turn can already read the
// setting. Entries whose names already exist are left untouched: a pack ships
// an initial world, it does not own the user's setting book afterwards, so
// applying the same pack twice is safe and never overwrites later edits.
func (s *StoryPackService) Apply(ctx context.Context, projectID, packID string) (StoryPackDraft, error) {
	pack, err := s.Get(packID)
	if err != nil {
		return StoryPackDraft{}, err
	}
	inputs := pack.WorldInputs()
	created, err := s.app.Lore().CreateItems(ctx, projectID, inputs)
	if err != nil {
		return StoryPackDraft{}, fmt.Errorf("写入剧本包世界书失败: %w", err)
	}
	slog.InfoContext(ctx, "[story-pack] applied pack",
		"pack_id", pack.ID, "project_id", projectID,
		"world_added", len(created), "world_total", len(inputs))
	return StoryPackDraft{
		PackID:      pack.ID,
		PackName:    pack.Name,
		WorldAdded:  len(created),
		WorldExists: len(inputs) - len(created),
		Request:     pack.StoryRequest(),
	}, nil
}
