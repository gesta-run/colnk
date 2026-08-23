//go:build linux

package network

import (
	"fmt"
	"os"
)

const resolverPath = "/etc/resolv.conf"

func ConfigureResolver(nameserver string) (func() error, error) {
	original, err := os.ReadFile(resolverPath)
	if err != nil {
		return nil, fmt.Errorf("read resolver configuration: %w", err)
	}
	configuration := []byte("nameserver " + nameserver + "\noptions timeout:2 attempts:2\n")
	if err := os.WriteFile(resolverPath, configuration, 0o644); err != nil {
		return nil, fmt.Errorf("write resolver configuration: %w", err)
	}
	return func() error {
		return os.WriteFile(resolverPath, original, 0o644)
	}, nil
}
