package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"ta/internal/history"
	"ta/internal/localfs"
	"ta/internal/webfs"
)

var historyStoredAssetName = regexp.MustCompile(`^[a-f0-9]{64}\.(png|webp)$`)

func (s *HistoryAppService) historyAssetRoot() (string, error) {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.cfg == nil || s.app.closed {
		return "", ErrHistoryProject
	}
	return filepath.Join(s.app.cfg.DataDir(), "games", "history", "assets"), nil
}

// ScenarioAsset serves immutable artwork by hash. Imported names never select
// arbitrary paths, and links or modified content are not served.
func (s *HistoryAppService) ScenarioAsset(name string) ([]byte, string, error) {
	if !historyStoredAssetName.MatchString(name) {
		return nil, "", history.ErrNotFound
	}
	root, err := s.historyAssetRoot()
	if err != nil {
		return nil, "", err
	}
	file := filepath.Join(root, name)
	info, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return nil, "", history.ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > history.MaxScenarioAssetBytes {
		return nil, "", history.ErrInvalid
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, "", err
	}
	url, err := history.ScenarioAssetURL(data, path.Ext(name))
	if err != nil || path.Base(url) != name {
		return nil, "", history.ErrInvalid
	}
	mime := "image/png"
	if strings.HasSuffix(name, ".webp") {
		mime = "image/webp"
	}
	return data, mime, nil
}
func (s *HistoryAppService) readScenarioAsset(url string) ([]byte, error) {
	if strings.HasPrefix(url, "/api/history/assets/") {
		b, _, err := s.ScenarioAsset(path.Base(url))
		return b, err
	}
	if !strings.HasPrefix(url, "/history/") || strings.Contains(url[9:], "/") {
		return nil, history.ErrInvalid
	}
	return webfs.ReadAsset(strings.TrimPrefix(url, "/"))
}
func (s *HistoryAppService) ExportScenarioBundle(projectID, id string) ([]byte, error) {
	p, err := s.ExportScenario(projectID, id)
	if err != nil {
		return nil, err
	}
	return history.EncodeScenarioBundle(p, s.readScenarioAsset)
}

func (s *HistoryAppService) ImportScenarioPackage(ctx context.Context, projectID string, data []byte) (history.Scenario, error) {
	store, _, err := s.binding(projectID)
	if err != nil {
		return history.Scenario{}, err
	}
	root, err := s.historyAssetRoot()
	if err != nil {
		return history.Scenario{}, err
	}
	// Serialize commits and rollback only files created by this importer. A failed
	// bundle cannot remove another import's already-referenced immutable image.
	release, err := localfs.AcquireLease(ctx, root+".lock")
	if err != nil {
		return history.Scenario{}, err
	}
	defer release()
	var bundle history.ScenarioBundle
	if bytes.HasPrefix(data, []byte("PK")) {
		bundle, err = history.DecodeScenarioBundle(data)
	} else {
		bundle.Scenario, err = history.DecodeScenario(data)
		bundle.Assets = map[string][]byte{}
		if err == nil {
			for _, url := range history.ScenarioAssetURLs(bundle.Scenario) {
				if _, readErr := s.readScenarioAsset(url); readErr != nil {
					return history.Scenario{}, readErr
				}
			}
		}
	}
	if err != nil {
		return history.Scenario{}, err
	}
	existing, oldErr := store.Scenario(bundle.Scenario.ID)
	if oldErr != nil && !errors.Is(oldErr, history.ErrNotFound) {
		return history.Scenario{}, oldErr
	}
	if oldErr == nil {
		replacements := map[string]string{}
		for _, url := range history.ScenarioAssetURLs(existing) {
			if strings.HasPrefix(url, "/api/history/assets/") {
				continue
			}
			b, err := s.readScenarioAsset(url)
			if err != nil {
				return history.Scenario{}, err
			}
			canonical, err := history.ScenarioAssetURL(b, path.Ext(url))
			if err != nil {
				return history.Scenario{}, err
			}
			replacements[url] = canonical
		}
		oldJSON, _ := json.Marshal(history.ReplaceScenarioAssets(existing, replacements))
		newJSON, _ := json.Marshal(bundle.Scenario)
		// Plain JSON can still match its existing authored representation exactly.
		originalJSON, _ := json.Marshal(existing)
		if !bytes.Equal(oldJSON, newJSON) && !bytes.Equal(originalJSON, newJSON) {
			return history.Scenario{}, history.ErrConflict
		}
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return history.Scenario{}, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return history.Scenario{}, history.ErrInvalid
	}
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, file := range created {
				if err := os.Remove(file); err != nil {
					slog.Warn("[history] asset rollback failed", "path", file, "error", err)
				}
			}
		}
	}()
	for url, b := range bundle.Assets {
		file := filepath.Join(root, path.Base(url))
		if _, err := os.Lstat(file); err == nil {
			existing, _, err := s.ScenarioAsset(path.Base(url))
			if err != nil || !bytes.Equal(existing, b) {
				return history.Scenario{}, history.ErrConflict
			}
			continue
		} else if !os.IsNotExist(err) {
			return history.Scenario{}, err
		}
		temp, err := os.CreateTemp(root, ".asset-*")
		if err != nil {
			return history.Scenario{}, err
		}
		name := temp.Name()
		_, writeErr := temp.Write(b)
		syncErr := temp.Sync()
		closeErr := temp.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			os.Remove(name)
			return history.Scenario{}, fmt.Errorf("scenario asset write failed: %v %v %v", writeErr, syncErr, closeErr)
		}
		err = os.Link(name, file)
		os.Remove(name)
		if err != nil {
			return history.Scenario{}, err
		}
		created = append(created, file)
	}
	if oldErr == nil {
		committed = true
		return existing, nil
	}
	raw, err := json.Marshal(bundle.Scenario)
	if err != nil {
		return history.Scenario{}, err
	}
	p, err := store.ImportScenario(raw)
	if err != nil {
		return history.Scenario{}, err
	}
	committed = true
	slog.InfoContext(ctx, "[history] scenario package imported", "scenario_id", p.ID, "assets", len(bundle.Assets))
	return p, nil
}
