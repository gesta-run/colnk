package transport

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	"github.com/hashicorp/yamux"
)

type Conn struct {
	raw     net.Conn
	session *yamux.Session
	ctx     context.Context
	cancel  context.CancelFunc
	once    sync.Once
	policy  protocol.NetworkPolicy
}

type Stream interface {
	io.ReadWriteCloser
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
}

type stream struct {
	raw  *yamux.Stream
	once sync.Once
}

func newConn(raw net.Conn, client bool) (*Conn, error) {
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	config.MaxStreamWindowSize = 8 << 20
	var session *yamux.Session
	var err error
	if client {
		session, err = yamux.Client(raw, config)
	} else {
		session, err = yamux.Server(raw, config)
	}
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	connection := &Conn{raw: raw, session: session, ctx: ctx, cancel: cancel}
	go func() {
		<-session.CloseChan()
		cancel()
	}()
	return connection, nil
}

func (c *Conn) OpenStreamSync(ctx context.Context) (Stream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	value, err := c.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &stream{raw: value}, nil
}

func (c *Conn) AcceptStream(ctx context.Context) (Stream, error) {
	value, err := c.session.AcceptStreamWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return &stream{raw: value}, nil
}

func (c *Conn) Close() error {
	var err error
	c.once.Do(func() {
		c.cancel()
		err = c.session.Close()
		_ = c.raw.Close()
	})
	return err
}

func (c *Conn) Context() context.Context {
	return c.ctx
}

func (c *Conn) NetworkPolicy() protocol.NetworkPolicy {
	return c.policy
}

func (s *stream) Read(data []byte) (int, error)  { return s.raw.Read(data) }
func (s *stream) Write(data []byte) (int, error) { return s.raw.Write(data) }
func (s *stream) Close() error {
	var err error
	s.once.Do(func() { err = s.raw.Close() })
	return err
}

func (s *stream) SetDeadline(deadline time.Time) error { return s.raw.SetDeadline(deadline) }
func (s *stream) SetReadDeadline(deadline time.Time) error {
	return s.raw.SetReadDeadline(deadline)
}
