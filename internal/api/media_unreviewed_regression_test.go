package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"ta/config"
	runtimeapp "ta/internal/app"
)

func TestMediaRouteCannotBypassPendingReview(t *testing.T) {
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const file = "bbbbbbbbbbbbbbbb.png"
	if err := os.WriteFile(filepath.Join(mediaDir, file), []byte("pending review bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"ta.media/v2","entries":[{"reviewed":false,"world":"one","source_key":"person","purpose":"portrait","file":"bbbbbbbbbbbbbbbb.png"}]}`
	if err := os.WriteFile(filepath.Join(mediaDir, "media.json"), []byte(manifest), 0o600); err != nil {
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
	server := NewServer(application, "0")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			recorder := ut.PerformRequest(server.engine.Engine, method, "/api/media/"+file, nil)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("unreviewed file status: got %d, want 404", recorder.Code)
			}
		})
	}
	recorder := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/media/"+file, nil, ut.Header{Key: "Range", Value: "bytes=0-3"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unreviewed range status: got %d, want 404", recorder.Code)
	}
}
