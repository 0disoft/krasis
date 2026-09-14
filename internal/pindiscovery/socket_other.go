//go:build !windows

package pindiscovery

import (
	"errors"
	"net"
)

func enableBroadcast(*net.UDPConn) error { return errors.New("PIN discovery is Windows-only") }
