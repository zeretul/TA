package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"ta/internal/media"
)

func TestGlobalMediaAssetRevocationCannotReleaseAliasGETHEADRange(t *testing.T) {
	for _, declared := range []string{"", strings.Repeat("f", 64)} {
		for _, prime := range []bool{false, true} {
			t.Run(fmt.Sprintf("sha=%q/prime=%v", declared, prime), func(t *testing.T) {
				server, target, payload := aliasHTTPFixture(t, declared, true)
				if prime {
					if before := ut.PerformRequest(server.engine.Engine, http.MethodGet, target, nil); before.Code != http.StatusNotFound {
						t.Fatalf("private alias before revocation: %d", before.Code)
					}
				}
				path := filepath.Join(server.app.Media().Dir(), "story-assets.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var manifest media.StoryManifest
				if err := json.Unmarshal(raw, &manifest); err != nil {
					t.Fatal(err)
				}
				if len(manifest.Assets) != 1 {
					t.Fatal("fixture must have exactly one restricted asset")
				}
				manifest.Assets = nil
				raw, err = json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				for _, method := range []string{http.MethodGet, http.MethodHead} {
					for _, ranged := range []bool{false, true} {
						var headers []ut.Header
						if ranged {
							headers = append(headers, ut.Header{Key: "Range", Value: "bytes=0-7"})
						}
						result := ut.PerformRequest(server.engine.Engine, method, target, nil, headers...)
						if result.Code != http.StatusNotFound {
							t.Fatalf("asset revocation exposed alias via %s ranged=%v: %d", method, ranged, result.Code)
						}
						if bytes.Contains(result.Body.Bytes(), payload[:8]) {
							t.Fatal("rejected response contained private PNG bytes")
						}
					}
				}
			})
		}
	}
}
