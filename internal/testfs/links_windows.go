package testfs

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func symlinkPrivilegeUnavailable(err error) bool {
	return errors.Is(err, syscall.ERROR_PRIVILEGE_NOT_HELD)
}

// DirectoryJunction exercises Windows mount-point reparse links without enabling system privileges.
// Both paths must belong to the caller's disposable test directories.
func DirectoryJunction(t *testing.T, target, link string) {
	t.Helper()
	target, err := filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	link, err = filepath.Abs(link)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil || !targetInfo.IsDir() {
		t.Fatalf("junction target is not a test directory: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("junction path must not exist: %v", err)
	}
	if err := os.Mkdir(link, 0o700); err != nil {
		t.Fatal(err)
	}
	removeLinkOnCleanup(t, link)
	linkName, err := windows.UTF16PtrFromString(link)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(linkName, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	// Mount-point reparse buffer: header, four uint16 name fields, UTF-16 target.
	// The empty print name is valid; the substitute name uses the NT namespace.
	name, err := windows.UTF16FromString(`\??\` + target)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 18+len(name)*2)
	if len(buffer) > windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE {
		t.Fatal("test junction target is too long")
	}
	binary.LittleEndian.PutUint32(buffer[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:6], uint16(len(buffer)-8))
	binary.LittleEndian.PutUint16(buffer[10:12], uint16((len(name)-1)*2))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(len(name)*2))
	for i, value := range name {
		binary.LittleEndian.PutUint16(buffer[16+i*2:], value)
	}
	var returned uint32
	if err := windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &buffer[0], uint32(len(buffer)), nil, 0, &returned, nil); err != nil {
		t.Fatalf("create directory junction: %v", err)
	}
	actual, err := os.Stat(link)
	if err != nil || !os.SameFile(targetInfo, actual) {
		t.Fatalf("junction did not resolve to its target: %v", err)
	}
	entry, err := os.Lstat(link)
	if err != nil || entry.IsDir() || entry.Mode().IsRegular() {
		t.Fatalf("fixture is not a directory reparse link: %v", err)
	}
}
