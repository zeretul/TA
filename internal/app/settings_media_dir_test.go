package app

import (
	"path/filepath"
	"testing"

	"ta/config"
	settingsapp "ta/internal/app/settings"
)

// The shipped media directory is resolved from the executable's own location at
// startup and never written to disk, so a settings refresh that rebuilds the
// configuration from the persisted layers must carry it across. Losing it makes
// every imported avatar unreadable for the rest of the process, and the image
// tool then generates from the prompt alone without saying so.
func TestSettingsRuntimeKeepsTheShippedMediaDirectory(t *testing.T) {
	dataDir := t.TempDir()
	mediaDir := filepath.Join(dataDir, "media")
	application := &App{cfg: &config.Config{
		DenovaDir: dataDir, NovaDir: dataDir, MediaDir: mediaDir,
	}}

	runtime, err := settingsHost{app: application}.SettingsRuntime(settingsapp.Global())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Config.MediaDir != mediaDir {
		t.Fatalf("media dir after settings refresh = %q, want %q", runtime.Config.MediaDir, mediaDir)
	}
}
