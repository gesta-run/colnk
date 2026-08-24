package configfile

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

func Load(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open configuration %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect configuration %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("configuration %q is not a regular file", path)
	}
	if err := verifySecurity(info); err != nil {
		return fmt.Errorf("configuration %q: %w", path, err)
	}
	decoder := toml.NewDecoder(file).DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode configuration %q: %w", path, err)
	}
	return nil
}
