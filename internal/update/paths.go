package update

import (
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// Update staging directories keep their historical names: they hold
	// already-staged packages for installations that predate the rename, and
	// renaming them would orphan a pending update instead of applying it.
	updateDataDirName       = ".denova-updates"
	legacyUpdateDataDirName = ".nova-updates"
	// The release package root is the directory inside the archive, which is
	// also the staged directory name. It follows the shipped product name.
	releasePackageRootName = "TA"
)

func updaterExecutableName() string {
	if runtime.GOOS == "windows" {
		return "TA-updater.exe"
	}
	return "TA-updater"
}

func relaunchArgs(args []string, executable string) []string {
	next := []string{executable}
	if len(args) > 1 {
		for _, arg := range args[1:] {
			if isNoOpenArg(arg) {
				continue
			}
			next = append(next, arg)
		}
	}
	return append(next, "--no-open")
}

func isNoOpenArg(arg string) bool {
	return arg == "--no-open" || arg == "-no-open" ||
		strings.HasPrefix(arg, "--no-open=") || strings.HasPrefix(arg, "-no-open=")
}

func installUpdaterTarget(installDir, stagedUpdater string) string {
	return filepath.Join(installDir, filepath.Base(stagedUpdater))
}

func updateDataDir(installDir string) string {
	return filepath.Join(installDir, updateDataDirName)
}

func legacyUpdateDataDir(installDir string) string {
	return filepath.Join(installDir, legacyUpdateDataDirName)
}
