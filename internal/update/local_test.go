package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ta/internal/hostruntime"
)

func TestInstallLocalRelease(t *testing.T) {
	exeName, extension := "TA", ".tar.gz"
	if runtime.GOOS == "windows" {
		exeName, extension = "TA.exe", ".zip"
	}
	packageFiles := map[string]string{
		"web/index.html":                               "<html>release</html>",
		"skills/example/SKILL.md":                      "skill",
		"tools/" + hostruntime.RipgrepExecutableName(): "ripgrep",
		"licenses/ripgrep/LICENSE-MIT":                 "MIT",
		"licenses/ripgrep/UNLICENSE":                   "Unlicense",
	}
	packageFiles[exeName] = "new executable"
	packageFiles[updaterExecutableName()] = "new updater"
	archiveName := "TA-v0.5.0-" + platformKey(runtime.GOOS, runtime.GOARCH) + extension
	archive := localTestArchive(t, packageFiles, extension)
	installDir := t.TempDir()
	service := &Service{currentVersion: "0.4.5", executablePath: filepath.Join(installDir, exeName)}
	if err := os.WriteFile(service.executablePath, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := service.InstallLocal(context.Background(), archiveName, bytes.NewReader(archive), localChecksums(archiveName, archive))
	if err != nil {
		t.Fatal(err)
	}
	status, statusErr := service.Status(context.Background())
	if statusErr != nil || !status.Staged || !status.ApplyReady || status.InstalledVersion != "0.5.0" {
		t.Fatalf("staged status = %+v, %v", status, statusErr)
	}
	if !result.ApplyReady || !result.Staged || result.Installed || result.PreviousVersion != "0.4.5" || result.InstalledVersion != "0.5.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
	stagedWeb := filepath.Join(result.StagedPath, "web", "index.html")
	originalWeb, err := os.ReadFile(stagedWeb)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedWeb, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Status(context.Background()); err == nil {
		t.Fatal("tampered staged package accepted")
	}
	if err := os.WriteFile(stagedWeb, originalWeb, 0644); err != nil {
		t.Fatal(err)
	}
	firstStage := result.StagedPath
	result, err = service.InstallLocal(context.Background(), archiveName, bytes.NewReader(archive), localChecksums(archiveName, archive))
	if err != nil || result.StagedPath == firstStage {
		t.Fatalf("restage did not use distinct pending path: %+v, %v", result, err)
	}
	if _, err := os.Stat(firstStage); err != nil {
		t.Fatalf("previous stage removed: %v", err)
	}
	manifestPath, err := readPendingManifestRef(updateDataDir(installDir))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.5.0" || manifest.SourceDir != result.StagedPath {
		t.Fatalf("manifest: %#v", manifest)
	}
	service.currentVersion = "0.5.0"
	currentStatus, err := service.Status(context.Background())
	if err != nil || currentStatus.ApplyReady || currentStatus.Staged {
		t.Fatalf("already applied status = %+v, %v", currentStatus, err)
	}
	service.currentVersion = "0.4.5"
	content, _ := os.ReadFile(service.executablePath)
	if string(content) != "current" {
		t.Fatal("upload replaced the running installation before apply")
	}
	for _, tc := range []struct {
		name, current, archive string
		data                   []byte
		want                   error
	}{
		{"same version", "0.5.0", archiveName, archive, ErrPackageVersion},
		{"older version", "0.6.0", archiveName, archive, ErrPackageVersion},
		{"development", "dev", archiveName, archive, ErrPackageVersion},
		{"invalid name", "0.4.5", "source.zip", archive, ErrInvalidPackage},
		{"traversal", "0.4.5", "../" + archiveName, archive, ErrInvalidPackage},
		{"corrupt archive", "0.4.5", archiveName, []byte("invalid archive"), ErrInvalidPackage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service.currentVersion = tc.current
			if _, err := service.InstallLocal(context.Background(), tc.archive, bytes.NewReader(tc.data), localChecksums(tc.archive, tc.data)); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			pending, err := readPendingManifestRef(updateDataDir(installDir))
			if err != nil || pending != manifestPath {
				t.Fatalf("failed upload changed pending update: %s, %v", pending, err)
			}
		})
	}
	service.currentVersion = "0.4.5"
	wrongPlatform := "windows-x64"
	if runtime.GOOS == "windows" {
		wrongPlatform = "linux-x64"
	}
	if _, err := service.InstallLocal(context.Background(), "TA-v0.5.0-"+wrongPlatform+extension, bytes.NewReader(archive), localChecksums(archiveName, archive)); !errors.Is(err, ErrPackagePlatform) {
		t.Fatalf("wrong platform: %v", err)
	}
	delete(packageFiles, updaterExecutableName())
	missingUpdater := localTestArchive(t, packageFiles, extension)
	if _, err := service.InstallLocal(context.Background(), archiveName, bytes.NewReader(missingUpdater), localChecksums(archiveName, missingUpdater)); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("missing updater: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(updateDataDir(installDir), "local-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary upload files leaked: %v, %v", entries, err)
	}
	var relaunched string
	if err := RunUpdater(context.Background(), manifestPath, UpdaterOptions{
		ProcessAlive: func(int) bool { return false },
		StartProcess: func(executable string, args, env []string) error { relaunched = executable; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if relaunched != service.executablePath {
		t.Fatalf("relaunch = %s", relaunched)
	}
	backup, err := os.ReadFile(filepath.Join(result.BackupPath, exeName))
	if err != nil || string(backup) != "current" {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	installed, err := os.ReadFile(service.executablePath)
	if err != nil || string(installed) != "new executable" {
		t.Fatalf("installed executable = %q, %v", installed, err)
	}
}

func TestLocalUpdateRejectsConcurrentOperations(t *testing.T) {
	updateOperation.Lock()
	defer updateOperation.Unlock()
	service := NewService()
	if _, err := service.InstallLocal(context.Background(), "package.zip", strings.NewReader(""), nil); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if _, err := service.Install(context.Background()); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background()); !errors.Is(err, ErrUpdateBusy) {
		t.Fatal(err)
	}
}

func localTestArchive(t *testing.T, files map[string]string, extension string) []byte {
	t.Helper()
	if extension == ".tar.gz" {
		return testReleaseArchive(t, "TA", files)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, content := range files {
		header := &zip.FileHeader{Name: "TA/" + name, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func localChecksums(name string, archive []byte) io.Reader {
	sum := sha256.Sum256(archive)
	return strings.NewReader(fmt.Sprintf("%x  %s\n", sum, name))
}

func TestInstallLocalRequiresValidChecksum(t *testing.T) {
	extension := ".tar.gz"
	exe := "TA"
	if runtime.GOOS == "windows" {
		extension = ".zip"
		exe = "TA.exe"
	}
	name := "TA-v0.5.0-" + platformKey(runtime.GOOS, runtime.GOARCH) + extension
	s := &Service{currentVersion: "0.4.0", executablePath: filepath.Join(t.TempDir(), exe)}
	for _, check := range []io.Reader{nil, strings.NewReader(""), strings.NewReader(strings.Repeat("0", 64) + "  " + name + "\n")} {
		_, err := s.InstallLocal(context.Background(), name, strings.NewReader("invalid"), check)
		if !errors.Is(err, ErrPackageChecksum) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestStatusDistinguishesMissingAndBrokenPending(t *testing.T) {
	root := t.TempDir()
	s := &Service{executablePath: filepath.Join(root, "TA.exe"), currentVersion: "1.0.0"}
	status, err := s.Status(context.Background())
	if err != nil || status.Staged || status.ApplyReady || status.CurrentVersion != "1.0.0" {
		t.Fatalf("empty status: %+v, %v", status, err)
	}
	updateDir := updateDataDir(root)
	if err := writePendingManifestRef(updateDir, filepath.Join(updateDir, "missing", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	status, err = s.Status(context.Background())
	if err == nil || status.ApplyReady {
		t.Fatalf("broken status: %+v, %v", status, err)
	}
}
