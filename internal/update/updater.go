package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	defaultWaitTimeout  = 60 * time.Second
	defaultWaitInterval = 500 * time.Millisecond
)

type UpdaterOptions struct {
	WaitTimeout  time.Duration
	WaitInterval time.Duration
	ProcessAlive func(int) bool
	StartProcess func(string, []string, []string) error
	Sleep        func(time.Duration)
	LogOutput    io.Writer
}

type replaceEntry struct {
	Source   string
	Target   string
	Backup   string
	Dir      bool
	Optional bool
}

// RunUpdater applies a staged update and relaunches TA. It is the entrypoint
// used by cmd/ta-updater.
func RunUpdater(ctx context.Context, manifestPath string, options UpdaterOptions) error {
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	logger, closeLog, err := updaterLogger(manifest.LogPath, options.LogOutput)
	if err != nil {
		return err
	}
	defer closeLog()
	logger.InfoContext(ctx, "updater_started", "manifest", manifestPath, "version", manifest.Version, "install_dir", manifest.InstallDir, "source_dir", manifest.SourceDir)
	if err := waitForProcessExit(ctx, manifest.CurrentPID, options, logger); err != nil {
		logger.ErrorContext(ctx, "updater_wait_failed", "error", err)
		return err
	}
	if err := applyStagedUpdateAndFinish(ctx, manifest, logger, func() error { return startRelaunch(ctx, manifest, options, logger) }); err != nil {
		logger.ErrorContext(ctx, "updater_apply_failed", "error", err)
		startErr := startRelaunch(ctx, manifest, options, logger)
		if startErr != nil {
			logger.ErrorContext(ctx, "updater_failure_restart_failed", "error", startErr)
		}
		return err
	}
	logger.InfoContext(ctx, "updater_apply_completed")
	return nil
}

func updaterLogger(logPath string, extra io.Writer) (*slog.Logger, func(), error) {
	writers := []io.Writer{}
	var closeLog func()
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return nil, func() {}, err
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, func() {}, err
		}
		writers = append(writers, f)
		closeLog = func() { _ = f.Close() }
	} else {
		closeLog = func() {}
	}
	if extra != nil {
		writers = append(writers, extra)
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}
	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo})
	return slog.New(handler).With("component", "TA-updater"), closeLog, nil
}

func waitForProcessExit(ctx context.Context, pid int, options UpdaterOptions, logger *slog.Logger) error {
	alive := options.ProcessAlive
	if alive == nil {
		alive = processRunning
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	timeout := options.WaitTimeout
	if timeout <= 0 {
		timeout = defaultWaitTimeout
	}
	interval := options.WaitInterval
	if interval <= 0 {
		interval = defaultWaitInterval
	}
	deadline := time.Now().Add(timeout)
	for alive(pid) {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for TA to exit pid=%d", pid)
		}
		logger.InfoContext(ctx, "updater_waiting_for_process_exit", "pid", pid)
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sleep(interval)
		}
	}
	return nil
}

func applyStagedUpdate(ctx context.Context, manifest ApplyManifest, logger *slog.Logger) error {
	return applyStagedUpdateAndFinish(ctx, manifest, logger, nil)
}
func applyStagedUpdateAndFinish(ctx context.Context, manifest ApplyManifest, logger *slog.Logger, finish func() error) error {
	if err := validateApplyManifest(manifest); err != nil {
		return err
	}
	if err := os.MkdirAll(manifest.BackupDir, 0o755); err != nil {
		return fmt.Errorf("create update backup directory: %w", err)
	}
	entries := updateEntries(manifest)
	active := make([]replaceEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Optional {
			if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
				continue
			}
		}
		active = append(active, entry)
	}
	return replaceUpdateEntriesAndFinish(ctx, active, logger, copyEntry, finish)
}

