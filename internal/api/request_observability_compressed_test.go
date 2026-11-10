package api

import (
	"bytes"
	stdgzip "compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ta/internal/observability"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/hertz-contrib/gzip"
)

// The correlation contract is what a bug report carries: request_id, the
// domain code, backend_version, platform and the route. It is applied at the
// HTTP boundary by requestObservabilityMiddleware, which reads the response
// body back after the handler has written it.
//
// A browser sends Accept-Encoding: gzip on every request, including the ones
// that fail. If the gzip middleware runs inside the observability middleware it
// replaces the body with a stream, the read-back sees nothing, and the envelope
// is skipped: the client gets the raw error, without the code, without a
// request id, and with a local absolute path that DiagnosticText would
// otherwise have redacted. This test pins the order the real server uses —
// compression outside, diagnostics inside — so the contract holds whether or
// not the client asked for compression.
func compressedErrorServer() *hertzserver.Hertz {
	server := hertzserver.Default()
	server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(requestObservabilityMiddleware)
	server.GET("/files/:id", func(_ context.Context, c *app.RequestContext) {
		c.JSON(http.StatusNotFound, map[string]any{
			"error": "open D:\\games\\save\\chapter.jsonl: The system cannot find the file specified.",
		})
	})
	return server
}

func TestCompressedErrorKeepsTheDiagnosticsEnvelope(t *testing.T) {
	server := compressedErrorServer()
	request := ut.PerformRequest(server.Engine, http.MethodGet, "/files/missing", nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip"})
	if request.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("the response was not compressed, so this is not the case under test: %q", request.Header().Get("Content-Encoding"))
	}
	reader, err := stdgzip.NewReader(bytes.NewReader(request.Body.Bytes()))
	if err != nil {
		t.Fatalf("compressed error body is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("compressed error body could not be read: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(decoded, &body); err != nil {
		t.Fatalf("compressed error body is not JSON: %v (%q)", err, string(decoded))
	}
	details, _ := body["details"].(map[string]any)
	if details == nil {
		t.Fatalf("the compressed error lost its details envelope: %s", string(decoded))
	}
	if body["request_id"] == nil || body["request_id"] != request.Header().Get(observability.RequestIDHeader) {
		t.Fatalf("the compressed error lost its request id: %s", string(decoded))
	}
	if code, _ := body["code"].(string); code == "" {
		t.Fatalf("the compressed error lost its domain code: %s", string(decoded))
	}
	if details["backend_version"] == nil || details["platform"] == nil || details["operation"] != "GET /files/:id" {
		t.Fatalf("the compressed error lost its build context: %#v", details)
	}
	if strings.Contains(string(decoded), `games\\save`) {
		t.Fatalf("the compressed error exposed an absolute path: %s", string(decoded))
	}
}
