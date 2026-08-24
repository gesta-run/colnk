package server

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/gesta-run/colnk/pkg/configfile"
	"github.com/gesta-run/colnk/pkg/configvalidate"
	"github.com/gesta-run/colnk/pkg/protocol"
)

type Config struct {
	Listen            string
	APIKey            string
	Mountpoint        string
	InterfaceName     string
	DNSAddress        string
	UpstreamDNS       string
	ProxyPort         int
	ConfigureResolver bool
	AllowOther        bool
	MetadataCacheTTL  time.Duration
	Policy            protocol.NetworkPolicy
}

type fileConfig struct {
	Listen           string `toml:"listen"`
	Mountpoint       string `toml:"mountpoint"`
	AllowOther       bool   `toml:"allowOther"`
	MetadataCacheTTL string `toml:"metadataCacheTTL"`
	Auth             struct {
		APIKey string `toml:"apiKey"`
	} `toml:"auth"`
	Network struct {
		Interface         string `toml:"interface"`
		ProxyPort         int    `toml:"proxyPort"`
		MaxTCPConnections int    `toml:"maxTCPConnections"`
	} `toml:"network"`
	DNS struct {
		Listen            string `toml:"listen"`
		Upstream          string `toml:"upstream"`
		ConfigureResolver bool   `toml:"configureResolver"`
	} `toml:"dns"`
}

func LoadConfig(path string) (Config, error) {
	file := defaultFileConfig()
	if err := configfile.Load(path, &file); err != nil {
		return Config{}, err
	}
	metadataCacheTTL, err := time.ParseDuration(file.MetadataCacheTTL)
	if err != nil {
		return Config{}, fmt.Errorf("validate configuration %q: invalid metadataCacheTTL: %w", path, err)
	}
	policy := protocol.DefaultNetworkPolicy()
	policy.MaxTCPConnections = file.Network.MaxTCPConnections
	config := Config{
		Listen: strings.TrimSpace(file.Listen), APIKey: strings.TrimSpace(file.Auth.APIKey),
		Mountpoint: file.Mountpoint, AllowOther: file.AllowOther, MetadataCacheTTL: metadataCacheTTL,
		InterfaceName: file.Network.Interface, ProxyPort: file.Network.ProxyPort, Policy: policy,
		DNSAddress: file.DNS.Listen, UpstreamDNS: file.DNS.Upstream, ConfigureResolver: file.DNS.ConfigureResolver,
	}
	if err := validateConfig(&config); err != nil {
		return Config{}, fmt.Errorf("validate configuration %q: %w", path, err)
	}
	return config, nil
}

func defaultFileConfig() fileConfig {
	file := fileConfig{
		Listen: ":7443", Mountpoint: "/mnt/local", MetadataCacheTTL: "10s",
	}
	file.Network.Interface = "local0"
	file.Network.ProxyPort = 15001
	file.Network.MaxTCPConnections = 256
	file.DNS.Listen = "127.0.0.1:53"
	file.DNS.Upstream = "1.1.1.1:53"
	file.DNS.ConfigureResolver = true
	return file
}

func validateConfig(config *Config) error {
	if config.APIKey == "" {
		return errors.New("auth.apiKey is required")
	}
	if err := validateAddress("listen", config.Listen, true); err != nil {
		return err
	}
	if config.Mountpoint == "" || !filepath.IsAbs(config.Mountpoint) {
		return errors.New("mountpoint must be an absolute path")
	}
	config.Mountpoint = filepath.Clean(config.Mountpoint)
	if !validInterfaceName(config.InterfaceName) {
		return errors.New("network.interface must contain 1 to 15 letters, digits, dots, underscores, or hyphens")
	}
	if config.ProxyPort <= 0 || config.ProxyPort > 65535 {
		return errors.New("network.proxyPort must be between 1 and 65535")
	}
	if config.Policy.MaxTCPConnections <= 0 || config.Policy.MaxTCPConnections > 1024 {
		return errors.New("network.maxTCPConnections must be between 1 and 1024")
	}
	if config.MetadataCacheTTL < 100*time.Millisecond || config.MetadataCacheTTL > time.Minute {
		return errors.New("metadataCacheTTL must be between 100ms and 1m")
	}
	if err := validateAddress("dns.listen", config.DNSAddress, false); err != nil {
		return err
	}
	dnsHost, _, _ := net.SplitHostPort(config.DNSAddress)
	if net.ParseIP(dnsHost) == nil {
		return errors.New("dns.listen host must be an IP address")
	}
	return validateAddress("dns.upstream", config.UpstreamDNS, false)
}

func validInterfaceName(value string) bool {
	if value == "" || len(value) > 15 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validateAddress(field, value string, allowEmptyHost bool) error {
	if err := configvalidate.ValidateAddress(value, allowEmptyHost); err != nil {
		return fmt.Errorf("%s %w", field, err)
	}
	return nil
}

func ValidateRuntime(effectiveUserID int, goos string) error {
	if goos != "linux" {
		return errors.New("colnk-server is supported only on Linux")
	}
	if effectiveUserID != 0 {
		return errors.New("colnk-server requires root")
	}
	return nil
}
