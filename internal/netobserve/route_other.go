//go:build !windows

package netobserve

import "net/netip"

func bestRoute(netip.Addr) int                { return 0 }
func routeDetails(netip.Addr) (int, [32]byte) { return 0, [32]byte{} }
