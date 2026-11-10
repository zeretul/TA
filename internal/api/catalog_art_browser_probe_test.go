package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"ta/internal/catalogart"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

// This opt-in bridge exposes the real catalog handler to a separate headless
// browser, using a temporary application and no model calls or player saves.
func TestNativeCatalogBrowserProbe(t *testing.T) {
	dir := os.Getenv("TA_NATIVE_BROWSER_DIR")
	if dir == "" {
		t.Skip("Opt-in isolated browser bridge")
	}
	server := NewServer(newTestApplication(t), "0")
	library := catalogart.Default()
	var variant catalogart.Variant
	for _, entry := range library.EntriesForAudit() {
		if entry.Kind == "definition" && entry.World == "lanchuan-qunxia" && entry.ID == "lanchuan-i01" {
			variant = entry.Variants["illustration"]
			break
		}
	}
	if variant.Format != "webp" {
		t.Fatal("Native fixture missing")
	}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<!doctype html><title>Native catalog image probe</title>"))
			return
		}
		if r.URL.Path == "/original.png" {
			data, err := os.ReadFile(filepath.Join(os.Getenv("TA_CATALOG_ORIGINAL_DIR"), variant.LegacySHA256+".png"))
			if err != nil {
				http.Error(w, "Original missing", 404)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(data)
			return
		}
		result := ut.PerformRequest(server.engine.Engine, r.Method, r.URL.RequestURI(), nil)
		result.Header().VisitAll(func(key, value []byte) { w.Header().Add(string(key), string(value)) })
		w.WriteHeader(result.Code)
		w.Write(result.Body.Bytes())
	}))
	defer bridge.Close()
	data, _ := json.Marshal(map[string]any{"url": bridge.URL, "variant": variant})
	if err := os.WriteFile(filepath.Join(dir, "browser-endpoint.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "browser-done")); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Browser probe did not finish")
}
