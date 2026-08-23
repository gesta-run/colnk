//go:build linux

package network

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

const soOriginalDst = 80

func (r *networkRuntime) acceptLoop(ctx context.Context) {
	for {
		connection, err := r.listener.Accept()
		if err != nil {
			return
		}
		tcpConnection := connection.(*net.TCPConn)
		select {
		case r.tcpSlots <- struct{}{}:
			go func() {
				defer func() { <-r.tcpSlots }()
				r.proxyConnection(ctx, tcpConnection)
			}()
		default:
			_ = tcpConnection.Close()
		}
	}
}

func (r *networkRuntime) proxyConnection(ctx context.Context, localConnection *net.TCPConn) {
	defer localConnection.Close()
	if !interfaceIsUp(r.config.InterfaceName) {
		r.logger.Warn("reject transparent connection while local interface is down")
		return
	}
	target, err := originalDestination(localConnection)
	if err != nil {
		r.logger.Warn("read original destination", "error", err)
		return
	}
	remoteStream, err := r.remote.OpenTCP(ctx, target)
	if err != nil {
		r.logger.Warn("open local tcp connection", "error", err)
		return
	}
	defer remoteStream.Close()
	done := make(chan struct{}, 2)
	go copyNetwork(remoteStream, localConnection, done)
	go copyNetwork(localConnection, remoteStream, done)
	<-done
}

func interfaceIsUp(name string) bool {
	device, err := net.InterfaceByName(name)
	return err == nil && device.Flags&net.FlagUp != 0
}

func originalDestination(connection *net.TCPConn) (string, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return "", err
	}
	var address [16]byte
	var operationError error
	err = raw.Control(func(fd uintptr) {
		size := uint32(len(address))
		_, _, callErr := unix.Syscall6(
			unix.SYS_GETSOCKOPT, fd, unix.SOL_IP, soOriginalDst,
			uintptr(unsafe.Pointer(&address[0])), uintptr(unsafe.Pointer(&size)), 0,
		)
		if callErr != 0 {
			operationError = callErr
		}
	})
	if err != nil {
		return "", err
	}
	if operationError != nil {
		return "", operationError
	}
	port := binary.BigEndian.Uint16(address[2:4])
	ip := net.IPv4(address[4], address[5], address[6], address[7])
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(port))), nil
}

func copyNetwork(destination io.Writer, source io.Reader, done chan<- struct{}) {
	_, _ = io.Copy(destination, source)
	done <- struct{}{}
}
