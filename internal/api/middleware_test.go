package api

import (
	"context"
	"ta/internal/update"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"ta/config"
	runtimeapp "ta/internal/app"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestNewServerUsesLocalHostByDefault(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if server.host != config.LocalHTTPHost {
		t.Fatalf("server host = %q, want %q", server.host, config.LocalHTTPHost)
	}
}

func TestNewServerUsesLANHostWhenEnabled(t *testing.T) {
	root := t.TempDir()
	hash, err := config.HashRemoteAccessPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:              "test-model",
		NovaDir:                  root,
		Workspace:                root,
		ResumeLastWorkspace:      false,
		AllowLANAccess:           true,
		RemoteAccessUsername:     "reader",
		RemoteAccessPasswordHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")
	if server.host != config.LANHTTPHost {
		t.Fatalf("server host = %q, want %q", server.host, config.LANHTTPHost)
	}
}

func TestIsLocalClientIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "::1"} {
		if !isLocalClientIP(value) {
			t.Fatalf("%s should be local", value)
		}
	}
	for _, value := range []string{"192.168.1.8", "10.0.0.2", ""} {
		if isLocalClientIP(value) {
			t.Fatalf("%s should be remote", value)
		}
	}
}

func TestForwardedClientIPUsesFirstValidAddress(t *testing.T) {
	got := forwardedClientIP(" 192.168.1.8, 127.0.0.1")
	if got != "192.168.1.8" {
		t.Fatalf("forwardedClientIP = %q", got)
	}
	if got := forwardedClientIP("unknown, "); got != "" {
		t.Fatalf("invalid forwarded header should be ignored: %q", got)
	}
}

func TestLocalHostEffectMiddlewareRejectsForwardedRemoteClient(t *testing.T) {
	called := false
	server := hertzserver.Default()
	server.POST("/native", localHostEffectMiddleware, func(_ context.Context, c *app.RequestContext) {
		called = true
		c.Status(http.StatusNoContent)
	})
	response := ut.PerformRequest(
		server.Engine,
		http.MethodPost,
		"/native",
		nil,
		ut.Header{Key: "X-Forwarded-For", Value: "192.168.1.8"},
		ut.Header{Key: "X-Denova-Locale", Value: "en-US"},
	)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("remote host effect response = %d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestUpdateAdmissionMiddlewareKeepsActiveRequestSafe(t *testing.T) {
	application := &runtimeapp.App{}
	server := hertzserver.Default()
	entered := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan struct{})
	server.Use(updateAdmissionMiddleware(application))
	server.POST("/api/write", func(_ context.Context, c *app.RequestContext) {
		close(entered)
		<-finish
		c.Status(http.StatusNoContent)
	})
	go func() { ut.PerformRequest(server.Engine, http.MethodPost, "/api/write", nil); close(done) }()
	<-entered
	if _, err := application.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("active request: %v", err)
	}
	close(finish)
	<-done
}

// While apply holds the exclusive side, a request that arrives during the
// restart window must be refused with a code the client can act on. Without
// the code the response degrades to "http.409" and the player is told the
// generic operation-failed message instead of the update-specific one.
func TestUpdateAdmissionMiddlewareRefusalNamesItsCode(t *testing.T) {
	application := &runtimeapp.App{}
	server := hertzserver.Default()
	entered := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan struct{})
	server.Use(updateAdmissionMiddleware(application))
	server.POST("/api/write", func(_ context.Context, c *app.RequestContext) {
		close(entered)
		<-finish
		c.Status(http.StatusNoContent)
	})
	go func() { ut.PerformRequest(server.Engine, http.MethodPost, "/api/write", nil); close(done) }()
	<-entered
	if _, err := application.ApplyUpdate(context.Background()); !errors.Is(err, update.ErrUpdateBusy) {
		t.Fatalf("active request: %v", err)
	}
	close(finish)
	<-done

	// The refusal body is what the client reads, so assert on it directly.
	recorder := hertzserver.Default()
	recorder.GET("/refused", func(_ context.Context, c *app.RequestContext) {
		abortWithLocalizedError(c, http.StatusConflict, "api.update.busy")
	})
	response := ut.PerformRequest(recorder.Engine, http.MethodGet, "/refused", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refusal: %v body=%s", err, response.Body.String())
	}
	if body["code"] != "api.update.busy" {
		t.Fatalf("code = %q, want api.update.busy", body["code"])
	}
	if body["error"] == "" {
		t.Fatalf("refusal has no message: %s", response.Body.String())
	}
}
