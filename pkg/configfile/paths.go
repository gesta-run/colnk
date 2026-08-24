package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const ServerPath = "/etc/colnk/server.toml"

func ClientPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	directory := "colnk"
	if runtime.GOOS == "darwin" {
		directory = "CoLnk"
	}
	return filepath.Join(base, directory, "client.toml"), nil
}
