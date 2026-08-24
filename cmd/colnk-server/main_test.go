package main

import (
	"errors"
	"flag"
	"testing"
)

func TestParseArguments(t *testing.T) {
	path, showVersion, err := parseArguments([]string{"--config", "/tmp/server.toml"}, "/default.toml", "colnk-server")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/server.toml" || showVersion {
		t.Fatalf("unexpected arguments: path=%q version=%t", path, showVersion)
	}
	_, showVersion, err = parseArguments([]string{"--version"}, "/default.toml", "colnk-server")
	if err != nil || !showVersion {
		t.Fatalf("version flag was not accepted: version=%t err=%v", showVersion, err)
	}
}

func TestParseArgumentsRejectsBusinessAndPositionalArguments(t *testing.T) {
	for _, arguments := range [][]string{{"start"}, {"--listen", ":7443"}, {"--api-key", "sk-test"}} {
		if _, _, err := parseArguments(arguments, "/default.toml", "colnk-server"); err == nil {
			t.Fatalf("arguments were accepted: %v", arguments)
		}
	}
}

func TestParseArgumentsHelp(t *testing.T) {
	if _, _, err := parseArguments([]string{"--help"}, "/default.toml", "colnk-server"); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("unexpected help error: %v", err)
	}
}
