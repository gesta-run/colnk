//go:build !darwin

package client

import "errors"

func readKeychain(string) (string, error) {
	return "", errors.New("system Keychain is only available on macOS")
}

func writeKeychain(string, string) error {
	return errors.New("system Keychain is only available on macOS")
}
