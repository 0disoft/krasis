//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

const pipeName = `\\.\pipe\Krasis.Input.v1`

// AcceptAgent is a bounded, local-only, single-instance service endpoint. No
// frame is read before the exact agent process has been authenticated.
func AcceptAgent(ctx context.Context, agentDigest [sha256.Size]byte) (*Channel, error) {
	return acceptAgentReady(ctx, agentDigest, nil, nil)
}

func acceptAgentReady(ctx context.Context, agentDigest [sha256.Size]byte, ready func() error, installation *Installation) (*Channel, error) {
	return acceptAgentReadyOnPipe(ctx, agentDigest, ready, installation, nil)
}

func acceptAgentReadyOnPipe(ctx context.Context, agentDigest [sha256.Size]byte, ready func() error, installation *Installation, initial *pipeConn) (*Channel, error) {
	accepted := false
	defer func() {
		if initial != nil && !accepted {
			_ = initial.Close()
		}
	}()
	if ctx == nil || ctx.Err() != nil || agentDigest == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, ErrAuthority
	}
	allowed := self.system && self.session == 0 && !self.restricted && !self.appContainer
	_ = self.close()
	if !allowed {
		return nil, ErrAuthority
	}
	pipe := initial
	if pipe == nil {
		pipe, err = makePipe(pipeName)
		if err != nil {
			return nil, ErrAuthority
		}
	}
	// Persist readiness only after this service owns the first pipe instance,
	// and before authenticating or consuming any agent lease/input.
	if ready != nil && ready() != nil {
		_ = pipe.Close()
		return nil, ErrAuthority
	}
	if err := pipe.connect(ctx); err != nil {
		_ = pipe.Close()
		return nil, ErrAuthority
	}
	proof, err := authenticateAgentInstalled(uintptr(pipe.handle), agentDigest, installation)
	if err != nil {
		_ = pipe.Close()
		return nil, ErrAuthority
	}
	accepted = true
	return &Channel{stream: pipe, authority: proof}, nil
}

// ConnectService trusts neither the pipe name nor a claimed server identity.
// The service proof is constructed only after checking the real kernel PID.
func ConnectService(ctx context.Context, serviceDigest [sha256.Size]byte) (*Channel, error) {
	return connectInstalledService(ctx, serviceDigest, nil)
}

func connectInstalledService(ctx context.Context, serviceDigest [sha256.Size]byte, installation *Installation) (*Channel, error) {
	if ctx == nil || ctx.Err() != nil || serviceDigest == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	started := time.Now()
	pipe, err := openServicePipe(ctx)
	handoff.Record(ctx, handoff.PipeOpen, time.Since(started))
	if err != nil {
		return nil, connectionFailed("pipe_open", err)
	}
	started = time.Now()
	proof, err := authenticateServiceInstalled(ctx, uintptr(pipe.handle), serviceDigest, installation)
	handoff.Record(ctx, handoff.ServiceAuth, time.Since(started))
	if err != nil {
		_ = pipe.Close()
		return nil, connectionFailed("service_config", err)
	}
	return &Channel{stream: pipe, authority: proof}, nil
}

type pipeConn struct {
	handle                      windows.Handle
	mu                          sync.Mutex
	closing                     bool
	closed                      chan struct{}
	closeDone                   chan struct{}
	active                      sync.WaitGroup
	readMu, writeMu             sync.Mutex
	readDeadline, writeDeadline time.Time
	closeErr                    error
}

func makePipe(name string) (*pipeConn, error) {
	return makePipeACL(name, "D:P(A;;GA;;;SY)(A;;0x0012019B;;;IU)")
}

func makePipeACL(name, dacl string) (*pipeConn, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, ErrAuthority
	}
	// Give interactive users stream and attribute access. Do not grant
	// FILE_CREATE_PIPE_INSTANCE (the FILE_APPEND_DATA bit in GENERIC_WRITE).
	sd, err := windows.SecurityDescriptorFromString(dacl)
	if err != nil {
		return nil, ErrAuthority
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	handle, err := windows.CreateNamedPipe(path, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 0, &sa)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, ErrAuthority
	}
	return &pipeConn{handle: handle, closed: make(chan struct{}), closeDone: make(chan struct{})}, nil
}

