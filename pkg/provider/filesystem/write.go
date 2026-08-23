package filesystem

import (
	"os"
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
	"golang.org/x/sys/unix"
)

func (s *FileService) write(name string, offset int64, data []byte, setModTime bool, modTimeNS int64, syncData bool) (protocol.Response, []byte, error) {
	file, err := s.openRegular(name, os.O_WRONLY|syscall.O_NONBLOCK)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	defer file.Close()
	if _, err = file.WriteAt(data, offset); err != nil {
		return protocol.Response{}, nil, err
	}
	if setModTime {
		value := unix.NsecToTimeval(modTimeNS)
		if err := unix.Futimes(int(file.Fd()), []unix.Timeval{value, value}); err != nil {
			return protocol.Response{}, nil, err
		}
	}
	if syncData {
		if err := file.Sync(); err != nil {
			return protocol.Response{}, nil, err
		}
	}
	info, err := file.Stat()
	if err != nil {
		return protocol.Response{}, nil, err
	}
	attr := fileAttr(info)
	return protocol.Response{Attr: &attr}, nil, nil
}

func (s *FileService) sync(name string) error {
	file, err := s.openRegular(name, os.O_RDWR|syscall.O_NONBLOCK)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
