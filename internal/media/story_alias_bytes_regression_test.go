package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aliasRegressionPNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, c)
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// With the v17 implementation both empty and false SHA declarations open the
// private PNG successfully under this ordinary reviewed alias. The fixture is
// a real PNG with an independently computed SHA, not an assumed filename.
func TestGlobalMediaChecksOpenedBytesDespiteAliasMetadata(t *testing.T) {
	secret := aliasRegressionPNG(t, color.RGBA{R: 210, A: 255})
	sum := sha256.Sum256(secret)
	secretSHA := hex.EncodeToString(sum[:])
	ordinary := aliasRegressionPNG(t, color.RGBA{G: 180, A: 255})
	for _, declared := range []string{"", strings.Repeat("f", 64)} {
		for _, privateBytes := range []bool{true, false} {
			label := "missing"
			if declared != "" {
				label = "false"
			}
			if privateBytes {
				label += "/private"
			} else {
				label += "/ordinary"
			}
			t.Run(label, func(t *testing.T) {
				dir := t.TempDir()
				const alias = "ordinary-import.png"
				payload := ordinary
				if privateBytes {
					payload = secret
				}
				if err := os.WriteFile(filepath.Join(dir, alias), payload, 0600); err != nil {
					t.Fatal(err)
				}
				restriction := StoryManifest{Schema: "ta.story-media/v1", Assets: []StoryAsset{{ID: "restricted", File: secretSHA + ".png", SHA256: secretSHA, Bytes: int64(len(secret)), Reviewed: true}}}
				raw, _ := json.Marshal(restriction)
				if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				legacy := map[string]any{"schema": "ta.media/v2", "entries": []map[string]any{{"source_key": "legacy-npc", "purpose": "portrait", "file": alias, "reviewed": true, "sha256": declared, "bytes": len(payload)}}}
				raw, _ = json.Marshal(legacy)
				if err := os.WriteFile(filepath.Join(dir, "media.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				f, _, err := NewLibrary(dir).Open(alias)
				if privateBytes {
					if err == nil {
						f.Close()
						t.Fatal("private bytes escaped through reviewed alias with untrusted SHA metadata")
					}
					return
				}
				if err != nil {
					t.Fatalf("ordinary legacy media was rejected: %v", err)
				}
				defer f.Close()
				got, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, payload) {
					t.Fatal("hash scan did not rewind ordinary handle")
				}
			})
		}
	}
}
