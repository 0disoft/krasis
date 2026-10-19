//go:build !windows

package transport

import "net"

func configureConnectionACK(net.Conn) string { return "tcp_ack_frequency_unsupported" }
