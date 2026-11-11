//go:build linux

package winlifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/godbus/dbus/v5"
)

const (
	loginName                           = "org.freedesktop.login1"
	loginPath           dbus.ObjectPath = "/org/freedesktop/login1"
	loginManager                        = loginName + ".Manager"
	loginSession                        = loginName + ".Session"
	propertiesInterface                 = "org.freedesktop.DBus.Properties"
	busName                             = "org.freedesktop.DBus"
	busPath             dbus.ObjectPath = "/org/freedesktop/DBus"
	linuxCallBudget                     = 2 * time.Second
)

type loginBus interface {
	call(context.Context, string, dbus.ObjectPath, string, ...any) ([]any, error)
	match(context.Context, ...dbus.MatchOption) error
	signals() <-chan *dbus.Signal
	done() <-chan struct{}
	close() error
}

// Linux requires a systemd-logind session on the real system bus. There is no
// unmonitored fallback for headless hosts or other session managers.
type LinuxMonitor struct {
	bus              loginBus
	locker           *desktopLocker
	owner, sessionID string
	session          dbus.ObjectPath
	uid              uint32
	initial          agentlifecycle.Event
	events           chan agentlifecycle.Event
	done             chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
	once             sync.Once
	err              error // Written by run, read only after done closes.
}

func StartMonitor() (agentlifecycle.Monitor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bus, err := connectLoginBus(ctx)
	if err != nil {
		return nil, fmt.Errorf("Linux lifecycle requires the system logind bus: %w", err)
	}
	monitor, err := startLinuxMonitor(ctx, bus, os.Getenv("XDG_SESSION_ID"), uint32(os.Getpid()), uint32(os.Geteuid()), os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		_ = bus.close()
		return nil, err
	}
	return monitor, nil
}

func startLinuxMonitor(ctx context.Context, bus loginBus, sessionID string, pid, uid uint32, sessionBusAddress string) (*LinuxMonitor, error) {
	m := &LinuxMonitor{bus: bus, uid: uid, events: make(chan agentlifecycle.Event, 16), done: make(chan struct{})}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	fail := func(err error) (*LinuxMonitor, error) {
		m.cancel()
		if m.locker != nil {
			err = errors.Join(err, m.locker.close())
			m.locker = nil
		}
		return nil, err
	}
	// Subscribe before resolving the unique owner; service replacement during
	// setup must be visible, not mistaken for continuity of the old monitor.
	if err := bus.match(ctx, dbus.WithMatchSender(busName), dbus.WithMatchObjectPath(busPath), dbus.WithMatchInterface(busName), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, loginName)); err != nil {
		return fail(err)
	}
	body, err := m.call(ctx, busName, busPath, busName+".GetNameOwner", loginName)
	if err != nil || dbus.Store(body, &m.owner) != nil || !strings.HasPrefix(m.owner, ":") {
		return fail(errors.Join(errors.New("logind owner lookup failed"), err))
	}
	for _, rule := range [][]dbus.MatchOption{
		{dbus.WithMatchSender(m.owner), dbus.WithMatchObjectPath(loginPath), dbus.WithMatchInterface(loginManager)},
		{dbus.WithMatchSender(m.owner), dbus.WithMatchPathNamespace("/org/freedesktop/login1/session"), dbus.WithMatchInterface(loginSession)},
		{dbus.WithMatchSender(m.owner), dbus.WithMatchPathNamespace("/org/freedesktop/login1/session"), dbus.WithMatchInterface(propertiesInterface), dbus.WithMatchMember("PropertiesChanged")},
	} {
		if err := bus.match(ctx, rule...); err != nil {
			return fail(err)
		}
	}
	method, argument := loginManager+".GetSessionByPID", any(pid)
	if sessionID != "" {
		if len(sessionID) > 128 || strings.ContainsAny(sessionID, "/\\\x00\r\n\t ") {
			return fail(errors.New("invalid XDG_SESSION_ID"))
		}
		method, argument = loginManager+".GetSession", sessionID
	}
	body, err = m.call(ctx, m.owner, loginPath, method, argument)
	if err != nil || dbus.Store(body, &m.session) != nil || !m.session.IsValid() || !strings.HasPrefix(string(m.session), "/org/freedesktop/login1/session/") {
		return fail(errors.Join(errors.New("current logind session lookup failed"), err))
	}
	state, err := m.sessionState(ctx)
	if err != nil {
		return fail(err)
	}
	if sessionID != "" && m.sessionID != sessionID {
		return fail(errors.New("logind returned a different session"))
	}
	manager, err := m.getAll(ctx, loginPath, loginManager)
	if err != nil {
		return fail(err)
	}
	sleeping, sleepOK := manager["PreparingForSleep"].Value().(bool)
	shutdown, shutdownOK := manager["PreparingForShutdown"].Value().(bool)
	if !sleepOK || !shutdownOK {
		return fail(errors.New("logind preparation state is unavailable"))
	}
	if shutdown {
		state = agentlifecycle.EventShutdown
	} else if sleeping {
		state = agentlifecycle.EventSuspend
	}
	locker, err := startDesktopLocker(ctx, sessionBusAddress)
	if err != nil {
		return fail(err)
	}
	m.locker = locker
	if state != "" {
		m.initial = state
		m.events <- state
	}
	go m.run()
	return m, nil
}

