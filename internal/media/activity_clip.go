package media

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func (asset Asset) isActivityClip() bool {
	return asset.Role == RoleIntimate && asset.Purpose == PurposeClip && asset.Subject == SubjectCharacter
}

// OpenActivityClip resolves a clip for a caller that has already authorized the
// current activity and participant. It checks the declared bytes on the handle
// the caller will stream. The general media route cannot open these files.
func (l *Library) OpenActivityClip(query PictureQuery, file string) (*os.File, Asset, error) {
	if query.Role != RoleIntimate || query.World == "" || !validToken(file) {
		return nil, Asset{}, fs.ErrNotExist
	}
	for _, asset := range l.ClipsFor(query) {
		if asset.File != file || !asset.isActivityClip() || asset.Bytes <= 0 || len(asset.SHA256) != sha256.Size*2 {
			continue
		}
		handle, err := os.Open(filepath.Join(l.dir, file))
		if err != nil {
			return nil, Asset{}, err
		}
		hash := sha256.New()
		n, readErr := io.Copy(hash, io.LimitReader(handle, asset.Bytes+1))
		if readErr == nil && n == asset.Bytes && hex.EncodeToString(hash.Sum(nil)) == asset.SHA256 && l.storyFileRestriction(file, asset.SHA256) != storyRestrictionDenied {
			if _, err := handle.Seek(0, io.SeekStart); err == nil {
				return handle, asset, nil
			}
		}
		_ = handle.Close()
		break
	}
	return nil, Asset{}, fs.ErrNotExist
}
