//go:build !windows

package peer

func runNetworkReader(read func()) { read() }