func openPipe(ctx context.Context, name string) (*pipeConn, error) {
	return openPipeWaiting(ctx, name, 0)
}

// The service removes its single pipe between owned workers. Only a new
// service connection may wait briefly for that name to reappear. No lease or
// input has been sent yet; authentication still follows the successful open.
const servicePipeMissingGrace = 250 * time.Millisecond

func openServicePipe(ctx context.Context) (*pipeConn, error) {
	return openPipeWaiting(ctx, pipeName, servicePipeMissingGrace)
}

func openPipeWaiting(ctx context.Context, name string, missingGrace time.Duration) (*pipeConn, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, ErrAuthority
	}
	return dialPipe(ctx, missingGrace, func() (*pipeConn, error) {
		access := uint32(windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE) &^ uint32(windows.FILE_APPEND_DATA)
		handle, err := windows.CreateFile(path, access, 0, nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if err != nil {
			return nil, err
		}
		return &pipeConn{handle: handle, closed: make(chan struct{}), closeDone: make(chan struct{})}, nil
	})
}

func (pipe *pipeConn) start() error {
	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if pipe.closing {
		return net.ErrClosed
	}
	pipe.active.Add(1)
	return nil
}

func (pipe *pipeConn) deadline(read bool) time.Time {
	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if read {
		return pipe.readDeadline
	}
	return pipe.writeDeadline
}

func (pipe *pipeConn) connect(ctx context.Context) error {
	if err := pipe.start(); err != nil {
		return err
	}
	defer pipe.active.Done()
	_, err := pipe.overlapped(ctx, true, nil, func(o *windows.Overlapped) error {
		err := windows.ConnectNamedPipe(pipe.handle, o)
		if err == windows.ERROR_PIPE_CONNECTED {
			return nil
		}
		return err
	})
	return err
}

func (pipe *pipeConn) Read(data []byte) (int, error) {
	pipe.readMu.Lock()
	defer pipe.readMu.Unlock()
	if err := pipe.start(); err != nil {
		return 0, err
	}
	defer pipe.active.Done()
	if len(data) == 0 {
		return 0, nil
	}
	n, err := pipe.overlapped(context.Background(), true, data, func(o *windows.Overlapped) error {
		// x/sys inspects this pointer when race instrumentation is enabled.
		// Overlapped completion remains the authoritative transferred count.
		var immediate uint32
		return windows.ReadFile(pipe.handle, data, &immediate, o)
	})
	if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_PIPE_NOT_CONNECTED {
		return int(n), io.EOF
	}
	if err == nil && n == 0 {
		return 0, io.EOF
	}
	return int(n), err
}

func (pipe *pipeConn) Write(data []byte) (int, error) {
	pipe.writeMu.Lock()
	defer pipe.writeMu.Unlock()
	if err := pipe.start(); err != nil {
		return 0, err
	}
	defer pipe.active.Done()
	if len(data) == 0 {
		return 0, nil
	}
	n, err := pipe.overlapped(context.Background(), false, data, func(o *windows.Overlapped) error {
		var immediate uint32
		return windows.WriteFile(pipe.handle, data, &immediate, o)
	})
	if err == nil && int(n) != len(data) {
		return int(n), io.ErrShortWrite
	}
	return int(n), err
}

