package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const applyRestartDelay = 500 * time.Millisecond

type ApplyInvocation struct {
	Executable string
	Args       []string
	Env        []string
}

// ApplyScheduler starts TA-updater after the HTTP response has had time to
// flush, then exits the current TA process.
type ApplyScheduler struct {
	Delay        time.Duration
	ManifestPath string
	Manifest     ApplyManifest
	Start        func(ApplyInvocation) error
	Exit         func(int)
	Sleep        func(time.Duration)
	Logger       *slog.Logger
}

func (s ApplyScheduler) Schedule(ctx context.Context) error {
	manifest := s.Manifest
	if manifest.UpdaterExecutable == "" {
		return fmt.Errorf("update manifest is missing updater_executable")
	}
	if _, err := os.Stat(manifest.UpdaterExecutable); err != nil {
		return fmt.Errorf("start updater: %w", err)
	}
	if s.ManifestPath == "" {
		return fmt.Errorf("update manifest path is required")
	}
	if _, err := os.Stat(s.ManifestPath); err != nil {
		return fmt.Errorf("update manifest is missing: %w", err)
	}
	delay := s.Delay
	if delay <= 0 {
		delay = applyRestartDelay
	}
	start := s.Start
	if start == nil {
		start = startApplyProcess
	}
	exit := s.Exit
	if exit == nil {
		exit = os.Exit
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	invocation := ApplyInvocation{
		Executable: manifest.UpdaterExecutable,
		Args:       []string{manifest.UpdaterExecutable, "--manifest", s.ManifestPath},
		Env:        append([]string(nil), os.Environ()...),
	}
	// Starting synchronously lets the caller release its admission fence on
	// failure instead of reporting a restart that never began.
	if err := start(invocation); err != nil {
		return fmt.Errorf("start updater: %w", err)
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(ctx, "updater_schedule_panic_recovered", "error", recovered)
			}
		}()
		logger.InfoContext(ctx, "updater_scheduled", "executable", invocation.Executable, "manifest", s.ManifestPath, "delay", delay)
		sleep(delay)
		exit(0)
	}()
	return nil
}

// pendingManifest accepts only packages staged by the current validator and
// bound to this installation. Older unattributed staged packages are ignored.
func (s *Service) pendingManifest() (string, ApplyManifest, error) {
	if s.executablePath == "" {
		return "", ApplyManifest{}, fmt.Errorf("cannot locate current executable")
	}
	installDir := filepath.Dir(s.executablePath)
	updateDir := updateDataDir(installDir)
	manifestPath, err := readPendingManifestRef(updateDir)
	if err != nil {
		return "", ApplyManifest{}, err
	}
	rel, err := filepath.Rel(updateDir, manifestPath)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ApplyManifest{}, fmt.Errorf("invalid pending manifest location")
	}
	m, err := readManifest(manifestPath)
	if err != nil {
		return "", ApplyManifest{}, err
	}
	pendingDir := filepath.Dir(manifestPath)
	if filepath.Base(manifestPath) != manifestFileName || !strings.HasPrefix(filepath.Base(pendingDir), "pending-") || filepath.Clean(filepath.Dir(pendingDir)) != filepath.Clean(updateDir) {
		return "", ApplyManifest{}, fmt.Errorf("invalid pending directory")
	}
	expectedRoot := filepath.Join(pendingDir, releasePackageRootName)
	if filepath.Clean(m.LogPath) != filepath.Join(pendingDir, applyLogFileName) {
		return "", ApplyManifest{}, fmt.Errorf("invalid update log path")
	}
	backupRel, backupErr := filepath.Rel(updateDir, m.BackupDir)
	if backupErr != nil || filepath.Dir(backupRel) != "." || !strings.HasPrefix(backupRel, "backup-") {
		return "", ApplyManifest{}, fmt.Errorf("invalid backup location")
	}
	if m.SchemaVersion != 1 || !strings.EqualFold(filepath.Clean(m.InstallDir), filepath.Clean(installDir)) || !strings.EqualFold(filepath.Clean(m.SourceDir), filepath.Clean(expectedRoot)) || !strings.EqualFold(filepath.Clean(m.TargetExecutable), filepath.Clean(s.executablePath)) || !strings.EqualFold(filepath.Clean(m.UpdaterExecutable), filepath.Join(expectedRoot, updaterExecutableName())) || !stableTagPattern.MatchString(m.Version) {
		return "", ApplyManifest{}, fmt.Errorf("invalid pending update")
	}
	if isDevVersion(s.currentVersion) || compareVersions(s.currentVersion, m.Version) >= 0 {
		return "", ApplyManifest{}, ErrPackageVersion
	}
	if err := validateReleasePackage(m.SourceDir, filepath.Base(s.executablePath), updaterExecutableName()); err != nil {
		return "", ApplyManifest{}, err
	}
	digest, err := stagedPackageDigest(m.SourceDir)
	if err != nil || len(m.PackageDigest) != 64 || digest != m.PackageDigest {
		return "", ApplyManifest{}, fmt.Errorf("staged package checksum mismatch")
	}
	return manifestPath, m, nil
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	_ = ctx
	result := Status{CurrentVersion: s.currentVersion, ChannelConfigured: ChannelConfigured(s.repository)}
	ref := filepath.Join(updateDataDir(filepath.Dir(s.executablePath)), pendingManifestRefName)
	if _, err := os.Stat(ref); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	_, m, err := s.pendingManifest()
	if errors.Is(err, ErrPackageVersion) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Staged = true
	result.ApplyReady = true
	result.InstalledVersion = m.Version
	return result, nil
}

func (s *Service) Apply(ctx context.Context) (ApplyResult, error) {
	if !updateOperation.TryLock() {
		return ApplyResult{}, ErrUpdateBusy
	}
	defer updateOperation.Unlock()
	manifestPath, manifest, err := s.pendingManifest()
	if err != nil {
		return ApplyResult{}, err
	}
	// Staged packages survive process restarts; bind the updater to this process.
	manifest.CurrentPID = os.Getpid()
	manifest.RelaunchArgs = relaunchArgs(os.Args, s.executablePath)
	if err := writeManifest(manifestPath, manifest); err != nil {
		return ApplyResult{}, err
	}
	if err := (ApplyScheduler{ManifestPath: manifestPath, Manifest: manifest}).Schedule(ctx); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Status: "restarting", Version: manifest.Version, LogPath: manifest.LogPath}, nil
}

func startApplyProcess(invocation ApplyInvocation) error {
	args := []string(nil)
	if len(invocation.Args) > 1 {
		args = invocation.Args[1:]
	}
	cmd := exec.Command(invocation.Executable, args...)
	cmd.Env = invocation.Env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
