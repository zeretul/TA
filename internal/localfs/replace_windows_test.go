package localfs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func holdReplaceDestination(t *testing.T, path string) windows.Handle {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

func TestReplaceFileRetriesReleasedWindowsReader(t *testing.T) {
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "candidate"), filepath.Join(dir, "current")
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	handle := holdReplaceDestination(t, destination)
	if err := os.Rename(source, destination); err == nil || !retryableReplaceError(err) {
		windows.CloseHandle(handle)
		t.Fatalf("expected a transient locked replacement error, got %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Errorf("reader release panicked: %v", recovered)
			}
		}()
		time.Sleep(80 * time.Millisecond)
		if err := windows.CloseHandle(handle); err != nil {
			t.Errorf("close reader: %v", err)
		}
	}()
	err := ReplaceFile(source, destination)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "new" {
		t.Fatalf("wrong replacement: %q, %v", data, err)
	}
}

func TestReplaceFileRetainsBothFilesWhenWindowsReaderStaysLocked(t *testing.T) {
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "candidate"), filepath.Join(dir, "current")
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	handle := holdReplaceDestination(t, destination)
	defer windows.CloseHandle(handle)
	if err := ReplaceFile(source, destination); err == nil {
		t.Fatal("permanent reader lock unexpectedly replaced destination")
	}
	for file, want := range map[string]string{source: "new", destination: "old"} {
		if data, err := os.ReadFile(file); err != nil || string(data) != want {
			t.Fatalf("failed replacement changed %s: %q, %v", file, data, err)
		}
	}
}