// LinuxSessionReady takes authoritative logind and desktop-locker snapshots for
// the opt-in graphical supervisor. It never starts or resumes a product session.
// Callers must run it again immediately before a side effect and wait for a
// later safe transition after any failure.
func LinuxSessionReady(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, errors.New("Linux lifecycle context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	sessionBusAddress := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if sessionBusAddress == "" {
		return false, errors.New("Linux watch requires a graphical session bus")
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	bus, err := connectLoginBus(bounded)
	if err != nil {
		return false, fmt.Errorf("Linux lifecycle requires the system logind bus: %w", err)
	}
	return probeLinuxSession(bounded, ctx, bus, os.Getenv("XDG_SESSION_ID"), uint32(os.Getpid()), uint32(os.Geteuid()), sessionBusAddress)
}

func probeLinuxSession(startContext, cancelContext context.Context, bus loginBus, sessionID string, pid, uid uint32, sessionBusAddress string) (bool, error) {
	monitor, err := startLinuxMonitor(startContext, bus, sessionID, pid, uid, sessionBusAddress)
	if err != nil {
		_ = bus.close()
		return false, err
	}
	ready := monitor.initial == ""
	if ready {
		var lockerDone <-chan struct{}
		if monitor.locker != nil {
			lockerDone = monitor.locker.done()
		}
		select {
		case <-monitor.events:
			ready = false
		case <-monitor.done:
			ready = false
		case <-lockerDone:
			ready = false
		case <-cancelContext.Done():
			_ = monitor.Close()
			return false, cancelContext.Err()
		default:
		}
	}
	if err := monitor.Close(); err != nil {
		return false, err
	}
	return ready, nil
}

func (m *LinuxMonitor) Events() <-chan agentlifecycle.Event { return m.events }
func (m *LinuxMonitor) Done() <-chan struct{}               { return m.done }
func (m *LinuxMonitor) Close() error {
	if m == nil {
		return nil
	}
	m.once.Do(m.cancel)
	<-m.done
	return m.err
}

func (m *LinuxMonitor) call(ctx context.Context, destination string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	bounded, cancel := context.WithTimeout(ctx, linuxCallBudget)
	defer cancel()
	return m.bus.call(bounded, destination, path, method, args...)
}

func (m *LinuxMonitor) getAll(ctx context.Context, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	body, err := m.call(ctx, m.owner, path, propertiesInterface+".GetAll", iface)
	var properties map[string]dbus.Variant
	if err == nil {
		err = dbus.Store(body, &properties)
	}
	return properties, err
}

func (m *LinuxMonitor) sessionState(ctx context.Context) (agentlifecycle.Event, error) {
	p, err := m.getAll(ctx, m.session, loginSession)
	if err != nil {
		return "", err
	}
	id, ok := p["Id"].Value().(string)
	userProperty, userOK := p["User"]
	var user struct {
		UID  uint32
		Path dbus.ObjectPath
	}
	if !ok || id == "" || m.sessionID != "" && id != m.sessionID || !userOK || userProperty.Value() == nil ||
		dbus.Store([]any{userProperty.Value()}, &user) != nil || user.UID != m.uid {
		return "", errors.New("logind session identity does not belong to this user")
	}
	m.sessionID = id
	for _, key := range []string{"Active", "LockedHint", "State"} {
		if _, ok := p[key]; !ok {
			return "", errors.New("logind session state is incomplete")
		}
	}
	return sessionEvent(p)
}

func sessionEvent(p map[string]dbus.Variant) (agentlifecycle.Event, error) {
	var event agentlifecycle.Event
	if v, found := p["LockedHint"]; found {
		locked, ok := v.Value().(bool)
		if !ok {
			return "", errors.New("invalid logind LockedHint")
		}
		currentSessionLocked.Store(locked)
		if locked {
			event = agentlifecycle.EventSessionLock
		}
	}
	if v, found := p["Active"]; found {
		active, ok := v.Value().(bool)
		if !ok {
			return "", errors.New("invalid logind Active state")
		}
		if !active {
			event = agentlifecycle.EventSessionDisconnect
		}
	}
	if v, found := p["State"]; found {
		state, ok := v.Value().(string)
		if !ok || state != "online" && state != "active" && state != "closing" {
			return "", errors.New("invalid logind session State")
		}
		if state == "closing" {
			event = agentlifecycle.EventLogoff
		}
	}
	return event, nil
}

func (m *LinuxMonitor) run() {
	defer close(m.done)
	defer close(m.events)
	defer m.cancel()
	defer func() { m.err = errors.Join(m.err, m.bus.close()) }()
	if m.locker != nil {
		defer func() { m.err = errors.Join(m.err, m.locker.close()) }()
	}
	// Signals provide immediate revocation; periodic authoritative reads also
	// catch invalidated properties and a session silently disappearing.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var lockerSignals <-chan *dbus.Signal
	var lockerDone <-chan struct{}
	if m.locker != nil {
		lockerSignals = m.locker.signals()
		lockerDone = m.locker.done()
	}
	for {
		var event agentlifecycle.Event
		var err error
		select {
		case <-m.ctx.Done():
			return
		case <-m.bus.done():
			if m.ctx.Err() == nil {
				m.err = errors.New("logind monitoring connection stopped")
			}
			return
		case <-lockerDone:
			if m.ctx.Err() == nil {
				m.err = errors.New("desktop locker monitoring connection stopped")
			}
			return
		case signal, ok := <-m.bus.signals():
			if !ok {
				m.err = errors.New("logind signal stream closed")
				return
			}
			event, err = m.signalEvent(signal)
		case signal, ok := <-lockerSignals:
			if !ok {
				m.err = errors.New("desktop locker signal stream closed")
				return
			}
			event, err = m.locker.signalEvent(signal)
		case <-ticker.C:
			event, err = m.sessionState(m.ctx)
			if err == nil && event == "" {
				// A locker can stall while acquiring its grab, before publishing
				// ActiveChanged. A failed bounded read must also revoke input.
				event, err = m.locker.state(m.ctx)
			}
		}
		if err != nil {
			if m.ctx.Err() == nil {
				m.err = err
			}
			return
		}
		if event != "" {
			select {
			case m.events <- event:
			default:
				m.err = errors.New("logind lifecycle event queue overflow")
				return
			}
		}
	}
}

func (m *LinuxMonitor) signalEvent(s *dbus.Signal) (agentlifecycle.Event, error) {
	if s == nil {
		return "", errors.New("invalid logind signal")
	}
	if s.Sender == busName && s.Path == busPath && s.Name == busName+".NameOwnerChanged" {
		var name, old, next string
		if dbus.Store(s.Body, &name, &old, &next) != nil {
			return "", errors.New("invalid bus owner signal")
		}
		if name == loginName && next != m.owner {
			return "", errors.New("logind service owner changed")
		}
		return "", nil
	}
	if s.Sender != m.owner {
		return "", nil
	}
	if s.Path == loginPath {
		switch s.Name {
		case loginManager + ".PrepareForSleep", loginManager + ".PrepareForShutdown":
			var preparing bool
			if dbus.Store(s.Body, &preparing) != nil {
				return "", errors.New("invalid logind preparation signal")
			}
			if s.Name == loginManager+".PrepareForSleep" {
				if preparing {
					return agentlifecycle.EventSuspend, nil
				}
				return agentlifecycle.EventResume, nil
			}
			if preparing {
				return agentlifecycle.EventShutdown, nil
			}
		case loginManager + ".SessionRemoved":
			var id string
			var path dbus.ObjectPath
			if dbus.Store(s.Body, &id, &path) != nil {
				return "", errors.New("invalid logind session removal signal")
			}
			if id == m.sessionID || path == m.session {
				return agentlifecycle.EventLogoff, nil
			}
		}
		return "", nil
	}
	if s.Path != m.session {
		return "", nil
	}
	switch s.Name {
	case loginSession + ".Lock", loginSession + ".Unlock":
		if len(s.Body) != 0 {
			return "", errors.New("invalid logind lock signal")
		}
		locked := s.Name == loginSession+".Lock"
		currentSessionLocked.Store(locked)
		if locked {
			return agentlifecycle.EventSessionLock, nil
		}
		return agentlifecycle.EventDesktopSwitch, nil
	case propertiesInterface + ".PropertiesChanged":
		var iface string
		var changed map[string]dbus.Variant
		var invalidated []string
		if dbus.Store(s.Body, &iface, &changed, &invalidated) != nil {
			return "", errors.New("invalid logind properties signal")
		}
		if iface != loginSession {
			return "", nil
		}
		for _, key := range invalidated {
			if key == "Active" || key == "LockedHint" || key == "State" || key == "User" || key == "Id" {
				return m.sessionState(m.ctx)
			}
		}
		if _, ok := changed["User"]; ok {
			return m.sessionState(m.ctx)
		}
		if _, ok := changed["Id"]; ok {
			return m.sessionState(m.ctx)
		}
		return sessionEvent(changed)
	}
	return "", nil
}

// The default godbus handler starts deferred goroutines when its consumer is
// slow. A bounded handler instead terminates monitoring if any signal is lost.
type loginSignals struct {
	queue   chan *dbus.Signal
	stopped chan struct{}
	once    sync.Once
}

func (s *loginSignals) DeliverSignal(_, _ string, signal *dbus.Signal) {
	select {
	case s.queue <- signal:
	default:
		s.Terminate()
	}
}
func (s *loginSignals) Terminate() { s.once.Do(func() { close(s.stopped) }) }

type systemLoginBus struct {
	conn    *dbus.Conn
	handler *loginSignals
}

func (b *systemLoginBus) signals() <-chan *dbus.Signal { return b.handler.queue }
func (b *systemLoginBus) done() <-chan struct{}        { return b.handler.stopped }
func (b *systemLoginBus) close() error                 { return b.conn.Close() }
func (b *systemLoginBus) call(ctx context.Context, destination string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	call := b.conn.Object(destination, path).CallWithContext(ctx, method, 0, args...)
	return call.Body, call.Err
}
func (b *systemLoginBus) match(ctx context.Context, options ...dbus.MatchOption) error {
	bounded, cancel := context.WithTimeout(ctx, linuxCallBudget)
	defer cancel()
	return b.conn.AddMatchSignalContext(bounded, options...)
}

func connectLoginBus(ctx context.Context) (loginBus, error) {
	// Ignore DBUS_SYSTEM_BUS_ADDRESS: lifecycle authority is the host system bus,
	// never a caller-selected session bus. Bound authentication as well as calls.
	raw, err := (&net.Dialer{Timeout: linuxCallBudget}).DialContext(ctx, "unix", "/run/dbus/system_bus_socket")
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(linuxCallBudget)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := raw.SetDeadline(deadline); err != nil {
		raw.Close()
		return nil, err
	}
	handler := &loginSignals{queue: make(chan *dbus.Signal, 64), stopped: make(chan struct{})}
	conn, err := dbus.NewConn(raw, dbus.WithSignalHandler(handler))
	if err != nil {
		raw.Close()
		return nil, err
	}
	if err := conn.Auth([]dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Geteuid()))}); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return &systemLoginBus{conn: conn, handler: handler}, nil
}
