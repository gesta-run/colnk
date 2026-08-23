package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func (s *FileService) openMutable(name string) (*os.File, error) {
	info, err := s.root.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return nil, syscall.EPERM
	}
	return s.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func (s *FileService) openParent(name string) (*os.File, string, error) {
	if name == "." {
		return nil, "", syscall.EPERM
	}
	parent, base := filepath.Split(name)
	parent = strings.TrimSuffix(parent, string(filepath.Separator))
	if parent == "" {
		parent = "."
	}
	directory, err := s.root.Open(parent)
	if err != nil {
		return nil, "", err
	}
	info, err := directory.Stat()
	if err != nil || !info.IsDir() {
		_ = directory.Close()
		if err == nil {
			err = syscall.ENOTDIR
		}
		return nil, "", err
	}
	return directory, base, nil
}

func (s *FileService) rejectSpecial(name string) error {
	info, err := s.root.Lstat(name)
	if err != nil {
		return err
	}
	if isSpecial(info.Mode()) {
		return syscall.EPERM
	}
	return nil
}

func (s *FileService) rejectSpecialIfPresent(name string) error {
	err := s.rejectSpecial(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func cleanRemotePath(remotePath string) (string, error) {
	for _, component := range strings.FieldsFunc(remotePath, func(value rune) bool { return value == '/' || value == '\\' }) {
		if component == ".." {
			return "", syscall.EPERM
		}
	}
	clean := filepath.Clean(strings.TrimLeft(remotePath, "/\\"))
	if clean == "" {
		clean = "."
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", syscall.EPERM
	}
	return clean, nil
}

func isSpecial(mode os.FileMode) bool {
	return !mode.IsRegular() && !mode.IsDir() && mode&os.ModeSymlink == 0
}
