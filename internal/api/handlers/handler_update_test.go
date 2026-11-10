package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	denovaapp "ta/internal/app"
	"ta/internal/buildinfo"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestLocalUpdateUploadValidationUsesRequestLocale(t *testing.T) {
	previous := buildinfo.Version
	buildinfo.Version = "0.4.5"
	t.Cleanup(func() { buildinfo.Version = previous })
	for _, tc := range []struct{ locale, missing, invalid string }{
		{"en-US", "Select a GitHub Release archive.", "The package is invalid or incomplete. Upload an unmodified Text Adventure stable release archive without extracting or renaming it."},
		{"zh-CN", "请选择 GitHub Release 安装包。", "安装包无效或不完整。请上传未经解压、重命名的 Text Adventure 正式发布压缩包。"},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", tc.locale)
			New(&denovaapp.App{}).HandleUpdateUpload(context.Background(), c)
			var body map[string]any
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if c.Response.StatusCode() != 400 || body["error"] != tc.missing {
				t.Fatalf("missing file: %d %s", c.Response.StatusCode(), c.Response.Body())
			}
			var upload bytes.Buffer
			writer := multipart.NewWriter(&upload)
			file, err := writer.CreateFormFile("file", "source.zip")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write([]byte("invalid")); err != nil {
				t.Fatal(err)
			}
			checksum, err := writer.CreateFormFile("checksums", "checksums.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = checksum.Write([]byte("unused"))
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request.SetBody(upload.Bytes())
			c.Response.Reset()
			New(&denovaapp.App{}).HandleUpdateUpload(context.Background(), c)
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if c.Response.StatusCode() != 400 || body["error"] != tc.invalid {
				t.Fatalf("invalid file: %d %s", c.Response.StatusCode(), c.Response.Body())
			}
		})
	}
}

type unavailableUpdateTransport struct{}

func (unavailableUpdateTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("private upstream diagnostics")
}

// TestUpstreamUpdateRefusalsUseRequestLocale covers the shipped answer of the
// upstream channel, in both locales.
//
// The channel is closed for this distribution, so the check and the install
// refuse before any network work. The refusal is what the player actually sees,
// which makes it the thing worth asserting: a 403 carrying the localized
// explanation, rather than the 502 gateway error the transport would have
// produced.
//
// The transport is failed on purpose. If the guard sat below the request it
// would be the transport's error that surfaced, and this test would see the
// gateway message instead of the refusal. That is what pins the guard above the
// network work rather than merely somewhere in the call.
//
// The transport-failure branch of these handlers — the one that reports
// api.update.checkFailed and api.update.installFailed — is therefore no longer
// reachable through the API in this build. Its localized strings are kept for
// distributions that do publish releases, and are exercised where the guard can
// be released, but they cannot be reached from here.
func TestUpstreamUpdateRefusalsUseRequestLocale(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = unavailableUpdateTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	handlers := New(&denovaapp.App{})
	for _, test := range []struct{ locale, want string }{
		{"zh-CN", "此版本尚未配置 Text Adventure 在线更新渠道，请使用经过校验的离线更新包。"},
		{"en-US", "This build has no configured Text Adventure update channel. Use a verified offline package."},
	} {
		t.Run(test.locale, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.Set("X-Denova-Locale", test.locale)

			for name, call := range map[string]func(context.Context, *app.RequestContext){
				"install": handlers.HandleUpdateInstall,
			} {
				request.Response.Reset()
				call(context.Background(), request)
				var body map[string]any
				if err := json.Unmarshal(request.Response.Body(), &body); err != nil {
					t.Fatal(err)
				}
				if request.Response.StatusCode() != 403 || body["code"] != "api.update.upstreamDisabled" || body["error"] != test.want {
					t.Fatalf("%s response = %d %s", name, request.Response.StatusCode(), request.Response.Body())
				}
			}

			// The streamed install reports through its event channel instead of a
			// status code, so it is checked for the same localized refusal.
			task := handlers.app.StartInstallUpdateTask(test.locale)
			select {
			case <-task.Done():
			case <-time.After(time.Second):
				t.Fatal("update task did not finish")
			}
			events, subscription := task.Subscribe()
			defer task.Unsubscribe(subscription)
			found := false
			for _, event := range events {
				if event.Event.Type != "error" {
					continue
				}
				data, ok := event.Event.Data.(map[string]any)
				if !ok || data["message"] != test.want || data["code"] != "api.update.upstreamDisabled" {
					t.Fatalf("stream error = %#v", event)
				}
				found = true
			}
			if !found {
				t.Fatal("update task did not emit an error")
			}
		})
	}
}

func TestUpdateCheckAndStatusWithoutConfiguredChannel(t *testing.T) {
	previous := buildinfo.UpdateRepository
	buildinfo.UpdateRepository = ""
	defer func() { buildinfo.UpdateRepository = previous }()
	h := New(&denovaapp.App{})
	for _, call := range []func(context.Context, *app.RequestContext){h.HandleUpdateCheck, h.HandleUpdateStatus} {
		c := app.NewContext(0)
		call(context.Background(), c)
		var body map[string]any
		if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
			t.Fatal(err)
		}
		if c.Response.StatusCode() != 200 || body["channel_configured"] != false || body["current_version"] != buildinfo.Version {
			t.Fatalf("response: %d %s", c.Response.StatusCode(), c.Response.Body())
		}
	}
}
