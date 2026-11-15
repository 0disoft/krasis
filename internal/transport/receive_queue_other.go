//go:build !windows

package transport

import "net"

func socketReceiveQueue(net.Conn) (uint32, bool) { return 0, false }
