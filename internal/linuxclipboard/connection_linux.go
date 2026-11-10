package linuxclipboard

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const xTimeout = 2 * time.Second

func connectX(ctx context.Context, display string) (*xgb.Conn, net.Conn, error) {
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil, nil, ErrWayland
	}
	if display == "" {
		display = os.Getenv("DISPLAY")
	}
	d, err := parseDisplay(display)
	if err != nil {
		return nil, nil, err
	}
	wire, err := (&net.Dialer{Timeout: xTimeout}).DialContext(ctx, d.network, d.address)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, errors.New("cannot connect to X11 display")
	}
	stop := context.AfterFunc(ctx, func() { _ = wire.Close() })
	defer stop()
	deadline := time.Now().Add(xTimeout)
	contextDeadline := false
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
		contextDeadline = true
	}
	_ = wire.SetDeadline(deadline)
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".Xauthority")
	}
	cookie := ""
	if f, openErr := os.Open(path); openErr == nil {
		host, _ := os.Hostname()
		if d.host != "" && d.host != "localhost" {
			host = d.host
		}
		cookie, err = authorityCookie(io.LimitReader(f, 1<<20), host, d.number)
		_ = f.Close()
		if err != nil && err != io.EOF {
			_ = wire.Close()
			return nil, nil, errors.New("invalid Xauthority file")
		}
	}
	var c *xgb.Conn
	if cookie != "" {
		c, err = xgb.NewConnNetWithCookieHex(wire, cookie)
	} else {
		c, err = xgb.NewConnNet(wire)
	}
	if err != nil {
		_ = wire.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		// The socket deadline can fire before the context timer is scheduled.
		var timed net.Error
		if contextDeadline && errors.As(err, &timed) && timed.Timeout() {
			return nil, nil, context.DeadlineExceeded
		}
		return nil, nil, errors.New("X11 handshake or authentication failed")
	}
	c.DisplayNumber, _ = strconv.Atoi(d.number)
	c.DefaultScreen = d.screen
	if d.screen >= len(xproto.Setup(c).Roots) {
		_ = wire.Close()
		c.Close()
		return nil, nil, errors.New("X11 screen does not exist")
	}
	if ctx.Err() != nil {
		_ = wire.Close()
		c.Close()
		return nil, nil, ctx.Err()
	}
	return c, wire, nil
}
