package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	config, err := LoadConfig(writeServerConfig(t, "[auth]\napiKey = \"sk-test\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Listen != ":7443" || config.Mountpoint != "/mnt/local" ||
		config.MetadataCacheTTL != 10*time.Second || config.ProxyPort != 15001 ||
		config.Policy.MaxTCPConnections != 256 || !config.ConfigureResolver {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestLoadCompleteConfig(t *testing.T) {
	path := writeServerConfig(t, `
listen = "127.0.0.1:8443"
mountpoint = "/data/local"
allowOther = true
metadataCacheTTL = "750ms"

[auth]
apiKey = "sk-test"

[network]
interface = "bridge0"
proxyPort = 16001
maxTCPConnections = 32

[dns]
listen = "127.0.0.1:5353"
upstream = "9.9.9.9:53"
configureResolver = false
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Listen != "127.0.0.1:8443" || config.Mountpoint != "/data/local" || !config.AllowOther ||
		config.MetadataCacheTTL != 750*time.Millisecond || config.InterfaceName != "bridge0" ||
		config.ProxyPort != 16001 || config.Policy.MaxTCPConnections != 32 || config.ConfigureResolver {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestExampleConfig(t *testing.T) {
	contents, err := os.ReadFile("../../configs/server.toml.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(writeServerConfig(t, string(contents))); err != nil {
		t.Fatalf("example configuration is invalid: %v", err)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	tests := map[string]string{
		"missing key":       `listen = ":7443"`,
		"relative mount":    "mountpoint = \"relative\"\n[auth]\napiKey = \"sk-test\"\n",
		"invalid duration":  "metadataCacheTTL = \"fast\"\n[auth]\napiKey = \"sk-test\"\n",
		"invalid interface": "[auth]\napiKey = \"sk-test\"\n[network]\ninterface = \"bad name\"\n",
		"invalid DNS host":  "[auth]\napiKey = \"sk-test\"\n[dns]\nlisten = \"resolver.example:53\"\n",
		"unknown key":       "unknown = true\n[auth]\napiKey = \"sk-test\"\n",
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeServerConfig(t, contents)); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestValidateRuntime(t *testing.T) {
	if err := ValidateRuntime(0, "linux"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntime(501, "linux"); err == nil {
		t.Fatal("non-root server was accepted")
	}
	if err := ValidateRuntime(0, "darwin"); err == nil {
		t.Fatal("non-Linux server was accepted")
	}
}

func writeServerConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.toml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
