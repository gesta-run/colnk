package configfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadStrictConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("value = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Value string `toml:"value"`
	}
	if err := Load(path, &config); err != nil {
		t.Fatal(err)
	}
	if config.Value != "ok" {
		t.Fatalf("unexpected value %q", config.Value)
	}
	if err := os.WriteFile(path, []byte("unknown = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path, &config); err == nil {
		t.Fatalf("unknown key was not rejected: %v", err)
	}
}

func TestLoadRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix permission bits are not enforced on this platform")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("value = \"ok\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Value string `toml:"value"`
	}
	if err := Load(path, &config); err == nil || !strings.Contains(err.Error(), "group or other") {
		t.Fatalf("broad permissions were not rejected: %v", err)
	}
}

func TestLoadAcceptsSecureSymbolicLinks(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.toml")
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(target, []byte("value = \"ok\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Value string `toml:"value"`
	}
	if err := Load(path, &config); err != nil {
		t.Fatalf("secure symbolic link was rejected: %v", err)
	}
	if config.Value != "ok" {
		t.Fatalf("unexpected value %q", config.Value)
	}
}

func TestLoadRejectsSymbolicLinkToBroadPermissions(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix permission bits are not enforced on this platform")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target.toml")
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(target, []byte("value = \"ok\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Value string `toml:"value"`
	}
	if err := Load(path, &config); err == nil || !strings.Contains(err.Error(), "group or other") {
		t.Fatalf("insecure symbolic-link target was not rejected: %v", err)
	}
}
