//go:build linux

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
	"golang.org/x/sync/semaphore"
)

func Mount(ctx context.Context, remote *remote.Remote, mountpoint string, allowOther bool, metadataTTL time.Duration) (func() error, error) {
	if metadataTTL <= 0 {
		metadataTTL = 10 * time.Second
	}
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return nil, fmt.Errorf("create mountpoint: %w", err)
	}
	options := []fuse.MountOption{
		fuse.FSName("colnk"), fuse.Subtype("colnk"),
		fuse.AsyncRead(), fuse.WritebackCache(), fuse.MaxReadahead(uint32(protocol.MaxPayloadBytes)),
	}
	if allowOther {
		options = append(options, fuse.AllowOther())
	}
	connection, err := fuse.Mount(mountpoint, options...)
	if err != nil {
		return nil, fmt.Errorf("mount fuse: %w", err)
	}
	serveErrors := make(chan error, 1)
	filesystem := &remoteFS{
		remote: remote, nodes: make(map[string]*remoteNode),
		cache: newMetadataCache(4096, metadataTTL), dataCache: newReadBlockCache(128, metadataTTL),
		writeBudget: semaphore.NewWeighted(maxTotalWriteBlocks), mountpoint: path.Clean(mountpoint),
	}
	go func() {
		serveErrors <- fs.Serve(connection, filesystem)
	}()
	var cleanupOnce sync.Once
	var cleanupError error
	cleanup := func() error {
		cleanupOnce.Do(func() {
			cleanupError = errors.Join(remote.Close(), connection.Close(), unmountFUSE(mountpoint))
		})
		return cleanupError
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = cleanup()
		case serveError := <-serveErrors:
			_ = cleanup()
			if serveError != nil && ctx.Err() == nil {
				_ = remote.Close()
			}
		}
	}()
	return cleanup, nil
}

func unmountFUSE(mountpoint string) error {
	var lastError error
	for _, option := range []string{"-u", "-uz"} {
		commandContext, cancel := context.WithTimeout(context.Background(), fuseUnmountTimeout)
		output, err := exec.CommandContext(commandContext, "fusermount3", option, "--", mountpoint).CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "not mounted") || strings.Contains(message, "No such file or directory") {
			return nil
		}
		lastError = fmt.Errorf("unmount FUSE with %s: %w: %s", option, err, message)
	}
	return lastError
}
