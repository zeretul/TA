package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"ta/internal/catalogart"
	"ta/internal/cultivation"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestNativeCatalogRedirectsOldAndFrontendPNGURLsToExactWebP(t *testing.T) {
	server := NewServer(newTestApplication(t), "0")
	library := catalogart.Default()
	var variant catalogart.Variant
	for _, entry := range library.EntriesForAudit() {
		if entry.Kind == "definition" && entry.World == "lanchuan-qunxia" && entry.ID == "lanchuan-i01" {
			variant = entry.Variants["illustration"]
			break
		}
	}
	if variant.Format != "webp" || variant.LegacySHA256 == "" {
		t.Skip("Native fixture is not installed")
	}
	canonical := catalogart.PathPrefix + variant.SHA256 + ".webp"
	for _, id := range []string{variant.SHA256, variant.LegacySHA256} {
		if err := (cultivation.DefinitionArt{Source: "catalog", CatalogID: id}).Validate(); err != nil {
			t.Fatalf("Saved art identity rejected: %v", err)
		}
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			response := ut.PerformRequest(server.engine.Engine, method, catalogart.PathPrefix+id+".png", nil)
			if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != canonical || response.Body.Len() != 0 {
				t.Fatalf("Bad redirect: %s %d %q", method, response.Code, response.Header().Get("Location"))
			}
			if !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("Redirect cache header absent")
			}
		}
	}
	response := ut.PerformRequest(server.engine.Engine, http.MethodGet, canonical, nil)
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("Native WebP response: %d %s", response.Code, response.Header().Get("Content-Type"))
	}
	h := sha256.Sum256(response.Body.Bytes())
	if hex.EncodeToString(h[:]) != variant.SHA256 {
		t.Fatal("Response bytes mismatch canonical URL hash")
	}
	for _, entry := range library.EntriesForAudit() {
		if entry.Kind == "npc" {
			for _, v := range entry.Variants {
				if v.LegacySHA256 != "" && library.IsDefinitionAsset(v.LegacySHA256) {
					t.Fatal("Legacy portrait alias became definition art")
				}
			}
		}
	}
}
