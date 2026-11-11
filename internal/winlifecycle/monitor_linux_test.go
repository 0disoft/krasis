//go:build linux

package winlifecycle

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/godbus/dbus/v5"
)

const testSession dbus.ObjectPath = "/org/freedesktop/login1/session/c1"

type fakeLoginBus struct {
	queue      chan *dbus.Signal
	stopped    chan struct{}
	once       sync.Once
	properties map[string]dbus.Variant
	manager    map[string]dbus.Variant
	matches    int
	method     string
	argument   any
	callError  error
	block      bool
}

func newFakeLoginBus() *fakeLoginBus {
	return &fakeLoginBus{
		queue: make(chan *dbus.Signal, 64), stopped: make(chan struct{}),
		properties: map[string]dbus.Variant{
			"Id": dbus.MakeVariant("c1"), "User": dbus.MakeVariant([]any{uint32(1000), dbus.ObjectPath("/org/freedesktop/login1/user/_1000")}),
			"Active": dbus.MakeVariant(true), "LockedHint": dbus.MakeVariant(false), "State": dbus.MakeVariant("active"),
		},
		manager: map[string]dbus.Variant{"PreparingForSleep": dbus.MakeVariant(false), "PreparingForShutdown": dbus.MakeVariant(false)},
	}
}
func (b *fakeLoginBus) signals() <-chan *dbus.Signal { return b.queue }
func (b *fakeLoginBus) done() <-chan struct{}        { return b.stopped }
func (b *fakeLoginBus) close() error                 { b.once.Do(func() { close(b.stopped) }); return nil }
func (b *fakeLoginBus) match(ctx context.Context, _ ...dbus.MatchOption) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("unbounded subscription")
	}
	b.matches++
	return nil
}
func (b *fakeLoginBus) call(ctx context.Context, _ string, _ dbus.ObjectPath, method string, args ...any) ([]any, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("unbounded logind call")
	}
	if b.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if b.callError != nil {
		return nil, b.callError
	}
	switch method {
	case busName + ".GetNameOwner":
		return []any{":1.5"}, nil
	case loginManager + ".GetSession", loginManager + ".GetSessionByPID":
		if b.matches != 4 {
			return nil, errors.New("session lookup preceded subscriptions")
		}
		b.method, b.argument = method, args[0]
		return []any{testSession}, nil
	case propertiesInterface + ".GetAll":
		if args[0] == loginSession {
			return []any{b.properties}, nil
		}
		return []any{b.manager}, nil
	}
	return nil, errors.New("unexpected logind call")
}

func startFakeLoginMonitor(t *testing.T, b *fakeLoginBus, id string) *LinuxMonitor {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	m, err := startLinuxMonitor(ctx, b, id, 123, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestLinuxLogindInitialStateAndSessionLookup(t *testing.T) {
	for _, test := range []struct {
		name, key string
		value     any
		event     agentlifecycle.Event
	}{
		{"active", "Active", true, ""},
		{"inactive", "Active", false, agentlifecycle.EventSessionDisconnect},
		{"locked", "LockedHint", true, agentlifecycle.EventSessionLock},
		{"closing", "State", "closing", agentlifecycle.EventLogoff},
		{"sleeping", "PreparingForSleep", true, agentlifecycle.EventSuspend},
		{"shutdown", "PreparingForShutdown", true, agentlifecycle.EventShutdown},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := newFakeLoginBus()
			if _, ok := b.manager[test.key]; ok {
				b.manager[test.key] = dbus.MakeVariant(test.value)
			} else {
				b.properties[test.key] = dbus.MakeVariant(test.value)
			}
			m := startFakeLoginMonitor(t, b, "c1")
			if b.method != loginManager+".GetSession" || b.argument != "c1" {
				t.Fatal("XDG session lookup was not used")
			}
			select {
			case got := <-m.Events():
				if got != test.event {
					t.Fatalf("initial event=%s want=%s", got, test.event)
				}
			default:
				if test.event != "" {
					t.Fatal("unsafe initial state was ignored")
				}
			}
		})
	}
	b := newFakeLoginBus()
	_ = startFakeLoginMonitor(t, b, "")
	if b.method != loginManager+".GetSessionByPID" || b.argument != uint32(123) {
		t.Fatal("PID session lookup was not used")
	}
}

