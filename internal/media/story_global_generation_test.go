package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalMediaRemovingManifestAssetDoesNotReleaseOldBytes(t *testing.T) {
	for _, declared := range []string{"", strings.Repeat("f", 64), "actual"} {
		for _, prime := range []bool{false, true} {
			for _, lateInstall := range []bool{false, true} {
				t.Run(fmt.Sprintf("sha=%q/prime=%v/late=%v", declared, prime, lateInstall), func(t *testing.T) {
					dir := t.TempDir()
					payload := aliasRegressionPNG(t, color.RGBA{R: 210, A: 255})
					sum := sha256.Sum256(payload)
					sha := hex.EncodeToString(sum[:])
					metadataSHA := declared
					if declared == "actual" {
						metadataSHA = sha
					}
					const alias = "reviewed-alias.png"
					if err := os.WriteFile(filepath.Join(dir, alias), payload, 0600); err != nil {
						t.Fatal(err)
					}
					write := func(file string, value any) {
						t.Helper()
						raw, err := json.Marshal(value)
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, file), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					write("media.json", map[string]any{"schema": "ta.media/v2", "entries": []map[string]any{{"source_key": "legacy", "purpose": "portrait", "reviewed": true, "file": alias, "sha256": metadataSHA, "bytes": len(payload)}}})
					manifest := StoryManifest{Schema: "ta.story-media/v1", Assets: []StoryAsset{{ID: "private", File: sha + ".png", SHA256: sha, Bytes: int64(len(payload)), Reviewed: true}}}
					if !lateInstall {
						write("story-assets.json", manifest)
					}
					library := NewLibrary(dir)
					if lateInstall {
						f, _, err := library.Open(alias)
						if err != nil {
							t.Fatalf("never-installed ordinary media refused: %v", err)
						}
						_, err = io.ReadAll(f)
						f.Close()
						if err != nil {
							t.Fatal(err)
						}
						write("story-assets.json", manifest)
					}
					if prime || lateInstall {
						if f, _, err := library.Open(alias); err == nil {
							f.Close()
							t.Fatal("private alias opened before revocation")
						}
					}
					// Keep a valid installed manifest, but revoke its only asset.
					manifest.Assets = nil
					write("story-assets.json", manifest)
					// A normal library reload must not silently approve a new restriction generation.
					library.Reload()
					if f, _, err := library.Open(alias); err == nil {
						f.Close()
						t.Fatal("removing a restricted asset exposed its old bytes via a reviewed alias")
					}
				})
			}
		}
	}
}
