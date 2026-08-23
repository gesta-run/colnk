package filesystem

import (
	"os"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	"golang.org/x/sys/unix"
)

func (s *FileService) create(name string, mode os.FileMode) (protocol.Response, []byte, error) {
	file, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return protocol.Response{}, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return protocol.Response{}, nil, err
	}
	if err := file.Close(); err != nil {
		return protocol.Response{}, nil, err
	}
	attr := fileAttr(info)
	return protocol.Response{Attr: &attr}, nil, nil
}

func (s *FileService) rename(oldName, newRemotePath string) (protocol.Response, []byte, error) {
	if err := s.rejectSpecial(oldName); err != nil {
		return protocol.Response{}, nil, err
	}
	newName, err := cleanRemotePath(newRemotePath)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	if err := s.rejectSpecialIfPresent(newName); err != nil {
		return protocol.Response{}, nil, err
	}
	oldDirectory, oldBase, err := s.openParent(oldName)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	defer oldDirectory.Close()
	newDirectory, newBase, err := s.openParent(newName)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	defer newDirectory.Close()
	if err = unix.Renameat(int(oldDirectory.Fd()), oldBase, int(newDirectory.Fd()), newBase); err != nil {
		return protocol.Response{}, nil, err
	}
	return s.stat(newName)
}

func (s *FileService) chmod(name string, mode os.FileMode) error {
	file, err := s.openMutable(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Chmod(mode.Perm())
}

func (s *FileService) chtimes(name string, modified time.Time) error {
	file, err := s.openMutable(name)
	if err != nil {
		return err
	}
	defer file.Close()
	value := unix.NsecToTimeval(modified.UnixNano())
	return unix.Futimes(int(file.Fd()), []unix.Timeval{value, value})
}

func (s *FileService) truncate(name string, size int64) error {
	file, err := s.openRegular(name, os.O_WRONLY|syscall.O_NONBLOCK)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Truncate(size)
}

func (s *FileService) remove(name string) error {
	if err := s.rejectSpecial(name); err != nil {
		return err
	}
	return s.root.Remove(name)
}
