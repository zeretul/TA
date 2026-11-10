package media

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// This measures actual process read bytes, not elapsed time or a production
// test hook. Sparse fixtures avoid copying large media. Format decoding is not
// involved: Open's responsibility here is the stream and restriction boundary.
func TestGlobalMediaNeverInstalledKeepsNoScanStream(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("read-byte accounting requires Linux /proc/self/io; HTTP compatibility is tested separately")
	}
	readBytes := func(t *testing.T) uint64 {
		t.Helper()
		raw, err := os.ReadFile("/proc/self/io")
		if err != nil {
			t.Skipf("process read-byte accounting unavailable: %v", err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "rchar:" {
				n, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				return n
			}
		}
		t.Fatal("process read-byte accounting has no rchar field")
		return 0
	}
	for _, mode := range []string{"never-installed", "installed"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			const name = "legacy-large.mp4"
			const size int64 = 64 << 20
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := f.Truncate(size); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			writeJSON := func(name string, value any) {
				t.Helper()
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeJSON("media.json", map[string]any{"schema": "ta.media/v2", "entries": []map[string]any{{"source_key": "legacy", "purpose": "clip", "reviewed": true, "file": name, "bytes": size}}})
			if mode == "installed" {
				writeJSON("story-assets.json", StoryManifest{Schema: "ta.story-media/v1"})
			}
			library := NewLibrary(dir)
			if len(library.Entries()) != 1 {
				t.Fatal("fixture was not indexed")
			}
			before := readBytes(t)
			handle, _, err := library.Open(name)
			after := readBytes(t)
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Close()
			offset, err := handle.Seek(0, io.SeekCurrent)
			if err != nil || offset != 0 {
				t.Fatalf("stream must start at byte zero: offset=%d err=%v", offset, err)
			}
			read := after - before
			if mode == "never-installed" && read >= uint64(size/4) {
				t.Fatalf("ordinary Open scanned large media: read=%d size=%d", read, size)
			}
			if mode == "installed" && read < uint64(size) {
				t.Fatalf("installed restriction skipped actual byte verification: read=%d size=%d", read, size)
			}
		})
	}
}