func TestLinuxSessionProbeUsesAuthoritativeInitialState(t *testing.T) {
	for _, test := range []struct {
		name, key string
		value     any
		ready     bool
	}{
		{"active", "Active", true, true},
		{"inactive", "Active", false, false},
		{"locked", "LockedHint", true, false},
		{"closing", "State", "closing", false},
		{"sleeping", "PreparingForSleep", true, false},
		{"shutdown", "PreparingForShutdown", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			bus := newFakeLoginBus()
			if _, ok := bus.manager[test.key]; ok {
				bus.manager[test.key] = dbus.MakeVariant(test.value)
			} else {
				bus.properties[test.key] = dbus.MakeVariant(test.value)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ready, err := probeLinuxSession(ctx, ctx, bus, "c1", 123, 1000)
			if err != nil {
				t.Fatal(err)
			}
			if ready != test.ready {
				t.Fatalf("ready=%t want=%t", ready, test.ready)
			}
		})
	}
}

func TestLinuxLogindRejectsForeignOrIncompleteInitialState(t *testing.T) {
	for _, key := range []string{"User", "Id", "Active", "LockedHint", "State"} {
		t.Run(key, func(t *testing.T) {
			b := newFakeLoginBus()
			defer b.close()
			delete(b.properties, key)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := startLinuxMonitor(ctx, b, "c1", 123, 1000); err == nil {
				t.Fatal("incomplete initial state accepted")
			}
		})
	}
	b := newFakeLoginBus()
	defer b.close()
	b.properties["User"] = dbus.MakeVariant([]any{uint32(1001), dbus.ObjectPath("/other")})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := startLinuxMonitor(ctx, b, "c1", 123, 1000); err == nil {
		t.Fatal("foreign user session accepted")
	}
}

func TestLinuxLogindSignalMapping(t *testing.T) {
	property := func(key string, value any) []any {
		return []any{loginSession, map[string]dbus.Variant{key: dbus.MakeVariant(value)}, []string{}}
	}
	for _, test := range []struct {
		name  string
		path  dbus.ObjectPath
		body  []any
		event agentlifecycle.Event
	}{
		{loginSession + ".Lock", testSession, nil, agentlifecycle.EventSessionLock},
		{loginSession + ".Unlock", testSession, nil, agentlifecycle.EventDesktopSwitch},
		{propertiesInterface + ".PropertiesChanged", testSession, property("Active", false), agentlifecycle.EventSessionDisconnect},
		{propertiesInterface + ".PropertiesChanged", testSession, property("LockedHint", true), agentlifecycle.EventSessionLock},
		{propertiesInterface + ".PropertiesChanged", testSession, property("State", "closing"), agentlifecycle.EventLogoff},
		{loginManager + ".PrepareForSleep", loginPath, []any{true}, agentlifecycle.EventSuspend},
		{loginManager + ".PrepareForSleep", loginPath, []any{false}, agentlifecycle.EventResume},
		{loginManager + ".PrepareForShutdown", loginPath, []any{true}, agentlifecycle.EventShutdown},
		{loginManager + ".PrepareForShutdown", loginPath, []any{false}, ""},
		{loginManager + ".SessionRemoved", loginPath, []any{"c1", testSession}, agentlifecycle.EventLogoff},
		{loginManager + ".SessionRemoved", loginPath, []any{"other", dbus.ObjectPath("/other")}, ""},
	} {
		b := newFakeLoginBus()
		m := &LinuxMonitor{bus: b, ctx: context.Background(), owner: ":1.5", session: testSession, sessionID: "c1", uid: 1000}
		got, err := m.signalEvent(&dbus.Signal{Sender: m.owner, Path: test.path, Name: test.name, Body: test.body})
		if err != nil || got != test.event {
			t.Fatalf("%s %v: event=%q error=%v want=%q", test.name, test.body, got, err, test.event)
		}
		got, err = m.signalEvent(&dbus.Signal{Sender: ":1.attacker", Path: test.path, Name: test.name, Body: test.body})
		if err != nil || got != "" {
			t.Fatal("foreign sender supplied a lifecycle event")
		}
	}
}