// replaceUpdateEntries tracks only paths touched by this transaction, so a
// failed copy removes new partial targets without deleting unrelated files.
func replaceUpdateEntries(ctx context.Context, entries []replaceEntry, logger *slog.Logger, copyFn func(replaceEntry) error) error {
	return replaceUpdateEntriesAndFinish(ctx, entries, logger, copyFn, nil)
}
func replaceUpdateEntriesAndFinish(ctx context.Context, entries []replaceEntry, logger *slog.Logger, copyFn func(replaceEntry) error, finish func() error) (resultErr error) {
	backed := make([]replaceEntry, 0, len(entries))
	touched := make([]replaceEntry, 0, len(entries))
	defer func() {
		if resultErr == nil {
			return
		}
		var rollbackErr error
		for i := len(touched) - 1; i >= 0; i-- {
			if err := os.RemoveAll(touched[i].Target); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		for i := len(backed) - 1; i >= 0; i-- {
			entry := backed[i]
			if err := os.Rename(entry.Backup, entry.Target); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		if rollbackErr != nil {
			logger.ErrorContext(ctx, "updater_rollback_failed", "error", rollbackErr)
			resultErr = errors.Join(resultErr, rollbackErr)
		}
	}()
	for _, entry := range entries {
		_, statErr := os.Stat(entry.Target)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if err := backupEntry(entry); err != nil {
			return err
		}
		if statErr == nil {
			backed = append(backed, entry)
		}
	}
	for _, entry := range entries {
		logger.InfoContext(ctx, "updater_replacing_entry", "target", entry.Target, "source", entry.Source)
		touched = append(touched, entry)
		if err := copyFn(entry); err != nil {
			return err
		}
	}
	if finish != nil {
		return finish()
	}
	return nil
}

func validateApplyManifest(manifest ApplyManifest) error {
	if manifest.SourceDir == "" || manifest.InstallDir == "" || manifest.BackupDir == "" || manifest.TargetExecutable == "" || manifest.UpdaterExecutable == "" {
		return fmt.Errorf("update manifest fields are incomplete")
	}
	if err := validateReleasePackage(manifest.SourceDir, filepath.Base(manifest.TargetExecutable), filepath.Base(manifest.UpdaterExecutable)); err != nil {
		return err
	}
	if manifest.SchemaVersion == 1 {
		digest, err := stagedPackageDigest(manifest.SourceDir)
		if err != nil || len(manifest.PackageDigest) != 64 || digest != manifest.PackageDigest {
			return fmt.Errorf("staged package checksum mismatch")
		}
	}
	return nil
}

func updateEntries(manifest ApplyManifest) []replaceEntry {
	installDir := manifest.InstallDir
	return []replaceEntry{
		fileEntry(manifest.SourceDir, manifest.TargetExecutable, manifest.BackupDir, filepath.Base(manifest.TargetExecutable), false),
		fileEntry(manifest.SourceDir, installUpdaterTarget(installDir, manifest.UpdaterExecutable), manifest.BackupDir, filepath.Base(manifest.UpdaterExecutable), false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "web", false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "skills", false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "tools", false),
		dirEntry(manifest.SourceDir, installDir, manifest.BackupDir, "licenses", false),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "README.md"), manifest.BackupDir, "README.md", true),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "CHANGELOG.md"), manifest.BackupDir, "CHANGELOG.md", true),
		fileEntry(manifest.SourceDir, filepath.Join(installDir, "LICENSE"), manifest.BackupDir, "LICENSE", true),
	}
}

func fileEntry(sourceDir, target, backupDir, name string, optional bool) replaceEntry {
	return replaceEntry{
		Source:   filepath.Join(sourceDir, name),
		Target:   target,
		Backup:   filepath.Join(backupDir, name),
		Optional: optional,
	}
}

func dirEntry(sourceDir, installDir, backupDir, name string, optional bool) replaceEntry {
	return replaceEntry{
		Source:   filepath.Join(sourceDir, name),
		Target:   filepath.Join(installDir, name),
		Backup:   filepath.Join(backupDir, name),
		Dir:      true,
		Optional: optional,
	}
}

func backupEntry(entry replaceEntry) error {
	if _, err := os.Stat(entry.Target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(entry.Backup), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(entry.Backup); err != nil {
		return err
	}
	if err := os.Rename(entry.Target, entry.Backup); err != nil {
		return fmt.Errorf("back up update target target=%s err=%w", entry.Target, err)
	}
	return nil
}

func copyEntry(entry replaceEntry) error {
	if entry.Dir {
		if err := copyDir(entry.Source, entry.Target); err != nil {
			return fmt.Errorf("replace update directory target=%s err=%w", entry.Target, err)
		}
		return nil
	}
	info, err := os.Stat(entry.Source)
	if err != nil {
		if entry.Optional && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("update package is missing file %s: %w", filepath.Base(entry.Source), err)
	}
	if info.IsDir() {
		return fmt.Errorf("update package file is a directory: %s", entry.Source)
	}
	if err := copyFile(entry.Source, entry.Target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("replace update file target=%s err=%w", entry.Target, err)
	}
	return nil
}

func rollbackUpdate(ctx context.Context, manifest ApplyManifest, logger *slog.Logger) error {
	var firstErr error
	for _, entry := range updateEntries(manifest) {
		if _, err := os.Stat(entry.Backup); err != nil {
			continue
		}
		logger.InfoContext(ctx, "updater_rolling_back_entry", "target", entry.Target, "backup", entry.Backup)
		if err := os.RemoveAll(entry.Target); err != nil && firstErr == nil {
			firstErr = err
			continue
		}
		if err := os.Rename(entry.Backup, entry.Target); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func startRelaunch(ctx context.Context, manifest ApplyManifest, options UpdaterOptions, logger *slog.Logger) error {
	start := options.StartProcess
	if start == nil {
		start = startProcess
	}
	args := manifest.RelaunchArgs
	if len(args) == 0 {
		args = []string{manifest.TargetExecutable, "--no-open"}
	}
	logger.InfoContext(ctx, "updater_starting_ta", "executable", manifest.TargetExecutable, "args", len(args))
	return start(manifest.TargetExecutable, args, os.Environ())
}

func startProcess(executable string, args []string, env []string) error {
	cmdArgs := []string(nil)
	if len(args) > 1 {
		cmdArgs = args[1:]
	}
	cmd := exec.Command(executable, cmdArgs...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
