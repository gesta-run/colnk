package filesystem

import (
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
	"golang.org/x/sys/unix"
)

func (s *FileService) symlink(name, target string) error {
	directory, base, err := s.openParent(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return unix.Symlinkat(target, int(directory.Fd()), base)
}

func (s *FileService) readlink(name string) (string, error) {
	directory, base, err := s.openParent(name)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	buffer := make([]byte, 256)
	for len(buffer) <= protocol.MaxHeaderBytes {
		count, err := unix.Readlinkat(int(directory.Fd()), base, buffer)
		if err != nil {
			return "", err
		}
		if count < len(buffer) {
			return string(buffer[:count]), nil
		}
		buffer = make([]byte, len(buffer)*2)
	}
	return "", syscall.ENAMETOOLONG
}