func (pipe *pipeConn) overlapped(ctx context.Context, read bool, data []byte, begin func(*windows.Overlapped) error) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline := pipe.deadline(read)
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return 0, os.ErrDeadlineExceeded
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, ErrProtocol
	}
	defer windows.CloseHandle(event)
	overlap := windows.Overlapped{HEvent: event}
	var pinned runtime.Pinner
	pinned.Pin(&overlap)
	if len(data) > 0 {
		pinned.Pin(&data[0])
	}
	defer pinned.Unpin()
	err = begin(&overlap)
	var transferred uint32
	if err == nil {
		// ConnectNamedPipe can complete via ERROR_PIPE_CONNECTED without an
		// overlapped operation. There are no transferred bytes for connect.
		if data == nil {
			return 0, nil
		}
		if err := windows.GetOverlappedResult(pipe.handle, &overlap, &transferred, false); err != nil {
			return 0, ErrProtocol
		}
		return transferred, nil
	}
	if err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	for {
		var stop error
		if ctx.Err() != nil {
			stop = ctx.Err()
		}
		select {
		case <-pipe.closed:
			stop = net.ErrClosed
		default:
		}
		deadline := pipe.deadline(read)
		if !deadline.IsZero() && !time.Now().Before(deadline) && stop == nil {
			stop = os.ErrDeadlineExceeded
		}
		if stop != nil {
			_ = windows.CancelIoEx(pipe.handle, &overlap)
			// Cancellation is asynchronous. Do not unpin or reuse the buffer
			// until the kernel has completed the exact submitted operation.
			_ = windows.GetOverlappedResult(pipe.handle, &overlap, &transferred, true)
			return transferred, stop
		}
		state, err := windows.WaitForSingleObject(event, 20)
		if err != nil || (state != uint32(windows.WAIT_TIMEOUT) && state != windows.WAIT_OBJECT_0) {
			_ = windows.CancelIoEx(pipe.handle, &overlap)
			_ = windows.GetOverlappedResult(pipe.handle, &overlap, &transferred, true)
			return transferred, ErrProtocol
		}
		if state == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		err = windows.GetOverlappedResult(pipe.handle, &overlap, &transferred, false)
		if err == windows.ERROR_OPERATION_ABORTED {
			return transferred, net.ErrClosed
		}
		if err != nil && err != windows.ERROR_BROKEN_PIPE && err != windows.ERROR_PIPE_NOT_CONNECTED {
			return transferred, ErrProtocol
		}
		return transferred, err
	}
}

func (pipe *pipeConn) Close() error {
	pipe.mu.Lock()
	if pipe.closing {
		pipe.mu.Unlock()
		<-pipe.closeDone
		pipe.mu.Lock()
		defer pipe.mu.Unlock()
		return pipe.closeErr
	}
	pipe.closing = true
	close(pipe.closed)
	pipe.mu.Unlock()
	_ = windows.CancelIoEx(pipe.handle, nil)
	pipe.active.Wait()
	err := windows.CloseHandle(pipe.handle)
	pipe.mu.Lock()
	if err != nil {
		pipe.closeErr = net.ErrClosed
	}
	pipe.mu.Unlock()
	close(pipe.closeDone)
	return pipe.closeErr
}

func (pipe *pipeConn) SetDeadline(value time.Time) error {
	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if pipe.closing {
		return net.ErrClosed
	}
	pipe.readDeadline, pipe.writeDeadline = value, value
	return nil
}
func (pipe *pipeConn) SetReadDeadline(value time.Time) error {
	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if pipe.closing {
		return net.ErrClosed
	}
	pipe.readDeadline = value
	return nil
}
func (pipe *pipeConn) SetWriteDeadline(value time.Time) error {
	pipe.mu.Lock()
	defer pipe.mu.Unlock()
	if pipe.closing {
		return net.ErrClosed
	}
	pipe.writeDeadline = value
	return nil
}

type pipeAddress struct{}

func (pipeAddress) Network() string    { return "krasis-local-pipe" }
func (pipeAddress) String() string     { return "local-input-service" }
func (*pipeConn) LocalAddr() net.Addr  { return pipeAddress{} }
func (*pipeConn) RemoteAddr() net.Addr { return pipeAddress{} }

var _ net.Conn = (*pipeConn)(nil)
