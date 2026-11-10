package api

import (
	"bytes"
	"fmt"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"net/http"
	"strings"
	"ta/internal/catalogart"
	"testing"
)

func TestCatalogArtRouteReadsOnlyRegisteredImagesAndHandlesHEAD(t *testing.T) {
	server := NewServer(newTestApplication(t), "0")
	library := catalogart.Default()
	id := library.AssetID("lanchuan-qunxia", "definition", "lanchuan-i01", 1, "illustration")
	if id == "" {
		t.Fatal("dedicated fixture image missing")
	}
	file := library.CanonicalFile(id + ".png")
	data, err := library.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := ut.PerformRequest(server.engine.Engine, method, catalogart.PathPrefix+file, nil, ut.Header{Key: "Accept-Encoding", Value: "gzip"})
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != library.ContentType(file) {
			t.Fatalf("%s: %d %s", method, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Length") != fmt.Sprint(len(data)) || response.Header().Get("Content-Encoding") != "" {
			t.Fatalf("%s PNG transport: length=%q want=%d encoding=%q", method, response.Header().Get("Content-Length"), len(data), response.Header().Get("Content-Encoding"))
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("content-addressed image headers missing")
		}
		if method == http.MethodGet && !bytes.Equal(response.Body.Bytes(), data) {
			t.Fatal("image bytes changed")
		}
		if method == http.MethodHead && response.Body.Len() != 0 {
			t.Fatal("HEAD returned a body")
		}
	}
	for _, file := range []string{"manifest.json", strings.Repeat("0", 64) + ".png", id + ".svg", "..%2fmanifest.json"} {
		response := ut.PerformRequest(server.engine.Engine, http.MethodGet, catalogart.PathPrefix+file, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%q returned %d", file, response.Code)
		}
	}
}