func TestLinuxLogindInvalidatedStateIsReadAgain(t *testing.T) {
	b := newFakeLoginBus()
	m := startFakeLoginMonitor(t, b, "c1")
	b.properties["LockedHint"] = dbus.MakeVariant(true)
	b.queue <- &dbus.Signal{Sender: m.owner, Path: testSession, Name: propertiesInterface + ".PropertiesChanged", Body: []any{loginSession, map[string]dbus.Variant{}, []string{"LockedHint"}}}
	select {
	case got := <-m.Events():
		if got != agentlifecycle.EventSessionLock {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("invalidated lock state was not refreshed")
	}
}

func TestLinuxLogindMonitorFailuresCancelTheOwner(t *testing.T) {
	for _, failure := range []string{"disconnect", "replacement", "malformed"} {
		t.Run(failure, func(t *testing.T) {
			b := newFakeLoginBus()
			m := startFakeLoginMonitor(t, b, "c1")
			owner, err := agentlifecycle.Start(context.Background(), func() (agentlifecycle.Monitor, error) { return m, nil })
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			switch failure {
			case "disconnect":
				b.close()
			case "replacement":
				b.queue <- &dbus.Signal{Sender: busName, Path: busPath, Name: busName + ".NameOwnerChanged", Body: []any{loginName, m.owner, ":1.99"}}
			case "malformed":
				b.queue <- &dbus.Signal{Sender: m.owner, Path: loginPath, Name: loginManager + ".PrepareForSleep", Body: []any{"true"}}
			}
			select {
			case <-owner.Context().Done():
			case <-time.After(time.Second):
				t.Fatal("monitor failure left owner running")
			}
			if !errors.Is(context.Cause(owner.Context()), agentlifecycle.ErrMonitorStopped) {
				t.Fatal(context.Cause(owner.Context()))
			}
			if err := m.Close(); err == nil {
				t.Fatal("monitor failure was hidden")
			}
		})
	}
}

func TestLinuxLogindCloseAndSignalOverflowAreBounded(t *testing.T) {
	b := newFakeLoginBus()
	m := startFakeLoginMonitor(t, b, "c1")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	h := &loginSignals{queue: make(chan *dbus.Signal, 1), stopped: make(chan struct{})}
	h.DeliverSignal("", "", &dbus.Signal{})
	h.DeliverSignal("", "", &dbus.Signal{})
	select {
	case <-h.stopped:
	default:
		t.Fatal("signal loss did not terminate monitoring")
	}
	b = newFakeLoginBus()
	b.block = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := startLinuxMonitor(ctx, b, "c1", 123, 1000); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked logind lookup=%v", err)
	}
}

func TestLinuxSystemLogindInitialState(t *testing.T) {
	if os.Getenv("KRASIS_TEST_SYSTEM_LOGIND") != "1" {
		t.Skip("set KRASIS_TEST_SYSTEM_LOGIND=1 in a real logind user session")
	}
	for _, lookup := range []string{"environment", "pid"} {
		t.Run(lookup, func(t *testing.T) {
			if lookup == "pid" {
				t.Setenv("XDG_SESSION_ID", "")
			}
			monitor, err := StartMonitor()
			if err != nil {
				t.Fatal(err)
			}
			defer monitor.Close()
			m := monitor.(*LinuxMonitor)
			t.Logf("system logind owner=%s session=%s path=%s uid=%d", m.owner, m.sessionID, m.session, m.uid)
			select {
			case event := <-monitor.Events():
				t.Fatalf("test session is not active/unlocked: %s", event)
			case <-monitor.Done():
				t.Fatal("real logind monitor terminated")
			case <-time.After(50 * time.Millisecond):
			}
			if err := monitor.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
