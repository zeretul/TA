package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"ta/internal/catalogart"
	"testing"
)

// This is the actual application artwork handler, distinct from the browser's
// controlled fixture server. Every currently imported western variant must load.
func TestFantasyEveryImportedImageReachesActualArtworkRoute(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "catalogart", "assets", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []catalogart.Entry `json:"entries"`
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	server := NewServer(newTestApplication(t), "0")
	library := catalogart.Default()
	entries, variants := 0, 0
	for _, entry := range manifest.Entries {
		if entry.World != "sui-guan-realms" {
			continue
		}
		entries++
		if library.ReviewState(entry.World, entry.Kind, entry.ID, entry.Version) != "reviewed" {
			t.Fatal("unreviewed or skipped imported image", entry.ID)
		}
		for role, variant := range entry.Variants {
			variants++
			if library.AssetID(entry.World, entry.Kind, entry.ID, entry.Version, role) != variant.SHA256 {
				t.Fatal("identity or variant did not load", entry.ID, role)
			}
			file := library.CanonicalFile(variant.SHA256 + ".png")
			response := ut.PerformRequest(server.engine.Engine, http.MethodGet, catalogart.PathPrefix+file, nil)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != library.ContentType(file) {
				t.Fatal("Artwork handler refused registered image", entry.ID, response.Code)
			}
			digest := sha256.Sum256(response.Body.Bytes())
			if hex.EncodeToString(digest[:]) != variant.SHA256 {
				t.Fatal("handler changed bytes", entry.ID)
			}
			config, format, e := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
			wantFormat := variant.Format
			if wantFormat == "" {
				wantFormat = "png"
			}
			if e != nil || config.Width != variant.Width || config.Height != variant.Height || format != wantFormat {
				t.Fatal("transport dimensions do not match actual image", entry.ID, e)
			}
		}
	}
	if entries == 0 || variants == 0 {
		t.Fatal("no western image was actually tested")
	}
	t.Logf("real handler verified %d imported identities and %d artwork variants; missing source-art coverage is reported separately", entries, variants)
}
