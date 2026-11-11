//go:build linux

package winlifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

type fakeDesktopBus struct {
	queue     chan *dbus.Signal
	stopped   chan struct{}
	once      sync.Once
	owners    map[string]string
	active    map[string]bool
	activeErr error
}

func newFakeDesktopBus() *fakeDesktopBus {
	return &fakeDesktopBus{
		queue:   make(chan *dbus.Signal, 16),
		stopped: make(chan struct{}),
		owners:  map[string]string{},
		active:  map[string]bool{},
	}
}

func TestDesktopLockerUnknownServiceAcceptsRemoteValueAndPointerErrors(t *testing.T) {
	for _, name := range []string{"ServiceUnknown", "NameHasNoOwner", "AccessDenied"} {
		value := dbus.Error{Name: sessionBusName + ".Error." + name}
		for _, err := range []error{value, &value, fmt.Errorf("query: %w", value), fmt.Errorf("query: %w", &value)} {
			if got := serviceUnknown(err); got != (name != "AccessDenied") {
				t.Fatalf("serviceUnknown(%T %s) = %v", err, name, got)
			}
		}
	}
	if serviceUnknown(nil) || serviceUnknown(errors.New("NameHasNoOwner")) {
		t.Fatal("untyped failure classified as missing service")
	}
}

func (bus *fakeDesktopBus) signals() <-chan *dbus.Signal { return bus.queue }
func (bus *fakeDesktopBus) done() <-chan struct{}        { return bus.stopped }
func (bus *fakeDesktopBus) close() error {
	bus.once.Do(func() { close(bus.stopped) })
	return nil
}
func (bus *fakeDesktopBus) match(ctx context.Context, _ ...dbus.MatchOption) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("unbounded locker subscription")
	}
	return nil
}
func (bus *fakeDesktopBus) call(ctx context.Context, destination string, _ dbus.ObjectPath, method string, args ...any) ([]any, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("unbounded locker call")
	}
	switch method {
	case sessionBusName + ".GetNameOwner":
		name, ok := args[0].(string)
		if !ok || bus.owners[name] == "" {
			return nil, &dbus.Error{Name: sessionBusName + ".Error.NameHasNoOwner"}
		}
		return []any{bus.owners[name]}, nil
	case "org.xfce.ScreenSaver.GetActive", "org.freedesktop.ScreenSaver.GetActive", "org.gnome.ScreenSaver.GetActive":
		if bus.activeErr != nil {
			return nil, bus.activeErr
		}
		return []any{bus.active[destination]}, nil
	default:
		return nil, errors.New("unexpected locker call")
	}
}

func TestDesktopLockerPinsOwnerAndFailsClosedOnActiveOrLoss(t *testing.T) {
	bus := newFakeDesktopBus()
	bus.owners["org.xfce.ScreenSaver"] = ":1.42"
	locker, err := startDesktopLockerOnBus(context.Background(), bus)
	if err != nil {
		t.Fatal(err)
	}
	if len(locker.services) != 1 || locker.services[0].owner != ":1.42" {
		t.Fatalf("service pin = %+v", locker.services)
	}
	active := &dbus.Signal{Sender: ":1.42", Path: "/org/xfce/ScreenSaver", Name: "org.xfce.ScreenSaver.ActiveChanged", Body: []any{true}}
	event, err := locker.signalEvent(active)
	if err != nil || event != agentlifecycle.EventSessionLock {
		t.Fatalf("active signal = %q, %v", event, err)
	}
	inactive := &dbus.Signal{Sender: ":1.42", Path: "/org/xfce/ScreenSaver", Name: "org.xfce.ScreenSaver.ActiveChanged", Body: []any{false}}
	event, err = locker.signalEvent(inactive)
	if err != nil || event != "" {
		t.Fatalf("inactive signal = %q, %v", event, err)
	}
	ownerChanged := &dbus.Signal{Sender: sessionBusName, Path: sessionBusPath, Name: sessionBusName + ".NameOwnerChanged", Body: []any{"org.xfce.ScreenSaver", ":1.42", ":1.43"}}
	if _, err := locker.signalEvent(ownerChanged); err == nil {
		t.Fatal("desktop locker owner replacement was accepted")
	}
}

