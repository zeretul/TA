package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"ta/config"
	runtimeapp "ta/internal/app"
)

// clipPayload is deliberately bigger than one read so the stream has to produce
// several chunks, and it is not text: a clip is binary and a byte-for-byte
// comparison is the only honest check.
func clipPayload() []byte {
	payload := make([]byte, 64*1024+37)
	for i := range payload {
		payload[i] = byte(i*7 + 11)
	}
	return payload
}

// mediaTestServer builds a server whose media directory holds one manifest entry
// and one clip file on disk.
func mediaTestServer(t *testing.T) (*Server, string, []byte) {
	t.Helper()
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := clipPayload()
	const file = "aaaaaaaaaaaaaaaa.mp4"
	if err := os.WriteFile(filepath.Join(mediaDir, file), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schema":"ta.media/v1","entries":[{"reviewed":true,"source_key":"npc_test","file":%q,"sha256":%q,"bytes":%d,"duration_ms":1000,"width":640,"height":640,"role":"general"}]}`,
		file, hex.EncodeToString(func() []byte { sum := sha256.Sum256(payload); return sum[:] }()), len(payload))
	if err := os.WriteFile(filepath.Join(mediaDir, "media.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	application, err := runtimeapp.New(t.Context(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "http://127.0.0.1:1/v1", MediaDir: mediaDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	// The real server, not a bare engine: the gzip and observability middleware
	// sit in front of the media route in production, and both have to be present
	// for these checks to mean anything.
	return NewServer(application, "0"), file, payload
}

// TestMediaClipDeliversEveryByte is the regression for a clip that answered 200
// with a correct Content-Length and an empty body: the handler closed the file
// when it returned, and hertz reads the body stream only afterwards.
func TestMediaClipDeliversEveryByte(t *testing.T) {
	server, file, payload := mediaTestServer(t)

	recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+file, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got, want := recorder.Header().Get("Content-Length"), fmt.Sprint(len(payload)); got != want {
		t.Fatalf("Content-Length: got %q want %q", got, want)
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, payload) {
		t.Fatalf("body: got %d bytes, want %d (first divergence at %d)",
			len(got), len(payload), firstDivergence(got, payload))
	}
}

// TestMediaClipRangeReturnsExactSpan checks the byte span itself, not only the
// 206 and the Content-Range header. A seek that receives the right headers and
// the wrong bytes shows a frozen or garbled frame.
func TestMediaClipRangeReturnsExactSpan(t *testing.T) {
	server, file, payload := mediaTestServer(t)

	recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+file, nil,
		ut.Header{Key: "Range", Value: "bytes=1000-2023"})
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got, want := recorder.Header().Get("Content-Range"), fmt.Sprintf("bytes 1000-2023/%d", len(payload)); got != want {
		t.Fatalf("Content-Range: got %q want %q", got, want)
	}
	got := recorder.Body.Bytes()
	want := payload[1000:2024]
	if !bytes.Equal(got, want) {
		t.Fatalf("range body: got %d bytes, want %d (first divergence at %d)",
			len(got), len(want), firstDivergence(got, want))
	}
}

// TestMediaClipOpenEndedRange covers the form a player actually sends first:
// an open-ended range that must run to the end of the file.
func TestMediaClipOpenEndedRange(t *testing.T) {
	server, file, payload := mediaTestServer(t)

	recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+file, nil,
		ut.Header{Key: "Range", Value: "bytes=60000-"})
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
	}
	got := recorder.Body.Bytes()
	want := payload[60000:]
	if !bytes.Equal(got, want) {
		t.Fatalf("open-ended range: got %d bytes, want %d", len(got), len(want))
	}
}

// TestMediaClipHeadAnswersForTheClip pins the method a player uses to size the
// stream. An unregistered HEAD used to fall through to the SPA fallback, which
// answered index.html: a 200 with a text/html type and the wrong length.
func TestMediaClipHeadAnswersForTheClip(t *testing.T) {
	server, file, payload := mediaTestServer(t)

	recorder := ut.PerformRequest(server.engine.Engine, http.MethodHead, "/api/media/"+file, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d", recorder.Code)
	}
	if got, want := recorder.Header().Get("Content-Type"), "video/mp4"; got != want {
		t.Fatalf("Content-Type: got %q want %q", got, want)
	}
	if got, want := recorder.Header().Get("Content-Length"), fmt.Sprint(len(payload)); got != want {
		t.Fatalf("Content-Length: got %q want %q", got, want)
	}
	if got := recorder.Body.Len(); got != 0 {
		t.Fatalf("HEAD must not carry a body: got %d bytes", got)
	}
}

// TestMediaClipIsNotCompressed keeps the range contract intact for a browser.
// Every browser sends Accept-Encoding: gzip; compressing a clip would rewrite
// the Content-Length and hand back a gzip stream where the player asked for a
// byte span of the file.
func TestMediaClipIsNotCompressed(t *testing.T) {
	server, file, payload := mediaTestServer(t)

	recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+file, nil,
		ut.Header{Key: "Accept-Encoding", Value: "gzip, deflate, br"})
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("a clip must not be compressed: Content-Encoding=%q", got)
	}
	if got, want := recorder.Header().Get("Content-Length"), fmt.Sprint(len(payload)); got != want {
		t.Fatalf("Content-Length: got %q want %q", got, want)
	}
	if got := recorder.Body.Bytes(); !bytes.Equal(got, payload) {
		t.Fatalf("body: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestMediaClipUnknownFileIsNotFound keeps the lookup boundary: only a file the
// manifest names can be read, whatever is on disk.
func TestMediaClipUnknownFileIsNotFound(t *testing.T) {
	server, _, _ := mediaTestServer(t)

	for _, name := range []string{"not-listed.mp4", "media.json", "..", "."} {
		recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+name, nil)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%q: got %d, want 404 (body=%s)", name, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCharacterPictureReadFailureIsLocalized(t *testing.T) {
	server, _, _ := mediaTestServer(t)
	response := ut.PerformRequest(server.engine.Engine, http.MethodGet,
		"/api/interactive/stories/missing-story/characters/missing-person/picture",
		nil, ut.Header{Key: "Accept-Language", Value: "zh-CN"})
	if response.Code != http.StatusNotFound || !bytes.Contains(response.Body.Bytes(), []byte(`"code":"api.media.notFound"`)) {
		t.Fatalf("picture read did not use its localized error: status=%d body=%s", response.Code, response.Body.String())
	}
}

func firstDivergence(got, want []byte) int {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for i := 0; i < limit; i++ {
		if got[i] != want[i] {
			return i
		}
	}
	return limit
}
