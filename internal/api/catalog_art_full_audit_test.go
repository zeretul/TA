package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"ta/internal/catalogart"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

// This explicit release audit verifies the entire independently generated URL
// inventory against the actual HTTP handler without opening sockets or saves.
func TestNativeFullReleaseURLAudit(t *testing.T) {
	path := os.Getenv("TA_CATALOG_URL_AUDIT")
	if path == "" {
		t.Skip("Full URL inventory required for release audit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		URLs  map[string]string `json:"urls"`
		Count int               `json:"historical_urls_verified"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.URLs) == 0 || len(report.URLs) != report.Count {
		t.Fatal("Invalid URL inventory")
	}
	server := NewServer(newTestApplication(t), "0")
	files := make([]string, 0, len(report.URLs))
	canonical := map[string]bool{}
	for file, target := range report.URLs {
		files = append(files, file)
		canonical[target] = true
	}
	sort.Strings(files)
	redirects := 0
	for _, file := range files {
		target := report.URLs[file]
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			response := ut.PerformRequest(server.engine.Engine, method, catalogart.PathPrefix+file, nil)
			if file != target {
				if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != catalogart.PathPrefix+target || response.Body.Len() != 0 {
					t.Fatalf("Historical URL redirect failed: %s %s, status=%d location=%q", method, file, response.Code, response.Header().Get("Location"))
				}
				if !strings.Contains(response.Header().Get("Cache-Control"), "immutable") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("Missing redirect safety/cache headers: %s", file)
				}
			} else if response.Code != http.StatusOK {
				t.Fatalf("Canonical historical URL failed: %s %s, status=%d", method, file, response.Code)
			}
		}
		if file != target {
			redirects++
		}
	}
	bytesVerified := int64(0)
	for file := range canonical {
		response := ut.PerformRequest(server.engine.Engine, http.MethodGet, catalogart.PathPrefix+file, nil)
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/webp" || response.Header().Get("Content-Encoding") != "" {
			t.Fatalf("Canonical WebP response failed: %s status=%d MIME=%q encoding=%q", file, response.Code, response.Header().Get("Content-Type"), response.Header().Get("Content-Encoding"))
		}
		if response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) || response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
			t.Fatalf("Canonical response headers failed: %s", file)
		}
		h := sha256.Sum256(response.Body.Bytes())
		if hex.EncodeToString(h[:])+".webp" != file {
			t.Fatalf("Canonical response hash failed: %s", file)
		}
		if body := response.Body.Bytes(); len(body) < 20 || string(body[:4]) != "RIFF" || string(body[8:16]) != "WEBPVP8L" {
			t.Fatalf("Not plain lossless WebP: %s", file)
		}
		head := ut.PerformRequest(server.engine.Engine, http.MethodHead, catalogart.PathPrefix+file, nil)
		if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != response.Header().Get("Content-Length") || head.Header().Get("Content-Type") != "image/webp" {
			t.Fatalf("Canonical HEAD failed: %s", file)
		}
		frontend := ut.PerformRequest(server.engine.Engine, http.MethodGet, catalogart.PathPrefix+strings.TrimSuffix(file, ".webp")+".png", nil)
		if frontend.Code != http.StatusPermanentRedirect || frontend.Header().Get("Location") != catalogart.PathPrefix+file {
			t.Fatalf("Frontend current .png URL failed: %s", file)
		}
		bytesVerified += int64(response.Body.Len())
	}
	missing := ut.PerformRequest(server.engine.Engine, http.MethodGet, catalogart.PathPrefix+strings.Repeat("0", 64)+".webp", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatal("Unknown artwork did not return 404")
	}
	t.Logf("Verified %d registered source URLs (%d redirects), %d canonical WebP GET/HEAD/current-PNG URLs, %d exact-hash bytes, and unknown 404", len(files), redirects, len(canonical), bytesVerified)
}
