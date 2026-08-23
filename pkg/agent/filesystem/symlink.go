package filesystem

import (
	"path"
	"strings"
)

func mountSymlinkTarget(mountpoint, target string) string {
	if !path.IsAbs(target) {
		return target
	}
	return path.Join(mountpoint, target)
}

func localSymlinkTarget(mountpoint, target string) string {
	mountpoint = path.Clean(mountpoint)
	if target == mountpoint {
		return "/"
	}
	prefix := mountpoint + "/"
	if strings.HasPrefix(target, prefix) {
		return "/" + strings.TrimPrefix(target, prefix)
	}
	return target
}
