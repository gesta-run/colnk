//go:build !linux

package network

import "fmt"

func ConfigureResolver(_ string) (func() error, error) {
	return nil, fmt.Errorf("resolver configuration is supported only on Linux")
}
