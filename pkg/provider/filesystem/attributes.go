package filesystem

import (
	"errors"
	"os"
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func (s *FileService) stat(name string) (protocol.Response, []byte, error) {
	info, err := s.root.Lstat(name)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	if isSpecial(info.Mode()) {
		return protocol.Response{}, nil, syscall.EPERM
	}
	attr := fileAttr(info)
	return protocol.Response{Attr: &attr}, nil, nil
}

func (s *FileService) mutationResult(name string, err error) (protocol.Response, []byte, error) {
	if err != nil {
		return protocol.Response{}, nil, err
	}
	return s.stat(name)
}

func fileAttr(info os.FileInfo) protocol.FileAttr {
	attr := protocol.FileAttr{Mode: uint32(info.Mode()), Size: uint64(info.Size()), ModTimeNS: info.ModTime().UnixNano()}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		attr.Inode = stat.Ino
		attr.UID = stat.Uid
		attr.GID = stat.Gid
	}
	return attr
}

func Errno(err error) syscall.Errno {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		err = pathError.Err
	}
	if err != nil && err.Error() == "path escapes from parent" {
		return syscall.EPERM
	}
	var value syscall.Errno
	if errors.As(err, &value) {
		return value
	}
	if errors.Is(err, os.ErrPermission) {
		return syscall.EACCES
	}
	return syscall.EIO
}
