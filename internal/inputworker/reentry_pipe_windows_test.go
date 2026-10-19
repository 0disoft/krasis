//go:build windows

package inputworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReentryListenerExistsWhileOptionalChildPreparationIsPending(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\Krasis.Input.Test.%d.%s`, os.Getpid(), t.Name())
	previous, err := makePipe(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = previous.Close() })
	listener, err := rebindServiceInputPipe(&Channel{stream: previous}, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	pending := startCursorInputPreparation(ctx, func(run context.Context) (*preparedConsole, error) {
		close(started)
		select {
		case <-release:
			return nil, nil
		case <-run.Done():
			return nil, run.Err()
		}
	})
	t.Cleanup(func() { cancel(); _, _ = pending.join() })
	<-started
	// Even zero missing-name grace must suffice while the serial service
	// is still joining optional construction. No auth or lease is bypassed.
	client, err := openPipe(ctx, name)
	if err != nil {
		t.Fatal("reentry endpoint disappeared during child preparation", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var pid uint32
	if windows.GetNamedPipeServerProcessId(client.handle, &pid) != nil || pid != windows.GetCurrentProcessId() {
		t.Fatal("reserved endpoint lost its exact kernel owner")
	}
	select {
	case <-pending.done:
		t.Fatal("preparation did not remain pending")
	default:
	}
	close(release)
	if _, err := pending.join(); err != nil {
		t.Fatal(err)
	}
	// The same reserved instance proceeds through ConnectNamedPipe; it is
	// not replaced with a different listener after the client opens it.
	if err := listener.connect(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestReentryListenerCannotReplaceUnclosedOrAlreadyOwnedPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\Krasis.Input.Test.%d.%s`, os.Getpid(), t.Name())
	closed := &Channel{closeErr: ErrAuthority}
	closed.closeOnce.Do(func() {})
	if listener, err := rebindServiceInputPipe(closed, name); listener != nil || !errors.Is(err, ErrRecovery) {
		t.Fatal("uncertain prior cleanup admitted another listener", err)
	}
	owner, err := makePipe(name)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if listener, err := rebindServiceInputPipe(&Channel{}, name); listener != nil || !errors.Is(err, ErrRecovery) {
		t.Fatal("existing endpoint ownership was replaced", err)
	}
}
