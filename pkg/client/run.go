package client

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	localservice "github.com/gesta-run/colnk/pkg/provider"
	"github.com/gesta-run/colnk/pkg/transport"
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	backoff := time.Second
	for ctx.Err() == nil {
		wasConnected, err := runSession(ctx, config, logger)
		if wasConnected {
			backoff = time.Second
		}
		if ctx.Err() != nil {
			return nil
		}
		if transport.IsPermanent(err) {
			return err
		}
		logger.Warn("CoLnk disconnected; reconnecting", "error", err, "delay", backoff)
		if !waitForRetry(ctx, backoff) {
			return nil
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return nil
}

func runSession(ctx context.Context, config Config, logger *slog.Logger) (bool, error) {
	conn, ack, err := transport.Dial(ctx, transport.Config{
		Endpoint: config.Endpoint, Credential: config.APIKey, Policy: config.Policy,
	})
	if err != nil {
		return false, fmt.Errorf("connect to endpoint: %w", err)
	}
	defer conn.Close()
	service, err := localservice.NewService(config.Root, ack.Policy, logger, config.AuditResources)
	if err != nil {
		return true, fmt.Errorf("initialize local services: %w", err)
	}
	attributes := []any{"endpoint", config.Endpoint}
	if config.AuditResources {
		attributes = append(attributes, "root", config.Root)
	}
	logger.Info("CoLnk connected", attributes...)
	if err := service.Serve(ctx, conn); err != nil && ctx.Err() == nil {
		return true, fmt.Errorf("serve CoLnk: %w", err)
	}
	return true, ctx.Err()
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
