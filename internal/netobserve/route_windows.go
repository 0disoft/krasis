//go:build windows

package netobserve

import (
	"crypto/sha256"
	"encoding/binary"
	"net"
	"net/netip"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getBestRoute2 = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetBestRoute2")

func bestRoute(ip netip.Addr) int {
	i, _ := routeDetails(ip)
	return i
}

func routeDetails(ip netip.Addr) (int, [32]byte) {
	var unknown [32]byte
	if getBestRoute2.Find() != nil {
		return 0, unknown
	}
	var destination windows.RawSockaddrInet6
	ip = ip.Unmap()
	if ip.Is4() {
		v4 := (*windows.RawSockaddrInet4)(unsafe.Pointer(&destination))
		v4.Family = windows.AF_INET
		v4.Addr = ip.As4()
	} else if ip.Is6() {
		destination.Family = windows.AF_INET6
		destination.Addr = ip.As16()
		if zone := ip.Zone(); zone != "" {
			n, e := strconv.Atoi(zone)
			if e != nil {
				i, e := net.InterfaceByName(zone)
				if e != nil {
					return 0, unknown
				}
				n = i.Index
			}
			destination.Scope_id = uint32(n)
		}
	} else {
		return 0, unknown
	}
	var route windows.MibIpForwardRow2
	var source windows.RawSockaddrInet6
	r, _, _ := getBestRoute2.Call(0, 0, 0, uintptr(unsafe.Pointer(&destination)), 0, uintptr(unsafe.Pointer(&route)), uintptr(unsafe.Pointer(&source)))
	if r != 0 {
		return 0, unknown
	}
	// Ignore age/lifetimes and volatile flags. Source/prefix/next-hop/metric
	// changes alter the route revision even when the NIC index stays the same.
	h := sha256.New()
	for _, v := range []any{route.InterfaceLuid, route.InterfaceIndex, route.DestinationPrefix, route.NextHop, route.Metric, source} {
		_ = binary.Write(h, binary.LittleEndian, v)
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return int(route.InterfaceIndex), digest
}
