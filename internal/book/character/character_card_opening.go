package character

import (
	"ta/internal/book"
	"ta/internal/book/lore"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type characterCardFileSnapshot struct {
	path    string
	data    []byte
	existed bool
}

func snapshotCharacterCardImportFiles(workspace string) ([]characterCardFileSnapshot, error) {
	relPaths := []string{tavernCardCoverPath, loreItemsRelPath(workspace)}
	snapshots := make([]characterCardFileSnapshot, 0, len(relPaths))
	for _, relPath := range relPaths {
		absPath := ""
		if relPath == loreItemsRelPath(workspace) {
			absPath = lore.ItemsPath(workspace)
		} else {
			var err error
			absPath, err = book.SafePath(workspace, relPath)
			if err != nil {
				return nil, err
			}
		}
		data, err := os.ReadFile(absPath)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, characterCardFileSnapshot{path: absPath})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取导入事务快照失败: %w", err)
		}
		snapshots = append(snapshots, characterCardFileSnapshot{path: absPath, data: data, existed: true})
	}
	return snapshots, nil
}

func restoreCharacterCardImportFiles(snapshots []characterCardFileSnapshot) error {
	var firstErr error
	for _, snapshot := range snapshots {
		if !snapshot.existed {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(snapshot.path), 0o755); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.data, 0o644); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) importTavernCardCover(card normalizedTavernCard, data []byte) (string, error) {
	if !card.IsPNG {
		return "", nil
	}
	absPath, err := book.SafePath(s.workspace, tavernCardCoverPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("创建封面目录失败: %w", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", fmt.Errorf("写入酒馆角色卡封面失败: %w", err)
	}
	return tavernCardCoverPath, nil
}
