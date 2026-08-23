package remote

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type contextTestStream struct {
	closed chan struct{}
	once   sync.Once
}

func (s *contextTestStream) Read([]byte) (int, error)        { return 0, io.EOF }
func (s *contextTestStream) Write(data []byte) (int, error)  { return len(data), nil }
func (s *contextTestStream) SetDeadline(time.Time) error     { return nil }
func (s *contextTestStream) SetReadDeadline(time.Time) error { return nil }
func (s *contextTestStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestBindStreamContextClosesCanceledStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := &contextTestStream{closed: make(chan struct{})}
	stop := bindStreamContext(ctx, stream)
	defer stop()
	cancel()
	select {
	case <-stream.closed:
	case <-time.After(time.Second):
		t.Fatal("canceled context did not close stream")
	}
}
