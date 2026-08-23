//go:build linux

package network

import (
	"fmt"
	"os/exec"
	"strings"
)

func interfaceOwnedByCoLnk(name string) (bool, error) {
	details, err := commandOutput("ip", "-d", "-o", "link", "show", "dev", name)
	if err != nil {
		return false, err
	}
	if strings.Contains(details, "alias "+interfaceOwnerAlias) {
		return true, nil
	}
	addresses, err := commandOutput("ip", "-o", "addr", "show", "dev", name)
	if err != nil {
		return false, err
	}
	return strings.Contains(addresses, "100.64.0.2/32"), nil
}

func containsFields(fields []string, left, right string) bool {
	for index := 0; index+1 < len(fields); index++ {
		if fields[index] == left && fields[index+1] == right {
			return true
		}
	}
	return false
}

func commandSucceeds(arguments ...string) bool {
	return exec.Command(arguments[0], arguments[1:]...).Run() == nil
}

func runCommand(arguments ...string) error {
	if len(arguments) == 0 {
		return nil
	}
	output, err := commandOutput(arguments...)
	if err != nil {
		return fmt.Errorf("run %v: %w: %s", arguments, err, output)
	}
	return nil
}

func commandOutput(arguments ...string) (string, error) {
	output, err := exec.Command(arguments[0], arguments[1:]...).CombinedOutput()
	return string(output), err
}
