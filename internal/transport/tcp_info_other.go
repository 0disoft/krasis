//go:build !windows

package transport

import "net"

func socketTCPInfo(net.Conn) ([4]uint32, bool) { return [4]uint32{}, false }
