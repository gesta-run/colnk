package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/gesta-run/colnk/pkg/configfile"
	"github.com/gesta-run/colnk/pkg/server"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	configPath, showVersion, err := parseArguments(os.Args[1:], configfile.ServerPath, "colnk-server")
	if errors.Is(err, flag.ErrHelp) {
		printUsage(os.Stdout, "colnk-server", configfile.ServerPath)
		return
	}
	if err != nil {
		logger.Error("parse command", "error", err)
		os.Exit(2)
	}
	if showVersion {
		_, _ = fmt.Fprintf(os.Stdout, "colnk-server %s\n", version)
		return
	}
	if err := server.ValidateRuntime(os.Geteuid(), runtime.GOOS); err != nil {
		logger.Error("validate runtime", "error", err)
		os.Exit(2)
	}
	config, err := server.LoadConfig(configPath)
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, config, logger); err != nil {
		logger.Error("run CoLnk server", "error", err)
		os.Exit(1)
	}
}

func parseArguments(arguments []string, defaultPath, name string) (string, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := defaultPath
	showVersion := false
	flags.StringVar(&configPath, "config", defaultPath, "TOML configuration file")
	flags.BoolVar(&showVersion, "version", false, "print version")
	if err := flags.Parse(arguments); err != nil {
		return "", false, err
	}
	if flags.NArg() != 0 {
		return "", false, errors.New("unexpected positional arguments")
	}
	return configPath, showVersion, nil
}

func printUsage(output io.Writer, name, defaultPath string) {
	_, _ = fmt.Fprintf(output, "Usage:\n  %s [--config path]\n\nOptions:\n  --config string  TOML configuration file (default %q)\n  --version        print version\n", name, defaultPath)
}
