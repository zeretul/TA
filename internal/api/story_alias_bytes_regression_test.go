package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"ta/config"
	runtimeapp "ta/internal/app"
	"ta/internal/media"
)

func aliasHTTPFixture(t *testing.T, declared string, privateBytes bool, restrictions ...bool) (*Server, string, []byte) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "media")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	pngBytes := func(c color.RGBA) []byte {
		im := image.NewRGBA(image.Rect(0, 0, 2, 2))
		im.Set(0, 0, c)
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	secret := pngBytes(color.RGBA{R: 210, A: 255})
	sum := sha256.Sum256(secret)
	sha := hex.EncodeToString(sum[:])
	payload := pngBytes(color.RGBA{G: 180, A: 255})
	if privateBytes {
		payload = secret
	}
	const alias = "legacy-person.png"
	if err := os.WriteFile(filepath.Join(dir, alias), payload, 0600); err != nil {
		t.Fatal(err)
	}
	restricted := media.StoryManifest{Schema: "ta.story-media/v1", Assets: []media.StoryAsset{{ID: "restricted-art", File: sha + ".png", SHA256: sha, Bytes: int64(len(secret)), Reviewed: true}}}
	raw, _ := json.Marshal(restricted)
	if len(restrictions) == 0 || restrictions[0] {
		if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	legacy := map[string]any{"schema": "ta.media/v2", "entries": []map[string]any{{"id": "legacy", "source_key": "legacy-npc", "purpose": "portrait", "file": alias, "sha256": declared, "bytes": len(payload), "reviewed": true}}}
	raw, _ = json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "media.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(t.Context(), &config.Config{OpenAIModel: "test", NovaDir: root, Workspace: root, ResumeLastWorkspace: false, OpenAIBaseURL: "http://127.0.0.1:1/v1", MediaDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return NewServer(application, "0"), "/api/media/" + alias, payload
}

func TestGlobalMediaAliasCannotBypassStoryGateGETHEADRange(t *testing.T) {
	for _, declared := range []string{"", strings.Repeat("f", 64)} {
		label := "missing-sha"
		if declared != "" {
			label = "false-sha"
		}
		t.Run(label, func(t *testing.T) {
			server, target, payload := aliasHTTPFixture(t, declared, true)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				for _, ranged := range []bool{false, true} {
					var headers []ut.Header
					if ranged {
						headers = append(headers, ut.Header{Key: "Range", Value: "bytes=0-7"})
					}
					result := ut.PerformRequest(server.engine.Engine, method, target, nil, headers...)
					if result.Code != http.StatusNotFound {
						t.Fatalf("%s ranged=%v escaped private bytes: status=%d", method, ranged, result.Code)
					}
					if bytes.Contains(result.Body.Bytes(), payload) {
						t.Fatal("private body in rejected response")
					}
				}
			}
		})
	}
}

func TestGlobalMediaOrdinaryHashlessLegacyGETHEADRangeStillWorks(t *testing.T) {
	server, target, payload := aliasHTTPFixture(t, "", false)
	get := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil)
	if get.Code != 200 || !bytes.Equal(get.Body.Bytes(), payload) {
		t.Fatalf("legacy GET: status=%d bytes=%d", get.Code, get.Body.Len())
	}
	head := ut.PerformRequest(server.engine.Engine, http.MethodHead, target, nil)
	if head.Code != 200 || head.Header().Get("Content-Length") != fmt.Sprint(len(payload)) {
		t.Fatalf("legacy HEAD: %d length=%q", head.Code, head.Header().Get("Content-Length"))
	}
	ranged := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if ranged.Code != 206 || !bytes.Equal(ranged.Body.Bytes(), payload[:8]) {
		t.Fatalf("legacy Range: status=%d bytes=%d", ranged.Code, ranged.Body.Len())
	}
}

func TestGlobalMediaManifestDeletionCannotReleaseAliases(t *testing.T) {
	for _, prime := range []bool{false, true} {
		t.Run(fmt.Sprint("prior read=", prime), func(t *testing.T) {
			server, target, _ := aliasHTTPFixture(t, "", true)
			if prime {
				if before := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil); before.Code != 404 {
					t.Fatalf("private alias before removal=%d", before.Code)
				}
			}
			if err := os.Remove(filepath.Join(server.app.Media().Dir(), "story-assets.json")); err != nil {
				t.Fatal(err)
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				result := ut.PerformRequest(server.engine.Engine, method, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
				if result.Code != 404 {
					t.Fatalf("manifest removal exposed alias via %s: %d", method, result.Code)
				}
			}
		})
	}
}
func TestGlobalMediaNeverInstalledRestrictionKeepsLegacySupport(t *testing.T) {
	server, target, payload := aliasHTTPFixture(t, "", false, false)
	got := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil)
	if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), payload) {
		t.Fatalf("ordinary directory without story media failed: %d", got.Code)
	}
	head := ut.PerformRequest(server.engine.Engine, http.MethodHead, target, nil)
	if head.Code != 200 {
		t.Fatal("legacy HEAD rejected")
	}
	ranged := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil, ut.Header{Key: "Range", Value: "bytes=0-7"})
	if ranged.Code != 206 || !bytes.Equal(ranged.Body.Bytes(), payload[:8]) {
		t.Fatal("legacy Range rejected")
	}
}
