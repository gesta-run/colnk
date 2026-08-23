// Command colnk provides Mac files and network access to a CoLnk server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gesta-run/colnk/pkg/client"
)

const usage = `Usage:
  colnk start [options]   provide local files and network access

Run "colnk start --help" for options.
`

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if len(os.Args) == 2 && (os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h") {
		_, _ = fmt.Fprint(os.Stdout, usage)
		return
	}
	if err := client.ValidateUser(os.Geteuid()); err != nil {
		logger.Error("validate user", "error", err)
		os.Exit(2)
	}
	config, err := client.ParseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(os.Stdout, client.Usage)
			return
		}
		logger.Error("parse command", "error", err)
		os.Exit(2)
	}
	if err := client.ResolveAPIKey(&config); err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(2)
	}
	if err := client.ConfirmRisk(config, os.Stdin, os.Stdout); err != nil {
		logger.Error("confirm filesystem access", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.Run(ctx, config, logger); err != nil {
		logger.Error("run CoLnk", "error", err)
		os.Exit(1)
	}
}
