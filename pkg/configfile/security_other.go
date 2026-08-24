//go:build !darwin && !linux

package configfile

import "os"

func verifySecurity(_ os.FileInfo) error {
	return nil
}
