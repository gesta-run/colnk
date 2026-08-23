package server

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

const Usage = `Usage:
  colnk-server --api-key sk-xxx [options]

Options:
  --listen string             TCP listen address (default ":7443")
  --api-key string            shared API key; COLNK_API_KEY is safer
  --api-key-file string       file containing the shared API key
  --mountpoint string         FUSE mountpoint (default "/mnt/local")
  --allow-other               allow other trusted VM users to access the mount
  --metadata-cache-ttl duration
                              metadata cache lifetime (default 10s)
  --interface string          tunnel interface name (default "local0")
  --proxy-port int            transparent TCP proxy port (default 15001)
  --dns-listen string         split DNS listener (default "127.0.0.1:53")
  --upstream-dns string       public DNS upstream (default "1.1.1.1:53")
  --configure-resolver        route Agent DNS through split DNS (default true)
  --max-tcp-connections int   maximum concurrent TCP connections (default 256)
`

type Config struct {
	Listen            string
	APIKey            string
	APIKeyFile        string
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

func ParseConfig(arguments []string) (Config, error) {
	var config Config
	config.Policy = protocol.DefaultNetworkPolicy()
	flags := flag.NewFlagSet("colnk-server", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.Listen, "listen", ":7443", "TCP listen address")
	flags.StringVar(&config.APIKey, "api-key", "", "shared API key; prefer COLNK_API_KEY")
	flags.StringVar(&config.APIKeyFile, "api-key-file", "", "file containing the shared API key")
	flags.StringVar(&config.Mountpoint, "mountpoint", "/mnt/local", "FUSE mountpoint")
	flags.BoolVar(&config.AllowOther, "allow-other", false, "allow trusted VM users to access the FUSE mount")
	flags.DurationVar(&config.MetadataCacheTTL, "metadata-cache-ttl", 10*time.Second, "metadata cache lifetime")
	flags.StringVar(&config.InterfaceName, "interface", "local0", "Agent tunnel interface")
	flags.IntVar(&config.ProxyPort, "proxy-port", 15001, "transparent TCP proxy port")
	flags.StringVar(&config.DNSAddress, "dns-listen", "127.0.0.1:53", "split DNS listener")
	flags.StringVar(&config.UpstreamDNS, "upstream-dns", "1.1.1.1:53", "public DNS upstream")
	flags.BoolVar(&config.ConfigureResolver, "configure-resolver", true, "route Agent DNS through split DNS")
	flags.IntVar(&config.Policy.MaxTCPConnections, "max-tcp-connections", 256, "maximum concurrent TCP connections")
	if err := flags.Parse(arguments); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, errors.New("unexpected positional arguments")
	}
	return config, nil
}

func ResolveConfig(config *Config) error {
	if config.APIKey == "" && config.APIKeyFile != "" {
		data, err := os.ReadFile(config.APIKeyFile)
		if err != nil {
			return fmt.Errorf("read API key file: %w", err)
		}
		config.APIKey = strings.TrimSpace(string(data))
	}
	if config.APIKey == "" {
		config.APIKey = strings.TrimSpace(os.Getenv("COLNK_API_KEY"))
	}
	if config.APIKey == "" {
		return errors.New("API key is required")
	}
	if runtime.GOOS != "linux" {
		return errors.New("colnk-server is supported only on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("colnk-server requires root")
	}
	if config.Policy.MaxTCPConnections <= 0 || config.Policy.MaxTCPConnections > 1024 {
		return errors.New("max-tcp-connections must be between 1 and 1024")
	}
	if config.MetadataCacheTTL < 100*time.Millisecond || config.MetadataCacheTTL > time.Minute {
		return errors.New("metadata-cache-ttl must be between 100ms and 1m")
	}
	return nil
}
