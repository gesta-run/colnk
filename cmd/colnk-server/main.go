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

	"github.com/gesta-run/colnk/pkg/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config, err := server.ParseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(os.Stdout, server.Usage)
			return
		}
		logger.Error("parse command", "error", err)
		os.Exit(2)
	}
	if err := server.ResolveConfig(&config); err != nil {
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
