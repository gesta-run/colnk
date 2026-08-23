//go:build linux

package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func (r *networkRuntime) reconcileStaleState() error {
	if err := r.cleanupFirewall(); err != nil {
		return err
	}
	r.removeLegacyFirewallRules()
	if _, err := net.InterfaceByName(r.config.InterfaceName); err != nil {
		return nil
	}
	owned, err := interfaceOwnedByCoLnk(r.config.InterfaceName)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("network interface %q already exists and is not owned by CoLnk", r.config.InterfaceName)
	}
	return runCommand("ip", "link", "del", r.config.InterfaceName)
}

func (r *networkRuntime) prepareFirewall() error {
	if err := runCommand("iptables", "-t", "nat", "-N", firewallChain); err != nil {
		return err
	}
	return runCommand("iptables", "-t", "nat", "-A", "OUTPUT", "-j", firewallChain)
}

func (r *networkRuntime) removeLegacyFirewallRules() {
	output, err := commandOutput("iptables", "-t", "nat", "-S", "OUTPUT")
	if err != nil {
		return
	}
	port := strconv.Itoa(r.config.ProxyPort)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !containsFields(fields, "-j", "REDIRECT") || !containsFields(fields, "--to-ports", port) {
			continue
		}
		fields[0] = "-D"
		_ = runCommand(append([]string{"iptables", "-t", "nat"}, fields...)...)
	}
}

func (r *networkRuntime) addRoute(cidr string) error {
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("invalid network policy cidr %q: %w", cidr, err)
	}
	if err := runCommand("ip", "route", "add", cidr, "dev", r.config.InterfaceName); err != nil {
		return err
	}
	port := strconv.Itoa(r.config.ProxyPort)
	rule := []string{"iptables", "-t", "nat", "-A", firewallChain, "-p", "tcp", "-d", cidr, "-j", "REDIRECT", "--to-ports", port}
	if err := runCommand(rule...); err != nil {
		_ = runCommand("ip", "route", "del", cidr, "dev", r.config.InterfaceName)
		return err
	}
	r.routes = append(r.routes, cidr)
	return nil
}

func (r *networkRuntime) cleanupFirewall() error {
	var firstError error
	for commandSucceeds("iptables", "-t", "nat", "-C", "OUTPUT", "-j", firewallChain) {
		if err := runCommand("iptables", "-t", "nat", "-D", "OUTPUT", "-j", firewallChain); err != nil {
			firstError = err
			break
		}
	}
	if commandSucceeds("iptables", "-t", "nat", "-L", firewallChain) {
		if err := runCommand("iptables", "-t", "nat", "-F", firewallChain); err != nil && firstError == nil {
			firstError = err
		}
		if err := runCommand("iptables", "-t", "nat", "-X", firewallChain); err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}
