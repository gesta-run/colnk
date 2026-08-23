package remote

import (
	"context"
	"sync"
	"syscall"

	"github.com/gesta-run/colnk/pkg/transport"
)

type Remote struct {
	conn *transport.Conn
}

type RemoteError struct {
	Code    syscall.Errno
	Message string
}

func (e *RemoteError) Error() string { return e.Message }

func NewRemote(conn *transport.Conn) *Remote { return &Remote{conn: conn} }

func (r *Remote) Close() error { return r.conn.Close() }

type contextStream struct {
	transport.Stream
	stop func() bool
	once sync.Once
}

func (s *contextStream) Close() error {
	var err error
	s.once.Do(func() {
		s.stop()
		err = s.Stream.Close()
	})
	return err
}

func bindStreamContext(ctx context.Context, stream transport.Stream) func() bool {
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetDeadline(deadline)
	}
	return context.AfterFunc(ctx, func() { _ = stream.Close() })
}
