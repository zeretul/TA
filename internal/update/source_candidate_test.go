package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestReleaseChannelRejectsUnconfiguredAndUpstream(t *testing.T) {
	for _, repo := range []string{"", "alfredxw/denova", "ALFREDXW/DENOVA", "bad/repo/extra", "../ta", "owner/.."} {
		s := &Service{repository: repo, currentVersion: "1.0.0", httpClient: http.DefaultClient}
		result, err := s.Check(context.Background())
		if err != nil || result.ChannelConfigured || result.CurrentVersion != "1.0.0" {
			t.Errorf("repository %q: %+v, %v", repo, result, err)
		}
	}
}

func TestReleaseChannelRejectsDraftAndPrerelease(t *testing.T) {
	for _, release := range []githubRelease{{TagName: "v1.2.3", Draft: true}, {TagName: "v1.2.3", Prerelease: true}, {TagName: "v1.2.3-rc1"}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(release) }))
		s := &Service{repository: "owner/ta", currentVersion: "1.0.0", httpClient: server.Client(), githubAPIBase: server.URL}
		_, err := s.Check(context.Background())
		server.Close()
		if err == nil {
			t.Errorf("accepted non-stable release: %+v", release)
		}
	}
}

func TestSelectReleaseAssetRequiresExactVersionPlatformAndFormat(t *testing.T) {
	assets := []githubAsset{
		{Name: "TA-v9.0.0-windows-x64.zip"},
		{Name: "TA-v1.2.3-windows-x64-extra.zip"},
		{Name: "TA-v1.2.3-windows-x64.tar.gz"},
		{Name: "TA-v1.2.3-windows-x64.zip"},
	}
	got := selectReleaseAsset(assets, "v1.2.3", "windows-x64")
	if got == nil || got.Name != "TA-v1.2.3-windows-x64.zip" {
		t.Fatalf("asset = %#v", got)
	}
	if got := selectReleaseAsset(assets[:3], "v1.2.3", "windows-x64"); got != nil {
		t.Fatalf("wrong asset = %#v", got)
	}
}

func TestCheckStableReleaseAvailability(t *testing.T) {
	tests := []struct {
		name, tag, asset   string
		available, install bool
	}{
		{"current", "v1.0.0", "", false, false},
		{"new missing asset", "v1.2.3", "", true, false},
		{"new matching asset", "v1.2.3", "TA-v1.2.3-" + platformKey(runtime.GOOS, runtime.GOARCH), true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := tt.asset
			if asset != "" {
				if runtime.GOOS == "windows" {
					asset += ".zip"
				} else {
					asset += ".tar.gz"
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(githubRelease{TagName: tt.tag, Assets: []githubAsset{{Name: asset}}})
			}))
			defer server.Close()
			s := &Service{repository: "owner/ta", currentVersion: "1.0.0", httpClient: server.Client(), githubAPIBase: server.URL}
			result, err := s.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.UpdateAvailable != tt.available || result.CanInstall != tt.install {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestCheckReleaseNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	s := &Service{repository: "owner/ta", currentVersion: "1.0.0", httpClient: server.Client(), githubAPIBase: server.URL}
	if _, err := s.Check(context.Background()); err == nil {
		t.Fatal("expected network failure")
	}
}
