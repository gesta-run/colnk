//go:build !linux

package filesystem

import (
	"context"
	"errors"
	"time"

	"github.com/gesta-run/colnk/pkg/agent/remote"
)

func Mount(_ context.Context, _ *remote.Remote, _ string, _ bool, _ time.Duration) (func() error, error) {
	return nil, errors.New("agent FUSE mount is supported only on Linux")
}
