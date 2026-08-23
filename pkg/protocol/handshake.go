package protocol

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func ValidateHandshake(handshake Handshake) error {
	if handshake.MajorVersion != MajorVersion {
		return fmt.Errorf("unsupported protocol major version %d", handshake.MajorVersion)
	}
	if handshake.MinorVersion < 0 {
		return fmt.Errorf("invalid protocol minor version %d", handshake.MinorVersion)
	}
	if handshake.APIKey == "" {
		return errors.New("API key is required")
	}
	return ValidateNetworkPolicy(handshake.Policy)
}

func ValidateNetworkPolicy(policy NetworkPolicy) error {
	for _, value := range policy.AllowedCIDRs {
		ip, _, err := net.ParseCIDR(value)
		if err != nil || ip.To4() == nil {
			return fmt.Errorf("only IPv4 CIDRs are supported: %q", value)
		}
	}
	for _, suffix := range policy.DNSSuffixes {
		normalized := strings.TrimSuffix(strings.TrimSpace(suffix), ".")
		if normalized == "" || len(normalized) > 253 || strings.ContainsAny(normalized, " /:") {
			return fmt.Errorf("invalid DNS suffix %q", suffix)
		}
	}
	return nil
}
