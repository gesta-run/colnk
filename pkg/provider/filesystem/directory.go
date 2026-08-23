package filesystem

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func (s *FileService) StreamDir(remotePath string, yield func([]byte, bool) error) error {
	name, err := cleanRemotePath(remotePath)
	if err != nil {
		return err
	}
	directory, err := s.root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	for {
		entries, readErr := directory.ReadDir(256)
		atEnd := errors.Is(readErr, io.EOF) || len(entries) == 0
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		result := make([]protocol.DirEntry, 0, len(entries))
		for _, entry := range entries {
			info, infoErr := entry.Info()
			if infoErr != nil || isSpecial(info.Mode()) {
				continue
			}
			value := protocol.DirEntry{Name: entry.Name(), Attr: fileAttr(info)}
			if info.Mode()&os.ModeSymlink != 0 {
				value.LinkTarget, _ = s.readlink(filepath.Join(name, entry.Name()))
			}
			result = append(result, value)
		}
		pages, encodeErr := protocol.EncodeDirEntryPages(result)
		if encodeErr != nil {
			return syscall.EOVERFLOW
		}
		for _, page := range pages {
			if err := yield(page, true); err != nil {
				return err
			}
		}
		if atEnd {
			return yield(nil, false)
		}
	}
}
