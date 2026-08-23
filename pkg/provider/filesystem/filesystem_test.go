package filesystem

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestFileServiceReadWriteAndRename(t *testing.T) {
	root := t.TempDir()
	service, err := NewFileService(root)
	if err != nil {
		t.Fatal(err)
	}
	response, _ := service.Handle(protocol.Request{Operation: "create", Path: "source.txt", Mode: 0o600}, nil)
	if response.ErrorCode != 0 {
		t.Fatalf("create failed: %s", response.Error)
	}
	if response.Attr == nil || response.Attr.Size != 0 {
		t.Fatalf("create omitted attributes: %#v", response.Attr)
	}
	modTime := time.Unix(1_700_000_000, 123_456_000)
	response, _ = service.Handle(protocol.Request{
		Operation: "write", Path: "source.txt", SetModTime: true, ModTimeNS: modTime.UnixNano(), Sync: true,
	}, []byte("hello"))
	if response.ErrorCode != 0 {
		t.Fatalf("write failed: %s", response.Error)
	}
	if response.Attr == nil || response.Attr.Size != 5 {
		t.Fatalf("write omitted attributes: %#v", response.Attr)
	}
	if response.Attr.ModTimeNS != modTime.UnixNano() {
		t.Fatalf("write did not apply modification time: %d", response.Attr.ModTimeNS)
	}
	response, data := service.Handle(protocol.Request{Operation: "read", Path: "source.txt", Size: 16}, nil)
	if response.ErrorCode != 0 || string(data) != "hello" {
		t.Fatalf("read mismatch: %q, error %s", data, response.Error)
	}
	response, _ = service.Handle(protocol.Request{Operation: "rename", Path: "source.txt", NewPath: "target.txt"}, nil)
	if response.ErrorCode != 0 {
		t.Fatalf("rename failed: %s", response.Error)
	}
	if response.Attr == nil || response.Attr.Size != 5 {
		t.Fatalf("rename omitted attributes: %#v", response.Attr)
	}
	if _, err := os.Stat(filepath.Join(root, "target.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestFileServiceMetadataAndDirectoryOperations(t *testing.T) {
	root := t.TempDir()
	service, _ := NewFileService(root)
	operations := []protocol.Request{
		{Operation: "mkdir", Path: "nested", Mode: 0o755},
		{Operation: "create", Path: "nested/data.bin", Mode: 0o600},
		{Operation: "write", Path: "nested/data.bin", Offset: 0},
		{Operation: "write", Path: "nested/data.bin", Offset: 5},
		{Operation: "fsync", Path: "nested/data.bin"},
		{Operation: "truncate", Path: "nested/data.bin", Offset: 5},
		{Operation: "chmod", Path: "nested/data.bin", Mode: 0o400},
	}
	payloads := [][]byte{nil, nil, []byte("hello"), []byte("-world"), nil, nil, nil}
	for index, operation := range operations {
		response, _ := service.Handle(operation, payloads[index])
		if response.ErrorCode != 0 {
			t.Fatalf("operation %s failed: %#v", operation.Operation, response)
		}
	}
	response, entries := service.Handle(protocol.Request{Operation: "read", Path: "nested/data.bin", Size: 32}, nil)
	if response.ErrorCode != 0 || string(entries) != "hello" {
		t.Fatalf("unexpected truncated file %q, response %#v", entries, response)
	}
	response, _ = service.Handle(protocol.Request{Operation: "write", Path: "nested/data.bin"}, []byte("denied"))
	if os.Geteuid() != 0 && response.ErrorCode != int(syscall.EACCES) {
		t.Fatalf("read-only file write returned %#v", response)
	}
	_ = os.Chmod(filepath.Join(root, "nested", "data.bin"), 0o600)
	for _, operation := range []protocol.Request{
		{Operation: "remove", Path: "nested/data.bin"},
		{Operation: "remove", Path: "nested"},
	} {
		response, _ = service.Handle(operation, nil)
		if response.ErrorCode != 0 {
			t.Fatalf("remove failed: %#v", response)
		}
	}
}

func TestFileServiceNamesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	service, _ := NewFileService(root)
	name := "unicode space e\u0301.txt"
	response, _ := service.Handle(protocol.Request{Operation: "create", Path: name, Mode: 0o600}, nil)
	if response.ErrorCode != 0 {
		t.Fatal(response.Error)
	}
	response, _ = service.Handle(protocol.Request{Operation: "write", Path: name}, []byte("unicode"))
	if response.ErrorCode != 0 {
		t.Fatal(response.Error)
	}
	response, _ = service.Handle(protocol.Request{Operation: "symlink", Path: "working-link", Target: name}, nil)
	if response.ErrorCode != 0 {
		t.Fatal(response.Error)
	}
	response, data := service.Handle(protocol.Request{Operation: "read", Path: "working-link", Size: 32}, nil)
	if response.ErrorCode != 0 || string(data) != "unicode" {
		t.Fatalf("symlink read failed: %#v %q", response, data)
	}
	_, _ = service.Handle(protocol.Request{Operation: "symlink", Path: "broken-link", Target: "missing"}, nil)
	response, _ = service.Handle(protocol.Request{Operation: "read", Path: "broken-link", Size: 32}, nil)
	if response.ErrorCode != int(syscall.ENOENT) {
		t.Fatalf("broken symlink returned %#v", response)
	}
}

func TestFileServiceFiltersSpecialFiles(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "colnk-socket-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socketPath := filepath.Join(root, "test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.WriteFile(filepath.Join(root, "regular.txt"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular.txt", filepath.Join(root, "regular-link")); err != nil {
		t.Fatal(err)
	}
	service, _ := NewFileService(root)
	response, _ := service.Handle(protocol.Request{Operation: "stat", Path: "test.sock"}, nil)
	if response.ErrorCode != int(syscall.EPERM) {
		t.Fatalf("special file was exposed: %#v", response)
	}
	entries := readDirectoryEntries(t, service, "")
	foundRegular := false
	foundLink := false
	for _, entry := range entries {
		if entry.Name == "test.sock" {
			t.Fatal("special file appeared in directory listing")
		}
		if entry.Name == "regular.txt" {
			foundRegular = true
			if entry.Attr.Mode == 0 || entry.Attr.Size != 4 {
				t.Fatalf("directory entry omitted file attributes: %#v", entry)
			}
		}
		if entry.Name == "regular-link" {
			foundLink = true
			if entry.Attr.Mode&uint32(os.ModeSymlink) == 0 || entry.LinkTarget != "regular.txt" {
				t.Fatalf("directory entry omitted symlink data: %#v", entry)
			}
		}
	}
	if !foundRegular {
		t.Fatal("regular file was missing from directory listing")
	}
	if !foundLink {
		t.Fatal("symlink was missing from directory listing")
	}
	if err := os.Symlink("test.sock", filepath.Join(root, "socket-link")); err != nil {
		t.Fatal(err)
	}
	response, _ = service.Handle(protocol.Request{Operation: "write", Path: "socket-link"}, []byte("blocked"))
	if response.ErrorCode != int(syscall.EPERM) {
		t.Fatalf("write through special-file symlink returned %#v", response)
	}
	response, _ = service.Handle(protocol.Request{Operation: "remove", Path: "test.sock"}, nil)
	if response.ErrorCode != int(syscall.EPERM) {
		t.Fatalf("special file removal returned %#v", response)
	}
}

func readDirectoryEntries(t *testing.T, service *FileService, path string) []protocol.DirEntry {
	t.Helper()
	var entries []protocol.DirEntry
	if err := service.StreamDir(path, func(payload []byte, _ bool) error {
		if len(payload) == 0 {
			return nil
		}
		page, err := protocol.DecodeDirEntries(payload)
		if err != nil {
			return err
		}
		entries = append(entries, page...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestFileServiceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("fake-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	service, _ := NewFileService(root)
	response, _ := service.Handle(protocol.Request{Operation: "read", Path: "escape/secret", Size: 64}, nil)
	if response.ErrorCode != int(syscall.EPERM) {
		t.Fatalf("expected EPERM, got %#v", response)
	}
	response, _ = service.Handle(protocol.Request{Operation: "write", Path: "escape/secret"}, []byte("overwrite"))
	if response.ErrorCode != int(syscall.EPERM) {
		t.Fatalf("expected write EPERM, got %#v", response)
	}
}

func TestFileServiceExclusiveCreate(t *testing.T) {
	root := t.TempDir()
	service, _ := NewFileService(root)
	request := protocol.Request{Operation: "create", Path: "index.lock", Mode: 0o600}
	first, _ := service.Handle(request, nil)
	second, _ := service.Handle(request, nil)
	if first.ErrorCode != 0 || second.ErrorCode != int(syscall.EEXIST) {
		t.Fatalf("unexpected create responses: %#v %#v", first, second)
	}
}

func TestWriteDoesNotRecreateRemovedFile(t *testing.T) {
	root := t.TempDir()
	service, _ := NewFileService(root)
	_, _ = service.Handle(protocol.Request{Operation: "create", Path: "removed.txt", Mode: 0o600}, nil)
	_, _ = service.Handle(protocol.Request{Operation: "remove", Path: "removed.txt"}, nil)
	response, _ := service.Handle(protocol.Request{Operation: "write", Path: "removed.txt"}, []byte("must-not-return"))
	if response.ErrorCode != int(syscall.ENOENT) {
		t.Fatalf("write to removed file returned %#v", response)
	}
	if _, err := os.Stat(filepath.Join(root, "removed.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed file was recreated: %v", err)
	}
}

func TestFileServiceRejectsUnknownOperationAndOversizedRead(t *testing.T) {
	root := t.TempDir()
	service, _ := NewFileService(root)
	response, _ := service.Handle(protocol.Request{Operation: "unknown", Path: ""}, nil)
	if response.ErrorCode != int(syscall.ENOSYS) {
		t.Fatalf("unknown operation returned %#v", response)
	}
	if err := os.WriteFile(filepath.Join(root, "data"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	response, _ = service.Handle(protocol.Request{Operation: "read", Path: "data", Size: protocol.MaxPayloadBytes + 1}, nil)
	if response.ErrorCode != int(syscall.EINVAL) {
		t.Fatalf("oversized read returned %#v", response)
	}
}
