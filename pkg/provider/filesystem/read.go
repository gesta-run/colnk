package filesystem

import (
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func (s *FileService) read(name string, offset int64, size int) (protocol.Response, []byte, error) {
	if size <= 0 || size > protocol.MaxPayloadBytes {
		return protocol.Response{}, nil, syscall.EINVAL
	}
	file, err := s.openRegular(name, os.O_RDONLY|syscall.O_NONBLOCK)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	defer file.Close()
	data := make([]byte, size)
	count, err := file.ReadAt(data, offset)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return protocol.Response{}, data[:count], err
}

func (s *FileService) openRegular(name string, flags int) (*os.File, error) {
	info, err := s.root.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, syscall.EPERM
	}
	file, err := s.root.OpenFile(name, flags, 0)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		if err == nil {
			err = syscall.EPERM
		}
		return nil, err
	}
	return file, nil
}
