package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ta/config"
	appsettings "ta/internal/app/settings"
	"ta/internal/concurrency"
	"ta/internal/history"
	"ta/internal/localfs"
	projectdomain "ta/internal/project"
)

var (
	ErrHistoryBusy            = errors.New("history campaign has an active request")
	ErrHistoryModel           = errors.New("history model request failed")
	ErrHistoryModelBudget     = errors.New("history model output budget exhausted")
	ErrHistoryModelOutput     = errors.New("history model response is invalid")
	ErrHistoryInput           = errors.New("history input is invalid")
	ErrHistoryProject         = errors.New("project does not own the history game")
	ErrHistoryCommandConflict = errors.New("history command id is already bound to different input")
)

// HistoryAppService owns AI orchestration for the independent management game.
// The campaign journal is its only durable facts source. Adventure Story state,
// foreground Book selection and adventure-specific prompts are never consulted.
type HistoryAppService struct {
	app     *App
	mu      sync.Mutex
	flights map[string]*historyFlight
	closed  bool
}

func (a *App) HistoryService() *HistoryAppService {
	a.ensureServices()
	return a.historyApp
}

type HistoryBootstrap struct {
	ProjectID string            `json:"project_id"`
	Campaigns []history.Summary `json:"campaigns"`
	Scenarios []HistoryScenario `json:"scenarios"`
}

type HistoryScenario struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Bootstrap registers a managed content directory once without selecting it as
// a Book. Registry identity and StoreDir remain stable when the data root moves.
func (s *HistoryAppService) Bootstrap(ctx context.Context, locale string) (HistoryBootstrap, error) {
	s.app.mu.RLock()
	if s.app.cfg == nil || s.app.projectRegistry == nil || s.app.closed {
		s.app.mu.RUnlock()
		return HistoryBootstrap{}, concurrency.ErrClosed
	}
	dataDir := s.app.cfg.DataDir()
	registry := s.app.projectRegistry
	s.app.mu.RUnlock()
	release, err := localfs.AcquireLease(ctx, filepath.Join(dataDir, "runtime", "history-bootstrap.lock"))
	if err != nil {
		return HistoryBootstrap{}, err
	}
	defer release()
	contentRoot := filepath.Join(dataDir, "games", "history")
	if err := os.MkdirAll(contentRoot, 0o700); err != nil {
		return HistoryBootstrap{}, err
	}
	record, found, err := registry.FindByPath(contentRoot, true)
	if err != nil {
		return HistoryBootstrap{}, err
	}
	if !found {
		record, err = registry.Add(contentRoot, projectdomain.TypeGeneral, "History Simulation")
		if err != nil {
			return HistoryBootstrap{}, err
		}
	}
	if record.Type != projectdomain.TypeGeneral || record.Status == projectdomain.StatusArchived {
		return HistoryBootstrap{}, ErrHistoryProject
	}
	_, layout, err := registry.Resolve(record.ID, true)
	if err != nil {
		return HistoryBootstrap{}, err
	}
	campaigns, err := history.NewStore(layout.StoreRoot).List(ctx)
	if err != nil {
		return HistoryBootstrap{}, err
	}
	if campaigns == nil {
		campaigns = []history.Summary{}
	}
	packs, err := history.NewStore(layout.StoreRoot).Scenarios()
	if err != nil {
		return HistoryBootstrap{}, err
	}
	scenarios := []HistoryScenario{}
	for _, p := range packs {
		name, description := p.Name.ZH, p.Description.ZH
		if strings.HasPrefix(strings.ToLower(locale), "en") {
			name, description = p.Name.EN, p.Description.EN
		}
		scenarios = append(scenarios, HistoryScenario{ID: p.ID, Name: name, Description: description})
	}
	return HistoryBootstrap{ProjectID: record.ID, Campaigns: campaigns, Scenarios: scenarios}, nil
}

// binding verifies the dedicated managed Project even for direct API callers.
// A valid arbitrary adventure Project ID must not become a history data target.
func (s *HistoryAppService) binding(projectID string) (*history.Store, config.Config, error) {
	s.app.mu.RLock()
	registry := s.app.projectRegistry
	var dataDir string
	if s.app.cfg != nil {
		dataDir = s.app.cfg.DataDir()
	}
	closed := s.app.closed
	s.app.mu.RUnlock()
	if closed || registry == nil || dataDir == "" {
		return nil, config.Config{}, concurrency.ErrClosed
	}
	record, layout, err := registry.Resolve(projectID, true)
	if err != nil {
		return nil, config.Config{}, err
	}
	if record.Type != projectdomain.TypeGeneral || record.Location.Kind != projectdomain.LocationManaged || record.Location.Path != "games/history" {
		return nil, config.Config{}, ErrHistoryProject
	}
	runtime, err := (settingsHost{app: s.app}).SettingsRuntime(appsettings.Project(projectID))
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("resolve history project settings: %w", err)
	}
	return history.NewStore(layout.StoreRoot), runtime.Config, nil
}

func (s *HistoryAppService) List(ctx context.Context, projectID string) ([]history.Summary, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return nil, err
	}
	result, err := store.List(ctx)
	if result == nil {
		result = []history.Summary{}
	}
	return result, err
}

func (s *HistoryAppService) Read(ctx context.Context, projectID, campaignID string) (history.State, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return history.State{}, err
	}
	state, err := store.Read(ctx, campaignID)
	return history.PlayerState(state), err
}

func (s *HistoryAppService) Create(ctx context.Context, projectID string, req history.CreateRequest) (history.State, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return history.State{}, err
	}
	state, err := store.Create(ctx, req)
	return history.PlayerState(state), err
}

func (s *HistoryAppService) ImportScenario(projectID string, data []byte) (history.Scenario, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return history.Scenario{}, err
	}
	return store.ImportScenario(data)
}
func (s *HistoryAppService) ExportScenario(projectID, id string) (history.Scenario, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return history.Scenario{}, err
	}
	return store.Scenario(id)
}
