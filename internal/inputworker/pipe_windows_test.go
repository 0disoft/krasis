//go:build windows

package inputworker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func nativePair(t *testing.T) (*pipeConn, *pipeConn) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\Krasis.Input.Test.%d.%s`, os.Getpid(), t.Name())
	server, err := makePipe(name)
	if err != nil {
		t.Fatal("native test pipe creation failed")
	}
	t.Cleanup(func() { _ = server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.connect(ctx) }()
	client, err := openPipe(ctx, name)
	if err != nil {
		var code syscall.Errno
		_ = errors.As(err, &code)
		t.Fatalf("native test pipe connect failed: Win32=%d", uint32(code))
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := <-done; err != nil {
		t.Fatal("native test pipe accept failed")
	}
	return server, client
}

func TestNativePipeBytesAndKernelProcessMetadata(t *testing.T) {
	server, client := nativePair(t)
	var clientPID, serverPID uint32
	if windows.GetNamedPipeClientProcessId(server.handle, &clientPID) != nil || windows.GetNamedPipeServerProcessId(client.handle, &serverPID) != nil || clientPID != windows.GetCurrentProcessId() || serverPID != clientPID {
		t.Fatal("kernel pipe PID metadata mismatch")
	}
	if err := server.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Write([]byte{1, 2, 3}); done <- err }()
	var data [3]byte
	if _, err := io.ReadFull(server, data[:]); err != nil || data != [3]byte{1, 2, 3} {
		t.Fatal("native byte stream failed")
	}
	if err := <-done; err != nil {
		t.Fatal("native write failed")
	}
}

func TestNativePipeReadDeadlineCancelsPendingIO(t *testing.T) {
	server, client := nativePair(t)
	if err := server.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if _, err := server.Read(data[:]); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("native pending read deadline not respected")
	}
	if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Write([]byte{7}); done <- err }()
	if _, err := io.ReadFull(server, data[:]); err != nil || data[0] != 7 {
		t.Fatal("stream did not recover after canceled read")
	}
	if err := <-done; err != nil {
		t.Fatal("native post-cancel write failed")
	}
}

func TestNativePipeCloseDrainsPendingReadOnce(t *testing.T) {
	server, _ := nativePair(t)
	done := make(chan error, 1)
	pending := make(chan struct{})
	go func() {
		if err := server.start(); err != nil {
			done <- err
			return
		}
		defer server.active.Done()
		var data [1]byte
		_, err := server.overlapped(context.Background(), true, data[:], func(o *windows.Overlapped) error {
			var immediate uint32
			err := windows.ReadFile(server.handle, data[:], &immediate, o)
			if err == windows.ERROR_IO_PENDING {
				close(pending)
			}
			return err
		})
		done <- err
	}()
	select {
	case <-pending:
	case <-done:
		t.Fatal("native read did not enter pending state")
	case <-time.After(time.Second):
		t.Fatal("native read was not submitted")
	}
	var closers sync.WaitGroup
	for range 2 {
		closers.Go(func() {
			if err := server.Close(); err != nil {
				t.Error("native close failed")
			}
		})
	}
	closers.Wait()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatal("pending read survived close")
		}
	case <-time.After(time.Second):
		t.Fatal("native close leaked reader")
	}
}

func TestNativePipeFirstInstanceCannotBeDuplicated(t *testing.T) {
	server, _ := nativePair(t)
	name := fmt.Sprintf(`\\.\pipe\Krasis.Input.Test.%d.%s`, os.Getpid(), t.Name())
	if duplicate, err := makePipe(name); err == nil {
		_ = duplicate.Close()
		t.Fatal("second first-instance server accepted")
	}
	if err := server.Close(); err != nil {
		t.Fatal("native first server close failed")
	}
}

func TestPublicPipeEndpointsRejectMissingAuthorityBeforeConnecting(t *testing.T) {
	if channel, err := AcceptAgent(context.Background(), [32]byte{}); channel != nil || err != ErrAuthority {
		t.Fatal("zero agent pin accepted")
	}
	if channel, err := ConnectService(context.Background(), [32]byte{}); channel != nil || err != ErrAuthority {
		t.Fatal("zero service pin accepted")
	}
}