func TestDesktopLockerSnapshotRevokesWhenSignalsAreMissingOrQueryStalls(t *testing.T) {
	bus := newFakeDesktopBus()
	bus.owners["org.xfce.ScreenSaver"] = ":1.42"
	locker, err := startDesktopLockerOnBus(context.Background(), bus)
	if err != nil {
		t.Fatal(err)
	}
	if event, err := locker.state(context.Background()); event != "" || err != nil {
		t.Fatalf("unlocked snapshot = %q, %v", event, err)
	}
	bus.active[":1.42"] = true
	if event, err := locker.state(context.Background()); event != agentlifecycle.EventSessionLock || err != nil {
		t.Fatalf("silent active snapshot = %q, %v", event, err)
	}
	bus.activeErr = context.DeadlineExceeded
	if _, err := locker.state(context.Background()); err == nil {
		t.Fatal("unresponsive locker was accepted as unlocked")
	}
}

func TestDesktopLockerRejectsActiveMissingOrUnreadableService(t *testing.T) {
	for _, test := range []struct {
		name    string
		owner   string
		active  bool
		callErr error
	}{
		{name: "active", owner: ":1.42", active: true},
		{name: "missing service"},
		{name: "unreadable", owner: ":1.42", callErr: errors.New("unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus := newFakeDesktopBus()
			if test.owner != "" {
				bus.owners["org.xfce.ScreenSaver"] = test.owner
				bus.active[test.owner] = test.active
				bus.activeErr = test.callErr
			}
			if locker, err := startDesktopLockerOnBus(context.Background(), bus); err == nil {
				t.Fatalf("unsafe service accepted: %+v", locker)
			}
		})
	}
}

func TestDesktopLockerFencesSupportedServiceAppearingAfterSetup(t *testing.T) {
	bus := newFakeDesktopBus()
	bus.owners["org.xfce.ScreenSaver"] = ":1.42"
	locker, err := startDesktopLockerOnBus(context.Background(), bus)
	if err != nil {
		t.Fatal(err)
	}
	signal := &dbus.Signal{
		Sender: sessionBusName,
		Path:   sessionBusPath,
		Name:   sessionBusName + ".NameOwnerChanged",
		Body:   []any{"org.gnome.ScreenSaver", "", ":1.77"},
	}
	if _, err := locker.signalEvent(signal); err == nil {
		t.Fatal("newly appearing supported locker was ignored")
	}
}

func TestSessionBusAddressAcceptsOnlyBoundedUnixTransports(t *testing.T) {
	for _, address := range []string{
		"unix:path=/run/user/1000/bus",
		"unix:path=/run/user/1000/bus,guid=0123456789abcdef0123456789abcdef",
		"unix:abstract=/tmp/krasis-dbus,guid=0123456789abcdef0123456789abcdef",
	} {
		if _, err := parseSessionBusAddress(address); err != nil {
			t.Fatalf("valid address %q: %v", address, err)
		}
	}
	for _, address := range []string{
		"tcp:host=127.0.0.1,port=1234",
		"unix:path=/tmp/one;unix:path=/tmp/two",
		"unix:path=relative",
		"unix:path=/tmp/one,abstract=other",
		"unix:dir=/tmp",
		"unix:path=/tmp/one,unknown=value",
		"unix:path=/tmp/one,guid=not-a-valid-guid-000000000000",
		"unix:path=/tmp/one,guid=0123456789abcdef0123456789abcdef,guid=0123456789abcdef0123456789abcdef",
	} {
		if _, err := parseSessionBusAddress(address); err == nil {
			t.Fatalf("unsafe address accepted: %q", address)
		}
	}
}

func TestSessionBusPeerCredentialsRequireUnixSocket(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	if err := sameUserUnixPeer(left); err == nil {
		t.Fatal("non-Unix peer accepted")
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftFile := os.NewFile(uintptr(fds[0]), "peer-left")
	rightFile := os.NewFile(uintptr(fds[1]), "peer-right")
	defer leftFile.Close()
	defer rightFile.Close()
	raw, err := net.FileConn(leftFile)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := sameUserUnixPeer(raw); err != nil {
		t.Fatalf("same-user Unix peer rejected: %v", err)
	}
}
