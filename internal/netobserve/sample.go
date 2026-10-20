package netobserve

import (
	"crypto/sha256"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

func Sample() Snapshot {
	power, saver := powerStatus()
	s := sampleInterfaces(net.Interfaces, func(i net.Interface) ([]net.Addr, error) { return i.Addrs() })
	s.Power, s.Saver = power, saver
	return s
}

func sampleInterfaces(list func() ([]net.Interface, error), addresses func(net.Interface) ([]net.Addr, error)) Snapshot {
	s := Snapshot{Link: LinkUnknown}
	interfaces, err := list()
	if err != nil {
		return s
	}
	complete, usable := true, false
	var parts []string
	for _, i := range interfaces {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		parts = append(parts, strconv.Itoa(i.Index)+":"+strconv.FormatUint(uint64(i.Flags), 10))
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := addresses(i)
		if err != nil {
			complete = false
			continue
		}
		for _, a := range addrs {
			prefix, err := netip.ParsePrefix(a.String())
			if err != nil {
				complete = false
				continue
			}
			ip := prefix.Addr()
			if !ip.IsGlobalUnicast() {
				continue
			}
			usable = true
			parts = append(parts, strconv.Itoa(i.Index)+":"+prefix.String())
		}
	}
	if usable {
		s.Link = LinkUsable
	} else if complete {
		s.Link = LinkDown
	}
	if complete {
		sort.Strings(parts)
		s.topology, s.topologyKnown = sha256.Sum256([]byte(strings.Join(parts, "\n"))), true
	}
	return s
}
