package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	path := writeConfig(t, `
endpoint = "agent.example.test:7443"

[auth]
apiKey = "sk-test"
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Endpoint != "agent.example.test:7443" || config.Root != "/" ||
		len(config.Policy.AllowedCIDRs) != 1 || config.Policy.AllowedCIDRs[0] != "100.64.0.1/32" ||
		len(config.Policy.AllowedPorts) != 0 || len(config.Policy.DNSSuffixes) != 1 {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestLoadNetworkSharing(t *testing.T) {
	path := writeConfig(t, `
endpoint = "agent.example.test:7443"
root = "/tmp"

[auth]
apiKey = "sk-test"

[network]
allowCIDRs = ["192.168.1.0/24", "10.20.0.0/16", "192.168.1.0/24"]
allowPorts = [22, 443, 443]
dnsSuffixes = ["corp.example"]

[logging]
auditResources = true
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Policy.AllowedCIDRs) != 2 || len(config.Policy.AllowedPorts) != 2 ||
		len(config.Policy.DNSSuffixes) != 1 || !config.AuditResources {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestExampleConfig(t *testing.T) {
	contents, err := os.ReadFile("../../configs/client.toml.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(writeConfig(t, string(contents))); err != nil {
		t.Fatalf("example configuration is invalid: %v", err)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	tests := map[string]string{
		"missing key":      `endpoint = "agent.example.test:7443"`,
		"relative root":    "endpoint = \"agent.example.test:7443\"\nroot = \"relative\"\n[auth]\napiKey = \"sk-test\"\n",
		"missing root":     "endpoint = \"agent.example.test:7443\"\nroot = \"/path/that/does/not/exist\"\n[auth]\napiKey = \"sk-test\"\n",
		"invalid endpoint": "endpoint = \"https://agent.example.test\"\n[auth]\napiKey = \"sk-test\"\n",
		"unknown key":      "endpoint = \"agent.example.test:7443\"\nunknown = true\n[auth]\napiKey = \"sk-test\"\n",
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, contents)); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestRootUserIsRejected(t *testing.T) {
	if err := ValidateUser(0); err == nil {
		t.Fatal("root user was accepted")
	}
	if err := ValidateUser(501); err != nil {
		t.Fatalf("non-root user was rejected: %v", err)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "client.toml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
