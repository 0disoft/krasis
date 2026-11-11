package x11desktop

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/syncgate"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const operationTimeout = 750 * time.Millisecond

var ErrWayland = errors.New("native Wayland input requires an InputCapture/RemoteDesktop portal backend; select an X11 session")

type connection struct {
	x         *xgb.Conn
	wire      net.Conn
	mu        syncgate.Mutex
	root      xproto.Window
	closeOnce sync.Once
	closeErr  error
}

// Only the current local X server is accepted. No unbounded TCP dial, root
// privileges, xhost modifications, or remote DISPLAY authentication fallback.
func localDisplay(display string) (string, int, error) {
	if !strings.HasPrefix(display, ":") {
		return "", 0, errors.New("a local :N[.screen] X11 DISPLAY is required")
	}
	parts := strings.Split(strings.TrimPrefix(display, ":"), ".")
	if len(parts) > 2 {
		return "", 0, errors.New("invalid DISPLAY")
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 0 || n > 65535 {
		return "", 0, errors.New("invalid X11 display number")
	}
	screen := 0
	if len(parts) == 2 {
		screen, err = strconv.Atoi(parts[1])
		if err != nil || screen < 0 || screen > 255 {
			return "", 0, errors.New("invalid X11 screen")
		}
	}
	return strconv.Itoa(n), screen, nil
}

func openConnection(ctx context.Context) (*connection, error) {
	if ctx == nil {
		return nil, errors.New("X11 context required")
	}
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil, ErrWayland
	}
	display, screen, err := localDisplay(os.Getenv("DISPLAY"))
	if err != nil {
		return nil, err
	}
	wire, err := (&net.Dialer{Timeout: operationTimeout}).DialContext(ctx, "unix", "/tmp/.X11-unix/X"+display)
	if err != nil {
		return nil, fmt.Errorf("connect X11: %w", err)
	}
	deadline := time.Now().Add(operationTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = wire.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = wire.Close() })
	cookie, err := authorityCookie(display)
	if err != nil {
		stop()
		_ = wire.Close()
		return nil, err
	}
	var x *xgb.Conn
	if cookie != "" {
		x, err = xgb.NewConnNetWithCookieHex(wire, cookie)
	} else {
		x, err = xgb.NewConnNet(wire)
	}
	stop()
	if err != nil {
		_ = wire.Close()
		return nil, err
	}
	_ = wire.SetDeadline(time.Time{})
	setup := xproto.Setup(x)
	if screen >= len(setup.Roots) {
		_ = wire.Close()
		x.Close()
		return nil, errors.New("X11 screen unavailable")
	}
	c := &connection{x: x, wire: wire, root: setup.Roots[screen].Root}
	if err = ctx.Err(); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

func (c *connection) close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.wire.Close()
		// XGB's read loop can have already closed the same Unix socket.
		// That proves retirement; any other first close failure is retained.
		if errors.Is(c.closeErr, net.ErrClosed) {
			c.closeErr = nil
		}
		if c.x != nil {
			c.x.Close()
		}
	})
	return c.closeErr
}

// One bounded operation owns both the socket deadline and cancellation. A
// canceled/failed request closes this native owner; it is never replayed.
func (c *connection) do(ctx context.Context, fn func() error) error {
	if ctx == nil {
		return errors.New("X11 context required")
	}
	if err := c.mu.LockContext(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(operationTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.wire.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.close() })
	err := fn()
	if !stop() {
		return errors.Join(err, ctx.Err())
	}
	_ = c.wire.SetDeadline(time.Time{})
	if err != nil {
		_ = c.close()
	}
	return errors.Join(err, ctx.Err())
}

// The X authority format uses big-endian counted strings. Never include cookie
// data in errors. A missing file is allowed for local server peer-credential ACLs.
func authorityCookie(display string) (string, error) {
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, ".Xauthority")
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("cannot read X11 authority")
	}
	defer f.Close()
	r := io.LimitReader(f, 1<<20)
	host, _ := os.Hostname()
	read := func() ([]byte, error) {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		b := make([]byte, int(n))
		_, err := io.ReadFull(r, b)
		return b, err
	}
	for {
		var family uint16
		err := binary.Read(r, binary.BigEndian, &family)
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", errors.New("invalid X11 authority")
		}
		var fields [4][]byte
		for i := range fields {
			fields[i], err = read()
			if err != nil {
				return "", errors.New("invalid X11 authority")
			}
		}
		if (family == 65535 || family == 256 && string(fields[0]) == host) && (string(fields[1]) == display || len(fields[1]) == 0) && string(fields[2]) == "MIT-MAGIC-COOKIE-1" && len(fields[3]) == 16 {
			return hex.EncodeToString(fields[3]), nil
		}
	}
}
