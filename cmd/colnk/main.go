// Command colnk provides local files and network access to a CoLnk server.
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
	"syscall"

	"github.com/gesta-run/colnk/pkg/client"
	"github.com/gesta-run/colnk/pkg/configfile"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	defaultPath, err := configfile.ClientPath()
	if err != nil {
		logger.Error("locate configuration", "error", err)
		os.Exit(2)
	}
	configPath, showVersion, err := parseArguments(os.Args[1:], defaultPath, "colnk")
	if errors.Is(err, flag.ErrHelp) {
		printUsage(os.Stdout, "colnk", defaultPath)
		return
	}
	if err != nil {
		logger.Error("parse command", "error", err)
		os.Exit(2)
	}
	if showVersion {
		_, _ = fmt.Fprintf(os.Stdout, "colnk %s\n", version)
		return
	}
	if err := client.ValidateUser(os.Geteuid()); err != nil {
		logger.Error("validate user", "error", err)
		os.Exit(2)
	}
	config, err := client.LoadConfig(configPath)
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.Run(ctx, config, logger); err != nil {
		logger.Error("run CoLnk", "error", err)
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
