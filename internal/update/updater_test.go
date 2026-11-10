package update

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ta/internal/hostruntime"
)

func TestRunUpdaterAppliesStagedPackageAndRelaunches(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "nova")
	backupDir := filepath.Join(installDir, ".denova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	stagedUpdater := filepath.Join(sourceDir, updaterName)
	writeUpdateTestPackage(t, installDir, updateTestPackageContents{
		Executable: "old executable", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old ripgrep", License: "old license",
	})
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{
		Executable: "new executable", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license",
	})
	manifest := ApplyManifest{
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		CurrentPID:        12345,
		TargetExecutable:  targetExe,
		UpdaterExecutable: stagedUpdater,
		RelaunchArgs:      []string{targetExe, "--port", "8080", "--no-open"},
		Version:           "0.2.0",
		LogPath:           filepath.Join(installDir, ".denova-updates", "apply.log"),
	}
	manifestPath := filepath.Join(t.TempDir(), manifestFileName)
	if err := writeManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	var launched string
	err := RunUpdater(context.Background(), manifestPath, UpdaterOptions{
		ProcessAlive: func(int) bool { return false },
		StartProcess: func(executable string, args []string, env []string) error {
			launched = executable
			if len(args) == 0 || args[len(args)-1] != "--no-open" {
				t.Fatalf("relaunch args should force --no-open: %#v", args)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RunUpdater failed: %v", err)
	}
	assertFileContent(t, targetExe, "new executable")
	assertFileContent(t, filepath.Join(installDir, updaterName), "new updater")
	assertFileContent(t, filepath.Join(installDir, "web", "index.html"), "new web")
	assertFileContent(t, filepath.Join(installDir, "skills", "demo", "SKILL.md"), "new skill")
	assertFileContent(t, filepath.Join(installDir, "tools", hostruntime.RipgrepExecutableName()), "new ripgrep")
	assertFileContent(t, filepath.Join(installDir, "licenses", "ripgrep", "LICENSE-MIT"), "new license MIT")
	assertFileContent(t, filepath.Join(backupDir, "nova"), "old executable")
	assertFileContent(t, filepath.Join(backupDir, updaterName), "old updater")
	assertFileContent(t, filepath.Join(backupDir, "tools", hostruntime.RipgrepExecutableName()), "old ripgrep")
	if launched != targetExe {
		t.Fatalf("launched executable = %q, want %q", launched, targetExe)
	}
	if _, err := os.Stat(manifest.LogPath); err != nil {
		t.Fatalf("apply log missing: %v", err)
	}
}

func TestRollbackUpdateRestoresBackups(t *testing.T) {
	installDir := t.TempDir()
	sourceDir := filepath.Join(t.TempDir(), "nova")
	backupDir := filepath.Join(installDir, ".denova-updates", "backup-test")
	targetExe := filepath.Join(installDir, "nova")
	updaterName := updaterExecutableName()
	writeUpdateTestPackage(t, installDir, updateTestPackageContents{
		Executable: "new executable", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new ripgrep", License: "new license",
	})
	writeUpdateTestPackage(t, sourceDir, updateTestPackageContents{
		Executable: "staged executable", Updater: "staged updater", Web: "staged web", Skill: "staged skill", Ripgrep: "staged ripgrep", License: "staged license",
	})
	writeUpdateTestPackage(t, backupDir, updateTestPackageContents{
		Executable: "old executable", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old ripgrep", License: "old license",
	})
	manifest := ApplyManifest{
		SourceDir:         sourceDir,
		InstallDir:        installDir,
		BackupDir:         backupDir,
		TargetExecutable:  targetExe,
		UpdaterExecutable: filepath.Join(sourceDir, updaterName),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := rollbackUpdate(context.Background(), manifest, logger); err != nil {
		t.Fatalf("rollbackUpdate failed: %v", err)
	}
	assertFileContent(t, targetExe, "old executable")
	assertFileContent(t, filepath.Join(installDir, updaterName), "old updater")
	assertFileContent(t, filepath.Join(installDir, "web", "index.html"), "old web")
	assertFileContent(t, filepath.Join(installDir, "skills", "demo", "SKILL.md"), "old skill")
	assertFileContent(t, filepath.Join(installDir, "tools", hostruntime.RipgrepExecutableName()), "old ripgrep")
	assertFileContent(t, filepath.Join(installDir, "licenses", "ripgrep", "UNLICENSE"), "old license Unlicense")
}

type updateTestPackageContents struct {
	Executable string
	Updater    string
	Web        string
	Skill      string
	Ripgrep    string
	License    string
}

func writeUpdateTestPackage(t *testing.T, root string, content updateTestPackageContents) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "licenses", "ripgrep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nova"), []byte(content.Executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, updaterExecutableName()), []byte(content.Updater), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "index.html"), []byte(content.Web), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "demo", "SKILL.md"), []byte(content.Skill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", hostruntime.RipgrepExecutableName()), []byte(content.Ripgrep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "licenses", "ripgrep", "LICENSE-MIT"), []byte(content.License+" MIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "licenses", "ripgrep", "UNLICENSE"), []byte(content.License+" Unlicense"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func TestFailedReplacementRemovesOnlyTouchedNewTargets(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	fresh := filepath.Join(root, "fresh")
	untouched := filepath.Join(root, "untouched")
	if err := os.WriteFile(old, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	entries := []replaceEntry{
		{Target: old, Backup: filepath.Join(root, "backup", "old")},
		{Target: fresh, Backup: filepath.Join(root, "backup", "fresh")},
		{Target: untouched, Backup: filepath.Join(root, "backup", "untouched")},
	}
	count := 0
	err := replaceUpdateEntries(context.Background(), entries, slog.New(slog.NewTextHandler(io.Discard, nil)), func(entry replaceEntry) error {
		count++
		if err := os.WriteFile(entry.Target, []byte("new"), 0600); err != nil {
			return err
		}
		if count == 2 {
			return errors.New("injected failure")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	assertFileContent(t, old, "original")
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatalf("new target survived: %v", err)
	}
	if _, err := os.Stat(untouched); !os.IsNotExist(err) {
		t.Fatalf("untouched target changed: %v", err)
	}
}

func TestUpdatePreservesUserData(t *testing.T) {
	install := t.TempDir()
	staged := t.TempDir()
	old := updateTestPackageContents{Executable: "old", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old rg", License: "old license"}
	next := updateTestPackageContents{Executable: "new", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new rg", License: "new license"}
	writeUpdateTestPackage(t, install, old)
	writeUpdateTestPackage(t, staged, next)
	names := []string{"config.toml", ".denova/stories/story.jsonl", ".denova/stories/branch.jsonl", "user-media/portrait.png", "worlds/external-pack.json"}
	for _, name := range names {
		path := filepath.Join(install, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("user:"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := ApplyManifest{SourceDir: staged, InstallDir: install, BackupDir: filepath.Join(install, ".denova-updates", "backup"), TargetExecutable: filepath.Join(install, "nova"), UpdaterExecutable: filepath.Join(staged, updaterExecutableName())}
	if err := applyStagedUpdate(context.Background(), m, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		assertFileContent(t, filepath.Join(install, filepath.FromSlash(name)), "user:"+name)
	}
}

func TestRunUpdaterRollsBackWhenRelaunchFails(t *testing.T) {
	install := t.TempDir()
	staged := t.TempDir()
	backup := filepath.Join(install, ".denova-updates", "backup")
	old := updateTestPackageContents{Executable: "old", Updater: "old updater", Web: "old web", Skill: "old skill", Ripgrep: "old rg", License: "old license"}
	next := updateTestPackageContents{Executable: "new", Updater: "new updater", Web: "new web", Skill: "new skill", Ripgrep: "new rg", License: "new license"}
	writeUpdateTestPackage(t, install, old)
	writeUpdateTestPackage(t, staged, next)
	m := ApplyManifest{SourceDir: staged, InstallDir: install, BackupDir: backup, TargetExecutable: filepath.Join(install, "nova"), UpdaterExecutable: filepath.Join(staged, updaterExecutableName())}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := writeManifest(path, m); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	err := RunUpdater(context.Background(), path, UpdaterOptions{ProcessAlive: func(int) bool { return false }, StartProcess: func(string, []string, []string) error {
		attempts++
		if attempts == 1 {
			return errors.New("injected restart failure")
		}
		return nil
	}, LogOutput: io.Discard})
	if err == nil || attempts != 2 {
		t.Fatalf("result %v attempts %d", err, attempts)
	}
	assertFileContent(t, filepath.Join(install, "nova"), "old")
	assertFileContent(t, filepath.Join(install, "web/index.html"), "old web")
}

func TestMissingSourceCopyFails(t *testing.T) {
	root := t.TempDir()
	if err := copyFile(filepath.Join(root, "missing"), filepath.Join(root, "target"), 0600); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing source error = %v", err)
	}
}
