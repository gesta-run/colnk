package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gesta-run/colnk/pkg/protocol"
)

const riskNotice = "CoLnk gives the cloud agent read-write access to the selected local filesystem root and access to the selected local networks."

const Usage = `Usage:
  colnk start --endpoint agent.example.com:7443 [options]

Options:
  --api-key string    API key; COLNK_API_KEY and macOS Keychain are safer
  --root string       local filesystem root (default "/")
  --allow-cidrs string
                      comma-separated local IPv4 CIDRs (default "100.64.0.1/32")
  --allow-ports string
                      comma-separated local TCP ports (default: all)
  --dns-suffixes string
                      comma-separated DNS suffixes resolved locally (default "colnk")
  --save-key          save the selected key in macOS Keychain
  --accept-risk       confirm filesystem and network sharing
  --audit-resources   include local paths and network targets in logs
`

type Config struct {
	Endpoint       string
	APIKey         string
	Root           string
	AcceptRisk     bool
	SaveKey        bool
	AuditResources bool
	Policy         protocol.NetworkPolicy
}

func ParseConfig(arguments []string) (Config, error) {
	if len(arguments) > 0 && arguments[0] == "start" {
		arguments = arguments[1:]
	} else if len(arguments) > 0 && !strings.HasPrefix(arguments[0], "-") {
		return Config{}, fmt.Errorf("unknown command %q; expected start", arguments[0])
	}
	var config Config
	config.Policy = protocol.DefaultNetworkPolicy()
	var allowedCIDRs, allowedPorts, dnsSuffixes string
	flags := flag.NewFlagSet("colnk start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.Endpoint, "endpoint", "", "CoLnk server host:port")
	flags.StringVar(&config.APIKey, "api-key", "", "API key; prefer COLNK_API_KEY")
	flags.StringVar(&config.Root, "root", "/", "local filesystem root")
	flags.StringVar(&allowedCIDRs, "allow-cidrs", strings.Join(config.Policy.AllowedCIDRs, ","), "local IPv4 CIDRs exposed to the agent")
	flags.StringVar(&allowedPorts, "allow-ports", "", "local TCP ports exposed to the agent; empty allows all")
	flags.StringVar(&dnsSuffixes, "dns-suffixes", strings.Join(config.Policy.DNSSuffixes, ","), "DNS suffixes resolved on the local host")
	flags.BoolVar(&config.AcceptRisk, "accept-risk", false, "confirm global read-write filesystem sharing")
	flags.BoolVar(&config.SaveKey, "save-key", false, "save the selected API key in macOS Keychain")
	flags.BoolVar(&config.AuditResources, "audit-resources", false, "include local paths and network targets in logs")
	if err := flags.Parse(arguments); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, errors.New("unexpected positional arguments")
	}
	ports, err := parsePorts(allowedPorts)
	if err != nil {
		return Config{}, err
	}
	config.Policy.AllowedCIDRs = splitValues(allowedCIDRs)
	config.Policy.AllowedPorts = ports
	config.Policy.DNSSuffixes = splitValues(dnsSuffixes)
	if err := protocol.ValidateNetworkPolicy(config.Policy); err != nil {
		return Config{}, err
	}
	return config, nil
}

func ResolveAPIKey(config *Config) error {
	return resolveAPIKey(config, os.Getenv, readKeychain)
}

func resolveAPIKey(config *Config, getenv func(string) string, keychain func(string) (string, error)) error {
	if config.APIKey == "" {
		config.APIKey = getenv("COLNK_API_KEY")
	}
	if config.APIKey == "" {
		config.APIKey, _ = keychain(config.Endpoint)
	}
	if config.Endpoint == "" || config.APIKey == "" {
		return errors.New("endpoint and API key are required")
	}
	if config.SaveKey {
		if err := writeKeychain(config.Endpoint, config.APIKey); err != nil {
			return fmt.Errorf("save API key in Keychain: %w", err)
		}
	}
	return nil
}

func ValidateUser(effectiveUserID int) error {
	if effectiveUserID == 0 {
		return errors.New("colnk must run as a non-root user")
	}
	return nil
}

func ConfirmRisk(config Config, input io.Reader, output io.Writer) error {
	statePath, err := riskStatePath(config)
	if err != nil {
		return err
	}
	if _, err := os.Stat(statePath); err == nil {
		return nil
	}
	accepted := config.AcceptRisk || os.Getenv("COLNK_ACCEPT_RISK") == "1"
	if !accepted {
		ports := "all"
		if len(config.Policy.AllowedPorts) > 0 {
			values := make([]string, 0, len(config.Policy.AllowedPorts))
			for _, port := range config.Policy.AllowedPorts {
				values = append(values, strconv.Itoa(int(port)))
			}
			ports = strings.Join(values, ",")
		}
		_, _ = fmt.Fprintf(output, "%s\nEndpoint: %s\nRoot: %s\nCIDRs: %s\nPorts: %s\nDNS suffixes: %s\nType 'accept' to continue: ",
			riskNotice, config.Endpoint, config.Root, strings.Join(config.Policy.AllowedCIDRs, ","), ports,
			strings.Join(config.Policy.DNSSuffixes, ","))
		var response string
		_, _ = fmt.Fscanln(input, &response)
		accepted = strings.EqualFold(response, "accept")
	}
	if !accepted {
		return errors.New("sharing was not accepted")
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create CoLnk state directory: %w", err)
	}
	if err := os.WriteFile(statePath, []byte(riskNotice+"\n"), 0o600); err != nil {
		return fmt.Errorf("save risk confirmation: %w", err)
	}
	return nil
}

func riskStatePath(config Config) (string, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return "", fmt.Errorf("resolve shared root: %w", err)
	}
	credentialHash := sha256.Sum256([]byte(config.APIKey))
	policyJSON, _ := json.Marshal(config.Policy)
	scope := strings.Join([]string{config.Endpoint, filepath.Clean(root), "rw", string(policyJSON), hex.EncodeToString(credentialHash[:])}, "\x00")
	scopeHash := sha256.Sum256([]byte(scope))
	filename := "risk-accepted-v2-" + hex.EncodeToString(scopeHash[:16])
	if stateDir := os.Getenv("COLNK_STATE_DIR"); stateDir != "" {
		return filepath.Join(stateDir, filename), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "CoLnk", filename), nil
}

func parsePorts(raw string) ([]uint16, error) {
	var ports []uint16
	for _, value := range splitValues(raw) {
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("invalid TCP port %q", value)
		}
		ports = append(ports, uint16(port))
	}
	return ports, nil
}

func splitValues(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
