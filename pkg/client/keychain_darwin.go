//go:build darwin

package client

import (
	"fmt"
	"os/exec"
	"strings"
)

const keychainService = "ai.cloudpilot.colnk"

func readKeychain(endpoint string) (string, error) {
	output, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", endpoint, "-w").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func writeKeychain(endpoint, key string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint is required for Keychain storage")
	}
	command := exec.Command(
		"security", "add-generic-password", "-U", "-s", keychainService,
		"-a", endpoint, "-w",
	)
	command.Stdin = strings.NewReader(key + "\n")
	return command.Run()
}
