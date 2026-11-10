package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"ta/internal/buildinfo"
)

const (
	githubAPIBase         = "https://api.github.com/repos"
	updateInstallTimeout  = 30 * time.Minute
	updateDownloadTimeout = 20 * time.Minute
)

type Service struct {
	repository     string
	currentVersion string
	httpClient     *http.Client
	executablePath string
	githubAPIBase  string
}

func NewService() *Service {
	exe, _ := os.Executable()
	return &Service{
		repository:     buildinfo.UpdateRepository,
		currentVersion: buildinfo.Version,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
		executablePath: exe,
		githubAPIBase:  githubAPIBase,
	}
}

func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	if !ChannelConfigured(s.repository) {
		return CheckResult{CurrentVersion: s.currentVersion, ChannelConfigured: false}, nil
	}
	release, err := s.latestRelease(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	platform := platformKey(runtime.GOOS, runtime.GOARCH)
	asset := selectReleaseAsset(release.Assets, release.TagName, platform)
	current := s.currentVersion
	latest := normalizeVersion(release.TagName)
	updateAvailable := !isDevVersion(current) && latest != "" && compareVersions(current, latest) < 0
	result := CheckResult{
		ChannelConfigured: true,
		CurrentVersion:    current,
		LatestVersion:     latest,
		UpdateAvailable:   updateAvailable,
		CanInstall:        updateAvailable && asset != nil,
		Platform:          platform,
		ReleaseURL:        release.HTMLURL,
		PublishedAt:       release.PublishedAt,
		ReleaseNotes:      release.Body,
	}
	if asset != nil {
		result.Asset = &Asset{Name: asset.Name, Size: asset.Size, DownloadURL: asset.DownloadURL, BrowserDownloadURL: asset.BrowserDownloadURL}
	}
	return result, nil
}

var ErrUpdateChannelUnconfigured = errors.New("TA update channel is not configured")
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var stableTagPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// ChannelConfigured rejects the upstream attribution repository even when a
// release builder accidentally supplies it as the product update source.
func ChannelConfigured(repository string) bool {
	if !repositoryPattern.MatchString(repository) || strings.EqualFold(repository, buildinfo.Repository) {
		return false
	}
	for _, part := range strings.Split(repository, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (s *Service) latestRelease(ctx context.Context) (githubRelease, error) {
	if !ChannelConfigured(s.repository) {
		return githubRelease{}, ErrUpdateChannelUnconfigured
	}
	url := s.githubLatestReleaseURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TA-update-checker")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("check GitHub release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return githubRelease{}, fmt.Errorf("check GitHub release: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("decode GitHub release response: %w", err)
	}
	if release.Draft || release.Prerelease || !stableTagPattern.MatchString(release.TagName) {
		return githubRelease{}, errors.New("release is not a stable TA release")
	}
	return release, nil
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	HTMLURL     string        `json:"html_url"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadURL        string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func platformKey(goos, goarch string) string {
	arch := goarch
	if arch == "amd64" {
		arch = "x64"
	}
	return goos + "-" + arch
}

func selectAsset(assets []githubAsset, platform string) *githubAsset {
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if strings.Contains(name, strings.ToLower(platform)) && strings.HasPrefix(name, "ta-") &&
			(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip")) {
			return &assets[i]
		}
	}
	return nil
}

func selectReleaseAsset(assets []githubAsset, tag, platform string) *githubAsset {
	extension := ".tar.gz"
	if strings.HasPrefix(platform, "windows-") {
		extension = ".zip"
	}
	expected := "TA-" + tag + "-" + platform + extension
	for i := range assets {
		if assets[i].Name == expected {
			return &assets[i]
		}
	}
	return nil
}

func selectChecksumAsset(assets []githubAsset) *githubAsset {
	for i := range assets {
		if strings.EqualFold(assets[i].Name, "checksums.txt") {
			return &assets[i]
		}
	}
	return nil
}

func (s *Service) githubLatestReleaseURL() string {
	base := strings.TrimRight(s.githubAPIBase, "/")
	if base == "" {
		base = githubAPIBase
	}
	return base + "/" + strings.Trim(s.repository, "/") + "/releases/latest"
}

func (s *Service) downloadHTTPClient() *http.Client {
	if s.httpClient == nil {
		return &http.Client{}
	}
	client := *s.httpClient
	client.Timeout = 0
	return &client
}

func updateAssetDownloadURL(asset *Asset) string {
	if asset == nil {
		return ""
	}
	if strings.TrimSpace(asset.BrowserDownloadURL) != "" {
		return asset.BrowserDownloadURL
	}
	return asset.DownloadURL
}

func githubAssetDownloadURL(asset githubAsset) string {
	if strings.TrimSpace(asset.BrowserDownloadURL) != "" {
		return asset.BrowserDownloadURL
	}
	return asset.DownloadURL
}
