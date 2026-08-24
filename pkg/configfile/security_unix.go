//go:build darwin || linux

package configfile

import (
	"fmt"
	"os"
	"syscall"
)

func verifySecurity(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("must not be accessible by group or other users")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("must be owned by uid %d", os.Geteuid())
	}
	return nil
}
