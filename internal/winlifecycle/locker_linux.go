//go:build linux

package winlifecycle

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const (
	sessionBusName = "org.freedesktop.DBus"
	sessionBusPath = dbus.ObjectPath("/org/freedesktop/DBus")
)

type screenSaverService struct {
	name, owner string
	path        dbus.ObjectPath
	iface       string
}

var screenSaverServices = []screenSaverService{
	{name: "org.xfce.ScreenSaver", path: "/org/xfce/ScreenSaver", iface: "org.xfce.ScreenSaver"},
	{name: "org.freedesktop.ScreenSaver", path: "/org/freedesktop/ScreenSaver", iface: "org.freedesktop.ScreenSaver"},
	{name: "org.gnome.ScreenSaver", path: "/org/gnome/ScreenSaver", iface: "org.gnome.ScreenSaver"},
}

type desktopLocker struct {
	bus      loginBus
	services []screenSaverService
}

func startDesktopLocker(ctx context.Context, address string) (*desktopLocker, error) {
	if address == "" {
		return nil, nil
	}
	bus, err := connectSessionLockerBus(ctx, address)
	if err != nil {
		return nil, err
	}
	locker, err := startDesktopLockerOnBus(ctx, bus)
	if err != nil {
		_ = bus.close()
		return nil, err
	}
	return locker, nil
}

func startDesktopLockerOnBus(ctx context.Context, bus loginBus) (*desktopLocker, error) {
	locker := &desktopLocker{bus: bus}
	for _, service := range screenSaverServices {
		if err := matchLockerBus(ctx, bus,
			dbus.WithMatchSender(sessionBusName),
			dbus.WithMatchObjectPath(sessionBusPath),
			dbus.WithMatchInterface(sessionBusName),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, service.name),
		); err != nil {
			return nil, err
		}
		body, err := callLockerBus(ctx, bus, sessionBusName, sessionBusPath, sessionBusName+".GetNameOwner", service.name)
		if serviceUnknown(err) {
			continue
		}
		if err != nil || dbus.Store(body, &service.owner) != nil || !strings.HasPrefix(service.owner, ":") {
			return nil, errors.New("desktop locker owner lookup failed")
		}
		if err := matchLockerBus(ctx, bus,
			dbus.WithMatchSender(service.owner),
			dbus.WithMatchObjectPath(service.path),
			dbus.WithMatchInterface(service.iface),
			dbus.WithMatchMember("ActiveChanged"),
		); err != nil {
			return nil, err
		}
		body, err = callLockerBus(ctx, bus, service.owner, service.path, service.iface+".GetActive")
		var active bool
		if err != nil || dbus.Store(body, &active) != nil {
			return nil, errors.New("desktop locker state lookup failed")
		}
		if active {
			return nil, errors.New("desktop locker is active")
		}
		locker.services = append(locker.services, service)
	}
	if len(locker.services) == 0 {
		return nil, errors.New("no supported desktop locker is available")
	}
	return locker, nil
}

func (locker *desktopLocker) signals() <-chan *dbus.Signal {
	if locker == nil {
		return nil
	}
	return locker.bus.signals()
}

func (locker *desktopLocker) done() <-chan struct{} {
	if locker == nil {
		return nil
	}
	return locker.bus.done()
}

func (locker *desktopLocker) close() error {
	if locker == nil {
		return nil
	}
	return locker.bus.close()
}

func (locker *desktopLocker) state(ctx context.Context) (agentlifecycle.Event, error) {
	if locker == nil {
		return "", nil
	}
	for _, service := range locker.services {
		body, err := callLockerBus(ctx, locker.bus, service.owner, service.path, service.iface+".GetActive")
		var active bool
		if err != nil || dbus.Store(body, &active) != nil {
			return "", errors.New("desktop locker state lookup failed")
		}
		if active {
			return agentlifecycle.EventSessionLock, nil
		}
	}
	return "", nil
}

func (locker *desktopLocker) signalEvent(signal *dbus.Signal) (agentlifecycle.Event, error) {
	if signal == nil {
		return "", errors.New("invalid desktop locker signal")
	}
	if signal.Sender == sessionBusName && signal.Path == sessionBusPath && signal.Name == sessionBusName+".NameOwnerChanged" {
		var name, old, next string
		if dbus.Store(signal.Body, &name, &old, &next) != nil {
			return "", errors.New("invalid desktop locker owner signal")
		}
		// A supported locker that was absent during setup must still fence a
		// later appearance; it may already be active by the time it appears.
		for _, service := range screenSaverServices {
			if name == service.name && (old != service.owner || next != service.owner) {
				return "", errors.New("desktop locker service owner changed")
			}
		}
		return "", nil
	}
	for _, service := range locker.services {
		if signal.Sender != service.owner || signal.Path != service.path || signal.Name != service.iface+".ActiveChanged" {
			continue
		}
		var active bool
		if dbus.Store(signal.Body, &active) != nil {
			return "", errors.New("invalid desktop locker active signal")
		}
		if active {
			return agentlifecycle.EventSessionLock, nil
		}
		return "", nil
	}
	return "", nil
}

