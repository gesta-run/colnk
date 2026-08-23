package filesystem

import (
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

type FileService struct {
	root *os.Root
}

func NewFileService(rootPath string) (*FileService, error) {
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, syscall.ENOTDIR
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, err
	}
	return &FileService{root: root}, nil
}

func (s *FileService) Close() error {
	return s.root.Close()
}

func (s *FileService) Handle(request protocol.Request, data []byte) (protocol.Response, []byte) {
	response, payload, err := s.execute(request, data)
	if err == nil {
		return response, payload
	}
	return protocol.Response{ErrorCode: int(Errno(err)), Error: err.Error()}, nil
}

func (s *FileService) execute(request protocol.Request, data []byte) (protocol.Response, []byte, error) {
	name, err := cleanRemotePath(request.Path)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	switch request.Operation {
	case "stat":
		return s.stat(name)
	case "read":
		return s.read(name, request.Offset, request.Size)
	case "write":
		return s.write(name, request.Offset, data, request.SetModTime, request.ModTimeNS, request.Sync)
	case "create":
		return s.create(name, os.FileMode(request.Mode))
	case "truncate":
		return s.mutationResult(name, s.truncate(name, request.Offset))
	case "mkdir":
		return s.mutationResult(name, s.root.Mkdir(name, os.FileMode(request.Mode).Perm()))
	case "remove":
		return protocol.Response{}, nil, s.remove(name)
	case "rename":
		return s.rename(name, request.NewPath)
	case "symlink":
		return s.mutationResult(name, s.symlink(name, request.Target))
	case "readlink":
		target, readErr := s.readlink(name)
		return protocol.Response{}, []byte(target), readErr
	case "chmod":
		return s.mutationResult(name, s.chmod(name, os.FileMode(request.Mode)))
	case "chtimes":
		return s.mutationResult(name, s.chtimes(name, time.Unix(0, request.ModTimeNS)))
	case "fsync":
		return protocol.Response{}, nil, s.sync(name)
	default:
		return protocol.Response{}, nil, syscall.ENOSYS
	}
}
