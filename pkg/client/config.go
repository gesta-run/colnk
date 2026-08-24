package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gesta-run/colnk/pkg/configfile"
	"github.com/gesta-run/colnk/pkg/configvalidate"
	"github.com/gesta-run/colnk/pkg/protocol"
)

const (
	maxSharedCIDRs = 256
	maxSharedPorts = 256
	maxDNSSuffixes = 256
)

type Config struct {
	Endpoint       string
	APIKey         string
	Root           string
	AuditResources bool
	Policy         protocol.NetworkPolicy
}

type fileConfig struct {
	Endpoint string `toml:"endpoint"`
	Root     string `toml:"root"`
	Auth     struct {
		APIKey string `toml:"apiKey"`
	} `toml:"auth"`
	Network struct {
		AllowCIDRs  []string `toml:"allowCIDRs"`
		AllowPorts  []uint16 `toml:"allowPorts"`
		DNSSuffixes []string `toml:"dnsSuffixes"`
	} `toml:"network"`
	Logging struct {
		AuditResources bool `toml:"auditResources"`
	} `toml:"logging"`
}

func LoadConfig(path string) (Config, error) {
	defaults := protocol.DefaultNetworkPolicy()
	file := fileConfig{Root: "/"}
	file.Network.AllowCIDRs = defaults.AllowedCIDRs
	file.Network.DNSSuffixes = defaults.DNSSuffixes
	if err := configfile.Load(path, &file); err != nil {
		return Config{}, err
	}
	for _, port := range file.Network.AllowPorts {
		if port == 0 {
			return Config{}, fmt.Errorf("validate configuration %q: network.allowPorts must contain ports between 1 and 65535", path)
		}
	}
	config := Config{
		Endpoint: strings.TrimSpace(file.Endpoint), APIKey: strings.TrimSpace(file.Auth.APIKey),
		Root: file.Root, AuditResources: file.Logging.AuditResources,
		Policy: protocol.NetworkPolicy{
			AllowedCIDRs: dedupeStrings(file.Network.AllowCIDRs),
			AllowedPorts: dedupePorts(file.Network.AllowPorts),
			DNSSuffixes:  dedupeStrings(file.Network.DNSSuffixes),
		},
	}
	if err := validateConfig(&config); err != nil {
		return Config{}, fmt.Errorf("validate configuration %q: %w", path, err)
	}
	return config, nil
}

func validateConfig(config *Config) error {
	if config.Endpoint == "" || config.APIKey == "" {
		return errors.New("endpoint and auth.apiKey are required")
	}
	if err := configvalidate.ValidateAddress(config.Endpoint, false); err != nil {
		return fmt.Errorf("endpoint %w", err)
	}
	if config.Root == "" || !filepath.IsAbs(config.Root) {
		return errors.New("root must be an absolute path")
	}
	config.Root = filepath.Clean(config.Root)
	root, err := os.OpenRoot(config.Root)
	if err != nil {
		return fmt.Errorf("root must be an accessible directory: %w", err)
	}
	if err := root.Close(); err != nil {
		return fmt.Errorf("close root after validation: %w", err)
	}
	if len(config.Policy.AllowedCIDRs) > maxSharedCIDRs || len(config.Policy.AllowedPorts) > maxSharedPorts || len(config.Policy.DNSSuffixes) > maxDNSSuffixes {
		return errors.New("network policy exceeds the maximum of 256 CIDRs, ports, or DNS suffixes")
	}
	return protocol.ValidateNetworkPolicy(config.Policy)
}

func ValidateUser(effectiveUserID int) error {
	if effectiveUserID == 0 {
		return errors.New("colnk must run as a non-root user")
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, exists := seen[value]; value == "" || exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func dedupePorts(values []uint16) []uint16 {
	seen := make(map[uint16]struct{}, len(values))
	result := make([]uint16, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