func callLockerBus(ctx context.Context, bus loginBus, destination string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	bounded, cancel := context.WithTimeout(ctx, linuxCallBudget)
	defer cancel()
	return bus.call(bounded, destination, path, method, args...)
}

func matchLockerBus(ctx context.Context, bus loginBus, options ...dbus.MatchOption) error {
	bounded, cancel := context.WithTimeout(ctx, linuxCallBudget)
	defer cancel()
	return bus.match(bounded, options...)
}

func serviceUnknown(err error) bool {
	var dbusErr *dbus.Error
	if errors.As(err, &dbusErr) && dbusErr != nil {
		return missingLockerService(dbusErr.Name)
	}
	var value dbus.Error // Remote godbus replies use the value form.
	return errors.As(err, &value) && missingLockerService(value.Name)
}

func missingLockerService(name string) bool {
	return name == sessionBusName+".Error.ServiceUnknown" || name == sessionBusName+".Error.NameHasNoOwner"
}

type sessionBusEndpoint struct {
	name string
}

func parseSessionBusAddress(address string) (sessionBusEndpoint, error) {
	if address == "" || strings.TrimSpace(address) != address || strings.ContainsAny(address, "\x00\r\n;") || !strings.HasPrefix(address, "unix:") {
		return sessionBusEndpoint{}, errors.New("session bus must be one local Unix socket")
	}
	var endpoint sessionBusEndpoint
	hasPath, hasAbstract, hasGUID := false, false, false
	for _, part := range strings.Split(strings.TrimPrefix(address, "unix:"), ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" {
			return sessionBusEndpoint{}, errors.New("invalid session bus address")
		}
		decoded, err := url.PathUnescape(value)
		if err != nil || strings.ContainsRune(decoded, 0) {
			return sessionBusEndpoint{}, errors.New("invalid session bus address")
		}
		switch key {
		case "path":
			if hasPath || hasAbstract || !filepath.IsAbs(decoded) {
				return sessionBusEndpoint{}, errors.New("invalid session bus path")
			}
			hasPath, endpoint.name = true, decoded
		case "abstract":
			if hasPath || hasAbstract || decoded == "" {
				return sessionBusEndpoint{}, errors.New("invalid session bus abstract address")
			}
			hasAbstract, endpoint.name = true, "@"+decoded
		case "guid":
			if hasGUID || len(decoded) != 32 {
				return sessionBusEndpoint{}, errors.New("invalid session bus guid")
			}
			if _, err := hex.DecodeString(decoded); err != nil {
				return sessionBusEndpoint{}, errors.New("invalid session bus guid")
			}
			hasGUID = true
		default:
			return sessionBusEndpoint{}, errors.New("unsupported session bus address option")
		}
	}
	if !hasPath && !hasAbstract {
		return sessionBusEndpoint{}, errors.New("session bus endpoint is missing")
	}
	return endpoint, nil
}

func connectSessionLockerBus(ctx context.Context, address string) (loginBus, error) {
	endpoint, err := parseSessionBusAddress(address)
	if err != nil {
		return nil, err
	}
	raw, err := (&net.Dialer{Timeout: linuxCallBudget}).DialContext(ctx, "unix", endpoint.name)
	if err != nil {
		return nil, errors.New("session bus connect failed")
	}
	deadline := time.Now().Add(linuxCallBudget)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := raw.SetDeadline(deadline); err != nil {
		raw.Close()
		return nil, errors.New("session bus setup failed")
	}
	if err := sameUserUnixPeer(raw); err != nil {
		raw.Close()
		return nil, err
	}
	handler := &loginSignals{queue: make(chan *dbus.Signal, 64), stopped: make(chan struct{})}
	conn, err := dbus.NewConn(raw, dbus.WithSignalHandler(handler))
	if err != nil {
		raw.Close()
		return nil, errors.New("session bus setup failed")
	}
	if err := conn.Auth([]dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Geteuid()))}); err != nil {
		conn.Close()
		return nil, errors.New("session bus authentication failed")
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, errors.New("session bus handshake failed")
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, errors.New("session bus setup failed")
	}
	return &systemLoginBus{conn: conn, handler: handler}, nil
}

func sameUserUnixPeer(raw net.Conn) error {
	conn, ok := raw.(*net.UnixConn)
	if !ok {
		return errors.New("session bus is not a Unix connection")
	}
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return errors.New("session bus peer check failed")
	}
	var credential *unix.Ucred
	var credentialErr error
	if err := rawConn.Control(func(fd uintptr) {
		credential, credentialErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || credentialErr != nil || credential == nil {
		return errors.New("session bus peer credentials are unavailable")
	}
	if credential.Uid != uint32(os.Geteuid()) || credential.Pid <= 0 {
		return errors.New("session bus is not owned by this user")
	}
	return nil
}
