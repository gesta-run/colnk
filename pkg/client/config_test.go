package client

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseStartCommand(t *testing.T) {
	config, err := ParseConfig([]string{"start", "--endpoint", "agent.example.test:7443"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Endpoint != "agent.example.test:7443" || config.Root != "/" ||
		len(config.Policy.AllowedCIDRs) != 1 || config.Policy.AllowedCIDRs[0] != "100.64.0.1/32" ||
		len(config.Policy.AllowedPorts) != 0 {
		t.Fatalf("unexpected config %#v", config)
	}
}

func TestParseNetworkSharing(t *testing.T) {
	config, err := ParseConfig([]string{
		"start", "--endpoint", "agent.example.test:7443",
		"--allow-cidrs", "192.168.1.0/24,10.20.0.0/16",
		"--allow-ports", "22,443", "--dns-suffixes", "corp.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Policy.AllowedCIDRs) != 2 || len(config.Policy.AllowedPorts) != 2 ||
		len(config.Policy.DNSSuffixes) != 1 || config.Policy.DNSSuffixes[0] != "corp.example" {
		t.Fatalf("unexpected network policy %#v", config.Policy)
	}
}

func TestAPIKeyPriority(t *testing.T) {
	t.Setenv("COLNK_API_KEY", "sk-test-environment")
	config := Config{Endpoint: "agent.example.test:7443", APIKey: "sk-test-flag"}
	if err := ResolveAPIKey(&config); err != nil {
		t.Fatal(err)
	}
	if config.APIKey != "sk-test-flag" {
		t.Fatalf("explicit API key did not take priority: %q", config.APIKey)
	}
	config.APIKey = ""
	if err := ResolveAPIKey(&config); err != nil {
		t.Fatal(err)
	}
	if config.APIKey != "sk-test-environment" {
		t.Fatalf("environment API key not selected: %q", config.APIKey)
	}
}

func TestAPIKeyFallsBackToKeychain(t *testing.T) {
	config := Config{Endpoint: "agent.example.test:7443"}
	keychain := func(endpoint string) (string, error) {
		if endpoint != config.Endpoint {
			t.Fatalf("unexpected Keychain account %q", endpoint)
		}
		return "sk-test-keychain", nil
	}
	if err := resolveAPIKey(&config, func(string) string { return "" }, keychain); err != nil {
		t.Fatal(err)
	}
	if config.APIKey != "sk-test-keychain" {
		t.Fatalf("Keychain API key was not selected: %q", config.APIKey)
	}
}

func TestRiskConfirmation(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("COLNK_STATE_DIR", stateDir)
	config := Config{Endpoint: "agent.example.test:7443", APIKey: "sk-test", Root: "/", AcceptRisk: true}
	if err := ConfirmRisk(config, bytes.NewBuffer(nil), bytes.NewBuffer(nil)); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(stateDir, "risk-accepted-v2-*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected risk state files %#v: %v", files, err)
	}
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected risk file mode %o", info.Mode().Perm())
	}
	config.Endpoint = "other.example.test:7443"
	if err := ConfirmRisk(config, bytes.NewBuffer(nil), bytes.NewBuffer(nil)); err != nil {
		t.Fatal(err)
	}
	files, _ = filepath.Glob(filepath.Join(stateDir, "risk-accepted-v2-*"))
	if len(files) != 2 {
		t.Fatalf("risk consent was not scoped by endpoint: %#v", files)
	}
}

func TestRiskConfirmationDeclined(t *testing.T) {
	t.Setenv("COLNK_STATE_DIR", t.TempDir())
	if err := ConfirmRisk(Config{Endpoint: "agent.example.test:7443", APIKey: "sk-test", Root: "/"}, bytes.NewBufferString("no\n"), bytes.NewBuffer(nil)); err == nil {
		t.Fatal("expected declined risk confirmation to fail")
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

func TestMissingEndpointOrAPIKey(t *testing.T) {
	t.Setenv("COLNK_API_KEY", "")
	if err := ResolveAPIKey(&Config{APIKey: "sk-test"}); err == nil {
		t.Fatal("missing endpoint was accepted")
	}
}
