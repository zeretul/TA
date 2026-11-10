package update

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) verifyChecksum(ctx context.Context, assetName, archivePath string) error {
	release, err := s.latestRelease(ctx)
	if err != nil {
		return err
	}
	checksumAsset := selectChecksumAsset(release.Assets)
	if checksumAsset == nil {
		return fmt.Errorf("release is missing checksums.txt; refusing to install asset %q", assetName)
	}
	temp, err := os.CreateTemp("", "TA-checksums-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Close(); err != nil {
		return err
	}
	if err := s.downloadAsset(ctx, githubAssetDownloadURL(*checksumAsset), temp.Name(), 0, nil); err != nil {
		return err
	}
	expected, err := checksumForAsset(temp.Name(), assetName)
	if err != nil {
		return err
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("update package checksum mismatch: expected=%s actual=%s", expected, actual)
	}
	return nil
}

func checksumForAsset(path, assetName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return checksumForAssetData(data, assetName)
}

func checksumForAssetData(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt is missing %s", assetName)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// stagedPackageDigest detects changes after archive verification and before apply.
func stagedPackageDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("staged package contains non-regular file")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := []byte(filepath.ToSlash(rel))
		if err := binary.Write(h, binary.BigEndian, uint64(len(name))); err != nil {
			return err
		}
		if _, err := h.Write(name); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := binary.Write(h, binary.BigEndian, uint64(info.Size())); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
